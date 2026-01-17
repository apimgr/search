package tls

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
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

func TestManagerRenewCertificateDNS01NoClient(t *testing.T) {
	cfg := &config.SSLConfig{Enabled: true}
	m := NewManager(cfg, "/tmp/test")

	// Should return nil when legoClient is nil
	err := m.RenewCertificateDNS01(nil)
	if err != nil {
		t.Errorf("RenewCertificateDNS01() error = %v, want nil", err)
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

// Helper function to generate test certificates
func generateTestCert(dir string) (certFile, keyFile string, err error) {
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
		NotAfter:              time.Now().Add(24 * time.Hour),
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
	certOut.Write([]byte(encodeBase64(derBytes)))
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
	keyOut.Write([]byte(encodeBase64(keyBytes)))
	keyOut.Write([]byte("\n-----END EC PRIVATE KEY-----\n"))
	keyOut.Close()

	return certFile, keyFile, nil
}

// Simple base64 encoding helper
func encodeBase64(data []byte) string {
	const base64Table = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	var result []byte

	for i := 0; i < len(data); i += 3 {
		var b0, b1, b2 byte
		b0 = data[i]
		if i+1 < len(data) {
			b1 = data[i+1]
		}
		if i+2 < len(data) {
			b2 = data[i+2]
		}

		result = append(result, base64Table[b0>>2])
		result = append(result, base64Table[((b0&0x03)<<4)|(b1>>4)])

		if i+1 < len(data) {
			result = append(result, base64Table[((b1&0x0f)<<2)|(b2>>6)])
		} else {
			result = append(result, '=')
		}

		if i+2 < len(data) {
			result = append(result, base64Table[b2&0x3f])
		} else {
			result = append(result, '=')
		}
	}

	return string(result)
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
}

func TestGetFQDN(t *testing.T) {
	// This test just verifies the function returns a non-empty string
	fqdn := GetFQDN("testproject")
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

func TestCreateDNSProviderMissingCredentials(t *testing.T) {
	tests := []struct {
		provider    string
		credentials map[string]string
	}{
		{"cloudflare", map[string]string{}},
		{"cloudflare_legacy", map[string]string{"api_key": "key"}}, // missing email
		{"route53", map[string]string{"access_key_id": "id"}},      // missing secret
		{"digitalocean", map[string]string{}},
		{"godaddy", map[string]string{"api_key": "key"}},                         // missing secret
		{"namecheap", map[string]string{"api_user": "user", "api_key": "key"}},   // missing client_ip
		{"rfc2136", map[string]string{"nameserver": "ns1.example.com:53"}},       // missing tsig
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
