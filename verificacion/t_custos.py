#!/usr/bin/env python3
"""Verificador de Custos Legis (KAMPE IR) sin necesidad de asyncpg ni Postgres.

Extrae por AST las piezas puras del modulo (el HMAC pre-hasheado y la
canonicalizacion) y las ejecuta tal como estan escritas en el archivo.
Instrumento independiente: hmac/hashlib de la stdlib.

Mide dos cosas separadas:
  INVARIANTES : lo que debe seguir siendo cierto.
  DEFECTOS    : estado medido hoy; si cambia, rojo, para forzar bitacora.
Corrido: 2026-09-07, CPython 3.12.13.
"""
import ast, datetime, hashlib, hmac, json, os, secrets, sys, textwrap

RUTA = os.path.join(os.path.dirname(__file__), "..", "audit", "custos_legis.py")
QUIERO = {"_HmacKeyPreHashed", "_json_dumps_sorted", "_canonicalize_value",
          "canonicalize_event"}

arbol = ast.parse(open(RUTA, encoding="utf-8").read())
piezas = [n for n in arbol.body
          if isinstance(n, (ast.FunctionDef, ast.ClassDef)) and n.name in QUIERO]
faltan = QUIERO - {n.name for n in piezas}
if faltan:
    print("ROJO: no encontre en el modulo:", sorted(faltan)); sys.exit(1)

ns = {"hashlib": hashlib, "json": json, "datetime": datetime,
      "Any": object, "Dict": dict}
exec(compile(ast.Module(body=piezas, type_ignores=[]), RUTA, "exec"), ns)
PreHashed      = ns["_HmacKeyPreHashed"]
canonicalizar  = ns["canonicalize_event"]

fallos = 0
def invariante(nombre, ok):
    global fallos
    print(f"  [{'ok  ' if ok else 'ROJO'}] INVARIANTE  {nombre}")
    if not ok: fallos += 1
def defecto(nombre, sigue):
    global fallos
    print(f"  [{'presente' if sigue else 'CAMBIO'}] DEFECTO     {nombre}")
    if not sigue: fallos += 1

print("== 1. el HMAC casero contra el hmac de la stdlib ==")
iguales = 0
for _ in range(200):
    k = secrets.token_bytes(32)
    m = secrets.token_bytes(secrets.randbelow(300))
    if PreHashed(k).sign(m) == hmac.new(k, m, hashlib.sha256).digest():
        iguales += 1
print(f"   claves de 32 B: {iguales}/200 coinciden")
largo = secrets.token_bytes(200)   # clave mas larga que el bloque de 64 B
igual_largo = (PreHashed(largo).sign(b"x")
               == hmac.new(largo, b"x", hashlib.sha256).digest())
print(f"   clave de 200 B (> bloque): coincide = {igual_largo}")
k = secrets.token_bytes(32)
control = PreHashed(k).sign(b"payload") != hmac.new(k, b"payload!", hashlib.sha256).digest()
print(f"   control positivo (mensaje distinto NO coincide): {control}")
invariante("_HmacKeyPreHashed == HMAC-SHA256 de la stdlib, 200/200", iguales == 200)
invariante("clave > 64 B se pre-hashea segun RFC 2104", igual_largo)
invariante("control positivo: un byte distinto rompe la firma", control)

print("\n== 2. colision de canonicalizacion (el punto del log de auditoria) ==")
# NOTA: mi primer par de prueba NO colisiono y la causa fue mia, no del
# codigo: elegi una clave inyectada que ordena DESPUES de la siguiente clave
# real. El instrumento me falso y corregi el caso. Para colisionar, la clave
# inyectada tiene que caer en la misma posicion del orden alfabetico.
a = {"accion": "pausar&agent=n1"}          # un solo campo, con & y = adentro
b = {"accion": "pausar", "agent": "n1"}    # dos campos legitimos
ca, cb = canonicalizar(a), canonicalizar(b)
print(f"   evento A = {a}\n   evento B = {b}")
print(f"   canonico A = {ca!r}\n   canonico B = {cb!r}")
firma = PreHashed(k)
fa, fb = firma.sign(ca.encode()), firma.sign(cb.encode())
print(f"   HMAC A = {fa.hex()[:32]}...\n   HMAC B = {fb.hex()[:32]}...")
print(f"   misma firma para eventos distintos: {fa == fb}")
defecto("D-06 canonicalize_event usa k=v unido por & sin escapar: dos eventos "
        "DISTINTOS producen el mismo canonico y la misma firma HMAC",
        ca == cb and fa == fb)

print("\n== 3. importar el modulo tiene efecto de lado ==")
tiene_side_effect = any(
    isinstance(n, ast.Assign) and any(
        isinstance(v, ast.Call) and getattr(v.func, "id", "") == "_load_hmac_keys"
        for v in ast.walk(n))
    for n in arbol.body)
print(f"   _load_hmac_keys() se ejecuta a nivel de modulo: {tiene_side_effect}")
defecto("D-07 importar custos_legis lanza EnvironmentError sin las dos env "
        "vars: rompe tests, tooling y cualquier import", tiene_side_effect)

print(f"\nfallos={fallos}")
sys.exit(1 if fallos else 0)
