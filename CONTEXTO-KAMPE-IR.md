# CONTEXTO-KAMPE-IR.md

Contexto vivo de KAMPE IR. **Se lee antes de responder cualquier cosa sobre este
proyecto.** Responder de memoria en lugar de abrir este archivo es un defecto.
Este archivo es estado y **se sobreescribe**; la acumulacion va en `respuestas/`.

Ultima actualizacion: 2026-09-07 (E-008, auditoria de la auditoria).

---

## 1. Que es KAMPE IR

**Ecosistema matriz de contencion perimetral para agentes autonomos.** Deja
correr agentes sin soltarles la punta: los identifica por certificado, los mide
con estadistica en vez de reglas, los tensa cuando se desvian, los afloja cuando
se portan bien, y notaria cada tiron con firma.

**Kampe** = la carcelera de Tartaro (Apolodoro 1.2.1). Existe en las fuentes
unicamente como esa funcion.

**IR = evidencia y triage de incidentes**, decidido en ADR-001. No es Incident
Response completo: no hay case management con SLAs, ni forense, ni notificacion
regulatoria. Nombrarlo de mas es prometer un modulo inexistente.

Nombre anterior: CORREAI, bautizado y descartado el mismo dia.

## 2. Los subsistemas

| Subsistema | Rol oficial | Estado |
| --- | --- | --- |
| **HiperSec** (`gateway/`, Go) | Gateway de Contencion Perimetral | **compila y `go vet` sale limpio** en VM limpia de Actions; cero tests |
| **DualBrain** (`dualbrain/engine.hpp`, C++) | Motor Geometrico Embebido Zero-Heap | compila en g++ 12.2 y 13.3; **la varianza colapsa a 0 y kappa con ella** |
| **Custos Legis** (`audit/`, Python + SQL) | Boveda Criptografica Legal y No-Repudio | HMAC correcto; canonicalizacion colisionable, sin cadena de hash, **cero productores** |
| **Fleet Manager** (`fleet/` + `mqtt/`, FastAPI) | Gestor de Flota OTA y Telemetria | no ejecutado; **su ACL de MQTT no concede nada a los nodos** |
| **Testis** (`gateway/verdict.go`, `gateway/recorder.go`, `ir/`) | Grabador de Veredictos y Caso por Agente | **especificado y con validador en verde; cero codigo Go** |

KAMPE IR es el ecosistema; **HiperSec es solo el gateway**.

**Ojo con el nombre DualBrain:** este es C++, 262 KiB, 1024x64. El DualBrain del
motor de MUDH es C99 y **704 bytes**. No son el mismo artefacto.

## 3. El agujero que Testis llena (medido, y revalidado por auditoria)

- El gateway tiene **9 caminos de rechazo y 7 de bloqueo**, y **cero llamadas de
  log**. El paquete `log` no esta ni importado.
- El gateway **no tiene driver de base de datos**.
- Custos Legis tiene **cero importadores** en codigo de produccion.
- `rejectBytes = []byte{0xFF}`: **un byte igual para los 9 rechazos**.

**El sistema aplica y no atestigua.**

## 4. Estado de Testis

**Verde y medido:** `verificacion/t_testis.py` corre en 0 con **11 controles
positivos**, canonico de **181 bytes de ancho fijo**, cadena de hash por agente,
y el **Sello** (checkpoint con raiz Merkle de los heads) firmado con ed25519.

**Cero codigo Go.** El validador demuestra que el diseno cierra; no demuestra
que el gateway lo haga.

D-20 (borrar la cola verifica en verde) esta **cerrado**. Cualquier auditoria
que lo cite como abierto esta en destiempo.

## 5. Lo medido el 2026-09-07 sobre los tres subsistemas restantes

Evidencia: `respuestas/2026-09-07-08-auditoria-de-la-auditoria.md` y
`verificacion/resultados-actions/`.

### Motor (`engine.hpp`, blob `53327c6a`)

- `sizeof(Engine_v4_3) = 268288`; datos utiles `266240` > `262144`. **No cabe en
  un L2 de 256 KiB.** La premisa de rendimiento del PDF esta refutada por el
  compilador.
- **La varianza colapsa a 0 antes de la iteracion 300** en dimensiones estables,
  porque `kMinSigmaSq = 0.0001` cuantiza a `int16 0` con `kSigmaScale = 1/256`.
- **El cero es estado absorbente:** escapar exige `sq >= 0.390625`, imposible en
  el regimen de norma unitaria. Irreversible, no transitorio.
- **`kappa` muere con la varianza** (`0.00000000` exacto), asi que la senal que
  alimenta `/telemetry/kappa` -> `kappa_drift` -> OTA queda muda.
- La amplificacion de la distancia es **39,1x**, no una explosion.
- `process_token` **no normaliza el token**: norma N da `dist=0.000000` y norma
  2N da `dist=1.000000` con el mismo baseline.
- El comentario que promete **spinlock es falso**: cero primitivas en el archivo.
- Falta `<algorithm>` pero **compila igual** en g++ 12.2 y 13.3. Riesgo latente,
  no defecto activo.

### ACL de MQTT (`mqtt/acl.conf`, sha256 `e8370c8d`)

- **El broker arranca sin error.** No hay error de sintaxis.
- **`user %u` no sustituye nada:** crea un usuario llamado literalmente `%u`.
- Consecuencia medida con `Denied PUBLISH ... rc135` y entrega vacia: **ningun
  nodo puede publicar telemetria ni recibir su OTA**. El fallo es
  **fail-closed**, bug de disponibilidad total, no fuga.
- El bloque `user anonymous` concede `read #`/`write #`, pero es **inalcanzable**
  via el `mqtt_username` de Fleet (siempre contiene `_`). Guard muerto.

### Gateway (Go)

- `go build ./...` = 0, `go vet ./...` = 0 en `ubuntu-latest` con go 1.22.12.
- `gofmt -l` marca **los tres archivos**, y sale 0 mientras los lista.
- El orden replay-antes-de-HMAC existe, pero exige el **certificado del agente**:
  es auto-DoS, no DoS de terceros.
- El HMAC usa **una clave de todo el gateway**, asi que no ata el paquete a un
  agente mas de lo que ya lo hace mTLS.

## 6. Donde corre

| Maquina | Para que sirve | Estado para KAMPE IR |
| --- | --- | --- |
| brain-env | taller persistente | descarga de blobs y sondeos de linea; **sin g++ ni go** |
| Actions x64 | fabrica, gratis en repo publico | **el testigo bueno**: go, g++ 13.3, mosquitto. Los workflows commitean su resultado |
| Actions arm64 | fabrica nativa arm64 | candidata: el motor es para el extremo |
| Kaggle GPU | GPU | no aplica (el motor no entrena) |

El inventario **se re-mide, no se recuerda**:
https://github.com/gatehot59-star/mudh-mobile/blob/main/00-ENTORNOS-Y-CAPACIDADES.md

## 7. Decisiones tomadas

| # | Decision | Fecha | Evidencia |
| --- | --- | --- | --- |
| D-01 | Repo publico | 2026-09-07 | commit 1. **Sin confirmar por Abraham** |
| D-02 | Rama por defecto `main` | 2026-09-07 | commit 1 |
| D-03 | Los 9 archivos del adjunto entran tal cual, blob SHA 9/9 | 2026-09-07 | 9 commits `codigo:` |
| D-04 | KAMPE IR es el ecosistema; HiperSec es el gateway | 2026-09-07 | E-003 |
| D-05 | Los verificadores separan INVARIANTE, CONTROL POSITIVO y DEFECTO | 2026-09-07 | `verificacion/` |
| D-06 | Bitacora append-only | 2026-09-07 | E-003 |
| D-07 | `go.mod` apunta a `kampe-ir` aunque el slug siga siendo `correai` | 2026-09-07 | **MEDIDO: no rompe. `go build` y `go vet` en 0** |
| D-08 | El modulo IR es Testis. No SOAR, no forense, no threat intel | 2026-09-07 | ADR-001 |
| D-09 | Testis serializa con ancho fijo: 181 B por veredicto, verificado | 2026-09-07 | `t_testis.py` |
| D-10 | El grabador nunca bloquea la aplicacion, y el descarte queda declarado | 2026-09-07 | E-005 |
| D-11 | El Sello **fija un punto**, no compara heads: la cadena puede crecer | 2026-09-07 | E-005 |
| D-12 | Los Sellos se firman con ed25519, no HMAC | 2026-09-07 | E-005, CP-10 |
| D-13 | Todo workflow **commitea su propio resultado**: nadie puede leer el texto de un log de Actions | 2026-09-07 | E-008 |
| D-14 | Mahalanobis diagonal **no se adopta por defecto**: medido, no mejora recall | 2026-09-07 | D-30 + benchmark HNSW |

## 8. Orden de trabajo (bloqueantes primero, corregido por lo medido)

1. **`mqtt/acl.conf`**: hoy la flota entera esta muda. `pattern read fleet/%u/#`
   en vez de `user %u`, y borrar el bloque `user anonymous`. Es el unico fallo
   que rompe el producto **completo** y cuesta cuatro lineas.
2. **`engine.hpp` Q4.12 + normalizar el token + sacar la palabra spinlock.** Sin
   esto el motor se apaga solo y el OTA nunca se dispara.
3. **Formato `centroids.bin` v2** unico para Fleet y motor (D-42): un solo numero
   de tamano en los dos lados, con cabecera y hash.
4. **`download_ota`**: guardar `sha256(delivery_token)` y comparar con
   `compare_digest`, o sacar el header. Un header exigido y no verificado es peor
   que no tenerlo.
5. **Primer test Go con `-race`.** No existe ninguno, y `go vet` no puede ver ni
   el orden de D-46 ni el data race.
6. **D-01+D-02 del gateway** (orden HMAC/replay) y **D-29** (`Snapshot()` del FSM).
7. **Testis en Go**: `verdict.go` -> `recorder.go` -> los 9 `Emit` -> SQL -> `ir/case.py`.
8. D-06 (canonicalizacion colisionable), D-53 (cadena en `audit_logs`), el resto.

## 9. Lo que espera decision de Abraham

1. **Renombrar el slug del repo** de `correai` a `kampe-ir`. Solo lo puede hacer
   el: la API con la que trabajo no expone rename de repositorios.
2. **Publico o privado.**
3. **Cuanto vale T**, el intervalo de sellado (D-28).
4. **Si KAMPE IR es proyecto o pieza** de MUDH / AURA / SIAO.
5. **Que L2 tiene el A53 objetivo:** 256 KiB obliga a bajar a 768 centroides;
   512 KiB no. Hoy el motor no cabe en 256 KiB.
6. **Si las dos ramas `titan/*` se mergean a `main`.** No lo hago yo.

## 10. NO MEDIDO abiertos

1. `go test -race`: **no existe un solo test Go**. El data race sigue sin medir.
2. `engine.hpp` con libc++ o MSVC (D-50 como riesgo real de portabilidad).
3. El costo del `fsync` por veredicto y donde el WAL se vuelve cuello de botella.
4. Nada corrio contra Postgres: `verdicts`, `anchors`, `cases`, `audit_logs`,
   `ota_packages` y `api_keys` no existen.
5. Si el Sello se puede publicar fuera del alcance del DBA en el deployment real.
6. Ninguna prueba de integracion: no existe cliente mTLS.
7. Q4.12 como reemplazo de Q8.8: la aritmetica cierra en papel, sin corrida.
8. **Marca:** no se consultaron USPTO, EUIPO ni INPI.
9. Si hay comprador. Cero llamadas reales.
