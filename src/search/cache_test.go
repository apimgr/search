package search

import (
	"testing"
	"time"

	"github.com/apimgr/search/src/cache"
	"github.com/apimgr/search/src/model"
)

// newTestCache creates a ResultCache backed by a fast in-memory cache for tests.
func newTestCache(ttl time.Duration) *ResultCache {
	if ttl <= 0 {
		ttl = time.Minute
	}
	backend := cache.NewMemoryCache(1000, ttl)
	return NewResultCache(backend, ttl)
}

func TestNewResultCache(t *testing.T) {
	rc := newTestCache(5 * time.Minute)
	if rc == nil {
		t.Fatal("NewResultCache() returned nil")
	}
	if rc.ttl != 5*time.Minute {
		t.Errorf("ttl = %v, want 5m", rc.ttl)
	}
}

func TestNewResultCacheDefaults(t *testing.T) {
	// Zero TTL should default to 5 minutes
	backend := cache.NewMemoryCache(1000, 0)
	rc := NewResultCache(backend, 0)
	if rc.ttl != 5*time.Minute {
		t.Errorf("default ttl = %v, want 5m", rc.ttl)
	}
}

func TestNewResultCacheNilBackend(t *testing.T) {
	rc := NewResultCache(nil, time.Minute)
	if rc == nil {
		t.Fatal("NewResultCache() with nil backend returned nil")
	}
	// Should behave as disabled (Get returns nil, Set is no-op)
	rc.Set("key1", &model.SearchResults{Query: "test"})
	if rc.Get("key1") != nil {
		t.Error("nil backend should not cache anything")
	}
}

func TestResultCacheSetGet(t *testing.T) {
	rc := newTestCache(time.Minute)

	results := &model.SearchResults{
		Query:   "test",
		Results: []model.Result{{URL: "https://example.com", Title: "Test"}},
	}

	rc.Set("key1", results)

	got := rc.Get("key1")
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
	rc := newTestCache(time.Minute)

	got := rc.Get("nonexistent")
	if got != nil {
		t.Error("Get() should return nil for nonexistent key")
	}
}

func TestResultCacheGetExpired(t *testing.T) {
	rc := newTestCache(10 * time.Millisecond)

	results := &model.SearchResults{Query: "test"}
	rc.Set("key1", results)

	time.Sleep(50 * time.Millisecond)

	got := rc.Get("key1")
	if got != nil {
		t.Error("Get() should return nil for expired item")
	}
}

func TestResultCacheDelete(t *testing.T) {
	rc := newTestCache(time.Minute)

	results := &model.SearchResults{Query: "test"}
	rc.Set("key1", results)

	rc.Delete("key1")

	got := rc.Get("key1")
	if got != nil {
		t.Error("Get() should return nil after Delete()")
	}
}

func TestResultCacheDeleteNonExistent(t *testing.T) {
	rc := newTestCache(time.Minute)
	// Should not panic
	rc.Delete("nonexistent")
}

func TestResultCacheClear(t *testing.T) {
	rc := newTestCache(time.Minute)

	rc.Set("key1", &model.SearchResults{Query: "test1"})
	rc.Set("key2", &model.SearchResults{Query: "test2"})

	rc.Clear()

	if rc.Get("key1") != nil {
		t.Error("Get() should return nil after Clear()")
	}
	if rc.Get("key2") != nil {
		t.Error("Get() should return nil after Clear()")
	}
}

func TestResultCacheClearResetsStats(t *testing.T) {
	rc := newTestCache(time.Minute)

	rc.Set("key1", &model.SearchResults{Query: "test"})
	rc.Get("key1")         // hit
	rc.Get("nonexistent")  // miss

	rc.Clear()

	stats := rc.Stats()
	if stats.Hits != 0 || stats.Misses != 0 {
		t.Errorf("Clear should reset stats, Hits=%d, Misses=%d", stats.Hits, stats.Misses)
	}
}

func TestResultCacheStats(t *testing.T) {
	rc := newTestCache(time.Minute)

	rc.Set("key1", &model.SearchResults{Query: "test"})

	rc.Get("key1")         // hit
	rc.Get("key1")         // hit
	rc.Get("nonexistent")  // miss

	stats := rc.Stats()

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

func TestResultCacheStatsZeroTotal(t *testing.T) {
	rc := newTestCache(time.Minute)

	stats := rc.Stats()
	if stats.HitRate != 0 {
		t.Errorf("HitRate with no gets = %f, want 0", stats.HitRate)
	}
}

func TestResultCacheSetOverwrite(t *testing.T) {
	rc := newTestCache(time.Minute)

	rc.Set("key1", &model.SearchResults{Query: "original"})
	rc.Set("key1", &model.SearchResults{Query: "updated"})

	got := rc.Get("key1")
	if got.Query != "updated" {
		t.Errorf("Set should overwrite, Query = %q, want 'updated'", got.Query)
	}
}

func TestResultCacheSetWithEmptyResults(t *testing.T) {
	rc := newTestCache(time.Minute)

	results := &model.SearchResults{
		Query:   "test",
		Results: []model.Result{},
	}
	rc.Set("key1", results)

	got := rc.Get("key1")
	if got == nil {
		t.Fatal("Should be able to cache empty results")
	}
	if len(got.Results) != 0 {
		t.Errorf("Results count = %d, want 0", len(got.Results))
	}
}

func TestResultCacheStatsAfterOperations(t *testing.T) {
	rc := newTestCache(time.Minute)

	rc.Set("key1", &model.SearchResults{Query: "test1"})
	rc.Set("key2", &model.SearchResults{Query: "test2"})
	rc.Get("key1")         // hit
	rc.Get("key2")         // hit
	rc.Get("key3")         // miss
	rc.Get("key4")         // miss
	rc.Delete("key1")
	rc.Get("key1")         // miss (after delete)

	stats := rc.Stats()

	if stats.Hits != 2 {
		t.Errorf("Hits = %d, want 2", stats.Hits)
	}
	if stats.Misses != 3 {
		t.Errorf("Misses = %d, want 3", stats.Misses)
	}
}

func TestCacheStatsStruct(t *testing.T) {
	stats := CacheStats{
		Hits:    200,
		Misses:  50,
		HitRate: 0.8,
	}

	if stats.Hits != 200 {
		t.Errorf("Hits = %d, want 200", stats.Hits)
	}
	if stats.HitRate != 0.8 {
		t.Errorf("HitRate = %f, want 0.8", stats.HitRate)
	}
}

func TestCacheKeyFormat(t *testing.T) {
	key := cacheKey("abc123")
	if key != "search:abc123" {
		t.Errorf("cacheKey = %q, want search:abc123", key)
	}
}
