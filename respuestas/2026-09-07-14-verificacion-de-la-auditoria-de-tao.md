# Verifico la auditoria de Tao: le doy razon en las cuatro duras, y una es un
# defecto mio que bloquea el merge

## Pedido

"Revisa que te dejo Tao."

## Que dejo

Tao es un **super agente de ClickUp**, no un agente de `nexus.db` (ahi lo busque
primero y no hay una sola fila suya: cero mensajes, cero reportes). Lo que dejo
esta en ClickUp y en el repo:

1. Un Doc: *"[TITAN] KAMPE IR (correai) - Auditoria integra 2026-09-07 y orden de
   merge de las 7 ramas"*.
2. **PR #6**, para `titan/hnsw-public-benchmark`: la rama que yo deje verde y
   **sin PR**.
3. **PR #8** con un fix ya verificado por Actions.
4. **Issue #7** (el porton rojo) e **issue #9** (cerrar el contexto tras el merge).

## Las cuatro afirmaciones duras, verificadas por mi

No le crei de palabra. Cada una con la llamada que la mide:

### 1. El porton de Go de `main` esta ROJO en la punta de la cadena. CONFIRMADO

`get_check_runs` del PR #5, seis jobs:

```
hipersec (go build + vet + test -race)                  failure   21:08:58 -> 21:09:50  (52 s)
dualbrain                                               success
hipersec anti-replay                                    success
testis                                                  success
audit + fleet                                           success
custos legis                                            success
```

Y el PR #4, el de abajo en la cadena: **6/6 success**. La biseccion es limpia y
el unico Go que #5 agrega es `gateway/latencia_test.go`.

### 2. La causa: el `ci.yml` de `main` corre sin `-short`. CONFIRMADO verbatim

```yaml
      - run: go test -race ./...
```

**Y aca esta MI defecto.** Mis workflows de rama corren `go test -short -race` y
yo lo llame *"la suite de correctitud"*. El porton compartido de `main` hace lo
contrario: mete bajo `-race` exactamente los cuatro tests de tiempo de pared que
**yo mismo** declare invalidos de medir bajo `-race`, con el factor **16,9x**
medido y commiteado en `bench-candado-vs-race.txt`.

O sea que medi con mi propio porton y nunca abri el compartido. **Sexto defecto
del dia y la sexta vez que esta en el instrumento**, no en el sujeto. Y el peor
de los seis por consecuencia: los otros cinco daban un numero raro, este pone
`main` en rojo el dia del merge.

### 3. Su fix pasa de rojo a verde con el mismo arbol. CONFIRMADO

PR #8, base `titan/latencia-p99` a proposito, o sea corriendo sobre el arbol que
hoy falla. Es una falsacion, no una confirmacion:

```
hipersec (go build + vet + test -race -short)            success   (36 s)
hipersec tiempo de pared (bench y cuantiles, SIN -race)  skipped
los otros 5                                              success
```

**Rojo -> verde, un flag de diferencia.** Y los cuatro tests de tiempo de pared
no se borran: pasan a un job con nombre propio, sin `-race`, a pedido.

### 4. El estado combinado dice "no hay checks" sobre 6 jobs verdes. CONFIRMADO

Mismo commit `2e5fbcbd`, dos instrumentos:

```
check runs      -> total_count: 6, seis success
estado combinado -> state: pending, total_count: 0, statuses: []
```

Un lector que consulte lo segundo concluye "aca no hay gate" **con tono seguro** y
mergea. En este repo hay que pedir siempre check runs.

## Y me corrige un archivo mio, con razon

Mi `ESTADO-KAMPE-IR.md` dice que `titan/fix-acl-mqtt` y
`titan/auditoria-de-la-auditoria` *"estan por fuera de esa cadena y van a
necesitar merge aparte"*. **Es falso, y el que se equivoco fui yo.** Cree
`fix-acl-mqtt` **desde** `auditoria-de-la-auditoria`, asi que la PR #1 ya arrastra
los tres workflows de auditoria, `t_engine_audit.{cpp,out}`, la respuesta 08 y la
reescritura del contexto.

**No hacen falta 7 merges. Hacen falta 3:** PR #1 (hilo del ACL, ya trae la
auditoria), PR #6 (HNSW) y la cadena de Go, con el PR #8 adentro primero.
Corregido en el archivo, en este mismo commit.

## Donde le pongo un limite

Su Doc dice que **NO MEDIDO cual de los dos guards cae** bajo `-race` (el margen
de 3x sobre el piso del reloj, o el `factorMin = 1.2` de contencion), porque el
texto del log de Actions no lo puede leer ningun agente. **Eso lo declara el, y
es correcto declararlo.** Pero tiene una consecuencia que su Doc no saca:

**`-short` saca el guard del porton; no lo arregla.** Si el guard es fragil bajo
el detector, sigue fragil, y el dia que alguien corra la suite completa sin
`-short` va a volver el mismo rojo sin entender por que. El issue #7 lo tiene
abierto; **el fix es un desbloqueo, no un cierre**, y hay que decirlo con esas
palabras antes de mergear.

Y una cosa mas, chica: su fix renombra el job (`test -race` -> `test -race
-short`). Un rename deja huerfano cualquier *required status check*. Hoy no
rompe nada porque verifique que `main` esta `protected: false`, pero eso es
suerte, no diseno.

## Que NO verifique de su auditoria

Sus hallazgos BAJO (A-04 gofmt sin porton, A-06 workflows de rama entrando a
`main` con `contents: write`, A-07 dos corridas de CPU distinta en el arbol,
A-08 la bitacora partida) **no los medi uno por uno**. Los cuatro coinciden con
lo que yo mismo deje escrito, asi que no los contradigo, pero tampoco los firmo
como verificados por mi.

## NO MEDIDO

1. **Cual de los dos guards cae bajo `-race`.** Hace falta el texto del log.
2. **Si el guard es fragil o el piso se infla:** son dos causas distintas y el
   `-short` tapa las dos.
3. Los cuatro hallazgos BAJO de Tao, arriba.
4. **Si el PR #8 deja `main` verde al mergear:** lo verificado es que deja verde
   la PUNTA. El `main` post-merge es otro arbol y otro NO MEDIDO.

--- METODO TITAN ---
Accion delicada: NO
Modo aplicado:   TITAN FULL
Rubrica:         89/95 -> 93,7/100
N/A declarados:  5 (DevOps: la entrega es una verificacion de auditoria ajena)
Review externo:  el auditado soy yo; Tao es el instrumento ajeno de este turno
Instrumento:     check runs por PR (no el estado combinado), ci.yml de main leido
                 verbatim, list_branches, list_pull_requests, list_issues
Maquina:         ninguna: solo lectura de GitHub
Artefactos:      este archivo + ESTADO-KAMPE-IR.md corregido + Doc de ClickUp
