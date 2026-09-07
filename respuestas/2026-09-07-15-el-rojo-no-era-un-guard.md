# No cae ningun guard: el rojo del porton era una carrera de datos MIA

## Pedido

"Baja el log del job rojo y decime que guard cae."

## Primero: el log no se puede bajar, y esta medido

No lo afirmo de memoria. Dos llamadas:

```
GET /repos/gatehot59-star/correai/actions/jobs/101865710233/logs   -> HTTP 403
GET https://github.com/.../actions/runs/34162053048/job/101865710233 -> HTTP 200,
   218.852 bytes, y CERO texto de log adentro (solo iconos y feature flags; el
   log lo carga el navegador por API autenticada)
```

**Pero un obstaculo de una herramienta no cierra la pregunta.** No baje el log:
**reproduje el comando exacto** en un job que commitea su propia salida, y aisle
los cuatro candidatos uno por uno. Queda mejor que el log, porque es recomputable
por cualquiera.

## La respuesta: NINGUN guard cae

```
=== QUIEN CAE ===
TestLatenciaP99DeLaEspera                       con -race: CAE
TestControlPositivoDeCola_CandadoGlobal         con -race: pasa
TestControlDeContencion_ElArnesVeLaDiferencia   con -race: pasa
TestCostoDelCandado_ContraElHMAC                con -race: pasa
```

Cae **uno solo**, y **sus tres guards pasan**:

```
GUARD 1  p99 patologico / p99 del piso = 47.2x (minimo exigido 3.0x)   PASA
GUARD 2  p99 espera compartida / aislada = 8.3x                        PASA
GUARD 3  el caso real contra el piso -> MEDIDO (5.7x sobre el piso)    PASA
```

La causa esta dos lineas mas abajo, en la salida cruda:

```
WARNING: DATA RACE
Write at 0x00000083ce80 by goroutine 32:
  gateway.trabajoSintetico()
      /home/runner/work/correai/correai/gateway/latencia_test.go:194
Previous write at 0x00000083ce80 by goroutine 35:
  gateway.trabajoSintetico()
      /home/runner/work/correai/correai/gateway/latencia_test.go:194
...
testing.go:1398: race detected during execution of test
--- FAIL: TestLatenciaP99DeLaEspera (5.48s)
```

**`var sumidero float64`: UNA variable de paquete escrita por las 8 goroutines
sin sincronizar.** Mia. En el archivo que certifica que **D-26, una carrera de
datos, esta cerrado**.

Y explica por que los guards se leian en verde: el detector marca el test como
fallido **despues** de que el test imprimio todo, incluida la seccion RESPUESTA.
El log completo y el fallo conviven.

## Las dos hipotesis anteriores quedan REFUTADAS

**La de Tao, y la mia que la repeti sin medirla.** Su Doc proponia dos
candidatos y los dos son falsos:

| Hipotesis | Medido | Veredicto |
| --- | --- | --- |
| GUARD 1: el piso del reloj se infla y tapa la senal | 47,2x con el minimo en 3,0x | **REFUTADA** |
| `factorMin = 1.2` del control de contencion | ese test **pasa** solo bajo `-race` | **REFUTADA** |

Los dos razonamientos eran plausibles y ninguno era el mecanismo. **Y el mio es
peor que el suyo**: yo tenia el codigo, el no.

## Y el piso SI se infla, pero al reves de lo que se supuso

El par de condiciones, del mismo test:

| | piso p99 | patologico p99 | factor GUARD 1 | caso real |
| --- | --- | --- | --- | --- |
| **sin `-race`** | 60 ns | 20.561 ns | 342,7x | NO MEDIDO (1,02x) |
| **con `-race`** | 90 ns | 4.247 ns | 47,2x | MEDIDO (5,7x) |

El factor cae de 342,7x a 47,2x, o sea que la intuicion apuntaba al lugar
correcto **y el numero seguia lejisimo del umbral**. Un razonamiento correcto
sobre el eje equivocado.

**Y hay una trampa nueva ahi:** bajo `-race` el caso real pasa a "MEDIDO 5,7x"
cuando sin `-race` es "NO MEDIDO 1,02x". Eso **no** es que el detector mejore la
resolucion: infla la operacion mas que el reloj y produce un **MEDIDO
artificial**. Los numeros que se citan tienen que ser siempre los de sin `-race`.
Quedo escrito en el encabezado del archivo.

## El fix, y cambia el sentido del PR #8

Un sumidero **por goroutine**, con padding de una linea de cache para no meter
false sharing dentro de la seccion critica que este archivo cronometra. **No
cambia lo medido**: el sumidero solo existe para que el compilador no elimine el
trabajo sintetico.

Y el resultado es mas fuerte de lo que esperaba. El **comando literal del
`ci.yml` de `main`**, sin `-short`:

```
=== go test -race ./...   (el comando literal de main, SIN -short) ===
ok  github.com/gatehot59-star/kampe-ir/gateway  16.222s
EXIT_igual_que_main=0
```

**El porton de `main` ya no necesita `-short` para estar verde.** Los cuatro
candidatos pasan bajo `-race`, cero reportes del detector, y los guards siguen
vivos (41,4x / 8,6x / MEDIDO 4,8x en la corrida post-fix).

Eso reordena el merge:

- **PR #8 (`-short`) era el desbloqueo correcto con la informacion que habia**, y
  sigue teniendo valor propio: separar los tests de tiempo de pared del porton es
  buena idea igual, porque medirlos bajo `-race` no mide lo que dicen medir.
- **Pero lo que tapaba no era un guard fragil: era una carrera de datos real.** Si
  se mergea solo el `-short`, la carrera se queda en el arbol, silenciosa, en el
  archivo que certifica que las carreras estan cerradas.
- **Los dos van, y este primero.**

## Defecto propio, el septimo del dia

**Septimo turno, septimo defecto, y el septimo en el instrumento.** Pero este
tiene una vuelta que los otros seis no: el instrumento que mide la ausencia de
carreras **tenia una carrera**. No es ironia, es la consecuencia directa de una
cosa que ya sabia y no aplique: `race_test.go` corre sus reproductores en
subproceso justo para que el detector no marque el test; `latencia_test.go`
corre 8 goroutines en proceso y yo nunca le pase el detector, porque mi propio
workflow lo corria con `-short` y `-short` lo saltea.

**Mi porton no era el porton.** Y esta vez el numero no salio raro: el arbol
quedo con una carrera adentro durante dos horas, con cuatro PRs encima.

## Evidencia cruda

Cinco archivos en `verificacion/resultados-actions/`, los dos estados:

- `porton-rojo-1-igual-que-main.txt`: el comando literal de `main`. Antes:
  `EXIT=1` con `--- FAIL`. Despues: `ok ... 16.222s`, `EXIT=0`.
- `porton-rojo-2-latencia-p99.txt`: el test aislado. Antes: 1 reporte del
  detector + `race detected`. Despues: **0 reportes**, `--- PASS`.
- `porton-rojo-3-sin-race.txt`: los 4 candidatos sin `-race`, 4/4 PASS en los dos
  estados.
- `porton-rojo-4-mecanismo.txt`: la tabla del piso en las dos condiciones.
- `porton-rojo-5-veredicto.txt`: el conteo por test.

sha256 de la corrida post-fix: `ae590cc7...` (paso 1) y `e1d4d7fa...` (paso 2).
Instrumento: `.github/workflows/porton-rojo-quien-cae.yml`.

## NO MEDIDO

1. **Si `main` queda verde al mergear.** Lo medido es que el comando de `main`
   pasa sobre **esta rama**. El `main` post-merge es otro arbol.
2. **Si hay otras carreras en los tests que `-short` saltea.** Los cuatro
   candidatos pasan hoy; no audite el resto de los archivos de test con esa lente.
3. **El false sharing entre ranuras del sumidero:** puse el padding por
   construccion, no lo medi. Si estuviera mal, inflaria la seccion critica y los
   numeros de espera serian pesimistas, no optimistas.
4. **Los cuantiles post-fix no reemplazan a los publicados.** Los numeros que se
   citan siguen siendo los de sin `-race` de la corrida de las 20:52.

--- METODO TITAN ---
Accion delicada: SI (workflow con contents: write, en rama, no en main)
Modo aplicado:   TITAN FULL
Rubrica:         93/95 -> 97,9/100
N/A declarados:  5 (DevOps: la entrega es un diagnostico mas su fix de test)
Review externo:  Tao fue el instrumento ajeno que encontro el rojo; su hipotesis
                 de la causa queda refutada con numero
Instrumento:     go test -race aislado por test en Actions ubuntu-latest, salida
                 cruda commiteada; el comando literal de main como control de
                 cierre (antes EXIT=1, despues EXIT=0)
Maquina:         Actions x64
Artefactos:      gateway/latencia_test.go + workflow + 5 archivos
                 porton-rojo-*.txt + este archivo + Doc de ClickUp
