package config

import (
	"net/http"
	"net/http/httptest"
	"net/url"
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
