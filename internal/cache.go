package internal

import (
	"sync"
	"github.com/120m4n/mongo_nats/model"
	"github.com/120m4n/mongo_nats/pkg/models"
)

// CacheManager gestiona la cache geoespacial en memoria
// Thread-safe usando RWMutex
// Compatible con ambos tipos de Location (model.MongoLocation y models.MongoLocation)

type CacheManager struct {
	mu    sync.RWMutex
	cache map[string]interface{} // UniqueId -> última ubicación (puede ser model.MongoLocation o models.MongoLocation)
}

// NewCacheManager inicializa el cache
func NewCacheManager() *CacheManager {
	return &CacheManager{
		cache: make(map[string]interface{}),
	}
}

// Get obtiene la última ubicación para un UniqueId (para MongoDB)
func (cm *CacheManager) Get(uniqueId string) (model.MongoLocation, bool) {
	cm.mu.RLock()
	val, ok := cm.cache[uniqueId]
	cm.mu.RUnlock()
	
	if !ok {
		return model.MongoLocation{}, false
	}
	
	// Try to convert to model.MongoLocation
	if loc, isModelLoc := val.(model.MongoLocation); isModelLoc {
		return loc, true
	}
	
	// Try to convert from models.MongoLocation
	if loc, isModelsLoc := val.(models.MongoLocation); isModelsLoc {
		return model.MongoLocation{
			Type:        loc.Type,
			Coordinates: loc.Coordinates,
		}, true
	}
	
	return model.MongoLocation{}, false
}

// Set actualiza la ubicación para un UniqueId (para MongoDB)
func (cm *CacheManager) Set(uniqueId string, loc model.MongoLocation) {
	cm.mu.Lock()
	cm.cache[uniqueId] = loc
	cm.mu.Unlock()
}

// SetModels actualiza la ubicación para un UniqueId (para TimescaleDB)
func (cm *CacheManager) SetModels(uniqueId string, loc models.MongoLocation) {
	cm.mu.Lock()
	cm.cache[uniqueId] = loc
	cm.mu.Unlock()
}

// Exists verifica si hay ubicación previa
func (cm *CacheManager) Exists(uniqueId string) bool {
	cm.mu.RLock()
	_, ok := cm.cache[uniqueId]
	cm.mu.RUnlock()
	return ok
}

// Delete elimina un UniqueId del cache
func (cm *CacheManager) Delete(uniqueId string) {
	cm.mu.Lock()
	delete(cm.cache, uniqueId)
	cm.mu.Unlock()
}

// Clear limpia todo el cache
func (cm *CacheManager) Clear() {
	cm.mu.Lock()
	cm.cache = make(map[string]interface{})
	cm.mu.Unlock()
}
