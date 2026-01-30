package instant

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/apimgr/search/src/version"
)

// AnswerTypeExpand is the answer type for URL expansion
const AnswerTypeExpand AnswerType = "expand"

// RedirectHop represents a single redirect in the chain
type RedirectHop struct {
	URL        string `json:"url"`
	StatusCode int    `json:"status_code"`
	Status     string `json:"status"`
}

// ExpandHandler handles URL expansion by following redirects
type ExpandHandler struct {
	patterns []*regexp.Regexp
}

// NewExpandHandler creates a new URL expansion handler
func NewExpandHandler() *ExpandHandler {
	return &ExpandHandler{
		patterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)^expand[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^unshorten[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^follow[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^expand\s+url[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^url\s+expand[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^unshort[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^redirect[:\s]+(.+)$`),
		},
	}
}

func (h *ExpandHandler) Name() string {
	return "expand"
}

func (h *ExpandHandler) Patterns() []*regexp.Regexp {
	return h.patterns
}

func (h *ExpandHandler) CanHandle(query string) bool {
	for _, p := range h.patterns {
		if p.MatchString(query) {
			return true
		}
	}
	return false
}

func (h *ExpandHandler) Handle(ctx context.Context, query string) (*Answer, error) {
	// Extract URL from query
	urlStr := ""
	for _, p := range h.patterns {
		if matches := p.FindStringSubmatch(query); len(matches) > 1 {
			urlStr = strings.TrimSpace(matches[1])
			break
		}
	}

	if urlStr == "" {
		return nil, nil
	}

	// Add https:// if no protocol specified
	if !strings.HasPrefix(urlStr, "http://") && !strings.HasPrefix(urlStr, "https://") {
		urlStr = "https://" + urlStr
	}

	// Validate URL
	parsedURL, err := url.Parse(urlStr)
	if err != nil || parsedURL.Host == "" {
		return &Answer{
			Type:    AnswerTypeExpand,
			Query:   query,
			Title:   "URL Expander",
			Content: fmt.Sprintf("<strong>Error:</strong> Invalid URL<br><br>%s", escapeHTML(err.Error())),
		}, nil
	}

	// Follow redirects manually to track the chain
	hops, finalURL, err := h.followRedirects(ctx, urlStr)
	if err != nil {
		return &Answer{
			Type:    AnswerTypeExpand,
			Query:   query,
			Title:   "URL Expander",
			Content: fmt.Sprintf("<strong>Error:</strong> Could not expand URL<br><br>%s", escapeHTML(err.Error())),
			Data: map[string]interface{}{
				"original_url": urlStr,
				"error":        err.Error(),
			},
		}, nil
	}

	// Build content
	var content strings.Builder
	content.WriteString("<div class=\"expand-result\">")

	// Original URL
	content.WriteString(fmt.Sprintf("<strong>Original URL:</strong><br>"))
	content.WriteString(fmt.Sprintf("&nbsp;&nbsp;<code>%s</code><br><br>", escapeHTML(urlStr)))

	// Final URL
	content.WriteString(fmt.Sprintf("<strong>Final URL:</strong><br>"))
	content.WriteString(fmt.Sprintf("&nbsp;&nbsp;<a href=\"%s\" target=\"_blank\">%s</a><br><br>", escapeHTML(finalURL), escapeHTML(finalURL)))

	// Redirect chain
	if len(hops) > 1 {
		content.WriteString(fmt.Sprintf("<strong>Redirect Chain (%d hops):</strong><br>", len(hops)-1))
		for i, hop := range hops {
			statusColor := "green"
			if hop.StatusCode >= 400 {
				statusColor = "red"
			} else if hop.StatusCode >= 300 {
				statusColor = "orange"
			}

			prefix := "&nbsp;&nbsp;"
			if i > 0 {
				prefix = "&nbsp;&nbsp;&#x2192;&nbsp;"
			}

			content.WriteString(fmt.Sprintf("%s<span style=\"color: %s;\">[%d]</span> %s<br>",
				prefix, statusColor, hop.StatusCode, escapeHTML(truncateURL(hop.URL, 80))))
		}
	} else {
		content.WriteString("<strong>No redirects</strong> - URL points directly to final destination<br>")
	}

	// URL analysis
	finalParsed, _ := url.Parse(finalURL)
	if finalParsed != nil {
		content.WriteString("<br><strong>Final URL Analysis:</strong><br>")
		content.WriteString(fmt.Sprintf("&nbsp;&nbsp;Host: %s<br>", escapeHTML(finalParsed.Host)))
		if finalParsed.Path != "" && finalParsed.Path != "/" {
			content.WriteString(fmt.Sprintf("&nbsp;&nbsp;Path: %s<br>", escapeHTML(finalParsed.Path)))
		}
		if finalParsed.RawQuery != "" {
			content.WriteString(fmt.Sprintf("&nbsp;&nbsp;Query: %s<br>", escapeHTML(finalParsed.RawQuery)))
		}
	}

	// Compare domains
	originalParsed, _ := url.Parse(urlStr)
	if originalParsed != nil && finalParsed != nil && originalParsed.Host != finalParsed.Host {
		content.WriteString(fmt.Sprintf("<br><span style=\"color: orange;\">Note: Domain changed from <strong>%s</strong> to <strong>%s</strong></span>",
			escapeHTML(originalParsed.Host), escapeHTML(finalParsed.Host)))
	}

	content.WriteString("</div>")

	data := map[string]interface{}{
		"original_url": urlStr,
		"final_url":    finalURL,
		"hops":         hops,
		"redirect_count": len(hops) - 1,
	}

	return &Answer{
		Type:    AnswerTypeExpand,
		Query:   query,
		Title:   "URL Expander",
		Content: content.String(),
		Data:    data,
	}, nil
}

// followRedirects follows redirect chain and returns all hops
func (h *ExpandHandler) followRedirects(ctx context.Context, startURL string) ([]RedirectHop, string, error) {
	var hops []RedirectHop
	currentURL := startURL
	maxRedirects := 20

	client := &http.Client{
		Timeout: 15 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// Don't follow redirects automatically - we handle them manually
			return http.ErrUseLastResponse
		},
	}

	for i := 0; i < maxRedirects; i++ {
		req, err := http.NewRequestWithContext(ctx, "HEAD", currentURL, nil)
		if err != nil {
			return hops, currentURL, err
		}

		req.Header.Set("User-Agent", version.BrowserUserAgent)

		resp, err := client.Do(req)
		if err != nil {
			// Try GET if HEAD fails
			req, _ = http.NewRequestWithContext(ctx, "GET", currentURL, nil)
			req.Header.Set("User-Agent", version.BrowserUserAgent)
			resp, err = client.Do(req)
			if err != nil {
				return hops, currentURL, err
			}
		}
		resp.Body.Close()

		hops = append(hops, RedirectHop{
			URL:        currentURL,
			StatusCode: resp.StatusCode,
			Status:     resp.Status,
		})

		// Check if this is a redirect
		if resp.StatusCode >= 300 && resp.StatusCode < 400 {
			location := resp.Header.Get("Location")
			if location == "" {
				// No location header, stop here
				break
			}

			// Handle relative redirects
			nextURL, err := url.Parse(location)
			if err != nil {
				return hops, currentURL, err
			}

			if !nextURL.IsAbs() {
				base, _ := url.Parse(currentURL)
				nextURL = base.ResolveReference(nextURL)
			}

			currentURL = nextURL.String()
		} else {
			// Not a redirect, we're done
			break
		}
	}

	return hops, currentURL, nil
}

// truncateURL truncates a URL for display
func truncateURL(urlStr string, maxLen int) string {
	if len(urlStr) <= maxLen {
		return urlStr
	}
	return urlStr[:maxLen-3] + "..."
}
