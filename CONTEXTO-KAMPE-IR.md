# CONTEXTO-KAMPE-IR.md

Contexto vivo de KAMPE IR. **Se lee antes de responder cualquier cosa sobre este
proyecto.** Responder de memoria en lugar de abrir este archivo es un defecto.
Este archivo es estado y **se sobreescribe**; la acumulacion va en `respuestas/`.

Ultima actualizacion: 2026-09-07 (E-005).

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
| **HiperSec** (`gateway/`, Go) | Gateway de Contencion Perimetral | **no compilado todavia** |
| **DualBrain** (`dualbrain/engine.hpp`, C++) | Motor Geometrico Embebido Zero-Heap | compila y corre, 262,00 KiB medidos |
| **Custos Legis** (`audit/`, Python + SQL) | Boveda Criptografica Legal y No-Repudio | HMAC correcto medido; canonicalizacion colisionable, sin cadena de hash, **cero productores** |
| **Fleet Manager** (`fleet/` + `mqtt/`, FastAPI) | Gestor de Flota OTA y Telemetria | no ejecutado |
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

**Verde y medido:** `verificacion/t_testis.py` (blob `354d5bc7`) corre en 0 con
**10 controles positivos**, canonico de **181 bytes de ancho fijo**, cadena de
hash por agente, y el **Sello** (checkpoint con raiz Merkle de los heads).

**Cero codigo Go.** El validador demuestra que el diseno cierra; no demuestra
que el gateway lo haga.

Las 5 correcciones de diseno que entraron por la auditoria:

1. **Sello** publicado fuera del alcance del DBA. Sin el, borrar la cola verifica
   en verde (D-20).
2. **El descarte no consume `Seq`**; el siguiente veredicto declara
   `DroppedSince` (D-21).
3. **El caso se cierra por `Streak`**, no contando aceptados muestreados, y el
   FSM emite `RuleDecayed = 10` (D-22).
4. **Orden `TriggerBlock` → `Emit` → `writeReject`** en los 9 sitios (D-23).
5. **Coalescer el caso 1** con `Repeat uint32`, y firmar los Sellos con
   `crypto/ed25519` en vez de HMAC (D-24, D-25).

## 5. Donde corre

| Maquina | Para que sirve | Estado para KAMPE IR |
| --- | --- | --- |
| brain-env | taller persistente | sin asignar |
| Actions x64 | fabrica, gratis en repo publico | **asignada**: 6 jobs de CI en `main` |
| Actions arm64 | fabrica nativa arm64 | candidata: el motor es para el extremo |
| Kaggle GPU | GPU | no aplica (el motor no entrena) |

El inventario **se re-mide, no se recuerda**:
https://github.com/gatehot59-star/mudh-mobile/blob/main/00-ENTORNOS-Y-CAPACIDADES.md

## 6. Decisiones tomadas

| # | Decision | Fecha | Evidencia |
| --- | --- | --- | --- |
| D-01 | Repo publico | 2026-09-07 | commit 1. **Sin confirmar por Abraham** |
| D-02 | Rama por defecto `main` | 2026-09-07 | commit 1 |
| D-03 | Los 9 archivos del adjunto entran tal cual, blob SHA 9/9 | 2026-09-07 | 9 commits `codigo:` |
| D-04 | KAMPE IR es el ecosistema; HiperSec es el gateway | 2026-09-07 | E-003 |
| D-05 | Los verificadores separan INVARIANTE, CONTROL POSITIVO y DEFECTO | 2026-09-07 | `verificacion/` |
| D-06 | Bitacora append-only | 2026-09-07 | E-003 |
| D-07 | `go.mod` apunta a `kampe-ir` aunque el slug siga siendo `correai` | 2026-09-07 | **NO MEDIDO si el CI lo acepta** |
| D-08 | El modulo IR es Testis. No SOAR, no forense, no threat intel | 2026-09-07 | ADR-001 |
| D-09 | Testis serializa con ancho fijo: 181 B por veredicto, verificado | 2026-09-07 | `t_testis.py` |
| D-10 | El grabador nunca bloquea la aplicacion, y el descarte queda declarado | 2026-09-07 | E-005 |
| D-11 | El Sello **fija un punto**, no compara heads: la cadena puede crecer | 2026-09-07 | E-005 |
| D-12 | Los Sellos se firman con ed25519, no HMAC, para que valgan ante un perito | 2026-09-07 | E-005, CP-10 |

## 7. Orden de trabajo (bloqueantes primero)

1. **D-01+D-02 del gateway** (anti-replay). Bloquea a Testis.
2. **D-06** (canonicalizacion colisionable). Bloquea el **release**.
3. **D-29**: agregarle `Snapshot()` al `AgentFSM`. Bloquea la primera linea de
   `verdict.go`, porque sin eso no hay `BackoffNs` ni `Streak` que grabar.
4. **Testis en Go**: `verdict.go` → `recorder.go` → los 9 `Emit` en el orden
   nuevo → SQL (`verdicts`, `anchors`, `cases`) → `ir/case.py`.
5. **D-26** (data race de los filtros) antes de confiar en los veredictos 8 y 9.
6. D-08 (token de OTA), D-03 (normalizacion del motor), el resto.

## 8. Lo que espera decision de Abraham

1. **Renombrar el slug del repo** de `correai` a `kampe-ir`. Solo lo puede hacer
   el: la API con la que trabajo no expone rename de repositorios.
2. **Publico o privado.**
3. **Cuanto vale T**, el intervalo de sellado (D-28). Es la ventana de
   exposicion que se le promete al comprador.
4. **Si KAMPE IR es proyecto o pieza** de MUDH / AURA / SIAO.

## 9. NO MEDIDO abiertos

1. Si el gateway Go compila. El CI esta commiteado y **su resultado no fue
   leido**.
2. Si el `go.mod` con un path que no resuelve rompe algo en el CI.
3. El costo del `fsync` por veredicto y donde el WAL se vuelve cuello de
   botella. Group-commit es hipotesis sin numero.
4. Nada corrio contra Postgres: `verdicts`, `anchors` y `cases` no existen.
5. Si el Sello se puede publicar fuera del alcance del DBA en el deployment
   real. Sin eso no ancla nada.
6. El data race bajo `-race`: hace falta un test con dos conexiones del mismo
   certificado.
7. Ninguna prueba de integracion: no existe cliente mTLS.
8. **Marca:** no se consultaron USPTO, EUIPO ni INPI.
9. Si hay comprador. Cero llamadas reales.
