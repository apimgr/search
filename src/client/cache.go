// Package main provides CLI cache configuration and implementation
// Per AI.md PART 36: Cache configuration (lines 42756-42760)
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/spf13/viper"

	"github.com/apimgr/search/src/client/paths"
)

// CacheConfig holds cache configuration
// Per AI.md PART 36 lines 42756-42760
type CacheConfig struct {
	Enabled bool // Enable response caching (default: true)
	TTL     int  // Cache TTL in seconds (default: 300 = 5 minutes)
	MaxSize int  // Max cache size in MB (default: 100)
}

// GetCacheConfig returns cache configuration from viper
func GetCacheConfig() CacheConfig {
	return CacheConfig{
		Enabled: viper.GetBool("cache.enabled"),
		TTL:     viper.GetInt("cache.ttl"),
		MaxSize: viper.GetInt("cache.max_size"),
	}
}

// CacheEntry represents a cached item
type CacheEntry struct {
	Data      []byte    `json:"data"`
	ExpiresAt time.Time `json:"expires_at"`
}

// CLICache provides in-memory and file-based caching for CLI responses
// Per AI.md PART 36: Response caching for performance
type CLICache struct {
	mu        sync.RWMutex
	memory    map[string]CacheEntry
	ttl       time.Duration
	maxSize   int64 // bytes
	cacheDir  string
	enabled   bool
	totalSize int64
}

var (
	cliCache     *CLICache
	cliCacheOnce sync.Once
)

// InitCache initializes the CLI cache
func InitCache() error {
	cfg := GetCacheConfig()

	// Default TTL is 300 seconds (5 minutes)
	ttl := cfg.TTL
	if ttl == 0 {
		ttl = 300
	}

	// Default max size is 100 MB
	maxSize := cfg.MaxSize
	if maxSize == 0 {
		maxSize = 100
	}

	// Default enabled is true
	enabled := cfg.Enabled
	if !viper.IsSet("cache.enabled") {
		enabled = true
	}

	cliCacheOnce.Do(func() {
		cliCache = &CLICache{
			memory:   make(map[string]CacheEntry),
			ttl:      time.Duration(ttl) * time.Second,
			maxSize:  int64(maxSize) * 1024 * 1024, // MB to bytes
			cacheDir: paths.CacheDir(),
			enabled:  enabled,
		}
	})

	// Ensure cache directory exists
	if err := os.MkdirAll(paths.CacheDir(), 0700); err != nil {
		return err
	}

	return nil
}

// Cache returns the CLI cache instance
func Cache() *CLICache {
	if cliCache == nil {
		InitCache()
	}
	return cliCache
}

// Get retrieves a cached value
func (c *CLICache) Get(key string) ([]byte, bool) {
	if !c.enabled {
		return nil, false
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	// Check memory cache first
	if entry, ok := c.memory[key]; ok {
		if time.Now().Before(entry.ExpiresAt) {
			return entry.Data, true
		}
		// Expired - will be cleaned up later
	}

	// Check file cache
	return c.getFromFile(key)
}

// Set stores a value in cache
func (c *CLICache) Set(key string, data []byte) {
	if !c.enabled {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	entry := CacheEntry{
		Data:      data,
		ExpiresAt: time.Now().Add(c.ttl),
	}

	// Check if adding would exceed max size
	dataSize := int64(len(data))
	if c.totalSize+dataSize > c.maxSize {
		// Evict oldest entries
		c.evictOldest(dataSize)
	}

	c.memory[key] = entry
	c.totalSize += dataSize

	// Also persist to file for larger responses
	if len(data) > 1024 { // > 1KB
		c.saveToFile(key, entry)
	}
}

// Delete removes a cached value
func (c *CLICache) Delete(key string) {
	if !c.enabled {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if entry, ok := c.memory[key]; ok {
		c.totalSize -= int64(len(entry.Data))
		delete(c.memory, key)
	}

	// Remove from file cache
	c.deleteFile(key)
}

// Clear removes all cached values
func (c *CLICache) Clear() {
	if !c.enabled {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.memory = make(map[string]CacheEntry)
	c.totalSize = 0

	// Clear file cache
	os.RemoveAll(c.cacheDir)
	os.MkdirAll(c.cacheDir, 0700)
}

// Cleanup removes expired entries
func (c *CLICache) Cleanup() {
	if !c.enabled {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	for key, entry := range c.memory {
		if now.After(entry.ExpiresAt) {
			c.totalSize -= int64(len(entry.Data))
			delete(c.memory, key)
			c.deleteFile(key)
		}
	}
}

// evictOldest removes oldest entries to make room for new data
func (c *CLICache) evictOldest(needed int64) {
	// Simple eviction: remove oldest by expiration time
	for c.totalSize+needed > c.maxSize && len(c.memory) > 0 {
		var oldestKey string
		var oldestTime time.Time
		first := true

		for key, entry := range c.memory {
			if first || entry.ExpiresAt.Before(oldestTime) {
				oldestKey = key
				oldestTime = entry.ExpiresAt
				first = false
			}
		}

		if oldestKey != "" {
			entry := c.memory[oldestKey]
			c.totalSize -= int64(len(entry.Data))
			delete(c.memory, oldestKey)
			c.deleteFile(oldestKey)
		}
	}
}

// getFromFile retrieves cached data from file
func (c *CLICache) getFromFile(key string) ([]byte, bool) {
	filePath := c.cacheFilePath(key)
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, false
	}

	var entry CacheEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil, false
	}

	if time.Now().After(entry.ExpiresAt) {
		os.Remove(filePath)
		return nil, false
	}

	return entry.Data, true
}

// saveToFile persists cache entry to file
func (c *CLICache) saveToFile(key string, entry CacheEntry) {
	filePath := c.cacheFilePath(key)
	data, err := json.Marshal(entry)
	if err != nil {
		return
	}
	os.WriteFile(filePath, data, 0600)
}

// deleteFile removes a cache file
func (c *CLICache) deleteFile(key string) {
	os.Remove(c.cacheFilePath(key))
}

// cacheFilePath returns the file path for a cache key
func (c *CLICache) cacheFilePath(key string) string {
	// Use a hash of the key to avoid filesystem issues
	hash := simpleHash(key)
	return filepath.Join(c.cacheDir, hash+".cache")
}

// simpleHash creates a simple hash of a string
func simpleHash(s string) string {
	h := uint32(0)
	for _, c := range s {
		h = h*31 + uint32(c)
	}
	return fmt.Sprintf("%08x", h)
}
