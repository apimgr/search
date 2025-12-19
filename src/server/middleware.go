package server

import (
	"compress/gzip"
	"crypto/rand"
	"encoding/hex"
	"io"
	"log"
	"net"
	"net/http"
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
				w.Header().Set("Access-Control-Max-Age", string(rune(cors.MaxAge)))
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
func (m *Middleware) Recovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				log.Printf("PANIC: %v", err)

				// In development mode, show detailed error
				if m.config.Server.Mode == "development" {
					http.Error(w, "Internal Server Error: "+string(err.(string)), http.StatusInternalServerError)
					return
				}

				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			}
		}()

		next.ServeHTTP(w, r)
	})
}

// RequestID middleware adds a unique request ID (UUID v4 per TEMPLATE.md)
func (m *Middleware) RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var requestID string

		// Check for incoming X-Request-ID header first (per TEMPLATE.md)
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
