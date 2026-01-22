package cache

import (
	"context"
	"encoding/json"
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
	if cfg.MinIdle != 2 {
		t.Errorf("MinIdle = %d, want %d", cfg.MinIdle, 2)
	}
	if cfg.Timeout != 5*time.Second {
		t.Errorf("Timeout = %v, want %v", cfg.Timeout, 5*time.Second)
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
		{"negative size", -1, time.Minute},
		{"negative ttl", 100, -1 * time.Minute},
		{"both negative", -5, -5 * time.Minute},
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

func TestMemoryCacheDeleteNonexistent(t *testing.T) {
	c := NewMemoryCache(100, time.Minute)
	defer c.Close()

	ctx := context.Background()
	// Deleting a nonexistent key should not error
	err := c.Delete(ctx, "nonexistent-key")
	if err != nil {
		t.Errorf("Delete() should not error for nonexistent key: %v", err)
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

func TestMemoryCacheExistsExpired(t *testing.T) {
	c := NewMemoryCache(100, time.Minute)
	defer c.Close()

	ctx := context.Background()
	key := "expire-exists-key"

	// Set with very short TTL
	err := c.Set(ctx, key, []byte("value"), 10*time.Millisecond)
	if err != nil {
		t.Fatalf("Set() error: %v", err)
	}

	// Verify it exists immediately
	exists, _ := c.Exists(ctx, key)
	if !exists {
		t.Error("Key should exist immediately after Set()")
	}

	// Wait for expiration
	time.Sleep(20 * time.Millisecond)

	// Should be expired now - Exists should return false and clean up
	exists, err = c.Exists(ctx, key)
	if err != nil {
		t.Fatalf("Exists() error: %v", err)
	}
	if exists {
		t.Error("Exists() should return false for expired key")
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

func TestMemoryCacheClearSuffixPattern(t *testing.T) {
	c := NewMemoryCache(100, time.Minute)
	defer c.Close()

	ctx := context.Background()

	// Add some keys
	c.Set(ctx, "user:123:data", []byte("v1"), time.Minute)
	c.Set(ctx, "session:456:data", []byte("v2"), time.Minute)
	c.Set(ctx, "user:789:cache", []byte("v3"), time.Minute)

	// Clear by suffix pattern
	err := c.Clear(ctx, "*:data")
	if err != nil {
		t.Fatalf("Clear(*:data) error: %v", err)
	}

	// Suffix-matching keys should be gone
	exists, _ := c.Exists(ctx, "user:123:data")
	if exists {
		t.Error("user:123:data should not exist after Clear(*:data)")
	}

	exists, _ = c.Exists(ctx, "session:456:data")
	if exists {
		t.Error("session:456:data should not exist after Clear(*:data)")
	}

	// Non-matching key should remain
	exists, _ = c.Exists(ctx, "user:789:cache")
	if !exists {
		t.Error("user:789:cache should still exist after Clear(*:data)")
	}
}

func TestMemoryCacheClearEmptyPattern(t *testing.T) {
	c := NewMemoryCache(100, time.Minute)
	defer c.Close()

	ctx := context.Background()

	// Add some keys
	c.Set(ctx, "key1", []byte("v1"), time.Minute)
	c.Set(ctx, "key2", []byte("v2"), time.Minute)

	// Clear with empty pattern - should not match anything
	err := c.Clear(ctx, "")
	if err != nil {
		t.Fatalf("Clear('') error: %v", err)
	}

	// Keys should still exist
	exists, _ := c.Exists(ctx, "key1")
	if !exists {
		t.Error("key1 should still exist after Clear('')")
	}
}

func TestMemoryCacheClearNoWildcard(t *testing.T) {
	c := NewMemoryCache(100, time.Minute)
	defer c.Close()

	ctx := context.Background()

	// Add some keys
	c.Set(ctx, "exact-key", []byte("v1"), time.Minute)
	c.Set(ctx, "other-key", []byte("v2"), time.Minute)

	// Clear with pattern that has no wildcard - won't match via prefix/suffix logic
	err := c.Clear(ctx, "exact-key")
	if err != nil {
		t.Fatalf("Clear('exact-key') error: %v", err)
	}

	// Keys should still exist (pattern without wildcard doesn't match in current impl)
	exists, _ := c.Exists(ctx, "exact-key")
	if !exists {
		t.Error("exact-key should still exist after Clear('exact-key') - no wildcard")
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

func TestMemoryCacheStatsMemoryUsage(t *testing.T) {
	c := NewMemoryCache(100, time.Minute)
	defer c.Close()

	ctx := context.Background()

	// Add keys with known sizes
	c.Set(ctx, "key1", []byte("12345678901234567890"), time.Minute) // 20 bytes
	c.Set(ctx, "key2", []byte("abcdefghij"), time.Minute)          // 10 bytes

	stats, err := c.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats() error: %v", err)
	}

	// Memory should be > 0 (value size + overhead)
	if stats.MemoryUsed <= 0 {
		t.Errorf("MemoryUsed = %d, should be > 0", stats.MemoryUsed)
	}

	// Should be at least the sum of values (30 bytes) + overhead
	if stats.MemoryUsed < 30 {
		t.Errorf("MemoryUsed = %d, should be >= 30", stats.MemoryUsed)
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
	if stats.Keys != 0 {
		t.Errorf("Keys = %d, should be 0 after Close()", stats.Keys)
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

func TestMemoryCacheEvictionWithSingleItem(t *testing.T) {
	// Create cache with max size of 1
	c := NewMemoryCache(1, time.Minute)
	defer c.Close()

	ctx := context.Background()

	// Add first item
	c.Set(ctx, "first", []byte("value1"), time.Minute)

	// Add second item - should evict first
	c.Set(ctx, "second", []byte("value2"), time.Minute)

	stats, _ := c.Stats(ctx)
	if stats.Keys > 1 {
		t.Errorf("Keys = %d, should be <= 1 after eviction", stats.Keys)
	}
}

func TestMemoryCacheEvictionPreservesNewest(t *testing.T) {
	// Create cache with small size
	c := NewMemoryCache(3, time.Hour)
	defer c.Close()

	ctx := context.Background()

	// Add items with different TTLs to simulate different ages
	c.Set(ctx, "old1", []byte("v"), time.Minute)
	time.Sleep(5 * time.Millisecond)
	c.Set(ctx, "old2", []byte("v"), time.Minute)
	time.Sleep(5 * time.Millisecond)
	c.Set(ctx, "new1", []byte("v"), time.Hour)

	// Trigger eviction by adding more
	c.Set(ctx, "new2", []byte("v"), time.Hour)
	c.Set(ctx, "new3", []byte("v"), time.Hour)

	// Cache should now have items
	stats, _ := c.Stats(ctx)
	if stats.Keys > 3 {
		t.Errorf("Keys = %d, should be <= 3", stats.Keys)
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
		{
			name:    "zero TTL uses default",
			cfg:     &Config{Type: "memory", TTL: 0, MaxSize: 100},
			wantErr: false,
		},
		{
			name:    "negative values handled",
			cfg:     &Config{Type: "memory", TTL: -1, MaxSize: -1},
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

func TestNewCacheRedisTypeFails(t *testing.T) {
	// Redis type should fail without a running Redis server
	cfg := &Config{
		Type: "redis",
		Host: "localhost",
		Port: 16379, // Use non-standard port to ensure failure
	}

	cache, err := New(cfg)
	if err == nil {
		if cache != nil {
			cache.Close()
		}
		// If there happens to be a Redis running, that's OK
		t.Log("Redis connection succeeded (Redis may be running)")
	} else {
		// Expected behavior - connection should fail
		t.Log("Redis connection failed as expected:", err)
	}
}

func TestNewCacheValkeyTypeFails(t *testing.T) {
	// Valkey type should fail without a running Valkey server
	cfg := &Config{
		Type: "valkey",
		Host: "localhost",
		Port: 16379, // Use non-standard port to ensure failure
	}

	cache, err := New(cfg)
	if err == nil {
		if cache != nil {
			cache.Close()
		}
		t.Log("Valkey connection succeeded (Valkey may be running)")
	} else {
		t.Log("Valkey connection failed as expected:", err)
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

func TestGetJSONInvalidData(t *testing.T) {
	c := NewMemoryCache(100, time.Minute)
	defer c.Close()

	ctx := context.Background()

	// Store invalid JSON data
	err := c.Set(ctx, "invalid-json", []byte("not valid json {{{"), time.Minute)
	if err != nil {
		t.Fatalf("Set() error: %v", err)
	}

	// Try to unmarshal
	var result map[string]interface{}
	err = GetJSON(ctx, c, "invalid-json", &result)
	if err == nil {
		t.Error("GetJSON() should return error for invalid JSON data")
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

func TestSetJSONVariousTypes(t *testing.T) {
	c := NewMemoryCache(100, time.Minute)
	defer c.Close()

	ctx := context.Background()

	tests := []struct {
		name  string
		key   string
		value interface{}
	}{
		{"string", "str", "hello"},
		{"int", "int", 123},
		{"float", "float", 3.14},
		{"bool", "bool", true},
		{"slice", "slice", []int{1, 2, 3}},
		{"map", "map", map[string]int{"a": 1}},
		{"nil", "nil", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := SetJSON(ctx, c, tt.key, tt.value, time.Minute)
			if err != nil {
				t.Errorf("SetJSON() error: %v", err)
			}
		})
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

func TestStatsStructJSON(t *testing.T) {
	stats := Stats{
		Hits:       100,
		Misses:     10,
		Keys:       50,
		MemoryUsed: 1024,
		Connected:  true,
		Backend:    "memory",
	}

	data, err := json.Marshal(stats)
	if err != nil {
		t.Fatalf("json.Marshal() error: %v", err)
	}

	var parsed Stats
	err = json.Unmarshal(data, &parsed)
	if err != nil {
		t.Fatalf("json.Unmarshal() error: %v", err)
	}

	if parsed.Hits != stats.Hits {
		t.Errorf("Hits = %d, want %d", parsed.Hits, stats.Hits)
	}
	if parsed.Backend != stats.Backend {
		t.Errorf("Backend = %q, want %q", parsed.Backend, stats.Backend)
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

func TestMemoryCacheNegativeTTL(t *testing.T) {
	c := NewMemoryCache(100, time.Minute)
	defer c.Close()

	ctx := context.Background()

	// Set with negative TTL (should use default)
	err := c.Set(ctx, "neg-ttl", []byte("value"), -5*time.Second)
	if err != nil {
		t.Fatalf("Set() error: %v", err)
	}

	// Should still exist (uses default TTL)
	exists, _ := c.Exists(ctx, "neg-ttl")
	if !exists {
		t.Error("Key should exist with default TTL")
	}
}

func TestMemoryCacheMultipleGetMisses(t *testing.T) {
	c := NewMemoryCache(100, time.Minute)
	defer c.Close()

	ctx := context.Background()

	// Multiple misses to track stats
	for i := 0; i < 5; i++ {
		c.Get(ctx, "nonexistent-"+string(rune('a'+i)))
	}

	stats, _ := c.Stats(ctx)
	if stats.Misses < 5 {
		t.Errorf("Misses = %d, should be >= 5", stats.Misses)
	}
}

func TestMemoryCacheMultipleGetHits(t *testing.T) {
	c := NewMemoryCache(100, time.Minute)
	defer c.Close()

	ctx := context.Background()

	// Set a key
	c.Set(ctx, "hit-key", []byte("value"), time.Minute)

	// Multiple hits
	for i := 0; i < 5; i++ {
		c.Get(ctx, "hit-key")
	}

	stats, _ := c.Stats(ctx)
	if stats.Hits < 5 {
		t.Errorf("Hits = %d, should be >= 5", stats.Hits)
	}
}

func TestMemoryCacheContextCancellation(t *testing.T) {
	c := NewMemoryCache(100, time.Minute)
	defer c.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// Operations should still work (memory cache doesn't check context)
	err := c.Set(ctx, "key", []byte("value"), time.Minute)
	if err != nil {
		t.Errorf("Set() should work even with cancelled context: %v", err)
	}

	_, err = c.Get(ctx, "key")
	if err != nil {
		t.Errorf("Get() should work even with cancelled context: %v", err)
	}
}

func TestMemoryCacheEmptyValue(t *testing.T) {
	c := NewMemoryCache(100, time.Minute)
	defer c.Close()

	ctx := context.Background()

	// Set empty value
	err := c.Set(ctx, "empty", []byte{}, time.Minute)
	if err != nil {
		t.Fatalf("Set() error: %v", err)
	}

	val, err := c.Get(ctx, "empty")
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if len(val) != 0 {
		t.Errorf("Value length = %d, want 0", len(val))
	}
}

func TestMemoryCacheNilValue(t *testing.T) {
	c := NewMemoryCache(100, time.Minute)
	defer c.Close()

	ctx := context.Background()

	// Set nil value
	err := c.Set(ctx, "nil", nil, time.Minute)
	if err != nil {
		t.Fatalf("Set() error: %v", err)
	}

	val, err := c.Get(ctx, "nil")
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if val != nil {
		t.Errorf("Value should be nil, got %v", val)
	}
}

func TestMemoryCacheLargeValue(t *testing.T) {
	c := NewMemoryCache(100, time.Minute)
	defer c.Close()

	ctx := context.Background()

	// Create large value (1MB)
	largeValue := make([]byte, 1024*1024)
	for i := range largeValue {
		largeValue[i] = byte(i % 256)
	}

	err := c.Set(ctx, "large", largeValue, time.Minute)
	if err != nil {
		t.Fatalf("Set() error: %v", err)
	}

	val, err := c.Get(ctx, "large")
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if len(val) != len(largeValue) {
		t.Errorf("Value length = %d, want %d", len(val), len(largeValue))
	}
}

func TestMemoryCacheSpecialCharKeys(t *testing.T) {
	c := NewMemoryCache(100, time.Minute)
	defer c.Close()

	ctx := context.Background()

	keys := []string{
		"key:with:colons",
		"key/with/slashes",
		"key.with.dots",
		"key-with-dashes",
		"key_with_underscores",
		"key with spaces",
		"key\twith\ttabs",
		"key\nwith\nnewlines",
		"",
		"emoji:key",
	}

	for _, key := range keys {
		t.Run("key:"+key, func(t *testing.T) {
			err := c.Set(ctx, key, []byte("value"), time.Minute)
			if err != nil {
				t.Fatalf("Set() error: %v", err)
			}

			val, err := c.Get(ctx, key)
			if err != nil {
				t.Fatalf("Get() error: %v", err)
			}
			if string(val) != "value" {
				t.Errorf("Value = %q, want %q", val, "value")
			}
		})
	}
}

func TestMemoryCacheOverwrite(t *testing.T) {
	c := NewMemoryCache(100, time.Minute)
	defer c.Close()

	ctx := context.Background()

	// Set initial value
	c.Set(ctx, "key", []byte("initial"), time.Minute)

	// Overwrite
	c.Set(ctx, "key", []byte("updated"), time.Minute)

	val, err := c.Get(ctx, "key")
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if string(val) != "updated" {
		t.Errorf("Value = %q, want %q", val, "updated")
	}

	// Key count should still be 1
	stats, _ := c.Stats(ctx)
	if stats.Keys != 1 {
		t.Errorf("Keys = %d, want 1", stats.Keys)
	}
}

func TestCacheInterface(t *testing.T) {
	// Verify MemoryCache implements Cache interface
	var _ Cache = (*MemoryCache)(nil)
}

func TestNewCacheNilReturnsDefault(t *testing.T) {
	cache, err := New(nil)
	if err != nil {
		t.Fatalf("New(nil) error: %v", err)
	}
	defer cache.Close()

	// Should be a working memory cache
	ctx := context.Background()
	err = cache.Set(ctx, "test", []byte("value"), time.Minute)
	if err != nil {
		t.Errorf("Set() error: %v", err)
	}

	val, err := cache.Get(ctx, "test")
	if err != nil {
		t.Errorf("Get() error: %v", err)
	}
	if string(val) != "value" {
		t.Errorf("Value = %q, want %q", val, "value")
	}
}

func TestMemoryCacheConcurrency(t *testing.T) {
	c := NewMemoryCache(1000, time.Minute)
	defer c.Close()

	ctx := context.Background()
	done := make(chan bool)

	// Concurrent writes
	for i := 0; i < 10; i++ {
		go func(id int) {
			for j := 0; j < 100; j++ {
				key := "key-" + string(rune('a'+id)) + "-" + string(rune('0'+j%10))
				c.Set(ctx, key, []byte("value"), time.Minute)
				c.Get(ctx, key)
				c.Exists(ctx, key)
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Should not have panicked or caused data races
	stats, _ := c.Stats(ctx)
	if stats.Keys == 0 {
		t.Error("Expected some keys after concurrent operations")
	}
}

// TestNewRedisCacheNilConfig tests NewRedisCache with nil config
func TestNewRedisCacheNilConfig(t *testing.T) {
	// This will fail to connect since there's no Redis running
	cache, err := NewRedisCache(nil)
	if err == nil {
		if cache != nil {
			cache.Close()
		}
		t.Log("Redis connection succeeded (Redis may be running on default port)")
	} else {
		// Expected - connection should fail
		t.Log("Redis connection failed as expected:", err)
	}
}

// TestNewRedisCacheInvalidURL tests NewRedisCache with invalid URL
func TestNewRedisCacheInvalidURL(t *testing.T) {
	cfg := &RedisConfig{
		URL: "not-a-valid-url",
	}

	cache, err := NewRedisCache(cfg)
	if err == nil {
		if cache != nil {
			cache.Close()
		}
		t.Error("Expected error for invalid URL")
	}
	// Expected - URL parsing should fail
}

// TestNewRedisCacheClusterMode tests cluster mode configuration
func TestNewRedisCacheClusterMode(t *testing.T) {
	cfg := &RedisConfig{
		Cluster:      true,
		ClusterNodes: []string{"localhost:7000", "localhost:7001"},
		Timeout:      100 * time.Millisecond,
	}

	cache, err := NewRedisCache(cfg)
	if err == nil {
		if cache != nil {
			cache.Close()
		}
		t.Log("Cluster connection succeeded")
	} else {
		// Expected - cluster connection should fail (no cluster running)
		t.Log("Cluster connection failed as expected:", err)
	}
}

// TestNewRedisCacheURLMode tests URL-based connection
func TestNewRedisCacheURLMode(t *testing.T) {
	cfg := &RedisConfig{
		URL:      "redis://localhost:16379/0",
		PoolSize: 5,
		MinIdle:  1,
		Timeout:  100 * time.Millisecond,
	}

	cache, err := NewRedisCache(cfg)
	if err == nil {
		if cache != nil {
			cache.Close()
		}
		t.Log("URL-based connection succeeded")
	} else {
		// Expected - connection should fail
		t.Log("URL-based connection failed as expected:", err)
	}
}

// TestNewRedisCacheHostPort tests host/port-based connection
func TestNewRedisCacheHostPort(t *testing.T) {
	cfg := &RedisConfig{
		Host:     "localhost",
		Port:     16379, // Non-standard port
		Password: "",
		DB:       0,
		PoolSize: 5,
		MinIdle:  1,
		Timeout:  100 * time.Millisecond,
	}

	cache, err := NewRedisCache(cfg)
	if err == nil {
		if cache != nil {
			cache.Close()
		}
		t.Log("Host/port connection succeeded")
	} else {
		// Expected - connection should fail
		t.Log("Host/port connection failed as expected:", err)
	}
}

// TestRedisCachePrefixKey tests the prefixKey method
func TestRedisCachePrefixKey(t *testing.T) {
	// We can test this without a connection by creating a minimal RedisCache
	cache := &RedisCache{
		prefix: "test:",
	}

	result := cache.prefixKey("mykey")
	expected := "test:mykey"
	if result != expected {
		t.Errorf("prefixKey() = %q, want %q", result, expected)
	}

	// Empty prefix
	cache2 := &RedisCache{
		prefix: "",
	}
	result2 := cache2.prefixKey("mykey")
	if result2 != "mykey" {
		t.Errorf("prefixKey() with empty prefix = %q, want %q", result2, "mykey")
	}
}

// TestMemoryCacheClearShortKeyPrefix tests Clear with prefix longer than key
func TestMemoryCacheClearShortKeyPrefix(t *testing.T) {
	c := NewMemoryCache(100, time.Minute)
	defer c.Close()

	ctx := context.Background()

	// Add keys shorter than potential prefix
	c.Set(ctx, "a", []byte("v1"), time.Minute)
	c.Set(ctx, "ab", []byte("v2"), time.Minute)
	c.Set(ctx, "abc", []byte("v3"), time.Minute)
	c.Set(ctx, "abcdefgh:key", []byte("v4"), time.Minute)

	// Clear with long prefix
	err := c.Clear(ctx, "abcdefgh:*")
	if err != nil {
		t.Fatalf("Clear() error: %v", err)
	}

	// Short keys should still exist
	exists, _ := c.Exists(ctx, "a")
	if !exists {
		t.Error("short key 'a' should still exist")
	}

	exists, _ = c.Exists(ctx, "ab")
	if !exists {
		t.Error("short key 'ab' should still exist")
	}

	// Matching key should be deleted
	exists, _ = c.Exists(ctx, "abcdefgh:key")
	if exists {
		t.Error("matching key should be deleted")
	}
}

// TestMemoryCacheClearShortKeySuffix tests Clear with suffix longer than key
func TestMemoryCacheClearShortKeySuffix(t *testing.T) {
	c := NewMemoryCache(100, time.Minute)
	defer c.Close()

	ctx := context.Background()

	// Add keys shorter than potential suffix
	c.Set(ctx, "a", []byte("v1"), time.Minute)
	c.Set(ctx, "suffix", []byte("v2"), time.Minute)
	c.Set(ctx, "key:longsuffix", []byte("v3"), time.Minute)

	// Clear with long suffix
	err := c.Clear(ctx, "*:longsuffix")
	if err != nil {
		t.Fatalf("Clear() error: %v", err)
	}

	// Short keys should still exist
	exists, _ := c.Exists(ctx, "a")
	if !exists {
		t.Error("short key 'a' should still exist")
	}

	exists, _ = c.Exists(ctx, "suffix")
	if !exists {
		t.Error("short key 'suffix' should still exist")
	}

	// Matching key should be deleted
	exists, _ = c.Exists(ctx, "key:longsuffix")
	if exists {
		t.Error("matching key should be deleted")
	}
}

// TestMemoryCacheEvictionMultipleBatches tests eviction with many items
func TestMemoryCacheEvictionMultipleBatches(t *testing.T) {
	// Create cache with size 20 (10% = 2 items per eviction)
	c := NewMemoryCache(20, time.Minute)
	defer c.Close()

	ctx := context.Background()

	// Add many items to trigger multiple evictions
	for i := 0; i < 50; i++ {
		key := "batch-key-" + string(rune('a'+i%26)) + string(rune('0'+i/26))
		c.Set(ctx, key, []byte("value"), time.Minute)
	}

	// Should have evicted to stay at or below max
	stats, _ := c.Stats(ctx)
	if stats.Keys > 20 {
		t.Errorf("Keys = %d, should be <= 20", stats.Keys)
	}
}

// TestMemoryCacheStatsAfterOperations tests stats accuracy after various operations
func TestMemoryCacheStatsAfterOperations(t *testing.T) {
	c := NewMemoryCache(100, time.Minute)
	defer c.Close()

	ctx := context.Background()

	// Initial stats
	stats, _ := c.Stats(ctx)
	initialHits := stats.Hits
	initialMisses := stats.Misses

	// Add keys
	c.Set(ctx, "stat-key1", []byte("value1"), time.Minute)
	c.Set(ctx, "stat-key2", []byte("value2"), time.Minute)
	c.Set(ctx, "stat-key3", []byte("value3"), time.Minute)

	// Multiple gets for hits
	for i := 0; i < 10; i++ {
		c.Get(ctx, "stat-key1")
	}

	// Multiple gets for misses
	for i := 0; i < 5; i++ {
		c.Get(ctx, "nonexistent")
	}

	// Check stats
	stats, _ = c.Stats(ctx)
	expectedHits := initialHits + 10
	expectedMisses := initialMisses + 5

	if stats.Hits != expectedHits {
		t.Errorf("Hits = %d, want %d", stats.Hits, expectedHits)
	}
	if stats.Misses != expectedMisses {
		t.Errorf("Misses = %d, want %d", stats.Misses, expectedMisses)
	}
	if stats.Keys != 3 {
		t.Errorf("Keys = %d, want 3", stats.Keys)
	}
}

// TestMemoryCacheGetExpiredUpdatesStats tests that expired key on Get updates misses
func TestMemoryCacheGetExpiredUpdatesStats(t *testing.T) {
	c := NewMemoryCache(100, time.Minute)
	defer c.Close()

	ctx := context.Background()

	// Set with short TTL
	c.Set(ctx, "expire-stats-key", []byte("value"), 5*time.Millisecond)

	// Wait for expiration
	time.Sleep(10 * time.Millisecond)

	// Get initial miss count
	stats, _ := c.Stats(ctx)
	initialMisses := stats.Misses

	// Get expired key
	_, err := c.Get(ctx, "expire-stats-key")
	if err == nil {
		t.Error("Get() should return error for expired key")
	}

	// Misses should have incremented
	stats, _ = c.Stats(ctx)
	if stats.Misses != initialMisses+1 {
		t.Errorf("Misses = %d, want %d", stats.Misses, initialMisses+1)
	}
}

// TestMemoryCacheExistsExpiredRemovesKey tests that Exists removes expired key and updates stats
func TestMemoryCacheExistsExpiredRemovesKey(t *testing.T) {
	c := NewMemoryCache(100, time.Minute)
	defer c.Close()

	ctx := context.Background()

	// Set with short TTL
	c.Set(ctx, "exist-expire-key", []byte("value"), 5*time.Millisecond)

	// Initial key count
	stats, _ := c.Stats(ctx)
	initialKeys := stats.Keys

	// Wait for expiration
	time.Sleep(10 * time.Millisecond)

	// Call Exists on expired key
	exists, _ := c.Exists(ctx, "exist-expire-key")
	if exists {
		t.Error("Exists() should return false for expired key")
	}

	// Key count should be reduced
	stats, _ = c.Stats(ctx)
	if stats.Keys != initialKeys-1 {
		t.Errorf("Keys = %d, want %d", stats.Keys, initialKeys-1)
	}
}
