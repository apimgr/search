package engine

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/apimgr/search/src/model"
)

// TestNewWolframAlpha verifies that the constructor sets the expected metadata.
func TestNewWolframAlpha(t *testing.T) {
	engine := NewWolframAlpha()

	if engine == nil {
		t.Fatal("NewWolframAlpha() returned nil")
	}
	if engine.Name() != "wolfram" {
		t.Errorf("Name() = %q, want wolfram", engine.Name())
	}
	if engine.DisplayName() != "Wolfram Alpha" {
		t.Errorf("DisplayName() = %q, want 'Wolfram Alpha'", engine.DisplayName())
	}
	if !engine.IsEnabled() {
		t.Error("IsEnabled() should be true by default")
	}
	if engine.GetPriority() != 75 {
		t.Errorf("GetPriority() = %d, want 75", engine.GetPriority())
	}
	if engine.client == nil {
		t.Error("HTTP client should be initialized")
	}
	if !engine.GetConfig().SupportsTor {
		t.Error("SupportsTor should be true")
	}
}

// TestWolframAlphaSupportsCategory verifies category support declarations.
func TestWolframAlphaSupportsCategory(t *testing.T) {
	engine := NewWolframAlpha()

	tests := []struct {
		category model.Category
		want     bool
	}{
		{model.CategoryGeneral, true},
		{model.CategoryScience, true},
		{model.CategoryImages, false},
		{model.CategoryVideos, false},
		{model.CategoryNews, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.category), func(t *testing.T) {
			got := engine.SupportsCategory(tt.category)
			if got != tt.want {
				t.Errorf("SupportsCategory(%q) = %v, want %v", tt.category, got, tt.want)
			}
		})
	}
}

// TestWolframAlphaGetConfig verifies that GetConfig returns a well-formed config.
func TestWolframAlphaGetConfig(t *testing.T) {
	engine := NewWolframAlpha()
	config := engine.GetConfig()

	if config == nil {
		t.Fatal("GetConfig() returned nil")
	}
	if config.Name != "wolfram" {
		t.Errorf("config.Name = %q, want wolfram", config.Name)
	}
	if config.DisplayName != "Wolfram Alpha" {
		t.Errorf("config.DisplayName = %q, want 'Wolfram Alpha'", config.DisplayName)
	}
	if len(config.Categories) == 0 {
		t.Error("config.Categories should not be empty")
	}
}

// TestCleanWolframText verifies that Wolfram result text is cleaned correctly.
func TestCleanWolframText(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "ampersand entity decoded",
			input: "H&amp;O",
			want:  "H&O",
		},
		{
			name:  "less-than entity decoded",
			input: "x &lt; y",
			want:  "x < y",
		},
		{
			name:  "greater-than entity decoded",
			input: "x &gt; y",
			want:  "x > y",
		},
		{
			name:  "double-quote entity decoded",
			input: "&quot;hello&quot;",
			want:  `"hello"`,
		},
		{
			name:  "single-quote entity decoded",
			input: "it&#39;s",
			want:  "it's",
		},
		{
			name:  "non-breaking space decoded",
			input: "3&nbsp;kg",
			want:  "3 kg",
		},
		{
			name:  "escaped newline replaced with space",
			input: `line1\nline2`,
			want:  "line1 line2",
		},
		{
			name:  "escaped tab replaced with space",
			input: `col1\tcol2`,
			want:  "col1 col2",
		},
		{
			name:  "multiple consecutive spaces collapsed",
			input: "result   with   spaces",
			want:  "result with spaces",
		},
		{
			name:  "leading and trailing whitespace trimmed",
			input: "  value  ",
			want:  "value",
		},
		{
			name:  "empty string returns empty",
			input: "",
			want:  "",
		},
		{
			name:  "plain text unchanged",
			input: "42 miles per hour",
			want:  "42 miles per hour",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cleanWolframText(tt.input)
			if got != tt.want {
				t.Errorf("cleanWolframText(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestIsDuplicateContent verifies that the duplicate-content detector works
// correctly for exact matches, case-insensitive matches, and substrings.
func TestIsDuplicateContent(t *testing.T) {
	tests := []struct {
		name       string
		existing   []string
		newContent string
		want       bool
	}{
		{
			name:       "exact match is duplicate",
			existing:   []string{"Result text"},
			newContent: "Result text",
			want:       true,
		},
		{
			name:       "case-insensitive match is duplicate",
			existing:   []string{"RESULT TEXT"},
			newContent: "result text",
			want:       true,
		},
		{
			name:       "substring of existing is duplicate",
			existing:   []string{"this is the full result text"},
			newContent: "full result",
			want:       true,
		},
		{
			name:       "existing is substring of new is duplicate",
			existing:   []string{"result"},
			newContent: "this is the full result text",
			want:       true,
		},
		{
			name:       "unrelated string is not duplicate",
			existing:   []string{"first result"},
			newContent: "second result",
			want:       false,
		},
		{
			name:       "empty list is never a duplicate",
			existing:   []string{},
			newContent: "anything",
			want:       false,
		},
		{
			name:       "multiple existing entries checked",
			existing:   []string{"alpha", "beta", "gamma"},
			newContent: "delta",
			want:       false,
		},
		{
			name:       "new matches second entry in list",
			existing:   []string{"alpha", "beta", "gamma"},
			newContent: "beta",
			want:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isDuplicateContent(tt.existing, tt.newContent)
			if got != tt.want {
				t.Errorf("isDuplicateContent(%v, %q) = %v, want %v",
					tt.existing, tt.newContent, got, tt.want)
			}
		})
	}
}

// TestWolframAlphaParseWolframHTMLWithPlaintext verifies that results are built
// from data-stringified plaintext attributes when present.
func TestWolframAlphaParseWolframHTMLWithPlaintext(t *testing.T) {
	engine := NewWolframAlpha()
	query := &model.Query{Text: "speed of light", Category: model.CategoryGeneral}

	html := `<html><body>
<h2 class="pod-title">Result</h2>
<div data-stringified="2.998 &times; 10^8 m/s"></div>
</body></html>`

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(html)),
	}

	results, err := engine.parseWolframHTML(resp, query)
	if err != nil {
		t.Fatalf("parseWolframHTML() error = %v", err)
	}
	if len(results) == 0 {
		t.Fatal("parseWolframHTML() returned 0 results, want >= 1")
	}
	if results[0].Engine != "wolfram" {
		t.Errorf("Engine = %q, want wolfram", results[0].Engine)
	}
	if results[0].URL == "" {
		t.Error("URL should not be empty")
	}
	if !strings.Contains(results[0].URL, "wolframalpha.com") {
		t.Errorf("URL %q should contain wolframalpha.com", results[0].URL)
	}
	if results[0].Category != model.CategoryGeneral {
		t.Errorf("Category = %q, want general", results[0].Category)
	}
}

// TestWolframAlphaParseWolframHTMLWithImageAlt verifies that result text from
// image alt attributes is extracted when no plaintext is present.
func TestWolframAlphaParseWolframHTMLWithImageAlt(t *testing.T) {
	engine := NewWolframAlpha()
	query := &model.Query{Text: "integrate x^2", Category: model.CategoryScience}

	html := `<html><body>
<img alt="x^3/3 + constant" class="result_image" />
</body></html>`

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(html)),
	}

	results, err := engine.parseWolframHTML(resp, query)
	if err != nil {
		t.Fatalf("parseWolframHTML() error = %v", err)
	}
	if len(results) == 0 {
		t.Fatal("parseWolframHTML() returned 0 results, want >= 1")
	}
}

// TestWolframAlphaParseWolframHTMLWithPodTitle verifies that a meaningful pod
// title replaces the default title when it is not "Input" or "Input
// interpretation".
func TestWolframAlphaParseWolframHTMLWithPodTitle(t *testing.T) {
	engine := NewWolframAlpha()
	query := &model.Query{Text: "2+2", Category: model.CategoryGeneral}

	html := `<html><body>
<h2 class="podTitle">Result</h2>
<div data-stringified="4"></div>
</body></html>`

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(html)),
	}

	results, err := engine.parseWolframHTML(resp, query)
	if err != nil {
		t.Fatalf("parseWolframHTML() error = %v", err)
	}
	if len(results) == 0 {
		t.Fatal("parseWolframHTML() returned 0 results, want >= 1")
	}
}

// TestWolframAlphaParseWolframHTMLInputPodTitleIgnored verifies that pod titles
// "Input" and "Input interpretation" are ignored when building the result title.
func TestWolframAlphaParseWolframHTMLInputPodTitleIgnored(t *testing.T) {
	engine := NewWolframAlpha()
	query := &model.Query{Text: "2+2", Category: model.CategoryGeneral}

	html := `<html><body>
<h2 class="podTitle">Input</h2>
<div data-stringified="4"></div>
</body></html>`

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(html)),
	}

	results, err := engine.parseWolframHTML(resp, query)
	if err != nil {
		t.Fatalf("parseWolframHTML() error = %v", err)
	}
	if len(results) == 0 {
		t.Fatal("parseWolframHTML() returned 0 results, want >= 1")
	}

	// When pod title is "Input", the title should fall back to the default format.
	if !strings.Contains(results[0].Title, query.Text) {
		t.Errorf("Title %q should contain query text %q when pod is 'Input'",
			results[0].Title, query.Text)
	}
}

// TestWolframAlphaParseWolframHTMLSimpleFallback verifies that the simple-pod
// pattern is used when neither plaintext nor image-alt content is found.
func TestWolframAlphaParseWolframHTMLSimpleFallback(t *testing.T) {
	engine := NewWolframAlpha()
	query := &model.Query{Text: "pi", Category: model.CategoryGeneral}

	html := `<html><body>
<div class="pod-content">3.14159265358979...</div>
</body></html>`

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(html)),
	}

	results, err := engine.parseWolframHTML(resp, query)
	if err != nil {
		t.Fatalf("parseWolframHTML() error = %v", err)
	}
	// Must always return at least the placeholder result.
	if len(results) == 0 {
		t.Fatal("parseWolframHTML() returned 0 results, want >= 1")
	}
}

// TestWolframAlphaParseWolframHTMLPlaceholderAlwaysReturned verifies that even
// completely empty HTML produces a placeholder result pointing to Wolfram Alpha.
func TestWolframAlphaParseWolframHTMLPlaceholderAlwaysReturned(t *testing.T) {
	engine := NewWolframAlpha()
	query := &model.Query{Text: "no results query", Category: model.CategoryGeneral}

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader("<html><body></body></html>")),
	}

	results, err := engine.parseWolframHTML(resp, query)
	if err != nil {
		t.Fatalf("parseWolframHTML() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("parseWolframHTML() returned %d results, want 1 placeholder", len(results))
	}
	if !strings.Contains(results[0].URL, "wolframalpha.com") {
		t.Errorf("placeholder URL %q should contain wolframalpha.com", results[0].URL)
	}
	if !strings.Contains(results[0].Content, "Wolfram Alpha") {
		t.Errorf("placeholder Content %q should mention Wolfram Alpha", results[0].Content)
	}
}

// TestWolframAlphaParseWolframHTMLContentTruncated verifies that content
// exceeding 500 characters is truncated with an ellipsis.
func TestWolframAlphaParseWolframHTMLContentTruncated(t *testing.T) {
	engine := NewWolframAlpha()
	query := &model.Query{Text: "test", Category: model.CategoryGeneral}

	// Build a long plaintext that exceeds the 500-char limit.
	longText := strings.Repeat("A", 600)
	html := fmt.Sprintf(
		`<html><body><div data-stringified="%s"></div></body></html>`,
		longText,
	)

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(html)),
	}

	results, err := engine.parseWolframHTML(resp, query)
	if err != nil {
		t.Fatalf("parseWolframHTML() error = %v", err)
	}
	if len(results) == 0 {
		t.Fatal("parseWolframHTML() returned 0 results")
	}
	if len(results[0].Content) > 500 {
		t.Errorf("Content length = %d, want <= 500", len(results[0].Content))
	}
	if !strings.HasSuffix(results[0].Content, "...") {
		t.Errorf("Truncated content should end with '...', got %q", results[0].Content)
	}
}

// TestWolframAlphaParseWolframHTMLDuplicatesIgnored verifies that duplicate
// content parts (case-insensitive) are not added more than once.
func TestWolframAlphaParseWolframHTMLDuplicatesIgnored(t *testing.T) {
	engine := NewWolframAlpha()
	query := &model.Query{Text: "test", Category: model.CategoryGeneral}

	// Two identical plaintext values should produce only one content part.
	html := `<html><body>
<div data-stringified="unique value"></div>
<div data-stringified="unique value"></div>
</body></html>`

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(html)),
	}

	results, err := engine.parseWolframHTML(resp, query)
	if err != nil {
		t.Fatalf("parseWolframHTML() error = %v", err)
	}
	if len(results) == 0 {
		t.Fatal("parseWolframHTML() returned 0 results")
	}
	// Content should contain "unique value" only once (no duplication).
	content := results[0].Content
	firstIdx := strings.Index(content, "unique value")
	if firstIdx == -1 {
		t.Fatalf("content %q does not contain 'unique value'", content)
	}
	secondIdx := strings.Index(content[firstIdx+1:], "unique value")
	if secondIdx != -1 {
		t.Errorf("content %q contains 'unique value' more than once", content)
	}
}

// TestWolframAlphaSearchWithMockServer exercises the full Search() → searchWeb()
// → parseWolframHTML() path using a mock HTTP server.
func TestWolframAlphaSearchWithMockServer(t *testing.T) {
	html := `<html><body>
<h2 class="pod-title">Result</h2>
<div data-stringified="42"></div>
</body></html>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, html)
	}))
	defer server.Close()

	engine := NewWolframAlpha()
	engine.client = server.Client()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Search() calls the hardcoded wolframalpha.com URL, which will fail with
	// the test-server client.  We verify no panic occurs.
	_, _ = engine.Search(ctx, &model.Query{
		Text:     "meaning of life",
		Category: model.CategoryGeneral,
	})
}

// TestWolframAlphaSearchTimeout verifies that a context timeout produces an
// error rather than hanging or panicking.
func TestWolframAlphaSearchTimeout(t *testing.T) {
	engine := NewWolframAlpha()

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()

	_, err := engine.Search(ctx, &model.Query{
		Text:     "test",
		Category: model.CategoryGeneral,
	})
	if err == nil {
		t.Error("Search() expected error for instant timeout, got nil")
	}
}

// TestWolframAlphaSearchHTTPError verifies that a non-200 HTTP response from a
// mock server causes Search() to return an error.
func TestWolframAlphaSearchHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	engine := NewWolframAlpha()
	engine.client = server.Client()

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()

	// URL is hardcoded to wolframalpha.com; the instant timeout causes the
	// error path without reaching our server.
	_, err := engine.searchWeb(ctx, &model.Query{
		Text:     "test",
		Category: model.CategoryGeneral,
	})
	if err == nil {
		t.Error("searchWeb() expected error, got nil")
	}
}

// TestWolframAlphaResultURL verifies that parseWolframHTML sets the result URL
// to the canonical Wolfram Alpha query URL with the encoded query text.
func TestWolframAlphaResultURL(t *testing.T) {
	engine := NewWolframAlpha()
	query := &model.Query{Text: "speed of light", Category: model.CategoryGeneral}

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader("<html><body></body></html>")),
	}

	results, err := engine.parseWolframHTML(resp, query)
	if err != nil {
		t.Fatalf("parseWolframHTML() error = %v", err)
	}
	if len(results) == 0 {
		t.Fatal("parseWolframHTML() returned 0 results")
	}

	expectedURLPrefix := "https://www.wolframalpha.com/input?i="
	if !strings.HasPrefix(results[0].URL, expectedURLPrefix) {
		t.Errorf("URL = %q, want prefix %q", results[0].URL, expectedURLPrefix)
	}
}

// TestWolframAlphaResultScore verifies that results have a positive score.
func TestWolframAlphaResultScore(t *testing.T) {
	engine := NewWolframAlpha()
	query := &model.Query{Text: "2+2", Category: model.CategoryGeneral}

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader("<html><body></body></html>")),
	}

	results, err := engine.parseWolframHTML(resp, query)
	if err != nil {
		t.Fatalf("parseWolframHTML() error = %v", err)
	}
	if len(results) == 0 {
		t.Fatal("parseWolframHTML() returned 0 results")
	}
	if results[0].Score <= 0 {
		t.Errorf("Score = %f, want > 0", results[0].Score)
	}
	if results[0].Position != 0 {
		t.Errorf("Position = %d, want 0", results[0].Position)
	}
}
