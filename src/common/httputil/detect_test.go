package httputil

import (
	"net/http/httptest"
	"testing"
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
		{"txt extension", "/api/v1/status.txt", "", "", "text/plain"},
		{"json accept", "/api/v1/status", "", "application/json", "application/json"},
		{"plain accept", "/api/v1/status", "", "text/plain", "text/plain"},
		{"html accept", "/page", "", "text/html", "text/html"},
		{"our cli", "/api/v1/status", "search-cli/1.0.0", "", "application/json"},
		{"http tool", "/api/v1/status", "curl/7.68.0", "", "text/plain"},
		{"browser", "/page", "Mozilla/5.0 Chrome", "", "text/html"},
		{"api path", "/api/v1/status", "", "", "application/json"},
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
