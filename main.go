package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	// "go.mongodb.org/mongo-driver/bson"
	"errors"

	"github.com/120m4n/mongo_nats/config"
	"github.com/120m4n/mongo_nats/model"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func main() {
	// Load configuration
	cfg := config.LoadConfig()

	// mostrar las variables configuradas
	fmt.Printf("Configuración cargada:\n")
	fmt.Printf("NatsURL: %s\n", cfg.NatsURL)
	fmt.Printf("MongoURI: %s\n", cfg.MongoURI)
	fmt.Printf("DatabaseName: %s\n", cfg.DatabaseName)
	fmt.Printf("Coor_CollectionName: %s\n", cfg.Coor_CollectionName)

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

	// Mapa en memoria para última coordenada por UniqueId
	lastCoords := make(map[string][2]float64)

	// Mutex para acceso concurrente al mapa
	var lastCoordsMutex = &sync.Mutex{}

	startWorkerPool(numWorkers, docsChan, collection, lastCoords, lastCoordsMutex)

	// Subscribe to "coordinates" topic
	if err := subscribeCoordinates(nc, docsChan); err != nil {
		log.Fatalf("Error subscribing to topic: %v", err)
	}

	// Mantener el proceso vivo
	select {}
}

func startWorkerPool(numWorkers int, docsChan <-chan model.Document, collection *mongo.Collection, lastCoords map[string][2]float64, lastCoordsMutex *sync.Mutex) {
	for i := 0; i < numWorkers; i++ {
		go func(id int) {
			for doc := range docsChan {
				uniqueId := doc.UniqueId
				coords := doc.Location.Coordinates
				if len(coords) != 2 {
					log.Printf("Worker %d: Coordenadas inválidas para UniqueId %s", id, uniqueId)
					continue
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
		}(i)
	}
}

// processDocument handles the insertion of a document into MongoDB
func processDocument(id int, doc model.Document, collection *mongo.Collection) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := collection.InsertOne(ctx, doc)
	if err != nil {
		log.Printf("Worker %d: Error inserting into MongoDB: %v", id, err)
	} else {
		fmt.Printf("Worker %d: Inserted document: %+v\n", id, doc)
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
			log.Printf("Documento inválido: %v. Datos: %+v", err, doc)
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
		return errors.New("UniqueId vacío")
	}
	if doc.UserId == "" {
		return errors.New("UserId vacío")
	}
	if doc.Fleet == "" {
		return errors.New("Fleet vacío")
	}
	if doc.Location.Type == "" {
		return errors.New("Location.Type vacío")
	}
	if len(doc.Location.Coordinates) != 2 {
		return errors.New("Location.Coordinates debe tener longitud 2 (lat,lon)")
	}
	// Puedes agregar más validaciones según tu modelo
	return nil
}
