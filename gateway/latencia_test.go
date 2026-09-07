// Copyright (c) 2026 Jorge Abraham Mendieta.
// Computational Substrate Theory. Todos los derechos reservados.

// Distribucion de latencia de la espera del candado que cerro D-26.
//
// El benchmark del candado dejo un NO MEDIDO explicito: "latencia de cola: todo
// lo de arriba es promedio. El p99 de la espera en el caso patologico no se
// midio, y para un gateway el p99 puede importar mas que la media".
//
// EL PROBLEMA QUE DECIDE SI ESTE NUMERO VALE. Cronometrar una operacion de 10 ns
// con un reloj que cuesta decenas de nanosegundos es medir el reloj, no la
// operacion. Por eso lo PRIMERO que hace este archivo es medir el costo del par
// time.Now() y publicarlo como PISO DE RUIDO, y hay un guard que falla si el p99
// medido no lo supera por un margen. Sin ese paso, todo lo de abajo seria un
// relato con forma de numero.
//
// DOS SUJETOS, porque no son la misma pregunta:
//
//  1. La ESPERA del candado: cuanto se tarda en ADQUIRIRLO. Se mide directo
//     sobre un sync.Mutex bajo el mismo patron de acceso que los filtros.
//     Es un PROXY DECLARADO: la misma primitiva y el mismo patron, no el mismo
//     callsite, porque medir dentro de Update exigiria instrumentar codigo de
//     produccion con un reloj que cuesta mas que la seccion critica.
//
//  2. La LATENCIA COMPLETA del par de Update que handleConn hace por paquete.
//     Ese es el numero que le importa al gateway, y ahi el reloj pesa mucho
//     menos porque la operacion medida es ~10x mas grande.
//
// TRES BRAZOS y su control positivo: 1 agente compartido (patologico), N agentes
// aislados (el caso real), N agentes con UN candado global. Si el p99 del candado
// global no resulta peor que el del candado por agente, este arnes no puede ver
// cola y ninguno de sus numeros es confiable.
//
// Nada se asigna durante la medicion: las muestras van a slices preasignados por
// goroutine, y los cuantiles se calculan despues.

package gateway

import (
	"fmt"
	"runtime"
	"sort"
	"sync"
	"testing"
	"time"
)

// distribucion son los cuantiles de una tanda de muestras en nanosegundos.
type distribucion struct {
	etiqueta string
	n        int
	p50      int64
	p90      int64
	p99      int64
	p999     int64
	max      int64
	media    float64
}

func (d distribucion) String() string {
	return fmt.Sprintf("%-46s n=%-9d media=%8.1f  p50=%6d  p90=%7d  p99=%8d  p99.9=%9d  max=%10d",
		d.etiqueta, d.n, d.media, d.p50, d.p90, d.p99, d.p999, d.max)
}

// cuantiles ordena las muestras y extrae la distribucion. Recibe el slice por
// valor y lo ordena in place: el llamador ya no lo necesita.
func cuantiles(etiqueta string, muestras []int64) distribucion {
	if len(muestras) == 0 {
		return distribucion{etiqueta: etiqueta}
	}

	sort.Slice(muestras, func(i, j int) bool { return muestras[i] < muestras[j] })

	idx := func(q float64) int64 {
		i := int(q * float64(len(muestras)))
		if i >= len(muestras) {
			i = len(muestras) - 1
		}
		if i < 0 {
			i = 0
		}
		return muestras[i]
	}

	var suma int64
	for _, v := range muestras {
		suma += v
	}

	return distribucion{
		etiqueta: etiqueta,
		n:        len(muestras),
		p50:      idx(0.50),
		p90:      idx(0.90),
		p99:      idx(0.99),
		p999:     idx(0.999),
		max:      muestras[len(muestras)-1],
		media:    float64(suma) / float64(len(muestras)),
	}
}

// ---------------------------------------------------------------------------
// PISO DE RUIDO: cuanto cuesta el propio instrumento
// ---------------------------------------------------------------------------

// pisoDelReloj mide el costo de un par time.Now() sin nada en el medio. Es el
// suelo por debajo del cual este archivo NO PUEDE MEDIR NADA: si un p99 de mas
// abajo esta en el mismo orden que este, ese p99 es del reloj.
//
// Se mide con `goroutines` corriendo a la vez porque el costo del reloj tambien
// se degrada bajo carga, y usarlo como piso medido en serie seria un piso
// optimista.
func pisoDelReloj(goroutines, porGoroutine int) distribucion {
	muestras := make([][]int64, goroutines)
	for g := range muestras {
		muestras[g] = make([]int64, 0, porGoroutine)
	}

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func(g int) {
			defer wg.Done()
			for i := 0; i < porGoroutine; i++ {
				t0 := time.Now()
				t1 := time.Now()
				muestras[g] = append(muestras[g], t1.Sub(t0).Nanoseconds())
			}
		}(g)
	}
	wg.Wait()

	return cuantiles(fmt.Sprintf("PISO: par time.Now() (g=%d)", goroutines), aplanar(muestras))
}

func aplanar(por [][]int64) []int64 {
	total := 0
	for _, s := range por {
		total += len(s)
	}
	plano := make([]int64, 0, total)
	for _, s := range por {
		plano = append(plano, s...)
	}
	return plano
}

// ---------------------------------------------------------------------------
// SUJETO 1: la espera para adquirir el candado
// ---------------------------------------------------------------------------

// trabajoSinteticoNs aproxima la duracion de la seccion critica de
// CoherenceFilter.Update, que es la que el perfil de mutex senalo como dos
// tercios del bloqueo. No pretende ser identica: pretende que el candado se
// mantenga tomado un rato comparable, que es lo que produce la cola.
var sumidero float64

func trabajoSintetico(vec [ContextVectorSize]float64) {
	var dot, normF float64
	for i := 0; i < ContextVectorSize; i++ {
		dot += vec[i] * vec[i]
		normF += vec[i]
	}
	sumidero = dot / (normF + DefaultEpsilon)
}

// medirEspera cronometra SOLO la adquisicion del candado, con `goroutines`
// disputandolo. `candados` decide el brazo: uno solo = todos se pelean; uno por
// goroutine = nadie se pelea.
func medirEspera(etiqueta string, goroutines, porGoroutine int, candados []*sync.Mutex) distribucion {
	vec := contextoDePrueba()
	muestras := make([][]int64, goroutines)
	for g := range muestras {
		muestras[g] = make([]int64, 0, porGoroutine)
	}

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func(g int) {
			defer wg.Done()
			mu := candados[g%len(candados)]
			for i := 0; i < porGoroutine; i++ {
				t0 := time.Now()
				mu.Lock()
				espera := time.Since(t0).Nanoseconds()
				trabajoSintetico(vec)
				mu.Unlock()
				muestras[g] = append(muestras[g], espera)
			}
		}(g)
	}
	wg.Wait()

	return cuantiles(etiqueta, aplanar(muestras))
}

// ---------------------------------------------------------------------------
// SUJETO 2: la latencia completa del par de Update por paquete
// ---------------------------------------------------------------------------

// medirLatenciaPorPaquete cronometra las DOS llamadas que handleConn hace por
// paquete, sobre los filtros REALES. Es el numero que le importa al gateway.
func medirLatenciaPorPaquete(etiqueta string, goroutines, porGoroutine int, agentes []*AgentState) distribucion {
	vec := contextoDePrueba()
	muestras := make([][]int64, goroutines)
	for g := range muestras {
		muestras[g] = make([]int64, 0, porGoroutine)
	}

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func(g int) {
			defer wg.Done()
			ag := agentes[g%len(agentes)]
			for i := 0; i < porGoroutine; i++ {
				t0 := time.Now()
				ag.Huber.Update(float64(i%17)+0.5, 0.01)
				ag.Coherence.Update(vec)
				muestras[g] = append(muestras[g], time.Since(t0).Nanoseconds())
			}
		}(g)
	}
	wg.Wait()

	return cuantiles(etiqueta, aplanar(muestras))
}

// ---------------------------------------------------------------------------
// LA MEDICION
// ---------------------------------------------------------------------------

// TestLatenciaP99DeLaEspera produce la distribucion completa y falla si el
// instrumento no puede sostener sus propias afirmaciones.
//
// Los dos guards que lo hacen falsable:
//
//  1. El p99 del caso patologico tiene que superar el PISO DEL RELOJ por al
//     menos 3x. Si no, lo medido es el reloj.
//  2. El p99 del candado global tiene que ser peor que el del candado por
//     agente. Si no, el arnes no ve cola.
func TestLatenciaP99DeLaEspera(t *testing.T) {
	if testing.Short() {
		t.Skip("depende del tiempo de pared; se corre en el job de latencia")
	}

	const (
		goroutines   = 8
		porGoroutine = 100000
		margenPiso   = 3.0
	)

	t.Logf("GOMAXPROCS=%d NumCPU=%d muestras=%d (%d goroutines x %d)",
		runtime.GOMAXPROCS(0), runtime.NumCPU(),
		goroutines*porGoroutine, goroutines, porGoroutine)

	// --- Piso de ruido, primero, porque decide si el resto significa algo ---
	piso := pisoDelReloj(goroutines, porGoroutine)
	t.Logf("\n=== PISO DE RUIDO DEL INSTRUMENTO ===\n%s", piso)

	// --- Sujeto 1: la espera del candado ---
	unCandado := []*sync.Mutex{{}}
	esperaCompartida := medirEspera(
		"ESPERA: 1 candado, 8 goroutines (patologico)",
		goroutines, porGoroutine, unCandado)

	candadosAislados := make([]*sync.Mutex, goroutines)
	for i := range candadosAislados {
		candadosAislados[i] = &sync.Mutex{}
	}
	esperaAislada := medirEspera(
		"ESPERA: 1 candado por goroutine (caso real)",
		goroutines, porGoroutine, candadosAislados)

	t.Logf("\n=== SUJETO 1: espera para ADQUIRIR el candado (ns) ===\n%s\n%s\n%s",
		piso, esperaAislada, esperaCompartida)

	// --- Sujeto 2: la latencia del par de Update, filtros reales ---
	agenteUnico := []*AgentState{nuevoAgenteCompartido()}
	latCompartida := medirLatenciaPorPaquete(
		"PAQUETE: 1 AgentState compartido (patologico)",
		goroutines, porGoroutine, agenteUnico)

	agentesAislados := make([]*AgentState, goroutines)
	for i := range agentesAislados {
		agentesAislados[i] = nuevoAgenteCompartido()
	}
	latAislada := medirLatenciaPorPaquete(
		"PAQUETE: 1 AgentState por goroutine (caso real)",
		goroutines, porGoroutine, agentesAislados)

	latSerial := medirLatenciaPorPaquete(
		"PAQUETE: 1 goroutine, sin disputa",
		1, porGoroutine, []*AgentState{nuevoAgenteCompartido()})

	t.Logf("\n=== SUJETO 2: latencia del par Huber+Coherence por paquete (ns) ===\n%s\n%s\n%s",
		latSerial, latAislada, latCompartida)

	// --- GUARD 1: el piso del reloj no puede explicar lo medido ---
	factorPiso := float64(esperaCompartida.p99) / float64(piso.p99)
	t.Logf("\nGUARD 1  p99 patologico / p99 del piso = %.1fx (minimo exigido %.1fx)",
		factorPiso, margenPiso)
	if piso.p99 <= 0 {
		t.Fatalf("el piso del reloj dio p99=%d: el instrumento no tiene resolucion "+
			"suficiente y ningun numero de este test vale", piso.p99)
	}
	if factorPiso < margenPiso {
		t.Fatalf("el p99 medido (%d ns) no supera el piso del reloj (%d ns) por %.1fx: "+
			"lo que se esta midiendo puede ser el reloj",
			esperaCompartida.p99, piso.p99, margenPiso)
	}

	// --- GUARD 2: el arnes ve la cola cuando existe ---
	if esperaCompartida.p99 <= esperaAislada.p99 {
		t.Fatalf("el p99 de la espera con UN candado disputado (%d ns) no es peor "+
			"que con un candado por goroutine (%d ns): este arnes no puede ver "+
			"cola, asi que sus numeros no son confiables",
			esperaCompartida.p99, esperaAislada.p99)
	}

	t.Logf("GUARD 2  p99 espera compartida / aislada = %.1fx",
		float64(esperaCompartida.p99)/float64(maxInt64(esperaAislada.p99, 1)))

	// --- El numero que se pidio, aislado ---
	t.Logf("\n=== RESPUESTA ===\n"+
		"p99 de la ESPERA, caso real (1 candado por agente):     %d ns\n"+
		"p99 de la ESPERA, caso patologico (1 candado, 8 gor.):  %d ns\n"+
		"p99.9 patologico: %d ns   max patologico: %d ns\n"+
		"p99 del PAQUETE completo, caso real:                    %d ns\n"+
		"p99 del PAQUETE completo, caso patologico:              %d ns\n"+
		"piso del reloj (p99): %d ns",
		esperaAislada.p99, esperaCompartida.p99,
		esperaCompartida.p999, esperaCompartida.max,
		latAislada.p99, latCompartida.p99, piso.p99)
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

// TestControlPositivoDeCola_CandadoGlobal es el control positivo de la cola.
//
// Compara el p99 del par de Update con candado por agente contra el mismo par
// con un candado GLOBAL, sobre N agentes aislados. Si el global no resulta peor
// en el p99, este arnes mide promedios disfrazados de cuantiles.
func TestControlPositivoDeCola_CandadoGlobal(t *testing.T) {
	if testing.Short() {
		t.Skip("depende del tiempo de pared; se corre en el job de latencia")
	}

	const (
		goroutines   = 8
		porGoroutine = 100000
	)

	vec := contextoDePrueba()

	medir := func(etiqueta string, ejecutar func(g, i int)) distribucion {
		muestras := make([][]int64, goroutines)
		for g := range muestras {
			muestras[g] = make([]int64, 0, porGoroutine)
		}
		var wg sync.WaitGroup
		wg.Add(goroutines)
		for g := 0; g < goroutines; g++ {
			go func(g int) {
				defer wg.Done()
				for i := 0; i < porGoroutine; i++ {
					t0 := time.Now()
					ejecutar(g, i)
					muestras[g] = append(muestras[g], time.Since(t0).Nanoseconds())
				}
			}(g)
		}
		wg.Wait()
		return cuantiles(etiqueta, aplanar(muestras))
	}

	agentes := make([]*AgentState, goroutines)
	for i := range agentes {
		agentes[i] = nuevoAgenteCompartido()
	}
	porAgente := medir("candado por agente (N agentes aislados)", func(g, i int) {
		agentes[g].Huber.Update(float64(i%17)+0.5, 0.01)
		agentes[g].Coherence.Update(vec)
	})

	hubers := make([]*huberCandadoGlobal, goroutines)
	cohs := make([]*coherenciaCandadoGlobal, goroutines)
	for i := range hubers {
		hubers[i] = nuevoHuberGlobal()
		cohs[i] = nuevoCoherenciaGlobal()
	}
	global := medir("candado GLOBAL (N agentes aislados)", func(g, i int) {
		hubers[g].Update(float64(i%17)+0.5, 0.01)
		cohs[g].Update(vec)
	})

	t.Logf("\n=== CONTROL POSITIVO DE COLA (ns) ===\n%s\n%s", porAgente, global)

	if global.p99 <= porAgente.p99 {
		t.Fatalf("el p99 del candado GLOBAL (%d ns) no es peor que el del candado "+
			"por agente (%d ns): el arnes no distingue colas, asi que los cuantiles "+
			"de este archivo no valen", global.p99, porAgente.p99)
	}

	t.Logf("factor p99 global/porAgente = %.1fx   |   factor p99.9 = %.1fx",
		float64(global.p99)/float64(maxInt64(porAgente.p99, 1)),
		float64(global.p999)/float64(maxInt64(porAgente.p999, 1)))
}
