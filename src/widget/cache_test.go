package widget

import (
	"testing"
	"time"
)

func TestNewCache(t *testing.T) {
	c := NewCache()
	if c == nil {
		t.Fatal("NewCache() returned nil")
	}
	if c.items == nil {
		t.Error("Cache items map not initialized")
	}
}

func TestCacheSetGet(t *testing.T) {
	c := NewCache()

	data := &WidgetData{
		Type:      WidgetWeather,
		Data:      map[string]interface{}{"temp": 25},
		UpdatedAt: time.Now(),
	}

	c.Set("weather:NYC", data, 5*time.Minute)

	result, ok := c.Get("weather:NYC")
	if !ok {
		t.Error("Get() returned false for existing key")
	}
	if result.Type != WidgetWeather {
		t.Errorf("Result type = %v, want %v", result.Type, WidgetWeather)
	}
}

func TestCacheGetNotFound(t *testing.T) {
	c := NewCache()

	result, ok := c.Get("nonexistent")
	if ok {
		t.Error("Get() should return false for nonexistent key")
	}
	if result != nil {
		t.Error("Get() should return nil for nonexistent key")
	}
}

func TestCacheGetExpired(t *testing.T) {
	c := NewCache()

	data := &WidgetData{
		Type:      WidgetWeather,
		UpdatedAt: time.Now(),
	}

	// Set with very short TTL
	c.Set("weather:NYC", data, 1*time.Millisecond)

	// Wait for expiration
	time.Sleep(5 * time.Millisecond)

	result, ok := c.Get("weather:NYC")
	if ok {
		t.Error("Get() should return false for expired key")
	}
	if result != nil {
		t.Error("Get() should return nil for expired key")
	}
}

func TestCacheDelete(t *testing.T) {
	c := NewCache()

	data := &WidgetData{Type: WidgetWeather}
	c.Set("weather:NYC", data, 5*time.Minute)

	// Verify it exists
	_, ok := c.Get("weather:NYC")
	if !ok {
		t.Fatal("Key should exist before delete")
	}

	c.Delete("weather:NYC")

	// Verify it's gone
	_, ok = c.Get("weather:NYC")
	if ok {
		t.Error("Key should not exist after delete")
	}
}

func TestCacheDeleteNonExistent(t *testing.T) {
	c := NewCache()

	// Should not panic
	c.Delete("nonexistent")
}

func TestCacheClear(t *testing.T) {
	c := NewCache()

	// Add multiple items
	c.Set("key1", &WidgetData{Type: WidgetWeather}, 5*time.Minute)
	c.Set("key2", &WidgetData{Type: WidgetNews}, 5*time.Minute)
	c.Set("key3", &WidgetData{Type: WidgetStocks}, 5*time.Minute)

	if c.Size() != 3 {
		t.Errorf("Size() = %d, want 3", c.Size())
	}

	c.Clear()

	if c.Size() != 0 {
		t.Errorf("Size() after Clear() = %d, want 0", c.Size())
	}
}

func TestCacheSize(t *testing.T) {
	c := NewCache()

	if c.Size() != 0 {
		t.Errorf("Initial Size() = %d, want 0", c.Size())
	}

	c.Set("key1", &WidgetData{}, 5*time.Minute)
	if c.Size() != 1 {
		t.Errorf("Size() = %d, want 1", c.Size())
	}

	c.Set("key2", &WidgetData{}, 5*time.Minute)
	if c.Size() != 2 {
		t.Errorf("Size() = %d, want 2", c.Size())
	}
}

func TestCacheKeys(t *testing.T) {
	c := NewCache()

	keys := c.Keys()
	if len(keys) != 0 {
		t.Errorf("Initial Keys() length = %d, want 0", len(keys))
	}

	c.Set("key1", &WidgetData{}, 5*time.Minute)
	c.Set("key2", &WidgetData{}, 5*time.Minute)

	keys = c.Keys()
	if len(keys) != 2 {
		t.Errorf("Keys() length = %d, want 2", len(keys))
	}

	// Verify keys are present
	keyMap := make(map[string]bool)
	for _, k := range keys {
		keyMap[k] = true
	}
	if !keyMap["key1"] || !keyMap["key2"] {
		t.Error("Keys() missing expected keys")
	}
}

func TestCacheRemoveExpired(t *testing.T) {
	c := NewCache()

	// Add expired and non-expired items
	c.Set("expired", &WidgetData{}, 1*time.Millisecond)
	c.Set("valid", &WidgetData{}, 5*time.Minute)

	// Wait for expiration
	time.Sleep(5 * time.Millisecond)

	// Manual call to removeExpired
	c.removeExpired()

	// Valid item should still exist
	_, ok := c.Get("valid")
	if !ok {
		t.Error("Valid item should still exist")
	}

	// Size should be 1 (only valid item)
	if c.Size() != 1 {
		t.Errorf("Size() after removeExpired = %d, want 1", c.Size())
	}
}

func TestCacheItemStruct(t *testing.T) {
	now := time.Now()
	data := &WidgetData{Type: WidgetWeather}

	item := &CacheItem{
		Data:      data,
		ExpiresAt: now.Add(5 * time.Minute),
	}

	if item.Data != data {
		t.Error("CacheItem.Data mismatch")
	}
	if item.ExpiresAt.Before(now) {
		t.Error("CacheItem.ExpiresAt should be in future")
	}
}

func TestCacheOverwrite(t *testing.T) {
	c := NewCache()

	data1 := &WidgetData{Type: WidgetWeather, Data: "first"}
	data2 := &WidgetData{Type: WidgetWeather, Data: "second"}

	c.Set("key", data1, 5*time.Minute)
	c.Set("key", data2, 5*time.Minute)

	result, ok := c.Get("key")
	if !ok {
		t.Fatal("Key should exist")
	}
	if result.Data != "second" {
		t.Error("Cache should return overwritten value")
	}
}

func TestCacheConcurrency(t *testing.T) {
	c := NewCache()

	// Test concurrent access
	done := make(chan bool)

	// Writer goroutine
	go func() {
		for i := 0; i < 100; i++ {
			c.Set("key", &WidgetData{}, 5*time.Minute)
		}
		done <- true
	}()

	// Reader goroutine
	go func() {
		for i := 0; i < 100; i++ {
			c.Get("key")
		}
		done <- true
	}()

	// Wait for both
	<-done
	<-done
}
