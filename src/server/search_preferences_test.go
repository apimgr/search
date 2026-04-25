package server

import (
	"encoding/base64"
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
	if prefs.NewTab {
		t.Fatal("NewTab = true, want false")
	}
	if prefs.InfiniteScroll {
		t.Fatal("InfiniteScroll = true, want false")
	}
	if !prefs.KeyboardShortcuts {
		t.Fatal("KeyboardShortcuts = false, want true")
	}
}
