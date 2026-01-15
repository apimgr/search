package cache

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisCache implements Cache interface using Valkey/Redis
// Per AI.md PART 18: Uses github.com/redis/go-redis/v9
type RedisCache struct {
	client  redis.UniversalClient
	prefix  string
	stats   Stats
}

// RedisConfig holds Redis/Valkey connection configuration
type RedisConfig struct {
	// Type: "redis" or "valkey" (same library, just for clarity)
	Type string

	// Connection URL (takes precedence over host/port)
	// Format: redis://user:password@host:port/db or valkey://...
	URL string

	// Individual connection settings
	Host     string
	Port     int
	Password string
	DB       int

	// Pool settings
	PoolSize int
	MinIdle  int
	Timeout  time.Duration

	// Key prefix
	Prefix string

	// Cluster mode
	Cluster      bool
	ClusterNodes []string
}

// NewRedisCache creates a new Redis/Valkey cache
func NewRedisCache(cfg *RedisConfig) (*RedisCache, error) {
	if cfg == nil {
		cfg = &RedisConfig{
			Host:     "localhost",
			Port:     6379,
			DB:       0,
			PoolSize: 10,
			MinIdle:  2,
			Timeout:  5 * time.Second,
			Prefix:   "apimgr:",
		}
	}

	var client redis.UniversalClient

	if cfg.Cluster && len(cfg.ClusterNodes) > 0 {
		// Cluster mode
		client = redis.NewClusterClient(&redis.ClusterOptions{
			Addrs:        cfg.ClusterNodes,
			Password:     cfg.Password,
			PoolSize:     cfg.PoolSize,
			MinIdleConns: cfg.MinIdle,
			DialTimeout:  cfg.Timeout,
			ReadTimeout:  cfg.Timeout,
			WriteTimeout: cfg.Timeout,
		})
	} else if cfg.URL != "" {
		// URL-based connection
		opts, err := redis.ParseURL(cfg.URL)
		if err != nil {
			return nil, fmt.Errorf("failed to parse Redis URL: %w", err)
		}
		if cfg.PoolSize > 0 {
			opts.PoolSize = cfg.PoolSize
		}
		if cfg.MinIdle > 0 {
			opts.MinIdleConns = cfg.MinIdle
		}
		client = redis.NewClient(opts)
	} else {
		// Host/port based connection
		addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
		client = redis.NewClient(&redis.Options{
			Addr:         addr,
			Password:     cfg.Password,
			DB:           cfg.DB,
			PoolSize:     cfg.PoolSize,
			MinIdleConns: cfg.MinIdle,
			DialTimeout:  cfg.Timeout,
			ReadTimeout:  cfg.Timeout,
			WriteTimeout: cfg.Timeout,
		})
	}

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	return &RedisCache{
		client: client,
		prefix: cfg.Prefix,
		stats:  Stats{Backend: cfg.Type, Connected: true},
	}, nil
}

// prefixKey adds the configured prefix to a key
func (c *RedisCache) prefixKey(key string) string {
	return c.prefix + key
}

// Get retrieves a value from Redis
func (c *RedisCache) Get(ctx context.Context, key string) ([]byte, error) {
	val, err := c.client.Get(ctx, c.prefixKey(key)).Bytes()
	if err != nil {
		if err == redis.Nil {
			c.stats.Misses++
			return nil, fmt.Errorf("key not found: %s", key)
		}
		c.stats.Misses++
		return nil, err
	}
	c.stats.Hits++
	return val, nil
}

// Set stores a value in Redis with TTL
func (c *RedisCache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	return c.client.Set(ctx, c.prefixKey(key), value, ttl).Err()
}

// Delete removes a value from Redis
func (c *RedisCache) Delete(ctx context.Context, key string) error {
	return c.client.Del(ctx, c.prefixKey(key)).Err()
}

// Exists checks if a key exists in Redis
func (c *RedisCache) Exists(ctx context.Context, key string) (bool, error) {
	count, err := c.client.Exists(ctx, c.prefixKey(key)).Result()
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// Clear removes all keys matching a pattern
func (c *RedisCache) Clear(ctx context.Context, pattern string) error {
	// Use SCAN for production-safe key iteration
	iter := c.client.Scan(ctx, 0, c.prefixKey(pattern), 100).Iterator()
	var keys []string

	for iter.Next(ctx) {
		keys = append(keys, iter.Val())
		// Delete in batches
		if len(keys) >= 100 {
			if err := c.client.Del(ctx, keys...).Err(); err != nil {
				return err
			}
			keys = keys[:0]
		}
	}

	if err := iter.Err(); err != nil {
		return err
	}

	// Delete remaining keys
	if len(keys) > 0 {
		return c.client.Del(ctx, keys...).Err()
	}

	return nil
}

// Close closes the Redis connection
func (c *RedisCache) Close() error {
	c.stats.Connected = false
	return c.client.Close()
}

// Ping checks Redis connectivity
func (c *RedisCache) Ping(ctx context.Context) error {
	return c.client.Ping(ctx).Err()
}

// Stats returns cache statistics
func (c *RedisCache) Stats(ctx context.Context) (*Stats, error) {
	stats := c.stats // Copy base stats

	// Get key count
	dbSize, err := c.client.DBSize(ctx).Result()
	if err == nil {
		stats.Keys = dbSize
	}

	// Get memory usage from INFO
	info, err := c.client.Info(ctx, "memory").Result()
	if err == nil {
		// Parse used_memory from INFO output
		// Format: "used_memory:12345\r\n"
		var memUsed int64
		fmt.Sscanf(info, "# Memory\r\nused_memory:%d", &memUsed)
		if memUsed > 0 {
			stats.MemoryUsed = memUsed
		}
	}

	return &stats, nil
}

// SetNX sets a key only if it doesn't exist (for distributed locking)
func (c *RedisCache) SetNX(ctx context.Context, key string, value []byte, ttl time.Duration) (bool, error) {
	return c.client.SetNX(ctx, c.prefixKey(key), value, ttl).Result()
}

// Publish publishes a message to a channel (for pub/sub)
func (c *RedisCache) Publish(ctx context.Context, channel string, message []byte) error {
	return c.client.Publish(ctx, c.prefix+channel, message).Err()
}

// Subscribe subscribes to a channel
func (c *RedisCache) Subscribe(ctx context.Context, channel string) *redis.PubSub {
	return c.client.Subscribe(ctx, c.prefix+channel)
}

// Incr increments a key
func (c *RedisCache) Incr(ctx context.Context, key string) (int64, error) {
	return c.client.Incr(ctx, c.prefixKey(key)).Result()
}

// Expire sets TTL on an existing key
func (c *RedisCache) Expire(ctx context.Context, key string, ttl time.Duration) error {
	return c.client.Expire(ctx, c.prefixKey(key), ttl).Err()
}

// HSet sets a hash field
func (c *RedisCache) HSet(ctx context.Context, key, field string, value []byte) error {
	return c.client.HSet(ctx, c.prefixKey(key), field, value).Err()
}

// HGet gets a hash field
func (c *RedisCache) HGet(ctx context.Context, key, field string) ([]byte, error) {
	return c.client.HGet(ctx, c.prefixKey(key), field).Bytes()
}

// HGetAll gets all fields in a hash
func (c *RedisCache) HGetAll(ctx context.Context, key string) (map[string]string, error) {
	return c.client.HGetAll(ctx, c.prefixKey(key)).Result()
}
