package server

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"html"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/apimgr/search/src/alert"
	"github.com/apimgr/search/src/api"
	"github.com/apimgr/search/src/backup"
	"github.com/apimgr/search/src/cache"
	"github.com/apimgr/search/src/common/httputil"
	"github.com/apimgr/search/src/common/i18n"
	"github.com/apimgr/search/src/common/qr"
	"github.com/apimgr/search/src/config"
	"github.com/apimgr/search/src/database"
	"github.com/apimgr/search/src/direct"
	"github.com/apimgr/search/src/email"
	"github.com/apimgr/search/src/geoip"
	graphqlpkg "github.com/apimgr/search/src/graphql"
	"github.com/apimgr/search/src/i2p"
	"github.com/apimgr/search/src/instant"
	"github.com/apimgr/search/src/logging"
	"github.com/apimgr/search/src/model"
	"github.com/apimgr/search/src/notify"
	"github.com/apimgr/search/src/owner"
	"github.com/apimgr/search/src/scheduler"
	"github.com/apimgr/search/src/search"
	"github.com/apimgr/search/src/search/bang"
	"github.com/apimgr/search/src/search/engine"
	"github.com/apimgr/search/src/security"
	"github.com/apimgr/search/src/service"
	"github.com/apimgr/search/src/ssl"
	"github.com/apimgr/search/src/widget"
	"github.com/go-chi/chi/v5"
)

// Server represents the HTTP server
type Server struct {
	config            *config.Config
	publicIPProviders []string
	httpServer        *http.Server
	// For dual port mode
	httpsServer       *http.Server
	preboundListeners map[int]net.Listener
	preboundRedirect  net.Listener
	redirectServer    *http.Server
	pidFile           string
	registry          *engine.Registry
	aggregator        *search.Aggregator
	startTime         time.Time
	middleware        *Middleware
	rateLimiter       *RateLimiter
	csrf              *CSRFMiddleware
	renderer          *TemplateRenderer
	apiHandler        *api.Handler
	torService        *service.TorService
	bangManager       *bang.Manager
	widgetManager     *widget.Manager
	relatedSearches   *search.RelatedSearches
	logManager        *logging.Manager
	tlsManager        *ssl.Manager
	instantManager    *instant.Manager
	directManager     *direct.Manager
	geoipLookup       *geoip.Lookup
	mailer            *email.Mailer
	notifyManager     *notify.Manager
	notifyCancel      context.CancelFunc
	scheduler         *scheduler.Scheduler
	metrics           *Metrics
	dbManager         *database.DatabaseManager
	alertManager      *alert.Manager
	ownerManager      *owner.Manager
	blocklistManager  *security.BlocklistManager
	cveManager        *security.CVEManager
	// Per AI.md PART 5: config sync persists settings back to server.yml
	configSync *config.ConfigSync

	// Internationalization per AI.md PART 32
	i18nManager *i18n.Manager

	// Debug accessors per AI.md PART 6
	router chi.Router
	cache  *search.ResultCache
	db     *sql.DB

	// Loopback port of the Tor PROXY-protocol backend listener; 0 when Tor is
	// not running. Mirrors Metrics.torBackendPort so middleware can identify
	// hidden-service requests per AI.md PART 12/31. Set after StartTorService.
	torBackendPort atomic.Int64

	// I2P eepsite manager per AI.md PART 31.2 — opt-in, disabled by default
	i2pManager *i2p.I2PManager

	// Set at the top of Shutdown per AI.md PART 13: the server reports
	// status "shutting_down" (503) once it begins draining.
	shuttingDown atomic.Bool
}

// NewServer creates a new server instance with the production public-IP providers.
func NewServer(cfg *config.Config) *Server {
	return newServer(cfg, append([]string(nil), publicIPProviders...))
}

// NewServerWithPublicIPProviders creates a server with an explicit public-IP
// provider list. An empty list disables outbound public-IP discovery, which is
// useful for deterministic tests and deployments that supply this value by
// another mechanism.
func NewServerWithPublicIPProviders(cfg *config.Config, providers []string) *Server {
	return newServer(cfg, append([]string{}, providers...))
}

func newServer(cfg *config.Config, providers []string) *Server {
	// Per AI.md PART 21: resolve and cache the backup dir once at startup —
	// cleanup/retention logic must never re-resolve the path later
	backup.SetBackupDir(config.GetBackupDir())

	// Create logging manager
	logDir := config.GetLogDir()
	logMgr := logging.NewManager(logDir)

	// Configure log levels based on config
	switch cfg.Server.Logs.Level {
	case "debug":
		logMgr.Server().SetLevel(logging.LevelDebug)
	case "warn":
		logMgr.Server().SetLevel(logging.LevelWarn)
	case "error":
		logMgr.Server().SetLevel(logging.LevelError)
	}

	// Configure access log format
	switch cfg.Server.Logs.Access.Format {
	case "json":
		logMgr.Access().SetFormat("json")
	case "common":
		logMgr.Access().SetFormat("common")
	}

	// Create engine registry with default engines
	registry := engine.DefaultRegistry()

	// Apply any operator-configured engine credentials (API keys, OAuth
	// client_id/client_secret, contact email) from server.yml onto the
	// matching registered engine's config so they take effect at runtime.
	applyEngineCredentials(registry, cfg.Engines)

	// Get all enabled engines (already filtered by IsEnabled())
	enabledEngines := registry.GetEnabled()

	// Create cache backend based on config
	// Per AI.md PART 9: Redis/Valkey cache wire-up when configured
	var cacheBackend cache.Cache
	cacheType := strings.ToLower(cfg.Server.Cache.Type)
	if cacheType == "redis" || cacheType == "valkey" {
		timeout := 5 * time.Second
		if cfg.Server.Cache.Timeout != "" {
			if d, err := time.ParseDuration(cfg.Server.Cache.Timeout); err == nil {
				timeout = d
			}
		}
		redisCfg := &cache.RedisConfig{
			Type:     cacheType,
			URL:      cfg.Server.Cache.URL,
			Host:     cfg.Server.Cache.Host,
			Port:     cfg.Server.Cache.Port,
			Password: cfg.Server.Cache.Password,
			DB:       cfg.Server.Cache.DB,
			PoolSize: cfg.Server.Cache.PoolSize,
			MinIdle:  cfg.Server.Cache.MinIdle,
			Timeout:  timeout,
			Prefix:   cfg.Server.Cache.Prefix,
		}
		if redisCfg.Prefix == "" {
			redisCfg.Prefix = "search:"
		}
		redisCache, err := cache.NewRedisCache(redisCfg)
		if err != nil {
			slog.Warn("Redis cache init failed, using memory cache", "err", err)
		} else {
			if err := redisCache.Ping(context.Background()); err != nil {
				slog.Warn("Redis ping failed, using memory cache", "err", err)
				if err := redisCache.Close(); err != nil {
					slog.Warn("failed to close inactive Redis cache", "error", err)
				}
			} else {
				cacheBackend = redisCache
				slog.Info("Redis cache enabled", "type", cacheType)
			}
		}
	}

	// Create aggregator with 30 second timeout and caching
	aggregator := search.NewAggregator(enabledEngines, search.AggregatorConfig{
		Timeout:       time.Duration(cfg.Search.Timeout) * time.Second,
		CacheEnabled:  true,
		CacheTTL:      5 * time.Minute,
		MaxConcurrent: cfg.Search.MaxConcurrent,
		Cache:         cacheBackend,
	})

	// Create middleware with logging
	mw := NewMiddleware(cfg, logMgr)

	// Wire additional trusted proxies into the httputil trust gate.
	// Private ranges (loopback, RFC1918, link-local) are always trusted without config.
	httputil.SetAdditionalTrustedProxies(cfg.Server.TrustedProxies.Additional)
	// Re-sync on every hot-reload so a live server.yml change takes effect immediately.
	// Tracks the last-applied SMTP config so only real SMTP changes are audited.
	lastSMTP := cfg.Server.Notifications.Email.SMTP
	cfg.OnReload(func(c *config.Config) {
		httputil.SetAdditionalTrustedProxies(c.Server.TrustedProxies.Additional)

		// Per AI.md audit table: an SMTP settings change is audited as
		// config.smtp_updated with the source IP (never the credentials).
		prevSMTP := lastSMTP
		cur := c.Server.Notifications.Email.SMTP
		if cur.Host != prevSMTP.Host || cur.Port != prevSMTP.Port ||
			cur.Username != prevSMTP.Username || cur.Password != prevSMTP.Password ||
			cur.TLS != prevSMTP.TLS {
			if al := logMgr.Audit(); al != nil {
				al.LogSMTPUpdated("operator", "-")
			}
		}
		lastSMTP = cur
	})

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

	// Create I2P manager per AI.md PART 31.2 — opt-in only, disabled by default.
	// Started in StartHTTPServer when config enables it.
	var i2pMgr *i2p.I2PManager
	if cfg.Server.I2P.Enabled {
		i2pMgr = i2p.NewI2PManager(context.Background(),
			&i2p.I2PConfig{
				Enabled:          cfg.Server.I2P.Enabled,
				Binary:           cfg.Server.I2P.Binary,
				SAMAddress:       cfg.Server.I2P.SAMAddress,
				VirtualPort:      cfg.Server.I2P.VirtualPort,
				InboundLength:    cfg.Server.I2P.InboundLength,
				OutboundLength:   cfg.Server.I2P.OutboundLength,
				InboundQuantity:  cfg.Server.I2P.InboundQuantity,
				OutboundQuantity: cfg.Server.I2P.OutboundQuantity,
			},
			fmt.Sprintf("%s:%d", cfg.Server.Address, cfg.Server.Port))
	}

	// Set I2P manager on API handler for health checks per AI.md PART 31.2
	apiHandler.SetI2PManager(i2pMgr)

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
		tlsMgr = ssl.NewManagerWithSecretAndHost(&cfg.Server.SSL, dataDir, cfg.Server.SecretKey, cfg.Server.FQDN)
	}

	// Create GeoIP lookup if enabled (uses MMDB from sapics/ip-location-db)
	// Per AI.md PART 19: dir defaults to {data_dir}/security/geoip via config.GetGeoIPDir()
	var geoLookup *geoip.Lookup
	if cfg.Server.GeoIP.Enabled {
		geoDir := cfg.Server.GeoIP.Dir
		if geoDir == "" {
			geoDir = config.GetGeoIPDir()
		}
		geoCfg := &geoip.Config{
			Enabled:          true,
			Dir:              geoDir,
			DenyCountries:    cfg.Server.GeoIP.DenyCountries,
			AllowedCountries: cfg.Server.GeoIP.AllowedCountries,
			ASN:              cfg.Server.GeoIP.Databases.ASN,
			Country:          cfg.Server.GeoIP.Databases.Country,
			City:             cfg.Server.GeoIP.Databases.City,
		}
		geoLookup = geoip.NewLookup(geoCfg)
		if err := geoLookup.LoadDatabases(); err != nil {
			slog.Warn("GeoIP database load failed", "err", err)
			geoLookup = nil
		} else {
			slog.Info("GeoIP databases loaded", "dir", geoDir)
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
		widgetMgr.RegisterFetcher(widget.NewCurrencyFetcher())
		widgetMgr.RegisterFetcher(widget.NewDictionaryFetcher())
		widgetMgr.RegisterFetcher(widget.NewNutritionFetcher(""))
		widgetMgr.RegisterFetcher(widget.NewTrackingFetcher())
		widgetMgr.RegisterFetcher(widget.NewTranslateFetcher())
		widgetMgr.RegisterFetcher(widget.NewWikipediaFetcher())
	}

	// Create instant answer manager
	instantMgr := instant.NewManager(widgetMgr)

	// Create direct answer manager (full-page results per IDEA.md)
	directMgr := direct.NewManager()
	directMgr.SetDirectorySearcher(func(ctx context.Context, searchText string) ([]direct.DirectoryResult, error) {
		query := model.NewQuery(searchText)
		query.Category = model.CategoryGeneral
		query.Page = 1
		query.PerPage = 20
		results, err := aggregator.Search(ctx, query)
		if err != nil {
			return nil, err
		}
		directResults := make([]direct.DirectoryResult, 0, len(results.Results))
		for _, result := range results.Results {
			directResults = append(directResults, direct.DirectoryResult{Title: result.Title, URL: result.URL, Description: result.Content})
		}
		return directResults, nil
	})

	// Create email mailer
	// Per AI.md PART 18: Email is auto-enabled if SMTP is configured
	var mailer *email.Mailer
	if cfg.Server.Notifications.Email.SMTP.Host != "" {
		// Per AI.md PART 12 "Contact Configuration": server.contact.admin.email
		// is the required admin notification recipient — "Empty → startup
		// warning. Auto-populated from the operator's email on first install
		// if unset." This is the recipient list SendToAdmins() delivers to.
		if cfg.Server.Contact.Admin.Email == "" {
			slog.Warn("server.contact.admin.email is not configured — admin email notifications will not be delivered")
		}

		if m, ok := email.NewMailerFromConfig(cfg); ok {
			// Per AI.md PART 17 "Email Queue with Retry Logic": SMTP-configured
			// mail must be delivered through the built-in queue, which retries
			// with exponential backoff and logs every success and failure.
			if err := m.StartQueue(context.Background(), email.QueueConfig{}); err != nil {
				slog.Error("Email queue failed to start", "err", err)
			}
			email.SetDefaultMailer(m)
			mailer = m
			slog.Info("Email mailer configured", "smtp_host", cfg.Server.Notifications.Email.SMTP.Host, "smtp_port", cfg.Server.Notifications.Email.SMTP.Port)
		}
	}

	// Scheduler is initialized after Server struct creation per AI.md PART 19
	// The scheduler is ALWAYS RUNNING - no enable/disable option

	// Create database manager for server persistence
	var dbMgr *database.DatabaseManager
	var ownerMgr *owner.Manager
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
		slog.Warn("Database initialization failed", "err", err)
	} else {
		if err := database.InitSchema(context.Background(), dbMgr); err != nil {
			slog.Warn("Database schema initialization failed", "err", err)
		} else {
			slog.Info("Database schema initialized")
			prefix := ""
			if dbMgr.ServerDB().Driver() == "libsql" {
				prefix = "srv_"
			}
			var ownerErr error
			ownerMgr, ownerErr = owner.NewManager(dbMgr.ServerDB().SQL(), prefix, cfg.Server.Security.EncryptionKey, cfg.Server.Tokens.MaxLinked)
			if ownerErr != nil {
				slog.Error("Owner token manager initialization failed", "err", ownerErr)
			} else {
				apiHandler.SetOwnerManager(ownerMgr)
			}
		}
	}

	// Per IDEA.md: privacy is the product — no admin web UI, no sessions,
	// no user accounts. Operator privilege is held by anyone with server.token.

	// Create metrics collector
	metrics := NewMetrics(cfg)
	apiHandler.SetHealthMetrics(metrics)
	rl.SetMetrics(metrics)

	// Initialize i18n manager per AI.md PART 32
	i18nMgr, err := i18n.DefaultManager()
	if err != nil {
		slog.Warn("i18n manager initialization failed, falling back to key passthrough", "err", err)
	} else {
		slog.Info("i18n translations loaded", "language_count", len(i18n.DefaultSupportedLanguages()))
	}

	renderer = NewTemplateRenderer(cfg, i18nMgr)

	var alertMgr *alert.Manager
	if dbMgr != nil && dbMgr.ServerDB() != nil && dbMgr.ServerDB().SQL() != nil {
		alertMgr = alert.NewManager(dbMgr.ServerDB().SQL(), cfg, aggregator, mailer)
	}

	// Create blocklist manager per AI.md PART 18
	blocklistMgr := security.NewBlocklistManagerWithSecurityDir(config.GetDataDir(), cfg.Data.SecurityDir, blocklistSources(cfg.Server.Security.Blocklists))
	mw.SetBlocklistManager(blocklistMgr)
	if handler, exists := directMgr.GetHandler(direct.AnswerTypeSafe); exists {
		if safeHandler, ok := handler.(*direct.SafeHandler); ok {
			safeHandler.SetDomainBlacklistLookup(func(domain string) (bool, bool) {
				return blocklistMgr.IsBlockedDomain(domain), blocklistMgr.DomainCount() > 0
			})
		}
	}
	cfg.OnReload(func(c *config.Config) {
		blocklistMgr.SetSecurityDir(c.Data.SecurityDir)
		blocklistMgr.SetSources(blocklistSources(c.Server.Security.Blocklists))
	})
	// Load any previously downloaded blocklists
	if err := blocklistMgr.LoadFromDisk(); err != nil {
		slog.Warn("blocklist load from disk failed", "err", err)
	}

	// Create CVE manager per AI.md PART 18.
	dataConfig := cfg.Data
	cveSource := strings.TrimSpace(dataConfig.CVE.Source)
	if cveSource == "" {
		cveSource = "https://services.nvd.nist.gov/rest/json/cves/2.0"
	}
	cveMgr := security.NewCVEManagerWithSecurityDir(config.GetDataDir(), dataConfig.SecurityDir, []security.CVESource{{Name: "nvd", URL: cveSource}})
	cfg.OnReload(func(c *config.Config) {
		source := strings.TrimSpace(c.Data.CVE.Source)
		if source == "" {
			source = "https://services.nvd.nist.gov/rest/json/cves/2.0"
		}
		cveMgr.SetSources([]security.CVESource{{Name: "nvd", URL: source}})
		if err := cveMgr.SetSecurityDir(c.Data.SecurityDir); err != nil {
			slog.Warn("CVE data reload after security directory change failed", "err", err)
		}
	})
	// Load any previously downloaded CVE data
	if err := cveMgr.LoadFromDisk(); err != nil {
		slog.Warn("CVE data load from disk failed", "err", err)
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
		config:            cfg,
		publicIPProviders: providers,
		registry:          registry,
		aggregator:        aggregator,
		middleware:        mw,
		rateLimiter:       rl,
		csrf:              csrf,
		renderer:          renderer,
		apiHandler:        apiHandler,
		torService:        torSvc,
		bangManager:       bangMgr,
		widgetManager:     widgetMgr,
		logManager:        logMgr,
		tlsManager:        tlsMgr,
		instantManager:    instantMgr,
		directManager:     directMgr,
		geoipLookup:       geoLookup,
		mailer:            mailer,
		// scheduler is initialized below after Server creation
		metrics:          metrics,
		dbManager:        dbMgr,
		alertManager:     alertMgr,
		ownerManager:     ownerMgr,
		blocklistManager: blocklistMgr,
		cveManager:       cveMgr,
		i18nManager:      i18nMgr,
		i2pManager:       i2pMgr,
		// Debug accessors per AI.md PART 6
		cache: resultCache,
		db:    serverDB,
	}
	s.notifyManager = notify.NewManager(func() *config.Config { return s.config }, nil, nil)
	s.middleware.SetNotificationManager(s.notifyManager)

	// Per AI.md PART 5: server.yml is the source of truth (single-instance, AI.md line 2055)
	if dbMgr != nil {
		configPath := cfg.GetPath()
		if configPath == "" {
			configPath = "/etc/search/server.yml"
		}
		s.configSync = config.NewConfigSync(dbMgr.ServerDB().SQL(), cfg, configPath)
	}

	// Share contact delivery with the REST and GraphQL APIs.
	s.apiHandler.SetContactSubmitter(s)
	if s.config.Server.ImageProxy.Enabled {
		s.apiHandler.SetThumbnailRewriter(s.proxyImageURL)
	}

	// Set widget manager on API handler
	s.apiHandler.SetWidgetManager(widgetMgr)

	// Set instant answer manager on API handler
	s.apiHandler.SetInstantManager(instantMgr)

	// Set direct answer manager on API handler
	s.apiHandler.SetDirectManager(directMgr)

	// Set related searches provider on API handler
	s.relatedSearches = search.NewRelatedSearches()
	s.apiHandler.SetRelatedSearches(s.relatedSearches)
	s.apiHandler.SetAlertManager(alertMgr)
	s.apiHandler.SetGeoIPLookup(s.geoipLookup)

	// Initialize scheduler - ALWAYS RUNNING per AI.md PART 19
	// Use server.db for persistent task state if available
	var schedulerDB *sql.DB
	if dbMgr != nil {
		schedulerDB = dbMgr.ServerDB().SQL()
	}
	s.initScheduler(schedulerDB)
	s.apiHandler.SetHealthProvider(s)

	// Wire the scheduler and audit logger into the API handler so the
	// /api/v1/server/scheduler/* endpoints (AI.md PART 18) work.
	s.apiHandler.SetScheduler(s.scheduler)
	if s.logManager != nil {
		s.apiHandler.SetAuditLogger(s.logManager.Audit())
	}

	return s
}

// applyEngineCredentials copies operator-configured credentials (API keys, OAuth
// client_id/client_secret, contact email) from server.yml's `engines:` section onto
// the matching registered engine's model.EngineConfig.Settings map, so engines that
// support authenticated/higher-rate-limit access (github, stackoverflow, pubmed,
// reddit, openstreetmap) can read them at search time. Engines/keys not present in cfg are left
// untouched — this never disables or reconfigures an engine's enabled/priority state.
func applyEngineCredentials(registry *engine.Registry, engines map[string]config.EngineConfig) {
	for name, ov := range engines {
		eng, err := registry.Get(name)
		if err != nil {
			continue
		}

		mc := eng.GetConfig()
		if mc.Settings == nil {
			mc.Settings = make(map[string]interface{})
		}
		if ov.APIKey != "" {
			mc.Settings["api_key"] = ov.APIKey
		}
		if ov.ClientID != "" {
			mc.Settings["client_id"] = ov.ClientID
		}
		if ov.ClientSecret != "" {
			mc.Settings["client_secret"] = ov.ClientSecret
		}
		if ov.ContactEmail != "" {
			mc.Settings["contact_email"] = ov.ContactEmail
		}
	}
}

// TorAddress returns the active .onion address, or empty string if Tor is not running.
// Per AI.md PART 31: available after StartHTTPServer signals readyCh.
func (s *Server) TorAddress() string {
	if s.torService == nil || !s.torService.IsRunning() {
		return ""
	}
	return s.torService.GetOnionAddress()
}

// SetPreboundListeners supplies sockets bound during privileged startup. The
// server consumes them by configured port after the process drops privileges.
func (s *Server) SetPreboundListeners(listeners map[int]net.Listener, redirect net.Listener) {
	s.preboundListeners = listeners
	s.preboundRedirect = redirect
}

func (s *Server) listen(port int, addr string) (net.Listener, error) {
	if listener, ok := s.preboundListeners[port]; ok {
		delete(s.preboundListeners, port)
		return listener, nil
	}
	return net.Listen("tcp", addr)
}

// StartHTTPServer starts the server and signals readyCh once the TCP socket is bound.
// Per AI.md PART 8 step 17-18: Tor starts first (inside here), then HTTP binds, then readyCh is closed.
// readyCh must be a buffered channel (capacity ≥ 1) or nil to skip the signal.
// The call blocks until shutdown — run it in a goroutine.
func (s *Server) StartHTTPServer(readyCh chan<- struct{}) error {
	// PID file is already written by main.go per AI.md PART 8 step 12
	// Do not create a duplicate PID file here

	// Step 17: Start Tor (before HTTP binds — per AI.md PART 8 startup sequence)
	if s.torService != nil {
		if err := s.torService.StartTorService(); err != nil {
			slog.Warn("Tor service start failed", "err", err)
		}
	}
	// Record the PROXY-protocol backend port so middleware and metrics can
	// identify hidden-service requests and key them on the Tor circuit ID
	// (AI.md PART 12 lines 16098/16109, PART 31 backend-listener rules).
	torBackendPort := 0
	if s.torService != nil {
		torBackendPort = s.torService.BackendPort()
	}
	if s.metrics != nil {
		s.metrics.SetTorBackendPort(torBackendPort)
	}
	s.torBackendPort.Store(int64(torBackendPort))
	// Share the live backend port and the circuit-ID resolver with the
	// middleware so hidden-service requests are keyed on tor:{circuit_id}
	// instead of the loopback peer (AI.md PART 12 lines 16097-16109).
	setMiddlewareTorBackendPort(&s.torBackendPort)
	setMiddlewareCircuitResolver(func(r *http.Request) string {
		return s.torIdentity(r)
	})
	if s.torService != nil && s.torService.IsRunning() {
		slog.Info("Tor hidden service active", "onion_address", s.torService.GetOnionAddress())
	}

	// Start the I2P eepsite when enabled per AI.md PART 31.2 (opt-in).
	// Monitor watches tunnel state so healthz reports live status.
	if s.i2pManager != nil {
		if err := s.i2pManager.Start(); err != nil {
			slog.Warn("I2P eepsite start failed", "err", err)
		} else {
			go s.i2pManager.Monitor()
			if addr := s.i2pManager.EepsiteAddress(); addr != "" {
				slog.Info("I2P eepsite active", "hostname", addr)
			}
		}
	}

	// Scheduler is already started by initScheduler() per AI.md PART 19
	// The scheduler is ALWAYS RUNNING - no enable/disable check needed

	// Setup routes
	mux := s.setupRoutes()

	// Resolve HTTP port (0 means random port in 64xxx range)
	httpPort := s.config.Server.GetHTTPPort()
	if httpPort != s.config.Server.Port {
		slog.Info("Using random HTTP port", "port", httpPort)
		s.config.Server.Port = httpPort
	}

	s.startTime = time.Now()

	// HTTPS configurations must never silently fall back to plaintext HTTP
	// when their certificate is unavailable.
	tlsEnabled := s.tlsManager != nil && s.tlsManager.IsEnabled()
	if (s.config.Server.IsDualPortMode() || s.config.Server.Port == 443) && !tlsEnabled {
		return fmt.Errorf("configured HTTPS port requires a valid TLS certificate")
	}
	// Check for dual port mode
	if s.config.Server.IsDualPortMode() && tlsEnabled {
		return s.startDualPortMode(mux, httpPort, readyCh)
	}

	// Single port mode
	return s.startSinglePortMode(mux, httpPort, readyCh)
}

func (s *Server) startNotifyWorkers() {
	if s.notifyManager == nil || s.notifyCancel != nil {
		return
	}
	workerCtx, cancel := context.WithCancel(context.Background())
	s.notifyCancel = cancel
	s.notifyManager.Start(workerCtx)
}

// startDualPortMode starts both HTTP and HTTPS servers on separate ports.
// Binds both sockets before signaling readyCh so the banner is shown only once both are live.
func (s *Server) startDualPortMode(mux http.Handler, httpPort int, readyCh chan<- struct{}) error {
	httpsPort := s.config.Server.GetHTTPSPort()

	httpAddr := fmt.Sprintf("%s:%d", s.config.Server.Address, httpPort)
	httpsAddr := fmt.Sprintf("%s:%d", s.config.Server.Address, httpsPort)

	// Bind HTTP socket
	httpLn, err := s.listen(httpPort, httpAddr)
	if err != nil {
		return fmt.Errorf("HTTP listen error: %w", err)
	}

	// Bind HTTPS socket
	httpsLn, err := s.listen(httpsPort, httpsAddr)
	if err != nil {
		return fmt.Errorf("HTTPS listen error: %w", errors.Join(err, httpLn.Close()))
	}

	// HTTP-01 ACME challenges arrive on the cleartext listener, so install the
	// challenge handler there and leave HTTPS to serve the issued certificate.
	httpHandler := mux
	if s.tlsManager != nil && s.config.Server.SSL.LetsEncrypt.Enabled {
		httpHandler = s.tlsManager.GetHTTPChallengeHandler(mux)
	}
	// Create HTTP server
	// Timeouts come from server.yml server.limits (AI.md "Request Limits"),
	// defaulting to 30s/30s/120s. WriteTimeout must stay >= the search
	// aggregator timeout or net/http resets the connection mid-request
	// instead of the aggregator returning a proper error response.
	s.httpServer = &http.Server{
		Addr:         httpAddr,
		Handler:      httpHandler,
		ReadTimeout:  s.config.Server.Limits.GetReadTimeout(),
		WriteTimeout: s.config.Server.Limits.GetWriteTimeout(),
		IdleTimeout:  s.config.Server.Limits.GetIdleTimeout(),
	}

	// Create HTTPS server
	httpsHandler := mux
	s.httpsServer = &http.Server{
		Addr:         httpsAddr,
		Handler:      httpsHandler,
		ReadTimeout:  s.config.Server.Limits.GetReadTimeout(),
		WriteTimeout: s.config.Server.Limits.GetWriteTimeout(),
		IdleTimeout:  s.config.Server.Limits.GetIdleTimeout(),
	}
	if s.tlsManager != nil {
		s.httpsServer.TLSConfig = s.tlsManager.GetTLSConfig()
	}

	slog.Info("Dual port mode enabled")
	slog.Info("HTTP server listening", "addr", "http://"+httpAddr)
	slog.Info("HTTPS server listening", "addr", "https://"+httpsAddr)

	// Both sockets bound — signal readiness before blocking
	s.startNotifyWorkers()
	if readyCh != nil {
		close(readyCh)
	}

	// Start HTTP server in goroutine
	errChan := make(chan error, 2)
	go func() {
		if err := s.httpServer.Serve(httpLn); err != nil && err != http.ErrServerClosed {
			errChan <- fmt.Errorf("HTTP server error: %w", err)
		}
	}()

	// Start HTTPS server (blocking via ServeTLS with pre-bound listener)
	if err := s.httpsServer.ServeTLS(httpsLn, "", ""); err != nil && err != http.ErrServerClosed {
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

// startSinglePortMode starts server on a single port (HTTP or HTTPS).
// Binds the socket via net.Listen first, then signals readyCh so the banner
// is shown only after the port is actually accepting connections.
func (s *Server) startSinglePortMode(mux http.Handler, port int, readyCh chan<- struct{}) error {
	addr := fmt.Sprintf("%s:%d", s.config.Server.Address, port)

	s.httpServer = &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  s.config.Server.Limits.GetReadTimeout(),
		WriteTimeout: s.config.Server.Limits.GetWriteTimeout(),
		IdleTimeout:  s.config.Server.Limits.GetIdleTimeout(),
	}

	if s.tlsManager != nil && s.tlsManager.IsEnabled() {
		s.httpServer.TLSConfig = s.tlsManager.GetTLSConfig()

		// Bind the TLS socket before signaling readiness
		ln, err := s.listen(port, addr)
		if err != nil {
			return fmt.Errorf("server listen error: %w", err)
		}

		slog.Info("Server listening", "addr", "https://"+addr)

		// Start HTTP->HTTPS redirect server on port 80 if configured
		if s.config.Server.SSL.AutoTLS {
			redirectAddr := net.JoinHostPort(s.config.Server.Address, "80")
			redirectHandler := ssl.HTTPSRedirectHandler(port)
			if s.config.Server.SSL.LetsEncrypt.Enabled {
				redirectHandler = s.tlsManager.GetHTTPChallengeHandler(redirectHandler)
			}
			if s.preboundRedirect != nil {
				s.redirectServer = ssl.StartHTTPSRedirectListenerWithHandler(s.preboundRedirect, port, redirectHandler)
				s.preboundRedirect = nil
			} else {
				s.redirectServer = ssl.StartHTTPSRedirectWithHandler(redirectAddr, port, redirectHandler)
			}
		}

		// Socket is bound — signal readiness
		s.startNotifyWorkers()
		if readyCh != nil {
			close(readyCh)
		}

		if err := s.httpServer.ServeTLS(ln, "", ""); err != nil && err != http.ErrServerClosed {
			s.removePIDFile()
			return fmt.Errorf("server error: %w", err)
		}
	} else {
		ln, err := s.listen(port, addr)
		if err != nil {
			return fmt.Errorf("server listen error: %w", err)
		}

		slog.Info("Server listening", "addr", "http://"+addr)

		// Socket is bound — signal readiness
		s.startNotifyWorkers()
		if readyCh != nil {
			close(readyCh)
		}

		if err := s.httpServer.Serve(ln); err != nil && err != http.ErrServerClosed {
			s.removePIDFile()
			return fmt.Errorf("server error: %w", err)
		}
	}

	return nil
}

// Shutdown gracefully shuts down the server with a context
func (s *Server) Shutdown(ctx context.Context) error {
	// Flag first per AI.md PART 13: healthz reports "shutting_down" (503)
	// from the moment draining begins.
	s.shuttingDown.Store(true)
	s.logManager.Server().Info("Server shutting down...")
	if s.notifyCancel != nil {
		s.notifyCancel()
	}

	// Stop scheduler
	if s.scheduler != nil {
		s.scheduler.StopTaskScheduler()
		slog.Info("Scheduler stopped")
	}

	// Drain and stop the email delivery queue. StopQueue drains messages
	// already accepted before workers exit (AI.md PART 17: queue shutdown).
	if s.mailer != nil {
		s.mailer.StopQueue()
	}

	// Stop Tor service
	if s.torService != nil {
		if err := s.torService.StopTorService(); err != nil {
			slog.Warn("failed to stop Tor service during shutdown", "error", err)
		}
	}

	// Stop the I2P eepsite manager per AI.md PART 31.2
	if s.i2pManager != nil {
		if err := s.i2pManager.Close(); err != nil {
			slog.Warn("failed to stop I2P service during shutdown", "error", err)
		}
	}

	// Stop HTTP->HTTPS redirect server if running
	if s.redirectServer != nil {
		if err := s.redirectServer.Shutdown(ctx); err != nil {
			slog.Warn("failed to stop redirect server", "error", err)
		}
	}

	// Remove PID file
	s.removePIDFile()

	// Shutdown HTTPS server if running (dual port mode)
	if s.httpsServer != nil {
		if err := s.httpsServer.Shutdown(ctx); err != nil {
			slog.Error("HTTPS server shutdown error", "err", err)
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
			slog.Error("Database close error", "err", err)
		}
	}

	// Close log files
	if err := s.logManager.Close(); err != nil {
		slog.Warn("failed to close log manager", "error", err)
	}

	slog.Info("Server stopped gracefully")
	return nil
}

// UpdateConfig updates the server configuration
func (s *Server) UpdateConfig(cfg *config.Config) {
	s.config = cfg
	slog.Info("Server configuration updated")
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
	// Per AI.md PART 16: Theme read from cookie. "auto" renders the theme-auto class so
	// the no-JS CSS fallback (@media (prefers-color-scheme: light)) can resolve it; JS
	// additionally applies the live system preference on page load when mode is "auto".
	themeMode := GetTheme(r)
	if prefs.Theme != "" {
		themeMode = prefs.Theme
	}
	data.ThemeMode = themeMode
	data.Theme = themeMode
	data.PrefsQuery = prefsQuery
	// CSRF token needed by the no-JS theme-switch form in the header partial, which
	// renders on every page that uses newPageData. Issues the csrf_token cookie
	// (stateless double-submit per AI.md PART 16) once per page render.
	data.CSRFToken = s.getCSRFToken(w, r)
	data.RequestPath = r.URL.RequestURI()
	// Per AI.md URL & FQDN Detection: embedded template URLs must use {proto}://{fqdn}/path,
	// never a bare /path — set once here so every page template can build full URLs.
	data.ServerURL = s.getBaseURL(r)
	if prefs.DefaultCategory != "" {
		data.Category = prefs.DefaultCategory.String()
	}
	// Set Tor status per AI.md PART 32
	if s.torService != nil {
		data.TorEnabled = true
		if s.torService.IsRunning() {
			data.OnionAddress = s.torService.GetOnionAddress()
			if data.OnionAddress != "" {
				data.TorStatus = "connected"
			} else {
				data.TorStatus = "connecting..."
			}
		} else {
			data.TorStatus = "starting..."
		}
	}
	// Set HasConsentCookie: skip banner when a valid cookie_consent cookie exists.
	// A cookie that is present but unparseable is NOT a valid consent, so the
	// banner is re-shown and the visitor can choose again (AI.md PART 16).
	consent := consentFromRequest(r)
	if consent != nil {
		data.HasConsentCookie = true
		data.ConsentPreferences = consent.Preferences
		data.ConsentAnalytics = consent.Analytics
	}
	// Analytics injection is gated on the same consent, per AI.md PART 16:
	// no valid analytics consent means no third-party script in the page.
	data.TrackingHTML = s.trackingScript(r)
	// Reflect the CCPA "Do Not Sell" opt-out cookie so the privacy page can render
	// the correct toggle state (opt-out vs opt-in) without JavaScript.
	if c, err := r.Cookie("ccpa_opt_out"); err == nil && c.Value == "true" {
		data.CCPAOptedOut = true
	}
	// Filter out already-dismissed announcements from the dismissed_announcements cookie
	if dc, err := r.Cookie("dismissed_announcements"); err == nil && dc.Value != "" {
		dismissed := strings.Split(dc.Value, ",")
		dmSet := make(map[string]bool, len(dismissed))
		for _, id := range dismissed {
			dmSet[strings.TrimSpace(id)] = true
		}
		filtered := data.Announcements[:0]
		for _, a := range data.Announcements {
			if !dmSet[a.ID] {
				filtered = append(filtered, a)
			}
		}
		data.Announcements = filtered
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
		if err := os.Remove(s.pidFile); err != nil && !os.IsNotExist(err) {
			slog.Warn("failed to remove server PID file", "error", err)
		}
	}
}

// csrfProtect wraps a browser-facing handler with stateless double-submit CSRF
// validation per AI.md PART 16. Safe methods and bypassed requests (bearer auth,
// websockets, exempt paths) pass through and get a token cookie issued;
// mutating browser requests that fail validation receive the canonical 403
// CSRF_FAILED body.
func (s *Server) csrfProtect(h http.HandlerFunc) http.HandlerFunc {
	return s.csrf.Protect(h).ServeHTTP
}

// setupRoutes sets up HTTP routes with middleware
func (s *Server) setupRoutes() http.Handler {
	r := chi.NewRouter()
	if s.metrics != nil {
		r.Use(s.metrics.MetricsMiddleware)
	}
	// Store router reference for debug route introspection per AI.md PART 6
	s.router = r
	// chi NotFound handler: stdlib mux's "/" matched everything; with chi we
	// route "/" exactly and dispatch other unmatched paths to handleNotFound.
	r.NotFound(s.handleNotFound)

	// Health check endpoints per AI.md PART 13
	// Canonical route: /server/healthz (content-negotiated HTML/JSON/text)
	// Root alias (/healthz) is opt-in via server.healthz.root.enabled in server.yml (default: false)
	// Versioned /api/v1/server/healthz and direct /api/healthz are registered by apiHandler.RegisterRoutes().
	r.HandleFunc("/server/healthz", s.handleHealthz)
	r.HandleFunc("/server/healthz.txt", s.handleHealthz)
	if s.config.Server.Healthz.Root.Enabled {
		r.HandleFunc("/healthz", s.handleHealthz)
		r.HandleFunc("/healthz.txt", s.handleHealthz)
	}
	r.HandleFunc("/readyz", s.handleReadyz)
	r.HandleFunc("/livez", s.handleLivez)

	r.Get("/s/{code}", s.handleQRShortlinkRedirect)
	if s.config.Server.ImageProxy.Enabled {
		r.Get("/image-proxy", newImageProxyHandler(s.config.Server.ImageProxy, s.config.Server.SecretKey).ServeHTTP)
	}

	// Home page (root catch-all)
	r.HandleFunc("/", s.handleHome)

	// Search
	r.HandleFunc("/search", s.handleSearch)
	// Alert routes mutate on POST; CSRF-protect them (GET renders pass through).
	r.HandleFunc("/alerts/new", s.csrfProtect(s.handleAlertNew))
	r.HandleFunc("/alerts", s.csrfProtect(s.handleAlerts))
	r.HandleFunc("/alerts/*", s.csrfProtect(s.handleAlertAction))

	// Direct answers (full-page results for type:term queries per IDEA.md)
	r.HandleFunc("/direct/cache-download", s.handleCacheDownload)
	r.HandleFunc("/direct/cache-download-archive-today", s.handleArchiveTodayDownload)
	r.HandleFunc("/direct/audio", handleDictionaryAudio)
	r.HandleFunc("/direct/*", s.csrfProtect(s.handleDirect))

	// Autocomplete (per AI.md PART 32 line 28280)
	r.HandleFunc("/autocomplete", s.handleAutocomplete)

	// Operator-gated server management endpoints per API.md PART 13/14
	r.Get("/server/status", s.RequireOperator(s.handleServerStatus))
	r.Get("/server/config", s.RequireOperator(s.handleServerConfig))

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
	r.HandleFunc("/server/contact", s.csrfProtect(s.handleContact))
	r.HandleFunc("/server/help", s.handleHelp)
	r.HandleFunc("/server/terms", s.handleTerms)

	// Coordinated-disclosure security pages per AI.md PART 11 "Public Pages"
	r.HandleFunc("/server/security", s.handleSecurityOverview)
	r.HandleFunc("/server/security/policy", s.handleSecurityPolicy)
	r.HandleFunc("/server/security/thanks", s.handleSecurityThanks)
	r.Get("/server/security/report/{tracking_id}", s.handleSecurityReportStatus)

	// Public browser-report endpoints advertised by the CSP report-uri/report-to,
	// Reporting-Endpoints, Report-To, and NEL headers per AI.md PART 11 "Reporting
	// API" — CSP violations, NEL, deprecation/intervention/crash, and the generic
	// default group all POST here and receive 204 No Content.
	s.registerReportRoutes(api.APIPrefix)

	// robots.txt, sitemap.xml, and security.txt (/.well-known/ only per RFC 9116)
	r.HandleFunc("/robots.txt", s.handleRobotsTxt)
	r.HandleFunc("/sitemap.xml", s.handleSitemap)
	r.HandleFunc("/.well-known/security.txt", s.handleSecurityTxtEnhanced)
	// Project's PGP public key per AI.md PART 11 "Public Pages" table
	r.HandleFunc("/.well-known/pgp-key.asc", s.handlePGPKeyAsc)

	// llms.txt - AI agent discovery file per AI.md PART 14
	// Served at both /.well-known/llms.txt and /llms.txt (alias)
	r.HandleFunc("/.well-known/llms.txt", s.handleLlmsTxt)
	r.HandleFunc("/llms.txt", s.handleLlmsTxt)

	// Well-known URIs per RFC 8615
	// Password change redirect per AI.md PART 11
	r.HandleFunc("/.well-known/change-password", s.handleWellKnownChangePassword)

	// Catch-all for unknown /.well-known/* paths — returns 404 per AI.md PART 11
	// Must be registered after specific handlers
	r.HandleFunc("/.well-known/*", s.handleWellKnownCatchAll)
	r.HandleFunc("/.well-known/", s.handleWellKnownCatchAll)

	// OpenSearch
	if s.config.Search.OpenSearch.Enabled {
		r.HandleFunc("/opensearch.xml", s.handleOpenSearch)
	}

	// Swagger/OpenAPI endpoints are registered by apiHandler.RegisterOpenAPIRoutes() below

	// Bang proxy (for privacy-preserving redirects)
	if s.config.Search.Bangs.Enabled {
		r.HandleFunc("/bang", s.handleBangProxy)
	}

	// Preferences. Per AI.md: canonical path is /server/preferences - a bare
	// /preferences (or /prefs/*) alias must never be kept for "backward
	// compatibility".
	r.HandleFunc("/server/preferences", s.csrfProtect(s.handlePreferences))
	// Preferences-page widget selection is submitted through the same
	// general-settings form (form="general-settings" checkboxes) - one Save
	// action, one route. /preferences/widgets remains for the homepage
	// widget grid's drag-and-drop reorder, which has no form of its own.
	r.Post("/server/preferences/widgets", s.csrfProtect(s.handleWidgetOrderSave))
	r.Post("/server/preferences/token/reveal", s.csrfProtect(s.handleOwnerPrimaryReveal))
	r.Post("/server/preferences/token/rotate", s.csrfProtect(s.handleOwnerPrimaryRotate))
	r.Post("/server/preferences/token/restore", s.csrfProtect(s.handleOwnerRestore))
	r.Post("/server/preferences/tokens", s.csrfProtect(s.handleOwnerLinkedCreate))
	r.Post("/server/preferences/tokens/{id}/reveal", s.csrfProtect(s.handleOwnerLinkedReveal))
	r.Post("/server/preferences/tokens/{id}/rotate", s.csrfProtect(s.handleOwnerLinkedRotate))
	r.Post("/server/preferences/tokens/{id}/update", s.csrfProtect(s.handleOwnerLinkedUpdate))
	r.Post("/server/preferences/tokens/{id}/revoke", s.csrfProtect(s.handleOwnerLinkedRevoke))
	// Cross-device preference sync (stateless export/import, no CSRF needed - GET only)
	r.Get("/server/preferences/export", s.handlePreferencesExport)
	r.Get("/server/preferences/import", s.handlePreferencesImport)

	// No-JS theme switch (header partial fallback per AI.md PART 16)
	r.Post("/theme", s.csrfProtect(s.handleThemeSet))

	// Cookie consent and CCPA per AI.md PART 16/PART 12
	// POST /server/consent: sets cookie_consent JSON cookie (accept/decline/save), redirects back
	r.Post("/server/consent", s.csrfProtect(s.handleConsent))
	// POST /server/ccpa: sets/clears ccpa_opt_out cookie, redirects back
	r.Post("/server/ccpa", s.csrfProtect(s.handleConsentCCPA))
	// POST /announcements/dismiss: appends id to dismissed_announcements cookie, redirects back
	r.Post("/announcements/dismiss", s.csrfProtect(s.handleAnnouncementDismiss))

	// PWA root routes per AI.md PART 16: the service worker must be served from
	// the root so its scope covers the whole origin; the manifest and offline
	// page live at the root alongside it.
	r.HandleFunc("/sw.js", s.handleServiceWorker)
	r.HandleFunc("/manifest.json", s.handleManifest)
	r.HandleFunc("/offline.html", s.handleOfflinePage)

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
	graphqlDependencies := graphqlpkg.Dependencies{
		Aggregator:   s.aggregator,
		Direct:       s.directManager,
		Instant:      s.instantManager,
		StartedAt:    s.startTime,
		Autocomplete: s.apiHandler.AutocompleteSuggestions,
		RelatedSearches: func(ctx context.Context, query string, limit int) ([]string, error) {
			return s.relatedSearches.GetRelated(ctx, query, limit)
		},
		Widgets:           s.widgetManager,
		API:               s.apiHandler,
		ServerInfo:        s.apiHandler.Info,
		GeoIPLookup:       s.apiHandler.LookupGeoIP,
		OperatorStatus:    s.apiHandler.OperatorServerStatus,
		OperatorConfig:    s.apiHandler.OperatorServerConfig,
		Preferences:       func() interface{} { return s.apiHandler.PreferenceSchema() },
		SavePreferences:   func() interface{} { return s.apiHandler.SavePreferences() },
		EngineInventory:   s.apiHandler.EngineInventory,
		ThumbnailRewriter: s.apiHandler.RewriteThumbnail,
	}
	r.Post("/api/graphql", graphqlpkg.QueryHandlerWithDependencies(s.config, graphqlDependencies))
	// POST /api/{api_version}/server/graphql → GraphQL queries (versioned canonical)
	r.Post(api.APIPrefix+"/server/graphql", graphqlpkg.QueryHandlerWithDependencies(s.config, graphqlDependencies))

	// OpenAPI/Swagger routes
	s.apiHandler.RegisterOpenAPIRoutes(r)

	// Metrics endpoints (Prometheus/Grafana/Loki) per AI.md PART 20 —
	// every alias invokes the same handler, never a redirect.
	if s.config.Server.Metrics.Enabled {
		s.registerMetricsRoutes(r)
	}

	// Debug endpoints (DEBUG=true only)
	// Per AI.md PART 7: pprof, expvar, and custom debug endpoints
	s.registerDebugRoutes(r)

	// Internal Tor control endpoints (loopback-only) per AI.md PART 31 —
	// used by `search tor <subcommand>` to reach the embedded Tor process
	// owned by this running server. Never public, never documented.
	s.registerTorInternalRoutes(r)

	// Apply middleware chain per AI.md PART 5 execution order (1→10).
	// Chain() iterates from last to first, so index 0 = outermost = first to execute.
	// Middleware order per AI.md PART 5 (NON-NEGOTIABLE):
	// Recovery → URLNormalize(1) → RequestID(2) → PathSecurity(3) →
	// SecurityHeaders(4) → SecGPC → CORS → Allowlist(5) → Blocklist(6) →
	// RateLimit(7) → GeoIP(8) → Logging(10). Auth(9) is per-route.
	handler := Chain(
		r,
		// outermost: catches all panics
		s.middleware.Recovery,
		// 1. normalize URLs (trailing slash, etc.)
		URLNormalizeMiddleware,
		// 2. attach request ID (before logging)
		s.middleware.RequestID,
		// 3. validate paths, block traversal
		PathSecurityMiddleware,
		// 4. add security headers
		s.middleware.SecurityHeaders,
		// 4b. honor Sec-GPC privacy signal
		s.middleware.SecGPC,
		// 4c. validate Sec-Fetch-* headers (defense-in-depth CSRF/clickjacking)
		s.middleware.SecFetch,
		// 4d. CORS (near security headers; handles preflight)
		s.middleware.CORS,
		// 5. set allowlisted flag (bypasses 6/7/8, not auth)
		s.middleware.Allowlist,
		// 6. IP/domain blocklist check
		s.middleware.Blocklist,
		// 7. rate limiting
		s.middleware.RateLimit(s.rateLimiter),
		// 8. country blocking
		s.middleware.GeoBlock(s.geoipLookup),
		// 10. log requests (innermost of spec chain)
		s.middleware.Logger,
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
	writeResponseText(w, "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n")
	writeResponseText(w, "<urlset xmlns=\"http://www.sitemaps.org/schemas/sitemap/0.9\">\n")

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
		writeResponseText(w, "  <url>\n")
		writeResponseText(w, fmt.Sprintf("    <loc>%s%s</loc>\n", baseURL, entry.loc))
		writeResponseText(w, fmt.Sprintf("    <lastmod>%s</lastmod>\n", lastMod))
		writeResponseText(w, fmt.Sprintf("    <changefreq>%s</changefreq>\n", entry.changefreq))
		writeResponseText(w, fmt.Sprintf("    <priority>%s</priority>\n", entry.priority))
		writeResponseText(w, "  </url>\n")
	}

	writeResponseText(w, "</urlset>\n")
}

// handleSearch handles search requests
// resolveUnits resolves the global unit system ("metric" or "imperial") for
// a request: a client-side "units" cookie (mirroring the visitor's
// localStorage preference, consistent with other client-side widget
// preferences) overrides the operator-configured server default.
func (s *Server) resolveUnits(r *http.Request) string {
	if c, err := r.Cookie("search_pref_units"); err == nil {
		if c.Value == "metric" || c.Value == "imperial" {
			return c.Value
		}
	}
	if s.config != nil && (s.config.Search.Widgets.Weather.Units == "metric" || s.config.Search.Widgets.Weather.Units == "imperial") {
		return s.config.Search.Widgets.Weather.Units
	}
	return "imperial"
}

func (s *Server) resolveWeatherUnits(r *http.Request) string {
	if c, err := r.Cookie("search_widget_weather_units"); err == nil {
		if c.Value == "auto" || c.Value == "metric" || c.Value == "imperial" {
			return c.Value
		}
	}
	return "auto"
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	prefs := parseSearchPreferences(r.URL.Query().Get("prefs"))

	// Sanitize and validate input
	queryStr := sanitizeInput(strings.TrimSpace(r.URL.Query().Get("q")))
	if queryStr == "" {
		queryStr = buildAdvancedSearchQuery(r.URL.Query())
	}
	queryStr = appendDirectoryFilterParams(queryStr, r.URL.Query())
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
		perPage = 100
	}
	if strings.TrimSpace(r.URL.Query().Get("safe_search")) == "" {
		safeSearch = prefs.SafeSearch
	}

	if queryStr == "" {
		s.handleError(w, r, http.StatusBadRequest, i18n.RequestString(r, "search.error_title"), i18n.RequestString(r, "search.empty_query"))
		return
	}

	// Check for bang commands
	if s.config.Search.Bangs.Enabled {
		if bangResult := s.bangManager.Parse(queryStr); bangResult != nil {
			// Handle bang search
			if s.config.Search.Bangs.ProxyRequests {
				// Proxy mode: redirect to our bang proxy handler. TargetURL must be
				// query-escaped as a whole value — bang URLs with multiple params
				// (e.g. Google Images' tbm=isch&q=...) contain a literal "&" that
				// would otherwise split into extra top-level query params and get
				// silently dropped by handleBangProxy's r.URL.Query().Get("url").
				http.Redirect(w, r, "/bang?url="+url.QueryEscape(bangResult.TargetURL), http.StatusFound)
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
			target := "/direct/" + string(answerType) + "/" + url.PathEscape(term)
			if language := firstNonEmptyQueryValue(r.URL.Query(), "lang", "as_lang"); language != "" {
				target += "?lang=" + url.QueryEscape(language)
			}
			http.Redirect(w, r, target, http.StatusFound)
			return
		}
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	// Inject client IP so instant handlers (e.g. "what is my ip") can return
	// the requester's address rather than the server's own addresses.
	ctx = instant.WithClientIP(ctx, getClientIPSimple(r))

	// Inject GeoIP lookup so instant handlers can enrich IP answers with
	// country, city, ASN, and timezone data when the MMDB is loaded.
	ctx = instant.WithGeoIPLookup(ctx, s.geoipLookup)

	// Inject the resolved request language so instant handlers can return
	// translated Title/Content text.
	ctx = instant.WithLang(ctx, s.getI18nManager().DetectLanguage(r))

	// Inject the global unit system ("metric" or "imperial") so instant
	// handlers such as the unit converter can pick a conversion direction
	// when a query doesn't specify a target unit. A client-side "units"
	// cookie (set from the visitor's localStorage preference) overrides the
	// operator-configured server default.
	ctx = instant.WithUnits(ctx, s.resolveUnits(r))
	ctx = instant.WithWeatherUnits(ctx, s.resolveWeatherUnits(r))

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
	query.Language = firstNonEmptyQueryValue(r.URL.Query(), "lang", "as_lang")
	if query.Language == "" {
		query.Language = s.getI18nManager().DetectLanguage(r)
	}
	query.Region = firstNonEmptyQueryValue(r.URL.Query(), "region", "as_region")
	query.TimeRange = firstNonEmptyQueryValue(r.URL.Query(), "time_range", "as_time")
	query.DateAfter = strings.TrimSpace(r.URL.Query().Get("time_from"))
	query.DateBefore = strings.TrimSpace(r.URL.Query().Get("time_to"))

	results, err := s.aggregator.Search(ctx, query)

	if err != nil && !errors.Is(err, model.ErrNoResults) {
		// For HTTP tools, render the error as plain text
		if httputil.IsHttpTool(r) {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			writeResponseFormatted(w, "Search error: %v\n", err)
			return
		}
		// For our CLI, encode the error as JSON
		if httputil.IsOurCliClient(r) {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			writeResponseFormatted(w, `{"ok":false,"error":"SEARCH_ERROR","message":%q}`+"\n", err.Error())
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		s.renderSearchError(w, r, queryStr, err)
		return
	}

	// 1. Our CLI client — INTERACTIVE, receives JSON, renders its own TUI/GUI
	if httputil.IsOurCliClient(r) {
		s.renderJSONSearchResults(w, results)
		return
	}

	// 2. Text browsers (lynx, w3m, links, elinks) — INTERACTIVE, NO JavaScript
	//    Serve completely different HTML: server-rendered, forms via GET, nav via <a href>
	if httputil.IsTextBrowser(r) {
		data := s.buildSearchPageData(w, r, queryStr, results, category, instantAnswer)
		s.renderNoJSSearch(w, r, data)
		return
	}

	// 3. HTTP tools (curl, wget, httpie) — NON-INTERACTIVE, just dump output
	//    Render full HTML first, then convert to terminal-friendly plain text
	if httputil.IsHttpTool(r) {
		data := s.buildSearchPageData(w, r, queryStr, results, category, instantAnswer)
		s.renderHTMLToText(w, "search", data)
		return
	}

	// 4. Regular browsers — full HTML with JavaScript
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	s.renderSearchResultsWithInstant(w, r, queryStr, results, category, instantAnswer)
}

var advancedSearchFileTypePattern = regexp.MustCompile(`^[a-z0-9]{1,10}$`)

func firstNonEmptyQueryValue(parameters url.Values, names ...string) string {
	for _, name := range names {
		if value := strings.TrimSpace(parameters.Get(name)); value != "" {
			return value
		}
	}
	return ""
}

func buildAdvancedSearchQuery(parameters url.Values) string {
	parts := make([]string, 0, 6)
	if value := strings.TrimSpace(sanitizeInput(parameters.Get("as_allwords"))); value != "" {
		parts = append(parts, value)
	}
	if value := strings.TrimSpace(sanitizeInput(parameters.Get("as_exactphrase"))); value != "" {
		parts = append(parts, `"`+value+`"`)
	}
	if value := strings.TrimSpace(sanitizeInput(parameters.Get("as_anywords"))); value != "" {
		words := strings.Fields(value)
		if len(words) > 0 {
			parts = append(parts, "("+strings.Join(words, " OR ")+")")
		}
	}
	if value := strings.TrimSpace(sanitizeInput(parameters.Get("as_nonewords"))); value != "" {
		words := strings.Fields(value)
		for index := range words {
			words[index] = "-" + strings.TrimLeft(words[index], "-")
		}
		parts = append(parts, strings.Join(words, " "))
	}
	if value := strings.TrimSpace(sanitizeInput(parameters.Get("as_site"))); value != "" && !strings.ContainsAny(value, " \t\r\n") {
		parts = append(parts, "site:"+strings.TrimPrefix(value, "site:"))
	}
	if value := strings.ToLower(strings.TrimSpace(parameters.Get("as_filetype"))); advancedSearchFileTypePattern.MatchString(value) {
		parts = append(parts, "filetype:"+value)
	}
	return strings.Join(parts, " ")
}

func appendDirectoryFilterParams(query string, parameters url.Values) string {
	if !strings.HasPrefix(strings.ToLower(query), "directory:") {
		return query
	}
	for _, filter := range []struct{ parameter, operator string }{
		{"directory_type", "type"},
		{"directory_min_size", "min-size"},
		{"directory_after", "after"},
		{"directory_before", "before"},
		{"directory_site", "site"},
		{"directory_scan", "scan"},
	} {
		value := sanitizeInput(strings.TrimSpace(parameters.Get(filter.parameter)))
		if value != "" && !strings.ContainsAny(value, " \t\r\n") {
			query += " " + filter.operator + ":" + value
		}
	}
	return query
}

// handleDirect handles direct answer requests
// URL format: /direct/{type}/{term}
func chmodModeFromQuery(query url.Values) string {
	var mode strings.Builder
	for _, principal := range []string{"owner", "group", "other"} {
		digit := 0
		for _, permission := range []struct {
			name string
			bit  int
		}{{"read", 4}, {"write", 2}, {"execute", 1}} {
			if query.Get(principal+"_"+permission.name) == "1" {
				digit += permission.bit
			}
		}
		mode.WriteByte(byte('0' + digit))
	}
	return mode.String()
}

func (s *Server) handleDirect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		s.handleError(w, r, http.StatusMethodNotAllowed, i18n.RequestString(r, "errors.method_not_allowed_title"), i18n.RequestString(r, "errors.method_not_allowed_message"))
		return
	}

	// Parse path: /direct/{type}/{term}
	requestPath := strings.SplitN(r.RequestURI, "?", 2)[0]
	if requestPath == "" {
		requestPath = r.URL.EscapedPath()
	}
	path := strings.TrimPrefix(requestPath, "/direct/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) < 1 || parts[0] == "" {
		s.handleError(w, r, http.StatusBadRequest, i18n.RequestString(r, "errors.invalid_request_title"), i18n.RequestString(r, "errors.invalid_request_direct"))
		return
	}

	answerType := direct.AnswerType(parts[0])
	if r.Method == http.MethodPost && answerType != direct.AnswerTypeQR {
		s.handleError(w, r, http.StatusMethodNotAllowed, i18n.RequestString(r, "errors.method_not_allowed_title"), i18n.RequestString(r, "errors.method_not_allowed_message"))
		return
	}
	var term string
	var err error
	if len(parts) < 2 || parts[1] == "" {
		if answerType == direct.AnswerTypeChmod && r.URL.Query().Get("grid") == "1" {
			term = chmodModeFromQuery(r.URL.Query())
		} else if answerType != direct.AnswerTypeQR {
			s.handleError(w, r, http.StatusBadRequest, i18n.RequestString(r, "errors.invalid_request_title"), i18n.RequestString(r, "errors.invalid_request_direct"))
			return
		} else {
			term = r.URL.Query().Get("url")
		}
	} else {
		term, err = url.PathUnescape(parts[1])
		if err != nil {
			term = parts[1]
		}
	}
	term = strings.TrimSpace(term)

	if term == "" {
		s.handleError(w, r, http.StatusBadRequest, i18n.RequestString(r, "errors.invalid_request_title"), i18n.RequestString(r, "errors.invalid_request_term"))
		return
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	// Inject the resolved request language so direct handlers can return
	// translated Title/Content text.
	ctx = direct.WithLang(ctx, s.getI18nManager().DetectLanguage(r))

	// Inject the resolved unit system ("metric" or "imperial") so direct
	// handlers can render unit-bearing fields (area, distance, etc.) using
	// the visitor's preference instead of a hardcoded unit.
	ctx = direct.WithUnits(ctx, s.resolveUnits(r))
	ctx = direct.WithGeoIPLookup(ctx, s.geoipLookup)
	if answerType == direct.AnswerTypeQR {
		var logo []byte
		if r.Method == http.MethodPost {
			r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
			if err := r.ParseMultipartForm(2 << 20); err != nil {
				s.handleError(w, r, http.StatusBadRequest, i18n.RequestString(r, "errors.invalid_request_title"), "Invalid QR customization form")
				return
			}
			if r.MultipartForm != nil {
				defer func() {
					if err := r.MultipartForm.RemoveAll(); err != nil {
						slog.Warn("failed to remove multipart form temp files", "error", err)
					}
				}()
			}
			file, _, fileErr := r.FormFile("logo")
			if fileErr == nil {
				logo, err = io.ReadAll(io.LimitReader(file, (1<<20)+1))
				_ = file.Close()
				if err != nil || len(logo) > 1<<20 || qr.ValidateLogo(logo) != nil {
					s.handleError(w, r, http.StatusBadRequest, i18n.RequestString(r, "errors.invalid_request_title"), "QR logo must be a PNG or JPEG image no larger than 512 by 512 pixels and 1 MiB")
					return
				}
			} else if !errors.Is(fileErr, http.ErrMissingFile) {
				s.handleError(w, r, http.StatusBadRequest, i18n.RequestString(r, "errors.invalid_request_title"), "Invalid QR logo upload")
				return
			}
		}
		query := r.URL.Query()
		getOption := func(key string) string {
			if r.Method == http.MethodPost {
				if r.MultipartForm != nil {
					if values := r.MultipartForm.Value[key]; len(values) > 0 {
						return values[0]
					}
				}
				return r.PostFormValue(key)
			}
			return query.Get(key)
		}
		options := direct.QROptions{Size: 256, Level: "M", Foreground: "#000000", Background: "#FFFFFF", Logo: logo, CSRFToken: s.getCSRFToken(w, r)}
		if r.Method == http.MethodGet {
			options.FormAction = "/direct/qr?url=" + url.QueryEscape(term)
		}
		switch getOption("size") {
		case "S":
			options.Size = 128
		case "L":
			options.Size = 512
		case "XL":
			options.Size = 1024
		case "M", "":
		}
		if level := strings.ToUpper(getOption("level")); level == "L" || level == "M" || level == "Q" || level == "H" {
			options.Level = level
		}
		if foreground := getOption("foreground"); qr.ValidColor(foreground) {
			options.Foreground = foreground
		}
		if background := getOption("background"); qr.ValidColor(background) {
			options.Background = background
		}
		if config.IsTruthy(getOption("shorten_url")) {
			code, shortErr := s.createQRShortlink(ctx, term)
			if shortErr != nil {
				s.handleError(w, r, http.StatusBadRequest, i18n.RequestString(r, "errors.invalid_request_title"), shortErr.Error())
				return
			}
			term = s.getBaseURL(r) + "/s/" + code
			options.ShortURL = term
		}
		ctx = direct.WithQROptions(ctx, options)
	}
	if answerType == direct.AnswerTypeCache {
		ctx = direct.WithArchiveSnapshot(ctx, r.URL.Query().Get("snapshot"))
		ctx = direct.WithArchiveSnapshotIndex(ctx, r.URL.Query().Get("snapshot_index"))
		ctx = direct.WithArchiveSnapshotID(ctx, r.URL.Query().Get("snapshot_id"))
	}

	// Process the direct answer
	answer, err := s.directManager.ProcessType(ctx, answerType, term)
	if err != nil {
		if answerType == direct.AnswerTypeQR {
			s.handleError(w, r, http.StatusBadRequest, i18n.RequestString(r, "errors.invalid_request_title"), err.Error())
			return
		}
		s.handleInternalError(w, r, "direct answer processing", err)
		return
	}

	// Render the direct answer page
	if answer != nil && answer.Type == direct.AnswerTypeWiki {
		answer.Content = s.proxyWikipediaImageSources(answer.Content)
	}
	s.renderDirectAnswer(w, r, answer)
}

func (s *Server) handleCacheDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	query := r.URL.Query()
	original := query.Get("url")
	parsed, err := url.Parse(original)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil {
		http.Error(w, "Invalid archive download request", http.StatusBadRequest)
		return
	}
	filename := query.Get("filename")
	if !strings.HasPrefix(filename, "crawl-data/") || strings.Contains(filename, "..") || strings.ContainsAny(filename, "\\?#") {
		http.Error(w, "Invalid archive download request", http.StatusBadRequest)
		return
	}
	offset, offsetErr := strconv.ParseInt(query.Get("offset"), 10, 64)
	length, lengthErr := strconv.ParseInt(query.Get("length"), 10, 64)
	if offsetErr != nil || lengthErr != nil || offset < 0 || length <= 0 || length > 20<<20 || offset > int64(^uint64(0)>>1)-length+1 {
		http.Error(w, "Invalid archive download request", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	body, contentType, err := direct.NewCacheHandler().DownloadCommonCrawlRecord(ctx, parsed.String(), filename, offset, length)
	if err != nil {
		slog.Warn("Common Crawl archive download failed", "err", err)
		http.Error(w, "Archived capture is unavailable", http.StatusBadGateway)
		return
	}
	if strings.HasPrefix(strings.ToLower(contentType), "text/html") {
		w.Header().Set("Content-Disposition", `attachment; filename="archive-capture.html"`)
	} else {
		w.Header().Set("Content-Disposition", `attachment; filename="archive-capture.txt"`)
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "sandbox; default-src 'none'")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

func (s *Server) handleArchiveTodayDownload(w http.ResponseWriter, r *http.Request) {
	handleArchiveTodayDownload(w, r, direct.NewCacheHandler().DownloadArchiveTodaySnapshot)
}

func handleArchiveTodayDownload(w http.ResponseWriter, r *http.Request, download func(context.Context, string, string, string) ([]byte, string, error)) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	query := r.URL.Query()
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	body, contentType, err := download(ctx, query.Get("url"), query.Get("timestamp"), query.Get("snapshot_url"))
	if err != nil {
		if errors.Is(err, direct.ErrInvalidArchiveTodaySnapshot) {
			http.Error(w, "Invalid archive download request", http.StatusBadRequest)
			return
		}
		slog.Warn("archive.today download failed", "err", err)
		http.Error(w, "Archived capture is unavailable", http.StatusBadGateway)
		return
	}
	filename := "archive-capture.txt"
	if strings.HasPrefix(strings.ToLower(contentType), "text/html") {
		filename = "archive-capture.html"
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename=%q`, filename))
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "sandbox; default-src 'none'")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
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
	writeResponseText(w, `<!DOCTYPE html>
<html lang="`)
	writeResponseText(w, html.EscapeString(baseData.Lang))
	writeResponseText(w, `" dir="`)
	writeResponseText(w, html.EscapeString(baseData.Dir))
	writeResponseText(w, `">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>`)
	writeResponseText(w, html.EscapeString(answer.Title))
	writeResponseText(w, ` - `)
	writeResponseText(w, html.EscapeString(appName))
	writeResponseText(w, `</title>
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
	writeResponseText(w, html.EscapeString(string(answer.Type)))
	writeResponseText(w, `</span>`)
	writeResponseText(w, html.EscapeString(answer.Title))
	writeResponseText(w, `</h1>
<div class="meta">`)
	if answer.Description != "" {
		writeResponseText(w, html.EscapeString(answer.Description))
	}
	writeResponseText(w, `</div>
</header>
<main>
<div class="content`)
	if answer.Error != "" {
		writeResponseText(w, ` error`)
	}
	writeResponseText(w, `">`)
	// Content is already HTML formatted by handlers
	writeResponseText(w, answer.Content)
	writeResponseText(w, `</div>`)
	if answer.Source != "" || answer.SourceURL != "" {
		writeResponseText(w, `<div class="source">`)
		writeResponseText(w, html.EscapeString(t("direct.source_label")))
		writeResponseText(w, ` `)
		if answer.SourceURL != "" {
			writeResponseText(w, `<a href="`)
			writeResponseText(w, html.EscapeString(answer.SourceURL))
			writeResponseText(w, `" rel="noopener noreferrer">`)
			if answer.Source != "" {
				writeResponseText(w, html.EscapeString(answer.Source))
			} else {
				writeResponseText(w, html.EscapeString(answer.SourceURL))
			}
			writeResponseText(w, `</a>`)
		} else {
			writeResponseText(w, html.EscapeString(answer.Source))
		}
		writeResponseText(w, `</div>`)
	}
	writeResponseText(w, `
</main>
<footer>
<a href="`)
	writeResponseText(w, baseURL)
	writeResponseText(w, `/">`)
	writeResponseText(w, html.EscapeString(appName))
	writeResponseText(w, `</a> &middot; `)
	writeResponseText(w, html.EscapeString(t("direct.footer_label")))
	writeResponseText(w, `
</footer>
</div>
</body>
</html>`)
}

// renderSearchError renders an error page
// Per AI.md PART 9: Never expose internal error details to users
func (s *Server) renderSearchError(w http.ResponseWriter, r *http.Request, query string, err error) {
	// Log actual error for debugging (no query logged — privacy is the product)
	slog.Error("Search error", "err", err)

	// Per AI.md canonical status table: validation errors are 400, no
	// configured/available engines is 503 (maintenance/overloaded), and
	// any other search failure is 502 (upstream search-engine failure)
	// rather than a generic 500 - mirrors src/api/api.go's discrimination.
	statusCode := http.StatusBadGateway
	switch {
	case errors.Is(err, model.ErrEmptyQuery), errors.Is(err, model.ErrInvalidCategory):
		statusCode = http.StatusBadRequest
	case errors.Is(err, model.ErrNoEngines):
		statusCode = http.StatusServiceUnavailable
	}

	// Use newPageData for TorAddress support per AI.md PART 32
	baseData := s.newPageData(w, r, i18n.RequestString(r, "search.error_title"), "error")
	baseData.Description = i18n.RequestString(r, "search.error_description")
	baseData.Query = query

	data := &ErrorPageData{
		PageData:   *baseData,
		StatusCode: statusCode,
		StatusText: i18n.RequestString(r, "search.error_title"),
		Message:    i18n.RequestString(r, "search.error_message"),
	}

	w.WriteHeader(statusCode)
	if renderErr := s.renderer.Render(w, "error", data); renderErr != nil {
		// Fallback to plain error - still no internal details
		localizedHTTPError(w, r, statusCode, "errors.server_error")
	}
}

// buildSearchPageData constructs a SearchPageData struct without writing any response.
// Used by renderSearchResultsWithInstant, renderNoJSSearch, and renderHTMLToText.
func (s *Server) buildSearchPageData(w http.ResponseWriter, r *http.Request, query string, results *model.SearchResults, category string, instantAnswer *instant.Answer) *SearchPageData {
	safeSearch, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("safe_search")))
	s.prepareInstantWikipediaAnswer(instantAnswer)
	filters := make(url.Values)
	for name, value := range map[string]string{
		"lang":       firstNonEmptyQueryValue(r.URL.Query(), "lang", "as_lang"),
		"region":     firstNonEmptyQueryValue(r.URL.Query(), "region", "as_region"),
		"time_range": firstNonEmptyQueryValue(r.URL.Query(), "time_range", "as_time"),
		"time_from":  strings.TrimSpace(r.URL.Query().Get("time_from")),
		"time_to":    strings.TrimSpace(r.URL.Query().Get("time_to")),
	} {
		if value != "" {
			filters.Set(name, value)
		}
	}
	for _, name := range []string{"engines", "exclude_engines"} {
		for _, value := range r.URL.Query()[name] {
			if value = strings.TrimSpace(value); value != "" {
				filters.Add(name, value)
			}
		}
	}

	baseData := s.newPageData(w, r, query, "search")
	baseData.Description = fmt.Sprintf("Search results for: %s", query)
	baseData.Query = query
	baseData.Category = category

	pageResults := append([]model.Result(nil), results.GetPage(results.Page)...)
	if s.config.Server.ImageProxy.Enabled {
		for i := range pageResults {
			if pageResults[i].Thumbnail != "" {
				pageResults[i].Thumbnail = s.proxyImageURL(pageResults[i].Thumbnail)
			}
			if pageResults[i].ImageURL == "" && pageResults[i].Metadata != nil {
				pageResults[i].ImageURL, _ = pageResults[i].Metadata["full_image"].(string)
			}
			if pageResults[i].ImageURL != "" {
				pageResults[i].ImageURL = s.proxyImageURL(pageResults[i].ImageURL)
			}
		}
	}
	data := &SearchPageData{
		PageData:      *baseData,
		Query:         query,
		Category:      category,
		Results:       pageResults,
		TotalResults:  results.TotalResults,
		SearchTime:    results.SearchTime,
		Engines:       results.Engines,
		PerPage:       results.PerPage,
		SafeSearch:    safeSearch,
		Language:      firstNonEmptyQueryValue(r.URL.Query(), "lang", "as_lang"),
		Region:        firstNonEmptyQueryValue(r.URL.Query(), "region", "as_region"),
		TimeRange:     firstNonEmptyQueryValue(r.URL.Query(), "time_range", "as_time"),
		TimeFrom:      strings.TrimSpace(r.URL.Query().Get("time_from")),
		TimeTo:        strings.TrimSpace(r.URL.Query().Get("time_to")),
		FilterQuery:   filters.Encode(),
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

	return data
}

// renderSearchResultsWithInstant renders search results with optional instant answer
func (s *Server) renderSearchResultsWithInstant(w http.ResponseWriter, r *http.Request, query string, results *model.SearchResults, category string, instantAnswer *instant.Answer) {
	data := s.buildSearchPageData(w, r, query, results, category, instantAnswer)

	if err := s.renderer.Render(w, "search", data); err != nil {
		// Fallback to inline rendering
		s.renderSearchResultsInline(w, r, query, results, category)
	}
}

// renderSearchResultsInline is a fallback for when templates fail
func (s *Server) renderSearchResultsInline(w http.ResponseWriter, r *http.Request, query string, results *model.SearchResults, category string) {
	baseData := s.newPageData(w, r, query, "search")
	writeResponseFormatted(w, `<!DOCTYPE html>
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
		writeResponseFormatted(w, `
        <article class="result">
            <h3><a href="%s" target="_blank">%s</a></h3>
            <p>%s</p>
        </article>`,
			html.EscapeString(result.URL),
			html.EscapeString(result.Title),
			html.EscapeString(result.Content),
		)
	}

	writeResponseFormatted(w, `
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

// handleServerStatus returns runtime detail as JSON, gated by operator token.
// Per API.md PART 13/14: /server/status returns uptime, goroutines, memory, version, mode.
func (s *Server) handleServerStatus(w http.ResponseWriter, r *http.Request) {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	respondJSON(w, http.StatusOK, map[string]any{
		"ok": true,
		"data": map[string]any{
			"version":    getVersion(),
			"mode":       s.config.Server.Mode,
			"uptime":     formatDuration(time.Since(s.startTime)),
			"goroutines": runtime.NumGoroutine(),
			"memory": map[string]any{
				"alloc_bytes":       memStats.Alloc,
				"total_alloc_bytes": memStats.TotalAlloc,
				"sys_bytes":         memStats.Sys,
				"num_gc":            memStats.NumGC,
			},
		},
	})
}

// handleServerConfig returns the sanitized running configuration as JSON, gated by operator token.
// Per API.md PART 13/14: /server/config exposes redacted config to operators.
func (s *Server) handleServerConfig(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, map[string]any{
		"ok":   true,
		"data": s.config.Sanitized(),
	})
}
