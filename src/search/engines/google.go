package engines

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/apimgr/search/src/model"
	"github.com/apimgr/search/src/search"
)

// Google implements Google search engine
type Google struct {
	*search.BaseEngine
	client *http.Client
}

// NewGoogle creates a new Google engine
func NewGoogle() *Google {
	config := model.NewEngineConfig("google")
	config.DisplayName = "Google"
	config.Priority = 90 // Second priority after DuckDuckGo
	config.Categories = []string{"general", "images", "news", "videos"}
	config.SupportsTor = false // Google often blocks Tor

	return &Google{
		BaseEngine: search.NewBaseEngine(config),
		client: &http.Client{
			Timeout: time.Duration(config.GetTimeout()) * time.Second,
		},
	}
}

// Search performs a Google search
func (e *Google) Search(ctx context.Context, query *model.Query) ([]model.Result, error) {
	switch query.Category {
	case model.CategoryImages:
		return e.searchImages(ctx, query)
	case model.CategoryNews:
		return e.searchNews(ctx, query)
	case model.CategoryVideos:
		return e.searchVideos(ctx, query)
	default:
		return e.searchGeneral(ctx, query)
	}
}

// searchGeneral performs a general web search
func (e *Google) searchGeneral(ctx context.Context, query *model.Query) ([]model.Result, error) {
	// Google search URL
	baseURL := "https://www.google.com/search"

	params := url.Values{}
	params.Set("q", query.Text)
	params.Set("num", "20") // Results per page

	if query.Page > 1 {
		start := (query.Page - 1) * 20
		params.Set("start", fmt.Sprintf("%d", start))
	}

	// Safe search
	if query.SafeSearch == 2 {
		params.Set("safe", "active")
	}

	// Language
	if query.Language != "" {
		params.Set("hl", query.Language)
	}

	// Time range
	switch query.TimeRange {
	case "day":
		params.Set("tbs", "qdr:d")
	case "week":
		params.Set("tbs", "qdr:w")
	case "month":
		params.Set("tbs", "qdr:m")
	case "year":
		params.Set("tbs", "qdr:y")
	}

	reqURL := fmt.Sprintf("%s?%s", baseURL, params.Encode())

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, err
	}

	// Set realistic user agent
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36 Edg/131.0.0.0")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Google returned status %d", resp.StatusCode)
	}

	// Parse HTML response
	results, err := e.parseGoogleHTML(resp, query)
	if err != nil {
		return nil, err
	}

	return results, nil
}

// parseGoogleHTML parses Google search results from HTML
func (e *Google) parseGoogleHTML(resp *http.Response, query *model.Query) ([]model.Result, error) {
	// Note: This is a simplified parser. Google's HTML structure changes frequently.
	// In production, consider using a proper HTML parser like goquery

	results := make([]model.Result, 0)

	// Read response body
	body := make([]byte, 1024*1024) // 1MB max
	n, _ := resp.Body.Read(body)
	html := string(body[:n])

	// Extract search results using regex (simplified)
	// Google result structure: <div class="g">...</div>

	// Extract titles and URLs
	titlePattern := regexp.MustCompile(`<h3[^>]*>([^<]+)</h3>`)
	urlPattern := regexp.MustCompile(`<a href="(/url\?q=|https?://)[^"]*"`)
	snippetPattern := regexp.MustCompile(`<div class="[^"]*VwiC3b[^"]*"[^>]*>([^<]+)</div>`)

	titles := titlePattern.FindAllStringSubmatch(html, -1)
	urls := urlPattern.FindAllStringSubmatch(html, -1)
	snippets := snippetPattern.FindAllStringSubmatch(html, -1)

	maxResults := len(titles)
	if len(urls) < maxResults {
		maxResults = len(urls)
	}
	if maxResults > e.GetConfig().GetMaxResults() {
		maxResults = e.GetConfig().GetMaxResults()
	}

	for i := 0; i < maxResults; i++ {
		if i >= len(titles) || i >= len(urls) {
			break
		}

		title := cleanHTML(titles[i][1])
		resultURL := extractGoogleURL(urls[i][0])

		// Skip if URL is empty or is Google itself
		if resultURL == "" || strings.Contains(resultURL, "google.com") {
			continue
		}

		content := ""
		if i < len(snippets) {
			content = cleanHTML(snippets[i][1])
		}

		results = append(results, model.Result{
			Title:    title,
			URL:      resultURL,
			Content:  content,
			Engine:   e.Name(),
			Category: model.CategoryGeneral,
			Score:    calculateScore(e.GetPriority(), i, 1),
			Position: i,
		})
	}

	return results, nil
}

// searchImages performs an image search
func (e *Google) searchImages(ctx context.Context, query *model.Query) ([]model.Result, error) {
	// Google Images search URL
	baseURL := "https://www.google.com/search"

	params := url.Values{}
	params.Set("q", query.Text)
	params.Set("tbm", "isch") // Image search
	params.Set("num", "20")

	if query.Page > 1 {
		start := (query.Page - 1) * 20
		params.Set("start", fmt.Sprintf("%d", start))
	}

	// Safe search
	if query.SafeSearch == 2 {
		params.Set("safe", "active")
	}

	// Language
	if query.Language != "" {
		params.Set("hl", query.Language)
	}

	reqURL := fmt.Sprintf("%s?%s", baseURL, params.Encode())

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Google Images returned status %d", resp.StatusCode)
	}

	return e.parseImageResults(resp, query)
}

// parseImageResults parses Google Images search results
func (e *Google) parseImageResults(resp *http.Response, query *model.Query) ([]model.Result, error) {
	results := make([]model.Result, 0)

	body := make([]byte, 2*1024*1024) // 2MB max for images page
	n, _ := resp.Body.Read(body)
	html := string(body[:n])

	// Google Images embeds image data in JSON within the page
	// Pattern to extract image URLs and metadata
	imgPattern := regexp.MustCompile(`\["(https?://[^"]+\.(jpg|jpeg|png|gif|webp)[^"]*)",\s*(\d+),\s*(\d+)\]`)
	titlePattern := regexp.MustCompile(`\["([^"]{10,100})","[^"]*"\]`)

	imgMatches := imgPattern.FindAllStringSubmatch(html, -1)
	titleMatches := titlePattern.FindAllStringSubmatch(html, -1)

	maxResults := len(imgMatches)
	if maxResults > e.GetConfig().GetMaxResults() {
		maxResults = e.GetConfig().GetMaxResults()
	}

	for i := 0; i < maxResults; i++ {
		imgURL := imgMatches[i][1]

		// Skip Google's own images
		if strings.Contains(imgURL, "gstatic.com") || strings.Contains(imgURL, "google.com") {
			continue
		}

		title := fmt.Sprintf("Image %d", i+1)
		if i < len(titleMatches) {
			title = cleanHTML(titleMatches[i][1])
		}

		// Extract dimensions
		width := imgMatches[i][3]
		height := imgMatches[i][4]

		results = append(results, model.Result{
			Title:     title,
			URL:       imgURL,
			Thumbnail: imgURL,
			Content:   fmt.Sprintf("%sx%s", width, height),
			Engine:    e.Name(),
			Category:  model.CategoryImages,
			Score:     calculateScore(e.GetPriority(), i, 1),
			Position:  i,
		})
	}

	return results, nil
}

// searchNews performs a news search
func (e *Google) searchNews(ctx context.Context, query *model.Query) ([]model.Result, error) {
	// Google News search URL
	baseURL := "https://www.google.com/search"

	params := url.Values{}
	params.Set("q", query.Text)
	params.Set("tbm", "nws") // News search
	params.Set("num", "20")

	if query.Page > 1 {
		start := (query.Page - 1) * 20
		params.Set("start", fmt.Sprintf("%d", start))
	}

	// Safe search
	if query.SafeSearch == 2 {
		params.Set("safe", "active")
	}

	// Language
	if query.Language != "" {
		params.Set("hl", query.Language)
	}

	// Time range for news
	switch query.TimeRange {
	case "day":
		params.Set("tbs", "qdr:d")
	case "week":
		params.Set("tbs", "qdr:w")
	case "month":
		params.Set("tbs", "qdr:m")
	case "year":
		params.Set("tbs", "qdr:y")
	}

	reqURL := fmt.Sprintf("%s?%s", baseURL, params.Encode())

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Google News returned status %d", resp.StatusCode)
	}

	return e.parseNewsResults(resp, query)
}

// parseNewsResults parses Google News search results
func (e *Google) parseNewsResults(resp *http.Response, query *model.Query) ([]model.Result, error) {
	results := make([]model.Result, 0)

	body := make([]byte, 1024*1024) // 1MB max
	n, _ := resp.Body.Read(body)
	html := string(body[:n])

	// Google News result patterns
	// News results have a distinct structure with source and time info
	titlePattern := regexp.MustCompile(`<div[^>]*role="heading"[^>]*>([^<]+)</div>`)
	urlPattern := regexp.MustCompile(`<a[^>]*href="(https?://(?!google\.com)[^"]+)"[^>]*>`)
	snippetPattern := regexp.MustCompile(`<div[^>]*class="[^"]*GI74Re[^"]*"[^>]*>([^<]+)</div>`)
	sourcePattern := regexp.MustCompile(`<div[^>]*class="[^"]*CEMjEf[^"]*"[^>]*>([^<]+)</div>`)
	timePattern := regexp.MustCompile(`<span[^>]*class="[^"]*WG9SHc[^"]*"[^>]*>([^<]+)</span>`)

	titles := titlePattern.FindAllStringSubmatch(html, -1)
	urls := urlPattern.FindAllStringSubmatch(html, -1)
	snippets := snippetPattern.FindAllStringSubmatch(html, -1)
	sources := sourcePattern.FindAllStringSubmatch(html, -1)
	times := timePattern.FindAllStringSubmatch(html, -1)

	maxResults := len(titles)
	if len(urls) < maxResults {
		maxResults = len(urls)
	}
	if maxResults > e.GetConfig().GetMaxResults() {
		maxResults = e.GetConfig().GetMaxResults()
	}

	for i := 0; i < maxResults; i++ {
		if i >= len(titles) || i >= len(urls) {
			break
		}

		title := cleanHTML(titles[i][1])
		resultURL := urls[i][1]

		// Skip Google's own URLs
		if strings.Contains(resultURL, "google.com") {
			continue
		}

		content := ""
		if i < len(snippets) {
			content = cleanHTML(snippets[i][1])
		}

		// Add source and time info
		source := ""
		if i < len(sources) {
			source = cleanHTML(sources[i][1])
		}
		publishedTime := ""
		if i < len(times) {
			publishedTime = cleanHTML(times[i][1])
		}

		if source != "" || publishedTime != "" {
			content = fmt.Sprintf("%s — %s | %s", source, publishedTime, content)
		}

		results = append(results, model.Result{
			Title:    title,
			URL:      resultURL,
			Content:  content,
			Engine:   e.Name(),
			Category: model.CategoryNews,
			Score:    calculateScore(e.GetPriority(), i, 1),
			Position: i,
		})
	}

	return results, nil
}

// searchVideos performs a video search
func (e *Google) searchVideos(ctx context.Context, query *model.Query) ([]model.Result, error) {
	// Google Videos search URL
	baseURL := "https://www.google.com/search"

	params := url.Values{}
	params.Set("q", query.Text)
	params.Set("tbm", "vid") // Video search
	params.Set("num", "20")

	if query.Page > 1 {
		start := (query.Page - 1) * 20
		params.Set("start", fmt.Sprintf("%d", start))
	}

	// Safe search
	if query.SafeSearch == 2 {
		params.Set("safe", "active")
	}

	// Language
	if query.Language != "" {
		params.Set("hl", query.Language)
	}

	// Time range
	switch query.TimeRange {
	case "day":
		params.Set("tbs", "qdr:d")
	case "week":
		params.Set("tbs", "qdr:w")
	case "month":
		params.Set("tbs", "qdr:m")
	case "year":
		params.Set("tbs", "qdr:y")
	}

	reqURL := fmt.Sprintf("%s?%s", baseURL, params.Encode())

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Google Videos returned status %d", resp.StatusCode)
	}

	return e.parseVideoResults(resp, query)
}

// parseVideoResults parses Google Videos search results
func (e *Google) parseVideoResults(resp *http.Response, query *model.Query) ([]model.Result, error) {
	results := make([]model.Result, 0)

	body := make([]byte, 1024*1024) // 1MB max
	n, _ := resp.Body.Read(body)
	html := string(body[:n])

	// Google Video result patterns
	titlePattern := regexp.MustCompile(`<h3[^>]*>([^<]+)</h3>`)
	urlPattern := regexp.MustCompile(`<a[^>]*href="(https?://(?:www\.)?(?:youtube\.com|vimeo\.com|dailymotion\.com)[^"]+)"`)
	durationPattern := regexp.MustCompile(`<div[^>]*class="[^"]*J1mWY[^"]*"[^>]*>([^<]+)</div>`)
	channelPattern := regexp.MustCompile(`<div[^>]*class="[^"]*Zg1NU[^"]*"[^>]*>([^<]+)</div>`)
	thumbPattern := regexp.MustCompile(`<img[^>]*src="(https?://[^"]*(?:ytimg|vimeocdn|dmcdn)[^"]*)"`)

	titles := titlePattern.FindAllStringSubmatch(html, -1)
	urls := urlPattern.FindAllStringSubmatch(html, -1)
	durations := durationPattern.FindAllStringSubmatch(html, -1)
	channels := channelPattern.FindAllStringSubmatch(html, -1)
	thumbs := thumbPattern.FindAllStringSubmatch(html, -1)

	maxResults := len(urls)
	if maxResults > e.GetConfig().GetMaxResults() {
		maxResults = e.GetConfig().GetMaxResults()
	}

	for i := 0; i < maxResults; i++ {
		if i >= len(urls) {
			break
		}

		resultURL := urls[i][1]

		title := fmt.Sprintf("Video %d", i+1)
		if i < len(titles) {
			title = cleanHTML(titles[i][1])
		}

		// Build content with duration and channel info
		content := ""
		if i < len(durations) {
			content = cleanHTML(durations[i][1])
		}
		if i < len(channels) {
			channel := cleanHTML(channels[i][1])
			if content != "" {
				content = fmt.Sprintf("%s — %s", channel, content)
			} else {
				content = channel
			}
		}

		thumbnailURL := ""
		if i < len(thumbs) {
			thumbnailURL = thumbs[i][1]
		}

		results = append(results, model.Result{
			Title:     title,
			URL:       resultURL,
			Thumbnail: thumbnailURL,
			Content:   content,
			Engine:    e.Name(),
			Category:  model.CategoryVideos,
			Score:     calculateScore(e.GetPriority(), i, 1),
			Position:  i,
		})
	}

	return results, nil
}

// extractGoogleURL extracts the actual URL from Google's redirect URL
func extractGoogleURL(googleURL string) string {
	// Google wraps URLs like: /url?q=https://example.com&sa=...
	if strings.Contains(googleURL, "/url?q=") {
		parts := strings.Split(googleURL, "q=")
		if len(parts) > 1 {
			urlPart := strings.Split(parts[1], "&")[0]
			decoded, err := url.QueryUnescape(urlPart)
			if err == nil {
				return decoded
			}
		}
	}

	// Direct URL
	if strings.HasPrefix(googleURL, "http") {
		return strings.Trim(googleURL, `"`)
	}

	return ""
}

// cleanHTML removes HTML tags and entities
func cleanHTML(text string) string {
	// Remove HTML tags
	re := regexp.MustCompile(`<[^>]*>`)
	text = re.ReplaceAllString(text, "")

	// Decode common HTML entities
	text = strings.ReplaceAll(text, "&amp;", "&")
	text = strings.ReplaceAll(text, "&lt;", "<")
	text = strings.ReplaceAll(text, "&gt;", ">")
	text = strings.ReplaceAll(text, "&quot;", `"`)
	text = strings.ReplaceAll(text, "&#39;", "'")
	text = strings.ReplaceAll(text, "&nbsp;", " ")

	// Trim whitespace
	text = strings.TrimSpace(text)

	return text
}
