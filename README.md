# KAMPE IR

**Ecosistema matriz de contencion perimetral para agentes autonomos.**
Autor: Jorge Abraham Mendieta. Computational Substrate Theory.

Bautizado el 2026-09-07. Nombre anterior del repo: CORREAI (ver
`02-BITACORA.md`, entrada E-003).

## Los cuatro subsistemas

| Subsistema | Rol oficial | Que es tecnicamente |
| --- | --- | --- |
| **HiperSec** · `gateway/` (Go) | Gateway de Contencion Perimetral | mTLS 1.3 obligatorio, un FSM con circuit breaker por agente, anti-replay, HMAC por paquete, y dos filtros estadisticos (derivada de varianza de Huber + coherencia coseno) |
| **DualBrain** · `dualbrain/` (C++) | Motor Geometrico Embebido Zero-Heap | 1024 centroides x 64 dimensiones, Q8.8, sin heap dinamico, **268.288 bytes = 262,00 KiB medidos** |
| **Custos Legis** · `audit/` (Python + SQL) | Boveda Criptografica Legal y No-Repudio | log de auditoria firmado con HMAC-SHA256, rotacion de clave, uuid v7 |
| **Fleet Manager** · `fleet/` + `mqtt/` (FastAPI) | Gestor de Flota OTA y Telemetria | multi-tenant: registro de nodos, telemetria de curvatura kappa, OTA con descarga, ACL de MQTT por identidad |

## Sobre el nombre

**Kampe** (Kάμπη) es, en la mitologia griega, la **guardiana de la carcel de
Tartaro**: el monstruo que Kronos designa para que los Ciclopes y los
Hecatonquiros no se escapen del pozo. No tiene mitologia propia aparte de esa
funcion: existe en las fuentes **unicamente** como la carcelera. Fuente:
Apolodoro, *Biblioteca* 1.2.1.

Para un producto cuyo unico trabajo es contener, el nombre es exacto.

Y la contracara, que queda escrita aca a proposito: **Kampe pierde.** Zeus la
mata para liberar a los prisioneros, y esa muerte es lo que hace posible la
victoria olimpica. Ademas la designa Kronos, o sea el regimen que cae. Un
comprador tecnico que conozca el mito puede leer "la contencion que fue
sorteada".

Lo interesante del mito para la arquitectura es lo que Zeus hace **despues**:
reemplaza a la guardiana monstruosa unica por los Hecatonquiros, ex prisioneros
convertidos en guardianes. Guardia distribuida con interes alineado en lugar de
un unico cuello de botella obediente. Eso se parece mas al FSM por agente de
HiperSec que a un gateway como punto unico.

## Estado real

| Pieza | Estado |
| --- | --- |
| DualBrain | **compila y corre**, huella exacta verificada con `static_assert` y con `sizeof` medido |
| Custos Legis | HMAC verificado contra la stdlib 200/200. **Canonicalizacion colisionable** |
| HiperSec | **nunca compilado**: no hay toolchain de Go donde se escribio esto. El CI lo mide |
| Fleet Manager | nunca ejecutado. Sin fastapi, asyncpg ni broker |
| Integracion | **cero**. No existe un cliente que complete un handshake mTLS |
| Clientes | cero |

17 defectos abiertos, 9 con numero e instrumento, en
`respuestas/2026-09-07-01-deduccion-del-nombre.md`. Los cuatro que importan:

1. Un paquete con `timestamp = 2^64-1` **y HMAC invalido** deja a un agente
   bloqueado para siempre: el `int64` de la ventana desborda a negativo, el
   contador se escribe antes de verificar la firma, y el estado del agente nunca
   se evicta.
2. Custos Legis firma dos eventos **distintos** con la misma firma.
3. El `X-Delivery-Token` de la descarga OTA no se guarda ni se compara nunca.
4. La deteccion de anomalias depende de la norma de la entrada: un baseline de
   norma 10 marca anomalo su propio token.

## Metodo

- `CONTEXTO-KAMPE-IR.md` — estado vivo. Se lee antes de responder, se sobreescribe.
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

## Pendiente que solo puede hacer Abraham

- **Renombrar el slug del repo** de `correai` a `kampe-ir` (Settings → General
  → Repository name). GitHub redirige los links viejos. La API con la que
  trabajo no expone rename de repositorios.
- **Decidir publico o privado.** Hoy es publico por una decision mia.

Copyright (c) 2026 Jorge Abraham Mendieta. Computational Substrate Theory.
