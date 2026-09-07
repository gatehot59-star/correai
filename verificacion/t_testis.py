#!/usr/bin/env python3
"""Verificador de TESTIS, el modulo IR de KAMPE IR.  ADR-001 + auditoria E-005.

Implementa y MIDE tres cosas:

  1. El canonico de ANCHO FIJO del Verdict (spec 6.1 + DroppedSince).
  2. La cadena de hash por agente, con huecos DECLARADOS por descarte.
  3. El SELLO: checkpoint firmado con raiz Merkle de los heads de todos los
     agentes. Sin el, borrar la COLA de la cadena verifica en VERDE (D-20).

Reporta CUATRO categorias separadas, y la diferencia entre las dos del medio
importa mas de lo que parece:

  INVARIANTE      : lo que debe seguir siendo cierto. Si se rompe, rojo.
  CONTROL POSITIVO: un ataque que el validador TIENE que rechazar. Si pasa en
                    verde, el validador no sirve y el script sale 1.
  DEFECTO         : se mide LEYENDO el repo. Si desaparece, rojo, y obliga a
                    tocar la bitacora en el commit que lo arregla.
  PROPIEDAD       : verdad sobre un diseno o sobre datos que construyo aca.
                    NO puede detectar que alguien arreglo el codigo Go.
  NO MEDIDO       : declarado, no escondido.

Por que existe PROPIEDAD (E-006): la version anterior llamaba "DEFECTO" a las
seis, y el README prometia que un defecto arreglado pone el test en rojo. Para
CUATRO de las seis eso era FALSO: D-20, D-21, D-25 y D-28 se calculan sobre
cadenas que construyo yo, asi que arreglar el gateway no las mueve. El cargo lo
levante primero contra el validador del auditor (sus E1/F1/G1 tienen el mismo
problema, y su nota decia explicitamente lo contrario) y despues me lo tuve que
aplicar. Solo D-26 y D-29 leen la fuente.

Un validador de cadena que no puede dar rojo no es un validador.

MATRIZ DE MUTACION SOBRE ESTE MISMO ARCHIVO (2026-09-07, E-006). Apague cada
chequeo del validador de a uno y anote que control se cae:

  apago seq                  rc=1   cae: la PROPIEDAD D-21 (nada mas)
  apago prev_hash            rc=1   cae: CP-5
  apago hash                 rc=1   cae: CP-2
  apago firma del veredicto  rc=1   cae: CP-4
  apago largo del Sello      rc=1   CRASH (IndexError, feo pero detectado)
  apago firma del Sello      rc=1   cae: CP-8

Lo que revelo: en la version anterior, apagar el chequeo de prev_hash daba
**rc=0**. Ningun control lo tocaba, porque el de seq agarra el borrado y el
reordenamiento primero. O sea que la CADENA DE HASH, que es el producto entero,
estaba sin probar. CP-5 se agrego para aislarla: borrar el 50 y RENUMERAR el
resto, que es lo que haria un atacante competente porque renumerar es un UPDATE.
Y el chequeo de `largo del Sello` se detecta por excepcion, no por control: es
deuda declarada, no cobertura.

Corrido: 2026-09-07, CPython 3.12.13. Sin dependencias para el nucleo;
`cryptography` solo para el control positivo de ed25519 (D-25), que se declara
NO MEDIDO si la libreria no esta.
"""

import hashlib
import hmac
import os
import re
import struct
import sys

# ----------------------------------------------------------------------
# Claves de prueba. En produccion: TESTIS_HMAC_KEY, distinta de
# GATEWAY_HMAC_KEY, y el Sello firmado con ed25519 (clave privada que no sale
# del gateway).
#
# Claves FIJAS a proposito: con `os.urandom` la evidencia no es recomputable y
# un auditor no puede rehacer los hashes de una corrida commiteada.
# ----------------------------------------------------------------------
K_VERDICT = bytes(range(32))
K_SELLO   = bytes(range(32, 64))
CERO32    = b"\x00" * 32

# ----------------------------------------------------------------------
# 1. El canonico de ancho fijo
# ----------------------------------------------------------------------
# Big-endian, sin padding de alineacion, sin UN SOLO separador.
#   seq Q | prev_hash 32s | agent_id 32s | observed_at q | rule B |
#   packet_ts Q | epoch I | nonce I | payload_len I | context 8d |
#   backoff_ns q | streak i | dropped_since I
FMT = ">Q32s32sqBQIII8dqiI"
CANONICAL_LEN = struct.calcsize(FMT)   # 181, verificado abajo como invariante

REGLAS = {
    1: "FSMBlocked", 2: "FrameSize", 3: "DecodeFailed", 4: "AgentIDMismatch",
    5: "TimestampWindow", 6: "Replay", 7: "HMACInvalid", 8: "HuberTrip",
    9: "CoherenceTrip", 10: "Decayed", 200: "Accepted",
}


class Verdict:
    __slots__ = ("seq", "prev_hash", "agent_id", "observed_at", "rule",
                 "packet_ts", "epoch", "nonce", "payload_len", "context",
                 "backoff_ns", "streak", "dropped_since", "hash", "signature")

    def __init__(self, seq, prev_hash, agent_id, observed_at, rule,
                 packet_ts=0, epoch=0, nonce=0, payload_len=0, context=None,
                 backoff_ns=0, streak=0, dropped_since=0):
        self.seq, self.prev_hash, self.agent_id = seq, prev_hash, agent_id
        self.observed_at, self.rule = observed_at, rule
        self.packet_ts, self.epoch, self.nonce = packet_ts, epoch, nonce
        self.payload_len = payload_len
        self.context = list(context) if context else [0.0] * 8
        self.backoff_ns, self.streak = backoff_ns, streak
        self.dropped_since = dropped_since
        self.hash = self.signature = None

    def canonico(self) -> bytes:
        return struct.pack(FMT, self.seq, self.prev_hash, self.agent_id,
                           self.observed_at, self.rule, self.packet_ts,
                           self.epoch, self.nonce, self.payload_len,
                           *self.context, self.backoff_ns, self.streak,
                           self.dropped_since)

    def sellar(self, key=K_VERDICT):
        self.hash = hashlib.sha256(self.canonico()).digest()
        self.signature = hmac.new(key, self.hash, hashlib.sha256).digest()
        return self


# ----------------------------------------------------------------------
# 2. El validador de cadena
# ----------------------------------------------------------------------
def validar_cadena(vs, agent_id, key=K_VERDICT, sello=None):
    """Devuelve (ok: bool, motivo: str, info: dict).

    Regla de huecos: un descarte del grabador NO consume seq (fix D-21). El
    veredicto siguiente declara cuantos se perdieron en `dropped_since`. Un
    salto de seq es SIEMPRE manipulacion.
    """
    info = {"n": len(vs), "huecos_declarados": [], "perdidos": 0}

    if sello is not None:
        ok, motivo = _validar_contra_sello(vs, agent_id, sello)
        if not ok:
            return False, motivo, info
    elif not vs:
        return True, ("cadena vacia y SIN SELLO: indecidible, no verde. "
                      "Este es D-20"), info

    if not vs:
        return False, "cadena vacia con sello vigente: truncamiento total", info

    for i, v in enumerate(vs):
        if len(v.canonico()) != CANONICAL_LEN:
            return False, f"canonico de largo variable en pos {i}", info
        if v.agent_id != agent_id:
            return False, f"agent_id ajeno en pos {i}", info
        if v.seq != i + 1:
            return False, (f"salto de seq en pos {i} "
                           f"(esperaba {i + 1}, vino {v.seq})"), info
        esperado = CERO32 if i == 0 else vs[i - 1].hash
        if v.prev_hash != esperado:
            return False, f"prev_hash roto en seq {v.seq}", info
        if hashlib.sha256(v.canonico()).digest() != v.hash:
            return False, f"hash no coincide en seq {v.seq}", info
        if not hmac.compare_digest(
                hmac.new(key, v.hash, hashlib.sha256).digest(), v.signature):
            return False, f"firma invalida en seq {v.seq}", info
        if v.dropped_since:
            info["huecos_declarados"].append((v.seq, v.dropped_since))
            info["perdidos"] += v.dropped_since

    return True, "cadena integra", info


# ----------------------------------------------------------------------
# 3. El Sello (ancla externa)
# ----------------------------------------------------------------------
def _hoja(agent_id, seq, head):
    return hashlib.sha256(b"\x00" + agent_id + struct.pack(">Q", seq) + head).digest()


def raiz_merkle(hojas):
    if not hojas:
        return CERO32
    n = list(hojas)
    while len(n) > 1:
        if len(n) % 2:
            n.append(n[-1])
        n = [hashlib.sha256(b"\x01" + n[i] + n[i + 1]).digest()
             for i in range(0, len(n), 2)]
    return n[0]


class Sello:
    """Checkpoint firmado. Se publica FUERA del alcance del DBA: archivo
    append-only en otro host, S3 Object Lock, o syslog remoto. Si vive en la
    misma base que los veredictos, no ancla nada."""

    __slots__ = ("anchor_seq", "prev_hash", "created_at", "instantanea",
                 "merkle_root", "hash", "signature")

    def __init__(self, anchor_seq, prev_hash, created_at, instantanea):
        self.anchor_seq, self.prev_hash = anchor_seq, prev_hash
        self.created_at = created_at
        # instantanea: {agent_id: (seq, head_hash)}
        self.instantanea = dict(instantanea)
        hojas = [_hoja(a, s, h)
                 for a, (s, h) in sorted(self.instantanea.items())]
        self.merkle_root = raiz_merkle(hojas)
        cuerpo = (struct.pack(">QqI", anchor_seq, created_at,
                              len(self.instantanea))
                  + prev_hash + self.merkle_root)
        self.hash = hashlib.sha256(cuerpo).digest()
        self.signature = hmac.new(K_SELLO, self.hash, hashlib.sha256).digest()

    def firma_valida(self):
        cuerpo = (struct.pack(">QqI", self.anchor_seq, self.created_at,
                              len(self.instantanea))
                  + self.prev_hash + self.merkle_root)
        if hashlib.sha256(cuerpo).digest() != self.hash:
            return False
        # La raiz tiene que recomputar desde la instantanea publicada.
        hojas = [_hoja(a, s, h)
                 for a, (s, h) in sorted(self.instantanea.items())]
        if raiz_merkle(hojas) != self.merkle_root:
            return False
        return hmac.compare_digest(
            hmac.new(K_SELLO, self.hash, hashlib.sha256).digest(),
            self.signature)


def _validar_contra_sello(vs, agent_id, sello):
    """El sello fija un punto de la cadena. La cadena PUEDE crecer despues del
    sello; lo que no puede es encogerse por debajo ni cambiar lo sellado.

    Esto es mas correcto que comparar head == ancla: comparar heads daria rojo
    en toda cadena viva que recibio un veredicto nuevo despues del checkpoint.
    Hay un control para eso en el bloque D.
    """
    if not sello.firma_valida():
        return False, "SELLO adulterado (hash, raiz Merkle o firma)"
    if agent_id not in sello.instantanea:
        return False, "el agente no figura en el sello"
    seq_sellada, head_sellado = sello.instantanea[agent_id]
    if len(vs) < seq_sellada:
        return False, (f"truncamiento de cola: el sello fija seq {seq_sellada} "
                       f"y la cadena tiene {len(vs)}")
    if vs[seq_sellada - 1].hash != head_sellado:
        return False, f"el veredicto seq {seq_sellada} no es el sellado"
    return True, "ok"


# ----------------------------------------------------------------------
# Constructor de cadenas de prueba
# ----------------------------------------------------------------------
def construir(agent_id, n, descartes=None, consumir_seq=False):
    """descartes: {indice_1based: cuantos_se_descartaron_antes}."""
    descartes = descartes or {}
    vs, prev, seq = [], CERO32, 1
    for i in range(1, n + 1):
        perdidos = descartes.get(i, 0)
        if perdidos and consumir_seq:
            seq += perdidos          # politica MALA de la spec 6.2
            perdidos = 0
        v = Verdict(seq=seq, prev_hash=prev, agent_id=agent_id,
                    observed_at=1_788_800_000_000_000_000 + i * 1_000_000,
                    rule=200 if i % 7 else 9,
                    packet_ts=1_788_800_000_000_000_000 + i * 900_000,
                    epoch=1, nonce=i, payload_len=512 + i,
                    context=[i / 8.0] * 8,
                    backoff_ns=100_000_000 * (1 + i % 5),
                    streak=i % 11, dropped_since=perdidos).sellar()
        vs.append(v)
        prev = v.hash
        seq += 1
    return vs


def sello_de(cadenas, anchor_seq=1, prev=CERO32):
    inst = {aid: (len(vs), vs[-1].hash) for aid, vs in cadenas.items()}
    return Sello(anchor_seq, prev, 1_788_800_100_000_000_000, inst)


# ----------------------------------------------------------------------
# Contadores del reporte
# ----------------------------------------------------------------------
fallos = 0
_cp = 0


def invariante(nombre, ok):
    global fallos
    print(f"  [{'ok  ' if ok else 'ROJO'}] INVARIANTE       {nombre}")
    if not ok:
        fallos += 1


def control_positivo(nombre, resultado):
    """resultado = (ok, motivo) del validador. TIENE que ser ok=False."""
    global fallos, _cp
    _cp += 1
    ok, motivo = resultado
    if ok:
        print(f"  [ROJO] CP-{_cp} NO RECHAZO   {nombre}")
        print(f"         el validador dijo VERDE. No sirve.")
        fallos += 1
    else:
        print(f"  [ok  ] CP-{_cp} rechazado    {nombre}")
        print(f"         -> {motivo}")


def defecto(nombre, sigue):
    """DEFECTO: se mide LEYENDO el repo. Si desaparece, rojo, y hay que tocar la
    bitacora en el mismo commit que lo arregla. Solo vale la etiqueta si la
    condicion sale de la fuente, no de datos que construyo yo."""
    global fallos
    print(f"  [{'presente' if sigue else 'CAMBIO'}] DEFECTO          {nombre}")
    if not sigue:
        fallos += 1


def propiedad(nombre, vale):
    """PROPIEDAD: verdad sobre un diseno o sobre datos sinteticos que construyo
    aca. NO puede detectar que alguien arreglo el codigo Go, porque no lo lee.
    Etiqueta separada a proposito: llamar a esto 'defecto medido' era simular
    rigor. Vino de una critica del auditor que primero le hice a el (H-5) y
    despues me tuve que hacer a mi."""
    global fallos
    print(f"  [{'ok  ' if vale else 'ROJO'}] PROPIEDAD        {nombre}")
    if not vale:
        fallos += 1


A1 = hashlib.sha256(b"agente-1").digest()
A2 = hashlib.sha256(b"agente-2").digest()

print(__doc__.split("\n\n")[0])
print(f"\ncanonico de ancho fijo: {CANONICAL_LEN} bytes por veredicto "
      f"(formato {FMT})\n")

# ======================================================================
print("== 0. INVARIANTES: la cadena limpia y el canonico ==")
base = construir(A1, 100)
ok, motivo, info = validar_cadena(base, A1)
print(f"   cadena de 100 veredictos: {'VERDE' if ok else 'ROJO'} ({motivo})")
largos = {len(v.canonico()) for v in base}
print(f"   largos distintos del canonico entre los 100: {largos}")
invariante("una cadena integra de 100 verifica en verde", ok)
invariante(f"el canonico mide siempre {CANONICAL_LEN} B (ancho fijo real)",
           largos == {CANONICAL_LEN})
invariante("el primer prev_hash son 32 ceros", base[0].prev_hash == CERO32)

sello_base = sello_de({A1: base, A2: construir(A2, 40)})
ok_s, motivo_s, _ = validar_cadena(base, A1, sello=sello_base)
print(f"   la misma cadena contra su Sello: {'VERDE' if ok_s else 'ROJO'}")
invariante("el Sello no da falso positivo sobre la cadena que sello", ok_s)

crecida = base + [Verdict(seq=101, prev_hash=base[-1].hash, agent_id=A1,
                          observed_at=1_788_800_200_000_000_000, rule=7,
                          packet_ts=1, epoch=1, nonce=101, payload_len=9,
                          context=[0.5] * 8, backoff_ns=200_000_000,
                          streak=0).sellar()]
ok_c, motivo_c, _ = validar_cadena(crecida, A1, sello=sello_base)
print(f"   cadena que CRECIO despues del Sello: "
      f"{'VERDE' if ok_c else 'ROJO'} ({motivo_c})")
invariante("el Sello permite crecimiento posterior (no compara heads)", ok_c)

# ======================================================================
print("\n== CONTROLES POSITIVOS: cada uno TIENE que dar rojo ==")
print("   (pediste 8; quedaron 11: tres salieron de hallazgos nuevos)")

sin50 = base[:49] + base[50:]
ok, motivo, _ = validar_cadena(sin50, A1)
control_positivo("borrar el veredicto 50 (del medio)", (ok, motivo))

mut = construir(A1, 100)
mut[49].context[3] = struct.unpack(">d", struct.pack(
    ">Q", struct.unpack(">Q", struct.pack(">d", mut[49].context[3]))[0] ^ 1))[0]
ok, motivo, _ = validar_cadena(mut, A1)
control_positivo("voltear 1 bit del Context del veredicto 50", (ok, motivo))

reord = construir(A1, 100)
reord[49], reord[50] = reord[50], reord[49]
ok, motivo, _ = validar_cadena(reord, A1)
control_positivo("reordenar los veredictos 50 y 51", (ok, motivo))

falsa = construir(A1, 100)
falsa[70].signature = hmac.new(b"clave-del-atacante" + b"\x00" * 14,
                               falsa[70].hash, hashlib.sha256).digest()
ok, motivo, _ = validar_cadena(falsa, A1)
control_positivo("refirmar el veredicto 71 con otra clave HMAC", (ok, motivo))

# CP-5 <- HALLAZGO DE LA MATRIZ DE MUTACION (E-006).
# Apague el chequeo de prev_hash y el script siguio dando rc=0: NINGUN control
# positivo lo tocaba, porque el de seq agarra el borrado y el reordenamiento
# primero. O sea que la cadena de hash, que ES el producto, estaba sin probar.
# El ataque que lo aisla es el que haria un atacante competente: borra el 50 y
# RENUMERA el resto, asi que seq queda contiguo 1..99 y lo unico roto es el
# eslabon. Renumerar es trivial: es un UPDATE.
renum = []
prev_falso = CERO32
for j, v in enumerate([x for k, x in enumerate(base) if k != 49], start=1):
    w = Verdict(seq=j, prev_hash=prev_falso, agent_id=v.agent_id,
                observed_at=v.observed_at, rule=v.rule, packet_ts=v.packet_ts,
                epoch=v.epoch, nonce=v.nonce, payload_len=v.payload_len,
                context=v.context, backoff_ns=v.backoff_ns, streak=v.streak,
                dropped_since=v.dropped_since)
    # el atacante NO recalcula el eslabon: reusa el prev_hash que ya estaba.
    w.prev_hash = v.prev_hash
    w.sellar()
    renum.append(w)
    prev_falso = w.hash
ok, motivo, _ = validar_cadena(renum, A1)
control_positivo("borrar el 50 y RENUMERAR el resto: seq queda contiguo y solo "
                 "se rompe el eslabon de hash", (ok, motivo))

# truncamiento de cola <- D-20, el agujero que encontro la auditoria
cola = construir(A1, 100)[:90]
ok_sin, motivo_sin, _ = validar_cadena(cola, A1)
print(f"  [ .. ] borrar los ULTIMOS 10 SIN Sello: "
      f"{'VERDE' if ok_sin else 'ROJO'}  <-- D-20 reproducido")
sello_cola = sello_de({A1: construir(A1, 100), A2: construir(A2, 40)})
ok, motivo, _ = validar_cadena(cola, A1, sello=sello_cola)
control_positivo("borrar los ULTIMOS 10 (truncamiento de cola), con Sello",
                 (ok, motivo))
propiedad("D-20 sin Sello el truncamiento de cola verifica en VERDE "
          "[sintetico: no detecta el arreglo]", ok_sin)

# cadena entera borrada
ok, motivo, _ = validar_cadena([], A1, sello=sello_cola)
control_positivo("borrar la cadena ENTERA del agente, con Sello", (ok, motivo))

# Sello adulterado
sello_adulterado = sello_de({A1: construir(A1, 100), A2: construir(A2, 40)})
sello_adulterado.merkle_root = bytes(
    b ^ 1 for b in sello_adulterado.merkle_root)
ok, motivo, _ = validar_cadena(base, A1, sello=sello_adulterado)
control_positivo("adulterar la raiz Merkle del Sello", (ok, motivo))

# colision de canonicalizacion <- el test D del auditor, corregido
print("  [ .. ] colision de canonicalizacion, al estilo D-06")


def naive(campos):
    """La canonicalizacion mala: valores unidos por un separador, sin escapar.
    Es la forma de canonicalize_event de Custos Legis."""
    return "|".join(campos)


par = (("a|b", "c"), ("a", "b|c"))     # el par CORRECTO: los dos dan a|b|c
n1, n2 = naive(par[0]), naive(par[1])
print(f"         naive({par[0]}) = {n1!r}")
print(f"         naive({par[1]}) = {n2!r}")
print(f"         naive colisiona: {n1 == n2}   <-- TIENE que ser True, "
      f"si no el test no prueba nada")
control_positivo(
    "el canonico NAIVE colisiona con el par ('a|b','c') vs ('a','b|c') "
    "[control del propio test]",
    (n1 != n2, f"naive produce {n1!r} para los dos: colision confirmada"))

# Mejora que le tomo al auditor: mi version anterior comparaba un helper
# `ancho_fijo()` de juguete. Eso probaba mi helper, NO el canonico del
# producto. Ahora el mismo desplazamiento de frontera va metido en dos campos
# reales del Verdict (agent_id y prev_hash) y se compara canonico() de verdad.
b1 = Verdict(seq=1, prev_hash=b"c".ljust(32, b"\x00"),
             agent_id=b"a|b".ljust(32, b"\x00"), observed_at=1, rule=7).sellar()
b2 = Verdict(seq=1, prev_hash=b"b|c".ljust(32, b"\x00"),
             agent_id=b"a".ljust(32, b"\x00"), observed_at=1, rule=7).sellar()
print(f"         canonico() real, frontera corrida entre agent_id y prev_hash:")
print(f"           hash A = {b1.hash.hex()[:32]}...")
print(f"           hash B = {b2.hash.hex()[:32]}...")
print(f"         canonico() real colisiona: {b1.hash == b2.hash}")
invariante("el canonico REAL de Verdict no colisiona al correr la frontera "
           "entre dos campos contiguos", b1.hash != b2.hash)
invariante("dos Verdict que difieren en un solo campo dan hash distinto",
           construir(A1, 1)[0].hash != construir(A2, 1)[0].hash)

# Otra que le tomo: meter el valor de ataque ADENTRO de una cadena real, para
# probar que el canonico sobrevive un uint64 al maximo.
ataque = Verdict(seq=1, prev_hash=CERO32, agent_id=A1, observed_at=1, rule=5,
                 packet_ts=2**64 - 1, epoch=1, nonce=1, payload_len=1,
                 context=[0.0] * 8).sellar()
ok_at, motivo_at, _ = validar_cadena([ataque], A1)
print(f"   veredicto con packet_ts = 2^64-1 dentro de la cadena: "
      f"{'VERDE' if ok_at else 'ROJO'}")
invariante("el canonico serializa packet_ts = 2^64-1 sin desbordar "
           "(por eso la columna va numeric(20), no bigint)", ok_at)

# ======================================================================
print("\n== D. la regla ingenua del ancla (head != ancla) da ROJO FALSO ==")
# Guard de regresion. Si alguien "simplifica" _validar_contra_sello a comparar
# heads, este bloque lo agarra: toda cadena viva entre dos checkpoints daria
# manipulacion. Es el defecto que tenia la propuesta original del ancla.
def validar_regla_ingenua(vs, sello, agent_id):
    seq_sellada, head_sellado = sello.instantanea[agent_id]
    return (vs[-1].hash == head_sellado) if vs else False


print(f"   cadena de 100 contra su Sello, regla ingenua: "
      f"{'VERDE' if validar_regla_ingenua(base, sello_base, A1) else 'ROJO'}")
print(f"   MISMA cadena + 1 veredicto legitimo, regla ingenua: "
      f"{'VERDE' if validar_regla_ingenua(crecida, sello_base, A1) else 'ROJO'}"
      f"   <-- cadena SANA")
invariante("la regla implementada NO se rompe con el crecimiento, y la ingenua "
           "SI (por eso no se usa)",
           ok_c and not validar_regla_ingenua(crecida, sello_base, A1))

# ======================================================================
print("\n== E. D-28: cuanto NO cubre el Sello (hallazgo nuevo) ==")
# El Sello fija un punto. Todo lo que crecio DESPUES del ultimo Sello y antes
# del siguiente es borrable sin que nada de rojo. La ventana de exposicion es
# exactamente T, el intervalo de sellado.
larga = construir(A1, 120)
sello_en_100 = Sello(1, CERO32, 1_788_800_100_000_000_000,
                     {A1: (100, larga[99].hash), A2: (40, construir(A2, 40)[-1].hash)})
truncada = larga[:105]                      # el atacante borra 15, no 20
ok_v, motivo_v, _ = validar_cadena(truncada, A1, sello=sello_en_100)
print(f"   sello en seq 100, cadena crecio a 120, atacante trunca a 105:")
print(f"      {'VERDE' if ok_v else 'ROJO'} ({motivo_v})")
print(f"   -> con Sello cada T, la ventana borrable son los veredictos "
      f"posteriores")
print(f"      al ultimo Sello. Aca: 20 veredictos, borro 15, nadie se entera.")
propiedad("D-28 el Sello NO cubre lo emitido despues del ultimo checkpoint: "
          "la ventana de exposicion es T, y T es el parametro de riesgo del "
          "producto [sintetico]", ok_v)
por_debajo = larga[:99]
ok_pd, motivo_pd, _ = validar_cadena(por_debajo, A1, sello=sello_en_100)
control_positivo("truncar por DEBAJO del punto sellado (seq 99 < 100)",
                 (ok_pd, motivo_pd))

# ======================================================================
print("\n== F. la politica de descarte (fix D-21) ==")
mala = construir(A1, 30, descartes={21: 2}, consumir_seq=True)
ok_m, motivo_m, _ = validar_cadena(mala, A1)
print(f"   descarte que CONSUME seq: {'VERDE' if ok_m else 'ROJO'} "
      f"({motivo_m})")
print("   -> indistinguible de un borrado: el grabador fabrica falsa evidencia")
buena = construir(A1, 30, descartes={21: 2})
ok_b, motivo_b, info_b = validar_cadena(buena, A1)
print(f"   descarte SIN consumir seq + dropped_since: "
      f"{'VERDE' if ok_b else 'ROJO'}")
print(f"      huecos declarados (seq, perdidos): {info_b['huecos_declarados']}")
print(f"      veredictos perdidos, declarados: {info_b['perdidos']}")
propiedad("D-21 la politica de la spec 6.2 (descarte consume seq) da ROJO falso "
          "[sintetico: el constructor es mio]", not ok_m)
invariante("con el fix, el hueco queda DECLARADO y la cadena en verde",
           ok_b and info_b["huecos_declarados"] == [(21, 2)])

# ======================================================================
print("\n== G. D-25: HMAC simetrico no es no-repudio frente a terceros ==")
refabricada = construir(A1, 100)          # cualquiera con K_VERDICT la rehace
ok_r, motivo_r, _ = validar_cadena(refabricada, A1)
print(f"   cadena REFABRICADA por el tenedor de la clave: "
      f"{'VERDE' if ok_r else 'ROJO'}")
propiedad("D-25 con TESTIS_HMAC_KEY se refabrica la cadena entera y verifica "
          "en verde: tamper-evidence, no no-repudio [propiedad de HMAC, "
          "no flipea nunca]", ok_r)

try:
    from cryptography.hazmat.primitives.asymmetric import ed25519
    priv = ed25519.Ed25519PrivateKey.from_private_bytes(bytes(range(32)))
    pub_ok = priv.public_key()
    pub_mal = ed25519.Ed25519PrivateKey.from_private_bytes(
        bytes(range(1, 33))).public_key()
    firma = priv.sign(sello_base.hash)

    def verifica(pub):
        try:
            pub.verify(firma, sello_base.hash)
            return True
        except Exception:
            return False

    v_ok, v_mal = verifica(pub_ok), verifica(pub_mal)
    print(f"   Sello firmado ed25519, verificado con la publica correcta: "
          f"{'VERDE' if v_ok else 'ROJO'}")
    print(f"   el mismo Sello con OTRA publica: "
          f"{'VERDE' if v_mal else 'ROJO'}")
    invariante("ed25519: la publica correcta verifica el Sello", v_ok)
    control_positivo("verificar el Sello con una clave publica ajena",
                     (v_mal, "firma ed25519 invalida para esa publica"))
    print("   -> con el Sello firmado ed25519 (crypto/ed25519 de la stdlib de")
    print("      Go), la privada no sale del gateway y el perito verifica con")
    print("      la publica. ESO cierra D-25 hacia afuera.")
except ImportError:
    print("   NO MEDIDO: falta `cryptography`, el control de ed25519 no corrio")

# ======================================================================
print("\n== H. estado de la implementacion en el repo (readiness) ==")
GW = os.path.join(os.path.dirname(__file__), "..", "gateway", "gateway.go")
try:
    src = open(GW, encoding="utf-8").read()
    cuerpo = src[src.index("func (g *Gateway) handleConn"):
                 src.index("// validateTimestamp verifica")]
    n_rej = len(re.findall(r"\bwriteReject\(conn\)", cuerpo))
    n_blk = len(re.findall(r"\bTriggerBlock\(now\)", cuerpo))
    n_emit = len(re.findall(r"\brecorder\.Emit\(", cuerpo))
    print(f"   en handleConn: writeReject={n_rej}  TriggerBlock={n_blk}  "
          f"Emit={n_emit}")
    if n_emit == 0:
        print("   -> TESTIS NO IMPLEMENTADO. El chequeo queda ARMADO: cuando "
              "aparezca")
        print("      el primer Emit, este test exige Emit == writeReject y se "
              "pone")
        print("      rojo si falta uno. No falla hoy para no dejar el CI rojo "
              "eternamente.")
        invariante("los 9 rechazos siguen ahi para instrumentar", n_rej == 9)
    else:
        invariante(f"cada writeReject tiene su Emit ({n_emit} vs {n_rej})",
                   n_emit == n_rej)
except (OSError, ValueError) as exc:
    print(f"   NO MEDIDO: no pude leer handleConn ({exc})")

# ======================================================================
print("\n== I. audito los hallazgos de la auditoria contra la fuente ==")
SQL = os.path.join(os.path.dirname(__file__), "..", "audit", "schema.sql")
FSM = os.path.join(os.path.dirname(__file__), "..", "gateway", "fsm.go")
try:
    sql = open(SQL, encoding="utf-8").read()
    fsm = open(FSM, encoding="utf-8").read()

    # D-27 del auditor: "uuid_generate_v7 no existe en PG estandar"
    define_v7 = bool(re.search(
        r"CREATE OR REPLACE FUNCTION\s+uuid_generate_v7", sql))
    print(f"   schema.sql DEFINE uuid_generate_v7() en plpgsql: {define_v7}")
    invariante("D-27 REFUTADO: el esquema define la funcion, no la importa de "
               "una extension", define_v7)

    # D-29: el FSM no expone su estado
    getters = re.findall(r"func \(f \*AgentFSM\) (\w+)", fsm)
    print(f"   metodos publicos del AgentFSM: {getters}")
    print(f"   campos que Testis necesita, y son privados: "
          f"{['backoff', 'successStreak']}")
    hay_getter = any(g.lower().startswith(("snapshot", "state", "backoff",
                                           "streak")) for g in getters)
    print(f"   existe algun getter de backoff/streak: {hay_getter}")
    defecto("D-29 el AgentFSM no expone backoff ni successStreak (privados, "
            "sin getter): Testis NO puede llenar BackoffNs ni Streak sin "
            "agregarle un Snapshot() al FSM", not hay_getter)

    # D-26: lastSeen por conexion
    gw = open(GW, encoding="utf-8").read()
    cuerpo2 = gw[gw.index("func (g *Gateway) handleConn"):]
    local = "lastSeen := time.Now()" in cuerpo2
    en_state = "lastSeen" in gw[:gw.index("func (g *Gateway) handleConn")]
    print(f"   lastSeen declarado DENTRO de handleConn: {local}   "
          f"en AgentState: {en_state}")
    defecto("D-26 CONFIRMADO: lastSeen (el dt del Huber) es local a la "
            "conexion, no al agente", local and not en_state)
except (OSError, ValueError) as exc:
    print(f"   NO MEDIDO: {exc}")

# ======================================================================
print("\n== NO MEDIDO ==")
for i, t in enumerate([
    "El costo del fsync por veredicto y la tasa a la que el WAL se vuelve "
    "cuello de botella. Group-commit es hipotesis sin numero.",
    "Nada de esto corrio contra Postgres: `verdicts` y `anchors` no existen.",
    "Cual es el T de sellado correcto (D-28). Depende del riesgo que "
    "aceptes por ventana, y eso es decision tuya, no medicion mia.",
    "Si el gateway Go compila. Sigue sin compilar y el CI sin leerse.",
    "Si el Sello publicado fuera del alcance del DBA es operativamente "
    "posible en el deployment real.",
    "4 de los 6 hallazgos de este archivo son PROPIEDAD, no DEFECTO: no leen "
    "el repo, asi que no pueden avisar cuando el Go se arregle. Lo que falta "
    "para convertirlos es un cliente mTLS y un Postgres, no otro test.",
], 1):
    print(f"  {i}. {t}")

print(f"\ncontroles positivos ejecutados: {_cp}")
print(f"fallos={fallos}")
sys.exit(1 if fallos else 0)
