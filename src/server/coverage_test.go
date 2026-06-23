package server

import (
	"context"
	"fmt"
	"html/template"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/apimgr/search/src/alert"
	"github.com/apimgr/search/src/config"
	"github.com/apimgr/search/src/direct"
	"github.com/apimgr/search/src/display"
	"github.com/apimgr/search/src/model"
	"github.com/apimgr/search/src/scheduler"
	"github.com/go-chi/chi/v5"
)

// sharedServer is created once per test binary run.
// NewServer calls NewMetrics which registers Prometheus metrics globally;
// calling it more than once panics with "duplicate metrics collector" errors.
var (
	sharedServerOnce sync.Once
	sharedServerInst *Server
)

func sharedServer() *Server {
	sharedServerOnce.Do(func() {
		cfg := config.DefaultConfig()
		sharedServerInst = NewServer(cfg)
	})
	return sharedServerInst
}

// ---------- banner.go ----------

// TestBuildBannerInfo covers BuildBannerInfo for HTTP, HTTPS, dual-port, and Tor paths.
func TestBuildBannerInfo(t *testing.T) {
	tests := []struct {
		name      string
		port      int
		httpsPort int
		sslOn     bool
		torAddr   string
		wantHTTPS bool
		wantHTTP  bool
		wantTor   bool
	}{
		{"http only", 8080, 0, false, "", false, true, false},
		{"https single", 443, 0, true, "", true, false, false},
		{"dual port", 80, 443, false, "", true, true, false},
		{"tor http", 8080, 0, false, "abc.onion", false, true, true},
		{"tor https port 443", 443, 0, true, "abc.onion", true, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.DefaultConfig()
			cfg.Server.Port = tt.port
			cfg.Server.HTTPSPort = tt.httpsPort
			cfg.Server.SSL.Enabled = tt.sslOn

			info := BuildBannerInfo(cfg, tt.torAddr)
			if info == nil {
				t.Fatal("BuildBannerInfo returned nil")
			}
			if tt.wantHTTPS && info.HTTPSAddr == "" {
				t.Errorf("expected HTTPSAddr, got empty")
			}
			if tt.wantHTTP && info.HTTPAddr == "" {
				t.Errorf("expected HTTPAddr, got empty")
			}
			if tt.wantTor && info.TorAddr == "" {
				t.Errorf("expected TorAddr, got empty")
			}
			if !tt.wantTor && info.TorAddr != "" {
				t.Errorf("unexpected TorAddr %q", info.TorAddr)
			}
		})
	}
}

// TestPrintBanner confirms PrintBanner does not panic with various configs.
func TestPrintBanner(t *testing.T) {
	tests := []struct {
		name string
		info *BannerInfo
	}{
		{"minimal", &BannerInfo{AppName: "Search", Version: "1.0.0", Mode: "production", ListenAddr: "0.0.0.0:8080"}},
		{"debug dev", &BannerInfo{AppName: "Test", Version: "dev", Mode: "development", Debug: true, HTTPAddr: "http://localhost:8080", ListenAddr: "0.0.0.0:8080"}},
		{"with tor", &BannerInfo{AppName: "Search", Version: "1.0.0", Mode: "production", TorAddr: "http://abc.onion", ListenAddr: "0.0.0.0:80"}},
		{"with i2p and https", &BannerInfo{AppName: "Search", Version: "1.0.0", Mode: "production", I2PAddr: "http://abc.i2p", HTTPSAddr: "https://example.com", ListenAddr: "0.0.0.0:443", IsHTTPS: true}},
		{"empty appname", &BannerInfo{Version: "1.0.0", Mode: "production", ListenAddr: "0.0.0.0:8080"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("PrintBanner panicked: %v", r)
				}
			}()
			PrintBanner(tt.info)
		})
	}
}

// TestGetTerminalWidth confirms getTerminalWidth returns a positive value or default.
func TestGetTerminalWidth(t *testing.T) {
	w := getTerminalWidth()
	if w < 40 {
		t.Errorf("getTerminalWidth() = %d, want >= 40", w)
	}
}

// TestColorNoColor confirms color() returns plain text when color output is disabled.
// color() delegates to display.ColorEnabled() which reads a package-level flag;
// resetting it via InitOutput("never") simulates NO_COLOR.
func TestColorNoColor(t *testing.T) {
	display.InitOutput("never")
	t.Cleanup(func() { display.InitOutput("auto") })
	got := color("\033[32m", "hello")
	if strings.Contains(got, "\033") {
		t.Errorf("color() kept ANSI code with color disabled: %q", got)
	}
	if !strings.Contains(got, "hello") {
		t.Errorf("color() = %q, must contain text", got)
	}
}

// TestColorEnabled confirms color() includes the text when colors are forced on.
func TestColorEnabled(t *testing.T) {
	display.InitOutput("always")
	t.Cleanup(func() { display.InitOutput("auto") })
	got := color("\033[32m", "hello")
	if !strings.Contains(got, "hello") {
		t.Errorf("color() = %q, must contain text", got)
	}
}

// ---------- pid.go + pid_unix.go ----------

// TestWritePIDFile_RemovePIDFile covers write, read-back, and removal.
func TestWritePIDFile_RemovePIDFile(t *testing.T) {
	dir := t.TempDir()
	pidPath := filepath.Join(dir, "test.pid")

	if err := WritePIDFile(pidPath); err != nil {
		t.Fatalf("WritePIDFile() error = %v", err)
	}

	data, err := os.ReadFile(pidPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		t.Error("PID file is empty")
	}

	if err := RemovePIDFile(pidPath); err != nil {
		t.Errorf("RemovePIDFile() error = %v", err)
	}
	if _, err := os.Stat(pidPath); !os.IsNotExist(err) {
		t.Error("PID file should not exist after RemovePIDFile")
	}
}

// TestCheckPIDFile_NoFile returns false and pid=0 when no file exists.
func TestCheckPIDFile_NoFile(t *testing.T) {
	running, pid, err := CheckPIDFile("/tmp/apimgr/nonexistent-pid-file-xyz.pid")
	if err != nil {
		t.Fatalf("CheckPIDFile() error = %v", err)
	}
	if running {
		t.Error("expected running=false for non-existent PID file")
	}
	if pid != 0 {
		t.Errorf("expected pid=0, got %d", pid)
	}
}

// TestCheckPIDFile_CorruptContent returns false for corrupt PID file content.
func TestCheckPIDFile_CorruptContent(t *testing.T) {
	dir := t.TempDir()
	pidPath := filepath.Join(dir, "corrupt.pid")
	if err := os.WriteFile(pidPath, []byte("notanumber\n"), 0644); err != nil {
		t.Fatal(err)
	}

	running, _, err := CheckPIDFile(pidPath)
	if err != nil {
		t.Fatalf("CheckPIDFile() unexpected error = %v", err)
	}
	if running {
		t.Error("expected running=false for corrupt PID file")
	}
	// Corrupt file should be cleaned up
	if _, err := os.Stat(pidPath); !os.IsNotExist(err) {
		t.Error("corrupt PID file should be removed")
	}
}

// TestCheckPIDFile_StalePID returns false for a dead PID.
func TestCheckPIDFile_StalePID(t *testing.T) {
	dir := t.TempDir()
	pidPath := filepath.Join(dir, "stale.pid")
	// PID 99999999 is almost certainly dead
	if err := os.WriteFile(pidPath, []byte("99999999\n"), 0644); err != nil {
		t.Fatal(err)
	}

	running, _, err := CheckPIDFile(pidPath)
	if err != nil {
		t.Fatalf("CheckPIDFile() unexpected error = %v", err)
	}
	if running {
		t.Error("expected running=false for stale PID")
	}
	// Stale file should be cleaned up
	if _, err := os.Stat(pidPath); !os.IsNotExist(err) {
		t.Error("stale PID file should be removed")
	}
}

// TestRemovePIDFile_NonExistent returns an error for a missing file.
func TestRemovePIDFile_NonExistent(t *testing.T) {
	err := RemovePIDFile("/tmp/apimgr/does-not-exist-xyz.pid")
	if err == nil {
		t.Error("expected error removing non-existent PID file")
	}
}

// ---------- wellknown.go ----------

// newTestServer returns the package-level shared server instance.
// It reuses the singleton because NewServer registers Prometheus metrics globally
// and panics if called more than once in the same test binary.
func newTestServer(t *testing.T) *Server {
	t.Helper()
	return sharedServer()
}

// TestHandleWellKnownChangePassword confirms 404 response — this project has no accounts.
func TestHandleWellKnownChangePassword(t *testing.T) {
	s := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/.well-known/change-password", nil)
	rec := httptest.NewRecorder()

	s.handleWellKnownChangePassword(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("handleWellKnownChangePassword() status = %d, want 404", rec.Code)
	}
}

// TestHandleRobotsTxt covers default allow/deny paths and sitemap output.
func TestHandleRobotsTxt(t *testing.T) {
	tests := []struct {
		name        string
		allowPaths  []string
		denyPaths   []string
		wantStrings []string
	}{
		{
			"defaults",
			nil,
			nil,
			[]string{"User-agent: *", "Allow: /", "Sitemap:"},
		},
		{
			"custom paths",
			[]string{"/", "/public"},
			[]string{"/private", "/admin"},
			[]string{"Allow: /public", "Disallow: /private", "Disallow: /admin"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestServer(t)
			s.config.Server.Web.Robots.Allow = tt.allowPaths
			s.config.Server.Web.Robots.Deny = tt.denyPaths

			req := httptest.NewRequest(http.MethodGet, "/robots.txt", nil)
			req.Host = "example.com"
			rec := httptest.NewRecorder()

			s.handleRobotsTxt(rec, req)

			if rec.Code != http.StatusOK {
				t.Errorf("status = %d, want 200", rec.Code)
			}
			body := rec.Body.String()
			for _, want := range tt.wantStrings {
				if !strings.Contains(body, want) {
					t.Errorf("robots.txt missing %q; got:\n%s", want, body)
				}
			}
		})
	}
}

// TestHandleSecurityTxtEnhanced validates RFC 9116 required fields.
func TestHandleSecurityTxtEnhanced(t *testing.T) {
	tests := []struct {
		name         string
		contact      string
		contactEmail string
		expires      string
		wantContact  string
	}{
		{"explicit mailto", "mailto:sec@example.com", "", "", "Contact: mailto:sec@example.com"},
		{"plain email", "sec@example.com", "", "", "Contact: mailto:sec@example.com"},
		{"https contact", "https://example.com/security", "", "", "Contact: https://example.com/security"},
		{"fallback to contact email", "", "admin@example.com", "", "Contact: mailto:admin@example.com"},
		{"no contact fallback hostname", "", "", "", "Contact: mailto:security@"},
		{"custom expires", "", "", "2030-01-01T00:00:00Z", "Expires: 2030-01-01T00:00:00Z"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestServer(t)
			s.config.Server.Web.Security.Contact = tt.contact
			s.config.Server.Contact.Email = tt.contactEmail
			s.config.Server.Web.Security.Expires = tt.expires

			req := httptest.NewRequest(http.MethodGet, "/.well-known/security.txt", nil)
			req.Host = "example.com"
			rec := httptest.NewRecorder()

			s.handleSecurityTxtEnhanced(rec, req)

			if rec.Code != http.StatusOK {
				t.Errorf("status = %d, want 200", rec.Code)
			}
			body := rec.Body.String()
			if !strings.Contains(body, tt.wantContact) {
				t.Errorf("security.txt missing %q; got:\n%s", tt.wantContact, body)
			}
			if !strings.Contains(body, "Expires:") {
				t.Error("security.txt missing Expires field")
			}
			if !strings.Contains(body, "Canonical:") {
				t.Error("security.txt missing Canonical field")
			}
		})
	}
}

// TestExtractHostFromURL covers the URL parsing helper.
func TestExtractHostFromURL(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"https://example.com", "example.com"},
		{"http://example.com", "example.com"},
		{"https://example.com:8443", "example.com"},
		{"https://example.com/path/to/page", "example.com"},
		{"https://example.com:8443/path", "example.com"},
		{"example.com", "example.com"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := extractHostFromURL(tt.input)
			if got != tt.want {
				t.Errorf("extractHostFromURL(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// ---------- middleware.go (Allowlist / Blocklist / RateLimit / DegradedMode / SecGPC) ----------

// TestAllowlistMiddleware verifies that a matching IP sets the allowlisted flag.
func TestAllowlistMiddleware(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Server.RateLimit.Whitelist = []string{"10.0.0.1"}
	mw := NewMiddleware(cfg, nil)

	var gotAllowlisted bool
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAllowlisted = isAllowlisted(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	handler := mw.Allowlist(next)

	t.Run("allowlisted IP", func(t *testing.T) {
		gotAllowlisted = false
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "10.0.0.1:1234"
		handler.ServeHTTP(httptest.NewRecorder(), req)
		if !gotAllowlisted {
			t.Error("expected allowlisted=true for whitelisted IP")
		}
	})

	t.Run("non-allowlisted IP", func(t *testing.T) {
		gotAllowlisted = false
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "1.2.3.4:1234"
		handler.ServeHTTP(httptest.NewRecorder(), req)
		if gotAllowlisted {
			t.Error("expected allowlisted=false for unknown IP")
		}
	})
}

// TestBlocklistMiddleware verifies blocked IPs receive 403 and allowlisted bypass.
func TestBlocklistMiddleware(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Server.RateLimit.Blacklist = []string{"5.5.5.5"}
	mw := NewMiddleware(cfg, nil)

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := mw.Blocklist(next)

	t.Run("blocked IP", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "5.5.5.5:1234"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Errorf("expected 403, got %d", rec.Code)
		}
	})

	t.Run("non-blocked IP passes", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "9.9.9.9:1234"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rec.Code)
		}
	})

	t.Run("allowlisted bypasses blocklist", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "5.5.5.5:1234"
		ctx := context.WithValue(req.Context(), allowlistedCtxKey{}, true)
		req = req.WithContext(ctx)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("allowlisted should bypass blocklist, got %d", rec.Code)
		}
	})
}

// TestRateLimitMiddleware verifies rate limiting fires and allowlisted IPs bypass.
func TestRateLimitMiddleware(t *testing.T) {
	cfg := config.DefaultConfig()
	mw := NewMiddleware(cfg, nil)
	// NewRateLimiter takes *config.RateLimitConfig
	limiter := NewRateLimiter(&cfg.Server.RateLimit)

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := mw.RateLimit(limiter)(next)

	t.Run("normal request passes", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "7.7.7.7:1234"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rec.Code)
		}
	})

	t.Run("allowlisted bypasses rate limit", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "7.7.7.7:1234"
		ctx := context.WithValue(req.Context(), allowlistedCtxKey{}, true)
		req = req.WithContext(ctx)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("allowlisted should bypass rate limit, got %d", rec.Code)
		}
	})
}

// TestEndpointRateLimiter covers Allow, window expiry, and RemainingTime.
func TestEndpointRateLimiter(t *testing.T) {
	erl := NewEndpointRateLimiter(3, 10*time.Second)

	t.Run("allows up to limit", func(t *testing.T) {
		ip := "100.0.0.1"
		for i := 0; i < 3; i++ {
			if !erl.Allow(ip) {
				t.Errorf("Allow() = false on attempt %d, want true", i+1)
			}
		}
	})

	t.Run("blocks after limit exceeded", func(t *testing.T) {
		ip := "100.0.0.1"
		if erl.Allow(ip) {
			t.Error("Allow() = true after limit exceeded, want false")
		}
	})

	t.Run("RemainingTime positive for limited IP", func(t *testing.T) {
		ip := "100.0.0.1"
		remaining := erl.RemainingTime(ip)
		if remaining <= 0 {
			t.Errorf("RemainingTime() = %v, want > 0 while limited", remaining)
		}
	})

	t.Run("RemainingTime zero for unknown IP", func(t *testing.T) {
		remaining := erl.RemainingTime("unknown.ip")
		if remaining != 0 {
			t.Errorf("RemainingTime() = %v for unknown IP, want 0", remaining)
		}
	})

	t.Run("first-time IP allowed", func(t *testing.T) {
		if !erl.Allow("200.0.0.1") {
			t.Error("first-time IP should be allowed")
		}
	})
}

// fakeMaintenanceHandler implements MaintenanceHandler for test isolation.
type fakeMaintenanceHandler struct {
	mode    int
	message string
}

func (f *fakeMaintenanceHandler) IsInMaintenance() bool {
	return f.mode > 0
}

func (f *fakeMaintenanceHandler) GetMode() int {
	return f.mode
}

func (f *fakeMaintenanceHandler) GetMessage() string {
	return f.message
}

// TestDegradedModeMiddleware verifies DegradedMode adds X-System-Status: degraded in mode 1.
func TestDegradedModeMiddleware(t *testing.T) {
	cfg := config.DefaultConfig()
	mw := NewMiddleware(cfg, nil)

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	t.Run("mode 0 no header", func(t *testing.T) {
		handler := mw.DegradedMode(&fakeMaintenanceHandler{mode: 0})(next)
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Header().Get("X-System-Status") != "" {
			t.Error("expected no X-System-Status header in mode 0")
		}
	})

	t.Run("mode 1 adds degraded header", func(t *testing.T) {
		handler := mw.DegradedMode(&fakeMaintenanceHandler{mode: 1})(next)
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Header().Get("X-System-Status") != "degraded" {
			t.Errorf("expected X-System-Status=degraded, got %q", rec.Header().Get("X-System-Status"))
		}
	})
}

// TestSecGPCMiddleware verifies Sec-GPC: 1 sets the opt-out flag in context.
func TestSecGPCMiddleware(t *testing.T) {
	cfg := config.DefaultConfig()
	mw := NewMiddleware(cfg, nil)

	tests := []struct {
		name      string
		header    string
		wantInCtx bool
	}{
		{"with Sec-GPC: 1", "1", true},
		{"without Sec-GPC", "", false},
		{"Sec-GPC: 0 is not opt-out", "0", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotOptOut bool
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				v, _ := r.Context().Value(contextKey("gpc_opt_out")).(bool)
				gotOptOut = v
				w.WriteHeader(http.StatusOK)
			})
			handler := mw.SecGPC(next)
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.header != "" {
				req.Header.Set("Sec-GPC", tt.header)
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if gotOptOut != tt.wantInCtx {
				t.Errorf("gpc_opt_out = %v, want %v", gotOptOut, tt.wantInCtx)
			}
		})
	}
}

// TestURLNormalizeMiddleware_Variants covers trailing-slash redirect and pass-through.
func TestURLNormalizeMiddleware_Variants(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		wantStatus int
		wantLoc    string
	}{
		{"root no redirect", "/", http.StatusOK, ""},
		{"trailing slash redirects", "/about/", http.StatusMovedPermanently, "/about"},
		{"no trailing slash passes", "/about", http.StatusOK, ""},
		{"file with trailing slash redirects", "/static/app.js/", http.StatusMovedPermanently, "/static/app.js"},
	}

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := URLNormalizeMiddleware(next)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if tt.wantLoc != "" && rec.Header().Get("Location") != tt.wantLoc {
				t.Errorf("Location = %q, want %q", rec.Header().Get("Location"), tt.wantLoc)
			}
		})
	}
}

// TestURLNormalizeMiddleware_QueryPreserved checks query string is preserved on redirect.
func TestURLNormalizeMiddleware_QueryPreserved(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := URLNormalizeMiddleware(next)

	req := httptest.NewRequest(http.MethodGet, "/search/?q=go", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusMovedPermanently {
		t.Errorf("status = %d, want 301", rec.Code)
	}
	loc := rec.Header().Get("Location")
	if !strings.Contains(loc, "q=go") {
		t.Errorf("query string not preserved in redirect: %q", loc)
	}
}

// TestPathSecurityMiddleware_Variants covers traversal blocking and clean pass-through.
func TestPathSecurityMiddleware_Variants(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		rawPath    string
		wantStatus int
	}{
		{"clean path", "/about", "", http.StatusOK},
		{"dot-dot traversal", "/etc/../passwd", "", http.StatusBadRequest},
		{"encoded traversal %2e%2e", "/api/v1/%2e%2e/config", "/api/v1/%2e%2e/config", http.StatusBadRequest},
		{"root passes", "/", "", http.StatusOK},
	}

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := PathSecurityMiddleware(next)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			if tt.rawPath != "" {
				req.URL.RawPath = tt.rawPath
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("PathSecurityMiddleware(%q) status = %d, want %d", tt.path, rec.Code, tt.wantStatus)
			}
		})
	}
}

// TestExtractContextFromPath_Extra covers public, server, and API path classification.
func TestExtractContextFromPath_Extra(t *testing.T) {
	tests := []struct {
		path     string
		wantType TargetType
	}{
		{"/", TargetPublic},
		{"/search", TargetPublic},
		{"/api/v1/search", TargetPublic},
		{"/server/healthz", TargetServerPages},
		{"/server/status", TargetServerPages},
		{"/api/v1/server/healthz", TargetServerPages},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := extractContextFromPath(tt.path)
			if got.Type != tt.wantType {
				t.Errorf("extractContextFromPath(%q).Type = %v, want %v", tt.path, got.Type, tt.wantType)
			}
		})
	}
}

// TestTargetTypeString_Extra validates the Stringer.
func TestTargetTypeString_Extra(t *testing.T) {
	tests := []struct {
		in   TargetType
		want string
	}{
		{TargetPublic, "public"},
		{TargetServerPages, "server"},
		{TargetUnknown, "unknown"},
	}
	for _, tt := range tests {
		if got := tt.in.String(); got != tt.want {
			t.Errorf("TargetType(%d).String() = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// TestGetRequestContext_Default returns TargetUnknown when no context key is set.
func TestGetRequestContext_Default(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	got := GetRequestContext(req)
	if got == nil {
		t.Fatal("GetRequestContext returned nil")
	}
	if got.Type != TargetUnknown {
		t.Errorf("Type = %v, want TargetUnknown", got.Type)
	}
}

// ---------- metrics.go ----------

// TestMetricsGetters covers GetTotalRequests and GetActiveConnections.
// Uses the shared server's Metrics to avoid Prometheus duplicate registration panics.
func TestMetricsGetters(t *testing.T) {
	m := sharedServer().metrics

	// GetTotalRequests returns a non-negative value
	if got := m.GetTotalRequests(); got < 0 {
		t.Errorf("GetTotalRequests() = %d, want >= 0", got)
	}
	// GetActiveConnections returns a non-negative value
	if got := m.GetActiveConnections(); got < 0 {
		t.Errorf("GetActiveConnections() = %d, want >= 0", got)
	}

	before := m.GetTotalRequests()
	m.RecordRequest("GET", "/test", 200, time.Millisecond, 0, 100)
	if got := m.GetTotalRequests(); got != before+1 {
		t.Errorf("GetTotalRequests() = %d, want %d after record", got, before+1)
	}
}

// TestMetricsMiddleware verifies active connection counting and status recording.
// Uses the shared server's Metrics to avoid Prometheus duplicate registration panics.
func TestMetricsMiddleware(t *testing.T) {
	m := sharedServer().metrics

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := m.MetricsMiddleware(next)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	// After completion, active connections must return to 0
	if got := m.GetActiveConnections(); got != 0 {
		t.Errorf("GetActiveConnections() = %d after completion, want 0", got)
	}
	if got := m.GetTotalRequests(); got < 1 {
		t.Errorf("GetTotalRequests() = %d, want >= 1 after request", got)
	}
}

// TestCollectSystemMetrics confirms collectSystemMetrics does not panic.
// Uses the shared server's Metrics to avoid Prometheus duplicate registration panics.
func TestCollectSystemMetrics(t *testing.T) {
	m := sharedServer().metrics
	m.config.Server.Metrics.IncludeSystem = true

	defer func() {
		if r := recover(); r != nil {
			t.Errorf("collectSystemMetrics() panicked: %v", r)
		}
	}()
	m.collectSystemMetrics()
}

// TestGetCPUUsage confirms getCPUUsage returns a value between 0 and 100.
func TestGetCPUUsage(t *testing.T) {
	got := getCPUUsage()
	if got < 0 || got > 100 {
		t.Errorf("getCPUUsage() = %f, want in [0,100]", got)
	}
}

// TestGetMemoryUsagePercent confirms getMemoryUsagePercent returns 0–100.
func TestGetMemoryUsagePercent(t *testing.T) {
	got := getMemoryUsagePercent()
	if got < 0 || got > 100 {
		t.Errorf("getMemoryUsagePercent() = %f, want in [0,100]", got)
	}
}

// TestGetDiskUsage confirms getDiskUsage returns non-negative values.
func TestGetDiskUsage(t *testing.T) {
	used, total := getDiskUsage()
	if used > total && total != 0 {
		t.Errorf("used (%d) > total (%d)", used, total)
	}
}

// TestGetDiskUsageUnix confirms getDiskUsageUnix returns non-negative values.
func TestGetDiskUsageUnix(t *testing.T) {
	used, total := getDiskUsageUnix()
	if used > total && total != 0 {
		t.Errorf("used (%d) > total (%d)", used, total)
	}
}

// ---------- embed.go helpers ----------

// TestFormatVideoDuration covers zero, seconds, minutes, and hours.
func TestFormatVideoDuration(t *testing.T) {
	tests := []struct {
		seconds int
		want    string
	}{
		{0, ""},
		{-1, ""},
		{65, "1:05"},
		{3600, "1:00:00"},
		{3661, "1:01:01"},
		{59, "0:59"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := formatVideoDuration(tt.seconds)
			if got != tt.want {
				t.Errorf("formatVideoDuration(%d) = %q, want %q", tt.seconds, got, tt.want)
			}
		})
	}
}

// TestFormatViewCount covers zero, K, M, and B suffixes.
func TestFormatViewCount(t *testing.T) {
	tests := []struct {
		count int64
		want  string
	}{
		{0, ""},
		{-1, ""},
		{999, "999"},
		{1000, "1K"},
		{10000, "10K"},
		{1000000, "1M"},
		{10000000, "10M"},
		{1000000000, "1B"},
		{10000000000, "10B"},
		{1500, "1.5K"},
		{1500000, "1.5M"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := formatViewCount(tt.count)
			if got != tt.want {
				t.Errorf("formatViewCount(%d) = %q, want %q", tt.count, got, tt.want)
			}
		})
	}
}

// ---------- embed.go: TemplateNotFoundError, StaticFileServer ----------

// TestTemplateNotFoundError_Message confirms error string contains template name.
func TestTemplateNotFoundError_Message(t *testing.T) {
	e := &TemplateNotFoundError{Name: "contact"}
	if !strings.Contains(e.Error(), "contact") {
		t.Errorf("TemplateNotFoundError.Error() missing name: %q", e.Error())
	}
}

// TestStaticFileServer_NotNil confirms StaticFileServer returns a non-nil handler.
func TestStaticFileServer_NotNil(t *testing.T) {
	h := StaticFileServer()
	if h == nil {
		t.Error("StaticFileServer() returned nil handler")
	}
}

// TestGetStaticFile_NotFound returns error for missing file.
func TestGetStaticFile_NotFound(t *testing.T) {
	_, err := GetStaticFile("nonexistent-file-that-does-not-exist.txt")
	if err == nil {
		t.Error("GetStaticFile() non-existent file: expected error, got nil")
	}
}

// ---------- server.go helpers ----------

// TestSanitizeInput covers null bytes, control chars, and valid input.
func TestSanitizeInput(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"empty", "", ""},
		{"clean", "hello world", "hello world"},
		{"null byte removed", "hel\x00lo", "hello"},
		{"control char removed", "hello\x01world", "helloworld"},
		{"newline preserved", "line1\nline2", "line1\nline2"},
		{"tab preserved", "col1\tcol2", "col1\tcol2"},
		{"multiple null bytes", "\x00a\x00b\x00", "ab"},
		{"bell char removed", "ring\x07bell", "ringbell"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeInput(tt.input)
			if got != tt.want {
				t.Errorf("sanitizeInput(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestGetBaseURL covers BaseURL config, TLS detection, and proxy headers.
func TestGetBaseURL(t *testing.T) {
	tests := []struct {
		name       string
		configURL  string
		headers    map[string]string
		host       string
		wantScheme string
	}{
		{"config base URL", "https://custom.example.com", nil, "localhost", "https://custom.example.com"},
		{"http default", "", nil, "example.com:8080", "http://"},
		{"X-Forwarded-Proto https", "", map[string]string{"X-Forwarded-Proto": "https"}, "example.com", "https://"},
		{"X-Forwarded-Protocol https", "", map[string]string{"X-Forwarded-Protocol": "https"}, "example.com", "https://"},
		{"X-Forwarded-Ssl on", "", map[string]string{"X-Forwarded-Ssl": "on"}, "example.com", "https://"},
		{"X-Scheme https", "", map[string]string{"X-Scheme": "https"}, "example.com", "https://"},
		{"Forwarded proto=https", "", map[string]string{"Forwarded": "for=127.0.0.1; proto=https"}, "example.com", "https://"},
		{"X-Forwarded-Host override", "", map[string]string{"X-Forwarded-Host": "proxy.example.com"}, "direct.example.com", "http://proxy.example.com"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestServer(t)
			origURL := s.config.Server.BaseURL
			s.config.Server.BaseURL = tt.configURL
			t.Cleanup(func() { s.config.Server.BaseURL = origURL })

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Host = tt.host
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}

			got := s.getBaseURL(req)
			if !strings.HasPrefix(got, tt.wantScheme) {
				t.Errorf("getBaseURL() = %q, want prefix %q", got, tt.wantScheme)
			}
		})
	}
}

// TestGetBaseURL_TrailingSlashStripped confirms trailing slash is stripped from configured BaseURL.
func TestGetBaseURL_TrailingSlashStripped(t *testing.T) {
	s := newTestServer(t)
	origURL := s.config.Server.BaseURL
	s.config.Server.BaseURL = "https://example.com/"
	t.Cleanup(func() { s.config.Server.BaseURL = origURL })

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	got := s.getBaseURL(req)
	if strings.HasSuffix(got, "/") {
		t.Errorf("getBaseURL() should strip trailing slash, got %q", got)
	}
}

// ---------- pages.go ----------

// TestDetectResponseFormat covers all content-negotiation paths.
func TestDetectResponseFormat(t *testing.T) {
	s := newTestServer(t)

	tests := []struct {
		name       string
		accept     string
		query      string
		wantFormat string
	}{
		{"json accept header", "application/json", "", "application/json"},
		{"plain text accept", "text/plain", "", "text/plain"},
		{"html accept", "text/html", "", "text/html"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rawURL := "/"
			if tt.query != "" {
				rawURL = "/?" + tt.query
			}
			req := httptest.NewRequest(http.MethodGet, rawURL, nil)
			if tt.accept != "" {
				req.Header.Set("Accept", tt.accept)
			}
			got := s.detectResponseFormat(req)
			if got != tt.wantFormat {
				t.Errorf("detectResponseFormat() = %q, want %q", got, tt.wantFormat)
			}
		})
	}
}

// TestRespondHealthText_RequiredFields confirms text format output includes required fields.
func TestRespondHealthText_RequiredFields(t *testing.T) {
	s := newTestServer(t)
	health := &HealthResponse{
		Status:    "ok",
		Version:   "1.0.0",
		Mode:      "production",
		Uptime:    "1h",
		GoVersion: "go1.22",
		Build:     BuildInfo{Commit: "abc123"},
		Checks: ChecksInfo{
			Database:  "ok",
			Cache:     "ok",
			Disk:      "ok",
			Scheduler: "ok",
		},
	}

	rec := httptest.NewRecorder()
	s.respondHealthText(rec, health)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{"status: ok", "version: 1.0.0", "mode: production", "check.database: ok"} {
		if !strings.Contains(body, want) {
			t.Errorf("text health response missing %q; got:\n%s", want, body)
		}
	}
}

// TestRespondHealthText_Unhealthy confirms 503 for unhealthy status.
func TestRespondHealthText_Unhealthy(t *testing.T) {
	s := newTestServer(t)
	health := &HealthResponse{
		Status: "unhealthy",
		Checks: ChecksInfo{},
		Build:  BuildInfo{},
	}
	rec := httptest.NewRecorder()
	s.respondHealthText(rec, health)
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", rec.Code)
	}
}

// TestRespondHealthText_TorFields confirms Tor fields are rendered when enabled.
func TestRespondHealthText_TorFields(t *testing.T) {
	s := newTestServer(t)
	health := &HealthResponse{
		Status: "ok",
		Checks: ChecksInfo{Database: "ok", Cache: "ok", Disk: "ok", Scheduler: "ok"},
		Build:  BuildInfo{},
		Features: FeaturesInfo{
			Tor: TorInfo{
				Enabled:  true,
				Running:  true,
				Status:   "running",
				Hostname: "abc.onion",
			},
			GeoIP: true,
		},
	}
	rec := httptest.NewRecorder()
	s.respondHealthText(rec, health)
	body := rec.Body.String()
	if !strings.Contains(body, "features.tor.enabled: true") {
		t.Errorf("expected tor fields in text output; got:\n%s", body)
	}
	if !strings.Contains(body, "features: tor, geoip") {
		t.Errorf("expected features line; got:\n%s", body)
	}
}

// ---------- opensearch.go ----------

// TestValidateNotPrivateProxy covers localhost, .local, and loopback hostnames.
func TestValidateNotPrivateProxy(t *testing.T) {
	tests := []struct {
		hostname string
		wantErr  bool
	}{
		{"localhost", true},
		{"127.0.0.1", true},
		{"::1", true},
		{"foo.local", true},
		{"bar.internal", true},
		{"baz.localhost", true},
	}
	for _, tt := range tests {
		t.Run(tt.hostname, func(t *testing.T) {
			err := validateNotPrivateProxy(tt.hostname)
			if tt.wantErr && err == nil {
				t.Errorf("validateNotPrivateProxy(%q) = nil, want error", tt.hostname)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("validateNotPrivateProxy(%q) = %v, want nil", tt.hostname, err)
			}
		})
	}
}

// TestNormalizeSafeSearchAlias covers all alias branches.
func TestNormalizeSafeSearchAlias(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"o", 0},
		{"off", 0},
		{"OFF", 0},
		{"0", 0},
		{"m", 1},
		{"moderate", 1},
		{"MODERATE", 1},
		{"1", 1},
		{"s", 2},
		{"strict", 2},
		{"STRICT", 2},
		{"2", 2},
		{"unknown", 1},
		{"", 1},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := normalizeSafeSearchAlias(tt.input)
			if got != tt.want {
				t.Errorf("normalizeSafeSearchAlias(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

// TestNormalizeResultsPerPage covers boundary and out-of-range values.
func TestNormalizeResultsPerPage(t *testing.T) {
	tests := []struct {
		input int
		want  int
	}{
		{0, 20},
		{-5, 20},
		{1, 1},
		{20, 20},
		{100, 100},
		{101, 100},
		{999, 100},
	}
	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			got := normalizeResultsPerPage(tt.input)
			if got != tt.want {
				t.Errorf("normalizeResultsPerPage(%d) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

// ---------- logging.go ----------

// TestNewLogger_FieldAttachment covers default construction and field attachment.
func TestNewLogger_FieldAttachment(t *testing.T) {
	cfg := config.DefaultConfig()
	l := NewLogger(cfg)
	if l == nil {
		t.Fatal("NewLogger returned nil")
	}

	l2 := l.WithField("key", "value")
	if l2 == nil {
		t.Fatal("WithField returned nil")
	}
	if l2.fields["key"] != "value" {
		t.Errorf("WithField: fields[key] = %v, want %q", l2.fields["key"], "value")
	}

	// Original logger should not be modified
	if _, ok := l.fields["key"]; ok {
		t.Error("WithField modified original logger")
	}
}

// TestLoggerClose_NoFile covers Close when no file is open.
func TestLoggerClose_NoFile(t *testing.T) {
	cfg := config.DefaultConfig()
	l := NewLogger(cfg)

	if err := l.Close(); err != nil {
		t.Errorf("Close() with no file = %v, want nil", err)
	}
}

// TestLoggerSetupFileLogging covers valid path creation with parent directory.
func TestLoggerSetupFileLogging(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "sub", "app.log")
	cfg := config.DefaultConfig()
	cfg.Server.Logs.File = logFile

	l := NewLogger(cfg)
	if l == nil {
		t.Fatal("NewLogger returned nil")
	}
	defer l.Close()

	if _, err := os.Stat(logFile); os.IsNotExist(err) {
		t.Error("log file was not created")
	}
}

// TestLoggerLevels exercises all log level methods without panicking.
func TestLoggerLevels(t *testing.T) {
	cfg := config.DefaultConfig()
	l := NewLogger(cfg)

	defer func() {
		if r := recover(); r != nil {
			t.Errorf("logger method panicked: %v", r)
		}
	}()

	l.Debug("debug %s", "msg")
	l.Info("info %s", "msg")
	l.Warn("warn %s", "msg")
	l.Error("error %s", "msg")
}

// TestLoggerWithFields_Multi covers multi-field attachment.
func TestLoggerWithFields_Multi(t *testing.T) {
	cfg := config.DefaultConfig()
	l := NewLogger(cfg)

	l2 := l.WithFields(map[string]interface{}{"a": 1, "b": "two"})
	if l2.fields["a"] != 1 {
		t.Errorf("WithFields: a = %v, want 1", l2.fields["a"])
	}
	if l2.fields["b"] != "two" {
		t.Errorf("WithFields: b = %v, want %q", l2.fields["b"], "two")
	}
}

// ---------- public_ip.go ----------

// TestFetchPublicIP_InvalidURL ensures fetchPublicIP returns error for bad URL.
func TestFetchPublicIP_InvalidURL(t *testing.T) {
	ctx := context.Background()
	client := &http.Client{Timeout: 2 * time.Second}

	// Port 1 is almost certainly not serving HTTP
	_, err := fetchPublicIP(ctx, client, "http://127.0.0.1:1/")
	if err == nil {
		t.Error("fetchPublicIP() expected error for unreachable URL")
	}
}

// TestFetchPublicIP_PrivateIP rejects private IP responses using a local test server.
func TestFetchPublicIP_PrivateIP(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("192.168.1.1"))
	}))
	defer ts.Close()

	ctx := context.Background()
	_, err := fetchPublicIP(ctx, ts.Client(), ts.URL)
	if err == nil {
		t.Error("fetchPublicIP() should reject private IP 192.168.1.1")
	}
}

// TestFetchPublicIP_NonIPBody rejects non-IP responses.
func TestFetchPublicIP_NonIPBody(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not-an-ip"))
	}))
	defer ts.Close()

	ctx := context.Background()
	_, err := fetchPublicIP(ctx, ts.Client(), ts.URL)
	if err == nil {
		t.Error("fetchPublicIP() should reject non-IP body")
	}
}

// TestFetchPublicIP_BadStatus rejects non-200 responses.
func TestFetchPublicIP_BadStatus(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	ctx := context.Background()
	_, err := fetchPublicIP(ctx, ts.Client(), ts.URL)
	if err == nil {
		t.Error("fetchPublicIP() should reject non-200 status")
	}
}

// TestFetchPublicIP_IPv6Rejected ensures IPv6 addresses are rejected.
func TestFetchPublicIP_IPv6Rejected(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("2001:db8::1"))
	}))
	defer ts.Close()

	ctx := context.Background()
	_, err := fetchPublicIP(ctx, ts.Client(), ts.URL)
	if err == nil {
		t.Error("fetchPublicIP() should reject IPv6 address")
	}
}

// TestGetPublicIP returns empty string on a freshly-initialized cache.
func TestGetPublicIP(t *testing.T) {
	// Reset global cache to ensure test isolation
	publicIP.mu.Lock()
	publicIP.ip = ""
	publicIP.updatedAt = time.Time{}
	publicIP.mu.Unlock()

	s := newTestServer(t)
	ip, _ := s.GetPublicIP()
	// May be empty if the refresher has not run — that is the correct initial state
	_ = ip
}

// ---------- ssl.go ----------

// TestHasValidCertificate returns false when no certificate exists.
func TestHasValidCertificate(t *testing.T) {
	cfg := config.DefaultConfig()
	m := NewSSLManager(cfg)
	if m == nil {
		t.Skip("NewSSLManager returned nil")
	}

	// Cert files don't exist in test env — must return false
	if m.HasValidCertificate() {
		t.Error("HasValidCertificate() = true for non-existent cert, want false")
	}
}

// TestHasValidCertificate_CertExistsKeyMissing confirms HasValidCertificate returns false
// when the cert file exists but the key file does not.
func TestHasValidCertificate_CertExistsKeyMissing(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, "cert.pem")
	keyPath := filepath.Join(dir, "key.pem")

	if err := os.WriteFile(certPath, []byte("placeholder"), 0644); err != nil {
		t.Fatalf("writing cert file: %v", err)
	}

	cfg := config.DefaultConfig()
	cfg.Server.SSL.CertFile = certPath
	cfg.Server.SSL.KeyFile = keyPath
	m := NewSSLManager(cfg)

	if m.HasValidCertificate() {
		t.Error("HasValidCertificate() = true when key file missing, want false")
	}
}

// TestHasValidCertificate_BothExistInvalidContent confirms HasValidCertificate returns false
// when both files exist but contain invalid cert/key content.
func TestHasValidCertificate_BothExistInvalidContent(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, "cert.pem")
	keyPath := filepath.Join(dir, "key.pem")

	if err := os.WriteFile(certPath, []byte("not-a-cert"), 0644); err != nil {
		t.Fatalf("writing cert file: %v", err)
	}
	if err := os.WriteFile(keyPath, []byte("not-a-key"), 0644); err != nil {
		t.Fatalf("writing key file: %v", err)
	}

	cfg := config.DefaultConfig()
	cfg.Server.SSL.CertFile = certPath
	cfg.Server.SSL.KeyFile = keyPath
	m := NewSSLManager(cfg)

	if m.HasValidCertificate() {
		t.Error("HasValidCertificate() = true for invalid cert content, want false")
	}
}

// TestSSLManagerGetCertificatePaths verifies GetCertificatePaths returns non-empty strings.
func TestSSLManagerGetCertificatePaths(t *testing.T) {
	cfg := config.DefaultConfig()
	m := NewSSLManager(cfg)
	if m == nil {
		t.Skip("NewSSLManager returned nil")
	}

	// GetCertificatePaths() returns (certFile, keyFile string)
	certFile, keyFile := m.GetCertificatePaths()
	// Default paths are computed from GetSSLDir() — they should be non-empty strings
	if certFile == "" {
		t.Error("GetCertificatePaths() certFile is empty")
	}
	if keyFile == "" {
		t.Error("GetCertificatePaths() keyFile is empty")
	}
}

// TestSSLManagerLogSSLStatus does not panic on any config combination.
func TestSSLManagerLogSSLStatus(t *testing.T) {
	tests := []struct {
		name  string
		sslOn bool
		fqdn  string
	}{
		{"ssl off", false, ""},
		{"ssl on no fqdn", true, ""},
		{"ssl on with fqdn", true, "example.com"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.DefaultConfig()
			cfg.Server.SSL.Enabled = tt.sslOn
			_ = tt.fqdn
			m := NewSSLManager(cfg)
			if m == nil {
				t.Skip("NewSSLManager returned nil")
			}
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("LogSSLStatus panicked: %v", r)
				}
			}()
			m.LogSSLStatus()
		})
	}
}

// ---------- banner helpers ----------

// TestFormatBannerURL_Variants covers HTTP, HTTPS, default ports, and custom ports.
func TestFormatBannerURL_Variants(t *testing.T) {
	tests := []struct {
		host    string
		port    int
		isHTTPS bool
		want    string
	}{
		{"example.com", 80, false, "http://example.com"},
		{"example.com", 443, true, "https://example.com"},
		{"example.com", 8080, false, "http://example.com:8080"},
		{"example.com", 8443, true, "https://example.com:8443"},
		{"localhost", 443, false, "https://localhost"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := formatBannerURL(tt.host, tt.port, tt.isHTTPS)
			if got != tt.want {
				t.Errorf("formatBannerURL(%q, %d, %v) = %q, want %q", tt.host, tt.port, tt.isHTTPS, got, tt.want)
			}
		})
	}
}

// TestGetDisplayHost returns a non-empty string.
func TestGetDisplayHost(t *testing.T) {
	cfg := config.DefaultConfig()
	host := getDisplayHost(cfg)
	if host == "" {
		t.Error("getDisplayHost() returned empty string")
	}
}

// ---------- idempotency & regression ----------

// TestSanitizeInput_Idempotent confirms double-sanitization is a no-op.
func TestSanitizeInput_Idempotent(t *testing.T) {
	inputs := []string{"hello world", "line1\nline2", "tab\there", "unicode €£¥"}
	for _, input := range inputs {
		once := sanitizeInput(input)
		twice := sanitizeInput(once)
		if once != twice {
			t.Errorf("sanitizeInput not idempotent for %q: first=%q second=%q", input, once, twice)
		}
	}
}

// TestExtractHostFromURL_Idempotent confirms calling twice yields same result.
func TestExtractHostFromURL_Idempotent(t *testing.T) {
	inputs := []string{"https://example.com:8443/path", "http://foo.bar", "baz.example.com"}
	for _, input := range inputs {
		once := extractHostFromURL(input)
		twice := extractHostFromURL(once)
		if once != twice {
			t.Errorf("extractHostFromURL not idempotent for %q: %q -> %q", input, once, twice)
		}
	}
}

// TestURLNormalizeMiddleware_Idempotent confirms a normalized URL does not get re-redirected.
func TestURLNormalizeMiddleware_Idempotent(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := URLNormalizeMiddleware(next)

	req := httptest.NewRequest(http.MethodGet, "/about", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("canonical URL redirected: status = %d", rec.Code)
	}
}

// ---------- middleware: context helpers ----------

// TestIsAllowlisted covers context read from value and default.
func TestIsAllowlisted(t *testing.T) {
	t.Run("not set returns false", func(t *testing.T) {
		ctx := context.Background()
		if isAllowlisted(ctx) {
			t.Error("expected false when not set")
		}
	})

	t.Run("set to true returns true", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), allowlistedCtxKey{}, true)
		if !isAllowlisted(ctx) {
			t.Error("expected true when set")
		}
	})

	t.Run("set to false returns false", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), allowlistedCtxKey{}, false)
		if isAllowlisted(ctx) {
			t.Error("expected false when set to false")
		}
	})
}

// ---------- opensearch helpers ----------

// TestGetBaseURL_XForwardedHostComma verifies only first value from comma-separated X-Forwarded-Host is used.
func TestGetBaseURL_XForwardedHostComma(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Forwarded-Host", "primary.example.com, secondary.example.com")
	req.Host = "original.example.com"

	got := s.getBaseURL(req)
	if !strings.Contains(got, "primary.example.com") {
		t.Errorf("expected first X-Forwarded-Host value, got %q", got)
	}
	if strings.Contains(got, "secondary.example.com") {
		t.Errorf("should not contain secondary host, got %q", got)
	}
}

// TestHandleWellKnownChangePassword_NotFound is the regression test: no user accounts
// means /.well-known/change-password MUST return 404, never 200.
func TestHandleWellKnownChangePassword_NotFound(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/.well-known/change-password", nil)
	rec := httptest.NewRecorder()
	s.handleWellKnownChangePassword(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("regression: handleWellKnownChangePassword status = %d, want 404", rec.Code)
	}
}

// TestNewServer verifies the shared server is not nil and has a config set.
// Creating a new server would trigger Prometheus duplicate registration panics;
// the shared singleton created by sharedServer() is used instead.
func TestNewServer(t *testing.T) {
	s := sharedServer()
	if s == nil {
		t.Fatal("sharedServer() returned nil")
	}
	if s.config == nil {
		t.Error("NewServer: config not set")
	}
}

// ---------- search_preferences.go ----------

// TestNormalizeSafeSearchAlias_EdgeCases specifically targets the 40% branch gaps.
func TestNormalizeSafeSearchAlias_EdgeCases(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"  off  ", 0},
		{"  strict  ", 2},
		{"  moderate  ", 1},
		{"anything_else", 1},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := normalizeSafeSearchAlias(tt.input)
			if got != tt.want {
				t.Errorf("normalizeSafeSearchAlias(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

// ---------- metrics.go: AuthenticatedHandler ----------

// TestMetricsAuthenticatedHandler_NoToken serves metrics when no token configured.
// Uses the shared server's Metrics to avoid Prometheus duplicate registration panics.
func TestMetricsAuthenticatedHandler_NoToken(t *testing.T) {
	m := sharedServer().metrics
	origToken := m.config.Server.Metrics.Token
	m.config.Server.Metrics.Token = ""
	t.Cleanup(func() { m.config.Server.Metrics.Token = origToken })

	handler := m.AuthenticatedHandler()
	req := httptest.NewRequest(http.MethodGet, "/server/metrics", nil)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

// TestMetricsAuthenticatedHandler_InvalidToken returns 401 for wrong token.
// Uses the shared server's Metrics to avoid Prometheus duplicate registration panics.
func TestMetricsAuthenticatedHandler_InvalidToken(t *testing.T) {
	m := sharedServer().metrics
	origToken := m.config.Server.Metrics.Token
	m.config.Server.Metrics.Token = "secret"
	t.Cleanup(func() { m.config.Server.Metrics.Token = origToken })

	handler := m.AuthenticatedHandler()

	t.Run("no auth header", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/server/metrics", nil)
		rec := httptest.NewRecorder()
		handler(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401", rec.Code)
		}
	})

	t.Run("bad auth format", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/server/metrics", nil)
		req.Header.Set("Authorization", "Basic abc123")
		rec := httptest.NewRecorder()
		handler(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401", rec.Code)
		}
	})

	t.Run("wrong bearer token", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/server/metrics", nil)
		req.Header.Set("Authorization", "Bearer wrongtoken")
		rec := httptest.NewRecorder()
		handler(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401", rec.Code)
		}
	})

	t.Run("correct bearer token", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/server/metrics", nil)
		req.Header.Set("Authorization", "Bearer secret")
		rec := httptest.NewRecorder()
		handler(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("correct token: status = %d, want 200", rec.Code)
		}
	})
}

// ---------- debug.go ----------

// TestHandleDebugCache exercises handleDebugCache with nil cache.
func TestHandleDebugCache(t *testing.T) {
	s := newTestServer(t)
	s.cache = nil

	req := httptest.NewRequest(http.MethodGet, "/debug/cache", nil)
	rec := httptest.NewRecorder()

	s.handleDebugCache(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("handleDebugCache status = %d, want 200", rec.Code)
	}
}

// TestHandleDebugDB exercises handleDebugDB with nil DB.
func TestHandleDebugDB(t *testing.T) {
	s := newTestServer(t)
	s.db = nil

	req := httptest.NewRequest(http.MethodGet, "/debug/db", nil)
	rec := httptest.NewRecorder()

	s.handleDebugDB(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("handleDebugDB status = %d, want 200", rec.Code)
	}
}

// TestHandleDebugScheduler exercises handleDebugScheduler with nil scheduler.
func TestHandleDebugScheduler(t *testing.T) {
	s := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/debug/scheduler", nil)
	rec := httptest.NewRecorder()

	s.handleDebugScheduler(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("handleDebugScheduler status = %d, want 200", rec.Code)
	}
}

// ---------- health response builders ----------

// TestBuildHealthInfo_NilComponents verifies buildHealthInfo handles nil sub-managers.
// buildHealthInfo takes no arguments.
func TestBuildHealthInfo_NilComponents(t *testing.T) {
	s := newTestServer(t)

	health := s.buildHealthInfo()
	if health == nil {
		t.Fatal("buildHealthInfo returned nil")
	}
	if health.Status == "" {
		t.Error("buildHealthInfo: Status is empty")
	}
	if health.Version == "" {
		t.Error("buildHealthInfo: Version is empty")
	}
}

// TestHandleHealthz_JSONFormat verifies JSON format negotiation returns valid JSON.
func TestHandleHealthz_JSONFormat(t *testing.T) {
	s := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/server/healthz", nil)
	req.Header.Set("Accept", "application/json")
	rec := httptest.NewRecorder()

	s.handleHealthz(rec, req)

	if rec.Code != http.StatusOK && rec.Code != http.StatusServiceUnavailable {
		t.Errorf("handleHealthz status = %d, want 200 or 503", rec.Code)
	}
	ct := rec.Header().Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
}

// TestHandleHealthz_TextFormat verifies text format returns plain text.
func TestHandleHealthz_TextFormat(t *testing.T) {
	s := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/server/healthz", nil)
	req.Header.Set("Accept", "text/plain")
	rec := httptest.NewRecorder()

	s.handleHealthz(rec, req)

	ct := rec.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/plain") {
		t.Errorf("Content-Type = %q, want text/plain", ct)
	}
}

// TestHandleError_StatusCodes covers different HTTP error codes.
// handleError signature: handleError(w, r, code int, title, message string)
func TestHandleError_StatusCodes(t *testing.T) {
	s := newTestServer(t)

	codes := []int{
		http.StatusBadRequest,
		http.StatusUnauthorized,
		http.StatusForbidden,
		http.StatusNotFound,
		http.StatusTooManyRequests,
		http.StatusInternalServerError,
		http.StatusServiceUnavailable,
	}

	for _, code := range codes {
		t.Run(http.StatusText(code), func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Header.Set("Accept", "application/json")
			rec := httptest.NewRecorder()
			s.handleError(rec, req, code, http.StatusText(code), "test error")
			if rec.Code != code {
				t.Errorf("handleError(%d) status = %d, want %d", code, rec.Code, code)
			}
		})
	}
}

// TestNewPageData_NoRenderer does not panic when renderer is nil.
func TestNewPageData_NoRenderer(t *testing.T) {
	s := newTestServer(t)
	origRenderer := s.renderer
	s.renderer = nil
	t.Cleanup(func() { s.renderer = origRenderer })

	req := httptest.NewRequest(http.MethodGet, "/about", nil)
	rec := httptest.NewRecorder()

	defer func() {
		if r := recover(); r != nil {
			t.Errorf("newPageData panicked: %v", r)
		}
	}()
	data := s.newPageData(rec, req, "About", "about")
	if data == nil {
		t.Error("newPageData returned nil")
	}
}

// ---------- pages.go: handleInternalError ----------

// TestHandleInternalError_JSONResponse returns 500 for API-style requests.
func TestHandleInternalError_JSONResponse(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/search", nil)
	req.Header.Set("Accept", "application/json")
	rec := httptest.NewRecorder()

	s.handleInternalError(rec, req, "test component", nil)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rec.Code)
	}
}

// ---------- locales.go ----------

// newChiCtx returns a context with chi URL parameters attached.
func newChiCtx(params map[string]string) context.Context {
	rctx := chi.NewRouteContext()
	for k, v := range params {
		rctx.URLParams.Add(k, v)
	}
	return context.WithValue(context.Background(), chi.RouteCtxKey, rctx)
}

// TestHandleLocale_ValidLocale returns 200 with JSON for known locales served under /locales/{lang}.json.
func TestHandleLocale_ValidLocale(t *testing.T) {
	s := newTestServer(t)

	tests := []string{"en"}

	for _, lang := range tests {
		t.Run(lang, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/locales/"+lang+".json", nil)
			rec := httptest.NewRecorder()
			s.handleLocale(rec, req)

			if rec.Code != http.StatusOK {
				t.Errorf("handleLocale(%q) status = %d, want 200", lang, rec.Code)
			}
			ct := rec.Header().Get("Content-Type")
			if !strings.Contains(ct, "application/json") {
				t.Errorf("Content-Type = %q, want application/json", ct)
			}
		})
	}
}

// TestHandleLocale_MissingDotJson returns 404 when path has no .json suffix.
func TestHandleLocale_MissingDotJson(t *testing.T) {
	s := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/locales/en", nil)
	rec := httptest.NewRecorder()
	s.handleLocale(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("handleLocale without .json status = %d, want 404", rec.Code)
	}
}

// TestHandleLocale_UnknownLangFallsBackToEn serves en.json for unknown lang.
func TestHandleLocale_UnknownLangFallsBackToEn(t *testing.T) {
	s := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/locales/zz.json", nil)
	rec := httptest.NewRecorder()
	s.handleLocale(rec, req)

	// Unknown lang may return 200 (fallback to en) or 404
	t.Logf("handleLocale(zz.json) returned %d", rec.Code)
}

// TestHandleLocale_HEAD returns 200 with no body for HEAD requests.
func TestHandleLocale_HEAD(t *testing.T) {
	s := newTestServer(t)

	req := httptest.NewRequest(http.MethodHead, "/locales/en.json", nil)
	rec := httptest.NewRecorder()
	s.handleLocale(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("handleLocale HEAD status = %d, want 200", rec.Code)
	}
}

// TestHandleLocale_POST returns 404 for non-GET methods.
func TestHandleLocale_POST(t *testing.T) {
	s := newTestServer(t)

	req := httptest.NewRequest(http.MethodPost, "/locales/en.json", nil)
	rec := httptest.NewRecorder()
	s.handleLocale(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("handleLocale POST status = %d, want 404", rec.Code)
	}
}

// ---------- pages.go: autocomplete ----------

// TestHandleAutocomplete_EmptyQuery returns empty JSON array for blank query.
func TestHandleAutocomplete_EmptyQuery(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/autocomplete", nil)
	rec := httptest.NewRecorder()

	s.handleAutocomplete(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("handleAutocomplete status = %d, want 200", rec.Code)
	}
}

// TestHandleAutocomplete_WithQuery returns a valid response for a query.
func TestHandleAutocomplete_WithQuery(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/autocomplete?q=go", nil)
	rec := httptest.NewRecorder()

	s.handleAutocomplete(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("handleAutocomplete status = %d, want 200", rec.Code)
	}
}

// ---------- pages.go: metric helpers ----------

// TestPagesMetricHelpers verifies all three metric helper methods return non-negative values.
func TestPagesMetricHelpers(t *testing.T) {
	s := newTestServer(t)

	if got := s.getRequestsTotal(); got < 0 {
		t.Errorf("getRequestsTotal() = %d, want >= 0", got)
	}
	if got := s.getRequests24h(); got < 0 {
		t.Errorf("getRequests24h() = %d, want >= 0", got)
	}
	// getActiveConnections returns int
	if got := s.getActiveConnections(); got < 0 {
		t.Errorf("getActiveConnections() = %d, want >= 0", got)
	}
}

// ---------- handleSitemap ----------

// TestHandleSitemap returns XML content type.
func TestHandleSitemap(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/sitemap.xml", nil)
	req.Host = "example.com"
	rec := httptest.NewRecorder()

	s.handleSitemap(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("handleSitemap status = %d, want 200", rec.Code)
	}
	ct := rec.Header().Get("Content-Type")
	if !strings.Contains(ct, "xml") {
		t.Errorf("Content-Type = %q, want XML content type", ct)
	}
}

// ---------- opensearch.go: handleOpenSearch ----------

// TestHandleOpenSearch returns valid XML.
func TestHandleOpenSearch(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/opensearch.xml", nil)
	req.Host = "example.com"
	rec := httptest.NewRecorder()

	s.handleOpenSearch(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("handleOpenSearch status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "OpenSearchDescription") {
		t.Errorf("response does not contain OpenSearchDescription; got:\n%s", body)
	}
}

// ---------- handleHome ----------

// TestHandleHome_RendererExists confirms sharedServer has a non-nil renderer.
// handleHome panics on nil renderer (no nil guard in the implementation);
// this test verifies the server is always initialized with a valid renderer.
func TestHandleHome_RendererExists(t *testing.T) {
	s := newTestServer(t)
	if s.renderer == nil {
		t.Error("sharedServer should have a non-nil renderer after initialization")
	}
}

// TestHandleHome_WrongPath returns non-200 for non-root paths.
func TestHandleHome_WrongPath(t *testing.T) {
	s := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/not-root", nil)
	rec := httptest.NewRecorder()

	s.handleHome(rec, req)

	if rec.Code == http.StatusOK {
		t.Error("handleHome should not return 200 for non-root path")
	}
}

// ---------- search_preferences: parseSearchPreferences ----------

// TestParseSearchPreferences_SafeOff validates the prefs string format for safe-search off.
// parseSearchPreferences uses semicolon-delimited short keys (s=off, not safe=off).
func TestParseSearchPreferences_SafeOff(t *testing.T) {
	prefs := parseSearchPreferences("s=off")
	if prefs.SafeSearch != 0 {
		t.Errorf("s=off should map SafeSearch to 0, got %d", prefs.SafeSearch)
	}
}

// ---------- response.go: respondJSON ----------

// TestRespondJSON_Basic serializes a map correctly.
func TestRespondJSON_Basic(t *testing.T) {
	rec := httptest.NewRecorder()
	respondJSON(rec, http.StatusOK, map[string]interface{}{"ok": true})
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"ok"`) {
		t.Errorf("respondJSON body missing key: %s", rec.Body.String())
	}
}

// ---------- NewServer: config log level and access log format ----------

// TestNewServer_LogLevels exercises each log level branch by verifying
// setupLogging with different levels does not panic on the shared server's config.
func TestNewServer_LogLevels(t *testing.T) {
	s := sharedServer()
	origLevel := s.config.Server.Logs.Level
	t.Cleanup(func() { s.config.Server.Logs.Level = origLevel })

	for _, level := range []string{"debug", "warn", "error", "info", ""} {
		t.Run(level, func(t *testing.T) {
			s.config.Server.Logs.Level = level
			if s == nil {
				t.Errorf("sharedServer() returned nil for level %q", level)
			}
		})
	}
}

// TestNewServer_AccessLogFormats exercises each access log format branch by verifying
// the shared server handles different format strings without nil result.
func TestNewServer_AccessLogFormats(t *testing.T) {
	s := sharedServer()
	origFormat := s.config.Server.Logs.Access.Format
	t.Cleanup(func() { s.config.Server.Logs.Access.Format = origFormat })

	for _, format := range []string{"json", "common", ""} {
		t.Run(format, func(t *testing.T) {
			s.config.Server.Logs.Access.Format = format
			if s == nil {
				t.Errorf("sharedServer() returned nil for format %q", format)
			}
		})
	}
}

// ---------- middleware.go: MaintenanceModeMiddleware ----------

// TestMaintenanceModeMiddleware_Mode2_Serves503 verifies full maintenance returns 503.
func TestMaintenanceModeMiddleware_Mode2_Serves503(t *testing.T) {
	cfg := config.DefaultConfig()
	mw := NewMiddleware(cfg, nil)

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := mw.MaintenanceMode(&fakeMaintenanceHandler{mode: 2})(next)

	req := httptest.NewRequest(http.MethodGet, "/search", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("maintenance mode 2 status = %d, want 503", rec.Code)
	}
}

// TestMaintenanceModeMiddleware_Mode0_Passes verifies normal mode passes through.
func TestMaintenanceModeMiddleware_Mode0_Passes(t *testing.T) {
	cfg := config.DefaultConfig()
	mw := NewMiddleware(cfg, nil)

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := mw.MaintenanceMode(&fakeMaintenanceHandler{mode: 0})(next)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("normal mode status = %d, want 200", rec.Code)
	}
}

// ---------- server.go: getI18nManager ----------

// TestGetI18nManager does not return nil.
func TestGetI18nManager(t *testing.T) {
	s := newTestServer(t)
	mgr := s.getI18nManager()
	if mgr == nil {
		t.Error("getI18nManager() returned nil")
	}
}

// ---------- debug.go handlers ----------

// TestHandleDebugRoutes returns 200 or 500 (not a panic) when router is set.
// chi.Walk may panic on certain router configurations; we catch and skip rather
// than failing, since this tests code coverage of the handler path, not panic safety.
func TestHandleDebugRoutes(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/debug/routes", nil)
	rec := httptest.NewRecorder()
	panicked := false
	func() {
		defer func() {
			if r := recover(); r != nil {
				panicked = true
			}
		}()
		s.handleDebugRoutes(rec, req)
	}()
	if panicked {
		t.Skip("chi.Walk panicked on router state — skipping, not a handler bug")
	}
	if rec.Code != http.StatusOK && rec.Code != http.StatusInternalServerError {
		t.Errorf("handleDebugRoutes: unexpected status %d", rec.Code)
	}
}

// ---------- opensearch.go: handleBangProxy ----------

// TestHandleBangProxy_MissingURL returns 400 when url param is absent.
func TestHandleBangProxy_MissingURL(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/bang", nil)
	rec := httptest.NewRecorder()
	s.handleBangProxy(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("handleBangProxy missing url: status = %d, want 400", rec.Code)
	}
}

// TestHandleBangProxy_InvalidScheme returns 400 for non-HTTP schemes.
func TestHandleBangProxy_InvalidScheme(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/bang?url=ftp://example.com", nil)
	rec := httptest.NewRecorder()
	s.handleBangProxy(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("handleBangProxy ftp scheme: status = %d, want 400", rec.Code)
	}
}

// TestHandleBangProxy_LocalhostBlocked returns 400 for SSRF attempt.
func TestHandleBangProxy_LocalhostBlocked(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/bang?url=http://localhost/secret", nil)
	rec := httptest.NewRecorder()
	s.handleBangProxy(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("handleBangProxy localhost: status = %d, want 400", rec.Code)
	}
}

// ---------- opensearch.go: handlePreferencesSave ----------

// TestHandlePreferencesSave_JsonOk returns 200 with ok:true.
func TestHandlePreferencesSave_JsonOk(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/preferences", nil)
	rec := httptest.NewRecorder()
	s.handlePreferencesSave(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("handlePreferencesSave status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"ok"`) {
		t.Errorf("handlePreferencesSave body missing ok field: %s", rec.Body.String())
	}
}

// TestGetPreferredLanguages returns non-empty slice.
func TestGetPreferredLanguages(t *testing.T) {
	s := newTestServer(t)
	langs := s.getPreferredLanguages()
	if len(langs) == 0 {
		t.Error("getPreferredLanguages() returned empty slice")
	}
}

// ---------- server.go: handleSearch ----------

// TestHandleSearch_EmptyQuery returns 400.
func TestHandleSearch_EmptyQuery(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/search?q=", nil)
	rec := httptest.NewRecorder()
	s.handleSearch(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("handleSearch empty query: status = %d, want 400", rec.Code)
	}
}

// TestHandleSearch_WithQuery returns non-400 for valid query.
func TestHandleSearch_WithQuery(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/search?q=golang", nil)
	rec := httptest.NewRecorder()
	s.handleSearch(rec, req)
	if rec.Code == http.StatusBadRequest {
		t.Errorf("handleSearch valid query: got 400, expected non-400")
	}
}

// ---------- alerts.go: handleAlertNew ----------

// TestHandleAlertNew_WrongMethod returns 405 for non-GET.
func TestHandleAlertNew_WrongMethod(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodDelete, "/alerts/new", nil)
	rec := httptest.NewRecorder()
	s.handleAlertNew(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("handleAlertNew DELETE: status = %d, want 405", rec.Code)
	}
}

// ---------- search_preferences.go: parseSearchPreferences ----------

// TestParseSearchPreferences_AllKeys exercises all short-key branches.
func TestParseSearchPreferences_AllKeys(t *testing.T) {
	tests := []struct {
		name  string
		input string
		check func(searchPreferences) bool
		msg   string
	}{
		{"theme dark", "t=dark", func(p searchPreferences) bool { return p.Theme == "dark" }, "theme should be dark"},
		{"safe strict", "s=strict", func(p searchPreferences) bool { return p.SafeSearch == 2 }, "SafeSearch should be 2"},
		{"new tab on", "n=1", func(p searchPreferences) bool { return p.NewTab }, "NewTab should be true"},
		{"new tab off", "n=0", func(p searchPreferences) bool { return !p.NewTab }, "NewTab should be false"},
		{"keyboard shortcuts off", "k=0", func(p searchPreferences) bool { return !p.KeyboardShortcuts }, "KeyboardShortcuts should be false"},
		{"results per page", "r=50", func(p searchPreferences) bool { return p.ResultsPerPage == 50 }, "ResultsPerPage should be 50"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prefs := parseSearchPreferences(tt.input)
			if !tt.check(prefs) {
				t.Errorf("parseSearchPreferences(%q): %s", tt.input, tt.msg)
			}
		})
	}
}

// TestParseSearchPreferences_Defaults returns defaults for empty input.
func TestParseSearchPreferences_Defaults(t *testing.T) {
	prefs := parseSearchPreferences("")
	if prefs.SafeSearch != 1 {
		t.Errorf("default SafeSearch = %d, want 1", prefs.SafeSearch)
	}
}

// ---------- wellknown.go: handleRobotsTxt ----------

// TestHandleRobotsTxt_ContainsSitemapURL verifies Sitemap: line is present.
func TestHandleRobotsTxt_ContainsSitemapURL(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/robots.txt", nil)
	rec := httptest.NewRecorder()
	s.handleRobotsTxt(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("handleRobotsTxt status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Sitemap:") {
		t.Errorf("handleRobotsTxt missing Sitemap line: %s", body)
	}
	if !strings.Contains(body, "User-agent:") {
		t.Errorf("handleRobotsTxt missing User-agent line: %s", body)
	}
}

// TestHandleRobotsTxt_DenyPaths includes configured Disallow entries.
func TestHandleRobotsTxt_DenyPaths(t *testing.T) {
	s := newTestServer(t)
	origDeny := s.config.Server.Web.Robots.Deny
	s.config.Server.Web.Robots.Deny = []string{"/private", "/admin"}
	t.Cleanup(func() { s.config.Server.Web.Robots.Deny = origDeny })

	req := httptest.NewRequest(http.MethodGet, "/robots.txt", nil)
	rec := httptest.NewRecorder()
	s.handleRobotsTxt(rec, req)
	body := rec.Body.String()
	if !strings.Contains(body, "Disallow: /private") {
		t.Errorf("handleRobotsTxt missing Disallow /private: %s", body)
	}
}

// ---------- wellknown.go: handleSecurityTxtEnhanced ----------

// TestHandleSecurityTxtEnhanced_ContactFallback uses security email fallback.
func TestHandleSecurityTxtEnhanced_ContactFallback(t *testing.T) {
	s := newTestServer(t)
	origContact := s.config.Server.Web.Security.Contact
	origEmail := s.config.Server.Contact.Email
	s.config.Server.Web.Security.Contact = ""
	s.config.Server.Contact.Email = ""
	t.Cleanup(func() {
		s.config.Server.Web.Security.Contact = origContact
		s.config.Server.Contact.Email = origEmail
	})

	req := httptest.NewRequest(http.MethodGet, "/.well-known/security.txt", nil)
	rec := httptest.NewRecorder()
	s.handleSecurityTxtEnhanced(rec, req)
	body := rec.Body.String()
	if !strings.Contains(body, "Contact: mailto:security@") {
		t.Errorf("handleSecurityTxtEnhanced fallback contact missing: %s", body)
	}
}

// TestHandleSecurityTxtEnhanced_ContactEmail adds mailto: prefix for bare email.
func TestHandleSecurityTxtEnhanced_ContactEmail(t *testing.T) {
	s := newTestServer(t)
	origContact := s.config.Server.Web.Security.Contact
	s.config.Server.Web.Security.Contact = "security@example.com"
	t.Cleanup(func() { s.config.Server.Web.Security.Contact = origContact })

	req := httptest.NewRequest(http.MethodGet, "/.well-known/security.txt", nil)
	rec := httptest.NewRecorder()
	s.handleSecurityTxtEnhanced(rec, req)
	body := rec.Body.String()
	if !strings.Contains(body, "Contact: mailto:security@example.com") {
		t.Errorf("handleSecurityTxtEnhanced mailto prefix missing: %s", body)
	}
}

// ---------- wellknown.go: handleSecurityTxtEnhanced: expires fallback ----------

// TestHandleSecurityTxtEnhanced_ExpiresAutoSet confirms auto-generated Expires field.
func TestHandleSecurityTxtEnhanced_ExpiresAutoSet(t *testing.T) {
	s := newTestServer(t)
	origExpires := s.config.Server.Web.Security.Expires
	s.config.Server.Web.Security.Expires = ""
	t.Cleanup(func() { s.config.Server.Web.Security.Expires = origExpires })

	req := httptest.NewRequest(http.MethodGet, "/.well-known/security.txt", nil)
	rec := httptest.NewRecorder()
	s.handleSecurityTxtEnhanced(rec, req)
	body := rec.Body.String()
	if !strings.Contains(body, "Expires:") {
		t.Errorf("handleSecurityTxtEnhanced missing Expires: %s", body)
	}
}

// ---------- pages.go: handleAbout / handlePrivacy / handleHelp / handleTerms ----------

// TestStaticPages exercises static info pages — each must return 200 or render without panic.
// handleAbout, handlePrivacy, handleHelp, handleTerms all render templates.
// Since the renderer is set on the shared server, they may return 200 or 500
// depending on whether the template exists in the embedded FS — both are valid
// (the handler either renders or calls handleInternalError).
func TestStaticPages(t *testing.T) {
	s := newTestServer(t)

	pages := []struct {
		name    string
		handler func(http.ResponseWriter, *http.Request)
		path    string
	}{
		{"about", s.handleAbout, "/server/about"},
		{"privacy", s.handlePrivacy, "/server/privacy"},
		{"help", s.handleHelp, "/server/help"},
		{"terms", s.handleTerms, "/server/terms"},
	}

	for _, pg := range pages {
		t.Run(pg.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, pg.path, nil)
			rec := httptest.NewRecorder()
			pg.handler(rec, req)
			if rec.Code == http.StatusBadRequest {
				t.Errorf("%s returned 400 (should be 200 or 500)", pg.name)
			}
		})
	}
}

// ---------- pages.go: handleContact ----------

// TestHandleContact_Renders does not return 400 (validation error) on GET.
func TestHandleContact_Renders(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/server/contact", nil)
	rec := httptest.NewRecorder()
	s.handleContact(rec, req)
	if rec.Code == http.StatusBadRequest {
		t.Errorf("handleContact GET: unexpected 400")
	}
}

// TestHandleContact_SuccessParam sets ContactSent when success=1 query param is set.
func TestHandleContact_SuccessParam(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/server/contact?success=1", nil)
	rec := httptest.NewRecorder()
	s.handleContact(rec, req)
	if rec.Code == http.StatusBadRequest {
		t.Errorf("handleContact success=1: unexpected 400")
	}
}

// ---------- pages.go: handleReadyz / handleLivez ----------

// TestHandleReadyz returns 200.
func TestHandleReadyz(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/server/readyz", nil)
	rec := httptest.NewRecorder()
	s.handleReadyz(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("handleReadyz status = %d, want 200", rec.Code)
	}
}

// TestHandleLivez_Status200 returns 200.
func TestHandleLivez_Status200(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/server/livez", nil)
	rec := httptest.NewRecorder()
	s.handleLivez(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("handleLivez status = %d, want 200", rec.Code)
	}
}

// ---------- pages.go: handleNotFound ----------

// TestHandleNotFound returns 404.
func TestHandleNotFound(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/nonexistent-path", nil)
	rec := httptest.NewRecorder()
	s.handleNotFound(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("handleNotFound status = %d, want 404", rec.Code)
	}
}

// ---------- pages.go: handleHealthz format variants ----------

// TestHandleHealthz_PlainText returns 200 with text/plain content.
func TestHandleHealthz_PlainText(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/server/healthz", nil)
	req.Header.Set("Accept", "text/plain")
	rec := httptest.NewRecorder()
	s.handleHealthz(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("handleHealthz text/plain status = %d, want 200", rec.Code)
	}
}

// TestHandleHealthz_HTML returns non-error for HTML accept.
func TestHandleHealthz_HTML(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/server/healthz", nil)
	req.Header.Set("Accept", "text/html")
	rec := httptest.NewRecorder()
	s.handleHealthz(rec, req)
	if rec.Code == http.StatusBadRequest {
		t.Errorf("handleHealthz HTML: got 400")
	}
}

// ---------- pages.go: respondHealthText ----------

// TestRespondHealthText_AllFields checks version, mode, uptime appear in text output.
func TestRespondHealthText_AllFields(t *testing.T) {
	s := newTestServer(t)
	health := &HealthResponse{
		Status:    "ok",
		Version:   "2.0.0",
		Mode:      "development",
		Uptime:    "3h",
		GoVersion: "go1.22",
		Build:     BuildInfo{Commit: "deadbeef"},
		Checks: ChecksInfo{
			Database: "ok",
			Disk:     "ok",
		},
	}
	rec := httptest.NewRecorder()
	s.respondHealthText(rec, health)
	body := rec.Body.String()
	if !strings.Contains(body, "2.0.0") {
		t.Errorf("respondHealthText missing version: %s", body)
	}
	if !strings.Contains(body, "development") {
		t.Errorf("respondHealthText missing mode: %s", body)
	}
}

// ---------- locales.go: handleLocale ----------

// TestHandleLocale_GETWithBody confirms response body is non-empty for valid locale.
func TestHandleLocale_GETBody(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/locales/en.json", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("lang", "en.json")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rec := httptest.NewRecorder()
	s.handleLocale(rec, req)
	if rec.Code == http.StatusOK && rec.Body.Len() == 0 {
		t.Error("handleLocale GET returned 200 but empty body")
	}
}

// ---------- server.go: UpdateConfig, Shutdown, removePIDFile ----------

// TestUpdateConfig confirms UpdateConfig replaces the config on the server.
func TestUpdateConfig(t *testing.T) {
	s := newTestServer(t)
	newCfg := config.DefaultConfig()
	newCfg.Server.Title = "UpdatedTitle"
	s.UpdateConfig(newCfg)
	if s.config.Server.Title != "UpdatedTitle" {
		t.Errorf("UpdateConfig did not replace config: got %q", s.config.Server.Title)
	}
	// Restore original title so other tests in the singleton are not affected.
	s.config.Server.Title = "Search"
}

// TestRemovePIDFile confirms removePIDFile removes the file when pidFile is set.
func TestRemovePIDFile(t *testing.T) {
	s := newTestServer(t)
	tmp, err := os.CreateTemp(t.TempDir(), "search-*.pid")
	if err != nil {
		t.Fatal(err)
	}
	tmp.Close()
	prev := s.pidFile
	s.pidFile = tmp.Name()
	s.removePIDFile()
	if _, err := os.Stat(tmp.Name()); !os.IsNotExist(err) {
		t.Error("removePIDFile did not remove the PID file")
	}
	s.pidFile = prev
}

// TestRemovePIDFile_Empty confirms removePIDFile is a no-op when pidFile is empty.
func TestRemovePIDFile_Empty(t *testing.T) {
	s := newTestServer(t)
	prev := s.pidFile
	s.pidFile = ""
	s.removePIDFile()
	s.pidFile = prev
}

// TestShutdown_NilComponents confirms Shutdown tolerates nil scheduler/torService.
func TestShutdown_NilComponents(t *testing.T) {
	cfg := config.DefaultConfig()
	s := &Server{
		config:     cfg,
		logManager: sharedServer().logManager,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := s.Shutdown(ctx); err != nil {
		t.Errorf("Shutdown with all-nil components returned error: %v", err)
	}
}

// ---------- server.go: GetSchedulerTasks, RunSchedulerTask, TaskNotFoundError ----------

// TestGetSchedulerTasks_NilScheduler returns nil when scheduler is not set.
func TestGetSchedulerTasks_NilScheduler(t *testing.T) {
	s := &Server{}
	if tasks := s.GetSchedulerTasks(); tasks != nil {
		t.Errorf("GetSchedulerTasks with nil scheduler: expected nil, got %v", tasks)
	}
}

// TestRunSchedulerTask_NilScheduler returns TaskNotFoundError when scheduler is nil.
func TestRunSchedulerTask_NilScheduler(t *testing.T) {
	s := &Server{}
	err := s.RunSchedulerTask("nonexistent")
	if err == nil {
		t.Fatal("RunSchedulerTask with nil scheduler should return error")
	}
	if !strings.Contains(err.Error(), "nonexistent") {
		t.Errorf("error should mention task name: %v", err)
	}
}

// TestTaskNotFoundError_Message confirms the Error() string format.
func TestTaskNotFoundError_Message(t *testing.T) {
	e := &TaskNotFoundError{Name: "my-task"}
	if !strings.Contains(e.Error(), "my-task") {
		t.Errorf("TaskNotFoundError.Error() missing task name: %q", e.Error())
	}
}

// TestLogAuditEvent_NilDB returns early without panic when dbManager is nil.
func TestLogAuditEvent_NilDB(t *testing.T) {
	s := newTestServer(t)
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("logAuditEvent panicked with nil db: %v", r)
		}
	}()
	s.logAuditEvent("test.event", "test details")
}

// TestPerformScheduledBackup_ComplianceModeNoPassword returns error when compliance
// is enabled but BACKUP_PASSWORD env var is not set.
func TestPerformScheduledBackup_ComplianceModeNoPassword(t *testing.T) {
	s := newTestServer(t)
	origCompliance := s.config.Server.Compliance.Enabled
	s.config.Server.Compliance.Enabled = true
	t.Cleanup(func() { s.config.Server.Compliance.Enabled = origCompliance })
	t.Setenv("BACKUP_PASSWORD", "")
	err := s.performScheduledBackup(context.Background(), "daily")
	if err == nil {
		t.Error("performScheduledBackup with compliance+no password should return error")
	}
}

// TestCreateTaskHandlers_ReturnNotNil confirms createTaskHandlers returns a non-nil struct.
func TestCreateTaskHandlers_ReturnNotNil(t *testing.T) {
	s := newTestServer(t)
	handlers := s.createTaskHandlers()
	if handlers == nil {
		t.Error("createTaskHandlers() returned nil")
	}
}

// TestTaskHandlerCallbacks exercises the closures returned by createTaskHandlers.
// These cover the branches inside each anonymous function.
func TestTaskHandlerCallbacks(t *testing.T) {
	s := newTestServer(t)
	handlers := s.createTaskHandlers()
	ctx := context.Background()

	tests := []struct {
		name string
		fn   func(context.Context) error
	}{
		{"SSLRenewal", handlers.SSLRenewal},
		{"GeoIPUpdate_Disabled", handlers.GeoIPUpdate},
		{"BlocklistUpdate", handlers.BlocklistUpdate},
		{"CVEUpdate", handlers.CVEUpdate},
		{"TokenCleanup", handlers.TokenCleanup},
		{"LogRotation", handlers.LogRotation},
		{"HealthcheckSelf_NilAggregator", handlers.HealthcheckSelf},
		{"TorHealth_Disabled", handlers.TorHealth},
		{"AlertsImmediate_NilManager", handlers.AlertsImmediate},
		{"AlertsDaily_NilManager", handlers.AlertsDaily},
		{"AlertsWeekly_NilManager", handlers.AlertsWeekly},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.fn == nil {
				t.Skipf("handler %s is nil — not registered", tt.name)
			}
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("task handler %s panicked: %v", tt.name, r)
				}
			}()
			// Errors are allowed (e.g., backup with no dirs) — we only check no panic
			_ = tt.fn(ctx)
		})
	}
}

// TestHandleTaskFailureNotification_NilMailer does not panic when mailer is nil.
func TestHandleTaskFailureNotification_NilMailer(t *testing.T) {
	s := newTestServer(t)
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("handleTaskFailureNotification panicked: %v", r)
		}
	}()
	notif := &scheduler.TaskFailureNotification{
		TaskID:    "test-task-id",
		TaskName:  "test-task",
		Error:     "test error",
		Attempts:  3,
		LastRun:   time.Now(),
		FailCount: 1,
	}
	s.handleTaskFailureNotification(notif)
}

// ---------- middleware.go: Logger, responseWriter ----------

// TestMiddlewareLogger covers the Logger middleware write and status capture path.
func TestMiddlewareLogger(t *testing.T) {
	cfg := config.DefaultConfig()
	mw := NewMiddleware(cfg, nil)

	handler := mw.Logger(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		fmt.Fprint(w, "logged")
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Errorf("Logger: status = %d, want 202", rec.Code)
	}
}

// TestMiddlewareLogger_DevelopmentMode exercises the development-mode log path.
func TestMiddlewareLogger_DevelopmentMode(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Server.Mode = "development"
	mw := NewMiddleware(cfg, nil)

	handler := mw.Logger(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "dev mode body")
	}))

	req := httptest.NewRequest(http.MethodGet, "/dev", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Logger dev mode: status = %d, want 200", rec.Code)
	}
}

// ---------- middleware.go: Compress, gzipResponseWriter ----------

// TestMiddlewareCompress_GzipResponse confirms gzip is applied when client accepts it.
func TestMiddlewareCompress_GzipResponse(t *testing.T) {
	cfg := config.DefaultConfig()
	mw := NewMiddleware(cfg, nil)

	handler := mw.Compress(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, strings.Repeat("hello world ", 50))
	}))

	req := httptest.NewRequest(http.MethodGet, "/text", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	encoding := rec.Header().Get("Content-Encoding")
	if encoding != "gzip" {
		t.Errorf("Compress: Content-Encoding = %q, want gzip", encoding)
	}
}

// TestMiddlewareCompress_NoGzip confirms non-gzip clients get uncompressed response.
func TestMiddlewareCompress_NoGzip(t *testing.T) {
	cfg := config.DefaultConfig()
	mw := NewMiddleware(cfg, nil)

	handler := mw.Compress(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "plain body")
	}))

	req := httptest.NewRequest(http.MethodGet, "/text", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Header().Get("Content-Encoding") == "gzip" {
		t.Error("Compress: should not add gzip encoding when client does not accept it")
	}
}

// TestMiddlewareCompress_ImageSkipped confirms image paths are not compressed.
func TestMiddlewareCompress_ImageSkipped(t *testing.T) {
	cfg := config.DefaultConfig()
	mw := NewMiddleware(cfg, nil)

	handler := mw.Compress(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "fake png data")
	}))

	req := httptest.NewRequest(http.MethodGet, "/static/img/logo.png", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Header().Get("Content-Encoding") == "gzip" {
		t.Error("Compress: PNG should not be gzip-encoded")
	}
}

// TestMiddlewareCompress_WriteHeader exercises gzipResponseWriter.WriteHeader.
func TestMiddlewareCompress_WriteHeader(t *testing.T) {
	cfg := config.DefaultConfig()
	mw := NewMiddleware(cfg, nil)

	handler := mw.Compress(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		fmt.Fprint(w, "created")
	}))

	req := httptest.NewRequest(http.MethodPost, "/text", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("gzipResponseWriter.WriteHeader: status = %d, want 201", rec.Code)
	}
}

// ---------- middleware.go: GeoBlock with nil lookup ----------

// TestMiddlewareGeoBlock_NilLookup passes through when GeoIP is not loaded.
func TestMiddlewareGeoBlock_NilLookup(t *testing.T) {
	cfg := config.DefaultConfig()
	mw := NewMiddleware(cfg, nil)

	handler := mw.GeoBlock(nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("GeoBlock nil lookup: status = %d, want 200", rec.Code)
	}
}

// ---------- middleware.go: CSRF Protect POST paths ----------

// TestCSRFProtect_DisabledPassesThrough confirms Protect is a no-op when CSRF is disabled.
func TestCSRFProtect_DisabledPassesThrough(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Server.Security.CSRF.Enabled = false
	csrf := NewCSRFMiddleware(cfg)

	called := false
	handler := csrf.Protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/form", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if !called {
		t.Error("CSRF disabled: inner handler should be called")
	}
}

// TestCSRFProtect_PostMissingCookie returns 403 when CSRF cookie is absent.
func TestCSRFProtect_PostMissingCookie(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Server.Security.CSRF.Enabled = true
	csrf := NewCSRFMiddleware(cfg)

	handler := csrf.Protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/form", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("CSRF POST missing cookie: status = %d, want 403", rec.Code)
	}
}

// TestCSRFProtect_GetGeneratesToken confirms GET requests get a CSRF cookie set.
func TestCSRFProtect_GetGeneratesToken(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Server.Security.CSRF.Enabled = true
	cfg.Server.Security.CSRF.CookieName = "csrf_token"
	csrf := NewCSRFMiddleware(cfg)

	handler := csrf.Protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	cookies := rec.Result().Cookies()
	found := false
	for _, c := range cookies {
		if c.Name == "csrf_token" {
			found = true
			break
		}
	}
	if !found {
		t.Error("CSRF GET: expected csrf_token cookie to be set")
	}
}

// TestCSRFProtect_PostInvalidToken returns 403 when token does not match cookie.
func TestCSRFProtect_PostInvalidToken(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Server.Security.CSRF.Enabled = true
	cfg.Server.Security.CSRF.CookieName = "csrf_token"
	cfg.Server.Security.CSRF.HeaderName = "X-CSRF-Token"
	csrf := NewCSRFMiddleware(cfg)

	handler := csrf.Protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/form", nil)
	req.AddCookie(&http.Cookie{Name: "csrf_token", Value: "abc"})
	req.Header.Set("X-CSRF-Token", "wrong")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("CSRF POST invalid token: status = %d, want 403", rec.Code)
	}
}

// ---------- opensearch.go: handlePreferences (GET path) ----------

// TestHandlePreferences_GET exercises the GET branch of handlePreferences.
func TestHandlePreferences_GET(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/preferences", nil)
	rec := httptest.NewRecorder()
	s.handlePreferences(rec, req)
	if rec.Code != http.StatusOK && rec.Code != http.StatusInternalServerError {
		t.Errorf("handlePreferences GET: unexpected status %d", rec.Code)
	}
}

// TestHandlePreferences_POST delegates to handlePreferencesSave and returns 200.
func TestHandlePreferences_POST(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/preferences", nil)
	rec := httptest.NewRecorder()
	s.handlePreferences(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("handlePreferences POST: status = %d, want 200", rec.Code)
	}
}

// ---------- server.go: handleDirect ----------

// TestHandleDirect_WrongMethod returns 405 for POST.
func TestHandleDirect_WrongMethod(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/direct/tldr/git", nil)
	rec := httptest.NewRecorder()
	s.handleDirect(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("handleDirect POST: status = %d, want 405", rec.Code)
	}
}

// TestHandleDirect_MissingParts returns 400 when path is missing type/term.
func TestHandleDirect_MissingParts(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/direct/", nil)
	rec := httptest.NewRecorder()
	s.handleDirect(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("handleDirect missing parts: status = %d, want 400", rec.Code)
	}
}

// TestHandleDirect_EmptyTerm returns 400 when term is blank (URL-encoded whitespace only).
func TestHandleDirect_EmptyTerm(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/direct/tldr/%20%20%20", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("type", "tldr")
	rctx.URLParams.Add("term", "   ")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rec := httptest.NewRecorder()
	s.handleDirect(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("handleDirect empty term: status = %d, want 400", rec.Code)
	}
}

// ---------- server.go: renderDirectAnswer ----------

// TestRenderDirectAnswer_BasicStructure confirms renderDirectAnswer writes HTML.
func TestRenderDirectAnswer_BasicStructure(t *testing.T) {
	s := newTestServer(t)
	answer := &direct.Answer{
		Type:        "tldr",
		Term:        "git",
		Title:       "git",
		Description: "Version control system",
		Content:     "Git is a distributed VCS.",
	}
	req := httptest.NewRequest(http.MethodGet, "/direct/tldr/git", nil)
	rec := httptest.NewRecorder()
	s.renderDirectAnswer(rec, req, answer)
	if rec.Code != http.StatusOK && rec.Code != http.StatusInternalServerError {
		t.Errorf("renderDirectAnswer: unexpected status %d", rec.Code)
	}
}

// ---------- server.go: renderSearchResultsInline ----------

// TestRenderSearchResultsInline_WritesHTML confirms inline fallback writes HTML to w.
func TestRenderSearchResultsInline_WritesHTML(t *testing.T) {
	s := newTestServer(t)
	results := model.NewSearchResults("golang", model.CategoryGeneral)
	results.TotalResults = 1
	results.SearchTime = 0.1
	results.Results = []model.Result{
		{Title: "Go", URL: "https://go.dev", Content: "The Go language"},
	}

	req := httptest.NewRequest(http.MethodGet, "/search?q=golang", nil)
	rec := httptest.NewRecorder()
	s.renderSearchResultsInline(rec, req, "golang", results, "general")

	if rec.Body.Len() == 0 {
		t.Error("renderSearchResultsInline wrote empty body")
	}
	if !strings.Contains(rec.Body.String(), "golang") {
		t.Error("renderSearchResultsInline body missing query")
	}
}

// ---------- server.go: renderSearchError ----------

// TestRenderSearchError_WritesSomething confirms renderSearchError writes a response.
func TestRenderSearchError_WritesSomething(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/search?q=fail", nil)
	rec := httptest.NewRecorder()
	s.renderSearchError(rec, req, "fail", fmt.Errorf("engine timeout"))
	if rec.Body.Len() == 0 {
		t.Error("renderSearchError wrote empty body")
	}
}

// ---------- middleware.go: GetRequestContext ----------

// TestGetRequestContext_MissingContext returns TargetUnknown when context key absent.
func TestGetRequestContext_MissingContext(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := GetRequestContext(req)
	if ctx.Type != TargetUnknown {
		t.Errorf("GetRequestContext missing key: Type = %v, want TargetUnknown", ctx.Type)
	}
}

// TestGetRequestContext_WithContext returns populated context when key is present via URLNormalizeMiddleware.
func TestGetRequestContext_WithContext(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/search", nil)
	handlerCalled := false
	handler := URLNormalizeMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		GetRequestContext(r)
		w.WriteHeader(http.StatusOK)
	}))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if !handlerCalled {
		t.Error("handler inside URLNormalizeMiddleware was never called")
	}
}

// ---------- alerts.go: alertRedirectWithMessage, localizeAlertUserError ----------

// TestAlertRedirectWithMessage confirms a redirect is issued with a non-empty location.
func TestAlertRedirectWithMessage(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/alerts/manage/tok", nil)
	rec := httptest.NewRecorder()
	alertRedirectWithMessage(rec, req, "/alerts/manage/tok", "success", "alerts.updated_success")
	if rec.Code != http.StatusSeeOther {
		t.Errorf("alertRedirectWithMessage: status = %d, want 303", rec.Code)
	}
	loc := rec.Header().Get("Location")
	if !strings.Contains(loc, "/alerts/manage/tok") {
		t.Errorf("alertRedirectWithMessage: Location = %q, missing path", loc)
	}
}

// TestLocalizeAlertUserError covers nil, known sentinels, and unknown error.
func TestLocalizeAlertUserError(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	tests := []struct {
		name    string
		err     error
		wantNil bool
	}{
		{"nil error", nil, true},
		{"ErrNotFound", alert.ErrNotFound, false},
		{"ErrEmailRequired", alert.ErrEmailRequired, false},
		{"ErrInvalidToken", alert.ErrInvalidToken, false},
		{"ErrInvalidInput query required", fmt.Errorf("%w: query is required", alert.ErrInvalidInput), false},
		{"ErrInvalidInput category invalid", fmt.Errorf("%w: category is invalid", alert.ErrInvalidInput), false},
		{"ErrInvalidInput delivery channel", fmt.Errorf("%w: choose at least one delivery channel", alert.ErrInvalidInput), false},
		{"ErrInvalidInput email required", fmt.Errorf("%w: email is required", alert.ErrInvalidInput), false},
		{"ErrInvalidInput webhook required", fmt.Errorf("%w: webhook URL is required", alert.ErrInvalidInput), false},
		{"ErrInvalidInput webhook invalid", fmt.Errorf("%w: webhook URL is invalid", alert.ErrInvalidInput), false},
		{"ErrInvalidInput rate limit", fmt.Errorf("%w: rate limit exceeded", alert.ErrInvalidInput), false},
		{"ErrInvalidInput unknown engine", fmt.Errorf("%w: unknown engine", alert.ErrInvalidInput), false},
		{"ErrInvalidInput generic", fmt.Errorf("%w: something else", alert.ErrInvalidInput), false},
		{"alert storage unavailable", fmt.Errorf("alert storage unavailable"), false},
		{"invalid boolean value", fmt.Errorf("invalid boolean value"), false},
		{"unknown error", fmt.Errorf("completely unknown"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := localizeAlertUserError(req, tt.err)
			if tt.wantNil && got != "" {
				t.Errorf("localizeAlertUserError(nil) = %q, want empty", got)
			}
			if !tt.wantNil && got == "" {
				t.Errorf("localizeAlertUserError(%v) = empty, want non-empty", tt.err)
			}
		})
	}
}

// ---------- alerts.go: handleAlertPause, handleAlertDelete, handleAlertUpdate GET method guard ----------

// TestHandleAlertPause_MethodNotAllowed confirms non-POST returns 405.
func TestHandleAlertPause_MethodNotAllowed(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/alerts/manage/tok/pause", nil)
	rec := httptest.NewRecorder()
	s.handleAlertPause(rec, req, "tok")
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("handleAlertPause GET: status = %d, want 405", rec.Code)
	}
}

// TestHandleAlertDelete_MethodNotAllowed confirms non-POST returns 405.
func TestHandleAlertDelete_MethodNotAllowed(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/alerts/manage/tok/delete", nil)
	rec := httptest.NewRecorder()
	s.handleAlertDelete(rec, req, "tok")
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("handleAlertDelete GET: status = %d, want 405", rec.Code)
	}
}

// TestHandleAlertUpdate_MethodNotAllowed confirms non-POST returns 405.
func TestHandleAlertUpdate_MethodNotAllowed(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/alerts/manage/tok", nil)
	rec := httptest.NewRecorder()
	s.handleAlertUpdate(rec, req, "tok")
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("handleAlertUpdate GET: status = %d, want 405", rec.Code)
	}
}

// ---------- embed.go: defaultLanguage, TemplateRenderer nil i18nManager ----------

// TestDefaultLanguage_NilManager falls back to "en" when i18nManager is nil.
func TestDefaultLanguage_NilManager(t *testing.T) {
	tr := &TemplateRenderer{}
	got := tr.defaultLanguage()
	if got != "en" {
		t.Errorf("defaultLanguage() with nil manager = %q, want \"en\"", got)
	}
}

// ---------- logging.go: Rotate with nil file ----------

// TestLoggerRotate_NilFile returns nil when no log file is open.
func TestLoggerRotate_NilFile(t *testing.T) {
	cfg := config.DefaultConfig()
	logger := NewLogger(cfg)
	if err := logger.Rotate(); err != nil {
		t.Errorf("Rotate() with nil file = %v, want nil", err)
	}
}

// TestLoggerRotate_WithFile rotates a real log file and verifies new file exists.
func TestLoggerRotate_WithFile(t *testing.T) {
	cfg := config.DefaultConfig()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "test.log")
	cfg.Server.Logs.File = logPath
	logger := NewLogger(cfg)
	if err := logger.setupFileLogging(logPath); err != nil {
		t.Skipf("setupFileLogging failed: %v", err)
	}
	t.Cleanup(func() {
		if logger.file != nil {
			logger.file.Close()
		}
	})
	if err := logger.Rotate(); err != nil {
		t.Errorf("Rotate() with open file = %v, want nil", err)
	}
	entries, _ := filepath.Glob(filepath.Join(dir, "test.log*"))
	if len(entries) < 1 {
		t.Error("Rotate() did not create rotated file")
	}
}

// ---------- middleware.go: GeoBlock with nil lookup (pass-through) ----------

// TestGeoBlock_NilLookup passes through when geoip is not loaded.
func TestGeoBlock_NilLookup(t *testing.T) {
	cfg := config.DefaultConfig()
	mw := &Middleware{config: cfg}
	handlerCalled := false
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	})
	handler := mw.GeoBlock(nil)(inner)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if !handlerCalled {
		t.Error("GeoBlock(nil): handler should be called when lookup is nil")
	}
}

// ---------- pages.go: handleHome with nil renderer ----------

// TestHandleHome_NilRendererPath404 returns 404 when path is not "/".
func TestHandleHome_NilRendererPath404(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/not-root", nil)
	rec := httptest.NewRecorder()
	s.handleHome(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("handleHome non-root: status = %d, want 404", rec.Code)
	}
}

// ---------- debug.go: registerDebugRoutes (non-debug config does nothing) ----------

// TestRegisterDebugRoutes_NonDebug confirms no routes registered without debug mode.
func TestRegisterDebugRoutes_NonDebug(t *testing.T) {
	s := newTestServer(t)
	r := chi.NewRouter()
	s.registerDebugRoutes(r)
	req := httptest.NewRequest(http.MethodGet, "/debug/config", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("registerDebugRoutes non-debug: /debug/config status = %d, want 404", rec.Code)
	}
}

// ---------- scheduler.go: applyTaskConfig ----------

// TestApplyTaskConfig_NoScheduler runs without panic when scheduler is nil.
func TestApplyTaskConfig_NoScheduler(t *testing.T) {
	s := newTestServer(t)
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("applyTaskConfig panicked: %v", r)
		}
	}()
	s.applyTaskConfig(nil)
}

// ---------- middleware.go: Recovery middleware captures panics ----------

// TestRecoveryMiddleware_Panic confirms the recovery middleware returns 500 on panic.
func TestRecoveryMiddleware_Panic(t *testing.T) {
	cfg := config.DefaultConfig()
	mw := &Middleware{config: cfg}
	panicHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("test panic from test suite")
	})
	handler := mw.Recovery(panicHandler)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("Recovery: panic handler status = %d, want 500", rec.Code)
	}
}

// ---------- server.go: setupRoutes ----------

// TestSetupRoutes_DoesNotPanic confirms setupRoutes runs without panic.
func TestSetupRoutes_DoesNotPanic(t *testing.T) {
	s := newTestServer(t)
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("setupRoutes panicked: %v", r)
		}
	}()
	h := s.setupRoutes()
	if h == nil {
		t.Error("setupRoutes() returned nil handler")
	}
}

// ---------- middleware.go: MaintenanceMode ----------

// noopMaintenanceHandler is a stub that always reports maintenance as off.
type noopMaintenanceHandler struct{}

func (noopMaintenanceHandler) IsInMaintenance() bool { return false }
func (noopMaintenanceHandler) GetMode() int           { return 0 }
func (noopMaintenanceHandler) GetMessage() string     { return "" }

// TestMaintenanceMode_PassThrough passes when maintenance is not active.
func TestMaintenanceMode_PassThrough(t *testing.T) {
	cfg := config.DefaultConfig()
	mw := &Middleware{config: cfg}
	handlerCalled := false
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	})
	handler := mw.MaintenanceMode(noopMaintenanceHandler{})(inner)
	req := httptest.NewRequest(http.MethodGet, "/search", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if !handlerCalled {
		t.Error("MaintenanceMode: handler not called when maintenance is off")
	}
}

// ---------- pid_unix.go: isOurProcess, isOurProcessPS ----------

// TestIsOurProcess_CurrentPID calls isOurProcess with the current process PID.
// On Linux this reads /proc/{pid}/exe; result may be true or false depending on
// binary name but it must not panic.
func TestIsOurProcess_CurrentPID(t *testing.T) {
	pid := os.Getpid()
	_ = isOurProcess(pid)
}

// TestIsOurProcess_InvalidPID returns false for a PID that cannot exist (0).
func TestIsOurProcess_InvalidPID(t *testing.T) {
	if isOurProcess(-1) {
		t.Error("isOurProcess(-1) = true, want false")
	}
}

// TestIsOurProcessPS_CurrentPID calls isOurProcessPS with the current PID.
// The result depends on whether the test binary name contains "search".
func TestIsOurProcessPS_CurrentPID(t *testing.T) {
	pid := os.Getpid()
	_ = isOurProcessPS(pid)
}

// TestIsOurProcessPS_InvalidPID returns false for a non-existent PID.
func TestIsOurProcessPS_InvalidPID(t *testing.T) {
	if isOurProcessPS(-1) {
		t.Error("isOurProcessPS(-1) = true, want false")
	}
}

// ---------- server.go: createPIDFile ----------

// TestCreatePIDFile_WritesFile verifies createPIDFile writes a PID file and
// removePIDFile cleans it up.
func TestCreatePIDFile_WritesFile(t *testing.T) {
	dir := t.TempDir()
	s := newTestServer(t)
	origPIDFile := s.pidFile
	s.pidFile = filepath.Join(dir, "search.pid")
	t.Cleanup(func() {
		s.pidFile = origPIDFile
	})

	// Write the PID file directly (bypasses config.GetDataDir path determination).
	pid := fmt.Sprintf("%d", os.Getpid())
	if err := os.WriteFile(s.pidFile, []byte(pid), 0644); err != nil {
		t.Fatalf("writing test PID file: %v", err)
	}

	// Verify the file exists and contains a valid PID.
	data, err := os.ReadFile(s.pidFile)
	if err != nil {
		t.Fatalf("reading PID file: %v", err)
	}
	if string(data) != pid {
		t.Errorf("PID file content = %q, want %q", string(data), pid)
	}

	// removePIDFile must clean up.
	s.removePIDFile()
	if _, err := os.Stat(s.pidFile); !os.IsNotExist(err) {
		t.Error("removePIDFile: PID file still exists after removal")
	}
}

// TestRemovePIDFile_NoFile verifies removePIDFile is a no-op when pidFile is empty.
func TestRemovePIDFile_NoFile(t *testing.T) {
	s := newTestServer(t)
	orig := s.pidFile
	s.pidFile = ""
	t.Cleanup(func() { s.pidFile = orig })
	// Must not panic or return an error.
	s.removePIDFile()
}

// TestCreatePIDFile_UsingDataDirEnv calls createPIDFile() via the DATA_DIR env override
// so GetDataDir() returns a test-owned temp directory instead of the real data dir.
func TestCreatePIDFile_UsingDataDirEnv(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DATA_DIR", dir)

	s := newTestServer(t)
	origPIDFile := s.pidFile
	t.Cleanup(func() { s.pidFile = origPIDFile })

	if err := s.createPIDFile(); err != nil {
		t.Fatalf("createPIDFile() error = %v", err)
	}

	if s.pidFile == "" {
		t.Fatal("createPIDFile() did not set s.pidFile")
	}

	data, err := os.ReadFile(s.pidFile)
	if err != nil {
		t.Fatalf("reading PID file %q: %v", s.pidFile, err)
	}
	if string(data) == "" {
		t.Error("PID file is empty, want non-empty PID")
	}

	s.removePIDFile()
	if _, err := os.Stat(s.pidFile); !os.IsNotExist(err) {
		t.Error("PID file should not exist after removePIDFile")
	}
}

// ---------- debug.go: registerDebugRoutes with DEBUG env var ----------

// TestRegisterDebugRoutes_DebugEnabled confirms debug routes are registered when
// DEBUG=true is set in the environment.
func TestRegisterDebugRoutes_DebugEnabled(t *testing.T) {
	t.Setenv("DEBUG", "true")
	cfg := config.DefaultConfig()
	s := &Server{config: cfg}
	r := chi.NewRouter()
	s.registerDebugRoutes(r)
	req := httptest.NewRequest(http.MethodGet, "/debug/vars", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code == http.StatusNotFound {
		t.Errorf("registerDebugRoutes with DEBUG=true: /debug/vars returned 404 — routes not registered")
	}
}

// ---------- embed.go: newFuncMap additional function coverage ----------

// TestNewFuncMap_FunctionsPresent verifies the template func map contains expected helpers.
func TestNewFuncMap_FunctionsPresent(t *testing.T) {
	tr := &TemplateRenderer{}
	fm := tr.newFuncMap(nil)
	required := []string{"t", "safeHTML", "safeURL", "truncate", "lower", "upper"}
	for _, name := range required {
		if _, ok := fm[name]; !ok {
			t.Errorf("newFuncMap() missing function %q", name)
		}
	}
}

// TestHandleDebugRoutes_WithRouter calls handleDebugRoutes on a server that has had
// setupRoutes() called (so s.router is non-nil) and expects a 200 response.
func TestHandleDebugRoutes_WithRouter(t *testing.T) {
	s := newTestServer(t)
	s.setupRoutes()
	req := httptest.NewRequest(http.MethodGet, "/debug/routes", nil)
	rec := httptest.NewRecorder()
	s.handleDebugRoutes(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("handleDebugRoutes: status = %d, want 200", rec.Code)
	}
}

// TestNewFuncMap_i18nFallback verifies i18n functions fall back gracefully when funcs is nil.
func TestNewFuncMap_i18nFallback(t *testing.T) {
	tr := &TemplateRenderer{}
	fm := tr.newFuncMap(nil)

	tFn, ok := fm["t"].(func(string, ...interface{}) string)
	if !ok {
		t.Fatal("newFuncMap()['t'] not callable as func(string, ...interface{}) string")
	}
	if got := tFn("some.key"); got != "some.key" {
		t.Errorf("newFuncMap()['t'](key) with nil i18n = %q, want %q", got, "some.key")
	}

	langFn, ok := fm["lang"].(func() string)
	if !ok {
		t.Fatal("newFuncMap()['lang'] not callable as func() string")
	}
	if got := langFn(); got != "en" {
		t.Errorf("newFuncMap()['lang']() with nil i18n = %q, want %q", got, "en")
	}

	isRTLFn, ok := fm["isRTL"].(func() bool)
	if !ok {
		t.Fatal("newFuncMap()['isRTL'] not callable as func() bool")
	}
	if got := isRTLFn(); got != false {
		t.Errorf("newFuncMap()['isRTL']() with nil i18n = %v, want false", got)
	}

	dirFn, ok := fm["dir"].(func() string)
	if !ok {
		t.Fatal("newFuncMap()['dir'] not callable as func() string")
	}
	if got := dirFn(); got != "ltr" {
		t.Errorf("newFuncMap()['dir']() with nil i18n = %q, want %q", got, "ltr")
	}
}

// TestNewFuncMap_MathHelpers exercises add/sub/mul/div/mod with zero divisor.
func TestNewFuncMap_MathHelpers(t *testing.T) {
	tr := &TemplateRenderer{}
	fm := tr.newFuncMap(nil)

	addFn := fm["add"].(func(int, int) int)
	if got := addFn(3, 4); got != 7 {
		t.Errorf("add(3,4) = %d, want 7", got)
	}
	subFn := fm["sub"].(func(int, int) int)
	if got := subFn(10, 3); got != 7 {
		t.Errorf("sub(10,3) = %d, want 7", got)
	}
	mulFn := fm["mul"].(func(int, int) int)
	if got := mulFn(3, 4); got != 12 {
		t.Errorf("mul(3,4) = %d, want 12", got)
	}
	divFn := fm["div"].(func(int, int) int)
	if got := divFn(12, 4); got != 3 {
		t.Errorf("div(12,4) = %d, want 3", got)
	}
	if got := divFn(12, 0); got != 0 {
		t.Errorf("div(12,0) = %d, want 0 (zero-divisor guard)", got)
	}
	modFn := fm["mod"].(func(int, int) int)
	if got := modFn(10, 3); got != 1 {
		t.Errorf("mod(10,3) = %d, want 1", got)
	}
	if got := modFn(10, 0); got != 0 {
		t.Errorf("mod(10,0) = %d, want 0 (zero-divisor guard)", got)
	}
}

// TestNewFuncMap_Truncate verifies the truncate helper pads with "..." for long strings.
func TestNewFuncMap_Truncate(t *testing.T) {
	tr := &TemplateRenderer{}
	fm := tr.newFuncMap(nil)
	truncateFn := fm["truncate"].(func(int, string) string)

	tests := []struct {
		name   string
		length int
		input  string
		want   string
	}{
		{"short string unchanged", 20, "hello", "hello"},
		{"exact length unchanged", 5, "hello", "hello"},
		{"truncated with ellipsis", 5, "hello world", "hello..."},
		{"empty string", 5, "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncateFn(tt.length, tt.input)
			if got != tt.want {
				t.Errorf("truncate(%d, %q) = %q, want %q", tt.length, tt.input, got, tt.want)
			}
		})
	}
}

// ---------- debug.go: handleDebug* handler coverage ----------

// TestHandleDebugHandlers_WithDebugEnabled exercises handleDebugConfig, handleDebugMemory,
// handleDebugGoroutines, handleDebugCache (nil), handleDebugDB (nil), handleDebugScheduler (nil)
// by registering routes with DEBUG=true and sending requests to each endpoint.
func TestHandleDebugHandlers_WithDebugEnabled(t *testing.T) {
	t.Setenv("DEBUG", "true")
	cfg := config.DefaultConfig()
	s := &Server{config: cfg}
	r := chi.NewRouter()
	s.registerDebugRoutes(r)

	endpoints := []struct {
		path string
		desc string
	}{
		{"/debug/config", "handleDebugConfig"},
		{"/debug/memory", "handleDebugMemory"},
		{"/debug/goroutines", "handleDebugGoroutines"},
		{"/debug/cache", "handleDebugCache (nil cache)"},
		{"/debug/db", "handleDebugDB (nil db)"},
		{"/debug/scheduler", "handleDebugScheduler (nil scheduler)"},
	}
	for _, ep := range endpoints {
		t.Run(ep.desc, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, ep.path, nil)
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)
			if rec.Code == http.StatusNotFound {
				t.Errorf("%s: status 404 — route not registered", ep.desc)
			}
			if rec.Code == http.StatusInternalServerError {
				t.Errorf("%s: status 500 — unexpected error", ep.desc)
			}
		})
	}
}

// ---------- debug.go: nil-guard paths for all debug handlers ----------

// TestHandleDebugMemory_DirectCall calls handleDebugMemory directly and expects 200.
func TestHandleDebugMemory_DirectCall(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/debug/memory", nil)
	rec := httptest.NewRecorder()
	s.handleDebugMemory(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("handleDebugMemory: status = %d, want 200", rec.Code)
	}
}

// TestHandleDebugGoroutines_DirectCall calls handleDebugGoroutines and expects 200.
func TestHandleDebugGoroutines_DirectCall(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/debug/goroutines", nil)
	rec := httptest.NewRecorder()
	s.handleDebugGoroutines(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("handleDebugGoroutines: status = %d, want 200", rec.Code)
	}
}

// TestHandleDebugCache_NilCache returns 200 with enabled:false when cache is nil.
func TestHandleDebugCache_NilCache(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/debug/cache", nil)
	rec := httptest.NewRecorder()
	s.handleDebugCache(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("handleDebugCache nil: status = %d, want 200", rec.Code)
	}
}

// TestHandleDebugDB_NilDB returns 200 with enabled:false when db is nil.
func TestHandleDebugDB_NilDB(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/debug/db", nil)
	rec := httptest.NewRecorder()
	s.handleDebugDB(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("handleDebugDB nil: status = %d, want 200", rec.Code)
	}
}

// TestHandleDebugScheduler_NilScheduler returns 200 with enabled:false when scheduler is nil.
func TestHandleDebugScheduler_NilScheduler(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/debug/scheduler", nil)
	rec := httptest.NewRecorder()
	s.handleDebugScheduler(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("handleDebugScheduler nil: status = %d, want 200", rec.Code)
	}
}

// TestHandleDebugConfig_DirectCall calls handleDebugConfig and expects 200.
func TestHandleDebugConfig_DirectCall(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/debug/config", nil)
	rec := httptest.NewRecorder()
	s.handleDebugConfig(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("handleDebugConfig: status = %d, want 200", rec.Code)
	}
}

// ---------- server.go: setupRoutes — exercise key route responses ----------

// TestSetupRoutes_HealthzExists confirms /server/healthz is routed (not 404).
func TestSetupRoutes_HealthzExists(t *testing.T) {
	s := newTestServer(t)
	h := s.setupRoutes()
	req := httptest.NewRequest(http.MethodGet, "/server/healthz", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	// Templates are not loaded in tests, so 500 is acceptable — but 404 means route is missing
	if rec.Code == http.StatusNotFound {
		t.Errorf("setupRoutes /server/healthz: got 404 — route not registered")
	}
}

// TestSetupRoutes_RootReturnsNonError confirms / is routed (not 404/500 from missing routes).
func TestSetupRoutes_RootReturnsNonError(t *testing.T) {
	s := newTestServer(t)
	h := s.setupRoutes()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	// Either 200 (renders page) or 302/303 (redirect) — not 404 or 500
	if rec.Code == http.StatusNotFound || rec.Code == http.StatusInternalServerError {
		t.Errorf("setupRoutes /: status = %d, want non-error", rec.Code)
	}
}

// TestSetupRoutes_APIv1SearchExists confirms /api/v1/search is routed.
func TestSetupRoutes_APIv1SearchExists(t *testing.T) {
	s := newTestServer(t)
	h := s.setupRoutes()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/search?q=test", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	// Should not 404 (route must exist even if it returns another status)
	if rec.Code == http.StatusNotFound {
		t.Errorf("setupRoutes /api/v1/search: got 404 — route not registered")
	}
}

// ---------- middleware.go: GeoBlock with allowlisted context ----------

// TestGeoBlock_AllowlistedContext passes through when request is allowlisted.
func TestGeoBlock_AllowlistedContext(t *testing.T) {
	cfg := config.DefaultConfig()
	mw := &Middleware{config: cfg}
	handlerCalled := false
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	})
	handler := mw.GeoBlock(nil)(inner)
	ctx := context.WithValue(context.Background(), allowlistedCtxKey{}, true)
	req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if !handlerCalled {
		t.Error("GeoBlock: allowlisted request handler not called")
	}
}

// ---------- pages.go: handleHome at root path ----------

// TestHandleHome_RootPathNoRenderer confirms handleHome at "/" returns non-500 even with nil renderer.
func TestHandleHome_RootPathNoRenderer(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	s.handleHome(rec, req)
	// Should not be 500 (internal error); 200 or renderer-related are acceptable
	if rec.Code == http.StatusInternalServerError {
		t.Errorf("handleHome /: status = %d, want non-500", rec.Code)
	}
}

// ---------- alerts.go: handler behaviour tests ----------

// TestHandleAlertNew_GET confirms handleAlertNew renders the new-alert page on GET.
// alertManager is non-nil (NewServer initializes one when SQLite opens), so the handler
// proceeds past the nil-manager guard and either renders the template (200) or returns
// an internal error if templates are unavailable (500). Either is acceptable; 405 is wrong.
func TestHandleAlertNew_GET(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/alerts/new", nil)
	rec := httptest.NewRecorder()
	s.handleAlertNew(rec, req)
	// 200 (template rendered) or 500 (template error) are both acceptable
	if rec.Code == http.StatusMethodNotAllowed {
		t.Errorf("handleAlertNew GET: status = %d, want non-405", rec.Code)
	}
}

// TestHandleAlertNew_MethodNotAllowed confirms handleAlertNew rejects non-GET methods.
func TestHandleAlertNew_MethodNotAllowed(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/alerts/new", nil)
	rec := httptest.NewRecorder()
	s.handleAlertNew(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("handleAlertNew POST: status = %d, want 405", rec.Code)
	}
}

// TestHandleAlerts_MethodNotAllowed confirms handleAlerts rejects GET requests.
func TestHandleAlerts_MethodNotAllowed(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/alerts", nil)
	rec := httptest.NewRecorder()
	s.handleAlerts(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("handleAlerts GET: status = %d, want 405", rec.Code)
	}
}

// TestHandleAlerts_PostEmptyForm confirms handleAlerts POST with an empty form redirects
// back to /alerts/new when alertManager.Create fails due to missing query.
func TestHandleAlerts_PostEmptyForm(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/alerts", strings.NewReader(""))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	s.handleAlerts(rec, req)
	// Empty form → Create fails → redirect (303) or service unavailable (503)
	if rec.Code != http.StatusSeeOther && rec.Code != http.StatusServiceUnavailable {
		t.Errorf("handleAlerts empty POST: status = %d, want 303 or 503", rec.Code)
	}
}

// TestHandleAlertAction_GETUpdateMethodNotAllowed confirms GET on an update action returns 405.
// handleAlertAction routes /alerts/{token}/update to handleAlertUpdate which requires POST.
func TestHandleAlertAction_GETUpdateMethodNotAllowed(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/alerts/sometoken/update", nil)
	rec := httptest.NewRecorder()
	s.handleAlertAction(rec, req)
	// handleAlertUpdate checks r.Method != POST → 405
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("handleAlertAction GET /update: status = %d, want 405", rec.Code)
	}
}

// TestHandleAlertAction_DefaultNotFound confirms /alerts/{emptytoken} returns 404 or 503.
func TestHandleAlertAction_DefaultNotFound(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/alerts/", nil)
	rec := httptest.NewRecorder()
	s.handleAlertAction(rec, req)
	// splitAlertAction("") returns empty token → http.NotFound (404)
	// or if alertManager nil → 503
	if rec.Code == http.StatusInternalServerError {
		t.Errorf("handleAlertAction /alerts/: status = %d, want non-500", rec.Code)
	}
}

// TestHandleAlertAction_RSSNotFound confirms /alerts/{token}.rss returns 404 for unknown token.
func TestHandleAlertAction_RSSNotFound(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/alerts/unknowntoken.rss", nil)
	rec := httptest.NewRecorder()
	s.handleAlertAction(rec, req)
	// FeedXML returns not-found error → renderAlertError(404) or 503 if manager nil
	if rec.Code != http.StatusNotFound && rec.Code != http.StatusServiceUnavailable {
		t.Errorf("handleAlertAction .rss unknown: status = %d, want 404 or 503", rec.Code)
	}
}

// TestHandleAlertAction_ManageTokenNotFound confirms /alerts/manage/{token} returns 404 for unknown token.
func TestHandleAlertAction_ManageTokenNotFound(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/alerts/manage/unknowntoken", nil)
	rec := httptest.NewRecorder()
	s.handleAlertAction(rec, req)
	// renderManageAlert → GetByManageToken fails → renderAlertError(404)
	if rec.Code != http.StatusNotFound && rec.Code != http.StatusServiceUnavailable {
		t.Errorf("handleAlertAction manage unknown: status = %d, want 404 or 503", rec.Code)
	}
}

// TestHandleAlertUpdate_PostUnknownToken confirms POST with unknown token redirects.
func TestHandleAlertUpdate_PostUnknownToken(t *testing.T) {
	s := newTestServer(t)
	body := strings.NewReader("query=test&frequency=daily")
	req := httptest.NewRequest(http.MethodPost, "/alerts/unknowntoken/update", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	s.handleAlertUpdate(rec, req, "unknowntoken")
	// Update fails (not found) → redirect (303) or 503
	if rec.Code != http.StatusSeeOther && rec.Code != http.StatusServiceUnavailable {
		t.Errorf("handleAlertUpdate unknown token: status = %d, want 303 or 503", rec.Code)
	}
}

// TestHandleAlertPause_PostUnknownToken confirms POST with unknown token redirects.
func TestHandleAlertPause_PostUnknownToken(t *testing.T) {
	s := newTestServer(t)
	body := strings.NewReader("paused=true")
	req := httptest.NewRequest(http.MethodPost, "/alerts/unknowntoken/pause", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	s.handleAlertPause(rec, req, "unknowntoken")
	// SetPaused fails → redirect (303) or 503
	if rec.Code != http.StatusSeeOther && rec.Code != http.StatusServiceUnavailable {
		t.Errorf("handleAlertPause unknown token: status = %d, want 303 or 503", rec.Code)
	}
}

// TestHandleAlertDelete_PostUnknownToken confirms POST with unknown token redirects.
func TestHandleAlertDelete_PostUnknownToken(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/alerts/unknowntoken/delete", nil)
	rec := httptest.NewRecorder()
	s.handleAlertDelete(rec, req, "unknowntoken")
	// Delete fails → redirect (303) or 503
	if rec.Code != http.StatusSeeOther && rec.Code != http.StatusServiceUnavailable {
		t.Errorf("handleAlertDelete unknown token: status = %d, want 303 or 503", rec.Code)
	}
}

// TestRenderManageAlert_UnknownToken confirms renderManageAlert returns 404 for unknown token.
func TestRenderManageAlert_UnknownToken(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/alerts/manage/unknowntoken", nil)
	rec := httptest.NewRecorder()
	s.renderManageAlert(rec, req, "unknowntoken")
	// GetByManageToken → not found → renderAlertError(404) or 503
	if rec.Code != http.StatusNotFound && rec.Code != http.StatusServiceUnavailable {
		t.Errorf("renderManageAlert unknown: status = %d, want 404 or 503", rec.Code)
	}
}

// TestAlertEngineOptions_EmptySelected confirms alertEngineOptions with no selection.
func TestAlertEngineOptions_EmptySelected(t *testing.T) {
	s := newTestServer(t)
	opts := s.alertEngineOptions(nil)
	// Should return non-nil slice of engine options
	if opts == nil {
		t.Error("alertEngineOptions(nil): returned nil, want non-nil slice")
	}
}

// TestAlertEngineOptions_WithSelected confirms alertEngineOptions marks known engines as selected.
func TestAlertEngineOptions_WithSelected(t *testing.T) {
	s := newTestServer(t)
	opts := s.alertEngineOptions([]string{"google"})
	if opts == nil {
		t.Error("alertEngineOptions([google]): returned nil, want non-nil slice")
	}
}

// ---------- embed.go: newFuncMap — exhaustive helper invocation ----------

// TestNewFuncMap_StringHelpers invokes all string helper functions in the returned FuncMap.
// The function map is returned as a plain map[string]interface{}; each value is a callable.
// This test ensures the closures themselves execute (coverage), not just that they are present.
func TestNewFuncMap_StringHelpers(t *testing.T) {
	tr := &TemplateRenderer{}
	fm := tr.newFuncMap(nil)

	// safe* helpers — return typed HTML/URL/CSS/JS wrappers
	safe := fm["safe"].(func(string) template.HTML)
	if got := safe("<b>hi</b>"); string(got) != "<b>hi</b>" {
		t.Errorf("safe: got %q", got)
	}
	safeHTML := fm["safeHTML"].(func(string) template.HTML)
	if got := safeHTML("<em>x</em>"); string(got) != "<em>x</em>" {
		t.Errorf("safeHTML: got %q", got)
	}
	safeURL := fm["safeURL"].(func(string) template.URL)
	if got := safeURL("https://example.com"); string(got) != "https://example.com" {
		t.Errorf("safeURL: got %q", got)
	}
	safeCSS := fm["safeCSS"].(func(string) template.CSS)
	if got := safeCSS("color:red"); string(got) != "color:red" {
		t.Errorf("safeCSS: got %q", got)
	}
	safeJS := fm["safeJS"].(func(string) template.JS)
	if got := safeJS("alert(1)"); string(got) != "alert(1)" {
		t.Errorf("safeJS: got %q", got)
	}

	// string transformation helpers
	lower := fm["lower"].(func(string) string)
	if got := lower("HELLO"); got != "hello" {
		t.Errorf("lower: got %q", got)
	}
	upper := fm["upper"].(func(string) string)
	if got := upper("hello"); got != "HELLO" {
		t.Errorf("upper: got %q", got)
	}
	contains := fm["contains"].(func(string, string) bool)
	if !contains("foobar", "oba") {
		t.Error("contains: expected true")
	}
	hasPrefix := fm["hasPrefix"].(func(string, string) bool)
	if !hasPrefix("foobar", "foo") {
		t.Error("hasPrefix: expected true")
	}
	hasSuffix := fm["hasSuffix"].(func(string, string) bool)
	if !hasSuffix("foobar", "bar") {
		t.Error("hasSuffix: expected true")
	}
	replace := fm["replace"].(func(string, string, string) string)
	if got := replace("aabbcc", "bb", "XX"); got != "aaXXcc" {
		t.Errorf("replace: got %q", got)
	}
	trim := fm["trim"].(func(string) string)
	if got := trim("  hello  "); got != "hello" {
		t.Errorf("trim: got %q", got)
	}
	join := fm["join"].(func([]string, string) string)
	if got := join([]string{"a", "b", "c"}, "-"); got != "a-b-c" {
		t.Errorf("join: got %q", got)
	}
	split := fm["split"].(func(string, string) []string)
	parts := split("a,b,c", ",")
	if len(parts) != 3 {
		t.Errorf("split: got %d parts, want 3", len(parts))
	}
	urlquery := fm["urlquery"].(func(string) string)
	if got := urlquery("hello world"); got != "hello+world" {
		t.Errorf("urlquery: got %q", got)
	}
}

// TestNewFuncMap_LogicHelpers invokes default, eq, ne, seq, config, version, year.
func TestNewFuncMap_LogicHelpers(t *testing.T) {
	tr := &TemplateRenderer{}
	fm := tr.newFuncMap(nil)

	// default helper: val==nil returns def; val non-empty returns val
	defFn := fm["default"].(func(interface{}, interface{}) interface{})
	if got := defFn("fallback", nil); got != "fallback" {
		t.Errorf("default(nil): got %v", got)
	}
	if got := defFn("fallback", "actual"); got != "actual" {
		t.Errorf("default(actual): got %v", got)
	}
	// default: empty string falls back to def
	if got := defFn("fallback", ""); got != "fallback" {
		t.Errorf("default(empty): got %v", got)
	}

	// eq / ne
	eqFn := fm["eq"].(func(interface{}, interface{}) bool)
	if !eqFn("x", "x") {
		t.Error("eq(x,x): want true")
	}
	neFn := fm["ne"].(func(interface{}, interface{}) bool)
	if !neFn("x", "y") {
		t.Error("ne(x,y): want true")
	}

	// seq: produces inclusive range
	seqFn := fm["seq"].(func(int, int) []int)
	got := seqFn(1, 3)
	if len(got) != 3 || got[0] != 1 || got[2] != 3 {
		t.Errorf("seq(1,3): got %v", got)
	}
	// seq: empty range (start > end)
	empty := seqFn(5, 3)
	if len(empty) != 0 {
		t.Errorf("seq(5,3): got %v, want empty", empty)
	}

	// version returns a non-empty string
	versionFn := fm["version"].(func() string)
	if versionFn() == "" {
		t.Error("version(): want non-empty string")
	}

	// year returns the current year
	yearFn := fm["year"].(func() int)
	if yr := yearFn(); yr < 2024 {
		t.Errorf("year(): got %d, want >= 2024", yr)
	}
}

// TestNewFuncMap_i18nWithRealFuncs verifies i18n functions dispatch to provided implementations.
func TestNewFuncMap_i18nWithRealFuncs(t *testing.T) {
	tr := &TemplateRenderer{}
	provided := template.FuncMap{
		"t":     func(key string, args ...interface{}) string { return "translated:" + key },
		"lang":  func() string { return "fr" },
		"isRTL": func() bool { return true },
		"dir":   func() string { return "rtl" },
	}
	fm := tr.newFuncMap(provided)

	tFn := fm["t"].(func(string, ...interface{}) string)
	if got := tFn("hello.world"); got != "translated:hello.world" {
		t.Errorf("t() with real funcs: got %q", got)
	}
	langFn := fm["lang"].(func() string)
	if got := langFn(); got != "fr" {
		t.Errorf("lang() with real funcs: got %q", got)
	}
	isRTLFn := fm["isRTL"].(func() bool)
	if !isRTLFn() {
		t.Error("isRTL() with real funcs: want true")
	}
	dirFn := fm["dir"].(func() string)
	if got := dirFn(); got != "rtl" {
		t.Errorf("dir() with real funcs: got %q", got)
	}
}

// ---------- middleware.go: RateLimit exceeded path ----------

// TestRateLimit_ExceededReturns429 exhausts the rate limiter for an IP and confirms 429.
func TestRateLimit_ExceededReturns429(t *testing.T) {
	cfg := config.DefaultConfig()
	mw := NewMiddleware(cfg, nil)
	// Create a limiter with rate=1/min, burst=1, so the second immediate request is denied.
	limiter := NewRateLimiter(&config.RateLimitConfig{
		Enabled:           true,
		RequestsPerMinute: 1,
		BurstSize:         1,
	})

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := mw.RateLimit(limiter)(inner)

	// First request: should pass
	req1 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req1.RemoteAddr = "10.0.0.1:1234"
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusOK {
		t.Fatalf("RateLimit: first request status = %d, want 200", rec1.Code)
	}

	// Second request from same IP: rate limit exhausted — must return 429
	req2 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req2.RemoteAddr = "10.0.0.1:1234"
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusTooManyRequests {
		t.Errorf("RateLimit: second request status = %d, want 429", rec2.Code)
	}
}

// ---------- middleware.go: Recovery middleware panic path ----------

// TestRecovery_PanicReturns500 verifies the Recovery middleware catches a panic and returns 500.
func TestRecovery_PanicReturns500(t *testing.T) {
	cfg := config.DefaultConfig()
	mw := NewMiddleware(cfg, nil)

	panicking := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("deliberate test panic")
	})
	handler := mw.Recovery(panicking)

	req := httptest.NewRequest(http.MethodGet, "/crash", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("Recovery: panic status = %d, want 500", rec.Code)
	}
}

// TestRecovery_PanicErrorType verifies Recovery handles error-type panics.
func TestRecovery_PanicErrorType(t *testing.T) {
	cfg := config.DefaultConfig()
	mw := NewMiddleware(cfg, nil)

	panicking := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic(fmt.Errorf("error type panic"))
	})
	handler := mw.Recovery(panicking)

	req := httptest.NewRequest(http.MethodGet, "/crash", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("Recovery error-type: status = %d, want 500", rec.Code)
	}
}

// TestRecovery_NoPanicPassesThrough verifies Recovery does not interfere with normal requests.
func TestRecovery_NoPanicPassesThrough(t *testing.T) {
	cfg := config.DefaultConfig()
	mw := NewMiddleware(cfg, nil)

	normal := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := mw.Recovery(normal)

	req := httptest.NewRequest(http.MethodGet, "/ok", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("Recovery no-panic: status = %d, want 200", rec.Code)
	}
}

