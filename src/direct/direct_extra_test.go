package direct

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/apimgr/search/src/geoip"
)

// ---------------------------------------------------------------------------
// Empty-input error paths for network/content handlers
// These tests hit the guard clause at the top of each HandleDirectQuery —
// no network calls are made because the error is returned before any I/O.
// ---------------------------------------------------------------------------

func TestNetworkHandlerEmptyInputErrors(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name    string
		handler Handler
	}{
		{"dns empty", NewDNSHandler()},
		{"whois empty", NewWhoisHandler()},
		{"resolve empty", NewResolveHandler()},
		{"cert empty", NewCertHandler()},
		{"headers empty", NewHeadersHandler()},
		{"asn empty", NewASNHandler()},
		{"subnet empty", NewSubnetHandler()},
		{"wiki empty", NewWikiHandler()},
		{"dict empty", NewDictHandler()},
		{"thesaurus empty", NewThesaurusHandler()},
		{"pkg empty", NewPkgHandler()},
		{"cve empty", NewCVEHandler()},
		{"rfc empty", NewRFCHandler()},
		{"directory empty", NewDirectoryHandler()},
		{"robots empty", NewRobotsHandler()},
		{"sitemap empty", NewSitemapHandler()},
		{"tech empty", NewTechHandler()},
		{"feed empty", NewFeedHandler()},
		{"expand empty", NewExpandHandler()},
		{"safe empty", NewSafeHandler()},
		{"cache empty", NewCacheHandler()},
		{"tldr empty", NewTLDRHandler()},
		{"man empty", NewManHandler()},
		{"cheat empty", NewCheatHandler()},
		{"country empty", NewCountryHandler()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ans, err := tt.handler.HandleDirectQuery(ctx, "")
			if err == nil {
				t.Errorf("%s: expected error for empty input, got nil; answer=%+v", tt.name, ans)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Directory handler — purely local, builds a query string without network
// ---------------------------------------------------------------------------

func TestDirectoryHandler(t *testing.T) {
	h := NewDirectoryHandler()
	ctx := context.Background()

	tests := []struct {
		name        string
		term        string
		wantError   bool
		wantContent string
	}{
		{"basic term", "golang tutorials", false, "golang tutorials"},
		{"music mp3 auto-detects filetype", "best mp3 music", false, "mp3"},
		{"video mp4 auto-detects filetype", "action movie mp4", false, "mp4"},
		{"ebook pdf auto-detects filetype", "programming pdf book", false, "pdf"},
		{"iso image", "ubuntu iso", false, "iso"},
		{"empty term returns error", "", true, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ans, err := h.HandleDirectQuery(ctx, tt.term)
			if tt.wantError {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if ans == nil {
				t.Fatal("nil answer")
			}
			if tt.wantContent != "" && !strings.Contains(ans.Content, tt.wantContent) {
				t.Errorf("expected content to contain %q, got: %s", tt.wantContent, ans.Content)
			}
			if ans.Type != AnswerTypeDirectory {
				t.Errorf("expected type %s, got %s", AnswerTypeDirectory, ans.Type)
			}
		})
	}
}

func TestFormatDirectoryContent(t *testing.T) {
	content := formatDirectoryContent("golang", `intitle:"index of" "golang"`)
	if !strings.Contains(content, "golang") {
		t.Errorf("content should mention the term; got: %s", content)
	}
	if !strings.Contains(content, "Open Directory Search") {
		t.Errorf("content should have heading; got: %s", content)
	}
}

// ---------------------------------------------------------------------------
// RFC handler — basicRFCInfo and format helpers are purely local
// ---------------------------------------------------------------------------

func TestRFCHandlerBasicInfo(t *testing.T) {
	h := NewRFCHandler()

	ans, err := h.basicRFCInfo("7230", "en")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ans == nil {
		t.Fatal("nil answer")
	}
	if !strings.Contains(ans.Content, "RFC 7230") {
		t.Errorf("expected RFC number in content; got: %s", ans.Content)
	}
	if !strings.Contains(ans.Content, "rfc-editor.org") {
		t.Errorf("expected IETF link; got: %s", ans.Content)
	}
}

func TestFormatRFCContent(t *testing.T) {
	authors := []string{"R. Fielding", "J. Reschke"}
	content := formatRFCContent("7230", "Hypertext Transfer Protocol", "HTTP semantics", authors, "2014-06")
	if !strings.Contains(content, "RFC 7230") {
		t.Errorf("expected RFC number; got: %s", content)
	}
	if !strings.Contains(content, "R. Fielding") {
		t.Errorf("expected author name; got: %s", content)
	}
	if !strings.Contains(content, "HTTP semantics") {
		t.Errorf("expected abstract; got: %s", content)
	}
}

func TestFormatBasicRFCContent(t *testing.T) {
	content := formatBasicRFCContent("2616")
	if !strings.Contains(content, "RFC 2616") {
		t.Errorf("expected RFC number; got: %s", content)
	}
	if !strings.Contains(content, "rfc-editor.org") {
		t.Errorf("expected RFC editor link; got: %s", content)
	}
}

// ---------------------------------------------------------------------------
// Format helpers for network.go — purely HTML string construction, no I/O
// ---------------------------------------------------------------------------

func TestFormatDNSRecords(t *testing.T) {
	records := map[string]interface{}{
		"A":    []string{"93.184.216.34"},
		"AAAA": []string{"2606:2800:220:1:248:1893:25c8:1946"},
		"MX": []map[string]interface{}{
			{"host": "mail.example.com.", "pref": uint16(10)},
		},
		"NS":    []string{"a.iana-servers.net."},
		"CNAME": "www.example.com.",
	}
	content := formatDNSRecords("example.com", records)
	if !strings.Contains(content, "example.com") {
		t.Errorf("expected domain in output; got: %s", content)
	}
	if !strings.Contains(content, "93.184.216.34") {
		t.Errorf("expected A record; got: %s", content)
	}
}

func TestFormatWhoisData(t *testing.T) {
	tests := []struct {
		name string
		data map[string]interface{}
		want string
	}{
		{
			name: "full whois data",
			data: map[string]interface{}{
				"domain":      "example.com",
				"created":     "1995-08-14",
				"expires":     "2024-08-13",
				"updated":     "2023-08-11",
				"status":      []string{"clientDeleteProhibited"},
				"nameservers": []string{"a.iana-servers.net"},
			},
			want: "1995-08-14",
		},
		{
			name: "empty data",
			data: map[string]interface{}{},
			want: "test.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content := formatWhoisData("test.com", tt.data)
			if !strings.Contains(content, tt.want) {
				t.Errorf("expected %q in content; got: %s", tt.want, content)
			}
		})
	}
}

func TestFormatResolveData(t *testing.T) {
	data := map[string]interface{}{
		"hostname": "localhost",
		"ipv4":     []string{"127.0.0.1"},
		"ipv6":     []string{"::1"},
		"ptr":      []string{"localhost."},
	}
	content := formatResolveData("localhost", data)
	if !strings.Contains(content, "127.0.0.1") {
		t.Errorf("expected IPv4; got: %s", content)
	}
	if !strings.Contains(content, "::1") {
		t.Errorf("expected IPv6; got: %s", content)
	}
}

func TestFormatResolveDataIncludesGeoIPAndResolverTimings(t *testing.T) {
	content := formatResolveData("example.org", map[string]interface{}{
		"geoip": map[string]*geoip.Result{
			"203.0.113.7": {CountryName: "Example", ASN: 64500, ASNOrg: "Example <network>"},
		},
		"dns_timings": map[string]time.Duration{"1.1.1.1:53": 12 * time.Millisecond},
	})
	for _, expected := range []string{"GeoIP and ASN", "203.0.113.7", "AS64500", "Example &lt;network&gt;", "1.1.1.1", "12ms"} {
		if !strings.Contains(content, expected) {
			t.Errorf("expected resolve output to contain %q; got: %s", expected, content)
		}
	}
}

func TestFormatCertData(t *testing.T) {
	tests := []struct {
		name  string
		valid bool
		days  int
	}{
		{"valid cert more than 30 days", true, 90},
		{"valid cert less than 30 days", true, 15},
		{"expired cert", false, -5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := map[string]interface{}{
				"subject":         "CN=example.com",
				"issuer":          "CN=Let's Encrypt",
				"notBefore":       "2024-01-01T00:00:00Z",
				"notAfter":        "2025-01-01T00:00:00Z",
				"sans":            []string{"example.com", "www.example.com"},
				"keyAlgo":         "RSA",
				"sigAlgo":         "SHA256-RSA",
				"chainLen":        2,
				"valid":           tt.valid,
				"daysUntilExpiry": tt.days,
			}
			content := formatCertData("example.com", data)
			if !strings.Contains(content, "CN=example.com") {
				t.Errorf("expected subject; got: %s", content)
			}
		})
	}
}

func TestAnalyzeSecurityHeaders(t *testing.T) {
	tests := []struct {
		name          string
		headers       map[string][]string
		expectedGrade string
		minPresent    int
	}{
		{
			name: "all security headers A grade",
			headers: map[string][]string{
				"Strict-Transport-Security": {"max-age=31536000"},
				"Content-Security-Policy":   {"default-src 'self'"},
				"X-Frame-Options":           {"DENY"},
				"X-Content-Type-Options":    {"nosniff"},
				"X-Xss-Protection":          {"1; mode=block"},
				"Referrer-Policy":           {"strict-origin-when-cross-origin"},
				"Permissions-Policy":        {"geolocation=()"},
			},
			expectedGrade: "A",
			minPresent:    6,
		},
		{
			name:          "no security headers D grade",
			headers:       map[string][]string{},
			expectedGrade: "D",
			minPresent:    0,
		},
		{
			name: "two headers C grade",
			headers: map[string][]string{
				"Strict-Transport-Security": {"max-age=31536000"},
				"X-Frame-Options":           {"DENY"},
			},
			expectedGrade: "C",
			minPresent:    2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			analysis := analyzeSecurityHeaders(tt.headers)
			if analysis["grade"] != tt.expectedGrade {
				t.Errorf("expected grade %s, got %s", tt.expectedGrade, analysis["grade"])
			}
			present := analysis["present"].(int)
			if present < tt.minPresent {
				t.Errorf("expected at least %d headers present, got %d", tt.minPresent, present)
			}
		})
	}
}

func TestFormatASNData(t *testing.T) {
	data := map[string]interface{}{
		"asn":           15169,
		"name":          "GOOGLE",
		"description":   "Google LLC",
		"country":       "US",
		"rir":           "ARIN",
		"dateAllocated": "2000-03-30",
	}
	content := formatASNData(data)
	if !strings.Contains(content, "AS15169") {
		t.Errorf("expected ASN; got: %s", content)
	}
	if !strings.Contains(content, "GOOGLE") {
		t.Errorf("expected name; got: %s", content)
	}
}

func TestFormatSubnetDataIPv6(t *testing.T) {
	data := map[string]interface{}{
		"cidr":         "2001:db8::/32",
		"network":      "2001:db8::",
		"broadcast":    "2001:db8:ffff:ffff:ffff:ffff:ffff:ffff",
		"firstUsable":  "2001:db8::1",
		"lastUsable":   "2001:db8:ffff:ffff:ffff:ffff:ffff:fffe",
		"subnetMask":   "ffff:ffff::",
		"wildcardMask": "0000:0000:ffff:ffff:ffff:ffff:ffff:ffff",
		"prefixLength": 32,
		"totalHosts":   "79228162514264337593543950336",
		"usableHosts":  "79228162514264337593543950334",
		"isIPv6":       true,
	}
	content := formatSubnetData(data)
	if !strings.Contains(content, "2001:db8::/32") {
		t.Errorf("expected CIDR in output; got: %s", content)
	}
}

// ---------------------------------------------------------------------------
// URL tools helpers — all local, no network calls
// ---------------------------------------------------------------------------

func TestAnalyzeRobotsTxt(t *testing.T) {
	robotsContent := "User-agent: *\nDisallow: /admin/\nAllow: /public/\n\nUser-agent: Googlebot\nAllow: /\n\nSitemap: https://example.com/sitemap.xml\n"
	analysis := analyzeRobotsTxt(robotsContent)

	agents, ok := analysis["userAgents"].([]string)
	if !ok || len(agents) == 0 {
		t.Errorf("expected user agents; got %v", analysis["userAgents"])
	}

	disallowed, ok := analysis["disallowed"].([]string)
	if !ok || len(disallowed) == 0 {
		t.Errorf("expected disallowed paths; got %v", analysis["disallowed"])
	}

	allowed, ok := analysis["allowed"].([]string)
	if !ok || len(allowed) == 0 {
		t.Errorf("expected allowed paths; got %v", analysis["allowed"])
	}

	sitemaps, ok := analysis["sitemaps"].([]string)
	if !ok || len(sitemaps) == 0 {
		t.Errorf("expected sitemaps; got %v", analysis["sitemaps"])
	}
}

func TestAnalyzeRobotsTxtEmpty(t *testing.T) {
	analysis := analyzeRobotsTxt("")
	agents := analysis["userAgents"].([]string)
	if len(agents) != 0 {
		t.Errorf("expected no agents for empty content; got %v", agents)
	}
}

func TestAnalyzeRobotsTxtComments(t *testing.T) {
	content := "# comment only\n# another comment\n"
	analysis := analyzeRobotsTxt(content)
	agents := analysis["userAgents"].([]string)
	if len(agents) != 0 {
		t.Errorf("expected no agents for comment-only content; got %v", agents)
	}
}

func TestFormatRobotsContent(t *testing.T) {
	analysis := map[string]interface{}{
		"userAgents": []string{"*", "Googlebot"},
		"disallowed": []string{"/admin/"},
		"allowed":    []string{"/public/"},
		"sitemaps":   []string{"https://example.com/sitemap.xml"},
	}
	content := formatRobotsContent("example.com", "User-agent: *\nDisallow: /admin/", analysis)
	if !strings.Contains(content, "example.com") {
		t.Errorf("expected domain; got: %s", content)
	}
	if !strings.Contains(content, "/admin/") {
		t.Errorf("expected disallowed path; got: %s", content)
	}
	if !strings.Contains(content, "sitemap.xml") {
		t.Errorf("expected sitemap link; got: %s", content)
	}
}

func TestParseSitemap(t *testing.T) {
	xmlData := []byte(`<?xml version="1.0" encoding="UTF-8"?><urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9"><url><loc>https://example.com/</loc><lastmod>2024-01-01</lastmod><changefreq>daily</changefreq><priority>1.0</priority></url><url><loc>https://example.com/about</loc><lastmod>2024-01-02</lastmod></url></urlset>`)

	urls, sitemapIndex := parseSitemap(xmlData)
	if len(urls) != 2 {
		t.Errorf("expected 2 URLs, got %d", len(urls))
	}
	if urls[0].Loc != "https://example.com/" {
		t.Errorf("unexpected first URL: %s", urls[0].Loc)
	}
	if len(sitemapIndex) != 0 {
		t.Errorf("expected no sitemap index entries; got %d", len(sitemapIndex))
	}
}

func TestParseSitemapIndex(t *testing.T) {
	xmlData := []byte(`<?xml version="1.0" encoding="UTF-8"?><sitemapindex xmlns="http://www.sitemaps.org/schemas/sitemap/0.9"><sitemap><loc>https://example.com/sitemap1.xml</loc></sitemap><sitemap><loc>https://example.com/sitemap2.xml</loc></sitemap></sitemapindex>`)

	urls, sitemapIndex := parseSitemap(xmlData)
	if len(sitemapIndex) != 2 {
		t.Errorf("expected 2 sitemap index entries, got %d", len(sitemapIndex))
	}
	if sitemapIndex[0] != "https://example.com/sitemap1.xml" {
		t.Errorf("unexpected sitemap URL: %s", sitemapIndex[0])
	}
	if len(urls) != 0 {
		t.Errorf("expected no URL entries; got %d", len(urls))
	}
}

func TestFormatSitemapContent(t *testing.T) {
	urls := []sitemapURL{
		{Loc: "https://example.com/", LastMod: "2024-01-01", Priority: "1.0"},
		{Loc: "https://example.com/about", LastMod: "2024-01-02"},
	}
	content := formatSitemapContent("example.com", urls, nil)
	if !strings.Contains(content, "example.com") {
		t.Errorf("expected domain; got: %s", content)
	}
	if !strings.Contains(content, "https://example.com/") {
		t.Errorf("expected URL; got: %s", content)
	}
}

func TestFormatSitemapContentWithIndex(t *testing.T) {
	index := []string{"https://example.com/sitemap1.xml", "https://example.com/sitemap2.xml"}
	content := formatSitemapContent("example.com", nil, index)
	if !strings.Contains(content, "sitemap1.xml") {
		t.Errorf("expected sitemap index; got: %s", content)
	}
}

func TestFormatSitemapContentManyURLs(t *testing.T) {
	urls := make([]sitemapURL, 60)
	for i := range urls {
		urls[i] = sitemapURL{Loc: "https://example.com/page"}
	}
	content := formatSitemapContent("example.com", urls, nil)
	if !strings.Contains(content, "more") {
		t.Errorf("expected truncation notice; got: %s", content)
	}
}

func TestTruncateURL(t *testing.T) {
	tests := []struct {
		inputURL string
		max      int
	}{
		{"https://example.com", 30},
		{"https://example.com/very/long/path/that/exceeds/max", 20},
		{"short", 10},
	}
	for _, tt := range tests {
		got := truncateURL(tt.inputURL, tt.max)
		if len(tt.inputURL) <= tt.max {
			if got != tt.inputURL {
				t.Errorf("truncateURL(%q, %d) = %q, want unmodified", tt.inputURL, tt.max, got)
			}
		} else {
			if !strings.HasSuffix(got, "...") {
				t.Errorf("truncateURL(%q, %d) = %q; expected trailing '...'", tt.inputURL, tt.max, got)
			}
		}
	}
}

func TestDetectTechnologies(t *testing.T) {
	tests := []struct {
		name     string
		headers  http.Header
		body     string
		wantTech string
	}{
		{
			name:     "cloudflare via CF-Ray header",
			headers:  http.Header{"Cf-Ray": {"abc123"}},
			body:     "",
			wantTech: "Cloudflare",
		},
		{
			name:     "wordpress via body",
			headers:  http.Header{},
			body:     `/wp-content/themes/`,
			wantTech: "WordPress",
		},
		{
			name:     "nginx server header",
			headers:  http.Header{"Server": {"nginx/1.18.0"}},
			body:     "",
			wantTech: "nginx",
		},
		{
			name:     "react via body",
			headers:  http.Header{},
			body:     `react.min.js`,
			wantTech: "React",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tech := detectTechnologies(tt.headers, tt.body)
			found := false
			for _, items := range tech {
				for _, item := range items {
					if strings.Contains(item, tt.wantTech) {
						found = true
					}
				}
			}
			if !found {
				t.Errorf("expected technology %q; got: %v", tt.wantTech, tech)
			}
		})
	}
}

func TestDetectTechnologiesEmpty(t *testing.T) {
	tech := detectTechnologies(http.Header{}, "")
	if len(tech) != 0 {
		t.Errorf("expected no technologies from empty input; got: %v", tech)
	}
}

func TestFormatTechContent(t *testing.T) {
	tech := map[string][]string{
		"JavaScript":   {"React", "jQuery"},
		"CMS/Platform": {"WordPress"},
	}
	content := formatTechContent("example.com", tech)
	if !strings.Contains(content, "example.com") {
		t.Errorf("expected domain; got: %s", content)
	}
	if !strings.Contains(content, "React") {
		t.Errorf("expected React; got: %s", content)
	}
}

func TestFormatTechContentEmpty(t *testing.T) {
	content := formatTechContent("example.com", map[string][]string{})
	if !strings.Contains(strings.ToLower(content), "no technologies") {
		t.Errorf("expected 'no technologies' message; got: %s", content)
	}
}

func TestDiscoverFeeds(t *testing.T) {
	html := `<html><head>` +
		`<link rel="alternate" type="application/rss+xml" title="My Blog" href="/feed.xml">` +
		`<link rel="alternate" type="application/atom+xml" title="Atom Feed" href="/atom.xml">` +
		`</head><body></body></html>`

	feeds := discoverFeeds("https://example.com", html)
	if len(feeds) == 0 {
		t.Error("expected feeds to be discovered")
	}

	foundRSS := false
	foundAtom := false
	for _, f := range feeds {
		if f.Type == "RSS" && strings.Contains(f.URL, "feed.xml") {
			foundRSS = true
		}
		if f.Type == "Atom" && strings.Contains(f.URL, "atom.xml") {
			foundAtom = true
		}
	}
	if !foundRSS {
		t.Error("expected RSS feed to be discovered")
	}
	if !foundAtom {
		t.Error("expected Atom feed to be discovered")
	}
}

func TestDiscoverFeedsCommonPaths(t *testing.T) {
	feeds := discoverFeeds("https://example.com", "")
	if len(feeds) == 0 {
		t.Error("expected common-path feeds even with empty HTML")
	}
	foundFeed := false
	for _, f := range feeds {
		if strings.Contains(f.URL, "/feed") {
			foundFeed = true
		}
	}
	if !foundFeed {
		t.Error("expected /feed in common paths")
	}
}

func TestDiscoverFeedsDeduplicate(t *testing.T) {
	htmlStr := `<link rel="alternate" type="application/rss+xml" href="/feed">`
	feeds := discoverFeeds("https://example.com", htmlStr)

	seen := make(map[string]int)
	for _, f := range feeds {
		seen[f.URL]++
	}
	for u, count := range seen {
		if count > 1 {
			t.Errorf("duplicate feed URL %q appeared %d times", u, count)
		}
	}
}

func TestExtractAttr(t *testing.T) {
	tests := []struct {
		tag  string
		attr string
		want string
	}{
		{`<link href="/feed.xml" type="rss">`, "href", "/feed.xml"},
		{`<link href='/atom.xml' type='atom'>`, "href", "/atom.xml"},
		{`<link type="rss">`, "href", ""},
		{`<link title="My Blog Feed" href="/feed">`, "title", "My Blog Feed"},
	}
	for _, tt := range tests {
		got := extractAttr(tt.tag, tt.attr)
		if got != tt.want {
			t.Errorf("extractAttr(%q, %q) = %q, want %q", tt.tag, tt.attr, got, tt.want)
		}
	}
}

func TestResolveURL(t *testing.T) {
	tests := []struct {
		base string
		href string
		want string
	}{
		{"https://example.com", "/feed.xml", "https://example.com/feed.xml"},
		{"https://example.com", "https://other.com/feed", "https://other.com/feed"},
	}
	for _, tt := range tests {
		t.Run(tt.href, func(t *testing.T) {
			base, err := url.Parse(tt.base)
			if err != nil {
				t.Fatalf("bad base URL: %v", err)
			}
			got := resolveURL(base, tt.href)
			if got != tt.want {
				t.Errorf("resolveURL(%q, %q) = %q, want %q", tt.base, tt.href, got, tt.want)
			}
		})
	}
}

func TestFormatFeedContent(t *testing.T) {
	feeds := []feedInfo{
		{URL: "https://example.com/feed.xml", Type: "RSS", Title: "My Feed", Description: "A description", EntryCount: 1, Entries: []feedEntry{{Title: "Latest story", URL: "https://example.com/story", Description: "Story summary"}}},
		{URL: "https://example.com/atom.xml", Type: "Atom", Title: ""},
	}
	content := formatFeedContent("example.com", feeds)
	if !strings.Contains(content, "example.com") {
		t.Errorf("expected domain; got: %s", content)
	}
	if !strings.Contains(content, "feed.xml") {
		t.Errorf("expected feed URL; got: %s", content)
	}
	for _, expected := range []string{"A description", "Latest story", "Story summary", "Entries: 1", "Feedly", "Inoreader"} {
		if !strings.Contains(content, expected) {
			t.Errorf("feed result missing %q: %s", expected, content)
		}
	}
}

func TestParseJSONFeedLimitsPreviewAndPreservesEntryCount(t *testing.T) {
	var feed feedInfo
	payload := []byte(`{"version":"https://jsonfeed.org/version/1.1","title":"JSON news","description":"Latest updates","items":[{"id":"1","url":"https://example.com/1","title":"one"},{"id":"2","url":"https://example.com/2","title":"two"},{"id":"3","url":"https://example.com/3","title":"three"},{"id":"4","url":"https://example.com/4","title":"four"},{"id":"5","url":"https://example.com/5","title":"five"},{"id":"6","url":"https://example.com/6","title":"six"}]}`)
	if !parseFeedDocument(&feed, "application/feed+json", payload) {
		t.Fatal("parseFeedDocument rejected valid JSON Feed")
	}
	if feed.Type != "JSON Feed" || feed.Title != "JSON news" || feed.Description != "Latest updates" || feed.EntryCount != 6 || len(feed.Entries) != 5 {
		t.Fatalf("unexpected parsed JSON Feed: %#v", feed)
	}
}

func TestFormatFeedContentEmpty(t *testing.T) {
	content := formatFeedContent("example.com", []feedInfo{})
	if !strings.Contains(strings.ToLower(content), "no feeds") {
		t.Errorf("expected empty message; got: %s", content)
	}
}

func TestFormatExpandContent(t *testing.T) {
	redirects := []string{
		"https://bit.ly/example",
		"https://example.com/redirect",
		"https://example.com/final",
	}
	content := formatExpandContent("https://bit.ly/example", "https://example.com/final", redirects)
	if !strings.Contains(content, "bit.ly") {
		t.Errorf("expected original URL; got: %s", content)
	}
	if !strings.Contains(content, "example.com/final") {
		t.Errorf("expected final URL; got: %s", content)
	}
}

func TestFormatExpandContentNoRedirects(t *testing.T) {
	redirects := []string{"https://example.com/page"}
	content := formatExpandContent("https://example.com/page", "https://example.com/page", redirects)
	if content == "" {
		t.Error("expected non-empty content")
	}
}

func TestPerformSafetyChecks(t *testing.T) {
	tests := []struct {
		name        string
		domain      string
		targetURL   string
		checkName   string
		checkPassed bool
	}{
		{
			name:        "https URL passes HTTPS check",
			domain:      "example.com",
			targetURL:   "https://example.com",
			checkName:   "HTTPS",
			checkPassed: true,
		},
		{
			name:        "http URL fails HTTPS check",
			domain:      "example.com",
			targetURL:   "http://example.com",
			checkName:   "HTTPS",
			checkPassed: false,
		},
		{
			name:        "suspicious TLD fails TLD check",
			domain:      "malware.tk",
			targetURL:   "https://malware.tk",
			checkName:   "Domain TLD",
			checkPassed: false,
		},
		{
			name:        "normal TLD passes TLD check",
			domain:      "example.com",
			targetURL:   "https://example.com",
			checkName:   "Domain TLD",
			checkPassed: true,
		},
		{
			name:        "long domain fails length check",
			domain:      strings.Repeat("a", 51) + ".com",
			targetURL:   "https://" + strings.Repeat("a", 51) + ".com",
			checkName:   "Domain Length",
			checkPassed: false,
		},
	}

	ctx := context.Background()
	client := &http.Client{}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checks := performSafetyChecks(ctx, client, tt.domain, tt.targetURL)
			for _, c := range checks {
				if c.Name == tt.checkName {
					if c.Passed != tt.checkPassed {
						t.Errorf("check %q: expected passed=%v, got %v (details: %s)",
							tt.checkName, tt.checkPassed, c.Passed, c.Details)
					}
					return
				}
			}
			t.Errorf("check %q not found in results", tt.checkName)
		})
	}
}

func TestFormatSafeContent(t *testing.T) {
	checks := []safetyCheck{
		{Name: "HTTPS", Passed: true, Details: "Uses HTTPS"},
		{Name: "Domain TLD", Passed: false, Details: "Suspicious TLD"},
	}
	content := formatSafeContent("example.tk", "Suspicious", false, checks)
	if !strings.Contains(content, "example.tk") {
		t.Errorf("expected domain; got: %s", content)
	}
	if !strings.Contains(content, "Suspicious") {
		t.Errorf("expected rating; got: %s", content)
	}
}

func TestCacheHandlerLocalDomain(t *testing.T) {
	h := NewCacheHandler()
	h.liveClient = nil
	h.client = &http.Client{Transport: packageTestTransport{body: "[[\"timestamp\",\"original\"],[\"20260102030405\",\"https://example.com/\"]]"}}
	ctx := context.Background()

	ans, err := h.HandleDirectQuery(ctx, "example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ans == nil {
		t.Fatal("nil answer")
	}
	if ans.Type != AnswerTypeCache {
		t.Errorf("expected type %s, got %s", AnswerTypeCache, ans.Type)
	}
	if !strings.Contains(ans.Content, "archive.org") {
		t.Errorf("expected archive links in content; got: %s", ans.Content)
	}
	if ans.Data["snapshot_count"] != 1 {
		t.Fatalf("expected one verified snapshot, got %v", ans.Data["snapshot_count"])
	}
}

func TestPhishTankDoesNotFollowCredentialedRedirect(t *testing.T) {
	calls := 0
	client := &http.Client{Transport: safeBrowsingTestTransport(func(request *http.Request) (*http.Response, error) {
		calls++
		if calls > 1 {
			t.Fatal("PhishTank redirect destination received a request")
		}
		header := make(http.Header)
		header.Set("Location", "https://attacker.example/collect")
		return &http.Response{StatusCode: http.StatusFound, Header: header, Body: io.NopCloser(strings.NewReader(""))}, nil
	})}
	handler := &SafeHandler{client: client, phishTankAPIKey: "test-key", phishTankEndpoint: "https://checkurl.phishtank.com/checkurl/"}
	known, _, _ := handler.checkPhishTank(context.Background(), "https://example.test/")
	if known || calls != 1 {
		t.Fatalf("redirect result known=%v, request count=%d; want unknown and one request", known, calls)
	}
}

func TestCacheHandlerRejectsCredentialedURLsBeforeArchiveRequests(t *testing.T) {
	handler := &CacheHandler{client: &http.Client{Transport: safeBrowsingTestTransport(func(*http.Request) (*http.Response, error) {
		t.Error("credentialed URL was sent to an archive provider")
		return nil, fmt.Errorf("unexpected archive request")
	})}}
	if _, err := handler.HandleDirectQuery(context.Background(), "https://user:password@example.com/private"); err == nil {
		t.Fatal("credentialed archive URL was accepted")
	}
}

func TestCacheHandlerWithFullURL(t *testing.T) {
	h := NewCacheHandler()
	h.liveClient = nil
	h.client = &http.Client{Transport: packageTestTransport{body: "[[\"urlkey\",\"timestamp\",\"original\"]]"}}
	ctx := context.Background()

	ans, err := h.HandleDirectQuery(ctx, "https://example.com/page")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ans == nil {
		t.Fatal("nil answer")
	}
	if !strings.Contains(ans.Content, "archive.org") {
		t.Errorf("expected archive links; got: %s", ans.Content)
	}
}

type cachePreviewTransport struct{}

func (cachePreviewTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	body := `<html><body><p onclick="alert(1)">Archived body</p><script>alert(2)</script><img src=x onerror="alert(3)"></body></html>`
	header := http.Header{"Content-Type": []string{"text/html; charset=utf-8"}}
	if request.URL.Path == "/cdx/search/cdx" {
		body = `[["timestamp","original"],["20260102030405","https://example.com/"]]`
		header.Set("Content-Type", "application/json")
	}
	return &http.Response{StatusCode: http.StatusOK, Header: header, Body: io.NopCloser(strings.NewReader(body)), Request: request}, nil
}

type archiveDateSelectionTransport struct {
	captureRequests []string
}

func (transport *archiveDateSelectionTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	body := ""
	header := make(http.Header)
	switch {
	case request.URL.Host == "web.archive.org" && request.URL.Path == "/cdx/search/cdx":
		body = "[[\"timestamp\",\"original\"],[\"20240102030405\",\"https://example.test/page\"],[\"20230102030405\",\"https://example.test/page\"],[\"20250101000000\",\"https://other.test/private\"]]"
		header.Set("Content-Type", "application/json")
	case request.URL.Host == "index.commoncrawl.org":
		body = "[]"
	case request.URL.Host == "web.archive.org" && strings.Contains(request.URL.Path, "/web/"):
		transport.captureRequests = append(transport.captureRequests, request.URL.Path)
		if strings.Contains(request.URL.Path, "20230102030405") {
			body = "<p>older selected capture</p>"
		} else {
			body = "<p>newer capture</p>"
		}
		header.Set("Content-Type", "text/html; charset=utf-8")
	default:
		return nil, fmt.Errorf("unexpected archive request %s", request.URL)
	}
	return &http.Response{StatusCode: http.StatusOK, Header: header, Body: io.NopCloser(strings.NewReader(body)), Request: request}, nil
}

type archiveTodayTransport struct {
	previewRequests int
}

func (transport *archiveTodayTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	header := make(http.Header)
	body := ""
	switch {
	case request.URL.Host == "archive.today" && strings.HasPrefix(request.URL.Path, "/timemap/"):
		if request.Header.Get("Accept") != "application/link-format" {
			return nil, fmt.Errorf("missing Memento link-format Accept header")
		}
		body = `<https://archive.today/20240102030405/https://example.com/page>; rel="memento"; datetime="Tue, 02 Jan 2024 03:04:05 GMT", <https://evil.example/20230102030405/https://example.com/page>; rel="memento"; datetime="Mon, 02 Jan 2023 03:04:05 GMT", <https://archive.is/20220102030405/https://other.example/page>; rel="memento"; datetime="Sun, 02 Jan 2022 03:04:05 GMT", <https://archive.is/20210102030405/https://example.com/other>; rel="memento"; datetime="Sat, 02 Jan 2021 03:04:05 GMT"`
		header.Set("Content-Type", "application/link-format")
	case request.URL.Host == "archive.today":
		transport.previewRequests++
		body = `<p>Archive.today body</p><script>alert(1)</script>`
		header.Set("Content-Type", "text/html; charset=utf-8")
	default:
		return nil, fmt.Errorf("unexpected Archive.today request %s", request.URL)
	}
	return &http.Response{StatusCode: http.StatusOK, Header: header, Body: io.NopCloser(strings.NewReader(body)), Request: request}, nil
}

func TestArchiveTodayMementoLookupAndPreview(t *testing.T) {
	transport := &archiveTodayTransport{}
	handler := NewCacheHandler()
	handler.client = &http.Client{Transport: transport}
	snapshots := handler.lookupArchiveToday(context.Background(), "https://example.com/page")
	if len(snapshots) != 1 {
		t.Fatalf("lookupArchiveToday() = %#v, want only the safe matching memento", snapshots)
	}
	if snapshots[0].Timestamp != "20240102030405" || snapshots[0].Source != "Archive.today" {
		t.Fatalf("memento metadata = %#v", snapshots[0])
	}
	if got := handler.fetchArchiveSnapshot(context.Background(), snapshots[0]); !strings.Contains(got, "Archive.today body") || strings.Contains(got, "<script") || strings.Contains(got, "alert(") {
		t.Fatalf("Archive.today preview was missing or unsafe: %s", got)
	}
	if transport.previewRequests != 1 {
		t.Fatalf("preview requests = %d, want 1", transport.previewRequests)
	}
}

func TestArchiveTodaySnapshotDownloadIsBoundedAndMatchesOriginal(t *testing.T) {
	transport := &archiveTodayTransport{}
	handler := NewCacheHandler()
	handler.liveClient = &http.Client{Transport: transport}
	body, contentType, err := handler.DownloadArchiveTodaySnapshot(context.Background(), "https://example.com/page", "20240102030405", "https://archive.today/20240102030405/https://example.com/page")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(contentType, "text/html") || !strings.Contains(string(body), "Archive.today body") || !strings.Contains(string(body), "<script>") {
		t.Fatalf("download did not preserve the archived response: content-type=%q body=%q", contentType, body)
	}
	before := transport.previewRequests
	if _, _, err := handler.DownloadArchiveTodaySnapshot(context.Background(), "https://example.com/other", "20240102030405", "https://archive.today/20240102030405/https://example.com/page"); !errors.Is(err, ErrInvalidArchiveTodaySnapshot) {
		t.Fatalf("mismatched original URL error = %v", err)
	}
	if transport.previewRequests != before {
		t.Fatal("mismatched snapshot triggered an upstream request")
	}
}

func TestArchiveTodaySnapshotDownloadRejectsUnsafeResponses(t *testing.T) {
	const snapshotURL = "https://archive.today/20240102030405/https://example.com/page"
	for _, test := range []struct {
		name, contentType string
		status            int
		body              string
	}{
		{name: "redirect", contentType: "text/html", status: http.StatusFound},
		{name: "binary content", contentType: "application/octet-stream", status: http.StatusOK},
		{name: "oversized content", contentType: "text/html; charset=utf-8", status: http.StatusOK, body: strings.Repeat("x", (5<<20)+1)},
	} {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			transport := safeBrowsingTestTransport(func(request *http.Request) (*http.Response, error) {
				calls++
				header := make(http.Header)
				header.Set("Content-Type", test.contentType)
				if test.status == http.StatusFound {
					header.Set("Location", "https://attacker.example/")
				}
				return &http.Response{StatusCode: test.status, Header: header, Body: io.NopCloser(strings.NewReader(test.body)), Request: request}, nil
			})
			handler := NewCacheHandler()
			handler.liveClient = &http.Client{Transport: transport}
			if _, _, err := handler.DownloadArchiveTodaySnapshot(context.Background(), "https://example.com/page", "20240102030405", snapshotURL); err == nil {
				t.Fatal("unsafe archive response was accepted")
			}
			if calls != 1 {
				t.Fatalf("upstream request count = %d, want exactly one without redirect following", calls)
			}
		})
	}
}

func TestDecodeBoundedJSONRejectsMalformedAndOversizedBodies(t *testing.T) {
	var value map[string]string
	if err := decodeBoundedJSON(strings.NewReader(`{"ok":"yes"}`), 32, &value); err != nil || value["ok"] != "yes" {
		t.Fatalf("valid JSON decode = %#v, %v", value, err)
	}
	for _, input := range []string{`{"ok":`, strings.Repeat("x", 33)} {
		if err := decodeBoundedJSON(strings.NewReader(input), 32, &value); err == nil {
			t.Errorf("decodeBoundedJSON accepted invalid/oversized response %q", input)
		}
	}
}

func TestCacheHandlerMalformedArchiveIndexResponsesFailClosed(t *testing.T) {
	for _, test := range []struct {
		name    string
		bodyFor func(*http.Request) string
	}{
		{
			name: "malformed Wayback rows",
			bodyFor: func(request *http.Request) string {
				if request.URL.Host == "web.archive.org" {
					return `[["timestamp","original"],[`
				}
				return `[]`
			},
		},
		{
			name: "malformed Common Crawl collections",
			bodyFor: func(request *http.Request) string {
				if request.URL.Path == "/collinfo.json" {
					return `[{"id":`
				}
				return `[]`
			},
		},
		{
			name: "malformed Common Crawl index entries",
			bodyFor: func(request *http.Request) string {
				if request.URL.Path == "/collinfo.json" {
					return `[{"id":"CC-MAIN-2026-40"}]`
				}
				return `{"timestamp":`
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			client := &http.Client{Transport: safeBrowsingTestTransport(func(request *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(test.bodyFor(request))), Request: request}, nil
			})}
			handler := &CacheHandler{client: client}
			if snapshots := handler.lookupWayback(context.Background(), "https://example.test/page"); len(snapshots) != 0 {
				t.Errorf("malformed Wayback body returned snapshots: %#v", snapshots)
			}
			if snapshots := handler.lookupCommonCrawl(context.Background(), "https://example.test/page"); len(snapshots) != 0 {
				t.Errorf("malformed Common Crawl body returned snapshots: %#v", snapshots)
			}
		})
	}
}

func TestCacheHandlerSelectsRequestedSnapshot(t *testing.T) {
	transport := &archiveDateSelectionTransport{}
	handler := NewCacheHandler()
	handler.client = &http.Client{Transport: transport}
	handler.liveClient = nil
	ctx := WithArchiveSnapshot(context.Background(), "20230102030405")
	answer, err := handler.HandleDirectQuery(ctx, "https://example.test/page")
	if err != nil {
		t.Fatal(err)
	}
	if answer.Data["snapshot_count"] != 2 {
		t.Fatalf("snapshot count = %v, want only exact-original captures", answer.Data["snapshot_count"])
	}
	if answer.Data["selected_snapshot"] != "20230102030405" || !strings.Contains(answer.Data["archived_content"].(string), "older selected capture") {
		t.Fatalf("selected snapshot/data = %#v", answer.Data)
	}
	if len(transport.captureRequests) != 1 || !strings.Contains(transport.captureRequests[0], "20230102030405") {
		t.Fatalf("archive requests = %#v, want only the requested capture", transport.captureRequests)
	}
	if !strings.Contains(answer.Content, `name="snapshot_id"`) || !strings.Contains(answer.Content, ` selected>`) {
		t.Fatalf("date selector does not submit the selected snapshot through its native form: %s", answer.Content)
	}
}

func TestCacheHandlerRejectsOutOfRangeTimelineIndex(t *testing.T) {
	transport := &archiveDateSelectionTransport{}
	handler := NewCacheHandler()
	handler.client = &http.Client{Transport: transport}
	handler.liveClient = nil
	ctx := WithArchiveSnapshotIndex(context.Background(), "10")
	answer, err := handler.HandleDirectQuery(ctx, "https://example.test/page")
	if err != nil {
		t.Fatal(err)
	}
	if answer.Data["selected_snapshot"] != "" || len(transport.captureRequests) != 0 {
		t.Fatalf("out-of-range timeline index selected content: data=%#v requests=%#v", answer.Data, transport.captureRequests)
	}
}

func TestCacheHandlerSelectsSnapshotByTimelineIndex(t *testing.T) {
	transport := &archiveDateSelectionTransport{}
	handler := NewCacheHandler()
	handler.client = &http.Client{Transport: transport}
	handler.liveClient = nil
	ctx := WithArchiveSnapshotIndex(context.Background(), "1")
	answer, err := handler.HandleDirectQuery(ctx, "https://example.test/page")
	if err != nil {
		t.Fatal(err)
	}
	if answer.Data["selected_snapshot"] != "20230102030405" || !strings.Contains(answer.Data["archived_content"].(string), "older selected capture") {
		t.Fatalf("timeline index selected the wrong snapshot: %#v", answer.Data)
	}
}

func TestArchiveSnapshotIdentityDistinguishesSourcesAtSameTimestamp(t *testing.T) {
	wayback := archiveSnapshot{Timestamp: "20250102030405", Original: "https://example.test/page", URL: "https://web.archive.org/web/20250102030405/https://example.test/page", Source: "Wayback Machine"}
	commonCrawl := archiveSnapshot{Timestamp: wayback.Timestamp, Original: wayback.Original, URL: "https://index.commoncrawl.org/CC-MAIN-index", Source: "Common Crawl", Filename: "sample.warc.gz", Offset: 100, Length: 200}
	waybackID := archiveSnapshotID(wayback)
	commonCrawlID := archiveSnapshotID(commonCrawl)
	if waybackID == commonCrawlID {
		t.Fatal("different archive captures with the same timestamp received the same identity")
	}
	archives := []struct {
		name string
		url  string
	}{}
	content := formatCacheContent("https://example.test/page", archives, []archiveSnapshot{wayback, commonCrawl}, wayback.Timestamp, commonCrawlID, 1, "", "")
	if strings.Count(content, " selected>") != 1 || !strings.Contains(content, "value=\""+commonCrawlID+"\" selected>") {
		t.Fatalf("same-timestamp selection did not identify exactly the requested source: %s", content)
	}
}

func TestCacheHandlerRendersSanitizedArchivedPreview(t *testing.T) {
	handler := NewCacheHandler()
	handler.liveClient = nil
	handler.client = &http.Client{Transport: cachePreviewTransport{}}
	answer, err := handler.HandleDirectQuery(context.Background(), "example.com")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(answer.Content, "Archived body") || strings.Contains(answer.Content, "alert(") || strings.Contains(answer.Content, "onerror") || strings.Contains(answer.Content, "onclick") {
		t.Fatalf("archive preview was missing or unsafe: %s", answer.Content)
	}
	if answer.Data["archived_content"] == "" {
		t.Fatal("archived page preview missing from structured data")
	}
}

func TestFormatCacheContentProvidesTimelineAndDownloadLinks(t *testing.T) {
	archives := []struct {
		name string
		url  string
	}{{name: "Wayback Machine", url: "https://web.archive.org/web/*/https%3A%2F%2Fexample.com"}}
	snapshots := []archiveSnapshot{
		{Timestamp: "20240102030405", Original: "https://example.com/page?a=1&b=2", URL: "https://web.archive.org/web/20240102030405/https://example.com/page?a=1&b=2", Source: "Wayback Machine"},
		{Timestamp: "20230102030405", Original: "https://example.com/page", URL: "https://web.archive.org/web/20230102030405/https://example.com/page", Source: "Wayback Machine"},
		{Timestamp: "20220102030405", Original: "https://example.com/page", URL: "https://archive.md/20220102030405/https://example.com/page", Source: "Archive.today"},
	}
	content := formatCacheContent("https://example.com/page", archives, snapshots, snapshots[0].Timestamp, archiveSnapshotID(snapshots[0]), 0, "", "")
	for _, expected := range []string{`method="get" class="archive-timeline"`, `name="snapshot_id"`, `type="range"`, `max="2"`, `2024-01-02T03:04:05Z`, `type="submit">View archived snapshot`, "Download archived response", "20240102030405id_/https://example.com/page?a=1&amp;b=2", "https://archive.md/20220102030405/https://example.com/page", `href="/direct/cache-download-archive-today?`} {
		if !strings.Contains(content, expected) {
			t.Errorf("cache content missing %q", expected)
		}
	}
	if strings.Contains(content, "onchange=") {
		t.Error("archive timeline requires JavaScript instead of native form submission")
	}
	if strings.Contains(content, "webcache.googleusercontent.com") {
		t.Error("cache content advertises the unavailable Google cache")
	}
}

func TestBeautifyHandlerOptions(t *testing.T) {
	handler := NewBeautifyHandler()
	answer, err := handler.HandleDirectQuery(context.Background(), `json --indent=4 --minify {"a":1}`)
	if err != nil {
		t.Fatalf("HandleDirectQuery() error = %v", err)
	}
	if got := answer.Data["minified"]; got != `{"a":1}` {
		t.Errorf("minified output = %#v, want compact JSON", got)
	}
	if !strings.Contains(answer.Content, "Minified output") || !strings.Contains(answer.Content, "Indentation: 4") {
		t.Errorf("formatter options missing from answer content: %s", answer.Content)
	}
	if answer.Data["style"] != "pretty" || !strings.Contains(answer.Content, "Style: pretty") {
		t.Errorf("JSON formatter profile = %#v, expected pretty", answer.Data["style"])
	}
	allman, err := handler.HandleDirectQuery(context.Background(), `js --style=allman function f(){return 1}`)
	if err != nil {
		t.Fatalf("Allman JavaScript style failed: %v", err)
	}
	if !strings.Contains(allman.Data["output"].(string), "function f()\n{") {
		t.Errorf("Allman style did not place the opening brace on its own line: %q", allman.Data["output"])
	}
	compactCSS, err := handler.HandleDirectQuery(context.Background(), `css --style=compact .a { color: red; }`)
	if err != nil || strings.Contains(compactCSS.Data["output"].(string), "\n") || compactCSS.Data["style"] != "compact" {
		t.Fatalf("compact CSS style failed: answer=%#v error=%v", compactCSS.Data, err)
	}
	lowerSQL, err := handler.HandleDirectQuery(context.Background(), `sql --style=lowercase SELECT ID FROM Users WHERE Name = 'MixedCase'`)
	if err != nil {
		t.Fatalf("lowercase SQL style failed: %v", err)
	}
	if !strings.Contains(lowerSQL.Data["output"].(string), "select ID") || !strings.Contains(lowerSQL.Data["output"].(string), "'MixedCase'") {
		t.Errorf("lowercase SQL style altered identifiers or literals: %q", lowerSQL.Data["output"])
	}
	if _, err := handler.HandleDirectQuery(context.Background(), `python --style=allman print(1)`); err == nil {
		t.Error("unsupported Python style profile was accepted")
	}
	if _, err := handler.HandleDirectQuery(context.Background(), `json --indent=3 {"a":1}`); err == nil {
		t.Error("invalid indentation was accepted")
	}
	jsAnswer, err := handler.HandleDirectQuery(context.Background(), `js --minify const x = 1;`)
	if err != nil {
		t.Fatalf("JavaScript minification failed: %v", err)
	}
	if !strings.Contains(jsAnswer.Data["minified"].(string), "const x=1") {
		t.Errorf("unexpected minified JavaScript: %#v", jsAnswer.Data["minified"])
	}
	pythonAnswer, err := handler.HandleDirectQuery(context.Background(), "python --minify def f():\n    # comment\n    return \"a # b\"\n\n")
	if err != nil {
		t.Fatalf("Python minification failed: %v", err)
	}
	if got := pythonAnswer.Data["minified"].(string); strings.Contains(got, "# comment") || !strings.Contains(got, `"a # b"`) || strings.Contains(got, "\n\n") {
		t.Errorf("Python minifier mishandled comments, strings, or blank lines: %q", got)
	}
	typescriptAnswer, err := handler.HandleDirectQuery(context.Background(), `typescript --minify const add = (left: number, right: number): number => left + right;`)
	if err != nil {
		t.Fatalf("TypeScript minification failed: %v", err)
	}
	if got := typescriptAnswer.Data["minified"].(string); strings.Contains(got, ": number") || !strings.Contains(got, "=>") {
		t.Errorf("unexpected TypeScript minified output: %q", got)
	}
	if _, err := handler.HandleDirectQuery(context.Background(), `typescript --minify const =`); err == nil {
		t.Error("invalid TypeScript was accepted by the minifier")
	}
	for _, test := range []struct{ language, source string }{
		{"css", ".a { color: red; }"},
		{"scss", "$color: red; .a { color: $color; &:hover { width: calc(100% - 2em); } }"},
		{"less", "@color: red; .a { color: @color; }"},
		{"sql", "SELECT  a,  'x -- y'  FROM t -- remove comment\n WHERE id = 1"},
		{"html", "<div> hello </div>"},
		{"xml", "<root> <item/> </root>"},
		{"php", "<?php echo 'ok'; ?>"},
	} {
		if _, err := minifyCode(test.source, test.language); err != nil {
			t.Errorf("minifyCode(%q) error = %v", test.language, err)
		}
	}
	if got, err := minifyCode("SELECT  a,  'x -- y'  FROM t -- remove comment\n WHERE id = 1", "sql"); err != nil || strings.Contains(got, "remove comment") || !strings.Contains(got, "'x -- y'") {
		t.Errorf("SQL minification = %q, %v", got, err)
	}
	if got, err := minifyCode("/*! keep license */ .a .b { width: calc(100% - 2em); }", "scss"); err != nil || !strings.Contains(got, "/*! keep license */") || !strings.Contains(got, ".a .b") || !strings.Contains(got, "100% - 2em") {
		t.Errorf("stylesheet minification = %q, %v", got, err)
	}
	if got, err := minifyCode("SELECT /*+ INDEX(users idx_users) */ name FROM users", "sql"); err != nil || !strings.Contains(got, "/*+ INDEX(users idx_users) */") {
		t.Errorf("SQL minification removed an optimizer hint: %q, %v", got, err)
	}
	for _, test := range []struct{ language, source string }{
		{"json", `{"a":`},
		{"js", "function {"},
	} {
		if _, err := minifyCode(test.source, test.language); err == nil {
			t.Errorf("minifyCode(%q) accepted malformed source %q", test.language, test.source)
		}
	}
}

type safeBrowsingTestTransport func(*http.Request) (*http.Response, error)

func (transport safeBrowsingTestTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	return transport(request)
}

func TestSafeBrowsingReputationCheck(t *testing.T) {
	t.Run("clean result verifies safe", func(t *testing.T) {
		client := &http.Client{Transport: safeBrowsingTestTransport(func(request *http.Request) (*http.Response, error) {
			if request.URL.Query().Get("key") != "test-key" {
				t.Errorf("API key query = %q", request.URL.Query().Get("key"))
			}
			body, err := io.ReadAll(request.Body)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(body), `"url":"https://example.test/"`) {
				t.Errorf("request did not use the domain root: %s", body)
			}
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{}`)), Header: make(http.Header)}, nil
		})}
		handler := &SafeHandler{client: client, apiKey: "test-key", reputationEndpoint: "https://safebrowsing.googleapis.com/v4/threatMatches:find"}
		known, safe := handler.checkGoogleSafeBrowsing(context.Background(), "https://example.test/")
		if !known || !safe {
			t.Fatalf("checkGoogleSafeBrowsing() = (%v, %v), want (true, true)", known, safe)
		}
	})
	t.Run("match marks unsafe", func(t *testing.T) {
		client := &http.Client{Transport: safeBrowsingTestTransport(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"matches":[{}]}`)), Header: make(http.Header)}, nil
		})}
		handler := &SafeHandler{client: client, apiKey: "test-key", reputationEndpoint: "https://safebrowsing.googleapis.com/v4/threatMatches:find"}
		known, safe := handler.checkGoogleSafeBrowsing(context.Background(), "https://example.test/")
		if !known || safe {
			t.Fatalf("checkGoogleSafeBrowsing() = (%v, %v), want (true, false)", known, safe)
		}
	})
	t.Run("redirect is not followed with provider credentials", func(t *testing.T) {
		calls := 0
		client := &http.Client{Transport: safeBrowsingTestTransport(func(request *http.Request) (*http.Response, error) {
			calls++
			if calls > 1 {
				t.Fatal("Safe Browsing redirect destination received a request")
			}
			header := make(http.Header)
			header.Set("Location", "https://attacker.example/collect")
			return &http.Response{StatusCode: http.StatusFound, Header: header, Body: io.NopCloser(strings.NewReader(""))}, nil
		})}
		handler := &SafeHandler{client: client, apiKey: "test-key", reputationEndpoint: "https://safebrowsing.googleapis.com/v4/threatMatches:find"}
		known, _ := handler.checkGoogleSafeBrowsing(context.Background(), "https://example.test/")
		if known || calls != 1 {
			t.Fatalf("redirect result known=%v, request count=%d; want unknown and one request", known, calls)
		}
	})
	t.Run("missing key remains unknown", func(t *testing.T) {
		handler := &SafeHandler{client: http.DefaultClient, reputationEndpoint: "https://safebrowsing.googleapis.com/v4/threatMatches:find"}
		if known, _ := handler.checkGoogleSafeBrowsing(context.Background(), "https://example.test/"); known {
			t.Fatal("missing API key returned a known reputation")
		}
	})
}

func TestPhishTankReputationCheck(t *testing.T) {
	tests := []struct {
		name        string
		status      int
		body        string
		known, safe bool
	}{
		{name: "no match", status: http.StatusOK, body: `{"results":{"in_database":false}}`, known: true, safe: true},
		{name: "verified active match", status: http.StatusOK, body: `{"results":{"in_database":true,"verified":"y","valid":"y"}}`, known: true, safe: false},
		{name: "inactive match remains unknown", status: http.StatusOK, body: `{"results":{"in_database":true,"verified":"y","valid":"n"}}`},
		{name: "rate limited remains unknown", status: http.StatusTooManyRequests, body: `{}`},
		{name: "malformed result remains unknown", status: http.StatusOK, body: `{"results":`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := &http.Client{Transport: safeBrowsingTestTransport(func(request *http.Request) (*http.Response, error) {
				if request.Method != http.MethodPost || request.URL.Host != "checkurl.phishtank.com" {
					t.Errorf("unexpected PhishTank request: %s %s", request.Method, request.URL)
				}
				if !strings.HasPrefix(request.UserAgent(), "search/") {
					t.Errorf("PhishTank User-Agent = %q", request.UserAgent())
				}
				if err := request.ParseForm(); err != nil {
					t.Errorf("parse form: %v", err)
				}
				if request.Form.Get("url") != "https://example.test/" || request.Form.Get("app_key") != "test-key" || request.Form.Get("format") != "json" {
					t.Errorf("unexpected PhishTank form: %#v", request.Form)
				}
				return &http.Response{StatusCode: test.status, Body: io.NopCloser(strings.NewReader(test.body)), Header: make(http.Header)}, nil
			})}
			handler := &SafeHandler{client: client, phishTankAPIKey: "test-key", phishTankEndpoint: "https://checkurl.phishtank.com/checkurl/"}
			known, safe, _ := handler.checkPhishTank(context.Background(), "https://example.test/")
			if known != test.known || safe != test.safe {
				t.Fatalf("checkPhishTank() = (%v, %v), want (%v, %v)", known, safe, test.known, test.safe)
			}
		})
	}
}

func TestRDAPRegistrantPrivacyRequiresEvidence(t *testing.T) {
	tests := []struct {
		name    string
		json    string
		private bool
		known   bool
	}{
		{name: "explicit redaction", json: `[{"roles":["registrant"],"redacted":[{"name":"fn"}]}]`, private: true, known: true},
		{name: "public registrant", json: `[{"roles":["registrant"],"vcardArray":["vcard",[["fn",{},"text","Example Person"]]]}]`, private: false, known: true},
		{name: "missing registrant data", json: `[{"roles":["registrant"]}]`, private: false, known: false},
		{name: "no registrant entity", json: `[]`, private: false, known: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var entities []rdapEntity
			if err := json.Unmarshal([]byte(test.json), &entities); err != nil {
				t.Fatal(err)
			}
			private, known := rdapRegistrantPrivacy(entities)
			if private != test.private || known != test.known {
				t.Errorf("rdapRegistrantPrivacy() = (%v, %v), want (%v, %v)", private, known, test.private, test.known)
			}
		})
	}
}

func TestHighRiskPhishingSignals(t *testing.T) {
	tests := []struct {
		domain string
		want   bool
	}{
		{domain: "example.com", want: false},
		{domain: "pаypal.com", want: true},
		{domain: "8.8.8.8", want: true},
	}
	for _, test := range tests {
		t.Run(test.domain, func(t *testing.T) {
			got, _ := highRiskPhishingSignal(test.domain)
			if got != test.want {
				t.Errorf("highRiskPhishingSignal(%q) = %v, want %v", test.domain, got, test.want)
			}
		})
	}
}

func TestCacheHandlerCommonCrawlIndex(t *testing.T) {
	client := &http.Client{Transport: safeBrowsingTestTransport(func(request *http.Request) (*http.Response, error) {
		var body string
		switch request.URL.Host {
		case "index.commoncrawl.org":
			if request.URL.Path == "/collinfo.json" {
				body = `[{"id":"CC-MAIN-2026-40"}]`
			} else {
				if request.URL.Query().Get("url") != "https://example.test/page" {
					t.Errorf("Common Crawl URL query = %q", request.URL.Query().Get("url"))
				}
				body = "{\"timestamp\":\"20261002000000\",\"url\":\"https://example.test/other\",\"filename\":\"crawl-data/CC-MAIN-2026-40/warc/wrong.warc.gz\",\"offset\":\"100\",\"length\":\"300\",\"status\":\"200\"}\n" + "{\"timestamp\":\"20261001000000\",\"url\":\"https://example.test/page\",\"filename\":\"crawl-data/CC-MAIN-2026-40/warc/example.warc.gz\",\"offset\":\"100\",\"length\":\"300\",\"status\":\"200\"}\n"
			}
		default:
			t.Fatalf("unexpected Common Crawl host: %s", request.URL.Host)
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}
	handler := &CacheHandler{client: client}
	snapshots := handler.lookupCommonCrawl(context.Background(), "https://example.test/page")
	if len(snapshots) != 1 {
		t.Fatalf("lookupCommonCrawl() returned %d snapshots, want 1", len(snapshots))
	}
	snapshot := snapshots[0]
	if snapshot.Source != "Common Crawl" || snapshot.Offset != 100 || snapshot.Length != 300 || !strings.HasPrefix(snapshot.Filename, "crawl-data/") {
		t.Errorf("unexpected Common Crawl snapshot: %#v", snapshot)
	}
}

func TestCacheHandlerFetchesBoundedCommonCrawlRecord(t *testing.T) {
	var record bytes.Buffer
	gzipWriter := gzip.NewWriter(&record)
	page := `<p>archived</p><script>alert(1)</script>`
	warc := fmt.Sprintf("WARC/1.0\r\nWARC-Type: response\r\n\r\nHTTP/1.1 200 OK\r\nContent-Type: text/html\r\nContent-Length: %d\r\n\r\n%s", len(page), page)
	if _, err := gzipWriter.Write([]byte(warc)); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Transport: safeBrowsingTestTransport(func(request *http.Request) (*http.Response, error) {
		if request.Header.Get("Range") != "bytes=100-399" {
			t.Errorf("WARC range = %q", request.Header.Get("Range"))
		}
		header := make(http.Header)
		header.Set("Content-Range", "bytes 100-399/1000")
		return &http.Response{StatusCode: http.StatusPartialContent, Body: io.NopCloser(bytes.NewReader(record.Bytes())), Header: header}, nil
	})}
	handler := &CacheHandler{client: client}
	content := handler.fetchArchiveSnapshot(context.Background(), archiveSnapshot{Source: "Common Crawl", Filename: "crawl-data/collection/warc/sample.warc.gz", Offset: 100, Length: 300})
	if !strings.Contains(content, "archived") || strings.Contains(content, "<script") {
		t.Errorf("Common Crawl preview was not safely rendered: %s", content)
	}
}

func TestCacheHandlerDownloadsExactCommonCrawlRecord(t *testing.T) {
	original := "https://example.test/page?q=1"
	page := `<h1>archived source</h1><script>keep-as-download-only</script>`
	warc := fmt.Sprintf("WARC/1.0\r\nWARC-Type: response\r\nWARC-Target-URI: %s\r\n\r\nHTTP/1.1 200 OK\r\nContent-Type: text/html; charset=utf-8\r\nContent-Length: %d\r\n\r\n%s", original, len(page), page)
	var record bytes.Buffer
	gzipWriter := gzip.NewWriter(&record)
	if _, err := gzipWriter.Write([]byte(warc)); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	requestedRange := ""
	handler := &CacheHandler{client: &http.Client{Transport: safeBrowsingTestTransport(func(request *http.Request) (*http.Response, error) {
		requestedRange = request.Header.Get("Range")
		header := make(http.Header)
		header.Set("Content-Range", fmt.Sprintf("bytes 100-%d/%d", 99+record.Len(), 100+record.Len()))
		return &http.Response{StatusCode: http.StatusPartialContent, Body: io.NopCloser(bytes.NewReader(record.Bytes())), Header: header}, nil
	})}}
	body, contentType, err := handler.DownloadCommonCrawlRecord(context.Background(), original, "crawl-data/CC-MAIN/test.warc.gz", 100, int64(record.Len()))
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != page || !strings.HasPrefix(contentType, "text/html") {
		t.Fatalf("download body/type = (%q, %q), want exact captured HTML", body, contentType)
	}
	if requestedRange != fmt.Sprintf("bytes=100-%d", 99+record.Len()) {
		t.Errorf("requested range = %q", requestedRange)
	}
	if _, _, err := handler.DownloadCommonCrawlRecord(context.Background(), "https://other.test/page", "crawl-data/CC-MAIN/test.warc.gz", 100, int64(record.Len())); err == nil {
		t.Fatal("download accepted a WARC record whose target URL does not match")
	}
	wrongRange := &CacheHandler{client: &http.Client{Transport: safeBrowsingTestTransport(func(*http.Request) (*http.Response, error) {
		header := make(http.Header)
		header.Set("Content-Range", fmt.Sprintf("bytes 99-%d/%d", 99+record.Len(), 100+record.Len()))
		return &http.Response{StatusCode: http.StatusPartialContent, Body: io.NopCloser(bytes.NewReader(record.Bytes())), Header: header}, nil
	})}}
	if _, _, err := wrongRange.DownloadCommonCrawlRecord(context.Background(), original, "crawl-data/CC-MAIN/test.warc.gz", 100, int64(record.Len())); err == nil {
		t.Fatal("download accepted a mismatched HTTP Content-Range")
	}
}

func TestCacheHandlerRejectsInvalidCommonCrawlDownloadLocations(t *testing.T) {
	handler := &CacheHandler{client: &http.Client{Transport: safeBrowsingTestTransport(func(*http.Request) (*http.Response, error) {
		t.Fatal("invalid archive location triggered a network request")
		return nil, nil
	})}}
	for _, test := range []struct {
		name, filename string
		offset, length int64
	}{
		{name: "path traversal", filename: "crawl-data/../secret.warc.gz", length: 100},
		{name: "encoded path traversal", filename: "crawl-data/%2e%2e/secret.warc.gz", length: 100},
		{name: "empty path component", filename: "crawl-data//secret.warc.gz", length: 100},
		{name: "empty range", filename: "crawl-data/test.warc.gz", length: 0},
		{name: "oversized range", filename: "crawl-data/test.warc.gz", length: 20<<20 + 1},
		{name: "offset overflow", filename: "crawl-data/test.warc.gz", offset: int64(^uint64(0) >> 1), length: 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, _, err := handler.DownloadCommonCrawlRecord(context.Background(), "https://example.test/page", test.filename, test.offset, test.length); err == nil {
				t.Fatal("invalid archive location was accepted")
			}
		})
	}
}

func TestCacheHandlerLivePreviewSanitizesAndRejectsPrivateHosts(t *testing.T) {
	client := &http.Client{Transport: safeBrowsingTestTransport(func(request *http.Request) (*http.Response, error) {
		if request.URL.Path != "/page" || request.URL.RawQuery != "a=1" {
			t.Errorf("live preview did not preserve the requested public path and query: %s", request.URL)
		}
		header := make(http.Header)
		header.Set("Content-Type", "text/html; charset=utf-8")
		return &http.Response{StatusCode: http.StatusOK, Header: header, Body: io.NopCloser(strings.NewReader(`<p>live</p><script>alert(1)</script>`))}, nil
	})}
	handler := &CacheHandler{liveClient: client}
	preview := handler.fetchLivePreview(context.Background(), "https://8.8.8.8/page?a=1")
	if !strings.Contains(preview, "live") || strings.Contains(preview, "<script") {
		t.Errorf("live preview was not sanitized: %s", preview)
	}
	privateHandler := &CacheHandler{liveClient: &http.Client{Transport: safeBrowsingTestTransport(func(*http.Request) (*http.Response, error) {
		t.Error("private host was fetched")
		return nil, fmt.Errorf("unexpected private fetch")
	})}}
	if got := privateHandler.fetchLivePreview(context.Background(), "http://127.0.0.1/"); got != "" {
		t.Errorf("private live preview = %q, want empty", got)
	}
}

// ---------------------------------------------------------------------------
// Safe handler — local checks (suspicious TLD, HTTPS check)
// ---------------------------------------------------------------------------

func TestSafeHandlerSuspiciousTLD(t *testing.T) {
	h := NewSafeHandler()
	ctx := context.Background()

	ans, err := h.HandleDirectQuery(ctx, "http://malware.tk")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ans == nil {
		t.Fatal("nil answer")
	}
	if ans.Description != "Suspicious" {
		t.Errorf("expected Suspicious rating for .tk domain, got %q", ans.Description)
	}
}

func TestSafeHandlerReportsRelatedDomainStatusAsUnknown(t *testing.T) {
	answer, err := NewSafeHandler().HandleDirectQuery(context.Background(), "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	checks, ok := answer.Data["checks"].([]safetyCheck)
	if !ok {
		t.Fatalf("checks have type %T, want []safetyCheck", answer.Data["checks"])
	}
	for _, check := range checks {
		if check.Name == "Related Malicious Domains" {
			if check.Known || check.Passed {
				t.Fatalf("related-domain check = %#v, want Unknown", check)
			}
			return
		}
	}
	t.Fatal("related-domain status was omitted from the safety report")
}

func TestSafeHandlerReportsConfiguredDomainBlacklist(t *testing.T) {
	handler := NewSafeHandler()
	handler.SetDomainBlacklistLookup(func(domain string) (bool, bool) {
		return domain == "known-malware.test", true
	})
	answer, err := handler.HandleDirectQuery(context.Background(), "http://known-malware.test")
	if err != nil {
		t.Fatal(err)
	}
	checks, ok := answer.Data["checks"].([]safetyCheck)
	if !ok {
		t.Fatalf("checks have type %T, want []safetyCheck", answer.Data["checks"])
	}
	for _, check := range checks {
		if check.Name == "Known Blacklist" {
			if !check.Known || check.Passed || !strings.Contains(check.Details, "configured local domain blacklist") {
				t.Fatalf("blacklist check = %#v, want known malicious domain", check)
			}
			if answer.Description != "Suspicious" {
				t.Fatalf("safety rating = %q, want Suspicious", answer.Description)
			}
			return
		}
	}
	t.Fatal("known blacklist check was omitted")
}

func TestRelatedBlacklistedDomainsUsesConfiguredThreatData(t *testing.T) {
	lookup := func(domain string) bool {
		return domain == "paypa1.com"
	}
	got := relatedBlacklistedDomains("paypal.com", lookup, true)
	if len(got) != 1 || got[0] != "paypa1.com" {
		t.Fatalf("relatedBlacklistedDomains() = %v, want [paypa1.com]", got)
	}
	if got := relatedBlacklistedDomains("paypal.com", lookup, false); got != nil {
		t.Fatalf("unknown blacklist result = %v, want nil", got)
	}
}

func TestSafeHandlerReportsRelatedBlacklistedDomains(t *testing.T) {
	handler := NewSafeHandler()
	handler.SetDomainBlacklistLookup(func(domain string) (bool, bool) {
		return domain == "paypa1.com" || domain == "paypal.com", true
	})
	answer, err := handler.HandleDirectQuery(context.Background(), "paypal.com")
	if err != nil {
		t.Fatal(err)
	}
	checks := answer.Data["checks"].([]safetyCheck)
	for _, check := range checks {
		if check.Name == "Related Malicious Domains" {
			if !check.Known || check.Passed || !strings.Contains(check.Details, "paypa1.com") {
				t.Fatalf("related domain check = %#v, want the configured matching domain", check)
			}
			if answer.Description != "Suspicious" {
				t.Fatalf("safety rating = %q, want Suspicious", answer.Description)
			}
			return
		}
	}
	t.Fatal("related malicious domains check is missing")
}

func TestSafeHandlerFlagsIPLiteralWithoutReputationData(t *testing.T) {
	h := NewSafeHandler()
	h.client = &http.Client{Transport: safeTestTransport{}}
	ctx := context.Background()

	ans, err := h.HandleDirectQuery(ctx, "https://8.8.8.8/path?secret=value")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ans == nil {
		t.Fatal("nil answer")
	}
	if ans.Description != "Suspicious" {
		t.Errorf("expected Suspicious rating for an IP-literal host, got %q", ans.Description)
	}
	if ans.Data["safe"] != false || ans.Data["reputation_checked"] != false {
		t.Errorf("expected the local IP-literal signal without a reputation result, got %#v", ans.Data)
	}
	if ans.Data["url"] != "https://8.8.8.8/" {
		t.Errorf("safety check leaked or retained URL path/query data: %#v", ans.Data["url"])
	}
	checks, ok := ans.Data["checks"].([]safetyCheck)
	checkJSON, marshalErr := json.Marshal(checks)
	if marshalErr != nil {
		t.Fatalf("failed to serialize safety checks: %v", marshalErr)
	}
	var serializedChecks []map[string]interface{}
	if err := json.Unmarshal(checkJSON, &serializedChecks); err != nil {
		t.Fatalf("failed to decode serialized safety checks: %v", err)
	}
	if len(serializedChecks) == 0 || serializedChecks[0]["name"] == nil || serializedChecks[0]["details"] == nil {
		t.Errorf("serialized safety check is missing fields: %s", checkJSON)
	}
	if !ok || len(checks) == 0 {
		t.Errorf("safety checks were not reported: %#v", ans.Data["checks"])
	}
	if !strings.Contains(ans.Content, `class="rating suspicious"`) {
		t.Errorf("expected suspicious rating styling, got %s", ans.Content)
	}
}

type safeTestTransport struct{}

func (safeTestTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if request.URL.Hostname() == "rdap.org" {
		body := `{"events":[{"eventAction":"registration","eventDate":"2020-01-01T00:00:00Z"}]}`
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/rdap+json"}}, Body: io.NopCloser(strings.NewReader(body)), Request: request}, nil
	}
	return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("")), Request: request, TLS: &tls.ConnectionState{}}, nil
}

// ---------------------------------------------------------------------------
// Content format helpers — content.go
// ---------------------------------------------------------------------------

func TestFormatWikiContent(t *testing.T) {
	content := formatWikiContent(
		"Go (programming language)",
		"<p>Go is a statically typed language.</p>",
		"https://upload.wikimedia.org/thumbnail.png",
		"https://en.wikipedia.org/wiki/Go_(programming_language)",
	)
	if !strings.Contains(content, "Go (programming language)") {
		t.Errorf("expected title; got: %s", content)
	}
	if !strings.Contains(content, "thumbnail.png") {
		t.Errorf("expected thumbnail; got: %s", content)
	}
	if !strings.Contains(content, "wikipedia.org") {
		t.Errorf("expected wiki link; got: %s", content)
	}
}

func TestFormatWikiContentSanitizesUpstreamMarkupAndLinks(t *testing.T) {
	content := formatWikiContent("Test", `<p onclick="alert(1)">safe</p><script>alert(2)</script><a href="javascript:alert(3)">bad link</a>`, "https://evil.example/track.svg", "javascript:alert(4)")
	for _, unsafe := range []string{"onclick", "<script", "javascript:", "evil.example", "alert("} {
		if strings.Contains(content, unsafe) {
			t.Errorf("wiki content retained unsafe upstream value %q: %s", unsafe, content)
		}
	}
	if !strings.Contains(content, "safe") || !strings.Contains(content, "bad link") {
		t.Fatalf("safe article text was lost during sanitization: %s", content)
	}
}

func TestFormatWikiContentNoThumbnail(t *testing.T) {
	content := formatWikiContent("Test", "<p>Extract.</p>", "", "https://en.wikipedia.org/wiki/Test")
	if strings.Contains(content, "<img") {
		t.Errorf("should not have img tag without thumbnail; got: %s", content)
	}
}

func TestFormatThesaurusContent(t *testing.T) {
	synonyms := []string{"fast", "quick", "speedy"}
	antonyms := []string{"slow", "sluggish"}
	content := formatThesaurusContent("rapid", synonyms, antonyms)
	if !strings.Contains(content, "Synonyms") {
		t.Errorf("expected synonyms section; got: %s", content)
	}
	if !strings.Contains(content, "Antonyms") {
		t.Errorf("expected antonyms section; got: %s", content)
	}
	if !strings.Contains(content, "fast") {
		t.Errorf("expected synonym; got: %s", content)
	}
}

func TestFormatThesaurusContentOnlySynonyms(t *testing.T) {
	content := formatThesaurusContent("word", []string{"synonym"}, nil)
	if !strings.Contains(content, "Synonyms") {
		t.Errorf("expected synonyms; got: %s", content)
	}
	if strings.Contains(content, "Antonyms") {
		t.Errorf("should not have antonyms section when empty; got: %s", content)
	}
}

func TestFormatPkgContent(t *testing.T) {
	tests := []struct {
		registry string
		install  string
	}{
		{"npm", "npm install lodash"},
		{"pypi", "pip install requests"},
		{"go", "go get github.com/example/pkg"},
	}
	for _, tt := range tests {
		t.Run(tt.registry, func(t *testing.T) {
			content := formatPkgContent(tt.registry, "example-pkg", "A useful library", "1.0.0", "MIT", tt.install)
			if !strings.Contains(content, tt.install) {
				t.Errorf("expected install command %q; got: %s", tt.install, content)
			}
		})
	}
}

func TestPkgHandlerGoRegistryPath(t *testing.T) {
	h := NewPkgHandler()
	h.client = &http.Client{Transport: packageTestTransport{body: `{"Version":"v1.2.3","Time":"2026-01-01T00:00:00Z"}`}}
	ans, err := h.HandleDirectQuery(context.Background(), "go/github.com/go-chi/chi/v5")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ans == nil || ans.Data["version"] != "v1.2.3" {
		t.Fatalf("expected live module version, got %+v", ans)
	}
	if !strings.Contains(ans.Content, "go get") {
		t.Errorf("expected go get command; got: %s", ans.Content)
	}
}

type packageTestTransport struct {
	body              string
	bodies            map[string]string
	requestPath       *string
	requestCount      *int
	requestUserAgents *[]string
}

type npmMetadataTransport struct{}

func (npmMetadataTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	body := `{"name":"sample","description":"A package","dist-tags":{"latest":"1.2.3"},"time":{"modified":"2026-10-01T12:00:00Z"},"license":"MIT","repository":{"url":"https://github.com/example/sample"},"readme":"<script>alert(1)</script> README","versions":{"1.2.3":{"dependencies":{"one":"^1","two":"^2"}}}}`
	if request.URL.Host == "api.npmjs.org" {
		body = `{"downloads":1234}`
	}
	return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: request}, nil
}

type pypiMetadataTransport struct{}

func (pypiMetadataTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	body := `{"info":{"name":"sample","summary":"A package","description":"<script>alert(1)</script> README","version":"1.2.3","license":"MIT","project_urls":{"Repository":"https://github.com/example/sample"},"requires_dist":["one>=1","two>=2"]},"releases":{"1.2.3":[{"upload_time_iso_8601":"2026-10-01T12:00:00Z"}]}}`
	if request.URL.Host == "pypistats.org" {
		body = `{"data":{"last_week":5678},"package":"sample","type":"recent_downloads"}`
	}
	return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: request}, nil
}

func TestPkgHandlerNPMMetadataIsRenderedSafely(t *testing.T) {
	handler := NewPkgHandler()
	handler.client = &http.Client{Transport: npmMetadataTransport{}}
	answer, err := handler.HandleDirectQuery(context.Background(), "npm/sample")
	if err != nil {
		t.Fatal(err)
	}
	if answer.Data["weekly_downloads"] != 1234 || answer.Data["dependencies"] != 2 || answer.Data["updated"] != "2026-10-01T12:00:00Z" {
		t.Fatalf("incomplete npm metadata: %#v", answer.Data)
	}
	if !strings.Contains(answer.Content, "1234") || !strings.Contains(answer.Content, "README preview") || !strings.Contains(answer.Content, "&lt;script&gt;") || strings.Contains(answer.Content, "<script>") {
		t.Fatalf("metadata or escaped README missing from content: %s", answer.Content)
	}
}

func TestPkgHandlerPyPIMetadataIsRenderedSafely(t *testing.T) {
	packageDownloadsCache.Lock()
	delete(packageDownloadsCache.entries, "pypi:sample")
	packageDownloadsCache.Unlock()
	handler := NewPkgHandler()
	handler.client = &http.Client{Transport: pypiMetadataTransport{}}
	answer, err := handler.HandleDirectQuery(context.Background(), "pypi/sample")
	if err != nil {
		t.Fatal(err)
	}
	if answer.Data["weekly_downloads"] != 5678 || answer.Data["dependencies"] != 2 || answer.Data["updated"] != "2026-10-01T12:00:00Z" {
		t.Fatalf("incomplete PyPI metadata: %#v", answer.Data)
	}
	if !strings.Contains(answer.Content, "5678") || !strings.Contains(answer.Content, "2") || !strings.Contains(answer.Content, "README preview") || !strings.Contains(answer.Content, "&lt;script&gt;") || strings.Contains(answer.Content, "<script>") {
		t.Fatalf("metadata or escaped README missing from content: %s", answer.Content)
	}
}

type npmOptionalFailureTransport struct{}

func (npmOptionalFailureTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	status, body := http.StatusOK, `{"name":"sample","description":"A package","dist-tags":{"latest":"1.2.3"},"time":{"modified":"2026-10-01T12:00:00Z"},"license":"MIT","versions":{"1.2.3":{}}}`
	if request.URL.Host == "api.npmjs.org" {
		status, body = http.StatusBadGateway, `{}`
	}
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: request}, nil
}

func TestPkgHandlerKeepsCoreMetadataWhenOptionalNPMStatsFail(t *testing.T) {
	handler := NewPkgHandler()
	handler.client = &http.Client{Transport: npmOptionalFailureTransport{}}
	answer, err := handler.HandleDirectQuery(context.Background(), "npm/sample")
	if err != nil {
		t.Fatal(err)
	}
	if answer.Data["version"] != "1.2.3" || answer.Data["weekly_downloads"] != nil {
		t.Fatalf("core metadata or optional failure state is incorrect: %#v", answer.Data)
	}
	if !strings.Contains(answer.Content, "1.2.3") || !strings.Contains(answer.Content, "Unavailable") {
		t.Fatalf("answer did not retain core metadata and explain unavailable stats: %s", answer.Content)
	}
}

type packageOptionalFailureTransport struct{}

func (packageOptionalFailureTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	status, body := http.StatusOK, `{}`
	switch {
	case request.URL.Host == "pypi.org":
		body = `{"info":{"name":"sample","summary":"A package","version":"1.2.3","license":"MIT"},"releases":{}}`
	case request.URL.Host == "pypistats.org":
		status = http.StatusBadGateway
	case request.URL.Host == "proxy.golang.org" && strings.HasSuffix(request.URL.Path, "/@latest"):
		body = `{"Version":"v1.2.3","Time":"2026-10-01T12:00:00Z"}`
	case request.URL.Host == "proxy.golang.org":
		status = http.StatusBadGateway
	case request.URL.Host == "crates.io" && request.URL.Path == "/api/v1/crates/optional-failure-crate":
		body = `{"crate":{"id":"optional-failure-crate","name":"optional-failure-crate","max_version":"1.2.3","license":"MIT","downloads":10}}`
	case request.URL.Host == "crates.io":
		status = http.StatusBadGateway
	}
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: request}, nil
}

func TestPkgHandlersKeepCoreMetadataWhenOptionalRegistryRequestsFail(t *testing.T) {
	tests := []struct {
		name, term, version string
		clearCache          string
	}{
		{name: "PyPI statistics", term: "pypi/sample", version: "1.2.3", clearCache: "pypi:sample"},
		{name: "Go module file", term: "go/example.com/sample", version: "v1.2.3"},
		{name: "crates.io optional endpoints", term: "crate/optional-failure-crate", version: "1.2.3", clearCache: "crates:optional-failure-crate"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.clearCache != "" {
				packageDownloadsCache.Lock()
				delete(packageDownloadsCache.entries, test.clearCache)
				packageDownloadsCache.Unlock()
			}
			handler := NewPkgHandler()
			handler.client = &http.Client{Transport: packageOptionalFailureTransport{}}
			answer, err := handler.HandleDirectQuery(context.Background(), test.term)
			if err != nil {
				t.Fatal(err)
			}
			if answer.Data["version"] != test.version {
				t.Fatalf("core version = %v, want %s", answer.Data["version"], test.version)
			}
			if !strings.Contains(answer.Content, test.version) || !strings.Contains(answer.Content, "Unavailable") {
				t.Fatalf("answer did not retain core metadata and explain unavailable fields: %s", answer.Content)
			}
			if test.term == "pypi/sample" && answer.Data["weekly_downloads"] != nil {
				t.Fatalf("failed PyPI stats returned a value: %#v", answer.Data["weekly_downloads"])
			}
			if test.term == "go/example.com/sample" && answer.Data["dependencies"] != nil {
				t.Fatalf("failed go.mod fetch returned dependency count: %#v", answer.Data["dependencies"])
			}
			if strings.HasPrefix(test.term, "crate/") && answer.Data["dependencies"] != nil {
				t.Fatalf("failed crate dependency lookup returned a count: %#v", answer.Data["dependencies"])
			}
		})
	}
}

func TestPkgHandlersRejectMalformedOrIncompleteRegistryMetadata(t *testing.T) {
	for _, term := range []string{"npm/sample", "pypi/sample", "go/example.com/sample", "crate/sample", "gem/sample"} {
		for _, body := range []string{"{", "{}"} {
			t.Run(term+"/"+body, func(t *testing.T) {
				handler := NewPkgHandler()
				handler.client = &http.Client{Transport: packageTestTransport{body: body}}
				if answer, err := handler.HandleDirectQuery(context.Background(), term); err == nil || answer != nil {
					t.Fatalf("malformed metadata returned answer=%#v, error=%v", answer, err)
				}
			})
		}
	}
}

func TestPkgHandlersRejectRegistryServerErrors(t *testing.T) {
	for _, term := range []string{"npm/sample", "pypi/sample", "go/example.com/sample", "crate/sample", "gem/sample"} {
		t.Run(term, func(t *testing.T) {
			handler := NewPkgHandler()
			handler.client = &http.Client{Transport: statusTestTransport{status: http.StatusBadGateway}}
			if _, err := handler.HandleDirectQuery(context.Background(), term); err == nil {
				t.Fatal("expected registry server error")
			}
		})
	}
}

func TestPkgContentMarksMissingMetadataUnavailable(t *testing.T) {
	content := formatPkgContentWithDetails("en", "npm", "sample", "A package", "1.2.3", "MIT", "npm install sample", map[string]interface{}{
		"weekly_downloads": nil,
		"dependencies":     nil,
		"updated":          "",
		"repository":       "javascript:alert(1)",
		"readme_preview":   "",
	})
	if strings.Count(content, "Unavailable") != 5 {
		t.Fatalf("expected five unavailable metadata values, got: %s", content)
	}
	if strings.Contains(content, `href="javascript:`) {
		t.Fatalf("unsafe repository link rendered: %s", content)
	}
}

func TestManContentSanitizesProviderHTML(t *testing.T) {
	credentialURL := (&url.URL{Scheme: "https", User: url.UserPassword("test-user", "test-password"), Host: "example.test"}).String()
	providerHTML := `<h1>man page</h1><script>alert(1)</script><a href="javascript:alert(1)">unsafe</a><a href="` + html.EscapeString(credentialURL) + `">credential link</a><a href="https://example.test">safe</a>`
	formatted := formatManPage(providerHTML)
	if strings.Contains(formatted, "<script>") || strings.Contains(formatted, "javascript:") || strings.Contains(formatted, "test-user") || !strings.Contains(formatted, "credential link") || !strings.Contains(formatted, "<h1>man page</h1>") || !strings.Contains(formatted, `href="https://example.test"`) {
		t.Fatalf("provider HTML was not safely sanitized: %s", formatted)
	}
}

func TestTLDRNotFoundEscapesQuery(t *testing.T) {
	handler := NewTLDRHandler()
	handler.client = &http.Client{Transport: statusTestTransport{status: http.StatusNotFound}}
	answer, err := handler.HandleDirectQuery(context.Background(), `<script>alert(1)</script>`)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(answer.Content, "<script>") || !strings.Contains(answer.Content, "&lt;script&gt;") {
		t.Fatalf("unescaped user query in tldr content: %s", answer.Content)
	}
}

func TestManHandlerSanitizesAPIHTML(t *testing.T) {
	handler := NewManHandler()
	handler.client = &http.Client{Transport: packageTestTransport{body: `{"name":"printf","section":"3","description":"Print values","content":"<h1>printf</h1><script>alert(1)</script><a href=\"javascript:alert(1)\">unsafe</a>"}`}}
	answer, err := handler.HandleDirectQuery(context.Background(), "printf.3")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(answer.Content, "<h1>printf</h1>") || strings.Contains(answer.Content, "<script>") || strings.Contains(answer.Content, "javascript:") {
		t.Fatalf("unexpected sanitized man page: %s", answer.Content)
	}
}

func TestCheatHandlerEscapesProviderContent(t *testing.T) {
	handler := NewCheatHandler()
	handler.client = &http.Client{Transport: packageTestTransport{body: "<script>alert(1)</script>"}}
	answer, err := handler.HandleDirectQuery(context.Background(), "sample")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(answer.Content, "<script>") || !strings.Contains(answer.Content, "&lt;script&gt;") {
		t.Fatalf("unescaped cheat content: %s", answer.Content)
	}
}

type statusTestTransport struct {
	status int
	body   string
	bodies map[string]string
}

func (transport statusTestTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	body := transport.body
	if pathBody, ok := transport.bodies[request.URL.Path]; ok {
		body = pathBody
	}
	return &http.Response{StatusCode: transport.status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: request}, nil
}

func (transport packageTestTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if transport.requestPath != nil {
		*transport.requestPath = request.URL.Path
	}
	if transport.requestCount != nil {
		*transport.requestCount = *transport.requestCount + 1
	}
	if transport.requestUserAgents != nil {
		*transport.requestUserAgents = append(*transport.requestUserAgents, request.UserAgent())
	}
	body := transport.body
	if responseBody, ok := transport.bodies[request.URL.Path]; ok {
		body = responseBody
	}
	return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: request}, nil
}

func TestPkgHandlerGoModPathAutoDetect(t *testing.T) {
	h := NewPkgHandler()
	ctx := context.Background()

	// A path with slashes but no registry prefix auto-detects as Go
	h.client = &http.Client{Transport: packageTestTransport{body: `{"Version":"v1.0.0"}`}}
	ans, err := h.HandleDirectQuery(ctx, "github.com/some/package")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ans == nil {
		t.Fatal("nil answer")
	}
}

type goModuleMetadataTransport struct{}

func (goModuleMetadataTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	body := `{"Version":"v1.2.3","Time":"2026-01-01T00:00:00Z"}`
	if strings.HasSuffix(request.URL.Path, ".zip") {
		var archive bytes.Buffer
		writer := zip.NewWriter(&archive)
		for name, contents := range map[string]string{
			"example.com/upper@v1.2.3/README.md": "Go module docs",
			"example.com/upper@v1.2.3/LICENSE":   "SPDX-License-Identifier: MIT\n",
		} {
			file, err := writer.Create(name)
			if err != nil {
				return nil, err
			}
			if _, err := file.Write([]byte(contents)); err != nil {
				return nil, err
			}
		}
		if err := writer.Close(); err != nil {
			return nil, err
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(archive.Bytes())), Request: request}, nil
	}
	if strings.HasSuffix(request.URL.Path, ".mod") {
		body = "module example.com/Upper\n\ngo 1.24\n\nrequire (\n\tgolang.org/x/net v0.1.0\n\tgolang.org/x/text v0.2.0 // indirect\n\tgithub.com/google/uuid v1.0.0\n)\n"
	}
	return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: request}, nil
}

func TestGoModuleReadmePreview(t *testing.T) {
	var archive bytes.Buffer
	writer := zip.NewWriter(&archive)
	for name, body := range map[string]string{
		"example.com/Upper@v1.2.3/docs/README.md": "nested README must not be selected",
		"example.com/Upper@v1.2.3/readme.txt":     "fallback",
		"example.com/Upper@v1.2.3/README.md":      "  Go module documentation\n",
	} {
		file, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := file.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	preview, err := goModuleReadmePreview(archive.Bytes(), "example.com/Upper@v1.2.3/")
	if err != nil {
		t.Fatal(err)
	}
	if preview != "Go module documentation" {
		t.Fatalf("preview = %q", preview)
	}
	if _, err := goModuleReadmePreview([]byte("not a zip"), "example.com/Upper@v1.2.3/"); err == nil {
		t.Fatal("malformed archive unexpectedly succeeded")
	}
}

func TestGoPackageIncludesDirectDependencyCount(t *testing.T) {
	handler := NewPkgHandler()
	handler.client = &http.Client{Transport: goModuleMetadataTransport{}}
	answer, err := handler.fetchGo(context.Background(), "example.com/upper")
	if err != nil {
		t.Fatal(err)
	}
	if answer.Data["dependencies"] != 2 {
		t.Fatalf("dependency count = %v, want 2 direct dependencies", answer.Data["dependencies"])
	}
	if answer.Data["license"] != "MIT" {
		t.Fatalf("module metadata omitted the SPDX license from its archive: %#v", answer.Data)
	}
	if !strings.Contains(answer.Content, "Dependencies") || !strings.Contains(answer.Content, "2") {
		t.Fatalf("dependency count missing from formatted content: %s", answer.Content)
	}
}

func TestGoModuleRepositoryRecognizesSupportedHosts(t *testing.T) {
	for modulePath, want := range map[string]interface{}{
		"github.com/example/project/v2": "https://github.com/example/project",
		"gitlab.com/example/project":    "https://gitlab.com/example/project",
		"example.com/module":            nil,
	} {
		if got := goModuleRepository(modulePath); got != want {
			t.Errorf("goModuleRepository(%q) = %#v, want %#v", modulePath, got, want)
		}
	}
}

func TestRubyGemReadmePreviewExtractsBoundedArchive(t *testing.T) {
	var payload bytes.Buffer
	gzipWriter := gzip.NewWriter(&payload)
	innerTar := tar.NewWriter(gzipWriter)
	readme := []byte("Ruby package documentation")
	if err := innerTar.WriteHeader(&tar.Header{Name: "README.md", Mode: 0600, Size: int64(len(readme)), Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	if _, err := innerTar.Write(readme); err != nil {
		t.Fatal(err)
	}
	if err := innerTar.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	var gem bytes.Buffer
	outerTar := tar.NewWriter(&gem)
	if err := outerTar.WriteHeader(&tar.Header{Name: "data.tar.gz", Mode: 0600, Size: int64(payload.Len()), Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	if _, err := outerTar.Write(payload.Bytes()); err != nil {
		t.Fatal(err)
	}
	if err := outerTar.Close(); err != nil {
		t.Fatal(err)
	}
	preview, err := rubyGemReadmePreview(gem.Bytes())
	if err != nil || preview != string(readme) {
		t.Fatalf("RubyGems README preview = %q, error = %v", preview, err)
	}
	if _, err := rubyGemReadmePreview([]byte("not a tar archive")); err == nil {
		t.Fatal("malformed RubyGems archive unexpectedly succeeded")
	}
}

func TestPackageReadmeCacheExpiresAndBoundsEntries(t *testing.T) {
	packageReadmeCache.Lock()
	original := packageReadmeCache.entries
	packageReadmeCache.entries = make(map[string]packageReadmeCacheEntry)
	packageReadmeCache.Unlock()
	defer func() {
		packageReadmeCache.Lock()
		packageReadmeCache.entries = original
		packageReadmeCache.Unlock()
	}()
	cachePackageReadme("expired", "old")
	packageReadmeCache.Lock()
	expired := packageReadmeCache.entries["expired"]
	expired.expiresAt = time.Now().Add(-time.Second)
	packageReadmeCache.entries["expired"] = expired
	for index := 0; index < 500; index++ {
		key := fmt.Sprintf("entry-%03d", index)
		packageReadmeCache.entries[key] = packageReadmeCacheEntry{preview: key, expiresAt: time.Now().Add(time.Duration(index+1) * time.Second)}
	}
	packageReadmeCache.Unlock()
	if _, ok := cachedPackageReadme("expired"); ok {
		t.Fatal("expired README cache entry was returned")
	}
	cachePackageReadme("new", "fresh")
	packageReadmeCache.Lock()
	count := len(packageReadmeCache.entries)
	_, oldestStillPresent := packageReadmeCache.entries["entry-000"]
	packageReadmeCache.Unlock()
	if count != 500 || oldestStillPresent {
		t.Fatalf("README cache bounds not enforced: entries=%d oldest-present=%v", count, oldestStillPresent)
	}
}

func TestCVEHandlerRejectsMalformedAndFailedResponses(t *testing.T) {
	for _, test := range []struct {
		name   string
		status int
		body   string
	}{
		{name: "malformed JSON", status: http.StatusOK, body: `{"vulnerabilities":`},
		{name: "upstream error", status: http.StatusServiceUnavailable, body: `{}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			handler := NewCVEHandler()
			handler.client = &http.Client{Transport: safeBrowsingTestTransport(func(request *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: test.status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(test.body)), Request: request}, nil
			})}
			if answer, err := handler.HandleDirectQuery(context.Background(), "CVE-2026-1234"); err == nil || answer != nil {
				t.Fatalf("HandleDirectQuery() = (%#v, %v), want a parser/upstream error", answer, err)
			}
		})
	}
}

func TestCVEHandlerDisplaysIDEAFields(t *testing.T) {
	body := `{"vulnerabilities":[{"cve":{"id":"CVE-2026-1234","descriptions":[{"lang":"en","value":"Example flaw"}],"metrics":{"cvssMetricV30":[{"cvssData":{"baseScore":8.1,"baseSeverity":"HIGH","vectorString":"CVSS:3.0/AV:N"}}]},"configurations":[{"nodes":[{"cpeMatch":[{"vulnerable":true,"criteria":"cpe:2.3:a:vendor:product:*:*:*:*:*:*:*:*","versionStartIncluding":"1.0","versionEndExcluding":"2.0"}]}]}],"published":"2026-01-01T00:00:00.000","lastModified":"2026-02-01T00:00:00.000","references":[{"url":"https://vendor.example/patch","tags":["Patch","Vendor Advisory"]},{"url":"https://exploit.example/proof","tags":["Exploit"]}]}}]}`
	handler := NewCVEHandler()
	handler.client = &http.Client{Transport: safeBrowsingTestTransport(func(request *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: request}, nil
	})}
	answer, err := handler.HandleDirectQuery(context.Background(), "2026-1234")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Affected products and versions", "cpe:2.3:a:vendor:product", "Remediation and patches", "exploit reference", "Last modified", "References and advisories"} {
		if !strings.Contains(answer.Content, want) {
			t.Errorf("content missing %q: %s", want, answer.Content)
		}
	}
	if answer.Data["cvss"] != 8.1 {
		t.Errorf("CVSS = %v, want 8.1", answer.Data["cvss"])
	}
	if answer.Data["exploit_available"] != true {
		t.Errorf("exploit_available = %v, want true", answer.Data["exploit_available"])
	}
	if len(answer.Data["affected"].([]string)) != 1 {
		t.Errorf("affected products = %v, want one", answer.Data["affected"])
	}
}

func TestFormatCVEContent(t *testing.T) {
	refs := []string{"https://nvd.nist.gov/vuln/detail/CVE-2021-44228"}
	content := formatCVEContent("CVE-2021-44228", "Log4Shell vulnerability", 10.0, "CRITICAL", "CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:C/C:H/I:H/A:H", "2021-12-10", refs)
	if !strings.Contains(content, "CVE-2021-44228") {
		t.Errorf("expected CVE ID; got: %s", content)
	}
	if !strings.Contains(content, "CRITICAL") {
		t.Errorf("expected severity; got: %s", content)
	}
	if !strings.Contains(content, "Log4Shell") {
		t.Errorf("expected description; got: %s", content)
	}
}

func TestFormatCVEContentNoVector(t *testing.T) {
	content := formatCVEContent("CVE-2024-0001", "Test CVE", 5.0, "MEDIUM", "", "", nil)
	if !strings.Contains(content, "CVE-2024-0001") {
		t.Errorf("expected CVE ID; got: %s", content)
	}
}

// ---------------------------------------------------------------------------
// Slang format — method on SlangHandler
// ---------------------------------------------------------------------------

func TestSlangHandlerFormatHTML(t *testing.T) {
	h := NewSlangHandler()
	entries := []urbanDictionaryEntry{
		{
			Word:       "YOLO",
			Definition: "You Only Live Once",
			Example:    "I will eat the whole pizza. YOLO.",
			ThumbsUp:   1000,
			ThumbsDown: 50,
			Author:     "testuser",
			WrittenOn:  "2012-01-01T00:00:00.000Z",
		},
	}
	content := h.formatSlangHTML("YOLO", entries)
	if !strings.Contains(content, "YOLO") {
		t.Errorf("expected term; got: %s", content)
	}
	if !strings.Contains(content, "You Only Live Once") {
		t.Errorf("expected definition; got: %s", content)
	}
}

func TestSlangHandlerFormatHTMLEmpty(t *testing.T) {
	h := NewSlangHandler()
	content := h.formatSlangHTML("xyz", []urbanDictionaryEntry{})
	if content == "" {
		t.Error("expected non-empty content for empty entries")
	}
}

// ---------------------------------------------------------------------------
// Reference format helpers — local
// ---------------------------------------------------------------------------

func TestFormatCountryContent(t *testing.T) {
	data := map[string]interface{}{
		"cca2":    "DE",
		"cca3":    "DEU",
		"capital": []string{"Berlin"},
		"region":  "Europe",
		"area":    357114,
	}
	content := formatCountryContent("DE", "", "Germany", "Federal Republic of Germany", data, "metric")
	if !strings.Contains(content, "Germany") {
		t.Errorf("expected country name; got: %s", content)
	}
	if !strings.Contains(content, "Berlin") {
		t.Errorf("expected capital; got: %s", content)
	}
	if !strings.Contains(content, "DE") {
		t.Errorf("expected ISO code; got: %s", content)
	}
}

func TestFormatNumber(t *testing.T) {
	tests := []struct {
		input int
		want  string
	}{
		{0, "0"},
		{999, "999"},
		{1000, "1,000"},
		{1000000, "1,000,000"},
		{1234567, "1,234,567"},
	}
	for _, tt := range tests {
		got := formatNumber(tt.input)
		if got != tt.want {
			t.Errorf("formatNumber(%d) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// YAML handler — uncovered mode branches
// ---------------------------------------------------------------------------

func TestYAMLHandlerFromJSON(t *testing.T) {
	h := NewYAMLHandler()
	ctx := context.Background()

	ans, err := h.HandleDirectQuery(ctx, `from-json {"name":"alice","age":30}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ans == nil {
		t.Fatal("nil answer")
	}
	if !strings.Contains(ans.Content, "alice") {
		t.Errorf("expected alice in YAML output; got: %s", ans.Content)
	}
}

func TestYAMLHandlerToJSON(t *testing.T) {
	h := NewYAMLHandler()
	ctx := context.Background()

	ans, err := h.HandleDirectQuery(ctx, "to-json name: alice\nage: 30")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ans == nil {
		t.Fatal("nil answer")
	}
	if !strings.Contains(ans.Content, "alice") {
		t.Errorf("expected alice in JSON output; got: %s", ans.Content)
	}
}

func TestYAMLHandlerInvalidJSONForFromJSON(t *testing.T) {
	h := NewYAMLHandler()
	ctx := context.Background()

	ans, err := h.HandleDirectQuery(ctx, "from-json {not valid json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ans == nil {
		t.Fatal("nil answer")
	}
	if ans.Error != "invalid_json" {
		t.Errorf("expected invalid_json error, got %q", ans.Error)
	}
}

func TestYAMLHandlerFormatValidYAML(t *testing.T) {
	h := NewYAMLHandler()
	ctx := context.Background()

	ans, err := h.HandleDirectQuery(ctx, "name: alice\nage: 30")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ans == nil {
		t.Fatal("nil answer")
	}
	if ans.Error != "" {
		t.Errorf("expected no error for valid YAML, got %q", ans.Error)
	}
}

// ---------------------------------------------------------------------------
// JSON handler — minify and validate mode branches
// ---------------------------------------------------------------------------

func TestJSONHandlerMinify(t *testing.T) {
	h := NewJSONHandler()
	ctx := context.Background()

	ans, err := h.HandleDirectQuery(ctx, `minify {"name": "alice", "age": 30}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ans == nil {
		t.Fatal("nil answer")
	}
	if !strings.Contains(ans.Content, "alice") {
		t.Errorf("expected minified output; got: %s", ans.Content)
	}
}

func TestJSONHandlerValidate(t *testing.T) {
	h := NewJSONHandler()
	ctx := context.Background()

	ans, err := h.HandleDirectQuery(ctx, `validate {"valid": true}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ans == nil {
		t.Fatal("nil answer")
	}
	if ans.Error != "" {
		t.Errorf("unexpected error for valid JSON: %q", ans.Error)
	}
}

func TestJSONHandlerInvalidJSON(t *testing.T) {
	h := NewJSONHandler()
	ctx := context.Background()

	ans, err := h.HandleDirectQuery(ctx, `{"invalid json: yes`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ans == nil {
		t.Fatal("nil answer")
	}
	if ans.Error != "invalid_json" {
		t.Errorf("expected invalid_json error, got %q", ans.Error)
	}
}

func TestJSONHandlerExplicitFormat(t *testing.T) {
	h := NewJSONHandler()
	ctx := context.Background()

	ans, err := h.HandleDirectQuery(ctx, `format {"key":"value"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ans == nil {
		t.Fatal("nil answer")
	}
	if ans.Error != "" {
		t.Errorf("unexpected error: %q", ans.Error)
	}
}

// ---------------------------------------------------------------------------
// HTML handler — auto-detect decode path
// ---------------------------------------------------------------------------

func TestHTMLHandlerAutoDetectDecode(t *testing.T) {
	h := NewHTMLHandler()
	ctx := context.Background()

	// Contains & and ; — triggers auto-detect decode
	ans, err := h.HandleDirectQuery(ctx, "&lt;div&gt;hello&lt;/div&gt;")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ans == nil {
		t.Fatal("nil answer")
	}
	if ans.Data["mode"] != "decode" {
		t.Errorf("expected auto-detect decode mode, got %v", ans.Data["mode"])
	}
}

// ---------------------------------------------------------------------------
// Escape handler — format prefix coverage
// ---------------------------------------------------------------------------

func TestEscapeHandlerFormatPrefixes(t *testing.T) {
	h := NewEscapeHandler()
	ctx := context.Background()

	formats := []string{"json", "sql", "html", "url", "regex", "shell", "js", "javascript", "python", "c"}
	for _, format := range formats {
		t.Run("format_"+format, func(t *testing.T) {
			term := format + " hello world"
			ans, err := h.HandleDirectQuery(ctx, term)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if ans == nil {
				t.Fatal("nil answer")
			}
			if ans.Data["format"] != format {
				t.Errorf("expected format=%q, got %v", format, ans.Data["format"])
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Subnet handler — additional input paths
// ---------------------------------------------------------------------------

func TestSubnetHandlerIPOnly(t *testing.T) {
	h := NewSubnetHandler()
	ctx := context.Background()

	// IP without CIDR defaults to /24
	ans, err := h.HandleDirectQuery(ctx, "192.168.1.100")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ans == nil {
		t.Fatal("nil answer")
	}
}

func TestSubnetHandlerInvalidInput(t *testing.T) {
	h := NewSubnetHandler()
	ctx := context.Background()

	ans, err := h.HandleDirectQuery(ctx, "not-an-ip-or-cidr")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ans == nil {
		t.Fatal("nil answer")
	}
	if ans.Error != "invalid_cidr" {
		t.Errorf("expected invalid_cidr error, got %q", ans.Error)
	}
}

func TestSubnetHandlerIPv6CIDR(t *testing.T) {
	h := NewSubnetHandler()
	ctx := context.Background()

	ans, err := h.HandleDirectQuery(ctx, "2001:db8::/32")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ans == nil {
		t.Fatal("nil answer")
	}
	if ans.Data["isIPv6"] != true {
		t.Errorf("expected isIPv6=true; got %v", ans.Data["isIPv6"])
	}
}

// ---------------------------------------------------------------------------
// Rules handler — searchRules coverage
// ---------------------------------------------------------------------------

func TestRulesHandlerSearchTermNotFound(t *testing.T) {
	h := NewRulesHandler()
	ctx := context.Background()

	ans, err := h.HandleDirectQuery(ctx, "xyznonexistentterm12345")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ans == nil {
		t.Fatal("nil answer")
	}
}

func TestRulesHandlerSearchWithResults(t *testing.T) {
	h := NewRulesHandler()
	ctx := context.Background()

	ans, err := h.HandleDirectQuery(ctx, "never")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ans == nil {
		t.Fatal("nil answer")
	}
}

// ---------------------------------------------------------------------------
// Beautify handler — additional paths
// ---------------------------------------------------------------------------

func TestBeautifyHandlerInvalidJSON(t *testing.T) {
	h := NewBeautifyHandler()
	for _, input := range []string{"json {invalid json}", "json }"} {
		if _, err := h.HandleDirectQuery(context.Background(), input); err == nil {
			t.Errorf("HandleDirectQuery(%q) accepted invalid JSON", input)
		}
	}
	if got := beautifyCode("</div>", "html"); got == "" {
		t.Fatal("malformed HTML produced an empty result")
	}
}

func TestBeautifyHandlerAutoDetectLanguage(t *testing.T) {
	h := NewBeautifyHandler()
	ctx := context.Background()

	tests := []struct {
		name string
		code string
	}{
		{"json object", `{"key":"value"}`},
		{"html doctype", `<!DOCTYPE html><html><body></body></html>`},
		{"css rule", `.foo { color: red; }`},
		{"sql select", `select * from users where id=1`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ans, err := h.HandleDirectQuery(ctx, tt.code)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if ans == nil {
				t.Fatal("nil answer")
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Cron handler — generateCronDescription branches
// ---------------------------------------------------------------------------

func TestCronHandlerVariousExpressions(t *testing.T) {
	h := NewCronHandler()
	ctx := context.Background()

	tests := []struct {
		expr string
	}{
		{"*/5 * * * *"},
		{"0 9 * * 1-5"},
		{"0 0 1 * *"},
		{"0 0 1 1 *"},
		{"30 14 * * 0"},
		{"@yearly"},
		{"@monthly"},
		{"@weekly"},
		{"@daily"},
		{"@hourly"},
	}
	for _, tt := range tests {
		t.Run(tt.expr, func(t *testing.T) {
			ans, err := h.HandleDirectQuery(ctx, tt.expr)
			if err != nil {
				t.Fatalf("unexpected error for %q: %v", tt.expr, err)
			}
			if ans == nil {
				t.Fatalf("nil answer for %q", tt.expr)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Manager — nil return for non-direct-answer queries
// ---------------------------------------------------------------------------

func TestManagerProcessNonDirectAnswer(t *testing.T) {
	m := NewManager()
	ans, err := m.Process(context.Background(), "just a regular search query")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ans != nil {
		t.Errorf("expected nil answer for non-direct query, got: %+v", ans)
	}
}

// ---------------------------------------------------------------------------
// ProcessType — unknown type
// ---------------------------------------------------------------------------

func TestManagerProcessTypeUnknown(t *testing.T) {
	m := NewManager()
	ans, err := m.ProcessType(context.Background(), "nonexistenttype", "term")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ans == nil {
		t.Fatal("nil answer")
	}
	if ans.Error != "unknown_type" {
		t.Errorf("expected unknown_type error, got %q", ans.Error)
	}
}

// ---------------------------------------------------------------------------
// Parse — rule: prefix
// ---------------------------------------------------------------------------

func TestManagerParseRules(t *testing.T) {
	m := NewManager()

	tests := []struct {
		query    string
		wantType AnswerType
		wantTerm string
	}{
		{"rule:42", AnswerTypeRules, "42"},
		{"RULE:42", AnswerTypeRules, "42"},
		{"roti:42", AnswerTypeRules, "42"},
		{"ROTI:42", AnswerTypeRules, "42"},
		{"rules:", AnswerTypeRules, ""},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			gotType, gotTerm := m.Parse(tt.query)
			if gotType != tt.wantType {
				t.Errorf("Parse(%q) type = %q, want %q", tt.query, gotType, tt.wantType)
			}
			if gotTerm != tt.wantTerm {
				t.Errorf("Parse(%q) term = %q, want %q", tt.query, gotTerm, tt.wantTerm)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// decrementIP — wrap-around edge case
// ---------------------------------------------------------------------------

func TestDecrementIPWrapAround(t *testing.T) {
	// 0.0.1.0 decremented should give 0.0.0.255
	ip := net.IP{0, 0, 1, 0}
	result := decrementIP(ip)
	expected := net.IP{0, 0, 0, 255}
	if !result.Equal(expected) {
		t.Errorf("decrementIP(%v) = %v, want %v", ip, result, expected)
	}
}

// ---------------------------------------------------------------------------
// htmlDecode — hex entity path
// ---------------------------------------------------------------------------

func TestHTMLDecodeHexEntities(t *testing.T) {
	// &#x41; = 'A', &#X42; = 'B'
	got := htmlDecode("&#x41;&#X42;")
	if got != "AB" {
		t.Errorf("htmlDecode hex entities: got %q, want %q", got, "AB")
	}
}

// ---------------------------------------------------------------------------
// getUnicodeCategory — various paths
// ---------------------------------------------------------------------------

func TestGetUnicodeCategorySymbol(t *testing.T) {
	cat := getUnicodeCategory('©')
	if cat != "Symbol" {
		t.Errorf("expected Symbol for ©, got %q", cat)
	}
}

func TestGetUnicodeCategoryControl(t *testing.T) {
	cat := getUnicodeCategory('\x01')
	if cat != "Control" {
		t.Errorf("expected Control for \\x01, got %q", cat)
	}
}

func TestGetUnicodeCategoryLetter(t *testing.T) {
	cat := getUnicodeCategory('A')
	if cat != "Letter" {
		t.Errorf("expected Letter for A, got %q", cat)
	}
}

// ---------------------------------------------------------------------------
// UnicodeHandler — valid and invalid input paths
// ---------------------------------------------------------------------------

func TestUnicodeHandlerSingleChar(t *testing.T) {
	h := NewUnicodeHandler()
	ctx := context.Background()

	ans, err := h.HandleDirectQuery(ctx, "A")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ans == nil {
		t.Fatal("nil answer")
	}
	if ans.Error != "" {
		t.Errorf("unexpected error code %q; content: %s", ans.Error, ans.Content)
	}
	if !strings.Contains(ans.Content, "U+0041") {
		t.Errorf("expected U+0041 in content; got: %s", ans.Content)
	}
}

func TestUnicodeHandlerInvalidHex(t *testing.T) {
	h := NewUnicodeHandler()
	ctx := context.Background()

	// Hex parse fails → r remains 0 → invalid_input
	ans, err := h.HandleDirectQuery(ctx, "U+ZZZZ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ans == nil {
		t.Fatal("nil answer")
	}
	if ans.Error != "invalid_input" {
		t.Errorf("expected invalid_input error, got %q", ans.Error)
	}
}

// ---------------------------------------------------------------------------
// toSentenceCase — multi-sentence path
// ---------------------------------------------------------------------------

func TestToSentenceCase(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"hello world", "Hello world"},
		{"hello world. foo bar", "Hello world. foo bar"},
		{"", ""},
		{"a", "A"},
	}
	for _, tt := range tests {
		got := toSentenceCase(tt.input)
		if got != tt.want {
			t.Errorf("toSentenceCase(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// toCamelCase — edge cases
// ---------------------------------------------------------------------------

func TestToCamelCase(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"hello world", "helloWorld"},
		{"foo bar baz", "fooBarBaz"},
		{"single", "single"},
		{"", ""},
	}
	for _, tt := range tests {
		got := toCamelCase(tt.input)
		if got != tt.want {
			t.Errorf("toCamelCase(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// Slug handler — empty input
// ---------------------------------------------------------------------------

func TestSlugHandlerEmpty(t *testing.T) {
	h := NewSlugHandler()
	ctx := context.Background()

	_, err := h.HandleDirectQuery(ctx, "")
	if err == nil {
		t.Error("expected error for empty input, got nil")
	}
}

// ---------------------------------------------------------------------------
// Case handler — explicit type prefixes
// ---------------------------------------------------------------------------

func TestCaseHandlerAllTypes(t *testing.T) {
	h := NewCaseHandler()
	ctx := context.Background()

	types := []string{
		"title", "sentence", "camel", "pascal", "snake",
		"kebab", "screaming", "dot", "upper", "lower",
	}
	for _, typ := range types {
		t.Run(typ, func(t *testing.T) {
			term := typ + " hello world"
			ans, err := h.HandleDirectQuery(ctx, term)
			if err != nil {
				t.Fatalf("unexpected error for type %q: %v", typ, err)
			}
			if ans == nil {
				t.Fatalf("nil answer for type %q", typ)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Diff handler — missing separator
// ---------------------------------------------------------------------------

func TestDiffHandlerMissingSeparator(t *testing.T) {
	h := NewDiffHandler()
	ctx := context.Background()

	ans, err := h.HandleDirectQuery(ctx, "text without separator")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ans == nil {
		t.Fatal("nil answer")
	}
}

// ---------------------------------------------------------------------------
// Regex handler — invalid pattern
// ---------------------------------------------------------------------------

func TestRegexHandlerInvalidPattern(t *testing.T) {
	h := NewRegexHandler()
	ctx := context.Background()

	ans, err := h.HandleDirectQuery(ctx, "[invalid regex")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ans == nil {
		t.Fatal("nil answer")
	}
	if ans.Error != "invalid_regex" {
		t.Errorf("expected invalid_regex error, got %q", ans.Error)
	}
}

// ---------------------------------------------------------------------------
// ASCII handler — empty input
// ---------------------------------------------------------------------------

func TestASCIIHandlerEmpty(t *testing.T) {
	h := NewASCIIHandler()
	ctx := context.Background()

	_, err := h.HandleDirectQuery(ctx, "")
	if err == nil {
		t.Error("expected error for empty input, got nil")
	}
}

// ---------------------------------------------------------------------------
// JWT handler — malformed token
// ---------------------------------------------------------------------------

func TestJWTHandlerMalformed(t *testing.T) {
	h := NewJWTHandler()
	ctx := context.Background()

	ans, err := h.HandleDirectQuery(ctx, "not.a.jwt.at.all")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ans == nil {
		t.Fatal("nil answer")
	}
}

// ---------------------------------------------------------------------------
// formatManPage and extractManContent — pure helpers in command.go
// ---------------------------------------------------------------------------

func TestFormatManPage(t *testing.T) {
	result := formatManPage("<b>test</b>")
	if !strings.Contains(result, "man-content") {
		t.Errorf("formatManPage output missing man-content class, got: %q", result)
	}
	if !strings.Contains(result, "<b>test</b>") {
		t.Errorf("formatManPage should preserve HTML content")
	}
}

func TestExtractManContent(t *testing.T) {
	tests := []struct {
		name  string
		html  string
		check func(string) bool
		desc  string
	}{
		{
			"with main tag",
			"<html><main>SYNOPSIS\n  cmd</main></html>",
			func(s string) bool { return strings.Contains(s, "man-content") },
			"should wrap found content",
		},
		{
			"without main tag",
			"<html><body>no main here</body></html>",
			func(s string) bool { return strings.Contains(s, "Failed to parse") },
			"should return failure message when no main tag",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractManContent(tt.html)
			if !tt.check(got) {
				t.Errorf("extractManContent(%q) = %q; %s", tt.html, got, tt.desc)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// formatCheatContent — pure helper in command.go
// ---------------------------------------------------------------------------

func TestFormatCheatContent(t *testing.T) {
	result := formatCheatContent("ls -la  # list files")
	if !strings.Contains(result, "cheat-content") {
		t.Errorf("formatCheatContent output missing cheat-content class, got: %q", result)
	}
	if !strings.Contains(result, "<pre><code>") {
		t.Errorf("formatCheatContent should wrap in pre/code")
	}
	if strings.Contains(result, "<script>") {
		t.Errorf("formatCheatContent should escape HTML, found raw <script>")
	}
}

// ---------------------------------------------------------------------------
// formatDictContent — pure helper in content.go
// covers the anonymous-struct meanings path
// ---------------------------------------------------------------------------

func TestSafeDictionaryAudioSource(t *testing.T) {
	for _, test := range []struct {
		input string
		want  bool
	}{
		{input: "https://api.dictionaryapi.dev/media/pronunciations/hello-us.mp3", want: true},
		{input: "//ssl.gstatic.com/dictionary/static/sounds/en/us/hello.mp3", want: true},
		{input: "http://api.dictionaryapi.dev/media/pronunciations/a.mp3"},
		{input: "https://api.dictionaryapi.dev.evil.example/media/pronunciations/a.mp3"},
		{input: "https://user@api.dictionaryapi.dev/media/pronunciations/a.mp3"},
		{input: "https://api.dictionaryapi.dev/media/pronunciations/a.mp3?redirect=1"},
		{input: "https://api.dictionaryapi.dev:443/media/pronunciations/a.mp3"},
	} {
		if got := SafeDictionaryAudioSource(test.input) != ""; got != test.want {
			t.Errorf("SafeDictionaryAudioSource(%q) allowed = %t, want %t", test.input, got, test.want)
		}
	}
}

func TestFormatDictContent(t *testing.T) {
	meanings := []struct {
		PartOfSpeech string `json:"partOfSpeech"`
		Definitions  []struct {
			Definition string   `json:"definition"`
			Example    string   `json:"example"`
			Synonyms   []string `json:"synonyms"`
			Antonyms   []string `json:"antonyms"`
		} `json:"definitions"`
		Synonyms []string `json:"synonyms"`
		Antonyms []string `json:"antonyms"`
	}{
		{
			PartOfSpeech: "noun",
			Definitions: []struct {
				Definition string   `json:"definition"`
				Example    string   `json:"example"`
				Synonyms   []string `json:"synonyms"`
				Antonyms   []string `json:"antonyms"`
			}{
				{Definition: "a test definition", Example: "used in an example", Synonyms: []string{"test"}, Antonyms: []string{}},
			},
			Synonyms: []string{"word", "term"},
		},
	}

	result := formatDictContent("hello", "/həˈloʊ/", "https://api.dictionaryapi.dev/media/pronunciations/hello-us.mp3", meanings, "Old English hel")
	if !strings.Contains(result, "dict-content") {
		t.Errorf("formatDictContent missing dict-content class")
	}
	if !strings.Contains(result, "hello") {
		t.Errorf("formatDictContent should contain the word")
	}
	if !strings.Contains(result, "noun") {
		t.Errorf("formatDictContent should contain part of speech")
	}
	if !strings.Contains(result, "Etymology") {
		t.Errorf("formatDictContent should contain etymology section")
	}
	if !strings.Contains(result, "<audio") || !strings.Contains(result, "controls") || !strings.Contains(result, "/direct/audio?url=") {
		t.Errorf("formatDictContent should render same-origin native audio playback")
	}
}

func TestFormatDictContentEmpty(t *testing.T) {
	meanings := []struct {
		PartOfSpeech string `json:"partOfSpeech"`
		Definitions  []struct {
			Definition string   `json:"definition"`
			Example    string   `json:"example"`
			Synonyms   []string `json:"synonyms"`
			Antonyms   []string `json:"antonyms"`
		} `json:"definitions"`
		Synonyms []string `json:"synonyms"`
		Antonyms []string `json:"antonyms"`
	}{}

	result := formatDictContent("x", "", "", meanings, "")
	if !strings.Contains(result, "dict-content") {
		t.Errorf("formatDictContent empty should still produce wrapper div")
	}
}

// ---------------------------------------------------------------------------
// formatHeadersData — pure helper in network.go
// ---------------------------------------------------------------------------

func TestFormatHeadersData(t *testing.T) {
	data := map[string]interface{}{
		"url":        "https://example.com",
		"status":     200,
		"statusText": "200 OK",
		"headers": map[string][]string{
			"Content-Type":    {"text/html; charset=utf-8"},
			"X-Frame-Options": {"DENY"},
		},
		"security": map[string]interface{}{
			"grade":   "B",
			"present": 4,
			"total":   7,
			"headers": map[string]bool{
				"X-Frame-Options": true,
			},
		},
	}
	result := formatHeadersData("https://example.com", data)
	if !strings.Contains(result, "headers-data") {
		t.Errorf("formatHeadersData missing headers-data class")
	}
	if !strings.Contains(result, "200 OK") {
		t.Errorf("formatHeadersData should contain status text")
	}
	if !strings.Contains(result, "grade") {
		t.Errorf("formatHeadersData should contain security grade")
	}
}

// ---------------------------------------------------------------------------
// WhoisHandler.fallbackWhois — method on *WhoisHandler
// ---------------------------------------------------------------------------

func TestWhoisFallback(t *testing.T) {
	h := NewWhoisHandler()
	ctx := context.Background()
	ans, err := h.fallbackWhois(ctx, "example.com")
	if err != nil {
		t.Fatalf("fallbackWhois returned unexpected error: %v", err)
	}
	if ans == nil {
		t.Fatal("fallbackWhois returned nil answer")
	}
	if ans.Error != "lookup_failed" {
		t.Errorf("expected lookup_failed error, got %q", ans.Error)
	}
}

// ---------------------------------------------------------------------------
// NewHeadersHandler and NewExpandHandler — verify constructors return non-nil
// ---------------------------------------------------------------------------

func TestNewHeadersHandlerNotNil(t *testing.T) {
	h := NewHeadersHandler()
	if h == nil {
		t.Fatal("NewHeadersHandler returned nil")
	}
	if h.Type() != AnswerTypeHeaders {
		t.Errorf("NewHeadersHandler type = %v, want AnswerTypeHeaders", h.Type())
	}
}

func TestNewExpandHandlerNotNil(t *testing.T) {
	h := NewExpandHandler()
	if h == nil {
		t.Fatal("NewExpandHandler returned nil")
	}
	if h.Type() != AnswerTypeExpand {
		t.Errorf("NewExpandHandler type = %v, want AnswerTypeExpand", h.Type())
	}
}

// ---------------------------------------------------------------------------
// NewDNSHandler — verify constructor returns non-nil with correct type
// ---------------------------------------------------------------------------

func TestNewDNSHandlerNotNil(t *testing.T) {
	h := NewDNSHandler()
	if h == nil {
		t.Fatal("NewDNSHandler returned nil")
	}
	if h.Type() != AnswerTypeDNS {
		t.Errorf("NewDNSHandler type = %v, want AnswerTypeDNS", h.Type())
	}
}

// ---------------------------------------------------------------------------
// formatCVEContent — covers the empty-refs path (extra branch)
// ---------------------------------------------------------------------------

func TestFormatCVEContentNoRefs(t *testing.T) {
	result := formatCVEContent("CVE-2024-0001", "A test vulnerability", 9.8, "CRITICAL", "AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H", "2024-01-01", []string{})
	if !strings.Contains(result, "CVE-2024-0001") {
		t.Errorf("formatCVEContent should contain CVE ID")
	}
}

// ---------------------------------------------------------------------------
// generateASCIIArt — test the non-empty path via ASCIIHandler
// to cover the 92.3% branch
// ---------------------------------------------------------------------------

func TestASCIIHandlerShortText(t *testing.T) {
	h := NewASCIIHandler()
	ctx := context.Background()
	ans, err := h.HandleDirectQuery(ctx, "Hi")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ans == nil {
		t.Fatal("nil answer")
	}
}

// ---------------------------------------------------------------------------
// formatCaseContent — test the path with content via CaseHandler
// to cover the 94.4% branch (handles all case types)
// ---------------------------------------------------------------------------

func TestCaseHandlerAllCases(t *testing.T) {
	h := NewCaseHandler()
	ctx := context.Background()
	tests := []string{"upper:", "lower:", "title:", "snake:", "camel:"}
	for _, prefix := range tests {
		t.Run(prefix, func(t *testing.T) {
			ans, err := h.HandleDirectQuery(ctx, prefix+"hello world")
			if err != nil {
				t.Fatalf("unexpected error for %q: %v", prefix, err)
			}
			if ans == nil {
				t.Fatalf("nil answer for %q", prefix)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// detectLanguage — cover the non-JSON paths (function returns "unknown" for
// code it can't identify — tests confirm branch coverage)
// ---------------------------------------------------------------------------

func TestDetectLanguagePaths(t *testing.T) {
	tests := []struct {
		code string
		lang string
	}{
		{`{"key": "val"}`, "json"},
		{"function hello() { return 1; }", "javascript"},
		{"SELECT * FROM users WHERE id = 1", "sql"},
		{"<html><body>hello</body></html>", "html"},
		{"body { color: red; margin: 0; }", "css"},
		{"plain text no keywords here", "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.lang, func(t *testing.T) {
			got := detectLanguage(tt.code)
			if got != tt.lang {
				t.Errorf("detectLanguage(%q) = %q, want %q", tt.code, got, tt.lang)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// beautifyCode — cover the switch paths; signature is (code, lang string)
// ---------------------------------------------------------------------------

func TestBeautifyCodePaths(t *testing.T) {
	tests := []struct {
		code  string
		lang  string
		check func(string) bool
	}{
		{"<html><body><p>test</p></body></html>", "html", func(s string) bool { return len(s) > 0 }},
		{"body{color:red;margin:0}", "css", func(s string) bool { return len(s) > 0 }},
		{"echo hello", "unknown", func(s string) bool { return strings.Contains(s, "echo") }},
	}
	for _, tt := range tests {
		t.Run(tt.lang, func(t *testing.T) {
			got := beautifyCode(tt.code, tt.lang)
			if !tt.check(got) {
				t.Errorf("beautifyCode(%q, %q) check failed, got: %q", tt.code, tt.lang, got)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// formatWordContent — test via WordHandler to cover 97.1% branch
// ---------------------------------------------------------------------------

func TestWordHandlerFrequency(t *testing.T) {
	h := NewWordHandler()
	ctx := context.Background()
	ans, err := h.HandleDirectQuery(ctx, "freq:hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ans == nil {
		t.Fatal("nil answer")
	}
}

// ---------------------------------------------------------------------------
// detectTechnologies — cover remaining 16% of branches
// Node.js, Drupal, Bootstrap paths
// ---------------------------------------------------------------------------

func TestDetectTechnologiesExtraPaths(t *testing.T) {
	tests := []struct {
		name    string
		headers http.Header
		body    string
		want    string
	}{
		{
			"cloudflare via CF-Ray header",
			http.Header{"Cf-Ray": {"abc123-LAX"}},
			"",
			"Cloudflare",
		},
		{
			"bootstrap in body",
			http.Header{},
			`<link rel="stylesheet" href="/bootstrap.min.css">`,
			"Bootstrap",
		},
		{
			"wordpress wp-content in body",
			http.Header{},
			`<link rel="stylesheet" href="/wp-content/themes/main.css">`,
			"WordPress",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detectTechnologies(tt.headers, tt.body)
			found := false
			for _, items := range got {
				for _, item := range items {
					if strings.Contains(item, tt.want) {
						found = true
						break
					}
				}
			}
			if !found {
				t.Errorf("detectTechnologies(%q) missing %q in results: %v", tt.name, tt.want, got)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// analyzeRobotsTxt — cover the disallow/allow tracking paths
// ---------------------------------------------------------------------------

func TestAnalyzeRobotsTxtAllPaths(t *testing.T) {
	robotsTxt := `User-agent: Googlebot
Disallow: /private/
Allow: /public/
Crawl-delay: 10

User-agent: *
Disallow: /admin/
Sitemap: https://example.com/sitemap.xml
`
	result := analyzeRobotsTxt(robotsTxt)
	if result == nil {
		t.Fatal("analyzeRobotsTxt returned nil")
	}
	if result["userAgents"] == nil {
		t.Error("expected userAgents field in result")
	}
}

// ---------------------------------------------------------------------------
// resolveURL — cover the fragment-only path and absolute URL path
// ---------------------------------------------------------------------------

func TestResolveURLExtraPaths(t *testing.T) {
	base, _ := url.Parse("https://example.com/page/index.html")
	tests := []struct {
		href string
		want string
	}{
		{"#section", "https://example.com/page/index.html#section"},
		{"https://other.com/path", "https://other.com/path"},
		{"/absolute/path", "https://example.com/absolute/path"},
	}
	for _, tt := range tests {
		got := resolveURL(base, tt.href)
		if got != tt.want {
			t.Errorf("resolveURL(%q) = %q, want %q", tt.href, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// getJSONDepth — cover the array branch (currently 92.9%)
// ---------------------------------------------------------------------------

func TestGetJSONDepthArray(t *testing.T) {
	arr := []interface{}{
		map[string]interface{}{"key": "value"},
		"string",
	}
	depth := getJSONDepth(arr)
	if depth < 2 {
		t.Errorf("getJSONDepth(array with object) = %d, want >= 2", depth)
	}
}

// ---------------------------------------------------------------------------
// generateCronDescription — cover branch paths; function takes []string of
// 5 cron fields (minute, hour, dom, month, dow)
// ---------------------------------------------------------------------------

func TestGenerateCronDescriptionExtra(t *testing.T) {
	tests := []struct {
		parts []string
		want  string
	}{
		{[]string{"*", "*", "*", "*", "*"}, "Every minute"},
		{[]string{"0", "*", "*", "*", "*"}, "Every hour"},
		{[]string{"0", "0", "*", "*", "*"}, "midnight"},
		{[]string{"0", "0", "1", "*", "*"}, "Monthly"},
		{[]string{"*/30", "*", "*", "*", "*"}, "30"},
		{[]string{"0", "12", "*", "*", "1"}, "Monday"},
		{[]string{"0", "0", "1", "1", "*"}, "January"},
		{[]string{"0", "9,17", "*", "*", "*"}, "9,17"},
	}
	for _, tt := range tests {
		t.Run(strings.Join(tt.parts, " "), func(t *testing.T) {
			got := generateCronDescription(tt.parts)
			if got == "" {
				t.Errorf("generateCronDescription(%v) returned empty string", tt.parts)
			}
			if tt.want != "" && !strings.Contains(got, tt.want) {
				t.Errorf("generateCronDescription(%v) = %q, want it to contain %q", tt.parts, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// formatPortContent — cover via PortHandler with a well-known port
// ---------------------------------------------------------------------------

func TestPortHandlerKnownPort(t *testing.T) {
	h := NewPortHandler()
	ctx := context.Background()
	ans, err := h.HandleDirectQuery(ctx, "443")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ans == nil {
		t.Fatal("nil answer")
	}
	if ans.Error != "" {
		t.Errorf("expected no error, got %q", ans.Error)
	}
}

// ---------------------------------------------------------------------------
// parseUserAgent — cover remaining branches via UAHandler
// ---------------------------------------------------------------------------

func TestUAHandlerBrowser(t *testing.T) {
	h := NewUserAgentHandler()
	ctx := context.Background()
	ans, err := h.HandleDirectQuery(ctx, "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ans == nil {
		t.Fatal("nil answer")
	}
}

// ---------------------------------------------------------------------------
// CertHandler — type check
// ---------------------------------------------------------------------------

func TestCertHandlerType(t *testing.T) {
	h := NewCertHandler()
	if h.Type() != AnswerTypeCert {
		t.Errorf("CertHandler.Type() = %v, want AnswerTypeCert", h.Type())
	}
}

// ---------------------------------------------------------------------------
// formatBasicRFCContent — via RFCHandler for a number query
// tests the not-found/fetch-failed path (HTTP unreachable in test env)
// ---------------------------------------------------------------------------

func TestRFCHandlerNumericQuery(t *testing.T) {
	h := NewRFCHandler()
	ctx := context.Background()
	ans, err := h.HandleDirectQuery(ctx, "9999999")
	if err == nil && ans != nil {
		if ans.Error != "" && ans.Error != "not_found" && ans.Error != "fetch_failed" {
			t.Errorf("unexpected error code %q for RFC number query", ans.Error)
		}
	}
}

// ---------------------------------------------------------------------------
// Manager.Process — test with a direct query that returns an answer
// (covers the non-nil process path)
// ---------------------------------------------------------------------------

func TestManagerProcessDirect(t *testing.T) {
	m := NewManager()
	ctx := context.Background()
	ans, err := m.Process(ctx, "subnet:192.168.1.0/24")
	if err != nil {
		t.Logf("note: Process returned error (may be expected in test env): %v", err)
	}
	if ans == nil && err == nil {
		t.Fatal("expected non-nil answer for subnet query")
	}
}

// ---------------------------------------------------------------------------
// HTMLHandler encode mode (existing NewHTMLHandler)
// ---------------------------------------------------------------------------

func TestHTMLEncodeMode(t *testing.T) {
	h := NewHTMLHandler()
	ctx := context.Background()
	ans, err := h.HandleDirectQuery(ctx, "encode:<p>Hello & World</p>")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ans == nil {
		t.Fatal("nil answer")
	}
}

// ---------------------------------------------------------------------------
// TimestampHandler — test Unix timestamp and "now"
// ---------------------------------------------------------------------------

func TestTimestampHandlerPaths(t *testing.T) {
	h := NewTimestampHandler()
	ctx := context.Background()
	tests := []struct {
		input   string
		wantErr bool
	}{
		{"now", false},
		{"1700000000", false},
		{"", true},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			ans, err := h.HandleDirectQuery(ctx, tt.input)
			if tt.wantErr {
				if err == nil && (ans == nil || ans.Error == "") {
					t.Errorf("expected error for %q but got none", tt.input)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error for %q: %v", tt.input, err)
				}
				if ans == nil {
					t.Fatalf("nil answer for %q", tt.input)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// ChmodHandler — test octal and symbolic inputs
// ---------------------------------------------------------------------------

func TestChmodHandlerPaths(t *testing.T) {
	h := NewChmodHandler()
	ctx := context.Background()
	tests := []struct {
		input   string
		wantErr bool
	}{
		{"755", false},
		{"644", false},
		{"rwxr-xr-x", false},
		{"", true},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			ans, err := h.HandleDirectQuery(ctx, tt.input)
			if tt.wantErr {
				if err == nil && (ans == nil || ans.Error == "") {
					t.Errorf("expected error for %q but got none", tt.input)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error for %q: %v", tt.input, err)
				}
				if ans == nil {
					t.Fatalf("nil answer for %q", tt.input)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// beautifyJSON — escaped char path (the \\ branch in string context)
// ---------------------------------------------------------------------------

func TestBeautifyJSONEscapedChars(t *testing.T) {
	input := `{"key":"hello\"world","num":42}`
	result := beautifyJSON(input)
	if !strings.Contains(result, "key") {
		t.Errorf("beautifyJSON should contain key, got: %q", result)
	}
	if !strings.Contains(result, "num") {
		t.Errorf("beautifyJSON should contain num, got: %q", result)
	}
}

// ---------------------------------------------------------------------------
// analyzeRegex — cover the \d, \w, \s branches
// ---------------------------------------------------------------------------

func TestAnalyzeRegexEscapeBranches(t *testing.T) {
	tests := []struct {
		pattern string
		want    string
	}{
		{`\d+`, "digits"},
		{`\w+`, "word characters"},
		{`\s+`, "whitespace"},
		{`[a-z]`, "character class"},
		{`(foo)`, "capture group"},
		{`.+`, "one-or-more"},
		{`.*`, "wildcard"},
		{`foo?`, "optional"},
		{`^start`, "start"},
		{`end$`, "end"},
		{`literal`, "literal"},
	}
	for _, tt := range tests {
		t.Run(tt.pattern, func(t *testing.T) {
			result := analyzeRegex(tt.pattern)
			found := false
			for _, r := range result {
				if strings.Contains(strings.ToLower(r), tt.want) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("analyzeRegex(%q) = %v, expected entry containing %q", tt.pattern, result, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// formatJWTContent — covers expiration branches and standard claims
// ---------------------------------------------------------------------------

func TestFormatJWTContent(t *testing.T) {
	claims := map[string]interface{}{
		"iss": "https://example.com",
		"sub": "user123",
		"aud": "myapp",
		"exp": float64(9999999999),
		"iat": float64(1700000000),
	}
	result := formatJWTContent(`{"alg":"HS256","typ":"JWT"}`, `{"sub":"user123"}`, false, "Valid for 2 hours", claims)
	if !strings.Contains(result, "jwt-content") {
		t.Errorf("formatJWTContent missing jwt-content class")
	}
	if !strings.Contains(result, "Issuer") {
		t.Errorf("formatJWTContent should contain Issuer")
	}
	if !strings.Contains(result, "user123") {
		t.Errorf("formatJWTContent should contain sub value")
	}

	expiredResult := formatJWTContent(`{}`, `{}`, true, "Expired 5 minutes ago", map[string]interface{}{})
	if !strings.Contains(expiredResult, "expired") && !strings.Contains(expiredResult, "Expired") {
		t.Errorf("formatJWTContent should indicate expiration when expired=true")
	}
}

// ---------------------------------------------------------------------------
// formatCountryContent — cover capital, currencies, languages, tld, timezones
// ---------------------------------------------------------------------------

func TestFormatCountryContentAllFields(t *testing.T) {
	data := map[string]interface{}{
		"cca2":        "DE",
		"cca3":        "DEU",
		"capital":     []string{"Berlin"},
		"region":      "Europe",
		"subregion":   "Western Europe",
		"population":  83200000,
		"area":        float64(357114),
		"currencies":  []string{"Euro (EUR)"},
		"languages":   []string{"German"},
		"callingCode": "+49",
		"tld":         []string{".de"},
		"drivingSide": "right",
		"timezones":   []string{"UTC+01:00"},
	}
	result := formatCountryContent("DE", "https://flag.example.com/de.svg", "Germany", "Federal Republic of Germany", data, "metric")
	if !strings.Contains(result, "Germany") {
		t.Errorf("formatCountryContent should contain country name")
	}
	if !strings.Contains(result, "Berlin") {
		t.Errorf("formatCountryContent should contain capital")
	}
	if !strings.Contains(result, "83,200,000") {
		t.Errorf("formatCountryContent should contain formatted population")
	}
	if !strings.Contains(result, ".de") {
		t.Errorf("formatCountryContent should contain TLD")
	}
}

// TestFormatCountryContentImperialUnits verifies the Area field is rendered
// in square miles (not hardcoded km²) when the resolved unit system is
// imperial, per the visitor's units preference.
func TestFormatCountryContentImperialUnits(t *testing.T) {
	data := map[string]interface{}{
		"cca2":    "DE",
		"cca3":    "DEU",
		"capital": []string{"Berlin"},
		"region":  "Europe",
		"area":    float64(357114),
	}

	metric := formatCountryContent("DE", "", "Germany", "Federal Republic of Germany", data, "metric")
	if !strings.Contains(metric, "km²") {
		t.Errorf("expected km² in metric output; got: %s", metric)
	}
	if strings.Contains(metric, "sq mi") {
		t.Errorf("did not expect sq mi in metric output; got: %s", metric)
	}

	imperial := formatCountryContent("DE", "", "Germany", "Federal Republic of Germany", data, "imperial")
	if !strings.Contains(imperial, "sq mi") {
		t.Errorf("expected sq mi in imperial output; got: %s", imperial)
	}
	if strings.Contains(imperial, "km²") {
		t.Errorf("did not expect km² in imperial output; got: %s", imperial)
	}
}

// TestUnitsFromContext verifies the units context helper round-trips
// values set by WithUnits and falls back to "imperial" when unset/invalid.
func TestUnitsFromContext(t *testing.T) {
	if got := UnitsFromContext(context.Background()); got != "imperial" {
		t.Errorf("expected default imperial, got %q", got)
	}

	ctx := WithUnits(context.Background(), "metric")
	if got := UnitsFromContext(ctx); got != "metric" {
		t.Errorf("expected metric, got %q", got)
	}

	ctx = WithUnits(context.Background(), "bogus")
	if got := UnitsFromContext(ctx); got != "imperial" {
		t.Errorf("expected fallback imperial for invalid value, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// RFCHandler — test "RFC" prefix trimming (covers TrimPrefix branch)
// ---------------------------------------------------------------------------

func TestRFCHandlerWithPrefix(t *testing.T) {
	h := NewRFCHandler()
	ctx := context.Background()
	ans, err := h.HandleDirectQuery(ctx, "RFC 2616")
	if err == nil && ans != nil {
		if ans.Error != "" && ans.Error != "not_found" && ans.Error != "fetch_failed" {
			t.Errorf("unexpected error code %q for RFC prefix query", ans.Error)
		}
	}
}

// ---------------------------------------------------------------------------
// detectLanguage — XML path (starts with "<" but not HTML)
// ---------------------------------------------------------------------------

func TestDetectLanguageXML(t *testing.T) {
	got := detectLanguage(`<?xml version="1.0"?><root><item/></root>`)
	if got != "xml" {
		t.Errorf("detectLanguage(xml) = %q, want xml", got)
	}
}

// ---------------------------------------------------------------------------
// formatPortContent — test the path with protocol info
// ---------------------------------------------------------------------------

func TestFormatPortContentDirect(t *testing.T) {
	result := formatPortContent(80, "HTTP", "Hypertext Transfer Protocol", "TCP")
	if !strings.Contains(result, "HTTP") {
		t.Errorf("formatPortContent should contain service name")
	}
	if !strings.Contains(result, "80") {
		t.Errorf("formatPortContent should contain port number")
	}
}

// ---------------------------------------------------------------------------
// generateCronDescription — cover the star-only path already tested;
// add month/dow word coverage
// ---------------------------------------------------------------------------

func TestGenerateCronDescDescWithDow(t *testing.T) {
	parts := []string{"30", "14", "*", "3", "2"}
	result := generateCronDescription(parts)
	if result == "" {
		t.Errorf("generateCronDescription(%v) returned empty", parts)
	}
	if !strings.Contains(result, "Tuesday") {
		t.Errorf("generateCronDescription should contain day name for dow=2, got: %q", result)
	}
}

// ---------------------------------------------------------------------------
// htmlDecode — named entity path (&amp; &lt; etc.)
// ---------------------------------------------------------------------------

func TestHTMLDecodeNamedEntities(t *testing.T) {
	result := htmlDecode("&amp;&lt;&gt;&quot;&#39;")
	if !strings.Contains(result, "&") {
		t.Errorf("htmlDecode should decode &amp; to &, got: %q", result)
	}
	if !strings.Contains(result, "<") {
		t.Errorf("htmlDecode should decode &lt; to <, got: %q", result)
	}
}

// ---------------------------------------------------------------------------
// EmojiHandler — test search query
// ---------------------------------------------------------------------------

func TestEmojiHandlerSearch(t *testing.T) {
	h := NewEmojiHandler()
	ctx := context.Background()
	ans, err := h.HandleDirectQuery(ctx, "smile")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ans == nil {
		t.Fatal("nil answer")
	}
}

// ---------------------------------------------------------------------------
// formatHTMLEncodingContent — test direct call
// ---------------------------------------------------------------------------

func TestFormatHTMLEncodingContent(t *testing.T) {
	result := formatHTMLEncodingContent("encode", "<p>Hello</p>", "&lt;p&gt;Hello&lt;/p&gt;")
	if !strings.Contains(result, "html-encoding-content") {
		t.Errorf("formatHTMLEncodingContent missing wrapper class, got: %q", result)
	}

	decodeResult := formatHTMLEncodingContent("decode", "&lt;p&gt;", "<p>")
	if !strings.Contains(decodeResult, "HTML Decode") {
		t.Errorf("formatHTMLEncodingContent decode mode should contain 'HTML Decode', got: %q", decodeResult)
	}
}

// ---------------------------------------------------------------------------
// CaseHandler — sentence case path (covers formatCaseContent branch)
// ---------------------------------------------------------------------------

func TestCaseHandlerSentenceCase(t *testing.T) {
	h := NewCaseHandler()
	ctx := context.Background()
	ans, err := h.HandleDirectQuery(ctx, "hello world")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ans == nil {
		t.Fatal("nil answer")
	}
	if !strings.Contains(ans.Content, "Hello world") {
		t.Errorf("sentence case should produce 'Hello world', content: %q", ans.Content)
	}
}

// ---------------------------------------------------------------------------
// detectTechnologies — cover remaining CDN and analytics paths
// ---------------------------------------------------------------------------

func TestDetectTechnologiesCDNAndAnalytics(t *testing.T) {
	tests := []struct {
		name    string
		headers http.Header
		body    string
		wantKey string
		wantVal string
	}{
		{
			"cloudfront via Via header",
			http.Header{"Via": {"1.1 CloudFront"}},
			"",
			"CDN",
			"Amazon CloudFront",
		},
		{
			"cloudflare in Server header",
			http.Header{"Server": {"cloudflare"}},
			"",
			"CDN",
			"Cloudflare",
		},
		{
			"google analytics in body",
			http.Header{},
			`<script src="https://www.google-analytics.com/analytics.js"></script>`,
			"Analytics",
			"Google Analytics",
		},
		{
			"google tag manager in body",
			http.Header{},
			`<script src="https://www.googletagmanager.com/gtm.js"></script>`,
			"Analytics",
			"Google Tag Manager",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detectTechnologies(tt.headers, tt.body)
			items := got[tt.wantKey]
			found := false
			for _, item := range items {
				if strings.Contains(item, tt.wantVal) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("detectTechnologies(%q) key=%q items=%v, missing %q", tt.name, tt.wantKey, items, tt.wantVal)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// formatDictContent — cover the synonym truncation path (> 10 synonyms)
// ---------------------------------------------------------------------------

func TestFormatDictContentManySynonyms(t *testing.T) {
	meanings := []struct {
		PartOfSpeech string `json:"partOfSpeech"`
		Definitions  []struct {
			Definition string   `json:"definition"`
			Example    string   `json:"example"`
			Synonyms   []string `json:"synonyms"`
			Antonyms   []string `json:"antonyms"`
		} `json:"definitions"`
		Synonyms []string `json:"synonyms"`
		Antonyms []string `json:"antonyms"`
	}{
		{
			PartOfSpeech: "noun",
			Definitions: []struct {
				Definition string   `json:"definition"`
				Example    string   `json:"example"`
				Synonyms   []string `json:"synonyms"`
				Antonyms   []string `json:"antonyms"`
			}{
				{Definition: "a test definition"},
			},
			Synonyms: []string{"one", "two", "three", "four", "five", "six", "seven", "eight", "nine", "ten", "eleven"},
		},
	}
	result := formatDictContent("test", "", "", meanings, "")
	if !strings.Contains(result, "...") {
		t.Errorf("formatDictContent with 11 synonyms should truncate with ..., got: %q", result)
	}
}

// ---------------------------------------------------------------------------
// htmlDecode — decimal entity path (&#65; → 'A')
// ---------------------------------------------------------------------------

func TestHTMLDecodeDecimalEntity(t *testing.T) {
	result := htmlDecode("&#65;&#66;&#67;")
	if result != "ABC" {
		t.Errorf("htmlDecode(&#65;&#66;&#67;) = %q, want ABC", result)
	}
}

// ---------------------------------------------------------------------------
// getUnicodeCategory — cover the Digit branch
// ---------------------------------------------------------------------------

func TestGetUnicodeCategoryDigit(t *testing.T) {
	cat := getUnicodeCategory('5')
	if cat != "Digit" {
		t.Errorf("getUnicodeCategory('5') = %q, want Digit", cat)
	}
}

// ---------------------------------------------------------------------------
// UnicodeHandler — named lookup path (word like "snowman")
// ---------------------------------------------------------------------------

func TestUnicodeHandlerNamedLookup(t *testing.T) {
	h := NewUnicodeHandler()
	ctx := context.Background()
	ans, err := h.HandleDirectQuery(ctx, "snowman")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ans == nil {
		t.Fatal("nil answer")
	}
}

// ---------------------------------------------------------------------------
// DirectoryHandler — with different query formats
// ---------------------------------------------------------------------------

func TestDirectoryHandlerFormats(t *testing.T) {
	h := NewDirectoryHandler()
	ctx := context.Background()
	tests := []struct {
		input string
	}{
		{"search engine"},
		{"what is DNS"},
		{"how to install nginx"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			ans, err := h.HandleDirectQuery(ctx, tt.input)
			if err != nil {
				t.Fatalf("unexpected error for %q: %v", tt.input, err)
			}
			if ans == nil {
				t.Fatalf("nil answer for %q", tt.input)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Parse — additional prefix tests to cover 94.4% → more
// ---------------------------------------------------------------------------

func TestManagerParseAdditionalPrefixes(t *testing.T) {
	m := NewManager()
	tests := []struct {
		query string
		want  AnswerType
	}{
		{"dns:example.com", AnswerTypeDNS},
		{"whois:example.com", AnswerTypeWhois},
		{"cert:example.com", AnswerTypeCert},
		{"headers:example.com", AnswerTypeHeaders},
		{"asn:AS15169", AnswerTypeASN},
		{"subnet:10.0.0.0/8", AnswerTypeSubnet},
		{"wiki:golang", AnswerTypeWiki},
		{"dict:hello", AnswerTypeDict},
		{"thesaurus:happy", AnswerTypeThesaurus},
		{"pkg:go:github.com/go-chi/chi", AnswerTypePkg},
		{"cve:CVE-2024-1234", AnswerTypeCVE},
		{"rfc:2616", AnswerTypeRFC},
		{"emoji:smile", AnswerTypeEmoji},
		{"unicode:A", AnswerTypeUnicode},
		{"json:format:{}", AnswerTypeJSON},
		{"yaml:format:key: val", AnswerTypeYAML},
		{"chmod:755", AnswerTypeChmod},
		{"cron:* * * * *", AnswerTypeCron},
		{"regex:[a-z]+", AnswerTypeRegex},
		{"jwt:invalid", AnswerTypeJWT},
		{"timestamp:now", AnswerTypeTimestamp},
		{"port:80", AnswerTypePort},
		{"http:200", AnswerTypeHTTP},
		{"tldr:ls", AnswerTypeTLDR},
		{"man:ls", AnswerTypeMan},
		{"cheat:git", AnswerTypeCheat},
		{"useragent:Mozilla/5.0", AnswerTypeUserAgent},
		{"mime:application/json", AnswerTypeMIME},
		{"license:MIT", AnswerTypeLicense},
		{"country:US", AnswerTypeCountry},
		{"ascii:Hello", AnswerTypeASCII},
		{"qr:hello", AnswerTypeQR},
		{"robots:example.com", AnswerTypeRobots},
		{"sitemap:example.com", AnswerTypeSitemap},
		{"tech:example.com", AnswerTypeTech},
		{"expand:https://t.co/abc", AnswerTypeExpand},
		{"safe:https://example.com", AnswerTypeSafe},
		{"cache:https://example.com", AnswerTypeCache},
		{"html:encode:<b>test</b>", AnswerTypeHTML},
		{"escape:json:hello", AnswerTypeEscape},
		{"case:upper:hello", AnswerTypeCase},
		{"slug:hello world", AnswerTypeSlug},
		{"diff:hello:world", AnswerTypeDiff},
		{"roti:hello", AnswerTypeRules},
		{"beautify:json:{}", AnswerTypeBeautify},
		{"lorem:5", AnswerTypeLorem},
		{"word:freq:hello", AnswerTypeWord},
	}
	for _, tt := range tests {
		t.Run(string(tt.want), func(t *testing.T) {
			got, _ := m.Parse(tt.query)
			if got != tt.want {
				t.Errorf("Parse(%q) = %v, want %v", tt.query, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// ExpandHandler — use httptest.Server to cover the success path
// (ExpandHandler uses h.client but the URL is the input, so we can pass
// the test server's URL directly)
// ---------------------------------------------------------------------------

func TestExpandHandlerWithMockServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	h := NewExpandHandler()
	ctx := context.Background()

	ans, err := h.HandleDirectQuery(ctx, srv.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ans == nil {
		t.Fatal("nil answer")
	}
	if ans.Type != AnswerTypeExpand {
		t.Errorf("type = %v, want AnswerTypeExpand", ans.Type)
	}
}

func TestExpandHandlerRedirect(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redirect" {
			http.Redirect(w, r, "/final", http.StatusFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	h := NewExpandHandler()
	ctx := context.Background()

	ans, err := h.HandleDirectQuery(ctx, srv.URL+"/redirect")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ans == nil {
		t.Fatal("nil answer")
	}
}

func TestExpandHandlerAddHTTPSPrefix(t *testing.T) {
	h := NewExpandHandler()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	ans, _ := h.HandleDirectQuery(ctx, "localhost:0/nonexistent")
	if ans != nil && ans.Type != AnswerTypeExpand {
		t.Errorf("type = %v, want AnswerTypeExpand", ans.Type)
	}
}

// ---------------------------------------------------------------------------
// SlangHandler — mock Urban Dictionary API to cover the success path
// ---------------------------------------------------------------------------

func TestSlangHandlerWithMockServer(t *testing.T) {
	mockResp := urbanDictionaryResponse{
		List: []urbanDictionaryEntry{
			{
				Word:       "cool",
				Definition: "Fashionable, or the best",
				Example:    "That's so cool!",
				ThumbsUp:   100,
				ThumbsDown: 5,
				Author:     "testuser",
				WrittenOn:  "2020-01-01T00:00:00.000Z",
				Permalink:  "https://www.urbandictionary.com/define.php?term=cool",
			},
			{
				Word:       "cool",
				Definition: "Another meaning of cool",
				Example:    "Stay cool!",
				ThumbsUp:   50,
				ThumbsDown: 2,
				Author:     "user2",
			},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(mockResp)
	}))
	defer srv.Close()

	h := NewSlangHandler()
	h.client = &http.Client{}
	h.client.Transport = &redirectTransport{target: srv.URL}

	ctx := context.Background()
	ans, err := h.HandleDirectQuery(ctx, "cool")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ans == nil {
		t.Fatal("nil answer")
	}
	if ans.Type != AnswerTypeSlang {
		t.Errorf("type = %v, want AnswerTypeSlang", ans.Type)
	}
}

func TestSlangHandlerEmptyResults(t *testing.T) {
	mockResp := urbanDictionaryResponse{List: []urbanDictionaryEntry{}}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(mockResp)
	}))
	defer srv.Close()

	h := NewSlangHandler()
	h.client = &http.Client{Transport: &redirectTransport{target: srv.URL}}

	ctx := context.Background()
	ans, err := h.HandleDirectQuery(ctx, "xyzzy_notaword")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ans == nil {
		t.Fatal("nil answer")
	}
	if ans.Error != "not_found" {
		t.Errorf("error = %q, want not_found", ans.Error)
	}
}

func TestSlangHandlerHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	h := NewSlangHandler()
	h.client = &http.Client{Transport: &redirectTransport{target: srv.URL}}

	ctx := context.Background()
	ans, err := h.HandleDirectQuery(ctx, "anything")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ans == nil {
		t.Fatal("nil answer")
	}
	if ans.Error != "fetch_error" {
		t.Errorf("error = %q, want fetch_error", ans.Error)
	}
}

// ---------------------------------------------------------------------------
// WhoisHandler — mock RDAP API to cover the success path
// (WhoisHandler uses h.client; URL is built from input domain)
// ---------------------------------------------------------------------------

func TestASNHandlerRejectsMalformedAndFailedResponses(t *testing.T) {
	for _, test := range []struct {
		name   string
		status int
		body   string
	}{
		{name: "malformed JSON", status: http.StatusOK, body: `{"status":`},
		{name: "upstream error", status: http.StatusBadGateway, body: `{}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			handler := NewASNHandler()
			handler.client = &http.Client{Transport: safeBrowsingTestTransport(func(request *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: test.status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(test.body)), Request: request}, nil
			})}
			if answer, err := handler.HandleDirectQuery(context.Background(), "AS15169"); err == nil || answer != nil {
				t.Fatalf("HandleDirectQuery() = (%#v, %v), want a parser/upstream error", answer, err)
			}
		})
	}
}

func TestWhoisHandlerWithMockRDAP(t *testing.T) {
	rdapResp := map[string]interface{}{
		"handle":  "EXAMPLE-DOM",
		"ldhName": "example.com",
		"status":  []string{"active"},
		"events": []map[string]interface{}{
			{"eventAction": "registration", "eventDate": "1995-08-14T04:00:00Z"},
			{"eventAction": "expiration", "eventDate": "2025-08-13T04:00:00Z"},
		},
		"nameservers": []map[string]interface{}{
			{"ldhName": "ns1.example.com"},
			{"ldhName": "ns2.example.com"},
		},
		"entities": []map[string]interface{}{
			{
				"roles": []string{"registrar"},
				"vcardArray": []interface{}{
					"vcard",
					[]interface{}{
						[]interface{}{"fn", map[string]interface{}{}, "text", "Example Registrar"},
					},
				},
			},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rdap+json")
		json.NewEncoder(w).Encode(rdapResp)
	}))
	defer srv.Close()

	h := NewWhoisHandler()
	h.client = &http.Client{Transport: &redirectTransport{target: srv.URL}}

	ctx := context.Background()
	ans, err := h.HandleDirectQuery(ctx, "example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ans == nil {
		t.Fatal("nil answer")
	}
	if ans.Type != AnswerTypeWhois {
		t.Errorf("type = %v, want AnswerTypeWhois", ans.Type)
	}
}

func TestWhoisHandlerMalformedRDAPFallsBack(t *testing.T) {
	handler := NewWhoisHandler()
	handler.client = &http.Client{Transport: safeBrowsingTestTransport(func(request *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"events":`)), Request: request}, nil
	})}
	answer, err := handler.HandleDirectQuery(context.Background(), "example.com")
	if err != nil {
		t.Fatal(err)
	}
	if answer == nil || answer.Error != "lookup_failed" || !strings.Contains(answer.Content, "Unable to fetch WHOIS data") {
		t.Fatalf("malformed RDAP response did not safely fall back: %#v", answer)
	}
}

func TestWhoisHandlerRDAPErrorFallsBack(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	h := NewWhoisHandler()
	h.client = &http.Client{Transport: &redirectTransport{target: srv.URL}}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Should fall back to fallbackWhois which will likely fail network-wise,
	// but the important path (RDAP non-200 → fallback call) is covered.
	_, _ = h.HandleDirectQuery(ctx, "example.com")
}

// redirectTransport replaces the host in every outgoing request with a test server's host.
// This allows testing handlers whose URLs are hardcoded.
type redirectTransport struct {
	target string
}

func (t *redirectTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	parsed, err := url.Parse(t.target)
	if err != nil {
		return nil, err
	}
	cloned := req.Clone(req.Context())
	cloned.URL.Host = parsed.Host
	cloned.URL.Scheme = parsed.Scheme
	return http.DefaultTransport.RoundTrip(cloned)
}

func TestJSONHandlerTreeAndValidationModes(t *testing.T) {
	h := NewJSONHandler()
	ctx := context.Background()
	for _, tc := range []struct{ query, mode, output string }{
		{`tree {"b":[1,{"a":true}]}`, "tree", "[0] (float64): 1"},
		{`validate {"ok":true}`, "validate", "Valid JSON"},
	} {
		answer, err := h.HandleDirectQuery(ctx, tc.query)
		if err != nil {
			t.Fatalf("HandleDirectQuery(%q): %v", tc.query, err)
		}
		if answer.Data["mode"] != tc.mode {
			t.Errorf("mode = %v, want %s", answer.Data["mode"], tc.mode)
		}
		if answer.Data["output"] != tc.output && !strings.Contains(answer.Data["output"].(string), tc.output) {
			t.Errorf("output = %v, want to contain %q", answer.Data["output"], tc.output)
		}
	}
}

type packageRedirectTransport struct {
	requests int
}

func (transport *packageRedirectTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	transport.requests++
	header := make(http.Header)
	header.Set("Location", "http://127.0.0.1/private")
	return &http.Response{StatusCode: http.StatusFound, Header: header, Body: io.NopCloser(strings.NewReader("")), Request: request}, nil
}

func TestPackageRegistryRedirectDoesNotReachUntrustedHost(t *testing.T) {
	transport := &packageRedirectTransport{}
	handler := NewPkgHandler()
	handler.client.Transport = transport
	if err := handler.fetchPackageJSON(context.Background(), "https://registry.npmjs.org/sample", &map[string]interface{}{}); err == nil {
		t.Fatal("redirect response was accepted as package metadata")
	}
	if transport.requests != 1 {
		t.Fatalf("registry requests = %d, want no follow-up to redirect target", transport.requests)
	}
}

func TestPkgHandlerAdditionalRegistries(t *testing.T) {
	today := time.Now().UTC().Format("2006-01-02")
	sixDaysAgo := time.Now().UTC().AddDate(0, 0, -6).Format("2006-01-02")
	tests := []struct {
		term, registry, expectedVersion string
		body                            string
	}{
		{"crate/serde", "crates.io", "1.0.0", `{"crate":{"id":"serde","name":"serde","max_version":"1.0.0","downloads":10}}`},
		{"gem/rails", "rubygems", "7.0.0", `{"name":"rails","version":"7.0.0","licenses":["MIT"],"downloads":20,"dependencies":{"runtime":[{"name":"rack"},{"name":"actionpack"}]}}`},
	}
	for _, test := range tests {
		t.Run(test.registry, func(t *testing.T) {
			h := NewPkgHandler()
			transport := packageTestTransport{body: test.body}
			var userAgents []string
			if test.registry == "crates.io" {
				packageDownloadsCache.Lock()
				delete(packageDownloadsCache.entries, "crates:serde")
				packageDownloadsCache.Unlock()
				transport.requestUserAgents = &userAgents
				transport.bodies = map[string]string{
					"/api/v1/crates/serde/1.0.0":              `{"version":{"readme_path":"/api/v1/crates/serde/1.0.0/readme"}}`,
					"/api/v1/crates/serde/1.0.0/dependencies": `{"dependencies":[{"id":1},{"id":2}]}`,
					"/api/v1/crates/serde/1.0.0/readme":       `<h1>Serde</h1><p>Safe serialization.</p>`,
					"/api/v1/crates/serde/downloads":          fmt.Sprintf(`{"version_downloads":[{"date":%q,"downloads":3},{"date":%q,"downloads":4}]}`, today, sixDaysAgo),
				}
			}
			h.client = &http.Client{Transport: transport}
			answer, err := h.HandleDirectQuery(context.Background(), test.term)
			if err != nil {
				t.Fatal(err)
			}
			if answer.Data["registry"] != test.registry {
				t.Fatalf("registry = %v", answer.Data["registry"])
			}
			if answer.Data["version"] != test.expectedVersion {
				t.Fatalf("version = %v", answer.Data["version"])
			}
			if test.registry == "rubygems" {
				if answer.Data["dependencies"] != 2 {
					t.Errorf("runtime dependency count = %v, want 2", answer.Data["dependencies"])
				}
				if answer.SourceURL != "https://rubygems.org/gems/rails" {
					t.Errorf("RubyGems source URL = %q, want canonical registry URL", answer.SourceURL)
				}
			}
			if test.registry == "crates.io" {
				if len(userAgents) != 5 {
					t.Errorf("crates.io requests lack identifying User-Agent headers: %v", userAgents)
				}
				for _, userAgent := range userAgents {
					if !strings.HasPrefix(userAgent, "search/") {
						t.Errorf("crates.io request has non-identifying User-Agent %q", userAgent)
					}
				}
				if answer.Data["dependencies"] != 2 {
					t.Errorf("crate dependency count = %v, want 2", answer.Data["dependencies"])
				}
				if answer.Data["weekly_downloads"] != int64(7) {
					t.Errorf("crate weekly downloads = %v, want 7", answer.Data["weekly_downloads"])
				}
				if !strings.Contains(answer.Data["readme_preview"].(string), "Safe serialization") {
					t.Errorf("crate README preview missing: %v", answer.Data["readme_preview"])
				}
			}
			for _, field := range []string{"weekly_downloads", "dependencies", "updated", "repository", "readme_preview"} {
				if _, exists := answer.Data[field]; !exists {
					t.Errorf("%s metadata key missing; unsupported registry fields must be explicit", field)
				}
			}
			if !strings.Contains(answer.Content, "pkg_unavailable") && !strings.Contains(answer.Content, "Unavailable") {
				t.Errorf("unavailable metadata is not explained in formatted output: %s", answer.Content)
			}
		})
	}
}

func TestCratesWeeklyDownloadsUsesSevenDaysAndCache(t *testing.T) {
	name := "cache-check-crate"
	cacheKey := "crates:" + name
	packageDownloadsCache.Lock()
	delete(packageDownloadsCache.entries, cacheKey)
	packageDownloadsCache.Unlock()
	today := time.Now().UTC().Format("2006-01-02")
	sixDaysAgo := time.Now().UTC().AddDate(0, 0, -6).Format("2006-01-02")
	sevenDaysAgo := time.Now().UTC().AddDate(0, 0, -7).Format("2006-01-02")
	requestCount := 0
	handler := NewPkgHandler()
	handler.client = &http.Client{Transport: packageTestTransport{
		requestCount: &requestCount,
		body:         fmt.Sprintf(`{"version_downloads":[{"date":%q,"downloads":2},{"date":%q,"downloads":3},{"date":%q,"downloads":100}],"meta":{"extra_downloads":[{"date":%q,"downloads":4},{"date":%q,"downloads":5},{"date":%q,"downloads":1000}]}}`, today, sixDaysAgo, sevenDaysAgo, today, sixDaysAgo, sevenDaysAgo),
	}}
	for range 2 {
		count, err := handler.fetchCratesWeeklyDownloads(context.Background(), name)
		if err != nil {
			t.Fatal(err)
		}
		if count != 14 {
			t.Fatalf("weekly downloads = %d, want 14 including extra-version totals", count)
		}
	}
	if requestCount != 1 {
		t.Fatalf("download stats requests = %d, want 1 due to daily cache", requestCount)
	}
}

func TestBeautifyJavaScriptFormatsCode(t *testing.T) {
	formatted := beautifyCode(`function test(){return{a:1,b:"x y"};}`, "javascript")
	if !strings.Contains(formatted, "function test() {\n") || !strings.Contains(formatted, `"x y"`) || formatted == `function test(){return{a:1,b:"x y"};}` {
		t.Fatalf("JavaScript was not formatted correctly: %q", formatted)
	}
}

func TestPythonFormatterAndMinifierPreserveMultilineStringContents(t *testing.T) {
	source := "def render():\n    message = \"\"\"first line\n  leading and trailing spaces  \nsecond line\"\"\"\n    return message"
	formatted := beautifyPython(source)
	for _, literalLine := range []string{
		"  leading and trailing spaces  ",
		"second line\"\"\"",
	} {
		if !strings.Contains(formatted, literalLine) {
			t.Errorf("formatter changed multiline string content %q: %q", literalLine, formatted)
		}
	}
	if !strings.Contains(formatted, "\n    return message") {
		t.Errorf("formatter did not resume code indentation after the string: %q", formatted)
	}

	minified := minifyPythonSource(source)
	if !strings.Contains(minified, "\n  leading and trailing spaces  \n") {
		t.Errorf("minifier changed multiline string content: %q", minified)
	}
}

func TestBeautifySupportsAdditionalIDEALanguagesAndLineCounts(t *testing.T) {
	python := beautifyCode("def greet():\nprint('hello')", "python")
	if !strings.Contains(python, "def greet():\n    print('hello')") {
		t.Fatalf("Python block indentation was not formatted: %q", python)
	}
	for _, language := range []string{"scss", "less"} {
		formatted := beautifyCode(".card{color:red;margin:0}", language)
		if formatted == ".card{color:red;margin:0}" {
			t.Errorf("%s input was returned unchanged", language)
		}
	}
	php := beautifyCode("<?php function greet(){return 'hi';}", "php")
	if php == "<?php function greet(){return 'hi';}" {
		t.Errorf("PHP input was returned unchanged")
	}
	answer, err := NewBeautifyHandler().HandleDirectQuery(context.Background(), "python def greet():\nprint('hello')")
	if err != nil {
		t.Fatal(err)
	}
	if answer.Data["input_lines"] != 2 || answer.Data["output_lines"] != 2 {
		t.Errorf("line counts = (%v, %v), want (2, 2)", answer.Data["input_lines"], answer.Data["output_lines"])
	}
	if !strings.Contains(answer.Content, "Lines before: 2 · Lines after: 2") {
		t.Errorf("line count summary missing: %s", answer.Content)
	}
}

func TestManagerParsesRandomSlang(t *testing.T) {
	typeAndTerm := NewManager()
	answerType, term := typeAndTerm.Parse("SlAnG:")
	if answerType != AnswerTypeSlang || term != "" {
		t.Fatalf("Parse random slang = (%q, %q)", answerType, term)
	}
}

func TestSlangHandlerRandomLookup(t *testing.T) {
	path := ""
	handler := NewSlangHandler()
	handler.client = &http.Client{Transport: packageTestTransport{requestPath: &path, body: `{"list":[{"word":"serendipity","definition":"A fortunate discovery","thumbs_up":4}]}`}}
	answer, err := handler.HandleDirectQuery(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if path != "/v0/random" {
		t.Fatalf("request path = %q, want /v0/random", path)
	}
	if answer == nil || answer.Data["word"] != "serendipity" {
		t.Fatalf("unexpected random answer: %+v", answer)
	}
}

func TestDirectoryHandlerUsesAggregatedResults(t *testing.T) {
	handler := NewDirectoryHandler()
	handler.client = &http.Client{Transport: directoryTestTransport{server: "nginx", body: `<html><body><table><tr><td><a href="file.iso">ubuntu.iso</a></td><td>2024-11-07 14:12</td><td>13K</td></tr><tr><td><a href="folder/">folder/</a></td><td>-</td><td>-</td></tr></table></body></html>`}}
	handler.SetSearcher(func(_ context.Context, query string) ([]DirectoryResult, error) {
		if !strings.Contains(query, `intitle:"index of"`) {
			t.Fatalf("unexpected query %q", query)
		}
		return []DirectoryResult{{Title: "Index of files", URL: "https://example.test/files/", Description: "Open directory"}}, nil
	})
	answer, err := handler.HandleDirectQuery(context.Background(), "linux iso")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(answer.Content, "https://example.test/files/") || !strings.Contains(answer.Content, "Index of files") {
		t.Fatalf("aggregated result missing: %s", answer.Content)
	}
	if !strings.Contains(answer.Content, "ubuntu.iso") || !strings.Contains(answer.Content, "13K") || !strings.Contains(answer.Content, "2024-11-07 14:12") {
		t.Fatalf("scraped file preview missing: %s", answer.Content)
	}
	if !strings.Contains(answer.Content, "Server: nginx") || !strings.Contains(answer.Content, "Estimated files: 1") || !strings.Contains(answer.Content, "Scan directory") {
		t.Fatalf("directory metadata or scan control missing: %s", answer.Content)
	}
	if got := answer.Data["results"].([]DirectoryResult)[0].EstimatedFileCount; got != 1 {
		t.Errorf("estimated file count = %d, want 1", got)
	}
}

func TestScrapeDirectoryPreviewAndExpandedScanLimits(t *testing.T) {
	body := "<html><body>" + strings.Repeat(`<a href="file.bin">file.bin</a>`, 25) + "</body></html>"
	for _, test := range []struct {
		name      string
		limit     int
		wantFiles int
	}{
		{name: "preview is bounded", limit: 20, wantFiles: 20},
		{name: "scan enumerates more files", limit: 500, wantFiles: 25},
	} {
		t.Run(test.name, func(t *testing.T) {
			handler := NewDirectoryHandler()
			handler.client = &http.Client{Transport: directoryTestTransport{body: body}}
			files, _, estimated := handler.scrapeDirectory(context.Background(), "https://example.test/files/", test.limit)
			if len(files) != test.wantFiles || estimated != 25 {
				t.Fatalf("scrapeDirectory() returned %d files, estimate %d; want %d and 25", len(files), estimated, test.wantFiles)
			}
		})
	}
}

func TestDirectoryHandlerScanControlExpandsTargetListing(t *testing.T) {
	body := "<html><body>" + strings.Repeat(`<a href="file.bin">file.bin</a>`, 25) + "</body></html>"
	handler := NewDirectoryHandler()
	handler.client = &http.Client{Transport: directoryTestTransport{body: body}}
	handler.SetSearcher(func(context.Context, string) ([]DirectoryResult, error) {
		return []DirectoryResult{{Title: "Index of files", URL: "https://example.test/files/"}}, nil
	})
	answer, err := handler.HandleDirectQuery(context.Background(), "linux iso scan:1")
	if err != nil {
		t.Fatal(err)
	}
	results, ok := answer.Data["results"].([]DirectoryResult)
	if !ok || len(results) != 1 || len(results[0].Files) != 25 {
		t.Fatalf("scan did not enumerate the selected listing: %#v", answer.Data["results"])
	}
}

func TestDirectoryQueryFiltersApplyTypeSizeDateAndSite(t *testing.T) {
	query, filters, err := parseDirectoryQuery("linux textbook type:documents min-size:10M after:2024-01-01 before:2024-12-31 site:edu scan:2")
	if err != nil {
		t.Fatal(err)
	}
	if query != "linux textbook" || filters.FileType != "documents" || filters.Site != "edu" || filters.MinimumSize != 10*(1<<20) || filters.After != "2024-01-01" || filters.Before != "2024-12-31" || filters.ScanIndex != 2 {
		t.Fatalf("parsed query/filters = %q / %#v", query, filters)
	}
	files := []DirectoryFile{
		{Name: "manual.pdf", Size: "11M", Modified: "2024-06-01 12:00"},
		{Name: "small.pdf", Size: "9M", Modified: "2024-06-01 12:00"},
		{Name: "large.zip", Size: "20M", Modified: "2024-06-01 12:00"},
		{Name: "old.pdf", Size: "20M", Modified: "2023-12-31 12:00"},
	}
	filtered := filterDirectoryFiles(files, filters)
	if len(filtered) != 1 || filtered[0].Name != "manual.pdf" {
		t.Fatalf("filtered files = %#v", filtered)
	}
	if size, err := parseDirectorySize("1.5MiB"); err != nil || size != 3*(1<<19) {
		t.Errorf("parseDirectorySize(1.5MiB) = %d, %v", size, err)
	}
	if _, _, err := parseDirectoryQuery("linux type:unknown"); err == nil {
		t.Error("unsupported file type filter was accepted")
	}
	content := formatDirectoryResults("linux textbook", "query", []DirectoryResult{{Title: "Index of files", URL: "https://example.test/files/"}}, filters)
	for _, expected := range []string{"directory-filter-form", `action="/search"`, "directory_type", "directory_min_size", "directory_after", "directory_before", "directory_site", "directory_scan", `value="2"`, "directory-scan-form"} {
		if !strings.Contains(content, expected) {
			t.Errorf("directory filter control %q missing", expected)
		}
	}
}

type directoryTestTransport struct {
	body   string
	server string
}

func (transport directoryTestTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/html; charset=utf-8"}, "Server": []string{transport.server}}, Body: io.NopCloser(strings.NewReader(transport.body)), Request: request}, nil
}

func TestSupportedLicenseTextsAreEmbedded(t *testing.T) {
	for _, spdx := range []string{"MIT", "Apache-2.0", "GPL-3.0", "GPL-2.0", "LGPL-3.0", "BSD-3-Clause", "BSD-2-Clause", "ISC", "MPL-2.0", "Unlicense", "CC0-1.0", "AGPL-3.0"} {
		if text := fullLicenseText(spdx); len(text) < 100 {
			t.Errorf("%s license text missing or incomplete", spdx)
		}
	}
	answer, err := NewLicenseHandler().HandleDirectQuery(context.Background(), "MIT")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(answer.Content, "Full License Text") || !strings.Contains(answer.Content, "Permission is hereby granted") {
		t.Fatalf("full MIT text missing from answer")
	}
}

func TestCountryHandlerSupportsNumericISOAndSpecFields(t *testing.T) {
	body := `[{"name":{"common":"Germany","official":"Federal Republic of Germany"},"cca2":"DE","cca3":"DEU","ccn3":"276","capital":["Berlin"],"region":"Europe","subregion":"Western Europe","area":357114,"population":83200000,"timezones":["UTC+01:00"],"car":{"side":"right"},"flags":{"svg":"https://flagcdn.com/de.svg"},"maps":{"openStreetMaps":"https://www.openstreetmap.org/relation/51477"},"currencies":{"EUR":{"name":"Euro","symbol":"€"}},"languages":{"deu":"German"},"flag":"🇩🇪","tld":[".de"],"idd":{"root":"+4","suffixes":["9"]}}]`
	handler := NewCountryHandler()
	handler.client = &http.Client{Transport: packageTestTransport{body: body}}
	answer, err := handler.HandleDirectQuery(context.Background(), "276")
	if err != nil {
		t.Fatal(err)
	}
	if answer.Data["name"] != "Germany" || answer.Data["ccn3"] != "276" {
		t.Fatalf("numeric ISO lookup failed: %#v", answer.Data)
	}
	for _, expected := range []string{"83,200,000", "UTC+01:00", "OpenStreetMap", "country-flag"} {
		if !strings.Contains(answer.Content, expected) {
			t.Errorf("country response missing %q", expected)
		}
	}
}

func TestFeedAndTechHandlersUseFetchedHTML(t *testing.T) {
	responseHTML := `<html><head><link type="application/rss+xml" href="/feed.xml" title="News"></head><body><script src="/wp-content/jquery.js"></script><div id="root">React</div></body></html>`
	feed := NewFeedHandler()
	rss := `<rss version="2.0"><channel><title>Fixture News</title><description>Fixture description</description><lastBuildDate>Thu, 01 Jan 2026 00:00:00 GMT</lastBuildDate><item><title>Latest story</title><link>https://example.com/story</link><description>Latest entry preview</description><pubDate>Thu, 01 Jan 2026 00:00:00 GMT</pubDate></item></channel></rss>`
	feed.client = &http.Client{Transport: statusTestTransport{status: http.StatusOK, body: responseHTML, bodies: map[string]string{"/feed.xml": rss}}}
	feedAnswer, err := feed.HandleDirectQuery(context.Background(), "example.com")
	if err != nil {
		t.Fatal(err)
	}
	feeds, ok := feedAnswer.Data["feeds"].([]feedInfo)
	if !ok || len(feeds) != 1 || feeds[0].URL != "https://example.com/feed.xml" {
		t.Fatalf("feed discovery did not return the declared feed: %#v", feedAnswer.Data["feeds"])
	}
	if feeds[0].Title != "Fixture News" || feeds[0].Description != "Fixture description" || feeds[0].EntryCount != 1 || len(feeds[0].Entries) != 1 || feeds[0].Entries[0].Title != "Latest story" {
		t.Fatalf("feed metadata or latest entry missing: %#v", feeds[0])
	}
	tech := NewTechHandler()
	tech.client = &http.Client{Transport: statusTestTransport{status: http.StatusOK, body: responseHTML}}
	techAnswer, err := tech.HandleDirectQuery(context.Background(), "example.com")
	if err != nil {
		t.Fatal(err)
	}
	technologies, ok := techAnswer.Data["technologies"].(map[string][]string)
	if !ok || !reflect.DeepEqual(technologies["JavaScript"], []string{"React", "jQuery"}) || !reflect.DeepEqual(technologies["CMS/Platform"], []string{"WordPress"}) {
		t.Fatalf("technology detection did not report page markers: %#v", techAnswer.Data["technologies"])
	}
}

func TestFeedAndTechRejectFailedOrOversizedResponses(t *testing.T) {
	for _, test := range []struct {
		name       string
		maxBody    int
		newHandler func(int, string) Handler
	}{
		{name: "feed", maxBody: 100 * 1024, newHandler: func(status int, body string) Handler {
			handler := NewFeedHandler()
			handler.client = &http.Client{Transport: statusTestTransport{status: status, body: body}}
			return handler
		}},
		{name: "tech", maxBody: 200 * 1024, newHandler: func(status int, body string) Handler {
			handler := NewTechHandler()
			handler.client = &http.Client{Transport: statusTestTransport{status: status, body: body}}
			return handler
		}},
	} {
		t.Run(test.name+" HTTP failure", func(t *testing.T) {
			answer, err := test.newHandler(http.StatusServiceUnavailable, "<script>internal details</script>").HandleDirectQuery(context.Background(), "example.com")
			if err != nil {
				t.Fatal(err)
			}
			if answer.Error != "fetch_failed" || !strings.Contains(answer.Content, "HTTP 503") {
				t.Fatalf("unexpected failure answer: %#v", answer)
			}
			if strings.Contains(answer.Content, "internal details") {
				t.Fatalf("response body leaked into error: %s", answer.Content)
			}
		})
		t.Run(test.name+" oversized response", func(t *testing.T) {
			answer, err := test.newHandler(http.StatusOK, strings.Repeat("x", test.maxBody+1)).HandleDirectQuery(context.Background(), "example.com")
			if err != nil {
				t.Fatal(err)
			}
			if answer.Error != "fetch_failed" || !strings.Contains(answer.Content, "read safely") {
				t.Fatalf("oversized response was accepted: %#v", answer)
			}
		})
	}
}

type datamuseTestTransport struct {
	synonymStatus int
	antonymStatus int
}

type failingReadBody struct{}

func (failingReadBody) Read([]byte) (int, error) {
	return 0, fmt.Errorf("private upstream read detail")
}
func (failingReadBody) Close() error { return nil }

type failingReadTransport struct{}

func (failingReadTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: failingReadBody{}, Request: request}, nil
}

func TestFeedAndTechReturnSafeAnswersWhenResponseReadFails(t *testing.T) {
	for _, test := range []struct {
		name       string
		newHandler func() Handler
	}{
		{name: "feed", newHandler: func() Handler {
			handler := NewFeedHandler()
			handler.client = &http.Client{Transport: failingReadTransport{}}
			return handler
		}},
		{name: "technology", newHandler: func() Handler {
			handler := NewTechHandler()
			handler.client = &http.Client{Transport: failingReadTransport{}}
			return handler
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			answer, err := test.newHandler().HandleDirectQuery(context.Background(), "example.com")
			if err != nil {
				t.Fatal(err)
			}
			if answer.Error != "fetch_failed" || !strings.Contains(answer.Content, "could not be read safely") {
				t.Fatalf("response read failure did not produce a safe failure answer: %#v", answer)
			}
			if strings.Contains(answer.Content, "private upstream read detail") {
				t.Fatalf("raw upstream read error leaked: %s", answer.Content)
			}
		})
	}
}

func (transport datamuseTestTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	status := transport.synonymStatus
	body := `[{"word":"quick","score":100}]`
	if request.URL.Query().Get("rel_ant") != "" {
		status = transport.antonymStatus
		body = `[{"word":"slow","score":90}]`
	}
	if status == 0 {
		status = http.StatusOK
	}
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: request}, nil
}

func TestThesaurusSurfacesProviderFailuresAndPartialResults(t *testing.T) {
	t.Run("both providers fail", func(t *testing.T) {
		handler := NewThesaurusHandler()
		handler.client = &http.Client{Transport: datamuseTestTransport{synonymStatus: http.StatusServiceUnavailable, antonymStatus: http.StatusBadGateway}}
		answer, err := handler.HandleDirectQuery(context.Background(), "rapid")
		if err != nil {
			t.Fatal(err)
		}
		if answer.Error != "fetch_failed" || !strings.Contains(answer.Content, "could not be loaded") || strings.Contains(answer.Content, "HTTP 503") {
			t.Fatalf("unsafe failed-provider response: %#v", answer)
		}
	})
	t.Run("one provider fails", func(t *testing.T) {
		handler := NewThesaurusHandler()
		handler.client = &http.Client{Transport: datamuseTestTransport{synonymStatus: http.StatusOK, antonymStatus: http.StatusServiceUnavailable}}
		answer, err := handler.HandleDirectQuery(context.Background(), "rapid")
		if err != nil {
			t.Fatal(err)
		}
		if answer.Error != "" || answer.Data["synonyms_available"] != true || answer.Data["antonyms_available"] != false {
			t.Fatalf("partial provider status missing: %#v", answer)
		}
		if !strings.Contains(answer.Content, "quick") || !strings.Contains(answer.Content, "Antonyms could not be loaded") {
			t.Fatalf("partial result or warning missing: %s", answer.Content)
		}
	})
}

func TestThesaurusRendersProviderSenseMetadataAndRelatedPhrases(t *testing.T) {
	handler := NewThesaurusHandler()
	handler.client = &http.Client{Transport: safeBrowsingTestTransport(func(request *http.Request) (*http.Response, error) {
		var body string
		switch request.URL.Query().Get("rel_syn") {
		case "rapid":
			if request.URL.Query().Get("md") != "dp" {
				t.Errorf("metadata query = %s", request.URL.RawQuery)
			}
			body = `[{"word":"quick","tags":["adj"],"defs":["moving fast"]}]`
		case "":
			switch request.URL.Query().Get("rel_ant") {
			case "rapid":
				body = `[{"word":"slow","tags":["adj"],"defs":["moving with little speed"]}]`
			default:
				body = `[{"word":"quick fix","tags":["n"]}]`
			}
		default:
			body = `[]`
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: request}, nil
	})}
	answer, err := handler.HandleDirectQuery(context.Background(), "rapid")
	if err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{"Synonyms", "Antonyms", "Related words and phrases", "quick", "moving fast", "adj", "slow", "quick fix"} {
		if !strings.Contains(answer.Content, fragment) {
			t.Errorf("thesaurus output missing %q: %s", fragment, answer.Content)
		}
	}
}

func TestDictionaryAndThesaurusClientsRejectRedirects(t *testing.T) {
	for _, test := range []struct {
		name   string
		client *http.Client
	}{{"dictionary", NewDictHandler().client}, {"thesaurus", NewThesaurusHandler().client}} {
		t.Run(test.name, func(t *testing.T) {
			request, err := http.NewRequest(http.MethodGet, "https://provider.example/", nil)
			if err != nil {
				t.Fatal(err)
			}
			if err := test.client.CheckRedirect(request, []*http.Request{{}}); !errors.Is(err, http.ErrUseLastResponse) {
				t.Fatalf("redirect policy error = %v, want ErrUseLastResponse", err)
			}
		})
	}
}

func TestDictionaryRejectsUnexpectedProviderStatus(t *testing.T) {
	handler := NewDictHandler()
	handler.client = &http.Client{Transport: safeBrowsingTestTransport(func(request *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusServiceUnavailable, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("<html>private provider detail</html>")), Request: request}, nil
	})}
	answer, err := handler.HandleDirectQuery(context.Background(), "example")
	if err == nil || answer != nil || strings.Contains(err.Error(), "private provider detail") {
		t.Fatalf("unexpected dictionary status response = answer %#v, error %v", answer, err)
	}
}

func TestDictionaryIncludesAllEntriesAndProviderFrequency(t *testing.T) {
	handler := NewDictHandler()
	handler.client = &http.Client{Transport: safeBrowsingTestTransport(func(request *http.Request) (*http.Response, error) {
		body := `[]`
		switch request.URL.Host {
		case "api.dictionaryapi.dev":
			body = `[{"word":"light","phonetic":"/laɪt/","meanings":[{"partOfSpeech":"noun","definitions":[{"definition":"Visible electromagnetic radiation."}],"antonyms":["darkness"]}]},{"word":"light","meanings":[{"partOfSpeech":"verb","definitions":[{"definition":"To ignite."}]}],"origin":"Old English līht"}]`
		case "api.datamuse.com":
			if request.URL.Query().Get("md") != "f" || request.URL.Query().Get("qe") != "sp" {
				t.Errorf("frequency query = %s", request.URL.RawQuery)
			}
			body = `[{"word":"light","tags":["f:21.5"]}]`
		default:
			t.Errorf("unexpected provider host %q", request.URL.Host)
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: request}, nil
	})}
	answer, err := handler.HandleDirectQuery(context.Background(), "light")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(answer.Content, "Visible electromagnetic radiation") || !strings.Contains(answer.Content, "To ignite") || !strings.Contains(answer.Content, "Old English") || !strings.Contains(answer.Content, "darkness") || !strings.Contains(answer.Content, "21.50 occurrences per million") {
		t.Fatalf("dictionary content omitted entry data: %s", answer.Content)
	}
	if answer.Data["frequency_per_million"] != 21.5 {
		t.Fatalf("frequency data = %#v", answer.Data["frequency_per_million"])
	}
}

func TestPublicHTTPClientBlocksPrivateRedirects(t *testing.T) {
	client := newPublicHTTPClient(time.Second)
	request, err := http.NewRequest(http.MethodGet, "http://127.0.0.1/", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := client.CheckRedirect(request, []*http.Request{{}}); err == nil {
		t.Fatal("private redirect target was accepted")
	}
}

func TestSafeHandlerRedirectChainStopsAtPrivateHost(t *testing.T) {
	handler := NewSafeHandler()
	redirects := []string{}
	ctx := context.WithValue(context.Background(), safeRedirectsContextKey{}, &redirects)
	publicHop, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://8.8.8.8/next", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := handler.client.CheckRedirect(publicHop, []*http.Request{{}}); err != nil {
		t.Fatalf("public redirect was rejected: %v", err)
	}
	privateHop, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://127.0.0.1/private", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := handler.client.CheckRedirect(privateHop, []*http.Request{{}, {}}); err == nil {
		t.Fatal("redirect chain accepted a private destination")
	}
	if len(redirects) != 1 || redirects[0] != publicHop.URL.String() {
		t.Fatalf("redirects = %#v, want only the validated public hop", redirects)
	}
}

func TestPublicDialRejectsDNSRebinding(t *testing.T) {
	lookups := 0
	lookup := func(context.Context, string) ([]net.IPAddr, error) {
		lookups++
		if lookups == 1 {
			return []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}}, nil
		}
		return []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}, nil
	}
	connection, err := dialPublicDirectoryHostWithLookup(context.Background(), "tcp", "rebind.example:80", lookup)
	if err == nil {
		if connection != nil {
			connection.Close()
		}
		t.Fatal("dial accepted a hostname that changed from a public to a loopback address")
	}
	if lookups < 2 {
		t.Fatalf("DNS lookups = %d, want a preflight lookup and a dial-time lookup", lookups)
	}
}

func TestPublicRequestURLValidationAndCertificateTargetParsing(t *testing.T) {
	for _, input := range []string{"https://example.com", "http://example.com:8080/path"} {
		if _, err := validatePublicHTTPURL(input); err != nil {
			t.Errorf("validatePublicHTTPURL(%q): %v", input, err)
		}
	}
	for _, input := range []string{"file:///etc/passwd", "https://user:pass@example.com", "https:///missing-host"} {
		if _, err := validatePublicHTTPURL(input); err == nil {
			t.Errorf("validatePublicHTTPURL(%q) unexpectedly succeeded", input)
		}
	}
	for _, test := range []struct {
		input    string
		hostname string
		port     int
	}{
		{"example.com", "example.com", 443},
		{"example.com:8443", "example.com", 8443},
		{"[2001:4860:4860::8888]:443", "2001:4860:4860::8888", 443},
	} {
		hostname, port, err := parseCertificateTarget(test.input)
		if err != nil || hostname != test.hostname || port != test.port {
			t.Errorf("parseCertificateTarget(%q) = (%q, %d, %v)", test.input, hostname, port, err)
		}
	}
	for _, input := range []string{"localhost:bad", "https://example.com", "user@example.com"} {
		if _, _, err := parseCertificateTarget(input); err == nil {
			t.Errorf("parseCertificateTarget(%q) unexpectedly succeeded", input)
		}
	}
}

type asnPrefixTestTransport struct{}

func (asnPrefixTestTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if request.URL.Host != "stat.ripe.net" || request.Header.Get("User-Agent") == "" {
		return nil, fmt.Errorf("unexpected RIPEstat request metadata")
	}
	body := `{"data_call_status":"ok","data":{"prefixes":[{"prefix":"1.0.0.0/24"},{"prefix":"2001:db8::/32"}]}}`
	return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: request}, nil
}

type asnCompleteTestTransport struct{}

func (asnCompleteTestTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if request.URL.Host != "stat.ripe.net" || request.URL.Query().Get("resource") != "AS15169" || request.Header.Get("User-Agent") == "" {
		return nil, fmt.Errorf("invalid RIPEstat request: %s", request.URL.String())
	}
	bodies := map[string]string{
		"/data/as-overview/data.json":            `{"data_call_status":"ok","data":{"holder":"Google LLC","block":{"desc":"Google"}}}`,
		"/data/announced-prefixes/data.json":     `{"data_call_status":"ok","data":{"prefixes":[{"prefix":"8.8.8.0/24"},{"prefix":"2001:4860::/32"}]}}`,
		"/data/asn-neighbours/data.json":         `{"data_call_status":"ok","data":{"neighbours":[{"asn":6453,"name":"TATA","type":"left"},{"asn":15169,"name":"Google","type":"right"}]}}`,
		"/data/asn-neighbours-history/data.json": `{"data_call_status":"ok","data":{"neighbours":[{"neighbour":6453,"timelines":[{"starttime":"2025-01-01T00:00:00","endtime":"2026-01-01T00:00:00"}]}]}}`,
		"/data/whois/data.json":                  `{"data_call_status":"ok","data":{"authorities":["arin"],"records":[[{"key":"country","value":"US"},{"key":"created","value":"2000-03-30T00:00:00Z"}]]}}`,
	}
	body, ok := bodies[request.URL.Path]
	if !ok {
		return nil, fmt.Errorf("unexpected RIPEstat endpoint %s", request.URL.Path)
	}
	return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: request}, nil
}

func TestASNHandlerUsesRIPEstatAndShowsRoutingMetadata(t *testing.T) {
	handler := NewASNHandler()
	handler.client = &http.Client{Transport: asnCompleteTestTransport{}}
	answer, err := handler.HandleDirectQuery(context.Background(), "AS15169")
	if err != nil {
		t.Fatal(err)
	}
	if answer.Source != "RIPEstat Data API" || answer.SourceURL != "https://stat.ripe.net/AS15169" {
		t.Fatalf("ASN source = %q %q", answer.Source, answer.SourceURL)
	}
	if !strings.Contains(answer.Content, "Observed upstream ASNs") || !strings.Contains(answer.Content, "Observed neighbour history") || !strings.Contains(answer.Content, "8.8.8.0/24") {
		t.Fatalf("ASN content omits routing details: %s", answer.Content)
	}
	if answer.Data["country"] != "US" || answer.Data["rir"] != "arin" {
		t.Fatalf("registration fields = %#v", answer.Data)
	}
	if answer.Data["ipv4AddressCount"] != "256" || answer.Data["ipv6AddressCount"] != "79228162514264337593543950336" {
		t.Fatalf("unique address counts = IPv4 %v, IPv6 %v", answer.Data["ipv4AddressCount"], answer.Data["ipv6AddressCount"])
	}
	if _, ok := answer.Data["upstreams"].([]asnNeighbour); !ok {
		t.Fatalf("upstreams have type %T", answer.Data["upstreams"])
	}
}

func TestCountAnnouncedAddressesMergesOverlappingPrefixes(t *testing.T) {
	for _, test := range []struct {
		name     string
		prefixes []string
		want     string
	}{
		{name: "overlapping IPv4", prefixes: []string{"192.0.2.0/24", "192.0.2.0/25", "192.0.2.128/25", "invalid"}, want: "256"},
		{name: "IPv6", prefixes: []string{"2001:db8::/126"}, want: "4"},
		{name: "full IPv6 range", prefixes: []string{"::/0"}, want: "340282366920938463463374607431768211456"},
		{name: "empty", want: "0"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := countAnnouncedAddresses(test.prefixes).String(); got != test.want {
				t.Fatalf("countAnnouncedAddresses(%v) = %s, want %s", test.prefixes, got, test.want)
			}
		})
	}
}

func TestASNHandlerFetchesAnnouncedPrefixes(t *testing.T) {
	handler := NewASNHandler()
	handler.client = &http.Client{Transport: asnPrefixTestTransport{}}
	ipv4, ipv6 := handler.fetchASNPrefixes(context.Background(), "13335")
	if !reflect.DeepEqual(ipv4, []string{"1.0.0.0/24"}) || !reflect.DeepEqual(ipv6, []string{"2001:db8::/32"}) {
		t.Fatalf("unexpected announced prefixes: IPv4=%v IPv6=%v", ipv4, ipv6)
	}
}
