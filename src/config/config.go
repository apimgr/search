package config

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/apimgr/search/src/display"
	"github.com/fsnotify/fsnotify"
	"gopkg.in/yaml.v3"
)

// Build info - set via -ldflags at build time
// Per AI.md PART 26: LDFLAGS must include Version, CommitID, BuildDate, OfficialSite
var (
	Version   = "dev"
	CommitID  = "unknown"
	BuildDate = "unknown"
	// Default, can be overridden via -ldflags
	OfficialSite = "https://search.apimgr.us"
)

// debugOverride is set by the --debug CLI flag before config is loaded.
// It is applied to Config.debugEnabled in Initialize().
var debugOverride bool

// SetDebugOverride records the --debug CLI flag state before config is loaded.
// Call this from applyCliOverrides() when --debug is present.
func SetDebugOverride(enabled bool) {
	debugOverride = enabled
}

// Config represents the complete application configuration
type Config struct {
	mu sync.RWMutex
	// Path to config file for reload
	configPath string
	// True if this is first run (no config existed)
	firstRun bool
	// debugEnabled is set by the --debug CLI flag; takes precedence over DEBUG env var
	debugEnabled bool

	Server  ServerConfig            `yaml:"server"`
	Search  SearchConfig            `yaml:"search"`
	Engines map[string]EngineConfig `yaml:"engines"`
}

// IsFirstRun returns true if this is the first run (config was just created)
// Per AI.md PART 14: First run shows setup token for admin creation
func (c *Config) IsFirstRun() bool {
	return c.firstRun
}

// SetDebug explicitly enables or disables debug mode on this Config instance.
// Called when the --debug CLI flag is present; takes precedence over the DEBUG env var.
func (c *Config) SetDebug(enabled bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.debugEnabled = enabled
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
// Per AI.md PART 5: Unknown config keys are ERRORS, not ignored
func (c *Config) Reload() error {
	c.mu.RLock()
	path := c.configPath
	c.mu.RUnlock()

	if path == "" {
		return fmt.Errorf("config path not set, cannot reload")
	}

	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}
	defer file.Close()

	var newCfg Config
	decoder := yaml.NewDecoder(file)
	// Per AI.md PART 5: Unknown keys cause errors
	decoder.KnownFields(true)

	if err := decoder.Decode(&newCfg); err != nil {
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

// watcherDebounce is the delay between receiving a filesystem event and reloading
// the config. Multiple rapid writes (e.g. editor swap files) collapse into one reload.
const watcherDebounce = 250 * time.Millisecond

// StartWatcher watches the config file for changes and calls Reload() on write or create
// events. It debounces rapid events by 250 ms. The watcher stops when ctx is cancelled.
// Per AI.md PART 5: Hot reload — watch server.yml for changes, reload without restart.
func (c *Config) StartWatcher(ctx context.Context) error {
	path := c.GetPath()
	if path == "" {
		return fmt.Errorf("config path not set, cannot start watcher")
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("failed to create config file watcher: %w", err)
	}

	if err := watcher.Add(path); err != nil {
		watcher.Close()
		return fmt.Errorf("failed to watch config file %s: %w", path, err)
	}

	go func() {
		defer watcher.Close()

		var debounce *time.Timer

		for {
			select {
			case <-ctx.Done():
				if debounce != nil {
					debounce.Stop()
				}
				return

			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) {
					if debounce != nil {
						debounce.Stop()
					}
					debounce = time.AfterFunc(watcherDebounce, func() {
						if err := c.Reload(); err != nil {
							slog.Error("config hot-reload failed", "err", err, "path", path)
						} else {
							slog.Info("config reloaded", "path", path)
						}
					})
				}

			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				slog.Error("config watcher error", "err", err, "path", path)
			}
		}
	}()

	return nil
}

// ServerConfig represents server configuration
type ServerConfig struct {
	// Core settings
	Title       string `yaml:"title"`
	Description string `yaml:"description"`
	// HTTP port (or single port if HTTPSPort not set)
	Port int `yaml:"port"`
	// HTTPS port for dual port mode (optional)
	HTTPSPort int    `yaml:"https_port"`
	Address   string `yaml:"address"`
	Mode      string `yaml:"mode"`
	SecretKey string `yaml:"secret_key"`
	BaseURL   string `yaml:"base_url"`
	// Fully qualified domain name — auto-detected from host if empty
	FQDN string `yaml:"fqdn"`
	// API version prefix used in /api/{api_version}/ routes
	APIVersion string `yaml:"api_version"`
	// PID file path; "true" uses the default platform path, "false" disables
	PIDFile string `yaml:"pidfile"`
	// Daemonize on start — detach from terminal (false for modern service managers)
	Daemonize bool `yaml:"daemonize"`
	// Service user the binary runs as after privilege drop
	User string `yaml:"user"`
	// Service group the binary runs as after privilege drop
	Group string `yaml:"group"`

	// SSL/TLS
	SSL SSLConfig `yaml:"ssl"`

	// Operator/Server bearer token (Authorization: Bearer tok_...)
	// Auto-generated on first run if empty. Validated by SHA-256 comparison.
	// Per AI.md: two-tier auth — server.token + per-resource api_tokens.
	Token string `yaml:"token"`

	// Branding
	Branding BrandingConfig `yaml:"branding"`

	// Rate Limiting
	RateLimit RateLimitConfig `yaml:"rate_limit"`

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

	// Healthz controls whether /healthz is also registered at the root path
	Healthz HealthzConfig `yaml:"healthz"`

	// Maintenance mode - when enabled, shows maintenance page to all users
	MaintenanceMode bool `yaml:"maintenance_mode"`

	// Backup configuration per AI.md PART 22
	Backup BackupConfig `yaml:"backup"`

	// Compliance configuration per AI.md PART 22
	Compliance ComplianceConfig `yaml:"compliance"`

	// Database driver and connection configuration
	Database DatabaseDriverConfig `yaml:"database"`

	// Maintenance mode self-healing configuration
	Maintenance MaintenanceSelfHealConfig `yaml:"maintenance"`
}

// SSLConfig represents SSL/TLS configuration
type SSLConfig struct {
	Enabled     bool   `yaml:"enabled"`
	AutoTLS     bool   `yaml:"auto_tls"`
	CertFile    string `yaml:"cert_file"`
	KeyFile     string `yaml:"key_file"`
	LetsEncrypt struct {
		Enabled bool     `yaml:"enabled"`
		Email   string   `yaml:"email"`
		Domains []string `yaml:"domains"`
		Staging bool     `yaml:"staging"`
		// http-01, tls-alpn-01, dns-01
		Challenge string `yaml:"challenge"`
	} `yaml:"letsencrypt"`
	// DNS-01 provider configuration per AI.md PART 17
	DNS01 DNS01Config `yaml:"dns01"`
}

// DNS01Config represents DNS-01 ACME challenge configuration
// Per AI.md: ALL DNS providers are supported via go-acme/lego
type DNS01Config struct {
	// Provider identifier (cloudflare, route53, etc.)
	Provider string `yaml:"provider"`
	// AES-256-GCM encrypted JSON
	CredentialsEncrypted string `yaml:"credentials_encrypted"`
	// Timestamp of last successful validation
	ValidatedAt string `yaml:"validated_at"`
}

// BrandingConfig represents branding configuration
// Per AI.md PART 13/16: branding fields for healthz project info
type BrandingConfig struct {
	// Per PART 13: project.name source
	Title string `yaml:"title"`
	// Per PART 13: project.tagline source
	Tagline string `yaml:"tagline"`
	// Per PART 13: project.description source
	Description string `yaml:"description"`
	// Per AI.md: {PLATFORM_REPO_URL} - repository URL
	SourceCodeURL string `yaml:"source_code_url"`
	LogoURL       string `yaml:"logo_url"`
	FaviconURL    string `yaml:"favicon_url"`
	FooterText    string `yaml:"footer_text"`
	Theme         string `yaml:"theme"`
	PrimaryColor  string `yaml:"primary_color"`
}

// RateLimitConfig represents rate limiting configuration
type RateLimitConfig struct {
	Enabled           bool     `yaml:"enabled"`
	RequestsPerMinute int      `yaml:"requests_per_minute"`
	RequestsPerHour   int      `yaml:"requests_per_hour"`
	RequestsPerDay    int      `yaml:"requests_per_day"`
	BurstSize         int      `yaml:"burst_size"`
	ByIP              bool     `yaml:"by_ip"`
	ByUser            bool     `yaml:"by_user"`
	Whitelist         []string `yaml:"whitelist"`
	Blacklist         []string `yaml:"blacklist"`
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
	// Runtime state - NOT configurable
	// Computed at runtime when Tor binary is found
	Enabled bool `yaml:"-"`
	// Set at runtime when Tor starts
	OnionAddress string `yaml:"-"`

	// Binary path (empty = auto-detect)
	Binary string `yaml:"binary"`

	// --- Outbound Network Settings ---
	// Use Tor network for outbound connections (server-wide default)
	UseNetwork bool `yaml:"use_network"`
	// Allow users to set their own Tor network preference
	AllowUserPreference bool `yaml:"allow_user_preference"`

	// --- Performance Settings ---
	// Maximum circuits to keep open (1-128, default 32)
	MaxCircuits int `yaml:"max_circuits"`
	// Circuit timeout in seconds (10-300, default 60)
	CircuitTimeout int `yaml:"circuit_timeout"`
	// Bootstrap timeout in seconds (30-600, default 180)
	BootstrapTimeout int `yaml:"bootstrap_timeout"`

	// --- Security Settings ---
	// Scrub sensitive info from Tor logs (default true)
	SafeLogging bool `yaml:"safe_logging"`
	// Maximum concurrent streams per circuit (10-500, default 100)
	MaxStreamsPerCircuit int `yaml:"max_streams_per_circuit"`
	// Close circuit when stream limit exceeded (default true)
	CloseCircuitOnStreamLimit bool `yaml:"close_circuit_on_stream_limit"`

	// --- Bandwidth Settings ---
	// Maximum bandwidth rate per second (e.g., "1 MB", "500 KB")
	BandwidthRate string `yaml:"bandwidth_rate"`
	// Maximum bandwidth burst per second (e.g., "2 MB", "1 MB")
	BandwidthBurst string `yaml:"bandwidth_burst"`
	// Maximum monthly bandwidth (e.g., "100 GB", "50 GB", "unlimited")
	MaxMonthlyBandwidth string `yaml:"max_monthly_bandwidth"`

	// --- Hidden Service Settings ---
	// Number of introduction points (3-10, default 3)
	NumIntroPoints int `yaml:"num_intro_points"`
	// Virtual port for hidden service (1-65535, default 80)
	// Per AI.md PART 32: yaml key is "virtual_port"
	VirtualPort int `yaml:"virtual_port"`
}

// EmailConfig represents email/SMTP configuration
// Per AI.md PART 18: Nested SMTP and From blocks
type EmailConfig struct {
	// Enabled is auto-set based on SMTP availability (no manual toggle)
	// Computed, not stored
	Enabled bool            `yaml:"-"`
	SMTP    SMTPConfig      `yaml:"smtp"`
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
	TLS string `yaml:"tls"`
}

// EmailFromConfig represents the from address configuration
// Per AI.md PART 18: From name and email defaults
type EmailFromConfig struct {
	// Default: app title
	Name string `yaml:"name"`
	// Default: no-reply@{fqdn}
	Email string `yaml:"email"`
}

// SecurityConfig represents security configuration
type SecurityConfig struct {
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
		// Per AI.md PART 11: cross-origin isolation headers
		// Cross-Origin-Opener-Policy (default: unsafe-none)
		COOP string `yaml:"coop"`
		// Cross-Origin-Embedder-Policy (default: unsafe-none)
		COEP string `yaml:"coep"`
		// Cross-Origin-Resource-Policy (default: cross-origin)
		CORP string `yaml:"corp"`
		// Emit Origin-Agent-Cluster: ?1 (always on per spec)
		OriginAgentCluster bool `yaml:"origin_agent_cluster"`
		// X-Permitted-Cross-Domain-Policies (default: none)
		CrossDomainPolicies string `yaml:"cross_domain_policies"`
	} `yaml:"headers"`
	// HSTS per AI.md PART 11
	HSTS struct {
		Enabled           bool `yaml:"enabled"`
		MaxAgeSeconds     int  `yaml:"max_age_seconds"`
		IncludeSubDomains bool `yaml:"include_subdomains"`
		Preload           bool `yaml:"preload"`
	} `yaml:"hsts"`
	// NEL (Network Error Logging) per AI.md PART 11
	NEL struct {
		Enabled           bool    `yaml:"enabled"`
		MaxAgeSeconds     int     `yaml:"max_age_seconds"`
		IncludeSubDomains bool    `yaml:"include_subdomains"`
		SampleRate        float64 `yaml:"sample_rate"`
	} `yaml:"nel"`
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
	GroupsClaim  string   `yaml:"groups_claim"`
	AutoCreate   bool     `yaml:"auto_create"`
}

// LDAPConfig represents LDAP authentication configuration
type LDAPConfig struct {
	ID              string `yaml:"id"`
	Name            string `yaml:"name"`
	Enabled         bool   `yaml:"enabled"`
	Host            string `yaml:"host"`
	Port            int    `yaml:"port"`
	UseTLS          bool   `yaml:"use_tls"`
	SkipTLSVerify   bool   `yaml:"skip_tls_verify"`
	BindDN          string `yaml:"bind_dn"`
	BindPassword    string `yaml:"bind_password"`
	BaseDN          string `yaml:"base_dn"`
	UserFilter      string `yaml:"user_filter"`
	UsernameAttr    string `yaml:"username_attr"`
	EmailAttr       string `yaml:"email_attr"`
	GroupFilter     string `yaml:"group_filter"`
	GroupMemberAttr string `yaml:"group_member_attr"`
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
		// Security contact email (mailto: prefix added automatically)
		Contact string `yaml:"contact"`
		// Expiration date (auto-calculated 1 year from now if not set)
		Expires string `yaml:"expires"`
	} `yaml:"security"`
	Announcements AnnouncementsConfig `yaml:"announcements"`
	CookieConsent CookieConsentConfig `yaml:"cookie_consent"`
	// "*", "origin1,origin2", or ""
	CORS string `yaml:"cors"`
}

// AnnouncementsConfig represents announcement settings (per AI.md)
type AnnouncementsConfig struct {
	Enabled  bool           `yaml:"enabled"`
	Messages []Announcement `yaml:"messages"`
}

// Announcement represents a single announcement message
type Announcement struct {
	ID string `yaml:"id"`
	// warning, info, error, success
	Type    string `yaml:"type"`
	Title   string `yaml:"title"`
	Message string `yaml:"message"`
	// ISO 8601 datetime
	Start string `yaml:"start"`
	// ISO 8601 datetime
	End string `yaml:"end"`
	// User can dismiss
	Dismissible bool `yaml:"dismissible"`
}

// CookieConsentConfig represents cookie consent popup settings
type CookieConsentConfig struct {
	Enabled   bool   `yaml:"enabled"`
	Message   string `yaml:"message"`
	PolicyURL string `yaml:"policy_url"`
	// Google Analytics ID
	TrackingID string `yaml:"tracking_id"`
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
				// Zero time means always started
				startTime = time.Time{}
			}
		}

		if a.End != "" {
			endTime, err = time.Parse(time.RFC3339, a.End)
			if err != nil {
				// Zero time means never ends
				endTime = time.Time{}
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
	// Cron expression or @every interval
	Schedule string `yaml:"schedule"`
	Enabled  bool   `yaml:"enabled"`
}

// CacheConfig represents cache configuration per AI.md PART 18
// Valkey/Redis is used as a local cache only — not for clustering (AI.md line 800)
type CacheConfig struct {
	// Type: none (disabled), memory (default), valkey, redis
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
	PoolSize int `yaml:"pool_size"`
	MinIdle  int `yaml:"min_idle"`
	// Connection timeout (e.g., "5s")
	Timeout string `yaml:"timeout"`

	// Key prefix to avoid collisions (use unique prefix per app)
	Prefix string `yaml:"prefix"`

	// Default TTL in seconds
	TTL int `yaml:"ttl"`
}

// GeoIPConfig represents GeoIP configuration (uses MMDB from sapics/ip-location-db)
// Per AI.md PART 20: GeoIP configuration
type GeoIPConfig struct {
	Enabled bool `yaml:"enabled"`
	// Directory for MMDB files
	Dir string `yaml:"dir"`
	// never, daily, weekly, monthly
	Update string `yaml:"update"`
	// Countries to block (ISO 3166-1 alpha-2)
	DenyCountries []string `yaml:"deny_countries"`
	// If set, only these countries allowed
	AllowedCountries []string `yaml:"allowed_countries"`
	// Database toggles per AI.md PART 20
	// Enable ASN lookups
	ASN bool `yaml:"asn"`
	// Enable country lookups
	Country bool `yaml:"country"`
	// Enable city lookups (larger download)
	City bool `yaml:"city"`
	// Enable WHOIS lookups
	WHOIS bool `yaml:"whois"`
}

// MetricsConfig represents Prometheus-compatible metrics configuration
// Per AI.md PART 21: Metrics configuration
type MetricsConfig struct {
	Enabled bool `yaml:"enabled"`
	// Endpoint path (default: /server/metrics)
	Endpoint string `yaml:"endpoint"`
	// Include system metrics (CPU, memory, disk)
	IncludeSystem bool `yaml:"include_system"`
	// Include Go runtime metrics
	IncludeRuntime bool `yaml:"include_runtime"`
	// Bearer token for authentication (empty = no auth)
	Token string `yaml:"token"`
	// Histogram buckets for request duration (seconds)
	DurationBuckets []float64 `yaml:"duration_buckets"`
	// Histogram buckets for request size (bytes)
	SizeBuckets []float64 `yaml:"size_buckets"`
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
	// true if password was set during setup
	Enabled bool `yaml:"enabled"`
	// Optional password hint (stored, NOT the password)
	Hint string `yaml:"hint"`
}

// BackupRetentionConfig represents backup retention policy
// Per AI.md PART 22: Retention settings
type BackupRetentionConfig struct {
	// Daily full backups to keep (default: 1)
	MaxBackups int `yaml:"max_backups"`
	// Weekly backups (Sunday) to keep (0 = disabled)
	KeepWeekly int `yaml:"keep_weekly"`
	// Monthly backups (1st) to keep (0 = disabled)
	KeepMonthly int `yaml:"keep_monthly"`
	// Yearly backups (Jan 1st) to keep (0 = disabled)
	KeepYearly int `yaml:"keep_yearly"`
}

// ComplianceConfig represents compliance mode configuration
// Per AI.md PART 22: When enabled, backup encryption is REQUIRED
type ComplianceConfig struct {
	// HIPAA, SOC2, etc. compliance mode
	Enabled bool `yaml:"enabled"`
}

// HealthzConfig controls the /healthz root-level alias per AI.md PART 13.
// The canonical endpoint is always /server/healthz; the root alias is opt-in.
type HealthzConfig struct {
	Root struct {
		// Enabled controls whether /healthz and /healthz.txt are registered.
		// Default: false (root alias disabled; use /server/healthz instead).
		Enabled bool `yaml:"enabled"`
	} `yaml:"root"`
}

// DatabaseDriverConfig represents database driver and connection configuration
// Per AI.md PART 5: server.database.driver and server.database.url
type DatabaseDriverConfig struct {
	// Driver: sqlite (default, pure Go) or libsql for remote Turso/libsql
	Driver string `yaml:"driver"`
	// URL: auto-created sqlite path for sqlite driver, or libsql://... for remote
	URL string `yaml:"url"`
}

// MaintenanceSelfHealConfig represents maintenance mode and self-healing configuration
// Per AI.md PART 5: server.maintenance block with self-healing settings
type MaintenanceSelfHealConfig struct {
	// SelfHealing retry settings
	SelfHealing MaintenanceSelfHealingConfig `yaml:"self_healing"`
	// Cleanup thresholds for auto-cleanup
	Cleanup MaintenanceCleanupConfig `yaml:"cleanup"`
	// Notification settings for maintenance transitions
	Notify MaintenanceNotifyConfig `yaml:"notify"`
}

// MaintenanceSelfHealingConfig holds retry settings for the self-healing loop
// Per AI.md PART 5: enabled, retry_interval, max_attempts
type MaintenanceSelfHealingConfig struct {
	// Enable automatic self-healing attempts
	Enabled bool `yaml:"enabled"`
	// Duration between retry attempts (e.g., "30s")
	RetryInterval string `yaml:"retry_interval"`
	// Maximum retry attempts before giving up; 0 = unlimited
	MaxAttempts int `yaml:"max_attempts"`
}

// MaintenanceCleanupConfig holds auto-cleanup thresholds used during maintenance
// Per AI.md PART 5: disk_threshold, log_retention_days, backup_keep_count
type MaintenanceCleanupConfig struct {
	// Start cleanup when disk usage exceeds this percentage
	DiskThreshold int `yaml:"disk_threshold"`
	// Delete logs older than this many days during cleanup
	LogRetentionDays int `yaml:"log_retention_days"`
	// Keep this many backups during cleanup
	BackupKeepCount int `yaml:"backup_keep_count"`
}

// MaintenanceNotifyConfig holds notification triggers for maintenance mode transitions
// Per AI.md PART 5: on_enter, on_exit
type MaintenanceNotifyConfig struct {
	// Send notification when entering maintenance mode
	OnEnter bool `yaml:"on_enter"`
	// Send notification when exiting maintenance mode
	OnExit bool `yaml:"on_exit"`
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
	Enabled            bool              `yaml:"enabled"`
	DefaultTitle       string            `yaml:"default_title"`
	TitleSeparator     string            `yaml:"title_separator"`
	DefaultDescription string            `yaml:"default_description"`
	Keywords           []string          `yaml:"keywords"`
	MetaTags           map[string]string `yaml:"meta_tags"`
	OpenGraph          OpenGraphConfig   `yaml:"opengraph"`
	Twitter            TwitterConfig     `yaml:"twitter"`
	// Include canonical URLs
	Canonical bool `yaml:"canonical"`
	// Set noindex on search results
	NoIndex bool `yaml:"noindex"`
	// Generate sitemap.xml
	Sitemap bool `yaml:"sitemap"`
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
	Enabled bool `yaml:"enabled"`
	// summary, summary_large_image
	Card string `yaml:"card"`
	// @username
	Site    string `yaml:"site"`
	Creator string `yaml:"creator"`
}

// CompressionConfig represents HTTP response compression settings
type CompressionConfig struct {
	Enabled bool `yaml:"enabled"`
	// 1-9, higher = more compression, more CPU
	Level int `yaml:"level"`
	// Minimum response size to compress (bytes)
	MinSize int `yaml:"min_size"`
	// MIME types to compress
	MimeTypes []string `yaml:"mime_types"`
	// Enable gzip compression
	Gzip bool `yaml:"gzip"`
	// Enable Brotli compression
	Brotli bool `yaml:"brotli"`
	// Disable compression for proxied requests
	DisableProxy bool `yaml:"disable_proxy"`
}

// LimitsConfig represents request limits configuration per AI.md PART 18
// Protects against DoS attacks (Slowloris, large uploads)
type LimitsConfig struct {
	// Maximum request body size (e.g., "10MB")
	MaxBodySize string `yaml:"max_body_size"`
	// HTTP read timeout (e.g., "30s")
	ReadTimeout string `yaml:"read_timeout"`
	// HTTP write timeout (e.g., "30s")
	WriteTimeout string `yaml:"write_timeout"`
	// HTTP idle connection timeout (e.g., "120s")
	IdleTimeout string `yaml:"idle_timeout"`
}

// GetMaxBodySizeBytes parses MaxBodySize and returns bytes
func (l *LimitsConfig) GetMaxBodySizeBytes() int64 {
	if l.MaxBodySize == "" {
		// Default 10MB
		return 10 * 1024 * 1024
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
	Enabled bool `yaml:"enabled"`
	// BCP 47 language tag (e.g., en, en-US, de)
	DefaultLanguage string `yaml:"default_language"`
	// List of supported languages
	SupportedLanguages []string `yaml:"supported_languages"`
	// Detect from Accept-Language header
	AutoDetect bool `yaml:"auto_detect"`
	// Show language selector in UI
	ShowSelector bool `yaml:"show_selector"`
	// Right-to-left languages (ar, he, etc.)
	RTLLanguages []string `yaml:"rtl_languages"`
	// Directory for translation files
	TranslationsDir string `yaml:"translations_dir"`
	// Fallback if requested language not available
	FallbackLanguage string `yaml:"fallback_language"`
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
	Alerts            AlertsConfig     `yaml:"alerts"`
}

type AlertsConfig struct {
	CreateRateLimitPerHour   int    `yaml:"create_rate_limit_per_hour"`
	WebhookMaxRetries        int    `yaml:"webhook_max_retries"`
	WebhookRetryDelayMinutes int    `yaml:"webhook_retry_delay_minutes"`
	RetentionDays            int    `yaml:"retention_days"`
	DefaultFrequency         string `yaml:"default_frequency"`
	DefaultDeliverRSS        bool   `yaml:"default_deliver_rss"`
	DefaultDeliverWebhook    bool   `yaml:"default_deliver_webhook"`
}

// WidgetsConfig represents widget system configuration
type WidgetsConfig struct {
	Enabled        bool     `yaml:"enabled"`
	DefaultWidgets []string `yaml:"default_widgets"`
	// seconds
	CacheTTL int `yaml:"cache_ttl"`

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
	// "metric" or "imperial"
	Units string `yaml:"units"`
}

// NewsWidgetConfig holds news widget configuration
type NewsWidgetConfig struct {
	Enabled bool `yaml:"enabled"`
	// RSS feed URLs
	Sources  []string `yaml:"sources"`
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
	// "usd", "eur", etc.
	Currency string `yaml:"currency"`
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
			Port:        0,
			Address:     "[::]",
			Mode:        "production",
			SecretKey:   generateSecret(),
			// Empty = auto-detect from request headers
			BaseURL: "",
			// Empty = auto-detected from hostname at runtime
			FQDN: "",
			// API version prefix used in /api/{api_version}/ routes
			APIVersion: "v1",
			// "true" = create PID file at default platform path
			PIDFile:   "true",
			Daemonize: false,
			// System service user and group (auto-created by binary on first root run)
			User:  "search",
			Group: "search",
			SSL: SSLConfig{
				Enabled: false,
				AutoTLS: false,
			},
			// Operator/server bearer token. Treat like a root password.
			// SHA-256 compared against Authorization: Bearer header.
			Token: generateSecret(),
			Branding: BrandingConfig{
				Title:        "Scour",
				Tagline:      "Privacy-respecting metasearch",
				Description:  "A privacy-respecting metasearch engine",
				Theme:        "dark",
				PrimaryColor: "#bd93f9",
			},
			RateLimit: RateLimitConfig{
				Enabled:           true,
				RequestsPerMinute: 1000,
				RequestsPerHour:   20000,
				RequestsPerDay:    100000,
				BurstSize:         100,
				ByIP:              true,
				ByUser:            false,
				Whitelist:         []string{"127.0.0.1", "::1"},
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
				AllowUserPreference:       true,
				MaxCircuits:               32,
				CircuitTimeout:            60,
				BootstrapTimeout:          180,
				SafeLogging:               true,
				MaxStreamsPerCircuit:      100,
				CloseCircuitOnStreamLimit: true,
				BandwidthRate:             "1 MB",
				BandwidthBurst:            "2 MB",
				MaxMonthlyBandwidth:       "100 GB",
				NumIntroPoints:            3,
				VirtualPort:               80,
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
					COOP                  string `yaml:"coop"`
					COEP                  string `yaml:"coep"`
					CORP                  string `yaml:"corp"`
					OriginAgentCluster    bool   `yaml:"origin_agent_cluster"`
					CrossDomainPolicies   string `yaml:"cross_domain_policies"`
				}{
					XFrameOptions:         "DENY",
					XContentTypeOptions:   "nosniff",
					XXSSProtection:        "1; mode=block",
					ReferrerPolicy:        "strict-origin-when-cross-origin",
					ContentSecurityPolicy: "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; img-src * data: blob:; media-src *",
					// Per AI.md PART 11: full Permissions-Policy — locked tracking proposals,
					// spec-required features scoped to self, all sensors locked by default.
					PermissionsPolicy:   "accelerometer=(), ambient-light-sensor=(), attribution-reporting=(), battery=(), browsing-topics=(), camera=(), display-capture=(), geolocation=(), gyroscope=(), hid=(), idle-detection=(), interest-cohort=(), magnetometer=(), microphone=(), midi=(), screen-wake-lock=(), serial=(), usb=(), xr-spatial-tracking=(), autoplay=(self), encrypted-media=(self), fullscreen=(self), payment=(self), picture-in-picture=(self), publickey-credentials-get=(self), storage-access=(self), web-share=(self)",
					COOP:                "unsafe-none",
					COEP:                "unsafe-none",
					CORP:                "cross-origin",
					OriginAgentCluster:  true,
					CrossDomainPolicies: "none",
				},
				HSTS: struct {
					Enabled           bool `yaml:"enabled"`
					MaxAgeSeconds     int  `yaml:"max_age_seconds"`
					IncludeSubDomains bool `yaml:"include_subdomains"`
					Preload           bool `yaml:"preload"`
				}{
					Enabled:           true,
					MaxAgeSeconds:     63072000,
					IncludeSubDomains: true,
					Preload:           true,
				},
				NEL: struct {
					Enabled           bool    `yaml:"enabled"`
					MaxAgeSeconds     int     `yaml:"max_age_seconds"`
					IncludeSubDomains bool    `yaml:"include_subdomains"`
					SampleRate        float64 `yaml:"sample_rate"`
				}{
					Enabled:           true,
					MaxAgeSeconds:     2592000,
					IncludeSubDomains: true,
					SampleRate:        1.0,
				},
			},
			Pages: PagesConfig{
				About: struct {
					Enabled bool   `yaml:"enabled"`
					Content string `yaml:"content"`
				}{
					Enabled: true,
					Content: "",
				},
				Privacy: struct {
					Enabled bool   `yaml:"enabled"`
					Content string `yaml:"content"`
				}{
					Enabled: true,
					Content: "",
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
					Content: "",
				},
			},
			Web: WebConfig{
				Robots: struct {
					Allow []string `yaml:"allow"`
					Deny  []string `yaml:"deny"`
				}{
					Allow: []string{"/", "/api"},
					Deny:  []string{},
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
				Endpoint:      "/server/metrics",
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
				Enabled:            true,
				DefaultTitle:       "Search",
				TitleSeparator:     " - ",
				DefaultDescription: "A privacy-respecting metasearch engine",
				Keywords:           []string{"search", "privacy", "metasearch"},
				MetaTags:           map[string]string{},
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
				// Don't index search results by default
				NoIndex: true,
				Sitemap: false,
			},
			Compression: CompressionConfig{
				Enabled: true,
				Level:   6,
				// Only compress responses > 1KB
				MinSize: 1024,
				MimeTypes: []string{
					"text/html",
					"text/css",
					"text/javascript",
					"application/javascript",
					"application/json",
					"application/xml",
					"text/xml",
				},
				Gzip: true,
				// Brotli requires additional CPU
				Brotli:       false,
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
			// Per AI.md PART 13: /healthz root alias is opt-in; disabled by default
			Healthz: HealthzConfig{
				Root: struct {
					Enabled bool `yaml:"enabled"`
				}{Enabled: false},
			},
			Database: DatabaseDriverConfig{
				// Pure Go SQLite driver (CGO_ENABLED=0 compliant)
				Driver: "sqlite",
				// Empty = auto-generated from data dir at runtime
				URL: "",
			},
			Maintenance: MaintenanceSelfHealConfig{
				SelfHealing: MaintenanceSelfHealingConfig{
					Enabled:       true,
					RetryInterval: "30s",
					// 0 = unlimited retries
					MaxAttempts: 0,
				},
				Cleanup: MaintenanceCleanupConfig{
					DiskThreshold:    90,
					LogRetentionDays: 7,
					BackupKeepCount:  5,
				},
				Notify: MaintenanceNotifyConfig{
					OnEnter: true,
					OnExit:  true,
				},
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
				Enabled: true,
				// Uses server.title if empty
				ShortName: "",
				// Uses server.description if empty
				Description: "",
				Tags:        "search privacy metasearch",
				// Uses server.title if empty
				LongName: "",
				Image:    "/static/img/favicon.png",
			},
			Alerts: AlertsConfig{
				CreateRateLimitPerHour:   10,
				WebhookMaxRetries:        3,
				WebhookRetryDelayMinutes: 5,
				RetentionDays:            30,
				DefaultFrequency:         "daily",
				DefaultDeliverRSS:        true,
				DefaultDeliverWebhook:    false,
			},
			Widgets: WidgetsConfig{
				Enabled:        true,
				DefaultWidgets: []string{"clock", "weather", "quicklinks", "calculator"},
				// 5 minutes
				CacheTTL: 300,
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
				Categories: []string{"general", "it"},
				Timeout:    10,
				Weight:     0.8,
			},
			"github": {
				Enabled:    true,
				Priority:   50,
				Categories: []string{"general", "it"},
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
// Per AI.md PART 5: Unknown config keys are ERRORS, not ignored
func Load(path string) (*Config, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var cfg Config
	decoder := yaml.NewDecoder(file)
	// Per AI.md PART 5: Unknown keys cause errors
	decoder.KnownFields(true)

	if err := decoder.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	// Store path for reload
	cfg.configPath = path

	return &cfg, nil
}

// Save saves configuration to file with comments per AI.md config-rules.md
// Comments are placed ABOVE each setting, never inline
func (c *Config) Save(path string) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Marshal to yaml.Node for comment support
	var node yaml.Node
	if err := node.Encode(c); err != nil {
		return err
	}

	// Add comments to top-level sections
	addConfigComments(&node)

	// Encode with comments
	var buf strings.Builder
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&node); err != nil {
		return err
	}
	enc.Close()

	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	return os.WriteFile(path, []byte(buf.String()), 0600)
}

// addConfigComments adds comments to configuration node
// Per AI.md config-rules.md: Comments ABOVE the setting, never inline
func addConfigComments(node *yaml.Node) {
	if node.Kind != yaml.DocumentNode || len(node.Content) == 0 {
		return
	}

	root := node.Content[0]
	if root.Kind != yaml.MappingNode {
		return
	}

	// Section comments (key → comment)
	sectionComments := map[string]string{
		"server":  "Server configuration",
		"search":  "Search settings and behavior",
		"engines": "Search engine configurations",
	}

	// Subsection comments under server
	serverSubComments := map[string]string{
		"title":            "Server title displayed in UI",
		"description":      "Server description/tagline",
		"port":             "HTTP port (default: 64580)",
		"https_port":       "HTTPS port for dual-port mode (optional)",
		"address":          "Listen address ([::]  for all interfaces)",
		"mode":             "Application mode: production or development",
		"secret_key":       "Secret key for session encryption (auto-generated)",
		"base_url":         "Public URL for this instance",
		"ssl":              "SSL/TLS configuration",
		"token":            "Operator bearer token (server.token). Auto-generated on first run.",
		"branding":         "Branding and appearance",
		"rate_limit":       "Rate limiting configuration",
		"logs":             "Logging configuration",
		"tor":              "Tor hidden service settings",
		"email":            "Email/SMTP configuration",
		"security":         "Security settings",
		"auth":             "External authentication (OIDC/LDAP)",
		"users":            "User management settings",
		"pages":            "Custom pages configuration",
		"web":              "Web settings (robots.txt, security.txt)",
		"scheduler":        "Built-in task scheduler",
		"cache":            "Response caching",
		"geoip":            "GeoIP database settings",
		"metrics":          "Prometheus metrics endpoint",
		"image_proxy":      "Image proxy for privacy",
		"contact":          "Contact form settings",
		"seo":              "SEO and sitemap settings",
		"compression":      "Response compression",
		"limits":           "Request size limits",
		"i18n":             "Internationalization settings",
		"maintenance_mode": "Enable maintenance mode to show maintenance page",
		"backup":           "Backup configuration",
		"compliance":       "Compliance settings (GDPR, etc.)",
		"healthz":          "Healthz endpoint configuration (root alias /healthz)",
		"fqdn":             "Fully qualified domain name (auto-detected from hostname if empty)",
		"api_version":      "API version prefix used in /api/{api_version}/ routes",
		"pidfile":          "PID file: true = default platform path, false = disabled, or an explicit path",
		"daemonize":        "Daemonize on start (detach from terminal); false for modern service managers",
		"user":             "System user the binary runs as after privilege drop",
		"group":            "System group the binary runs as after privilege drop",
		"database":         "Database driver and connection settings",
		"maintenance":      "Maintenance mode self-healing configuration",
	}

	// Add comments to top-level sections
	for i := 0; i < len(root.Content)-1; i += 2 {
		key := root.Content[i]
		if comment, ok := sectionComments[key.Value]; ok {
			key.HeadComment = comment
		}

		// Add subsection comments for server section
		if key.Value == "server" {
			value := root.Content[i+1]
			if value.Kind == yaml.MappingNode {
				for j := 0; j < len(value.Content)-1; j += 2 {
					subKey := value.Content[j]
					if comment, ok := serverSubComments[subKey.Value]; ok {
						subKey.HeadComment = comment
					}
				}
			}
		}
	}
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
		// Store path for reload
		cfg.configPath = path
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

// ApplyEnv applies init-only environment variable overrides to config.
// Per AI.md PART 5: Init-only variables are used ONCE during first run, then ignored.
// This function is called only when created==true (first run).
func (c *Config) ApplyEnv(env *EnvConfig) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Per AI.md PART 5: Init-Only Variables (First Run Only)
	// CONFIG_DIR, DATA_DIR, LOG_DIR — handled by directory functions
	// DATABASE_DIR, BACKUP_DIR — directory overrides
	if env.DatabaseDir != "" {
		SetDatabaseDirOverride(env.DatabaseDir)
	}
	if env.BackupDir != "" {
		SetBackupDirOverride(env.BackupDir)
	}

	// PORT — server port
	if env.Port != "" {
		var port int
		fmt.Sscanf(env.Port, "%d", &port)
		if port > 0 {
			c.Server.Port = port
		}
	}

	// LISTEN — listen address
	if env.Listen != "" {
		c.Server.Address = env.Listen
	}

	// APPLICATION_NAME — application title
	if env.ApplicationName != "" {
		c.Server.Title = env.ApplicationName
		c.Server.Branding.Title = env.ApplicationName
	}

	// APPLICATION_TAGLINE — application description
	if env.ApplicationTagline != "" {
		c.Server.Branding.Tagline = env.ApplicationTagline
	}
}

// ApplyRuntimeEnv applies runtime environment variable overrides to config.
// Per AI.md PART 5: Runtime variables are always checked on every startup.
// NO_COLOR and TERM are checked directly where needed via IsNoColor()/IsDumbTerminal().
// DATABASE_DRIVER and DATABASE_URL are consumed directly by the database package
// via GetDatabaseDriver() and GetDatabaseURL().
func (c *Config) ApplyRuntimeEnv(env *EnvConfig) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// DOMAIN — FQDN override (highest priority for hostname resolution)
	// Per AI.md PART 5: Sets FQDN directly, not just BaseURL
	if env.Domain != "" {
		c.Server.FQDN = env.Domain
	}

	// MODE — production (default) or development
	if env.Mode != "" {
		c.Server.Mode = env.GetMode()
	}

	// SMTP_HOST — SMTP server hostname
	if env.SMTPHost != "" {
		c.Server.Email.SMTP.Host = env.SMTPHost
	}

	// SMTP_PORT — SMTP server port
	if env.SMTPPort != 0 {
		c.Server.Email.SMTP.Port = env.SMTPPort
	}

	// SMTP_USERNAME — SMTP authentication username
	if env.SMTPUsername != "" {
		c.Server.Email.SMTP.Username = env.SMTPUsername
	}

	// SMTP_PASSWORD — SMTP authentication password
	if env.SMTPPassword != "" {
		c.Server.Email.SMTP.Password = env.SMTPPassword
	}

	// SMTP_FROM_NAME — Sender name
	if env.SMTPFromName != "" {
		c.Server.Email.From.Name = env.SMTPFromName
	}

	// SMTP_FROM_EMAIL — Sender email
	if env.SMTPFromEmail != "" {
		c.Server.Email.From.Email = env.SMTPFromEmail
	}

	// SMTP_TLS — TLS mode: auto, starttls, tls, none
	if env.SMTPTLS != "" {
		c.Server.Email.SMTP.TLS = env.SMTPTLS
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

// ResolvePort resolves port 0 to a random 64xxx port; non-zero ports pass through.
// Containers use explicit port configuration (server.yml); the binary never auto-detects.
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

	// Use CONFIG_DIR env var if set (AI.md PART 5: init-only variable)
	configDir := env.ConfigDir
	if configDir == "" {
		configDir = GetConfigDir()
	}

	return filepath.Join(configDir, "server.yml")
}

// Initialize initializes the configuration system
// Per AI.md PART 5: Runtime env vars are always applied; init-only vars apply on first run only.
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

	// Per AI.md PART 5: Runtime env vars (MODE, DOMAIN, DATABASE_DRIVER, DATABASE_URL, SMTP_*)
	// are always applied on every startup, regardless of whether this is first run.
	cfg.ApplyRuntimeEnv(env)

	// Per AI.md PART 5: Init-only env vars (PORT, APPLICATION_NAME, CONFIG_DIR, etc.)
	// are applied only on first run, then persisted in server.yml.
	if created {
		cfg.ApplyEnv(env)

		// Per AI.md PART 5: First run selects a random port in 64000-64999 and persists it.
		// Port 0 means "not yet assigned"; resolve it now so the saved value is stable.
		if cfg.Server.Port == 0 {
			cfg.Server.Port = GetRandomPort()
		}

		// Save config with first-run overrides and resolved port applied
		if err := cfg.Save(configPath); err != nil {
			// Log but don't print to console - banner handles output
			_ = err
		}

		// Mark as first run - banner will show setup token
		// Per AI.md PART 14: Admin credentials NOT auto-generated
		// User creates admin via setup wizard with setup token
		cfg.firstRun = true
	}

	// Apply --debug CLI flag state recorded before config was loaded
	if debugOverride {
		cfg.debugEnabled = true
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

// IsDebug returns true if debug mode is enabled.
// Per AI.md PART 6: Priority is --debug CLI flag > DEBUG env var > default false.
// c.debugEnabled is set by the --debug flag; the env var is the fallback.
func (c *Config) IsDebug() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.debugEnabled || IsTruthy(os.Getenv("DEBUG"))
}

// Sanitized returns a copy of the config with all sensitive values redacted.
// Used by the debug /config endpoint.
func (c *Config) Sanitized() map[string]any {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return map[string]any{
		"mode":       c.Server.Mode,
		"port":       c.Server.Port,
		"address":    c.Server.Address,
		"title":      c.Server.Title,
		"token":      "xxxxx",
		"secret_key": "xxxxx",
		"ssl": map[string]any{
			"enabled": c.Server.SSL.Enabled,
		},
		"debug": c.debugEnabled || IsTruthy(os.Getenv("DEBUG")),
	}
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
				Default: 1000,
			})
			c.Server.RateLimit.RequestsPerMinute = 1000
		}
		if c.Server.RateLimit.BurstSize <= 0 {
			c.Server.RateLimit.BurstSize = 100
		}
	}

	// GeoIP configuration - just ensure dir is set
	if c.Server.GeoIP.Enabled && c.Server.GeoIP.Dir == "" {
		c.Server.GeoIP.Dir = GetGeoIPDir()
	}

	// Tor: Per AI.md PART 32, auto-enabled at runtime if binary found
	// No validation needed - TorService handles everything

	// Metrics configuration
	if c.Server.Metrics.Enabled && c.Server.Metrics.Endpoint == "" {
		c.Server.Metrics.Endpoint = "/server/metrics"
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

	fmt.Printf("%s  Configuration warnings (%d):\n", display.Emoji("⚠️", "[WARN]"), len(warnings))
	for _, w := range warnings {
		fmt.Printf("   • %s: %s (default: %v)\n", w.Field, w.Message, w.Default)
	}
	fmt.Println()
}
