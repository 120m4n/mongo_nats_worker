# Architecture Diagram - TimescaleDB GPS Worker

```
┌─────────────────────────────────────────────────────────────────┐
│                        GPS Tracking System                       │
│                     TimescaleDB Implementation                   │
└─────────────────────────────────────────────────────────────────┘

┌──────────────┐                                                    
│  GPS Devices │                                                    
│   / Sensors  │                                                    
└──────┬───────┘                                                    
       │ Send position data                                        
       │ (JSON via NATS)                                          
       ▼                                                            
┌──────────────────────────────────────────────────────────┐      
│                    NATS Message Broker                    │      
│  Topic: "coordinates"                                     │      
│  Port: 4222 (client), 8222 (monitoring)                 │      
└──────────────┬───────────────────────────────────────────┘      
               │                                                   
               │ Subscribe to topic                               
               ▼                                                   
┌──────────────────────────────────────────────────────────┐      
│            GPS Worker (main_timescale.go)                │      
│ ┌────────────────────────────────────────────────────┐  │      
│ │  Message Processor                                  │  │      
│ │  - Validate incoming JSON                          │  │      
│ │  - Convert to GPSPosition model                    │  │      
│ │  - Check proximity cache (distance threshold)      │  │      
│ │  - Filter redundant positions                      │  │      
│ └────────────────────────────────────────────────────┘  │      
│ ┌────────────────────────────────────────────────────┐  │      
│ │  Worker Pool (5 concurrent workers)                │  │      
│ │  - Process messages in parallel                    │  │      
│ │  - Insert into TimescaleDB                        │  │      
│ │  - Update cache on success                        │  │      
│ └────────────────────────────────────────────────────┘  │      
│ ┌────────────────────────────────────────────────────┐  │      
│ │  Cache Manager (in-memory)                         │  │      
│ │  - Thread-safe location cache                     │  │      
│ │  - Track last known position per device           │  │      
│ └────────────────────────────────────────────────────┘  │      
└──────────────┬───────────────────────────────────────────┘      
               │ Insert GPS positions                             
               │ (PostgreSQL protocol)                            
               ▼                                                   
┌──────────────────────────────────────────────────────────┐      
│              TimescaleDB (PostgreSQL + Extensions)        │      
│  Port: 5432 (external: 5433)                            │      
│ ┌────────────────────────────────────────────────────┐  │      
│ │  gps_positions (Hypertable)                        │  │      
│ │  - Automatic time-based partitioning (1-day chunks)│  │      
│ │  - Columns: time, device_id, user_id, fleet,      │  │      
│ │    latitude, longitude, altitude, speed, etc.     │  │      
│ │  - PostGIS geography type for spatial queries     │  │      
│ └────────────────────────────────────────────────────┘  │      
│ ┌────────────────────────────────────────────────────┐  │      
│ │  Indexes                                           │  │      
│ │  - (device_id, time DESC) - for device queries    │  │      
│ │  - (user_id, time DESC) - for user queries       │  │      
│ │  - GIST(geom) - for spatial queries              │  │      
│ │  - GIN(metadata) - for JSON queries               │  │      
│ └────────────────────────────────────────────────────┘  │      
│ ┌────────────────────────────────────────────────────┐  │      
│ │  Policies                                          │  │      
│ │  - Retention: 90 days (automatic cleanup)         │  │      
│ │  - Compression: TimescaleDB background workers    │  │      
│ └────────────────────────────────────────────────────┘  │      
└──────────────┬───────────────────────────────────────────┘      
               │                                                   
               │ Query and manage                                 
               ▼                                                   
┌──────────────────────────────────────────────────────────┐      
│                    Management Tools                       │      
│                                                          │      
│  ┌─────────────────┐        ┌─────────────────┐        │      
│  │   PgAdmin 4     │        │    Grafana      │        │      
│  │  Port: 5050     │        │   Port: 3000    │        │      
│  │                 │        │                 │        │      
│  │  - Database UI  │        │  - Dashboards   │        │      
│  │  - SQL queries  │        │  - Metrics      │        │      
│  │  - Table view   │        │  - Alerts       │        │      
│  └─────────────────┘        └─────────────────┘        │      
└──────────────────────────────────────────────────────────┘      

┌─────────────────────────────────────────────────────────────────┐
│                        Data Flow Example                         │
└─────────────────────────────────────────────────────────────────┘

1. GPS Device sends:
   {
     "unique_id": "truck-001",
     "user_id": "driver-123",
     "fleet": "delivery",
     "location": {
       "type": "Point",
       "coordinates": [-99.1332, 19.4326]  // [lon, lat]
     },
     "ip_origin": "10.0.1.5",
     "last_modified": 1708000000
   }

2. NATS receives and queues message

3. Worker processes:
   - Validates required fields
   - Converts to GPSPosition
   - Checks cache: Is device nearby last position?
   - If distance > 5m threshold: Insert to DB
   - Updates cache with new position

4. TimescaleDB stores:
   INSERT INTO gps_positions (
     time, device_id, user_id, fleet,
     latitude, longitude, geom, ...
   ) VALUES (
     '2024-02-15 12:00:00+00', 'truck-001', 'driver-123',
     'delivery', 19.4326, -99.1332,
     ST_SetSRID(ST_MakePoint(-99.1332, 19.4326), 4326), ...
   );

5. Available for querying:
   - Last position per device
   - Route history
   - Geofence checks
   - Speed analytics
   - Fleet reporting

┌─────────────────────────────────────────────────────────────────┐
│                      Network Configuration                       │
└─────────────────────────────────────────────────────────────────┘

Docker Network: gps_network (172.20.0.0/16)

Services:
  timescaledb:5432 → localhost:5433
  pgadmin:80       → localhost:5050
  nats:4222        → localhost:4222
  nats:8222        → localhost:8222
  grafana:3000     → localhost:3000

Internal DNS:
  - timescaledb (container name)
  - nats (container name)
  - pgadmin (container name)
  - grafana (container name)
```
