package httputil

import (
	"crypto/tls"
	"net/http/httptest"
	"testing"

	"github.com/apimgr/search/src/config"
	"github.com/apimgr/search/src/version"
)

func TestProjectName(t *testing.T) {
	if ProjectName != "search" {
		t.Errorf("ProjectName = %q, want %q", ProjectName, "search")
	}
}

func TestIsOurCliClient(t *testing.T) {
	tests := []struct {
		userAgent string
		want      bool
	}{
		{"search-cli/1.0.0", true},
		{"search-cli/2.0.0 Linux", true},
		{"Mozilla/5.0 Chrome", false},
		{"curl/7.68.0", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.userAgent, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			req.Header.Set("User-Agent", tt.userAgent)
			got := IsOurCliClient(req)
			if got != tt.want {
				t.Errorf("IsOurCliClient() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsTextBrowser(t *testing.T) {
	tests := []struct {
		userAgent string
		want      bool
	}{
		{"Lynx/2.8.9rel.1", true},
		{"w3m/0.5.3", true},
		{"Links (2.21; Linux)", true},
		{"ELinks/0.13.1", true},
		{"Browsh/1.6.4", true},
		{"Mozilla/5.0 Chrome", false},
		{"curl/7.68.0", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.userAgent, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			req.Header.Set("User-Agent", tt.userAgent)
			got := IsTextBrowser(req)
			if got != tt.want {
				t.Errorf("IsTextBrowser() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsHttpTool(t *testing.T) {
	tests := []struct {
		userAgent string
		want      bool
	}{
		{"curl/7.68.0", true},
		{"wget/1.21", true},
		{"HTTPie/2.4.0", true},
		{"python-requests/2.25.1", true},
		{"Go-http-client/1.1", true},
		{"axios/0.21.1", true},
		{"node-fetch/2.6.1", true},
		{"Postman Runtime", true},
		{"insomnia/2021.3.0", true},
		{"Mozilla/5.0 Chrome", false},
		{"Lynx/2.8.9", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.userAgent, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			req.Header.Set("User-Agent", tt.userAgent)
			got := IsHttpTool(req)
			if got != tt.want {
				t.Errorf("IsHttpTool() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsNonInteractiveClient(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("User-Agent", "curl/7.68.0")
	if !IsNonInteractiveClient(req) {
		t.Error("curl should be non-interactive")
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 Chrome")
	if IsNonInteractiveClient(req) {
		t.Error("Chrome should be interactive")
	}
}

func TestIsBrowser(t *testing.T) {
	tests := []struct {
		userAgent string
		accept    string
		want      bool
	}{
		{"Mozilla/5.0 Chrome/91.0", "", true},
		{"Mozilla/5.0 Firefox/89.0", "", true},
		{"Mozilla/5.0 Safari/605.1", "", true},
		{"Mozilla/5.0 Edge/91.0", "", true},
		{"Opera/9.80", "", true},
		{"curl/7.68.0", "", false},
		{"search-cli/1.0.0", "", false},
		{"Lynx/2.8.9", "", false},
		{"Unknown Agent", "text/html", true},
		{"Unknown Agent", "application/json", false},
	}

	for _, tt := range tests {
		t.Run(tt.userAgent, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			req.Header.Set("User-Agent", tt.userAgent)
			if tt.accept != "" {
				req.Header.Set("Accept", tt.accept)
			}
			got := IsBrowser(req)
			if got != tt.want {
				t.Errorf("IsBrowser() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestClientTypeString(t *testing.T) {
	tests := []struct {
		ct   ClientType
		want string
	}{
		{ClientTypeUnknown, "unknown"},
		{ClientTypeBrowser, "browser"},
		{ClientTypeTextBrowser, "text_browser"},
		{ClientTypeHttpTool, "http_tool"},
		{ClientTypeOurCLI, "our_cli"},
		{ClientTypeAPI, "api"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := tt.ct.String()
			if got != tt.want {
				t.Errorf("ClientType(%d).String() = %q, want %q", tt.ct, got, tt.want)
			}
		})
	}
}

func TestDetectClientType(t *testing.T) {
	tests := []struct {
		name      string
		userAgent string
		accept    string
		want      ClientType
	}{
		{"our cli", "search-cli/1.0.0", "", ClientTypeOurCLI},
		{"text browser", "Lynx/2.8.9", "", ClientTypeTextBrowser},
		{"http tool", "curl/7.68.0", "", ClientTypeHttpTool},
		{"api client", "Custom Agent", "application/json", ClientTypeAPI},
		{"browser", "Mozilla/5.0 Chrome", "", ClientTypeBrowser},
		{"unknown", "Unknown", "", ClientTypeUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			req.Header.Set("User-Agent", tt.userAgent)
			if tt.accept != "" {
				req.Header.Set("Accept", tt.accept)
			}
			got := DetectClientType(req)
			if got != tt.want {
				t.Errorf("DetectClientType() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetPreferredFormat(t *testing.T) {
	tests := []struct {
		name      string
		path      string
		userAgent string
		accept    string
		want      string
	}{
		{"txt extension", version.APIPrefix + "/status.txt", "", "", "text/plain"},
		{"json accept", version.APIPrefix + "/status", "", "application/json", "application/json"},
		{"plain accept", version.APIPrefix + "/status", "", "text/plain", "text/plain"},
		{"html accept", "/page", "", "text/html", "text/html"},
		{"our cli", version.APIPrefix + "/status", "search-cli/1.0.0", "", "application/json"},
		{"http tool", version.APIPrefix + "/status", "curl/7.68.0", "", "text/plain"},
		{"browser", "/page", "Mozilla/5.0 Chrome", "", "text/html"},
		{"api path", version.APIPrefix + "/status", "", "", "application/json"},
		{"default", "/page", "", "", "text/html"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", tt.path, nil)
			if tt.userAgent != "" {
				req.Header.Set("User-Agent", tt.userAgent)
			}
			if tt.accept != "" {
				req.Header.Set("Accept", tt.accept)
			}
			got := GetPreferredFormat(req)
			if got != tt.want {
				t.Errorf("GetPreferredFormat() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestClientTypeConstants(t *testing.T) {
	// Verify constants have expected values
	if ClientTypeUnknown != 0 {
		t.Errorf("ClientTypeUnknown = %d, want 0", ClientTypeUnknown)
	}
	if ClientTypeBrowser != 1 {
		t.Errorf("ClientTypeBrowser = %d, want 1", ClientTypeBrowser)
	}
}

func TestNormalizeBasePath(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"empty string", "", "/"},
		{"root slash", "/", "/"},
		{"simple path", "/app", "/app"},
		{"trailing slash stripped", "/app/", "/app"},
		{"no leading slash added", "app", "/app"},
		// double leading slash: already starts with "/" so no prefix added, trailing slash stripped
		{"double slash normalized", "//app/", "//app"},
		{"nested path trailing slash stripped", "/app/sub/", "/app/sub"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeBasePath(tt.input)
			if got != tt.want {
				t.Errorf("normalizeBasePath(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestGetProtoFromRequest(t *testing.T) {
	tests := []struct {
		name               string
		forwardedProto     string
		forwardedSsl       string
		tlsConnectionState bool
		want               string
	}{
		{"X-Forwarded-Proto https", "https", "", false, "https"},
		{"X-Forwarded-Proto http", "http", "", false, "http"},
		{"X-Forwarded-Proto uppercase HTTPS", "HTTPS", "", false, "https"},
		{"X-Forwarded-Ssl on", "", "on", false, "https"},
		{"X-Forwarded-Ssl off", "", "off", false, "http"},
		{"TLS connection state set", "", "", true, "https"},
		{"no headers no TLS", "", "", false, "http"},
		// X-Forwarded-Proto takes priority over X-Forwarded-Ssl
		{"X-Forwarded-Proto overrides Ssl", "http", "on", false, "http"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			// Loopback so the trust gate passes for X-Forwarded-* headers
			req.RemoteAddr = "127.0.0.1:1234"
			if tt.forwardedProto != "" {
				req.Header.Set("X-Forwarded-Proto", tt.forwardedProto)
			}
			if tt.forwardedSsl != "" {
				req.Header.Set("X-Forwarded-Ssl", tt.forwardedSsl)
			}
			if tt.tlsConnectionState {
				req.TLS = &tls.ConnectionState{}
			}
			got := GetProtoFromRequest(req)
			if got != tt.want {
				t.Errorf("GetProtoFromRequest() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGetHostFromRequest(t *testing.T) {
	tests := []struct {
		name          string
		forwardedHost string
		requestHost   string
		want          string
	}{
		{"X-Forwarded-Host single", "example.com", "", "example.com"},
		{"X-Forwarded-Host multiple takes first", "example.com, proxy.internal", "", "example.com"},
		{"X-Forwarded-Host with port", "example.com:8443", "", "example.com:8443"},
		{"falls back to r.Host", "", "localhost:8080", "localhost:8080"},
		{"no headers no host", "", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			// Loopback so the trust gate passes for X-Forwarded-* headers
			req.RemoteAddr = "127.0.0.1:1234"
			if tt.forwardedHost != "" {
				req.Header.Set("X-Forwarded-Host", tt.forwardedHost)
			}
			// httptest.NewRequest sets r.Host from the URL; override it explicitly
			req.Host = tt.requestHost
			got := GetHostFromRequest(req)
			if got != tt.want {
				t.Errorf("GetHostFromRequest() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGetPortFromRequest(t *testing.T) {
	tests := []struct {
		name           string
		forwardedPort  string
		forwardedProto string
		forwardedHost  string
		requestHost    string
		want           string
	}{
		{"X-Forwarded-Port explicit", "8443", "", "", "", "8443"},
		{"port extracted from host header", "", "", "", "example.com:9090", "9090"},
		// no port in host, no TLS → proto defaults to http → port 80
		{"default port for http", "", "", "", "example.com", "80"},
		// no port in host, X-Forwarded-Proto https → port 443
		{"default port for https", "", "https", "", "example.com", "443"},
		// X-Forwarded-Port takes priority over everything
		{"forwarded port overrides host port", "8080", "", "", "example.com:9090", "8080"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			// Loopback so the trust gate passes for X-Forwarded-* headers
			req.RemoteAddr = "127.0.0.1:1234"
			if tt.forwardedPort != "" {
				req.Header.Set("X-Forwarded-Port", tt.forwardedPort)
			}
			if tt.forwardedProto != "" {
				req.Header.Set("X-Forwarded-Proto", tt.forwardedProto)
			}
			if tt.forwardedHost != "" {
				req.Header.Set("X-Forwarded-Host", tt.forwardedHost)
			}
			req.Host = tt.requestHost
			got := GetPortFromRequest(req)
			if got != tt.want {
				t.Errorf("GetPortFromRequest() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGetBaseURLFromRequest(t *testing.T) {
	tests := []struct {
		name            string
		forwardedPrefix string
		forwardedPath   string
		scriptName      string
		want            string
	}{
		{"X-Forwarded-Prefix used first", "/myapp", "/other", "/alt", "/myapp"},
		{"X-Forwarded-Path used when no prefix", "", "/myapp", "/alt", "/myapp"},
		{"X-Script-Name used as fallback", "", "", "/myapp", "/myapp"},
		// trailing slash on prefix is normalized away
		{"prefix trailing slash stripped", "/myapp/", "", "", "/myapp"},
		// no headers: falls back to config.GetBaseURL()
		{"no headers falls back to config", "", "", "", "/"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset config override to "/" so the fallback is predictable
			config.SetBaseURLOverride("/")
			req := httptest.NewRequest("GET", "/", nil)
			// Loopback so the trust gate passes for X-Forwarded-* headers
			req.RemoteAddr = "127.0.0.1:1234"
			if tt.forwardedPrefix != "" {
				req.Header.Set("X-Forwarded-Prefix", tt.forwardedPrefix)
			}
			if tt.forwardedPath != "" {
				req.Header.Set("X-Forwarded-Path", tt.forwardedPath)
			}
			if tt.scriptName != "" {
				req.Header.Set("X-Script-Name", tt.scriptName)
			}
			got := GetBaseURLFromRequest(req)
			if got != tt.want {
				t.Errorf("GetBaseURLFromRequest() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildFullURL(t *testing.T) {
	tests := []struct {
		name            string
		forwardedProto  string
		forwardedHost   string
		forwardedPort   string
		forwardedPrefix string
		requestHost     string
		path            string
		want            string
	}{
		{
			name:        "plain http localhost with port",
			requestHost: "localhost:8080",
			path:        "/search",
			want:        "http://localhost:8080/search",
		},
		{
			name:           "https via forwarded headers default port omitted",
			forwardedProto: "https",
			forwardedHost:  "example.com",
			forwardedPort:  "443",
			path:           "/search",
			want:           "https://example.com/search",
		},
		{
			name:           "http default port 80 omitted from host",
			forwardedProto: "http",
			forwardedHost:  "example.com",
			forwardedPort:  "80",
			path:           "/search",
			want:           "http://example.com/search",
		},
		{
			name:            "base path prepended to path",
			requestHost:     "host",
			forwardedPrefix: "/app",
			path:            "/results",
			want:            "http://host/app/results",
		},
		{
			name:           "non-default https port kept in host",
			forwardedProto: "https",
			forwardedHost:  "example.com:8443",
			forwardedPort:  "8443",
			path:           "/search",
			want:           "https://example.com:8443/search",
		},
		{
			name:        "path without leading slash gets one added",
			requestHost: "localhost:9000",
			path:        "search",
			want:        "http://localhost:9000/search",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset config base URL so no stored state bleeds between sub-tests
			config.SetBaseURLOverride("/")
			req := httptest.NewRequest("GET", "/", nil)
			// Loopback so the trust gate passes for X-Forwarded-* headers
			req.RemoteAddr = "127.0.0.1:1234"
			if tt.forwardedProto != "" {
				req.Header.Set("X-Forwarded-Proto", tt.forwardedProto)
			}
			if tt.forwardedHost != "" {
				req.Header.Set("X-Forwarded-Host", tt.forwardedHost)
			}
			if tt.forwardedPort != "" {
				req.Header.Set("X-Forwarded-Port", tt.forwardedPort)
			}
			if tt.forwardedPrefix != "" {
				req.Header.Set("X-Forwarded-Prefix", tt.forwardedPrefix)
			}
			if tt.requestHost != "" {
				req.Host = tt.requestHost
			}
			got := BuildFullURL(req, tt.path)
			if got != tt.want {
				t.Errorf("BuildFullURL() = %q, want %q", got, tt.want)
			}
		})
	}
}
