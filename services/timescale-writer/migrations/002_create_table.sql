-- ==========================================
-- 002_create_table.sql
-- ==========================================
-- Tabla principal: coordinates_history con TimescaleDB

CREATE TABLE IF NOT EXISTS coordinates_history (
    ts              TIMESTAMPTZ         NOT NULL,
    device_id       TEXT                NOT NULL,
    user_id         TEXT                NOT NULL,
    fleet           TEXT                NOT NULL,
    longitude       DOUBLE PRECISION    NOT NULL,
    latitude        DOUBLE PRECISION    NOT NULL,
    geom            GEOMETRY(Point, 4326) NOT NULL,
    ip_origin       INET
);

-- Convertir a hypertable con chunks de 1 día
SELECT create_hypertable(
    'coordinates_history',
    by_range('ts', INTERVAL '1 day'),
    if_not_exists => TRUE
);
