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

// Mojeek implements Mojeek search engine
// Mojeek is a privacy-focused search engine with its own crawler
type Mojeek struct {
	*search.BaseEngine
	client *http.Client
}

// NewMojeek creates a new Mojeek search engine
func NewMojeek() *Mojeek {
	config := model.NewEngineConfig("mojeek")
	config.DisplayName = "Mojeek"
	config.Priority = 65
	config.Categories = []string{"general", "images", "news"}
	config.SupportsTor = true

	return &Mojeek{
		BaseEngine: search.NewBaseEngine(config),
		client: &http.Client{
			Timeout: time.Duration(config.GetTimeout()) * time.Second,
		},
	}
}

// Search performs a Mojeek search
func (e *Mojeek) Search(ctx context.Context, query *model.Query) ([]model.Result, error) {
	searchURL := "https://www.mojeek.com/search"

	params := url.Values{}
	params.Set("q", query.Text)

	// Handle categories
	switch query.Category {
	case model.CategoryImages:
		params.Set("fmt", "images")
	case model.CategoryNews:
		params.Set("fmt", "news")
	}

	// Safe search (0: off, 1: moderate, 2: strict)
	if query.SafeSearch == 2 {
		params.Set("safe", "1")
	} else if query.SafeSearch == 0 {
		params.Set("safe", "0")
	}

	// Language
	if query.Language != "" {
		params.Set("lb", query.Language)
	}

	// Pagination
	if query.Page > 1 {
		params.Set("s", fmt.Sprintf("%d", (query.Page-1)*10+1))
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
		return nil, fmt.Errorf("Mojeek returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return e.parseResults(string(body), query.Category)
}

// parseResults parses HTML results from Mojeek
func (e *Mojeek) parseResults(html string, category model.Category) ([]model.Result, error) {
	results := make([]model.Result, 0)

	// Mojeek result pattern
	resultPattern := regexp.MustCompile(`<li[^>]*class="[^"]*results-standard[^"]*"[^>]*>.*?</li>`)
	titlePattern := regexp.MustCompile(`<a[^>]*class="[^"]*title[^"]*"[^>]*href="([^"]*)"[^>]*>([^<]*)</a>`)
	descPattern := regexp.MustCompile(`<p[^>]*class="[^"]*s[^"]*"[^>]*>([^<]*)</p>`)
	urlPattern := regexp.MustCompile(`<p[^>]*class="[^"]*u[^"]*"[^>]*>([^<]*)</p>`)

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

		// Use display URL if main URL not found
		if resultURL == "" {
			urlMatch := urlPattern.FindStringSubmatch(match)
			if urlMatch != nil && len(urlMatch) >= 2 {
				resultURL = urlMatch[1]
				if !strings.HasPrefix(resultURL, "http") {
					resultURL = "https://" + resultURL
				}
			}
		}

		if resultURL == "" || title == "" {
			continue
		}

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
