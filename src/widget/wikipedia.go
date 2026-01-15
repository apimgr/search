package widget

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// WikipediaFetcher fetches Wikipedia summaries
type WikipediaFetcher struct {
	httpClient *http.Client
}

// WikipediaData represents Wikipedia summary result
type WikipediaData struct {
	Title       string `json:"title"`
	Extract     string `json:"extract"`
	Description string `json:"description,omitempty"`
	Thumbnail   string `json:"thumbnail,omitempty"`
	URL         string `json:"url"`
	Language    string `json:"language"`
}

// NewWikipediaFetcher creates a new Wikipedia fetcher
func NewWikipediaFetcher() *WikipediaFetcher {
	return &WikipediaFetcher{
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// Fetch fetches Wikipedia summary
func (f *WikipediaFetcher) Fetch(ctx context.Context, params map[string]string) (*WidgetData, error) {
	query := params["query"]
	if query == "" {
		return &WidgetData{
			Type:      WidgetWikipedia,
			Error:     "query parameter required",
			UpdatedAt: time.Now(),
		}, nil
	}

	lang := params["lang"]
	if lang == "" {
		lang = "en"
	}

	// Use Wikipedia REST API for summaries
	apiURL := fmt.Sprintf("https://%s.wikipedia.org/api/rest_v1/page/summary/%s",
		url.PathEscape(lang),
		url.PathEscape(query))

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
		return &WidgetData{
			Type:      WidgetWikipedia,
			Error:     "article not found",
			UpdatedAt: time.Now(),
		}, nil
	}

	var result struct {
		Title       string `json:"title"`
		Extract     string `json:"extract"`
		Description string `json:"description"`
		Thumbnail   struct {
			Source string `json:"source"`
		} `json:"thumbnail"`
		ContentURLs struct {
			Desktop struct {
				Page string `json:"page"`
			} `json:"desktop"`
		} `json:"content_urls"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	data := &WikipediaData{
		Title:       result.Title,
		Extract:     result.Extract,
		Description: result.Description,
		Thumbnail:   result.Thumbnail.Source,
		URL:         result.ContentURLs.Desktop.Page,
		Language:    lang,
	}

	return &WidgetData{
		Type:      WidgetWikipedia,
		Data:      data,
		UpdatedAt: time.Now(),
	}, nil
}

// CacheDuration returns how long to cache Wikipedia data
func (f *WikipediaFetcher) CacheDuration() time.Duration {
	return 1 * time.Hour
}

// WidgetType returns the widget type
func (f *WikipediaFetcher) WidgetType() WidgetType {
	return WidgetWikipedia
}
