-- ==========================================
-- 003_create_indexes.sql
-- ==========================================
-- Índices optimizados para TimescaleDB + PostGIS

-- BRIN para rangos temporales (ultra compacto, ideal para millones de puntos)
CREATE INDEX IF NOT EXISTS idx_coordinates_ts_brin
    ON coordinates_history USING BRIN (ts)
    WITH (pages_per_range = 32);

-- B-tree compuesto para "última posición de dispositivo X"
-- Patrón: WHERE device_id = 'X' ORDER BY ts DESC LIMIT 1
CREATE INDEX IF NOT EXISTS idx_coordinates_device_time
    ON coordinates_history (device_id, ts DESC);

-- GIST espacial para queries PostGIS (ST_DWithin, ST_Contains, ST_Within)
CREATE INDEX IF NOT EXISTS idx_coordinates_geom_gist
    ON coordinates_history USING GIST (geom);

-- B-tree para filtrado por flota + tiempo
CREATE INDEX IF NOT EXISTS idx_coordinates_fleet_time
    ON coordinates_history (fleet, ts DESC);
