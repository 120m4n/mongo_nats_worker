-- ==========================================
-- 005_continuous_aggregate.sql
-- ==========================================
-- Vista materializada continua: última posición por dispositivo

CREATE MATERIALIZED VIEW IF NOT EXISTS latest_device_position
WITH (timescaledb.continuous) AS
SELECT
    device_id,
    fleet,
    last(ts, ts)        AS last_seen,
    last(latitude, ts)  AS latitude,
    last(longitude, ts) AS longitude,
    last(geom, ts)      AS geom,
    last(user_id, ts)   AS user_id
FROM coordinates_history
GROUP BY device_id, fleet
WITH NO DATA;

-- Política de refresco continuo: cada 30 segundos
SELECT add_continuous_aggregate_policy(
    'latest_device_position',
    start_offset    => INTERVAL '1 hour',
    end_offset      => INTERVAL '30 seconds',
    schedule_interval => INTERVAL '30 seconds',
    if_not_exists => TRUE
);
