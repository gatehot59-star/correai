# GUÍA DE MERGE — KAMPE IR · 2026-09-08
**De: Tachi (Auditor Supremo) · Para: BRAIN**
**Leela de una vez. No tiene segunda vuelta.**

---

## 0 · DIAGNÓSTICO

Cerré la auditoría hace 6 horas. En ese momento había **12 PRs abiertos, 0 mergeados**.
Ahora hay **0 PRs abiertos, 5 mergeados (#1, #6, #14, #15, #16), 9 cerrados sin merge**.

**El repo público (`main`) muestra código del día 1.** Las 20 mediciones con evidencia
cruda commiteada, los 11 controles de Testis, el fix del Huber, el ACL arreglado, el
cliente mTLS — **nada de eso llegó a main.**

| Qué | Estado |
|---|---|
| main | `630b558` (merge de PR #16). **Anterior: `aa98a0d` (día 1).** |
| PRs mergeados | 5 de 14 (#1, #6, #14, #15, #16) |
| PRs cerrados sin merge | 9 — toda la cadena Go + cliente mTLS |
| Ramas titan/* vivas | 16 |
| Portón de Go en main | **ROJO** (`go test -race ./...` sin `-short`) |
| CONTEXTO-KAMPE-IR.md | Desactualizado por diseño (delta en issue #9) |

**No es un problema técnico. Es un problema de gestión de releases.** Mediste 20 cosas
con evidencia cruda en 2 días. Ningún proyecto de tu tamaño tiene ese rigor. Pero el
producto no existe en main.

---

## 1 · REGLA DE ORO

**Nunca cerrás un PR sin mergearlo.** Cerrar = perder el diff, los checks, la discusión.
Si una rama ya no aplica, la mergeás a una rama de integración o la dejás abierta con
un comentario. **Si no la mergeaste, no está cerrada.**

---

## 2 · LOS 9 PRs QUE FALTAN — reposición

Reabrilos desde la CLI o desde GitHub. Son estos, en este orden:

| # | PR | Rama | De qué va |
|---|----|------|-----------|
| 1 | ~~ex-PR #8~~ → **nuevo PR #17** | `titan/porton-de-go-short` | Fix del portón. Mergealo PRIMERO |
| 2 | ~~ex-PR #2~~ → **nuevo PR #18** | `titan/test-go-race` | Primer test Go con -race |
| 3 | ~~ex-PR #3~~ → **nuevo PR #19** | `titan/fix-d26-race` | D-26 cerrado |
| 4 | ~~ex-PR #4~~ → **nuevo PR #20** | `titan/bench-candado` | Benchmark del candado |
| 5 | ~~ex-PR #5~~ → **nuevo PR #21** | `titan/latencia-p99` | Latencia p99 |
| 6 | ~~ex-PR #13~~ → **nuevo PR #22** | `titan/cliente-mtls-e2e` | Cliente mTLS |
| 7 | ~~ex-PR #10~~ | `titan/log-del-porton-rojo` | Log del portón rojo — **podés mergearlo directo a main, no necesita PR** |
| 8 | ~~ex-PR #11~~ | `titan/carreras-que-short-tapa` | Carreras tapadas por -short — **merge directo a main** |
| 9 | ~~ex-PR #12~~ | `titan/benchmarks-bajo-race` | Benchmarks bajo -race — **merge directo a main** |

**Para reponer un PR cerrado en GitHub CLI:**
```bash
cd /workspace/correai
gh pr create --base main --head titan/porton-de-go-short \
  --title "fix(ci): el portón de Go corría sin -short, main estaba rojo" \
  --body "Fix del portón de Go. PR #8 original, cerrada sin merge."
```

Si no tenés `gh`, creá el PR desde https://github.com/gatehot59-star/correai/pulls → New pull request → base: main, compare: titan/porton-de-go-short.

---

## 3 · ORDEN DE MERGE (estricto, no salteable)

```
PASO 1: PR #17 (ex-PR #8, porton-de-go-short)
  |
  v
  El portón de main pasa de ROJO a VERDE.
  Sin esto, cualquier merge de Go rompe el CI.
  |
PASO 2: PR #18 -> #19 -> #20 -> #21 (cadena Go, en orden)
  |
  v
  test-go-race -> fix-d26-race -> bench-candado -> latencia-p99
  Mergeás de a uno y esperás checks verdes entre cada merge.
  |
PASO 3: PR #22 (cliente-mtls-e2e)
  |
  v
  El primer cliente mTLS. Con esto el gateway YA ATIENDE AGENTES.
  |
PASO 4: Mergeá directo a main: log-del-porton-rojo, carreras-que-short-tapa,
         benchmarks-bajo-race. No necesitan PR (son complementos de medición).
```

**Verificás que main está verde después de cada paso.** Si un merge rompe algo,
parás y arreglás antes de seguir.

---

## 4 · LO HECHO (para que no lo pierdas de vista)

Esto YA está en main (5 PRs mergeados):

| PR | Qué |
|----|-----|
| #1 | ACL MQTT arreglado (9/9 verde) |
| #6 | Benchmark HNSW público |
| #14 | Fix del Huber (0/11 → 11/11, UNA línea) |
| #15 | Parche R6+R7 (sed idempotente) |
| #16 | Mata el gatillo de fix-huber-medicion |

**main pasó del commit del día 1 al commit `630b558`.** Ya no es el esqueleto
vacío. Pero le falta TODO lo medido en el núcleo Go.

---

## 5 · LAS 16 RAMAS TITAN/* — limpieza post-merge

Después de mergear todo, te van a quedar ramas huérfanas. Plan:

| Rama | Destino |
|------|---------|
| `titan/auditoria-de-la-auditoria` | Ya fue absorbida por PR #1. **Borrala.** |
| `titan/mata-el-gatillo` | Mergeada vía PR #16. **Borrala.** |
| `titan/aplicar-parche-r6-r7` | Mergeada vía PR #15. **Borrala.** |
| `titan/fix-acl-mqtt` | Mergeada vía PR #1. **Borrala.** |
| `titan/fix-huber-primer-paquete` | Mergeada vía PR #14. **Borrala.** |
| `titan/hnsw-public-benchmark` | Mergeada vía PR #6. **Borrala.** |
| `titan/integracion-orden-de-merge` | Rama de trabajo. **Borrala.** |
| Las 9 de la cadena Go | Mergeadas → **borralas.** |
| `titan/guia-tachi-merge-2026-09-08` | Esta guía. **Borrala** cuando termines. |

**Objetivo: 0 ramas `titan/*`, 100% código en main.**

---

## 6 · LO QUE FALTA DESPUÉS DE LOS MERGES (NO MEDIDO)

No es urgente, pero es el producto real:

1. **Testis en Go.** Cero líneas hoy.
2. **Postgres.** Las 5 tablas de `schema.sql` no existen.
3. **`download_ota` con verificación de token.**
4. **`engine.hpp` v4.4** (Q4.12, normalizar token, sacar "spinlock" falso).
5. **Formato `centroids.bin` v2** único para Fleet y motor.
6. **Límite de conexiones concurrentes por certificado.**
7. **Cerrar CONTEXTO-KAMPE-IR.md** (issue #9).
8. **Renombrar slug `correai` → `kampe-ir`** (solo Abraham).

---

## 7 · MÉTODO PARA NO REPETIR EL PATRÓN

Documentaste 6 errores en 6 turnos, todos en el INSTRUMENTO, no en el sujeto.
**Lección operativa:** cada instrumento que creás necesita un control que lo
refute ANTES de que su número valga. Y **el portón de tu rama no es el portón
de main.**

Reglas simples:
- Antes de mergear, corré el CI de main sobre tu rama (no solo el de tu rama).
- Un PR existe para ser mergeado, no para ser cerrado.
- Una rama mergeada se borra.

---

## 8 · RESUMEN PARA ABRAHAM

Abraham: esto es lo que BRAIN va a hacer. No necesitás decidir nada técnico.
Solo observá:

1. PRs repuestos → cadena de merge → main verde → ramas limpias.
2. Producto funcional: gateway que atiende agentes (PR #14 + #22 + cliente real).
3. Renombrar slug `correai` → `kampe-ir` cuando esté todo en main (vos).