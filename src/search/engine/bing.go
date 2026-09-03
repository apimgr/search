package engine

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/apimgr/search/src/model"
	"github.com/apimgr/search/src/search"
)

// BingEngine implements Bing search
type BingEngine struct {
	*search.BaseEngine
	client *http.Client
}

// NewBing creates a new Bing engine
func NewBing() *BingEngine {
	config := model.NewEngineConfig("bing")
	config.DisplayName = "Bing"
	config.Priority = 80
	config.Categories = []string{"general", "images", "news", "videos", "files", "music"}
	config.SupportsTor = false

	client := &http.Client{
		Timeout:   time.Duration(config.GetTimeout()) * time.Second,
		Transport: SharedTransport,
	}

	return &BingEngine{
		BaseEngine: search.NewBaseEngine(config),
		client:     client,
	}
}

func (e *BingEngine) Search(ctx context.Context, query *model.Query) ([]model.Result, error) {
	if !e.IsEnabled() {
		return nil, model.ErrEngineDisabled
	}

	// Build search URL
	searchURL := e.buildURL(query)

	// Create request
	req, err := http.NewRequestWithContext(ctx, "GET", searchURL, nil)
	if err != nil {
		return nil, err
	}

	// Set headers to mimic browser
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")

	// Perform request
	resp, err := e.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Parse results
	return e.parseResults(string(body), query)
}

func (e *BingEngine) buildURL(query *model.Query) string {
	params := url.Values{}
	params.Set("q", query.Text)
	params.Set("first", fmt.Sprintf("%d", (query.Page-1)*10+1))

	return fmt.Sprintf("https://www.bing.com/search?%s", params.Encode())
}

func (e *BingEngine) parseResults(html string, query *model.Query) ([]model.Result, error) {
	results := make([]model.Result, 0)

	// Regex patterns for Bing results
	// Bing uses <li class="b_algo"> for organic results, followed by extra
	// attributes (e.g. `data-id iid=SERP.5331`) before the tag actually
	// closes, so the closing ">" cannot be assumed to immediately follow the
	// class attribute's closing quote.
	resultPattern := regexp.MustCompile(`(?s)<li class="b_algo[^"]*"[^>]*>(.*?)</li>`)
	// The <h2> always carries a class attribute (e.g. <h2 class="">), and the
	// title text frequently wraps matched query terms in <strong> tags, so the
	// tag and the capture group must both tolerate that instead of requiring
	// an exact "<h2><a" prefix and tag-free text.
	titlePattern := regexp.MustCompile(`(?s)<h2[^>]*><a[^>]*href="([^"]*)"[^>]*>(.*?)</a></h2>`)
	contentPattern := regexp.MustCompile(`<p[^>]*>(.*?)</p>`)

	matches := resultPattern.FindAllStringSubmatch(html, -1)

	for i, match := range matches {
		// Limit to 20 results
		if i >= 20 {
			break
		}

		resultHTML := match[1]

		// Extract title and URL
		titleMatch := titlePattern.FindStringSubmatch(resultHTML)
		if titleMatch == nil {
			continue
		}

		resultURL := titleMatch[1]
		title := cleanHTML(titleMatch[2])

		// Skip Bing internal URLs
		if strings.Contains(resultURL, "bing.com") && !strings.Contains(resultURL, "http") {
			continue
		}

		// Bing wraps organic results behind a bing.com/ck/a tracking redirect
		// whose real target is base64-encoded in the "u" query parameter, so
		// resolve it to the actual destination URL before returning it. The
		// href attribute HTML-escapes "&" as "&amp;", which must be undone
		// first so the query-parameter boundary regex can find "u=".
		if strings.Contains(resultURL, "bing.com/ck/a") {
			if realURL := extractBingRedirectURL(unescapeHTML(resultURL)); realURL != "" {
				resultURL = realURL
			}
		}

		// Extract content/snippet
		content := ""
		contentMatch := contentPattern.FindStringSubmatch(resultHTML)
		if contentMatch != nil {
			content = cleanHTML(contentMatch[1])
		}

		// Calculate score based on position
		score := float64(1000 - i*10)

		result := model.Result{
			Title:    title,
			URL:      resultURL,
			Content:  content,
			Engine:   e.Name(),
			Category: query.Category,
			Score:    score,
		}

		results = append(results, result)
	}

	if len(results) == 0 {
		return nil, model.ErrNoResults
	}

	return results, nil
}

// bingRedirectURe extracts the "u" query parameter from a Bing
// bing.com/ck/a tracking-redirect URL, which holds the real destination
// URL base64-encoded with a leading two-character version marker
// (e.g. "u=a1aHR0cHM6Ly9nby5kZXYv" decodes to "https://go.dev/").
var bingRedirectURe = regexp.MustCompile(`[?&]u=([^&]+)`)

// extractBingRedirectURL extracts the real URL from a Bing ck/a redirect URL
func extractBingRedirectURL(redirectURL string) string {
	match := bingRedirectURe.FindStringSubmatch(redirectURL)
	if len(match) < 2 {
		return ""
	}

	encoded := match[1]
	// Strip the leading version marker (e.g. "a1") before the base64 payload.
	if len(encoded) < 3 {
		return ""
	}
	encoded = encoded[2:]

	// Restore standard base64 padding stripped from the URL-safe payload.
	if pad := len(encoded) % 4; pad != 0 {
		encoded += strings.Repeat("=", 4-pad)
	}

	decoded, err := base64.URLEncoding.DecodeString(encoded)
	if err != nil {
		decoded, err = base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			return ""
		}
	}

	return string(decoded)
}
