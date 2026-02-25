# Quick Start Guide - TimescaleDB GPS Worker

## 🚀 Get Started in 5 Minutes

### 1. Clone and Navigate
```bash
git clone https://github.com/120m4n/mongo_nats_worker.git
cd mongo_nats_worker
git checkout copilot/use-timescaledb-instead-of-mongodb
```

### 2. Start the Services
```bash
docker-compose -f docker-compose-timescale.yml up -d
```

This starts:
- ✅ TimescaleDB (PostgreSQL + TimescaleDB + PostGIS)
- ✅ NATS message broker
- ✅ PgAdmin (database UI)
- ✅ Grafana (dashboards)
- ✅ GPS Worker

### 3. Check Status
```bash
docker-compose -f docker-compose-timescale.yml ps
docker-compose -f docker-compose-timescale.yml logs -f gps_worker
```

### 4. Send Test Message
```bash
cd test
go run test_sender.go
```

### 5. Verify Data
```bash
# Using PgAdmin: http://localhost:5050
# Login: admin@gps.local / admin
# Connect to timescaledb:5432

# Or use psql:
docker-compose -f docker-compose-timescale.yml exec timescaledb \
  psql -U gps_admin -d gps_tracking \
  -c "SELECT time, device_id, latitude, longitude FROM gps_positions ORDER BY time DESC LIMIT 10;"
```

## 📊 Access Services

| Service | URL | Credentials |
|---------|-----|-------------|
| PgAdmin | http://localhost:5050 | admin@gps.local / admin |
| Grafana | http://localhost:3000 | admin / admin |
| NATS Monitor | http://localhost:8222 | - |

## 🔧 Configuration

### Quick Config (.env file)
```bash
# Copy example
cp .env.timescale.example .env

# Edit as needed
DB_USER=gps_admin
DB_PASSWORD=your_secure_password
DB_NAME=gps_tracking
DISTANCE_THRESHOLD=5.0
```

### Environment Variables
- `NATS_URL`: NATS server URL (default: nats://localhost:4222)
- `POSTGRES_HOST`: Database host (default: localhost)
- `POSTGRES_PORT`: Database port (default: 5432)
- `POSTGRES_USER`: Database user (default: gps_admin)
- `POSTGRES_PASSWORD`: Database password
- `POSTGRES_DB`: Database name (default: gps_tracking)
- `DISTANCE_THRESHOLD`: Min distance in meters to store (default: 5.0)

## 📝 Message Format

Send JSON messages to NATS topic `coordinates`:

```json
{
  "unique_id": "device-123",
  "user_id": "user-456",
  "fleet": "fleet-789",
  "location": {
    "type": "Point",
    "coordinates": [-99.1332, 19.4326]
  },
  "ip_origin": "192.168.1.100",
  "last_modified": 1708000000
}
```

⚠️ **Important**: Coordinates are `[longitude, latitude]` (GeoJSON standard)

## 🏗️ Local Development

### Build Worker
```bash
# Build
go build -o mongo_nats_worker_timescale main_timescale.go

# Or use script
./build_timescale.sh

# Run
./mongo_nats_worker_timescale
```

### Run Tests
```bash
# Unit tests only
go test -short -v ./internal/storage/

# Integration tests (requires TimescaleDB running)
go test -v ./internal/storage/
```

## 📈 Common Queries

### Last position per device
```sql
SELECT DISTINCT ON (device_id)
  device_id, time, latitude, longitude, speed
FROM gps_positions
ORDER BY device_id, time DESC;
```

### Positions in last hour
```sql
SELECT device_id, time, latitude, longitude
FROM gps_positions
WHERE time > NOW() - INTERVAL '1 hour'
ORDER BY time DESC;
```

### Device route for today
```sql
SELECT time, latitude, longitude, speed
FROM gps_positions
WHERE device_id = 'device-123'
  AND time > CURRENT_DATE
ORDER BY time;
```

### Positions within area (geofence)
```sql
SELECT device_id, time, latitude, longitude
FROM gps_positions
WHERE ST_DWithin(
  geom,
  ST_MakePoint(-99.1332, 19.4326)::geography,
  1000  -- 1km radius
)
AND time > NOW() - INTERVAL '1 day';
```

## 🛠️ Troubleshooting

### Worker not connecting to TimescaleDB
```bash
# Check TimescaleDB health
docker-compose -f docker-compose-timescale.yml ps timescaledb

# Check logs
docker-compose -f docker-compose-timescale.yml logs timescaledb
```

### No messages being processed
```bash
# Check NATS connection
curl http://localhost:8222/varz

# Check worker logs
docker-compose -f docker-compose-timescale.yml logs gps_worker
```

### Database queries slow
```sql
-- Check table size
SELECT pg_size_pretty(hypertable_size('gps_positions'));

-- Check indexes
\d gps_positions

-- Analyze table
ANALYZE gps_positions;
```

## 🧹 Cleanup

```bash
# Stop services
docker-compose -f docker-compose-timescale.yml down

# Remove volumes (deletes data!)
docker-compose -f docker-compose-timescale.yml down -v
```

## 📚 More Documentation

- [README_TIMESCALE.md](README_TIMESCALE.md) - Complete setup guide
- [TIMESCALE_IMPLEMENTATION.md](TIMESCALE_IMPLEMENTATION.md) - Implementation details
- [ARCHITECTURE.md](ARCHITECTURE.md) - System architecture

## 💡 Tips

1. **Performance**: Adjust `postgresql.conf` based on your hardware
2. **Retention**: Default is 90 days, change in `init-scripts/01_init.sql`
3. **Workers**: Default is 5, adjust `numWorkers` in `main_timescale.go`
4. **Threshold**: Tune `DISTANCE_THRESHOLD` to filter redundant positions
5. **Monitoring**: Use Grafana dashboards for real-time metrics

## 🆘 Need Help?

- Check logs: `docker-compose logs -f [service_name]`
- Review documentation files
- Verify environment variables
- Test database connection with psql
- Check NATS monitoring UI

## ✅ Verification Checklist

- [ ] All containers running (`docker-compose ps`)
- [ ] TimescaleDB healthy (health check passing)
- [ ] Worker logs show "Successfully connected to TimescaleDB"
- [ ] Test message sent successfully
- [ ] Data visible in PgAdmin or via psql
- [ ] No errors in worker logs

---

**Ready to scale?** This setup handles thousands of GPS positions per second with proper tuning! 🚀
