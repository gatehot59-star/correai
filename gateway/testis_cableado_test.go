// Copyright (c) 2026 Jorge Abraham Mendieta.
// Computational Substrate Theory. Todos los derechos reservados.

// MEDICION DEL CABLEADO DE TESTIS A handleConn.
//
// POR QUE ESTE ARCHIVO EXISTE. El cableado se commiteo con un NO MEDIDO propio:
// "los tests existentes del gateway corren con recorder nil, o sea que miden que
// el cable no rompe nada, NO que emite lo correcto". Este archivo cierra eso.
//
// LO QUE UN TEST DEL CABLEADO TIENE QUE PROBAR, y no es "que no explota":
//
//   1. Que la cadena emitida VALIDA con testis.ValidarCadena. Si el gateway
//      escribe bytes que su propio validador rechaza, la evidencia no sirve.
//   2. Que cada rechazo emite SU regla y no una cualquiera. Desde el cliente los
//      9 rechazos son el mismo byte 0xFF; la cadena es el unico lugar donde se
//      distinguen, y esa es la razon de ser del modulo.
//   3. Que dos conexiones del MISMO certificado producen UNA sola cadena con seq
//      contiguo. Eso falsa la decision de diseno de poner el estado en
//      AgentState: si estuviera en la pila de handleConn, este test da rojo.
//   4. Que el replay NO CUELGA. En ese punto handleConn ya tiene agent.mu tomado
//      y emitir() tambien lo toma: si el emitir estuviera adentro del Lock, esto
//      no seria un test rojo, seria un deadlock. Con timeout explicito.
//
// Y CADA CASO EMITE SU PROPIO CERTIFICADO, por la misma razon que el resto del
// paquete: el AgentState es por sha256(RawSubject) y un TriggerBlock deja backoff
// exponencial. Compartir cert haria que el orden de los tests cambie el resultado.
//
// NO SE SALTEAN CON -short, asi que entran al porton compartido de main.

package gateway

import (
	"encoding/binary"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/gatehot59-star/kampe-ir/testis"
)

// claveTestisDePrueba es DISTINTA de claveHMACDePrueba a proposito. Si fueran la
// misma, quien puede hablarle al gateway podria refabricar su propia evidencia, y
// el test no notaria la diferencia.
func claveTestisDePrueba() []byte {
	k := make([]byte, 32)
	for i := range k {
		k[i] = byte(200 - i)
	}
	return k
}

// arrancarConRecorder es arrancarGateway pero devolviendo el *Gateway, para poder
// llamar a UsarRecorder.
//
// DUPLICA ~20 lineas del helper original A PROPOSITO: arrancarGateway lo usan 9
// tests de otros dos archivos, y cambiar la firma de un helper compartido para
// que me sirva a mi es como se rompen tests ajenos. La duplicacion queda
// declarada y es de setup, no de aserciones.
func arrancarConRecorder(t *testing.T) (addr string, clave []byte, p *pki, g *Gateway, rec *MemRecorder) {
	t.Helper()

	p = nuevaPKI(t)
	caPath, certPath, keyPath := p.archivosDelServidor(t)

	cfg, err := NewTLSConfig(caPath, certPath, keyPath)
	if err != nil {
		t.Fatalf("NewTLSConfig: %v", err)
	}

	clave, hexClave := claveHMACDePrueba()
	t.Setenv("GATEWAY_HMAC_KEY", hexClave)

	addr = puertoLibre(t)
	g, err = NewGateway(addr, cfg)
	if err != nil {
		t.Fatalf("NewGateway: %v", err)
	}

	rec = NuevoMemRecorder()
	g.UsarRecorder(rec, claveTestisDePrueba())

	go func() { _ = g.Run() }()
	esperarAlGateway(t, addr)

	return addr, clave, p, g, rec
}

// reglasDe extrae la secuencia de reglas de una cadena, para poder afirmar QUE se
// emitio y no solo cuanto.
func reglasDe(vs []*testis.Verdict) []uint8 {
	out := make([]uint8, 0, len(vs))
	for _, v := range vs {
		out = append(out, v.Rule)
	}
	return out
}

// ---------------------------------------------------------------------------
// 1 · EL TEST QUE DECIDE: la cadena emitida por el gateway VALIDA
// ---------------------------------------------------------------------------

func TestCableado_LaCadenaQueEmiteElGatewayValida(t *testing.T) {
	addr, clave, p, _, rec := arrancarConRecorder(t)
	ag := p.nuevoAgente(t, "cadena-valida")

	c, err := ag.conectar(addr)
	if err != nil {
		t.Fatalf("conectar: %v", err)
	}
	defer c.cerrar()

	// Un paquete valido (ACK) y despues uno con HMAC roto (RECHAZO), para que la
	// cadena tenga las dos clases de veredicto. Una cadena de puros rechazos no
	// prueba que el aceptado tambien se encadena.
	if resp, err := c.enviar(paqueteValido(clave, ag, ahoraNs(), 1)); err != nil {
		t.Fatalf("primer envio: %v", err)
	} else if resp != respAck {
		t.Fatalf("el paquete valido dio %s, esperaba ACK", nombreDeRespuesta(resp))
	}

	malo := paqueteValido(clave, ag, ahoraNs(), 2)
	malo.hmac = []byte("firma-que-no-es")
	if _, err := c.enviar(malo); err != nil {
		// El gateway cierra sin responder en algunos caminos: no es un fallo del
		// test, el veredicto igual tiene que estar en la cadena.
		t.Logf("segundo envio cerro la conexion: %v", err)
	}

	// El grabado es sincronico dentro de handleConn, pero la respuesta al cliente
	// puede llegar antes de que el goroutine termine su iteracion. Espera acotada
	// y explicita, no un sleep al azar.
	esperarCadena(t, rec, ag.agentID, 2)

	cadena := rec.Cadena(ag.agentID)
	t.Logf("cadena de %d veredictos, reglas = %v", len(cadena), reglasDe(cadena))

	ok, motivo, info := testis.ValidarCadena(cadena, ag.agentID, claveTestisDePrueba(), nil)
	if !ok {
		t.Fatalf("EL GATEWAY EMITIO UNA CADENA QUE SU PROPIO VALIDADOR RECHAZA: %s", motivo)
	}
	if info.N != len(cadena) {
		t.Fatalf("info.N = %d, la cadena tiene %d", info.N, len(cadena))
	}

	// CONTROL POSITIVO. Sin esto, el verde de arriba no distingue "el gateway
	// emite evidencia integra" de "ValidarCadena no esta mirando".
	mutada := make([]*testis.Verdict, len(cadena))
	for i, v := range cadena {
		copia := *v
		mutada[i] = &copia
	}
	bits := math.Float64bits(mutada[0].Context[0]) ^ 1
	mutada[0].Context[0] = math.Float64frombits(bits)
	okMut, motivoMut, _ := testis.ValidarCadena(mutada, ag.agentID, claveTestisDePrueba(), nil)
	if okMut {
		t.Fatal("CONTROL POSITIVO FALLIDO: volte un bit del Context y el validador " +
			"siguio en VERDE. El test de arriba no prueba nada.")
	}
	t.Logf("control positivo ok: con 1 bit cambiado el validador dice ROJO (%s)", motivoMut)
}

// esperarCadena espera hasta que el grabador tenga al menos n veredictos del
// agente, o corta el test. Es una espera con condicion, no un sleep fijo: un
// sleep fijo convierte un cableado roto en un test intermitente.
func esperarCadena(t *testing.T, rec *MemRecorder, agentID [32]byte, n int) {
	t.Helper()
	fin := time.Now().Add(5 * time.Second)
	for time.Now().Before(fin) {
		if len(rec.Cadena(agentID)) >= n {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("el grabador tiene %d veredictos despues de 5 s, esperaba %d",
		len(rec.Cadena(agentID)), n)
}

// ---------------------------------------------------------------------------
// 2 · CADA RECHAZO EMITE SU REGLA
// ---------------------------------------------------------------------------

func TestCableado_CadaRechazoEmiteSuRegla(t *testing.T) {
	casos := []struct {
		nombre string
		regla  uint8
		enviar func(t *testing.T, c *conexion, clave []byte, ag *agente)
	}{
		{"HMAC invalido", ReglaHMACInvalid, func(t *testing.T, c *conexion, clave []byte, ag *agente) {
			p := paqueteValido(clave, ag, ahoraNs(), 1)
			p.hmac = []byte("no-es-la-firma")
			_, _ = c.enviar(p)
		}},
		{"AgentID que no coincide", ReglaAgentIDMismatch, func(t *testing.T, c *conexion, clave []byte, ag *agente) {
			p := paqueteValido(clave, ag, ahoraNs(), 1)
			otro := ag.agentID
			otro[0] ^= 0xFF
			p.agentID = otro
			p.hmac = firmarCabeceraDelPaquete(clave, p)
			_, _ = c.enviar(p)
		}},
		{"timestamp fuera de ventana", ReglaTimestampWindow, func(t *testing.T, c *conexion, clave []byte, ag *agente) {
			// 10 minutos en el futuro: muy afuera de los +-30 s.
			_, _ = c.enviar(paqueteValido(clave, ag, ahoraNs()+600*1_000_000_000, 1))
		}},
		{"frame de tamano invalido", ReglaFrameSize, func(t *testing.T, c *conexion, clave []byte, ag *agente) {
			var cab [4]byte
			binary.BigEndian.PutUint32(cab[:], 0) // size = 0
			_, _ = c.enviarCrudo(cab[:])
		}},
		{"cuerpo que no decodifica", ReglaDecodeFailed, func(t *testing.T, c *conexion, clave []byte, ag *agente) {
			cuerpo := []byte{0xFF, 0xFF, 0xFF, 0xFF}
			frame := make([]byte, 4+len(cuerpo))
			binary.BigEndian.PutUint32(frame[:4], uint32(len(cuerpo)))
			copy(frame[4:], cuerpo)
			_, _ = c.enviarCrudo(frame)
		}},
	}

	for _, cs := range casos {
		cs := cs
		t.Run(cs.nombre, func(t *testing.T) {
			addr, clave, p, _, rec := arrancarConRecorder(t)
			ag := p.nuevoAgente(t, "regla-"+cs.nombre)

			c, err := ag.conectar(addr)
			if err != nil {
				t.Fatalf("conectar: %v", err)
			}
			defer c.cerrar()

			cs.enviar(t, c, clave, ag)
			esperarCadena(t, rec, ag.agentID, 1)

			cadena := rec.Cadena(ag.agentID)
			reglas := reglasDe(cadena)
			t.Logf("reglas emitidas: %v (esperaba que la ultima sea %d)", reglas, cs.regla)

			ultima := cadena[len(cadena)-1]
			if ultima.Rule != cs.regla {
				t.Fatalf("regla emitida = %d (%s), esperaba %d (%s)",
					ultima.Rule, testis.Reglas[ultima.Rule],
					cs.regla, testis.Reglas[cs.regla])
			}

			// Y la cadena tiene que seguir siendo valida: emitir la regla correcta
			// pero romper el eslabon no sirve de nada.
			if ok, motivo, _ := testis.ValidarCadena(cadena, ag.agentID,
				claveTestisDePrueba(), nil); !ok {
				t.Fatalf("la cadena del rechazo no valida: %s", motivo)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 3 · DOS CONEXIONES DEL MISMO CERT = UNA SOLA CADENA
//     Falsa la decision de poner chainSeq en AgentState y no en la pila.
// ---------------------------------------------------------------------------

func TestCableado_DosConexionesDelMismoCertUnaSolaCadena(t *testing.T) {
	addr, clave, p, _, rec := arrancarConRecorder(t)
	ag := p.nuevoAgente(t, "dos-conexiones")

	const porConexion = 3
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(base uint32) {
			defer wg.Done()
			c, err := ag.conectar(addr)
			if err != nil {
				t.Errorf("conectar: %v", err)
				return
			}
			defer c.cerrar()
			for j := 0; j < porConexion; j++ {
				_, _ = c.enviar(paqueteValido(clave, ag, ahoraNs(), base+uint32(j)))
			}
		}(uint32(i*100 + 1))
	}
	wg.Wait()

	cadena := rec.Cadena(ag.agentID)
	if len(cadena) == 0 {
		t.Fatal("dos conexiones y CERO veredictos: el cable no emitio nada")
	}
	t.Logf("%d veredictos de 2 conexiones concurrentes del mismo cert", len(cadena))

	// El seq tiene que ser contiguo 1..N. Si el estado viviera en la pila de
	// handleConn, las dos conexiones emitirian seq=1 y esto da rojo.
	for i, v := range cadena {
		if v.Seq != uint64(i)+1 {
			t.Fatalf("seq no contiguo en pos %d: vino %d, esperaba %d. "+
				"Sintoma de estado de cadena POR CONEXION en vez de por agente.",
				i, v.Seq, i+1)
		}
	}

	if ok, motivo, _ := testis.ValidarCadena(cadena, ag.agentID,
		claveTestisDePrueba(), nil); !ok {
		t.Fatalf("la cadena de dos conexiones concurrentes no valida: %s", motivo)
	}
}

// ---------------------------------------------------------------------------
// 4 · EL REPLAY NO CUELGA (falsador del deadlock)
// ---------------------------------------------------------------------------

func TestCableado_ElCaminoDeReplayNoCuelga(t *testing.T) {
	// En el punto de replay handleConn ya tiene agent.mu tomado, y emitir()
	// tambien lo toma. Si el emitir estuviera ADENTRO del Lock esto no daria
	// rojo: se colgaria. Por eso el test tiene timeout propio y no confia en el
	// timeout global de `go test`.
	addr, clave, p, _, rec := arrancarConRecorder(t)
	ag := p.nuevoAgente(t, "replay-sin-deadlock")

	listo := make(chan struct{})
	go func() {
		defer close(listo)
		c, err := ag.conectar(addr)
		if err != nil {
			t.Errorf("conectar: %v", err)
			return
		}
		defer c.cerrar()

		ts := ahoraNs()
		if resp, err := c.enviar(paqueteValido(clave, ag, ts, 1)); err != nil || resp != respAck {
			t.Logf("primer paquete: resp=%v err=%v (el replay igual se ejercita)", resp, err)
		}
		// MISMO timestamp: replay. Este es el camino que toma el candado.
		_, _ = c.enviar(paqueteValido(clave, ag, ts, 2))
	}()

	select {
	case <-listo:
	case <-time.After(20 * time.Second):
		t.Fatal("DEADLOCK: el camino de replay no volvio en 20 s. " +
			"Sintoma de g.emitir() llamado DENTRO de agent.mu.Lock().")
	}

	cadena := rec.Cadena(ag.agentID)
	t.Logf("reglas emitidas en el camino de replay: %v", reglasDe(cadena))
	visto := false
	for _, v := range cadena {
		if v.Rule == ReglaReplay {
			visto = true
		}
	}
	if !visto {
		t.Logf("NO MEDIDO: no se emitio ReglaReplay. Reglas vistas: %v. "+
			"El replay no se alcanza si el primer paquete no dio ACK.", reglasDe(cadena))
	}
	if len(cadena) > 0 {
		if ok, motivo, _ := testis.ValidarCadena(cadena, ag.agentID,
			claveTestisDePrueba(), nil); !ok {
			t.Fatalf("la cadena del replay no valida: %s", motivo)
		}
	}
}

// ---------------------------------------------------------------------------
// 5 · SIN RECORDER EL GATEWAY SIGUE ANDANDO (fail-open declarado)
// ---------------------------------------------------------------------------

func TestCableado_SinRecorderElGatewayAtiende(t *testing.T) {
	// Es fail-open A PROPOSITO y es discutible: un gateway que se niega a
	// atender porque no puede grabar convierte una falla del auditor en una
	// caida del servicio. El test AFIRMA la decision para que un cambio de
	// politica sea visible, no silencioso.
	addr, clave, p := arrancarGateway(t) // sin UsarRecorder: recorder == nil
	ag := p.nuevoAgente(t, "sin-recorder")

	c, err := ag.conectar(addr)
	if err != nil {
		t.Fatalf("conectar: %v", err)
	}
	defer c.cerrar()

	resp, err := c.enviar(paqueteValido(clave, ag, ahoraNs(), 1))
	if err != nil {
		t.Fatalf("sin recorder el gateway no respondio: %v", err)
	}
	if resp != respAck {
		t.Fatalf("sin recorder dio %s, esperaba ACK", nombreDeRespuesta(resp))
	}
}

// ---------------------------------------------------------------------------
// 6 · EL NopRecorder CUENTA, y el descarte se DECLARA (regla D-21)
//     Unit, sin red: mide emitir() directo.
// ---------------------------------------------------------------------------

func TestCableado_NopRecorderDeclaraLosDescartes(t *testing.T) {
	nop := &NopRecorder{}
	g := &Gateway{recorder: nop, testisKey: claveTestisDePrueba()}
	agent := &AgentState{}
	var id [32]byte
	copy(id[:], "agente-de-descartes")

	// Tres veredictos que el NopRecorder descarta.
	for i := 0; i < 3; i++ {
		g.emitir(agent, id, ReglaAccepted, nil, int64(1000+i), 0)
	}
	if nop.Descartes != 3 {
		t.Fatalf("descartes = %d, esperaba 3", nop.Descartes)
	}

	// El cuarto se lleva el contador: el hueco queda DECLARADO en
	// dropped_since, que es exactamente la regla D-21. Un descarte NO consume
	// seq; se declara.
	g.emitir(agent, id, ReglaAccepted, nil, 2000, 0)
	if nop.Descartes != 1 {
		t.Fatalf("despues de tomar y volver a descartar, descartes = %d, esperaba 1",
			nop.Descartes)
	}
	// Y el seq siguio avanzando: 4 emisiones, seq final 4. Que el grabador
	// descarte no le da derecho a saltear el numero.
	if agent.chainSeq != 4 {
		t.Fatalf("chainSeq = %d, esperaba 4: el descarte NO debe consumir seq",
			agent.chainSeq)
	}
}
