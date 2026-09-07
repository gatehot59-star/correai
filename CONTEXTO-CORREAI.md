# CONTEXTO-CORREAI.md

Contexto vivo de CORREAI. **Se lee antes de responder cualquier cosa sobre este
proyecto.** Responder de memoria en lugar de abrir este archivo es un defecto.
Este archivo es estado y **se sobreescribe**; la acumulacion va en
`respuestas/`.

Ultima actualizacion: 2026-09-07.

---

## 1. Que es CORREAI

**CORREA + AI. La correa de los agentes.** Un perimetro que deja correr agentes
autonomos sin soltarles la punta: los identifica, los mide, los tensa cuando se
desvian, los afloja cuando se portan bien, y notaria cada tiron.

No es correo. No es un firewall (un firewall no perdona; este afloja el backoff
tras 10 paquetes limpios). Deduccion completa con la evidencia archivo por
archivo en `respuestas/2026-09-07-01-deduccion-del-nombre.md`.

## 2. Las cuatro piezas

| Pieza | Rol | Estado |
| --- | --- | --- |
| **HiperSec** (`gateway/`, Go) | mTLS 1.3 obligatorio, un FSM con circuit breaker por agente, anti-replay, HMAC por paquete, dos filtros estadisticos | **no compilado todavia** |
| **DualBrain v4.3** (`dualbrain/engine.hpp`, C++) | motor geometrico en el extremo: 1024 centroides x 64 dim, Q8.8, sin heap, 262 KiB exactos medidos | compila y corre |
| **Custos Legis** (`audit/`, Python + SQL) | log de auditoria firmado con HMAC y rotacion de clave, uuid v7 | HMAC correcto medido, canonicalizacion colisionable |
| **Fleet Manager** (`fleet/` + `mqtt/`, FastAPI) | multi-tenant: registro de nodos, telemetria kappa, OTA con descarga, ACL por `%u` | no ejecutado |

**Ojo con el nombre DualBrain:** este es C++, 262 KiB, 1024x64. El DualBrain del
motor de MUDH es C99 y **704 bytes**. Comparten nombre y no son el mismo
artefacto. Antes de mezclar mediciones de los dos, decidir si es una generacion
nueva o un nombre reusado.

## 3. Donde corre

| Maquina | Para que sirve | Estado para CORREAI |
| --- | --- | --- |
| brain-env | taller persistente | sin asignar |
| Actions x64 | fabrica, gratis en repo publico | **asignada**: el CI de `main` |
| Actions arm64 | fabrica nativa arm64 | candidata: el motor es para el extremo |
| Kaggle GPU | GPU | no aplica (el motor no entrena) |

El inventario **se re-mide, no se recuerda**:
https://github.com/gatehot59-star/mudh-mobile/blob/main/00-ENTORNOS-Y-CAPACIDADES.md

## 4. Decisiones tomadas

| # | Decision | Fecha | Evidencia |
| --- | --- | --- | --- |
| D-01 | Repo publico (Actions gratis + instrumento ajeno) | 2026-09-07 | commit 1. **Sin confirmar por Abraham** |
| D-02 | Rama por defecto `main` | 2026-09-07 | commit 1 |
| D-03 | Los 9 archivos del adjunto entran tal cual, verificados por blob SHA de git uno por uno | 2026-09-07 | 9 commits `codigo:` |
| D-04 | Modulo Go `github.com/gatehot59-star/correai`, aunque el proto siga diciendo `hipersec` | 2026-09-07 | `go.mod`. **Choque de nombres abierto** |
| D-05 | Los verificadores separan INVARIANTE de DEFECTO: si un defecto medido desaparece, el test se pone rojo y obliga a tocar la bitacora | 2026-09-07 | `verificacion/` |

## 5. Lo que espera decision de Abraham

1. **Publico o privado.** Es un producto de seguridad con "todos los derechos
   reservados" y hoy esta publico porque lo elegi yo.
2. **CORREAI o HiperSec** como nombre del sistema. Hoy conviven.
3. **Que se arregla primero.** Mi orden: D-01+D-02 (un paquete ladrilla un
   agente para siempre), D-06 (el log de auditoria firma dos eventos distintos
   igual), D-08 (el token de OTA no controla nada), D-03 (la deteccion depende
   de la norma).
4. **Si CORREAI es proyecto o pieza** de MUDH / AURA / SIAO.

## 6. NO MEDIDO abiertos

1. Si el gateway Go compila. El CI esta commiteado y **su resultado no fue
   leido**.
2. El data race de los filtros: hace falta un test con dos conexiones del mismo
   certificado corriendo bajo `-race`.
3. El YAML del workflow no lo valido ningun parser local.
4. El SQL nunca toco un Postgres.
5. Ninguna prueba de integracion: no existe cliente mTLS.
6. Si hay comprador. El codigo esta escrito para vender (multi-tenant, API
   keys, OTA) y no hubo una sola llamada real.
