# CONTEXTO-KAMPE-IR.md

Contexto vivo de KAMPE IR. **Se lee antes de responder cualquier cosa sobre este
proyecto.** Responder de memoria en lugar de abrir este archivo es un defecto.
Este archivo es estado y **se sobreescribe**; la acumulacion va en `respuestas/`.

Ultima actualizacion: 2026-09-07 (E-004).

---

## 1. Que es KAMPE IR

**Ecosistema matriz de contencion perimetral para agentes autonomos.** Deja
correr agentes sin soltarles la punta: los identifica por certificado, los mide
con estadistica en vez de reglas, los tensa cuando se desvian, los afloja cuando
se portan bien, y notaria cada tiron con firma.

**Kampe** = la carcelera de Tartaro en la mitologia griega, designada por Kronos
para que los Ciclopes y Hecatonquiros no se escapen (Apolodoro 1.2.1). Existe en
las fuentes unicamente como esa funcion.

**IR = evidencia y triage de incidentes**, decidido en ADR-001. No es Incident
Response completo: no hay case management con SLAs, ni forense, ni notificacion
regulatoria. Nombrarlo de mas es prometer un modulo inexistente.

Nombre anterior: CORREAI, bautizado y descartado el mismo dia.

## 2. Los subsistemas

| Subsistema | Rol oficial | Estado |
| --- | --- | --- |
| **HiperSec** (`gateway/`, Go) | Gateway de Contencion Perimetral | **no compilado todavia** |
| **DualBrain** (`dualbrain/engine.hpp`, C++) | Motor Geometrico Embebido Zero-Heap | compila y corre, 262,00 KiB medidos |
| **Custos Legis** (`audit/`, Python + SQL) | Boveda Criptografica Legal y No-Repudio | HMAC correcto medido; canonicalizacion colisionable y **sin cadena de hash** |
| **Fleet Manager** (`fleet/` + `mqtt/`, FastAPI) | Gestor de Flota OTA y Telemetria | no ejecutado |
| **Testis** (`gateway/verdict.go`, `gateway/recorder.go`, `ir/`) | Grabador de Veredictos y Caso por Agente | **especificado, sin escribir** · ADR-001 |

KAMPE IR es el ecosistema; **HiperSec es solo el gateway**. El
`go_package = "hipersec/gateway/v1"` del proto es correcto: nombra el componente.

**Ojo con el nombre DualBrain:** este es C++, 262 KiB, 1024x64. El DualBrain del
motor de MUDH es C99 y **704 bytes**. Comparten nombre y no son el mismo
artefacto.

## 3. El agujero que Testis llena (medido)

- El gateway tiene **9 caminos de rechazo y 7 de bloqueo**, y **cero llamadas de
  log**. El paquete `log` no esta ni importado.
- El gateway **no tiene driver de base de datos**: ni una escritura.
- Custos Legis tiene **cero importadores** en codigo de producto.
- `rejectBytes = []byte{0xFF}`: **un byte igual para los 9 rechazos**. Nadie,
  adentro ni afuera, puede saber por que fue rechazado un agente.

O sea: **el sistema aplica y no atestigua.** Ese es el producto IR.

## 4. Donde corre

| Maquina | Para que sirve | Estado para KAMPE IR |
| --- | --- | --- |
| brain-env | taller persistente | sin asignar |
| Actions x64 | fabrica, gratis en repo publico | **asignada**: el CI de `main` |
| Actions arm64 | fabrica nativa arm64 | candidata: el motor es para el extremo |
| Kaggle GPU | GPU | no aplica (el motor no entrena) |

El inventario **se re-mide, no se recuerda**:
https://github.com/gatehot59-star/mudh-mobile/blob/main/00-ENTORNOS-Y-CAPACIDADES.md

## 5. Decisiones tomadas

| # | Decision | Fecha | Evidencia |
| --- | --- | --- | --- |
| D-01 | Repo publico | 2026-09-07 | commit 1. **Sin confirmar por Abraham** |
| D-02 | Rama por defecto `main` | 2026-09-07 | commit 1 |
| D-03 | Los 9 archivos del adjunto entran tal cual, blob SHA verificado 9/9 | 2026-09-07 | 9 commits `codigo:` |
| D-04 | KAMPE IR es el ecosistema; HiperSec es el gateway | 2026-09-07 | E-003 |
| D-05 | Los verificadores separan INVARIANTE de DEFECTO | 2026-09-07 | `verificacion/` |
| D-06 | Bitacora append-only: E-001 y E-002 no se reescriben | 2026-09-07 | E-003 |
| D-07 | `go.mod` apunta a `kampe-ir` aunque el slug siga siendo `correai` | 2026-09-07 | **NO MEDIDO si el CI lo acepta** |
| D-08 | **El modulo IR es Testis: grabador de veredictos con cadena de hash + caso por agente.** No SOAR, no forense, no threat intel | 2026-09-07 | ADR-001 |
| D-09 | Testis serializa con ancho fijo, no con separadores, para no repetir D-06 | 2026-09-07 | ADR-001 §6.1 |
| D-10 | El grabador **nunca** puede bloquear la aplicacion; descarta y atestigua el descarte | 2026-09-07 | ADR-001 §6.2 |

## 6. Orden de trabajo (bloqueantes primero)

1. **D-01+D-02 del gateway** (anti-replay: un paquete con HMAC invalido ladrilla
   a un agente para siempre). Bloquea a Testis: si no, el primer caso IR del
   primer cliente es un bug nuestro, firmado y encadenado.
2. **D-06** (canonicalizacion colisionable). Bloquea el **release** de Testis:
   no se puede vender no-repudio con una funcion de firma ambigua viva en el
   mismo producto.
3. **Testis**, en el orden del ADR: `verdict.go` → `recorder.go` → los 9 puntos
   de insercion → SQL → `ir/case.py` → `verificacion/t_testis.py`.
4. D-08 (token de OTA), D-03 (normalizacion del motor), el resto.

## 7. Lo que espera decision de Abraham

1. **Renombrar el slug del repo** de `correai` a `kampe-ir`. Solo lo puede hacer
   el: la API con la que trabajo no expone rename de repositorios.
2. **Publico o privado.**
3. **Si KAMPE IR es proyecto o pieza** de MUDH / AURA / SIAO.

## 8. NO MEDIDO abiertos

1. Si el gateway Go compila. El CI esta commiteado y **su resultado no fue
   leido**.
2. Si el `go.mod` con un path que no resuelve rompe algo en el CI.
3. El costo de escritura del grabador: `fsync` por veredicto sin medir, y el
   muestreo de aceptados es hipotesis sin numero.
4. El data race de los filtros: hace falta un test con dos conexiones del mismo
   certificado bajo `-race`.
5. El YAML del workflow no lo valido ningun parser local.
6. El SQL nunca toco un Postgres.
7. Ninguna prueba de integracion: no existe cliente mTLS.
8. **Marca:** una busqueda de texto no encontro empresas de seguridad llamadas
   Kampe. Eso **no es** una busqueda de marca: no se consultaron USPTO, EUIPO ni
   INPI.
9. Si hay comprador. Cero llamadas reales.
