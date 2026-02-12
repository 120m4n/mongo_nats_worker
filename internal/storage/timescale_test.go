package storage

import (
	"context"
	"testing"
	"time"

	"github.com/120m4n/mongo_nats/pkg/models"
)

// TestGPSPositionConversion tests the conversion from NATSMessage to GPSPosition
func TestGPSPositionConversion(t *testing.T) {
	now := time.Now().Unix()
	natsMsg := models.NATSMessage{
		UniqueID:     "device-123",
		UserID:       "user-456",
		Fleet:        "fleet-789",
		Location: models.Location{
			Type:        "Point",
			Coordinates: []float64{-74.0060, 40.7128}, // GeoJSON format: [longitude, latitude]
		},
		OriginIP:     "192.168.1.100",
		LastModified: now,
	}

	gps := natsMsg.ToGPSPosition()

	if gps.DeviceID != natsMsg.UniqueID {
		t.Errorf("Expected DeviceID %s, got %s", natsMsg.UniqueID, gps.DeviceID)
	}

	if gps.UserID != natsMsg.UserID {
		t.Errorf("Expected UserID %s, got %s", natsMsg.UserID, gps.UserID)
	}

	if gps.Fleet != natsMsg.Fleet {
		t.Errorf("Expected Fleet %s, got %s", natsMsg.Fleet, gps.Fleet)
	}

	// GeoJSON coordinates are [longitude, latitude]
	if gps.Longitude != natsMsg.Location.Coordinates[0] {
		t.Errorf("Expected Longitude %f, got %f", natsMsg.Location.Coordinates[0], gps.Longitude)
	}

	if gps.Latitude != natsMsg.Location.Coordinates[1] {
		t.Errorf("Expected Latitude %f, got %f", natsMsg.Location.Coordinates[1], gps.Latitude)
	}

	if gps.OriginIP != natsMsg.OriginIP {
		t.Errorf("Expected OriginIP %s, got %s", natsMsg.OriginIP, gps.OriginIP)
	}

	expectedTime := time.Unix(now, 0).UTC()
	if !gps.Time.Equal(expectedTime) {
		t.Errorf("Expected Time %v, got %v", expectedTime, gps.Time)
	}
}

// TestTimescaleDBConnection tests basic connection (requires running TimescaleDB)
func TestTimescaleDBConnection(t *testing.T) {
	// Skip if not in integration test mode
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	ts, err := NewTimescaleDB("localhost", "5433", "gps_admin", "changeme_strong_password", "gps_tracking")
	if err != nil {
		t.Skipf("Skipping test - TimescaleDB not available: %v", err)
		return
	}
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := ts.HealthCheck(ctx); err != nil {
		t.Errorf("Health check failed: %v", err)
	}
}

// TestInsertPosition tests inserting a GPS position (requires running TimescaleDB)
func TestInsertPosition(t *testing.T) {
	// Skip if not in integration test mode
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	ts, err := NewTimescaleDB("localhost", "5433", "gps_admin", "changeme_strong_password", "gps_tracking")
	if err != nil {
		t.Skipf("Skipping test - TimescaleDB not available: %v", err)
		return
	}
	defer ts.Close()

	pos := &models.GPSPosition{
		Time:      time.Now().UTC(),
		DeviceID:  "test-device-1",
		UserID:    "test-user-1",
		Fleet:     "test-fleet-1",
		Longitude: -74.0060, // New York longitude
		Latitude:  40.7128,  // New York latitude
		OriginIP:  "127.0.0.1",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := ts.InsertPosition(ctx, pos); err != nil {
		t.Errorf("Failed to insert position: %v", err)
	}
}
