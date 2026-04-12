package engines

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/apimgr/search/src/model"
	"github.com/apimgr/search/src/search"
)

// DuckDuckGo implements DuckDuckGo search engine
type DuckDuckGo struct {
	*search.BaseEngine
	client *http.Client
}

// NewDuckDuckGo creates a new DuckDuckGo engine
func NewDuckDuckGo() *DuckDuckGo {
	config := model.NewEngineConfig("duckduckgo")
	config.DisplayName = "DuckDuckGo"
	config.Priority = 100 // Highest priority (default engine)
	config.Categories = []string{"general", "images", "videos", "news"}
	config.SupportsTor = true

	return &DuckDuckGo{
		BaseEngine: search.NewBaseEngine(config),
		client: &http.Client{
			Timeout: time.Duration(config.GetTimeout()) * time.Second,
			Transport: SharedTransport,
		},
	}
}

// Search performs a DuckDuckGo search
func (e *DuckDuckGo) Search(ctx context.Context, query *model.Query) ([]model.Result, error) {
	switch query.Category {
	case model.CategoryImages:
		return e.searchImages(ctx, query)
	case model.CategoryVideos:
		return e.searchVideos(ctx, query)
	case model.CategoryNews:
		return e.searchNews(ctx, query)
	default:
		return e.searchGeneral(ctx, query)
	}
}

// searchGeneral performs a general web search using DuckDuckGo HTML endpoint.
// The Instant Answer API (api.duckduckgo.com) only returns category/disambiguation
// pages, not real web results, so we scrape the HTML search page instead.
func (e *DuckDuckGo) searchGeneral(ctx context.Context, query *model.Query) ([]model.Result, error) {
	params := url.Values{}
	params.Set("q", query.Text)
	params.Set("kl", "us-en")

	switch query.SafeSearch {
	case 0:
		params.Set("kp", "-2") // Off
	case 2:
		params.Set("kp", "1") // Strict
	default:
		params.Set("kp", "-1") // Moderate
	}

	reqURL := "https://html.duckduckgo.com/html/?" + params.Encode()

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.5")

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("duckduckgo returned status %d", resp.StatusCode)
	}

	body, err := ReadBody(resp)
	if err != nil {
		return nil, err
	}

	return e.parseWebResults(string(body), query)
}

var (
	ddgTitleRe   = regexp.MustCompile(`(?i)<a[^>]+class="result__a"[^>]+href="([^"]*)"[^>]*>([\s\S]*?)</a>`)
	ddgSnippetRe = regexp.MustCompile(`(?i)class="result__snippet"[^>]*>([\s\S]*?)</(?:a|div|p)>`)
	ddgTagRe     = regexp.MustCompile(`<[^>]+>`)
)

func (e *DuckDuckGo) parseWebResults(html string, query *model.Query) ([]model.Result, error) {
	titleMatches := ddgTitleRe.FindAllStringSubmatch(html, -1)
	snippetMatches := ddgSnippetRe.FindAllStringSubmatch(html, -1)

	maxResults := e.GetConfig().GetMaxResults()
	results := make([]model.Result, 0, min(len(titleMatches), maxResults))

	for i, m := range titleMatches {
		if len(results) >= maxResults {
			break
		}

		realURL := ddgDecodeRedirect(m[1])
		if realURL == "" {
			continue
		}

		title := ddgStripHTML(m[2])
		snippet := ""
		if i < len(snippetMatches) {
			snippet = ddgStripHTML(snippetMatches[i][1])
		}

		results = append(results, model.Result{
			Title:    title,
			URL:      realURL,
			Content:  snippet,
			Engine:   e.Name(),
			Category: model.CategoryGeneral,
			Score:    calculateScore(e.GetPriority(), len(results), 1),
			Position: len(results),
		})
	}

	return results, nil
}

// ddgDecodeRedirect extracts the real destination URL from a DDG redirect href.
// DDG wraps outbound links as: //duckduckgo.com/l/?uddg=<encoded_url>&rut=...
func ddgDecodeRedirect(href string) string {
	if strings.HasPrefix(href, "//") {
		href = "https:" + href
	} else if strings.HasPrefix(href, "/") {
		href = "https://duckduckgo.com" + href
	}

	u, err := url.Parse(href)
	if err != nil {
		return ""
	}

	if uddg := u.Query().Get("uddg"); uddg != "" {
		return uddg
	}

	// Not a redirect — return only if it's an external URL
	if u.Host != "" && u.Host != "duckduckgo.com" {
		return href
	}

	return ""
}

// ddgStripHTML removes HTML tags and decodes common entities.
func ddgStripHTML(s string) string {
	s = ddgTagRe.ReplaceAllString(s, "")
	s = strings.ReplaceAll(s, "&amp;", "&")
	s = strings.ReplaceAll(s, "&lt;", "<")
	s = strings.ReplaceAll(s, "&gt;", ">")
	s = strings.ReplaceAll(s, "&quot;", `"`)
	s = strings.ReplaceAll(s, "&#39;", "'")
	s = strings.ReplaceAll(s, "&#x27;", "'")
	s = strings.ReplaceAll(s, "&#x2F;", "/")
	s = strings.ReplaceAll(s, "&nbsp;", " ")
	return strings.TrimSpace(s)
}

// searchImages performs an image search using DuckDuckGo images
func (e *DuckDuckGo) searchImages(ctx context.Context, query *model.Query) ([]model.Result, error) {
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

	req.Header.Set("User-Agent", UserAgent)
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

	results := make([]model.Result, 0, len(data.Results))

	for i, img := range data.Results {
		if i >= e.GetConfig().GetMaxResults() {
			break
		}

		results = append(results, model.Result{
			Title:       img.Title,
			URL:         img.URL,
			Thumbnail:   img.Thumbnail,
			Content:     fmt.Sprintf("%dx%d - %s", img.Width, img.Height, img.Source),
			Engine:      e.Name(),
			Category:    model.CategoryImages,
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

	req.Header.Set("User-Agent", UserAgent)

	resp, err := e.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	// Read body to extract vqd token
	body, err := ReadBody(resp)
	if err != nil {
		return "", fmt.Errorf("reading body: %w", err)
	}
	html := string(body)

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
func (e *DuckDuckGo) searchVideos(ctx context.Context, query *model.Query) ([]model.Result, error) {
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

	req.Header.Set("User-Agent", UserAgent)
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

	results := make([]model.Result, 0, len(data.Results))

	for i, vid := range data.Results {
		if i >= e.GetConfig().GetMaxResults() {
			break
		}

		// Parse duration to seconds
		duration := parseDuration(vid.Duration)

		results = append(results, model.Result{
			Title:     vid.Title,
			URL:       vid.URL,
			Content:   vid.Description,
			Thumbnail: vid.Thumbnail,
			Author:    vid.Publisher,
			Engine:    e.Name(),
			Category:  model.CategoryVideos,
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
func (e *DuckDuckGo) searchNews(ctx context.Context, query *model.Query) ([]model.Result, error) {
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

	req.Header.Set("User-Agent", UserAgent)
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

	results := make([]model.Result, 0, len(data.Results))

	for i, news := range data.Results {
		if i >= e.GetConfig().GetMaxResults() {
			break
		}

		publishedAt := time.Unix(news.Date, 0)

		results = append(results, model.Result{
			Title:       news.Title,
			URL:         news.URL,
			Content:     news.Excerpt,
			Thumbnail:   news.Image,
			Author:      news.Source,
			PublishedAt: publishedAt,
			Engine:      e.Name(),
			Category:    model.CategoryNews,
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
