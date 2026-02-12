package storage

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/lib/pq"
	"github.com/120m4n/mongo_nats/pkg/models"
)

// TimescaleDB implements the storage interface for TimescaleDB
type TimescaleDB struct {
	db *sql.DB
}

// NewTimescaleDB creates a new TimescaleDB storage instance
func NewTimescaleDB(host, port, user, password, dbname string) (*TimescaleDB, error) {
	connStr := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		host, port, user, password, dbname)

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("error opening database: %w", err)
	}

	// Configure connection pool
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("error connecting to database: %w", err)
	}

	return &TimescaleDB{db: db}, nil
}

// InsertPosition inserts a GPS position into TimescaleDB
func (ts *TimescaleDB) InsertPosition(ctx context.Context, pos *models.GPSPosition) error {
	query := `
		INSERT INTO gps_positions (
			time, device_id, user_id, fleet, latitude, longitude,
			altitude, speed, heading, accuracy, battery_level, origin_ip, metadata, geom
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13,
			ST_SetSRID(ST_MakePoint($6, $5), 4326)::geography
		)
	`

	_, err := ts.db.ExecContext(ctx, query,
		pos.Time,
		pos.DeviceID,
		pos.UserID,
		pos.Fleet,
		pos.Latitude,
		pos.Longitude,
		pos.Altitude,
		pos.Speed,
		pos.Heading,
		pos.Accuracy,
		pos.BatteryLevel,
		pos.OriginIP,
		pos.Metadata,
	)

	if err != nil {
		return fmt.Errorf("error inserting position: %w", err)
	}

	return nil
}

// InsertPositionBatch inserts multiple GPS positions in a batch
func (ts *TimescaleDB) InsertPositionBatch(ctx context.Context, positions []*models.GPSPosition) error {
	if len(positions) == 0 {
		return nil
	}

	tx, err := ts.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("error starting transaction: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO gps_positions (
			time, device_id, user_id, fleet, latitude, longitude,
			altitude, speed, heading, accuracy, battery_level, origin_ip, metadata, geom
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13,
			ST_SetSRID(ST_MakePoint($6, $5), 4326)::geography
		)
	`)
	if err != nil {
		return fmt.Errorf("error preparing statement: %w", err)
	}
	defer stmt.Close()

	for _, pos := range positions {
		_, err := stmt.ExecContext(ctx,
			pos.Time,
			pos.DeviceID,
			pos.UserID,
			pos.Fleet,
			pos.Latitude,
			pos.Longitude,
			pos.Altitude,
			pos.Speed,
			pos.Heading,
			pos.Accuracy,
			pos.BatteryLevel,
			pos.OriginIP,
			pos.Metadata,
		)
		if err != nil {
			return fmt.Errorf("error inserting position in batch: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("error committing transaction: %w", err)
	}

	return nil
}

// GetLastPosition retrieves the last known position for a device
func (ts *TimescaleDB) GetLastPosition(ctx context.Context, deviceID string) (*models.GPSPosition, error) {
	query := `
		SELECT time, device_id, user_id, fleet, latitude, longitude,
		       altitude, speed, heading, accuracy, battery_level, origin_ip, metadata
		FROM gps_positions
		WHERE device_id = $1
		ORDER BY time DESC
		LIMIT 1
	`

	var pos models.GPSPosition
	err := ts.db.QueryRowContext(ctx, query, deviceID).Scan(
		&pos.Time,
		&pos.DeviceID,
		&pos.UserID,
		&pos.Fleet,
		&pos.Latitude,
		&pos.Longitude,
		&pos.Altitude,
		&pos.Speed,
		&pos.Heading,
		&pos.Accuracy,
		&pos.BatteryLevel,
		&pos.OriginIP,
		&pos.Metadata,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("error getting last position: %w", err)
	}

	return &pos, nil
}

// Close closes the database connection
func (ts *TimescaleDB) Close() error {
	if ts.db != nil {
		return ts.db.Close()
	}
	return nil
}

// HealthCheck verifies the database connection is healthy
func (ts *TimescaleDB) HealthCheck(ctx context.Context) error {
	return ts.db.PingContext(ctx)
}
