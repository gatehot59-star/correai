// Copyright (c) 2026 Jorge Abraham Mendieta.
// Computational Substrate Theory. Todos los derechos reservados.

package gateway

import (
	"math"
	"sync"
)

const (
	DefaultAlpha              = 0.20
	DefaultSigma              = 1.00
	DefaultEpsilon            = 1e-8
	DefaultCoherenceThreshold = 0.75
	ContextVectorSize         = 8
	DefaultFastAlpha          = 0.30
	DefaultMediumAlpha        = 0.10
)

// HuberFilter implementa rate limiting estocástico con varianza asimétrica
// de Huber: EWMA, Winsorización a ±3σ y gatillo por derivada de varianza.
//
// FIX 1: f.s (EWMA de inercia) ahora se incorpora al umbral dinámico,
//
//	dándole uso real en lugar de ser código muerto.
//
// FIX 2: el caller ahora pasa bytes reales en lugar del valor fijo 1.0.
//
// FIX D-26: Update es ahora segura para uso concurrente.
//
// El detector de carreras del runtime reportó 27 carreras citando 16 líneas de
// este archivo, porque getOrCreateAgent entrega el MISMO *AgentState a toda
// conexión que presente el mismo certificado, y handleConn llama a Update
// fuera de agent.mu. El candado vive acá, en el tipo, y no en el call site:
// el puntero es compartido por construcción, así que la seguridad tiene que
// ser una propiedad del filtro y no una regla que cada llamador futuro deba
// recordar.
//
// El campo se llama mtx y NO mu porque mu ya existe en este struct y es la
// media Winsorizada, no un mutex.
//
// LÍMITE DECLARADO: esto elimina la carrera de datos, no la mezcla semántica.
// Dos conexiones del mismo agente siguen alimentando una sola EWMA.
//
// ===========================================================================
// FIX E0: LA PRIMERA MUESTRA INICIALIZA EL FILTRO, NO PUEDE SER UNA ANOMALÍA.
//
// El gateway rechazaba el primer paquete de TODA conexión. Medido con el primer
// cliente mTLS del proyecto: 0 de 11 combinaciones de payload (8 a 512 B) y
// espera (0 a 1,9 s) lograban un ACK, y como el rechazo dispara TriggerBlock y
// cierra, ninguna conexión pasaba nunca de un paquete.
//
// La causa aritmética: sobre un filtro recién creado lastV = 0, así que
//
//	derivative = (v - lastV)/dt = v/dt
//
// o sea la varianza entera dividida por un dt chico, contra un umbral que es
// una fracción de sqrt(v). Con dt de microsegundos eso son cuatro órdenes de
// magnitud de diferencia.
//
// EL ARGUMENTO ES MÁS FUERTE QUE EL BUG: no se puede detectar un CAMBIO de
// varianza con UNA muestra. Un filtro que marca anomalía en su primera
// observación no está midiendo una desviación, está midiendo que nació.
//
// Así que la primera muestra inicializa s y mu con el valor observado (que es
// la inicialización estándar de una EWMA, en vez del sesgo de arrancar en cero)
// y devuelve false sin evaluar el gatillo.
//
// EL CONTROL QUE HACE QUE ESTO NO SEA ROMPER EL PRODUCTO: una ráfaga real sigue
// bloqueada. Modelado sobre la aritmética del filtro, tras 30 paquetes estables:
//
//	 ráfaga  1 KB -> BLOQUEA (derivada 3,15 contra umbral 0,021)
//	 ráfaga  4 KB -> BLOQUEA (57,5 contra 0,121)
//	 ráfaga 16 KB -> BLOQUEA (1.184 contra 0,839)
//	 ráfaga 64 KB -> BLOQUEA (20.093 contra 5,139)
//
// LO QUE ESTE FIX NO ARREGLA, y está cuantificado: D-48 sigue vivo. Con payloads
// VARIABLES el filtro bloquea 46 de 50 paquetes, antes y después del fix. La
// tolerancia depende de dt:
//
//	dt = 1 ms   -> bloquea con +1,6%  de cambio de tamaño
//	dt = 40 ms  -> bloquea con +10,9%
//	dt = 1 s    -> bloquea con +243,8%
//
// Cuanto más rápido habla el agente, MENOS variación tolera. Eso es al revés de
// lo que un rate limiter debería hacer, y es exactamente la inconsistencia
// dimensional de D-48 (derivada en var/s contra un umbral en unidades de
// desvío). No se arregla acá: hay que reemplazar el algoritmo por Page-Hinkley,
// y eso es una decisión de producto.
// ===========================================================================
type HuberFilter struct {
	mtx   sync.Mutex
	alpha float64
	sigma float64
	s     float64 // S_t, EWMA de inercia — ahora usado en threshold
	mu    float64 // media Winsorizada (NO es un mutex)
	v     float64 // varianza de Huber
	lastV float64 // varianza previa para derivada

	// visto marca si el filtro ya procesó al menos una muestra. Con visto=false
	// no hay varianza previa contra la que comparar, así que no hay derivada que
	// evaluar. Es el estado que faltaba: antes, "sin historia" y "varianza cero"
	// eran indistinguibles, y el filtro trataba lo primero como lo segundo.
	visto bool
}

func NewHuberFilter(alpha, sigma float64) *HuberFilter {
	if alpha <= 0 || alpha > 1 {
		alpha = DefaultAlpha
	}
	if sigma <= 0 {
		sigma = DefaultSigma
	}

	return &HuberFilter{
		alpha: alpha,
		sigma: sigma,
	}
}

// Update procesa una muestra x (bytes recibidos normalizados) y dt (segundos
// desde el último paquete). Devuelve true si debe activarse el bloqueo
// preventivo por derivada excesiva de varianza.
//
// FIX: el umbral dinámico ahora incorpora f.s (inercia del sistema) para
// distinguir ráfagas legítimas sostenidas de picos anómalos.
//
// FIX D-26: toma f.mtx durante toda la actualización. Cada llamada es atómica;
// la SECUENCIA de llamadas no lo es, y eso es deliberado.
//
// FIX E0: la primera muestra inicializa y devuelve false. Ver el comentario del
// tipo para el argumento y para los números del control positivo.
func (f *HuberFilter) Update(x float64, dt float64) bool {
	f.mtx.Lock()
	defer f.mtx.Unlock()

	if f.alpha <= 0 || f.alpha > 1 {
		f.alpha = DefaultAlpha
	}
	if f.sigma <= 0 {
		f.sigma = DefaultSigma
	}
	if dt <= 1e-9 {
		dt = 1e-9
	}

	// FIX E0. La primera muestra no tiene con qué compararse: inicializa el
	// estado y sale. Un filtro que marca anomalía con n=1 no mide una desviación.
	if !f.visto {
		f.visto = true
		f.s = x
		f.mu = x
		f.v = 0
		f.lastV = 0
		return false
	}

	// EWMA de inercia: S_t = (1-α)·S_{t-1} + α·x_t
	// FIX: ahora se usa en el cálculo del umbral dinámico.
	f.s = (1-f.alpha)*f.s + f.alpha*x

	// Winsorización: absorción de ráfagas cortas legítimas.
	delta := x - f.mu
	clamp := 3 * f.sigma
	if delta > clamp {
		delta = clamp
	} else if delta < -clamp {
		delta = -clamp
	}
	f.mu = f.mu + f.alpha*delta

	// Varianza de Huber asimila el residuo real.
	residual := x - f.mu
	f.lastV = f.v
	f.v = (1-f.alpha)*f.v + f.alpha*residual*residual

	// Derivada de varianza normalizada por dt.
	derivative := (f.v - f.lastV) / dt

	// FIX: umbral dinámico incorpora f.s para tolerar ráfagas
	// sostenidas legítimas (alta inercia = mayor tolerancia).
	inertiaFactor := 1.0 + math.Log1p(f.s)
	threshold := 0.05 * math.Sqrt(f.v+DefaultEpsilon) * inertiaFactor

	return derivative > threshold || derivative < -threshold
}

// CoherenceFilter implementa el factor de coherencia de estado mediante
// similitud coseno entre un filtro rápido y uno medio.
//
// FIX D-26: Update es ahora segura para uso concurrente. Mismo motivo y mismo
// límite declarado que en HuberFilter.
type CoherenceFilter struct {
	mtx         sync.Mutex
	fastAlpha   float64
	mediumAlpha float64
	fast        [ContextVectorSize]float64
	medium      [ContextVectorSize]float64
	epsilon     float64
	threshold   float64
	last        float64
}

func NewCoherenceFilter(fastAlpha, mediumAlpha, threshold float64) *CoherenceFilter {
	if fastAlpha <= 0 || fastAlpha > 1 {
		fastAlpha = DefaultFastAlpha
	}
	if mediumAlpha <= 0 || mediumAlpha > 1 {
		mediumAlpha = DefaultMediumAlpha
	}
	if threshold <= 0 {
		threshold = DefaultCoherenceThreshold
	}

	return &CoherenceFilter{
		fastAlpha:   fastAlpha,
		mediumAlpha: mediumAlpha,
		epsilon:     DefaultEpsilon,
		threshold:   threshold,
		last:        1.0,
	}
}

// Update recibe el vector de contexto y devuelve true si la coherencia
// cae drásticamente, indicando desalineamiento anómalo.
//
// FIX D-26: toma c.mtx durante toda la actualización. Los dos arreglos EWMA y
// c.last se leían y escribían sin protección; eran 22 de las citas del
// detector.
func (c *CoherenceFilter) Update(vec [ContextVectorSize]float64) bool {
	c.mtx.Lock()
	defer c.mtx.Unlock()

	for i := range vec {
		c.fast[i] = (1-c.fastAlpha)*c.fast[i] + c.fastAlpha*vec[i]
		c.medium[i] = (1-c.mediumAlpha)*c.medium[i] + c.mediumAlpha*vec[i]
	}

	var dot, normF, normM float64
	for i := 0; i < ContextVectorSize; i++ {
		dot += c.fast[i] * c.medium[i]
		normF += c.fast[i] * c.fast[i]
		normM += c.medium[i] * c.medium[i]
	}

	denominator := normF*normM + c.epsilon
	if denominator == 0 {
		denominator = DefaultEpsilon
	}

	coh := (dot * dot) / denominator
	anomalous := coh < c.threshold || coh < c.last*0.5
	c.last = coh

	return anomalous
}
