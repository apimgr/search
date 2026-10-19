package server

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/apimgr/search/src/config"
)

// consentRequest builds a request carrying a cookie_consent value the way a
// browser would: the JSON payload percent-encoded, per AI.md PART 16.
func consentRequest(t *testing.T, analytics, preferences bool) *http.Request {
	t.Helper()
	body := `{"essential":true,"preferences":` + boolStr(preferences) + `,"analytics":` + boolStr(analytics) + `,"timestamp":1704067200}`
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.AddCookie(&http.Cookie{Name: "cookie_consent", Value: url.PathEscape(body)})
	return r
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func TestConsentFromRequest(t *testing.T) {
	tests := []struct {
		name    string
		cookie  *http.Cookie
		wantNil bool
		analyt  bool
		prefs   bool
	}{
		{name: "absent", wantNil: true},
		{name: "empty", cookie: &http.Cookie{Name: "cookie_consent", Value: ""}, wantNil: true},
		{
			name:   "accept all",
			cookie: &http.Cookie{Name: "cookie_consent", Value: url.PathEscape(`{"essential":true,"preferences":true,"analytics":true,"timestamp":1704067200}`)},
			analyt: true,
			prefs:  true,
		},
		{
			name:   "analytics accepted preferences declined",
			cookie: &http.Cookie{Name: "cookie_consent", Value: url.PathEscape(`{"essential":true,"preferences":false,"analytics":true,"timestamp":1704067200}`)},
			analyt: true,
		},
		{
			// Decline: analytics must never be unlocked.
			name:   "decline",
			cookie: &http.Cookie{Name: "cookie_consent", Value: url.PathEscape(`{"essential":true,"preferences":false,"analytics":false,"timestamp":1704067200}`)},
		},
		{
			// A corrupt cookie is treated as no consent, never as consent.
			name:    "corrupt json",
			cookie:  &http.Cookie{Name: "cookie_consent", Value: "not-json"},
			wantNil: true,
		},
		{
			// The pre-fix wire format: raw JSON with the quotes stripped by
			// net/http. Must not parse, so tracking stays off.
			name:    "legacy unquoted",
			cookie:  &http.Cookie{Name: "cookie_consent", Value: "{essential:true,analytics:true}"},
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.cookie != nil {
				r.AddCookie(tt.cookie)
			}
			got := consentFromRequest(r)
			if tt.wantNil {
				if got != nil {
					t.Fatalf("consentFromRequest = %+v, want nil", got)
				}
				return
			}
			if got == nil {
				t.Fatal("consentFromRequest = nil, want a parsed consent")
			}
			if !got.Essential {
				t.Error("Essential = false, want true")
			}
			if got.Analytics != tt.analyt {
				t.Errorf("Analytics = %v, want %v", got.Analytics, tt.analyt)
			}
			if got.Preferences != tt.prefs {
				t.Errorf("Preferences = %v, want %v", got.Preferences, tt.prefs)
			}
		})
	}
}

// TestConsentFromRequestNilRequest guards the nil-receiver path so a
// programmatic caller cannot panic the handler chain.
func TestConsentFromRequestNilRequest(t *testing.T) {
	if got := consentFromRequest(nil); got != nil {
		t.Errorf("consentFromRequest(nil) = %+v, want nil", got)
	}
}

func TestCanSetPreferenceCookie(t *testing.T) {
	tests := []struct {
		name string
		r    *http.Request
		want bool
	}{
		{name: "missing consent", r: httptest.NewRequest(http.MethodGet, "/", nil)},
		{name: "declined", r: consentRequest(t, false, false)},
		{name: "analytics only", r: consentRequest(t, true, false)},
		{name: "preferences allowed", r: consentRequest(t, false, true), want: true},
		{name: "nil request", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CanSetPreferenceCookie(tt.r); got != tt.want {
				t.Errorf("CanSetPreferenceCookie() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCheckTrackingAllowed(t *testing.T) {
	// Built directly rather than via NewServer: NewServer registers
	// Prometheus collectors on the default registry, so constructing more
	// than one in a test binary panics on duplicate registration. The
	// consent gate reads only s.config.
	s := &Server{config: &config.Config{}}
	s.config.Server.Tracking = config.TrackingConfig{Type: "plausible", ID: "example.com"}

	tests := []struct {
		name string
		r    *http.Request
		want bool
	}{
		{name: "no consent cookie", r: httptest.NewRequest(http.MethodGet, "/", nil), want: false},
		{name: "declined", r: consentRequest(t, false, false), want: false},
		{name: "accepted", r: consentRequest(t, true, true), want: true},
		{name: "analytics only", r: consentRequest(t, true, false), want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := s.CheckTrackingAllowed(tt.r); got != tt.want {
				t.Errorf("CheckTrackingAllowed = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestCheckTrackingAllowedNilConfig guards the path where a Server was built
// without config, which must not panic the handler chain.
func TestCheckTrackingAllowedNilConfig(t *testing.T) {
	var s *Server
	if s.CheckTrackingAllowed(consentRequest(t, true, true)) {
		t.Error("CheckTrackingAllowed on a nil-config server = true, want false")
	}
}

// TestCheckTrackingAllowedProviderUnset is the other half of the spec's
// condition: consent alone is not enough, a provider must be configured.
func TestCheckTrackingAllowedProviderUnset(t *testing.T) {
	for _, typ := range []string{"", "none"} {
		s := &Server{config: &config.Config{}}
		s.config.Server.Tracking = config.TrackingConfig{Type: typ, ID: "example.com"}
		r := consentRequest(t, true, true)
		if s.CheckTrackingAllowed(r) {
			t.Errorf("tracking.type = %q: CheckTrackingAllowed = true, want false", typ)
		}
	}
}

func TestTrackingScriptProviders(t *testing.T) {
	tests := []struct {
		name      string
		cfg       config.TrackingConfig
		wantEmpty bool
		wantIn    []string
	}{
		{
			name:      "disabled",
			cfg:       config.TrackingConfig{Type: "none"},
			wantEmpty: true,
		},
		{
			name:      "empty type",
			cfg:       config.TrackingConfig{},
			wantEmpty: true,
		},
		{
			name:      "unknown provider emits nothing",
			cfg:       config.TrackingConfig{Type: "bogus", ID: "x"},
			wantEmpty: true,
		},
		{
			name:   "google ga4",
			cfg:    config.TrackingConfig{Type: "google", ID: "G-ABC123"},
			wantIn: []string{"googletagmanager.com/gtag/js?id=G-ABC123", "gtag('config', 'G-ABC123')"},
		},
		{
			name:   "google universal analytics",
			cfg:    config.TrackingConfig{Type: "google", ID: "UA-1234-1"},
			wantIn: []string{"google-analytics.com/analytics.js", "ga('create', 'UA-1234-1', 'auto')"},
		},
		{
			name:   "matomo",
			cfg:    config.TrackingConfig{Type: "matomo", ID: "1", URL: "https://matomo.example.com/"},
			wantIn: []string{`var u="https://matomo.example.com/";`, "matomo.php", "setSiteId', '1'"},
		},
		{
			name:   "piwik shares the matomo branch",
			cfg:    config.TrackingConfig{Type: "piwik", ID: "2", URL: "https://piwik.example.com"},
			wantIn: []string{"https://piwik.example.com/", "setSiteId', '2'"},
		},
		{
			name:   "owa",
			cfg:    config.TrackingConfig{Type: "owa", ID: "3", URL: "https://owa.example.com"},
			wantIn: []string{"owa_baseUrl = 'https://owa.example.com/'", "owa.tracker-combined-min.js"},
		},
		{
			name:   "fathom hosted",
			cfg:    config.TrackingConfig{Type: "fathom", ID: "ABCDEFGH"},
			wantIn: []string{"cdn.usefathom.com/script.js", `data-site="ABCDEFGH"`},
		},
		{
			name:   "fathom self-hosted lite",
			cfg:    config.TrackingConfig{Type: "fathom", ID: "ABCDEFGH", URL: "https://fs.example.com"},
			wantIn: []string{"https://fs.example.com/tracker.js"},
		},
		{
			name:   "plausible hosted",
			cfg:    config.TrackingConfig{Type: "plausible", ID: "example.com"},
			wantIn: []string{"plausible.io/js/script.js", `data-domain="example.com"`},
		},
		{
			name:   "plausible self-hosted",
			cfg:    config.TrackingConfig{Type: "plausible", ID: "example.com", URL: "https://pl.example.com"},
			wantIn: []string{"https://pl.example.com/js/script.js"},
		},
		{
			name:   "umami",
			cfg:    config.TrackingConfig{Type: "umami", ID: "uuid-1", URL: "https://umami.example.com"},
			wantIn: []string{"https://umami.example.com/script.js", `data-website-id="uuid-1"`},
		},
		{
			name:   "simple",
			cfg:    config.TrackingConfig{Type: "simple"},
			wantIn: []string{"scripts.simpleanalyticscdn.com/latest.js"},
		},
		{
			name:   "cloudflare",
			cfg:    config.TrackingConfig{Type: "cloudflare", ID: "cf-token"},
			wantIn: []string{"static.cloudflareinsights.com/beacon.min.js", "cf-token"},
		},
		// Missing required values must produce nothing rather than a snippet
		// that would fetch a tracker with no destination.
		{name: "matomo without url", cfg: config.TrackingConfig{Type: "matomo", ID: "1"}, wantEmpty: true},
		{name: "matomo without id", cfg: config.TrackingConfig{Type: "matomo", URL: "https://m.example.com"}, wantEmpty: true},
		{name: "umami without url", cfg: config.TrackingConfig{Type: "umami", ID: "u"}, wantEmpty: true},
		{name: "owa without url", cfg: config.TrackingConfig{Type: "owa", ID: "3"}, wantEmpty: true},
		{name: "google without id", cfg: config.TrackingConfig{Type: "google"}, wantEmpty: true},
		{name: "cloudflare without id", cfg: config.TrackingConfig{Type: "cloudflare"}, wantEmpty: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := string(TrackingScript(tt.cfg))
			if tt.wantEmpty {
				if got != "" {
					t.Fatalf("TrackingScript = %q, want empty", got)
				}
				return
			}
			if got == "" {
				t.Fatal("TrackingScript = empty, want markup")
			}
			for _, want := range tt.wantIn {
				if !strings.Contains(got, want) {
					t.Errorf("TrackingScript = %q, want it to contain %q", got, want)
				}
			}
		})
	}
}

// TestTrackingScriptEscapesConfig covers the template.HTML injection: the
// values come from an operator config file, and the gate must not be defeated
// by a value that breaks out of its attribute or JS string.
func TestTrackingScriptEscapesConfig(t *testing.T) {
	got := string(TrackingScript(config.TrackingConfig{
		Type: "plausible",
		ID:   `x" onload="alert(1)`,
		URL:  "https://pl.example.com",
	}))
	if strings.Contains(got, `onload="alert(1)"`) && !strings.Contains(got, `&#34;`) {
		t.Errorf("plausible site id was not escaped: %q", got)
	}

	gotJS := string(TrackingScript(config.TrackingConfig{
		Type: "google",
		ID:   "G-';alert(1);//",
	}))
	if strings.Contains(gotJS, "'G-';alert(1);//'") {
		t.Errorf("google measurement id was not JS-escaped: %q", gotJS)
	}
}
