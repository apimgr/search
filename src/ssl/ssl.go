package ssl

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/go-acme/lego/v4/certcrypto"
	"github.com/go-acme/lego/v4/certificate"
	"github.com/go-acme/lego/v4/lego"
	"github.com/go-acme/lego/v4/registration"
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
	legoClient  *lego.Client // For DNS-01 challenges
	secretKey   string       // For credential decryption
}

// legoUser implements registration.User for lego ACME client
type legoUser struct {
	email        string
	registration *registration.Resource
	key          crypto.PrivateKey
}

func (u *legoUser) GetEmail() string                        { return u.email }
func (u *legoUser) GetRegistration() *registration.Resource { return u.registration }
func (u *legoUser) GetPrivateKey() crypto.PrivateKey        { return u.key }

// NewManager creates a new TLS manager
func NewManager(cfg *config.SSLConfig, dataDir string) *Manager {
	return NewManagerWithSecret(cfg, dataDir, "")
}

// NewManagerWithSecret creates a new TLS manager with a secret key for DNS-01 credential decryption
func NewManagerWithSecret(cfg *config.SSLConfig, dataDir, secretKey string) *Manager {
	m := &Manager{
		config:    cfg,
		dataDir:   dataDir,
		secretKey: secretKey,
	}

	if cfg.LetsEncrypt.Enabled {
		// Choose challenge type based on config
		challenge := cfg.LetsEncrypt.Challenge
		if challenge == "" {
			challenge = "http-01" // default
		}

		switch challenge {
		case "dns-01":
			if err := m.initDNS01(); err != nil {
				log.Printf("[TLS] DNS-01 initialization failed: %v, falling back to HTTP-01", err)
				m.initLetsEncrypt()
			}
		default:
			// http-01 or tls-alpn-01 use autocert
			m.initLetsEncrypt()
		}
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

// initDNS01 initializes Let's Encrypt with DNS-01 challenge using lego
// Per AI.md PART 17: DNS-01 challenge for wildcard certs and firewalled servers
func (m *Manager) initDNS01() error {
	cacheDir := filepath.Join(m.dataDir, "certs")
	os.MkdirAll(cacheDir, 0700)

	// Validate DNS-01 configuration
	if m.config.DNS01.Provider == "" {
		return fmt.Errorf("dns01.provider is required for DNS-01 challenge")
	}
	if m.config.DNS01.CredentialsEncrypted == "" {
		return fmt.Errorf("dns01.credentials_encrypted is required for DNS-01 challenge")
	}
	if m.secretKey == "" {
		return fmt.Errorf("secret key is required for DNS-01 credential decryption")
	}

	// Decrypt credentials
	credentials, err := DecryptCredentials(m.config.DNS01.CredentialsEncrypted, m.secretKey)
	if err != nil {
		return fmt.Errorf("failed to decrypt DNS credentials: %w", err)
	}

	// Create DNS provider
	dnsProvider, err := CreateDNSProvider(m.config.DNS01.Provider, credentials)
	if err != nil {
		return fmt.Errorf("failed to create DNS provider: %w", err)
	}

	// Generate or load ACME account key
	privateKey, err := m.loadOrCreateAccountKey()
	if err != nil {
		return fmt.Errorf("failed to load/create account key: %w", err)
	}

	// Create lego user
	user := &legoUser{
		email: m.config.LetsEncrypt.Email,
		key:   privateKey,
	}

	// Configure lego client
	legoConfig := lego.NewConfig(user)
	legoConfig.Certificate.KeyType = certcrypto.EC256

	if m.config.LetsEncrypt.Staging {
		legoConfig.CADirURL = "https://acme-staging-v02.api.letsencrypt.org/directory"
	}

	// Create lego client
	client, err := lego.NewClient(legoConfig)
	if err != nil {
		return fmt.Errorf("failed to create ACME client: %w", err)
	}

	// Set DNS-01 challenge provider
	if err := client.Challenge.SetDNS01Provider(dnsProvider); err != nil {
		return fmt.Errorf("failed to set DNS-01 provider: %w", err)
	}

	// Register with ACME server if needed
	if user.registration == nil {
		reg, err := client.Registration.Register(registration.RegisterOptions{TermsOfServiceAgreed: true})
		if err != nil {
			return fmt.Errorf("failed to register with ACME: %w", err)
		}
		user.registration = reg
	}

	m.legoClient = client

	// Try to obtain certificate
	if err := m.obtainCertificateDNS01(); err != nil {
		return fmt.Errorf("failed to obtain certificate: %w", err)
	}

	log.Printf("[TLS] DNS-01 challenge enabled for domains: %v (provider: %s)",
		m.config.LetsEncrypt.Domains, m.config.DNS01.Provider)
	if m.config.LetsEncrypt.Staging {
		log.Printf("[TLS] Using Let's Encrypt STAGING environment")
	}

	return nil
}

// loadOrCreateAccountKey loads or creates an ACME account private key
func (m *Manager) loadOrCreateAccountKey() (crypto.PrivateKey, error) {
	keyPath := filepath.Join(m.dataDir, "certs", "account.key")

	// Try to load existing key
	keyData, err := os.ReadFile(keyPath)
	if err == nil {
		key, err := x509.ParseECPrivateKey(keyData)
		if err == nil {
			return key, nil
		}
	}

	// Generate new key
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}

	// Save key
	keyBytes, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(keyPath, keyBytes, 0600); err != nil {
		log.Printf("[TLS] Warning: failed to save account key: %v", err)
	}

	return key, nil
}

// obtainCertificateDNS01 obtains a certificate using DNS-01 challenge
func (m *Manager) obtainCertificateDNS01() error {
	if m.legoClient == nil {
		return fmt.Errorf("lego client not initialized")
	}

	// Check for existing certificate
	certPath := filepath.Join(m.dataDir, "certs", "certificate.pem")
	keyPath := filepath.Join(m.dataDir, "certs", "private.key")

	// Try to load existing certificate
	if cert, err := tls.LoadX509KeyPair(certPath, keyPath); err == nil {
		// Check if certificate is still valid
		if len(cert.Certificate) > 0 {
			parsed, err := x509.ParseCertificate(cert.Certificate[0])
			if err == nil && time.Until(parsed.NotAfter) > 30*24*time.Hour {
				// Certificate is valid for more than 30 days
				m.mu.Lock()
				m.tlsConfig = m.createTLSConfig(cert)
				m.mu.Unlock()
				log.Printf("[TLS] Loaded existing certificate (expires: %v)", parsed.NotAfter)
				return nil
			}
		}
	}

	// Request new certificate
	request := certificate.ObtainRequest{
		Domains: m.config.LetsEncrypt.Domains,
		Bundle:  true,
	}

	certificates, err := m.legoClient.Certificate.Obtain(request)
	if err != nil {
		return fmt.Errorf("failed to obtain certificate: %w", err)
	}

	// Save certificate
	if err := os.WriteFile(certPath, certificates.Certificate, 0644); err != nil {
		return fmt.Errorf("failed to save certificate: %w", err)
	}
	if err := os.WriteFile(keyPath, certificates.PrivateKey, 0600); err != nil {
		return fmt.Errorf("failed to save private key: %w", err)
	}

	// Load and configure certificate
	cert, err := tls.X509KeyPair(certificates.Certificate, certificates.PrivateKey)
	if err != nil {
		return fmt.Errorf("failed to parse certificate: %w", err)
	}

	m.mu.Lock()
	m.tlsConfig = m.createTLSConfig(cert)
	m.mu.Unlock()

	log.Printf("[TLS] Obtained new certificate via DNS-01 for: %v", m.config.LetsEncrypt.Domains)
	return nil
}

// createTLSConfig creates a TLS config with the given certificate
func (m *Manager) createTLSConfig(cert tls.Certificate) *tls.Config {
	return &tls.Config{
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
}

// RenewCertificateDNS01 renews the certificate using DNS-01 challenge
// Called by scheduler for ssl_renewal task
func (m *Manager) RenewCertificateDNS01(ctx context.Context) error {
	if m.legoClient == nil {
		return nil // Not using DNS-01
	}

	// Check if renewal is needed
	info, err := m.GetCertInfo()
	if err != nil {
		return m.obtainCertificateDNS01()
	}

	// Renew if expiring within 30 days
	if !info.IsExpiring {
		return nil
	}

	log.Printf("[TLS] Certificate expiring soon, renewing via DNS-01...")
	return m.obtainCertificateDNS01()
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
