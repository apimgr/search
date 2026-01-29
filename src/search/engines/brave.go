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

// Brave implements Brave Search engine
type Brave struct {
	*search.BaseEngine
	client *http.Client
}

// NewBrave creates a new Brave search engine
func NewBrave() *Brave {
	config := model.NewEngineConfig("brave")
	config.DisplayName = "Brave Search"
	config.Priority = 75
	config.Categories = []string{"general", "images", "news"}
	config.SupportsTor = true

	return &Brave{
		BaseEngine: search.NewBaseEngine(config),
		client: &http.Client{
			Timeout: time.Duration(config.GetTimeout()) * time.Second,
		},
	}
}

// Search performs a Brave search
func (e *Brave) Search(ctx context.Context, query *model.Query) ([]model.Result, error) {
	searchURL := "https://search.brave.com/search"

	params := url.Values{}
	params.Set("q", query.Text)
	params.Set("source", "web")

	if query.Category == model.CategoryImages {
		searchURL = "https://search.brave.com/images"
	} else if query.Category == model.CategoryNews {
		searchURL = "https://search.brave.com/news"
	}

	reqURL := fmt.Sprintf("%s?%s", searchURL, params.Encode())

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
		return nil, fmt.Errorf("Brave returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return e.parseResults(string(body), query.Category)
}

// parseResults parses HTML results from Brave
func (e *Brave) parseResults(html string, category model.Category) ([]model.Result, error) {
	results := make([]model.Result, 0)

	// Pattern for result blocks
	resultPattern := regexp.MustCompile(`<div[^>]*class="[^"]*snippet[^"]*"[^>]*>.*?</div>`)
	titlePattern := regexp.MustCompile(`<a[^>]*class="[^"]*result-header[^"]*"[^>]*href="([^"]*)"[^>]*>.*?<span[^>]*class="[^"]*snippet-title[^"]*"[^>]*>([^<]*)</span>`)
	descPattern := regexp.MustCompile(`<p[^>]*class="[^"]*snippet-description[^"]*"[^>]*>([^<]*)</p>`)

	// Find all result blocks
	matches := resultPattern.FindAllString(html, -1)

	position := 0
	for _, match := range matches {
		// Extract URL and title
		titleMatch := titlePattern.FindStringSubmatch(match)
		if titleMatch == nil || len(titleMatch) < 3 {
			continue
		}

		resultURL := titleMatch[1]
		title := strings.TrimSpace(titleMatch[2])

		// Extract description
		content := ""
		descMatch := descPattern.FindStringSubmatch(match)
		if descMatch != nil && len(descMatch) >= 2 {
			content = strings.TrimSpace(descMatch[1])
		}

		// Skip empty results
		if resultURL == "" || title == "" {
			continue
		}

		// Unescape HTML entities
		title = unescapeHTML(title)
		content = unescapeHTML(content)

		results = append(results, model.Result{
			Title:    title,
			URL:      resultURL,
			Content:  content,
			Engine:   e.Name(),
			Category: category,
			Score:    calculateScore(e.GetPriority(), position, 1),
			Position: position,
		})

		position++
		if position >= e.GetConfig().GetMaxResults() {
			break
		}
	}

	return results, nil
}

// unescapeHTML unescapes common HTML entities
func unescapeHTML(s string) string {
	s = strings.ReplaceAll(s, "&amp;", "&")
	s = strings.ReplaceAll(s, "&lt;", "<")
	s = strings.ReplaceAll(s, "&gt;", ">")
	s = strings.ReplaceAll(s, "&quot;", "\"")
	s = strings.ReplaceAll(s, "&#39;", "'")
	s = strings.ReplaceAll(s, "&nbsp;", " ")
	return s
}
