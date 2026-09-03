package engine

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
	config.Categories = []string{"general", "images", "news", "files", "music"}
	config.SupportsTor = true

	return &Brave{
		BaseEngine: search.NewBaseEngine(config),
		client: &http.Client{
			Timeout:   time.Duration(config.GetTimeout()) * time.Second,
			Transport: SharedTransport,
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

// parseResults parses HTML results from Brave.
//
// Brave's markup is a Svelte-rendered SPA shell with no stable top-level
// result container; the only reliable per-result anchor is the numeric
// `data-pos` attribute on each result's wrapper (e.g.
// `class="snippet svelte-jmfu5f" data-pos="1" data-type="web"`), so results
// are isolated by splitting on that marker rather than a result-block class.
func (e *Brave) parseResults(html string, category model.Category) ([]model.Result, error) {
	results := make([]model.Result, 0)

	blockPattern := regexp.MustCompile(`class="snippet[^"]*"\s+data-pos="\d+"\s+data-type="web"`)
	titlePattern := regexp.MustCompile(`(?s)<a[^>]*href="([^"]*)"[^>]*>.*?class="[^"]*search-snippet-title[^"]*"[^>]*title="([^"]*)"`)
	descPattern := regexp.MustCompile(`(?s)class="content[^"]*"[^>]*>(.*?)</div>`)
	tagPattern := regexp.MustCompile(`<[^>]*>`)

	blockStarts := blockPattern.FindAllStringIndex(html, -1)

	position := 0
	for i, start := range blockStarts {
		end := len(html)
		if i+1 < len(blockStarts) {
			end = blockStarts[i+1][0]
		}
		block := html[start[1]:end]

		// Extract URL and title
		titleMatch := titlePattern.FindStringSubmatch(block)
		if len(titleMatch) < 3 {
			continue
		}

		resultURL := titleMatch[1]
		title := strings.TrimSpace(titleMatch[2])

		// Extract description; strip inline tags (e.g. <strong>) from the content
		content := ""
		if descMatch := descPattern.FindStringSubmatch(block); len(descMatch) >= 2 {
			content = strings.TrimSpace(tagPattern.ReplaceAllString(descMatch[1], ""))
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
