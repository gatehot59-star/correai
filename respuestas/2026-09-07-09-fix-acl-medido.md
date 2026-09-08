# Fix del ACL de MQTT: escrito, medido 9/9 y con control por mutacion

## Pedido

Escribir el fix del ACL y medirlo en Actions.

## Herramientas y maquina

**Actions ubuntu-latest**, mosquitto 2.0.18, VM limpia. Dos workflows nuevos
que commitean su propio resultado. Rama `titan/fix-acl-mqtt`, nada mergeado a
`main`. Cero runtime ajeno.

## El fix, en cuatro lineas efectivas

```
pattern read  fleet/%u/#
pattern write fleet/%u/#
user fleet_server
topic readwrite fleet/#
```

Dos cambios y una eliminacion:

1. **`user %u` -> `pattern`.** mosquitto sustituye `%u` solo en lineas
   `pattern`. En `user` lo tomaba literal, y creaba un usuario llamado `%u`.
2. **El bloque `user anonymous` se elimina.** No denegaba: **concedia**
   `read #`/`write #`. El ACL de mosquitto es allowlist pura, no tiene `deny`.
3. `allow_anonymous false` queda documentado como cosa de `mosquitto.conf`.

blob viejo `37d2aec0` (sha256 `e8370c8d`) -> nuevo (sha256 `b1db1c48`).

## Qué se midió, y el número

**9 casos, 9 PASS, 0 FAIL. `RESULTADO=VERDE`.** Commit `d6e27496`.

```
T0 PASS  el broker arranca con este ACL                        VIVO
T1 PASS  probe publica en su propio subarbol fleet/probe/t1     ALLOWED   <- EL FIX
T2 PASS  probe publica en el subarbol de OTRO fleet/otro/t2     DENIED
T3 PASS  probe publica fuera del arbol fleet: libre/t3          DENIED
T4 PASS  fleet_server publica en fleet/t4 (control positivo)    ALLOWED
T5 PASS  usuario anonymous publica en libre/t5                  DENIED    <- cierra N-04
T6 PASS  OTA llega al nodo: probe recibe en fleet/probe/#       RECIBIDO  <- el producto
T7 PASS  aislamiento de lectura: probe NO recibe fleet/otro/#   VACIO
T8 PASS  a_b_c recibe fleet/a_b_c/# (D-45 sigue ABIERTO)        RECIBIDO
```

La linea que importa del log del broker, la que antes decia `rc135`:

```
Received PUBLISH from ... (d0, q1, r0, m1, 'fleet/probe/t1', ... (1 bytes))
Sending PUBACK to ... (m1, rc0)
...
Sending PUBLISH to ... 'fleet/probe/ota' ... (9 bytes)   <- la OTA llega al nodo
```

Y las tres denegaciones que **tienen que seguir existiendo**:
`'fleet/otro/t2'`, `'libre/t3'`, `'libre/t5'`, las tres con `rc135`.

## El control que hace que el verde valga: mutacion

Un verde solo cuenta si **el mismo script** podia dar rojo. Corri los 9 casos
identicos contra el `acl.conf` **viejo**, recuperado con
`git cat-file -p 37d2aec0` (no reescrito a mano). Commit `c8fbf6ed`:

```
T1 FAIL  probe publica en su propio subarbol   esperado=ALLOWED  obtenido=DENIED
T5 FAIL  anonymous publica en libre/t5         esperado=DENIED   obtenido=ALLOWED
T6 FAIL  OTA llega al nodo                     esperado=RECIBIDO obtenido=VACIO
T8 FAIL  a_b_c recibe su subarbol              esperado=RECIBIDO obtenido=VACIO
PASSES=5  FAILS=4  MUTACION=DETECTADA
```

**4 rojos con el ACL viejo, 0 con el nuevo, mismo instrumento y misma VM.** Eso
es lo que convierte "pasa" en "mide". Y T5 confirma en la direccion contraria
que el bloque `anonymous` concedia de verdad: con el viejo, `PUBACK rc0`
publicando en `libre/t5`.

## Dos decisiones del instrumento, aprendidas midiendo

1. **El exit code de `mosquitto_pub` no sirve:** devuelve 0 aunque el broker
   deniegue (medido: `EXIT_B=0` junto a `Denied PUBLISH`). La decision se lee
   del **log del broker**.
2. **El SUBACK tampoco:** en MQTT 3.1.1 mosquitto responde 0 tanto si concede
   como si va a filtrar la entrega. La lectura se mide por **entrega real**. Ese
   fue el defecto que invalido mi instrumento del turno anterior.

## Lo que este fix NO cierra

**D-45 sigue abierto, y T8 lo documenta en vez de taparlo.** `tenant a_b` +
`nodo c` y `tenant a` + `nodo b_c` producen el mismo `mqtt_username` `a_b_c`, y
un ACL no puede distinguir dos identidades que llegan con el mismo nombre. El
cierre esta en `fleet_manager.py:262` y son dos cosas: separador `/` en vez de
`_`, y `^[A-Za-z0-9-]+$` en `tenant_id` y `node_id`. No lo toque en este turno
porque cambia el `mqtt_topic_prefix` que se le devuelve al nodo y el formato de
credencial de la flota entera: eso se decide, no se cuela en un fix de ACL.

## Evidencia cruda

- `mqtt/acl.conf` (sha256 `b1db1c48...`)
- `verificacion/resultados-actions/acl-fix.txt` (commit `d6e27496`, 9/9 VERDE)
- `verificacion/resultados-actions/acl-fix-mutacion.txt` (commit `c8fbf6ed`, 4 rojos)
- `.github/workflows/acl-fix-medicion.yml` y `.github/workflows/acl-fix-mutacion.yml`

Los dos jobs **fallan** si el resultado no es el esperado: el de medicion exige
`RESULTADO=VERDE`, el de mutacion exige `MUTACION=DETECTADA`. No son jobs que
solo imprimen.

## NO MEDIDO

1. **El ACL no se probo sobre TLS ni con el broker real del deployment.** Aca
   corrio en `127.0.0.1` con `password_file`, no con certificados de flota.
2. **`mosquitto.conf` de produccion no existe en el repo.** `allow_anonymous
   false` esta documentado y no configurado en ningun archivo commiteado.
3. **Cero clientes reales:** ningun nodo de verdad se conecto nunca.
4. **Fleet Manager sigue sin ejecutarse:** que el broker acepte el topico no
   prueba que Fleet publique en el correcto.
5. **D-45**: abierto y medido como abierto.
6. Mosquitto 2.1+: NO MEDIDO. La semantica de `pattern` podria cambiar.

## Defecto propio de este turno

El "antes" de T1 se midio con el topico `fleet/probe/x` en el turno anterior y
el "despues" con `fleet/probe/t1`. Mismo esquema y misma regla, pero **no es
literalmente el mismo comando**. Lo que cierra el hueco es el job de mutacion,
que si corre el topico identico contra los dos ACL. Sin ese job, la comparacion
era desprolija.

--- METODO TITAN ---
Accion delicada: SI (frontera de confianza: allowlist de un broker + 2 workflows
                 con contents: write, en rama, no en main)
Modo aplicado:   TITAN FULL
Rubrica:         92/95 -> 96,8/100
N/A declarados:  5 (DevOps: la entrega es configuracion mas su instrumento)
Review externo:  pedido a Copilot en el PR; si no emite hallazgos es NO MEDIDO,
                 no aprobacion
Instrumento:     mosquitto 2.0.18 en Actions ubuntu-latest; 9 casos + control
                 por mutacion; logs del broker commiteados verbatim
Maquina:         Actions x64
Artefactos:      mqtt/acl.conf + 2 workflows + 2 archivos de resultados +
                 este archivo + Doc de ClickUp
