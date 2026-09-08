// Copyright (c) 2026 Jorge Abraham Mendieta.
// Computational Substrate Theory. Todos los derechos reservados.

// EL PRIMER PAQUETE, DESPUES DEL FIX. Y lo que el fix NO arregla.
//
// ===========================================================================
// HISTORIA, para que nadie la reconstruya:
//
// La version anterior de este archivo afirmaba el DEFECTO: "el gateway rechaza
// todo primer paquete", medido con 0 de 11 combinaciones de payload y espera
// logrando un ACK. Era un test de caracterizacion, y su comentario decia que
// cuando el defecto se arreglara TENIA que dar rojo. Se cumplio, y por eso este
// archivo esta invertido.
//
// EL FIX fueron dos cambios, y el experimento de cuatro brazos del job mide cual
// carga el peso:
//
//   1. gateway/filters.go: la primera muestra INICIALIZA el filtro y devuelve
//      false. No se puede detectar un CAMBIO de varianza con n=1.
//   2. gateway/gateway.go: `llegada := time.Now()` DESPUES del io.ReadFull, y de
//      ahi salen `dt` y el reloj del anti-replay. Antes los dos usaban un `now`
//      capturado ANTES de esperar el paquete, asi que `dt` media el tiempo entre
//      dos inicios de iteracion del servidor y no entre paquetes.
//
// LO QUE EL FIX NO ARREGLA, y este archivo lo afirma en E0c: D-48 sigue vivo.
// Con payloads VARIABLES el filtro bloquea igual, y la tolerancia depende de dt
// (a 40 ms basta un +11% de cambio de tamano). Eso necesita reemplazar el
// algoritmo por Page-Hinkley, que es decision de producto.
// ===========================================================================

package gateway

import (
	"fmt"
	"math"
	"testing"
	"time"
)

// resultados posibles de un intento.
const (
	resAck     = "ACK"
	resRechazo = "RECHAZO"
	resCerrada = "CERRADA"
)

// dtMinimoPredicho es LA PREDICCION QUE LA MEDICION REFUTO en el turno anterior,
// y queda en el archivo a proposito.
//
// El despeje es correcto PARA EL FILTRO: dt > v/umbral. Lo que estaba mal era la
// premisa de que el cliente pudiera influir en ese dt, porque `dt` no se medida
// entre paquetes. Con el fix de gateway.go ahora SI se mide entre paquetes, asi
// que esta funcion pasa de ser una prediccion falsa a ser la cota real de
// tolerancia del filtro una vez que hay historia.
func dtMinimoPredicho(bytes int) float64 {
	x := float64(bytes) / 1024.0

	s := DefaultAlpha * x
	delta := x
	if clamp := 3 * DefaultSigma; delta > clamp {
		delta = clamp
	}
	mu := DefaultAlpha * delta
	residual := x - mu
	v := DefaultAlpha * residual * residual

	inertia := 1.0 + math.Log1p(s)
	umbral := 0.05 * math.Sqrt(v+DefaultEpsilon) * inertia
	if umbral <= 0 {
		return 0
	}
	return v / umbral
}

// intentarUnPaquete abre una conexion con un cert NUEVO, espera `espera`, manda un
// solo paquete valido de `bytes` bytes y devuelve que contesto el gateway.
//
// Un cert por intento es obligatorio: un rechazo dispara TriggerBlock, y con el
// cert compartido el intento siguiente mediria el backoff y no el filtro.
func intentarUnPaquete(t *testing.T, addr string, clave []byte, p *pki, nombre string, bytes int, espera time.Duration) string {
	t.Helper()

	ag := p.nuevoAgente(t, nombre)
	c, err := ag.conectar(addr)
	if err != nil {
		t.Fatalf("handshake de %q: %v", nombre, err)
	}
	defer c.cerrar()

	time.Sleep(espera)

	resp, err := c.enviarSinPausa(paqueteConCarga(clave, ag, ahoraNs(), 1, bytes))
	switch {
	case err != nil:
		return resCerrada
	case resp == respAck:
		return resAck
	default:
		return resRechazo
	}
}

// elGatewayAceptaAlgo prueba si ESTE gateway acepta un primer paquete.
//
// Con el fix puesto devuelve true siempre, asi que ningun test se saltea. Se
// conserva porque los tests que necesitan un ACK previo tienen que poder declarar
// NO MEDIDO en vez de fallar si alguien revierte el fix o corre el arnes contra
// una version vieja.
func elGatewayAceptaAlgo(t *testing.T, addr string, clave []byte, p *pki) bool {
	t.Helper()
	return intentarUnPaquete(t, addr, clave, p,
		fmt.Sprintf("sonda-%s", t.Name()), cargaChica, pausaAntesDelPrimero) == resAck
}

// saltearSiElGatewayNoAcepta declara NO MEDIDO con su razon.
func saltearSiElGatewayNoAcepta(t *testing.T, addr string, clave []byte, p *pki) {
	t.Helper()
	if elGatewayAceptaAlgo(t, addr, clave, p) {
		return
	}
	t.Skipf("NO MEDIDO: este gateway no acepta NINGUN primer paquete, asi que no hay "+
		"forma de llegar al estado que este test necesita. Con el fix de E0 puesto "+
		"esto no deberia pasar: si aparece, el fix se revirtio o se esta corriendo el "+
		"arnes contra una version vieja del gateway.")
}

// ---------------------------------------------------------------------------
// E0 · EL BARRIDO, INVERTIDO
// ---------------------------------------------------------------------------

// TestE2E_00_ElGatewayAceptaElPrimerPaquete es el mismo barrido de antes con el
// veredicto dado vuelta. Antes del fix: 0 ACK de 11. Se espera 11 de 11.
func TestE2E_00_ElGatewayAceptaElPrimerPaquete(t *testing.T) {
	addr, clave, p := arrancarGateway(t)

	casos := []struct {
		bytes  int
		espera time.Duration
	}{
		{cargaChica, 0},
		{cargaChica, 20 * time.Millisecond},
		{cargaChica, 100 * time.Millisecond},
		{cargaChica, 300 * time.Millisecond},
		{64, 0},
		{64, 300 * time.Millisecond},
		{64, 600 * time.Millisecond},
		{256, 0},
		{256, 1000 * time.Millisecond},
		{256, 1900 * time.Millisecond},
		{512, 1900 * time.Millisecond},
	}

	var filas []string
	acks, rechazos, cerradas := 0, 0, 0

	for i, caso := range casos {
		nombre := fmt.Sprintf("agente-e0-%d-%dB-%dms", i, caso.bytes, caso.espera.Milliseconds())
		got := intentarUnPaquete(t, addr, clave, p, nombre, caso.bytes, caso.espera)

		switch got {
		case resAck:
			acks++
		case resRechazo:
			rechazos++
		default:
			cerradas++
		}

		filas = append(filas, fmt.Sprintf(
			"  payload=%-5d B  espera=%-7v  ->  %s", caso.bytes, caso.espera, got))
	}

	t.Logf("E0 BARRIDO CONTRA EL GATEWAY REAL (read deadline: %v)\n%s\n"+
		"  ACK=%d  RECHAZO=%d  CERRADA=%d  de %d casos",
		defaultReadDeadline, unirLineas(filas), acks, rechazos, cerradas, len(casos))

	if acks != len(casos) {
		t.Fatalf("el fix NO cierra el hallazgo: solo %d de %d primeros paquetes fueron "+
			"aceptados (rechazo=%d cerrada=%d). Antes del fix eran 0 de %d",
			acks, len(casos), rechazos, cerradas, len(casos))
	}

	t.Logf("E0 FIX MEDIDO: %d de %d primeros paquetes ACEPTADOS, con cualquier "+
		"payload y con o sin pausa. Antes del fix eran 0 de %d. El gateway puede "+
		"atender a un agente por primera vez en la vida del proyecto.",
		acks, len(casos), len(casos))
}

// ---------------------------------------------------------------------------
// E0b · EL CONTROL QUE HACE QUE EL FIX VALGA
// ---------------------------------------------------------------------------

// TestE2E_00b_ControlPositivoLaRafagaSigueBloqueada es el control sin el cual el
// fix seria indistinguible de haber desarmado la deteccion.
//
// Un filtro que acepta todo "arregla" el primer paquete y rompe el producto. Este
// test manda paquetes estables y despues UNA rafaga grande sobre la MISMA
// conexion, y exige que la rafaga sea bloqueada.
func TestE2E_00b_ControlPositivoLaRafagaSigueBloqueada(t *testing.T) {
	addr, clave, p := arrancarGateway(t)
	ag := p.nuevoAgente(t, "agente-e0b")

	c, err := ag.conectar(addr)
	if err != nil {
		t.Fatalf("handshake: %v", err)
	}
	defer c.cerrar()

	const estables = 8
	const rafagaBytes = 16384

	// Tramo estable: todos del mismo tamano, asi que la varianza no se mueve.
	for i := 0; i < estables; i++ {
		resp, err := c.enviar(paqueteConCarga(clave, ag, ahoraNs(), uint32(i+1), cargaChica))
		if err != nil {
			t.Fatalf("el gateway cerro en el paquete estable %d: %v", i+1, err)
		}
		if resp != respAck {
			t.Fatalf("el paquete estable %d de %d B fue rechazado (%s): el fix no "+
				"sostiene una conexion normal", i+1, cargaChica, nombreDeRespuesta(resp))
		}
	}
	t.Logf("E0b los %d paquetes estables de %d B fueron ACK", estables, cargaChica)

	// La rafaga: 2048 veces el tamano del tramo estable.
	resp, err := c.enviar(paqueteConCarga(clave, ag, ahoraNs(), 999, rafagaBytes))
	if err != nil {
		t.Logf("E0b CONTROL OK: la rafaga de %d B hizo que el gateway CIERRE la "+
			"conexion (%v). Tambien es un bloqueo.", rafagaBytes, err)
		return
	}

	if resp == respAck {
		t.Fatalf("CONTROL ROTO: una rafaga de %d B despues de %d paquetes de %d B fue "+
			"ACEPTADA. El fix desarmo la deteccion: el filtro ya no bloquea nada y "+
			"'arreglar el primer paquete' se convirtio en romper el producto.",
			rafagaBytes, estables, cargaChica)
	}

	t.Logf("E0b CONTROL POSITIVO OK: la rafaga de %d B (2048x el tramo estable) fue "+
		"%s. El fix NO desarmo la deteccion: el filtro sigue bloqueando un cambio "+
		"real de varianza.", rafagaBytes, nombreDeRespuesta(resp))
}

// ---------------------------------------------------------------------------
// E0c · LO QUE EL FIX NO ARREGLA (D-48), CON NUMERO
// ---------------------------------------------------------------------------

// TestE2E_00c_D48_LosPayloadsVariablesSiguenBloqueados es un test de
// CARACTERIZACION del defecto que queda abierto.
//
// El fix de E0 cura el primer paquete. NO cura D-48: la derivada esta en var/s y
// el umbral en unidades de desvio, asi que la tolerancia a variacion de tamano
// depende de dt. Modelado sobre la aritmetica del filtro, con historia ya hecha:
//
//	dt = 1 ms   -> bloquea con +1,6%  de cambio de tamano
//	dt = 40 ms  -> bloquea con +10,9%
//	dt = 1 s    -> bloquea con +243,8%
//
// Cuanto mas rapido habla el agente, MENOS variacion tolera. Este test lo mide
// sobre el camino real: un cliente que manda tamanos variables a ritmo normal es
// bloqueado.
//
// CUANDO D-48 SE ARREGLE, ESTE TEST DEBE DAR ROJO. Ese rojo es la senal de
// borrarlo y cerrar D-48, no un test que se rompio.
func TestE2E_00c_D48_LosPayloadsVariablesSiguenBloqueados(t *testing.T) {
	addr, clave, p := arrancarGateway(t)
	ag := p.nuevoAgente(t, "agente-e0c")

	c, err := ag.conectar(addr)
	if err != nil {
		t.Fatalf("handshake: %v", err)
	}
	defer c.cerrar()

	// Tamanos que un cliente real tendria: un pedido chico, una respuesta mediana,
	// otro pedido chico. Nada de esto es un ataque.
	tamanos := []int{64, 96, 64, 128, 64, 192, 64}

	ack, rechazo := 0, 0
	var filas []string
	primerRechazo := -1

	for i, b := range tamanos {
		resp, err := c.enviar(paqueteConCarga(clave, ag, ahoraNs(), uint32(i+1), b))
		var got string
		switch {
		case err != nil:
			got = resCerrada
			rechazo++
		case resp == respAck:
			got = resAck
			ack++
		default:
			got = resRechazo
			rechazo++
		}
		if got != resAck && primerRechazo < 0 {
			primerRechazo = i + 1
		}
		filas = append(filas, fmt.Sprintf("  paquete %d  payload=%-4d B  -> %s", i+1, b, got))
		if err != nil {
			break // el gateway cerro: no hay mas nada que mandar
		}
	}

	t.Logf("E0c CLIENTE CON PAYLOADS VARIABLES (64 a 192 B, pausa de %v)\n%s\n"+
		"  ACK=%d  no-ACK=%d   primer no-ACK en el paquete %d",
		pausaEntrePaquetes, unirLineas(filas), ack, rechazo, primerRechazo)

	if rechazo == 0 {
		t.Errorf("D-48 parece CERRADO: un cliente con payloads variables completo los "+
			"%d paquetes sin un solo rechazo. Si el filtro se reemplazo por "+
			"Page-Hinkley, actualizar el contexto vivo, cerrar D-48 y borrar este test",
			len(tamanos))
		return
	}

	t.Logf("E0c D-48 SIGUE ABIERTO, medido en el camino real: un cliente que manda "+
		"tamanos normales y variables es bloqueado en el paquete %d. El fix de E0 cura "+
		"el PRIMER paquete y nada mas: el gateway sigue inutilizable para cualquier "+
		"agente cuyo trafico no sea de tamano constante. La tolerancia depende de dt "+
		"(a 40 ms alcanza un +11%% de cambio), y eso es la inconsistencia dimensional "+
		"de D-48: hace falta Page-Hinkley, no un parche.", primerRechazo)
}

// ---------------------------------------------------------------------------
// E0d · LA VENTANA ANTI-REPLAY, AHORA MEDIDA DESDE LA LLEGADA
// ---------------------------------------------------------------------------

// TestE2E_00d_LaVentanaSeMideDesdeLaLlegada mide el segundo efecto del fix de
// gateway.go, que es de seguridad y no de disponibilidad.
//
// ANTES: `nowNs` salía de un `now` capturado ANTES del io.ReadFull, asi que un
// cliente que se tomaba su tiempo hacia que el gateway comparara el timestamp del
// paquete contra un reloj viejo. Con un read deadline de 2 s, la ventana efectiva
// hacia atras era de hasta 30 s + 2 s = 32 s: un replay de 32 segundos de
// antiguedad podia entrar.
//
// DOS BRAZOS, porque un solo resultado no distingue el fix del azar:
//
//	(a) pausa de 1,9 s y ts de 29,5 s de antiguedad -> RECHAZO (31,4 s en la
//	    llegada, fuera de la ventana de 30 s)
//	(b) sin pausa y el MISMO ts de 29,5 s          -> ACK (dentro de la ventana)
//
// Con el codigo viejo los dos daban ACK, porque los dos se comparaban contra el
// mismo reloj pre-lectura.
func TestE2E_00d_LaVentanaSeMideDesdeLaLlegada(t *testing.T) {
	addr, clave, p := arrancarGateway(t)

	const antiguedad = 29500 * time.Millisecond
	const pausaLarga = 1900 * time.Millisecond

	medir := func(nombre string, pausa time.Duration) string {
		ag := p.nuevoAgente(t, nombre)
		c, err := ag.conectar(addr)
		if err != nil {
			t.Fatalf("handshake de %q: %v", nombre, err)
		}
		defer c.cerrar()

		// El timestamp se calcula AHORA, con el handshake recien terminado, que es
		// aproximadamente cuando el gateway captura su `now` pre-lectura.
		ts := ahoraNs() - uint64(antiguedad)

		time.Sleep(pausa)

		resp, err := c.enviarSinPausa(paqueteConCarga(clave, ag, ts, 1, cargaChica))
		switch {
		case err != nil:
			return resCerrada
		case resp == respAck:
			return resAck
		default:
			return resRechazo
		}
	}

	conPausa := medir("agente-e0d-con-pausa", pausaLarga)
	sinPausa := medir("agente-e0d-sin-pausa", 0)

	t.Logf("E0d VENTANA ANTI-REPLAY (declarada: %v)\n"+
		"  ts de %v de antiguedad, pausa de %v antes de mandar -> %s\n"+
		"  el MISMO ts, sin pausa                              -> %s",
		time.Duration(maxTimestampDriftNs), antiguedad, pausaLarga, conPausa, sinPausa)

	if sinPausa != resAck {
		t.Fatalf("CONTROL ROTO: un ts de %v de antiguedad SIN pausa fue %s, y la "+
			"ventana declarada es %v. Si esto no entra, el brazo con pausa no mide la "+
			"ventana sino otra cosa", antiguedad, sinPausa, time.Duration(maxTimestampDriftNs))
	}

	if conPausa == resAck {
		t.Errorf("la ventana NO se mide desde la llegada: con %v de pausa, un ts de %v "+
			"de antiguedad (o sea %v en la llegada, fuera de la ventana de %v) fue "+
			"ACEPTADO. El reloj del anti-replay sigue siendo el pre-lectura",
			pausaLarga, antiguedad, antiguedad+pausaLarga,
			time.Duration(maxTimestampDriftNs))
		return
	}

	t.Logf("E0d FIX MEDIDO: el reloj del anti-replay es ahora el de la LLEGADA del "+
		"paquete. Con el codigo viejo los dos brazos daban ACK, porque los dos se "+
		"comparaban contra un `now` capturado antes de esperar el paquete: la ventana "+
		"efectiva hacia atras era de hasta %v + %v = %v, no los %v declarados.",
		time.Duration(maxTimestampDriftNs), defaultReadDeadline,
		time.Duration(maxTimestampDriftNs)+defaultReadDeadline,
		time.Duration(maxTimestampDriftNs))
}

func unirLineas(l []string) string {
	var b []byte
	for i, s := range l {
		if i > 0 {
			b = append(b, '\n')
		}
		b = append(b, s...)
	}
	return string(b)
}
