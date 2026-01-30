package instant

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/apimgr/search/src/version"
	"golang.org/x/net/html"
)

// FeedHandler discovers RSS/Atom feeds on a website
type FeedHandler struct {
	client   *http.Client
	patterns []*regexp.Regexp
}

// FeedInfo represents a discovered feed
type FeedInfo struct {
	Title string `json:"title"`
	URL   string `json:"url"`
	Type  string `json:"type"` // "rss" or "atom"
}

// NewFeedHandler creates a new feed discovery handler
func NewFeedHandler() *FeedHandler {
	return &FeedHandler{
		client: &http.Client{Timeout: 15 * time.Second},
		patterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)^feed[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^rss[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^feeds[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^find\s+feed[s]?[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^discover\s+feed[s]?[:\s]+(.+)$`),
		},
	}
}

func (h *FeedHandler) Name() string {
	return "feed"
}

func (h *FeedHandler) Patterns() []*regexp.Regexp {
	return h.patterns
}

func (h *FeedHandler) CanHandle(query string) bool {
	for _, p := range h.patterns {
		if p.MatchString(query) {
			return true
		}
	}
	return false
}

func (h *FeedHandler) Handle(ctx context.Context, query string) (*Answer, error) {
	// Extract domain from query
	domain := ""
	for _, p := range h.patterns {
		if matches := p.FindStringSubmatch(query); len(matches) > 1 {
			domain = strings.TrimSpace(matches[1])
			break
		}
	}

	if domain == "" {
		return nil, nil
	}

	// Normalize domain to URL
	baseURL := normalizeDomainToURL(domain)

	// Discover feeds
	feeds, err := h.discoverFeeds(ctx, baseURL)
	if err != nil {
		return &Answer{
			Type:    AnswerTypeFeed,
			Query:   query,
			Title:   fmt.Sprintf("Feed Discovery: %s", domain),
			Content: fmt.Sprintf("Error discovering feeds: %v", err),
		}, nil
	}

	if len(feeds) == 0 {
		return &Answer{
			Type:    AnswerTypeFeed,
			Query:   query,
			Title:   fmt.Sprintf("Feed Discovery: %s", domain),
			Content: "No RSS/Atom feeds found on this website.",
		}, nil
	}

	// Build content
	var content strings.Builder
	content.WriteString(fmt.Sprintf("<strong>Discovered %d feed(s) on %s:</strong><br><br>", len(feeds), domain))

	for i, feed := range feeds {
		feedType := strings.ToUpper(feed.Type)
		title := feed.Title
		if title == "" {
			title = "Untitled Feed"
		}
		content.WriteString(fmt.Sprintf("%d. <strong>%s</strong> [%s]<br>", i+1, title, feedType))
		content.WriteString(fmt.Sprintf("   <a href=\"%s\">%s</a><br><br>", feed.URL, feed.URL))
	}

	return &Answer{
		Type:    AnswerTypeFeed,
		Query:   query,
		Title:   fmt.Sprintf("Feed Discovery: %s", domain),
		Content: content.String(),
		Data: map[string]interface{}{
			"domain": domain,
			"feeds":  feeds,
			"count":  len(feeds),
		},
	}, nil
}

// discoverFeeds discovers RSS/Atom feeds on a website
func (h *FeedHandler) discoverFeeds(ctx context.Context, baseURL string) ([]FeedInfo, error) {
	var feeds []FeedInfo
	seenURLs := make(map[string]bool)

	// Common feed paths to check
	commonPaths := []string{
		"/feed",
		"/feed/",
		"/rss",
		"/rss.xml",
		"/atom.xml",
		"/feed.xml",
		"/feeds/posts/default",
		"/blog/feed",
		"/blog/rss",
		"/index.xml",
		"/rss/index.xml",
		"/atom/index.xml",
	}

	// Try common paths
	for _, path := range commonPaths {
		feedURL := baseURL + path
		if seenURLs[feedURL] {
			continue
		}

		feed, err := h.checkFeedURL(ctx, feedURL)
		if err == nil && feed != nil {
			seenURLs[feedURL] = true
			feeds = append(feeds, *feed)
		}
	}

	// Parse HTML for feed links
	htmlFeeds, err := h.parseFeedLinksFromHTML(ctx, baseURL)
	if err == nil {
		for _, feed := range htmlFeeds {
			if !seenURLs[feed.URL] {
				seenURLs[feed.URL] = true
				feeds = append(feeds, feed)
			}
		}
	}

	return feeds, nil
}

// checkFeedURL checks if a URL is a valid RSS/Atom feed
func (h *FeedHandler) checkFeedURL(ctx context.Context, feedURL string) (*FeedInfo, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", feedURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", version.BrowserUserAgent)
	req.Header.Set("Accept", "application/rss+xml, application/atom+xml, application/xml, text/xml")

	resp, err := h.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}

	// Read limited body
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return nil, err
	}

	// Try to detect feed type and title
	return parseFeedInfo(feedURL, body)
}

// parseFeedInfo parses feed XML to extract info
func parseFeedInfo(feedURL string, body []byte) (*FeedInfo, error) {
	bodyStr := string(body)

	// Check for RSS
	if strings.Contains(bodyStr, "<rss") || strings.Contains(bodyStr, "<channel>") {
		title := extractXMLElement(body, "title")
		return &FeedInfo{
			Title: title,
			URL:   feedURL,
			Type:  "rss",
		}, nil
	}

	// Check for Atom
	if strings.Contains(bodyStr, "<feed") && strings.Contains(bodyStr, "xmlns=\"http://www.w3.org/2005/Atom\"") {
		title := extractXMLElement(body, "title")
		return &FeedInfo{
			Title: title,
			URL:   feedURL,
			Type:  "atom",
		}, nil
	}

	// Check for generic XML feed indicators
	if strings.Contains(bodyStr, "<?xml") && (strings.Contains(bodyStr, "<item>") || strings.Contains(bodyStr, "<entry>")) {
		title := extractXMLElement(body, "title")
		feedType := "rss"
		if strings.Contains(bodyStr, "<entry>") {
			feedType = "atom"
		}
		return &FeedInfo{
			Title: title,
			URL:   feedURL,
			Type:  feedType,
		}, nil
	}

	return nil, fmt.Errorf("not a valid feed")
}

// extractXMLElement extracts the text content of an XML element
func extractXMLElement(data []byte, element string) string {
	decoder := xml.NewDecoder(strings.NewReader(string(data)))
	for {
		tok, err := decoder.Token()
		if err != nil {
			break
		}
		if se, ok := tok.(xml.StartElement); ok && se.Name.Local == element {
			var content string
			if err := decoder.DecodeElement(&content, &se); err == nil {
				return strings.TrimSpace(content)
			}
		}
	}
	return ""
}

// parseFeedLinksFromHTML parses HTML to find feed links
func (h *FeedHandler) parseFeedLinksFromHTML(ctx context.Context, baseURL string) ([]FeedInfo, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", baseURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", version.BrowserUserAgent)

	resp, err := h.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}

	doc, err := html.Parse(resp.Body)
	if err != nil {
		return nil, err
	}

	var feeds []FeedInfo
	var parseNode func(*html.Node)
	parseNode = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "link" {
			var rel, href, title, linkType string
			for _, attr := range n.Attr {
				switch attr.Key {
				case "rel":
					rel = attr.Val
				case "href":
					href = attr.Val
				case "title":
					title = attr.Val
				case "type":
					linkType = attr.Val
				}
			}

			// Check if it's a feed link
			if rel == "alternate" && href != "" {
				feedType := ""
				if strings.Contains(linkType, "rss") {
					feedType = "rss"
				} else if strings.Contains(linkType, "atom") {
					feedType = "atom"
				}

				if feedType != "" {
					// Resolve relative URL
					feedURL := resolveURL(baseURL, href)
					feeds = append(feeds, FeedInfo{
						Title: title,
						URL:   feedURL,
						Type:  feedType,
					})
				}
			}
		}

		for c := n.FirstChild; c != nil; c = c.NextSibling {
			parseNode(c)
		}
	}

	parseNode(doc)
	return feeds, nil
}

// normalizeDomainToURL normalizes a domain to a full URL
func normalizeDomainToURL(domain string) string {
	domain = strings.TrimSpace(domain)

	// Remove protocol if already present and re-add https
	domain = strings.TrimPrefix(domain, "http://")
	domain = strings.TrimPrefix(domain, "https://")

	// Remove trailing slash
	domain = strings.TrimSuffix(domain, "/")

	return "https://" + domain
}

// resolveURL resolves a relative URL against a base URL
func resolveURL(baseURL, href string) string {
	if strings.HasPrefix(href, "http://") || strings.HasPrefix(href, "https://") {
		return href
	}

	base, err := url.Parse(baseURL)
	if err != nil {
		return href
	}

	ref, err := url.Parse(href)
	if err != nil {
		return href
	}

	return base.ResolveReference(ref).String()
}
