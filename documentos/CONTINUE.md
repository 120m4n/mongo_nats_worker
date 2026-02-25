# Estado del proyecto — 17 Feb 2026

## Servicios funcionando (docker-compose-timescale.yml)

| Servicio | Estado | Puerto |
|---|---|---|
| TimescaleDB | healthy | 5433 |
| NATS | healthy | 4222, 8222 |
| GPS Worker | running | — |
| Grafana | running | 3000 |
| pgAdmin | running | 5050 |

## Problemas corregidos hoy

1. **postgresql.conf** — Agregado `shared_preload_libraries = 'timescaledb'` y `listen_addresses = '*'`
2. **NATS command** — Removido flag inválido `--max_file_store`, cambiado a formato array en compose
3. **NATS healthcheck** — Agregado healthcheck con `nats-server --help` (imagen scratch no tiene wget/curl)
4. **PostGIS eliminado** — La imagen `timescale/timescaledb:latest-pg15` no incluye PostGIS. Se removió la columna `geom`, extensión `postgis`, índice GIST y funciones `ST_*` de:
   - `init-scripts/01_init.sql`
   - `internal/storage/timescale.go` (InsertPosition y InsertPositionBatch)
5. **docker-compose** — Removido `version: '3.8'` obsoleto, `depends_on` con `condition: service_healthy`

## Pendiente para mañana

### 1. Rebuild del worker (PRIORITARIO)
El fix del campo `metadata` (JSONB) ya está en el código pero **NO se ha rebuildeado** el container.

```bash
docker compose -f docker-compose-timescale.yml up -d --build gps_worker
```

**Problema:** El worker envía `[]byte` vacío como metadata, postgres rechaza bytes no-JSON.  
**Fix aplicado:** Función `metadataToJSON()` en `internal/storage/timescale.go` que convierte a `nil` si está vacío.

### 2. Verificar inserción de datos
Después del rebuild, enviar datos de prueba:

```bash
docker run --rm --network mongo_nats_worker_gps_network natsio/nats-box:latest \
  nats pub coordinates \
  '{"unique_id":"truck-001","user_id":"driver-1","fleet":"fleet-A","location":{"type":"Point","coordinates":[-99.1332,19.4326]},"ip_origin":"10.0.0.1","last_modified":1739764800}' \
  -s nats://gps_nats:4222
```

Verificar en la BD:

```bash
docker exec gps_timescaledb psql -U gps_admin -d gps_tracking \
  -c "SELECT time, device_id, latitude, longitude FROM gps_positions ORDER BY time;"
```

### 3. Verificar en Grafana
- Acceder a http://localhost:3000 (admin/admin)
- Verificar datasource TimescaleDB configurado
- Crear/verificar dashboard de posiciones GPS

### 4. Considerar re-agregar PostGIS (opcional)
Si se necesitan queries geoespaciales, usar la imagen `timescale/timescaledb-ha:pg15` que sí incluye PostGIS. Requiere ajustar el volume mount (`/home/postgres/pgdata/data` en vez de `/var/lib/postgresql/data`).

## Archivos modificados

- `postgresql.conf` — shared_preload_libraries, listen_addresses
- `docker-compose-timescale.yml` — image, command, healthchecks, depends_on
- `init-scripts/01_init.sql` — removido postgis, geom column
- `internal/storage/timescale.go` — removido geom/ST_*, agregado metadataToJSON()

## Cómo levantar todo desde cero

```bash
docker compose -f docker-compose-timescale.yml down -v
docker compose -f docker-compose-timescale.yml up -d --build
```

> El `-v` borra volúmenes. Necesario si se cambia el init SQL ya que postgres solo ejecuta init scripts con volumen vacío.
