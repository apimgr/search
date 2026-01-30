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

// WolframAlpha implements Wolfram Alpha search engine
// Uses the Short Answers API for computational queries
type WolframAlpha struct {
	*search.BaseEngine
	client *http.Client
}

// NewWolframAlpha creates a new Wolfram Alpha search engine
func NewWolframAlpha() *WolframAlpha {
	config := model.NewEngineConfig("wolfram")
	config.DisplayName = "Wolfram Alpha"
	config.Priority = 75 // High priority for computational queries
	config.Categories = []string{"general", "science"}
	config.SupportsTor = true

	return &WolframAlpha{
		BaseEngine: search.NewBaseEngine(config),
		client: &http.Client{
			Timeout: time.Duration(config.GetTimeout()) * time.Second,
		},
	}
}

// Search performs a Wolfram Alpha search
// Returns instant answer results for computational queries
func (e *WolframAlpha) Search(ctx context.Context, query *model.Query) ([]model.Result, error) {
	// Try web scraping approach for Wolfram Alpha results
	// This provides richer results than the Short Answers API which requires an API key
	return e.searchWeb(ctx, query)
}

// searchWeb scrapes Wolfram Alpha web results
func (e *WolframAlpha) searchWeb(ctx context.Context, query *model.Query) ([]model.Result, error) {
	// Wolfram Alpha web search URL
	baseURL := "https://www.wolframalpha.com/input"

	params := url.Values{}
	params.Set("i", query.Text)

	reqURL := fmt.Sprintf("%s?%s", baseURL, params.Encode())

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Wolfram Alpha returned status %d", resp.StatusCode)
	}

	return e.parseWolframHTML(resp, query)
}

// parseWolframHTML parses Wolfram Alpha web results
func (e *WolframAlpha) parseWolframHTML(resp *http.Response, query *model.Query) ([]model.Result, error) {
	results := make([]model.Result, 0)

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024)) // 2MB max
	if err != nil {
		return nil, err
	}
	html := string(body)

	// Wolfram Alpha embeds results in various formats
	// Look for plaintext results and pod titles

	// Pattern to extract pod titles (section headers)
	podTitlePattern := regexp.MustCompile(`<h2[^>]*class="[^"]*"[^>]*>([^<]+)</h2>`)
	// Pattern to extract result text/values
	resultPattern := regexp.MustCompile(`<img[^>]*alt="([^"]+)"[^>]*class="[^"]*_image[^"]*"`)
	// Pattern for plaintext results embedded in data attributes
	plaintextPattern := regexp.MustCompile(`data-stringified="([^"]+)"`)

	podTitles := podTitlePattern.FindAllStringSubmatch(html, -1)
	resultTexts := resultPattern.FindAllStringSubmatch(html, -1)
	plaintexts := plaintextPattern.FindAllStringSubmatch(html, -1)

	// Build result URL
	resultURL := fmt.Sprintf("https://www.wolframalpha.com/input?i=%s", url.QueryEscape(query.Text))

	// Combine extracted information
	contentParts := make([]string, 0)

	// Add plaintext results (most accurate)
	for _, pt := range plaintexts {
		if len(pt) > 1 {
			text := cleanWolframText(pt[1])
			if text != "" && !isDuplicateContent(contentParts, text) {
				contentParts = append(contentParts, text)
			}
		}
	}

	// Add result texts from image alt attributes
	for _, rt := range resultTexts {
		if len(rt) > 1 {
			text := cleanWolframText(rt[1])
			if text != "" && !isDuplicateContent(contentParts, text) {
				contentParts = append(contentParts, text)
			}
		}
	}

	// If we found any content, create a result
	if len(contentParts) > 0 {
		// Build title from pod titles or use query
		title := fmt.Sprintf("Wolfram Alpha: %s", query.Text)
		if len(podTitles) > 0 && len(podTitles[0]) > 1 {
			firstPod := cleanHTML(podTitles[0][1])
			if firstPod != "" && firstPod != "Input" && firstPod != "Input interpretation" {
				title = firstPod
			}
		}

		// Combine content
		content := strings.Join(contentParts, " | ")
		if len(content) > 500 {
			content = content[:497] + "..."
		}

		results = append(results, model.Result{
			Title:    title,
			URL:      resultURL,
			Content:  content,
			Engine:   e.Name(),
			Category: model.CategoryGeneral,
			Score:    calculateScore(e.GetPriority(), 0, 1),
			Position: 0,
		})
	}

	// If no structured content was found, try simpler extraction
	if len(results) == 0 {
		// Look for any meaningful text in result containers
		simplePattern := regexp.MustCompile(`<div[^>]*class="[^"]*pod[^"]*"[^>]*>([^<]+)</div>`)
		simpleMatches := simplePattern.FindAllStringSubmatch(html, 10)

		content := ""
		for _, match := range simpleMatches {
			if len(match) > 1 {
				text := strings.TrimSpace(match[1])
				if len(text) > 10 && !strings.Contains(text, "{") {
					if content != "" {
						content += " | "
					}
					content += text
				}
			}
		}

		if content != "" {
			results = append(results, model.Result{
				Title:    fmt.Sprintf("Wolfram Alpha: %s", query.Text),
				URL:      resultURL,
				Content:  content,
				Engine:   e.Name(),
				Category: model.CategoryGeneral,
				Score:    calculateScore(e.GetPriority(), 0, 1),
				Position: 0,
			})
		}
	}

	// If still no results, return a placeholder that links to Wolfram Alpha
	if len(results) == 0 {
		results = append(results, model.Result{
			Title:    fmt.Sprintf("Compute: %s", query.Text),
			URL:      resultURL,
			Content:  "View computational answer on Wolfram Alpha",
			Engine:   e.Name(),
			Category: model.CategoryGeneral,
			Score:    calculateScore(e.GetPriority(), 0, 1),
			Position: 0,
		})
	}

	return results, nil
}

// cleanWolframText cleans up Wolfram Alpha text results
func cleanWolframText(text string) string {
	// Decode HTML entities
	text = strings.ReplaceAll(text, "&amp;", "&")
	text = strings.ReplaceAll(text, "&lt;", "<")
	text = strings.ReplaceAll(text, "&gt;", ">")
	text = strings.ReplaceAll(text, "&quot;", `"`)
	text = strings.ReplaceAll(text, "&#39;", "'")
	text = strings.ReplaceAll(text, "&nbsp;", " ")
	text = strings.ReplaceAll(text, "\\n", " ")
	text = strings.ReplaceAll(text, "\\t", " ")

	// Remove extra whitespace
	spacePattern := regexp.MustCompile(`\s+`)
	text = spacePattern.ReplaceAllString(text, " ")

	return strings.TrimSpace(text)
}

// isDuplicateContent checks if content is already in the list
func isDuplicateContent(existing []string, newContent string) bool {
	newLower := strings.ToLower(newContent)
	for _, e := range existing {
		if strings.ToLower(e) == newLower {
			return true
		}
		// Also check for substring matches
		if strings.Contains(strings.ToLower(e), newLower) || strings.Contains(newLower, strings.ToLower(e)) {
			return true
		}
	}
	return false
}
