# 2026-09-07 · 01 · Deduccion del nombre CORREAI

## 1. Pedido

"AHI TIENES EL TRABAJO DEDUCE EL NOMBRE", con un adjunto de 2.100 lineas
(`CORREAI.MD`, 61.212 bytes) titulado *Sistema Completo Sin Bugs — HiperSec /
DualBrain / Custos Legis / Fleet Manager*, con 9 archivos y una tabla final de
14 bugs declarados como corregidos.

## 2. Herramientas declaradas

| Herramienta | Escribio | Cuota ajena |
| --- | --- | --- |
| sandbox local (g++ 12.2.0, gcc, CPython 3.12.13) | archivos temporales | no |
| GitHub API: create_or_update_file x9, push_files x2 | si, este repo | no |
| ClickUp: creacion de un Doc publico | si | no |
| Actions | commiteado, **no leido en este turno** | minutos del repo publico |

No hay toolchain de Go en el sandbox (`go: AUSENTE`), asi que **nada del
gateway se compilo en este turno**. Por eso el CI existe.

## 3. La deduccion

**CORREAI = CORREA + AI. La correa.**

No es "correo". Eso fue mi hipotesis de hace 20 minutos, cuando solo tenia el
nombre, y el codigo la refuta: no hay cola, ni buzon, ni SMTP, ni entrega de
mensajes en ninguno de los 9 archivos. Lo que hay es un perimetro.

La evidencia, archivo por archivo:

| Pieza | Que hace | Por que es una correa y no un firewall |
| --- | --- | --- |
| `gateway/fsm.go` | Activo/Bloqueado, backoff que **duplica** hasta 30 s y **se afloja** tras 10 paquetes limpios | un firewall no perdona. Una correa se tensa y se suelta |
| `gateway/proto/perimeter.proto` | el header se llama literal `SovereigntyHeader` | la pregunta del paquete es **quien tiene la punta de la correa** |
| `gateway/filters.go` | derivada de varianza + coherencia coseno | es la mano sintiendo el tiron, no una lista de reglas |
| `dualbrain/engine.hpp` | 262 KiB, sin heap, entra en L2 | la correa tiene cerebro **en el extremo**: sigue decidiendo con la nube caida |
| `audit/custos_legis.py` | cada evento firmado con HMAC | *guardian de la ley*: cada tiron queda notariado |
| `fleet/` + `mqtt/acl.conf` | un `%u` y un subarbol por nodo | muchas correas a la vez, una por agente |

Y del otro lado no hay un dispositivo: hay un **agente**. De ahi el AI.

Segunda lectura que tambien cierra y conviene conservar: *correa* es tambien la
**correa de transmision**, la pieza que transmite fuerza entre dos maquinas sin
acoplarlas rigido y que esta disenada para **cortarse primero** y salvar el
motor. Eso es exactamente un circuit breaker. Las dos lecturas describen el
mismo sistema, asi que el nombre no es una metafora colgada: es la funcion.

**Choque de nombres que ya quedo en el repo:** el `go_package` del proto y los
prefijos de error dicen `hipersec`, el modulo Go dice `correai`. Si CORREAI es
el nombre del producto, `hipersec` es a lo sumo el nombre del componente
gateway. Decidirlo ahora cuesta un rename; en seis meses cuesta una
documentacion mintiendo.

## 4. Lo que se midio, con numero e instrumento

### 4.1 El motor: la huella es exacta y la deteccion no es invariante de escala

Instrumento: `g++ -std=c++17 -O2 -Wall -Wextra`, ejecutado.

```
sizeof(Engine_v4_3) = 268288 bytes = 262.00 KiB

A) ||v||=1   winner=0 distance=0.000000 curvature=0.988281 anomaly=0
B) ||w||=10  winner=0 distance=9.000000 curvature=1.000000 anomaly=1   <-- token IDENTICO a su propio baseline
C) control positivo, z ortogonal a v: distance=1.422574 anomaly=0

  [ok ] INVARIANTE  huella estatica == 262 KiB (268288 B)
  [ok ] INVARIANTE  baseline de norma 1 reconoce su propio token (d < 1e-3)
  [ok ] INVARIANTE  control positivo: token distinto da mas distancia que el propio
  [presente] DEFECTO     D-03 process_token no normaliza: baseline de norma 10 marca ANOMALO su propio token (d=9 > umbral 3)
  [presente] DEFECTO     D-03b el umbral no es invariante de escala: d(identico,||10||) > d(ortogonal,||1||)

fallos=0
```

La huella declarada es cierta al byte. El defecto es que el FIX v4.3 normaliza
en `inject_baseline` y **no** en `process_token`: el mismo vector, cargado como
baseline y mandado como token, da distancia 9 y se marca anomalo, mientras un
vector ortogonal a otro baseline da 1,42 y pasa. Con un solo
`anomaly_threshold` global, la deteccion depende de la norma de la entrada.

### 4.2 El anti-replay: el int64 desborda

Instrumento: `gcc -O0 -Wall -Wextra`, reimplementacion literal de
`validateTimestamp`. Go define la conversion `uint64 -> int64` como truncamiento
a la misma representacion en dos complementos, identica a C99.

```
  legitimo now       ts= 1788784000000000000 int64(diff)=                    0 acepta=1 espera=1 ok
  legitimo +10s      ts= 1788784010000000000 int64(diff)=          10000000000 acepta=1 espera=1 ok
  viejo -60s         ts= 1788783940000000000 int64(diff)=          60000000000 acepta=0 espera=0 ok
  futuro +60s        ts= 1788784060000000000 int64(diff)=          60000000000 acepta=0 espera=0 ok
  ATAQUE ts=2^64-1   ts=18446744073709551615 int64(diff)= -1788784000000000001 acepta=1 espera=0 <<< ACEPTA LO QUE NO DEBE
  ATAQUE ts=2^63     ts= 9223372036854775808 int64(diff)=  7434588036854775808 acepta=0 espera=0 ok
```

La ventana de ±30 s funciona para timestamps normales. Con `ts = 2^64-1` la
resta da un `int64` **negativo** y `diff <= maxDriftNs` es verdadero: el
paquete pasa.

Y lo que lo vuelve grave es el **orden** en `handleConn`: `agent.lastTimestampNs
= pkt.Timestamp` se escribe **antes** de `verifyPacketHMAC`. Un paquete con HMAC
invalido ya dejo el contador en `2^64-1`, y desde ahi todo paquete legitimo de
ese agente cae en `pkt.Timestamp <= agent.lastTimestampNs` y se rechaza. En
`gateway.go` hay **cero** `delete(` sobre el mapa de agentes: el estado no se
evicta nunca, asi que el ladrillo es permanente.

### 4.3 El log de auditoria: HMAC correcto, canonicalizacion colisionable

Instrumento: `hmac`/`hashlib` de la stdlib contra el HMAC casero del modulo,
extraido por AST del archivo real.

```
   claves de 32 B: 200/200 coinciden
   clave de 200 B (> bloque): coincide = True
   control positivo (mensaje distinto NO coincide): True

   evento A = {'accion': 'pausar&agent=n1'}
   evento B = {'accion': 'pausar', 'agent': 'n1'}
   canonico A = 'accion=pausar&agent=n1'
   canonico B = 'accion=pausar&agent=n1'
   HMAC A = e06f3ffb33e3e5d003505153e6491b83...
   HMAC B = e06f3ffb33e3e5d003505153e6491b83...
   misma firma para eventos distintos: True
```

`_HmacKeyPreHashed` es HMAC-SHA256 de verdad: 200/200 contra la stdlib, incluido
el pre-hash de claves > 64 B del RFC 2104. Eso esta bien y es medido.

Lo que falla es lo de arriba: `canonicalize_event` arma `k=v` unido por `&` sin
escapar nada, asi que un valor de texto que contenga `&` y `=` produce el mismo
canonico que dos campos distintos, y **la misma firma**. En un log cuyo unico
valor es "no se puede alterar un registro sin que se note", eso es el producto.

**Defecto mio en este mismo turno:** mi primer par de prueba NO colisiono y
conclui por un segundo que la hipotesis era falsa. La causa era mia: elegi una
clave inyectada que ordena DESPUES de la siguiente clave real. El instrumento me
falso, corregi el caso y quedo anotado dentro del test.

### 4.4 El fleet: un token decorativo y tres numeros que dicen 262

```
== 1. el delivery_token que /ota/download exige ==
   delivery_token = secrets.token_hex(16)
   delivery_token: str    = Header(..., alias="X-Delivery-Token")
   apariciones en el codigo: 5
   columna en audit/schema.sql: False
   se compara alguna vez contra algo: False

== 2. cuantos bytes acepta el OTA vs cuantos tiene el motor ==
   CENTROIDS_EXPECTED_BYTES = 262144 (1024 * 64 * 4)
   engine.hpp centroids_ int16 : 131072
   engine.hpp + sigma_sq_      : 262144
   + kappa_ (los 3 buffers)    : 266240
   sizeof(Engine_v4_3) medido  : 268288

== 3. el usuario MQTT: tenant_id + '_' + node_id ==
   ('acme','node_1') -> 'acme_node_1'   vs   ('acme_node','1') -> 'acme_node_1'   COLISION
   ('a','b_c') -> 'a_b_c'   vs   ('a_b','c') -> 'a_b_c'   COLISION
   node_id se valida solo por longitud (sin charset): True
```

- El `X-Delivery-Token` es obligatorio en la firma del endpoint, se genera, se
  publica por MQTT, se devuelve al cliente y **no existe en el esquema ni se
  compara nunca**. Cualquier valor entra. La autorizacion real es solo
  `require_tenant`.
- `1024*64*4` esta justificado como float32, pero el motor guarda `int16_t`. El
  numero sale bien por casualidad porque coincide con centroides + sigma. Un
  volcado de los tres buffers (266.240 B) lo **rechaza** el OTA, y el `sizeof`
  de la clase (268.288 B) tampoco entra. Tres cantidades distintas se llaman
  "262" en este sistema.
- `tenant_id + "_" + node_id` no es inyectivo y el ACL de MQTT aisla justo por
  ese `%u`. El aislamiento depende de que a nadie le toque un guion bajo en el
  lugar equivocado.

## 5. Los 17 defectos, y las 2 mentiras de la tabla de FIX

El adjunto se titula "Sin Bugs" y trae 14 correcciones. Dos de esas 14 no son
tales:

| # | Lo que dice el FIX | Lo que dice el archivo |
| --- | --- | --- |
| FIX v4.3 | "process_token es ahora thread-safe mediante spinlock liviano" | **cero** `spinlock`, `atomic`, `mutex` o `__sync` en `engine.hpp`. El unico hit del grep es ese comentario |
| FIX 7 | "decodeHeader rechaza campos futuros — corregido" | corregido en `decodeHeader`. `decodeContext` exige `fieldNum == 1` y `decodePayload` cierra con `default: return false`. La compatibilidad futura quedo 1 de 3 |

Inventario completo:

| ID | Defecto | Estado |
| --- | --- | --- |
| D-01 | `int64(diff)` desborda: `ts=2^64-1` pasa la ventana anti-replay | **MEDIDO** (C) |
| D-02 | `lastTimestampNs` se escribe ANTES de verificar el HMAC; con D-01, un paquete sin firma valida deja al agente bloqueado para siempre (el mapa no se evicta) | leido |
| D-03 | `process_token` no normaliza: un baseline de norma 10 marca anomalo su propio token | **MEDIDO** (g++) |
| D-04 | el spinlock del FIX v4.3 no existe | **MEDIDO** (grep, 0 hits) |
| D-05 | `HuberFilter` y `CoherenceFilter` no tienen ningun candado y se llaman fuera de `agent.mu`: dos conexiones del mismo cert = data race | leido (grep sync=0) |
| D-06 | `canonicalize_event` colisiona: dos eventos distintos, misma firma | **MEDIDO** (stdlib) |
| D-07 | importar `custos_legis` ejecuta `_load_hmac_keys()` y explota sin env vars | **MEDIDO** (AST) |
| D-08 | `X-Delivery-Token` exigido, nunca persistido, nunca comparado | **MEDIDO** (AST + esquema) |
| D-09 | `1024*64*4` justificado como float32 sobre un motor int16; 262.144 vs 266.240 vs 268.288 | **MEDIDO** |
| D-10 | `tenant_'_'node` no inyectivo y el ACL aisla por ese `%u` | **MEDIDO** |
| D-11 | `node_id` solo validado por longitud: `/`, `#`, `+` entran al topico | **MEDIDO** |
| D-12 | `isinstance(conn, asyncpg.Connection)` rechaza las conexiones de pool (`PoolConnectionProxy`), que es como se usa el pool en FastAPI | leido |
| D-13 | el HMAC firma solo identidad: el vector de contexto y el ciphertext, que son los que mueven los dos filtros, van **sin firmar**. Y la clave es un unico secreto compartido por toda la flota | leido |
| D-14 | compatibilidad futura arreglada en 1 de los 3 decoders | leido |
| D-15 | un vector de contexto todo en cero da `coh = 0 < 0.75` y bloquea al agente en el primer paquete | leido |
| D-16 | el timestamp estrictamente creciente por agente impide que un agente tenga dos conexiones concurrentes sin auto-bloquearse | leido |
| D-17 | `net.Error.Temporary()` deprecado, `@app.on_event` deprecado, `import asyncio` sin usar en `custos_legis`, 3 structs sin alinear a gofmt | leido |

## 6. Archivos generados en este mismo commit

- `respuestas/2026-09-07-01-deduccion-del-nombre.md` (este)
- `CONTEXTO-CORREAI.md` (sobreescrito: el proyecto dejo de ser NO MEDIDO)
- `02-BITACORA.md` (entrada E-002)
- `README.md` (reescrito con lo que CORREAI es)

Commiteados antes, en el mismo turno, con el blob SHA de git verificado uno por
uno contra el hash local: los 9 archivos del adjunto, `go.mod`,
`.github/workflows/ci.yml` y los 4 verificadores de `verificacion/`.

## 7. NO MEDIDO

1. **Si el gateway compila.** No hay Go en el sandbox. `go build`, `go vet` y
   `go test -race` quedaron commiteados en el CI y **no lei el resultado**.
2. **D-05 con el detector de carreras real.** No hay tests en el paquete, asi
   que `-race` no va a disparar solo: hace falta un test que abra dos
   conexiones del mismo cert.
3. **El YAML del workflow no lo valido ningun parser local** (no hay PyYAML en
   el sandbox). Si esta mal indentado, no corre ningun job.
4. **D-12** sin `asyncpg` instalado.
5. **D-11 sobre el broker**: no hay mosquitto para ver que hace el ACL con un
   `%u` que contiene `#`.
6. **El SQL nunca toco un Postgres**: `uuid_generate_v7()` esta sin ejecutar.
7. **Cero pruebas de integracion**: no existe un cliente que complete un
   handshake mTLS contra el gateway. Todo lo de arriba es analisis de piezas.
8. **Si CORREAI es un proyecto o una pieza** de MUDH / AURA / SIAO.
9. **Si hay comprador.** El sistema tiene multi-tenant, API keys y OTA, o sea
   que esta escrito para vender. No hay una sola llamada real todavia.
