package direct

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/apimgr/search/src/common/version"
)

// TLDRHandler handles tldr:{command} queries
type TLDRHandler struct {
	client *http.Client
}

// NewTLDRHandler creates a new TLDR handler
func NewTLDRHandler() *TLDRHandler {
	return &TLDRHandler{
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

func (h *TLDRHandler) Type() AnswerType {
	return AnswerTypeTLDR
}

func (h *TLDRHandler) Handle(ctx context.Context, term string) (*Answer, error) {
	term = strings.TrimSpace(strings.ToLower(term))
	if term == "" {
		return nil, fmt.Errorf("command name required")
	}

	// Fetch from tldr.sh API
	// Try multiple platforms: common, linux, osx, windows
	platforms := []string{"common", "linux", "osx", "windows"}

	for _, platform := range platforms {
		apiURL := fmt.Sprintf("https://raw.githubusercontent.com/tldr-pages/tldr/main/pages/%s/%s.md", platform, url.PathEscape(term))

		req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
		if err != nil {
			continue
		}
		req.Header.Set("User-Agent", version.BrowserUserAgent)

		resp, err := h.client.Do(req)
		if err != nil {
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			var content strings.Builder
			buf := make([]byte, 1024)
			for {
				n, err := resp.Body.Read(buf)
				if n > 0 {
					content.Write(buf[:n])
				}
				if err != nil {
					break
				}
			}

			// Parse tldr markdown format
			markdown := content.String()
			htmlContent := parseTLDRMarkdown(markdown, term)

			return &Answer{
				Type:        AnswerTypeTLDR,
				Term:        term,
				Title:       fmt.Sprintf("tldr: %s", term),
				Description: fmt.Sprintf("Quick reference for the %s command", term),
				Content:     htmlContent,
				Source:      "tldr-pages",
				SourceURL:   "https://tldr.sh",
				Data: map[string]interface{}{
					"platform": platform,
					"command":  term,
					"markdown": markdown,
				},
			}, nil
		}
	}

	return &Answer{
		Type:        AnswerTypeTLDR,
		Term:        term,
		Title:       fmt.Sprintf("tldr: %s", term),
		Description: "Command not found",
		Content:     fmt.Sprintf("<p>No tldr page found for <code>%s</code>.</p><p>Try searching for it using <code>man:%s</code> or <code>cheat:%s</code>.</p>", term, term, term),
		Error:       "not_found",
	}, nil
}

// parseTLDRMarkdown converts tldr markdown to HTML
func parseTLDRMarkdown(markdown, command string) string {
	var html strings.Builder
	lines := strings.Split(markdown, "\n")

	html.WriteString("<div class=\"tldr-content\">")

	for _, line := range lines {
		line = strings.TrimSpace(line)

		if line == "" {
			continue
		}

		if strings.HasPrefix(line, "# ") {
			// Command title
			title := strings.TrimPrefix(line, "# ")
			html.WriteString(fmt.Sprintf("<h1>%s</h1>", escapeHTML(title)))
		} else if strings.HasPrefix(line, "> ") {
			// Description
			desc := strings.TrimPrefix(line, "> ")
			html.WriteString(fmt.Sprintf("<p class=\"description\">%s</p>", escapeHTML(desc)))
		} else if strings.HasPrefix(line, "- ") {
			// Example description
			desc := strings.TrimPrefix(line, "- ")
			html.WriteString(fmt.Sprintf("<p class=\"example-desc\">%s</p>", escapeHTML(desc)))
		} else if strings.HasPrefix(line, "`") && strings.HasSuffix(line, "`") {
			// Code example
			code := strings.Trim(line, "`")
			html.WriteString(fmt.Sprintf("<pre class=\"example-code\"><code>%s</code></pre>", escapeHTML(code)))
			html.WriteString("<button class=\"copy-btn\" onclick=\"copyCode(this)\">Copy</button>")
		}
	}

	html.WriteString("</div>")
	return html.String()
}

// ManHandler handles man:{page} queries
type ManHandler struct {
	client *http.Client
}

// NewManHandler creates a new man page handler
func NewManHandler() *ManHandler {
	return &ManHandler{
		client: &http.Client{Timeout: 15 * time.Second},
	}
}

func (h *ManHandler) Type() AnswerType {
	return AnswerTypeMan
}

func (h *ManHandler) Handle(ctx context.Context, term string) (*Answer, error) {
	term = strings.TrimSpace(term)
	if term == "" {
		return nil, fmt.Errorf("man page name required")
	}

	// Parse section if provided (e.g., "printf.3")
	page := term
	section := ""
	if idx := strings.LastIndex(term, "."); idx > 0 {
		potentialSection := term[idx+1:]
		if len(potentialSection) == 1 && potentialSection[0] >= '1' && potentialSection[0] <= '8' {
			page = term[:idx]
			section = potentialSection
		}
	}

	// Fetch from man.cx API
	var apiURL string
	if section != "" {
		apiURL = fmt.Sprintf("https://man.cx/api/doc/%s/%s", url.PathEscape(page), section)
	} else {
		apiURL = fmt.Sprintf("https://man.cx/api/doc/%s", url.PathEscape(page))
	}

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", version.BrowserUserAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := h.client.Do(req)
	if err != nil {
		// Fallback to HTML scraping
		return h.fetchManHTML(ctx, page, section)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return h.fetchManHTML(ctx, page, section)
	}

	var data struct {
		Name        string `json:"name"`
		Section     string `json:"section"`
		Description string `json:"description"`
		Content     string `json:"content"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return h.fetchManHTML(ctx, page, section)
	}

	return &Answer{
		Type:        AnswerTypeMan,
		Term:        term,
		Title:       fmt.Sprintf("man %s(%s)", data.Name, data.Section),
		Description: data.Description,
		Content:     formatManPage(data.Content),
		Source:      "man.cx",
		SourceURL:   fmt.Sprintf("https://man.cx/%s", url.PathEscape(term)),
		Data: map[string]interface{}{
			"name":    data.Name,
			"section": data.Section,
		},
	}, nil
}

func (h *ManHandler) fetchManHTML(ctx context.Context, page, section string) (*Answer, error) {
	// Fallback: fetch HTML from man.cx
	var manURL string
	if section != "" {
		manURL = fmt.Sprintf("https://man.cx/%s(%s)", url.PathEscape(page), section)
	} else {
		manURL = fmt.Sprintf("https://man.cx/%s", url.PathEscape(page))
	}

	req, err := http.NewRequestWithContext(ctx, "GET", manURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", version.BrowserUserAgent)

	resp, err := h.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		term := page
		if section != "" {
			term = fmt.Sprintf("%s.%s", page, section)
		}
		return &Answer{
			Type:        AnswerTypeMan,
			Term:        term,
			Title:       fmt.Sprintf("man %s", page),
			Description: "Manual page not found",
			Content:     fmt.Sprintf("<p>No manual entry for <code>%s</code>.</p>", escapeHTML(page)),
			Error:       "not_found",
		}, nil
	}

	// Read response body
	var content strings.Builder
	buf := make([]byte, 4096)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			content.Write(buf[:n])
		}
		if err != nil {
			break
		}
	}

	term := page
	if section != "" {
		term = fmt.Sprintf("%s.%s", page, section)
	}

	return &Answer{
		Type:        AnswerTypeMan,
		Term:        term,
		Title:       fmt.Sprintf("man %s", page),
		Description: fmt.Sprintf("Manual page for %s", page),
		Content:     extractManContent(content.String()),
		Source:      "man.cx",
		SourceURL:   manURL,
	}, nil
}

// formatManPage formats man page content as HTML
func formatManPage(content string) string {
	// Already HTML from API, just wrap it
	return fmt.Sprintf("<div class=\"man-content\">%s</div>", content)
}

// extractManContent extracts man page content from HTML
func extractManContent(html string) string {
	// Simple extraction - look for the main content div
	// In production, use proper HTML parsing
	start := strings.Index(html, "<pre>")
	end := strings.LastIndex(html, "</pre>")
	if start >= 0 && end > start {
		return fmt.Sprintf("<div class=\"man-content\">%s</div>", html[start:end+6])
	}
	return "<p>Failed to parse man page content.</p>"
}

// CheatHandler handles cheat:{command} queries
type CheatHandler struct {
	client *http.Client
}

// NewCheatHandler creates a new cheat.sh handler
func NewCheatHandler() *CheatHandler {
	return &CheatHandler{
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

func (h *CheatHandler) Type() AnswerType {
	return AnswerTypeCheat
}

func (h *CheatHandler) Handle(ctx context.Context, term string) (*Answer, error) {
	term = strings.TrimSpace(term)
	if term == "" {
		return nil, fmt.Errorf("command name required")
	}

	// Fetch from cheat.sh
	apiURL := fmt.Sprintf("https://cheat.sh/%s?T", url.PathEscape(term))

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "curl/7.64.1") // cheat.sh prefers curl UA

	resp, err := h.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var content strings.Builder
	buf := make([]byte, 4096)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			content.Write(buf[:n])
		}
		if err != nil {
			break
		}
	}

	cheatContent := content.String()

	if strings.Contains(cheatContent, "Unknown topic") {
		return &Answer{
			Type:        AnswerTypeCheat,
			Term:        term,
			Title:       fmt.Sprintf("cheat: %s", term),
			Description: "Command not found",
			Content:     fmt.Sprintf("<p>No cheat sheet found for <code>%s</code>.</p>", escapeHTML(term)),
			Error:       "not_found",
		}, nil
	}

	return &Answer{
		Type:        AnswerTypeCheat,
		Term:        term,
		Title:       fmt.Sprintf("cheat: %s", term),
		Description: fmt.Sprintf("Cheat sheet for %s", term),
		Content:     formatCheatContent(cheatContent),
		Source:      "cheat.sh",
		SourceURL:   fmt.Sprintf("https://cheat.sh/%s", url.PathEscape(term)),
		Data: map[string]interface{}{
			"command": term,
			"raw":     cheatContent,
		},
	}, nil
}

// formatCheatContent converts cheat.sh plain text to HTML
func formatCheatContent(content string) string {
	var html strings.Builder
	html.WriteString("<div class=\"cheat-content\"><pre><code>")
	html.WriteString(escapeHTML(content))
	html.WriteString("</code></pre></div>")
	return html.String()
}

// escapeHTML escapes HTML special characters
func escapeHTML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	s = strings.ReplaceAll(s, "'", "&#39;")
	return s
}
