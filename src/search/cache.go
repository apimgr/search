package search

import (
	"sync"
	"time"

	"github.com/apimgr/search/src/models"
)

// ResultCache provides an in-memory cache for search results
type ResultCache struct {
	mu       sync.RWMutex
	items    map[string]*cacheItem
	maxSize  int
	ttl      time.Duration
	hits     int64
	misses   int64
}

type cacheItem struct {
	results   *models.SearchResults
	expiresAt time.Time
}

// NewResultCache creates a new result cache
func NewResultCache(maxSize int, ttl time.Duration) *ResultCache {
	if maxSize <= 0 {
		maxSize = 1000
	}
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}

	cache := &ResultCache{
		items:   make(map[string]*cacheItem),
		maxSize: maxSize,
		ttl:     ttl,
	}

	// Start cleanup goroutine
	go cache.cleanup()

	return cache
}

// Get retrieves cached results for a key
func (c *ResultCache) Get(key string) *models.SearchResults {
	c.mu.RLock()
	defer c.mu.RUnlock()

	item, exists := c.items[key]
	if !exists {
		c.misses++
		return nil
	}

	if time.Now().After(item.expiresAt) {
		c.misses++
		return nil
	}

	c.hits++

	// Return a copy to prevent modification
	result := *item.results
	resultsCopy := make([]models.Result, len(item.results.Results))
	copy(resultsCopy, item.results.Results)
	result.Results = resultsCopy

	return &result
}

// Set stores results in the cache
func (c *ResultCache) Set(key string, results *models.SearchResults) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Evict oldest if at capacity
	if len(c.items) >= c.maxSize {
		c.evictOldest()
	}

	// Make a copy of results
	resultsCopy := *results
	itemsCopy := make([]models.Result, len(results.Results))
	copy(itemsCopy, results.Results)
	resultsCopy.Results = itemsCopy

	c.items[key] = &cacheItem{
		results:   &resultsCopy,
		expiresAt: time.Now().Add(c.ttl),
	}
}

// Delete removes an item from the cache
func (c *ResultCache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.items, key)
}

// Clear removes all items from the cache
func (c *ResultCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items = make(map[string]*cacheItem)
	c.hits = 0
	c.misses = 0
}

// Size returns the current number of cached items
func (c *ResultCache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.items)
}

// Stats returns cache statistics
type CacheStats struct {
	Size    int     `json:"size"`
	MaxSize int     `json:"max_size"`
	Hits    int64   `json:"hits"`
	Misses  int64   `json:"misses"`
	HitRate float64 `json:"hit_rate"`
}

func (c *ResultCache) Stats() CacheStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	total := c.hits + c.misses
	hitRate := float64(0)
	if total > 0 {
		hitRate = float64(c.hits) / float64(total)
	}

	return CacheStats{
		Size:    len(c.items),
		MaxSize: c.maxSize,
		Hits:    c.hits,
		Misses:  c.misses,
		HitRate: hitRate,
	}
}

// evictOldest removes the oldest item (must be called with lock held)
func (c *ResultCache) evictOldest() {
	var oldestKey string
	var oldestTime time.Time

	for key, item := range c.items {
		if oldestKey == "" || item.expiresAt.Before(oldestTime) {
			oldestKey = key
			oldestTime = item.expiresAt
		}
	}

	if oldestKey != "" {
		delete(c.items, oldestKey)
	}
}

// cleanup periodically removes expired items
func (c *ResultCache) cleanup() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		c.removeExpired()
	}
}

// removeExpired removes all expired items
func (c *ResultCache) removeExpired() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	for key, item := range c.items {
		if now.After(item.expiresAt) {
			delete(c.items, key)
		}
	}
}
