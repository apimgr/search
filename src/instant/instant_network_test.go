package instant

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// redirectClient returns an *http.Client that redirects all requests to the given test server.
// It relies on the redirectTransport type already defined in instant_test.go.
func redirectClient(srv *httptest.Server) *http.Client {
	return &http.Client{
		Transport: &redirectTransport{target: srv.URL},
		Timeout:   5 * time.Second,
	}
}

// ---- CVE helper functions ----

func TestGetSeverityClass(t *testing.T) {
	tests := []struct {
		severity string
		want     string
	}{
		{"CRITICAL", "severity-critical"},
		{"critical", "severity-critical"},
		{"HIGH", "severity-high"},
		{"high", "severity-high"},
		{"MEDIUM", "severity-medium"},
		{"medium", "severity-medium"},
		{"LOW", "severity-low"},
		{"low", "severity-low"},
		{"NONE", "severity-unknown"},
		{"", "severity-unknown"},
		{"UNKNOWN", "severity-unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.severity, func(t *testing.T) {
			got := getSeverityClass(tt.severity)
			if got != tt.want {
				t.Errorf("getSeverityClass(%q) = %q, want %q", tt.severity, got, tt.want)
			}
		})
	}
}

func TestFormatCVEDate(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"2023-11-15T00:00:00Z", "November 15, 2023"},
		{"not-a-date", "not-a-date"},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := formatCVEDate(tt.input)
			if tt.input == "" || tt.input == "not-a-date" {
				if got != tt.input {
					t.Errorf("formatCVEDate(%q) = %q, want input unchanged %q", tt.input, got, tt.input)
				}
			} else {
				if got != tt.want {
					t.Errorf("formatCVEDate(%q) = %q, want %q", tt.input, got, tt.want)
				}
			}
		})
	}
}

func TestTruncateString(t *testing.T) {
	tests := []struct {
		input  string
		maxLen int
		want   string
	}{
		{"hello", 10, "hello"},
		{"hello", 5, "hello"},
		{"hello world", 8, "hello..."},
		{"", 5, ""},
		{"abcdefgh", 6, "abc..."},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%q/%d", tt.input, tt.maxLen), func(t *testing.T) {
			got := truncateString(tt.input, tt.maxLen)
			if got != tt.want {
				t.Errorf("truncateString(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.want)
			}
		})
	}
}

func TestGetEnglishDescription(t *testing.T) {
	tests := []struct {
		name    string
		entries []struct {
			Lang  string `json:"lang"`
			Value string `json:"value"`
		}
		want string
	}{
		{
			name: "english first",
			entries: []struct {
				Lang  string `json:"lang"`
				Value string `json:"value"`
			}{{"en", "English desc"}, {"fr", "French"}},
			want: "English desc",
		},
		{
			name: "no english falls back to first",
			entries: []struct {
				Lang  string `json:"lang"`
				Value string `json:"value"`
			}{{"fr", "French"}, {"de", "German"}},
			want: "French",
		},
		{
			name: "empty returns empty",
			entries: []struct {
				Lang  string `json:"lang"`
				Value string `json:"value"`
			}{},
			want: "",
		},
		{
			name: "english not first",
			entries: []struct {
				Lang  string `json:"lang"`
				Value string `json:"value"`
			}{{"fr", "French"}, {"en", "English"}},
			want: "English",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getEnglishDescription(tt.entries)
			if got != tt.want {
				t.Errorf("getEnglishDescription() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCVEHandlerExtractCVEID(t *testing.T) {
	h := NewCVEHandler()
	tests := []struct {
		query string
		want  string
	}{
		{"CVE-2021-44228", "CVE-2021-44228"},
		{"cve-2021-44228", "cve-2021-44228"},
		{"not a cve", ""},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			got := h.extractCVEID(tt.query)
			if got != tt.want {
				t.Errorf("extractCVEID(%q) = %q, want %q", tt.query, got, tt.want)
			}
		})
	}
}

func TestCVEHandlerErrorAnswer(t *testing.T) {
	h := NewCVEHandler()
	ans := h.errorAnswer("CVE-2021-44228", "CVE-2021-44228", "test error message")
	if ans == nil {
		t.Fatal("expected non-nil answer")
	}
	if ans.Type != AnswerTypeCVE {
		t.Errorf("Type = %q, want %q", ans.Type, AnswerTypeCVE)
	}
	if !strings.Contains(ans.Content, "test error message") {
		t.Errorf("Content missing error message; got: %s", ans.Content)
	}
}

func TestCVEHandlerHTTPSuccess(t *testing.T) {
	nvdResp := map[string]interface{}{
		"resultsPerPage": 1,
		"startIndex":     0,
		"totalResults":   1,
		"vulnerabilities": []map[string]interface{}{
			{
				"cve": map[string]interface{}{
					"id":           "CVE-2021-44228",
					"published":    "2021-12-10T00:00:00Z",
					"lastModified": "2021-12-15T00:00:00Z",
					"descriptions": []map[string]interface{}{
						{"lang": "en", "value": "Log4Shell critical RCE vulnerability in Apache Log4j2"},
					},
					"metrics": map[string]interface{}{
						"cvssMetricV31": []map[string]interface{}{
							{
								"cvssData": map[string]interface{}{
									"version":      "3.1",
									"vectorString": "CVSS:3.1/AV:N/AC:L",
									"baseScore":    10.0,
									"baseSeverity": "CRITICAL",
								},
							},
						},
					},
					"references": []map[string]interface{}{
						{"url": "https://example.com/ref1", "source": "nvd"},
					},
				},
			},
		},
	}
	body, _ := json.Marshal(nvdResp)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	}))
	defer srv.Close()

	h := NewCVEHandler()
	h.client = redirectClient(srv)

	ans, err := h.HandleInstantQuery(context.Background(), "CVE-2021-44228")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ans == nil {
		t.Fatal("expected non-nil answer")
	}
	if ans.Type != AnswerTypeCVE {
		t.Errorf("Type = %q, want %q", ans.Type, AnswerTypeCVE)
	}
	if !strings.Contains(ans.Content, "Log4Shell") {
		t.Errorf("Content missing description; content len=%d", len(ans.Content))
	}
}

func TestCVEHandlerHTTPRateLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	h := NewCVEHandler()
	h.client = redirectClient(srv)

	ans, err := h.HandleInstantQuery(context.Background(), "CVE-2021-44228")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ans == nil {
		t.Fatal("expected non-nil answer")
	}
	if !strings.Contains(ans.Content, "rate limit") {
		t.Errorf("Content missing rate limit message; got: %s", ans.Content)
	}
}

func TestCVEHandlerHTTPNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := json.Marshal(map[string]interface{}{
			"totalResults":    0,
			"vulnerabilities": []interface{}{},
		})
		w.Write(body)
	}))
	defer srv.Close()

	h := NewCVEHandler()
	h.client = redirectClient(srv)

	ans, err := h.HandleInstantQuery(context.Background(), "CVE-2099-99999")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ans == nil {
		t.Fatal("expected non-nil answer")
	}
}

func TestCVEHandlerHTTPBadStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	h := NewCVEHandler()
	h.client = redirectClient(srv)

	ans, err := h.HandleInstantQuery(context.Background(), "CVE-2021-44228")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ans == nil {
		t.Fatal("expected non-nil error answer")
	}
}

func TestCVEHandlerHTTPBadJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not valid json"))
	}))
	defer srv.Close()

	h := NewCVEHandler()
	h.client = redirectClient(srv)

	ans, err := h.HandleInstantQuery(context.Background(), "CVE-2021-44228")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ans == nil {
		t.Fatal("expected non-nil error answer")
	}
}

func TestCVEHandlerNonCVEQuery(t *testing.T) {
	h := NewCVEHandler()
	ans, err := h.HandleInstantQuery(context.Background(), "not a cve query")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ans != nil {
		t.Error("expected nil answer for non-CVE query")
	}
}

// ---- CheatHandler ----

func TestCheatHandlerHTTPSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("# curl\ncurl -X GET https://example.com\ncurl -X POST -d 'data' https://example.com\n"))
	}))
	defer srv.Close()

	h := NewCheatHandler()
	h.client = redirectClient(srv)

	ans, err := h.HandleInstantQuery(context.Background(), "cheat: curl")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ans == nil {
		t.Fatal("expected non-nil answer")
	}
	if ans.Type != AnswerTypeCheat {
		t.Errorf("Type = %q, want %q", ans.Type, AnswerTypeCheat)
	}
}

func TestCheatHandlerHTTPNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	h := NewCheatHandler()
	h.client = redirectClient(srv)

	ans, err := h.HandleInstantQuery(context.Background(), "cheat: unknowntopic999xyz")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ans == nil {
		t.Fatal("expected non-nil error answer")
	}
	if !strings.Contains(ans.Content, "not found") {
		t.Errorf("Content missing not found message; got: %s", ans.Content)
	}
}

func TestCheatHandlerHTTPRateLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	h := NewCheatHandler()
	h.client = redirectClient(srv)

	ans, err := h.HandleInstantQuery(context.Background(), "cheat: curl")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ans == nil {
		t.Fatal("expected non-nil answer")
	}
	if !strings.Contains(ans.Content, "rate limit") {
		t.Errorf("Content missing rate limit; got: %s", ans.Content)
	}
}

func TestCheatHandlerHTTPBadStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	h := NewCheatHandler()
	h.client = redirectClient(srv)

	ans, err := h.HandleInstantQuery(context.Background(), "cheat: curl")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ans == nil {
		t.Fatal("expected non-nil answer")
	}
}

func TestCheatHandlerUnknownTopicBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("Unknown topic: notreal\n"))
	}))
	defer srv.Close()

	h := NewCheatHandler()
	h.client = redirectClient(srv)

	ans, err := h.HandleInstantQuery(context.Background(), "cheat: notreal")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ans == nil {
		t.Fatal("expected non-nil answer")
	}
}

func TestCheatHandlerEmptyTopic(t *testing.T) {
	h := NewCheatHandler()
	ans, err := h.HandleInstantQuery(context.Background(), "cheat:   ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ans != nil {
		t.Error("expected nil for empty topic")
	}
}

func TestCheatHandlerErrorAnswer(t *testing.T) {
	h := NewCheatHandler()
	ans := h.errorAnswer("cheat: curl", "curl", "connection failed")
	if ans == nil {
		t.Fatal("expected non-nil answer")
	}
	if ans.Type != AnswerTypeCheat {
		t.Errorf("Type = %q, want %q", ans.Type, AnswerTypeCheat)
	}
	if !strings.Contains(ans.Content, "connection failed") {
		t.Errorf("Content missing error; got: %s", ans.Content)
	}
}

// ---- RFC pure helpers ----

func TestContainsMonth(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"January 2006", true},
		{"Published February 2001", true},
		{"March is a month", true},
		{"No dates here", false},
		{"", false},
		{"December", true},
		{"July 4th 2023", true},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := containsMonth(tt.input)
			if got != tt.want {
				t.Errorf("containsMonth(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseRFCList(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"2616, 7230", 2},
		{"1234", 1},
		{"", 0},
		{"  2616  ,  7230  ", 2},
		{"2616,7230,7231", 3},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseRFCList(tt.input)
			if len(got) != tt.want {
				t.Errorf("parseRFCList(%q) returned %d items, want %d; got: %v", tt.input, len(got), tt.want, got)
			}
		})
	}
}

func TestIsRFCSectionHeader(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"abstract", true},
		{"Abstract", true},
		{"introduction", true},
		{"Table of Contents", true},
		{"1. Introduction", true},
		{"2.3 Sub Section", false},
		{"This is a very long paragraph that should not match as it is too descriptive and lengthy", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := isRFCSectionHeader(tt.input)
			if got != tt.want {
				t.Errorf("isRFCSectionHeader(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestIsLikelyTitle(t *testing.T) {
	tests := []struct {
		name  string
		line  string
		lines []string
		index int
		want  bool
	}{
		{
			name:  "too short returns false",
			line:  "Short",
			lines: []string{"", "", "", "", "", "", "Short"},
			index: 6,
			want:  false,
		},
		{
			name:  "line followed by email is title",
			line:  "Hypertext Transfer Protocol Specification",
			lines: []string{"", "", "", "", "", "", "Hypertext Transfer Protocol Specification", "author@example.com"},
			index: 6,
			want:  true,
		},
		{
			name:  "line followed by month is title",
			line:  "The Domain Name System Protocol",
			lines: []string{"", "", "", "", "", "", "The Domain Name System Protocol", "January 2024"},
			index: 6,
			want:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isLikelyTitle(tt.line, tt.lines, tt.index)
			if got != tt.want {
				t.Errorf("isLikelyTitle() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseRFCHeader(t *testing.T) {
	h := NewRFCHandler()
	content := `Network Working Group                                     J. Postel
Request for Comments: 791                                      ISI
                                                        September 1981

                         INTERNET PROTOCOL

                      DARPA INTERNET PROGRAM
                      PROTOCOL SPECIFICATION


Abstract

   This is a test abstract for testing purposes only.

1. Introduction

   Introduction text here.

Category: Standards Track
Obsoletes: 760
Updates: 777, 778
`
	info := h.parseRFCHeader(content)
	if info.Abstract == "" {
		t.Error("expected non-empty abstract")
	}
}

func TestRFCHandlerErrorAnswer(t *testing.T) {
	h := NewRFCHandler()
	ans := h.errorAnswer("rfc 791", "791", "connection refused")
	if ans == nil {
		t.Fatal("expected non-nil answer")
	}
	if ans.Type != AnswerTypeRFC {
		t.Errorf("Type = %q, want %q", ans.Type, AnswerTypeRFC)
	}
	if !strings.Contains(ans.Content, "connection refused") {
		t.Errorf("Content missing error; got: %s", ans.Content)
	}
}

func TestRFCHandlerHTTPSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`Network Working Group
Request for Comments: 791

                 INTERNET PROTOCOL

Abstract

   This is the test abstract content for testing.

Category: Standards Track
`))
	}))
	defer srv.Close()

	h := NewRFCHandler()
	h.client = redirectClient(srv)

	ans, err := h.HandleInstantQuery(context.Background(), "rfc 791")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ans == nil {
		t.Fatal("expected non-nil answer")
	}
	if ans.Type != AnswerTypeRFC {
		t.Errorf("Type = %q, want %q", ans.Type, AnswerTypeRFC)
	}
}

func TestRFCHandlerHTTPNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	h := NewRFCHandler()
	h.client = redirectClient(srv)

	ans, err := h.HandleInstantQuery(context.Background(), "rfc 99999")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ans == nil {
		t.Fatal("expected non-nil error answer")
	}
	if ans.Type != AnswerTypeRFC {
		t.Errorf("Type = %q, want %q", ans.Type, AnswerTypeRFC)
	}
}

func TestRFCHandlerHTTPServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	h := NewRFCHandler()
	h.client = redirectClient(srv)

	ans, err := h.HandleInstantQuery(context.Background(), "rfc 791")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ans == nil {
		t.Fatal("expected non-nil answer")
	}
}

func TestRFCHandlerNonRFCQuery(t *testing.T) {
	h := NewRFCHandler()
	ans, err := h.HandleInstantQuery(context.Background(), "not an rfc query")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ans != nil {
		t.Error("expected nil for non-RFC query")
	}
}

// ---- RobotsHandler ----

func TestRobotsHandlerHTTPSuccess(t *testing.T) {
	robotsContent := `User-agent: *
Disallow: /private/
Disallow: /admin/
Allow: /public/

User-agent: Googlebot
Disallow: /no-google/

Sitemap: https://example.com/sitemap.xml
`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(robotsContent))
	}))
	defer srv.Close()

	h := NewRobotsHandler()
	h.client = redirectClient(srv)

	ans, err := h.HandleInstantQuery(context.Background(), "robots: example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ans == nil {
		t.Fatal("expected non-nil answer")
	}
	if ans.Type != AnswerTypeRobots {
		t.Errorf("Type = %q, want %q", ans.Type, AnswerTypeRobots)
	}
}

func TestRobotsHandlerHTTPNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	h := NewRobotsHandler()
	h.client = redirectClient(srv)

	ans, err := h.HandleInstantQuery(context.Background(), "robots: example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ans == nil {
		t.Fatal("expected non-nil answer")
	}
	if !strings.Contains(ans.Content, "not have a robots.txt") {
		t.Errorf("Content = %s", ans.Content)
	}
}

func TestRobotsHandlerHTTPServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	h := NewRobotsHandler()
	h.client = redirectClient(srv)

	ans, err := h.HandleInstantQuery(context.Background(), "robots: example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ans == nil {
		t.Fatal("expected non-nil answer")
	}
}

func TestRobotsHandlerEmptyQuery(t *testing.T) {
	h := NewRobotsHandler()
	ans, err := h.HandleInstantQuery(context.Background(), "robots:   ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ans != nil {
		t.Error("expected nil for empty domain")
	}
}

// ---- ASNHandler ----

func TestASNHandlerHTTPSuccess(t *testing.T) {
	asnResp := map[string]interface{}{
		"status":         "ok",
		"status_message": "Query was successful",
		"data": map[string]interface{}{
			"asn":               15169,
			"name":              "GOOGLE",
			"description_short": "Google LLC",
			"description_full":  "Google LLC - Mountain View",
			"country_code":      "US",
			"website":           "https://www.google.com",
			"email_contacts":    []string{"abuse@google.com"},
			"abuse_contacts":    []string{"abuse@google.com"},
			"owner_address":     []string{"Google LLC", "Mountain View"},
		},
	}
	body, _ := json.Marshal(asnResp)

	prefixesResp := map[string]interface{}{
		"status": "ok",
		"data": map[string]interface{}{
			"ipv4_prefixes": []map[string]interface{}{
				{"prefix": "8.8.8.0/24", "name": "GOOGLE"},
			},
			"ipv6_prefixes": []interface{}{},
		},
	}
	prefixBody, _ := json.Marshal(prefixesResp)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "prefixes") {
			w.Write(prefixBody)
		} else {
			w.Write(body)
		}
	}))
	defer srv.Close()

	h := NewASNHandler()
	h.client = redirectClient(srv)

	ans, err := h.HandleInstantQuery(context.Background(), "asn 15169")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ans == nil {
		t.Fatal("expected non-nil answer")
	}
	if ans.Type != AnswerTypeASN {
		t.Errorf("Type = %q, want %q", ans.Type, AnswerTypeASN)
	}
	if !strings.Contains(ans.Content, "GOOGLE") {
		t.Errorf("Content missing ASN name")
	}
}

func TestASNHandlerHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	h := NewASNHandler()
	h.client = redirectClient(srv)

	ans, err := h.HandleInstantQuery(context.Background(), "asn 99999")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ans == nil {
		t.Fatal("expected non-nil answer on error")
	}
}

func TestASNHandlerHTTPBadJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("invalid json body"))
	}))
	defer srv.Close()

	h := NewASNHandler()
	h.client = redirectClient(srv)

	ans, err := h.HandleInstantQuery(context.Background(), "asn 15169")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ans == nil {
		t.Fatal("expected non-nil answer on bad JSON")
	}
}

func TestASNHandlerNonASNQuery(t *testing.T) {
	h := NewASNHandler()
	ans, err := h.HandleInstantQuery(context.Background(), "not an asn query")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ans != nil {
		t.Error("expected nil answer for non-ASN query")
	}
}

// ---- HeadersHandler ----

func TestHeadersHandlerHTTPSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Custom-Header", "test-value")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Server", "TestServer/1.0")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	h := NewHeadersHandler()
	h.client = redirectClient(srv)

	ans, err := h.HandleInstantQuery(context.Background(), "headers: example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ans == nil {
		t.Fatal("expected non-nil answer")
	}
	if ans.Type != AnswerTypeHeaders {
		t.Errorf("Type = %q, want %q", ans.Type, AnswerTypeHeaders)
	}
}

func TestHeadersHandlerEmptyURL(t *testing.T) {
	h := NewHeadersHandler()
	ans, err := h.HandleInstantQuery(context.Background(), "headers:   ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ans != nil {
		t.Error("expected nil for empty URL")
	}
}

func TestHeadersHandlerHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	h := NewHeadersHandler()
	h.client = redirectClient(srv)

	ans, err := h.HandleInstantQuery(context.Background(), "headers: example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ans == nil {
		t.Fatal("expected non-nil answer even on error")
	}
}

// ---- SitemapHandler ----

func TestSitemapHandlerHTTPSuccess(t *testing.T) {
	sitemapXML := `<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url><loc>https://example.com/</loc><lastmod>2024-01-01</lastmod></url>
  <url><loc>https://example.com/about</loc></url>
  <url><loc>https://example.com/contact</loc></url>
</urlset>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.Write([]byte(sitemapXML))
	}))
	defer srv.Close()

	h := NewSitemapHandler()
	h.client = redirectClient(srv)

	ans, err := h.HandleInstantQuery(context.Background(), "sitemap: example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ans == nil {
		t.Fatal("expected non-nil answer")
	}
}

func TestSitemapHandlerEmptyQuery(t *testing.T) {
	h := NewSitemapHandler()
	ans, err := h.HandleInstantQuery(context.Background(), "sitemap:   ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ans != nil {
		t.Error("expected nil for empty domain")
	}
}

func TestSitemapHandlerHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	h := NewSitemapHandler()
	h.client = redirectClient(srv)

	ans, err := h.HandleInstantQuery(context.Background(), "sitemap: example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ans == nil {
		t.Fatal("expected non-nil answer on error")
	}
}

// ---- TechHandler ----

func TestTechHandlerHTTPSuccess(t *testing.T) {
	htmlContent := `<!DOCTYPE html>
<html>
<head>
<meta name="generator" content="WordPress 6.0">
<meta name="viewport" content="width=device-width">
<link rel="stylesheet" href="/wp-content/themes/theme/style.css">
</head>
<body>
<script src="/wp-includes/js/jquery/jquery.min.js"></script>
</body>
</html>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("X-Powered-By", "PHP/8.1.0")
		w.Header().Set("Server", "Apache/2.4.54")
		w.Write([]byte(htmlContent))
	}))
	defer srv.Close()

	h := NewTechHandler()
	h.client = redirectClient(srv)

	ans, err := h.HandleInstantQuery(context.Background(), "tech: example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ans == nil {
		t.Fatal("expected non-nil answer")
	}
	if ans.Type != AnswerTypeTech {
		t.Errorf("Type = %q, want %q", ans.Type, AnswerTypeTech)
	}
}

func TestTechHandlerEmptyQuery(t *testing.T) {
	h := NewTechHandler()
	ans, err := h.HandleInstantQuery(context.Background(), "tech:   ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ans != nil {
		t.Error("expected nil for empty URL")
	}
}

func TestTechHandlerHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	h := NewTechHandler()
	h.client = redirectClient(srv)

	ans, err := h.HandleInstantQuery(context.Background(), "tech: example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ans == nil {
		t.Fatal("expected non-nil answer even on error")
	}
}

// ---- FeedHandler ----

func TestFeedHandlerEmptyQuery(t *testing.T) {
	h := NewFeedHandler()
	ans, err := h.HandleInstantQuery(context.Background(), "feed:   ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ans != nil {
		t.Error("expected nil for empty URL")
	}
}

func TestFeedHandlerHTTPRSSFeed(t *testing.T) {
	rssXML := `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>Test RSS Feed</title>
    <link>https://example.com</link>
    <description>A test RSS feed</description>
    <item><title>Post 1</title><link>https://example.com/1</link></item>
    <item><title>Post 2</title><link>https://example.com/2</link></item>
  </channel>
</rss>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		w.Write([]byte(rssXML))
	}))
	defer srv.Close()

	h := NewFeedHandler()
	h.client = redirectClient(srv)

	ans, err := h.HandleInstantQuery(context.Background(), "feed: example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ans == nil {
		t.Fatal("expected non-nil answer")
	}
	if ans.Type != AnswerTypeFeed {
		t.Errorf("Type = %q, want %q", ans.Type, AnswerTypeFeed)
	}
}

// ---- SafeHandler helpers ----

func TestNormalizeURLForSafety(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"example.com", "https://example.com"},
		{"https://example.com", "https://example.com"},
		{"http://example.com", "http://example.com"},
		{"  example.com  ", "https://example.com"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := normalizeURLForSafety(tt.input)
			if got != tt.want {
				t.Errorf("normalizeURLForSafety(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestTruncateSafeURL(t *testing.T) {
	tests := []struct {
		input  string
		maxLen int
		want   string
	}{
		{"https://example.com", 50, "https://example.com"},
		{"https://example.com/very/long/path/that/is/way/too/long/to/display", 30, "https://example.com/very/lo..."},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := truncateSafeURL(tt.input, tt.maxLen)
			if got != tt.want {
				t.Errorf("truncateSafeURL(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.want)
			}
		})
	}
}

func TestSafeHandlerEmptyQuery(t *testing.T) {
	h := NewSafeHandler()
	ans, err := h.HandleInstantQuery(context.Background(), "safe:   ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ans != nil {
		t.Error("expected nil for empty URL")
	}
}

func TestSafeHandlerHTTPSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Strict-Transport-Security", "max-age=31536000")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	h := NewSafeHandler()
	h.client = redirectClient(srv)

	ans, err := h.HandleInstantQuery(context.Background(), "safe: example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ans == nil {
		t.Fatal("expected non-nil answer")
	}
	if ans.Type != AnswerTypeSafe {
		t.Errorf("Type = %q, want %q", ans.Type, AnswerTypeSafe)
	}
}

// ---- ExpandHandler ----

func TestExpandHandlerEmptyQuery(t *testing.T) {
	h := NewExpandHandler()
	ans, err := h.HandleInstantQuery(context.Background(), "expand:   ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ans != nil {
		t.Error("expected nil for empty URL")
	}
}

func TestTruncateURL(t *testing.T) {
	tests := []struct {
		input  string
		maxLen int
		want   string
	}{
		{"https://example.com/short", 50, "https://example.com/short"},
		{"https://example.com/very/long/path/that/exceeds", 30, "https://example.com/very/lo..."},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := truncateURL(tt.input, tt.maxLen)
			if got != tt.want {
				t.Errorf("truncateURL(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.want)
			}
		})
	}
}

func TestExpandHandlerHTTPRedirects(t *testing.T) {
	var srvURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/short":
			http.Redirect(w, r, srvURL+"/intermediate", http.StatusFound)
		case "/intermediate":
			http.Redirect(w, r, srvURL+"/final", http.StatusMovedPermanently)
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()
	srvURL = srv.URL

	h := NewExpandHandler()

	ans, err := h.HandleInstantQuery(context.Background(), "expand: "+srv.URL+"/short")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ans == nil {
		t.Fatal("expected non-nil answer")
	}
	if ans.Type != AnswerTypeExpand {
		t.Errorf("Type = %q, want %q", ans.Type, AnswerTypeExpand)
	}
}

// ---- Regex pure helpers ----

func TestCountCapturingGroups(t *testing.T) {
	tests := []struct {
		pattern string
		want    int
	}{
		{"(abc)", 1},
		{"(abc)(def)", 2},
		{"(?:abc)", 0},
		{"(abc)(?:def)(ghi)", 2},
		{"no groups here", 0},
		{"", 0},
		{"(outer (inner) end)", 2},
	}
	for _, tt := range tests {
		t.Run(tt.pattern, func(t *testing.T) {
			got := countCapturingGroups(tt.pattern)
			if got != tt.want {
				t.Errorf("countCapturingGroups(%q) = %d, want %d", tt.pattern, got, tt.want)
			}
		})
	}
}

func TestExplainRegex(t *testing.T) {
	tests := []struct {
		pattern string
		wantAny []string
	}{
		{"^hello$", []string{"start", "end"}},
		{"\\d+", []string{"digit"}},
		{"\\w*", []string{"word"}},
		{"\\s?", []string{"whitespace"}},
		{"(?:abc)", []string{"non-capturing"}},
		{"[abc]", []string{"character class"}},
		{"a{2,5}", []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.pattern, func(t *testing.T) {
			result := explainRegex(tt.pattern)
			joined := strings.ToLower(strings.Join(result, " "))
			for _, check := range tt.wantAny {
				if !strings.Contains(joined, check) {
					t.Errorf("explainRegex(%q) missing %q; got: %v", tt.pattern, check, result)
				}
			}
		})
	}
}

// ---- Calendar pure helpers ----

func TestDateParserNextWeekday(t *testing.T) {
	p := newDateParser()
	from := time.Date(2024, time.January, 1, 0, 0, 0, 0, time.UTC)

	targets := []time.Weekday{
		time.Monday, time.Tuesday, time.Wednesday,
		time.Thursday, time.Friday, time.Saturday, time.Sunday,
	}
	for _, target := range targets {
		t.Run(target.String(), func(t *testing.T) {
			result := p.nextWeekday(from, target)
			diff := int(result.Sub(from).Hours() / 24)
			if diff < 1 || diff > 7 {
				t.Errorf("nextWeekday from %v to %v: diff=%d, want 1..7", from, target, diff)
			}
			if result.Weekday() != target {
				t.Errorf("nextWeekday returned weekday %v, want %v", result.Weekday(), target)
			}
		})
	}
}

func TestDateParserLastWeekday(t *testing.T) {
	p := newDateParser()
	from := time.Date(2024, time.January, 10, 0, 0, 0, 0, time.UTC)

	targets := []time.Weekday{
		time.Monday, time.Tuesday, time.Wednesday,
		time.Thursday, time.Friday, time.Saturday, time.Sunday,
	}
	for _, target := range targets {
		t.Run(target.String(), func(t *testing.T) {
			result := p.lastWeekday(from, target)
			if !result.Before(from) {
				t.Errorf("lastWeekday returned %v which is not before from=%v", result, from)
			}
			if result.Weekday() != target {
				t.Errorf("lastWeekday returned weekday %v, want %v", result.Weekday(), target)
			}
			diff := int(from.Sub(result).Hours() / 24)
			if diff < 1 || diff > 7 {
				t.Errorf("lastWeekday diff=%d, want 1..7", diff)
			}
		})
	}
}

func TestDateParserParse(t *testing.T) {
	p := newDateParser()
	tests := []struct {
		input   string
		wantErr bool
	}{
		{"today", false},
		{"tomorrow", false},
		{"yesterday", false},
		{"next monday", false},
		{"last friday", false},
		{"2024-01-15", false},
		{"January 15, 2024", false},
		{"01/15/2024", false},
		{"this is definitely not a date xyz123", true},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			_, err := p.parse(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parse(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

// ---- Timezone helpers ----

func TestTimezoneResolveTZ(t *testing.T) {
	h := NewTimezoneHandler()
	tests := []struct {
		input     string
		wantEmpty bool
	}{
		{"utc", false},
		{"gmt", false},
		{"est", false},
		{"pst", false},
		{"america/new_york", false},
		{"london", false},
		{"notarealtzzzz", true},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := h.resolveTZ(tt.input)
			if tt.wantEmpty && got != "" {
				t.Errorf("resolveTZ(%q) = %q, want empty", tt.input, got)
			}
			if !tt.wantEmpty && got == "" {
				t.Errorf("resolveTZ(%q) returned empty, want non-empty", tt.input)
			}
		})
	}
}

func TestTimezoneParseTime(t *testing.T) {
	h := NewTimezoneHandler()
	tests := []struct {
		input   string
		wantErr bool
	}{
		{"10:30am", false},
		{"10:30 am", false},
		{"10am", false},
		{"10 am", false},
		{"14:30", false},
		{"14", false},
		{"not a time xyz", true},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			_, err := h.parseTime(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseTime(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestTimezoneHandlerHandleConversion(t *testing.T) {
	h := NewTimezoneHandler()
	tests := []struct {
		name    string
		timeStr string
		fromTZ  string
		toTZ    string
		wantNil bool
	}{
		{"valid utc to est", "10:00am", "utc", "est", false},
		{"unknown from TZ", "10:00am", "notarealtz", "utc", false},
		{"unknown to TZ", "10:00am", "utc", "notarealtz", false},
		{"invalid time string", "not-a-time-xyz", "utc", "est", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			query := tt.timeStr + " " + tt.fromTZ + " to " + tt.toTZ
			ans, err := h.handleConversion(query, tt.timeStr, tt.fromTZ, tt.toTZ)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantNil && ans != nil {
				t.Error("expected nil answer")
			}
			if !tt.wantNil && ans == nil {
				t.Error("expected non-nil answer")
			}
		})
	}
}

// ---- YAML pure helpers ----

func TestGetYAMLDataType(t *testing.T) {
	tests := []struct {
		name  string
		input interface{}
		want  string
	}{
		{"string", "hello", "String"},
		{"int", 42, "Number"},
		{"float", 3.14, "Number"},
		{"bool true", true, "Boolean"},
		{"bool false", false, "Boolean"},
		{"nil", nil, "Null"},
		{"map 2 keys", map[string]interface{}{"a": 1, "b": 2}, "Object (2 keys)"},
		{"slice 3 items", []interface{}{1, 2, 3}, "Array (3 items)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getYAMLDataType(tt.input)
			if got != tt.want {
				t.Errorf("getYAMLDataType() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGetYAMLTreeDepth(t *testing.T) {
	tests := []struct {
		name  string
		input interface{}
		want  int
	}{
		{"nil", nil, 0},
		{"string", "hello", 0},
		{"flat map", map[string]interface{}{"a": "b"}, 1},
		{"nested map", map[string]interface{}{"a": map[string]interface{}{"b": "c"}}, 2},
		{"flat slice", []interface{}{1, 2, 3}, 1},
		{"nested slice", []interface{}{[]interface{}{1, 2}}, 2},
		{"deeply nested", map[string]interface{}{"a": map[string]interface{}{"b": map[string]interface{}{"c": "d"}}}, 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getYAMLTreeDepth(tt.input)
			if got != tt.want {
				t.Errorf("getYAMLTreeDepth() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestCountYAMLTotalKeys(t *testing.T) {
	tests := []struct {
		name  string
		input interface{}
		want  int
	}{
		{"nil", nil, 0},
		{"flat map 2 keys", map[string]interface{}{"a": 1, "b": 2}, 2},
		{"nested map counts all", map[string]interface{}{"a": map[string]interface{}{"x": 1}}, 2},
		{"slice no keys", []interface{}{1, 2, 3}, 0},
		{"mixed", map[string]interface{}{"a": []interface{}{1, 2}, "b": "c"}, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := countYAMLTotalKeys(tt.input)
			if got != tt.want {
				t.Errorf("countYAMLTotalKeys() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestCountYAMLLists(t *testing.T) {
	tests := []struct {
		name  string
		input interface{}
		want  int
	}{
		{"nil", nil, 0},
		{"string", "hello", 0},
		{"single list", []interface{}{1, 2}, 1},
		{"map with one list", map[string]interface{}{"a": []interface{}{1, 2}}, 1},
		{"map no list", map[string]interface{}{"a": "b"}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := countYAMLLists(tt.input)
			if got != tt.want {
				t.Errorf("countYAMLLists() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestCountYAMLMaps(t *testing.T) {
	tests := []struct {
		name  string
		input interface{}
		want  int
	}{
		{"nil", nil, 0},
		{"string", "hello", 0},
		{"single map", map[string]interface{}{"a": "b"}, 1},
		{"nested maps", map[string]interface{}{"a": map[string]interface{}{"b": "c"}}, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := countYAMLMaps(tt.input)
			if got != tt.want {
				t.Errorf("countYAMLMaps() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestFormatYAMLError(t *testing.T) {
	tests := []struct {
		name string
		line int
		col  int
		want string
	}{
		{"no position uses error text", 0, 0, "test error"},
		{"line only shows line", 5, 0, "line 5"},
		{"line and col shows both", 5, 10, "line 5, column 10"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := fmt.Errorf("test error")
			got := formatYAMLError(err, tt.line, tt.col)
			if !strings.Contains(got, tt.want) {
				t.Errorf("formatYAMLError(_, %d, %d) = %q, want to contain %q", tt.line, tt.col, got, tt.want)
			}
		})
	}
}

// ---- Unicode pure helpers ----

func TestGetUnicodeCategory(t *testing.T) {
	tests := []struct {
		char rune
		want string
	}{
		{'A', "Uppercase"},
		{'a', "Lowercase"},
		{'5', "Digit"},
		{' ', "Space"},
	}
	for _, tt := range tests {
		t.Run(string(tt.char), func(t *testing.T) {
			got := getUnicodeCategory(tt.char)
			if !strings.Contains(got, tt.want) {
				t.Errorf("getUnicodeCategory(%q) = %q, want to contain %q", tt.char, got, tt.want)
			}
		})
	}
}

func TestGetCharacterName(t *testing.T) {
	tests := []struct {
		char rune
		want string
	}{
		{'A', "LATIN CAPITAL LETTER A"},
		{'a', "LATIN SMALL LETTER A"},
		{' ', "SPACE"},
		{'\n', "LINE FEED"},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("U+%04X", tt.char), func(t *testing.T) {
			got := getCharacterName(tt.char)
			if !strings.Contains(got, tt.want) {
				t.Errorf("getCharacterName(%q) = %q, want to contain %q", tt.char, got, tt.want)
			}
		})
	}
}

func TestGetUTF16Encoding(t *testing.T) {
	tests := []struct {
		char rune
		want string
	}{
		{'A', "0x0041"},
		{0x1F600, "0xD83D"},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("U+%04X", tt.char), func(t *testing.T) {
			got := getUTF16Encoding(tt.char)
			if !strings.Contains(got, tt.want) {
				t.Errorf("getUTF16Encoding(%q) = %q, want to contain %q", tt.char, got, tt.want)
			}
		})
	}
}

// ---- Cert helper functions ----

func TestEscapeHTML(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"<script>alert('xss')</script>", "&lt;script&gt;alert(&#39;xss&#39;)&lt;/script&gt;"},
		{"hello & world", "hello &amp; world"},
		{`say "hello"`, "say &quot;hello&quot;"},
		{"plain text", "plain text"},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := escapeHTML(tt.input)
			if got != tt.want {
				t.Errorf("escapeHTML(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestFormatSerialNumber(t *testing.T) {
	tests := []struct {
		bytes []byte
		want  string
	}{
		{[]byte{0x01, 0x02, 0x03}, "01:02:03"},
		{[]byte{0xAB, 0xCD}, "AB:CD"},
		{[]byte{}, ""},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%x", tt.bytes), func(t *testing.T) {
			got := formatSerialNumber(tt.bytes)
			if got != tt.want {
				t.Errorf("formatSerialNumber(%x) = %q, want %q", tt.bytes, got, tt.want)
			}
		})
	}
}

func TestCertHandlerConnectionError(t *testing.T) {
	h := NewCertHandler()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	ans, err := h.HandleInstantQuery(ctx, "cert: 127.0.0.1:1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ans == nil {
		t.Fatal("expected non-nil answer on connection error")
	}
	if ans.Type != AnswerTypeCert {
		t.Errorf("Type = %q, want %q", ans.Type, AnswerTypeCert)
	}
}

func TestCertHandlerEmptyQuery(t *testing.T) {
	h := NewCertHandler()
	ans, err := h.HandleInstantQuery(context.Background(), "cert:   ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ans != nil {
		t.Error("expected nil for empty domain")
	}
}

// ---- WHOISHandler pure helpers ----

func TestWHOISHandlerGetTLD(t *testing.T) {
	h := NewWHOISHandler()
	tests := []struct {
		domain string
		want   string
	}{
		{"example.com", "com"},
		{"sub.example.co.uk", "uk"},
		{"test.org", "org"},
		{"localhost", ""},
		{"example.io", "io"},
	}
	for _, tt := range tests {
		t.Run(tt.domain, func(t *testing.T) {
			got := h.getTLD(tt.domain)
			if got != tt.want {
				t.Errorf("getTLD(%q) = %q, want %q", tt.domain, got, tt.want)
			}
		})
	}
}

func TestWHOISHandlerEmptyQuery(t *testing.T) {
	h := NewWHOISHandler()
	ans, err := h.HandleInstantQuery(context.Background(), "whois:   ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ans != nil {
		t.Error("expected nil for empty domain")
	}
}

// ---- DNSHandler ----

func TestDNSHandlerNonDNSQuery(t *testing.T) {
	h := NewDNSHandler()
	ans, err := h.HandleInstantQuery(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ans != nil {
		t.Error("expected nil for empty query")
	}
}

func TestDNSHandlerInvalidDomain(t *testing.T) {
	h := NewDNSHandler()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	ans, err := h.HandleInstantQuery(ctx, "dns: this.domain.does.not.exist.invalid.example")
	if err != nil {
		t.Logf("error acceptable for DNS lookup: %v", err)
		return
	}
	if ans == nil {
		t.Fatal("expected non-nil answer even on DNS failure")
	}
}

// ---- ResolveHandler ----

func TestResolveHandlerNonQuery(t *testing.T) {
	h := NewResolveHandler()
	ans, err := h.HandleInstantQuery(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ans != nil {
		t.Error("expected nil for empty query")
	}
}

func TestResolveHandlerInvalidDomain(t *testing.T) {
	h := NewResolveHandler()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	ans, err := h.HandleInstantQuery(ctx, "resolve: this.domain.does.not.exist.invalid.example")
	if err != nil {
		t.Logf("error acceptable: %v", err)
		return
	}
	if ans == nil {
		t.Fatal("expected non-nil answer even on failure")
	}
}

// ---- WHOIS pure parsing ----

func TestWHOISParseWHOISResponse(t *testing.T) {
	h := NewWHOISHandler()

	raw := `
Domain Name: EXAMPLE.COM
Registrar: Test Registrar, Inc.
Registrant Organization: Example Corp
Creation Date: 2000-01-01T00:00:00Z
Registry Expiry Date: 2030-01-01T00:00:00Z
Updated Date: 2024-01-01T00:00:00Z
Name Server: NS1.EXAMPLE.COM
Name Server: NS2.EXAMPLE.COM
Domain Status: clientTransferProhibited https://icann.org/epp#clientTransferProhibited
DNSSEC: unsigned
`
	result := h.parseWHOISResponse(raw)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.registrar == "" {
		t.Error("expected registrar to be parsed")
	}
	if len(result.nameServers) < 2 {
		t.Errorf("expected 2 name servers, got %d", len(result.nameServers))
	}
	if result.creationDate == "" {
		t.Error("expected creation date to be parsed")
	}
}

func TestWHOISParseWHOISResponseEmpty(t *testing.T) {
	h := NewWHOISHandler()
	result := h.parseWHOISResponse("")
	if result == nil {
		t.Fatal("expected non-nil result even for empty input")
	}
}

func TestWHOISParseWHOISResponseComments(t *testing.T) {
	h := NewWHOISHandler()
	raw := `% This is a comment
# Another comment

Domain Name: TEST.COM
`
	result := h.parseWHOISResponse(raw)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

// ---- man.go pure helpers ----

func TestParseSeealso(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"ls(1), cp(1), mv(1)", []string{"ls", "cp", "mv"}},
		{"grep(1)", []string{"grep"}},
		{"", nil},
		{"no commands here", nil},
		{"find(1), xargs(1)", []string{"find", "xargs"}},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseSeealso(tt.input)
			if len(got) != len(tt.want) {
				t.Errorf("parseSeealso(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestManHandlerParseQuery(t *testing.T) {
	h := NewManHandler()
	tests := []struct {
		query       string
		wantSection string
		wantCmd     string
	}{
		{"man: ls", "", "ls"},
		{"man: 1 ls", "1", "ls"},
		{"manpage: grep", "", "grep"},
		{"man: 3 printf", "3", "printf"},
		{"not a man query", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			section, cmd := h.parseQuery(tt.query)
			if section != tt.wantSection {
				t.Errorf("parseQuery(%q) section = %q, want %q", tt.query, section, tt.wantSection)
			}
			if cmd != tt.wantCmd {
				t.Errorf("parseQuery(%q) cmd = %q, want %q", tt.query, cmd, tt.wantCmd)
			}
		})
	}
}

func TestManHandlerParseManPageHTML(t *testing.T) {
	h := NewManHandler()
	rawHTML := `<html><body><main>
<h2>NAME</h2>
<p>ls - list directory contents</p>
<h2>SYNOPSIS</h2>
<p>ls [OPTION]... [FILE]...</p>
<h2>DESCRIPTION</h2>
<p>List information about the FILEs (the current directory by default).</p>
<h2>SEE ALSO</h2>
<p>dir(1), vdir(1)</p>
</main></body></html>`
	info := h.parseManPageHTML(rawHTML)
	if info.Name == "" {
		t.Error("expected Name to be parsed")
	}
	if info.Synopsis == "" {
		t.Error("expected Synopsis to be parsed")
	}
	if info.Description == "" {
		t.Error("expected Description to be parsed")
	}
	if len(info.SeeAlso) != 2 {
		t.Errorf("expected 2 SeeAlso entries, got %d: %v", len(info.SeeAlso), info.SeeAlso)
	}
}

// TestManHandlerParseManPageHTMLIgnoresAsideNav verifies that duplicated
// section text inside the <aside> navigation list (which man.cx renders
// alongside the same headings as the <main> content) is not walked, since
// parseManPageHTML only descends into the <main> subtree.
func TestManHandlerParseManPageHTMLIgnoresAsideNav(t *testing.T) {
	h := NewManHandler()
	rawHTML := `<html><body>
<aside><h2>NAME</h2><p>should not appear</p></aside>
<main>
<h2>NAME</h2>
<p>ls - list directory contents</p>
</main>
</body></html>`
	info := h.parseManPageHTML(rawHTML)
	if strings.Contains(info.Name, "should not appear") {
		t.Errorf("parseManPageHTML leaked aside-nav content into Name: %q", info.Name)
	}
	if info.Name == "" {
		t.Error("expected Name to be parsed from main content")
	}
}

func TestManHandlerErrorAnswer(t *testing.T) {
	h := NewManHandler()
	ans := h.errorAnswer("man: ls", "ls", "page not found")
	if ans == nil {
		t.Fatal("expected non-nil answer")
	}
	if !strings.Contains(ans.Content, "page not found") {
		t.Errorf("Content = %s", ans.Content)
	}
}

// ---- cert.go pure helpers ----

func TestFormatKeyUsage(t *testing.T) {
	tests := []struct {
		name  string
		usage x509.KeyUsage
		want  []string
	}{
		{"digital signature", x509.KeyUsageDigitalSignature, []string{"Digital Signature"}},
		{"key encipherment", x509.KeyUsageKeyEncipherment, []string{"Key Encipherment"}},
		{"cert sign", x509.KeyUsageCertSign, []string{"Certificate Sign"}},
		{"crl sign", x509.KeyUsageCRLSign, []string{"CRL Sign"}},
		{"empty", 0, nil},
		{
			"multiple",
			x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
			[]string{"Digital Signature", "Key Encipherment"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatKeyUsage(tt.usage)
			if len(got) != len(tt.want) {
				t.Errorf("formatKeyUsage() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFormatExtKeyUsage(t *testing.T) {
	tests := []struct {
		name  string
		usage []x509.ExtKeyUsage
		want  []string
	}{
		{"server auth", []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, []string{"Server Authentication"}},
		{"client auth", []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}, []string{"Client Authentication"}},
		{"code signing", []x509.ExtKeyUsage{x509.ExtKeyUsageCodeSigning}, []string{"Code Signing"}},
		{"empty", []x509.ExtKeyUsage{}, nil},
		{
			"multiple",
			[]x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
			[]string{"Server Authentication", "Client Authentication"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatExtKeyUsage(tt.usage)
			if len(got) != len(tt.want) {
				t.Errorf("formatExtKeyUsage() = %v, want %v", got, tt.want)
			}
		})
	}
}

// ---- json_handler.go pure helpers ----

func TestGetJSONValueType(t *testing.T) {
	tests := []struct {
		name  string
		input interface{}
		want  string
	}{
		{"object", map[string]interface{}{"a": 1}, "object"},
		{"array", []interface{}{1, 2}, "array"},
		{"string", "hello", "string"},
		{"number", float64(42), "number"},
		{"bool", true, "boolean"},
		{"nil", nil, "null"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getJSONValueType(tt.input)
			if got != tt.want {
				t.Errorf("getJSONValueType() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGetJSONTreeDepth(t *testing.T) {
	tests := []struct {
		name  string
		input interface{}
		want  int
	}{
		{"nil", nil, 0},
		{"string", "hello", 0},
		{"flat object", map[string]interface{}{"a": "b"}, 1},
		{"nested object", map[string]interface{}{"a": map[string]interface{}{"b": "c"}}, 2},
		{"flat array", []interface{}{1, 2, 3}, 1},
		{"nested array", []interface{}{[]interface{}{1, 2}}, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getJSONTreeDepth(tt.input)
			if got != tt.want {
				t.Errorf("getJSONTreeDepth() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestCountJSONTotalKeys(t *testing.T) {
	tests := []struct {
		name  string
		input interface{}
		want  int
	}{
		{"nil", nil, 0},
		{"flat object 2 keys", map[string]interface{}{"a": 1, "b": 2}, 2},
		{"nested object", map[string]interface{}{"a": map[string]interface{}{"x": 1}}, 2},
		{"array no keys", []interface{}{1, 2, 3}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := countJSONTotalKeys(tt.input)
			if got != tt.want {
				t.Errorf("countJSONTotalKeys() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestCountJSONObjects(t *testing.T) {
	tests := []struct {
		name  string
		input interface{}
		want  int
	}{
		{"nil", nil, 0},
		{"single object", map[string]interface{}{"a": "b"}, 1},
		{"nested objects", map[string]interface{}{"a": map[string]interface{}{"b": "c"}}, 2},
		{"array of objects", []interface{}{map[string]interface{}{"a": 1}}, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := countJSONObjects(tt.input)
			if got != tt.want {
				t.Errorf("countJSONObjects() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestCountJSONArrays(t *testing.T) {
	tests := []struct {
		name  string
		input interface{}
		want  int
	}{
		{"nil", nil, 0},
		{"no arrays", map[string]interface{}{"a": "b"}, 0},
		{"one array", []interface{}{1, 2}, 1},
		{"array in object", map[string]interface{}{"a": []interface{}{1, 2}}, 1},
		{"nested arrays", []interface{}{[]interface{}{1}}, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := countJSONArrays(tt.input)
			if got != tt.want {
				t.Errorf("countJSONArrays() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestGetJSONStats(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		wantType string
	}{
		{"object", map[string]interface{}{"a": 1}, "Object"},
		{"array", []interface{}{1, 2, 3}, "Array"},
		{"string", "hello", "String"},
		{"number", float64(42), "Number"},
		{"bool", true, "Boolean"},
		{"nil", nil, "Null"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getJSONStats(tt.input)
			if got.Type != tt.wantType {
				t.Errorf("getJSONStats() Type = %q, want %q", got.Type, tt.wantType)
			}
		})
	}
}

func TestCalculateJSONLineAndPosition(t *testing.T) {
	tests := []struct {
		name     string
		s        string
		offset   int
		wantLine int
	}{
		{"zero offset", "hello", 0, 1},
		{"first line", `{"a": 1}`, 5, 1},
		{"second line", "line1\nline2", 6, 2},
		{"beyond length", "hello", 100, 1},
		{"negative offset", "hello", -1, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			line, _ := calculateJSONLineAndPosition(tt.s, tt.offset)
			if line != tt.wantLine {
				t.Errorf("calculateJSONLineAndPosition(%q, %d) line = %d, want %d", tt.s, tt.offset, line, tt.wantLine)
			}
		})
	}
}

func TestGetJSONErrorLinePosition(t *testing.T) {
	tests := []struct {
		name    string
		jsonStr string
		wantErr string
	}{
		{"syntax error", `{"a": }`, "invalid character"},
		{"valid json", `{"a": 1}`, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var v interface{}
			err := json.Unmarshal([]byte(tt.jsonStr), &v)
			if tt.wantErr != "" && err != nil {
				line, pos := getJSONErrorLinePosition(tt.jsonStr, err)
				if line < 1 {
					t.Errorf("getJSONErrorLinePosition() line = %d, want >= 1", line)
				}
				if pos < 1 {
					t.Errorf("getJSONErrorLinePosition() pos = %d, want >= 1", pos)
				}
			}
		})
	}
}

// ---- pkg.go pure helpers ----

func TestPkgHandlerParseQuery(t *testing.T) {
	h := NewPkgHandler()
	tests := []struct {
		query        string
		wantRegistry string
		wantPkg      string
	}{
		{"npm: express", "npm", "express"},
		{"npm express", "npm", "express"},
		{"pypi: requests", "pypi", "requests"},
		{"pypi requests", "pypi", "requests"},
		{"gopkg: github.com/gin-gonic/gin", "go", "github.com/gin-gonic/gin"},
		{"pkg: npm: lodash", "npm", "lodash"},
		{"pkg: express", "", "express"},
		{"not a pkg query", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			registry, pkg := h.parseQuery(tt.query)
			if registry != tt.wantRegistry {
				t.Errorf("parseQuery(%q) registry = %q, want %q", tt.query, registry, tt.wantRegistry)
			}
			if pkg != tt.wantPkg {
				t.Errorf("parseQuery(%q) pkg = %q, want %q", tt.query, pkg, tt.wantPkg)
			}
		})
	}
}

func TestPkgHandlerDetectRegistry(t *testing.T) {
	h := NewPkgHandler()
	tests := []struct {
		pkgName string
		want    string
	}{
		{"github.com/user/repo", "go"},
		{"golang.org/x/net", "go"},
		{"express", "npm"},
		{"lodash", "npm"},
	}
	for _, tt := range tests {
		t.Run(tt.pkgName, func(t *testing.T) {
			got := h.detectRegistry(tt.pkgName)
			if got != tt.want {
				t.Errorf("detectRegistry(%q) = %q, want %q", tt.pkgName, got, tt.want)
			}
		})
	}
}

func TestPkgHandlerErrorAnswer(t *testing.T) {
	h := NewPkgHandler()
	ans := h.errorAnswer("express", "npm", "package not found")
	if ans == nil {
		t.Fatal("expected non-nil answer")
	}
	if !strings.Contains(ans.Content, "package not found") {
		t.Errorf("Content = %s", ans.Content)
	}
}

func TestCleanGitURL(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"git+https://github.com/user/repo.git", "https://github.com/user/repo"},
		{"https://github.com/user/repo.git", "https://github.com/user/repo"},
		{"git+https://github.com/user/repo", "https://github.com/user/repo"},
		{"https://github.com/user/repo", "https://github.com/user/repo"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := cleanGitURL(tt.input)
			if got != tt.want {
				t.Errorf("cleanGitURL(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestPkgMin(t *testing.T) {
	tests := []struct {
		a, b int
		want int
	}{
		{1, 2, 1},
		{5, 3, 3},
		{4, 4, 4},
		{0, 1, 0},
		{-1, 0, -1},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%d,%d", tt.a, tt.b), func(t *testing.T) {
			got := min(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("min(%d, %d) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestPkgHandlerHTTPNPM(t *testing.T) {
	npmResp := map[string]interface{}{
		"name":        "express",
		"description": "Fast HTTP web framework",
		"license":     "MIT",
		"homepage":    "https://expressjs.com",
		"dist-tags": map[string]interface{}{
			"latest": "4.18.2",
		},
		"versions": map[string]interface{}{
			"4.18.2": map[string]interface{}{
				"dependencies": map[string]interface{}{},
			},
		},
	}
	body, _ := json.Marshal(npmResp)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	}))
	defer srv.Close()

	h := NewPkgHandler()
	h.client = redirectClient(srv)

	ans, err := h.HandleInstantQuery(context.Background(), "npm: express")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ans == nil {
		t.Fatal("expected non-nil answer")
	}
	if !strings.Contains(ans.Content, "express") {
		t.Errorf("Content missing package name; got: %s", ans.Content)
	}
}

func TestPkgHandlerHTTPNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	h := NewPkgHandler()
	h.client = redirectClient(srv)

	ans, err := h.HandleInstantQuery(context.Background(), "npm: nonexistent-pkg-12345")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ans == nil {
		t.Fatal("expected non-nil answer")
	}
}

// ---- feed.go:resolveURL ----

func TestFeedResolveURL(t *testing.T) {
	tests := []struct {
		base string
		href string
		want string
	}{
		{"https://example.com/blog", "https://other.com/feed", "https://other.com/feed"},
		{"https://example.com/blog/", "/feed.xml", "https://example.com/feed.xml"},
		{"https://example.com/blog/", "feed.xml", "https://example.com/blog/feed.xml"},
		{"https://example.com", "/rss", "https://example.com/rss"},
		{"https://example.com", "http://other.com/feed", "http://other.com/feed"},
		{"not a url", "/path", "/path"},
	}
	for _, tt := range tests {
		t.Run(tt.href, func(t *testing.T) {
			got := resolveURL(tt.base, tt.href)
			if got != tt.want {
				t.Errorf("resolveURL(%q, %q) = %q, want %q", tt.base, tt.href, got, tt.want)
			}
		})
	}
}

// ---- ManHandler network tests ----

func TestManHandlerHTTPSuccess(t *testing.T) {
	manContent := `
NAME
       ls - list directory contents

SYNOPSIS
       ls [OPTION]... [FILE]...

DESCRIPTION
       List information about the FILEs.

OPTIONS
       -a
              do not ignore entries starting with .
`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(manContent))
	}))
	defer srv.Close()

	h := NewManHandler()
	h.client = redirectClient(srv)

	ans, err := h.HandleInstantQuery(context.Background(), "man: ls")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ans == nil {
		t.Fatal("expected non-nil answer")
	}
}

func TestManHandlerHTTPNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	h := NewManHandler()
	h.client = redirectClient(srv)

	ans, err := h.HandleInstantQuery(context.Background(), "man: nonexistentcmd999")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ans == nil {
		t.Fatal("expected non-nil error answer")
	}
}

func TestManHandlerHTTPServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	h := NewManHandler()
	h.client = redirectClient(srv)

	ans, err := h.HandleInstantQuery(context.Background(), "man: ls")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ans == nil {
		t.Fatal("expected non-nil answer on server error")
	}
}

func TestManHandlerHTTPNotFoundBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("No manual entry for fakecommand not found"))
	}))
	defer srv.Close()

	h := NewManHandler()
	h.client = redirectClient(srv)

	ans, err := h.HandleInstantQuery(context.Background(), "man: fakecommand")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ans == nil {
		t.Fatal("expected non-nil answer")
	}
}

func TestManHandlerEmptyCommand(t *testing.T) {
	h := NewManHandler()
	ans, err := h.HandleInstantQuery(context.Background(), "not a man query")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ans != nil {
		t.Error("expected nil for non-man query")
	}
}

// ---- MathHandler ----

func TestMathHandlerCanHandleExtended(t *testing.T) {
	h := NewMathHandler()
	tests := []struct {
		query string
		want  bool
	}{
		{"calc: 2+2", true},
		{"math: 5*10", true},
		{"eval: 3^2", true},
		{"2+2", true},
		{"15% of 200", true},
		{"hello world", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			got := h.CanHandle(tt.query)
			if got != tt.want {
				t.Errorf("CanHandle(%q) = %v, want %v", tt.query, got, tt.want)
			}
		})
	}
}

func TestMathHandlerHandleInstantQuery(t *testing.T) {
	h := NewMathHandler()
	tests := []struct {
		query   string
		wantNil bool
	}{
		{"calc: 2+2", false},
		{"math: 10*5", false},
		{"eval: 100/4", false},
		{"15% of 200", false},
		{"calc: sqrt(16)", false},
		{"calc: not_valid_math_abc+xyz!!!", false},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			ans, err := h.HandleInstantQuery(context.Background(), tt.query)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantNil && ans != nil {
				t.Error("expected nil answer")
			}
			if !tt.wantNil && ans == nil {
				t.Error("expected non-nil answer")
			}
		})
	}
}
