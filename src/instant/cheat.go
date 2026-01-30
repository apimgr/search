package instant

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// AnswerTypeCheat is the answer type for cheatsheet lookups
const AnswerTypeCheat AnswerType = "cheat"

// CheatHandler handles cheatsheet lookups from cheat.sh
type CheatHandler struct {
	client   *http.Client
	patterns []*regexp.Regexp
}

// NewCheatHandler creates a new cheatsheet handler
func NewCheatHandler() *CheatHandler {
	return &CheatHandler{
		client: &http.Client{Timeout: 15 * time.Second},
		patterns: []*regexp.Regexp{
			// cheat:command or cheat:language/topic
			regexp.MustCompile(`(?i)^cheat[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^cheatsheet[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^cht[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^tldr[:\s]+(.+)$`),
		},
	}
}

func (h *CheatHandler) Name() string              { return "cheat" }
func (h *CheatHandler) Patterns() []*regexp.Regexp { return h.patterns }

func (h *CheatHandler) CanHandle(query string) bool {
	for _, p := range h.patterns {
		if p.MatchString(query) {
			return true
		}
	}
	return false
}

func (h *CheatHandler) Handle(ctx context.Context, query string) (*Answer, error) {
	// Extract topic from query
	var topic string
	for _, p := range h.patterns {
		if matches := p.FindStringSubmatch(query); len(matches) > 1 {
			topic = strings.TrimSpace(matches[1])
			break
		}
	}

	if topic == "" {
		return nil, nil
	}

	// Build cheat.sh URL with ?T for plain text without ANSI colors
	cheatURL := fmt.Sprintf("https://cheat.sh/%s?T", url.PathEscape(topic))

	req, err := http.NewRequestWithContext(ctx, "GET", cheatURL, nil)
	if err != nil {
		return nil, err
	}

	// cheat.sh expects curl-like user agent for text output
	req.Header.Set("User-Agent", "curl/7.68.0")
	req.Header.Set("Accept", "text/plain")

	resp, err := h.client.Do(req)
	if err != nil {
		return h.errorAnswer(query, topic, "Failed to connect to cheat.sh"), nil
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return h.errorAnswer(query, topic, fmt.Sprintf("Cheatsheet for '%s' not found", topic)), nil
	}

	// Handle rate limiting
	if resp.StatusCode == http.StatusTooManyRequests {
		return h.errorAnswer(query, topic, "cheat.sh rate limit exceeded. Please try again later."), nil
	}

	if resp.StatusCode != http.StatusOK {
		return h.errorAnswer(query, topic, fmt.Sprintf("cheat.sh returned status %d", resp.StatusCode)), nil
	}

	// Read content (limit size)
	limitedReader := io.LimitReader(resp.Body, 64*1024)
	contentBytes, err := io.ReadAll(limitedReader)
	if err != nil {
		return h.errorAnswer(query, topic, "Failed to read cheatsheet"), nil
	}

	rawContent := string(contentBytes)

	// Check for "Unknown topic" response
	if strings.Contains(rawContent, "Unknown topic") || strings.Contains(rawContent, "not found") {
		return h.errorAnswer(query, topic, fmt.Sprintf("Cheatsheet for '%s' not found", topic)), nil
	}

	// Parse and format the cheatsheet content
	formattedContent := h.formatCheatsheet(rawContent, topic)

	// Build web URL (without ?T)
	webURL := fmt.Sprintf("https://cheat.sh/%s", url.PathEscape(topic))

	var content strings.Builder
	content.WriteString(fmt.Sprintf("<strong>%s</strong><br><br>", escapeHTML(topic)))
	content.WriteString(formattedContent)
	content.WriteString("<br><strong>Links:</strong><br>")
	content.WriteString(fmt.Sprintf("&bull; <a href=\"%s\" target=\"_blank\">View on cheat.sh</a><br>", webURL))

	// Add related topics if it's a command
	if !strings.Contains(topic, "/") {
		content.WriteString(fmt.Sprintf("&bull; <a href=\"https://cheat.sh/%s/:learn\" target=\"_blank\">Learn more</a><br>", url.PathEscape(topic)))
		content.WriteString(fmt.Sprintf("&bull; <a href=\"https://cheat.sh/%s/:list\" target=\"_blank\">List all topics</a><br>", url.PathEscape(topic)))
	}

	return &Answer{
		Type:      AnswerTypeCheat,
		Query:     query,
		Title:     fmt.Sprintf("Cheatsheet: %s", topic),
		Content:   content.String(),
		Source:    "cheat.sh",
		SourceURL: webURL,
		Data: map[string]interface{}{
			"topic":   topic,
			"content": rawContent,
		},
	}, nil
}

func (h *CheatHandler) formatCheatsheet(content, topic string) string {
	lines := strings.Split(content, "\n")
	var result strings.Builder

	// Track if we're in a code block
	inCode := false
	codeLines := []string{}
	maxLines := 50 // Limit output

	lineCount := 0
	for _, line := range lines {
		if lineCount >= maxLines {
			result.WriteString("<em>... (truncated, see full cheatsheet on cheat.sh)</em><br>")
			break
		}

		// Skip empty lines at the beginning
		if lineCount == 0 && strings.TrimSpace(line) == "" {
			continue
		}

		// Detect comment lines (start with #)
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			// If we have accumulated code, output it first
			if len(codeLines) > 0 {
				result.WriteString("<pre><code>")
				result.WriteString(escapeHTML(strings.Join(codeLines, "\n")))
				result.WriteString("</code></pre>")
				codeLines = nil
				inCode = false
			}

			// Output comment as explanation
			comment := strings.TrimPrefix(strings.TrimSpace(line), "#")
			comment = strings.TrimSpace(comment)
			if comment != "" {
				result.WriteString(fmt.Sprintf("<strong>%s</strong><br>", escapeHTML(comment)))
			}
		} else if strings.TrimSpace(line) != "" {
			// Code line
			codeLines = append(codeLines, line)
			inCode = true
		} else if inCode && len(codeLines) > 0 {
			// Empty line after code - output the code block
			result.WriteString("<pre><code>")
			result.WriteString(escapeHTML(strings.Join(codeLines, "\n")))
			result.WriteString("</code></pre>")
			codeLines = nil
			inCode = false
		}

		lineCount++
	}

	// Output any remaining code
	if len(codeLines) > 0 {
		result.WriteString("<pre><code>")
		result.WriteString(escapeHTML(strings.Join(codeLines, "\n")))
		result.WriteString("</code></pre>")
	}

	return result.String()
}

func (h *CheatHandler) errorAnswer(query, topic, message string) *Answer {
	var content strings.Builder
	content.WriteString(fmt.Sprintf("<span class=\"error\">%s</span><br><br>", message))
	content.WriteString("<strong>Suggestions:</strong><br>")
	content.WriteString("&bull; Check the spelling of the command/topic<br>")
	content.WriteString("&bull; Try a more general topic (e.g., 'git' instead of 'git-specific-command')<br>")
	content.WriteString("&bull; Browse available topics at <a href=\"https://cheat.sh/:list\" target=\"_blank\">cheat.sh/:list</a><br>")

	return &Answer{
		Type:      AnswerTypeCheat,
		Query:     query,
		Title:     fmt.Sprintf("Cheatsheet: %s", topic),
		Content:   content.String(),
		Source:    "cheat.sh",
		SourceURL: "https://cheat.sh/",
	}
}
