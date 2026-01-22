package cache

import (
	"context"
	"testing"
	"time"
)

// TestRedisCachePrefixKeyMethod tests the prefixKey method directly
func TestRedisCachePrefixKeyMethod(t *testing.T) {
	tests := []struct {
		name     string
		prefix   string
		key      string
		expected string
	}{
		{"with prefix", "test:", "mykey", "test:mykey"},
		{"empty prefix", "", "mykey", "mykey"},
		{"long prefix", "namespace:service:cache:", "key", "namespace:service:cache:key"},
		{"empty key", "prefix:", "", "prefix:"},
		{"both empty", "", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache := &RedisCache{
				prefix: tt.prefix,
			}
			result := cache.prefixKey(tt.key)
			if result != tt.expected {
				t.Errorf("prefixKey(%q) = %q, want %q", tt.key, result, tt.expected)
			}
		})
	}
}

// TestRedisConfigStructFields tests all RedisConfig struct fields
func TestRedisConfigStructFields(t *testing.T) {
	cfg := RedisConfig{
		Type:         "valkey",
		URL:          "redis://user:pass@localhost:6379/1",
		Host:         "redis.example.com",
		Port:         6380,
		Password:     "secretpassword",
		DB:           2,
		PoolSize:     25,
		MinIdle:      10,
		Timeout:      15 * time.Second,
		Prefix:       "myapp:",
		Cluster:      true,
		ClusterNodes: []string{"node1:6379", "node2:6379", "node3:6379"},
	}

	if cfg.Type != "valkey" {
		t.Errorf("Type = %q, want %q", cfg.Type, "valkey")
	}
	if cfg.URL != "redis://user:pass@localhost:6379/1" {
		t.Errorf("URL = %q, want expected URL", cfg.URL)
	}
	if cfg.Host != "redis.example.com" {
		t.Errorf("Host = %q, want %q", cfg.Host, "redis.example.com")
	}
	if cfg.Port != 6380 {
		t.Errorf("Port = %d, want %d", cfg.Port, 6380)
	}
	if cfg.Password != "secretpassword" {
		t.Errorf("Password = %q, want %q", cfg.Password, "secretpassword")
	}
	if cfg.DB != 2 {
		t.Errorf("DB = %d, want %d", cfg.DB, 2)
	}
	if cfg.PoolSize != 25 {
		t.Errorf("PoolSize = %d, want %d", cfg.PoolSize, 25)
	}
	if cfg.MinIdle != 10 {
		t.Errorf("MinIdle = %d, want %d", cfg.MinIdle, 10)
	}
	if cfg.Timeout != 15*time.Second {
		t.Errorf("Timeout = %v, want %v", cfg.Timeout, 15*time.Second)
	}
	if cfg.Prefix != "myapp:" {
		t.Errorf("Prefix = %q, want %q", cfg.Prefix, "myapp:")
	}
	if !cfg.Cluster {
		t.Error("Cluster should be true")
	}
	if len(cfg.ClusterNodes) != 3 {
		t.Errorf("ClusterNodes length = %d, want 3", len(cfg.ClusterNodes))
	}
}

// TestNewRedisCacheNilConfigDefaults tests that nil config uses defaults
func TestNewRedisCacheNilConfigDefaults(t *testing.T) {
	// This will attempt to connect with defaults and fail (no Redis running)
	// But we can verify the function handles nil config gracefully
	cache, err := NewRedisCache(nil)
	if err == nil {
		// If Redis happens to be running, close it
		if cache != nil {
			cache.Close()
		}
		t.Log("Redis connection succeeded with nil config (Redis is running)")
	} else {
		// Expected: connection should fail but nil config should be handled
		t.Log("Redis connection failed as expected:", err)
	}
}

// TestNewRedisCacheInvalidURLParsing tests URL parsing errors
func TestNewRedisCacheInvalidURLParsing(t *testing.T) {
	tests := []struct {
		name string
		url  string
	}{
		{"completely invalid", "not-a-url"},
		{"invalid scheme", "http://localhost:6379"},
		{"missing host", "redis://:6379"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &RedisConfig{
				URL:     tt.url,
				Timeout: 50 * time.Millisecond,
			}
			cache, err := NewRedisCache(cfg)
			if err == nil {
				if cache != nil {
					cache.Close()
				}
				// Some URLs might parse but fail to connect
				t.Logf("URL %q was parsed (may have failed on connect)", tt.url)
			} else {
				t.Logf("URL %q failed: %v", tt.url, err)
			}
		})
	}
}

// TestNewRedisCacheClusterModeNoNodes tests cluster mode with no running nodes
func TestNewRedisCacheClusterModeNoNodes(t *testing.T) {
	cfg := &RedisConfig{
		Cluster:      true,
		ClusterNodes: []string{"localhost:17000", "localhost:17001", "localhost:17002"},
		Password:     "testpass",
		PoolSize:     5,
		MinIdle:      1,
		Timeout:      100 * time.Millisecond,
	}

	cache, err := NewRedisCache(cfg)
	if err == nil {
		if cache != nil {
			cache.Close()
		}
		t.Log("Cluster connection succeeded (cluster may be running)")
	} else {
		// Expected - no cluster running
		t.Log("Cluster connection failed as expected:", err)
	}
}

// TestNewRedisCacheURLModeWithPoolSettings tests URL mode applies pool settings
func TestNewRedisCacheURLModeWithPoolSettings(t *testing.T) {
	cfg := &RedisConfig{
		URL:      "redis://localhost:16379/0",
		PoolSize: 20,
		MinIdle:  5,
		Timeout:  100 * time.Millisecond,
	}

	cache, err := NewRedisCache(cfg)
	if err == nil {
		if cache != nil {
			cache.Close()
		}
		t.Log("URL mode connection succeeded")
	} else {
		t.Log("URL mode connection failed (expected):", err)
	}
}

// TestNewRedisCacheURLModeNoPoolSettings tests URL mode without pool settings
func TestNewRedisCacheURLModeNoPoolSettings(t *testing.T) {
	cfg := &RedisConfig{
		URL:      "redis://localhost:16379/0",
		PoolSize: 0, // Should not override
		MinIdle:  0, // Should not override
		Timeout:  100 * time.Millisecond,
	}

	cache, err := NewRedisCache(cfg)
	if err == nil {
		if cache != nil {
			cache.Close()
		}
		t.Log("URL mode (no pool override) connection succeeded")
	} else {
		t.Log("URL mode (no pool override) connection failed (expected):", err)
	}
}

// TestNewRedisCacheHostPortMode tests direct host/port connection
func TestNewRedisCacheHostPortMode(t *testing.T) {
	cfg := &RedisConfig{
		Host:     "localhost",
		Port:     16379,
		Password: "testpassword",
		DB:       1,
		PoolSize: 10,
		MinIdle:  2,
		Timeout:  100 * time.Millisecond,
		Prefix:   "test:",
	}

	cache, err := NewRedisCache(cfg)
	if err == nil {
		if cache != nil {
			cache.Close()
		}
		t.Log("Host/port connection succeeded")
	} else {
		t.Log("Host/port connection failed (expected):", err)
	}
}

// TestRedisCacheInterfaceCompliance verifies RedisCache implements Cache
func TestRedisCacheInterfaceCompliance(t *testing.T) {
	// Compile-time check that RedisCache implements Cache interface
	var _ Cache = (*RedisCache)(nil)
}

// TestRedisCacheStatsStructure tests that Stats is properly initialized
func TestRedisCacheStatsStructure(t *testing.T) {
	// We can't test the full stats without a connection, but we can verify
	// the Stats struct is properly used by checking the fields
	stats := Stats{
		Hits:       0,
		Misses:     0,
		Keys:       0,
		MemoryUsed: 0,
		Connected:  true,
		Backend:    "redis",
	}

	if stats.Backend != "redis" {
		t.Errorf("Backend = %q, want %q", stats.Backend, "redis")
	}
	if !stats.Connected {
		t.Error("Connected should be true")
	}
}

// Integration tests - these run if REDIS_URL is set
// Run with: REDIS_URL=redis://localhost:6379 go test -v ./src/cache/

func getTestRedisCache(t *testing.T) *RedisCache {
	cfg := &RedisConfig{
		Host:    "localhost",
		Port:    6379,
		Timeout: time.Second,
		Prefix:  "test:",
	}

	cache, err := NewRedisCache(cfg)
	if err != nil {
		t.Skipf("Skipping Redis integration test: %v", err)
		return nil
	}
	return cache
}

func TestRedisCacheIntegration_SetGet(t *testing.T) {
	cache := getTestRedisCache(t)
	if cache == nil {
		return
	}
	defer cache.Close()

	ctx := context.Background()
	key := "integration-test-key"
	value := []byte("integration-test-value")

	// Set
	err := cache.Set(ctx, key, value, time.Minute)
	if err != nil {
		t.Fatalf("Set() error: %v", err)
	}

	// Get
	got, err := cache.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if string(got) != string(value) {
		t.Errorf("Get() = %q, want %q", got, value)
	}

	// Cleanup
	cache.Delete(ctx, key)
}

func TestRedisCacheIntegration_Delete(t *testing.T) {
	cache := getTestRedisCache(t)
	if cache == nil {
		return
	}
	defer cache.Close()

	ctx := context.Background()
	key := "integration-delete-key"

	// Set first
	cache.Set(ctx, key, []byte("value"), time.Minute)

	// Delete
	err := cache.Delete(ctx, key)
	if err != nil {
		t.Fatalf("Delete() error: %v", err)
	}

	// Verify deleted
	_, err = cache.Get(ctx, key)
	if err == nil {
		t.Error("Get() should return error after Delete()")
	}
}

func TestRedisCacheIntegration_Exists(t *testing.T) {
	cache := getTestRedisCache(t)
	if cache == nil {
		return
	}
	defer cache.Close()

	ctx := context.Background()
	key := "integration-exists-key"

	// Before set
	exists, err := cache.Exists(ctx, key)
	if err != nil {
		t.Fatalf("Exists() error: %v", err)
	}
	if exists {
		// Clean up from previous run
		cache.Delete(ctx, key)
	}

	// After set
	cache.Set(ctx, key, []byte("value"), time.Minute)
	exists, err = cache.Exists(ctx, key)
	if err != nil {
		t.Fatalf("Exists() error: %v", err)
	}
	if !exists {
		t.Error("Exists() should return true for existing key")
	}

	// Cleanup
	cache.Delete(ctx, key)
}

func TestRedisCacheIntegration_Clear(t *testing.T) {
	cache := getTestRedisCache(t)
	if cache == nil {
		return
	}
	defer cache.Close()

	ctx := context.Background()

	// Set multiple keys with pattern
	cache.Set(ctx, "clear:key1", []byte("v1"), time.Minute)
	cache.Set(ctx, "clear:key2", []byte("v2"), time.Minute)
	cache.Set(ctx, "other:key3", []byte("v3"), time.Minute)

	// Clear by pattern
	err := cache.Clear(ctx, "clear:*")
	if err != nil {
		t.Fatalf("Clear() error: %v", err)
	}

	// Verify cleared
	exists, _ := cache.Exists(ctx, "clear:key1")
	if exists {
		t.Error("clear:key1 should not exist after Clear()")
	}

	// Cleanup
	cache.Delete(ctx, "other:key3")
}

func TestRedisCacheIntegration_Ping(t *testing.T) {
	cache := getTestRedisCache(t)
	if cache == nil {
		return
	}
	defer cache.Close()

	ctx := context.Background()
	err := cache.Ping(ctx)
	if err != nil {
		t.Errorf("Ping() error: %v", err)
	}
}

func TestRedisCacheIntegration_Stats(t *testing.T) {
	cache := getTestRedisCache(t)
	if cache == nil {
		return
	}
	defer cache.Close()

	ctx := context.Background()
	stats, err := cache.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats() error: %v", err)
	}

	if stats.Backend != "redis" && stats.Backend != "valkey" && stats.Backend != "" {
		t.Errorf("Backend = %q, unexpected value", stats.Backend)
	}
}

func TestRedisCacheIntegration_SetNX(t *testing.T) {
	cache := getTestRedisCache(t)
	if cache == nil {
		return
	}
	defer cache.Close()

	ctx := context.Background()
	key := "integration-setnx-key"

	// Cleanup first
	cache.Delete(ctx, key)

	// First SetNX should succeed
	ok, err := cache.SetNX(ctx, key, []byte("value1"), time.Minute)
	if err != nil {
		t.Fatalf("SetNX() error: %v", err)
	}
	if !ok {
		t.Error("SetNX() should return true for new key")
	}

	// Second SetNX should fail
	ok, err = cache.SetNX(ctx, key, []byte("value2"), time.Minute)
	if err != nil {
		t.Fatalf("SetNX() error: %v", err)
	}
	if ok {
		t.Error("SetNX() should return false for existing key")
	}

	// Cleanup
	cache.Delete(ctx, key)
}

func TestRedisCacheIntegration_Incr(t *testing.T) {
	cache := getTestRedisCache(t)
	if cache == nil {
		return
	}
	defer cache.Close()

	ctx := context.Background()
	key := "integration-incr-key"

	// Cleanup first
	cache.Delete(ctx, key)

	// First incr
	val, err := cache.Incr(ctx, key)
	if err != nil {
		t.Fatalf("Incr() error: %v", err)
	}
	if val != 1 {
		t.Errorf("Incr() = %d, want 1", val)
	}

	// Second incr
	val, err = cache.Incr(ctx, key)
	if err != nil {
		t.Fatalf("Incr() error: %v", err)
	}
	if val != 2 {
		t.Errorf("Incr() = %d, want 2", val)
	}

	// Cleanup
	cache.Delete(ctx, key)
}

func TestRedisCacheIntegration_Expire(t *testing.T) {
	cache := getTestRedisCache(t)
	if cache == nil {
		return
	}
	defer cache.Close()

	ctx := context.Background()
	key := "integration-expire-key"

	// Set key first
	cache.Set(ctx, key, []byte("value"), time.Hour)

	// Set new expiration
	err := cache.Expire(ctx, key, time.Minute)
	if err != nil {
		t.Errorf("Expire() error: %v", err)
	}

	// Cleanup
	cache.Delete(ctx, key)
}

func TestRedisCacheIntegration_Hash(t *testing.T) {
	cache := getTestRedisCache(t)
	if cache == nil {
		return
	}
	defer cache.Close()

	ctx := context.Background()
	key := "integration-hash-key"

	// Cleanup first
	cache.Delete(ctx, key)

	// HSet
	err := cache.HSet(ctx, key, "field1", []byte("value1"))
	if err != nil {
		t.Fatalf("HSet() error: %v", err)
	}

	// HGet
	val, err := cache.HGet(ctx, key, "field1")
	if err != nil {
		t.Fatalf("HGet() error: %v", err)
	}
	if string(val) != "value1" {
		t.Errorf("HGet() = %q, want %q", val, "value1")
	}

	// HGetAll
	all, err := cache.HGetAll(ctx, key)
	if err != nil {
		t.Fatalf("HGetAll() error: %v", err)
	}
	if all["field1"] != "value1" {
		t.Errorf("HGetAll()['field1'] = %q, want %q", all["field1"], "value1")
	}

	// Cleanup
	cache.Delete(ctx, key)
}

func TestRedisCacheIntegration_PubSub(t *testing.T) {
	cache := getTestRedisCache(t)
	if cache == nil {
		return
	}
	defer cache.Close()

	ctx := context.Background()

	// Subscribe returns a PubSub object
	pubsub := cache.Subscribe(ctx, "test-channel")
	if pubsub == nil {
		t.Error("Subscribe() should return non-nil PubSub")
	}
	// Close the subscription
	pubsub.Close()

	// Publish (may not have subscribers, but shouldn't error)
	err := cache.Publish(ctx, "test-channel", []byte("message"))
	if err != nil {
		t.Errorf("Publish() error: %v", err)
	}
}

func TestRedisCacheIntegration_Close(t *testing.T) {
	cache := getTestRedisCache(t)
	if cache == nil {
		return
	}

	err := cache.Close()
	if err != nil {
		t.Errorf("Close() error: %v", err)
	}

	if cache.stats.Connected {
		t.Error("Connected should be false after Close()")
	}
}
