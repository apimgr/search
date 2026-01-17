package user

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAuthConfigDefaults(t *testing.T) {
	config := AuthConfig{}

	if config.SessionDurationDays != 0 {
		t.Error("Default SessionDurationDays should be 0 (will be set to 7 by NewAuthManager)")
	}
	if config.CookieName != "" {
		t.Error("Default CookieName should be empty (will be set by NewAuthManager)")
	}
}

func TestExtractDeviceName(t *testing.T) {
	tests := []struct {
		userAgent string
		want      string
	}{
		// Mobile devices
		{"Mozilla/5.0 (iPhone; CPU iPhone OS 14_0 like Mac OS X)", "iPhone"},
		{"Mozilla/5.0 (iPad; CPU OS 14_0 like Mac OS X)", "iPad"},
		{"Mozilla/5.0 (Linux; Android 10; Mobile)", "Android Phone"},
		{"Mozilla/5.0 (Linux; Android 10; Tablet)", "Android Tablet"},
		// Desktop
		{"Mozilla/5.0 (Windows NT 10.0; Win64; x64)", "Windows PC"},
		{"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)", "Mac"},
		{"Mozilla/5.0 (X11; Linux x86_64)", "Linux PC"},
		// Browsers (fallback when OS not detected)
		{"Mozilla/5.0 Chrome/91.0", "Chrome Browser"},
		{"Mozilla/5.0 Firefox/89.0", "Firefox Browser"},
		{"Mozilla/5.0 Safari/605.1", "Safari Browser"},
		// Unknown
		{"", "Unknown Device"},
		{"SomeRandomBot/1.0", "Unknown Device"},
	}

	for _, tt := range tests {
		t.Run(tt.userAgent, func(t *testing.T) {
			got := extractDeviceName(tt.userAgent)
			if got != tt.want {
				t.Errorf("extractDeviceName(%q) = %q, want %q", tt.userAgent, got, tt.want)
			}
		})
	}
}

func TestAuthManagerSetSessionCookie(t *testing.T) {
	am := &AuthManager{
		cookieName:   "test_session",
		cookieDomain: "example.com",
		cookieSecure: true,
	}

	w := httptest.NewRecorder()
	am.SetSessionCookie(w, "ses_testtoken123")

	cookies := w.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("Expected 1 cookie, got %d", len(cookies))
	}

	cookie := cookies[0]
	if cookie.Name != "test_session" {
		t.Errorf("Cookie name = %q, want %q", cookie.Name, "test_session")
	}
	if cookie.Value != "ses_testtoken123" {
		t.Errorf("Cookie value = %q, want %q", cookie.Value, "ses_testtoken123")
	}
	if !cookie.HttpOnly {
		t.Error("Cookie should be HttpOnly")
	}
	if !cookie.Secure {
		t.Error("Cookie should be Secure")
	}
	if cookie.Domain != "example.com" {
		t.Errorf("Cookie domain = %q, want %q", cookie.Domain, "example.com")
	}
}

func TestAuthManagerClearSessionCookie(t *testing.T) {
	am := &AuthManager{
		cookieName:   "test_session",
		cookieDomain: "example.com",
		cookieSecure: true,
	}

	w := httptest.NewRecorder()
	am.ClearSessionCookie(w)

	cookies := w.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("Expected 1 cookie, got %d", len(cookies))
	}

	cookie := cookies[0]
	if cookie.Value != "" {
		t.Errorf("Cookie value should be empty, got %q", cookie.Value)
	}
	if cookie.MaxAge != -1 {
		t.Errorf("Cookie MaxAge should be -1, got %d", cookie.MaxAge)
	}
}

func TestAuthManagerGetSessionTokenFromCookie(t *testing.T) {
	am := &AuthManager{
		cookieName: "test_session",
	}

	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: "test_session", Value: "ses_cookietoken"})

	token := am.GetSessionToken(req)
	if token != "ses_cookietoken" {
		t.Errorf("GetSessionToken() = %q, want %q", token, "ses_cookietoken")
	}
}

func TestAuthManagerGetSessionTokenFromHeader(t *testing.T) {
	am := &AuthManager{
		cookieName: "test_session",
	}

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer ses_headertoken")

	token := am.GetSessionToken(req)
	if token != "ses_headertoken" {
		t.Errorf("GetSessionToken() = %q, want %q", token, "ses_headertoken")
	}
}

func TestAuthManagerGetSessionTokenCookiePriority(t *testing.T) {
	am := &AuthManager{
		cookieName: "test_session",
	}

	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: "test_session", Value: "ses_cookietoken"})
	req.Header.Set("Authorization", "Bearer ses_headertoken")

	// Cookie should take priority
	token := am.GetSessionToken(req)
	if token != "ses_cookietoken" {
		t.Errorf("GetSessionToken() should prefer cookie, got %q, want %q", token, "ses_cookietoken")
	}
}

func TestAuthManagerGetSessionTokenEmpty(t *testing.T) {
	am := &AuthManager{
		cookieName: "test_session",
	}

	req := httptest.NewRequest("GET", "/", nil)
	token := am.GetSessionToken(req)

	if token != "" {
		t.Errorf("GetSessionToken() should return empty string, got %q", token)
	}
}

func TestAuthManagerGetSessionTokenWrongAuthScheme(t *testing.T) {
	am := &AuthManager{
		cookieName: "test_session",
	}

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Basic dXNlcjpwYXNz")

	token := am.GetSessionToken(req)
	if token != "" {
		t.Errorf("GetSessionToken() should return empty for Basic auth, got %q", token)
	}
}

func TestAuthConfigStruct(t *testing.T) {
	config := AuthConfig{
		SessionDurationDays: 14,
		CookieName:          "custom_session",
		CookieDomain:        "example.com",
		CookieSecure:        true,
	}

	if config.SessionDurationDays != 14 {
		t.Errorf("SessionDurationDays = %d, want %d", config.SessionDurationDays, 14)
	}
	if config.CookieName != "custom_session" {
		t.Errorf("CookieName = %q, want %q", config.CookieName, "custom_session")
	}
	if config.CookieDomain != "example.com" {
		t.Errorf("CookieDomain = %q, want %q", config.CookieDomain, "example.com")
	}
	if !config.CookieSecure {
		t.Error("CookieSecure should be true")
	}
}

func TestAuthManagerStruct(t *testing.T) {
	am := &AuthManager{
		cookieName:   "session",
		cookieDomain: "test.com",
		cookieSecure: false,
	}

	if am.cookieName != "session" {
		t.Errorf("cookieName = %q, want %q", am.cookieName, "session")
	}
	if am.cookieDomain != "test.com" {
		t.Errorf("cookieDomain = %q, want %q", am.cookieDomain, "test.com")
	}
	if am.cookieSecure {
		t.Error("cookieSecure should be false")
	}
}
