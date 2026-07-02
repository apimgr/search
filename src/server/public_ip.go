// Public IP cache and refresher.
//
// Per AI.md PART 8 step 16: a hardcoded `public_ip_refresh` scheduler task
// runs on startup and every 12 hours, queries a small set of well-known
// "what is my IP" endpoints, caches the result in memory, and exposes it to
// the rest of the server (FQDN detection) via Server.GetPublicIP().
//
// Privacy note: this only reads the *server's* own public IP, not any user's
// IP. The cached value is never persisted to disk and never logged.
package server

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/apimgr/search/src/ssl"
)

// publicIPProviders is the fallback chain used to discover the server's
// public IPv4 address. The providers return only an IP literal in the body.
var publicIPProviders = []string{
	"https://api.ipify.org",
	"https://ifconfig.me/ip",
	"https://icanhazip.com",
}

// publicIPCache holds the last known good public IPv4 address.
type publicIPCache struct {
	mu        sync.RWMutex
	ip        string
	updatedAt time.Time
}

// publicIP is the process-wide cache. It is populated by the
// public_ip_refresh scheduler task and consumed by FQDN detection.
var publicIP = &publicIPCache{}

// GetPublicIP returns the most recently cached server public IPv4 address
// and the time it was last refreshed. Returns empty string if the refresher
// has not yet succeeded (callers must fall back to other discovery paths).
func (s *Server) GetPublicIP() (string, time.Time) {
	publicIP.mu.RLock()
	defer publicIP.mu.RUnlock()
	return publicIP.ip, publicIP.updatedAt
}

// refreshPublicIP queries publicIPProviders in order and returns the first
// well-formed public IPv4 address. On failure it logs a WARN and leaves the
// cached value untouched, per AI.md PART 8 step 16.
func (s *Server) refreshPublicIP(ctx context.Context) error {
	client := &http.Client{Timeout: 5 * time.Second}
	var lastErr error
	for _, url := range publicIPProviders {
		ip, err := fetchPublicIP(ctx, client, url)
		if err != nil {
			lastErr = err
			continue
		}
		publicIP.mu.Lock()
		previous := publicIP.ip
		publicIP.ip = ip
		publicIP.updatedAt = time.Now()
		publicIP.mu.Unlock()
		// Hand the value to ssl so GetFQDN can use it for FQDN detection.
		ssl.SetCachedPublicIP(ip)
		if previous != ip {
			slog.Info("Refreshed public IP", "provider", url)
		}
		return nil
	}
	slog.Warn("All public IP providers failed, keeping previous value", "err", lastErr)
	if lastErr == nil {
		lastErr = fmt.Errorf("no public IP providers configured")
	}
	return lastErr
}

// fetchPublicIP makes a single GET against a "what is my IP" endpoint and
// validates the response is a public, non-private IPv4 literal.
func fetchPublicIP(ctx context.Context, client *http.Client, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("get %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%s: status %d", url, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64))
	if err != nil {
		return "", fmt.Errorf("read %s: %w", url, err)
	}
	ipStr := strings.TrimSpace(string(body))
	parsed := net.ParseIP(ipStr)
	if parsed == nil {
		return "", fmt.Errorf("%s: invalid IP %q", url, ipStr)
	}
	ip4 := parsed.To4()
	if ip4 == nil {
		return "", fmt.Errorf("%s: not an IPv4 address %q", url, ipStr)
	}
	if !ip4.IsGlobalUnicast() || ip4.IsPrivate() || ip4.IsLoopback() || ip4.IsLinkLocalUnicast() {
		return "", fmt.Errorf("%s: %s is not a public IP", url, ipStr)
	}
	return ip4.String(), nil
}
