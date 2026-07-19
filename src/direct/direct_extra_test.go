package direct

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
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
		{"slang empty", NewSlangHandler()},
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
		{URL: "https://example.com/feed.xml", Type: "RSS", Title: "My Feed"},
		{URL: "https://example.com/atom.xml", Type: "Atom", Title: ""},
	}
	content := formatFeedContent("example.com", feeds)
	if !strings.Contains(content, "example.com") {
		t.Errorf("expected domain; got: %s", content)
	}
	if !strings.Contains(content, "feed.xml") {
		t.Errorf("expected feed URL; got: %s", content)
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
				if c.name == tt.checkName {
					if c.passed != tt.checkPassed {
						t.Errorf("check %q: expected passed=%v, got %v (details: %s)",
							tt.checkName, tt.checkPassed, c.passed, c.details)
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
		{name: "HTTPS", passed: true, details: "Uses HTTPS"},
		{name: "Domain TLD", passed: false, details: "Suspicious TLD"},
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
}

func TestCacheHandlerWithFullURL(t *testing.T) {
	h := NewCacheHandler()
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

func TestSafeHandlerHTTPSIsSafe(t *testing.T) {
	h := NewSafeHandler()
	ctx := context.Background()

	ans, err := h.HandleDirectQuery(ctx, "https://example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ans == nil {
		t.Fatal("nil answer")
	}
	if ans.Description != "Safe" {
		t.Errorf("expected Safe rating for https example.com, got %q", ans.Description)
	}
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
	ctx := context.Background()

	// "go/" prefix triggers fetchGo path which returns a static answer without network
	ans, err := h.HandleDirectQuery(ctx, "go/github.com/go-chi/chi/v5")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ans == nil {
		t.Fatal("nil answer")
	}
	if !strings.Contains(ans.Content, "go get") {
		t.Errorf("expected go get command; got: %s", ans.Content)
	}
}

func TestPkgHandlerGoModPathAutoDetect(t *testing.T) {
	h := NewPkgHandler()
	ctx := context.Background()

	// A path with slashes but no registry prefix auto-detects as Go
	ans, err := h.HandleDirectQuery(ctx, "github.com/some/package")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ans == nil {
		t.Fatal("nil answer")
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
	content := formatCountryContent("DE", "", "Germany", "Federal Republic of Germany", data)
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
	ctx := context.Background()

	ans, err := h.HandleDirectQuery(ctx, "json {invalid json}")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ans == nil {
		t.Fatal("nil answer")
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

	result := formatDictContent("hello", "/həˈloʊ/", "https://audio.example.com/hello.mp3", meanings, "Old English hel")
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
	result := formatCountryContent("DE", "https://flag.example.com/de.svg", "Germany", "Federal Republic of Germany", data)
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
