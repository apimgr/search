package config

import (
	"net/http"
	"os"
	"strconv"
	"strings"
)

// EnvConfig holds environment variable values per AI.md PART 5.
// Runtime variables are checked on every startup.
// Init-only variables are used once during first run, then ignored.
type EnvConfig struct {
	// Runtime Variables (Always Checked) - per AI.md PART 5
	// NO_COLOR and TERM are checked directly where needed (not stored here)
	Domain         string // FQDN override (highest priority for hostname resolution)
	Mode           string // production (default) or development
	DatabaseDriver string // sqlite, sqlite2, sqlite3, libsql, turso
	DatabaseURL    string // Database connection string
	SMTPHost       string // SMTP server hostname
	SMTPPort       int    // SMTP server port (default: 587)
	SMTPUsername   string // SMTP authentication username
	SMTPPassword   string // SMTP authentication password
	SMTPFromName   string // Sender name (default: app title)
	SMTPFromEmail  string // Sender email (default: no-reply@{fqdn})
	SMTPTLS        string // TLS mode: auto, starttls, tls, none

	// Init-Only Variables (First Run Only) - per AI.md PART 5
	ConfigDir          string // Configuration directory
	DataDir            string // Data directory
	LogDir             string // Log directory
	DatabaseDir        string // SQLite database directory
	BackupDir          string // Backup directory
	Port               string // Server port
	Listen             string // Listen address
	ApplicationName    string // Application title
	ApplicationTagline string // Application description
}

// LoadFromEnv loads configuration from environment variables per AI.md PART 5.
// Only loads the exact env vars specified in AI.md - no undocumented variants.
func LoadFromEnv() *EnvConfig {
	cfg := &EnvConfig{}

	// Runtime Variables (Always Checked) - per AI.md PART 5
	// NO_COLOR and TERM checked directly via os.Getenv where needed
	cfg.Domain = os.Getenv("DOMAIN")
	cfg.Mode = getEnvWithDefault("MODE", "production")
	cfg.DatabaseDriver = os.Getenv("DATABASE_DRIVER")
	cfg.DatabaseURL = os.Getenv("DATABASE_URL")
	cfg.SMTPHost = os.Getenv("SMTP_HOST")
	cfg.SMTPPort = ParseInt(os.Getenv("SMTP_PORT"), 0)
	cfg.SMTPUsername = os.Getenv("SMTP_USERNAME")
	cfg.SMTPPassword = os.Getenv("SMTP_PASSWORD")
	cfg.SMTPFromName = os.Getenv("SMTP_FROM_NAME")
	cfg.SMTPFromEmail = os.Getenv("SMTP_FROM_EMAIL")
	cfg.SMTPTLS = os.Getenv("SMTP_TLS")

	// Init-Only Variables (First Run Only) - per AI.md PART 5
	cfg.ConfigDir = os.Getenv("CONFIG_DIR")
	cfg.DataDir = os.Getenv("DATA_DIR")
	cfg.LogDir = os.Getenv("LOG_DIR")
	cfg.DatabaseDir = os.Getenv("DATABASE_DIR")
	cfg.BackupDir = os.Getenv("BACKUP_DIR")
	cfg.Port = os.Getenv("PORT")
	cfg.Listen = os.Getenv("LISTEN")
	cfg.ApplicationName = os.Getenv("APPLICATION_NAME")
	cfg.ApplicationTagline = os.Getenv("APPLICATION_TAGLINE")

	return cfg
}

// getEnvWithDefault returns the env var value or the default if empty
func getEnvWithDefault(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

// GetMode returns the normalized application mode
// Per AI.md PART 6: --mode dev or --mode development → development
// Per AI.md PART 6: --mode prod or --mode production → production
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

// parseBool is a helper that calls ParseBool from bool.go with default false
// Per AI.md PART 5: Boolean Handling - always use config.ParseBool()
func parseBool(val string) bool {
	result, _ := ParseBool(val, false)
	return result
}

// TrimmedFormValue returns a form value with leading and trailing whitespace stripped
func TrimmedFormValue(r *http.Request, key string) string {
	return strings.TrimSpace(r.FormValue(key))
}

// TrimmedPostFormValue returns a POST form value with whitespace stripped
func TrimmedPostFormValue(r *http.Request, key string) string {
	return strings.TrimSpace(r.PostFormValue(key))
}

// ParseFormBool parses boolean from HTML form values using the canonical ParseBool parser
// Delegates to ParseBool to cover the full extended truthy/falsy set from AI.md PART 5
func ParseFormBool(val string) bool {
	result, _ := ParseBool(val, false)
	return result
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
// Used for CORS configuration and SSL certificates
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
// Per AI.md PART 5: Runtime variable - always checked
func GetDatabaseDriver() string {
	return os.Getenv("DATABASE_DRIVER")
}

// GetDatabaseURL returns the DATABASE_URL environment variable
// Per AI.md PART 5: Runtime variable - always checked
func GetDatabaseURL() string {
	return os.Getenv("DATABASE_URL")
}

// IsNoColor returns true if NO_COLOR env var is set and non-empty
// Per AI.md PART 5: Runtime variable - always checked
func IsNoColor() bool {
	return os.Getenv("NO_COLOR") != ""
}

// IsDumbTerminal returns true if TERM=dumb
// Per AI.md PART 5: Runtime variable - forces CLI mode
func IsDumbTerminal() bool {
	return os.Getenv("TERM") == "dumb"
}
