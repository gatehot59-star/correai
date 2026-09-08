# Barrido de carreras en lo que `-short` tapa: cero mas, y tres defectos mios

## Pedido

"Busca otras carreras en tests que `-short` saltea."

Era un NO MEDIDO que yo mismo habia declarado dos turnos antes.

## Herramientas y maquina

**Actions ubuntu-latest**, go 1.22.12, 4 vCPU. Un workflow nuevo que commitea sus
seis archivos. **13 guards, 13 PASS, `RESULTADO=VERDE`**, commit `33289738`.

## El sujeto, enumerado en vez de recordado

No asumi que eran cuatro. El job los saca del fuente con `awk`, y despues
**controla el sujeto** contando los `SKIP` reales:

```
CANTIDAD_SALTEADOS=4        <- enumerado del fuente
SKIPS_OBSERVADOS=4          <- contados de go test -short

TestControlDeContencion_ElArnesVeLaDiferencia   (candado_bench_test.go)
TestControlPositivoDeCola_CandadoGlobal         (latencia_test.go)
TestCostoDelCandado_ContraElHMAC                (candado_bench_test.go)
TestLatenciaP99DeLaEspera                       (latencia_test.go)
```

Los dos numeros coinciden, asi que el sujeto es el correcto y no hay un quinto
test escondido.

## El resultado: cero carreras mas

| Medicion | Resultado |
| --- | --- |
| los 4 salteados, **x5 repeticiones**, `-cpu=4` | **0** `race detected`, **20/20 PASS** |
| la suite completa (`go test -race ./...`), **x3** | **0** `race detected`, 0 FAIL |
| exit de las dos | 0 y 0 |

Repeti a proposito: **el detector reporta lo que OBSERVA**, no prueba un teorema.
Mas corridas = mas chances de ver un interleaving malo.

## Y el control que hace que ese cero valga

Un "no encontre nada" es la afirmacion mas facil de falsear del mundo: alcanza
con que el arnes este ciego. Asi que **muto mi propio fix**: copio el paquete a
`/tmp`, revierto `sumideros[ranura&63]` a `sumideros[0]` (el bug exacto de hace
dos horas: las 8 goroutines escribiendo la misma ranura) y exijo que la suite de
ROJO.

```
la mutacion se aplico (lineas)        : 1
carreras detectadas al revertir el fix : 1
el test cayo                          : 1
```

**El arnes SI ve una carrera cuando la hay.** Sin este paso, los ceros de arriba
no significarian nada.

## Un hallazgo que cierra una clase entera, no una muestra

```
T_PARALLEL_HITS=0
```

**No hay un solo `t.Parallel()` en el repo.** Eso importa mas que las 8 corridas:
los tests de un paquete corren **secuenciales**, asi que un var de paquete
compartido **entre** tests no puede correr contra otro test. Es un argumento
estructural, no un muestreo.

## La superficie de estado compartido, enumerada y clasificada

Porque el bug del sumidero era exactamente esto, y buscarlo con el detector es
necesario pero no suficiente:

| Identificador | Donde | Veredicto |
| --- | --- | --- |
| `candadoGlobal sync.Mutex` | `candado_bench_test.go:50` | es un candado, no dato. Solo `Lock`/`Unlock` |
| `sumideros [64]ranuraSumidero` | `latencia_test.go:221` | una ranura por goroutine + padding. **Arreglado** |
| `contadorSinProteccion int` | `race_test.go:283` | carrera **deliberada**, y corre solo en el subproceso `armado` |
| `bufferPool`, `packetPool` | `gateway.go:34` (produccion) | `sync.Pool`, segura por contrato |
| `ackBytes`, `rejectBytes` | `gateway.go` (produccion) | solo se pasan a `conn.Write`, que no muta |

Tres vars de paquete en los tests y un bloque `var()` en produccion. **Toda la
superficie, no una muestra.**

### Dos observaciones de baja severidad que salen de esa tabla

1. **`ackBytes` y `rejectBytes` son `[]byte`, o sea MUTABLES.** Hoy nadie los
   escribe y `conn.Write` no los toca, asi que no hay carrera. Pero un
   `[]byte` de paquete es un misil sin seguro: cualquiera que en el futuro haga
   `rejectBytes[0] = ...` desde un handler crea una carrera con las otras
   conexiones. Con `const` no se podria.
2. **`candadoGlobal` lo comparten tres tests distintos** (el brazo D del
   benchmark, el control de contencion y el control positivo de cola). Con los
   tests secuenciales no es una carrera, pero esos tres tests **no son
   independientes por diseno**, y nada lo declara.

Ninguna de las dos es un defecto activo. Las anoto porque son la superficie por
donde volveria a entrar el mismo bug.

## Tres defectos propios, en el mismo turno, los tres en el aparato

**1. Mi guard leyo MI PROPIO echo. Octavo de la familia.** El grep sobre el
fuente dio vacio (no hay `t.Parallel()`), pero mi guard del veredicto grepeaba
`t\.Parallel()` en el **archivo completo**, donde estaba mi propio encabezado
`=== hay t.Parallel() en algun lado? ===`. Dijo `aparece=SI` y puso **ROJO FALSO**
sobre un barrido que era 9/9 limpio.

Tres veces ya, y en las dos direcciones:

| # | El guard buscaba | Que paso |
| --- | --- | --- |
| 1 | `DATA RACE` | conto un `echo` mio -> **verde falso** |
| 2 | `Total:` | cadena que `pprof -top` no emite -> **rojo falso** |
| 3 | `t.Parallel()` | conto un encabezado mio -> **rojo falso** |

**La regla que sale, y ya no es una anecdota: UN GUARD NO PUEDE BUSCAR UNA
PALABRA, TIENE QUE LEER UN NUMERO CALCULADO.** Un grep de presencia no distingue
el instrumento de la prosa que lo describe. Los 13 guards de la version final
comparan contadores (`T_PARALLEL_HITS`, `RACES_EN_PROCESO_*`, `MUTACION_*`) que
el paso calcula y el veredicto lee con `^CLAVE=[0-9]+`.

**2. El `cd` se filtro y los contadores fueron a la copia mutada.** El paso de la
mutacion hace `cd /tmp/mut/repo`, y el bloque siguiente escribia `$OUT`
**relativo**: `MUTACION_DETECTADA` y `MUTACION_HIZO_CAER` terminaron dentro de la
copia. El veredicto los leyo `NA` y dio el **segundo rojo falso**, sobre una
mutacion que si habia funcionado (el `race detected` y el `--- FAIL` estaban en
la salida cruda). **R2: rutas absolutas para la evidencia, y el `cd` en subshell.**

**3. El veredicto se acumulaba.** Usaba `>>` sin truncar, asi que el archivo
quedo con **dos veredictos contradictorios** de dos corridas distintas, uno arriba
del otro. Eso es peor que no tener veredicto: el que lo lea no sabe cual rige.
**R3: el veredicto se trunca.**

Van **diez** defectos propios en ocho turnos, y los diez en el instrumento. El
sujeto medido casi siempre resulto como la medicion decia.

## Evidencia cruda

Seis archivos en `verificacion/resultados-actions/`:

- `carreras-1-sujeto.txt`: enumeracion + control de SKIP (4 = 4)
- `carreras-2-vars-de-paquete.txt`: la superficie de estado compartido completa
- `carreras-3-salteados-con-race.txt`: los 4 x5, con sus contadores
- `carreras-4-suite-completa.txt`: `go test -race ./...` x3
- `carreras-5-mutacion.txt`: la mutacion y su deteccion
- `carreras-6-veredicto.txt`: 13/13

Instrumento: `.github/workflows/carreras-que-short-tapa.yml`.

## NO MEDIDO

1. **Los CUERPOS de los benchmarks no corrieron en este barrido.** `go test`
   compila los benchmarks pero no los ejecuta sin `-bench`, y los 4 brazos de
   `BenchmarkCandado` lanzan 8 goroutines sobre filtros compartidos. El turno
   anterior si corrio `go test -race -bench Candado/A...` y salio en 0, pero fue
   **una corrida y de un brazo**, no de los cuatro. Es el hueco mas concreto que
   queda.
2. **Cero es "no observada en 8 corridas", no "no existe".** El detector ve los
   caminos que se ejecutan; no hace analisis estatico.
3. **Ningun test toca el camino de red del gateway** (`handleConn`, el decoder),
   porque no hay cliente mTLS. Las carreras de ese camino siguen sin medirse por
   ausencia de sujeto, no por ausencia de instrumento.
4. **No corri `-race` en arm64** ni con `GOMAXPROCS` alto (8, 16), que es donde
   los interleavings raros aparecen mas.
5. **Las dos observaciones de baja severidad no las arregle**: son cambios de
   codigo (uno de produccion) y este turno era buscar, no corregir.

--- METODO TITAN ---
Accion delicada: SI (workflow con contents: write, en rama, no en main)
Modo aplicado:   TITAN FULL
Rubrica:         92/95 -> 96,8/100
N/A declarados:  5 (DevOps: la entrega es un barrido de diagnostico)
Review externo:  pedido a Copilot en el PR; sin hallazgos = NO MEDIDO
Instrumento:     go test -race con -count=5 (salteados) y -count=3 (suite),
                 -cpu=4; mutacion del propio fix sobre una copia como control
                 positivo; 13 guards que comparan numeros calculados, no palabras
Maquina:         Actions x64
Artefactos:      workflow + 6 archivos carreras-*.txt + este archivo + Doc
