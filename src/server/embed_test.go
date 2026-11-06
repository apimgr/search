package server

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestFormatSearchDate(t *testing.T) {
	if got := formatSearchDate(time.Time{}); got != "" {
		t.Fatalf("formatSearchDate(zero) = %q, want empty string", got)
	}

	value := time.Date(2026, time.April, 23, 22, 4, 44, 0, time.UTC)
	if got := formatSearchDate(value); got != "2026-04-23" {
		t.Fatalf("formatSearchDate() = %q, want 2026-04-23", got)
	}
}

func TestMarkdownToHTMLSanitizesRenderedContent(t *testing.T) {
	got := string(markdownToHTML("<script>alert(1)</script>\n\n[unsafe](javascript:alert(1))\n\n[Search](https://example.com)"))
	for _, forbidden := range []string{"<script", "javascript:", "alert(1)"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("markdown output contains %q: %s", forbidden, got)
		}
	}
	if !strings.Contains(got, `<a href="https://example.com" rel="noopener noreferrer nofollow ugc">Search</a>`) {
		t.Fatalf("safe external link missing required attributes: %s", got)
	}
}

// TestGetStringField covers valid field, missing field, and non-string field.
func TestGetStringField(t *testing.T) {
	type sampleStruct struct {
		Name  string
		Count int
	}
	v := reflect.ValueOf(sampleStruct{Name: "  test  ", Count: 42})
	tests := []struct {
		field string
		want  string
	}{
		{"Name", "test"},
		{"Count", ""},
		{"Missing", ""},
	}
	for _, tt := range tests {
		t.Run(tt.field, func(t *testing.T) {
			got := getStringField(v, tt.field)
			if got != tt.want {
				t.Errorf("getStringField(v, %q) = %q, want %q", tt.field, got, tt.want)
			}
		})
	}
}
