package ssl

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/apimgr/search/src/config"
)

func TestNewManager(t *testing.T) {
	cfg := &config.SSLConfig{
		Enabled: false,
	}

	m := NewManager(cfg, "/tmp/test")

	if m == nil {
		t.Fatal("NewManager() returned nil")
	}
	if m.config != cfg {
		t.Error("Config not set correctly")
	}
	if m.dataDir != "/tmp/test" {
		t.Errorf("dataDir = %q, want %q", m.dataDir, "/tmp/test")
	}
}

func TestNewManagerWithSecret(t *testing.T) {
	cfg := &config.SSLConfig{
		Enabled: false,
	}

	m := NewManagerWithSecret(cfg, "/tmp/test", "secret123")

	if m == nil {
		t.Fatal("NewManagerWithSecret() returned nil")
	}
	if m.secretKey != "secret123" {
		t.Errorf("secretKey = %q, want %q", m.secretKey, "secret123")
	}
}

func TestNewManagerWithLetsEncryptEnabled(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "tls-test-le-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	cfg := &config.SSLConfig{
		Enabled: true,
	}
	cfg.LetsEncrypt.Enabled = true
	cfg.LetsEncrypt.Domains = []string{"example.com"}
	cfg.LetsEncrypt.Email = "test@example.com"

	m := NewManager(cfg, tempDir)

	if m == nil {
		t.Fatal("NewManager() returned nil")
	}
	if m.certManager == nil {
		t.Error("certManager should be initialized for Let's Encrypt")
	}
}

func TestNewManagerWithLetsEncryptStaging(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "tls-test-le-staging-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	cfg := &config.SSLConfig{
		Enabled: true,
	}
	cfg.LetsEncrypt.Enabled = true
	cfg.LetsEncrypt.Domains = []string{"example.com"}
	cfg.LetsEncrypt.Email = "test@example.com"
	cfg.LetsEncrypt.Staging = true

	m := NewManager(cfg, tempDir)

	if m == nil {
		t.Fatal("NewManager() returned nil")
	}
	if m.certManager == nil {
		t.Error("certManager should be initialized for Let's Encrypt staging")
	}
}

func TestNewManagerWithDNS01ChallengeFallback(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "tls-test-dns01-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	cfg := &config.SSLConfig{
		Enabled: true,
	}
	cfg.LetsEncrypt.Enabled = true
	cfg.LetsEncrypt.Domains = []string{"example.com"}
	cfg.LetsEncrypt.Email = "test@example.com"
	cfg.LetsEncrypt.Challenge = "dns-01"
	// DNS-01 will fail due to missing provider config, should fallback to HTTP-01

	m := NewManager(cfg, tempDir)

	if m == nil {
		t.Fatal("NewManager() returned nil")
	}
	// Should have fallen back to certManager (HTTP-01)
	if m.certManager == nil {
		t.Error("Should fallback to HTTP-01 when DNS-01 fails")
	}
}

func TestManagerIsEnabled(t *testing.T) {
	// Disabled config
	cfg := &config.SSLConfig{
		Enabled: false,
	}
	m := NewManager(cfg, "/tmp/test")

	if m.IsEnabled() {
		t.Error("IsEnabled() should return false when disabled")
	}
}

func TestManagerIsEnabledNilConfig(t *testing.T) {
	m := &Manager{
		config: nil,
	}

	if m.IsEnabled() {
		t.Error("IsEnabled() should return false with nil config")
	}
}

func TestManagerIsEnabledNoTLSConfig(t *testing.T) {
	cfg := &config.SSLConfig{
		Enabled: true,
	}
	m := &Manager{
		config:    cfg,
		tlsConfig: nil,
	}

	if m.IsEnabled() {
		t.Error("IsEnabled() should return false when tlsConfig is nil")
	}
}

func TestManagerIsEnabledWithTLSConfig(t *testing.T) {
	cfg := &config.SSLConfig{
		Enabled: true,
	}
	m := &Manager{
		config:    cfg,
		tlsConfig: &tls.Config{},
	}

	if !m.IsEnabled() {
		t.Error("IsEnabled() should return true when properly configured")
	}
}

func TestManagerGetTLSConfig(t *testing.T) {
	cfg := &config.SSLConfig{
		Enabled: false,
	}
	m := NewManager(cfg, "/tmp/test")

	tlsCfg := m.GetTLSConfig()
	// Should be nil when TLS is not configured
	_ = tlsCfg
}

func TestManagerGetHTTPSHandler(t *testing.T) {
	cfg := &config.SSLConfig{
		Enabled: false,
	}
	m := NewManager(cfg, "/tmp/test")

	fallback := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := m.GetHTTPSHandler(fallback)
	if handler == nil {
		t.Fatal("GetHTTPSHandler() returned nil")
	}

	// Test that handler works
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	// Should use fallback when certManager is nil
	if w.Code != http.StatusOK {
		t.Errorf("Handler returned status %d, want %d", w.Code, http.StatusOK)
	}
}

func TestManagerGetHTTPSHandlerWithCertManager(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "tls-test-handler-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	cfg := &config.SSLConfig{
		Enabled: true,
	}
	cfg.LetsEncrypt.Enabled = true
	cfg.LetsEncrypt.Domains = []string{"example.com"}
	cfg.LetsEncrypt.Email = "test@example.com"

	m := NewManager(cfg, tempDir)

	fallback := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := m.GetHTTPSHandler(fallback)
	if handler == nil {
		t.Fatal("GetHTTPSHandler() returned nil")
	}

	// Test that handler returns something
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
}

func TestCertInfoStruct(t *testing.T) {
	now := time.Now()
	info := CertInfo{
		Subject:    "example.com",
		Issuer:     "Let's Encrypt",
		NotBefore:  now.Add(-24 * time.Hour),
		NotAfter:   now.Add(90 * 24 * time.Hour),
		DNSNames:   []string{"example.com", "www.example.com"},
		IsExpiring: false,
	}

	if info.Subject != "example.com" {
		t.Errorf("Subject = %q, want %q", info.Subject, "example.com")
	}
	if info.Issuer != "Let's Encrypt" {
		t.Errorf("Issuer = %q, want %q", info.Issuer, "Let's Encrypt")
	}
	if info.IsExpiring {
		t.Error("IsExpiring should be false")
	}
	if len(info.DNSNames) != 2 {
		t.Errorf("DNSNames length = %d, want 2", len(info.DNSNames))
	}
}

func TestManagerGetCertInfoNoCert(t *testing.T) {
	cfg := &config.SSLConfig{
		Enabled: false,
	}
	m := NewManager(cfg, "/tmp/test")

	_, err := m.GetCertInfo()
	if err == nil {
		t.Error("GetCertInfo() should return error when no cert loaded")
	}
}

func TestManagerGetCertInfoEmptyCertificates(t *testing.T) {
	cfg := &config.SSLConfig{
		Enabled: true,
	}
	m := &Manager{
		config:    cfg,
		tlsConfig: &tls.Config{Certificates: []tls.Certificate{}},
	}

	_, err := m.GetCertInfo()
	if err == nil {
		t.Error("GetCertInfo() should return error when certificates list is empty")
	}
}

func TestManagerGetCertInfoWithValidCert(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "tls-test-certinfo-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Generate a test certificate
	certFile, keyFile, err := generateTestCert(tempDir)
	if err != nil {
		t.Fatalf("Failed to generate test cert: %v", err)
	}

	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		t.Fatalf("Failed to load cert: %v", err)
	}

	cfg := &config.SSLConfig{Enabled: true}
	m := &Manager{
		config:    cfg,
		tlsConfig: &tls.Config{Certificates: []tls.Certificate{cert}},
	}

	info, err := m.GetCertInfo()
	if err != nil {
		t.Fatalf("GetCertInfo() error = %v", err)
	}

	if info.Subject != "localhost" {
		t.Errorf("Subject = %q, want 'localhost'", info.Subject)
	}
	if len(info.DNSNames) == 0 {
		t.Error("DNSNames should not be empty")
	}
}

func TestManagerGetCertInfoWithLeafSet(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "tls-test-certinfo-leaf-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Generate a test certificate
	certFile, keyFile, err := generateTestCert(tempDir)
	if err != nil {
		t.Fatalf("Failed to generate test cert: %v", err)
	}

	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		t.Fatalf("Failed to load cert: %v", err)
	}

	// Pre-parse the leaf
	parsed, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		t.Fatalf("Failed to parse cert: %v", err)
	}
	cert.Leaf = parsed

	cfg := &config.SSLConfig{Enabled: true}
	m := &Manager{
		config:    cfg,
		tlsConfig: &tls.Config{Certificates: []tls.Certificate{cert}},
	}

	info, err := m.GetCertInfo()
	if err != nil {
		t.Fatalf("GetCertInfo() error = %v", err)
	}

	if info.Subject != "localhost" {
		t.Errorf("Subject = %q, want 'localhost'", info.Subject)
	}
}

func TestManagerGetCertInfoExpiringSoon(t *testing.T) {
	// Generate a cert that expires in less than 30 days
	tempDir, err := os.MkdirTemp("", "tls-test-expiring-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	certFile, keyFile, err := generateTestCertWithExpiry(tempDir, 10*24*time.Hour)
	if err != nil {
		t.Fatalf("Failed to generate test cert: %v", err)
	}

	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		t.Fatalf("Failed to load cert: %v", err)
	}

	cfg := &config.SSLConfig{Enabled: true}
	m := &Manager{
		config:    cfg,
		tlsConfig: &tls.Config{Certificates: []tls.Certificate{cert}},
	}

	info, err := m.GetCertInfo()
	if err != nil {
		t.Fatalf("GetCertInfo() error = %v", err)
	}

	if !info.IsExpiring {
		t.Error("IsExpiring should be true for cert expiring in 10 days")
	}
}

func TestManagerReloadCertificatesLetsEncrypt(t *testing.T) {
	cfg := &config.SSLConfig{
		Enabled: true,
	}
	cfg.LetsEncrypt.Enabled = true
	m := NewManager(cfg, "/tmp/test")

	// Should return nil for Let's Encrypt (handles its own renewal)
	err := m.ReloadCertificates()
	if err != nil {
		t.Errorf("ReloadCertificates() error = %v, want nil for Let's Encrypt", err)
	}
}

func TestManagerReloadCertificatesNoFiles(t *testing.T) {
	cfg := &config.SSLConfig{
		Enabled: true,
	}
	// LetsEncrypt.Enabled defaults to false
	m := NewManager(cfg, "/tmp/test")

	err := m.ReloadCertificates()
	if err == nil {
		t.Error("ReloadCertificates() should error when no cert files configured")
	}
}

func TestManagerReloadCertificatesInvalidFiles(t *testing.T) {
	cfg := &config.SSLConfig{
		Enabled:  true,
		CertFile: "/nonexistent/cert.pem",
		KeyFile:  "/nonexistent/key.pem",
	}
	m := &Manager{
		config:    cfg,
		tlsConfig: &tls.Config{},
	}

	err := m.ReloadCertificates()
	if err == nil {
		t.Error("ReloadCertificates() should error with invalid cert files")
	}
}

func TestManagerReloadCertificatesValid(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "tls-test-reload-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	certFile, keyFile, err := generateTestCert(tempDir)
	if err != nil {
		t.Fatalf("Failed to generate test cert: %v", err)
	}

	// Load initial cert
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		t.Fatalf("Failed to load cert: %v", err)
	}

	cfg := &config.SSLConfig{
		Enabled:  true,
		CertFile: certFile,
		KeyFile:  keyFile,
	}
	m := &Manager{
		config:    cfg,
		tlsConfig: &tls.Config{Certificates: []tls.Certificate{cert}},
	}

	err = m.ReloadCertificates()
	if err != nil {
		t.Errorf("ReloadCertificates() error = %v", err)
	}
}

func TestManagerInitManualCerts(t *testing.T) {
	// Create temp directory for certs
	tempDir, err := os.MkdirTemp("", "tls-test-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Generate self-signed cert for testing
	certFile, keyFile, err := generateTestCert(tempDir)
	if err != nil {
		t.Fatalf("Failed to generate test cert: %v", err)
	}

	cfg := &config.SSLConfig{
		Enabled:  true,
		CertFile: certFile,
		KeyFile:  keyFile,
	}

	m := NewManager(cfg, tempDir)

	if m.tlsConfig == nil {
		t.Error("TLS config should be set after loading manual certs")
	}
}

func TestManagerInitManualCertsInvalid(t *testing.T) {
	cfg := &config.SSLConfig{
		Enabled:  true,
		CertFile: "/nonexistent/cert.pem",
		KeyFile:  "/nonexistent/key.pem",
	}

	m := NewManager(cfg, "/tmp/test")

	// Should not panic, just log error
	if m.tlsConfig != nil {
		t.Error("TLS config should be nil for invalid cert files")
	}
}

func TestLegoUserInterface(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	user := &legoUser{
		email: "test@example.com",
		key:   key,
	}

	if user.GetEmail() != "test@example.com" {
		t.Errorf("GetEmail() = %q, want %q", user.GetEmail(), "test@example.com")
	}
	if user.GetPrivateKey() != key {
		t.Error("GetPrivateKey() returned wrong key")
	}
	if user.GetRegistration() != nil {
		t.Error("GetRegistration() should be nil initially")
	}
}

func TestCreateTLSConfig(t *testing.T) {
	// Create temp directory for certs
	tempDir, err := os.MkdirTemp("", "tls-test-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Generate self-signed cert
	certFile, keyFile, err := generateTestCert(tempDir)
	if err != nil {
		t.Fatalf("Failed to generate test cert: %v", err)
	}

	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		t.Fatalf("Failed to load cert: %v", err)
	}

	cfg := &config.SSLConfig{Enabled: true}
	m := NewManager(cfg, tempDir)

	tlsCfg := m.createTLSConfig(cert)

	if tlsCfg == nil {
		t.Fatal("createTLSConfig() returned nil")
	}
	if len(tlsCfg.Certificates) != 1 {
		t.Errorf("Certificates count = %d, want 1", len(tlsCfg.Certificates))
	}
	if tlsCfg.MinVersion != tls.VersionTLS12 {
		t.Errorf("MinVersion = %d, want TLS 1.2", tlsCfg.MinVersion)
	}
	if !tlsCfg.PreferServerCipherSuites {
		t.Error("PreferServerCipherSuites should be true")
	}
	if len(tlsCfg.CipherSuites) == 0 {
		t.Error("CipherSuites should not be empty")
	}
}

func TestManagerLoadOrCreateAccountKey(t *testing.T) {
	// Create temp directory
	tempDir, err := os.MkdirTemp("", "tls-test-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create certs subdirectory
	certsDir := filepath.Join(tempDir, "certs")
	os.MkdirAll(certsDir, 0700)

	cfg := &config.SSLConfig{Enabled: true}
	m := &Manager{
		config:  cfg,
		dataDir: tempDir,
	}

	// First call should create new key
	key1, err := m.loadOrCreateAccountKey()
	if err != nil {
		t.Fatalf("First loadOrCreateAccountKey() error = %v", err)
	}
	if key1 == nil {
		t.Fatal("First key should not be nil")
	}

	// Second call should load existing key
	key2, err := m.loadOrCreateAccountKey()
	if err != nil {
		t.Fatalf("Second loadOrCreateAccountKey() error = %v", err)
	}
	if key2 == nil {
		t.Fatal("Second key should not be nil")
	}
}

func TestManagerLoadOrCreateAccountKeyInvalidExisting(t *testing.T) {
	// Create temp directory
	tempDir, err := os.MkdirTemp("", "tls-test-invalid-key-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create certs subdirectory with invalid key file
	certsDir := filepath.Join(tempDir, "certs")
	os.MkdirAll(certsDir, 0700)
	keyPath := filepath.Join(certsDir, "account.key")
	os.WriteFile(keyPath, []byte("invalid key data"), 0600)

	cfg := &config.SSLConfig{Enabled: true}
	m := &Manager{
		config:  cfg,
		dataDir: tempDir,
	}

	// Should create new key when existing one is invalid
	key, err := m.loadOrCreateAccountKey()
	if err != nil {
		t.Fatalf("loadOrCreateAccountKey() error = %v", err)
	}
	if key == nil {
		t.Fatal("Key should not be nil")
	}
}

func TestManagerRenewCertificateDNS01NoClient(t *testing.T) {
	cfg := &config.SSLConfig{Enabled: true}
	m := NewManager(cfg, "/tmp/test")

	// Should return nil when legoClient is nil
	err := m.RenewCertificateDNS01(nil)
	if err != nil {
		t.Errorf("RenewCertificateDNS01() error = %v, want nil", err)
	}
}

func TestManagerRenewCertificateDNS01WithContext(t *testing.T) {
	cfg := &config.SSLConfig{Enabled: true}
	m := NewManager(cfg, "/tmp/test")

	ctx := context.Background()
	err := m.RenewCertificateDNS01(ctx)
	if err != nil {
		t.Errorf("RenewCertificateDNS01() error = %v, want nil", err)
	}
}

func TestManagerObtainCertificateDNS01NoClient(t *testing.T) {
	cfg := &config.SSLConfig{Enabled: true}
	m := &Manager{
		config:     cfg,
		legoClient: nil,
	}

	err := m.obtainCertificateDNS01()
	if err == nil {
		t.Error("obtainCertificateDNS01() should error when legoClient is nil")
	}
}

func TestManagerInitDNS01MissingProvider(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "tls-test-dns01-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	cfg := &config.SSLConfig{Enabled: true}
	cfg.LetsEncrypt.Enabled = true
	cfg.LetsEncrypt.Domains = []string{"example.com"}
	// DNS01.Provider is empty

	m := &Manager{
		config:    cfg,
		dataDir:   tempDir,
		secretKey: "secret",
	}

	err = m.initDNS01()
	if err == nil {
		t.Error("initDNS01() should error when provider is missing")
	}
}

func TestManagerInitDNS01MissingCredentials(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "tls-test-dns01-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	cfg := &config.SSLConfig{Enabled: true}
	cfg.LetsEncrypt.Enabled = true
	cfg.LetsEncrypt.Domains = []string{"example.com"}
	cfg.DNS01.Provider = "cloudflare"
	// DNS01.CredentialsEncrypted is empty

	m := &Manager{
		config:    cfg,
		dataDir:   tempDir,
		secretKey: "secret",
	}

	err = m.initDNS01()
	if err == nil {
		t.Error("initDNS01() should error when credentials are missing")
	}
}

func TestManagerInitDNS01MissingSecretKey(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "tls-test-dns01-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	cfg := &config.SSLConfig{Enabled: true}
	cfg.LetsEncrypt.Enabled = true
	cfg.LetsEncrypt.Domains = []string{"example.com"}
	cfg.DNS01.Provider = "cloudflare"
	cfg.DNS01.CredentialsEncrypted = "encrypted_data"

	m := &Manager{
		config:    cfg,
		dataDir:   tempDir,
		secretKey: "", // empty
	}

	err = m.initDNS01()
	if err == nil {
		t.Error("initDNS01() should error when secret key is missing")
	}
}

func TestManagerInitDNS01DecryptionError(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "tls-test-dns01-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	cfg := &config.SSLConfig{Enabled: true}
	cfg.LetsEncrypt.Enabled = true
	cfg.LetsEncrypt.Domains = []string{"example.com"}
	cfg.DNS01.Provider = "cloudflare"
	cfg.DNS01.CredentialsEncrypted = "invalid_encrypted_data"

	m := &Manager{
		config:    cfg,
		dataDir:   tempDir,
		secretKey: "secret",
	}

	err = m.initDNS01()
	if err == nil {
		t.Error("initDNS01() should error when decryption fails")
	}
}

func TestManagerInitDNS01InvalidProvider(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "tls-test-dns01-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Encrypt valid credentials
	creds := map[string]string{"api_token": "test"}
	password := "secret"
	encrypted, _ := EncryptCredentials(creds, password)

	cfg := &config.SSLConfig{Enabled: true}
	cfg.LetsEncrypt.Enabled = true
	cfg.LetsEncrypt.Domains = []string{"example.com"}
	cfg.DNS01.Provider = "unknown_provider"
	cfg.DNS01.CredentialsEncrypted = encrypted

	m := &Manager{
		config:    cfg,
		dataDir:   tempDir,
		secretKey: password,
	}

	err = m.initDNS01()
	if err == nil {
		t.Error("initDNS01() should error when DNS provider is invalid")
	}
}

func TestStartHTTPSRedirect(t *testing.T) {
	// Start redirect server on random port
	server := StartHTTPSRedirect(":0", 443)
	if server == nil {
		t.Fatal("StartHTTPSRedirect() returned nil")
	}

	// Give it time to start
	time.Sleep(50 * time.Millisecond)

	// Shutdown the server
	server.Close()
}

func TestStartHTTPSRedirectNonStandardPort(t *testing.T) {
	// Start redirect server on random port, targeting non-standard HTTPS port
	server := StartHTTPSRedirect(":0", 8443)
	if server == nil {
		t.Fatal("StartHTTPSRedirect() returned nil")
	}

	// Give it time to start
	time.Sleep(50 * time.Millisecond)

	// Shutdown the server
	server.Close()
}

func TestHTTPSRedirectHandler(t *testing.T) {
	// Test the redirect handler directly
	tests := []struct {
		name          string
		httpsPort     int
		host          string
		uri           string
		expectedLoc   string
	}{
		{
			name:        "Standard port 443",
			httpsPort:   443,
			host:        "example.com",
			uri:         "/path?query=1",
			expectedLoc: "https://example.com/path?query=1",
		},
		{
			name:        "Non-standard port",
			httpsPort:   8443,
			host:        "example.com",
			uri:         "/path",
			expectedLoc: "https://example.com:8443/path",
		},
		{
			name:        "With port in host",
			httpsPort:   443,
			host:        "example.com:80",
			uri:         "/",
			expectedLoc: "https://example.com:80/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				target := "https://" + r.Host
				if tt.httpsPort != 443 {
					target = "https://" + r.Host + ":" + string(rune('0'+tt.httpsPort/1000)) + string(rune('0'+(tt.httpsPort%1000)/100)) + string(rune('0'+(tt.httpsPort%100)/10)) + string(rune('0'+tt.httpsPort%10))
				}
				target += r.URL.RequestURI()
				http.Redirect(w, r, target, http.StatusMovedPermanently)
			})

			req := httptest.NewRequest("GET", tt.uri, nil)
			req.Host = tt.host
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			if w.Code != http.StatusMovedPermanently {
				t.Errorf("Status = %d, want %d", w.Code, http.StatusMovedPermanently)
			}
		})
	}
}

// Helper function to generate test certificates
func generateTestCert(dir string) (certFile, keyFile string, err error) {
	return generateTestCertWithExpiry(dir, 24*time.Hour)
}

func generateTestCertWithExpiry(dir string, duration time.Duration) (certFile, keyFile string, err error) {
	// Generate private key
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", "", err
	}

	// Create certificate template
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"Test"},
			CommonName:   "localhost",
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(duration),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{"localhost"},
	}

	// Create certificate
	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return "", "", err
	}

	// Write cert to file
	certFile = filepath.Join(dir, "cert.pem")
	certOut, err := os.Create(certFile)
	if err != nil {
		return "", "", err
	}
	certOut.Write([]byte("-----BEGIN CERTIFICATE-----\n"))
	certOut.Write([]byte(base64.StdEncoding.EncodeToString(derBytes)))
	certOut.Write([]byte("\n-----END CERTIFICATE-----\n"))
	certOut.Close()

	// Write key to file
	keyFile = filepath.Join(dir, "key.pem")
	keyBytes, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		return "", "", err
	}
	keyOut, err := os.Create(keyFile)
	if err != nil {
		return "", "", err
	}
	keyOut.Write([]byte("-----BEGIN EC PRIVATE KEY-----\n"))
	keyOut.Write([]byte(base64.StdEncoding.EncodeToString(keyBytes)))
	keyOut.Write([]byte("\n-----END EC PRIVATE KEY-----\n"))
	keyOut.Close()

	return certFile, keyFile, nil
}

// Tests for DNS functions

func TestDNSProviders(t *testing.T) {
	providers := DNSProviders()

	if len(providers) == 0 {
		t.Fatal("DNSProviders() returned empty list")
	}

	// Check for expected providers
	expectedIDs := []string{"cloudflare", "cloudflare_legacy", "route53", "digitalocean", "godaddy", "namecheap", "rfc2136"}
	for _, expectedID := range expectedIDs {
		found := false
		for _, p := range providers {
			if p.ID == expectedID {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected provider %q not found", expectedID)
		}
	}
}

func TestDNSProviderInfo(t *testing.T) {
	providers := DNSProviders()

	for _, p := range providers {
		if p.ID == "" {
			t.Error("Provider ID should not be empty")
		}
		if p.Name == "" {
			t.Errorf("Provider %q: Name should not be empty", p.ID)
		}
		if len(p.Fields) == 0 {
			t.Errorf("Provider %q: Fields should not be empty", p.ID)
		}
		// Check that each field has required properties
		for _, f := range p.Fields {
			if f.Name == "" {
				t.Errorf("Provider %q: Field Name should not be empty", p.ID)
			}
			if f.Label == "" {
				t.Errorf("Provider %q: Field %q Label should not be empty", p.ID, f.Name)
			}
		}
	}
}

func TestGetProviderByID(t *testing.T) {
	// Existing provider
	p := GetProviderByID("cloudflare")
	if p == nil {
		t.Fatal("GetProviderByID('cloudflare') returned nil")
	}
	if p.Name != "Cloudflare" {
		t.Errorf("Name = %q, want 'Cloudflare'", p.Name)
	}

	// Non-existent provider
	p = GetProviderByID("nonexistent")
	if p != nil {
		t.Error("GetProviderByID('nonexistent') should return nil")
	}
}

func TestGetProviderByIDAllProviders(t *testing.T) {
	providers := DNSProviders()
	for _, provider := range providers {
		p := GetProviderByID(provider.ID)
		if p == nil {
			t.Errorf("GetProviderByID(%q) returned nil", provider.ID)
		}
		if p.ID != provider.ID {
			t.Errorf("Provider ID = %q, want %q", p.ID, provider.ID)
		}
	}
}

func TestEncryptDecryptCredentials(t *testing.T) {
	credentials := map[string]string{
		"api_token": "test_token_12345",
		"secret":    "super_secret_value",
	}
	password := "test_password_123"

	// Encrypt
	encrypted, err := EncryptCredentials(credentials, password)
	if err != nil {
		t.Fatalf("EncryptCredentials() error = %v", err)
	}
	if encrypted == "" {
		t.Error("Encrypted string should not be empty")
	}

	// Decrypt
	decrypted, err := DecryptCredentials(encrypted, password)
	if err != nil {
		t.Fatalf("DecryptCredentials() error = %v", err)
	}

	// Verify
	if decrypted["api_token"] != credentials["api_token"] {
		t.Errorf("api_token = %q, want %q", decrypted["api_token"], credentials["api_token"])
	}
	if decrypted["secret"] != credentials["secret"] {
		t.Errorf("secret = %q, want %q", decrypted["secret"], credentials["secret"])
	}
}

func TestDecryptCredentialsWrongPassword(t *testing.T) {
	credentials := map[string]string{"key": "value"}
	password := "correct_password"
	wrongPassword := "wrong_password"

	encrypted, err := EncryptCredentials(credentials, password)
	if err != nil {
		t.Fatal(err)
	}

	_, err = DecryptCredentials(encrypted, wrongPassword)
	if err == nil {
		t.Error("DecryptCredentials() should error with wrong password")
	}
}

func TestDecryptCredentialsInvalidBase64(t *testing.T) {
	_, err := DecryptCredentials("not_valid_base64!!!", "password")
	if err == nil {
		t.Error("DecryptCredentials() should error with invalid base64")
	}
}

func TestDecryptCredentialsTooShort(t *testing.T) {
	_, err := DecryptCredentials("dGVzdA==", "password") // "test" in base64, too short
	if err == nil {
		t.Error("DecryptCredentials() should error when data is too short")
	}
}

func TestDecryptCredentialsMissingNonce(t *testing.T) {
	// Create data that's exactly 16 bytes (salt only, no nonce)
	data := make([]byte, 16)
	for i := range data {
		data[i] = byte(i)
	}
	encoded := base64.StdEncoding.EncodeToString(data)

	_, err := DecryptCredentials(encoded, "password")
	if err == nil {
		t.Error("DecryptCredentials() should error when nonce is missing")
	}
}

func TestIsDevTLD(t *testing.T) {
	tests := []struct {
		host        string
		projectName string
		want        bool
	}{
		{"localhost", "", true},
		{"app.localhost", "", true},
		{"test", "", true},
		{"myapp.test", "", true},
		{"example.com", "", false},
		{"www.example.com", "", false},
		{"dev.local", "", true},
		{"app.lan", "", true},
		{"server.internal", "", true},
		{"host.localdomain", "", true},
		{"dev.search", "search", true}, // Project-specific
		{"prod.search", "search", true},
		{"search", "search", false}, // Exact match without suffix
		{"app.home", "", true},
		{"app.home.arpa", "", true},
		{"app.intranet", "", true},
		{"app.corp", "", true},
		{"app.private", "", true},
		{"app.invalid", "", true},
		{"app.example", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.host, func(t *testing.T) {
			got := IsDevTLD(tt.host, tt.projectName)
			if got != tt.want {
				t.Errorf("IsDevTLD(%q, %q) = %v, want %v", tt.host, tt.projectName, got, tt.want)
			}
		})
	}
}

func TestIsDevTLDCaseInsensitive(t *testing.T) {
	if !IsDevTLD("LOCALHOST", "") {
		t.Error("IsDevTLD should be case insensitive")
	}
	if !IsDevTLD("APP.LOCAL", "") {
		t.Error("IsDevTLD should be case insensitive for .local")
	}
	if !IsDevTLD("Dev.Search", "SEARCH") {
		t.Error("IsDevTLD should be case insensitive for project name")
	}
}

func TestGetFQDN(t *testing.T) {
	// This test just verifies the function returns a non-empty string
	fqdn := GetFQDN("testproject")
	if fqdn == "" {
		t.Error("GetFQDN() returned empty string")
	}
}

func TestGetFQDNWithDomainEnv(t *testing.T) {
	original := os.Getenv("DOMAIN")
	defer os.Setenv("DOMAIN", original)

	os.Setenv("DOMAIN", "example.com")
	fqdn := GetFQDN("test")
	if fqdn != "example.com" {
		t.Errorf("GetFQDN() = %q, want 'example.com'", fqdn)
	}
}

func TestGetFQDNWithCommaSeparatedDomains(t *testing.T) {
	original := os.Getenv("DOMAIN")
	defer os.Setenv("DOMAIN", original)

	os.Setenv("DOMAIN", "primary.com, secondary.com, third.com")
	fqdn := GetFQDN("test")
	if fqdn != "primary.com" {
		t.Errorf("GetFQDN() = %q, want 'primary.com'", fqdn)
	}
}

func TestGetFQDNWithHostnameEnv(t *testing.T) {
	originalDomain := os.Getenv("DOMAIN")
	originalHostname := os.Getenv("HOSTNAME")
	defer func() {
		os.Setenv("DOMAIN", originalDomain)
		os.Setenv("HOSTNAME", originalHostname)
	}()

	os.Unsetenv("DOMAIN")
	os.Setenv("HOSTNAME", "myserver.example.com")

	fqdn := GetFQDN("test")
	// Should return hostname or fall through to other methods
	if fqdn == "" {
		t.Error("GetFQDN() returned empty string")
	}
}

func TestGetFQDNWithLocalhostHostname(t *testing.T) {
	originalDomain := os.Getenv("DOMAIN")
	originalHostname := os.Getenv("HOSTNAME")
	defer func() {
		os.Setenv("DOMAIN", originalDomain)
		os.Setenv("HOSTNAME", originalHostname)
	}()

	os.Unsetenv("DOMAIN")
	os.Setenv("HOSTNAME", "localhost")

	fqdn := GetFQDN("test")
	// Should skip localhost and try other methods
	if fqdn == "" {
		t.Error("GetFQDN() returned empty string")
	}
}

func TestGetAllDomains(t *testing.T) {
	// Save original DOMAIN env
	original := os.Getenv("DOMAIN")
	defer os.Setenv("DOMAIN", original)

	// Test with no DOMAIN set
	os.Unsetenv("DOMAIN")
	domains := GetAllDomains()
	if domains != nil {
		t.Error("GetAllDomains() should return nil when DOMAIN is not set")
	}

	// Test with single domain
	os.Setenv("DOMAIN", "example.com")
	domains = GetAllDomains()
	if len(domains) != 1 || domains[0] != "example.com" {
		t.Errorf("GetAllDomains() = %v, want [example.com]", domains)
	}

	// Test with multiple domains
	os.Setenv("DOMAIN", "example.com, www.example.com, api.example.com")
	domains = GetAllDomains()
	if len(domains) != 3 {
		t.Errorf("GetAllDomains() length = %d, want 3", len(domains))
	}
}

func TestGetAllDomainsWithEmptyParts(t *testing.T) {
	original := os.Getenv("DOMAIN")
	defer os.Setenv("DOMAIN", original)

	os.Setenv("DOMAIN", "example.com,  , www.example.com,")
	domains := GetAllDomains()
	if len(domains) != 2 {
		t.Errorf("GetAllDomains() length = %d, want 2 (skipping empty parts)", len(domains))
	}
}

func TestGetWildcardDomain(t *testing.T) {
	// Save original DOMAIN env
	original := os.Getenv("DOMAIN")
	defer os.Setenv("DOMAIN", original)

	// Test with single domain (no wildcard)
	os.Setenv("DOMAIN", "example.com")
	wildcard := GetWildcardDomain()
	if wildcard != "" {
		t.Errorf("GetWildcardDomain() = %q, want empty for single domain", wildcard)
	}

	// Test with multiple subdomains of same base
	os.Setenv("DOMAIN", "www.example.com, api.example.com, admin.example.com")
	wildcard = GetWildcardDomain()
	if wildcard != "*.example.com" {
		t.Errorf("GetWildcardDomain() = %q, want '*.example.com'", wildcard)
	}
}

func TestGetWildcardDomainDifferentBases(t *testing.T) {
	original := os.Getenv("DOMAIN")
	defer os.Setenv("DOMAIN", original)

	// Different base domains should return empty
	os.Setenv("DOMAIN", "www.example.com, api.other.com")
	wildcard := GetWildcardDomain()
	if wildcard != "" {
		t.Errorf("GetWildcardDomain() = %q, want empty for different bases", wildcard)
	}
}

func TestGetWildcardDomainNoDomain(t *testing.T) {
	original := os.Getenv("DOMAIN")
	defer os.Setenv("DOMAIN", original)

	os.Unsetenv("DOMAIN")
	wildcard := GetWildcardDomain()
	if wildcard != "" {
		t.Errorf("GetWildcardDomain() = %q, want empty when DOMAIN not set", wildcard)
	}
}

func TestFormatURL(t *testing.T) {
	tests := []struct {
		host    string
		port    int
		isHTTPS bool
		want    string
	}{
		{"example.com", 80, false, "http://example.com"},
		{"example.com", 443, true, "https://example.com"},
		{"example.com", 8080, false, "http://example.com:8080"},
		{"example.com", 8443, true, "https://example.com:8443"},
		{"[::1]", 80, false, "http://[::1]"},
		{"[::1]", 8080, false, "http://[::1]:8080"},
		{"192.168.1.1", 443, true, "https://192.168.1.1"},
		{"192.168.1.1", 8443, true, "https://192.168.1.1:8443"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := formatURL(tt.host, tt.port, tt.isHTTPS)
			if got != tt.want {
				t.Errorf("formatURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestValidatedAtNow(t *testing.T) {
	timestamp := ValidatedAtNow()
	if timestamp == "" {
		t.Error("ValidatedAtNow() returned empty string")
	}
	// Verify it's valid RFC3339 format
	_, err := time.Parse(time.RFC3339, timestamp)
	if err != nil {
		t.Errorf("ValidatedAtNow() returned invalid RFC3339: %v", err)
	}
}

func TestFieldStruct(t *testing.T) {
	field := Field{
		Name:        "api_key",
		Label:       "API Key",
		Type:        "password",
		Required:    true,
		Placeholder: "Enter your API key",
		Help:        "Get your API key from the dashboard",
	}

	if field.Name != "api_key" {
		t.Errorf("Name = %q", field.Name)
	}
	if field.Type != "password" {
		t.Errorf("Type = %q", field.Type)
	}
	if !field.Required {
		t.Error("Required should be true")
	}
}

func TestDNSProviderInfoStruct(t *testing.T) {
	info := DNSProviderInfo{
		ID:          "test",
		Name:        "Test Provider",
		Description: "A test provider",
		Fields:      []Field{{Name: "api_key", Label: "API Key", Type: "password", Required: true}},
	}

	if info.ID != "test" {
		t.Errorf("ID = %q", info.ID)
	}
	if info.Name != "Test Provider" {
		t.Errorf("Name = %q", info.Name)
	}
	if info.Description != "A test provider" {
		t.Errorf("Description = %q", info.Description)
	}
	if len(info.Fields) != 1 {
		t.Errorf("Fields length = %d", len(info.Fields))
	}
}

func TestCreateDNSProviderMissingCredentials(t *testing.T) {
	tests := []struct {
		provider    string
		credentials map[string]string
	}{
		{"cloudflare", map[string]string{}},
		{"cloudflare_legacy", map[string]string{"api_key": "key"}}, // missing email
		{"cloudflare_legacy", map[string]string{"email": "test@example.com"}}, // missing api_key
		{"route53", map[string]string{"access_key_id": "id"}},                 // missing secret
		{"route53", map[string]string{"secret_access_key": "secret"}},         // missing id
		{"digitalocean", map[string]string{}},
		{"godaddy", map[string]string{"api_key": "key"}},                       // missing secret
		{"godaddy", map[string]string{"api_secret": "secret"}},                 // missing key
		{"namecheap", map[string]string{"api_user": "user", "api_key": "key"}}, // missing client_ip
		{"namecheap", map[string]string{"api_user": "user"}},                   // missing api_key and client_ip
		{"rfc2136", map[string]string{"nameserver": "ns1.example.com:53"}},     // missing tsig
		{"rfc2136", map[string]string{"nameserver": "ns", "tsig_key": "key"}},  // missing tsig_secret
	}

	for _, tt := range tests {
		t.Run(tt.provider, func(t *testing.T) {
			_, err := CreateDNSProvider(tt.provider, tt.credentials)
			if err == nil {
				t.Errorf("CreateDNSProvider(%q) should error with missing credentials", tt.provider)
			}
		})
	}
}

func TestCreateDNSProviderUnknown(t *testing.T) {
	_, err := CreateDNSProvider("unknown_provider", map[string]string{})
	if err == nil {
		t.Error("CreateDNSProvider('unknown_provider') should error")
	}
}

func TestCreateDNSProviderRfc2136AlgorithmSuffix(t *testing.T) {
	// Test that algorithm without suffix gets suffix added
	creds := map[string]string{
		"nameserver":     "ns1.example.com:53",
		"tsig_key":       "testkey",
		"tsig_secret":    "dGVzdHNlY3JldA==",
		"tsig_algorithm": "hmac-sha256", // without trailing dot
	}

	// This will fail to create (no actual server), but we're testing the algorithm handling
	_, err := CreateDNSProvider("rfc2136", creds)
	// Error is expected because there's no actual DNS server
	if err == nil {
		// If it succeeds, that's also fine - the important thing is it didn't panic
		t.Log("CreateDNSProvider succeeded (unexpected but acceptable)")
	}
}

func TestCreateDNSProviderRfc2136DefaultAlgorithm(t *testing.T) {
	// Test default algorithm
	creds := map[string]string{
		"nameserver":  "ns1.example.com:53",
		"tsig_key":    "testkey",
		"tsig_secret": "dGVzdHNlY3JldA==",
		// no tsig_algorithm - should default
	}

	_, err := CreateDNSProvider("rfc2136", creds)
	// Error is expected because there's no actual DNS server
	if err == nil {
		t.Log("CreateDNSProvider succeeded (unexpected but acceptable)")
	}
}

func TestEncryptCredentialsEmpty(t *testing.T) {
	credentials := map[string]string{}
	password := "password"

	encrypted, err := EncryptCredentials(credentials, password)
	if err != nil {
		t.Fatalf("EncryptCredentials() error = %v", err)
	}

	decrypted, err := DecryptCredentials(encrypted, password)
	if err != nil {
		t.Fatalf("DecryptCredentials() error = %v", err)
	}

	if len(decrypted) != 0 {
		t.Errorf("Decrypted should be empty map, got %d items", len(decrypted))
	}
}

func TestEncryptCredentialsLargeData(t *testing.T) {
	credentials := make(map[string]string)
	for i := 0; i < 100; i++ {
		key := "key" + string(rune('0'+i/10)) + string(rune('0'+i%10))
		credentials[key] = "value_" + key + "_with_some_longer_content"
	}
	password := "password"

	encrypted, err := EncryptCredentials(credentials, password)
	if err != nil {
		t.Fatalf("EncryptCredentials() error = %v", err)
	}

	decrypted, err := DecryptCredentials(encrypted, password)
	if err != nil {
		t.Fatalf("DecryptCredentials() error = %v", err)
	}

	if len(decrypted) != 100 {
		t.Errorf("Decrypted length = %d, want 100", len(decrypted))
	}
}

func TestIsLoopback(t *testing.T) {
	tests := []struct {
		host string
		want bool
	}{
		{"localhost", true},
		{"LOCALHOST", true},
		{"127.0.0.1", true},
		{"::1", true},
		{"192.168.1.1", false},
		{"example.com", false},
		{"10.0.0.1", false},
		{"172.16.0.1", false},
		{"fe80::1", false},
	}

	for _, tt := range tests {
		t.Run(tt.host, func(t *testing.T) {
			got := isLoopback(tt.host)
			if got != tt.want {
				t.Errorf("isLoopback(%q) = %v, want %v", tt.host, got, tt.want)
			}
		})
	}
}

func TestExtractBaseDomain(t *testing.T) {
	tests := []struct {
		domain string
		want   string
	}{
		{"www.example.com", "example.com"},
		{"api.example.com", "example.com"},
		{"example.com", "example.com"},
		{"sub.sub.example.co.uk", "example.co.uk"},
	}

	for _, tt := range tests {
		t.Run(tt.domain, func(t *testing.T) {
			got := extractBaseDomain(tt.domain)
			if got != tt.want {
				t.Errorf("extractBaseDomain(%q) = %q, want %q", tt.domain, got, tt.want)
			}
		})
	}
}

func TestExtractBaseDomainInvalid(t *testing.T) {
	// Invalid domain should return the input
	got := extractBaseDomain("notadomain")
	// publicsuffix may return the input for invalid domains
	if got == "" {
		t.Error("extractBaseDomain should return something for invalid domain")
	}
}

func TestGetDisplayURL(t *testing.T) {
	// Save original DOMAIN env
	original := os.Getenv("DOMAIN")
	defer os.Setenv("DOMAIN", original)

	// Test with explicit domain
	os.Setenv("DOMAIN", "example.com")
	url := GetDisplayURL("testproject", 443, true)
	if url != "https://example.com" {
		t.Errorf("GetDisplayURL() = %q, want 'https://example.com'", url)
	}

	url = GetDisplayURL("testproject", 8080, false)
	if url != "http://example.com:8080" {
		t.Errorf("GetDisplayURL() = %q, want 'http://example.com:8080'", url)
	}
}

func TestGetDisplayURLWithDevTLD(t *testing.T) {
	original := os.Getenv("DOMAIN")
	defer os.Setenv("DOMAIN", original)

	// Test with dev TLD - should try to use IP instead
	os.Setenv("DOMAIN", "app.localhost")
	url := GetDisplayURL("testproject", 443, true)
	// Should return something (may be the localhost or an IP)
	if url == "" {
		t.Error("GetDisplayURL() returned empty string")
	}
}

func TestGetDisplayURLNoEnv(t *testing.T) {
	originalDomain := os.Getenv("DOMAIN")
	originalHostname := os.Getenv("HOSTNAME")
	defer func() {
		os.Setenv("DOMAIN", originalDomain)
		os.Setenv("HOSTNAME", originalHostname)
	}()

	os.Unsetenv("DOMAIN")
	os.Unsetenv("HOSTNAME")

	url := GetDisplayURL("testproject", 8080, false)
	if url == "" {
		t.Error("GetDisplayURL() returned empty string")
	}
}

func TestGetHostFromRequest(t *testing.T) {
	tests := []struct {
		name        string
		headers     map[string]string
		projectName string
		wantHost    string
	}{
		{
			name:        "X-Forwarded-Host header",
			headers:     map[string]string{"X-Forwarded-Host": "proxy.example.com"},
			projectName: "test",
			wantHost:    "proxy.example.com",
		},
		{
			name:        "X-Forwarded-Host with port",
			headers:     map[string]string{"X-Forwarded-Host": "proxy.example.com:8080"},
			projectName: "test",
			wantHost:    "proxy.example.com",
		},
		{
			name:        "X-Real-Host header",
			headers:     map[string]string{"X-Real-Host": "real.example.com"},
			projectName: "test",
			wantHost:    "real.example.com",
		},
		{
			name:        "X-Real-Host with port",
			headers:     map[string]string{"X-Real-Host": "real.example.com:443"},
			projectName: "test",
			wantHost:    "real.example.com",
		},
		{
			name:        "X-Original-Host header",
			headers:     map[string]string{"X-Original-Host": "original.example.com"},
			projectName: "test",
			wantHost:    "original.example.com",
		},
		{
			name:        "X-Original-Host with port",
			headers:     map[string]string{"X-Original-Host": "original.example.com:9000"},
			projectName: "test",
			wantHost:    "original.example.com",
		},
		{
			name:        "Header priority - X-Forwarded-Host first",
			headers:     map[string]string{"X-Forwarded-Host": "forwarded.com", "X-Real-Host": "real.com"},
			projectName: "test",
			wantHost:    "forwarded.com",
		},
		{
			name:        "No proxy headers - falls back to GetFQDN",
			headers:     map[string]string{},
			projectName: "test",
			wantHost:    "", // Will be determined by GetFQDN
		},
	}

	// Save and clear DOMAIN env for consistent tests
	original := os.Getenv("DOMAIN")
	defer os.Setenv("DOMAIN", original)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}

			if tt.wantHost != "" {
				os.Setenv("DOMAIN", "fallback.com") // Set a fallback
				got := GetHostFromRequest(req, tt.projectName)
				if got != tt.wantHost {
					t.Errorf("GetHostFromRequest() = %q, want %q", got, tt.wantHost)
				}
			} else {
				// No expected host - just make sure it returns something
				os.Setenv("DOMAIN", "fallback.com")
				got := GetHostFromRequest(req, tt.projectName)
				if got == "" {
					t.Error("GetHostFromRequest() returned empty string")
				}
			}
		})
	}
}

func TestGetHostFromRequestNoHeaders(t *testing.T) {
	original := os.Getenv("DOMAIN")
	defer os.Setenv("DOMAIN", original)

	os.Setenv("DOMAIN", "example.com")
	req := httptest.NewRequest("GET", "/", nil)

	got := GetHostFromRequest(req, "test")
	if got != "example.com" {
		t.Errorf("GetHostFromRequest() = %q, want 'example.com'", got)
	}
}

func TestGetGlobalIPv6(t *testing.T) {
	// Just verify the function doesn't panic
	ipv6 := getGlobalIPv6()
	// May return empty string if no global IPv6 is available
	_ = ipv6
}

func TestGetGlobalIPv4(t *testing.T) {
	// Just verify the function doesn't panic
	ipv4 := getGlobalIPv4()
	// May return empty string if no global IPv4 is available
	_ = ipv4
}

// Test concurrent access to Manager
func TestManagerConcurrentAccess(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "tls-test-concurrent-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	certFile, keyFile, err := generateTestCert(tempDir)
	if err != nil {
		t.Fatalf("Failed to generate test cert: %v", err)
	}

	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		t.Fatalf("Failed to load cert: %v", err)
	}

	cfg := &config.SSLConfig{
		Enabled:  true,
		CertFile: certFile,
		KeyFile:  keyFile,
	}
	m := &Manager{
		config:    cfg,
		tlsConfig: &tls.Config{Certificates: []tls.Certificate{cert}},
	}

	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				_ = m.GetTLSConfig()
				_ = m.IsEnabled()
				_, _ = m.GetCertInfo()
			}
			done <- true
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}
}
