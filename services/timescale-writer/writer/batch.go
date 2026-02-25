package writer

import (
	"context"
	"fmt"
	"log"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/120m4n/timescale-writer/consumer"
)

// WriteMetrics tracks write statistics
type WriteMetrics struct {
	BatchesWritten int64
	RowsWritten    int64
	WriterErrors   int64
}

// MetricsData is the global metrics instance for the writer
var MetricsData = &WriteMetrics{}

// BatchWriter orchestrates batch inserts to TimescaleDB
type BatchWriter struct {
	pool            *pgxpool.Pool
	batchSize       int
	flushIntervalMs int
	inputChan       chan *consumer.CoordinateRow
}

// NewBatchWriter creates a batch writer
func NewBatchWriter(pool *pgxpool.Pool, batchSize, flushIntervalMs int) *BatchWriter {
	return &BatchWriter{
		pool:            pool,
		batchSize:       batchSize,
		flushIntervalMs: flushIntervalMs,
		inputChan:       make(chan *consumer.CoordinateRow, 10000),
	}
}

// InputChannel returns the input channel
func (bw *BatchWriter) InputChannel() chan *consumer.CoordinateRow {
	return bw.inputChan
}

// Start starts the writer in a goroutine
func (bw *BatchWriter) Start(ctx context.Context) {
	go bw.run(ctx)
}

// run is the main batching loop
func (bw *BatchWriter) run(ctx context.Context) {
	batch := make([]*consumer.CoordinateRow, 0, bw.batchSize)
	ticker := time.NewTicker(time.Duration(bw.flushIntervalMs) * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			// Final flush
			if len(batch) > 0 {
				bw.writeBatch(context.Background(), batch)
			}
			log.Println("[WRITER] Shutting down")
			return

		case row := <-bw.inputChan:
			batch = append(batch, row)
			if len(batch) >= bw.batchSize {
				bw.writeBatch(ctx, batch)
				batch = batch[:0]
				ticker.Reset(time.Duration(bw.flushIntervalMs) * time.Millisecond)
			}

		case <-ticker.C:
			if len(batch) > 0 {
				bw.writeBatch(ctx, batch)
				batch = batch[:0]
			}
		}
	}
}

// writeBatch writes a batch of rows using batch INSERT (parameterized)
func (bw *BatchWriter) writeBatch(ctx context.Context, rows []*consumer.CoordinateRow) {
	if len(rows) == 0 {
		return
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// Use pgx Batch for efficient multi-row insert with ST_SetSRID/ST_MakePoint
	batch := &pgx.Batch{}
	for _, row := range rows {
		batch.Queue(
			`INSERT INTO coordinates_history (ts, device_id, user_id, fleet, longitude, latitude, geom, ip_origin)
			 VALUES ($1, $2, $3, $4, $5, $6, ST_SetSRID(ST_MakePoint($5, $6), 4326), $7)`,
			row.Timestamp,
			row.DeviceID,
			row.UserID,
			row.Fleet,
			row.Longitude,
			row.Latitude,
			row.IPOrigin,
		)
	}

	br := bw.pool.SendBatch(ctx, batch)
	defer br.Close()

	var inserted int64
	for range rows {
		_, err := br.Exec()
		if err != nil {
			atomic.AddInt64(&MetricsData.WriterErrors, 1)
			log.Printf("[WRITER] Error inserting row: %v", err)
			continue
		}
		inserted++
	}

	if inserted > 0 {
		atomic.AddInt64(&MetricsData.BatchesWritten, 1)
		atomic.AddInt64(&MetricsData.RowsWritten, inserted)
		log.Printf("[WRITER] Batch written: %d rows", inserted)
	}
}

// formatEWKT creates an EWKT representation for a Point geometry with SRID 4326
func formatEWKT(lon, lat float64) string {
	return fmt.Sprintf("SRID=4326;POINT(%f %f)", lon, lat)
}
