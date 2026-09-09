#!/usr/bin/env python3
"""Verificador del Fleet Manager (KAMPE IR) sin fastapi, asyncpg ni broker.

Mide tres cosas que no necesitan servidor: (1) si el delivery_token que el
endpoint de descarga exige se persiste y se COMPARA (D-08, cerrado el
2026-09-09, con control positivo por mutacion), (2) si el tamano
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
# ----------------------------------------------------------------------
# D-08 CERRADO el 2026-09-09. Esta seccion INVIRTIO su polaridad: antes
# afirmaba que el defecto seguia presente, ahora afirma que el fix esta.
#
# Y el guard cambio de instrumento, porque el viejo era falsable por mi
# propio comentario: buscaba la CADENA "compare_digest" en el fuente, y el
# fix trae un comentario que la nombra para explicar por que no se usa `==`.
# Contando cadenas el archivo da 2; contando LLAMADAS con el AST da 1. Es la
# sexta vez en este proyecto que un guard mio cuenta una palabra en vez de
# leer estructura, y esta vez se caza antes de publicar el numero.
# ----------------------------------------------------------------------
arbol = ast.parse(fuente)

def llamadas_compare_digest(codigo):
    return [n for n in ast.walk(ast.parse(codigo))
            if isinstance(n, ast.Call) and isinstance(n.func, ast.Attribute)
            and n.func.attr == "compare_digest"]

n_cmp     = len(llamadas_compare_digest(fuente))
en_schema = "delivery_token_sha256" in schema
en_claro  = bool(re.search(r"delivery_token\s*(==|!=)\s*", fuente))
hashea    = "hashlib.sha256(delivery_token.encode())" in fuente
rechaza   = "status_code=403" in fuente

apariciones = [l.strip() for l in fuente.splitlines()
               if "delivery_token" in l and not l.strip().startswith("#")]
print(f"   lineas de CODIGO que lo mencionan: {len(apariciones)}")
print(f"   columna delivery_token_sha256 en audit/schema.sql: {en_schema}")
print(f"   llamadas reales a compare_digest (AST): {n_cmp}")
print(f"   el token se hashea antes de comparar: {hashea}")
print(f"   la descarga rechaza con 403: {rechaza}")
print(f"   se compara con '==' en algun lado: {en_claro}  (tiene que ser False)")

invariante("D-08 el delivery_token se PERSISTE hasheado en el schema", en_schema)
invariante("D-08 la descarga lo COMPARA con compare_digest, no con ==",
           n_cmp == 1 and not en_claro)
invariante("D-08 se compara hash contra hash, no el token en claro", hashea)
invariante("D-08 un token invalido corta con 403, no entrega el binario", rechaza)

# CONTROL POSITIVO: si le saco la comparacion al fuente, el guard TIENE que
# ponerse rojo. Sin esto, los cuatro verdes de arriba no distinguen "esta bien"
# de "mi guard no esta mirando".
mutado = fuente.replace(
    "if not secrets.compare_digest(esperado, recibido):",
    "if False:")
cayo = len(llamadas_compare_digest(mutado)) == 0
print(f"   CONTROL POSITIVO: con la comparacion mutada el guard cae: {cayo}")
invariante("D-08 el control positivo por mutacion pone el guard en ROJO", cayo)

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
