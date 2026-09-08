// Copyright (c) 2026 Jorge Abraham Mendieta.
// Computational Substrate Theory. Todos los derechos reservados.

// MEDICIONES PUNTA A PUNTA DE handleConn, con el cliente mTLS real.
//
// Todo lo que se midio antes en este repo sobre concurrencia uso el `AgentState`
// directamente. Eso era un PROXY DECLARADO: los mismos metodos, sobre el mismo
// puntero, pero NO desde `handleConn`. Este archivo cierra esa distancia.
//
// ===========================================================================
// LEER E0 PRIMERO. E0 midio que el gateway RECHAZA EL PRIMER PAQUETE DE TODA
// CONEXION, siempre, con cualquier payload y sin importar lo que haga el cliente
// (11 de 11 combinaciones). Eso parte este archivo en dos:
//
//   - Los tests que solo necesitan UN paquete y esperan RECHAZO (E2, E4, E5, E8)
//     corren igual. Sobre el gateway real su rechazo es INDICATIVO, porque desde
//     el cliente no se puede saber la causa: los 9 caminos escriben el mismo
//     0xFF. Sobre el gateway sin HuberFilter son CONCLUYENTES, porque ahi la
//     unica causa posible es la declarada.
//
//   - Los tests que necesitan llegar a un estado POSTERIOR a un ACK (E3 replay,
//     E6 D-46, E7 D-47, E9 D-26 en el callsite) son INMEDIBLES sobre el gateway
//     real. Declaran NO MEDIDO con su razon via t.Skip. El job los corre otra vez
//     contra una copia con el bloque del HuberFilter desactivado, y ahi SI miden.
//     El sujeto es otro y queda declarado.
//
// EL CONTROL DEL ARNES vive en E1, y solo puede cumplirse sobre el gateway
// mutado: si con el Huber desactivado un paquete valido da ACK, entonces el
// codificador coincide con `decodePerimeterPacket` y el unico obstaculo era el
// filtro. Sin ese ACK en algun sujeto, ningun rechazo de este archivo seria
// atribuible.
// ===========================================================================
//
// ESTOS TESTS NO SE SALTEAN CON -short A PROPOSITO, asi que entran al porton
// compartido de `main` y lo fortalecen.
//
// Y CADA CASO EMITE SU PROPIO CERTIFICADO: el AgentState es por
// sha256(RawSubject), y un TriggerBlock deja al agente con backoff exponencial.
// Compartir cert entre casos haria que el orden de los tests cambie el resultado.

package gateway

import (
	"encoding/binary"
	"fmt"
	"sync"
	"testing"
	"time"
)

// nombreDeRespuesta traduce el byte para que los logs se lean.
func nombreDeRespuesta(b byte) string {
	switch b {
	case respAck:
		return "ACK (0x00)"
	case respRechazo:
		return "RECHAZO (0xFF)"
	default:
		return fmt.Sprintf("DESCONOCIDO (0x%02X)", b)
	}
}

// ---------------------------------------------------------------------------
// E1 · EL PRIMER PAQUETE, Y EL CONTROL DEL ARNES
// ---------------------------------------------------------------------------

// TestE2E_01_ElArnesYElPrimerPaquete reporta los dos casos posibles y no falla en
// ninguno, porque el resultado correcto depende del sujeto:
//
//   - gateway REAL: rechaza. Es la caracterizacion del hallazgo de E0.
//   - gateway SIN HuberFilter: acepta, y ESO valida el codificador. Es el unico
//     control capaz de probar que mi arnes esta bien escrito.
func TestE2E_01_ElArnesYElPrimerPaquete(t *testing.T) {
	addr, clave, p := arrancarGateway(t)
	ag := p.nuevoAgente(t, "agente-e1")

	c, err := ag.conectar(addr)
	if err != nil {
		t.Fatalf("el handshake mTLS fallo: %v", err)
	}
	defer c.cerrar()

	resp, err := c.enviar(paqueteValido(clave, ag, ahoraNs(), 1))
	if err != nil {
		t.Fatalf("el gateway cerro sin responder: %v", err)
	}

	t.Logf("E1 handshake mTLS OK, agentID derivado del cert = %x", ag.agentID[:8])

	if resp != respAck {
		t.Logf("E1 CARACTERIZACION: el gateway respondio %s a un paquete VALIDO. "+
			"Coincide con E0: rechaza el primer paquete de toda conexion. El handshake "+
			"mTLS, el frame y el decoder se ejercitaron de verdad; lo que no se puede "+
			"ejercitar es nada posterior al primer paquete.", nombreDeRespuesta(resp))
		return
	}

	// Solo se llega aca sobre un gateway sin el bloque del HuberFilter.
	resp2, err := c.enviar(paqueteValido(clave, ag, ahoraNs()+1, 2))
	if err != nil {
		t.Fatalf("segundo paquete, el gateway cerro: %v", err)
	}
	if resp2 != respAck {
		t.Fatalf("el primer paquete dio ACK y el segundo %s: handleConn no hace loop "+
			"o hay estado que se corrompe entre paquetes", nombreDeRespuesta(resp2))
	}

	t.Logf("E1 ARNES VALIDADO: dos paquetes validos en la misma conexion, los dos "+
		"ACK. El codificador coincide con decodePerimeterPacket campo por campo, "+
		"handleConn hace loop, y verifyPacketHMAC acepta mi firma. Todos los "+
		"rechazos de este archivo son atribuibles a su causa declarada.")
}

// ---------------------------------------------------------------------------
// E2, E4, E5 · rechazos de UN solo paquete
// ---------------------------------------------------------------------------

func TestE2E_02_HMACInvalido(t *testing.T) {
	addr, clave, p := arrancarGateway(t)
	ag := p.nuevoAgente(t, "agente-e2")

	c, err := ag.conectar(addr)
	if err != nil {
		t.Fatalf("handshake: %v", err)
	}
	defer c.cerrar()

	pkt := paqueteValido(clave, ag, ahoraNs(), 1)
	pkt.hmac[0] ^= 0xFF // un solo bit invalida la firma

	resp, err := c.enviar(pkt)
	if err != nil {
		t.Fatalf("el gateway cerro sin responder: %v", err)
	}
	if resp != respRechazo {
		t.Fatalf("un HMAC invalido fue ACEPTADO (%s): la verificacion no corre en "+
			"el camino real", nombreDeRespuesta(resp))
	}
	t.Logf("E2 MEDIDO: HMAC alterado en 1 byte -> %s", nombreDeRespuesta(resp))
}

func TestE2E_04_AgentIDQueNoCoincide(t *testing.T) {
	addr, clave, p := arrancarGateway(t)
	ag := p.nuevoAgente(t, "agente-e4")
	otro := p.nuevoAgente(t, "agente-e4-suplantado")

	c, err := ag.conectar(addr)
	if err != nil {
		t.Fatalf("handshake: %v", err)
	}
	defer c.cerrar()

	// Firmado correctamente, pero declarando el AgentID de OTRO agente.
	resp, err := c.enviar(paqueteValido(clave, otro, ahoraNs(), 1))
	if err != nil {
		t.Fatalf("el gateway cerro sin responder: %v", err)
	}
	if resp != respRechazo {
		t.Fatalf("un paquete con el AgentID de otro agente fue ACEPTADO (%s)",
			nombreDeRespuesta(resp))
	}
	t.Logf("E4 MEDIDO: AgentID del paquete (%x) != derivado del cert (%x) -> %s",
		otro.agentID[:8], ag.agentID[:8], nombreDeRespuesta(resp))
	t.Logf("E4 CONSECUENCIA: para forjar un paquete a nombre de un agente hay que "+
		"presentar SU certificado. Eso acota D-46 a auto-DoS o a credencial robada, "+
		"y ahora esta medido en el camino real, no deducido de leer el codigo.")
}

func TestE2E_05_TimestampFueraDeVentana(t *testing.T) {
	addr, clave, p := arrancarGateway(t)
	ag := p.nuevoAgente(t, "agente-e5")

	c, err := ag.conectar(addr)
	if err != nil {
		t.Fatalf("handshake: %v", err)
	}
	defer c.cerrar()

	// 31 s adelante: fuera de maxTimestampDriftNs (30 s).
	fuera := ahoraNs() + uint64(31*time.Second)

	resp, err := c.enviar(paqueteValido(clave, ag, fuera, 1))
	if err != nil {
		t.Fatalf("el gateway cerro sin responder: %v", err)
	}
	if resp != respRechazo {
		t.Fatalf("un timestamp 31 s adelantado fue ACEPTADO (%s)", nombreDeRespuesta(resp))
	}
	t.Logf("E5 MEDIDO: ts = now + 31 s (ventana = %v) -> %s",
		time.Duration(maxTimestampDriftNs), nombreDeRespuesta(resp))
}

// ---------------------------------------------------------------------------
// E3 · ANTI-REPLAY (necesita un ACK previo)
// ---------------------------------------------------------------------------

func TestE2E_03_AntiReplay(t *testing.T) {
	addr, clave, p := arrancarGateway(t)
	saltearSiElGatewayNoAcepta(t, addr, clave, p)

	ag := p.nuevoAgente(t, "agente-e3")
	c, err := ag.conectar(addr)
	if err != nil {
		t.Fatalf("handshake: %v", err)
	}
	defer c.cerrar()

	ts := ahoraNs()

	primera, err := c.enviar(paqueteValido(clave, ag, ts, 1))
	if err != nil {
		t.Fatalf("primer envio: %v", err)
	}
	if primera != respAck {
		t.Fatalf("el primer paquete tenia que pasar, dio %s", nombreDeRespuesta(primera))
	}

	// EXACTAMENTE el mismo timestamp: el chequeo es `<=`, asi que repetirlo basta.
	segunda, err := c.enviar(paqueteValido(clave, ag, ts, 2))
	if err != nil {
		t.Logf("E3 MEDIDO: el replay hizo que el gateway CIERRE la conexion (%v). "+
			"Tambien es un rechazo, y ademas cuesta un handshake TLS nuevo.", err)
		return
	}
	if segunda != respRechazo {
		t.Fatalf("un replay con el MISMO timestamp fue aceptado (%s)", nombreDeRespuesta(segunda))
	}
	t.Logf("E3 MEDIDO: primer paquete ACK, mismo timestamp otra vez -> %s",
		nombreDeRespuesta(segunda))
}

// ---------------------------------------------------------------------------
// E6 · D-46 SOBRE EL CAMINO REAL
// ---------------------------------------------------------------------------

// TestE2E_06_D46_EnvenenamientoDeLaMarcaDeAgua mide el hallazgo que hasta hoy solo
// estaba LEIDO en el codigo: `handleConn` escribe `lastTimestampNs` ANTES de
// verificar el HMAC, asi que un paquete con firma invalida y timestamp adelantado
// mueve la marca de agua y deja al agente rechazando sus propios paquetes.
//
// TRES PARTES, porque el hallazgo sin sus controles seria alarmismo:
//
//	(a) el envenenamiento: forjado con ts = now+29s y HMAC roto, y despues un
//	    paquete LEGITIMO del mismo agente tambien rechazado
//	(b) control de agente limpio: otro cert sigue dando ACK, o sea que el gateway
//	    no se cayo
//	(c) control del MECANISMO: un ts POSTERIOR al envenenado SI se acepta, lo que
//	    fija que el defecto es un salto de la marca de agua y nada mas
func TestE2E_06_D46_EnvenenamientoDeLaMarcaDeAgua(t *testing.T) {
	addr, clave, p := arrancarGateway(t)
	saltearSiElGatewayNoAcepta(t, addr, clave, p)

	victima := p.nuevoAgente(t, "agente-e6-victima")
	limpio := p.nuevoAgente(t, "agente-e6-limpio")

	const adelanto = 29 * time.Second
	tsEnvenenado := ahoraNs() + uint64(adelanto)

	// (a) el paquete forjado
	c1, err := victima.conectar(addr)
	if err != nil {
		t.Fatalf("handshake de la victima: %v", err)
	}
	forjado := paqueteValido(clave, victima, tsEnvenenado, 1)
	forjado.hmac[0] ^= 0xFF // firma invalida: el paquete NO deberia dejar rastro

	respForjado, err := c1.enviar(forjado)
	if err != nil {
		t.Fatalf("envio del forjado: %v", err)
	}
	c1.cerrar()

	if respForjado != respRechazo {
		t.Fatalf("el paquete forjado fue aceptado (%s)", nombreDeRespuesta(respForjado))
	}

	// El rechazo dispara TriggerBlock: hay que esperar el backoff (100 ms x2).
	time.Sleep(350 * time.Millisecond)

	c2, err := victima.conectar(addr)
	if err != nil {
		t.Fatalf("segundo handshake de la victima: %v", err)
	}
	respLegitimo, errLegitimo := c2.enviar(paqueteValido(clave, victima, ahoraNs(), 2))
	c2.cerrar()

	envenenado := errLegitimo != nil || respLegitimo == respRechazo

	// (b) control: un agente limpio no esta afectado
	c3, err := limpio.conectar(addr)
	if err != nil {
		t.Fatalf("handshake del agente limpio: %v", err)
	}
	respLimpio, err := c3.enviar(paqueteValido(clave, limpio, ahoraNs(), 1))
	c3.cerrar()
	if err != nil {
		t.Fatalf("el agente limpio no obtuvo respuesta: %v", err)
	}
	if respLimpio != respAck {
		t.Fatalf("CONTROL ROTO: un agente limpio fue rechazado (%s). El gateway se "+
			"cayo por otra razon y E6 no mide D-46", nombreDeRespuesta(respLimpio))
	}

	// (c) control del mecanismo: un ts POSTERIOR al envenenado se acepta.
	time.Sleep(700 * time.Millisecond)
	c4, err := victima.conectar(addr)
	if err != nil {
		t.Fatalf("tercer handshake de la victima: %v", err)
	}
	respPosterior, errPosterior := c4.enviar(paqueteValido(clave, victima, tsEnvenenado+1000, 3))
	c4.cerrar()

	t.Logf("E6 RESULTADOS\n"+
		"  forjado (HMAC roto, ts=now+%v)       -> %s\n"+
		"  legitimo despues (ts=now)            -> %s%s\n"+
		"  agente LIMPIO (control)              -> %s\n"+
		"  ts POSTERIOR al envenenado (control) -> %s%s",
		adelanto, nombreDeRespuesta(respForjado),
		nombreDeRespuesta(respLegitimo), errorCorto(errLegitimo),
		nombreDeRespuesta(respLimpio),
		nombreDeRespuesta(respPosterior), errorCorto(errPosterior))

	if !envenenado {
		t.Errorf("D-46 NO reproduce en el camino real: el paquete legitimo posterior "+
			"al forjado fue aceptado (%s). Si el orden replay/HMAC se arreglo, borrar "+
			"este test y cerrar D-46", nombreDeRespuesta(respLegitimo))
		return
	}

	t.Logf("E6 D-46 MEDIDO EN EL CAMINO REAL: un paquete con FIRMA INVALIDA movio "+
		"lastTimestampNs %v al futuro, y el agente quedo rechazando sus propios "+
		"paquetes legitimos hasta que el reloj alcance esa marca.", adelanto)

	if errPosterior == nil && respPosterior == respAck {
		t.Logf("E6 MECANISMO CONFIRMADO: el agente NO esta muerto. Un ts posterior a "+
			"la marca envenenada se acepta, asi que el defecto es exactamente un salto "+
			"de la marca de agua, ni mas ni menos. La ventana de dano es %v.", adelanto)
	} else {
		t.Logf("E6 NO MEDIDO: no pude confirmar el mecanismo por el otro lado. El ts "+
			"posterior dio %s%s, probablemente por el backoff del FSM todavia activo. "+
			"El envenenamiento SI quedo medido; su mecanismo exacto queda parcial.",
			nombreDeRespuesta(respPosterior), errorCorto(errPosterior))
	}
}

func errorCorto(err error) string {
	if err == nil {
		return ""
	}
	return "  [conexion cerrada: " + err.Error() + "]"
}

// ---------------------------------------------------------------------------
// E7 · D-47 SOBRE EL CAMINO REAL
// ---------------------------------------------------------------------------

// TestE2E_07_D47_ContextYCiphertextNoEstanFirmados altera el vector de contexto y
// el payload DESPUES de firmar la cabecera. Si el gateway acepta el paquete, es
// porque el HMAC cubre 48 bytes de cabecera y nada mas.
//
// Antes esto estaba medido llamando a verifyPacketHMAC. Aca esta medido con un
// paquete que entra por el socket: es la diferencia entre "la funcion no lo cubre"
// y "un atacante puede hacerlo".
func TestE2E_07_D47_ContextYCiphertextNoEstanFirmados(t *testing.T) {
	addr, clave, p := arrancarGateway(t)
	saltearSiElGatewayNoAcepta(t, addr, clave, p)

	ag := p.nuevoAgente(t, "agente-e7")
	c, err := ag.conectar(addr)
	if err != nil {
		t.Fatalf("handshake: %v", err)
	}
	defer c.cerrar()

	pkt := paqueteValido(clave, ag, ahoraNs(), 1)

	// La firma ya esta hecha. Ahora se altera lo que la firma NO cubre.
	contextoOriginal := pkt.contexto[0]
	cargaOriginal := string(pkt.ciphertext)
	pkt.contexto[0] = 999999.0
	pkt.ciphertext = []byte("CARGA-REEMPLAZADA-POR-UN-ATACANTE")
	pkt.flags = 0xDEAD

	resp, err := c.enviar(pkt)
	if err != nil {
		t.Fatalf("el gateway cerro sin responder: %v", err)
	}

	if resp != respAck {
		t.Errorf("D-47 parece cerrado: el paquete con Context y Ciphertext alterados "+
			"fue rechazado (%s). Si el HMAC se extendio, actualizar el contexto vivo y "+
			"borrar este test", nombreDeRespuesta(resp))
		return
	}

	t.Logf("E7 D-47 MEDIDO EN EL CAMINO REAL: Context[0] paso de %v a 999999, el "+
		"ciphertext paso de %q a %q, flags a 0x%X, y el gateway respondio %s. El "+
		"HMAC cubre 48 bytes de cabecera; el contexto que alimenta al "+
		"CoherenceFilter y el payload entero viajan SIN FIRMAR.",
		contextoOriginal, cargaOriginal, string(pkt.ciphertext), pkt.flags,
		nombreDeRespuesta(resp))
}

// ---------------------------------------------------------------------------
// E8 · LOS RECHAZOS DEVUELVEN EL MISMO BYTE
// ---------------------------------------------------------------------------

// TestE2E_08_TodosLosRechazosSonElMismoByte es el argumento de Testis, medido
// desde afuera por primera vez. Cinco causas distintas, una sola respuesta.
//
// El ADR de Testis decia "los 9 rechazos escriben el mismo byte 0xFF, asi que
// nadie puede saber por que se bloqueo un agente". Eso estaba contado leyendo el
// codigo. Aca esta medido: un cliente real, cinco causas, cinco bytes identicos.
//
// Y tiene una vuelta incomoda: esta indistinguibilidad es la razon por la que YO,
// auditando, no puedo atribuir los rechazos de este archivo sin un ACK de
// referencia. El agujero que Testis existe para llenar me mordio como auditor.
func TestE2E_08_TodosLosRechazosSonElMismoByte(t *testing.T) {
	addr, clave, p := arrancarGateway(t)

	casos := []struct {
		nombre string
		armar  func(ag *agente) (*paquete, []byte)
	}{
		{
			nombre: "HMAC invalido",
			armar: func(ag *agente) (*paquete, []byte) {
				k := paqueteValido(clave, ag, ahoraNs(), 1)
				k.hmac[0] ^= 0xFF
				return k, nil
			},
		},
		{
			nombre: "timestamp fuera de ventana",
			armar: func(ag *agente) (*paquete, []byte) {
				return paqueteValido(clave, ag, ahoraNs()+uint64(31*time.Second), 1), nil
			},
		},
		{
			nombre: "AgentID de otro",
			armar: func(ag *agente) (*paquete, []byte) {
				k := paqueteValido(clave, ag, ahoraNs(), 1)
				k.agentID[0] ^= 0xFF
				return k, nil
			},
		},
		{
			nombre: "contexto de largo equivocado",
			armar: func(ag *agente) (*paquete, []byte) {
				k := paqueteValido(clave, ag, ahoraNs(), 1)
				return nil, recortarUnFloatDelContexto(k.codificar())
			},
		},
		{
			nombre: "frame de tamano cero",
			armar: func(ag *agente) (*paquete, []byte) {
				frame := make([]byte, frameHeaderSize)
				binary.BigEndian.PutUint32(frame, 0)
				return nil, frame
			},
		},
	}

	respuestas := make(map[byte]int)
	var detalle []string

	for i, caso := range casos {
		// Un certificado por caso: si compartieran, el TriggerBlock del anterior
		// cambiaria el resultado del siguiente y esto mediria el orden.
		ag := p.nuevoAgente(t, fmt.Sprintf("agente-e8-%d", i))

		c, err := ag.conectar(addr)
		if err != nil {
			t.Fatalf("handshake del caso %q: %v", caso.nombre, err)
		}

		pkt, crudo := caso.armar(ag)
		var resp byte
		if crudo != nil {
			resp, err = c.enviarCrudo(crudo)
		} else {
			resp, err = c.enviar(pkt)
		}
		c.cerrar()

		if err != nil {
			detalle = append(detalle, fmt.Sprintf("  %-30s conexion cerrada sin responder", caso.nombre))
			continue
		}
		respuestas[resp]++
		detalle = append(detalle, fmt.Sprintf("  %-30s %s", caso.nombre, nombreDeRespuesta(resp)))
	}

	t.Logf("E8 CINCO CAUSAS DE RECHAZO DISTINTAS:\n%s", unir(detalle))

	if len(respuestas) == 0 {
		t.Fatal("ningun caso obtuvo respuesta: el arnes no midio nada")
	}
	if len(respuestas) > 1 {
		t.Errorf("los rechazos NO son indistinguibles: aparecieron %d bytes distintos "+
			"(%v). Si el gateway empezo a diferenciar causas, actualizar el ADR de "+
			"Testis", len(respuestas), respuestas)
		return
	}

	for b, n := range respuestas {
		t.Logf("E8 MEDIDO: %d causas distintas, UNA sola respuesta: %s. Desde el "+
			"cliente es imposible saber POR QUE lo rechazaron, y el gateway no escribe "+
			"un solo log. Ese es el agujero que Testis existe para llenar, y ahora esta "+
			"medido desde afuera en vez de contado leyendo el codigo.",
			n, nombreDeRespuesta(b))
	}
}

// recortarUnFloatDelContexto reescribe el frame para que el contexto tenga 7
// float64 en vez de 8, ajustando el largo del frame. Los largos internos quedan
// desalineados a proposito: cualquiera de las dos cosas produce un rechazo del
// decoder, que es lo que este caso mide.
//
// Es fragil por construccion: si el codificador cambia, no encuentra el patron y
// devuelve el frame intacto, y entonces el caso mide OTRA cosa.
func recortarUnFloatDelContexto(frame []byte) []byte {
	const largoContexto = 8 * ContextVectorSize

	// El contexto interno se codifica como: tag(0x0A) + len(64) + 64 bytes.
	idx := -1
	for i := 0; i+1 < len(frame); i++ {
		if frame[i] == 0x0A && frame[i+1] == byte(largoContexto) {
			idx = i
			break
		}
	}
	if idx < 0 || idx+2+largoContexto > len(frame) {
		return frame
	}

	nuevo := make([]byte, 0, len(frame)-8)
	nuevo = append(nuevo, frame[:idx+1]...)
	nuevo = append(nuevo, byte(largoContexto-8))
	nuevo = append(nuevo, frame[idx+2:idx+2+largoContexto-8]...)
	nuevo = append(nuevo, frame[idx+2+largoContexto:]...)

	if len(nuevo) > frameHeaderSize {
		binary.BigEndian.PutUint32(nuevo[:frameHeaderSize], uint32(len(nuevo)-frameHeaderSize))
	}
	return nuevo
}

func unir(lineas []string) string {
	var b []byte
	for i, l := range lineas {
		if i > 0 {
			b = append(b, '\n')
		}
		b = append(b, l...)
	}
	return string(b)
}

// ---------------------------------------------------------------------------
// E9 · EL SUJETO REAL DE D-26
// ---------------------------------------------------------------------------

// TestE2E_09_DosConexionesDelMismoCertificado es el test que todas las mediciones
// de concurrencia de este repo aproximaron con un proxy.
//
// Hasta hoy: 8 goroutines llamando `agente.Huber.Update` directamente, declarado
// como "los mismos metodos sobre el mismo puntero, pero no desde handleConn".
// Aca: DOS CONEXIONES TLS REALES con el MISMO certificado, o sea el mismo
// *AgentState via getOrCreateAgent, atravesando handleConn de punta a punta.
//
// Con un control: dos conexiones de certificados DISTINTOS, que no comparten
// AgentState. Si el caso compartido se degrada y el aislado no, la causa es el
// estado compartido y no la concurrencia en general.
func TestE2E_09_DosConexionesDelMismoCertificado(t *testing.T) {
	addr, clave, p := arrancarGateway(t)
	saltearSiElGatewayNoAcepta(t, addr, clave, p)

	const porConexion = 20

	compartido := p.nuevoAgente(t, "agente-e9-compartido")
	ackComp, rechComp, cerradasComp := martillar(t, addr, clave, compartido, compartido, porConexion)

	unoA := p.nuevoAgente(t, "agente-e9-aislado-a")
	unoB := p.nuevoAgente(t, "agente-e9-aislado-b")
	ackAisl, rechAisl, cerradasAisl := martillar(t, addr, clave, unoA, unoB, porConexion)

	t.Logf("E9 DOS CONEXIONES CONCURRENTES, %d paquetes por conexion\n"+
		"  MISMO cert (comparten AgentState) : ack=%-4d rechazo=%-4d conexiones cerradas=%d\n"+
		"  certs DISTINTOS (control)         : ack=%-4d rechazo=%-4d conexiones cerradas=%d",
		porConexion, ackComp, rechComp, cerradasComp, ackAisl, rechAisl, cerradasAisl)

	if ackAisl == 0 {
		t.Fatalf("CONTROL ROTO: dos conexiones de certificados distintos no lograron "+
			"ni un ACK (ack=%d rechazo=%d cerradas=%d)", ackAisl, rechAisl, cerradasAisl)
	}

	t.Logf("E9 D-26 MEDIDO EN EL CALLSITE REAL: los filtros se ejercitaron desde dos "+
		"conexiones TLS concurrentes sobre el mismo *AgentState, atravesando "+
		"handleConn. Bajo -race, cero reportes = el mutex de filters.go cubre el "+
		"camino real, no solo el proxy.")

	if rechComp+cerradasComp > rechAisl+cerradasAisl {
		t.Logf("E9 HALLAZGO: el caso compartido perdio %d paquetes (rechazo+cerradas) "+
			"contra %d del aislado. La marca de agua anti-replay es POR AGENTE, asi que "+
			"dos conexiones del mismo certificado se pisan los timestamps: la que llega "+
			"tarde cae en el chequeo `<=` y arrastra un TriggerBlock. El gateway YA "+
			"castiga las conexiones concurrentes del mismo cert, pero con bloqueos en vez "+
			"de un rechazo limpio.", rechComp+cerradasComp, rechAisl+cerradasAisl)
	} else {
		t.Logf("E9 el caso compartido NO se degrado mas que el aislado en esta corrida "+
			"(%d contra %d paquetes perdidos). Con 2 conexiones y %d paquetes la ventana "+
			"de colision es chica; que no aparezca no prueba que no exista.",
			rechComp+cerradasComp, rechAisl+cerradasAisl, porConexion)
	}
}

// martillar abre dos conexiones concurrentes y manda `porConexion` paquetes por
// cada una, contando acks, rechazos y conexiones que el gateway cerro.
func martillar(t *testing.T, addr string, clave []byte, a, b *agente, porConexion int) (ack, rechazo, cerradas int) {
	t.Helper()

	var mu sync.Mutex
	var wg sync.WaitGroup

	lanzar := func(ag *agente, base uint32) {
		defer wg.Done()

		c, err := ag.conectar(addr)
		if err != nil {
			mu.Lock()
			cerradas++
			mu.Unlock()
			return
		}
		defer c.cerrar()

		for i := 0; i < porConexion; i++ {
			resp, err := c.enviar(paqueteValido(clave, ag, ahoraNs(), base+uint32(i)))
			mu.Lock()
			switch {
			case err != nil:
				cerradas++
			case resp == respAck:
				ack++
			default:
				rechazo++
			}
			mu.Unlock()
			if err != nil {
				return // el gateway cerro: no hay mas nada que mandar por aca
			}
		}
	}

	wg.Add(2)
	go lanzar(a, 1000)
	go lanzar(b, 5000)
	wg.Wait()

	return ack, rechazo, cerradas
}
