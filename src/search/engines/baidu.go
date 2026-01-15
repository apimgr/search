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

// Baidu implements Baidu search engine
type Baidu struct {
	*search.BaseEngine
	client *http.Client
}

// NewBaidu creates a new Baidu search engine
func NewBaidu() *Baidu {
	config := model.NewEngineConfig("baidu")
	config.DisplayName = "Baidu"
	config.Priority = 60
	config.Categories = []string{"general", "images", "news", "videos"}
	config.SupportsTor = false // Baidu blocks Tor

	return &Baidu{
		BaseEngine: search.NewBaseEngine(config),
		client: &http.Client{
			Timeout: time.Duration(config.GetTimeout()) * time.Second,
		},
	}
}

// Search performs a Baidu search
func (e *Baidu) Search(ctx context.Context, query *model.Query) ([]model.Result, error) {
	searchURL := "https://www.baidu.com/s"

	params := url.Values{}
	params.Set("wd", query.Text)
	params.Set("ie", "utf-8")

	// Handle categories
	switch query.Category {
	case model.CategoryImages:
		searchURL = "https://image.baidu.com/search/index"
		params.Set("word", query.Text)
		params.Del("wd")
	case model.CategoryNews:
		searchURL = "https://news.baidu.com/ns"
		params.Set("word", query.Text)
		params.Del("wd")
	case model.CategoryVideos:
		searchURL = "https://v.baidu.com/v"
		params.Set("word", query.Text)
		params.Del("wd")
	}

	// Pagination
	if query.Page > 1 {
		params.Set("pn", fmt.Sprintf("%d", (query.Page-1)*10))
	}

	reqURL := fmt.Sprintf("%s?%s", searchURL, params.Encode())

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Baidu returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return e.parseResults(string(body), query.Category)
}

// parseResults parses HTML results from Baidu
func (e *Baidu) parseResults(html string, category model.Category) ([]model.Result, error) {
	results := make([]model.Result, 0)

	// Baidu result patterns
	resultPattern := regexp.MustCompile(`<div[^>]*class="[^"]*result[^"]*c-container[^"]*"[^>]*>.*?</div>`)
	titlePattern := regexp.MustCompile(`<h3[^>]*class="[^"]*t[^"]*"[^>]*>.*?<a[^>]*href="([^"]*)"[^>]*>([^<]*(?:<em>[^<]*</em>[^<]*)*)</a>`)
	descPattern := regexp.MustCompile(`<div[^>]*class="[^"]*c-abstract[^"]*"[^>]*>([^<]*(?:<[^>]*>[^<]*</[^>]*>[^<]*)*)</div>`)

	// Alternative result pattern
	altResultPattern := regexp.MustCompile(`<div[^>]*class="[^"]*result[^"]*"[^>]*id="[^"]*"[^>]*>.*?</div>`)

	matches := resultPattern.FindAllString(html, -1)
	if len(matches) == 0 {
		matches = altResultPattern.FindAllString(html, -1)
	}

	position := 0
	for _, match := range matches {
		// Extract URL and title
		titleMatch := titlePattern.FindStringSubmatch(match)
		if titleMatch == nil || len(titleMatch) < 3 {
			continue
		}

		resultURL := titleMatch[1]
		title := strings.TrimSpace(titleMatch[2])

		// Remove HTML tags from title (Baidu uses <em> for highlighting)
		title = regexp.MustCompile(`<[^>]*>`).ReplaceAllString(title, "")

		// Extract description
		content := ""
		descMatch := descPattern.FindStringSubmatch(match)
		if descMatch != nil && len(descMatch) >= 2 {
			content = strings.TrimSpace(descMatch[1])
			content = regexp.MustCompile(`<[^>]*>`).ReplaceAllString(content, "")
		}

		if resultURL == "" || title == "" {
			continue
		}

		// Baidu uses redirects through baidu.com/link
		// The actual URL needs to be resolved, but for privacy we keep the redirect
		// Alternatively, we could follow the redirect

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
