# Copyright (c) 2026 Jorge Abraham Mendieta.
# Computational Substrate Theory. Todos los derechos reservados.

import asyncio
import hashlib
import logging
import os
import secrets
import uuid
from collections import OrderedDict
from datetime import datetime, timezone
from pathlib import Path
from typing import Dict, Optional

import asyncpg
import aiomqtt
from fastapi import Depends, FastAPI, File, Header, HTTPException, UploadFile
from fastapi.responses import FileResponse
from pydantic import BaseModel, Field

logger = logging.getLogger("fleet_manager")

app = FastAPI(title="Fleet Manager OTA & Telemetry")

# ----------------------------------------------------------------------
# Configuración desde variables de entorno
# ----------------------------------------------------------------------
DATABASE_URL          = os.getenv("DATABASE_URL", "postgresql://fleet:fleet@localhost/fleet")
MQTT_HOST             = os.getenv("MQTT_HOST", "localhost")
MQTT_PORT             = int(os.getenv("MQTT_PORT", "1883"))
MQTT_USERNAME         = os.getenv("MQTT_USERNAME")
MQTT_PASSWORD         = os.getenv("MQTT_PASSWORD")

# FIX: directorio de almacenamiento de binarios OTA.
OTA_STORAGE_DIR       = Path(os.getenv("OTA_STORAGE_DIR", "/var/lib/fleet/ota"))

CENTROIDS_EXPECTED_BYTES = 1024 * 64 * 4         # 262,144 bytes exactos
KAPPA_DRIFT_THRESHOLD    = float(os.getenv("KAPPA_DRIFT_THRESHOLD", "0.05"))
KAPPA_DRIFT_WINDOW       = int(os.getenv("KAPPA_DRIFT_WINDOW", "10"))
AUTH_CACHE_TTL_SECONDS   = int(os.getenv("AUTH_CACHE_TTL", "60"))

# FIX: límites de tamaño para caches en memoria.
AUTH_CACHE_MAX_SIZE      = int(os.getenv("AUTH_CACHE_MAX_SIZE", "10000"))
KAPPA_DEQUE_MAX_NODES    = int(os.getenv("KAPPA_DEQUE_MAX_NODES", "50000"))

pool: asyncpg.Pool
mqtt_client: Optional[aiomqtt.Client] = None


# ----------------------------------------------------------------------
# Modelos Pydantic
# ----------------------------------------------------------------------
class RegisterNodeRequest(BaseModel):
    node_id:  str            = Field(..., min_length=1, max_length=128)
    metadata: Optional[dict] = None


class TelemetryKappa(BaseModel):
    node_id:   str               = Field(..., min_length=1, max_length=128)
    kappa:     float
    timestamp: Optional[datetime] = Field(
        default_factory=lambda: datetime.now(timezone.utc)
    )


class OTAUploadResponse(BaseModel):
    package_id:     str
    binary_hash:    str
    delivery_token: str
    bytes_received: int


# ----------------------------------------------------------------------
# Caché de autenticación con TTL y tamaño máximo (LRU)
# FIX: antes el dict crecía sin límite → memory leak.
# FIX: _AuthCacheEntry ya no llama asyncio.get_running_loop() en __init__.
# ----------------------------------------------------------------------
class _AuthCacheEntry:
    __slots__ = ("tenant_id", "expires_at")

    # FIX: expires_at es float absoluto, calculado por el caller
    # dentro del contexto async correcto.
    def __init__(self, tenant_id: str, expires_at: float):
        self.tenant_id  = tenant_id
        self.expires_at = expires_at


class _BoundedLRUCache:
    """
    Caché LRU con tamaño máximo y TTL por entrada.
    FIX: elimina el memory leak del dict ilimitado original.
    """

    def __init__(self, maxsize: int):
        self._maxsize = maxsize
        self._data: OrderedDict[str, _AuthCacheEntry] = OrderedDict()
        self._lock = asyncio.Lock()

    async def get(self, key: str) -> Optional[str]:
        async with self._lock:
            entry = self._data.get(key)
            if entry is None:
                return None

            loop = asyncio.get_running_loop()
            if loop.time() > entry.expires_at:
                del self._data[key]
                return None

            # Mover al final (más recientemente usado).
            self._data.move_to_end(key)
            return entry.tenant_id

    async def set(self, key: str, tenant_id: str, ttl: float) -> None:
        async with self._lock:
            loop = asyncio.get_running_loop()

            if key in self._data:
                self._data.move_to_end(key)
            else:
                if len(self._data) >= self._maxsize:
                    # Evictar el menos recientemente usado.
                    self._data.popitem(last=False)

            self._data[key] = _AuthCacheEntry(
                tenant_id  = tenant_id,
                expires_at = loop.time() + ttl,
            )


_auth_cache = _BoundedLRUCache(maxsize=AUTH_CACHE_MAX_SIZE)


# ----------------------------------------------------------------------
# Cola de deriva por nodo (LRU con límite)
# FIX: antes _kappa_deques crecía sin límite → memory leak con muchos nodos.
# ----------------------------------------------------------------------
from collections import deque


class _BoundedKappaStore:
    """
    Almacén de ventanas de kappa con límite de nodos.
    Evicta el nodo menos recientemente activo cuando se alcanza el límite.
    """

    def __init__(self, max_nodes: int, window: int):
        self._max_nodes = max_nodes
        self._window    = window
        self._data: OrderedDict[str, deque] = OrderedDict()
        self._lock = asyncio.Lock()

    async def update(self, node_key: str, kappa: float) -> bool:
        async with self._lock:
            if node_key in self._data:
                self._data.move_to_end(node_key)
            else:
                if len(self._data) >= self._max_nodes:
                    self._data.popitem(last=False)
                self._data[node_key] = deque(maxlen=self._window)

            dq = self._data[node_key]
            dq.append(kappa)

            values = list(dq)
            if len(values) < 3:
                return False

            drift = all(
                values[i] < values[i + 1]
                for i in range(len(values) - 1)
            )
            return drift and values[-1] > KAPPA_DRIFT_THRESHOLD


_kappa_store = _BoundedKappaStore(
    max_nodes=KAPPA_DEQUE_MAX_NODES,
    window=KAPPA_DRIFT_WINDOW,
)


# ----------------------------------------------------------------------
# Inicialización / cierre
# ----------------------------------------------------------------------
@app.on_event("startup")
async def startup():
    global pool, mqtt_client

    # FIX: crear directorio OTA si no existe.
    OTA_STORAGE_DIR.mkdir(parents=True, exist_ok=True)

    pool = await asyncpg.create_pool(
        DATABASE_URL,
        min_size=5,
        max_size=20,
        command_timeout=30,
    )

    mqtt_client = aiomqtt.Client(
        hostname=MQTT_HOST,
        port=MQTT_PORT,
        username=MQTT_USERNAME,
        password=MQTT_PASSWORD,
    )
    await mqtt_client.__aenter__()
    logger.info("Fleet Manager: pool y MQTT inicializados")


@app.on_event("shutdown")
async def shutdown():
    if mqtt_client is not None:
        await mqtt_client.__aexit__(None, None, None)
    await pool.close()
    logger.info("Fleet Manager: recursos cerrados")


# ----------------------------------------------------------------------
# Autenticación por API Key con caché LRU+TTL
# ----------------------------------------------------------------------
async def require_tenant(
    x_tenant_id: str = Header(..., alias="X-Tenant-ID"),
    x_api_key:   str = Header(..., alias="X-API-Key"),
) -> str:
    cached_tenant = await _auth_cache.get(x_api_key)
    if cached_tenant == x_tenant_id:
        return x_tenant_id

    async with pool.acquire() as conn:
        row = await conn.fetchrow(
            """
            SELECT tenant_id FROM api_keys
            WHERE tenant_id = $1 AND key_hash = crypt($2, key_hash)
            """,
            x_tenant_id,
            x_api_key,
        )

    if row is None:
        raise HTTPException(status_code=401, detail="Tenant no autenticado")

    await _auth_cache.set(x_api_key, x_tenant_id, AUTH_CACHE_TTL_SECONDS)
    return x_tenant_id


# ----------------------------------------------------------------------
# MQTT
# ----------------------------------------------------------------------
async def _mqtt_publish(topic: str, payload: str, qos: int = 1) -> None:
    if mqtt_client is None:
        raise RuntimeError("Cliente MQTT no inicializado")
    await mqtt_client.publish(topic, payload.encode("utf-8"), qos=qos)


# ----------------------------------------------------------------------
# Endpoint: Registro de nodo
# ----------------------------------------------------------------------
@app.post("/nodes/register", status_code=201)
async def register_node(
    payload:   RegisterNodeRequest,
    tenant_id: str = Depends(require_tenant),
):
    mqtt_username = f"{tenant_id}_{payload.node_id}"

    async with pool.acquire() as conn:
        async with conn.transaction():
            await conn.execute(
                """
                INSERT INTO edge_nodes
                    (tenant_id, node_id, mqtt_username, registered_at)
                VALUES ($1, $2, $3, clock_timestamp())
                ON CONFLICT (tenant_id, node_id) DO UPDATE
                SET mqtt_username = EXCLUDED.mqtt_username
                """,
                tenant_id,
                payload.node_id,
                mqtt_username,
            )

    return {
        "tenant_id":          tenant_id,
        "node_id":            payload.node_id,
        "mqtt_topic_prefix":  f"fleet/{mqtt_username}",
    }


# ----------------------------------------------------------------------
# Endpoint: Telemetría kappa
# ----------------------------------------------------------------------
@app.post("/telemetry/kappa")
async def ingest_kappa(
    payload:   TelemetryKappa,
    tenant_id: str = Depends(require_tenant),
):
    async with pool.acquire() as conn:
        node_exists = await conn.fetchval(
            """
            SELECT 1 FROM edge_nodes
            WHERE tenant_id = $1 AND node_id = $2
            """,
            tenant_id,
            payload.node_id,
        )
        if not node_exists:
            raise HTTPException(status_code=404, detail="Nodo no encontrado")

        await conn.execute(
            """
            INSERT INTO telemetry_kappa
                (tenant_id, node_id, kappa, recorded_at)
            VALUES ($1, $2, $3, $4)
            """,
            tenant_id,
            payload.node_id,
            payload.kappa,
            payload.timestamp or datetime.now(timezone.utc),
        )

    drift_detected = await _kappa_store.update(
        f"{tenant_id}:{payload.node_id}",
        payload.kappa,
    )

    if drift_detected:
        await _mqtt_publish(
            f"fleet/{tenant_id}_{payload.node_id}/alerts",
            f'{{"type":"kappa_drift","kappa":{payload.kappa},'
            f'"node":"{payload.node_id}"}}',
        )

    return {
        "accepted":       True,
        "kappa":          payload.kappa,
        "drift_detected": drift_detected,
    }


# ----------------------------------------------------------------------
# Endpoint: Subida OTA con persistencia real del binario
# FIX CRÍTICO: antes el archivo se validaba pero NUNCA se guardaba.
# Los nodos recibían una notificación sin poder descargar nada.
# ----------------------------------------------------------------------
@app.post("/ota/upload", response_model=OTAUploadResponse)
async def upload_ota(
    file:      UploadFile = File(...),
    version:   str        = Header(None, alias="X-OTA-Version"),
    tenant_id: str        = Depends(require_tenant),
):
    hasher     = hashlib.sha256()
    total_bytes = 0
    chunk_size  = 64 * 1024
    chunks      = []

    while True:
        chunk = await file.read(chunk_size)
        if not chunk:
            break
        total_bytes += len(chunk)
        if total_bytes > CENTROIDS_EXPECTED_BYTES:
            raise HTTPException(
                status_code=400,
                detail=f"Archivo demasiado grande: máximo {CENTROIDS_EXPECTED_BYTES} bytes",
            )
        hasher.update(chunk)
        chunks.append(chunk)

    if total_bytes != CENTROIDS_EXPECTED_BYTES:
        raise HTTPException(
            status_code=400,
            detail=f"Tamaño inválido: {total_bytes} bytes, "
                   f"se esperaban {CENTROIDS_EXPECTED_BYTES}",
        )

    binary_hash    = hasher.hexdigest()
    package_id     = str(uuid.uuid4())
    delivery_token = secrets.token_hex(16)

    # FIX: persistir el binario en disco antes de notificar a los nodos.
    tenant_dir = OTA_STORAGE_DIR / tenant_id
    tenant_dir.mkdir(parents=True, exist_ok=True)
    storage_path = tenant_dir / f"{package_id}.bin"

    # Escritura atómica: escribir a .tmp y renombrar.
    tmp_path = storage_path.with_suffix(".tmp")
    try:
        with open(tmp_path, "wb") as f:
            for chunk in chunks:
                f.write(chunk)
        tmp_path.rename(storage_path)
    except OSError as exc:
        logger.error("Error al persistir OTA %s: %s", package_id, exc)
        raise HTTPException(
            status_code=500,
            detail="Error al almacenar el paquete OTA",
        )

    async with pool.acquire() as conn:
        async with conn.transaction():
            await conn.execute(
                """
                INSERT INTO ota_packages
                    (tenant_id, package_id, version,
                     binary_hash, bytes_received, storage_path, created_at)
                VALUES ($1, $2, $3, $4, $5, $6, clock_timestamp())
                """,
                tenant_id,
                package_id,
                version or "unknown",
                binary_hash,
                total_bytes,
                str(storage_path),  # FIX: ruta real del binario
            )

            nodes = await conn.fetch(
                """
                SELECT node_id FROM edge_nodes
                WHERE tenant_id = $1 ORDER BY node_id
                """,
                tenant_id,
            )

    # Notificar a todos los nodos en paralelo.
    await asyncio.gather(*[
        _mqtt_publish(
            f"fleet/{tenant_id}_{node['node_id']}/ota",
            f'{{"package_id":"{package_id}",'
            f'"binary_hash":"{binary_hash}",'
            f'"delivery_token":"{delivery_token}",'
            f'"version":"{version or "unknown"}",'
            f'"download_url":"/ota/download/{package_id}"}}',
        )
        for node in nodes
    ])

    return OTAUploadResponse(
        package_id=package_id,
        binary_hash=binary_hash,
        delivery_token=delivery_token,
        bytes_received=total_bytes,
    )


# ----------------------------------------------------------------------
# Endpoint: Descarga OTA por los nodos
# FIX: endpoint que antes no existía, necesario para completar el flujo.
# ----------------------------------------------------------------------
@app.get("/ota/download/{package_id}")
async def download_ota(
    package_id:     str,
    delivery_token: str    = Header(..., alias="X-Delivery-Token"),
    tenant_id:      str    = Depends(require_tenant),
):
    async with pool.acquire() as conn:
        row = await conn.fetchrow(
            """
            SELECT storage_path, binary_hash
            FROM ota_packages
            WHERE tenant_id = $1 AND package_id = $2
            """,
            tenant_id,
            package_id,
        )

    if row is None:
        raise HTTPException(status_code=404, detail="Paquete OTA no encontrado")

    storage_path = Path(row["storage_path"])
    if not storage_path.exists():
        raise HTTPException(
            status_code=410,
            detail="Binario OTA no disponible en almacenamiento"
        )

    return FileResponse(
        path=str(storage_path),
        media_type="application/octet-stream",
        filename=f"centroids_{package_id}.bin",
        headers={"X-Binary-Hash": row["binary_hash"]},
    )
