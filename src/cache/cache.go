package cache

import (
	"context"
	"encoding/json"
	"time"
)

// Cache is the interface for cache implementations
type Cache interface {
	// Get retrieves a value from the cache
	Get(ctx context.Context, key string) ([]byte, error)
	// Set stores a value in the cache with TTL
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
	// Delete removes a value from the cache
	Delete(ctx context.Context, key string) error
	// Exists checks if a key exists
	Exists(ctx context.Context, key string) (bool, error)
	// Clear removes all keys matching a pattern
	Clear(ctx context.Context, pattern string) error
	// Close closes the cache connection
	Close() error
	// Ping checks cache connectivity
	Ping(ctx context.Context) error
	// Stats returns cache statistics
	Stats(ctx context.Context) (*Stats, error)
}

// Stats represents cache statistics
type Stats struct {
	Hits       int64  `json:"hits"`
	Misses     int64  `json:"misses"`
	Keys       int64  `json:"keys"`
	MemoryUsed int64  `json:"memory_used"`
	Connected  bool   `json:"connected"`
	Backend    string `json:"backend"`
}

// Config holds cache configuration
type Config struct {
	Enabled  bool   `yaml:"enabled"`
	Backend  string `yaml:"backend"`  // redis, memory
	Address  string `yaml:"address"`  // Redis address (host:port)
	Password string `yaml:"password"` // Redis password
	DB       int    `yaml:"db"`       // Redis database number
	TTL      int    `yaml:"ttl"`      // Default TTL in seconds
	MaxSize  int    `yaml:"max_size"` // Max items for memory cache
}

// DefaultConfig returns default cache configuration
func DefaultConfig() *Config {
	return &Config{
		Enabled: false,
		Backend: "memory",
		Address: "localhost:6379",
		DB:      0,
		TTL:     300, // 5 minutes
		MaxSize: 10000,
	}
}

// New creates a new cache based on configuration
func New(cfg *Config) (Cache, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	if !cfg.Enabled {
		return NewMemoryCache(cfg.MaxSize, time.Duration(cfg.TTL)*time.Second), nil
	}

	switch cfg.Backend {
	case "redis":
		return NewRedisCache(cfg)
	case "memory":
		return NewMemoryCache(cfg.MaxSize, time.Duration(cfg.TTL)*time.Second), nil
	default:
		return NewMemoryCache(cfg.MaxSize, time.Duration(cfg.TTL)*time.Second), nil
	}
}

// GetJSON retrieves and unmarshals a JSON value
func GetJSON(ctx context.Context, c Cache, key string, v interface{}) error {
	data, err := c.Get(ctx, key)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

// SetJSON marshals and stores a JSON value
func SetJSON(ctx context.Context, c Cache, key string, v interface{}, ttl time.Duration) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return c.Set(ctx, key, data, ttl)
}
