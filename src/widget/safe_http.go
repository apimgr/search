package widget

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var widgetPublicLookup = net.DefaultResolver.LookupIPAddr

func newWidgetPublicHTTPClient(timeout time.Duration) *http.Client {
	transport := &http.Transport{}
	if defaultTransport, ok := http.DefaultTransport.(*http.Transport); ok {
		transport = defaultTransport.Clone()
	}
	transport.Proxy = nil
	transport.DialContext = dialWidgetPublicHost
	transport.MaxResponseHeaderBytes = 1 << 20
	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
		CheckRedirect: func(request *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("too many redirects")
			}
			return validateWidgetPublicURL(request.Context(), request.URL)
		},
	}
}

func validateWidgetPublicURL(ctx context.Context, target *url.URL) error {
	if target == nil || (target.Scheme != "http" && target.Scheme != "https") || target.Hostname() == "" || target.User != nil {
		return fmt.Errorf("feed URL must use HTTP or HTTPS and must not contain user information")
	}
	return validateWidgetPublicHost(ctx, target.Hostname(), widgetPublicLookup)
}

func dialWidgetPublicHost(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	ips, err := widgetPublicLookup(ctx, host)
	if ip := net.ParseIP(host); ip != nil {
		ips, err = []net.IPAddr{{IP: ip}}, nil
	}
	if err != nil {
		return nil, err
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("feed host has no address")
	}
	for _, ip := range ips {
		if !isWidgetPublicIP(ip.IP) {
			return nil, fmt.Errorf("feed host resolves to a non-public address")
		}
	}
	for _, ip := range ips {
		conn, err := (&net.Dialer{Timeout: 3 * time.Second}).DialContext(ctx, network, net.JoinHostPort(ip.IP.String(), port))
		if err == nil {
			return conn, nil
		}
	}
	return nil, fmt.Errorf("unable to connect to feed host")
}

func validateWidgetPublicHost(ctx context.Context, host string, lookup func(context.Context, string) ([]net.IPAddr, error)) error {
	if host == "" {
		return fmt.Errorf("feed host is empty")
	}
	if ip := net.ParseIP(host); ip != nil {
		if !isWidgetPublicIP(ip) {
			return fmt.Errorf("feed host is not public")
		}
		return nil
	}
	ips, err := lookup(ctx, host)
	if err != nil {
		return err
	}
	if len(ips) == 0 {
		return fmt.Errorf("feed host has no address")
	}
	for _, ip := range ips {
		if !isWidgetPublicIP(ip.IP) {
			return fmt.Errorf("feed host resolves to a non-public address")
		}
	}
	return nil
}

func isWidgetPublicIP(ip net.IP) bool {
	if ip == nil || !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() {
		return false
	}
	for _, cidr := range []string{"100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "2001:db8::/32"} {
		if strings.Contains(cidr, ":") {
			_, network, _ := net.ParseCIDR(cidr)
			if network.Contains(ip) {
				return false
			}
			continue
		}
		_, network, _ := net.ParseCIDR(cidr)
		if network.Contains(ip) {
			return false
		}
	}
	return true
}
