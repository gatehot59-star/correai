# Anexo de bitacora · E-006

> Este anexo existe porque `02-BITACORA.md` paso los 12 KB y el chat se corta por
> tamano. La bitacora principal sigue siendo la fuente para E-001 a E-005.

---

## 2026-09-07 · E-006 · Contraste de validadores: 5 hallazgos contra el ajeno, 1 contra el mio, y el mio era el peor

**Quien pidio:** Abraham. **Literal:** "CONTRASTA CON TU PRODUCCION", con el
script del auditor pegado completo.

**Correccion previa de hecho:** el auditor abrio con "no hay enlace... no tengo
acceso de escritura a tu GitHub". Cierto para el, **falso para mi**: mi
validador estaba commiteado desde las 14:39 en `verificacion/t_testis.py`, blob
`354d5bc7`, y el commit `997aeddf` agrego el job `testis` al CI. Son dos
herramientas distintas.

**Su script corrido verbatim:** 13 OK y 1 FAIL (el H, que el mismo anticipo),
rc=1. Su test D quedo bien corregido y su conteo de `handleConn` reprodujo el
mio: 9 rechazos, 0 Emit.

**Cinco hallazgos contra su version, todos medidos:**

| ID | Hallazgo | Evidencia |
| --- | --- | --- |
| H-1 | su regla de ancla (`head != ancla`) da **ROJO FALSO** en toda cadena viva | cadena de 100 + 1 veredicto legitimo: `ROJO: head != ancla`. Entre dos checkpoints, todo agente activo daria manipulacion |
| H-2 | su chequeo de firma HMAC **no lo ejercita ningun control** | borre la linea y sus 7 casos dieron **7/7 veredictos identicos**. Guard inalcanzable |
| H-3 | su test B dice "1 bit" y mueve ~9.000 ULP | y con `ctx >= 1e6` el `+1e-12` es un **no-op**: el test pasaria a FAIL sin que nadie toque el validador |
| H-4 | la etiqueta DEFECTO es **cosmetica** en su runner | `fails += 0 if ok else 1` no mira `kind`. "Cambiar la etiqueta" no saca el CI del rojo: hay que cambiar la CONDICION |
| H-5 | sus tres tests DEFECTO **no pueden detectar el arreglo** | G1 son tres constantes de Python; F1 construye la politica mala a mano; E1 tiene `anchor=None` por default. Su nota afirmaba lo contrario |

**El mismo cargo contra mi archivo:** clasifique mis seis `defecto()` y **solo 2
de 6 leen la fuente** (D-26 y D-29). Los otros cuatro (D-20, D-21, D-25, D-28)
se calculan sobre cadenas que construyo yo, asi que arreglar el gateway no los
mueve. Y mi README prometia lo contrario. Arreglado con una etiqueta nueva,
`propiedad()`, con semantica de falla distinta y no solo texto distinto. Mismo
cargo contra `t_ts.c` de E-002.

**EL HALLAZGO GRANDE, y es contra mi:** corri una matriz de mutacion apagando
cada chequeo de mi validador de a uno.

```
  apago prev_hash    rc=0   cae: NADA
```

**El chequeo de `prev_hash` no estaba cubierto por ningun control positivo**,
porque el de `seq` agarra el borrado y el reordenamiento primero. O sea que la
**cadena de hash, que es el producto entero**, estaba sin probar, y yo venia
reportando "10 controles positivos" sobre un validador cuyo mecanismo central
nadie habia demostrado que funcione. Es exactamente el H-2 que le cobre al
auditor, en el eslabon mas caro del sistema.

**CP-5 nuevo** lo aisla: borrar el veredicto 50 y **RENUMERAR el resto**, asi
seq queda contiguo y lo unico roto es el eslabon. Es el ataque realista, porque
renumerar es un `UPDATE`. Post-arreglo: `apago prev_hash -> rc=1, cae CP-5`.

**Deuda declarada:** `largo del Sello` se detecta por excepcion (IndexError), no
por control positivo. Es deteccion, no cobertura.

**Dos mejoras que le tomo al auditor:**

1. Su D2 usa el `canon()` real. Mi CP-8 comparaba un helper de juguete: probaba
   mi helper, no el canonico del producto. Ahora la frontera corrida va en
   `agent_id` y `prev_hash` reales.
2. Su `packet_ts = 2**64-1` metido ADENTRO de una cadena real. Agregado como
   invariante: es la justificacion viva de la columna `numeric(20)`.

**Una diferencia a favor de la mia:** su `KEY = os.urandom(32)` hace la
evidencia **no recomputable**. Mis claves son fijas a proposito.

**Estado final:** 11 controles positivos, `fallos=0`, rc=0. Blob del commit
`7c589bec`, 31.413 bytes.

**NO MEDIDO, y uno es nuevo e incomodo:** esta vez **NO puedo afirmar la
igualdad byte a byte** entre mi copia local y el blob commiteado: difieren en 8
bytes y no se cual linea. En E-002 y E-005 el hash coincidio. Lo resuelve el job
`testis` del CI. Ademas: el `fsync` sin numero, cero Postgres, el gateway Go
**sigue sin compilar** y el CI sin leerse.

**Archivo de esta respuesta:**
`respuestas/2026-09-07-05-contraste-de-validadores.md`.
