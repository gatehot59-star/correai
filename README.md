# CORREAI

**CORREA + AI.** Un perimetro que deja correr agentes autonomos sin soltarles la
punta: los identifica, los mide, los tensa cuando se desvian, los afloja cuando
se portan bien, y notaria cada tiron.

No es un firewall. Un firewall no perdona; este devuelve el backoff al valor
inicial tras 10 paquetes limpios. Es una correa: se tensa y se suelta. Y en la
segunda acepcion, la correa de transmision, la pieza disenada para cortarse
primero y salvar el motor. Eso es un circuit breaker.

## Las cuatro piezas

| Pieza | Que es |
| --- | --- |
| **HiperSec** · `gateway/` (Go) | mTLS 1.3 obligatorio, un FSM con circuit breaker por agente, anti-replay, HMAC por paquete y dos filtros estadisticos (derivada de varianza de Huber + coherencia coseno) |
| **DualBrain v4.3** · `dualbrain/` (C++) | motor geometrico para el extremo: 1024 centroides x 64 dimensiones, Q8.8, sin heap, **268.288 bytes = 262,00 KiB medidos** |
| **Custos Legis** · `audit/` (Python + SQL) | log de auditoria firmado con HMAC-SHA256, rotacion de clave y uuid v7 |
| **Fleet Manager** · `fleet/` + `mqtt/` (FastAPI) | multi-tenant: registro de nodos, telemetria de curvatura kappa, OTA con descarga y ACL de MQTT por identidad |

## Estado real

| Pieza | Estado |
| --- | --- |
| DualBrain | **compila y corre**, huella exacta verificada con `static_assert` y con `sizeof` medido |
| Custos Legis | HMAC verificado contra la stdlib 200/200. **Canonicalizacion colisionable** |
| HiperSec | **nunca compilado**: no hay toolchain de Go donde se escribio esto. El CI lo mide |
| Fleet Manager | nunca ejecutado. Sin fastapi, asyncpg ni broker |
| Integracion | **cero**. No existe un cliente que complete un handshake mTLS |
| Clientes | cero |

17 defectos abiertos, 9 de ellos con numero e instrumento, en
`respuestas/2026-09-07-01-deduccion-del-nombre.md`. Los cuatro que importan:

1. Un paquete con `timestamp = 2^64-1` **y HMAC invalido** deja a un agente
   bloqueado para siempre: el `int64` de la ventana desborda a negativo, el
   contador se escribe antes de verificar la firma, y el estado del agente nunca
   se evicta.
2. El log de auditoria firma dos eventos **distintos** con la misma firma.
3. El `X-Delivery-Token` de la descarga OTA no se guarda ni se compara nunca.
4. La deteccion de anomalias depende de la norma de la entrada: un baseline de
   norma 10 marca anomalo su propio token.

## Metodo

- `CONTEXTO-CORREAI.md` — estado vivo. Se lee antes de responder, se sobreescribe.
- `02-BITACORA.md` — append-only: hipotesis, falsadores, hipotesis muertas.
- `respuestas/` — una entrega por archivo, con la evidencia cruda verbatim.
- `verificacion/` — verificadores ejecutados. Separan **INVARIANTE** (si se
  rompe, rojo) de **DEFECTO** (estado medido hoy; si desaparece, tambien rojo,
  para que nadie arregle un bug sin tocar la bitacora).
- Entorno canonico, que se re-mide y no se recuerda:
  [00-ENTORNOS-Y-CAPACIDADES.md](https://github.com/gatehot59-star/mudh-mobile/blob/main/00-ENTORNOS-Y-CAPACIDADES.md)

## Reglas desde el commit 1

1. Tres estados: **bien**, **mal** y **NO MEDIDO**. Nunca dos.
2. La independencia es del **instrumento**, no del operador.
3. Nada de binarios commiteados. El CI compila desde fuente.
4. Ningun script se commitea sin haberlo ejecutado.
5. Cada entrega deja dos artefactos: el commit aca y un Doc de ClickUp, linkeados.

## Correr los verificadores

```sh
g++ -std=c++17 -O2 -Wall -Wextra -Werror -o t_engine verificacion/t_engine.cpp && ./t_engine
gcc -O0 -Wall -Wextra -Werror -o t_ts verificacion/t_ts.c && ./t_ts
python3 verificacion/t_custos.py
python3 verificacion/t_fleet.py
```

Copyright (c) 2026 Jorge Abraham Mendieta. Computational Substrate Theory.
