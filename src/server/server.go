package server

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"html"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/apimgr/search/src/alert"
	"github.com/apimgr/search/src/api"
	"github.com/apimgr/search/src/config"
	"github.com/apimgr/search/src/database"
	"github.com/apimgr/search/src/direct"
	"github.com/apimgr/search/src/email"
	"github.com/apimgr/search/src/geoip"
	graphqlpkg "github.com/apimgr/search/src/graphql"
	"github.com/apimgr/search/src/i18n"
	"github.com/apimgr/search/src/instant"
	"github.com/apimgr/search/src/logging"
	"github.com/apimgr/search/src/model"
	"github.com/apimgr/search/src/scheduler"
	"github.com/apimgr/search/src/search"
	"github.com/apimgr/search/src/search/bang"
	"github.com/apimgr/search/src/search/engine"
	"github.com/apimgr/search/src/service"
	"github.com/apimgr/search/src/ssl"
	"github.com/apimgr/search/src/widget"
	"github.com/go-chi/chi/v5"
)

// Server represents the HTTP server
type Server struct {
	config     *config.Config
	httpServer *http.Server
	// For dual port mode
	httpsServer    *http.Server
	redirectServer *http.Server
	pidFile        string
	registry       *engine.Registry
	aggregator     *search.Aggregator
	startTime      time.Time
	middleware     *Middleware
	rateLimiter    *RateLimiter
	csrf           *CSRFMiddleware
	renderer       *TemplateRenderer
	apiHandler     *api.Handler
	torService     *service.TorService
	bangManager    *bang.Manager
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
	alertManager   *alert.Manager
	// Per AI.md PART 5: config sync persists settings back to server.yml
	configSync *config.ConfigSync

	// Internationalization per AI.md PART 32
	i18nManager *i18n.Manager

	// Debug accessors per AI.md PART 6
	router chi.Router
	cache  *search.ResultCache
	db     *sql.DB
}

// NewServer creates a new server instance
func NewServer(cfg *config.Config) *Server {
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
	registry := engine.DefaultRegistry()

	// Get all enabled engines (already filtered by IsEnabled())
	enabledEngines := registry.GetEnabled()

	// Create aggregator with 30 second timeout and caching
	aggregator := search.NewAggregator(enabledEngines, search.AggregatorConfig{
		Timeout:       time.Duration(cfg.Search.Timeout) * time.Second,
		CacheEnabled:  true,
		CacheTTL:      5 * time.Minute,
		MaxConcurrent: cfg.Search.MaxConcurrent,
		// Cache backend defaults to in-memory; set Cache field to use Valkey/Redis
	})

	// Create middleware with logging
	mw := NewMiddleware(cfg, logMgr)

	// Create rate limiter
	rl := NewRateLimiter(&cfg.Server.RateLimit)

	// Create CSRF middleware
	csrf := NewCSRFMiddleware(cfg)
	csrf.SetLogManager(logMgr)

	// Create template renderer (i18n funcs set later after i18n manager init)
	var renderer *TemplateRenderer

	// Create API handler
	apiHandler := api.NewHandler(cfg, registry, aggregator)

	// Create Tor service - auto-enabled if tor binary found per AI.md PART 32
	// "Auto-enabled if tor binary is installed - no enable flag needed"
	torSvc := service.NewTorService(cfg)

	// Set Tor service on API handler for health checks per AI.md PART 32
	apiHandler.SetTorService(torSvc)

	// Create bang manager
	bangMgr := bang.NewManager()

	// Set custom bangs from config
	if len(cfg.Search.Bangs.Custom) > 0 {
		customBangs := make([]*bang.Bang, 0, len(cfg.Search.Bangs.Custom))
		for _, bc := range cfg.Search.Bangs.Custom {
			customBangs = append(customBangs, &bang.Bang{
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
			fromName = cfg.Server.Branding.Title
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

	// Create database manager for server persistence
	var dbMgr *database.DatabaseManager
	var err error

	dbConfig := &database.Config{
		Driver:   "sqlite",
		DataDir:  config.GetDatabaseDir(),
		MaxOpen:  10,
		MaxIdle:  5,
		Lifetime: 300,
	}
	dbMgr, err = database.NewDatabaseManager(dbConfig)
	if err != nil {
		log.Printf("[Database] Warning: Failed to initialize database: %v", err)
	} else {
		if err := database.InitSchema(context.Background(), dbMgr); err != nil {
			log.Printf("[Database] Warning: Failed to initialize schema: %v", err)
		} else {
			log.Printf("[Database] Schema initialized successfully")
		}
	}

	// Per IDEA.md: privacy is the product — no admin web UI, no sessions,
	// no user accounts. Operator privilege is held by anyone with server.token.

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

	renderer = NewTemplateRenderer(cfg, i18nMgr)

	var alertMgr *alert.Manager
	if dbMgr != nil && dbMgr.ServerDB() != nil && dbMgr.ServerDB().SQL() != nil {
		alertMgr = alert.NewManager(dbMgr.ServerDB().SQL(), cfg, aggregator, mailer)
	}

	// Set debug accessor for cache per AI.md PART 6
	var resultCache *search.ResultCache
	if aggregator != nil {
		resultCache = aggregator.Cache()
	}

	// Set debug accessor for db per AI.md PART 6
	var serverDB *sql.DB
	if dbMgr != nil && dbMgr.ServerDB() != nil {
		serverDB = dbMgr.ServerDB().SQL()
	}

	s := &Server{
		config:         cfg,
		registry:       registry,
		aggregator:     aggregator,
		middleware:     mw,
		rateLimiter:    rl,
		csrf:           csrf,
		renderer:       renderer,
		apiHandler:     apiHandler,
		torService:     torSvc,
		bangManager:    bangMgr,
		widgetManager:  widgetMgr,
		logManager:     logMgr,
		tlsManager:     tlsMgr,
		instantManager: instantMgr,
		directManager:  directMgr,
		geoipLookup:    geoLookup,
		mailer:         mailer,
		// scheduler is initialized below after Server creation
		metrics:      metrics,
		dbManager:    dbMgr,
		alertManager: alertMgr,
		i18nManager:  i18nMgr,
		// Debug accessors per AI.md PART 6
		cache: resultCache,
		db:    serverDB,
	}

	// Per AI.md PART 5: server.yml is the source of truth (single-instance, AI.md line 2055)
	if dbMgr != nil {
		configPath := cfg.GetPath()
		if configPath == "" {
			configPath = "/etc/search/server.yml"
		}
		s.configSync = config.NewConfigSync(dbMgr.ServerDB().SQL(), cfg, configPath)
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
	s.apiHandler.SetAlertManager(alertMgr)

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
func (s *Server) StartHTTPServer() error {
	// Create PID file
	if err := s.createPIDFile(); err != nil {
		return fmt.Errorf("failed to create PID file: %w", err)
	}

	// Start Tor service if enabled
	if s.torService != nil {
		if err := s.torService.StartTorService(); err != nil {
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
	// Used for Tor logging above
	_ = torAddr

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
		s.scheduler.StopTaskScheduler()
		log.Printf("[Scheduler] Stopped")
	}

	// Stop Tor service
	if s.torService != nil {
		s.torService.StopTorService()
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

// newPageData creates PageData with Tor status and theme automatically set
// Per AI.md PART 32: Tor section shown if enabled (always, not just when connected)
// Per AI.md PART 16: Theme read from cookie set by client-side JS
func (s *Server) newPageData(w http.ResponseWriter, r *http.Request, title, page string) *PageData {
	data := NewPageData(s.config, title, page)
	i18nManager := s.getI18nManager()
	data.Lang = i18nManager.ResolveLanguage(w, r)
	if i18nManager.IsRTL(data.Lang) {
		data.Dir = "rtl"
	} else {
		data.Dir = "ltr"
	}
	data.AvailableLanguages = i18nManager.SupportedLanguages()
	prefsQuery := strings.TrimSpace(r.URL.Query().Get("prefs"))
	prefs := parseSearchPreferences(prefsQuery)
	// Per AI.md PART 16: Theme read from cookie; resolve "auto" to "dark" server-side
	// JS overrides with system preference on page load when mode is "auto"
	themeMode := GetTheme(r)
	if prefs.Theme != "" {
		themeMode = prefs.Theme
	}
	data.ThemeMode = themeMode
	if themeMode == ThemeAuto {
		// server-side fallback; JS handles system preference
		data.Theme = ThemeDark
	} else {
		data.Theme = themeMode
	}
	data.PrefsQuery = prefsQuery
	if prefs.DefaultCategory != "" {
		data.Category = prefs.DefaultCategory.String()
	}
	// Set Tor status per AI.md PART 32
	if s.torService != nil {
		data.TorEnabled = true
		if s.torService.IsRunning() {
			data.TorAddress = s.torService.GetOnionAddress()
			if data.TorAddress != "" {
				data.TorStatus = "connected"
			} else {
				data.TorStatus = "connecting..."
			}
		} else {
			data.TorStatus = "starting..."
		}
	}
	return data
}

func (s *Server) getI18nManager() *i18n.Manager {
	if s.i18nManager != nil {
		return s.i18nManager
	}
	return i18n.NewManager("en", i18n.DefaultSupportedLanguages())
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
	r := chi.NewRouter()
	// Store router reference for debug route introspection per AI.md PART 6
	s.router = r
	// chi NotFound handler: stdlib mux's "/" matched everything; with chi we
	// route "/" exactly and dispatch other unmatched paths to handleNotFound.
	r.NotFound(s.handleNotFound)

	// Health check endpoints per AI.md PART 13
	// Canonical frontend route: /server/healthz (content-negotiated HTML/JSON/text)
	// Root alias: /healthz → /server/healthz (optional; treated as always-on here)
	// Note: /api/v1/server/healthz and /api/v1/healthz are registered by apiHandler.RegisterRoutes()
	r.HandleFunc("/server/healthz", s.handleHealthz)
	r.HandleFunc("/server/healthz.txt", s.handleHealthz)
	r.HandleFunc("/healthz", s.handleHealthz)
	r.HandleFunc("/healthz.txt", s.handleHealthz)
	r.HandleFunc("/readyz", s.handleReadyz)
	r.HandleFunc("/livez", s.handleLivez)

	// Home page (root catch-all)
	r.HandleFunc("/", s.handleHome)

	// Search
	r.HandleFunc("/search", s.handleSearch)
	r.HandleFunc("/alerts/new", s.handleAlertNew)
	r.HandleFunc("/alerts", s.handleAlerts)
	r.HandleFunc("/alerts/*", s.handleAlertAction)

	// Direct answers (full-page results for type:term queries per IDEA.md)
	r.HandleFunc("/direct/*", s.handleDirect)

	// Autocomplete (per AI.md PART 32 line 28280)
	r.HandleFunc("/autocomplete", s.handleAutocomplete)

	// Standard server pages (per AI.md spec)
	// /server → /server/about redirect per AI.md line 17696
	r.HandleFunc("/server", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/server/about", http.StatusMovedPermanently)
	})
	r.HandleFunc("/server/about", s.handleAbout)
	r.HandleFunc("/server/about/*", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/server/about", http.StatusMovedPermanently)
	})
	r.HandleFunc("/server/privacy", s.handlePrivacy)
	r.HandleFunc("/server/contact", s.handleContact)
	r.HandleFunc("/server/help", s.handleHelp)
	r.HandleFunc("/server/terms", s.handleTerms)

	// robots.txt, sitemap.xml, and security.txt (/.well-known/ only per RFC 9116)
	r.HandleFunc("/robots.txt", s.handleRobotsTxt)
	r.HandleFunc("/sitemap.xml", s.handleSitemap)
	r.HandleFunc("/.well-known/security.txt", s.handleSecurityTxtEnhanced)

	// Well-known URIs per RFC 8615
	// Password change redirect per AI.md PART 11
	r.HandleFunc("/.well-known/change-password", s.handleWellKnownChangePassword)

	// OpenSearch
	if s.config.Search.OpenSearch.Enabled {
		r.HandleFunc("/opensearch.xml", s.handleOpenSearch)
	}

	// Swagger/OpenAPI endpoints are registered by apiHandler.RegisterOpenAPIRoutes() below

	// Bang proxy (for privacy-preserving redirects)
	if s.config.Search.Bangs.Enabled {
		r.HandleFunc("/bang", s.handleBangProxy)
	}

	// Preferences
	r.HandleFunc("/preferences", s.handlePreferences)
	r.HandleFunc("/server/preferences", s.handlePreferences)

	// Static files (served from embedded filesystem)
	r.Handle("/static/*", http.StripPrefix("/static/", StaticFileServer()))
	r.HandleFunc("/locales/*", s.handleLocale)

	// No admin web UI, no login routes — per AI.md, configuration is via
	// server.yml only and the only API privilege check is the operator
	// bearer token (see src/server/auth.go).

	// API routes
	s.apiHandler.RegisterRoutes(r)

	// GraphQL routes per AI.md PART 14
	// GET /server/docs/graphql → GraphiQL UI (interactive explorer, POSTs to /api/graphql)
	r.Get("/server/docs/graphql", graphqlpkg.UIHandler(s.config))
	// POST /api/graphql → GraphQL queries (unversioned alias for current api version)
	r.Post("/api/graphql", graphqlpkg.QueryHandler(s.config))
	// POST /api/{api_version}/server/graphql → GraphQL queries (versioned canonical)
	r.Post(api.APIPrefix+"/server/graphql", graphqlpkg.QueryHandler(s.config))

	// OpenAPI/Swagger routes
	s.apiHandler.RegisterOpenAPIRoutes(r)

	// Metrics endpoint (Prometheus-compatible)
	if s.config.Server.Metrics.Enabled {
		metricsPath := s.config.Server.Metrics.Endpoint
		if metricsPath == "" {
			metricsPath = "/metrics"
		}
		r.HandleFunc(metricsPath, s.metrics.AuthenticatedHandler())
	}

	// Debug endpoints (DEBUG=true only)
	// Per AI.md PART 7: pprof, expvar, and custom debug endpoints
	s.registerDebugRoutes(r)

	// Apply middleware chain per AI.md PART 5 execution order (1→10).
	// Chain() iterates from last to first, so index 0 = outermost = first to execute.
	// Middleware order per AI.md PART 5 (NON-NEGOTIABLE):
	// Recovery → URLNormalize(1) → RequestID(2) → PathSecurity(3) →
	// SecurityHeaders(4) → SecGPC → CORS → Allowlist(5) → Blocklist(6) →
	// RateLimit(7) → GeoIP(8) → Logging(10). Auth(9) is per-route.
	handler := Chain(
		r,
		s.middleware.Recovery,                 // outermost: catches all panics
		URLNormalizeMiddleware,                // 1. normalize URLs (trailing slash, etc.)
		s.middleware.RequestID,                // 2. attach request ID (before logging)
		PathSecurityMiddleware,                // 3. validate paths, block traversal
		s.middleware.SecurityHeaders,          // 4. add security headers
		s.middleware.SecGPC,                   // 4b. honor Sec-GPC privacy signal
		s.middleware.CORS,                     // 4c. CORS (near security headers; handles preflight)
		s.middleware.Allowlist,                // 5. set allowlisted flag (bypasses 6/7/8, not auth)
		s.middleware.Blocklist,                // 6. IP/domain blocklist check
		s.middleware.RateLimit(s.rateLimiter), // 7. rate limiting
		s.middleware.GeoBlock(s.geoipLookup),  // 8. country blocking
		s.middleware.Logger,                   // 10. log requests (innermost of spec chain)
	)

	return handler
}

// handleSitemap serves sitemap.xml per AI.md spec
func (s *Server) handleSitemap(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	// Cache for 1 day
	w.Header().Set("Cache-Control", "public, max-age=86400")

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
		{"/server/docs/swagger", "0.4", "weekly"},
		{"/server/docs/graphql", "0.4", "weekly"},
		{"/server/healthz", "0.2", "always"},
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
	prefs := parseSearchPreferences(r.URL.Query().Get("prefs"))

	// Sanitize and validate input
	queryStr := sanitizeInput(strings.TrimSpace(r.URL.Query().Get("q")))
	categoryParam := sanitizeInput(strings.TrimSpace(r.URL.Query().Get("category")))
	category := prefs.DefaultCategory.String()
	if category == "" {
		category = "general"
	}
	if categoryParam != "" {
		category = model.ParseCategory(categoryParam).String()
	}
	page, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("page")))
	perPage, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("per_page")))
	if perPage == 0 {
		perPage, _ = strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("limit")))
	}
	safeSearch, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("safe_search")))

	// Default to general if no category specified
	if category == "" {
		category = "general"
	}
	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 100 {
		perPage = prefs.ResultsPerPage
	}
	if perPage < 1 || perPage > 100 {
		perPage = 20
	}
	if strings.TrimSpace(r.URL.Query().Get("safe_search")) == "" {
		safeSearch = prefs.SafeSearch
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

	// Perform search
	query := model.NewQuery(queryStr)
	query.Category = model.ParseCategory(category)
	query.Page = page
	query.PerPage = perPage
	query.SafeSearch = safeSearch

	results, err := s.aggregator.Search(ctx, query)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	if err != nil && !errors.Is(err, model.ErrNoResults) {
		s.renderSearchError(w, r, queryStr, err)
		return
	}

	s.renderSearchResultsWithInstant(w, r, queryStr, results, category, instantAnswer)
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
		s.handleInternalError(w, r, "direct answer processing", err)
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

	// Use newPageData for TorAddress support per AI.md PART 32
	baseData := s.newPageData(w, r, answer.Title, "direct")
	baseData.Description = answer.Description
	baseData.Query = fmt.Sprintf("%s:%s", answer.Type, answer.Term)

	data := &DirectAnswerPageData{
		PageData: *baseData,
		Answer:   answer,
	}

	// Try to render with template
	if err := s.renderer.Render(w, "direct", data); err != nil {
		// Fallback to inline rendering
		s.renderDirectAnswerFallback(w, r, answer)
	}
}

// DirectAnswerPageData contains data for rendering a direct answer page
type DirectAnswerPageData struct {
	PageData
	Answer *direct.Answer
}

// renderDirectAnswerFallback renders a direct answer without templates
func (s *Server) renderDirectAnswerFallback(w http.ResponseWriter, r *http.Request, answer *direct.Answer) {
	baseURL := s.config.Server.BaseURL
	if baseURL == "" {
		baseURL = ""
	}
	appName := s.config.Server.Branding.Title
	if appName == "" {
		appName = "Search"
	}
	baseData := s.newPageData(w, r, answer.Title, "direct")
	t := func(key string, args ...interface{}) string {
		return s.getI18nManager().T(baseData.Lang, key, args...)
	}

	// Build minimal HTML5 page
	fmt.Fprint(w, `<!DOCTYPE html>
<html lang="`)
	fmt.Fprint(w, html.EscapeString(baseData.Lang))
	fmt.Fprint(w, `" dir="`)
	fmt.Fprint(w, html.EscapeString(baseData.Dir))
	fmt.Fprint(w, `">
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
		fmt.Fprint(w, `<div class="source">`)
		fmt.Fprint(w, html.EscapeString(t("direct.source_label")))
		fmt.Fprint(w, ` `)
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
	fmt.Fprint(w, `</a> &middot; `)
	fmt.Fprint(w, html.EscapeString(t("direct.footer_label")))
	fmt.Fprint(w, `
</footer>
</div>
</body>
</html>`)
}

// renderSearchError renders an error page
// Per AI.md PART 9: Never expose internal error details to users
func (s *Server) renderSearchError(w http.ResponseWriter, r *http.Request, query string, err error) {
	// Log actual error for debugging
	// Privacy: never log user queries (privacy is the product per CLAUDE.md rule #10)
	log.Printf("[ERROR] search error: %v", err)

	// Use newPageData for TorAddress support per AI.md PART 32
	baseData := s.newPageData(w, r, "Search Error", "error")
	baseData.Description = "An error occurred while searching"
	baseData.Query = query

	data := &ErrorPageData{
		PageData:   *baseData,
		StatusCode: http.StatusInternalServerError,
		StatusText: "Search Error",
		Message:    "An error occurred while processing your search. Please try again.",
	}

	if renderErr := s.renderer.Render(w, "error", data); renderErr != nil {
		// Fallback to plain error - still no internal details
		localizedHTTPError(w, r, http.StatusInternalServerError, "errors.server_error")
	}
}

// renderSearchResultsWithInstant renders search results with optional instant answer
func (s *Server) renderSearchResultsWithInstant(w http.ResponseWriter, r *http.Request, query string, results *model.SearchResults, category string, instantAnswer *instant.Answer) {
	safeSearch, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("safe_search")))

	// Use newPageData for TorAddress support per AI.md PART 32
	baseData := s.newPageData(w, r, query, "search")
	baseData.Description = fmt.Sprintf("Search results for: %s", query)
	baseData.Query = query
	baseData.Category = category

	data := &SearchPageData{
		PageData:      *baseData,
		Query:         query,
		Category:      category,
		Results:       results.GetPage(results.Page),
		TotalResults:  results.TotalResults,
		SearchTime:    results.SearchTime,
		Engines:       results.Engines,
		PerPage:       results.PerPage,
		SafeSearch:    safeSearch,
		InstantAnswer: instantAnswer,
	}

	pageLinks := make([]int, 0, results.TotalPages)
	for page := 1; page <= results.TotalPages; page++ {
		pageLinks = append(pageLinks, page)
	}

	data.Pagination = &Pagination{
		CurrentPage: results.Page,
		TotalPages:  results.TotalPages,
		HasPrev:     results.Page > 1,
		HasNext:     results.Page < results.TotalPages,
		PrevPage:    results.Page - 1,
		NextPage:    results.Page + 1,
		Pages:       pageLinks,
	}

	if err := s.renderer.Render(w, "search", data); err != nil {
		// Fallback to inline rendering
		s.renderSearchResultsInline(w, r, query, results, category)
	}
}

// renderSearchResultsInline is a fallback for when templates fail
func (s *Server) renderSearchResultsInline(w http.ResponseWriter, r *http.Request, query string, results *model.SearchResults, category string) {
	baseData := s.newPageData(w, r, query, "search")
	fmt.Fprintf(w, `<!DOCTYPE html>
<html lang="%s" dir="%s" class="theme-%s">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>%s - %s</title>
    <link rel="stylesheet" href="/static/css/common.css">
    <link rel="stylesheet" href="/static/css/components.css">
    <link rel="stylesheet" href="/static/css/public.css">
        <h1>Results for: %s</h1>
        <p>%d results (%.3fs)</p>`,
		html.EscapeString(baseData.Lang),
		html.EscapeString(baseData.Dir),
		html.EscapeString(baseData.Theme),
		html.EscapeString(query),
		html.EscapeString(s.config.Server.Title),
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
