# REPORTE TÉCNICO PARA AUDITORÍA EXTERNA

**Proyecto:** CORREAI / KAMPE IR · **Repo:** `gatehot59-star/correai` (público)
**Auditor destinatario:** Fable 5.1 dentro de Abacus.AI
**Fecha del turno:** 2026-09-08 · **`main` cierra en:** `2d68e53`
**Autor del trabajo auditado:** BRAIN (agente). El humano responsable es Jorge Abraham Mendieta.

---

## 0. Qué se te pide, auditor

No se te pide aprobar. Se te pide **intentar falsar**. Este documento está armado para que recomputes cada número **sin pedirnos nada**: todo lo que se afirma tiene su comando al lado y todo lo que no se midió está declarado como tal.

Si encontrás una afirmación sin comando que la reproduzca, **ese es un hallazgo válido contra nosotros**, y queremos que lo digas así.

Tres estados en todo este documento: **MEDIDO**, **REFUTADO** y **NO MEDIDO**. No usamos "probablemente" ni "debería".

### Cómo empezar

```bash
git clone https://github.com/gatehot59-star/correai
cd correai
git log --oneline --first-parent -6
```

Esperado exacto:

```
2d68e53 bitacora: el orden de merge de las 13 ramas, su ejecucion y los defectos del turno
c531d10 Grupo C: benchmark HNSW real sobre datasets UCI (#6)
72cae57 Grupo B: el fix del ACL de mosquitto (D-44), 9/9 con control por mutacion (#1)
9753834 Grupo A: la cadena Go entera (10 ramas) - fix E0, el gateway pasa de 0/11 a 11/11 (#14)
429d943 parche: los dos cambios de workflow que el PAT del taller no puede pushear
ef2f124 evidencia: los 4 brazos del fix del Huber, con el contador corregido
```

---

## 1. El hallazgo central del turno

**El gateway no procesaba un solo paquete.** De 11 casos de un barrido de payload y espera, **0 terminaban en ACK**. No era degradación: era falla total del camino de datos.

Dos causas, y el experimento de 4 brazos dice cuánto pesa cada una:

1. **`gateway/filters.go`** — la primera muestra de toda conexión se comparaba contra un estado vacío del HuberFilter y salía anomalía. El fix: la primera muestra **inicializa** el filtro y devuelve `false`.
2. **`gateway/gateway.go`** — `dt` y el reloj del anti-replay se calculaban con un `now` capturado **antes** del `io.ReadFull`, o sea antes de que el paquete existiera. El fix: `llegada := time.Now()` **después** de leer el socket. `now` se sigue usando para `FSM.Allow` y el read deadline, que sí deben mirar el inicio de la iteración.

### El experimento de 4 brazos (recibo `ef2f124`)

Archivo: `verificacion/resultados-actions/brazos-2-veredicto.txt` · Corrida Actions `34242363279`, conclusion `success`. N=11 por brazo, y **el N lo reporta el propio test**, no un `grep` del lector.

| brazo | filters_fix | gateway_fix | ACK | RECHAZO |
|---|---|---|---|---|
| A | sí | sí | **11** | 0 |
| B | sí | no | **11** | 0 |
| C | no | sí | **4** | 7 |
| D | no | no | **0** | 11 |

`FALLAS_DE_VEREDICTO=0` · `RESULTADO=VERDE` · `BRAZO_C_COINCIDE=1`

**Por qué este diseño es falsable y no decorativo:**

- **D es control negativo.** Sin ninguno de los dos fixes reproduce el hallazgo original exacto (0/11). Si D hubiera dado ACK, el experimento completo no mediría nada y el fix no tendría contra qué compararse.
- **C vindica una aritmética que habíamos declarado refutada.** Predecía **4** ACK sobre 11 y dio 4. Conclusión: el diagnóstico sobre el filtro nunca estuvo mal; lo que estaba mal era la atribución de **quién calculaba `dt`**. Una refutación se convirtió en diagnóstico.
- **El sujeto se cuenta en cada copia**, no se supone: brazo A `visto=1 llegada=1`, brazo D `visto=0 llegada=0`. Esto responde "¿cada brazo midió el árbol que dice medir?".
- **El brazo C no tiene guard de igualdad, a propósito.** Su valor era lo que queríamos *averiguar*. Ponerle un guard sería exigirle a la medición que confirme lo que ya creíamos.

**Verificalo:**

```bash
cat verificacion/resultados-actions/brazos-2-veredicto.txt
cat verificacion/resultados-actions/brazos-1-medicion.txt   # la salida cruda
```

---

## 2. Estado de `main`, medido sobre el árbol

No sobre nuestro relato. Cada línea es un comando.

```bash
git show main:gateway/gateway.go | grep -c 'llegada := time.Now()'   # esperado 1
git show main:gateway/gateway.go | grep -c 'dt := llegada.Sub'       # esperado 1
git show main:gateway/gateway.go | grep -c 'dt := now.Sub'           # esperado 0
git show main:gateway/filters.go | grep -c 'if !f.visto {'           # esperado 1
```

Los cuatro dieron lo esperado. **El tercero es el que importa**: cero residuo del bug.

El ACL de mosquitto, reglas **efectivas** (descartando comentarios):

```bash
git show main:mqtt/acl.conf | grep -vE '^\s*#' | grep -vE '^\s*$'
```

```
pattern read  fleet/%u/#
pattern write fleet/%u/#
user fleet_server
topic readwrite fleet/#
```

Cuatro líneas, ninguna concede a `anonymous`.

### El CI, que es el único testigo del compilador

En el taller donde se escribió el código **no hay toolchain de Go** (`go: not found`, medido). Así que ningún verde de este informe lo firma el autor: lo firma GitHub Actions en VM limpia bajando el repo de cero.

| corrida | ref | jobs | conclusión |
|---|---|---|---|
| `34242363279` | rama del fix | — | success (4 brazos) |
| `34266874342` | rama de integración A+B+C | 6 | success |
| `34298179010` | `main` | 6 | success |
| `34298238259` | `main` | 6 | success |
| `34298289157` | `main` | 6 | success |
| `34300933301` | `main` (post force-push) | 6 | success |
| `34300950661` | `main` (post force-push) | 6 | success |

Jobs: `go build + vet + test -race`, anti-replay en C, dualbrain en C++, custos legis, fleet, testis.

---

## 3. El orden de merge, y por qué 13 ramas fueron 3 merges

Había 13 ramas `titan/` con cero merges a `main`. **Nueve ya eran ancestros** de `titan/fix-huber-primer-paquete`, y `titan/auditoria-de-la-auditoria` vivía dentro de `titan/fix-acl-mqtt`. 10 + 2 + 1 = 13.

```bash
for b in test-go-race fix-d26-race bench-candado latencia-p99 log-del-porton-rojo \
         carreras-que-short-tapa benchmarks-bajo-race cliente-mtls-e2e; do
  echo "$b -> $(git rev-list --count origin/titan/fix-huber-primer-paquete..origin/titan/$b)"
done   # los ocho esperados en 0
```

**Control de asimetría, y es lo que hace que el 0 signifique algo:**

```bash
git merge-base --is-ancestor origin/titan/auditoria-de-la-auditoria origin/titan/fix-acl-mqtt; echo $?  # 0
git merge-base --is-ancestor origin/titan/fix-acl-mqtt origin/titan/auditoria-de-la-auditoria; echo $?  # 1
```

Sin el segundo, un 0 podría significar "son la misma rama" y no mediría nada.

**El orden, con criterio nombrado** (los tres grupos no comparten ni un archivo, así que no había dependencia técnica: el orden lo dicta un criterio, y por eso se declara):

1. **Grupo A**, cadena Go (`9753834`) — primero porque **es lo que hace que el producto exista**.
2. **Grupo B**, ACL (`72cae57`) — cero archivos de Go, no puede romper ni ser roto por A.
3. **Grupo C**, HNSW (`c531d10`) — el único que no toca producción.

**El orden está medido, no razonado:** se armó `titan/integracion-orden-de-merge` con A+B+C en ese orden y se le corrió el CI antes de tocar `main` (corrida `34266874342`, 6 jobs success). *Cero conflictos de texto solo prueba que git puede juntarlos, no que el resultado compile.*

Contención final:

```bash
for b in $(git branch -r | grep 'origin/titan/'); do
  git merge-base --is-ancestor $b origin/main && echo "DENTRO $b" || echo "FUERA $b"
done
```

**12 de 13 dentro.** La que queda fuera es `titan/porton-de-go-short` y es correcto que quede: su PR se cerró falsado (§5).

---

## 4. Los 5 defectos propios del turno

Esta sección existe porque un informe sin ella no es auditable. Ninguno de estos lo encontró un revisor: los encontró el instrumento, y dos de ellos casi se pasan por alto.

### D-1 · Un `sed` no idempotente publicó un `gateway.go` que NO COMPILABA

`fix-huber-medicion.yml` aplica el fix por `sed` y después commitea `gateway.go`. Corrió **dos veces** sobre la misma rama (`43ab1f0`, `27287be`). Su guard cuenta `>=1`, no `==1`, así que la segunda pasada insertó el bloque de nuevo y **el guard siguió en verde**.

El head publicado tenía `llegada := time.Now()` declarado dos veces:

```
gateway/gateway.go:278:11: no new variables on left side of :=
```

Corregido en `588c962`.

**La lección no es "faltaba un guard": el guard del veredicto SÍ cazó el duplicado** (`esperado=1 obtenido=2`). Lo que falló es que el paso de commit tenía `if: always()` y **publicó el archivo roto de todos modos**. Un guard cuyo veredicto nadie obedece no es un guard.

Por eso los recibos `huber-*.txt` de la rama están **ROJOS y son obsoletos** (miden `44e0b49`, con el duplicado). Se dejaron sin editar a propósito: son la evidencia del defecto. **El recibo vigente son los `brazos-*.txt`.**

### D-2 · Una carrera de push se comió un recibo, en silencio

Corrida `34227120895`: midió los 4 brazos, emitió el veredicto y su paso `Commitear` **falló**. `fix-huber-medicion` había arrancado en el mismo push (los dos con `created_at 2026-09-08T12:37:23Z`) y este perdió la carrera.

Estado correcto de ese momento: **medición ejecutada, recibo NO commiteado**, o sea **NO MEDIDO**, no verde. Una medición cuyo resultado no persiste no es un resultado.

### D-3 · Un guard propio dio ROJO FALSO contando un comentario

Al verificar `main` se contó `grep -c 'user anonymous'` sobre `acl.conf`: dio **1**, esperando 0. Parecía que el bloque peligroso había sobrevivido al merge.

**Era un comentario**, justamente el que explica por qué se eliminó el bloque. La versión que sí mide es la de §2: filtrar comentarios y leer reglas efectivas.

Es la **quinta vez** en este proyecto que un guard cuenta una palabra en vez de leer estructura (las anteriores: `DATA RACE` matcheando el propio `echo`; `Total:` que `pprof` no imprime; `t.Parallel()` matcheando el propio encabezado; `-> ACK` con un espacio de menos). Un rojo falso gasta el mismo tiempo ajeno que un verde falso.

### D-4 · Un commit salió con el mensaje de OTRO REPO

El mensaje se escribió en `/tmp/msg.txt` y se commiteó con `git commit -F`. El `printf` **falló**:

```
sh: 1: cannot create /tmp/msg.txt: Permission denied
```

Ese archivo ya existía, `owner root`, del 29-ago: sobreviviente de otro turno en un taller **persistente**. El `printf` no pudo sobreescribirlo **pero `git commit -F` sí pudo leerlo** y tomó el mensaje viejo (de MUDH-Mobile). El `&&` de la cadena no protegió nada porque el que falló fue el `printf`, no el `git`.

Variante nueva de un error viejo: antes era *"mi guard lee un patrón que el instrumento no emite"*; acá fue *"mi comando leyó un archivo que otro turno dejó"*.

### D-5 · Dos SHA de este turno YA NO EXISTEN

A pedido explícito del humano se limpió el historial: `e5f2099` (el del mensaje errado) y `464b1f3` (su corrección) se colapsaron en `2d68e53` con `push --force-with-lease`.

**Consecuencia para vos, auditor:** el defecto D-4 **se puede leer pero no reproducir**. Esos dos objetos no están en el repo. La única evidencia que sobrevive es el texto de la bitácora, escrito por el mismo que cometió el error.

Eso es exactamente el caso límite de W-01 (*el operador no puede ser el único testigo de su propio resultado*). Lo declaramos porque **es el punto más débil de este informe** y preferimos que lo señales sobre nuestra propia admisión que sobre tu descubrimiento.

Guards que corrieron **antes** del force-push, cualquiera de los cuales lo habría abortado: la punta remota tenía que ser exactamente `464b1f3`; el padre, el merge del grupo C; el resultado, **1** commit; el mensaje, sin la palabra `proot`; y el árbol podía diferir **solo en el archivo de bitácora**. Los cinco pasaron. Se usó `--force-with-lease`, no `--force`.

---

## 5. Una decisión que revierte un PR previo: #8 cerrado y falsado

El PR #8 proponía cambiar `ci.yml` a `go test -race -short`, sacando del portón de cada PR los 4 tests de tiempo de pared. Su argumento: ese portón **no puede** correrlos bajo `-race`.

**Falsado.** El rojo que #8 quería esquivar lo había arreglado el #10 en la causa real: **no era un guard frágil, era una carrera de datos propia.**

Medición con el instrumento en la configuración que #8 declara imposible:

- El `ci.yml` de la punta corre `go test -race ./...` **sin `-short`**.
- Los 4 tests se saltean **solo** con `-short` (`latencia_test.go:310` y `:437`, `candado_bench_test.go:311` y `:368`), y `testing.Short()=2` con `t.Skip=2` por archivo: **ningún skip por otra causa** los estaba tapando.
- Con los 4 corriendo bajo `-race`: **3 de 3 corridas en success** (`34252841099`, `34266508969`, `34266526347`).

**Tres repeticiones y no una, porque el argumento de #8 era fragilidad y un test flaky pasa una vez.** Si alguna hubiera dado rojo, #8 tenía razón y el veredicto sería el opuesto. La medición podía falsarnos.

Mergear #8 habría **debilitado el portón** de forma permanente para arreglar un rojo que ya no existía. Lo que sí vale rescatar de #8 (su job `go-tiempo-de-pared` por `workflow_dispatch`) queda como mejora aparte.

---

## 6. Los 6 estados NO MEDIDO

Ninguno de estos es un "pendiente menor". Están acá porque **sin ellos este informe reclamaría más de lo que midió**.

1. **El `sed` de `fix-huber-medicion.yml` sigue NO siendo idempotente.** El fix está escrito, validado con `yaml.safe_load`, y **NO aplicado**: el PAT disponible no tiene scope `workflow`. Vive en `verificacion/parches/2026-09-08-workflows-R6-R7.patch` (commit `429d943`). **Consecuencia operativa: no disparar ese workflow sobre ninguna rama hasta aplicarlo**, o vuelve a publicar un `gateway.go` que no compila. Verificá con `git apply --check` y su reverso.
2. **La carrera de push (D-2) sigue abierta**, mismo parche, mismo bloqueo.
3. **D-48 abierto.** El fix E0 cura **el primer paquete y nada más**: con payloads variables el gateway sigue bloqueando. Estaba medido en `E0c` de la suite E2E, y esa suite hoy no corre en verde por el punto 1, así que **su número actual no está medido sobre este head**.
4. **D-45 abierto.** Dos identidades MQTT distintas pueden producir el mismo username (`tenant a_b`+`nodo c` y `tenant a`+`nodo b_c` → `a_b_c`). Ningún ACL distingue dos identidades con el mismo nombre. Cerrarlo cambia el formato de credencial de la flota entera: es decisión, no fix.
5. **Estabilidad a largo plazo de los 4 tests de tiempo de pared.** 3/3 es evidencia, no prueba. Un `-count=5` lo mediría mejor y no corrió. Si el portón parpadea, #8 vuelve a la mesa.
6. **El ACL no se probó sobre TLS ni contra el broker del deployment real**, `mosquitto.conf` de producción no existe en el repo (`allow_anonymous false` está documentado y **no** configurado), y mosquitto 2.1+ es NO MEDIDO.

---

## 7. Seis verificaciones adversariales, para que no nos creas

Cada una **puede dar rojo**. Si alguna lo hace, es un hallazgo contra nosotros.

**V-1 · ¿El control negativo realmente reproduce el hallazgo?**
```bash
grep 'BRAZO_D_ACK' verificacion/resultados-actions/brazos-1-medicion.txt   # tiene que ser 0
```
Si D no da 0, el experimento de 4 brazos no mide nada y el §1 se cae.

**V-2 · ¿El `N` lo produce el instrumento o el lector?**
Leé el paso `1 - los cuatro brazos` de `.github/workflows/huber-cuatro-brazos.yml`. El `N` sale de `de N casos` en la salida del test. Si encontrás un esperado hardcodeado, es un hallazgo.

**V-3 · ¿Queda residuo del bug en `main`?**
```bash
git show main:gateway/gateway.go | grep -n 'now.Sub\|nowNs := uint64(now'   # esperado: vacío
```

**V-4 · ¿El verde de `main` es de `main` o de una rama?**
Pedí los **check runs** del commit `2d68e53`, no el estado combinado. En este repo el combinado devuelve `total_count: 0` sobre 6 jobs verdes, y leer solo eso produce un "acá no hay checks" falso. Ese detalle ya nos hizo tropezar antes.

**V-5 · ¿El parche pendiente aplica de verdad?**
```bash
git apply --check verificacion/parches/2026-09-08-workflows-R6-R7.patch            # esperado 0
git apply --check --reverse verificacion/parches/2026-09-08-workflows-R6-R7.patch  # esperado != 0
```
El reverso es lo que hace que el 0 signifique algo: sin él, un 0 podría ser "ya estaba aplicado".

**V-6 · Control positivo por mutación en el ACL.**
```bash
cat verificacion/resultados-actions/acl-fix.txt            # 9 PASS, 0 FAIL
cat verificacion/resultados-actions/acl-fix-mutacion.txt   # 4 FAIL, MUTACION=DETECTADA
```
Mismo script, misma VM, ACL viejo → 4 rojos. Sin ese segundo archivo el verde no vale.

---

## 8. Dos límites del instrumento que descubrimos midiendo

Útiles para cualquiera que audite este repo:

1. **`mosquitto_pub` sale con código 0 aunque el broker deniegue**, y el `SUBACK` es 0 tanto si concede como si va a filtrar. Por eso las publicaciones se juzgan por el **log del broker** y las lecturas por **entrega real**. Un instrumento que mira exit codes acá mide ruido.
2. **Ningún agente puede leer el texto de un log de Actions** (403 incluso en repo público). Los runs y los steps sí: dicen **dónde** murió, no **por qué**. De ahí la regla de este repo: **todo workflow commitea su propio resultado**, o es invisible. Es también la razón por la que D-2 fue tan caro.

---

## 9. Autoevaluación, y por qué no ponemos un número alto

**Lo fuerte:** todo verde de este turno lo firma un instrumento ajeno; hay control negativo (brazo D), control positivo por mutación (ACL), control de asimetría (contención de ramas) y control en reverso (el parche); una predicción propia quedó vindicada por medición y otra decisión previa quedó **revertida** por medición; y la evidencia cruda está commiteada verbatim, no resumida.

**Lo débil, sin adornos:**

- Un defecto estuvo **publicado**: el head tenía un archivo que no compilaba, y el guard que lo detectó fue ignorado por un `if: always()`.
- **Cinco** guards propios en la historia del proyecto contaron palabras en vez de leer estructura. Es un patrón, no un accidente.
- **Dos SHA de este turno ya no existen.** Un tramo de la evidencia dejó de ser reproducible por una decisión de higiene del historial.
- Los tres NO MEDIDO más importantes (§6.1, §6.2, §6.3) **están bloqueados por una credencial**, no por una dificultad técnica. Eso significa que el riesgo sigue vivo mientras nadie aplique el parche.

**No reclamamos un score.** Un número lo pone el auditor, no el auditado. Lo que reclamamos es que este documento sea **falsable**: si algo de acá no se puede recomputar con los comandos dados, decilo y tenés razón.

---

**--- METODO PROMETEO ---**

- **Máquinas:** `brain-env` (taller persistente, sin toolchain de Go) para escribir parches, topología y guards sobre el árbol; **GitHub Actions** (VM limpia) como **único** testigo de todo compilador y de todo verde citado.
- **W-01:** evidencia cruda commiteada verbatim en `verificacion/resultados-actions/`; veredictos derivados aparte. Excepción declarada y no disimulada: el defecto D-4/D-5, cuyos objetos fueron borrados del historial.
- **Todo merge y el reescrito del historial fueron decisiones humanas.** Medir no es decidir.
- **Bitácora del turno:** `respuestas/2026-09-08-03-orden-de-merge-de-las-13-ramas.md`
