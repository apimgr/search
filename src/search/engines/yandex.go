package engines

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/apimgr/search/src/model"
	"github.com/apimgr/search/src/search"
)

// Pre-compiled regexes for Yandex HTML parsing.
//
// Strategy: locate title anchors by the stable class fragment "organic__title-link"
// (present since ~2018), then search ahead in the HTML for the nearest snippet.
// Fall back to any <h2> that wraps an external https:// link.
var (
	// yandexAnchorRe captures the full opening <a> tag and inner content for
	// title anchors. Group 1 = opening tag (used to extract href).
	// Group 2 = inner HTML (stripped to get title text).
	yandexAnchorRe = regexp.MustCompile(`(?s)(<a\b[^>]*\bclass="[^"]*organic__title-link[^"]*"[^>]*>)([\s\S]{0,300}?)</a>`)

	// yandexH2Re is a structural fallback: any <h2> containing a direct https:// link.
	yandexH2Re = regexp.MustCompile(`(?s)<h2[^>]*>([\s\S]{0,300}?href="(https?://[^"]{5,400})"[\s\S]{0,300}?)</h2>`)

	// yandexHrefRe extracts href from an opening <a> tag.
	yandexHrefRe = regexp.MustCompile(`\bhref="(https?://[^"]{5,500})"`)

	// yandexSnipRe matches the snippet <div> using several stable class fragments.
	yandexSnipRe = regexp.MustCompile(`(?s)<div[^>]+class="[^"]*(?:OrganicTextContentSpan|organic__text|TextContainer)[^"]*"[^>]*>([\s\S]{0,700}?)</div>`)

	// yandexTagRe strips HTML tags for plain-text extraction.
	yandexTagRe = regexp.MustCompile(`<[^>]+>`)
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
	config.SupportsTor = false // Yandex blocks Tor exit nodes

	return &Yandex{
		BaseEngine: search.NewBaseEngine(config),
		client: &http.Client{
			Timeout:   time.Duration(config.GetTimeout()) * time.Second,
			Transport: SharedTransport,
		},
	}
}

// Search performs a Yandex search
func (e *Yandex) Search(ctx context.Context, query *model.Query) ([]model.Result, error) {
	searchURL := "https://yandex.com/search/"
	params := url.Values{}
	params.Set("text", query.Text)

	switch query.Category {
	case model.CategoryImages:
		searchURL = "https://yandex.com/images/search"
	case model.CategoryNews:
		searchURL = "https://yandex.com/news/search"
	case model.CategoryVideos:
		searchURL = "https://yandex.com/video/search"
	}

	if query.SafeSearch == 2 {
		params.Set("family", "yes")
	} else if query.SafeSearch == 0 {
		params.Set("family", "no")
	}
	if query.Language != "" {
		params.Set("lr", getYandexRegion(query.Language))
	}
	if query.Page > 1 {
		params.Set("p", fmt.Sprintf("%d", query.Page-1))
	}

	req, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("%s?%s", searchURL, params.Encode()), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("DNT", "1")

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("yandex returned status %d", resp.StatusCode)
	}

	body, err := ReadBody(resp)
	if err != nil {
		return nil, err
	}

	return e.parseResults(string(body), query.Category)
}

// parseResults parses HTML results from Yandex.
func (e *Yandex) parseResults(html string, category model.Category) ([]model.Result, error) {
	// Collect (url, title, matchEnd) for all title anchors.
	type hit struct {
		rawURL   string
		title    string
		matchEnd int
	}

	var hits []hit

	// Primary: class="organic__title-link" anchors.
	for _, m := range yandexAnchorRe.FindAllStringSubmatchIndex(html, -1) {
		openTag := html[m[2]:m[3]]
		inner := html[m[4]:m[5]]

		hrefM := yandexHrefRe.FindStringSubmatch(openTag)
		if hrefM == nil {
			continue
		}
		rawURL := hrefM[1]
		// Skip Yandex-internal URLs.
		if strings.Contains(rawURL, "yandex.") {
			continue
		}
		title := strings.TrimSpace(unescapeHTML(yandexTagRe.ReplaceAllString(inner, " ")))
		if title == "" {
			continue
		}
		hits = append(hits, hit{rawURL: rawURL, title: title, matchEnd: m[1]})
	}

	// Fallback: structural <h2> + external https link.
	if len(hits) == 0 {
		for _, m := range yandexH2Re.FindAllStringSubmatchIndex(html, -1) {
			block := html[m[2]:m[3]]
			rawURL := html[m[4]:m[5]]
			if strings.Contains(rawURL, "yandex.") {
				continue
			}
			title := strings.TrimSpace(unescapeHTML(yandexTagRe.ReplaceAllString(block, " ")))
			if title == "" {
				continue
			}
			hits = append(hits, hit{rawURL: rawURL, title: title, matchEnd: m[1]})
		}
	}

	maxResults := e.GetConfig().GetMaxResults()
	results := make([]model.Result, 0, len(hits))

	for i, h := range hits {
		// Search for snippet in HTML between this result and the next.
		snippetEnd := len(html)
		if i+1 < len(hits) {
			snippetEnd = hits[i+1].matchEnd
		}
		if snippetEnd > h.matchEnd+3000 {
			snippetEnd = h.matchEnd + 3000
		}
		block := html[h.matchEnd:snippetEnd]

		content := ""
		if sm := yandexSnipRe.FindStringSubmatch(block); sm != nil {
			content = strings.TrimSpace(unescapeHTML(yandexTagRe.ReplaceAllString(sm[1], " ")))
		}

		results = append(results, model.Result{
			Title:    h.title,
			URL:      h.rawURL,
			Content:  content,
			Engine:   e.Name(),
			Category: category,
			Score:    calculateScore(e.GetPriority(), i, 1),
			Position: i,
		})

		if len(results) >= maxResults {
			break
		}
	}

	return results, nil
}

// getYandexRegion maps language codes to Yandex region codes.
func getYandexRegion(lang string) string {
	regions := map[string]string{
		"en": "84",  // USA
		"ru": "225", // Russia
		"uk": "187", // UK
		"de": "96",  // Germany
		"fr": "124", // France
		"es": "203", // Spain
		"it": "205", // Italy
		"tr": "983", // Turkey
		"kz": "159", // Kazakhstan
		"by": "149", // Belarus
		"ua": "187", // Ukraine
	}
	if region, ok := regions[lang]; ok {
		return region
	}
	return "84" // default: USA
}

// extractYandexURL extracts the actual URL from a Yandex /clck/ redirect.
// Kept for backwards compatibility with existing tests.
func extractYandexURL(redirectURL string) string {
	parsed, err := url.Parse(redirectURL)
	if err != nil {
		return ""
	}
	if u := parsed.Query().Get("url"); u != "" {
		decoded, err := url.QueryUnescape(u)
		if err == nil {
			return decoded
		}
	}
	return ""
}
