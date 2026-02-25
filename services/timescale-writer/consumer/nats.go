package consumer

import (
	"log"
	"sync/atomic"

	"github.com/nats-io/nats.go"

	"github.com/120m4n/timescale-writer/cache"
)

// MetricsConsumer para tracking
type MetricsConsumer struct {
	MessagesReceived  int64
	MessagesDropped   int64
	ValidationErrors  int64
	MarshalErrors     int64
	FilteredByCache   int64
	NatsConnected     bool
	NatsConnectedMu   int32 // atomic
}

var MetricsData = &MetricsConsumer{}

// Handler procesa mensajes del topic NATS
type Handler struct {
	subject   string
	batch     chan *CoordinateRow
	metrics   *MetricsConsumer
	geoCache  *cache.GeoCache
	threshold float64 // metros
}

// NewHandler crea un nuevo handler de NATS
func NewHandler(subject string, batchChan chan *CoordinateRow, geoCache *cache.GeoCache, threshold float64) *Handler {
	return &Handler{
		subject:   subject,
		batch:     batchChan,
		metrics:   MetricsData,
		geoCache:  geoCache,
		threshold: threshold,
	}
}

// MessageHandler es el callback para NATS
func (h *Handler) MessageHandler() nats.MsgHandler {
	return func(m *nats.Msg) {
		atomic.AddInt64(&h.metrics.MessagesReceived, 1)

		// Deserializar JSON
		doc, err := UnmarshalDocument(m.Data)
		if err != nil {
			atomic.AddInt64(&h.metrics.MarshalErrors, 1)
			log.Printf("[NATS] Error unmarshalling message: %v", err)
			return
		}

		// Validar
		if err := doc.Validate(); err != nil {
			atomic.AddInt64(&h.metrics.ValidationErrors, 1)
			log.Printf("[NATS] Validation error for device %s: %v", doc.UniqueID, err)
			return
		}

		// Filtro de distancia: descartar posiciones estáticas
		newLat := doc.Location.Coordinates[1]
		newLon := doc.Location.Coordinates[0]

		if prev, ok := h.geoCache.Get(doc.UniqueID); ok {
			dist := cache.HaversineDistance(prev.Lat, prev.Lon, newLat, newLon)
			if dist <= h.threshold {
				atomic.AddInt64(&h.metrics.FilteredByCache, 1)
				return
			}
		}

		// Convertir a fila de BD
		row := DocumentToRow(doc)

		// Enviar al batch channel (no-blocking; si está lleno, descartar)
		// IMPORTANTE: actualizar cache solo si el mensaje fue aceptado en el canal,
		// para evitar drift entre la cache y las posiciones realmente persistidas.
		select {
		case h.batch <- row:
			h.geoCache.Set(doc.UniqueID, newLat, newLon)
		default:
			atomic.AddInt64(&h.metrics.MessagesDropped, 1)
			log.Printf("[NATS] Batch channel full, dropping message from device %s", doc.UniqueID)
		}
	}
}

// Subscribe se suscribe al topic NATS
func (h *Handler) Subscribe(nc *nats.Conn) (*nats.Subscription, error) {
	sub, err := nc.Subscribe(h.subject, h.MessageHandler())
	if err != nil {
		return nil, err
	}
	log.Printf("[NATS] Subscribed to subject '%s' (distance threshold: %.1f m)", h.subject, h.threshold)
	return sub, nil
}
