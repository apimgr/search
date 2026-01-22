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

// Additional tests for 100% coverage

func TestResultCacheGetModifyDoesNotAffectCache(t *testing.T) {
	cache := NewResultCache(10, time.Minute)

	original := &model.SearchResults{
		Query: "test",
		Results: []model.Result{
			{URL: "https://example.com", Title: "Test"},
		},
	}
	cache.Set("key1", original)

	// Get and modify the returned value
	got := cache.Get("key1")
	got.Results = append(got.Results, model.Result{URL: "https://modified.com"})

	// Get again and verify original is unchanged
	got2 := cache.Get("key1")
	if len(got2.Results) != 1 {
		t.Error("Cache should return a copy, modifications should not affect cached value")
	}
}

func TestResultCacheSetOverwrite(t *testing.T) {
	cache := NewResultCache(10, time.Minute)

	cache.Set("key1", &model.SearchResults{Query: "original"})
	cache.Set("key1", &model.SearchResults{Query: "updated"})

	got := cache.Get("key1")
	if got.Query != "updated" {
		t.Errorf("Set should overwrite, Query = %q, want 'updated'", got.Query)
	}
}

func TestResultCacheEvictOldestMultiple(t *testing.T) {
	cache := NewResultCache(2, time.Minute)

	// Fill cache
	cache.Set("key1", &model.SearchResults{Query: "test1"})
	time.Sleep(5 * time.Millisecond)
	cache.Set("key2", &model.SearchResults{Query: "test2"})

	// Add two more to trigger eviction twice
	time.Sleep(5 * time.Millisecond)
	cache.Set("key3", &model.SearchResults{Query: "test3"})
	time.Sleep(5 * time.Millisecond)
	cache.Set("key4", &model.SearchResults{Query: "test4"})

	if cache.Size() != 2 {
		t.Errorf("Size() = %d, want 2", cache.Size())
	}

	// key1 and key2 should be evicted
	if cache.Get("key1") != nil {
		t.Error("key1 should be evicted")
	}
	if cache.Get("key2") != nil {
		t.Error("key2 should be evicted")
	}
}

func TestResultCacheDeleteNonExistent(t *testing.T) {
	cache := NewResultCache(10, time.Minute)

	// Should not panic when deleting non-existent key
	cache.Delete("nonexistent")

	if cache.Size() != 0 {
		t.Errorf("Size() = %d, want 0", cache.Size())
	}
}

func TestResultCacheClearResetsStats(t *testing.T) {
	cache := NewResultCache(10, time.Minute)

	cache.Set("key1", &model.SearchResults{Query: "test"})
	cache.Get("key1") // hit
	cache.Get("nonexistent") // miss

	cache.Clear()

	stats := cache.Stats()
	if stats.Hits != 0 || stats.Misses != 0 {
		t.Errorf("Clear should reset stats, Hits=%d, Misses=%d", stats.Hits, stats.Misses)
	}
}

func TestResultCacheSetCopiesResults(t *testing.T) {
	cache := NewResultCache(10, time.Minute)

	results := &model.SearchResults{
		Query: "test",
		Results: []model.Result{
			{URL: "https://example.com"},
		},
	}
	cache.Set("key1", results)

	// Modify original after setting
	results.Results[0].URL = "https://modified.com"

	// Cache should still have original value
	got := cache.Get("key1")
	if got.Results[0].URL != "https://example.com" {
		t.Error("Cache should store a copy, not reference to original")
	}
}

func TestResultCacheEvictOldestEmpty(t *testing.T) {
	cache := NewResultCache(10, time.Minute)

	// Manually call evictOldest on empty cache
	cache.mu.Lock()
	cache.evictOldest()
	cache.mu.Unlock()

	// Should not panic
	if cache.Size() != 0 {
		t.Errorf("Size() = %d, want 0", cache.Size())
	}
}

func TestResultCacheNegativeMaxSize(t *testing.T) {
	cache := NewResultCache(-5, time.Minute)

	// Negative maxSize should default to 1000
	if cache.maxSize != 1000 {
		t.Errorf("maxSize = %d, want 1000 (default)", cache.maxSize)
	}
}

func TestResultCacheNegativeTTL(t *testing.T) {
	cache := NewResultCache(10, -5*time.Minute)

	// Negative TTL should default to 5 minutes
	if cache.ttl != 5*time.Minute {
		t.Errorf("ttl = %v, want 5m (default)", cache.ttl)
	}
}

func TestResultCacheRemoveExpiredPartial(t *testing.T) {
	cache := NewResultCache(10, 10*time.Millisecond)

	// Add one that will expire and one that won't
	cache.mu.Lock()
	cache.items["expired"] = &cacheItem{
		results:   &model.SearchResults{Query: "expired"},
		expiresAt: time.Now().Add(-1 * time.Hour),
	}
	cache.items["valid"] = &cacheItem{
		results:   &model.SearchResults{Query: "valid"},
		expiresAt: time.Now().Add(1 * time.Hour),
	}
	cache.mu.Unlock()

	cache.removeExpired()

	if cache.Size() != 1 {
		t.Errorf("Size() after partial removeExpired = %d, want 1", cache.Size())
	}

	if cache.Get("valid") == nil {
		t.Error("Valid item should remain after removeExpired")
	}
}

func TestResultCacheStatsAfterOperations(t *testing.T) {
	cache := NewResultCache(10, time.Minute)

	// Multiple operations
	cache.Set("key1", &model.SearchResults{Query: "test1"})
	cache.Set("key2", &model.SearchResults{Query: "test2"})
	cache.Get("key1") // hit
	cache.Get("key2") // hit
	cache.Get("key3") // miss
	cache.Get("key4") // miss
	cache.Delete("key1")
	cache.Get("key1") // miss (after delete)

	stats := cache.Stats()

	if stats.Hits != 2 {
		t.Errorf("Hits = %d, want 2", stats.Hits)
	}
	if stats.Misses != 3 {
		t.Errorf("Misses = %d, want 3", stats.Misses)
	}
	if stats.Size != 1 {
		t.Errorf("Size = %d, want 1", stats.Size)
	}
}

func TestResultCacheSetWithEmptyResults(t *testing.T) {
	cache := NewResultCache(10, time.Minute)

	results := &model.SearchResults{
		Query:   "test",
		Results: []model.Result{}, // Empty results
	}
	cache.Set("key1", results)

	got := cache.Get("key1")
	if got == nil {
		t.Fatal("Should be able to cache empty results")
	}
	if len(got.Results) != 0 {
		t.Errorf("Results count = %d, want 0", len(got.Results))
	}
}

func TestCacheItemStruct(t *testing.T) {
	item := cacheItem{
		results: &model.SearchResults{
			Query: "test",
		},
		expiresAt: time.Now().Add(1 * time.Hour),
	}

	if item.results.Query != "test" {
		t.Errorf("Query = %q, want test", item.results.Query)
	}
	if time.Now().After(item.expiresAt) {
		t.Error("Item should not be expired")
	}
}
