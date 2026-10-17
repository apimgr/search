package server

import (
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/apimgr/search/src/common/i18n"
	"github.com/apimgr/search/src/config"
	"github.com/apimgr/search/src/security"
	"github.com/apimgr/search/src/swagger"
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
		http.Error(w, i18n.RequestString(r, "errors.method_not_allowed"), http.StatusMethodNotAllowed)
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
	writeResponseText(w, "# robots.txt - Search Engine Crawling Rules\n")
	writeResponseFormatted(w, "# %s\n", s.config.Server.Title)
	writeResponseText(w, "\n")

	writeResponseText(w, "User-agent: *\n")

	// Allow paths (default: /, /api)
	allowPaths := web.Robots.Allow
	if len(allowPaths) == 0 {
		allowPaths = []string{"/", "/api"}
	}
	for _, path := range allowPaths {
		writeResponseFormatted(w, "Allow: %s\n", path)
	}

	// Deny paths (no defaults — there is no admin panel in this project).
	for _, path := range web.Robots.Deny {
		writeResponseFormatted(w, "Disallow: %s\n", path)
	}

	// Add sitemap URL per AI.md spec
	writeResponseText(w, "\n")
	baseURL := s.getBaseURL(r)
	writeResponseFormatted(w, "Sitemap: %s/sitemap.xml\n", baseURL)
}

// handleSecurityTxtEnhanced serves security.txt per RFC 9116 and AI.md PART 11.
// Required fields: Contact, Expires
// Optional fields: Encryption, Preferred-Languages, Canonical
// Per AI.md PART 11 "Security Reports — Coordinated Disclosure Pipeline", three
// Contact: lines are emitted in RFC 9116 preference order: the project's
// primary vulnerability-reporting channel (report_url), the rotating
// {security_id} contact-form URL, then the mailto: fallback.
func (s *Server) handleSecurityTxtEnhanced(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	// Cache for 1 day per AI.md spec
	w.Header().Set("Cache-Control", "public, max-age=86400")

	webSecurity := s.config.Server.Web.Security
	baseURL := s.getBaseURL(r)

	// Contact 1 (REQUIRED per RFC 9116): primary vulnerability-reporting channel
	if webSecurity.ReportURL != "" {
		writeResponseFormatted(w, "Contact: %s\n", webSecurity.ReportURL)
	}

	// Contact 2: rotating {security_id} contact-form URL per AI.md PART 11
	securityID := security.GenerateSecurityID(s.config.Server.Security.InstallationSecret, time.Now().Unix())
	writeResponseFormatted(w, "Contact: %s/server/contact?security_id=%s\n", baseURL, securityID)

	// Contact 3: mailto: fallback
	// Uses security.contact then general.contact per AI.md PART 12 fallback chain
	contact := webSecurity.Contact
	if contact == "" && s.config.Server.Contact.Security.Email != "" {
		contact = s.config.Server.Contact.Security.Email
	}
	if contact == "" && s.config.Server.Contact.General.Email != "" {
		contact = s.config.Server.Contact.General.Email
	}
	if contact == "" {
		// Fallback to fqdn-based security email
		fqdn := extractHostFromURL(baseURL)
		contact = "security@" + fqdn
	}
	// Ensure mailto: prefix for email addresses
	if strings.Contains(contact, "@") && !strings.HasPrefix(contact, "mailto:") && !strings.HasPrefix(contact, "https://") && !strings.HasPrefix(contact, "tel:") {
		contact = "mailto:" + contact
	}
	writeResponseFormatted(w, "Contact: %s\n", contact)

	// Expires (REQUIRED per RFC 9116)
	// Must be in ISO 8601 format (YYYY-MM-DDTHH:MM:SSZ)
	expires := webSecurity.Expires
	if expires == "" {
		// Default: 1 year from now (auto-renewed yearly per AI.md)
		expiryTime := time.Now().AddDate(1, 0, 0)
		expires = expiryTime.UTC().Format(time.RFC3339)
	}
	writeResponseFormatted(w, "Expires: %s\n", expires)

	// Encryption (OPTIONAL per RFC 9116): only when a keypair exists and
	// publishing is enabled, per AI.md PART 11 GPG Keypair Management.
	if webSecurity.PublishPGPKey {
		if _, err := security.LoadPublicKey(config.GetConfigDir()); err == nil {
			writeResponseFormatted(w, "Encryption: %s/.well-known/pgp-key.asc\n", baseURL)
		}
	}

	// Preferred-Languages (OPTIONAL per RFC 9116)
	// Auto-generated from i18n config per AI.md
	languages := s.getPreferredLanguages()
	if len(languages) > 0 {
		writeResponseFormatted(w, "Preferred-Languages: %s\n", strings.Join(languages, ", "))
	}

	// Canonical (OPTIONAL per RFC 9116)
	// Canonical URL of the security.txt file
	writeResponseFormatted(w, "Canonical: %s/.well-known/security.txt\n", baseURL)
}

// handlePGPKeyAsc serves the project's PGP public key per AI.md PART 11
// Public Pages table: "/.well-known/pgp-key.asc" — 404 if no keypair has been
// generated yet, or if publishing is disabled via web.security.publish_pgp_key.
func (s *Server) handlePGPKeyAsc(w http.ResponseWriter, r *http.Request) {
	if !s.config.Server.Web.Security.PublishPGPKey {
		http.NotFound(w, r)
		return
	}

	pubArmor, err := security.LoadPublicKey(config.GetConfigDir())
	if err != nil {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "application/pgp-keys")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	writeResponseText(w, pubArmor)
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
	writeResponseFormatted(w, "# %s\n", s.config.Server.Title)
	if s.config.Server.Branding.Tagline != "" {
		writeResponseFormatted(w, "> %s\n", s.config.Server.Branding.Tagline)
	}
	writeResponseText(w, "\n")

	// API section
	writeResponseFormatted(w, "%s\n", "## API")
	writeResponseFormatted(w, "Base URL: %s/api/%s\n", baseURL, apiVersion)
	writeResponseFormatted(w, "%s\n", "Authentication: Bearer token (owner token, linked token, or operator token)")
	writeResponseFormatted(w, "Rate limit: %d requests per %ds (read), %d requests per %ds (write)\n",
		s.config.Server.RateLimit.Read.Requests, s.config.Server.RateLimit.Read.Window,
		s.config.Server.RateLimit.Write.Requests, s.config.Server.RateLimit.Write.Window)
	writeResponseFormatted(w, "MCP: %s/mcp (Streamable HTTP; same Bearer token as the API)\n", baseURL)
	writeResponseText(w, "\n")

	// Endpoints section - public API endpoints
	writeResponseFormatted(w, "%s\n", "## Endpoints")
	spec := swagger.GenerateSpec(s.config, baseURL)
	paths := make([]string, 0, len(spec.Paths))
	for path := range spec.Paths {
		if strings.HasPrefix(path, "/api/"+apiVersion+"/") {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	for _, path := range paths {
		item := spec.Paths[path]
		for _, operation := range []struct {
			method string
			value  *swagger.Operation
		}{{"GET", item.Get}, {"POST", item.Post}, {"PUT", item.Put}, {"PATCH", item.Patch}, {"DELETE", item.Delete}} {
			if operation.value == nil {
				continue
			}
			summary := operation.value.Summary
			if len(operation.value.Security) > 0 {
				summary += " (Bearer token required)"
			}
			writeResponseFormatted(w, "- %s %s - %s\n", operation.method, strings.TrimPrefix(path, "/api/"+apiVersion), summary)
		}
	}
	writeResponseText(w, "\n")

	// Capabilities
	writeResponseFormatted(w, "%s\n", "## Capabilities")
	writeResponseFormatted(w, "%s\n", "- Privacy-respecting metasearch across multiple engines")
	writeResponseFormatted(w, "%s\n", "- Instant answers (calculator, unit conversion, definitions)")
	writeResponseFormatted(w, "%s\n", "- Direct answers (wiki:, dns:, http:, tldr:, port:)")
	writeResponseFormatted(w, "%s\n", "- No tracking, no ads, no user accounts required")
	writeResponseText(w, "\n")

	// Contact
	writeResponseFormatted(w, "%s\n", "## Contact")
	security := s.config.Server.Web.Security
	contact := security.Contact
	if contact == "" && s.config.Server.Contact.Security.Email != "" {
		contact = s.config.Server.Contact.Security.Email
	}
	if contact == "" && s.config.Server.Contact.General.Email != "" {
		contact = s.config.Server.Contact.General.Email
	}
	if contact == "" {
		fqdn := extractHostFromURL(baseURL)
		contact = "security@" + fqdn
	}
	writeResponseFormatted(w, "Security: %s\n", contact)
	writeResponseFormatted(w, "Source: https://github.com/apimgr/search\n")
}
