package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"

	"github.com/120m4n/timescale-writer/config"
	"github.com/120m4n/timescale-writer/consumer"
	"github.com/120m4n/timescale-writer/health"
	"github.com/120m4n/timescale-writer/writer"
)

func main() {
	log.Println("[MAIN] Starting timescale-writer service")

	cfg := config.LoadConfig()
	log.Printf("[CONFIG] NATS: %s @ %s", cfg.NatsAddress, cfg.NatsSubject)
	log.Printf("[CONFIG] TimescaleDB: %s", maskDSN(cfg.TimescaleDSN))
	log.Printf("[CONFIG] Batch: size=%d, flush=%dms", cfg.BatchSize, cfg.FlushIntervalMs)

	// Conectar a NATS
	nc, err := nats.Connect(cfg.NatsAddress)
	if err != nil {
		log.Fatalf("[NATS] Failed to connect: %v", err)
	}
	defer nc.Close()
	log.Println("[NATS] Connected")

	// Connectar a TimescaleDB
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	pool, err := pgxpool.New(ctx, cfg.TimescaleDSN)
	cancel()
	if err != nil {
		log.Fatalf("[DB] Failed to connect: %v", err)
	}
	defer pool.Close()

	// Health check DB
	ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
	if err := pool.Ping(ctx); err != nil {
		cancel()
		log.Fatalf("[DB] Health check failed: %v", err)
	}
	cancel()
	log.Println("[DB] Connected and healthy")

	// Crear batch writer
	bw := writer.NewBatchWriter(pool, cfg.BatchSize, cfg.FlushIntervalMs)
	bw.Start(context.Background())

	// Crear NATS consumer
	handler := consumer.NewHandler(cfg.NatsSubject, bw.InputChannel())
	_, err = handler.Subscribe(nc)
	if err != nil {
		log.Fatalf("[CONSUMER] Subscribe failed: %v", err)
	}

	// Health server
	healthSrv := health.NewServer(cfg.HealthPort, nc, pool)
	healthSrv.Start()
	log.Printf("[HEALTH] Server started on :%s", cfg.HealthPort)

	// Graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	log.Println("[MAIN] Service ready. Waiting for messages...")
	<-sigChan

	log.Println("[MAIN] Shutdown signal received")
	pool.Close()
	nc.Close()
	log.Println("[MAIN] Goodbye")
}

// maskDSN enmascarada la contraseña de la DSN para logs
func maskDSN(dsn string) string {
	// Simplemente no mostrar la contraseña completa
	if len(dsn) > 50 {
		return dsn[:50] + "..."
	}
	return dsn
}
