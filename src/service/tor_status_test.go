package service

import (
	"bufio"
	"net"
	"net/textproto"
	"testing"
	"time"

	"github.com/cretz/bine/control"
	"github.com/cretz/bine/tor"
)

func TestTorControlStatusOmitsUnavailableFields(t *testing.T) {
	ts := newTestTorService(t, nil)
	status := ts.ControlStatus()
	for _, field := range []string{"running", "version", "bootstrap_percent", "circuit_established", "vanity"} {
		if _, ok := status[field]; !ok {
			t.Errorf("ControlStatus() omitted required field %q", field)
		}
	}
	for _, field := range []string{"pid", "onion_address", "enabled", "backend_port", "circuits", "status", "outbound_enabled", "hostname"} {
		if _, ok := status[field]; ok {
			t.Errorf("ControlStatus() included unavailable or non-contract field %q", field)
		}
	}
	vanity, ok := status["vanity"].(map[string]any)
	if !ok {
		t.Fatalf("vanity status has type %T, want map[string]any", status["vanity"])
	}
	if vanity["state"] != "idle" {
		t.Errorf("vanity state = %v, want idle", vanity["state"])
	}
	if _, ok := vanity["candidates"].([]string); !ok {
		t.Errorf("vanity candidates have type %T, want []string", vanity["candidates"])
	}
}

func TestTorControlStatusDoesNotHoldLifecycleLockDuringControlIO(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	t.Cleanup(func() { _ = serverConn.Close() })
	t.Cleanup(func() { _ = clientConn.Close() })
	requestRead := make(chan struct{})
	go func() {
		_, _ = bufio.NewReader(serverConn).ReadString('\n')
		close(requestRead)
	}()

	ts := newTestTorService(t, nil)
	ts.running = true
	ts.tor = &tor.Tor{Control: control.NewConn(textproto.NewConn(clientConn))}
	statusDone := make(chan map[string]any, 1)
	go func() { statusDone <- ts.ControlStatus() }()

	select {
	case <-requestRead:
	case <-time.After(time.Second):
		t.Fatal("Tor status did not issue its control request")
	}
	if !ts.mu.TryLock() {
		t.Fatal("Tor status held the lifecycle lock while waiting for control I/O")
	}
	ts.mu.Unlock()

	select {
	case status := <-statusDone:
		if status["running"] != true {
			t.Errorf("ControlStatus().running = %v, want true", status["running"])
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Tor status did not return after its control timeout")
	}
}

func TestTorBootstrapPercent(t *testing.T) {
	tests := []struct {
		name   string
		status string
		want   int
	}{
		{name: "partial", status: `NOTICE BOOTSTRAP PROGRESS=42 TAG=loading`, want: 42},
		{name: "complete", status: `NOTICE BOOTSTRAP PROGRESS=100 TAG=done`, want: 100},
		{name: "invalid", status: `NOTICE BOOTSTRAP PROGRESS=unknown`, want: 0},
		{name: "missing", status: `NOTICE BOOTSTRAP TAG=starting`, want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := torBootstrapPercent(tt.status); got != tt.want {
				t.Errorf("torBootstrapPercent(%q) = %d, want %d", tt.status, got, tt.want)
			}
		})
	}
}

func TestTorCircuitEstablished(t *testing.T) {
	if !torCircuitEstablished("7 BUILT $relay PURPOSE=GENERAL\n8 LAUNCHED $other PURPOSE=GENERAL") {
		t.Error("torCircuitEstablished() = false, want true for a BUILT circuit")
	}
	if torCircuitEstablished("8 LAUNCHED $other PURPOSE=GENERAL") {
		t.Error("torCircuitEstablished() = true, want false without a BUILT circuit")
	}
}
