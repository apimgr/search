package instant

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/apimgr/search/src/version"
)

// AnswerTypeHeaders is the answer type for HTTP headers analysis
const AnswerTypeHeaders AnswerType = "headers"

// HeadersHandler handles HTTP response header queries
type HeadersHandler struct {
	client   *http.Client
	patterns []*regexp.Regexp
}

// NewHeadersHandler creates a new headers handler
func NewHeadersHandler() *HeadersHandler {
	return &HeadersHandler{
		client: &http.Client{
			Timeout: 15 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				// Don't follow redirects - we want to see the headers of the requested URL
				return http.ErrUseLastResponse
			},
		},
		patterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)^headers[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^http\s+headers[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^response\s+headers[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^check\s+headers[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^head[:\s]+(.+)$`),
		},
	}
}

func (h *HeadersHandler) Name() string {
	return "headers"
}

func (h *HeadersHandler) Patterns() []*regexp.Regexp {
	return h.patterns
}

func (h *HeadersHandler) CanHandle(query string) bool {
	for _, p := range h.patterns {
		if p.MatchString(query) {
			return true
		}
	}
	return false
}

func (h *HeadersHandler) Handle(ctx context.Context, query string) (*Answer, error) {
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

	// Make HEAD request
	req, err := http.NewRequestWithContext(ctx, "HEAD", urlStr, nil)
	if err != nil {
		return &Answer{
			Type:    AnswerTypeHeaders,
			Query:   query,
			Title:   fmt.Sprintf("HTTP Headers: %s", urlStr),
			Content: fmt.Sprintf("<strong>Error:</strong> Invalid URL<br><br>%s", escapeHTML(err.Error())),
		}, nil
	}

	req.Header.Set("User-Agent", version.BrowserUserAgent)

	resp, err := h.client.Do(req)
	if err != nil {
		return &Answer{
			Type:    AnswerTypeHeaders,
			Query:   query,
			Title:   fmt.Sprintf("HTTP Headers: %s", urlStr),
			Content: fmt.Sprintf("<strong>Error:</strong> Could not fetch headers<br><br>%s", escapeHTML(err.Error())),
			Data: map[string]interface{}{
				"url":   urlStr,
				"error": err.Error(),
			},
		}, nil
	}
	defer resp.Body.Close()

	// Build content
	var content strings.Builder
	content.WriteString("<div class=\"headers-result\">")

	// Status line
	content.WriteString(fmt.Sprintf("<strong>Status:</strong> %s<br><br>", escapeHTML(resp.Status)))

	// Analyze security headers
	securityAnalysis := analyzeSecurityHeaders(resp.Header)
	if len(securityAnalysis) > 0 {
		content.WriteString("<strong>Security Analysis:</strong><br>")
		for _, analysis := range securityAnalysis {
			content.WriteString(fmt.Sprintf("&nbsp;&nbsp;%s<br>", analysis))
		}
		content.WriteString("<br>")
	}

	// Response headers sorted alphabetically
	content.WriteString("<strong>Response Headers:</strong><br>")
	content.WriteString("<table style=\"font-family: monospace; font-size: 0.9em;\">")

	// Sort header names
	var headerNames []string
	for name := range resp.Header {
		headerNames = append(headerNames, name)
	}
	sort.Strings(headerNames)

	for _, name := range headerNames {
		values := resp.Header[name]
		for _, value := range values {
			content.WriteString(fmt.Sprintf("<tr><td style=\"vertical-align: top; padding-right: 10px;\"><strong>%s:</strong></td><td>%s</td></tr>",
				escapeHTML(name), escapeHTML(value)))
		}
	}
	content.WriteString("</table>")

	content.WriteString("</div>")

	// Build data map
	headerMap := make(map[string][]string)
	for name, values := range resp.Header {
		headerMap[name] = values
	}

	data := map[string]interface{}{
		"url":               urlStr,
		"status_code":       resp.StatusCode,
		"status":            resp.Status,
		"protocol":          resp.Proto,
		"headers":           headerMap,
		"security_analysis": securityAnalysis,
	}

	return &Answer{
		Type:    AnswerTypeHeaders,
		Query:   query,
		Title:   fmt.Sprintf("HTTP Headers: %s", urlStr),
		Content: content.String(),
		Data:    data,
	}, nil
}

// analyzeSecurityHeaders checks for important security headers
func analyzeSecurityHeaders(headers http.Header) []string {
	var analysis []string

	// Check for security headers
	securityHeaders := map[string]struct {
		present  string
		missing  string
		critical bool
	}{
		"Strict-Transport-Security": {
			present:  "<span style=\"color: green;\">HSTS enabled</span>",
			missing:  "<span style=\"color: orange;\">HSTS not set (recommended)</span>",
			critical: true,
		},
		"Content-Security-Policy": {
			present:  "<span style=\"color: green;\">CSP configured</span>",
			missing:  "<span style=\"color: orange;\">CSP not set (recommended)</span>",
			critical: true,
		},
		"X-Frame-Options": {
			present:  "<span style=\"color: green;\">Clickjacking protection enabled</span>",
			missing:  "<span style=\"color: orange;\">X-Frame-Options not set</span>",
			critical: false,
		},
		"X-Content-Type-Options": {
			present:  "<span style=\"color: green;\">MIME type sniffing protection enabled</span>",
			missing:  "<span style=\"color: orange;\">X-Content-Type-Options not set</span>",
			critical: false,
		},
		"X-XSS-Protection": {
			present:  "<span style=\"color: green;\">XSS protection header present</span>",
			missing:  "<span style=\"color: gray;\">X-XSS-Protection not set (deprecated)</span>",
			critical: false,
		},
		"Referrer-Policy": {
			present:  "<span style=\"color: green;\">Referrer policy configured</span>",
			missing:  "<span style=\"color: orange;\">Referrer-Policy not set</span>",
			critical: false,
		},
		"Permissions-Policy": {
			present:  "<span style=\"color: green;\">Permissions policy configured</span>",
			missing:  "<span style=\"color: gray;\">Permissions-Policy not set</span>",
			critical: false,
		},
	}

	for header, info := range securityHeaders {
		if headers.Get(header) != "" {
			analysis = append(analysis, info.present)
		} else if info.critical {
			analysis = append(analysis, info.missing)
		}
	}

	// Check for server header (information disclosure)
	if server := headers.Get("Server"); server != "" {
		analysis = append(analysis, fmt.Sprintf("<span style=\"color: orange;\">Server header exposed: %s</span>", escapeHTML(server)))
	}

	// Check for X-Powered-By (information disclosure)
	if powered := headers.Get("X-Powered-By"); powered != "" {
		analysis = append(analysis, fmt.Sprintf("<span style=\"color: orange;\">X-Powered-By exposed: %s</span>", escapeHTML(powered)))
	}

	sort.Strings(analysis)
	return analysis
}
