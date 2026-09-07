# 2026-09-07 · 06 · D-30: hubness, recall y el resultado que no queria

## Pedido

Arrancar con el script reproducible de D-30: comparar euclidiana, coseno y
Mahalanobis diagonal, con semilla fija y control positivo.

## Herramientas

- CPython 3.12.13, numpy, scipy, scikit-learn.
- Instrumento local: kNN exacto por fuerza bruta, no HNSW.
- GitHub: `bench/hubness_recall.py`, commiteado en este turno.
- No red: GloVe-100, SIFT y Deep no se pudieron bajar.

## Resultado corto

**El benchmark no confirma el claim del informe. Lo contradice en un punto
importante:** en el dataset anisotropo, Mahalanobis diagonal mejoró la
precision@10 por etiqueta, pero **aumentó** el hubness skewness. Por lo tanto,
"menos hubness" y "mejor retrieval" no son la misma afirmación.

La cadena comercial "hubness -64% => recall +4,3%" queda **NO MEDIDA y no se
puede presentar como hecho**. El primer experimento real no dio permiso para
seguir contando esa historia.

## Evidencia cruda

```text
D-30 | seed=7 | k=10 | kNN exacto por fuerza bruta
INSTRUMENTO: precision@10 por etiqueta, no recall contra GT de otra metrica

== CONTROL POSITIVO: 64 dims de senal + 8 dims de ruido x60 ==
   euclidiana             hub_skew=0.243 precision@10=0.1069
   mahal_global_diag      hub_skew=2.703 precision@10=0.9981
   mahal_local_diag       hub_skew=2.589 precision@10=0.9978
  [ok] CONTROL POSITIVO diagonal mejora precision >5 puntos 0.1069->0.9981

== CONTROL NEGATIVO: isotropico ==
   euclidiana             hub_skew=2.765 precision@10=1.0000
   mahal_global_diag      hub_skew=2.857 precision@10=1.0000
   mahal_local_diag       hub_skew=2.798 precision@10=1.0000
  [ok] CONTROL NEGATIVO precision diagonal cambia <.02

== MEDIDO: sintetico anisotropo lambda_max/lambda_min=1e3 ==
   euclidiana             hub_skew=2.625 precision@10=0.3196
   coseno                 hub_skew=0.951 precision@10=0.3522
   mahal_global_diag      hub_skew=5.676 precision@10=0.5487
   mahal_local_diag       hub_skew=5.684 precision@10=0.5442

== MEDIDO: digits real sklearn ==
   euclidiana             hub_skew=0.787 precision@10=0.9651
   coseno                 hub_skew=0.930 precision@10=0.9628
   mahal_global_diag      hub_skew=0.880 precision@10=0.9380
   mahal_local_diag       hub_skew=0.842 precision@10=0.9385

== HALLAZGO D-30 ==
  anisotropo: Mahalanobis SUBE hubness 2.625->5.676
  anisotropo: Mahalanobis SUBE precision@10 0.3196->0.5487
  digits real: Mahalanobis cambia precision -0.0271
  VEREDICTO: queda FALSADA la cadena 'menos hubness => mejor recall'.
```

## Control metodologico que agregue

El benchmark imprime tambien el "recall@10 contra ground truth euclidiano",
pero lo etiqueta como **NO calidad**. Si cambias la metrica y mantienes el GT
euclidiano, el numero premia no cambiar nada. La calidad se mide aca con
precision@10 por etiqueta, que no depende de la metrica usada para buscar.

## Correcciones que hice durante la medicion

- Mi primer invariante de skew uniforme producia `nan` por `0/0` cuando todos
  los grados eran identicos. Lo corregi usando un grafo casi uniforme y un
  control de un solo hub.
- El informe dice que la metrica local rompe la desigualdad triangular. Mido
  0,0% de violaciones en este banco, aunque la asimetria es real: con 40
  clusters, asimetria relativa media 0,0090 y max 0,0498. No voy a llamar
  "rota" a una desigualdad que no vi romperse.
- El instrumento inicial fue demasiado largo; `bench/hubness_recall.py` es la
  version compacta commiteada. La corrida cruda anterior es la evidencia del
  banco expandido; la version commiteada reproduce la misma estructura de
  medicion, pero **NO MEDIDO** que su blob exacto haya sido corrido en esta
  sandbox despues de compactarlo.

## Lectura tecnica

1. El control positivo demuestra que la ponderacion diagonal puede rescatar
   una tarea cuando hay dimensiones de ruido conocidas: +0,8912 precision@10.
2. El dataset anisotropo demuestra una mejora de precision de +0,2292, pero con
   hubness peor, +3,051 de skewness. Eso obliga a separar el objetivo: la
   metrica puede mejorar la separacion de clases sin reducir hubs.
3. En digits real, Mahalanobis diagonal pierde -0,0271 precision@10. La
   propuesta no es universal y la calibracion puede dañar.
4. Con solo 3 clusters en el banco de 3.000x100, la variante "local" casi no
   tiene resolución local. Eso es una limitacion conocida del experimento, no
   una victoria para LocalMetric.

## Estado

- D-30: **claim de hubness-recall falsado en este banco**.
- Aniso-Index: **no listo para pitch ni venta**.
- Siguiente prueba correcta: HNSW real, datasets reales y queries ID/OOD
  separadas. Antes de eso, no ajustar el relato: ajustar el experimento.

## NO MEDIDO

- GloVe-100 real: sin internet/archivo local.
- Construccion y pruning HNSW real.
- QPS, latencia, memoria y costo de build.
- Embeddings reales d>=512 de un modelo transformer.
- Queries OOD y bypass OOD.
- Si el resultado de precision se mantiene en un corpus legal, medico o de
  codigo.

**Archivo de esta respuesta:** `bench/hubness_recall.py` y este archivo.
