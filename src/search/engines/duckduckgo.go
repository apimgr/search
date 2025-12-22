package engines

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/apimgr/search/src/models"
	"github.com/apimgr/search/src/search"
)

// DuckDuckGo implements DuckDuckGo search engine
type DuckDuckGo struct {
	*search.BaseEngine
	client *http.Client
}

// NewDuckDuckGo creates a new DuckDuckGo engine
func NewDuckDuckGo() *DuckDuckGo {
	config := models.NewEngineConfig("duckduckgo")
	config.DisplayName = "DuckDuckGo"
	config.Priority = 100 // Highest priority (default engine)
	config.Categories = []string{"general", "images", "videos", "news"}
	config.SupportsTor = true

	return &DuckDuckGo{
		BaseEngine: search.NewBaseEngine(config),
		client: &http.Client{
			Timeout: time.Duration(config.GetTimeout()) * time.Second,
		},
	}
}

// Search performs a DuckDuckGo search
func (e *DuckDuckGo) Search(ctx context.Context, query *models.Query) ([]models.Result, error) {
	switch query.Category {
	case models.CategoryImages:
		return e.searchImages(ctx, query)
	case models.CategoryVideos:
		return e.searchVideos(ctx, query)
	case models.CategoryNews:
		return e.searchNews(ctx, query)
	default:
		return e.searchGeneral(ctx, query)
	}
}

// searchGeneral performs a general web search
func (e *DuckDuckGo) searchGeneral(ctx context.Context, query *models.Query) ([]models.Result, error) {
	// DuckDuckGo Instant Answer API
	apiURL := "https://api.duckduckgo.com/"
	
	params := url.Values{}
	params.Set("q", query.Text)
	params.Set("format", "json")
	params.Set("no_html", "1")
	params.Set("skip_disambig", "1")
	
	reqURL := fmt.Sprintf("%s?%s", apiURL, params.Encode())
	
	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, err
	}
	
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36 Edg/131.0.0.0")
	
	resp, err := e.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("DuckDuckGo API returned status %d", resp.StatusCode)
	}
	
	var data struct {
		AbstractText   string `json:"AbstractText"`
		AbstractSource string `json:"AbstractSource"`
		AbstractURL    string `json:"AbstractURL"`
		Heading        string `json:"Heading"`
		RelatedTopics  []struct {
			FirstURL string `json:"FirstURL"`
			Text     string `json:"Text"`
		} `json:"RelatedTopics"`
		Results []struct {
			FirstURL string `json:"FirstURL"`
			Text     string `json:"Text"`
		} `json:"Results"`
	}
	
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}
	
	results := make([]models.Result, 0)
	position := 0
	
	// Add abstract as first result if available
	if data.AbstractText != "" && data.AbstractURL != "" {
		results = append(results, models.Result{
			Title:    data.Heading,
			URL:      data.AbstractURL,
			Content:  data.AbstractText,
			Engine:   e.Name(),
			Category: models.CategoryGeneral,
			Score:    calculateScore(e.GetPriority(), position, 1),
			Position: position,
		})
		position++
	}
	
	// Add related topics
	for _, topic := range data.RelatedTopics {
		if topic.FirstURL != "" && topic.Text != "" {
			results = append(results, models.Result{
				Title:    extractTitle(topic.Text),
				URL:      topic.FirstURL,
				Content:  topic.Text,
				Engine:   e.Name(),
				Category: models.CategoryGeneral,
				Score:    calculateScore(e.GetPriority(), position, 1),
				Position: position,
			})
			position++
			
			if position >= e.GetConfig().GetMaxResults() {
				break
			}
		}
	}
	
	// Add results
	for _, result := range data.Results {
		if result.FirstURL != "" && result.Text != "" {
			results = append(results, models.Result{
				Title:    extractTitle(result.Text),
				URL:      result.FirstURL,
				Content:  result.Text,
				Engine:   e.Name(),
				Category: models.CategoryGeneral,
				Score:    calculateScore(e.GetPriority(), position, 1),
				Position: position,
			})
			position++
			
			if position >= e.GetConfig().GetMaxResults() {
				break
			}
		}
	}
	
	return results, nil
}

// searchImages performs an image search using DuckDuckGo images
func (e *DuckDuckGo) searchImages(ctx context.Context, query *models.Query) ([]models.Result, error) {
	// First, get a VQD token
	vqd, err := e.getVQDToken(ctx, query.Text)
	if err != nil {
		return nil, err
	}

	// DuckDuckGo Image API
	apiURL := "https://duckduckgo.com/i.js"

	params := url.Values{}
	params.Set("q", query.Text)
	params.Set("o", "json")
	params.Set("vqd", vqd)
	params.Set("f", ",,,,,")
	params.Set("p", "1")

	// Safe search
	switch query.SafeSearch {
	case 0:
		params.Set("p", "-1") // Off
	case 2:
		params.Set("p", "1") // Strict
	default:
		params.Set("p", "1") // Moderate (default)
	}

	// Image size filter
	if query.ImageSize != "" {
		sizeMap := map[string]string{
			"small":  "Small",
			"medium": "Medium",
			"large":  "Large",
			"xlarge": "Wallpaper",
		}
		if s, ok := sizeMap[query.ImageSize]; ok {
			params.Set("iaf", fmt.Sprintf("size:%s", s))
		}
	}

	// Image type filter
	if query.ImageType != "" {
		typeMap := map[string]string{
			"photo":    "photo",
			"clipart":  "clipart",
			"animated": "gif",
		}
		if t, ok := typeMap[query.ImageType]; ok {
			params.Set("iaf", fmt.Sprintf("type:%s", t))
		}
	}

	reqURL := fmt.Sprintf("%s?%s", apiURL, params.Encode())

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Referer", "https://duckduckgo.com/")

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("DuckDuckGo images returned status %d", resp.StatusCode)
	}

	var data struct {
		Results []struct {
			Title     string `json:"title"`
			URL       string `json:"url"`       // Page URL
			Image     string `json:"image"`     // Full image URL
			Thumbnail string `json:"thumbnail"` // Thumbnail URL
			Width     int    `json:"width"`
			Height    int    `json:"height"`
			Source    string `json:"source"`
		} `json:"results"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	results := make([]models.Result, 0, len(data.Results))

	for i, img := range data.Results {
		if i >= e.GetConfig().GetMaxResults() {
			break
		}

		results = append(results, models.Result{
			Title:       img.Title,
			URL:         img.URL,
			Thumbnail:   img.Thumbnail,
			Content:     fmt.Sprintf("%dx%d - %s", img.Width, img.Height, img.Source),
			Engine:      e.Name(),
			Category:    models.CategoryImages,
			ImageWidth:  img.Width,
			ImageHeight: img.Height,
			Score:       calculateScore(e.GetPriority(), i, 1),
			Position:    i,
			Metadata: map[string]interface{}{
				"full_image": img.Image,
				"source":     img.Source,
			},
		})
	}

	return results, nil
}

// getVQDToken gets the VQD token required for DuckDuckGo image/video search
func (e *DuckDuckGo) getVQDToken(ctx context.Context, query string) (string, error) {
	reqURL := fmt.Sprintf("https://duckduckgo.com/?q=%s", url.QueryEscape(query))

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	resp, err := e.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	// Read body to extract vqd token
	body := make([]byte, 50000)
	n, _ := resp.Body.Read(body)
	html := string(body[:n])

	// Extract vqd from response
	// Format: vqd="3-xxxxxxxxxxxx..."
	vqdStart := "vqd=\""
	if idx := findSubstring(html, vqdStart); idx != -1 {
		start := idx + len(vqdStart)
		end := findSubstring(html[start:], "\"")
		if end != -1 {
			return html[start : start+end], nil
		}
	}

	// Alternative: vqd='3-xxxxxxxxxxxx...'
	vqdStart = "vqd='"
	if idx := findSubstring(html, vqdStart); idx != -1 {
		start := idx + len(vqdStart)
		end := findSubstring(html[start:], "'")
		if end != -1 {
			return html[start : start+end], nil
		}
	}

	return "", fmt.Errorf("failed to extract VQD token")
}

// findSubstring finds substring index (simple implementation)
func findSubstring(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

// searchVideos performs a video search using DuckDuckGo
func (e *DuckDuckGo) searchVideos(ctx context.Context, query *models.Query) ([]models.Result, error) {
	// First, get a VQD token
	vqd, err := e.getVQDToken(ctx, query.Text)
	if err != nil {
		return nil, err
	}

	// DuckDuckGo Video API
	apiURL := "https://duckduckgo.com/v.js"

	params := url.Values{}
	params.Set("q", query.Text)
	params.Set("o", "json")
	params.Set("vqd", vqd)

	// Safe search
	switch query.SafeSearch {
	case 0:
		params.Set("p", "-1")
	case 2:
		params.Set("p", "1")
	default:
		params.Set("p", "1")
	}

	// Video duration filter
	if query.VideoLength != "" {
		durationMap := map[string]string{
			"short":  "short",  // < 5 min
			"medium": "medium", // 5-20 min
			"long":   "long",   // > 20 min
		}
		if d, ok := durationMap[query.VideoLength]; ok {
			params.Set("duration", d)
		}
	}

	// Video quality filter
	if query.VideoQuality == "hd" {
		params.Set("hd", "1")
	}

	reqURL := fmt.Sprintf("%s?%s", apiURL, params.Encode())

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Referer", "https://duckduckgo.com/")

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("DuckDuckGo videos returned status %d", resp.StatusCode)
	}

	var data struct {
		Results []struct {
			Title       string `json:"title"`
			URL         string `json:"content"`    // Video page URL
			Description string `json:"description"`
			Duration    string `json:"duration"`
			ViewCount   int64  `json:"views"`
			Published   string `json:"published"`
			Publisher   string `json:"publisher"`
			Thumbnail   string `json:"images"`
		} `json:"results"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	results := make([]models.Result, 0, len(data.Results))

	for i, vid := range data.Results {
		if i >= e.GetConfig().GetMaxResults() {
			break
		}

		// Parse duration to seconds
		duration := parseDuration(vid.Duration)

		results = append(results, models.Result{
			Title:     vid.Title,
			URL:       vid.URL,
			Content:   vid.Description,
			Thumbnail: vid.Thumbnail,
			Author:    vid.Publisher,
			Engine:    e.Name(),
			Category:  models.CategoryVideos,
			Duration:  duration,
			ViewCount: vid.ViewCount,
			Score:     calculateScore(e.GetPriority(), i, 1),
			Position:  i,
			Metadata: map[string]interface{}{
				"published": vid.Published,
				"publisher": vid.Publisher,
			},
		})
	}

	return results, nil
}

// searchNews performs a news search using DuckDuckGo
func (e *DuckDuckGo) searchNews(ctx context.Context, query *models.Query) ([]models.Result, error) {
	// First, get a VQD token
	vqd, err := e.getVQDToken(ctx, query.Text)
	if err != nil {
		return nil, err
	}

	// DuckDuckGo News API
	apiURL := "https://duckduckgo.com/news.js"

	params := url.Values{}
	params.Set("q", query.Text)
	params.Set("o", "json")
	params.Set("vqd", vqd)

	// Time filter
	switch query.TimeRange {
	case "day":
		params.Set("df", "d")
	case "week":
		params.Set("df", "w")
	case "month":
		params.Set("df", "m")
	}

	reqURL := fmt.Sprintf("%s?%s", apiURL, params.Encode())

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Referer", "https://duckduckgo.com/")

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("DuckDuckGo news returned status %d", resp.StatusCode)
	}

	var data struct {
		Results []struct {
			Title   string `json:"title"`
			URL     string `json:"url"`
			Excerpt string `json:"excerpt"`
			Source  string `json:"source"`
			Image   string `json:"image"`
			Date    int64  `json:"date"` // Unix timestamp
		} `json:"results"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	results := make([]models.Result, 0, len(data.Results))

	for i, news := range data.Results {
		if i >= e.GetConfig().GetMaxResults() {
			break
		}

		publishedAt := time.Unix(news.Date, 0)

		results = append(results, models.Result{
			Title:       news.Title,
			URL:         news.URL,
			Content:     news.Excerpt,
			Thumbnail:   news.Image,
			Author:      news.Source,
			PublishedAt: publishedAt,
			Engine:      e.Name(),
			Category:    models.CategoryNews,
			Score:       calculateScore(e.GetPriority(), i, 1),
			Position:    i,
		})
	}

	return results, nil
}

// parseDuration converts duration string to seconds
func parseDuration(d string) int {
	// Common formats: "1:23", "1:23:45"
	parts := make([]int, 0)
	current := 0
	for _, c := range d {
		if c >= '0' && c <= '9' {
			current = current*10 + int(c-'0')
		} else if c == ':' {
			parts = append(parts, current)
			current = 0
		}
	}
	parts = append(parts, current)

	switch len(parts) {
	case 1:
		return parts[0]
	case 2:
		return parts[0]*60 + parts[1]
	case 3:
		return parts[0]*3600 + parts[1]*60 + parts[2]
	default:
		return 0
	}
}

// calculateScore calculates result score based on priority, position, and duplicates
func calculateScore(priority, position, duplicates int) float64 {
	return float64(priority*100) + float64(100-position) + float64(duplicates*50)
}

// extractTitle extracts title from DuckDuckGo result text
// DuckDuckGo results often have format "Title - Site Name" or "Title | Site Name"
func extractTitle(text string) string {
	text = strings.TrimSpace(text)

	// Try to find title before common separators
	// Check for " - " separator (most common)
	if idx := strings.Index(text, " - "); idx > 10 && idx < len(text)-5 {
		// Only split if we have meaningful content on both sides
		title := strings.TrimSpace(text[:idx])
		if len(title) > 5 {
			return title
		}
	}

	// Check for " | " separator
	if idx := strings.Index(text, " | "); idx > 10 && idx < len(text)-5 {
		title := strings.TrimSpace(text[:idx])
		if len(title) > 5 {
			return title
		}
	}

	// Check for " :: " separator (used by some sites)
	if idx := strings.Index(text, " :: "); idx > 10 && idx < len(text)-5 {
		title := strings.TrimSpace(text[:idx])
		if len(title) > 5 {
			return title
		}
	}

	// No separator found or too short - truncate if needed
	if len(text) > 100 {
		// Try to break at a word boundary
		truncated := text[:100]
		if lastSpace := strings.LastIndex(truncated, " "); lastSpace > 50 {
			return truncated[:lastSpace] + "..."
		}
		return truncated + "..."
	}

	return text
}
