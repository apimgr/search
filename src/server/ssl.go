package server

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/apimgr/search/src/config"
)

// SSLManager handles SSL/TLS configuration
type SSLManager struct {
	config *config.Config
}

// NewSSLManager creates a new SSL manager
func NewSSLManager(cfg *config.Config) *SSLManager {
	return &SSLManager{config: cfg}
}

// GetTLSConfig returns a TLS configuration based on settings
func (m *SSLManager) GetTLSConfig() (*tls.Config, error) {
	ssl := m.config.Server.SSL

	if !ssl.Enabled {
		return nil, nil
	}

	// Create TLS config with secure defaults
	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
		MaxVersion: tls.VersionTLS13,
		CipherSuites: []uint16{
			// TLS 1.3 ciphers
			tls.TLS_AES_128_GCM_SHA256,
			tls.TLS_AES_256_GCM_SHA384,
			tls.TLS_CHACHA20_POLY1305_SHA256,
			// TLS 1.2 ciphers
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
			tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
		},
		CurvePreferences: []tls.CurveID{
			tls.X25519,
			tls.CurveP256,
			tls.CurveP384,
		},
		PreferServerCipherSuites: true,
	}

	// Load certificate if provided
	if ssl.CertFile != "" && ssl.KeyFile != "" {
		cert, err := tls.LoadX509KeyPair(ssl.CertFile, ssl.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load TLS certificate: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	return tlsConfig, nil
}

// certOwnership describes who is responsible for certificate renewal
type certOwnership int

const (
	// certOwnerSystem means the system (certbot) manages renewal; the app must not renew
	certOwnerSystem certOwnership = iota
	// certOwnerApp means the app auto-renews 7 days before expiry via the scheduler
	certOwnerApp
	// certOwnerUser means the user manages renewal manually; the app must not renew
	certOwnerUser
)

// certLookupResult holds a discovered certificate pair together with renewal ownership
type certLookupResult struct {
	certFile  string
	keyFile   string
	ownership certOwnership
}

// getFQDN returns the best FQDN for certificate lookup.
// Priority: DOMAIN env var → os.Hostname() → "localhost".
func (m *SSLManager) getFQDN() string {
	if domain := config.GetDomain(); domain != "" {
		return domain
	}
	hostname, err := os.Hostname()
	if err == nil && hostname != "" {
		return hostname
	}
	return "localhost"
}

// certFileMatchesFQDN parses the PEM certificate at certFile and returns true when it
// covers fqdn (via SAN DNS names or CN) and has not yet expired.
func certFileMatchesFQDN(certFile, fqdn string) bool {
	data, err := os.ReadFile(certFile)
	if err != nil {
		return false
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return false
	}
	leaf, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return false
	}
	if time.Now().After(leaf.NotAfter) {
		return false
	}
	for _, san := range leaf.DNSNames {
		if san == fqdn {
			return true
		}
	}
	return leaf.Subject.CommonName == fqdn
}

// certPairReadable returns true when both files exist and are readable
func certPairReadable(certFile, keyFile string) bool {
	if _, err := os.Stat(certFile); err != nil {
		return false
	}
	if _, err := os.Stat(keyFile); err != nil {
		return false
	}
	return true
}

// findCertificatePaths walks the 4 spec-mandated directories in priority order and
// returns the first valid certificate pair together with renewal ownership.
// Per AI.md PART 15:
//
//	1. /etc/letsencrypt/live/domain/          → system-managed (certbot)
//	2. /etc/letsencrypt/live/{fqdn}/          → system-managed (certbot)
//	3. {config_dir}/ssl/letsencrypt/{fqdn}/   → app-managed (auto-renew 7d before expiry)
//	4. {config_dir}/ssl/local/{fqdn}/         → user-managed (no auto-renewal)
func (m *SSLManager) findCertificatePaths() *certLookupResult {
	fqdn := m.getFQDN()
	configDir := config.GetConfigDir()

	// Candidate directories in priority order; letsencrypt layout uses fullchain.pem + privkey.pem,
	// local layout uses cert.pem + key.pem.
	type candidate struct {
		dir       string
		certName  string
		keyName   string
		ownership certOwnership
	}

	candidates := []candidate{
		{
			dir:       "/etc/letsencrypt/live/domain",
			certName:  "fullchain.pem",
			keyName:   "privkey.pem",
			ownership: certOwnerSystem,
		},
		{
			dir:       filepath.Join("/etc/letsencrypt/live", fqdn),
			certName:  "fullchain.pem",
			keyName:   "privkey.pem",
			ownership: certOwnerSystem,
		},
		{
			dir:       filepath.Join(configDir, "ssl", "letsencrypt", fqdn),
			certName:  "fullchain.pem",
			keyName:   "privkey.pem",
			ownership: certOwnerApp,
		},
		{
			dir:      filepath.Join(configDir, "ssl", "local", fqdn),
			certName: "cert.pem",
			keyName:  "key.pem",
			ownership: certOwnerUser,
		},
	}

	for _, c := range candidates {
		certFile := filepath.Join(c.dir, c.certName)
		keyFile := filepath.Join(c.dir, c.keyName)
		if !certPairReadable(certFile, keyFile) {
			continue
		}
		if !certFileMatchesFQDN(certFile, fqdn) {
			continue
		}
		return &certLookupResult{
			certFile:  certFile,
			keyFile:   keyFile,
			ownership: c.ownership,
		}
	}

	return nil
}

// GetCertificatePaths returns the paths for SSL certificates following the
// 4-step startup lookup defined in AI.md PART 15.
//
// When ssl.cert_file and ssl.key_file are explicitly configured, those values
// are returned immediately. Otherwise the function walks the spec-ordered
// directories and returns the first valid pair. Falls back to the default
// {config_dir}/ssl/local/{fqdn}/ paths when nothing is found so that callers
// always receive non-empty strings.
func (m *SSLManager) GetCertificatePaths() (certFile, keyFile string) {
	ssl := m.config.Server.SSL

	// Explicit operator override takes highest priority
	if ssl.CertFile != "" && ssl.KeyFile != "" {
		return ssl.CertFile, ssl.KeyFile
	}

	// Walk the 4-step spec order
	if result := m.findCertificatePaths(); result != nil {
		return result.certFile, result.keyFile
	}

	// Nothing found — return the default app-managed letsencrypt path so that
	// callers always have a non-empty target to write new certificates into.
	fqdn := m.getFQDN()
	configDir := config.GetConfigDir()
	defaultDir := filepath.Join(configDir, "ssl", "letsencrypt", fqdn)
	return filepath.Join(defaultDir, "fullchain.pem"), filepath.Join(defaultDir, "privkey.pem")
}

// HasValidCertificate checks if valid SSL certificates exist
func (m *SSLManager) HasValidCertificate() bool {
	certFile, keyFile := m.GetCertificatePaths()

	// Check if files exist
	if _, err := os.Stat(certFile); os.IsNotExist(err) {
		return false
	}
	if _, err := os.Stat(keyFile); os.IsNotExist(err) {
		return false
	}

	// Try to load the certificate
	_, err := tls.LoadX509KeyPair(certFile, keyFile)
	return err == nil
}

// LogSSLStatus logs the current SSL status
func (m *SSLManager) LogSSLStatus() {
	ssl := m.config.Server.SSL

	if !ssl.Enabled {
		slog.Info("SSL/TLS disabled")
		return
	}

	if ssl.LetsEncrypt.Enabled {
		slog.Info("SSL/TLS enabled via Let's Encrypt with auto-renewal", "domains", ssl.LetsEncrypt.Domains)
	} else if m.HasValidCertificate() {
		certFile, _ := m.GetCertificatePaths()
		slog.Info("SSL/TLS enabled", "certificate", certFile)
	} else {
		slog.Info("SSL/TLS enabled but no certificate found")
	}
}
