# 02-BITACORA.md · KAMPE IR

Append-only. Entradas nuevas al final. Nada se reescribe: si algo estaba mal, se agrega la correccion con su fecha.
Una hipotesis muerta registrada vale mas que una hipotesis viva sin medir.

El repo se llamo CORREAI entre E-001 y E-002. Esas entradas **no se editan**.

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

---

## 2026-09-07 · E-003 · Bautismo: KAMPE IR, y el nombre trae una contracara

**Quien pidio:** Abraham. **Literal:** "Bautizado. KAMPE IR" y despues "KAMPE IR
ES EL NUEVO NOMBRE DE COORREAI", con la jerarquia oficial de los cuatro
subsistemas y sus roles.

**Lo que resuelve:** el choque de nombres que quedo abierto en E-002. `go.mod`
decia `correai` y el proto decia `hipersec`. Ahora **KAMPE IR es el ecosistema y
HiperSec es el gateway**, asi que el `go_package = "hipersec/gateway/v1"` y los
prefijos `hipersec:` son correctos por primera vez: nombran el componente.

**Lo que se verifico del nombre (instrumento: fuentes primarias, no memoria):**
Kampe (Kάμπη) es la carcelera de Tartaro, designada por Kronos para custodiar a
los Ciclopes y Hecatonquiros. Apolodoro, *Biblioteca* 1.2.1: "mato a su
carcelera Kampe y solto sus cadenas". Confirmado en Nonno (*Dionisiaca* 18.237),
Diodoro 3.72 y el diccionario de Smith. **Existe en las fuentes unicamente como
esa funcion**: no tiene mitologia propia fuera de la custodia. Para un producto
de contencion, el nombre es exacto y no es decorativo.

**La contracara, que va escrita y no escondida:**

1. **Kampe pierde.** Zeus la mata para liberar a los prisioneros, y esa muerte
   es el evento que habilita la victoria olimpica. Es la guardiana cuyo fallo es
   el nudo de la trama.
2. **La designa Kronos**, el regimen que cae.
3. El nombre significa literalmente "torcida, curvada" (de *kampsos/kamptô*), y
   el sustantivo comun κάμπη es **"larva, oruga, gusano de seda"**. En una
   licencia B2B de alta seguridad, "la torcida" y "la oruga" son lecturas
   disponibles.
4. **"IR" ya tiene dueno semantico**: en el mercado es Incident Response, una
   categoria de producto. Tal como esta construido, KAMPE IR hace prevencion,
   contencion y no-repudio; **no hace** case management, forense ni triage. Lo
   unico IR-adyacente es la alerta de deriva kappa por MQTT. Y fuera de
   seguridad, "IR" es la abreviatura estandar de Investor Relations.

**El hallazgo util del mito, que va a favor de la arquitectura:** despues de
ganar, Zeus **no** repone una guardiana monstruosa unica: pone a los
Hecatonquiros, ex prisioneros convertidos en guardianes. Guardia distribuida con
interes alineado en lugar de un cuello de botella obediente. Eso describe mejor
el FSM por agente de HiperSec que un gateway como punto unico, y sugiere que la
ventaja arquitectonica del sistema esta en el `AgentState` por certificado, no
en el perimetro.

**Que se hizo:** `README.md` reescrito, `CONTEXTO-KAMPE-IR.md` creado,
`CONTEXTO-CORREAI.md` borrado, `go.mod` apuntado a `kampe-ir`, y los encabezados
de los 4 verificadores actualizados. **E-001 y E-002 no se tocaron:** la
bitacora es append-only y la deduccion de "correa" queda como historia.

**Lo que NO pude hacer:** renombrar el slug del repositorio. La API con la que
trabajo expone crear, leer, escribir y borrar archivos, y **no** expone rename
de repositorios. Lo hace Abraham en Settings → General → Repository name, y
GitHub redirige los links viejos.

**NO MEDIDO:** si el `go.mod` con un path que todavia no resuelve pasa el CI
(no hay Go en el sandbox); y la marca, porque una busqueda de texto sin colision
**no es** una busqueda de marca: no se consultaron USPTO, EUIPO ni INPI.

**Archivo de esta respuesta:** `respuestas/2026-09-07-02-bautismo-kampe-ir.md`.

---

## 2026-09-07 · E-004 · ADR-001: el modulo IR es Testis, y aparecieron D-18 y D-19

**Quien pidio:** Abraham. **Literal:** "PUES LE METAMOS UN PRODUCTO IR, SOLO
DIME DE QUE TIPO QUIRES Y YO TE LO DOY".

En E-003 le objete el "IR" del nombre porque prometia un modulo inexistente.
Respuesta correcta de su parte: construirlo. Asi que la objecion queda cerrada
por construccion, no por rebaja del nombre.

**La eleccion no se hizo por catalogo de mercado, se hizo por medicion del
agujero:**

| Medicion | Instrumento | Resultado |
| --- | --- | --- |
| llamadas de log en el paquete `gateway` | grep | **0**. El paquete `log` no esta ni importado |
| caminos de rechazo / de bloqueo en `handleConn` | grep | **9 rechazos, 7 bloqueos** |
| driver de base de datos en el gateway | grep | **ninguno**, cero escrituras |
| importadores de `custos_legis` en codigo de producto | grep | **0** |
| bytes distintos que ve un cliente rechazado | lectura | **1**: `0xFF` para los 9 casos |

**El sistema aplica y no atestigua.** La "Boveda Criptografica de No-Repudio"
esta bien construida y **no tiene un solo productor**. Ese es el hueco, y es el
unico que ningun competidor puede llenar por afuera, porque la evidencia solo
existe en el punto de aplicacion en el instante de la decision.

**Decision · Testis** (lat. *testigo*, para que rime con Custos Legis): grabador
de veredictos con **cadena de hash** + **caso por agente** construido sobre esa
cadena. Explicitamente **no**: SOAR, forense de memoria, threat intel, SIEM.
Razones por tipo en el ADR §7.

**Hallazgo nuevo · D-18:** `audit_logs` firma **cada fila por separado**. Eso
prueba que una fila no fue modificada y **no prueba nada sobre el conjunto**: se
puede **borrar** una fila entera o reordenar, y todas las firmas restantes
siguen verificando. Para un producto de no-repudio legal, el ataque obvio del
insider no es editar el registro incomodo: es hacerlo desaparecer. **Por eso la
cadena de hash no es un adorno de Testis: es Testis.**

**Hallazgo nuevo · D-19:** 2 de los 9 rechazos (FSM ya bloqueado, y tamano de
frame invalido) **no** escalan el circuit breaker. Spam de frames invalidos le
cuesta al atacante un handshake TLS y el FSM no lo acumula nunca. Menor, pero es
exactamente la clase de cosa que hoy es invisible por falta de grabador.

**Dos bloqueantes declarados:**

1. **D-01+D-02 antes de Testis.** Si un paquete con HMAC invalido ladrilla a un
   agente para siempre, el primer caso IR del primer cliente va a ser un bug
   nuestro, y el grabador lo va a documentar firmado y encadenado.
2. **D-06 antes del release.** No se puede vender no-repudio con
   `canonicalize_event` colisionable viva en el mismo producto.

**Detalle tecnico que evita un bug futuro:** `packet_ts` va `numeric(20)` y no
`bigint`, porque un `uint64` de ataque (`2^64-1`) no entra en un `bigint` de
Postgres. Con `bigint`, el veredicto que registra el ataque D-01 explotaria al
insertarse.

**Defecto propio del turno:** mi primer grep de importadores de la boveda dio
"2" y los dos hits eran el propio modulo y **mi verificador**. Instrumento
sucio; recontado a 0 y corregido en el ADR antes de sacar la conclusion.

**Que NO se hizo:** cero codigo. Abraham lo genera; esta entrega es la
especificacion contra la que se va a medir.

**NO MEDIDO:** el costo del `fsync` por veredicto y la tasa a la que el WAL se
vuelve cuello de botella (el muestreo de aceptados es hipotesis sin numero); si
el `AgentState` aguanta el estado de cadena sin cambiar la granularidad de
`agent.mu` (afirmado por lectura); si D-18 es explotable en el deployment real.

**Archivo de esta respuesta:** `respuestas/2026-09-07-03-spec-ir-testis.md`.

---

## 2026-09-07 · E-005 · La auditoria del ADR: 6 hallazgos aceptados, 1 refutado, 2 nuevos, y el validador en verde

**Quien pidio:** Abraham, con una auditoria del ADR-001 hecha contra la fuente y
un validador propio ya ejecutado. Pedido concreto: **corregir el test D y
agregar `Sello`** para dejar `verificacion/t_testis.py` listo para commitear.

**Entregado:** el archivo esta commiteado, corre en 0 y tiene **10 controles
positivos**, no 8. Dos salieron de hallazgos que aparecieron implementando el
Sello.

**Lo que la auditoria valido de E-004:** las 7 mediciones del ADR se
recontaron una por una contra `gateway.go` y dieron todas iguales (0 logs, 9
rechazos, 7 bloqueos, sin driver de DB, `0xFF` unico, D-19, D-01/D-02
reproducido aritmeticamente, `numeric(20)`).

**Aceptados sin discutir (7):**

| ID | Hallazgo | Como quedo |
| --- | --- | --- |
| D-20 | la cadena resiste borrado del medio y **no de la cola** | reproducido: borrar los ultimos 10 sin ancla da **VERDE**. Cerrado con el Sello (CP-5, CP-6) |
| D-21 | el descarte que consume `Seq` da **ROJO falso** | reproducido. Fix: el descarte no consume seq y el siguiente veredicto declara `DroppedSince` |
| D-22 | cerrar el caso con "10 aceptados" es incomputable con muestreo 1/K | aceptado. Se cierra por `Streak`, y se agrega `RuleDecayed = 10` |
| D-23 | `BackoffNs` "despues de aplicar" es falso en 5 de 9 sitios | verificado en la fuente: 5 con `writeReject` antes de `TriggerBlock`, 2 al revez, 2 sin bloqueo. Orden nuevo: `TriggerBlock` → `Emit` → `writeReject` |
| D-24 | el caso 1 es un firehose | aceptado. Coalescer con `Repeat uint32` por ventana |
| D-25 | HMAC simetrico no es no-repudio frente a terceros | reproducido: la cadena refabricada por el tenedor de la clave da **VERDE**. Fix: Sello con ed25519, control positivo ejecutado |
| D-26 | data race: `lastSeen` es local a la conexion | verificado: `lastSeen := time.Now()` esta dentro de `handleConn` y no aparece en `AgentState` |

Y se acepta la correccion metodologica: **el par del test D no colisionaba**, asi
que ese control no probaba nada. Corregido con `("a|b","c")` vs `("a","b|c")`,
que en naive dan los dos `a|b|c`.

**Refutado (1) · D-27.** El auditor declaro que no leyo `audit/`.
`audit/schema.sql` **define `uuid_generate_v7()` el mismo**, en plpgsql, linea 7.
No lo importa de ninguna extension: la unica dependencia real es `pgcrypto` por
`gen_random_bytes`, declarada en la linea 4. Verificado como invariante dentro
del test.

**Nuevos (2):**

- **D-28.** El Sello cierra D-20 para lo sellado, y **no cubre lo emitido
  despues del ultimo checkpoint**: con sello en seq 100 y cadena crecida a 120,
  truncar a 105 da **VERDE**. La ventana de exposicion es exactamente **T**, o
  sea que T no es tuning: es el parametro de riesgo que se le promete al
  comprador y tiene que estar en el contrato.
- **D-29.** Cierra el NO MEDIDO del auditor sobre `fsm.go`: los metodos publicos
  del `AgentFSM` son `Allow`, `TriggerBlock`, `RecordSuccess`, `DecayBackoff`, y
  ninguno devuelve nada; `backoff` y `successStreak` son privados sin getter.
  **Testis no puede llenar `BackoffNs` ni `Streak`** sin agregarle un
  `Snapshot()` al FSM.

**Correccion mia sobre el mecanismo del Sello:** el ADR y la auditoria decian
comparar `head != ancla`. Eso **da rojo en toda cadena viva**, porque cualquier
agente que recibio un veredicto nuevo despues del checkpoint tiene otro head. La
regla implementada es: el Sello fija un punto, la cadena puede crecer por
encima, y no puede encogerse por debajo ni cambiar lo sellado. Medido: cadena
crecida → VERDE; truncada por debajo → ROJO.

**Agujero mio que nadie habia marcado:** el CI de E-002 corria `compileall`
sobre `audit` y `fleet`, o sea que **los verificadores `t_custos.py` y
`t_fleet.py` no los ejecutaba nadie**. Arreglado: dos jobs nuevos,
`custos-legis` y `testis`.

**Control de mutacion sobre mi propio instrumento, con hallazgo:** desactive el
chequeo de contiguidad de `seq` y volvi a correr. `fallos=1`, `rc=1`: el sabotaje
se detecta. **Pero CP-1 y CP-3 siguieron dando "rechazado"**, porque los agarra
`prev_hash` de forma redundante. O sea que esos dos controles **no son
diagnosticos** del chequeo de seq; el unico que depende exclusivamente de seq es
distinguir un hueco declarado de un salto. Es defensa en profundidad, no un bug,
pero "10 controles positivos" y "10 controles independientes" no son lo mismo.
Quedo escrito en el encabezado del propio archivo.

**Integridad:** `verificacion/t_testis.py`, 25.320 bytes, blob SHA `354d5bc7`,
comparado contra el `git hash-object` local: **coincide byte por byte**.

**NO MEDIDO:** el costo del `fsync` (hipotesis sin numero de los dos lados); nada
corrio contra Postgres, `verdicts` y `anchors` no existen; cual es el T correcto;
el gateway Go **sigue sin compilar** y el CI sin leerse; si el Sello se puede
publicar fuera del alcance del DBA en el deployment real; y ninguna de las 5
innovaciones esta implementada en Go.

**Archivo de esta respuesta:**
`respuestas/2026-09-07-04-testis-validador-ejecutado.md`.
