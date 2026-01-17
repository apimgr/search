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
