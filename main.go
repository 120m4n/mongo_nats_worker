package main

import (
	"context"
	"encoding/json"
	"log"
	"sync/atomic"
	"time"
	"github.com/nats-io/nats.go"
	"errors"

	"github.com/120m4n/mongo_nats/config"
	"github.com/120m4n/mongo_nats/model"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"math"
    "github.com/120m4n/mongo_nats/internal"
)

// Contadores globales para estadísticas
var (
    processedCount   int64
    errorCount       int64
    validationErrors int64
    cacheHits        int64
    cacheMisses      int64
)

func main() {
	// Load configuration
	cfg := config.LoadConfig()

	// Log inicial de configuración (solo al iniciar)
	log.Printf("Worker iniciado - DB: %s, Collection: %s, Workers: 5", cfg.DatabaseName, cfg.Coor_CollectionName)

	// Connect to NATS server
	nc, err := nats.Connect(cfg.NatsURL)
	if err != nil {
		log.Fatalf("Error connecting to NATS: %v", err)
	}
	defer nc.Close()

	// Connect to MongoDB
	clientOptions := options.Client().ApplyURI(cfg.MongoURI)
	mongoClient, err := mongo.Connect(context.Background(), clientOptions)
	if err != nil {
		log.Fatalf("Error connecting to MongoDB: %v", err)
	}
	defer mongoClient.Disconnect(context.Background())

	// Get a handle for your collection usando configuración
	collection := mongoClient.Database(cfg.DatabaseName).Collection(cfg.Coor_CollectionName)

	// Canal para documentos recibidos
	docsChan := make(chan model.Document, 100) // buffer configurable

	// Pool de workers
	numWorkers := 5 // puedes ajustar este valor
	cache := internal.NewCacheManager()
	startWorkerPool(numWorkers, docsChan, collection, cache, cfg.DistanceThreshold)

	// Iniciar reporte de estadísticas cada 30 segundos
	go startStatsReporter()

	// Subscribe to "coordinates" topic
	if err := subscribeCoordinates(nc, docsChan); err != nil {
		log.Fatalf("Error subscribing to topic: %v", err)
	}

	// Mantener el proceso vivo
	select {}
}

// startWorkerPool launches a pool of workers to process documents
func startWorkerPool(numWorkers int, docsChan <-chan model.Document, collection *mongo.Collection, cache *internal.CacheManager, threshold float64) {
    for i := 0; i < numWorkers; i++ {
        go func(id int) {
            for doc := range docsChan {
                processDocument(id, doc, collection, cache, threshold)
            }
        }(i)
    }
}

func handleDocument(id int, doc model.Document, collection *mongo.Collection, lastCoords map[string][2]float64, lastCoordsMutex *sync.Mutex) {
	uniqueId := doc.UniqueId
	coords := doc.Location.Coordinates
	if len(coords) != 2 {
		log.Printf("Worker %d: Coordenadas inválidas para UniqueId %s", id, uniqueId)
		return
	}
	lat, lon := coords[0], coords[1]
	store := false

	lastCoordsMutex.Lock()
	prev, exists := lastCoords[uniqueId]
	if !exists {
		// No existe coordenada previa, almacenar y registrar
		lastCoords[uniqueId] = [2]float64{lat, lon}
		store = true
	} else {
		dist := haversine(prev[0], prev[1], lat, lon)
		if dist >= 5.0 {
			lastCoords[uniqueId] = [2]float64{lat, lon}
			store = true
		}
	}
	lastCoordsMutex.Unlock()

	if store {
		processDocument(id, doc, collection)
	} else {
		log.Printf("Worker %d: Coordenada ignorada para UniqueId %s, distancia < 5m", id, uniqueId)
	}
}

// processDocument handles the insertion of a document into MongoDB
func processDocument(id int, doc model.Document, collection *mongo.Collection, cache *internal.CacheManager, threshold float64) {
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    doc.Fecha = time.Unix(doc.LastModified, 0).UTC()
    uniqueId := doc.UniqueId
    newLoc := doc.Location

    // Lógica de cache y proximidad
    if prevLoc, exists := cache.Get(uniqueId); exists {
		atomic.AddInt64(&cacheHits, 1)
        dist := haversineDistance(prevLoc.Coordinates[0], prevLoc.Coordinates[1], newLoc.Coordinates[0], newLoc.Coordinates[1])
        if dist <= threshold {
            // Si la distancia es menor o igual al umbral, no persistir ni actualizar cache
            return
        }
    } else {
		atomic.AddInt64(&cacheMisses, 1)
	}

    // Persistir en MongoDB
    _, err := collection.InsertOne(ctx, doc)
    if err != nil {
        atomic.AddInt64(&errorCount, 1)
        log.Printf("Worker %d: Error inserting into MongoDB: %v", id, err)
    } else {
        atomic.AddInt64(&processedCount, 1)
        // Actualizar cache con nueva ubicación
        cache.Set(uniqueId, newLoc)
    }
}

// startStatsReporter reporta estadísticas cada 30 segundos
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

		// Reset de contadores para evitar crecimiento indefinido
		atomic.StoreInt64(&processedCount, 0)
		atomic.StoreInt64(&errorCount, 0)
		atomic.StoreInt64(&validationErrors, 0)
		atomic.StoreInt64(&cacheHits, 0)
		atomic.StoreInt64(&cacheMisses, 0)
	}
}

// subscribeCoordinates subscribes to the "coordinates" topic and sends valid documents to docsChan
func subscribeCoordinates(nc *nats.Conn, docsChan chan<- model.Document) error {
	_, err := nc.Subscribe("coordinates", func(m *nats.Msg) {
		var doc model.Document
		if err := json.Unmarshal(m.Data, &doc); err != nil {
			log.Printf("Error unmarshalling data: %v", err)
			return
		}
		// Validar el documento antes de enviarlo al canal
		if err := validateDocument(doc); err != nil {
			atomic.AddInt64(&validationErrors, 1)
			// Solo loguear errores de validación críticos ocasionalmente
			return
		}
		// Enviar el documento al canal para procesamiento concurrente
		docsChan <- doc
	})
	return err
}

// validateDocument verifica los campos obligatorios y formato básico
// haversine calcula la distancia en metros entre dos puntos lat/lon
func haversine(lat1, lon1, lat2, lon2 float64) float64 {
	const R = 6371000.0 // radio de la Tierra en metros
	dLat := (lat2 - lat1) * math.Pi / 180.0
	dLon := (lon2 - lon1) * math.Pi / 180.0
	a := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Cos(lat1*math.Pi/180.0)*math.Cos(lat2*math.Pi/180.0)*math.Sin(dLon/2)*math.Sin(dLon/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	return R * c
}
func validateDocument(doc model.Document) error {
	if doc.UniqueId == "" {
		return errors.New("uniqueId vacío")
	}
	if doc.UserId == "" {
		return errors.New("userId vacío")
	}
	if doc.Fleet == "" {
		return errors.New("fleet vacío")
	}
	if doc.Location.Type == "" {
		return errors.New("kocation.Type vacío")
	}
	if len(doc.Location.Coordinates) != 2 {
		return errors.New("location.Coordinates debe tener longitud 2 (lat,lon)")
	}
	// Puedes agregar más validaciones según tu modelo
	return nil
}

// haversineDistance calcula la distancia en metros entre dos puntos geográficos
func haversineDistance(lat1, lon1, lat2, lon2 float64) float64 {
    const R = 6371000 // Radio de la Tierra en metros
    latRad1 := lat1 * math.Pi / 180
    latRad2 := lat2 * math.Pi / 180
    dLat := (lat2 - lat1) * math.Pi / 180
    dLon := (lon2 - lon1) * math.Pi / 180

    a := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Cos(latRad1)*math.Cos(latRad2)*math.Sin(dLon/2)*math.Sin(dLon/2)
    c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
    return R * c
}
