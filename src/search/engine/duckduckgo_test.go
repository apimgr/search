package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/apimgr/search/src/model"
)

// TestDdgDecodeRedirect verifies that DDG redirect URLs are decoded correctly.
func TestDdgDecodeRedirect(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "protocol-relative DDG redirect with uddg param",
			input: "//duckduckgo.com/l/?uddg=https%3A%2F%2Fgolang.org",
			want:  "https://golang.org",
		},
		{
			name:  "absolute path DDG redirect with uddg param",
			input: "/l/?uddg=https%3A%2F%2Fexample.com",
			want:  "https://example.com",
		},
		{
			name:  "external URL without redirect passes through",
			input: "https://golang.org",
			want:  "https://golang.org",
		},
		{
			name:  "internal DDG URL without uddg returns empty",
			input: "https://duckduckgo.com/home",
			want:  "",
		},
		{
			name:  "invalid URL returns empty",
			input: "://invalid url",
			want:  "",
		},
		{
			name:  "empty string returns empty",
			input: "",
			want:  "",
		},
		{
			name:  "absolute DDG redirect with encoded path and query",
			input: "https://duckduckgo.com/l/?uddg=https%3A%2F%2Fexample.com%2Fpath%3Ffoo%3Dbar",
			want:  "https://example.com/path?foo=bar",
		},
		{
			name:  "protocol-relative non-DDG external host passes through",
			input: "//example.com/page",
			want:  "https://example.com/page",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ddgDecodeRedirect(tt.input)
			if got != tt.want {
				t.Errorf("ddgDecodeRedirect(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestDdgStripHTML verifies that HTML tags and entities are removed or decoded.
func TestDdgStripHTML(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "bold tag stripped",
			input: "<b>Hello</b>",
			want:  "Hello",
		},
		{
			name:  "ampersand entity decoded",
			input: "Search &amp; Results",
			want:  "Search & Results",
		},
		{
			name:  "less-than and greater-than entities decoded",
			input: "&lt;script&gt;",
			want:  "<script>",
		},
		{
			name:  "double-quote entity decoded",
			input: "&quot;quoted&quot;",
			want:  `"quoted"`,
		},
		{
			name:  "non-breaking space entity decoded and trimmed",
			input: "&nbsp;text&nbsp;",
			want:  "text",
		},
		{
			name:  "nested tags stripped",
			input: "<div><span><b>deep</b></span></div>",
			want:  "deep",
		},
		{
			name:  "empty string returns empty",
			input: "",
			want:  "",
		},
		{
			name:  "single-quote hex entity decoded",
			input: "it&#x27;s",
			want:  "it's",
		},
		{
			name:  "forward-slash hex entity decoded",
			input: "path&#x2F;to",
			want:  "path/to",
		},
		{
			name:  "decimal single-quote entity decoded",
			input: "it&#39;s",
			want:  "it's",
		},
		{
			name:  "leading and trailing whitespace trimmed",
			input: "  hello world  ",
			want:  "hello world",
		},
		{
			name:  "tag with attributes stripped",
			input: `<a href="https://example.com" class="result__a">Example</a>`,
			want:  "Example",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ddgStripHTML(tt.input)
			if got != tt.want {
				t.Errorf("ddgStripHTML(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// buildDDGHTML constructs a minimal DDG-style HTML result page for use in
// parseWebResults tests.
func buildDDGHTML(results []struct{ href, title, snippet string }) string {
	var sb strings.Builder
	for _, r := range results {
		sb.WriteString(fmt.Sprintf(
			`<a class="result__a" href="%s">%s</a>`,
			r.href, r.title,
		))
		if r.snippet != "" {
			sb.WriteString(fmt.Sprintf(
				`<div class="result__snippet">%s</div>`,
				r.snippet,
			))
		}
	}
	return sb.String()
}

// TestDuckDuckGoParseWebResults verifies the HTML parser for general search.
func TestDuckDuckGoParseWebResults(t *testing.T) {
	engine := NewDuckDuckGo()

	tests := []struct {
		name       string
		html       string
		wantCount  int
		wantErr    bool
		checkURL   string
		checkTitle string
	}{
		{
			name: "single valid result with DDG redirect URL",
			html: buildDDGHTML([]struct{ href, title, snippet string }{
				{
					href:    "//duckduckgo.com/l/?uddg=https%3A%2F%2Fgolang.org",
					title:   "The Go Programming Language",
					snippet: "Go is an open source language",
				},
			}),
			wantCount:  1,
			checkURL:   "https://golang.org",
			checkTitle: "The Go Programming Language",
		},
		{
			name: "two valid results",
			html: buildDDGHTML([]struct{ href, title, snippet string }{
				{href: "//duckduckgo.com/l/?uddg=https%3A%2F%2Fexample.com", title: "Example", snippet: "desc one"},
				{href: "//duckduckgo.com/l/?uddg=https%3A%2F%2Ftest.org", title: "Test", snippet: "desc two"},
			}),
			wantCount: 2,
		},
		{
			name:      "empty HTML returns zero results without error",
			html:      "",
			wantCount: 0,
			wantErr:   false,
		},
		{
			name:      "HTML with no matching class returns zero results",
			html:      "<html><body><a href='https://example.com'>no class</a></body></html>",
			wantCount: 0,
			wantErr:   false,
		},
		{
			name: "result with internal DDG URL is skipped",
			html: buildDDGHTML([]struct{ href, title, snippet string }{
				{href: "https://duckduckgo.com/settings", title: "Settings", snippet: ""},
			}),
			wantCount: 0,
		},
		{
			name: "HTML entities in title decoded",
			html: buildDDGHTML([]struct{ href, title, snippet string }{
				{
					href:    "//duckduckgo.com/l/?uddg=https%3A%2F%2Fexample.com",
					title:   "Hello &amp; World",
					snippet: "",
				},
			}),
			wantCount:  1,
			checkTitle: "Hello & World",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			query := &model.Query{Text: "test", Category: model.CategoryGeneral}
			results, err := engine.parseWebResults(tt.html, query)

			if tt.wantErr && err == nil {
				t.Error("parseWebResults() expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("parseWebResults() unexpected error: %v", err)
			}
			if len(results) != tt.wantCount {
				t.Errorf("parseWebResults() result count = %d, want %d", len(results), tt.wantCount)
			}
			if tt.checkURL != "" && len(results) > 0 {
				if results[0].URL != tt.checkURL {
					t.Errorf("results[0].URL = %q, want %q", results[0].URL, tt.checkURL)
				}
			}
			if tt.checkTitle != "" && len(results) > 0 {
				if results[0].Title != tt.checkTitle {
					t.Errorf("results[0].Title = %q, want %q", results[0].Title, tt.checkTitle)
				}
			}
		})
	}
}

// TestDuckDuckGoParseWebResultsFields verifies that all result struct fields are
// populated correctly for a single result.
func TestDuckDuckGoParseWebResultsFields(t *testing.T) {
	engine := NewDuckDuckGo()
	html := buildDDGHTML([]struct{ href, title, snippet string }{
		{
			href:    "//duckduckgo.com/l/?uddg=https%3A%2F%2Fexample.com%2Fpage",
			title:   "<b>Result Title</b>",
			snippet: "Snippet &amp; content",
		},
	})
	query := &model.Query{Text: "test", Category: model.CategoryGeneral}

	results, err := engine.parseWebResults(html, query)
	if err != nil {
		t.Fatalf("parseWebResults() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("parseWebResults() count = %d, want 1", len(results))
	}

	r := results[0]
	if r.URL != "https://example.com/page" {
		t.Errorf("URL = %q, want https://example.com/page", r.URL)
	}
	if r.Title != "Result Title" {
		t.Errorf("Title = %q, want 'Result Title'", r.Title)
	}
	if r.Content != "Snippet & content" {
		t.Errorf("Content = %q, want 'Snippet & content'", r.Content)
	}
	if r.Engine != "duckduckgo" {
		t.Errorf("Engine = %q, want duckduckgo", r.Engine)
	}
	if r.Category != model.CategoryGeneral {
		t.Errorf("Category = %q, want general", r.Category)
	}
	if r.Score <= 0 {
		t.Errorf("Score = %f, want > 0", r.Score)
	}
}

// TestDuckDuckGoParseWebResultsMaxResults verifies that parseWebResults
// respects the engine's max-results cap.
func TestDuckDuckGoParseWebResultsMaxResults(t *testing.T) {
	engine := NewDuckDuckGo()
	maxResults := engine.GetConfig().GetMaxResults()

	// Build more results than the cap.
	items := make([]struct{ href, title, snippet string }, maxResults+5)
	for i := range items {
		items[i] = struct{ href, title, snippet string }{
			href:    fmt.Sprintf("//duckduckgo.com/l/?uddg=https%%3A%%2F%%2Fexample%d.com", i),
			title:   fmt.Sprintf("Result %d", i),
			snippet: fmt.Sprintf("Snippet %d", i),
		}
	}
	html := buildDDGHTML(items)
	query := &model.Query{Text: "test", Category: model.CategoryGeneral}

	results, err := engine.parseWebResults(html, query)
	if err != nil {
		t.Fatalf("parseWebResults() unexpected error: %v", err)
	}
	if len(results) > maxResults {
		t.Errorf("parseWebResults() returned %d results, want <= %d", len(results), maxResults)
	}
}

// TestDuckDuckGoParseWebResultsScore verifies that score and position fields
// are set correctly and that higher-ranked results have higher scores.
func TestDuckDuckGoParseWebResultsScore(t *testing.T) {
	engine := NewDuckDuckGo()
	html := buildDDGHTML([]struct{ href, title, snippet string }{
		{href: "//duckduckgo.com/l/?uddg=https%3A%2F%2Ffirst.com", title: "First", snippet: ""},
		{href: "//duckduckgo.com/l/?uddg=https%3A%2F%2Fsecond.com", title: "Second", snippet: ""},
	})
	query := &model.Query{Text: "test", Category: model.CategoryGeneral}

	results, err := engine.parseWebResults(html, query)
	if err != nil {
		t.Fatalf("parseWebResults() error = %v", err)
	}
	if len(results) < 2 {
		t.Fatalf("parseWebResults() returned %d results, want 2", len(results))
	}

	if results[0].Position != 0 {
		t.Errorf("results[0].Position = %d, want 0", results[0].Position)
	}
	if results[1].Position != 1 {
		t.Errorf("results[1].Position = %d, want 1", results[1].Position)
	}
	if results[0].Score <= results[1].Score {
		t.Errorf("first result score (%f) should be higher than second (%f)",
			results[0].Score, results[1].Score)
	}
}

// TestDuckDuckGoSearchDispatch verifies that Search() dispatches to the correct
// sub-method for each category without panicking.
// TestDuckDuckGoSearchDispatch verifies Search() routes each category to the
// right handler and returns real parsed results, using ddgFixtureHandler
// (defined in engines_test.go) served over httptest and redirected via
// redirectToServer — no real network calls.
func TestDuckDuckGoSearchDispatch(t *testing.T) {
	server := httptest.NewServer(ddgFixtureHandler())
	defer server.Close()

	categories := []model.Category{
		model.CategoryGeneral,
		model.CategoryImages,
		model.CategoryVideos,
		model.CategoryNews,
	}

	for _, cat := range categories {
		t.Run(string(cat), func(t *testing.T) {
			engine := NewDuckDuckGo()
			engine.client = &http.Client{Transport: redirectToServer(server.URL)}

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			q := &model.Query{Text: "test", Category: cat}
			results, err := engine.Search(ctx, q)
			if err != nil {
				t.Fatalf("Search() error = %v", err)
			}
			if len(results) != 1 {
				t.Fatalf("Search() returned %d results, want 1", len(results))
			}
		})
	}
}

// TestDuckDuckGoSearchGeneralSafeSearchValues exercises all SafeSearch branches
// in searchGeneral (values 0, 1, 2) against a mock server, asserting real results.
func TestDuckDuckGoSearchGeneralSafeSearchValues(t *testing.T) {
	server := httptest.NewServer(ddgFixtureHandler())
	defer server.Close()

	for _, ss := range []int{0, 1, 2} {
		t.Run(fmt.Sprintf("safesearch_%d", ss), func(t *testing.T) {
			engine := NewDuckDuckGo()
			engine.client = &http.Client{Transport: redirectToServer(server.URL)}

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			q := &model.Query{Text: "test", Category: model.CategoryGeneral, SafeSearch: ss}
			results, err := engine.searchGeneral(ctx, q)
			if err != nil {
				t.Fatalf("searchGeneral() error = %v", err)
			}
			if len(results) != 1 {
				t.Fatalf("searchGeneral() returned %d results, want 1", len(results))
			}
		})
	}
}

// TestDuckDuckGoImageFilters exercises image size and type filter branches
// against a mock server, asserting real results for each filter combination.
func TestDuckDuckGoImageFilters(t *testing.T) {
	server := httptest.NewServer(ddgFixtureHandler())
	defer server.Close()

	tests := []struct {
		size      string
		imageType string
	}{
		{"small", "photo"},
		{"medium", "clipart"},
		{"large", "animated"},
		{"xlarge", "unknown"},
		{"unknown", "photo"},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("size=%s_type=%s", tt.size, tt.imageType), func(t *testing.T) {
			engine := NewDuckDuckGo()
			engine.client = &http.Client{Transport: redirectToServer(server.URL)}

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			q := &model.Query{
				Text:      "test",
				Category:  model.CategoryImages,
				ImageSize: tt.size,
				ImageType: tt.imageType,
			}
			results, err := engine.searchImages(ctx, q)
			if err != nil {
				t.Fatalf("searchImages() error = %v", err)
			}
			if len(results) != 1 {
				t.Fatalf("searchImages() returned %d results, want 1", len(results))
			}
		})
	}
}

// TestDuckDuckGoSearchImagesSafeSearch exercises the images safe-search
// branches (values 0, 1, 2) against a mock server, asserting real results.
func TestDuckDuckGoSearchImagesSafeSearch(t *testing.T) {
	server := httptest.NewServer(ddgFixtureHandler())
	defer server.Close()

	for _, ss := range []int{0, 1, 2} {
		t.Run(fmt.Sprintf("safesearch_%d", ss), func(t *testing.T) {
			engine := NewDuckDuckGo()
			engine.client = &http.Client{Transport: redirectToServer(server.URL)}

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			q := &model.Query{Text: "cats", Category: model.CategoryImages, SafeSearch: ss}
			results, err := engine.searchImages(ctx, q)
			if err != nil {
				t.Fatalf("searchImages() error = %v", err)
			}
			if len(results) != 1 {
				t.Fatalf("searchImages() returned %d results, want 1", len(results))
			}
		})
	}
}

// TestDuckDuckGoSearchVideosDurationAndQuality exercises the video duration and
// quality filter branches against a mock server, asserting real results.
func TestDuckDuckGoSearchVideosDurationAndQuality(t *testing.T) {
	server := httptest.NewServer(ddgFixtureHandler())
	defer server.Close()

	tests := []struct {
		name         string
		videoLength  string
		videoQuality string
		safeSearch   int
	}{
		{"short hd safe2", "short", "hd", 2},
		{"medium sd safe0", "medium", "sd", 0},
		{"long hd safe1", "long", "hd", 1},
		{"unknown length", "unknown", "", 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			engine := NewDuckDuckGo()
			engine.client = &http.Client{Transport: redirectToServer(server.URL)}

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			q := &model.Query{
				Text:         "test",
				Category:     model.CategoryVideos,
				VideoLength:  tt.videoLength,
				VideoQuality: tt.videoQuality,
				SafeSearch:   tt.safeSearch,
			}
			results, err := engine.searchVideos(ctx, q)
			if err != nil {
				t.Fatalf("searchVideos() error = %v", err)
			}
			if len(results) != 1 {
				t.Fatalf("searchVideos() returned %d results, want 1", len(results))
			}
		})
	}
}

// TestDuckDuckGoSearchNewsTimeRanges exercises the news time-range branches
// against a mock server, asserting real results.
func TestDuckDuckGoSearchNewsTimeRanges(t *testing.T) {
	server := httptest.NewServer(ddgFixtureHandler())
	defer server.Close()

	timeRanges := []string{"day", "week", "month", "year", ""}

	for _, tr := range timeRanges {
		t.Run("timerange_"+tr, func(t *testing.T) {
			engine := NewDuckDuckGo()
			engine.client = &http.Client{Transport: redirectToServer(server.URL)}

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			q := &model.Query{Text: "test", Category: model.CategoryNews, TimeRange: tr}
			results, err := engine.searchNews(ctx, q)
			if err != nil {
				t.Fatalf("searchNews() error = %v", err)
			}
			if len(results) != 1 {
				t.Fatalf("searchNews() returned %d results, want 1", len(results))
			}
		})
	}
}

// TestDuckDuckGoGetVQDTokenTimeout verifies both VQD extraction paths (double
// and single quote) against a mock server.
func TestDuckDuckGoGetVQDTokenTimeout(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{"double quote", `<html><body>vqd="3-double-quote-token"</body></html>`, "3-double-quote-token"},
		{"single quote", `<html><body>vqd='3-single-quote-token'</body></html>`, "3-single-quote-token"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/html")
				fmt.Fprint(w, tt.body)
			}))
			defer server.Close()

			engine := NewDuckDuckGo()
			engine.client = &http.Client{Transport: redirectToServer(server.URL)}

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			token, err := engine.getVQDToken(ctx, "test query")
			if err != nil {
				t.Fatalf("getVQDToken() error = %v", err)
			}
			if token != tt.want {
				t.Errorf("getVQDToken() = %q, want %q", token, tt.want)
			}
		})
	}
}

// TestDuckDuckGoSearchImagesWithMockVQD exercises the image-search path with a
// mock server that provides both the VQD token and the images JSON response,
// redirected via redirectToServer so the engine's hardcoded target URLs are
// rewritten to the test server. Asserts real parsed results.
func TestDuckDuckGoSearchImagesWithMockVQD(t *testing.T) {
	imgJSON, _ := json.Marshal(map[string]interface{}{
		"results": []map[string]interface{}{
			{
				"title":     "Test Image",
				"url":       "https://example.com",
				"image":     "https://example.com/img.jpg",
				"thumbnail": "https://example.com/thumb.jpg",
				"width":     800,
				"height":    600,
				"source":    "example.com",
			},
		},
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/i.js" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write(imgJSON)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `<html><body>vqd="3-mock-token-abc"</body></html>`)
	}))
	defer server.Close()

	engine := NewDuckDuckGo()
	engine.client = &http.Client{Transport: redirectToServer(server.URL)}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	results, err := engine.searchImages(ctx, &model.Query{
		Text:      "cats",
		Category:  model.CategoryImages,
		ImageSize: "large",
		ImageType: "photo",
	})
	if err != nil {
		t.Fatalf("searchImages() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("searchImages() returned %d results, want 1", len(results))
	}
	if results[0].Title != "Test Image" {
		t.Errorf("Title = %q, want %q", results[0].Title, "Test Image")
	}
}

// TestDuckDuckGoSearchVideosStatusError verifies that a non-200 response from
// the VQD-token request produces an error, using a real mock server (not a
// timeout race) so the failure is deterministic.
func TestDuckDuckGoSearchVideosStatusError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	engine := NewDuckDuckGo()
	engine.client = &http.Client{Transport: redirectToServer(server.URL)}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := engine.searchVideos(ctx, &model.Query{
		Text:     "test",
		Category: model.CategoryVideos,
	})
	if err == nil {
		t.Error("searchVideos() expected error for non-200 response, got nil")
	}
}

// TestDuckDuckGoSearchNewsStatusError verifies that a non-200 response from
// the VQD-token request produces an error, using a real mock server.
func TestDuckDuckGoSearchNewsStatusError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	engine := NewDuckDuckGo()
	engine.client = &http.Client{Transport: redirectToServer(server.URL)}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := engine.searchNews(ctx, &model.Query{
		Text:     "test",
		Category: model.CategoryNews,
	})
	if err == nil {
		t.Error("searchNews() expected error for non-200 response, got nil")
	}
}
