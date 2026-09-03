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

	"github.com/apimgr/search/src/common/i18n"
	"github.com/apimgr/search/src/version"
	"golang.org/x/net/html"
)

// AnswerTypeMan is the answer type for man page lookups
const AnswerTypeMan AnswerType = "man"

// ManHandler handles man page lookups using man.cx API
type ManHandler struct {
	client   *http.Client
	patterns []*regexp.Regexp
}

// NewManHandler creates a new man page handler
func NewManHandler() *ManHandler {
	return &ManHandler{
		client: &http.Client{Timeout: 15 * time.Second},
		patterns: []*regexp.Regexp{
			// man:command or man:section:command
			regexp.MustCompile(`(?i)^man[:\s]+(\d)?[:\s]*([a-zA-Z0-9_.-]+)$`),
			regexp.MustCompile(`(?i)^man[:\s]+([a-zA-Z0-9_.-]+)$`),
			regexp.MustCompile(`(?i)^manpage[:\s]+([a-zA-Z0-9_.-]+)$`),
		},
	}
}

func (h *ManHandler) Name() string               { return "man" }
func (h *ManHandler) Patterns() []*regexp.Regexp { return h.patterns }

func (h *ManHandler) CanHandle(query string) bool {
	for _, p := range h.patterns {
		if p.MatchString(query) {
			return true
		}
	}
	return false
}

func (h *ManHandler) HandleInstantQuery(ctx context.Context, query string) (*Answer, error) {
	section, command := h.parseQuery(query)

	if command == "" {
		return nil, nil
	}

	// Build man.cx URL.
	// man.cx no longer serves a plain-text response via "?f=t" (it now returns the
	// same HTML page regardless of that query parameter), so the page HTML is
	// fetched directly and parsed with an HTML tokenizer instead.
	var manURL string
	if section != "" {
		manURL = fmt.Sprintf("https://man.cx/%s(%s)", url.PathEscape(command), section)
	} else {
		manURL = fmt.Sprintf("https://man.cx/%s", url.PathEscape(command))
	}

	req, err := http.NewRequestWithContext(ctx, "GET", manURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", version.BrowserUserAgent)

	lang := LangFromContext(ctx)

	resp, err := h.client.Do(req)
	if err != nil {
		return h.errorAnswer(query, command, i18n.T(lang, "instant.man_connect_failed")), nil
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return h.errorAnswer(query, command, i18n.T(lang, "instant.man_not_found", command)), nil
	}

	if resp.StatusCode != http.StatusOK {
		return h.errorAnswer(query, command, i18n.T(lang, "instant.man_bad_status", resp.StatusCode)), nil
	}

	// Read content (limit to prevent huge pages)
	limitedReader := io.LimitReader(resp.Body, 64*1024)
	contentBytes, err := io.ReadAll(limitedReader)
	if err != nil {
		return h.errorAnswer(query, command, i18n.T(lang, "instant.man_read_failed")), nil
	}

	rawContent := string(contentBytes)

	// Parse the man page content
	manInfo := h.parseManPageHTML(rawContent)
	manInfo.Command = command
	manInfo.Section = section

	// man.cx returns HTTP 200 with a "Search results for ..." page (no NAME/DESCRIPTION
	// sections) when the command has no man page, so absence of both is the not-found signal.
	if manInfo.Name == "" && manInfo.Description == "" {
		return h.errorAnswer(query, command, i18n.T(lang, "instant.man_not_found", command)), nil
	}

	// Build HTML content
	var content strings.Builder
	content.WriteString(fmt.Sprintf("<strong>%s", escapeHTML(command)))
	if section != "" {
		content.WriteString(fmt.Sprintf("(%s)", section))
	}
	content.WriteString("</strong><br><br>")

	if manInfo.Name != "" {
		content.WriteString(fmt.Sprintf("<strong>NAME</strong><br>%s<br><br>", escapeHTML(manInfo.Name)))
	}

	if manInfo.Synopsis != "" {
		content.WriteString(fmt.Sprintf("<strong>SYNOPSIS</strong><br><code>%s</code><br><br>", escapeHTML(manInfo.Synopsis)))
	}

	if manInfo.Description != "" {
		// Limit description length
		desc := manInfo.Description
		if len(desc) > 800 {
			desc = desc[:800] + "..."
		}
		content.WriteString(fmt.Sprintf("<strong>DESCRIPTION</strong><br>%s<br><br>", escapeHTML(desc)))
	}

	// Show a few common options if available
	if len(manInfo.Options) > 0 {
		content.WriteString("<strong>COMMON OPTIONS</strong><br>")
		maxOptions := 5
		if len(manInfo.Options) < maxOptions {
			maxOptions = len(manInfo.Options)
		}
		for i := 0; i < maxOptions; i++ {
			opt := manInfo.Options[i]
			content.WriteString(fmt.Sprintf("<code>%s</code> - %s<br>", escapeHTML(opt.Flag), escapeHTML(opt.Description)))
		}
		if len(manInfo.Options) > 5 {
			content.WriteString(fmt.Sprintf("<em>... and %d more options</em><br>", len(manInfo.Options)-5))
		}
		content.WriteString("<br>")
	}

	// See also
	if len(manInfo.SeeAlso) > 0 {
		content.WriteString(fmt.Sprintf("<strong>SEE ALSO</strong><br>%s<br><br>", escapeHTML(strings.Join(manInfo.SeeAlso, ", "))))
	}

	// Links
	content.WriteString("<strong>Links:</strong><br>")
	content.WriteString(fmt.Sprintf("&bull; <a href=\"%s\" target=\"_blank\">View Full Man Page (man.cx)</a><br>", manURL))
	content.WriteString(fmt.Sprintf("&bull; <a href=\"https://linux.die.net/man/1/%s\" target=\"_blank\">linux.die.net</a><br>", url.PathEscape(command)))

	return &Answer{
		Type:      AnswerTypeMan,
		Query:     query,
		Title:     fmt.Sprintf("man %s", command),
		Content:   content.String(),
		Source:    "man.cx",
		SourceURL: manURL,
		Data: map[string]interface{}{
			"command":     command,
			"section":     section,
			"name":        manInfo.Name,
			"synopsis":    manInfo.Synopsis,
			"description": manInfo.Description,
		},
	}, nil
}

func (h *ManHandler) parseQuery(query string) (section, command string) {
	// Try pattern with section first
	sectionPattern := regexp.MustCompile(`(?i)^man[:\s]+(\d)[:\s]+([a-zA-Z0-9_.-]+)$`)
	if matches := sectionPattern.FindStringSubmatch(query); len(matches) == 3 {
		return matches[1], matches[2]
	}

	// Try simple pattern
	simplePattern := regexp.MustCompile(`(?i)^(?:man|manpage)[:\s]+([a-zA-Z0-9_.-]+)$`)
	if matches := simplePattern.FindStringSubmatch(query); len(matches) == 2 {
		return "", matches[1]
	}

	return "", ""
}

type ManPageInfo struct {
	Command     string
	Section     string
	Name        string
	Synopsis    string
	Description string
	Options     []ManOption
	SeeAlso     []string
}

type ManOption struct {
	Flag        string
	Description string
}

// parseManPageHTML extracts man page sections from man.cx's HTML page.
// man.cx renders each section as an <h2> heading followed by block-level
// content (<p>, <table>, <dl>, ...) inside the page's <main> element; the
// same markup also appears (duplicated) in an <aside> navigation list, so
// only the <main> subtree is walked to avoid picking up nav-list text.
func (h *ManHandler) parseManPageHTML(rawHTML string) ManPageInfo {
	info := ManPageInfo{}

	doc, err := html.Parse(strings.NewReader(rawHTML))
	if err != nil {
		return info
	}

	root := findFirstElement(doc, "main")
	if root == nil {
		root = doc
	}

	currentSection := ""
	var sectionContent []string

	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "h2":
				h.saveSectionContent(&info, currentSection, sectionContent)
				currentSection = strings.ToUpper(strings.TrimSpace(manTextContent(n)))
				sectionContent = nil
				return
			case "script", "style", "form", "nav":
				return
			case "p", "table", "dl", "ul", "ol", "pre":
				if currentSection != "" {
					text := strings.TrimSpace(manTextContent(n))
					if text != "" {
						sectionContent = append(sectionContent, text)
					}
				}
				return
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(root)

	// Save last section
	h.saveSectionContent(&info, currentSection, sectionContent)

	return info
}

// findFirstElement returns the first descendant element node with the given tag name.
func findFirstElement(n *html.Node, tag string) *html.Node {
	if n.Type == html.ElementNode && n.Data == tag {
		return n
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if found := findFirstElement(c, tag); found != nil {
			return found
		}
	}
	return nil
}

// manTextContent returns the whitespace-normalized text content of a node and its descendants.
func manTextContent(n *html.Node) string {
	var sb strings.Builder
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.TextNode {
			sb.WriteString(node.Data)
			sb.WriteString(" ")
			return
		}
		if node.Type == html.ElementNode && (node.Data == "script" || node.Data == "style") {
			return
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return strings.Join(strings.Fields(sb.String()), " ")
}

func (h *ManHandler) saveSectionContent(info *ManPageInfo, section string, content []string) {
	if len(content) == 0 {
		return
	}

	joined := strings.Join(content, " ")

	switch section {
	case "NAME":
		info.Name = joined
	case "SYNOPSIS":
		info.Synopsis = strings.Join(content, "\n")
	case "DESCRIPTION":
		info.Description = joined
	case "OPTIONS":
		info.Options = h.parseOptions(content)
	case "SEE ALSO":
		// Parse "cmd(1), cmd2(1)" format
		info.SeeAlso = parseSeealso(joined)
	}
}

func (h *ManHandler) parseOptions(lines []string) []ManOption {
	var options []ManOption
	var currentFlag string
	var currentDesc []string

	for _, line := range lines {
		// Check if line starts with a flag (-x, --xxx)
		if strings.HasPrefix(line, "-") {
			// Save previous option
			if currentFlag != "" {
				options = append(options, ManOption{
					Flag:        currentFlag,
					Description: strings.Join(currentDesc, " "),
				})
			}

			// Parse new flag
			parts := strings.SplitN(line, " ", 2)
			currentFlag = parts[0]
			if len(parts) > 1 {
				currentDesc = []string{strings.TrimSpace(parts[1])}
			} else {
				currentDesc = nil
			}
		} else if currentFlag != "" {
			// Continue description
			currentDesc = append(currentDesc, line)
		}
	}

	// Save last option
	if currentFlag != "" {
		options = append(options, ManOption{
			Flag:        currentFlag,
			Description: strings.Join(currentDesc, " "),
		})
	}

	return options
}

func (h *ManHandler) errorAnswer(query, command, message string) *Answer {
	return &Answer{
		Type:    AnswerTypeMan,
		Query:   query,
		Title:   fmt.Sprintf("man %s", command),
		Content: fmt.Sprintf("<span class=\"error\">%s</span>", message),
		Source:  "man.cx",
	}
}

func parseSeealso(s string) []string {
	// Parse "cmd(1), cmd2(1), cmd3(8)" format
	var result []string
	pattern := regexp.MustCompile(`([a-zA-Z0-9_.-]+)\(\d+\)`)
	matches := pattern.FindAllStringSubmatch(s, -1)
	for _, m := range matches {
		if len(m) > 1 {
			result = append(result, m[1])
		}
	}
	return result
}
