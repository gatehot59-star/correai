# Primer test Go del repo: D-26 deja de ser NO MEDIDO

## Pedido

Escribir el primer test Go con `-race`.

## Herramientas y maquina

**Actions ubuntu-latest**, go 1.22.12, VM limpia. Un workflow nuevo que
commitea sus tres archivos de resultado.

**Por que ahi y no en el taller:** medido, `command -v go` devuelve AUSENTE en
`brain-env` y en el sandbox. Actions es la unica de las tres maquinas con
toolchain de Go, y ademas es el mejor testigo para W-01 porque arranca de una
imagen inmutable.

Rama `titan/test-go-race`, desde `main`. Nada mergeado. Cero runtime ajeno.

## El hallazgo, medido

**`go test -race`: 7 tests PASS, 0 FAIL, y 27 reportes del detector.** Sin
`-race`, 0 reportes y el control positivo cae. Commit `82221fbd`.

El detector cita **16 lineas distintas de `filters.go`**:

```
filters.go:66  69  76  79  80  81  84  88  89     <- HuberFilter.Update
filters.go:130 131 136 137 138 147 148            <- CoherenceFilter.Update
```

Y reparte los reportes asi: **28 en `(*HuberFilter).Update`** y **22 en
`(*CoherenceFilter).Update`**. Un fragmento crudo:

```
WARNING: DATA RACE
Read at 0x00c00015e0a0 by goroutine 8:
  gateway.(*CoherenceFilter).Update()
      /home/runner/work/correai/correai/gateway/filters.go:147 +0x3e4
Previous write at 0x00c00015e0a0 by goroutine 7:
  gateway.(*CoherenceFilter).Update()
      /home/runner/work/correai/correai/gateway/filters.go:148 +0x41c
```

El mecanismo es el que el contexto ya sospechaba y nadie habia medido:
`getOrCreateAgent` devuelve **el mismo `*AgentState`** a toda conexion que
presente el mismo certificado, `handleConn` llama `agent.Huber.Update` y
`agent.Coherence.Update` **fuera de `agent.mu`** (ese mutex solo envuelve el
chequeo de `lastTimestampNs`), y ninguno de los dos filtros tiene una sola
primitiva de sincronizacion.

Esto tambien confirma lo que dije del auditor: **`go vet ./...` sale en 0 y no
ve nada de esto.** Su silencio nunca fue una aprobacion.

## Los tres controles, y por que existen

| Control | Que descarta | Resultado |
| --- | --- | --- |
| `TestControlPositivo_DetectorArmado` | que el verde venga de correr `go test` a secas | PASS con `-race`, **FAIL sin** `-race` |
| `TestControlNegativo_ElFSMNoReportaCarrera` | que el detector reporte todo lo concurrente | PASS: el FSM, que si toma su mutex, no emite nada |
| el nonce en `TestD47_...` | que `verifyPacketHMAC` diga si a todo | PASS: cambiar el nonce invalida |

El primero es el que importa: **la suite no puede pasar en verde con el
detector apagado**, y el job lo prueba corriendo la misma suite sin el flag y
exigiendo `EXIT_test_sin_race=1`. Eso es control por mutacion del flag, no del
codigo.

## De paso, D-47 medido en Go sobre la funcion real

Llamando `g.verifyPacketHMAC` (no `hmac.Equal` por separado, que seria el
sujeto equivocado):

```
D-47 MEDIDO: Context[0] paso de 0.5 a 999999 y la firma sigue valida
D-47 MEDIDO: el ciphertext se reemplazo entero y la firma sigue valida
```

## Por que los reproductores corren en un subproceso

Si la carrera se disparara dentro del proceso de test, el detector marcaria el
test como fallido y el binario terminaria en 66. El hallazgo ("la carrera
existe") quedaria disfrazado de "la suite esta roja", que es lo que hace que
nadie lea el resultado. Corriendo en un subproceso del **mismo binario
instrumentado**, la carrera se mide, su reporte crudo queda en la salida, y el
veredicto es una afirmacion verificable.

## Dos defectos propios, los dos encontrados CORRIENDO

**1. Mi guard paso leyendo mi propio comentario.** El chequeo "el detector
reporto al menos una carrera" buscaba `DATA RACE`, y en la primera corrida dio
`conteo_DATA_RACE=1` **contando un `echo` mio** que mencionaba la cadena. Cero
reportes reales y el guard en verde. Es el patron de la constante que nadie
consulta en su version peor: un guard que pasa por una razon que no tiene nada
que ver con lo que mide. Corregido: se busca `WARNING: DATA RACE`, la marca
completa del runtime, y solo dentro de la salida de `go test`.

**2. El reporte crudo no quedaba en ninguna parte.** Los tests afirmaban "D-26
MEDIDO" y la salida del subproceso solo se imprimia en el camino de fallo. Con
todo en verde quedaba un veredicto sin su medicion: el testigo unico que W-01
prohibe. Corregido: cada test loguea **siempre** la salida cruda y cuenta los
reportes. Los 27 de esta corrida vienen de ahi.

**3. Un rojo falso, menor:** el chequeo "ningun test en FAIL" grepeaba el
archivo entero, que incluia el FAIL deliberado del control sin `-race`. Ahora
cada seccion escribe su propio archivo y cada guard mira el suyo.

Los tres son la misma leccion: **el instrumento tambien hay que medirlo**, y
los tres aparecieron en la corrida, no releyendo el codigo.

## Los dos tests de caracterizacion, declarados

`TestD26_...` y `TestD47_...` afirman los defectos **tal como estan hoy**.
Cuando se arreglen, van a dar rojo. Ese rojo es el recordatorio de borrarlos y
cerrar el hallazgo en el contexto vivo, no un test que se rompio. Esta escrito
en el comentario de cada uno para que el que los toque no dude.

## Evidencia cruda

- `gateway/race_test.go` (sha256 `b1c84bf5...`, ya pasado por `gofmt`)
- `verificacion/resultados-actions/go-test-race.txt`: formato, build, vet y el veredicto 13/13
- `verificacion/resultados-actions/go-test-conrace.txt`: **851 lineas**, la suite con `-race` y los 27 reportes completos (sha256 `141fbc1b...`)
- `verificacion/resultados-actions/go-test-sinrace.txt`: el control por mutacion del flag
- `.github/workflows/go-test-race.yml`

`gofmt -l gateway/` sigue marcando `filters.go`, `fsm.go` y `gateway.go`: son
preexistentes y no los toque en este turno.

## Estado que cambia en el contexto vivo

El **NO MEDIDO numero 1** de `CONTEXTO-KAMPE-IR.md` ("no existe un solo test
Go; el data race sigue sin medir") queda cerrado: existe la suite y la carrera
esta medida. **El diff del contexto no va en esta rama** porque su version
actualizada vive en `titan/auditoria-de-la-auditoria`, y pisarla desde aca
crearia dos verdades. Se aplica al mergear, y queda declarado aca para que no
se pierda.

## NO MEDIDO

1. **El fix de D-26 no se escribio.** Este turno mide, no corrige. La carrera
   sigue viva en `main`.
2. **Nada probo el gateway punta a punta:** no hay cliente mTLS, asi que el
   reproductor golpea el `AgentState` compartido directamente en vez de abrir
   dos conexiones TLS reales. El sujeto es el correcto (los mismos metodos,
   sobre el mismo puntero, desde donde `handleConn` los llama), pero el camino
   completo sigue sin medirse.
3. **El data race del gateway bajo carga real:** 4 goroutines y 500 iteraciones
   no son trafico de produccion.
4. **D-46 sigue sin test.** El orden replay/HMAC necesita un cliente que hable
   el protocolo.
5. `go test -race` en arm64: NO MEDIDO. Corrio solo en x64.
6. Cobertura: no se midio `-cover`. La suite toca 3 de los 4 archivos del
   paquete y ninguna linea de `decodePerimeterPacket`.

--- METODO TITAN ---
Accion delicada: SI (workflow nuevo con contents: write, en rama, no en main)
Modo aplicado:   TITAN FULL
Rubrica:         90/95 -> 94,7/100
N/A declarados:  5 (DevOps: la entrega es un instrumento de test, sin deployment)
Review externo:  pedido a Copilot en el PR; sin hallazgos = NO MEDIDO, no aprobacion
Instrumento:     go 1.22.12 + detector de carreras del runtime en Actions
                 ubuntu-latest; 27 reportes crudos commiteados verbatim;
                 control por mutacion del flag con 0 reportes sin -race
Maquina:         Actions x64 (go AUSENTE en brain-env y en el sandbox, medido)
Artefactos:      gateway/race_test.go + .github/workflows/go-test-race.yml +
                 3 archivos en verificacion/resultados-actions/ + este archivo +
                 Doc de ClickUp
