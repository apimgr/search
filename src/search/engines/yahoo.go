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

	"github.com/apimgr/search/src/models"
	"github.com/apimgr/search/src/search"
)

// Yahoo implements Yahoo search engine
type Yahoo struct {
	*search.BaseEngine
	client *http.Client
}

// NewYahoo creates a new Yahoo search engine
func NewYahoo() *Yahoo {
	config := models.NewEngineConfig("yahoo")
	config.DisplayName = "Yahoo"
	config.Priority = 65
	config.Categories = []string{"general", "images", "news"}
	config.SupportsTor = false

	return &Yahoo{
		BaseEngine: search.NewBaseEngine(config),
		client: &http.Client{
			Timeout: time.Duration(config.GetTimeout()) * time.Second,
		},
	}
}

// Search performs a Yahoo search
func (e *Yahoo) Search(ctx context.Context, query *models.Query) ([]models.Result, error) {
	searchURL := "https://search.yahoo.com/search"

	params := url.Values{}
	params.Set("p", query.Text)
	params.Set("ei", "UTF-8")

	if query.Category == models.CategoryImages {
		searchURL = "https://images.search.yahoo.com/search/images"
	} else if query.Category == models.CategoryNews {
		searchURL = "https://news.search.yahoo.com/search"
	}

	reqURL := fmt.Sprintf("%s?%s", searchURL, params.Encode())

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36 Edg/131.0.0.0")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.5")

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Yahoo returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return e.parseResults(string(body), query.Category)
}

// parseResults parses HTML results from Yahoo
func (e *Yahoo) parseResults(html string, category models.Category) ([]models.Result, error) {
	results := make([]models.Result, 0)

	// Pattern for Yahoo search results
	// Yahoo results are in <li> elements with specific classes
	resultPattern := regexp.MustCompile(`<div[^>]*class="[^"]*algo[^"]*"[^>]*>.*?</div>`)
	linkPattern := regexp.MustCompile(`<a[^>]*class="[^"]*ac-algo[^"]*"[^>]*href="([^"]*)"[^>]*>([^<]*)</a>`)
	descPattern := regexp.MustCompile(`<p[^>]*class="[^"]*s-desc[^"]*"[^>]*>([^<]*)</p>`)

	// Alternative simpler pattern
	simpleLinkPattern := regexp.MustCompile(`<a[^>]*href="(https?://[^"]*)"[^>]*><h3[^>]*>([^<]*)</h3></a>`)

	position := 0

	// Try simple pattern first
	matches := simpleLinkPattern.FindAllStringSubmatch(html, -1)
	for _, match := range matches {
		if len(match) < 3 {
			continue
		}

		resultURL := match[1]
		title := strings.TrimSpace(match[2])

		// Skip Yahoo internal links
		if strings.Contains(resultURL, "yahoo.com") && !strings.Contains(resultURL, "r.search.yahoo.com") {
			continue
		}

		// Extract real URL from Yahoo redirect
		if strings.Contains(resultURL, "r.search.yahoo.com") {
			if realURL := extractYahooRedirectURL(resultURL); realURL != "" {
				resultURL = realURL
			}
		}

		// Skip empty results
		if resultURL == "" || title == "" {
			continue
		}

		title = unescapeHTML(title)

		results = append(results, models.Result{
			Title:    title,
			URL:      resultURL,
			Content:  "",
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

	// If no results from simple pattern, try complex pattern
	if len(results) == 0 {
		resultMatches := resultPattern.FindAllString(html, -1)
		for _, match := range resultMatches {
			linkMatch := linkPattern.FindStringSubmatch(match)
			if linkMatch == nil || len(linkMatch) < 3 {
				continue
			}

			resultURL := linkMatch[1]
			title := strings.TrimSpace(linkMatch[2])

			content := ""
			descMatch := descPattern.FindStringSubmatch(match)
			if descMatch != nil && len(descMatch) >= 2 {
				content = strings.TrimSpace(descMatch[1])
			}

			// Extract real URL from Yahoo redirect
			if strings.Contains(resultURL, "r.search.yahoo.com") {
				if realURL := extractYahooRedirectURL(resultURL); realURL != "" {
					resultURL = realURL
				}
			}

			if resultURL == "" || title == "" {
				continue
			}

			title = unescapeHTML(title)
			content = unescapeHTML(content)

			results = append(results, models.Result{
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
	}

	return results, nil
}

// extractYahooRedirectURL extracts the real URL from Yahoo's redirect URL
func extractYahooRedirectURL(redirectURL string) string {
	// Yahoo redirect URLs contain the real URL as a parameter
	parsed, err := url.Parse(redirectURL)
	if err != nil {
		return ""
	}

	// Try to get RU parameter (Real URL)
	if ru := parsed.Query().Get("RU"); ru != "" {
		decoded, err := url.QueryUnescape(ru)
		if err == nil {
			return decoded
		}
	}

	return ""
}
