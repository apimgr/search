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

// Yahoo implements Yahoo search engine
type Yahoo struct {
	*search.BaseEngine
	client *http.Client
}

// NewYahoo creates a new Yahoo search engine
func NewYahoo() *Yahoo {
	config := model.NewEngineConfig("yahoo")
	config.DisplayName = "Yahoo"
	config.Priority = 65
	config.Categories = []string{"general", "images", "news", "files", "music"}
	config.SupportsTor = false

	return &Yahoo{
		BaseEngine: search.NewBaseEngine(config),
		client: &http.Client{
			Timeout:   time.Duration(config.GetTimeout()) * time.Second,
			Transport: SharedTransport,
		},
	}
}

// Search performs a Yahoo search
func (e *Yahoo) Search(ctx context.Context, query *model.Query) ([]model.Result, error) {
	searchURL := "https://search.yahoo.com/search"

	params := url.Values{}
	params.Set("p", query.Text)
	params.Set("ei", "UTF-8")

	if query.Category == model.CategoryImages {
		searchURL = "https://images.search.yahoo.com/search/images"
	} else if query.Category == model.CategoryNews {
		searchURL = "https://news.search.yahoo.com/search"
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
		return nil, fmt.Errorf("Yahoo returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return e.parseResults(string(body), query.Category)
}

// yahooTagRe strips nested HTML tags (e.g. <span>) from captured title/desc text.
var yahooTagRe = regexp.MustCompile(`<[^>]+>`)

// parseResults parses HTML results from Yahoo
func (e *Yahoo) parseResults(html string, category model.Category) ([]model.Result, error) {
	results := make([]model.Result, 0)

	// Pattern for Yahoo search results
	// Yahoo results are in <li> elements with specific classes
	resultPattern := regexp.MustCompile(`(?s)<div[^>]*class="[^"]*algo[^"]*"[^>]*>.*?</div>`)
	// The title anchor's own class attribute is a volatile utility-class list
	// (no stable "ac-algo" marker in current markup), so match on the
	// href+h3 structure instead, bounding the gap to avoid runaway matches.
	linkPattern := regexp.MustCompile(`<a[^>]*href="([^"]*)"[^>]*>[\s\S]{0,1000}?<h3[^>]*>([\s\S]{0,300}?)</h3>`)
	descPattern := regexp.MustCompile(`<p[^>]*>([\s\S]{0,500}?)</p>`)

	// Alternative simpler pattern: the anchor wraps an <h3> whose title text
	// is itself wrapped in a nested <span>, so the gap between the h3 open
	// tag and the title text must tolerate nested tags (bounded, not [^<]*).
	simpleLinkPattern := regexp.MustCompile(`<a[^>]*href="(https?://[^"]*)"[^>]*>[\s\S]{0,1000}?<h3[^>]*>([\s\S]{0,300}?)</h3>`)

	// Flat-anchor fallback pattern: some Yahoo markup has no <h3> at all —
	// the title text sits directly inside the "ac-algo" anchor.
	flatLinkPattern := regexp.MustCompile(`<a[^>]*class="[^"]*ac-algo[^"]*"[^>]*href="([^"]*)"[^>]*>([\s\S]{0,300}?)</a>`)

	position := 0

	// Try simple pattern first
	matches := simpleLinkPattern.FindAllStringSubmatch(html, -1)
	for _, match := range matches {
		if len(match) < 3 {
			continue
		}

		resultURL := match[1]
		title := strings.TrimSpace(yahooTagRe.ReplaceAllString(match[2], ""))

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

		results = append(results, model.Result{
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
			if len(linkMatch) < 3 {
				// No <h3> title wrapper present — fall back to the flat
				// "ac-algo" anchor whose text is the title directly.
				linkMatch = flatLinkPattern.FindStringSubmatch(match)
			}
			if len(linkMatch) < 3 {
				continue
			}

			resultURL := linkMatch[1]
			title := strings.TrimSpace(yahooTagRe.ReplaceAllString(linkMatch[2], ""))

			content := ""
			descMatch := descPattern.FindStringSubmatch(match)
			if len(descMatch) >= 2 {
				content = strings.TrimSpace(yahooTagRe.ReplaceAllString(descMatch[1], ""))
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
	}

	return results, nil
}

// yahooRedirectPathRURe extracts a path-segment-style RU= (Real URL) from a
// Yahoo redirect URL (".../RU=https%3a%2f%2fexample.com%2f/RK=..."). The
// query-string style ("...?RU=...") is handled separately via url.Parse so
// that unencoded "/" characters inside the captured value are preserved.
var yahooRedirectPathRURe = regexp.MustCompile(`[/;]RU=([^/;]+)`)

// extractYahooRedirectURL extracts the real URL from Yahoo's redirect URL
func extractYahooRedirectURL(redirectURL string) string {
	parsed, err := url.Parse(redirectURL)
	if err == nil {
		if ru := parsed.Query().Get("RU"); ru != "" {
			if decoded, decErr := url.QueryUnescape(ru); decErr == nil {
				return decoded
			}
		}
	}

	match := yahooRedirectPathRURe.FindStringSubmatch(redirectURL)
	if len(match) < 2 {
		return ""
	}

	decoded, decErr := url.QueryUnescape(match[1])
	if decErr != nil {
		return ""
	}

	return decoded
}
