package model

import "testing"

func TestNewQuery(t *testing.T) {
	query := NewQuery("test search")

	if query.Text != "test search" {
		t.Errorf("Expected text 'test search', got %q", query.Text)
	}

	if query.Category != CategoryGeneral {
		t.Errorf("Expected category 'general', got %v", query.Category)
	}

	if query.Language != "en" {
		t.Errorf("Expected language 'en', got %q", query.Language)
	}

	if query.SafeSearch != 1 {
		t.Errorf("Expected safe_search 1, got %d", query.SafeSearch)
	}

	if query.Page != 1 {
		t.Errorf("Expected page 1, got %d", query.Page)
	}

	if query.PerPage != 20 {
		t.Errorf("Expected per_page 20, got %d", query.PerPage)
	}

	if query.TimeRange != "any" {
		t.Errorf("Expected time_range 'any', got %q", query.TimeRange)
	}
}

func TestQueryValidate(t *testing.T) {
	tests := []struct {
		name    string
		query   *Query
		wantErr bool
	}{
		{
			name:    "valid query",
			query:   NewQuery("test"),
			wantErr: false,
		},
		{
			name:    "empty query",
			query:   NewQuery(""),
			wantErr: true,
		},
		{
			name: "invalid category",
			query: &Query{
				Text:     "test",
				Category: Category("invalid"),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.query.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Query.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestQueryValidateCorrectsPagination(t *testing.T) {
	// Test page correction
	query := NewQuery("test")
	query.Page = 0
	_ = query.Validate()
	if query.Page != 1 {
		t.Errorf("Expected page to be corrected to 1, got %d", query.Page)
	}

	// Test per_page lower bound
	query = NewQuery("test")
	query.PerPage = 0
	_ = query.Validate()
	if query.PerPage != 20 {
		t.Errorf("Expected per_page to be corrected to 20, got %d", query.PerPage)
	}

	// Test per_page upper bound
	query = NewQuery("test")
	query.PerPage = 200
	_ = query.Validate()
	if query.PerPage != 100 {
		t.Errorf("Expected per_page to be corrected to 100, got %d", query.PerPage)
	}

	// Test safe_search correction
	query = NewQuery("test")
	query.SafeSearch = 5
	_ = query.Validate()
	if query.SafeSearch != 1 {
		t.Errorf("Expected safe_search to be corrected to 1, got %d", query.SafeSearch)
	}
}

func TestQueryIsEmpty(t *testing.T) {
	tests := []struct {
		name  string
		text  string
		empty bool
	}{
		{"empty string", "", true},
		{"non-empty string", "test", false},
		{"whitespace", "  ", true}, // whitespace is trimmed by NewQuery, so it becomes empty
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			query := NewQuery(tt.text)
			if query.IsEmpty() != tt.empty {
				t.Errorf("Query.IsEmpty() = %v, want %v", query.IsEmpty(), tt.empty)
			}
		})
	}
}

func TestQueryHasAdvancedFilters(t *testing.T) {
	tests := []struct {
		name       string
		site       string
		fileType   string
		exactTerms string
		hasFilters bool
	}{
		{"no filters", "", "", "", false},
		{"site filter", "example.com", "", "", true},
		{"file type filter", "", "pdf", "", true},
		{"exact terms filter", "", "", "exact phrase", true},
		{"multiple filters", "example.com", "pdf", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			query := NewQuery("test")
			query.Site = tt.site
			query.FileType = tt.fileType
			query.ExactTerms = tt.exactTerms

			if query.HasAdvancedFilters() != tt.hasFilters {
				t.Errorf("Query.HasAdvancedFilters() = %v, want %v",
					query.HasAdvancedFilters(), tt.hasFilters)
			}
		})
	}
}

func TestQueryHasAdvancedFiltersAllFields(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(*Query)
		want    bool
	}{
		{"ExcludeSite", func(q *Query) { q.ExcludeSite = "bad.com" }, true},
		{"FileTypes", func(q *Query) { q.FileTypes = []string{"pdf", "doc"} }, true},
		{"InURL", func(q *Query) { q.InURL = "blog" }, true},
		{"InTitle", func(q *Query) { q.InTitle = "tutorial" }, true},
		{"InText", func(q *Query) { q.InText = "golang" }, true},
		{"ExactPhrases", func(q *Query) { q.ExactPhrases = []string{"exact match"} }, true},
		{"ExcludeTerms", func(q *Query) { q.ExcludeTerms = []string{"spam"} }, true},
		{"DateBefore", func(q *Query) { q.DateBefore = "2024-01-01" }, true},
		{"DateAfter", func(q *Query) { q.DateAfter = "2023-01-01" }, true},
		{"empty FileTypes", func(q *Query) { q.FileTypes = []string{} }, false},
		{"empty ExactPhrases", func(q *Query) { q.ExactPhrases = []string{} }, false},
		{"empty ExcludeTerms", func(q *Query) { q.ExcludeTerms = []string{} }, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			query := NewQuery("test")
			tt.setup(query)
			if query.HasAdvancedFilters() != tt.want {
				t.Errorf("Query.HasAdvancedFilters() = %v, want %v",
					query.HasAdvancedFilters(), tt.want)
			}
		})
	}
}

func TestQueryHasMediaFilters(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*Query)
		want  bool
	}{
		{"no filters", func(q *Query) {}, false},
		{"ImageSize", func(q *Query) { q.ImageSize = "large" }, true},
		{"ImageType", func(q *Query) { q.ImageType = "photo" }, true},
		{"ImageColor", func(q *Query) { q.ImageColor = "color" }, true},
		{"ImageAspect", func(q *Query) { q.ImageAspect = "wide" }, true},
		{"VideoLength", func(q *Query) { q.VideoLength = "short" }, true},
		{"VideoQuality", func(q *Query) { q.VideoQuality = "hd" }, true},
		{"multiple media filters", func(q *Query) {
			q.ImageSize = "large"
			q.VideoQuality = "4k"
		}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			query := NewQuery("test")
			tt.setup(query)
			if query.HasMediaFilters() != tt.want {
				t.Errorf("Query.HasMediaFilters() = %v, want %v",
					query.HasMediaFilters(), tt.want)
			}
		})
	}
}

func TestQueryGetEffectiveText(t *testing.T) {
	tests := []struct {
		name        string
		text        string
		cleanedText string
		want        string
	}{
		{"cleaned text set", "raw query site:example.com", "raw query", "raw query"},
		{"cleaned text empty", "simple query", "", "simple query"},
		{"both empty", "", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			query := &Query{
				Text:        tt.text,
				CleanedText: tt.cleanedText,
			}
			got := query.GetEffectiveText()
			if got != tt.want {
				t.Errorf("Query.GetEffectiveText() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestQuerySanitize(t *testing.T) {
	query := &Query{
		Text:         "  test query  ",
		CleanedText:  "  clean  ",
		Language:     "  en  ",
		Region:       "  us  ",
		TimeRange:    "  week  ",
		Site:         "  example.com  ",
		ExcludeSite:  "  bad.com  ",
		FileType:     "  pdf  ",
		InURL:        "  blog  ",
		InTitle:      "  title  ",
		InText:       "  text  ",
		ExactTerms:   "  exact  ",
		DateBefore:   "  2024-01-01  ",
		DateAfter:    "  2023-01-01  ",
		ImageSize:    "  large  ",
		ImageType:    "  photo  ",
		ImageColor:   "  color  ",
		ImageAspect:  "  wide  ",
		VideoLength:  "  short  ",
		VideoQuality: "  hd  ",
		NewsSource:   "  nytimes  ",
	}

	query.Sanitize()

	if query.Text != "test query" {
		t.Errorf("Text = %q, want %q", query.Text, "test query")
	}
	if query.CleanedText != "clean" {
		t.Errorf("CleanedText = %q, want %q", query.CleanedText, "clean")
	}
	if query.Language != "en" {
		t.Errorf("Language = %q, want %q", query.Language, "en")
	}
	if query.Region != "us" {
		t.Errorf("Region = %q, want %q", query.Region, "us")
	}
	if query.TimeRange != "week" {
		t.Errorf("TimeRange = %q, want %q", query.TimeRange, "week")
	}
	if query.Site != "example.com" {
		t.Errorf("Site = %q, want %q", query.Site, "example.com")
	}
	if query.ExcludeSite != "bad.com" {
		t.Errorf("ExcludeSite = %q, want %q", query.ExcludeSite, "bad.com")
	}
	if query.FileType != "pdf" {
		t.Errorf("FileType = %q, want %q", query.FileType, "pdf")
	}
	if query.InURL != "blog" {
		t.Errorf("InURL = %q, want %q", query.InURL, "blog")
	}
	if query.InTitle != "title" {
		t.Errorf("InTitle = %q, want %q", query.InTitle, "title")
	}
	if query.InText != "text" {
		t.Errorf("InText = %q, want %q", query.InText, "text")
	}
	if query.ExactTerms != "exact" {
		t.Errorf("ExactTerms = %q, want %q", query.ExactTerms, "exact")
	}
	if query.DateBefore != "2024-01-01" {
		t.Errorf("DateBefore = %q, want %q", query.DateBefore, "2024-01-01")
	}
	if query.DateAfter != "2023-01-01" {
		t.Errorf("DateAfter = %q, want %q", query.DateAfter, "2023-01-01")
	}
	if query.ImageSize != "large" {
		t.Errorf("ImageSize = %q, want %q", query.ImageSize, "large")
	}
	if query.ImageType != "photo" {
		t.Errorf("ImageType = %q, want %q", query.ImageType, "photo")
	}
	if query.ImageColor != "color" {
		t.Errorf("ImageColor = %q, want %q", query.ImageColor, "color")
	}
	if query.ImageAspect != "wide" {
		t.Errorf("ImageAspect = %q, want %q", query.ImageAspect, "wide")
	}
	if query.VideoLength != "short" {
		t.Errorf("VideoLength = %q, want %q", query.VideoLength, "short")
	}
	if query.VideoQuality != "hd" {
		t.Errorf("VideoQuality = %q, want %q", query.VideoQuality, "hd")
	}
	if query.NewsSource != "nytimes" {
		t.Errorf("NewsSource = %q, want %q", query.NewsSource, "nytimes")
	}
}

func TestIsValidSortOrder(t *testing.T) {
	tests := []struct {
		order SortOrder
		valid bool
	}{
		{SortRelevance, true},
		{SortDate, true},
		{SortDateAsc, true},
		{SortPopularity, true},
		{SortRandom, true},
		{SortOrder("invalid"), false},
		{SortOrder(""), false},
		{SortOrder("RELEVANCE"), false}, // case sensitive
	}

	for _, tt := range tests {
		t.Run(string(tt.order), func(t *testing.T) {
			got := IsValidSortOrder(tt.order)
			if got != tt.valid {
				t.Errorf("IsValidSortOrder(%q) = %v, want %v", tt.order, got, tt.valid)
			}
		})
	}
}

func TestQueryValidateSortBy(t *testing.T) {
	// Test empty SortBy gets default
	query := NewQuery("test")
	query.SortBy = ""
	_ = query.Validate()
	if query.SortBy != SortRelevance {
		t.Errorf("Empty SortBy should default to SortRelevance, got %q", query.SortBy)
	}

	// Test invalid SortBy gets default
	query = NewQuery("test")
	query.SortBy = SortOrder("invalid")
	_ = query.Validate()
	if query.SortBy != SortRelevance {
		t.Errorf("Invalid SortBy should default to SortRelevance, got %q", query.SortBy)
	}

	// Test valid SortBy is preserved
	query = NewQuery("test")
	query.SortBy = SortDate
	_ = query.Validate()
	if query.SortBy != SortDate {
		t.Errorf("Valid SortBy should be preserved, got %q", query.SortBy)
	}
}

func TestQueryValidateCleanedText(t *testing.T) {
	// Test empty CleanedText gets initialized from Text
	query := &Query{
		Text:        "test query",
		Category:    CategoryGeneral,
		CleanedText: "",
	}
	_ = query.Validate()
	if query.CleanedText != "test query" {
		t.Errorf("CleanedText should be initialized from Text, got %q", query.CleanedText)
	}
}

func TestQueryValidateNegativeSafeSearch(t *testing.T) {
	query := NewQuery("test")
	query.SafeSearch = -1
	_ = query.Validate()
	if query.SafeSearch != 1 {
		t.Errorf("Negative SafeSearch should be corrected to 1, got %d", query.SafeSearch)
	}
}

func TestQueryValidateNegativePage(t *testing.T) {
	query := NewQuery("test")
	query.Page = -5
	_ = query.Validate()
	if query.Page != 1 {
		t.Errorf("Negative Page should be corrected to 1, got %d", query.Page)
	}
}

func TestQueryValidateNegativePerPage(t *testing.T) {
	query := NewQuery("test")
	query.PerPage = -10
	_ = query.Validate()
	if query.PerPage != 20 {
		t.Errorf("Negative PerPage should be corrected to 20, got %d", query.PerPage)
	}
}

func TestNewQueryWithWhitespace(t *testing.T) {
	query := NewQuery("  test query  ")
	if query.Text != "test query" {
		t.Errorf("NewQuery should trim whitespace, got %q", query.Text)
	}
	if query.CleanedText != "test query" {
		t.Errorf("NewQuery should set CleanedText with trimmed text, got %q", query.CleanedText)
	}
}

func TestValidSortOrdersVariable(t *testing.T) {
	// Verify all expected sort orders are in ValidSortOrders
	expected := []SortOrder{SortRelevance, SortDate, SortDateAsc, SortPopularity, SortRandom}
	if len(ValidSortOrders) != len(expected) {
		t.Errorf("ValidSortOrders has %d items, want %d", len(ValidSortOrders), len(expected))
	}

	for _, so := range expected {
		found := false
		for _, v := range ValidSortOrders {
			if v == so {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("ValidSortOrders missing %q", so)
		}
	}
}

func TestQueryStruct(t *testing.T) {
	// Test full struct initialization
	query := &Query{
		Text:           "test search",
		Category:       CategoryImages,
		Language:       "de",
		Region:         "de",
		SafeSearch:     2,
		Page:           3,
		PerPage:        50,
		SortBy:         SortDate,
		TimeRange:      "week",
		Site:           "example.com",
		ExcludeSite:    "spam.com",
		FileType:       "pdf",
		FileTypes:      []string{"pdf", "doc"},
		InURL:          "blog",
		InTitle:        "tutorial",
		InText:         "golang",
		ExactTerms:     "exact phrase",
		ExactPhrases:   []string{"phrase1", "phrase2"},
		ExcludeTerms:   []string{"spam", "ad"},
		DateBefore:     "2024-12-31",
		DateAfter:      "2024-01-01",
		ImageSize:      "large",
		ImageType:      "photo",
		ImageColor:     "color",
		ImageAspect:    "wide",
		VideoLength:    "medium",
		VideoQuality:   "hd",
		NewsSource:     "nytimes",
		Engines:        []string{"google", "bing"},
		ExcludeEngines: []string{"yahoo"},
		CleanedText:    "test search",
	}

	if query.Text != "test search" {
		t.Errorf("Text = %q, want %q", query.Text, "test search")
	}
	if query.Category != CategoryImages {
		t.Errorf("Category = %v, want %v", query.Category, CategoryImages)
	}
	if query.SafeSearch != 2 {
		t.Errorf("SafeSearch = %d, want %d", query.SafeSearch, 2)
	}
	if len(query.FileTypes) != 2 {
		t.Errorf("FileTypes length = %d, want %d", len(query.FileTypes), 2)
	}
	if len(query.Engines) != 2 {
		t.Errorf("Engines length = %d, want %d", len(query.Engines), 2)
	}
	if len(query.ExcludeEngines) != 1 {
		t.Errorf("ExcludeEngines length = %d, want %d", len(query.ExcludeEngines), 1)
	}
}
