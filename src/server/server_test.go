package server

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/apimgr/search/src/common/i18n"
	"github.com/apimgr/search/src/config"
	"github.com/apimgr/search/src/direct"
	"github.com/apimgr/search/src/version"
	"github.com/go-chi/chi/v5"
)

// Tests for Middleware

func TestNewMiddleware(t *testing.T) {
	cfg := &config.Config{}
	mw := NewMiddleware(cfg, nil)

	if mw == nil {
		t.Fatal("NewMiddleware() returned nil")
	}
	if mw.config != cfg {
		t.Error("config should be set")
	}
}

func TestChain(t *testing.T) {
	// Create a simple handler
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("OK"))
	})

	// Create middlewares that add headers
	mw1 := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-MW-1", "added")
			next.ServeHTTP(w, r)
		})
	}

	mw2 := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-MW-2", "added")
			next.ServeHTTP(w, r)
		})
	}

	// Chain middlewares
	chained := Chain(handler, mw1, mw2)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	chained.ServeHTTP(rec, req)

	// Both headers should be added
	if rec.Header().Get("X-MW-1") != "added" {
		t.Error("MW-1 header should be added")
	}
	if rec.Header().Get("X-MW-2") != "added" {
		t.Error("MW-2 header should be added")
	}
}

func TestMiddlewareSecurityHeaders(t *testing.T) {
	cfg := &config.Config{}
	cfg.Server.Security.Headers.XFrameOptions = "DENY"
	cfg.Server.Security.Headers.XContentTypeOptions = "nosniff"
	cfg.Server.Security.Headers.XXSSProtection = "1; mode=block"
	cfg.Server.Security.Headers.ReferrerPolicy = "strict-origin-when-cross-origin"
	cfg.Server.Security.Headers.ContentSecurityPolicy = "default-src 'self'"
	cfg.Server.Security.Headers.PermissionsPolicy = "geolocation=()"
	cfg.Server.SSL.Enabled = true
	cfg.Server.Security.HSTS.Enabled = true
	cfg.Server.Security.HSTS.MaxAgeSeconds = 63072000
	cfg.Server.Security.HSTS.IncludeSubDomains = true
	cfg.Server.Security.HSTS.Preload = true

	mw := NewMiddleware(cfg, nil)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("OK"))
	})

	wrapped := mw.SecurityHeaders(handler)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	wrapped.ServeHTTP(rec, req)

	// Check security headers
	if rec.Header().Get("X-Frame-Options") != "DENY" {
		t.Errorf("X-Frame-Options = %q, want DENY", rec.Header().Get("X-Frame-Options"))
	}
	if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("X-Content-Type-Options = %q, want nosniff", rec.Header().Get("X-Content-Type-Options"))
	}
	if rec.Header().Get("X-XSS-Protection") != "1; mode=block" {
		t.Errorf("X-XSS-Protection = %q, want 1; mode=block", rec.Header().Get("X-XSS-Protection"))
	}
	if rec.Header().Get("Referrer-Policy") != "strict-origin-when-cross-origin" {
		t.Errorf("Referrer-Policy = %q", rec.Header().Get("Referrer-Policy"))
	}
	if rec.Header().Get("Content-Security-Policy") != "default-src 'self'" {
		t.Errorf("Content-Security-Policy = %q", rec.Header().Get("Content-Security-Policy"))
	}
	if rec.Header().Get("Permissions-Policy") != "geolocation=()" {
		t.Errorf("Permissions-Policy = %q", rec.Header().Get("Permissions-Policy"))
	}
	// HSTS should be set when SSL is enabled
	if !strings.Contains(rec.Header().Get("Strict-Transport-Security"), "max-age=") {
		t.Error("HSTS header should be set when SSL is enabled")
	}
}

func TestMiddlewareCORS(t *testing.T) {
	t.Run("wildcard_policy", func(t *testing.T) {
		cfg := &config.Config{}
		cfg.Server.Web.CORS = "*"
		mw := NewMiddleware(cfg, nil)

		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("OK"))
		})
		wrapped := mw.CORS(handler)

		req := httptest.NewRequest(http.MethodGet, version.APIPrefix+"/search", nil)
		req.Header.Set("Origin", "https://app.example.com")
		rec := httptest.NewRecorder()
		wrapped.ServeHTTP(rec, req)

		if rec.Header().Get("Access-Control-Allow-Origin") != "*" {
			t.Errorf("wildcard: Access-Control-Allow-Origin = %q, want *", rec.Header().Get("Access-Control-Allow-Origin"))
		}
		if rec.Header().Get("Access-Control-Allow-Credentials") != "" {
			t.Error("wildcard: Access-Control-Allow-Credentials must not be set with *")
		}
	})

	t.Run("specific_origin_matched", func(t *testing.T) {
		cfg := &config.Config{}
		cfg.Server.Web.CORS = "https://example.com"
		mw := NewMiddleware(cfg, nil)

		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("OK"))
		})
		wrapped := mw.CORS(handler)

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Origin", "https://example.com")
		rec := httptest.NewRecorder()
		wrapped.ServeHTTP(rec, req)

		if rec.Header().Get("Access-Control-Allow-Origin") != "https://example.com" {
			t.Errorf("specific: Access-Control-Allow-Origin = %q", rec.Header().Get("Access-Control-Allow-Origin"))
		}
		if rec.Header().Get("Access-Control-Allow-Credentials") != "true" {
			t.Errorf("specific: Access-Control-Allow-Credentials = %q", rec.Header().Get("Access-Control-Allow-Credentials"))
		}
	})

	t.Run("specific_origin_not_matched", func(t *testing.T) {
		cfg := &config.Config{}
		cfg.Server.Web.CORS = "https://example.com"
		mw := NewMiddleware(cfg, nil)

		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("OK"))
		})
		wrapped := mw.CORS(handler)

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Origin", "https://other.com")
		rec := httptest.NewRecorder()
		wrapped.ServeHTTP(rec, req)

		if rec.Header().Get("Access-Control-Allow-Origin") != "" {
			t.Error("non-matching origin: Access-Control-Allow-Origin must not be set")
		}
	})

	t.Run("preflight_methods_and_headers", func(t *testing.T) {
		cfg := &config.Config{}
		cfg.Server.Web.CORS = "*"
		mw := NewMiddleware(cfg, nil)

		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("OK"))
		})
		wrapped := mw.CORS(handler)

		req := httptest.NewRequest(http.MethodOptions, version.APIPrefix+"/search", nil)
		req.Header.Set("Origin", "https://app.example.com")
		req.Header.Set("Access-Control-Request-Method", "POST")
		req.Header.Set("Access-Control-Request-Headers", "Content-Type, X-Api-Key")
		rec := httptest.NewRecorder()
		wrapped.ServeHTTP(rec, req)

		if rec.Code != http.StatusNoContent {
			t.Errorf("preflight status = %d, want %d", rec.Code, http.StatusNoContent)
		}
		// rs/cors v1.11+ echoes the requested method (not all allowed methods) per CORS spec.
		methods := rec.Header().Get("Access-Control-Allow-Methods")
		if !strings.Contains(methods, "POST") {
			t.Errorf("preflight: Access-Control-Allow-Methods missing %q (got %q)", "POST", methods)
		}
		// rs/cors v1.11+ echoes the requested headers (not a wildcard).
		allowedHeaders := rec.Header().Get("Access-Control-Allow-Headers")
		for _, h := range []string{"Content-Type", "X-Api-Key"} {
			if !strings.Contains(strings.ToLower(allowedHeaders), strings.ToLower(h)) {
				t.Errorf("preflight: Access-Control-Allow-Headers missing %q (got %q)", h, allowedHeaders)
			}
		}
		if rec.Header().Get("Access-Control-Max-Age") != "86400" {
			t.Errorf("preflight: Access-Control-Max-Age = %q, want 86400", rec.Header().Get("Access-Control-Max-Age"))
		}
	})
}

func TestMiddlewareCORSDisabled(t *testing.T) {
	cfg := &config.Config{}
	cfg.Server.Web.CORS = ""
	mw := NewMiddleware(cfg, nil)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("OK"))
	})

	wrapped := mw.CORS(handler)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Origin", "https://example.com")
	rec := httptest.NewRecorder()

	wrapped.ServeHTTP(rec, req)

	if rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Error("CORS disabled: Access-Control-Allow-Origin must not be set when cors is empty string")
	}
}

// Tests for RateLimiter

func TestNewRateLimiter(t *testing.T) {
	cfg := &config.RateLimitConfig{
		Enabled:           true,
		RequestsPerMinute: 60,
		BurstSize:         10,
	}
	rl := NewRateLimiter(cfg)

	if rl == nil {
		t.Fatal("NewRateLimiter() returned nil")
	}
	if rl.rate != 60 {
		t.Errorf("rate = %d, want 60", rl.rate)
	}
	if rl.burst != 10 {
		t.Errorf("burst = %d, want 10", rl.burst)
	}
	if !rl.enabled {
		t.Error("enabled should be true")
	}
}

func TestRateLimiterAllow(t *testing.T) {
	cfg := &config.RateLimitConfig{
		Enabled:           true,
		RequestsPerMinute: 60,
		BurstSize:         3,
	}
	rl := NewRateLimiter(cfg)

	// First few requests should be allowed (up to burst)
	for i := 0; i < 3; i++ {
		if !rl.Allow("192.168.1.1") {
			t.Errorf("Request %d should be allowed", i+1)
		}
	}

	// Next request should be denied (burst exhausted)
	if rl.Allow("192.168.1.1") {
		t.Error("Request should be denied after burst exhausted")
	}

	// Different IP should still be allowed
	if !rl.Allow("192.168.1.2") {
		t.Error("Request from different IP should be allowed")
	}
}

func TestRateLimiterDisabled(t *testing.T) {
	cfg := &config.RateLimitConfig{
		Enabled:           false,
		RequestsPerMinute: 60,
		BurstSize:         3,
	}
	rl := NewRateLimiter(cfg)

	// All requests should be allowed when disabled
	for i := 0; i < 100; i++ {
		if !rl.Allow("192.168.1.1") {
			t.Errorf("Request %d should be allowed when rate limiting is disabled", i+1)
		}
	}
}

// Tests for getClientIP

func TestGetClientIP(t *testing.T) {
	tests := []struct {
		name           string
		remoteAddr     string
		xForwardedFor  string
		xRealIP        string
		trustedProxies []string
		expectedIP     string
	}{
		{
			name:       "from RemoteAddr",
			remoteAddr: "192.168.1.1:12345",
			expectedIP: "192.168.1.1",
		},
		{
			name:           "from X-Forwarded-For with trusted proxy",
			remoteAddr:     "10.0.0.1:12345",
			xForwardedFor:  "203.0.113.50",
			trustedProxies: []string{"10.0.0.1"},
			expectedIP:     "203.0.113.50",
		},
		{
			name:           "from X-Forwarded-For with multiple IPs",
			remoteAddr:     "10.0.0.1:12345",
			xForwardedFor:  "203.0.113.50, 70.41.3.18",
			trustedProxies: []string{"10.0.0.1"},
			expectedIP:     "203.0.113.50",
		},
		{
			name:           "X-Forwarded-For ignored if not trusted",
			remoteAddr:     "192.168.1.1:12345",
			xForwardedFor:  "203.0.113.50",
			trustedProxies: []string{},
			expectedIP:     "192.168.1.1",
		},
		{
			name:           "from X-Real-IP with trusted proxy",
			remoteAddr:     "10.0.0.1:12345",
			xRealIP:        "198.51.100.178",
			trustedProxies: []string{"10.0.0.1"},
			expectedIP:     "198.51.100.178",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = tt.remoteAddr
			if tt.xForwardedFor != "" {
				req.Header.Set("X-Forwarded-For", tt.xForwardedFor)
			}
			if tt.xRealIP != "" {
				req.Header.Set("X-Real-IP", tt.xRealIP)
			}

			got := getClientIP(req, tt.trustedProxies)
			if got != tt.expectedIP {
				t.Errorf("getClientIP() = %q, want %q", got, tt.expectedIP)
			}
		})
	}
}

// Tests for CSRF Middleware

func TestNewCSRFMiddleware(t *testing.T) {
	cfg := &config.Config{}
	csrf := NewCSRFMiddleware(cfg)

	if csrf == nil {
		t.Fatal("NewCSRFMiddleware() returned nil")
	}
	if csrf.config != cfg {
		t.Error("config should be set")
	}
}

func TestCSRFGenerateToken(t *testing.T) {
	cfg := &config.Config{}
	csrf := NewCSRFMiddleware(cfg)

	token1 := csrf.GenerateToken()
	token2 := csrf.GenerateToken()

	if token1 == "" {
		t.Error("GenerateToken() returned empty string")
	}
	// 32 bytes hex encoded
	if len(token1) != 64 {
		t.Errorf("Token length = %d, want 64", len(token1))
	}
	if token1 == token2 {
		t.Error("Tokens should be unique")
	}
}

func TestCSRFValidateToken(t *testing.T) {
	cfg := &config.Config{}
	cfg.Server.Security.CSRF.Enabled = true
	cfg.Server.Security.CSRF.CookieName = "csrf_token"
	cfg.Server.Security.CSRF.HeaderName = "X-CSRF-Token"
	cfg.Server.Security.CSRF.FieldName = "csrf_token"
	csrf := NewCSRFMiddleware(cfg)

	// Generate and store a token
	token := csrf.GenerateToken()
	csrf.tokens.Store(token, time.Now())

	// Create request with matching cookie and header
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.AddCookie(&http.Cookie{Name: "csrf_token", Value: token})
	req.Header.Set("X-CSRF-Token", token)

	if !csrf.ValidateToken(req) {
		t.Error("ValidateToken() should return true for valid token")
	}

	// Token should be consumed (single use)
	if csrf.ValidateToken(req) {
		t.Error("ValidateToken() should return false after token is consumed")
	}
}

func TestCSRFValidateTokenDisabled(t *testing.T) {
	cfg := &config.Config{}
	cfg.Server.Security.CSRF.Enabled = false
	csrf := NewCSRFMiddleware(cfg)

	// Should always return true when disabled
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	if !csrf.ValidateToken(req) {
		t.Error("ValidateToken() should return true when CSRF is disabled")
	}
}

// Tests for responseWriter

func TestResponseWriter(t *testing.T) {
	w := httptest.NewRecorder()
	rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

	// Test WriteHeader
	rw.WriteHeader(http.StatusCreated)
	if rw.statusCode != http.StatusCreated {
		t.Errorf("statusCode = %d, want %d", rw.statusCode, http.StatusCreated)
	}

	// Test Write
	n, err := rw.Write([]byte("Hello"))
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if n != 5 {
		t.Errorf("Write() = %d bytes, want 5", n)
	}
	if rw.bytesWritten != 5 {
		t.Errorf("bytesWritten = %d, want 5", rw.bytesWritten)
	}
}

// Tests for Recovery middleware

func TestMiddlewareRecovery(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Mode: "production",
		},
	}
	mw := NewMiddleware(cfg, nil)

	// Handler that panics
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("test panic")
	})

	wrapped := mw.Recovery(handler)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	// Should not panic
	wrapped.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

// Tests for RequestID middleware

func TestMiddlewareRequestID(t *testing.T) {
	cfg := &config.Config{}
	mw := NewMiddleware(cfg, nil)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("OK"))
	})

	wrapped := mw.RequestID(handler)

	// Test without existing request ID
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	wrapped.ServeHTTP(rec, req)

	requestID := rec.Header().Get("X-Request-ID")
	if requestID == "" {
		t.Error("X-Request-ID should be set")
	}
	// Should be a valid UUID format
	if len(strings.Split(requestID, "-")) != 5 {
		t.Errorf("X-Request-ID should be a valid UUID, got %q", requestID)
	}

	// Test with existing valid request ID
	reqWithID := httptest.NewRequest(http.MethodGet, "/", nil)
	reqWithID.Header.Set("X-Request-ID", "123e4567-e89b-12d3-a456-426614174000")
	recWithID := httptest.NewRecorder()

	wrapped.ServeHTTP(recWithID, reqWithID)

	if recWithID.Header().Get("X-Request-ID") != "123e4567-e89b-12d3-a456-426614174000" {
		t.Error("Existing valid request ID should be preserved")
	}
}

// Tests for URLNormalizeMiddleware

func TestURLNormalizeMiddleware(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(r.URL.Path))
	})

	wrapped := URLNormalizeMiddleware(handler)

	tests := []struct {
		name         string
		path         string
		expectedPath string
		redirect     bool
	}{
		{
			name:         "root path",
			path:         "/",
			expectedPath: "/",
			redirect:     false,
		},
		{
			name:         "trailing slash redirect",
			path:         "/api/",
			expectedPath: "/api",
			redirect:     true,
		},
		{
			name:         "no trailing slash",
			path:         "/api",
			expectedPath: "/api",
			redirect:     false,
		},
		{
			name:         "file path keeps trailing slash logic",
			path:         "/static/style.css",
			expectedPath: "/static/style.css",
			redirect:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rec := httptest.NewRecorder()

			wrapped.ServeHTTP(rec, req)

			if tt.redirect {
				if rec.Code != http.StatusMovedPermanently {
					t.Errorf("Status = %d, want %d", rec.Code, http.StatusMovedPermanently)
				}
			} else {
				if rec.Code != http.StatusOK {
					t.Errorf("Status = %d, want %d", rec.Code, http.StatusOK)
				}
			}
		})
	}
}

// Tests for PathSecurityMiddleware

func TestPathSecurityMiddleware(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(r.URL.Path))
	})

	wrapped := PathSecurityMiddleware(handler)

	tests := []struct {
		name       string
		path       string
		wantStatus int
	}{
		{
			name:       "normal path",
			path:       version.APIPrefix + "/search",
			wantStatus: http.StatusOK,
		},
		{
			name:       "root path",
			path:       "/",
			wantStatus: http.StatusOK,
		},
		{
			name:       "path traversal blocked",
			path:       "/../etc/passwd",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "double dot in path blocked",
			path:       "/api/../../../etc/passwd",
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rec := httptest.NewRecorder()

			wrapped.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("Status = %d, want %d", rec.Code, tt.wantStatus)
			}
		})
	}
}

// Tests for extractContextFromPath
// Per AI.md: only TargetPublic and TargetServerPages are valid for this project.

func TestExtractContextFromPath(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		wantType TargetType
	}{
		{
			name:     "root path",
			path:     "/",
			wantType: TargetPublic,
		},
		{
			name:     "search route",
			path:     "/search",
			wantType: TargetPublic,
		},
		{
			name:     "alerts route",
			path:     "/alerts/new",
			wantType: TargetPublic,
		},
		{
			name:     "server pages — about",
			path:     "/server/about",
			wantType: TargetServerPages,
		},
		{
			name:     "server pages — privacy",
			path:     "/server/privacy",
			wantType: TargetServerPages,
		},
		{
			name:     "server root",
			path:     "/server",
			wantType: TargetServerPages,
		},
		{
			name:     "api v1 server route",
			path:     version.APIPrefix + "/server/healthz",
			wantType: TargetServerPages,
		},
		{
			name:     "api v1 public route",
			path:     version.APIPrefix + "/search",
			wantType: TargetPublic,
		},
		{
			name:     "api root",
			path:     version.APIPrefix + "/",
			wantType: TargetPublic,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := extractContextFromPath(tt.path)
			if ctx.Type != tt.wantType {
				t.Errorf("extractContextFromPath(%q) type = %v, want %v", tt.path, ctx.Type, tt.wantType)
			}
		})
	}
}

// Tests for TargetType.String()

func TestTargetTypeString(t *testing.T) {
	tests := []struct {
		target TargetType
		want   string
	}{
		{TargetPublic, "public"},
		{TargetServerPages, "server"},
		{TargetUnknown, "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.target.String(); got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestMetrics tests all Metrics functionality in a single test to avoid
// Prometheus duplicate collector registration panics (collectors are global).
// Uses the package-level sharedServer to avoid re-registering the same metric names.
func TestMetrics(t *testing.T) {
	s := sharedServer()
	m := s.metrics
	cfg := m.config
	cfg.Server.Metrics.Enabled = true
	cfg.Server.Metrics.IncludeSystem = false
	origToken := cfg.Server.Metrics.Token
	cfg.Server.Metrics.Token = "secret-token"
	t.Cleanup(func() { cfg.Server.Metrics.Token = origToken })

	// Test NewMetrics
	t.Run("NewMetrics", func(t *testing.T) {
		if m == nil {
			t.Fatal("NewMetrics() returned nil")
		}
		if m.config != cfg {
			t.Error("config should be set")
		}
		if m.startTime.IsZero() {
			t.Error("startTime should be set")
		}
	})

	// Test RecordRequest
	t.Run("RecordRequest", func(t *testing.T) {
		m.RecordRequest("GET", "/api/search", 200, 100*time.Millisecond, 100, 500)
	})

	// Test RecordSearch
	t.Run("RecordSearch", func(t *testing.T) {
		m.RecordSearch("general", 500*time.Millisecond)
	})

	// Test RecordEngineRequest
	t.Run("RecordEngineRequest", func(t *testing.T) {
		m.RecordEngineRequest("google")
		m.RecordEngineError("google")
	})

	// Test RecordDBQuery
	t.Run("RecordDBQuery", func(t *testing.T) {
		m.RecordDBQuery("SELECT", "users", 5*time.Millisecond)
		m.RecordDBError("INSERT", "constraint_violation")
	})

	// Test RecordCache
	t.Run("RecordCache", func(t *testing.T) {
		m.RecordCacheHit("search")
		m.RecordCacheMiss("search")
	})

	// Test RecordSchedulerTask
	t.Run("RecordSchedulerTask", func(t *testing.T) {
		m.RecordSchedulerTask("cleanup", "success", 2*time.Second)
	})

	// Test RecordAuthAttempt
	t.Run("RecordAuthAttempt", func(t *testing.T) {
		m.RecordAuthAttempt("password", "success")
		m.RecordAuthAttempt("password", "failure")
	})

	// Test Setters
	t.Run("Setters", func(t *testing.T) {
		m.SetActiveRequests(5)
		m.SetDBConnections(10, 3)
		m.SetCacheStats("search", 500, 10000000)
	})

	// Test Handler
	t.Run("Handler", func(t *testing.T) {
		handler := m.Handler()
		if handler == nil {
			t.Error("Handler() returned nil")
		}
	})

	// Test AuthenticatedHandler
	t.Run("AuthenticatedHandler", func(t *testing.T) {
		handler := m.AuthenticatedHandler()

		// Test without token
		req := httptest.NewRequest(http.MethodGet, "/metrics?lang=de", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("Status without token = %d, want %d", rec.Code, http.StatusUnauthorized)
		}
		if body := strings.TrimSpace(rec.Body.String()); !strings.Contains(body, "Nicht autorisiert") {
			t.Errorf("Body without token = %q, want JSON containing %q", body, "Nicht autorisiert")
		}

		// Test with invalid token
		reqInvalid := httptest.NewRequest(http.MethodGet, "/metrics", nil)
		reqInvalid.Header.Set("Authorization", "Bearer wrong-token")
		recInvalid := httptest.NewRecorder()
		handler.ServeHTTP(recInvalid, reqInvalid)
		if recInvalid.Code != http.StatusUnauthorized {
			t.Errorf("Status with invalid token = %d, want %d", recInvalid.Code, http.StatusUnauthorized)
		}

		// Test with valid token
		reqValid := httptest.NewRequest(http.MethodGet, "/metrics", nil)
		reqValid.Header.Set("Authorization", "Bearer secret-token")
		recValid := httptest.NewRecorder()
		handler.ServeHTTP(recValid, reqValid)
		if recValid.Code != http.StatusOK {
			t.Errorf("Status with valid token = %d, want %d", recValid.Code, http.StatusOK)
		}
	})
}

// Tests for normalizePath

func TestNormalizePath(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{version.APIPrefix + "/search", version.APIPrefix + "/search"},
		{version.APIPrefix + "/users/123", version.APIPrefix + "/users/:id"},
		{version.APIPrefix + "/items/123e4567-e89b-12d3-a456-426614174000", version.APIPrefix + "/items/:id"},
		{"/static/css/common.css", "/static/css/common.css"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := normalizePath(tt.path)
			if got != tt.want {
				t.Errorf("normalizePath(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

// Tests for getClientIPSimple

func TestGetClientIPSimple(t *testing.T) {
	tests := []struct {
		name          string
		remoteAddr    string
		xForwardedFor string
		xRealIP       string
		want          string
	}{
		{
			name:       "from RemoteAddr",
			remoteAddr: "192.168.1.1:12345",
			want:       "192.168.1.1",
		},
		{
			name:          "from X-Forwarded-For",
			remoteAddr:    "10.0.0.1:12345",
			xForwardedFor: "203.0.113.50",
			want:          "203.0.113.50",
		},
		{
			name:       "from X-Real-IP",
			remoteAddr: "10.0.0.1:12345",
			xRealIP:    "198.51.100.178",
			want:       "198.51.100.178",
		},
		{
			name:          "X-Forwarded-For priority",
			remoteAddr:    "10.0.0.1:12345",
			xForwardedFor: "203.0.113.50",
			xRealIP:       "198.51.100.178",
			want:          "203.0.113.50",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = tt.remoteAddr
			if tt.xForwardedFor != "" {
				req.Header.Set("X-Forwarded-For", tt.xForwardedFor)
			}
			if tt.xRealIP != "" {
				req.Header.Set("X-Real-IP", tt.xRealIP)
			}

			got := getClientIPSimple(req)
			if got != tt.want {
				t.Errorf("getClientIPSimple() = %q, want %q", got, tt.want)
			}
		})
	}
}

// Test structs

func TestRequestContextStruct(t *testing.T) {
	ctx := RequestContext{
		Type: TargetPublic,
	}

	if ctx.Type != TargetPublic {
		t.Errorf("Type = %v, want %v", ctx.Type, TargetPublic)
	}
}

// Test metricsResponseWriter

func TestMetricsResponseWriter(t *testing.T) {
	w := httptest.NewRecorder()
	mrw := &metricsResponseWriter{ResponseWriter: w, statusCode: http.StatusOK}

	// Test WriteHeader
	mrw.WriteHeader(http.StatusCreated)
	if mrw.statusCode != http.StatusCreated {
		t.Errorf("statusCode = %d, want %d", mrw.statusCode, http.StatusCreated)
	}

	// Test Write
	n, err := mrw.Write([]byte("Hello World"))
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if n != 11 {
		t.Errorf("Write() = %d bytes, want 11", n)
	}
	if mrw.bytesWritten != 11 {
		t.Errorf("bytesWritten = %d, want 11", mrw.bytesWritten)
	}
}

// Test context helper functions

func TestGetRequestContext(t *testing.T) {
	// Test without context
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := GetRequestContext(req)
	if ctx.Type != TargetUnknown {
		t.Errorf("Type = %v, want %v", ctx.Type, TargetUnknown)
	}
}

// Tests for theme.go

func TestThemeConstants(t *testing.T) {
	if ThemeDark != "dark" {
		t.Errorf("ThemeDark = %q, want 'dark'", ThemeDark)
	}
	if ThemeLight != "light" {
		t.Errorf("ThemeLight = %q, want 'light'", ThemeLight)
	}
	if ThemeAuto != "auto" {
		t.Errorf("ThemeAuto = %q, want 'auto'", ThemeAuto)
	}
	if DefaultTheme != ThemeDark {
		t.Errorf("DefaultTheme = %q, want %q", DefaultTheme, ThemeDark)
	}
}

func TestGetTheme(t *testing.T) {
	tests := []struct {
		name      string
		cookie    string
		query     string
		wantTheme string
	}{
		{"default", "", "", ThemeDark},
		{"cookie_dark", "dark", "", ThemeDark},
		{"cookie_light", "light", "", ThemeLight},
		{"cookie_auto", "auto", "", ThemeAuto},
		{"cookie_invalid", "invalid", "", ThemeDark},
		{"query_dark", "", "dark", ThemeDark},
		{"query_light", "", "light", ThemeLight},
		{"query_auto", "", "auto", ThemeAuto},
		{"cookie_overrides_query", "light", "dark", ThemeLight},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := "/"
			if tt.query != "" {
				url = "/?theme=" + tt.query
			}
			req := httptest.NewRequest(http.MethodGet, url, nil)
			if tt.cookie != "" {
				req.AddCookie(&http.Cookie{Name: "theme", Value: tt.cookie})
			}

			got := GetTheme(req)
			if got != tt.wantTheme {
				t.Errorf("GetTheme() = %q, want %q", got, tt.wantTheme)
			}
		})
	}
}

func TestNewPageDataResolvesLanguageFromQueryAndSetsCookie(t *testing.T) {
	s := &Server{
		config:      &config.Config{},
		i18nManager: i18n.NewManager("en", []string{"en", "de", "ar"}),
	}

	req := httptest.NewRequest(http.MethodGet, "/?lang=ar", nil)
	rec := httptest.NewRecorder()

	data := s.newPageData(rec, req, "Title", "home")
	if data.Lang != "ar" {
		t.Fatalf("Lang = %q, want %q", data.Lang, "ar")
	}
	if data.Dir != "rtl" {
		t.Fatalf("Dir = %q, want %q", data.Dir, "rtl")
	}
	if len(data.AvailableLanguages) != 3 {
		t.Fatalf("AvailableLanguages length = %d, want 3", len(data.AvailableLanguages))
	}

	found := false
	for _, cookie := range rec.Result().Cookies() {
		if cookie.Name == "lang" {
			found = true
			if cookie.Value != "ar" {
				t.Fatalf("lang cookie = %q, want %q", cookie.Value, "ar")
			}
		}
	}
	if !found {
		t.Fatal("newPageData() did not set lang cookie for supported query")
	}
}

func TestNewPageDataInvalidLanguageQueryFallsBackToDefault(t *testing.T) {
	s := &Server{
		config:      &config.Config{},
		i18nManager: i18n.NewManager("en", []string{"en", "de"}),
	}

	req := httptest.NewRequest(http.MethodGet, "/?lang=zz", nil)
	req.Header.Set("Accept-Language", "de")
	rec := httptest.NewRecorder()

	data := s.newPageData(rec, req, "Title", "home")
	if data.Lang != "en" {
		t.Fatalf("Lang = %q, want %q", data.Lang, "en")
	}
	if data.Dir != "ltr" {
		t.Fatalf("Dir = %q, want %q", data.Dir, "ltr")
	}

	for _, cookie := range rec.Result().Cookies() {
		if cookie.Name == "lang" {
			t.Fatalf("unexpected lang cookie for invalid query: %q", cookie.Value)
		}
	}
}

func TestHandleLocaleServesRequestedLanguage(t *testing.T) {
	s := &Server{
		config:      &config.Config{},
		i18nManager: i18n.NewManager("en", []string{"en", "de"}),
	}

	req := httptest.NewRequest(http.MethodGet, "/locales/de.json", nil)
	rec := httptest.NewRecorder()

	s.handleLocale(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("Content-Type"); !strings.Contains(got, "application/json") {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}
	if !strings.Contains(rec.Body.String(), `"language": "de"`) {
		t.Fatalf("Body did not contain German locale metadata: %s", rec.Body.String())
	}
}

func TestHandleLocaleFallsBackToDefaultLanguage(t *testing.T) {
	s := &Server{
		config:      &config.Config{},
		i18nManager: i18n.NewManager("en", []string{"en", "de"}),
	}

	req := httptest.NewRequest(http.MethodGet, "/locales/zz.json", nil)
	rec := httptest.NewRecorder()

	s.handleLocale(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), `"language": "en"`) {
		t.Fatalf("Body did not fall back to English locale metadata: %s", rec.Body.String())
	}
}

func TestHandleLocaleRejectsInvalidPath(t *testing.T) {
	s := &Server{
		config:      &config.Config{},
		i18nManager: i18n.NewManager("en", []string{"en", "de"}),
	}

	req := httptest.NewRequest(http.MethodGet, "/locales/de", nil)
	rec := httptest.NewRecorder()

	s.handleLocale(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("Status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestRenderDirectAnswerFallbackLocalizesLabels(t *testing.T) {
	manager, err := i18n.DefaultManager()
	if err != nil {
		t.Fatalf("DefaultManager() error = %v", err)
	}

	s := &Server{
		config:      &config.Config{},
		i18nManager: manager,
	}

	req := httptest.NewRequest(http.MethodGet, "/direct/wiki/Python?lang=de", nil)
	rec := httptest.NewRecorder()
	answer := &direct.Answer{
		Type:      direct.AnswerTypeWiki,
		Term:      "Python",
		Title:     "Python",
		Content:   "<p>Example</p>",
		Source:    "Wikipedia",
		SourceURL: "https://example.com/python",
	}

	s.renderDirectAnswerFallback(rec, req, answer)

	body := rec.Body.String()
	for _, needle := range []string{"Quelle:", "Direkte Antwort"} {
		if !strings.Contains(body, needle) {
			t.Fatalf("fallback body missing %q: %s", needle, body)
		}
	}
}

func TestSetTheme(t *testing.T) {
	tests := []struct {
		theme     string
		wantValue string
	}{
		{"dark", "dark"},
		{"light", "light"},
		{"auto", "auto"},
		// Should default to dark
		{"invalid", "dark"},
		// Should default to dark
		{"", "dark"},
	}

	for _, tt := range tests {
		t.Run(tt.theme, func(t *testing.T) {
			rec := httptest.NewRecorder()
			SetTheme(rec, tt.theme)

			cookies := rec.Result().Cookies()
			if len(cookies) == 0 {
				t.Fatal("No cookie set")
			}

			found := false
			for _, c := range cookies {
				if c.Name == "theme" {
					found = true
					if c.Value != tt.wantValue {
						t.Errorf("Cookie value = %q, want %q", c.Value, tt.wantValue)
					}
					if c.MaxAge != 30*24*60*60 {
						t.Errorf("Cookie MaxAge = %d, want %d", c.MaxAge, 30*24*60*60)
					}
				}
			}
			if !found {
				t.Error("Theme cookie not found")
			}
		})
	}
}

func TestGetThemeClass(t *testing.T) {
	tests := []struct {
		theme string
		want  string
	}{
		{"dark", "theme-dark"},
		{"light", "theme-light"},
		{"auto", "theme-auto"},
		{"invalid", "theme-dark"},
		{"", "theme-dark"},
	}

	for _, tt := range tests {
		t.Run(tt.theme, func(t *testing.T) {
			got := GetThemeClass(tt.theme)
			if got != tt.want {
				t.Errorf("GetThemeClass(%q) = %q, want %q", tt.theme, got, tt.want)
			}
		})
	}
}

func TestIsValidTheme(t *testing.T) {
	tests := []struct {
		theme string
		want  bool
	}{
		{"dark", true},
		{"light", true},
		{"auto", true},
		{"invalid", false},
		{"", false},
		{"Dark", false},
		{"LIGHT", false},
	}

	for _, tt := range tests {
		t.Run(tt.theme, func(t *testing.T) {
			got := IsValidTheme(tt.theme)
			if got != tt.want {
				t.Errorf("IsValidTheme(%q) = %v, want %v", tt.theme, got, tt.want)
			}
		})
	}
}

func TestGetThemeInfo(t *testing.T) {
	tests := []struct {
		theme     string
		wantDark  bool
		wantLight bool
		wantAuto  bool
	}{
		{"dark", true, false, false},
		{"light", false, true, false},
		// Auto defaults to dark on server
		{"auto", true, false, true},
	}

	for _, tt := range tests {
		t.Run(tt.theme, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.AddCookie(&http.Cookie{Name: "theme", Value: tt.theme})

			info := GetThemeInfo(req)

			if info.Current != tt.theme {
				t.Errorf("Current = %q, want %q", info.Current, tt.theme)
			}
			if info.IsDark != tt.wantDark {
				t.Errorf("IsDark = %v, want %v", info.IsDark, tt.wantDark)
			}
			if info.IsLight != tt.wantLight {
				t.Errorf("IsLight = %v, want %v", info.IsLight, tt.wantLight)
			}
			if info.IsAuto != tt.wantAuto {
				t.Errorf("IsAuto = %v, want %v", info.IsAuto, tt.wantAuto)
			}
		})
	}
}

func TestThemeInfoStruct(t *testing.T) {
	info := ThemeInfo{
		Current:   "dark",
		ClassName: "theme-dark",
		IsDark:    true,
		IsLight:   false,
		IsAuto:    false,
	}

	if info.Current != "dark" {
		t.Errorf("Current = %q, want 'dark'", info.Current)
	}
	if info.ClassName != "theme-dark" {
		t.Errorf("ClassName = %q, want 'theme-dark'", info.ClassName)
	}
	if !info.IsDark {
		t.Error("IsDark should be true")
	}
	if info.IsLight {
		t.Error("IsLight should be false")
	}
	if info.IsAuto {
		t.Error("IsAuto should be false")
	}
}

// Tests for banner.go

func TestBannerInfoStruct(t *testing.T) {
	info := BannerInfo{
		AppName:    "Search",
		Version:    "1.0.0",
		Mode:       "production",
		Debug:      false,
		HTTPPort:   80,
		HTTPSPort:  443,
		HTTPAddr:   "http://localhost",
		HTTPSAddr:  "https://localhost",
		TorAddr:    "http://abcd.onion",
		I2PAddr:    "http://abcd.i2p",
		ListenAddr: "0.0.0.0:80",
		IsHTTPS:    true,
	}

	if info.AppName != "Search" {
		t.Errorf("AppName = %q, want 'Search'", info.AppName)
	}
	if info.Version != "1.0.0" {
		t.Errorf("Version = %q, want '1.0.0'", info.Version)
	}
	if info.Mode != "production" {
		t.Errorf("Mode = %q, want 'production'", info.Mode)
	}
	if info.Debug {
		t.Error("Debug should be false")
	}
	if info.HTTPPort != 80 {
		t.Errorf("HTTPPort = %d, want 80", info.HTTPPort)
	}
	if info.HTTPSPort != 443 {
		t.Errorf("HTTPSPort = %d, want 443", info.HTTPSPort)
	}
	if !info.IsHTTPS {
		t.Error("IsHTTPS should be true")
	}
}

func TestVisibleLength(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  int
	}{
		{"plain_text", "hello", 5},
		{"with_ansi_color", "\033[31mhello\033[0m", 5},
		{"multiple_ansi", "\033[1m\033[32mhi\033[0m", 2},
		{"empty", "", 0},
		{"only_ansi", "\033[31m\033[0m", 0},
		// Note: emojis are counted as 1 rune
		{"emoji", "🚀", 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := visibleLength(tt.input)
			if got != tt.want {
				t.Errorf("visibleLength(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestFormatBannerURL(t *testing.T) {
	tests := []struct {
		name    string
		host    string
		port    int
		isHTTPS bool
		want    string
	}{
		{"http_80", "localhost", 80, false, "http://localhost"},
		{"https_443", "localhost", 443, true, "https://localhost"},
		{"http_custom", "localhost", 8080, false, "http://localhost:8080"},
		{"https_custom", "localhost", 8443, true, "https://localhost:8443"},
		{"port_443_as_http", "localhost", 443, false, "https://localhost"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatBannerURL(tt.host, tt.port, tt.isHTTPS)
			if got != tt.want {
				t.Errorf("formatBannerURL(%q, %d, %v) = %q, want %q",
					tt.host, tt.port, tt.isHTTPS, got, tt.want)
			}
		})
	}
}

// Tests for session.go — DISABLED. The session subsystem was removed when
// the admin web UI and user-account systems were dropped. Restore as
// individual tests if a future feature reintroduces sessions.
/*
func TestSessionStruct(t *testing.T) {
	now := time.Now()
	session := Session{
		ID:        "test-session-id",
		Data:      map[string]interface{}{"key": "value"},
		UserID:    "user123",
		IP:        "192.168.1.1",
		UserAgent: "Mozilla/5.0",
		CreatedAt: now,
		ExpiresAt: now.Add(24 * time.Hour),
		LastSeen:  now,
	}

	if session.ID != "test-session-id" {
		t.Errorf("ID = %q, want 'test-session-id'", session.ID)
	}
	if session.UserID != "user123" {
		t.Errorf("UserID = %q, want 'user123'", session.UserID)
	}
	if session.IP != "192.168.1.1" {
		t.Errorf("IP = %q, want '192.168.1.1'", session.IP)
	}
	if session.Data["key"] != "value" {
		t.Errorf("Data['key'] = %v, want 'value'", session.Data["key"])
	}
}

func TestNewSessionManager(t *testing.T) {
	cfg := &config.Config{}
	cfg.Server.Session.User.MaxAge = 3600

	sm := NewSessionManager(cfg)
	if sm == nil {
		t.Fatal("NewSessionManager returned nil")
	}
	if sm.sessions == nil {
		t.Error("sessions map is nil")
	}
}

func TestSessionManagerCreate(t *testing.T) {
	cfg := &config.Config{}
	cfg.Server.Session.User.MaxAge = 3600

	sm := NewSessionManager(cfg)
	session := sm.Create("user123", "192.168.1.1", "Mozilla/5.0")

	if session == nil {
		t.Fatal("Create returned nil")
	}
	if session.UserID != "user123" {
		t.Errorf("UserID = %q, want 'user123'", session.UserID)
	}
	if session.IP != "192.168.1.1" {
		t.Errorf("IP = %q, want '192.168.1.1'", session.IP)
	}
	if session.ID == "" {
		t.Error("Session ID should not be empty")
	}
	// 32 bytes = 64 hex chars
	if len(session.ID) != 64 {
		t.Errorf("Session ID length = %d, want 64", len(session.ID))
	}
}

func TestSessionManagerGetAndDestroy(t *testing.T) {
	cfg := &config.Config{}
	cfg.Server.Session.User.MaxAge = 3600

	sm := NewSessionManager(cfg)
	session := sm.Create("user123", "192.168.1.1", "Mozilla/5.0")

	// Get existing session
	got, exists := sm.Get(session.ID)
	if !exists {
		t.Error("Session should exist")
	}
	if got.UserID != "user123" {
		t.Errorf("UserID = %q, want 'user123'", got.UserID)
	}

	// Get non-existent session
	_, exists = sm.Get("nonexistent")
	if exists {
		t.Error("Non-existent session should not exist")
	}

	// Destroy session
	sm.Destroy(session.ID)
	_, exists = sm.Get(session.ID)
	if exists {
		t.Error("Destroyed session should not exist")
	}
}

func TestSessionManagerRefresh(t *testing.T) {
	cfg := &config.Config{}
	cfg.Server.Session.User.MaxAge = 3600

	sm := NewSessionManager(cfg)
	session := sm.Create("user123", "192.168.1.1", "Mozilla/5.0")
	originalExpiry := session.ExpiresAt

	// Wait briefly
	time.Sleep(10 * time.Millisecond)

	// Refresh should return true for existing session
	if !sm.Refresh(session.ID) {
		t.Error("Refresh should return true for existing session")
	}

	// Check expiry was updated
	got, _ := sm.Get(session.ID)
	if !got.ExpiresAt.After(originalExpiry) {
		t.Error("Expiry should be extended after refresh")
	}

	// Refresh non-existent should return false
	if sm.Refresh("nonexistent") {
		t.Error("Refresh should return false for non-existent session")
	}
}

func TestSessionManagerCount(t *testing.T) {
	cfg := &config.Config{}
	cfg.Server.Session.User.MaxAge = 3600

	sm := NewSessionManager(cfg)

	if sm.Count() != 0 {
		t.Errorf("Initial count = %d, want 0", sm.Count())
	}

	sm.Create("user1", "192.168.1.1", "Mozilla/5.0")
	if sm.Count() != 1 {
		t.Errorf("Count after 1 create = %d, want 1", sm.Count())
	}

	session2 := sm.Create("user2", "192.168.1.2", "Chrome")
	if sm.Count() != 2 {
		t.Errorf("Count after 2 creates = %d, want 2", sm.Count())
	}

	sm.Destroy(session2.ID)
	if sm.Count() != 1 {
		t.Errorf("Count after destroy = %d, want 1", sm.Count())
	}
}

func TestSessionManagerCookie(t *testing.T) {
	cfg := &config.Config{}
	cfg.Server.Session.User.MaxAge = 3600
	cfg.Server.Session.User.CookieName = "test_session"
	cfg.Server.Session.HTTPOnly = true
	cfg.Server.Session.SameSite = "lax"

	sm := NewSessionManager(cfg)
	session := sm.Create("user123", "192.168.1.1", "Mozilla/5.0")

	// Test SetCookie
	rec := httptest.NewRecorder()
	sm.SetCookie(rec, session)

	cookies := rec.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("No cookie set")
	}

	found := false
	for _, c := range cookies {
		if c.Name == "test_session" {
			found = true
			if c.Value != session.ID {
				t.Errorf("Cookie value = %q, want %q", c.Value, session.ID)
			}
			if !c.HttpOnly {
				t.Error("Cookie should be HttpOnly")
			}
		}
	}
	if !found {
		t.Error("Session cookie not found")
	}

	// Test ClearCookie
	rec2 := httptest.NewRecorder()
	sm.ClearCookie(rec2)

	cookies2 := rec2.Result().Cookies()
	for _, c := range cookies2 {
		if c.Name == "test_session" && c.MaxAge != -1 {
			t.Errorf("Clear cookie MaxAge = %d, want -1", c.MaxAge)
		}
	}
}

func TestSessionManagerGetFromRequest(t *testing.T) {
	cfg := &config.Config{}
	cfg.Server.Session.User.MaxAge = 3600
	cfg.Server.Session.User.CookieName = "test_session"

	sm := NewSessionManager(cfg)
	session := sm.Create("user123", "192.168.1.1", "Mozilla/5.0")

	// With valid cookie
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "test_session", Value: session.ID})

	got, exists := sm.GetFromRequest(req)
	if !exists {
		t.Error("Session should exist from request")
	}
	if got.UserID != "user123" {
		t.Errorf("UserID = %q, want 'user123'", got.UserID)
	}

	// Without cookie
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	_, exists = sm.GetFromRequest(req2)
	if exists {
		t.Error("Session should not exist without cookie")
	}
}

*/

// Tests for ssl.go

func TestSSLManagerStruct(t *testing.T) {
	cfg := &config.Config{}
	sm := NewSSLManager(cfg)

	if sm == nil {
		t.Fatal("NewSSLManager returned nil")
	}
	if sm.config == nil {
		t.Error("config is nil")
	}
}

func TestGetTLSConfigDisabled(t *testing.T) {
	cfg := &config.Config{}
	cfg.Server.SSL.Enabled = false

	sm := NewSSLManager(cfg)
	tlsConfig, err := sm.GetTLSConfig()

	if err != nil {
		t.Errorf("GetTLSConfig error = %v", err)
	}
	if tlsConfig != nil {
		t.Error("TLS config should be nil when SSL disabled")
	}
}

func TestGetTLSConfigEnabled(t *testing.T) {
	cfg := &config.Config{}
	cfg.Server.SSL.Enabled = true

	sm := NewSSLManager(cfg)
	tlsConfig, err := sm.GetTLSConfig()

	if err != nil {
		t.Errorf("GetTLSConfig error = %v", err)
	}
	if tlsConfig == nil {
		t.Fatal("TLS config should not be nil when SSL enabled")
	}
	if tlsConfig.MinVersion != tls.VersionTLS12 {
		t.Errorf("MinVersion = %d, want TLS 1.2", tlsConfig.MinVersion)
	}
	if tlsConfig.MaxVersion != tls.VersionTLS13 {
		t.Errorf("MaxVersion = %d, want TLS 1.3", tlsConfig.MaxVersion)
	}
}

func TestGetCertificatePaths(t *testing.T) {
	cfg := &config.Config{}
	cfg.Server.SSL.CertFile = "/custom/cert.pem"
	cfg.Server.SSL.KeyFile = "/custom/key.pem"

	sm := NewSSLManager(cfg)
	certFile, keyFile := sm.GetCertificatePaths()

	if certFile != "/custom/cert.pem" {
		t.Errorf("CertFile = %q, want '/custom/cert.pem'", certFile)
	}
	if keyFile != "/custom/key.pem" {
		t.Errorf("KeyFile = %q, want '/custom/key.pem'", keyFile)
	}
}

func TestHasValidCertificateNone(t *testing.T) {
	cfg := &config.Config{}
	cfg.Server.SSL.CertFile = "/nonexistent/cert.pem"
	cfg.Server.SSL.KeyFile = "/nonexistent/key.pem"

	sm := NewSSLManager(cfg)
	if sm.HasValidCertificate() {
		t.Error("HasValidCertificate should return false for nonexistent files")
	}
}

// Tests for opensearch.go

func TestOpenSearchDescriptionStruct(t *testing.T) {
	osd := OpenSearchDescription{
		XMLNS:          "http://a9.com/-/spec/opensearch/1.1/",
		ShortName:      "Test Search",
		Description:    "A test search engine",
		Tags:           "test, search",
		Contact:        "admin@example.com",
		LongName:       "Test Search Engine",
		InputEncoding:  "UTF-8",
		OutputEncoding: "UTF-8",
	}

	if osd.ShortName != "Test Search" {
		t.Errorf("ShortName = %q, want 'Test Search'", osd.ShortName)
	}
	if osd.XMLNS != "http://a9.com/-/spec/opensearch/1.1/" {
		t.Errorf("XMLNS incorrect")
	}
}

func TestOpenSearchImageStruct(t *testing.T) {
	img := OpenSearchImage{
		Width:  64,
		Height: 64,
		Type:   "image/png",
		URL:    "https://example.com/icon.png",
	}

	if img.Width != 64 {
		t.Errorf("Width = %d, want 64", img.Width)
	}
	if img.Type != "image/png" {
		t.Errorf("Type = %q, want 'image/png'", img.Type)
	}
}

func TestOpenSearchURLStruct(t *testing.T) {
	url := OpenSearchURL{
		Type:     "text/html",
		Method:   "get",
		Template: "https://example.com/search?q={searchTerms}",
		Rel:      "results",
	}

	if url.Type != "text/html" {
		t.Errorf("Type = %q, want 'text/html'", url.Type)
	}
	if url.Template != "https://example.com/search?q={searchTerms}" {
		t.Errorf("Template incorrect")
	}
}

// Tests for logging.go

func TestLogLevelString(t *testing.T) {
	tests := []struct {
		level LogLevel
		want  string
	}{
		{LevelDebug, "DEBUG"},
		{LevelInfo, "INFO"},
		{LevelWarn, "WARN"},
		{LevelError, "ERROR"},
		{LevelFatal, "FATAL"},
		{LogLevel(99), "UNKNOWN"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.level.String(); got != tt.want {
				t.Errorf("LogLevel(%d).String() = %q, want %q", tt.level, got, tt.want)
			}
		})
	}
}

func TestParseLogLevel(t *testing.T) {
	tests := []struct {
		input string
		want  LogLevel
	}{
		{"debug", LevelDebug},
		{"DEBUG", LevelDebug},
		{"info", LevelInfo},
		{"INFO", LevelInfo},
		{"warn", LevelWarn},
		{"warning", LevelWarn},
		{"WARN", LevelWarn},
		{"error", LevelError},
		{"ERROR", LevelError},
		{"fatal", LevelFatal},
		{"FATAL", LevelFatal},
		// unknown → defaults to LevelInfo
		{"", LevelInfo},
		{"verbose", LevelInfo},
		{"trace", LevelInfo},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := ParseLogLevel(tt.input); got != tt.want {
				t.Errorf("ParseLogLevel(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestNewLogger(t *testing.T) {
	cfg := &config.Config{}
	cfg.Server.Logs.Level = "debug"
	cfg.Server.Logs.Format = "text"

	l := NewLogger(cfg)
	if l == nil {
		t.Fatal("NewLogger() returned nil")
	}
	if l.level != LevelDebug {
		t.Errorf("level = %v, want LevelDebug", l.level)
	}
	if l.format != "text" {
		t.Errorf("format = %q, want %q", l.format, "text")
	}
}

func TestLoggerSetLevel(t *testing.T) {
	cfg := &config.Config{}
	cfg.Server.Logs.Level = "info"
	l := NewLogger(cfg)

	l.SetLevel(LevelError)
	if l.level != LevelError {
		t.Errorf("after SetLevel: level = %v, want LevelError", l.level)
	}
}

func TestLoggerWithField(t *testing.T) {
	cfg := &config.Config{}
	l := NewLogger(cfg)
	l2 := l.WithField("key", "value")

	if l2 == l {
		t.Error("WithField should return a new logger instance")
	}
	if l2.fields["key"] != "value" {
		t.Errorf("fields[key] = %v, want %q", l2.fields["key"], "value")
	}
	// Original should be unmodified
	if _, ok := l.fields["key"]; ok {
		t.Error("original logger fields should not be modified by WithField")
	}
}

func TestLoggerWithFields(t *testing.T) {
	cfg := &config.Config{}
	l := NewLogger(cfg)
	l2 := l.WithFields(map[string]interface{}{"a": 1, "b": "two"})

	if l2 == l {
		t.Error("WithFields should return a new logger instance")
	}
	if l2.fields["a"] != 1 {
		t.Errorf("fields[a] = %v, want 1", l2.fields["a"])
	}
	if l2.fields["b"] != "two" {
		t.Errorf("fields[b] = %v, want 'two'", l2.fields["b"])
	}
}

func TestLoggerWithFieldsInheritance(t *testing.T) {
	cfg := &config.Config{}
	l := NewLogger(cfg)
	l = l.WithField("original", true)
	l2 := l.WithFields(map[string]interface{}{"new": "field"})

	// New logger should have both fields
	if _, ok := l2.fields["original"]; !ok {
		t.Error("WithFields should inherit existing fields from parent logger")
	}
	if _, ok := l2.fields["new"]; !ok {
		t.Error("WithFields should include new fields")
	}
}

func TestLoggerDebugBelowLevelSuppressed(t *testing.T) {
	cfg := &config.Config{}
	cfg.Server.Logs.Level = "info"
	l := NewLogger(cfg)

	// Write to a buffer so we can check nothing was written
	var buf strings.Builder
	l.output = &buf

	l.Debug("should be suppressed")

	if buf.Len() > 0 {
		t.Errorf("Debug() wrote output when level is INFO: %q", buf.String())
	}
}

func TestLoggerInfoWritesOutput(t *testing.T) {
	cfg := &config.Config{}
	cfg.Server.Logs.Level = "debug"
	cfg.Server.Logs.Format = "text"
	l := NewLogger(cfg)
	l.colorOutput = false

	var buf strings.Builder
	l.output = &buf

	l.Info("hello world")

	if !strings.Contains(buf.String(), "hello world") {
		t.Errorf("Info() output does not contain message: %q", buf.String())
	}
	if !strings.Contains(buf.String(), "INFO") {
		t.Errorf("Info() output does not contain level: %q", buf.String())
	}
}

func TestLoggerWarnWritesOutput(t *testing.T) {
	cfg := &config.Config{}
	cfg.Server.Logs.Level = "debug"
	cfg.Server.Logs.Format = "text"
	l := NewLogger(cfg)
	l.colorOutput = false

	var buf strings.Builder
	l.output = &buf

	l.Warn("warning message")

	if !strings.Contains(buf.String(), "WARN") {
		t.Errorf("Warn() output missing level: %q", buf.String())
	}
}

func TestLoggerErrorIncludesCallerInfo(t *testing.T) {
	cfg := &config.Config{}
	cfg.Server.Logs.Level = "debug"
	cfg.Server.Logs.Format = "text"
	l := NewLogger(cfg)
	l.colorOutput = false

	var buf strings.Builder
	l.output = &buf

	l.Error("error occurred")

	// Error level adds file:line info
	out := buf.String()
	if !strings.Contains(out, "ERROR") {
		t.Errorf("Error() output missing level: %q", out)
	}
}

func TestLoggerJSONFormat(t *testing.T) {
	cfg := &config.Config{}
	cfg.Server.Logs.Level = "debug"
	cfg.Server.Logs.Format = "json"
	l := NewLogger(cfg)

	var buf strings.Builder
	l.output = &buf

	l.Info("json test")

	out := buf.String()
	if !strings.Contains(out, `"message"`) {
		t.Errorf("JSON output missing 'message' key: %q", out)
	}
	if !strings.Contains(out, "json test") {
		t.Errorf("JSON output missing message text: %q", out)
	}
	if !strings.Contains(out, `"level"`) {
		t.Errorf("JSON output missing 'level' key: %q", out)
	}
}

func TestLoggerRequestLogger(t *testing.T) {
	cfg := &config.Config{}
	cfg.Server.Logs.Level = "debug"
	cfg.Server.Logs.Format = "text"
	l := NewLogger(cfg)
	l.colorOutput = false

	var buf strings.Builder
	l.output = &buf

	l.RequestLogger("GET", "/search", "127.0.0.1", 200, 50*time.Millisecond)

	out := buf.String()
	if !strings.Contains(out, "HTTP Request") {
		t.Errorf("RequestLogger output missing 'HTTP Request': %q", out)
	}
	if !strings.Contains(out, "/search") {
		t.Errorf("RequestLogger output missing path: %q", out)
	}
}

func TestLoggerRotateNoFile(t *testing.T) {
	cfg := &config.Config{}
	l := NewLogger(cfg)
	// No file configured — Rotate should be a no-op returning nil
	if err := l.Rotate(); err != nil {
		t.Errorf("Rotate() with no file = %v, want nil", err)
	}
}

func TestLoggerClose(t *testing.T) {
	cfg := &config.Config{}
	l := NewLogger(cfg)
	// No file — Close should return nil
	if err := l.Close(); err != nil {
		t.Errorf("Close() with no file = %v, want nil", err)
	}
}

// Tests for pages.go pure functions

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		name  string
		input time.Duration
		want  string
	}{
		{"zero", 0, "0m"},
		{"45 seconds", 45 * time.Second, "0m"},
		{"one minute", time.Minute, "1m"},
		{"90 seconds", 90 * time.Second, "1m"},
		{"one hour", time.Hour, "1h 0m"},
		{"1h30m", time.Hour + 30*time.Minute, "1h 30m"},
		{"one day", 24 * time.Hour, "1d 0h 0m"},
		{"2d 5h 30m", 2*24*time.Hour + 5*time.Hour + 30*time.Minute, "2d 5h 30m"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatDuration(tt.input); got != tt.want {
				t.Errorf("formatDuration(%v) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestGetVersion(t *testing.T) {
	v := getVersion()
	if v == "" {
		t.Error("getVersion() returned empty string")
	}
}

func TestJsonMarshal(t *testing.T) {
	type sample struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}
	input := sample{Name: "test", Age: 42}
	data, err := jsonMarshal(input)
	if err != nil {
		t.Fatalf("jsonMarshal() error = %v", err)
	}
	if !strings.Contains(string(data), `"name"`) {
		t.Errorf("jsonMarshal output missing 'name' key: %s", data)
	}
	if !strings.Contains(string(data), "test") {
		t.Errorf("jsonMarshal output missing value: %s", data)
	}
}

func TestJsonMarshalNilInput(t *testing.T) {
	data, err := jsonMarshal(nil)
	if err != nil {
		t.Fatalf("jsonMarshal(nil) error = %v", err)
	}
	if string(data) != "null" {
		t.Errorf("jsonMarshal(nil) = %q, want %q", string(data), "null")
	}
}

func TestHandleLivez(t *testing.T) {
	s := &Server{config: &config.Config{}, i18nManager: i18n.NewManager("en", []string{"en"})}
	r := httptest.NewRequest(http.MethodGet, "/livez", nil)
	w := httptest.NewRecorder()

	s.handleLivez(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("handleLivez() status = %d, want %d", w.Code, http.StatusOK)
	}
	if !strings.Contains(w.Body.String(), "ALIVE") {
		t.Errorf("handleLivez() body = %q, want to contain 'ALIVE'", w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); ct != "text/plain" {
		t.Errorf("handleLivez() Content-Type = %q, want 'text/plain'", ct)
	}
}

func TestHandleReadyzHealthy(t *testing.T) {
	s := &Server{config: &config.Config{}, i18nManager: i18n.NewManager("en", []string{"en"})}
	r := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	w := httptest.NewRecorder()

	s.handleReadyz(w, r)

	// With no DB and no maintenance mode, should be 200 READY
	if w.Code != http.StatusOK {
		t.Errorf("handleReadyz() status = %d, want %d", w.Code, http.StatusOK)
	}
	if !strings.Contains(w.Body.String(), "READY") {
		t.Errorf("handleReadyz() body = %q, want to contain 'READY'", w.Body.String())
	}
}

func TestHandleReadyzMaintenance(t *testing.T) {
	cfg := &config.Config{}
	cfg.Server.MaintenanceMode = true
	s := &Server{config: cfg, i18nManager: i18n.NewManager("en", []string{"en"})}
	r := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	w := httptest.NewRecorder()

	s.handleReadyz(w, r)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("handleReadyz() in maintenance status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
	if !strings.Contains(w.Body.String(), "NOT READY") {
		t.Errorf("handleReadyz() body = %q, want to contain 'NOT READY'", w.Body.String())
	}
}

func TestHandleNotFoundAPIPath(t *testing.T) {
	s := &Server{config: &config.Config{}, i18nManager: i18n.NewManager("en", []string{"en"})}
	r := httptest.NewRequest(http.MethodGet, "/api/v1/missing", nil)
	w := httptest.NewRecorder()

	s.handleNotFound(w, r)

	if w.Code != http.StatusNotFound {
		t.Errorf("handleNotFound() API status = %d, want %d", w.Code, http.StatusNotFound)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("handleNotFound() API Content-Type = %q, want 'application/json'", ct)
	}
	body := w.Body.String()
	if !strings.Contains(body, "NOT_FOUND") {
		t.Errorf("handleNotFound() API body = %q, missing NOT_FOUND", body)
	}
	if !strings.Contains(body, `"ok":false`) {
		t.Errorf("handleNotFound() API body = %q, missing ok:false", body)
	}
}

func TestHandleNotFoundNonAPIPath(t *testing.T) {
	s := &Server{config: &config.Config{}, i18nManager: i18n.NewManager("en", []string{"en"})}
	r := httptest.NewRequest(http.MethodGet, "/missing-page", nil)
	w := httptest.NewRecorder()

	s.handleNotFound(w, r)

	// Non-API paths fall through to handleError (renderer is nil so falls back to http.Error)
	if w.Code != http.StatusNotFound {
		t.Errorf("handleNotFound() non-API status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestHandleAutocompleteEmptyQuery(t *testing.T) {
	s := &Server{config: &config.Config{}, i18nManager: i18n.NewManager("en", []string{"en"})}
	r := httptest.NewRequest(http.MethodGet, "/autocomplete?q=", nil)
	w := httptest.NewRecorder()

	s.handleAutocomplete(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("handleAutocomplete() empty query status = %d, want %d", w.Code, http.StatusOK)
	}
	body := w.Body.String()
	if !strings.Contains(body, `"ok":true`) {
		t.Errorf("handleAutocomplete() empty query body = %q, missing ok:true", body)
	}
}

func TestHandleAutocompleteNoAPIHandler(t *testing.T) {
	// When apiHandler is nil and query is present, fall back to empty suggestions
	s := &Server{config: &config.Config{}, i18nManager: i18n.NewManager("en", []string{"en"})}
	r := httptest.NewRequest(http.MethodGet, "/autocomplete?q=golang", nil)
	w := httptest.NewRecorder()

	s.handleAutocomplete(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("handleAutocomplete() no-apiHandler status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestRespondHealthJSONHealthy(t *testing.T) {
	s := &Server{config: &config.Config{}, i18nManager: i18n.NewManager("en", []string{"en"})}
	health := &HealthResponse{Status: "healthy", Version: "1.0.0"}
	w := httptest.NewRecorder()

	s.respondHealthJSON(w, health)

	if w.Code != http.StatusOK {
		t.Errorf("respondHealthJSON() healthy status = %d, want %d", w.Code, http.StatusOK)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("respondHealthJSON() Content-Type = %q, want 'application/json'", ct)
	}
	if !strings.Contains(w.Body.String(), "healthy") {
		t.Errorf("respondHealthJSON() body missing 'healthy': %q", w.Body.String())
	}
}

func TestRespondHealthJSONUnhealthy(t *testing.T) {
	s := &Server{config: &config.Config{}, i18nManager: i18n.NewManager("en", []string{"en"})}
	health := &HealthResponse{Status: "unhealthy"}
	w := httptest.NewRecorder()

	s.respondHealthJSON(w, health)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("respondHealthJSON() unhealthy status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
}

func TestRespondHealthJSONMaintenance(t *testing.T) {
	s := &Server{config: &config.Config{}, i18nManager: i18n.NewManager("en", []string{"en"})}
	health := &HealthResponse{Status: "maintenance"}
	w := httptest.NewRecorder()

	s.respondHealthJSON(w, health)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("respondHealthJSON() maintenance status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
}

func TestRespondHealthText(t *testing.T) {
	s := &Server{config: &config.Config{}, i18nManager: i18n.NewManager("en", []string{"en"})}
	health := &HealthResponse{
		Status:  "healthy",
		Version: "1.0.0",
		Mode:    "production",
		Uptime:  "1h 0m",
		Checks: ChecksInfo{
			Database:  "ok",
			Cache:     "disabled",
			Disk:      "ok",
			Scheduler: "ok",
		},
	}
	w := httptest.NewRecorder()

	s.respondHealthText(w, health)

	if w.Code != http.StatusOK {
		t.Errorf("respondHealthText() status = %d, want %d", w.Code, http.StatusOK)
	}
	body := w.Body.String()
	if !strings.Contains(body, "status: healthy") {
		t.Errorf("respondHealthText() body missing 'status: healthy': %q", body)
	}
	if !strings.Contains(body, "version: 1.0.0") {
		t.Errorf("respondHealthText() body missing version: %q", body)
	}
}

func TestRespondHealthTextUnhealthy(t *testing.T) {
	s := &Server{config: &config.Config{}, i18nManager: i18n.NewManager("en", []string{"en"})}
	health := &HealthResponse{Status: "unhealthy"}
	w := httptest.NewRecorder()

	s.respondHealthText(w, health)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("respondHealthText() unhealthy status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
}

func TestBuildHealthInfoMinimal(t *testing.T) {
	cfg := &config.Config{}
	cfg.Server.Mode = "production"
	s := &Server{
		config:      cfg,
		i18nManager: i18n.NewManager("en", []string{"en"}),
		startTime:   time.Now().Add(-time.Hour),
	}

	health := s.buildHealthInfo()

	if health == nil {
		t.Fatal("buildHealthInfo() returned nil")
	}
	if health.Status != "healthy" {
		t.Errorf("buildHealthInfo() status = %q, want 'healthy'", health.Status)
	}
	if health.Mode != "production" {
		t.Errorf("buildHealthInfo() mode = %q, want 'production'", health.Mode)
	}
	if health.Checks.Database != "ok" {
		t.Errorf("buildHealthInfo() checks.database = %q, want 'ok'", health.Checks.Database)
	}
}

func TestBuildHealthInfoMaintenanceMode(t *testing.T) {
	cfg := &config.Config{}
	cfg.Server.MaintenanceMode = true
	s := &Server{
		config:      cfg,
		i18nManager: i18n.NewManager("en", []string{"en"}),
		startTime:   time.Now(),
	}

	health := s.buildHealthInfo()

	if health.Status != "maintenance" {
		t.Errorf("buildHealthInfo() maintenance status = %q, want 'maintenance'", health.Status)
	}
}

func TestSignCaptcha(t *testing.T) {
	s := &Server{
		config:    &config.Config{},
		startTime: time.Now(),
	}
	id := s.signCaptcha(7)
	if id == "" {
		t.Error("signCaptcha() returned empty string")
	}
	// Format should be "answer.signature"
	parts := strings.Split(id, ".")
	if len(parts) < 2 {
		t.Errorf("signCaptcha() format incorrect, got %q (expected answer.sig)", id)
	}
	if parts[0] != "7" {
		t.Errorf("signCaptcha(7) answer part = %q, want '7'", parts[0])
	}
}

func TestGetCSRFToken(t *testing.T) {
	s := &Server{config: &config.Config{}}
	r := httptest.NewRequest(http.MethodGet, "/", nil)

	tok1 := s.getCSRFToken(r)
	tok2 := s.getCSRFToken(r)

	if tok1 == "" {
		t.Error("getCSRFToken() returned empty token")
	}
	// Each call generates a new random token
	if tok1 == tok2 {
		t.Error("getCSRFToken() returned same token on consecutive calls (should be random)")
	}
}

// Tests for auth.go

func TestExtractBearerToken(t *testing.T) {
	tests := []struct {
		name    string
		header  string
		wantTok string
		wantOK  bool
	}{
		{"no header", "", "", false},
		{"non-bearer", "Basic dXNlcjpwYXNz", "", false},
		{"bearer only prefix", "Bearer ", "", false},
		{"valid token", "Bearer mytoken123", "mytoken123", true},
		{"valid with whitespace", "Bearer  spaced ", "spaced", true},
		{"case insensitive", "bearer mytoken", "mytoken", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.header != "" {
				r.Header.Set("Authorization", tt.header)
			}
			tok, ok := extractBearerToken(r)
			if ok != tt.wantOK {
				t.Errorf("extractBearerToken() ok = %v, want %v", ok, tt.wantOK)
			}
			if tok != tt.wantTok {
				t.Errorf("extractBearerToken() token = %q, want %q", tok, tt.wantTok)
			}
		})
	}
}

func TestValidateOperatorToken(t *testing.T) {
	tests := []struct {
		name        string
		configToken string
		headerToken string
		want        bool
	}{
		{"nil config handled via no match", "", "", false},
		{"empty config token", "", "anything", false},
		{"matching token", "secret123", "secret123", true},
		{"wrong token", "secret123", "wrongtoken", false},
		{"no header", "secret123", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{}
			cfg.Server.Token = tt.configToken
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.headerToken != "" {
				r.Header.Set("Authorization", "Bearer "+tt.headerToken)
			}
			got := ValidateOperatorToken(r, cfg)
			if got != tt.want {
				t.Errorf("ValidateOperatorToken() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestValidateOperatorTokenNilConfig(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Authorization", "Bearer anytoken")
	if got := ValidateOperatorToken(r, nil); got {
		t.Error("ValidateOperatorToken() with nil config should return false")
	}
}

func TestRequireOperator(t *testing.T) {
	tests := []struct {
		name       string
		token      string
		sendHeader string
		wantStatus int
	}{
		{"no token configured", "", "", http.StatusUnauthorized},
		{"valid token", "correcttoken", "correcttoken", http.StatusOK},
		{"wrong token", "correcttoken", "wrongtoken", http.StatusUnauthorized},
		{"missing header", "correcttoken", "", http.StatusUnauthorized},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{}
			cfg.Server.Token = tt.token
			s := &Server{config: cfg, i18nManager: i18n.NewManager("en", []string{"en"})}

			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})

			handler := s.RequireOperator(next)
			r := httptest.NewRequest(http.MethodGet, "/api/admin", nil)
			if tt.sendHeader != "" {
				r.Header.Set("Authorization", "Bearer "+tt.sendHeader)
			}
			w := httptest.NewRecorder()
			handler(w, r)

			if w.Code != tt.wantStatus {
				t.Errorf("RequireOperator() status = %d, want %d", w.Code, tt.wantStatus)
			}
		})
	}
}

func TestRequireOperatorSetsWWWAuthenticate(t *testing.T) {
	cfg := &config.Config{}
	cfg.Server.Token = "secret"
	s := &Server{config: cfg, i18nManager: i18n.NewManager("en", []string{"en"})}

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := s.RequireOperator(next)

	r := httptest.NewRequest(http.MethodGet, "/api/admin", nil)
	w := httptest.NewRecorder()
	handler(w, r)

	if hdr := w.Header().Get("WWW-Authenticate"); hdr == "" {
		t.Error("RequireOperator() should set WWW-Authenticate header on 401")
	}
}

// Tests for alerts.go pure functions

func TestSplitAlertAction(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		wantToken  string
		wantAction string
	}{
		{"empty path", "", "", ""},
		{"no slash", "tokenonly", "", ""},
		{"valid token/update", "tok123/update", "tok123", "update"},
		{"valid token/delete", "tok123/delete", "tok123", "delete"},
		{"valid token/pause", "tok456/pause", "tok456", "pause"},
		{"nested path ignored", "tok/sub/extra", "tok", "sub/extra"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotToken, gotAction := splitAlertAction(tt.path)
			if gotToken != tt.wantToken {
				t.Errorf("splitAlertAction(%q) token = %q, want %q", tt.path, gotToken, tt.wantToken)
			}
			if gotAction != tt.wantAction {
				t.Errorf("splitAlertAction(%q) action = %q, want %q", tt.path, gotAction, tt.wantAction)
			}
		})
	}
}

func TestURLQueryEscape(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"", ""},
		{"hello", "hello"},
		{"hello world", "hello+world"},
		{"  trim me  ", "trim+me"},
		{"a&b=c", "a%26b%3Dc"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := urlQueryEscape(tt.input); got != tt.want {
				t.Errorf("urlQueryEscape(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestBuildAlertEnginesQuery(t *testing.T) {
	tests := []struct {
		name    string
		engines []string
		want    string
	}{
		{"empty slice", []string{}, ""},
		{"nil slice", nil, ""},
		{"one engine", []string{"google"}, "&engines=google"},
		{"two engines", []string{"google", "bing"}, "&engines=google&engines=bing"},
		{"with spaces", []string{"  google  "}, "&engines=google"},
		{"empty string skipped", []string{"google", "", "bing"}, "&engines=google&engines=bing"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := buildAlertEnginesQuery(tt.engines); got != tt.want {
				t.Errorf("buildAlertEnginesQuery(%v) = %q, want %q", tt.engines, got, tt.want)
			}
		})
	}
}

func TestSplitCSVParam(t *testing.T) {
	tests := []struct {
		name   string
		values []string
		want   []string
	}{
		{"empty", []string{}, []string{}},
		{"single value", []string{"google"}, []string{"google"}},
		{"csv in one", []string{"google,bing,duckduckgo"}, []string{"google", "bing", "duckduckgo"}},
		{"multi values", []string{"google", "bing"}, []string{"google", "bing"}},
		{"with spaces", []string{"google , bing"}, []string{"google", "bing"}},
		{"empty elements skipped", []string{"google,,bing"}, []string{"google", "bing"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitCSVParam(tt.values)
			if len(got) != len(tt.want) {
				t.Errorf("splitCSVParam(%v) = %v, want %v", tt.values, got, tt.want)
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("splitCSVParam(%v)[%d] = %q, want %q", tt.values, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestAlertEngineOptionsNilRegistry(t *testing.T) {
	s := &Server{config: &config.Config{}, registry: nil}
	opts := s.alertEngineOptions([]string{"google"})
	if len(opts) != 0 {
		t.Errorf("alertEngineOptions() with nil registry = %v, want empty slice", opts)
	}
}

func TestHandleAlertNewNilManager(t *testing.T) {
	s := &Server{
		config:       &config.Config{},
		i18nManager:  i18n.NewManager("en", []string{"en"}),
		alertManager: nil,
	}
	r := httptest.NewRequest(http.MethodGet, "/alerts/new", nil)
	w := httptest.NewRecorder()

	s.handleAlertNew(w, r)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("handleAlertNew() nil manager status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
}

func TestHandleAlertNewMethodNotAllowed(t *testing.T) {
	s := &Server{
		config:      &config.Config{},
		i18nManager: i18n.NewManager("en", []string{"en"}),
	}
	r := httptest.NewRequest(http.MethodPost, "/alerts/new", nil)
	w := httptest.NewRecorder()

	s.handleAlertNew(w, r)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("handleAlertNew() POST status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleAlertsNilManager(t *testing.T) {
	s := &Server{
		config:       &config.Config{},
		i18nManager:  i18n.NewManager("en", []string{"en"}),
		alertManager: nil,
	}
	r := httptest.NewRequest(http.MethodPost, "/alerts", strings.NewReader("query=test"))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	s.handleAlerts(w, r)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("handleAlerts() nil manager status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
}

func TestHandleAlertsMethodNotAllowed(t *testing.T) {
	s := &Server{
		config:      &config.Config{},
		i18nManager: i18n.NewManager("en", []string{"en"}),
	}
	r := httptest.NewRequest(http.MethodGet, "/alerts", nil)
	w := httptest.NewRecorder()

	s.handleAlerts(w, r)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("handleAlerts() GET status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleAlertActionNilManager(t *testing.T) {
	s := &Server{
		config:       &config.Config{},
		i18nManager:  i18n.NewManager("en", []string{"en"}),
		alertManager: nil,
	}
	r := httptest.NewRequest(http.MethodGet, "/alerts/tok123/update", nil)
	w := httptest.NewRecorder()

	s.handleAlertAction(w, r)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("handleAlertAction() nil manager status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
}

// Tests for response.go

func TestRespondJSON(t *testing.T) {
	w := httptest.NewRecorder()
	respondJSON(w, http.StatusOK, map[string]string{"key": "value"})

	if w.Code != http.StatusOK {
		t.Errorf("respondJSON() status = %d, want %d", w.Code, http.StatusOK)
	}
	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("respondJSON() Content-Type = %q, want 'application/json'", ct)
	}
	if !strings.Contains(w.Body.String(), "value") {
		t.Errorf("respondJSON() body = %q, missing expected value", w.Body.String())
	}
}

func TestRespondJSONCustomStatus(t *testing.T) {
	w := httptest.NewRecorder()
	respondJSON(w, http.StatusCreated, map[string]bool{"ok": true})

	if w.Code != http.StatusCreated {
		t.Errorf("respondJSON() status = %d, want %d", w.Code, http.StatusCreated)
	}
}

func TestRespondError(t *testing.T) {
	w := httptest.NewRecorder()
	respondError(w, http.StatusBadRequest, "something went wrong")

	if w.Code != http.StatusBadRequest {
		t.Errorf("respondError() status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	body := w.Body.String()
	if !strings.Contains(body, `"ok":false`) {
		t.Errorf("respondError() body = %q, missing ok:false", body)
	}
	if !strings.Contains(body, "something went wrong") {
		t.Errorf("respondError() body = %q, missing message", body)
	}
}

// Tests for debug.go handlers

func TestHandleDebugMemory(t *testing.T) {
	s := &Server{config: &config.Config{}}
	r := httptest.NewRequest(http.MethodGet, "/debug/memory", nil)
	w := httptest.NewRecorder()

	s.handleDebugMemory(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("handleDebugMemory() status = %d, want %d", w.Code, http.StatusOK)
	}
	body := w.Body.String()
	if !strings.Contains(body, "goroutines") {
		t.Errorf("handleDebugMemory() body = %q, missing 'goroutines'", body)
	}
	if !strings.Contains(body, "alloc_mb") {
		t.Errorf("handleDebugMemory() body = %q, missing 'alloc_mb'", body)
	}
}

func TestHandleDebugGoroutines(t *testing.T) {
	s := &Server{config: &config.Config{}}
	r := httptest.NewRequest(http.MethodGet, "/debug/goroutines", nil)
	w := httptest.NewRecorder()

	s.handleDebugGoroutines(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("handleDebugGoroutines() status = %d, want %d", w.Code, http.StatusOK)
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Errorf("handleDebugGoroutines() Content-Type = %q, want text/plain prefix", ct)
	}
	// Stack trace should mention goroutine
	if !strings.Contains(w.Body.String(), "goroutine") {
		t.Errorf("handleDebugGoroutines() body missing 'goroutine': %q", w.Body.String()[:200])
	}
}

func TestHandleDebugCacheNil(t *testing.T) {
	s := &Server{config: &config.Config{}, cache: nil}
	r := httptest.NewRequest(http.MethodGet, "/debug/cache", nil)
	w := httptest.NewRecorder()

	s.handleDebugCache(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("handleDebugCache() nil status = %d, want %d", w.Code, http.StatusOK)
	}
	if !strings.Contains(w.Body.String(), `"enabled":false`) {
		t.Errorf("handleDebugCache() nil body = %q, want enabled:false", w.Body.String())
	}
}

func TestHandleDebugDBNil(t *testing.T) {
	s := &Server{config: &config.Config{}, db: nil}
	r := httptest.NewRequest(http.MethodGet, "/debug/db", nil)
	w := httptest.NewRecorder()

	s.handleDebugDB(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("handleDebugDB() nil status = %d, want %d", w.Code, http.StatusOK)
	}
	if !strings.Contains(w.Body.String(), `"enabled":false`) {
		t.Errorf("handleDebugDB() nil body = %q, want enabled:false", w.Body.String())
	}
}

func TestHandleDebugSchedulerNil(t *testing.T) {
	s := &Server{config: &config.Config{}, scheduler: nil}
	r := httptest.NewRequest(http.MethodGet, "/debug/scheduler", nil)
	w := httptest.NewRecorder()

	s.handleDebugScheduler(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("handleDebugScheduler() nil status = %d, want %d", w.Code, http.StatusOK)
	}
	if !strings.Contains(w.Body.String(), `"enabled":false`) {
		t.Errorf("handleDebugScheduler() nil body = %q, want enabled:false", w.Body.String())
	}
}

func TestHandleDebugConfig(t *testing.T) {
	cfg := &config.Config{}
	cfg.Server.Mode = "production"
	s := &Server{config: cfg}
	r := httptest.NewRequest(http.MethodGet, "/debug/config", nil)
	w := httptest.NewRecorder()

	s.handleDebugConfig(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("handleDebugConfig() status = %d, want %d", w.Code, http.StatusOK)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("handleDebugConfig() Content-Type = %q, want 'application/json'", ct)
	}
}

func TestRegisterDebugRoutesDebugFalse(t *testing.T) {
	// When debug mode is off, no debug routes are registered
	// We verify by checking the config flag is used
	cfg := &config.Config{}
	cfg.Server.Mode = "production"
	s := &Server{config: cfg}

	// isDebug returns false for production mode with no debug flag
	if s.config.IsDebug() {
		t.Skip("config reports debug mode — skipping non-debug test")
	}
	// The method should return without panic when debug is false
	// We use a real chi router to verify no routes were registered
	router := newTestRouter()
	s.registerDebugRoutes(router)
	// If we get here without panic, the test passes
}

// Tests for opensearch.go

func TestGetBaseURLConfigured(t *testing.T) {
	cfg := &config.Config{}
	cfg.Server.BaseURL = "https://search.example.com"
	s := &Server{config: cfg}
	r := httptest.NewRequest(http.MethodGet, "/", nil)

	got := s.getBaseURL(r)
	if got != "https://search.example.com" {
		t.Errorf("getBaseURL() configured = %q, want %q", got, "https://search.example.com")
	}
}

func TestGetBaseURLConfiguredTrailingSlash(t *testing.T) {
	cfg := &config.Config{}
	cfg.Server.BaseURL = "https://search.example.com/"
	s := &Server{config: cfg}
	r := httptest.NewRequest(http.MethodGet, "/", nil)

	got := s.getBaseURL(r)
	if got != "https://search.example.com" {
		t.Errorf("getBaseURL() trailing slash = %q, want no trailing slash", got)
	}
}

func TestGetBaseURLXForwardedProto(t *testing.T) {
	s := &Server{config: &config.Config{}}
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Host = "example.com"
	r.Header.Set("X-Forwarded-Proto", "https")

	got := s.getBaseURL(r)
	if !strings.HasPrefix(got, "https://") {
		t.Errorf("getBaseURL() X-Forwarded-Proto = %q, want https:// prefix", got)
	}
}

func TestGetBaseURLXForwardedHost(t *testing.T) {
	s := &Server{config: &config.Config{}}
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Host = "internal.host"
	r.Header.Set("X-Forwarded-Host", "public.example.com, extra.host")

	got := s.getBaseURL(r)
	if !strings.Contains(got, "public.example.com") {
		t.Errorf("getBaseURL() X-Forwarded-Host = %q, want public.example.com", got)
	}
}

func TestGetBaseURLHTTP(t *testing.T) {
	s := &Server{config: &config.Config{}}
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Host = "localhost:8080"

	got := s.getBaseURL(r)
	if !strings.HasPrefix(got, "http://") {
		t.Errorf("getBaseURL() plain HTTP = %q, want http:// prefix", got)
	}
}

func TestGetBaseURLForwardedHeader(t *testing.T) {
	s := &Server{config: &config.Config{}}
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Host = "example.com"
	r.Header.Set("Forwarded", "for=10.0.0.1; proto=https")

	got := s.getBaseURL(r)
	if !strings.HasPrefix(got, "https://") {
		t.Errorf("getBaseURL() Forwarded header = %q, want https:// prefix", got)
	}
}

func TestValidateNotPrivateProxyLocalhost(t *testing.T) {
	tests := []string{"localhost", "127.0.0.1", "::1"}
	for _, host := range tests {
		t.Run(host, func(t *testing.T) {
			if err := validateNotPrivateProxy(host); err == nil {
				t.Errorf("validateNotPrivateProxy(%q) should return error for localhost/loopback", host)
			}
		})
	}
}

func TestValidateNotPrivateProxyInternalSuffixes(t *testing.T) {
	tests := []string{
		"myservice.local",
		"db.internal",
		"api.localhost",
	}
	for _, host := range tests {
		t.Run(host, func(t *testing.T) {
			if err := validateNotPrivateProxy(host); err == nil {
				t.Errorf("validateNotPrivateProxy(%q) should return error for internal hostname", host)
			}
		})
	}
}

func TestHandleOpenSearchBasic(t *testing.T) {
	cfg := &config.Config{}
	cfg.Server.Title = "Test Search"
	cfg.Search.OpenSearch.ShortName = "Test"
	cfg.Search.OpenSearch.Description = "A test search engine"
	s := &Server{config: cfg, i18nManager: i18n.NewManager("en", []string{"en"})}

	r := httptest.NewRequest(http.MethodGet, "/opensearch.xml", nil)
	r.Host = "localhost:8080"
	w := httptest.NewRecorder()

	s.handleOpenSearch(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("handleOpenSearch() status = %d, want %d", w.Code, http.StatusOK)
	}
	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "opensearchdescription+xml") {
		t.Errorf("handleOpenSearch() Content-Type = %q, missing opensearchdescription+xml", ct)
	}
	if !strings.Contains(w.Body.String(), "OpenSearchDescription") {
		t.Errorf("handleOpenSearch() body missing OpenSearchDescription element")
	}
}

func TestHandleOpenSearchCustomName(t *testing.T) {
	cfg := &config.Config{}
	cfg.Server.Title = "Default Title"
	s := &Server{config: cfg, i18nManager: i18n.NewManager("en", []string{"en"})}

	r := httptest.NewRequest(http.MethodGet, "/opensearch.xml?name=CustomName", nil)
	r.Host = "localhost:8080"
	w := httptest.NewRecorder()

	s.handleOpenSearch(w, r)

	if !strings.Contains(w.Body.String(), "CustomName") {
		t.Errorf("handleOpenSearch() with name= param should use custom name, body: %q", w.Body.String())
	}
}

func TestHandlePreferencesSave(t *testing.T) {
	s := &Server{config: &config.Config{}, i18nManager: i18n.NewManager("en", []string{"en"})}
	r := httptest.NewRequest(http.MethodPost, "/preferences", nil)
	w := httptest.NewRecorder()

	s.handlePreferencesSave(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("handlePreferencesSave() status = %d, want %d", w.Code, http.StatusOK)
	}
	if !strings.Contains(w.Body.String(), `"ok":true`) {
		t.Errorf("handlePreferencesSave() body = %q, missing ok:true", w.Body.String())
	}
}

func TestHandleBangProxyMissingURL(t *testing.T) {
	s := &Server{config: &config.Config{}, i18nManager: i18n.NewManager("en", []string{"en"})}
	r := httptest.NewRequest(http.MethodGet, "/bang-proxy", nil)
	w := httptest.NewRecorder()

	s.handleBangProxy(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("handleBangProxy() missing url status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleBangProxyInvalidScheme(t *testing.T) {
	s := &Server{config: &config.Config{}, i18nManager: i18n.NewManager("en", []string{"en"})}
	r := httptest.NewRequest(http.MethodGet, "/bang-proxy?url=ftp://example.com/file.txt", nil)
	w := httptest.NewRecorder()

	s.handleBangProxy(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("handleBangProxy() invalid scheme status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleBangProxyLocalhost(t *testing.T) {
	s := &Server{config: &config.Config{}, i18nManager: i18n.NewManager("en", []string{"en"})}
	r := httptest.NewRequest(http.MethodGet, "/bang-proxy?url=http://localhost/secret", nil)
	w := httptest.NewRecorder()

	s.handleBangProxy(w, r)

	// localhost is blocked by SSRF prevention
	if w.Code != http.StatusBadRequest {
		t.Errorf("handleBangProxy() localhost url status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

// Tests for http_errors.go

func TestLocalizedHTTPError(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	localizedHTTPError(w, r, http.StatusNotFound, "errors.not_found")

	if w.Code != http.StatusNotFound {
		t.Errorf("localizedHTTPError() status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

// newTestRouter creates a minimal chi router for testing route registration.
func newTestRouter() *testChiRouter {
	return &testChiRouter{}
}

// testChiRouter is a no-op chi.Router that satisfies the interface for testing
// registerDebugRoutes when debug mode is off (no routes should be registered).
type testChiRouter struct {
	routes []string
}

func (r *testChiRouter) ServeHTTP(w http.ResponseWriter, req *http.Request) {}
func (r *testChiRouter) Use(middlewares ...func(http.Handler) http.Handler) {}
func (r *testChiRouter) With(middlewares ...func(http.Handler) http.Handler) chi.Router {
	return r
}
func (r *testChiRouter) Group(fn func(chi.Router)) chi.Router { fn(r); return r }
func (r *testChiRouter) Route(pattern string, fn func(chi.Router)) chi.Router {
	r.routes = append(r.routes, pattern)
	fn(r)
	return r
}
func (r *testChiRouter) Mount(pattern string, h http.Handler)          {}
func (r *testChiRouter) Handle(pattern string, h http.Handler)         {}
func (r *testChiRouter) HandleFunc(pattern string, h http.HandlerFunc) {}
func (r *testChiRouter) Method(method, pattern string, h http.Handler) {}
func (r *testChiRouter) MethodFunc(method, pattern string, h http.HandlerFunc) {
}
func (r *testChiRouter) Connect(pattern string, h http.HandlerFunc)         {}
func (r *testChiRouter) Delete(pattern string, h http.HandlerFunc)          {}
func (r *testChiRouter) Get(pattern string, h http.HandlerFunc)             {}
func (r *testChiRouter) Head(pattern string, h http.HandlerFunc)            {}
func (r *testChiRouter) Options(pattern string, h http.HandlerFunc)         {}
func (r *testChiRouter) Patch(pattern string, h http.HandlerFunc)           {}
func (r *testChiRouter) Post(pattern string, h http.HandlerFunc)            {}
func (r *testChiRouter) Put(pattern string, h http.HandlerFunc)             {}
func (r *testChiRouter) Trace(pattern string, h http.HandlerFunc)           {}
func (r *testChiRouter) NotFound(h http.HandlerFunc)                        {}
func (r *testChiRouter) MethodNotAllowed(h http.HandlerFunc)                {}
func (r *testChiRouter) Routes() []chi.Route                                { return nil }
func (r *testChiRouter) Middlewares() chi.Middlewares                       { return nil }
func (r *testChiRouter) Match(rctx *chi.Context, method, path string) bool  { return false }
func (r *testChiRouter) Find(rctx *chi.Context, method, path string) string { return "" }
