package alert

import (
	"errors"
	"net"
	"testing"
)

func TestIsDisallowedIP(t *testing.T) {
	tests := []struct {
		name string
		ip   string
		want bool
	}{
		{"loopback v4", "127.0.0.1", true},
		{"loopback v6", "::1", true},
		{"private 10", "10.0.0.5", true},
		{"private 192", "192.168.1.1", true},
		{"private 172", "172.16.0.1", true},
		{"link-local", "169.254.169.254", true},
		{"unspecified", "0.0.0.0", true},
		{"multicast", "224.0.0.1", true},
		{"public v4", "93.184.216.34", false},
		{"public v6", "2606:2800:220:1:248:1893:25c8:1946", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ip := net.ParseIP(tt.ip)
			if ip == nil {
				t.Fatalf("bad test IP %q", tt.ip)
			}
			if got := isDisallowedIP(ip); got != tt.want {
				t.Errorf("isDisallowedIP(%s) = %v, want %v", tt.ip, got, tt.want)
			}
		})
	}
}

func TestValidateWebhookURLScheme(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{"empty", "", true},
		{"no scheme", "example.com/hook", true},
		{"ftp scheme", "ftp://example.com/hook", true},
		{"file scheme", "file:///etc/passwd", true},
		{"gopher scheme", "gopher://example.com", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateWebhookURL(tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateWebhookURL(%q) err = %v, wantErr %v", tt.url, err, tt.wantErr)
			}
			if err != nil && !errors.Is(err, ErrInvalidInput) {
				t.Errorf("validateWebhookURL(%q) err = %v, want ErrInvalidInput", tt.url, err)
			}
		})
	}
}

func TestDialControlBlocksInternal(t *testing.T) {
	if err := dialControl("tcp", "10.0.0.1:443", nil); err == nil {
		t.Error("dialControl allowed a private address")
	}
	if err := dialControl("tcp", "93.184.216.34:443", nil); err != nil {
		t.Errorf("dialControl blocked a public address: %v", err)
	}
}
