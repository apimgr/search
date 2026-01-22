package search

import (
	"context"
	"testing"
	"time"

	"github.com/apimgr/search/src/model"
)

// Mock engine for testing
type mockEngine struct {
	*BaseEngine
	searchResults []model.Result
	searchError   error
}

func newMockEngine(name string, category model.Category, enabled bool) *mockEngine {
	cfg := &model.EngineConfig{
		Name:        name,
		DisplayName: name + " Engine",
		Enabled:     enabled,
		Categories:  []string{string(category)},
		Priority:    10,
	}
	return &mockEngine{
		BaseEngine: NewBaseEngine(cfg),
	}
}

func (m *mockEngine) Search(ctx context.Context, query *model.Query) ([]model.Result, error) {
	if m.searchError != nil {
		return nil, m.searchError
	}
	return m.searchResults, nil
}

func (m *mockEngine) SetResults(results []model.Result) {
	m.searchResults = results
}

func (m *mockEngine) SetError(err error) {
	m.searchError = err
}

// Tests for BaseEngine

func TestNewBaseEngine(t *testing.T) {
	cfg := &model.EngineConfig{
		Name:        "test",
		DisplayName: "Test Engine",
		Enabled:     true,
		Priority:    50,
		Categories:  []string{string(model.CategoryGeneral)},
	}

	engine := NewBaseEngine(cfg)

	if engine == nil {
		t.Fatal("NewBaseEngine() returned nil")
	}
	if engine.config != cfg {
		t.Error("Config not set correctly")
	}
}

func TestBaseEngineName(t *testing.T) {
	cfg := &model.EngineConfig{Name: "testengine"}
	engine := NewBaseEngine(cfg)

	if engine.Name() != "testengine" {
		t.Errorf("Name() = %q, want %q", engine.Name(), "testengine")
	}
}

func TestBaseEngineDisplayName(t *testing.T) {
	cfg := &model.EngineConfig{DisplayName: "Test Engine Display"}
	engine := NewBaseEngine(cfg)

	if engine.DisplayName() != "Test Engine Display" {
		t.Errorf("DisplayName() = %q, want %q", engine.DisplayName(), "Test Engine Display")
	}
}

func TestBaseEngineIsEnabled(t *testing.T) {
	cfg := &model.EngineConfig{Enabled: true}
	engine := NewBaseEngine(cfg)

	if !engine.IsEnabled() {
		t.Error("IsEnabled() should return true")
	}
}

func TestBaseEngineGetPriority(t *testing.T) {
	cfg := &model.EngineConfig{Priority: 75}
	engine := NewBaseEngine(cfg)

	if engine.GetPriority() != 75 {
		t.Errorf("GetPriority() = %d, want 75", engine.GetPriority())
	}
}

func TestBaseEngineSupportsCategory(t *testing.T) {
	cfg := &model.EngineConfig{
		Categories: []string{string(model.CategoryGeneral), string(model.CategoryImages)},
	}
	engine := NewBaseEngine(cfg)

	if !engine.SupportsCategory(model.CategoryGeneral) {
		t.Error("SupportsCategory(General) should return true")
	}
	if !engine.SupportsCategory(model.CategoryImages) {
		t.Error("SupportsCategory(Images) should return true")
	}
	if engine.SupportsCategory(model.CategoryNews) {
		t.Error("SupportsCategory(News) should return false")
	}
}

func TestBaseEngineGetConfig(t *testing.T) {
	cfg := &model.EngineConfig{Name: "test"}
	engine := NewBaseEngine(cfg)

	if engine.GetConfig() != cfg {
		t.Error("GetConfig() returned wrong config")
	}
}

// Tests for Aggregator

func TestNewAggregator(t *testing.T) {
	engines := []Engine{newMockEngine("test", model.CategoryGeneral, true)}
	config := AggregatorConfig{
		Timeout:      10 * time.Second,
		CacheEnabled: true,
		CacheTTL:     5 * time.Minute,
		MaxCacheSize: 100,
	}

	agg := NewAggregator(engines, config)

	if agg == nil {
		t.Fatal("NewAggregator() returned nil")
	}
	if len(agg.engines) != 1 {
		t.Errorf("engines count = %d, want 1", len(agg.engines))
	}
	if agg.timeout != 10*time.Second {
		t.Errorf("timeout = %v, want 10s", agg.timeout)
	}
	if agg.cache == nil {
		t.Error("cache should be created when enabled")
	}
}

func TestNewAggregatorDefaults(t *testing.T) {
	engines := []Engine{newMockEngine("test", model.CategoryGeneral, true)}
	config := AggregatorConfig{} // All defaults

	agg := NewAggregator(engines, config)

	if agg.timeout != 30*time.Second {
		t.Errorf("default timeout = %v, want 30s", agg.timeout)
	}
	if agg.cacheTTL != 5*time.Minute {
		t.Errorf("default cacheTTL = %v, want 5m", agg.cacheTTL)
	}
}

func TestNewAggregatorSimple(t *testing.T) {
	engines := []Engine{newMockEngine("test", model.CategoryGeneral, true)}

	agg := NewAggregatorSimple(engines, 15*time.Second)

	if agg == nil {
		t.Fatal("NewAggregatorSimple() returned nil")
	}
	if agg.timeout != 15*time.Second {
		t.Errorf("timeout = %v, want 15s", agg.timeout)
	}
	if agg.cacheEnabled {
		t.Error("cache should be disabled for simple aggregator")
	}
}

func TestAggregatorSearchNoEngines(t *testing.T) {
	agg := NewAggregatorSimple([]Engine{}, 10*time.Second)

	query := &model.Query{Text: "test", Category: model.CategoryGeneral}
	_, err := agg.Search(context.Background(), query)

	if err != model.ErrNoEngines {
		t.Errorf("Search() error = %v, want ErrNoEngines", err)
	}
}

func TestAggregatorSearchWithResults(t *testing.T) {
	engine := newMockEngine("test", model.CategoryGeneral, true)
	engine.SetResults([]model.Result{
		{URL: "https://example.com/1", Title: "Result 1"},
		{URL: "https://example.com/2", Title: "Result 2"},
	})

	agg := NewAggregatorSimple([]Engine{engine}, 10*time.Second)

	query := &model.Query{Text: "test", Category: model.CategoryGeneral}
	results, err := agg.Search(context.Background(), query)

	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if results == nil {
		t.Fatal("Search() returned nil results")
	}
	if len(results.Results) != 2 {
		t.Errorf("results count = %d, want 2", len(results.Results))
	}
}

func TestAggregatorSearchValidationError(t *testing.T) {
	engine := newMockEngine("test", model.CategoryGeneral, true)
	agg := NewAggregatorSimple([]Engine{engine}, 10*time.Second)

	// Empty query should fail validation
	query := &model.Query{Text: "", Category: model.CategoryGeneral}
	_, err := agg.Search(context.Background(), query)

	if err == nil {
		t.Error("Search() should return error for empty query")
	}
}

func TestAggregatorSearchCaching(t *testing.T) {
	engine := newMockEngine("test", model.CategoryGeneral, true)
	engine.SetResults([]model.Result{
		{URL: "https://example.com/1", Title: "Result 1"},
	})

	config := AggregatorConfig{
		Timeout:      10 * time.Second,
		CacheEnabled: true,
		CacheTTL:     5 * time.Minute,
		MaxCacheSize: 100,
	}
	agg := NewAggregator([]Engine{engine}, config)

	query := &model.Query{Text: "cached query", Category: model.CategoryGeneral}

	// First search
	results1, _ := agg.Search(context.Background(), query)

	// Second search (should be cached)
	results2, _ := agg.Search(context.Background(), query)

	// Cached result should have nearly instant search time
	if results2.SearchTime > 0.01 {
		t.Logf("Cached search time = %v (may indicate cache miss)", results2.SearchTime)
	}

	if len(results1.Results) != len(results2.Results) {
		t.Error("Cached results should match original")
	}
}

func TestAggregatorGenerateCacheKey(t *testing.T) {
	agg := NewAggregatorSimple([]Engine{}, 10*time.Second)

	query1 := &model.Query{Text: "test", Category: model.CategoryGeneral, Language: "en"}
	query2 := &model.Query{Text: "test", Category: model.CategoryGeneral, Language: "en"}
	query3 := &model.Query{Text: "different", Category: model.CategoryGeneral, Language: "en"}

	key1 := agg.generateCacheKey(query1)
	key2 := agg.generateCacheKey(query2)
	key3 := agg.generateCacheKey(query3)

	if key1 != key2 {
		t.Error("Same queries should produce same cache key")
	}
	if key1 == key3 {
		t.Error("Different queries should produce different cache keys")
	}
}

func TestAggregatorFilterEngines(t *testing.T) {
	generalEngine := newMockEngine("general", model.CategoryGeneral, true)
	imagesEngine := newMockEngine("images", model.CategoryImages, true)
	newsEngine := newMockEngine("news", model.CategoryNews, true)

	agg := NewAggregatorSimple([]Engine{generalEngine, imagesEngine, newsEngine}, 10*time.Second)

	// Query for general category should only return the general engine
	query := &model.Query{Text: "test", Category: model.CategoryGeneral}
	engines := agg.filterEngines(query)

	if len(engines) != 1 {
		t.Errorf("filterEngines() count = %d, want 1", len(engines))
	}
	if engines[0].Name() != "general" {
		t.Errorf("filterEngines() returned %q, want 'general'", engines[0].Name())
	}
}

func TestAggregatorFilterEnginesExplicitSelection(t *testing.T) {
	engine1 := newMockEngine("engine1", model.CategoryGeneral, true)
	engine2 := newMockEngine("engine2", model.CategoryGeneral, true)

	agg := NewAggregatorSimple([]Engine{engine1, engine2}, 10*time.Second)

	query := &model.Query{
		Text:     "test",
		Category: model.CategoryGeneral,
		Engines:  []string{"engine1"},
	}
	engines := agg.filterEngines(query)

	if len(engines) != 1 {
		t.Errorf("filterEngines() count = %d, want 1", len(engines))
	}
	if engines[0].Name() != "engine1" {
		t.Errorf("Selected engine = %q, want engine1", engines[0].Name())
	}
}

func TestAggregatorFilterEnginesExclusion(t *testing.T) {
	engine1 := newMockEngine("engine1", model.CategoryGeneral, true)
	engine2 := newMockEngine("engine2", model.CategoryGeneral, true)

	agg := NewAggregatorSimple([]Engine{engine1, engine2}, 10*time.Second)

	query := &model.Query{
		Text:           "test",
		Category:       model.CategoryGeneral,
		ExcludeEngines: []string{"engine1"},
	}
	engines := agg.filterEngines(query)

	if len(engines) != 1 {
		t.Errorf("filterEngines() count = %d, want 1", len(engines))
	}
	if engines[0].Name() != "engine2" {
		t.Errorf("Remaining engine = %q, want engine2", engines[0].Name())
	}
}

func TestAggregatorApplyFilters(t *testing.T) {
	agg := NewAggregatorSimple([]Engine{}, 10*time.Second)

	results := []model.Result{
		{URL: "https://example.com/1", Title: "Keep this"},
		{URL: "https://excluded.com/2", Title: "Exclude this"},
		{URL: "https://example.com/3", Title: "Also keep"},
	}

	query := &model.Query{
		Text:        "test",
		ExcludeSite: "excluded.com",
	}

	filtered := agg.applyFilters(results, query)

	if len(filtered) != 2 {
		t.Errorf("applyFilters() count = %d, want 2", len(filtered))
	}
}

func TestAggregatorApplyFiltersExcludeTerms(t *testing.T) {
	agg := NewAggregatorSimple([]Engine{}, 10*time.Second)

	results := []model.Result{
		{URL: "https://example.com/1", Title: "Good result", Content: "Some content"},
		{URL: "https://example.com/2", Title: "Bad result", Content: "Contains spam"},
	}

	query := &model.Query{
		Text:         "test",
		ExcludeTerms: []string{"spam"},
	}

	filtered := agg.applyFilters(results, query)

	if len(filtered) != 1 {
		t.Errorf("applyFilters() count = %d, want 1", len(filtered))
	}
}

func TestAggregatorApplyFiltersDateRange(t *testing.T) {
	agg := NewAggregatorSimple([]Engine{}, 10*time.Second)

	now := time.Now()
	results := []model.Result{
		{URL: "https://example.com/1", Title: "Recent", PublishedAt: now.AddDate(0, 0, -1)},
		{URL: "https://example.com/2", Title: "Old", PublishedAt: now.AddDate(-1, 0, 0)},
		{URL: "https://example.com/3", Title: "No date"},
	}

	query := &model.Query{
		Text:      "test",
		DateAfter: now.AddDate(0, -1, 0).Format("2006-01-02"),
	}

	filtered := agg.applyFilters(results, query)

	// Should keep recent and no-date results
	if len(filtered) != 2 {
		t.Errorf("applyFilters() count = %d, want 2", len(filtered))
	}
}

// Tests for deduplication and sorting

func TestDeduplicateResults(t *testing.T) {
	results := []model.Result{
		{URL: "https://example.com/1", Title: "First", Engine: "google"},
		{URL: "https://example.com/1", Title: "First Duplicate", Engine: "bing", Thumbnail: "thumb.jpg"},
		{URL: "https://example.com/2", Title: "Second", Engine: "google"},
	}

	deduped := deduplicateResults(results)

	if len(deduped) != 2 {
		t.Errorf("deduplicateResults() count = %d, want 2", len(deduped))
	}

	// Check that duplicate info was merged
	for _, r := range deduped {
		if r.URL == "https://example.com/1" {
			if r.DuplicateCount != 2 {
				t.Errorf("DuplicateCount = %d, want 2", r.DuplicateCount)
			}
			if r.Thumbnail != "thumb.jpg" {
				t.Error("Thumbnail should be merged from duplicate")
			}
		}
	}
}

func TestDeduplicateResultsMergesContent(t *testing.T) {
	results := []model.Result{
		{URL: "https://example.com/1", Title: "Result", Content: "Short"},
		{URL: "https://example.com/1", Title: "Result", Content: "Much longer content here"},
	}

	deduped := deduplicateResults(results)

	if len(deduped) != 1 {
		t.Fatalf("Expected 1 result, got %d", len(deduped))
	}
	if deduped[0].Content != "Much longer content here" {
		t.Error("Should keep longer content")
	}
}

func TestSortResultsRelevance(t *testing.T) {
	results := []model.Result{
		{URL: "1", Score: 50},
		{URL: "2", Score: 100},
		{URL: "3", Score: 75},
	}

	sortResults(results, model.SortRelevance)

	if results[0].URL != "2" {
		t.Errorf("First result URL = %q, want 2 (highest score)", results[0].URL)
	}
	if results[2].URL != "1" {
		t.Errorf("Last result URL = %q, want 1 (lowest score)", results[2].URL)
	}
}

func TestSortResultsDate(t *testing.T) {
	now := time.Now()
	results := []model.Result{
		{URL: "1", PublishedAt: now.AddDate(0, -1, 0)}, // 1 month ago
		{URL: "2", PublishedAt: now},                   // Now
		{URL: "3", PublishedAt: now.AddDate(0, 0, -1)}, // 1 day ago
	}

	sortResults(results, model.SortDate)

	if results[0].URL != "2" {
		t.Errorf("First result URL = %q, want 2 (newest)", results[0].URL)
	}
}

func TestSortResultsDateAsc(t *testing.T) {
	now := time.Now()
	results := []model.Result{
		{URL: "1", PublishedAt: now.AddDate(0, -1, 0)}, // 1 month ago
		{URL: "2", PublishedAt: now},                   // Now
		{URL: "3", PublishedAt: now.AddDate(0, 0, -1)}, // 1 day ago
	}

	sortResults(results, model.SortDateAsc)

	if results[0].URL != "1" {
		t.Errorf("First result URL = %q, want 1 (oldest)", results[0].URL)
	}
}

func TestSortResultsPopularity(t *testing.T) {
	results := []model.Result{
		{URL: "1", Popularity: 10, ViewCount: 100},
		{URL: "2", Popularity: 50, ViewCount: 200},
		{URL: "3", Popularity: 30, ViewCount: 150},
	}

	sortResults(results, model.SortPopularity)

	if results[0].URL != "2" {
		t.Errorf("First result URL = %q, want 2 (most popular)", results[0].URL)
	}
}

func TestSortResultsRandom(t *testing.T) {
	results := []model.Result{
		{URL: "1"}, {URL: "2"}, {URL: "3"}, {URL: "4"}, {URL: "5"},
	}

	// Store original order
	original := make([]string, len(results))
	for i, r := range results {
		original[i] = r.URL
	}

	// Sort randomly multiple times to check randomness
	sortResults(results, model.SortRandom)

	// Just verify it doesn't crash - randomness is probabilistic
	if len(results) != 5 {
		t.Error("Random sort should preserve result count")
	}
}

func TestRankResults(t *testing.T) {
	results := []model.Result{
		{URL: "1", Score: 25},
		{URL: "2", Score: 100},
		{URL: "3", Score: 50},
	}

	rankResults(results)

	// rankResults is backwards compatible with sortResults(relevance)
	if results[0].URL != "2" {
		t.Errorf("First result URL = %q, want 2", results[0].URL)
	}
}

// Tests for applyOperators

func TestAggregatorApplyOperators(t *testing.T) {
	agg := NewAggregatorSimple([]Engine{}, 10*time.Second)

	query := &model.Query{
		Text:     "test",
		Language: "en", // default
	}

	ops := &SearchOperators{
		Site:       "example.com",
		FileType:   "pdf",
		Language:   "de",
		Before:     "2024-01-01",
		After:      "2023-01-01",
		InURL:      "docs",
		InTitle:    "tutorial",
		InText:     "golang",
		Source:     "nytimes",
	}

	agg.applyOperators(query, ops)

	if query.Site != "example.com" {
		t.Errorf("Site = %q, want example.com", query.Site)
	}
	if query.FileType != "pdf" {
		t.Errorf("FileType = %q, want pdf", query.FileType)
	}
	if query.Language != "de" {
		t.Errorf("Language = %q, want de", query.Language)
	}
	if query.DateBefore != "2024-01-01" {
		t.Errorf("DateBefore = %q, want 2024-01-01", query.DateBefore)
	}
	if query.DateAfter != "2023-01-01" {
		t.Errorf("DateAfter = %q, want 2023-01-01", query.DateAfter)
	}
}

func TestAggregatorApplyOperatorsDoesNotOverwrite(t *testing.T) {
	agg := NewAggregatorSimple([]Engine{}, 10*time.Second)

	query := &model.Query{
		Text:     "test",
		Site:     "original.com", // Already set
		Language: "fr",           // Not default
	}

	ops := &SearchOperators{
		Site:     "new.com",
		Language: "de",
	}

	agg.applyOperators(query, ops)

	// Should NOT overwrite existing values
	if query.Site != "original.com" {
		t.Errorf("Site should not be overwritten, got %q", query.Site)
	}
	if query.Language != "fr" {
		t.Errorf("Language should not be overwritten, got %q", query.Language)
	}
}

// Additional tests for 100% coverage

func TestAggregatorApplyOperatorsAllFields(t *testing.T) {
	agg := NewAggregatorSimple([]Engine{}, 10*time.Second)

	query := &model.Query{
		Text:     "test",
		Language: "en", // default
	}

	ops := &SearchOperators{
		ExcludeSite:  "spam.com",
		FileTypes:    []string{"pdf", "doc"},
		InURL:        "docs",
		InTitle:      "guide",
		InText:       "tutorial",
		ExactPhrases: []string{"exact match"},
		ExcludeTerms: []string{"exclude"},
	}

	agg.applyOperators(query, ops)

	if query.ExcludeSite != "spam.com" {
		t.Errorf("ExcludeSite = %q, want spam.com", query.ExcludeSite)
	}
	if len(query.FileTypes) != 2 {
		t.Errorf("FileTypes count = %d, want 2", len(query.FileTypes))
	}
	if query.InURL != "docs" {
		t.Errorf("InURL = %q, want docs", query.InURL)
	}
	if query.InTitle != "guide" {
		t.Errorf("InTitle = %q, want guide", query.InTitle)
	}
	if query.InText != "tutorial" {
		t.Errorf("InText = %q, want tutorial", query.InText)
	}
	if len(query.ExactPhrases) != 1 {
		t.Errorf("ExactPhrases count = %d, want 1", len(query.ExactPhrases))
	}
	if len(query.ExcludeTerms) != 1 {
		t.Errorf("ExcludeTerms count = %d, want 1", len(query.ExcludeTerms))
	}
}

func TestAggregatorApplyOperatorsDoesNotOverwriteExisting(t *testing.T) {
	agg := NewAggregatorSimple([]Engine{}, 10*time.Second)

	query := &model.Query{
		Text:         "test",
		Language:     "en",
		ExcludeSite:  "existing.com",
		FileTypes:    []string{"existing"},
		InURL:        "existing",
		InTitle:      "existing",
		InText:       "existing",
		ExactPhrases: []string{"existing"},
		ExcludeTerms: []string{"existing"},
		DateBefore:   "2024-01-01",
		DateAfter:    "2023-01-01",
		NewsSource:   "existing",
	}

	ops := &SearchOperators{
		ExcludeSite:  "new.com",
		FileTypes:    []string{"new"},
		InURL:        "new",
		InTitle:      "new",
		InText:       "new",
		ExactPhrases: []string{"new"},
		ExcludeTerms: []string{"new"},
		Before:       "2025-01-01",
		After:        "2024-01-01",
		Source:       "new",
	}

	agg.applyOperators(query, ops)

	// All values should remain as "existing" since they were already set
	if query.ExcludeSite != "existing.com" {
		t.Errorf("ExcludeSite should not be overwritten")
	}
	if query.InURL != "existing" {
		t.Errorf("InURL should not be overwritten")
	}
	if query.InTitle != "existing" {
		t.Errorf("InTitle should not be overwritten")
	}
	if query.InText != "existing" {
		t.Errorf("InText should not be overwritten")
	}
	if query.DateBefore != "2024-01-01" {
		t.Errorf("DateBefore should not be overwritten")
	}
	if query.DateAfter != "2023-01-01" {
		t.Errorf("DateAfter should not be overwritten")
	}
	if query.NewsSource != "existing" {
		t.Errorf("NewsSource should not be overwritten")
	}
}

func TestAggregatorApplyFiltersDateBefore(t *testing.T) {
	agg := NewAggregatorSimple([]Engine{}, 10*time.Second)

	now := time.Now()
	results := []model.Result{
		{URL: "https://example.com/1", Title: "Old", PublishedAt: now.AddDate(-1, 0, 0)},
		{URL: "https://example.com/2", Title: "Recent", PublishedAt: now.AddDate(0, 0, -1)},
		{URL: "https://example.com/3", Title: "No date"},
	}

	query := &model.Query{
		Text:       "test",
		DateBefore: now.AddDate(0, -6, 0).Format("2006-01-02"), // 6 months ago
	}

	filtered := agg.applyFilters(results, query)

	// Should keep old and no-date results
	if len(filtered) != 2 {
		t.Errorf("applyFilters() count = %d, want 2", len(filtered))
	}
}

func TestAggregatorApplyFiltersInvalidDateFormat(t *testing.T) {
	agg := NewAggregatorSimple([]Engine{}, 10*time.Second)

	now := time.Now()
	results := []model.Result{
		{URL: "https://example.com/1", Title: "Result", PublishedAt: now},
	}

	// Invalid date formats - should not filter anything due to parse error
	query := &model.Query{
		Text:       "test",
		DateBefore: "invalid-date",
		DateAfter:  "also-invalid",
	}

	filtered := agg.applyFilters(results, query)

	// All results should remain since date parsing fails
	if len(filtered) != 1 {
		t.Errorf("applyFilters() with invalid dates should keep results, got %d", len(filtered))
	}
}

func TestAggregatorApplyFiltersEmptyResults(t *testing.T) {
	agg := NewAggregatorSimple([]Engine{}, 10*time.Second)

	results := []model.Result{}
	query := &model.Query{
		Text:        "test",
		ExcludeSite: "example.com",
	}

	filtered := agg.applyFilters(results, query)

	if len(filtered) != 0 {
		t.Errorf("applyFilters() on empty should return empty, got %d", len(filtered))
	}
}

func TestDeduplicateResultsMergesAuthor(t *testing.T) {
	results := []model.Result{
		{URL: "https://example.com/1", Title: "Result", Author: ""},
		{URL: "https://example.com/1", Title: "Result", Author: "John Doe"},
	}

	deduped := deduplicateResults(results)

	if len(deduped) != 1 {
		t.Fatalf("Expected 1 result, got %d", len(deduped))
	}
	if deduped[0].Author != "John Doe" {
		t.Errorf("Author should be merged, got %q", deduped[0].Author)
	}
}

func TestDeduplicateResultsMergesPublishedAt(t *testing.T) {
	now := time.Now()
	earlier := now.Add(-24 * time.Hour)

	results := []model.Result{
		{URL: "https://example.com/1", Title: "Result", PublishedAt: now},
		{URL: "https://example.com/1", Title: "Result", PublishedAt: earlier},
	}

	deduped := deduplicateResults(results)

	if len(deduped) != 1 {
		t.Fatalf("Expected 1 result, got %d", len(deduped))
	}
	// Should keep the earlier date
	if !deduped[0].PublishedAt.Equal(earlier) {
		t.Errorf("PublishedAt should be the earlier date")
	}
}

func TestDeduplicateResultsMergesPublishedAtFromZero(t *testing.T) {
	now := time.Now()

	results := []model.Result{
		{URL: "https://example.com/1", Title: "Result"}, // Zero PublishedAt
		{URL: "https://example.com/1", Title: "Result", PublishedAt: now},
	}

	deduped := deduplicateResults(results)

	if len(deduped) != 1 {
		t.Fatalf("Expected 1 result, got %d", len(deduped))
	}
	if deduped[0].PublishedAt.IsZero() {
		t.Error("PublishedAt should be set from duplicate")
	}
}

func TestDeduplicateResultsMergesRelevance(t *testing.T) {
	results := []model.Result{
		{URL: "https://example.com/1", Title: "Result", Relevance: 0.8},
		{URL: "https://example.com/1", Title: "Result", Relevance: 0.6},
	}

	deduped := deduplicateResults(results)

	if len(deduped) != 1 {
		t.Fatalf("Expected 1 result, got %d", len(deduped))
	}
	// Should average the relevance: (0.8 + 0.6) / 2 = 0.7
	expected := 0.7
	if deduped[0].Relevance < expected-0.01 || deduped[0].Relevance > expected+0.01 {
		t.Errorf("Relevance = %f, want %f", deduped[0].Relevance, expected)
	}
}

func TestDeduplicateResultsAccumulatesPopularity(t *testing.T) {
	results := []model.Result{
		{URL: "https://example.com/1", Title: "Result", Popularity: 100},
		{URL: "https://example.com/1", Title: "Result", Popularity: 50},
	}

	deduped := deduplicateResults(results)

	if len(deduped) != 1 {
		t.Fatalf("Expected 1 result, got %d", len(deduped))
	}
	if deduped[0].Popularity != 150 {
		t.Errorf("Popularity = %f, want 150", deduped[0].Popularity)
	}
}

func TestSortResultsDateBothZero(t *testing.T) {
	results := []model.Result{
		{URL: "1", Score: 50}, // No date
		{URL: "2", Score: 100}, // No date
	}

	sortResults(results, model.SortDate)

	// Should fall back to score when both have no date
	if results[0].URL != "2" {
		t.Errorf("First result should be higher score, got URL %q", results[0].URL)
	}
}

func TestSortResultsDateAscBothZero(t *testing.T) {
	results := []model.Result{
		{URL: "1", Score: 50}, // No date
		{URL: "2", Score: 100}, // No date
	}

	sortResults(results, model.SortDateAsc)

	// Should fall back to score when both have no date
	if results[0].URL != "2" {
		t.Errorf("First result should be higher score, got URL %q", results[0].URL)
	}
}

func TestSortResultsPopularityEqualFallbackToScore(t *testing.T) {
	results := []model.Result{
		{URL: "1", Popularity: 100, ViewCount: 1000, Score: 50},
		{URL: "2", Popularity: 100, ViewCount: 1000, Score: 100},
	}

	sortResults(results, model.SortPopularity)

	// Equal popularity, should fall back to score
	if results[0].URL != "2" {
		t.Errorf("First result should be higher score, got URL %q", results[0].URL)
	}
}

func TestSortResultsDateMixed(t *testing.T) {
	now := time.Now()
	results := []model.Result{
		{URL: "1"},                                    // No date, lower score
		{URL: "2", PublishedAt: now, Score: 50},       // Has date
		{URL: "3", Score: 200},                        // No date, higher score
	}

	sortResults(results, model.SortDate)

	// Result with date should be first, then no-date results by score
	if results[0].URL != "2" {
		t.Errorf("First result should be the one with date, got URL %q", results[0].URL)
	}
}

func TestSortResultsDateAscMixed(t *testing.T) {
	now := time.Now()
	results := []model.Result{
		{URL: "1"},                                    // No date
		{URL: "2", PublishedAt: now, Score: 50},       // Has date
		{URL: "3", Score: 200},                        // No date, higher score
	}

	sortResults(results, model.SortDateAsc)

	// Result with date should be first in ascending order too
	if results[0].URL != "2" {
		t.Errorf("First result should be the one with date, got URL %q", results[0].URL)
	}
}

func TestAggregatorSearchNoResultsFromEngines(t *testing.T) {
	engine := newMockEngine("test", model.CategoryGeneral, true)
	engine.SetResults([]model.Result{}) // Empty results

	agg := NewAggregatorSimple([]Engine{engine}, 10*time.Second)

	query := &model.Query{Text: "test", Category: model.CategoryGeneral}
	_, err := agg.Search(context.Background(), query)

	if err != model.ErrNoResults {
		t.Errorf("Search() error = %v, want ErrNoResults", err)
	}
}

func TestAggregatorSearchEngineError(t *testing.T) {
	engine := newMockEngine("test", model.CategoryGeneral, true)
	engine.SetError(model.ErrEngineTimeout) // Simulate error

	agg := NewAggregatorSimple([]Engine{engine}, 10*time.Second)

	query := &model.Query{Text: "test", Category: model.CategoryGeneral}
	_, err := agg.Search(context.Background(), query)

	// When engine returns error, results are empty, so we get ErrNoResults
	if err != model.ErrNoResults {
		t.Errorf("Search() error = %v, want ErrNoResults", err)
	}
}

func TestAggregatorSearchFilteredEnginesNoMatch(t *testing.T) {
	engine := newMockEngine("test", model.CategoryGeneral, true)
	engine.SetResults([]model.Result{{URL: "https://example.com"}})

	agg := NewAggregatorSimple([]Engine{engine}, 10*time.Second)

	// Request engines that don't exist
	query := &model.Query{
		Text:     "test",
		Category: model.CategoryGeneral,
		Engines:  []string{"nonexistent"},
	}
	_, err := agg.Search(context.Background(), query)

	if err != model.ErrNoEngines {
		t.Errorf("Search() error = %v, want ErrNoEngines", err)
	}
}

func TestAggregatorSearchMultipleEngines(t *testing.T) {
	engine1 := newMockEngine("engine1", model.CategoryGeneral, true)
	engine1.SetResults([]model.Result{
		{URL: "https://example.com/1", Title: "Result 1", Engine: "engine1"},
	})

	engine2 := newMockEngine("engine2", model.CategoryGeneral, true)
	engine2.SetResults([]model.Result{
		{URL: "https://example.com/2", Title: "Result 2", Engine: "engine2"},
	})

	agg := NewAggregatorSimple([]Engine{engine1, engine2}, 10*time.Second)

	query := &model.Query{Text: "test", Category: model.CategoryGeneral}
	results, err := agg.Search(context.Background(), query)

	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(results.Results) != 2 {
		t.Errorf("results count = %d, want 2", len(results.Results))
	}
	if len(results.Engines) != 2 {
		t.Errorf("engines count = %d, want 2", len(results.Engines))
	}
}

func TestAggregatorSearchWithOperators(t *testing.T) {
	engine := newMockEngine("test", model.CategoryGeneral, true)
	engine.SetResults([]model.Result{
		{URL: "https://example.com/1", Title: "Result 1"},
	})

	agg := NewAggregatorSimple([]Engine{engine}, 10*time.Second)

	query := &model.Query{
		Text:     "golang site:example.com",
		Category: model.CategoryGeneral,
	}
	results, err := agg.Search(context.Background(), query)

	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	// Verify operators were parsed
	if query.Site != "example.com" {
		t.Errorf("Site = %q, want example.com", query.Site)
	}
	if results == nil {
		t.Fatal("results should not be nil")
	}
}

func TestAggregatorFilterEnginesCaseInsensitive(t *testing.T) {
	engine := newMockEngine("TestEngine", model.CategoryGeneral, true)

	agg := NewAggregatorSimple([]Engine{engine}, 10*time.Second)

	// Test case-insensitive engine selection
	query := &model.Query{
		Text:     "test",
		Category: model.CategoryGeneral,
		Engines:  []string{"testengine"}, // lowercase
	}
	engines := agg.filterEngines(query)

	if len(engines) != 1 {
		t.Errorf("filterEngines() count = %d, want 1", len(engines))
	}
}

func TestAggregatorFilterEnginesExclusionCaseInsensitive(t *testing.T) {
	engine := newMockEngine("TestEngine", model.CategoryGeneral, true)

	agg := NewAggregatorSimple([]Engine{engine}, 10*time.Second)

	// Test case-insensitive engine exclusion
	query := &model.Query{
		Text:           "test",
		Category:       model.CategoryGeneral,
		ExcludeEngines: []string{"TESTENGINE"}, // uppercase
	}
	engines := agg.filterEngines(query)

	if len(engines) != 0 {
		t.Errorf("filterEngines() count = %d, want 0", len(engines))
	}
}

func TestDeduplicateResultsEngineCountBonus(t *testing.T) {
	// Same URL from 3 different engines
	results := []model.Result{
		{URL: "https://example.com/1", Title: "Result", Engine: "google", Score: 100},
		{URL: "https://example.com/1", Title: "Result", Engine: "bing", Score: 100},
		{URL: "https://example.com/1", Title: "Result", Engine: "duckduckgo", Score: 100},
	}

	deduped := deduplicateResults(results)

	if len(deduped) != 1 {
		t.Fatalf("Expected 1 result, got %d", len(deduped))
	}
	// Score should include duplicate bonus (2 * 50 = 100) and engine diversity bonus (3 * 25 = 75)
	// Original score 100 + 100 + 75 = 275
	if deduped[0].Score < 200 {
		t.Errorf("Score should include bonuses, got %f", deduped[0].Score)
	}
}

func TestDeduplicateResultsNoEngineCountBonus(t *testing.T) {
	// Single URL from single engine
	results := []model.Result{
		{URL: "https://example.com/1", Title: "Result", Engine: "google", Score: 100},
	}

	deduped := deduplicateResults(results)

	if len(deduped) != 1 {
		t.Fatalf("Expected 1 result, got %d", len(deduped))
	}
	// Score should not include engine diversity bonus for single engine
	if deduped[0].Score != 100 {
		t.Errorf("Score should be unchanged for single engine, got %f", deduped[0].Score)
	}
}

func TestNewAggregatorCacheDisabled(t *testing.T) {
	engines := []Engine{newMockEngine("test", model.CategoryGeneral, true)}
	config := AggregatorConfig{
		Timeout:      10 * time.Second,
		CacheEnabled: false,
	}

	agg := NewAggregator(engines, config)

	if agg.cache != nil {
		t.Error("cache should be nil when disabled")
	}
}

func TestNewAggregatorMaxCacheSizeDefault(t *testing.T) {
	engines := []Engine{newMockEngine("test", model.CategoryGeneral, true)}
	config := AggregatorConfig{
		CacheEnabled: true,
		// MaxCacheSize not set
	}

	agg := NewAggregator(engines, config)

	if agg.cache == nil {
		t.Fatal("cache should be created")
	}
	// Default maxSize should be 1000
	if agg.cache.maxSize != 1000 {
		t.Errorf("cache maxSize = %d, want 1000", agg.cache.maxSize)
	}
}

func TestAggregatorSearchNoCacheForEmptyResults(t *testing.T) {
	engine := newMockEngine("test", model.CategoryGeneral, true)
	engine.SetResults([]model.Result{}) // Empty results

	config := AggregatorConfig{
		Timeout:      10 * time.Second,
		CacheEnabled: true,
		CacheTTL:     5 * time.Minute,
		MaxCacheSize: 100,
	}
	agg := NewAggregator([]Engine{engine}, config)

	query := &model.Query{Text: "empty results", Category: model.CategoryGeneral}
	_, _ = agg.Search(context.Background(), query)

	// Cache should not store empty results
	cacheKey := agg.generateCacheKey(query)
	if agg.cache.Get(cacheKey) != nil {
		t.Error("Cache should not store empty results")
	}
}

func TestAggregatorApplyOperatorsLanguageDefaultOnly(t *testing.T) {
	agg := NewAggregatorSimple([]Engine{}, 10*time.Second)

	// Language is default "en"
	query := &model.Query{
		Text:     "test",
		Language: "en",
	}

	ops := &SearchOperators{
		Language: "de",
	}

	agg.applyOperators(query, ops)

	// Should be overwritten because query.Language == "en" (default)
	if query.Language != "de" {
		t.Errorf("Language = %q, want de", query.Language)
	}
}

func TestAggregatorApplyOperatorsLanguageNonDefault(t *testing.T) {
	agg := NewAggregatorSimple([]Engine{}, 10*time.Second)

	// Language is NOT default "en"
	query := &model.Query{
		Text:     "test",
		Language: "fr",
	}

	ops := &SearchOperators{
		Language: "de",
	}

	agg.applyOperators(query, ops)

	// Should NOT be overwritten because query.Language != "en"
	if query.Language != "fr" {
		t.Errorf("Language should not be overwritten, got %q", query.Language)
	}
}

func TestDeduplicateResultsZeroRelevance(t *testing.T) {
	results := []model.Result{
		{URL: "https://example.com/1", Title: "Result", Relevance: 0.5},
		{URL: "https://example.com/1", Title: "Result", Relevance: 0}, // Zero relevance
	}

	deduped := deduplicateResults(results)

	if len(deduped) != 1 {
		t.Fatalf("Expected 1 result, got %d", len(deduped))
	}
	// Relevance should not change when second has 0
	if deduped[0].Relevance != 0.5 {
		t.Errorf("Relevance = %f, want 0.5", deduped[0].Relevance)
	}
}

func TestAggregatorSearchSetsQueryFields(t *testing.T) {
	engine := newMockEngine("test", model.CategoryGeneral, true)
	engine.SetResults([]model.Result{
		{URL: "https://example.com/1", Title: "Result 1"},
	})

	agg := NewAggregatorSimple([]Engine{engine}, 10*time.Second)

	query := &model.Query{
		Text:     "test site:example.com",
		Category: model.CategoryGeneral,
		Page:     2,
		PerPage:  10,
		SortBy:   model.SortDate,
	}
	results, err := agg.Search(context.Background(), query)

	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}

	// Verify result fields are set correctly
	if results.Page != 2 {
		t.Errorf("Page = %d, want 2", results.Page)
	}
	if results.PerPage != 10 {
		t.Errorf("PerPage = %d, want 10", results.PerPage)
	}
	if results.SortedBy != model.SortDate {
		t.Errorf("SortedBy = %q, want date", results.SortedBy)
	}
	// CleanedText should be set
	if query.CleanedText == "" {
		t.Error("CleanedText should be set after parsing operators")
	}
}

func TestAggregatorApplyFiltersExcludeTermsNoMatch(t *testing.T) {
	agg := NewAggregatorSimple([]Engine{}, 10*time.Second)

	results := []model.Result{
		{URL: "https://example.com/1", Title: "Good result", Content: "No excluded terms here"},
	}

	query := &model.Query{
		Text:         "test",
		ExcludeTerms: []string{"spam", "junk"},
	}

	filtered := agg.applyFilters(results, query)

	if len(filtered) != 1 {
		t.Errorf("applyFilters() count = %d, want 1", len(filtered))
	}
}

func TestSortResultsDefault(t *testing.T) {
	results := []model.Result{
		{URL: "1", Score: 50},
		{URL: "2", Score: 100},
		{URL: "3", Score: 75},
	}

	// Test with empty SortOrder (should default to relevance)
	sortResults(results, "")

	if results[0].URL != "2" {
		t.Errorf("First result URL = %q, want 2 (highest score)", results[0].URL)
	}
}

func TestAggregatorApplyOperatorsFileType(t *testing.T) {
	agg := NewAggregatorSimple([]Engine{}, 10*time.Second)

	query := &model.Query{
		Text:     "test",
		Language: "en",
	}

	ops := &SearchOperators{
		FileType: "pdf",
	}

	agg.applyOperators(query, ops)

	if query.FileType != "pdf" {
		t.Errorf("FileType = %q, want pdf", query.FileType)
	}
}

func TestAggregatorApplyOperatorsFileTypeNotOverwritten(t *testing.T) {
	agg := NewAggregatorSimple([]Engine{}, 10*time.Second)

	query := &model.Query{
		Text:     "test",
		Language: "en",
		FileType: "docx", // Already set
	}

	ops := &SearchOperators{
		FileType: "pdf",
	}

	agg.applyOperators(query, ops)

	if query.FileType != "docx" {
		t.Errorf("FileType should not be overwritten, got %q", query.FileType)
	}
}

func TestDeduplicateResultsEmpty(t *testing.T) {
	results := []model.Result{}

	deduped := deduplicateResults(results)

	if len(deduped) != 0 {
		t.Errorf("deduplicateResults for empty slice should return empty, got %d", len(deduped))
	}
}

func TestSortResultsEmpty(t *testing.T) {
	results := []model.Result{}

	// Should not panic on empty slice
	sortResults(results, model.SortRelevance)
	sortResults(results, model.SortDate)
	sortResults(results, model.SortDateAsc)
	sortResults(results, model.SortPopularity)
	sortResults(results, model.SortRandom)

	if len(results) != 0 {
		t.Error("Empty results should remain empty")
	}
}

func TestSortResultsSingle(t *testing.T) {
	results := []model.Result{{URL: "1", Score: 100}}

	// Should not panic on single element
	sortResults(results, model.SortRelevance)

	if results[0].URL != "1" {
		t.Error("Single result should remain unchanged")
	}
}
