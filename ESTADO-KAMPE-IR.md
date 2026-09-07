# ESTADO-KAMPE-IR.md

Mapa del proyecto al **2026-09-07 21:00 UTC**, escrito para leerse de una vez.
El estado tecnico vivo esta en `CONTEXTO-KAMPE-IR.md`; esto es el mapa de
**que existe, que esta medido y que falta**, mas el problema de proceso que
ninguno de los dos archivos declaraba.

---

## 1. Lo primero, porque cambia como se lee todo lo demas

**Hay 7 ramas y CERO mergeadas.** `main` sigue en el commit `aa98aa0d`, que es el
codigo del dia 1 tal como entro del adjunto. Todo lo que se midio y arreglo hoy
vive en ramas que esperan tu merge.

Consecuencia concreta: **el repo no tiene una sola verdad, tiene siete.** Y
`respuestas/` esta partido: la 07 esta en una rama, la 08 en otra, la 09 en otra,
y ninguna rama tiene la bitacora completa. El indice real de hoy es este archivo.

| Rama | Que trae | Estado |
| --- | --- | --- |
| `main` | los 9 archivos del adjunto, sin tests | intacto desde el dia 1 |
| `titan/hnsw-public-benchmark` | benchmark HNSW real (FAISS, Iris y Wine UCI) | verde, sin PR |
| `titan/auditoria-de-la-auditoria` | auditoria de D-40..D-54 + `CONTEXTO` actualizado | verde, sin PR |
| `titan/fix-acl-mqtt` | el fix del ACL de MQTT | **PR #1**, 9/9 verde |
| `titan/test-go-race` | primer test Go, mide D-26 | **PR #2**, 13/13 verde |
| `titan/fix-d26-race` | el fix de D-26 | **PR #3**, 16/16 verde |
| `titan/bench-candado` | costo del candado | **PR #4**, 26/26 verde |
| `titan/latencia-p99` | p99 de la espera | verde, **sin PR** |

Las ramas estan **encadenadas**, no en paralelo: `latencia-p99` desciende de
`bench-candado`, que desciende de `fix-d26-race`, que desciende de `test-go-race`.
Asi que `titan/latencia-p99` es la punta y contiene todo el trabajo de Go.

**`titan/fix-acl-mqtt` y `titan/auditoria-de-la-auditoria` estan por fuera de esa
cadena** y van a necesitar merge aparte.

---

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
+-- .github/workflows/  4 workflows, todos commitean su propio resultado
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
mqtt/acl.conf                    el fix esta en OTRA rama

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

**Todos con evidencia cruda commiteada y control que puede dar rojo.**

## 5. Que FALTA, en orden de lo que rompe el producto

1. **Mergear.** Siete ramas, cero merges. Cada dia que pasa el conflicto crece.
2. **Limitar conexiones concurrentes por certificado.** Cierra TRES hallazgos de
   una vez: la contencion, el p99 de 28,9 us y la mezcla semantica de la EWMA.
   Es la unica pieza que tres mediciones distintas senalaron sola.
3. **`engine.hpp` v4.4:** Q4.12 en vez de Q8.8, normalizar el token, sacar la
   palabra "spinlock" que promete un candado inexistente. Sin esto el motor se
   apaga solo y el OTA nunca se dispara.
4. **Formato `centroids.bin` v2** unico para Fleet y motor (D-42): hoy Fleet
   acepta un paquete que el motor **no puede cargar**.
5. **`download_ota`:** guardar `sha256(delivery_token)` y comparar, o sacar el
   header. Un header exigido y no verificado es peor que no tenerlo.
6. **Testis en Go.** Todo el modulo IR esta especificado y validado en Python, y
   **no existe una linea de Go**. Es el diferencial del producto.
7. **Nada toco Postgres.** Las 5 tablas de `schema.sql` no existen en ninguna
   base.
8. **No hay cliente mTLS**, asi que no hay una sola prueba punta a punta.
9. **Cero clientes reales.** Ninguna de las 18 mediciones prueba que alguien
   quiera esto.

## 6. Lo que espera decision tuya

1. **Mergear las 7 ramas, y en que orden.** Yo no mergeo.
2. **Renombrar el slug** `correai` -> `kampe-ir`. Solo lo podes hacer vos.
3. **Que hace el gateway con la segunda conexion** del mismo certificado:
   rechazarla o multiplexar. De esto dependen los puntos 2 y 3 de arriba.
4. **Que L2 tiene el A53 objetivo:** 256 KiB obliga a bajar a 768 centroides.
5. **Cuanto vale T**, el intervalo de sellado de Testis.
6. **Publico o privado**, y si KAMPE IR es proyecto o pieza de MUDH / AURA / SIAO.

## 7. El patron de mis propios errores, hoy

**Cinco turnos, cinco defectos, y los cinco en el INSTRUMENTO, no en el sujeto:**

1. Un guard que paso **contando un `echo` mio** que mencionaba `DATA RACE`.
2. Evidencia cruda que solo se imprimia en el camino de fallo (testigo unico).
3. Un log que imprimia `1.000000`, exactamente el valor que el guard prohibia.
4. Un guard que buscaba `Total:`, cadena que `pprof -top` **no emite**.
5. Un guard de un lado solo, que casi publico **el piso del reloj** como el p99.

Y dos estructurales, repetidos cuatro veces: **un archivo compartido produce un
guard que mide otra cosa**, y **una seccion sin guard falla en silencio** (la
dispersion entre corridas imprimio `NA` y el veredicto siguio en verde).

La leccion operativa: en este proyecto el sujeto medido casi siempre resulto
como la medicion decia. **Lo que falla es el aparato.** Cada instrumento nuevo
necesita su propio control antes de que su numero valga.
