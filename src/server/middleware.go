package server

import (
	"compress/gzip"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net"
	"net/http"
	"path"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/apimgr/search/src/config"
	"github.com/apimgr/search/src/geoip"
	"github.com/apimgr/search/src/logging"
	"github.com/google/uuid"
)

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

// SecurityHeaders adds security headers to all responses
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

		// X-XSS-Protection enables browser XSS filter
		if headers.XXSSProtection != "" {
			w.Header().Set("X-XSS-Protection", headers.XXSSProtection)
		}

		// Referrer-Policy controls referrer information
		if headers.ReferrerPolicy != "" {
			w.Header().Set("Referrer-Policy", headers.ReferrerPolicy)
		}

		// Content-Security-Policy prevents XSS and injection attacks
		if headers.ContentSecurityPolicy != "" {
			w.Header().Set("Content-Security-Policy", headers.ContentSecurityPolicy)
		}

		// Permissions-Policy controls browser features
		if headers.PermissionsPolicy != "" {
			w.Header().Set("Permissions-Policy", headers.PermissionsPolicy)
		}

		// Strict-Transport-Security for HTTPS (only when SSL enabled)
		if m.config.Server.SSL.Enabled {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}

		next.ServeHTTP(w, r)
	})
}

// CORS handles Cross-Origin Resource Sharing
func (m *Middleware) CORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cors := m.config.Server.Security.CORS

		if !cors.Enabled {
			next.ServeHTTP(w, r)
			return
		}

		origin := r.Header.Get("Origin")
		if origin != "" {
			// Check if origin is allowed
			allowed := false
			for _, allowedOrigin := range cors.AllowedOrigins {
				if allowedOrigin == "*" || allowedOrigin == origin {
					allowed = true
					break
				}
			}

			if allowed {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				if cors.AllowCredentials {
					w.Header().Set("Access-Control-Allow-Credentials", "true")
				}
			}
		}

		// Handle preflight requests
		if r.Method == http.MethodOptions {
			w.Header().Set("Access-Control-Allow-Methods", strings.Join(cors.AllowedMethods, ", "))
			w.Header().Set("Access-Control-Allow-Headers", strings.Join(cors.AllowedHeaders, ", "))
			if cors.MaxAge > 0 {
				w.Header().Set("Access-Control-Max-Age", strconv.Itoa(cors.MaxAge))
			}
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

// RateLimit middleware applies rate limiting
func (m *Middleware) RateLimit(limiter *RateLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := getClientIP(r, m.config.Server.Security.TrustedProxies)

			// Check whitelist
			for _, whitelisted := range m.config.Server.RateLimit.Whitelist {
				if ip == whitelisted || strings.HasPrefix(ip, whitelisted) {
					next.ServeHTTP(w, r)
					return
				}
			}

			// Check blacklist
			for _, blacklisted := range m.config.Server.RateLimit.Blacklist {
				if ip == blacklisted || strings.HasPrefix(ip, blacklisted) {
					// Log blocked request
					if m.logManager != nil {
						m.logManager.Security().LogBlocked(ip, r.URL.Path, "blacklisted")
					}
					http.Error(w, "Forbidden", http.StatusForbidden)
					return
				}
			}

			if !limiter.Allow(ip) {
				// Log rate limited request
				if m.logManager != nil {
					m.logManager.Security().LogRateLimited(ip, r.URL.Path)
				}
				w.Header().Set("Retry-After", "60")
				http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
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

	// Validate token matches cookie
	if token != cookie.Value {
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

			// Set cookie
			http.SetCookie(w, &http.Cookie{
				Name:     csrf.CookieName,
				Value:    token,
				Path:     "/",
				HttpOnly: true,
				Secure:   c.config.Server.SSL.Enabled,
				SameSite: http.SameSiteLaxMode,
			})

			next.ServeHTTP(w, r)
			return
		}

		// Validate token for unsafe methods
		cookie, err := r.Cookie(csrf.CookieName)
		if err != nil {
			// Log CSRF violation
			if c.logManager != nil {
				ip := r.RemoteAddr
				if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
					ip = strings.Split(xff, ",")[0]
				}
				c.logManager.Security().LogCSRFViolation(ip, r.URL.Path)
			}
			http.Error(w, "CSRF token missing", http.StatusForbidden)
			return
		}

		// Get token from header or form
		token := r.Header.Get(csrf.HeaderName)
		if token == "" {
			token = r.FormValue(csrf.FieldName)
		}

		if token != cookie.Value {
			// Log CSRF violation
			if c.logManager != nil {
				ip := r.RemoteAddr
				if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
					ip = strings.Split(xff, ",")[0]
				}
				c.logManager.Security().LogCSRFViolation(ip, r.URL.Path)
			}
			http.Error(w, "CSRF token invalid", http.StatusForbidden)
			return
		}

		// Validate token exists in store
		if _, ok := c.tokens.Load(token); !ok {
			// Log CSRF violation
			if c.logManager != nil {
				ip := r.RemoteAddr
				if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
					ip = strings.Split(xff, ",")[0]
				}
				c.logManager.Security().LogCSRFViolation(ip, r.URL.Path)
			}
			http.Error(w, "CSRF token expired", http.StatusForbidden)
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
		if m.config.Server.Mode == "development" {
			ip := getClientIP(r, m.config.Server.Security.TrustedProxies)
			log.Printf("%s - - [%s] \"%s %s %s\" %d %d \"%.3fms\"",
				ip,
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
				// Use slog for structured logging with all context
				attrs := []slog.Attr{
					slog.String("error", errMsg),
					slog.String("method", r.Method),
					slog.String("path", r.URL.Path),
					slog.String("remote_addr", r.RemoteAddr),
					slog.String("user_agent", r.UserAgent()),
				}
				if requestID != "" {
					attrs = append(attrs, slog.String("request_id", requestID))
				}

				// Include stack trace in development mode
				if m.config.Server.Mode == "development" || m.config.IsDebug() {
					attrs = append(attrs, slog.String("stack", string(debug.Stack())))
				}

				slog.Error("PANIC recovered", slog.Any("attrs", attrs))

				// Also log to standard log for backward compatibility
				if requestID != "" {
					log.Printf("PANIC (request_id=%s): %s", requestID, errMsg)
				} else {
					log.Printf("PANIC: %s", errMsg)
				}

				// In development mode, show detailed error
				if m.config.Server.Mode == "development" {
					http.Error(w, "Internal Server Error: "+errMsg, http.StatusInternalServerError)
					return
				}

				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
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
		w.ResponseWriter.Header().Del("Content-Length") // Length changes with compression
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

			// Get client IP
			ip := getClientIP(r, m.config.Server.Security.TrustedProxies)

			// Get configured allowed/denied countries
			allowedCountries := m.config.Server.GeoIP.AllowedCountries
			denyCountries := m.config.Server.GeoIP.DenyCountries

			// Check if country is blocked
			if lookup.IsBlocked(ip, denyCountries) {
				result := lookup.Lookup(ip)
				// Log blocked request
				if m.logManager != nil {
					m.logManager.Security().LogBlocked(ip, r.URL.Path, "country:"+result.CountryCode)
				}
				http.Error(w, "Access Denied", http.StatusForbidden)
				return
			}

			// Check if country is allowed (if allowlist is configured)
			if !lookup.IsAllowed(ip, allowedCountries) {
				result := lookup.Lookup(ip)
				// Log blocked request
				if m.logManager != nil {
					m.logManager.Security().LogBlocked(ip, r.URL.Path, "country:"+result.CountryCode)
				}
				http.Error(w, "Access Denied", http.StatusForbidden)
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
			if path == "/healthz" || path == "/api/v1/healthz" {
				next.ServeHTTP(w, r)
				return
			}

			// Always allow admin routes (admins can work during maintenance)
			// Per AI.md PART 17: Admin path is configurable (default: "admin")
			adminPath := "/" + config.GetAdminPath()
			if strings.HasPrefix(path, adminPath) {
				next.ServeHTTP(w, r)
				return
			}

			// Always allow API maintenance status endpoint
			if path == "/api/v1/maintenance" || path == "/api/v1/status" {
				next.ServeHTTP(w, r)
				return
			}

			// Always allow static assets
			if strings.HasPrefix(path, "/static/") {
				next.ServeHTTP(w, r)
				return
			}

			// Log maintenance block
			if m.logManager != nil {
				ip := getClientIP(r, m.config.Server.Security.TrustedProxies)
				m.logManager.Security().LogBlocked(ip, path, "maintenance")
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

			// Simple maintenance page
			maintenanceHTML := `<!DOCTYPE html>
<html lang="en">
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
        <p>` + message + `</p>
    </div>
</body>
</html>`
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

// TargetType represents the context type extracted from URL path
// Per AI.md PART 11: Server-Side Context from URL (NON-NEGOTIABLE)
type TargetType int

const (
	TargetUnknown     TargetType = iota // Unknown/invalid target
	TargetPublic                        // Public routes (/, /api/v1/, project-specific like /search)
	TargetServerPages                   // Server pages - about, help, contact, privacy (/server/*)
	TargetAuth                          // Auth flows (/auth/*)
	TargetCurrentUser                   // Current user from token (/users/*)
	TargetUser                          // Specific user (/users/{username}/*)
	TargetOrg                           // Organization (/orgs/{slug}/*)
	TargetAdmin                         // Server admin panel (/admin/*)
	TargetAdminServer                   // Server settings within admin (/admin/server/*)
)

// String returns the string representation of TargetType
func (t TargetType) String() string {
	switch t {
	case TargetPublic:
		return "public"
	case TargetServerPages:
		return "server"
	case TargetAuth:
		return "auth"
	case TargetCurrentUser:
		return "current_user"
	case TargetUser:
		return "user"
	case TargetOrg:
		return "org"
	case TargetAdmin:
		return "admin"
	case TargetAdminServer:
		return "admin_server"
	default:
		return "unknown"
	}
}

// RequestContext holds context extracted from URL path
// Per AI.md PART 11: Context is determined from URL path, NOT headers
type RequestContext struct {
	Type TargetType
	Name string // Username or org slug when applicable
}

// ContextMiddleware extracts context from URL path and validates token access
// Per AI.md PART 11: Routes are always URL-scoped. Context is determined from URL path.
func (m *Middleware) ContextMiddleware(adminPath string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Extract context from URL path
			ctx := extractContextFromPath(r.URL.Path, adminPath)

			// Store in request context for downstream handlers
			r = r.WithContext(context.WithValue(r.Context(), "target_context", ctx))

			next.ServeHTTP(w, r)
		})
	}
}

// extractContextFromPath determines context from URL path
// Per AI.md PART 11 line 11314-11365
func extractContextFromPath(urlPath, adminPath string) *RequestContext {
	// Normalize admin path
	if adminPath == "" {
		adminPath = "admin"
	}
	adminPath = strings.TrimPrefix(adminPath, "/")

	// Remove leading slash for easier parsing
	path := strings.TrimPrefix(urlPath, "/")
	parts := strings.Split(path, "/")

	if len(parts) == 0 || parts[0] == "" {
		return &RequestContext{Type: TargetPublic}
	}

	// Check for API routes
	if parts[0] == "api" {
		if len(parts) < 2 {
			return &RequestContext{Type: TargetPublic}
		}
		// Skip API version (v1, v2, etc.)
		if len(parts) < 3 {
			return &RequestContext{Type: TargetPublic}
		}
		parts = parts[2:] // Remove "api" and version
		if len(parts) == 0 {
			return &RequestContext{Type: TargetPublic}
		}
	}

	switch parts[0] {
	case "server":
		// /server/* or /api/v1/server/* - public server pages (about, help, contact, privacy)
		return &RequestContext{Type: TargetServerPages}
	case "auth":
		// /auth/* or /api/v1/auth/* - authentication flows (public)
		return &RequestContext{Type: TargetAuth}
	case "users":
		if len(parts) > 1 && parts[1] != "" {
			// /users/{username}/* - specific user
			return &RequestContext{Type: TargetUser, Name: parts[1]}
		}
		// /users/* - current user (from token)
		return &RequestContext{Type: TargetCurrentUser}
	case "orgs":
		if len(parts) < 2 || parts[1] == "" {
			return &RequestContext{Type: TargetPublic} // Invalid org route
		}
		// /orgs/{slug}/*
		return &RequestContext{Type: TargetOrg, Name: parts[1]}
	case adminPath:
		// Check for server settings within admin
		if len(parts) > 1 && parts[1] == "server" {
			// /admin/server/* - server settings
			return &RequestContext{Type: TargetAdminServer}
		}
		// /admin/* - admin panel
		return &RequestContext{Type: TargetAdmin}
	default:
		// Project-specific public routes (e.g., /search, /healthz)
		return &RequestContext{Type: TargetPublic}
	}
}

// GetRequestContext retrieves the request context from the request
func GetRequestContext(r *http.Request) *RequestContext {
	if ctx := r.Context().Value("target_context"); ctx != nil {
		if rc, ok := ctx.(*RequestContext); ok {
			return rc
		}
	}
	return &RequestContext{Type: TargetUnknown}
}

// TokenType represents the type of API token
// Per AI.md PART 11: Token prefixes (NON-NEGOTIABLE)
type TokenType int

const (
	TokenTypeUnknown  TokenType = iota
	TokenTypeAdmin              // adm_ prefix
	TokenTypeUser               // usr_ prefix
	TokenTypeOrg                // org_ prefix
	TokenTypeAdminAgt           // adm_agt_ prefix (admin agent)
	TokenTypeUserAgt            // usr_agt_ prefix (user agent)
	TokenTypeOrgAgt             // org_agt_ prefix (org agent)
)

// TokenInfo holds validated token information
// Per AI.md PART 11: Token validation
type TokenInfo struct {
	Type     TokenType
	OwnerID  int64  // admin.id, user.id, or org.id
	Prefix   string // First 8 chars for display
	Scope    string // global, read-write, read
	Username string // For user tokens, the associated username
	OrgSlug  string // For org tokens, the specific org slug
}

// getTokenFromRequest extracts the token from Authorization header or cookie
// Per AI.md PART 11: Token validation
func getTokenFromRequest(r *http.Request) string {
	// Check Authorization header first (Bearer token)
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}

	// Check X-API-Key header
	if apiKey := r.Header.Get("X-API-Key"); apiKey != "" {
		return apiKey
	}

	// Check query parameter (for debugging/testing only)
	if token := r.URL.Query().Get("token"); token != "" {
		return token
	}

	return ""
}

// parseTokenType determines the token type from prefix
// Per AI.md PART 11 lines 11078-11136
func parseTokenType(token string) TokenType {
	// Check compound agent prefixes first (adm_agt_, usr_agt_, org_agt_)
	if strings.HasPrefix(token, "adm_agt_") {
		return TokenTypeAdminAgt
	}
	if strings.HasPrefix(token, "usr_agt_") {
		return TokenTypeUserAgt
	}
	if strings.HasPrefix(token, "org_agt_") {
		return TokenTypeOrgAgt
	}

	// Standard single-prefix tokens
	if strings.HasPrefix(token, "adm_") {
		return TokenTypeAdmin
	}
	if strings.HasPrefix(token, "usr_") {
		return TokenTypeUser
	}
	if strings.HasPrefix(token, "org_") {
		return TokenTypeOrg
	}

	return TokenTypeUnknown
}

// ErrNoAccess is returned when token lacks access to requested context
var ErrNoAccess = fmt.Errorf("no access to requested resource")

// ErrInvalidToken is returned for malformed tokens
var ErrInvalidToken = fmt.Errorf("invalid token format")

// validateTokenAccess validates that a token has access to the given context
// Per AI.md PART 11 lines 11251-11262: Token scope determines allowed access
func validateTokenAccess(tokenType TokenType, ctx *RequestContext) error {
	// Public routes are always accessible
	if ctx.Type == TargetPublic || ctx.Type == TargetServerPages || ctx.Type == TargetAuth {
		return nil
	}

	switch tokenType {
	case TokenTypeAdmin:
		// Admin tokens can access admin panel and server settings
		if ctx.Type == TargetAdmin || ctx.Type == TargetAdminServer {
			return nil
		}
		return ErrNoAccess

	case TokenTypeUser:
		// User tokens can access user routes and orgs they belong to
		// Note: Actual org membership validation happens in the handler
		if ctx.Type == TargetCurrentUser || ctx.Type == TargetUser || ctx.Type == TargetOrg {
			return nil
		}
		return ErrNoAccess

	case TokenTypeOrg:
		// Org tokens can only access their specific org
		// Note: Specific org validation happens in the handler
		if ctx.Type == TargetOrg {
			return nil
		}
		return ErrNoAccess

	case TokenTypeAdminAgt:
		// Admin agent tokens can access admin server agents routes
		if ctx.Type == TargetAdminServer {
			return nil
		}
		return ErrNoAccess

	case TokenTypeUserAgt:
		// User agent tokens can access user agent routes
		if ctx.Type == TargetCurrentUser {
			return nil
		}
		return ErrNoAccess

	case TokenTypeOrgAgt:
		// Org agent tokens can access their org's agent routes
		if ctx.Type == TargetOrg {
			return nil
		}
		return ErrNoAccess

	case TokenTypeUnknown:
		// Unknown token type - reject non-public routes
		return ErrInvalidToken
	}

	return ErrNoAccess
}

// TokenValidationMiddleware validates API tokens and checks access per URL context
// Per AI.md PART 11 lines 11282-11311: Server request handling
func (m *Middleware) TokenValidationMiddleware(adminPath string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := getTokenFromRequest(r)
			if token == "" {
				// No token - anonymous access (if route allows)
				next.ServeHTTP(w, r)
				return
			}

			// Parse token type from prefix
			tokenType := parseTokenType(token)
			if tokenType == TokenTypeUnknown {
				http.Error(w, `{"error": "invalid token format"}`, http.StatusUnauthorized)
				return
			}

			// Extract context from URL path
			ctx := extractContextFromPath(r.URL.Path, adminPath)

			// Validate token has access to this context
			if err := validateTokenAccess(tokenType, ctx); err != nil {
				if err == ErrNoAccess {
					http.Error(w, `{"error": "no access to requested resource"}`, http.StatusForbidden)
				} else {
					http.Error(w, `{"error": "invalid token"}`, http.StatusUnauthorized)
				}
				return
			}

			// Store token type in context for downstream handlers
			r = r.WithContext(context.WithValue(r.Context(), "token_type", tokenType))
			r = r.WithContext(context.WithValue(r.Context(), "token", token))

			next.ServeHTTP(w, r)
		})
	}
}

// GetTokenFromContext retrieves the token string from request context
func GetTokenFromContext(r *http.Request) string {
	if token := r.Context().Value("token"); token != nil {
		if t, ok := token.(string); ok {
			return t
		}
	}
	return ""
}

// GetTokenTypeFromContext retrieves the token type from request context
func GetTokenTypeFromContext(r *http.Request) TokenType {
	if tt := r.Context().Value("token_type"); tt != nil {
		if t, ok := tt.(TokenType); ok {
			return t
		}
	}
	return TokenTypeUnknown
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
			http.Error(w, "Bad Request", http.StatusBadRequest)
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
