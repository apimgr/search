package email

// email_extra_test.go adds targeted coverage for sendMail, TestConnection,
// and tryDetectSMTP inner execution paths using a real fake SMTP TCP server.
//
// Coverage targets:
//   - sendMail: plain (none) mode — full path through Mail/Rcpt/Data/Write/Close/Quit
//   - sendMail: auto mode without STARTTLS advertised
//   - sendMail: with auth credentials (AUTH extension advertised)
//   - sendMail: multiple recipients (To + CC + BCC)
//   - sendMail: HTML body via Send()
//   - sendMail: custom headers via Send()
//   - sendMail: tlsDial error path (tls mode, dial returns error)
//   - sendMail: connection refused (unreachable server)
//   - sendMail: non-SMTP greeting (smtpNewClient fails)
//   - TestConnection: plain success, auth success, auto no-STARTTLS, TLS dial error, refused
//   - tryDetectSMTP: success with AUTH, success with STARTTLS, no server, TLS mode dial fails
//   - SendToAdmins / SendAlert / SendSecurityAlert happy paths via fake server
//   - getDefaultGateway: interfaces error, loopback-only
//   - DetectAndConfigure: no-server path returns DefaultConfig

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"
)

// fakeServer implements a minimal SMTP server over a plain TCP connection.
// It speaks just enough ESMTP for smtp.NewClient to handshake, then handles
// MAIL/RCPT/DATA/AUTH/QUIT commands.
type fakeServer struct {
	supportsSTARTTLS bool
	supportsAUTH     bool
	lastUnknownCmd   string
}

func (fs *fakeServer) serve(conn net.Conn) {
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(10 * time.Second))

	send := func(s string) {
		conn.Write([]byte(s + "\r\n"))
	}

	rd := bufio.NewReader(conn)
	// readline reads one line, trims CRLF, returns (line, nil) on success.
	// Returns ("", err) on read error. Blank lines in email content come back as ("", nil).
	readline := func() (string, error) {
		conn.SetReadDeadline(time.Now().Add(10 * time.Second))
		line, err := rd.ReadString('\n')
		if err != nil {
			return "", err
		}
		return strings.TrimRight(line, "\r\n"), nil
	}

	send("220 fakesmtp.local ESMTP ready")

	line, err := readline()
	if err != nil {
		return
	}
	upper := strings.ToUpper(line)
	if !strings.HasPrefix(upper, "EHLO") && !strings.HasPrefix(upper, "HELO") {
		send("500 expected EHLO")
		return
	}

	// Build capability response. Last line must not have a dash.
	var caps []string
	caps = append(caps, "250-fakesmtp.local Hello")
	if fs.supportsSTARTTLS {
		caps = append(caps, "250-STARTTLS")
	}
	if fs.supportsAUTH {
		caps = append(caps, "250-AUTH PLAIN LOGIN")
	}
	caps = append(caps, "250 OK")
	for _, c := range caps {
		send(c)
	}

	for {
		line, err = readline()
		if err != nil {
			return
		}
		upper = strings.ToUpper(line)
		switch {
		case strings.HasPrefix(upper, "STARTTLS"):
			// We don't actually upgrade TLS; send 220 then close.
			send("220 Go ahead")
			return
		case strings.HasPrefix(upper, "AUTH"):
			// PLAIN auth sends credentials inline (AUTH PLAIN <base64>).
			// LOGIN auth requires a challenge/response cycle.
			if strings.Contains(upper, "PLAIN") && len(line) > len("AUTH PLAIN ") {
				send("235 OK auth accepted")
			} else {
				send("334 ")
				readline()
				send("235 OK auth accepted")
			}
		case strings.HasPrefix(upper, "MAIL FROM"):
			send("250 OK")
		case strings.HasPrefix(upper, "RCPT TO"):
			send("250 OK")
		case strings.HasPrefix(upper, "DATA"):
			send("354 Start input")
			for {
				l, rerr := readline()
				if rerr != nil {
					return
				}
				if l == "." {
					break
				}
			}
			send("250 OK queued")
		case strings.HasPrefix(upper, "QUIT"):
			send("221 Bye")
			return
		case strings.HasPrefix(upper, "RSET"):
			send("250 OK")
		default:
			fs.lastUnknownCmd = line
			send("502 not implemented")
		}
	}
}

// startFakeServer binds a random port and handles connections in goroutines.
// Returns (host, port, closeFunc).
func startFakeServer(fs *fakeServer) (host string, port int, stop func()) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(fmt.Sprintf("startFakeServer listen: %v", err))
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go fs.serve(conn)
		}
	}()
	h, p := splitTestAddr(ln.Addr().String())
	return h, p, func() {
		ln.Close()
		<-done
	}
}

func splitTestAddr(addr string) (string, int) {
	h, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		panic(err)
	}
	var p int
	fmt.Sscanf(portStr, "%d", &p)
	return h, p
}

// newTestMailer is a helper that builds a Mailer pointing at the fake server.
func newTestMailer(host string, port int, tlsMode, user, pass string) *Mailer {
	return NewMailer(&Config{
		Enabled: true,
		SMTP: SMTPConfig{
			Host:     host,
			Port:     port,
			TLS:      tlsMode,
			Username: user,
			Password: pass,
		},
		From: FromConfig{Name: "Test", Email: "test@example.com"},
	})
}

// ---- sendMail via Send() happy paths ----

func TestSendMailSuccessPlain(t *testing.T) {
	host, port, stop := startFakeServer(&fakeServer{})
	defer stop()

	m := newTestMailer(host, port, "none", "", "")
	msg := NewMessage([]string{"to@example.com"}, "Subject", "Body text")
	if err := m.Send(msg); err != nil {
		t.Fatalf("Send() plain error = %v", err)
	}
}

func TestSendMailSuccessAutoNoSTARTTLS(t *testing.T) {
	// Server does not advertise STARTTLS — auto mode should still succeed.
	host, port, stop := startFakeServer(&fakeServer{})
	defer stop()

	m := newTestMailer(host, port, "auto", "", "")
	msg := NewMessage([]string{"to@example.com"}, "Auto", "body")
	if err := m.Send(msg); err != nil {
		t.Fatalf("Send() auto (no STARTTLS) error = %v", err)
	}
}

func TestSendMailSuccessWithAuth(t *testing.T) {
	host, port, stop := startFakeServer(&fakeServer{supportsAUTH: true})
	defer stop()

	m := newTestMailer(host, port, "none", "user", "pass")
	msg := NewMessage([]string{"to@example.com"}, "Auth test", "body")
	if err := m.Send(msg); err != nil {
		t.Fatalf("Send() with auth error = %v", err)
	}
}

func TestSendMailMultipleRecipients(t *testing.T) {
	host, port, stop := startFakeServer(&fakeServer{})
	defer stop()

	m := newTestMailer(host, port, "none", "", "")
	msg := NewMessage([]string{"to@example.com"}, "Multi", "body")
	msg.CC = []string{"cc@example.com"}
	msg.BCC = []string{"bcc@example.com"}
	if err := m.Send(msg); err != nil {
		t.Fatalf("Send() multi-recipient error = %v", err)
	}
}

func TestSendMailHTMLBody(t *testing.T) {
	host, port, stop := startFakeServer(&fakeServer{})
	defer stop()

	m := newTestMailer(host, port, "none", "", "")
	msg := NewMessage([]string{"to@example.com"}, "HTML", "plain text")
	msg.SetHTML("<h1>Hello</h1>")
	if err := m.Send(msg); err != nil {
		t.Fatalf("Send() HTML body error = %v", err)
	}
}

func TestSendMailCustomHeaders(t *testing.T) {
	host, port, stop := startFakeServer(&fakeServer{})
	defer stop()

	m := newTestMailer(host, port, "none", "", "")
	msg := NewMessage([]string{"to@example.com"}, "Custom Headers", "body")
	msg.Headers["X-Custom"] = "value"
	msg.Headers["X-Priority"] = "1"
	if err := m.Send(msg); err != nil {
		t.Fatalf("Send() custom headers error = %v", err)
	}
}

// ---- sendMail error paths ----

func TestSendMailConnectionRefused(t *testing.T) {
	m := newTestMailer("127.0.0.1", 19999, "none", "", "")
	msg := NewMessage([]string{"to@example.com"}, "test", "body")
	err := m.Send(msg)
	if err == nil {
		t.Fatal("Send() should fail when server is unreachable")
	}
	if !strings.Contains(err.Error(), "failed to connect") {
		t.Errorf("error = %q, want 'failed to connect' substring", err)
	}
}

func TestSendMailTLSDialError(t *testing.T) {
	orig := tlsDial
	defer func() { tlsDial = orig }()

	tlsDial = func(network, addr string, config *tls.Config) (*tls.Conn, error) {
		return nil, fmt.Errorf("tls: handshake timeout")
	}

	m := newTestMailer("localhost", 465, "tls", "", "")
	msg := NewMessage([]string{"to@example.com"}, "test", "body")
	err := m.Send(msg)
	if err == nil {
		t.Fatal("Send() should return error when tlsDial fails")
	}
	if !strings.Contains(err.Error(), "failed to connect") {
		t.Errorf("error = %q, want 'failed to connect' substring", err)
	}
}

func TestSendMailNonSMTPGreeting(t *testing.T) {
	// Server sends garbage so smtp.NewClient fails.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		conn.Write([]byte("NOT-SMTP garbage\r\n"))
		conn.Close()
	}()

	host, port := splitTestAddr(ln.Addr().String())
	m := newTestMailer(host, port, "none", "", "")
	msg := NewMessage([]string{"to@example.com"}, "test", "body")
	if err := m.Send(msg); err == nil {
		t.Fatal("Send() should return error when server sends non-SMTP greeting")
	}
}

// ---- TestConnection paths ----

func TestConnectionSuccessPlain(t *testing.T) {
	host, port, stop := startFakeServer(&fakeServer{})
	defer stop()

	m := &Mailer{config: &Config{
		Enabled: true,
		SMTP:    SMTPConfig{Host: host, Port: port, TLS: "none"},
	}}
	if err := m.TestConnection(); err != nil {
		t.Fatalf("TestConnection() plain error = %v", err)
	}
}

func TestConnectionSuccessWithAuth(t *testing.T) {
	host, port, stop := startFakeServer(&fakeServer{supportsAUTH: true})
	defer stop()

	m := &Mailer{config: &Config{
		Enabled: true,
		SMTP:    SMTPConfig{Host: host, Port: port, TLS: "none", Username: "u", Password: "p"},
	}}
	if err := m.TestConnection(); err != nil {
		t.Fatalf("TestConnection() auth error = %v", err)
	}
}

func TestConnectionAutoNoSTARTTLS(t *testing.T) {
	host, port, stop := startFakeServer(&fakeServer{})
	defer stop()

	m := &Mailer{config: &Config{
		Enabled: true,
		SMTP:    SMTPConfig{Host: host, Port: port, TLS: "auto"},
	}}
	if err := m.TestConnection(); err != nil {
		t.Fatalf("TestConnection() auto (no STARTTLS) error = %v", err)
	}
}

func TestConnectionTLSDialError(t *testing.T) {
	orig := tlsDial
	defer func() { tlsDial = orig }()

	tlsDial = func(network, addr string, config *tls.Config) (*tls.Conn, error) {
		return nil, fmt.Errorf("tls: certificate expired")
	}

	m := &Mailer{config: &Config{
		Enabled: true,
		SMTP:    SMTPConfig{Host: "localhost", Port: 465, TLS: "tls"},
	}}
	err := m.TestConnection()
	if err == nil {
		t.Fatal("TestConnection() should return error when tlsDial fails")
	}
	if !strings.Contains(err.Error(), "connection failed") {
		t.Errorf("error = %q, want 'connection failed' substring", err)
	}
}

func TestConnectionRefused(t *testing.T) {
	m := &Mailer{config: &Config{
		Enabled: true,
		SMTP:    SMTPConfig{Host: "127.0.0.1", Port: 19998, TLS: "none"},
	}}
	err := m.TestConnection()
	if err == nil {
		t.Fatal("TestConnection() should return error when server unreachable")
	}
	if !strings.Contains(err.Error(), "connection failed") {
		t.Errorf("error = %q, want 'connection failed' substring", err)
	}
}

// ---- tryDetectSMTP paths ----

func TestTryDetectSMTPSuccess(t *testing.T) {
	host, port, stop := startFakeServer(&fakeServer{supportsAUTH: true})
	defer stop()

	result := tryDetectSMTP(host, port, false)
	if result == nil {
		t.Fatal("tryDetectSMTP() should return non-nil when server responds")
	}
	if result.Host != host {
		t.Errorf("Host = %q, want %q", result.Host, host)
	}
	if result.Port != port {
		t.Errorf("Port = %d, want %d", result.Port, port)
	}
	if !result.AuthRequired {
		t.Error("AuthRequired = false, want true (server advertises AUTH)")
	}
}

func TestTryDetectSMTPSuccessWithSTARTTLS(t *testing.T) {
	host, port, stop := startFakeServer(&fakeServer{supportsSTARTTLS: true})
	defer stop()

	result := tryDetectSMTP(host, port, false)
	if result == nil {
		t.Fatal("tryDetectSMTP() should return non-nil when server responds")
	}
}

func TestTryDetectSMTPNoServer(t *testing.T) {
	result := tryDetectSMTP("127.0.0.1", 19997, false)
	if result != nil {
		t.Errorf("tryDetectSMTP() = %+v, want nil (no server listening)", result)
	}
}

func TestTryDetectSMTPTLSDialFails(t *testing.T) {
	orig := tlsDialWithDialer
	defer func() { tlsDialWithDialer = orig }()

	tlsDialWithDialer = func(d *net.Dialer, network, addr string, config *tls.Config) (*tls.Conn, error) {
		return nil, fmt.Errorf("tls refused")
	}

	result := tryDetectSMTP("127.0.0.1", 465, true)
	if result != nil {
		t.Errorf("tryDetectSMTP() = %+v, want nil when TLS dial fails", result)
	}
}

// ---- SendToAdmins / SendAlert / SendSecurityAlert via fake server ----

func TestSendToAdminsSuccess(t *testing.T) {
	fs := &fakeServer{}
	host, port, stop := startFakeServer(fs)
	defer stop()

	m := NewMailer(&Config{
		Enabled:     true,
		SMTP:        SMTPConfig{Host: host, Port: port, TLS: "none"},
		From:        FromConfig{Email: "sys@example.com"},
		AdminEmails: []string{"admin@example.com"},
	})
	if err := m.SendToAdmins("Test Subject", "Test body"); err != nil {
		t.Logf("lastUnknownCmd = %q", fs.lastUnknownCmd)
		t.Fatalf("SendToAdmins() error = %v", err)
	}
}

func TestSendAlertSuccess(t *testing.T) {
	host, port, stop := startFakeServer(&fakeServer{})
	defer stop()

	m := NewMailer(&Config{
		Enabled:     true,
		SMTP:        SMTPConfig{Host: host, Port: port, TLS: "none"},
		From:        FromConfig{Email: "noreply@example.com"},
		AdminEmails: []string{"ops@example.com"},
	})
	if err := m.SendAlert("CPU High", "CPU usage is above 90%"); err != nil {
		t.Fatalf("SendAlert() error = %v", err)
	}
}

func TestSendSecurityAlertSuccess(t *testing.T) {
	host, port, stop := startFakeServer(&fakeServer{})
	defer stop()

	m := NewMailer(&Config{
		Enabled:     true,
		SMTP:        SMTPConfig{Host: host, Port: port, TLS: "none"},
		From:        FromConfig{Email: "sec@example.com"},
		AdminEmails: []string{"security@example.com"},
	})
	if err := m.SendSecurityAlert("brute force", "1.2.3.4", "50 failed logins"); err != nil {
		t.Fatalf("SendSecurityAlert() error = %v", err)
	}
}

// ---- getDefaultGateway extra paths ----

// TestGetDefaultGatewayInterfacesError covers the error return from netInterfaces.
func TestGetDefaultGatewayInterfacesError(t *testing.T) {
	orig := netInterfaces
	defer func() { netInterfaces = orig }()

	netInterfaces = func() ([]net.Interface, error) {
		return nil, fmt.Errorf("netlink error")
	}

	result := getDefaultGateway()
	if result != "" {
		t.Errorf("getDefaultGateway() = %q, want empty string on interfaces error", result)
	}
}

// TestGetDefaultGatewayLoopbackOnly covers the path where all interfaces are
// loopback or down — no usable IPv4 address found.
func TestGetDefaultGatewayLoopbackOnly(t *testing.T) {
	orig := netInterfaces
	defer func() { netInterfaces = orig }()

	netInterfaces = func() ([]net.Interface, error) {
		return []net.Interface{
			{
				Index: 1,
				Name:  "lo",
				Flags: net.FlagUp | net.FlagLoopback,
			},
		}, nil
	}

	result := getDefaultGateway()
	if result != "" {
		t.Errorf("getDefaultGateway() = %q, want empty string for loopback-only", result)
	}
}

// ---- DetectAndConfigure paths ----

// TestDetectAndConfigureNoServer covers DetectAndConfigure returning
// DefaultConfig when no SMTP server is reachable.
func TestDetectAndConfigureNoServer(t *testing.T) {
	orig := netDialTimeout
	defer func() { netDialTimeout = orig }()

	origTLS := tlsDialWithDialer
	defer func() { tlsDialWithDialer = origTLS }()

	netDialTimeout = func(network, addr string, timeout time.Duration) (net.Conn, error) {
		return nil, fmt.Errorf("blocked")
	}
	tlsDialWithDialer = func(d *net.Dialer, network, addr string, cfg *tls.Config) (*tls.Conn, error) {
		return nil, fmt.Errorf("blocked")
	}

	cfg := DetectAndConfigure("", "")
	if cfg == nil {
		t.Fatal("DetectAndConfigure() should return non-nil config")
	}
	if cfg.Enabled {
		t.Error("DetectAndConfigure() Enabled = true, want false (no server found)")
	}
}
