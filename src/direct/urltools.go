package direct

import (
	"bufio"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/apimgr/search/src/common/closeutil"
	"github.com/apimgr/search/src/common/i18n"
	"github.com/apimgr/search/src/version"
	xhtml "golang.org/x/net/html"
	"golang.org/x/net/idna"
	"golang.org/x/net/publicsuffix"
)

// RobotsHandler handles robots:{domain} queries
type RobotsHandler struct {
	client *http.Client
}

// NewRobotsHandler creates a new robots.txt handler
func NewRobotsHandler() *RobotsHandler {
	return &RobotsHandler{
		client: newPublicHTTPClient(10 * time.Second),
	}
}

func (h *RobotsHandler) Type() AnswerType {
	return AnswerTypeRobots
}

func (h *RobotsHandler) HandleDirectQuery(ctx context.Context, term string) (*Answer, error) {
	lang := LangFromContext(ctx)
	term = strings.TrimSpace(term)
	if term == "" {
		return nil, fmt.Errorf("domain required")
	}

	// Build robots.txt URL
	robotsURL := term
	if !strings.HasPrefix(strings.ToLower(term), "http://") && !strings.HasPrefix(strings.ToLower(term), "https://") {
		robotsURL = "https://" + term
	}
	if !strings.HasSuffix(robotsURL, "/robots.txt") {
		robotsURL = strings.TrimSuffix(robotsURL, "/") + "/robots.txt"
	}
	if _, err := validatePublicHTTPURL(robotsURL); err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "GET", robotsURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", version.BrowserUserAgent)

	resp, err := h.client.Do(req)
	if err != nil {
		return &Answer{
			Type:        AnswerTypeRobots,
			Term:        term,
			Title:       fmt.Sprintf("robots.txt: %s", term),
			Description: i18n.T(lang, "direct.failed_to_fetch"),
			Content:     fmt.Sprintf("<p class=\"error\">Failed to fetch robots.txt: %s</p>", escapeHTML(err.Error())),
			Error:       "fetch_failed",
		}, nil
	}
	defer closeutil.Close(resp.Body)

	if resp.StatusCode == http.StatusNotFound {
		return &Answer{
			Type:        AnswerTypeRobots,
			Term:        term,
			Title:       fmt.Sprintf("robots.txt: %s", term),
			Description: i18n.T(lang, "direct.not_found"),
			Content:     "<p>No robots.txt file found. This means all user-agents are allowed to crawl all paths.</p>",
			Error:       "not_found",
		}, nil
	}

	// Limit to 100KB
	body, err := io.ReadAll(io.LimitReader(resp.Body, 100*1024))
	if err != nil {
		return nil, err
	}

	content := string(body)
	analysis := analyzeRobotsTxt(content)

	data := map[string]interface{}{
		"url":      robotsURL,
		"content":  content,
		"analysis": analysis,
	}

	return &Answer{
		Type:        AnswerTypeRobots,
		Term:        term,
		Title:       fmt.Sprintf("robots.txt: %s", term),
		Description: i18n.T(lang, "direct.robots_exclusion_protocol_file"),
		Content:     formatRobotsContent(term, content, analysis),
		Source:      "Direct Fetch",
		SourceURL:   robotsURL,
		Data:        data,
	}, nil
}

func analyzeRobotsTxt(content string) map[string]interface{} {
	analysis := make(map[string]interface{})

	lines := strings.Split(content, "\n")
	userAgents := []string{}
	disallowed := []string{}
	allowed := []string{}
	sitemaps := []string{}

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}

		directive := strings.ToLower(strings.TrimSpace(parts[0]))
		value := strings.TrimSpace(parts[1])

		switch directive {
		case "user-agent":
			if value != "" {
				userAgents = append(userAgents, value)
			}
		case "disallow":
			if value != "" {
				disallowed = append(disallowed, value)
			}
		case "allow":
			if value != "" {
				allowed = append(allowed, value)
			}
		case "sitemap":
			if value != "" {
				sitemaps = append(sitemaps, value)
			}
		}
	}

	analysis["userAgents"] = userAgents
	analysis["disallowed"] = disallowed
	analysis["allowed"] = allowed
	analysis["sitemaps"] = sitemaps

	return analysis
}

func formatRobotsContent(domain, content string, analysis map[string]interface{}) string {
	var html strings.Builder
	html.WriteString("<div class=\"robots-content\">")
	fmt.Fprintf(&html, "<h1>robots.txt: %s</h1>", escapeHTML(domain))

	// Analysis summary
	html.WriteString("<h2>Analysis</h2>")

	if ua, ok := analysis["userAgents"].([]string); ok && len(ua) > 0 {
		html.WriteString("<h3>User-Agents</h3><ul>")
		for _, agent := range ua {
			fmt.Fprintf(&html, "<li><code>%s</code></li>", escapeHTML(agent))
		}
		html.WriteString("</ul>")
	}

	if disallowed, ok := analysis["disallowed"].([]string); ok && len(disallowed) > 0 {
		html.WriteString("<h3>Disallowed Paths</h3><ul>")
		for _, path := range disallowed {
			fmt.Fprintf(&html, "<li><code>%s</code></li>", escapeHTML(path))
		}
		html.WriteString("</ul>")
	}

	if sitemaps, ok := analysis["sitemaps"].([]string); ok && len(sitemaps) > 0 {
		html.WriteString("<h3>Sitemaps</h3><ul>")
		for _, sitemap := range sitemaps {
			fmt.Fprintf(&html, "<li><a href=\"%s\">%s</a></li>", escapeHTML(sitemap), escapeHTML(sitemap))
		}
		html.WriteString("</ul>")
	}

	// Raw content
	html.WriteString("<h2>Raw Content</h2>")
	fmt.Fprintf(&html, "<pre class=\"robots-raw\"><code>%s</code></pre>", escapeHTML(content))

	html.WriteString("</div>")
	return html.String()
}

// SitemapHandler handles sitemap:{domain} queries
type SitemapHandler struct {
	client *http.Client
}

// NewSitemapHandler creates a new sitemap handler
func NewSitemapHandler() *SitemapHandler {
	return &SitemapHandler{
		client: newPublicHTTPClient(15 * time.Second),
	}
}

func (h *SitemapHandler) Type() AnswerType {
	return AnswerTypeSitemap
}

func (h *SitemapHandler) HandleDirectQuery(ctx context.Context, term string) (*Answer, error) {
	lang := LangFromContext(ctx)
	term = strings.TrimSpace(term)
	if term == "" {
		return nil, fmt.Errorf("domain required")
	}

	// Build sitemap URL
	sitemapURL := term
	if !strings.HasPrefix(strings.ToLower(term), "http://") && !strings.HasPrefix(strings.ToLower(term), "https://") {
		sitemapURL = "https://" + term
	}
	if !strings.Contains(sitemapURL, "sitemap") {
		sitemapURL = strings.TrimSuffix(sitemapURL, "/") + "/sitemap.xml"
	}
	if _, err := validatePublicHTTPURL(sitemapURL); err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "GET", sitemapURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", version.BrowserUserAgent)

	resp, err := h.client.Do(req)
	if err != nil {
		return &Answer{
			Type:        AnswerTypeSitemap,
			Term:        term,
			Title:       fmt.Sprintf("Sitemap: %s", term),
			Description: i18n.T(lang, "direct.failed_to_fetch"),
			Content:     fmt.Sprintf("<p class=\"error\">Failed to fetch sitemap: %s</p>", escapeHTML(err.Error())),
			Error:       "fetch_failed",
		}, nil
	}
	defer closeutil.Close(resp.Body)

	if resp.StatusCode == http.StatusNotFound {
		return &Answer{
			Type:        AnswerTypeSitemap,
			Term:        term,
			Title:       fmt.Sprintf("Sitemap: %s", term),
			Description: i18n.T(lang, "direct.not_found"),
			Content:     "<p>No sitemap.xml found at the standard location.</p>",
			Error:       "not_found",
		}, nil
	}

	// Limit to 500KB
	body, err := io.ReadAll(io.LimitReader(resp.Body, 500*1024))
	if err != nil {
		return nil, err
	}

	// Parse sitemap XML
	urls, sitemapIndex := parseSitemap(body)

	data := map[string]interface{}{
		"url":          sitemapURL,
		"urlCount":     len(urls),
		"isIndex":      len(sitemapIndex) > 0,
		"sitemapIndex": sitemapIndex,
	}

	return &Answer{
		Type:        AnswerTypeSitemap,
		Term:        term,
		Title:       fmt.Sprintf("Sitemap: %s", term),
		Description: fmt.Sprintf("%d URLs found", len(urls)),
		Content:     formatSitemapContent(term, urls, sitemapIndex),
		Source:      "Direct Fetch",
		SourceURL:   sitemapURL,
		Data:        data,
	}, nil
}

type sitemapURL struct {
	Loc        string `xml:"loc"`
	LastMod    string `xml:"lastmod"`
	ChangeFreq string `xml:"changefreq"`
	Priority   string `xml:"priority"`
}

func parseSitemap(data []byte) ([]sitemapURL, []string) {
	// Try as URL set
	var urlset struct {
		URLs []sitemapURL `xml:"url"`
	}
	if err := xml.Unmarshal(data, &urlset); err != nil {
		return nil, nil
	}

	// Try as sitemap index
	var index struct {
		Sitemaps []struct {
			Loc string `xml:"loc"`
		} `xml:"sitemap"`
	}
	if err := xml.Unmarshal(data, &index); err != nil {
		return nil, nil
	}

	sitemapIndex := make([]string, len(index.Sitemaps))
	for i, sm := range index.Sitemaps {
		sitemapIndex[i] = sm.Loc
	}

	return urlset.URLs, sitemapIndex
}

func formatSitemapContent(domain string, urls []sitemapURL, sitemapIndex []string) string {
	var html strings.Builder
	html.WriteString("<div class=\"sitemap-content\">")
	fmt.Fprintf(&html, "<h1>Sitemap: %s</h1>", escapeHTML(domain))

	if len(sitemapIndex) > 0 {
		html.WriteString("<h2>Sitemap Index</h2>")
		fmt.Fprintf(&html, "<p>Found %d child sitemaps</p>", len(sitemapIndex))
		html.WriteString("<ul>")
		for _, sm := range sitemapIndex {
			fmt.Fprintf(&html, "<li><a href=\"%s\">%s</a></li>", escapeHTML(sm), escapeHTML(sm))
		}
		html.WriteString("</ul>")
	}

	if len(urls) > 0 {
		html.WriteString("<h2>URLs</h2>")
		fmt.Fprintf(&html, "<p>Found %d URLs</p>", len(urls))
		html.WriteString("<table class=\"sitemap-table\">")
		html.WriteString("<thead><tr><th>URL</th><th>Last Modified</th><th>Priority</th></tr></thead>")
		html.WriteString("<tbody>")

		maxShow := 50
		for i, u := range urls {
			if i >= maxShow {
				fmt.Fprintf(&html, "<tr><td colspan=\"3\">... and %d more URLs</td></tr>", len(urls)-maxShow)
				break
			}
			fmt.Fprintf(&html, "<tr><td><a href=\"%s\">%s</a></td><td>%s</td><td>%s</td></tr>",
				escapeHTML(u.Loc), escapeHTML(truncateURL(u.Loc, 60)), escapeHTML(u.LastMod), escapeHTML(u.Priority))
		}

		html.WriteString("</tbody></table>")
	}

	html.WriteString("</div>")
	return html.String()
}

func truncateURL(u string, max int) string {
	if len(u) <= max {
		return u
	}
	return u[:max-3] + "..."
}

// TechHandler handles tech:{domain} queries (technology detection)
type TechHandler struct {
	client *http.Client
}

// NewTechHandler creates a new technology detection handler
func NewTechHandler() *TechHandler {
	return &TechHandler{
		client: newPublicHTTPClient(15 * time.Second),
	}
}

func (h *TechHandler) Type() AnswerType {
	return AnswerTypeTech
}

func pageFetchFailure(answerType AnswerType, term, description, source, code, message string) *Answer {
	title := fmt.Sprintf("Tech Stack: %s", term)
	if answerType == AnswerTypeFeed {
		title = fmt.Sprintf("Feeds: %s", term)
	}
	return &Answer{
		Type: answerType, Term: term, Title: title, Description: description,
		Content: fmt.Sprintf("<p class=\"error\">%s</p>", escapeHTML(message)),
		Source:  source, Error: code,
	}
}

func (h *TechHandler) HandleDirectQuery(ctx context.Context, term string) (*Answer, error) {
	lang := LangFromContext(ctx)
	term = strings.TrimSpace(term)
	if term == "" {
		return nil, fmt.Errorf("domain required")
	}

	targetURL := term
	if !strings.HasPrefix(strings.ToLower(term), "http://") && !strings.HasPrefix(strings.ToLower(term), "https://") {
		targetURL = "https://" + term
	}
	if _, err := validatePublicHTTPURL(targetURL); err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "GET", targetURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", version.BrowserUserAgent)

	resp, err := h.client.Do(req)
	if err != nil {
		return pageFetchFailure(AnswerTypeTech, term, i18n.T(lang, "direct.failed_to_analyze"), "Technology Detector", "fetch_failed", "The site could not be fetched."), nil
	}
	defer closeutil.Close(resp.Body)
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return pageFetchFailure(AnswerTypeTech, term, i18n.T(lang, "direct.failed_to_analyze"), "Technology Detector", "fetch_failed", fmt.Sprintf("The site returned HTTP %d.", resp.StatusCode)), nil
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, (200*1024)+1))
	if err != nil || len(body) > 200*1024 {
		return pageFetchFailure(AnswerTypeTech, term, i18n.T(lang, "direct.failed_to_analyze"), "Technology Detector", "fetch_failed", "The site response could not be read safely."), nil
	}

	// Analyze headers and HTML
	tech := detectTechnologies(resp.Header, string(body))

	data := map[string]interface{}{
		"url":          targetURL,
		"technologies": tech,
	}

	return &Answer{
		Type:        AnswerTypeTech,
		Term:        term,
		Title:       fmt.Sprintf("Tech Stack: %s", term),
		Description: i18n.T(lang, "direct.technology_detection"),
		Content:     formatTechContent(term, tech),
		Source:      "Technology Detector",
		Data:        data,
	}, nil
}

func detectTechnologies(headers http.Header, html string) map[string][]string {
	tech := make(map[string][]string)

	// Server header
	if server := headers.Get("Server"); server != "" {
		tech["Server"] = []string{server}
	}

	// Powered-By header
	if powered := headers.Get("X-Powered-By"); powered != "" {
		tech["Runtime"] = append(tech["Runtime"], powered)
	}

	// Check HTML for frameworks/libraries
	htmlLower := strings.ToLower(html)

	// JavaScript frameworks
	jsFrameworks := []struct{ name, pattern string }{
		{"React", "react"},
		{"Vue.js", "vue"},
		{"Angular", "angular"},
		{"jQuery", "jquery"},
		{"Bootstrap", "bootstrap"},
		{"Tailwind CSS", "tailwind"},
		{"Next.js", "next"},
		{"Nuxt.js", "nuxt"},
		{"Svelte", "svelte"},
	}

	for _, fw := range jsFrameworks {
		if strings.Contains(htmlLower, fw.pattern) {
			tech["JavaScript"] = append(tech["JavaScript"], fw.name)
		}
	}

	// CMS/Platforms
	cms := []struct{ name, pattern string }{
		{"WordPress", "wp-content"},
		{"Drupal", "drupal"},
		{"Joomla", "joomla"},
		{"Shopify", "shopify"},
		{"Squarespace", "squarespace"},
		{"Wix", "wix.com"},
		{"Ghost", "ghost"},
	}

	for _, c := range cms {
		if strings.Contains(htmlLower, c.pattern) {
			tech["CMS/Platform"] = append(tech["CMS/Platform"], c.name)
		}
	}

	// Analytics
	analytics := []struct{ name, pattern string }{
		{"Google Analytics", "google-analytics"},
		{"Google Tag Manager", "googletagmanager"},
		{"Facebook Pixel", "facebook.net"},
		{"Hotjar", "hotjar"},
		{"Mixpanel", "mixpanel"},
	}

	for _, a := range analytics {
		if strings.Contains(htmlLower, a.pattern) {
			tech["Analytics"] = append(tech["Analytics"], a.name)
		}
	}

	// CDN detection from headers
	if headers.Get("CF-Ray") != "" {
		tech["CDN"] = append(tech["CDN"], "Cloudflare")
	}
	if strings.Contains(headers.Get("Server"), "cloudflare") {
		tech["CDN"] = append(tech["CDN"], "Cloudflare")
	}
	if strings.Contains(headers.Get("Via"), "CloudFront") {
		tech["CDN"] = append(tech["CDN"], "Amazon CloudFront")
	}

	return tech
}

func formatTechContent(domain string, tech map[string][]string) string {
	var html strings.Builder
	html.WriteString("<div class=\"tech-content\">")
	fmt.Fprintf(&html, "<h1>Technology Stack: %s</h1>", escapeHTML(domain))

	if len(tech) == 0 {
		html.WriteString("<p>No technologies detected.</p>")
	} else {
		for category, items := range tech {
			if len(items) > 0 {
				fmt.Fprintf(&html, "<h2>%s</h2>", escapeHTML(category))
				html.WriteString("<ul>")
				for _, item := range items {
					fmt.Fprintf(&html, "<li>%s</li>", escapeHTML(item))
				}
				html.WriteString("</ul>")
			}
		}
	}

	html.WriteString("<p class=\"note\">Detection is based on HTTP headers and HTML content analysis.</p>")
	html.WriteString("</div>")
	return html.String()
}

// FeedHandler handles feed:{domain} queries (RSS/Atom discovery)
type FeedHandler struct {
	client *http.Client
}

// NewFeedHandler creates a new feed discovery handler
func NewFeedHandler() *FeedHandler {
	return &FeedHandler{
		client: newPublicHTTPClient(10 * time.Second),
	}
}

func (h *FeedHandler) Type() AnswerType {
	return AnswerTypeFeed
}

func (h *FeedHandler) HandleDirectQuery(ctx context.Context, term string) (*Answer, error) {
	lang := LangFromContext(ctx)
	term = strings.TrimSpace(term)
	if term == "" {
		return nil, fmt.Errorf("domain required")
	}

	targetURL := term
	if !strings.HasPrefix(strings.ToLower(term), "http://") && !strings.HasPrefix(strings.ToLower(term), "https://") {
		targetURL = "https://" + term
	}
	if _, err := validatePublicHTTPURL(targetURL); err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "GET", targetURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", version.BrowserUserAgent)

	resp, err := h.client.Do(req)
	if err != nil {
		return pageFetchFailure(AnswerTypeFeed, term, i18n.T(lang, "direct.failed_to_fetch"), "Feed Discovery", "fetch_failed", "The site could not be fetched."), nil
	}
	defer closeutil.Close(resp.Body)
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return pageFetchFailure(AnswerTypeFeed, term, i18n.T(lang, "direct.failed_to_fetch"), "Feed Discovery", "fetch_failed", fmt.Sprintf("The site returned HTTP %d.", resp.StatusCode)), nil
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, (100*1024)+1))
	if err != nil || len(body) > 100*1024 {
		return pageFetchFailure(AnswerTypeFeed, term, i18n.T(lang, "direct.failed_to_fetch"), "Feed Discovery", "fetch_failed", "The site response could not be read safely."), nil
	}

	// Discover and fetch feeds with strict caps so a page with many feed links
	// cannot fan out arbitrary or unbounded outbound requests.
	feeds := discoverFeeds(targetURL, string(body))
	feeds = h.fetchDiscoveredFeeds(ctx, feeds)

	data := map[string]interface{}{
		"url":   targetURL,
		"feeds": feeds,
	}

	return &Answer{
		Type:        AnswerTypeFeed,
		Term:        term,
		Title:       fmt.Sprintf("Feeds: %s", term),
		Description: fmt.Sprintf("%d feeds found", len(feeds)),
		Content:     formatFeedContent(term, feeds),
		Source:      "Feed Discovery",
		Data:        data,
	}, nil
}

type feedInfo struct {
	URL         string
	Type        string
	Title       string
	Description string
	LastUpdate  string
	EntryCount  int
	Method      string
	Entries     []feedEntry
}

type feedEntry struct {
	Title       string
	URL         string
	Description string
	Published   string
}

func stripFeedHTML(value string) string {
	tokenizer := xhtml.NewTokenizer(strings.NewReader(value))
	var text strings.Builder
	for {
		tokenType := tokenizer.Next()
		if tokenType == xhtml.ErrorToken {
			break
		}
		if tokenType == xhtml.TextToken {
			if text.Len() > 0 {
				text.WriteByte(' ')
			}
			text.Write(tokenizer.Text())
		}
	}
	return strings.Join(strings.Fields(text.String()), " ")
}

func (h *FeedHandler) fetchDiscoveredFeeds(ctx context.Context, feeds []feedInfo) []feedInfo {
	if len(feeds) > 12 {
		feeds = feeds[:12]
	}
	ctx, cancelAll := context.WithTimeout(ctx, 8*time.Second)
	defer cancelAll()
	updated := make([]feedInfo, 0, len(feeds))
	for _, feed := range feeds {
		if _, err := validatePublicHTTPURL(feed.URL); err != nil {
			continue
		}
		feedCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
		request, err := http.NewRequestWithContext(feedCtx, http.MethodGet, feed.URL, nil)
		if err == nil {
			request.Header.Set("User-Agent", version.BrowserUserAgent)
			response, requestErr := h.client.Do(request)
			if requestErr == nil {
				if response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices {
					contentType, _, _ := mime.ParseMediaType(response.Header.Get("Content-Type"))
					payload, readErr := io.ReadAll(io.LimitReader(response.Body, (1024*1024)+1))
					if readErr == nil && len(payload) <= 1024*1024 {
						if parseFeedDocument(&feed, contentType, payload) {
							updated = append(updated, feed)
						}
					}
				}
				closeutil.Close(response.Body)
			}
		}
		cancel()
	}
	return updated
}

func parseFeedDocument(feed *feedInfo, contentType string, payload []byte) bool {
	if strings.Contains(contentType, "json") || strings.HasPrefix(strings.TrimSpace(string(payload)), "{") {
		var document struct {
			Title       string `json:"title"`
			Description string `json:"description"`
			HomePageURL string `json:"home_page_url"`
			Items       []struct {
				ID            string `json:"id"`
				URL           string `json:"url"`
				Title         string `json:"title"`
				ContentText   string `json:"content_text"`
				ContentHTML   string `json:"content_html"`
				Summary       string `json:"summary"`
				DatePublished string `json:"date_published"`
			} `json:"items"`
		}
		if json.Unmarshal(payload, &document) != nil || (document.Title == "" && document.Description == "" && len(document.Items) == 0) {
			return false
		}
		feed.Type, feed.Title, feed.Description, feed.EntryCount = "JSON Feed", document.Title, document.Description, len(document.Items)
		for _, item := range document.Items {
			url := item.URL
			if url == "" {
				url = item.ID
			}
			description := item.Summary
			if description == "" {
				description = item.ContentText
			}
			if description == "" {
				description = item.ContentHTML
			}
			feed.Entries = append(feed.Entries, feedEntry{Title: item.Title, URL: url, Description: stripFeedHTML(description), Published: item.DatePublished})
			if feed.LastUpdate == "" {
				feed.LastUpdate = item.DatePublished
			}
			if len(feed.Entries) == 5 {
				break
			}
		}
		return true
	}
	var document struct {
		Channel struct {
			Title       string `xml:"title"`
			Description string `xml:"description"`
			Updated     string `xml:"lastBuildDate"`
			Items       []struct {
				Title       string `xml:"title"`
				Link        string `xml:"link"`
				Description string `xml:"description"`
				Published   string `xml:"pubDate"`
			} `xml:"item"`
		} `xml:"channel"`
		AtomTitle   string `xml:"title"`
		AtomDesc    string `xml:"subtitle"`
		AtomUpdated string `xml:"updated"`
		AtomEntries []struct {
			Title string `xml:"title"`
			Link  struct {
				Href string `xml:"href,attr"`
			} `xml:"link"`
			Summary string `xml:"summary"`
			Content string `xml:"content"`
			Updated string `xml:"updated"`
			Pub     string `xml:"published"`
		} `xml:"entry"`
	}
	if xml.Unmarshal(payload, &document) != nil || (document.Channel.Title == "" && document.AtomTitle == "" && len(document.Channel.Items) == 0 && len(document.AtomEntries) == 0) {
		return false
	}
	feed.Type, feed.Title, feed.Description = "RSS", document.Channel.Title, document.Channel.Description
	items := document.Channel.Items
	feed.LastUpdate = document.Channel.Updated
	if document.AtomTitle != "" {
		feed.Type, feed.Title, feed.Description, feed.LastUpdate = "Atom", document.AtomTitle, document.AtomDesc, document.AtomUpdated
		for _, item := range document.AtomEntries {
			description := item.Summary
			if description == "" {
				description = item.Content
			}
			published := item.Pub
			if published == "" {
				published = item.Updated
			}
			feed.Entries = append(feed.Entries, feedEntry{Title: item.Title, URL: item.Link.Href, Description: stripFeedHTML(description), Published: published})
			if feed.LastUpdate == "" {
				feed.LastUpdate = published
			}
			if len(feed.Entries) == 5 {
				break
			}
		}
		feed.EntryCount = len(document.AtomEntries)
	} else {
		for _, item := range items {
			feed.Entries = append(feed.Entries, feedEntry{Title: item.Title, URL: item.Link, Description: stripFeedHTML(item.Description), Published: item.Published})
			if feed.LastUpdate == "" {
				feed.LastUpdate = item.Published
			}
			if len(feed.Entries) == 5 {
				break
			}
		}
		feed.EntryCount = len(items)
	}
	return true
}

func discoverFeeds(baseURL, html string) []feedInfo {
	var feeds []feedInfo

	base, _ := url.Parse(baseURL)

	// Look for link tags
	patterns := []struct{ pattern, feedType string }{
		{`<link[^>]+type="application/rss\+xml"[^>]*>`, "RSS"},
		{`<link[^>]+type="application/atom\+xml"[^>]*>`, "Atom"},
		{`<link[^>]+type="application/feed\+json"[^>]*>`, "JSON Feed"},
	}

	for _, p := range patterns {
		re := regexp.MustCompile(p.pattern)
		matches := re.FindAllString(html, -1)
		for _, match := range matches {
			href := extractAttr(match, "href")
			title := extractAttr(match, "title")

			if href != "" {
				feedURL := resolveURL(base, href)
				feeds = append(feeds, feedInfo{
					URL:    feedURL,
					Type:   p.feedType,
					Title:  title,
					Method: "HTML autodiscovery",
				})
			}
		}
	}

	// Check common feed paths
	commonPaths := []struct{ path, feedType string }{
		{"/feed", "RSS"},
		{"/rss", "RSS"},
		{"/rss.xml", "RSS"},
		{"/feed.xml", "RSS"},
		{"/atom.xml", "Atom"},
		{"/index.xml", "RSS"},
		{"/feed.json", "JSON Feed"},
	}

	for _, cp := range commonPaths {
		feedURL := base.Scheme + "://" + base.Host + cp.path
		// Check if already discovered
		found := false
		for _, f := range feeds {
			if f.URL == feedURL {
				found = true
				break
			}
		}
		if !found {
			feeds = append(feeds, feedInfo{
				URL:    feedURL,
				Type:   cp.feedType + " (guessed)",
				Method: "common feed path",
			})
		}
	}

	return feeds
}

func extractAttr(tag, attr string) string {
	patterns := []string{
		attr + `="([^"]*)"`,
		attr + `='([^']*)'`,
	}
	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		if matches := re.FindStringSubmatch(tag); len(matches) > 1 {
			return matches[1]
		}
	}
	return ""
}

func resolveURL(base *url.URL, href string) string {
	ref, err := url.Parse(href)
	if err != nil {
		return href
	}
	return base.ResolveReference(ref).String()
}

func formatFeedContent(domain string, feeds []feedInfo) string {
	var html strings.Builder
	html.WriteString("<div class=\"feed-content\">")
	fmt.Fprintf(&html, "<h1>Feeds: %s</h1>", escapeHTML(domain))

	if len(feeds) == 0 {
		html.WriteString("<p>No feeds discovered.</p>")
	} else {
		fmt.Fprintf(&html, "<p>Found %d feeds</p>", len(feeds))

		for _, f := range feeds {
			title := f.Title
			if title == "" {
				title = "-"
			}
			html.WriteString("<section class=\"feed-item\">")
			fmt.Fprintf(&html, "<h2>%s</h2>", escapeHTML(title))
			fmt.Fprintf(&html, "<p><a href=\"%s\">%s</a> · %s · discovered by %s</p>", escapeHTML(f.URL), escapeHTML(truncateURL(f.URL, 50)), escapeHTML(strings.TrimSuffix(f.Type, " (guessed)")), escapeHTML(f.Method))
			if f.Description != "" {
				fmt.Fprintf(&html, "<p>%s</p>", escapeHTML(f.Description))
			}
			if f.LastUpdate != "" {
				fmt.Fprintf(&html, "<p>Last update: <time>%s</time></p>", escapeHTML(f.LastUpdate))
			}
			fmt.Fprintf(&html, "<p>Entries: %d</p>", f.EntryCount)
			fmt.Fprintf(&html, "<p><a href=\"feed:%s\">Subscribe with a feed reader</a> · <a href=\"https://feedly.com/i/subscription/feed/%s\" rel=\"noopener noreferrer\">Feedly</a> · <a href=\"https://www.inoreader.com/?add_feed=%s\" rel=\"noopener noreferrer\">Inoreader</a></p>", escapeHTML(f.URL), escapeHTML(url.QueryEscape(f.URL)), escapeHTML(url.QueryEscape(f.URL)))
			if len(f.Entries) > 0 {
				html.WriteString("<h3>Latest entries</h3><ol>")
				for _, entry := range f.Entries {
					html.WriteString("<li>")
					if entry.URL != "" {
						if parsed, err := url.Parse(entry.URL); err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != "" {
							fmt.Fprintf(&html, "<a href=\"%s\" rel=\"noopener noreferrer\">%s</a>", escapeHTML(entry.URL), escapeHTML(entry.Title))
						} else {
							html.WriteString(escapeHTML(entry.Title))
						}
					} else {
						html.WriteString(escapeHTML(entry.Title))
					}
					if entry.Published != "" {
						html.WriteString(" <time>" + escapeHTML(entry.Published) + "</time>")
					}
					if entry.Description != "" {
						html.WriteString("<p>" + escapeHTML(entry.Description) + "</p>")
					}
					html.WriteString("</li>")
				}
				html.WriteString("</ol>")
			}
			html.WriteString("</section>")
		}
	}

	html.WriteString("</div>")
	return html.String()
}

// ExpandHandler handles expand:{url} queries (URL expander)
type ExpandHandler struct {
	client *http.Client
}

// NewExpandHandler creates a new URL expander handler
func NewExpandHandler() *ExpandHandler {
	return &ExpandHandler{
		client: newPublicHTTPClient(15 * time.Second),
	}
}

func (h *ExpandHandler) Type() AnswerType {
	return AnswerTypeExpand
}

func (h *ExpandHandler) HandleDirectQuery(ctx context.Context, term string) (*Answer, error) {
	lang := LangFromContext(ctx)
	term = strings.TrimSpace(term)
	if term == "" {
		return nil, fmt.Errorf("URL required")
	}

	targetURL := term
	if !strings.HasPrefix(strings.ToLower(term), "http://") && !strings.HasPrefix(strings.ToLower(term), "https://") {
		targetURL = "https://" + term
	}
	if _, err := validatePublicHTTPURL(targetURL); err != nil {
		return nil, err
	}

	// Track redirects
	redirects := []string{targetURL}

	client := *h.client
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return fmt.Errorf("too many redirects")
		}
		if _, err := validatePublicHTTPURL(req.URL.String()); err != nil {
			return err
		}
		if err := validatePublicDirectoryHost(req.Context(), req.URL.Hostname()); err != nil {
			return err
		}
		redirects = append(redirects, req.URL.String())
		return nil
	}

	req, err := http.NewRequestWithContext(ctx, "GET", targetURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", version.BrowserUserAgent)

	resp, err := client.Do(req)
	if err != nil && !strings.Contains(err.Error(), "redirect") {
		return &Answer{
			Type:        AnswerTypeExpand,
			Term:        term,
			Title:       i18n.T(lang, "direct.url_expander_title"),
			Description: i18n.T(lang, "direct.failed_to_expand"),
			Content:     fmt.Sprintf("<p class=\"error\">Failed to expand URL: %s</p>", escapeHTML(err.Error())),
			Error:       "expand_failed",
		}, nil
	}
	if resp != nil {
		defer closeutil.Close(resp.Body)
		redirects = append(redirects, resp.Request.URL.String())
	}

	// Remove duplicates from redirects
	seen := make(map[string]bool)
	uniqueRedirects := []string{}
	for _, r := range redirects {
		if !seen[r] {
			seen[r] = true
			uniqueRedirects = append(uniqueRedirects, r)
		}
	}

	finalURL := uniqueRedirects[len(uniqueRedirects)-1]

	data := map[string]interface{}{
		"original":  targetURL,
		"final":     finalURL,
		"redirects": uniqueRedirects,
		"hops":      len(uniqueRedirects) - 1,
	}

	return &Answer{
		Type:        AnswerTypeExpand,
		Term:        term,
		Title:       i18n.T(lang, "direct.url_expander_title"),
		Description: fmt.Sprintf("Expanded to: %s", finalURL),
		Content:     formatExpandContent(targetURL, finalURL, uniqueRedirects),
		Source:      "URL Expander",
		Data:        data,
	}, nil
}

func formatExpandContent(original, final string, redirects []string) string {
	var html strings.Builder
	html.WriteString("<div class=\"expand-content\">")
	html.WriteString("<h1>URL Expander</h1>")

	html.WriteString("<h2>Original URL</h2>")
	fmt.Fprintf(&html, "<p><code>%s</code></p>", escapeHTML(original))

	html.WriteString("<h2>Final URL</h2>")
	fmt.Fprintf(&html, "<p><a href=\"%s\">%s</a></p>", escapeHTML(final), escapeHTML(final))

	if len(redirects) > 1 {
		fmt.Fprintf(&html, "<h2>Redirect Chain (%d hops)</h2>", len(redirects)-1)
		html.WriteString("<ol class=\"redirect-chain\">")
		for i, r := range redirects {
			if i == 0 {
				fmt.Fprintf(&html, "<li><strong>Start:</strong> <code>%s</code></li>", escapeHTML(r))
			} else if i == len(redirects)-1 {
				fmt.Fprintf(&html, "<li><strong>Final:</strong> <code>%s</code></li>", escapeHTML(r))
			} else {
				fmt.Fprintf(&html, "<li><code>%s</code></li>", escapeHTML(r))
			}
		}
		html.WriteString("</ol>")
	}

	html.WriteString("</div>")
	return html.String()
}

// SafeHandler handles safe:{url} queries (URL safety check)
type SafeHandler struct {
	client             *http.Client
	apiKey             string
	reputationEndpoint string
	phishTankAPIKey    string
	phishTankEndpoint  string
	domainBlacklist    func(string) (bool, bool)
}

// NewSafeHandler creates a new URL safety handler
func NewSafeHandler() *SafeHandler {
	client := newPublicHTTPClient(8 * time.Second)
	client.CheckRedirect = func(request *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return fmt.Errorf("too many redirects")
		}
		if request.URL.Scheme != "http" && request.URL.Scheme != "https" {
			return fmt.Errorf("redirect uses a disallowed URL scheme")
		}
		if err := validatePublicDirectoryHost(request.Context(), request.URL.Hostname()); err != nil {
			return err
		}
		if redirects, ok := request.Context().Value(safeRedirectsContextKey{}).(*[]string); ok {
			*redirects = append(*redirects, request.URL.String())
		}
		return nil
	}
	return &SafeHandler{
		client:             client,
		apiKey:             os.Getenv("GOOGLE_SAFE_BROWSING_API_KEY"),
		reputationEndpoint: "https://safebrowsing.googleapis.com/v4/threatMatches:find",
		phishTankAPIKey:    os.Getenv("PHISHTANK_API_KEY"),
		phishTankEndpoint:  "https://checkurl.phishtank.com/checkurl/",
	}
}

// SetDomainBlacklistLookup connects the configured local domain blocklists to
// the safety report. The second return value indicates whether any domain
// intelligence is loaded, so an empty cache stays Unknown rather than clean.
func (h *SafeHandler) SetDomainBlacklistLookup(lookup func(string) (bool, bool)) {
	h.domainBlacklist = lookup
}

type safeRedirectsContextKey struct{}

func (h *SafeHandler) Type() AnswerType {
	return AnswerTypeSafe
}

func (h *SafeHandler) HandleDirectQuery(ctx context.Context, term string) (*Answer, error) {
	lang := LangFromContext(ctx)
	term = strings.TrimSpace(term)
	if term == "" {
		return nil, fmt.Errorf("URL or domain required")
	}

	targetURL := term
	if !strings.HasPrefix(strings.ToLower(term), "http://") && !strings.HasPrefix(strings.ToLower(term), "https://") {
		targetURL = "https://" + term
	}

	parsedURL, err := url.Parse(targetURL)
	if err != nil || parsedURL.Hostname() == "" || parsedURL.User != nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") {
		return &Answer{
			Type:        AnswerTypeSafe,
			Term:        term,
			Title:       i18n.T(lang, "direct.url_safety_check_title"),
			Description: i18n.T(lang, "direct.invalid_url"),
			Content:     fmt.Sprintf("<p class=\"error\">Invalid URL: %s</p>", escapeHTML(term)),
			Error:       "invalid_url",
		}, nil
	}

	domain := parsedURL.Hostname()

	// Perform safety checks
	checkedURL := parsedURL.Scheme + "://" + parsedURL.Host + "/"
	checks := performSafetyChecks(ctx, h.client, domain, checkedURL)
	if h.domainBlacklist == nil {
		checks = append(checks, safetyCheck{Name: "Known Blacklist", Details: "No local domain blacklist data is available"})
	} else {
		blocked, known := h.domainBlacklist(domain)
		details := "Domain is not present in the loaded local domain blacklists"
		if !known {
			details = "Local domain blacklist data is unavailable"
		} else if blocked {
			details = "Domain is present in a configured local domain blacklist"
		}
		checks = append(checks, safetyCheck{Name: "Known Blacklist", Passed: !blocked, Known: known, Details: details})
	}
	if h.domainBlacklist == nil {
		checks = append(checks, safetyCheck{Name: "Related Malicious Domains", Details: "Related-domain intelligence is unavailable because no local domain blacklist is loaded"})
	} else {
		_, blacklistKnown := h.domainBlacklist(domain)
		related := relatedBlacklistedDomains(domain, func(candidate string) bool {
			blocked, _ := h.domainBlacklist(candidate)
			return blocked
		}, blacklistKnown)
		details := "No one-edit related domain was found in the loaded local domain blacklists"
		if len(related) > 0 {
			details = "Related blacklisted domains: " + strings.Join(related, ", ")
		}
		checks = append(checks, safetyCheck{Name: "Related Malicious Domains", Passed: len(related) == 0, Known: blacklistKnown, Details: details})
	}
	var redirects []string
	localChecksPassed := true
	for _, check := range checks {
		if check.Known && !check.Passed {
			localChecksPassed = false
			break
		}
	}
	if localChecksPassed {
		if err := validatePublicDirectoryHost(ctx, domain); err != nil {
			checks = append(checks, safetyCheck{Name: "Public Address", Passed: false, Known: true, Details: "The domain does not resolve exclusively to public addresses"})
			localChecksPassed = false
		}
	}
	reputationProviders, reputationResponses := 0, 0
	reputationSafe := true
	if localChecksPassed {
		if h.apiKey != "" {
			reputationProviders++
			known, safe := h.checkGoogleSafeBrowsing(ctx, checkedURL)
			if known {
				reputationResponses++
			}
			if !known || !safe {
				reputationSafe = false
			}
			details := "Google Safe Browsing reported no matching threat"
			if !known {
				details = "Google Safe Browsing reputation is unavailable"
			} else if !safe {
				details = "Google Safe Browsing reported a matching threat"
			}
			checks = append(checks, safetyCheck{Name: "Google Safe Browsing", Passed: safe, Known: known, Details: details})
		}
		if h.phishTankAPIKey != "" {
			reputationProviders++
			known, safe, detail := h.checkPhishTank(ctx, checkedURL)
			if known {
				reputationResponses++
			}
			if !known || !safe {
				reputationSafe = false
			}
			checks = append(checks, safetyCheck{Name: "PhishTank", Passed: safe, Known: known, Details: detail})
		}
	}
	reputationKnown := reputationProviders > 0 && reputationResponses == reputationProviders
	if localChecksPassed {
		networkContext, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		networkContext = context.WithValue(networkContext, safeRedirectsContextKey{}, &redirects)
		checks = append(checks, performNetworkSafetyChecks(networkContext, h.client, domain, checkedURL)...)
	}

	allChecksPassed := true
	for _, check := range checks {
		if check.Known && !check.Passed {
			allChecksPassed = false
			break
		}
	}

	var rating string
	if !allChecksPassed {
		rating = i18n.T(lang, "direct.suspicious_rating")
	} else if reputationKnown {
		rating = i18n.T(lang, "direct.safe_rating")
	} else {
		rating = i18n.T(lang, "direct.unknown_rating")
	}

	data := map[string]interface{}{
		"url":                checkedURL,
		"domain":             domain,
		"safe":               nil,
		"reputation_checked": reputationKnown,
		"rating":             rating,
		"checks":             checks,
		"redirects":          redirects,
	}
	if rating == i18n.T(lang, "direct.suspicious_rating") {
		data["safe"] = false
	}

	return &Answer{
		Type:        AnswerTypeSafe,
		Term:        term,
		Title:       fmt.Sprintf("URL Safety: %s", domain),
		Description: rating,
		Content:     formatSafeContent(domain, rating, reputationKnown && reputationSafe && allChecksPassed, checks),
		Source:      "Safety Analyzer",
		Data:        data,
	}, nil
}

func (h *SafeHandler) checkGoogleSafeBrowsing(ctx context.Context, targetURL string) (bool, bool) {
	if h.apiKey == "" || h.client == nil || h.reputationEndpoint == "" {
		return false, false
	}
	endpoint, err := url.Parse(h.reputationEndpoint)
	if err != nil || endpoint.Scheme != "https" || endpoint.Hostname() == "" {
		return false, false
	}
	query := endpoint.Query()
	query.Set("key", h.apiKey)
	endpoint.RawQuery = query.Encode()
	body, err := json.Marshal(map[string]interface{}{
		"client": map[string]string{"clientId": "apimgr-search", "clientVersion": "1.0"},
		"threatInfo": map[string]interface{}{
			"threatTypes":      []string{"MALWARE", "SOCIAL_ENGINEERING", "UNWANTED_SOFTWARE", "POTENTIALLY_HARMFUL_APPLICATION"},
			"platformTypes":    []string{"ANY_PLATFORM"},
			"threatEntryTypes": []string{"URL"},
			"threatEntries":    []map[string]string{{"url": targetURL}},
		},
	})
	if err != nil {
		return false, false
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), strings.NewReader(string(body)))
	if err != nil {
		return false, false
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	client := *h.client
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, err := client.Do(request)
	if err != nil {
		return false, false
	}
	defer closeutil.Close(response.Body)
	if response.StatusCode != http.StatusOK {
		return false, false
	}
	var result struct {
		Matches []json.RawMessage `json:"matches"`
	}
	if decodeBoundedJSON(response.Body, 1<<20, &result) != nil {
		return false, false
	}
	return true, len(result.Matches) == 0
}

func relatedBlacklistedDomains(domain string, lookup func(string) bool, known bool) []string {
	if lookup == nil || !known {
		return nil
	}
	asciiDomain, err := idna.ToASCII(strings.ToLower(strings.TrimSuffix(domain, ".")))
	if err != nil {
		return nil
	}
	registrable, err := publicsuffix.EffectiveTLDPlusOne(asciiDomain)
	if err != nil {
		return nil
	}
	label, suffix, found := strings.Cut(registrable, ".")
	if !found || len(label) < 4 || len(label) > 40 {
		return nil
	}
	const alphabet = "abcdefghijklmnopqrstuvwxyz0123456789"
	candidates := make(map[string]struct{}, len(label)*len(alphabet)*2)
	add := func(candidate string) {
		if candidate == label || candidate == "" || strings.HasPrefix(candidate, "-") || strings.HasSuffix(candidate, "-") {
			return
		}
		candidates[candidate+"."+suffix] = struct{}{}
	}
	for index := 0; index < len(label); index++ {
		add(label[:index] + label[index+1:])
		if index+1 < len(label) {
			add(label[:index] + label[index+1:index+2] + label[index:index+1] + label[index+2:])
		}
		for _, char := range alphabet {
			if byte(char) != label[index] {
				add(label[:index] + string(char) + label[index+1:])
			}
		}
	}
	for index := 0; index <= len(label); index++ {
		for _, char := range alphabet {
			add(label[:index] + string(char) + label[index:])
		}
	}
	orderedCandidates := make([]string, 0, len(candidates))
	for candidate := range candidates {
		orderedCandidates = append(orderedCandidates, candidate)
	}
	sort.Strings(orderedCandidates)
	var matches []string
	for _, candidate := range orderedCandidates {
		if lookup(candidate) {
			matches = append(matches, candidate)
			if len(matches) == 5 {
				break
			}
		}
	}
	return matches
}

func (h *SafeHandler) checkPhishTank(ctx context.Context, targetURL string) (bool, bool, string) {
	if h.phishTankAPIKey == "" || h.client == nil || h.phishTankEndpoint == "" {
		return false, false, "PhishTank reputation is unavailable"
	}
	parsedTarget, err := url.Parse(targetURL)
	if err != nil || (parsedTarget.Scheme != "http" && parsedTarget.Scheme != "https") || parsedTarget.Host == "" || parsedTarget.User != nil {
		return false, false, "PhishTank reputation is unavailable for an invalid URL"
	}
	endpoint, err := url.Parse(h.phishTankEndpoint)
	if err != nil || endpoint.Scheme != "https" || endpoint.Hostname() == "" {
		return false, false, "PhishTank reputation is unavailable"
	}
	form := url.Values{
		"url":     {targetURL},
		"format":  {"json"},
		"app_key": {h.phishTankAPIKey},
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), strings.NewReader(form.Encode()))
	if err != nil {
		return false, false, "PhishTank reputation is unavailable"
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "search/"+version.Version+" (+https://scour.li)")
	client := *h.client
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, err := client.Do(request)
	if err != nil {
		return false, false, "PhishTank reputation is unavailable"
	}
	defer closeutil.Close(response.Body)
	if response.StatusCode != http.StatusOK {
		return false, false, "PhishTank reputation is unavailable"
	}
	var result struct {
		Results struct {
			InDatabase bool            `json:"in_database"`
			Verified   json.RawMessage `json:"verified"`
			Valid      json.RawMessage `json:"valid"`
		} `json:"results"`
	}
	if decodeBoundedJSON(response.Body, 1<<20, &result) != nil {
		return false, false, "PhishTank returned an invalid response"
	}
	if !result.Results.InDatabase {
		return true, true, "PhishTank reported no matching phishing URL"
	}
	if phishTankFlag(result.Results.Verified) && phishTankFlag(result.Results.Valid) {
		return true, false, "PhishTank reported a verified and active phishing URL"
	}
	return false, false, "PhishTank returned an unverified or inactive match"
}

func phishTankFlag(value json.RawMessage) bool {
	var textValue string
	if json.Unmarshal(value, &textValue) == nil {
		return strings.EqualFold(textValue, "y") || strings.EqualFold(textValue, "true") || textValue == "1"
	}
	var boolValue bool
	return json.Unmarshal(value, &boolValue) == nil && boolValue
}

type safetyCheck struct {
	Name    string `json:"name"`
	Passed  bool   `json:"passed"`
	Known   bool   `json:"known"`
	Details string `json:"details"`
}

func performSafetyChecks(ctx context.Context, client *http.Client, domain, targetURL string) []safetyCheck {
	var checks []safetyCheck

	// Check 1: HTTPS
	checks = append(checks, safetyCheck{
		Name:    "HTTPS",
		Passed:  strings.HasPrefix(targetURL, "https://"),
		Known:   true,
		Details: "Input URL uses HTTPS; certificate verification is reported separately",
	})

	// Check 2: Valid domain
	validDomain := !strings.Contains(domain, "--") && !strings.HasPrefix(domain, "-")
	checks = append(checks, safetyCheck{
		Name:    "Valid Domain",
		Passed:  validDomain,
		Known:   true,
		Details: "Domain format is valid",
	})
	phishingSignal, phishingDetails := highRiskPhishingSignal(domain)
	checks = append(checks, safetyCheck{Name: "Phishing Indicators", Passed: !phishingSignal, Known: true, Details: phishingDetails})

	// Check 3: Not a suspicious TLD
	suspiciousTLDs := []string{".tk", ".ml", ".ga", ".cf", ".gq", ".top", ".xyz", ".pw", ".cc"}
	hasSuspiciousTLD := false
	for _, tld := range suspiciousTLDs {
		if strings.HasSuffix(strings.ToLower(domain), tld) {
			hasSuspiciousTLD = true
			break
		}
	}
	checks = append(checks, safetyCheck{
		Name:    "Domain TLD",
		Passed:  !hasSuspiciousTLD,
		Known:   true,
		Details: "TLD is not commonly associated with spam",
	})

	// Check 4: Not excessive subdomains
	parts := strings.Split(domain, ".")
	checks = append(checks, safetyCheck{
		Name:    "Domain Structure",
		Passed:  len(parts) <= 4,
		Known:   true,
		Details: "Not excessive subdomains",
	})

	// Check 5: Domain length
	checks = append(checks, safetyCheck{
		Name:    "Domain Length",
		Passed:  len(domain) < 50,
		Known:   true,
		Details: "Domain length is reasonable",
	})

	return checks
}

func highRiskPhishingSignal(domain string) (bool, string) {
	if net.ParseIP(domain) != nil {
		return true, "The URL host is an IP literal instead of a registered domain"
	}
	unicodeDomain, err := idna.ToUnicode(domain)
	if err != nil {
		return true, "The internationalized domain name could not be decoded"
	}
	for _, label := range strings.Split(unicodeDomain, ".") {
		latin, cyrillic, greek := false, false, false
		for _, character := range label {
			switch {
			case unicode.Is(unicode.Latin, character):
				latin = true
			case unicode.Is(unicode.Cyrillic, character):
				cyrillic = true
			case unicode.Is(unicode.Greek, character):
				greek = true
			}
		}
		if latin && (cyrillic || greek) {
			return true, "A domain label mixes Latin with Cyrillic or Greek characters, a common look-alike pattern"
		}
	}
	return false, "No IP-literal host or mixed Latin/Cyrillic/Greek label was detected"
}

func performNetworkSafetyChecks(ctx context.Context, client *http.Client, domain, targetURL string) []safetyCheck {
	checks := make([]safetyCheck, 0, 3)
	parsed, err := url.Parse(targetURL)
	if err != nil || parsed.Scheme != "https" {
		checks = append(checks, safetyCheck{Name: "TLS Certificate", Details: "HTTPS certificate cannot be checked for a non-HTTPS URL"})
	} else {
		request, requestErr := http.NewRequestWithContext(ctx, http.MethodHead, targetURL, nil)
		if requestErr != nil {
			checks = append(checks, safetyCheck{Name: "TLS Certificate", Details: "Certificate check could not be started"})
		} else {
			request.Header.Set("User-Agent", version.BrowserUserAgent)
			response, requestErr := client.Do(request)
			if requestErr != nil {
				validityFailure := isTLSValidationError(requestErr)
				details := "TLS certificate status could not be determined because the host could not be reached"
				if validityFailure {
					details = "TLS certificate validation failed"
				}
				checks = append(checks, safetyCheck{Name: "TLS Certificate", Passed: !validityFailure, Known: validityFailure, Details: details})
			} else {
				closeutil.Close(response.Body)
				checks = append(checks, safetyCheck{Name: "TLS Certificate", Passed: response.TLS != nil, Known: response.TLS != nil, Details: "The HTTPS connection completed with standard certificate verification"})
			}
		}
	}

	registration, found, registrantPrivate, privacyKnown := fetchDomainRegistrationInfo(ctx, client, domain)
	if found {
		checks = append(checks, safetyCheck{Name: "Domain Age", Passed: true, Known: true, Details: fmt.Sprintf("RDAP registration date %s; domain age %d days", registration.Format("2006-01-02"), int(time.Since(registration).Hours()/24))})
	} else {
		checks = append(checks, safetyCheck{Name: "Domain Age", Details: "RDAP registration date is unavailable"})
	}
	if privacyKnown {
		details := "RDAP exposes registrant contact information"
		if registrantPrivate {
			details = "RDAP explicitly redacts registrant contact information"
		}
		checks = append(checks, safetyCheck{Name: "WHOIS Privacy", Passed: !registrantPrivate, Known: true, Details: details})
	} else {
		checks = append(checks, safetyCheck{Name: "WHOIS Privacy", Details: "RDAP does not provide enough registrant data to determine privacy status"})
	}
	redirects, _ := ctx.Value(safeRedirectsContextKey{}).(*[]string)
	redirectCount := 0
	if redirects != nil {
		redirectCount = len(*redirects)
	}
	checks = append(checks, safetyCheck{Name: "Redirect Chain", Passed: true, Known: true, Details: fmt.Sprintf("Checked the domain root; %d public redirect hops observed", redirectCount)})
	return checks
}

func isTLSValidationError(err error) bool {
	var invalid x509.CertificateInvalidError
	var authority x509.UnknownAuthorityError
	var hostname x509.HostnameError
	var roots x509.SystemRootsError
	return errors.As(err, &invalid) || errors.As(err, &authority) || errors.As(err, &hostname) || errors.As(err, &roots)
}

type rdapEntity struct {
	Roles    []string          `json:"roles"`
	VCard    json.RawMessage   `json:"vcardArray"`
	Redacted []json.RawMessage `json:"redacted"`
	Remarks  []struct {
		Description []string `json:"description"`
	} `json:"remarks"`
}

func fetchDomainRegistrationInfo(ctx context.Context, client *http.Client, domain string) (time.Time, bool, bool, bool) {
	lookupCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	lookupCtx = context.WithValue(lookupCtx, safeRedirectsContextKey{}, nil)
	request, err := http.NewRequestWithContext(lookupCtx, http.MethodGet, "https://rdap.org/domain/"+url.PathEscape(domain), nil)
	if err != nil {
		return time.Time{}, false, false, false
	}
	request.Header.Set("Accept", "application/rdap+json, application/json")
	request.Header.Set("User-Agent", version.BrowserUserAgent)
	response, err := client.Do(request)
	if err != nil {
		return time.Time{}, false, false, false
	}
	defer closeutil.Close(response.Body)
	if response.StatusCode != http.StatusOK {
		return time.Time{}, false, false, false
	}
	var rdap struct {
		Events []struct {
			Action string `json:"eventAction"`
			Date   string `json:"eventDate"`
		} `json:"events"`
		Entities []rdapEntity `json:"entities"`
	}
	if decodeBoundedJSON(response.Body, 1<<20, &rdap) != nil {
		return time.Time{}, false, false, false
	}
	registrantPrivate, privacyKnown := rdapRegistrantPrivacy(rdap.Entities)
	for _, event := range rdap.Events {
		if !strings.EqualFold(event.Action, "registration") && !strings.EqualFold(event.Action, "registered") {
			continue
		}
		for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05.000Z"} {
			if registered, err := time.Parse(layout, event.Date); err == nil && !registered.After(time.Now()) {
				return registered, true, registrantPrivate, privacyKnown
			}
		}
	}
	return time.Time{}, false, registrantPrivate, privacyKnown
}

func rdapRegistrantPrivacy(entities []rdapEntity) (bool, bool) {
	for _, entity := range entities {
		registrant := false
		for _, role := range entity.Roles {
			if strings.EqualFold(role, "registrant") {
				registrant = true
				break
			}
		}
		if !registrant {
			continue
		}
		if len(entity.Redacted) > 0 {
			return true, true
		}
		var vcard []interface{}
		if json.Unmarshal(entity.VCard, &vcard) == nil && len(vcard) > 1 {
			properties, _ := vcard[1].([]interface{})
			for _, propertyValue := range properties {
				property, _ := propertyValue.([]interface{})
				if len(property) < 4 {
					continue
				}
				name, _ := property[0].(string)
				if name != "fn" && name != "org" && name != "email" && name != "adr" {
					continue
				}
				value := fmt.Sprint(property[3])
				lowerValue := strings.ToLower(value)
				if strings.Contains(lowerValue, "redact") || strings.Contains(lowerValue, "privacy") {
					return true, true
				}
				if value != "" && value != "<nil>" {
					return false, true
				}
			}
		}
		for _, remark := range entity.Remarks {
			for _, description := range remark.Description {
				if strings.Contains(strings.ToLower(description), "redact") || strings.Contains(strings.ToLower(description), "privacy") {
					return true, true
				}
			}
		}
	}
	return false, false
}

func formatSafeContent(domain, rating string, safe bool, checks []safetyCheck) string {
	var html strings.Builder
	html.WriteString("<div class=\"safe-content\">")
	fmt.Fprintf(&html, "<h1>URL Safety: %s</h1>", escapeHTML(domain))

	ratingClass := "unknown"
	if rating == "Suspicious" {
		ratingClass = "suspicious"
	} else if safe {
		ratingClass = "safe"
	}
	fmt.Fprintf(&html, "<p class=\"rating %s\">%s</p>", ratingClass, escapeHTML(rating))

	html.WriteString("<h2>Security Checks</h2>")
	html.WriteString("<table class=\"checks-table\">")
	html.WriteString("<tbody>")

	for _, check := range checks {
		status := "✓"
		statusClass := "passed"
		if !check.Known {
			status = "?"
			statusClass = "unknown"
		} else if !check.Passed {
			status = "✗"
			statusClass = "failed"
		}
		fmt.Fprintf(&html, "<tr class=\"%s\"><td>%s</td><td>%s</td><td>%s</td></tr>",
			statusClass, status, escapeHTML(check.Name), escapeHTML(check.Details))
	}

	html.WriteString("</tbody></table>")

	html.WriteString("<p class=\"note\"><strong>Note:</strong> This is a basic check. For comprehensive security analysis, use dedicated security services.</p>")
	html.WriteString("</div>")
	return html.String()
}

// CacheHandler handles cache:{url} queries (web archive lookup)
type CacheHandler struct {
	client     *http.Client
	liveClient *http.Client
}

type archiveSnapshot struct {
	Timestamp   string `json:"timestamp"`
	Original    string `json:"original"`
	URL         string `json:"url"`
	DownloadURL string `json:"download_url,omitempty"`
	Source      string `json:"source"`
	Filename    string `json:"filename,omitempty"`
	Offset      int64  `json:"offset,omitempty"`
	Length      int64  `json:"length,omitempty"`
}

// NewCacheHandler creates a new web cache/archive handler
func NewCacheHandler() *CacheHandler {
	return &CacheHandler{
		client: &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		}},
		liveClient: newPublicHTTPClient(8 * time.Second),
	}
}

type archiveSnapshotContextKey struct{}

type archiveSnapshotIndexContextKey struct{}

type archiveSnapshotIDContextKey struct{}

func WithArchiveSnapshot(ctx context.Context, timestamp string) context.Context {
	return context.WithValue(ctx, archiveSnapshotContextKey{}, timestamp)
}

func WithArchiveSnapshotIndex(ctx context.Context, index string) context.Context {
	return context.WithValue(ctx, archiveSnapshotIndexContextKey{}, index)
}

func WithArchiveSnapshotID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, archiveSnapshotIDContextKey{}, id)
}

func archiveSnapshotID(snapshot archiveSnapshot) string {
	identity := strings.Join([]string{snapshot.Source, snapshot.Timestamp, snapshot.Original, snapshot.URL, snapshot.Filename, strconv.FormatInt(snapshot.Offset, 10), strconv.FormatInt(snapshot.Length, 10)}, "\x00")
	digest := sha256.Sum256([]byte(identity))
	return base64.RawURLEncoding.EncodeToString(digest[:])
}

func (h *CacheHandler) Type() AnswerType {
	return AnswerTypeCache
}

func (h *CacheHandler) HandleDirectQuery(ctx context.Context, term string) (*Answer, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	term = strings.TrimSpace(term)
	if term == "" {
		return nil, fmt.Errorf("URL required")
	}

	targetURL := term
	if !strings.HasPrefix(strings.ToLower(term), "http://") && !strings.HasPrefix(strings.ToLower(term), "https://") {
		targetURL = "https://" + term
	}
	parsedTarget, err := validatePublicHTTPURL(targetURL)
	if err != nil {
		return nil, fmt.Errorf("invalid archive URL: %w", err)
	}
	if parsedTarget.Path == "" {
		parsedTarget.Path = "/"
	}
	targetURL = parsedTarget.String()

	// Generate links to various archive services
	archives := []struct {
		name string
		url  string
	}{
		{"Wayback Machine", fmt.Sprintf("https://web.archive.org/web/*/%s", url.QueryEscape(targetURL))},
		{"Wayback Machine (Latest)", fmt.Sprintf("https://web.archive.org/web/%s", url.QueryEscape(targetURL))},
		{"Archive.today", fmt.Sprintf("https://archive.today/%s", url.QueryEscape(targetURL))},
		{"Common Crawl Index", "https://index.commoncrawl.org/?url=" + url.QueryEscape(targetURL)},
	}

	lookups := []func(context.Context, string) []archiveSnapshot{
		h.lookupWayback,
		h.lookupArchiveToday,
		h.lookupCommonCrawl,
	}
	results := make(chan []archiveSnapshot, len(lookups))
	for _, lookup := range lookups {
		go func(lookup func(context.Context, string) []archiveSnapshot) {
			results <- lookup(ctx, targetURL)
		}(lookup)
	}
	var snapshots []archiveSnapshot
	for range lookups {
		snapshots = append(snapshots, (<-results)...)
	}
	sort.Slice(snapshots, func(i, j int) bool {
		if snapshots[i].Timestamp != snapshots[j].Timestamp {
			return snapshots[i].Timestamp > snapshots[j].Timestamp
		}
		if snapshots[i].Source != snapshots[j].Source {
			return snapshots[i].Source < snapshots[j].Source
		}
		if snapshots[i].Original != snapshots[j].Original {
			return snapshots[i].Original < snapshots[j].Original
		}
		if snapshots[i].URL != snapshots[j].URL {
			return snapshots[i].URL < snapshots[j].URL
		}
		if snapshots[i].Filename != snapshots[j].Filename {
			return snapshots[i].Filename < snapshots[j].Filename
		}
		if snapshots[i].Offset != snapshots[j].Offset {
			return snapshots[i].Offset < snapshots[j].Offset
		}
		return snapshots[i].Length < snapshots[j].Length
	})
	if len(snapshots) > 10 {
		snapshots = snapshots[:10]
	}
	for index := range snapshots {
		if snapshots[index].Source == "Common Crawl" {
			snapshots[index].DownloadURL = commonCrawlDownloadURL(snapshots[index])
		}
	}
	requestedSnapshot, _ := ctx.Value(archiveSnapshotContextKey{}).(string)
	requestedSnapshotID, _ := ctx.Value(archiveSnapshotIDContextKey{}).(string)
	selectedIndex := -1
	if rawIndex, ok := ctx.Value(archiveSnapshotIndexContextKey{}).(string); ok && rawIndex != "" {
		index, err := strconv.Atoi(rawIndex)
		if err != nil || index < 0 || index >= len(snapshots) {
			requestedSnapshot = "__invalid_snapshot_index__"
		} else {
			selectedIndex = index
			requestedSnapshotID = archiveSnapshotID(snapshots[index])
		}
	}
	archivedContent := ""
	selected := ""
	selectedID := ""
	for index, snapshot := range snapshots {
		if requestedSnapshotID != "" && archiveSnapshotID(snapshot) != requestedSnapshotID {
			continue
		}
		if requestedSnapshotID == "" && requestedSnapshot != "" && snapshot.Timestamp != requestedSnapshot {
			continue
		}
		archivedContent = h.fetchArchiveSnapshot(ctx, snapshot)
		if archivedContent != "" {
			selected = snapshot.Timestamp
			selectedID = archiveSnapshotID(snapshot)
			selectedIndex = index
			break
		}
	}
	liveContent := h.fetchLivePreview(ctx, targetURL)
	data := map[string]interface{}{"url": targetURL, "archives": archives, "snapshots": snapshots, "snapshot_count": len(snapshots), "selected_snapshot": selected, "selected_snapshot_id": selectedID, "selected_snapshot_index": selectedIndex, "archived_content": archivedContent, "live_content": liveContent}
	return &Answer{
		Type:        AnswerTypeCache,
		Term:        term,
		Title:       fmt.Sprintf("Cached: %s", term),
		Description: fmt.Sprintf("%d archived snapshots found", len(snapshots)),
		Content:     formatCacheContent(targetURL, archives, snapshots, selected, selectedID, selectedIndex, archivedContent, liveContent),
		Source:      "Web Archives",
		Data:        data,
	}, nil
}

func (h *CacheHandler) fetchLivePreview(ctx context.Context, targetURL string) string {
	if h.liveClient == nil {
		return ""
	}
	parsed, err := validatePublicHTTPURL(targetURL)
	if err != nil || validatePublicDirectoryHost(ctx, parsed.Hostname()) != nil {
		return ""
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return ""
	}
	request.Header.Set("User-Agent", version.BrowserUserAgent)
	response, err := h.liveClient.Do(request)
	if err != nil {
		return ""
	}
	defer closeutil.Close(response.Body)
	if response.StatusCode != http.StatusOK {
		return ""
	}
	contentType := strings.ToLower(response.Header.Get("Content-Type"))
	if !strings.HasPrefix(contentType, "text/html") && !strings.HasPrefix(contentType, "text/plain") {
		return ""
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	if err != nil || len(body) > 1<<20 {
		return ""
	}
	if strings.HasPrefix(contentType, "text/html") {
		return sanitizeManHTML(string(body))
	}
	return "<pre><code>" + escapeHTML(string(body)) + "</code></pre>"
}

var ErrInvalidArchiveTodaySnapshot = errors.New("invalid Archive.today snapshot")

func archiveTodaySnapshotMatches(original *url.URL, timestamp string, snapshot *url.URL) bool {
	if original == nil || snapshot == nil || !regexp.MustCompile(`^[0-9]{14}$`).MatchString(timestamp) ||
		snapshot.Scheme != "https" || !validArchiveTodayHost(snapshot.Hostname()) || snapshot.User != nil || snapshot.Fragment != "" {
		return false
	}
	if _, err := time.Parse("20060102150405", timestamp); err != nil {
		return false
	}
	if original.User != nil || (original.Scheme != "http" && original.Scheme != "https") {
		return false
	}
	path := snapshot.EscapedPath()
	marker := "/" + timestamp + "/"
	if !strings.HasPrefix(path, marker) {
		return false
	}
	captured, err := url.Parse(strings.TrimPrefix(path, marker))
	return err == nil && captured.User == nil && strings.EqualFold(captured.Scheme, original.Scheme) &&
		strings.EqualFold(captured.Host, original.Host) && captured.EscapedPath() == original.EscapedPath() &&
		captured.RawQuery == "" && snapshot.RawQuery == original.RawQuery
}

func (h *CacheHandler) DownloadArchiveTodaySnapshot(ctx context.Context, originalURL, timestamp, snapshotURL string) ([]byte, string, error) {
	original, err := validatePublicHTTPURL(originalURL)
	if err != nil {
		return nil, "", ErrInvalidArchiveTodaySnapshot
	}
	if original.Path == "" {
		original.Path = "/"
	}
	original.Fragment = ""
	snapshot, err := url.Parse(snapshotURL)
	if err != nil || !archiveTodaySnapshotMatches(original, timestamp, snapshot) {
		return nil, "", ErrInvalidArchiveTodaySnapshot
	}
	if h.liveClient == nil || h.liveClient.Transport == nil {
		return nil, "", fmt.Errorf("public archive transport is unavailable")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, snapshot.String(), nil)
	if err != nil {
		return nil, "", ErrInvalidArchiveTodaySnapshot
	}
	request.Header.Set("User-Agent", version.BrowserUserAgent)
	client := &http.Client{Transport: h.liveClient.Transport, Timeout: 15 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	response, err := client.Do(request)
	if err != nil {
		return nil, "", err
	}
	defer closeutil.Close(response.Body)
	if response.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("archive.today returned HTTP %d", response.StatusCode)
	}
	contentType, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil || (contentType != "text/html" && contentType != "text/plain") {
		return nil, "", fmt.Errorf("archive.today response is not a downloadable text page")
	}
	const maximumBytes = 5 << 20
	body, err := io.ReadAll(io.LimitReader(response.Body, maximumBytes+1))
	if err != nil || len(body) > maximumBytes {
		return nil, "", fmt.Errorf("archive.today response exceeds the download limit")
	}
	return body, contentType, nil
}

func archiveTodayDownloadURL(snapshot archiveSnapshot) string {
	query := url.Values{
		"url":          {snapshot.Original},
		"timestamp":    {snapshot.Timestamp},
		"snapshot_url": {snapshot.URL},
	}
	return "/direct/cache-download-archive-today?" + query.Encode()
}

func (h *CacheHandler) fetchArchivePreview(ctx context.Context, target string) string {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return ""
	}
	request.Header.Set("User-Agent", version.BrowserUserAgent)
	response, err := h.client.Do(request)
	if err != nil {
		return ""
	}
	defer closeutil.Close(response.Body)
	if response.StatusCode != http.StatusOK {
		return ""
	}
	contentType := strings.ToLower(response.Header.Get("Content-Type"))
	if !strings.HasPrefix(contentType, "text/html") && !strings.HasPrefix(contentType, "text/plain") {
		return ""
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	if err != nil || len(body) > 1<<20 {
		return ""
	}
	if strings.HasPrefix(contentType, "text/html") {
		return sanitizeManHTML(string(body))
	}
	return "<pre><code>" + escapeHTML(string(body)) + "</code></pre>"
}

var archiveMementoRelationPattern = regexp.MustCompile(`(?:^|;)\s*rel="?memento"?(?:;|$)`)
var archiveMementoDatePattern = regexp.MustCompile(`(?:^|;)\s*datetime="([^"]+)"`)

func validArchiveTodayHost(host string) bool {
	switch strings.ToLower(strings.TrimSuffix(host, ".")) {
	case "archive.today", "archive.is", "archive.md", "archive.ph", "archive.vn", "archive.fo":
		return true
	default:
		return false
	}
}

func (h *CacheHandler) lookupArchiveToday(ctx context.Context, targetURL string) []archiveSnapshot {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://archive.today/timemap/"+targetURL, nil)
	if err != nil {
		return nil
	}
	request.Header.Set("Accept", "application/link-format")
	request.Header.Set("User-Agent", version.BrowserUserAgent)
	response, err := h.client.Do(request)
	if err != nil {
		return nil
	}
	defer closeutil.Close(response.Body)
	if response.StatusCode != http.StatusOK {
		return nil
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	if err != nil || len(body) > 1<<20 {
		return nil
	}
	original, err := url.Parse(targetURL)
	if err != nil {
		return nil
	}
	var snapshots []archiveSnapshot
	remaining := string(body)
	for len(remaining) > 0 {
		start := strings.IndexByte(remaining, '<')
		if start < 0 {
			break
		}
		remaining = remaining[start+1:]
		end := strings.IndexByte(remaining, '>')
		if end < 0 {
			break
		}
		linkText := remaining[:end]
		remaining = remaining[end+1:]
		next := strings.IndexByte(remaining, '<')
		parameters := remaining
		if next >= 0 {
			parameters, remaining = remaining[:next], remaining[next:]
		} else {
			remaining = ""
		}
		link, parseErr := url.Parse(linkText)
		if parseErr != nil {
			continue
		}
		if !archiveMementoRelationPattern.MatchString(parameters) {
			continue
		}
		datetime := archiveMementoDatePattern.FindStringSubmatch(parameters)
		if len(datetime) != 2 {
			continue
		}
		parsedTime, timeErr := time.Parse(time.RFC1123, datetime[1])
		if timeErr != nil {
			parsedTime, timeErr = time.Parse(time.RFC1123Z, datetime[1])
		}
		if timeErr != nil {
			continue
		}
		timestamp := parsedTime.UTC().Format("20060102150405")
		if !archiveTodaySnapshotMatches(original, timestamp, link) {
			continue
		}
		link.Scheme = "https"
		link.Fragment = ""
		snapshots = append(snapshots, archiveSnapshot{Timestamp: timestamp, Original: targetURL, URL: link.String(), Source: "Archive.today"})
		if len(snapshots) == 5 {
			break
		}
	}
	return snapshots
}

func (h *CacheHandler) fetchSnapshot(ctx context.Context, snapshot archiveSnapshot) string {
	if !regexp.MustCompile(`^[0-9]{14}$`).MatchString(snapshot.Timestamp) {
		return ""
	}
	if snapshot.Source == "Archive.today" {
		parsed, err := url.Parse(snapshot.URL)
		if err != nil || parsed.Scheme != "https" || !validArchiveTodayHost(parsed.Hostname()) || parsed.User != nil {
			return ""
		}
		return h.fetchArchivePreview(ctx, parsed.String())
	}
	original, err := url.Parse(snapshot.Original)
	if err != nil || (original.Scheme != "http" && original.Scheme != "https") || original.Host == "" || original.User != nil {
		return ""
	}
	archiveURL := "https://web.archive.org/web/" + snapshot.Timestamp + "id_/" + snapshot.Original
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, archiveURL, nil)
	if err != nil {
		return ""
	}
	request.Header.Set("User-Agent", version.BrowserUserAgent)
	response, err := h.client.Do(request)
	if err != nil {
		return ""
	}
	defer closeutil.Close(response.Body)
	if response.StatusCode != http.StatusOK {
		return ""
	}
	contentType := strings.ToLower(response.Header.Get("Content-Type"))
	if !strings.HasPrefix(contentType, "text/html") && !strings.HasPrefix(contentType, "text/plain") {
		return ""
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	if err != nil || len(body) > 1<<20 {
		return ""
	}
	if strings.HasPrefix(contentType, "text/html") {
		return sanitizeManHTML(string(body))
	}
	return "<pre><code>" + escapeHTML(string(body)) + "</code></pre>"
}

func (h *CacheHandler) fetchArchiveSnapshot(ctx context.Context, snapshot archiveSnapshot) string {
	if snapshot.Source != "Common Crawl" {
		return h.fetchSnapshot(ctx, snapshot)
	}
	body, contentType, err := h.fetchCommonCrawlRecord(ctx, snapshot, 1<<20)
	if err != nil {
		return ""
	}
	if strings.HasPrefix(contentType, "text/html") {
		return sanitizeManHTML(string(body))
	}
	return "<pre><code>" + escapeHTML(string(body)) + "</code></pre>"
}

func (h *CacheHandler) DownloadCommonCrawlRecord(ctx context.Context, original, filename string, offset, length int64) ([]byte, string, error) {
	parsed, err := url.Parse(original)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil {
		return nil, "", fmt.Errorf("invalid archived URL")
	}
	snapshot := archiveSnapshot{Source: "Common Crawl", Original: parsed.String(), Filename: filename, Offset: offset, Length: length}
	return h.fetchCommonCrawlRecord(ctx, snapshot, 20<<20)
}

func (h *CacheHandler) fetchCommonCrawlRecord(ctx context.Context, snapshot archiveSnapshot, maximumBytes int64) ([]byte, string, error) {
	if h.client == nil || !validCommonCrawlFilename(snapshot.Filename) || snapshot.Offset < 0 || snapshot.Length <= 0 || snapshot.Length > 20<<20 || snapshot.Offset > int64(^uint64(0)>>1)-snapshot.Length+1 {
		return nil, "", fmt.Errorf("invalid Common Crawl record location")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://data.commoncrawl.org/"+snapshot.Filename, nil)
	if err != nil {
		return nil, "", err
	}
	request.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", snapshot.Offset, snapshot.Offset+snapshot.Length-1))
	request.Header.Set("User-Agent", version.BrowserUserAgent)
	response, err := h.client.Do(request)
	if err != nil {
		return nil, "", err
	}
	defer closeutil.Close(response.Body)
	if response.StatusCode != http.StatusPartialContent {
		return nil, "", fmt.Errorf("common crawl returned HTTP %d", response.StatusCode)
	}
	lastByte := snapshot.Offset + snapshot.Length - 1
	expectedRange := fmt.Sprintf("bytes %d-%d/", snapshot.Offset, lastByte)
	contentRange := strings.TrimSpace(response.Header.Get("Content-Range"))
	if !strings.HasPrefix(contentRange, expectedRange) {
		return nil, "", fmt.Errorf("common crawl returned a mismatched byte range")
	}
	totalBytes, rangeErr := strconv.ParseInt(strings.TrimPrefix(contentRange, expectedRange), 10, 64)
	if rangeErr != nil || totalBytes <= lastByte {
		return nil, "", fmt.Errorf("common crawl returned an invalid content range")
	}
	gzipReader, err := gzip.NewReader(io.LimitReader(response.Body, snapshot.Length+1))
	if err != nil {
		return nil, "", fmt.Errorf("invalid Common Crawl WARC record: %w", err)
	}
	defer closeutil.Close(gzipReader)
	warc := bufio.NewReader(io.LimitReader(gzipReader, maximumBytes+64<<10))
	line, err := warc.ReadString('\n')
	if err != nil || !strings.HasPrefix(strings.TrimSpace(line), "WARC/") {
		return nil, "", fmt.Errorf("invalid WARC header")
	}
	warcType := ""
	warcTarget := ""
	var headerBytes int
	for {
		line, err = warc.ReadString('\n')
		headerBytes += len(line)
		if err != nil || headerBytes > 64<<10 {
			return nil, "", fmt.Errorf("invalid WARC headers")
		}
		if strings.TrimSpace(line) == "" {
			break
		}
		key, value, found := strings.Cut(strings.TrimSpace(line), ":")
		if !found {
			return nil, "", fmt.Errorf("invalid WARC header field")
		}
		switch strings.ToLower(key) {
		case "warc-type":
			warcType = strings.TrimSpace(value)
		case "warc-target-uri":
			warcTarget = strings.TrimSpace(value)
		}
	}
	if !strings.EqualFold(warcType, "response") {
		return nil, "", fmt.Errorf("WARC record is not an HTTP response")
	}
	if snapshot.Original != "" && warcTarget != snapshot.Original {
		return nil, "", fmt.Errorf("WARC target does not match the indexed URL")
	}
	httpResponse, err := http.ReadResponse(warc, nil)
	if err != nil {
		return nil, "", fmt.Errorf("invalid archived HTTP response: %w", err)
	}
	defer closeutil.Close(httpResponse.Body)
	contentType := httpResponse.Header.Get("Content-Type")
	contentTypeLower := strings.ToLower(contentType)
	if !strings.HasPrefix(contentTypeLower, "text/html") && !strings.HasPrefix(contentTypeLower, "text/plain") {
		return nil, "", fmt.Errorf("archived content type is not downloadable")
	}
	var bodyReader io.Reader = httpResponse.Body
	switch strings.ToLower(strings.TrimSpace(httpResponse.Header.Get("Content-Encoding"))) {
	case "", "identity":
	case "gzip":
		decoded, decodeErr := gzip.NewReader(bodyReader)
		if decodeErr != nil {
			return nil, "", fmt.Errorf("invalid compressed archived response: %w", decodeErr)
		}
		defer closeutil.Close(decoded)
		bodyReader = decoded
	default:
		return nil, "", fmt.Errorf("archived content encoding is unsupported")
	}
	body, err := io.ReadAll(io.LimitReader(bodyReader, maximumBytes+1))
	if err != nil || int64(len(body)) > maximumBytes {
		return nil, "", fmt.Errorf("archived response exceeds the download limit")
	}
	return body, contentType, nil
}

func validCommonCrawlFilename(filename string) bool {
	if !strings.HasPrefix(filename, "crawl-data/") {
		return false
	}
	for _, character := range filename {
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') || character == '/' || character == '.' || character == '_' || character == '-' {
			continue
		}
		return false
	}
	components := strings.Split(filename, "/")
	if len(components) < 3 || components[0] != "crawl-data" {
		return false
	}
	for _, component := range components {
		if component == "" || component == "." || component == ".." {
			return false
		}
	}
	return true
}

func commonCrawlDownloadURL(snapshot archiveSnapshot) string {
	query := url.Values{
		"url":      {snapshot.Original},
		"filename": {snapshot.Filename},
		"offset":   {strconv.FormatInt(snapshot.Offset, 10)},
		"length":   {strconv.FormatInt(snapshot.Length, 10)},
	}
	return "/direct/cache-download?" + query.Encode()
}

func (h *CacheHandler) lookupWayback(ctx context.Context, targetURL string) []archiveSnapshot {
	query := url.Values{"url": []string{targetURL}, "output": []string{"json"}, "filter": []string{"statuscode:200"}, "fl": []string{"timestamp,original"}, "limit": []string{"5"}, "sort": []string{"descending"}}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://web.archive.org/cdx/search/cdx?"+query.Encode(), nil)
	if err != nil {
		return nil
	}
	request.Header.Set("User-Agent", version.BrowserUserAgent)
	response, err := h.client.Do(request)
	if err != nil {
		return nil
	}
	defer closeutil.Close(response.Body)
	if response.StatusCode != http.StatusOK {
		return nil
	}
	var rows [][]string
	if decodeBoundedJSON(response.Body, 1<<20, &rows) != nil || len(rows) < 2 {
		return nil
	}
	snapshots := make([]archiveSnapshot, 0, len(rows)-1)
	for _, row := range rows[1:] {
		if len(row) < 2 || !regexp.MustCompile(`^[0-9]{14}$`).MatchString(row[0]) || row[1] != targetURL {
			continue
		}
		snapshots = append(snapshots, archiveSnapshot{Timestamp: row[0], Original: row[1], URL: "https://web.archive.org/web/" + row[0] + "/" + row[1], Source: "Wayback Machine"})
	}
	return snapshots
}

func (h *CacheHandler) lookupCommonCrawl(ctx context.Context, targetURL string) []archiveSnapshot {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://index.commoncrawl.org/collinfo.json", nil)
	if err != nil {
		return nil
	}
	request.Header.Set("User-Agent", version.BrowserUserAgent)
	response, err := h.client.Do(request)
	if err != nil {
		return nil
	}
	defer closeutil.Close(response.Body)
	if response.StatusCode != http.StatusOK {
		return nil
	}
	var collections []struct {
		ID string `json:"id"`
	}
	if decodeBoundedJSON(response.Body, 1<<20, &collections) != nil || len(collections) == 0 || !regexp.MustCompile(`^CC-MAIN-[0-9]{4}-[0-9]{2}$`).MatchString(collections[0].ID) {
		return nil
	}
	query := url.Values{"url": []string{targetURL}, "output": []string{"json"}, "filter": []string{"status:200", "mime-detected:text/html"}, "limit": []string{"5"}}
	indexURL := "https://index.commoncrawl.org/" + collections[0].ID + "-index?" + query.Encode()
	request, err = http.NewRequestWithContext(ctx, http.MethodGet, indexURL, nil)
	if err != nil {
		return nil
	}
	request.Header.Set("User-Agent", version.BrowserUserAgent)
	response, err = h.client.Do(request)
	if err != nil {
		return nil
	}
	defer closeutil.Close(response.Body)
	if response.StatusCode != http.StatusOK {
		return nil
	}
	type indexEntry struct {
		Timestamp string `json:"timestamp"`
		URL       string `json:"url"`
		Filename  string `json:"filename"`
		Offset    string `json:"offset"`
		Length    string `json:"length"`
		Status    string `json:"status"`
	}
	var snapshots []archiveSnapshot
	scanner := bufio.NewScanner(io.LimitReader(response.Body, 1<<20))
	scanner.Buffer(make([]byte, 4096), 1<<20)
	for scanner.Scan() && len(snapshots) < 5 {
		var entry indexEntry
		if json.Unmarshal(scanner.Bytes(), &entry) != nil || entry.Status != "200" || !regexp.MustCompile(`^[0-9]{14}$`).MatchString(entry.Timestamp) || !strings.HasPrefix(entry.Filename, "crawl-data/") || strings.Contains(entry.Filename, "..") {
			continue
		}
		entryURL, urlErr := validatePublicHTTPURL(entry.URL)
		target, targetErr := url.Parse(targetURL)
		if urlErr != nil || targetErr != nil || entryURL.String() != target.String() {
			continue
		}
		offset, offsetErr := strconv.ParseInt(entry.Offset, 10, 64)
		length, lengthErr := strconv.ParseInt(entry.Length, 10, 64)
		if offsetErr != nil || lengthErr != nil || offset < 0 || length <= 0 || length > 20<<20 {
			continue
		}
		metadataURL := "https://index.commoncrawl.org/" + collections[0].ID + "-index?" + url.Values{"url": []string{entryURL.String()}, "output": []string{"json"}}.Encode()
		snapshots = append(snapshots, archiveSnapshot{Timestamp: entry.Timestamp, Original: entryURL.String(), URL: metadataURL, Source: "Common Crawl", Filename: entry.Filename, Offset: offset, Length: length})
	}
	return snapshots
}

func formatCacheContent(targetURL string, archives []struct {
	name string
	url  string
}, snapshots []archiveSnapshot, selected, selectedID string, selectedIndex int, archivedContent, liveContent string) string {
	var html strings.Builder
	html.WriteString("<div class=\"cache-content\">")
	html.WriteString("<h1>Web Cache/Archive</h1>")
	fmt.Fprintf(&html, "<p>Looking for cached version of: <code>%s</code></p>", escapeHTML(targetURL))

	html.WriteString("<h2>Archive Services</h2>")
	html.WriteString("<ul class=\"archive-links\">")

	for _, a := range archives {
		fmt.Fprintf(&html, "<li><a href=\"%s\" target=\"_blank\" rel=\"noopener\">%s</a></li>",
			escapeHTML(a.url), escapeHTML(a.name))
	}

	html.WriteString("</ul>")
	if len(snapshots) > 0 {
		if selectedIndex < 0 || selectedIndex >= len(snapshots) {
			selectedIndex = 0
		}
		selectedLabel := snapshots[0].Timestamp
		if parsed, err := time.Parse("20060102150405", selectedLabel); err == nil {
			selectedLabel = parsed.UTC().Format(time.RFC3339)
		}
		if selectedID != "" && selectedIndex < len(snapshots) {
			selectedLabel = snapshots[selectedIndex].Timestamp
			if parsed, err := time.Parse("20060102150405", selectedLabel); err == nil {
				selectedLabel = parsed.UTC().Format(time.RFC3339)
			}
		}
		html.WriteString("<h2>Archived snapshots</h2><form method=\"get\" class=\"archive-timeline\"><label for=\"archive-timeline\">Choose an archived date</label><select id=\"archive-timeline\" name=\"snapshot_id\">")
		for _, snapshot := range snapshots {
			date := snapshot.Timestamp
			if parsed, err := time.Parse("20060102150405", snapshot.Timestamp); err == nil {
				date = parsed.UTC().Format(time.RFC3339)
			}
			selectedAttribute := ""
			if archiveSnapshotID(snapshot) == selectedID && selectedID != "" {
				selectedAttribute = " selected"
			}
			fmt.Fprintf(&html, "<option value=\"%s\"%s>%s · %s</option>", archiveSnapshotID(snapshot), selectedAttribute, escapeHTML(date), escapeHTML(snapshot.Source))
		}
		fmt.Fprintf(&html, "</select><label for=\"archive-snapshot-range\">Timeline position</label><input id=\"archive-snapshot-range\" type=\"range\" min=\"0\" max=\"%d\" value=\"%d\" aria-valuetext=\"%s\"><output for=\"archive-snapshot-range\">%s</output><button type=\"submit\">View archived snapshot</button></form>", len(snapshots)-1, selectedIndex, escapeHTML(selectedLabel), escapeHTML(selectedLabel))
		for _, snapshot := range snapshots {
			date := snapshot.Timestamp
			if parsed, err := time.Parse("20060102150405", snapshot.Timestamp); err == nil {
				date = parsed.UTC().Format(time.RFC3339)
			}
			archiveURL := snapshot.URL
			switch snapshot.Source {
			case "Wayback Machine":
				archiveURL = "https://web.archive.org/web/" + snapshot.Timestamp + "/" + snapshot.Original
				downloadURL := "https://web.archive.org/web/" + snapshot.Timestamp + "id_/" + snapshot.Original
				fmt.Fprintf(&html, "<p><strong>%s</strong>: <a href=\"%s\" rel=\"noopener\">View %s</a> · <a href=\"%s\" rel=\"noopener\">Download archived response</a></p>", escapeHTML(snapshot.Source), escapeHTML(archiveURL), escapeHTML(date), escapeHTML(downloadURL))
			case "Archive.today":
				downloadURL := archiveTodayDownloadURL(snapshot)
				fmt.Fprintf(&html, "<p><strong>%s</strong>: <a href=\"%s\" rel=\"noopener\">View %s</a> · <a href=\"%s\">Download archived response</a></p>", escapeHTML(snapshot.Source), escapeHTML(archiveURL), escapeHTML(date), escapeHTML(downloadURL))
			default:
				downloadURL := snapshot.DownloadURL
				if downloadURL == "" {
					downloadURL = commonCrawlDownloadURL(snapshot)
				}
				fmt.Fprintf(&html, "<p><strong>%s</strong>: <a href=\"%s\" rel=\"noopener\">View capture metadata for %s</a> · <a href=\"%s\">Download archived response</a></p>", escapeHTML(snapshot.Source), escapeHTML(archiveURL), escapeHTML(date), escapeHTML(downloadURL))
			}
		}
	} else {
		html.WriteString("<p>No archived snapshots were found by the available archive services.</p>")
	}
	if archivedContent != "" {
		html.WriteString("<details class=\"archived-page-preview\"><summary>View archived page preview</summary><div class=\"archived-page-content\">")
		html.WriteString(archivedContent)
		html.WriteString("</div></details>")
	}
	if liveContent != "" && archivedContent != "" {
		html.WriteString("<h2>Live and archived comparison</h2><div class=\"archive-comparison\"><section><h3>Live page</h3>")
		html.WriteString(liveContent)
		html.WriteString("</section><section><h3>Archived page</h3>")
		html.WriteString(archivedContent)
		html.WriteString("</section></div>")
	} else if liveContent == "" {
		html.WriteString("<p class=\"live-preview-unavailable\">Live preview unavailable; the destination could not be safely fetched as a public HTML or text page.</p>")
	}

	html.WriteString("<h2>Save to Archive</h2>")
	fmt.Fprintf(&html, "<p><a href=\"https://web.archive.org/save/%s\" target=\"_blank\" rel=\"noopener\">Save current page to Wayback Machine</a></p>",
		url.QueryEscape(targetURL))

	html.WriteString("</div>")
	return html.String()
}
