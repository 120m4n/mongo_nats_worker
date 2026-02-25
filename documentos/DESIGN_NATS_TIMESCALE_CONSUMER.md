# Documento Agéntico de Diseño: Consumidor NATS → TimescaleDB

> **Objetivo**: Especificación completa para desarrollar un servicio consumidor del topic NATS `coordinates` con almacenamiento persistente en TimescaleDB (con PostGIS), optimizado para series temporales de alta frecuencia y consultas geoespaciales.

---

## 1. Análisis del Emisor: `coordinates-service`

### 1.1 Flujo de publicación actual

```
Cliente HTTP
│
▼ POST /coordinates (CoordinatesRequest JSON)
│
├──► Tile38: SET <fleet> <unique_id> POINT <lat> <lng> EX 90
│    (posición en tiempo real, TTL 90s, volátil)
│
└──► NATS: Publish("coordinates", Document JSON)
     ⚠ ACTUALMENTE SIN SUSCRIPTOR — los mensajes se pierden
```

### 1.2 Código fuente de publicación

Archivo: `services/coordinates-service/main.go`, líneas 78-93:

```go
doc := model.Document{
    UniqueId: request.UniqueID,
    UserId:   request.UserID,
    Fleet:    fleet,
    Location: model.MongoLocation{
        Type:        "Point",
        Coordinates: []float64{request.Coordinates.Longitude, request.Coordinates.Latitude},
    },
    OriginIp:     c.ClientIP(),
    LastModified: time.Now().Unix(),
}

docJson, _ := json.Marshal(doc)
nc.Publish("coordinates", docJson)
```

### 1.3 Modelos de datos involucrados

**`CoordinatesRequest`** (entrada HTTP):
```go
type CoordinatesRequest struct {
    Coordinates struct {
        Latitude  float64 `json:"latitude" binding:"required"`
        Longitude float64 `json:"longitude" binding:"required"`
    } `json:"coordinates" binding:"required"`
    UserID    string `json:"user_id" binding:"required"`
    UniqueID  string `json:"unique_id" binding:"required"`
    Fleet     string `json:"fleet" binding:"required"`
    FleetType string `json:"fleet_type"`
    AvatarIco string `json:"avatar_ico"`
}
```

**`Document`** (mensaje publicado a NATS):
```go
type Document struct {
    UniqueId     string        `json:"unique_id"`
    UserId       string        `json:"user_id"`
    Fleet        string        `json:"fleet"`
    Location     MongoLocation `json:"location"`
    OriginIp     string        `json:"ip_origin"`
    LastModified int64         `json:"last_modified"`
}

type MongoLocation struct {
    Type        string    `json:"type"`
    Coordinates []float64 `json:"coordinates"`
}
```

---

## 2. Definición del Topic NATS

### 2.1 Topic de publicación

| Propiedad         | Valor                        |
|-------------------|------------------------------|
| **Subject**       | `coordinates`                |
| **Protocolo**     | Core NATS (no JetStream)     |
| **QoS**           | At-most-once (fire-and-forget) |
| **Serialización** | JSON (UTF-8)                 |
| **Publisher**      | `coordinates-service`        |
| **Subscriber**     | **NINGUNO** (a implementar)  |

### 2.2 Recomendación: Migrar a NATS JetStream

> **CRÍTICO**: El sistema actual usa Core NATS. Si el consumidor TimescaleDB se desconecta o reinicia, **todos los mensajes durante la desconexión se pierden irrecuperablemente**.

Para producción se recomienda:

```
Subject JetStream:  geo.coordinates
Stream name:        COORDINATES
Retention:          WorkQueue (se eliminan tras ACK)
Max Age:            24h (buffer ante caídas prolongadas)
Storage:            File
Replicas:           1 (dev) / 3 (prod)
Consumer:           timescale-writer (durable, deliver-all)
Ack Policy:         Explicit
Max Ack Pending:    1000
```

**Cambio requerido en `coordinates-service`** (publicador):
```go
// Antes (Core NATS):
nc.Publish("coordinates", docJson)

// Después (JetStream):
js, _ := nc.JetStream()
js.Publish("geo.coordinates", docJson)
```

**Si se mantiene Core NATS** (fase inicial): el subject sigue siendo `coordinates` y el consumidor debe suscribirse con `nc.Subscribe("coordinates", handler)`.

---

## 3. Diseño del Mensaje Típico NATS

### 3.1 Estructura JSON actual

```json
{
  "unique_id": "cuadrilla-norte-07",
  "user_id": "usr_4f8a2b",
  "fleet": "operaciones_campo",
  "location": {
    "type": "Point",
    "coordinates": [-69.9388, 18.4861]
  },
  "ip_origin": "192.168.1.45",
  "last_modified": 1739808000
}
```

### 3.2 Campos y semántica

| Campo JSON          | Tipo Go    | Tipo DB          | Descripción                                                    |
|---------------------|------------|------------------|----------------------------------------------------------------|
| `unique_id`         | `string`   | `TEXT NOT NULL`   | Identificador del dispositivo/cuadrilla (ej: `"cuadrilla-07"`) |
| `user_id`           | `string`   | `TEXT NOT NULL`   | Usuario operador asociado al dispositivo                       |
| `fleet`             | `string`   | `TEXT NOT NULL`   | Flota/grupo operativo (ej: `"operaciones_campo"`, `"avatar"`)  |
| `location.type`     | `string`   | —                 | Siempre `"Point"` (GeoJSON). No se almacena, implícito.        |
| `location.coordinates[0]` | `float64` | `DOUBLE PRECISION` | **Longitud** (GeoJSON: lng primero)                           |
| `location.coordinates[1]` | `float64` | `DOUBLE PRECISION` | **Latitud** (GeoJSON: lat segundo)                            |
| `ip_origin`         | `string`   | `INET`            | IP de origen del reporte                                       |
| `last_modified`     | `int64`    | `TIMESTAMPTZ`     | Unix epoch → convertir a TIMESTAMPTZ en el consumidor          |

### 3.3 Volumen estimado

| Escenario        | Dispositivos | Frecuencia     | Mensajes/día | Tamaño/día (≈120B/msg) |
|------------------|-------------|----------------|-------------|------------------------|
| Piloto           | 50          | cada 30s       | 144,000     | ~17 MB                 |
| Producción       | 500         | cada 15s       | 2,880,000   | ~330 MB                |
| Alta escala      | 5,000       | cada 10s       | 43,200,000  | ~4.9 GB                |

---

## 4. Diseño de Tabla TimescaleDB + PostGIS

### 4.1 Extensiones requeridas

```sql
CREATE EXTENSION IF NOT EXISTS timescaledb;
CREATE EXTENSION IF NOT EXISTS postgis;
```

### 4.2 Tabla principal: `coordinates_history`

```sql
CREATE TABLE coordinates_history (
    -- Timestamp con zona horaria (OBLIGATORIO para cuadrillas en diferentes zonas)
    ts              TIMESTAMPTZ     NOT NULL,

    -- Identificadores
    device_id       TEXT            NOT NULL,    -- unique_id del mensaje
    user_id         TEXT            NOT NULL,
    fleet           TEXT            NOT NULL,

    -- Coordenadas raw (para respuestas rápidas sin overhead PostGIS)
    longitude       DOUBLE PRECISION NOT NULL,
    latitude        DOUBLE PRECISION NOT NULL,

    -- Geometría PostGIS (para queries geoespaciales: ST_DWithin, ST_Contains, etc.)
    geom            GEOMETRY(Point, 4326) NOT NULL,

    -- Metadata
    ip_origin       INET
);
```

### 4.3 Conversión a Hypertable

```sql
-- Hypertable particionada por tiempo
-- chunk_time_interval: 1 día (óptimo para retención y compresión)
SELECT create_hypertable(
    'coordinates_history',
    by_range('ts', INTERVAL '1 day')
);
```

### 4.4 Índices optimizados

#### 4.4.1 Índice BRIN para series temporales (ultra compacto)

```sql
-- BRIN: ~1000x más pequeño que B-tree para datos temporales ordenados.
-- Ideal porque los INSERTs son naturalmente ordenados por tiempo.
-- pages_per_range = 32: buen balance precisión/tamaño para chunks diarios.
CREATE INDEX idx_coordinates_ts_brin
    ON coordinates_history
    USING BRIN (ts)
    WITH (pages_per_range = 32);
```

> **¿Por qué BRIN?** Los datos de coordenadas llegan naturalmente ordenados por tiempo. BRIN almacena solo min/max por bloque de páginas, resultando en un índice de ~100KB para millones de filas, vs ~500MB de un B-tree equivalente.

#### 4.4.2 Índice compuesto para patrón "última posición de dispositivo X"

```sql
-- Patrón de consulta: "¿Dónde está la cuadrilla-07 ahora?"
-- SELECT * FROM coordinates_history
-- WHERE device_id = 'cuadrilla-07'
-- ORDER BY ts DESC LIMIT 1;
CREATE INDEX idx_coordinates_device_time
    ON coordinates_history (device_id, ts DESC);
```

> **Justificación**: B-tree compuesto con `ts DESC` permite resolver `ORDER BY ts DESC LIMIT 1` como un simple index scan sin sort. Es el patrón más frecuente: "última posición conocida".

#### 4.4.3 Índice espacial GIST para queries geográficas PostGIS

```sql
-- Patrón: "¿Qué dispositivos están dentro de 500m de este punto?"
-- SELECT * FROM coordinates_history
-- WHERE ts > NOW() - INTERVAL '5 minutes'
--   AND ST_DWithin(geom, ST_MakePoint(-69.93, 18.48)::geography, 500);
CREATE INDEX idx_coordinates_geom_gist
    ON coordinates_history
    USING GIST (geom);
```

#### 4.4.4 Índice para filtrado por flota

```sql
-- Patrón: "Todas las posiciones de la flota 'operaciones_campo' en la última hora"
CREATE INDEX idx_coordinates_fleet_time
    ON coordinates_history (fleet, ts DESC);
```

### 4.5 Compresión TimescaleDB

```sql
-- Habilitar compresión automática
ALTER TABLE coordinates_history SET (
    timescaledb.compress,
    timescaledb.compress_segmentby = 'device_id, fleet',
    timescaledb.compress_orderby = 'ts DESC'
);

-- Comprimir chunks mayores a 3 días
SELECT add_compression_policy(
    'coordinates_history',
    compress_after => INTERVAL '3 days'
);
```

> **Ratio de compresión esperado**: 10:1 a 20:1 para datos de coordenadas. Un día de producción (330 MB) → ~20-30 MB comprimido.

### 4.6 Política de retención

```sql
-- Eliminar datos mayores a 90 días (ajustar según requisitos)
SELECT add_retention_policy(
    'coordinates_history',
    drop_after => INTERVAL '90 days'
);
```

### 4.7 Vista materializada continua: última posición por dispositivo

```sql
-- Continuous aggregate para "última posición" sin escanear toda la tabla
CREATE MATERIALIZED VIEW latest_device_position
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

-- Refrescar cada 30 segundos
SELECT add_continuous_aggregate_policy(
    'latest_device_position',
    start_offset    => INTERVAL '1 hour',
    end_offset      => INTERVAL '30 seconds',
    schedule_interval => INTERVAL '30 seconds'
);
```

---

## 5. Diseño del Servicio Consumidor

### 5.1 Arquitectura

```
                    ┌─────────────────────────────────┐
                    │       NATS Server                │
                    │   Subject: "coordinates"         │
                    └──────────┬──────────────────────┘
                               │ Subscribe
                               ▼
                    ┌─────────────────────────────────┐
                    │   timescale-writer-service       │
                    │                                  │
                    │  ┌─────────┐   ┌──────────────┐ │
                    │  │  NATS   │──▶│  Buffer/Batch │ │
                    │  │Listener │   │  (chan []Doc) │ │
                    │  └─────────┘   └──────┬───────┘ │
                    │                       │         │
                    │              ┌────────▼───────┐ │
                    │              │  Batch Writer  │ │
                    │              │  (COPY proto)  │ │
                    │              └────────┬───────┘ │
                    │                       │         │
                    └───────────────────────┼─────────┘
                                            │
                               ┌────────────▼─────────────┐
                               │     TimescaleDB          │
                               │  + PostGIS               │
                               │  coordinates_history     │
                               └──────────────────────────┘
```

### 5.2 Estrategia de inserción por lotes (Batch Insert)

Para maximizar el throughput de escritura:

```go
// Parámetros de batching
const (
    BatchSize     = 500           // Máximo de filas por batch INSERT
    FlushInterval = 2 * time.Second // Flush cada 2s aunque no se llene el batch
    ChannelBuffer = 10000        // Buffer del canal entre listener y writer
)
```

**Método de inserción recomendado**: `pgx.CopyFrom` (protocolo COPY de PostgreSQL)

```go
// Pseudocódigo del batch writer
func batchWriter(ctx context.Context, pool *pgxpool.Pool, batch []CoordinateRow) error {
    _, err := pool.CopyFrom(
        ctx,
        pgx.Identifier{"coordinates_history"},
        []string{"ts", "device_id", "user_id", "fleet", "longitude", "latitude", "geom", "ip_origin"},
        pgx.CopyFromSlice(len(batch), func(i int) ([]interface{}, error) {
            r := batch[i]
            // ST_MakePoint se maneja como WKB binario para máximo rendimiento
            geomWKB := encodePointWKB(r.Longitude, r.Latitude, 4326)
            return []interface{}{
                r.Timestamp,   // TIMESTAMPTZ
                r.DeviceID,    // TEXT
                r.UserID,      // TEXT
                r.Fleet,       // TEXT
                r.Longitude,   // DOUBLE PRECISION
                r.Latitude,    // DOUBLE PRECISION
                geomWKB,       // GEOMETRY (WKB binary)
                r.IPOrigin,    // INET (puede ser nil)
            }, nil
        }),
    )
    return err
}
```

### 5.3 Transformación del mensaje NATS → fila TimescaleDB

```go
type CoordinateRow struct {
    Timestamp  time.Time  // last_modified (unix epoch) → time.Unix(epoch, 0).UTC()
    DeviceID   string     // unique_id
    UserID     string     // user_id
    Fleet      string     // fleet
    Longitude  float64    // location.coordinates[0]
    Latitude   float64    // location.coordinates[1]
    IPOrigin   *net.IP    // ip_origin (nullable)
}

func documentToRow(doc model.Document) CoordinateRow {
    var ip *net.IP
    if parsed := net.ParseIP(doc.OriginIp); parsed != nil {
        ip = &parsed
    }
    return CoordinateRow{
        Timestamp: time.Unix(doc.LastModified, 0).UTC(),
        DeviceID:  doc.UniqueId,
        UserID:    doc.UserId,
        Fleet:     doc.Fleet,
        Longitude: doc.Location.Coordinates[0], // GeoJSON: [lng, lat]
        Latitude:  doc.Location.Coordinates[1],
        IPOrigin:  ip,
    }
}
```

### 5.4 Variables de entorno

```env
# Conexión NATS
NATS_ADDRESS=nats://localhost:4222
NATS_SUBJECT=coordinates

# Conexión TimescaleDB
TIMESCALE_DSN=postgres://geo_user:password@localhost:5432/geosmart?sslmode=disable

# Batching
BATCH_SIZE=500
FLUSH_INTERVAL_MS=2000

# Health
HEALTH_PORT=3010
```

### 5.5 Estructura del servicio propuesto

```
services/timescale-writer/
├── Dockerfile
├── go.mod
├── main.go              # Entrypoint: conexiones, lifecycle
├── README.md
├── config/
│   └── config.go        # Carga de env vars
├── consumer/
│   └── nats.go          # Suscripción NATS + deserialización
├── writer/
│   └── batch.go         # Batch buffer + CopyFrom a TimescaleDB
├── migrations/
│   ├── 001_extensions.sql
│   ├── 002_create_table.sql
│   ├── 003_create_indexes.sql
│   ├── 004_compression.sql
│   └── 005_continuous_aggregate.sql
└── health/
    └── health.go        # /health endpoint
```

---

## 6. Queries Típicos Esperados

### 6.1 Última posición de un dispositivo

```sql
-- Usa idx_coordinates_device_time → index-only scan
SELECT ts, latitude, longitude, fleet
FROM coordinates_history
WHERE device_id = 'cuadrilla-07'
ORDER BY ts DESC
LIMIT 1;
```

### 6.2 Trayectoria de un dispositivo en un rango de tiempo

```sql
-- Usa BRIN(ts) + idx_coordinates_device_time
SELECT ts, latitude, longitude
FROM coordinates_history
WHERE device_id = 'cuadrilla-07'
  AND ts BETWEEN '2026-02-17 08:00:00-04'::timestamptz
              AND '2026-02-17 18:00:00-04'::timestamptz
ORDER BY ts ASC;
```

### 6.3 Dispositivos cercanos a un punto (radio 500m)

```sql
-- Usa GIST(geom) + BRIN(ts)
SELECT DISTINCT ON (device_id)
    device_id, fleet, ts, latitude, longitude,
    ST_Distance(
        geom::geography,
        ST_MakePoint(-69.9388, 18.4861)::geography
    ) AS distance_m
FROM coordinates_history
WHERE ts > NOW() - INTERVAL '5 minutes'
  AND ST_DWithin(
        geom::geography,
        ST_MakePoint(-69.9388, 18.4861)::geography,
        500  -- metros
      )
ORDER BY device_id, ts DESC;
```

### 6.4 Dispositivos dentro de un polígono (zona operativa)

```sql
-- Usa GIST(geom) para intersección con polígono
SELECT DISTINCT ON (device_id)
    device_id, fleet, ts, latitude, longitude
FROM coordinates_history
WHERE ts > NOW() - INTERVAL '10 minutes'
  AND ST_Within(
        geom,
        ST_GeomFromGeoJSON('{
            "type": "Polygon",
            "coordinates": [[
                [-70.0, 18.5], [-69.9, 18.5],
                [-69.9, 18.4], [-70.0, 18.4],
                [-70.0, 18.5]
            ]]
        }')
      )
ORDER BY device_id, ts DESC;
```

### 6.5 Estadísticas de actividad por flota

```sql
-- Cuenta reportes por flota en la última hora
SELECT fleet,
       COUNT(DISTINCT device_id) AS active_devices,
       COUNT(*) AS total_reports,
       MIN(ts) AS first_report,
       MAX(ts) AS last_report
FROM coordinates_history
WHERE ts > NOW() - INTERVAL '1 hour'
GROUP BY fleet
ORDER BY total_reports DESC;
```

---

## 7. Migraciones SQL Completas

### 7.1 `001_extensions.sql`

```sql
-- Extensiones requeridas
CREATE EXTENSION IF NOT EXISTS timescaledb;
CREATE EXTENSION IF NOT EXISTS postgis;
```

### 7.2 `002_create_table.sql`

```sql
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
    by_range('ts', INTERVAL '1 day')
);
```

### 7.3 `003_create_indexes.sql`

```sql
-- BRIN para rangos temporales (ultra compacto, ideal para millones de puntos)
CREATE INDEX idx_coordinates_ts_brin
    ON coordinates_history USING BRIN (ts)
    WITH (pages_per_range = 32);

-- B-tree compuesto para "última posición de dispositivo X"
CREATE INDEX idx_coordinates_device_time
    ON coordinates_history (device_id, ts DESC);

-- GIST espacial para queries PostGIS (ST_DWithin, ST_Contains, ST_Within)
CREATE INDEX idx_coordinates_geom_gist
    ON coordinates_history USING GIST (geom);

-- B-tree para filtrado por flota + tiempo
CREATE INDEX idx_coordinates_fleet_time
    ON coordinates_history (fleet, ts DESC);
```

### 7.4 `004_compression.sql`

```sql
-- Compresión nativa de TimescaleDB
ALTER TABLE coordinates_history SET (
    timescaledb.compress,
    timescaledb.compress_segmentby = 'device_id, fleet',
    timescaledb.compress_orderby = 'ts DESC'
);

-- Auto-comprimir chunks mayores a 3 días
SELECT add_compression_policy(
    'coordinates_history',
    compress_after => INTERVAL '3 days'
);

-- Retención: eliminar datos mayores a 90 días
SELECT add_retention_policy(
    'coordinates_history',
    drop_after => INTERVAL '90 days'
);
```

### 7.5 `005_continuous_aggregate.sql`

```sql
-- Vista materializada continua: última posición por dispositivo
CREATE MATERIALIZED VIEW latest_device_position
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

SELECT add_continuous_aggregate_policy(
    'latest_device_position',
    start_offset    => INTERVAL '1 hour',
    end_offset      => INTERVAL '30 seconds',
    schedule_interval => INTERVAL '30 seconds'
);
```

---

## 8. docker-compose parcial (TimescaleDB + PostGIS)

```yaml
services:
  timescaledb:
    image: timescale/timescaledb-ha:pg17  # Incluye PostGIS
    environment:
      POSTGRES_USER: geo_user
      POSTGRES_PASSWORD: geo_password
      POSTGRES_DB: geosmart
    ports:
      - "5432:5432"
    volumes:
      - timescale_data:/home/postgres/pgdata/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U geo_user -d geosmart"]
      interval: 5s
      timeout: 5s
      retries: 5

  timescale-writer:
    build: ./services/timescale-writer
    environment:
      NATS_ADDRESS: nats://nats:4222
      NATS_SUBJECT: coordinates
      TIMESCALE_DSN: postgres://geo_user:geo_password@timescaledb:5432/geosmart?sslmode=disable
      BATCH_SIZE: "500"
      FLUSH_INTERVAL_MS: "2000"
      HEALTH_PORT: "3010"
    depends_on:
      timescaledb:
        condition: service_healthy
      nats:
        condition: service_started
    ports:
      - "3010:3010"

volumes:
  timescale_data:
```

---

## 9. Consideraciones de Producción

### 9.1 Resiliencia ante pérdida de mensajes

| Estrategia                            | Impacto     | Complejidad |
|---------------------------------------|-------------|-------------|
| Migrar a NATS JetStream              | **Alta**    | Media       |
| Buffer en disco local (WAL propio)    | Media       | Alta        |
| Reinsertar desde Tile38 (backfill)    | Baja        | Baja        |

**Recomendación**: Migrar el publisher a JetStream en fase 2. Para fase 1, aceptar at-most-once y monitorear gaps.

### 9.2 Métricas a exponer (Prometheus)

```
timescale_writer_messages_received_total     # Counter
timescale_writer_messages_written_total      # Counter
timescale_writer_batch_size                  # Histogram
timescale_writer_batch_write_duration_seconds # Histogram
timescale_writer_errors_total                # Counter (por tipo)
timescale_writer_nats_connected              # Gauge (0/1)
timescale_writer_db_connected                # Gauge (0/1)
```

### 9.3 Manejo de errores

| Error                          | Acción                                                |
|--------------------------------|-------------------------------------------------------|
| Mensaje JSON inválido          | Log + descartar + incrementar error counter           |
| Coordenadas fuera de rango     | Log + descartar (lat ∉ [-90,90], lng ∉ [-180,180])   |
| TimescaleDB no disponible      | Reintentar con backoff exponencial (max 30s)          |
| Batch parcialmente fallido     | Reintentar el batch completo una vez, luego insertar uno a uno |
| NATS desconexión               | Reconexión automática (`nats.MaxReconnects(-1)`)       |

---

## 10. Resumen Ejecutivo de Implementación

| Componente                  | Decisión                                          |
|-----------------------------|---------------------------------------------------|
| **Topic NATS**              | `coordinates` (Core NATS, fase 1)                 |
| **Formato mensaje**         | JSON `model.Document` (≈120 bytes)                |
| **Tabla**                   | `coordinates_history` (TimescaleDB hypertable)    |
| **Chunk interval**          | 1 día                                             |
| **Índice temporal**         | BRIN (`pages_per_range=32`)                       |
| **Índice device+time**      | B-tree compuesto `(device_id, ts DESC)`           |
| **Índice geoespacial**      | GIST sobre `GEOMETRY(Point, 4326)`                |
| **Método escritura**        | `pgx.CopyFrom` en batches de 500                 |
| **Compresión**              | `segmentby=device_id,fleet` / `orderby=ts DESC`  |
| **Retención**               | 90 días (configurable)                            |
| **Columna tiempo**          | `TIMESTAMPTZ` (timezone-aware, obligatorio)       |
| **PostGIS**                 | Columna `geom` para `ST_DWithin`, `ST_Within`     |

---

> **Próximo paso**: Usar este documento como entrada para el agente que implementará `services/timescale-writer/` completo, incluyendo Go code, migraciones SQL, Dockerfile, tests y documentación.
