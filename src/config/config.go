package config

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// Version info (set at build time)
var (
	Version   = "dev"
	BuildTime = ""
	GoVersion = ""
	GitCommit = ""
)

// Config represents the complete application configuration
type Config struct {
	mu         sync.RWMutex
	configPath string // Path to config file for reload

	Server  ServerConfig           `yaml:"server"`
	Search  SearchConfig           `yaml:"search"`
	Engines map[string]EngineConfig `yaml:"engines"`
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

	// Users
	Users UsersConfig `yaml:"users"`

	// Pages
	Pages PagesConfig `yaml:"pages"`

	// Web (robots.txt, security.txt)
	Web WebConfig `yaml:"web"`

	// Scheduler
	Scheduler SchedulerConfig `yaml:"scheduler"`

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

	// I18n (Internationalization)
	I18n I18nConfig `yaml:"i18n"`
}

// SSLConfig represents SSL/TLS configuration
type SSLConfig struct {
	Enabled     bool   `yaml:"enabled"`
	AutoTLS     bool   `yaml:"auto_tls"`
	CertFile    string `yaml:"cert_file"`
	KeyFile     string `yaml:"key_file"`
	LetsEncrypt struct {
		Enabled bool   `yaml:"enabled"`
		Email   string `yaml:"email"`
		Domains []string `yaml:"domains"`
		Staging bool   `yaml:"staging"`
	} `yaml:"letsencrypt"`
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

// SessionConfig represents session configuration
type SessionConfig struct {
	Duration       string `yaml:"duration"`
	Timeout        int    `yaml:"timeout"`
	CookieName     string `yaml:"cookie_name"`
	CookieSecure   bool   `yaml:"cookie_secure"`
	CookieHTTPOnly bool   `yaml:"cookie_http_only"`
	CookieSameSite string `yaml:"cookie_same_site"`
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
type TorConfig struct {
	Enabled           bool   `yaml:"enabled"`
	Binary            string `yaml:"binary"`
	SocksProxy        string `yaml:"socks_proxy"`
	SocksPort         int    `yaml:"socks_port"`
	ControlPort       int    `yaml:"control_port"`
	ControlPassword   string `yaml:"control_password"`
	StreamIsolation   bool   `yaml:"stream_isolation"`
	OnionAddress      string `yaml:"onion_address"`
	HiddenServicePort int    `yaml:"hidden_service_port"`
}

// EmailConfig represents email/SMTP configuration
type EmailConfig struct {
	Enabled     bool   `yaml:"enabled"`
	SMTPHost    string `yaml:"smtp_host"`
	SMTPPort    int    `yaml:"smtp_port"`
	SMTPUser    string `yaml:"smtp_user"`
	SMTPPass    string `yaml:"smtp_pass"`
	FromAddress string `yaml:"from_address"`
	FromName    string `yaml:"from_name"`
	TLS         bool   `yaml:"tls"`
	StartTLS    bool   `yaml:"starttls"`
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

// UsersConfig represents user management configuration
type UsersConfig struct {
	Enabled bool `yaml:"enabled"`
	Registration struct {
		Enabled                bool     `yaml:"enabled"`
		RequireEmailVerification bool    `yaml:"require_email_verification"`
		RequireApproval        bool     `yaml:"require_approval"`
		AllowedDomains         []string `yaml:"allowed_domains"`
		BlockedDomains         []string `yaml:"blocked_domains"`
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
		SessionDuration         string `yaml:"session_duration"`
		Require2FA              bool   `yaml:"require_2fa"`
		Allow2FA                bool   `yaml:"allow_2fa"`
		PasswordMinLength       int    `yaml:"password_min_length"`
		PasswordRequireUppercase bool   `yaml:"password_require_uppercase"`
		PasswordRequireNumber   bool   `yaml:"password_require_number"`
		PasswordRequireSpecial  bool   `yaml:"password_require_special"`
	} `yaml:"auth"`
	Limits struct {
		RequestsPerMinute int `yaml:"requests_per_minute"`
		RequestsPerDay    int `yaml:"requests_per_day"`
	} `yaml:"limits"`
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
}

// WebConfig represents web settings (robots.txt, security.txt, announcements)
type WebConfig struct {
	Robots struct {
		Allow []string `yaml:"allow"`
		Deny  []string `yaml:"deny"`
	} `yaml:"robots"`
	Security struct {
		Contact string `yaml:"contact"`
		Expires string `yaml:"expires"`
	} `yaml:"security"`
	Announcements AnnouncementsConfig `yaml:"announcements"`
	CookieConsent CookieConsentConfig `yaml:"cookie_consent"`
	CORS          string              `yaml:"cors"` // "*", "origin1,origin2", or ""
}

// AnnouncementsConfig represents announcement settings (per TEMPLATE.md)
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

// SchedulerConfig represents scheduler configuration
type SchedulerConfig struct {
	Enabled bool `yaml:"enabled"`
	Tasks []ScheduledTask `yaml:"tasks"`
}

// ScheduledTask represents a scheduled task
type ScheduledTask struct {
	ID       string `yaml:"id"`
	Name     string `yaml:"name"`
	Schedule string `yaml:"schedule"`
	Enabled  bool   `yaml:"enabled"`
	Command  string `yaml:"command"`
}

// GeoIPConfig represents GeoIP configuration (uses MMDB from sapics/ip-location-db)
type GeoIPConfig struct {
	Enabled          bool     `yaml:"enabled"`
	Dir              string   `yaml:"dir"`             // Directory for MMDB files
	Update           string   `yaml:"update"`          // never, daily, weekly, monthly
	DenyCountries    []string `yaml:"deny_countries"`  // Countries to block (ISO 3166-1 alpha-2)
	AllowedCountries []string `yaml:"allowed_countries"` // If set, only these countries allowed
	// Database toggles
	ASN     bool `yaml:"asn"`     // Enable ASN lookups
	Country bool `yaml:"country"` // Enable country lookups
	City    bool `yaml:"city"`    // Enable city lookups (larger download)
}

// MetricsConfig represents Prometheus-compatible metrics configuration
type MetricsConfig struct {
	Enabled       bool   `yaml:"enabled"`
	Path          string `yaml:"path"`           // Endpoint path (default: /metrics)
	IncludeSystem bool   `yaml:"include_system"` // Include system metrics (CPU, memory, disk)
	Token         string `yaml:"token"`          // Bearer token for authentication (empty = no auth)
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
			Port:        64080,
			Address:     "[::]",
			Mode:        "production",
			SecretKey:   generateSecret(),
			BaseURL:     "https://scour.li",
			SSL: SSLConfig{
				Enabled: false,
				AutoTLS: false,
			},
			Admin: AdminConfig{
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
			},
			Session: SessionConfig{
				Duration:       "30d",
				CookieName:     "search_session",
				CookieSecure:   true,
				CookieHTTPOnly: true,
				CookieSameSite: "Lax",
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
				Enabled:           false,
				SocksProxy:        "127.0.0.1:9050",
				SocksPort:         9050,
				ControlPort:       9051,
				StreamIsolation:   true,
				HiddenServicePort: 80,
			},
			Email: EmailConfig{
				Enabled:  false,
				SMTPPort: 587,
				TLS:      true,
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
				Enabled: false,
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
					SessionDuration         string `yaml:"session_duration"`
					Require2FA              bool   `yaml:"require_2fa"`
					Allow2FA                bool   `yaml:"allow_2fa"`
					PasswordMinLength       int    `yaml:"password_min_length"`
					PasswordRequireUppercase bool   `yaml:"password_require_uppercase"`
					PasswordRequireNumber   bool   `yaml:"password_require_number"`
					PasswordRequireSpecial  bool   `yaml:"password_require_special"`
				}{
					SessionDuration:   "30d",
					Require2FA:        false,
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
			Scheduler: SchedulerConfig{
				Enabled: true,
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
				Path:          "/metrics",
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
func LoadOrCreate(path string) (*Config, bool, error) {
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

	// Tor
	if env.UseTor {
		c.Server.Tor.Enabled = true
	}
	if env.TorProxy != "" {
		c.Server.Tor.SocksProxy = env.TorProxy
	}
	if env.TorControlPort != "" {
		if port := ParseInt(env.TorControlPort, 0); port > 0 {
			c.Server.Tor.ControlPort = port
		}
	}
	if env.TorControlPass != "" {
		c.Server.Tor.ControlPassword = env.TorControlPass
	}

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
func Initialize() (*Config, error) {
	// Ensure directories exist
	if err := EnsureDirectories(); err != nil {
		return nil, fmt.Errorf("failed to create directories: %w", err)
	}

	// Load environment variables
	env := LoadFromEnv()

	// Get config path
	configPath := GetConfigPath()

	// Load or create config
	cfg, created, err := LoadOrCreate(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	// Apply environment overrides
	cfg.ApplyEnv(env)

	// If config was created, show credentials
	if created {
		fmt.Println("✅ Configuration created:", configPath)
		fmt.Println()
		fmt.Println("🔐 Admin Credentials (save these now - shown only once):")
		fmt.Println("   Username:", cfg.Server.Admin.Username)
		fmt.Println("   Password:", cfg.Server.Admin.Password)
		fmt.Println("   Token:   ", cfg.Server.Admin.Token)
		fmt.Println()
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
