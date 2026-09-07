# Benchmark HNSW real, datasets públicos

## Pedido
Ejecutar un benchmark HNSW real con datasets públicos.

## Herramientas y máquina
Se usó `mcp_gateway__gateway_call_tool`, servicio `build`, en `brain-env`. Instrumento real: `faiss.IndexHNSWFlat`, FAISS 1.8.0, Python 3.12.14. Se descargaron Iris UCI y Wine Quality Red UCI por sus URLs oficiales; no se usó runtime ajeno ni cuota de Actions/Kaggle.

## Qué se midió
Semilla 20260907, `k=10`, `M=16`, `efSearch=64`, ground truth exacto con `faiss.IndexFlatL2` bajo la misma transformación. Se midieron recall@10, build_ms, latencia p50/p95, QPS, tamaño serializado del índice y hubness skew, para L2, cosine y Mahalanobis diagonal. Se ejecutaron 6 configuraciones, sobre 150 Iris y 1.599 vinos.

Resultado clave: HNSW dio recall 0.9767 a 1.0000 en Iris y 0.9972 a 0.9988 en Wine. Mahalanobis no mejoró recall frente a L2 en estos datos: empató en Iris y bajó 0.00125 en Wine; en Wine aumentó hubness de 0.715 a 1.236 y bajó QPS de 5176 a 377. La hipótesis de que reducir hubness mejora recall queda sin apoyo en esta corrida.

## Evidencia cruda
Comando exacto:
```sh
curl -fsSL https://raw.githubusercontent.com/gatehot59-star/correai/69f699c34d765a8c29db906c8b731f6370fc82a9/bench/hnsw_public.py -o hnsw_public.py && PYTHONPATH=/workspace/homebrain/.local/lib/python3.12/site-packages python3 hnsw_public.py --out results.json
```
Exit code: `0`. Hash del instrumento ejecutado: `633e38278e3d0e202edf56e95dc97a677df58848f433b91f0b7452af801d8646`. Hash de resultados: `599b93bea197833605bf4edc1a0fd3d7d16617313bc68c1a95da820746f1b652`. La salida completa y los hashes de ambos datasets están en `bench/hnsw_public_results.json`.

## Archivos generados
- `bench/hnsw_public.py`
- `bench/hnsw_public_results.json`
- este archivo de respuesta

## NO MEDIDO
No se midieron SIFT-1M, GloVe-100 ni Deep-1B, OOD queries, memoria residente incremental confiable en este proceso pequeño, ni barrido de `efSearch`/`M`. Los datasets UCI son públicos y reales, pero pequeños: no autorizan conclusiones de escala industrial.

Rama: `titan/hnsw-public-benchmark`. Commit del instrumento: `69f699c34d765a8c29db906c8b731f6370fc82a9`; commit de resultados: `bf7e8260d1e2bf7fad48be3b443886b733bf3121`.

--- METODO TITAN ---
Accion delicada: NO
Modo aplicado:   TITAN FULL
Rubrica:         86/95 -> 90,5/100
N/A declarados:  5 (DevOps: instrumento de benchmark; no hay deployment propio)
Review externo:  ninguno pedido; deuda declarada
Instrumento:     FAISS HNSW + IndexFlatL2, exit 0, evidencia en `bench/hnsw_public_results.json`
Maquina:         brain-env
Artefactos:      `bench/hnsw_public.py` + `bench/hnsw_public_results.json` + este archivo + Doc de ClickUp
