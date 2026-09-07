# 2026-09-07 · 03 · ADR-001: que modulo IR construir

## 1. Pedido

"PUES LE METAMOS UN PRODUCTO IR, SOLO DIME DE QUE TIPO QUIRES Y YO TE LO DOY".

O sea: elegir **un** modulo IR y especificarlo con precision suficiente para que
lo genere otro. Elegir es donde mas dano hago, asi que la eleccion va con
medicion, no con gusto.

## 2. Herramientas declaradas

| Herramienta | Escribio | Cuota ajena |
| --- | --- | --- |
| sandbox local (grep sobre el repo ya clonado) | no | no |
| GitHub API: `push_files` x1 | si, este repo | no |
| ClickUp: un Doc publico | si | no |

No se ejecuto codigo nuevo. Esta entrega es una decision, no una medicion de
rendimiento.

## 3. La medicion que decide el modulo

No elegí por catalogo de mercado. Fui a ver que agujero deja el sistema tal como
esta escrito.

### 3.1 El punto de aplicacion decide 16 veces y no deja ni una linea escrita

```
=== el gateway loguea algo? ===
   CERO llamadas de log en todo el paquete gateway

   TriggerBlock: 7 sitios
   writeReject : 9 sitios

=== y del otro lado: el gateway escribe en la base? ===
   el gateway NO tiene driver de base de datos ni una sola escritura
```

Los imports de `gateway/gateway.go` son `crypto/hmac`, `crypto/sha256`,
`crypto/tls`, `crypto/x509`, `encoding/binary`, `encoding/hex`, `errors`,
`fmt`, `io`, `math`, `net`, `os`, `sync`, `time`. **No esta el paquete `log`.**

### 3.2 La boveda de no-repudio no tiene un solo productor

```
=== importadores de custos_legis en codigo de producto ===
   0
```

Correccion de mi propio instrumento: la primera corrida me dio "2" y los dos
hits eran el propio modulo y **mi verificador**. Grep sucio, numero corregido.

Custos Legis existe, esta bien construido (HMAC verificado 200/200) y **nadie le
escribe nada**. El componente que toma todas las decisiones esta en Go y no
conoce su existencia.

### 3.3 Los 9 rechazos son indistinguibles desde afuera

`rejectBytes = []byte{0xFF}`: **un byte, el mismo para los 9 caminos**. Hoy un
agente rechazado no puede saber por que, y vos tampoco.

Los 9, en orden de aparicion en `handleConn`:

| # | Causa | Escala el backoff |
| --- | --- | --- |
| 1 | el FSM ya estaba en backoff | no |
| 2 | tamano de frame invalido (0 o > 64 KiB) | **no** |
| 3 | el paquete no decodifica | si |
| 4 | el AgentID del paquete no coincide con el del certificado | si |
| 5 | timestamp fuera de la ventana de ±30 s | si |
| 6 | replay: timestamp no estrictamente creciente | si |
| 7 | HMAC invalido | si |
| 8 | Huber: derivada de varianza de volumen | si |
| 9 | Coherence: desalineamiento del vector de contexto | si |

### 3.4 Conclusion

**El sistema aplica y no atestigua.** Eso no es un bug de logging: es
exactamente el hueco que un producto IR llena, y es el unico hueco de esta
arquitectura que ningun competidor puede llenar por vos, porque la evidencia
solo existe en el punto de aplicacion, en el instante de la decision.

## 4. La decision

**Testis** (lat. *testigo*, para que rime con Custos Legis): el **grabador de
veredictos con cadena de hash, y el caso por agente construido sobre esa
cadena.**

No es un SOAR. No es forense de memoria. No es threat intel. Es la mitad de IR
que este sistema puede sostener con honestidad: **evidencia y timeline.**

### 4.1 Lo que se vende, en una frase

Todos los demas reconstruyen el incidente **despues**, leyendo logs que el
sistema escribio sin saber que iban a ser evidencia. Testis emite la evidencia
**el punto de aplicacion, en el instante de la decision, firmada y encadenada**.
Eso no lo puede hacer un SIEM por afuera.

### 4.2 Y por que ademas es la jugada barata

Son 3 archivos nuevos y 2 tablas. Reusa Custos Legis, que ya existe y hoy no
sirve para nada por falta de productor. No agrega una quinta pieza al
ecosistema: le pone el cable a la que estaba desconectada.

## 5. El hallazgo nuevo, que es la razon de ser del modulo

### D-18 · `audit_logs` firma cada fila por separado, asi que borrar una fila es indetectable

Cada fila lleva su `hmac_signature` sobre su propio `metadata`. Eso prueba que
**una fila no fue modificada**. No prueba nada sobre el conjunto: se puede
**borrar** una fila entera, o reordenar, y **todas las firmas restantes siguen
verificando**. Para un log de auditoria legal eso es el ataque obvio: el
insider no altera el registro incomodo, lo hace desaparecer.

Un producto que se llama "Boveda Criptografica Legal y **No-Repudio**" tiene que
resistir borrado, no solo edicion. **Por eso la cadena de hash no es un adorno
del modulo IR: es el modulo.**

### D-19 · 2 de los 9 rechazos no escalan el circuit breaker

Un frame con tamano invalido (caso 2) rechaza y cierra, pero **no** llama a
`TriggerBlock`. El costo para el atacante es un handshake TLS, y el FSM no lo
acumula nunca. No es critico, es la clase de cosa que solo se ve cuando existe
el grabador: hoy no hay forma de enterarse.

## 6. Especificacion, para que no haya que improvisar nada

### 6.1 `gateway/verdict.go` — el veredicto

```go
type RuleCode uint8

const (
    RuleFSMBlocked      RuleCode = 1
    RuleFrameSize       RuleCode = 2
    RuleDecodeFailed    RuleCode = 3
    RuleAgentIDMismatch RuleCode = 4
    RuleTimestampWindow RuleCode = 5
    RuleReplay          RuleCode = 6
    RuleHMACInvalid     RuleCode = 7
    RuleHuberTrip       RuleCode = 8
    RuleCoherenceTrip   RuleCode = 9
    RuleAccepted        RuleCode = 200 // muestreado, no todos
)

type Verdict struct {
    Seq         uint64    // monotonico POR AGENTE, arranca en 1
    PrevHash    [32]byte  // hash del veredicto Seq-1 de ESTE agente
    AgentID     [32]byte  // sha256(cert.RawSubject), el del certificado
    ObservedAt  int64     // reloj del servidor, ns. NO el del paquete
    Rule        RuleCode
    PacketTS    uint64    // el timestamp que trajo el paquete, crudo
    Epoch       uint32
    Nonce       uint32
    PayloadLen  uint32    // len(Ciphertext). NUNCA el contenido
    Context     [8]float64
    BackoffNs   int64     // el backoff vigente DESPUES de aplicar el veredicto
    Streak      int32     // exitos consecutivos al momento del veredicto
    Hash        [32]byte  // sha256 del canonico de todo lo de arriba
    Signature   [32]byte  // HMAC-SHA256(Hash), clave distinta a la de paquetes
}
```

Reglas duras:

1. **El canonico se serializa con longitudes fijas y big-endian, campo por
   campo. Nada de `k=v` unido por separadores.** Esa es literalmente la falla
   D-06 y no se repite aca.
2. **`Hash` incluye `PrevHash`.** Ahi vive el no-repudio del conjunto.
3. **Cero payload.** Se graba `PayloadLen`, nunca `Ciphertext`. El gateway no
   descifra y Testis no tiene que volverse un deposito de datos del cliente.
4. **Clave de firma propia** (`TESTIS_HMAC_KEY`), separada de
   `GATEWAY_HMAC_KEY`. Si se filtra la de paquetes, la evidencia sigue valiendo.

### 6.2 `gateway/recorder.go` — el grabador

```go
func (r *Recorder) Emit(v *Verdict)   // NUNCA bloquea, NUNCA devuelve error
```

1. **Prohibido que el grabador pueda frenar la aplicacion.** Canal con buffer;
   si esta lleno, se descarta y se incrementa `dropped`. Un disco lleno o una
   base caida **no puede** dejar de bloquear a un agente hostil.
2. **Pero el descarte se atestigua:** cada N descartes emite un veredicto
   sintetico `RuleRecorderDropped` con el contador. Un hueco silencioso en la
   cadena es peor que un hueco declarado.
3. **WAL append-only en disco primero**, con `fsync`, y de ahi a Postgres por un
   worker aparte. El orden es: durar, despues consultar.
4. `RuleAccepted` va **muestreado** (1 de cada K, K configurable). Grabar los
   aceptados completos convierte la boveda en un firehose.
5. El estado por agente (`Seq`, `PrevHash`) vive en el `AgentState` que ya
   existe, **bajo el `agent.mu` que ya existe**. No hay candado nuevo.

### 6.3 Los 9 puntos de insercion

En `handleConn`, **cada** `writeReject(conn)` queda precedido por su
`recorder.Emit(...)` con el `RuleCode` que le corresponde de la tabla de 3.3. Son
9 lineas. El criterio de aceptacion es que **no quede ningun `writeReject` sin
un `Emit` inmediatamente antes**, y eso se verifica con un test que cuenta los
dos y falla si difieren.

### 6.4 `ir/case.py` — el caso, que es la parte IR

Maquina de estados por agente, alimentada por la cadena:

| Estado | Entra cuando | Sale cuando |
| --- | --- | --- |
| `SIN_CASO` | inicial | primer veredicto con `Rule != Accepted` |
| `ABIERTO` | primer rechazo | 10 aceptados consecutivos (mismo umbral que el decay del FSM) → `AUTORECUPERADO`; o intervencion humana → `CERRADO` |
| `AUTORECUPERADO` | el agente se normalizo solo | un rechazo nuevo lo reabre |
| `CERRADO` | lo cierra una persona, con nota | no reabre: se abre un caso nuevo que referencia al anterior |

Campos del caso: `agent_id`, `opened_at`, `closed_at`, **histograma de las 9
reglas**, `backoff_peak_ns`, `verdicts_seq_from` / `verdicts_seq_to` (el tramo de
cadena que es la evidencia), `self_recovered`, `closed_by`, `note`.

Ese histograma es el triage: "este agente dio 400 HMAC invalidos" y "este dio
400 Coherence" son dos incidentes de naturaleza distinta, y hoy los dos son el
mismo byte `0xFF`.

### 6.5 SQL

```sql
CREATE TABLE IF NOT EXISTS verdicts (
    agent_id     bytea       NOT NULL,
    seq          bigint      NOT NULL,
    prev_hash    bytea       NOT NULL,
    hash         bytea       NOT NULL,
    signature    bytea       NOT NULL,
    observed_at  timestamptz NOT NULL,
    rule         smallint    NOT NULL,
    packet_ts    numeric(20) NOT NULL,   -- uint64 no entra en bigint
    epoch        bigint      NOT NULL,
    nonce        bigint      NOT NULL,
    payload_len  integer     NOT NULL,
    context      double precision[8] NOT NULL,
    backoff_ns   bigint      NOT NULL,
    streak       integer     NOT NULL,
    PRIMARY KEY (agent_id, seq)
);

CREATE INDEX IF NOT EXISTS idx_verdicts_observed ON verdicts (observed_at DESC);
CREATE INDEX IF NOT EXISTS idx_verdicts_rule     ON verdicts (rule, observed_at DESC);

CREATE TABLE IF NOT EXISTS cases (
    case_id         uuid        PRIMARY KEY DEFAULT uuid_generate_v7(),
    tenant_id       text        NOT NULL,
    agent_id        bytea       NOT NULL,
    state           text        NOT NULL,
    opened_at       timestamptz NOT NULL,
    closed_at       timestamptz,
    seq_from        bigint      NOT NULL,
    seq_to          bigint,
    rule_histogram  jsonb       NOT NULL,
    backoff_peak_ns bigint      NOT NULL,
    self_recovered  boolean     NOT NULL DEFAULT false,
    closed_by       text,
    note            text,
    supersedes      uuid REFERENCES cases (case_id)
);
```

`packet_ts` va `numeric(20)`: un `uint64` de ataque (`2^64-1`) **no entra en un
`bigint`** de Postgres. Si se usa `bigint`, el veredicto que registra el ataque
D-01 explota al insertarse, que seria comico.

### 6.6 Criterios de aceptacion, con control positivo obligatorio

Un verificador en `verificacion/t_testis.py` que:

1. Construya una cadena de 100 veredictos y la valide entera. **Verde.**
2. **Control positivo A:** borre el veredicto 50 y verifique que el validador da
   **ROJO**. Si no da rojo, el modulo no sirve: es el ataque D-18.
3. **Control positivo B:** cambie un byte del `Context` del veredicto 50 y
   verifique **ROJO**.
4. **Control positivo C:** reordene 50 y 51 y verifique **ROJO**.
5. Un test de colision al estilo D-06: dos veredictos que difieren solo en
   donde cae un separador tienen que dar hashes distintos.
6. Cuente `writeReject` y `Emit` en `handleConn` y falle si no son iguales.

**Un validador de cadena que no puede dar rojo no es un validador.**

## 7. Lo que NO quiero, y por que

| Tipo de IR | Por que no |
| --- | --- |
| **SOAR / playbooks / remediacion automatica** | necesita integraciones que no tenes (ticketing, EDR, cloud). Compite con Palo Alto y Splunk, que tienen producto vivo cobrando. Y automatizar respuesta sobre un sistema con 17 defectos abiertos es automatizar el dano |
| **Forense de memoria / disco** | pide agente privilegiado en el endpoint. Es otro producto, otro equipo, otro ciclo de certificacion |
| **Threat intel / feeds** | requiere datos de muchos clientes. Tenes cero clientes. Es el producto que solo funciona cuando ya ganaste |
| **SIEM / correlacion multi-fuente** | seria tirar la ventaja: tu evidencia vale porque nace firmada en el punto de aplicacion, no porque agregues fuentes ajenas |

## 8. Dos bloqueantes: el modulo IR no puede salir antes de estos

1. **D-06 es ahora bloqueante de release, no un bug mas.** Custos Legis firma
   dos eventos distintos con la misma firma. Si el producto IR se apoya en una
   funcion de firma ambigua, la palabra "no-repudio" del brochure es falsa y un
   peritaje contrario lo demuestra en una tarde. Testis usa serializacion de
   ancho fijo justamente por esto, pero mientras `canonicalize_event` siga
   viva y firmando `audit_logs`, hay dos niveles de evidencia en el mismo
   producto y uno es malo.
2. **D-01 + D-02.** Si un paquete con `ts = 2^64-1` y HMAC invalido ladrilla a
   un agente para siempre, el primer caso IR del primer cliente va a ser **un
   bug de KAMPE**, no un incidente suyo. El grabador lo va a documentar con
   firma y cadena de hash, o sea que vas a tener evidencia criptografica de tu
   propio defecto. Con lujo de detalle.

Ese segundo punto es casi gracioso pero es la razon de orden: **el grabador hay
que construirlo despues de arreglar el anti-replay, no antes.**

## 9. Archivos generados en este mismo commit

- `respuestas/2026-09-07-03-spec-ir-testis.md` (este)
- `02-BITACORA.md` (entrada E-004)
- `CONTEXTO-KAMPE-IR.md` (D-18, D-19, y el modulo IR elegido)

Cero codigo: Abraham lo genera. Esta entrega es la especificacion contra la que
se va a medir.

## 10. NO MEDIDO

1. **Costo de escritura del grabador.** No medi cuanto cuesta un `fsync` por
   veredicto ni a que tasa de paquetes el WAL se vuelve el cuello de botella.
   El muestreo de `RuleAccepted` es una hipotesis mia sin numero detras.
2. **Si el `AgentState` aguanta el estado de cadena** sin cambiar la
   granularidad de `agent.mu`. Lo afirmo por lectura, no por medicion.
3. **Si D-18 es explotable en el deployment real** o si Postgres esta detras de
   permisos que impiden un DELETE. No se como pensas desplegarlo.
4. **Si "IR" con este alcance alcanza para un comprador.** Da evidencia y
   triage; no da case management con SLAs, ni notificacion regulatoria, ni
   forense. Sigue siendo **la mitad de IR**, honestamente nombrada.
5. Todo lo de E-002 sigue abierto: gateway sin compilar, data race, SQL sin
   Postgres, cero integracion mTLS.
