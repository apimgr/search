package widget

import (
	"context"
	"net"
	"testing"
)

func TestValidateWidgetPublicHostRejectsPrivateAndMixedDNS(t *testing.T) {
	lookup := func(_ context.Context, host string) ([]net.IPAddr, error) {
		switch host {
		case "private.example":
			return []net.IPAddr{{IP: net.ParseIP("10.1.2.3")}}, nil
		case "mixed.example":
			return []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}, {IP: net.ParseIP("127.0.0.1")}}, nil
		default:
			return []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}}, nil
		}
	}
	for _, host := range []string{"private.example", "mixed.example", "127.0.0.1", "169.254.1.1", "192.0.2.1"} {
		t.Run(host, func(t *testing.T) {
			if err := validateWidgetPublicHost(context.Background(), host, lookup); err == nil {
				t.Fatalf("validateWidgetPublicHost(%q) unexpectedly succeeded", host)
			}
		})
	}
	if err := validateWidgetPublicHost(context.Background(), "public.example", lookup); err != nil {
		t.Fatalf("public host rejected: %v", err)
	}
}

func TestRSSFeedURLRejectsUnsafeSchemesAndCredentials(t *testing.T) {
	fetcher := NewRSSFetcher(nil)
	for _, rawURL := range []string{
		"file:///etc/passwd",
		"ftp://example.com/feed.xml",
		"http://user:password@example.com/feed.xml",
		"//example.com/feed.xml",
	} {
		t.Run(rawURL, func(t *testing.T) {
			if _, err := fetcher.fetchSingleFeed(context.Background(), rawURL); err == nil {
				t.Fatalf("fetchSingleFeed(%q) unexpectedly succeeded", rawURL)
			}
		})
	}
}
