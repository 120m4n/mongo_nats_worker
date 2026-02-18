package main

import (
	"encoding/json"
	"log"
	"os"
	"time"

	"github.com/nats-io/nats.go"
)

// Document y MongoLocation deben coincidir con el modelo del worker

type MongoLocation struct {
	Type        string    `json:"type"`
	Coordinates []float64 `json:"coordinates"`
}

type Document struct {
	UniqueId     string        `json:"unique_id"`
	UserId       string        `json:"user_id"`
	Fleet        string        `json:"fleet"`
	Location     MongoLocation `json:"location"`
	OriginIp     string        `json:"ip_origin"`
	LastModified int64         `json:"last_modified"`
}

func main() {
	natsURL := os.Getenv("NATS_URL")
	if natsURL == "" {
		natsURL = "nats://localhost:4222"
	}

	nc, err := nats.Connect(natsURL)
	if err != nil {
		log.Fatalf("Error connecting to NATS: %v", err)
	}
	defer nc.Close()

	doc := Document{
		UniqueId:     "test-123",
		UserId:       "user-1",
		Fleet:        "fleet-A",
		Location:     MongoLocation{Type: "Point", Coordinates: []float64{-99.1332, 19.4326}},
		OriginIp:     "127.0.0.1",
		LastModified: time.Now().Unix(),
	}

	data, err := json.Marshal(doc)
	if err != nil {
		log.Fatalf("Error marshalling document: %v", err)
	}

	err = nc.Publish("coordinates", data)
	if err != nil {
		log.Fatalf("Error publishing to NATS: %v", err)
	}

	log.Println("Mensaje enviado al tópico 'coordinates'")
}
