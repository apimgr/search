package server

import (
	"crypto/tls"
	"fmt"
	"log"
	"os"
	"path/filepath"

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

// GetCertificatePaths returns the paths for SSL certificates
func (m *SSLManager) GetCertificatePaths() (certFile, keyFile string) {
	ssl := m.config.Server.SSL

	if ssl.CertFile != "" && ssl.KeyFile != "" {
		return ssl.CertFile, ssl.KeyFile
	}

	// Default paths
	sslDir := config.GetSSLDir()
	return filepath.Join(sslDir, "cert.pem"), filepath.Join(sslDir, "key.pem")
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
		log.Println("   SSL/TLS: Disabled")
		return
	}

	if ssl.LetsEncrypt.Enabled {
		log.Println("   SSL/TLS: Let's Encrypt (auto-renewal)")
		log.Printf("   Domains: %v", ssl.LetsEncrypt.Domains)
	} else if m.HasValidCertificate() {
		certFile, _ := m.GetCertificatePaths()
		log.Printf("   SSL/TLS: Enabled (certificate: %s)", certFile)
	} else {
		log.Println("   SSL/TLS: Enabled (no certificate found)")
	}
}
