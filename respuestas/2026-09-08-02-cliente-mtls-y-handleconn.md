# El gateway no procesa un solo paquete. Medido con el primer cliente del proyecto

## Pedido

Escribir el cliente mTLS y medir `handleConn`. Era el hueco mas grande que
quedaba: sin cliente, `handleConn`, el decoder, el anti-replay y
`verifyPacketHMAC` nunca habian sido ejercitados por nadie.

## Herramientas y maquina

**Actions ubuntu-latest**, go 1.22.12. Tres archivos de test nuevos: una PKI de
prueba, el codificador del `PerimeterPacket` escrito contra `decodePerimeterPacket`
campo por campo, y el cliente. **Cero cambios en codigo de produccion.**

## EL HALLAZGO

**El gateway rechaza el primer paquete de toda conexion, siempre.**

```
payload=8   B  espera=0s      -> RECHAZO
payload=8   B  espera=20ms    -> RECHAZO
payload=8   B  espera=100ms   -> RECHAZO
payload=8   B  espera=300ms   -> RECHAZO
payload=64  B  espera=0s      -> RECHAZO
payload=64  B  espera=300ms   -> RECHAZO
payload=64  B  espera=600ms   -> RECHAZO
payload=256 B  espera=0s      -> RECHAZO
payload=256 B  espera=1s      -> RECHAZO
payload=256 B  espera=1.9s    -> RECHAZO
payload=512 B  espera=1.9s    -> RECHAZO
```

**11 de 11.** Y como el rechazo hace `TriggerBlock` + `writeReject` + `return`,
ninguna conexion pasa nunca de un paquete. **HiperSec no puede atender a ningun
agente.** Nadie lo sabia porque nadie habia mandado un paquete.

### La causa, aislada por mutacion

Una sola linea de diferencia: agregarle `&& false` a la condicion del
`HuberFilter` (dejando la llamada al filtro viva).

| Sujeto | ACK en el barrido |
| --- | --- |
| gateway REAL | **0 de 11** |
| gateway sin el BLOQUEO del Huber | **11 de 11** |

Eso deja la causa sin ambiguedad. Y el mismo cambio hace que el test de
caracterizacion E0 **falle**, que es exactamente lo que un test de
caracterizacion tiene que hacer cuando el defecto no esta.

### El mecanismo, y por que mi primera hipotesis era falsa

Cuando el control del arnes fallo, reproduje la aritmetica del `HuberFilter`:
sobre un filtro recien creado `lastV=0`, asi que `derivative = v/dt`, y con dt
chico eso queda cuatro ordenes de magnitud sobre el umbral. **De ahi despeje una
prediccion con numero: bastaba con que el cliente pausara ~7,2 s por KB.**

**La medicion refuto esa prediccion en 4 de los 11 casos.** Ninguna pausa sirve, y
la razon esta en `handleConn`:

```go
lastSeen := time.Now()
for {
    now := time.Now()          // <- se captura ANTES de leer del socket
    ...
    io.ReadFull(conn, ...)     // <- ACA se espera el paquete del cliente
    ...
    dt := now.Sub(lastSeen).Seconds()
```

`dt` no mide el tiempo entre paquetes: mide el tiempo entre dos **inicios de
iteracion del loop del servidor**. En la primera iteracion `lastSeen` y `now` se
tomaron a microsegundos, con el `ReadFull` todavia por delante. **La pausa del
cliente no entra en dt: no hay nada que el cliente pueda hacer.**

Es D-48 (derivada en var/s contra un umbral en unidades de desvio) llevado al
limite. El auditor externo lo habia clasificado como "Alto" con la frase "bloquea
casi cualquier rafaga". La medicion dice que **bloquea el primer paquete de todo
agente**, siempre, y eso es un bloqueante de producto, no un defecto Alto.

## Lo que quedo medido en el camino real, por primera vez

Sobre el sujeto declarado (gateway **sin** el bloqueo del Huber), porque sobre el
gateway real son inalcanzables. **9 de 10 tests PASS, cero carreras.**

| Hallazgo | Estado | Como |
| --- | --- | --- |
| **el codificador es correcto** | VALIDADO | dos paquetes validos, dos ACK, en la misma conexion |
| **anti-replay** | MEDIDO | ACK, y el mismo timestamp otra vez -> rechazo |
| **D-46** | MEDIDO | forjado con HMAC roto y ts=now+29s envenena la marca; el paquete legitimo posterior es rechazado |
| **D-46, mecanismo** | CONFIRMADO | un ts POSTERIOR al envenenado si se acepta: es un salto de la marca, no un agente muerto |
| **D-47** | MEDIDO | `Context[0]` a 999999 y el ciphertext reemplazado entero, con la cabecera bien firmada -> **ACK** |
| **D-26 en el callsite** | EJERCITADO | dos conexiones TLS del mismo cert atravesando `handleConn` |
| **los 9 rechazos son 1 byte** | MEDIDO | 5 causas distintas, 5 veces `0xFF`, desde el cliente |

**El de los rechazos indistinguibles tiene una vuelta incomoda:** es la razon por
la que YO, auditando, no puedo atribuir un rechazo sin un ACK de referencia. El
agujero que Testis existe para llenar me mordio como auditor, y eso vale mas como
argumento de venta que el ADR.

### Y un hallazgo nuevo sobre conexiones concurrentes

```
MISMO cert (comparten AgentState) : ack=20  rechazo=1  conexiones cerradas=1
certs DISTINTOS (control)         : ack=40  rechazo=0  conexiones cerradas=0
```

La marca de agua anti-replay es **por agente**, asi que dos conexiones del mismo
certificado se pisan los timestamps: la que llega tarde cae en el chequeo `<=` y
arrastra un `TriggerBlock`. **El gateway ya castiga las conexiones concurrentes
del mismo cert, pero con bloqueos en vez de un rechazo limpio.**

Eso refuerza la recomendacion que tres mediciones anteriores ya senalaban:
**limitar conexiones concurrentes por certificado** no es una optimizacion, es
formalizar algo que el gateway ya hace mal por accidente.

## Defectos propios

**1. Mi prediccion aritmetica era falsa, y la medicion la refuto.** Despeje
`dt_min` correctamente del filtro y me equivoque en la premisa: supuse que el
cliente podia influir en ese `dt`. Queda escrita en el archivo con los 4 casos
marcados, porque borrarla haria que el proximo repita el razonamiento.

**2. Un `: ` en el nombre de un paso rompio el YAML.** Pero esta vez **el guard
que escribi anoche lo agarro antes de gastar un ciclo de CI**: pase el archivo por
un parser antes de esperar nada. El turno pasado el mismo error me costo una
corrida completa y un rojo falso leido de un experimento viejo.

**3. Y el peor, porque es una CONCLUSION MIA que mi propio control refuto.** E9
afirma en su log: *"cero reportes = el mutex de filters.go cubre el camino real"*.
El control de carreras (sacarle los candados a los filtros) dio **0 reportes**, y
la razon esta en su propia tabla: sin candados el caso compartido logro solo **3
ACK** contra 20. O sea que **el anti-replay mata las conexiones concurrentes antes
de que lleguen a los filtros lo suficiente como para que el detector observe algo**.

Asi que la frase de E9 esta mal: el cero no prueba que el mutex cubra el camino
real, prueba que **casi no hay trafico concurrente que llegue a los filtros**. El
mutex sigue siendo correcto (el proxy dio 27 reportes sin el), pero esa afirmacion
especifica queda **NO MEDIDA** y hay que corregir el texto del test.

Son **16 defectos en 10 turnos**, y este es el primero que no esta en el
instrumento sino en una conclusion.

## Evidencia cruda

- `gateway/cliente_mtls_test.go`: PKI, codificador y cliente
- `gateway/handleconn_e2e_test.go`: E1 a E9
- `gateway/huber_frontera_e2e_test.go`: E0, el barrido, y el helper de NO MEDIDO
- `verificacion/resultados-actions/e2e-0-entorno.txt` a `e2e-5-veredicto.txt`
- `.github/workflows/handleconn-e2e.yml`

El veredicto dio **24 de 29 guards en PASS**. Los 5 rojos son los dos huecos
declarados abajo, no mediciones fallidas.

## NO MEDIDO

1. **El control de carreras no tiene poder** (defecto 3). Para que lo tenga hay
   que desactivar tambien el anti-replay, y ahi el sujeto ya son tres mutaciones
   encimadas: cada una lo aleja mas del producto.
2. **Mi guard "pasaron los 10 declarados" es incorrecto para el sujeto 2**: sobre
   un gateway sin el defecto, E0 **debe** fallar. Es R4 otra vez (el valor
   esperado tambien es una medicion) en una forma nueva: el esperado depende del
   sujeto.
3. **Cual es el fix del Huber.** Reemplazarlo por Page-Hinkley (D-48), inicializar
   `lastV` con la primera muestra, o mover el `now` despues del `ReadFull` son tres
   arreglos con consecuencias distintas. Es diseno de producto.
4. **El cliente es de test, no un SDK.** Vive en `_test.go`, asi que no se puede
   entregar a un cliente real ni usar para un demo.
5. **`Run()` no se puede apagar**: no hay `Shutdown` en el gateway, asi que cada
   test deja su goroutine y su listener vivos. Es un hueco del producto.
6. **Un solo agente por conexion, dos conexiones como maximo.** Nada se probo con
   decenas de agentes ni con trafico sostenido.
7. **Nada de esto toco Postgres ni Testis en Go**, que siguen sin existir.

--- METODO TITAN ---
Accion delicada: SI (workflow con contents: write, en rama, no en main)
Modo aplicado:   TITAN FULL
Rubrica:         92/95 -> 96,8/100
N/A declarados:  5 (DevOps: la entrega es un arnes de medicion)
Review externo:  pedido a Copilot en el PR; sin hallazgos = NO MEDIDO
Instrumento:     primer cliente mTLS del proyecto contra el gateway real; barrido
                 de 11 casos payload x espera; mutacion del bloqueo del Huber como
                 control positivo (0/11 ACK con el, 11/11 sin el); 29 guards que
                 comparan numeros calculados
Maquina:         Actions x64
Artefactos:      gateway/cliente_mtls_test.go + gateway/handleconn_e2e_test.go +
                 gateway/huber_frontera_e2e_test.go +
                 .github/workflows/handleconn-e2e.yml + 6 archivos e2e-*.txt +
                 este archivo + Doc de ClickUp
