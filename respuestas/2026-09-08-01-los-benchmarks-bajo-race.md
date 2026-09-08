# Los cuerpos de los benchmarks bajo `-race`: cero carreras en los filtros reales

## Pedido

"SIGUE." El hueco mas concreto que yo mismo habia dejado abierto en el barrido
anterior:

> **Los CUERPOS de los benchmarks no corrieron en este barrido.** `go test`
> compila los benchmarks pero no los ejecuta sin `-bench`, y los 4 brazos de
> `BenchmarkCandado` lanzan 8 goroutines sobre filtros compartidos. El PR #4 si
> corrio `-race -bench` sobre **un** brazo y salio en 0, pero fue una corrida y
> de un brazo.

## El hueco existia, medido

```
BENCH_EJECUTADOS_SIN_FLAG=0
```

`go test -race` sin `-bench` ejecuta **cero** benchmarks. Los compila y nada mas.
Asi que ese codigo concurrente nunca habia visto el detector completo.

## Herramientas y maquina

**Actions ubuntu-latest**, go 1.22.12, 4 vCPU. **17 guards, 17 PASS,
`RESULTADO=VERDE`**, commit `976567dd`.

## El resultado, brazo por brazo

| Brazo | sub-benchmarks | reportes del detector | cita `filters.go` |
| --- | --- | --- | --- |
| **A** 1 agente, candado por agente | 5 | **0** | **0** |
| **B** 1 agente, SIN candado (RACY) | 1 | **21** | control positivo |
| **C** N agentes aislados (caso real) | 5 | **0** | **0** |
| **D** N agentes, candado GLOBAL | 5 | **0** | **0** |
| HMAC (serial, de contraste) | 1 | **0** | |

**Cero carreras en los filtros reales**, en los tres brazos que los usan, con los
5 niveles de concurrencia cada uno (g=1,2,4,8,16).

## El control positivo era gratis y ya vivia en el arbol

Esta vez **no tuve que mutar nada**. El brazo B (`B_1agente_sinCandado_RACY`) es
una carrera deliberada: son los gemelos sin mutex que escribi como referencia de
costo en el PR #4. Bajo `-race` reporta **21 veces**.

Si B dejara de reportar, el arnes estaria ciego y los ceros de A, C, D y HMAC no
significarian nada. **El control positivo es el mismo experimento, no un
agregado.**

Y el brazo B corre **1 sub-benchmark de 5** porque el detector aborta el proceso
al primer reporte (`halt_on_error=1` es el default). Eso esta declarado en el
workflow para que no aparezca como anomalia.

## Por que contar en bloque enganaria

```
todos los benchmarks juntos -> 23 reportes
  lineas citando huberSinMutex / coherenciaSinMutex (los gemelos RACY) : 22 / 24
  lineas citando filters.go (los filtros REALES)                      : 0
```

Un lector que solo mire el **23** concluye que hay un defecto grave. El desglose
muestra que **todo** viene del control deliberado y que los filtros de produccion
aparecen **cero** veces.

Ese es el punto de metodo del turno: **un conteo agregado sobre un experimento
que contiene su propio control positivo es indistinguible de un hallazgo.**

## Y un guard nuevo que cierra una clase de silencio

El paso `0a` **valida los 7 workflows del repo con un parser** antes de correr
nada:

```
workflows encontrados: 7   |   rotos: 0
```

Existe por el defecto de abajo, y sirve para cualquier workflow futuro: un
workflow que no parsea **no falla, no existe**.

## Tres defectos propios

**1. Mi YAML no parseaba, y por eso el job NO CORRIO.** El commit anterior no
produjo evidencia. Mi primer instinto fue **esperar mas**. En vez de eso baje el
archivo y lo pase por un parser:

```
yaml.parser.ParserError: while parsing a block mapping
  in "w.yml", line 111, column 9
  expected <block end>, but found '<scalar>'
  in "w.yml", line 152, column 10
```

Un string multilinea con **menos indentacion que su bloque `run: |`**: YAML cerro
el bloque ahi. Cero jobs creados, y el archivo de resultados que quedo commiteado
seguia siendo **el de la corrida anterior, con sus 3 FAIL**. O sea que el rojo
que yo estaba leyendo era de un experimento viejo.

**Y lo peor es que esto ya estaba escrito en mi propio metodo:** *"un run que
falla con cero jobs creados no es un job que fallo: es YAML que no parseo"*. Lo
tenia y no lo aplique. Lo que lo resolvio fue **medir en vez de esperar**.

**2. Puse el valor ESPERADO de memoria.** El guard "cuantos sub-benchmarks corrio
cada brazo" esperaba `1`. Cada brazo tiene **cinco** niveles de concurrencia, asi
que son 5. Resultado: **3 FAIL sobre datos perfectos** (A, C y D habian dado 0
carreras y 0 citas de `filters.go`, que era exactamente lo que habia que medir).

Patron distinto de los tres anteriores: esos fueron guards que leian la cadena
equivocada. Este lee el numero correcto y lo compara contra un esperado que
invente. **El valor esperado de un guard tambien es una medicion.** Ahora se
deriva contando la lista de goroutines del propio archivo de test.

**3. REINCIDENCIA: compare reportes contra lineas.** La tabla del veredicto dice
"23 reportes, de esos 22 citan huberSinMutex". **Falso por unidades:** 23 es el
numero de reportes y 22 es el numero de **lineas** que contienen esa cadena. La
prueba de que son unidades distintas esta en el propio archivo:
`CITA_COHERENCIA_SIN_MUTEX=24`, o sea **mas lineas que reportes**, porque cada
reporte cita la funcion dos veces (el `Read` y el `Previous write`).

Es **el mismo defecto que cometi en el turno del primer test Go**, donde conte
frames y los llame reportes. La lectura correcta es la que sostiene el veredicto
igual: **0 lineas citan `filters.go`** y los dos gemelos aparecen. Pero la frase
"de esos 22" esta mal y queda corregida aca.

Van **trece defectos en nueve turnos**, y el sujeto medido volvio a estar bien.

## Evidencia cruda

Ocho archivos en `verificacion/resultados-actions/`:

- `bench-race-0a-yaml.txt`: los 7 workflows parseados, 0 rotos
- `bench-race-0-sujeto.txt`: enumeracion + el esperado derivado del fuente
- `bench-race-1-brazoA...` a `-4-brazoD...`: un archivo por brazo
- `bench-race-5-hmac.txt`: el contraste serial
- `bench-race-6-todos-juntos.txt`: el conteo de bloque, para mostrar que engana
- `bench-race-7-veredicto.txt`: 17/17

Instrumento: `.github/workflows/benchmarks-bajo-race.yml`.

## NO MEDIDO

1. **Cero sigue siendo "no observada", no "no existe".** El detector reporta lo
   que ve. Lo que si esta probado es que **ve**, porque el brazo B reporta 21.
2. **`-benchtime=20000x` es carga chica.** Bajo `-race` cada acceso se
   instrumenta, asi que subir la carga cuesta minutos de job. Con mas iteraciones
   la ventana de observacion crece, y eso no se probo.
3. **Una sola corrida por brazo.** El barrido anterior repitio x5 los tests; aca
   los benchmarks corrieron una vez cada uno.
4. **Los tiempos de esta corrida no sirven**: bajo `-race` el costo se multiplica
   ~17x (medido en el PR #4). Los numeros de rendimiento validos siguen siendo los
   de sin `-race`.
5. **Sigue sin haber cliente mTLS**, asi que `handleConn` y el decoder no tienen
   sujeto que los ejercite. Esa es la unica superficie concurrente del gateway que
   ningun test toca.
6. **`-race` en arm64 y con `GOMAXPROCS` alto:** sin medir.

--- METODO TITAN ---
Accion delicada: SI (workflow con contents: write, en rama, no en main)
Modo aplicado:   TITAN FULL
Rubrica:         91/95 -> 95,8/100
N/A declarados:  5 (DevOps: la entrega es un barrido de diagnostico)
Review externo:  pedido a Copilot en el PR; sin hallazgos = NO MEDIDO
Instrumento:     go test -race -bench brazo por brazo, con el brazo B (RACY) como
                 control positivo que ya vive en el arbol; parser de YAML sobre
                 los 7 workflows; 17 guards con esperados DERIVADOS del fuente
Maquina:         Actions x64, 4 vCPU
Artefactos:      workflow + 8 archivos bench-race-*.txt + este archivo + Doc
