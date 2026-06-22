package cache

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
)

// startMiniredis starts a miniredis server for testing and returns the address + cleanup func
func startMiniredis(t *testing.T) (*miniredis.Miniredis, string) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}
	t.Cleanup(mr.Close)
	return mr, mr.Addr()
}

func TestNewRedisCacheWithMiniredisURL(t *testing.T) {
	_, addr := startMiniredis(t)

	cfg := &RedisConfig{
		URL:     "redis://" + addr + "/0",
		Prefix:  "test:",
		Timeout: 5 * time.Second,
	}

	c, err := NewRedisCache(cfg)
	if err != nil {
		t.Fatalf("NewRedisCache() error: %v", err)
	}
	defer c.Close()

	if c == nil {
		t.Fatal("NewRedisCache() returned nil")
	}
}

func TestNewRedisCacheWithMiniredisHostPort(t *testing.T) {
	mr, _ := startMiniredis(t)

	port, _ := strconv.Atoi(mr.Port())
	cfg := &RedisConfig{
		Host:     mr.Host(),
		Port:     port,
		DB:       0,
		PoolSize: 5,
		MinIdle:  1,
		Prefix:   "test:",
		Timeout:  5 * time.Second,
	}

	c, err := NewRedisCache(cfg)
	if err != nil {
		t.Fatalf("NewRedisCache() error: %v", err)
	}
	defer c.Close()

	if c == nil {
		t.Fatal("NewRedisCache() returned nil")
	}
}

func newTestRedisCache(t *testing.T) (*RedisCache, *miniredis.Miniredis) {
	t.Helper()
	mr, _ := startMiniredis(t)

	port, _ := strconv.Atoi(mr.Port())
	cfg := &RedisConfig{
		Host:    mr.Host(),
		Port:    port,
		Prefix:  "t:",
		Timeout: 5 * time.Second,
		Type:    "redis",
	}

	c, err := NewRedisCache(cfg)
	if err != nil {
		t.Fatalf("NewRedisCache() error: %v", err)
	}
	t.Cleanup(func() { c.Close() })

	return c, mr
}

func TestRedisCacheSetAndGet(t *testing.T) {
	c, _ := newTestRedisCache(t)
	ctx := context.Background()

	err := c.Set(ctx, "key1", []byte("value1"), time.Minute)
	if err != nil {
		t.Fatalf("Set() error: %v", err)
	}

	val, err := c.Get(ctx, "key1")
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if string(val) != "value1" {
		t.Errorf("Get() = %q, want %q", val, "value1")
	}

	// Verify stats
	if c.stats.Hits != 1 {
		t.Errorf("Hits = %d, want 1", c.stats.Hits)
	}
}

func TestRedisCacheGetNotFound(t *testing.T) {
	c, _ := newTestRedisCache(t)
	ctx := context.Background()

	_, err := c.Get(ctx, "nonexistent-key")
	if err == nil {
		t.Fatal("Get() should return error for nonexistent key")
	}

	// Miss should be counted
	if c.stats.Misses != 1 {
		t.Errorf("Misses = %d, want 1", c.stats.Misses)
	}
}

func TestRedisCacheDelete(t *testing.T) {
	c, _ := newTestRedisCache(t)
	ctx := context.Background()

	c.Set(ctx, "delete-key", []byte("value"), time.Minute)

	err := c.Delete(ctx, "delete-key")
	if err != nil {
		t.Fatalf("Delete() error: %v", err)
	}

	_, err = c.Get(ctx, "delete-key")
	if err == nil {
		t.Error("Get() should return error after Delete()")
	}
}

func TestRedisCacheExists(t *testing.T) {
	c, _ := newTestRedisCache(t)
	ctx := context.Background()

	// Key does not exist
	exists, err := c.Exists(ctx, "no-key")
	if err != nil {
		t.Fatalf("Exists() error: %v", err)
	}
	if exists {
		t.Error("Exists() should return false for nonexistent key")
	}

	// Set key
	c.Set(ctx, "yes-key", []byte("v"), time.Minute)

	// Key exists
	exists, err = c.Exists(ctx, "yes-key")
	if err != nil {
		t.Fatalf("Exists() error: %v", err)
	}
	if !exists {
		t.Error("Exists() should return true for existing key")
	}
}

func TestRedisCacheClear(t *testing.T) {
	c, _ := newTestRedisCache(t)
	ctx := context.Background()

	// Set multiple keys
	for i := 0; i < 5; i++ {
		c.Set(ctx, "prefix:key"+string(rune('0'+i)), []byte("v"), time.Minute)
	}
	c.Set(ctx, "other:key", []byte("v"), time.Minute)

	// Clear matching pattern
	err := c.Clear(ctx, "prefix:*")
	if err != nil {
		t.Fatalf("Clear() error: %v", err)
	}

	// Prefix keys should be gone
	exists, _ := c.Exists(ctx, "prefix:key0")
	if exists {
		t.Error("prefix:key0 should not exist after Clear(prefix:*)")
	}

	// Other key should remain
	exists, _ = c.Exists(ctx, "other:key")
	if !exists {
		t.Error("other:key should still exist after Clear(prefix:*)")
	}
}

func TestRedisCacheClearLargeBatch(t *testing.T) {
	c, _ := newTestRedisCache(t)
	ctx := context.Background()

	// Add more than 100 keys to test batch deletion
	for i := 0; i < 150; i++ {
		key := "batch:key" + string(rune('a'+i%26)) + string(rune('0'+i/26))
		c.Set(ctx, key, []byte("v"), time.Minute)
	}

	err := c.Clear(ctx, "batch:*")
	if err != nil {
		t.Fatalf("Clear() error: %v", err)
	}

	// Spot check a few keys are gone
	exists, _ := c.Exists(ctx, "batch:keya0")
	if exists {
		t.Error("batch:keya0 should not exist after Clear")
	}
}

func TestRedisCachePing(t *testing.T) {
	c, _ := newTestRedisCache(t)
	ctx := context.Background()

	err := c.Ping(ctx)
	if err != nil {
		t.Fatalf("Ping() error: %v", err)
	}
}

func TestRedisCacheStats(t *testing.T) {
	c, _ := newTestRedisCache(t)
	ctx := context.Background()

	c.Set(ctx, "stats-key", []byte("value"), time.Minute)

	stats, err := c.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats() error: %v", err)
	}

	if stats == nil {
		t.Fatal("Stats() returned nil")
	}

	if stats.Keys < 1 {
		t.Errorf("Keys = %d, want >= 1", stats.Keys)
	}
}

func TestRedisCacheClose(t *testing.T) {
	mr, _ := startMiniredis(t)

	port, _ := strconv.Atoi(mr.Port())
	cfg := &RedisConfig{
		Host:    mr.Host(),
		Port:    port,
		Prefix:  "t:",
		Timeout: 5 * time.Second,
		Type:    "redis",
	}

	c, err := NewRedisCache(cfg)
	if err != nil {
		t.Fatalf("NewRedisCache() error: %v", err)
	}

	err = c.Close()
	if err != nil {
		t.Fatalf("Close() error: %v", err)
	}

	if c.stats.Connected {
		t.Error("stats.Connected should be false after Close()")
	}
}

func TestRedisCacheSetNX(t *testing.T) {
	c, _ := newTestRedisCache(t)
	ctx := context.Background()

	// Set if not exists - should succeed
	ok, err := c.SetNX(ctx, "nx-key", []byte("first"), time.Minute)
	if err != nil {
		t.Fatalf("SetNX() error: %v", err)
	}
	if !ok {
		t.Error("SetNX() should return true for new key")
	}

	// Set if not exists - should fail (key already exists)
	ok, err = c.SetNX(ctx, "nx-key", []byte("second"), time.Minute)
	if err != nil {
		t.Fatalf("SetNX() error: %v", err)
	}
	if ok {
		t.Error("SetNX() should return false for existing key")
	}

	// Value should still be the first one
	val, _ := c.Get(ctx, "nx-key")
	if string(val) != "first" {
		t.Errorf("Value = %q, want %q", val, "first")
	}
}

func TestRedisCachePublish(t *testing.T) {
	c, _ := newTestRedisCache(t)
	ctx := context.Background()

	// Publish should not error even without subscribers
	err := c.Publish(ctx, "test-channel", []byte("message"))
	if err != nil {
		t.Fatalf("Publish() error: %v", err)
	}
}

func TestRedisCacheSubscribe(t *testing.T) {
	c, _ := newTestRedisCache(t)
	ctx := context.Background()

	// Subscribe returns a PubSub object - just verify no panic
	pubsub := c.Subscribe(ctx, "test-channel")
	if pubsub == nil {
		t.Fatal("Subscribe() returned nil")
	}
	pubsub.Close()
}

func TestRedisCacheIncr(t *testing.T) {
	c, _ := newTestRedisCache(t)
	ctx := context.Background()

	// First increment should return 1
	val, err := c.Incr(ctx, "counter")
	if err != nil {
		t.Fatalf("Incr() error: %v", err)
	}
	if val != 1 {
		t.Errorf("Incr() = %d, want 1", val)
	}

	// Second increment should return 2
	val, err = c.Incr(ctx, "counter")
	if err != nil {
		t.Fatalf("Incr() error: %v", err)
	}
	if val != 2 {
		t.Errorf("Incr() = %d, want 2", val)
	}
}

func TestRedisCacheExpire(t *testing.T) {
	c, _ := newTestRedisCache(t)
	ctx := context.Background()

	c.Set(ctx, "expire-key", []byte("value"), time.Hour)

	// Set a short expiry
	err := c.Expire(ctx, "expire-key", 10*time.Millisecond)
	if err != nil {
		t.Fatalf("Expire() error: %v", err)
	}

	// Key should exist initially
	exists, _ := c.Exists(ctx, "expire-key")
	if !exists {
		t.Error("Key should exist before expiry")
	}
}

func TestRedisCacheHSet(t *testing.T) {
	c, _ := newTestRedisCache(t)
	ctx := context.Background()

	err := c.HSet(ctx, "hash-key", "field1", []byte("value1"))
	if err != nil {
		t.Fatalf("HSet() error: %v", err)
	}

	err = c.HSet(ctx, "hash-key", "field2", []byte("value2"))
	if err != nil {
		t.Fatalf("HSet() error: %v", err)
	}
}

func TestRedisCacheHGet(t *testing.T) {
	c, _ := newTestRedisCache(t)
	ctx := context.Background()

	c.HSet(ctx, "hget-key", "name", []byte("alice"))

	val, err := c.HGet(ctx, "hget-key", "name")
	if err != nil {
		t.Fatalf("HGet() error: %v", err)
	}
	if string(val) != "alice" {
		t.Errorf("HGet() = %q, want %q", val, "alice")
	}
}

func TestRedisCacheHGetAll(t *testing.T) {
	c, _ := newTestRedisCache(t)
	ctx := context.Background()

	c.HSet(ctx, "hgetall-key", "f1", []byte("v1"))
	c.HSet(ctx, "hgetall-key", "f2", []byte("v2"))

	result, err := c.HGetAll(ctx, "hgetall-key")
	if err != nil {
		t.Fatalf("HGetAll() error: %v", err)
	}

	if len(result) != 2 {
		t.Errorf("HGetAll() len = %d, want 2", len(result))
	}
	if result["f1"] != "v1" {
		t.Errorf("HGetAll()[f1] = %q, want %q", result["f1"], "v1")
	}
	if result["f2"] != "v2" {
		t.Errorf("HGetAll()[f2] = %q, want %q", result["f2"], "v2")
	}
}

