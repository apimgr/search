//go:build e2e

// Tier 1 — server-side rendering. No browser, no JavaScript: a plain
// net/http client asserts that the initial HTML response already contains the
// real page. Per AI.md PART 28 ("SSR Correctness (Tier 1)"), a 200 with an
// empty shell that client-side code fills in later is a failure, not a pass —
// so every test here asserts on domain content and document structure rather
// than on status alone.
package e2e

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"golang.org/x/net/html"
	"image"
	"image/color"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
)

// TestMain owns the lifetime of the shared test sites.
func TestMain(m *testing.M) {
	code := m.Run()
	shutdownTestSite()
	shutdownTrackedSite()
	os.Exit(code)
}

// publicPages are the server-rendered resource and policy pages. Each must
// return complete HTML in the initial response.
var publicPages = []struct {
	path     string
	contains string
}{
	{"/server/about", `class="static-page about-page"`},
	{"/server/privacy", `class="static-page privacy-page"`},
	{"/server/contact", `class="static-page contact-page"`},
	{"/server/help", `class="static-page help-page"`},
	{"/server/terms", `class="static-page terms-page"`},
	{"/server/security", `class="static-page security-page"`},
	{"/server/security/policy", `class="static-page security-policy-page"`},
	{"/server/security/thanks", `class="static-page security-thanks-page"`},
	{"/server/preferences", `class="static-page preferences-page"`},
}

// TestSSRInteractivePagesCarryTheirForms asserts the two pages whose whole
// purpose is collecting input ship a native form, not a JS-driven widget. A
// contact or preferences page that renders without a <form> is a Tier 2
// failure waiting to happen.
func TestSSRInteractivePagesCarryTheirForms(t *testing.T) {
	s := newSession(t)

	for _, tc := range []struct{ path, formAction string }{
		{"/server/contact", `action="/server/contact"`},
		{"/server/preferences", `action="/server/preferences"`},
	} {
		t.Run(tc.path, func(t *testing.T) {
			resp, body := s.get(tc.path)
			requireStatus(t, resp, http.StatusOK)
			requireContains(t, body, "<form", tc.path)
			requireContains(t, body, tc.formAction, tc.path)
			if tc.path == "/server/contact" && strings.Contains(body, `name="captcha_id"`) {
				t.Fatal("initial contact form should not require CAPTCHA before a failed attempt")
			}
			// The double-submit CSRF token must be in the rendered markup;
			// without it the no-JS POST cannot be made at all.
			requireContains(t, body, `name="csrf_token"`, tc.path)
		})
	}
}

// TestSSRServerRootRedirectsToAbout covers the "/server → /server/about"
// redirect. A 200 at /server would mean the alias outranks the canonical page.
func TestSSRServerRootRedirectsToAbout(t *testing.T) {
	s := newSession(t).withoutRedirects()

	requireRedirect(t, mustGet(s, "/server"), http.StatusMovedPermanently, "/server/about")
}

// mustGet performs a GET and returns the response, discarding the body.
func mustGet(s *session, path string) *http.Response {
	s.t.Helper()
	resp, _ := s.get(path)
	return resp
}

// TestSSRPublicPagesRender walks every public HTML page and asserts the full
// document scaffold plus page-specific content is present before any script
// runs.
func TestSSRPublicPagesRender(t *testing.T) {
	s := newSession(t)

	for _, tc := range publicPages {
		t.Run(tc.path, func(t *testing.T) {
			resp, body := s.get(tc.path)
			requireStatus(t, resp, http.StatusOK)
			requireHTMLShell(t, body, tc.path)
			requireContains(t, body, `<meta charset="UTF-8">`, tc.path)
			requireContains(t, body, `name="viewport"`, tc.path)
			requireContains(t, body, `rel="manifest" href="/manifest.json"`, tc.path)
			requireContains(t, body, tc.contains, tc.path)
		})
	}
}

func TestSSRKeyboardShortcutGuideMatchesSupportedActions(t *testing.T) {
	s := newSession(t)
	resp, body := s.get("/server/help")
	requireStatus(t, resp, http.StatusOK)
	for _, marker := range []string{`id="keyboard-shortcuts"`, `<kbd>/</kbd>`, `<kbd>s</kbd>`, "Settings", `<kbd>Escape</kbd>`, `<kbd>?</kbd>`} {
		requireContains(t, body, marker, "keyboard shortcut guide")
	}
}

func TestSSRBangSearchRedirectsToSelectedEngine(t *testing.T) {
	s := newSession(t).withoutRedirects()
	resp, _ := s.get("/search?q=%21g+cats")
	requireStatus(t, resp, http.StatusFound)
	location := resp.Header.Get("Location")
	parsed, err := url.Parse(location)
	if err != nil {
		t.Fatalf("parse bang redirect location %q: %v", location, err)
	}
	if parsed.Host != "www.google.com" || parsed.Path != "/search" || parsed.Query().Get("q") != "cats" {
		t.Fatalf("bang redirect location = %q, want Google search for cats", location)
	}
}

// TestSSRHomePageIsFullyRendered guards the single most important Tier 1
// property: the landing page arrives complete. It checks the search form, the
// category selector, the navigation, and the footer — and explicitly rejects
// the empty-shell failure mode named in the spec.
func TestSSRHomePageIsFullyRendered(t *testing.T) {
	s := newSession(t)

	resp, body := s.get("/")
	requireStatus(t, resp, http.StatusOK)
	requireHTMLShell(t, body, "/")

	// The search form is a native GET form to /search, so it works with no
	// script at all. This is the app's primary surface.
	requireContains(t, body, `<form action="/search" method="GET"`, "/")
	requireContains(t, body, `name="q"`, "/")
	requireContains(t, body, `id="searchQuery"`, "/")
	requireContains(t, body, `type="submit"`, "/")
	requireContains(t, body, `name="category"`, "/")

	// Title and meta tags come from template/partial/head.tmpl and must name
	// the configured server title, not be left empty.
	requireContains(t, body, "<title>", "/")
	requireContains(t, body, s.title, "/")
	requireContains(t, body, `name="description"`, "/")
	requireContains(t, body, `property="og:title"`, "/")
	requireContains(t, body, `name="twitter:card"`, "/")

	// Real navigation, not a placeholder.
	requireContains(t, body, `href="/server/about"`, "/")
	requireContains(t, body, `href="/server/preferences"`, "/")

	// The empty-shell failure mode: a JS-only mount point standing in for the
	// page. A complete SSR page has substantial body copy already.
	if len(body) < 2000 {
		t.Errorf("/: response body is only %d bytes, looks like an empty shell rather than a rendered page", len(body))
	}
	requireNotContains(t, body, `<div id="app"></div>`, "/")
}

// TestSSRSearchPageRendersQueryAndCategories covers the search results page.
// Aggregated engine results require outbound network, so the assertions target
// what the server itself owns: the echoed query, the category tab strip, and
// the results container — all present in the SSR output.
func TestSSRSearchPageRendersQueryAndCategories(t *testing.T) {
	s := newSession(t)

	resp, body := s.get("/search?q=golang&lang=de&region=fr&time_range=week&time_from=2025-01-01&time_to=2025-01-31")
	requireStatus(t, resp, http.StatusOK)
	requireHTMLShell(t, body, "/search?q=golang")

	// The query is echoed into the results container's data attributes, which
	// is what the client-side infinite scroll reads.
	requireContains(t, body, `data-query="golang"`, "search page")
	requireContains(t, body, `data-category="general"`, "search page")
	for _, filter := range []string{`data-lang="de"`, `data-region="fr"`, `data-time-range="week"`, `data-time-from="2025-01-01"`, `data-time-to="2025-01-31"`} {
		requireContains(t, body, filter, "search page filters")
	}
	requireContains(t, body, `id="results-container"`, "search page")

	// Category switching is real anchors, not JS handlers, so it survives with
	// scripting off.
	for _, cat := range []string{"general", "images", "videos", "news", "maps", "science"} {
		want := `href="/search?q=golang&category=` + cat
		requireContains(t, body, want, "search page category "+cat)
	}

	// The alert-creation flow is reachable from the results page.
	requireContains(t, body, `href="/alerts/new?q=golang`, "search page")
}

func TestSSRRelatedSearchSuggestions(t *testing.T) {
	s := newSession(t)
	query := "ssr related fixture"
	response, body := s.get("/api/v1/search/related?q=" + url.QueryEscape(query))
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("related-search status = %d, want 200: %s", response.StatusCode, body)
	}
	assertRelatedSuggestions(t, body, query)
}

func TestSSRAutocompleteSuggestions(t *testing.T) {
	s := newSession(t)
	query := "ssr autocomplete"
	response, body := s.get("/autocomplete?q=" + url.QueryEscape(query))
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("autocomplete status = %d, want 200: %s", response.StatusCode, body)
	}
	assertAutocompleteSuggestions(t, body, query)
}

func TestSSRPreferenceSyncExportAndImport(t *testing.T) {
	s := newSession(t)
	response, body := s.get("/server/preferences/export", func(request *http.Request) {
		request.Header.Set("Accept", "application/json")
		for _, cookie := range []*http.Cookie{
			{Name: "theme", Value: "light"},
			{Name: "lang", Value: "de"},
			{Name: "search_pref_category", Value: "images"},
			{Name: "search_pref_infinite_scroll", Value: "0"},
		} {
			request.AddCookie(cookie)
		}
	})
	requireStatus(t, response, http.StatusOK)
	var envelope struct {
		Data struct {
			Theme string            `json:"theme"`
			Lang  string            `json:"lang"`
			Prefs map[string]string `json:"prefs"`
			URL   string            `json:"url"`
			Code  string            `json:"code"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &envelope); err != nil {
		t.Fatalf("decode preference export: %v", err)
	}
	if envelope.Data.Theme != "light" || envelope.Data.Lang != "de" || envelope.Data.Prefs["category"] != "images" || envelope.Data.Prefs["infinite_scroll"] != "0" {
		t.Fatalf("preference export omitted allowed values: %#v", envelope.Data)
	}
	if !strings.Contains(envelope.Data.URL, "/server/preferences/import?") || envelope.Data.Code == "" {
		t.Fatalf("preference export did not provide a URL and short code: %#v", envelope.Data)
	}
	response, _ = s.withoutRedirects().get("/server/preferences/import?code=" + url.QueryEscape(envelope.Data.Code) + "&redirect=%2Fserver%2Fpreferences")
	requireRedirect(t, response, http.StatusSeeOther, "/")
	pageResponse, preferences := s.get("/server/preferences")
	requireStatus(t, pageResponse, http.StatusOK)
	requireContains(t, preferences, `value="light" selected`, "imported theme")
	requireContains(t, preferences, `value="de" selected`, "imported language")
	requireContains(t, preferences, `value="images" selected`, "imported category")
}

// TestSSRDirectAnswers renders the direct-answer pages. Only pure-computation
// answer types are exercised so the assertions do not depend on outbound DNS,
// WHOIS, or HTTP to third parties.
//
// Terms are lowercase on purpose: PathSecurityMiddleware (PART 5) rejects any
// path segment containing an uppercase character, so "U+0041" is refused with
// a 400 before it reaches the direct handler. That rejection is asserted
// separately in TestSSRDirectAnswerRejectsUppercasePath.
func TestSSRDirectAnswers(t *testing.T) {
	s := newSession(t)

	t.Run("unicode character", func(t *testing.T) {
		resp, body := s.get("/direct/unicode/a")
		requireStatus(t, resp, http.StatusOK)
		requireHTMLShell(t, body, "/direct/unicode/a")
		requireContains(t, body, `class="direct-answer-page"`, "direct page")
		requireContains(t, body, `data-type="unicode"`, "direct page")
	})

	t.Run("json pretty print", func(t *testing.T) {
		resp, body := s.get("/direct/json/%7B%22a%22%3A1%7D")
		requireStatus(t, resp, http.StatusOK)
		requireHTMLShell(t, body, "direct json page")
		requireContains(t, body, `data-type="json"`, "direct json page")
	})

	t.Run("QR code generator", func(t *testing.T) {
		resp, body := s.get("/direct/qr/hello%20world")
		requireStatus(t, resp, http.StatusOK)
		requireHTMLShell(t, body, "direct QR page")
		requireContains(t, body, `data-type="qr"`, "direct QR page")
		requireContains(t, body, `download="qr-1024.png"`, "direct QR page")
		requireContains(t, body, `data:image/svg+xml;base64,`, "direct QR page")
	})

	t.Run("missing term is a rendered 400", func(t *testing.T) {
		resp, body := s.get("/direct/unicode")
		requireStatus(t, resp, http.StatusBadRequest)
		requireHTMLShell(t, body, "/direct/unicode")
	})
}

// TestSSRDirectQRLogoUpload verifies the native multipart form, its CSRF
// protection, raster-logo sanitization, and logo embedding in PNG/SVG outputs.
func TestSSRDirectQRLogoUpload(t *testing.T) {
	s := newSession(t)
	_, page := s.get("/direct/qr/logo%20fixture")
	const tokenMarker = `name="csrf_token" value="`
	tokenStart := strings.Index(page, tokenMarker)
	if tokenStart < 0 {
		t.Fatal("QR page did not render its CSRF token")
	}
	tokenStart += len(tokenMarker)
	tokenEnd := strings.IndexByte(page[tokenStart:], '"')
	if tokenEnd < 0 {
		t.Fatal("QR page rendered an incomplete CSRF token")
	}
	token := page[tokenStart : tokenStart+tokenEnd]

	logo := image.NewRGBA(image.Rect(0, 0, 24, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 24; x++ {
			logo.SetRGBA(x, y, color.RGBA{R: 220, G: 30, B: 30, A: 255})
		}
	}
	var logoBytes bytes.Buffer
	if err := png.Encode(&logoBytes, logo); err != nil {
		t.Fatal(err)
	}

	submit := func(csrf string) (*http.Response, string) {
		t.Helper()
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		for key, value := range map[string]string{"csrf_token": csrf, "size": "XL", "level": "L", "foreground": "#123456", "background": "#ABCDEF"} {
			if err := writer.WriteField(key, value); err != nil {
				t.Fatal(err)
			}
		}
		file, err := writer.CreateFormFile("logo", "logo.png")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := file.Write(logoBytes.Bytes()); err != nil {
			t.Fatal(err)
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
		req, err := http.NewRequest(http.MethodPost, s.baseURL+"/direct/qr/logo%20fixture", &body)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", writer.FormDataContentType())
		req.Header.Set("User-Agent", browserUserAgent)
		resp, err := s.client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		result, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		return resp, string(result)
	}

	resp, body := submit(token)
	requireStatus(t, resp, http.StatusOK)
	for _, marker := range []string{`value="XL" selected`, `value="H" selected`, `width="1024" height="1024"`, `data:image/png;base64,`, `data:image/svg+xml;base64,`} {
		if !strings.Contains(body, marker) {
			t.Errorf("logo QR result missing %q", marker)
		}
	}
	linkStart := strings.Index(body, `href="data:image/svg+xml;base64,`)
	if linkStart < 0 {
		t.Fatal("logo SVG download link is missing")
	}
	encodedStart := linkStart + len(`href="data:image/svg+xml;base64,`)
	encodedEnd := strings.IndexByte(body[encodedStart:], '"')
	vector, err := base64.StdEncoding.DecodeString(body[encodedStart : encodedStart+encodedEnd])
	if err != nil {
		t.Fatalf("decode generated SVG link: %v", err)
	}
	if !strings.Contains(string(vector), `<image `) || !strings.Contains(string(vector), `data:image/png;base64,`) {
		t.Fatal("generated SVG did not embed the sanitized raster logo")
	}

	resp, body = submit("")
	requireStatus(t, resp, http.StatusForbidden)
	requireContains(t, body, `"error":"CSRF_FAILED"`, "QR logo upload CSRF")
}

// TestSSRLocalInstantAnswerFeatures verifies every deterministic instant
// answer is returned with the expected type in the initial HTML response.
func TestSSRLocalInstantAnswerFeatures(t *testing.T) {
	s := newSession(t)
	for _, tc := range localInstantAnswerPages {
		t.Run(tc.name, func(t *testing.T) {
			resp, body := s.get("/search?q=" + url.QueryEscape(tc.query))
			requireStatus(t, resp, http.StatusOK)
			requireContains(t, body, `class="instant-answer-box" data-type="`+tc.typeName+`"`, tc.name)
			if !strings.Contains(body, `class="instant-answer-content"`) {
				t.Errorf("%s SSR output has no answer content", tc.name)
			}
			assertInstantAnswerFixture(t, tc.typeName, body)
		})
	}
}

// TestSSRLocalDirectAnswerFeatures asserts every deterministic direct-answer
// tool renders its feature page and nonempty domain content in the first response.
func TestSSRLocalDirectAnswerFeatures(t *testing.T) {
	s := newSession(t)
	for _, tc := range localDirectAnswerPages {
		t.Run(tc.name, func(t *testing.T) {
			resp, body := s.get(tc.path)
			requireStatus(t, resp, http.StatusOK)
			requireContains(t, body, `class="direct-answer-page" data-type="`+tc.typeName+`"`, tc.name)
			content := strings.SplitN(body, `<div class="direct-content`, 2)
			if len(content) != 2 || len(content[1]) < 100 {
				t.Errorf("%s SSR output has no substantial answer content", tc.name)
			}
			assertDirectAnswerFixture(t, tc.typeName, tc.path, body)
			assertFormatterHighlighting(t, tc.typeName, tc.path, body)
			if tc.typeName == "man" {
				assertManPageControls(t, body)
			}
			if strings.Contains(tc.name, "Wikipedia") {
				assertFullWikipediaFixture(t, body)
				if strings.Contains(tc.name, "localized") && !strings.Contains(body, `value="de" selected`) {
					t.Error("localized Wikipedia answer did not preserve the selected language")
				}
			}
			for _, forbidden := range []string{"onclick=", "onchange=", "oninput=", "onsubmit="} {
				if strings.Contains(body, forbidden) {
					t.Errorf("%s SSR output contains forbidden inline handler %q", tc.name, forbidden)
				}
			}
		})
	}
}

func TestSSRWikiSearchRedirectPreservesLanguage(t *testing.T) {
	s := newSession(t).withoutRedirects()
	resp, _ := s.get("/search?q=" + url.QueryEscape("wiki:Python") + "&lang=de")
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("wiki search redirect status = %d, want %d", resp.StatusCode, http.StatusFound)
	}
	if got, want := resp.Header.Get("Location"), "/direct/wiki/Python?lang=de"; got != want {
		t.Fatalf("wiki search redirect Location = %q, want %q", got, want)
	}
}

func TestSSRArchiveTimelineSelectsIndexedCapture(t *testing.T) {
	s := newSession(t)
	resp, body := s.get("/direct/cache/8.8.8.8?snapshot_index=1")
	requireStatus(t, resp, http.StatusOK)
	requireContains(t, body, "Archived fixture page older capture", "archive timeline index")
	requireContains(t, body, `name="snapshot_id"`, "archive timeline control")
}

// TestSSRMixedCaseDirectAnswerTerm verifies path filtering preserves case-sensitive
// user data while still allowing the lower-case direct-answer route to match.
func TestSSRMixedCaseDirectAnswerTerm(t *testing.T) {
	s := newSession(t)

	resp, body := s.get("/direct/unicode/U%2B0041")
	requireStatus(t, resp, http.StatusOK)
	requireContains(t, body, `class="direct-answer-page" data-type="unicode"`, "mixed-case direct-answer term")
	requireContains(t, body, "U+0041", "Unicode code point")
}

// TestSSRNotFoundRendersErrorPage asserts an unknown path returns 404 with a
// complete server-rendered error page, not a bare body.
func TestSSRNotFoundRendersErrorPage(t *testing.T) {
	s := newSession(t)

	resp, body := s.get("/this-page-does-not-exist")
	requireStatus(t, resp, http.StatusNotFound)
	requireHTMLShell(t, body, "404")
	requireContains(t, body, `href="/"`, "404 page")
}

func TestSSRRecoveredPanicNegotiatesThemedHTMLAndJSON(t *testing.T) {
	site := newPanicTestSite(t)
	t.Cleanup(site.Close)
	response, err := http.Get(site.URL + "/__e2e/panic")
	if err != nil {
		t.Fatal(err)
	}
	body, readErr := io.ReadAll(response.Body)
	response.Body.Close()
	if readErr != nil {
		t.Fatal(readErr)
	}
	if response.StatusCode != http.StatusInternalServerError || !strings.Contains(response.Header.Get("Content-Type"), "text/html") {
		t.Fatalf("browser panic response = %d %q", response.StatusCode, response.Header.Get("Content-Type"))
	}
	for _, marker := range []string{`class="theme-dark"`, `class="error-page"`, "Something went wrong", `href="/"`} {
		if !strings.Contains(string(body), marker) {
			t.Errorf("recovered HTML error is missing %q", marker)
		}
	}
	if strings.Contains(string(body), "e2e panic sentinel") || strings.Contains(string(body), "goroutine ") {
		t.Fatal("recovered HTML error leaked panic details or a stack trace")
	}
	request, err := http.NewRequest(http.MethodGet, site.URL+"/__e2e/panic", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Accept", "application/json")
	response, err = http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusInternalServerError || !strings.Contains(response.Header.Get("Content-Type"), "application/json") {
		t.Fatalf("API panic response = %d %q", response.StatusCode, response.Header.Get("Content-Type"))
	}
}

func TestSSRInternalLinkCrawlHasNoDeadLinksOrServerErrors(t *testing.T) {
	s := newSession(t)
	queue := []string{"/"}
	visited := make(map[string]bool)
	for len(queue) > 0 {
		path := queue[0]
		queue = queue[1:]
		if visited[path] {
			continue
		}
		visited[path] = true
		resp, body := s.get(path)
		if resp.StatusCode == http.StatusNotFound || resp.StatusCode >= http.StatusInternalServerError {
			t.Errorf("internal link %q returned HTTP %d", path, resp.StatusCode)
		}
		if !strings.Contains(resp.Header.Get("Content-Type"), "text/html") || resp.StatusCode >= 400 {
			continue
		}
		base, baseErr := url.Parse(path)
		if baseErr != nil {
			t.Errorf("parse crawl base %q: %v", path, baseErr)
			continue
		}
		doc, err := html.Parse(strings.NewReader(body))
		if err != nil {
			t.Errorf("parse %q: %v", path, err)
			continue
		}
		var collect func(*html.Node)
		collect = func(node *html.Node) {
			if node.Type == html.ElementNode && node.Data == "a" {
				for _, attr := range node.Attr {
					if attr.Key != "href" || attr.Val == "" {
						continue
					}
					ref, err := url.Parse(attr.Val)
					if err != nil || ref.Scheme != "" || ref.Host != "" || ref.Fragment != "" && ref.Path == "" {
						continue
					}
					resolved := base.ResolveReference(ref)
					resolved.Fragment = ""
					target := resolved.RequestURI()
					if resolved.Path == "" {
						target = "/" + resolved.RawQuery
					}
					if strings.HasPrefix(target, "/") && !visited[target] {
						queue = append(queue, target)
					}
				}
			}
			for child := node.FirstChild; child != nil; child = child.NextSibling {
				collect(child)
			}
		}
		collect(doc)
		if len(visited)+len(queue) > 500 {
			t.Fatal("internal link crawl exceeded 500 unique routes; possible unbounded query-link expansion")
		}
	}
	t.Logf("crawled %d internal routes", len(visited))
}

// TestSSRUnknownWellKnownIs404 checks the RFC 8615 catch-all only answers
// 404 for resources this project does not define, and never invents a page.
func TestSSRUnknownWellKnownIs404(t *testing.T) {
	s := newSession(t)

	resp, _ := s.get("/.well-known/nonexistent-resource")
	requireStatus(t, resp, http.StatusNotFound)
}

// TestSSROperatorPagesRequireToken covers the RequireOperator gate from
// PART 11. /server/status and /server/config must refuse an anonymous request
// and admit a request carrying the operator bearer token.
func TestSSROperatorPagesRequireToken(t *testing.T) {
	s := newSession(t)

	for _, path := range []string{"/server/status", "/server/config"} {
		t.Run(path+" refuses anonymous", func(t *testing.T) {
			resp, _ := s.get(path)
			requireStatus(t, resp, http.StatusUnauthorized)
		})
		t.Run(path+" admits operator", func(t *testing.T) {
			resp, _ := s.get(path, asOperator)
			requireStatus(t, resp, http.StatusOK)
		})
	}
}

// TestSSRPWAAndWellKnownAssets asserts the machine-readable surface is served
// with the right content type and non-empty payload. These are consumed by
// browsers, crawlers, and AI agents, so an empty or wrong-typed response is a
// real defect.
func TestSSRPWAAndWellKnownAssets(t *testing.T) {
	s := newSession(t)

	cases := []struct {
		path        string
		contentType string
		contains    string
	}{
		// The manifest content type is the spec-mandated
		// application/manifest+json, not application/json; a browser refuses
		// to install the PWA when it is wrong.
		{"/manifest.json", "application/manifest+json", `"name"`},
		{"/sw.js", "javascript", ""},
		{"/robots.txt", "text/plain", "User-agent:"},
		{"/sitemap.xml", "xml", "<urlset"},
		{"/llms.txt", "text/plain", ""},
		{"/opensearch.xml", "xml", "<OpenSearchDescription"},
		{"/.well-known/security.txt", "text/plain", "Contact:"},
		{"/offline.html", "text/html", "<!DOCTYPE html>"},
	}

	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			resp, body := s.get(tc.path)
			requireStatus(t, resp, http.StatusOK)
			if tc.contentType != "" {
				got := resp.Header.Get("Content-Type")
				if !strings.Contains(got, tc.contentType) {
					t.Errorf("GET %s Content-Type = %q, want it to contain %q", tc.path, got, tc.contentType)
				}
			}
			if tc.contains != "" {
				requireContains(t, body, tc.contains, tc.path)
			}
			if len(body) == 0 {
				t.Errorf("GET %s returned an empty body", tc.path)
			}
		})
	}
}

// TestSSRPGPKeyIsAbsentWithoutAKeypair covers the documented precondition on
// /.well-known/pgp-key.asc (src/server/wellknown.go): the route answers 404
// until a keypair has actually been generated, even though publishing is
// enabled by default. Serving a placeholder or an empty body here would be
// worse than a 404 — a client would pin a key that does not exist.
func TestSSRPGPKeyIsAbsentWithoutAKeypair(t *testing.T) {
	s := newSession(t)

	resp, body := s.get("/.well-known/pgp-key.asc")
	requireStatus(t, resp, http.StatusNotFound)
	if strings.Contains(body, "BEGIN PGP PUBLIC KEY BLOCK") {
		t.Error("/.well-known/pgp-key.asc served a key block with no keypair configured")
	}
}

// TestSSRChangePasswordIsNotFound confirms the project advertises no accounts:
// RFC 8615 expects a 404 there, and a 200 would falsely imply a login flow.
func TestSSRChangePasswordIsNotFound(t *testing.T) {
	s := newSession(t)

	resp, _ := s.get("/.well-known/change-password")
	requireStatus(t, resp, http.StatusNotFound)
}

// TestSSRHealthEndpoints covers the PART 13 health surface in both its
// content-negotiated HTML form and its plain-text form.
func TestSSRHealthEndpoints(t *testing.T) {
	s := newSession(t)

	t.Run("browser gets HTML", func(t *testing.T) {
		resp, body := s.get("/server/healthz")
		requireStatus(t, resp, http.StatusOK)
		if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "text/html") {
			t.Errorf("/server/healthz Content-Type = %q, want text/html for a browser", ct)
		}
		requireHTMLShell(t, body, "/server/healthz")
	})

	t.Run("text form is available", func(t *testing.T) {
		resp, body := s.get("/server/healthz.txt")
		requireStatus(t, resp, http.StatusOK)
		if len(body) == 0 {
			t.Error("/server/healthz.txt returned an empty body")
		}
	})

	t.Run("liveness and readiness answer", func(t *testing.T) {
		for _, path := range []string{"/livez", "/readyz"} {
			resp, _ := s.get(path)
			requireStatus(t, resp, http.StatusOK)
		}
	})
}

// TestSSRThemeRendersDarkByDefault asserts the PART 16 theme contract: the
// server renders the theme class onto <html> and dark is the default, so no
// flash of the wrong theme occurs before CSS loads.
func TestSSRThemeRendersDarkByDefault(t *testing.T) {
	s := newSession(t)

	resp, body := s.get("/")
	requireStatus(t, resp, http.StatusOK)
	requireContains(t, body, `class="theme-dark"`, "default theme")
	requireContains(t, body, `data-theme-mode="dark"`, "default theme")
}

func TestSSRLanguageQueryChangesRenderedStrings(t *testing.T) {
	s := newSession(t)

	resp, body := s.get("/?lang=de")
	requireStatus(t, resp, http.StatusOK)
	requireContains(t, body, `<html lang="de"`, "German document language")
	requireContains(t, body, `aria-label="Suchen"`, "German search label")
}

// TestSSRThemePersistsAcrossRequest exercises the no-JS theme toggle: the
// POST /theme endpoint (src/server/theme.go) stores the theme in a cookie and
// redirects back, and the next render reflects it. This is the fallback path
// for browsers with scripting disabled, so it must work with a plain POST and
// the redirect must be a 303 so the browser replays as GET.
func TestSSRThemePersistsAcrossRequest(t *testing.T) {
	s := newSession(t).withoutRedirects()

	token, _ := s.csrfTokenFor("/")

	requireRedirect(t, mustPost(s, "/theme", url.Values{
		"csrf_token": {token},
		"theme":      {"light"},
		"redirect":   {"/"},
	}), http.StatusSeeOther, "/")

	// The session jar must now hold the theme cookie, and the next render
	// must come back light.
	_, body := s.get("/")
	requireContains(t, body, `class="theme-light"`, "theme after POST /theme")
}

// mustPost performs a form POST and returns the response, discarding the body.
func mustPost(s *session, path string, form url.Values) *http.Response {
	s.t.Helper()
	resp, _ := s.postForm(path, form)
	return resp
}

// TestSSRThemeRedirectRejectsExternalTarget asserts the open-redirect guard:
// handleThemeSet only honours a redirect path that is a single-slash
// relative path, so an attacker-supplied absolute URL cannot be used to bounce
// a visitor off-site after they toggle the theme.
func TestSSRThemeRedirectRejectsExternalTarget(t *testing.T) {
	s := newSession(t).withoutRedirects()

	token, _ := s.csrfTokenFor("/")

	for _, target := range []string{"https://evil.example/", "//evil.example/"} {
		requireRedirect(t, mustPost(s, "/theme", url.Values{
			"csrf_token": {token},
			"theme":      {"dark"},
			"redirect":   {target},
		}), http.StatusSeeOther, "/")
	}
}

// TestSSRAPIDocsPagesRender covers the PART 14 documentation surface. Swagger
// UI and GraphiQL are self-contained pages that do not use the project layout
// scaffold, so they are asserted on their own markers: a real doctype, a
// title, and the mount div plus the spec URL the UI fetches. A docs page that
// renders an empty div with no reference to the spec is a dead page.
func TestSSRAPIDocsPagesRender(t *testing.T) {
	s := newSession(t)

	t.Run("swagger ui", func(t *testing.T) {
		resp, body := s.get("/server/docs/swagger")
		requireStatus(t, resp, http.StatusOK)
		requireContains(t, body, "<!DOCTYPE html>", "/server/docs/swagger")
		requireContains(t, body, "swagger-ui", "/server/docs/swagger")
		requireContains(t, body, "/api/swagger", "/server/docs/swagger")
	})

	t.Run("graphiql", func(t *testing.T) {
		resp, body := s.get("/server/docs/graphql")
		requireStatus(t, resp, http.StatusOK)
		requireContains(t, body, "<!DOCTYPE html>", "/server/docs/graphql")
		requireContains(t, body, "/api/graphql", "/server/docs/graphql")
	})
}

// TestSSRConsentFormSetsCookie exercises POST /server/consent, which is a
// POST-only endpoint (src/server/pages.go). The banner forms in
// template/partial/cookie_consent.tmpl post here with zero JavaScript, so the
// whole flow is: read the CSRF token out of the rendered page, post choice,
// receive a 303 back, and carry the cookie on the next request. An accept
// followed by a decline must be observable on the server, not just in the
// client's storage.
func TestSSRConsentFormSetsCookie(t *testing.T) {
	for _, choice := range []string{"accept", "decline"} {
		t.Run(choice, func(t *testing.T) {
			// withoutRedirects: the 303 is the assertion. A following client
			// would report the 200 of the redirect target and hide it.
			s := newSession(t).withoutRedirects()

			token, _ := s.csrfTokenFor("/")
			resp, _ := s.postForm("/server/consent", url.Values{
				"csrf_token": {token},
				"choice":     {choice},
			})

			if resp.StatusCode != http.StatusSeeOther {
				t.Fatalf("POST /server/consent status = %d, want %d", resp.StatusCode, http.StatusSeeOther)
			}

			// The cookie value is percent-encoded JSON (AI.md PART 16), so it
			// survives net/http's cookie sanitization intact and the client's
			// decodeURIComponent + JSON.parse round-trips it. Assert on the
			// decoded value: that is exactly what app.js readConsentCookie
			// does, so a regression here is a real client-side break.
			var raw string
			for _, c := range resp.Cookies() {
				if c.Name == "cookie_consent" {
					raw = c.Value
				}
			}
			if raw == "" {
				t.Fatalf("no cookie_consent in Set-Cookie: %v", resp.Header.Values("Set-Cookie"))
			}
			decoded, err := url.PathUnescape(raw)
			if err != nil {
				t.Fatalf("cookie_consent is not percent-encoded: %v (value %q)", err, raw)
			}
			var consent struct {
				Essential   bool  `json:"essential"`
				Preferences bool  `json:"preferences"`
				Analytics   bool  `json:"analytics"`
				Timestamp   int64 `json:"timestamp"`
			}
			if err := json.Unmarshal([]byte(decoded), &consent); err != nil {
				t.Fatalf("cookie_consent does not round-trip as JSON: %v (value %q)", err, decoded)
			}
			if !consent.Essential {
				t.Errorf("cookie_consent essential = false, want true (%q)", decoded)
			}
			if consent.Timestamp == 0 {
				t.Errorf("cookie_consent timestamp = 0, want a unix time (%q)", decoded)
			}

			analyticsOn := choice == "accept"
			if got := consent.Analytics; got != analyticsOn {
				t.Errorf("cookie_consent analytics = %v, want %v (value %q)", got, analyticsOn, decoded)
			}

			// The jar must replay it, so the next page render sees consent.
			_, body := s.get("/")
			if strings.Contains(body, `id="cookie-consent"`) {
				t.Errorf("/ still renders the cookie banner after choice=%s", choice)
			}
		})
	}
}

// TestSSRConsentGatesTracking is the full round trip of the AI.md PART 16
// consent contract against a site that actually has an analytics provider
// configured: a first visit must render no third-party script, "I Agree" must
// start injecting it on the next request, and "Decline" must keep it out even
// though the provider is configured.
//
// Without this the gate is only proven at the unit level, and a template that
// stopped rendering {{.TrackingHTML}} — or one that rendered it unconditionally
// — would pass every other test in the suite.
func TestSSRConsentGatesTracking(t *testing.T) {
	t.Run("no consent", func(t *testing.T) {
		s := newTrackedSession(t)
		_, body := s.get("/")
		if strings.Contains(body, trackingMarker) {
			t.Error("analytics script was injected before any consent was given")
		}
	})

	for _, tt := range []struct {
		choice    string
		wantTrack bool
	}{
		{choice: "accept", wantTrack: true},
		{choice: "decline", wantTrack: false},
	} {
		t.Run(tt.choice, func(t *testing.T) {
			s := newTrackedSession(t)

			token, _ := s.csrfTokenFor("/")
			s.postForm("/server/consent", url.Values{
				"csrf_token": {token},
				"choice":     {tt.choice},
			})

			// The jar replays cookie_consent, so this render is the one the
			// spec describes as "later requests".
			_, body := s.get("/")
			got := strings.Contains(body, trackingMarker)
			if got != tt.wantTrack {
				t.Errorf("after choice=%s the analytics script is present = %v, want %v", tt.choice, got, tt.wantTrack)
			}
		})
	}
}

// TestSSRConsentRejectsBadCSRF is the negative half of the double-submit
// contract from PART 11: a POST whose token does not match the cookie must be
// refused, or the consent endpoint is an open cross-site form target.
func TestSSRConsentRejectsBadCSRF(t *testing.T) {
	s := newSession(t)

	// Prime a session so a csrf_token cookie exists, then send a mismatched one.
	s.csrfTokenFor("/")

	resp, _ := s.postForm("/server/consent", url.Values{
		"csrf_token": {"not-the-real-token"},
		"choice":     {"accept"},
	})

	if resp.StatusCode == http.StatusSeeOther {
		t.Errorf("POST /server/consent with a bad CSRF token returned %d, want a refusal", resp.StatusCode)
	}
}

// TestSSRCCPAPostRoundTrip covers POST /server/ccpa: opt-out sets the cookie,
// opt-in clears it, and both redirect back. The cookie must be observable in
// the session jar afterwards, not merely in the redirect response.
func TestSSRCCPAPostRoundTrip(t *testing.T) {
	// withoutRedirects: both 303s are under assertion, and the opt-in branch
	// must be observed as an expiring Set-Cookie on the 303 itself.
	s := newSession(t).withoutRedirects()

	token, _ := s.csrfTokenFor("/")

	resp, _ := s.postForm("/server/ccpa", url.Values{
		"csrf_token": {token},
		"action":     {"opt-out"},
	})
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("POST /server/ccpa opt-out status = %d, want %d", resp.StatusCode, http.StatusSeeOther)
	}

	var optOut *http.Cookie
	for _, c := range resp.Cookies() {
		if c.Name == "ccpa_opt_out" {
			optOut = c
		}
	}
	if optOut == nil {
		t.Fatal("POST /server/ccpa opt-out set no ccpa_opt_out cookie")
	}
	if optOut.Value != "true" {
		t.Errorf("ccpa_opt_out = %q, want %q", optOut.Value, "true")
	}

	// Opt back in: the handler must actively expire the cookie, not just skip
	// setting it, so an already-opted-out client is really cleared.
	token2, _ := s.csrfTokenFor("/")
	resp2, _ := s.postForm("/server/ccpa", url.Values{
		"csrf_token": {token2},
		"action":     {"opt-in"},
	})
	if resp2.StatusCode != http.StatusSeeOther {
		t.Fatalf("POST /server/ccpa opt-in status = %d, want %d", resp2.StatusCode, http.StatusSeeOther)
	}

	cleared := false
	for _, c := range resp2.Cookies() {
		if c.Name == "ccpa_opt_out" {
			if c.Value != "" || c.MaxAge >= 0 {
				t.Errorf("ccpa_opt_out opt-in = value %q maxAge %d, want it cleared with a negative MaxAge", c.Value, c.MaxAge)
			}
			cleared = true
		}
	}
	if !cleared {
		t.Error("POST /server/ccpa opt-in sent no ccpa_opt_out cookie at all; an already-set cookie would survive")
	}
}

// TestSSRTextBrowserGetsNoJSRender checks PART 14 content negotiation: a text
// browser is interactive but cannot run script, so the server serves the
// dedicated minimal renderers in src/server/nojs.go rather than the full
// templates. That renderer emits its own minimal document — no theme class, no
// skip link — so it is asserted on its own markers: a native GET form to
// /search, a real title, and zero <script> tags.
func TestSSRTextBrowserGetsNoJSRender(t *testing.T) {
	s := newSession(t).withUserAgent(textBrowserUserAgent)

	resp, body := s.get("/")
	requireStatus(t, resp, http.StatusOK)
	requireContains(t, body, "<!DOCTYPE html>", "text-browser home")
	requireContains(t, body, "<title>", "text-browser home")
	requireContains(t, body, `<form method="GET" action="/search" role="search">`, "text-browser home form")
	requireContains(t, body, `name="q"`, "text-browser home form")
	requireNotContains(t, body, "<script", "text-browser home")

	// The search results page must offer a way back, or a text-browser user
	// navigates into a dead end.
	resp2, body2 := s.get("/search?q=golang")
	requireStatus(t, resp2, http.StatusOK)
	requireContains(t, body2, `href="/"`, "text-browser results")
	requireNotContains(t, body2, "<script", "text-browser results")
}

// TestSSRHTTPToolGetsPlainText checks the other half of the negotiation: an
// HTTP tool is non-interactive and receives the page converted to text, never
// raw markup.
func TestSSRHTTPToolGetsPlainText(t *testing.T) {
	s := newSession(t).withUserAgent(httpToolUserAgent)

	resp, body := s.get("/")
	requireStatus(t, resp, http.StatusOK)
	requireNotContains(t, body, "<html", "http tool response")
	if len(strings.TrimSpace(body)) == 0 {
		t.Error("HTTP tool response was empty")
	}
}

// TestSSRSecurityHeaders pins the PART 11 response headers on a page
// response. A regression here is invisible to the DOM but is a real
// vulnerability, so it is asserted at the transport layer.
func TestSSRSecurityHeaders(t *testing.T) {
	s := newSession(t)

	resp, _ := s.get("/")

	want := map[string]string{
		"X-Frame-Options":        "SAMEORIGIN",
		"X-Content-Type-Options": "nosniff",
		"Referrer-Policy":        "strict-origin-when-cross-origin",
	}
	for header, value := range want {
		if got := resp.Header.Get(header); got != value {
			t.Errorf("GET / %s = %q, want %q", header, got, value)
		}
	}

	csp := resp.Header.Get("Content-Security-Policy")
	if csp == "" {
		t.Error("GET / returned no Content-Security-Policy header")
	}
}
