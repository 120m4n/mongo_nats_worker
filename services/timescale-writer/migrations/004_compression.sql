-- ==========================================
-- 004_compression.sql
-- ==========================================
-- Compresión nativa de TimescaleDB

-- Habilitar compresión con segmentación por device_id y fleet
ALTER TABLE coordinates_history SET (
    timescaledb.compress,
    timescaledb.compress_segmentby = 'device_id, fleet',
    timescaledb.compress_orderby = 'ts DESC'
);

-- Auto-comprimir chunks mayores a 3 días
SELECT add_compression_policy(
    'coordinates_history',
    compress_after => INTERVAL '3 days',
    if_not_exists => TRUE
);

-- Retención: eliminar datos mayores a 90 días
SELECT add_retention_policy(
    'coordinates_history',
    drop_after => INTERVAL '90 days',
    if_not_exists => TRUE
);
