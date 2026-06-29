package engine

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/apimgr/search/src/model"
)

// ============================================================================
// ArXiv tests
// ============================================================================

func TestCleanArXivText(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "single newline replaced with space",
			input: "line one\nline two",
			want:  "line one line two",
		},
		{
			name:  "multiple newlines collapsed to single space",
			input: "a\n\n\nb",
			want:  "a b",
		},
		{
			name:  "multiple spaces collapsed to one",
			input: "hello   world",
			want:  "hello world",
		},
		{
			name:  "leading and trailing whitespace trimmed",
			input: "  hello world  ",
			want:  "hello world",
		},
		{
			name:  "newline and multiple spaces combined",
			input: "  foo\n  bar  \n  baz  ",
			want:  "foo bar baz",
		},
		{
			name:  "empty string unchanged",
			input: "",
			want:  "",
		},
		{
			name:  "already clean text unchanged",
			input: "clean text here",
			want:  "clean text here",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cleanArXivText(tt.input)
			if got != tt.want {
				t.Errorf("cleanArXivText(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// buildArXivAtomFeed returns a minimal valid Atom XML feed for testing.
func buildArXivAtomFeed(entries []string) string {
	body := `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">` + strings.Join(entries, "") + `</feed>`
	return body
}

func TestArXivSearch(t *testing.T) {
	entry := `<entry>
		<id>http://arxiv.org/abs/2301.00001v1</id>
		<title>A Great Paper Title</title>
		<summary>This is the abstract. It describes the paper content.</summary>
		<published>2023-01-01T00:00:00Z</published>
		<updated>2023-01-02T00:00:00Z</updated>
		<author><name>Alice Smith</name></author>
		<author><name>Bob Jones</name></author>
		<link rel="alternate" type="text/html" href="https://arxiv.org/abs/2301.00001"/>
		<category term="cs.AI"/>
		<category term="cs.LG"/>
	</entry>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/atom+xml")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, buildArXivAtomFeed([]string{entry}))
	}))
	defer server.Close()

	engine := NewArXiv()
	engine.client = &http.Client{Transport: redirectToServer(server.URL)}

	ctx := context.Background()
	query := &model.Query{Text: "machine learning", Page: 1}

	results, err := engine.Search(ctx, query)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(results) == 0 {
		t.Fatal("Search() returned no results")
	}
	if results[0].Title == "" {
		t.Error("result.Title should not be empty")
	}
	if results[0].URL == "" {
		t.Error("result.URL should not be empty")
	}
	if results[0].Category != model.CategoryScience {
		t.Errorf("result.Category = %q, want %q", results[0].Category, model.CategoryScience)
	}
	if results[0].Author == "" {
		t.Error("result.Author should not be empty for multi-author paper")
	}
}

func TestArXivSearchMoreThanThreeAuthors(t *testing.T) {
	entry := `<entry>
		<id>http://arxiv.org/abs/2302.00001v1</id>
		<title>Big Team Paper</title>
		<summary>Abstract text.</summary>
		<published>2023-02-01T00:00:00Z</published>
		<author><name>Alice Smith</name></author>
		<author><name>Bob Jones</name></author>
		<author><name>Carol White</name></author>
		<author><name>Dave Brown</name></author>
		<link rel="alternate" type="text/html" href="https://arxiv.org/abs/2302.00001"/>
	</entry>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/atom+xml")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, buildArXivAtomFeed([]string{entry}))
	}))
	defer server.Close()

	engine := NewArXiv()
	engine.client = &http.Client{Transport: redirectToServer(server.URL)}

	results, err := engine.Search(context.Background(), &model.Query{Text: "test", Page: 1})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(results) == 0 {
		t.Fatal("Search() returned no results")
	}
	if !strings.Contains(results[0].Author, "et al.") {
		t.Errorf("Author with 4 names should contain 'et al.', got %q", results[0].Author)
	}
}

func TestArXivSearchLongSummaryTruncated(t *testing.T) {
	// Use 600 chars to exceed the 500 char limit
	longSummary := strings.Repeat("x", 600)
	entry := `<entry>
		<id>http://arxiv.org/abs/2303.00001v1</id>
		<title>Long Abstract Paper</title>
		<summary>` + longSummary + `</summary>
		<published>2023-03-01T00:00:00Z</published>
		<author><name>Alice Smith</name></author>
		<link rel="alternate" type="text/html" href="https://arxiv.org/abs/2303.00001"/>
	</entry>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/atom+xml")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, buildArXivAtomFeed([]string{entry}))
	}))
	defer server.Close()

	engine := NewArXiv()
	engine.client = &http.Client{Transport: redirectToServer(server.URL)}

	results, err := engine.Search(context.Background(), &model.Query{Text: "test", Page: 1})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(results) == 0 {
		t.Fatal("Search() returned no results")
	}
	if !strings.Contains(results[0].Content, "...") {
		t.Error("long abstract should be truncated with '...'")
	}
}

func TestArXivSearchNon200Status(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	engine := NewArXiv()
	engine.client = &http.Client{Transport: redirectToServer(server.URL)}

	_, err := engine.Search(context.Background(), &model.Query{Text: "test", Page: 1})
	if err == nil {
		t.Error("Search() should return error for non-200 status")
	}
}

func TestArXivSearchInvalidXML(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/atom+xml")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "this is not xml <<>><<")
	}))
	defer server.Close()

	engine := NewArXiv()
	engine.client = &http.Client{Transport: redirectToServer(server.URL)}

	_, err := engine.Search(context.Background(), &model.Query{Text: "test", Page: 1})
	if err == nil {
		t.Error("Search() should return error for invalid XML")
	}
}

func TestArXivSearchContextTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	engine := NewArXiv()
	engine.client = &http.Client{Transport: redirectToServer(server.URL)}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()

	_, err := engine.Search(ctx, &model.Query{Text: "test", Page: 1})
	if err == nil {
		t.Error("Search() should return error on context timeout")
	}
}

// ============================================================================
// HackerNews tests
// ============================================================================

func TestHackerNewsSearchWithExternalURL(t *testing.T) {
	payload := `{
		"hits": [
			{
				"title": "Show HN: My cool project",
				"url": "https://example.com/project",
				"author": "hacker123",
				"points": 250,
				"num_comments": 42,
				"created_at": "2023-06-01T10:00:00Z",
				"objectID": "12345678",
				"story_text": ""
			}
		]
	}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, payload)
	}))
	defer server.Close()

	engine := NewHackerNews()
	engine.client = &http.Client{Transport: redirectToServer(server.URL)}

	results, err := engine.Search(context.Background(), &model.Query{Text: "project", Page: 1})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("Search() returned %d results, want 1", len(results))
	}
	if results[0].URL != "https://example.com/project" {
		t.Errorf("result.URL = %q, want %q", results[0].URL, "https://example.com/project")
	}
	if results[0].Author != "hacker123" {
		t.Errorf("result.Author = %q, want hacker123", results[0].Author)
	}
}

func TestHackerNewsSearchNoURL(t *testing.T) {
	payload := `{
		"hits": [
			{
				"title": "Ask HN: Best resources for learning Go?",
				"url": "",
				"author": "golearner",
				"points": 100,
				"num_comments": 30,
				"created_at": "2023-07-01T12:00:00Z",
				"objectID": "99887766",
				"story_text": ""
			}
		]
	}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, payload)
	}))
	defer server.Close()

	engine := NewHackerNews()
	engine.client = &http.Client{Transport: redirectToServer(server.URL)}

	results, err := engine.Search(context.Background(), &model.Query{Text: "go", Page: 1})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("Search() returned %d results, want 1", len(results))
	}
	expectedURL := "https://news.ycombinator.com/item?id=99887766"
	if results[0].URL != expectedURL {
		t.Errorf("result.URL = %q, want %q", results[0].URL, expectedURL)
	}
}

func TestHackerNewsSearchWithStoryText(t *testing.T) {
	payload := `{
		"hits": [
			{
				"title": "Ask HN: Discussion topic",
				"url": "",
				"author": "discusser",
				"points": 50,
				"num_comments": 10,
				"created_at": "2023-08-01T08:00:00Z",
				"objectID": "11223344",
				"story_text": "This is the story body text that was written in the original post."
			}
		]
	}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, payload)
	}))
	defer server.Close()

	engine := NewHackerNews()
	engine.client = &http.Client{Transport: redirectToServer(server.URL)}

	results, err := engine.Search(context.Background(), &model.Query{Text: "discussion", Page: 1})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("Search() returned %d results, want 1", len(results))
	}
	if !strings.Contains(results[0].Content, "story body text") {
		t.Errorf("result.Content should contain story_text snippet, got %q", results[0].Content)
	}
}

func TestHackerNewsSearchNon200(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	engine := NewHackerNews()
	engine.client = &http.Client{Transport: redirectToServer(server.URL)}

	_, err := engine.Search(context.Background(), &model.Query{Text: "test", Page: 1})
	if err == nil {
		t.Error("Search() should return error for non-200 status")
	}
}

func TestHackerNewsSearchInvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "not valid json {{{{")
	}))
	defer server.Close()

	engine := NewHackerNews()
	engine.client = &http.Client{Transport: redirectToServer(server.URL)}

	_, err := engine.Search(context.Background(), &model.Query{Text: "test", Page: 1})
	if err == nil {
		t.Error("Search() should return error for invalid JSON")
	}
}

func TestHackerNewsSearchContextTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	engine := NewHackerNews()
	engine.client = &http.Client{Transport: redirectToServer(server.URL)}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()

	_, err := engine.Search(ctx, &model.Query{Text: "test", Page: 1})
	if err == nil {
		t.Error("Search() should return error on context timeout")
	}
}

// ============================================================================
// GitHub tests
// ============================================================================

func TestGitHubSearchWithLanguageAndStars(t *testing.T) {
	payload := `{
		"items": [
			{
				"full_name": "owner/awesome-repo",
				"html_url": "https://github.com/owner/awesome-repo",
				"description": "An awesome repository",
				"stargazers_count": 1500,
				"forks_count": 200,
				"language": "Go",
				"updated_at": "2023-09-01T00:00:00Z"
			}
		]
	}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, payload)
	}))
	defer server.Close()

	engine := NewGitHub()
	engine.client = &http.Client{Transport: redirectToServer(server.URL)}

	results, err := engine.Search(context.Background(), &model.Query{Text: "awesome", Page: 1})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("Search() returned %d results, want 1", len(results))
	}
	if results[0].Title != "owner/awesome-repo" {
		t.Errorf("result.Title = %q, want owner/awesome-repo", results[0].Title)
	}
	if !strings.Contains(results[0].Content, "[Go]") {
		t.Errorf("result.Content should contain language, got %q", results[0].Content)
	}
	if !strings.Contains(results[0].Content, "★ 1500") {
		t.Errorf("result.Content should contain star count, got %q", results[0].Content)
	}
}

func TestGitHubSearchEmptyDescription(t *testing.T) {
	payload := `{
		"items": [
			{
				"full_name": "owner/silent-repo",
				"html_url": "https://github.com/owner/silent-repo",
				"description": "",
				"stargazers_count": 0,
				"forks_count": 0,
				"language": "",
				"updated_at": "2023-09-01T00:00:00Z"
			}
		]
	}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, payload)
	}))
	defer server.Close()

	engine := NewGitHub()
	engine.client = &http.Client{Transport: redirectToServer(server.URL)}

	results, err := engine.Search(context.Background(), &model.Query{Text: "silent", Page: 1})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("Search() returned %d results, want 1", len(results))
	}
	if results[0].Title != "owner/silent-repo" {
		t.Errorf("result.Title = %q, want owner/silent-repo", results[0].Title)
	}
}

func TestGitHubSearchNon200(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	engine := NewGitHub()
	engine.client = &http.Client{Transport: redirectToServer(server.URL)}

	_, err := engine.Search(context.Background(), &model.Query{Text: "test", Page: 1})
	if err == nil {
		t.Error("Search() should return error for non-200 status")
	}
}

func TestGitHubSearchInvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "{{bad json")
	}))
	defer server.Close()

	engine := NewGitHub()
	engine.client = &http.Client{Transport: redirectToServer(server.URL)}

	_, err := engine.Search(context.Background(), &model.Query{Text: "test", Page: 1})
	if err == nil {
		t.Error("Search() should return error for invalid JSON")
	}
}

func TestGitHubSearchContextTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	engine := NewGitHub()
	engine.client = &http.Client{Transport: redirectToServer(server.URL)}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()

	_, err := engine.Search(ctx, &model.Query{Text: "test", Page: 1})
	if err == nil {
		t.Error("Search() should return error on context timeout")
	}
}

// ============================================================================
// Google parse function tests
// ============================================================================

func TestGoogleParseImageResults(t *testing.T) {
	engine := NewGoogle()
	query := &model.Query{Text: "cats", Category: model.CategoryImages}

	tests := []struct {
		name      string
		html      string
		wantCount int
	}{
		{
			name: "valid image JSON arrays",
			html: `["https://example.com/cat.jpg",640,480]
["https://other.com/dog.png",800,600]`,
			wantCount: 2,
		},
		{
			name:      "empty HTML returns no results",
			html:      "",
			wantCount: 0,
		},
		{
			name: "gstatic.com URLs are skipped",
			html: `["https://encrypted-tbn0.gstatic.com/images?q=test.jpg",100,100]
["https://example.com/real.jpg",640,480]`,
			wantCount: 1,
		},
		{
			name: "google.com URLs are skipped",
			html: `["https://www.google.com/logo.jpg",100,100]
["https://example.com/valid.jpg",400,300]`,
			wantCount: 1,
		},
		{
			name:      "no JSON arrays in HTML",
			html:      "<html><body>no images here</body></html>",
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results := engine.parseImageResults(tt.html, query)
			if len(results) != tt.wantCount {
				t.Errorf("parseImageResults() returned %d results, want %d", len(results), tt.wantCount)
			}
			for _, r := range results {
				if r.Category != model.CategoryImages {
					t.Errorf("result.Category = %q, want %q", r.Category, model.CategoryImages)
				}
				if r.URL == "" {
					t.Error("result.URL should not be empty")
				}
			}
		})
	}
}

func TestGoogleParseVideoResults(t *testing.T) {
	engine := NewGoogle()
	query := &model.Query{Text: "music video", Category: model.CategoryVideos}

	tests := []struct {
		name      string
		html      string
		wantCount int
	}{
		{
			name: "youtube.com redirect kept",
			html: `<a href="/url?q=https://www.youtube.com/watch?v=abc123"><h3>Great Video</h3></a>`,
			wantCount: 1,
		},
		{
			name: "youtu.be short link kept",
			html: `<a href="/url?q=https://youtu.be/xyz789"><h3>Short Link Video</h3></a>`,
			wantCount: 1,
		},
		{
			name: "vimeo.com URL kept",
			html: `<a href="/url?q=https://vimeo.com/123456"><h3>Vimeo Video</h3></a>`,
			wantCount: 1,
		},
		{
			name: "non-video URL skipped",
			html: `<a href="/url?q=https://example.com/page"><h3>Not a video</h3></a>`,
			wantCount: 0,
		},
		{
			name:      "empty HTML returns no results",
			html:      "",
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results := engine.parseVideoResults(tt.html, query)
			if len(results) != tt.wantCount {
				t.Errorf("parseVideoResults() returned %d results, want %d (html: %q)", len(results), tt.wantCount, tt.html)
			}
		})
	}
}

func TestGoogleParseWebResults(t *testing.T) {
	engine := NewGoogle()
	query := &model.Query{Text: "golang", Category: model.CategoryGeneral}

	tests := []struct {
		name      string
		html      string
		wantMin   int
		wantMax   int
	}{
		{
			name:    "anchor containing h3 pattern matched",
			html:    `<a href="/url?q=https://example.com/page&amp;sa=U"><h3>Example Page Title</h3></a>`,
			wantMin: 1,
			wantMax: 10,
		},
		{
			name:    "google-owned URL is skipped",
			html:    `<a href="/url?q=https://www.google.com/search?q=test"><h3>Google Search</h3></a>`,
			wantMin: 0,
			wantMax: 0,
		},
		{
			name:    "empty HTML returns empty slice",
			html:    "",
			wantMin: 0,
			wantMax: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results := engine.parseWebResults(tt.html, query)
			if len(results) < tt.wantMin || len(results) > tt.wantMax {
				t.Errorf("parseWebResults() returned %d results, want between %d and %d",
					len(results), tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestIsVideoURL(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"youtube.com full URL", "https://www.youtube.com/watch?v=abc", true},
		{"youtu.be short link", "https://youtu.be/abc123", true},
		{"vimeo.com URL", "https://vimeo.com/12345", true},
		{"dailymotion.com URL", "https://www.dailymotion.com/video/x123", true},
		{"twitch.tv URL", "https://www.twitch.tv/channel", true},
		{"example.com is not video", "https://example.com/video.mp4", false},
		{"empty string is not video", "", false},
		{"google.com is not video", "https://www.google.com/search", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isVideoURL(tt.input)
			if got != tt.want {
				t.Errorf("isVideoURL(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestExtractTextBlock(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantText string
	}{
		{
			name:     "plain prose extracted",
			input:    "<span>This is a plain description of the page.</span>",
			wantText: "This is a plain description of the page.",
		},
		{
			name:     "URL words are skipped",
			input:    "https://example.com/path here is real text",
			wantText: "here is real text",
		},
		{
			name:     "HTML tags stripped",
			input:    "<b>bold</b> and <i>italic</i> text",
			wantText: "bold and italic text",
		},
		{
			name:     "empty input returns empty",
			input:    "",
			wantText: "",
		},
		{
			name:     "long text truncated at 500 chars",
			input:    strings.Repeat("word ", 200),
			wantText: strings.TrimSpace(strings.Repeat("word ", 100)),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractTextBlock(tt.input)
			if tt.name == "long text truncated at 500 chars" {
				if len(got) > 505 {
					t.Errorf("extractTextBlock() returned %d chars, want <= 505", len(got))
				}
				return
			}
			if got != tt.wantText {
				t.Errorf("extractTextBlock(%q) = %q, want %q", tt.input, got, tt.wantText)
			}
		})
	}
}

func TestIsGoogleURL(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"google.com domain", "https://www.google.com/search", true},
		{"google.co.uk domain", "https://www.google.co.uk/search", true},
		{"googleapis.com domain", "https://storage.googleapis.com/file", true},
		{"gstatic.com domain", "https://encrypted-tbn0.gstatic.com/img", true},
		{"relative path", "/url?q=something", true},
		{"example.com is not google", "https://example.com/page", false},
		{"youtube.com is not google", "https://www.youtube.com/watch", false},
		{"empty string is not google", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isGoogleURL(tt.input)
			if got != tt.want {
				t.Errorf("isGoogleURL(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestGoogleDecodeRedirect(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "valid /url?q= path returns decoded URL",
			input: "/url?q=https://example.com/page&sa=U",
			want:  "https://example.com/page",
		},
		{
			name:  "URL-encoded value is decoded",
			input: "/url?q=https%3A%2F%2Fexample.com%2Fpath",
			want:  "https://example.com/path",
		},
		{
			name:  "no /url? returns empty string",
			input: "https://example.com/direct",
			want:  "",
		},
		{
			name:  "empty string returns empty string",
			input: "",
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := googleDecodeRedirect(tt.input)
			if got != tt.want {
				t.Errorf("googleDecodeRedirect(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestExtractGoogleURLPassThrough(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "https URL passes through unchanged",
			input: "https://example.com/page",
			want:  "https://example.com/page",
		},
		{
			name:  "http URL passes through unchanged",
			input: "http://example.com/page",
			want:  "http://example.com/page",
		},
		{
			name:  "/url?q= redirect is decoded",
			input: "/url?q=https://example.com/path",
			want:  "https://example.com/path",
		},
		{
			name:  "no /url? and not http returns empty",
			input: "ftp://example.com/file",
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractGoogleURL(tt.input)
			if got != tt.want {
				t.Errorf("extractGoogleURL(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// ============================================================================
// OpenStreetMap helper tests
// ============================================================================

func TestOSMBuildTitle(t *testing.T) {
	engine := NewOpenStreetMap()

	tests := []struct {
		name     string
		nr       nominatimResult
		contains string
	}{
		{
			name:     "place/city adds City qualifier",
			nr:       nominatimResult{DisplayName: "Springfield, Illinois, USA", Class: "place", Type: "city"},
			contains: "City",
		},
		{
			name:     "place/county adds County qualifier",
			nr:       nominatimResult{DisplayName: "Orange County, California, USA", Class: "place", Type: "county"},
			contains: "County",
		},
		{
			name:     "highway adds Road qualifier",
			nr:       nominatimResult{DisplayName: "Main Street, Springfield", Class: "highway", Type: "residential"},
			contains: "Road",
		},
		{
			name:     "building adds Building qualifier",
			nr:       nominatimResult{DisplayName: "City Hall, Springfield", Class: "building", Type: "public"},
			contains: "Building",
		},
		{
			name:     "amenity adds formatted type",
			nr:       nominatimResult{DisplayName: "McDonald's, Springfield", Class: "amenity", Type: "fast_food"},
			contains: "Fast Food",
		},
		{
			name:     "tourism adds formatted type",
			nr:       nominatimResult{DisplayName: "Grand Canyon, Arizona", Class: "tourism", Type: "attraction"},
			contains: "Attraction",
		},
		{
			name:     "natural adds formatted type",
			nr:       nominatimResult{DisplayName: "Mount Rainier, Washington", Class: "natural", Type: "peak"},
			contains: "Peak",
		},
		{
			name:     "boundary/administrative adds qualifier",
			nr:       nominatimResult{DisplayName: "King County, Washington", Class: "boundary", Type: "administrative"},
			contains: "Administrative Area",
		},
		{
			name:     "unknown class returns primary name only",
			nr:       nominatimResult{DisplayName: "Some Place, Somewhere", Class: "unknown_class", Type: "unknown_type"},
			contains: "Some Place",
		},
		{
			name:     "qualifier already in name not duplicated",
			nr:       nominatimResult{DisplayName: "Road Street, Springfield", Class: "highway", Type: "residential"},
			contains: "Road Street",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := engine.buildTitle(tt.nr)
			if !strings.Contains(got, tt.contains) {
				t.Errorf("buildTitle() = %q, should contain %q", got, tt.contains)
			}
		})
	}
}

func TestOSMBuildOSMURL(t *testing.T) {
	engine := NewOpenStreetMap()

	tests := []struct {
		name     string
		nr       nominatimResult
		wantPath string
	}{
		{
			name:     "node type produces n/ URL",
			nr:       nominatimResult{OSMType: "node", OSMID: 12345},
			wantPath: "/n/12345",
		},
		{
			name:     "way type produces w/ URL",
			nr:       nominatimResult{OSMType: "way", OSMID: 67890},
			wantPath: "/w/67890",
		},
		{
			name:     "relation type produces r/ URL",
			nr:       nominatimResult{OSMType: "relation", OSMID: 99999},
			wantPath: "/r/99999",
		},
		{
			name:     "no OSMType falls back to lat/lon URL",
			nr:       nominatimResult{OSMType: "", OSMID: 0, Lat: "47.6062", Lon: "-122.3321"},
			wantPath: "mlat=47.6062",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := engine.buildOSMURL(tt.nr)
			if !strings.Contains(got, tt.wantPath) {
				t.Errorf("buildOSMURL() = %q, should contain %q", got, tt.wantPath)
			}
		})
	}
}

func TestOSMBuildContent(t *testing.T) {
	engine := NewOpenStreetMap()

	tests := []struct {
		name     string
		nr       nominatimResult
		wantPart string
	}{
		{
			name: "address and coordinates both present",
			nr: func() nominatimResult {
				r := nominatimResult{Lat: "47.606", Lon: "-122.332"}
				r.Address.Road = "Main St"
				r.Address.City = "Seattle"
				return r
			}(),
			wantPart: "Coordinates:",
		},
		{
			name:     "coordinates only when no address",
			nr:       nominatimResult{Lat: "48.000", Lon: "-120.000"},
			wantPart: "Coordinates:",
		},
		{
			name: "no coordinates returns address only",
			nr: func() nominatimResult {
				r := nominatimResult{}
				r.Address.City = "Portland"
				r.Address.Country = "United States"
				return r
			}(),
			wantPart: "Portland",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := engine.buildContent(tt.nr)
			if !strings.Contains(got, tt.wantPart) {
				t.Errorf("buildContent() = %q, should contain %q", got, tt.wantPart)
			}
		})
	}
}

func TestOSMFormatAddress(t *testing.T) {
	engine := NewOpenStreetMap()

	tests := []struct {
		name     string
		nr       nominatimResult
		wantPart string
		wantNot  string
	}{
		{
			name: "house number and road combined",
			nr: func() nominatimResult {
				r := nominatimResult{}
				r.Address.HouseNumber = "123"
				r.Address.Road = "Main Street"
				return r
			}(),
			wantPart: "123 Main Street",
		},
		{
			name: "road only without house number",
			nr: func() nominatimResult {
				r := nominatimResult{}
				r.Address.Road = "Oak Avenue"
				return r
			}(),
			wantPart: "Oak Avenue",
		},
		{
			name: "suburb included",
			nr: func() nominatimResult {
				r := nominatimResult{}
				r.Address.Road = "Elm St"
				r.Address.Suburb = "Northside"
				return r
			}(),
			wantPart: "Northside",
		},
		{
			name: "city field used when present",
			nr: func() nominatimResult {
				r := nominatimResult{}
				r.Address.City = "Seattle"
				return r
			}(),
			wantPart: "Seattle",
		},
		{
			name: "town used when city is empty",
			nr: func() nominatimResult {
				r := nominatimResult{}
				r.Address.Town = "Redmond"
				return r
			}(),
			wantPart: "Redmond",
		},
		{
			name: "village used when city and town are empty",
			nr: func() nominatimResult {
				r := nominatimResult{}
				r.Address.Village = "Smallville"
				return r
			}(),
			wantPart: "Smallville",
		},
		{
			name: "municipality used when others are empty",
			nr: func() nominatimResult {
				r := nominatimResult{}
				r.Address.Municipality = "Township"
				return r
			}(),
			wantPart: "Township",
		},
		{
			name: "state included",
			nr: func() nominatimResult {
				r := nominatimResult{}
				r.Address.City = "Portland"
				r.Address.State = "Oregon"
				return r
			}(),
			wantPart: "Oregon",
		},
		{
			name: "county used when state is absent",
			nr: func() nominatimResult {
				r := nominatimResult{}
				r.Address.County = "Multnomah County"
				return r
			}(),
			wantPart: "Multnomah County",
		},
		{
			name: "postcode included",
			nr: func() nominatimResult {
				r := nominatimResult{}
				r.Address.City = "Portland"
				r.Address.Postcode = "97201"
				return r
			}(),
			wantPart: "97201",
		},
		{
			name: "country included",
			nr: func() nominatimResult {
				r := nominatimResult{}
				r.Address.Country = "United States"
				return r
			}(),
			wantPart: "United States",
		},
		{
			name:     "empty address returns empty string",
			nr:       nominatimResult{},
			wantPart: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := engine.formatAddress(tt.nr)
			if tt.wantPart == "" {
				if got != "" {
					t.Errorf("formatAddress() = %q, want empty", got)
				}
				return
			}
			if !strings.Contains(got, tt.wantPart) {
				t.Errorf("formatAddress() = %q, should contain %q", got, tt.wantPart)
			}
		})
	}
}

func TestFormatType(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"restaurant kept as-is (Title)", "restaurant", "Restaurant"},
		{"fast_food underscore becomes space and titled", "fast_food", "Fast Food"},
		{"bank titled", "bank", "Bank"},
		{"car_wash multi-word", "car_wash", "Car Wash"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatType(tt.input)
			if got != tt.want {
				t.Errorf("formatType(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestOSMParseResultsDirect(t *testing.T) {
	engine := NewOpenStreetMap()

	nrs := []nominatimResult{
		{
			PlaceID:     1,
			OSMType:     "way",
			OSMID:       111,
			Lat:         "47.606",
			Lon:         "-122.332",
			DisplayName: "Seattle, Washington, USA",
			Class:       "place",
			Type:        "city",
			Importance:  0.8,
		},
	}

	query := &model.Query{Text: "Seattle", Page: 1}
	results := engine.parseResults(nrs, query)

	if len(results) != 1 {
		t.Fatalf("parseResults() returned %d results, want 1", len(results))
	}
	if results[0].Category != model.CategoryMaps {
		t.Errorf("result.Category = %q, want %q", results[0].Category, model.CategoryMaps)
	}
	if !strings.Contains(results[0].URL, "/w/111") {
		t.Errorf("result.URL = %q, should contain /w/111", results[0].URL)
	}
	if results[0].Metadata == nil {
		t.Error("result.Metadata should not be nil")
	}
}

func TestOSMSearchValidJSON(t *testing.T) {
	payload := `[
		{
			"place_id": 1,
			"osm_type": "way",
			"osm_id": 222,
			"lat": "51.5074",
			"lon": "-0.1278",
			"display_name": "London, Greater London, England, United Kingdom",
			"class": "place",
			"type": "city",
			"importance": 0.9
		}
	]`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, payload)
	}))
	defer server.Close()

	engine := NewOpenStreetMap()
	engine.client = &http.Client{Transport: redirectToServer(server.URL)}

	results, err := engine.Search(context.Background(), &model.Query{Text: "London", Page: 1})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("Search() returned %d results, want 1", len(results))
	}
}

func TestOSMSearchWithLanguage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lang := r.URL.Query().Get("accept-language")
		if lang != "fr" {
			t.Errorf("accept-language = %q, want fr", lang)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `[]`)
	}))
	defer server.Close()

	engine := NewOpenStreetMap()
	engine.client = &http.Client{Transport: redirectToServer(server.URL)}

	_, err := engine.Search(context.Background(), &model.Query{Text: "Paris", Page: 1, Language: "fr"})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
}

func TestOSMSearchNon200(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	engine := NewOpenStreetMap()
	engine.client = &http.Client{Transport: redirectToServer(server.URL)}

	_, err := engine.Search(context.Background(), &model.Query{Text: "test", Page: 1})
	if err == nil {
		t.Error("Search() should return error for non-200 status")
	}
}

func TestOSMSearchInvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "not valid json {{{")
	}))
	defer server.Close()

	engine := NewOpenStreetMap()
	engine.client = &http.Client{Transport: redirectToServer(server.URL)}

	_, err := engine.Search(context.Background(), &model.Query{Text: "test", Page: 1})
	if err == nil {
		t.Error("Search() should return error for invalid JSON")
	}
}

func TestOSMSearchContextTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	engine := NewOpenStreetMap()
	engine.client = &http.Client{Transport: redirectToServer(server.URL)}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()

	_, err := engine.Search(ctx, &model.Query{Text: "test", Page: 1})
	if err == nil {
		t.Error("Search() should return error on context timeout")
	}
}

// ============================================================================
// PubMed helper tests
// ============================================================================

func TestCleanPubMedText(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "whitespace collapsed",
			input: "  hello   world  ",
			want:  "hello world",
		},
		{
			name:  "newlines collapsed",
			input: "line one\nline two\n\nline three",
			want:  "line one line two line three",
		},
		{
			name:  "HTML entity &amp; decoded",
			input: "cats &amp; dogs",
			want:  "cats & dogs",
		},
		{
			name:  "HTML entity &lt; decoded",
			input: "1 &lt; 2",
			want:  "1 < 2",
		},
		{
			name:  "HTML entity &gt; decoded",
			input: "2 &gt; 1",
			want:  "2 > 1",
		},
		{
			name:  "HTML entity &quot; decoded",
			input: "&quot;quoted&quot;",
			want:  `"quoted"`,
		},
		{
			name:  "HTML entity &#39; decoded",
			input: "it&#39;s fine",
			want:  "it's fine",
		},
		{
			name:  "HTML entity &apos; decoded",
			input: "it&apos;s also fine",
			want:  "it's also fine",
		},
		{
			name:  "empty string unchanged",
			input: "",
			want:  "",
		},
		{
			name:  "already clean text unchanged",
			input: "Clean text here.",
			want:  "Clean text here.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cleanPubMedText(tt.input)
			if got != tt.want {
				t.Errorf("cleanPubMedText(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestPubMedBuildAbstract(t *testing.T) {
	engine := NewPubMed()

	tests := []struct {
		name  string
		texts []abstractText
		want  string
	}{
		{
			name:  "empty slice returns empty string",
			texts: []abstractText{},
			want:  "",
		},
		{
			name:  "single text without label",
			texts: []abstractText{{Text: "This is the abstract text."}},
			want:  "This is the abstract text.",
		},
		{
			name:  "single text with label",
			texts: []abstractText{{Label: "BACKGROUND", Text: "Background information here."}},
			want:  "BACKGROUND: Background information here.",
		},
		{
			name: "multiple sections joined",
			texts: []abstractText{
				{Label: "BACKGROUND", Text: "Some background."},
				{Label: "METHODS", Text: "Study methods."},
			},
			want: "BACKGROUND: Some background. METHODS: Study methods.",
		},
		{
			name: "text longer than 500 chars truncated",
			texts: []abstractText{
				{Text: strings.Repeat("x", 600)},
			},
			want: strings.Repeat("x", 497) + "...",
		},
		{
			name:  "whitespace-only text element skipped",
			texts: []abstractText{{Text: "   "}, {Text: "Real content."}},
			want:  "Real content.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := engine.buildAbstract(tt.texts)
			if got != tt.want {
				t.Errorf("buildAbstract() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPubMedBuildAuthorString(t *testing.T) {
	engine := NewPubMed()

	// buildAuthorString is called via articleToResult; test it indirectly through
	// a constructed pubmedArticle so the anonymous struct type matches exactly.
	makeAuthors := func(pairs ...string) pubmedArticle {
		a := pubmedArticle{}
		for i := 0; i+1 < len(pairs); i += 2 {
			a.MedlineCitation.Article.AuthorList.Authors = append(
				a.MedlineCitation.Article.AuthorList.Authors,
				struct {
					LastName string `xml:"LastName"`
					ForeName string `xml:"ForeName"`
				}{LastName: pairs[i], ForeName: pairs[i+1]},
			)
		}
		return a
	}

	tests := []struct {
		name    string
		article pubmedArticle
		want    string
	}{
		{
			name:    "empty authors returns empty string",
			article: pubmedArticle{},
			want:    "",
		},
		{
			name:    "one author with forename initial",
			article: makeAuthors("Smith", "Alice"),
			want:    "Smith A",
		},
		{
			name:    "two authors comma-separated",
			article: makeAuthors("Smith", "Alice", "Jones", "Bob"),
			want:    "Smith A, Jones B",
		},
		{
			name:    "three authors all listed",
			article: makeAuthors("Smith", "Alice", "Jones", "Bob", "White", "Carol"),
			want:    "Smith A, Jones B, White C",
		},
		{
			name:    "four or more authors gets et al.",
			article: makeAuthors("Smith", "Alice", "Jones", "Bob", "White", "Carol", "Brown", "Dave"),
			want:    "Smith A, Jones B, White C, et al.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := engine.buildAuthorString(tt.article.MedlineCitation.Article.AuthorList.Authors)
			if got != tt.want {
				t.Errorf("buildAuthorString() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPubMedParsePublicationDate(t *testing.T) {
	engine := NewPubMed()

	// Use the embedded PubDate from pubmedArticle to get the exact anonymous struct type.
	makePubDate := func(year, month, day string) pubmedArticle {
		a := pubmedArticle{}
		a.MedlineCitation.Article.Journal.JournalIssue.PubDate.Year = year
		a.MedlineCitation.Article.Journal.JournalIssue.PubDate.Month = month
		a.MedlineCitation.Article.Journal.JournalIssue.PubDate.Day = day
		return a
	}

	tests := []struct {
		name    string
		article pubmedArticle
		wantNil bool
		wantY   int
		wantM   time.Month
	}{
		{
			name:    "empty year returns zero time",
			article: makePubDate("", "", ""),
			wantNil: true,
		},
		{
			name:    "year only parsed",
			article: makePubDate("2020", "", ""),
			wantY:   2020,
		},
		{
			name:    "year and numeric month parsed",
			article: makePubDate("2021", "06", ""),
			wantY:   2021,
			wantM:   time.June,
		},
		{
			name:    "year and text month parsed",
			article: makePubDate("2022", "January", ""),
			wantY:   2022,
			wantM:   time.January,
		},
		{
			name:    "year month day parsed",
			article: makePubDate("2023", "03", "15"),
			wantY:   2023,
			wantM:   time.March,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pd := tt.article.MedlineCitation.Article.Journal.JournalIssue.PubDate
			got := engine.parsePublicationDate(pd)
			if tt.wantNil {
				if !got.IsZero() {
					t.Errorf("parsePublicationDate() = %v, want zero time", got)
				}
				return
			}
			if got.Year() != tt.wantY {
				t.Errorf("parsePublicationDate() year = %d, want %d", got.Year(), tt.wantY)
			}
			if tt.wantM != 0 && got.Month() != tt.wantM {
				t.Errorf("parsePublicationDate() month = %v, want %v", got.Month(), tt.wantM)
			}
		})
	}
}

func TestPubMedArticleToResult(t *testing.T) {
	engine := NewPubMed()

	article := pubmedArticle{}
	article.MedlineCitation.PMID.Value = "12345678"
	article.MedlineCitation.Article.ArticleTitle = "Effects of Exercise on Health"
	article.MedlineCitation.Article.Journal.Title = "Journal of Health Sciences"
	article.MedlineCitation.Article.Abstract.AbstractText = []abstractText{
		{Text: "This study examines the effects of regular exercise on human health outcomes."},
	}
	article.MedlineCitation.Article.AuthorList.Authors = []struct {
		LastName string `xml:"LastName"`
		ForeName string `xml:"ForeName"`
	}{{LastName: "Johnson", ForeName: "Mark"}}
	article.MedlineCitation.Article.Journal.JournalIssue.PubDate.Year = "2023"

	result := engine.articleToResult(article, 0)

	if result.Title == "" {
		t.Error("result.Title should not be empty")
	}
	if !strings.Contains(result.URL, "12345678") {
		t.Errorf("result.URL = %q, should contain PMID 12345678", result.URL)
	}
	if !strings.Contains(result.URL, "pubmed.ncbi.nlm.nih.gov") {
		t.Errorf("result.URL = %q, should be a PubMed URL", result.URL)
	}
	if result.Category != model.CategoryScience {
		t.Errorf("result.Category = %q, want %q", result.Category, model.CategoryScience)
	}
	if result.Author == "" {
		t.Error("result.Author should not be empty")
	}
}

// buildPubMedEsearchXML returns a minimal valid esearch XML response.
func buildPubMedEsearchXML(ids []string) string {
	idElems := ""
	for _, id := range ids {
		idElems += "<Id>" + id + "</Id>"
	}
	return `<?xml version="1.0" encoding="UTF-8"?>
<eSearchResult>
	<Count>` + fmt.Sprintf("%d", len(ids)) + `</Count>
	<RetMax>` + fmt.Sprintf("%d", len(ids)) + `</RetMax>
	<RetStart>0</RetStart>
	<IdList>` + idElems + `</IdList>
</eSearchResult>`
}

// buildPubMedEfetchXML returns a minimal valid efetch XML response.
func buildPubMedEfetchXML(pmid, title, journal string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<PubmedArticleSet>
	<PubmedArticle>
		<MedlineCitation>
			<PMID>` + pmid + `</PMID>
			<Article>
				<Journal>
					<Title>` + journal + `</Title>
					<JournalIssue>
						<PubDate>
							<Year>2023</Year>
							<Month>06</Month>
						</PubDate>
					</JournalIssue>
				</Journal>
				<ArticleTitle>` + title + `</ArticleTitle>
				<Abstract>
					<AbstractText>This is the abstract text for the article.</AbstractText>
				</Abstract>
				<AuthorList>
					<Author>
						<LastName>Doe</LastName>
						<ForeName>John</ForeName>
					</Author>
				</AuthorList>
			</Article>
		</MedlineCitation>
	</PubmedArticle>
</PubmedArticleSet>`
}

func TestPubMedSearchReturnsResults(t *testing.T) {
	esearchBody := buildPubMedEsearchXML([]string{"11111111"})
	efetchBody := buildPubMedEfetchXML("11111111", "Cancer Research Study", "Nature Medicine")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusOK)
		if strings.Contains(r.URL.Path, "esearch") {
			fmt.Fprint(w, esearchBody)
		} else {
			fmt.Fprint(w, efetchBody)
		}
	}))
	defer server.Close()

	engine := NewPubMed()
	engine.client = &http.Client{Transport: redirectToServer(server.URL)}

	results, err := engine.Search(context.Background(), &model.Query{Text: "cancer", Page: 1})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(results) == 0 {
		t.Fatal("Search() returned no results")
	}
	if !strings.Contains(results[0].URL, "11111111") {
		t.Errorf("result.URL = %q, should contain PMID", results[0].URL)
	}
}

func TestPubMedSearchEsearchNon200(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	engine := NewPubMed()
	engine.client = &http.Client{Transport: redirectToServer(server.URL)}

	_, err := engine.Search(context.Background(), &model.Query{Text: "test", Page: 1})
	if err == nil {
		t.Error("Search() should return error when esearch returns non-200")
	}
}

func TestPubMedSearchEfetchNon200(t *testing.T) {
	esearchBody := buildPubMedEsearchXML([]string{"22222222"})

	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, esearchBody)
		} else {
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	engine := NewPubMed()
	engine.client = &http.Client{Transport: redirectToServer(server.URL)}

	_, err := engine.Search(context.Background(), &model.Query{Text: "test", Page: 1})
	if err == nil {
		t.Error("Search() should return error when efetch returns non-200")
	}
}

func TestPubMedSearchEmptyIDListReturnsEmpty(t *testing.T) {
	emptyEsearch := buildPubMedEsearchXML([]string{})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, emptyEsearch)
	}))
	defer server.Close()

	engine := NewPubMed()
	engine.client = &http.Client{Transport: redirectToServer(server.URL)}

	results, err := engine.Search(context.Background(), &model.Query{Text: "xyzzy12345", Page: 1})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(results) != 0 {
		t.Errorf("Search() returned %d results for empty ID list, want 0", len(results))
	}
}

func TestPubMedSearchContextTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	engine := NewPubMed()
	engine.client = &http.Client{Transport: redirectToServer(server.URL)}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()

	_, err := engine.Search(ctx, &model.Query{Text: "test", Page: 1})
	if err == nil {
		t.Error("Search() should return error on context timeout")
	}
}

// ============================================================================
// redirectToServer is a test helper that rewrites all HTTP requests to a
// given base URL, allowing engines whose URLs are hardcoded to be redirected
// to an httptest server.
// ============================================================================

// serverRedirectTransport rewrites all outbound requests to the test server base URL.
type serverRedirectTransport struct {
	base string
}

func (t *serverRedirectTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	cloned := req.Clone(req.Context())
	cloned.URL.Scheme = "http"
	cloned.URL.Host = strings.TrimPrefix(t.base, "http://")
	return http.DefaultTransport.RoundTrip(cloned)
}

// redirectToServer returns an http.RoundTripper that rewrites requests to baseURL.
func redirectToServer(baseURL string) http.RoundTripper {
	return &serverRedirectTransport{base: baseURL}
}
