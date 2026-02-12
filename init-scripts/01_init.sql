-- ==========================================
-- SETUP INICIAL: GPS TRACKING SYSTEM
-- ==========================================

-- Habilitar extensión TimescaleDB
CREATE EXTENSION IF NOT EXISTS timescaledb;
CREATE EXTENSION IF NOT EXISTS postgis;

-- ==========================================
-- 1. TABLA PRINCIPAL: Posiciones GPS
-- ==========================================
CREATE TABLE IF NOT EXISTS gps_positions (
    time TIMESTAMPTZ NOT NULL,
    device_id TEXT NOT NULL,
    user_id TEXT NOT NULL,
    fleet TEXT NOT NULL,
    latitude DOUBLE PRECISION NOT NULL,
    longitude DOUBLE PRECISION NOT NULL,
    altitude DOUBLE PRECISION,
    speed DOUBLE PRECISION,
    heading DOUBLE PRECISION,
    accuracy DOUBLE PRECISION,
    battery_level INTEGER,
    origin_ip TEXT,
    metadata JSONB,
    geom GEOGRAPHY(POINT, 4326),
    
    PRIMARY KEY (time, device_id)
);

-- Convertir a hypertable (particionamiento automático por tiempo)
SELECT create_hypertable('gps_positions', 'time', 
                         chunk_time_interval => INTERVAL '1 day',
                         if_not_exists => TRUE);

-- Índices optimizados
CREATE INDEX IF NOT EXISTS idx_gps_device_time ON gps_positions (device_id, time DESC);
CREATE INDEX IF NOT EXISTS idx_gps_user_time ON gps_positions (user_id, time DESC);
CREATE INDEX IF NOT EXISTS idx_gps_fleet_time ON gps_positions (fleet, time DESC);
CREATE INDEX IF NOT EXISTS idx_gps_geom ON gps_positions USING GIST(geom);
CREATE INDEX IF NOT EXISTS idx_gps_metadata ON gps_positions USING GIN(metadata);

-- ==========================================
-- 2. FUNCIONES ÚTILES PARA EL WORKER
-- ==========================================

-- Insertar posición con cálculo automático de geometría
CREATE OR REPLACE FUNCTION insert_gps_position(
    p_device_id TEXT,
    p_user_id TEXT,
    p_fleet TEXT,
    p_latitude DOUBLE PRECISION,
    p_longitude DOUBLE PRECISION,
    p_altitude DOUBLE PRECISION DEFAULT NULL,
    p_speed DOUBLE PRECISION DEFAULT NULL,
    p_heading DOUBLE PRECISION DEFAULT NULL,
    p_accuracy DOUBLE PRECISION DEFAULT NULL,
    p_battery INTEGER DEFAULT NULL,
    p_origin_ip TEXT DEFAULT NULL,
    p_metadata JSONB DEFAULT NULL,
    p_time TIMESTAMPTZ DEFAULT NOW()
) RETURNS VOID AS $$
BEGIN
    INSERT INTO gps_positions (
        time, device_id, user_id, fleet, latitude, longitude,
        altitude, speed, heading, accuracy,
        battery_level, origin_ip, metadata, geom
    )
    VALUES (
        p_time, p_device_id, p_user_id, p_fleet, p_latitude, p_longitude,
        p_altitude, p_speed, p_heading, p_accuracy,
        p_battery, p_origin_ip, p_metadata,
        ST_SetSRID(ST_MakePoint(p_longitude, p_latitude), 4326)::geography
    );
END;
$$ LANGUAGE plpgsql;

-- ==========================================
-- 3. RETENCIÓN AUTOMÁTICA
-- ==========================================
-- Datos crudos: 90 días
SELECT add_retention_policy('gps_positions', INTERVAL '90 days', if_not_exists => TRUE);

-- ==========================================
-- 4. USUARIO PARA APLICACIÓN
-- ==========================================
DO $$
BEGIN
    IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'gps_app') THEN
        CREATE USER gps_app WITH PASSWORD 'app_secure_password';
    END IF;
END
$$;

GRANT CONNECT ON DATABASE gps_tracking TO gps_app;
GRANT USAGE ON SCHEMA public TO gps_app;
GRANT SELECT, INSERT ON ALL TABLES IN SCHEMA public TO gps_app;
GRANT USAGE ON ALL SEQUENCES IN SCHEMA public TO gps_app;
GRANT EXECUTE ON FUNCTION insert_gps_position TO gps_app;
