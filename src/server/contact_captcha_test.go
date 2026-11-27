package server

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/apimgr/search/src/config"
	"github.com/apimgr/search/src/database"
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
	s := &Server{config: cfg, dbManager: newTestServer(t).dbManager}
	request := httptest.NewRequest(http.MethodPost, "http://search-example.onion/server/contact", nil)
	id, answer := issueTestContactCaptcha(t, s, request)
	request.PostForm = url.Values{"captcha_id": {id}, "captcha_answer": {answer}}
	if !s.VerifyContactProof(request, "", id, answer) {
		t.Fatal("Tor request should use the built-in challenge instead of sending a verification request over clearnet")
	}
}

func TestConfiguredProviderAcceptsLocalFallbackOnClearnet(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Server.Pages.Contact.Captcha = "recaptcha"
	cfg.Server.Pages.Contact.CaptchaSiteKey = "public-site-key"
	cfg.Server.Pages.Contact.CaptchaSecret = "private-secret-key"
	s := &Server{config: cfg, dbManager: newTestServer(t).dbManager}
	request := httptest.NewRequest(http.MethodPost, "https://search.example/server/contact", nil)
	id, answer := issueTestContactCaptcha(t, s, request)
	if !s.VerifyContactProof(request, "", id, answer) {
		t.Fatal("configured provider should accept the issued local fallback challenge when no provider proof is submitted")
	}
}

func TestContactCaptchaChallengesAreHashedOneUseAndInRange(t *testing.T) {
	cfg := config.DefaultConfig()
	s := &Server{config: cfg, dbManager: newTestServer(t).dbManager}
	for range 20 {
		request := httptest.NewRequest(http.MethodGet, "/server/contact", nil)
		request.Header.Set("Accept-Language", "en")
		id, answer := issueTestContactCaptcha(t, s, request)
		if len(id) != 32 {
			t.Fatalf("challenge ID length = %d, want 32", len(id))
		}
		var storedHash string
		table := database.ServerTableName(s.dbManager.ServerDB(), "captcha_challenges")
		if err := s.dbManager.ServerDB().QueryRow(context.Background(), "SELECT answer_hash FROM "+table+" WHERE id = ?", id).Scan(&storedHash); err != nil {
			t.Fatalf("read stored challenge hash: %v", err)
		}
		if storedHash != captchaAnswerHash(answer) || storedHash == answer {
			t.Fatalf("stored CAPTCHA answer is not its SHA-256 hash")
		}
		if !s.VerifyContactChallenge(request, id, answer) {
			t.Fatal("server rejected a valid challenge answer")
		}
		if s.VerifyContactChallenge(request, id, answer) {
			t.Fatal("server accepted a challenge twice")
		}
	}
}

func TestContactCaptchaWrongAttemptConsumesChallenge(t *testing.T) {
	s := &Server{config: config.DefaultConfig(), dbManager: newTestServer(t).dbManager}
	request := httptest.NewRequest(http.MethodGet, "/server/contact", nil)
	request.Header.Set("Accept-Language", "en")
	id, answer := issueTestContactCaptcha(t, s, request)
	wrong := "0"
	if answer == wrong {
		wrong = "1"
	}
	if s.VerifyContactChallenge(request, id, wrong) {
		t.Fatal("server accepted an incorrect challenge answer")
	}
	if s.VerifyContactChallenge(request, id, answer) {
		t.Fatal("challenge was reusable after an incorrect attempt")
	}
}

func TestTokenCleanupRemovesExpiredCaptchaChallenges(t *testing.T) {
	s := newTestServer(t)
	id := "deadbeefdeadbeefdeadbeefdeadbeef"
	db := s.dbManager.ServerDB()
	if err := db.CreateCaptchaChallenge(context.Background(), id, strings.Repeat("a", 64), time.Now().UTC().Add(-time.Minute)); err != nil {
		t.Fatalf("create expired challenge: %v", err)
	}
	if err := s.createTaskHandlers().TokenCleanup(context.Background()); err != nil {
		t.Fatalf("token cleanup: %v", err)
	}
	table := database.ServerTableName(db, "captcha_challenges")
	var count int
	if err := db.QueryRow(context.Background(), "SELECT COUNT(*) FROM "+table+" WHERE id = ?", id).Scan(&count); err != nil {
		t.Fatalf("count expired challenge: %v", err)
	}
	if count != 0 {
		t.Fatalf("expired challenge count = %d, want 0", count)
	}
}

func issueTestContactCaptcha(t *testing.T, s *Server, request *http.Request) (string, string) {
	t.Helper()
	id, question := s.NewContactChallenge(request)
	if id == "" {
		t.Fatal("NewContactChallenge() returned an empty ID")
	}
	match := regexp.MustCompile(`What is ([0-9]+) ([+-]) ([0-9]+)\?`).FindStringSubmatch(question)
	if len(match) != 4 {
		t.Fatalf("translated CAPTCHA question = %q, want an English arithmetic question", question)
	}
	a, err := strconv.Atoi(match[1])
	if err != nil {
		t.Fatalf("parse first operand: %v", err)
	}
	b, err := strconv.Atoi(match[3])
	if err != nil {
		t.Fatalf("parse second operand: %v", err)
	}
	if a < 1 || a > 20 || b < 1 || b > 20 || (match[2] == "-" && b > a) {
		t.Fatalf("CAPTCHA operands violate range/nonnegative-result rule: %d %s %d", a, match[2], b)
	}
	answer := a + b
	if match[2] == "-" {
		answer = a - b
	}
	return id, strconv.Itoa(answer)
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
