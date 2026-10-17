package server

import (
	"net"
	"net/http"
	"strconv"
)

// torIdentity resolves the request's Tor identity token per AI.md PART 12
// "Tor Request Logging & Identity" (line 16109) and line 16098: for a request
// relayed from the hidden service, the TCP peer is the local Tor daemon
// (loopback), which is meaningless as a client identifier — every Tor request
// would otherwise collapse into one loopback bucket and clearnet IP rules
// would apply to it.
//
// The token is the circuit ID exported via HiddenServiceExportCircuitID
// haproxy and read from the PROXY-protocol header by the dedicated backend
// listener (src/service/tor_backend.go), formatted as "tor:{circuit_id}".
// When circuit-ID export is not enabled, the literal sentinel "tor" is used.
// The empty string means the request did not arrive over the hidden service.
//
// The token is opaque, never an IP, and never deanonymizing: wherever the app
// would key or record a client identity — rate-limit keys, audit trails — a
// Tor request must use this token, never "127.0.0.1".
func (s *Server) torIdentity(r *http.Request) string {
	if !s.torRelayed(r) {
		return ""
	}
	return s.circuitIdentity(r)
}

// circuitIdentity resolves the tor:{circuit_id} token for a relayed request
// via the TorService backend's PROXY-header mapping. Falls back to the
// sentinel "tor" when circuit-ID export is unavailable (AI.md line 16109).
func (s *Server) circuitIdentity(r *http.Request) string {
	if s.torService != nil {
		if token := s.torService.CircuitID(r.RemoteAddr); token != "" {
			return token
		}
	}
	return "tor"
}

// torRelayed reports whether the request's local address is the dedicated
// PROXY-protocol backend listener port, using the same probe as the metrics
// middleware (AI.md PART 20 tor_requests_total). A Tor relay is always
// loopback; plain loopback callers (CLI, healthcheck) are never relays because
// they connect to the clearnet listener, not the backend port.
func (s *Server) torRelayed(r *http.Request) bool {
	port := s.torBackendPort.Load()
	if port == 0 {
		return false
	}
	local, ok := r.Context().Value(http.LocalAddrContextKey).(net.Addr)
	if !ok {
		return false
	}
	_, p, err := net.SplitHostPort(local.String())
	return err == nil && p == strconv.FormatInt(port, 10)
}
