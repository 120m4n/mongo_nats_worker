package config

import (
	"os"
	"strconv"
)

type Config struct {
	// NATS
	NatsAddress string
	NatsSubject string

	// TimescaleDB
	TimescaleDSN string

	// Batching
	BatchSize       int
	FlushIntervalMs int

	// Health
	HealthPort string

	// Geo cache filter
	DistanceThresholdM float64
}

func LoadConfig() *Config {
	return &Config{
		NatsAddress: getEnv("NATS_ADDRESS", "nats://localhost:4222"),
		NatsSubject: getEnv("NATS_SUBJECT", "coordinates"),

		TimescaleDSN: getEnv("TIMESCALE_DSN", "postgres://geo_user:geo_password@localhost:5432/geosmart?sslmode=disable"),

		BatchSize:       getEnvInt("BATCH_SIZE", 500),
		FlushIntervalMs: getEnvInt("FLUSH_INTERVAL_MS", 2000),

		HealthPort: getEnv("HEALTH_PORT", "3010"),

		DistanceThresholdM: getEnvFloat("DISTANCE_THRESHOLD", 10.0),
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
	}
	return defaultValue
}

func getEnvFloat(key string, defaultValue float64) float64 {
	if value := os.Getenv(key); value != "" {
		if f, err := strconv.ParseFloat(value, 64); err == nil {
			return f
		}
	}
	return defaultValue
}
