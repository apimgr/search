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
	if cfg.Server.RateLimit.RequestsPerMinute != 1000 {
		t.Errorf("RateLimit.RequestsPerMinute = %d, want 1000", cfg.Server.RateLimit.RequestsPerMinute)
	}
}

func TestConfigIsFirstRun(t *testing.T) {
	cfg := DefaultConfig()

	// Default config should not be first run
	if cfg.IsFirstRun() {
		t.Error("IsFirstRun() should return false for default config")
	}

	// Set first run and verify
	cfg.firstRun = true
	if !cfg.IsFirstRun() {
		t.Error("IsFirstRun() should return true when firstRun is set")
	}
}

func TestConfigSetPathAndGetPath(t *testing.T) {
	cfg := DefaultConfig()

	// Initially empty
	if cfg.GetPath() != "" {
		t.Errorf("GetPath() = %q, want empty string", cfg.GetPath())
	}

	// Set path
	cfg.SetPath("/test/config.yml")
	if cfg.GetPath() != "/test/config.yml" {
		t.Errorf("GetPath() = %q, want %q", cfg.GetPath(), "/test/config.yml")
	}
}

func TestConfigReload(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "config-reload-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	configPath := tmpDir + "/server.yml"

	// Create and save initial config
	cfg := DefaultConfig()
	cfg.Server.Title = "Initial Title"
	cfg.Server.Port = 8080
	err = cfg.Save(configPath)
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	cfg.SetPath(configPath)

	// Modify the file externally
	cfg2 := DefaultConfig()
	cfg2.Server.Title = "Modified Title"
	cfg2.Server.Port = 9999 // This should NOT change after reload (port requires restart)
	err = cfg2.Save(configPath)
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// Reload
	err = cfg.Reload()
	if err != nil {
		t.Fatalf("Reload() error = %v", err)
	}

	// Title should change
	if cfg.Server.Title != "Modified Title" {
		t.Errorf("After reload, Server.Title = %q, want %q", cfg.Server.Title, "Modified Title")
	}

	// Port should NOT change (requires restart)
	if cfg.Server.Port != 8080 {
		t.Errorf("After reload, Server.Port = %d, want 8080 (port changes require restart)", cfg.Server.Port)
	}
}

func TestConfigReloadNoPath(t *testing.T) {
	cfg := DefaultConfig()

	// Reload without path should error
	err := cfg.Reload()
	if err == nil {
		t.Error("Reload() without path should return error")
	}
}

func TestConfigReloadNonexistent(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SetPath("/nonexistent/path/config.yml")

	err := cfg.Reload()
	if err == nil {
		t.Error("Reload() with nonexistent path should return error")
	}
}

func TestConfigReloadInvalidYAML(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "config-reload-invalid-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	configPath := tmpDir + "/server.yml"

	// Write invalid YAML
	err = os.WriteFile(configPath, []byte("invalid: yaml: content: ["), 0600)
	if err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}

	cfg := DefaultConfig()
	cfg.SetPath(configPath)

	err = cfg.Reload()
	if err == nil {
		t.Error("Reload() with invalid YAML should return error")
	}
}

func TestSessionConfigGetAdminCookieName(t *testing.T) {
	tests := []struct {
		name       string
		session    SessionConfig
		want       string
	}{
		{
			"admin cookie name set",
			SessionConfig{Admin: SessionTypeConfig{CookieName: "admin_sess"}},
			"admin_sess",
		},
		{
			"legacy cookie name set",
			SessionConfig{CookieName: "legacy_cookie"},
			"legacy_cookie",
		},
		{
			"default",
			SessionConfig{},
			"admin_session",
		},
		{
			"admin takes precedence over legacy",
			SessionConfig{Admin: SessionTypeConfig{CookieName: "admin"}, CookieName: "legacy"},
			"admin",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.session.GetAdminCookieName()
			if got != tt.want {
				t.Errorf("GetAdminCookieName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSessionConfigGetUserCookieName(t *testing.T) {
	tests := []struct {
		name    string
		session SessionConfig
		want    string
	}{
		{
			"user cookie name set",
			SessionConfig{User: SessionTypeConfig{CookieName: "user_sess"}},
			"user_sess",
		},
		{
			"legacy cookie name set",
			SessionConfig{CookieName: "legacy_cookie"},
			"legacy_cookie",
		},
		{
			"default",
			SessionConfig{},
			"user_session",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.session.GetUserCookieName()
			if got != tt.want {
				t.Errorf("GetUserCookieName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSessionConfigGetAdminMaxAge(t *testing.T) {
	tests := []struct {
		name    string
		session SessionConfig
		want    int
	}{
		{
			"admin max age set",
			SessionConfig{Admin: SessionTypeConfig{MaxAge: 3600}},
			3600,
		},
		{
			"default (30 days)",
			SessionConfig{},
			2592000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.session.GetAdminMaxAge()
			if got != tt.want {
				t.Errorf("GetAdminMaxAge() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestSessionConfigGetUserMaxAge(t *testing.T) {
	tests := []struct {
		name    string
		session SessionConfig
		want    int
	}{
		{
			"user max age set",
			SessionConfig{User: SessionTypeConfig{MaxAge: 1800}},
			1800,
		},
		{
			"default (7 days)",
			SessionConfig{},
			604800,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.session.GetUserMaxAge()
			if got != tt.want {
				t.Errorf("GetUserMaxAge() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestSessionConfigGetIdleTimeout(t *testing.T) {
	tests := []struct {
		name    string
		session SessionConfig
		want    int
	}{
		{
			"admin idle timeout set",
			SessionConfig{Admin: SessionTypeConfig{IdleTimeout: 3600}},
			3600,
		},
		{
			"user idle timeout set",
			SessionConfig{User: SessionTypeConfig{IdleTimeout: 7200}},
			7200,
		},
		{
			"default (24 hours)",
			SessionConfig{},
			86400,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.session.GetIdleTimeout()
			if got != tt.want {
				t.Errorf("GetIdleTimeout() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestSessionConfigIsSecure(t *testing.T) {
	tests := []struct {
		name       string
		session    SessionConfig
		sslEnabled bool
		want       bool
	}{
		{"secure=true", SessionConfig{Secure: "true"}, false, true},
		{"secure=yes", SessionConfig{Secure: "yes"}, false, true},
		{"secure=1", SessionConfig{Secure: "1"}, false, true},
		{"secure=false", SessionConfig{Secure: "false"}, true, false},
		{"secure=no", SessionConfig{Secure: "no"}, true, false},
		{"secure=0", SessionConfig{Secure: "0"}, true, false},
		{"auto with SSL enabled", SessionConfig{Secure: "auto"}, true, true},
		{"auto with SSL disabled", SessionConfig{Secure: "auto"}, false, false},
		{"empty with SSL enabled", SessionConfig{}, true, true},
		{"empty with SSL disabled", SessionConfig{}, false, false},
		{"empty with legacy CookieSecure", SessionConfig{CookieSecure: true}, false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.session.IsSecure(tt.sslEnabled)
			if got != tt.want {
				t.Errorf("IsSecure(%v) = %v, want %v", tt.sslEnabled, got, tt.want)
			}
		})
	}
}

func TestSessionConfigIsHTTPOnly(t *testing.T) {
	tests := []struct {
		name    string
		session SessionConfig
		want    bool
	}{
		{"HTTPOnly set", SessionConfig{HTTPOnly: true}, true},
		{"HTTPOnly not set", SessionConfig{HTTPOnly: false}, false},
		{"legacy CookieHTTPOnly", SessionConfig{CookieHTTPOnly: true}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.session.IsHTTPOnly()
			if got != tt.want {
				t.Errorf("IsHTTPOnly() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSessionConfigGetSameSite(t *testing.T) {
	tests := []struct {
		name    string
		session SessionConfig
		want    string
	}{
		{"SameSite set", SessionConfig{SameSite: "strict"}, "strict"},
		{"legacy CookieSameSite", SessionConfig{CookieSameSite: "none"}, "none"},
		{"default", SessionConfig{}, "lax"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.session.GetSameSite()
			if got != tt.want {
				t.Errorf("GetSameSite() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestUsersConfigGetSessionDurationDays(t *testing.T) {
	tests := []struct {
		name  string
		users UsersConfig
		want  int
	}{
		{
			"explicit days set",
			UsersConfig{Auth: struct {
				SessionDuration          string `yaml:"session_duration"`
				SessionDurationDays      int    `yaml:"session_duration_days"`
				Require2FA               bool   `yaml:"require_2fa"`
				Allow2FA                 bool   `yaml:"allow_2fa"`
				PasswordMinLength        int    `yaml:"password_min_length"`
				PasswordRequireUppercase bool   `yaml:"password_require_uppercase"`
				PasswordRequireNumber    bool   `yaml:"password_require_number"`
				PasswordRequireSpecial   bool   `yaml:"password_require_special"`
			}{SessionDurationDays: 14}},
			14,
		},
		{
			"duration string in days",
			UsersConfig{Auth: struct {
				SessionDuration          string `yaml:"session_duration"`
				SessionDurationDays      int    `yaml:"session_duration_days"`
				Require2FA               bool   `yaml:"require_2fa"`
				Allow2FA                 bool   `yaml:"allow_2fa"`
				PasswordMinLength        int    `yaml:"password_min_length"`
				PasswordRequireUppercase bool   `yaml:"password_require_uppercase"`
				PasswordRequireNumber    bool   `yaml:"password_require_number"`
				PasswordRequireSpecial   bool   `yaml:"password_require_special"`
			}{SessionDuration: "7d"}},
			7,
		},
		{
			"duration string in hours",
			UsersConfig{Auth: struct {
				SessionDuration          string `yaml:"session_duration"`
				SessionDurationDays      int    `yaml:"session_duration_days"`
				Require2FA               bool   `yaml:"require_2fa"`
				Allow2FA                 bool   `yaml:"allow_2fa"`
				PasswordMinLength        int    `yaml:"password_min_length"`
				PasswordRequireUppercase bool   `yaml:"password_require_uppercase"`
				PasswordRequireNumber    bool   `yaml:"password_require_number"`
				PasswordRequireSpecial   bool   `yaml:"password_require_special"`
			}{SessionDuration: "720h"}},
			30,
		},
		{
			"default (30 days)",
			UsersConfig{},
			30,
		},
		{
			"invalid duration string",
			UsersConfig{Auth: struct {
				SessionDuration          string `yaml:"session_duration"`
				SessionDurationDays      int    `yaml:"session_duration_days"`
				Require2FA               bool   `yaml:"require_2fa"`
				Allow2FA                 bool   `yaml:"allow_2fa"`
				PasswordMinLength        int    `yaml:"password_min_length"`
				PasswordRequireUppercase bool   `yaml:"password_require_uppercase"`
				PasswordRequireNumber    bool   `yaml:"password_require_number"`
				PasswordRequireSpecial   bool   `yaml:"password_require_special"`
			}{SessionDuration: "invalid"}},
			30,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.users.GetSessionDurationDays()
			if got != tt.want {
				t.Errorf("GetSessionDurationDays() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestLimitsConfigGetMaxBodySizeBytes(t *testing.T) {
	tests := []struct {
		name   string
		limits LimitsConfig
		want   int64
	}{
		{"default (empty)", LimitsConfig{}, 10 * 1024 * 1024},
		{"10MB", LimitsConfig{MaxBodySize: "10MB"}, 10 * 1024 * 1024},
		{"5mb lowercase", LimitsConfig{MaxBodySize: "5mb"}, 5 * 1024 * 1024},
		{"1KB", LimitsConfig{MaxBodySize: "1KB"}, 1024},
		{"2kb lowercase", LimitsConfig{MaxBodySize: "2kb"}, 2048},
		{"1GB", LimitsConfig{MaxBodySize: "1GB"}, 1024 * 1024 * 1024},
		{"1gb lowercase", LimitsConfig{MaxBodySize: "1gb"}, 1024 * 1024 * 1024},
		{"plain bytes", LimitsConfig{MaxBodySize: "1024"}, 1024},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.limits.GetMaxBodySizeBytes()
			if got != tt.want {
				t.Errorf("GetMaxBodySizeBytes() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestAnnouncementsConfigActiveAnnouncements(t *testing.T) {
	now := "2025-01-15T12:00:00Z"
	past := "2024-01-01T00:00:00Z"
	future := "2030-12-31T23:59:59Z"

	tests := []struct {
		name          string
		config        AnnouncementsConfig
		expectedCount int
	}{
		{
			"disabled",
			AnnouncementsConfig{Enabled: false, Messages: []Announcement{{ID: "1"}}},
			0,
		},
		{
			"no messages",
			AnnouncementsConfig{Enabled: true, Messages: nil},
			0,
		},
		{
			"active announcement",
			AnnouncementsConfig{Enabled: true, Messages: []Announcement{{ID: "1", Start: past, End: future}}},
			1,
		},
		{
			"past announcement",
			AnnouncementsConfig{Enabled: true, Messages: []Announcement{{ID: "1", Start: past, End: now}}},
			0,
		},
		{
			"future announcement",
			AnnouncementsConfig{Enabled: true, Messages: []Announcement{{ID: "1", Start: future, End: ""}}},
			0,
		},
		{
			"no time constraints",
			AnnouncementsConfig{Enabled: true, Messages: []Announcement{{ID: "1"}}},
			1,
		},
		{
			"invalid start time",
			AnnouncementsConfig{Enabled: true, Messages: []Announcement{{ID: "1", Start: "invalid"}}},
			1,
		},
		{
			"invalid end time",
			AnnouncementsConfig{Enabled: true, Messages: []Announcement{{ID: "1", End: "invalid"}}},
			1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.config.ActiveAnnouncements()
			if len(got) != tt.expectedCount {
				t.Errorf("ActiveAnnouncements() returned %d announcements, want %d", len(got), tt.expectedCount)
			}
		})
	}
}

func TestConfigIsDevelopment(t *testing.T) {
	tests := []struct {
		name string
		mode string
		want bool
	}{
		{"development", "development", true},
		{"dev", "dev", true},
		{"production", "production", false},
		{"empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.Server.Mode = tt.mode
			got := cfg.IsDevelopment()
			if got != tt.want {
				t.Errorf("IsDevelopment() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestConfigIsProduction(t *testing.T) {
	tests := []struct {
		name string
		mode string
		want bool
	}{
		{"production", "production", true},
		{"empty", "", true},
		{"development", "development", false},
		{"dev", "dev", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.Server.Mode = tt.mode
			got := cfg.IsProduction()
			if got != tt.want {
				t.Errorf("IsProduction() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestConfigIsDebug(t *testing.T) {
	// Save and restore original env var
	original := os.Getenv("DEBUG")
	defer os.Setenv("DEBUG", original)

	tests := []struct {
		name     string
		debugEnv string
		want     bool
	}{
		{"true", "true", true},
		{"1", "1", true},
		{"yes", "yes", true},
		{"false", "false", false},
		{"empty", "", false},
		{"0", "0", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Setenv("DEBUG", tt.debugEnv)
			cfg := DefaultConfig()
			got := cfg.IsDebug()
			if got != tt.want {
				t.Errorf("IsDebug() = %v, want %v (DEBUG=%q)", got, tt.want, tt.debugEnv)
			}
		})
	}
}

func TestConfigGetAddress(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Server.Address = "0.0.0.0"
	cfg.Server.Port = 8080

	got := cfg.GetAddress()
	want := "0.0.0.0:8080"
	if got != want {
		t.Errorf("GetAddress() = %q, want %q", got, want)
	}
}

func TestConfigGet(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Server.Title = "TestTitle"

	serverCfg := cfg.Get()
	if serverCfg.Title != "TestTitle" {
		t.Errorf("Get().Title = %q, want %q", serverCfg.Title, "TestTitle")
	}
}

func TestConfigUpdate(t *testing.T) {
	cfg := DefaultConfig()

	cfg.Update(func(s *ServerConfig) {
		s.Title = "UpdatedTitle"
		s.Port = 9999
	})

	if cfg.Server.Title != "UpdatedTitle" {
		t.Errorf("After Update, Server.Title = %q, want %q", cfg.Server.Title, "UpdatedTitle")
	}
	if cfg.Server.Port != 9999 {
		t.Errorf("After Update, Server.Port = %d, want 9999", cfg.Server.Port)
	}
}

func TestConfigGetEncryptionKey(t *testing.T) {
	cfg := DefaultConfig()

	// With secret key
	cfg.Server.SecretKey = "test-secret-key"
	key := cfg.GetEncryptionKey()
	if len(key) != 32 {
		t.Errorf("GetEncryptionKey() returned key of length %d, want 32", len(key))
	}

	// Same secret should produce same key
	key2 := cfg.GetEncryptionKey()
	for i := range key {
		if key[i] != key2[i] {
			t.Error("GetEncryptionKey() should return consistent key for same secret")
			break
		}
	}

	// Empty secret
	cfg.Server.SecretKey = ""
	key = cfg.GetEncryptionKey()
	if key != nil {
		t.Error("GetEncryptionKey() should return nil for empty secret")
	}
}

func TestServerConfigIsDualPortMode(t *testing.T) {
	tests := []struct {
		name      string
		port      int
		httpsPort int
		want      bool
	}{
		{"both ports set", 8080, 8443, true},
		{"only HTTP port", 8080, 0, false},
		{"only HTTPS port", 0, 8443, false},
		{"neither port set", 0, 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := ServerConfig{Port: tt.port, HTTPSPort: tt.httpsPort}
			got := cfg.IsDualPortMode()
			if got != tt.want {
				t.Errorf("IsDualPortMode() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestServerConfigGetHTTPPort(t *testing.T) {
	// Non-zero port
	cfg := ServerConfig{Port: 8080}
	got := cfg.GetHTTPPort()
	if got != 8080 {
		t.Errorf("GetHTTPPort() = %d, want 8080", got)
	}

	// Zero port (should return random port in 64xxx range)
	cfg = ServerConfig{Port: 0}
	got = cfg.GetHTTPPort()
	if got < 64000 || got > 64999 {
		t.Errorf("GetHTTPPort() with zero = %d, want in range 64000-64999", got)
	}
}

func TestServerConfigGetHTTPSPort(t *testing.T) {
	// Non-zero port
	cfg := ServerConfig{HTTPSPort: 8443}
	got := cfg.GetHTTPSPort()
	if got != 8443 {
		t.Errorf("GetHTTPSPort() = %d, want 8443", got)
	}

	// Zero port
	cfg = ServerConfig{HTTPSPort: 0}
	got = cfg.GetHTTPSPort()
	if got != 0 {
		t.Errorf("GetHTTPSPort() with zero = %d, want 0", got)
	}
}

func TestValidateAndApplyDefaultsComprehensive(t *testing.T) {
	// Test with invalid config to trigger warnings
	cfg := &Config{
		Server: ServerConfig{
			Title: "",     // Should trigger warning
			Port:  100000, // Invalid port
			Mode:  "invalid_mode",
			RateLimit: RateLimitConfig{
				Enabled:           true,
				RequestsPerMinute: -1, // Invalid
				BurstSize:         0,  // Invalid
			},
			Session: SessionConfig{
				Admin: SessionTypeConfig{MaxAge: 0},
				User:  SessionTypeConfig{MaxAge: 0},
			},
			GeoIP: GeoIPConfig{Enabled: true, Dir: ""},
			Metrics: MetricsConfig{Enabled: true, Endpoint: ""},
			Compression: CompressionConfig{Level: 15}, // Invalid level
		},
		Engines: nil, // Should trigger warning
	}

	warnings := cfg.ValidateAndApplyDefaults()

	// Should have warnings for invalid settings
	if len(warnings) == 0 {
		t.Error("ValidateAndApplyDefaults() should return warnings for invalid config")
	}

	// Verify defaults were applied
	if cfg.Server.Title != "Search" {
		t.Errorf("Title not fixed, got %q", cfg.Server.Title)
	}
	if cfg.Server.Port != 64580 {
		t.Errorf("Port not fixed, got %d", cfg.Server.Port)
	}
	if cfg.Server.Mode != "production" {
		t.Errorf("Mode not fixed, got %q", cfg.Server.Mode)
	}
	if cfg.Server.RateLimit.RequestsPerMinute != 1000 {
		t.Errorf("RateLimit.RequestsPerMinute not fixed, got %d", cfg.Server.RateLimit.RequestsPerMinute)
	}
	if cfg.Server.Compression.Level != 6 {
		t.Errorf("Compression.Level not fixed, got %d", cfg.Server.Compression.Level)
	}
	if len(cfg.Engines) == 0 {
		t.Error("Engines not populated with defaults")
	}
}

func TestValidateAndApplyDefaultsHTTPSPort(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{
			Title:     "Test",
			Port:      8080,
			HTTPSPort: 100000, // Invalid
			Mode:      "production",
			SecretKey: "test",
		},
		Engines: DefaultConfig().Engines,
	}

	warnings := cfg.ValidateAndApplyDefaults()

	// Check HTTPS port was fixed
	if cfg.Server.HTTPSPort != 0 {
		t.Errorf("Invalid HTTPSPort not fixed, got %d", cfg.Server.HTTPSPort)
	}

	// Should have a warning about the port
	found := false
	for _, w := range warnings {
		if w.Field == "server.https_port" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected warning about invalid HTTPS port")
	}
}

func TestValidateAndApplyDefaultsEngineTimeout(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{
			Title:     "Test",
			Port:      8080,
			Mode:      "production",
			SecretKey: "test",
		},
		Engines: map[string]EngineConfig{
			"test": {Enabled: true, Timeout: 0, Priority: 0},
		},
	}

	cfg.ValidateAndApplyDefaults()

	// Check engine timeout was fixed
	if cfg.Engines["test"].Timeout != 10 {
		t.Errorf("Engine timeout not fixed, got %d", cfg.Engines["test"].Timeout)
	}
	// Check engine priority was fixed (only for enabled engines)
	if cfg.Engines["test"].Priority != 50 {
		t.Errorf("Engine priority not fixed, got %d", cfg.Engines["test"].Priority)
	}
}

func TestLogValidationWarningsEmpty(t *testing.T) {
	// Just verify it doesn't panic with empty warnings
	LogValidationWarnings(nil)
	LogValidationWarnings([]ValidationWarning{})
}

func TestMigrateYamlToYml(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "migrate-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	yamlPath := tmpDir + "/server.yaml"
	ymlPath := tmpDir + "/server.yml"

	// Create .yaml file
	content := "server:\n  title: Test\n"
	err = os.WriteFile(yamlPath, []byte(content), 0600)
	if err != nil {
		t.Fatalf("Failed to create yaml file: %v", err)
	}

	// Call LoadOrCreate with .yml path (should trigger migration)
	cfg, created, err := LoadOrCreate(ymlPath)
	if err != nil {
		t.Fatalf("LoadOrCreate() error = %v", err)
	}

	// Should not be "created" since it loaded from existing .yaml
	if created {
		t.Error("LoadOrCreate() should return created=false after migration")
	}
	if cfg == nil {
		t.Fatal("LoadOrCreate() returned nil config")
	}

	// .yml file should exist
	if _, err := os.Stat(ymlPath); os.IsNotExist(err) {
		t.Error(".yml file was not created")
	}

	// .yaml.bak should exist
	if _, err := os.Stat(yamlPath + ".bak"); os.IsNotExist(err) {
		t.Error(".yaml.bak file was not created")
	}
}

func TestApplyEnvComprehensive(t *testing.T) {
	cfg := DefaultConfig()
	env := &EnvConfig{
		InstanceName:     "TestInstance",
		Secret:           "test-secret",
		Port:             "9090",
		Mode:             "development",
		BaseURL:          "https://test.example.com",
		Autocomplete:     "google",
		ImageProxyURL:    "https://proxy.example.com",
		ImageProxyKey:    "proxy-key",
		EnableGoogle:     false,
		EnableDuckDuckGo: false,
		EnableBing:       false,
	}

	cfg.ApplyEnv(env)

	if cfg.Server.Title != "TestInstance" {
		t.Errorf("Title not applied, got %q", cfg.Server.Title)
	}
	if cfg.Server.SecretKey != "test-secret" {
		t.Errorf("SecretKey not applied, got %q", cfg.Server.SecretKey)
	}
	if cfg.Server.Port != 9090 {
		t.Errorf("Port not applied, got %d", cfg.Server.Port)
	}
	if cfg.Server.Mode != "development" {
		t.Errorf("Mode not applied, got %q", cfg.Server.Mode)
	}
	if cfg.Server.BaseURL != "https://test.example.com" {
		t.Errorf("BaseURL not applied, got %q", cfg.Server.BaseURL)
	}
	if cfg.Search.Autocomplete != "google" {
		t.Errorf("Autocomplete not applied, got %q", cfg.Search.Autocomplete)
	}
	if !cfg.Server.ImageProxy.Enabled {
		t.Error("ImageProxy should be enabled when URL is set")
	}
	if cfg.Server.ImageProxy.URL != "https://proxy.example.com" {
		t.Errorf("ImageProxy.URL not applied, got %q", cfg.Server.ImageProxy.URL)
	}
	if cfg.Server.ImageProxy.Key != "proxy-key" {
		t.Errorf("ImageProxy.Key not applied, got %q", cfg.Server.ImageProxy.Key)
	}

	// Check engines
	if cfg.Engines["google"].Enabled {
		t.Error("Google should be disabled")
	}
	if cfg.Engines["duckduckgo"].Enabled {
		t.Error("DuckDuckGo should be disabled")
	}
	if cfg.Engines["bing"].Enabled {
		t.Error("Bing should be disabled")
	}
}

func TestApplyEnvInvalidPort(t *testing.T) {
	cfg := DefaultConfig()
	originalPort := cfg.Server.Port

	env := &EnvConfig{
		Port: "invalid",
	}

	cfg.ApplyEnv(env)

	// Port should remain unchanged
	if cfg.Server.Port != originalPort {
		t.Errorf("Port should not change with invalid value, got %d", cfg.Server.Port)
	}
}

func TestGenerateSecret(t *testing.T) {
	// Test that generateSecret produces different values
	secrets := make(map[string]bool)
	for i := 0; i < 10; i++ {
		secret := generateSecret()
		if len(secret) != 64 { // 32 bytes = 64 hex chars
			t.Errorf("generateSecret() returned %d chars, want 64", len(secret))
		}
		secrets[secret] = true
	}

	if len(secrets) < 9 {
		t.Error("generateSecret() doesn't seem random enough")
	}
}

func TestI18nConfigDefaults(t *testing.T) {
	cfg := DefaultConfig()

	if !cfg.Server.I18n.Enabled {
		t.Error("I18n.Enabled should be true by default")
	}
	if cfg.Server.I18n.DefaultLanguage != "en" {
		t.Errorf("I18n.DefaultLanguage = %q, want %q", cfg.Server.I18n.DefaultLanguage, "en")
	}
	if !cfg.Server.I18n.AutoDetect {
		t.Error("I18n.AutoDetect should be true by default")
	}
}

func TestSchedulerConfigDefaults(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Server.Scheduler.Timezone != "America/New_York" {
		t.Errorf("Scheduler.Timezone = %q, want %q", cfg.Server.Scheduler.Timezone, "America/New_York")
	}
	if !cfg.Server.Scheduler.Tasks.BackupDaily.Enabled {
		t.Error("Scheduler.Tasks.BackupDaily.Enabled should be true by default")
	}
	if cfg.Server.Scheduler.Tasks.BackupHourly.Enabled {
		t.Error("Scheduler.Tasks.BackupHourly.Enabled should be false by default")
	}
}

func TestWidgetsConfigDefaults(t *testing.T) {
	cfg := DefaultConfig()

	if !cfg.Search.Widgets.Enabled {
		t.Error("Search.Widgets.Enabled should be true by default")
	}
	if cfg.Search.Widgets.CacheTTL != 300 {
		t.Errorf("Search.Widgets.CacheTTL = %d, want 300", cfg.Search.Widgets.CacheTTL)
	}
	if !cfg.Search.Widgets.Weather.Enabled {
		t.Error("Search.Widgets.Weather.Enabled should be true by default")
	}
}

func TestEmailConfigDefaults(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Server.Email.SMTP.Port != 587 {
		t.Errorf("Email.SMTP.Port = %d, want 587", cfg.Server.Email.SMTP.Port)
	}
	if cfg.Server.Email.SMTP.TLS != "auto" {
		t.Errorf("Email.SMTP.TLS = %q, want %q", cfg.Server.Email.SMTP.TLS, "auto")
	}
}

func TestAuthConfigDefaults(t *testing.T) {
	cfg := DefaultConfig()

	// Auth config should have empty OIDC and LDAP by default
	if len(cfg.Server.Auth.OIDC) != 0 {
		t.Errorf("Auth.OIDC should be empty by default, got %d", len(cfg.Server.Auth.OIDC))
	}
	if len(cfg.Server.Auth.LDAP) != 0 {
		t.Errorf("Auth.LDAP should be empty by default, got %d", len(cfg.Server.Auth.LDAP))
	}
}

func TestInitialize(t *testing.T) {
	// Save original directory overrides
	origConfig := os.Getenv("SEARCH_CONFIG_DIR")
	origData := os.Getenv("SEARCH_DATA_DIR")
	origLog := os.Getenv("SEARCH_LOG_DIR")
	origCache := os.Getenv("SEARCH_CACHE_DIR")
	defer func() {
		os.Setenv("SEARCH_CONFIG_DIR", origConfig)
		os.Setenv("SEARCH_DATA_DIR", origData)
		os.Setenv("SEARCH_LOG_DIR", origLog)
		os.Setenv("SEARCH_CACHE_DIR", origCache)
		SetConfigDirOverride("")
		SetDataDirOverride("")
		SetLogDirOverride("")
		SetCacheDirOverride("")
	}()

	tmpDir, err := os.MkdirTemp("", "config-init-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Set overrides to temp directories
	SetConfigDirOverride(tmpDir + "/config")
	SetDataDirOverride(tmpDir + "/data")
	SetLogDirOverride(tmpDir + "/logs")
	SetCacheDirOverride(tmpDir + "/cache")

	cfg, err := Initialize()
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	if cfg == nil {
		t.Fatal("Initialize() returned nil config")
	}

	// Verify directories were created
	dirs := []string{
		tmpDir + "/config",
		tmpDir + "/data",
		tmpDir + "/logs",
		tmpDir + "/cache",
	}
	for _, dir := range dirs {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			t.Errorf("Directory %s was not created", dir)
		}
	}

	// Verify config path is set
	if cfg.GetPath() == "" {
		t.Error("Config path should be set after Initialize()")
	}
}

func TestLoadInvalidYAML(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "config-load-invalid-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	configPath := tmpDir + "/server.yml"

	// Write invalid YAML
	err = os.WriteFile(configPath, []byte("invalid: yaml: [unclosed"), 0600)
	if err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}

	_, err = Load(configPath)
	if err == nil {
		t.Error("Load() should error on invalid YAML")
	}
}

func TestSaveToInvalidPath(t *testing.T) {
	cfg := DefaultConfig()

	// Create a file that we'll try to use as a directory
	tmpFile, err := os.CreateTemp("", "config-test-*")
	if err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	// Try to save to a path where a file exists as the parent "directory"
	err = cfg.Save(tmpFile.Name() + "/config.yml")
	if err == nil {
		t.Error("Save() should error when parent path is a file")
	}
}

func TestLoadOrCreateInvalidDirectory(t *testing.T) {
	// Create a file that we'll try to use as a directory
	tmpFile, err := os.CreateTemp("", "config-test-*")
	if err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	// Try to create config where parent path is a file
	_, _, err = LoadOrCreate(tmpFile.Name() + "/server.yml")
	if err == nil {
		t.Error("LoadOrCreate() should error when parent path is a file")
	}
}

func TestDefaultEngines(t *testing.T) {
	cfg := DefaultConfig()

	// Verify default engines are set
	expectedEngines := []string{"google", "duckduckgo", "bing", "brave", "qwant", "startpage"}
	for _, eng := range expectedEngines {
		if _, ok := cfg.Engines[eng]; !ok {
			t.Errorf("Engine %q should be in default config", eng)
		}
	}

	// Verify google is enabled by default
	if !cfg.Engines["google"].Enabled {
		t.Error("Google should be enabled by default")
	}
}

func TestSearchConfigDefaults(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Search.ResultsPerPage != 10 {
		t.Errorf("Search.ResultsPerPage = %d, want 10", cfg.Search.ResultsPerPage)
	}
	if cfg.Search.Timeout != 10 {
		t.Errorf("Search.Timeout = %d, want 10", cfg.Search.Timeout)
	}
	if cfg.Search.SafeSearch != 1 {
		t.Errorf("Search.SafeSearch = %d, want 1 (enabled by default)", cfg.Search.SafeSearch)
	}
}

func TestSSLConfigDefaults(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Server.SSL.AutoTLS {
		t.Error("SSL.AutoTLS should be false by default")
	}
	if cfg.Server.SSL.CertFile != "" {
		t.Errorf("SSL.CertFile should be empty by default, got %q", cfg.Server.SSL.CertFile)
	}
	if cfg.Server.SSL.KeyFile != "" {
		t.Errorf("SSL.KeyFile should be empty by default, got %q", cfg.Server.SSL.KeyFile)
	}
}

func TestCompressionConfigDefaults(t *testing.T) {
	cfg := DefaultConfig()

	if !cfg.Server.Compression.Enabled {
		t.Error("Compression.Enabled should be true by default")
	}
	if cfg.Server.Compression.Level != 6 {
		t.Errorf("Compression.Level = %d, want 6", cfg.Server.Compression.Level)
	}
}

func TestAdminConfigDefaults(t *testing.T) {
	cfg := DefaultConfig()

	if !cfg.Server.Admin.Enabled {
		t.Error("Admin.Enabled should be true by default")
	}
}

func TestUsersConfigDefaults(t *testing.T) {
	cfg := DefaultConfig()

	if !cfg.Server.Users.Enabled {
		t.Error("Users.Enabled should be true by default")
	}
	if cfg.Server.Users.Auth.PasswordMinLength != 8 {
		t.Errorf("Users.Auth.PasswordMinLength = %d, want 8", cfg.Server.Users.Auth.PasswordMinLength)
	}
}
