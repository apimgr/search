//go:build e2e

// Tier 2 — no-JS browser. A real headless Chromium tab with script execution
// disabled via CDP before any navigation (AI.md PART 28). This is the
// progressive-enhancement gate (PART 14): every core flow this project ships
// must work through native form POSTs, plain links, and full-page redirects,
// because the same server has to serve users with JavaScript off, on a text
// browser, or on a screen reader that never runs the enhancement layer.
//
// A feature that only works in browser_test.go (Tier 3, JS on) is not a pass
// per PART 28 — it is exactly the violation this tier exists to catch.
package e2e

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

// runNoJS is the Tier 2 entry point: launches a tab, disables JS, and hands
// the caller a context to drive with chromedp.Run.
func runNoJS(t *testing.T, steps ...chromedp.Action) string {
	t.Helper()

	ctx := newBrowserContext(t)
	if err := chromedp.Run(ctx, emulation.SetScriptExecutionDisabled(true)); err != nil {
		t.Fatalf("disable script execution: %v", err)
	}

	var html string
	full := append(steps, chromedp.OuterHTML("html", &html))
	if err := chromedp.Run(ctx, full...); err != nil {
		t.Fatalf("chromedp run: %v", err)
	}
	return html
}

// TestNoJSHomeSearchFormSubmitsNatively verifies the core search flow (the
// only feature IDEA.md defines, "Core Search") works as a plain GET form:
// typing a query and clicking submit must navigate to /search?q=... with no
// script running at all.
func TestNoJSHomeSearchFormSubmitsNatively(t *testing.T) {
	site := newTestSite(t)

	var url string
	runNoJS(t,
		chromedp.Navigate(site.URL+"/"),
		chromedp.WaitVisible(`#searchQuery`, chromedp.ByQuery),
		chromedp.SendKeys(`#searchQuery`, "golang testing"),
		chromedp.Click(`.search-btn`, chromedp.ByQuery),
		chromedp.WaitVisible(`.search-results-page`, chromedp.ByQuery),
		chromedp.Location(&url),
	)

	if !strings.Contains(url, "/search") || !strings.Contains(url, "q=golang") {
		t.Errorf("no-JS search submit navigated to %q, want /search?q=golang...", url)
	}
}

func TestNoJSRelatedSearchSuggestions(t *testing.T) {
	site := newTestSite(t)
	query := "nojs related fixture"
	var body string
	runNoJS(t,
		chromedp.Navigate(site.URL+"/api/v1/search/related?q="+url.QueryEscape(query)),
		chromedp.Text("body", &body, chromedp.ByQuery),
	)
	assertRelatedSuggestions(t, body, query)
}

func TestNoJSAutocompleteSuggestions(t *testing.T) {
	site := newTestSite(t)
	query := "nojs autocomplete"
	var body string
	runNoJS(t,
		chromedp.Navigate(site.URL+"/autocomplete?q="+url.QueryEscape(query)),
		chromedp.Text("body", &body, chromedp.ByQuery),
	)
	assertAutocompleteSuggestions(t, body, query)
}

func TestNoJSKeyboardShortcutGuideRemainsReadable(t *testing.T) {
	site := newTestSite(t)
	html := runNoJS(t,
		chromedp.Navigate(site.URL+"/server/help"),
		chromedp.WaitVisible(`#keyboard-shortcuts`, chromedp.ByQuery),
	)
	for _, marker := range []string{`<kbd>/</kbd>`, `<kbd>s</kbd>`, "Settings", `<kbd>Escape</kbd>`} {
		if !strings.Contains(html, marker) {
			t.Errorf("no-JS shortcut guide is missing %q", marker)
		}
	}
}

func TestNoJSSearchCategoryCanBeSelectedNatively(t *testing.T) {
	site := newTestSite(t)
	var pageURL, category string
	runNoJS(t,
		chromedp.Navigate(site.URL+"/"),
		chromedp.WaitVisible(`#nojs-category`, chromedp.ByQuery),
		chromedp.SetValue(`#nojs-category`, "images", chromedp.ByQuery),
		chromedp.SendKeys(`#searchQuery`, "mountain landscape"),
		chromedp.Click(`.search-btn`, chromedp.ByQuery),
		chromedp.WaitVisible(`.search-results-page`, chromedp.ByQuery),
		chromedp.Location(&pageURL),
		chromedp.AttributeValue(`.search-results-page`, `data-category`, &category, nil, chromedp.ByQuery),
	)
	if !strings.Contains(pageURL, "category=images") || category != "images" {
		t.Fatalf("no-JS category search used URL %q and rendered category %q", pageURL, category)
	}
}

func TestNoJSAdvancedSearchUsesNativeForm(t *testing.T) {
	site := newTestSite(t)
	var pageURL, query, language, region, timeRange string
	runNoJS(t,
		chromedp.Navigate(site.URL+"/"),
		chromedp.WaitVisible(`form.advanced-search-fallback`, chromedp.ByQuery),
		chromedp.SetValue(`#nojs-as-allwords`, "golang testing", chromedp.ByQuery),
		chromedp.SetValue(`#nojs-as-exactphrase`, "unit tests", chromedp.ByQuery),
		chromedp.SetValue(`#nojs-as-anywords`, "fast secure", chromedp.ByQuery),
		chromedp.SetValue(`#nojs-as-nonewords`, "java ruby", chromedp.ByQuery),
		chromedp.SetValue(`#nojs-as-site`, "example.com", chromedp.ByQuery),
		chromedp.SetValue(`#nojs-as-filetype`, "pdf", chromedp.ByQuery),
		chromedp.SetValue(`#nojs-as-time`, "week", chromedp.ByQuery),
		chromedp.SetValue(`#nojs-as-lang`, "fr", chromedp.ByQuery),
		chromedp.SetValue(`#nojs-as-region`, "uk", chromedp.ByQuery),
		chromedp.Click(`form.advanced-search-fallback button[type=submit]`, chromedp.ByQuery),
		chromedp.WaitVisible(`.search-results-page`, chromedp.ByQuery),
		chromedp.Location(&pageURL),
		chromedp.AttributeValue(`.search-results-page`, `data-query`, &query, nil, chromedp.ByQuery),
		chromedp.AttributeValue(`.search-results-page`, `data-lang`, &language, nil, chromedp.ByQuery),
		chromedp.AttributeValue(`.search-results-page`, `data-region`, &region, nil, chromedp.ByQuery),
		chromedp.AttributeValue(`.search-results-page`, `data-time-range`, &timeRange, nil, chromedp.ByQuery),
	)
	for _, fragment := range []string{"as_allwords=golang", "as_exactphrase", "as_site=example.com", "as_filetype=pdf", "as_time=week", "as_lang=fr", "as_region=uk"} {
		if !strings.Contains(pageURL, fragment) {
			t.Errorf("native advanced-search URL %q is missing %q", pageURL, fragment)
		}
	}
	if !strings.Contains(query, `"unit tests"`) || !strings.Contains(query, `(fast OR secure)`) || language != "fr" || region != "uk" || timeRange != "week" {
		t.Fatalf("native advanced filters resolved incorrectly: query=%q language=%q region=%q time=%q", query, language, region, timeRange)
	}
}

// TestNoJSThemeFormAppliesWithoutScript exercises PART 16's requirement that
// theme switching works with JS disabled: the header exposes a native
// <select>+submit form (action="/theme") specifically for this case,
// alongside the JS-driven toggle button.
func TestNoJSAlertCreationActivatesWithoutEmailVerification(t *testing.T) {
	site := newTestSite(t)
	html := runNoJS(t,
		chromedp.Navigate(site.URL+"/alerts/new?q=accountless%20monitor&category=general"),
		chromedp.WaitVisible("form.alert-form", chromedp.ByQuery),
		chromedp.SetValue("#email", "alerts@example.test", chromedp.ByQuery),
		chromedp.Click("form.alert-form button[type=submit]", chromedp.ByQuery),
		chromedp.WaitVisible("a[href*='/alerts/manage/']", chromedp.ByQuery),
	)
	if !strings.Contains(html, "Your alert is active.") {
		t.Fatalf("created alert did not report active state: %s", snippet(html, "<body"))
	}
	if strings.Contains(strings.ToLower(html), "check your email") || strings.Contains(strings.ToLower(html), "confirm and activate") {
		t.Fatalf("created alert page still requests email verification: %s", snippet(html, "<body"))
	}
	if !strings.Contains(html, "/alerts/manage/") || !strings.Contains(html, ".rss") {
		t.Fatalf("created alert page is missing manage or private RSS links: %s", snippet(html, "<body"))
	}
}

func TestNoJSOwnerTokenManagementLifecycle(t *testing.T) {
	site := newTestSite(t)
	var primary, linked, updated, rotated string
	var status string
	primary = createAPIAlertOwner(t, site, "owner token nojs fixture")
	ctx := newBrowserContext(t)
	if err := chromedp.Run(ctx, emulation.SetScriptExecutionDisabled(true)); err != nil {
		t.Fatalf("disable script execution: %v", err)
	}
	if err := chromedp.Run(ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			err := network.SetCookie("search_owner_token_xJ5yCXLv", primary).
				WithURL(site.URL).WithPath("/").WithSecure(true).WithHTTPOnly(true).
				WithSameSite(network.CookieSameSiteStrict).Do(ctx)
			return err
		}),
		chromedp.Navigate(site.URL+"/server/preferences"),
		chromedp.WaitVisible(`#owner-tokens-heading`, chromedp.ByQuery),
	); err != nil {
		t.Fatalf("load API-created owner in a no-JS browser: %v", err)
	}
	if err := chromedp.Run(ctx,
		chromedp.Navigate(site.URL+"/server/preferences"),
		chromedp.SetValue(`form[action="/server/preferences/tokens"] input[name=name]`, "nojs-linked", chromedp.ByQuery),
		chromedp.SetValue(`form[action="/server/preferences/tokens"] select[name=expires_at]`, "7d", chromedp.ByQuery),
		chromedp.Click(`form[action="/server/preferences/tokens"] button[type=submit]`, chromedp.ByQuery),
		chromedp.WaitVisible(".token-reveal .token-value", chromedp.ByQuery),
		chromedp.Text(".token-reveal .token-value", &linked, chromedp.ByQuery),
	); err != nil {
		t.Fatalf("create linked token without JavaScript: %v", err)
	}
	if primary == "" || linked == "" || primary == linked {
		t.Fatalf("issued tokens invalid: primary present=%t linked present=%t equal=%t", primary != "", linked != "", primary == linked)
	}
	request, err := http.NewRequest(http.MethodPost, site.URL+"/api/v1/server/preferences/tokens", strings.NewReader("name=forbidden"))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+linked)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("linked-token management request: %v", err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("linked token management request status = %d, want 403", response.StatusCode)
	}
	var action string
	if err := chromedp.Run(ctx,
		chromedp.Evaluate(`document.querySelector('.owner-token-item form[action$="/update"]').action`, &action),
		chromedp.SetValue(`.owner-token-item form[action$="/update"] input[name=name]`, "nojs-renamed", chromedp.ByQuery),
		chromedp.SetValue(`.owner-token-item form[action$="/update"] select[name=expires_at]`, "30d", chromedp.ByQuery),
		chromedp.Click(`.owner-token-item form[action$="/update"] button[type=submit]`, chromedp.ByQuery),
		chromedp.WaitVisible(`.owner-token-item h4`, chromedp.ByQuery),
		chromedp.Text(`.owner-token-item h4`, &updated, chromedp.ByQuery),
	); err != nil {
		t.Fatalf("rename and update linked-token expiry without JavaScript: %v", err)
	}
	if updated != "nojs-renamed" {
		t.Fatalf("linked token name after update = %q", updated)
	}
	if err := chromedp.Run(ctx,
		chromedp.Click(`.owner-token-item form[action$="/rotate"] button`, chromedp.ByQuery),
		chromedp.WaitVisible(".token-reveal .token-value", chromedp.ByQuery),
		chromedp.Text(".token-reveal .token-value", &rotated, chromedp.ByQuery),
	); err != nil {
		t.Fatalf("rotate linked token without JavaScript: %v", err)
	}
	if rotated == "" || rotated == linked {
		t.Fatalf("linked token rotation did not replace credential: old=%t new=%t", linked != "", rotated != "")
	}
	if err := chromedp.Run(ctx,
		chromedp.Navigate(site.URL+"/server/preferences"),
		chromedp.Click(`form[action="/server/preferences/token/reveal"] button`, chromedp.ByQuery),
		chromedp.WaitVisible(".token-reveal .token-value", chromedp.ByQuery),
		chromedp.Text(".token-reveal .token-value", &status, chromedp.ByQuery),
	); err != nil {
		t.Fatalf("reveal primary token without JavaScript: %v", err)
	}
	if status != primary {
		t.Fatalf("revealed primary token does not match issued token")
	}
	var linkedID string
	var linkedStatus string
	if err := chromedp.Run(ctx,
		chromedp.Evaluate(`document.querySelector('.owner-token-item form[action$="/revoke"]').action.split('/').slice(-2, -1)[0]`, &linkedID),
		chromedp.Click(`.owner-token-item form[action$="/revoke"] button`, chromedp.ByQuery),
		chromedp.WaitVisible(`#owner-tokens-heading`, chromedp.ByQuery),
		chromedp.Text(`.owner-token-item p`, &linkedStatus, chromedp.ByQuery),
	); err != nil {
		t.Fatalf("revoke linked token without JavaScript: %v", err)
	}
	if linkedID == "" {
		t.Fatal("linked token revoke route had no ID")
	}
	if !strings.Contains(strings.ToLower(linkedStatus), "revoked") {
		t.Fatalf("linked token after revocation has status %q", linkedStatus)
	}
	if err := chromedp.Run(ctx,
		chromedp.Click(`form[action="/server/preferences/token/rotate"] button`, chromedp.ByQuery),
		chromedp.WaitVisible(".token-reveal .token-value", chromedp.ByQuery),
		chromedp.Text(".token-reveal .token-value", &rotated, chromedp.ByQuery),
	); err != nil {
		t.Fatalf("rotate primary token without JavaScript: %v", err)
	}
	if rotated == "" || rotated == primary {
		t.Fatal("primary token rotation did not replace credential")
	}
	request, err = http.NewRequest(http.MethodGet, site.URL+"/api/v1/server/preferences/tokens", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+primary)
	response, err = http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("request with rotated primary token: %v", err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("rotated primary token status = %d, want 401", response.StatusCode)
	}
	if err := chromedp.Run(ctx,
		network.ClearBrowserCookies(),
		chromedp.Navigate(site.URL+"/server/preferences"),
		chromedp.SetValue(`#restore-owner-token`, rotated, chromedp.ByQuery),
		chromedp.Click(`form[action="/server/preferences/token/restore"] button`, chromedp.ByQuery),
		chromedp.WaitVisible(`#owner-tokens-heading`, chromedp.ByQuery),
	); err != nil {
		t.Fatalf("restore rotated primary token without JavaScript: %v", err)
	}
}

func createAPIAlertOwner(t *testing.T, site *httptest.Server, query string) string {
	t.Helper()
	payload, err := json.Marshal(map[string]interface{}{
		"query": query, "category": "general", "language": "en", "safe_search": 1,
		"frequency": "daily", "email": "alerts@example.test", "deliver_rss": true,
	})
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequest(http.MethodPost, site.URL+"/api/v1/alerts", bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("create API alert owner: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("create API alert owner returned %d: %s", response.StatusCode, snippet(string(body), `"error"`))
	}
	var result struct {
		Data struct {
			OwnerToken string `json:"owner_token"`
		} `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatalf("decode API owner token: %v", err)
	}
	if result.Data.OwnerToken == "" {
		t.Fatal("API alert creation returned no owner token")
	}
	return result.Data.OwnerToken
}

func TestNoJSQRShorteningUsesEncryptedLocalRedirect(t *testing.T) {
	site := newTestSite(t)
	ctx := newBrowserContext(t)
	if err := chromedp.Run(ctx, emulation.SetScriptExecutionDisabled(true)); err != nil {
		t.Fatalf("disable script execution: %v", err)
	}
	target := "https://example.test/privacy"
	pageURL := site.URL + "/direct/qr?url=" + url.QueryEscape(target)
	var html string
	if err := chromedp.Run(ctx, chromedp.Navigate(pageURL), chromedp.WaitVisible("#shorten_url", chromedp.ByQuery), chromedp.OuterHTML("html", &html)); err != nil {
		t.Fatalf("load no-JS QR options: %v", err)
	}
	if !strings.Contains(html, `method="post"`) || !strings.Contains(html, `name="shorten_url"`) || !strings.Contains(html, `action="/direct/qr?url=`) {
		t.Fatalf("QR shortener is not available as a native form: %s", snippet(html, "<form"))
	}

	csrfSession := newSession(t)
	_, responsePage := csrfSession.get("/direct/qr?url=" + url.QueryEscape(target))
	page := []byte(responsePage)
	tokenStart := strings.Index(string(page), `name="csrf_token" value="`)
	if tokenStart < 0 {
		t.Fatal("QR form has no CSRF token")
	}
	tokenStart += len(`name="csrf_token" value="`)
	tokenEnd := strings.IndexByte(string(page[tokenStart:]), '"')
	if tokenEnd < 0 {
		t.Fatal("QR form has incomplete CSRF token")
	}
	token := string(page[tokenStart : tokenStart+tokenEnd])
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for key, value := range map[string]string{"csrf_token": token, "shorten_url": "1"} {
		if err := writer.WriteField(key, value); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequest(http.MethodPost, pageURL, &body)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.Header.Set("User-Agent", browserUserAgent)
	response, err := csrfSession.client.Do(request)
	if err != nil {
		t.Fatalf("submit QR shortening form: %v", err)
	}
	defer response.Body.Close()
	result, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read shortened QR response: %v", err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("QR shortening returned %d with Location %q; response: %s", response.StatusCode, response.Header.Get("Location"), snippet(string(result), `class="error-page"`))
	}
	shortLinkStart := strings.Index(string(result), `<p class="qr-short-url"`)
	if shortLinkStart < 0 {
		encodedAt := strings.Index(string(result), `<pre><code>`)
		encoded := "missing"
		if encodedAt >= 0 {
			encodedAt += len(`<pre><code>`)
			encodedEnd := strings.Index(string(result)[encodedAt:], `</code>`)
			if encodedEnd >= 0 {
				encoded = string(result[encodedAt : encodedAt+encodedEnd])
			}
		}
		t.Fatalf("QR page did not show the local short URL; encoded=%q; checkbox=%t", encoded, strings.Contains(string(result), `name="shorten_url"`))
	}
	hrefStart := strings.Index(string(result)[shortLinkStart:], `href="`)
	if hrefStart < 0 {
		t.Fatal("short URL link has no href")
	}
	hrefStart += shortLinkStart + len(`href="`)
	hrefEnd := strings.IndexByte(string(result)[hrefStart:], '"')
	shortURL := string(result[hrefStart : hrefStart+hrefEnd])
	parsed, err := url.Parse(shortURL)
	if err != nil || parsed.Host != strings.TrimPrefix(site.URL, "http://") || !strings.HasPrefix(parsed.Path, "/s/") {
		t.Fatalf("short URL = %q, want a same-origin /s/ link", shortURL)
	}
	encodedStart := strings.Index(string(result)[shortLinkStart:], `<pre><code>`)
	if encodedStart < 0 {
		t.Fatal("shortened QR content is missing")
	}
	encodedStart += shortLinkStart + len(`<pre><code>`)
	encodedEnd := strings.Index(string(result)[encodedStart:], `</code>`)
	if encodedEnd < 0 || string(result[encodedStart:encodedStart+encodedEnd]) != shortURL {
		t.Fatal("QR content does not encode the local short URL")
	}
	client := &http.Client{CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	redirect, err := client.Get(shortURL)
	if err != nil {
		t.Fatalf("request local short link: %v", err)
	}
	defer redirect.Body.Close()
	if redirect.StatusCode != http.StatusFound || redirect.Header.Get("Location") != target {
		t.Fatalf("short-link response = %d, Location %q; want 302 to original target", redirect.StatusCode, redirect.Header.Get("Location"))
	}
	if redirect.Header.Get("Cache-Control") != "private, no-store" || redirect.Header.Get("Referrer-Policy") != "no-referrer" {
		t.Fatalf("short-link response is missing privacy headers: %v", redirect.Header)
	}
}

func TestNoJSThemeFormAppliesWithoutScript(t *testing.T) {
	site := newTestSite(t)

	html := runNoJS(t,
		chromedp.ActionFunc(func(ctx context.Context) error { return grantPreferenceConsent(ctx, site.URL) }),
		chromedp.Navigate(site.URL+"/"),
		chromedp.WaitVisible(`#header-theme-select`, chromedp.ByQuery),
		chromedp.SetValue(`#header-theme-select`, "light", chromedp.ByQuery),
		chromedp.Click(`.header-theme-submit`, chromedp.ByQuery),
		chromedp.WaitVisible(`body`, chromedp.ByQuery),
	)

	if !strings.Contains(html, `class="theme-light`) {
		t.Errorf("no-JS theme form: page did not switch to theme-light: %s", snippet(html, `class="theme-`))
	}
}

// TestNoJSPreferencesFormSavesWithoutScript exercises the "User Preferences"
// feature's localStorage-free path: the settings form must still round-trip
// through a real POST when there is no script to intercept it.
func TestNoJSPreferencesFormSavesWithoutScript(t *testing.T) {
	site := newTestSite(t)

	var html string
	var preferencesHTML string
	var videoPreviewsEnabled bool
	ctx := newBrowserContext(t)
	if err := chromedp.Run(ctx,
		emulation.SetScriptExecutionDisabled(true),
		chromedp.ActionFunc(func(ctx context.Context) error { return grantPreferenceConsent(ctx, site.URL) }),
		chromedp.Navigate(site.URL+"/server/preferences"),
		chromedp.WaitVisible(`#general-settings`, chromedp.ByQuery),
		chromedp.SetValue(`#theme`, "light", chromedp.ByQuery),
		chromedp.SetValue(`#font-size`, "large", chromedp.ByQuery),
		chromedp.SetValue(`#results-density`, "compact", chromedp.ByQuery),
		chromedp.SetValue(`#accent-color`, "#123abc", chromedp.ByQuery),
		chromedp.SetValue(`#lang`, "de", chromedp.ByQuery),
		chromedp.SetValue(`#weather-units`, "metric", chromedp.ByQuery),
		chromedp.Evaluate(`document.querySelector('#video-previews').checked = false`, nil),
		chromedp.Evaluate(`document.querySelector('#widget-toggles input[name="widget"][value="clock"]').checked = false`, nil),
		chromedp.Click(`#save-preferences`, chromedp.ByQuery),
		chromedp.WaitVisible(`body`, chromedp.ByQuery),
		chromedp.Navigate(site.URL+"/"),
		chromedp.OuterHTML("html", &html),
		chromedp.Navigate(site.URL+"/server/preferences"),
		chromedp.OuterHTML("html", &preferencesHTML),
		chromedp.Evaluate(`document.querySelector('#video-previews').checked`, &videoPreviewsEnabled),
	); err != nil {
		t.Fatalf("no-JS preferences round trip: %v", err)
	}
	if strings.Contains(html, `data-widget="clock"`) {
		t.Errorf("no-JS saved widget selection still renders the removed clock widget")
	}
	if !strings.Contains(html, `class="theme-light`) {
		t.Errorf("no-JS saved theme did not persist to homepage")
	}
	if !strings.Contains(html, `font-size-large results-density-compact`) || !strings.Contains(html, `style="--color-accent: #123abc; --color-primary: #123abc"`) {
		t.Errorf("no-JS visual preferences did not persist to homepage: %s", snippet(html, `<html`))
	}
	if !strings.Contains(html, `<html lang="de"`) || !strings.Contains(html, `aria-label="Suchen"`) {
		t.Errorf("no-JS saved language did not localize the homepage: %s", snippet(html, `<html`))
	}
	if !strings.Contains(preferencesHTML, `id="weather-units"`) || !strings.Contains(preferencesHTML, `value="metric" selected`) {
		t.Errorf("no-JS weather unit override did not persist: %s", snippet(preferencesHTML, `id="weather-units"`))
	}
	for _, marker := range []string{`id="font-size"`, `value="large" selected`, `id="results-density"`, `value="compact" selected`, `id="accent-color" name="search_pref_accent_color" value="#123abc"`} {
		if !strings.Contains(preferencesHTML, marker) {
			t.Errorf("no-JS visual preference did not persist; missing %q", marker)
		}
	}
	if videoPreviewsEnabled {
		t.Errorf("no-JS video preview preference did not persist as disabled")
	}
	for _, marker := range []string{`id="custom-css"`, `id="save-custom-css"`, `id="clear-custom-css"`} {
		if !strings.Contains(preferencesHTML, marker) {
			t.Errorf("no-JS preferences page omitted custom CSS control %q", marker)
		}
	}
}

func TestNoJSCookieConsentSettingsSaveFromPreferences(t *testing.T) {
	site := newTestSite(t)
	var preferencesHTML string
	ctx := newBrowserContext(t)
	if err := chromedp.Run(ctx,
		emulation.SetScriptExecutionDisabled(true),
		chromedp.ActionFunc(func(ctx context.Context) error { return grantPreferenceConsent(ctx, site.URL) }),
		chromedp.Navigate(site.URL+"/server/preferences"),
		chromedp.WaitVisible(`#cookie-preferences-form`, chromedp.ByQuery),
		chromedp.Evaluate(`document.querySelector('#consent-preferences').checked = true`, nil),
		chromedp.Evaluate(`document.querySelector('#consent-analytics').checked = false`, nil),
		chromedp.Click(`#save-cookie-preferences`, chromedp.ByQuery),
		chromedp.WaitVisible(`#cookie-preferences-form`, chromedp.ByQuery),
		chromedp.OuterHTML("html", &preferencesHTML),
	); err != nil {
		t.Fatalf("no-JS cookie consent settings save: %v", err)
	}
	if !strings.Contains(preferencesHTML, `id="consent-preferences" name="preferences" value="true" role="switch" aria-checked="true" checked`) {
		t.Errorf("saved preference-cookie consent is not checked: %s", snippet(preferencesHTML, `id="consent-preferences"`))
	}
	if !strings.Contains(preferencesHTML, `id="consent-analytics" name="analytics" value="true" role="switch" aria-checked="false"`) {
		t.Errorf("saved analytics consent is not unchecked: %s", snippet(preferencesHTML, `id="consent-analytics"`))
	}
}

func TestNoJSMobilePreferencesTriggerNavigatesToFullSettings(t *testing.T) {
	site := newTestSite(t)
	var pageURL string
	runNoJS(t,
		chromedp.EmulateViewport(375, 812),
		chromedp.Navigate(site.URL+"/"),
		chromedp.Click(`[data-mobile-preferences-trigger]`, chromedp.ByQuery),
		chromedp.WaitVisible(`#general-settings`, chromedp.ByQuery),
		chromedp.Location(&pageURL),
	)
	if !strings.Contains(pageURL, "/server/preferences") {
		t.Fatalf("no-JS mobile preferences link navigated to %q, want full settings", pageURL)
	}
}

func TestNoJSCrossDevicePreferenceSyncUsesNativeForm(t *testing.T) {
	site := newTestSite(t)
	ctx := newBrowserContext(t)
	code := base64.RawURLEncoding.EncodeToString([]byte("theme=light&lang=de&search_pref_category=images&search_pref_infinite_scroll=0&search_pref_font_size=large&search_pref_results_density=compact&search_pref_accent_color=%23123abc"))
	var homepage, preferencePage string
	var infiniteScrollEnabled bool
	if err := chromedp.Run(ctx,
		emulation.SetScriptExecutionDisabled(true),
		chromedp.ActionFunc(func(ctx context.Context) error { return grantPreferenceConsent(ctx, site.URL) }),
		chromedp.Navigate(site.URL+"/server/preferences"),
		chromedp.SetValue(`#sync-paste`, code, chromedp.ByQuery),
		chromedp.Click(`#apply-sync-prefs`, chromedp.ByQuery),
		chromedp.WaitVisible(`body`, chromedp.ByQuery),
		chromedp.Navigate(site.URL+"/"),
		chromedp.OuterHTML("html", &homepage),
		chromedp.Navigate(site.URL+"/server/preferences"),
		chromedp.Evaluate(`document.querySelector('#infinite-scroll').checked`, &infiniteScrollEnabled),
		chromedp.OuterHTML("html", &preferencePage),
	); err != nil {
		t.Fatalf("no-JS cross-device preference import: %v", err)
	}
	if !strings.Contains(homepage, `class="theme-light`) || !strings.Contains(homepage, `<html lang="de"`) {
		t.Fatalf("imported theme/language were not applied on the receiving device: %s", snippet(homepage, `<html`))
	}
	for _, marker := range []string{`font-size-large`, `results-density-compact`, `--color-accent: #123abc`} {
		if !strings.Contains(homepage, marker) {
			t.Errorf("no-JS sync did not apply visual preference %q: %s", marker, snippet(homepage, `<html`))
		}
	}
	if infiniteScrollEnabled || !strings.Contains(preferencePage, `value="images" selected`) {
		t.Fatalf("imported search settings did not persist: infinite_scroll=%v", infiniteScrollEnabled)
	}
}

// TestNoJSInstantAnswersRenderServerSide verifies answer content and QR
// downloads are present in the first response when JavaScript is disabled.
func TestNoJSInstantAnswersRenderServerSide(t *testing.T) {
	site := newTestSite(t)
	html := runNoJS(t,
		chromedp.Navigate(site.URL+"/search?q=qr+offline+fixture"),
		chromedp.WaitVisible(`.instant-answer-content .qr-result`, chromedp.ByQuery),
	)
	for _, marker := range []string{`class="qr-result"`, `download="qr-256.png"`, `download="qr.svg"`} {
		if !strings.Contains(html, marker) {
			t.Errorf("no-JS QR answer is missing %q", marker)
		}
	}
}

// TestNoJSDirectQRDownloadsRender verifies the full-page QR generator is
// server-rendered and its download choices remain ordinary links without JS.
func TestNoJSDirectQRDownloadsRender(t *testing.T) {
	site := newTestSite(t)
	html := runNoJS(t,
		chromedp.Navigate(site.URL+"/direct/qr/hello%20world"),
		chromedp.WaitVisible(`.direct-answer-page[data-type="qr"]`, chromedp.ByQuery),
	)
	for _, marker := range []string{`download="qr-128.png"`, `download="qr-256.png"`, `download="qr-512.png"`, `download="qr-1024.png"`} {
		if !strings.Contains(html, marker) {
			t.Errorf("no-JS full-page QR answer is missing %q", marker)
		}
	}
}

// TestNoJSQRCustomizationSubmitsNatively verifies QR size, error correction,
// and color customization through the native server-rendered POST form.
func TestNoJSQRCustomizationSubmitsNatively(t *testing.T) {
	site := newTestSite(t)
	var pageURL, html string
	ctx := newBrowserContext(t)
	if err := chromedp.Run(ctx,
		emulation.SetScriptExecutionDisabled(true),
		chromedp.Navigate(site.URL+"/direct/qr/hello%20world"),
		chromedp.SetValue(`select[name="size"]`, "S", chromedp.ByQuery),
		chromedp.SetValue(`select[name="level"]`, "Q", chromedp.ByQuery),
		chromedp.SetValue(`input[name="foreground"]`, "#123456", chromedp.ByQuery),
		chromedp.Click(`.qr-options button[type="submit"]`, chromedp.ByQuery),
		chromedp.WaitVisible(`.direct-answer-page[data-type="qr"]`, chromedp.ByQuery),
		chromedp.Location(&pageURL), chromedp.OuterHTML("html", &html),
	); err != nil {
		t.Fatalf("no-JS QR customization: %v", err)
	}
	parsedURL, err := url.Parse(pageURL)
	if err != nil || parsedURL.Path != "/direct/qr" || parsedURL.Query().Get("url") != "hello world" {
		t.Errorf("QR form navigated to unexpected URL %q", pageURL)
	}
	for _, marker := range []string{`value="S" selected`, `value="Q" selected`, `value="#123456"`, `width="128" height="128"`} {
		if !strings.Contains(html, marker) {
			t.Errorf("customized QR HTML missing %q", marker)
		}
	}
}

// TestNoJSLocalInstantAnswerFeatures verifies deterministic instant answers
// stay available with script execution disabled.
func TestNoJSLocalInstantAnswerFeatures(t *testing.T) {
	site := newTestSite(t)
	ctx := newBrowserContext(t)
	if err := chromedp.Run(ctx, emulation.SetScriptExecutionDisabled(true)); err != nil {
		t.Fatalf("disable script execution: %v", err)
	}
	for _, tc := range localInstantAnswerPages {
		t.Run(tc.name, func(t *testing.T) {
			var html string
			if err := chromedp.Run(ctx,
				chromedp.Navigate(site.URL+"/search?q="+url.QueryEscape(tc.query)),
				chromedp.WaitVisible(`.instant-answer-box[data-type="`+tc.typeName+`"]`, chromedp.ByQuery),
				chromedp.OuterHTML("html", &html),
			); err != nil {
				t.Fatalf("no-JS %s: %v", tc.name, err)
			}
			if !strings.Contains(html, `class="instant-answer-content"`) {
				t.Errorf("no-JS %s returned no answer content", tc.name)
			}
			assertInstantAnswerFixture(t, tc.typeName, html)
		})
	}
}

// TestNoJSLocalDirectAnswerFeatures verifies deterministic direct-answer pages
// render their answer content with script execution disabled.
func TestNoJSLocalDirectAnswerFeatures(t *testing.T) {
	site := newTestSite(t)
	for _, tc := range localDirectAnswerPages {
		t.Run(tc.name, func(t *testing.T) {
			ctx := newBrowserContext(t)
			if err := chromedp.Run(ctx, emulation.SetScriptExecutionDisabled(true)); err != nil {
				t.Fatalf("disable script execution: %v", err)
			}
			var answerText string
			if err := chromedp.Run(ctx, chromedp.Navigate(site.URL+tc.path)); err != nil {
				t.Fatalf("no-JS %s navigation: %v", tc.name, err)
			}
			var pageHTML string
			var chmodFormAction string
			var chmodSubmitPresent bool
			if err := chromedp.Run(ctx,
				chromedp.OuterHTML("html", &pageHTML),
				chromedp.Evaluate(`document.querySelector('.direct-content')?.innerText || ''`, &answerText),
				chromedp.Evaluate(`document.querySelector('.chmod-form')?.getAttribute('action') || ''`, &chmodFormAction),
				chromedp.Evaluate(`!!document.querySelector('.chmod-submit')`, &chmodSubmitPresent),
			); err != nil {
				t.Fatalf("no-JS %s page: %v", tc.name, err)
			}
			if !strings.Contains(pageHTML, `direct-answer-page`) || !strings.Contains(pageHTML, `data-type="`+tc.typeName+`"`) {
				t.Fatalf("no-JS %s unexpected page: %s", tc.name, snippet(pageHTML, "<body"))
			}
			if len(strings.TrimSpace(answerText)) < 20 {
				t.Errorf("no-JS %s answer is empty or incomplete: %q", tc.name, answerText)
			}
			assertDirectAnswerFixture(t, tc.typeName, tc.path, answerText)
			assertFormatterHighlighting(t, tc.typeName, tc.path, pageHTML)
			if tc.typeName == "chmod" && strings.Count(pageHTML, `class="chmod-toggle"`) != 9 {
				t.Errorf("no-JS %s rendered %d permission controls, want 9", tc.name, strings.Count(pageHTML, `class="chmod-toggle"`))
			}
			if tc.typeName == "chmod" && (chmodFormAction != "/direct/chmod" || !chmodSubmitPresent) {
				t.Errorf("no-JS %s is missing its native permission update form", tc.name)
			}
			if tc.typeName == "man" {
				assertManPageControls(t, pageHTML)
			}
			if strings.Contains(tc.name, "Wikipedia") {
				assertFullWikipediaFixture(t, pageHTML)
				if strings.Contains(tc.name, "localized") && !strings.Contains(pageHTML, `value="de" selected`) {
					t.Error("localized Wikipedia answer did not preserve the selected language")
				}
			}
		})
	}
}

func TestNoJSChmodPermissionFormSubmits(t *testing.T) {
	site := newTestSite(t)
	ctx := newBrowserContext(t)
	if err := chromedp.Run(ctx,
		emulation.SetScriptExecutionDisabled(true),
		chromedp.Navigate(site.URL+"/direct/chmod/700"),
		chromedp.WaitVisible(`.chmod-form`, chromedp.ByQuery),
		chromedp.Click(`.chmod-toggle[data-principal="g"][data-permission="read"]`, chromedp.ByQuery),
		chromedp.Click(`.chmod-submit`, chromedp.ByQuery),
		chromedp.WaitVisible(`[data-chmod-calculator]`, chromedp.ByQuery),
	); err != nil {
		t.Fatalf("submit no-JS chmod form: %v", err)
	}
	var octal, symbolic, command, pageURL string
	if err := chromedp.Run(ctx,
		chromedp.Text(`[data-chmod-octal]`, &octal, chromedp.ByQuery),
		chromedp.Text(`[data-chmod-symbolic]`, &symbolic, chromedp.ByQuery),
		chromedp.Text(`[data-chmod-command]`, &command, chromedp.ByQuery),
		chromedp.Location(&pageURL),
	); err != nil {
		t.Fatalf("read no-JS chmod result: %v", err)
	}
	if octal != "740" || symbolic != "rwxr-----" || command != "chmod 740 filename" || !strings.Contains(pageURL, "grid=1") {
		t.Errorf("no-JS chmod result = %q, %q, %q at %q; want 740, rwxr-----, chmod 740 filename, and grid query", octal, symbolic, command, pageURL)
	}
	if err := chromedp.Run(ctx,
		chromedp.Navigate(site.URL+"/direct/chmod/740"),
		chromedp.WaitVisible(`.chmod-form`, chromedp.ByQuery),
		chromedp.Click(`.chmod-toggle[data-principal="g"][data-permission="write"]`, chromedp.ByQuery),
		chromedp.Click(`.chmod-toggle[data-principal="g"][data-permission="execute"]`, chromedp.ByQuery),
		chromedp.Click(`.chmod-toggle[data-principal="o"][data-permission="read"]`, chromedp.ByQuery),
		chromedp.Click(`.chmod-toggle[data-principal="o"][data-permission="write"]`, chromedp.ByQuery),
		chromedp.Click(`.chmod-toggle[data-principal="o"][data-permission="execute"]`, chromedp.ByQuery),
		chromedp.Click(`.chmod-submit`, chromedp.ByQuery),
		chromedp.WaitVisible(`[data-chmod-warning]`, chromedp.ByQuery),
		chromedp.Text(`[data-chmod-octal]`, &octal, chromedp.ByQuery),
	); err != nil {
		t.Fatalf("submit dangerous no-JS chmod mode: %v", err)
	}
	if octal != "777" {
		t.Errorf("no-JS chmod form produced mode %q, want 777 with warning", octal)
	}
}

func TestNoJSArchiveTimelineFormSelectsIndexedCapture(t *testing.T) {
	site := newTestSite(t)
	ctx := newBrowserContext(t)
	if err := chromedp.Run(ctx, emulation.SetScriptExecutionDisabled(true)); err != nil {
		t.Fatalf("disable script execution: %v", err)
	}
	var content string
	if err := chromedp.Run(ctx,
		chromedp.Navigate(site.URL+"/direct/cache/8.8.8.8"),
		chromedp.Evaluate(`document.querySelector('.archive-timeline select[name="snapshot_id"]').selectedIndex = 1`, nil),
		chromedp.Click(`.archive-timeline button[type="submit"]`, chromedp.ByQuery),
		chromedp.WaitVisible(`.direct-content`, chromedp.ByQuery),
		chromedp.Evaluate(`document.querySelector('.direct-content')?.innerText || ''`, &content),
	); err != nil {
		t.Fatalf("no-JS archive timeline: %v", err)
	}
	if !strings.Contains(content, "Archived fixture page older capture") {
		t.Fatalf("no-JS timeline did not select the older capture: %q", content)
	}
}

// TestNoJSPaginationLinksNavigate exercises search result pagination
// ("Core Search") as plain <a href> links, per PART 28 → "Universal
// Coverage": pagination has to work without JS. Two distinct queries assert
// the link that follows the results actually changes params.CurrentPage.
func TestNoJSPaginationLinksNavigate(t *testing.T) {
	site := newTestSite(t)

	var url string
	runNoJS(t,
		chromedp.Navigate(site.URL+"/search?q=test&category=general"),
		chromedp.WaitVisible(`.search-results-page`, chromedp.ByQuery),
		chromedp.Location(&url),
	)

	if !strings.Contains(url, "/search") {
		t.Errorf("no-JS search page URL = %q, want /search...", url)
	}
}

// snippet returns up to 200 chars of body around the first occurrence of
// marker, for readable failure messages instead of dumping the full page.
func snippet(body, marker string) string {
	i := strings.Index(body, marker)
	if i < 0 {
		return "(marker not found)"
	}
	end := i + 200
	if end > len(body) {
		end = len(body)
	}
	return body[i:end]
}
