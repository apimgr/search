package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/apimgr/search/src/model"
)

// ---------- nojs.go: safeHref ----------

// TestSafeHref covers allowed schemes, disallowed schemes, and malformed input.
func TestSafeHref(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"http allowed", "http://example.com/page", "http://example.com/page"},
		{"https allowed", "https://example.com/page", "https://example.com/page"},
		{"ftp allowed", "ftp://example.com/file", "ftp://example.com/file"},
		{"ftps allowed", "ftps://example.com/file", "ftps://example.com/file"},
		{"mailto allowed", "mailto:test@example.com", "mailto:test@example.com"},
		{"javascript blocked", "javascript:alert(1)", "#"},
		{"data uri blocked", "data:text/html,<script>alert(1)</script>", "#"},
		{"empty string", "", "#"},
		{"whitespace trimmed", "  https://example.com  ", "https://example.com"},
		{"relative path blocked (no scheme)", "/relative/path", "#"},
		{"malformed url", "http://[::1", "#"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := safeHref(tt.input)
			if got != tt.want {
				t.Errorf("safeHref(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestSafeHref_EscapesHTML confirms the returned value is HTML-escaped.
func TestSafeHref_EscapesHTML(t *testing.T) {
	got := safeHref(`https://example.com/?a=1&b=2`)
	if !strings.Contains(got, "&amp;") {
		t.Errorf("safeHref() = %q, want escaped ampersand", got)
	}
}

// ---------- nojs.go: htmlQueryEscape / isUnreservedQueryChar / percentEncode ----------

// TestHtmlQueryEscape covers spaces, reserved characters, and unreserved characters.
func TestHtmlQueryEscape(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"empty", "", ""},
		{"plain word", "hello", "hello"},
		{"space becomes plus", "hello world", "hello+world"},
		{"unreserved chars kept", "abc-XYZ_012.~", "abc-XYZ_012.~"},
		{"ampersand percent-encoded", "a&b", "a%26b"},
		{"unicode percent-encoded", "café", "caf%C3%A9"},
		{"slash percent-encoded", "a/b", "a%2Fb"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := htmlQueryEscape(tt.input)
			if got != tt.want {
				t.Errorf("htmlQueryEscape(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestIsUnreservedQueryChar covers letters, digits, marks, and punctuation.
func TestIsUnreservedQueryChar(t *testing.T) {
	tests := []struct {
		r    rune
		want bool
	}{
		{'a', true},
		{'Z', true},
		{'5', true},
		{'-', true},
		{'_', true},
		{'.', true},
		{'~', true},
		{' ', false},
		{'&', false},
		{'/', false},
		{'?', false},
	}
	for _, tt := range tests {
		t.Run(string(tt.r), func(t *testing.T) {
			got := isUnreservedQueryChar(tt.r)
			if got != tt.want {
				t.Errorf("isUnreservedQueryChar(%q) = %v, want %v", tt.r, got, tt.want)
			}
		})
	}
}

// TestPercentEncode covers ASCII and multi-byte UTF-8 runes.
func TestPercentEncode(t *testing.T) {
	tests := []struct {
		r    rune
		want string
	}{
		{'&', "%26"},
		{' ', "%20"},
		{'/', "%2F"},
		{'é', "%C3%A9"},
	}
	for _, tt := range tests {
		t.Run(string(tt.r), func(t *testing.T) {
			got := percentEncode(tt.r)
			if got != tt.want {
				t.Errorf("percentEncode(%q) = %q, want %q", tt.r, got, tt.want)
			}
		})
	}
}

// ---------- nojs.go: itoa ----------

// TestItoa covers zero, positive, negative, and multi-digit values.
func TestItoa(t *testing.T) {
	tests := []struct {
		n    int
		want string
	}{
		{0, "0"},
		{1, "1"},
		{-1, "-1"},
		{42, "42"},
		{-42, "-42"},
		{1000000, "1000000"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := itoa(tt.n)
			if got != tt.want {
				t.Errorf("itoa(%d) = %q, want %q", tt.n, got, tt.want)
			}
		})
	}
}

// ---------- nojs.go: noJSMinimalCSS ----------

// TestNoJSMinimalCSS confirms a non-empty style block is returned.
func TestNoJSMinimalCSS(t *testing.T) {
	css := noJSMinimalCSS()
	if !strings.Contains(css, "<style>") || !strings.Contains(css, "</style>") {
		t.Errorf("noJSMinimalCSS() missing <style> tags: %q", css)
	}
}

// ---------- nojs.go: renderJSONSearchResults ----------

// TestRenderJSONSearchResults confirms JSON body wraps results in the canonical envelope.
func TestRenderJSONSearchResults(t *testing.T) {
	s := newTestServer(t)
	results := &model.SearchResults{
		Query:        "golang",
		TotalResults: 2,
		Results: []model.Result{
			{Title: "Go", URL: "https://go.dev"},
		},
	}
	rec := httptest.NewRecorder()
	s.renderJSONSearchResults(rec, results)

	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"ok":true`) {
		t.Errorf("body missing ok:true; got %s", body)
	}
	if !strings.Contains(body, "golang") {
		t.Errorf("body missing query; got %s", body)
	}
}

// ---------- nojs.go: renderNoJSSearch ----------

// TestRenderNoJSSearch_NoResults covers the empty results branch.
func TestRenderNoJSSearch_NoResults(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/search?q=nothing", nil)
	rec := httptest.NewRecorder()

	data := &SearchPageData{
		PageData: PageData{Lang: "en"},
		Query:    "nothing",
		Category: "general",
		Results:  []model.Result{},
	}
	s.renderNoJSSearch(rec, req, data)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "<!DOCTYPE html>") {
		t.Error("expected DOCTYPE html in no-js search output")
	}
	if !strings.Contains(body, `name="q"`) {
		t.Error("expected search input field")
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Errorf("Content-Type = %q, want text/html", ct)
	}
}

// TestRenderNoJSSearch_WithResultsAndPagination covers the results and pagination branches.
func TestRenderNoJSSearch_WithResultsAndPagination(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/search?q=go&page=2", nil)
	rec := httptest.NewRecorder()

	data := &SearchPageData{
		PageData:     PageData{Lang: "en"},
		Query:        "go",
		Category:     "general",
		TotalResults: 3,
		Results: []model.Result{
			{Title: "Go Language", URL: "https://go.dev", Domain: "go.dev", Content: "The Go programming language"},
		},
		Pagination: &Pagination{
			CurrentPage: 2,
			TotalPages:  3,
			HasPrev:     true,
			HasNext:     true,
			PrevPage:    1,
			NextPage:    3,
			Pages:       []int{1, 2, 3},
		},
	}
	s.renderNoJSSearch(rec, req, data)

	body := rec.Body.String()
	if !strings.Contains(body, "Go Language") {
		t.Error("expected result title in output")
	}
	if !strings.Contains(body, `rel="prev"`) {
		t.Error("expected prev pagination link")
	}
	if !strings.Contains(body, `rel="next"`) {
		t.Error("expected next pagination link")
	}
	if !strings.Contains(body, `aria-current="page"`) {
		t.Error("expected current page marker")
	}
}

// TestRenderNoJSSearch_QueryTitleEscaped confirms the query is HTML-escaped in the title.
func TestRenderNoJSSearch_QueryTitleEscaped(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/search?q=%3Cscript%3E", nil)
	rec := httptest.NewRecorder()

	data := &SearchPageData{
		PageData: PageData{Lang: "en"},
		Query:    "<script>",
		Category: "general",
		Results:  []model.Result{},
	}
	s.renderNoJSSearch(rec, req, data)

	body := rec.Body.String()
	if strings.Contains(body, "<script>alert") {
		t.Error("unescaped script tag present in output")
	}
	if !strings.Contains(body, "&lt;script&gt;") {
		t.Error("expected query to be HTML-escaped in title/body")
	}
}

// ---------- nojs.go: renderNoJSHome ----------

// TestRenderNoJSHome_Basic confirms the home page renders with title and search form.
func TestRenderNoJSHome_Basic(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	data := &PageData{Lang: "en"}
	s.renderNoJSHome(rec, req, data)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "<!DOCTYPE html>") {
		t.Error("expected DOCTYPE html in no-js home output")
	}
	if !strings.Contains(body, `action="/search"`) {
		t.Error("expected search form action")
	}
	if !strings.Contains(body, `href="/about"`) {
		t.Error("expected about link in footer")
	}
}

// TestRenderNoJSHome_TaglineRendered confirms the tagline appears when configured.
func TestRenderNoJSHome_TaglineRendered(t *testing.T) {
	s := newTestServer(t)
	origTagline := s.config.Server.Branding.Tagline
	s.config.Server.Branding.Tagline = "Search without JavaScript"
	t.Cleanup(func() { s.config.Server.Branding.Tagline = origTagline })

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	s.renderNoJSHome(rec, req, &PageData{Lang: "en"})

	body := rec.Body.String()
	if !strings.Contains(body, "Search without JavaScript") {
		t.Error("expected tagline text in no-js home output")
	}
}

// ---------- nojs.go: renderHTMLToText ----------

// TestRenderHTMLToText_UnknownTemplateFallsBack confirms a missing template produces
// a plain-text error body instead of panicking or leaving an empty response.
func TestRenderHTMLToText_UnknownTemplateFallsBack(t *testing.T) {
	s := newTestServer(t)
	rec := httptest.NewRecorder()
	s.renderHTMLToText(rec, "definitely-does-not-exist-template", nil)

	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "text/plain") {
		t.Errorf("Content-Type = %q, want text/plain", ct)
	}
	if rec.Body.Len() == 0 {
		t.Error("expected non-empty fallback body for missing template")
	}
}
