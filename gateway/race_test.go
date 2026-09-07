// Copyright (c) 2026 Jorge Abraham Mendieta.
// Computational Substrate Theory. Todos los derechos reservados.

// Primer test Go del repositorio. Su objetivo es cerrar un NO MEDIDO concreto:
// D-26, la carrera de datos de los filtros. `go vet ./...` sale limpio y NO
// puede ver esta clase de defecto, asi que el instrumento correcto es el
// detector de carreras del runtime.
//
// POR QUE SUBPROCESOS: si el reproductor corriera dentro de este mismo proceso,
// el detector marcaria el test como fallido y el binario terminaria en 66. El
// hallazgo real ("la carrera existe") quedaria disfrazado de "la suite esta
// roja". Corriendolo en un subproceso del MISMO binario instrumentado, la
// carrera se mide, su reporte crudo queda en la salida, y el veredicto es una
// afirmacion verificable en vez de un accidente.
//
// TRES CONTROLES, porque un test que no puede dar rojo no mide nada:
//   1. TestControlPositivo_DetectorArmado: siembra una carrera trivial. Si el
//      binario se compilo SIN -race, este test da ROJO. O sea que la suite no
//      puede pasar en verde con el detector apagado.
//   2. TestControlNegativo_ElFSMNoReportaCarrera: el AgentFSM si toma su mutex.
//      Distingue "el detector reporta todo" de "reporta lo que hay".
//   3. Dentro del test de HMAC, cambiar el nonce debe invalidar la firma.
//
// DEFECTO PROPIO CORREGIDO (encontrado corriendo, no releyendo): en la primera
// version la salida del subproceso -- el reporte del detector, o sea LA
// EVIDENCIA -- solo se imprimia en el camino de fallo. Con todo en verde
// quedaba un veredicto sin su medicion, que es el testigo unico que W-01
// prohibe. Ahora se loguea siempre.

package gateway

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
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

func TestMain(m *testing.M) {
	switch os.Getenv(envModo) {
	case "filtros":
		reproducirFiltrosConcurrentes()
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

// reproducirFiltrosConcurrentes copia EXACTAMENTE las dos llamadas que
// handleConn hace sobre el agente compartido, y las hace desde donde las hace:
// FUERA de agent.mu, que solo envuelve el chequeo de lastTimestampNs.
func reproducirFiltrosConcurrentes() {
	agente := nuevoAgenteCompartido()

	var contexto [ContextVectorSize]float64
	for i := range contexto {
		contexto[i] = 0.1 * float64(i+1)
	}

	var wg sync.WaitGroup
	for c := 0; c < 4; c++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 500; i++ {
				agente.Huber.Update(float64(i%17)+0.5, 0.01)
				agente.Coherence.Update(contexto)
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
	for c := 0; c < 4; c++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 500; i++ {
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
	for c := 0; c < 4; c++ {
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
// CONTROL POSITIVO DEL INSTRUMENTO
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
// D-26: EL HALLAZGO
// ---------------------------------------------------------------------------

// TestD26_LosFiltrosCorrenSinSincronizacion mide la carrera real.
//
// handleConn llama agent.Huber.Update y agent.Coherence.Update sin tomar
// agent.mu, y getOrCreateAgent devuelve el mismo *AgentState a toda conexion
// que presente el mismo certificado. Ni HuberFilter ni CoherenceFilter tienen
// una sola primitiva de sincronizacion: mutan f.s, f.mu, f.v, f.lastV,
// c.fast[], c.medium[] y c.last en cada llamada.
//
// ESTE ES UN TEST DE CARACTERIZACION: afirma el defecto tal como esta hoy.
// Cuando D-26 se arregle, este test DEBE dar rojo. Ese rojo es el recordatorio
// de borrarlo y mover D-26 a cerrado en el contexto vivo, no un test que se
// rompio.
func TestD26_LosFiltrosCorrenSinSincronizacion(t *testing.T) {
	salida := correrEnSubproceso(t, "filtros")

	if !strings.Contains(salida, marcaReporte) {
		t.Fatalf("D-26 no reproduce. Si los filtros se sincronizaron, borrar este " +
			"test y cerrar D-26; si no, el reproductor dejo de golpear el mismo " +
			"AgentState.")
	}

	if !strings.Contains(salida, "filters.go") {
		t.Errorf("hay carrera, pero el reporte no cita filters.go: el sujeto " +
			"medido puede no ser el que digo")
	}

	t.Logf("D-26 MEDIDO: %d reporte(s) del detector sobre los filtros del agente compartido",
		strings.Count(salida, marcaReporte))
}

// ---------------------------------------------------------------------------
// CONTROL NEGATIVO
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
// Tambien de caracterizacion: cuando el HMAC se extienda a context y
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
