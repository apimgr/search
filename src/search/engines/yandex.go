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

// Yandex implements Yandex search engine
type Yandex struct {
	*search.BaseEngine
	client *http.Client
}

// NewYandex creates a new Yandex search engine
func NewYandex() *Yandex {
	config := model.NewEngineConfig("yandex")
	config.DisplayName = "Yandex"
	config.Priority = 70
	config.Categories = []string{"general", "images", "news", "videos"}
	config.SupportsTor = false // Yandex blocks Tor

	return &Yandex{
		BaseEngine: search.NewBaseEngine(config),
		client: &http.Client{
			Timeout: time.Duration(config.GetTimeout()) * time.Second,
		},
	}
}

// Search performs a Yandex search
func (e *Yandex) Search(ctx context.Context, query *model.Query) ([]model.Result, error) {
	searchURL := "https://yandex.com/search/"

	params := url.Values{}
	params.Set("text", query.Text)

	// Handle categories
	switch query.Category {
	case model.CategoryImages:
		searchURL = "https://yandex.com/images/search"
	case model.CategoryNews:
		searchURL = "https://news.yandex.com/yandsearch"
	case model.CategoryVideos:
		searchURL = "https://yandex.com/video/search"
	}

	// Safe search (family filter) - 0: off, 1: moderate, 2: strict
	if query.SafeSearch == 2 {
		params.Set("family", "yes")
	} else if query.SafeSearch == 0 {
		params.Set("family", "no")
	}

	// Language/region
	if query.Language != "" {
		params.Set("lr", getYandexRegion(query.Language))
	}

	// Pagination
	if query.Page > 1 {
		params.Set("p", fmt.Sprintf("%d", query.Page-1))
	}

	reqURL := fmt.Sprintf("%s?%s", searchURL, params.Encode())

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.5")

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Yandex returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return e.parseResults(string(body), query.Category)
}

// parseResults parses HTML results from Yandex
func (e *Yandex) parseResults(html string, category model.Category) ([]model.Result, error) {
	results := make([]model.Result, 0)

	// Yandex result patterns (they use data attributes)
	resultPattern := regexp.MustCompile(`<li[^>]*class="[^"]*serp-item[^"]*"[^>]*>.*?</li>`)
	titlePattern := regexp.MustCompile(`<a[^>]*class="[^"]*OrganicTitle-Link[^"]*"[^>]*href="([^"]*)"[^>]*>.*?<span[^>]*>([^<]*)</span>`)
	descPattern := regexp.MustCompile(`<span[^>]*class="[^"]*OrganicTextContentSpan[^"]*"[^>]*>([^<]*)</span>`)

	// Alternative patterns for different page layouts
	altTitlePattern := regexp.MustCompile(`<a[^>]*href="([^"]*)"[^>]*class="[^"]*Link[^"]*"[^>]*>.*?<h2[^>]*>([^<]*)</h2>`)

	matches := resultPattern.FindAllString(html, -1)

	position := 0
	for _, match := range matches {
		var resultURL, title, content string

		// Try primary pattern
		titleMatch := titlePattern.FindStringSubmatch(match)
		if titleMatch != nil && len(titleMatch) >= 3 {
			resultURL = titleMatch[1]
			title = strings.TrimSpace(titleMatch[2])
		} else {
			// Try alternative pattern
			altMatch := altTitlePattern.FindStringSubmatch(match)
			if altMatch != nil && len(altMatch) >= 3 {
				resultURL = altMatch[1]
				title = strings.TrimSpace(altMatch[2])
			}
		}

		// Extract description
		descMatch := descPattern.FindStringSubmatch(match)
		if descMatch != nil && len(descMatch) >= 2 {
			content = strings.TrimSpace(descMatch[1])
		}

		if resultURL == "" || title == "" {
			continue
		}

		// Yandex uses redirects, extract actual URL if needed
		if strings.Contains(resultURL, "/clck/") {
			// Try to extract actual URL from redirect
			actualURL := extractYandexURL(resultURL)
			if actualURL != "" {
				resultURL = actualURL
			}
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

// getYandexRegion maps language codes to Yandex region codes
func getYandexRegion(lang string) string {
	regions := map[string]string{
		"en": "84",    // USA
		"ru": "225",   // Russia
		"uk": "187",   // UK
		"de": "96",    // Germany
		"fr": "124",   // France
		"es": "203",   // Spain
		"it": "205",   // Italy
		"tr": "983",   // Turkey
		"kz": "159",   // Kazakhstan
		"by": "149",   // Belarus
		"ua": "187",   // Ukraine
	}

	if region, ok := regions[lang]; ok {
		return region
	}
	return "84" // Default to USA
}

// extractYandexURL tries to extract the actual URL from Yandex redirect
func extractYandexURL(redirectURL string) string {
	parsed, err := url.Parse(redirectURL)
	if err != nil {
		return ""
	}

	// Try to get URL from query parameters
	if urlParam := parsed.Query().Get("url"); urlParam != "" {
		decoded, err := url.QueryUnescape(urlParam)
		if err == nil {
			return decoded
		}
	}

	return ""
}
