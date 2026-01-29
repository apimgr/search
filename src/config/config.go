package config

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// Build info - set via -ldflags at build time
var (
	Version   = "dev"
	CommitID  = "unknown"
	BuildDate = "unknown"
)

// Config represents the complete application configuration
type Config struct {
	mu         sync.RWMutex
	configPath string // Path to config file for reload
	firstRun   bool   // True if this is first run (no config existed)

	Server  ServerConfig           `yaml:"server"`
	Search  SearchConfig           `yaml:"search"`
	Engines map[string]EngineConfig `yaml:"engines"`
}

// IsFirstRun returns true if this is the first run (config was just created)
// Per AI.md PART 14: First run shows setup token for admin creation
func (c *Config) IsFirstRun() bool {
	return c.firstRun
}

// SetPath sets the config file path for reload
func (c *Config) SetPath(path string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.configPath = path
}

// GetPath returns the config file path
func (c *Config) GetPath() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.configPath
}

// Reload reloads the configuration from the original file
// Note: Some settings (port, address) may require restart to take effect
func (c *Config) Reload() error {
	c.mu.RLock()
	path := c.configPath
	c.mu.RUnlock()

	if path == "" {
		return fmt.Errorf("config path not set, cannot reload")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	var newCfg Config
	if err := yaml.Unmarshal(data, &newCfg); err != nil {
		return fmt.Errorf("failed to parse config file: %w", err)
	}

	// Update all reloadable settings
	c.mu.Lock()
	defer c.mu.Unlock()

	// Preserve path and mutex
	newCfg.configPath = c.configPath

	// Copy all reloadable settings
	// Note: Port and Address changes require restart
	oldPort := c.Server.Port
	oldAddress := c.Server.Address

	c.Server = newCfg.Server
	c.Search = newCfg.Search
	c.Engines = newCfg.Engines

	// Restore port/address if changed (require restart)
	if c.Server.Port != oldPort || c.Server.Address != oldAddress {
		// Log warning that port/address changes require restart
		c.Server.Port = oldPort
		c.Server.Address = oldAddress
	}

	return nil
}

// ServerConfig represents server configuration
type ServerConfig struct {
	// Core settings
	Title       string `yaml:"title"`
	Description string `yaml:"description"`
	Port        int    `yaml:"port"`        // HTTP port (or single port if HTTPSPort not set)
	HTTPSPort   int    `yaml:"https_port"`  // HTTPS port for dual port mode (optional)
	Address     string `yaml:"address"`
	Mode        string `yaml:"mode"`
	SecretKey   string `yaml:"secret_key"`
	BaseURL     string `yaml:"base_url"`

	// SSL/TLS
	SSL SSLConfig `yaml:"ssl"`

	// Admin
	Admin AdminConfig `yaml:"admin"`

	// Branding
	Branding BrandingConfig `yaml:"branding"`

	// Rate Limiting
	RateLimit RateLimitConfig `yaml:"rate_limit"`

	// Session
	Session SessionConfig `yaml:"session"`

	// Logs
	Logs LogsConfig `yaml:"logs"`

	// Tor
	Tor TorConfig `yaml:"tor"`

	// Email
	Email EmailConfig `yaml:"email"`

	// Security
	Security SecurityConfig `yaml:"security"`

	// External Auth (OIDC/LDAP) per AI.md PART 31
	Auth AuthConfig `yaml:"auth"`

	// Users
	Users UsersConfig `yaml:"users"`

	// Pages
	Pages PagesConfig `yaml:"pages"`

	// Web (robots.txt, security.txt)
	Web WebConfig `yaml:"web"`

	// Scheduler
	Scheduler SchedulerConfig `yaml:"scheduler"`

	// Cache - per AI.md PART 18
	Cache CacheConfig `yaml:"cache"`

	// GeoIP
	GeoIP GeoIPConfig `yaml:"geoip"`

	// Metrics
	Metrics MetricsConfig `yaml:"metrics"`

	// Image Proxy
	ImageProxy ImageProxyConfig `yaml:"image_proxy"`

	// Contact
	Contact ContactConfig `yaml:"contact"`

	// SEO
	SEO SEOConfig `yaml:"seo"`

	// Compression
	Compression CompressionConfig `yaml:"compression"`

	// Request Limits per AI.md PART 18
	Limits LimitsConfig `yaml:"limits"`

	// I18n (Internationalization)
	I18n I18nConfig `yaml:"i18n"`

	// Maintenance mode - when enabled, shows maintenance page to all users
	MaintenanceMode bool `yaml:"maintenance_mode"`

	// Backup configuration per AI.md PART 22
	Backup BackupConfig `yaml:"backup"`

	// Compliance configuration per AI.md PART 22
	Compliance ComplianceConfig `yaml:"compliance"`
}

// SSLConfig represents SSL/TLS configuration
type SSLConfig struct {
	Enabled     bool   `yaml:"enabled"`
	AutoTLS     bool   `yaml:"auto_tls"`
	CertFile    string `yaml:"cert_file"`
	KeyFile     string `yaml:"key_file"`
	LetsEncrypt struct {
		Enabled   bool     `yaml:"enabled"`
		Email     string   `yaml:"email"`
		Domains   []string `yaml:"domains"`
		Staging   bool     `yaml:"staging"`
		Challenge string   `yaml:"challenge"` // http-01, tls-alpn-01, dns-01
	} `yaml:"letsencrypt"`
	// DNS-01 provider configuration per AI.md PART 17
	DNS01 DNS01Config `yaml:"dns01"`
}

// DNS01Config represents DNS-01 ACME challenge configuration
// Per AI.md: ALL DNS providers are supported via go-acme/lego
type DNS01Config struct {
	Provider             string `yaml:"provider"`              // Provider identifier (cloudflare, route53, etc.)
	CredentialsEncrypted string `yaml:"credentials_encrypted"` // AES-256-GCM encrypted JSON
	ValidatedAt          string `yaml:"validated_at"`          // Timestamp of last successful validation
}

// AdminConfig represents admin configuration
type AdminConfig struct {
	Username string `yaml:"username"`
	Password string `yaml:"password"`
	Token    string `yaml:"token"`
	APIToken string `yaml:"api_token"`
	Email    string `yaml:"email"`
	Enabled  bool   `yaml:"enabled"`
}

// BrandingConfig represents branding configuration
type BrandingConfig struct {
	AppName     string `yaml:"app_name"`
	LogoURL     string `yaml:"logo_url"`
	FaviconURL  string `yaml:"favicon_url"`
	FooterText  string `yaml:"footer_text"`
	Theme       string `yaml:"theme"`
	PrimaryColor string `yaml:"primary_color"`
}

// RateLimitConfig represents rate limiting configuration
type RateLimitConfig struct {
	Enabled           bool `yaml:"enabled"`
	RequestsPerMinute int  `yaml:"requests_per_minute"`
	RequestsPerHour   int  `yaml:"requests_per_hour"`
	RequestsPerDay    int  `yaml:"requests_per_day"`
	BurstSize         int  `yaml:"burst_size"`
	ByIP              bool `yaml:"by_ip"`
	ByUser            bool `yaml:"by_user"`
	Whitelist         []string `yaml:"whitelist"`
	Blacklist         []string `yaml:"blacklist"`
}

// SessionTypeConfig represents configuration for a specific session type (admin or user)
type SessionTypeConfig struct {
	CookieName  string `yaml:"cookie_name"`
	MaxAge      int    `yaml:"max_age"`      // Absolute session lifetime in seconds
	IdleTimeout int    `yaml:"idle_timeout"` // Expires after inactivity in seconds
}

// SessionConfig represents session configuration per AI.md PART 13
type SessionConfig struct {
	// Admin sessions (server.db admin_sessions table)
	Admin SessionTypeConfig `yaml:"admin"`
	// User sessions (user.db user_sessions table)
	User SessionTypeConfig `yaml:"user"`
	// Common settings
	ExtendOnActivity bool   `yaml:"extend_on_activity"` // Reset idle timeout on each request
	Secure           string `yaml:"secure"`             // auto, true, false
	HTTPOnly         bool   `yaml:"http_only"`
	SameSite         string `yaml:"same_site"` // strict, lax, none

	// Legacy fields for backward compatibility
	Duration       string `yaml:"duration,omitempty"`
	CookieName     string `yaml:"cookie_name,omitempty"`
	CookieSecure   bool   `yaml:"cookie_secure,omitempty"`
	CookieHTTPOnly bool   `yaml:"cookie_http_only,omitempty"`
	CookieSameSite string `yaml:"cookie_same_site,omitempty"`
}

// GetAdminCookieName returns the admin session cookie name
func (s *SessionConfig) GetAdminCookieName() string {
	if s.Admin.CookieName != "" {
		return s.Admin.CookieName
	}
	if s.CookieName != "" {
		return s.CookieName
	}
	return "admin_session"
}

// GetUserCookieName returns the user session cookie name
func (s *SessionConfig) GetUserCookieName() string {
	if s.User.CookieName != "" {
		return s.User.CookieName
	}
	if s.CookieName != "" {
		return s.CookieName
	}
	return "user_session"
}

// GetAdminMaxAge returns admin session max age in seconds (default 30 days)
func (s *SessionConfig) GetAdminMaxAge() int {
	if s.Admin.MaxAge > 0 {
		return s.Admin.MaxAge
	}
	return 2592000 // 30 days
}

// GetUserMaxAge returns user session max age in seconds (default 7 days)
func (s *SessionConfig) GetUserMaxAge() int {
	if s.User.MaxAge > 0 {
		return s.User.MaxAge
	}
	return 604800 // 7 days
}

// GetIdleTimeout returns idle timeout in seconds (default 24 hours)
func (s *SessionConfig) GetIdleTimeout() int {
	if s.Admin.IdleTimeout > 0 {
		return s.Admin.IdleTimeout
	}
	if s.User.IdleTimeout > 0 {
		return s.User.IdleTimeout
	}
	return 86400 // 24 hours
}

// IsSecure returns whether cookies should be secure
func (s *SessionConfig) IsSecure(sslEnabled bool) bool {
	switch s.Secure {
	case "true", "yes", "1":
		return true
	case "false", "no", "0":
		return false
	default: // "auto" or empty
		return sslEnabled || s.CookieSecure
	}
}

// IsHTTPOnly returns whether cookies should be HTTP only
func (s *SessionConfig) IsHTTPOnly() bool {
	if s.HTTPOnly {
		return true
	}
	return s.CookieHTTPOnly
}

// GetSameSite returns the SameSite cookie attribute
func (s *SessionConfig) GetSameSite() string {
	if s.SameSite != "" {
		return s.SameSite
	}
	if s.CookieSameSite != "" {
		return s.CookieSameSite
	}
	return "lax"
}

// LogsConfig represents logging configuration
type LogsConfig struct {
	Level  string `yaml:"level"`
	File   string `yaml:"file"`
	Format string `yaml:"format"`
	Access struct {
		Filename string `yaml:"filename"`
		Format   string `yaml:"format"`
		Custom   string `yaml:"custom"`
		Rotate   string `yaml:"rotate"`
		Keep     string `yaml:"keep"`
	} `yaml:"access"`
	Server struct {
		Filename string `yaml:"filename"`
		Format   string `yaml:"format"`
		Custom   string `yaml:"custom"`
		Rotate   string `yaml:"rotate"`
		Keep     string `yaml:"keep"`
	} `yaml:"server"`
	Error struct {
		Filename string `yaml:"filename"`
		Format   string `yaml:"format"`
		Custom   string `yaml:"custom"`
		Rotate   string `yaml:"rotate"`
		Keep     string `yaml:"keep"`
	} `yaml:"error"`
	Audit struct {
		Filename string `yaml:"filename"`
		Format   string `yaml:"format"`
		Custom   string `yaml:"custom"`
		Rotate   string `yaml:"rotate"`
		Keep     string `yaml:"keep"`
	} `yaml:"audit"`
	Security struct {
		Filename string `yaml:"filename"`
		Format   string `yaml:"format"`
		Custom   string `yaml:"custom"`
		Rotate   string `yaml:"rotate"`
		Keep     string `yaml:"keep"`
	} `yaml:"security"`
	Debug struct {
		Enabled  bool   `yaml:"enabled"`
		Filename string `yaml:"filename"`
		Format   string `yaml:"format"`
		Custom   string `yaml:"custom"`
		Rotate   string `yaml:"rotate"`
		Keep     string `yaml:"keep"`
	} `yaml:"debug"`
}

// TorConfig represents Tor configuration
// Per AI.md PART 32: "Auto-enabled if tor binary is installed - no enable flag needed"
type TorConfig struct {
	Enabled           bool   `yaml:"-"` // Computed at runtime, NOT configurable
	Binary            string `yaml:"binary"`
	DataDir           string `yaml:"data_dir"`
	OnionAddress      string `yaml:"-"` // Set at runtime when Tor starts
	HiddenServicePort int    `yaml:"hidden_service_port"`
}

// EmailConfig represents email/SMTP configuration
// Per AI.md PART 18: Nested SMTP and From blocks
type EmailConfig struct {
	// Enabled is auto-set based on SMTP availability (no manual toggle)
	Enabled bool `yaml:"-"` // Computed, not stored
	SMTP    SMTPConfig `yaml:"smtp"`
	From    EmailFromConfig `yaml:"from"`
}

// SMTPConfig represents SMTP server configuration
// Per AI.md PART 18: SMTP configuration with env var overrides
type SMTPConfig struct {
	// If empty: autodetect local SMTP on startup
	// If set: test connection on startup
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
	// TLS mode: auto, starttls, tls, none
	TLS      string `yaml:"tls"`
}

// EmailFromConfig represents the from address configuration
// Per AI.md PART 18: From name and email defaults
type EmailFromConfig struct {
	// Default: app title
	Name  string `yaml:"name"`
	// Default: no-reply@{fqdn}
	Email string `yaml:"email"`
}

// SecurityConfig represents security configuration
type SecurityConfig struct {
	// CORS
	CORS struct {
		Enabled        bool     `yaml:"enabled"`
		AllowedOrigins []string `yaml:"allowed_origins"`
		AllowedMethods []string `yaml:"allowed_methods"`
		AllowedHeaders []string `yaml:"allowed_headers"`
		AllowCredentials bool    `yaml:"allow_credentials"`
		MaxAge         int      `yaml:"max_age"`
	} `yaml:"cors"`
	// CSRF
	CSRF struct {
		Enabled    bool   `yaml:"enabled"`
		CookieName string `yaml:"cookie_name"`
		HeaderName string `yaml:"header_name"`
		FieldName  string `yaml:"field_name"`
	} `yaml:"csrf"`
	// Headers
	Headers struct {
		XFrameOptions         string `yaml:"x_frame_options"`
		XContentTypeOptions   string `yaml:"x_content_type_options"`
		XXSSProtection        string `yaml:"x_xss_protection"`
		ReferrerPolicy        string `yaml:"referrer_policy"`
		ContentSecurityPolicy string `yaml:"content_security_policy"`
		PermissionsPolicy     string `yaml:"permissions_policy"`
	} `yaml:"headers"`
	// Trusted Proxies
	TrustedProxies []string `yaml:"trusted_proxies"`
}

// AuthConfig represents external authentication configuration per AI.md PART 31
type AuthConfig struct {
	OIDC []OIDCProviderConfig `yaml:"oidc"`
	LDAP []LDAPConfig         `yaml:"ldap"`
}

// OIDCProviderConfig represents an OIDC provider configuration
type OIDCProviderConfig struct {
	ID           string   `yaml:"id"`
	Name         string   `yaml:"name"`
	Enabled      bool     `yaml:"enabled"`
	Issuer       string   `yaml:"issuer"`
	ClientID     string   `yaml:"client_id"`
	ClientSecret string   `yaml:"client_secret"`
	RedirectURL  string   `yaml:"redirect_url"`
	Scopes       []string `yaml:"scopes"`
	// Admin group mapping per AI.md PART 31
	AdminGroups []string `yaml:"admin_groups"`
	GroupsClaim string   `yaml:"groups_claim"`
	AutoCreate  bool     `yaml:"auto_create"`
}

// LDAPConfig represents LDAP authentication configuration
type LDAPConfig struct {
	ID             string   `yaml:"id"`
	Name           string   `yaml:"name"`
	Enabled        bool     `yaml:"enabled"`
	Host           string   `yaml:"host"`
	Port           int      `yaml:"port"`
	UseTLS         bool     `yaml:"use_tls"`
	SkipTLSVerify  bool     `yaml:"skip_tls_verify"`
	BindDN         string   `yaml:"bind_dn"`
	BindPassword   string   `yaml:"bind_password"`
	BaseDN         string   `yaml:"base_dn"`
	UserFilter     string   `yaml:"user_filter"`
	UsernameAttr   string   `yaml:"username_attr"`
	EmailAttr      string   `yaml:"email_attr"`
	// Admin group mapping per AI.md PART 31
	AdminGroups     []string `yaml:"admin_groups"`
	GroupFilter     string   `yaml:"group_filter"`
	GroupMemberAttr string   `yaml:"group_member_attr"`
}

// UsersConfig represents user management configuration
type UsersConfig struct {
	Enabled bool `yaml:"enabled"`
	Registration struct {
		Enabled                  bool     `yaml:"enabled"`
		RequireEmailVerification bool     `yaml:"require_email_verification"`
		RequireApproval          bool     `yaml:"require_approval"`
		AllowedDomains           []string `yaml:"allowed_domains"`
		BlockedDomains           []string `yaml:"blocked_domains"`
	} `yaml:"registration"`
	Roles struct {
		Available []string `yaml:"available"`
		Default   string   `yaml:"default"`
	} `yaml:"roles"`
	Tokens struct {
		Enabled        bool `yaml:"enabled"`
		MaxPerUser     int  `yaml:"max_per_user"`
		ExpirationDays int  `yaml:"expiration_days"`
	} `yaml:"tokens"`
	Profile struct {
		AllowAvatar      bool `yaml:"allow_avatar"`
		AllowDisplayName bool `yaml:"allow_display_name"`
		AllowBio         bool `yaml:"allow_bio"`
	} `yaml:"profile"`
	Auth struct {
		SessionDuration          string `yaml:"session_duration"`
		SessionDurationDays      int    `yaml:"session_duration_days"` // Parsed from SessionDuration
		Require2FA               bool   `yaml:"require_2fa"`
		Allow2FA                 bool   `yaml:"allow_2fa"`
		PasswordMinLength        int    `yaml:"password_min_length"`
		PasswordRequireUppercase bool   `yaml:"password_require_uppercase"`
		PasswordRequireNumber    bool   `yaml:"password_require_number"`
		PasswordRequireSpecial   bool   `yaml:"password_require_special"`
	} `yaml:"auth"`
	Limits struct {
		RequestsPerMinute int `yaml:"requests_per_minute"`
		RequestsPerDay    int `yaml:"requests_per_day"`
	} `yaml:"limits"`
	SSO struct {
		Enabled bool `yaml:"enabled"`
		OIDC    map[string]struct {
			Name         string `yaml:"name"`
			ClientID     string `yaml:"client_id"`
			ClientSecret string `yaml:"client_secret"`
			Issuer       string `yaml:"issuer"`
			IconURL      string `yaml:"icon_url"`
		} `yaml:"oidc"`
		LDAP struct {
			Enabled  bool   `yaml:"enabled"`
			Server   string `yaml:"server"`
			Port     int    `yaml:"port"`
			BaseDN   string `yaml:"base_dn"`
			BindDN   string `yaml:"bind_dn"`
			BindPass string `yaml:"bind_pass"`
		} `yaml:"ldap"`
	} `yaml:"sso"`
}

// GetSessionDurationDays returns the session duration in days, parsing from string if needed
func (u *UsersConfig) GetSessionDurationDays() int {
	// If explicitly set, use it
	if u.Auth.SessionDurationDays > 0 {
		return u.Auth.SessionDurationDays
	}

	// Parse from SessionDuration string (e.g., "30d", "7d")
	dur := u.Auth.SessionDuration
	if dur == "" {
		return 30 // default 30 days
	}

	// Check suffix to determine unit
	if strings.HasSuffix(dur, "d") {
		var days int
		if n, _ := fmt.Sscanf(dur, "%dd", &days); n == 1 && days > 0 {
			return days
		}
	}

	// Try parsing as hours (e.g., "720h")
	if strings.HasSuffix(dur, "h") {
		var hours int
		if n, _ := fmt.Sscanf(dur, "%dh", &hours); n == 1 && hours > 0 {
			return hours / 24
		}
	}

	return 30 // default
}

// PagesConfig represents standard pages configuration
type PagesConfig struct {
	About struct {
		Enabled bool   `yaml:"enabled"`
		Content string `yaml:"content"`
	} `yaml:"about"`
	Privacy struct {
		Enabled bool   `yaml:"enabled"`
		Content string `yaml:"content"`
	} `yaml:"privacy"`
	Contact struct {
		Enabled bool   `yaml:"enabled"`
		Email   string `yaml:"email"`
	} `yaml:"contact"`
	Help struct {
		Enabled bool   `yaml:"enabled"`
		Content string `yaml:"content"`
	} `yaml:"help"`
	Terms struct {
		Enabled bool   `yaml:"enabled"`
		Content string `yaml:"content"`
	} `yaml:"terms"`
}

// WebConfig represents web settings (robots.txt, security.txt, announcements)
type WebConfig struct {
	Robots struct {
		Allow []string `yaml:"allow"`
		Deny  []string `yaml:"deny"`
	} `yaml:"robots"`
	Security struct {
		Contact string `yaml:"contact"` // Security contact email (mailto: prefix added automatically)
		Expires string `yaml:"expires"` // Expiration date (auto-calculated 1 year from now if not set)
	} `yaml:"security"`
	Announcements AnnouncementsConfig `yaml:"announcements"`
	CookieConsent CookieConsentConfig `yaml:"cookie_consent"`
	CORS          string              `yaml:"cors"` // "*", "origin1,origin2", or ""
}

// AnnouncementsConfig represents announcement settings (per AI.md)
type AnnouncementsConfig struct {
	Enabled  bool           `yaml:"enabled"`
	Messages []Announcement `yaml:"messages"`
}

// Announcement represents a single announcement message
type Announcement struct {
	ID          string `yaml:"id"`
	Type        string `yaml:"type"` // warning, info, error, success
	Title       string `yaml:"title"`
	Message     string `yaml:"message"`
	Start       string `yaml:"start"`       // ISO 8601 datetime
	End         string `yaml:"end"`         // ISO 8601 datetime
	Dismissible bool   `yaml:"dismissible"` // User can dismiss
}

// CookieConsentConfig represents cookie consent popup settings
type CookieConsentConfig struct {
	Enabled    bool   `yaml:"enabled"`
	Message    string `yaml:"message"`
	PolicyURL  string `yaml:"policy_url"`
	TrackingID string `yaml:"tracking_id"` // Google Analytics ID
}

// ActiveAnnouncements returns announcements that are currently active
func (c *AnnouncementsConfig) ActiveAnnouncements() []Announcement {
	if !c.Enabled {
		return nil
	}

	now := time.Now()
	var active []Announcement

	for _, a := range c.Messages {
		// Parse start time
		var startTime, endTime time.Time
		var err error

		if a.Start != "" {
			startTime, err = time.Parse(time.RFC3339, a.Start)
			if err != nil {
				startTime = time.Time{} // Zero time means always started
			}
		}

		if a.End != "" {
			endTime, err = time.Parse(time.RFC3339, a.End)
			if err != nil {
				endTime = time.Time{} // Zero time means never ends
			}
		}

		// Check if announcement is active
		isActive := true
		if !startTime.IsZero() && now.Before(startTime) {
			isActive = false
		}
		if !endTime.IsZero() && now.After(endTime) {
			isActive = false
		}

		if isActive {
			active = append(active, a)
		}
	}

	return active
}

// SchedulerConfig represents scheduler configuration per AI.md PART 19
// Note: Scheduler is ALWAYS RUNNING - no enable/disable option for the scheduler itself
type SchedulerConfig struct {
	// Timezone for scheduled tasks (default: America/New_York)
	Timezone string `yaml:"timezone"`
	// CatchUpWindow: run missed tasks if within this duration (default: 1h)
	CatchUpWindow string `yaml:"catch_up_window"`
	// Task-specific configuration (only skippable tasks can be disabled)
	Tasks SchedulerTasksConfig `yaml:"tasks"`
}

// SchedulerTasksConfig represents per-task configuration
type SchedulerTasksConfig struct {
	// Daily backup at 02:00 (skippable)
	BackupDaily TaskConfig `yaml:"backup_daily"`
	// Hourly incremental backup (skippable, disabled by default)
	BackupHourly TaskConfig `yaml:"backup_hourly"`
	// GeoIP database update (skippable)
	GeoIPUpdate TaskConfig `yaml:"geoip_update"`
	// Blocklist update (skippable)
	BlocklistUpdate TaskConfig `yaml:"blocklist_update"`
	// CVE database update (skippable)
	CVEUpdate TaskConfig `yaml:"cve_update"`
}

// TaskConfig represents configuration for a scheduled task
type TaskConfig struct {
	Schedule string `yaml:"schedule"` // Cron expression or @every interval
	Enabled  bool   `yaml:"enabled"`
}

// CacheConfig represents cache configuration per AI.md PART 18
// EVERY application MUST support Valkey/Redis for clustering
type CacheConfig struct {
	// Type: none (disabled), memory (default), valkey, redis
	// IMPORTANT: Use valkey/redis for cluster or mixed mode deployments
	Type string `yaml:"type"`

	// Connection: Use EITHER url OR host/port/password (not both)
	// url takes precedence if both are specified
	// Format: redis://user:password@host:port/db or valkey://...
	URL string `yaml:"url"`

	// Individual connection settings (alternative to url)
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	Password string `yaml:"password"`
	DB       int    `yaml:"db"`

	// Connection pool settings
	PoolSize int    `yaml:"pool_size"`
	MinIdle  int    `yaml:"min_idle"`
	Timeout  string `yaml:"timeout"` // Connection timeout (e.g., "5s")

	// Key prefix to avoid collisions (use unique prefix per app)
	Prefix string `yaml:"prefix"`

	// Default TTL in seconds
	TTL int `yaml:"ttl"`

	// Cluster settings (when using Valkey/Redis Cluster)
	Cluster      bool     `yaml:"cluster"`
	ClusterNodes []string `yaml:"cluster_nodes"` // e.g., ["node1:6379", "node2:6379"]
}

// GeoIPConfig represents GeoIP configuration (uses MMDB from sapics/ip-location-db)
// Per AI.md PART 20: GeoIP configuration
type GeoIPConfig struct {
	Enabled          bool     `yaml:"enabled"`
	Dir              string   `yaml:"dir"`              // Directory for MMDB files
	Update           string   `yaml:"update"`           // never, daily, weekly, monthly
	DenyCountries    []string `yaml:"deny_countries"`   // Countries to block (ISO 3166-1 alpha-2)
	AllowedCountries []string `yaml:"allowed_countries"` // If set, only these countries allowed
	// Database toggles per AI.md PART 20
	ASN     bool `yaml:"asn"`     // Enable ASN lookups
	Country bool `yaml:"country"` // Enable country lookups
	City    bool `yaml:"city"`    // Enable city lookups (larger download)
	WHOIS   bool `yaml:"whois"`   // Enable WHOIS lookups
}

// MetricsConfig represents Prometheus-compatible metrics configuration
// Per AI.md PART 21: Metrics configuration
type MetricsConfig struct {
	Enabled         bool      `yaml:"enabled"`
	Endpoint        string    `yaml:"endpoint"`         // Endpoint path (default: /metrics)
	IncludeSystem   bool      `yaml:"include_system"`   // Include system metrics (CPU, memory, disk)
	IncludeRuntime  bool      `yaml:"include_runtime"`  // Include Go runtime metrics
	Token           string    `yaml:"token"`            // Bearer token for authentication (empty = no auth)
	DurationBuckets []float64 `yaml:"duration_buckets"` // Histogram buckets for request duration (seconds)
	SizeBuckets     []float64 `yaml:"size_buckets"`     // Histogram buckets for request size (bytes)
}

// BackupConfig represents backup configuration
// Per AI.md PART 22: Backup & Restore configuration
type BackupConfig struct {
	// Encryption configuration
	Encryption BackupEncryptionConfig `yaml:"encryption"`
	// Retention policy
	Retention BackupRetentionConfig `yaml:"retention"`
}

// BackupEncryptionConfig represents backup encryption settings
// Per AI.md PART 22: Password is NEVER stored - derived on-demand
type BackupEncryptionConfig struct {
	Enabled bool   `yaml:"enabled"` // true if password was set during setup
	Hint    string `yaml:"hint"`    // Optional password hint (stored, NOT the password)
}

// BackupRetentionConfig represents backup retention policy
// Per AI.md PART 22: Retention settings
type BackupRetentionConfig struct {
	MaxBackups  int `yaml:"max_backups"`  // Daily full backups to keep (default: 1)
	KeepWeekly  int `yaml:"keep_weekly"`  // Weekly backups (Sunday) to keep (0 = disabled)
	KeepMonthly int `yaml:"keep_monthly"` // Monthly backups (1st) to keep (0 = disabled)
	KeepYearly  int `yaml:"keep_yearly"`  // Yearly backups (Jan 1st) to keep (0 = disabled)
}

// ComplianceConfig represents compliance mode configuration
// Per AI.md PART 22: When enabled, backup encryption is REQUIRED
type ComplianceConfig struct {
	Enabled bool `yaml:"enabled"` // HIPAA, SOC2, etc. compliance mode
}

// ImageProxyConfig represents image proxy configuration
type ImageProxyConfig struct {
	Enabled bool   `yaml:"enabled"`
	URL     string `yaml:"url"`
	Key     string `yaml:"key"`
}

// ContactConfig represents contact page configuration
type ContactConfig struct {
	Enabled bool   `yaml:"enabled"`
	Email   string `yaml:"email"`
}

// SEOConfig represents SEO configuration
type SEOConfig struct {
	Enabled           bool              `yaml:"enabled"`
	DefaultTitle      string            `yaml:"default_title"`
	TitleSeparator    string            `yaml:"title_separator"`
	DefaultDescription string           `yaml:"default_description"`
	Keywords          []string          `yaml:"keywords"`
	MetaTags          map[string]string `yaml:"meta_tags"`
	OpenGraph         OpenGraphConfig   `yaml:"opengraph"`
	Twitter           TwitterConfig     `yaml:"twitter"`
	Canonical         bool              `yaml:"canonical"` // Include canonical URLs
	NoIndex           bool              `yaml:"noindex"`   // Set noindex on search results
	Sitemap           bool              `yaml:"sitemap"`   // Generate sitemap.xml
}

// OpenGraphConfig represents OpenGraph meta tags
type OpenGraphConfig struct {
	Enabled     bool   `yaml:"enabled"`
	Type        string `yaml:"type"`
	SiteName    string `yaml:"site_name"`
	Image       string `yaml:"image"`
	Description string `yaml:"description"`
}

// TwitterConfig represents Twitter card meta tags
type TwitterConfig struct {
	Enabled     bool   `yaml:"enabled"`
	Card        string `yaml:"card"` // summary, summary_large_image
	Site        string `yaml:"site"` // @username
	Creator     string `yaml:"creator"`
}

// CompressionConfig represents HTTP response compression settings
type CompressionConfig struct {
	Enabled      bool     `yaml:"enabled"`
	Level        int      `yaml:"level"`         // 1-9, higher = more compression, more CPU
	MinSize      int      `yaml:"min_size"`      // Minimum response size to compress (bytes)
	MimeTypes    []string `yaml:"mime_types"`    // MIME types to compress
	Gzip         bool     `yaml:"gzip"`          // Enable gzip compression
	Brotli       bool     `yaml:"brotli"`        // Enable Brotli compression
	DisableProxy bool     `yaml:"disable_proxy"` // Disable compression for proxied requests
}

// LimitsConfig represents request limits configuration per AI.md PART 18
// Protects against DoS attacks (Slowloris, large uploads)
type LimitsConfig struct {
	MaxBodySize  string `yaml:"max_body_size"`  // Maximum request body size (e.g., "10MB")
	ReadTimeout  string `yaml:"read_timeout"`   // HTTP read timeout (e.g., "30s")
	WriteTimeout string `yaml:"write_timeout"`  // HTTP write timeout (e.g., "30s")
	IdleTimeout  string `yaml:"idle_timeout"`   // HTTP idle connection timeout (e.g., "120s")
}

// GetMaxBodySizeBytes parses MaxBodySize and returns bytes
func (l *LimitsConfig) GetMaxBodySizeBytes() int64 {
	if l.MaxBodySize == "" {
		return 10 * 1024 * 1024 // Default 10MB
	}
	size := l.MaxBodySize
	multiplier := int64(1)
	if len(size) > 2 {
		suffix := size[len(size)-2:]
		switch suffix {
		case "KB", "kb":
			multiplier = 1024
			size = size[:len(size)-2]
		case "MB", "mb":
			multiplier = 1024 * 1024
			size = size[:len(size)-2]
		case "GB", "gb":
			multiplier = 1024 * 1024 * 1024
			size = size[:len(size)-2]
		}
	}
	var n int64
	fmt.Sscanf(size, "%d", &n)
	return n * multiplier
}

// I18nConfig represents internationalization configuration
type I18nConfig struct {
	Enabled          bool     `yaml:"enabled"`
	DefaultLanguage  string   `yaml:"default_language"`  // BCP 47 language tag (e.g., en, en-US, de)
	SupportedLanguages []string `yaml:"supported_languages"` // List of supported languages
	AutoDetect       bool     `yaml:"auto_detect"`       // Detect from Accept-Language header
	ShowSelector     bool     `yaml:"show_selector"`     // Show language selector in UI
	RTLLanguages     []string `yaml:"rtl_languages"`     // Right-to-left languages (ar, he, etc.)
	TranslationsDir  string   `yaml:"translations_dir"`  // Directory for translation files
	FallbackLanguage string   `yaml:"fallback_language"` // Fallback if requested language not available
}

// SearchConfig represents search configuration
type SearchConfig struct {
	SafeSearch        int              `yaml:"safe_search"`
	Autocomplete      string           `yaml:"autocomplete"`
	DefaultLang       string           `yaml:"default_lang"`
	DefaultCategories []string         `yaml:"default_categories"`
	ResultsPerPage    int              `yaml:"results_per_page"`
	Timeout           int              `yaml:"timeout"`
	MaxConcurrent     int              `yaml:"max_concurrent"`
	Bangs             BangsConfig      `yaml:"bangs"`
	OpenSearch        OpenSearchConfig `yaml:"opensearch"`
	Widgets           WidgetsConfig    `yaml:"widgets"`
}

// WidgetsConfig represents widget system configuration
type WidgetsConfig struct {
	Enabled        bool     `yaml:"enabled"`
	DefaultWidgets []string `yaml:"default_widgets"`
	CacheTTL       int      `yaml:"cache_ttl"` // seconds

	Weather WeatherWidgetConfig `yaml:"weather"`
	News    NewsWidgetConfig    `yaml:"news"`
	Stocks  StocksWidgetConfig  `yaml:"stocks"`
	Crypto  CryptoWidgetConfig  `yaml:"crypto"`
	Sports  SportsWidgetConfig  `yaml:"sports"`
	RSS     RSSWidgetConfig     `yaml:"rss"`
}

// WeatherWidgetConfig holds weather widget configuration
type WeatherWidgetConfig struct {
	Enabled     bool   `yaml:"enabled"`
	DefaultCity string `yaml:"default_city"`
	Units       string `yaml:"units"` // "metric" or "imperial"
}

// NewsWidgetConfig holds news widget configuration
type NewsWidgetConfig struct {
	Enabled  bool     `yaml:"enabled"`
	Sources  []string `yaml:"sources"` // RSS feed URLs
	MaxItems int      `yaml:"max_items"`
}

// StocksWidgetConfig holds stocks widget configuration
type StocksWidgetConfig struct {
	Enabled        bool     `yaml:"enabled"`
	DefaultSymbols []string `yaml:"default_symbols"`
}

// CryptoWidgetConfig holds crypto widget configuration
type CryptoWidgetConfig struct {
	Enabled      bool     `yaml:"enabled"`
	DefaultCoins []string `yaml:"default_coins"`
	Currency     string   `yaml:"currency"` // "usd", "eur", etc.
}

// SportsWidgetConfig holds sports widget configuration
type SportsWidgetConfig struct {
	Enabled        bool     `yaml:"enabled"`
	DefaultLeagues []string `yaml:"default_leagues"`
}

// RSSWidgetConfig holds RSS widget configuration
type RSSWidgetConfig struct {
	Enabled  bool `yaml:"enabled"`
	MaxFeeds int  `yaml:"max_feeds"`
	MaxItems int  `yaml:"max_items"`
}

// BangsConfig represents bang configuration
type BangsConfig struct {
	Enabled       bool         `yaml:"enabled"`
	ProxyRequests bool         `yaml:"proxy_requests"`
	Custom        []BangConfig `yaml:"custom"`
}

// OpenSearchConfig represents OpenSearch configuration
type OpenSearchConfig struct {
	Enabled     bool   `yaml:"enabled"`
	ShortName   string `yaml:"short_name"`
	Description string `yaml:"description"`
	Contact     string `yaml:"contact"`
	Tags        string `yaml:"tags"`
	LongName    string `yaml:"long_name"`
	Image       string `yaml:"image"`
}

// BangConfig represents a custom bang configuration
type BangConfig struct {
	Shortcut    string   `yaml:"shortcut"`
	Name        string   `yaml:"name"`
	URL         string   `yaml:"url"`
	Category    string   `yaml:"category"`
	Description string   `yaml:"description,omitempty"`
	Aliases     []string `yaml:"aliases,omitempty"`
}

// EngineConfig represents search engine configuration
type EngineConfig struct {
	Enabled    bool     `yaml:"enabled"`
	Priority   int      `yaml:"priority"`
	Categories []string `yaml:"categories"`
	Timeout    int      `yaml:"timeout"`
	Weight     float64  `yaml:"weight"`
	APIKey     string   `yaml:"api_key,omitempty"`
}

// DefaultConfig returns a default configuration
func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Title:       "Scour",
			Description: "Privacy-Respecting Metasearch Engine",
			Port:        64580,
			Address:     "[::]",
			Mode:        "production",
			SecretKey:   generateSecret(),
			BaseURL:     "https://scour.li",
			SSL: SSLConfig{
				Enabled: false,
				AutoTLS: false,
			},
			Admin: AdminConfig{
				Enabled:  true,
				Username: "administrator",
				Password: generateSecret(),
				Token:    generateSecret(),
				Email:    "",
			},
			Branding: BrandingConfig{
				AppName:      "Scour",
				Theme:        "dark",
				PrimaryColor: "#bd93f9",
			},
			RateLimit: RateLimitConfig{
				Enabled:           true,
				RequestsPerMinute: 60,
				RequestsPerHour:   1000,
				RequestsPerDay:    10000,
				BurstSize:         10,
				ByIP:              true,
				ByUser:            false,
				Whitelist:         []string{"127.0.0.1", "::1"},
			},
			Session: SessionConfig{
				Admin: SessionTypeConfig{
					CookieName:  "admin_session",
					MaxAge:      2592000, // 30 days
					IdleTimeout: 86400,   // 24 hours
				},
				User: SessionTypeConfig{
					CookieName:  "user_session",
					MaxAge:      604800, // 7 days
					IdleTimeout: 86400,  // 24 hours
				},
				ExtendOnActivity: true,
				Secure:           "auto",
				HTTPOnly:         true,
				SameSite:         "lax",
			},
			Logs: LogsConfig{
				Level: "warn",
				Access: struct {
					Filename string `yaml:"filename"`
					Format   string `yaml:"format"`
					Custom   string `yaml:"custom"`
					Rotate   string `yaml:"rotate"`
					Keep     string `yaml:"keep"`
				}{
					Filename: "access.log",
					Format:   "apache",
					Rotate:   "monthly",
					Keep:     "none",
				},
				Server: struct {
					Filename string `yaml:"filename"`
					Format   string `yaml:"format"`
					Custom   string `yaml:"custom"`
					Rotate   string `yaml:"rotate"`
					Keep     string `yaml:"keep"`
				}{
					Filename: "server.log",
					Format:   "text",
					Rotate:   "weekly,50MB",
					Keep:     "none",
				},
				Error: struct {
					Filename string `yaml:"filename"`
					Format   string `yaml:"format"`
					Custom   string `yaml:"custom"`
					Rotate   string `yaml:"rotate"`
					Keep     string `yaml:"keep"`
				}{
					Filename: "error.log",
					Format:   "text",
					Rotate:   "weekly,50MB",
					Keep:     "none",
				},
				Audit: struct {
					Filename string `yaml:"filename"`
					Format   string `yaml:"format"`
					Custom   string `yaml:"custom"`
					Rotate   string `yaml:"rotate"`
					Keep     string `yaml:"keep"`
				}{
					Filename: "audit.log",
					Format:   "json",
					Rotate:   "weekly,50MB",
					Keep:     "none",
				},
				Security: struct {
					Filename string `yaml:"filename"`
					Format   string `yaml:"format"`
					Custom   string `yaml:"custom"`
					Rotate   string `yaml:"rotate"`
					Keep     string `yaml:"keep"`
				}{
					Filename: "security.log",
					Format:   "fail2ban",
					Rotate:   "weekly,50MB",
					Keep:     "none",
				},
				Debug: struct {
					Enabled  bool   `yaml:"enabled"`
					Filename string `yaml:"filename"`
					Format   string `yaml:"format"`
					Custom   string `yaml:"custom"`
					Rotate   string `yaml:"rotate"`
					Keep     string `yaml:"keep"`
				}{
					Enabled:  false,
					Filename: "debug.log",
					Format:   "text",
					Rotate:   "weekly,50MB",
					Keep:     "none",
				},
			},
			Tor: TorConfig{
				// Per AI.md PART 32: Enabled is auto-detected at runtime
				// Binary path auto-detected if empty
				HiddenServicePort: 80,
			},
			Email: EmailConfig{
				// Per AI.md PART 18: Enabled is auto-set based on SMTP availability
				SMTP: SMTPConfig{
					Port: 587,
					TLS:  "auto",
				},
				// From defaults applied at runtime based on config
			},
			Security: SecurityConfig{
				CORS: struct {
					Enabled        bool     `yaml:"enabled"`
					AllowedOrigins []string `yaml:"allowed_origins"`
					AllowedMethods []string `yaml:"allowed_methods"`
					AllowedHeaders []string `yaml:"allowed_headers"`
					AllowCredentials bool    `yaml:"allow_credentials"`
					MaxAge         int      `yaml:"max_age"`
				}{
					Enabled:        false,
					AllowedOrigins: []string{"*"},
					AllowedMethods: []string{"GET", "POST", "OPTIONS"},
					AllowedHeaders: []string{"Content-Type", "Authorization"},
					MaxAge:         86400,
				},
				CSRF: struct {
					Enabled    bool   `yaml:"enabled"`
					CookieName string `yaml:"cookie_name"`
					HeaderName string `yaml:"header_name"`
					FieldName  string `yaml:"field_name"`
				}{
					Enabled:    true,
					CookieName: "csrf_token",
					HeaderName: "X-CSRF-Token",
					FieldName:  "_csrf",
				},
				Headers: struct {
					XFrameOptions         string `yaml:"x_frame_options"`
					XContentTypeOptions   string `yaml:"x_content_type_options"`
					XXSSProtection        string `yaml:"x_xss_protection"`
					ReferrerPolicy        string `yaml:"referrer_policy"`
					ContentSecurityPolicy string `yaml:"content_security_policy"`
					PermissionsPolicy     string `yaml:"permissions_policy"`
				}{
					XFrameOptions:         "SAMEORIGIN",
					XContentTypeOptions:   "nosniff",
					XXSSProtection:        "1; mode=block",
					ReferrerPolicy:        "strict-origin-when-cross-origin",
					ContentSecurityPolicy: "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'",
					PermissionsPolicy:     "geolocation=(), microphone=(), camera=()",
				},
			},
			Users: UsersConfig{
				Enabled: true,
				Registration: struct {
					Enabled                bool     `yaml:"enabled"`
					RequireEmailVerification bool    `yaml:"require_email_verification"`
					RequireApproval        bool     `yaml:"require_approval"`
					AllowedDomains         []string `yaml:"allowed_domains"`
					BlockedDomains         []string `yaml:"blocked_domains"`
				}{
					Enabled:                false,
					RequireEmailVerification: true,
					RequireApproval:        false,
				},
				Roles: struct {
					Available []string `yaml:"available"`
					Default   string   `yaml:"default"`
				}{
					Available: []string{"admin", "user"},
					Default:   "user",
				},
				Tokens: struct {
					Enabled        bool `yaml:"enabled"`
					MaxPerUser     int  `yaml:"max_per_user"`
					ExpirationDays int  `yaml:"expiration_days"`
				}{
					Enabled:        true,
					MaxPerUser:     5,
					ExpirationDays: 0,
				},
				Profile: struct {
					AllowAvatar      bool `yaml:"allow_avatar"`
					AllowDisplayName bool `yaml:"allow_display_name"`
					AllowBio         bool `yaml:"allow_bio"`
				}{
					AllowAvatar:      true,
					AllowDisplayName: true,
					AllowBio:         true,
				},
				Auth: struct {
					SessionDuration          string `yaml:"session_duration"`
					SessionDurationDays      int    `yaml:"session_duration_days"`
					Require2FA               bool   `yaml:"require_2fa"`
					Allow2FA                bool   `yaml:"allow_2fa"`
					PasswordMinLength       int    `yaml:"password_min_length"`
					PasswordRequireUppercase bool   `yaml:"password_require_uppercase"`
					PasswordRequireNumber   bool   `yaml:"password_require_number"`
					PasswordRequireSpecial  bool   `yaml:"password_require_special"`
				}{
					SessionDuration:     "30d",
					SessionDurationDays: 30,
					Require2FA:          false,
					Allow2FA:          true,
					PasswordMinLength: 8,
				},
			},
			Pages: PagesConfig{
				About: struct {
					Enabled bool   `yaml:"enabled"`
					Content string `yaml:"content"`
				}{
					Enabled: true,
					Content: "Search is a privacy-respecting metasearch engine that aggregates results from multiple search engines without tracking you.",
				},
				Privacy: struct {
					Enabled bool   `yaml:"enabled"`
					Content string `yaml:"content"`
				}{
					Enabled: true,
					Content: "We don't track you. We don't store your searches. Your privacy is our priority.",
				},
				Contact: struct {
					Enabled bool   `yaml:"enabled"`
					Email   string `yaml:"email"`
				}{
					Enabled: true,
				},
				Help: struct {
					Enabled bool   `yaml:"enabled"`
					Content string `yaml:"content"`
				}{
					Enabled: true,
					Content: "How to use Search...",
				},
			},
			Web: WebConfig{
				Robots: struct {
					Allow []string `yaml:"allow"`
					Deny  []string `yaml:"deny"`
				}{
					Allow: []string{"/", "/api"},
					Deny:  []string{"/admin"},
				},
				Security: struct {
					Contact string `yaml:"contact"`
					Expires string `yaml:"expires"`
				}{
					Contact: "",
					Expires: time.Now().AddDate(1, 0, 0).Format("2006-01-02T15:04:05Z"),
				},
				Announcements: AnnouncementsConfig{
					Enabled:  true,
					Messages: []Announcement{},
				},
				CookieConsent: CookieConsentConfig{
					Enabled:   false,
					Message:   "This site uses cookies for functionality and analytics.",
					PolicyURL: "/server/privacy",
				},
				CORS: "*",
			},
			// Scheduler is ALWAYS RUNNING per AI.md PART 19
			Scheduler: SchedulerConfig{
				Timezone:      "America/New_York",
				CatchUpWindow: "1h",
				Tasks: SchedulerTasksConfig{
					BackupDaily:     TaskConfig{Schedule: "0 2 * * *", Enabled: true},
					BackupHourly:    TaskConfig{Schedule: "@hourly", Enabled: false},
					GeoIPUpdate:     TaskConfig{Schedule: "0 3 * * 0", Enabled: true},
					BlocklistUpdate: TaskConfig{Schedule: "0 4 * * *", Enabled: true},
					CVEUpdate:       TaskConfig{Schedule: "0 5 * * *", Enabled: true},
				},
			},
			Cache: CacheConfig{
				Type:     "memory",
				Host:     "localhost",
				Port:     6379,
				DB:       0,
				PoolSize: 10,
				MinIdle:  2,
				Timeout:  "5s",
				Prefix:   "apimgr:",
				TTL:      3600,
			},
			GeoIP: GeoIPConfig{
				Enabled: false,
				Dir:     "/data/geoip",
				Update:  "weekly",
				ASN:     true,
				Country: true,
				City:    false,
			},
			Metrics: MetricsConfig{
				Enabled:       false,
				Endpoint:      "/metrics",
				IncludeSystem: true,
				Token:         "",
			},
			ImageProxy: ImageProxyConfig{
				Enabled: false,
			},
			Contact: ContactConfig{
				Enabled: true,
			},
			SEO: SEOConfig{
				Enabled:           true,
				DefaultTitle:      "Search",
				TitleSeparator:    " - ",
				DefaultDescription: "A privacy-respecting metasearch engine",
				Keywords:          []string{"search", "privacy", "metasearch"},
				MetaTags:          map[string]string{},
				OpenGraph: OpenGraphConfig{
					Enabled:  true,
					Type:     "website",
					SiteName: "Search",
				},
				Twitter: TwitterConfig{
					Enabled: true,
					Card:    "summary",
				},
				Canonical: true,
				NoIndex:   true, // Don't index search results by default
				Sitemap:   false,
			},
			Compression: CompressionConfig{
				Enabled: true,
				Level:   6,
				MinSize: 1024, // Only compress responses > 1KB
				MimeTypes: []string{
					"text/html",
					"text/css",
					"text/javascript",
					"application/javascript",
					"application/json",
					"application/xml",
					"text/xml",
				},
				Gzip:         true,
				Brotli:       false, // Brotli requires additional CPU
				DisableProxy: false,
			},
			// Request limits per AI.md PART 18
			Limits: LimitsConfig{
				MaxBodySize:  "10MB",
				ReadTimeout:  "30s",
				WriteTimeout: "30s",
				IdleTimeout:  "120s",
			},
			I18n: I18nConfig{
				Enabled:            true,
				DefaultLanguage:    "en",
				SupportedLanguages: []string{"en", "de", "fr", "es", "it", "pt", "nl", "pl", "ru", "ja", "zh"},
				AutoDetect:         true,
				ShowSelector:       true,
				RTLLanguages:       []string{"ar", "he", "fa", "ur"},
				TranslationsDir:    "/data/translations",
				FallbackLanguage:   "en",
			},
		},
		Search: SearchConfig{
			SafeSearch:        1,
			Autocomplete:      "",
			DefaultLang:       "en",
			DefaultCategories: []string{"general"},
			ResultsPerPage:    10,
			Timeout:           10,
			MaxConcurrent:     5,
			Bangs: BangsConfig{
				Enabled:       true,
				ProxyRequests: true,
				Custom:        []BangConfig{},
			},
			OpenSearch: OpenSearchConfig{
				Enabled:     true,
				ShortName:   "",   // Uses server.title if empty
				Description: "",   // Uses server.description if empty
				Tags:        "search privacy metasearch",
				LongName:    "",   // Uses server.title if empty
				Image:       "/static/img/favicon.png",
			},
			Widgets: WidgetsConfig{
				Enabled:        true,
				DefaultWidgets: []string{"clock", "weather", "quicklinks", "calculator"},
				CacheTTL:       300, // 5 minutes
				Weather: WeatherWidgetConfig{
					Enabled:     true,
					DefaultCity: "",
					Units:       "metric",
				},
				News: NewsWidgetConfig{
					Enabled:  true,
					Sources:  []string{},
					MaxItems: 10,
				},
				Stocks: StocksWidgetConfig{
					Enabled:        true,
					DefaultSymbols: []string{"AAPL", "GOOGL", "MSFT"},
				},
				Crypto: CryptoWidgetConfig{
					Enabled:      true,
					DefaultCoins: []string{"bitcoin", "ethereum"},
					Currency:     "usd",
				},
				Sports: SportsWidgetConfig{
					Enabled:        false,
					DefaultLeagues: []string{},
				},
				RSS: RSSWidgetConfig{
					Enabled:  true,
					MaxFeeds: 5,
					MaxItems: 10,
				},
			},
		},
		Engines: map[string]EngineConfig{
			"duckduckgo": {
				Enabled:    true,
				Priority:   100,
				Categories: []string{"general", "images"},
				Timeout:    10,
				Weight:     1.0,
			},
			"google": {
				Enabled:    true,
				Priority:   90,
				Categories: []string{"general", "images", "news", "videos"},
				Timeout:    10,
				Weight:     1.0,
			},
			"bing": {
				Enabled:    true,
				Priority:   80,
				Categories: []string{"general", "images", "news", "videos"},
				Timeout:    10,
				Weight:     1.0,
			},
			"brave": {
				Enabled:    true,
				Priority:   75,
				Categories: []string{"general", "images", "news"},
				Timeout:    10,
				Weight:     1.0,
			},
			"qwant": {
				Enabled:    true,
				Priority:   70,
				Categories: []string{"general", "images", "news"},
				Timeout:    10,
				Weight:     1.0,
			},
			"startpage": {
				Enabled:    true,
				Priority:   68,
				Categories: []string{"general", "images"},
				Timeout:    10,
				Weight:     1.0,
			},
			"yahoo": {
				Enabled:    true,
				Priority:   65,
				Categories: []string{"general", "images", "news"},
				Timeout:    10,
				Weight:     1.0,
			},
			"wikipedia": {
				Enabled:    true,
				Priority:   60,
				Categories: []string{"general"},
				Timeout:    10,
				Weight:     0.5,
			},
			"stackoverflow": {
				Enabled:    true,
				Priority:   55,
				Categories: []string{"general", "code"},
				Timeout:    10,
				Weight:     0.8,
			},
			"github": {
				Enabled:    true,
				Priority:   50,
				Categories: []string{"general", "code"},
				Timeout:    10,
				Weight:     0.8,
			},
			"reddit": {
				Enabled:    true,
				Priority:   45,
				Categories: []string{"general", "social"},
				Timeout:    10,
				Weight:     0.7,
			},
		},
	}
}

// Load loads configuration from file
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	// Store path for reload
	cfg.configPath = path

	return &cfg, nil
}

// Save saves configuration to file
func (c *Config) Save(path string) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	data, err := yaml.Marshal(c)
	if err != nil {
		return err
	}

	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	return os.WriteFile(path, data, 0600)
}

// LoadOrCreate loads configuration from file or creates default if not exists
// Per AI.md: If server.yaml found, auto-migrate to server.yml on startup
func LoadOrCreate(path string) (*Config, bool, error) {
	// Check for .yaml → .yml migration per AI.md PART 3
	if strings.HasSuffix(path, ".yml") {
		yamlPath := strings.TrimSuffix(path, ".yml") + ".yaml"
		if _, err := os.Stat(yamlPath); err == nil {
			// .yaml file exists, migrate to .yml
			if err := migrateYamlToYml(yamlPath, path); err != nil {
				return nil, false, fmt.Errorf("failed to migrate %s to %s: %w", yamlPath, path, err)
			}
		}
	}

	// Try to load existing config
	cfg, err := Load(path)
	if err == nil {
		return cfg, false, nil
	}

	// If file doesn't exist, create default
	if os.IsNotExist(err) {
		cfg = DefaultConfig()
		cfg.configPath = path // Store path for reload
		if err := cfg.Save(path); err != nil {
			return nil, false, err
		}
		return cfg, true, nil
	}

	return nil, false, err
}

// migrateYamlToYml migrates a .yaml config file to .yml format
// Per AI.md PART 3: Auto-migrate .yaml to .yml on startup
func migrateYamlToYml(yamlPath, ymlPath string) error {
	// Read the .yaml file
	data, err := os.ReadFile(yamlPath)
	if err != nil {
		return fmt.Errorf("failed to read yaml file: %w", err)
	}

	// Write to .yml file
	if err := os.WriteFile(ymlPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write yml file: %w", err)
	}

	// Rename old file to .yaml.bak (don't delete in case of issues)
	backupPath := yamlPath + ".bak"
	if err := os.Rename(yamlPath, backupPath); err != nil {
		// Non-fatal: file was copied successfully
		_ = err
	}

	return nil
}

// ApplyEnv applies environment variable overrides to config
func (c *Config) ApplyEnv(env *EnvConfig) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if env.InstanceName != "" {
		c.Server.Title = env.InstanceName
	}
	if env.Secret != "" {
		c.Server.SecretKey = env.Secret
	}
	if env.Port != "" {
		// Convert port string to int
		var port int
		fmt.Sscanf(env.Port, "%d", &port)
		if port > 0 {
			c.Server.Port = port
		}
	}
	if env.Mode != "" {
		c.Server.Mode = env.GetMode()
	}
	if env.BaseURL != "" {
		c.Server.BaseURL = env.BaseURL
	}
	if env.Autocomplete != "" {
		c.Search.Autocomplete = env.Autocomplete
	}

	// Image proxy
	if env.ImageProxyURL != "" {
		c.Server.ImageProxy.Enabled = true
		c.Server.ImageProxy.URL = env.ImageProxyURL
	}
	if env.ImageProxyKey != "" {
		c.Server.ImageProxy.Key = env.ImageProxyKey
	}

	// Tor: Per AI.md PART 32, Tor is auto-enabled if binary found
	// No env vars needed - detection happens at runtime in TorService.Start()

	// Engines
	if c.Engines != nil {
		if engine, ok := c.Engines["google"]; ok {
			engine.Enabled = env.EnableGoogle
			c.Engines["google"] = engine
		}
		if engine, ok := c.Engines["duckduckgo"]; ok {
			engine.Enabled = env.EnableDuckDuckGo
			c.Engines["duckduckgo"] = engine
		}
		if engine, ok := c.Engines["bing"]; ok {
			engine.Enabled = env.EnableBing
			c.Engines["bing"] = engine
		}
	}
}

// generateSecret generates a random secret key
func generateSecret() string {
	b := make([]byte, 32)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// GetRandomPort returns a random port in the 64xxx range (64000-64999)
// Per spec, port 0 means random port in 64xxx range to avoid conflicts
func GetRandomPort() int {
	var b [2]byte
	rand.Read(b[:])
	// Generate random number 0-999 and add to 64000
	return 64000 + int(binary.LittleEndian.Uint16(b[:]))%1000
}

// ResolvePort resolves the port, using random port if 0
func ResolvePort(port int) int {
	if port == 0 {
		return GetRandomPort()
	}
	return port
}

// IsDualPortMode returns true if both HTTP and HTTPS ports are configured
func (c *ServerConfig) IsDualPortMode() bool {
	return c.Port > 0 && c.HTTPSPort > 0
}

// GetHTTPPort returns the HTTP port, resolving random if needed
func (c *ServerConfig) GetHTTPPort() int {
	return ResolvePort(c.Port)
}

// GetHTTPSPort returns the HTTPS port, resolving random if needed
func (c *ServerConfig) GetHTTPSPort() int {
	if c.HTTPSPort == 0 {
		return 0
	}
	return ResolvePort(c.HTTPSPort)
}

// GetConfigPath returns the path to the configuration file
func GetConfigPath() string {
	env := LoadFromEnv()

	// Use environment variable if set
	if env.SettingsPath != "" {
		return env.SettingsPath
	}

	// Use config directory
	configDir := env.ConfigDir
	if configDir == "" {
		configDir = GetConfigDir()
	}

	return filepath.Join(configDir, "server.yml")
}

// Initialize initializes the configuration system
// Per AI.md PART 3: Environment variables only work on first run
func Initialize() (*Config, error) {
	// Ensure directories exist
	if err := EnsureDirectories(); err != nil {
		return nil, fmt.Errorf("failed to create directories: %w", err)
	}

	// Load environment variables
	env := LoadFromEnv()

	// Get config path
	configPath := GetConfigPath()

	// Load or create config (handles .yaml to .yml migration internally)
	cfg, created, err := LoadOrCreate(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	// Per AI.md PART 3: Environment overrides only apply on first run
	// After config exists, environment variables are ignored
	if created {
		// Apply environment overrides only on first run
		cfg.ApplyEnv(env)

		// Save config with env overrides applied
		if err := cfg.Save(configPath); err != nil {
			// Log but don't print to console - banner handles output
			_ = err
		}

		// Mark as first run - banner will show setup token
		// Per AI.md PART 14: Admin credentials NOT auto-generated
		// User creates admin via setup wizard with setup token
		cfg.firstRun = true
	}

	return cfg, nil
}

// IsDevelopment returns true if in development mode
func (c *Config) IsDevelopment() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	mode := c.Server.Mode
	return mode == "development" || mode == "dev"
}

// IsProduction returns true if in production mode
func (c *Config) IsProduction() bool {
	return !c.IsDevelopment()
}

// IsDebug returns true if debug mode is enabled via DEBUG=true environment variable
// or if server mode is set to "debug"
func (c *Config) IsDebug() bool {
	// Check environment variable
	debug := os.Getenv("DEBUG")
	if debug == "true" || debug == "1" || debug == "yes" {
		return true
	}
	// Check server mode
	return c.Server.Mode == "debug"
}

// GetAddress returns the full bind address
func (c *Config) GetAddress() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return fmt.Sprintf("%s:%d", c.Server.Address, c.Server.Port)
}

// Get returns a read-locked copy of server config
func (c *Config) Get() ServerConfig {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.Server
}

// Update allows thread-safe config updates
func (c *Config) Update(fn func(*ServerConfig)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	fn(&c.Server)
}

// GetEncryptionKey returns a 32-byte encryption key derived from the SecretKey
// Uses SHA256 to ensure consistent 32-byte length for AES-256
func (c *Config) GetEncryptionKey() []byte {
	c.mu.RLock()
	secret := c.Server.SecretKey
	c.mu.RUnlock()

	if secret == "" {
		return nil
	}

	// Use SHA256 to derive a consistent 32-byte key
	h := sha256.Sum256([]byte(secret))
	return h[:]
}

// ValidationWarning represents a configuration validation warning
// Per AI.md PART 12: Config validation should warn and use defaults, not error
type ValidationWarning struct {
	Field   string
	Message string
	Default interface{}
}

// ValidateAndApplyDefaults validates configuration and applies defaults
// Per AI.md PART 12: Config validation should warn and use defaults, NOT error
// Returns list of warnings (errors are logged but defaults are applied)
func (c *Config) ValidateAndApplyDefaults() []ValidationWarning {
	c.mu.Lock()
	defer c.mu.Unlock()

	var warnings []ValidationWarning

	// Server title
	if c.Server.Title == "" {
		warnings = append(warnings, ValidationWarning{
			Field:   "server.title",
			Message: "Title is empty, using default",
			Default: "Search",
		})
		c.Server.Title = "Search"
	}

	// Port validation
	if c.Server.Port < 0 || c.Server.Port > 65535 {
		warnings = append(warnings, ValidationWarning{
			Field:   "server.port",
			Message: fmt.Sprintf("Invalid port %d, using default", c.Server.Port),
			Default: 64580,
		})
		c.Server.Port = 64580
	}

	// HTTPS port validation (if set)
	if c.Server.HTTPSPort != 0 && (c.Server.HTTPSPort < 0 || c.Server.HTTPSPort > 65535) {
		warnings = append(warnings, ValidationWarning{
			Field:   "server.https_port",
			Message: fmt.Sprintf("Invalid HTTPS port %d, disabling", c.Server.HTTPSPort),
			Default: 0,
		})
		c.Server.HTTPSPort = 0
	}

	// Mode validation
	mode := strings.ToLower(c.Server.Mode)
	if mode != "production" && mode != "development" && mode != "dev" && mode != "" {
		warnings = append(warnings, ValidationWarning{
			Field:   "server.mode",
			Message: fmt.Sprintf("Unknown mode '%s', using production", c.Server.Mode),
			Default: "production",
		})
		c.Server.Mode = "production"
	}
	if c.Server.Mode == "" {
		c.Server.Mode = "production"
	}

	// Secret key
	if c.Server.SecretKey == "" {
		warnings = append(warnings, ValidationWarning{
			Field:   "server.secret_key",
			Message: "Secret key is empty, generating random key",
			Default: "<generated>",
		})
		c.Server.SecretKey = generateSecret()
	}

	// Rate limit validation
	if c.Server.RateLimit.Enabled {
		if c.Server.RateLimit.RequestsPerMinute <= 0 {
			warnings = append(warnings, ValidationWarning{
				Field:   "server.rate_limit.requests_per_minute",
				Message: "Invalid rate limit, using default",
				Default: 30,
			})
			c.Server.RateLimit.RequestsPerMinute = 30
		}
		if c.Server.RateLimit.BurstSize <= 0 {
			c.Server.RateLimit.BurstSize = 10
		}
	}

	// Session configuration
	if c.Server.Session.Admin.MaxAge <= 0 {
		c.Server.Session.Admin.MaxAge = 2592000 // 30 days
	}
	if c.Server.Session.User.MaxAge <= 0 {
		c.Server.Session.User.MaxAge = 604800 // 7 days
	}

	// GeoIP configuration - just ensure dir is set
	if c.Server.GeoIP.Enabled && c.Server.GeoIP.Dir == "" {
		c.Server.GeoIP.Dir = GetGeoIPDir()
	}

	// Tor: Per AI.md PART 32, auto-enabled at runtime if binary found
	// No validation needed - TorService handles everything

	// Metrics configuration
	if c.Server.Metrics.Enabled && c.Server.Metrics.Endpoint == "" {
		c.Server.Metrics.Endpoint = "/metrics"
	}

	// Compression configuration
	if c.Server.Compression.Level < 1 || c.Server.Compression.Level > 9 {
		if c.Server.Compression.Level != 0 {
			warnings = append(warnings, ValidationWarning{
				Field:   "server.compression.level",
				Message: fmt.Sprintf("Invalid compression level %d, using default", c.Server.Compression.Level),
				Default: 6,
			})
		}
		c.Server.Compression.Level = 6
	}

	// Engines validation
	if len(c.Engines) == 0 {
		warnings = append(warnings, ValidationWarning{
			Field:   "engines",
			Message: "No search engines configured, using defaults",
			Default: "duckduckgo, google, bing",
		})
		c.Engines = DefaultConfig().Engines
	}

	// Validate each engine
	for name, engine := range c.Engines {
		if engine.Timeout <= 0 {
			engine.Timeout = 10
			c.Engines[name] = engine
		}
		if engine.Priority <= 0 && engine.Enabled {
			engine.Priority = 50
			c.Engines[name] = engine
		}
	}

	return warnings
}

// LogValidationWarnings prints validation warnings to stdout
// Per AI.md PART 12: Warn and use defaults, not error
func LogValidationWarnings(warnings []ValidationWarning) {
	if len(warnings) == 0 {
		return
	}

	fmt.Printf("⚠️  Configuration warnings (%d):\n", len(warnings))
	for _, w := range warnings {
		fmt.Printf("   • %s: %s (default: %v)\n", w.Field, w.Message, w.Default)
	}
	fmt.Println()
}
