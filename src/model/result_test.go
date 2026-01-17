package model

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestResultSanitize(t *testing.T) {
	r := &Result{
		Title:       "  Test Title  ",
		URL:         "  https://example.com  ",
		Content:     "  Description  ",
		Engine:      "  google  ",
		Thumbnail:   "  https://example.com/thumb.jpg  ",
		Author:      "  John Doe  ",
		Domain:      "  example.com  ",
		ImageFormat: "  jpeg  ",
		FileType:    "  pdf  ",
		Language:    "  en  ",
	}

	r.Sanitize()

	if r.Title != "Test Title" {
		t.Errorf("Title = %q, want %q", r.Title, "Test Title")
	}
	if r.URL != "https://example.com" {
		t.Errorf("URL = %q, want %q", r.URL, "https://example.com")
	}
	if r.Content != "Description" {
		t.Errorf("Content = %q, want %q", r.Content, "Description")
	}
	if r.Engine != "google" {
		t.Errorf("Engine = %q, want %q", r.Engine, "google")
	}
	if r.Thumbnail != "https://example.com/thumb.jpg" {
		t.Errorf("Thumbnail = %q, want %q", r.Thumbnail, "https://example.com/thumb.jpg")
	}
	if r.Author != "John Doe" {
		t.Errorf("Author = %q, want %q", r.Author, "John Doe")
	}
	if r.Domain != "example.com" {
		t.Errorf("Domain = %q, want %q", r.Domain, "example.com")
	}
	if r.ImageFormat != "jpeg" {
		t.Errorf("ImageFormat = %q, want %q", r.ImageFormat, "jpeg")
	}
	if r.FileType != "pdf" {
		t.Errorf("FileType = %q, want %q", r.FileType, "pdf")
	}
	if r.Language != "en" {
		t.Errorf("Language = %q, want %q", r.Language, "en")
	}
}

func TestResultExtractDomain(t *testing.T) {
	tests := []struct {
		name   string
		url    string
		domain string
		want   string
	}{
		{"https", "https://example.com/page", "", "example.com"},
		{"http", "http://test.org/path", "", "test.org"},
		{"with www", "https://www.example.com/", "", "example.com"},
		{"subdomain", "https://blog.example.com/post", "", "blog.example.com"},
		{"already set", "https://example.com/page", "preset.com", "preset.com"},
		{"no path", "https://example.com", "", "example.com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Result{URL: tt.url, Domain: tt.domain}
			got := r.ExtractDomain()
			if got != tt.want {
				t.Errorf("ExtractDomain() = %q, want %q", got, tt.want)
			}
			// Verify it's also stored
			if r.Domain != tt.want {
				t.Errorf("Domain field = %q, want %q", r.Domain, tt.want)
			}
		})
	}
}

func TestResultAge(t *testing.T) {
	// Test with zero time
	r := &Result{}
	if r.Age() != 0 {
		t.Error("Age() should return 0 for zero PublishedAt")
	}

	// Test with actual time
	r.PublishedAt = time.Now().Add(-2 * time.Hour)
	age := r.Age()
	if age < 2*time.Hour || age > 3*time.Hour {
		t.Errorf("Age() = %v, expected around 2 hours", age)
	}
}

func TestResultIsRecent(t *testing.T) {
	tests := []struct {
		name        string
		publishedAt time.Time
		hours       int
		want        bool
	}{
		{"zero time", time.Time{}, 24, false},
		{"recent", time.Now().Add(-1 * time.Hour), 24, true},
		{"old", time.Now().Add(-48 * time.Hour), 24, false},
		{"exact boundary", time.Now().Add(-23 * time.Hour), 24, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Result{PublishedAt: tt.publishedAt}
			got := r.IsRecent(tt.hours)
			if got != tt.want {
				t.Errorf("IsRecent(%d) = %v, want %v", tt.hours, got, tt.want)
			}
		})
	}
}

func TestNewSearchResults(t *testing.T) {
	sr := NewSearchResults("test query", CategoryGeneral)

	if sr.Query != "test query" {
		t.Errorf("Query = %q, want %q", sr.Query, "test query")
	}
	if sr.Category != CategoryGeneral {
		t.Errorf("Category = %v, want %v", sr.Category, CategoryGeneral)
	}
	if len(sr.Results) != 0 {
		t.Error("Results should be empty initially")
	}
	if len(sr.Engines) != 0 {
		t.Error("Engines should be empty initially")
	}
	if sr.Page != 1 {
		t.Errorf("Page = %d, want %d", sr.Page, 1)
	}
	if sr.PerPage != 20 {
		t.Errorf("PerPage = %d, want %d", sr.PerPage, 20)
	}
	if sr.SortedBy != SortRelevance {
		t.Errorf("SortedBy = %v, want %v", sr.SortedBy, SortRelevance)
	}
	if sr.Domains == nil {
		t.Error("Domains should not be nil")
	}
	if sr.Languages == nil {
		t.Error("Languages should not be nil")
	}
}

func TestSearchResultsAddResult(t *testing.T) {
	sr := NewSearchResults("test", CategoryGeneral)

	result := Result{
		Title:    "  Test Title  ",
		URL:      "https://example.com/page",
		Content:  "Test content",
		Engine:   "google",
		Language: "en",
	}

	sr.AddResult(result)

	if len(sr.Results) != 1 {
		t.Errorf("Results length = %d, want %d", len(sr.Results), 1)
	}
	if sr.TotalResults != 1 {
		t.Errorf("TotalResults = %d, want %d", sr.TotalResults, 1)
	}
	// Check sanitization happened
	if sr.Results[0].Title != "Test Title" {
		t.Errorf("Result title not sanitized: %q", sr.Results[0].Title)
	}
	// Check facets updated
	if sr.Domains["example.com"] != 1 {
		t.Errorf("Domains[example.com] = %d, want %d", sr.Domains["example.com"], 1)
	}
	if sr.Languages["en"] != 1 {
		t.Errorf("Languages[en] = %d, want %d", sr.Languages["en"], 1)
	}
}

func TestSearchResultsAddResults(t *testing.T) {
	sr := NewSearchResults("test", CategoryGeneral)

	results := []Result{
		{Title: "  Result 1  ", URL: "https://a.com", Language: "en"},
		{Title: "  Result 2  ", URL: "https://b.com", Language: "de"},
		{Title: "  Result 3  ", URL: "https://a.com/page", Language: "en"},
	}

	sr.AddResults(results)

	if len(sr.Results) != 3 {
		t.Errorf("Results length = %d, want %d", len(sr.Results), 3)
	}
	if sr.TotalResults != 3 {
		t.Errorf("TotalResults = %d, want %d", sr.TotalResults, 3)
	}
	// Check sanitization
	if sr.Results[0].Title != "Result 1" {
		t.Errorf("Result 0 title not sanitized: %q", sr.Results[0].Title)
	}
	// Check domain facets
	if sr.Domains["a.com"] != 2 {
		t.Errorf("Domains[a.com] = %d, want %d", sr.Domains["a.com"], 2)
	}
	// Check language facets
	if sr.Languages["en"] != 2 {
		t.Errorf("Languages[en] = %d, want %d", sr.Languages["en"], 2)
	}
	if sr.Languages["de"] != 1 {
		t.Errorf("Languages[de] = %d, want %d", sr.Languages["de"], 1)
	}
}

func TestSearchResultsCalculateTotalPages(t *testing.T) {
	tests := []struct {
		name         string
		totalResults int
		perPage      int
		wantPages    int
	}{
		{"exact fit", 20, 10, 2},
		{"partial page", 25, 10, 3},
		{"single page", 5, 10, 1},
		{"zero results", 0, 10, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sr := &SearchResults{
				TotalResults: tt.totalResults,
				PerPage:      tt.perPage,
			}
			sr.CalculateTotalPages()
			if sr.TotalPages != tt.wantPages {
				t.Errorf("TotalPages = %d, want %d", sr.TotalPages, tt.wantPages)
			}
		})
	}
}

func TestSearchResultsGetPage(t *testing.T) {
	sr := NewSearchResults("test", CategoryGeneral)
	sr.PerPage = 2

	// Add 5 results
	for i := 1; i <= 5; i++ {
		sr.Results = append(sr.Results, Result{Title: string(rune('A' + i - 1))})
	}

	tests := []struct {
		page       int
		wantCount  int
		wantTitles []string
	}{
		{1, 2, []string{"A", "B"}},
		{2, 2, []string{"C", "D"}},
		{3, 1, []string{"E"}},
		{4, 0, nil},
	}

	for _, tt := range tests {
		results := sr.GetPage(tt.page)
		if len(results) != tt.wantCount {
			t.Errorf("GetPage(%d) returned %d results, want %d", tt.page, len(results), tt.wantCount)
		}
		for i, r := range results {
			if i < len(tt.wantTitles) && r.Title != tt.wantTitles[i] {
				t.Errorf("GetPage(%d)[%d].Title = %q, want %q", tt.page, i, r.Title, tt.wantTitles[i])
			}
		}
	}
}

func TestSearchResultsToJSON(t *testing.T) {
	sr := NewSearchResults("test", CategoryGeneral)
	sr.AddResult(Result{Title: "Test", URL: "https://example.com", Engine: "google"})

	var buf bytes.Buffer
	err := sr.ToJSON(&buf, false)
	if err != nil {
		t.Fatalf("ToJSON() error = %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, `"query":"test"`) {
		t.Error("JSON output should contain query")
	}
	if !strings.Contains(output, `"title":"Test"`) {
		t.Error("JSON output should contain result title")
	}
}

func TestSearchResultsToJSONPretty(t *testing.T) {
	sr := NewSearchResults("test", CategoryGeneral)
	sr.AddResult(Result{Title: "Test", URL: "https://example.com", Engine: "google"})

	var buf bytes.Buffer
	err := sr.ToJSON(&buf, true)
	if err != nil {
		t.Fatalf("ToJSON(pretty) error = %v", err)
	}

	output := buf.String()
	// Pretty output should have indentation
	if !strings.Contains(output, "\n  ") {
		t.Error("Pretty JSON should have indentation")
	}
}

func TestSearchResultsToCSV(t *testing.T) {
	sr := NewSearchResults("test", CategoryGeneral)
	sr.AddResult(Result{
		Title:       "Test Result",
		URL:         "https://example.com",
		Content:     "Test content",
		Engine:      "google",
		Category:    CategoryGeneral,
		Author:      "John",
		PublishedAt: time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC),
		Score:       0.95,
	})

	var buf bytes.Buffer
	err := sr.ToCSV(&buf)
	if err != nil {
		t.Fatalf("ToCSV() error = %v", err)
	}

	output := buf.String()
	lines := strings.Split(strings.TrimSpace(output), "\n")

	if len(lines) < 2 {
		t.Fatalf("CSV should have at least 2 lines (header + data), got %d", len(lines))
	}

	// Check header
	if !strings.Contains(lines[0], "Title") {
		t.Error("CSV header should contain Title")
	}
	if !strings.Contains(lines[0], "URL") {
		t.Error("CSV header should contain URL")
	}

	// Check data row
	if !strings.Contains(lines[1], "Test Result") {
		t.Error("CSV data should contain result title")
	}
	if !strings.Contains(lines[1], "https://example.com") {
		t.Error("CSV data should contain URL")
	}
}

func TestSearchResultsToCSVNoPublishedAt(t *testing.T) {
	sr := NewSearchResults("test", CategoryGeneral)
	sr.AddResult(Result{
		Title:  "Test Result",
		URL:    "https://example.com",
		Engine: "google",
	})

	var buf bytes.Buffer
	err := sr.ToCSV(&buf)
	if err != nil {
		t.Fatalf("ToCSV() error = %v", err)
	}

	output := buf.String()
	// Should not error and should have empty published field
	if strings.Count(output, "\n") < 1 {
		t.Error("CSV should have content")
	}
}

func TestSearchResultsToRSS(t *testing.T) {
	sr := NewSearchResults("test query", CategoryGeneral)
	sr.Engines = []string{"google", "bing"}
	sr.AddResult(Result{
		Title:       "Test Result",
		URL:         "https://example.com",
		Content:     "Test description",
		Engine:      "google",
		Author:      "John Doe",
		PublishedAt: time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC),
	})

	var buf bytes.Buffer
	err := sr.ToRSS(&buf, "https://search.example.com")
	if err != nil {
		t.Fatalf("ToRSS() error = %v", err)
	}

	output := buf.String()

	if !strings.Contains(output, "<?xml") {
		t.Error("RSS should start with XML declaration")
	}
	if !strings.Contains(output, "<rss") {
		t.Error("RSS should contain <rss> element")
	}
	if !strings.Contains(output, `version="2.0"`) {
		t.Error("RSS should have version 2.0")
	}
	if !strings.Contains(output, "Search results for: test query") {
		t.Error("RSS title should contain query")
	}
	if !strings.Contains(output, "<item>") {
		t.Error("RSS should contain items")
	}
	if !strings.Contains(output, "<title>Test Result</title>") {
		t.Error("RSS item should have title")
	}
}

func TestSearchResultsToRSSNoPubDate(t *testing.T) {
	sr := NewSearchResults("test", CategoryGeneral)
	sr.AddResult(Result{
		Title:  "Test",
		URL:    "https://example.com",
		Engine: "google",
	})

	var buf bytes.Buffer
	err := sr.ToRSS(&buf, "https://search.example.com")
	if err != nil {
		t.Fatalf("ToRSS() error = %v", err)
	}
}

func TestSearchResultsToAtom(t *testing.T) {
	sr := NewSearchResults("test query", CategoryGeneral)
	sr.AddResult(Result{
		Title:       "Test Result",
		URL:         "https://example.com",
		Content:     "Test summary",
		Author:      "John Doe",
		PublishedAt: time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC),
	})

	var buf bytes.Buffer
	err := sr.ToAtom(&buf, "https://search.example.com")
	if err != nil {
		t.Fatalf("ToAtom() error = %v", err)
	}

	output := buf.String()

	if !strings.Contains(output, "<?xml") {
		t.Error("Atom should start with XML declaration")
	}
	if !strings.Contains(output, "<feed") {
		t.Error("Atom should contain <feed> element")
	}
	if !strings.Contains(output, "http://www.w3.org/2005/Atom") {
		t.Error("Atom should have correct namespace")
	}
	if !strings.Contains(output, "<entry>") {
		t.Error("Atom should contain entries")
	}
	if !strings.Contains(output, "<title>Test Result</title>") {
		t.Error("Atom entry should have title")
	}
	if !strings.Contains(output, "<author>") {
		t.Error("Atom entry should have author")
	}
}

func TestSearchResultsToAtomNoAuthor(t *testing.T) {
	sr := NewSearchResults("test", CategoryGeneral)
	sr.AddResult(Result{
		Title:  "Test",
		URL:    "https://example.com",
		Engine: "google",
	})

	var buf bytes.Buffer
	err := sr.ToAtom(&buf, "https://search.example.com")
	if err != nil {
		t.Fatalf("ToAtom() error = %v", err)
	}

	output := buf.String()
	// Should not have author element when author is empty
	if strings.Contains(output, "<author>") {
		t.Error("Atom entry without author should not have <author> element")
	}
}

func TestSearchResultsToAtomNoPubDate(t *testing.T) {
	sr := NewSearchResults("test", CategoryGeneral)
	sr.AddResult(Result{
		Title:  "Test",
		URL:    "https://example.com",
		Engine: "google",
	})

	var buf bytes.Buffer
	err := sr.ToAtom(&buf, "https://search.example.com")
	if err != nil {
		t.Fatalf("ToAtom() error = %v", err)
	}
}

func TestResultStruct(t *testing.T) {
	now := time.Now()
	r := Result{
		Title:          "Test Title",
		URL:            "https://example.com/page",
		Content:        "Test content description",
		Engine:         "google",
		Category:       CategoryImages,
		Thumbnail:      "https://example.com/thumb.jpg",
		Author:         "John Doe",
		PublishedAt:    now,
		Domain:         "example.com",
		ImageWidth:     800,
		ImageHeight:    600,
		ImageFormat:    "jpeg",
		Duration:       120,
		ViewCount:      1000,
		FileSize:       1024,
		FileType:       "pdf",
		Score:          0.95,
		Position:       1,
		Relevance:      0.9,
		Popularity:     0.8,
		DuplicateCount: 2,
		Language:       "en",
		Metadata:       map[string]interface{}{"key": "value"},
	}

	if r.Title != "Test Title" {
		t.Errorf("Title = %q, want %q", r.Title, "Test Title")
	}
	if r.ImageWidth != 800 {
		t.Errorf("ImageWidth = %d, want %d", r.ImageWidth, 800)
	}
	if r.Duration != 120 {
		t.Errorf("Duration = %d, want %d", r.Duration, 120)
	}
	if r.Score != 0.95 {
		t.Errorf("Score = %f, want %f", r.Score, 0.95)
	}
}

func TestSearchResultsStruct(t *testing.T) {
	sr := &SearchResults{
		Query:        "test",
		Category:     CategoryNews,
		Results:      []Result{{Title: "Test"}},
		TotalResults: 1,
		Page:         2,
		PerPage:      10,
		TotalPages:   5,
		SearchTime:   0.5,
		Engines:      []string{"google", "bing"},
		Suggestions:  []string{"test 1", "test 2"},
		SortedBy:     SortDate,
		Domains:      map[string]int{"example.com": 1},
		Languages:    map[string]int{"en": 1},
	}

	if sr.Query != "test" {
		t.Errorf("Query = %q, want %q", sr.Query, "test")
	}
	if sr.Page != 2 {
		t.Errorf("Page = %d, want %d", sr.Page, 2)
	}
	if len(sr.Suggestions) != 2 {
		t.Errorf("Suggestions length = %d, want %d", len(sr.Suggestions), 2)
	}
}

func TestResultJSONMarshal(t *testing.T) {
	r := Result{
		Title:    "Test",
		URL:      "https://example.com",
		Content:  "Description",
		Engine:   "google",
		Category: CategoryGeneral,
	}

	data, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("json.Marshal error = %v", err)
	}

	var decoded Result
	err = json.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("json.Unmarshal error = %v", err)
	}

	if decoded.Title != r.Title {
		t.Errorf("Decoded Title = %q, want %q", decoded.Title, r.Title)
	}
}

func TestRSSStructs(t *testing.T) {
	item := RSSItem{
		Title:       "Test",
		Link:        "https://example.com",
		Description: "Description",
		Author:      "John",
		PubDate:     "Mon, 15 Jan 2024 10:00:00 +0000",
		Source:      "google",
		GUID:        "https://example.com",
	}

	if item.Title != "Test" {
		t.Errorf("RSSItem.Title = %q, want %q", item.Title, "Test")
	}

	channel := RSSChannel{
		Title:         "Test Channel",
		Link:          "https://example.com",
		Description:   "Test",
		Language:      "en",
		LastBuildDate: "Mon, 15 Jan 2024 10:00:00 +0000",
		Items:         []RSSItem{item},
	}

	if channel.Title != "Test Channel" {
		t.Errorf("RSSChannel.Title = %q, want %q", channel.Title, "Test Channel")
	}

	feed := RSSFeed{
		Version: "2.0",
		Channel: channel,
	}

	if feed.Version != "2.0" {
		t.Errorf("RSSFeed.Version = %q, want %q", feed.Version, "2.0")
	}
}
