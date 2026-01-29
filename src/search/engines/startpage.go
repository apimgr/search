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

// Startpage implements Startpage Search engine
type Startpage struct {
	*search.BaseEngine
	client *http.Client
}

// NewStartpageEngine creates a new Startpage search engine
func NewStartpageEngine() *Startpage {
	config := model.NewEngineConfig("startpage")
	config.DisplayName = "Startpage"
	config.Priority = 70
	config.Categories = []string{"general", "images"}
	config.SupportsTor = true

	return &Startpage{
		BaseEngine: search.NewBaseEngine(config),
		client: &http.Client{
			Timeout: time.Duration(config.GetTimeout()) * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

// Search performs a Startpage search
func (e *Startpage) Search(ctx context.Context, query *model.Query) ([]model.Result, error) {
	searchURL := "https://www.startpage.com/sp/search"

	params := url.Values{}
	params.Set("query", query.Text)
	params.Set("cat", "web")
	params.Set("language", "english")
	params.Set("t", "default")
	params.Set("lui", "english")

	reqURL := fmt.Sprintf("%s?%s", searchURL, params.Encode())

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Referer", "https://www.startpage.com/")

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("startpage returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return e.parseResults(string(body), query)
}

func (e *Startpage) parseResults(html string, query *model.Query) ([]model.Result, error) {
	var results []model.Result
	maxResults := 10

	// Startpage uses specific class names for results
	// Look for result containers with w-gl__result class
	resultPattern := regexp.MustCompile(`<a[^>]+class="[^"]*w-gl__result-url[^"]*"[^>]*href="([^"]+)"[^>]*>`)
	titlePattern := regexp.MustCompile(`<h3[^>]*class="[^"]*w-gl__result-title[^"]*"[^>]*>([^<]+)</h3>`)
	descPattern := regexp.MustCompile(`<p[^>]*class="[^"]*w-gl__description[^"]*"[^>]*>([^<]+)</p>`)

	urlMatches := resultPattern.FindAllStringSubmatch(html, 20)
	titleMatches := titlePattern.FindAllStringSubmatch(html, 20)
	descMatches := descPattern.FindAllStringSubmatch(html, 20)

	// If standard patterns don't work, try generic extraction
	if len(urlMatches) == 0 {
		// Look for any external result-like links
		// Note: Go regexp doesn't support (?!) lookahead, so filter in code
		genericPattern := regexp.MustCompile(`<a[^>]+href="(https?://[^"]+)"[^>]*>([^<]+)</a>`)
		matches := genericPattern.FindAllStringSubmatch(html, 30)

		seen := make(map[string]bool)
		for _, match := range matches {
			if len(results) >= maxResults {
				break
			}
			if len(match) >= 3 {
				urlStr := strings.TrimSpace(match[1])
				title := strings.TrimSpace(match[2])

				// Skip navigation and internal links (replaces regex negative lookahead)
				if strings.Contains(urlStr, "startpage.com") ||
					strings.Contains(urlStr, "javascript:") ||
					strings.Contains(urlStr, "google.com/s2/favicons") ||
					title == "" || len(title) < 5 {
					continue
				}

				if seen[urlStr] {
					continue
				}
				seen[urlStr] = true

				results = append(results, model.Result{
					Title:    cleanHTML(title),
					URL:      urlStr,
					Content:  "",
					Engine:   "Startpage",
					Category: query.Category,
					Score:    float64(70) / float64(len(results)+1),
				})
			}
		}
		return results, nil
	}

	// Process matched results
	for i := 0; i < len(urlMatches) && i < maxResults; i++ {
		urlStr := ""
		title := ""
		desc := ""

		if len(urlMatches[i]) > 1 {
			urlStr = urlMatches[i][1]
		}
		if i < len(titleMatches) && len(titleMatches[i]) > 1 {
			title = titleMatches[i][1]
		}
		if i < len(descMatches) && len(descMatches[i]) > 1 {
			desc = descMatches[i][1]
		}

		if urlStr == "" {
			continue
		}

		results = append(results, model.Result{
			Title:    cleanHTML(title),
			URL:      urlStr,
			Content:  cleanHTML(desc),
			Engine:   "Startpage",
			Category: query.Category,
			Score:    float64(70) / float64(i+1),
		})
	}

	return results, nil
}
