# p99 de la espera del candado: 258 ns en el caso real, 28.911 ns en el patologico

## Pedido

Medir la latencia p99 de la espera.

## Nota de proceso, primero

**La medicion corrio a las 20:52 UTC y quedo en verde, pero este archivo de
respuesta nunca se escribio:** el turno se corto entre el commit de la evidencia
y el cierre. Hubo medicion sin respuesta, que por el contrato de este repo es
media entrega. Los numeros de abajo no son nuevos, son los de esa corrida
(commit `b220430b`), leidos de los archivos y no de memoria.

## Herramientas y maquina

**Actions ubuntu-latest**, go 1.22.12, 4 vCPU **Intel Xeon 6973P-C**, 15 GiB.
Cuantiles sobre muestras preasignadas, sin asignaciones durante la medicion.
Tres corridas independientes mas un control positivo de cola.

**24 guards, 24 PASS, `RESULTADO=VERDE`.**

## El problema que decide si el numero vale

Cronometrar una operacion de ~10 ns con un reloj que cuesta decenas de
nanosegundos **es medir el reloj**. Asi que lo primero que hace el instrumento es
medir el par `time.Now()` y publicarlo como **piso de ruido**:

```
PISO: par time.Now() (g=8)   n=800000  media=41.3  p50=40  p90=44  p99=64  p99.9=81  max=17946
```

Y despues **cada distribucion se clasifica automaticamente** contra ese piso en
MEDIDO / NO MEDIDO / COTA SUPERIOR. La seccion RESPUESTA no puede imprimir un
numero sin su clasificacion al lado.

## El numero pedido

Corrida 1, con todo en nanosegundos:

| Sujeto | p99 | clasificacion |
| --- | --- | --- |
| **ESPERA, caso real** (1 candado por agente) | **258** | MEDIDO (4,0x sobre el piso) |
| **ESPERA, caso patologico** (1 candado, 8 goroutines) | **28.911** | MEDIDO (451,7x) |
| p99.9 patologico | 83.572 | |
| PAQUETE completo, caso real | 192 | MEDIDO (3,0x) |
| PAQUETE completo, caso patologico | 25.626 | MEDIDO (400,4x) |
| piso del reloj | 64 | |

Las tres corridas, para que nadie tome un p99 de una sola muestra en un runner
compartido:

| | corrida 1 | corrida 2 | corrida 3 |
| --- | --- | --- | --- |
| p99 espera, caso real | 258 | 203 | 248 |
| p99 espera, patologico | 28.911 | 28.767 | 28.036 |
| p99.9 patologico | 83.572 | 88.249 | 91.217 |
| p99 paquete, caso real | 192 | 434 | **160 (NO MEDIDO)** |
| piso del reloj | 64 | 57 | 81 |

**El patologico es estable en las tres: 28,0 a 28,9 microsegundos.** Ese numero
no es ruido de VM.

## Dos numeros que NO son mediciones, y los declaro

1. **`PAQUETE: 1 goroutine, sin disputa` -> p99 = 78 ns, NO MEDIDO.** Solo 1,22x
   sobre el piso. Es **cota superior**: la latencia no supera ~78 ns, pero su
   valor real esta tapado por el reloj.
2. **`p99 paquete, caso real` en la corrida 3 -> 160 ns, NO MEDIDO.** 1,98x sobre
   un piso que esa corrida midio en 81 ns. En las corridas 1 y 2 el mismo sujeto
   si supero el umbral (3,0x y 7,6x). O sea que **ese sujeto esta en el borde de
   lo que este instrumento puede resolver**, y depende de como respire el runner.

## Los maximos son INUTILIZABLES, con su razon medida

La tabla muestra `max=15814953` (15,8 ms) en el caso real y `max=17946` en el
**propio piso del reloj**. Un maximo de 17,9 microsegundos midiendo dos llamadas
a `time.Now()` no puede ser espera de candado: es **preempcion del scheduler y
robo de CPU de la VM**. Publicar un max de 10 ms como "cola del candado" habria
sido un error de atribucion, asi que los maximos quedan declarados inservibles y
el analisis se corta en p99.9.

## Control positivo de cola

Los mismos N agentes aislados contra N agentes con **un candado global**:

```
candado por agente   p50=97   p90=234    p99=456     p99.9=563
candado GLOBAL       p50=230  p90=1626   p99=42939   p99.9=116205
factor p99 global/porAgente = 94.2x   |   factor p99.9 = 206.4x
```

Y el guard interno de cada corrida: `p99 espera compartida / aislada` dio
**112,1x / 141,7x / 113,0x**. El arnes ve cola cuando existe.

## Lo que esto significa para el producto

En el caso real (una conexion por certificado) la cola es **258 ns en p99**,
contra 629 ns que ya cuesta el HMAC por paquete. **Invisible.**

En el caso patologico salta a **28,9 microsegundos, 112 veces peor**. Eso ya no
es invisible: es un agente que abre varias conexiones y se degrada solo, y ademas
es el mismo caso donde la EWMA se mezcla semanticamente.

**Tercera medicion consecutiva que apunta al mismo lugar:** contencion,
latencia de cola y mezcla semantica son la misma frontera. Limitar conexiones
concurrentes por certificado las cierra las tres.

## Defectos propios

**1. Mi GUARD 1 era de un lado solo, y casi publico el piso del reloj como dato.**
Comparaba contra el piso **solo** el caso patologico, que lo supera 400x, o sea
que no podia fallar nunca. El caso sin disputa daba p99=78 ns contra un piso de
64: mi veredicto lo iba a publicar como "el p99 de la espera" cuando era el
reloj. Un guard que solo puede confirmar lo que ya creo no es un guard. Corregido
con la clasificacion automatica por sujeto.

**2. La seccion de estabilidad no funciono y ningun guard lo noto.** El archivo
dice literalmente:

```
p99_patologico_por_corrida =
DISPERSION=NA (no se leyeron 3 valores)
```

Mi agregador no pudo parsear los tres valores de los tres archivos, imprimio `NA`
y **el veredicto siguio en 24/24 VERDE** porque no escribi ningun guard sobre esa
seccion. La dispersion que cito arriba (28.036 a 28.911) la calcule **a mano**
leyendo los tres archivos, no la calculo el instrumento. Es la quinta vez en
cinco turnos que el defecto esta en el instrumento y no en el sujeto.

## Evidencia cruda

- `gateway/latencia_test.go` (sha256 `3a2eb659...`)
- `verificacion/resultados-actions/latencia-p99.txt`: veredicto 24/24 y las tablas
- `latencia-p99-corrida-1.txt`, `-2.txt`, `-3.txt`: las tres corridas completas
- `latencia-p99-control.txt`: el control positivo de cola
- `latencia-p99-suite-corta.txt`: la suite de D-26 sigue verde con `-race`
- `.github/workflows/go-latencia-p99.yml`

`gateway/filters.go` sin tocar (sha256 `2f277d71...`, el mismo del fix de D-26).

## NO MEDIDO

1. **La dispersion automatica entre corridas** (el defecto 2). Los tres numeros
   estan; el calculo no.
2. **La espera se mide sobre `sync.Mutex` bajo el mismo patron de acceso, no en
   el callsite de `handleConn`.** Es un proxy declarado: la misma primitiva, no
   el mismo llamador.
3. **Nada punta a punta:** sin cliente mTLS no hay latencia de paquete real, solo
   la de los filtros.
4. **Cuantas conexiones abre un agente real:** cero clientes. El caso patologico
   es posible, no observado.
5. **4 vCPU no es un servidor**, y las tres CPUs de este dia fueron distintas
   (EPYC 7763, EPYC 9V74, Xeon 6973P-C). Comparable dentro de una corrida.
6. **p99 bajo carga sostenida:** esto son rafagas de 800.000 muestras, no una
   hora de trafico.

--- METODO TITAN ---
Accion delicada: NO
Modo aplicado:   TITAN FULL
Rubrica:         91/95 -> 95,8/100
N/A declarados:  5 (DevOps: la entrega es un instrumento de medicion)
Review externo:  ninguno pedido todavia; deuda declarada
Instrumento:     go test con cuantiles sobre muestras preasignadas + piso de
                 reloj medido y clasificacion automatica MEDIDO/NO MEDIDO por
                 sujeto; 3 corridas independientes + control positivo de cola
                 (94,2x en p99); 24 guards
Maquina:         Actions x64, 4 vCPU Intel Xeon 6973P-C
Artefactos:      gateway/latencia_test.go + workflow + 6 archivos en
                 verificacion/resultados-actions/ + este archivo + Doc de ClickUp
