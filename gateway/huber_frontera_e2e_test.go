// Copyright (c) 2026 Jorge Abraham Mendieta.
// Computational Substrate Theory. Todos los derechos reservados.

// LA FRONTERA DEL HuberFilter, medida contra el gateway real.
//
// COMO APARECIO ESTE ARCHIVO: el control del arnes del cliente mTLS (un paquete
// perfectamente formado y bien firmado) fue RECHAZADO. La causa no era el
// codificador: es que el gateway rechaza el primer paquete de todo agente.
//
// EL MECANISMO. En `HuberFilter.Update`, sobre un filtro recien creado:
//
//	lastV = 0  ->  derivative = (v - 0)/dt = v/dt
//	threshold  = 0.05 * sqrt(v) * (1 + log1p(s))
//
// El `dt` que recibe es el tiempo entre el fin del handshake y la llegada del
// primer paquete. Un cliente que escribe apenas conecta da dt de microsegundos, y
// ahi v/dt queda cuatro ordenes de magnitud sobre el umbral.
//
// Es D-48 (derivada en var/s comparada contra un umbral en unidades de desvio) en
// su forma mas concreta. El hallazgo original decia "bloquea casi cualquier
// rafaga". La medicion dice algo mas fuerte: bloquea el PRIMER paquete, siempre.
//
// LA PREDICCION, despejada del propio filtro:
//
//	v/dt < 0.05*sqrt(v)*inertia   =>   dt > sqrt(v)/(0.05*inertia)
//
// con v = alpha*(x - alpha*x)^2 = 0.128*x^2 y x = payload_bytes/1024. O sea
// dt_min crece LINEAL con el tamano del payload: ~7,2 s por cada KB.
//
// ESTE TEST NO CONFIA EN ESA ARITMETICA. Barre el espacio (payload x espera)
// contra el gateway real y pone la prediccion al lado de cada medicion. Si no
// coinciden, mi calculo estaba mal y queda escrito en la salida.
//
// Y LA CONSECUENCIA QUE LO CONVIERTE EN BLOQUEANTE DE PRODUCTO: el propio gateway
// pone un `defaultReadDeadline` de 2 s. Si el dt que el filtro exige supera esos
// 2 s, no hay espera posible: esperar mas hace que el gateway cierre la conexion,
// y esperar menos hace que el filtro bloquee al agente. La prediccion dice que eso
// pasa a partir de ~303 bytes de payload.

package gateway

import (
	"fmt"
	"math"
	"testing"
	"time"
)

// dtMinimoPredicho despeja del HuberFilter el `dt` minimo que NO dispara el
// bloqueo en el primer paquete, para un payload de `bytes` bytes.
//
// Usa las constantes del repo (DefaultAlpha, DefaultSigma, DefaultEpsilon), no
// numeros copiados: si alguien cambia el alpha, esta prediccion cambia con el.
func dtMinimoPredicho(bytes int) float64 {
	x := float64(bytes) / 1024.0

	// Un paso de Update sobre un filtro en cero.
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
	// derivative = v/dt  <  umbral   =>   dt > v/umbral
	return v / umbral
}

// TestE2E_00_LaFronteraDelHuberFilter barre payload x espera contra el gateway
// real y reporta la tabla completa, con la prediccion al lado.
func TestE2E_00_LaFronteraDelHuberFilter(t *testing.T) {
	addr, clave, p := arrancarGateway(t)

	tipo := struct{ ack, rechazo, cerrada string }{"ACK", "RECHAZO", "CERRADA"}

	// medir abre una conexion nueva con un cert nuevo, espera `espera`, y manda UN
	// paquete valido con `bytes` de payload.
	//
	// Un cert por caso es obligatorio: un rechazo dispara TriggerBlock, y con el
	// cert compartido el caso siguiente mediria el backoff en vez del filtro.
	medir := func(nombre string, bytes int, espera time.Duration) string {
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
			return tipo.cerrada
		case resp == respAck:
			return tipo.ack
		default:
			return tipo.rechazo
		}
	}

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
	resultados := make(map[string]string)
	desacuerdos := 0

	for i, caso := range casos {
		nombre := fmt.Sprintf("agente-e0-%d-%dB-%dms", i, caso.bytes, caso.espera.Milliseconds())
		got := medir(nombre, caso.bytes, caso.espera)

		dtMin := dtMinimoPredicho(caso.bytes)
		predicho := tipo.rechazo
		if caso.espera.Seconds() > dtMin {
			predicho = tipo.ack
		}

		marca := "  "
		if got != predicho {
			marca = "<<"
			desacuerdos++
		}

		filas = append(filas, fmt.Sprintf(
			"  payload=%-5d B  espera=%-7v  dt_min predicho=%7.3f s  ->  medido=%-8s predicho=%-8s %s",
			caso.bytes, caso.espera, dtMin, got, predicho, marca))

		resultados[fmt.Sprintf("%d/%d", caso.bytes, caso.espera.Milliseconds())] = got
	}

	t.Logf("E0 BARRIDO DEL HuberFilter EN EL PRIMER PAQUETE\n"+
		"  (read deadline del gateway: %v)\n%s\n"+
		"  desacuerdos entre medicion y prediccion: %d",
		defaultReadDeadline, unirLineas(filas), desacuerdos)

	// --- LOS TRES GUARDS QUE SON EL HALLAZGO ---

	// 1. Sin espera, el primer paquete de un agente NUEVO es rechazado. Con el
	//    payload mas chico posible del arnes.
	sinEspera := resultados[fmt.Sprintf("%d/0", cargaChica)]
	if sinEspera == tipo.ack {
		t.Errorf("D-48 parece cerrado: un primer paquete de %d B sin espera fue "+
			"ACEPTADO. Si el HuberFilter se arreglo, actualizar el contexto vivo y "+
			"borrar este test", cargaChica)
	} else {
		t.Logf("E0 HALLAZGO 1 MEDIDO: el primer paquete de un agente nuevo, con solo "+
			"%d bytes de payload y sin pausa, es %s. El gateway no puede atender a un "+
			"agente que habla apenas conecta.", cargaChica, sinEspera)
	}

	// 2. Con espera suficiente SI pasa. Este es ademas el control del codificador:
	//    si nunca hubiera un ACK, todos los rechazos serian ambiguos.
	conEspera := resultados[fmt.Sprintf("%d/300", cargaChica)]
	if conEspera != tipo.ack {
		t.Fatalf("CONTROL ROTO: con %d B y 300 ms de espera (dt_min predicho %.3f s) "+
			"el gateway respondio %s. Sin un ACK en algun punto del barrido, este "+
			"archivo no distingue 'el filtro bloquea' de 'el codificador esta mal'",
			cargaChica, dtMinimoPredicho(cargaChica), conEspera)
	}
	t.Logf("E0 CONTROL OK: con %d B y 300 ms de pausa el gateway responde ACK, asi "+
		"que el codificador coincide con decodePerimeterPacket y los rechazos de "+
		"este barrido son del filtro, no del formato.", cargaChica)

	// 3. LA FRONTERA: hay payloads para los que NINGUNA espera sirve, porque el dt
	//    que el filtro exige supera el read deadline del propio gateway.
	dt512 := dtMinimoPredicho(512)
	res512 := resultados["512/1900"]
	if dt512 <= defaultReadDeadline.Seconds() {
		t.Logf("E0 NO MEDIDO: con 512 B el dt exigido (%.3f s) no supera el read "+
			"deadline (%v), asi que este caso no demuestra imposibilidad. La frontera "+
			"esta en otro tamano.", dt512, defaultReadDeadline)
	} else if res512 == tipo.ack {
		t.Errorf("la prediccion falla: con 512 B el dt exigido es %.3f s, mas que el "+
			"read deadline de %v, y sin embargo el gateway ACEPTO. Mi despeje del "+
			"filtro esta mal", dt512, defaultReadDeadline)
	} else {
		t.Logf("E0 HALLAZGO 2 MEDIDO: con 512 B de payload el filtro exige %.3f s de "+
			"pausa, pero el gateway cierra la conexion a los %v. Esperar menos hace que "+
			"el filtro bloquee (%s medido a 1900 ms) y esperar mas hace que el gateway "+
			"corte. NO HAY ESPERA POSIBLE: el primer paquete de 512 B no puede ser "+
			"aceptado nunca.", dt512, defaultReadDeadline, res512)
	}

	// La frontera exacta, calculada con las constantes del repo.
	frontera := 0
	for b := 1; b <= 4096; b++ {
		if dtMinimoPredicho(b) > defaultReadDeadline.Seconds() {
			frontera = b
			break
		}
	}
	if frontera > 0 {
		t.Logf("E0 FRONTERA PREDICHA: a partir de %d bytes de payload, el dt que el "+
			"HuberFilter exige en el primer paquete supera el read deadline de %v. "+
			"Todo agente cuyo primer mensaje pese %d bytes o mas queda bloqueado "+
			"pase lo que pase. (Predicho de las constantes del repo, no medido byte "+
			"por byte: lo medido son los 11 casos de la tabla.)", frontera,
			defaultReadDeadline, frontera)
	}

	if desacuerdos > 0 {
		t.Logf("E0 OJO: %d de %d casos no coincidieron con la prediccion. La medicion "+
			"manda sobre mi aritmetica; los casos marcados con << en la tabla son los "+
			"que hay que mirar.", desacuerdos, len(casos))
	}
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
