package engines

import (
	"context"
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
	config.Categories = []string{"general", "images", "news", "videos"}
	config.SupportsTor = false

	client := &http.Client{
		Timeout: time.Duration(config.GetTimeout()) * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     90 * time.Second,
		},
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
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36 Edg/131.0.0.0")
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
	// Bing uses <li class="b_algo"> for organic results
	resultPattern := regexp.MustCompile(`(?s)<li class="b_algo[^"]*">(.*?)</li>`)
	titlePattern := regexp.MustCompile(`<h2><a[^>]*href="([^"]*)"[^>]*>([^<]*)</a></h2>`)
	contentPattern := regexp.MustCompile(`<p[^>]*>(.*?)</p>`)

	matches := resultPattern.FindAllStringSubmatch(html, -1)

	for i, match := range matches {
		if i >= 20 { // Limit to 20 results
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
