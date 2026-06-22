package engine

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/apimgr/search/src/model"
)

// ============================================================================
// Baidu parseResults
// ============================================================================

func TestBaiduParseResultsValidHTML(t *testing.T) {
	engine := NewBaidu()

	// Minimal HTML that satisfies all three regexes in one block:
	// baiduMuRe  — mu="url"
	// baiduTitleRe — <h3 class="c-title ...">...</h3>
	// baiduSnipRe  — <div class="c-abstract ...">...</div>
	html := `<div mu="https://example.com/page"><h3 class="c-title">Example <em>Title</em></h3><div class="c-abstract">This is the snippet text.</div></div>`

	results, err := engine.parseResults(html, model.CategoryGeneral)
	if err != nil {
		t.Fatalf("parseResults() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("parseResults() count = %d, want 1", len(results))
	}
	if results[0].URL != "https://example.com/page" {
		t.Errorf("URL = %q, want https://example.com/page", results[0].URL)
	}
	if results[0].Title == "" {
		t.Error("Title should not be empty")
	}
	if results[0].Content == "" {
		t.Error("Content should not be empty")
	}
	if results[0].Engine != "baidu" {
		t.Errorf("Engine = %q, want baidu", results[0].Engine)
	}
	if results[0].Category != model.CategoryGeneral {
		t.Errorf("Category = %q, want general", results[0].Category)
	}
}

func TestBaiduParseResultsNoH3Title(t *testing.T) {
	engine := NewBaidu()

	// mu= present but no h3.c-title → result must be skipped
	html := `<div mu="https://example.com/"><p>Some other content</p></div>`

	results, err := engine.parseResults(html, model.CategoryGeneral)
	if err != nil {
		t.Fatalf("parseResults() error = %v", err)
	}
	if len(results) != 0 {
		t.Errorf("parseResults() count = %d, want 0 (no h3.c-title)", len(results))
	}
}

func TestBaiduParseResultsEmptyHTML(t *testing.T) {
	engine := NewBaidu()

	results, err := engine.parseResults("", model.CategoryGeneral)
	if err != nil {
		t.Errorf("parseResults() error = %v (want nil)", err)
	}
	if results != nil {
		t.Errorf("parseResults() = %v, want nil for empty HTML", results)
	}
}

func TestBaiduParseResultsMaxResultsCap(t *testing.T) {
	engine := NewBaidu()

	// Build more entries than the default maxResults (10)
	var sb strings.Builder
	for i := 0; i < 15; i++ {
		sb.WriteString(fmt.Sprintf(
			`<div mu="https://example%d.com/"><h3 class="c-title">Title %d</h3><div class="c-abstract">Snippet %d</div></div>`,
			i, i, i,
		))
	}

	results, err := engine.parseResults(sb.String(), model.CategoryGeneral)
	if err != nil {
		t.Fatalf("parseResults() error = %v", err)
	}
	maxResults := engine.GetConfig().GetMaxResults()
	if len(results) > maxResults {
		t.Errorf("parseResults() count = %d exceeds maxResults cap %d", len(results), maxResults)
	}
}

func TestBaiduSearchCategoryURLs(t *testing.T) {
	tests := []struct {
		name        string
		category    model.Category
		wantURLPart string
	}{
		{"images", model.CategoryImages, "image.baidu.com"},
		{"news", model.CategoryNews, "news.baidu.com"},
		{"videos", model.CategoryVideos, "v.baidu.com"},
		{"general", model.CategoryGeneral, "www.baidu.com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var capturedURL string
			engine := NewBaidu()
			// urlCapturingTransport records the original URL before any redirect and
			// always responds with a 200 so Search() does not error on the network call.
			engine.client = &http.Client{
				Transport: &urlCapturingTransport{
					captured: &capturedURL,
				},
			}

			ctx := context.Background()
			query := &model.Query{
				Text:     "test",
				Page:     1,
				Category: tt.category,
			}
			// Ignore the result — we only care about which URL was called
			_, _ = engine.Search(ctx, query)

			if !strings.Contains(capturedURL, tt.wantURLPart) {
				t.Errorf("category %s: captured URL %q does not contain %q",
					tt.category, capturedURL, tt.wantURLPart)
			}
		})
	}
}

// ============================================================================
// Bing Search with mock server
// ============================================================================

func TestBingSearchWithMockServer(t *testing.T) {
	tests := []struct {
		name        string
		responseHTML string
		wantCount   int
		wantErr     bool
	}{
		{
			name: "valid results",
			responseHTML: `<li class="b_algo"><h2><a href="https://example.com">Result Title</a></h2><p>Result snippet here</p></li>` +
				`<li class="b_algo"><h2><a href="https://test.org">Second Title</a></h2><p>Second snippet</p></li>`,
			wantCount: 2,
			wantErr:   false,
		},
		{
			name:         "no b_algo results returns ErrNoResults",
			responseHTML: "<html><body><p>Nothing here</p></body></html>",
			wantCount:    0,
			wantErr:      true,
		},
		{
			name:         "non-200 response does not crash",
			responseHTML: "",
			wantCount:    0,
			// Bing.Search does not check status code — parseResults gets empty string → ErrNoResults
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/html")
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(tt.responseHTML))
			}))
			defer server.Close()

			engine := NewBing()
			engine.client = &http.Client{
				Transport: &prefixRewriteTransport{
					prefix: server.URL,
					inner:  server.Client().Transport,
				},
			}

			ctx := context.Background()
			query := &model.Query{Text: "golang", Page: 1, Category: model.CategoryGeneral}
			results, err := engine.Search(ctx, query)

			if tt.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if len(results) != tt.wantCount {
				t.Errorf("result count = %d, want %d", len(results), tt.wantCount)
			}
		})
	}
}

func TestBingSearchPagePaginationURL(t *testing.T) {
	var receivedURLs []string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedURLs = append(receivedURLs, r.URL.RawQuery)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(""))
	}))
	defer server.Close()

	engine := NewBing()
	engine.client = &http.Client{
		Transport: &prefixRewriteTransport{
			prefix: server.URL,
			inner:  server.Client().Transport,
		},
	}

	ctx := context.Background()

	// Page 1 → first=1
	_, _ = engine.Search(ctx, &model.Query{Text: "test", Page: 1, Category: model.CategoryGeneral})
	// Page 2 → first=11
	_, _ = engine.Search(ctx, &model.Query{Text: "test", Page: 2, Category: model.CategoryGeneral})

	if len(receivedURLs) < 2 {
		t.Fatalf("expected 2 requests, got %d", len(receivedURLs))
	}
	if !strings.Contains(receivedURLs[0], "first=1") {
		t.Errorf("page 1 URL %q should contain first=1", receivedURLs[0])
	}
	if !strings.Contains(receivedURLs[1], "first=11") {
		t.Errorf("page 2 URL %q should contain first=11", receivedURLs[1])
	}
}

// ============================================================================
// Brave parseResults
// ============================================================================

func TestBraveParseResultsValidHTMLSingleLine(t *testing.T) {
	engine := NewBrave()

	// The resultPattern has no (?s) flag so it matches single-line div only.
	// We craft a single-line snippet block that satisfies both titlePattern and descPattern.
	html := `<div class="snippet"><a class="result-header" href="https://brave-example.com"><span class="snippet-title">Brave Result</span></a><p class="snippet-description">This is the description.</p></div>`

	results, err := engine.parseResults(html, model.CategoryGeneral)
	if err != nil {
		t.Fatalf("parseResults() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("parseResults() count = %d, want 1", len(results))
	}
	if results[0].URL != "https://brave-example.com" {
		t.Errorf("URL = %q, want https://brave-example.com", results[0].URL)
	}
	if results[0].Title != "Brave Result" {
		t.Errorf("Title = %q, want Brave Result", results[0].Title)
	}
	if results[0].Content != "This is the description." {
		t.Errorf("Content = %q, want 'This is the description.'", results[0].Content)
	}
}

func TestBraveParseResultsEmptyURLOrTitleSkipped(t *testing.T) {
	engine := NewBrave()

	tests := []struct {
		name string
		html string
	}{
		{
			// href is present but title span is missing → titlePattern won't match
			name: "missing title span",
			html: `<div class="snippet"><a class="result-header" href="https://example.com"><b>no span</b></a></div>`,
		},
		{
			// title pattern matches but empty href
			name: "empty href",
			html: `<div class="snippet"><a class="result-header" href=""><span class="snippet-title">Title</span></a><p class="snippet-description">desc</p></div>`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, err := engine.parseResults(tt.html, model.CategoryGeneral)
			if err != nil {
				t.Errorf("parseResults() error = %v", err)
			}
			if len(results) != 0 {
				t.Errorf("parseResults() count = %d, want 0 for %s", len(results), tt.name)
			}
		})
	}
}

func TestBraveParseResultsHTMLEntitiesDecoded(t *testing.T) {
	engine := NewBrave()

	html := `<div class="snippet"><a class="result-header" href="https://example.com"><span class="snippet-title">Hello &amp; World</span></a><p class="snippet-description">Fish &amp; Chips &lt;tasty&gt;</p></div>`

	results, err := engine.parseResults(html, model.CategoryGeneral)
	if err != nil {
		t.Fatalf("parseResults() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("parseResults() count = %d, want 1", len(results))
	}
	if results[0].Title != "Hello & World" {
		t.Errorf("Title = %q, want 'Hello & World'", results[0].Title)
	}
	if results[0].Content != "Fish & Chips <tasty>" {
		t.Errorf("Content = %q, want 'Fish & Chips <tasty>'", results[0].Content)
	}
}

func TestBraveParseResultsEmptyHTML(t *testing.T) {
	engine := NewBrave()

	results, err := engine.parseResults("", model.CategoryGeneral)
	if err != nil {
		t.Errorf("parseResults() error = %v (want nil)", err)
	}
	if len(results) != 0 {
		t.Errorf("parseResults() count = %d, want 0", len(results))
	}
}

func TestBraveParseResultsMaxResultsCap(t *testing.T) {
	engine := NewBrave()

	// Build more single-line snippets than the default maxResults (10)
	var sb strings.Builder
	for i := 0; i < 15; i++ {
		sb.WriteString(fmt.Sprintf(
			`<div class="snippet"><a class="result-header" href="https://example%d.com"><span class="snippet-title">Title %d</span></a><p class="snippet-description">Desc %d</p></div>`,
			i, i, i,
		))
	}

	results, err := engine.parseResults(sb.String(), model.CategoryGeneral)
	if err != nil {
		t.Fatalf("parseResults() error = %v", err)
	}
	maxResults := engine.GetConfig().GetMaxResults()
	if len(results) > maxResults {
		t.Errorf("parseResults() count = %d exceeds maxResults cap %d", len(results), maxResults)
	}
}

// ============================================================================
// Mojeek parseResults
// ============================================================================

func TestMojeekParseResultsValidHTML(t *testing.T) {
	engine := NewMojeek()

	// The resultPattern has no (?s) flag — keep it single-line.
	html := `<li class="results-standard"><a class="title" href="https://mojeek-example.com">Mojeek Result</a><p class="s">Snippet text here</p><p class="u">mojeek-example.com</p></li>`

	results, err := engine.parseResults(html, model.CategoryGeneral)
	if err != nil {
		t.Fatalf("parseResults() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("parseResults() count = %d, want 1", len(results))
	}
	if results[0].URL != "https://mojeek-example.com" {
		t.Errorf("URL = %q, want https://mojeek-example.com", results[0].URL)
	}
	if results[0].Title != "Mojeek Result" {
		t.Errorf("Title = %q, want 'Mojeek Result'", results[0].Title)
	}
	if results[0].Content != "Snippet text here" {
		t.Errorf("Content = %q, want 'Snippet text here'", results[0].Content)
	}
}

func TestMojeekParseResultsFallbackToDisplayURL(t *testing.T) {
	engine := NewMojeek()

	// href is empty → must fall back to class="u" p tag; no http prefix → prepend https://
	html := `<li class="results-standard"><a class="title" href="">Fallback Title</a><p class="s">desc</p><p class="u">fallback-url.com/page</p></li>`

	results, err := engine.parseResults(html, model.CategoryGeneral)
	if err != nil {
		t.Fatalf("parseResults() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("parseResults() count = %d, want 1", len(results))
	}
	if !strings.HasPrefix(results[0].URL, "https://") {
		t.Errorf("URL = %q, want https:// prefix when href was empty", results[0].URL)
	}
}

func TestMojeekParseResultsFallbackURLAlreadyHTTPS(t *testing.T) {
	engine := NewMojeek()

	// href empty, display URL already starts with https:// — must not double-prefix
	html := `<li class="results-standard"><a class="title" href="">Title</a><p class="s">desc</p><p class="u">https://already-prefixed.com</p></li>`

	results, err := engine.parseResults(html, model.CategoryGeneral)
	if err != nil {
		t.Fatalf("parseResults() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("parseResults() count = %d, want 1", len(results))
	}
	if strings.HasPrefix(results[0].URL, "https://https://") {
		t.Errorf("URL = %q, double https:// prefix applied incorrectly", results[0].URL)
	}
}

func TestMojeekParseResultsMissingTitle(t *testing.T) {
	engine := NewMojeek()

	// titlePattern won't match if there's no class="title" anchor → skipped
	html := `<li class="results-standard"><p class="s">Snippet without a title link</p></li>`

	results, err := engine.parseResults(html, model.CategoryGeneral)
	if err != nil {
		t.Errorf("parseResults() error = %v", err)
	}
	if len(results) != 0 {
		t.Errorf("parseResults() count = %d, want 0 (no title)", len(results))
	}
}

func TestMojeekParseResultsEmptyHTML(t *testing.T) {
	engine := NewMojeek()

	results, err := engine.parseResults("", model.CategoryGeneral)
	if err != nil {
		t.Errorf("parseResults() error = %v (want nil)", err)
	}
	if len(results) != 0 {
		t.Errorf("parseResults() count = %d, want 0", len(results))
	}
}

func TestMojeekSearchParamsWithMockServer(t *testing.T) {
	tests := []struct {
		name       string
		query      *model.Query
		wantParams []string
	}{
		{
			name:       "safe search off",
			query:      &model.Query{Text: "test", Page: 1, SafeSearch: 0},
			wantParams: []string{"safe=0"},
		},
		{
			name:       "safe search strict",
			query:      &model.Query{Text: "test", Page: 1, SafeSearch: 2},
			wantParams: []string{"safe=1"},
		},
		{
			name:       "with language",
			query:      &model.Query{Text: "test", Page: 1, Language: "fr"},
			wantParams: []string{"lb=fr"},
		},
		{
			name:       "pagination page 3",
			query:      &model.Query{Text: "test", Page: 3},
			wantParams: []string{"s=21"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var capturedQuery string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				capturedQuery = r.URL.RawQuery
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("<html></html>"))
			}))
			defer server.Close()

			engine := NewMojeek()
			engine.client = &http.Client{
				Transport: &prefixRewriteTransport{
					prefix: server.URL,
					inner:  server.Client().Transport,
				},
			}

			ctx := context.Background()
			_, _ = engine.Search(ctx, tt.query)

			for _, param := range tt.wantParams {
				if !strings.Contains(capturedQuery, param) {
					t.Errorf("query string %q does not contain %q", capturedQuery, param)
				}
			}
		})
	}
}

// ============================================================================
// unescapeHTML (already partially tested in engines_test.go, but we verify
// the full entity set explicitly)
// ============================================================================

func TestUnescapeHTMLAllEntities(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"amp", "&amp;", "&"},
		{"lt", "&lt;", "<"},
		{"gt", "&gt;", ">"},
		{"quot", "&quot;", `"`},
		{"apos", "&#39;", "'"},
		{"nbsp", "&nbsp;", " "},
		{"plain", "no entities", "no entities"},
		{"combined", "a&amp;b&lt;c&gt;d", "a&b<c>d"},
		{"empty", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := unescapeHTML(tt.input)
			if got != tt.want {
				t.Errorf("unescapeHTML(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// ============================================================================
// Test transport helpers
// ============================================================================

// prefixRewriteTransport rewrites every request's host to a fixed mock-server
// address.  This lets Bing/Mojeek/Brave (single hostname) be exercised without
// live network access.
type prefixRewriteTransport struct {
	prefix string
	inner  http.RoundTripper
}

func (t *prefixRewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	cloned := req.Clone(req.Context())
	// Rewrite scheme+host to the mock server prefix
	cloned.URL.Scheme = "http"
	cloned.URL.Host = strings.TrimPrefix(t.prefix, "http://")
	cloned.Host = cloned.URL.Host
	return t.inner.RoundTrip(cloned)
}

// urlCapturingTransport records the requested URL and returns an empty 200 OK
// response.  It never makes a real network call, which avoids flakiness when
// verifying URL-construction logic (e.g. Baidu's per-category hostname).
type urlCapturingTransport struct {
	captured *string
}

func (t *urlCapturingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	*t.captured = req.URL.String()
	// Return a minimal 200 response so the caller's body-reading code does not panic.
	recorder := httptest.NewRecorder()
	recorder.WriteHeader(http.StatusOK)
	recorder.Body.WriteString("<html></html>")
	return recorder.Result(), nil
}
