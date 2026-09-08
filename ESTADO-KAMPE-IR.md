# ESTADO-KAMPE-IR.md

Mapa del proyecto al **2026-09-07 22:50 UTC**, escrito para leerse de una vez.
El estado tecnico vivo esta en `CONTEXTO-KAMPE-IR.md`; esto es el mapa de
**que existe, que esta medido y que falta**, mas los problemas de proceso que
ninguno de los dos archivos declaraba.

---

## 0. LO PRIMERO: el porton de Go esta ROJO en la punta de la cadena

**Mergear la cadena de Go hoy pone `main` en rojo.** Medido con check runs por
PR, no con el estado combinado:

| PR | Rama | `hipersec (go build + vet + test -race)` |
| --- | --- | --- |
| #1 | `titan/fix-acl-mqtt` | success, 6/6 verde |
| #2 | `titan/test-go-race` | success, 6/6 verde |
| #3 | `titan/fix-d26-race` | success, 6/6 verde |
| #4 | `titan/bench-candado` | success, 6/6 verde |
| **#5** | `titan/latencia-p99` | **failure (52 s)**, los otros 5 verde |

**La causa, y es un defecto mio:** el `ci.yml` de `main` corre
`go test -race ./...` **sin `-short`**, y ahi entran los cuatro tests de tiempo
de pared que **yo mismo** declare invalidos de medir bajo `-race`, con el factor
**16,9x** medido y commiteado. Mis workflows de rama corren `-short -race` y a eso
le puse el nombre de "suite de correctitud": medi con mi propio porton y nunca
abri el compartido.

**El fix ya existe y esta verificado: PR #8** (`titan/porton-de-go-short`), con
base en esta rama a proposito, o sea corriendo sobre el arbol que hoy falla.
Rojo -> verde, un flag de diferencia, y los 4 tests pasan a un job propio sin
`-race`.

**Ojo: `-short` saca el guard del porton, NO lo arregla.** Si el guard es fragil
bajo el detector sigue fragil. Issue #7 abierto.

**Y un aviso de instrumento:** el estado combinado devuelve
`total_count: 0, statuses: []` sobre 6 jobs verdes. Los jobs de Actions publican
**check runs**, no commit statuses. En este repo, siempre check runs.

Lo encontro **Tao** (super agente de ClickUp), no yo. Verificacion en
`respuestas/2026-09-07-14-verificacion-de-la-auditoria-de-tao.md`.

## 1. Las ramas: 8 ramas, CERO mergeadas, y hacen falta 3 merges

`main` sigue en el commit `aa98aa0d`, que es el codigo del dia 1. Todo lo que se
midio y arreglo hoy vive en ramas.

**CORRECCION de una version anterior de este archivo:** decia que
`titan/fix-acl-mqtt` y `titan/auditoria-de-la-auditoria` estaban "por fuera de la
cadena y van a necesitar merge aparte". **Era falso.** `fix-acl-mqtt` se creo
**desde** `auditoria-de-la-auditoria`, asi que la PR #1 ya arrastra los tres
workflows de auditoria, `t_engine_audit.{cpp,out}`, la respuesta 08 y la
reescritura del contexto. Me lo corrigio Tao con el diff en la mano.

**Son 3 hilos y 3 merges, no 7.**

| Hilo | Ramas, de abajo hacia arriba | PR |
| --- | --- | --- |
| **A · ACL + auditoria** | `auditoria-de-la-auditoria` -> `fix-acl-mqtt` | **#1** (base `main`) |
| **B · Go (cadena de 4 + el fix)** | `test-go-race` -> `fix-d26-race` -> `bench-candado` -> `latencia-p99` <- `porton-de-go-short` | #2, #3, #4, #5, **#8** |
| **C · benchmark HNSW** | `hnsw-public-benchmark` | **#6** (la abrio Tao; yo la habia dejado sin PR) |

`titan/auditoria-de-la-auditoria` **no necesita PR propia.**

## 2. El arbol, general

```
kampe-ir/  (slug del repo todavia: correai)
|
+-- gateway/          HiperSec .......... Go, el perimetro
+-- dualbrain/        DualBrain ......... C++, el motor geometrico
+-- audit/            Custos Legis ...... Python + SQL, la boveda
+-- fleet/ + mqtt/    Fleet Manager ..... FastAPI + ACL, la flota
+-- (sin carpeta)     Testis ............ el modulo IR: especificado, cero Go
|
+-- verificacion/     los instrumentos de medicion y su evidencia
+-- bench/            benchmarks de recuperacion vectorial
+-- respuestas/       bitacora append-only (PARTIDA entre ramas)
+-- .github/workflows/  5 workflows; los de rama commitean su propio resultado
```

## 3. El arbol, particular (rama `titan/latencia-p99`, la punta)

```
gateway/                        HiperSec: compila, go vet limpio
|-- gateway.go          15.156 B  mTLS, anti-replay, HMAC, 9 rechazos, 0 logs
|-- filters.go           5.707 B  Huber + Coherence  <- D-26 ARREGLADO aca
|-- fsm.go               2.188 B  circuit breaker por agente
|-- proto/perimeter.proto          el contrato del paquete
|-- race_test.go        20.932 B  suite de concurrencia + 4 controles
|-- candado_bench_test.go 13.584 B  4 brazos + control positivo
`-- latencia_test.go    16.044 B  cuantiles + piso de reloj

dualbrain/engine.hpp             el motor: compila, y la varianza colapsa
audit/custos_legis.py            HMAC ok, canonicalizacion colisionable
audit/schema.sql                 5 tablas, ninguna creada en ningun Postgres
fleet/fleet_manager.py  15.916 B  no ejecutado nunca
mqtt/acl.conf                    el fix esta en OTRA rama (PR #1)

verificacion/
|-- t_testis.py         31.413 B  11 controles positivos, fallos=0
|-- t_custos.py, t_fleet.py, t_engine.cpp, t_ts.c
`-- resultados-actions/  18 archivos de evidencia cruda commiteada

bench/hubness_recall.py          D-30, kNN exacto
```

---

## 4. Que esta MEDIDO, con su numero

| # | Que | Numero | Donde |
| --- | --- | --- | --- |
| 1 | El gateway compila y `go vet` sale limpio | exit 0 y 0 | VM limpia de Actions |
| 2 | El motor compila en dos toolchains | g++ 12.2 y 13.3, exit 0 | sandbox + Actions |
| 3 | El motor **no cabe** en un L2 de 256 KiB | 266.240 B > 262.144 B | el compilador |
| 4 | La varianza del motor **colapsa a 0** | antes de la iteracion 300 | `t_engine_audit` |
| 5 | El cero es **estado absorbente** | escapar exige sq >= 0,390625 | barrido de amplitudes |
| 6 | `process_token` **no** normaliza el token | dist 0,000000 vs 1,000000 | mismo instrumento |
| 7 | El ACL viejo **cerraba todo** | `Denied PUBLISH rc135` | mosquitto 2.0.18 |
| 8 | El ACL nuevo funciona | 9/9, la OTA llega al nodo | + mutacion: 4 rojos |
| 9 | **D-26 existia** | 27 reportes, 16 lineas de `filters.go` | detector de carreras |
| 10 | **D-26 esta cerrado** | 0 reportes, 0 citas | + mutacion: 23 reportes |
| 11 | D-47: el HMAC no cubre context ni payload | los dos alterables | sobre la funcion real |
| 12 | El candado cuesta | **10,03 ns** por paquete | 1,6% del HMAC (629 ns) |
| 13 | El caso real **no tiene contencion** | 322 us / 500k ops, 0 en mis candados | perfil de mutex |
| 14 | Quitar el candado es **mas lento** | hasta 1,58x | 4 brazos |
| 15 | p99 de la espera, caso real | **258 ns** | 3 corridas |
| 16 | p99 de la espera, patologico | **28.911 ns** (112x) | estable en 3 corridas |
| 17 | Mahalanobis diagonal **no mejora** recall | empata o baja | HNSW real, Iris y Wine |
| 18 | Testis: el diseno cierra | 11 controles, fallos=0 | `t_testis.py` |
| 19 | **El porton de Go de `main` esta rojo** | failure en 52 s, PR #5 | check runs |
| 20 | El `-short` lo pone verde | 36 s, mismo arbol | PR #8 |

**Todos con evidencia cruda commiteada y control que puede dar rojo.**

## 5. Que FALTA, en orden de lo que rompe el producto

1. **Mergear el PR #8 primero.** Sin eso, mergear la cadena pone `main` en rojo.
2. **Los 3 merges.** Ocho ramas, cero merges. El conflicto crece solo.
3. **Limitar conexiones concurrentes por certificado.** Cierra TRES hallazgos de
   una vez: la contencion, el p99 de 28,9 us y la mezcla semantica de la EWMA.
   Es la unica pieza que tres mediciones distintas senalaron sola.
4. **`engine.hpp` v4.4:** Q4.12 en vez de Q8.8, normalizar el token, sacar la
   palabra "spinlock" que promete un candado inexistente. Sin esto el motor se
   apaga solo y el OTA nunca se dispara.
5. **Formato `centroids.bin` v2** unico para Fleet y motor (D-42): hoy Fleet
   acepta un paquete que el motor **no puede cargar**.
6. **`download_ota`:** guardar `sha256(delivery_token)` y comparar, o sacar el
   header. Un header exigido y no verificado es peor que no tenerlo.
7. **Testis en Go.** Todo el modulo IR esta especificado y validado en Python, y
   **no existe una linea de Go**. Es el diferencial del producto.
8. **Nada toco Postgres.** Las 5 tablas de `schema.sql` no existen en ninguna base.
9. **No hay cliente mTLS**, asi que no hay una sola prueba punta a punta.
10. **Cero clientes reales.** Ninguna de las 20 mediciones prueba que alguien
    quiera esto.

## 6. Lo que espera decision tuya

1. **Mergear.** Orden: PR #8 -> la cadena de Go -> PR #1 -> PR #6. Yo no mergeo.
2. **Renombrar el slug** `correai` -> `kampe-ir`. Solo lo podes hacer vos.
3. **Que hace el gateway con la segunda conexion** del mismo certificado:
   rechazarla o multiplexar. De esto dependen los puntos 3 y 4 de arriba.
4. **Que L2 tiene el A53 objetivo:** 256 KiB obliga a bajar a 768 centroides.
5. **Cuanto vale T**, el intervalo de sellado de Testis.
6. **Publico o privado**, y si KAMPE IR es proyecto o pieza de MUDH / AURA / SIAO.

## 7. El patron de mis propios errores, hoy

**Seis turnos, seis defectos, y los seis en el INSTRUMENTO, no en el sujeto:**

1. Un guard que paso **contando un `echo` mio** que mencionaba `DATA RACE`.
2. Evidencia cruda que solo se imprimia en el camino de fallo (testigo unico).
3. Un log que imprimia `1.000000`, exactamente el valor que el guard prohibia.
4. Un guard que buscaba `Total:`, cadena que `pprof -top` **no emite**.
5. Un guard de un lado solo, que casi publico **el piso del reloj** como el p99.
6. **Medi con mi propio porton de rama (`-short -race`) y nunca abri el porton
   compartido de `main`**, que corre sin `-short`. Los cinco primeros daban un
   numero raro; este pone `main` en rojo el dia del merge. Lo encontro Tao.

Y dos estructurales, repetidos cuatro veces: **un archivo compartido produce un
guard que mide otra cosa**, y **una seccion sin guard falla en silencio** (la
dispersion entre corridas imprimio `NA` y el veredicto siguio en verde).

La leccion operativa: en este proyecto el sujeto medido casi siempre resulto
como la medicion decia. **Lo que falla es el aparato.** Cada instrumento nuevo
necesita su propio control antes de que su numero valga, **y el porton propio no
es el porton compartido.**
