# Copyright (c) 2026 Jorge Abraham Mendieta.
# Computational Substrate Theory. Todos los derechos reservados.

import asyncio
import datetime
import hashlib
import hmac
import json
import logging
import os
import uuid  # FIX CRÍTICO: faltaba este import en verify_entry
from typing import Any, Dict, Optional, Union

import asyncpg

logger = logging.getLogger("custos_legis.audit")


# ----------------------------------------------------------------------
# Carga de claves desde variables de entorno (Zero-Trust)
# ----------------------------------------------------------------------
def _load_hmac_keys() -> tuple[bytes, bytes]:
    current_hex  = os.environ.get("AUDIT_HMAC_CURRENT")
    previous_hex = os.environ.get("AUDIT_HMAC_PREVIOUS")

    if not current_hex:
        raise EnvironmentError("AUDIT_HMAC_CURRENT no está definida")
    if not previous_hex:
        raise EnvironmentError("AUDIT_HMAC_PREVIOUS no está definida")

    try:
        current  = bytes.fromhex(current_hex)
        previous = bytes.fromhex(previous_hex)
    except ValueError as exc:
        raise EnvironmentError(
            "Las claves HMAC deben estar en formato hexadecimal"
        ) from exc

    if len(current) != 32 or len(previous) != 32:
        raise EnvironmentError(
            "Las claves HMAC deben tener 32 bytes (SHA-256)"
        )

    return current, previous


class _HmacKeyPreHashed:
    """
    Pre-calcula ipad/opad para HMAC-SHA256, eliminando la recreación
    del contexto en cada firma. Las claves en claro nunca se almacenan.
    """

    __slots__ = ("_opad", "_ipad")

    def __init__(self, key: bytes):
        block_size = 64  # SHA-256
        if len(key) > block_size:
            key = hashlib.sha256(key).digest()
        if len(key) < block_size:
            key = key + b"\x00" * (block_size - len(key))

        self._ipad = bytes(x ^ 0x36 for x in key)
        self._opad = bytes(x ^ 0x5C for x in key)

    def sign(self, data: bytes) -> bytes:
        inner = hashlib.sha256(self._ipad + data).digest()
        return hashlib.sha256(self._opad + inner).digest()


# Instancias globales pre-cargadas al importar el módulo.
_raw_current, _raw_previous = _load_hmac_keys()
CURRENT_KEY  = _HmacKeyPreHashed(_raw_current)
PREVIOUS_KEY = _HmacKeyPreHashed(_raw_previous)

# Eliminar referencias a claves en claro de la memoria del proceso.
del _raw_current, _raw_previous


# ----------------------------------------------------------------------
# Canonicalización determinista
# ----------------------------------------------------------------------
def _json_dumps_sorted(obj: Any) -> str:
    return json.dumps(
        obj,
        sort_keys=True,
        separators=(",", ":"),
        ensure_ascii=False,
    )


def _canonicalize_value(value: Any) -> str:
    if value is None:
        return "null"
    if isinstance(value, bool):
        return "true" if value else "false"
    if isinstance(value, datetime.datetime):
        return value.astimezone(datetime.timezone.utc).isoformat()
    if isinstance(value, dict):
        return _json_dumps_sorted(value)
    if isinstance(value, (list, tuple)):
        return _json_dumps_sorted(list(value))
    if isinstance(value, (int, float, str)):
        return str(value)
    return str(value)


def canonicalize_event(event: Dict[str, Any]) -> str:
    if not isinstance(event, dict):
        raise TypeError("El evento debe ser un diccionario")

    sorted_keys = sorted(event.keys())
    parts = []
    for key in sorted_keys:
        parts.append(f"{key}={_canonicalize_value(event[key])}")
    return "&".join(parts)


# ----------------------------------------------------------------------
# Firma y verificación HMAC en tiempo constante
# ----------------------------------------------------------------------
def _sign_canonical(canonical_str: str, key: _HmacKeyPreHashed) -> bytes:
    return key.sign(canonical_str.encode("utf-8"))


def _verify_canonical(canonical_str: str, provided_signature: bytes) -> bool:
    sig_current  = _sign_canonical(canonical_str, CURRENT_KEY)
    if hmac.compare_digest(provided_signature, sig_current):
        return True

    sig_previous = _sign_canonical(canonical_str, PREVIOUS_KEY)
    if hmac.compare_digest(provided_signature, sig_previous):
        return True

    return False


# ----------------------------------------------------------------------
# Operaciones atómicas con asyncpg
# ----------------------------------------------------------------------
async def write(
    conn: asyncpg.Connection,
    event: Dict[str, Any],
    event_type: str,
    agent_id: Optional[str] = None,
) -> str:
    if not isinstance(conn, asyncpg.Connection):
        raise TypeError("conn debe ser una conexión asyncpg válida")

    canonical_str = canonicalize_event(event)
    signature     = _sign_canonical(canonical_str, CURRENT_KEY)
    occurred_at   = datetime.datetime.now(datetime.timezone.utc)

    try:
        row = await conn.fetchrow(
            """
            INSERT INTO audit_logs
                (event_type, agent_id, occurred_at, metadata, hmac_signature)
            VALUES ($1, $2, $3, $4, $5)
            RETURNING id
            """,
            event_type,
            agent_id,
            occurred_at,
            canonical_str,
            signature,
        )
    except Exception as exc:
        logger.error("Fallo al insertar auditoría: %s", exc, exc_info=True)
        raise

    return str(row["id"])


async def verify_entry(
    conn: asyncpg.Connection,
    entry_id: Union[str, uuid.UUID],  # FIX: uuid ahora sí está importado
) -> bool:
    # FIX: conversión segura con manejo de tipo explícito.
    if isinstance(entry_id, str):
        try:
            entry_id = uuid.UUID(entry_id)
        except ValueError as exc:
            raise ValueError(f"ID de entrada no válido: {entry_id}") from exc

    row = await conn.fetchrow(
        """
        SELECT metadata, hmac_signature
        FROM audit_logs
        WHERE id = $1
        """,
        entry_id,
    )

    if row is None:
        raise LookupError(
            f"Entrada de auditoría no encontrada: {entry_id}"
        )

    canonical_str      = row["metadata"]
    provided_signature = row["hmac_signature"]

    if not isinstance(canonical_str, str) or \
       not isinstance(provided_signature, bytes):
        logger.error(
            "Datos corruptos en fila de auditoría %s", entry_id
        )
        return False

    return _verify_canonical(canonical_str, provided_signature)
