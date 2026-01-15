package widget

import (
	"sync"
	"time"
)

// CacheItem represents a cached item with expiration
type CacheItem struct {
	Data      *WidgetData
	ExpiresAt time.Time
}

// Cache provides thread-safe caching for widget data
type Cache struct {
	mu    sync.RWMutex
	items map[string]*CacheItem
}

// NewCache creates a new cache instance
func NewCache() *Cache {
	c := &Cache{
		items: make(map[string]*CacheItem),
	}

	// Start cleanup goroutine
	go c.cleanup()

	return c
}

// Get retrieves an item from the cache
func (c *Cache) Get(key string) (*WidgetData, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	item, ok := c.items[key]
	if !ok {
		return nil, false
	}

	// Check if expired
	if time.Now().After(item.ExpiresAt) {
		return nil, false
	}

	return item.Data, true
}

// Set stores an item in the cache with TTL
func (c *Cache) Set(key string, data *WidgetData, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.items[key] = &CacheItem{
		Data:      data,
		ExpiresAt: time.Now().Add(ttl),
	}
}

// Delete removes an item from the cache
func (c *Cache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.items, key)
}

// Clear removes all items from the cache
func (c *Cache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.items = make(map[string]*CacheItem)
}

// cleanup periodically removes expired items
func (c *Cache) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		c.removeExpired()
	}
}

// removeExpired removes all expired items from the cache
func (c *Cache) removeExpired() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	for key, item := range c.items {
		if now.After(item.ExpiresAt) {
			delete(c.items, key)
		}
	}
}

// Size returns the number of items in the cache
func (c *Cache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return len(c.items)
}

// Keys returns all keys in the cache
func (c *Cache) Keys() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	keys := make([]string, 0, len(c.items))
	for k := range c.items {
		keys = append(keys, k)
	}
	return keys
}
