# 2026-09-07 · 04 · t_testis.py ejecutado, con Sello y el test D corregido

## 1. Pedido

"que corrijas el test D y agregues `Sello` al validador para que quede como
`verificacion/t_testis.py` listo para commitear con sus 8 controles positivos".

Entregado. Quedaron **10** controles positivos, no 8: dos salieron de hallazgos
nuevos que aparecieron implementando el Sello.

## 2. Herramientas declaradas

| Herramienta | Escribio | Cuota ajena |
| --- | --- | --- |
| sandbox local: CPython 3.12.13, `cryptography` presente | archivos temporales | no |
| GitHub API: `push_files` x2, `get_file_contents` x1 | si, este repo | no |
| ClickUp: un Doc publico | si | no |

## 3. Lo que acepto de la auditoria, sin discutir

| ID | Hallazgo | Estado |
| --- | --- | --- |
| D-20 | la cadena resiste borrado del medio y **no** de la cola | **aceptado y reproducido**, CP-5 |
| D-21 | el descarte que consume `Seq` da ROJO falso, indistinguible de D-18 | **aceptado y reproducido**, bloque F |
| D-22 | el cierre del caso por "10 aceptados" es incomputable con muestreo 1/K | aceptado. Fix: cerrar por `Streak`, y `RuleDecayed = 10` agregado al enum |
| D-23 | `BackoffNs` "despues de aplicar" es falso en 5 de los 9 sitios | **aceptado y verificado en la fuente**: 5 sitios tienen `writeReject` antes de `TriggerBlock`, 2 al revez, 2 sin `TriggerBlock`. El orden pasa a `TriggerBlock` → `Emit` → `writeReject` |
| D-24 | el caso 1 es un firehose de veredictos | aceptado. Fix: coalescer con `Repeat uint32` por ventana de backoff |
| D-25 | HMAC simetrico no es no-repudio frente a terceros | **aceptado y reproducido**, bloque G. La cadena refabricada por el tenedor de la clave verifica en VERDE |
| D-26 | data race preexistente: `lastSeen` es local a la conexion | **aceptado y verificado**: `lastSeen := time.Now()` esta dentro de `handleConn`, y no aparece en `AgentState` |

Y acepto la correccion metodologica de fondo: el par del test D no colisionaba,
asi que ese control no probaba nada. Corregido con el par correcto.

## 4. Lo que refuto

### D-27 REFUTADO · `uuid_generate_v7()` no es una dependencia sin nombrar

El auditor declaro que no leyo `audit/`. `audit/schema.sql` **define la funcion
el mismo**, en plpgsql, en la linea 7:

```
   4:CREATE EXTENSION IF NOT EXISTS pgcrypto;
   7:CREATE OR REPLACE FUNCTION uuid_generate_v7()
  51:    id             uuid        PRIMARY KEY DEFAULT uuid_generate_v7(),
```

No importa `uuidv7()` de PG 18 ni la busca en `uuid-ossp`: la escribe. La unica
dependencia real es `pgcrypto`, por `gen_random_bytes`, y esta declarada en la
linea 4. Verificado como invariante dentro del test (bloque I).

El NO MEDIDO que el auditor declaro sobre `custos_legis` tambien quedo cerrado:
**0 importadores en codigo de produccion**, ya medido en E-004 y con la
correccion de mi propio grep sucio anotada ahi.

## 5. Los dos hallazgos nuevos, que salieron de implementar el Sello

### D-28 · El Sello no cubre lo emitido despues del ultimo checkpoint

El Sello cierra D-20 para todo lo sellado. Lo que crecio **despues** del ultimo
checkpoint sigue siendo borrable en silencio:

```
   sello en seq 100, cadena crecio a 120, atacante trunca a 105:
      VERDE (cadena integra)
```

20 veredictos posteriores al Sello, borro 15, nadie se entera. **La ventana de
exposicion es exactamente T**, el intervalo de sellado, y por lo tanto T no es
un parametro de tuning: es el parametro de riesgo que se le promete al
comprador. Si el brochure dice "no-repudio", T tiene que estar en el contrato.

El control positivo que si da rojo (CP-9) es truncar **por debajo** del punto
sellado.

### D-29 · El `AgentFSM` no expone lo que el `Verdict` necesita

Este era el NO MEDIDO #3 del auditor ("no lei fsm.go"). Lo lei:

```
   metodos publicos del AgentFSM: ['Allow', 'TriggerBlock', 'RecordSuccess', 'DecayBackoff']
   campos que Testis necesita, y son privados: ['backoff', 'successStreak']
   existe algun getter de backoff/streak: False
```

`backoff` y `successStreak` son minuscula, y los cuatro metodos publicos no
devuelven nada. **Testis no puede llenar `BackoffNs` ni `Streak` sin agregarle
un `Snapshot()` al FSM.** Es una linea de codigo, y es la clase de cosa que
aparece a mitad de la implementacion y descoloca si no esta declarada antes.

## 6. Correccion mia sobre el mecanismo del Sello

El ADR y la auditoria proponian comparar `head != ancla`. **Eso da rojo en toda
cadena viva**: cualquier agente que recibio un veredicto nuevo despues del
checkpoint tendria un head distinto al sellado, y el validador gritaria
manipulacion sobre una cadena sana.

La regla implementada es: el Sello **fija un punto**, la cadena puede crecer por
encima, y lo que no puede es encogerse por debajo ni cambiar lo sellado. Medido:

```
   cadena que CRECIO despues del Sello: VERDE (cadena integra)
   sello fija seq 100 y la cadena tiene 90  -> ROJO
   sello fija seq 100 y la cadena tiene 0   -> ROJO
```

## 7. Evidencia cruda del verificador

```
canonico de ancho fijo: 181 bytes por veredicto (formato >Q32s32sqBQIII8dqiI)

== 0. INVARIANTES: la cadena limpia y el canonico ==
   cadena de 100 veredictos: VERDE (cadena integra)
   largos distintos del canonico entre los 100: {181}
  [ok  ] INVARIANTE       una cadena integra de 100 verifica en verde
  [ok  ] INVARIANTE       el canonico mide siempre 181 B (ancho fijo real)
  [ok  ] INVARIANTE       el primer prev_hash son 32 ceros
   la misma cadena contra su Sello: VERDE
  [ok  ] INVARIANTE       el Sello no da falso positivo sobre la cadena que sello
   cadena que CRECIO despues del Sello: VERDE (cadena integra)
  [ok  ] INVARIANTE       el Sello permite crecimiento posterior (no compara heads)

== CONTROLES POSITIVOS: cada uno TIENE que dar rojo ==
   (pediste 8; quedaron 10, dos de ellos por hallazgos nuevos)
  [ok  ] CP-1 rechazado    borrar el veredicto 50 (del medio)
         -> salto de seq en pos 49 (esperaba 50, vino 51)
  [ok  ] CP-2 rechazado    voltear 1 bit del Context del veredicto 50
         -> hash no coincide en seq 50
  [ok  ] CP-3 rechazado    reordenar los veredictos 50 y 51
         -> salto de seq en pos 49 (esperaba 50, vino 51)
  [ok  ] CP-4 rechazado    refirmar el veredicto 71 con otra clave HMAC
         -> firma invalida en seq 71
  [ .. ] borrar los ULTIMOS 10 SIN Sello: VERDE  <-- D-20 reproducido
  [ok  ] CP-5 rechazado    borrar los ULTIMOS 10 (truncamiento de cola), con Sello
         -> truncamiento de cola: el sello fija seq 100 y la cadena tiene 90
  [presente] DEFECTO          D-20 sin Sello el truncamiento de cola verifica en VERDE
  [ok  ] CP-6 rechazado    borrar la cadena ENTERA del agente, con Sello
         -> truncamiento de cola: el sello fija seq 100 y la cadena tiene 0
  [ok  ] CP-7 rechazado    adulterar la raiz Merkle del Sello
         -> SELLO adulterado (hash, raiz Merkle o firma)
  [ .. ] CP-8: colision de canonicalizacion, al estilo D-06
         naive(('a|b', 'c')) = 'a|b|c'
         naive(('a', 'b|c')) = 'a|b|c'
         naive colisiona: True   <-- TIENE que ser True, si no el test no prueba nada
         ancho_fijo(('a|b', 'c')) = b'a|b\x00\x00\x00\x00\x00c\x00\x00\x00\x00\x00\x00\x00'
         ancho_fijo(('a', 'b|c')) = b'a\x00\x00\x00\x00\x00\x00\x00b|c\x00\x00\x00\x00\x00'
         ancho fijo colisiona: False
  [ok  ] CP-8 rechazado    el canonico NAIVE colisiona con el par ('a|b','c') vs ('a','b|c') [control del propio test]
         -> naive produce 'a|b|c' para los dos: colision confirmada
  [ok  ] INVARIANTE       el mismo par NO colisiona en ancho fijo
  [ok  ] INVARIANTE       dos Verdict que difieren en un solo campo dan hash distinto

== E. D-28: cuanto NO cubre el Sello (hallazgo nuevo) ==
   sello en seq 100, cadena crecio a 120, atacante trunca a 105:
      VERDE (cadena integra)
   -> con Sello cada T, la ventana borrable son los veredictos posteriores
      al ultimo Sello. Aca: 20 veredictos, borro 15, nadie se entera.
  [presente] DEFECTO          D-28 el Sello NO cubre lo emitido despues del ultimo checkpoint: la ventana de exposicion es T, y T es el parametro de riesgo del producto
  [ok  ] CP-9 rechazado    truncar por DEBAJO del punto sellado (seq 99 < 100)
         -> truncamiento de cola: el sello fija seq 100 y la cadena tiene 99

== F. la politica de descarte (fix D-21) ==
   descarte que CONSUME seq: ROJO (salto de seq en pos 20 (esperaba 21, vino 23))
   -> indistinguible de un borrado: el grabador fabrica falsa evidencia
   descarte SIN consumir seq + dropped_since: VERDE
      huecos declarados (seq, perdidos): [(21, 2)]
      veredictos perdidos, declarados: 2
  [presente] DEFECTO          D-21 la politica de la spec 6.2 (descarte consume seq) da ROJO falso
  [ok  ] INVARIANTE       con el fix, el hueco queda DECLARADO y la cadena en verde

== G. D-25: HMAC simetrico no es no-repudio frente a terceros ==
   cadena REFABRICADA por el tenedor de la clave: VERDE
  [presente] DEFECTO          D-25 quien tiene TESTIS_HMAC_KEY refabrica la cadena entera y verifica en verde: es tamper-evidence, no no-repudio
   Sello firmado ed25519, verificado con la publica correcta: VERDE
   el mismo Sello con OTRA publica: ROJO
  [ok  ] INVARIANTE       ed25519: la publica correcta verifica el Sello
  [ok  ] CP-10 rechazado    verificar el Sello con una clave publica ajena
         -> firma ed25519 invalida para esa publica

== H. estado de la implementacion en el repo (readiness) ==
   en handleConn: writeReject=9  TriggerBlock=7  Emit=0
   -> TESTIS NO IMPLEMENTADO. El chequeo queda ARMADO: cuando aparezca
      el primer Emit, este test exige Emit == writeReject y se pone
      rojo si falta uno. No falla hoy para no dejar el CI rojo eternamente.
  [ok  ] INVARIANTE       los 9 rechazos siguen ahi para instrumentar

== I. audito los hallazgos de la auditoria contra la fuente ==
   schema.sql DEFINE uuid_generate_v7() en plpgsql: True
  [ok  ] INVARIANTE       D-27 REFUTADO: el esquema define la funcion, no la importa de una extension
   metodos publicos del AgentFSM: ['Allow', 'TriggerBlock', 'RecordSuccess', 'DecayBackoff']
   campos que Testis necesita, y son privados: ['backoff', 'successStreak']
   existe algun getter de backoff/streak: False
  [presente] DEFECTO          D-29 el AgentFSM no expone backoff ni successStreak (privados, sin getter): Testis NO puede llenar BackoffNs ni Streak sin agregarle un Snapshot() al FSM
   lastSeen declarado DENTRO de handleConn: True   en AgentState: False
  [presente] DEFECTO          D-26 CONFIRMADO: lastSeen (el dt del Huber) es local a la conexion, no al agente

controles positivos ejecutados: 10
fallos=0
```

Interprete de la corrida: CPython 3.12.13. Codigo de salida **0**.

## 8. Control de mutacion sobre mi propio instrumento, y lo que revelo

Desactive el chequeo de contiguidad de `seq` en una copia y volvi a correr:
**`fallos=1`, `rc=1`**, o sea que el sabotaje se detecta.

Pero el detalle importa: **CP-1 y CP-3 siguieron dando "rechazado"** con el
chequeo de seq apagado, porque los agarra el chequeo de `prev_hash` de forma
redundante. Consecuencia honesta: **esos dos controles no son diagnosticos del
chequeo de seq.** El unico que depende exclusivamente de seq es distinguir un
hueco declarado por descarte de un salto por manipulacion. Quedo escrito en el
encabezado del propio archivo, no solo aca.

Es defensa en profundidad, no un bug. Pero "tengo 10 controles positivos" y
"tengo 10 controles **independientes**" no son lo mismo, y decir el primero
sonando al segundo es simular rigor.

## 9. Archivos generados

- `verificacion/t_testis.py`, 25.320 bytes, blob SHA `354d5bc7`, **verificado
  contra el `git hash-object` local: coincide byte por byte**.
- `.github/workflows/ci.yml`: dos jobs nuevos, `custos-legis` y `testis`. Antes
  los verificadores de Python no los corria nadie, solo `compileall`. Eso era un
  agujero mio de E-002 que nadie habia marcado.
- `respuestas/2026-09-07-04-testis-validador-ejecutado.md` (este)
- `02-BITACORA.md` (E-005)
- `CONTEXTO-KAMPE-IR.md` (D-20 a D-29, y el orden de trabajo actualizado)

## 10. NO MEDIDO

1. **El costo del `fsync`** y la tasa a la que el WAL se vuelve cuello de
   botella. Group-commit sigue siendo hipotesis sin numero, de los dos lados.
2. **Nada corrio contra Postgres:** `verdicts` y `anchors` no existen como
   tablas. El SQL del ADR nunca se ejecuto.
3. **Cual es el T de sellado correcto** (D-28). Depende del riesgo que se acepte
   por ventana: es decision de producto, no medicion.
4. **El gateway Go sigue sin compilar** y el CI sin leerse. Todo lo que digo del
   gateway es analisis de fuente.
5. **Si el Sello publicado fuera del alcance del DBA es operativamente posible**
   en el deployment real. Sin eso, el Sello vive en la misma base que ancla y no
   ancla nada.
6. **Ninguna de las 5 innovaciones esta implementada en Go.** El validador
   demuestra que el diseno cierra; no demuestra que el gateway lo haga.
