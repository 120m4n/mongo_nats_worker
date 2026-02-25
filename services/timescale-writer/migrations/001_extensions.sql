-- ==========================================
-- 001_extensions.sql
-- ==========================================
-- Extensiones requeridas para TimescaleDB + PostGIS

CREATE EXTENSION IF NOT EXISTS timescaledb CASCADE;
CREATE EXTENSION IF NOT EXISTS postgis CASCADE;
