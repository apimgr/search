package search

import (
	"testing"
	"time"

	"github.com/apimgr/search/src/model"
)

func TestNewResultCache(t *testing.T) {
	cache := NewResultCache(100, 5*time.Minute)

	if cache == nil {
		t.Fatal("NewResultCache() returned nil")
	}
	if cache.maxSize != 100 {
		t.Errorf("maxSize = %d, want 100", cache.maxSize)
	}
	if cache.ttl != 5*time.Minute {
		t.Errorf("ttl = %v, want 5m", cache.ttl)
	}
}

func TestNewResultCacheDefaults(t *testing.T) {
	cache := NewResultCache(0, 0)

	if cache.maxSize != 1000 {
		t.Errorf("default maxSize = %d, want 1000", cache.maxSize)
	}
	if cache.ttl != 5*time.Minute {
		t.Errorf("default ttl = %v, want 5m", cache.ttl)
	}
}

func TestResultCacheSetGet(t *testing.T) {
	cache := NewResultCache(10, time.Minute)

	results := &model.SearchResults{
		Query:   "test",
		Results: []model.Result{{URL: "https://example.com", Title: "Test"}},
	}

	cache.Set("key1", results)

	got := cache.Get("key1")
	if got == nil {
		t.Fatal("Get() returned nil for existing key")
	}
	if got.Query != "test" {
		t.Errorf("Query = %q, want test", got.Query)
	}
	if len(got.Results) != 1 {
		t.Errorf("Results count = %d, want 1", len(got.Results))
	}
}

func TestResultCacheGetNotFound(t *testing.T) {
	cache := NewResultCache(10, time.Minute)

	got := cache.Get("nonexistent")
	if got != nil {
		t.Error("Get() should return nil for nonexistent key")
	}
}

func TestResultCacheGetExpired(t *testing.T) {
	cache := NewResultCache(10, 10*time.Millisecond)

	results := &model.SearchResults{Query: "test"}
	cache.Set("key1", results)

	// Wait for expiration
	time.Sleep(20 * time.Millisecond)

	got := cache.Get("key1")
	if got != nil {
		t.Error("Get() should return nil for expired item")
	}
}

func TestResultCacheDelete(t *testing.T) {
	cache := NewResultCache(10, time.Minute)

	results := &model.SearchResults{Query: "test"}
	cache.Set("key1", results)

	cache.Delete("key1")

	got := cache.Get("key1")
	if got != nil {
		t.Error("Get() should return nil after Delete()")
	}
}

func TestResultCacheClear(t *testing.T) {
	cache := NewResultCache(10, time.Minute)

	cache.Set("key1", &model.SearchResults{Query: "test1"})
	cache.Set("key2", &model.SearchResults{Query: "test2"})

	cache.Clear()

	if cache.Size() != 0 {
		t.Errorf("Size() after Clear() = %d, want 0", cache.Size())
	}
}

func TestResultCacheSize(t *testing.T) {
	cache := NewResultCache(10, time.Minute)

	if cache.Size() != 0 {
		t.Errorf("Initial Size() = %d, want 0", cache.Size())
	}

	cache.Set("key1", &model.SearchResults{Query: "test1"})
	cache.Set("key2", &model.SearchResults{Query: "test2"})

	if cache.Size() != 2 {
		t.Errorf("Size() = %d, want 2", cache.Size())
	}
}

func TestResultCacheStats(t *testing.T) {
	cache := NewResultCache(10, time.Minute)

	cache.Set("key1", &model.SearchResults{Query: "test"})

	// Generate hits and misses
	cache.Get("key1")      // hit
	cache.Get("key1")      // hit
	cache.Get("nonexistent") // miss

	stats := cache.Stats()

	if stats.Size != 1 {
		t.Errorf("stats.Size = %d, want 1", stats.Size)
	}
	if stats.MaxSize != 10 {
		t.Errorf("stats.MaxSize = %d, want 10", stats.MaxSize)
	}
	if stats.Hits != 2 {
		t.Errorf("stats.Hits = %d, want 2", stats.Hits)
	}
	if stats.Misses != 1 {
		t.Errorf("stats.Misses = %d, want 1", stats.Misses)
	}
	expectedHitRate := 2.0 / 3.0
	if stats.HitRate < expectedHitRate-0.01 || stats.HitRate > expectedHitRate+0.01 {
		t.Errorf("stats.HitRate = %f, want %f", stats.HitRate, expectedHitRate)
	}
}

func TestResultCacheEviction(t *testing.T) {
	cache := NewResultCache(3, time.Minute)

	// Fill cache
	cache.Set("key1", &model.SearchResults{Query: "test1"})
	time.Sleep(10 * time.Millisecond)
	cache.Set("key2", &model.SearchResults{Query: "test2"})
	time.Sleep(10 * time.Millisecond)
	cache.Set("key3", &model.SearchResults{Query: "test3"})

	// Add one more, should evict oldest
	time.Sleep(10 * time.Millisecond)
	cache.Set("key4", &model.SearchResults{Query: "test4"})

	if cache.Size() != 3 {
		t.Errorf("Size() after eviction = %d, want 3", cache.Size())
	}

	// key1 should be evicted (oldest)
	if cache.Get("key1") != nil {
		t.Error("key1 should be evicted")
	}
}

func TestResultCacheReturnsCopy(t *testing.T) {
	cache := NewResultCache(10, time.Minute)

	original := &model.SearchResults{
		Query:   "test",
		Results: []model.Result{{URL: "https://example.com"}},
	}
	cache.Set("key1", original)

	// Modify the original
	original.Results = append(original.Results, model.Result{URL: "https://modified.com"})

	// Get should return original cached value
	got := cache.Get("key1")
	if len(got.Results) != 1 {
		t.Error("Cache should store a copy, not reference")
	}
}

func TestResultCacheRemoveExpired(t *testing.T) {
	cache := NewResultCache(10, 10*time.Millisecond)

	cache.Set("key1", &model.SearchResults{Query: "test1"})
	cache.Set("key2", &model.SearchResults{Query: "test2"})

	// Wait for expiration
	time.Sleep(20 * time.Millisecond)

	// Manually trigger cleanup
	cache.removeExpired()

	if cache.Size() != 0 {
		t.Errorf("Size() after removeExpired() = %d, want 0", cache.Size())
	}
}

func TestCacheStatsStruct(t *testing.T) {
	stats := CacheStats{
		Size:    50,
		MaxSize: 100,
		Hits:    200,
		Misses:  50,
		HitRate: 0.8,
	}

	if stats.Size != 50 {
		t.Errorf("Size = %d, want 50", stats.Size)
	}
	if stats.HitRate != 0.8 {
		t.Errorf("HitRate = %f, want 0.8", stats.HitRate)
	}
}

func TestResultCacheStatsZeroTotal(t *testing.T) {
	cache := NewResultCache(10, time.Minute)

	stats := cache.Stats()

	// No gets yet, hit rate should be 0
	if stats.HitRate != 0 {
		t.Errorf("HitRate with no gets = %f, want 0", stats.HitRate)
	}
}
