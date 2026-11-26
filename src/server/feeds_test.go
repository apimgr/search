package server

import (
	"strings"
	"testing"
)

func TestSafeFeedQueryDropsSensitiveQueries(t *testing.T) {
	tests := []struct {
		name  string
		query string
	}{
		{name: "phone", query: "+1 (212) 555-0198"},
		{name: "email", query: "name@example.com"},
		{name: "street address", query: "42 Main Street"},
		{name: "po box", query: "PO Box 123"},
		{name: "government number", query: "123456789"},
		{name: "card number", query: "4111111111111111"},
		{name: "social security number", query: "123-45-6789"},
		{name: "ipv4", query: "192.0.2.1"},
		{name: "ipv6", query: "2001:db8::1"},
		{name: "token-like", query: "Abcdefghijklmnopqrst123"},
		{name: "credential URL", query: "https://example.com/?token=abc123"},
		{name: "too long", query: "search " + strings.Repeat("x", 95)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if safeFeedQuery(tt.query) {
				t.Fatalf("safeFeedQuery(%q) = true, want false", tt.query)
			}
		})
	}
}

func TestSafeFeedQueryKeepsOrdinarySearches(t *testing.T) {
	for _, query := range []string{"weather tomorrow", "best hiking trails", "how to bake bread"} {
		if !safeFeedQuery(query) {
			t.Errorf("safeFeedQuery(%q) = false, want true", query)
		}
	}
}
