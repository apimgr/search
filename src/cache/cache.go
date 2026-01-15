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

// Config holds cache configuration per AI.md PART 18
type Config struct {
	// Type: none (disabled), memory (default), valkey, redis
	Type string `yaml:"type"`

	// Connection URL (takes precedence over host/port)
	URL string `yaml:"url"`

	// Individual connection settings
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	Password string `yaml:"password"`
	DB       int    `yaml:"db"`

	// Pool settings
	PoolSize int           `yaml:"pool_size"`
	MinIdle  int           `yaml:"min_idle"`
	Timeout  time.Duration `yaml:"timeout"`

	// Key prefix
	Prefix string `yaml:"prefix"`

	// Default TTL
	TTL int `yaml:"ttl"` // seconds

	// Cluster mode
	Cluster      bool     `yaml:"cluster"`
	ClusterNodes []string `yaml:"cluster_nodes"`

	// Memory cache specific
	MaxSize int `yaml:"max_size"` // Max items for memory cache
}

// DefaultConfig returns default cache configuration
func DefaultConfig() *Config {
	return &Config{
		Type:     "memory",
		Host:     "localhost",
		Port:     6379,
		DB:       0,
		PoolSize: 10,
		MinIdle:  2,
		Timeout:  5 * time.Second,
		Prefix:   "apimgr:",
		TTL:      3600, // 1 hour
		MaxSize:  10000,
	}
}

// New creates a new cache based on configuration
func New(cfg *Config) (Cache, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	ttl := time.Duration(cfg.TTL) * time.Second
	if ttl == 0 {
		ttl = time.Hour
	}

	switch cfg.Type {
	case "redis", "valkey":
		return NewRedisCache(&RedisConfig{
			Type:         cfg.Type,
			URL:          cfg.URL,
			Host:         cfg.Host,
			Port:         cfg.Port,
			Password:     cfg.Password,
			DB:           cfg.DB,
			PoolSize:     cfg.PoolSize,
			MinIdle:      cfg.MinIdle,
			Timeout:      cfg.Timeout,
			Prefix:       cfg.Prefix,
			Cluster:      cfg.Cluster,
			ClusterNodes: cfg.ClusterNodes,
		})
	case "none":
		// No-op cache that discards everything
		return NewMemoryCache(1, time.Millisecond), nil
	case "memory", "":
		return NewMemoryCache(cfg.MaxSize, ttl), nil
	default:
		return NewMemoryCache(cfg.MaxSize, ttl), nil
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
