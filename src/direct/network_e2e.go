//go:build e2e

package direct

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"strings"
)

// SetE2ENetworkHooks replaces network dependencies only in the browser-test build.
func SetE2ENetworkHooks(dial func(context.Context, string, string) (net.Conn, error), dnsAddress string, lookup func(context.Context, string) ([]net.IPAddr, error), searcher DirectorySearcher) {
	publicNetworkDialOverride = dial
	dnsResolverAddress = dnsAddress
	resolveDNSServers = []string{dnsAddress, dnsAddress, dnsAddress}
	publicDirectoryLookup = lookup
	publicNetworkTLSConfig = &tls.Config{InsecureSkipVerify: true}
	publicNetworkTransportOverride = e2eFixtureTransport{
		providers: http.DefaultTransport,
		public: &http.Transport{
			Proxy:           nil,
			DialContext:     dial,
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
	directorySearcherOverride = searcher
}

type e2eFixtureTransport struct {
	providers http.RoundTripper
	public    http.RoundTripper
}

func (transport e2eFixtureTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	host := strings.ToLower(request.URL.Hostname())
	if net.ParseIP(host) != nil || host == "localhost" || strings.HasSuffix(host, ".test") {
		return transport.public.RoundTrip(request)
	}
	return transport.providers.RoundTrip(request)
}
