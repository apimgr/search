package search

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/apimgr/search/src/version"
)

// RelatedSearches provides related search suggestions
type RelatedSearches struct {
	mu         sync.RWMutex
	cache      map[string]*relatedCacheEntry
	httpClient *http.Client
}

type relatedCacheEntry struct {
	Suggestions []string
	ExpiresAt   time.Time
}

// NewRelatedSearches creates a new related searches provider
func NewRelatedSearches() *RelatedSearches {
	return &RelatedSearches{
		cache:      make(map[string]*relatedCacheEntry),
		httpClient: &http.Client{Timeout: 5 * time.Second},
	}
}

// GetRelated returns related search suggestions for a query
func (rs *RelatedSearches) GetRelated(ctx context.Context, query string, limit int) ([]string, error) {
	if query == "" {
		return nil, nil
	}

	if limit <= 0 {
		limit = 8
	}

	// Check cache first
	cacheKey := strings.ToLower(strings.TrimSpace(query))
	rs.mu.RLock()
	if entry, ok := rs.cache[cacheKey]; ok && time.Now().Before(entry.ExpiresAt) {
		rs.mu.RUnlock()
		if len(entry.Suggestions) > limit {
			return entry.Suggestions[:limit], nil
		}
		return entry.Suggestions, nil
	}
	rs.mu.RUnlock()

	// Fetch from multiple sources
	suggestions := rs.fetchRelatedSearches(ctx, query)

	// Cache the results
	rs.mu.Lock()
	rs.cache[cacheKey] = &relatedCacheEntry{
		Suggestions: suggestions,
		ExpiresAt:   time.Now().Add(1 * time.Hour),
	}
	rs.mu.Unlock()

	// Clean old cache entries periodically
	go rs.cleanCache()

	if len(suggestions) > limit {
		return suggestions[:limit], nil
	}
	return suggestions, nil
}

// fetchRelatedSearches fetches related searches from autocomplete APIs
func (rs *RelatedSearches) fetchRelatedSearches(ctx context.Context, query string) []string {
	results := make(chan []string, 3)
	var wg sync.WaitGroup

	// DuckDuckGo autocomplete
	wg.Add(1)
	go func() {
		defer wg.Done()
		suggestions := rs.fetchDuckDuckGo(ctx, query)
		results <- suggestions
	}()

	// Google suggestions (public API)
	wg.Add(1)
	go func() {
		defer wg.Done()
		suggestions := rs.fetchGoogle(ctx, query)
		results <- suggestions
	}()

	// Generate variations
	wg.Add(1)
	go func() {
		defer wg.Done()
		variations := rs.generateVariations(query)
		results <- variations
	}()

	// Wait for all fetchers
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect and deduplicate results
	seen := make(map[string]bool)
	var allSuggestions []string
	queryLower := strings.ToLower(query)

	for suggestions := range results {
		for _, s := range suggestions {
			sLower := strings.ToLower(s)
			if !seen[sLower] && sLower != queryLower {
				seen[sLower] = true
				allSuggestions = append(allSuggestions, s)
			}
		}
	}

	return allSuggestions
}

// fetchDuckDuckGo fetches autocomplete suggestions from DuckDuckGo
func (rs *RelatedSearches) fetchDuckDuckGo(ctx context.Context, query string) []string {
	// Use default format (without type=list) which returns [{"phrase": "..."}]
	apiURL := fmt.Sprintf("https://duckduckgo.com/ac/?q=%s", url.QueryEscape(query))

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("User-Agent", version.BrowserUserAgent)

	resp, err := rs.httpClient.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	var result []struct {
		Phrase string `json:"phrase"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil
	}

	suggestions := make([]string, 0, len(result))
	for _, r := range result {
		if r.Phrase != "" {
			suggestions = append(suggestions, r.Phrase)
		}
	}

	return suggestions
}

// fetchGoogle fetches autocomplete suggestions from Google
func (rs *RelatedSearches) fetchGoogle(ctx context.Context, query string) []string {
	apiURL := fmt.Sprintf("https://suggestqueries.google.com/complete/search?client=firefox&q=%s", url.QueryEscape(query))

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("User-Agent", version.BrowserUserAgent)

	resp, err := rs.httpClient.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	// Google returns [query, [suggestions]]
	var result []interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil
	}

	if len(result) < 2 {
		return nil
	}

	suggestions, ok := result[1].([]interface{})
	if !ok {
		return nil
	}

	var stringResults []string
	for _, s := range suggestions {
		if str, ok := s.(string); ok && str != "" {
			stringResults = append(stringResults, str)
		}
	}

	return stringResults
}

// generateVariations generates query variations
func (rs *RelatedSearches) generateVariations(query string) []string {
	var variations []string
	words := strings.Fields(query)

	if len(words) == 0 {
		return nil
	}

	// Question variations
	questionPrefixes := []string{"what is", "how to", "why is", "when was", "where is", "who is"}
	for _, prefix := range questionPrefixes {
		if !strings.HasPrefix(strings.ToLower(query), prefix) {
			variations = append(variations, prefix+" "+query)
		}
	}

	// Add common suffixes
	suffixes := []string{"examples", "tutorial", "guide", "vs", "alternatives", "review", "best"}
	for _, suffix := range suffixes {
		if !strings.HasSuffix(strings.ToLower(query), suffix) {
			variations = append(variations, query+" "+suffix)
		}
	}

	// If multiple words, try without first/last word
	if len(words) > 2 {
		variations = append(variations, strings.Join(words[1:], " "))
		variations = append(variations, strings.Join(words[:len(words)-1], " "))
	}

	return variations
}

// cleanCache removes expired cache entries
func (rs *RelatedSearches) cleanCache() {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	now := time.Now()
	for key, entry := range rs.cache {
		if now.After(entry.ExpiresAt) {
			delete(rs.cache, key)
		}
	}
}

// ClearCache clears the entire cache
func (rs *RelatedSearches) ClearCache() {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	rs.cache = make(map[string]*relatedCacheEntry)
}
