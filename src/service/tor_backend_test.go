package service

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/apimgr/search/src/config"
)

// TestCircuitIDFromAddr covers the decode of the rendezvous circuit ID that
// HiddenServiceExportCircuitID haproxy encodes into the PROXY v1 header's
// source address (AI.md PART 31.1). This is the value that decides what identity
// a Tor user is keyed on for rate limiting and audit, so a decode that returns
// the wrong string is a privacy bug, not a cosmetic one — and the token must
// never be an IP.
func TestCircuitIDFromAddr(t *testing.T) {
	// Tor builds the source address as an fc00::/8 IPv6 address whose low 64
	// bits carry the circuit ID. These are the byte patterns that encoding
	// produces, written out so the test does not depend on strconv parsing.
	allOnes := func() (net.IP, net.Addr) {
		ip := net.ParseIP("fc00:0000:0000:0000:ffff:ffff:ffff:ffff")
		return ip, &net.TCPAddr{IP: ip, Port: 12345}
	}
	_, addr := allOnes()

	tests := []struct {
		name string
		addr net.Addr
		want string
	}{
		{"max circuit id", addr, "tor:18446744073709551615"},
		{"low bits set", &net.TCPAddr{
			IP:   net.ParseIP("fc00:0000:0000:0000:0000:0000:0000:2a"),
			Port: 1,
		}, "tor:42"},
		{"single bit", &net.TCPAddr{
			IP:   net.ParseIP("fc00:0000:0000:0000:0000:0000:0000:8000"),
			Port: 1,
		}, "tor:32768"},
		{"low 32 bits only", &net.TCPAddr{
			IP:   circuitAddr(0x0000ffff00000000),
			Port: 1,
		}, "tor:281470681743360"},
		// A value whose bits straddle the 32-bit boundary is the one a
		// hand-converted IPv6 literal gets wrong, so the address is built
		// from the ID rather than parsed from text.
		{"bits straddling 32-bit boundary", &net.TCPAddr{
			IP:   circuitAddr(0x00000001_00000001),
			Port: 1,
		}, "tor:4294967297"},
		{"zero circuit id", &net.TCPAddr{
			IP:   net.ParseIP("fc00::"),
			Port: 1,
		}, "tor:0"},

		// Everything outside fc00::/8 is not a Tor circuit export. These must
		// all fall back to the sentinel rather than being decoded as a
		// circuit ID, or an arbitrary address becomes a usable identity.
		{"plain loopback", &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 1}, "tor:unknown"},
		{"public v4", &net.TCPAddr{IP: net.ParseIP("203.0.113.7"), Port: 1}, "tor:unknown"},
		{"public v6", &net.TCPAddr{IP: net.ParseIP("2001:db8::1"), Port: 1}, "tor:unknown"},
		// fd00::/8 (ULA) is the wrong /8; only fc00::/8 is Tor's range.
		{"fd00 unique local", &net.TCPAddr{IP: net.ParseIP("fd00::2a"), Port: 1}, "tor:unknown"},
		{"unrelated unix socket", &net.UnixAddr{Name: "/tmp/tor.sock", Net: "unix"}, "tor:unknown"},

		// A v4-mapped address has a 16-byte form whose first byte is not 0xfc.
		{"v4 mapped", &net.TCPAddr{IP: net.ParseIP("::ffff:203.0.113.7"), Port: 1}, "tor:unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := circuitIDFromAddr(tt.addr); got != tt.want {
				t.Errorf("circuitIDFromAddr(%v) = %q, want %q", tt.addr, got, tt.want)
			}
		})
	}
}

// TestCircuitIDFromAddrNeverReturnsAnIP states the property the decode table
// above only samples. A circuit token that is a bare address would defeat the
// whole mechanism: the point of exporting the circuit ID is that the identity
// recorded server-side is not a network address at all.
func TestCircuitIDFromAddrNeverReturnsAnIP(t *testing.T) {
	addresses := []net.Addr{
		&net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 1},
		&net.TCPAddr{IP: net.ParseIP("203.0.113.7"), Port: 1},
		&net.TCPAddr{IP: net.ParseIP("fc00::2a"), Port: 1},
		&net.TCPAddr{IP: net.ParseIP("2001:db8::1"), Port: 1},
		&net.TCPAddr{IP: nil, Port: 1},
		&net.UnixAddr{Name: "/tmp/sock", Net: "unix"},
	}

	for _, addr := range addresses {
		got := circuitIDFromAddr(addr)
		if got == "" {
			t.Errorf("circuitIDFromAddr(%v) is empty; callers cannot distinguish it from success", addr)
		}
		if !strings.HasPrefix(got, "tor:") {
			t.Errorf("circuitIDFromAddr(%v) = %q, want a tor:-prefixed token", addr, got)
		}
		// A sentinel of "tor:unknown" is fine; anything containing a colon
		// followed by digits is the decoded value, which is what we want.
		if strings.Count(got, ".") > 0 {
			t.Errorf("circuitIDFromAddr(%v) = %q looks like an address", addr, got)
		}
	}
}

// TestTorrcWiresHiddenServiceToBackendListener checks the three torrc directives
// PART 31.1 requires. HiddenServicePort pointing at the clearnet port instead of
// the dedicated backend is the failure that matters: Tor would then forward
// connections carrying a PROXY header to a listener that never parses one, and
// the HTTP server would read the header line as a malformed request.
func TestTorrcWiresHiddenServiceToBackendListener(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Server.Port = 8080
	cfg.Server.Tor.VirtualPort = 80

	ts := newTestTorService(t, cfg)
	ts.backendPort = 12345

	torrc := ts.getTorConfig()

	tests := []struct {
		name string
		want string
	}{
		{"hidden service targets the backend port",
			"HiddenServicePort 80 127.0.0.1:12345"},
		{"circuit ID export enabled", "HiddenServiceExportCircuitID haproxy"},
		{"vanguards stays on", "VanguardsLiteEnabled 1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !strings.Contains(torrc, tt.want) {
				t.Errorf("torrc is missing %q\n---\n%s", tt.want, torrc)
			}
		})
	}

	// The backend port must not have leaked into the clearnet-facing port, and
	// the clearnet port must not have leaked into HiddenServicePort.
	if strings.Contains(torrc, "HiddenServicePort 80 127.0.0.1:8080") {
		t.Error("HiddenServicePort points at the clearnet port; the PROXY header would reach a listener that does not parse it")
	}
}

// TestTorrcDefaultVirtualPort covers the fallback when virtual_port is unset:
// the generated torrc must still be valid rather than emitting "HiddenServicePort 0".
func TestTorrcDefaultVirtualPort(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Server.Port = 8080
	cfg.Server.Tor.VirtualPort = 0

	ts := newTestTorService(t, cfg)
	ts.backendPort = 12345

	torrc := ts.getTorConfig()
	if !strings.Contains(torrc, "HiddenServicePort 80 127.0.0.1:12345") {
		t.Errorf("unset virtual_port did not default to 80\n---\n%s", torrc)
	}
}

// TestTorBackendRelaysAndResolvesCircuitID drives the listener end to end: a
// client that speaks PROXY v1 with a Tor-format source address connects to the
// dedicated loopback port, the connection is relayed to the HTTP target, and
// the target's handler resolves the circuit ID from its own RemoteAddr.
//
// Both halves of the mapping are the fragile part. The relay keys the circuit
// on upstream.LocalAddr, which is what the HTTP server later sees as
// RemoteAddr — if that key were the downstream address instead, every lookup
// would miss and every Tor user would be bucketed under the "tor" sentinel.
func TestTorBackendRelaysAndResolvesCircuitID(t *testing.T) {
	const wantCircuit = uint64(0xdeadbeefcafebabe)

	// The stand-in for the clearnet HTTP listener.
	var (
		mu        sync.Mutex
		gotRemote string
	)
	target := newLocalListener(t)
	targetAddr := target.Addr().String()
	httpTarget := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		gotRemote = r.RemoteAddr
		mu.Unlock()
		fmt.Fprint(w, "ok")
	})}
	go httpTarget.Serve(target) //nolint:errcheck // closed via t.Cleanup

	// A free port for the backend listener.
	probe := newLocalListener(t)
	backendPort := probe.Addr().(*net.TCPAddr).Port
	probe.Close()

	backend, err := newTorBackend(backendPort, targetAddr)
	if err != nil {
		t.Fatalf("newTorBackend: %v", err)
	}
	t.Cleanup(func() { backend.Close() })

	// Speak PROXY v1 the way Tor does: TCP6, an fc00::/8 source address
	// carrying the circuit ID in its low 64 bits.
	conn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", backendPort))
	if err != nil {
		t.Fatalf("dial backend: %v", err)
	}
	defer conn.Close()

	// PROXY v1 spells out "PROXY <proto> <src> <dst> <sport> <dport>" — six
	// whitespace-separated tokens with the two ports LAST, not attached to each
	// address. The parser requires exactly six tokens, so the attached-port form
	// is rejected and the listener drops the connection before the request line
	// is ever read. The destination is not used for routing (the relay dials
	// b.target), but it must still be a well-formed address of the declared
	// family.
	header := fmt.Sprintf("PROXY TCP6 %s ::1 40000 %d\r\n",
		circuitAddr(wantCircuit), target.Addr().(*net.TCPAddr).Port)
	if _, err := conn.Write([]byte(header)); err != nil {
		t.Fatalf("write PROXY header: %v", err)
	}

	// A normal request; the PROXY header was consumed by the listener, so the
	// relayed stream must begin exactly at the request line.
	if _, err := conn.Write([]byte("GET / HTTP/1.1\r\nHost: x\r\nConnection: close\r\n\r\n")); err != nil {
		t.Fatalf("write request: %v", err)
	}

	resp, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	if !strings.Contains(resp, "200") {
		t.Errorf("response line = %q, want a 200", strings.TrimSpace(resp))
	}

	// Wait for the target handler to have recorded its view of the connection.
	waitFor(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return gotRemote != ""
	})

	mu.Lock()
	remote := gotRemote
	mu.Unlock()

	if token := backend.CircuitID(remote); token != fmt.Sprintf("tor:%d", wantCircuit) {
		t.Errorf("CircuitID(%q) = %q, want %q", remote, token, fmt.Sprintf("tor:%d", wantCircuit))
	}
}

// TestCircuitIDUnknownForNonTorRemoteAddr checks the lookup's miss path. A
// clearnet request shares the same HTTP target, and its RemoteAddr was never
// stored, so the resolver must return "" — the caller treats a non-empty token
// as "this is a Tor request" and would otherwise file every clearnet visitor
// under a circuit identity.
func TestCircuitIDUnknownForNonTorRemoteAddr(t *testing.T) {
	backend := &torBackend{}
	if got := backend.CircuitID("203.0.113.7:54321"); got != "" {
		t.Errorf("CircuitID for an unknown address = %q, want %q", got, "")
	}
	if got := backend.CircuitID(""); got != "" {
		t.Errorf("CircuitID for an empty address = %q, want %q", got, "")
	}
}

// circuitAddr builds the fc00::/8 IPv6 address Tor uses to export a circuit ID,
// placing the ID in the low 64 bits.
func circuitAddr(circuit uint64) net.IP {
	ip := make(net.IP, 16)
	ip[0] = 0xfc
	for i := 0; i < 8; i++ {
		ip[8+i] = byte(circuit >> (56 - 8*i))
	}
	return ip
}

// newLocalListener starts a listener on a free loopback port.
func newLocalListener(t *testing.T) net.Listener {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() })
	return ln
}

// waitFor polls cond until it holds or the test times out. The relay is
// asynchronous, so the assertion cannot simply follow the response.
func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition not met within 3s")
}
