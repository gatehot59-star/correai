// Copyright (c) 2026 Jorge Abraham Mendieta.
// Computational Substrate Theory. Todos los derechos reservados.

// Instrumento de costo del candado que cerro D-26.
//
// El fix de D-26 dejo un NO MEDIDO explicito: "el costo del candado: no se
// midio contencion ni latencia; con un mutex por agente y dos secciones
// criticas cortas la hipotesis es que no importa, pero es hipotesis sin
// numero". Este archivo produce el numero.
//
// CUATRO BRAZOS, misma aritmetica, cambia SOLO la estrategia de candado:
//
//	A  1 agente compartido, filtros REALES        el caso patologico: varias
//	                                              conexiones del mismo cert
//	B  1 agente compartido, gemelos SIN candado   referencia de costo
//	C  1 agente por goroutine, candado por agente EL CASO REAL
//	D  1 agente por goroutine, UN candado global  CONTROL POSITIVO
//
//
// El brazo B es una CARRERA DE DATOS deliberada. Sus numeros son un piso de
// costo aritmetico, no un resultado correcto: los valores que calcula estan
// corrompidos por construccion. Se incluye porque la unica forma de aislar el
// costo del candado es correr la misma aritmetica sin el.
//
// El brazo D existe porque un "no hay contencion" es una afirmacion de
// ausencia, y una ausencia medida con un arnes ciego es indistinguible de una
// ausencia real. Si D no resulta mas lento que C, este arnes no puede ver
// contencion y ninguna de las otras tres mediciones significa nada.
//
// POR QUE ESTOS BENCHMARKS CORREN SIN -race: el detector instrumenta cada
// acceso a memoria y multiplica el costo por un factor grande y variable. Una
// medicion de tiempo bajo -race no mide el candado, mide el detector. El job
// de CI lo verifica corriendo las dos y mostrando la diferencia, en vez de
// pedir que se me crea.

package gateway

import (
	"encoding/hex"
	"fmt"
	"runtime"
	"sync"
	"testing"
	"time"
)

// candadoGlobal es el candado unico del brazo D. No es codigo de produccion:
// modela la decision de diseno equivocada (un candado para toda la flota) para
// que el arnes tenga que demostrar que sabe distinguirla de la buena.
var candadoGlobal sync.Mutex

// huberCandadoGlobal envuelve la MISMA aritmetica del gemelo sin candado con un
// mutex compartido por todas las instancias.
type huberCandadoGlobal struct {
	h huberSinMutex
}

func (f *huberCandadoGlobal) Update(x float64, dt float64) bool {
	candadoGlobal.Lock()
	defer candadoGlobal.Unlock()
	return f.h.Update(x, dt)
}

type coherenciaCandadoGlobal struct {
	c coherenciaSinMutex
}

func (f *coherenciaCandadoGlobal) Update(vec [ContextVectorSize]float64) bool {
	candadoGlobal.Lock()
	defer candadoGlobal.Unlock()
	return f.c.Update(vec)
}

func nuevoHuberGlobal() *huberCandadoGlobal {
	return &huberCandadoGlobal{h: huberSinMutex{alpha: DefaultAlpha, sigma: DefaultSigma}}
}

func nuevoCoherenciaGlobal() *coherenciaCandadoGlobal {
	return &coherenciaCandadoGlobal{c: coherenciaSinMutex{
		fastAlpha:   DefaultFastAlpha,
		mediumAlpha: DefaultMediumAlpha,
		threshold:   DefaultCoherenceThreshold,
		last:        1.0,
	}}
}

func nuevoHuberSinCandado() *huberSinMutex {
	return &huberSinMutex{alpha: DefaultAlpha, sigma: DefaultSigma}
}

func nuevoCoherenciaSinCandado() *coherenciaSinMutex {
	return &coherenciaSinMutex{
		fastAlpha:   DefaultFastAlpha,
		mediumAlpha: DefaultMediumAlpha,
		threshold:   DefaultCoherenceThreshold,
		last:        1.0,
	}
}

// paso es una iteracion del par de llamadas que handleConn hace por paquete.
// Los cuatro brazos ejecutan exactamente este par; lo unico que cambia es el
// objeto sobre el que lo hacen.
type paso func(g, i int)

// medirConcurrente reparte b.N operaciones entre `goroutines` y reporta el
// tiempo por operacion REAL y el throughput agregado.
//
// El ns/op que imprime testing divide por b.N; el que reporta esta funcion
// divide por las operaciones que de verdad se ejecutaron, que difieren de b.N
// por el redondeo del reparto. Se reportan los dos para que nadie tenga que
// confiar en uno solo.
func medirConcurrente(b *testing.B, goroutines int, ejecutar paso) {
	b.Helper()

	if goroutines < 1 {
		goroutines = 1
	}

	porGoroutine := b.N / goroutines
	if porGoroutine < 1 {
		porGoroutine = 1
	}
	total := porGoroutine * goroutines

	var wg sync.WaitGroup
	wg.Add(goroutines)

	b.ResetTimer()
	inicio := time.Now()

	for g := 0; g < goroutines; g++ {
		go func(g int) {
			defer wg.Done()
			for i := 0; i < porGoroutine; i++ {
				ejecutar(g, i)
			}
		}(g)
	}

	wg.Wait()
	transcurrido := time.Since(inicio)
	b.StopTimer()

	if transcurrido <= 0 {
		b.Fatalf("tiempo transcurrido no positivo (%v): la medicion no sirve", transcurrido)
	}

	b.ReportMetric(float64(transcurrido.Nanoseconds())/float64(total), "ns/op-real")
	b.ReportMetric(float64(total)/transcurrido.Seconds()/1e6, "Mop/s")
	b.ReportMetric(float64(goroutines), "goroutines")
}

// ---------------------------------------------------------------------------
// LOS CUATRO BRAZOS
// ---------------------------------------------------------------------------

func BenchmarkCandado(b *testing.B) {
	b.Logf("GOMAXPROCS=%d NumCPU=%d", runtime.GOMAXPROCS(0), runtime.NumCPU())
	contexto := contextoDePrueba()

	for _, g := range []int{1, 2, 4, 8, 16} {
		goroutines := g

		// A: un solo AgentState con los filtros REALES. Es lo que pasa cuando
		// un agente abre varias conexiones con el mismo certificado.
		b.Run(fmt.Sprintf("A_1agente_candadoPorAgente/g=%d", goroutines), func(b *testing.B) {
			agente := nuevoAgenteCompartido()
			medirConcurrente(b, goroutines, func(_, i int) {
				agente.Huber.Update(float64(i%17)+0.5, 0.01)
				agente.Coherence.Update(contexto)
			})
		})

		// B: la misma aritmetica SIN candado. RACY a proposito.
		b.Run(fmt.Sprintf("B_1agente_sinCandado_RACY/g=%d", goroutines), func(b *testing.B) {
			huber := nuevoHuberSinCandado()
			coherencia := nuevoCoherenciaSinCandado()
			medirConcurrente(b, goroutines, func(_, i int) {
				huber.Update(float64(i%17)+0.5, 0.01)
				coherencia.Update(contexto)
			})
		})

		// C: EL CASO REAL. Un agente por goroutine, cada uno con su candado.
		b.Run(fmt.Sprintf("C_NagentesAislados_candadoPorAgente/g=%d", goroutines), func(b *testing.B) {
			agentes := make([]*AgentState, goroutines)
			for k := range agentes {
				agentes[k] = nuevoAgenteCompartido()
			}
			medirConcurrente(b, goroutines, func(g, i int) {
				agentes[g].Huber.Update(float64(i%17)+0.5, 0.01)
				agentes[g].Coherence.Update(contexto)
			})
		})

		// D: CONTROL POSITIVO. N agentes aislados pero UN candado global.
		b.Run(fmt.Sprintf("D_NagentesAislados_candadoGLOBAL/g=%d", goroutines), func(b *testing.B) {
			hubers := make([]*huberCandadoGlobal, goroutines)
			cohs := make([]*coherenciaCandadoGlobal, goroutines)
			for k := range hubers {
				hubers[k] = nuevoHuberGlobal()
				cohs[k] = nuevoCoherenciaGlobal()
			}
			medirConcurrente(b, goroutines, func(g, i int) {
				hubers[g].Update(float64(i%17)+0.5, 0.01)
				cohs[g].Update(contexto)
			})
		})
	}
}

// ---------------------------------------------------------------------------
// EL DENOMINADOR: algo que YA esta en el camino de cada paquete
// ---------------------------------------------------------------------------

// BenchmarkVerifyPacketHMAC mide el HMAC-SHA256 que el gateway calcula para
// CADA paquete. Sin este numero, decir que el candado es "barato" o "caro" no
// significa nada: se estaria comparando contra cero en vez de contra el trabajo
// que el sistema ya hace.
func BenchmarkVerifyPacketHMAC(b *testing.B) {
	clave := make([]byte, 32)
	for i := range clave {
		clave[i] = byte(i + 1)
	}
	b.Setenv("GATEWAY_HMAC_KEY", hex.EncodeToString(clave))

	g, err := NewGateway("127.0.0.1:0", nil)
	if err != nil {
		b.Fatalf("NewGateway: %v", err)
	}

	pkt := &PerimeterPacket{Timestamp: 1757260000000000000, Epoch: 7, Nonce: 99}
	copy(pkt.AgentID[:], []byte("agente-de-prueba"))
	pkt.AgentIDLen = 16
	pkt.Ciphertext = []byte("carga-original")
	pkt.HMAC = firmarCabecera(clave, pkt)

	if !g.verifyPacketHMAC(pkt) {
		b.Fatal("el arnes esta mal: el paquete firmado no verifica")
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if !g.verifyPacketHMAC(pkt) {
			b.Fatal("verificacion fallida en medio del benchmark")
		}
	}
}

// ---------------------------------------------------------------------------
// EL CONTROL, COMO TEST: falla si el arnes no puede ver contencion
// ---------------------------------------------------------------------------

// resultadoBrazo guarda lo medido por un brazo del control.
type resultadoBrazo struct {
	nombre string
	nsPorOp float64
	mopsSeg float64
}

// medirBrazo corre `ops` operaciones repartidas entre `goroutines` y devuelve
// el costo por operacion. Es la misma mecanica que medirConcurrente pero con
// carga fija, para que los brazos sean comparables entre si sin depender de
// cuantas iteraciones le asigno el framework a cada uno.
func medirBrazo(nombre string, goroutines, ops int, ejecutar paso) resultadoBrazo {
	porGoroutine := ops / goroutines
	total := porGoroutine * goroutines

	// Calentamiento: saca del numero el costo de la primera pasada (cache frio,
	// crecimiento del heap, arranque de las goroutines).
	for i := 0; i < porGoroutine/10+1; i++ {
		ejecutar(0, i)
	}

	var wg sync.WaitGroup
	wg.Add(goroutines)
	inicio := time.Now()

	for g := 0; g < goroutines; g++ {
		go func(g int) {
			defer wg.Done()
			for i := 0; i < porGoroutine; i++ {
				ejecutar(g, i)
			}
		}(g)
	}

	wg.Wait()
	transcurrido := time.Since(inicio)

	return resultadoBrazo{
		nombre:  nombre,
		nsPorOp: float64(transcurrido.Nanoseconds()) / float64(total),
		mopsSeg: float64(total) / transcurrido.Seconds() / 1e6,
	}
}

// TestControlDeContencion_ElArnesVeLaDiferencia es el control positivo del
// instrumento, escrito como test para que FALLE cuando corresponde.
//
// Compara el caso real (un candado por agente) contra la decision de diseno
// equivocada (un candado global), con la misma aritmetica, la misma cantidad de
// operaciones y la misma cantidad de goroutines. El candado global TIENE que
// resultar mas lento. Si no lo es, este arnes no puede ver contencion, y
// entonces cualquier numero que produzca sobre el candado real es indistinguible
// de un cero medido con los ojos cerrados.
//
// Se salta con -short porque depende del tiempo de pared y no tiene sentido en
// una corrida rapida.
func TestControlDeContencion_ElArnesVeLaDiferencia(t *testing.T) {
	if testing.Short() {
		t.Skip("depende del tiempo de pared; se corre en el job de bench")
	}

	const (
		goroutines = 8
		ops        = 400000
		factorMin  = 1.2
	)

	contexto := contextoDePrueba()

	agentes := make([]*AgentState, goroutines)
	for k := range agentes {
		agentes[k] = nuevoAgenteCompartido()
	}
	porAgente := medirBrazo("candado por agente", goroutines, ops, func(g, i int) {
		agentes[g].Huber.Update(float64(i%17)+0.5, 0.01)
		agentes[g].Coherence.Update(contexto)
	})

	hubers := make([]*huberCandadoGlobal, goroutines)
	cohs := make([]*coherenciaCandadoGlobal, goroutines)
	for k := range hubers {
		hubers[k] = nuevoHuberGlobal()
		cohs[k] = nuevoCoherenciaGlobal()
	}
	global := medirBrazo("candado global", goroutines, ops, func(g, i int) {
		hubers[g].Update(float64(i%17)+0.5, 0.01)
		cohs[g].Update(contexto)
	})

	factor := global.nsPorOp / porAgente.nsPorOp

	t.Logf("CONTROL DE CONTENCION (goroutines=%d ops=%d GOMAXPROCS=%d)\n"+
		"  %-20s %10.2f ns/op  %8.3f Mop/s\n"+
		"  %-20s %10.2f ns/op  %8.3f Mop/s\n"+
		"  factor global/porAgente = %.2fx (minimo exigido %.2fx)",
		goroutines, ops, runtime.GOMAXPROCS(0),
		porAgente.nombre, porAgente.nsPorOp, porAgente.mopsSeg,
		global.nombre, global.nsPorOp, global.mopsSeg,
		factor, factorMin)

	if factor < factorMin {
		t.Fatalf("el arnes NO distingue un candado global de uno por agente "+
			"(factor %.2fx < %.2fx): no puede medir contencion, asi que ningun "+
			"numero de este archivo sobre el candado real es confiable",
			factor, factorMin)
	}
}

// TestCostoDelCandado_ContraElHMAC pone el costo del candado en la unica escala
// que decide algo: la del trabajo que el gateway ya hace por paquete.
//
// No afirma un umbral de aceptacion, porque cual es "aceptable" es una decision
// de producto y no mia. Reporta el porcentaje y deja el numero escrito.
func TestCostoDelCandado_ContraElHMAC(t *testing.T) {
	if testing.Short() {
		t.Skip("depende del tiempo de pared; se corre en el job de bench")
	}

	const ops = 200000
	contexto := contextoDePrueba()

	// Serial, una sola goroutine: aisla el costo de tomar y soltar un candado
	// que nadie disputa, que es el caso del 100% de los agentes con una sola
	// conexion abierta.
	agente := nuevoAgenteCompartido()
	conCandado := medirBrazo("con candado (serial)", 1, ops, func(_, i int) {
		agente.Huber.Update(float64(i%17)+0.5, 0.01)
		agente.Coherence.Update(contexto)
	})

	huber := nuevoHuberSinCandado()
	coherencia := nuevoCoherenciaSinCandado()
	sinCandado := medirBrazo("sin candado (serial)", 1, ops, func(_, i int) {
		huber.Update(float64(i%17)+0.5, 0.01)
		coherencia.Update(contexto)
	})

	sobrecosto := conCandado.nsPorOp - sinCandado.nsPorOp

	t.Logf("COSTO DEL CANDADO SIN DISPUTA (ops=%d, 1 goroutine)\n"+
		"  %-24s %8.2f ns/op\n"+
		"  %-24s %8.2f ns/op\n"+
		"  sobrecosto de 2 Lock/Unlock = %.2f ns por paquete",
		ops,
		conCandado.nombre, conCandado.nsPorOp,
		sinCandado.nombre, sinCandado.nsPorOp,
		sobrecosto)

	if sinCandado.nsPorOp <= 0 || conCandado.nsPorOp <= 0 {
		t.Fatalf("medicion invalida: con=%v sin=%v", conCandado.nsPorOp, sinCandado.nsPorOp)
	}
}
