package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/apimgr/search/src/common/httputil"
	"github.com/apimgr/search/src/model"
)

// renderJSONSearchResults encodes search results as a canonical JSON response
// for our CLI client (User-Agent: search-cli/*).
// Per AI.md PART 14: our client is INTERACTIVE, receives JSON, renders its own TUI/GUI.
func (s *Server) renderJSONSearchResults(w http.ResponseWriter, results *model.SearchResults) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	type jsonResponse struct {
		OK   bool                 `json:"ok"`
		Data *model.SearchResults `json:"data"`
	}
	resp := jsonResponse{OK: true, Data: results}
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		slog.Error("nojs: failed to encode JSON search results", "err", err)
	}
}

// renderNoJSSearch renders a complete, JavaScript-free HTML page for text browsers
// (lynx, w3m, links, elinks). All forms use standard GET/POST, all navigation
// via <a href>. No AJAX, no inline scripts, no framework dependencies.
// Per AI.md PART 14: text browsers are INTERACTIVE, receive server-rendered HTML.
func (s *Server) renderNoJSSearch(w http.ResponseWriter, r *http.Request, data *SearchPageData) {
	lang := data.Lang
	im := s.getI18nManager()

	title := im.T(lang, "search.results_title")
	if data.Query != "" {
		title = fmt.Sprintf("%s - %s", html.EscapeString(data.Query), html.EscapeString(s.config.Server.Title))
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	var b strings.Builder
	b.WriteString(`<!DOCTYPE html>` + "\n")
	b.WriteString(`<html lang="` + html.EscapeString(lang) + `">` + "\n")
	b.WriteString("<head>\n")
	b.WriteString(`<meta charset="UTF-8">` + "\n")
	b.WriteString(`<meta name="viewport" content="width=device-width, initial-scale=1.0">` + "\n")
	b.WriteString(`<title>` + title + `</title>` + "\n")
	b.WriteString(noJSMinimalCSS())
	b.WriteString("</head>\n<body>\n")

	// Header with search form
	b.WriteString("<header>\n")
	b.WriteString(`<nav><a href="/">` + html.EscapeString(s.config.Server.Title) + `</a></nav>` + "\n")

	// Search form — GET /search, all controls via standard HTML
	b.WriteString(`<form method="GET" action="/search" role="search">` + "\n")
	b.WriteString(`<label for="q">` + html.EscapeString(im.T(lang, "search.placeholder")) + `</label>` + "\n")
	b.WriteString(`<input id="q" type="search" name="q" value="` + html.EscapeString(data.Query) + `" required autofocus>` + "\n")
	b.WriteString(`<input type="hidden" name="category" value="` + html.EscapeString(data.Category) + `">` + "\n")
	b.WriteString(`<button type="submit">` + html.EscapeString(im.T(lang, "search.button")) + `</button>` + "\n")
	b.WriteString("</form>\n")
	b.WriteString("</header>\n")

	b.WriteString("<main>\n")

	// Category navigation — plain anchor links per AI.md PART 14
	categories := []struct{ key, label string }{
		{"general", im.T(lang, "preferences.default_category_general")},
		{"images", im.T(lang, "search.categories.images")},
		{"videos", im.T(lang, "search.categories.videos")},
		{"news", im.T(lang, "search.categories.news")},
		{"maps", im.T(lang, "search.categories.maps")},
		{"files", im.T(lang, "search.categories.files")},
		{"music", im.T(lang, "preferences.default_category_music")},
		{"science", im.T(lang, "search.categories.science")},
		{"it", im.T(lang, "search.categories.it")},
		{"social", im.T(lang, "search.categories.social")},
	}
	b.WriteString(`<nav aria-label="` + html.EscapeString(im.T(lang, "search.categories_label")) + `">` + "\n<ul>\n")
	for _, cat := range categories {
		href := "/search?q=" + htmlQueryEscape(data.Query) + "&amp;category=" + cat.key
		active := ""
		if cat.key == data.Category {
			active = ` aria-current="page"`
		}
		b.WriteString(`<li><a href="` + href + `"` + active + `>` + html.EscapeString(cat.label) + `</a></li>` + "\n")
	}
	b.WriteString("</ul>\n</nav>\n")

	// Results section
	results, _ := data.Results.([]model.Result)

	if data.TotalResults > 0 {
		resultCountMsg := im.T(lang, "search.result_count_other", data.TotalResults)
		b.WriteString(`<p class="result-count">` + html.EscapeString(resultCountMsg) + `</p>` + "\n")
	}

	if len(results) == 0 {
		noResultsMsg := im.T(lang, "search.no_results_found")
		b.WriteString(`<p class="no-results">` + html.EscapeString(noResultsMsg) + `</p>` + "\n")
	} else {
		b.WriteString("<ol class=\"results\">\n")
		for _, result := range results {
			b.WriteString("<li>\n<article>\n")
			b.WriteString(`<h2><a href="` + safeHref(result.URL) + `" rel="noopener noreferrer">` + html.EscapeString(result.Title) + `</a></h2>` + "\n")
			if result.Domain != "" {
				b.WriteString(`<p class="result-url">` + html.EscapeString(result.Domain) + `</p>` + "\n")
			}
			if result.Content != "" {
				b.WriteString(`<p class="result-snippet">` + html.EscapeString(result.Content) + `</p>` + "\n")
			}
			b.WriteString("</article>\n</li>\n")
		}
		b.WriteString("</ol>\n")
	}

	// Pagination — plain anchor links, no JS
	if data.Pagination != nil && data.Pagination.TotalPages > 1 {
		b.WriteString(`<nav aria-label="` + html.EscapeString(im.T(lang, "search.pagination_label")) + `">` + "\n<ul>\n")
		if data.Pagination.HasPrev {
			href := "/search?q=" + htmlQueryEscape(data.Query) + "&amp;category=" + html.EscapeString(data.Category) + "&amp;page=" + itoa(data.Pagination.PrevPage)
			b.WriteString(`<li><a href="` + href + `" rel="prev">` + html.EscapeString(im.T(lang, "search.prev_page")) + `</a></li>` + "\n")
		}
		for _, p := range data.Pagination.Pages {
			href := "/search?q=" + htmlQueryEscape(data.Query) + "&amp;category=" + html.EscapeString(data.Category) + "&amp;page=" + itoa(p)
			current := ""
			if p == data.Pagination.CurrentPage {
				current = ` aria-current="page"`
			}
			b.WriteString(`<li><a href="` + href + `"` + current + `>` + itoa(p) + `</a></li>` + "\n")
		}
		if data.Pagination.HasNext {
			href := "/search?q=" + htmlQueryEscape(data.Query) + "&amp;category=" + html.EscapeString(data.Category) + "&amp;page=" + itoa(data.Pagination.NextPage)
			b.WriteString(`<li><a href="` + href + `" rel="next">` + html.EscapeString(im.T(lang, "search.next_page")) + `</a></li>` + "\n")
		}
		b.WriteString("</ul>\n</nav>\n")
	}

	b.WriteString("</main>\n")

	// Minimal footer
	b.WriteString("<footer>\n")
	b.WriteString(`<p><a href="/">` + html.EscapeString(s.config.Server.Title) + `</a> &mdash; <a href="/privacy">` + html.EscapeString(im.T(lang, "footer.privacy_policy")) + `</a></p>` + "\n")
	b.WriteString("</footer>\n</body>\n</html>\n")

	if _, err := fmt.Fprint(w, b.String()); err != nil {
		slog.Error("nojs: failed to write no-JS search response", "err", err)
	}
}

// renderNoJSHome renders a JavaScript-free home page for text browsers.
// Per AI.md PART 14: text browsers receive server-rendered HTML, no JS.
func (s *Server) renderNoJSHome(w http.ResponseWriter, r *http.Request, data *PageData) {
	lang := data.Lang
	im := s.getI18nManager()
	title := html.EscapeString(s.config.Server.Title)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	var b strings.Builder
	b.WriteString("<!DOCTYPE html>\n")
	b.WriteString(`<html lang="` + html.EscapeString(lang) + `">` + "\n")
	b.WriteString("<head>\n")
	b.WriteString(`<meta charset="UTF-8">` + "\n")
	b.WriteString(`<meta name="viewport" content="width=device-width, initial-scale=1.0">` + "\n")
	b.WriteString("<title>" + title + "</title>\n")
	b.WriteString(noJSMinimalCSS())
	b.WriteString("</head>\n<body>\n")

	b.WriteString("<main>\n")
	b.WriteString("<h1>" + title + "</h1>\n")
	if s.config.Server.Branding.Tagline != "" {
		b.WriteString(`<p class="tagline">` + html.EscapeString(s.config.Server.Branding.Tagline) + "</p>\n")
	}

	// Search form
	b.WriteString(`<form method="GET" action="/search" role="search">` + "\n")
	b.WriteString(`<label for="q">` + html.EscapeString(im.T(lang, "search.placeholder")) + `</label>` + "\n")
	b.WriteString(`<input id="q" type="search" name="q" required autofocus>` + "\n")
	b.WriteString(`<button type="submit">` + html.EscapeString(im.T(lang, "search.button")) + `</button>` + "\n")
	b.WriteString("</form>\n")
	b.WriteString("</main>\n")

	b.WriteString("<footer>\n")
	b.WriteString(`<ul>` + "\n")
	b.WriteString(`<li><a href="/about">` + html.EscapeString(im.T(lang, "nav.about")) + `</a></li>` + "\n")
	b.WriteString(`<li><a href="/privacy">` + html.EscapeString(im.T(lang, "footer.privacy_policy")) + `</a></li>` + "\n")
	b.WriteString(`<li><a href="/preferences">` + html.EscapeString(im.T(lang, "nav.preferences")) + `</a></li>` + "\n")
	b.WriteString(`</ul>` + "\n")
	b.WriteString("</footer>\n</body>\n</html>\n")

	if _, err := fmt.Fprint(w, b.String()); err != nil {
		slog.Error("nojs: failed to write no-JS home response", "err", err)
	}
}

// renderHTMLToText renders a named template to a buffer and converts the HTML
// to terminal-formatted plain text using HTML2TextConverter.
// Per AI.md PART 14: HTTP tools (curl, wget) receive formatted text, not HTML.
func (s *Server) renderHTMLToText(w http.ResponseWriter, name string, data interface{}) {
	var buf bytes.Buffer
	if err := s.renderer.Render(&buf, name, data); err != nil {
		slog.Warn("nojs: template render failed, using fallback plain text", "template", name, "err", err)
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprintf(w, "Error rendering page: %v\n", err)
		return
	}
	text := httputil.HTML2TextConverter(buf.String(), 80)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if _, err := fmt.Fprint(w, text); err != nil {
		slog.Error("nojs: failed to write plain text response", "err", err)
	}
}

// noJSMinimalCSS returns a minimal embedded stylesheet for no-JS pages.
// Ensures basic readability in graphical text browsers (w3m, links2).
// Dark theme as default per AI.md PART 16; respects prefers-color-scheme.
func noJSMinimalCSS() string {
	return `<style>
:root{color-scheme:dark light;--bg:#1a1a2e;--text:#eaeaea;--link:#7eb8f7;--border:#444}
@media(prefers-color-scheme:light){:root{--bg:#f5f5f5;--text:#1a1a2e;--link:#0050a0;--border:#ccc}}
body{background:var(--bg);color:var(--text);font-family:monospace,sans-serif;max-width:80ch;margin:0 auto;padding:1rem}
a{color:var(--link)}
header,footer{border-top:1px solid var(--border);padding:.5rem 0;margin:.5rem 0}
form{display:flex;flex-wrap:wrap;gap:.5rem;align-items:flex-end}
input[type=search]{flex:1;min-width:12ch;background:var(--bg);color:var(--text);border:1px solid var(--border);padding:.25rem}
button{background:var(--link);color:var(--bg);border:none;padding:.25rem .75rem;cursor:pointer}
nav ul{list-style:none;padding:0;display:flex;flex-wrap:wrap;gap:.5rem}
ol.results{list-style:none;padding:0}
ol.results li{margin-bottom:1.5rem}
.result-url{font-size:.85em;opacity:.7}
.result-snippet{margin:.25rem 0}
h2{margin:.25rem 0;font-size:1em}
[aria-current=page]{font-weight:bold;text-decoration:none}
</style>
`
}

// safeHref validates that a URL uses a safe scheme before use in an href attribute.
// html.EscapeString alone cannot prevent javascript: URI injection; this blocks it.
// Only http, https, ftp, ftps, and mailto are permitted; anything else becomes "#".
func safeHref(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "#"
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https", "ftp", "ftps", "mailto":
		return html.EscapeString(u.String())
	default:
		return "#"
	}
}

// htmlQueryEscape percent-encodes a string for use in an HTML attribute
// query string, using standard URL encoding with HTML entity for &.
func htmlQueryEscape(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == ' ':
			b.WriteByte('+')
		case isUnreservedQueryChar(r):
			b.WriteRune(r)
		default:
			b.WriteString(percentEncode(r))
		}
	}
	return b.String()
}

// isUnreservedQueryChar returns true for characters that need no encoding in query strings.
func isUnreservedQueryChar(r rune) bool {
	return (r >= 'A' && r <= 'Z') ||
		(r >= 'a' && r <= 'z') ||
		(r >= '0' && r <= '9') ||
		r == '-' || r == '_' || r == '.' || r == '~'
}

// percentEncode hex-encodes a single rune for URL query encoding.
func percentEncode(r rune) string {
	b := []byte(string(r))
	var sb strings.Builder
	for _, by := range b {
		sb.WriteString(fmt.Sprintf("%%%02X", by))
	}
	return sb.String()
}

// itoa converts an integer to a decimal string without importing strconv.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	digits := make([]byte, 0, 10)
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	if neg {
		return "-" + string(digits)
	}
	return string(digits)
}

