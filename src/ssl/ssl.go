package ssl

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
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

// renewalThreshold is how long before expiry an app-managed certificate is
// renewed. AI.md PART 15 (Renewal Rules) specifies 7 days before expiry for
// {config_dir}/ssl/letsencrypt/{fqdn}/.
const renewalThreshold = 7 * 24 * time.Hour

// certOwner identifies who is responsible for renewing the active certificate.
// AI.md PART 15 (Certificate Management Ownership) makes this decisive: the app
// renews only what lives under {config_dir}/ssl/letsencrypt/{fqdn}/.
type certOwner int

const (
	// certOwnerApp means the app manages the certificate and auto-renews it
	// 7 days before expiry. This is the zero value because every certificate
	// the app obtains itself is app-managed.
	certOwnerApp certOwner = iota
	// certOwnerSystem means /etc/letsencrypt/** — certbot renews, the app never does
	certOwnerSystem
	// certOwnerUser means {config_dir}/ssl/local/{fqdn}/ or an explicitly
	// configured cert/key pair — the user renews, the app never does
	certOwnerUser
)

// Manager handles TLS certificate management
type Manager struct {
	mu          sync.RWMutex
	config      *config.SSLConfig
	certManager *autocert.Manager
	tlsConfig   *tls.Config
	dataDir     string
	// Root of the app-managed certificate tree ({config_dir}/ssl)
	sslDir string
	// FQDN the active certificate is tracked under
	fqdn string
	// Normalized ACME challenge type: http-01, tls-alpn-01 or dns-01
	challenge string
	// Renewal ownership of the active certificate
	owner certOwner
	// For DNS-01 challenges
	legoClient *lego.Client
	// For credential decryption
	secretKey string
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

// leafOf returns the parsed leaf certificate of a loaded key pair.
func leafOf(cert tls.Certificate) (*x509.Certificate, error) {
	if cert.Leaf != nil {
		return cert.Leaf, nil
	}
	if len(cert.Certificate) == 0 {
		return nil, fmt.Errorf("certificate contains no chain")
	}
	return x509.ParseCertificate(cert.Certificate[0])
}

// hostMatchesPattern reports whether host is covered by a certificate name,
// honouring a single leading wildcard label.
func hostMatchesPattern(pattern, host string) bool {
	pattern = strings.ToLower(strings.TrimSuffix(pattern, "."))
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	if pattern == "" || host == "" {
		return false
	}
	if pattern == host {
		return true
	}
	if !strings.HasPrefix(pattern, "*.") {
		return false
	}
	suffix := pattern[1:]
	if !strings.HasSuffix(host, suffix) {
		return false
	}
	// A wildcard matches exactly one label
	return !strings.Contains(strings.TrimSuffix(host, suffix), ".")
}

// validateCertificate applies the AI.md PART 15 "Certificate Validation" rules:
// the certificate CN or a SAN must match the configured FQDN, and the
// certificate must not be expired. Readability is already proven by the caller
// having loaded the key pair.
func validateCertificate(cert tls.Certificate, fqdn string) error {
	leaf, err := leafOf(cert)
	if err != nil {
		return err
	}

	now := time.Now()
	if now.Before(leaf.NotBefore) {
		return fmt.Errorf("certificate is not valid until %s", leaf.NotBefore.Format(time.RFC3339))
	}
	if now.After(leaf.NotAfter) {
		return fmt.Errorf("certificate expired on %s", leaf.NotAfter.Format(time.RFC3339))
	}

	if ip := net.ParseIP(fqdn); ip != nil {
		for _, certIP := range leaf.IPAddresses {
			if certIP.Equal(ip) {
				return nil
			}
		}
		return fmt.Errorf("certificate does not cover IP %s", fqdn)
	}

	for _, san := range leaf.DNSNames {
		if hostMatchesPattern(san, fqdn) {
			return nil
		}
	}
	if hostMatchesPattern(leaf.Subject.CommonName, fqdn) {
		return nil
	}

	return fmt.Errorf("certificate does not cover %s", fqdn)
}

// discoverCertificate performs the 4-step certificate auto-detection per AI.md PART 15.
// Search order: system certbot (literal "domain"), system certbot (fqdn), app-managed, user-managed.
// Each candidate must load AND pass validateCertificate before it is accepted.
// Returns the discovered certificate, its renewal ownership, and whether discovery succeeded.
func discoverCertificate(sslDir, fqdn string) (tls.Certificate, certOwner, bool) {
	type candidate struct {
		certPath string
		keyPath  string
		label    string
		owner    certOwner
	}
	candidates := []candidate{
		{
			certPath: "/etc/letsencrypt/live/domain/fullchain.pem",
			keyPath:  "/etc/letsencrypt/live/domain/privkey.pem",
			label:    "system (certbot literal)",
			owner:    certOwnerSystem,
		},
		{
			certPath: filepath.Join("/etc/letsencrypt/live", fqdn, "fullchain.pem"),
			keyPath:  filepath.Join("/etc/letsencrypt/live", fqdn, "privkey.pem"),
			label:    "system (certbot fqdn)",
			owner:    certOwnerSystem,
		},
		{
			certPath: filepath.Join(sslDir, "letsencrypt", fqdn, "fullchain.pem"),
			keyPath:  filepath.Join(sslDir, "letsencrypt", fqdn, "privkey.pem"),
			label:    "app-managed (letsencrypt)",
			owner:    certOwnerApp,
		},
		{
			certPath: filepath.Join(sslDir, "local", fqdn, "cert.pem"),
			keyPath:  filepath.Join(sslDir, "local", fqdn, "key.pem"),
			label:    "user-managed (local)",
			owner:    certOwnerUser,
		},
	}
	for _, c := range candidates {
		cert, err := tls.LoadX509KeyPair(c.certPath, c.keyPath)
		if err != nil {
			continue
		}
		if err := validateCertificate(cert, fqdn); err != nil {
			slog.Warn("Skipping certificate that failed validation", "label", c.label, "fqdn", fqdn, "err", err)
			continue
		}
		slog.Info("Auto-discovered certificate", "label", c.label, "fqdn", fqdn)
		return cert, c.owner, true
	}
	return tls.Certificate{}, certOwnerApp, false
}

// NewManagerWithSecret creates a new TLS manager with a secret key for DNS-01 credential decryption
func NewManagerWithSecret(cfg *config.SSLConfig, dataDir, secretKey string) *Manager {
	return NewManagerWithSecretAndHost(cfg, dataDir, secretKey, "")
}

// NewManagerWithSecretAndHost initializes TLS using the server's configured
// public host for certificate validation and ACME name resolution.
func NewManagerWithSecretAndHost(cfg *config.SSLConfig, dataDir, secretKey, host string) *Manager {
	fqdn := strings.TrimSpace(host)
	if fqdn == "" {
		fqdn = GetFQDN(config.ProjectName)
	}
	m := &Manager{
		config:    cfg,
		dataDir:   dataDir,
		sslDir:    config.GetSSLDir(),
		fqdn:      fqdn,
		secretKey: secretKey,
	}

	// Normalize the challenge type. AI.md PART 15 supports exactly http-01,
	// tls-alpn-01 and dns-01; anything else falls back to the http-01 default.
	m.challenge = strings.ToLower(strings.TrimSpace(cfg.LetsEncrypt.Challenge))
	switch m.challenge {
	case "", "http-01", "tls-alpn-01", "dns-01":
		if m.challenge == "" {
			m.challenge = "http-01"
		}
	default:
		slog.Warn("Unknown ACME challenge type, using http-01", "challenge", cfg.LetsEncrypt.Challenge)
		m.challenge = "http-01"
	}

	// Explicitly configured cert/key paths win over auto-detection, per the
	// PART 5 precedence chain (config file beats defaults). The user owns
	// renewal for a certificate they pointed the server at directly.
	if cfg.CertFile != "" && cfg.KeyFile != "" {
		m.owner = certOwnerUser
		m.initManualCerts()
		if m.GetTLSConfig() != nil {
			return m
		}
	}

	// Run 4-step certificate discovery per AI.md PART 15.
	// This allows the server to find existing certbot or user-placed certs automatically.
	if cert, owner, ok := discoverCertificate(m.sslDir, m.fqdn); ok {
		m.mu.Lock()
		m.owner = owner
		m.tlsConfig = m.createTLSConfig(cert)
		m.mu.Unlock()
		if owner == certOwnerApp && !cfg.LetsEncrypt.Enabled {
			slog.Warn("App-managed certificate found but Let's Encrypt is disabled; automatic renewal is not possible", "fqdn", m.fqdn)
		}
		return m
	}

	if cfg.LetsEncrypt.Enabled {
		// AI.md PART 15: no certificate is ever issued for an overlay address.
		if len(m.certificateDomains()) == 0 {
			slog.Warn("Let's Encrypt enabled but no clearnet domain is configured; skipping certificate request")
			return m
		}

		m.owner = certOwnerApp
		switch m.challenge {
		case "dns-01":
			if err := m.initDNS01(); err != nil {
				slog.Error("DNS-01 initialization failed", "err", err)
			}
		default:
			// http-01 and tls-alpn-01 are both served by autocert
			if err := m.initLetsEncrypt(); err != nil {
				slog.Error("Let's Encrypt initialization failed", "err", err)
			}
		}
	}

	return m
}

// certificateDomains resolves the domain list used for ACME certificate
// requests: configured letsencrypt.domains first, then the DOMAIN env var list
// (AI.md PART 15 "GetAllDomains ... used for CORS configuration and SSL
// certificates"), then the resolved FQDN. Overlay addresses are always removed
// because Let's Encrypt never issues a certificate for .onion/.i2p.
func (m *Manager) certificateDomains() []string {
	domains := m.config.LetsEncrypt.Domains
	if len(domains) == 0 {
		domains = GetAllDomains()
	}
	if len(domains) == 0 && m.fqdn != "" {
		domains = []string{m.fqdn}
	}

	out := make([]string, 0, len(domains))
	for _, domain := range domains {
		domain = strings.TrimSpace(domain)
		if domain == "" {
			continue
		}
		if IsOverlayAddress(domain) {
			slog.Info("Skipping overlay address for certificate issuance", "domain", domain)
			continue
		}
		out = append(out, domain)
	}
	return out
}

// appCertPaths returns the AI.md PART 15 storage location for an app-managed
// certificate: {config_dir}/ssl/letsencrypt/{fqdn}/{fullchain.pem,privkey.pem}.
func (m *Manager) appCertPaths() (string, string) {
	dir := filepath.Join(m.sslDir, "letsencrypt", m.fqdn)
	return filepath.Join(dir, "fullchain.pem"), filepath.Join(dir, "privkey.pem")
}

// initLetsEncrypt initializes Let's Encrypt certificate manager
func (m *Manager) initLetsEncrypt() error {
	cacheDir := filepath.Join(m.dataDir, "certs")
	if err := os.MkdirAll(cacheDir, 0700); err != nil {
		return fmt.Errorf("create certificate cache directory: %w", err)
	}

	domains := m.certificateDomains()

	// Create autocert manager. The cache mirrors every issued certificate into
	// the AI.md PART 15 layout {config_dir}/ssl/letsencrypt/{fqdn}/.
	m.certManager = &autocert.Manager{
		Prompt:     autocert.AcceptTOS,
		Cache:      specMirrorCache{DirCache: autocert.DirCache(cacheDir), sslDir: m.sslDir},
		HostPolicy: autocert.HostWhitelist(domains...),
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
	m.tlsConfig.CipherSuites = []uint16{
		tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
		tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
		tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
	}

	slog.Info("Let's Encrypt enabled", "domains", domains, "challenge", m.challenge)
	if m.config.LetsEncrypt.Staging {
		slog.Info("Using Let's Encrypt STAGING environment")
	}
	return nil
}

// specMirrorCache is an autocert.Cache that keeps autocert's own cache layout
// and additionally mirrors issued certificates into the AI.md PART 15 tree
// {config_dir}/ssl/letsencrypt/{fqdn}/{fullchain.pem,privkey.pem}, which is
// also where startup discovery and renewal look for app-managed certificates.
type specMirrorCache struct {
	autocert.DirCache
	sslDir string
}

// Put stores the entry in autocert's cache and mirrors certificate entries into
// the PART 15 layout.
func (c specMirrorCache) Put(ctx context.Context, key string, data []byte) error {
	if err := c.DirCache.Put(ctx, key, data); err != nil {
		return err
	}
	if err := mirrorCertBundle(c.sslDir, key, data); err != nil {
		return fmt.Errorf("failed to mirror certificate to app-managed directory: %w", err)
	}
	return nil
}

// Get repairs the on-disk PART 15 certificate mirror when a cached bundle is
// found after a prior mirror write failed.
func (c specMirrorCache) Get(ctx context.Context, key string) ([]byte, error) {
	data, err := c.DirCache.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	if err := mirrorCertBundle(c.sslDir, key, data); err != nil {
		return nil, fmt.Errorf("failed to mirror cached certificate to app-managed directory: %w", err)
	}
	return data, nil
}

// mirrorCertBundle splits an autocert PEM bundle (private key first, then the
// certificate chain) into the PART 15 file layout. Cache entries that are not
// certificates — account keys and challenge tokens — are ignored.
func mirrorCertBundle(sslDir, key string, data []byte) error {
	if sslDir == "" {
		return nil
	}
	domain := strings.TrimSuffix(key, "+rsa")
	if strings.Contains(domain, "+") || !strings.Contains(domain, ".") {
		return nil
	}

	var certPEM, keyPEM []byte
	rest := data
	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		switch {
		case strings.HasSuffix(block.Type, "PRIVATE KEY"):
			keyPEM = append(keyPEM, pem.EncodeToMemory(block)...)
		case block.Type == "CERTIFICATE":
			certPEM = append(certPEM, pem.EncodeToMemory(block)...)
		}
	}
	if len(certPEM) == 0 || len(keyPEM) == 0 {
		return nil
	}

	dir := filepath.Join(sslDir, "letsencrypt", domain)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "fullchain.pem"), certPEM, 0644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "privkey.pem"), keyPEM, 0600)
}

// initDNS01 initializes Let's Encrypt with DNS-01 challenge using lego
// Per AI.md PART 17: DNS-01 challenge for wildcard certs and firewalled servers
func (m *Manager) initDNS01() error {
	cacheDir := filepath.Join(m.dataDir, "certs")
	if err := os.MkdirAll(cacheDir, 0700); err != nil {
		return fmt.Errorf("create certificate cache directory: %w", err)
	}

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

	// AI.md PART 15: the server validates credentials on startup and before
	// every certificate request.
	if err := ValidateCredentials(m.config.DNS01.Provider, credentials); err != nil {
		return fmt.Errorf("DNS provider credential validation failed: %w", err)
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

	slog.Info("DNS-01 challenge enabled",
		"domains", m.certificateDomains(),
		"provider", m.config.DNS01.Provider,
		"validated_at", ValidatedAtNow())
	if m.config.LetsEncrypt.Staging {
		slog.Info("Using Let's Encrypt STAGING environment")
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
		return nil, fmt.Errorf("failed to parse ACME account key: %w", err)
	}
	if !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to read ACME account key: %w", err)
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
		slog.Warn("Failed to save account key", "err", err)
	}

	return key, nil
}

// obtainCertificateDNS01 obtains a certificate using DNS-01 challenge
func (m *Manager) obtainCertificateDNS01() error {
	if m.legoClient == nil {
		return fmt.Errorf("lego client not initialized")
	}

	// AI.md PART 15 storage layout: app-managed certificates live in
	// {config_dir}/ssl/letsencrypt/{fqdn}/{fullchain.pem,privkey.pem}
	certPath, keyPath := m.appCertPaths()

	// Try to load existing certificate
	if cert, err := tls.LoadX509KeyPair(certPath, keyPath); err == nil {
		// Check if certificate is still valid
		if len(cert.Certificate) > 0 {
			parsed, err := x509.ParseCertificate(cert.Certificate[0])
			if err == nil && time.Until(parsed.NotAfter) > renewalThreshold {
				// Certificate is valid beyond the renewal threshold
				m.mu.Lock()
				m.tlsConfig = m.createTLSConfig(cert)
				m.mu.Unlock()
				slog.Info("Loaded existing certificate", "expires", parsed.NotAfter)
				return nil
			}
		}
	}

	domains := m.certificateDomains()
	if len(domains) == 0 {
		return fmt.Errorf("no certificate domains configured")
	}

	// AI.md PART 15: credentials are re-validated before every certificate
	// request, not only at startup.
	if m.config.DNS01.CredentialsEncrypted != "" && m.secretKey != "" {
		credentials, err := DecryptCredentials(m.config.DNS01.CredentialsEncrypted, m.secretKey)
		if err != nil {
			return fmt.Errorf("failed to decrypt DNS credentials: %w", err)
		}
		if err := ValidateCredentials(m.config.DNS01.Provider, credentials); err != nil {
			return fmt.Errorf("DNS provider credential validation failed: %w", err)
		}
	}

	// Request new certificate
	request := certificate.ObtainRequest{
		Domains: domains,
		Bundle:  true,
	}

	certificates, err := m.legoClient.Certificate.Obtain(request)
	if err != nil {
		return fmt.Errorf("failed to obtain certificate: %w", err)
	}

	// Save certificate
	if err := os.MkdirAll(filepath.Dir(certPath), 0700); err != nil {
		return fmt.Errorf("failed to create certificate directory: %w", err)
	}
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
	m.owner = certOwnerApp
	m.mu.Unlock()

	slog.Info("Obtained new certificate via DNS-01", "domains", domains, "path", certPath)
	return nil
}

// createTLSConfig creates a TLS config with the given certificate.
// GetCertificate resolves the active certificate at handshake time from the
// live manager state, so a *tls.Config captured by an already-running
// http.Server serves certificates renewed (DNS-01) or reloaded (manual)
// after startup without requiring a process restart. Certificates is still
// populated so GetCertInfo and manual reload continue to work.
func (m *Manager) createTLSConfig(cert tls.Certificate) *tls.Config {
	return &tls.Config{
		Certificates:   []tls.Certificate{cert},
		GetCertificate: m.getCertificate,
		MinVersion:     tls.VersionTLS12,
		CurvePreferences: []tls.CurveID{
			tls.X25519,
			tls.CurveP256,
			tls.CurveP384,
		},
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

// RenewCertificateDNS01 is the scheduler entry point for the daily ssl_renewal
// task. The name is retained for the scheduler wiring; the work is delegated to
// the ownership-aware RenewIfNeeded.
func (m *Manager) RenewCertificateDNS01(ctx context.Context) error {
	return m.RenewIfNeeded(ctx)
}

// RenewIfNeeded renews the active certificate when it is app-managed and inside
// the renewal window. AI.md PART 15 (Certificate Management Ownership) makes
// this conditional: /etc/letsencrypt/** is renewed by certbot and
// {config_dir}/ssl/local/{fqdn}/ is renewed by the user — the app renews
// neither. Certificates monitored but not renewable are logged when expiring so
// the operator still gets a warning.
func (m *Manager) RenewIfNeeded(ctx context.Context) error {
	if m.config == nil || !m.config.Enabled {
		return nil
	}

	info, err := m.GetCertInfo()
	if err != nil {
		// No usable certificate on disk. Only an app-managed setup may obtain
		// one; anything else is the owner's responsibility.
		if m.owner == certOwnerApp && m.legoClient != nil {
			return m.obtainCertificateDNS01()
		}
		return nil
	}

	if !info.IsExpiring {
		return nil
	}

	switch m.owner {
	case certOwnerSystem:
		slog.Warn("System-managed certificate is expiring; renewal is handled by certbot",
			"expires", info.NotAfter)
		return nil
	case certOwnerUser:
		slog.Warn("User-managed certificate is expiring; the operator must replace it",
			"expires", info.NotAfter)
		return nil
	}

	if m.legoClient != nil {
		slog.Info("Certificate expiring soon, renewing via DNS-01", "expires", info.NotAfter)
		return m.obtainCertificateDNS01()
	}

	if m.certManager != nil {
		// autocert renews on demand during the handshake for HTTP-01 and
		// TLS-ALPN-01; force a refresh now so renewal is not deferred to the
		// first request after expiry.
		for _, domain := range m.certificateDomains() {
			if _, err := m.certManager.GetCertificate(&tls.ClientHelloInfo{ServerName: domain}); err != nil {
				slog.Warn("Certificate renewal failed", "domain", domain, "err", err)
			}
		}
		return nil
	}

	slog.Warn("Certificate is expiring but no ACME client is configured to renew it",
		"expires", info.NotAfter)
	return nil
}

// initManualCerts initializes manual certificate configuration
func (m *Manager) initManualCerts() {
	cert, err := tls.LoadX509KeyPair(m.config.CertFile, m.config.KeyFile)
	if err != nil {
		slog.Error("Error loading certificates", "err", err)
		return
	}
	if err := validateCertificate(cert, m.fqdn); err != nil {
		slog.Error("Configured certificate failed validation", "fqdn", m.fqdn, "err", err)
		return
	}

	m.tlsConfig = m.createTLSConfig(cert)

	slog.Info("Loaded certificates", "cert", m.config.CertFile, "key", m.config.KeyFile)
}

// GetTLSConfig returns the TLS configuration
func (m *Manager) GetTLSConfig() *tls.Config {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.tlsConfig
}

// getCertificate resolves the active certificate at TLS handshake time from
// the live manager state. It is wired into every *tls.Config via
// createTLSConfig so an already-running http.Server picks up certificates
// renewed (DNS-01) or reloaded (manual) after startup without a restart.
func (m *Manager) getCertificate(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.tlsConfig == nil || len(m.tlsConfig.Certificates) == 0 {
		return nil, fmt.Errorf("no certificate available")
	}
	cert := m.tlsConfig.Certificates[0]
	return &cert, nil
}

// GetHTTPChallengeHandler answers HTTP-01 challenges on the cleartext listener.
func (m *Manager) GetHTTPChallengeHandler(fallback http.Handler) http.Handler {
	if m.certManager == nil || m.challenge != "http-01" {
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
	if err := validateCertificate(cert, m.fqdn); err != nil {
		return fmt.Errorf("failed to reload certificates: %w", err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.tlsConfig.Certificates = []tls.Certificate{cert}
	slog.Info("Certificates reloaded")
	return nil
}

// StartHTTPSRedirect starts an HTTP server that redirects to HTTPS.
func StartHTTPSRedirect(addr string, httpsPort int) *http.Server {
	return StartHTTPSRedirectWithHandler(addr, httpsPort, nil)
}

// StartHTTPSRedirectWithHandler starts the HTTP listener using fallback when
// provided, allowing ACME HTTP-01 handling before ordinary redirect requests.
func StartHTTPSRedirectWithHandler(addr string, httpsPort int, fallback http.Handler) *http.Server {
	redirect := newHTTPSRedirectServer(addr, httpsPort, fallback)
	go func() {
		slog.Info("HTTP->HTTPS redirect server started", "addr", addr)
		if err := redirect.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("Redirect server error", "err", err)
		}
	}()
	return redirect
}

// StartHTTPSRedirectListener serves the AutoTLS redirect handler on a listener
// that may have been bound before the process dropped privileges.
func StartHTTPSRedirectListener(listener net.Listener, httpsPort int) *http.Server {
	return StartHTTPSRedirectListenerWithHandler(listener, httpsPort, nil)
}

// StartHTTPSRedirectListenerWithHandler serves redirects and optional ACME
// HTTP-01 challenges on an already-bound HTTP listener.
func StartHTTPSRedirectListenerWithHandler(listener net.Listener, httpsPort int, fallback http.Handler) *http.Server {
	redirect := newHTTPSRedirectServer(listener.Addr().String(), httpsPort, fallback)
	go func() {
		slog.Info("HTTP->HTTPS redirect server started", "addr", redirect.Addr)
		if err := redirect.Serve(listener); err != nil && err != http.ErrServerClosed {
			slog.Error("Redirect server error", "err", err)
		}
	}()
	return redirect
}

func newHTTPSRedirectServer(addr string, httpsPort int, fallback http.Handler) *http.Server {
	if fallback == nil {
		fallback = HTTPSRedirectHandler(httpsPort)
	}
	return &http.Server{
		Addr:         addr,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
		Handler:      fallback,
	}
}

// HTTPSRedirectHandler redirects ordinary HTTP requests while remaining
// composable with the ACME HTTP-01 challenge handler.
func HTTPSRedirectHandler(httpsPort int) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Strip any existing port from the Host header before building the
		// redirect URL — r.Host may include the HTTP port, producing an invalid URL.
		host := r.Host
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		var target string
		if httpsPort == 443 {
			target = "https://" + host
		} else {
			target = fmt.Sprintf("https://%s:%d", host, httpsPort)
		}
		target += r.URL.RequestURI()
		http.Redirect(w, r, target, http.StatusMovedPermanently)
	})
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
		IsExpiring: time.Until(leaf.NotAfter) < renewalThreshold,
	}, nil
}
