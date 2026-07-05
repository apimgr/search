package httputil

import (
	"net"
	"net/http"
	"strings"
	"sync"

	"github.com/apimgr/search/src/config"
)

// ProjectName is the name of this project, used for CLI client detection
const ProjectName = "search"

// IsOurCliClient detects our own CLI client
// Our CLI is INTERACTIVE (TUI/GUI) - receives JSON, renders itself
// Per AI.md PART 14: Client Type Detection & Response
func IsOurCliClient(r *http.Request) bool {
	ua := r.Header.Get("User-Agent")
	return strings.HasPrefix(ua, ProjectName+"-cli/")
}

// IsTextBrowser detects text-mode browsers (lynx, w3m, links, etc.)
// Text browsers are INTERACTIVE but do NOT support JavaScript
// They receive no-JS HTML alternative (server-rendered, standard form POST)
// Per AI.md PART 14: Client Type Detection & Response
func IsTextBrowser(r *http.Request) bool {
	ua := strings.ToLower(r.Header.Get("User-Agent"))

	// Text browsers - INTERACTIVE, NO JavaScript support
	// Format: "browser/" or "browser " (links uses space)
	textBrowsers := []string{
		// Lynx - classic text browser
		"lynx/",
		// w3m - text browser with table support
		"w3m/",
		// Links - text browser (note: space after)
		"links ",
		// Links alternative format
		"links/",
		// ELinks - enhanced links
		"elinks/",
		// Browsh - modern text browser
		"browsh/",
		// Carbonyl - Chromium in terminal
		"carbonyl/",
		// NetSurf - lightweight browser (limited JS)
		"netsurf",
	}
	for _, browser := range textBrowsers {
		if strings.Contains(ua, browser) {
			return true
		}
	}
	return false
}

// IsHttpTool detects HTTP tools (curl, wget, httpie, etc.)
// HTTP tools are NON-INTERACTIVE - they just dump output
// Per AI.md PART 14: Client Type Detection & Response
func IsHttpTool(r *http.Request) bool {
	ua := strings.ToLower(r.Header.Get("User-Agent"))

	httpTools := []string{
		"curl/", "wget/", "httpie/",
		"libcurl/", "python-requests/",
		"go-http-client/", "axios/", "node-fetch/",
		"postman", "insomnia", "paw/",
	}
	for _, tool := range httpTools {
		if strings.Contains(ua, tool) {
			return true
		}
	}
	return false
}

// IsNonInteractiveClient detects non-interactive clients (HTTP tools)
// These clients just dump output and don't interact with forms
// Per AI.md PART 14: Non-interactive tools get pre-formatted text
func IsNonInteractiveClient(r *http.Request) bool {
	return IsHttpTool(r)
}

// IsBrowser detects if the request is from a regular browser
// Regular browsers get full HTML with JavaScript
// Per AI.md PART 14: Client Type Detection & Response
func IsBrowser(r *http.Request) bool {
	ua := strings.ToLower(r.Header.Get("User-Agent"))

	// Common browser signatures
	browserSignatures := []string{
		"mozilla/",
		"chrome/",
		"safari/",
		"firefox/",
		"edge/",
		"opera/",
		"msie",
		"trident/",
	}

	// Check if it's a known non-browser first
	if IsOurCliClient(r) || IsTextBrowser(r) || IsHttpTool(r) {
		return false
	}

	// Check for browser signatures
	for _, sig := range browserSignatures {
		if strings.Contains(ua, sig) {
			return true
		}
	}

	// Default to browser if Accept header suggests HTML
	accept := r.Header.Get("Accept")
	return strings.Contains(accept, "text/html")
}

// DetectClientType returns the type of client making the request
// Per AI.md PART 14: Client Type Detection & Response
type ClientType int

const (
	ClientTypeUnknown ClientType = iota
	// Regular browser - gets full HTML with JS
	ClientTypeBrowser
	// Text browser (lynx, w3m) - gets HTML without JS
	ClientTypeTextBrowser
	// HTTP tool (curl, wget) - gets formatted text
	ClientTypeHttpTool
	// Our CLI client - gets JSON
	ClientTypeOurCLI
	// API client - gets JSON
	ClientTypeAPI
)

// String returns the string representation of ClientType
func (c ClientType) String() string {
	switch c {
	case ClientTypeBrowser:
		return "browser"
	case ClientTypeTextBrowser:
		return "text_browser"
	case ClientTypeHttpTool:
		return "http_tool"
	case ClientTypeOurCLI:
		return "our_cli"
	case ClientTypeAPI:
		return "api"
	default:
		return "unknown"
	}
}

// DetectClientType determines the type of client from the request
// Per AI.md PART 14: Client Type Detection & Response
func DetectClientType(r *http.Request) ClientType {
	// Check for our CLI client first
	if IsOurCliClient(r) {
		return ClientTypeOurCLI
	}

	// Check for text browsers
	if IsTextBrowser(r) {
		return ClientTypeTextBrowser
	}

	// Check for HTTP tools
	if IsHttpTool(r) {
		return ClientTypeHttpTool
	}

	// Check if this is an API request (Accept: application/json)
	accept := r.Header.Get("Accept")
	if strings.Contains(accept, "application/json") {
		return ClientTypeAPI
	}

	// Check if it's a browser
	if IsBrowser(r) {
		return ClientTypeBrowser
	}

	return ClientTypeUnknown
}

// GetPreferredFormat returns the preferred response format based on client type
// Per AI.md PART 14: Content Negotiation Priority
func GetPreferredFormat(r *http.Request) string {
	// 1. Check for .txt extension - ALWAYS returns text
	if strings.HasSuffix(r.URL.Path, ".txt") {
		return "text/plain"
	}

	// 2. Check Accept header explicitly
	accept := r.Header.Get("Accept")
	if accept != "" {
		if strings.Contains(accept, "application/json") {
			return "application/json"
		}
		if strings.Contains(accept, "text/plain") {
			return "text/plain"
		}
		if strings.Contains(accept, "text/html") {
			return "text/html"
		}
	}

	// 3. Detect client type and return appropriate format
	clientType := DetectClientType(r)

	switch clientType {
	case ClientTypeOurCLI, ClientTypeAPI:
		return "application/json"
	case ClientTypeHttpTool:
		return "text/plain"
	case ClientTypeTextBrowser, ClientTypeBrowser:
		return "text/html"
	default:
		// Default based on path
		if strings.HasPrefix(r.URL.Path, "/api/") {
			return "application/json"
		}
		return "text/html"
	}
}

// additionalTrustedProxies holds extra IPs/CIDRs beyond always-trusted private ranges.
// Set once at startup via SetAdditionalTrustedProxies.
var (
	additionalTrustedProxies   []*net.IPNet
	additionalTrustedProxiesMu sync.RWMutex
)

// alwaysTrustedCIDRs lists private and loopback ranges that are always trusted.
// Per AI.md PART 12: loopback, RFC1918, fc00::/7, link-local always trusted.
var alwaysTrustedCIDRs = func() []*net.IPNet {
	cidrs := []string{
		"127.0.0.0/8",    // Loopback IPv4
		"::1/128",        // Loopback IPv6
		"10.0.0.0/8",     // RFC 1918
		"172.16.0.0/12",  // RFC 1918
		"192.168.0.0/16", // RFC 1918
		"fc00::/7",       // RFC 4193 unique-local
		"169.254.0.0/16", // Link-local IPv4
		"fe80::/10",      // Link-local IPv6
	}
	nets := make([]*net.IPNet, 0, len(cidrs))
	for _, cidr := range cidrs {
		_, ipNet, err := net.ParseCIDR(cidr)
		if err == nil {
			nets = append(nets, ipNet)
		}
	}
	return nets
}()

// SetAdditionalTrustedProxies configures extra IPs/CIDRs beyond always-trusted private ranges.
// Call once at startup with server.trusted_proxies.additional from config.
// Plain IPs are auto-expanded to /32 (IPv4) or /128 (IPv6).
func SetAdditionalTrustedProxies(additional []string) {
	parsed := make([]*net.IPNet, 0, len(additional))
	for _, s := range additional {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		// Try parsing as CIDR first
		_, ipNet, err := net.ParseCIDR(s)
		if err == nil {
			parsed = append(parsed, ipNet)
			continue
		}
		// Try as plain IP, expand to /32 (IPv4) or /128 (IPv6)
		ip := net.ParseIP(s)
		if ip == nil {
			continue
		}
		cidr := s + "/32"
		if ip.To4() == nil {
			cidr = s + "/128"
		}
		_, ipNet, err = net.ParseCIDR(cidr)
		if err == nil {
			parsed = append(parsed, ipNet)
		}
	}
	additionalTrustedProxiesMu.Lock()
	additionalTrustedProxies = parsed
	additionalTrustedProxiesMu.Unlock()
}

// isTrustedProxy returns true if remoteAddr is in the always-trusted private ranges
// or in the configured additional trusted proxies list.
// remoteAddr should be in "IP:port" or "IP" format (from r.RemoteAddr).
func isTrustedProxy(remoteAddr string) bool {
	ipStr, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		// No port — use as-is
		ipStr = remoteAddr
	}
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}
	// Check always-trusted private/loopback ranges
	for _, cidr := range alwaysTrustedCIDRs {
		if cidr.Contains(ip) {
			return true
		}
	}
	// Check additional configured proxies
	additionalTrustedProxiesMu.RLock()
	additional := additionalTrustedProxies
	additionalTrustedProxiesMu.RUnlock()
	for _, cidr := range additional {
		if cidr.Contains(ip) {
			return true
		}
	}
	return false
}

// GetClientIP extracts the real client IP from the request.
// Per AI.md PART 12: X-Forwarded-* headers honored only from trusted proxies.
// Header priority (from trusted proxy only): CF-Connecting-IP → True-Client-IP → X-Client-IP → X-Real-IP → X-Forwarded-For → RemoteAddr
func GetClientIP(r *http.Request) string {
	remoteIP, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		remoteIP = r.RemoteAddr
	}
	if isTrustedProxy(r.RemoteAddr) {
		// Cloudflare passes the real client IP in CF-Connecting-IP
		if ip := r.Header.Get("CF-Connecting-IP"); ip != "" {
			return strings.TrimSpace(ip)
		}
		// Cloudflare/Akamai True-Client-IP
		if ip := r.Header.Get("True-Client-IP"); ip != "" {
			return strings.TrimSpace(ip)
		}
		// X-Client-IP (HAProxy and others)
		if ip := r.Header.Get("X-Client-IP"); ip != "" {
			return strings.TrimSpace(ip)
		}
		// X-Real-IP (nginx standard)
		if xri := r.Header.Get("X-Real-IP"); xri != "" {
			return strings.TrimSpace(xri)
		}
		// X-Forwarded-For — use leftmost (client) IP
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			ips := strings.Split(xff, ",")
			if len(ips) > 0 {
				return strings.TrimSpace(ips[0])
			}
		}
	}
	return remoteIP
}

// GetBaseURLFromRequest returns the base URL path prefix from the request.
// Per AI.md PART 12: X-Forwarded headers only honored from trusted proxies.
// Priority: X-Forwarded-Prefix → X-Forwarded-Path → X-Script-Name → config → "/"
func GetBaseURLFromRequest(r *http.Request) string {
	if isTrustedProxy(r.RemoteAddr) {
		if prefix := r.Header.Get("X-Forwarded-Prefix"); prefix != "" {
			return normalizeBasePath(prefix)
		}
		if path := r.Header.Get("X-Forwarded-Path"); path != "" {
			return normalizeBasePath(path)
		}
		if scriptName := r.Header.Get("X-Script-Name"); scriptName != "" {
			return normalizeBasePath(scriptName)
		}
	}
	return config.GetBaseURL()
}

// GetProtoFromRequest returns the protocol from the request.
// Per AI.md PART 12: X-Forwarded headers only honored from trusted proxies.
// Priority: X-Forwarded-Proto → X-Forwarded-Protocol → X-Forwarded-Ssl → X-Url-Scheme → X-Scheme → Forwarded → TLS → "http"
func GetProtoFromRequest(r *http.Request) string {
	if isTrustedProxy(r.RemoteAddr) {
		if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
			return strings.ToLower(proto)
		}
		// X-Forwarded-Protocol is an alternate spelling used by some load balancers
		if proto := r.Header.Get("X-Forwarded-Protocol"); proto != "" {
			return strings.ToLower(proto)
		}
		if ssl := r.Header.Get("X-Forwarded-Ssl"); ssl != "" {
			if strings.ToLower(ssl) == "on" {
				return "https"
			}
			return "http"
		}
		if scheme := r.Header.Get("X-Url-Scheme"); scheme != "" {
			return strings.ToLower(scheme)
		}
		// X-Scheme is used by some nginx configurations
		if scheme := r.Header.Get("X-Scheme"); scheme != "" {
			return strings.ToLower(scheme)
		}
		// RFC 7239 Forwarded header: parse proto=https
		if fwd := r.Header.Get("Forwarded"); fwd != "" {
			if proto := parseForwardedProto(fwd); proto != "" {
				return proto
			}
		}
	}
	if r.TLS != nil {
		return "https"
	}
	return "http"
}

// parseForwardedProto extracts the proto value from an RFC 7239 Forwarded header.
// Example: "for=10.0.0.1; proto=https" → "https"
func parseForwardedProto(fwd string) string {
	for _, part := range strings.Split(fwd, ";") {
		part = strings.TrimSpace(part)
		if after, ok := strings.CutPrefix(strings.ToLower(part), "proto="); ok {
			return strings.Trim(strings.TrimSpace(after), `"`)
		}
	}
	return ""
}

// GetHostFromRequest returns the host from the request.
// Per AI.md PART 12: X-Forwarded headers only honored from trusted proxies.
// Priority: X-Forwarded-Host → X-Real-Host → X-Original-Host → Host header
func GetHostFromRequest(r *http.Request) string {
	if isTrustedProxy(r.RemoteAddr) {
		if host := r.Header.Get("X-Forwarded-Host"); host != "" {
			// May contain multiple hosts — use first one
			if idx := strings.Index(host, ","); idx != -1 {
				return strings.TrimSpace(host[:idx])
			}
			return host
		}
		if host := r.Header.Get("X-Real-Host"); host != "" {
			return host
		}
		if host := r.Header.Get("X-Original-Host"); host != "" {
			return host
		}
	}
	return r.Host
}

// GetPortFromRequest returns the port from the request.
// Per AI.md PART 12: X-Forwarded headers only honored from trusted proxies.
// Priority: X-Forwarded-Port → Host header → Proto default
func GetPortFromRequest(r *http.Request) string {
	if isTrustedProxy(r.RemoteAddr) {
		if port := r.Header.Get("X-Forwarded-Port"); port != "" {
			return port
		}
	}
	host := GetHostFromRequest(r)
	if idx := strings.LastIndex(host, ":"); idx != -1 {
		// Make sure it's not an IPv6 bracket
		if !strings.Contains(host[idx:], "]") {
			return host[idx+1:]
		}
	}
	// Default based on protocol
	if GetProtoFromRequest(r) == "https" {
		return "443"
	}
	return "80"
}

// GetURLVars returns the resolved proto, fqdn, and port from the request.
// Port is the empty string when it is the default for the protocol (80/443).
// Per AI.md PART 12: reverse-proxy headers honored only from trusted proxies.
func GetURLVars(r *http.Request) (proto, fqdn, port string) {
	proto = GetProtoFromRequest(r)
	fqdn = GetHostFromRequest(r)
	// Strip port from fqdn if present, capture it separately
	if idx := strings.LastIndex(fqdn, ":"); idx != -1 && !strings.Contains(fqdn[idx:], "]") {
		port = fqdn[idx+1:]
		fqdn = fqdn[:idx]
	} else {
		port = GetPortFromRequest(r)
	}
	// Suppress default ports (80 for http, 443 for https)
	if (proto == "http" && port == "80") || (proto == "https" && port == "443") {
		port = ""
	}
	return proto, fqdn, port
}

// BuildURL constructs a full URL from request context.
// Default ports (:80 and :443) are never included in the output.
// Per AI.md PART 12: reverse-proxy headers honored only from trusted proxies.
func BuildURL(r *http.Request, path string) string {
	proto, fqdn, port := GetURLVars(r)
	baseURL := GetBaseURLFromRequest(r)
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	var fullPath string
	if baseURL == "/" {
		fullPath = path
	} else {
		fullPath = strings.TrimRight(baseURL, "/") + path
	}
	if port == "" {
		return proto + "://" + fqdn + fullPath
	}
	return proto + "://" + fqdn + ":" + port + fullPath
}

// BuildFullURL is a backward-compatible alias for BuildURL.
// Deprecated: use BuildURL instead.
func BuildFullURL(r *http.Request, path string) string {
	return BuildURL(r, path)
}

// normalizeBasePath normalizes a base path
func normalizeBasePath(path string) string {
	if path == "" {
		return "/"
	}
	// Ensure it starts with /
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	// Remove trailing slash unless it's just "/"
	if len(path) > 1 && strings.HasSuffix(path, "/") {
		path = path[:len(path)-1]
	}
	return path
}
