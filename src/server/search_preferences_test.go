package server

import (
	"encoding/base64"
	"net/url"
	"strings"
	"testing"

	"github.com/apimgr/search/src/model"
)

func TestParseSearchPreferencesCompactString(t *testing.T) {
	prefs := parseSearchPreferences("t=l;c=web;s=s;r=50;n=1;p=i;k=0")

	if prefs.Theme != ThemeLight {
		t.Fatalf("Theme = %q, want %q", prefs.Theme, ThemeLight)
	}
	if prefs.DefaultCategory != model.CategoryGeneral {
		t.Fatalf("DefaultCategory = %q, want %q", prefs.DefaultCategory, model.CategoryGeneral)
	}
	if prefs.SafeSearch != 2 {
		t.Fatalf("SafeSearch = %d, want 2", prefs.SafeSearch)
	}
	if prefs.ResultsPerPage != 50 {
		t.Fatalf("ResultsPerPage = %d, want 50", prefs.ResultsPerPage)
	}
	if !prefs.NewTab {
		t.Fatal("NewTab = false, want true")
	}
	if !prefs.InfiniteScroll {
		t.Fatal("InfiniteScroll = false, want true")
	}
	if prefs.KeyboardShortcuts {
		t.Fatal("KeyboardShortcuts = true, want false")
	}
}

func TestAppendDirectoryFilterParamsSupportsNativeForms(t *testing.T) {
	parameters := url.Values{
		"directory_type":     {"documents"},
		"directory_min_size": {"10485760"},
		"directory_after":    {"2024-01-01"},
		"directory_before":   {"2024-12-31"},
		"directory_site":     {"example.edu"},
		"directory_scan":     {"2"},
	}
	got := appendDirectoryFilterParams("directory:linux books", parameters)
	want := "directory:linux books type:documents min-size:10485760 after:2024-01-01 before:2024-12-31 site:example.edu scan:2"
	if got != want {
		t.Fatalf("appendDirectoryFilterParams() = %q, want %q", got, want)
	}
	if got := appendDirectoryFilterParams("golang", parameters); got != "golang" {
		t.Errorf("non-directory query was modified: %q", got)
	}
	parameters.Set("directory_site", "example.edu site:internal")
	if got := appendDirectoryFilterParams("directory:linux", parameters); got == "directory:linux" || got[len("directory:linux"):] != " type:documents min-size:10485760 after:2024-01-01 before:2024-12-31 scan:2" {
		t.Errorf("invalid whitespace filter was not safely omitted: %q", got)
	}
}

func TestBuildAdvancedSearchQueryFromNativeForm(t *testing.T) {
	parameters := url.Values{
		"as_allwords":    {"golang testing"},
		"as_exactphrase": {"unit tests"},
		"as_anywords":    {"fast secure"},
		"as_nonewords":   {"java -ruby"},
		"as_site":        {"example.com"},
		"as_filetype":    {"PDF"},
	}
	got := buildAdvancedSearchQuery(parameters)
	want := `golang testing "unit tests" (fast OR secure) -java -ruby site:example.com filetype:pdf`
	if got != want {
		t.Fatalf("buildAdvancedSearchQuery() = %q, want %q", got, want)
	}
	parameters.Set("as_site", "example.com another.example")
	parameters.Set("as_filetype", "pdf;drop")
	got = buildAdvancedSearchQuery(parameters)
	if strings.Contains(got, "site:") || strings.Contains(got, "filetype:") {
		t.Fatalf("invalid site/filetype values were accepted: %q", got)
	}
}

func TestParseSearchPreferencesBase64JSON(t *testing.T) {
	raw := `{"theme":"auto","default_category":"news","safe_search":0,"results_per_page":30,"new_tab":true,"infinite_scroll":true,"keyboard_shortcuts":false}`
	encoded := base64.RawURLEncoding.EncodeToString([]byte(raw))
	prefs := parseSearchPreferences(encoded)

	if prefs.Theme != ThemeAuto {
		t.Fatalf("Theme = %q, want %q", prefs.Theme, ThemeAuto)
	}
	if prefs.DefaultCategory != model.CategoryNews {
		t.Fatalf("DefaultCategory = %q, want %q", prefs.DefaultCategory, model.CategoryNews)
	}
	if prefs.SafeSearch != 0 {
		t.Fatalf("SafeSearch = %d, want 0", prefs.SafeSearch)
	}
	if prefs.ResultsPerPage != 30 {
		t.Fatalf("ResultsPerPage = %d, want 30", prefs.ResultsPerPage)
	}
	if !prefs.NewTab {
		t.Fatal("NewTab = false, want true")
	}
	if !prefs.InfiniteScroll {
		t.Fatal("InfiniteScroll = false, want true")
	}
	if prefs.KeyboardShortcuts {
		t.Fatal("KeyboardShortcuts = true, want false")
	}
}

func TestParseSearchPreferencesInvalidValuesUseDefaults(t *testing.T) {
	prefs := parseSearchPreferences(`{"theme":"nope","safe_search":9,"results_per_page":1000}`)

	if prefs.Theme != "" {
		t.Fatalf("Theme = %q, want empty", prefs.Theme)
	}
	if prefs.DefaultCategory != model.CategoryGeneral {
		t.Fatalf("DefaultCategory = %q, want %q", prefs.DefaultCategory, model.CategoryGeneral)
	}
	if prefs.SafeSearch != 1 {
		t.Fatalf("SafeSearch = %d, want 1", prefs.SafeSearch)
	}
	if prefs.ResultsPerPage != 100 {
		t.Fatalf("ResultsPerPage = %d, want 100", prefs.ResultsPerPage)
	}
	if !prefs.NewTab {
		t.Fatal("NewTab = false, want true")
	}
	if !prefs.InfiniteScroll {
		t.Fatal("InfiniteScroll = false, want true")
	}
	if !prefs.KeyboardShortcuts {
		t.Fatal("KeyboardShortcuts = false, want true")
	}
}
