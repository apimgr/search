package netutil

import (
	"net"
	"testing"
)

func TestJoinHostPortAcceptsBracketedAndBareIPv6(t *testing.T) {
	for _, test := range []struct {
		address string
		want    string
	}{
		{address: "[::]", want: "[::]:8080"},
		{address: "::", want: "[::]:8080"},
		{address: "2001:db8::1", want: "[2001:db8::1]:8080"},
	} {
		if got := JoinHostPort(test.address, "8080"); got != test.want {
			t.Errorf("JoinHostPort(%q) = %q, want %q", test.address, got, test.want)
		}
	}
}

func TestListenTCPKeepsExplicitIPv4Address(t *testing.T) {
	listener, err := ListenTCP("0.0.0.0", 0)
	if err != nil {
		t.Fatalf("ListenTCP() error = %v", err)
	}
	defer listener.Close()
	host, _, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	if host != "0.0.0.0" {
		t.Fatalf("listener bound host %q, want explicit IPv4 wildcard", host)
	}
}

func TestListenTCPBindsDualStackDefaultOrDocumentedFallback(t *testing.T) {
	listener, err := ListenTCP("[::]", 0)
	if err != nil {
		t.Fatalf("ListenTCP() error = %v", err)
	}
	defer listener.Close()
	host, _, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	if host != "::" && host != "0.0.0.0" {
		t.Fatalf("listener bound host %q, want dual-stack default or IPv4 fallback", host)
	}
}
