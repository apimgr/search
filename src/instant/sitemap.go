package instant

import (
	"bufio"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/apimgr/search/src/version"
)

// SitemapHandler parses and displays sitemap.xml contents
type SitemapHandler struct {
	client   *http.Client
	patterns []*regexp.Regexp
}

// SitemapURL represents a URL entry in a sitemap
type SitemapURL struct {
	Loc        string `xml:"loc" json:"loc"`
	LastMod    string `xml:"lastmod" json:"lastmod,omitempty"`
	ChangeFreq string `xml:"changefreq" json:"changefreq,omitempty"`
	Priority   string `xml:"priority" json:"priority,omitempty"`
}

// SitemapIndex represents a sitemap index entry
type SitemapIndex struct {
	Loc     string `xml:"loc" json:"loc"`
	LastMod string `xml:"lastmod" json:"lastmod,omitempty"`
}

// Sitemap represents a parsed sitemap
type Sitemap struct {
	XMLName  xml.Name       `xml:"urlset"`
	URLs     []SitemapURL   `xml:"url"`
	Sitemaps []SitemapIndex `xml:"sitemap"` // for sitemap index
}

// SitemapIndexFile represents a sitemap index file
type SitemapIndexFile struct {
	XMLName  xml.Name       `xml:"sitemapindex"`
	Sitemaps []SitemapIndex `xml:"sitemap"`
}

// NewSitemapHandler creates a new sitemap handler
func NewSitemapHandler() *SitemapHandler {
	return &SitemapHandler{
		client: &http.Client{Timeout: 20 * time.Second},
		patterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)^sitemap[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^site\s*map[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^urls[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^list\s+urls[:\s]+(.+)$`),
		},
	}
}

func (h *SitemapHandler) Name() string {
	return "sitemap"
}

func (h *SitemapHandler) Patterns() []*regexp.Regexp {
	return h.patterns
}

func (h *SitemapHandler) CanHandle(query string) bool {
	for _, p := range h.patterns {
		if p.MatchString(query) {
			return true
		}
	}
	return false
}

func (h *SitemapHandler) Handle(ctx context.Context, query string) (*Answer, error) {
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

	// Try to find and parse sitemap
	sitemapURL, urls, sitemapIndexURLs, err := h.findAndParseSitemap(ctx, baseURL)
	if err != nil {
		return &Answer{
			Type:    AnswerTypeSitemap,
			Query:   query,
			Title:   fmt.Sprintf("Sitemap: %s", domain),
			Content: fmt.Sprintf("Error fetching sitemap: %v", err),
		}, nil
	}

	if len(urls) == 0 && len(sitemapIndexURLs) == 0 {
		return &Answer{
			Type:    AnswerTypeSitemap,
			Query:   query,
			Title:   fmt.Sprintf("Sitemap: %s", domain),
			Content: "No sitemap found or sitemap is empty.",
		}, nil
	}

	// Build content
	var content strings.Builder
	content.WriteString(fmt.Sprintf("<strong>Sitemap:</strong> <a href=\"%s\">%s</a><br><br>", sitemapURL, sitemapURL))

	// If it's a sitemap index, show the sitemaps
	if len(sitemapIndexURLs) > 0 {
		content.WriteString(fmt.Sprintf("<strong>Sitemap Index (%d sitemaps):</strong><br>", len(sitemapIndexURLs)))

		// Show first 20 sitemaps
		maxShow := 20
		if len(sitemapIndexURLs) < maxShow {
			maxShow = len(sitemapIndexURLs)
		}

		for i := 0; i < maxShow; i++ {
			sm := sitemapIndexURLs[i]
			lastMod := ""
			if sm.LastMod != "" {
				lastMod = fmt.Sprintf(" (Last modified: %s)", sm.LastMod)
			}
			content.WriteString(fmt.Sprintf("%d. <a href=\"%s\">%s</a>%s<br>", i+1, sm.Loc, sm.Loc, lastMod))
		}

		if len(sitemapIndexURLs) > maxShow {
			content.WriteString(fmt.Sprintf("<br><em>... and %d more sitemaps</em>", len(sitemapIndexURLs)-maxShow))
		}
	} else {
		content.WriteString(fmt.Sprintf("<strong>URLs (%d total):</strong><br>", len(urls)))

		// Show first 25 URLs
		maxShow := 25
		if len(urls) < maxShow {
			maxShow = len(urls)
		}

		for i := 0; i < maxShow; i++ {
			u := urls[i]
			meta := []string{}
			if u.LastMod != "" {
				meta = append(meta, fmt.Sprintf("Modified: %s", u.LastMod))
			}
			if u.Priority != "" {
				meta = append(meta, fmt.Sprintf("Priority: %s", u.Priority))
			}
			if u.ChangeFreq != "" {
				meta = append(meta, fmt.Sprintf("Freq: %s", u.ChangeFreq))
			}

			metaStr := ""
			if len(meta) > 0 {
				metaStr = fmt.Sprintf(" <small>(%s)</small>", strings.Join(meta, ", "))
			}

			content.WriteString(fmt.Sprintf("%d. <a href=\"%s\">%s</a>%s<br>", i+1, u.Loc, u.Loc, metaStr))
		}

		if len(urls) > maxShow {
			content.WriteString(fmt.Sprintf("<br><em>... and %d more URLs</em>", len(urls)-maxShow))
		}
	}

	data := map[string]interface{}{
		"domain":      domain,
		"sitemap_url": sitemapURL,
	}

	if len(sitemapIndexURLs) > 0 {
		data["is_index"] = true
		data["sitemaps"] = sitemapIndexURLs
		data["sitemap_count"] = len(sitemapIndexURLs)
	} else {
		data["is_index"] = false
		data["urls"] = urls
		data["url_count"] = len(urls)
	}

	return &Answer{
		Type:    AnswerTypeSitemap,
		Query:   query,
		Title:   fmt.Sprintf("Sitemap: %s", domain),
		Content: content.String(),
		Data:    data,
	}, nil
}

// findAndParseSitemap finds and parses the sitemap for a domain
func (h *SitemapHandler) findAndParseSitemap(ctx context.Context, baseURL string) (string, []SitemapURL, []SitemapIndex, error) {
	// First, check robots.txt for sitemap location
	sitemapURLs := h.getSitemapURLsFromRobots(ctx, baseURL)

	// Add default sitemap locations
	sitemapURLs = append(sitemapURLs,
		baseURL+"/sitemap.xml",
		baseURL+"/sitemap_index.xml",
		baseURL+"/sitemap-index.xml",
		baseURL+"/sitemaps/sitemap.xml",
	)

	// Remove duplicates
	seen := make(map[string]bool)
	unique := make([]string, 0)
	for _, u := range sitemapURLs {
		if !seen[u] {
			seen[u] = true
			unique = append(unique, u)
		}
	}

	// Try each sitemap URL
	for _, sitemapURL := range unique {
		urls, sitemapIndexURLs, err := h.parseSitemap(ctx, sitemapURL)
		if err == nil && (len(urls) > 0 || len(sitemapIndexURLs) > 0) {
			return sitemapURL, urls, sitemapIndexURLs, nil
		}
	}

	return "", nil, nil, fmt.Errorf("no valid sitemap found")
}

// getSitemapURLsFromRobots extracts sitemap URLs from robots.txt
func (h *SitemapHandler) getSitemapURLsFromRobots(ctx context.Context, baseURL string) []string {
	robotsURL := baseURL + "/robots.txt"

	req, err := http.NewRequestWithContext(ctx, "GET", robotsURL, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("User-Agent", version.BrowserUserAgent)

	resp, err := h.client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		if resp != nil {
			resp.Body.Close()
		}
		return nil
	}
	defer resp.Body.Close()

	var sitemapURLs []string
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		lowerLine := strings.ToLower(line)
		if strings.HasPrefix(lowerLine, "sitemap:") {
			// Extract URL after "Sitemap:"
			url := strings.TrimSpace(line[8:])
			if url != "" {
				sitemapURLs = append(sitemapURLs, url)
			}
		}
	}

	return sitemapURLs
}

// parseSitemap fetches and parses a sitemap
func (h *SitemapHandler) parseSitemap(ctx context.Context, sitemapURL string) ([]SitemapURL, []SitemapIndex, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", sitemapURL, nil)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("User-Agent", version.BrowserUserAgent)
	req.Header.Set("Accept", "application/xml, text/xml")

	resp, err := h.client.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("status %d", resp.StatusCode)
	}

	// Read body (limited to 5MB)
	body, err := io.ReadAll(io.LimitReader(resp.Body, 5*1024*1024))
	if err != nil {
		return nil, nil, err
	}

	// Try parsing as sitemap index first
	var sitemapIndex SitemapIndexFile
	if err := xml.Unmarshal(body, &sitemapIndex); err == nil && len(sitemapIndex.Sitemaps) > 0 {
		return nil, sitemapIndex.Sitemaps, nil
	}

	// Try parsing as regular sitemap
	var sitemap Sitemap
	if err := xml.Unmarshal(body, &sitemap); err == nil {
		return sitemap.URLs, nil, nil
	}

	return nil, nil, fmt.Errorf("failed to parse sitemap")
}
