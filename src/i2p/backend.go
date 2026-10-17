package i2p

import (
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync"
)

// i2pBackend is the dedicated loopback listener the eepsite forwards to, per
// AI.md PART 31.2 "Port Allocation". Unlike the Tor backend (PART 31.1) this
// listener is PLAIN: neither i2pd nor a SAM router prepends a PROXY-protocol
// header, so there is nothing to parse and go-proxyproto is not used. It stays
// separate from the clearnet port so eepsite traffic is always distinguishable.
type i2pBackend struct {
	listener net.Listener
	// Loopback address of the clearnet HTTP listener connections are relayed to
	target string
	// Local addresses of relayed connections, which become the HTTP server's
	// RemoteAddr, so request handling can tell an eepsite request apart
	conns  sync.Map
	closed chan struct{}
	once   sync.Once
}

// newI2PBackend binds the dedicated plain loopback listener on 127.0.0.1:port
// and relays accepted connections to target.
func newI2PBackend(port int, target string) (*i2pBackend, error) {
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return nil, fmt.Errorf("bind i2p backend listener: %w", err)
	}

	b := &i2pBackend{
		listener: listener,
		target:   target,
		closed:   make(chan struct{}),
	}
	go b.serve()
	return b, nil
}

// serve accepts eepsite connections until the listener is closed.
func (b *i2pBackend) serve() {
	for {
		conn, err := b.listener.Accept()
		if err != nil {
			select {
			case <-b.closed:
				return
			default:
			}
			slog.Warn("I2P backend accept failed", "err", err)
			return
		}
		go b.handle(conn)
	}
}

// handle relays one eepsite connection to the clearnet loopback listener.
func (b *i2pBackend) handle(conn net.Conn) {
	defer func() {
		if err := conn.Close(); err != nil {
			slog.Warn("failed to close I2P backend connection", "error", err)
		}
	}()

	upstream, err := net.Dial("tcp", b.target)
	if err != nil {
		slog.Warn("I2P backend upstream dial failed", "err", err)
		return
	}
	defer func() {
		if err := upstream.Close(); err != nil {
			slog.Warn("failed to close I2P upstream connection", "error", err)
		}
	}()

	// The local address of the upstream connection is what the HTTP server sees
	// as RemoteAddr, so it is the key that marks this request as eepsite traffic.
	key := upstream.LocalAddr().String()
	b.conns.Store(key, struct{}{})
	defer b.conns.Delete(key)

	done := make(chan struct{}, 2)
	go func() {
		if _, err := io.Copy(upstream, conn); err != nil {
			slog.Debug("I2P client relay ended with an error", "error", err)
		}
		if c, ok := upstream.(*net.TCPConn); ok {
			if err := c.CloseWrite(); err != nil {
				slog.Debug("failed to close I2P upstream write side", "error", err)
			}
		}
		done <- struct{}{}
	}()
	go func() {
		if _, err := io.Copy(conn, upstream); err != nil {
			slog.Debug("I2P upstream relay ended with an error", "error", err)
		}
		// Propagate the upstream half-close so the eepsite peer sees EOF instead
		// of waiting for the whole relay to tear down.
		if c, ok := conn.(*net.TCPConn); ok {
			if err := c.CloseWrite(); err != nil {
				slog.Debug("failed to close I2P client write side", "error", err)
			}
		}
		done <- struct{}{}
	}()
	<-done
	<-done
}

// IsEepsiteConn reports whether the request whose RemoteAddr is remoteAddr
// arrived over the eepsite rather than the clearnet listener.
func (b *i2pBackend) IsEepsiteConn(remoteAddr string) bool {
	_, ok := b.conns.Load(remoteAddr)
	return ok
}

// Port returns the loopback port the backend listener is bound to.
func (b *i2pBackend) Port() int {
	if addr, ok := b.listener.Addr().(*net.TCPAddr); ok {
		return addr.Port
	}
	return 0
}

// Close shuts the listener down; in-flight relays end with it.
func (b *i2pBackend) Close() error {
	var err error
	b.once.Do(func() {
		close(b.closed)
		err = b.listener.Close()
	})
	return err
}
