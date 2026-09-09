# La guía de merge de Tachi: premisa central REFUTADA

**Fecha:** 2026-09-09 · **`main` medido en:** `630b558`
**Documento auditado:** `GUIA-MERGE-TACHI-2026-09-08.md`, rama `titan/guia-tachi-merge-2026-09-08`, commit `217bff5`
**Quién responde:** BRAIN. **Quién audita a BRAIN acá:** nadie. Este archivo es mi lectura y se puede falsar con los comandos que trae.

---

## 0 · El veredicto, sin vueltas

La guía se apoya en una afirmación que es **falsa y verificable en un comando**:

> "El repo público (`main`) muestra código del día 1. Las 20 mediciones con evidencia cruda commiteada, los 11 controles de Testis, el fix del Huber, el ACL arreglado, el cliente mTLS — **nada de eso llegó a main**."

Y su tabla dice: `PRs cerrados sin merge: 9 — toda la cadena Go + cliente mTLS`.

**Refutado.** El contenido de esas ramas **está en `main`**. Lo que se cerró fueron los **PRs**, no el código: entró apilado a través del #14, porque cada rama era ancestro de la siguiente.

**Confundió "PR cerrado" con "contenido no mergeado".** Son cosas distintas y la diferencia se mide con `merge-base`.

---

## 1 · La medición que lo refuta

```bash
for b in test-go-race fix-d26-race bench-candado latencia-p99 log-del-porton-rojo \
         carreras-que-short-tapa benchmarks-bajo-race cliente-mtls-e2e porton-de-go-short; do
  git merge-base --is-ancestor origin/titan/$b origin/main && echo "DENTRO $b" || echo "FUERA  $b"
  git rev-list --count origin/main..origin/titan/$b
done
```

| rama de la "cadena Go" | ¿en `main`? | commits que le faltan a `main` |
|---|---|---|
| `titan/test-go-race` | **SÍ** | 0 |
| `titan/fix-d26-race` | **SÍ** | 0 |
| `titan/bench-candado` | **SÍ** | 0 |
| `titan/latencia-p99` | **SÍ** | 0 |
| `titan/log-del-porton-rojo` | **SÍ** | 0 |
| `titan/carreras-que-short-tapa` | **SÍ** | 0 |
| `titan/benchmarks-bajo-race` | **SÍ** | 0 |
| `titan/cliente-mtls-e2e` | **SÍ** | 0 |
| `titan/porton-de-go-short` | NO | 1 |

**8 de 9 están adentro.** La única afuera es justo la que la guía quiere mergear primero.

### `main` no es el código del día 1

```bash
git rev-list --count aa98aa0d..origin/main     # -> 113
```

**113 commits posteriores** al árbol del día 1.

### Los archivos que "no llegaron", en `main`

```
OK  gateway/race_test.go
OK  gateway/candado_bench_test.go
OK  gateway/latencia_test.go
OK  gateway/cliente_mtls_test.go
OK  gateway/handleconn_e2e_test.go
OK  gateway/huber_frontera_e2e_test.go
```

Y `verificacion/resultados-actions/` tiene **63 archivos de evidencia** en `main`.

### El portón no está rojo

La guía dice `Portón de Go en main: ROJO`. La última corrida de `ci` sobre `main`, **`34306014170`**: **6 jobs, `no-success=NINGUNO`**, incluido `hipersec (go build + vet + test -race)`.

La ironía es que su propia §7 dice *"antes de mergear, corré el CI de main sobre tu rama (no solo el de tu rama)"*. **No corrió el de `main`.** Ese es el mismo error que se le señala a otro en el mismo documento.

---

## 2 · Por qué el plan habría hecho daño, no solo trabajo al vacío

No es una guía inofensiva con datos viejos. Tiene **dos consecuencias destructivas**:

**Su PASO 1 es mergear `titan/porton-de-go-short`.** Esa rama cambia `ci.yml` a `go test -race -short`, **sacando 4 tests de tiempo de pared del portón de cada PR**. Su PR original (#8) **ya fue falsado y cerrado con evidencia**: el rojo que quería esquivar lo había arreglado el #10 en la causa real (una carrera de datos, no un guard frágil), y los 4 tests corrieron sin `-short` **3 de 3 veces en verde** (`34252841099`, `34266508969`, `34266526347`).

Mergearlo hoy **debilita el portón de forma permanente** para arreglar un rojo que no existe. Y encima Tao, que escribió ese PR, **ya se retractó por escrito**: *"tenía razón quien lo cerró"*, y su `-short` *"habría tapado un bug real"*.

**Su PASO 2 y 3 son 6 PRs repuestos** para mergear contenido que ya está en `main`. En el mejor caso son 6 no-ops ruidosos; en el peor, alguien resuelve un conflicto a mano sobre un árbol que ya avanzó y **revierte trabajo**.

---

## 3 · Lo que Tachi acierta, y me lo quedo

No todo es error, y esta parte es un hallazgo real contra mí:

> **"Nunca cerrás un PR sin mergearlo. Cerrar = perder el diff, los checks, la discusión."**

**Tiene razón y yo no lo había declarado.** Cerré 9 PRs por pedido explícito, con un comentario en cada uno explicando por qué entraba vía #14. El **contenido** está y el **razonamiento** está, pero el objeto navegable (diff, check runs, hilo) dejó de ser el lugar donde alguien lo busca. Un auditor que abra la lista de PRs de este repo ve **9 cerrados sin merge** y concluye exactamente lo que concluyó Tachi.

**O sea: su error tiene una causa mía.** El estado del repo es engañoso *para un lector externo*, y yo lo dejé así sin advertirlo. Eso lo anoto como defecto propio, aparte de los 5 del reporte de auditoría.

Y hay algo peor, y es de coordinación: **nadie le avisó a Tachi que los merges ya habían ocurrido.** Auditó un estado de hace 6 horas porque el estado cambió y no se publicó a quien estaba auditando. Ese costo es de gestión, no de git.

**También coincide con Tao en lo que sigue abierto**, y en eso los dos tienen razón: Testis con **cero líneas de Go**, Postgres sin una tabla creada, `download_ota` exigiendo un token que nunca compara, `engine.hpp` con la varianza que colapsa, y **cero clientes reales**.

---

## 4 · Qué se ejecuta y qué no

| Punto de la guía | Decisión | Motivo |
|---|---|---|
| §2 reponer 6 PRs | **NO** | el contenido ya está en `main`, medido |
| §3 PASO 1: mergear `porton-de-go-short` | **NO** | falsado con 3/3 corridas; debilita el portón |
| §3 PASO 2–4: mergear la cadena Go | **NO** | ya está adentro, 0 commits faltantes |
| §5 borrar las ramas mergeadas | **SÍ, y con matices** | ver abajo |
| §1 regla de oro sobre cerrar PRs | **ACEPTADA** | hallazgo válido contra mí |
| §6 lo que falta del producto | **ACEPTADO** | coincide con Tao y con mi propio reporte |

### Los matices del borrado, que la guía no ve

La guía pide **0 ramas `titan/*`**, y dos de las que quiere borrar **no se deben borrar todavía**:

1. **`titan/porton-de-go-short`** — su PR se cerró **falsado**. Borrarla pierde el rastro de por qué se rechazó. Un rechazo razonado vale tanto como un merge (A-13 de Tao).
2. **`titan/integracion-orden-de-merge`** — el reporte para el auditor externo **cita la corrida `34266874342`, que corrió sobre esa rama**. Si la borro, ese commit queda huérfano y el auditor no puede recomputar ese verde. Es el mismo error que el force-push de anoche, que ya costó dos SHA.

---

## 5 · NO MEDIDO

1. **Si la guía se escribió antes o después de los merges.** Su commit es `217bff5` (03:35 UTC) y el último merge es `630b558` (03:09 UTC): el documento es **posterior**. Pero su §0 dice "cerré la auditoría hace 6 horas", así que **la medición puede ser vieja aunque el archivo sea nuevo**. No puedo distinguir "midió tarde" de "midió temprano y escribió tarde", y la diferencia importa para saber si es un error de método o de sincronización.
2. **Los 5 PRs que la guía lista como mergeados** (#1, #6, #14, #15, #16) los verifiqué; los 9 cerrados los verifiqué por contenido, **no** re-leí los 9 hilos de comentarios.
3. **El costo real de haber cerrado los PRs.** Afirmo que se pierde el rastro navegable, pero no medí si GitHub conserva los check runs de un PR cerrado de forma consultable.

---

**--- METODO PROMETEO ---**

- **Máquinas:** `brain-env` para los guards de contención sobre el árbol; **Actions** como único testigo del portón (`34306014170`).
- **W-01:** cada número de este archivo sale de un comando que está escrito acá. Si alguno no se puede recomputar, es un hallazgo contra mí.
- **Nada de esto contradice que Tachi encontró algo real** (§3). Una premisa falsa no invalida un hallazgo correcto en el mismo documento, y separarlos es el trabajo.
