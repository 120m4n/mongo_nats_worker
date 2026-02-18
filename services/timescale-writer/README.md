# timescale-writer — Consumidor NATS → TimescaleDB

Servicio de consumo del topic NATS `coordinates` con almacenamiento persistente e indexación PostGIS en TimescaleDB.

## Especificación

Implementación 100% conforme al documento `DESIGN_NATS_TIMESCALE_CONSUMER.md`:

- **Topic NATS**: Core NATS (sin JetStream), subject `coordinates`
- **Formato mensaje**: JSON `Document` (unique_id, user_id, fleet, location.coordinates[lng, lat], ip_origin, last_modified)
- **Tabla**: `coordinates_history` (hypertable TimescaleDB con partición diaria)
- **Escritura**: Protocol COPY/CopyFrom en batches de 500 filas
- **Índices**: BRIN(ts), B-tree(device_id,ts), GIST(geom), B-tree(fleet,ts)
- **Compresión**: Automática post 3 días, retención 90 días
- **Geospatial**: PostGIS columna `geom(POINT, 4326)` para ST_DWithin, ST_Contains

## Arquitectura

```
NATS (Core)
  ↓ subscribe("coordinates")
Handler (consumer/nats.go)
  ↓ deserialize + validate
Batch Channel (10k buffer)
  ↓ batch accumulator (500 rows o 2s timeout)
CopyFrom
  ↓
TimescaleDB + PostGIS
  ↓
Health endpoint (:3010)
```

## Estructura

```
services/timescale-writer/
├── main.go              # Entrypoint: conexiones, lifecycle
├── go.mod               # Dependencias
├── Dockerfile           # Multi-stage build
├── config/
│   └── config.go        # Carga de env vars
├── consumer/
│   ├── models.go        # Document, CoordinateRow, validación
│   ├── errors.go        # Custom errors
│   ├── nats.go          # Subscribe + MessageHandler + metrics
├── writer/
│   └── batch.go         # Batch buffering + CopyFrom
├── health/
│   └── health.go        # /health, /metrics endpoints
└── migrations/
    ├── 001_extensions.sql
    ├── 002_create_table.sql
    ├── 003_create_indexes.sql
    ├── 004_compression.sql
    └── 005_continuous_aggregate.sql
```

## Instalación local

### 1. Build

```bash
cd services/timescale-writer
go mod download
go build -o timescale-writer main.go
```

### 2. Variables de entorno

```bash
export NATS_ADDRESS=nats://localhost:4222
export NATS_SUBJECT=coordinates
export TIMESCALE_DSN="postgres://geo_user:geo_password@localhost:5432/geosmart?sslmode=disable"
export BATCH_SIZE=500
export FLUSH_INTERVAL_MS=2000
export HEALTH_PORT=3010
```

### 3. Ejecutar

```bash
./timescale-writer
```

## Docker

### Build

```bash
docker build -t timescale-writer:latest services/timescale-writer/
```

### Run

```bash
docker run \
  --network gps_network \
  -e NATS_ADDRESS=nats://nats:4222 \
  -e TIMESCALE_DSN="postgres://geo_user:geo_password@timescaledb:5432/geosmart?sslmode=disable" \
  -p 3010:3010 \
  timescale-writer:latest
```

## Endpoints

### Health

```bash
curl http://localhost:3010/health
```

Respuesta:
```json
{
  "status": "ok",
  "timestamp": "2026-02-17T13:05:00Z",
  "uptime": "5m30s",
  "nats": {"connected": true},
  "timescaledb": {"connected": true},
  "metrics": {
    "messages_received": 1250,
    "batches_written": 2,
    "rows_written": 625,
    "validation_errors": 0,
    "writer_errors": 0
  }
}
```

### Métricas (Prometheus)

```bash
curl http://localhost:3010/metrics
```

## Pruebas

### Enviar mensaje de prueba via NATS

```bash
# Usando nats-box
docker run --rm --network gps_network natsio/nats-box:latest nats pub coordinates \
  '{
    "unique_id": "device-001",
    "user_id": "user-1",
    "fleet": "operaciones",
    "location": {
      "type": "Point",
      "coordinates": [-69.9388, 18.4861]
    },
    "ip_origin": "192.168.1.1",
    "last_modified": 1739808000
  }' \
  -s nats://nats:4222
```

### Verificar en TimescaleDB

```bash
docker exec timescaledb psql -U geo_user -d geosmart -c \
  "SELECT ts, device_id, fleet, latitude, longitude FROM coordinates_history ORDER BY ts DESC LIMIT 1;"
```

## Queries Típicos

### Última posición de un dispositivo

```sql
SELECT ts, latitude, longitude, fleet
FROM coordinates_history
WHERE device_id = 'device-001'
ORDER BY ts DESC LIMIT 1;
```

### Dispositivos cercanos (radio 500m)

```sql
SELECT DISTINCT ON (device_id)
    device_id, fleet, ts, latitude, longitude,
    ST_Distance(geom::geography, ST_MakePoint(-69.9388, 18.4861)::geography) AS distance_m
FROM coordinates_history
WHERE ts > NOW() - INTERVAL '5 minutes'
  AND ST_DWithin(geom::geography, ST_MakePoint(-69.9388, 18.4861)::geography, 500)
ORDER BY device_id, ts DESC;
```

### Trayectoria de dispositivo

```sql
SELECT ts, latitude, longitude
FROM coordinates_history
WHERE device_id = 'device-001'
  AND ts BETWEEN '2026-02-17 08:00'::timestamptz AND '2026-02-17 18:00'::timestamptz
ORDER BY ts ASC;
```

## Métricas y Monitoreo

| Métrica                      | Descripción                          |
|------------------------------|--------------------------------------|
| `messages_received_total`    | Mensajes recibidos desde NATS       |
| `messages_dropped_total`     | Mensajes descartados (batch lleno)  |
| `validation_errors_total`    | Errores de validación               |
| `batches_written_total`      | Lotes escritos a DB                 |
| `rows_written_total`         | Filas inseridas                     |
| `writer_errors_total`        | Errores de escritura                |

## Consideraciones de Producción

1. **Resiliencia**: Implementar JetStream o buffer persistente en fase 2
2. **Escalabilidad**: Considerar múltiples workers con consumer groups
3. **Backups**: Implementar política de snapshots diarios de TimescaleDB
4. **Alertas**: Monitorear `messages_dropped` y `writer_errors` con Prometheus/AlertManager

## Changelog

- `v1.0.0` (2026-02-17): Implementación inicial per especificación
