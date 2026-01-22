package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/spf13/viper"
)

// Tests for CacheConfig

func TestCacheConfigStruct(t *testing.T) {
	cfg := CacheConfig{
		Enabled: true,
		TTL:     300,
		MaxSize: 100,
	}

	if !cfg.Enabled {
		t.Error("Enabled should be true")
	}
	if cfg.TTL != 300 {
		t.Errorf("TTL = %d, want 300", cfg.TTL)
	}
	if cfg.MaxSize != 100 {
		t.Errorf("MaxSize = %d, want 100", cfg.MaxSize)
	}
}

func TestGetCacheConfig(t *testing.T) {
	// Reset viper for test
	viper.Reset()
	viper.Set("cache.enabled", true)
	viper.Set("cache.ttl", 600)
	viper.Set("cache.max_size", 200)

	cfg := GetCacheConfig()

	if !cfg.Enabled {
		t.Error("Enabled should be true")
	}
	if cfg.TTL != 600 {
		t.Errorf("TTL = %d, want 600", cfg.TTL)
	}
	if cfg.MaxSize != 200 {
		t.Errorf("MaxSize = %d, want 200", cfg.MaxSize)
	}
}

func TestGetCacheConfigDefaults(t *testing.T) {
	viper.Reset()

	cfg := GetCacheConfig()

	// Should return zero values when not set
	if cfg.Enabled {
		t.Error("Enabled should be false when not set")
	}
	if cfg.TTL != 0 {
		t.Errorf("TTL = %d, want 0 when not set", cfg.TTL)
	}
}

// Tests for CacheEntry

func TestCacheEntryStruct(t *testing.T) {
	now := time.Now()
	entry := CacheEntry{
		Data:      []byte("test data"),
		ExpiresAt: now,
	}

	if string(entry.Data) != "test data" {
		t.Errorf("Data = %q, want 'test data'", string(entry.Data))
	}
	if !entry.ExpiresAt.Equal(now) {
		t.Errorf("ExpiresAt mismatch")
	}
}

func TestCacheEntryJSON(t *testing.T) {
	entry := CacheEntry{
		Data:      []byte("json test"),
		ExpiresAt: time.Now().Add(time.Hour),
	}

	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	var decoded CacheEntry
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if string(decoded.Data) != string(entry.Data) {
		t.Errorf("decoded.Data = %q, want %q", string(decoded.Data), string(entry.Data))
	}
}

// Tests for CLICache

func TestCLICacheStruct(t *testing.T) {
	cache := &CLICache{
		memory:    make(map[string]CacheEntry),
		ttl:       300 * time.Second,
		maxSize:   100 * 1024 * 1024,
		cacheDir:  "/tmp/test-cache",
		enabled:   true,
		totalSize: 0,
	}

	if cache.ttl != 300*time.Second {
		t.Errorf("ttl = %v", cache.ttl)
	}
	if cache.maxSize != 100*1024*1024 {
		t.Errorf("maxSize = %d", cache.maxSize)
	}
	if !cache.enabled {
		t.Error("enabled should be true")
	}
}

// Tests for InitCache

func TestInitCache(t *testing.T) {
	// Reset state for test
	cliCache = nil
	cliCacheOnce = sync.Once{}
	viper.Reset()

	tempDir := t.TempDir()
	viper.Set("cache.enabled", true)
	viper.Set("cache.ttl", 300)
	viper.Set("cache.max_size", 100)

	// Override cache dir through viper isn't straightforward,
	// but we can test the function runs without error
	err := InitCache()
	if err != nil {
		t.Fatalf("InitCache() error = %v", err)
	}

	// Verify cache was created
	cache := Cache()
	if cache == nil {
		t.Error("Cache() returned nil after InitCache()")
	}

	// Clean up
	if cliCache != nil && cliCache.cacheDir != "" {
		os.RemoveAll(cliCache.cacheDir)
	}

	_ = tempDir // Use tempDir to avoid unused variable error
}

func TestInitCacheDefaults(t *testing.T) {
	// Reset state
	cliCache = nil
	cliCacheOnce = sync.Once{}
	viper.Reset()

	err := InitCache()
	if err != nil {
		t.Fatalf("InitCache() error = %v", err)
	}

	cache := Cache()
	if cache == nil {
		t.Error("Cache() should not return nil")
	}
}

// Tests for Cache()

func TestCacheReturnsInstance(t *testing.T) {
	// Reset state
	cliCache = nil
	cliCacheOnce = sync.Once{}
	viper.Reset()

	cache := Cache()
	if cache == nil {
		t.Error("Cache() returned nil")
	}
}

func TestCacheReturnsSameInstance(t *testing.T) {
	// Reset state
	cliCache = nil
	cliCacheOnce = sync.Once{}
	viper.Reset()

	cache1 := Cache()
	cache2 := Cache()

	if cache1 != cache2 {
		t.Error("Cache() should return same instance")
	}
}

// Tests for CLICache.Get

func TestCLICacheGetEnabled(t *testing.T) {
	cache := &CLICache{
		memory:   make(map[string]CacheEntry),
		ttl:      300 * time.Second,
		enabled:  true,
		cacheDir: t.TempDir(),
	}

	// Should return false for non-existent key
	data, found := cache.Get("nonexistent")
	if found {
		t.Error("Get() should return false for non-existent key")
	}
	if data != nil {
		t.Error("Get() should return nil data for non-existent key")
	}
}

func TestCLICacheGetDisabled(t *testing.T) {
	cache := &CLICache{
		memory:  make(map[string]CacheEntry),
		enabled: false,
	}

	// Set a value directly
	cache.memory["test"] = CacheEntry{
		Data:      []byte("test"),
		ExpiresAt: time.Now().Add(time.Hour),
	}

	// Should return false when disabled
	data, found := cache.Get("test")
	if found {
		t.Error("Get() should return false when cache is disabled")
	}
	if data != nil {
		t.Error("Get() should return nil data when cache is disabled")
	}
}

func TestCLICacheGetFromMemory(t *testing.T) {
	cache := &CLICache{
		memory:   make(map[string]CacheEntry),
		enabled:  true,
		cacheDir: t.TempDir(),
	}

	// Add entry to memory
	cache.memory["testkey"] = CacheEntry{
		Data:      []byte("testvalue"),
		ExpiresAt: time.Now().Add(time.Hour),
	}

	data, found := cache.Get("testkey")
	if !found {
		t.Error("Get() should return true for existing key")
	}
	if string(data) != "testvalue" {
		t.Errorf("Get() data = %q, want 'testvalue'", string(data))
	}
}

func TestCLICacheGetExpired(t *testing.T) {
	cache := &CLICache{
		memory:   make(map[string]CacheEntry),
		enabled:  true,
		cacheDir: t.TempDir(),
	}

	// Add expired entry
	cache.memory["expired"] = CacheEntry{
		Data:      []byte("old data"),
		ExpiresAt: time.Now().Add(-time.Hour), // expired
	}

	data, found := cache.Get("expired")
	if found {
		t.Error("Get() should return false for expired key")
	}
	if data != nil {
		t.Error("Get() should return nil data for expired key")
	}
}

// Tests for CLICache.Set

func TestCLICacheSetEnabled(t *testing.T) {
	cache := &CLICache{
		memory:   make(map[string]CacheEntry),
		ttl:      300 * time.Second,
		maxSize:  1024 * 1024,
		enabled:  true,
		cacheDir: t.TempDir(),
	}

	cache.Set("key1", []byte("value1"))

	// Verify it was stored
	entry, ok := cache.memory["key1"]
	if !ok {
		t.Error("Set() should store entry in memory")
	}
	if string(entry.Data) != "value1" {
		t.Errorf("Set() stored data = %q, want 'value1'", string(entry.Data))
	}
	if cache.totalSize != int64(len("value1")) {
		t.Errorf("totalSize = %d, want %d", cache.totalSize, len("value1"))
	}
}

func TestCLICacheSetDisabled(t *testing.T) {
	cache := &CLICache{
		memory:  make(map[string]CacheEntry),
		enabled: false,
	}

	cache.Set("key1", []byte("value1"))

	// Verify it was not stored
	if len(cache.memory) != 0 {
		t.Error("Set() should not store when disabled")
	}
}

func TestCLICacheSetLargeDataToFile(t *testing.T) {
	tempDir := t.TempDir()
	cache := &CLICache{
		memory:   make(map[string]CacheEntry),
		ttl:      300 * time.Second,
		maxSize:  10 * 1024 * 1024, // 10 MB
		enabled:  true,
		cacheDir: tempDir,
	}

	// Create large data (>1KB triggers file storage)
	largeData := make([]byte, 2048)
	for i := range largeData {
		largeData[i] = byte(i % 256)
	}

	cache.Set("largekey", largeData)

	// Verify stored in memory
	if _, ok := cache.memory["largekey"]; !ok {
		t.Error("Set() should store large data in memory")
	}

	// Verify file was created
	filePath := cache.cacheFilePath("largekey")
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		t.Error("Set() should create file for large data")
	}
}

// Tests for CLICache.Delete

func TestCLICacheDelete(t *testing.T) {
	tempDir := t.TempDir()
	cache := &CLICache{
		memory:    make(map[string]CacheEntry),
		enabled:   true,
		cacheDir:  tempDir,
		totalSize: 10,
	}

	cache.memory["todelete"] = CacheEntry{
		Data:      []byte("deletedata"),
		ExpiresAt: time.Now().Add(time.Hour),
	}

	cache.Delete("todelete")

	if _, ok := cache.memory["todelete"]; ok {
		t.Error("Delete() should remove entry from memory")
	}
}

func TestCLICacheDeleteDisabled(t *testing.T) {
	cache := &CLICache{
		memory:  make(map[string]CacheEntry),
		enabled: false,
	}

	cache.memory["test"] = CacheEntry{Data: []byte("test")}

	cache.Delete("test")

	// Should still exist when disabled
	if _, ok := cache.memory["test"]; !ok {
		t.Error("Delete() should not remove when disabled")
	}
}

func TestCLICacheDeleteNonexistent(t *testing.T) {
	cache := &CLICache{
		memory:   make(map[string]CacheEntry),
		enabled:  true,
		cacheDir: t.TempDir(),
	}

	// Should not panic
	cache.Delete("nonexistent")
}

// Tests for CLICache.Clear

func TestCLICacheClear(t *testing.T) {
	tempDir := t.TempDir()
	cache := &CLICache{
		memory:    make(map[string]CacheEntry),
		enabled:   true,
		cacheDir:  tempDir,
		totalSize: 100,
	}

	cache.memory["key1"] = CacheEntry{Data: []byte("val1")}
	cache.memory["key2"] = CacheEntry{Data: []byte("val2")}

	cache.Clear()

	if len(cache.memory) != 0 {
		t.Error("Clear() should empty memory map")
	}
	if cache.totalSize != 0 {
		t.Errorf("Clear() should reset totalSize to 0, got %d", cache.totalSize)
	}
}

func TestCLICacheClearDisabled(t *testing.T) {
	cache := &CLICache{
		memory:    make(map[string]CacheEntry),
		enabled:   false,
		totalSize: 100,
	}

	cache.memory["key1"] = CacheEntry{Data: []byte("val1")}

	cache.Clear()

	// Should not clear when disabled
	if len(cache.memory) != 1 {
		t.Error("Clear() should not clear when disabled")
	}
}

// Tests for CLICache.Cleanup

func TestCLICacheCleanup(t *testing.T) {
	tempDir := t.TempDir()
	cache := &CLICache{
		memory:    make(map[string]CacheEntry),
		enabled:   true,
		cacheDir:  tempDir,
		totalSize: 20,
	}

	// Add expired entry
	cache.memory["expired"] = CacheEntry{
		Data:      []byte("olddata"),
		ExpiresAt: time.Now().Add(-time.Hour),
	}
	// Add valid entry
	cache.memory["valid"] = CacheEntry{
		Data:      []byte("newdata"),
		ExpiresAt: time.Now().Add(time.Hour),
	}

	cache.Cleanup()

	if _, ok := cache.memory["expired"]; ok {
		t.Error("Cleanup() should remove expired entries")
	}
	if _, ok := cache.memory["valid"]; !ok {
		t.Error("Cleanup() should keep valid entries")
	}
}

func TestCLICacheCleanupDisabled(t *testing.T) {
	cache := &CLICache{
		memory:  make(map[string]CacheEntry),
		enabled: false,
	}

	cache.memory["expired"] = CacheEntry{
		Data:      []byte("old"),
		ExpiresAt: time.Now().Add(-time.Hour),
	}

	cache.Cleanup()

	// Should not clean when disabled
	if _, ok := cache.memory["expired"]; !ok {
		t.Error("Cleanup() should not remove when disabled")
	}
}

// Tests for evictOldest

func TestCLICacheEvictOldest(t *testing.T) {
	cache := &CLICache{
		memory:    make(map[string]CacheEntry),
		maxSize:   100,
		enabled:   true,
		cacheDir:  t.TempDir(),
		totalSize: 80,
	}

	// Add entries with different expiration times
	cache.memory["oldest"] = CacheEntry{
		Data:      []byte("1234567890"), // 10 bytes
		ExpiresAt: time.Now().Add(time.Minute),
	}
	cache.memory["newer"] = CacheEntry{
		Data:      []byte("1234567890"), // 10 bytes
		ExpiresAt: time.Now().Add(time.Hour),
	}

	// Evict to make room for 50 more bytes
	cache.evictOldest(50)

	// Should have evicted oldest entry
	if _, ok := cache.memory["oldest"]; ok {
		t.Error("evictOldest() should remove oldest entry")
	}
}

func TestCLICacheEvictOldestEmptyCache(t *testing.T) {
	cache := &CLICache{
		memory:    make(map[string]CacheEntry),
		maxSize:   100,
		enabled:   true,
		cacheDir:  t.TempDir(),
		totalSize: 0,
	}

	// Should not panic on empty cache
	cache.evictOldest(50)
}

// Tests for getFromFile

func TestCLICacheGetFromFile(t *testing.T) {
	tempDir := t.TempDir()
	cache := &CLICache{
		memory:   make(map[string]CacheEntry),
		enabled:  true,
		cacheDir: tempDir,
	}

	// Create a cache file
	entry := CacheEntry{
		Data:      []byte("filedata"),
		ExpiresAt: time.Now().Add(time.Hour),
	}
	entryJSON, _ := json.Marshal(entry)
	filePath := cache.cacheFilePath("filekey")
	os.WriteFile(filePath, entryJSON, 0600)

	data, found := cache.getFromFile("filekey")
	if !found {
		t.Error("getFromFile() should find valid file")
	}
	if string(data) != "filedata" {
		t.Errorf("getFromFile() data = %q, want 'filedata'", string(data))
	}
}

func TestCLICacheGetFromFileExpired(t *testing.T) {
	tempDir := t.TempDir()
	cache := &CLICache{
		memory:   make(map[string]CacheEntry),
		enabled:  true,
		cacheDir: tempDir,
	}

	// Create an expired cache file
	entry := CacheEntry{
		Data:      []byte("expireddata"),
		ExpiresAt: time.Now().Add(-time.Hour),
	}
	entryJSON, _ := json.Marshal(entry)
	filePath := cache.cacheFilePath("expiredfile")
	os.WriteFile(filePath, entryJSON, 0600)

	data, found := cache.getFromFile("expiredfile")
	if found {
		t.Error("getFromFile() should not find expired file")
	}
	if data != nil {
		t.Error("getFromFile() should return nil for expired file")
	}

	// File should be deleted
	if _, err := os.Stat(filePath); !os.IsNotExist(err) {
		t.Error("getFromFile() should delete expired file")
	}
}

func TestCLICacheGetFromFileNotFound(t *testing.T) {
	tempDir := t.TempDir()
	cache := &CLICache{
		memory:   make(map[string]CacheEntry),
		enabled:  true,
		cacheDir: tempDir,
	}

	data, found := cache.getFromFile("nonexistent")
	if found {
		t.Error("getFromFile() should return false for non-existent file")
	}
	if data != nil {
		t.Error("getFromFile() should return nil for non-existent file")
	}
}

func TestCLICacheGetFromFileInvalidJSON(t *testing.T) {
	tempDir := t.TempDir()
	cache := &CLICache{
		memory:   make(map[string]CacheEntry),
		enabled:  true,
		cacheDir: tempDir,
	}

	// Create invalid JSON file
	filePath := cache.cacheFilePath("invalidjson")
	os.WriteFile(filePath, []byte("not valid json"), 0600)

	data, found := cache.getFromFile("invalidjson")
	if found {
		t.Error("getFromFile() should return false for invalid JSON")
	}
	if data != nil {
		t.Error("getFromFile() should return nil for invalid JSON")
	}
}

// Tests for saveToFile

func TestCLICacheSaveToFile(t *testing.T) {
	tempDir := t.TempDir()
	cache := &CLICache{
		memory:   make(map[string]CacheEntry),
		enabled:  true,
		cacheDir: tempDir,
	}

	entry := CacheEntry{
		Data:      []byte("savedata"),
		ExpiresAt: time.Now().Add(time.Hour),
	}

	cache.saveToFile("savekey", entry)

	filePath := cache.cacheFilePath("savekey")
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		t.Error("saveToFile() should create file")
	}
}

// Tests for deleteFile

func TestCLICacheDeleteFile(t *testing.T) {
	tempDir := t.TempDir()
	cache := &CLICache{
		memory:   make(map[string]CacheEntry),
		enabled:  true,
		cacheDir: tempDir,
	}

	// Create a file to delete
	filePath := cache.cacheFilePath("delkey")
	os.WriteFile(filePath, []byte("to delete"), 0600)

	cache.deleteFile("delkey")

	if _, err := os.Stat(filePath); !os.IsNotExist(err) {
		t.Error("deleteFile() should remove file")
	}
}

func TestCLICacheDeleteFileNonexistent(t *testing.T) {
	tempDir := t.TempDir()
	cache := &CLICache{
		memory:   make(map[string]CacheEntry),
		enabled:  true,
		cacheDir: tempDir,
	}

	// Should not panic
	cache.deleteFile("nonexistent")
}

// Tests for cacheFilePath

func TestCLICacheCacheFilePath(t *testing.T) {
	cache := &CLICache{
		cacheDir: "/tmp/testcache",
	}

	path := cache.cacheFilePath("testkey")

	if !filepath.IsAbs(path) {
		t.Error("cacheFilePath() should return absolute path")
	}
	if filepath.Dir(path) != "/tmp/testcache" {
		t.Errorf("cacheFilePath() dir = %s", filepath.Dir(path))
	}
	if filepath.Ext(path) != ".cache" {
		t.Errorf("cacheFilePath() ext = %s, want .cache", filepath.Ext(path))
	}
}

func TestCLICacheCacheFilePathDifferentKeys(t *testing.T) {
	cache := &CLICache{
		cacheDir: "/tmp/testcache",
	}

	path1 := cache.cacheFilePath("key1")
	path2 := cache.cacheFilePath("key2")

	if path1 == path2 {
		t.Error("cacheFilePath() should return different paths for different keys")
	}
}

// Tests for simpleHash

func TestSimpleHash(t *testing.T) {
	hash1 := simpleHash("test")
	hash2 := simpleHash("test")

	if hash1 != hash2 {
		t.Error("simpleHash() should return same value for same input")
	}
}

func TestSimpleHashDifferentInputs(t *testing.T) {
	hash1 := simpleHash("test1")
	hash2 := simpleHash("test2")

	if hash1 == hash2 {
		t.Error("simpleHash() should return different values for different inputs")
	}
}

func TestSimpleHashFormat(t *testing.T) {
	hash := simpleHash("test")

	if len(hash) != 8 {
		t.Errorf("simpleHash() length = %d, want 8", len(hash))
	}

	// Should be hex characters only
	for _, c := range hash {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Errorf("simpleHash() contains non-hex character: %c", c)
		}
	}
}

func TestSimpleHashEmptyString(t *testing.T) {
	hash := simpleHash("")

	// Should still produce valid hash
	if len(hash) != 8 {
		t.Errorf("simpleHash('') length = %d, want 8", len(hash))
	}
}

func TestSimpleHashLongString(t *testing.T) {
	longStr := ""
	for i := 0; i < 1000; i++ {
		longStr += "a"
	}

	hash := simpleHash(longStr)

	if len(hash) != 8 {
		t.Errorf("simpleHash(longStr) length = %d, want 8", len(hash))
	}
}

// Tests for concurrent access

func TestCLICacheConcurrentAccess(t *testing.T) {
	cache := &CLICache{
		memory:   make(map[string]CacheEntry),
		ttl:      300 * time.Second,
		maxSize:  10 * 1024 * 1024,
		enabled:  true,
		cacheDir: t.TempDir(),
	}

	done := make(chan bool)

	// Writer goroutine
	go func() {
		for i := 0; i < 100; i++ {
			cache.Set("key", []byte("value"))
		}
		done <- true
	}()

	// Reader goroutine
	go func() {
		for i := 0; i < 100; i++ {
			cache.Get("key")
		}
		done <- true
	}()

	// Deleter goroutine
	go func() {
		for i := 0; i < 100; i++ {
			cache.Delete("key")
		}
		done <- true
	}()

	<-done
	<-done
	<-done
}

func TestCLICacheConcurrentCleanup(t *testing.T) {
	cache := &CLICache{
		memory:   make(map[string]CacheEntry),
		ttl:      300 * time.Second,
		maxSize:  10 * 1024 * 1024,
		enabled:  true,
		cacheDir: t.TempDir(),
	}

	done := make(chan bool)

	go func() {
		for i := 0; i < 50; i++ {
			cache.Set("key", []byte("value"))
		}
		done <- true
	}()

	go func() {
		for i := 0; i < 50; i++ {
			cache.Cleanup()
		}
		done <- true
	}()

	<-done
	<-done
}
