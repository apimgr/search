package widget

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/apimgr/search/src/common/closeutil"
	"golang.org/x/net/html"
)

// WikipediaFetcher fetches Wikipedia summaries
type WikipediaFetcher struct {
	httpClient *http.Client
}

// WikipediaData represents Wikipedia summary result
type WikipediaData struct {
	Title           string           `json:"title"`
	Extract         string           `json:"extract"`
	Description     string           `json:"description,omitempty"`
	Thumbnail       string           `json:"thumbnail,omitempty"`
	URL             string           `json:"url"`
	Language        string           `json:"language"`
	RelatedArticles []RelatedArticle `json:"related_articles,omitempty"`
}

// WikipediaSection is a bounded, text-only section of a full article.
type WikipediaSection struct {
	Anchor string `json:"anchor"`
	Title  string `json:"title"`
	Level  int    `json:"level"`
	Text   string `json:"text"`
}

// WikipediaField is one label/value pair extracted from an article infobox.
type WikipediaField struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// WikipediaArticleDetails contains the text-only additions used by full-page wiki answers.
type WikipediaArticleDetails struct {
	Sections        []WikipediaSection `json:"sections,omitempty"`
	Infobox         []WikipediaField   `json:"infobox,omitempty"`
	RelatedArticles []RelatedArticle   `json:"related_articles,omitempty"`
}

// RelatedArticle represents a related Wikipedia article
type RelatedArticle struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Extract string `json:"extract,omitempty"`
}

// NewWikipediaFetcher creates a new Wikipedia fetcher
func NewWikipediaFetcher() *WikipediaFetcher {
	return &WikipediaFetcher{
		httpClient: &http.Client{
			Timeout:       10 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}
}

// Fetch fetches Wikipedia summary
func (f *WikipediaFetcher) Fetch(ctx context.Context, params map[string]string) (*WidgetData, error) {
	// Support both "topic" and "query" params
	topic := params["topic"]
	if topic == "" {
		topic = params["query"]
	}
	if topic == "" {
		return &WidgetData{
			Type:      WidgetWikipedia,
			Error:     "topic parameter required",
			UpdatedAt: time.Now(),
		}, nil
	}

	// Parse natural language queries
	topic = f.parseQuery(topic)

	lang := params["lang"]
	if lang == "" {
		lang = "en"
	}
	if !wikipediaLanguagePattern.MatchString(lang) {
		lang = "en"
	}

	// First, try direct article lookup
	data, err := f.fetchArticleSummary(ctx, topic, lang)
	if err != nil {
		return nil, err
	}

	// If direct lookup failed, try search
	if data == nil {
		data, err = f.searchAndFetch(ctx, topic, lang)
		if err != nil {
			return nil, err
		}
	}
	if data == nil {
		return &WidgetData{
			Type:      WidgetWikipedia,
			Error:     "article not found",
			UpdatedAt: time.Now(),
		}, nil
	}
	// Fetch related articles
	related, _ := f.fetchRelatedArticles(ctx, data.Title, lang)
	if len(related) > 0 {
		data.RelatedArticles = related
	}

	return &WidgetData{
		Type:      WidgetWikipedia,
		Data:      data,
		UpdatedAt: time.Now(),
	}, nil
}

var wikipediaLanguagePattern = regexp.MustCompile(`^[a-z]{2,3}$`)

const (
	wikipediaDetailsLimit = 5 << 20
	wikipediaMaxSections  = 8
	wikipediaMaxFields    = 14
)

type wikipediaParseResponse struct {
	Parse struct {
		Sections []struct {
			Anchor string `json:"anchor"`
			Line   string `json:"line"`
			Level  string `json:"level"`
		} `json:"sections"`
		Text json.RawMessage `json:"text"`
	} `json:"parse"`
}

// fetchArticleDetails retrieves bounded parsed article HTML, then keeps only
// plain text from headings, sections, and infobox rows. Remote markup is never
// passed through to the browser.
func (f *WikipediaFetcher) fetchArticleDetails(ctx context.Context, title, lang string) ([]WikipediaSection, []WikipediaField, error) {
	params := url.Values{
		"action": {"parse"}, "page": {title}, "prop": {"sections|text"},
		"format": {"json"}, "formatversion": {"2"}, "disableeditsection": {"1"},
	}
	endpoint := fmt.Sprintf("https://%s.wikipedia.org/w/api.php?%s", lang, params.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("User-Agent", "Search/1.0 (privacy-focused search engine)")
	resp, err := f.httpClient.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer closeutil.Close(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("wikipedia parse API error: %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, wikipediaDetailsLimit+1))
	if err != nil {
		return nil, nil, err
	}
	if len(body) > wikipediaDetailsLimit {
		return nil, nil, fmt.Errorf("wikipedia article details exceed size limit")
	}
	var result wikipediaParseResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, nil, err
	}
	markup, err := decodeWikipediaHTML(result.Parse.Text)
	if err != nil || markup == "" {
		return nil, nil, fmt.Errorf("wikipedia parse response has no article HTML")
	}
	root, err := html.Parse(strings.NewReader(markup))
	if err != nil {
		return nil, nil, err
	}
	container := findWikiArticleBody(root)
	if container == nil {
		container = root
	}
	sections := extractWikiSections(container)
	infobox := extractWikiInfobox(container)
	return sections, infobox, nil
}

// FetchWikipediaArticleDetails fetches bounded article sections, infobox text,
// and related article links with the caller's already-hardened HTTP client.
func FetchWikipediaArticleDetails(ctx context.Context, client *http.Client, title, lang string) (*WikipediaArticleDetails, error) {
	if client == nil {
		return nil, fmt.Errorf("wikipedia HTTP client is required")
	}
	if !wikipediaLanguagePattern.MatchString(lang) {
		lang = "en"
	}
	fetcher := &WikipediaFetcher{httpClient: client}
	sections, infobox, err := fetcher.fetchArticleDetails(ctx, title, lang)
	if err != nil {
		return nil, err
	}
	related, _ := fetcher.fetchRelatedArticles(ctx, title, lang)
	return &WikipediaArticleDetails{Sections: sections, Infobox: infobox, RelatedArticles: related}, nil
}

func decodeWikipediaHTML(raw json.RawMessage) (string, error) {
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text, nil
	}
	var wrapped map[string]string
	if err := json.Unmarshal(raw, &wrapped); err != nil {
		return "", err
	}
	return wrapped["*"], nil
}

func findWikiArticleBody(node *html.Node) *html.Node {
	if node.Type == html.ElementNode && node.Data == "div" {
		for _, class := range strings.Fields(attribute(node, "class")) {
			if class == "mw-parser-output" {
				return node
			}
		}
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if found := findWikiArticleBody(child); found != nil {
			return found
		}
	}
	return nil
}

func extractWikiSections(root *html.Node) []WikipediaSection {
	sections := make([]WikipediaSection, 0, wikipediaMaxSections)
	var current *WikipediaSection
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.ElementNode && len(node.Data) == 2 && node.Data[0] == 'h' && node.Data[1] >= '2' && node.Data[1] <= '6' {
			if len(sections) == wikipediaMaxSections {
				current = nil
				return
			}
			anchor := attribute(node, "id")
			if anchor == "" {
				anchor = fmt.Sprintf("section-%d", len(sections)+1)
			}
			sections = append(sections, WikipediaSection{
				Anchor: anchor,
				Title:  strings.TrimSpace(wikiNodeText(node)),
				Level:  int(node.Data[1] - '0'),
			})
			current = &sections[len(sections)-1]
			return
		}
		if current != nil && node.Type == html.TextNode {
			appendWikiText(current, node.Data)
		}
		if current != nil && node.Type == html.ElementNode && (node.Data == "script" || node.Data == "style" || node.Data == "table") {
			return
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	for child := root.FirstChild; child != nil; child = child.NextSibling {
		walk(child)
	}
	for i := range sections {
		sections[i].Text = strings.TrimSpace(wikiPunctuationSpacingPattern.ReplaceAllString(sections[i].Text, "$1"))
		if len(sections[i].Text) > 1800 {
			sections[i].Text = sections[i].Text[:1800]
		}
	}
	return sections
}

func appendWikiText(section *WikipediaSection, text string) {
	if section == nil || len(section.Text) >= 1800 {
		return
	}
	text = strings.Join(strings.Fields(text), " ")
	if text == "" {
		return
	}
	if section.Text != "" {
		section.Text += " "
	}
	remaining := 1800 - len(section.Text)
	if len(text) > remaining {
		text = text[:remaining]
	}
	section.Text += text
}

func extractWikiInfobox(root *html.Node) []WikipediaField {
	var table *html.Node
	var find func(*html.Node)
	find = func(node *html.Node) {
		if table != nil {
			return
		}
		if node.Type == html.ElementNode && node.Data == "table" && strings.Contains(" "+attribute(node, "class")+" ", " infobox ") {
			table = node
			return
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			find(child)
		}
	}
	find(root)
	if table == nil {
		return nil
	}
	fields := make([]WikipediaField, 0, wikipediaMaxFields)
	var rows func(*html.Node)
	rows = func(node *html.Node) {
		if len(fields) >= wikipediaMaxFields {
			return
		}
		if node.Type == html.ElementNode && node.Data == "tr" {
			label := strings.TrimSpace(wikiNodeText(findWikiDescendant(node, "th")))
			value := strings.TrimSpace(wikiNodeText(findWikiDescendant(node, "td")))
			if label != "" && value != "" {
				fields = append(fields, WikipediaField{Label: truncateWikiText(label, 120), Value: truncateWikiText(value, 320)})
			}
			return
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			rows(child)
		}
	}
	rows(table)
	return fields
}

func findWikiDescendant(node *html.Node, tag string) *html.Node {
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == html.ElementNode && child.Data == tag {
			return child
		}
		if found := findWikiDescendant(child, tag); found != nil {
			return found
		}
	}
	return nil
}

func wikiNodeText(node *html.Node) string {
	if node == nil {
		return ""
	}
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(current *html.Node) {
		if current.Type == html.TextNode {
			b.WriteString(current.Data)
		} else if current.Type == html.ElementNode && (current.Data == "script" || current.Data == "style") {
			return
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(node)
	return wikiPunctuationSpacingPattern.ReplaceAllString(strings.Join(strings.Fields(b.String()), " "), "$1")
}

var wikiPunctuationSpacingPattern = regexp.MustCompile(`\s+([,.;:!?])`)

func attribute(node *html.Node, name string) string {
	for _, attr := range node.Attr {
		if attr.Key == name {
			return attr.Val
		}
	}
	return ""
}

func truncateWikiText(text string, max int) string {
	if len(text) > max {
		return text[:max]
	}
	return text
}

// parseQuery extracts the topic from natural language queries
func (f *WikipediaFetcher) parseQuery(query string) string {
	query = strings.TrimSpace(query)

	// Remove common prefixes
	prefixes := []string{
		"wiki ", "wikipedia ",
		"who is ", "who was ", "who are ",
		"what is ", "what are ", "what was ",
		"define ", "tell me about ", "information about ",
		"search for ", "look up ", "find ",
	}

	lowerQuery := strings.ToLower(query)
	for _, prefix := range prefixes {
		if strings.HasPrefix(lowerQuery, prefix) {
			query = query[len(prefix):]
			break
		}
	}

	// Remove trailing question marks
	query = strings.TrimSuffix(query, "?")
	query = strings.TrimSpace(query)

	return query
}

// fetchArticleSummary fetches a Wikipedia article summary by title
func (f *WikipediaFetcher) fetchArticleSummary(ctx context.Context, title, lang string) (*WikipediaData, error) {
	// Replace spaces with underscores for Wikipedia URLs
	encodedTitle := strings.ReplaceAll(title, " ", "_")

	apiURL := fmt.Sprintf("https://%s.wikipedia.org/api/rest_v1/page/summary/%s",
		url.PathEscape(lang),
		url.PathEscape(encodedTitle))

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Search/1.0 (privacy-focused search engine)")

	resp, err := f.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer closeutil.Close(resp.Body)

	if resp.StatusCode == 404 {
		// Not found, will try search
		return nil, nil
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("wikipedia API error: %d", resp.StatusCode)
	}

	var result struct {
		Type        string `json:"type"`
		Title       string `json:"title"`
		Extract     string `json:"extract"`
		Description string `json:"description"`
		Thumbnail   struct {
			Source string `json:"source"`
			Width  int    `json:"width"`
			Height int    `json:"height"`
		} `json:"thumbnail"`
		OriginalImage struct {
			Source string `json:"source"`
		} `json:"originalimage"`
		ContentURLs struct {
			Desktop struct {
				Page string `json:"page"`
			} `json:"desktop"`
		} `json:"content_urls"`
	}

	if err := decodeWidgetJSON(resp.Body, &result); err != nil {
		return nil, err
	}

	// Skip disambiguation pages - try search instead
	if result.Type == "disambiguation" {
		return nil, nil
	}

	thumbnail := result.Thumbnail.Source
	// Use original image if thumbnail not available
	if thumbnail == "" && result.OriginalImage.Source != "" {
		thumbnail = result.OriginalImage.Source
	}

	return &WikipediaData{
		Title:       result.Title,
		Extract:     result.Extract,
		Description: result.Description,
		Thumbnail:   thumbnail,
		URL:         result.ContentURLs.Desktop.Page,
		Language:    lang,
	}, nil
}

// searchAndFetch searches Wikipedia and fetches the top result
func (f *WikipediaFetcher) searchAndFetch(ctx context.Context, query, lang string) (*WikipediaData, error) {
	// Use MediaWiki API for search
	apiURL := fmt.Sprintf(
		"https://%s.wikipedia.org/w/api.php?action=query&list=search&srsearch=%s&srwhat=text&srlimit=1&format=json",
		url.PathEscape(lang),
		url.QueryEscape(query),
	)

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Search/1.0 (privacy-focused search engine)")

	resp, err := f.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer closeutil.Close(resp.Body)

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("wikipedia search API error: %d", resp.StatusCode)
	}

	var searchResult struct {
		Query struct {
			Search []struct {
				Title   string `json:"title"`
				Snippet string `json:"snippet"`
			} `json:"search"`
		} `json:"query"`
	}

	if err := decodeWidgetJSON(resp.Body, &searchResult); err != nil {
		return nil, err
	}

	if len(searchResult.Query.Search) == 0 {
		// No results
		return nil, nil
	}

	// Fetch the full summary for the top search result
	return f.fetchArticleSummary(ctx, searchResult.Query.Search[0].Title, lang)
}

// fetchRelatedArticles fetches related articles for a given title
func (f *WikipediaFetcher) fetchRelatedArticles(ctx context.Context, title, lang string) ([]RelatedArticle, error) {
	// Use MediaWiki API to get links from the article
	encodedTitle := strings.ReplaceAll(title, " ", "_")
	apiURL := fmt.Sprintf(
		"https://%s.wikipedia.org/w/api.php?action=query&titles=%s&prop=links&pllimit=10&plnamespace=0&format=json",
		url.PathEscape(lang),
		url.QueryEscape(encodedTitle),
	)

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Search/1.0 (privacy-focused search engine)")

	resp, err := f.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer closeutil.Close(resp.Body)

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("wikipedia links API error: %d", resp.StatusCode)
	}

	var linksResult struct {
		Query struct {
			Pages map[string]struct {
				Links []struct {
					Title string `json:"title"`
				} `json:"links"`
			} `json:"pages"`
		} `json:"query"`
	}

	if err := decodeWidgetJSON(resp.Body, &linksResult); err != nil {
		return nil, err
	}

	var related []RelatedArticle
	seenTitles := make(map[string]bool)

	// Filter for meaningful related articles
	titleRegex := regexp.MustCompile(`^[A-Z]`)

	for _, page := range linksResult.Query.Pages {
		for _, link := range page.Links {
			linkTitle := link.Title
			// Skip if already seen or doesn't start with capital letter
			if seenTitles[linkTitle] || !titleRegex.MatchString(linkTitle) {
				continue
			}
			// Skip meta pages
			if strings.Contains(linkTitle, ":") {
				continue
			}
			seenTitles[linkTitle] = true

			encodedLinkTitle := strings.ReplaceAll(linkTitle, " ", "_")
			related = append(related, RelatedArticle{
				Title: linkTitle,
				URL:   fmt.Sprintf("https://%s.wikipedia.org/wiki/%s", lang, url.PathEscape(encodedLinkTitle)),
			})

			// Limit to 5 related articles
			if len(related) >= 5 {
				break
			}
		}
		if len(related) >= 5 {
			break
		}
	}

	return related, nil
}

// CacheDuration returns how long to cache Wikipedia data
func (f *WikipediaFetcher) CacheDuration() time.Duration {
	return 1 * time.Hour
}

// WidgetType returns the widget type
func (f *WikipediaFetcher) WidgetType() WidgetType {
	return WidgetWikipedia
}
