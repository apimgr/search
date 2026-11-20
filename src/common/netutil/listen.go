package netutil

import (
	"fmt"
	"log/slog"
	"net"
	"strings"
	"sync"
)

var ipv6FallbackWarning sync.Once

func JoinHostPort(address, port string) string {
	address = strings.TrimPrefix(strings.TrimSuffix(strings.TrimSpace(address), "]"), "[")
	return net.JoinHostPort(address, port)
}

// ListenTCP binds the requested address, falling back to IPv4 only when the
// default dual-stack address is unavailable on the host.
func ListenTCP(address string, port int) (net.Listener, error) {
	address = strings.TrimPrefix(strings.TrimSuffix(strings.TrimSpace(address), "]"), "[")
	addr := net.JoinHostPort(address, fmt.Sprint(port))
	listener, err := listenDualStack(addr)
	if err == nil || address != "::" || !isIPv6Unavailable(err) {
		return listener, err
	}
	ipv6FallbackWarning.Do(func() {
		slog.Warn("IPv6 is unavailable; falling back to IPv4 listen address", "address", "0.0.0.0")
	})
	return net.Listen("tcp", net.JoinHostPort("0.0.0.0", fmt.Sprint(port)))
}

func isIPv6Unavailable(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "address family not supported") ||
		strings.Contains(message, "protocol not supported") ||
		strings.Contains(message, "protocol family not supported") ||
		strings.Contains(message, "protocol not available") ||
		strings.Contains(message, "address incompatible with the requested protocol")
}
