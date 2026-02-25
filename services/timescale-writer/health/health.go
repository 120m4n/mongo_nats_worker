package health

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/120m4n/timescale-writer/consumer"
	"github.com/120m4n/timescale-writer/writer"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"
)

type HealthResponse struct {
	Status       string    `json:"status"`
	Timestamp    time.Time `json:"timestamp"`
	Uptime       string    `json:"uptime"`
	NATS         ServiceStatus `json:"nats"`
	TimescaleDB  ServiceStatus `json:"timescaledb"`
	Metrics      MetricsData   `json:"metrics"`
}

type ServiceStatus struct {
	Connected bool   `json:"connected"`
	Error     string `json:"error,omitempty"`
}

type MetricsData struct {
	MessagesReceived int64 `json:"messages_received"`
	MessagesDropped  int64 `json:"messages_dropped"`
	ValidationErrors int64 `json:"validation_errors"`
	MarshalErrors    int64 `json:"marshal_errors"`
	BatchesWritten   int64 `json:"batches_written"`
	RowsWritten      int64 `json:"rows_written"`
	WriterErrors     int64 `json:"writer_errors"`
}

var startTime = time.Now()

// Server maneja los endpoints de health
type Server struct {
	port   string
	nc     *nats.Conn
	pool   *pgxpool.Pool
}

// NewServer crea un servidor de health
func NewServer(port string, nc *nats.Conn, pool *pgxpool.Pool) *Server {
	return &Server{
		port: port,
		nc:   nc,
		pool: pool,
	}
}

// Start levanta el servidor HTTP
func (s *Server) Start() {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/metrics", s.handleMetrics)

	addr := fmt.Sprintf(":%s", s.port)
	log.Printf("[HEALTH] Starting health server on %s", addr)
	go func() {
		if err := http.ListenAndServe(addr, mux); err != nil {
			log.Printf("[HEALTH] Server error: %v", err)
		}
	}()
}

// handleHealth retorna el estado del sistema
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	natsStatus := ServiceStatus{Connected: s.nc != nil && s.nc.IsConnected()}
	if !natsStatus.Connected && s.nc != nil {
		natsStatus.Error = "disconnected from NATS"
	}

	dbStatus := ServiceStatus{Connected: true}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.pool.Ping(ctx); err != nil {
		dbStatus.Connected = false
		dbStatus.Error = err.Error()
	}

	resp := HealthResponse{
		Status:      "ok",
		Timestamp:   time.Now(),
		Uptime:      time.Since(startTime).String(),
		NATS:        natsStatus,
		TimescaleDB: dbStatus,
		Metrics: MetricsData{
			MessagesReceived: atomic.LoadInt64(&consumer.MetricsData.MessagesReceived),
			MessagesDropped:  atomic.LoadInt64(&consumer.MetricsData.MessagesDropped),
			ValidationErrors: atomic.LoadInt64(&consumer.MetricsData.ValidationErrors),
			MarshalErrors:    atomic.LoadInt64(&consumer.MetricsData.MarshalErrors),
			BatchesWritten:   atomic.LoadInt64(&writer.MetricsData.BatchesWritten),
			RowsWritten:      atomic.LoadInt64(&writer.MetricsData.RowsWritten),
			WriterErrors:     atomic.LoadInt64(&writer.MetricsData.WriterErrors),
		},
	}

	json.NewEncoder(w).Encode(resp)
}

// handleMetrics retorna métricas en formato Prometheus (simplificado)
func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")

	fmt.Fprintf(w, "# HELP timescale_writer_messages_received_total Total messages received from NATS\n")
	fmt.Fprintf(w, "# TYPE timescale_writer_messages_received_total counter\n")
	fmt.Fprintf(w, "timescale_writer_messages_received_total %d\n", atomic.LoadInt64(&consumer.MetricsData.MessagesReceived))

	fmt.Fprintf(w, "\n# HELP timescale_writer_messages_dropped_total Total messages dropped (batch channel full)\n")
	fmt.Fprintf(w, "# TYPE timescale_writer_messages_dropped_total counter\n")
	fmt.Fprintf(w, "timescale_writer_messages_dropped_total %d\n", atomic.LoadInt64(&consumer.MetricsData.MessagesDropped))

	fmt.Fprintf(w, "\n# HELP timescale_writer_batches_written_total Total batches written to TimescaleDB\n")
	fmt.Fprintf(w, "# TYPE timescale_writer_batches_written_total counter\n")
	fmt.Fprintf(w, "timescale_writer_batches_written_total %d\n", atomic.LoadInt64(&writer.MetricsData.BatchesWritten))

	fmt.Fprintf(w, "\n# HELP timescale_writer_rows_written_total Total rows written to TimescaleDB\n")
	fmt.Fprintf(w, "# TYPE timescale_writer_rows_written_total counter\n")
	fmt.Fprintf(w, "timescale_writer_rows_written_total %d\n", atomic.LoadInt64(&writer.MetricsData.RowsWritten))

	fmt.Fprintf(w, "\n# HELP timescale_writer_errors_total Total errors\n")
	fmt.Fprintf(w, "# TYPE timescale_writer_errors_total counter\n")
	fmt.Fprintf(w, "timescale_writer_errors_total %d\n", atomic.LoadInt64(&consumer.MetricsData.MarshalErrors)+
		atomic.LoadInt64(&consumer.MetricsData.ValidationErrors)+
		atomic.LoadInt64(&writer.MetricsData.WriterErrors))
}
