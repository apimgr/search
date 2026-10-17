package server

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/apimgr/search/src/config"
)

func TestContactCaptchaVerifier(t *testing.T) {
	tests := []struct {
		name     string
		status   int
		body     string
		redirect bool
		want     bool
	}{
		{name: "accepted", status: http.StatusOK, body: `{"success":true}`, want: true},
		{name: "rejected", status: http.StatusOK, body: `{"success":false}`},
		{name: "invalid response", status: http.StatusOK, body: `{`},
		{name: "provider error", status: http.StatusServiceUnavailable, body: `{"success":true}`},
		{name: "redirect", status: http.StatusFound, redirect: true},
		{name: "oversized response", status: http.StatusOK, body: strings.Repeat("x", maxCaptchaResponseBytes+1)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
					t.Errorf("request = %s %s, content-type %q", r.Method, r.URL.Path, r.Header.Get("Content-Type"))
				}
				if err := r.ParseForm(); err != nil {
					t.Errorf("parse form: %v", err)
				}
				if r.Form.Get("secret") != "private-test-secret" || r.Form.Get("response") != "challenge-token" || r.Form.Get("remoteip") != "" {
					t.Errorf("provider form contains unexpected values: %#v", r.Form)
				}
				if test.redirect {
					http.Redirect(w, r, "/redirect-target", http.StatusFound)
					return
				}
				w.WriteHeader(test.status)
				_, _ = io.WriteString(w, test.body)
			}))
			defer server.Close()

			verifier := newContactCaptchaVerifier(server.Client(), server.URL, server.URL)
			if got := verifier.verify(context.Background(), "recaptcha", "private-test-secret", "challenge-token", ""); got != test.want {
				t.Fatalf("verify() = %t, want %t", got, test.want)
			}
		})
	}
}

func TestContactCaptchaVerifierRejectsMissingCredentialsAndUnknownProvider(t *testing.T) {
	verifier := newContactCaptchaVerifier(nil, "https://www.google.com/recaptcha/api/siteverify", "https://api.hcaptcha.com/siteverify")
	for _, test := range []struct {
		provider string
		secret   string
		token    string
	}{{"recaptcha", "", "token"}, {"recaptcha", "secret", ""}, {"unknown", "secret", "token"}} {
		if verifier.verify(context.Background(), test.provider, test.secret, test.token, "") {
			t.Fatalf("verify(%q, %q, %q) unexpectedly succeeded", test.provider, test.secret, test.token)
		}
	}
}

func TestConfiguredProviderUsesLocalCaptchaForOnionRequests(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Server.Pages.Contact.Captcha = "recaptcha"
	cfg.Server.Pages.Contact.CaptchaSiteKey = "public-site-key"
	cfg.Server.Pages.Contact.CaptchaSecret = "private-secret-key"
	s := &Server{config: cfg, startTime: time.Now()}
	challenge := s.signCaptcha(11)
	request := httptest.NewRequest(http.MethodPost, "http://search-example.onion/server/contact", nil)
	request.PostForm = url.Values{"captcha_id": {challenge}, "captcha": {"11"}}
	if !s.verifyConfiguredContactCaptcha(request) {
		t.Fatal("Tor request should use the built-in challenge instead of sending a verification request over clearnet")
	}
}

func TestContactCaptchaSignaturesSurviveServerRestart(t *testing.T) {
	cfg := config.DefaultConfig()
	first := &Server{config: cfg, startTime: time.Now()}
	challenge := first.signCaptcha(11)
	if !first.validCaptcha(challenge, "11") {
		t.Fatal("server rejected its own CAPTCHA challenge")
	}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/server/contact", nil)
	first.markContactCaptchaRequired(recorder, request)
	cookie := recorder.Result().Cookies()[0]
	second := &Server{config: cfg, startTime: time.Now().Add(time.Hour)}
	restartedRequest := httptest.NewRequest(http.MethodGet, "/server/contact", nil)
	restartedRequest.AddCookie(cookie)
	if !second.contactCaptchaRequired(restartedRequest) || !second.validCaptcha(challenge, "11") {
		t.Fatal("stable CAPTCHA signatures became invalid after a server restart")
	}
	cfg.Update(func(server *config.ServerConfig) {
		server.Security.InstallationSecret = "rotated-installation-secret"
	})
	if second.validCaptcha(challenge, "11") {
		t.Fatal("CAPTCHA challenge remained valid after its signing secret rotated")
	}
}

func TestContactCaptchaCSPAddsOnlySelectedProviderSources(t *testing.T) {
	base := "default-src 'self'; script-src 'self'; frame-src 'self'; connect-src 'self'"
	got := contactCaptchaCSP(base, "hcaptcha")
	for _, source := range []string{"https://js.hcaptcha.com", "https://hcaptcha.com", "https://*.hcaptcha.com"} {
		if !strings.Contains(got, source) {
			t.Errorf("CSP %q is missing %q", got, source)
		}
	}
	if gotSimple := contactCaptchaCSP(base, "simple"); gotSimple != base {
		t.Fatalf("simple CAPTCHA changed CSP: %q", gotSimple)
	}
}
