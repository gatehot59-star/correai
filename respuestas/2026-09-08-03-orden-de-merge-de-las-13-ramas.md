# Orden de merge de las 13 ramas de KAMPE IR

**Turno:** 2026-09-08 · **Maquina:** brain-env (taller) para la topologia, Actions (fabrica) para todo veredicto de compilador.
**Pedido literal:** decidir el orden de merge de las 13 ramas. Esto es O-01: una prioridad es una entrega y se audita.

---

## 1. El hallazgo que cambia el pedido: no son 13 merges, son 3

Nueve de las trece ramas **ya son ancestros** de `titan/fix-huber-primer-paquete`. No hay que mergearlas: mergear la punta las arrastra. Medido con `rev-list --count PUNTA..rama` (0 = contenida):

| rama | ahead de main | contenida en la punta |
|---|---|---|
| test-go-race | 9 | SI |
| fix-d26-race | 16 | SI |
| bench-candado | 22 | SI |
| latencia-p99 | 31 | SI |
| log-del-porton-rojo | 37 | SI |
| carreras-que-short-tapa | 45 | SI |
| benchmarks-bajo-race | 52 | SI |
| cliente-mtls-e2e | 71 | SI |
| **fix-huber-primer-paquete** | **81** | (es la punta) |
| auditoria-de-la-auditoria | 12 | NO |
| fix-acl-mqtt | 18 | NO |
| hnsw-public-benchmark | 4 | NO |
| porton-de-go-short | 30 | NO (1 commit propio) |

Y `auditoria-de-la-auditoria` **esta contenida en `fix-acl-mqtt`**, con control de asimetria:

```
merge-base --is-ancestor auditoria fix-acl   -> 0   (es ancestro)
merge-base --is-ancestor fix-acl auditoria   -> 1   (CONTROL: no es simetrico)
```

Sin ese control en reverso, un 0 podria significar "son la misma rama" y no mediria nada.

**Conclusion: 10 + 2 + 1 = 13.** Tres grupos, y el cuarto item no es un merge sino un cierre.

## 2. El orden decidido

**1o - Grupo A: la cadena Go completa (10 ramas), via `titan/fix-huber-primer-paquete` (PR #14).**
Va primero porque **es lo que hace que el producto exista**: sin el fix E0 el gateway no procesa un solo paquete (0/11). Todo lo demas del repo es medicion sobre un gateway que no funcionaba.

**2o - Grupo B: `titan/fix-acl-mqtt` (PR #1), que arrastra `auditoria-de-la-auditoria`.**
Segundo y no primero porque toca `mqtt/acl.conf`, cero archivos de Go: no puede romper ni ser roto por el grupo A. Su valor es seguridad (D-44), no habilitacion.

**3o - Grupo C: `titan/hnsw-public-benchmark` (PR #6).**
Ultimo porque es el unico que **no toca produccion**: solo agrega `bench/hnsw_public.py`, su JSON de resultados y una respuesta. Riesgo minimo, valor de merge minimo.

**4o - `titan/porton-de-go-short` (PR #8): CERRAR sin mergear.** Ver seccion 4.

## 3. Por que el orden entre grupos es libre pero este es el mejor

Los tres grupos **no comparten ni un archivo**. El grupo A toca `gateway/*`, workflows `go-*` y `verificacion/resultados-actions/*`; el B toca `mqtt/acl.conf`, `CONTEXTO-KAMPE-IR.md` y workflows `acl-*`; el C solo `bench/*`. Ni `ci.yml` aparece en ninguno de los tres.

Asi que el orden **no lo dicta una dependencia tecnica**: lo dicta el criterio de poner primero lo que habilita el producto. Lo digo explicito porque una prioridad sin criterio nombrado es una opinion disfrazada.

### Los cuatro merges, medidos en seco

```
A punta de la cadena Go (10 ramas)             rc=0  conflictos=NINGUNO
B fix-acl-mqtt (arrastra auditoria)            rc=0  conflictos=NINGUNO
C hnsw-public-benchmark                        rc=0  conflictos=NINGUNO
D porton-de-go-short                           rc=0  conflictos=NINGUNO
```

### Y el arbol integrado completo pasa el porton

No alcanza con "cero conflictos de texto": eso mide que git puede juntarlos, no que el resultado compile. Asi que armé `titan/integracion-orden-de-merge` con los grupos A+B+C mergeados en este orden y le corri el CI de main:

**Corrida `34266874342`, los 6 jobs en `success`:** `hipersec (go build + vet + test -race)`, anti-replay, dualbrain, custos legis, fleet y testis.

Eso es lo que hace que este orden sea **medido y no razonado**.

## 4. PR #8 se cierra, y su premisa esta falsada

`porton-de-go-short` cambia `ci.yml` para correr `go test -race -short`, sacando del porton los 4 tests de tiempo de pared. Su justificacion, textual del commit `a40c880`:

> `hipersec (go build + vet + test -race)` esta en FAILURE en la PR #5 (52 s, job 101865710233) ... La PR #5 es la punta de toda la cadena, asi que mergearla sin esto pone main en rojo.

**Ese rojo ya no existe, y no lo arreglo #8: lo arreglo #10.** `log-del-porton-rojo` (ya ancestro de la punta) encontro la causa real: no era un guard fragil, era una carrera de datos propia. Su titulo lo dice: *"y con el fix el comando de main pasa SIN -short"*.

La medicion decisiva, con el instrumento en la configuracion que #8 declara imposible:

- El `ci.yml` de la punta corre `go test -race ./...` **sin `-short`**.
- Los 4 tests solo se saltean con `-short`: `latencia_test.go:310` y `:437`, `candado_bench_test.go:311` y `:368`. Y `testing.Short()=2` con `t.Skip=2` en cada archivo, o sea que **no hay ningun skip por otra causa** que pudiera estar tapandolos.
- Con esos 4 corriendo bajo `-race`: **3 de 3 corridas en success** (`34252841099`, `34266508969`, `34266526347`).

Tres repeticiones y no una, precisamente porque el argumento de #8 es fragilidad: un test flaky pasa una vez. Si alguna hubiera dado rojo, #8 tendria razon y este veredicto seria el opuesto.

**Mergear #8 hoy debilitaria el porton** (saca 4 tests del gate de cada PR) para arreglar un rojo que ya no esta. Lo que si vale rescatar de #8 es su job `go-tiempo-de-pared` por `workflow_dispatch`, pero eso es una mejora aparte, no un fix.

## 5. NO MEDIDO

1. **Que el merge real a `main` reproduzca el verde de `34266874342`.** Lo medido es la rama de integracion, no `main`. Son el mismo arbol pero no el mismo ref, y `main` puede tener protecciones o checks que la rama no dispara.
2. **Si los 4 tests de tiempo de pared son estables a largo plazo.** 3/3 es evidencia, no prueba. Un `-count=5` sobre esos 4 lo mediria mejor y todavia no corrio.
3. **Por que el push de la rama de integracion SI paso teniendo workflows nuevos** y el parche R6/R7 no. Hipotesis (no medida): el bloqueo mira si el blob del workflow es nuevo para el repo, y los de la integracion ya existian en otras ramas. Lo unico medido es que ese push salio en 0 y el del parche en 1.
4. **D-48 sigue abierto** y ningun merge de esta lista lo cierra.

## 6. Los comandos

```bash
git checkout main && git pull --ff-only

git merge --no-ff origin/titan/fix-huber-primer-paquete   # A: 10 ramas
git merge --no-ff origin/titan/fix-acl-mqtt               # B: + auditoria
git merge --no-ff origin/titan/hnsw-public-benchmark      # C

git push origin main
```

`--no-ff` a proposito: deja en el historial de `main` que entraron tres grupos y no 81 commits sueltos. Es la forma en que se midieron.

Despues: cerrar los PRs #2, #3, #4, #5, #10, #11, #12, #13 (quedan mergeados via #14) y **cerrar #8 con la explicacion de la seccion 4**.

**El merge es decision humana. Medir no es decidir.**

---

**--- METODO PROMETEO ---**
- **Maquinas:** brain-env para topologia y merges en seco; Actions (`34266874342`, `34252841099`, `34266508969`, `34266526347`) como unico testigo del compilador.
- **W-01:** cada numero de este archivo sale de un comando cuya salida esta arriba verbatim. El veredicto se deriva aparte.
- **Rama de integracion:** `titan/integracion-orden-de-merge`, para que cualquiera recompute el verde sin tocar `main`.

---

# CIERRE DEL TURNO: los tres merges entraron

Commiteado DESPUES del merge, porque hasta que `main` no se movio esto era una prediccion y no un hecho.

## Lo que quedo en `main`

```
c531d10 Grupo C: benchmark HNSW real sobre datasets UCI (#6)
72cae57 Grupo B: el fix del ACL de mosquitto (D-44), 9/9 con control por mutacion (#1)
9753834 Grupo A: la cadena Go entera (10 ramas) - fix E0, 0/11 a 11/11 (#14)
aa98aa0 (main anterior)
```

## El NO MEDIDO 1 quedo CERRADO

Decia: *"que el merge real a `main` reproduzca el verde de la rama de integracion"*. Ya no es una hipotesis. El push a `main` disparo `ci` y las tres corridas dieron `success`, la ultima con los 6 jobs verdes:

- `34298179010` main -> success
- `34298238259` main -> success
- `34298289157` main -> success, 6 jobs, **no-success=NINGUNO**

## Guards sobre el arbol de `main`, no sobre mi relato

```
llegada := time.Now()   -> 1    (NO 2: el duplicado que no compilaba no llego)
dt := llegada.Sub       -> 1
dt := now.Sub           -> 0    (cero residuo del bug)
if !f.visto {           -> 1
```

Reglas efectivas del ACL en `main`, descartando comentarios:

```
pattern read  fleet/%u/#
pattern write fleet/%u/#
user fleet_server
topic readwrite fleet/#
```

Cuatro lineas, y **ninguna concede a `anonymous`**.

## Un guard mio dio ROJO FALSO, y lo dejo escrito

Mi verificacion conto `grep -c 'user anonymous'` sobre `acl.conf` y dio **1**, esperando 0. Parecia que el bloque peligroso habia sobrevivido al merge.

**Era un comentario**, justamente el que explica por que se elimino: *"El bloque 'user anonymous' anterior no denegaba nada... CONCEDIA 'read #'/'write #'"*.

Es la **quinta vez** que un guard mio cuenta una palabra en vez de leer la estructura, y es exactamente lo que R1 prohibe. La version que si mide es la de arriba: filtrar comentarios y mirar las reglas efectivas. Lo anoto porque un rojo falso gasta el mismo tiempo ajeno que un verde falso.

## Contencion final de las 13 ramas

**12 de 13 contenidas en `main`.** La que falta es `titan/porton-de-go-short`, con 1 commit propio, y **es correcto que falte**: su PR (#8) se cerro falsado. Ese 12 de 13 es el guard de que el orden se ejecuto como se planifico.

Control: `is-ancestor main -> punta` da 1, o sea que `main` avanzo mas alla de la punta. Si diera 0, los merges no habrian agregado nada.

## Lo que sigue abierto, sin cambios

1. **El parche R6/R7 no esta aplicado.** Esta en `main` como archivo (`verificacion/parches/2026-09-08-workflows-R6-R7.patch`) pero los workflows siguen con el bug: el `sed` no es idempotente y el push del recibo puede perder una carrera en silencio. **No disparar `fix-huber-medicion` sobre ninguna rama hasta aplicarlo.** Bloqueado por el scope `workflow` del PAT.
2. **D-48 abierto:** el fix E0 cura el primer paquete y nada mas.
3. **D-45 abierto:** dos identidades distintas pueden dar el mismo username MQTT.
4. **Estabilidad a largo plazo de los 4 tests de tiempo de pared:** 3/3 es evidencia, no prueba. Si el porton parpadea, el #8 vuelve a la mesa.

--- METODO PROMETEO ---
Maquina: Actions como unico testigo de los merges y del verde de `main`; brain-env para los guards sobre el arbol. El merge fue decision humana.

## DEFECTO PROPIO DEL TURNO: el commit de esta bitacora salio con el mensaje de OTRO REPO

El primer intento de commitear este cierre quedo con el mensaje

> `feat(container): la app lee su propio proot.stderr.log y lo publica en pantalla`

que es de **MUDH-Mobile**, del 29-ago. Contenido correcto, etiqueta de otro proyecto.

**Causa, medida.** Escribi el mensaje en `/tmp/msg.txt` y commitee con `git commit -F /tmp/msg.txt`. El `printf` **fallo**:

```
sh: 1: cannot create /tmp/msg.txt: Permission denied
```

Ese archivo ya existia con `owner root` y fecha `Aug 29 21:41`, sobreviviente de otro turno: **brain-env es persistente**, y eso, que normalmente es la ventaja, esta vez fue la trampa. Mi `printf` no pudo sobreescribirlo, **pero `git commit -F` si pudo leerlo** y tomo el mensaje viejo. El `&&` de mi cadena no protegio nada porque el que fallo fue el `printf`, no el `git`.

**Variante nueva de un error viejo:** hasta ahora era *"mi guard lee un patron que el instrumento no emite"*. Aca fue **"mi comando leyo un archivo que otro turno dejo"**. La regla que faltaba: **un temporal se VERIFICA despues de escribirlo**, no se supone escrito porque el comando de escritura estaba en la linea.

### Y el historial se limpio, por decision humana

Los dos commits originales, `e5f2099` (con el mensaje errado) y `464b1f3` (con la correccion), **ya no existen en `main`**: se colapsaron en este unico commit y se hizo `push --force-with-lease`. Fue pedido explicitamente.

Lo dejo escrito porque **cambia el estado de la evidencia**: esos dos SHA no se pueden verificar mas contra el repo. Lo unico que sobrevive del defecto es **este texto**, y eso es justo lo que W-01 advierte: evidencia que no vive en un objeto verificable depende de que alguien la haya escrito bien. Reescribir historial es el caso limite de *"el operador no puede ser el unico testigo de su propio resultado"*.

El beneficio de limpiar: `main` queda legible. El costo: un auditor ya no puede **reproducir** el defecto, solo leerlo.
