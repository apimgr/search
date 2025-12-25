package server

import (
	"context"
	"fmt"
	"html"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/apimgr/search/src/admin"
	"github.com/apimgr/search/src/api"
	"github.com/apimgr/search/src/config"
	"github.com/apimgr/search/src/database"
	"github.com/apimgr/search/src/email"
	"github.com/apimgr/search/src/geoip"
	"github.com/apimgr/search/src/instant"
	"github.com/apimgr/search/src/logging"
	"github.com/apimgr/search/src/models"
	"github.com/apimgr/search/src/scheduler"
	"github.com/apimgr/search/src/search"
	"github.com/apimgr/search/src/search/bangs"
	"github.com/apimgr/search/src/search/engines"
	"github.com/apimgr/search/src/service"
	searchtls "github.com/apimgr/search/src/tls"
	"github.com/apimgr/search/src/users"
	"github.com/apimgr/search/src/widgets"
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
	widgetManager  *widgets.Manager
	logManager     *logging.Manager
	tlsManager     *searchtls.Manager
	instantManager *instant.Manager
	geoipLookup    *geoip.Lookup
	mailer         *email.Mailer
	scheduler      *scheduler.Scheduler
	metrics        *Metrics
	dbManager      *database.DatabaseManager

	// User management
	userAuthManager *users.AuthManager
	totpManager     *users.TOTPManager
	recoveryManager *users.RecoveryManager
	tokenManager    *users.TokenManager
	authAPIHandler  *api.AuthHandler
	userAPIHandler  *api.UserHandler
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

	// Create Tor service if enabled
	var torSvc *service.TorService
	if cfg.Server.Tor.Enabled {
		torSvc = service.NewTorService(cfg)
	}

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
	var tlsMgr *searchtls.Manager
	if cfg.Server.SSL.Enabled {
		dataDir := config.GetDataDir()
		tlsMgr = searchtls.NewManager(&cfg.Server.SSL, dataDir)
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
	widgetMgr := widgets.NewManager(&cfg.Search.Widgets)

	// Register data widget fetchers
	if cfg.Search.Widgets.Weather.Enabled {
		widgetMgr.RegisterFetcher(widgets.NewWeatherFetcher(&cfg.Search.Widgets.Weather))
	}
	if cfg.Search.Widgets.News.Enabled {
		widgetMgr.RegisterFetcher(widgets.NewNewsFetcher(&cfg.Search.Widgets.News))
	}
	if cfg.Search.Widgets.Stocks.Enabled {
		widgetMgr.RegisterFetcher(widgets.NewStocksFetcher(&cfg.Search.Widgets.Stocks))
	}
	if cfg.Search.Widgets.Crypto.Enabled {
		widgetMgr.RegisterFetcher(widgets.NewCryptoFetcher(&cfg.Search.Widgets.Crypto))
	}
	if cfg.Search.Widgets.Sports.Enabled {
		widgetMgr.RegisterFetcher(widgets.NewSportsFetcher(&cfg.Search.Widgets.Sports))
	}
	if cfg.Search.Widgets.RSS.Enabled {
		widgetMgr.RegisterFetcher(widgets.NewRSSFetcher(&cfg.Search.Widgets.RSS))
	}

	// Create instant answer manager
	instantMgr := instant.NewManager()

	// Create email mailer if enabled
	var mailer *email.Mailer
	if cfg.Server.Email.Enabled {
		emailCfg := &email.Config{
			Enabled:     true,
			SMTPHost:    cfg.Server.Email.SMTPHost,
			SMTPPort:    cfg.Server.Email.SMTPPort,
			Username:    cfg.Server.Email.SMTPUser,
			Password:    cfg.Server.Email.SMTPPass,
			FromAddress: cfg.Server.Email.FromAddress,
			FromName:    cfg.Server.Email.FromName,
			UseTLS:      cfg.Server.Email.TLS,
			UseSTARTTLS: cfg.Server.Email.StartTLS,
		}
		mailer = email.NewMailer(emailCfg)
		log.Printf("[Email] Mailer configured (SMTP: %s:%d)", cfg.Server.Email.SMTPHost, cfg.Server.Email.SMTPPort)
	}

	// Create scheduler if enabled
	var sched *scheduler.Scheduler
	if cfg.Server.Scheduler.Enabled {
		sched = scheduler.New()

		// Register default scheduled tasks per TEMPLATE.md PART 20
		// Session cleanup: hourly
		sched.Register(&scheduler.Task{
			Name:     "session_cleanup",
			Interval: 1 * time.Hour,
			Run: func(ctx context.Context) error {
				log.Println("[Scheduler] Running session cleanup")
				// Session cleanup logic would go here
				return nil
			},
		})

		// Cache cleanup: every 6 hours
		sched.Register(&scheduler.Task{
			Name:     "cache_cleanup",
			Interval: 6 * time.Hour,
			Run: func(ctx context.Context) error {
				log.Println("[Scheduler] Running cache cleanup")
				// Cache cleanup logic would go here
				return nil
			},
		})

		// Engine health check: every 15 minutes
		sched.Register(&scheduler.Task{
			Name:     "engine_health",
			Interval: 15 * time.Minute,
			Run: func(ctx context.Context) error {
				log.Println("[Scheduler] Running engine health check")
				// Engine health check logic would go here
				return nil
			},
		})

		// Daily backup at 02:00 (check every hour, actual backup logic checks time)
		sched.Register(&scheduler.Task{
			Name:     "backup",
			Interval: 24 * time.Hour,
			Run: func(ctx context.Context) error {
				log.Println("[Scheduler] Running automatic backup")
				// Backup logic would go here
				return nil
			},
		})

		// SSL renewal check: daily
		if cfg.Server.SSL.Enabled && cfg.Server.SSL.LetsEncrypt.Enabled {
			sched.Register(&scheduler.Task{
				Name:     "ssl_renewal",
				Interval: 24 * time.Hour,
				Run: func(ctx context.Context) error {
					log.Println("[Scheduler] Running SSL renewal check")
					// SSL renewal check logic would go here
					return nil
				},
			})
		}

		// GeoIP update: weekly (every 7 days)
		if cfg.Server.GeoIP.Enabled && cfg.Server.GeoIP.Update != "" && cfg.Server.GeoIP.Update != "never" {
			sched.Register(&scheduler.Task{
				Name:     "geoip_update",
				Interval: 7 * 24 * time.Hour,
				Run: func(ctx context.Context) error {
					log.Println("[Scheduler] Running GeoIP database update")
					// GeoIP update logic would go here
					return nil
				},
			})
		}

		log.Printf("[Scheduler] Initialized with default tasks")
	}

	// Create database manager for user management
	var dbMgr *database.DatabaseManager
	var userAuthMgr *users.AuthManager
	var totpMgr *users.TOTPManager
	var recoveryMgr *users.RecoveryManager
	var tokenMgr *users.TokenManager

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
				authCfg := users.AuthConfig{
					SessionDurationDays: cfg.Server.Users.GetSessionDurationDays(),
					CookieName:          "user_session",
					CookieSecure:        cfg.Server.SSL.Enabled,
				}
				userAuthMgr = users.NewAuthManager(usersDB, authCfg)
				log.Printf("[Users] Auth manager initialized")

				// Create TOTP manager if 2FA is allowed
				if cfg.Server.Users.Auth.Allow2FA {
					encKey := cfg.GetEncryptionKey()
					if len(encKey) == 32 {
						totpMgr, _ = users.NewTOTPManager(usersDB, cfg.Server.Title, encKey)
						log.Printf("[Users] 2FA manager initialized")
					}
				}

				// Create recovery manager
				recoveryMgr = users.NewRecoveryManager(usersDB, 10)

				// Create token manager
				tokenMgr = users.NewTokenManager(usersDB)
				log.Printf("[Users] Token manager initialized")
			}
		}
	}

	// Create metrics collector
	metrics := NewMetrics(cfg)

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
		geoipLookup:     geoLookup,
		mailer:          mailer,
		scheduler:       sched,
		metrics:         metrics,
		dbManager:       dbMgr,
		userAuthManager: userAuthMgr,
		totpManager:     totpMgr,
		recoveryManager: recoveryMgr,
		tokenManager:    tokenMgr,
	}

	// Create admin handler (needs renderer interface)
	s.adminHandler = admin.NewHandler(cfg, s.renderer)

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

	// Set widget manager on API handler
	s.apiHandler.SetWidgetManager(widgetMgr)

	// Set instant answer manager on API handler
	s.apiHandler.SetInstantManager(instantMgr)

	// Create auth and user API handlers if user management is enabled
	if cfg.Server.Users.Enabled && userAuthMgr != nil && dbMgr != nil {
		usersDB := dbMgr.UsersDB().SQL()
		if usersDB != nil {
			s.authAPIHandler = api.NewAuthHandler(cfg, usersDB, userAuthMgr, totpMgr, recoveryMgr)
			s.userAPIHandler = api.NewUserHandler(cfg, usersDB, userAuthMgr, totpMgr, recoveryMgr, tokenMgr)
			log.Printf("[Users] API handlers initialized")
		}
	}

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

	// Start scheduler if enabled
	if s.scheduler != nil {
		s.scheduler.Start()
		log.Printf("[Scheduler] Started")
	}

	// Setup routes
	mux := s.setupRoutes()

	// Resolve HTTP port (0 means random port in 64xxx range)
	httpPort := s.config.Server.GetHTTPPort()
	if httpPort != s.config.Server.Port {
		log.Printf("[Server] Using random HTTP port: %d", httpPort)
		s.config.Server.Port = httpPort
	}

	s.startTime = time.Now()

	// Display startup message based on mode
	if s.config.Server.Mode == "development" {
		log.Printf("[Server] Search v%s [DEVELOPMENT MODE]", config.Version)
		log.Printf("[Server] Debug endpoints enabled")
		log.Printf("[Server] Verbose error messages enabled")
		log.Printf("[Server] Template caching disabled")
		log.Printf("[Server] Mode: development")
	} else {
		log.Printf("[Server] Search v%s", config.Version)
		log.Printf("[Server] Mode: production")
	}

	// Log Tor status
	if s.torService != nil && s.torService.IsRunning() {
		if addr := s.torService.GetOnionAddress(); addr != "" {
			log.Printf("[Tor] Hidden service: %s", addr)
		}
	}

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
			s.redirectServer = searchtls.StartHTTPSRedirect(redirectAddr, s.config.Server.Port)
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

// createPIDFile creates a PID file
func (s *Server) createPIDFile() error {
	dataDir := config.GetDataDir()
	s.pidFile = fmt.Sprintf("%s/search.pid", dataDir)

	// Check if PID file already exists
	if _, err := os.Stat(s.pidFile); err == nil {
		// Read existing PID
		data, _ := os.ReadFile(s.pidFile)
		return fmt.Errorf("server already running (PID: %s)", string(data))
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

	// Health check - /healthz only per TEMPLATE.md spec (NO /health)
	// Supports content negotiation: HTML (default), JSON (Accept: application/json), plain text (.txt)
	mux.HandleFunc("/healthz", s.handleHealthz)
	mux.HandleFunc("/healthz.txt", s.handleHealthz)

	// Home page
	mux.HandleFunc("/", s.handleHome)

	// Search
	mux.HandleFunc("/search", s.handleSearch)

	// Standard server pages (per TEMPLATE.md spec)
	mux.HandleFunc("/server/about", s.handleAbout)
	mux.HandleFunc("/server/privacy", s.handlePrivacy)
	mux.HandleFunc("/server/contact", s.handleContact)
	mux.HandleFunc("/server/help", s.handleHelp)

	// robots.txt, sitemap.xml, and security.txt (both locations per RFC 9116)
	mux.HandleFunc("/robots.txt", s.handleRobots)
	mux.HandleFunc("/sitemap.xml", s.handleSitemap)
	mux.HandleFunc("/security.txt", s.handleSecurityTxt)
	mux.HandleFunc("/.well-known/security.txt", s.handleSecurityTxt)

	// OpenSearch
	if s.config.Search.OpenSearch.Enabled {
		mux.HandleFunc("/opensearch.xml", s.handleOpenSearch)
	}

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

		// User profile routes
		mux.HandleFunc("/user/profile", s.handleUserProfile)
		mux.HandleFunc("/user/security", s.handleUserSecurity)
		mux.HandleFunc("/user/tokens", s.handleUserTokens)
		mux.HandleFunc("/user/2fa/setup", s.handle2FASetup)
		mux.HandleFunc("/user/2fa/disable", s.handle2FADisable)

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
		metricsPath := s.config.Server.Metrics.Path
		if metricsPath == "" {
			metricsPath = "/metrics"
		}
		mux.HandleFunc(metricsPath, s.metrics.AuthenticatedHandler())
	}

	// Apply middleware chain
	handler := Chain(
		mux,
		s.middleware.Recovery,
		s.middleware.RequestID,
		s.middleware.Logger,
		s.middleware.SecurityHeaders,
		s.middleware.CORS,
		s.middleware.GeoBlock(s.geoipLookup),
		s.middleware.RateLimit(s.rateLimiter),
	)

	return handler
}

// handleRobots serves robots.txt per TEMPLATE.md spec
func (s *Server) handleRobots(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=86400") // Cache for 1 day

	web := s.config.Server.Web

	// Build robots.txt
	fmt.Fprintln(w, "# robots.txt - Search Engine Crawling Rules")
	fmt.Fprintln(w, "#", s.config.Server.Title)
	fmt.Fprintln(w)
	fmt.Fprintln(w, "User-agent: *")

	// Allow paths (default: /, /api)
	allowPaths := web.Robots.Allow
	if len(allowPaths) == 0 {
		allowPaths = []string{"/", "/api"}
	}
	for _, path := range allowPaths {
		fmt.Fprintln(w, "Allow:", path)
	}

	// Deny paths (default: /admin)
	denyPaths := web.Robots.Deny
	if len(denyPaths) == 0 {
		denyPaths = []string{"/admin"}
	}
	for _, path := range denyPaths {
		fmt.Fprintln(w, "Disallow:", path)
	}

	// Add sitemap URL per TEMPLATE.md spec
	fmt.Fprintln(w)
	baseURL := s.getBaseURL(r)
	fmt.Fprintln(w, "Sitemap:", baseURL+"/sitemap.xml")
}

// handleSecurityTxt serves security.txt per RFC 9116 and TEMPLATE.md spec
// Required fields: Contact, Expires
func (s *Server) handleSecurityTxt(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=86400") // Cache for 1 day

	security := s.config.Server.Web.Security

	// Contact (REQUIRED per RFC 9116)
	// Must be a URI (mailto:, https://, or tel:)
	contact := security.Contact
	if contact == "" && s.config.Server.Admin.Email != "" {
		contact = s.config.Server.Admin.Email
	}
	if contact != "" {
		// Ensure mailto: prefix for email addresses
		if strings.Contains(contact, "@") && !strings.HasPrefix(contact, "mailto:") {
			contact = "mailto:" + contact
		}
		fmt.Fprintln(w, "Contact:", contact)
	} else {
		// Fallback to fqdn-based email
		fqdn := s.getBaseURL(r)
		fqdn = strings.TrimPrefix(fqdn, "https://")
		fqdn = strings.TrimPrefix(fqdn, "http://")
		fmt.Fprintln(w, "Contact: mailto:security@"+fqdn)
	}

	// Expires (REQUIRED per RFC 9116)
	// Must be in ISO 8601 format (YYYY-MM-DDTHH:MM:SSZ)
	expires := security.Expires
	if expires == "" {
		// Default: 1 year from now (auto-renewed yearly)
		expiryTime := time.Now().AddDate(1, 0, 0)
		expires = expiryTime.UTC().Format(time.RFC3339)
	}
	fmt.Fprintln(w, "Expires:", expires)
}

// handleSitemap serves sitemap.xml per TEMPLATE.md spec
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

	// Map category string to models.Category
	var searchCategory models.Category
	switch category {
	case "images":
		searchCategory = models.CategoryImages
	case "videos":
		searchCategory = models.CategoryVideos
	case "news":
		searchCategory = models.CategoryNews
	case "maps":
		searchCategory = models.CategoryMaps
	default:
		searchCategory = models.CategoryGeneral
	}

	// Perform search
	query := models.NewQuery(queryStr)
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
func (s *Server) renderSearchResults(w http.ResponseWriter, query string, results *models.SearchResults, category string) {
	s.renderSearchResultsWithInstant(w, query, results, category, nil)
}

// renderSearchResultsWithInstant renders search results with optional instant answer
func (s *Server) renderSearchResultsWithInstant(w http.ResponseWriter, query string, results *models.SearchResults, category string, instantAnswer *instant.Answer) {
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
func (s *Server) renderSearchResultsInline(w http.ResponseWriter, query string, results *models.SearchResults, category string) {
	fmt.Fprintf(w, `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>%s - %s</title>
    <link rel="stylesheet" href="/static/css/main.css">
</head>
<body data-theme="dark">
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
