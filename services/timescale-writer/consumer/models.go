package consumer

import (
	"encoding/json"
	"net"
	"time"
)

// MongoLocation compatible con el documento del sistema
type MongoLocation struct {
	Type        string    `json:"type"`
	Coordinates []float64 `json:"coordinates"`
}

// Document — mensaje que publica coordinates-service a NATS
type Document struct {
	UniqueID     string        `json:"unique_id"`
	UserID       string        `json:"user_id"`
	Fleet        string        `json:"fleet"`
	Location     MongoLocation `json:"location"`
	OriginIP     string        `json:"ip_origin"`
	LastModified int64         `json:"last_modified"`
}

// CoordinateRow — fila para insertar en TimescaleDB
type CoordinateRow struct {
	Timestamp  time.Time
	DeviceID   string
	UserID     string
	Fleet      string
	Longitude  float64
	Latitude   float64
	IPOrigin   *net.IP
}

// DocumentToRow convierte Document (mensaje NATS) a CoordinateRow (fila DB)
func DocumentToRow(doc *Document) *CoordinateRow {
	var ip *net.IP
	if parsed := net.ParseIP(doc.OriginIP); parsed != nil {
		ip = &parsed
	}

	return &CoordinateRow{
		Timestamp: time.Unix(doc.LastModified/1000, (doc.LastModified%1000)*int64(time.Millisecond)).UTC(),
		DeviceID:  doc.UniqueID,
		UserID:    doc.UserID,
		Fleet:     doc.Fleet,
		Longitude: doc.Location.Coordinates[0], // GeoJSON: [lng, lat]
		Latitude:  doc.Location.Coordinates[1],
		IPOrigin:  ip,
	}
}

// Validate verifica campos obligatorios
func (d *Document) Validate() error {
	if d.UniqueID == "" {
		return ErrEmptyUniqueID
	}
	if d.UserID == "" {
		return ErrEmptyUserID
	}
	if d.Fleet == "" {
		return ErrEmptyFleet
	}
	if d.Location.Type == "" {
		return ErrEmptyLocationType
	}
	if len(d.Location.Coordinates) != 2 {
		return ErrInvalidCoordinates
	}
	if d.Location.Coordinates[0] < -180 || d.Location.Coordinates[0] > 180 {
		return ErrInvalidLongitude
	}
	if d.Location.Coordinates[1] < -90 || d.Location.Coordinates[1] > 90 {
		return ErrInvalidLatitude
	}
	return nil
}

// UnmarshalDocument deserializa JSON a Document
func UnmarshalDocument(data []byte) (*Document, error) {
	var doc Document
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	return &doc, nil
}
