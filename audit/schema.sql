-- Copyright (c) 2026 Jorge Abraham Mendieta.
-- Computational Substrate Theory. Todos los derechos reservados.

CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- FIX: función uuid_v7 más robusta con manejo explícito de desbordamiento.
CREATE OR REPLACE FUNCTION uuid_generate_v7()
RETURNS uuid
LANGUAGE plpgsql
VOLATILE
AS $$
DECLARE
    ts_ms      bigint;
    ts_bytes   bytea;
    raw_bytes  bytea;
    v          int;
    hex_str    text;
    uuid_str   text;
BEGIN
    ts_ms    := (floor(extract(epoch FROM clock_timestamp()) * 1000))::bigint;
    ts_bytes := int8send(ts_ms);
    raw_bytes := gen_random_bytes(16);

    -- Primeros 6 bytes = 48 bits del timestamp.
    raw_bytes := overlay(raw_bytes
                   placing substring(ts_bytes from 3 for 6)
                   from 1 for 6);

    -- Versión 7 en nibble alto del byte 6.
    v := get_byte(raw_bytes, 6);
    v := (v & x'0F'::int) | x'70'::int;
    raw_bytes := set_byte(raw_bytes, 6, v);

    -- Variante RFC 4122 en bits altos del byte 8.
    v := get_byte(raw_bytes, 8);
    v := (v & x'3F'::int) | x'80'::int;
    raw_bytes := set_byte(raw_bytes, 8, v);

    hex_str  := encode(raw_bytes, 'hex');
    uuid_str := substr(hex_str,  1, 8) || '-' ||
                substr(hex_str,  9, 4) || '-' ||
                substr(hex_str, 13, 4) || '-' ||
                substr(hex_str, 17, 4) || '-' ||
                substr(hex_str, 21, 12);

    RETURN uuid_str::uuid;
END;
$$;

CREATE TABLE IF NOT EXISTS audit_logs (
    id             uuid        PRIMARY KEY DEFAULT uuid_generate_v7(),
    event_type     text        NOT NULL,
    agent_id       text,
    occurred_at    timestamptz NOT NULL DEFAULT clock_timestamp(),
    metadata       text        NOT NULL,
    hmac_signature bytea       NOT NULL,
    created_at     timestamptz NOT NULL DEFAULT clock_timestamp()
);

-- Índices para consultas frecuentes.
CREATE INDEX IF NOT EXISTS idx_audit_logs_occurred_at
    ON audit_logs (occurred_at DESC);

CREATE INDEX IF NOT EXISTS idx_audit_logs_agent_id
    ON audit_logs (agent_id)
    WHERE agent_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_audit_logs_event_type
    ON audit_logs (event_type, occurred_at DESC);

-- Tablas del Fleet Manager.
CREATE TABLE IF NOT EXISTS api_keys (
    tenant_id  text  NOT NULL,
    key_hash   text  NOT NULL,
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY (tenant_id, key_hash)
);

CREATE TABLE IF NOT EXISTS edge_nodes (
    tenant_id      text        NOT NULL,
    node_id        text        NOT NULL,
    mqtt_username  text        NOT NULL,
    registered_at  timestamptz NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY (tenant_id, node_id)
);

CREATE TABLE IF NOT EXISTS telemetry_kappa (
    id          bigserial   PRIMARY KEY,
    tenant_id   text        NOT NULL,
    node_id     text        NOT NULL,
    kappa       double precision NOT NULL,
    recorded_at timestamptz NOT NULL DEFAULT clock_timestamp()
);

CREATE INDEX IF NOT EXISTS idx_telemetry_kappa_node
    ON telemetry_kappa (tenant_id, node_id, recorded_at DESC);

CREATE TABLE IF NOT EXISTS ota_packages (
    tenant_id      text        NOT NULL,
    package_id     text        NOT NULL,
    version        text        NOT NULL DEFAULT 'unknown',
    binary_hash    text        NOT NULL,
    bytes_received integer     NOT NULL,
    storage_path   text        NOT NULL, -- FIX: ruta donde vive el binario
    created_at     timestamptz NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY (tenant_id, package_id)
);
