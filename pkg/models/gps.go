package models

import (
	"time"
)

// GPSPosition represents a GPS position from NATS
type GPSPosition struct {
	Time         time.Time `json:"time"`
	DeviceID     string    `json:"unique_id"`
	UserID       string    `json:"user_id"`
	Fleet        string    `json:"fleet"`
	Latitude     float64   `json:"latitude"`
	Longitude    float64   `json:"longitude"`
	Altitude     *float64  `json:"altitude,omitempty"`
	Speed        *float64  `json:"speed,omitempty"`
	Heading      *float64  `json:"heading,omitempty"`
	Accuracy     *float64  `json:"accuracy,omitempty"`
	BatteryLevel *int      `json:"battery_level,omitempty"`
	OriginIP     string    `json:"ip_origin,omitempty"`
	LastModified int64     `json:"last_modified"`
	Metadata     []byte    `json:"metadata,omitempty"` // JSONB stored as bytes
}

// Location represents the location part of the message for backward compatibility
type Location struct {
	Type        string    `json:"type"`
	Coordinates []float64 `json:"coordinates"`
}

// MongoLocation is an alias for Location for backward compatibility
type MongoLocation = Location

// NATSMessage represents the incoming message from NATS (compatible with MongoDB version)
type NATSMessage struct {
	UniqueID     string   `json:"unique_id"`
	UserID       string   `json:"user_id"`
	Fleet        string   `json:"fleet"`
	Location     Location `json:"location"`
	OriginIP     string   `json:"ip_origin"`
	LastModified int64    `json:"last_modified"`
}

// ToGPSPosition converts NATSMessage to GPSPosition
func (nm *NATSMessage) ToGPSPosition() *GPSPosition {
	gps := &GPSPosition{
		Time:         time.Unix(nm.LastModified, 0).UTC(),
		DeviceID:     nm.UniqueID,
		UserID:       nm.UserID,
		Fleet:        nm.Fleet,
		OriginIP:     nm.OriginIP,
		LastModified: nm.LastModified,
	}

	// Extract coordinates from location
	if len(nm.Location.Coordinates) >= 2 {
		gps.Latitude = nm.Location.Coordinates[0]
		gps.Longitude = nm.Location.Coordinates[1]
	}

	return gps
}
