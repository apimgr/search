package instant

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/apimgr/search/src/version"
)

// AnswerTypeRobots is the answer type for robots.txt analysis
const AnswerTypeRobots AnswerType = "robots"

// RobotDirective represents a parsed robots.txt directive
type RobotDirective struct {
	Type  string `json:"type"`  // "allow", "disallow", "crawl-delay", "sitemap"
	Value string `json:"value"` // the path or value
}

// RobotUserAgent represents rules for a specific user agent
type RobotUserAgent struct {
	Agent      string           `json:"agent"`
	Directives []RobotDirective `json:"directives"`
}

// RobotsHandler handles robots.txt fetching and parsing
type RobotsHandler struct {
	client   *http.Client
	patterns []*regexp.Regexp
}

// NewRobotsHandler creates a new robots.txt handler
func NewRobotsHandler() *RobotsHandler {
	return &RobotsHandler{
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		patterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)^robots[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^robots\.txt[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^get\s+robots[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^fetch\s+robots[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^check\s+robots[:\s]+(.+)$`),
		},
	}
}

func (h *RobotsHandler) Name() string {
	return "robots"
}

func (h *RobotsHandler) Patterns() []*regexp.Regexp {
	return h.patterns
}

func (h *RobotsHandler) CanHandle(query string) bool {
	for _, p := range h.patterns {
		if p.MatchString(query) {
			return true
		}
	}
	return false
}

func (h *RobotsHandler) Handle(ctx context.Context, query string) (*Answer, error) {
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

	// Clean up domain
	domain = strings.TrimPrefix(domain, "https://")
	domain = strings.TrimPrefix(domain, "http://")
	domain = strings.TrimSuffix(domain, "/")
	if idx := strings.Index(domain, "/"); idx != -1 {
		domain = domain[:idx]
	}

	// Build robots.txt URL
	robotsURL := fmt.Sprintf("https://%s/robots.txt", domain)

	// Fetch robots.txt
	req, err := http.NewRequestWithContext(ctx, "GET", robotsURL, nil)
	if err != nil {
		return &Answer{
			Type:    AnswerTypeRobots,
			Query:   query,
			Title:   fmt.Sprintf("robots.txt: %s", domain),
			Content: fmt.Sprintf("<strong>Error:</strong> Invalid domain<br><br>%s", escapeHTML(err.Error())),
		}, nil
	}

	req.Header.Set("User-Agent", version.BrowserUserAgent)

	resp, err := h.client.Do(req)
	if err != nil {
		// Try HTTP if HTTPS fails
		robotsURL = fmt.Sprintf("http://%s/robots.txt", domain)
		req, _ = http.NewRequestWithContext(ctx, "GET", robotsURL, nil)
		req.Header.Set("User-Agent", version.BrowserUserAgent)
		resp, err = h.client.Do(req)
		if err != nil {
			return &Answer{
				Type:    AnswerTypeRobots,
				Query:   query,
				Title:   fmt.Sprintf("robots.txt: %s", domain),
				Content: fmt.Sprintf("<strong>Error:</strong> Could not fetch robots.txt<br><br>%s", escapeHTML(err.Error())),
				Data: map[string]interface{}{
					"domain": domain,
					"error":  err.Error(),
				},
			}, nil
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return &Answer{
			Type:    AnswerTypeRobots,
			Query:   query,
			Title:   fmt.Sprintf("robots.txt: %s", domain),
			Content: fmt.Sprintf("<strong>No robots.txt found</strong><br><br>The site %s does not have a robots.txt file (HTTP 404).", escapeHTML(domain)),
			Data: map[string]interface{}{
				"domain":      domain,
				"status_code": resp.StatusCode,
				"found":       false,
			},
		}, nil
	}

	if resp.StatusCode != http.StatusOK {
		return &Answer{
			Type:    AnswerTypeRobots,
			Query:   query,
			Title:   fmt.Sprintf("robots.txt: %s", domain),
			Content: fmt.Sprintf("<strong>Error:</strong> HTTP %d when fetching robots.txt", resp.StatusCode),
			Data: map[string]interface{}{
				"domain":      domain,
				"status_code": resp.StatusCode,
			},
		}, nil
	}

	// Read and parse robots.txt
	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024)) // Limit to 1MB
	if err != nil {
		return &Answer{
			Type:    AnswerTypeRobots,
			Query:   query,
			Title:   fmt.Sprintf("robots.txt: %s", domain),
			Content: fmt.Sprintf("<strong>Error:</strong> Could not read robots.txt<br><br>%s", escapeHTML(err.Error())),
		}, nil
	}

	robotsContent := string(bodyBytes)
	parsed := parseRobotsTxt(robotsContent)

	// Build content
	var content strings.Builder
	content.WriteString("<div class=\"robots-result\">")
	content.WriteString(fmt.Sprintf("<strong>URL:</strong> <a href=\"%s\" target=\"_blank\">%s</a><br><br>", escapeHTML(robotsURL), escapeHTML(robotsURL)))

	// Summary
	content.WriteString("<strong>Summary:</strong><br>")
	content.WriteString(fmt.Sprintf("&nbsp;&nbsp;User-Agent sections: %d<br>", len(parsed.UserAgents)))
	content.WriteString(fmt.Sprintf("&nbsp;&nbsp;Sitemaps: %d<br><br>", len(parsed.Sitemaps)))

	// Show sitemaps first
	if len(parsed.Sitemaps) > 0 {
		content.WriteString("<strong>Sitemaps:</strong><br>")
		for _, sitemap := range parsed.Sitemaps {
			content.WriteString(fmt.Sprintf("&nbsp;&nbsp;<a href=\"%s\" target=\"_blank\">%s</a><br>", escapeHTML(sitemap), escapeHTML(sitemap)))
		}
		content.WriteString("<br>")
	}

	// Show user agents and their rules
	if len(parsed.UserAgents) > 0 {
		content.WriteString("<strong>Rules by User-Agent:</strong><br>")
		for _, ua := range parsed.UserAgents {
			content.WriteString(fmt.Sprintf("<br><em>User-Agent: %s</em><br>", escapeHTML(ua.Agent)))
			for _, directive := range ua.Directives {
				icon := ""
				switch directive.Type {
				case "disallow":
					icon = "<span style=\"color: red;\">Disallow:</span>"
				case "allow":
					icon = "<span style=\"color: green;\">Allow:</span>"
				case "crawl-delay":
					icon = "<span style=\"color: blue;\">Crawl-Delay:</span>"
				default:
					icon = directive.Type + ":"
				}
				content.WriteString(fmt.Sprintf("&nbsp;&nbsp;%s %s<br>", icon, escapeHTML(directive.Value)))
			}
		}
	}

	// Raw content (truncated)
	content.WriteString("<br><strong>Raw Content:</strong><br>")
	content.WriteString("<pre style=\"max-height: 300px; overflow-y: auto; background: #f5f5f5; padding: 10px; font-size: 0.85em;\">")
	rawContent := robotsContent
	if len(rawContent) > 5000 {
		rawContent = rawContent[:5000] + "\n... (truncated)"
	}
	content.WriteString(escapeHTML(rawContent))
	content.WriteString("</pre>")

	content.WriteString("</div>")

	data := map[string]interface{}{
		"domain":       domain,
		"url":          robotsURL,
		"status_code":  resp.StatusCode,
		"found":        true,
		"user_agents":  parsed.UserAgents,
		"sitemaps":     parsed.Sitemaps,
		"content_size": len(robotsContent),
	}

	return &Answer{
		Type:      AnswerTypeRobots,
		Query:     query,
		Title:     fmt.Sprintf("robots.txt: %s", domain),
		Content:   content.String(),
		Source:    domain,
		SourceURL: robotsURL,
		Data:      data,
	}, nil
}

// ParsedRobots represents parsed robots.txt content
type ParsedRobots struct {
	UserAgents []RobotUserAgent
	Sitemaps   []string
}

// parseRobotsTxt parses robots.txt content
func parseRobotsTxt(content string) ParsedRobots {
	result := ParsedRobots{
		UserAgents: make([]RobotUserAgent, 0),
		Sitemaps:   make([]string, 0),
	}

	scanner := bufio.NewScanner(strings.NewReader(content))
	var currentUA *RobotUserAgent

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Split on first colon
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}

		directive := strings.ToLower(strings.TrimSpace(parts[0]))
		value := strings.TrimSpace(parts[1])

		switch directive {
		case "user-agent":
			// Save current user agent if exists
			if currentUA != nil {
				result.UserAgents = append(result.UserAgents, *currentUA)
			}
			currentUA = &RobotUserAgent{
				Agent:      value,
				Directives: make([]RobotDirective, 0),
			}
		case "disallow":
			if currentUA != nil {
				currentUA.Directives = append(currentUA.Directives, RobotDirective{
					Type:  "disallow",
					Value: value,
				})
			}
		case "allow":
			if currentUA != nil {
				currentUA.Directives = append(currentUA.Directives, RobotDirective{
					Type:  "allow",
					Value: value,
				})
			}
		case "crawl-delay":
			if currentUA != nil {
				currentUA.Directives = append(currentUA.Directives, RobotDirective{
					Type:  "crawl-delay",
					Value: value,
				})
			}
		case "sitemap":
			result.Sitemaps = append(result.Sitemaps, value)
		}
	}

	// Don't forget the last user agent
	if currentUA != nil {
		result.UserAgents = append(result.UserAgents, *currentUA)
	}

	return result
}
