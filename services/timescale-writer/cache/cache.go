package cache

import (
	"math"
	"sync"
)

const earthRadiusMeters = 6_371_000.0

// Entry stores the last known position for a device.
type Entry struct {
	Lat float64
	Lon float64
}

// GeoCache is a thread-safe in-memory store of the last GPS position per device.
type GeoCache struct {
	mu    sync.RWMutex
	store map[string]Entry
}

// New creates a new GeoCache.
func New() *GeoCache {
	return &GeoCache{store: make(map[string]Entry)}
}

// Get returns the last known position for uniqueID and whether it exists.
func (c *GeoCache) Get(uniqueID string) (Entry, bool) {
	c.mu.RLock()
	e, ok := c.store[uniqueID]
	c.mu.RUnlock()
	return e, ok
}

// Set updates the last known position for uniqueID.
func (c *GeoCache) Set(uniqueID string, lat, lon float64) {
	c.mu.Lock()
	c.store[uniqueID] = Entry{Lat: lat, Lon: lon}
	c.mu.Unlock()
}

// HaversineDistance returns the great-circle distance in meters between two
// geographic points identified by (lat1, lon1) and (lat2, lon2).
func HaversineDistance(lat1, lon1, lat2, lon2 float64) float64 {
	dLat := (lat2 - lat1) * math.Pi / 180
	dLon := (lon2 - lon1) * math.Pi / 180
	lat1R := lat1 * math.Pi / 180
	lat2R := lat2 * math.Pi / 180

	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1R)*math.Cos(lat2R)*math.Sin(dLon/2)*math.Sin(dLon/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	return earthRadiusMeters * c
}
