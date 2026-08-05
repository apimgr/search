package engine

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	utls "github.com/refraction-networking/utls"
	"golang.org/x/net/http2"
)

// yahooRoundTripper is a dedicated http.RoundTripper for the Yahoo engine
// only. Yahoo's edge blocks requests by JA3 TLS ClientHello fingerprint
// regardless of HTTP headers/User-Agent (see TODO.AI.md), so every request
// is sent over a fresh TLS connection whose ClientHello is produced by utls
// (spoofing a real Chrome fingerprint) instead of Go's stdlib crypto/tls.
// Every other engine keeps using SharedTransport and the stock crypto/tls
// stack unchanged.
//
// Live testing found Yahoo's edge answers with an HTTP/2 SETTINGS frame
// even when the client requests HTTP/2 as one of two options, so both "h2"
// and "http/1.1" are offered via ALPN and the actual negotiated protocol
// decides how the request is sent.
type yahooRoundTripper struct {
	dialTimeout time.Duration
	h2Transport *http2.Transport
}

// newYahooRoundTripper builds a yahooRoundTripper ready for use as the
// Transport of an http.Client.
func newYahooRoundTripper() *yahooRoundTripper {
	return &yahooRoundTripper{
		dialTimeout: 10 * time.Second,
		h2Transport: &http2.Transport{},
	}
}

// RoundTrip dials and TLS-handshakes a fresh connection via utls for every
// request, then sends the request over that connection using HTTP/2 or
// HTTP/1.1 depending on what ALPN negotiated.
func (rt *yahooRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	ctx := req.Context()

	host := req.URL.Hostname()
	port := req.URL.Port()
	if port == "" {
		port = "443"
	}
	addr := net.JoinHostPort(host, port)

	dialer := &net.Dialer{Timeout: rt.dialTimeout}
	rawConn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}

	config := &utls.Config{
		ServerName: host,
		NextProtos: []string{"h2", "http/1.1"},
	}

	uConn := utls.UClient(rawConn, config, utls.HelloChrome_Auto)
	if err := uConn.HandshakeContext(ctx); err != nil {
		rawConn.Close()
		return nil, fmt.Errorf("yahoo: utls handshake failed: %w", err)
	}

	if uConn.ConnectionState().NegotiatedProtocol == http2.NextProtoTLS {
		clientConn, err := rt.h2Transport.NewClientConn(uConn)
		if err != nil {
			uConn.Close()
			return nil, fmt.Errorf("yahoo: http2 client conn failed: %w", err)
		}
		return clientConn.RoundTrip(req)
	}

	// HTTP/1.1: write the request and parse the response directly off the
	// utls connection (mirrors the pattern from utls's own examples).
	if err := req.Write(uConn); err != nil {
		uConn.Close()
		return nil, fmt.Errorf("yahoo: writing http/1.1 request failed: %w", err)
	}

	resp, err := http.ReadResponse(bufio.NewReader(uConn), req)
	if err != nil {
		uConn.Close()
		return nil, fmt.Errorf("yahoo: reading http/1.1 response failed: %w", err)
	}

	resp.Body = &connClosingBody{ReadCloser: resp.Body, conn: uConn}
	return resp, nil
}

// connClosingBody closes the underlying single-use connection once the
// response body is closed, since HTTP/1.1 responses here are read off a
// connection that yahooRoundTripper owns exclusively (no keep-alive pool).
type connClosingBody struct {
	io.ReadCloser
	conn net.Conn
}

func (b *connClosingBody) Close() error {
	err := b.ReadCloser.Close()
	if cerr := b.conn.Close(); err == nil {
		err = cerr
	}
	return err
}

// yahooTransport is the shared http.RoundTripper instance used by the
// Yahoo engine's http.Client.
var yahooTransport = newYahooRoundTripper()
