package httputil

import (
	"net/http"
	"strings"

	"github.com/apimgr/search/src/config"
)

// ProjectName is the name of this project, used for CLI client detection
const ProjectName = "search"

// isOurCliClient detects our own CLI client
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
		"lynx/",     // Lynx - classic text browser
		"w3m/",      // w3m - text browser with table support
		"links ",    // Links - text browser (note: space after)
		"links/",    // Links alternative format
		"elinks/",   // ELinks - enhanced links
		"browsh/",   // Browsh - modern text browser
		"carbonyl/", // Carbonyl - Chromium in terminal
		"netsurf",   // NetSurf - lightweight browser (limited JS)
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
	ClientTypeBrowser            // Regular browser - gets full HTML with JS
	ClientTypeTextBrowser        // Text browser (lynx, w3m) - gets HTML without JS
	ClientTypeHttpTool           // HTTP tool (curl, wget) - gets formatted text
	ClientTypeOurCLI             // Our CLI client - gets JSON
	ClientTypeAPI                // API client - gets JSON
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

// GetBaseURLFromRequest returns the base URL path prefix from the request
// Per AI.md PART 5: {baseurl} resolution from reverse proxy headers
// Priority: X-Forwarded-Prefix → X-Forwarded-Path → X-Script-Name → config → "/"
func GetBaseURLFromRequest(r *http.Request) string {
	// Check reverse proxy headers first (highest priority)
	if prefix := r.Header.Get("X-Forwarded-Prefix"); prefix != "" {
		return normalizeBasePath(prefix)
	}
	if path := r.Header.Get("X-Forwarded-Path"); path != "" {
		return normalizeBasePath(path)
	}
	if scriptName := r.Header.Get("X-Script-Name"); scriptName != "" {
		return normalizeBasePath(scriptName)
	}

	// Fall back to config
	return config.GetBaseURL()
}

// GetProtoFromRequest returns the protocol from the request
// Per AI.md PART 5: {proto} resolution from reverse proxy headers
// Priority: X-Forwarded-Proto → X-Forwarded-Ssl → TLS detection → "http"
func GetProtoFromRequest(r *http.Request) string {
	// Check X-Forwarded-Proto
	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		return strings.ToLower(proto)
	}

	// Check X-Forwarded-Ssl
	if ssl := r.Header.Get("X-Forwarded-Ssl"); ssl != "" {
		if strings.ToLower(ssl) == "on" {
			return "https"
		}
	}

	// Check if TLS connection
	if r.TLS != nil {
		return "https"
	}

	return "http"
}

// GetHostFromRequest returns the host from the request
// Per AI.md PART 5: {fqdn} resolution from reverse proxy headers
// Priority: X-Forwarded-Host → Host header → Request.Host
func GetHostFromRequest(r *http.Request) string {
	// Check X-Forwarded-Host
	if host := r.Header.Get("X-Forwarded-Host"); host != "" {
		// May contain multiple hosts, use first one
		if idx := strings.Index(host, ","); idx != -1 {
			return strings.TrimSpace(host[:idx])
		}
		return host
	}

	// Use Host header or Request.Host
	return r.Host
}

// GetPortFromRequest returns the port from the request
// Per AI.md PART 5: {port} resolution from reverse proxy headers
// Priority: X-Forwarded-Port → Host header → Server port → Proto default
func GetPortFromRequest(r *http.Request) string {
	// Check X-Forwarded-Port
	if port := r.Header.Get("X-Forwarded-Port"); port != "" {
		return port
	}

	// Extract from Host header
	host := GetHostFromRequest(r)
	if idx := strings.LastIndex(host, ":"); idx != -1 {
		// Make sure it's not IPv6 bracket
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

// BuildFullURL builds a full URL from request context
// Per AI.md PART 5: URL Variables (NON-NEGOTIABLE)
func BuildFullURL(r *http.Request, path string) string {
	proto := GetProtoFromRequest(r)
	host := GetHostFromRequest(r)
	baseURL := GetBaseURLFromRequest(r)

	// Remove port from host if it's the default port
	port := GetPortFromRequest(r)
	if (proto == "http" && port == "80") || (proto == "https" && port == "443") {
		// Strip default port from host if present
		if idx := strings.LastIndex(host, ":"); idx != -1 {
			hostPort := host[idx+1:]
			if hostPort == port {
				host = host[:idx]
			}
		}
	}

	// Normalize paths
	baseURL = normalizeBasePath(baseURL)
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	// Combine base URL and path, avoiding double slashes
	fullPath := baseURL + path
	if baseURL != "/" && strings.HasPrefix(path, "/") {
		fullPath = baseURL + path
	}

	return proto + "://" + host + fullPath
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
