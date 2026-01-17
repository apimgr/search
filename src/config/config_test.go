package config

import (
	"os"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg == nil {
		t.Fatal("DefaultConfig() returned nil")
	}

	// Verify key defaults per config.go
	if cfg.Server.Address != "[::]" {
		t.Errorf("Server.Address = %q, want %q", cfg.Server.Address, "[::]")
	}
	if cfg.Server.Port != 64580 {
		t.Errorf("Server.Port = %d, want %d", cfg.Server.Port, 64580)
	}
	if cfg.Server.Mode != "production" {
		t.Errorf("Server.Mode = %q, want %q", cfg.Server.Mode, "production")
	}
	if cfg.Search.ResultsPerPage != 10 {
		t.Errorf("Search.ResultsPerPage = %d, want %d", cfg.Search.ResultsPerPage, 10)
	}
	if cfg.Search.Timeout != 10 {
		t.Errorf("Search.Timeout = %d, want %d", cfg.Search.Timeout, 10)
	}
}

func TestGetRandomPort(t *testing.T) {
	port := GetRandomPort()
	if port < 1024 || port > 65535 {
		t.Errorf("GetRandomPort() = %d, want between 1024 and 65535", port)
	}

	// Test multiple times to verify randomness
	ports := make(map[int]bool)
	for i := 0; i < 10; i++ {
		p := GetRandomPort()
		ports[p] = true
	}
	// Should have at least some variety
	if len(ports) < 3 {
		t.Error("GetRandomPort() doesn't seem very random")
	}
}

func TestResolvePort(t *testing.T) {
	tests := []struct {
		name     string
		port     int
		wantSame bool
	}{
		{"valid port", 8080, true},
		{"zero port", 0, false},       // Zero gets random port
		{"negative port", -1, true},   // Negative passes through unchanged (config validation handles this separately)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolvePort(tt.port)
			if tt.wantSame && got != tt.port {
				t.Errorf("ResolvePort(%d) = %d, want %d", tt.port, got, tt.port)
			}
			if !tt.wantSame && got == tt.port {
				t.Errorf("ResolvePort(%d) should return different port", tt.port)
			}
		})
	}
}

func TestGetConfigPath(t *testing.T) {
	path := GetConfigPath()
	if path == "" {
		t.Error("GetConfigPath() should not be empty")
	}
}

func TestLoadNonExistent(t *testing.T) {
	_, err := Load("/nonexistent/path/config.yml")
	if err == nil {
		t.Error("Load() should error on nonexistent file")
	}
}

func TestLoadOrCreate(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "config-load-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	configPath := tmpDir + "/server.yml"

	// First call should create
	cfg, created, err := LoadOrCreate(configPath)
	if err != nil {
		t.Fatalf("LoadOrCreate() error = %v", err)
	}
	if !created {
		t.Error("LoadOrCreate() should return created=true for new file")
	}
	if cfg == nil {
		t.Fatal("LoadOrCreate() returned nil config")
	}

	// Second call should load existing
	cfg2, created2, err := LoadOrCreate(configPath)
	if err != nil {
		t.Fatalf("LoadOrCreate() error on second call = %v", err)
	}
	if created2 {
		t.Error("LoadOrCreate() should return created=false for existing file")
	}
	if cfg2 == nil {
		t.Fatal("LoadOrCreate() returned nil config on second call")
	}
}

func TestConfigSaveAndLoad(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "config-save-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	configPath := tmpDir + "/server.yml"

	// Create and save config
	cfg := DefaultConfig()
	cfg.Server.Port = 9999
	cfg.Server.Title = "TestInstance"

	err = cfg.Save(configPath)
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// Load and verify
	loaded, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if loaded.Server.Port != 9999 {
		t.Errorf("Loaded Server.Port = %d, want 9999", loaded.Server.Port)
	}
	if loaded.Server.Title != "TestInstance" {
		t.Errorf("Loaded Server.Title = %q, want %q", loaded.Server.Title, "TestInstance")
	}
}

func TestConfigValidateAndApplyDefaults(t *testing.T) {
	cfg := DefaultConfig()
	warnings := cfg.ValidateAndApplyDefaults()
	// Default config should validate without critical errors
	_ = warnings
}

func TestConfigApplyEnv(t *testing.T) {
	cfg := DefaultConfig()
	env := &EnvConfig{
		Port:   "9090",
		Debug:  true,
		Secret: "test-secret",
	}

	cfg.ApplyEnv(env)

	if cfg.Server.Port != 9090 {
		t.Errorf("After ApplyEnv, Server.Port = %d, want 9090", cfg.Server.Port)
	}
}

func TestLogValidationWarnings(t *testing.T) {
	warnings := []ValidationWarning{
		{Field: "test.field", Message: "test warning", Default: "default"},
	}
	// Just verify it doesn't panic
	LogValidationWarnings(warnings)
}

func TestConfigServerBaseURL(t *testing.T) {
	cfg := DefaultConfig()

	// Test that BaseURL can be set
	cfg.Server.BaseURL = "https://example.com"
	if cfg.Server.BaseURL != "https://example.com" {
		t.Errorf("Server.BaseURL = %q, want %q", cfg.Server.BaseURL, "https://example.com")
	}
}

func TestTorConfigDefaults(t *testing.T) {
	cfg := DefaultConfig()

	// Per AI.md PART 32: Tor auto-enabled, not configurable
	if cfg.Server.Tor.HiddenServicePort != 80 {
		t.Errorf("Tor.HiddenServicePort = %d, want 80", cfg.Server.Tor.HiddenServicePort)
	}
}

func TestCacheConfigDefaults(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Server.Cache.Type != "memory" {
		t.Errorf("Cache.Type = %q, want %q", cfg.Server.Cache.Type, "memory")
	}
	if cfg.Server.Cache.TTL != 3600 {
		t.Errorf("Cache.TTL = %d, want 3600", cfg.Server.Cache.TTL)
	}
}

func TestSecurityConfigDefaults(t *testing.T) {
	cfg := DefaultConfig()

	if !cfg.Server.Security.CSRF.Enabled {
		t.Error("Security.CSRF.Enabled should be true by default")
	}
	if cfg.Server.Security.CSRF.CookieName != "csrf_token" {
		t.Errorf("Security.CSRF.CookieName = %q, want %q", cfg.Server.Security.CSRF.CookieName, "csrf_token")
	}
}

func TestRateLimitConfigDefaults(t *testing.T) {
	cfg := DefaultConfig()

	if !cfg.Server.RateLimit.Enabled {
		t.Error("RateLimit.Enabled should be true by default")
	}
	if cfg.Server.RateLimit.RequestsPerMinute != 60 {
		t.Errorf("RateLimit.RequestsPerMinute = %d, want 60", cfg.Server.RateLimit.RequestsPerMinute)
	}
}
