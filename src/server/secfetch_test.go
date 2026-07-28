package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/apimgr/search/src/config"
)

// newSecFetchMiddleware builds a Middleware with a default config, then applies
// the supplied mutator so each test can toggle only the fields it exercises.
func newSecFetchMiddleware(mutate func(*config.Config)) *Middleware {
	cfg := config.DefaultConfig()
	if mutate != nil {
		mutate(cfg)
	}
	return NewMiddleware(cfg, nil)
}

// TestSecFetchMiddleware covers every reject and pass-through branch of the
// Sec-Fetch-* validation layer (AI.md PART 11).
func TestSecFetchMiddleware(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		path       string
		site       string
		mode       string
		dest       string
		authHeader string
		exempt     []string
		disable    bool
		wantStatus int
	}{
		{
			name:       "cross-site POST without bearer rejected",
			method:     http.MethodPost,
			path:       "/api/v1/alerts",
			site:       "cross-site",
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "cross-site POST with bearer allowed",
			method:     http.MethodPost,
			path:       "/api/v1/alerts",
			site:       "cross-site",
			authHeader: "Bearer sometoken",
			wantStatus: http.StatusOK,
		},
		{
			name:       "cross-site POST on exempt path allowed",
			method:     http.MethodPost,
			path:       "/api/v1/webhooks/github",
			site:       "cross-site",
			exempt:     []string{"/api/v1/webhooks/"},
			wantStatus: http.StatusOK,
		},
		{
			name:       "same-origin POST allowed",
			method:     http.MethodPost,
			path:       "/api/v1/alerts",
			site:       "same-origin",
			wantStatus: http.StatusOK,
		},
		{
			name:       "cross-site GET allowed (not state-changing)",
			method:     http.MethodGet,
			path:       "/api/v1/search",
			site:       "cross-site",
			wantStatus: http.StatusOK,
		},
		{
			name:       "navigate mode on api state-changer rejected",
			method:     http.MethodPost,
			path:       "/api/v1/alerts",
			site:       "same-origin",
			mode:       "navigate",
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "navigate mode on non-api path allowed",
			method:     http.MethodPost,
			path:       "/consent",
			site:       "same-origin",
			mode:       "navigate",
			wantStatus: http.StatusOK,
		},
		{
			name:       "navigate GET on api allowed (side-effect free)",
			method:     http.MethodGet,
			path:       "/api/v1/search",
			site:       "same-origin",
			mode:       "navigate",
			wantStatus: http.StatusOK,
		},
		{
			name:       "cross-site iframe embedding rejected",
			method:     http.MethodGet,
			path:       "/",
			site:       "cross-site",
			dest:       "iframe",
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "same-origin iframe allowed",
			method:     http.MethodGet,
			path:       "/",
			site:       "same-origin",
			dest:       "iframe",
			wantStatus: http.StatusOK,
		},
		{
			name:       "no sec-fetch headers pass through (legacy)",
			method:     http.MethodPost,
			path:       "/api/v1/alerts",
			wantStatus: http.StatusOK,
		},
		{
			name:       "validation disabled bypasses all checks",
			method:     http.MethodPost,
			path:       "/api/v1/alerts",
			site:       "cross-site",
			disable:    true,
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mw := newSecFetchMiddleware(func(c *config.Config) {
				if tt.disable {
					c.Server.Security.Headers.SecFetchValidation = false
				}
				if tt.exempt != nil {
					c.Server.Security.CSRF.ExemptPaths = tt.exempt
				}
			})

			handler := mw.SecFetch(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))

			req := httptest.NewRequest(tt.method, tt.path, nil)
			if tt.site != "" {
				req.Header.Set("Sec-Fetch-Site", tt.site)
			}
			if tt.mode != "" {
				req.Header.Set("Sec-Fetch-Mode", tt.mode)
			}
			if tt.dest != "" {
				req.Header.Set("Sec-Fetch-Dest", tt.dest)
			}
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}

			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("SecFetch() status = %d, want %d", rec.Code, tt.wantStatus)
			}
		})
	}
}

// TestIsCSRFExempt verifies exact and prefix matching against the exempt list.
func TestIsCSRFExempt(t *testing.T) {
	tests := []struct {
		name   string
		exempt []string
		path   string
		want   bool
	}{
		{"empty list", nil, "/api/v1/alerts", false},
		{"exact match", []string{"/webhook"}, "/webhook", true},
		{"prefix match", []string{"/webhook/"}, "/webhook/github", true},
		{"prefix match no trailing slash", []string{"/webhook"}, "/webhook/github", true},
		{"no match", []string{"/webhook"}, "/api/v1/alerts", false},
		{"empty entry skipped", []string{""}, "/anything", false},
		{"partial word not matched", []string{"/hook"}, "/hooked", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mw := newSecFetchMiddleware(func(c *config.Config) {
				c.Server.Security.CSRF.ExemptPaths = tt.exempt
			})
			if got := mw.isCSRFExempt(tt.path); got != tt.want {
				t.Errorf("isCSRFExempt(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

// TestSetClearSiteData verifies the Clear-Site-Data header composition for the
// cookie-include and executionContexts permutations (AI.md PART 11).
func TestSetClearSiteData(t *testing.T) {
	tests := []struct {
		name           string
		includeCookies bool
		execContexts   bool
		wantContains   []string
		wantAbsent     []string
	}{
		{
			name:           "default without cookies",
			includeCookies: false,
			execContexts:   false,
			wantContains:   []string{`"cache"`, `"storage"`},
			wantAbsent:     []string{`"cookies"`, `"executionContexts"`},
		},
		{
			name:           "with cookies",
			includeCookies: true,
			execContexts:   false,
			wantContains:   []string{`"cache"`, `"cookies"`, `"storage"`},
			wantAbsent:     []string{`"executionContexts"`},
		},
		{
			name:           "with execution contexts",
			includeCookies: true,
			execContexts:   true,
			wantContains:   []string{`"cache"`, `"cookies"`, `"storage"`, `"executionContexts"`},
			wantAbsent:     nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.DefaultConfig()
			cfg.Server.Security.Headers.ClearSiteData.ExecutionContexts = tt.execContexts
			s := &Server{config: cfg}

			rec := httptest.NewRecorder()
			s.setClearSiteData(rec, tt.includeCookies)

			got := rec.Header().Get("Clear-Site-Data")
			if got == "" {
				t.Fatal("setClearSiteData() set no Clear-Site-Data header")
			}
			for _, want := range tt.wantContains {
				if !strings.Contains(got, want) {
					t.Errorf("Clear-Site-Data = %q, missing %q", got, want)
				}
			}
			for _, absent := range tt.wantAbsent {
				if strings.Contains(got, absent) {
					t.Errorf("Clear-Site-Data = %q, should not contain %q", got, absent)
				}
			}
		})
	}
}
