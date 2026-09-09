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
// time.Now() y publicarlo como PISO DE RUIDO, y CADA distribucion se clasifica
// automaticamente contra ese piso.
//
// DEFECTO PROPIO CORREGIDO, y era el que importaba: la primera version tenia un
// guard de un lado solo. Comparaba contra el piso unicamente el caso patologico
// (que lo supera 400x) y no el caso real, que dio p99=80 ns contra un piso de
// 71 ns: 1,13x. Ese 80 no es una medicion, es el reloj, y el veredicto lo iba a
// publicar como "p99 de la espera en el caso real". Un guard que solo puede
// confirmar lo que ya creo no es un guard. Ahora la clasificacion es automatica
// y la seccion RESPUESTA no puede imprimir un numero sin ella.
//
// SEGUNDO DEFECTO PROPIO, PEOR, y es el que ponia ROJO el porton de `main`:
// este archivo tenia UNA CARRERA DE DATOS. `sumidero` era una sola variable de
// paquete escrita por las 8 goroutines sin sincronizar, en el archivo que
// certifica que D-26 (una carrera de datos) esta cerrado. Medido:
//
//	latencia_test.go:194  WARNING: DATA RACE
//	Write at 0x00000083ce80 by goroutine 32:  trabajoSintetico()
//	Previous write        by goroutine 35:  trabajoSintetico()
//	testing.go:1398: race detected during execution of test
//
// El detector marca el test como fallido DESPUES de que imprimio todo, asi que
// los tres guards se leian en VERDE y el test caia igual. Eso refuta las dos
// hipotesis que se habian escrito sobre la causa: GUARD 1 dio 47,2x con el
// minimo en 3,0x, GUARD 2 dio 8,3x, y el control de contencion pasa solo bajo
// -race. Ninguno de los dos guards era el problema.
//
// El arreglo es un sumidero POR GOROUTINE. No cambia lo que se mide: el sumidero
// existe solo para que el compilador no elimine el trabajo sintetico.
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
//     Ese es el numero que le importa al gateway, y ahi el reloj pesa menos
//     porque la operacion medida es mas grande.
//
// SOBRE EL MAX: es inutilizable y se declara asi. El max del PROPIO PISO DEL
// RELOJ dio 11.833.488 ns. Un maximo de esa escala es preempcion del scheduler
// y robo de CPU de la VM, no espera de candado. Publicar el max como cola del
// candado seria un error de atribucion, no un dato conservador.
//
// Y UNA TRAMPA MEDIDA, para el que lea los numeros de las dos condiciones: bajo
// -race el caso real sube a "MEDIDO 5,7x sobre el piso" cuando sin -race es
// "NO MEDIDO 1,02x". Eso NO es que el detector mejore la resolucion: infla la
// operacion mas que el reloj y produce un MEDIDO artificial. Los numeros que se
// citan son siempre los de SIN -race.
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

// margenSobreElPiso es cuantas veces tiene que superar al piso del reloj un
// cuantil para que se lo pueda llamar medicion.
const margenSobreElPiso = 3.0

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
	return fmt.Sprintf("%-46s n=%-9d media=%9.1f  p50=%6d  p90=%7d  p99=%8d  p99.9=%9d  max=%10d",
		d.etiqueta, d.n, d.media, d.p50, d.p90, d.p99, d.p999, d.max)
}

// veredictoContraElPiso clasifica el p99 de esta distribucion contra el piso del
// instrumento. Es lo que impide reportar el reloj como si fuera el candado.
func (d distribucion) veredictoContraElPiso(piso distribucion) string {
	if piso.p99 <= 0 {
		return "SIN PISO (el instrumento no tiene resolucion)"
	}
	factor := float64(d.p99) / float64(piso.p99)
	switch {
	case factor >= margenSobreElPiso:
		return fmt.Sprintf("MEDIDO (%.1fx sobre el piso)", factor)
	case d.p99 <= piso.p99:
		return fmt.Sprintf("NO MEDIDO: p99 por DEBAJO del piso (%.2fx). "+
			"Solo se puede afirmar que la espera es <= %d ns", factor, piso.p99)
	default:
		return fmt.Sprintf("NO MEDIDO: %.2fx sobre el piso, insuficiente (min %.1fx). "+
			"Es COTA SUPERIOR: la espera no supera ~%d ns, pero su valor real "+
			"esta tapado por el reloj", factor, margenSobreElPiso, d.p99)
	}
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
// suelo por debajo del cual este archivo NO PUEDE MEDIR NADA.
//
// Se mide con `goroutines` corriendo a la vez porque el costo del reloj tambien
// se degrada bajo carga, y un piso medido en serie seria un piso optimista.
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

// ranuraSumidero es una ranura de una linea de cache. El padding no es adorno:
// sin el, ocho ranuras contiguas comparten linea y el false sharing se cuela
// DENTRO de la seccion critica que este archivo esta cronometrando.
type ranuraSumidero struct {
	valor float64
	_     [56]byte
}

// sumideros evita que el compilador elimine el trabajo sintetico. Hay UNO POR
// GOROUTINE a proposito.
//
// La version anterior era `var sumidero float64`, una sola variable de paquete
// escrita por las 8 goroutines: una carrera de datos MIA, reportada por el
// detector en latencia_test.go:194, y la causa real del rojo del porton de
// `main`. No era ninguno de los guards.
var sumideros [64]ranuraSumidero

// trabajoSintetico aproxima la duracion de la seccion critica de
// CoherenceFilter.Update, que el perfil de mutex senalo como dos tercios del
// bloqueo. No pretende ser identica: pretende mantener el candado tomado un rato
// comparable, que es lo que produce la cola.
//
// `ranura` es el indice de la goroutine que llama. Cada una escribe SOLO la
// suya, asi que no hay carrera y tampoco hay sincronizacion que distorsione la
// medicion.
func trabajoSintetico(vec [ContextVectorSize]float64, ranura int) {
	var dot, normF float64
	for i := 0; i < ContextVectorSize; i++ {
		dot += vec[i] * vec[i]
		normF += vec[i]
	}
	sumideros[ranura&63].valor = dot / (normF + DefaultEpsilon)
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
				trabajoSintetico(vec, g)
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
func TestLatenciaP99DeLaEspera(t *testing.T) {
	if testing.Short() {
		t.Skip("depende del tiempo de pared; se corre en el job de latencia")
	}

	const (
		goroutines   = 8
		porGoroutine = 100000
	)

	t.Logf("GOMAXPROCS=%d NumCPU=%d muestras=%d (%d goroutines x %d)",
		runtime.GOMAXPROCS(0), runtime.NumCPU(),
		goroutines*porGoroutine, goroutines, porGoroutine)

	// --- Piso de ruido, primero, porque decide si el resto significa algo ---
	piso := pisoDelReloj(goroutines, porGoroutine)
	t.Logf("\n=== PISO DE RUIDO DEL INSTRUMENTO ===\n%s", piso)
	if piso.p99 <= 0 {
		t.Fatalf("el piso del reloj dio p99=%d: el instrumento no tiene resolucion "+
			"suficiente y ningun numero de este test vale", piso.p99)
	}
	t.Logf("MAX INUTILIZABLE: el max del propio piso es %d ns. Cualquier max de "+
		"este archivo es preempcion del scheduler o robo de CPU de la VM, no "+
		"espera de candado.", piso.max)

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

	// --- Tabla con la clasificacion pegada a cada numero ---
	todas := []distribucion{
		piso, esperaAislada, esperaCompartida,
		latSerial, latAislada, latCompartida,
	}
	t.Logf("\n=== DISTRIBUCIONES (ns) ===")
	for _, d := range todas {
		t.Logf("%s", d)
	}

	t.Logf("\n=== CLASIFICACION CONTRA EL PISO ===")
	for _, d := range todas[1:] {
		t.Logf("CLASIF  %-46s %s", d.etiqueta, d.veredictoContraElPiso(piso))
	}

	// --- GUARD 1: el piso no puede explicar el caso patologico ---
	factorPiso := float64(esperaCompartida.p99) / float64(piso.p99)
	t.Logf("\nGUARD 1  p99 patologico / p99 del piso = %.1fx (minimo exigido %.1fx)",
		factorPiso, margenSobreElPiso)
	if factorPiso < margenSobreElPiso {
		t.Fatalf("el p99 patologico (%d ns) no supera el piso del reloj (%d ns) por "+
			"%.1fx: lo que se esta midiendo puede ser el reloj",
			esperaCompartida.p99, piso.p99, margenSobreElPiso)
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

	// --- GUARD 3: el que faltaba. Impide vender el reloj como dato ---
	// No hace fallar el test: el caso real ESTA en el piso y eso es un hecho del
	// instrumento, no un error. Lo que hace es forzar la declaracion.
	clasifReal := esperaAislada.veredictoContraElPiso(piso)
	t.Logf("GUARD 3  el caso real contra el piso -> %s", clasifReal)

	// --- El numero que se pidio, cada uno con su clasificacion ---
	t.Logf("\n=== RESPUESTA ===\n"+
		"p99 ESPERA caso real (1 candado por agente):   %8d ns   [%s]\n"+
		"p99 ESPERA caso patologico (1 candado, 8 gor): %8d ns   [%s]\n"+
		"p99.9 patologico:                              %8d ns\n"+
		"p99 PAQUETE completo, caso real:               %8d ns   [%s]\n"+
		"p99 PAQUETE completo, caso patologico:         %8d ns   [%s]\n"+
		"piso del reloj (p99):                          %8d ns\n"+
		"max: NO SE REPORTA como cola del candado (el max del piso fue %d ns)",
		esperaAislada.p99, clasifReal,
		esperaCompartida.p99, esperaCompartida.veredictoContraElPiso(piso),
		esperaCompartida.p999,
		latAislada.p99, latAislada.veredictoContraElPiso(piso),
		latCompartida.p99, latCompartida.veredictoContraElPiso(piso),
		piso.p99, piso.max)
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
