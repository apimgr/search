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

// Pre-compiled regexes for Google HTML parsing.
//
// Strategy: split the page on `<div class="g"` to isolate each organic
// result block, then use structural patterns (anchor-containing-h3) rather
// than hashed CSS class names which Google rotates frequently.
var (
	// gResultRe finds the redirect href AND title in one pass.
	// Google wraps every web result as: <a href="/url?q=..."><h3>Title</h3>
	gResultRe = regexp.MustCompile(`<a[^>]+href="(/url\?[^"]{1,600})"[^>]*>[\s\S]{0,300}?<h3[^>]*>([\s\S]{0,300}?)</h3>`)

	// gNewsHeadingRe matches Google News headings (role="heading" is stable).
	gNewsHeadingRe = regexp.MustCompile(`<div[^>]+role="heading"[^>]*>([\s\S]{1,300}?)</div>`)

	// gDirectURLRe catches direct absolute links (used in news/video blocks).
	// Google-owned URLs are filtered in Go with isGoogleURL because Go's regexp
	// engine does not support Perl-style negative lookaheads.
	gDirectURLRe = regexp.MustCompile(`href="(https?://[^"]{5,500})"`)

	// gImgJSONRe extracts image metadata embedded as JSON arrays in Google Images.
	gImgJSONRe = regexp.MustCompile(`\["(https?://[^"]+\.(?:jpg|jpeg|png|gif|webp)[^"]*)",(\d+),(\d+)\]`)

	// gTagRe strips HTML tags for plain-text extraction.
	gTagRe = regexp.MustCompile(`<[^>]+>`)
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
	config.Priority = 90
	config.Categories = []string{"general", "images", "news", "videos", "files", "music"}
	config.SupportsTor = false // Google blocks Tor exit nodes

	return &Google{
		BaseEngine: search.NewBaseEngine(config),
		client: &http.Client{
			Timeout:   time.Duration(config.GetTimeout()) * time.Second,
			Transport: SharedTransport,
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

// googleDo builds and executes a Google request with realistic browser headers.
// The SOCS cookie bypasses Google's GDPR consent page.
func (e *Google) googleDo(ctx context.Context, rawURL string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("DNT", "1")
	req.Header.Set("Upgrade-Insecure-Requests", "1")
	req.Header.Set("Sec-Fetch-Dest", "document")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	req.Header.Set("Sec-Fetch-Site", "none")
	req.Header.Set("Cookie", "SOCS=CAI") // consent bypass
	return e.client.Do(req)
}

// googleParams builds the common query parameters for all Google searches.
func googleParams(query *model.Query) url.Values {
	params := url.Values{}
	params.Set("q", query.Text)
	params.Set("num", "20")
	params.Set("hl", "en")
	if query.Page > 1 {
		params.Set("start", fmt.Sprintf("%d", (query.Page-1)*20))
	}
	if query.SafeSearch == 2 {
		params.Set("safe", "active")
	}
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
	return params
}

// searchGeneral performs a general web search
func (e *Google) searchGeneral(ctx context.Context, query *model.Query) ([]model.Result, error) {
	params := googleParams(query)
	reqURL := "https://www.google.com/search?" + params.Encode()

	resp, err := e.googleDo(ctx, reqURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("google returned status %d", resp.StatusCode)
	}

	body, err := ReadBody(resp)
	if err != nil {
		return nil, err
	}
	return e.parseWebResults(string(body), query), nil
}

// parseWebResults parses organic results from a Google web search page.
// Uses the structural "anchor containing h3" pattern which is more stable
// than CSS class names that Google obfuscates and rotates.
func (e *Google) parseWebResults(html string, query *model.Query) []model.Result {
	matches := gResultRe.FindAllStringSubmatchIndex(html, -1)
	maxResults := e.GetConfig().GetMaxResults()
	results := make([]model.Result, 0, min(len(matches), maxResults))

	for _, idx := range matches {
		if len(results) >= maxResults {
			break
		}

		href := html[idx[2]:idx[3]]
		rawTitle := html[idx[4]:idx[5]]

		realURL := googleDecodeRedirect(href)
		if realURL == "" || isGoogleURL(realURL) {
			continue
		}

		title := googleCleanHTML(rawTitle)
		if title == "" {
			continue
		}

		// Extract snippet from the text immediately following the title closing tag.
		snippetStart := idx[1]
		snippetEnd := snippetStart + 600
		if snippetEnd > len(html) {
			snippetEnd = len(html)
		}
		snippet := extractTextBlock(html[snippetStart:snippetEnd])

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
	return results
}

// searchImages performs a Google Images search
func (e *Google) searchImages(ctx context.Context, query *model.Query) ([]model.Result, error) {
	params := googleParams(query)
	params.Set("tbm", "isch")
	reqURL := "https://www.google.com/search?" + params.Encode()

	resp, err := e.googleDo(ctx, reqURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("google images returned status %d", resp.StatusCode)
	}

	body, err := ReadBody(resp)
	if err != nil {
		return nil, err
	}
	return e.parseImageResults(string(body), query), nil
}

// parseImageResults parses Google Images results from embedded JSON arrays.
func (e *Google) parseImageResults(html string, query *model.Query) []model.Result {
	matches := gImgJSONRe.FindAllStringSubmatch(html, -1)
	maxResults := e.GetConfig().GetMaxResults()
	results := make([]model.Result, 0, min(len(matches), maxResults))

	for _, m := range matches {
		if len(results) >= maxResults {
			break
		}
		imgURL := m[1]
		if strings.Contains(imgURL, "gstatic.com") || strings.Contains(imgURL, "google.com") {
			continue
		}
		results = append(results, model.Result{
			Title:     fmt.Sprintf("Image %d", len(results)+1),
			URL:       imgURL,
			Thumbnail: imgURL,
			Content:   fmt.Sprintf("%s×%s", m[2], m[3]),
			Engine:    e.Name(),
			Category:  model.CategoryImages,
			Score:     calculateScore(e.GetPriority(), len(results), 1),
			Position:  len(results),
		})
	}
	return results
}

// searchNews performs a Google News search
func (e *Google) searchNews(ctx context.Context, query *model.Query) ([]model.Result, error) {
	params := googleParams(query)
	params.Set("tbm", "nws")
	reqURL := "https://www.google.com/search?" + params.Encode()

	resp, err := e.googleDo(ctx, reqURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("google news returned status %d", resp.StatusCode)
	}

	body, err := ReadBody(resp)
	if err != nil {
		return nil, err
	}
	return e.parseNewsResults(string(body), query), nil
}

// parseNewsResults parses Google News results.
// Uses role="heading" (stable ARIA attribute) for titles and direct URL extraction.
func (e *Google) parseNewsResults(html string, query *model.Query) []model.Result {
	headings := gNewsHeadingRe.FindAllStringSubmatch(html, -1)
	urlMatches := gDirectURLRe.FindAllStringSubmatch(html, -1)

	maxResults := e.GetConfig().GetMaxResults()
	results := make([]model.Result, 0)

	urlIdx := 0
	for _, h := range headings {
		if len(results) >= maxResults {
			break
		}
		title := googleCleanHTML(h[1])
		if title == "" {
			continue
		}

		// Find next non-Google URL
		resultURL := ""
		for urlIdx < len(urlMatches) {
			candidate := urlMatches[urlIdx][1]
			urlIdx++
			if !isGoogleURL(candidate) {
				resultURL = candidate
				break
			}
		}
		if resultURL == "" {
			continue
		}

		results = append(results, model.Result{
			Title:    title,
			URL:      resultURL,
			Engine:   e.Name(),
			Category: model.CategoryNews,
			Score:    calculateScore(e.GetPriority(), len(results), 1),
			Position: len(results),
		})
	}
	return results
}

// searchVideos performs a Google Videos search
func (e *Google) searchVideos(ctx context.Context, query *model.Query) ([]model.Result, error) {
	params := googleParams(query)
	params.Set("tbm", "vid")
	reqURL := "https://www.google.com/search?" + params.Encode()

	resp, err := e.googleDo(ctx, reqURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("google videos returned status %d", resp.StatusCode)
	}

	body, err := ReadBody(resp)
	if err != nil {
		return nil, err
	}
	return e.parseVideoResults(string(body), query), nil
}

// parseVideoResults parses Google Videos results using the same structural
// anchor-containing-h3 pattern as web results, filtering to video host URLs.
func (e *Google) parseVideoResults(html string, query *model.Query) []model.Result {
	// Reuse the same structural matcher; video results also use /url?q= redirects.
	matches := gResultRe.FindAllStringSubmatch(html, -1)
	maxResults := e.GetConfig().GetMaxResults()
	results := make([]model.Result, 0)

	for _, m := range matches {
		if len(results) >= maxResults {
			break
		}
		realURL := googleDecodeRedirect(m[1])
		if realURL == "" || !isVideoURL(realURL) {
			continue
		}
		title := googleCleanHTML(m[2])
		if title == "" {
			continue
		}
		results = append(results, model.Result{
			Title:    title,
			URL:      realURL,
			Engine:   e.Name(),
			Category: model.CategoryVideos,
			Score:    calculateScore(e.GetPriority(), len(results), 1),
			Position: len(results),
		})
	}
	return results
}

// googleDecodeRedirect extracts the real URL from a Google /url?q= redirect.
func googleDecodeRedirect(href string) string {
	// href is already the value inside the quotes, e.g. "/url?q=https%3A//..."
	if !strings.Contains(href, "/url?") {
		return ""
	}
	// Parse as a URL to get the q= parameter.
	full := "https://www.google.com" + href
	u, err := url.Parse(full)
	if err != nil {
		return ""
	}
	if q := u.Query().Get("q"); q != "" {
		return q
	}
	return ""
}

// isGoogleURL returns true if the URL belongs to Google's own domains.
func isGoogleURL(rawURL string) bool {
	return strings.Contains(rawURL, "google.com") ||
		strings.Contains(rawURL, "google.co.") ||
		strings.Contains(rawURL, "googleapis.com") ||
		strings.Contains(rawURL, "gstatic.com") ||
		strings.HasPrefix(rawURL, "/")
}

// isVideoURL returns true if the URL is a known video hosting site.
func isVideoURL(rawURL string) bool {
	return strings.Contains(rawURL, "youtube.com") ||
		strings.Contains(rawURL, "youtu.be") ||
		strings.Contains(rawURL, "vimeo.com") ||
		strings.Contains(rawURL, "dailymotion.com") ||
		strings.Contains(rawURL, "twitch.tv")
}

// extractTextBlock strips tags from a short HTML fragment and returns the
// first meaningful run of prose text (≥20 chars, not a bare URL).
func extractTextBlock(fragment string) string {
	text := gTagRe.ReplaceAllString(fragment, " ")
	text = googleDecodeEntities(text)
	words := strings.Fields(text)

	var parts []string
	for _, w := range words {
		if strings.HasPrefix(w, "http") {
			if len(parts) > 0 {
				break
			}
			continue
		}
		parts = append(parts, w)
		if len(strings.Join(parts, " ")) >= 300 {
			break
		}
	}
	return strings.TrimSpace(strings.Join(parts, " "))
}

// googleCleanHTML removes tags and decodes entities from a small HTML fragment.
func googleCleanHTML(s string) string {
	s = gTagRe.ReplaceAllString(s, "")
	return strings.TrimSpace(googleDecodeEntities(s))
}

// googleDecodeEntities decodes common HTML entities.
func googleDecodeEntities(s string) string {
	s = strings.ReplaceAll(s, "&amp;", "&")
	s = strings.ReplaceAll(s, "&lt;", "<")
	s = strings.ReplaceAll(s, "&gt;", ">")
	s = strings.ReplaceAll(s, "&quot;", `"`)
	s = strings.ReplaceAll(s, "&#39;", "'")
	s = strings.ReplaceAll(s, "&#x27;", "'")
	s = strings.ReplaceAll(s, "&nbsp;", " ")
	return s
}

// cleanHTML is kept for compatibility with other files that call it.
func cleanHTML(text string) string {
	return googleCleanHTML(text)
}

// extractGoogleURL is kept for compatibility.
func extractGoogleURL(googleURL string) string {
	googleURL = googleDecodeEntities(strings.TrimSpace(strings.Trim(googleURL, `"`)))
	if strings.HasPrefix(googleURL, "http://") || strings.HasPrefix(googleURL, "https://") {
		return googleURL
	}
	return googleDecodeRedirect(googleURL)
}
