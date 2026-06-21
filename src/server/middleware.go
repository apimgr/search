package server

import (
	"compress/gzip"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"html"
	"io"
	"log"
	"log/slog"
	"net"
	"net/http"
	"path"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/apimgr/search/src/api"
	"github.com/apimgr/search/src/config"
	"github.com/apimgr/search/src/geoip"
	"github.com/apimgr/search/src/i18n"
	"github.com/apimgr/search/src/logging"
	"github.com/google/uuid"
)

// contextKey is a package-local type for context keys to avoid collisions.
type contextKey string

// allowlistedCtxKey is the context key for the allowlisted flag.
// Set by AllowlistMiddleware; checked by BlocklistMiddleware, RateLimit, and GeoBlock.
type allowlistedCtxKey struct{}

// isAllowlisted returns true if the request IP is in the configured allowlist.
// Per AI.md PART 5: the flag bypasses blocklist, rate-limit, and GeoIP — NOT auth.
func isAllowlisted(ctx context.Context) bool {
	v, _ := ctx.Value(allowlistedCtxKey{}).(bool)
	return v
}

// Middleware wraps an http.Handler to add common functionality
type Middleware struct {
	config     *config.Config
	logManager *logging.Manager
}

// NewMiddleware creates a new middleware instance
func NewMiddleware(cfg *config.Config, logMgr *logging.Manager) *Middleware {
	return &Middleware{config: cfg, logManager: logMgr}
}

// Chain chains multiple middleware handlers together
func Chain(h http.Handler, middlewares ...func(http.Handler) http.Handler) http.Handler {
	for i := len(middlewares) - 1; i >= 0; i-- {
		h = middlewares[i](h)
	}
	return h
}

// SecurityHeaders adds security headers to all responses per AI.md PART 11.
func (m *Middleware) SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headers := m.config.Server.Security.Headers

		// X-Frame-Options prevents clickjacking
		if headers.XFrameOptions != "" {
			w.Header().Set("X-Frame-Options", headers.XFrameOptions)
		}

		// X-Content-Type-Options prevents MIME sniffing
		if headers.XContentTypeOptions != "" {
			w.Header().Set("X-Content-Type-Options", headers.XContentTypeOptions)
		}

		// X-XSS-Protection enables browser XSS filter (legacy compat)
		if headers.XXSSProtection != "" {
			w.Header().Set("X-XSS-Protection", headers.XXSSProtection)
		}

		// Referrer-Policy controls referrer information
		if headers.ReferrerPolicy != "" {
			w.Header().Set("Referrer-Policy", headers.ReferrerPolicy)
		}

		// Content-Security-Policy prevents XSS and injection attacks.
		// Per AI.md PART 11: development mode uses Report-Only so violations are
		// logged without breaking the app; production enforces the policy.
		if headers.ContentSecurityPolicy != "" {
			cspHeader := "Content-Security-Policy"
			if m.config.Server.Mode == "development" {
				cspHeader = "Content-Security-Policy-Report-Only"
			}
			w.Header().Set(cspHeader, headers.ContentSecurityPolicy)
		}

		// Permissions-Policy controls browser features
		if headers.PermissionsPolicy != "" {
			w.Header().Set("Permissions-Policy", headers.PermissionsPolicy)
		}

		// X-Permitted-Cross-Domain-Policies blocks Flash/PDF cross-domain embedding
		if headers.CrossDomainPolicies != "" {
			w.Header().Set("X-Permitted-Cross-Domain-Policies", headers.CrossDomainPolicies)
		}

		// Origin-Agent-Cluster: ?1 — security/perf isolation (always on per spec)
		if headers.OriginAgentCluster {
			w.Header().Set("Origin-Agent-Cluster", "?1")
		}

		// Cross-Origin-Opener-Policy (default: unsafe-none)
		if headers.COOP != "" {
			w.Header().Set("Cross-Origin-Opener-Policy", headers.COOP)
		}

		// Cross-Origin-Embedder-Policy (default: unsafe-none)
		if headers.COEP != "" {
			w.Header().Set("Cross-Origin-Embedder-Policy", headers.COEP)
		}

		// Cross-Origin-Resource-Policy (default: cross-origin)
		if headers.CORP != "" {
			w.Header().Set("Cross-Origin-Resource-Policy", headers.CORP)
		}

		// HSTS — only when SSL is enabled per AI.md PART 11 (max-age 2 years + preload)
		hsts := m.config.Server.Security.HSTS
		if m.config.Server.SSL.Enabled && hsts.Enabled {
			maxAge := hsts.MaxAgeSeconds
			if maxAge <= 0 {
				maxAge = 63072000
			}
			hstsVal := fmt.Sprintf("max-age=%d", maxAge)
			if hsts.IncludeSubDomains {
				hstsVal += "; includeSubDomains"
			}
			if hsts.Preload {
				hstsVal += "; preload"
			}
			w.Header().Set("Strict-Transport-Security", hstsVal)
		}

		// NEL + Reporting-Endpoints + Report-To per AI.md PART 11
		nel := m.config.Server.Security.NEL
		if nel.Enabled {
			scheme := "https"
			if !m.config.Server.SSL.Enabled {
				scheme = "http"
			}
			host := r.Host
			if host == "" {
				host = m.config.Server.BaseURL
			}
			reportsURL := fmt.Sprintf("%s://%s/api/v1/server/reports/default", scheme, host)
			w.Header().Set("Reporting-Endpoints", fmt.Sprintf(`default="%s"`, reportsURL))
			w.Header().Set("Report-To", fmt.Sprintf(
				`{"group":"default","max_age":%d,"endpoints":[{"url":"%s"}]}`,
				nel.MaxAgeSeconds, reportsURL,
			))
			nelSubdomains := "false"
			if nel.IncludeSubDomains {
				nelSubdomains = "true"
			}
			w.Header().Set("NEL", fmt.Sprintf(
				`{"report_to":"default","max_age":%d,"include_subdomains":%s,"failure_fraction":%.1f}`,
				nel.MaxAgeSeconds, nelSubdomains, nel.SampleRate,
			))
		}

		next.ServeHTTP(w, r)
	})
}

// CORS handles Cross-Origin Resource Sharing per AI.md PART 16.
// Config key: server.web.cors (string). Default: "*" (all origins, no credentials).
// - "*"       → Access-Control-Allow-Origin: * (no credentials per CORS spec)
// - "a,b,..."  → match request Origin against list; reflect matched origin + credentials
// - ""         → no CORS headers (same-origin only)
func (m *Middleware) CORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		corsPolicy := m.config.Server.Web.CORS
		if corsPolicy == "" {
			next.ServeHTTP(w, r)
			return
		}

		const (
			corsAllowMethods = "GET, POST, PUT, PATCH, DELETE, OPTIONS"
			corsAllowHeaders = "*"
			corsMaxAge       = "86400"
		)

		origin := r.Header.Get("Origin")
		if origin != "" {
			if corsPolicy == "*" {
				w.Header().Set("Access-Control-Allow-Origin", "*")
			} else {
				for _, allowed := range strings.Split(corsPolicy, ",") {
					if strings.TrimSpace(allowed) == origin {
						w.Header().Set("Access-Control-Allow-Origin", origin)
						w.Header().Set("Access-Control-Allow-Credentials", "true")
						break
					}
				}
			}
		}

		if r.Method == http.MethodOptions {
			w.Header().Set("Access-Control-Allow-Methods", corsAllowMethods)
			w.Header().Set("Access-Control-Allow-Headers", corsAllowHeaders)
			w.Header().Set("Access-Control-Max-Age", corsMaxAge)
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// RateLimiter implements token bucket rate limiting
type RateLimiter struct {
	mu       sync.RWMutex
	visitors map[string]*visitor
	rate     int
	burst    int
	enabled  bool
}

type visitor struct {
	tokens    float64
	lastSeen  time.Time
	blocked   bool
	blockTime time.Time
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(cfg *config.RateLimitConfig) *RateLimiter {
	rl := &RateLimiter{
		visitors: make(map[string]*visitor),
		rate:     cfg.RequestsPerMinute,
		burst:    cfg.BurstSize,
		enabled:  cfg.Enabled,
	}

	// Start cleanup goroutine
	go rl.cleanup()

	return rl
}

// cleanup removes old entries periodically
func (rl *RateLimiter) cleanup() {
	for {
		time.Sleep(5 * time.Minute)

		rl.mu.Lock()
		for ip, v := range rl.visitors {
			if time.Since(v.lastSeen) > 10*time.Minute {
				delete(rl.visitors, ip)
			}
		}
		rl.mu.Unlock()
	}
}

// Allow checks if a request is allowed
func (rl *RateLimiter) Allow(ip string) bool {
	if !rl.enabled {
		return true
	}

	rl.mu.Lock()
	defer rl.mu.Unlock()

	v, exists := rl.visitors[ip]
	if !exists {
		rl.visitors[ip] = &visitor{
			tokens:   float64(rl.burst - 1),
			lastSeen: time.Now(),
		}
		return true
	}

	// Check if still blocked
	if v.blocked {
		if time.Since(v.blockTime) < time.Minute {
			return false
		}
		v.blocked = false
	}

	// Refill tokens based on time passed
	elapsed := time.Since(v.lastSeen).Seconds()
	refillRate := float64(rl.rate) / 60.0
	v.tokens += elapsed * refillRate
	if v.tokens > float64(rl.burst) {
		v.tokens = float64(rl.burst)
	}

	v.lastSeen = time.Now()

	if v.tokens >= 1 {
		v.tokens--
		return true
	}

	// Block for 1 minute if exhausted
	v.blocked = true
	v.blockTime = time.Now()
	return false
}

// EndpointRateLimiter implements per-endpoint rate limiting
// Per AI.md PART 11: login (5/15min), password reset (3/1hr), registration (5/1hr)
type EndpointRateLimiter struct {
	mu      sync.Mutex
	entries map[string]*endpointEntry
	limit   int
	window  time.Duration
}

type endpointEntry struct {
	attempts int
	firstAt  time.Time
}

// NewEndpointRateLimiter creates a rate limiter for a specific endpoint
func NewEndpointRateLimiter(limit int, window time.Duration) *EndpointRateLimiter {
	erl := &EndpointRateLimiter{
		entries: make(map[string]*endpointEntry),
		limit:   limit,
		window:  window,
	}
	go erl.cleanup()
	return erl
}

// cleanup removes expired entries periodically
func (erl *EndpointRateLimiter) cleanup() {
	for {
		time.Sleep(5 * time.Minute)
		erl.mu.Lock()
		for key, entry := range erl.entries {
			if time.Since(entry.firstAt) > erl.window {
				delete(erl.entries, key)
			}
		}
		erl.mu.Unlock()
	}
}

// Allow checks if a request from the given IP is allowed
func (erl *EndpointRateLimiter) Allow(ip string) bool {
	erl.mu.Lock()
	defer erl.mu.Unlock()

	entry, exists := erl.entries[ip]
	if !exists {
		erl.entries[ip] = &endpointEntry{
			attempts: 1,
			firstAt:  time.Now(),
		}
		return true
	}

	// Window expired, reset
	if time.Since(entry.firstAt) > erl.window {
		erl.entries[ip] = &endpointEntry{
			attempts: 1,
			firstAt:  time.Now(),
		}
		return true
	}

	// Within window, check limit
	if entry.attempts >= erl.limit {
		return false
	}

	entry.attempts++
	return true
}

// RemainingTime returns how long until the rate limit window resets for an IP
func (erl *EndpointRateLimiter) RemainingTime(ip string) time.Duration {
	erl.mu.Lock()
	defer erl.mu.Unlock()

	entry, exists := erl.entries[ip]
	if !exists {
		return 0
	}

	remaining := erl.window - time.Since(entry.firstAt)
	if remaining < 0 {
		return 0
	}
	return remaining
}

// Allowlist is middleware step 5 per AI.md PART 5.
// Sets the allowlisted flag in the request context when the client IP is in the
// configured whitelist. Allowlisted requests bypass blocklist, rate-limit, and GeoIP
// checks downstream — but NOT auth.
func (m *Middleware) Allowlist(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := getClientIP(r, m.config.Server.Security.TrustedProxies)
		for _, allowed := range m.config.Server.RateLimit.Whitelist {
			if ip == allowed || strings.HasPrefix(ip, allowed) {
				ctx := context.WithValue(r.Context(), allowlistedCtxKey{}, true)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// Blocklist is middleware step 6 per AI.md PART 5.
// Blocks requests from IPs on the configured blacklist.
// Allowlisted IPs (flag set by Allowlist middleware) skip this check.
func (m *Middleware) Blocklist(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isAllowlisted(r.Context()) {
			next.ServeHTTP(w, r)
			return
		}
		ip := getClientIP(r, m.config.Server.Security.TrustedProxies)
		for _, blacklisted := range m.config.Server.RateLimit.Blacklist {
			if ip == blacklisted || strings.HasPrefix(ip, blacklisted) {
				// Per AI.md PART 11: no IP logging — privacy is the product.
				if m.logManager != nil {
					m.logManager.Security().LogBlocked("-", r.URL.Path, "blacklisted")
				}
				localizedHTTPError(w, r, http.StatusForbidden, "errors.forbidden")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// RateLimit is middleware step 7 per AI.md PART 5.
// Applies per-IP rate limiting. Allowlisted IPs (flag set by Allowlist middleware) skip this check.
func (m *Middleware) RateLimit(limiter *RateLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if isAllowlisted(r.Context()) {
				next.ServeHTTP(w, r)
				return
			}
			ip := getClientIP(r, m.config.Server.Security.TrustedProxies)
			if !limiter.Allow(ip) {
				// Per AI.md PART 11: no IP logging — privacy is the product.
				if m.logManager != nil {
					m.logManager.Security().LogRateLimited("-", r.URL.Path)
				}
				w.Header().Set("Retry-After", "60")
				localizedHTTPError(w, r, http.StatusTooManyRequests, "errors.rate_limit")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// getClientIP extracts the real client IP from request
func getClientIP(r *http.Request, trustedProxies []string) string {
	// Check X-Forwarded-For header if from trusted proxy
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		remoteIP, _, _ := net.SplitHostPort(r.RemoteAddr)
		for _, trusted := range trustedProxies {
			if remoteIP == trusted || strings.HasPrefix(remoteIP, trusted) {
				// Get first IP in chain
				ips := strings.Split(xff, ",")
				if len(ips) > 0 {
					return strings.TrimSpace(ips[0])
				}
			}
		}
	}

	// Check X-Real-IP header
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		remoteIP, _, _ := net.SplitHostPort(r.RemoteAddr)
		for _, trusted := range trustedProxies {
			if remoteIP == trusted || strings.HasPrefix(remoteIP, trusted) {
				return xri
			}
		}
	}

	// Use RemoteAddr
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}

// CSRF middleware handles Cross-Site Request Forgery protection
type CSRFMiddleware struct {
	config     *config.Config
	tokens     sync.Map
	logManager *logging.Manager
}

// NewCSRFMiddleware creates a new CSRF middleware
func NewCSRFMiddleware(cfg *config.Config) *CSRFMiddleware {
	return &CSRFMiddleware{config: cfg}
}

// SetLogManager sets the logging manager for security events
func (c *CSRFMiddleware) SetLogManager(logMgr *logging.Manager) {
	c.logManager = logMgr
}

// GenerateToken generates a new CSRF token
func (c *CSRFMiddleware) GenerateToken() string {
	b := make([]byte, 32)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// ValidateToken validates a CSRF token from the request
func (c *CSRFMiddleware) ValidateToken(r *http.Request) bool {
	csrf := c.config.Server.Security.CSRF

	// Skip validation if CSRF is disabled
	if !csrf.Enabled {
		return true
	}

	// Get token from cookie
	cookie, err := r.Cookie(csrf.CookieName)
	if err != nil {
		return false
	}

	// Get token from header or form
	token := r.Header.Get(csrf.HeaderName)
	if token == "" {
		token = r.FormValue(csrf.FieldName)
	}

	// Validate token matches cookie (constant-time per AI.md PART 11)
	if subtle.ConstantTimeCompare([]byte(token), []byte(cookie.Value)) != 1 {
		return false
	}

	// Validate token exists in store
	if _, ok := c.tokens.Load(token); !ok {
		return false
	}

	// Delete used token (single use)
	c.tokens.Delete(token)
	return true
}

// Protect applies CSRF protection to handlers
func (c *CSRFMiddleware) Protect(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		csrf := c.config.Server.Security.CSRF

		if !csrf.Enabled {
			next.ServeHTTP(w, r)
			return
		}

		// Skip for safe methods
		if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
			// Generate token for GET requests
			token := c.GenerateToken()
			c.tokens.Store(token, time.Now())

			// Set cookie — SameSite=Strict per AI.md PART 11 (blocks cross-site attachment,
			// neutralizing CSRF before the double-submit check even runs).
			http.SetCookie(w, &http.Cookie{
				Name:     csrf.CookieName,
				Value:    token,
				Path:     "/",
				HttpOnly: true,
				Secure:   c.config.Server.SSL.Enabled,
				SameSite: http.SameSiteStrictMode,
			})

			next.ServeHTTP(w, r)
			return
		}

		// Validate token for unsafe methods
		cookie, err := r.Cookie(csrf.CookieName)
		if err != nil {
			// Per AI.md PART 11: no IP logging — privacy is the product.
			if c.logManager != nil {
				c.logManager.Security().LogCSRFViolation("-", r.URL.Path)
			}
			localizedHTTPError(w, r, http.StatusForbidden, "errors.csrf_missing")
			return
		}

		// Get token from header or form
		token := r.Header.Get(csrf.HeaderName)
		if token == "" {
			token = r.FormValue(csrf.FieldName)
		}

		if subtle.ConstantTimeCompare([]byte(token), []byte(cookie.Value)) != 1 {
			// Per AI.md PART 11: no IP logging — privacy is the product.
			if c.logManager != nil {
				c.logManager.Security().LogCSRFViolation("-", r.URL.Path)
			}
			localizedHTTPError(w, r, http.StatusForbidden, "errors.csrf_invalid")
			return
		}

		// Validate token exists in store
		if _, ok := c.tokens.Load(token); !ok {
			// Per AI.md PART 11: no IP logging — privacy is the product.
			if c.logManager != nil {
				c.logManager.Security().LogCSRFViolation("-", r.URL.Path)
			}
			localizedHTTPError(w, r, http.StatusForbidden, "errors.csrf_expired")
			return
		}

		// Delete used token
		c.tokens.Delete(token)

		next.ServeHTTP(w, r)
	})
}

// Logger middleware logs all requests
func (m *Middleware) Logger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Wrap response writer to capture status code
		wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		next.ServeHTTP(wrapped, r)

		duration := time.Since(start)

		// Log to access log (Apache/Nginx combined format)
		if m.logManager != nil {
			m.logManager.Access().LogRequest(r, wrapped.statusCode, int64(wrapped.bytesWritten), duration)
		}

		// Also log to stdout in development mode
		// Privacy: never log client IP (privacy is the product per CLAUDE.md rule #10).
		if m.config.Server.Mode == "development" {
			log.Printf("- - - [%s] \"%s %s %s\" %d %d \"%.3fms\"",
				time.Now().Format("02/Jan/2006:15:04:05 -0700"),
				r.Method,
				r.URL.Path,
				r.Proto,
				wrapped.statusCode,
				wrapped.bytesWritten,
				float64(duration.Microseconds())/1000.0,
			)
		}
	})
}

// responseWriter wraps http.ResponseWriter to capture status code
type responseWriter struct {
	http.ResponseWriter
	statusCode   int
	bytesWritten int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	n, err := rw.ResponseWriter.Write(b)
	rw.bytesWritten += n
	return n, err
}

// Recovery middleware recovers from panics
// Per AI.md PART 9: All panics must be safely recovered and logged with context
func (m *Middleware) Recovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				// Safely convert panic value to string (handles all types)
				var errMsg string
				switch v := err.(type) {
				case string:
					errMsg = v
				case error:
					errMsg = v.Error()
				default:
					errMsg = fmt.Sprintf("%v", v)
				}

				// Get RequestID if available
				requestID := r.Header.Get("X-Request-ID")

				// Per AI.md PART 7-9: Structured logging for recovery middleware
				// Privacy: never log remote_addr (IPs) or user-agent in production.
				// Privacy is the product per CLAUDE.md rule #10 — no server-side
				// logs of IPs or user identifiers. Stack/UA only when --debug is on.
				attrs := []slog.Attr{
					slog.String("error", errMsg),
					slog.String("method", r.Method),
					slog.String("path", r.URL.Path),
				}
				if requestID != "" {
					attrs = append(attrs, slog.String("request_id", requestID))
				}

				// Include stack trace and user-agent in development/debug mode only
				if m.config.Server.Mode == "development" || m.config.IsDebug() {
					attrs = append(attrs, slog.String("user_agent", r.UserAgent()))
					attrs = append(attrs, slog.String("stack", string(debug.Stack())))
				}

				slog.LogAttrs(r.Context(), slog.LevelError, "PANIC recovered", attrs...)

				// Also log to standard log for backward compatibility
				if requestID != "" {
					log.Printf("PANIC (request_id=%s): %s", requestID, errMsg)
				} else {
					log.Printf("PANIC: %s", errMsg)
				}

				// In development mode, show detailed error
				if m.config.Server.Mode == "development" {
					http.Error(w, i18n.RequestString(r, "errors.server_error")+": "+errMsg, http.StatusInternalServerError)
					return
				}

				localizedHTTPError(w, r, http.StatusInternalServerError, "errors.server_error")
			}
		}()

		next.ServeHTTP(w, r)
	})
}

// RequestID middleware adds a unique request ID (UUID v4 per AI.md)
func (m *Middleware) RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var requestID string

		// Check for incoming X-Request-ID header first (per AI.md)
		if incoming := r.Header.Get("X-Request-ID"); incoming != "" {
			// Validate that it's a valid UUID v4
			if _, err := uuid.Parse(incoming); err == nil {
				requestID = incoming
			}
		}

		// Generate new UUID v4 if not provided or invalid
		if requestID == "" {
			requestID = uuid.New().String()
		}

		// Add to request context for logging
		r.Header.Set("X-Request-ID", requestID)

		// Add to response headers
		w.Header().Set("X-Request-ID", requestID)

		next.ServeHTTP(w, r)
	})
}

// gzipResponseWriter wraps http.ResponseWriter to provide gzip compression
type gzipResponseWriter struct {
	io.Writer
	http.ResponseWriter
	wroteHeader bool
}

func (w *gzipResponseWriter) Write(b []byte) (int, error) {
	if !w.wroteHeader {
		// Set Content-Encoding header before writing
		w.ResponseWriter.Header().Set("Content-Encoding", "gzip")
		// Length changes with compression
		w.ResponseWriter.Header().Del("Content-Length")
		w.wroteHeader = true
	}
	return w.Writer.Write(b)
}

func (w *gzipResponseWriter) WriteHeader(code int) {
	w.ResponseWriter.Header().Del("Content-Length")
	w.wroteHeader = true
	w.ResponseWriter.WriteHeader(code)
}

// Compress middleware adds gzip compression for text-based responses
func (m *Middleware) Compress(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check if client accepts gzip
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			next.ServeHTTP(w, r)
			return
		}

		// Skip compression for small responses or specific paths
		path := r.URL.Path
		if strings.HasPrefix(path, "/static/") && (strings.HasSuffix(path, ".png") ||
			strings.HasSuffix(path, ".jpg") ||
			strings.HasSuffix(path, ".gif") ||
			strings.HasSuffix(path, ".ico") ||
			strings.HasSuffix(path, ".woff") ||
			strings.HasSuffix(path, ".woff2")) {
			// Don't compress already-compressed formats
			next.ServeHTTP(w, r)
			return
		}

		// Create gzip writer
		gz, err := gzip.NewWriterLevel(w, gzip.DefaultCompression)
		if err != nil {
			next.ServeHTTP(w, r)
			return
		}
		defer gz.Close()

		// Wrap response writer
		gzw := &gzipResponseWriter{Writer: gz, ResponseWriter: w}

		// Add Vary header for caching
		w.Header().Set("Vary", "Accept-Encoding")

		next.ServeHTTP(gzw, r)
	})
}

// GeoBlock middleware blocks requests based on GeoIP location
func (m *Middleware) GeoBlock(lookup *geoip.Lookup) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip if GeoIP is not configured
			if lookup == nil || !lookup.IsLoaded() {
				next.ServeHTTP(w, r)
				return
			}
			// Allowlisted IPs bypass GeoIP per AI.md PART 5
			if isAllowlisted(r.Context()) {
				next.ServeHTTP(w, r)
				return
			}

			// Get client IP
			ip := getClientIP(r, m.config.Server.Security.TrustedProxies)

			// Get configured allowed/denied countries
			allowedCountries := m.config.Server.GeoIP.AllowedCountries
			denyCountries := m.config.Server.GeoIP.DenyCountries

			// Check if country is blocked
			if lookup.IsBlocked(ip, denyCountries) {
				result := lookup.Lookup(ip)
				// Per AI.md PART 11: no IP logging — log country code (non-PII) but not the IP.
				if m.logManager != nil {
					m.logManager.Security().LogBlocked("-", r.URL.Path, "country:"+result.CountryCode)
				}
				localizedHTTPError(w, r, http.StatusForbidden, "errors.forbidden")
				return
			}

			// Check if country is allowed (if allowlist is configured)
			if !lookup.IsAllowed(ip, allowedCountries) {
				result := lookup.Lookup(ip)
				// Per AI.md PART 11: no IP logging — log country code (non-PII) but not the IP.
				if m.logManager != nil {
					m.logManager.Security().LogBlocked("-", r.URL.Path, "country:"+result.CountryCode)
				}
				localizedHTTPError(w, r, http.StatusForbidden, "errors.forbidden")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// MaintenanceHandler is a function that checks maintenance mode
type MaintenanceHandler interface {
	IsInMaintenance() bool
	GetMode() int
	GetMessage() string
}

// MaintenanceMode middleware handles maintenance mode per AI.md PART 6
// - Allows admin routes even during maintenance
// - Shows maintenance page to regular users
// - Allows health checks for monitoring
func (m *Middleware) MaintenanceMode(handler MaintenanceHandler) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip if not in maintenance mode
			if !handler.IsInMaintenance() {
				next.ServeHTTP(w, r)
				return
			}

			path := r.URL.Path

			// Always allow health checks
			if path == "/healthz" || path == api.APIPrefix+"/healthz" {
				next.ServeHTTP(w, r)
				return
			}

			// Always allow API maintenance status endpoint
			if path == api.APIPrefix+"/maintenance" || path == api.APIPrefix+"/status" {
				next.ServeHTTP(w, r)
				return
			}

			// Always allow static assets
			if strings.HasPrefix(path, "/static/") {
				next.ServeHTTP(w, r)
				return
			}

			// Per AI.md PART 11: no IP logging — privacy is the product.
			if m.logManager != nil {
				m.logManager.Security().LogBlocked("-", path, "maintenance")
			}

			// Return maintenance page
			// Per AI.md PART 5 line 5488-5492: Maintenance mode headers
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Retry-After", "30")
			w.Header().Set("X-Maintenance-Mode", "true")
			w.Header().Set("X-Maintenance-Reason", "maintenance")
			w.WriteHeader(http.StatusServiceUnavailable)

			message := handler.GetMessage()
			if message == "" {
				message = "The system is currently undergoing maintenance. Please try again later."
			}
			lang, dir := i18n.DetectRequestLocale(r)

			// Simple maintenance page
			maintenanceHTML := fmt.Sprintf(`<!DOCTYPE html>
<html lang="%s" dir="%s">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Maintenance</title>
    <style>
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            background: #1a1a2e;
            color: #eee;
            display: flex;
            justify-content: center;
            align-items: center;
            min-height: 100vh;
            margin: 0;
        }
        .container {
            text-align: center;
            padding: 2rem;
            max-width: 600px;
        }
        h1 { color: #bd93f9; margin-bottom: 1rem; }
        p { color: #999; line-height: 1.6; }
        .icon { font-size: 4rem; margin-bottom: 1rem; }
    </style>
</head>
<body>
    <div class="container">
        <div class="icon">🔧</div>
        <h1>System Maintenance</h1>
        <p>%s</p>
    </div>
</body>
</html>`, lang, dir, html.EscapeString(message))
			w.Write([]byte(maintenanceHTML))
		})
	}
}

// DegradedMode middleware handles degraded mode per AI.md PART 6
// Shows warnings to users when system is in degraded state
func (m *Middleware) DegradedMode(handler MaintenanceHandler) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Check if in degraded mode (mode == 1)
			if handler.GetMode() == 1 {
				// Add header to indicate degraded state
				w.Header().Set("X-System-Status", "degraded")
			}

			next.ServeHTTP(w, r)
		})
	}
}

// URLNormalizeMiddleware normalizes URLs for consistent routing
// Per AI.md PART 16: Removes trailing slashes (except for root "/"), redirects to canonical URL
// This middleware MUST be FIRST in the chain - before PathSecurityMiddleware
func URLNormalizeMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path

		// Root path "/" stays as-is
		if p == "/" {
			next.ServeHTTP(w, r)
			return
		}

		// Remove trailing slash (canonical form: no trailing slash)
		if strings.HasSuffix(p, "/") {
			// Exception: explicit file requests (e.g., /dir/index.html)
			lastSlash := strings.LastIndex(p, "/")
			if lastSlash >= 0 && !strings.Contains(p[lastSlash:], ".") {
				canonical := strings.TrimSuffix(p, "/")
				// Preserve query string
				if r.URL.RawQuery != "" {
					canonical += "?" + r.URL.RawQuery
				}
				http.Redirect(w, r, canonical, http.StatusMovedPermanently)
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}

// TargetType represents the context type extracted from URL path.
// Per AI.md PART 11: this project has no admin UI, no user accounts,
// no organizations. The only valid context types are public routes and
// server pages (/server/*). All other paths are public.
type TargetType int

const (
	// TargetUnknown is an unknown/invalid target type
	TargetUnknown TargetType = iota
	// TargetPublic covers all public routes: /, /search, /api/v1/*, /alerts/*, etc.
	TargetPublic
	// TargetServerPages covers /server/* operator-info pages (about, privacy, contact, etc.)
	TargetServerPages
)

// String returns the string representation of TargetType
func (t TargetType) String() string {
	switch t {
	case TargetPublic:
		return "public"
	case TargetServerPages:
		return "server"
	default:
		return "unknown"
	}
}

// RequestContext holds context extracted from URL path.
// Per AI.md PART 11: context is determined from URL path, not headers.
type RequestContext struct {
	Type TargetType
}

// extractContextFromPath determines context from URL path.
// Per AI.md PART 1: this project has two route families — /server/* pages and
// everything else (public). No admin UI, no user accounts, no org routes.
func extractContextFromPath(urlPath string) *RequestContext {
	p := strings.TrimPrefix(urlPath, "/")

	// API routes: strip /api/{version}/ and re-classify the sub-path
	if strings.HasPrefix(p, "api/") {
		parts := strings.SplitN(p, "/", 4)
		if len(parts) >= 3 {
			// parts[0]="api" parts[1]="v1" parts[2]=sub-resource
			p = strings.Join(parts[2:], "/")
		} else {
			return &RequestContext{Type: TargetPublic}
		}
	}

	// /server/* routes are operator-info pages
	if p == "server" || strings.HasPrefix(p, "server/") {
		return &RequestContext{Type: TargetServerPages}
	}

	return &RequestContext{Type: TargetPublic}
}

// GetRequestContext retrieves the request context from the request.
func GetRequestContext(r *http.Request) *RequestContext {
	if ctx := r.Context().Value("target_context"); ctx != nil {
		if rc, ok := ctx.(*RequestContext); ok {
			return rc
		}
	}
	return &RequestContext{Type: TargetUnknown}
}

// PathSecurityMiddleware normalizes paths and blocks traversal attempts
// Per AI.md PART 5: This middleware MUST be after URLNormalizeMiddleware
func PathSecurityMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		original := r.URL.Path

		// Check both raw path and URL-decoded for traversal
		// Note: r.URL.Path is already decoded by net/http, but check RawPath too
		rawPath := r.URL.RawPath
		if rawPath == "" {
			rawPath = r.URL.Path
		}

		// Block path traversal attempts (encoded and decoded)
		// %2e = . so %2e%2e = ..
		if strings.Contains(original, "..") ||
			strings.Contains(rawPath, "..") ||
			strings.Contains(strings.ToLower(rawPath), "%2e") {
			localizedHTTPError(w, r, http.StatusBadRequest, "errors.bad_request")
			return
		}

		// Normalize the path
		cleaned := path.Clean(original)

		// Ensure leading slash
		if !strings.HasPrefix(cleaned, "/") {
			cleaned = "/" + cleaned
		}

		// Preserve trailing slash for directory paths
		if original != "/" && strings.HasSuffix(original, "/") && !strings.HasSuffix(cleaned, "/") {
			cleaned += "/"
		}

		// Update request
		r.URL.Path = cleaned

		next.ServeHTTP(w, r)
	})
}

// SecGPC handles the Sec-GPC (Global Privacy Control) request header per AI.md PART 11.
// When the browser signals GPC opt-out, the spec requires: set a request context flag,
// audit-log the signal, and skip any non-essential processing. This project has no
// personalization or behavioral analytics, so the flag is primarily a compliance hook.
func (m *Middleware) SecGPC(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.TrimSpace(r.Header.Get("Sec-GPC")) == "1" {
			// Set context flag so downstream handlers can respect the opt-out
			ctx := context.WithValue(r.Context(), contextKey("gpc_opt_out"), true)
			r = r.WithContext(ctx)
			// Log for compliance per spec ("Surface in the audit log")
			slog.LogAttrs(r.Context(), slog.LevelInfo, "Sec-GPC opt-out honored",
				slog.String("path", r.URL.Path),
				slog.String("method", r.Method),
			)
		}
		next.ServeHTTP(w, r)
	})
}
