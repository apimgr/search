package alert

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"syscall"
)

// validateWebhookURL enforces that a user-supplied webhook target cannot be used
// for SSRF. It requires an http/https scheme and rejects any host that resolves
// to a loopback, private, link-local, unspecified, or multicast address.
//
// This is a best-effort pre-flight check; the transport-level dialControl below
// re-verifies the actual connected IP to defend against DNS rebinding.
func validateWebhookURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("%w: webhook URL is invalid", ErrInvalidInput)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("%w: webhook URL must use http or https", ErrInvalidInput)
	}
	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("%w: webhook URL is missing a host", ErrInvalidInput)
	}
	ips, err := net.LookupIP(host)
	if err != nil {
		return fmt.Errorf("%w: webhook host could not be resolved", ErrInvalidInput)
	}
	for _, ip := range ips {
		if isDisallowedIP(ip) {
			return fmt.Errorf("%w: webhook host resolves to a disallowed address", ErrInvalidInput)
		}
	}
	return nil
}

// allowLoopbackWebhooks is a test-only seam. httptest servers bind to
// 127.0.0.1, so delivery tests set this to exercise the real send path.
// Production never enables it — loopback targets stay blocked.
var allowLoopbackWebhooks = false

// isDisallowedIP reports whether an IP is in a range that must never be reached
// by a user-controlled webhook (loopback, RFC1918/ULA private, link-local,
// unspecified, or multicast).
func isDisallowedIP(ip net.IP) bool {
	if ip.IsLoopback() {
		return !allowLoopbackWebhooks
	}
	return ip.IsPrivate() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() ||
		ip.IsMulticast() ||
		ip.IsUnspecified()
}

// dialControl is installed on the webhook HTTP client's dialer. It runs after
// DNS resolution with the concrete address that is about to be connected,
// closing the DNS-rebinding gap left by the pre-flight validateWebhookURL check.
func dialControl(_, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("webhook dial: invalid address %q", address)
	}
	ip := net.ParseIP(host)
	if ip == nil || isDisallowedIP(ip) {
		return fmt.Errorf("webhook dial: address %q is not permitted", address)
	}
	return nil
}

// newWebhookDialContext returns a DialContext that applies dialControl.
func newWebhookDialContext() func(ctx context.Context, network, addr string) (net.Conn, error) {
	d := &net.Dialer{Control: dialControl}
	return d.DialContext
}
