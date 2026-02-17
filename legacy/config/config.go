package config

import (
	"log"
	"os"

	"github.com/joho/godotenv"
	"fmt"
)

type Config struct {
    NatsURL             string
    MongoURI            string
    DatabaseName        string
    Coor_CollectionName string
    Hook_CollectionName string
    DistanceThreshold   float64
    
    // TimescaleDB config
    DBType         string // "mongo" or "timescale"
    PostgresHost   string
    PostgresPort   string
    PostgresUser   string
    PostgresPass   string
    PostgresDB     string
}

func LoadConfig() Config {
	err := godotenv.Load()
	if err != nil {
		log.Println("No .env file found, using default values")
	}

    return Config{
        NatsURL:             getEnv("NATS_URL", "nats://localhost:4222"),
        MongoURI:            getEnv("MONGO_URI", "mongodb://localhost:27017"),
        DatabaseName:        getEnv("DATABASE_NAME", "test"),
        Coor_CollectionName: getEnv("COORDINATE_COLLECTION_NAME", "coordinates"),
        Hook_CollectionName: getEnv("HOOK_COLLECTION_NAME", "hooks"),
        DistanceThreshold:   getEnvFloat("DISTANCE_THRESHOLD", 5.0),
        
        // TimescaleDB config
        DBType:       getEnv("DB_TYPE", "mongo"),
        PostgresHost: getEnv("POSTGRES_HOST", "localhost"),
        PostgresPort: getEnv("POSTGRES_PORT", "5432"),
        PostgresUser: getEnv("POSTGRES_USER", "gps_admin"),
        PostgresPass: getEnv("POSTGRES_PASSWORD", "changeme_strong_password"),
        PostgresDB:   getEnv("POSTGRES_DB", "gps_tracking"),
    }
}

func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}

func getEnvFloat(key string, defaultValue float64) float64 {
    if value, exists := os.LookupEnv(key); exists {
        var f float64
        _, err := fmt.Sscanf(value, "%f", &f)
        if err == nil {
            return f
        }
    }
    return defaultValue
}