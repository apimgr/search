package ssl

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/go-acme/lego/v4/challenge"
	legoDNS "github.com/go-acme/lego/v4/providers/dns"
	"github.com/go-acme/lego/v4/providers/dns/cloudflare"
	"github.com/go-acme/lego/v4/providers/dns/digitalocean"
	"github.com/go-acme/lego/v4/providers/dns/godaddy"
	"github.com/go-acme/lego/v4/providers/dns/namecheap"
	"github.com/go-acme/lego/v4/providers/dns/rfc2136"
	"github.com/go-acme/lego/v4/providers/dns/route53"

	"golang.org/x/crypto/argon2"
	"golang.org/x/net/publicsuffix"
)

//go:embed dns_providers.json
var legoDNSProviderCatalog []byte

type legoProviderDefinition struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Credentials []struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	} `json:"credentials"`
}

var legoProviderEnvMu sync.Mutex

// DNSProviderInfo contains metadata about a DNS provider
// Per AI.md PART 17: Admin WebUI provides dropdown with required fields
type DNSProviderInfo struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Fields      []Field `json:"fields"`
	Description string  `json:"description"`
}

// Field represents a credential field for a DNS provider
type Field struct {
	Name  string `json:"name"`
	Label string `json:"label"`
	// text, password
	Type        string `json:"type"`
	Required    bool   `json:"required"`
	Placeholder string `json:"placeholder"`
	Help        string `json:"help"`
}

// DNSProviders returns all supported DNS providers with their required fields
// Per AI.md: Full provider list from lego DNS providers
func DNSProviders() []DNSProviderInfo {
	providers := []DNSProviderInfo{
		{
			ID:          "cloudflare",
			Name:        "Cloudflare",
			Description: "Cloudflare DNS",
			Fields: []Field{
				{Name: "api_token", Label: "API Token", Type: "password", Required: false, Help: "Cloudflare API token with Zone:DNS:Edit permission (or use api_key + email)"},
				{Name: "api_key", Label: "Global API Key", Type: "password", Required: false, Help: "Legacy Cloudflare Global API Key, requires email"},
				{Name: "email", Label: "Email", Type: "text", Required: false, Help: "Cloudflare account email, required with api_key"},
			},
		},
		{
			ID:          "cloudflare_legacy",
			Name:        "Cloudflare (Legacy API Key)",
			Description: "Cloudflare DNS using legacy Global API Key",
			Fields: []Field{
				{Name: "api_key", Label: "Global API Key", Type: "password", Required: true, Help: "Cloudflare Global API Key"},
				{Name: "email", Label: "Email", Type: "text", Required: true, Help: "Cloudflare account email"},
			},
		},
		{
			ID:          "route53",
			Name:        "Amazon Route 53",
			Description: "AWS Route 53 DNS",
			Fields: []Field{
				{Name: "access_key_id", Label: "Access Key ID", Type: "text", Required: true, Help: "AWS Access Key ID"},
				{Name: "secret_access_key", Label: "Secret Access Key", Type: "password", Required: true, Help: "AWS Secret Access Key"},
				{Name: "region", Label: "Region", Type: "text", Required: false, Placeholder: "us-east-1", Help: "AWS Region (optional)"},
			},
		},
		{
			ID:          "digitalocean",
			Name:        "DigitalOcean",
			Description: "DigitalOcean DNS",
			Fields: []Field{
				{Name: "auth_token", Label: "Auth Token", Type: "password", Required: true, Help: "DigitalOcean API token"},
			},
		},
		{
			ID:          "godaddy",
			Name:        "GoDaddy",
			Description: "GoDaddy DNS",
			Fields: []Field{
				{Name: "api_key", Label: "API Key", Type: "password", Required: true, Help: "GoDaddy API Key"},
				{Name: "api_secret", Label: "API Secret", Type: "password", Required: true, Help: "GoDaddy API Secret"},
			},
		},
		{
			ID:          "namecheap",
			Name:        "Namecheap",
			Description: "Namecheap DNS",
			Fields: []Field{
				{Name: "api_user", Label: "API User", Type: "text", Required: true, Help: "Namecheap API username"},
				{Name: "api_key", Label: "API Key", Type: "password", Required: true, Help: "Namecheap API key"},
				{Name: "client_ip", Label: "Client IP", Type: "text", Required: true, Help: "Your whitelisted IP address"},
			},
		},
		{
			ID:          "rfc2136",
			Name:        "RFC 2136 (TSIG)",
			Description: "Dynamic DNS Updates (BIND, PowerDNS, etc.)",
			Fields: []Field{
				{Name: "nameserver", Label: "Nameserver", Type: "text", Required: true, Placeholder: "ns1.example.com:53", Help: "DNS server address with port"},
				{Name: "tsig_key", Label: "TSIG Key Name", Type: "text", Required: true, Help: "TSIG key name"},
				{Name: "tsig_secret", Label: "TSIG Secret", Type: "password", Required: true, Help: "Base64-encoded TSIG secret"},
				{Name: "tsig_algorithm", Label: "TSIG Algorithm", Type: "text", Required: false, Placeholder: "hmac-sha256", Help: "TSIG algorithm (default: hmac-sha256)"},
			},
		},
	}
	var definitions []legoProviderDefinition
	if err := json.Unmarshal(legoDNSProviderCatalog, &definitions); err != nil {
		panic(fmt.Sprintf("invalid embedded lego DNS provider catalog: %v", err))
	}
	existing := make(map[string]struct{}, len(providers))
	for _, provider := range providers {
		existing[provider.ID] = struct{}{}
	}
	for _, definition := range definitions {
		if _, ok := existing[definition.ID]; ok {
			continue
		}
		info := DNSProviderInfo{ID: definition.ID, Name: definition.Name, Description: definition.Name + " DNS"}
		for _, credential := range definition.Credentials {
			label := strings.ReplaceAll(strings.ToLower(credential.Name), "_", " ")
			label = strings.ToUpper(label[:1]) + label[1:]
			fieldType := "text"
			if strings.Contains(credential.Name, "KEY") || strings.Contains(credential.Name, "TOKEN") || strings.Contains(credential.Name, "SECRET") || strings.Contains(credential.Name, "PASSWORD") || strings.Contains(credential.Name, "CREDENTIAL") {
				fieldType = "password"
			}
			info.Fields = append(info.Fields, Field{Name: credential.Name, Label: label, Type: fieldType, Help: credential.Description})
		}
		providers = append(providers, info)
		existing[definition.ID] = struct{}{}
	}
	return providers
}

// GetProviderByID returns provider info by ID
func GetProviderByID(id string) *DNSProviderInfo {
	for _, p := range DNSProviders() {
		if p.ID == id {
			return &p
		}
	}
	return nil
}

// CreateDNSProvider creates a lego DNS provider from credentials
// Per AI.md: Credentials are encrypted JSON stored in config
func CreateDNSProvider(providerID string, credentials map[string]string) (challenge.Provider, error) {
	switch providerID {
	case "cloudflare":
		// AI.md PART 15 documents cloudflare as accepting api_token OR
		// (api_key + email), so both credential shapes are handled here.
		token := credentials["api_token"]
		if token != "" {
			return cloudflare.NewDNSProviderConfig(&cloudflare.Config{
				AuthToken: token,
			})
		}
		apiKey := credentials["api_key"]
		email := credentials["email"]
		if apiKey == "" || email == "" {
			return nil, fmt.Errorf("cloudflare: api_token or api_key and email are required")
		}
		return cloudflare.NewDNSProviderConfig(&cloudflare.Config{
			AuthEmail: email,
			AuthKey:   apiKey,
		})

	case "cloudflare_legacy":
		apiKey := credentials["api_key"]
		email := credentials["email"]
		if apiKey == "" || email == "" {
			return nil, fmt.Errorf("cloudflare_legacy: api_key and email are required")
		}
		return cloudflare.NewDNSProviderConfig(&cloudflare.Config{
			AuthEmail: email,
			AuthKey:   apiKey,
		})

	case "route53":
		accessKey := credentials["access_key_id"]
		secretKey := credentials["secret_access_key"]
		if accessKey == "" || secretKey == "" {
			return nil, fmt.Errorf("route53: access_key_id and secret_access_key are required")
		}
		cfg := &route53.Config{
			AccessKeyID:     accessKey,
			SecretAccessKey: secretKey,
		}
		if region := credentials["region"]; region != "" {
			cfg.Region = region
		}
		return route53.NewDNSProviderConfig(cfg)

	case "digitalocean":
		token := credentials["auth_token"]
		if token == "" {
			return nil, fmt.Errorf("digitalocean: auth_token is required")
		}
		return digitalocean.NewDNSProviderConfig(&digitalocean.Config{
			AuthToken: token,
		})

	case "godaddy":
		apiKey := credentials["api_key"]
		apiSecret := credentials["api_secret"]
		if apiKey == "" || apiSecret == "" {
			return nil, fmt.Errorf("godaddy: api_key and api_secret are required")
		}
		return godaddy.NewDNSProviderConfig(&godaddy.Config{
			APIKey:    apiKey,
			APISecret: apiSecret,
		})

	case "namecheap":
		apiUser := credentials["api_user"]
		apiKey := credentials["api_key"]
		clientIP := credentials["client_ip"]
		if apiUser == "" || apiKey == "" || clientIP == "" {
			return nil, fmt.Errorf("namecheap: api_user, api_key, and client_ip are required")
		}
		return namecheap.NewDNSProviderConfig(&namecheap.Config{
			APIUser:  apiUser,
			APIKey:   apiKey,
			ClientIP: clientIP,
		})

	case "rfc2136":
		nameserver := credentials["nameserver"]
		tsigKey := credentials["tsig_key"]
		tsigSecret := credentials["tsig_secret"]
		if nameserver == "" || tsigKey == "" || tsigSecret == "" {
			return nil, fmt.Errorf("rfc2136: nameserver, tsig_key, and tsig_secret are required")
		}
		algorithm := credentials["tsig_algorithm"]
		if algorithm == "" {
			algorithm = "hmac-sha256."
		}
		if !strings.HasSuffix(algorithm, ".") {
			algorithm += "."
		}
		return rfc2136.NewDNSProviderConfig(&rfc2136.Config{
			Nameserver:    nameserver,
			TSIGKey:       tsigKey,
			TSIGSecret:    tsigSecret,
			TSIGAlgorithm: algorithm,
		})

	default:
		info := GetProviderByID(providerID)
		if info == nil {
			return nil, fmt.Errorf("dns provider %s is not supported; supported providers: %s", providerID, strings.Join(SupportedProviderIDs(), ", "))
		}
		return createLegoDNSProvider(providerID, info, credentials)
	}
}

func createLegoDNSProvider(providerID string, info *DNSProviderInfo, credentials map[string]string) (result challenge.Provider, resultErr error) {
	legoProviderEnvMu.Lock()
	defer legoProviderEnvMu.Unlock()
	original := make(map[string]*string, len(info.Fields))
	for _, field := range info.Fields {
		value, exists := os.LookupEnv(field.Name)
		if exists {
			copy := value
			original[field.Name] = &copy
		} else {
			original[field.Name] = nil
		}
	}
	defer func() {
		var restoreErr error
		for name, value := range original {
			if value == nil {
				restoreErr = errors.Join(restoreErr, os.Unsetenv(name))
			} else {
				restoreErr = errors.Join(restoreErr, os.Setenv(name, *value))
			}
		}
		if restoreErr != nil {
			result = nil
			resultErr = errors.Join(resultErr, fmt.Errorf("restore lego DNS credential environment: %w", restoreErr))
		}
	}()
	for _, field := range info.Fields {
		if err := os.Unsetenv(field.Name); err != nil {
			return nil, fmt.Errorf("clear lego DNS credential %s: %w", field.Name, err)
		}
	}
	for _, field := range info.Fields {
		if value := credentials[field.Name]; value != "" {
			if err := os.Setenv(field.Name, value); err != nil {
				return nil, fmt.Errorf("set lego DNS credential %s: %w", field.Name, err)
			}
		}
	}
	provider, err := legoDNS.NewDNSChallengeProviderByName(providerID)
	if err != nil {
		return nil, fmt.Errorf("initialize DNS provider %s: %w", providerID, err)
	}
	return provider, nil
}

// SupportedProviderIDs returns the identifiers of every DNS-01 provider this
// build can construct, in registry order.
func SupportedProviderIDs() []string {
	providers := DNSProviders()
	ids := make([]string, 0, len(providers))
	for _, provider := range providers {
		ids = append(ids, provider.ID)
	}
	return ids
}

// ValidateCredentials verifies that a provider's required credentials are
// present. AI.md PART 15 requires the server to validate DNS-01 credentials on
// startup and again before every certificate request. An unknown provider is
// rejected; a known provider is checked against its declared required fields.
func ValidateCredentials(providerID string, credentials map[string]string) error {
	if providerID == "" {
		return fmt.Errorf("dns provider is required")
	}

	// Cloudflare accepts either credential shape per AI.md PART 15.
	if providerID == "cloudflare" {
		if credentials["api_token"] == "" && (credentials["api_key"] == "" || credentials["email"] == "") {
			return fmt.Errorf("cloudflare: api_token or api_key and email are required")
		}
		return nil
	}

	info := GetProviderByID(providerID)
	if info == nil {
		return fmt.Errorf("dns provider %s is not supported; supported providers: %s",
			providerID, strings.Join(SupportedProviderIDs(), ", "))
	}
	if !isExplicitDNSProvider(providerID) {
		_, err := CreateDNSProvider(providerID, credentials)
		return err
	}
	for _, field := range info.Fields {
		if field.Required && strings.TrimSpace(credentials[field.Name]) == "" {
			return fmt.Errorf("%s: %s is required", providerID, field.Name)
		}
	}
	return nil
}

func isExplicitDNSProvider(providerID string) bool {
	switch providerID {
	case "cloudflare", "cloudflare_legacy", "route53", "digitalocean", "godaddy", "namecheap", "rfc2136":
		return true
	default:
		return false
	}
}

// EncryptCredentials encrypts credentials JSON using AES-256-GCM
// Per AI.md: credentials are encrypted with AES-256-GCM before storage
func EncryptCredentials(credentials map[string]string, password string) (string, error) {
	// Serialize credentials to JSON
	plaintext, err := json.Marshal(credentials)
	if err != nil {
		return "", fmt.Errorf("failed to marshal credentials: %w", err)
	}

	// Derive key using Argon2id (same as password hashing)
	salt := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return "", fmt.Errorf("failed to generate salt: %w", err)
	}

	key := argon2.IDKey([]byte(password), salt, 1, 64*1024, 4, 32)

	// Create AES-256-GCM cipher
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}

	// Generate nonce
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Encrypt
	ciphertext := gcm.Seal(nil, nonce, plaintext, nil)

	// Combine: salt + nonce + ciphertext
	combined := make([]byte, len(salt)+len(nonce)+len(ciphertext))
	copy(combined, salt)
	copy(combined[len(salt):], nonce)
	copy(combined[len(salt)+len(nonce):], ciphertext)

	return base64.StdEncoding.EncodeToString(combined), nil
}

// DecryptCredentials decrypts credentials from encrypted storage
func DecryptCredentials(encrypted string, password string) (map[string]string, error) {
	// Decode base64
	combined, err := base64.StdEncoding.DecodeString(encrypted)
	if err != nil {
		return nil, fmt.Errorf("failed to decode encrypted credentials: %w", err)
	}

	// Extract salt (16 bytes)
	if len(combined) < 16 {
		return nil, fmt.Errorf("invalid encrypted data: too short")
	}
	salt := combined[:16]

	// Derive key
	key := argon2.IDKey([]byte(password), salt, 1, 64*1024, 4, 32)

	// Create cipher
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	// Extract nonce and ciphertext
	nonceSize := gcm.NonceSize()
	if len(combined) < 16+nonceSize {
		return nil, fmt.Errorf("invalid encrypted data: missing nonce")
	}
	nonce := combined[16 : 16+nonceSize]
	ciphertext := combined[16+nonceSize:]

	// Decrypt
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt credentials: %w", err)
	}

	// Unmarshal JSON
	var credentials map[string]string
	if err := json.Unmarshal(plaintext, &credentials); err != nil {
		return nil, fmt.Errorf("failed to unmarshal credentials: %w", err)
	}

	return credentials, nil
}

// ValidatedAtNow returns the current timestamp for validated_at field
func ValidatedAtNow() string {
	return time.Now().UTC().Format(time.RFC3339)
}

// devOnlyTLDs are TLDs that are only valid in development mode
// Per AI.md PART 21: Dev TLD Handling (NON-NEGOTIABLE)
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

// GetHostFromRequest resolves the host from an HTTP request
// Per AI.md PART 21: use this for request-time host resolution (preferred)
func GetHostFromRequest(r *http.Request, projectName string) string {
	// 1. Reverse proxy headers (highest priority - we prefer to be behind a proxy)
	for _, header := range []string{"X-Forwarded-Host", "X-Real-Host", "X-Original-Host"} {
		if host := r.Header.Get(header); host != "" {
			// Strip port if present (we handle port separately)
			if h, _, err := net.SplitHostPort(host); err == nil {
				return h
			}
			return host
		}
	}

	// 2. Fall back to static resolution
	return GetFQDN(projectName)
}

// cachedPublicIP holds a public IPv4 address detected by an external
// refresher (the server's public_ip_refresh scheduler task, AI.md PART 8
// step 16). GetFQDN consults this before scanning local interfaces, since
// modern servers usually sit behind NAT and have no globally-routable
// address on a local NIC.
var (
	cachedPublicIPMu sync.RWMutex
	cachedPublicIP   string
)

// SetCachedPublicIP updates the cached public IPv4 address used by
// GetFQDN. Called by the public_ip_refresh scheduler task.
func SetCachedPublicIP(ip string) {
	cachedPublicIPMu.Lock()
	cachedPublicIP = ip
	cachedPublicIPMu.Unlock()
}

// CachedPublicIP returns the most recently cached public IPv4 address (or
// empty string if no refresh has succeeded yet).
func CachedPublicIP() string {
	cachedPublicIPMu.RLock()
	defer cachedPublicIPMu.RUnlock()
	return cachedPublicIP
}

// GetFQDN resolves the Fully Qualified Domain Name
// Per AI.md PART 21: use this when no request context available (startup, background tasks)
// Returns first domain from DOMAIN env var (comma-separated list supported)
func GetFQDN(projectName string) string {
	// 1. DOMAIN env var (explicit user override, comma-separated)
	if domain := os.Getenv("DOMAIN"); domain != "" {
		// Return first domain as primary
		if idx := strings.Index(domain, ","); idx > 0 {
			return strings.TrimSpace(domain[:idx])
		}
		return domain
	}

	// 2. os.Hostname() - cross-platform (Linux, macOS, Windows, BSD)
	if hostname, err := os.Hostname(); err == nil && hostname != "" {
		if !isLoopback(hostname) {
			return hostname
		}
	}

	// 3. $HOSTNAME env var (skip loopback)
	if hostname := os.Getenv("HOSTNAME"); hostname != "" {
		if !isLoopback(hostname) {
			return hostname
		}
	}

	// 4. Global IPv6 (preferred for modern networks)
	if ipv6 := getGlobalIPv6(); ipv6 != "" {
		return ipv6
	}

	// 5. Global IPv4 from local NICs (works on bare metal with a public IP)
	if ipv4 := getGlobalIPv4(); ipv4 != "" {
		return ipv4
	}

	// 6. Cached public IPv4 from the public_ip_refresh scheduler task
	// (handles the common NAT case where no local NIC has a public address).
	if ipv4 := CachedPublicIP(); ipv4 != "" {
		return ipv4
	}

	// Last resort (not recommended)
	return "localhost"
}

// isLoopback checks if a hostname is loopback
func isLoopback(host string) bool {
	lower := strings.ToLower(host)
	if lower == "localhost" {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback()
	}
	return false
}

// IsDevTLD checks if the host is a development TLD
// Per AI.md PART 21: Dev TLD Handling (NON-NEGOTIABLE)
func IsDevTLD(host, projectName string) bool {
	lower := strings.ToLower(host)

	// Check dynamic project-specific TLD (e.g., app.jokes, dev.search) and the
	// bare project name itself, which AI.md PART 15 also treats as a dev TLD.
	if projectName != "" {
		pn := strings.ToLower(projectName)
		if lower == pn || strings.HasSuffix(lower, "."+pn) {
			return true
		}
	}

	// Check static dev TLDs
	for tld := range devOnlyTLDs {
		if strings.HasSuffix(lower, "."+tld) || lower == tld {
			return true
		}
	}

	return false
}

// overlayTLDs are overlay-network suffixes. AI.md PART 15 forbids issuing or
// self-signing any certificate for these — the overlay layer already provides
// end-to-end encryption and Let's Encrypt does not support them.
var overlayTLDs = []string{".onion", ".i2p", ".exit"}

// IsOverlayAddress reports whether host is a Tor or I2P overlay address.
// Overlay hosts are always served over plain HTTP and never get a certificate.
func IsOverlayAddress(host string) bool {
	lower := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
	for _, tld := range overlayTLDs {
		if strings.HasSuffix(lower, tld) {
			return true
		}
	}
	return false
}

// getGlobalIPv6 returns first public IPv6 address
// Excludes: loopback (::1), link-local (fe80::/10), unique local (fc00::/7)
func getGlobalIPv6() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok {
			ip := ipnet.IP
			// Must be IPv6 (not IPv4), globally routable, and not private
			if ip.To4() == nil && ip.IsGlobalUnicast() && !ip.IsPrivate() {
				return ip.String()
			}
		}
	}
	return ""
}

// getGlobalIPv4 returns first public IPv4 address
// Excludes: loopback (127.0.0.0/8), private (10/8, 172.16/12, 192.168/16), link-local (169.254/16)
func getGlobalIPv4() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok {
			ip := ipnet.IP
			// Must be IPv4, globally routable, and not private
			if ip4 := ip.To4(); ip4 != nil && ip.IsGlobalUnicast() && !ip.IsPrivate() {
				return ip4.String()
			}
		}
	}
	return ""
}

// GetAllDomains returns all domains from DOMAIN env var
// Used for CORS configuration and SSL certificates
func GetAllDomains() []string {
	domain := os.Getenv("DOMAIN")
	if domain == "" {
		return nil
	}
	parts := strings.Split(domain, ",")
	domains := make([]string, 0, len(parts))
	for _, p := range parts {
		if d := strings.TrimSpace(p); d != "" {
			domains = append(domains, d)
		}
	}
	return domains
}

// GetWildcardDomain infers wildcard from DOMAIN list or learned patterns
// Returns "*.example.com" if multiple subdomains share same base, else ""
func GetWildcardDomain() string {
	domains := GetAllDomains()
	if len(domains) < 2 {
		return ""
	}

	// Extract base domain from first (primary)
	base := extractBaseDomain(domains[0])

	// Check if all share same base
	for _, d := range domains[1:] {
		if extractBaseDomain(d) != base {
			return ""
		}
	}

	return "*." + base
}

// extractBaseDomain gets eTLD+1 using publicsuffix
func extractBaseDomain(domain string) string {
	base, err := publicsuffix.EffectiveTLDPlusOne(domain)
	if err != nil {
		return domain
	}
	return base
}

// GetDisplayURL returns the best URL for display/access
// Prefers valid FQDN, falls back to global IP if dev TLD
func GetDisplayURL(projectName string, port int, isHTTPS bool) string {
	fqdn := GetFQDN(projectName)

	// If valid production FQDN, use it
	if !IsDevTLD(fqdn, projectName) && fqdn != "localhost" {
		return formatURL(fqdn, port, isHTTPS)
	}

	// Dev TLD or localhost - use global IP instead
	if ipv6 := getGlobalIPv6(); ipv6 != "" {
		return formatURL("["+ipv6+"]", port, isHTTPS)
	}
	if ipv4 := getGlobalIPv4(); ipv4 != "" {
		return formatURL(ipv4, port, isHTTPS)
	}

	return formatURL(fqdn, port, isHTTPS)
}

// formatURL formats a URL with the appropriate protocol and port.
// Per AI.md PART 15 (URL Format Rule): port 443 always means HTTPS even when
// isHTTPS was not set, and :80 and :443 are ALWAYS stripped.
func formatURL(host string, port int, isHTTPS bool) string {
	proto := "http"
	if isHTTPS || port == 443 {
		proto = "https"
	}

	// Always strip :80 and :443
	if port == 80 || port == 443 {
		return proto + "://" + host
	}

	return fmt.Sprintf("%s://%s:%d", proto, host, port)
}
