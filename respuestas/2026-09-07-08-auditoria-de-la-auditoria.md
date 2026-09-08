# Auditoria de la auditoria: D-40 a D-54 contra el repo real

## Pedido

El auditor externo esta en destiempo: afirma que no existe trabajo que si
existe, y pide tres comandos como "lo que falta". Auditar su auditoria y
quedarse con lo relevante.

## Herramientas y maquinas

Tres instrumentos, dos maquinas:

1. **Actions ubuntu-latest** (VM limpia, instrumento ajeno): `go build`,
   `go vet`, `gofmt`, `g++ 13.3.0`, `mosquitto 2.0.18`. Dos workflows nuevos
   que **commitean su propio resultado** porque ningun agente puede leer el
   texto de un log de Actions.
2. **Sandbox con g++ 12.2.0**: compilo y corrio `t_engine_audit.cpp`.
3. **brain-env**: descargo los blobs y corrio los sondeos de linea.

Se escribio en la rama `titan/auditoria-de-la-auditoria`. Nada se mergeo a
`main`. Cero runtime ajeno.

## Lo primero: el auditor tiene razon en 12 de 15

Confirmados **con evidencia**, no con relectura:

| ID | Estado | Con que se midio |
| --- | --- | --- |
| D-40 | CONFIRMADO | `grep -nE 'spinlock\|atomic\|mutex\|test_and_set'` devuelve **una sola linea: el comentario**. Cero primitivas |
| D-41 | CONFIRMADO Y PEOR | reproducido corriendo el motor: sigma llega a 0 antes de la iteracion 300 |
| D-42 | CONFIRMADO | `fleet_manager.py:37` -> `CENTROIDS_EXPECTED_BYTES = 1024*64*4` = 262.144 B float32 solo centroides; el compilador dice que el motor son 266.240 B int16 + sigma |
| D-43 | CONFIRMADO | `download_ota` lineas 447-478: exige `X-Delivery-Token`, consulta por `tenant_id`+`package_id` y **nunca lo compara**. `compare_digest` no aparece en el archivo |
| D-45 | CONFIRMADO | `fleet_manager.py:262` -> `mqtt_username = f"{tenant_id}_{payload.node_id}"`; `node_id` solo valida largo 1..128, sin charset |
| D-46 | CONFIRMADO, con alcance corregido | en `gateway.go` la marca de agua se escribe **antes** de `verifyPacketHMAC`. Ver abajo por que la severidad baja |
| D-47 | CONFIRMADO, y hay algo mas grave | el HMAC cubre 48 bytes de cabecera; `Context` y `Ciphertext` quedan afuera |
| D-48 | CONFIRMADO | `filters.go`: `derivative = (v-lastV)/dt` en var/s contra `threshold = 0.05*sqrt(v)*inertia` en desvio |
| D-49 | CONFIRMADO MEDIDO | mismo baseline, token de norma N -> `dist=0.000000`; norma 2N -> `dist=1.000000` |
| D-51 | CONFIRMADO MEDIDO | el compilador: `sizeof=268288`, datos `266240` > `262144` de un L2 de 256 KiB. **No cabe** |
| D-52 | CONFIRMADO | `custos_legis.py:62-63` guarda `key^0x36` y `key^0x5C`; la linea 50 dice "Las claves en claro nunca se almacenan" |
| D-53 | CONFIRMADO | `audit_logs` no tiene `prev_hash`. Matiz: el `previous` que hay en `custos_legis.py` es **rotacion de clave**, no cadena |
| D-54 | CONFIRMADO | `fleet_manager.py:327` -> `f'"node":"{payload.node_id}"}}'`, sin `json.dumps` |

## Lo segundo: tres cosas que el auditor afirma y la medicion desmiente

### R-01 · "Ningun archivo de este .md ha sido compilado" -> FALSO desde hoy

En VM limpia de Actions, commit `14fd6879`:

```
go version go1.22.12 linux/amd64
=== go build ./... ===   EXIT_go_build=0
=== go vet ./... ===     EXIT_go_vet=0
=== g++ -std=c++17 -fsyntax-only dualbrain/engine.hpp ===  EXIT_gpp_syntax=0
```

Eso ademas cierra dos NO MEDIDO del contexto: el gateway **compila** y el
`go.mod` que apunta a `kampe-ir` (slug todavia `correai`) **no rompe nada**.

### R-02 · D-50 (falta `<algorithm>`) NO reproduce en dos toolchains

`g++ 12.2.0` (Debian) y `g++ 13.3.0` (Ubuntu): `-fsyntax-only` sale **0** en
los dos, con y sin `-include algorithm`. Es riesgo de portabilidad latente,
**no un defecto activo**. Con libc++ o MSVC: NO MEDIDO.

### R-03 · D-44 falla al REVES de lo que dice el auditor

El auditor: *"el broker o no arranca o abre todo"*. Medido con mosquitto
2.0.18 y el `acl.conf` del repo tal cual (sha256 `e8370c8d`):

- **Arranca sin error.** `Config loaded`, `mosquitto version 2.0.18 running`.
- Y **no abre todo: cierra todo.** Publicaciones, QoS 1, con el log del broker
  como testigo:

```
A: probe -> fleet/probe/x        Denied PUBLISH ... rc135   (el diseno lo QUIERE permitido)
B: probe -> cualquier/otro       Denied PUBLISH ... rc135
C: fleet_server -> fleet/x       PUBACK rc0     (control positivo: el instrumento puede dar verde)
D: usuario literal '%u' -> fleet/%u/x   PUBACK rc0
E: usuario 'anonymous' -> cualquier/cosa PUBACK rc0
F: probe suscrito a fleet/probe/#, fleet_server publica ahi -> recibido: []
G: a_b suscrito a fleet/a_b_c/#, fleet_server publica ahi   -> recibido: []
```

`user %u` **no sustituye nada**: crea un usuario llamado literalmente `%u`
(prueba D lo demuestra: ese usuario si tiene permisos). Consecuencia real:
ningun nodo puede publicar telemetria ni recibir su OTA. Es un bug de
**disponibilidad total**, no una fuga. La prioridad cambia y la severidad
tambien.

## Lo tercero: seis cosas que el auditor no vio

**N-01 · Kappa muere con la varianza, y kappa es la senal que alimenta el OTA.**
`update_kappa` promedia las varianzas almacenadas, asi que cuando colapsan
`kappa = 0.00000000` exacto. El motor deja de ser detector **antes** de
volverse maquina de falsos positivos, y `/telemetry/kappa` -> `kappa_drift`
-> OTA queda en silencio permanente. Medido en la tabla de checkpoints.

**N-02 · El cero no es un piso: es un estado ABSORBENTE.** Para que la EWMA
salga de 0 hace falta `kEwmaAlpha*sq >= kSigmaScale`, o sea `sq >= 0.390625`.
Medido barriendo amplitudes: 0.50 y 1.00 quedan en cero, 1.50 revive con 5.
En vectores de norma unitaria con componentes ~0.125, esa desviacion no
ocurre: **el colapso es irreversible**, no transitorio.

**N-03 · La amplificacion es 39,1x, no una explosion.** Una perturbacion de
0.01 aporta 1.0000 con la varianza colapsada contra 0.0256 con la minima
representable. Grave y acotado; decirlo como "explota" invita al fix
equivocado.

**N-04 · El bloque `user anonymous` es una concesion INALCANZABLE.** Concede
`read #`/`write #` a quien se autentique con ese nombre (prueba E), pero
`mqtt_username` siempre contiene `_`, y ni `anonymous` ni `%u` lo tienen. Es
un guard inalcanzable disfrazado de defensa: parece un riesgo y no lo es,
parece una denegacion y es lo contrario.

**N-05 · `gofmt -l` marca los tres archivos Go, y sale 0 mientras los lista.**
`gateway/filters.go`, `gateway/fsm.go`, `gateway/gateway.go`. El exit code 0
es la misma trampa que `grep -c` sin match: **cero no es no-medido**.

**N-06 · El HMAC del paquete usa UNA clave de todo el gateway.** Con
`GATEWAY_HMAC_KEY` compartida, cualquier agente puede firmar por su propio ID
lo que ya podia enviar, y TLS 1.3 con `RequireAndVerifyClientCert` ya cubre al
MITM. Extender el HMAC a `context` y `ciphertext` (D-47) esta bien, pero
**no compra autenticidad por agente** hasta que la clave sea por agente.

## Correccion de alcance en D-46

El camino: `deriveAgentID(cert)` -> `pkt.AgentID != agentID` -> rechazo. Asi
que para envenenar la marca de agua hay que presentar **el certificado de ese
agente**. No es "DoS del agente con un solo paquete basura" desde afuera: es
auto-DoS o DoS con credencial robada. El reordenamiento hay que hacerlo igual
(cuesta tres lineas), pero no es el bloqueante que la tabla sugiere.

## Y el destiempo que mas importa

1. **D-53 dice "el mismo agujero D-20 que tiene Testis".** Testis ya tiene
   cadena `prev_hash` por agente, **Sello** con raiz Merkle publicado fuera del
   alcance del DBA, y firma `ed25519`. El validador `verificacion/t_testis.py`
   corre con `fallos=0` y 11 controles positivos. D-20 esta cerrado desde el
   turno anterior.
2. **El punto 5 de su plan de hibridacion** ("veredicto -> Testis") esta
   especificado, no pendiente.
3. **Su D-31 ("medido, no estimado" para L2")** esta medido en este archivo.
4. **Y la apuesta matematica del PDF que el auditor propone conservar,
   Mahalanobis diagonal, ya se midio y no rindio:** en el banco de kNN exacto
   (D-30) y en HNSW real sobre Iris y Wine UCI, Mahalanobis diagonal empato o
   **bajo** el recall, subio el hubness de 0,715 a 1,236 y corto el QPS de
   5.176 a 377. Conservarlo como "lo que el PDF aporta" necesita un dato que
   todavia no existe.

## El instrumento que el auditor eligio no podia ver sus propios criticos

`go vet ./...` dio **limpio**. No encontro D-46 ni el data race porque `vet`
es analisis estatico de patrones, no un detector de carreras ni de orden de
operaciones de seguridad. K-02: un review sin hallazgos **no es aprobacion**.
El instrumento correcto es `go test -race`, y **no existe un solo test Go en
el repo**. Ese es el hueco, no `go vet`.

## Evidencia cruda

- `verificacion/t_engine_audit.cpp` (sha256 `25698cf8...`), corrido: exit 0.
- `verificacion/t_engine_audit.out`: salida verbatim.
- `verificacion/resultados-actions/tres-comandos.txt`: commit `14fd6879`, VM limpia.
- `verificacion/resultados-actions/acl-enforcement.txt`: **instrumento invalido**, ver abajo.
- `verificacion/resultados-actions/acl-enforcement-v2.txt`: commit `7d724ac9`, con log del broker.
- `engine.hpp` usado: git blob sha1 `53327c6aac3030da08a908f38f1318f6b12aa2f6`, identico al del repo.

## Defectos propios de este turno

1. **Mi primer control positivo compartia estado:** `static Engine_v4_3`
   dentro de la funcion y `run_amp` llamada **dos veces por linea**. Un control
   contaminado no es un control.
2. **Mi primer control no podia dar verde:** con amplitud 1.0 el ruido nunca
   superaba el umbral de escape, asi que "colapso" era el unico resultado
   posible. Lo detecte porque el control salio ROTO, no porque lo revisara.
3. **Mi primer instrumento de ACL no discriminaba:** en MQTT 3.1.1 mosquitto
   responde SUBACK 0 tanto si concede como si va a filtrar la entrega, asi que
   el caso y el control dieron lo mismo. `acl-enforcement.txt` queda commiteado
   como instrumento invalido, no como dato.
4. **Agregue un usuario despues de arrancar el broker** y mosquitto no recarga
   `password_file`: el `CONNACK 5` de la prueba 3 v1 no probaba nada sobre el ACL.
5. **Mi primer grep de D-45 corto en 14 coincidencias** y la linea real era la
   262. Un renglon mas de paciencia y habria firmado un "no existe" falso.

## NO MEDIDO

1. `go test -race`: no hay tests Go. El data race de D-26/D-40 sigue sin medir.
2. `engine.hpp` con libc++ o MSVC (D-50 como riesgo real de portabilidad).
3. Nada corrio contra Postgres: `audit_logs`, `ota_packages` y `api_keys` no existen.
4. El fix de D-44 no se escribio: esto es auditoria, no correccion.
5. Q4.12 como reemplazo de Q8.8: la aritmetica cierra en papel, sin corrida.
6. Si el A53 objetivo tiene L2 de 256 o 512 KiB. Nadie midio el target real.
7. Cero llamadas reales de cliente. Sigue sin haber comprador.

--- METODO TITAN ---
Accion delicada: SI (dos workflows nuevos con `contents: write`, en rama, no en main)
Modo aplicado:   TITAN FULL
Rubrica:         88/95 -> 92,6/100
N/A declarados:  5 (DevOps: la entrega es una auditoria, no infraestructura)
Review externo:  ninguno pedido; deuda declarada
Instrumento:     Actions VM limpia (go 1.22.12, g++ 13.3.0, mosquitto 2.0.18) +
                 g++ 12.2.0 local; exit codes y logs verbatim commiteados
Maquina:         Actions x64 + sandbox g++ + brain-env
Artefactos:      este archivo + `verificacion/t_engine_audit.{cpp,out}` +
                 `verificacion/resultados-actions/*.txt` + Doc de ClickUp
