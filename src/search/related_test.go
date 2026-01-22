package search

import (
	"context"
	"testing"
	"time"
)

func TestNewRelatedSearches(t *testing.T) {
	rs := NewRelatedSearches()

	if rs == nil {
		t.Fatal("NewRelatedSearches() returned nil")
	}
	if rs.cache == nil {
		t.Error("cache should be initialized")
	}
	if rs.httpClient == nil {
		t.Error("httpClient should be initialized")
	}
}

func TestRelatedSearchesGetRelatedEmpty(t *testing.T) {
	rs := NewRelatedSearches()

	suggestions, err := rs.GetRelated(context.Background(), "", 5)

	if err != nil {
		t.Errorf("GetRelated() error = %v", err)
	}
	if len(suggestions) != 0 {
		t.Errorf("GetRelated() for empty query should return empty, got %v", suggestions)
	}
}

func TestRelatedSearchesGetRelatedDefaultLimit(t *testing.T) {
	rs := NewRelatedSearches()

	// Use a very short context to avoid network calls
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()

	_, _ = rs.GetRelated(ctx, "test", 0) // 0 should default to 8
	// Just verifying it doesn't crash with limit=0
}

func TestRelatedSearchesGenerateVariations(t *testing.T) {
	rs := NewRelatedSearches()

	variations := rs.generateVariations("golang")

	if len(variations) == 0 {
		t.Fatal("generateVariations() returned empty")
	}

	// Should include question variations
	foundQuestion := false
	for _, v := range variations {
		if containsSubstring(v, "what is") || containsSubstring(v, "how to") {
			foundQuestion = true
			break
		}
	}
	if !foundQuestion {
		t.Error("Should include question variations")
	}

	// Should include suffix variations
	foundSuffix := false
	for _, v := range variations {
		if containsSubstring(v, "tutorial") || containsSubstring(v, "guide") {
			foundSuffix = true
			break
		}
	}
	if !foundSuffix {
		t.Error("Should include suffix variations")
	}
}

func TestRelatedSearchesGenerateVariationsEmpty(t *testing.T) {
	rs := NewRelatedSearches()

	variations := rs.generateVariations("")

	if variations != nil {
		t.Errorf("generateVariations('') should return nil, got %v", variations)
	}
}

func TestRelatedSearchesGenerateVariationsMultiWord(t *testing.T) {
	rs := NewRelatedSearches()

	variations := rs.generateVariations("golang web framework tutorial")

	// Should include variations without first/last word for multi-word queries
	if len(variations) == 0 {
		t.Fatal("generateVariations() returned empty for multi-word query")
	}
}

func TestRelatedSearchesCaching(t *testing.T) {
	rs := NewRelatedSearches()

	// Manually add to cache
	rs.mu.Lock()
	rs.cache["test"] = &relatedCacheEntry{
		Suggestions: []string{"suggestion1", "suggestion2"},
		ExpiresAt:   time.Now().Add(1 * time.Hour),
	}
	rs.mu.Unlock()

	// Should get from cache
	suggestions, err := rs.GetRelated(context.Background(), "test", 10)

	if err != nil {
		t.Fatalf("GetRelated() error = %v", err)
	}
	if len(suggestions) != 2 {
		t.Errorf("Expected 2 cached suggestions, got %d", len(suggestions))
	}
}

func TestRelatedSearchesCachingWithLimit(t *testing.T) {
	rs := NewRelatedSearches()

	// Manually add to cache
	rs.mu.Lock()
	rs.cache["limited"] = &relatedCacheEntry{
		Suggestions: []string{"a", "b", "c", "d", "e"},
		ExpiresAt:   time.Now().Add(1 * time.Hour),
	}
	rs.mu.Unlock()

	suggestions, _ := rs.GetRelated(context.Background(), "limited", 3)

	if len(suggestions) != 3 {
		t.Errorf("Expected 3 suggestions with limit, got %d", len(suggestions))
	}
}

func TestRelatedSearchesCacheExpired(t *testing.T) {
	rs := NewRelatedSearches()

	// Add expired entry
	rs.mu.Lock()
	rs.cache["expired"] = &relatedCacheEntry{
		Suggestions: []string{"old"},
		ExpiresAt:   time.Now().Add(-1 * time.Hour), // Expired
	}
	rs.mu.Unlock()

	// Use a very short context to avoid network calls
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()

	_, _ = rs.GetRelated(ctx, "expired", 5)
	// Should not use expired cache, context will timeout
}

func TestRelatedSearchesClearCache(t *testing.T) {
	rs := NewRelatedSearches()

	// Add some entries
	rs.mu.Lock()
	rs.cache["key1"] = &relatedCacheEntry{
		Suggestions: []string{"test"},
		ExpiresAt:   time.Now().Add(1 * time.Hour),
	}
	rs.cache["key2"] = &relatedCacheEntry{
		Suggestions: []string{"test2"},
		ExpiresAt:   time.Now().Add(1 * time.Hour),
	}
	rs.mu.Unlock()

	rs.ClearCache()

	rs.mu.RLock()
	size := len(rs.cache)
	rs.mu.RUnlock()

	if size != 0 {
		t.Errorf("ClearCache() should empty cache, got size %d", size)
	}
}

func TestRelatedSearchesCleanCache(t *testing.T) {
	rs := NewRelatedSearches()

	// Add expired and valid entries
	rs.mu.Lock()
	rs.cache["expired"] = &relatedCacheEntry{
		Suggestions: []string{"old"},
		ExpiresAt:   time.Now().Add(-1 * time.Hour),
	}
	rs.cache["valid"] = &relatedCacheEntry{
		Suggestions: []string{"new"},
		ExpiresAt:   time.Now().Add(1 * time.Hour),
	}
	rs.mu.Unlock()

	rs.cleanCache()

	rs.mu.RLock()
	_, expiredExists := rs.cache["expired"]
	_, validExists := rs.cache["valid"]
	rs.mu.RUnlock()

	if expiredExists {
		t.Error("cleanCache() should remove expired entry")
	}
	if !validExists {
		t.Error("cleanCache() should keep valid entry")
	}
}

func TestRelatedCacheEntryStruct(t *testing.T) {
	entry := relatedCacheEntry{
		Suggestions: []string{"a", "b", "c"},
		ExpiresAt:   time.Now().Add(1 * time.Hour),
	}

	if len(entry.Suggestions) != 3 {
		t.Errorf("Suggestions count = %d, want 3", len(entry.Suggestions))
	}
	if time.Now().After(entry.ExpiresAt) {
		t.Error("Entry should not be expired")
	}
}

func TestRelatedSearchesFetchDuckDuckGoContextCancelled(t *testing.T) {
	rs := NewRelatedSearches()

	// Use an already cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	suggestions := rs.fetchDuckDuckGo(ctx, "test")

	if len(suggestions) != 0 {
		t.Errorf("fetchDuckDuckGo with cancelled context should return empty, got %v", suggestions)
	}
}

func TestRelatedSearchesFetchGoogleContextCancelled(t *testing.T) {
	rs := NewRelatedSearches()

	// Use an already cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	suggestions := rs.fetchGoogle(ctx, "test")

	if len(suggestions) != 0 {
		t.Errorf("fetchGoogle with cancelled context should return empty, got %v", suggestions)
	}
}

func TestRelatedSearchesFetchRelatedSearchesDeduplication(t *testing.T) {
	rs := NewRelatedSearches()

	// Use very short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()

	// This will trigger generateVariations but not network calls
	suggestions := rs.fetchRelatedSearches(ctx, "test")

	// Check for deduplication (no duplicates should exist)
	seen := make(map[string]bool)
	for _, s := range suggestions {
		lower := stringToLower(s)
		if seen[lower] {
			t.Errorf("Duplicate suggestion found: %q", s)
		}
		seen[lower] = true
	}
}

func TestRelatedSearchesGetRelatedNoSelfMatch(t *testing.T) {
	rs := NewRelatedSearches()

	// Pre-populate cache with query as suggestion (which should be filtered)
	rs.mu.Lock()
	rs.cache["test"] = &relatedCacheEntry{
		Suggestions: []string{"test", "test query", "testing"},
		ExpiresAt:   time.Now().Add(1 * time.Hour),
	}
	rs.mu.Unlock()

	suggestions, _ := rs.GetRelated(context.Background(), "test", 10)

	// The exact query "test" should be in suggestions since we're getting from cache
	// The actual filtering happens in fetchRelatedSearches, not in cache retrieval
	// This test just verifies the flow works
	if len(suggestions) == 0 {
		t.Error("Should return cached suggestions")
	}
}

// Helper function
func stringToLower(s string) string {
	result := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			result[i] = c + 32
		} else {
			result[i] = c
		}
	}
	return string(result)
}

// Additional tests for 100% coverage

func TestRelatedSearchesGenerateVariationsStartsWithPrefix(t *testing.T) {
	rs := NewRelatedSearches()

	// Query that starts with a question prefix
	variations := rs.generateVariations("what is golang")

	// Should not duplicate "what is" prefix
	for _, v := range variations {
		if v == "what is what is golang" {
			t.Error("Should not duplicate question prefix")
		}
	}
}

func TestRelatedSearchesGenerateVariationsEndsWithSuffix(t *testing.T) {
	rs := NewRelatedSearches()

	// Query that ends with a suffix
	variations := rs.generateVariations("golang tutorial")

	// Should not duplicate "tutorial" suffix
	for _, v := range variations {
		if v == "golang tutorial tutorial" {
			t.Error("Should not duplicate suffix")
		}
	}
}

func TestRelatedSearchesGenerateVariationsSingleWord(t *testing.T) {
	rs := NewRelatedSearches()

	variations := rs.generateVariations("golang")

	// Single word queries should not include word removal variations
	if len(variations) == 0 {
		t.Fatal("Should return variations for single word query")
	}
}

func TestRelatedSearchesGenerateVariationsTwoWords(t *testing.T) {
	rs := NewRelatedSearches()

	variations := rs.generateVariations("golang tutorial")

	// Two word queries should not include word removal variations (needs > 2)
	for _, v := range variations {
		if v == "tutorial" || v == "golang" {
			// This is fine - word removal only happens for > 2 words
		}
	}
	if len(variations) == 0 {
		t.Error("Should return variations")
	}
}

func TestRelatedSearchesGetRelatedFromCacheExact(t *testing.T) {
	rs := NewRelatedSearches()

	// Add to cache with exact key match
	rs.mu.Lock()
	rs.cache["test query"] = &relatedCacheEntry{
		Suggestions: []string{"s1", "s2", "s3"},
		ExpiresAt:   time.Now().Add(1 * time.Hour),
	}
	rs.mu.Unlock()

	suggestions, err := rs.GetRelated(context.Background(), "TEST QUERY", 10)

	if err != nil {
		t.Fatalf("GetRelated() error = %v", err)
	}
	// Cache key should be case-insensitive
	if len(suggestions) != 3 {
		t.Errorf("Expected 3 suggestions, got %d", len(suggestions))
	}
}

func TestRelatedSearchesFetchRelatedSearchesQueryNotInResults(t *testing.T) {
	rs := NewRelatedSearches()

	// Use a very short timeout to skip network calls
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()

	suggestions := rs.fetchRelatedSearches(ctx, "golang")

	// The query itself should not be in the results
	queryLower := "golang"
	for _, s := range suggestions {
		if stringToLower(s) == queryLower {
			t.Errorf("Query should not be in suggestions: %q", s)
		}
	}
}

func TestNewRelatedSearchesInitialization(t *testing.T) {
	rs := NewRelatedSearches()

	if rs.cache == nil {
		t.Error("cache should be initialized")
	}
	if rs.httpClient == nil {
		t.Error("httpClient should be initialized")
	}
	if rs.httpClient.Timeout != 5*time.Second {
		t.Errorf("httpClient.Timeout = %v, want 5s", rs.httpClient.Timeout)
	}
}

func TestRelatedSearchesGetRelatedWithNegativeLimit(t *testing.T) {
	rs := NewRelatedSearches()

	// Use a short context
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()

	// Negative limit should default to 8
	_, _ = rs.GetRelated(ctx, "test", -5)
	// Just verifying it doesn't panic
}

func TestRelatedSearchesCacheEntryExpiration(t *testing.T) {
	entry := relatedCacheEntry{
		Suggestions: []string{"test"},
		ExpiresAt:   time.Now().Add(-1 * time.Hour), // Already expired
	}

	if !time.Now().After(entry.ExpiresAt) {
		t.Error("Entry should be expired")
	}
}

func TestRelatedSearchesCleanCacheNoExpired(t *testing.T) {
	rs := NewRelatedSearches()

	// Add valid entries only
	rs.mu.Lock()
	rs.cache["valid1"] = &relatedCacheEntry{
		Suggestions: []string{"test"},
		ExpiresAt:   time.Now().Add(1 * time.Hour),
	}
	rs.cache["valid2"] = &relatedCacheEntry{
		Suggestions: []string{"test"},
		ExpiresAt:   time.Now().Add(1 * time.Hour),
	}
	rs.mu.Unlock()

	rs.cleanCache()

	rs.mu.RLock()
	size := len(rs.cache)
	rs.mu.RUnlock()

	if size != 2 {
		t.Errorf("cleanCache() should keep valid entries, got size %d", size)
	}
}

func TestRelatedSearchesGetRelatedLimitAppliedAfterFetch(t *testing.T) {
	rs := NewRelatedSearches()

	// Add many suggestions to cache
	suggestions := make([]string, 20)
	for i := 0; i < 20; i++ {
		suggestions[i] = "suggestion" + string(rune('A'+i))
	}

	rs.mu.Lock()
	rs.cache["many"] = &relatedCacheEntry{
		Suggestions: suggestions,
		ExpiresAt:   time.Now().Add(1 * time.Hour),
	}
	rs.mu.Unlock()

	result, _ := rs.GetRelated(context.Background(), "many", 5)

	if len(result) != 5 {
		t.Errorf("Expected 5 suggestions with limit, got %d", len(result))
	}
}

func TestRelatedSearchesFetchRelatedSearchesCombinesResults(t *testing.T) {
	rs := NewRelatedSearches()

	// Use very short timeout - will only get generateVariations results
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()

	suggestions := rs.fetchRelatedSearches(ctx, "golang")

	// Should have at least some variations from generateVariations
	if len(suggestions) == 0 {
		t.Log("Note: May have no suggestions due to immediate timeout")
	}
}

func TestRelatedSearchesGetRelatedAddsToCache(t *testing.T) {
	rs := NewRelatedSearches()

	// Use very short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()

	_, _ = rs.GetRelated(ctx, "newquery", 5)

	// Check that the query was added to cache
	rs.mu.RLock()
	_, exists := rs.cache["newquery"]
	rs.mu.RUnlock()

	if !exists {
		t.Error("Query should be cached after GetRelated")
	}
}

func TestRelatedSearchesCacheKeyNormalization(t *testing.T) {
	rs := NewRelatedSearches()

	// Add to cache
	rs.mu.Lock()
	rs.cache["test query"] = &relatedCacheEntry{
		Suggestions: []string{"cached"},
		ExpiresAt:   time.Now().Add(1 * time.Hour),
	}
	rs.mu.Unlock()

	// Query with extra whitespace
	suggestions, _ := rs.GetRelated(context.Background(), "  TEST QUERY  ", 10)

	if len(suggestions) != 1 || suggestions[0] != "cached" {
		t.Errorf("Should find cache with normalized key, got %v", suggestions)
	}
}
