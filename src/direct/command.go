package direct

import (
	"context"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/apimgr/search/src/common/closeutil"

	"github.com/apimgr/search/src/common/i18n"
	"github.com/apimgr/search/src/version"
	xhtml "golang.org/x/net/html"
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

func (h *TLDRHandler) HandleDirectQuery(ctx context.Context, term string) (*Answer, error) {
	term = strings.TrimSpace(strings.ToLower(term))
	if term == "" {
		return nil, fmt.Errorf("command name required")
	}

	lang := LangFromContext(ctx)

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

		if resp.StatusCode == http.StatusOK {
			content, readErr := io.ReadAll(io.LimitReader(resp.Body, (1<<20)+1))
			closeutil.Close(resp.Body)
			if readErr != nil || len(content) > 1<<20 {
				continue
			}

			// Parse tldr markdown format
			markdown := string(content)
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

		// Close body before trying the next platform to avoid leaking connections
		closeutil.Close(resp.Body)
	}

	return &Answer{
		Type:        AnswerTypeTLDR,
		Term:        term,
		Title:       fmt.Sprintf("tldr: %s", term),
		Description: i18n.T(lang, "direct.command_not_found"),
		Content:     fmt.Sprintf("<p>No tldr page found for <code>%s</code>.</p><p>Try searching for it using <code>man:%s</code> or <code>cheat:%s</code>.</p>", escapeHTML(term), escapeHTML(term), escapeHTML(term)),
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
			fmt.Fprintf(&html, "<h1>%s</h1>", escapeHTML(title))
		} else if strings.HasPrefix(line, "> ") {
			// Description
			desc := strings.TrimPrefix(line, "> ")
			fmt.Fprintf(&html, "<p class=\"description\">%s</p>", escapeHTML(desc))
		} else if strings.HasPrefix(line, "- ") {
			// Example description
			desc := strings.TrimPrefix(line, "- ")
			fmt.Fprintf(&html, "<p class=\"example-desc\">%s</p>", escapeHTML(desc))
		} else if strings.HasPrefix(line, "`") && strings.HasSuffix(line, "`") {
			// Code example
			code := strings.Trim(line, "`")
			fmt.Fprintf(&html, "<pre class=\"example-code\"><code>%s</code></pre>", escapeHTML(code))
			html.WriteString("<button type=\"button\" class=\"copy-btn\">Copy</button>")
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

func (h *ManHandler) HandleDirectQuery(ctx context.Context, term string) (*Answer, error) {
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
	defer closeutil.Close(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return h.fetchManHTML(ctx, page, section)
	}

	var data struct {
		Name        string `json:"name"`
		Section     string `json:"section"`
		Description string `json:"description"`
		Content     string `json:"content"`
	}

	if err := decodeBoundedJSON(resp.Body, 2<<20, &data); err != nil {
		return h.fetchManHTML(ctx, page, section)
	}

	return &Answer{
		Type:        AnswerTypeMan,
		Term:        term,
		Title:       fmt.Sprintf("man %s(%s)", data.Name, data.Section),
		Description: data.Description,
		Content:     formatManPage(data.Content, term),
		Source:      "man.cx",
		SourceURL:   fmt.Sprintf("https://man.cx/%s", url.PathEscape(term)),
		Data: map[string]interface{}{
			"name":    data.Name,
			"section": data.Section,
		},
	}, nil
}

func (h *ManHandler) fetchManHTML(ctx context.Context, page, section string) (*Answer, error) {
	lang := LangFromContext(ctx)

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
	defer closeutil.Close(resp.Body)

	if resp.StatusCode == http.StatusNotFound {
		term := page
		if section != "" {
			term = fmt.Sprintf("%s.%s", page, section)
		}
		return &Answer{
			Type:        AnswerTypeMan,
			Term:        term,
			Title:       fmt.Sprintf("man %s", page),
			Description: i18n.T(lang, "direct.manual_page_not_found"),
			Content:     fmt.Sprintf("<p>No manual entry for <code>%s</code>.</p>", escapeHTML(page)),
			Error:       "not_found",
		}, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("man.cx returned HTTP %d", resp.StatusCode)
	}

	content, err := io.ReadAll(io.LimitReader(resp.Body, (2<<20)+1))
	if err != nil {
		return nil, err
	}
	if len(content) > 2<<20 {
		return nil, fmt.Errorf("man.cx page exceeds maximum size")
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
		Content:     formatManPage(extractManContent(string(content)), term),
		Source:      "man.cx",
		SourceURL:   manURL,
	}, nil
}

// formatManPage formats man page content as HTML
func formatManPage(content string, terms ...string) string {
	safe := sanitizeManHTML(content)
	document, err := xhtml.Parse(strings.NewReader("<main>" + safe + "</main>"))
	if err != nil {
		return "<div class=\"man-content\">" + safe + "</div>"
	}
	var main *xhtml.Node
	var findMain func(*xhtml.Node)
	findMain = func(node *xhtml.Node) {
		if main != nil {
			return
		}
		if node.Type == xhtml.ElementNode && node.Data == "main" {
			main = node
			return
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			findMain(child)
		}
	}
	findMain(document)
	if main == nil {
		return "<div class=\"man-content\">" + safe + "</div>"
	}
	flattenManContainers(main)
	var sections []manSection
	var preamble []*xhtml.Node
	var current *manSection
	for child := main.FirstChild; child != nil; {
		next := child.NextSibling
		if child.Type == xhtml.ElementNode && manHeadingPattern.MatchString(child.Data) {
			title := strings.TrimSpace(manNodeText(child))
			if title != "" {
				id := manSectionID(title, len(sections)+1)
				attrs := child.Attr[:0]
				for _, attr := range child.Attr {
					if attr.Key != "id" {
						attrs = append(attrs, attr)
					}
				}
				child.Attr = append(attrs, xhtml.Attribute{Key: "id", Val: id})
				sections = append(sections, manSection{id: id, title: title})
				current = &sections[len(sections)-1]
			}
		}
		if current != nil {
			detachNode(child)
			current.nodes = append(current.nodes, child)
		} else {
			detachNode(child)
			preamble = append(preamble, child)
		}
		child = next
	}
	if len(sections) == 0 {
		return "<div class=\"man-content\"><label class=\"man-search-label\">Search this page <input type=\"search\" data-man-search></label>" + safe + "</div>"
	}
	var out strings.Builder
	term := ""
	if len(terms) > 0 {
		term = terms[0]
	}
	out.WriteString("<div class=\"man-content\"><label class=\"man-search-label\">Search this page <input type=\"search\" data-man-search></label><nav class=\"man-toc\" aria-label=\"Manual page contents\"><strong>Contents</strong><ol>")
	for _, section := range sections {
		fmt.Fprintf(&out, "<li><a href=\"#%s\">%s</a></li>", html.EscapeString(section.id), html.EscapeString(section.title))
	}
	out.WriteString("</ol></nav>")
	page := term
	if index := strings.LastIndex(page, "."); index > 0 {
		page = page[:index]
	}
	if page != "" {
		out.WriteString("<nav class=\"man-sections\" aria-label=\"Manual section\"><strong>Sections:</strong>")
		for section := 1; section <= 8; section++ {
			query := "man:" + page + "." + fmt.Sprint(section)
			fmt.Fprintf(&out, " <a href=\"/?q=%s\">%d</a>", url.QueryEscape(query), section)
		}
		out.WriteString("</nav>")
	}
	out.WriteString("<div class=\"man-sections-content\">")
	if len(preamble) > 0 {
		out.WriteString("<div class=\"man-preamble\">")
		for _, node := range preamble {
			renderSafeManNode(&out, node)
		}
		out.WriteString("</div>")
	}
	for i, section := range sections {
		if i == 0 {
			out.WriteString("<section class=\"man-section\">")
		} else {
			fmt.Fprintf(&out, "<details class=\"man-section\"><summary id=\"%s\">%s</summary><div class=\"man-section-body\">", html.EscapeString(section.id), html.EscapeString(section.title))
		}
		for j, node := range section.nodes {
			if i > 0 && j == 0 && node.Type == xhtml.ElementNode && manHeadingPattern.MatchString(node.Data) {
				continue
			}
			renderSafeManNode(&out, node)
		}
		if i == 0 {
			out.WriteString("</section>")
		} else {
			out.WriteString("</div></details>")
		}
	}
	out.WriteString("</div></div>")
	return out.String()
}

type manSection struct {
	id, title string
	nodes     []*xhtml.Node
}

func manNodeText(node *xhtml.Node) string {
	if node.Type == xhtml.TextNode {
		return node.Data
	}
	var text strings.Builder
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		text.WriteString(manNodeText(child))
	}
	return text.String()
}

func manSectionID(title string, index int) string {
	var out strings.Builder
	for _, r := range strings.ToLower(title) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			out.WriteRune(r)
		} else if out.Len() > 0 && !strings.HasSuffix(out.String(), "-") {
			out.WriteByte('-')
		}
	}
	id := strings.Trim(out.String(), "-")
	if id == "" {
		id = "section"
	}
	return fmt.Sprintf("man-%s-%d", id, index)
}

func detachNode(node *xhtml.Node) {
	if node.Parent != nil {
		node.Parent.RemoveChild(node)
	}
}

func flattenManContainers(node *xhtml.Node) {
	if node.Type == xhtml.ElementNode && node.Data == "pre" {
		return
	}
	for child := node.FirstChild; child != nil; {
		next := child.NextSibling
		flattenManContainers(child)
		if child.Type == xhtml.ElementNode && child.Data == "div" {
			for nested := child.FirstChild; nested != nil; {
				nextNested := nested.NextSibling
				child.RemoveChild(nested)
				node.InsertBefore(nested, child)
				nested = nextNested
			}
			node.RemoveChild(child)
		}
		child = next
	}
}

// extractManContent extracts man page content from HTML
func extractManContent(rawHTML string) string {
	document, err := xhtml.Parse(strings.NewReader(rawHTML))
	if err != nil {
		return "<p>Failed to parse man page content.</p>"
	}
	var main *xhtml.Node
	var findMain func(*xhtml.Node)
	findMain = func(node *xhtml.Node) {
		if main != nil {
			return
		}
		if node.Type == xhtml.ElementNode && node.Data == "main" {
			main = node
			return
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			findMain(child)
		}
	}
	findMain(document)
	if main != nil {
		var content strings.Builder
		for child := main.FirstChild; child != nil; child = child.NextSibling {
			renderSafeManNode(&content, child)
		}
		return "<div class=\"man-content\">" + content.String() + "</div>"
	}
	return "<p>Failed to parse man page content.</p>"
}

func sanitizeManHTML(rawHTML string) string {
	document, err := xhtml.Parse(strings.NewReader(rawHTML))
	if err != nil {
		return html.EscapeString(rawHTML)
	}
	var output strings.Builder
	var renderBody func(*xhtml.Node)
	renderBody = func(node *xhtml.Node) {
		if node.Type == xhtml.ElementNode && node.Data == "body" {
			for child := node.FirstChild; child != nil; child = child.NextSibling {
				renderSafeManNode(&output, child)
			}
			return
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			renderBody(child)
		}
	}
	renderBody(document)
	return output.String()
}

var safeManElements = map[string]bool{
	"a": true, "b": true, "blockquote": true, "br": true, "caption": true, "code": true,
	"dd": true, "div": true, "dl": true, "dt": true, "em": true, "h1": true, "h2": true,
	"h3": true, "h4": true, "h5": true, "h6": true, "hr": true, "i": true, "li": true,
	"ol": true, "p": true, "pre": true, "span": true, "strong": true, "sub": true, "sup": true,
	"table": true, "tbody": true, "td": true, "th": true, "thead": true, "tr": true, "u": true, "ul": true,
}

var manCodeTokenPattern = regexp.MustCompile(`(?m)(^\s*(?:[$#]\s*)?[A-Za-z_][A-Za-z0-9_.-]*|--?[A-Za-z0-9][A-Za-z0-9-]*|\$[A-Za-z_][A-Za-z0-9_]*|"[^"\n]*"|'[^'\n]*')`)
var manHeadingPattern = regexp.MustCompile(`^h[1-6]$`)

func renderSafeManNode(output *strings.Builder, node *xhtml.Node) {
	switch node.Type {
	case xhtml.TextNode:
		output.WriteString(html.EscapeString(node.Data))
	case xhtml.ElementNode:
		if node.Data == "script" || node.Data == "style" || node.Data == "iframe" || node.Data == "object" || node.Data == "svg" || node.Data == "math" {
			return
		}
		if node.Data == "pre" {
			output.WriteString(`<pre class="man-code-block"><code class="man-code">`)
			output.WriteString(highlightManCode(manNodeText(node)))
			output.WriteString(`</code></pre><button type="button" class="man-copy-button" data-man-copy>Copy</button>`)
			return
		}
		allowed := safeManElements[node.Data]
		if allowed {
			output.WriteByte('<')
			output.WriteString(node.Data)
			for _, attr := range node.Attr {
				value := ""
				keep := false
				switch attr.Key {
				case "class", "id", "title":
					value, keep = attr.Val, true
				case "href":
					parsed, err := url.Parse(strings.TrimSpace(attr.Val))
					if err == nil && parsed.User == nil && (parsed.Scheme == "https" || parsed.Scheme == "http" || strings.HasPrefix(attr.Val, "#")) {
						value, keep = attr.Val, true
					}
				}
				if keep {
					output.WriteByte(' ')
					output.WriteString(attr.Key)
					output.WriteString("=\"")
					output.WriteString(html.EscapeString(value))
					output.WriteByte('"')
				}
			}
			output.WriteByte('>')
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			renderSafeManNode(output, child)
		}
		if allowed && node.Data != "br" && node.Data != "hr" {
			output.WriteString("</")
			output.WriteString(node.Data)
			output.WriteByte('>')
		}
	}
}

func highlightManCode(code string) string {
	var output strings.Builder
	last := 0
	for _, match := range manCodeTokenPattern.FindAllStringIndex(code, -1) {
		output.WriteString(html.EscapeString(code[last:match[0]]))
		token := code[match[0]:match[1]]
		class := "man-token-command"
		if strings.HasPrefix(token, "-") {
			class = "man-token-option"
		} else if strings.HasPrefix(token, "$") {
			class = "man-token-variable"
		} else if strings.HasPrefix(token, "'") || strings.HasPrefix(token, `"`) {
			class = "man-token-string"
		}
		fmt.Fprintf(&output, `<span class="%s">%s</span>`, class, html.EscapeString(token))
		last = match[1]
	}
	output.WriteString(html.EscapeString(code[last:]))
	return output.String()
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

func (h *CheatHandler) HandleDirectQuery(ctx context.Context, term string) (*Answer, error) {
	term = strings.TrimSpace(term)
	if term == "" {
		return nil, fmt.Errorf("command name required")
	}

	lang := LangFromContext(ctx)

	// Fetch from cheat.sh
	apiURL := fmt.Sprintf("https://cheat.sh/%s?T", url.PathEscape(term))

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	// cheat.sh prefers curl UA
	req.Header.Set("User-Agent", "curl/7.64.1")

	resp, err := h.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer closeutil.Close(resp.Body)

	// Treat a missing topic as not_found and any other non-200 as an error
	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusNotFound {
			return &Answer{
				Type:        AnswerTypeCheat,
				Term:        term,
				Title:       fmt.Sprintf("cheat: %s", term),
				Description: i18n.T(lang, "direct.command_not_found"),
				Content:     fmt.Sprintf("<p>No cheat sheet found for <code>%s</code>.</p>", escapeHTML(term)),
				Error:       "not_found",
			}, nil
		}
		return nil, fmt.Errorf("cheat.sh returned status %d", resp.StatusCode)
	}

	content, err := io.ReadAll(io.LimitReader(resp.Body, (1<<20)+1))
	if err != nil {
		return nil, err
	}
	if len(content) > 1<<20 {
		return nil, fmt.Errorf("cheat.sh response exceeds maximum size")
	}

	cheatContent := string(content)

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
