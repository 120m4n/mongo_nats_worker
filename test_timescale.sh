#!/bin/bash

# Script to test the TimescaleDB worker locally

echo "=== TimescaleDB Worker Test ==="
echo ""

# Check if docker-compose is available
if ! command -v docker-compose &> /dev/null; then
    echo "Error: docker-compose is not installed"
    exit 1
fi

echo "Starting TimescaleDB infrastructure..."
docker-compose -f docker-compose-timescale.yml up -d timescaledb nats

echo "Waiting for services to be ready..."
sleep 10

# Check if TimescaleDB is healthy
echo "Checking TimescaleDB health..."
docker-compose -f docker-compose-timescale.yml ps timescaledb

echo ""
echo "Building TimescaleDB worker..."
go build -o mongo_nats_worker_timescale main_timescale.go

if [ $? -ne 0 ]; then
    echo "Error: Build failed"
    exit 1
fi

echo ""
echo "Starting worker in background..."
export NATS_URL=nats://localhost:4222
export POSTGRES_HOST=localhost
export POSTGRES_PORT=5433
export POSTGRES_USER=gps_admin
export POSTGRES_PASSWORD=changeme_strong_password
export POSTGRES_DB=gps_tracking
export DISTANCE_THRESHOLD=5.0

./mongo_nats_worker_timescale &
WORKER_PID=$!

echo "Worker started with PID: $WORKER_PID"
echo "Waiting for worker to initialize..."
sleep 3

echo ""
echo "Sending test message..."
cd test
go run test_sender.go
cd ..

echo ""
echo "Waiting for message processing..."
sleep 2

echo ""
echo "Checking database for inserted record..."
docker-compose -f docker-compose-timescale.yml exec -T timescaledb psql -U gps_admin -d gps_tracking -c "SELECT time, device_id, user_id, fleet, latitude, longitude FROM gps_positions ORDER BY time DESC LIMIT 5;"

echo ""
echo "Stopping worker..."
kill $WORKER_PID

echo ""
echo "Test complete! To cleanup, run:"
echo "  docker-compose -f docker-compose-timescale.yml down -v"
