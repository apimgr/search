package cache

import (
	"context"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg == nil {
		t.Fatal("DefaultConfig() returned nil")
	}
	if cfg.Type != "memory" {
		t.Errorf("Type = %q, want %q", cfg.Type, "memory")
	}
	if cfg.Host != "localhost" {
		t.Errorf("Host = %q, want %q", cfg.Host, "localhost")
	}
	if cfg.Port != 6379 {
		t.Errorf("Port = %d, want %d", cfg.Port, 6379)
	}
	if cfg.DB != 0 {
		t.Errorf("DB = %d, want %d", cfg.DB, 0)
	}
	if cfg.PoolSize != 10 {
		t.Errorf("PoolSize = %d, want %d", cfg.PoolSize, 10)
	}
	if cfg.TTL != 3600 {
		t.Errorf("TTL = %d, want %d", cfg.TTL, 3600)
	}
	if cfg.MaxSize != 10000 {
		t.Errorf("MaxSize = %d, want %d", cfg.MaxSize, 10000)
	}
	if cfg.Prefix != "apimgr:" {
		t.Errorf("Prefix = %q, want %q", cfg.Prefix, "apimgr:")
	}
}

func TestNewMemoryCache(t *testing.T) {
	tests := []struct {
		name    string
		maxSize int
		ttl     time.Duration
	}{
		{"default", 0, 0},
		{"custom size", 100, 0},
		{"custom ttl", 0, time.Minute},
		{"custom both", 500, 10 * time.Minute},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewMemoryCache(tt.maxSize, tt.ttl)
			if c == nil {
				t.Fatal("NewMemoryCache returned nil")
			}
			defer c.Close()
		})
	}
}

func TestMemoryCacheSetGet(t *testing.T) {
	c := NewMemoryCache(100, time.Minute)
	defer c.Close()

	ctx := context.Background()
	key := "test-key"
	value := []byte("test-value")

	// Set
	err := c.Set(ctx, key, value, time.Minute)
	if err != nil {
		t.Fatalf("Set() error: %v", err)
	}

	// Get
	got, err := c.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if string(got) != string(value) {
		t.Errorf("Get() = %q, want %q", got, value)
	}
}

func TestMemoryCacheGetNotFound(t *testing.T) {
	c := NewMemoryCache(100, time.Minute)
	defer c.Close()

	ctx := context.Background()
	_, err := c.Get(ctx, "nonexistent")
	if err == nil {
		t.Error("Get() should return error for nonexistent key")
	}
}

func TestMemoryCacheDelete(t *testing.T) {
	c := NewMemoryCache(100, time.Minute)
	defer c.Close()

	ctx := context.Background()
	key := "delete-key"
	value := []byte("value")

	c.Set(ctx, key, value, time.Minute)

	err := c.Delete(ctx, key)
	if err != nil {
		t.Fatalf("Delete() error: %v", err)
	}

	_, err = c.Get(ctx, key)
	if err == nil {
		t.Error("Get() should return error after Delete()")
	}
}

func TestMemoryCacheExists(t *testing.T) {
	c := NewMemoryCache(100, time.Minute)
	defer c.Close()

	ctx := context.Background()
	key := "exists-key"

	// Before set
	exists, err := c.Exists(ctx, key)
	if err != nil {
		t.Fatalf("Exists() error: %v", err)
	}
	if exists {
		t.Error("Exists() should return false for nonexistent key")
	}

	// After set
	c.Set(ctx, key, []byte("value"), time.Minute)
	exists, err = c.Exists(ctx, key)
	if err != nil {
		t.Fatalf("Exists() error: %v", err)
	}
	if !exists {
		t.Error("Exists() should return true for existing key")
	}
}

func TestMemoryCacheClear(t *testing.T) {
	c := NewMemoryCache(100, time.Minute)
	defer c.Close()

	ctx := context.Background()

	// Add some keys
	c.Set(ctx, "key1", []byte("v1"), time.Minute)
	c.Set(ctx, "key2", []byte("v2"), time.Minute)
	c.Set(ctx, "other", []byte("v3"), time.Minute)

	// Clear all
	err := c.Clear(ctx, "*")
	if err != nil {
		t.Fatalf("Clear(*) error: %v", err)
	}

	exists, _ := c.Exists(ctx, "key1")
	if exists {
		t.Error("key1 should not exist after Clear(*)")
	}
}

func TestMemoryCacheClearPattern(t *testing.T) {
	c := NewMemoryCache(100, time.Minute)
	defer c.Close()

	ctx := context.Background()

	// Add some keys
	c.Set(ctx, "prefix:key1", []byte("v1"), time.Minute)
	c.Set(ctx, "prefix:key2", []byte("v2"), time.Minute)
	c.Set(ctx, "other:key3", []byte("v3"), time.Minute)

	// Clear by prefix
	err := c.Clear(ctx, "prefix:*")
	if err != nil {
		t.Fatalf("Clear(prefix:*) error: %v", err)
	}

	// Prefix keys should be gone
	exists, _ := c.Exists(ctx, "prefix:key1")
	if exists {
		t.Error("prefix:key1 should not exist after Clear(prefix:*)")
	}

	// Other key should remain
	exists, _ = c.Exists(ctx, "other:key3")
	if !exists {
		t.Error("other:key3 should still exist after Clear(prefix:*)")
	}
}

func TestMemoryCachePing(t *testing.T) {
	c := NewMemoryCache(100, time.Minute)
	defer c.Close()

	ctx := context.Background()
	err := c.Ping(ctx)
	if err != nil {
		t.Errorf("Ping() error: %v", err)
	}
}

func TestMemoryCacheStats(t *testing.T) {
	c := NewMemoryCache(100, time.Minute)
	defer c.Close()

	ctx := context.Background()

	// Add some keys
	c.Set(ctx, "key1", []byte("value1"), time.Minute)
	c.Set(ctx, "key2", []byte("value2"), time.Minute)

	// Get one key (hit)
	c.Get(ctx, "key1")

	// Get nonexistent key (miss)
	c.Get(ctx, "nonexistent")

	stats, err := c.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats() error: %v", err)
	}

	if stats.Backend != "memory" {
		t.Errorf("Backend = %q, want %q", stats.Backend, "memory")
	}
	if !stats.Connected {
		t.Error("Connected should be true")
	}
	if stats.Keys != 2 {
		t.Errorf("Keys = %d, want %d", stats.Keys, 2)
	}
	if stats.Hits < 1 {
		t.Errorf("Hits = %d, should be >= 1", stats.Hits)
	}
	if stats.Misses < 1 {
		t.Errorf("Misses = %d, should be >= 1", stats.Misses)
	}
}

func TestMemoryCacheClose(t *testing.T) {
	c := NewMemoryCache(100, time.Minute)

	ctx := context.Background()
	c.Set(ctx, "key", []byte("value"), time.Minute)

	err := c.Close()
	if err != nil {
		t.Errorf("Close() error: %v", err)
	}

	stats, _ := c.Stats(ctx)
	if stats.Connected {
		t.Error("Connected should be false after Close()")
	}
}

func TestMemoryCacheExpiration(t *testing.T) {
	c := NewMemoryCache(100, time.Minute)
	defer c.Close()

	ctx := context.Background()
	key := "expire-key"

	// Set with very short TTL
	err := c.Set(ctx, key, []byte("value"), 10*time.Millisecond)
	if err != nil {
		t.Fatalf("Set() error: %v", err)
	}

	// Verify it exists
	exists, _ := c.Exists(ctx, key)
	if !exists {
		t.Error("Key should exist immediately after Set()")
	}

	// Wait for expiration
	time.Sleep(20 * time.Millisecond)

	// Should be expired now
	_, err = c.Get(ctx, key)
	if err == nil {
		t.Error("Get() should return error for expired key")
	}
}

func TestMemoryCacheEviction(t *testing.T) {
	// Create cache with small max size
	c := NewMemoryCache(5, time.Minute)
	defer c.Close()

	ctx := context.Background()

	// Fill beyond capacity
	for i := 0; i < 10; i++ {
		key := "key-" + string(rune('a'+i))
		c.Set(ctx, key, []byte("value"), time.Minute)
	}

	// Should have evicted some items
	stats, _ := c.Stats(ctx)
	if stats.Keys > 5 {
		t.Errorf("Keys = %d, should be <= 5 after eviction", stats.Keys)
	}
}

func TestNewCacheWithConfig(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *Config
		wantErr bool
	}{
		{
			name:    "nil config",
			cfg:     nil,
			wantErr: false,
		},
		{
			name:    "memory type",
			cfg:     &Config{Type: "memory", MaxSize: 100, TTL: 60},
			wantErr: false,
		},
		{
			name:    "empty type defaults to memory",
			cfg:     &Config{Type: ""},
			wantErr: false,
		},
		{
			name:    "none type",
			cfg:     &Config{Type: "none"},
			wantErr: false,
		},
		{
			name:    "unknown type defaults to memory",
			cfg:     &Config{Type: "unknown"},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache, err := New(tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("New() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if cache == nil && !tt.wantErr {
				t.Error("New() returned nil cache")
				return
			}
			if cache != nil {
				cache.Close()
			}
		})
	}
}

func TestGetJSON(t *testing.T) {
	c := NewMemoryCache(100, time.Minute)
	defer c.Close()

	ctx := context.Background()

	type TestStruct struct {
		Name  string `json:"name"`
		Value int    `json:"value"`
	}

	// Set JSON value
	original := TestStruct{Name: "test", Value: 42}
	err := SetJSON(ctx, c, "json-key", original, time.Minute)
	if err != nil {
		t.Fatalf("SetJSON() error: %v", err)
	}

	// Get JSON value
	var retrieved TestStruct
	err = GetJSON(ctx, c, "json-key", &retrieved)
	if err != nil {
		t.Fatalf("GetJSON() error: %v", err)
	}

	if retrieved.Name != original.Name {
		t.Errorf("Name = %q, want %q", retrieved.Name, original.Name)
	}
	if retrieved.Value != original.Value {
		t.Errorf("Value = %d, want %d", retrieved.Value, original.Value)
	}
}

func TestGetJSONNotFound(t *testing.T) {
	c := NewMemoryCache(100, time.Minute)
	defer c.Close()

	ctx := context.Background()

	var result map[string]interface{}
	err := GetJSON(ctx, c, "nonexistent", &result)
	if err == nil {
		t.Error("GetJSON() should return error for nonexistent key")
	}
}

func TestSetJSONInvalidValue(t *testing.T) {
	c := NewMemoryCache(100, time.Minute)
	defer c.Close()

	ctx := context.Background()

	// Channels cannot be marshaled to JSON
	ch := make(chan int)
	err := SetJSON(ctx, c, "invalid", ch, time.Minute)
	if err == nil {
		t.Error("SetJSON() should return error for invalid value")
	}
}

func TestStatsStruct(t *testing.T) {
	stats := Stats{
		Hits:       100,
		Misses:     10,
		Keys:       50,
		MemoryUsed: 1024,
		Connected:  true,
		Backend:    "memory",
	}

	if stats.Hits != 100 {
		t.Errorf("Hits = %d, want %d", stats.Hits, 100)
	}
	if stats.Backend != "memory" {
		t.Errorf("Backend = %q, want %q", stats.Backend, "memory")
	}
}

func TestConfigStruct(t *testing.T) {
	cfg := Config{
		Type:         "redis",
		URL:          "redis://localhost:6379",
		Host:         "localhost",
		Port:         6379,
		Password:     "secret",
		DB:           1,
		PoolSize:     20,
		MinIdle:      5,
		Timeout:      10 * time.Second,
		Prefix:       "test:",
		TTL:          7200,
		Cluster:      true,
		ClusterNodes: []string{"node1:6379", "node2:6379"},
		MaxSize:      5000,
	}

	if cfg.Type != "redis" {
		t.Errorf("Type = %q, want %q", cfg.Type, "redis")
	}
	if cfg.Cluster != true {
		t.Error("Cluster should be true")
	}
	if len(cfg.ClusterNodes) != 2 {
		t.Errorf("ClusterNodes length = %d, want %d", len(cfg.ClusterNodes), 2)
	}
}

func TestMemoryCacheDefaultTTL(t *testing.T) {
	c := NewMemoryCache(100, time.Minute)
	defer c.Close()

	ctx := context.Background()

	// Set with zero TTL (should use default)
	err := c.Set(ctx, "default-ttl", []byte("value"), 0)
	if err != nil {
		t.Fatalf("Set() error: %v", err)
	}

	// Should still exist (default TTL is 1 minute)
	exists, _ := c.Exists(ctx, "default-ttl")
	if !exists {
		t.Error("Key should exist with default TTL")
	}
}
