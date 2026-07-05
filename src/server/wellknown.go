package server

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/apimgr/search/src/common/i18n"
)

// handleWellKnownChangePassword handles /.well-known/change-password per RFC 8615.
// Per IDEA.md, this project has no user accounts and no admin web UI — there is
// nothing for an end user to change. Respond with 404 so clients fall back.
func (s *Server) handleWellKnownChangePassword(w http.ResponseWriter, r *http.Request) {
	http.NotFound(w, r)
}

// handleWellKnownCatchAll handles unknown /.well-known/* paths per AI.md PART 11.
// Per spec: unsupported entries MUST return 404 Not Found.
// Per spec: GET and HEAD are the only valid methods; other methods return 405.
// Per spec: /.well-known/ itself MUST NOT list a directory index.
func (s *Server) handleWellKnownCatchAll(w http.ResponseWriter, r *http.Request) {
	// Only GET and HEAD are valid for /.well-known/**
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	// Unknown well-known path — return 404
	http.NotFound(w, r)
}

// handleRobotsTxt serves robots.txt per AI.md spec
// This is the enhanced handler that replaces the basic one
func (s *Server) handleRobotsTxt(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	// Cache for 1 day per AI.md spec
	w.Header().Set("Cache-Control", "public, max-age=86400")

	web := s.config.Server.Web

	// Header comment per AI.md spec
	fmt.Fprintln(w, "# robots.txt - Search Engine Crawling Rules")
	fmt.Fprintf(w, "# %s\n", s.config.Server.Title)
	fmt.Fprintln(w)

	fmt.Fprintln(w, "User-agent: *")

	// Allow paths (default: /, /api)
	allowPaths := web.Robots.Allow
	if len(allowPaths) == 0 {
		allowPaths = []string{"/", "/api"}
	}
	for _, path := range allowPaths {
		fmt.Fprintf(w, "Allow: %s\n", path)
	}

	// Deny paths (no defaults — there is no admin panel in this project).
	for _, path := range web.Robots.Deny {
		fmt.Fprintf(w, "Disallow: %s\n", path)
	}

	// Add sitemap URL per AI.md spec
	fmt.Fprintln(w)
	baseURL := s.getBaseURL(r)
	fmt.Fprintf(w, "Sitemap: %s/sitemap.xml\n", baseURL)
}

// handleSecurityTxtEnhanced serves security.txt per RFC 9116 and AI.md spec
// Required fields: Contact, Expires
// Optional fields: Preferred-Languages, Canonical
func (s *Server) handleSecurityTxtEnhanced(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	// Cache for 1 day per AI.md spec
	w.Header().Set("Cache-Control", "public, max-age=86400")

	security := s.config.Server.Web.Security
	baseURL := s.getBaseURL(r)

	// Contact (REQUIRED per RFC 9116)
	// Must be a URI (mailto:, https://, or tel:)
	contact := security.Contact
	if contact == "" && s.config.Server.Contact.Email != "" {
		contact = s.config.Server.Contact.Email
	}
	if contact != "" {
		// Ensure mailto: prefix for email addresses
		if strings.Contains(contact, "@") && !strings.HasPrefix(contact, "mailto:") && !strings.HasPrefix(contact, "https://") && !strings.HasPrefix(contact, "tel:") {
			contact = "mailto:" + contact
		}
		fmt.Fprintf(w, "Contact: %s\n", contact)
	} else {
		// Fallback to fqdn-based security email
		fqdn := extractHostFromURL(baseURL)
		fmt.Fprintf(w, "Contact: mailto:security@%s\n", fqdn)
	}

	// Expires (REQUIRED per RFC 9116)
	// Must be in ISO 8601 format (YYYY-MM-DDTHH:MM:SSZ)
	expires := security.Expires
	if expires == "" {
		// Default: 1 year from now (auto-renewed yearly per AI.md)
		expiryTime := time.Now().AddDate(1, 0, 0)
		expires = expiryTime.UTC().Format(time.RFC3339)
	}
	fmt.Fprintf(w, "Expires: %s\n", expires)

	// Preferred-Languages (OPTIONAL per RFC 9116)
	// Auto-generated from i18n config per AI.md
	languages := s.getPreferredLanguages()
	if len(languages) > 0 {
		fmt.Fprintf(w, "Preferred-Languages: %s\n", strings.Join(languages, ", "))
	}

	// Canonical (OPTIONAL per RFC 9116)
	// Canonical URL of the security.txt file
	fmt.Fprintf(w, "Canonical: %s/.well-known/security.txt\n", baseURL)
}

// getPreferredLanguages returns the list of supported languages for security.txt
// Per AI.md: auto-generated from i18n config
func (s *Server) getPreferredLanguages() []string {
	// Get supported languages from i18n manager
	if s.i18nManager != nil {
		return s.i18nManager.SupportedLanguageCodes()
	}

	// Fallback to default supported languages
	return i18n.DefaultSupportedLanguages()
}

// extractHostFromURL extracts the hostname from a URL
func extractHostFromURL(urlStr string) string {
	host := urlStr
	host = strings.TrimPrefix(host, "https://")
	host = strings.TrimPrefix(host, "http://")

	// Remove port if present
	if colonIdx := strings.Index(host, ":"); colonIdx != -1 {
		host = host[:colonIdx]
	}

	// Remove path if present
	if slashIdx := strings.Index(host, "/"); slashIdx != -1 {
		host = host[:slashIdx]
	}

	return host
}

// handleLlmsTxt serves /.well-known/llms.txt and /llms.txt per AI.md spec.
// Tells AI agents what the application does and what API endpoints are available.
func (s *Server) handleLlmsTxt(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	// Cache for 1 day per AI.md spec
	w.Header().Set("Cache-Control", "public, max-age=86400")

	baseURL := s.getBaseURL(r)
	apiVersion := s.config.Server.APIVersion
	if apiVersion == "" {
		apiVersion = "v1"
	}

	// Header
	fmt.Fprintf(w, "# %s\n", s.config.Server.Title)
	if s.config.Server.Branding.Tagline != "" {
		fmt.Fprintf(w, "> %s\n", s.config.Server.Branding.Tagline)
	}
	fmt.Fprintln(w)

	// API section
	fmt.Fprintln(w, "## API")
	fmt.Fprintf(w, "Base URL: %s/api/%s\n", baseURL, apiVersion)
	fmt.Fprintln(w, "Authentication: Bearer token (operator token for admin endpoints)")
	fmt.Fprintf(w, "Rate limit: %d requests/minute\n", s.config.Server.RateLimit.RequestsPerMinute)
	fmt.Fprintln(w)

	// Endpoints section - public API endpoints
	fmt.Fprintln(w, "## Endpoints")
	fmt.Fprintln(w, "- GET /server/healthz - Health check (no auth)")
	fmt.Fprintln(w, "- GET /server/status - Server status (operator auth)")
	fmt.Fprintf(w, "- GET /api/%s/search - Search API (no auth, rate limited)\n", apiVersion)
	fmt.Fprintf(w, "- GET /api/%s/engines - List search engines (no auth)\n", apiVersion)
	fmt.Fprintf(w, "- GET /api/%s/instant - Instant answers (no auth)\n", apiVersion)
	fmt.Fprintf(w, "- GET /api/%s/preferences - User preferences (no auth)\n", apiVersion)
	fmt.Fprintln(w)

	// Capabilities
	fmt.Fprintln(w, "## Capabilities")
	fmt.Fprintln(w, "- Privacy-respecting metasearch across multiple engines")
	fmt.Fprintln(w, "- Instant answers (calculator, unit conversion, definitions)")
	fmt.Fprintln(w, "- Direct answers (wiki:, dns:, http:, tldr:, port:)")
	fmt.Fprintln(w, "- No tracking, no ads, no user accounts required")
	fmt.Fprintln(w)

	// Contact
	fmt.Fprintln(w, "## Contact")
	security := s.config.Server.Web.Security
	contact := security.Contact
	if contact == "" && s.config.Server.Contact.Email != "" {
		contact = s.config.Server.Contact.Email
	}
	if contact == "" {
		fqdn := extractHostFromURL(baseURL)
		contact = "security@" + fqdn
	}
	fmt.Fprintf(w, "Security: %s\n", contact)
	fmt.Fprintf(w, "Source: https://github.com/apimgr/search\n")
}
