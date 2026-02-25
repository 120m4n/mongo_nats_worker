# SQL Tips & Tricks - TimescaleDB + PostGIS GPS Queries

Guía de consultas SQL útiles para análisis de datos GPS en TimescaleDB con PostGIS.

---

## 📋 Estructura de Tablas

### Tabla Principal: `coordinates_history`
```sql
- ts              TIMESTAMPTZ (índice BRIN)
- device_id       TEXT (índice B-tree compuesto)
- user_id         TEXT
- fleet           TEXT
- longitude       DOUBLE PRECISION
- latitude        DOUBLE PRECISION
- geom            GEOMETRY(Point, 4326) (índice GIST espacial)
- ip_origin       INET
```

### Vista Materializada: `latest_device_position`
- Actualización automática cada 30 segundos
- Última posición conocida por dispositivo

---

## 🎯 Consultas Comunes

### 1. ¿Dónde estuvo un usuario ayer a mediodía?

```sql
-- Posición exacta o más cercana a las 12:00 PM de ayer
SELECT 
    ts,
    user_id,
    device_id,
    latitude,
    longitude,
    ST_AsText(geom) as location,
    ST_Y(geom) as lat,
    ST_X(geom) as lon
FROM coordinates_history
WHERE user_id = 'user_123'
  AND ts BETWEEN 
      (CURRENT_DATE - INTERVAL '1 day' + TIME '11:55:00') AND
      (CURRENT_DATE - INTERVAL '1 day' + TIME '12:05:00')
ORDER BY ABS(EXTRACT(EPOCH FROM (ts - (CURRENT_DATE - INTERVAL '1 day' + TIME '12:00:00'))))
LIMIT 1;
```

```sql
-- Todas las posiciones del usuario ayer entre 11:00 AM y 1:00 PM
SELECT 
    ts,
    user_id,
    device_id,
    latitude,
    longitude,
    ST_AsGeoJSON(geom) as geojson
FROM coordinates_history
WHERE user_id = 'user_123'
  AND ts >= (CURRENT_DATE - INTERVAL '1 day' + TIME '11:00:00')
  AND ts <= (CURRENT_DATE - INTERVAL '1 day' + TIME '13:00:00')
ORDER BY ts;
```

---

### 2. Usuarios cercanos a un punto (lon, lat)

```sql
-- Dispositivos dentro de 1 km del punto (-99.1332, 19.4326)
-- Usando última posición conocida (vista materializada)
SELECT 
    device_id,
    user_id,
    fleet,
    last_seen,
    latitude,
    longitude,
    ST_Distance(
        geom::geography,
        ST_SetSRID(ST_MakePoint(-99.1332, 19.4326), 4326)::geography
    ) as distance_meters,
    ROUND(
        ST_Distance(
            geom::geography,
            ST_SetSRID(ST_MakePoint(-99.1332, 19.4326), 4326)::geography
        )::numeric, 2
    ) as distance_m
FROM latest_device_position
WHERE ST_DWithin(
    geom::geography,
    ST_SetSRID(ST_MakePoint(-99.1332, 19.4326), 4326)::geography,
    1000  -- 1000 metros = 1 km
)
ORDER BY distance_meters
LIMIT 20;
```

```sql
-- Usuarios cercanos en tiempo real (últimas 5 minutos)
SELECT DISTINCT ON (user_id)
    user_id,
    device_id,
    ts,
    latitude,
    longitude,
    ST_Distance(
        geom::geography,
        ST_SetSRID(ST_MakePoint(-99.1332, 19.4326), 4326)::geography
    ) as distance_meters
FROM coordinates_history
WHERE ts > NOW() - INTERVAL '5 minutes'
  AND ST_DWithin(
      geom::geography,
      ST_SetSRID(ST_MakePoint(-99.1332, 19.4326), 4326)::geography,
      500  -- 500 metros
  )
ORDER BY user_id, ts DESC;
```

---

### 3. Trayectoria de un dispositivo (últimas 24 horas)

```sql
-- Ruta completa con timestamps
SELECT 
    ts,
    device_id,
    user_id,
    latitude,
    longitude,
    ST_AsGeoJSON(geom) as point_geojson
FROM coordinates_history
WHERE device_id = 'device_456'
  AND ts > NOW() - INTERVAL '24 hours'
ORDER BY ts;
```

```sql
-- Crear LineString de la trayectoria
SELECT 
    device_id,
    user_id,
    MIN(ts) as trip_start,
    MAX(ts) as trip_end,
    COUNT(*) as num_points,
    ST_AsGeoJSON(ST_MakeLine(geom ORDER BY ts)) as trajectory_geojson,
    ST_Length(
        ST_MakeLine(geom ORDER BY ts)::geography
    ) as distance_traveled_meters
FROM coordinates_history
WHERE device_id = 'device_456'
  AND ts > NOW() - INTERVAL '24 hours'
GROUP BY device_id, user_id;
```

---

### 4. Dispositivos dentro de un polígono (geofence)

```sql
-- Definir un área rectangular (bounding box)
WITH geofence AS (
    SELECT ST_MakeEnvelope(
        -99.2000, 19.3000,  -- lon_min, lat_min (SW corner)
        -99.1000, 19.5000,  -- lon_max, lat_max (NE corner)
        4326
    ) as boundary
)
SELECT 
    ch.device_id,
    ch.user_id,
    ch.ts,
    ch.latitude,
    ch.longitude
FROM coordinates_history ch, geofence gf
WHERE ch.ts > NOW() - INTERVAL '1 hour'
  AND ST_Within(ch.geom, gf.boundary)
ORDER BY ch.ts DESC;
```

```sql
-- Polígono personalizado (ejemplo: zona centro CDMX)
WITH geofence AS (
    SELECT ST_GeomFromText('POLYGON((
        -99.1700 19.4200,
        -99.1200 19.4200,
        -99.1200 19.4500,
        -99.1700 19.4500,
        -99.1700 19.4200
    ))', 4326) as boundary
)
SELECT 
    device_id,
    user_id,
    fleet,
    last_seen,
    latitude,
    longitude
FROM latest_device_position ldp, geofence gf
WHERE ST_Within(ldp.geom, gf.boundary);
```

---

### 5. Velocidad promedio de un dispositivo

```sql
-- Calcular velocidad entre puntos consecutivos
WITH position_pairs AS (
    SELECT 
        device_id,
        ts,
        geom,
        LAG(ts) OVER (PARTITION BY device_id ORDER BY ts) as prev_ts,
        LAG(geom) OVER (PARTITION BY device_id ORDER BY ts) as prev_geom
    FROM coordinates_history
    WHERE device_id = 'device_456'
      AND ts > NOW() - INTERVAL '1 hour'
)
SELECT 
    device_id,
    ts,
    EXTRACT(EPOCH FROM (ts - prev_ts)) as time_diff_seconds,
    ST_Distance(geom::geography, prev_geom::geography) as distance_meters,
    ROUND(
        (ST_Distance(geom::geography, prev_geom::geography) / 
         NULLIF(EXTRACT(EPOCH FROM (ts - prev_ts)), 0) * 3.6)::numeric,
        2
    ) as speed_kmh
FROM position_pairs
WHERE prev_ts IS NOT NULL
ORDER BY ts;
```

---

### 6. Dispositivos más activos (más reportes)

```sql
-- Top 10 dispositivos con más reportes hoy
SELECT 
    device_id,
    user_id,
    fleet,
    COUNT(*) as num_reports,
    MIN(ts) as first_report,
    MAX(ts) as last_report
FROM coordinates_history
WHERE ts >= CURRENT_DATE
GROUP BY device_id, user_id, fleet
ORDER BY num_reports DESC
LIMIT 10;
```

---

### 7. Heat Map - Densidad de puntos por área

```sql
-- Grid de hexágonos con conteo de puntos (500m de lado)
SELECT 
    ST_AsGeoJSON(ST_SnapToGrid(geom, 0.005)) as grid_cell,
    COUNT(*) as point_count
FROM coordinates_history
WHERE ts > NOW() - INTERVAL '7 days'
  AND fleet = 'fleet_001'
GROUP BY ST_SnapToGrid(geom, 0.005)
HAVING COUNT(*) > 10
ORDER BY point_count DESC;
```

---

### 8. Tiempo de permanencia en una zona

```sql
-- Tiempo que un dispositivo estuvo en un radio de 100m
WITH target_zone AS (
    SELECT ST_Buffer(
        ST_SetSRID(ST_MakePoint(-99.1332, 19.4326), 4326)::geography,
        100  -- 100 metros
    )::geometry as zone
)
SELECT 
    device_id,
    user_id,
    MIN(ts) as entry_time,
    MAX(ts) as exit_time,
    MAX(ts) - MIN(ts) as duration,
    COUNT(*) as num_reports
FROM coordinates_history ch, target_zone tz
WHERE ts > NOW() - INTERVAL '24 hours'
  AND device_id = 'device_456'
  AND ST_Within(ch.geom, tz.zone)
GROUP BY device_id, user_id;
```

---

### 9. Última posición conocida (optimizada)

```sql
-- Usar la vista materializada (más rápido)
SELECT 
    device_id,
    user_id,
    fleet,
    last_seen,
    latitude,
    longitude,
    ST_AsText(geom) as location
FROM latest_device_position
WHERE device_id = 'device_456';
```

```sql
-- O desde la tabla principal (más actualizado)
SELECT DISTINCT ON (device_id)
    device_id,
    user_id,
    ts,
    latitude,
    longitude,
    ST_AsText(geom) as location
FROM coordinates_history
WHERE device_id = 'device_456'
ORDER BY device_id, ts DESC;
```

---

### 10. Dispositivos que se detuvieron (velocidad < 5 km/h)

```sql
WITH movement_data AS (
    SELECT 
        device_id,
        ts,
        geom,
        LAG(geom) OVER (PARTITION BY device_id ORDER BY ts) as prev_geom,
        LAG(ts) OVER (PARTITION BY device_id ORDER BY ts) as prev_ts
    FROM coordinates_history
    WHERE ts > NOW() - INTERVAL '2 hours'
)
SELECT 
    device_id,
    ts,
    latitude,
    longitude,
    ROUND(
        (ST_Distance(geom::geography, prev_geom::geography) / 
         NULLIF(EXTRACT(EPOCH FROM (ts - prev_ts)), 0) * 3.6)::numeric,
        2
    ) as speed_kmh
FROM (
    SELECT 
        md.*,
        ST_Y(md.geom::geometry) as latitude,
        ST_X(md.geom::geometry) as longitude
    FROM movement_data md
    WHERE prev_geom IS NOT NULL
) speeds
WHERE speed_kmh < 5
ORDER BY ts DESC;
```

---

## 🚀 Optimizaciones y Tips

### 1. Usar `geography` vs `geometry`
```sql
-- geography: distancias en metros (más lento pero preciso)
ST_Distance(geom::geography, point::geography)

-- geometry: distancias en grados (más rápido pero aproximado)
ST_Distance(geom, point)
```

### 2. Aprovechar índices BRIN para rangos temporales
```sql
-- EFICIENTE (usa índice BRIN)
WHERE ts > NOW() - INTERVAL '1 day'

-- INEFICIENTE
WHERE DATE(ts) = CURRENT_DATE
```

### 3. Usar la vista materializada para "última posición"
```sql
-- Rápido: Vista materializada (actualizada cada 30s)
SELECT * FROM latest_device_position WHERE device_id = 'X';

-- Más lento: Consulta directa
SELECT DISTINCT ON (device_id) * FROM coordinates_history 
WHERE device_id = 'X' ORDER BY device_id, ts DESC;
```

### 4. Limitar resultados con tiempo reciente
```sql
-- Siempre agregar filtro temporal para aprovechar chunks
WHERE ts > NOW() - INTERVAL '7 days'
```

---

## 📊 Consultas de Monitoreo

### Espacio usado por hypertable
```sql
SELECT 
    hypertable_name,
    pg_size_pretty(hypertable_size(format('%I.%I', hypertable_schema, hypertable_name)::regclass)) as size
FROM timescaledb_information.hypertables;
```

### Chunks activos
```sql
SELECT 
    chunk_name,
    range_start,
    range_end,
    pg_size_pretty(chunk_size) as chunk_size
FROM timescaledb_information.chunks
WHERE hypertable_name = 'coordinates_history'
ORDER BY range_start DESC
LIMIT 10;
```

### Estadísticas de compresión
```sql
SELECT 
    pg_size_pretty(before_compression_total_bytes) as uncompressed,
    pg_size_pretty(after_compression_total_bytes) as compressed,
    ROUND(
        (1 - after_compression_total_bytes::numeric / 
         NULLIF(before_compression_total_bytes, 0)) * 100,
        2
    ) as compression_ratio_pct
FROM timescaledb_information.compression_settings
WHERE hypertable_name = 'coordinates_history';
```

### Verificar continuous aggregates
```sql
-- ⚠️  Los continuous aggregates de TimescaleDB NO aparecen en pg_matviews.
-- Usar siempre timescaledb_information.continuous_aggregates:

-- Verificar si latest_device_position existe
SELECT view_name, view_definition 
FROM timescaledb_information.continuous_aggregates 
WHERE view_name = 'latest_device_position';

-- ❌ INCORRECTO (siempre retorna false para continuous aggregates)
-- SELECT EXISTS (SELECT 1 FROM pg_matviews WHERE matviewname = 'latest_device_position');
```

### Validar y recrear `latest_device_position`
```sql
-- Bloque completo para validar existencia y recrear si no existe
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM timescaledb_information.continuous_aggregates 
        WHERE view_name = 'latest_device_position'
    ) THEN
        RAISE NOTICE 'latest_device_position no existe. Creándola...';

        CREATE MATERIALIZED VIEW latest_device_position
        WITH (timescaledb.continuous) AS
        SELECT
            time_bucket('30 seconds', ts) AS bucket,
            device_id,
            fleet,
            last(ts, ts)        AS last_seen,
            last(latitude, ts)  AS latitude,
            last(longitude, ts) AS longitude,
            last(geom, ts)      AS geom,
            last(user_id, ts)   AS user_id
        FROM coordinates_history
        GROUP BY bucket, device_id, fleet
        WITH NO DATA;

        PERFORM add_continuous_aggregate_policy(
            'latest_device_position',
            start_offset      => INTERVAL '1 hour',
            end_offset        => INTERVAL '30 seconds',
            schedule_interval => INTERVAL '30 seconds',
            if_not_exists     => TRUE
        );

        RAISE NOTICE 'latest_device_position creada exitosamente.';
    ELSE
        RAISE NOTICE 'latest_device_position ya existe.';
    END IF;
END
$$;

-- Para forzar recreación (si tiene definición vieja):
-- DROP MATERIALIZED VIEW IF EXISTS latest_device_position CASCADE;
-- Luego ejecutar el bloque DO de arriba.
```

---

## 🔗 Referencias

- [PostGIS Documentation](https://postgis.net/docs/)
- [TimescaleDB Best Practices](https://docs.timescale.com/timescaledb/latest/how-to-guides/)
- [PostGIS Functions](https://postgis.net/docs/reference.html)

---

**Nota**: Reemplaza `user_123`, `device_456`, `fleet_001` y coordenadas de ejemplo con tus datos reales.
