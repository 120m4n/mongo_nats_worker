package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"math"
	"sync/atomic"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/120m4n/mongo_nats/config"
	"github.com/120m4n/mongo_nats/internal"
	"github.com/120m4n/mongo_nats/internal/storage"
	"github.com/120m4n/mongo_nats/pkg/models"
)

// Contadores globales para estadísticas
var (
	processedCount   int64
	errorCount       int64
	validationErrors int64
	cacheHits        int64
	cacheMisses      int64
)

// LocationCache stores the last known location for proximity filtering
type LocationCache struct {
	Latitude  float64
	Longitude float64
}

func main() {
	// Load configuration
	cfg := config.LoadConfig()

	// Log inicial de configuración
	log.Printf("TimescaleDB Worker iniciado - DB: %s, Host: %s:%s, Workers: 5", 
		cfg.PostgresDB, cfg.PostgresHost, cfg.PostgresPort)

	// Connect to NATS server
	nc, err := nats.Connect(cfg.NatsURL)
	if err != nil {
		log.Fatalf("Error connecting to NATS: %v", err)
	}
	defer nc.Close()

	// Connect to TimescaleDB
	ts, err := storage.NewTimescaleDB(
		cfg.PostgresHost,
		cfg.PostgresPort,
		cfg.PostgresUser,
		cfg.PostgresPass,
		cfg.PostgresDB,
	)
	if err != nil {
		log.Fatalf("Error connecting to TimescaleDB: %v", err)
	}
	defer ts.Close()

	// Verify connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	if err := ts.HealthCheck(ctx); err != nil {
		cancel()
		log.Fatalf("TimescaleDB health check failed: %v", err)
	}
	cancel()
	log.Println("Successfully connected to TimescaleDB")

	// Canal para documentos recibidos
	docsChan := make(chan *models.GPSPosition, 100)

	// Cache manager para filtrado por proximidad
	cache := internal.NewCacheManager()

	// Pool de workers
	numWorkers := 5
	startWorkerPool(numWorkers, docsChan, ts, cache, cfg.DistanceThreshold)

	// Iniciar reporte de estadísticas cada 120 segundos
	go startStatsReporter()

	// Subscribe to "coordinates" topic
	if err := subscribeCoordinates(nc, docsChan); err != nil {
		log.Fatalf("Error subscribing to topic: %v", err)
	}

	log.Println("Worker ready, waiting for messages...")

	// Mantener el proceso vivo
	select {}
}

// startWorkerPool launches a pool of workers to process GPS positions
func startWorkerPool(numWorkers int, docsChan <-chan *models.GPSPosition, 
	ts *storage.TimescaleDB, cache *internal.CacheManager, threshold float64) {
	for i := 0; i < numWorkers; i++ {
		go func(id int) {
			for pos := range docsChan {
				processPosition(id, pos, ts, cache, threshold)
			}
		}(i)
	}
}

// processPosition handles the insertion of a GPS position into TimescaleDB
func processPosition(id int, pos *models.GPSPosition, ts *storage.TimescaleDB, 
	cache *internal.CacheManager, threshold float64) {
	
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	deviceID := pos.DeviceID
	newLat := pos.Latitude
	newLon := pos.Longitude

	// Lógica de cache y proximidad - usar estructura compatible
	cacheKey := deviceID
	if prevLoc, exists := cache.Get(cacheKey); exists {
		atomic.AddInt64(&cacheHits, 1)
		
		// Extract coordinates from cached location
		if len(prevLoc.Coordinates) >= 2 {
			prevLat := prevLoc.Coordinates[0]
			prevLon := prevLoc.Coordinates[1]
			
			dist := haversineDistance(prevLat, prevLon, newLat, newLon)
			if dist <= threshold {
				// Si la distancia es menor o igual al umbral, no persistir
				return
			}
		}
	} else {
		atomic.AddInt64(&cacheMisses, 1)
	}

	// Persistir en TimescaleDB
	err := ts.InsertPosition(ctx, pos)
	if err != nil {
		atomic.AddInt64(&errorCount, 1)
		log.Printf("Worker %d: Error inserting into TimescaleDB: %v", id, err)
	} else {
		atomic.AddInt64(&processedCount, 1)
		
		// Actualizar cache con nueva ubicación (formato compatible con MongoDB)
		cache.SetModels(cacheKey, models.MongoLocation{
			Type:        "Point",
			Coordinates: []float64{newLat, newLon},
		})
	}
}

// startStatsReporter reporta estadísticas cada 120 segundos
func startStatsReporter() {
	ticker := time.NewTicker(120 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		processed := atomic.LoadInt64(&processedCount)
		errors := atomic.LoadInt64(&errorCount)
		validationErrs := atomic.LoadInt64(&validationErrors)
		hits := atomic.LoadInt64(&cacheHits)
		misses := atomic.LoadInt64(&cacheMisses)
		
		log.Printf("Stats - Procesados: %d, Errores DB: %d, Errores validación: %d, Cache hits: %d, Cache misses: %d",
			processed, errors, validationErrs, hits, misses)

		// Reset de contadores
		atomic.StoreInt64(&processedCount, 0)
		atomic.StoreInt64(&errorCount, 0)
		atomic.StoreInt64(&validationErrors, 0)
		atomic.StoreInt64(&cacheHits, 0)
		atomic.StoreInt64(&cacheMisses, 0)
	}
}

// subscribeCoordinates subscribes to the "coordinates" topic
func subscribeCoordinates(nc *nats.Conn, docsChan chan<- *models.GPSPosition) error {
	_, err := nc.Subscribe("coordinates", func(m *nats.Msg) {
		var natsMsg models.NATSMessage
		if err := json.Unmarshal(m.Data, &natsMsg); err != nil {
			log.Printf("Error unmarshalling data: %v", err)
			return
		}

		// Validar el mensaje
		if err := validateMessage(natsMsg); err != nil {
			atomic.AddInt64(&validationErrors, 1)
			return
		}

		// Convertir a GPSPosition
		gpsPos := natsMsg.ToGPSPosition()

		// Enviar al canal para procesamiento
		docsChan <- gpsPos
	})
	return err
}

// validateMessage verifica los campos obligatorios
func validateMessage(msg models.NATSMessage) error {
	if msg.UniqueID == "" {
		return errors.New("unique_id vacío")
	}
	if msg.UserID == "" {
		return errors.New("user_id vacío")
	}
	if msg.Fleet == "" {
		return errors.New("fleet vacío")
	}
	if msg.Location.Type == "" {
		return errors.New("location.type vacío")
	}
	if len(msg.Location.Coordinates) != 2 {
		return errors.New("location.coordinates debe tener longitud 2")
	}
	return nil
}

// haversineDistance calcula la distancia en metros entre dos puntos geográficos
func haversineDistance(lat1, lon1, lat2, lon2 float64) float64 {
	const R = 6371000 // Radio de la Tierra en metros
	latRad1 := lat1 * math.Pi / 180
	latRad2 := lat2 * math.Pi / 180
	dLat := (lat2 - lat1) * math.Pi / 180
	dLon := (lon2 - lon1) * math.Pi / 180

	a := math.Sin(dLat/2)*math.Sin(dLat/2) + 
		math.Cos(latRad1)*math.Cos(latRad2)*math.Sin(dLon/2)*math.Sin(dLon/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	return R * c
}
