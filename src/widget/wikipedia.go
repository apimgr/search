package widget

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// WikipediaFetcher fetches Wikipedia summaries
type WikipediaFetcher struct {
	httpClient *http.Client
}

// WikipediaData represents Wikipedia summary result
type WikipediaData struct {
	Title           string            `json:"title"`
	Extract         string            `json:"extract"`
	Description     string            `json:"description,omitempty"`
	Thumbnail       string            `json:"thumbnail,omitempty"`
	URL             string            `json:"url"`
	Language        string            `json:"language"`
	RelatedArticles []RelatedArticle  `json:"related_articles,omitempty"`
}

// RelatedArticle represents a related Wikipedia article
type RelatedArticle struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Extract string `json:"extract,omitempty"`
}

// NewWikipediaFetcher creates a new Wikipedia fetcher
func NewWikipediaFetcher() *WikipediaFetcher {
	return &WikipediaFetcher{
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// Fetch fetches Wikipedia summary
func (f *WikipediaFetcher) Fetch(ctx context.Context, params map[string]string) (*WidgetData, error) {
	// Support both "topic" and "query" params
	topic := params["topic"]
	if topic == "" {
		topic = params["query"]
	}
	if topic == "" {
		return &WidgetData{
			Type:      WidgetWikipedia,
			Error:     "topic parameter required",
			UpdatedAt: time.Now(),
		}, nil
	}

	// Parse natural language queries
	topic = f.parseQuery(topic)

	lang := params["lang"]
	if lang == "" {
		lang = "en"
	}

	// First, try direct article lookup
	data, err := f.fetchArticleSummary(ctx, topic, lang)
	if err != nil {
		return nil, err
	}

	// If direct lookup failed, try search
	if data == nil {
		data, err = f.searchAndFetch(ctx, topic, lang)
		if err != nil {
			return nil, err
		}
	}

	if data == nil {
		return &WidgetData{
			Type:      WidgetWikipedia,
			Error:     "article not found",
			UpdatedAt: time.Now(),
		}, nil
	}

	// Fetch related articles
	related, _ := f.fetchRelatedArticles(ctx, data.Title, lang)
	if len(related) > 0 {
		data.RelatedArticles = related
	}

	return &WidgetData{
		Type:      WidgetWikipedia,
		Data:      data,
		UpdatedAt: time.Now(),
	}, nil
}

// parseQuery extracts the topic from natural language queries
func (f *WikipediaFetcher) parseQuery(query string) string {
	query = strings.TrimSpace(query)

	// Remove common prefixes
	prefixes := []string{
		"wiki ", "wikipedia ",
		"who is ", "who was ", "who are ",
		"what is ", "what are ", "what was ",
		"define ", "tell me about ", "information about ",
		"search for ", "look up ", "find ",
	}

	lowerQuery := strings.ToLower(query)
	for _, prefix := range prefixes {
		if strings.HasPrefix(lowerQuery, prefix) {
			query = query[len(prefix):]
			break
		}
	}

	// Remove trailing question marks
	query = strings.TrimSuffix(query, "?")
	query = strings.TrimSpace(query)

	return query
}

// fetchArticleSummary fetches a Wikipedia article summary by title
func (f *WikipediaFetcher) fetchArticleSummary(ctx context.Context, title, lang string) (*WikipediaData, error) {
	// Replace spaces with underscores for Wikipedia URLs
	encodedTitle := strings.ReplaceAll(title, " ", "_")

	apiURL := fmt.Sprintf("https://%s.wikipedia.org/api/rest_v1/page/summary/%s",
		url.PathEscape(lang),
		url.PathEscape(encodedTitle))

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Search/1.0 (privacy-focused search engine)")

	resp, err := f.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return nil, nil // Not found, will try search
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("Wikipedia API error: %d", resp.StatusCode)
	}

	var result struct {
		Type        string `json:"type"`
		Title       string `json:"title"`
		Extract     string `json:"extract"`
		Description string `json:"description"`
		Thumbnail   struct {
			Source string `json:"source"`
			Width  int    `json:"width"`
			Height int    `json:"height"`
		} `json:"thumbnail"`
		OriginalImage struct {
			Source string `json:"source"`
		} `json:"originalimage"`
		ContentURLs struct {
			Desktop struct {
				Page string `json:"page"`
			} `json:"desktop"`
		} `json:"content_urls"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	// Skip disambiguation pages - try search instead
	if result.Type == "disambiguation" {
		return nil, nil
	}

	thumbnail := result.Thumbnail.Source
	// Use original image if thumbnail not available
	if thumbnail == "" && result.OriginalImage.Source != "" {
		thumbnail = result.OriginalImage.Source
	}

	return &WikipediaData{
		Title:       result.Title,
		Extract:     result.Extract,
		Description: result.Description,
		Thumbnail:   thumbnail,
		URL:         result.ContentURLs.Desktop.Page,
		Language:    lang,
	}, nil
}

// searchAndFetch searches Wikipedia and fetches the top result
func (f *WikipediaFetcher) searchAndFetch(ctx context.Context, query, lang string) (*WikipediaData, error) {
	// Use MediaWiki API for search
	apiURL := fmt.Sprintf(
		"https://%s.wikipedia.org/w/api.php?action=query&list=search&srsearch=%s&srwhat=text&srlimit=1&format=json",
		url.PathEscape(lang),
		url.QueryEscape(query),
	)

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Search/1.0 (privacy-focused search engine)")

	resp, err := f.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("Wikipedia search API error: %d", resp.StatusCode)
	}

	var searchResult struct {
		Query struct {
			Search []struct {
				Title   string `json:"title"`
				Snippet string `json:"snippet"`
			} `json:"search"`
		} `json:"query"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&searchResult); err != nil {
		return nil, err
	}

	if len(searchResult.Query.Search) == 0 {
		return nil, nil // No results
	}

	// Fetch the full summary for the top search result
	return f.fetchArticleSummary(ctx, searchResult.Query.Search[0].Title, lang)
}

// fetchRelatedArticles fetches related articles for a given title
func (f *WikipediaFetcher) fetchRelatedArticles(ctx context.Context, title, lang string) ([]RelatedArticle, error) {
	// Use MediaWiki API to get links from the article
	encodedTitle := strings.ReplaceAll(title, " ", "_")
	apiURL := fmt.Sprintf(
		"https://%s.wikipedia.org/w/api.php?action=query&titles=%s&prop=links&pllimit=10&plnamespace=0&format=json",
		url.PathEscape(lang),
		url.QueryEscape(encodedTitle),
	)

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Search/1.0 (privacy-focused search engine)")

	resp, err := f.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("Wikipedia links API error: %d", resp.StatusCode)
	}

	var linksResult struct {
		Query struct {
			Pages map[string]struct {
				Links []struct {
					Title string `json:"title"`
				} `json:"links"`
			} `json:"pages"`
		} `json:"query"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&linksResult); err != nil {
		return nil, err
	}

	var related []RelatedArticle
	seenTitles := make(map[string]bool)

	// Filter for meaningful related articles
	titleRegex := regexp.MustCompile(`^[A-Z]`)

	for _, page := range linksResult.Query.Pages {
		for _, link := range page.Links {
			linkTitle := link.Title
			// Skip if already seen or doesn't start with capital letter
			if seenTitles[linkTitle] || !titleRegex.MatchString(linkTitle) {
				continue
			}
			// Skip meta pages
			if strings.Contains(linkTitle, ":") {
				continue
			}
			seenTitles[linkTitle] = true

			encodedLinkTitle := strings.ReplaceAll(linkTitle, " ", "_")
			related = append(related, RelatedArticle{
				Title: linkTitle,
				URL:   fmt.Sprintf("https://%s.wikipedia.org/wiki/%s", lang, url.PathEscape(encodedLinkTitle)),
			})

			// Limit to 5 related articles
			if len(related) >= 5 {
				break
			}
		}
		if len(related) >= 5 {
			break
		}
	}

	return related, nil
}

// CacheDuration returns how long to cache Wikipedia data
func (f *WikipediaFetcher) CacheDuration() time.Duration {
	return 1 * time.Hour
}

// WidgetType returns the widget type
func (f *WikipediaFetcher) WidgetType() WidgetType {
	return WidgetWikipedia
}
