package tls

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"golang.org/x/crypto/acme"
	"golang.org/x/crypto/acme/autocert"

	"github.com/apimgr/search/src/config"
)

// Manager handles TLS certificate management
type Manager struct {
	mu          sync.RWMutex
	config      *config.SSLConfig
	certManager *autocert.Manager
	tlsConfig   *tls.Config
	dataDir     string
}

// NewManager creates a new TLS manager
func NewManager(cfg *config.SSLConfig, dataDir string) *Manager {
	m := &Manager{
		config:  cfg,
		dataDir: dataDir,
	}

	if cfg.LetsEncrypt.Enabled {
		m.initLetsEncrypt()
	} else if cfg.CertFile != "" && cfg.KeyFile != "" {
		m.initManualCerts()
	}

	return m
}

// initLetsEncrypt initializes Let's Encrypt certificate manager
func (m *Manager) initLetsEncrypt() {
	cacheDir := filepath.Join(m.dataDir, "certs")
	os.MkdirAll(cacheDir, 0700)

	// Create autocert manager
	m.certManager = &autocert.Manager{
		Prompt:     autocert.AcceptTOS,
		Cache:      autocert.DirCache(cacheDir),
		HostPolicy: autocert.HostWhitelist(m.config.LetsEncrypt.Domains...),
		Email:      m.config.LetsEncrypt.Email,
	}

	// Use staging endpoint for testing
	if m.config.LetsEncrypt.Staging {
		m.certManager.Client = &acme.Client{
			DirectoryURL: "https://acme-staging-v02.api.letsencrypt.org/directory",
		}
	}

	m.tlsConfig = m.certManager.TLSConfig()
	m.tlsConfig.MinVersion = tls.VersionTLS12
	m.tlsConfig.PreferServerCipherSuites = true
	m.tlsConfig.CipherSuites = []uint16{
		tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
		tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
		tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
	}

	log.Printf("[TLS] Let's Encrypt enabled for domains: %v", m.config.LetsEncrypt.Domains)
	if m.config.LetsEncrypt.Staging {
		log.Printf("[TLS] Using Let's Encrypt STAGING environment")
	}
}

// initManualCerts initializes manual certificate configuration
func (m *Manager) initManualCerts() {
	cert, err := tls.LoadX509KeyPair(m.config.CertFile, m.config.KeyFile)
	if err != nil {
		log.Printf("[TLS] Error loading certificates: %v", err)
		return
	}

	m.tlsConfig = &tls.Config{
		Certificates:             []tls.Certificate{cert},
		MinVersion:               tls.VersionTLS12,
		PreferServerCipherSuites: true,
		CipherSuites: []uint16{
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
			tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		},
	}

	log.Printf("[TLS] Loaded certificates from %s and %s", m.config.CertFile, m.config.KeyFile)
}

// GetTLSConfig returns the TLS configuration
func (m *Manager) GetTLSConfig() *tls.Config {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.tlsConfig
}

// GetHTTPSHandler returns an HTTP handler for ACME challenges
// This should be used for port 80 to handle Let's Encrypt HTTP-01 challenges
func (m *Manager) GetHTTPSHandler(fallback http.Handler) http.Handler {
	if m.certManager == nil {
		return fallback
	}
	return m.certManager.HTTPHandler(fallback)
}

// IsEnabled returns whether TLS is enabled
func (m *Manager) IsEnabled() bool {
	return m.config != nil && m.config.Enabled && m.tlsConfig != nil
}

// ReloadCertificates reloads manual certificates
func (m *Manager) ReloadCertificates() error {
	// Let's Encrypt handles its own renewal
	if m.config.LetsEncrypt.Enabled {
		return nil
	}

	if m.config.CertFile == "" || m.config.KeyFile == "" {
		return fmt.Errorf("no certificate files configured")
	}

	cert, err := tls.LoadX509KeyPair(m.config.CertFile, m.config.KeyFile)
	if err != nil {
		return fmt.Errorf("failed to reload certificates: %w", err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.tlsConfig.Certificates = []tls.Certificate{cert}
	log.Printf("[TLS] Certificates reloaded")
	return nil
}

// StartHTTPSRedirect starts an HTTP server that redirects to HTTPS
func StartHTTPSRedirect(addr string, httpsPort int) *http.Server {
	redirect := &http.Server{
		Addr:         addr,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Build HTTPS URL
			target := "https://" + r.Host
			if httpsPort != 443 {
				target = fmt.Sprintf("https://%s:%d", r.Host, httpsPort)
			}
			target += r.URL.RequestURI()
			http.Redirect(w, r, target, http.StatusMovedPermanently)
		}),
	}

	go func() {
		log.Printf("[TLS] HTTP->HTTPS redirect server on %s", addr)
		if err := redirect.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("[TLS] Redirect server error: %v", err)
		}
	}()

	return redirect
}

// CertInfo contains certificate information
type CertInfo struct {
	Subject    string
	Issuer     string
	NotBefore  time.Time
	NotAfter   time.Time
	DNSNames   []string
	IsExpiring bool
}

// GetCertInfo returns information about the current certificate
func (m *Manager) GetCertInfo() (*CertInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.tlsConfig == nil || len(m.tlsConfig.Certificates) == 0 {
		return nil, fmt.Errorf("no certificate loaded")
	}

	cert := m.tlsConfig.Certificates[0]
	if cert.Leaf == nil {
		// Parse the certificate if Leaf is not set
		parsed, err := x509.ParseCertificate(cert.Certificate[0])
		if err != nil {
			return nil, err
		}
		cert.Leaf = parsed
	}

	leaf := cert.Leaf
	return &CertInfo{
		Subject:    leaf.Subject.CommonName,
		Issuer:     leaf.Issuer.CommonName,
		NotBefore:  leaf.NotBefore,
		NotAfter:   leaf.NotAfter,
		DNSNames:   leaf.DNSNames,
		IsExpiring: time.Until(leaf.NotAfter) < 30*24*time.Hour,
	}, nil
}
