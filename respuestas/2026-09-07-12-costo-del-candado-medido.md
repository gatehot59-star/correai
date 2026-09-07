# Costo del candado bajo contencion: 10 ns por paquete, y en el caso compartido
# el candado sale MAS RAPIDO que no tenerlo

## Pedido

Medir el costo del candado bajo contencion.

## Herramientas y maquina

**Actions ubuntu-latest**, go 1.22.12, 4 vCPU, AMD EPYC 9V74, 15 GiB. Dos
instrumentos independientes: el cronometro de `go test -bench` y el **perfil de
mutex del runtime** (`Type: delay`), que contabiliza nanosegundos de bloqueo y no
tiempo de pared. `go` sigue AUSENTE en `brain-env` y en el sandbox (medido).

Rama `titan/bench-candado`, desde `titan/fix-d26-race`. Nada mergeado a `main`.

**26 guards, 26 PASS, `RESULTADO=VERDE`.** Commit `30835878`.

## El numero que se pidio

**Sin disputa (1 goroutine, que es el caso de todo agente con una sola conexion
abierta):**

```
con candado (serial)     69.53 ns/op
sin candado (serial)     59.50 ns/op
sobrecosto de 2 Lock/Unlock = 10.03 ns por paquete
```

Y el denominador, que es lo que convierte 10 ns en "caro" o "barato":
**`verifyPacketHMAC` cuesta 629 ns por paquete** (621,1 / 629,1 / 632,5 en tres
corridas). El candado es **1,6% de una sola** de las operaciones que el gateway
ya hace por paquete, sin contar TLS, decodificacion ni escritura.

## El caso real escala, y el runtime no ve contencion

Brazo C, un agente por goroutine con su propio candado, que es como el gateway
los reparte de verdad:

| goroutines | ns/op-real | Mop/s |
| --- | --- | --- |
| 1 | 74,83 | 13,4 |
| 2 | 60,14 | 16,6 |
| 4 | 38,57 | 25,9 |
| 8 | 34,51 | 29,0 |
| 16 | 28,51 | 35,1 |

Escala como tiene que escalar. Y el perfil de mutex del brazo C, completo:

```
Showing nodes accounting for 322us, 100% of 322us total
     322us   100%   100%   runtime._LostContendedRuntimeLock
```

**322 microsegundos en 500.000 operaciones, y CERO atribuido a los filtros y
CERO a `sync.Mutex`.** Lo unico que aparece es `_LostContendedRuntimeLock`, que
es contencion interna del runtime (scheduler, allocator), no de mis candados. El
guard exige que ni `Filter).Update` ni `sync.(*Mutex)` aparezcan en este archivo,
y se cumple.

## El caso patologico si tiene contencion, y esta cuantificado

Brazo A, varias conexiones del **mismo certificado** golpeando un solo
`AgentState`. Ahi el candado se disputa:

| goroutines | ns/op-real | Mop/s |
| --- | --- | --- |
| 1 | 71,68 | 14,0 |
| 2 | 81,07 | 12,3 |
| 4 | 122,10 | 8,2 |
| 8 | 148,30 | 6,7 |
| 16 | 188,70 | 5,3 |

Y el runtime lo contabiliza:

```
Showing nodes accounting for 77.72ms, 100% of 77.72ms total
   74.63ms 96.03%  sync.(*Mutex).Unlock
         0   ...    (*CoherenceFilter).Update   49.21ms  63.32%
         0   ...    (*HuberFilter).Update       25.42ms  32.71%
```

**77,72 ms de bloqueo agregado en 500.000 operaciones**, o sea ~155 ns de espera
sumada por operacion con 8 goroutines. Los dos instrumentos coinciden en orden de
magnitud con el tiempo de pared (148,3 ns/op), que es la razon para creerles a
los dos.

Dos tercios del bloqueo son de `CoherenceFilter`, no de `Huber`. Tiene sentido:
su seccion critica recorre el vector de contexto dos veces.

## El hallazgo que refuta la intuicion

**Quitar el candado no acelera: en el caso compartido lo hace MAS LENTO.** El
brazo B es la misma aritmetica sin candado (y por lo tanto una carrera de datos
deliberada):

| goroutines | A con candado | B sin candado | quien gana |
| --- | --- | --- | --- |
| 1 | 71,68 | **61,22** | sin candado, por 10 ns |
| 2 | **81,07** | 128,20 | **con candado, 1,58x** |
| 4 | **122,10** | 175,70 | **con candado, 1,44x** |
| 8 | **148,30** | 171,50 | **con candado, 1,16x** |
| 16 | 188,70 | **176,40** | sin candado, por 7% |

El mecanismo: con N cores escribiendo las mismas lineas de cache sin coordinarse,
el trafico de coherencia cuesta mas que serializar. El candado agrupa las
escrituras en una sola linea caliente por vez.

**Y la inversion en g=16 la reporto porque esta ahi**: con 16 goroutines sobre 4
vCPU la ventaja se da vuelta y el candado pierde 7%. No tengo el mecanismo
medido para eso (hipotesis: 4 goroutines por core hacen que el costo de dormir y
despertar en el candado supere el ahorro de coherencia), asi que queda como NO
MEDIDO en vez de como explicacion.

Lo que esto **no** dice: que el brazo B sea una alternativa. Sus numeros son un
piso aritmetico calculado sobre estado corrupto. La comparacion sirve para una
sola cosa: descartar la idea de que el candado se paga en throughput.

## El control positivo: el arnes ve contencion cuando existe

Un "no hay contencion" es una afirmacion de ausencia, y una ausencia medida con
un arnes ciego es indistinguible de una ausencia real. Brazo D: los mismos N
agentes aislados pero con **un candado global**, la decision de diseno
equivocada.

```
candado por agente        34.15 ns/op    29.279 Mop/s
candado global           141.00 ns/op     7.092 Mop/s
factor global/porAgente = 4.13x (minimo exigido 1.20x)
```

Y en bloqueo contabilizado: **163,21 ms contra 322 us, un factor de 507x**. El
arnes distingue. Si no lo hiciera, el test falla y con el todo el veredicto.

## Por que el bench NO corre con -race, medido en vez de afirmado

El mismo brazo, en las dos condiciones:

```
sin_race=145.5  con_race=2452  FACTOR_RACE=16.9
```

Un benchmark bajo `-race` no mide el candado: mide el detector, 17 veces. El
veredicto tiene un guard numerico que exige factor >= 5x, asi que esta decision
tambien puede dar rojo.

## La consecuencia accionable

Los dos problemas del `AgentState` compartido son el **mismo** problema: la
contencion medida en el brazo A y la mezcla semantica de la EWMA que quedo
declarada abierta en el fix de D-26 aparecen **exactamente** cuando un
certificado abre varias conexiones simultaneas.

**Limitar conexiones concurrentes por certificado cierra las dos de una vez**, y
es mas barato que cualquier optimizacion del candado: con una conexion por
certificado el costo es 10 ns y la contencion es cero medida. Optimizar el
candado seria trabajar sobre el sintoma de un caso que ademas rompe la
semantica.

No lo implemente: es una decision de producto (que hace el gateway con la segunda
conexion, rechazarla o multiplexar) y no mia.

## Defecto propio del turno

**Mi guard busco una cadena que la herramienta no imprime, y produjo un ROJO
FALSO.** La primera corrida grepeaba `Total:` en la salida de `pprof -top`, que
en go 1.22 dice `Showing nodes accounting for X, 100% of X total`. El perfil
habia salido perfecto (96,97 ms / 253,75 us / 120,69 ms) y mi veredicto lo
declaro ausente.

Es el **gemelo exacto** del defecto del turno pasado: entonces un guard paso por
una cadena que no era del instrumento, ahora fallo por una cadena que el
instrumento no emite. Los dos son el mismo error, escribir el guard contra lo que
supongo que imprime la herramienta en vez de contra su salida real. Van cuatro
turnos seguidos con un defecto de la familia "el instrumento no es lo que creo
que es".

Y el segundo, que es estructural y ya es la tercera vez: **los tres perfiles
iban al mismo archivo**, asi que el guard "el brazo C no atribuye bloqueo a los
filtros" era imposible de cumplir porque el grep leia el brazo A. Un archivo por
brazo.

## Nota de comparabilidad

Las dos corridas de este turno cayeron en CPUs distintas del pool de Actions
(EPYC 7763 la primera, EPYC 9V74 la segunda). **Los numeros son comparables
DENTRO de una corrida, no entre corridas.** Todos los que cito arriba son de la
misma, commit `30835878`. Cualquier tabla que mezcle las dos es invalida.

## Evidencia cruda

Siete archivos en `verificacion/resultados-actions/`:

- `bench-candado.txt`: entorno, build, vet, las tablas y el veredicto 26/26
- `bench-candado-brazos.txt`: los 4 brazos x 5 niveles de concurrencia x 3 repeticiones
- `bench-candado-perfil-A.txt`, `-C.txt`, `-D.txt`: un perfil de mutex por brazo
- `bench-candado-controles.txt`: los dos tests que pueden fallar
- `bench-candado-vs-race.txt`: el factor 16,9x del detector
- `bench-candado-suite-corta.txt`: la suite de D-26 sigue verde y los tests de
  tiempo se saltean con `-short`

Instrumento: `gateway/candado_bench_test.go` (sha256 `404d6a31...`).
`gateway/filters.go` sin tocar en este PR (sha256 `2f277d71...`, el mismo del fix).

## NO MEDIDO

1. **La inversion en g=16.** El candado pierde 7% ahi y no tengo el mecanismo
   medido.
2. **Latencia de cola.** Todo lo de arriba es promedio. El p99 de la espera en el
   caso patologico no se midio, y para un gateway el p99 puede importar mas que
   la media.
3. **Cuantas conexiones simultaneas abre un agente real:** cero clientes reales,
   asi que no se si el caso patologico ocurre alguna vez. El brazo A mide un caso
   posible, no uno observado.
4. **4 vCPU no es un servidor.** En 32 o 64 cores el punto donde la contencion
   duele se corre, y en que direccion es NO MEDIDO.
5. **El costo con TLS y decodificacion adentro:** el denominador que use es solo
   el HMAC. El costo real por paquete es mayor, asi que el 1,6% es una **cota
   superior** del peso del candado.
6. **Contencion del shard map** (`agentShard.mu`): no se midio. Con 64 shards la
   hipotesis es que no importa, y es hipotesis sin numero.

--- METODO TITAN ---
Accion delicada: SI (workflow con contents: write, en rama, no en main)
Modo aplicado:   TITAN FULL
Rubrica:         93/95 -> 97,9/100
N/A declarados:  5 (DevOps: la entrega es un instrumento de medicion)
Review externo:  pedido a Copilot en el PR; sin hallazgos = NO MEDIDO, no aprobacion
Instrumento:     go test -bench (cronometro) + perfil de mutex del runtime
                 (bloqueo contabilizado), dos instrumentos independientes que
                 coinciden en orden de magnitud; 26 guards, control positivo con
                 candado global y guard numerico sobre el factor de -race
Maquina:         Actions x64, 4 vCPU AMD EPYC 9V74 (go AUSENTE en brain-env y en
                 el sandbox, medido)
Artefactos:      gateway/candado_bench_test.go +
                 .github/workflows/go-bench-candado.yml + 7 archivos en
                 verificacion/resultados-actions/ + este archivo + Doc de ClickUp
