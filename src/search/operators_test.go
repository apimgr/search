package search

import (
	"testing"
)

func TestParseOperatorsSite(t *testing.T) {
	tests := []struct {
		query    string
		wantSite string
	}{
		{"golang site:example.com", "example.com"},
		{"site:github.com tutorials", "github.com"},
		{"no site here", ""},
		{"site:docs.go.dev/pkg", "docs.go.dev/pkg"},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			ops := ParseOperators(tt.query)
			if ops.Site != tt.wantSite {
				t.Errorf("Site = %q, want %q", ops.Site, tt.wantSite)
			}
		})
	}
}

func TestParseOperatorsExcludeSite(t *testing.T) {
	ops := ParseOperators("golang -site:pinterest.com")

	if ops.ExcludeSite != "pinterest.com" {
		t.Errorf("ExcludeSite = %q, want pinterest.com", ops.ExcludeSite)
	}
}

func TestParseOperatorsMultipleSites(t *testing.T) {
	ops := ParseOperators("test site:a.com site:b.com")

	if len(ops.Sites) != 2 {
		t.Errorf("Sites count = %d, want 2", len(ops.Sites))
	}
	if ops.Site != "a.com" {
		t.Errorf("Site (first) = %q, want a.com", ops.Site)
	}
}

func TestParseOperatorsFileType(t *testing.T) {
	tests := []struct {
		query        string
		wantFileType string
	}{
		{"golang filetype:pdf", "pdf"},
		{"report ext:docx", "docx"},
		{"no filetype", ""},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			ops := ParseOperators(tt.query)
			if ops.FileType != tt.wantFileType {
				t.Errorf("FileType = %q, want %q", ops.FileType, tt.wantFileType)
			}
		})
	}
}

func TestParseOperatorsMultipleFileTypes(t *testing.T) {
	ops := ParseOperators("report filetype:pdf filetype:docx")

	if len(ops.FileTypes) != 2 {
		t.Errorf("FileTypes count = %d, want 2", len(ops.FileTypes))
	}
}

func TestParseOperatorsInURL(t *testing.T) {
	ops := ParseOperators("tutorial inurl:golang")

	if ops.InURL != "golang" {
		t.Errorf("InURL = %q, want golang", ops.InURL)
	}
}

func TestParseOperatorsInTitle(t *testing.T) {
	ops := ParseOperators("search intitle:guide")

	if ops.InTitle != "guide" {
		t.Errorf("InTitle = %q, want guide", ops.InTitle)
	}
}

func TestParseOperatorsInText(t *testing.T) {
	ops := ParseOperators("article intext:important")

	if ops.InText != "important" {
		t.Errorf("InText = %q, want important", ops.InText)
	}
}

func TestParseOperatorsInAnchor(t *testing.T) {
	ops := ParseOperators("links inanchor:download")

	if ops.InAnchor != "download" {
		t.Errorf("InAnchor = %q, want download", ops.InAnchor)
	}
}

func TestParseOperatorsExactPhrases(t *testing.T) {
	ops := ParseOperators(`golang "exact phrase" tutorial`)

	if len(ops.ExactPhrases) != 1 {
		t.Fatalf("ExactPhrases count = %d, want 1", len(ops.ExactPhrases))
	}
	if ops.ExactPhrases[0] != "exact phrase" {
		t.Errorf("ExactPhrases[0] = %q, want 'exact phrase'", ops.ExactPhrases[0])
	}
}

func TestParseOperatorsMultipleExactPhrases(t *testing.T) {
	ops := ParseOperators(`"first phrase" and "second phrase"`)

	if len(ops.ExactPhrases) != 2 {
		t.Errorf("ExactPhrases count = %d, want 2", len(ops.ExactPhrases))
	}
}

func TestParseOperatorsExcludeTerms(t *testing.T) {
	ops := ParseOperators("golang -java -python")

	if len(ops.ExcludeTerms) != 2 {
		t.Fatalf("ExcludeTerms count = %d, want 2", len(ops.ExcludeTerms))
	}
}

func TestParseOperatorsRelated(t *testing.T) {
	ops := ParseOperators("related:example.com")

	if ops.Related != "example.com" {
		t.Errorf("Related = %q, want example.com", ops.Related)
	}
}

func TestParseOperatorsCache(t *testing.T) {
	ops := ParseOperators("cache:example.com")

	if ops.Cache != "example.com" {
		t.Errorf("Cache = %q, want example.com", ops.Cache)
	}
}

func TestParseOperatorsInfo(t *testing.T) {
	ops := ParseOperators("info:example.com")

	if ops.Info != "example.com" {
		t.Errorf("Info = %q, want example.com", ops.Info)
	}
}

func TestParseOperatorsDateRange(t *testing.T) {
	ops := ParseOperators("news daterange:2023-2024")

	if ops.DateRange != "2023-2024" {
		t.Errorf("DateRange = %q, want 2023-2024", ops.DateRange)
	}
}

func TestParseOperatorsBefore(t *testing.T) {
	ops := ParseOperators("news before:2024-01-01")

	if ops.Before != "2024-01-01" {
		t.Errorf("Before = %q, want 2024-01-01", ops.Before)
	}
}

func TestParseOperatorsAfter(t *testing.T) {
	ops := ParseOperators("news after:2023-01-01")

	if ops.After != "2023-01-01" {
		t.Errorf("After = %q, want 2023-01-01", ops.After)
	}
}

func TestParseOperatorsNumericRange(t *testing.T) {
	tests := []struct {
		query        string
		wantNumRange string
	}{
		{"laptop $500..$1000", "$500..$1000"},
		{"price 100..500", "100..500"},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			ops := ParseOperators(tt.query)
			if ops.NumRange != tt.wantNumRange {
				t.Errorf("NumRange = %q, want %q", ops.NumRange, tt.wantNumRange)
			}
		})
	}
}

func TestParseOperatorsDefine(t *testing.T) {
	ops := ParseOperators("define:algorithm")

	if ops.Define != "algorithm" {
		t.Errorf("Define = %q, want algorithm", ops.Define)
	}
}

func TestParseOperatorsWeather(t *testing.T) {
	ops := ParseOperators("weather:london")

	if ops.Weather != "london" {
		t.Errorf("Weather = %q, want london", ops.Weather)
	}
}

func TestParseOperatorsStocks(t *testing.T) {
	ops := ParseOperators("stocks:AAPL")

	if ops.Stocks != "AAPL" {
		t.Errorf("Stocks = %q, want AAPL", ops.Stocks)
	}
}

func TestParseOperatorsMap(t *testing.T) {
	ops := ParseOperators("map:paris")

	if ops.Map != "paris" {
		t.Errorf("Map = %q, want paris", ops.Map)
	}
}

func TestParseOperatorsMovie(t *testing.T) {
	ops := ParseOperators("movie:inception")

	if ops.Movie != "inception" {
		t.Errorf("Movie = %q, want inception", ops.Movie)
	}
}

func TestParseOperatorsSource(t *testing.T) {
	ops := ParseOperators("politics source:nytimes")

	if ops.Source != "nytimes" {
		t.Errorf("Source = %q, want nytimes", ops.Source)
	}
}

func TestParseOperatorsLocation(t *testing.T) {
	tests := []struct {
		query        string
		wantLocation string
	}{
		{"news loc:nyc", "nyc"},
		{"restaurants location:london", "london"},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			ops := ParseOperators(tt.query)
			if ops.Location != tt.wantLocation {
				t.Errorf("Location = %q, want %q", ops.Location, tt.wantLocation)
			}
		})
	}
}

func TestParseOperatorsLanguage(t *testing.T) {
	ops := ParseOperators("tutorial lang:de")

	if ops.Language != "de" {
		t.Errorf("Language = %q, want de", ops.Language)
	}
}

func TestParseOperatorsBooleanOR(t *testing.T) {
	ops := ParseOperators("golang OR rust")

	if !ops.HasOR {
		t.Error("HasOR should be true")
	}
}

func TestParseOperatorsBooleanAND(t *testing.T) {
	ops := ParseOperators("golang AND concurrency")

	if !ops.HasAND {
		t.Error("HasAND should be true")
	}
}

func TestParseOperatorsWildcard(t *testing.T) {
	ops := ParseOperators("golang * tutorial")

	if !ops.HasWildcard {
		t.Error("HasWildcard should be true")
	}
}

func TestParseOperatorsCleanedQuery(t *testing.T) {
	ops := ParseOperators("golang site:example.com filetype:pdf")

	if ops.CleanedQuery != "golang" {
		t.Errorf("CleanedQuery = %q, want 'golang'", ops.CleanedQuery)
	}
}

func TestParseOperatorsCleanedQueryMultipleSpaces(t *testing.T) {
	ops := ParseOperators("golang   site:example.com   tutorial")

	// Should normalize spaces
	if ops.CleanedQuery != "golang tutorial" {
		t.Errorf("CleanedQuery = %q, want 'golang tutorial'", ops.CleanedQuery)
	}
}

func TestParseOperatorsOriginalQuery(t *testing.T) {
	query := "golang site:example.com"
	ops := ParseOperators(query)

	if ops.OriginalQuery != query {
		t.Errorf("OriginalQuery = %q, want %q", ops.OriginalQuery, query)
	}
}

func TestSearchOperatorsHasOperators(t *testing.T) {
	tests := []struct {
		name string
		ops  SearchOperators
		want bool
	}{
		{"empty", SearchOperators{}, false},
		{"site", SearchOperators{Site: "example.com"}, true},
		{"filetype", SearchOperators{FileType: "pdf"}, true},
		{"exact phrase", SearchOperators{ExactPhrases: []string{"test"}}, true},
		{"exclude terms", SearchOperators{ExcludeTerms: []string{"spam"}}, true},
		{"OR operator", SearchOperators{HasOR: true}, true},
		{"AND operator", SearchOperators{HasAND: true}, true},
		{"define", SearchOperators{Define: "word"}, true},
		{"weather", SearchOperators{Weather: "london"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.ops.HasOperators(); got != tt.want {
				t.Errorf("HasOperators() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSearchOperatorsToGoogleQuery(t *testing.T) {
	ops := &SearchOperators{
		CleanedQuery:  "golang tutorial",
		Site:          "example.com",
		FileType:      "pdf",
		ExactPhrases:  []string{"best practices"},
		ExcludeTerms:  []string{"java"},
		Before:        "2024-01-01",
		After:         "2023-01-01",
	}

	result := ops.ToGoogleQuery()

	// Check components are present
	expected := []string{
		"golang tutorial",
		"site:example.com",
		"filetype:pdf",
		`"best practices"`,
		"-java",
		"before:2024-01-01",
		"after:2023-01-01",
	}

	for _, exp := range expected {
		found := false
		if containsSubstring(result, exp) {
			found = true
		}
		if !found {
			t.Errorf("ToGoogleQuery() missing %q in result: %q", exp, result)
		}
	}
}

func TestSearchOperatorsToDuckDuckGoQuery(t *testing.T) {
	ops := &SearchOperators{
		CleanedQuery: "golang tutorial",
		Site:         "example.com",
		FileType:     "pdf",
		InTitle:      "guide",
		ExactPhrases: []string{"best practices"},
		ExcludeTerms: []string{"java"},
	}

	result := ops.ToDuckDuckGoQuery()

	// DuckDuckGo supports these
	expected := []string{
		"golang tutorial",
		"site:example.com",
		"filetype:pdf",
		"intitle:guide",
		`"best practices"`,
		"-java",
	}

	for _, exp := range expected {
		if !containsSubstring(result, exp) {
			t.Errorf("ToDuckDuckGoQuery() missing %q in result: %q", exp, result)
		}
	}
}

func TestSearchOperatorsToBingQuery(t *testing.T) {
	ops := &SearchOperators{
		CleanedQuery: "golang tutorial",
		Site:         "example.com",
		FileType:     "pdf",
		InTitle:      "guide",
		InURL:        "docs",
		ExactPhrases: []string{"best practices"},
		ExcludeTerms: []string{"java"},
	}

	result := ops.ToBingQuery()

	expected := []string{
		"golang tutorial",
		"site:example.com",
		"filetype:pdf",
		"intitle:guide",
		"inurl:docs",
		`"best practices"`,
		"-java",
	}

	for _, exp := range expected {
		if !containsSubstring(result, exp) {
			t.Errorf("ToBingQuery() missing %q in result: %q", exp, result)
		}
	}
}

func TestSearchOperatorsToBasicQuery(t *testing.T) {
	ops := &SearchOperators{
		CleanedQuery: "golang tutorial",
		Site:         "example.com", // Should be ignored
		FileType:     "pdf",         // Should be ignored
		ExactPhrases: []string{"best practices"},
	}

	result := ops.ToBasicQuery()

	// Should include cleaned query and exact phrases only
	if !containsSubstring(result, "golang tutorial") {
		t.Errorf("ToBasicQuery() missing cleaned query in result: %q", result)
	}
	if !containsSubstring(result, `"best practices"`) {
		t.Errorf("ToBasicQuery() missing exact phrase in result: %q", result)
	}
	// Should NOT include operators
	if containsSubstring(result, "site:") {
		t.Errorf("ToBasicQuery() should not include site operator: %q", result)
	}
}

func TestParseOperatorsCaseInsensitive(t *testing.T) {
	tests := []struct {
		query    string
		wantSite string
	}{
		{"SITE:example.com", "example.com"},
		{"Site:Example.Com", "Example.Com"},
		{"sItE:EXAMPLE.COM", "EXAMPLE.COM"},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			ops := ParseOperators(tt.query)
			if ops.Site != tt.wantSite {
				t.Errorf("Site = %q, want %q", ops.Site, tt.wantSite)
			}
		})
	}
}

func TestSearchOperatorsStruct(t *testing.T) {
	ops := SearchOperators{
		OriginalQuery: "test query",
		CleanedQuery:  "test query",
		Site:          "example.com",
		Sites:         []string{"example.com", "test.com"},
		ExcludeSite:   "spam.com",
		FileType:      "pdf",
		FileTypes:     []string{"pdf", "docx"},
		InURL:         "docs",
		InTitle:       "guide",
		AllInURL:      "api docs",
		InText:        "important",
		AllInText:     "key terms",
		InAnchor:      "download",
		AllInAnchor:   "link text",
		ExactPhrases:  []string{"exact match"},
		ExcludeTerms:  []string{"exclude"},
		Related:       "related.com",
		Cache:         "cached.com",
		Info:          "info.com",
		DateRange:     "2023-2024",
		Before:        "2024-01-01",
		After:         "2023-01-01",
		NumRange:      "100..500",
		HasOR:         true,
		HasAND:        false,
		Define:        "word",
		Weather:       "london",
		Stocks:        "AAPL",
		Map:           "paris",
		Movie:         "inception",
		Source:        "nytimes",
		Location:      "nyc",
		Language:      "en",
		HasWildcard:   true,
	}

	if ops.OriginalQuery != "test query" {
		t.Errorf("OriginalQuery = %q", ops.OriginalQuery)
	}
	if ops.Site != "example.com" {
		t.Errorf("Site = %q", ops.Site)
	}
	if len(ops.Sites) != 2 {
		t.Errorf("Sites count = %d", len(ops.Sites))
	}
	if !ops.HasOR {
		t.Error("HasOR should be true")
	}
	if ops.HasAND {
		t.Error("HasAND should be false")
	}
}

// Helper function
func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstringHelper(s, substr))
}

func containsSubstringHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
