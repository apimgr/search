package server

import (
	"crypto/tls"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/apimgr/search/src/config"
)

// newBody returns a urlencoded request body. The consent handlers call
// r.ParseForm, so a POST built with httptest.NewRequest(nil) would parse to an
// empty form and the handler would take its "no decision" branch instead of the
// one under test.
func newBody(form string) io.ReadCloser {
	return io.NopCloser(strings.NewReader(form))
}

// tlsStub is a non-nil TLS state, which is what auto-mode reads to decide a
// request arrived over HTTPS. The contents are irrelevant to that check.
func tlsStub() *tls.ConnectionState {
	return &tls.ConnectionState{}
}

// TestSafeRedirectPath covers the same-site guard the consent,
// announcement-dismiss, and CCPA handlers run every redirect target through.
//
// The inputs that must collapse to "/" are the actual attack: each is a way to
// turn the post-consent redirect into an open redirect. A 303 to an
// attacker-chosen host immediately after the visitor posts on our own origin
// reads as a normal in-app confirmation, which is what makes it useful for
// phishing.
func TestSafeRedirectPath(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		// Ordinary same-site targets must survive untouched, query and
		// fragment included, or "back to the page I was on" stops working.
		{"root", "/", "/"},
		{"simple path", "/search", "/search"},
		{"path with query", "/search?q=golang&category=news", "/search?q=golang&category=news"},
		{"path with fragment", "/server/preferences#general", "/server/preferences#general"},
		{"path with encoded chars", "/direct/json/%7B%22a%22%3A1%7D", "/direct/json/%7B%22a%22%3A1%7D"},

		// Empty and whitespace are the "no Referer" case.
		{"empty", "", "/"},
		{"whitespace only", "   ", "/"},

		// Absolute URLs to another origin.
		{"https url", "https://evil.example/phish", "/"},
		{"http url", "http://evil.example/phish", "/"},
		{"url with same-site lookalike host", "https://search.example.com.evil.test/", "/"},

		// Protocol-relative references resolve against the current scheme
		// and therefore leave the origin just as effectively as a full URL.
		{"protocol relative", "//evil.example/phish", "/"},
		{"protocol relative bare host", "//evil.example", "/"},

		// Backslashes: several user agents normalize "\" to "/", which turns
		// "/\evil.example" into "//evil.example" at parse time — a same-origin
		// string that is cross-origin in the browser.
		{"backslash leading", "/\\evil.example", "/"},
		{"double backslash", "\\\\evil.example", "/"},
		{"backslash inside", "/search\\..\\..\\evil", "/"},

		// Non-http schemes, including javascript: and data:.
		{"javascript scheme", "javascript:alert(1)", "/"},
		{"data scheme", "data:text/html,<script>alert(1)</script>", "/"},
		{"mailto scheme", "mailto:victim@example.com", "/"},

		// Bare hosts and relative paths with no leading slash.
		{"bare host", "evil.example/phish", "/"},
		{"dot relative", "../evil", "/"},
		{"bare relative", "search", "/"},

		// Header injection: a CR or LF in the target would let a caller append
		// arbitrary response headers alongside Location.
		{"crlf injection", "/\r\nSet-Cookie: admin=1", "/"},
		{"newline only", "/\n", "/"},
		{"null byte", "/\x00evil", "/"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := safeRedirectPath(tt.input); got != tt.want {
				t.Errorf("safeRedirectPath(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestSafeRedirectPathNeverReturnsCrossOrigin is the invariant restated as a
// property rather than a list: whatever goes in, what comes out is always a
// path on this origin. A single missed case in the table above would still be
// an open redirect, so this checks the shape directly.
func TestSafeRedirectPathNeverReturnsCrossOrigin(t *testing.T) {
	inputs := []string{
		"https://evil.example",
		"//evil.example",
		"/\\evil.example",
		"\\\\evil.example",
		"javascript:alert(1)",
		"/\r\nLocation: https://evil.example",
		"data:text/html,x",
		"evil.example",
		"",
		"/",
		"/search?q=a+b",
	}
	for _, in := range inputs {
		got := safeRedirectPath(in)
		if got == "" {
			t.Errorf("safeRedirectPath(%q) returned an empty target, which Location would treat as this page", in)
		}
		if got[0] != '/' {
			t.Errorf("safeRedirectPath(%q) = %q, want a path starting with /", in, got)
		}
		if len(got) > 1 && got[1] == '/' {
			t.Errorf("safeRedirectPath(%q) = %q, which is protocol-relative and leaves the origin", in, got)
		}
	}
}

// TestConsentRedirectsAreSameSite drives the three handlers end to end and
// asserts the Location header cannot be pointed off-origin, which is the
// property the open redirect actually broke. The CSRF token is bypassed here by
// calling the handler directly, because the redirect target is what is under
// test, not the token check that already covers its own route.
func TestConsentRedirectsAreSameSite(t *testing.T) {
	s := &Server{config: &config.Config{}}

	tests := []struct {
		name    string
		handler http.HandlerFunc
		form    string
	}{
		{"consent", s.handleConsent, "choice=accept"},
		{"announcement dismiss", s.handleAnnouncementDismiss, "id=a1"},
		{"ccpa", s.handleConsentCCPA, "action=opt-out"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			form := tt.form + "&return_to=https%3A%2F%2Fevil.example%2Fphish"
			r := httptest.NewRequest(http.MethodPost, "/server/privacy", nil)
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			r.Header.Set("Referer", "https://evil.example/landing")
			r.Body = newBody(form)

			w := httptest.NewRecorder()
			tt.handler(w, r)

			if w.Code != http.StatusSeeOther {
				t.Fatalf("status = %d, want %d", w.Code, http.StatusSeeOther)
			}
			if loc := w.Header().Get("Location"); loc != "/" {
				t.Errorf("Location = %q, want %q (a tampered return_to and an off-origin Referer must both fall back)", loc, "/")
			}
		})
	}
}

// TestConsentRedirectHonorsSameSiteReturnTo is the other direction: a genuine
// return_to must still work, or the fix has broken the feature it was meant to
// protect. Without this a blanket "always redirect to /" would pass the test
// above while silently stranding every visitor on the homepage.
func TestConsentRedirectHonorsSameSiteReturnTo(t *testing.T) {
	s := &Server{config: &config.Config{}}

	form := "choice=decline&return_to=%2Fsearch%3Fq%3Dgolang%26category%3Dnews"
	r := httptest.NewRequest(http.MethodPost, "/server/privacy", nil)
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Body = newBody(form)

	w := httptest.NewRecorder()
	s.handleConsent(w, r)

	if loc := w.Header().Get("Location"); loc != "/search?q=golang&category=news" {
		t.Errorf("Location = %q, want the submitted same-site return_to", loc)
	}
}

// TestConsentCookieCarriesSecure checks the second half of the spec item: the
// consent cookies are set Secure when the request is secure, so a cookie the
// visitor just set over HTTPS is not replayed in the clear on a downgraded hop.
func TestConsentCookieCarriesSecure(t *testing.T) {
	s := &Server{config: &config.Config{}}
	s.config.Server.Security.CSRF.Secure = "true"

	r := httptest.NewRequest(http.MethodPost, "/server/consent", nil)
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Body = newBody("choice=accept")

	w := httptest.NewRecorder()
	s.handleConsent(w, r)

	found := false
	for _, c := range w.Result().Cookies() {
		if c.Name == "cookie_consent" {
			found = true
			if !c.Secure {
				t.Error("cookie_consent has Secure=false, want true when the server is configured for TLS")
			}
		}
	}
	if !found {
		t.Fatal("no cookie_consent cookie was set")
	}
}

// TestCookieSecureResolution checks the three branches of the setting that now
// governs every consent cookie.
func TestCookieSecureResolution(t *testing.T) {
	tests := []struct {
		name    string
		setting string
		tls     bool
		ssl     bool
		want    bool
	}{
		{"explicit true wins over plaintext", "true", false, false, true},
		{"explicit false wins over TLS", "false", true, true, false},
		{"auto follows the request scheme", "auto", true, false, true},
		{"auto follows the ssl setting", "auto", false, true, true},
		{"auto is off on a plain http server", "auto", false, false, false},
		{"unset behaves as auto", "", false, false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Server{config: &config.Config{}}
			s.config.Server.Security.CSRF.Secure = tt.setting
			s.config.Server.SSL.Enabled = tt.ssl

			r := httptest.NewRequest(http.MethodPost, "/server/consent", nil)
			if tt.tls {
				r.TLS = tlsStub()
			}
			if got := s.cookieSecure(r); got != tt.want {
				t.Errorf("cookieSecure(setting=%q, tls=%v, ssl=%v) = %v, want %v",
					tt.setting, tt.tls, tt.ssl, got, tt.want)
			}
		})
	}
}

// TestCookieSecureNilConfig guards the path where a Server was constructed
// without config, so a nil deref cannot take down the handler chain.
func TestCookieSecureNilConfig(t *testing.T) {
	var s *Server
	if s.cookieSecure(httptest.NewRequest(http.MethodPost, "/", nil)) {
		t.Error("cookieSecure on a nil-config server = true, want false")
	}
}
