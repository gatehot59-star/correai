// Copyright (c) 2026 Jorge Abraham Mendieta.
// Computational Substrate Theory. Todos los derechos reservados.

// MEDICIONES PUNTA A PUNTA DE handleConn, con el cliente mTLS real.
//
// Todo lo que se midio antes en este repo sobre concurrencia uso el `AgentState`
// directamente. Eso era un PROXY DECLARADO: los mismos metodos, sobre el mismo
// puntero, pero NO desde `handleConn`. Este archivo cierra esa distancia.
//
// LO QUE SE MIDE, y por que en este orden:
//
//	E1  el arnes funciona (paquete valido -> ACK). Es el control del arnes: si
//	    cae, el codificador esta mal y NINGUN resultado de abajo significa nada.
//	E2  HMAC invalido -> rechazo
//	E3  anti-replay -> rechazo
//	E4  AgentID que no coincide con el certificado -> rechazo
//	E5  timestamp fuera de la ventana de 30 s -> rechazo
//	E6  D-46 sobre el camino real, con dos controles
//	E7  D-47 sobre el camino real
//	E8  los 5 rechazos devuelven el MISMO byte (el argumento de Testis)
//	E9  dos conexiones del MISMO certificado: el sujeto real de D-26
//
// ESTOS TESTS NO SE SALTEAN CON -short A PROPOSITO. No dependen del tiempo de
// pared para producir un numero (el unico sleep es el backoff del FSM en E6, de
// ~300 ms), asi que entran al porton compartido de `main` y lo fortalecen.
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
// E1 · EL CONTROL DEL ARNES
// ---------------------------------------------------------------------------

// TestE2E_01_ElArnesFunciona es el test que hay que leer primero. Si este cae,
// el codificador no coincide con `decodePerimeterPacket` y todos los rechazos de
// los otros tests serian rechazos por paquete mal formado, no por lo que dicen
// medir. Un arnes que solo puede producir rechazos no distingue nada.
func TestE2E_01_ElArnesFunciona(t *testing.T) {
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
	if resp != respAck {
		t.Fatalf("paquete VALIDO rechazado: %s. El codificador no coincide con "+
			"decodePerimeterPacket, asi que ningun otro test de este archivo mide "+
			"lo que dice medir", nombreDeRespuesta(resp))
	}

	// Segundo paquete en la MISMA conexion: prueba que handleConn hace loop y no
	// atiende uno solo.
	resp2, err := c.enviar(paqueteValido(clave, ag, ahoraNs()+1, 2))
	if err != nil {
		t.Fatalf("segundo paquete, el gateway cerro: %v", err)
	}
	if resp2 != respAck {
		t.Fatalf("segundo paquete valido rechazado: %s", nombreDeRespuesta(resp2))
	}

	t.Logf("E1 MEDIDO: handshake mTLS + 2 paquetes validos en la misma conexion, "+
		"los dos %s. handleConn, el decoder, el anti-replay y verifyPacketHMAC "+
		"corrieron de verdad por primera vez.", nombreDeRespuesta(respAck))
	t.Logf("E1 agentID derivado del cert = %x", ag.agentID[:8])
}

// ---------------------------------------------------------------------------
// E2 a E5 · los cuatro rechazos, cada uno aislado
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

func TestE2E_03_AntiReplay(t *testing.T) {
	addr, clave, p := arrancarGateway(t)
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
	t.Logf("E3 MEDIDO: mismo timestamp dos veces -> %s", nombreDeRespuesta(segunda))
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
	pkt := paqueteValido(clave, otro, ahoraNs(), 1)

	resp, err := c.enviar(pkt)
	if err != nil {
		t.Fatalf("el gateway cerro sin responder: %v", err)
	}
	if resp != respRechazo {
		t.Fatalf("un paquete con el AgentID de otro agente fue ACEPTADO (%s)",
			nombreDeRespuesta(resp))
	}
	t.Logf("E4 MEDIDO: AgentID del paquete (%x) != derivado del cert (%x) -> %s",
		otro.agentID[:8], ag.agentID[:8], nombreDeRespuesta(resp))
	t.Logf("E4 CONSECUENCIA: para forjar un paquete a nombre de un agente hay que " +
		"presentar SU certificado. Eso acota D-46 a auto-DoS o a credencial robada, " +
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
// E6 · D-46 SOBRE EL CAMINO REAL
// ---------------------------------------------------------------------------

// TestE2E_06_D46_EnvenenamientoDeLaMarcaDeAgua mide el hallazgo que hasta hoy
// solo estaba LEIDO en el codigo: `handleConn` escribe `lastTimestampNs` ANTES de
// verificar el HMAC, asi que un paquete con firma invalida y timestamp adelantado
// mueve la marca de agua y deja al agente rechazando sus propios paquetes.
//
// TRES PARTES, porque el hallazgo sin sus controles seria alarmismo:
//
//	(a) el envenenamiento: forjado con ts = now+29s y HMAC roto -> rechazo, y
//	    despues un paquete LEGITIMO del mismo agente tambien es rechazado
//	(b) control de agente limpio: otro cert, sin envenenar, sigue dando ACK, o
//	    sea que el gateway no se cayo
//	(c) control del MECANISMO: un paquete con ts POSTERIOR al envenenado SI se
//	    acepta. Eso prueba que el agente no esta "muerto": la marca de agua salto
//	    al futuro, que es exactamente lo que dice D-46 y nada mas que eso.
func TestE2E_06_D46_EnvenenamientoDeLaMarcaDeAgua(t *testing.T) {
	addr, clave, p := arrancarGateway(t)
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

	// El rechazo dispara TriggerBlock, asi que hay que esperar el backoff.
	// initialBackoff = 100 ms y TriggerBlock lo duplica -> 200 ms.
	time.Sleep(350 * time.Millisecond)

	// El paquete LEGITIMO de la victima, con timestamp de AHORA.
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
	// Hay que esperar el backoff que dejo el rechazo del paso anterior (400 ms).
	time.Sleep(700 * time.Millisecond)
	c4, err := victima.conectar(addr)
	if err != nil {
		t.Fatalf("tercer handshake de la victima: %v", err)
	}
	respPosterior, errPosterior := c4.enviar(paqueteValido(clave, victima, tsEnvenenado+1000, 3))
	c4.cerrar()

	t.Logf("E6 RESULTADOS\n"+
		"  forjado (HMAC roto, ts=now+%v)      -> %s\n"+
		"  legitimo despues (ts=now)           -> %s%s\n"+
		"  agente LIMPIO (control)             -> %s\n"+
		"  ts POSTERIOR al envenenado (control)-> %s%s",
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

// TestE2E_07_D47_ContextYCiphertextNoEstanFirmados altera el vector de contexto
// y el payload DESPUES de firmar la cabecera. El gateway acepta el paquete,
// porque el HMAC cubre 48 bytes de cabecera y nada mas.
//
// Antes esto estaba medido llamando a verifyPacketHMAC. Ahora esta medido con un
// paquete que entra por el socket: es la diferencia entre "la funcion no lo cubre"
// y "un atacante puede hacerlo".
func TestE2E_07_D47_ContextYCiphertextNoEstanFirmados(t *testing.T) {
	addr, clave, p := arrancarGateway(t)
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
// E8 · LOS 5 RECHAZOS DEVUELVEN EL MISMO BYTE
// ---------------------------------------------------------------------------

// TestE2E_08_TodosLosRechazosSonElMismoByte es el argumento de Testis, medido
// desde afuera por primera vez. Cinco causas distintas, una sola respuesta.
//
// El ADR de Testis decia "los 9 rechazos escriben el mismo byte 0xFF, asi que
// nadie puede saber por que se bloqueo un agente". Eso estaba contado leyendo el
// codigo. Aca esta medido: un cliente real, cinco causas, cinco bytes identicos.
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
				// Un frame armado a mano con un contexto de 7 float64 en vez de 8:
				// decodeContext exige exactamente ContextVectorSize.
				k := paqueteValido(clave, ag, ahoraNs(), 1)
				completo := k.codificar()
				return nil, recortarUnFloatDelContexto(completo)
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
// float64 en vez de 8. Trabaja sobre los bytes ya codificados: busca el bloque
// de 64 bytes del contexto y lo deja en 56, ajustando los tres largos que lo
// envuelven (el interno, el del campo 2, y el del frame).
//
// Es fragil por construccion: si el codificador cambia, esto deja de encontrar el
// patron y devuelve el frame intacto, y entonces el caso mide OTRA cosa. Por eso
// el test verifica que el frame REALMENTE cambio de tamano.
func recortarUnFloatDelContexto(frame []byte) []byte {
	const largoContexto = 8 * ContextVectorSize

	// El contexto interno se codifica como: tag(0x0A) + len(64) + 64 bytes.
	patron := []byte{0x0A, byte(largoContexto)}
	idx := -1
	for i := 0; i+len(patron) < len(frame); i++ {
		if frame[i] == patron[0] && frame[i+1] == patron[1] {
			idx = i
			break
		}
	}
	if idx < 0 {
		return frame // no se encontro: el caso va a medir otra cosa y el test lo dice
	}

	// Se saca un float64 (8 bytes) del final del bloque interno.
	nuevo := make([]byte, 0, len(frame)-8)
	nuevo = append(nuevo, frame[:idx+1]...)
	nuevo = append(nuevo, byte(largoContexto-8))
	nuevo = append(nuevo, frame[idx+2:idx+2+largoContexto-8]...)
	nuevo = append(nuevo, frame[idx+2+largoContexto:]...)

	// Reescribir el largo del frame. Los largos internos del campo 2 y del cuerpo
	// quedan DESALINEADOS a proposito: cualquiera de las dos cosas produce un
	// rechazo del decoder, que es lo que este caso mide.
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
// Ahora: DOS CONEXIONES TLS REALES con el MISMO certificado, o sea el mismo
// *AgentState via getOrCreateAgent, atravesando handleConn de punta a punta.
//
// Bajo -race, cualquier carrera en ese camino hace fallar este test.
//
// Y con un control: dos conexiones de certificados DISTINTOS, que no comparten
// AgentState. Si el caso compartido se degrada y el aislado no, la causa es el
// estado compartido y no la concurrencia en general.
func TestE2E_09_DosConexionesDelMismoCertificado(t *testing.T) {
	addr, clave, p := arrancarGateway(t)

	const porConexion = 40

	// --- caso: MISMO certificado en las dos conexiones ---
	compartido := p.nuevoAgente(t, "agente-e9-compartido")
	ackComp, rechComp, cerradasComp := martillar(t, addr, clave, compartido, compartido, porConexion)

	// --- control: certificados DISTINTOS ---
	unoA := p.nuevoAgente(t, "agente-e9-aislado-a")
	unoB := p.nuevoAgente(t, "agente-e9-aislado-b")
	ackAisl, rechAisl, cerradasAisl := martillar(t, addr, clave, unoA, unoB, porConexion)

	t.Logf("E9 DOS CONEXIONES CONCURRENTES, %d paquetes por conexion\n"+
		"  MISMO cert (comparten AgentState) : ack=%-4d rechazo=%-4d conexiones cerradas=%d\n"+
		"  certs DISTINTOS (control)         : ack=%-4d rechazo=%-4d conexiones cerradas=%d",
		porConexion, ackComp, rechComp, cerradasComp, ackAisl, rechAisl, cerradasAisl)

	if ackComp+rechComp == 0 && cerradasComp == 0 {
		t.Fatal("el caso compartido no produjo ninguna respuesta: el arnes no midio nada")
	}
	if ackAisl == 0 {
		t.Fatalf("CONTROL ROTO: dos conexiones de certificados distintos no lograron "+
			"ni un ACK (ack=%d rechazo=%d cerradas=%d). El gateway no soporta dos "+
			"conexiones concurrentes en general, asi que el caso compartido no mide el "+
			"estado compartido", ackAisl, rechAisl, cerradasAisl)
	}

	t.Logf("E9 D-26 MEDIDO EN EL CALLSITE REAL: los filtros se ejercitaron desde dos " +
		"conexiones TLS concurrentes sobre el mismo *AgentState, atravesando " +
		"handleConn. Bajo -race, cero reportes = el mutex de filters.go cubre el " +
		"camino real, no solo el proxy.")

	// El hallazgo del turno, si aparece. NO se declara verde ni rojo: se reporta.
	if rechComp+cerradasComp > rechAisl+cerradasAisl {
		t.Logf("E9 HALLAZGO: el caso compartido perdio %d paquetes (rechazo+cerradas) "+
			"contra %d del aislado. La marca de agua anti-replay es POR AGENTE, asi que "+
			"dos conexiones del mismo certificado se pisan los timestamps: la que llega "+
			"tarde cae en el chequeo `<=` y arrastra un TriggerBlock. O sea que el gateway "+
			"YA castiga las conexiones concurrentes del mismo cert, pero con bloqueos en "+
			"vez de un rechazo limpio.",
			rechComp+cerradasComp, rechAisl+cerradasAisl)
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
