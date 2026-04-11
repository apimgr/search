package search

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"time"

	"github.com/apimgr/search/src/cache"
	"github.com/apimgr/search/src/model"
)

// ResultCache wraps cache.Cache to store search results.
// It supports any backend: memory, Valkey, or Redis (PART 9).
// Cache keys follow the PART 9 naming convention: search:{hash}
type ResultCache struct {
	backend cache.Cache
	ttl     time.Duration
	hits    atomic.Int64
	misses  atomic.Int64
}

// NewResultCache creates a result cache backed by the given cache.Cache.
// If backend is nil a no-op cache is used (caching effectively disabled).
func NewResultCache(backend cache.Cache, ttl time.Duration) *ResultCache {
	if ttl <= 0 {
		ttl = 5 * time.Minute // PART 9: page cache default
	}
	return &ResultCache{backend: backend, ttl: ttl}
}

// Get retrieves cached results for a key.
func (c *ResultCache) Get(key string) *model.SearchResults {
	if c.backend == nil {
		c.misses.Add(1)
		return nil
	}

	data, err := c.backend.Get(context.Background(), cacheKey(key))
	if err != nil {
		c.misses.Add(1)
		return nil
	}

	var results model.SearchResults
	if err := json.Unmarshal(data, &results); err != nil {
		c.misses.Add(1)
		return nil
	}

	c.hits.Add(1)
	return &results
}

// Set stores results in the cache with the configured TTL.
func (c *ResultCache) Set(key string, results *model.SearchResults) {
	if c.backend == nil || results == nil {
		return
	}

	data, err := json.Marshal(results)
	if err != nil {
		return
	}

	_ = c.backend.Set(context.Background(), cacheKey(key), data, c.ttl)
}

// Delete removes an item from the cache.
func (c *ResultCache) Delete(key string) {
	if c.backend == nil {
		return
	}
	_ = c.backend.Delete(context.Background(), cacheKey(key))
}

// Clear removes all search result cache entries.
func (c *ResultCache) Clear() {
	if c.backend == nil {
		return
	}
	_ = c.backend.Clear(context.Background(), "search:*")
	c.hits.Store(0)
	c.misses.Store(0)
}

// CacheStats holds cache hit/miss statistics.
type CacheStats struct {
	Hits    int64   `json:"hits"`
	Misses  int64   `json:"misses"`
	HitRate float64 `json:"hit_rate"`
}

// Stats returns cache hit/miss statistics.
func (c *ResultCache) Stats() CacheStats {
	hits := c.hits.Load()
	misses := c.misses.Load()
	total := hits + misses
	hitRate := float64(0)
	if total > 0 {
		hitRate = float64(hits) / float64(total)
	}
	return CacheStats{Hits: hits, Misses: misses, HitRate: hitRate}
}

// cacheKey returns the PART 9-compliant key: search:{hash}
func cacheKey(hash string) string {
	return "search:" + hash
}
