package server

import (
	"context"
	"database/sql"
	"fmt"
	"html"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/apimgr/search/src/admin"
	"github.com/apimgr/search/src/api"
	"github.com/apimgr/search/src/config"
	"github.com/apimgr/search/src/database"
	"github.com/apimgr/search/src/direct"
	"github.com/apimgr/search/src/email"
	"github.com/apimgr/search/src/geoip"
	"github.com/apimgr/search/src/i18n"
	"github.com/apimgr/search/src/instant"
	"github.com/apimgr/search/src/logging"
	"github.com/apimgr/search/src/model"
	"github.com/apimgr/search/src/scheduler"
	"github.com/apimgr/search/src/search"
	"github.com/apimgr/search/src/search/bangs"
	"github.com/apimgr/search/src/search/engines"
	"github.com/apimgr/search/src/service"
	"github.com/apimgr/search/src/ssl"
	"github.com/apimgr/search/src/user"
	"github.com/apimgr/search/src/widget"
)

// Server represents the HTTP server
type Server struct {
	config         *config.Config
	httpServer     *http.Server
	httpsServer    *http.Server // For dual port mode
	redirectServer *http.Server
	pidFile        string
	registry       *engines.Registry
	aggregator     *search.Aggregator
	startTime      time.Time
	middleware     *Middleware
	rateLimiter    *RateLimiter
	csrf           *CSRFMiddleware
	renderer       *TemplateRenderer
	adminHandler   *admin.Handler
	apiHandler     *api.Handler
	torService     *service.TorService
	bangManager    *bangs.Manager
	widgetManager  *widget.Manager
	logManager     *logging.Manager
	tlsManager     *ssl.Manager
	instantManager *instant.Manager
	directManager  *direct.Manager
	geoipLookup    *geoip.Lookup
	mailer         *email.Mailer
	scheduler      *scheduler.Scheduler
	metrics        *Metrics
	dbManager      *database.DatabaseManager
	configSync     *config.ConfigSync // Per AI.md PART 5: Cluster config sync (NON-NEGOTIABLE)

	// User management
	userAuthManager     *user.AuthManager
	totpManager         *user.TOTPManager
	recoveryManager     *user.RecoveryManager
	tokenManager        *user.TokenManager
	verificationManager *user.VerificationManager
	authAPIHandler      *api.AuthHandler
	userAPIHandler      *api.UserHandler

	// Internationalization per AI.md PART 32
	i18nManager *i18n.Manager
}

// registryAdapter wraps engines.Registry to implement admin.EngineRegistry
type registryAdapter struct {
	r *engines.Registry
}

func (a *registryAdapter) Count() int {
	return a.r.Count()
}

func (a *registryAdapter) GetEnabled() []interface{} {
	engines := a.r.GetEnabled()
	result := make([]interface{}, len(engines))
	for i, e := range engines {
		result[i] = e
	}
	return result
}

func (a *registryAdapter) GetAll() []interface{} {
	engines := a.r.GetAll()
	result := make([]interface{}, len(engines))
	for i, e := range engines {
		result[i] = e
	}
	return result
}

// New creates a new server instance
func New(cfg *config.Config) *Server {
	// Create logging manager
	logDir := config.GetLogDir()
	logMgr := logging.NewManager(logDir)

	// Configure log levels based on config
	if cfg.Server.Logs.Level == "debug" {
		logMgr.Server().SetLevel(logging.LevelDebug)
	} else if cfg.Server.Logs.Level == "warn" {
		logMgr.Server().SetLevel(logging.LevelWarn)
	} else if cfg.Server.Logs.Level == "error" {
		logMgr.Server().SetLevel(logging.LevelError)
	}

	// Configure access log format
	if cfg.Server.Logs.Access.Format == "json" {
		logMgr.Access().SetFormat("json")
	} else if cfg.Server.Logs.Access.Format == "common" {
		logMgr.Access().SetFormat("common")
	}

	// Create engine registry with default engines
	registry := engines.DefaultRegistry()

	// Get all enabled engines (already filtered by IsEnabled())
	enabledEngines := registry.GetEnabled()

	// Create aggregator with 30 second timeout and caching
	aggregator := search.NewAggregator(enabledEngines, search.AggregatorConfig{
		Timeout:      30 * time.Second,
		CacheEnabled: true,
		CacheTTL:     5 * time.Minute,
		MaxCacheSize: 1000,
	})

	// Create middleware with logging
	mw := NewMiddleware(cfg, logMgr)

	// Create rate limiter
	rl := NewRateLimiter(&cfg.Server.RateLimit)

	// Create CSRF middleware
	csrf := NewCSRFMiddleware(cfg)
	csrf.SetLogManager(logMgr)

	// Create template renderer
	renderer := NewTemplateRenderer(cfg)

	// Create API handler
	apiHandler := api.NewHandler(cfg, registry, aggregator)

	// Create Tor service - auto-enabled if tor binary found per AI.md PART 32
	// "Auto-enabled if tor binary is installed - no enable flag needed"
	torSvc := service.NewTorService(cfg)

	// Create bang manager
	bangMgr := bangs.NewManager()

	// Set custom bangs from config
	if len(cfg.Search.Bangs.Custom) > 0 {
		customBangs := make([]*bangs.Bang, 0, len(cfg.Search.Bangs.Custom))
		for _, bc := range cfg.Search.Bangs.Custom {
			customBangs = append(customBangs, &bangs.Bang{
				Shortcut:    bc.Shortcut,
				Name:        bc.Name,
				URL:         bc.URL,
				Category:    bc.Category,
				Description: bc.Description,
				Aliases:     bc.Aliases,
			})
		}
		bangMgr.SetCustomBangs(customBangs)
	}

	// Create TLS manager if SSL is enabled
	var tlsMgr *ssl.Manager
	if cfg.Server.SSL.Enabled {
		dataDir := config.GetDataDir()
		tlsMgr = ssl.NewManager(&cfg.Server.SSL, dataDir)
	}

	// Create GeoIP lookup if enabled (uses MMDB from sapics/ip-location-db)
	var geoLookup *geoip.Lookup
	if cfg.Server.GeoIP.Enabled {
		geoDir := cfg.Server.GeoIP.Dir
		if geoDir == "" {
			geoDir = filepath.Join(config.GetDataDir(), "geoip")
		}
		geoCfg := &geoip.Config{
			Enabled:          true,
			Dir:              geoDir,
			Update:           cfg.Server.GeoIP.Update,
			DenyCountries:    cfg.Server.GeoIP.DenyCountries,
			AllowedCountries: cfg.Server.GeoIP.AllowedCountries,
			ASN:              cfg.Server.GeoIP.ASN,
			Country:          cfg.Server.GeoIP.Country,
			City:             cfg.Server.GeoIP.City,
		}
		geoLookup = geoip.NewLookup(geoCfg)
		if err := geoLookup.LoadDatabases(); err != nil {
			log.Printf("[GeoIP] Warning: Failed to load GeoIP databases: %v", err)
			geoLookup = nil
		} else {
			log.Printf("[GeoIP] Loaded MMDB databases from %s", geoDir)
		}
	}

	// Create widget manager and register fetchers
	widgetMgr := widget.NewManager(&cfg.Search.Widgets)

	// Register data widget fetchers
	if cfg.Search.Widgets.Weather.Enabled {
		widgetMgr.RegisterFetcher(widget.NewWeatherFetcher(&cfg.Search.Widgets.Weather))
	}
	if cfg.Search.Widgets.News.Enabled {
		widgetMgr.RegisterFetcher(widget.NewNewsFetcher(&cfg.Search.Widgets.News))
	}
	if cfg.Search.Widgets.Stocks.Enabled {
		widgetMgr.RegisterFetcher(widget.NewStocksFetcher(&cfg.Search.Widgets.Stocks))
	}
	if cfg.Search.Widgets.Crypto.Enabled {
		widgetMgr.RegisterFetcher(widget.NewCryptoFetcher(&cfg.Search.Widgets.Crypto))
	}
	if cfg.Search.Widgets.Sports.Enabled {
		widgetMgr.RegisterFetcher(widget.NewSportsFetcher(&cfg.Search.Widgets.Sports))
	}
	if cfg.Search.Widgets.RSS.Enabled {
		widgetMgr.RegisterFetcher(widget.NewRSSFetcher(&cfg.Search.Widgets.RSS))
	}

	// Register additional widget fetchers (use free APIs, no API keys needed)
	// These widgets are always available when widgets are enabled
	if cfg.Search.Widgets.Enabled {
		widgetMgr.RegisterFetcher(widget.NewCurrencyFetcher(""))
		widgetMgr.RegisterFetcher(widget.NewDictionaryFetcher())
		widgetMgr.RegisterFetcher(widget.NewNutritionFetcher(""))
		widgetMgr.RegisterFetcher(widget.NewTrackingFetcher())
		widgetMgr.RegisterFetcher(widget.NewTranslateFetcher())
		widgetMgr.RegisterFetcher(widget.NewWikipediaFetcher())
	}

	// Create instant answer manager
	instantMgr := instant.NewManager()

	// Create direct answer manager (full-page results per IDEA.md)
	directMgr := direct.NewManager()

	// Create email mailer
	// Per AI.md PART 18: Email is auto-enabled if SMTP is configured
	var mailer *email.Mailer
	if cfg.Server.Email.SMTP.Host != "" {
		// Apply default from address if not set
		fromEmail := cfg.Server.Email.From.Email
		if fromEmail == "" {
			// Per AI.md PART 18: Default from email is no-reply@{fqdn}
			fqdn := "localhost"
			if cfg.Server.BaseURL != "" {
				// Extract hostname from BaseURL
				if u, err := url.Parse(cfg.Server.BaseURL); err == nil && u.Host != "" {
					fqdn = u.Host
				}
			}
			fromEmail = "no-reply@" + fqdn
		}
		fromName := cfg.Server.Email.From.Name
		if fromName == "" {
			// Per AI.md PART 18: Default from name is app title
			fromName = cfg.Server.Branding.AppName
			if fromName == "" {
				fromName = "Search"
			}
		}

		emailCfg := &email.Config{
			Enabled: true,
			SMTP: email.SMTPConfig{
				Host:     cfg.Server.Email.SMTP.Host,
				Port:     cfg.Server.Email.SMTP.Port,
				Username: cfg.Server.Email.SMTP.Username,
				Password: cfg.Server.Email.SMTP.Password,
				TLS:      cfg.Server.Email.SMTP.TLS,
			},
			From: email.FromConfig{
				Name:  fromName,
				Email: fromEmail,
			},
		}
		mailer = email.NewMailer(emailCfg)
		log.Printf("[Email] Mailer configured (SMTP: %s:%d)", cfg.Server.Email.SMTP.Host, cfg.Server.Email.SMTP.Port)
	}

	// Scheduler is initialized after Server struct creation per AI.md PART 19
	// The scheduler is ALWAYS RUNNING - no enable/disable option

	// Create database manager for user management
	var dbMgr *database.DatabaseManager
	var userAuthMgr *user.AuthManager
	var totpMgr *user.TOTPManager
	var recoveryMgr *user.RecoveryManager
	var tokenMgr *user.TokenManager
	var verificationMgr *user.VerificationManager

	if cfg.Server.Users.Enabled {
		// Initialize database manager
		dbConfig := &database.Config{
			Driver:   "sqlite",
			DataDir:  config.GetDatabaseDir(),
			MaxOpen:  10,
			MaxIdle:  5,
			Lifetime: 300,
		}
		var err error
		dbMgr, err = database.NewDatabaseManager(dbConfig)
		if err != nil {
			log.Printf("[Users] Warning: Failed to initialize database: %v", err)
		} else {
			// Get users database
			usersDB := dbMgr.UsersDB().SQL()
			if usersDB != nil {
				// Create auth manager
				authCfg := user.AuthConfig{
					SessionDurationDays: cfg.Server.Users.GetSessionDurationDays(),
					CookieName:          "user_session",
					CookieSecure:        cfg.Server.SSL.Enabled,
				}
				userAuthMgr = user.NewAuthManager(usersDB, authCfg)
				log.Printf("[Users] Auth manager initialized")

				// Create TOTP manager if 2FA is allowed
				if cfg.Server.Users.Auth.Allow2FA {
					encKey := cfg.GetEncryptionKey()
					if len(encKey) == 32 {
						totpMgr, _ = user.NewTOTPManager(usersDB, cfg.Server.Title, encKey)
						log.Printf("[Users] 2FA manager initialized")
					}
				}

				// Create recovery manager
				recoveryMgr = user.NewRecoveryManager(usersDB, 10)

				// Create token manager
				tokenMgr = user.NewTokenManager(usersDB)
				log.Printf("[Users] Token manager initialized")

				// Create verification manager for email verification and password reset tokens
				verificationMgr = user.NewVerificationManager(usersDB)
				log.Printf("[Users] Verification manager initialized")
			}
		}
	}

	// Create metrics collector
	metrics := NewMetrics(cfg)

	// Initialize i18n manager per AI.md PART 32
	i18nMgr, err := i18n.DefaultManager()
	if err != nil {
		log.Printf("[I18N] Warning: Failed to initialize i18n manager: %v", err)
		// Continue without translations - will use keys as fallback
	} else {
		log.Printf("[I18N] Translations loaded for %d languages", len(i18n.DefaultSupportedLanguages()))
	}

	s := &Server{
		config:          cfg,
		registry:        registry,
		aggregator:      aggregator,
		middleware:      mw,
		rateLimiter:     rl,
		csrf:            csrf,
		renderer:        renderer,
		apiHandler:      apiHandler,
		torService:      torSvc,
		bangManager:     bangMgr,
		widgetManager:   widgetMgr,
		logManager:      logMgr,
		tlsManager:      tlsMgr,
		instantManager:  instantMgr,
		directManager:   directMgr,
		geoipLookup:     geoLookup,
		mailer:          mailer,
		// scheduler is initialized below after Server creation
		metrics:         metrics,
		dbManager:       dbMgr,
		userAuthManager:     userAuthMgr,
		totpManager:         totpMgr,
		recoveryManager:     recoveryMgr,
		tokenManager:        tokenMgr,
		verificationManager: verificationMgr,
		i18nManager:         i18nMgr,
	}

	// Create admin handler (needs renderer interface)
	s.adminHandler = admin.NewHandler(cfg, s.renderer)

	// Set database for admin session persistence per AI.md PART 17
	if dbMgr != nil {
		s.adminHandler.SetDatabase(dbMgr.ServerDB())
		// Set admin service for multi-admin support per AI.md PART 17
		s.adminHandler.SetAdminService(admin.NewAdminService(dbMgr.ServerDB()))
	}

	// Configure admin handler with registry and reload callback
	s.adminHandler.SetRegistry(&registryAdapter{r: s.registry})
	s.adminHandler.SetReloadCallback(func() error {
		log.Println("[Server] Configuration reload triggered")
		// Re-render templates in development mode
		if cfg.IsDevelopment() {
			s.renderer = NewTemplateRenderer(cfg)
		}
		return nil
	})

	// Per AI.md PART 5 lines 5212-5310: Configuration Source of Truth (NON-NEGOTIABLE)
	// Initialize config sync for cluster mode
	if dbMgr != nil {
		isCluster := dbMgr.IsClusterMode()
		configPath := cfg.GetPath()
		if configPath == "" {
			configPath = "/etc/search/server.yml"
		}
		s.configSync = config.NewConfigSync(dbMgr.ServerDB().SQL(), cfg, configPath, isCluster)
		s.adminHandler.SetConfigSync(s.configSync)
		if isCluster {
			log.Printf("[Server] Cluster mode: database is source of truth")
			// Load config from database on startup
			if err := s.configSync.LoadFromSource(); err != nil {
				log.Printf("[Server] Failed to load config from database: %v", err)
			}
			// Start periodic sync (every 5 minutes per AI.md)
			s.configSync.StartPeriodicSync(5 * time.Minute)
		}
	}

	// Set widget manager on API handler
	s.apiHandler.SetWidgetManager(widgetMgr)

	// Set instant answer manager on API handler
	s.apiHandler.SetInstantManager(instantMgr)

	// Set direct answer manager on API handler
	s.apiHandler.SetDirectManager(directMgr)

	// Set related searches provider on API handler
	relatedSearches := search.NewRelatedSearches()
	s.apiHandler.SetRelatedSearches(relatedSearches)

	// Create auth and user API handlers if user management is enabled
	if cfg.Server.Users.Enabled && userAuthMgr != nil && dbMgr != nil {
		usersDB := dbMgr.UsersDB().SQL()
		if usersDB != nil {
			s.authAPIHandler = api.NewAuthHandler(cfg, usersDB, userAuthMgr, totpMgr, recoveryMgr, verificationMgr)
			s.userAPIHandler = api.NewUserHandler(cfg, usersDB, userAuthMgr, totpMgr, recoveryMgr, tokenMgr)
			log.Printf("[Users] API handlers initialized")
		}
	}

	// Initialize scheduler - ALWAYS RUNNING per AI.md PART 19
	// Use server.db for persistent task state if available
	var schedulerDB *sql.DB
	if dbMgr != nil {
		schedulerDB = dbMgr.ServerDB().SQL()
	}
	s.initScheduler(schedulerDB)

	return s
}

// Start starts the HTTP server
func (s *Server) Start() error {
	// Create PID file
	if err := s.createPIDFile(); err != nil {
		return fmt.Errorf("failed to create PID file: %w", err)
	}

	// Start Tor service if enabled
	if s.torService != nil {
		if err := s.torService.Start(); err != nil {
			log.Printf("[Tor] Warning: Failed to start Tor service: %v", err)
		}
	}

	// Scheduler is already started by initScheduler() per AI.md PART 19
	// The scheduler is ALWAYS RUNNING - no enable/disable check needed

	// Setup routes
	mux := s.setupRoutes()

	// Resolve HTTP port (0 means random port in 64xxx range)
	httpPort := s.config.Server.GetHTTPPort()
	if httpPort != s.config.Server.Port {
		log.Printf("[Server] Using random HTTP port: %d", httpPort)
		s.config.Server.Port = httpPort
	}

	s.startTime = time.Now()

	// Get Tor address if available
	var torAddr string
	if s.torService != nil && s.torService.IsRunning() {
		torAddr = s.torService.GetOnionAddress()
		// Log Tor address
		log.Printf("[Server] Tor hidden service: %s", torAddr)
	}

	// Banner is printed by main.go per AI.md PART 7/14
	// Server only logs startup info
	_ = torAddr // Used for Tor logging above

	// Check for dual port mode
	if s.config.Server.IsDualPortMode() && s.tlsManager != nil && s.tlsManager.IsEnabled() {
		return s.startDualPortMode(mux, httpPort)
	}

	// Single port mode
	return s.startSinglePortMode(mux, httpPort)
}

// startDualPortMode starts both HTTP and HTTPS servers on separate ports
func (s *Server) startDualPortMode(mux http.Handler, httpPort int) error {
	httpsPort := s.config.Server.GetHTTPSPort()

	// Create HTTP server
	httpAddr := fmt.Sprintf("%s:%d", s.config.Server.Address, httpPort)
	s.httpServer = &http.Server{
		Addr:         httpAddr,
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Create HTTPS server
	httpsAddr := fmt.Sprintf("%s:%d", s.config.Server.Address, httpsPort)
	httpsHandler := mux
	if s.config.Server.SSL.LetsEncrypt.Enabled {
		httpsHandler = s.tlsManager.GetHTTPSHandler(mux)
	}
	s.httpsServer = &http.Server{
		Addr:         httpsAddr,
		Handler:      httpsHandler,
		TLSConfig:    s.tlsManager.GetTLSConfig(),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	log.Printf("[Server] Dual port mode enabled")
	log.Printf("[Server] HTTP listening on: http://%s", httpAddr)
	log.Printf("[Server] HTTPS listening on: https://%s", httpsAddr)

	// Start HTTP server in goroutine
	errChan := make(chan error, 2)
	go func() {
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errChan <- fmt.Errorf("HTTP server error: %w", err)
		}
	}()

	// Start HTTPS server (blocking)
	if err := s.httpsServer.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
		s.removePIDFile()
		return fmt.Errorf("HTTPS server error: %w", err)
	}

	// Check if HTTP server had an error
	select {
	case err := <-errChan:
		s.removePIDFile()
		return err
	default:
		return nil
	}
}

// startSinglePortMode starts server on a single port (HTTP or HTTPS)
func (s *Server) startSinglePortMode(mux http.Handler, port int) error {
	addr := fmt.Sprintf("%s:%d", s.config.Server.Address, port)
	s.httpServer = &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server based on TLS configuration
	if s.tlsManager != nil && s.tlsManager.IsEnabled() {
		// Configure TLS
		s.httpServer.TLSConfig = s.tlsManager.GetTLSConfig()

		// If using Let's Encrypt, wrap handler for ACME challenges
		if s.config.Server.SSL.LetsEncrypt.Enabled {
			s.httpServer.Handler = s.tlsManager.GetHTTPSHandler(mux)
		}

		log.Printf("[Server] Listening on: https://%s", addr)

		// Start HTTP->HTTPS redirect server on port 80 if configured
		if s.config.Server.SSL.AutoTLS {
			redirectAddr := fmt.Sprintf("%s:80", s.config.Server.Address)
			s.redirectServer = ssl.StartHTTPSRedirect(redirectAddr, s.config.Server.Port)
		}

		// Start HTTPS server
		if err := s.httpServer.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
			s.removePIDFile()
			return fmt.Errorf("server error: %w", err)
		}
	} else {
		log.Printf("[Server] Listening on: http://%s", addr)

		// Start HTTP server (blocking)
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.removePIDFile()
			return fmt.Errorf("server error: %w", err)
		}
	}

	return nil
}

// Shutdown gracefully shuts down the server with a context
func (s *Server) Shutdown(ctx context.Context) error {
	s.logManager.Server().Info("Server shutting down...")

	// Stop scheduler
	if s.scheduler != nil {
		s.scheduler.Stop()
		log.Printf("[Scheduler] Stopped")
	}

	// Stop Tor service
	if s.torService != nil {
		s.torService.Stop()
	}

	// Stop HTTP->HTTPS redirect server if running
	if s.redirectServer != nil {
		s.redirectServer.Shutdown(ctx)
	}

	// Remove PID file
	s.removePIDFile()

	// Shutdown HTTPS server if running (dual port mode)
	if s.httpsServer != nil {
		if err := s.httpsServer.Shutdown(ctx); err != nil {
			log.Printf("[Server] HTTPS server shutdown error: %v", err)
		}
	}

	// Shutdown HTTP server
	if s.httpServer != nil {
		if err := s.httpServer.Shutdown(ctx); err != nil {
			return fmt.Errorf("server shutdown failed: %w", err)
		}
	}

	// Close database connections
	if s.dbManager != nil {
		if err := s.dbManager.Close(); err != nil {
			log.Printf("[Database] Close error: %v", err)
		}
	}

	// Close log files
	s.logManager.Close()

	log.Println("[Server] Stopped gracefully")
	return nil
}

// UpdateConfig updates the server configuration
func (s *Server) UpdateConfig(cfg *config.Config) {
	s.config = cfg
	log.Println("[Server] Configuration updated")
}

// createPIDFile creates a PID file with stale PID detection
// Per AI.md PART 8: Stale PID detection is REQUIRED
func (s *Server) createPIDFile() error {
	dataDir := config.GetDataDir()
	s.pidFile = fmt.Sprintf("%s/search.pid", dataDir)

	// Check for existing PID file and detect stale PIDs
	running, existingPID, err := CheckPIDFile(s.pidFile)
	if err != nil {
		return fmt.Errorf("checking PID file: %w", err)
	}

	if running {
		return fmt.Errorf("server already running (PID: %d)", existingPID)
	}

	// Write PID file
	pid := fmt.Sprintf("%d", os.Getpid())
	return os.WriteFile(s.pidFile, []byte(pid), 0644)
}

// removePIDFile removes the PID file
func (s *Server) removePIDFile() {
	if s.pidFile != "" {
		os.Remove(s.pidFile)
	}
}

// setupRoutes sets up HTTP routes with middleware
func (s *Server) setupRoutes() http.Handler {
	mux := http.NewServeMux()

	// Health check endpoints per AI.md PART 11/13
	// Supports content negotiation: HTML (default), JSON (Accept: application/json), plain text (.txt)
	// Note: /api/v1/healthz is registered by apiHandler.RegisterRoutes()
	mux.HandleFunc("/healthz", s.handleHealthz)
	mux.HandleFunc("/healthz.txt", s.handleHealthz)
	mux.HandleFunc("/readyz", s.handleReadyz)
	mux.HandleFunc("/livez", s.handleLivez)

	// Home page
	mux.HandleFunc("/", s.handleHome)

	// Search
	mux.HandleFunc("/search", s.handleSearch)

	// Direct answers (full-page results for type:term queries per IDEA.md)
	mux.HandleFunc("/direct/", s.handleDirect)

	// Autocomplete (per AI.md PART 36 line 28280)
	mux.HandleFunc("/autocomplete", s.handleAutocomplete)

	// Standard server pages (per AI.md spec)
	mux.HandleFunc("/server/about", s.handleAbout)
	mux.HandleFunc("/server/privacy", s.handlePrivacy)
	mux.HandleFunc("/server/contact", s.handleContact)
	mux.HandleFunc("/server/help", s.handleHelp)
	mux.HandleFunc("/server/terms", s.handleTerms)

	// robots.txt, sitemap.xml, and security.txt (both locations per RFC 9116)
	mux.HandleFunc("/robots.txt", s.handleRobotsTxt)
	mux.HandleFunc("/sitemap.xml", s.handleSitemap)
	mux.HandleFunc("/security.txt", s.handleSecurityTxtEnhanced)
	mux.HandleFunc("/.well-known/security.txt", s.handleSecurityTxtEnhanced)

	// Well-known URIs per RFC 8615
	// Password change redirect per AI.md PART 11
	mux.HandleFunc("/.well-known/change-password", s.handleWellKnownChangePassword)

	// OpenSearch
	if s.config.Search.OpenSearch.Enabled {
		mux.HandleFunc("/opensearch.xml", s.handleOpenSearch)
	}

	// Swagger/OpenAPI endpoints are registered by apiHandler.RegisterOpenAPIRoutes() below
	// GraphQL endpoint is registered by apiHandler.RegisterGraphQLRoutes() below
	// This provides both GraphQL API (POST) and GraphiQL interface (GET)

	// Bang proxy (for privacy-preserving redirects)
	if s.config.Search.Bangs.Enabled {
		mux.HandleFunc("/bang", s.handleBangProxy)
	}

	// Preferences
	mux.HandleFunc("/preferences", s.handlePreferences)
	mux.HandleFunc("/server/preferences", s.handlePreferences)

	// Static files (served from embedded filesystem)
	mux.Handle("/static/", http.StripPrefix("/static/", StaticFileServer()))

	// Admin routes (if enabled)
	if s.config.Server.Admin.Enabled || s.config.Server.Admin.Username != "" {
		s.adminHandler.RegisterRoutes(mux)
	}

	// User authentication routes (if user management is enabled)
	if s.config.Server.Users.Enabled {
		mux.HandleFunc("/auth/login", s.handleLogin)
		mux.HandleFunc("/auth/logout", s.handleLogout)
		mux.HandleFunc("/auth/register", s.handleRegister)
		mux.HandleFunc("/auth/forgot", s.handleForgot)
		mux.HandleFunc("/auth/verify", s.handleVerify)
		mux.HandleFunc("/auth/2fa", s.handle2FA)
		mux.HandleFunc("/auth/recovery", s.handleRecoveryLogin)

		// User profile routes per AI.md PART 14: plural /users/ not singular /user/
		mux.HandleFunc("/users/profile", s.handleUserProfile)
		mux.HandleFunc("/users/security", s.handleUserSecurity)
		mux.HandleFunc("/users/tokens", s.handleUserTokens)
		mux.HandleFunc("/users/2fa/setup", s.handle2FASetup)
		mux.HandleFunc("/users/2fa/disable", s.handle2FADisable)

		// Auth API routes
		if s.authAPIHandler != nil {
			s.authAPIHandler.RegisterRoutes(mux)
		}

		// User API routes
		if s.userAPIHandler != nil {
			s.userAPIHandler.RegisterRoutes(mux)
		}
	}

	// API routes
	s.apiHandler.RegisterRoutes(mux)

	// GraphQL routes
	if err := s.apiHandler.RegisterGraphQLRoutes(mux); err != nil {
		s.logManager.Server().Error("Failed to register GraphQL routes", map[string]interface{}{"error": err.Error()})
	}

	// OpenAPI/Swagger routes
	s.apiHandler.RegisterOpenAPIRoutes(mux)

	// Metrics endpoint (Prometheus-compatible)
	if s.config.Server.Metrics.Enabled {
		metricsPath := s.config.Server.Metrics.Endpoint
		if metricsPath == "" {
			metricsPath = "/metrics"
		}
		mux.HandleFunc(metricsPath, s.metrics.AuthenticatedHandler())
	}

	// Debug endpoints (DEBUG=true only)
	// Per AI.md PART 7: pprof, expvar, and custom debug endpoints
	s.registerDebugRoutes(mux)

	// Apply middleware chain
	// Per AI.md PART 16: URLNormalizeMiddleware MUST be first (last in chain list)
	// Per AI.md PART 5: PathSecurityMiddleware MUST be second
	handler := Chain(
		mux,
		s.middleware.Recovery,
		s.middleware.RequestID,
		s.middleware.Logger,
		s.middleware.SecurityHeaders,
		s.middleware.CORS,
		s.middleware.GeoBlock(s.geoipLookup),
		s.middleware.RateLimit(s.rateLimiter),
		PathSecurityMiddleware,   // SECOND - normalizes paths, blocks traversal
		URLNormalizeMiddleware,   // FIRST - removes trailing slashes, redirects to canonical
	)

	return handler
}

// handleSitemap serves sitemap.xml per AI.md spec
func (s *Server) handleSitemap(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=86400") // Cache for 1 day

	baseURL := s.getBaseURL(r)
	lastMod := time.Now().Format("2006-01-02")

	// XML header
	fmt.Fprintln(w, `<?xml version="1.0" encoding="UTF-8"?>`)
	fmt.Fprintln(w, `<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">`)

	// Define sitemap entries with priority and change frequency
	type sitemapEntry struct {
		loc        string
		priority   string
		changefreq string
	}

	entries := []sitemapEntry{
		{"/", "1.0", "daily"},
		{"/search", "0.9", "daily"},
		{"/server/about", "0.5", "monthly"},
		{"/server/privacy", "0.3", "monthly"},
		{"/server/help", "0.5", "monthly"},
		{"/server/terms", "0.3", "monthly"},
		{"/openapi", "0.4", "weekly"},
		{"/graphql", "0.4", "weekly"},
		{"/healthz", "0.2", "always"},
	}

	// Add contact page if enabled
	if s.config.Server.Pages.Contact.Enabled {
		entries = append(entries, sitemapEntry{"/server/contact", "0.4", "monthly"})
	}

	// Write each URL entry
	for _, entry := range entries {
		fmt.Fprintln(w, "  <url>")
		fmt.Fprintf(w, "    <loc>%s%s</loc>\n", baseURL, entry.loc)
		fmt.Fprintf(w, "    <lastmod>%s</lastmod>\n", lastMod)
		fmt.Fprintf(w, "    <changefreq>%s</changefreq>\n", entry.changefreq)
		fmt.Fprintf(w, "    <priority>%s</priority>\n", entry.priority)
		fmt.Fprintln(w, "  </url>")
	}

	fmt.Fprintln(w, `</urlset>`)
}

// handleSearch handles search requests
func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	// Sanitize and validate input
	queryStr := sanitizeInput(strings.TrimSpace(r.URL.Query().Get("q")))
	category := sanitizeInput(strings.TrimSpace(r.URL.Query().Get("category")))

	// Default to general if no category specified
	if category == "" {
		category = "general"
	}

	if queryStr == "" {
		s.handleError(w, r, http.StatusBadRequest, "Search Error", "Please enter a search query.")
		return
	}

	// Check for bang commands
	if s.config.Search.Bangs.Enabled {
		if bangResult := s.bangManager.Parse(queryStr); bangResult != nil {
			// Handle bang search
			if s.config.Search.Bangs.ProxyRequests {
				// Proxy mode: redirect to our bang proxy handler
				http.Redirect(w, r, "/bang?url="+bangResult.TargetURL, http.StatusFound)
			} else {
				// Direct redirect mode
				http.Redirect(w, r, bangResult.TargetURL, http.StatusFound)
			}
			return
		}
	}

	// Check for direct answer queries (type:term syntax per IDEA.md)
	// These are full-page results, not search results
	if s.directManager != nil {
		answerType, term := s.directManager.Parse(queryStr)
		if answerType != "" {
			// Redirect to direct answer page
			http.Redirect(w, r, "/direct/"+string(answerType)+"/"+url.PathEscape(term), http.StatusFound)
			return
		}
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	// Check for instant answers first (only for general category)
	var instantAnswer *instant.Answer
	if category == "general" && s.instantManager != nil {
		answer, err := s.instantManager.Process(ctx, queryStr)
		if err == nil && answer != nil {
			instantAnswer = answer
		}
	}

	// Map category string to model.Category
	var searchCategory model.Category
	switch category {
	case "images":
		searchCategory = model.CategoryImages
	case "videos":
		searchCategory = model.CategoryVideos
	case "news":
		searchCategory = model.CategoryNews
	case "maps":
		searchCategory = model.CategoryMaps
	default:
		searchCategory = model.CategoryGeneral
	}

	// Perform search
	query := model.NewQuery(queryStr)
	query.Category = searchCategory

	results, err := s.aggregator.Search(ctx, query)

	// Render results page
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	if err != nil {
		s.renderSearchError(w, queryStr, err)
		return
	}

	s.renderSearchResultsWithInstant(w, queryStr, results, category, instantAnswer)
}

// handleDirect handles direct answer requests
// URL format: /direct/{type}/{term}
func (s *Server) handleDirect(w http.ResponseWriter, r *http.Request) {
	// Only allow GET requests
	if r.Method != http.MethodGet {
		s.handleError(w, r, http.StatusMethodNotAllowed, "Method Not Allowed", "Only GET requests are allowed.")
		return
	}

	// Parse path: /direct/{type}/{term}
	path := strings.TrimPrefix(r.URL.Path, "/direct/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		s.handleError(w, r, http.StatusBadRequest, "Invalid Request", "Please specify a direct answer type and term. Example: /direct/tldr/git")
		return
	}

	answerType := direct.AnswerType(parts[0])
	term, err := url.PathUnescape(parts[1])
	if err != nil {
		term = parts[1]
	}
	term = strings.TrimSpace(term)

	if term == "" {
		s.handleError(w, r, http.StatusBadRequest, "Invalid Request", "Please specify a term to look up.")
		return
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	// Process the direct answer
	answer, err := s.directManager.ProcessType(ctx, answerType, term)
	if err != nil {
		s.handleError(w, r, http.StatusInternalServerError, "Direct Answer Error", "An error occurred while processing your request: "+err.Error())
		return
	}

	// Render the direct answer page
	s.renderDirectAnswer(w, r, answer)
}

// renderDirectAnswer renders a direct answer page
func (s *Server) renderDirectAnswer(w http.ResponseWriter, r *http.Request, answer *direct.Answer) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	// Set cache headers based on answer type
	if cacheTTL := direct.CacheDurations[answer.Type]; cacheTTL > 0 {
		w.Header().Set("Cache-Control", fmt.Sprintf("public, max-age=%d", int(cacheTTL.Seconds())))
	} else {
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	}

	// Build the page data
	data := &DirectAnswerPageData{
		PageData: PageData{
			Title:       answer.Title,
			Description: answer.Description,
			Page:        "direct",
			Theme:       "dark",
			Config:      s.config,
			Query:       fmt.Sprintf("%s:%s", answer.Type, answer.Term),
		},
		Answer: answer,
	}

	// Try to render with template
	if err := s.renderer.Render(w, "direct", data); err != nil {
		// Fallback to inline rendering
		s.renderDirectAnswerFallback(w, answer)
	}
}

// DirectAnswerPageData contains data for rendering a direct answer page
type DirectAnswerPageData struct {
	PageData
	Answer *direct.Answer
}

// renderDirectAnswerFallback renders a direct answer without templates
func (s *Server) renderDirectAnswerFallback(w http.ResponseWriter, answer *direct.Answer) {
	baseURL := s.config.Server.BaseURL
	if baseURL == "" {
		baseURL = ""
	}
	appName := s.config.Server.Branding.AppName
	if appName == "" {
		appName = "Search"
	}

	// Build minimal HTML5 page
	fmt.Fprint(w, `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>`)
	fmt.Fprint(w, html.EscapeString(answer.Title))
	fmt.Fprint(w, ` - `)
	fmt.Fprint(w, html.EscapeString(appName))
	fmt.Fprint(w, `</title>
<style>
:root{--bg:#0f0f0f;--fg:#e0e0e0;--accent:#4a9eff;--surface:#1a1a1a;--border:#2a2a2a;--code-bg:#1e1e1e}
*{box-sizing:border-box;margin:0;padding:0}
body{font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,sans-serif;background:var(--bg);color:var(--fg);line-height:1.6;min-height:100vh}
.container{max-width:900px;margin:0 auto;padding:20px}
header{border-bottom:1px solid var(--border);padding-bottom:15px;margin-bottom:20px}
header h1{font-size:1.5rem;color:var(--accent)}
header .meta{font-size:0.875rem;color:#888;margin-top:5px}
.answer-type{display:inline-block;background:var(--accent);color:#000;padding:2px 8px;border-radius:4px;font-size:0.75rem;font-weight:600;margin-right:10px}
.content{background:var(--surface);border:1px solid var(--border);border-radius:8px;padding:20px;margin-bottom:20px}
.content pre{background:var(--code-bg);padding:15px;border-radius:4px;overflow-x:auto;font-family:"SF Mono",Consolas,monospace;font-size:0.875rem}
.content code{font-family:"SF Mono",Consolas,monospace;font-size:0.875rem}
.content h2{color:var(--accent);margin:20px 0 10px;font-size:1.1rem}
.content h3{color:var(--fg);margin:15px 0 8px;font-size:1rem}
.content p{margin:10px 0}
.content ul,.content ol{margin:10px 0;padding-left:25px}
.content li{margin:5px 0}
.content table{width:100%;border-collapse:collapse;margin:15px 0}
.content th,.content td{border:1px solid var(--border);padding:8px 12px;text-align:left}
.content th{background:var(--code-bg)}
.source{font-size:0.875rem;color:#888;padding-top:15px;border-top:1px solid var(--border)}
.source a{color:var(--accent);text-decoration:none}
.source a:hover{text-decoration:underline}
.error{background:#331111;border-color:#662222;color:#ff6b6b}
footer{text-align:center;padding:20px;font-size:0.75rem;color:#666}
footer a{color:var(--accent);text-decoration:none}
@media(max-width:600px){.container{padding:15px}.content{padding:15px}}
</style>
</head>
<body>
<div class="container">
<header>
<h1><span class="answer-type">`)
	fmt.Fprint(w, html.EscapeString(string(answer.Type)))
	fmt.Fprint(w, `</span>`)
	fmt.Fprint(w, html.EscapeString(answer.Title))
	fmt.Fprint(w, `</h1>
<div class="meta">`)
	if answer.Description != "" {
		fmt.Fprint(w, html.EscapeString(answer.Description))
	}
	fmt.Fprint(w, `</div>
</header>
<main>
<div class="content`)
	if answer.Error != "" {
		fmt.Fprint(w, ` error`)
	}
	fmt.Fprint(w, `">`)
	// Content is already HTML formatted by handlers
	fmt.Fprint(w, answer.Content)
	fmt.Fprint(w, `</div>`)
	if answer.Source != "" || answer.SourceURL != "" {
		fmt.Fprint(w, `<div class="source">Source: `)
		if answer.SourceURL != "" {
			fmt.Fprint(w, `<a href="`)
			fmt.Fprint(w, html.EscapeString(answer.SourceURL))
			fmt.Fprint(w, `" rel="noopener noreferrer">`)
			if answer.Source != "" {
				fmt.Fprint(w, html.EscapeString(answer.Source))
			} else {
				fmt.Fprint(w, html.EscapeString(answer.SourceURL))
			}
			fmt.Fprint(w, `</a>`)
		} else {
			fmt.Fprint(w, html.EscapeString(answer.Source))
		}
		fmt.Fprint(w, `</div>`)
	}
	fmt.Fprint(w, `
</main>
<footer>
<a href="`)
	fmt.Fprint(w, baseURL)
	fmt.Fprint(w, `/">`)
	fmt.Fprint(w, html.EscapeString(appName))
	fmt.Fprint(w, `</a> &middot; Direct Answer
</footer>
</div>
</body>
</html>`)
}

// renderSearchError renders an error page
func (s *Server) renderSearchError(w http.ResponseWriter, query string, err error) {
	data := &ErrorPageData{
		PageData: PageData{
			Title:       "Search Error",
			Description: "An error occurred while searching",
			Page:        "error",
			Theme:       "dark",
			Config:      s.config,
			Query:       query,
		},
		ErrorCode:    http.StatusInternalServerError,
		ErrorTitle:   "Search Error",
		ErrorMessage: err.Error(),
	}

	if renderErr := s.renderer.Render(w, "error", data); renderErr != nil {
		// Fallback to plain error
		fmt.Fprintf(w, "<h1>Search Error</h1><p>%s</p>", html.EscapeString(err.Error()))
	}
}

// renderSearchResults renders the search results page
func (s *Server) renderSearchResults(w http.ResponseWriter, query string, results *model.SearchResults, category string) {
	s.renderSearchResultsWithInstant(w, query, results, category, nil)
}

// renderSearchResultsWithInstant renders search results with optional instant answer
func (s *Server) renderSearchResultsWithInstant(w http.ResponseWriter, query string, results *model.SearchResults, category string, instantAnswer *instant.Answer) {
	data := &SearchPageData{
		PageData: PageData{
			Title:       query,
			Description: fmt.Sprintf("Search results for: %s", query),
			Page:        "search",
			Theme:       "dark",
			Config:      s.config,
			Query:       query,
			Category:    category,
		},
		Query:         query,
		Category:      category,
		Results:       results.Results,
		TotalResults:  results.TotalResults,
		SearchTime:    results.SearchTime,
		InstantAnswer: instantAnswer,
	}

	// Calculate pagination
	totalPages := (results.TotalResults + 19) / 20
	currentPage := 1
	data.Pagination = &Pagination{
		CurrentPage: currentPage,
		TotalPages:  totalPages,
		HasPrev:     currentPage > 1,
		HasNext:     currentPage < totalPages,
	}

	if err := s.renderer.Render(w, "search", data); err != nil {
		// Fallback to inline rendering
		s.renderSearchResultsInline(w, query, results, category)
	}
}

// renderSearchResultsInline is a fallback for when templates fail
func (s *Server) renderSearchResultsInline(w http.ResponseWriter, query string, results *model.SearchResults, category string) {
	fmt.Fprintf(w, `<!DOCTYPE html>
<html lang="en" class="theme-dark">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>%s - %s</title>
    <link rel="stylesheet" href="/static/css/main.css">
</head>
<body>
    <main class="search-results">
        <h1>Results for: %s</h1>
        <p>%d results (%.3fs)</p>`,
		html.EscapeString(query),
		s.config.Server.Title,
		html.EscapeString(query),
		results.TotalResults,
		results.SearchTime,
	)

	for i, result := range results.Results {
		if i >= 20 {
			break
		}
		fmt.Fprintf(w, `
        <article class="result">
            <h3><a href="%s" target="_blank">%s</a></h3>
            <p>%s</p>
        </article>`,
			html.EscapeString(result.URL),
			html.EscapeString(result.Title),
			html.EscapeString(result.Content),
		)
	}

	fmt.Fprintf(w, `
    </main>
</body>
</html>`)
}

// sanitizeInput removes potentially dangerous characters from user input
func sanitizeInput(input string) string {
	// Remove any null bytes
	input = strings.ReplaceAll(input, "\x00", "")

	// Remove control characters except newlines and tabs
	var result strings.Builder
	for _, r := range input {
		if r >= 32 || r == '\n' || r == '\t' {
			result.WriteRune(r)
		}
	}

	return result.String()
}
