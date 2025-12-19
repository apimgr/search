package cache

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// MemoryCache implements Cache interface using in-memory storage
type MemoryCache struct {
	mu         sync.RWMutex
	items      map[string]*cacheItem
	maxSize    int
	defaultTTL time.Duration
	stats      Stats
}

// cacheItem represents a cached item
type cacheItem struct {
	value     []byte
	expiresAt time.Time
}

// NewMemoryCache creates a new memory cache
func NewMemoryCache(maxSize int, defaultTTL time.Duration) *MemoryCache {
	if maxSize <= 0 {
		maxSize = 10000
	}
	if defaultTTL <= 0 {
		defaultTTL = 5 * time.Minute
	}

	c := &MemoryCache{
		items:      make(map[string]*cacheItem),
		maxSize:    maxSize,
		defaultTTL: defaultTTL,
		stats: Stats{
			Backend:   "memory",
			Connected: true,
		},
	}

	// Start cleanup goroutine
	go c.cleanup()

	return c
}

// cleanup periodically removes expired items
func (c *MemoryCache) cleanup() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		c.mu.Lock()
		now := time.Now()
		for key, item := range c.items {
			if now.After(item.expiresAt) {
				delete(c.items, key)
			}
		}
		c.stats.Keys = int64(len(c.items))
		c.mu.Unlock()
	}
}

// Get retrieves a value from the cache
func (c *MemoryCache) Get(ctx context.Context, key string) ([]byte, error) {
	c.mu.RLock()
	item, exists := c.items[key]
	c.mu.RUnlock()

	if !exists {
		c.mu.Lock()
		c.stats.Misses++
		c.mu.Unlock()
		return nil, fmt.Errorf("key not found: %s", key)
	}

	if time.Now().After(item.expiresAt) {
		c.mu.Lock()
		delete(c.items, key)
		c.stats.Misses++
		c.stats.Keys = int64(len(c.items))
		c.mu.Unlock()
		return nil, fmt.Errorf("key expired: %s", key)
	}

	c.mu.Lock()
	c.stats.Hits++
	c.mu.Unlock()

	return item.value, nil
}

// Set stores a value in the cache with TTL
func (c *MemoryCache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	if ttl <= 0 {
		ttl = c.defaultTTL
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Evict oldest items if at capacity
	if len(c.items) >= c.maxSize {
		c.evictOldest()
	}

	c.items[key] = &cacheItem{
		value:     value,
		expiresAt: time.Now().Add(ttl),
	}
	c.stats.Keys = int64(len(c.items))

	return nil
}

// evictOldest removes the oldest items to make room
func (c *MemoryCache) evictOldest() {
	// Remove 10% of items or at least 1
	toRemove := c.maxSize / 10
	if toRemove < 1 {
		toRemove = 1
	}

	// Find and remove oldest items
	type keyExpiry struct {
		key       string
		expiresAt time.Time
	}
	oldest := make([]keyExpiry, 0, toRemove)

	for key, item := range c.items {
		oldest = append(oldest, keyExpiry{key, item.expiresAt})
		if len(oldest) > toRemove {
			// Keep only the oldest ones
			maxIdx := 0
			for i := 1; i < len(oldest); i++ {
				if oldest[i].expiresAt.After(oldest[maxIdx].expiresAt) {
					maxIdx = i
				}
			}
			oldest = append(oldest[:maxIdx], oldest[maxIdx+1:]...)
		}
	}

	for _, ke := range oldest {
		delete(c.items, ke.key)
	}
}

// Delete removes a value from the cache
func (c *MemoryCache) Delete(ctx context.Context, key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.items, key)
	c.stats.Keys = int64(len(c.items))

	return nil
}

// Exists checks if a key exists in the cache
func (c *MemoryCache) Exists(ctx context.Context, key string) (bool, error) {
	c.mu.RLock()
	item, exists := c.items[key]
	c.mu.RUnlock()

	if !exists {
		return false, nil
	}

	if time.Now().After(item.expiresAt) {
		c.mu.Lock()
		delete(c.items, key)
		c.stats.Keys = int64(len(c.items))
		c.mu.Unlock()
		return false, nil
	}

	return true, nil
}

// Clear removes all keys matching a pattern
func (c *MemoryCache) Clear(ctx context.Context, pattern string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if pattern == "*" {
		c.items = make(map[string]*cacheItem)
		c.stats.Keys = 0
		return nil
	}

	// Simple pattern matching (prefix only for simplicity)
	prefix := ""
	suffix := ""
	if len(pattern) > 0 {
		if pattern[len(pattern)-1] == '*' {
			prefix = pattern[:len(pattern)-1]
		} else if pattern[0] == '*' {
			suffix = pattern[1:]
		}
	}

	for key := range c.items {
		match := false
		if prefix != "" && len(key) >= len(prefix) && key[:len(prefix)] == prefix {
			match = true
		}
		if suffix != "" && len(key) >= len(suffix) && key[len(key)-len(suffix):] == suffix {
			match = true
		}
		if match {
			delete(c.items, key)
		}
	}
	c.stats.Keys = int64(len(c.items))

	return nil
}

// Close closes the cache (no-op for memory cache)
func (c *MemoryCache) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.items = make(map[string]*cacheItem)
	c.stats.Keys = 0
	c.stats.Connected = false

	return nil
}

// Ping checks cache connectivity (always succeeds for memory cache)
func (c *MemoryCache) Ping(ctx context.Context) error {
	return nil
}

// Stats returns cache statistics
func (c *MemoryCache) Stats(ctx context.Context) (*Stats, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	stats := c.stats
	stats.Keys = int64(len(c.items))

	// Calculate approximate memory usage
	var memUsed int64
	for _, item := range c.items {
		memUsed += int64(len(item.value) + 16) // value + overhead
	}
	stats.MemoryUsed = memUsed

	return &stats, nil
}
