package config

import (
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// EnvConfig loads configuration from environment variables
type EnvConfig struct {
	// Core configuration
	SettingsPath string
	Debug        bool
	Secret       string
	BindAddress  string
	InstanceName string
	Autocomplete string
	BaseURL      string

	// Per AI.md PART 5: DOMAIN env var for FQDN override
	Domain string

	// Per AI.md PART 5: DATABASE_DRIVER env var
	DatabaseDriver string
	DatabaseURL    string

	// Image proxy
	ImageProxyURL string
	ImageProxyKey string

	// Search-specific
	Port      string
	Mode      string
	DataDir   string
	ConfigDir string
	LogDir    string

	// Tor: Per AI.md PART 32, auto-enabled if binary found - no env vars needed

	// SMTP - Per AI.md PART 18: SMTP_* env vars override config file
	SMTPHost      string
	SMTPPort      int
	SMTPUsername  string
	SMTPPassword  string
	SMTPTLS       string
	SMTPFromName  string
	SMTPFromEmail string

	// Engines
	EnableGoogle      bool
	EnableDuckDuckGo  bool
	EnableBing        bool
}

// LoadFromEnv loads configuration from environment variables
func LoadFromEnv() *EnvConfig {
	cfg := &EnvConfig{
		// Set defaults
		InstanceName:     "Search",
		EnableDuckDuckGo: true,
		EnableGoogle:     true,
		EnableBing:       true,
	}

	// Core configuration
	cfg.SettingsPath = getEnv("SEARCH_SETTINGS_PATH", "SETTINGS_PATH")
	cfg.Debug = parseBool(getEnv("DEBUG", "SEARCH_DEBUG"))
	cfg.Secret = getEnv("SECRET_KEY", "SEARCH_SECRET")
	cfg.BindAddress = getEnv("BIND_ADDRESS", "SEARCH_BIND_ADDRESS")
	cfg.InstanceName = getEnv("INSTANCE_NAME", "APPLICATION_NAME", cfg.InstanceName)
	cfg.Autocomplete = getEnv("AUTOCOMPLETE", "")
	cfg.BaseURL = getEnv("BASE_URL", "")

	// Per AI.md PART 5: DOMAIN env var (highest priority for FQDN)
	cfg.Domain = getEnv("DOMAIN")

	// Per AI.md PART 5: Database configuration
	cfg.DatabaseDriver = getEnv("DATABASE_DRIVER")
	cfg.DatabaseURL = getEnv("DATABASE_URL")

	// Image proxy
	cfg.ImageProxyURL = getEnv("IMAGE_PROXY_URL", "MORTY_URL")
	cfg.ImageProxyKey = getEnv("IMAGE_PROXY_KEY", "MORTY_KEY")

	// Search-specific
	cfg.Port = getEnv("SEARCH_PORT", "PORT")
	cfg.Mode = getEnv("SEARCH_MODE", "MODE", "production")
	cfg.DataDir = getEnv("SEARCH_DATA_DIR", "DATA_DIR")
	cfg.ConfigDir = getEnv("SEARCH_CONFIG_DIR", "CONFIG_DIR")
	cfg.LogDir = getEnv("SEARCH_LOG_DIR", "LOG_DIR")

	// Tor - Auto-detection per AI.md PART 29
	// Tor: Per AI.md PART 32, auto-enabled if binary found at runtime
	// Detection happens in TorService.Start(), not via env vars

	// SMTP - Per AI.md PART 18: SMTP_* env vars override config file settings
	cfg.SMTPHost = getEnv("SMTP_HOST")
	cfg.SMTPPort = ParseInt(getEnv("SMTP_PORT"), 0)
	cfg.SMTPUsername = getEnv("SMTP_USERNAME")
	cfg.SMTPPassword = getEnv("SMTP_PASSWORD")
	cfg.SMTPTLS = getEnv("SMTP_TLS")
	cfg.SMTPFromName = getEnv("SMTP_FROM_NAME")
	cfg.SMTPFromEmail = getEnv("SMTP_FROM_EMAIL")

	// Engines
	cfg.EnableGoogle = parseBool(getEnv("ENABLE_GOOGLE", "SEARCH_ENGINES_GOOGLE", "true"))
	cfg.EnableDuckDuckGo = parseBool(getEnv("ENABLE_DUCKDUCKGO", "SEARCH_ENGINES_DUCKDUCKGO", "true"))
	cfg.EnableBing = parseBool(getEnv("ENABLE_BING", "SEARCH_ENGINES_BING", "true"))

	// Map DEBUG to MODE
	if cfg.Debug {
		cfg.Mode = "development"
	}

	// Parse BIND_ADDRESS into port if needed
	if cfg.BindAddress != "" && cfg.Port == "" {
		parts := strings.Split(cfg.BindAddress, ":")
		if len(parts) == 2 {
			cfg.Port = parts[1]
		}
	}

	// Use directory functions if not overridden
	if cfg.ConfigDir == "" {
		cfg.ConfigDir = GetConfigDir()
	}
	if cfg.DataDir == "" {
		cfg.DataDir = GetDataDir()
	}
	if cfg.LogDir == "" {
		cfg.LogDir = GetLogDir()
	}

	return cfg
}

// getEnv gets environment variable with multiple fallback keys
func getEnv(keys ...string) string {
	for _, key := range keys {
		if val := os.Getenv(key); val != "" {
			return val
		}
	}
	return ""
}

// parseBool is a helper that calls ParseBool from bool.go with default false
// Per AI.md PART 4: Boolean Handling (NON-NEGOTIABLE)
func parseBool(val string) bool {
	result, _ := ParseBool(val, false)
	return result
}

// TrimmedFormValue returns a form value with leading and trailing whitespace stripped
// Per AI.md: ALL input fields must have whitespace trimmed
func TrimmedFormValue(r *http.Request, key string) string {
	return strings.TrimSpace(r.FormValue(key))
}

// TrimmedPostFormValue returns a POST form value with whitespace stripped
func TrimmedPostFormValue(r *http.Request, key string) string {
	return strings.TrimSpace(r.PostFormValue(key))
}

// ParseFormBool parses boolean from HTML form values
// Per AI.md PART 4: Extended boolean handling
// HTML checkboxes send "on" when checked, nothing when unchecked
// HTML radio buttons send their value attribute
func ParseFormBool(val string) bool {
	if val == "" {
		return false
	}

	val = strings.ToLower(strings.TrimSpace(val))

	// HTML checkbox sends "on" when checked
	// Other common form values
	switch val {
	case "1", "true", "yes", "on", "checked":
		return true
	}

	return false
}

// ParseBoolDefault parses boolean with a default value
// Calls ParseBool from bool.go
func ParseBoolDefault(val string, defaultVal bool) bool {
	result, _ := ParseBool(val, defaultVal)
	return result
}

// ParseInt parses an integer from string with default
func ParseInt(val string, defaultVal int) int {
	if val == "" {
		return defaultVal
	}
	if i, err := strconv.Atoi(strings.TrimSpace(val)); err == nil {
		return i
	}
	return defaultVal
}

// ParseDuration parses a duration string (e.g., "30d", "24h", "30m", "60s")
func ParseDuration(val string) (int, error) {
	if val == "" {
		return 0, nil
	}

	val = strings.ToLower(strings.TrimSpace(val))
	if len(val) < 2 {
		return strconv.Atoi(val)
	}

	unit := val[len(val)-1]

	// If last character is a digit, treat whole value as seconds
	if unit >= '0' && unit <= '9' {
		return strconv.Atoi(val)
	}

	numStr := val[:len(val)-1]
	num, err := strconv.Atoi(numStr)
	if err != nil {
		return 0, err
	}

	switch unit {
	case 's':
		return num, nil
	case 'm':
		return num * 60, nil
	case 'h':
		return num * 3600, nil
	case 'd':
		return num * 86400, nil
	case 'w':
		return num * 604800, nil
	default:
		// Unknown letter unit, use numeric portion as seconds
		return num, nil
	}
}

// GetMode returns the application mode
func (e *EnvConfig) GetMode() string {
	mode := strings.ToLower(e.Mode)
	switch mode {
	case "dev", "development":
		return "development"
	case "prod", "production":
		return "production"
	default:
		return "production"
	}
}

// IsDevelopment returns true if in development mode
func (e *EnvConfig) IsDevelopment() bool {
	return e.GetMode() == "development"
}

// IsProduction returns true if in production mode
func (e *EnvConfig) IsProduction() bool {
	return e.GetMode() == "production"
}

// isTorAvailable checks if the tor binary is installed and available
// Per AI.md PART 29: Tor auto-enabled if tor binary installed
func isTorAvailable() bool {
	_, err := exec.LookPath("tor")
	return err == nil
}

// IsTorAvailable is an exported version of isTorAvailable
// Per AI.md PART 29: Tor auto-enabled if tor binary installed
func IsTorAvailable() bool {
	return isTorAvailable()
}

// GetDomain returns the DOMAIN environment variable
// Per AI.md PART 5: DOMAIN env var for FQDN override (highest priority)
// Returns first domain if comma-separated list
func GetDomain() string {
	domain := os.Getenv("DOMAIN")
	if domain == "" {
		return ""
	}
	// Return first domain as primary if comma-separated
	if idx := strings.Index(domain, ","); idx > 0 {
		return strings.TrimSpace(domain[:idx])
	}
	return strings.TrimSpace(domain)
}

// GetAllDomains returns all domains from DOMAIN env var
// Per AI.md PART 5: Used for CORS configuration and SSL certificates
func GetAllDomains() []string {
	domain := os.Getenv("DOMAIN")
	if domain == "" {
		return nil
	}

	parts := strings.Split(domain, ",")
	domains := make([]string, 0, len(parts))
	for _, d := range parts {
		d = strings.TrimSpace(d)
		if d != "" {
			domains = append(domains, d)
		}
	}
	return domains
}

// GetDatabaseDriver returns the DATABASE_DRIVER environment variable
// Per AI.md PART 5: Database driver override
func GetDatabaseDriver() string {
	return os.Getenv("DATABASE_DRIVER")
}

// GetDatabaseURL returns the DATABASE_URL environment variable
// Per AI.md PART 5: Database connection string
func GetDatabaseURL() string {
	return os.Getenv("DATABASE_URL")
}
