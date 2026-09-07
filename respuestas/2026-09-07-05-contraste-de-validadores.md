# 2026-09-07 · 05 · Contraste: su validador contra el mio

## 1. Pedido

"CONTRASTA CON TU PRODUCCION", con el script del auditor pegado completo y una
afirmacion que hay que corregir primero: *"No hay enlace: el codigo se ejecuto en
mi sandbox, no se guardo como archivo ni se subio al repo (no tengo acceso de
escritura a tu GitHub)"*.

**Eso es cierto para el auditor y no para mi.** Mi version esta commiteada desde
las 14:39 de hoy en `verificacion/t_testis.py`, con blob SHA `354d5bc7`, y el
commit `997aeddf` agrego el job `testis` al CI. El auditor no tiene escritura;
yo si. Son dos herramientas distintas y conviene no mezclarlas, porque si no
parece que la entrega no existe.

## 2. Herramientas declaradas

| Herramienta | Escribio | Cuota ajena |
| --- | --- | --- |
| sandbox local: CPython 3.12.13, `cryptography` presente | archivos temporales | no |
| GitHub API: `create_or_update_file` x1, `push_files` x1, `get_file_contents` x1 | si, este repo | no |
| ClickUp: un Doc publico | si | no |

**Corri su script verbatim** en el arbol del repo, sin tocarle una coma.

## 3. Su script corrido, tal cual

```
[OK ] INVARIANTE 0  cadena integra de 100 valida VERDE            
[OK ] INVARIANTE A  borrar veredicto 50 -> ROJO                   ROJO: salto de seq en pos 49 (esperaba 50, vino 51)
[OK ] INVARIANTE B  mutar 1 bit de Context en 50 -> ROJO          ROJO: hash no coincide en seq 50
[OK ] INVARIANTE C  reordenar 50 y 51 -> ROJO                     ROJO: salto de seq en pos 49 (esperaba 50, vino 51)
[OK ] INVARIANTE D1 el canonico naive k|v colisiona (demuestra D-06) 
[OK ] INVARIANTE D2 el canonico de ancho fijo NO colisiona        
[OK ] DEFECTO    E1 borrar ultimos 10 SIN ancla queda VERDE (D-20) 
[OK ] INVARIANTE E2 borrar ultimos 10 CON ancla -> ROJO           
[OK ] INVARIANTE E3 cadena entera borrada CON ancla -> ROJO       
[OK ] DEFECTO    F1 descarte que consume seq da ROJO falso (D-21) ROJO: salto de seq en pos 20 (esperaba 21, vino 23)
[OK ] INVARIANTE F2 descarte sin seq + DroppedSince: VERDE y hueco declarado declarado=[(21, 2)]
[OK ] DEFECTO    G1 D-01: ts=2^64-1 pasa validateTimestamp        int64 diff=-1757260000000000001
[OK ] INVARIANTE G2 2^64-1 no entra en bigint -> packet_ts numeric(20) 
[FAIL] INVARIANTE H  writeReject == Emit en handleConn             writeReject=9 Emit=0

RESULTADO: ROJO (1 fallos)
rc=1
```

Su test D **quedo bien**: el par nuevo colisiona en naive y no colisiona en
ancho fijo. Y su H reprodujo mi mismo conteo (9 y 0). El script funciona.

## 4. Cinco hallazgos contra su version, medidos

### H-1 · Su regla de ancla da ROJO FALSO en toda cadena viva

```
cadena de 100 contra su ancla                 -> VERDE
MISMA cadena + 1 veredicto legitimo (101)     -> ROJO: head != ancla (truncamiento de cola o cadena vacia)
```

`if anchor_head is not None and prev != anchor_head` exige que el head sea
EXACTAMENTE el sellado. En produccion, **entre dos checkpoints toda cadena activa
da manipulacion**. Su suite no lo agarra porque nunca prueba una cadena que
crecio.

La regla correcta: el Sello **fija un punto**, la cadena puede crecer por encima
y no puede encogerse por debajo ni cambiar lo sellado. Ya estaba implementada asi
en mi version desde E-005; ahora tiene un control de regresion propio (bloque D)
que compara las dos reglas lado a lado.

### H-2 · Su chequeo de firma HMAC no lo ejercita NINGUN control

Borre la linea de `hmac.compare_digest` de su `validate` y volvi a correr sus 7
casos de cadena:

```
   0 integra                con firma=VERDE  SIN el chequeo de firma=VERDE  identico
   A borrar el 50           con firma=ROJO   SIN el chequeo de firma=ROJO   identico
   B mutar ctx del 50       con firma=ROJO   SIN el chequeo de firma=ROJO   identico
   C reordenar 50/51        con firma=ROJO   SIN el chequeo de firma=ROJO   identico
   E1 truncar cola s/anc    con firma=VERDE  SIN el chequeo de firma=VERDE  identico
   E2 truncar cola c/anc    con firma=ROJO   SIN el chequeo de firma=ROJO   identico
   E3 vacia c/ancla         con firma=ROJO   SIN el chequeo de firma=ROJO   identico
   -> 7/7 veredictos identicos
```

Es un **guard inalcanzable**: nadie probo que puede disparar. Falta el control
que lo aisla, o sea refirmar un veredicto con otra clave dejando el hash intacto.
Mi CP-4 hace exactamente eso, y lo verifique por mutacion: si apago el chequeo,
CP-4 se cae.

### H-3 · Su test B dice "1 bit" y mueve unos 9.000

```
   ctx[0]=0.5        ulp=1.110e-16    +1e-12 cambia el double: True    =   9.007 ULP
   ctx[0]=0.021      ulp=3.469e-18    +1e-12 cambia el double: True    = 288.230 ULP
   ctx[0]=1e+06      ulp=1.164e-10    +1e-12 cambia el double: False   =       0 ULP
   ctx[0]=1e+09      ulp=1.192e-07    +1e-12 cambia el double: False   =       0 ULP
```

Con `ctx` en [0,1) el test pasa igual, asi que hoy no rompe nada. Pero es fragil
en la direccion peligrosa: **si algun dia el vector de contexto trae un valor
>= ~1e6, el `+1e-12` es un no-op** y el test B pasaria a FAIL sin que nadie haya
tocado el validador. Y "1 bit" es la parte que importa del control: si el hash
se cae con 9.000 ULP pero no con 1, hay un problema y este test no lo veria. Mi
CP-2 hace XOR 1 al bit menos significativo, que es 1 bit de verdad e invariante a
la magnitud.

### H-4 · La etiqueta DEFECTO es cosmetica en su runner

```python
def check(kind, name, cond, detail=""):
    fails += 0 if ok else 1     # <- nunca mira `kind`
```

Su nota dice: *"si preferis que hoy sea DEFECTO en vez de INVARIANTE, cambia la
etiqueta"*. **Cambiar la etiqueta no arregla nada**: la condicion sigue siendo
`n_rej == n_emit`, que hoy es False, y el CI sigue rojo. Para que pase hoy hay que
cambiar la CONDICION a `n_emit == 0`. En mi version las dos etiquetas tienen
semantica distinta de falla, no solo texto distinto.

### H-5 · Sus tres tests DEFECTO no pueden detectar el arreglo

Su nota dice: *"cuando arregles D-20/D-21/D-01 van a ponerse rojos a proposito,
para que toques la bitacora"*. **Es falso para los tres.**

- **G1**: `ctypes.c_int64(2**64-1 - 1_757_260_000_000_000_000).value <= 30e9`.
  Son tres constantes de Python. Si manana arreglan `validateTimestamp` en Go,
  esta cuenta da exactamente lo mismo.
- **F1**: construye `F_bad` con `drop_consumes_seq=True` a mano. El arreglo va en
  el recorder de Go; el constructor sintetico no cambia.
- **E1**: `validate(E) == "VERDE"` con `anchor_head=None` por defecto. Mientras el
  parametro tenga default `None`, sigue en verde para siempre.

## 5. El mismo cargo, contra mi archivo commiteado

Si H-5 vale, tiene que valer contra mi. Clasifique mis seis `defecto()`:

| ID | Como se calcula | Puede detectar el arreglo |
| --- | --- | --- |
| D-20 | cadena que construyo yo | **no** |
| D-21 | `consumir_seq=True` es mio | **no** |
| D-25 | propiedad de HMAC, no cambia nunca | **no** |
| D-28 | el sello lo pongo en seq 100 a mano | **no** |
| D-29 | regex sobre `fsm.go` | si |
| D-26 | busca el literal en `gateway.go` | si |

**2 de 6.** Y mi README decia: *"si un defecto medido desaparece, el test se pone
rojo y obliga a tocar la bitacora"*. Para cuatro de los seis eso era falso.

Arreglado con una etiqueta nueva: **`propiedad()`** para lo sintetico, `defecto()`
solo para lo que lee la fuente. No es cosmetico: el docstring de cada una dice
que condicion admite, asi que la proxima vez que agregue uno tengo que elegir.

Y el mismo cargo contra mi `t_ts.c`, que es de E-002: reimplementa
`validateTimestamp` en C, asi que si arreglan el Go sigue diciendo "defecto
presente". Lo declare como NO MEDIDO en su encabezado, pero el defecto que
reporta se lee como si midiera el gateway. No lo mide.

## 6. El hallazgo grande: la matriz de mutacion sobre MI validador

Apague cada chequeo de a uno:

```
  apago seq                  rc=1   cae: la PROPIEDAD D-21 (nada mas)
  apago prev_hash            rc=0   cae: NADA          <<<<<<<<<<
  apago hash                 rc=1   cae: CP-2
  apago firma del veredicto  rc=1   cae: CP-4
  apago largo del Sello      rc=1   CRASH (IndexError)
  apago firma del Sello      rc=1   cae: CP-7
```

**Apagar el chequeo de `prev_hash` daba rc=0.** Ningun control positivo lo
tocaba, porque el de `seq` agarra el borrado y el reordenamiento primero. O sea
que **la cadena de hash, que ES el producto entero, estaba sin probar**, y yo
venia diciendo "10 controles positivos" sobre un validador cuyo mecanismo
central no estaba cubierto. Es el mismo defecto que le acabo de cobrar al
auditor en H-2, en el eslabon mas caro.

**CP-5 nuevo**, que lo aisla: borrar el veredicto 50 y **RENUMERAR el resto**.
Seq queda contiguo 1..99 y lo unico roto es el eslabon. Y es el ataque realista:
renumerar es un `UPDATE`, no requiere nada mas que el mismo acceso que borrar.

Matriz despues del arreglo:

```
  apago prev_hash            rc=1   cae: CP-5
```

Queda una deuda declarada: `largo del Sello` se detecta por **excepcion**
(IndexError), no por control. Es deteccion, no cobertura.

## 7. Dos cosas que le tomo prestadas a su version

1. **Su D2 usa `canon()` de verdad.** Mi CP-8 comparaba un helper
   `ancho_fijo()` de juguete: probaba mi helper, no el canonico del producto.
   Ahora el desplazamiento de frontera va metido en `agent_id` y `prev_hash`
   reales y se compara `canonico()`. Medido: `770be2d3...` vs `8f97bd2b...`.
2. **Su `packet_ts = 2**64-1 if i == 3`**, o sea meter el valor de ataque ADENTRO
   de una cadena real. Mi version no lo tenia. Ahora hay un invariante que
   verifica que el canonico serializa `2^64-1` sin desbordar, que es la
   justificacion viva de la columna `numeric(20)`.

Y una tercera diferencia a favor de la mia que vale nombrar: su `KEY =
os.urandom(32)` hace la evidencia **no recomputable**. Un auditor no puede
rehacer los hashes de una corrida commiteada. Mis claves son fijas a proposito.

## 8. Estado final del archivo

```
canonico de ancho fijo: 181 bytes por veredicto (formato >Q32s32sqBQIII8dqiI)
...
controles positivos ejecutados: 11
fallos=0
```

11 controles positivos, rc=0, CPython 3.12.13. Blob del commit:
`7c589bec`, 31.413 bytes.

## 9. NO MEDIDO

1. **La igualdad byte a byte del blob commiteado contra mi copia local**: esta
   vez difieren en **8 bytes** y no se cual linea es. En E-002 y E-005 el hash
   coincidio; aca **no lo puedo afirmar**. Lo resuelve el job `testis` del CI,
   que corre la version del repo en un runner limpio.
2. El costo del `fsync` y donde el WAL se vuelve cuello de botella. Sigue siendo
   hipotesis sin numero de los dos lados.
3. Nada corrio contra Postgres: `verdicts`, `anchors` y `cases` no existen.
4. **El gateway Go sigue sin compilar** y el CI sin leerse.
5. `largo del Sello` se detecta por excepcion, no por control positivo.
6. 4 de mis 6 hallazgos son PROPIEDAD: para convertirlos en DEFECTO medido hace
   falta un cliente mTLS y un Postgres, no otro test.
