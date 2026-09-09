// Copyright (c) 2026 Jorge Abraham Mendieta.
// Computational Substrate Theory. Todos los derechos reservados.

// Suite de concurrencia del gateway. Nacio para MEDIR D-26 (la carrera de
// datos de los filtros) porque `go vet ./...` sale limpio y no puede ver esa
// clase de defecto. Hoy mide que D-26 esta CERRADO.
//
// HISTORIA, para que nadie tenga que reconstruirla:
//   - Version anterior: TestD26_LosFiltrosCorrenSinSincronizacion afirmaba el
//     defecto y exigia un reporte del detector. Dio 27 reportes citando 16
//     lineas de filters.go. Su comentario decia que al arreglarse tenia que
//     dar rojo. Se cumplio.
//   - Version actual: ese test se invirtio. Ahora exige CERO reportes, y hay
//     dos controles nuevos, porque un verde de ausencia es el mas facil de
//     falsear: alcanza con que el reproductor deje de tocar el estado
//     compartido para que "no hay carrera" sea verdad por el motivo equivocado.
//
// LOS CUATRO CONTROLES:
//   1. TestControlPositivo_DetectorArmado: carrera sembrada. Si el binario se
//      compilo SIN -race, da ROJO. La suite no puede pasar con el detector
//      apagado.
//   2. TestControlMutacion_ElPatronDetectaFaltaDeMutex: el mismo patron de
//      concurrencia sobre gemelos SIN mutex de los dos filtros. Prueba que el
//      patron sigue siendo capaz de detectar una carrera.
//   3. TestControlNegativo_ElFSMNoReportaCarrera: el FSM si toma su mutex.
//      Distingue "el detector reporta todo" de "reporta lo que hay".
//   4. Dentro del test de HMAC, cambiar el nonce debe invalidar la firma.
//
// POR QUE ALGUNOS REPRODUCTORES CORREN EN SUBPROCESO: un reporte del detector
// hace terminar el binario en 66. Para los casos en que la carrera SE ESPERA,
// el subproceso del mismo binario instrumentado deja medir la carrera y quedarse
// con su reporte crudo sin que el hallazgo se disfrace de "suite roja". Para el
// caso en que la carrera NO se espera (D-26 ya arreglado) el reproductor corre
// EN PROCESO a proposito: ahi si quiero que una carrera residual rompa el test.
//
// DEFECTO PROPIO DE REPORTE, corregido: la primera corrida post-fix imprimia
// "Coherence{last=1.000000}" con %.6f, que es exactamente el valor inicial que
// el guard prohibe. El guard estaba bien (last vale 0,9999999976 y el test no
// fallo), pero la linea que existe para que un tercero lo verifique mostraba lo
// contrario de lo que media. Ahora se imprime con %.17g y con el delta.

package gateway

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"math"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"
)

// envModo selecciona el reproductor que corre el subproceso. Cuando esta
// vacio, el binario se comporta como una suite de tests normal.
const envModo = "KAMPE_MODO_CARRERA"

// marcaReporte es el encabezado exacto que emite el detector del runtime.
// Se busca la cadena COMPLETA a proposito: buscar solo "DATA RACE" hace que
// cualquier comentario que mencione el tema cuente como si fuera un reporte.
// Ese error ya se cometio una vez en el guard del workflow.
const marcaReporte = "WARNING: DATA RACE"

// Parametros del reproductor. Los mismos para el codigo real y para los
// gemelos sin mutex: si fueran distintos, la comparacion no valdria.
const (
	goroutinesDelReproductor = 4
	iteracionesPorGoroutine  = 500
)

func TestMain(m *testing.M) {
	switch os.Getenv(envModo) {
	case "filtros":
		reproducirFiltrosConcurrentes(nuevoAgenteCompartido())
		os.Exit(0)
	case "filtros-sin-mutex":
		reproducirFiltrosSinMutex()
		os.Exit(0)
	case "fsm":
		reproducirFSMConcurrente()
		os.Exit(0)
	case "armado":
		reproducirCarreraSembrada()
		os.Exit(0)
	}

	os.Exit(m.Run())
}

// correrEnSubproceso reejecuta este mismo binario de test (por lo tanto con la
// misma instrumentacion) en el modo pedido, devuelve stdout+stderr juntos, y
// SIEMPRE los deja en el log del test. Esa ultima parte no es prolijidad: es
// lo que hace que el veredicto sea recomputable por otro.
// GORACE=halt_on_error=0 hace que el detector imprima todas las carreras en
// vez de abortar en la primera.
func correrEnSubproceso(t *testing.T, modo string) string {
	t.Helper()

	cmd := exec.Command(os.Args[0])
	cmd.Env = append(os.Environ(),
		envModo+"="+modo,
		"GORACE=halt_on_error=0",
	)

	crudo, err := cmd.CombinedOutput() // el exit 66 del detector no es un error del test
	salida := string(crudo)

	t.Logf("EVIDENCIA CRUDA modo=%q exit_err=%v bytes=%d reportes=%d\n"+
		"--- inicio salida del subproceso ---\n%s--- fin salida del subproceso ---",
		modo, err, len(salida), strings.Count(salida, marcaReporte), salida)

	return salida
}

// nuevoAgenteCompartido construye el AgentState igual que getOrCreateAgent.
// Es el punto central del hallazgo: dos conexiones que presentan el MISMO
// certificado de cliente reciben este MISMO puntero.
func nuevoAgenteCompartido() *AgentState {
	return &AgentState{
		FSM:       NewAgentFSM(100 * time.Millisecond),
		Huber:     NewHuberFilter(DefaultAlpha, DefaultSigma),
		Coherence: NewCoherenceFilter(DefaultFastAlpha, DefaultMediumAlpha, DefaultCoherenceThreshold),
	}
}

func contextoDePrueba() [ContextVectorSize]float64 {
	var contexto [ContextVectorSize]float64
	for i := range contexto {
		contexto[i] = 0.1 * float64(i+1)
	}
	return contexto
}

// reproducirFiltrosConcurrentes copia EXACTAMENTE las dos llamadas que
// handleConn hace sobre el agente compartido, y las hace desde donde las hace:
// FUERA de agent.mu, que solo envuelve el chequeo de lastTimestampNs.
func reproducirFiltrosConcurrentes(agente *AgentState) {
	contexto := contextoDePrueba()

	var wg sync.WaitGroup
	for c := 0; c < goroutinesDelReproductor; c++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < iteracionesPorGoroutine; i++ {
				agente.Huber.Update(float64(i%17)+0.5, 0.01)
				agente.Coherence.Update(contexto)
			}
		}()
	}
	wg.Wait()
}

// ---------------------------------------------------------------------------
// GEMELOS SIN MUTEX: el control por mutacion del CODIGO
// ---------------------------------------------------------------------------

// huberSinMutex reproduce la aritmetica y las mutaciones de HuberFilter tal
// como estaban ANTES del fix de D-26, sin candado. No es codigo de produccion:
// existe para probar que el reproductor sigue siendo capaz de detectar una
// carrera en este patron. Si este gemelo no reporta nada, el verde del fix es
// del instrumento y no del fix.
type huberSinMutex struct {
	alpha float64
	sigma float64
	s     float64
	mu    float64
	v     float64
	lastV float64
}

func (f *huberSinMutex) Update(x float64, dt float64) bool {
	if dt <= 1e-9 {
		dt = 1e-9
	}

	f.s = (1-f.alpha)*f.s + f.alpha*x

	delta := x - f.mu
	clamp := 3 * f.sigma
	if delta > clamp {
		delta = clamp
	} else if delta < -clamp {
		delta = -clamp
	}
	f.mu = f.mu + f.alpha*delta

	residual := x - f.mu
	f.lastV = f.v
	f.v = (1-f.alpha)*f.v + f.alpha*residual*residual

	derivative := (f.v - f.lastV) / dt
	threshold := 0.05 * math.Sqrt(f.v+DefaultEpsilon) * (1.0 + math.Log1p(f.s))

	return derivative > threshold || derivative < -threshold
}

// coherenciaSinMutex es el gemelo sin candado de CoherenceFilter.
type coherenciaSinMutex struct {
	fastAlpha   float64
	mediumAlpha float64
	fast        [ContextVectorSize]float64
	medium      [ContextVectorSize]float64
	threshold   float64
	last        float64
}

func (c *coherenciaSinMutex) Update(vec [ContextVectorSize]float64) bool {
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

	coh := (dot * dot) / (normF*normM + DefaultEpsilon)
	anomalous := coh < c.threshold || coh < c.last*0.5
	c.last = coh

	return anomalous
}

// reproducirFiltrosSinMutex usa EL MISMO patron y los mismos parametros que
// reproducirFiltrosConcurrentes, cambiando solo el tipo de los filtros.
func reproducirFiltrosSinMutex() {
	huber := &huberSinMutex{alpha: DefaultAlpha, sigma: DefaultSigma}
	coherencia := &coherenciaSinMutex{
		fastAlpha:   DefaultFastAlpha,
		mediumAlpha: DefaultMediumAlpha,
		threshold:   DefaultCoherenceThreshold,
		last:        1.0,
	}
	contexto := contextoDePrueba()

	var wg sync.WaitGroup
	for c := 0; c < goroutinesDelReproductor; c++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < iteracionesPorGoroutine; i++ {
				huber.Update(float64(i%17)+0.5, 0.01)
				coherencia.Update(contexto)
			}
		}()
	}
	wg.Wait()
}

// reproducirFSMConcurrente golpea el AgentFSM desde cuatro goroutines. El FSM
// SI toma su mutex en los cuatro metodos, asi que no debe reportar nada.
func reproducirFSMConcurrente() {
	fsm := NewAgentFSM(100 * time.Millisecond)

	var wg sync.WaitGroup
	for c := 0; c < goroutinesDelReproductor; c++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < iteracionesPorGoroutine; i++ {
				fsm.Allow(time.Now())
				fsm.RecordSuccess()
				if i%100 == 0 {
					fsm.TriggerBlock(time.Now())
				}
			}
		}()
	}
	wg.Wait()
}

// contadorSinProteccion es la carrera sembrada del control positivo. No tiene
// nada que ver con el codigo del gateway: solo prueba que el detector esta
// puesto y funcionando en esta compilacion.
var contadorSinProteccion int

func reproducirCarreraSembrada() {
	var wg sync.WaitGroup
	for c := 0; c < goroutinesDelReproductor; c++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 1000; i++ {
				contadorSinProteccion++
			}
		}()
	}
	wg.Wait()
}

// ---------------------------------------------------------------------------
// CONTROL 1: el detector esta armado
// ---------------------------------------------------------------------------

// TestControlPositivo_DetectorArmado da ROJO si el binario se compilo sin
// -race. Es la garantia de que ningun verde de este archivo puede venir de
// haber corrido `go test` a secas.
func TestControlPositivo_DetectorArmado(t *testing.T) {
	salida := correrEnSubproceso(t, "armado")

	if !strings.Contains(salida, marcaReporte) {
		t.Fatalf("el detector de carreras NO esta activo: la carrera sembrada " +
			"no fue reportada. Este archivo solo mide algo con `go test -race`.")
	}

	t.Logf("detector armado: %d reporte(s) sobre la carrera sembrada de contadorSinProteccion",
		strings.Count(salida, marcaReporte))
}

// ---------------------------------------------------------------------------
// CONTROL 2: el patron sigue siendo capaz de encontrar la carrera
// ---------------------------------------------------------------------------

// TestControlMutacion_ElPatronDetectaFaltaDeMutex corre el mismo patron de
// concurrencia sobre los gemelos SIN mutex. Es la mutacion del codigo: si el
// unico cambio entre reportar y no reportar es el candado, entonces el candado
// es lo que arreglo D-26.
func TestControlMutacion_ElPatronDetectaFaltaDeMutex(t *testing.T) {
	salida := correrEnSubproceso(t, "filtros-sin-mutex")

	if !strings.Contains(salida, marcaReporte) {
		t.Fatalf("el patron del reproductor ya no detecta una carrera ni sobre " +
			"filtros SIN mutex. El verde de D-26 no significa nada hasta que " +
			"esto vuelva a dar reporte.")
	}

	t.Logf("CONTROL MUTACION OK: %d reporte(s) sobre los gemelos sin mutex, "+
		"con el mismo patron (%d goroutines x %d iteraciones) que da CERO sobre "+
		"los filtros reales",
		strings.Count(salida, marcaReporte),
		goroutinesDelReproductor, iteracionesPorGoroutine)
}

// ---------------------------------------------------------------------------
// D-26: LA REGRESION
// ---------------------------------------------------------------------------

// TestD26_LosFiltrosSonSegurosEnConcurrencia corre el reproductor EN PROCESO,
// no en subproceso, y eso es deliberado: con -race, cualquier carrera residual
// hace fallar este test directamente en vez de quedar como un reporte que
// alguien tiene que acordarse de contar.
//
// Ademas verifica que el estado interno SE MOVIO. Sin esa parte, el verde
// podria venir de un reproductor que dejo de tocar el estado compartido, que es
// la forma mas facil de falsear un verde de ausencia.
func TestD26_LosFiltrosSonSegurosEnConcurrencia(t *testing.T) {
	agente := nuevoAgenteCompartido()

	reproducirFiltrosConcurrentes(agente)

	// El reproductor tiene que haber dejado marca en los tres campos EWMA de
	// Huber y en el ultimo coseno de Coherence.
	if agente.Huber.s == 0 || agente.Huber.mu == 0 || agente.Huber.v == 0 {
		t.Errorf("el reproductor no movio el estado de HuberFilter "+
			"(s=%v mu=%v v=%v): un verde asi no prueba nada",
			agente.Huber.s, agente.Huber.mu, agente.Huber.v)
	}

	// c.last arranca en 1.0 exacto y converge a 1 - eps/(normF*normM), que con
	// este contexto queda en 0,9999999976: distinto de 1.0 pero indistinguible
	// con %.6f. Por eso se compara con == y se imprime el delta, no el valor
	// redondeado: si el log mostrara 1.000000 a secas, nadie podria verificar
	// este guard desde la evidencia.
	delta := 1.0 - agente.Coherence.last
	if agente.Coherence.last == 1.0 {
		t.Errorf("el reproductor no movio c.last de CoherenceFilter (sigue en " +
			"su valor inicial 1.0 exacto): un verde asi no prueba nada")
	}

	esperadas := goroutinesDelReproductor * iteracionesPorGoroutine
	t.Logf("D-26 CERRADO: %d llamadas concurrentes a cada filtro sobre el mismo "+
		"*AgentState, sin reporte del detector.\n"+
		"  Huber.s     = %.17g\n"+
		"  Huber.mu    = %.17g\n"+
		"  Huber.v     = %.17g\n"+
		"  Coherence.last = %.17g   (1 - last = %.3g, distinto de 0 => se movio)",
		esperadas,
		agente.Huber.s, agente.Huber.mu, agente.Huber.v,
		agente.Coherence.last, delta)
}

// TestD26_ElSubprocesoTampocoReporta repite la medicion con EL MISMO
// instrumento del turno anterior (subproceso + conteo de reportes) para que el
// antes y el despues sean comparables sin interpretacion: ese modo daba 27
// reportes y ahora debe dar 0.
func TestD26_ElSubprocesoTampocoReporta(t *testing.T) {
	salida := correrEnSubproceso(t, "filtros")

	if n := strings.Count(salida, marcaReporte); n != 0 {
		t.Fatalf("D-26 sigue vivo: %d reporte(s) del detector en el mismo modo "+
			"que antes del fix", n)
	}

	t.Logf("mismo instrumento que el turno anterior: 0 reportes (antes: 27)")
}

// ---------------------------------------------------------------------------
// CONTROL 3: el detector no reporta lo que si esta protegido
// ---------------------------------------------------------------------------

// TestControlNegativo_ElFSMNoReportaCarrera distingue "el detector reporta
// todo lo concurrente" de "reporta lo que de verdad esta sin proteger".
func TestControlNegativo_ElFSMNoReportaCarrera(t *testing.T) {
	salida := correrEnSubproceso(t, "fsm")

	if strings.Contains(salida, marcaReporte) {
		t.Fatalf("el AgentFSM toma su mutex en los cuatro metodos y aun asi hay " +
			"carrera: el hallazgo es nuevo y mas grave")
	}

	t.Logf("control negativo OK: el FSM sincronizado no produce reporte")
}

// ---------------------------------------------------------------------------
// D-47 sobre la funcion REAL, no sobre una primitiva parecida
// ---------------------------------------------------------------------------

// firmarCabecera reproduce el mensaje canonico de verifyPacketHMAC:
// AgentID(32) || Timestamp(8, BE) || Epoch(4, BE) || Nonce(4, BE).
func firmarCabecera(clave []byte, pkt *PerimeterPacket) []byte {
	msg := make([]byte, 32+8+4+4)
	copy(msg[0:32], pkt.AgentID[:])
	binary.BigEndian.PutUint64(msg[32:40], pkt.Timestamp)
	binary.BigEndian.PutUint32(msg[40:44], pkt.Epoch)
	binary.BigEndian.PutUint32(msg[44:48], pkt.Nonce)

	mac := hmac.New(sha256.New, clave)
	mac.Write(msg)
	return mac.Sum(nil)
}

// TestD47_ElHMACNoCubreContextNiCiphertext llama a la funcion del repo, no a
// hmac.Equal por separado: la conclusion es sobre el llamador.
//
// Sigue siendo de caracterizacion: cuando el HMAC se extienda a context y
// ciphertext, los dos ultimos bloques dan rojo, y eso es la senal de cerrar
// D-47.
func TestD47_ElHMACNoCubreContextNiCiphertext(t *testing.T) {
	clave := make([]byte, 32)
	for i := range clave {
		clave[i] = byte(i + 1)
	}
	t.Setenv("GATEWAY_HMAC_KEY", hex.EncodeToString(clave))

	g, err := NewGateway("127.0.0.1:0", nil)
	if err != nil {
		t.Fatalf("NewGateway: %v", err)
	}

	pkt := &PerimeterPacket{
		Timestamp: 1757260000000000000,
		Epoch:     7,
		Nonce:     99,
	}
	copy(pkt.AgentID[:], []byte("agente-de-prueba"))
	pkt.AgentIDLen = 16
	for i := range pkt.Context {
		pkt.Context[i] = float64(i) + 0.5
	}
	pkt.ContextLen = ContextVectorSize
	pkt.Ciphertext = []byte("carga-original")
	pkt.HMAC = firmarCabecera(clave, pkt)

	if !g.verifyPacketHMAC(pkt) {
		t.Fatal("un paquete bien firmado tiene que verificar: el arnes esta mal")
	}

	// CONTROL POSITIVO de la funcion: tiene que poder decir NO.
	pkt.Nonce++
	if g.verifyPacketHMAC(pkt) {
		t.Fatal("cambiar el nonce debe invalidar la firma; si no, la funcion " +
			"no discrimina y todo lo de abajo no mide nada")
	}
	pkt.Nonce--

	// D-47, primera mitad: el vector de contexto no esta firmado.
	contextoOriginal := pkt.Context[0]
	pkt.Context[0] = 999999.0
	if !g.verifyPacketHMAC(pkt) {
		t.Errorf("D-47 parece cerrado: el context ahora esta firmado. Actualizar " +
			"el contexto vivo y borrar este bloque")
	} else {
		t.Logf("D-47 MEDIDO: Context[0] paso de %v a 999999 y la firma sigue valida",
			contextoOriginal)
	}
	pkt.Context[0] = contextoOriginal

	// D-47, segunda mitad: el payload no esta firmado.
	pkt.Ciphertext = []byte("carga-reemplazada-por-un-atacante")
	if !g.verifyPacketHMAC(pkt) {
		t.Errorf("D-47 parece cerrado: el ciphertext ahora esta firmado. " +
			"Actualizar el contexto vivo y borrar este bloque")
	} else {
		t.Logf("D-47 MEDIDO: el ciphertext se reemplazo entero y la firma sigue valida")
	}
}

// ---------------------------------------------------------------------------
// Tests de comportamiento, sin caracterizacion de defectos
// ---------------------------------------------------------------------------

func TestValidateTimestamp(t *testing.T) {
	const ahora = uint64(1757260000000000000)
	drift := uint64(maxTimestampDriftNs)

	casos := []struct {
		nombre  string
		paquete uint64
		valido  bool
	}{
		{"identico", ahora, true},
		{"adelantado dentro de la ventana", ahora + drift - 1, true},
		{"adelantado justo en el borde", ahora + drift, true},
		{"adelantado un ns de mas", ahora + drift + 1, false},
		{"atrasado justo en el borde", ahora - drift, true},
		{"atrasado un ns de mas", ahora - drift - 1, false},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			if got := validateTimestamp(ahora, c.paquete, maxTimestampDriftNs); got != c.valido {
				t.Errorf("validateTimestamp(%d, %d) = %v, esperado %v",
					ahora, c.paquete, got, c.valido)
			}
		})
	}
}

func TestAgentFSM_BloqueaYVuelve(t *testing.T) {
	fsm := NewAgentFSM(50 * time.Millisecond)
	base := time.Now()

	if !fsm.Allow(base) {
		t.Fatal("un FSM nuevo tiene que permitir")
	}

	fsm.TriggerBlock(base)
	if fsm.Allow(base.Add(10 * time.Millisecond)) {
		t.Error("dentro del backoff no tiene que permitir")
	}

	// El primer TriggerBlock duplica: 50ms -> 100ms.
	if !fsm.Allow(base.Add(101 * time.Millisecond)) {
		t.Error("pasado el backoff tiene que volver a permitir")
	}
}

func TestAgentFSM_RecordSuccessResetaElBackoff(t *testing.T) {
	fsm := NewAgentFSM(50 * time.Millisecond)
	base := time.Now()

	// Tres bloqueos: 50 -> 100 -> 200 -> 400ms.
	for i := 0; i < 3; i++ {
		fsm.TriggerBlock(base)
		fsm.Allow(base.Add(10 * time.Second)) // sale del bloqueo
	}

	// decayAfterSuccesses exitos consecutivos vuelven el backoff al inicial.
	for i := 0; i < decayAfterSuccesses; i++ {
		fsm.RecordSuccess()
	}

	fsm.TriggerBlock(base)
	if !fsm.Allow(base.Add(101 * time.Millisecond)) {
		t.Error("tras el decay el backoff deberia ser 100ms (50 inicial x2), " +
			"no los 800ms acumulados")
	}
}

// TestFiltrosNoSeCopianPorValor documenta por que este paquete no puede volver
// a pasar filtros por valor: ahora contienen un sync.Mutex, y copiar un candado
// duplica el estado de sincronizacion en silencio. `go vet` detecta esa clase
// de copia (copylocks), asi que el guard real es el vet del CI; este test solo
// deja la razon escrita donde se lee.
func TestFiltrosNoSeCopianPorValor(t *testing.T) {
	huber := NewHuberFilter(DefaultAlpha, DefaultSigma)
	coherencia := NewCoherenceFilter(DefaultFastAlpha, DefaultMediumAlpha, DefaultCoherenceThreshold)

	if huber == nil || coherencia == nil {
		t.Fatal("los constructores deben devolver punteros usables")
	}

	huber.Update(1.0, 0.01)
	coherencia.Update(contextoDePrueba())

	t.Logf("los filtros se usan siempre por puntero; go vet (copylocks) es el " +
		"guard que impide la copia por valor")
}
