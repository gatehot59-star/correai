// Copyright (c) 2026 Jorge Abraham Mendieta.
// Computational Substrate Theory. Todos los derechos reservados.

// EL GATEWAY NO ACEPTA NINGUN PAQUETE. Medido, no deducido.
//
// ===========================================================================
// COMO APARECIO. El control del arnes del cliente mTLS (un paquete perfectamente
// formado y bien firmado) fue RECHAZADO. Antes de tocar el arnes reproduje la
// aritmetica del HuberFilter y saque una hipotesis con numero: en el primer
// paquete lastV=0, asi que `derivative = v/dt`, y con dt chico eso queda muy sobre
// el umbral. De ahi predije que bastaba con que el cliente PAUSARA antes de
// mandar: ~7,2 s por KB de payload.
//
// LA MEDICION REFUTO ESA PREDICCION. 11 de 11 casos dieron RECHAZO, incluidos los
// 4 en los que yo predecia ACK. Gana la medicion.
//
// LA CAUSA REAL, leida en handleConn despues de que el barrido me contradijo:
//
//	lastSeen := time.Now()
//	for {
//	    now := time.Now()          // <- se captura ANTES de leer del socket
//	    ...
//	    io.ReadFull(conn, ...)     // <- ACA se espera el paquete del cliente
//	    ...
//	    dt := now.Sub(lastSeen).Seconds()
//
// `dt` no mide el tiempo entre paquetes: mide el tiempo entre dos INICIOS de
// iteracion del loop del servidor. En la primera iteracion `lastSeen` y `now` se
// tomaron a microsegundos de distancia, con el ReadFull todavia por delante. La
// pausa del cliente NO ENTRA EN dt, y por eso mi "esperar 7,2 s por KB" era falso.
//
// CONSECUENCIA, y es de producto, no de test:
//
//	EL GATEWAY RECHAZA EL PRIMER PAQUETE DE TODA CONEXION, SIEMPRE, con
//	cualquier payload y sin importar lo que haga el cliente. Y como el rechazo
//	hace TriggerBlock + writeReject + return, ninguna conexion pasa nunca de
//	un paquete. HiperSec no puede procesar un solo paquete de ningun agente.
//
// Nadie lo sabia porque nadie habia mandado un paquete: hasta este commit el repo
// no tenia cliente.
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

// dtMinimoPredicho es LA PREDICCION QUE LA MEDICION REFUTO. Queda en el archivo a
// proposito: sirve para mostrar en la tabla la distancia entre lo que calcule y lo
// que paso, y es la unica forma de que el proximo que lea esto no repita el
// razonamiento.
//
// El despeje es correcto PARA EL FILTRO: dt > v/umbral. Lo que estaba mal era la
// premisa de que el cliente pudiera influir en ese dt.
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
// Existe porque el gateway real NO acepta ninguno, y los tests que necesitan un
// ACK para medir su hallazgo (el replay, D-46, D-47, D-26 en el callsite) tienen
// que poder declarar NO MEDIDO en vez de fallar. Sobre un gateway con el bloque
// del HuberFilter desactivado esos mismos tests SI miden, y el sujeto queda
// declarado en el nombre de la corrida.
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
	t.Skipf("NO MEDIDO: este gateway no acepta NINGUN primer paquete (ver E0), asi " +
		"que no hay forma de llegar al estado que este test necesita medir. El " +
		"hallazgo se puede medir sobre una copia con el bloque del HuberFilter " +
		"desactivado; ese sujeto es OTRO y el job lo corre aparte.")
}

// ---------------------------------------------------------------------------
// E0 · EL BARRIDO
// ---------------------------------------------------------------------------

// TestE2E_00_ElGatewayRechazaTodoPrimerPaquete barre el espacio (payload x espera)
// y afirma lo que la medicion mostro: no hay combinacion que el gateway acepte.
//
// ES UN TEST DE CARACTERIZACION. Afirma el defecto tal como esta hoy, asi que
// cuando el HuberFilter se arregle este test DEBE dar rojo. Ese rojo es la senal
// de borrarlo y cerrar el hallazgo, no un test que se rompio.
func TestE2E_00_ElGatewayRechazaTodoPrimerPaquete(t *testing.T) {
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
	acks, rechazos, cerradas, desacuerdos := 0, 0, 0, 0

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

		dtMin := dtMinimoPredicho(caso.bytes)
		predicho := resRechazo
		if caso.espera.Seconds() > dtMin {
			predicho = resAck
		}
		marca := "  "
		if got != predicho {
			marca = "<< mi prediccion FALLO"
			desacuerdos++
		}

		filas = append(filas, fmt.Sprintf(
			"  payload=%-5d B  espera=%-7v  dt_min que yo predije=%7.3f s  ->  MEDIDO=%-8s predije=%-8s %s",
			caso.bytes, caso.espera, dtMin, got, predicho, marca))
	}

	t.Logf("E0 BARRIDO CONTRA EL GATEWAY REAL (read deadline del gateway: %v)\n%s\n"+
		"  ACK=%d  RECHAZO=%d  CERRADA=%d  de %d casos\n"+
		"  casos en los que mi prediccion aritmetica FALLO: %d",
		defaultReadDeadline, unirLineas(filas), acks, rechazos, cerradas, len(casos),
		desacuerdos)

	// --- EL HALLAZGO ---
	if acks > 0 {
		t.Errorf("HALLAZGO CERRADO: el gateway acepto %d de %d primeros paquetes. Si el "+
			"HuberFilter se arreglo, actualizar el contexto vivo, cerrar el hallazgo y "+
			"borrar este test de caracterizacion", acks, len(casos))
		return
	}

	t.Logf("E0 HALLAZGO MEDIDO: %d de %d combinaciones de payload y espera, y NINGUNA "+
		"logro que el gateway acepte un primer paquete. El gateway rechaza el primer "+
		"paquete de TODA conexion.", len(casos), len(casos))

	// --- LA CAUSA, y por que mi prediccion era falsa ---
	if desacuerdos > 0 {
		t.Logf("E0 MI PREDICCION QUEDA REFUTADA en %d casos, y la medicion gana. Yo "+
			"habia despejado dt_min = v/umbral del HuberFilter y supuse que el cliente "+
			"podia satisfacerlo pausando antes de mandar. Falso: en handleConn el "+
			"`now := time.Now()` se captura ANTES del io.ReadFull que espera el paquete, "+
			"asi que `dt = now - lastSeen` mide el tiempo entre dos INICIOS de iteracion "+
			"del loop del servidor, no entre paquetes. En la primera iteracion son "+
			"microsegundos con el ReadFull todavia por delante. La pausa del cliente no "+
			"entra en dt: NO HAY NADA que el cliente pueda hacer.", desacuerdos)
	} else {
		t.Logf("E0 mi prediccion coincidio en los %d casos. Ojo: coincidir no la valida, "+
			"porque todos los casos dieron el mismo resultado.", len(casos))
	}

	t.Logf("E0 CONSECUENCIA: como el rechazo hace TriggerBlock + writeReject + return, " +
		"ninguna conexion pasa nunca de UN paquete, y ese paquete siempre se rechaza. " +
		"HiperSec no puede procesar un solo paquete de ningun agente. Nadie lo sabia " +
		"porque hasta este commit el repo no tenia cliente.")

	t.Logf("E0 NO MEDIDO: cual es el fix. Reemplazar Huber por Page-Hinkley (D-48), " +
		"inicializar lastV con la primera muestra, o mover el `now` despues del " +
		"ReadFull son tres arreglos distintos con consecuencias distintas, y elegir " +
		"es diseno del producto.")
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
