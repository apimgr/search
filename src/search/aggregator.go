package search

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"math/rand"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/apimgr/search/src/model"
)

// Aggregator aggregates results from multiple search engines
type Aggregator struct {
	engines      []Engine
	timeout      time.Duration
	cache        *ResultCache
	cacheEnabled bool
	cacheTTL     time.Duration
}

// AggregatorConfig holds aggregator configuration
type AggregatorConfig struct {
	Timeout      time.Duration
	CacheEnabled bool
	CacheTTL     time.Duration
	MaxCacheSize int
}

// NewAggregator creates a new search aggregator
func NewAggregator(engines []Engine, config AggregatorConfig) *Aggregator {
	if config.Timeout == 0 {
		config.Timeout = 30 * time.Second
	}
	if config.CacheTTL == 0 {
		config.CacheTTL = 5 * time.Minute
	}
	if config.MaxCacheSize == 0 {
		config.MaxCacheSize = 1000
	}

	a := &Aggregator{
		engines:      engines,
		timeout:      config.Timeout,
		cacheEnabled: config.CacheEnabled,
		cacheTTL:     config.CacheTTL,
	}

	if config.CacheEnabled {
		a.cache = NewResultCache(config.MaxCacheSize, config.CacheTTL)
	}

	return a
}

// NewAggregatorSimple creates an aggregator with default settings (backwards compatible)
func NewAggregatorSimple(engines []Engine, timeout time.Duration) *Aggregator {
	return NewAggregator(engines, AggregatorConfig{
		Timeout:      timeout,
		CacheEnabled: false,
	})
}

// Search performs concurrent searches across all engines
func (a *Aggregator) Search(ctx context.Context, query *model.Query) (*model.SearchResults, error) {
	if err := query.Validate(); err != nil {
		return nil, err
	}

	if len(a.engines) == 0 {
		return nil, model.ErrNoEngines
	}

	// Parse search operators from query text
	ops := ParseOperators(query.Text)
	query.ParsedOperators = ops
	query.CleanedText = ops.CleanedQuery

	// Apply parsed operators to query fields
	a.applyOperators(query, ops)

	// Check cache
	cacheKey := a.generateCacheKey(query)
	if a.cacheEnabled && a.cache != nil {
		if cached := a.cache.Get(cacheKey); cached != nil {
			// Update search time to indicate cache hit
			cached.SearchTime = 0.001 // Nearly instant
			return cached, nil
		}
	}

	startTime := time.Now()

	// Create context with timeout
	searchCtx, cancel := context.WithTimeout(ctx, a.timeout)
	defer cancel()

	// Channel for collecting results
	type engineResult struct {
		engine  string
		results []model.Result
		err     error
	}

	// Filter engines
	activeEngines := a.filterEngines(query)
	if len(activeEngines) == 0 {
		return nil, model.ErrNoEngines
	}

	resultsChan := make(chan engineResult, len(activeEngines))
	var wg sync.WaitGroup

	// Launch concurrent searches
	for _, engine := range activeEngines {
		wg.Add(1)
		go func(eng Engine) {
			defer wg.Done()

			results, err := eng.Search(searchCtx, query)
			resultsChan <- engineResult{
				engine:  eng.Name(),
				results: results,
				err:     err,
			}
		}(engine)
	}

	// Wait for all goroutines to complete
	go func() {
		wg.Wait()
		close(resultsChan)
	}()

	// Collect all results
	searchResults := model.NewSearchResults(query.Text, query.Category)
	searchResults.Page = query.Page
	searchResults.PerPage = query.PerPage
	searchResults.SortedBy = query.SortBy

	usedEngines := make([]string, 0)

	for result := range resultsChan {
		if result.err == nil && len(result.results) > 0 {
			searchResults.AddResults(result.results)
			usedEngines = append(usedEngines, result.engine)
		}
	}

	searchResults.Engines = usedEngines

	// Deduplicate results
	searchResults.Results = deduplicateResults(searchResults.Results)
	searchResults.TotalResults = len(searchResults.Results)

	// Apply post-processing filters (site exclusion, date range, etc.)
	searchResults.Results = a.applyFilters(searchResults.Results, query)
	searchResults.TotalResults = len(searchResults.Results)

	// Rank and sort results
	sortResults(searchResults.Results, query.SortBy)

	// Calculate pagination
	searchResults.CalculateTotalPages()

	// Calculate search time
	searchResults.SearchTime = time.Since(startTime).Seconds()

	// Cache results
	if a.cacheEnabled && a.cache != nil && len(searchResults.Results) > 0 {
		a.cache.Set(cacheKey, searchResults)
	}

	if len(searchResults.Results) == 0 {
		return searchResults, model.ErrNoResults
	}

	return searchResults, nil
}

// applyOperators applies parsed operators to query fields
func (a *Aggregator) applyOperators(query *model.Query, ops *SearchOperators) {
	if ops.Site != "" && query.Site == "" {
		query.Site = ops.Site
	}
	if ops.ExcludeSite != "" && query.ExcludeSite == "" {
		query.ExcludeSite = ops.ExcludeSite
	}
	if ops.FileType != "" && query.FileType == "" {
		query.FileType = ops.FileType
	}
	if len(ops.FileTypes) > 0 && len(query.FileTypes) == 0 {
		query.FileTypes = ops.FileTypes
	}
	if ops.InURL != "" && query.InURL == "" {
		query.InURL = ops.InURL
	}
	if ops.InTitle != "" && query.InTitle == "" {
		query.InTitle = ops.InTitle
	}
	if ops.InText != "" && query.InText == "" {
		query.InText = ops.InText
	}
	if len(ops.ExactPhrases) > 0 && len(query.ExactPhrases) == 0 {
		query.ExactPhrases = ops.ExactPhrases
	}
	if len(ops.ExcludeTerms) > 0 && len(query.ExcludeTerms) == 0 {
		query.ExcludeTerms = ops.ExcludeTerms
	}
	if ops.Before != "" && query.DateBefore == "" {
		query.DateBefore = ops.Before
	}
	if ops.After != "" && query.DateAfter == "" {
		query.DateAfter = ops.After
	}
	if ops.Language != "" && query.Language == "en" {
		query.Language = ops.Language
	}
	if ops.Source != "" && query.NewsSource == "" {
		query.NewsSource = ops.Source
	}
}

// filterEngines returns engines that should be used for this query
func (a *Aggregator) filterEngines(query *model.Query) []Engine {
	var result []Engine

	for _, engine := range a.engines {
		// Check category support
		if !engine.SupportsCategory(query.Category) {
			continue
		}

		// Check if engine is explicitly selected
		if len(query.Engines) > 0 {
			found := false
			for _, e := range query.Engines {
				if strings.EqualFold(e, engine.Name()) {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}

		// Check if engine is excluded
		if len(query.ExcludeEngines) > 0 {
			excluded := false
			for _, e := range query.ExcludeEngines {
				if strings.EqualFold(e, engine.Name()) {
					excluded = true
					break
				}
			}
			if excluded {
				continue
			}
		}

		result = append(result, engine)
	}

	return result
}

// applyFilters applies post-search filters to results
func (a *Aggregator) applyFilters(results []model.Result, query *model.Query) []model.Result {
	if len(results) == 0 {
		return results
	}

	filtered := make([]model.Result, 0, len(results))

	for _, r := range results {
		// Site exclusion filter
		if query.ExcludeSite != "" {
			domain := r.ExtractDomain()
			if strings.Contains(strings.ToLower(domain), strings.ToLower(query.ExcludeSite)) {
				continue
			}
		}

		// Exclude terms filter (check title and content)
		if len(query.ExcludeTerms) > 0 {
			excluded := false
			text := strings.ToLower(r.Title + " " + r.Content)
			for _, term := range query.ExcludeTerms {
				if strings.Contains(text, strings.ToLower(term)) {
					excluded = true
					break
				}
			}
			if excluded {
				continue
			}
		}

		// Date range filter
		if !r.PublishedAt.IsZero() {
			if query.DateBefore != "" {
				before, err := time.Parse("2006-01-02", query.DateBefore)
				if err == nil && r.PublishedAt.After(before) {
					continue
				}
			}
			if query.DateAfter != "" {
				after, err := time.Parse("2006-01-02", query.DateAfter)
				if err == nil && r.PublishedAt.Before(after) {
					continue
				}
			}
		}

		filtered = append(filtered, r)
	}

	return filtered
}

// generateCacheKey creates a unique cache key for the query
func (a *Aggregator) generateCacheKey(query *model.Query) string {
	// Include relevant query parameters
	key := query.Text + "|" +
		string(query.Category) + "|" +
		query.Language + "|" +
		query.Region + "|" +
		string(query.SortBy) + "|" +
		query.TimeRange

	hash := sha256.Sum256([]byte(key))
	return hex.EncodeToString(hash[:16])
}

// deduplicateResults removes duplicate results based on URL with improved merging
func deduplicateResults(results []model.Result) []model.Result {
	seen := make(map[string]int) // URL -> index in unique slice
	unique := make([]model.Result, 0)
	duplicateCounts := make(map[string]int)
	engineSources := make(map[string][]string) // URL -> list of engines

	// First pass: count duplicates and track sources
	for _, result := range results {
		duplicateCounts[result.URL]++
		engineSources[result.URL] = append(engineSources[result.URL], result.Engine)
	}

	// Second pass: merge duplicates
	for _, result := range results {
		if idx, exists := seen[result.URL]; exists {
			// Merge with existing result
			existing := &unique[idx]

			// Keep better content (longer is usually better)
			if len(result.Content) > len(existing.Content) {
				existing.Content = result.Content
			}

			// Keep thumbnail if we don't have one
			if existing.Thumbnail == "" && result.Thumbnail != "" {
				existing.Thumbnail = result.Thumbnail
			}

			// Keep author if we don't have one
			if existing.Author == "" && result.Author != "" {
				existing.Author = result.Author
			}

			// Keep earlier publish date
			if !result.PublishedAt.IsZero() {
				if existing.PublishedAt.IsZero() || result.PublishedAt.Before(existing.PublishedAt) {
					existing.PublishedAt = result.PublishedAt
				}
			}

			// Merge relevance scores
			if result.Relevance > 0 {
				existing.Relevance = (existing.Relevance + result.Relevance) / 2
			}

			// Accumulate popularity
			existing.Popularity += result.Popularity
		} else {
			// First occurrence
			seen[result.URL] = len(unique)

			// Set duplicate count
			result.DuplicateCount = duplicateCounts[result.URL]

			// Calculate enhanced score with duplicate boost
			duplicateBonus := float64((duplicateCounts[result.URL] - 1) * 50)
			result.Score += duplicateBonus

			// Add diversity bonus for appearing in multiple engines
			engineCount := len(engineSources[result.URL])
			if engineCount > 1 {
				result.Score += float64(engineCount * 25) // 25 points per additional engine
			}

			unique = append(unique, result)
		}
	}

	return unique
}

// sortResults sorts results based on the specified sort order
func sortResults(results []model.Result, sortBy model.SortOrder) {
	switch sortBy {
	case model.SortDate:
		// Newest first
		sort.Slice(results, func(i, j int) bool {
			// Results without dates go to the end
			if results[i].PublishedAt.IsZero() && !results[j].PublishedAt.IsZero() {
				return false
			}
			if !results[i].PublishedAt.IsZero() && results[j].PublishedAt.IsZero() {
				return true
			}
			if results[i].PublishedAt.IsZero() && results[j].PublishedAt.IsZero() {
				return results[i].Score > results[j].Score // Fall back to score
			}
			return results[i].PublishedAt.After(results[j].PublishedAt)
		})

	case model.SortDateAsc:
		// Oldest first
		sort.Slice(results, func(i, j int) bool {
			if results[i].PublishedAt.IsZero() && !results[j].PublishedAt.IsZero() {
				return false
			}
			if !results[i].PublishedAt.IsZero() && results[j].PublishedAt.IsZero() {
				return true
			}
			if results[i].PublishedAt.IsZero() && results[j].PublishedAt.IsZero() {
				return results[i].Score > results[j].Score
			}
			return results[i].PublishedAt.Before(results[j].PublishedAt)
		})

	case model.SortPopularity:
		// Most popular first (uses popularity score + view count)
		sort.Slice(results, func(i, j int) bool {
			popI := results[i].Popularity + float64(results[i].ViewCount)/1000
			popJ := results[j].Popularity + float64(results[j].ViewCount)/1000
			if popI != popJ {
				return popI > popJ
			}
			return results[i].Score > results[j].Score // Fall back to relevance
		})

	case model.SortRandom:
		// Shuffle results
		rand.Shuffle(len(results), func(i, j int) {
			results[i], results[j] = results[j], results[i]
		})

	default: // SortRelevance
		// Default: sort by score (highest first)
		sort.Slice(results, func(i, j int) bool {
			return results[i].Score > results[j].Score
		})
	}
}

// rankResults sorts results by score (backwards compatible)
func rankResults(results []model.Result) {
	sortResults(results, model.SortRelevance)
}
