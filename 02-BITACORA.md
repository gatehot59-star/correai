# 02-BITACORA.md · CORREAI

Append-only. Entradas nuevas al final. Nada se reescribe: si algo estaba mal, se agrega la correccion con su fecha.
Una hipotesis muerta registrada vale mas que una hipotesis viva sin medir.

---

## 2026-09-07 · E-001 · Nacimiento del repo

**Quien pidio:** Abraham, por chat.
**Instruccion literal:** "NUEVO PROYECTO, GENERA SU REPO EN GIT 'CORREAI'".

**Que se hizo:**

- Se creo `gatehot59-star/correai`, publico, sin auto-init.
- Commit 1 `a72ffbd`: `README.md`.
- Commit 2: `CONTEXTO-CORREAI.md`, `02-BITACORA.md`, `respuestas/.gitkeep`, `.gitignore`.

**Que NO se hizo y por que:**

- Cero codigo, cero CI. No hay definicion del proyecto; inventarla seria simular rigor.

**Estados al cierre:**

| Afirmacion | Estado | Como se verifica |
| --- | --- | --- |
| El repo existe y es publico | **MEDIDO** | responde la API de GitHub con el repo creado; abrir la URL sin sesion |
| La rama `main` existe con 2 commits | **MEDIDO** | historial del repo |
| Que es CORREAI | **NO MEDIDO** | falta declaracion humana |
| Que este esqueleto sea el correcto para el proyecto | **NO MEDIDO** | depende de lo anterior |

**Proximo paso bloqueado por:** la definicion de CORREAI. Sin eso, cualquier estructura que agregue es ruido.

---

## 2026-09-07 · E-002 · El nombre, y 17 defectos sobre un documento que decia "Sin Bugs"

**Quien pidio:** Abraham. **Literal:** "AHI TIENES EL TRABAJO DEDUCE EL NOMBRE",
con `CORREAI.MD` adjunto: 2.100 lineas, 61.212 bytes, 9 archivos.

**La deduccion:** **CORREA + AI**. El sistema entero es una correa para agentes
autonomos: un FSM que se tensa (backoff x2 hasta 30 s) y se afloja (10 exitos
seguidos), un header llamado `SovereigntyHeader`, un motor de 262 KiB en el
extremo para seguir decidiendo sin nube, un log que notaria cada tiron y un ACL
que le da una correa por nodo. Segunda lectura consistente: correa de
transmision, la pieza disenada para cortarse primero. **Refutada** mi hipotesis
previa de "correo": no hay cola, buzon ni SMTP en ninguno de los 9 archivos.

**Que se midio (instrumento, no relato):**

| Medicion | Instrumento | Resultado |
| --- | --- | --- |
| Huella del motor | g++ 12.2.0 | 268.288 B = 262,00 KiB **exacto como declara** |
| Deteccion del motor | g++, 3 casos + control positivo | baseline de norma 10 marca anomalo su propio token: d=9 > umbral 3 |
| Anti-replay | gcc, 6 casos | `ts=2^64-1` da `int64(diff)` negativo y **pasa** la ventana |
| HMAC de auditoria | `hmac` de la stdlib | 200/200 identico, mas el pre-hash RFC 2104 de claves > 64 B |
| Canonicalizacion | stdlib | dos eventos distintos, **misma firma** |
| Token de OTA | AST + esquema | 5 apariciones, 0 en la base, 0 comparaciones |
| Bytes del OTA | aritmetica | 262.144 vs 266.240 vs 268.288: tres cosas llamadas "262" |
| Usuario MQTT | construccion | `('acme','node_1')` y `('acme_node','1')` dan el mismo `%u` |

**Dos FIX de la tabla de 14 que no son FIX:** el spinlock "agregado" a
`process_token` no existe (cero hits de `spinlock|atomic|mutex` en el header), y
la compatibilidad con campos futuros se arreglo en `decodeHeader` pero no en
`decodeContext` ni en `decodePayload`.

**Defecto propio del turno:** mi primer par de prueba para la colision de
canonicalizacion NO colisiono, y por un momento di la hipotesis por falsa. La
causa era mia: elegi una clave inyectada que ordena despues de la siguiente
clave real. El instrumento me falso, corregi el caso y quedo escrito dentro del
test.

**Integridad de la transcripcion:** los 9 archivos del adjunto se subieron uno
por uno y **cada blob SHA que devolvio GitHub se comparo contra el
`git hash-object` local**: 9 de 9 coincidieron. Esto importa porque el adjunto
llega al modelo con la indentacion alterada, y una transcripcion silenciosamente
corrupta habria envenenado todo lo demas.

**NO MEDIDO al cierre:** si el gateway Go compila (no hay Go en el sandbox; el
CI quedo commiteado y **su resultado no fue leido en este turno**), el data race
bajo `-race`, el YAML del workflow sin parser local, el SQL sin Postgres, cero
pruebas de integracion mTLS.

**Archivo de esta respuesta:** `respuestas/2026-09-07-01-deduccion-del-nombre.md`.
