package direct

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
)

// RobotsHandler handles robots:{domain} queries
type RobotsHandler struct {
	client *http.Client
}

// NewRobotsHandler creates a new robots.txt handler
func NewRobotsHandler() *RobotsHandler {
	return &RobotsHandler{
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

func (h *RobotsHandler) Type() AnswerType {
	return AnswerTypeRobots
}

func (h *RobotsHandler) Handle(ctx context.Context, term string) (*Answer, error) {
	term = strings.TrimSpace(term)
	if term == "" {
		return nil, fmt.Errorf("domain required")
	}

	// Build robots.txt URL
	robotsURL := term
	if !strings.HasPrefix(term, "http://") && !strings.HasPrefix(term, "https://") {
		robotsURL = "https://" + term
	}
	if !strings.HasSuffix(robotsURL, "/robots.txt") {
		robotsURL = strings.TrimSuffix(robotsURL, "/") + "/robots.txt"
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
			Description: "Failed to fetch",
			Content:     fmt.Sprintf("<p class=\"error\">Failed to fetch robots.txt: %s</p>", escapeHTML(err.Error())),
			Error:       "fetch_failed",
		}, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return &Answer{
			Type:        AnswerTypeRobots,
			Term:        term,
			Title:       fmt.Sprintf("robots.txt: %s", term),
			Description: "Not found",
			Content:     "<p>No robots.txt file found. This means all user-agents are allowed to crawl all paths.</p>",
			Error:       "not_found",
		}, nil
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 100*1024)) // Limit to 100KB
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
		Description: "Robots exclusion protocol file",
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
	html.WriteString(fmt.Sprintf("<h1>robots.txt: %s</h1>", escapeHTML(domain)))

	// Analysis summary
	html.WriteString("<h2>Analysis</h2>")

	if ua, ok := analysis["userAgents"].([]string); ok && len(ua) > 0 {
		html.WriteString("<h3>User-Agents</h3><ul>")
		for _, agent := range ua {
			html.WriteString(fmt.Sprintf("<li><code>%s</code></li>", escapeHTML(agent)))
		}
		html.WriteString("</ul>")
	}

	if disallowed, ok := analysis["disallowed"].([]string); ok && len(disallowed) > 0 {
		html.WriteString("<h3>Disallowed Paths</h3><ul>")
		for _, path := range disallowed {
			html.WriteString(fmt.Sprintf("<li><code>%s</code></li>", escapeHTML(path)))
		}
		html.WriteString("</ul>")
	}

	if sitemaps, ok := analysis["sitemaps"].([]string); ok && len(sitemaps) > 0 {
		html.WriteString("<h3>Sitemaps</h3><ul>")
		for _, sitemap := range sitemaps {
			html.WriteString(fmt.Sprintf("<li><a href=\"%s\">%s</a></li>", escapeHTML(sitemap), escapeHTML(sitemap)))
		}
		html.WriteString("</ul>")
	}

	// Raw content
	html.WriteString("<h2>Raw Content</h2>")
	html.WriteString(fmt.Sprintf("<pre class=\"robots-raw\"><code>%s</code></pre>", escapeHTML(content)))

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
		client: &http.Client{Timeout: 15 * time.Second},
	}
}

func (h *SitemapHandler) Type() AnswerType {
	return AnswerTypeSitemap
}

func (h *SitemapHandler) Handle(ctx context.Context, term string) (*Answer, error) {
	term = strings.TrimSpace(term)
	if term == "" {
		return nil, fmt.Errorf("domain required")
	}

	// Build sitemap URL
	sitemapURL := term
	if !strings.HasPrefix(term, "http://") && !strings.HasPrefix(term, "https://") {
		sitemapURL = "https://" + term
	}
	if !strings.Contains(sitemapURL, "sitemap") {
		sitemapURL = strings.TrimSuffix(sitemapURL, "/") + "/sitemap.xml"
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
			Description: "Failed to fetch",
			Content:     fmt.Sprintf("<p class=\"error\">Failed to fetch sitemap: %s</p>", escapeHTML(err.Error())),
			Error:       "fetch_failed",
		}, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return &Answer{
			Type:        AnswerTypeSitemap,
			Term:        term,
			Title:       fmt.Sprintf("Sitemap: %s", term),
			Description: "Not found",
			Content:     "<p>No sitemap.xml found at the standard location.</p>",
			Error:       "not_found",
		}, nil
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 500*1024)) // Limit to 500KB
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
	xml.Unmarshal(data, &urlset)

	// Try as sitemap index
	var index struct {
		Sitemaps []struct {
			Loc string `xml:"loc"`
		} `xml:"sitemap"`
	}
	xml.Unmarshal(data, &index)

	sitemapIndex := make([]string, len(index.Sitemaps))
	for i, sm := range index.Sitemaps {
		sitemapIndex[i] = sm.Loc
	}

	return urlset.URLs, sitemapIndex
}

func formatSitemapContent(domain string, urls []sitemapURL, sitemapIndex []string) string {
	var html strings.Builder
	html.WriteString("<div class=\"sitemap-content\">")
	html.WriteString(fmt.Sprintf("<h1>Sitemap: %s</h1>", escapeHTML(domain)))

	if len(sitemapIndex) > 0 {
		html.WriteString("<h2>Sitemap Index</h2>")
		html.WriteString(fmt.Sprintf("<p>Found %d child sitemaps</p>", len(sitemapIndex)))
		html.WriteString("<ul>")
		for _, sm := range sitemapIndex {
			html.WriteString(fmt.Sprintf("<li><a href=\"%s\">%s</a></li>", escapeHTML(sm), escapeHTML(sm)))
		}
		html.WriteString("</ul>")
	}

	if len(urls) > 0 {
		html.WriteString("<h2>URLs</h2>")
		html.WriteString(fmt.Sprintf("<p>Found %d URLs</p>", len(urls)))
		html.WriteString("<table class=\"sitemap-table\">")
		html.WriteString("<thead><tr><th>URL</th><th>Last Modified</th><th>Priority</th></tr></thead>")
		html.WriteString("<tbody>")

		maxShow := 50
		for i, u := range urls {
			if i >= maxShow {
				html.WriteString(fmt.Sprintf("<tr><td colspan=\"3\">... and %d more URLs</td></tr>", len(urls)-maxShow))
				break
			}
			html.WriteString(fmt.Sprintf("<tr><td><a href=\"%s\">%s</a></td><td>%s</td><td>%s</td></tr>",
				escapeHTML(u.Loc), escapeHTML(truncateURL(u.Loc, 60)), escapeHTML(u.LastMod), escapeHTML(u.Priority)))
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
		client: &http.Client{Timeout: 15 * time.Second},
	}
}

func (h *TechHandler) Type() AnswerType {
	return AnswerTypeTech
}

func (h *TechHandler) Handle(ctx context.Context, term string) (*Answer, error) {
	term = strings.TrimSpace(term)
	if term == "" {
		return nil, fmt.Errorf("domain required")
	}

	targetURL := term
	if !strings.HasPrefix(term, "http://") && !strings.HasPrefix(term, "https://") {
		targetURL = "https://" + term
	}

	req, err := http.NewRequestWithContext(ctx, "GET", targetURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", version.BrowserUserAgent)

	resp, err := h.client.Do(req)
	if err != nil {
		return &Answer{
			Type:        AnswerTypeTech,
			Term:        term,
			Title:       fmt.Sprintf("Tech Stack: %s", term),
			Description: "Failed to analyze",
			Content:     fmt.Sprintf("<p class=\"error\">Failed to fetch: %s</p>", escapeHTML(err.Error())),
			Error:       "fetch_failed",
		}, nil
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 200*1024)) // Limit to 200KB

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
		Description: "Technology detection",
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
	html.WriteString(fmt.Sprintf("<h1>Technology Stack: %s</h1>", escapeHTML(domain)))

	if len(tech) == 0 {
		html.WriteString("<p>No technologies detected.</p>")
	} else {
		for category, items := range tech {
			if len(items) > 0 {
				html.WriteString(fmt.Sprintf("<h2>%s</h2>", escapeHTML(category)))
				html.WriteString("<ul>")
				for _, item := range items {
					html.WriteString(fmt.Sprintf("<li>%s</li>", escapeHTML(item)))
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
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

func (h *FeedHandler) Type() AnswerType {
	return AnswerTypeFeed
}

func (h *FeedHandler) Handle(ctx context.Context, term string) (*Answer, error) {
	term = strings.TrimSpace(term)
	if term == "" {
		return nil, fmt.Errorf("domain required")
	}

	targetURL := term
	if !strings.HasPrefix(term, "http://") && !strings.HasPrefix(term, "https://") {
		targetURL = "https://" + term
	}

	req, err := http.NewRequestWithContext(ctx, "GET", targetURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", version.BrowserUserAgent)

	resp, err := h.client.Do(req)
	if err != nil {
		return &Answer{
			Type:        AnswerTypeFeed,
			Term:        term,
			Title:       fmt.Sprintf("Feeds: %s", term),
			Description: "Failed to fetch",
			Content:     fmt.Sprintf("<p class=\"error\">Failed to fetch: %s</p>", escapeHTML(err.Error())),
			Error:       "fetch_failed",
		}, nil
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 100*1024))

	// Discover feeds
	feeds := discoverFeeds(targetURL, string(body))

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
	URL   string
	Type  string
	Title string
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
					URL:   feedURL,
					Type:  p.feedType,
					Title: title,
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
				URL:  feedURL,
				Type: cp.feedType + " (guessed)",
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
	html.WriteString(fmt.Sprintf("<h1>Feeds: %s</h1>", escapeHTML(domain)))

	if len(feeds) == 0 {
		html.WriteString("<p>No feeds discovered.</p>")
	} else {
		html.WriteString(fmt.Sprintf("<p>Found %d potential feeds</p>", len(feeds)))
		html.WriteString("<table class=\"feed-table\">")
		html.WriteString("<thead><tr><th>URL</th><th>Type</th><th>Title</th></tr></thead>")
		html.WriteString("<tbody>")

		for _, f := range feeds {
			title := f.Title
			if title == "" {
				title = "-"
			}
			html.WriteString(fmt.Sprintf("<tr><td><a href=\"%s\">%s</a></td><td>%s</td><td>%s</td></tr>",
				escapeHTML(f.URL), escapeHTML(truncateURL(f.URL, 50)), escapeHTML(f.Type), escapeHTML(title)))
		}

		html.WriteString("</tbody></table>")
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
		client: &http.Client{
			Timeout: 15 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 10 {
					return fmt.Errorf("too many redirects")
				}
				return nil
			},
		},
	}
}

func (h *ExpandHandler) Type() AnswerType {
	return AnswerTypeExpand
}

func (h *ExpandHandler) Handle(ctx context.Context, term string) (*Answer, error) {
	term = strings.TrimSpace(term)
	if term == "" {
		return nil, fmt.Errorf("URL required")
	}

	targetURL := term
	if !strings.HasPrefix(term, "http://") && !strings.HasPrefix(term, "https://") {
		targetURL = "https://" + term
	}

	// Track redirects
	redirects := []string{targetURL}

	client := &http.Client{
		Timeout: 15 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("too many redirects")
			}
			redirects = append(redirects, req.URL.String())
			return nil
		},
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
			Title:       "URL Expander",
			Description: "Failed to expand",
			Content:     fmt.Sprintf("<p class=\"error\">Failed to expand URL: %s</p>", escapeHTML(err.Error())),
			Error:       "expand_failed",
		}, nil
	}
	if resp != nil {
		defer resp.Body.Close()
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
		Title:       "URL Expander",
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
	html.WriteString(fmt.Sprintf("<p><code>%s</code></p>", escapeHTML(original)))

	html.WriteString("<h2>Final URL</h2>")
	html.WriteString(fmt.Sprintf("<p><a href=\"%s\">%s</a></p>", escapeHTML(final), escapeHTML(final)))

	if len(redirects) > 1 {
		html.WriteString(fmt.Sprintf("<h2>Redirect Chain (%d hops)</h2>", len(redirects)-1))
		html.WriteString("<ol class=\"redirect-chain\">")
		for i, r := range redirects {
			if i == 0 {
				html.WriteString(fmt.Sprintf("<li><strong>Start:</strong> <code>%s</code></li>", escapeHTML(r)))
			} else if i == len(redirects)-1 {
				html.WriteString(fmt.Sprintf("<li><strong>Final:</strong> <code>%s</code></li>", escapeHTML(r)))
			} else {
				html.WriteString(fmt.Sprintf("<li><code>%s</code></li>", escapeHTML(r)))
			}
		}
		html.WriteString("</ol>")
	}

	html.WriteString("</div>")
	return html.String()
}

// SafeHandler handles safe:{url} queries (URL safety check)
type SafeHandler struct {
	client *http.Client
}

// NewSafeHandler creates a new URL safety handler
func NewSafeHandler() *SafeHandler {
	return &SafeHandler{
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

func (h *SafeHandler) Type() AnswerType {
	return AnswerTypeSafe
}

func (h *SafeHandler) Handle(ctx context.Context, term string) (*Answer, error) {
	term = strings.TrimSpace(term)
	if term == "" {
		return nil, fmt.Errorf("URL or domain required")
	}

	targetURL := term
	if !strings.HasPrefix(term, "http://") && !strings.HasPrefix(term, "https://") {
		targetURL = "https://" + term
	}

	parsedURL, err := url.Parse(targetURL)
	if err != nil {
		return &Answer{
			Type:        AnswerTypeSafe,
			Term:        term,
			Title:       "URL Safety Check",
			Description: "Invalid URL",
			Content:     fmt.Sprintf("<p class=\"error\">Invalid URL: %s</p>", escapeHTML(term)),
			Error:       "invalid_url",
		}, nil
	}

	domain := parsedURL.Hostname()

	// Perform safety checks
	checks := performSafetyChecks(ctx, h.client, domain, targetURL)

	// Determine overall safety
	safe := true
	for _, check := range checks {
		if !check.passed {
			safe = false
			break
		}
	}

	var rating string
	if safe {
		rating = "Safe"
	} else {
		rating = "Suspicious"
	}

	data := map[string]interface{}{
		"url":    targetURL,
		"domain": domain,
		"safe":   safe,
		"rating": rating,
		"checks": checks,
	}

	return &Answer{
		Type:        AnswerTypeSafe,
		Term:        term,
		Title:       fmt.Sprintf("URL Safety: %s", domain),
		Description: rating,
		Content:     formatSafeContent(domain, rating, safe, checks),
		Source:      "Safety Analyzer",
		Data:        data,
	}, nil
}

type safetyCheck struct {
	name    string
	passed  bool
	details string
}

func performSafetyChecks(ctx context.Context, client *http.Client, domain, targetURL string) []safetyCheck {
	var checks []safetyCheck

	// Check 1: HTTPS
	checks = append(checks, safetyCheck{
		name:    "HTTPS",
		passed:  strings.HasPrefix(targetURL, "https://"),
		details: "Site uses secure HTTPS connection",
	})

	// Check 2: Valid domain
	validDomain := !strings.Contains(domain, "--") && !strings.HasPrefix(domain, "-")
	checks = append(checks, safetyCheck{
		name:    "Valid Domain",
		passed:  validDomain,
		details: "Domain format is valid",
	})

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
		name:    "Domain TLD",
		passed:  !hasSuspiciousTLD,
		details: "TLD is not commonly associated with spam",
	})

	// Check 4: Not excessive subdomains
	parts := strings.Split(domain, ".")
	checks = append(checks, safetyCheck{
		name:    "Domain Structure",
		passed:  len(parts) <= 4,
		details: "Not excessive subdomains",
	})

	// Check 5: Domain length
	checks = append(checks, safetyCheck{
		name:    "Domain Length",
		passed:  len(domain) < 50,
		details: "Domain length is reasonable",
	})

	return checks
}

func formatSafeContent(domain, rating string, safe bool, checks []safetyCheck) string {
	var html strings.Builder
	html.WriteString("<div class=\"safe-content\">")
	html.WriteString(fmt.Sprintf("<h1>URL Safety: %s</h1>", escapeHTML(domain)))

	ratingClass := "safe"
	if !safe {
		ratingClass = "suspicious"
	}
	html.WriteString(fmt.Sprintf("<p class=\"rating %s\">%s</p>", ratingClass, escapeHTML(rating)))

	html.WriteString("<h2>Security Checks</h2>")
	html.WriteString("<table class=\"checks-table\">")
	html.WriteString("<tbody>")

	for _, check := range checks {
		status := "✓"
		statusClass := "passed"
		if !check.passed {
			status = "✗"
			statusClass = "failed"
		}
		html.WriteString(fmt.Sprintf("<tr class=\"%s\"><td>%s</td><td>%s</td><td>%s</td></tr>",
			statusClass, status, escapeHTML(check.name), escapeHTML(check.details)))
	}

	html.WriteString("</tbody></table>")

	html.WriteString("<p class=\"note\"><strong>Note:</strong> This is a basic check. For comprehensive security analysis, use dedicated security services.</p>")
	html.WriteString("</div>")
	return html.String()
}

// CacheHandler handles cache:{url} queries (web archive lookup)
type CacheHandler struct {
	client *http.Client
}

// NewCacheHandler creates a new web cache/archive handler
func NewCacheHandler() *CacheHandler {
	return &CacheHandler{
		client: &http.Client{Timeout: 15 * time.Second},
	}
}

func (h *CacheHandler) Type() AnswerType {
	return AnswerTypeCache
}

func (h *CacheHandler) Handle(ctx context.Context, term string) (*Answer, error) {
	term = strings.TrimSpace(term)
	if term == "" {
		return nil, fmt.Errorf("URL required")
	}

	targetURL := term
	if !strings.HasPrefix(term, "http://") && !strings.HasPrefix(term, "https://") {
		targetURL = "https://" + term
	}

	// Generate links to various archive services
	archives := []struct {
		name string
		url  string
	}{
		{"Wayback Machine", fmt.Sprintf("https://web.archive.org/web/*/%s", url.QueryEscape(targetURL))},
		{"Wayback Machine (Latest)", fmt.Sprintf("https://web.archive.org/web/%s", url.QueryEscape(targetURL))},
		{"Archive.today", fmt.Sprintf("https://archive.today/%s", url.QueryEscape(targetURL))},
		{"Google Cache", fmt.Sprintf("https://webcache.googleusercontent.com/search?q=cache:%s", url.QueryEscape(targetURL))},
	}

	data := map[string]interface{}{
		"url":      targetURL,
		"archives": archives,
	}

	return &Answer{
		Type:        AnswerTypeCache,
		Term:        term,
		Title:       fmt.Sprintf("Cached: %s", term),
		Description: "Web archive and cache links",
		Content:     formatCacheContent(targetURL, archives),
		Source:      "Web Archives",
		Data:        data,
	}, nil
}

func formatCacheContent(targetURL string, archives []struct {
	name string
	url  string
}) string {
	var html strings.Builder
	html.WriteString("<div class=\"cache-content\">")
	html.WriteString("<h1>Web Cache/Archive</h1>")
	html.WriteString(fmt.Sprintf("<p>Looking for cached version of: <code>%s</code></p>", escapeHTML(targetURL)))

	html.WriteString("<h2>Archive Services</h2>")
	html.WriteString("<ul class=\"archive-links\">")

	for _, a := range archives {
		html.WriteString(fmt.Sprintf("<li><a href=\"%s\" target=\"_blank\" rel=\"noopener\">%s</a></li>",
			escapeHTML(a.url), escapeHTML(a.name)))
	}

	html.WriteString("</ul>")

	html.WriteString("<h2>Save to Archive</h2>")
	html.WriteString(fmt.Sprintf("<p><a href=\"https://web.archive.org/save/%s\" target=\"_blank\" rel=\"noopener\">Save current page to Wayback Machine</a></p>",
		url.QueryEscape(targetURL)))

	html.WriteString("</div>")
	return html.String()
}
