package server

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/apimgr/search/src/config"
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
	cfg := &config.Config{}
	cfg.Server.Security.CORS.Enabled = true
	cfg.Server.Security.CORS.AllowedOrigins = []string{"https://example.com"}
	cfg.Server.Security.CORS.AllowedMethods = []string{"GET", "POST", "PUT"}
	cfg.Server.Security.CORS.AllowedHeaders = []string{"Content-Type", "Authorization"}
	cfg.Server.Security.CORS.AllowCredentials = true
	cfg.Server.Security.CORS.MaxAge = 3600
	mw := NewMiddleware(cfg, nil)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("OK"))
	})

	wrapped := mw.CORS(handler)

	// Test regular request with allowed origin
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Origin", "https://example.com")
	rec := httptest.NewRecorder()

	wrapped.ServeHTTP(rec, req)

	if rec.Header().Get("Access-Control-Allow-Origin") != "https://example.com" {
		t.Errorf("Access-Control-Allow-Origin = %q", rec.Header().Get("Access-Control-Allow-Origin"))
	}
	if rec.Header().Get("Access-Control-Allow-Credentials") != "true" {
		t.Errorf("Access-Control-Allow-Credentials = %q", rec.Header().Get("Access-Control-Allow-Credentials"))
	}

	// Test preflight request
	reqPreflight := httptest.NewRequest(http.MethodOptions, "/", nil)
	reqPreflight.Header.Set("Origin", "https://example.com")
	recPreflight := httptest.NewRecorder()

	wrapped.ServeHTTP(recPreflight, reqPreflight)

	if recPreflight.Code != http.StatusNoContent {
		t.Errorf("Preflight status = %d, want %d", recPreflight.Code, http.StatusNoContent)
	}
	if !strings.Contains(recPreflight.Header().Get("Access-Control-Allow-Methods"), "GET") {
		t.Error("Access-Control-Allow-Methods should include GET")
	}
}

func TestMiddlewareCORSDisabled(t *testing.T) {
	cfg := &config.Config{}
	cfg.Server.Security.CORS.Enabled = false
	mw := NewMiddleware(cfg, nil)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("OK"))
	})

	wrapped := mw.CORS(handler)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Origin", "https://example.com")
	rec := httptest.NewRecorder()

	wrapped.ServeHTTP(rec, req)

	// CORS headers should not be set when disabled
	if rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Error("CORS headers should not be set when disabled")
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
	if len(token1) != 64 { // 32 bytes hex encoded
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
			path:       "/api/v1/search",
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

func TestExtractContextFromPath(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		adminPath  string
		wantType   TargetType
		wantName   string
	}{
		{
			name:      "root path",
			path:      "/",
			adminPath: "admin",
			wantType:  TargetPublic,
		},
		{
			name:      "server pages",
			path:      "/server/about",
			adminPath: "admin",
			wantType:  TargetServerPages,
		},
		{
			name:      "auth routes",
			path:      "/auth/login",
			adminPath: "admin",
			wantType:  TargetAuth,
		},
		{
			name:      "current user",
			path:      "/users",
			adminPath: "admin",
			wantType:  TargetCurrentUser,
		},
		{
			name:      "specific user",
			path:      "/users/john",
			adminPath: "admin",
			wantType:  TargetUser,
			wantName:  "john",
		},
		{
			name:      "org route",
			path:      "/orgs/acme",
			adminPath: "admin",
			wantType:  TargetOrg,
			wantName:  "acme",
		},
		{
			name:      "admin panel",
			path:      "/admin",
			adminPath: "admin",
			wantType:  TargetAdmin,
		},
		{
			name:      "admin server settings",
			path:      "/admin/server/settings",
			adminPath: "admin",
			wantType:  TargetAdminServer,
		},
		{
			name:      "api v1 users",
			path:      "/api/v1/users/john",
			adminPath: "admin",
			wantType:  TargetUser,
			wantName:  "john",
		},
		{
			name:      "api v1 public",
			path:      "/api/v1/search",
			adminPath: "admin",
			wantType:  TargetPublic,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := extractContextFromPath(tt.path, tt.adminPath)

			if ctx.Type != tt.wantType {
				t.Errorf("Type = %v, want %v", ctx.Type, tt.wantType)
			}
			if ctx.Name != tt.wantName {
				t.Errorf("Name = %q, want %q", ctx.Name, tt.wantName)
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
		{TargetAuth, "auth"},
		{TargetCurrentUser, "current_user"},
		{TargetUser, "user"},
		{TargetOrg, "org"},
		{TargetAdmin, "admin"},
		{TargetAdminServer, "admin_server"},
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

// Tests for parseTokenType

func TestParseTokenType(t *testing.T) {
	tests := []struct {
		token    string
		wantType TokenType
	}{
		{"adm_abcdefgh12345", TokenTypeAdmin},
		{"usr_abcdefgh12345", TokenTypeUser},
		{"org_abcdefgh12345", TokenTypeOrg},
		{"adm_agt_abcdefgh12345", TokenTypeAdminAgt},
		{"usr_agt_abcdefgh12345", TokenTypeUserAgt},
		{"org_agt_abcdefgh12345", TokenTypeOrgAgt},
		{"invalid_token", TokenTypeUnknown},
		{"", TokenTypeUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.token, func(t *testing.T) {
			got := parseTokenType(tt.token)
			if got != tt.wantType {
				t.Errorf("parseTokenType(%q) = %v, want %v", tt.token, got, tt.wantType)
			}
		})
	}
}

// Tests for validateTokenAccess

func TestValidateTokenAccess(t *testing.T) {
	tests := []struct {
		name      string
		tokenType TokenType
		ctx       *RequestContext
		wantErr   bool
	}{
		{
			name:      "public route always allowed",
			tokenType: TokenTypeAdmin,
			ctx:       &RequestContext{Type: TargetPublic},
			wantErr:   false,
		},
		{
			name:      "admin token can access admin panel",
			tokenType: TokenTypeAdmin,
			ctx:       &RequestContext{Type: TargetAdmin},
			wantErr:   false,
		},
		{
			name:      "admin token cannot access user routes",
			tokenType: TokenTypeAdmin,
			ctx:       &RequestContext{Type: TargetUser, Name: "john"},
			wantErr:   true,
		},
		{
			name:      "user token can access user routes",
			tokenType: TokenTypeUser,
			ctx:       &RequestContext{Type: TargetCurrentUser},
			wantErr:   false,
		},
		{
			name:      "user token cannot access admin",
			tokenType: TokenTypeUser,
			ctx:       &RequestContext{Type: TargetAdmin},
			wantErr:   true,
		},
		{
			name:      "org token can access org routes",
			tokenType: TokenTypeOrg,
			ctx:       &RequestContext{Type: TargetOrg, Name: "acme"},
			wantErr:   false,
		},
		{
			name:      "unknown token can access public",
			tokenType: TokenTypeUnknown,
			ctx:       &RequestContext{Type: TargetPublic},
			wantErr:   false,
		},
		{
			name:      "unknown token cannot access admin",
			tokenType: TokenTypeUnknown,
			ctx:       &RequestContext{Type: TargetAdmin},
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateTokenAccess(tt.tokenType, tt.ctx)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateTokenAccess() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// Tests for getTokenFromRequest

func TestGetTokenFromRequest(t *testing.T) {
	tests := []struct {
		name       string
		authHeader string
		apiKeyHdr  string
		query      string
		want       string
	}{
		{
			name:       "bearer token",
			authHeader: "Bearer mytoken123",
			want:       "mytoken123",
		},
		{
			name:      "x-api-key header",
			apiKeyHdr: "apikey456",
			want:      "apikey456",
		},
		{
			name:  "query parameter",
			query: "token=querytoken",
			want:  "querytoken",
		},
		{
			name: "no token",
			want: "",
		},
		{
			name:       "bearer takes priority",
			authHeader: "Bearer bearer",
			apiKeyHdr:  "apikey",
			want:       "bearer",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := "/"
			if tt.query != "" {
				path += "?" + tt.query
			}
			req := httptest.NewRequest(http.MethodGet, path, nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}
			if tt.apiKeyHdr != "" {
				req.Header.Set("X-API-Key", tt.apiKeyHdr)
			}

			got := getTokenFromRequest(req)
			if got != tt.want {
				t.Errorf("getTokenFromRequest() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestMetrics tests all Metrics functionality in a single test to avoid
// Prometheus duplicate collector registration panics (collectors are global)
func TestMetrics(t *testing.T) {
	cfg := &config.Config{}
	cfg.Server.Metrics.Enabled = true
	cfg.Server.Metrics.IncludeSystem = false
	cfg.Server.Metrics.Token = "secret-token"
	m := NewMetrics(cfg)

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
		m.SetActiveSessions(100)
		m.SetUserCounts(1000, 500)
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
		req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("Status without token = %d, want %d", rec.Code, http.StatusUnauthorized)
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
		{"/api/v1/search", "/api/v1/search"},
		{"/api/v1/users/123", "/api/v1/users/:id"},
		{"/api/v1/items/123e4567-e89b-12d3-a456-426614174000", "/api/v1/items/:id"},
		{"/static/css/main.css", "/static/css/main.css"},
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

func TestAuthPageDataStruct(t *testing.T) {
	data := AuthPageData{
		Error:        "test error",
		Success:      "test success",
		Username:     "testuser",
		Email:        "test@example.com",
		RequireEmail: true,
	}

	if data.Error != "test error" {
		t.Errorf("Error = %q", data.Error)
	}
	if data.Username != "testuser" {
		t.Errorf("Username = %q", data.Username)
	}
}

func TestSSOProviderStruct(t *testing.T) {
	provider := SSOProvider{
		Name:    "Google",
		ID:      "google",
		IconURL: "/static/icons/google.svg",
		URL:     "/auth/sso/google",
	}

	if provider.Name != "Google" {
		t.Errorf("Name = %q", provider.Name)
	}
	if provider.ID != "google" {
		t.Errorf("ID = %q", provider.ID)
	}
}

func TestTwoFactorPageDataStruct(t *testing.T) {
	data := TwoFactorPageData{
		Error:          "test error",
		SessionID:      "session123",
		RemainingKeys:  5,
		UseRecoveryKey: false,
	}

	if data.SessionID != "session123" {
		t.Errorf("SessionID = %q", data.SessionID)
	}
	if data.RemainingKeys != 5 {
		t.Errorf("RemainingKeys = %d", data.RemainingKeys)
	}
}

func TestRequestContextStruct(t *testing.T) {
	ctx := RequestContext{
		Type: TargetUser,
		Name: "john",
	}

	if ctx.Type != TargetUser {
		t.Errorf("Type = %v", ctx.Type)
	}
	if ctx.Name != "john" {
		t.Errorf("Name = %q", ctx.Name)
	}
}

func TestTokenInfoStruct(t *testing.T) {
	info := TokenInfo{
		Type:     TokenTypeUser,
		OwnerID:  123,
		Prefix:   "usr_1234",
		Scope:    "read-write",
		Username: "john",
	}

	if info.Type != TokenTypeUser {
		t.Errorf("Type = %v", info.Type)
	}
	if info.OwnerID != 123 {
		t.Errorf("OwnerID = %d", info.OwnerID)
	}
	if info.Username != "john" {
		t.Errorf("Username = %q", info.Username)
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

func TestGetTokenFromContext(t *testing.T) {
	// Test without context
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	token := GetTokenFromContext(req)
	if token != "" {
		t.Errorf("Token = %q, want empty", token)
	}
}

func TestGetTokenTypeFromContext(t *testing.T) {
	// Test without context
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	tokenType := GetTokenTypeFromContext(req)
	if tokenType != TokenTypeUnknown {
		t.Errorf("TokenType = %v, want %v", tokenType, TokenTypeUnknown)
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
		name       string
		cookie     string
		query      string
		wantTheme  string
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

func TestSetTheme(t *testing.T) {
	tests := []struct {
		theme     string
		wantValue string
	}{
		{"dark", "dark"},
		{"light", "light"},
		{"auto", "auto"},
		{"invalid", "dark"}, // Should default to dark
		{"", "dark"},        // Should default to dark
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
		{"auto", true, false, true}, // Auto defaults to dark on server
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
		name   string
		input  string
		want   int
	}{
		{"plain_text", "hello", 5},
		{"with_ansi_color", "\033[31mhello\033[0m", 5},
		{"multiple_ansi", "\033[1m\033[32mhi\033[0m", 2},
		{"empty", "", 0},
		{"only_ansi", "\033[31m\033[0m", 0},
		{"emoji", "🚀", 1}, // Note: emojis are counted as 1 rune
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

// Tests for session.go

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
	if len(session.ID) != 64 { // 32 bytes = 64 hex chars
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
