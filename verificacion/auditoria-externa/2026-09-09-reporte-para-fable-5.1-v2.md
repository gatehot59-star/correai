# REPORTE TÉCNICO v2 PARA AUDITORÍA EXTERNA

**Proyecto:** CORREAI / KAMPE IR · **Repo público:** `gatehot59-star/correai`
**Auditor destinatario:** Fable 5.1 dentro de Abacus.AI
**Fecha:** 2026-09-09 · **`main` medido en:** `458ad39`
**Reporte anterior:** `verificacion/auditoria-externa/2026-09-08-reporte-para-fable-5.1.md`

---

## 0 · Lo primero, porque cambia cómo se lee todo lo demás

**DOS entregas de este turno tienen su CI en `queued`, no en `success`.** Las corridas `34312687284` y `34313580686` llevan más de 15 minutos encoladas.

Eso significa, con precisión: **el cableado de Testis a `handleConn` y su test son código ESCRITO y NO VERIFICADO.** No compilaron. No corrieron. En `brain-env` no hay toolchain de Go (`go: not found`, medido), así que el autor **no puede** haberlos compilado.

Lo pongo primero y no en los NO MEDIDO porque el reporte v1 se apoyaba enteramente en "todo verde lo firma un instrumento ajeno". Presentar esto como verde sería el error más caro del proyecto.

---

## 1 · Qué SÍ está medido y en `main`

### D-08 cerrado: un header exigido que nunca se comparaba

`/ota/download` declaraba `delivery_token: str = Header(..., alias="X-Delivery-Token")`. **`Header(...)` obliga a que el header VENGA, no a que valga algo:** FastAPI devolvía 422 si faltaba y **200 con el binario** si estaba, con cualquier cadena. Y el token no se persistía en ningún lado, así que no había contra qué compararlo.

Un header exigido y no verificado es **peor** que no tenerlo, porque parece un control.

```bash
git show main:fleet/fleet_manager.py | grep -c 'secrets.compare_digest'   # 2 (1 codigo, 1 comentario)
git show main:audit/schema.sql | grep -c 'delivery_token_sha256'          # 1
python3 verificacion/t_fleet.py                                          # fallos=0, exit 0
```

El fix: `sha256` del token persistido (nunca el token en claro: la base es donde un dump lo expondría), y comparación con **`secrets.compare_digest`, no con `==`** (el `==` de Python corta en el primer byte distinto y filtra por tiempo cuánto prefijo acertó el atacante). Corta con **403**, no 404: el paquete existe y el tenant ya probó ser su dueño.

**Y el verificador invirtió su polaridad con control positivo.** `t_fleet.py` afirmaba que D-08 seguía presente; arreglar el código sin tocarlo habría puesto en rojo el job `custos legis`. Ahora son 4 invariantes más una **mutación que le saca la comparación al fuente en memoria y exige que el guard se ponga rojo**. Sin ese control, los 4 verdes no distinguen "está bien" de "mi guard no mira".

CI sobre `main`: corrida `34309835047`, **6 jobs `success`**.

### Estado de `main`

```bash
git log -4 --format='%h %s' main
git ls-tree --name-only main verificacion/resultados-actions/ | wc -l   # 63
```

---

## 2 · Testis en Go: el módulo que le da el nombre al producto

**Tenía cero líneas de Go.** Es el diferencial, y las **tres auditorías** del proyecto (la propia, la de Tao y la de Tachi) coincidieron en ese punto: era su **único consenso**.

PR **#17**, rama `titan/testis-en-go`: 412 líneas de implementación, 445 de test. Corrida `34312092430`, **6 jobs `success`**. Esto **sí** está verificado.

### El control que decide si el puerto vale

El Go tiene que producir **los mismos bytes** que `verificacion/t_testis.py`, hash por hash. No "equivalente": **idéntico**. Si difieren en un byte, el auditor que verifique una cadena con una implementación y la rechace con la otra **tiene razón las dos veces**, y el producto no tiene un formato: tiene dos.

El test **no compara Go contra Go** (eso solo probaría consistencia interna). Trae **7 constantes hex golden calculadas por CPython 3.12**:

| vector | golden |
|---|---|
| canónico del primer veredicto | `678fcd4b0384fe96...` |
| su firma HMAC | `55d9a0cb6027e2f6...` |
| `packet_ts = 2^64-1` | `ea11173e80368428...` |
| hoja Merkle | `3cd1ed0f4a41c9ae...` |
| raíz con 3 hojas (impar) | `32c203487271fe73...` |
| hash del Sello | `7fedeaf1fd1f56cc...` |
| firma del Sello | `6bbbbf4caf9ce01c...` |

**Recomputá cualquiera** con `verificacion/t_testis.py` y compará contra las constantes de `testis/testis_test.go`. Si alguno difiere, es un hallazgo válido contra nosotros.

### El control positivo que más vale, y de dónde salió

CP-5: **borrar el veredicto 50 y RENUMERAR el resto.** Salió de la matriz de mutación E-006, que descubrió algo incómodo: apagar el chequeo de `prev_hash` dejaba el script en verde, porque el control de `seq` agarraba el borrado y el reordenamiento primero. O sea que **la cadena de hash, que ES el producto, estaba sin probar**. El ataque que lo aísla es el que haría un atacante competente: renumerar es un `UPDATE`.

### Tres tests que AFIRMAN límites

- **D-20:** sin Sello, el truncamiento de cola verifica en **verde**. El test lo afirma, para que quien "arregle" el validador sin agregar ancla se entere de que no alcanza.
- **D-28:** el Sello no cubre lo emitido después del último checkpoint. La ventana es exactamente **T**, parámetro de **riesgo**, no de performance.
- **D-25:** con la clave se refabrica la cadena entera y verifica en verde. **Tamper-evidence, no no-repudio.** Propiedad de HMAC.

---

## 3 · El cableado a `handleConn` · **NO VERIFICADO**

Rama `titan/testis-cableado`. Los **10 puntos de decisión** de `handleConn` emiten un `Verdict` encadenado: nueve rechazos más el aceptado. El aceptado va porque una cadena que solo registra rechazos no prueba que el gateway estaba vivo entre dos rechazos.

### El hallazgo del turno: un deadlock que solo el orden evita

En el punto de replay, `handleConn` **ya tiene `agent.mu` tomado**, y `emitir()` también lo toma para avanzar la cadena. Llamarlo adentro **no es un test rojo: es un cuelgue**. El `emitir` va después del `Unlock`, y el guard lo verifica **contando profundidad de Lock/Unlock sobre el árbol**, no leyendo el código: `emitir DENTRO de un Lock = 0`.

### Decisiones de diseño, con su razón

- **El estado de la cadena vive en `AgentState`, no en la pila.** Un certificado puede abrir varias conexiones concurrentes y la cadena es **una por agente**. En la pila, dos conexiones emitirían `seq=1` y el validador leería **manipulación donde solo hubo concurrencia**.
- **`Recorder` es una interfaz, no una implementación.** El destino real es Postgres y las 5 tablas no existen. Con interfaz el cable se escribe y se mide hoy.
- **`Grabar()` no devuelve error:** el grabador no puede vetar una decisión de seguridad ya tomada. Si falla, es un hueco **declarado** (`dropped_since`), que es la regla D-21.
- **`testisKey` es distinta de `hmacKey`.** Si fueran la misma, quien puede hablarle al gateway podría refabricar su propia evidencia.

### El test del cableado · **también NO VERIFICADO**

6 tests con `MemRecorder`. Mide que la cadena emitida **valida con su propio validador** (con control positivo: 1 bit volteado → rojo), que **cada rechazo emite su regla** (5 casos; desde el cliente los 9 rechazos son el mismo byte `0xFF`, y la cadena es el único lugar donde se distinguen), que **dos conexiones del mismo cert dan una sola cadena** con seq contiguo (falsa la decisión de arriba), y que **el replay no cuelga**, con timeout propio de 20 s.

**Dos defectos propios cazados antes de pushear, los dos con una llamada:** escribí `ag.idDerivado` cuando el campo se llama `ag.agentID` (12 ocurrencias; lo cazó **leer el struct**, no releer mi código), y un `t.Logf` con `%v` y **cero argumentos**, que `go vet` habría puesto en rojo.

---

## 4 · Dos auditorías ajenas de este turno, y cómo terminaron

### Tao: me encontró algo real y se retractó de lo suyo

Su **A-10** es un hallazgo válido que yo no vi: el paso 0 de `fix-huber-medicion.yml` copia `gateway.go` y llama a eso "copia prístina" para armar los brazos B y D. **Con el fix ya en `main`, esa copia lo contiene**, así que esos brazos dejaron de ser control. Queda declarado en la cabecera del workflow, **no arreglado**.

Y su propio PR #8 quedó **falsado**, con su autor escribiendo *"tenía razón quien lo cerró"* y que su `-short` *"habría tapado un bug real"*.

### Tachi: premisa central refutada, pero su regla de oro es válida contra mí

Afirmó que `main` era el código del día 1 y que la cadena Go no había llegado. Medido: **8 de 9 ramas de esa cadena están DENTRO de `main`** con 0 commits faltantes, y `main` tiene 113 commits posteriores al día 1. **Confundió "PR cerrado" con "contenido no mergeado".**

Pero su regla de oro es un hallazgo contra mí: cerré 9 PRs y el contenido está, **el objeto navegable no**. Un auditor que abra la lista de PRs concluye exactamente lo que él concluyó. **Dejé el repo engañoso para un lector externo y no lo advertí.**

Respuesta completa: `respuestas/2026-09-09-01-falsacion-de-la-guia-de-merge-de-tachi.md`.

---

## 5 · Los 9 NO MEDIDO

1. **El cableado y su test NO COMPILARON.** CI en `queued`. Lo más importante de esta lista.
2. **El paquete `testis` NO está en `main`**: vive en rama. `git cat-file -e main:testis/testis.go` falla.
3. **`Testis` no tiene llamador en `main`.** Existe como código, no como feature. Es exactamente el estado del Zod Gate de MUDH cuando se midió.
4. **Cero Postgres.** Las 5 tablas de `schema.sql` no existen en ninguna base. La columna `delivery_token_sha256` es DDL **sin aplicar**, y no hay migración (un `ALTER TABLE` sin destino es teatro).
5. **La equivalencia Python/Go está probada en 4 vectores, no exhaustivamente.** El instrumento fuerte sería un **fuzzer diferencial**, y no existe.
6. **La evidencia NO cubre el 100% de las conexiones rechazadas.** Los rechazos anteriores a identificar al agente (handshake fallido, cero certificados) no emiten nada: sin `agentID` no hay cadena a la que encadenar. Límite real del diseño.
7. **A-10 sigue abierto** (los brazos B y D de `fix-huber-medicion.yml` ya no son control).
8. **D-48, D-45, D-42, D-43** siguen abiertos. `engine.hpp` sigue con la varianza que colapsa, y **el OTA nunca se dispara**.
9. **Cero clientes reales.** Ninguna medición de este repo prueba que alguien quiera esto.

---

## 6 · Verificaciones adversariales, cada una puede dar rojo

**V-1 · ¿El CI del cableado habló?** Pedí los **check runs** de las corridas `34312687284` y `34313580686`. Si siguen en `queued`, este reporte dice la verdad. **Si alguna dio `failure`, el reporte está desactualizado y vos lo sabrás antes que yo.**

**V-2 · ¿Los golden coinciden?**
```bash
python3 verificacion/t_testis.py      # imprime los hashes
# comparar contra las constantes hex de testis/testis_test.go
```

**V-3 · ¿D-08 realmente compara?**
```bash
git show main:fleet/fleet_manager.py | grep -A3 'esperado = row'
```
Tiene que aparecer `secrets.compare_digest`, no `==`.

**V-4 · Control positivo del verificador de fleet.** En `t_fleet.py`, la mutación reemplaza la comparación por `if False:` y exige que el guard caiga. Sin eso, sus 4 verdes no miden nada.

**V-5 · Los check runs, no el estado combinado.** En este repo el combinado devuelve `total_count: 0` sobre 6 jobs verdes. Leer solo eso produce un "acá no hay checks" falso.

**V-6 · ¿Testis está en `main`?**
```bash
git cat-file -e main:testis/testis.go && echo SI || echo NO
```
Tiene que decir **NO**. Si dice SÍ, alguien mergeo y este reporte quedó viejo.

---

## 7 · Autoevaluación

**Lo fuerte:** todo verde citado lo firma Actions; hay control positivo en cada medición nueva (mutación en `t_fleet.py`, bit volteado en el test del cableado, golden cruzados en Testis); dos auditorías ajenas encontraron cosas y **una me corrigió de verdad** (A-10); y un defecto propio se cazó **antes** de publicar el número (el `grep` que contaba mi propio comentario).

**Lo débil, sin adornos:**

- **Dos entregas sin compilar.** El turno cierra con código cuyo único testigo posible no habló.
- **Seis guards propios en la historia del proyecto contaron palabras en vez de leer estructura.** Es un patrón, no un accidente. La sexta se cazó a tiempo; las cinco anteriores, no.
- **El producto sigue sin tocar una base de datos** y sin un solo cliente.
- Los cierres de este turno son **de código**, no de features: Testis existe y no está cableado en `main`.

**No reclamamos un score.** Un número lo pone el auditor. Lo que reclamamos es que este documento sea **falsable**, y su punto más falsable está en la sección 0: si el CI habló y dio rojo, todo lo de la sección 3 se cae.

---

**--- METODO PROMETEO ---**

- **Máquinas:** `brain-env` (sin toolchain de Go, medido) para escribir, generar golden con CPython y correr guards sobre el árbol; **GitHub Actions** como único testigo de todo compilador.
- **W-01:** los golden los produjo un instrumento distinto del que se audita. El Go no puede validarse contra números que él mismo calculó.
- **Transporte verificado byte a byte** en las tres transferencias del turno (`sha256` de tarball y `git hash-object` por archivo, comparados en los dos lados).
- **Todo merge fue decisión humana.** Medir no es decidir.
