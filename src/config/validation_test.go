package config

import "testing"

func TestIsValidHost(t *testing.T) {
	tests := []struct {
		name        string
		host        string
		devMode     bool
		projectName string
		want        bool
	}{
		// Valid production hosts
		{"valid FQDN", "example.com", false, "search", true},
		{"valid subdomain", "search.example.com", false, "search", true},
		{"valid multi-subdomain", "api.search.example.com", false, "search", true},

		// Invalid hosts - IP addresses
		{"IPv4 address", "192.168.1.1", false, "search", false},
		{"IPv6 address", "::1", false, "search", false},
		{"IPv4 loopback", "127.0.0.1", false, "search", false},

		// Invalid hosts - localhost in production
		{"localhost production", "localhost", false, "search", false},
		{"localhost with port production", "localhost:8080", false, "search", false},

		// Development mode allows localhost
		{"localhost dev mode", "localhost", true, "search", true},
		{"localhost with port dev mode", "localhost:8080", true, "search", true},

		// Invalid hosts - empty and whitespace
		{"empty string", "", false, "search", false},
		{"whitespace only", "   ", false, "search", false},

		// Invalid hosts - reserved TLDs in production
		{"example TLD production", "test.example", false, "search", false},
		{"localhost TLD production", "myapp.localhost", false, "search", false},
		{"test TLD production", "myapp.test", false, "search", false},
		{"invalid TLD production", "myapp.invalid", false, "search", false},

		// Development mode allows reserved TLDs
		{"example TLD dev mode", "test.example", true, "search", true},
		{"localhost TLD dev mode", "myapp.localhost", true, "search", true},

		// Valid onion addresses
		{"onion address", "abcdefghijklmnop.onion", false, "search", true},

		// Port stripping
		{"FQDN with port", "example.com:443", false, "search", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsValidHost(tt.host, tt.devMode, tt.projectName)
			if got != tt.want {
				t.Errorf("IsValidHost(%q, %v, %q) = %v, want %v",
					tt.host, tt.devMode, tt.projectName, got, tt.want)
			}
		})
	}
}

func TestIsValidPort(t *testing.T) {
	tests := []struct {
		name string
		port int
		want bool
	}{
		{"valid port 80", 80, true},
		{"valid port 443", 443, true},
		{"valid port 8080", 8080, true},
		{"valid port 1", 1, true},
		{"valid port 65535", 65535, true},
		{"invalid port 0", 0, false},
		{"invalid port negative", -1, false},
		{"invalid port too high", 65536, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsValidPort(tt.port)
			if got != tt.want {
				t.Errorf("IsValidPort(%d) = %v, want %v", tt.port, got, tt.want)
			}
		})
	}
}

func TestIsValidEmail(t *testing.T) {
	tests := []struct {
		name  string
		email string
		want  bool
	}{
		{"valid email", "user@example.com", true},
		{"valid email with subdomain", "user@mail.example.com", true},
		{"valid email with plus", "user+tag@example.com", true},
		{"valid email with dots", "first.last@example.com", true},
		{"invalid email no at", "userexample.com", false},
		{"invalid email no domain", "user@", false},
		{"invalid email no local", "@example.com", false},
		{"invalid email empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsValidEmail(tt.email)
			if got != tt.want {
				t.Errorf("IsValidEmail(%q) = %v, want %v", tt.email, got, tt.want)
			}
		})
	}
}
