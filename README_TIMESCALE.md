# TimescaleDB Implementation for GPS Worker

This implementation replaces MongoDB with TimescaleDB for storing GPS position data from NATS messages.

## Overview

The TimescaleDB version maintains the same NATS message format as the MongoDB version but stores data in a PostgreSQL database with TimescaleDB extension for time-series optimization.

## Architecture

```
NATS (coordinates topic) 
  → GPS Worker (Go)
    → TimescaleDB (PostgreSQL + TimescaleDB + PostGIS)
```

### Components

- **TimescaleDB**: Time-series database for GPS positions
- **NATS**: Message broker for receiving GPS data
- **PgAdmin**: Optional web UI for database management
- **Grafana**: Optional dashboards for metrics visualization
- **GPS Worker**: Go application that consumes NATS messages and stores in TimescaleDB

## Quick Start

### Prerequisites

- Docker and Docker Compose
- Go 1.21.6 or higher (for local development)

### Using Docker Compose (Recommended)

1. Create a `.env` file (optional, defaults are provided):

```bash
# Database
DB_USER=gps_admin
DB_PASSWORD=changeme_strong_password
DB_NAME=gps_tracking
DB_PORT=5433

# PgAdmin
PGADMIN_EMAIL=admin@gps.local
PGADMIN_PASSWORD=admin
PGADMIN_PORT=5050

# Grafana
GRAFANA_USER=admin
GRAFANA_PASSWORD=admin
GRAFANA_PORT=3000

# Worker
DISTANCE_THRESHOLD=5.0
```

2. Start all services:

```bash
docker-compose -f docker-compose-timescale.yml up -d
```

3. Check logs:

```bash
docker-compose -f docker-compose-timescale.yml logs -f gps_worker
```

### Local Development

1. Install dependencies:

```bash
go mod download
```

2. Set environment variables:

```bash
export NATS_URL=nats://localhost:4222
export POSTGRES_HOST=localhost
export POSTGRES_PORT=5433
export POSTGRES_USER=gps_admin
export POSTGRES_PASSWORD=changeme_strong_password
export POSTGRES_DB=gps_tracking
export DISTANCE_THRESHOLD=5.0
```

3. Run the worker:

```bash
go run main_timescale.go
```

Or build and run:

```bash
./build_timescale.sh
./mongo_nats_worker_timescale
```

## Message Format

The worker expects JSON messages on the `coordinates` NATS topic with the following structure:

```json
{
  "unique_id": "device-123",
  "user_id": "user-456",
  "fleet": "fleet-789",
  "location": {
    "type": "Point",
    "coordinates": [40.7128, -74.0060]
  },
  "ip_origin": "192.168.1.100",
  "last_modified": 1708000000
}
```

### Required Fields

- `unique_id`: Device identifier
- `user_id`: User identifier  
- `fleet`: Fleet identifier
- `location.type`: Always "Point"
- `location.coordinates`: Array with [latitude, longitude]
- `last_modified`: Unix timestamp

## Database Schema

### Main Table: gps_positions

```sql
CREATE TABLE gps_positions (
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
```

The table is converted to a TimescaleDB hypertable with 1-day chunks for optimal time-series performance.

## Features

### Proximity Filtering

Like the MongoDB version, the worker implements proximity filtering to reduce redundant data:

- Caches the last known position for each device
- Calculates Haversine distance between new and cached positions
- Only stores positions that exceed the distance threshold (default: 5 meters)

### Performance Optimizations

- Connection pooling (25 max connections, 5 idle)
- Worker pool (5 concurrent workers by default)
- Buffered channel for message processing (100 messages)
- Automatic data retention (90 days default)
- Optimized PostgreSQL configuration for time-series workloads

### Monitoring

The worker logs statistics every 120 seconds:
- Processed messages
- Database errors
- Validation errors
- Cache hits/misses

## Accessing Services

After starting with docker-compose:

- **PgAdmin**: http://localhost:5050
  - Login with credentials from .env
  - Connect to TimescaleDB: `timescaledb:5432`

- **Grafana**: http://localhost:3000
  - Login with credentials from .env
  - Add TimescaleDB as datasource

- **NATS Monitoring**: http://localhost:8222

## Configuration

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| NATS_URL | nats://localhost:4222 | NATS server URL |
| POSTGRES_HOST | localhost | PostgreSQL host |
| POSTGRES_PORT | 5432 | PostgreSQL port |
| POSTGRES_USER | gps_admin | Database user |
| POSTGRES_PASSWORD | changeme_strong_password | Database password |
| POSTGRES_DB | gps_tracking | Database name |
| DISTANCE_THRESHOLD | 5.0 | Minimum distance in meters to store new position |

### PostgreSQL Tuning

The included `postgresql.conf` is optimized for GPS workloads:
- High connection limit (200)
- Optimized memory settings
- WAL configuration for high write throughput
- TimescaleDB-specific optimizations

## Testing

Send test messages using NATS CLI or the test sender:

```bash
nats pub coordinates '{
  "unique_id": "test-device-1",
  "user_id": "user-1",
  "fleet": "fleet-1",
  "location": {
    "type": "Point",
    "coordinates": [40.7128, -74.0060]
  },
  "ip_origin": "192.168.1.100",
  "last_modified": 1708000000
}'
```

## Migration from MongoDB

The TimescaleDB implementation is designed to be compatible with the existing MongoDB message format. Key differences:

1. **Schema**: Structured schema vs. schemaless MongoDB
2. **Geospatial**: Uses PostGIS geography type instead of MongoDB's geospatial indexes
3. **Time-series**: Automatic partitioning by time with TimescaleDB hypertables
4. **Retention**: Built-in retention policies vs. MongoDB TTL indexes

## Troubleshooting

### Worker can't connect to TimescaleDB

Check that TimescaleDB is healthy:
```bash
docker-compose -f docker-compose-timescale.yml ps
```

### No data appearing in database

1. Check worker logs for errors
2. Verify NATS connection
3. Test message format validation

### Performance issues

1. Review PostgreSQL configuration
2. Check connection pool settings
3. Monitor worker statistics
4. Review TimescaleDB chunk size

## License

Same as parent project.
