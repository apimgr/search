package server

import (
	"testing"
)

func TestReportGroupFromPath(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{"csp", "/api/v1/server/reports/csp", "csp"},
		{"nel", "/api/v1/server/reports/nel", "nel"},
		{"default", "/api/v1/server/reports/default", "default"},
		{"trailing slash", "/api/v1/server/reports/csp/", "csp"},
		{"empty name falls back to default", "/api/v1/server/reports/", "default"},
		{"no reports segment", "/api/v1/server/other", "default"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := reportGroupFromPath(tt.path); got != tt.want {
				t.Errorf("reportGroupFromPath(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestExtractReportDirective(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		body        string
		want        string
	}{
		{
			name:        "empty body",
			contentType: "application/csp-report",
			body:        "",
			want:        "-",
		},
		{
			name:        "legacy csp effective-directive",
			contentType: "application/csp-report",
			body:        `{"csp-report":{"effective-directive":"script-src","violated-directive":"script-src 'self'"}}`,
			want:        "script-src",
		},
		{
			name:        "legacy csp falls back to violated-directive",
			contentType: "application/csp-report",
			body:        `{"csp-report":{"violated-directive":"img-src 'self'"}}`,
			want:        "img-src 'self'",
		},
		{
			name:        "reporting api csp-violation",
			contentType: "application/reports+json",
			body:        `[{"type":"csp-violation","body":{"effectiveDirective":"style-src"}}]`,
			want:        "style-src",
		},
		{
			name:        "reporting api nel uses type",
			contentType: "application/reports+json",
			body:        `[{"type":"network-error","body":{}}]`,
			want:        "network-error",
		},
		{
			name:        "garbage body",
			contentType: "application/reports+json",
			body:        `not json`,
			want:        "-",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractReportDirective(tt.contentType, []byte(tt.body))
			if got != tt.want {
				t.Errorf("extractReportDirective() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTruncateDirective(t *testing.T) {
	long := make([]byte, maxReportDirectiveLen+50)
	for i := range long {
		long[i] = 'a'
	}
	if got := truncateDirective(string(long)); len(got) != maxReportDirectiveLen {
		t.Errorf("truncateDirective length = %d, want %d", len(got), maxReportDirectiveLen)
	}
	if got := truncateDirective("a\nb\rc"); got != "a b c" {
		t.Errorf("truncateDirective newline strip = %q, want %q", got, "a b c")
	}
}
