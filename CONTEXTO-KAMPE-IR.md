# CONTEXTO-KAMPE-IR.md

Contexto vivo de KAMPE IR. **Se lee antes de responder cualquier cosa sobre este
proyecto.** Responder de memoria en lugar de abrir este archivo es un defecto.
Este archivo es estado y **se sobreescribe**; la acumulacion va en `respuestas/`.

Reemplaza a `CONTEXTO-CORREAI.md`, borrado en el mismo turno.
Ultima actualizacion: 2026-09-07.

---

## 1. Que es KAMPE IR

**Ecosistema matriz de contencion perimetral para agentes autonomos.** Deja
correr agentes sin soltarles la punta: los identifica por certificado, los mide
con estadistica en vez de reglas, los tensa cuando se desvian, los afloja cuando
se portan bien, y notaria cada tiron con firma.

**Kampe** = la carcelera de Tartaro en la mitologia griega, designada por Kronos
para que los Ciclopes y Hecatonquiros no se escapen (Apolodoro 1.2.1). Existe en
las fuentes unicamente como esa funcion. **IR** queda deliberadamente
polisemico: Incident Response, Infraestructura Restringida, o fonetica
corporativa.

Nombre anterior: CORREAI, bautizado y descartado el mismo dia. La deduccion de
"CORREA + AI" sigue en `respuestas/2026-09-07-01-deduccion-del-nombre.md` como
historia, no como estado.

## 2. Los cuatro subsistemas

| Subsistema | Rol oficial | Estado |
| --- | --- | --- |
| **HiperSec** (`gateway/`, Go) | Gateway de Contencion Perimetral | **no compilado todavia** |
| **DualBrain** (`dualbrain/engine.hpp`, C++) | Motor Geometrico Embebido Zero-Heap | compila y corre, 262,00 KiB medidos |
| **Custos Legis** (`audit/`, Python + SQL) | Boveda Criptografica Legal y No-Repudio | HMAC correcto medido, canonicalizacion colisionable |
| **Fleet Manager** (`fleet/` + `mqtt/`, FastAPI) | Gestor de Flota OTA y Telemetria | no ejecutado |

**Resuelto el choque de nombres** que estaba abierto: KAMPE IR es el ecosistema,
HiperSec es **solo el gateway**. El `go_package = "hipersec/gateway/v1"` del
proto y los prefijos de error `hipersec:` ahora son correctos por primera vez:
nombran el componente, no el sistema.

**Ojo con el nombre DualBrain:** este es C++, 262 KiB, 1024x64. El DualBrain del
motor de MUDH es C99 y **704 bytes**. Comparten nombre y no son el mismo
artefacto. Antes de mezclar mediciones de los dos, decidir si es una generacion
nueva o un nombre reusado.

## 3. Donde corre

| Maquina | Para que sirve | Estado para KAMPE IR |
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
| D-03 | Los 9 archivos del adjunto entran tal cual, verificados por blob SHA de git uno por uno (9/9) | 2026-09-07 | 9 commits `codigo:` |
| D-04 | **KAMPE IR es el ecosistema; HiperSec es el gateway.** Cierra el choque de nombres | 2026-09-07 | E-003 |
| D-05 | Los verificadores separan INVARIANTE de DEFECTO: si un defecto medido desaparece, el test se pone rojo y obliga a tocar la bitacora | 2026-09-07 | `verificacion/` |
| D-06 | La bitacora es append-only: la deduccion de CORREAI **no se reescribe**, queda como historia y E-003 la supersede | 2026-09-07 | E-003 |
| D-07 | `go.mod` apunta a `kampe-ir` aunque el slug del repo siga siendo `correai` | 2026-09-07 | `go.mod`. **NO MEDIDO si el CI lo acepta** |

## 5. Lo que espera decision de Abraham

1. **Renombrar el slug del repo** de `correai` a `kampe-ir`. Solo lo puede hacer
   el: la API con la que trabajo no expone rename de repositorios. GitHub
   redirige los links viejos, asi que nada se rompe.
2. **Publico o privado.** Es un producto de seguridad con "todos los derechos
   reservados" y hoy esta publico porque lo elegi yo.
3. **Si "IR" se vende como Incident Response.** Tal como esta construido, el
   sistema hace prevencion, contencion y auditoria; **no hace** case management
   ni forense ni triage. Lo unico IR-adyacente es la alerta de deriva kappa por
   MQTT. Prometer IR a un comprador de seguridad es prometer un modulo que no
   existe.
4. **Que se arregla primero.** Mi orden: D-01+D-02 (un paquete ladrilla un
   agente para siempre), D-06 (dos eventos distintos, misma firma), D-08 (el
   token de OTA no controla nada), D-03 (la deteccion depende de la norma).
5. **Si KAMPE IR es proyecto o pieza** de MUDH / AURA / SIAO.

## 6. NO MEDIDO abiertos

1. Si el gateway Go compila. El CI esta commiteado y **su resultado no fue
   leido**.
2. Si el cambio de `go.mod` a un path que no resuelve rompe algo en el CI.
3. El data race de los filtros: hace falta un test con dos conexiones del mismo
   certificado bajo `-race`.
4. El YAML del workflow no lo valido ningun parser local.
5. El SQL nunca toco un Postgres.
6. Ninguna prueba de integracion: no existe cliente mTLS.
7. **Marca:** una busqueda de texto no encontro ninguna empresa de seguridad
   llamada Kampe. Eso **no es** una busqueda de marca: no consulte USPTO, EUIPO
   ni INPI. Antes de vender licencias, esto lo mira un abogado.
8. Si hay comprador. El codigo esta escrito para vender (multi-tenant, API keys,
   OTA) y no hubo una sola llamada real.
