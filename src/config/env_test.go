package config

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
)

func TestLoadFromEnv(t *testing.T) {
	// Test that LoadFromEnv returns a valid config
	cfg := LoadFromEnv()
	if cfg == nil {
		t.Fatal("LoadFromEnv() returned nil")
	}
	// Verify config structure is properly initialized
	// The function returns a valid EnvConfig pointer
}

func TestTrimmedFormValue(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{"normal value", "test", "test"},
		{"leading spaces", "  test", "test"},
		{"trailing spaces", "test  ", "test"},
		{"both spaces", "  test  ", "test"},
		{"empty", "", ""},
		{"only spaces", "   ", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			form := url.Values{}
			form.Set("key", tt.value)
			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(form.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			req.ParseForm()

			got := TrimmedFormValue(req, "key")
			if got != tt.want {
				t.Errorf("TrimmedFormValue() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTrimmedPostFormValue(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{"normal value", "test", "test"},
		{"leading spaces", "  test", "test"},
		{"trailing spaces", "test  ", "test"},
		{"both spaces", "  test  ", "test"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			form := url.Values{}
			form.Set("key", tt.value)
			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(form.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			req.ParseForm()

			got := TrimmedPostFormValue(req, "key")
			if got != tt.want {
				t.Errorf("TrimmedPostFormValue() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseFormBool(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"empty", "", false},
		{"1", "1", true},
		{"true", "true", true},
		{"yes", "yes", true},
		{"on", "on", true},
		{"checked", "checked", true},
		{"0", "0", false},
		{"false", "false", false},
		{"no", "no", false},
		{"off", "off", false},
		{"TRUE", "TRUE", true},
		{"ON", "ON", true},
		{"with spaces", "  on  ", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseFormBool(tt.input)
			if got != tt.want {
				t.Errorf("ParseFormBool(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseBoolDefault(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		defaultVal bool
		want       bool
	}{
		{"true", "true", false, true},
		{"false", "false", true, false},
		{"empty default true", "", true, true},
		{"empty default false", "", false, false},
		// Note: Invalid values return false per ParseBool implementation
		{"invalid returns false", "invalid", true, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseBoolDefault(tt.input, tt.defaultVal)
			if got != tt.want {
				t.Errorf("ParseBoolDefault(%q, %v) = %v, want %v", tt.input, tt.defaultVal, got, tt.want)
			}
		})
	}
}

func TestParseInt(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		defaultVal int
		want       int
	}{
		{"valid positive", "42", 0, 42},
		{"valid zero", "0", 10, 0},
		{"valid negative", "-5", 0, -5},
		{"empty", "", 10, 10},
		{"invalid", "abc", 10, 10},
		{"with spaces", "  42  ", 0, 42},
		{"float truncates", "3.14", 0, 0}, // strconv.Atoi fails on floats
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseInt(tt.input, tt.defaultVal)
			if got != tt.want {
				t.Errorf("ParseInt(%q, %d) = %d, want %d", tt.input, tt.defaultVal, got, tt.want)
			}
		})
	}
}

func TestParseDuration(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    int
		wantErr bool
	}{
		{"empty", "", 0, false},
		{"seconds", "30s", 30, false},
		{"minutes", "5m", 300, false},
		{"hours", "2h", 7200, false},
		{"days", "1d", 86400, false},
		{"weeks", "1w", 604800, false},
		{"no unit", "60", 60, false},
		{"invalid", "abc", 0, true},
		{"uppercase S", "30S", 30, false},
		{"uppercase M", "5M", 300, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseDuration(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseDuration(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ParseDuration(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestEnvConfigGetMode(t *testing.T) {
	tests := []struct {
		name string
		mode string
		want string
	}{
		{"development", "development", "development"},
		{"dev", "dev", "development"},
		{"production", "production", "production"},
		{"prod", "prod", "production"},
		{"empty", "", "production"},
		{"invalid", "staging", "production"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &EnvConfig{Mode: tt.mode}
			got := cfg.GetMode()
			if got != tt.want {
				t.Errorf("GetMode() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestEnvConfigIsDevelopment(t *testing.T) {
	tests := []struct {
		name string
		mode string
		want bool
	}{
		{"development", "development", true},
		{"dev", "dev", true},
		{"production", "production", false},
		{"prod", "prod", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &EnvConfig{Mode: tt.mode}
			got := cfg.IsDevelopment()
			if got != tt.want {
				t.Errorf("IsDevelopment() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEnvConfigIsProduction(t *testing.T) {
	tests := []struct {
		name string
		mode string
		want bool
	}{
		{"development", "development", false},
		{"dev", "dev", false},
		{"production", "production", true},
		{"prod", "prod", true},
		{"empty defaults to production", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &EnvConfig{Mode: tt.mode}
			got := cfg.IsProduction()
			if got != tt.want {
				t.Errorf("IsProduction() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsTorAvailable(t *testing.T) {
	// This just tests that the function doesn't panic
	// Actual availability depends on system
	_ = IsTorAvailable()
}

func TestGetDomain(t *testing.T) {
	// Test returns empty when DOMAIN not set
	// (don't modify env in tests to avoid side effects)
	domain := GetDomain()
	// Just verify it doesn't panic and returns a string
	_ = domain
}

func TestGetAllDomains(t *testing.T) {
	// Test returns nil when DOMAIN not set
	domains := GetAllDomains()
	// Just verify it doesn't panic
	_ = domains
}

func TestGetDatabaseDriver(t *testing.T) {
	driver := GetDatabaseDriver()
	// Just verify it doesn't panic
	_ = driver
}

func TestGetDatabaseURL(t *testing.T) {
	url := GetDatabaseURL()
	// Just verify it doesn't panic
	_ = url
}

func TestGetEnvWithFallback(t *testing.T) {
	// Save and restore env vars
	original1 := os.Getenv("TEST_ENV_1")
	original2 := os.Getenv("TEST_ENV_2")
	defer func() {
		os.Setenv("TEST_ENV_1", original1)
		os.Setenv("TEST_ENV_2", original2)
	}()

	// Test first key present
	os.Setenv("TEST_ENV_1", "value1")
	os.Setenv("TEST_ENV_2", "value2")
	got := getEnv("TEST_ENV_1", "TEST_ENV_2")
	if got != "value1" {
		t.Errorf("getEnv() = %q, want %q", got, "value1")
	}

	// Test fallback to second key
	os.Setenv("TEST_ENV_1", "")
	got = getEnv("TEST_ENV_1", "TEST_ENV_2")
	if got != "value2" {
		t.Errorf("getEnv() with fallback = %q, want %q", got, "value2")
	}

	// Test both empty
	os.Setenv("TEST_ENV_2", "")
	got = getEnv("TEST_ENV_1", "TEST_ENV_2")
	if got != "" {
		t.Errorf("getEnv() with both empty = %q, want empty", got)
	}

	// Test with no keys
	got = getEnv()
	if got != "" {
		t.Errorf("getEnv() with no keys = %q, want empty", got)
	}
}

func TestLoadFromEnvWithEnvVars(t *testing.T) {
	// Save and restore env vars
	envVars := []string{
		"SEARCH_SETTINGS_PATH", "SETTINGS_PATH",
		"DEBUG", "SEARCH_DEBUG",
		"SECRET_KEY", "SEARCH_SECRET",
		"BIND_ADDRESS", "SEARCH_BIND_ADDRESS",
		"INSTANCE_NAME", "APPLICATION_NAME",
		"AUTOCOMPLETE",
		"BASE_URL",
		"DOMAIN",
		"DATABASE_DRIVER",
		"DATABASE_URL",
		"IMAGE_PROXY_URL", "MORTY_URL",
		"IMAGE_PROXY_KEY", "MORTY_KEY",
		"SEARCH_PORT", "PORT",
		"SEARCH_MODE", "MODE",
		"SMTP_HOST", "SMTP_PORT", "SMTP_USERNAME", "SMTP_PASSWORD",
		"SMTP_TLS", "SMTP_FROM_NAME", "SMTP_FROM_EMAIL",
		"ENABLE_GOOGLE", "SEARCH_ENGINES_GOOGLE",
		"ENABLE_DUCKDUCKGO", "SEARCH_ENGINES_DUCKDUCKGO",
		"ENABLE_BING", "SEARCH_ENGINES_BING",
	}

	originalValues := make(map[string]string)
	for _, v := range envVars {
		originalValues[v] = os.Getenv(v)
	}
	defer func() {
		for k, v := range originalValues {
			os.Setenv(k, v)
		}
	}()

	// Clear all env vars
	for _, v := range envVars {
		os.Setenv(v, "")
	}

	// Set test values
	os.Setenv("SEARCH_SETTINGS_PATH", "/test/settings.yml")
	os.Setenv("DEBUG", "true")
	os.Setenv("SECRET_KEY", "test-secret")
	os.Setenv("INSTANCE_NAME", "TestInstance")
	os.Setenv("AUTOCOMPLETE", "google")
	os.Setenv("BASE_URL", "https://test.example.com")
	os.Setenv("DOMAIN", "example.com,www.example.com")
	os.Setenv("IMAGE_PROXY_URL", "https://proxy.example.com")
	os.Setenv("IMAGE_PROXY_KEY", "proxy-key")
	os.Setenv("SEARCH_PORT", "9090")
	os.Setenv("SEARCH_MODE", "development")
	os.Setenv("SMTP_HOST", "smtp.example.com")
	os.Setenv("SMTP_PORT", "587")
	os.Setenv("SMTP_USERNAME", "user")
	os.Setenv("SMTP_PASSWORD", "pass")
	os.Setenv("SMTP_TLS", "starttls")
	os.Setenv("SMTP_FROM_NAME", "TestApp")
	os.Setenv("SMTP_FROM_EMAIL", "noreply@example.com")
	os.Setenv("ENABLE_GOOGLE", "false")
	os.Setenv("ENABLE_DUCKDUCKGO", "false")
	os.Setenv("ENABLE_BING", "false")

	cfg := LoadFromEnv()

	// Verify values
	if cfg.SettingsPath != "/test/settings.yml" {
		t.Errorf("SettingsPath = %q, want %q", cfg.SettingsPath, "/test/settings.yml")
	}
	if !cfg.Debug {
		t.Error("Debug should be true")
	}
	if cfg.Secret != "test-secret" {
		t.Errorf("Secret = %q, want %q", cfg.Secret, "test-secret")
	}
	if cfg.InstanceName != "TestInstance" {
		t.Errorf("InstanceName = %q, want %q", cfg.InstanceName, "TestInstance")
	}
	if cfg.Autocomplete != "google" {
		t.Errorf("Autocomplete = %q, want %q", cfg.Autocomplete, "google")
	}
	if cfg.BaseURL != "https://test.example.com" {
		t.Errorf("BaseURL = %q, want %q", cfg.BaseURL, "https://test.example.com")
	}
	if cfg.Domain != "example.com,www.example.com" {
		t.Errorf("Domain = %q, want %q", cfg.Domain, "example.com,www.example.com")
	}
	if cfg.ImageProxyURL != "https://proxy.example.com" {
		t.Errorf("ImageProxyURL = %q, want %q", cfg.ImageProxyURL, "https://proxy.example.com")
	}
	if cfg.ImageProxyKey != "proxy-key" {
		t.Errorf("ImageProxyKey = %q, want %q", cfg.ImageProxyKey, "proxy-key")
	}
	if cfg.Port != "9090" {
		t.Errorf("Port = %q, want %q", cfg.Port, "9090")
	}
	if cfg.Mode != "development" {
		t.Errorf("Mode = %q, want %q", cfg.Mode, "development")
	}
	if cfg.SMTPHost != "smtp.example.com" {
		t.Errorf("SMTPHost = %q, want %q", cfg.SMTPHost, "smtp.example.com")
	}
	if cfg.SMTPPort != 587 {
		t.Errorf("SMTPPort = %d, want 587", cfg.SMTPPort)
	}
	if cfg.SMTPUsername != "user" {
		t.Errorf("SMTPUsername = %q, want %q", cfg.SMTPUsername, "user")
	}
	if cfg.SMTPPassword != "pass" {
		t.Errorf("SMTPPassword = %q, want %q", cfg.SMTPPassword, "pass")
	}
	if cfg.SMTPTLS != "starttls" {
		t.Errorf("SMTPTLS = %q, want %q", cfg.SMTPTLS, "starttls")
	}
	if cfg.SMTPFromName != "TestApp" {
		t.Errorf("SMTPFromName = %q, want %q", cfg.SMTPFromName, "TestApp")
	}
	if cfg.SMTPFromEmail != "noreply@example.com" {
		t.Errorf("SMTPFromEmail = %q, want %q", cfg.SMTPFromEmail, "noreply@example.com")
	}
	if cfg.EnableGoogle {
		t.Error("EnableGoogle should be false")
	}
	if cfg.EnableDuckDuckGo {
		t.Error("EnableDuckDuckGo should be false")
	}
	if cfg.EnableBing {
		t.Error("EnableBing should be false")
	}
}

func TestLoadFromEnvBindAddressParsing(t *testing.T) {
	// Save and restore env vars
	originalAddr := os.Getenv("BIND_ADDRESS")
	originalPort := os.Getenv("SEARCH_PORT")
	originalPort2 := os.Getenv("PORT")
	defer func() {
		os.Setenv("BIND_ADDRESS", originalAddr)
		os.Setenv("SEARCH_PORT", originalPort)
		os.Setenv("PORT", originalPort2)
	}()

	// Clear env vars
	os.Setenv("BIND_ADDRESS", "")
	os.Setenv("SEARCH_PORT", "")
	os.Setenv("PORT", "")

	// Test BIND_ADDRESS with port extraction
	os.Setenv("BIND_ADDRESS", "0.0.0.0:8080")
	cfg := LoadFromEnv()

	if cfg.BindAddress != "0.0.0.0:8080" {
		t.Errorf("BindAddress = %q, want %q", cfg.BindAddress, "0.0.0.0:8080")
	}
	if cfg.Port != "8080" {
		t.Errorf("Port extracted from BindAddress = %q, want %q", cfg.Port, "8080")
	}
}

func TestLoadFromEnvDebugToMode(t *testing.T) {
	// Save and restore env vars
	originalDebug := os.Getenv("DEBUG")
	originalMode := os.Getenv("SEARCH_MODE")
	defer func() {
		os.Setenv("DEBUG", originalDebug)
		os.Setenv("SEARCH_MODE", originalMode)
	}()

	// Clear env vars
	os.Setenv("DEBUG", "")
	os.Setenv("SEARCH_MODE", "")
	os.Setenv("MODE", "")

	// Test DEBUG=true sets mode to development
	os.Setenv("DEBUG", "true")
	cfg := LoadFromEnv()

	if cfg.Mode != "development" {
		t.Errorf("Mode with DEBUG=true = %q, want %q", cfg.Mode, "development")
	}
}

func TestGetDomainWithEnvVar(t *testing.T) {
	// Save and restore env var
	original := os.Getenv("DOMAIN")
	defer os.Setenv("DOMAIN", original)

	// Test single domain
	os.Setenv("DOMAIN", "example.com")
	got := GetDomain()
	if got != "example.com" {
		t.Errorf("GetDomain() = %q, want %q", got, "example.com")
	}

	// Test comma-separated (returns first)
	os.Setenv("DOMAIN", "primary.com, secondary.com, third.com")
	got = GetDomain()
	if got != "primary.com" {
		t.Errorf("GetDomain() with list = %q, want %q", got, "primary.com")
	}

	// Test with whitespace
	os.Setenv("DOMAIN", "  whitespace.com  ")
	got = GetDomain()
	if got != "whitespace.com" {
		t.Errorf("GetDomain() with whitespace = %q, want %q", got, "whitespace.com")
	}

	// Test empty
	os.Setenv("DOMAIN", "")
	got = GetDomain()
	if got != "" {
		t.Errorf("GetDomain() empty = %q, want empty", got)
	}
}

func TestGetAllDomainsWithEnvVar(t *testing.T) {
	// Save and restore env var
	original := os.Getenv("DOMAIN")
	defer os.Setenv("DOMAIN", original)

	// Test single domain
	os.Setenv("DOMAIN", "example.com")
	got := GetAllDomains()
	if len(got) != 1 || got[0] != "example.com" {
		t.Errorf("GetAllDomains() = %v, want [example.com]", got)
	}

	// Test comma-separated
	os.Setenv("DOMAIN", "first.com, second.com, third.com")
	got = GetAllDomains()
	if len(got) != 3 {
		t.Errorf("GetAllDomains() returned %d domains, want 3", len(got))
	}
	if got[0] != "first.com" || got[1] != "second.com" || got[2] != "third.com" {
		t.Errorf("GetAllDomains() = %v, want [first.com, second.com, third.com]", got)
	}

	// Test with empty entries
	os.Setenv("DOMAIN", "first.com,,third.com,")
	got = GetAllDomains()
	if len(got) != 2 {
		t.Errorf("GetAllDomains() with empty entries returned %d domains, want 2", len(got))
	}

	// Test empty
	os.Setenv("DOMAIN", "")
	got = GetAllDomains()
	if got != nil {
		t.Errorf("GetAllDomains() empty = %v, want nil", got)
	}
}

func TestGetDatabaseDriverWithEnvVar(t *testing.T) {
	// Save and restore env var
	original := os.Getenv("DATABASE_DRIVER")
	defer os.Setenv("DATABASE_DRIVER", original)

	os.Setenv("DATABASE_DRIVER", "postgres")
	got := GetDatabaseDriver()
	if got != "postgres" {
		t.Errorf("GetDatabaseDriver() = %q, want %q", got, "postgres")
	}

	os.Setenv("DATABASE_DRIVER", "")
	got = GetDatabaseDriver()
	if got != "" {
		t.Errorf("GetDatabaseDriver() empty = %q, want empty", got)
	}
}

func TestGetDatabaseURLWithEnvVar(t *testing.T) {
	// Save and restore env var
	original := os.Getenv("DATABASE_URL")
	defer os.Setenv("DATABASE_URL", original)

	os.Setenv("DATABASE_URL", "postgres://user:pass@localhost/db")
	got := GetDatabaseURL()
	if got != "postgres://user:pass@localhost/db" {
		t.Errorf("GetDatabaseURL() = %q, want %q", got, "postgres://user:pass@localhost/db")
	}

	os.Setenv("DATABASE_URL", "")
	got = GetDatabaseURL()
	if got != "" {
		t.Errorf("GetDatabaseURL() empty = %q, want empty", got)
	}
}

func TestParseDurationEdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    int
		wantErr bool
	}{
		{"single digit", "5", 5, false},
		{"uppercase unit", "30S", 30, false},
		{"uppercase H", "2H", 7200, false},
		{"uppercase D", "1D", 86400, false},
		{"uppercase W", "1W", 604800, false},
		{"unknown unit defaults to seconds", "30x", 30, false}, // 'x' not recognized, uses val as seconds
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseDuration(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseDuration(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ParseDuration(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseBoolInEnv(t *testing.T) {
	// Test the internal parseBool function through LoadFromEnv
	original := os.Getenv("DEBUG")
	defer os.Setenv("DEBUG", original)

	tests := []struct {
		name  string
		value string
		want  bool
	}{
		{"true", "true", true},
		{"1", "1", true},
		{"yes", "yes", true},
		{"on", "on", true},
		{"false", "false", false},
		{"0", "0", false},
		{"no", "no", false},
		{"empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Setenv("DEBUG", tt.value)
			cfg := LoadFromEnv()
			if cfg.Debug != tt.want {
				t.Errorf("LoadFromEnv() Debug with %q = %v, want %v", tt.value, cfg.Debug, tt.want)
			}
		})
	}
}

func TestParseIntEdgeCases(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		defaultVal int
		want       int
	}{
		{"large number", "999999", 0, 999999},
		{"negative", "-100", 0, -100},
		{"zero", "0", 99, 0},
		{"leading zeros", "007", 0, 7},
		{"hex not supported", "0x10", 0, 0}, // strconv.Atoi doesn't parse hex
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseInt(tt.input, tt.defaultVal)
			if got != tt.want {
				t.Errorf("ParseInt(%q, %d) = %d, want %d", tt.input, tt.defaultVal, got, tt.want)
			}
		})
	}
}
