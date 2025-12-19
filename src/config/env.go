package config

import (
	"os"
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
	
	// Image proxy
	ImageProxyURL string
	ImageProxyKey string
	
	// Search-specific
	Port    string
	Mode    string
	DataDir string
	ConfigDir string
	LogDir  string
	
	// Tor
	UseTor          bool
	TorProxy        string
	TorControlPort  string
	TorControlPass  string
	
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
	
	// Image proxy
	cfg.ImageProxyURL = getEnv("IMAGE_PROXY_URL", "MORTY_URL")
	cfg.ImageProxyKey = getEnv("IMAGE_PROXY_KEY", "MORTY_KEY")
	
	// Search-specific
	cfg.Port = getEnv("SEARCH_PORT", "PORT")
	cfg.Mode = getEnv("SEARCH_MODE", "MODE", "production")
	cfg.DataDir = getEnv("SEARCH_DATA_DIR", "DATA_DIR")
	cfg.ConfigDir = getEnv("SEARCH_CONFIG_DIR", "CONFIG_DIR")
	cfg.LogDir = getEnv("SEARCH_LOG_DIR", "LOG_DIR")
	
	// Tor
	cfg.UseTor = parseBool(getEnv("USE_TOR", "ENABLE_TOR"))
	cfg.TorProxy = getEnv("TOR_PROXY", "SEARCH_TOR_PROXY", "127.0.0.1:9050")
	cfg.TorControlPort = getEnv("TOR_CONTROL_PORT", "SEARCH_TOR_CONTROL", "127.0.0.1:9051")
	cfg.TorControlPass = getEnv("TOR_CONTROL_PASSWORD", "TOR_PASSWORD")
	
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

// ParseBool parses boolean from string (compatible with multiple formats)
// Per spec: true/yes/on/1/enable/enabled are true
// Per spec: false/no/off/0/disable/disabled are false
// Any positive integer is true, 0 is false
func ParseBool(val string) bool {
	if val == "" {
		return false
	}

	val = strings.ToLower(strings.TrimSpace(val))

	// True values (per spec)
	switch val {
	case "1", "true", "yes", "on", "enable", "enabled", "y", "t":
		return true
	}

	// False values (per spec)
	switch val {
	case "0", "false", "no", "off", "disable", "disabled", "n", "f":
		return false
	}

	// Try parsing as integer - any positive number is true
	if i, err := strconv.Atoi(val); err == nil {
		return i > 0
	}

	return false
}

// parseBool is an alias for ParseBool (for internal use)
func parseBool(val string) bool {
	return ParseBool(val)
}

// ParseBoolDefault parses boolean with a default value
func ParseBoolDefault(val string, defaultVal bool) bool {
	if val == "" {
		return defaultVal
	}
	return ParseBool(val)
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
		// No unit, assume seconds
		return strconv.Atoi(val)
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
