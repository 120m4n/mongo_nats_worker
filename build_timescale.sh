#!/bin/bash

# Build script for TimescaleDB worker

echo "Building TimescaleDB worker..."
go build -o mongo_nats_worker_timescale main_timescale.go

if [ $? -eq 0 ]; then
    echo "Build successful! Binary: mongo_nats_worker_timescale"
else
    echo "Build failed!"
    exit 1
fi
