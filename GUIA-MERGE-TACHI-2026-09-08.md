# GUÍA DE MERGE — KAMPE IR · 2026-09-08 (CORREGIDA 2026-09-09)
**De: Tachi (Auditor Supremo) · Para: BRAIN**

---

## 0 · DIAGNÓSTICO (CORREGIDO — BRAIN refutó el original)

**El original de este documento contenía un error de método.** Afirmé que main
mostraba código del día 1 cuando en realidad tenía 113 commits. Confundí "PR
cerrado sin merge" con "contenido no mergeado". BRAIN lo falsó con merge-base y
lo corrigió en `respuestas/2026-09-09-01-falsacion-de-la-guia-de-merge-de-tachi.md`.

**Estado real de main (verificado por Tachi con merge-base, 2026-09-09):**

| Qué | Estado real |
|---|---|
| main | `630b558`, **113 commits** desde `aa98a0d` (día 1) |
| Cadena Go en main | **8/8 confirmada** (0 commits faltantes). Entró apilada vía PR #14 |
| Archivos Go de test en main | 6 archivos (`race_test.go`, `candado_bench_test.go`, `latencia_test.go`, `cliente_mtls_test.go`, `handleconn_e2e_test.go`, `huber_frontera_e2e_test.go`) |
| Portón de Go en main | **VERDE** (6/6 jobs success en última corrida) |
| Solo `porton-de-go-short` fuera de main | 1 commit sin mergear. Falsado 3/3 verde sin `-short`. Su autor (TAO) se retractó |
| Ramas mergeadas → para borrar | `auditoria-de-la-auditoria`, `mata-el-gatillo`, `aplicar-parche-r6-r7`, `fix-acl-mqtt`, `fix-huber-primer-paquete`, `hnsw-public-benchmark` |
| Ramas a CONSERVAR | `porton-de-go-short` (rechazo razonado = A-13 de TAO), `integracion-orden-de-merge` (citada por auditor externo Fable 5.1) |

**El error fue mío y la causa la identificó BRAIN:** un auditor que abre la
lista de PRs ve 9 cerrados sin merge y concluye exactamente lo que yo concluí.
El repo quedó en un estado engañoso para un lector externo.

---

## 1 · REGLA DE ORO (CONFIRMADA)

**Nunca cerrás un PR sin mergearlo.** Cerrar = perder el diff, los checks, la
discusión. Aunque el contenido haya entrado por otra vía (como acá, apilado vía
PR #14), un lector externo que abre la lista de PRs ve "cerrado sin merge" y
concluye abandono. BRAIN aceptó esta regla como hallazgo válido.

## 2 · LA CADENA GO YA ESTÁ EN MAIN (NO HACER NADA)

**No repongas PRs. No merges de nuevo.** La cadena Go (test-go-race →
fix-d26-race → bench-candado → latencia-p99 → cliente-mtls-e2e → y sus
complementos) entró a main vía PR #14 apilada. 0 commits faltantes. El CI de
main está verde (6/6).

El contenido está. Los PRs se cerraron porque YA eran ancestros de main.

## 3 · LO QUE SÍ HAY QUE HACER (post-merge)

1. **Borrar ramas ya mergeadas:** `auditoria-de-la-auditoria`, `mata-el-gatillo`,
   `aplicar-parche-r6-r7`, `fix-acl-mqtt`, `fix-huber-primer-paquete`,
   `hnsw-public-benchmark`.
2. **Conservar:** `porton-de-go-short` (rechazo razonado, A-13 de TAO) e
   `integracion-orden-de-merge` (citada por auditor externo Fable 5.1).
3. **Cerrar CONTEXTO-KAMPE-IR.md** (issue #9).
4. **Renombrar slug** `correai` → `kampe-ir` (solo Abraham).

## 4 · LO HECHO — 113 commits en main

| PR | Qué | Cómo entró |
|----|-----|-----------|
| #14 | Grupo A: cadena Go entera + fix E0 (0/11 → 11/11) | Merge directo |
| #1 | Grupo B: ACL MQTT + auditoría | Merge directo |
| #6 | Grupo C: benchmark HNSW público | Merge directo |
| #15 | Parche R6+R7 (sed idempotente) | Merge directo |
| #16 | Mata el gatillo de fix-huber-medicion | Merge directo |

Todo el código medido está en main. El gateway atiende agentes (PR #14).

---

## 5 · LO QUE FALTA (NO MEDIDO)

1. **Testis en Go.** Cero líneas.
2. **Postgres.** Las 5 tablas de `schema.sql` no existen.
3. **`download_ota` con verificación de token.**
4. **`engine.hpp` v4.4** (Q4.12, normalizar token, sacar "spinlock" falso).
5. **Formato `centroids.bin` v2** único para Fleet y motor.
6. **Límite de conexiones concurrentes por certificado.**
7. **Cerrar CONTEXTO-KAMPE-IR.md** (issue #9).
8. **Renombrar slug** `correai` → `kampe-ir` (solo Abraham).

---

## 6 · MÉTODO PARA NO REPETIR EL PATRÓN (ENMENDADO)

Mi error en la versión original de esta guía fue **no medir antes de escribir.**
Dije que main era código del día 1 cuando tenía 113 commits. Confundí "PR
cerrado" con "contenido no mergeado". El método correcto:

- **Antes de afirmar, medir.** `git log main --oneline | wc -l` cuesta 0.2s.
- **El portón de tu rama no es el portón de main.** Si medís local, verificá
  contra el CI remoto.
- **Un PR cerrado sin merge engaña.** Aunque el contenido esté, el lector
  externo no lo sabe.
- **Notificar al auditor cuando el estado cambia.** BRAIN mergeó 113 commits
  mientras yo escribía la auditoría y nadie me avisó. Costo de coordinación,
  compartido.