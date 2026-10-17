package service

import (
	"encoding/binary"
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync"

	"github.com/apimgr/search/src/common/closeutil"
	"github.com/pires/go-proxyproto"
)

// torBackend is the dedicated PROXY-protocol loopback listener the Tor hidden
// service forwards to, per AI.md PART 31.1 "Circuit-ID Export & PROXY-Protocol
// Backend Listener". Tor is configured with HiddenServiceExportCircuitID
// haproxy, so every connection Tor forwards carries a HAProxy PROXY v1 header
// whose source address encodes the 64-bit rendezvous circuit ID in fc00::/8.
// The public clearnet listener must never receive that header, which is why
// this listener is bound separately from the clearnet HTTP port.
type torBackend struct {
	listener net.Listener
	// Loopback address of the clearnet HTTP listener connections are relayed to
	target string
	// Maps the local address of each relayed connection (which becomes the HTTP
	// server's RemoteAddr) to its tor:{circuit_id} token
	circuits sync.Map
	closed   chan struct{}
	once     sync.Once
}

// newTorBackend binds the dedicated PROXY-protocol loopback listener on
// 127.0.0.1:port and relays accepted connections to target.
func newTorBackend(port int, target string) (*torBackend, error) {
	raw, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return nil, fmt.Errorf("bind tor backend listener: %w", err)
	}

	// REQUIRE the PROXY header: this listener exists solely for Tor, and a
	// connection without a header is not from our hidden service.
	pl := &proxyproto.Listener{
		Listener: raw,
		ConnPolicy: func(proxyproto.ConnPolicyOptions) (proxyproto.Policy, error) {
			return proxyproto.REQUIRE, nil
		},
	}

	b := &torBackend{
		listener: pl,
		target:   target,
		closed:   make(chan struct{}),
	}
	go b.serve()
	return b, nil
}

// serve accepts PROXY-wrapped connections until the listener is closed.
func (b *torBackend) serve() {
	for {
		conn, err := b.listener.Accept()
		if err != nil {
			select {
			case <-b.closed:
				return
			default:
			}
			slog.Warn("Tor backend accept failed", "err", err)
			return
		}
		go b.handle(conn)
	}
}

// handle relays one Tor connection to the clearnet loopback listener, keyed by
// its circuit ID for logging, audit trails, and rate limiting.
func (b *torBackend) handle(conn net.Conn) {
	defer closeutil.Close(conn)

	circuitID := circuitIDFromAddr(conn.RemoteAddr())

	upstream, err := net.Dial("tcp", b.target)
	if err != nil {
		slog.Warn("Tor backend upstream dial failed", "circuit", circuitID, "err", err)
		return
	}
	defer closeutil.Close(upstream)

	// The local address of the upstream connection is what the HTTP server sees
	// as RemoteAddr, so it is the key that lets request handling resolve the
	// circuit ID for this request.
	key := upstream.LocalAddr().String()
	b.circuits.Store(key, circuitID)
	defer b.circuits.Delete(key)

	slog.Debug("Tor backend connection", "circuit", circuitID)

	done := make(chan struct{}, 2)
	go func() {
		if _, err := io.Copy(upstream, conn); err != nil {
			slog.Debug("Tor client relay ended with an error", "error", err)
		}
		if c, ok := upstream.(*net.TCPConn); ok {
			if err := c.CloseWrite(); err != nil {
				slog.Debug("failed to close Tor upstream write side", "error", err)
			}
		}
		done <- struct{}{}
	}()
	go func() {
		if _, err := io.Copy(conn, upstream); err != nil {
			slog.Debug("Tor upstream relay ended with an error", "error", err)
		}
		done <- struct{}{}
	}()
	<-done
	<-done
}

// CircuitID returns the tor:{circuit_id} token for a request whose RemoteAddr
// is remoteAddr, or "" when the request did not arrive over the hidden service.
func (b *torBackend) CircuitID(remoteAddr string) string {
	if v, ok := b.circuits.Load(remoteAddr); ok {
		return v.(string)
	}
	return ""
}

// Close shuts the listener down; in-flight relays end with it.
func (b *torBackend) Close() error {
	var err error
	b.once.Do(func() {
		close(b.closed)
		err = b.listener.Close()
	})
	return err
}

// circuitIDFromAddr decodes the rendezvous circuit ID Tor encodes in the PROXY
// header source address. Tor uses an fc00::/8 IPv6 address whose low 64 bits
// carry the circuit ID; the value is an opaque per-session token, never an IP
// and never deanonymizing.
func circuitIDFromAddr(addr net.Addr) string {
	tcp, ok := addr.(*net.TCPAddr)
	if !ok || tcp.IP == nil {
		return "tor:unknown"
	}
	ip := tcp.IP.To16()
	if ip == nil || ip[0] != 0xfc {
		return "tor:unknown"
	}
	return fmt.Sprintf("tor:%d", binary.BigEndian.Uint64(ip[8:16]))
}
