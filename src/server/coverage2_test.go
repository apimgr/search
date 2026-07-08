package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// coverage2_test.go targets functions that were at 0 % or low coverage after
// the initial coverage run: banner sub-variants, well-known catchall, llms.txt,
// server status/config handlers, mapHTTPStatusToCode, certFileMatchesFQDN,
// isTrustedProxy additional branches, and validateNotPrivateProxy.

// ---------- banner.go (sub-variants) ----------

// TestPrintBannerCompact calls printBannerCompact directly for
// the 60-79-column compact code path that PrintBanner delegates to.
// Verifies it does not panic under all BannerInfo combinations.
func TestPrintBannerCompact(t *testing.T) {
	tests := []struct {
		name string
		info *BannerInfo
	}{
		{
			"empty appname defaults to Search",
			&BannerInfo{Version: "1.0.0", Mode: "production", ListenAddr: "0.0.0.0:8080"},
		},
		{
			"https addr only",
			&BannerInfo{AppName: "Search", Version: "1.0.0", Mode: "production",
				HTTPSAddr: "https://example.com", IsHTTPS: true, ListenAddr: "0.0.0.0:443"},
		},
		{
			"http addr only",
			&BannerInfo{AppName: "Search", Version: "1.0.0", Mode: "development",
				HTTPAddr: "http://localhost:8080", ListenAddr: "0.0.0.0:8080"},
		},
		{
			"both https and http dual-port",
			&BannerInfo{AppName: "Search", Version: "2.0.0", Mode: "production",
				HTTPSAddr: "https://example.com", HTTPAddr: "http://example.com",
				HTTPSPort: 443, HTTPPort: 80, IsHTTPS: true, ListenAddr: "0.0.0.0:80"},
		},
		{
			"tor addr set",
			&BannerInfo{AppName: "Search", Version: "1.0.0", Mode: "production",
				TorAddr: "http://abc.onion", ListenAddr: "0.0.0.0:8080"},
		},
		{
			"dev mode",
			&BannerInfo{AppName: "App", Version: "dev", Mode: "development",
				Debug: true, ListenAddr: "0.0.0.0:8080"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("printBannerCompact panicked: %v", r)
				}
			}()
			printBannerCompact(tt.info)
		})
	}
}

// TestPrintBannerMinimal calls printBannerMinimal directly for
// the 40-59-column minimal code path.
func TestPrintBannerMinimal(t *testing.T) {
	tests := []struct {
		name string
		info *BannerInfo
	}{
		{
			"no addr",
			&BannerInfo{AppName: "Search", Version: "1.0.0", Mode: "production", ListenAddr: "0.0.0.0:8080"},
		},
		{
			"https addr — primaryURLHost returns https",
			&BannerInfo{AppName: "Search", Version: "1.0.0", Mode: "production",
				HTTPSAddr: "https://example.com", ListenAddr: "0.0.0.0:443"},
		},
		{
			"http addr only — primaryURLHost returns http",
			&BannerInfo{AppName: "Search", Version: "1.0.0", Mode: "development",
				HTTPAddr: "http://localhost:8080", ListenAddr: "0.0.0.0:8080"},
		},
		{
			"empty appname",
			&BannerInfo{Version: "1.0.0", Mode: "production", ListenAddr: "0.0.0.0:8080"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("printBannerMinimal panicked: %v", r)
				}
			}()
			printBannerMinimal(tt.info)
		})
	}
}

// TestPrintBannerMicro calls printBannerMicro directly for
// the <40-column single-line path.
func TestPrintBannerMicro(t *testing.T) {
	tests := []struct {
		name string
		info *BannerInfo
	}{
		{
			"with https addr — prints addr",
			&BannerInfo{AppName: "Search", Version: "1.0.0", Mode: "production",
				HTTPSAddr: "https://example.com", ListenAddr: "0.0.0.0:443"},
		},
		{
			"no addr — prints appname only",
			&BannerInfo{AppName: "Search", Version: "1.0.0", Mode: "production", ListenAddr: "0.0.0.0:8080"},
		},
		{
			"empty appname defaults to Search",
			&BannerInfo{Version: "1.0.0", Mode: "production", ListenAddr: "0.0.0.0:8080"},
		},
		{
			"http addr only",
			&BannerInfo{AppName: "App", Version: "dev", Mode: "development",
				HTTPAddr: "http://localhost:8080", ListenAddr: "0.0.0.0:8080"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("printBannerMicro panicked: %v", r)
				}
			}()
			printBannerMicro(tt.info)
		})
	}
}

// TestPrintBannerPlain calls printBannerPlain directly for the
// NO_COLOR / TERM=dumb plain-text path.
func TestPrintBannerPlain(t *testing.T) {
	tests := []struct {
		name string
		info *BannerInfo
	}{
		{
			"http only plain",
			&BannerInfo{AppName: "Search", Version: "1.0.0", Mode: "production",
				HTTPAddr: "http://localhost:8080", ListenAddr: "0.0.0.0:8080"},
		},
		{
			"https only plain",
			&BannerInfo{AppName: "Search", Version: "1.0.0", Mode: "production",
				HTTPSAddr: "https://example.com", IsHTTPS: true, ListenAddr: "0.0.0.0:443"},
		},
		{
			"dual-port plain",
			&BannerInfo{AppName: "Search", Version: "1.0.0", Mode: "production",
				HTTPSAddr: "https://example.com", HTTPAddr: "http://example.com",
				HTTPSPort: 443, HTTPPort: 80, IsHTTPS: true, ListenAddr: "0.0.0.0:80"},
		},
		{
			"tor plain",
			&BannerInfo{AppName: "Search", Version: "1.0.0", Mode: "production",
				TorAddr: "http://abc.onion", ListenAddr: "0.0.0.0:8080"},
		},
		{
			"empty appname defaults to Search",
			&BannerInfo{Version: "1.0.0", Mode: "production", ListenAddr: "0.0.0.0:8080"},
		},
		{
			"is-https sets proto in listen line",
			&BannerInfo{AppName: "Search", Version: "1.0.0", Mode: "production",
				IsHTTPS: true, ListenAddr: "0.0.0.0:443"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("printBannerPlain panicked: %v", r)
				}
			}()
			printBannerPlain(tt.info)
		})
	}
}

// ---------- banner.go – primaryURLHost ----------

// TestPrimaryURLHost verifies the HTTPS-over-HTTP preference logic and the
// fallback to empty string when neither address is set.
func TestPrimaryURLHost(t *testing.T) {
	tests := []struct {
		name      string
		httpsAddr string
		httpAddr  string
		want      string
	}{
		{"https preferred over http", "https://example.com", "http://example.com", "https://example.com"},
		{"http only", "", "http://localhost:8080", "http://localhost:8080"},
		{"neither set returns empty", "", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := &BannerInfo{HTTPSAddr: tt.httpsAddr, HTTPAddr: tt.httpAddr}
			got := primaryURLHost(info)
			if got != tt.want {
				t.Errorf("primaryURLHost() = %q, want %q", got, tt.want)
			}
		})
	}
}

// ---------- wellknown.go ----------

// TestHandleWellKnownCatchAll_Methods covers the method-gating logic:
// GET/HEAD receive 404; non-GET/HEAD methods receive 405 with Allow header.
func TestHandleWellKnownCatchAll_Methods(t *testing.T) {
	s := newTestServer(t)
	tests := []struct {
		name       string
		method     string
		wantStatus int
		wantAllow  bool
	}{
		{"GET returns 404", http.MethodGet, http.StatusNotFound, false},
		{"HEAD returns 404", http.MethodHead, http.StatusNotFound, false},
		{"POST returns 405", http.MethodPost, http.StatusMethodNotAllowed, true},
		{"PUT returns 405", http.MethodPut, http.StatusMethodNotAllowed, true},
		{"DELETE returns 405", http.MethodDelete, http.StatusMethodNotAllowed, true},
		{"PATCH returns 405", http.MethodPatch, http.StatusMethodNotAllowed, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "/.well-known/some-resource", nil)
			rec := httptest.NewRecorder()
			s.handleWellKnownCatchAll(rec, req)
			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			allow := rec.Header().Get("Allow")
			if tt.wantAllow && allow == "" {
				t.Error("expected Allow header for 405 response")
			}
			if !tt.wantAllow && allow != "" {
				t.Errorf("unexpected Allow header %q for non-405 response", allow)
			}
		})
	}
}

// ---------- wellknown.go – handleLlmsTxt ----------

// TestHandleLlmsTxt confirms the handler returns 200, the correct content type,
// and the Cache-Control header.
func TestHandleLlmsTxt(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/llms.txt", nil)
	rec := httptest.NewRecorder()

	s.handleLlmsTxt(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("handleLlmsTxt status = %d, want 200", rec.Code)
	}
	ct := rec.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/plain") {
		t.Errorf("Content-Type = %q, want text/plain", ct)
	}
	cc := rec.Header().Get("Cache-Control")
	if cc == "" {
		t.Error("handleLlmsTxt: expected Cache-Control header")
	}
	body := rec.Body.String()
	if !strings.Contains(body, "## API") {
		t.Errorf("handleLlmsTxt: body does not contain '## API' section; got:\n%s", body)
	}
}

// TestHandleLlmsTxt_WithTagline exercises the tagline branch (non-empty Tagline)
// by temporarily setting the shared server's Branding.Tagline field and
// restoring it after the test to avoid mutating global state permanently.
func TestHandleLlmsTxt_WithTagline(t *testing.T) {
	s := newTestServer(t)
	origTagline := s.config.Server.Branding.Tagline
	t.Cleanup(func() { s.config.Server.Branding.Tagline = origTagline })
	s.config.Server.Branding.Tagline = "Privacy-first search"

	req := httptest.NewRequest(http.MethodGet, "/llms.txt", nil)
	rec := httptest.NewRecorder()
	s.handleLlmsTxt(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("handleLlmsTxt (tagline) status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Privacy-first search") {
		t.Error("handleLlmsTxt: tagline not present in output")
	}
}

// ---------- server.go – handleServerStatus / handleServerConfig ----------

// TestHandleServerStatus confirms the handler writes a 200 with ok:true and
// the expected top-level data keys.
func TestHandleServerStatus(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/server/status", nil)
	rec := httptest.NewRecorder()

	s.handleServerStatus(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("handleServerStatus status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, key := range []string{`"ok"`, `"data"`, `"version"`, `"mode"`, `"uptime"`, `"goroutines"`, `"memory"`} {
		if !strings.Contains(body, key) {
			t.Errorf("handleServerStatus: body missing key %q; body = %s", key, body)
		}
	}
}

// TestHandleServerConfig confirms the handler writes 200 with ok:true and a
// data field containing the sanitized configuration.
func TestHandleServerConfig(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/server/config", nil)
	rec := httptest.NewRecorder()

	s.handleServerConfig(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("handleServerConfig status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"ok"`) || !strings.Contains(body, `"data"`) {
		t.Errorf("handleServerConfig: body missing expected keys; body = %s", body)
	}
}

// ---------- response.go – mapHTTPStatusToCode ----------

// TestMapHTTPStatusToCode verifies every explicit switch case and the default.
func TestMapHTTPStatusToCode(t *testing.T) {
	tests := []struct {
		status int
		want   string
	}{
		{http.StatusBadRequest, "BAD_REQUEST"},
		{http.StatusUnauthorized, "UNAUTHORIZED"},
		{http.StatusForbidden, "FORBIDDEN"},
		{http.StatusNotFound, "NOT_FOUND"},
		{http.StatusTooManyRequests, "RATE_LIMITED"},
		{http.StatusServiceUnavailable, "MAINTENANCE"},
		{http.StatusInternalServerError, "SERVER_ERROR"},
		{http.StatusTeapot, "SERVER_ERROR"},
		{http.StatusGatewayTimeout, "SERVER_ERROR"},
	}
	for _, tt := range tests {
		t.Run(http.StatusText(tt.status), func(t *testing.T) {
			got := mapHTTPStatusToCode(tt.status)
			if got != tt.want {
				t.Errorf("mapHTTPStatusToCode(%d) = %q, want %q", tt.status, got, tt.want)
			}
		})
	}
}

// ---------- ssl.go – certFileMatchesFQDN ----------

// TestCertFileMatchesFQDN covers the early-exit error branches where a valid
// TLS certificate is not required.
func TestCertFileMatchesFQDN(t *testing.T) {
	invalidCertPEM := []byte("-----BEGIN CERTIFICATE-----\nYWJj\n-----END CERTIFICATE-----\n")
	tests := []struct {
		name     string
		content  []byte
		fqdn     string
		wantBool bool
	}{
		{"nonexistent file returns false", nil, "example.com", false},
		{"empty file not valid PEM returns false", []byte(""), "example.com", false},
		{"garbage content not valid PEM returns false", []byte("not a pem block at all"), "example.com", false},
		{"valid PEM block but invalid cert bytes returns false", invalidCertPEM, "example.com", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := ""
			if tt.content != nil {
				f, err := os.CreateTemp(t.TempDir(), "cert-*.pem")
				if err != nil {
					t.Fatalf("CreateTemp: %v", err)
				}
				if _, err := f.Write(tt.content); err != nil {
					t.Fatalf("Write: %v", err)
				}
				f.Close()
				path = f.Name()
			} else {
				path = t.TempDir() + "/nonexistent-cert.pem"
			}
			got := certFileMatchesFQDN(path, tt.fqdn)
			if got != tt.wantBool {
				t.Errorf("certFileMatchesFQDN(%q, %q) = %v, want %v", path, tt.fqdn, got, tt.wantBool)
			}
		})
	}
}

// ---------- middleware.go – isTrustedProxy ----------

// TestIsTrustedProxy covers the additional-proxies path (CIDR and plain-IP
// entries in the configured list) as well as invalid input.
func TestIsTrustedProxy(t *testing.T) {
	tests := []struct {
		name       string
		ip         string
		additional []string
		want       bool
	}{
		{"loopback 127.0.0.1 always trusted", "127.0.0.1", nil, true},
		{"IPv6 loopback ::1 always trusted", "::1", nil, true},
		{"RFC-1918 10.x always trusted", "10.0.0.1", nil, true},
		{"RFC-1918 172.16.x always trusted", "172.16.0.1", nil, true},
		{"RFC-1918 192.168.x always trusted", "192.168.1.1", nil, true},
		{"link-local 169.254.x always trusted", "169.254.1.1", nil, true},
		{"public IP not trusted without additional", "8.8.8.8", nil, false},
		{"invalid string returns false", "not-an-ip", nil, false},
		{"public IP matched by additional CIDR", "203.0.113.5", []string{"203.0.113.0/24"}, true},
		{"public IP matched by additional plain IP", "203.0.113.5", []string{"203.0.113.5"}, true},
		{"public IP not in additional CIDR", "203.0.113.5", []string{"198.51.100.0/24"}, false},
		{"empty additional entry is skipped", "8.8.8.8", []string{""}, false},
		{"additional plain IP no match", "1.2.3.5", []string{"1.2.3.4"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isTrustedProxy(tt.ip, tt.additional)
			if got != tt.want {
				t.Errorf("isTrustedProxy(%q, %v) = %v, want %v", tt.ip, tt.additional, got, tt.want)
			}
		})
	}
}

// ---------- opensearch.go – validateNotPrivateProxy ----------

// TestValidateNotPrivateProxy_Extra adds the uppercase case and the DNS
// lookup failure branch not covered by the existing TestValidateNotPrivateProxy.
func TestValidateNotPrivateProxy_Extra(t *testing.T) {
	tests := []struct {
		name    string
		host    string
		wantErr bool
	}{
		{"localhost rejected", "localhost", true},
		{"127.0.0.1 rejected", "127.0.0.1", true},
		{"::1 rejected", "::1", true},
		{".local suffix rejected", "myservice.local", true},
		{".internal suffix rejected", "myservice.internal", true},
		{".localhost suffix rejected", "foo.localhost", true},
		{"uppercase LOCALHOST also rejected", "LOCALHOST", true},
		{"DNS lookup fails for nonexistent host", "this-host-definitely-does-not-exist.invalid", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateNotPrivateProxy(tt.host)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateNotPrivateProxy(%q) error = %v, wantErr %v", tt.host, err, tt.wantErr)
			}
		})
	}
}
