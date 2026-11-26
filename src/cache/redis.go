package cache

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/apimgr/search/src/config"
)

// RedisCache implements Cache interface using Valkey/Redis
// Per AI.md PART 18: Uses github.com/redis/go-redis/v9
type RedisCache struct {
	client redis.UniversalClient
	prefix string
	stats  Stats
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
	TLS      bool
	TLSSkipVerify bool
	TLSCA, TLSCert, TLSKey string
	SSLDir string

	// Pool settings
	PoolSize int
	MinIdle  int
	Timeout  time.Duration

	// Key prefix
	Prefix string
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
			Prefix:   "search:",
		}
	}

	var client redis.UniversalClient

	var tlsEnabled bool
	var skipVerify = cfg.TLSSkipVerify
	if cfg.URL != "" {
		// URL-based connection
		connectionURL, enabled, skipOverride, err := parseRedisConnectionURL(cfg.URL)
		if err != nil {
			return nil, fmt.Errorf("failed to parse cache URL: %w", err)
		}
		tlsEnabled = enabled
		if skipOverride != nil {
			skipVerify = *skipOverride
		}
		opts, err := redis.ParseURL(connectionURL)
		if err != nil {
			return nil, fmt.Errorf("failed to parse cache URL: %w", err)
		}
		if cfg.PoolSize > 0 {
			opts.PoolSize = cfg.PoolSize
		}
		if cfg.MinIdle > 0 {
			opts.MinIdleConns = cfg.MinIdle
		}
		if !tlsEnabled {
			opts.TLSConfig = nil
		}
		if tlsEnabled {
			opts.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: skipVerify}
		}
		newClient, err := newRedisClient(opts, cfg)
		if err != nil {
			return nil, err
		}
		client = newClient
	} else {
		// Host/port based connection
		addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
		tlsEnabled = cfg.TLS || cfg.TLSCA != "" || cfg.TLSCert != "" || cfg.TLSKey != ""
		opts := &redis.Options{
			Addr:         addr,
			Password:     cfg.Password,
			DB:           cfg.DB,
			PoolSize:     cfg.PoolSize,
			MinIdleConns: cfg.MinIdle,
			DialTimeout:  cfg.Timeout,
			ReadTimeout:  cfg.Timeout,
			WriteTimeout: cfg.Timeout,
		}
		if tlsEnabled {
			opts.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: skipVerify}
		}
		newClient, err := newRedisClient(opts, cfg)
		if err != nil {
			return nil, err
		}
		client = newClient
	}

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to cache: %w", errors.Join(err, client.Close()))
	}

	return &RedisCache{
		client: client,
		prefix: cfg.Prefix,
		stats:  Stats{Backend: cfg.Type, Connected: true},
	}, nil
}

func parseRedisConnectionURL(raw string) (string, bool, *bool, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", false, nil, err
	}
	switch strings.ToLower(u.Scheme) {
	case "valkey":
		u.Scheme = "redis"
	case "valkeys":
		u.Scheme = "rediss"
	case "redis", "rediss":
	default:
		return "", false, nil, fmt.Errorf("unsupported cache URL scheme %q", u.Scheme)
	}
	enabled := u.Scheme == "rediss"
	query := u.Query()
	if values, exists := query["tls"]; exists {
		if len(values) != 1 {
			return "", false, nil, fmt.Errorf("cache URL tls option must appear once")
		}
		var parseErr error
		enabled, parseErr = config.ParseBool(values[0], enabled)
		if parseErr != nil {
			return "", false, nil, fmt.Errorf("invalid cache URL tls option: %w", parseErr)
		}
		query.Del("tls")
	}
	var skipOverride *bool
	if values, exists := query["skip_verify"]; exists {
		if len(values) != 1 {
			return "", false, nil, fmt.Errorf("cache URL skip_verify option must appear once")
		}
		value, parseErr := config.ParseBool(values[0], false)
		if parseErr != nil {
			return "", false, nil, fmt.Errorf("invalid cache URL skip_verify option: %w", parseErr)
		}
		skipOverride = &value
		query.Del("skip_verify")
	}
	u.RawQuery = query.Encode()
	return u.String(), enabled, skipOverride, nil
}

func newRedisClient(opts *redis.Options, cfg *RedisConfig) (redis.UniversalClient, error) {
	if opts.TLSConfig != nil {
		if cfg.TLSCA != "" {
			// A configured private CA is an explicit verify-full request.
			opts.TLSConfig.InsecureSkipVerify = false
			path, err := resolveSSLPath(cfg.SSLDir, cfg.TLSCA)
			if err != nil {
				return nil, err
			}
			pem, err := os.ReadFile(path)
			if err != nil {
				return nil, fmt.Errorf("read cache TLS CA file: %w", err)
			}
			pool, err := x509.SystemCertPool()
			if err != nil || pool == nil {
				pool = x509.NewCertPool()
			}
			if !pool.AppendCertsFromPEM(pem) {
				return nil, fmt.Errorf("cache TLS CA file contains no certificates")
			}
			opts.TLSConfig.RootCAs = pool
		}
		if cfg.TLSCert != "" || cfg.TLSKey != "" {
			if cfg.TLSCert == "" || cfg.TLSKey == "" {
				return nil, fmt.Errorf("cache TLS client certificate requires both tls_cert and tls_key")
			}
			certPath, err := resolveSSLPath(cfg.SSLDir, cfg.TLSCert)
			if err != nil {
				return nil, err
			}
			keyPath, err := resolveSSLPath(cfg.SSLDir, cfg.TLSKey)
			if err != nil {
				return nil, err
			}
			cert, err := tls.LoadX509KeyPair(certPath, keyPath)
			if err != nil {
				return nil, fmt.Errorf("load cache TLS client certificate: %w", err)
			}
			opts.TLSConfig.Certificates = []tls.Certificate{cert}
		}
	}
	return redis.NewClient(opts), nil
}

func resolveSSLPath(sslDir, value string) (string, error) {
	if filepath.IsAbs(value) {
		return value, nil
	}
	if strings.TrimSpace(sslDir) == "" {
		return "", fmt.Errorf("cache TLS file %q requires the config SSL directory", value)
	}
	clean := filepath.Clean(value)
	if clean == "." || clean == ".." || filepath.IsAbs(clean) || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("cache TLS file path %q must stay inside the config SSL directory", value)
	}
	return filepath.Join(sslDir, clean), nil
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
	// Copy base stats
	stats := c.stats

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
		if _, err := fmt.Sscanf(info, "# Memory\r\nused_memory:%d", &memUsed); err == nil && memUsed > 0 {
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
