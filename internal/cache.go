package internal

import (
	"sync"
	"github.com/120m4n/mongo_nats/model"
)

// CacheManager gestiona la cache geoespacial en memoria
// Thread-safe usando RWMutex

type CacheManager struct {
	mu    sync.RWMutex
	cache map[string]model.MongoLocation // UniqueId -> última ubicación
}

// NewCacheManager inicializa el cache
func NewCacheManager() *CacheManager {
	return &CacheManager{
		cache: make(map[string]model.MongoLocation),
	}
}

// Get obtiene la última ubicación para un UniqueId
func (cm *CacheManager) Get(uniqueId string) (model.MongoLocation, bool) {
	cm.mu.RLock()
	loc, ok := cm.cache[uniqueId]
	cm.mu.RUnlock()
	return loc, ok
}

// Set actualiza la ubicación para un UniqueId
func (cm *CacheManager) Set(uniqueId string, loc model.MongoLocation) {
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

// Opcional: método para limpiar todo el cache
func (cm *CacheManager) Clear() {
	cm.mu.Lock()
	cm.cache = make(map[string]model.MongoLocation)
	cm.mu.Unlock()
}
