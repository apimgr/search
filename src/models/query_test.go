package models

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
		{"whitespace", "  ", false}, // whitespace is not empty
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
