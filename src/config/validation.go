package config

import (
	"net"
	"strings"

	"golang.org/x/net/publicsuffix"
)

// devOnlyTLDs are TLDs that are only valid in development mode
var devOnlyTLDs = map[string]bool{
	"localhost":   true,
	"test":        true,
	"example":     true,
	"invalid":     true,
	"local":       true,
	"lan":         true,
	"internal":    true,
	"home":        true,
	"localdomain": true,
	"home.arpa":   true,
	"intranet":    true,
	"corp":        true,
	"private":     true,
}

// IsValidHost validates that a hostname is a valid FQDN.
// Per TEMPLATE.md specification:
// - devMode: if true, allows dev-only TLDs (.local, .test, localhost, etc.)
// - projectName: if non-empty, allows project-specific TLDs (e.g., .jokes) in dev mode
// - IP addresses are ALWAYS rejected
// - Uses golang.org/x/net/publicsuffix for proper TLD validation
func IsValidHost(host string, devMode bool, projectName string) bool {
	lower := strings.ToLower(strings.TrimSpace(host))

	// Reject empty
	if lower == "" {
		return false
	}

	// Remove port if present
	lower = stripPort(lower)

	// Reject IP addresses always
	if net.ParseIP(lower) != nil {
		return false
	}

	// Handle localhost
	if lower == "localhost" {
		return devMode
	}

	// Must contain at least one dot (except localhost handled above)
	if !strings.Contains(lower, ".") {
		return false
	}

	// Overlay network TLDs - valid but app-managed (not set via DOMAIN)
	// These are checked here for internal validation
	if strings.HasSuffix(lower, ".onion") ||
		strings.HasSuffix(lower, ".i2p") ||
		strings.HasSuffix(lower, ".exit") {
		return true
	}

	// Check dynamic project-specific TLD (e.g., app.jokes, dev.search)
	if projectName != "" && strings.HasSuffix(lower, "."+strings.ToLower(projectName)) {
		return devMode // Project TLDs only valid in dev mode
	}

	// Get the public suffix (TLD or eTLD like co.uk)
	suffix, icann := publicsuffix.PublicSuffix(lower)

	// Check if it's a dev-only TLD
	if devOnlyTLDs[suffix] {
		return devMode // Dev TLDs only valid in dev mode
	}

	// In production, require valid ICANN TLD
	if !devMode && !icann {
		return false
	}

	// Verify we have at least eTLD+1 (not just the suffix itself)
	etldPlusOne, err := publicsuffix.EffectiveTLDPlusOne(lower)
	if err != nil {
		return false
	}

	// Host must be at least eTLD+1 (e.g., "domain.co.uk" not just "co.uk")
	return len(etldPlusOne) > 0
}

// IsValidSSLHost validates a host for SSL certificate requests.
// SSL always requires production-valid host (no dev TLDs).
// .onion addresses cannot use Let's Encrypt (Tor provides encryption).
func IsValidSSLHost(host string) bool {
	lower := strings.ToLower(host)

	// .onion addresses cannot use Let's Encrypt (not publicly resolvable)
	// Tor provides end-to-end encryption, so SSL is optional for .onion
	if strings.HasSuffix(lower, ".onion") {
		return false
	}

	// SSL always requires production-valid host (devMode=false)
	return IsValidHost(host, false, "")
}

// stripPort removes the port from a host:port string
func stripPort(host string) string {
	// Handle IPv6 addresses with ports [::1]:8080
	if strings.HasPrefix(host, "[") {
		if idx := strings.LastIndex(host, "]"); idx != -1 {
			if idx+1 < len(host) && host[idx+1] == ':' {
				return host[1:idx]
			}
			return host[1:idx]
		}
	}

	// Handle regular host:port
	if idx := strings.LastIndex(host, ":"); idx != -1 {
		// Make sure it's not an IPv6 address without brackets
		if strings.Count(host, ":") == 1 {
			return host[:idx]
		}
	}

	return host
}

// IsValidEmail validates an email address format
func IsValidEmail(email string) bool {
	if email == "" {
		return false
	}

	// Basic format check
	at := strings.Index(email, "@")
	if at < 1 || at == len(email)-1 {
		return false
	}

	// Check local part
	local := email[:at]
	if len(local) > 64 {
		return false
	}

	// Check domain part - use production mode for email validation
	domain := email[at+1:]
	return IsValidHost(domain, false, "")
}

// IsValidPort validates a port number
func IsValidPort(port int) bool {
	return port > 0 && port <= 65535
}
