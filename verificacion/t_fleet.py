#!/usr/bin/env python3
"""Verificador del Fleet Manager (KAMPE IR) sin fastapi, asyncpg ni broker.

Mide tres cosas que no necesitan servidor: (1) si el delivery_token que el
endpoint de descarga exige llega a existir en algun lado, (2) si el tamano
exacto que el OTA acepta coincide con lo que el motor tiene en memoria, y
(3) si el usuario MQTT derivado de tenant+node es inyectivo.

Instrumentos: AST del propio archivo, texto de audit/schema.sql, aritmetica.
Corrido: 2026-09-07, CPython 3.12.13.
"""
import ast, os, re, sys

BASE   = os.path.join(os.path.dirname(__file__), "..")
FLEET  = os.path.join(BASE, "fleet", "fleet_manager.py")
SCHEMA = os.path.join(BASE, "audit", "schema.sql")
ENGINE = os.path.join(BASE, "dualbrain", "engine.hpp")

fuente = open(FLEET, encoding="utf-8").read()
schema = open(SCHEMA, encoding="utf-8").read()
motor  = open(ENGINE, encoding="utf-8").read()

fallos = 0
def invariante(n, ok):
    global fallos
    print(f"  [{'ok  ' if ok else 'ROJO'}] INVARIANTE  {n}")
    if not ok: fallos += 1
def defecto(n, sigue):
    global fallos
    print(f"  [{'presente' if sigue else 'CAMBIO'}] DEFECTO     {n}")
    if not sigue: fallos += 1

print("== 1. el delivery_token que /ota/download exige ==")
apariciones = [l.strip() for l in fuente.splitlines() if "delivery_token" in l]
for l in apariciones:
    print(f"   {l}")
en_schema = "delivery_token" in schema
comparado = bool(re.search(r"delivery_token\s*(==|!=)|compare_digest", fuente))
print(f"   apariciones en el codigo: {len(apariciones)}")
print(f"   columna en audit/schema.sql: {en_schema}")
print(f"   se compara alguna vez contra algo: {comparado}")
defecto("D-08 el header X-Delivery-Token se exige, nunca se persiste y nunca "
        "se compara: cualquier valor entra", not en_schema and not comparado)

print("\n== 2. cuantos bytes acepta el OTA vs cuantos tiene el motor ==")
ota = None
for nodo in ast.walk(ast.parse(fuente)):
    if (isinstance(nodo, ast.Assign) and getattr(nodo.targets[0], "id", "")
            == "CENTROIDS_EXPECTED_BYTES"):
        ota = eval(compile(ast.Expression(nodo.value), "x", "eval"))
        print(f"   CENTROIDS_EXPECTED_BYTES = {ota} ({ast.unparse(nodo.value)})")
centroides = 1024 * 64 * 2                      # int16_t[1024][64] en el header
sigma      = 1024 * 64 * 2
kappa      = 1024 * 4
print(f"   engine.hpp centroids_ int16 : {centroides}")
print(f"   engine.hpp + sigma_sq_      : {centroides+sigma}")
print(f"   + kappa_ (los 3 buffers)    : {centroides+sigma+kappa}")
print(f"   sizeof(Engine_v4_3) medido  : {262*1024}")
invariante("el numero del OTA es exactamente centroids_ + sigma_sq_",
           ota == centroides + sigma)
defecto("D-09 la formula dice 1024*64*4 (float32) pero el motor guarda int16: "
        "el numero sale bien por casualidad y nombra solo 'CENTROIDS'",
        "1024 * 64 * 4" in fuente and "int16_t" in motor)
defecto("D-09b un volcado de los 3 buffers (266240 B) lo RECHAZA el OTA, y el "
        "sizeof de la clase (268288 B) tampoco entra",
        ota != centroides+sigma+kappa and ota != 262*1024)

print("\n== 3. el usuario MQTT: tenant_id + '_' + node_id ==")
usuario = lambda t, n: f"{t}_{n}"
pares = [(("acme", "node_1"), ("acme_node", "1")),
         (("a", "b_c"),       ("a_b", "c"))]
colisiona = False
for (t1, n1), (t2, n2) in pares:
    u1, u2 = usuario(t1, n1), usuario(t2, n2)
    print(f"   ({t1!r},{n1!r}) -> {u1!r}   vs   ({t2!r},{n2!r}) -> {u2!r}   "
          f"{'COLISION' if u1 == u2 else 'ok'}")
    colisiona |= (u1 == u2)
solo_largo = bool(re.search(r"node_id:\s*str\s*=\s*Field\(\.\.\., min_length=1, max_length=128\)", fuente))
print(f"   node_id se valida solo por longitud (sin charset): {solo_largo}")
defecto("D-10 el usuario MQTT tenant_'_'node no es inyectivo: dos pares "
        "(tenant,node) distintos dan el mismo %u, y el ACL aisla por %u",
        colisiona)
defecto("D-11 node_id acepta '/', '#' y '+' porque solo se valida la longitud, "
        "y va crudo al topico y al usuario MQTT", solo_largo)

print(f"\nfallos={fallos}")
sys.exit(1 if fallos else 0)
