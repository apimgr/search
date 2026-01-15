package model

import "testing"

func TestCategoryString(t *testing.T) {
	tests := []struct {
		category Category
		expected string
	}{
		{CategoryGeneral, "general"},
		{CategoryImages, "images"},
		{CategoryVideos, "videos"},
		{CategoryNews, "news"},
		{CategoryMaps, "maps"},
		{CategoryFiles, "files"},
		{CategoryIT, "it"},
		{CategoryScience, "science"},
		{CategorySocial, "social"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if tt.category.String() != tt.expected {
				t.Errorf("Category.String() = %q, want %q", tt.category.String(), tt.expected)
			}
		})
	}
}

func TestCategoryIsValid(t *testing.T) {
	tests := []struct {
		category Category
		valid    bool
	}{
		{CategoryGeneral, true},
		{CategoryImages, true},
		{CategoryVideos, true},
		{CategoryNews, true},
		{CategoryMaps, true},
		{CategoryFiles, true},
		{CategoryIT, true},
		{CategoryScience, true},
		{CategorySocial, true},
		{Category("invalid"), false},
		{Category(""), false},
		{Category("GENERAL"), false}, // case sensitive
	}

	for _, tt := range tests {
		t.Run(string(tt.category), func(t *testing.T) {
			if tt.category.IsValid() != tt.valid {
				t.Errorf("Category(%q).IsValid() = %v, want %v",
					tt.category, tt.category.IsValid(), tt.valid)
			}
		})
	}
}

func TestAllCategories(t *testing.T) {
	categories := AllCategories()

	// Should have 9 categories
	if len(categories) != 9 {
		t.Errorf("AllCategories() returned %d categories, want 9", len(categories))
	}

	// Check that all categories are valid
	for _, cat := range categories {
		if !cat.IsValid() {
			t.Errorf("AllCategories() contains invalid category: %q", cat)
		}
	}

	// Check required categories exist
	required := []Category{
		CategoryGeneral,
		CategoryImages,
		CategoryVideos,
		CategoryNews,
		CategoryMaps,
	}

	for _, req := range required {
		found := false
		for _, cat := range categories {
			if cat == req {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("AllCategories() missing required category: %q", req)
		}
	}
}
