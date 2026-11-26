package health

import "testing"

func TestTorStatePublicContract(t *testing.T) {
	tests := []struct {
		name                       string
		enabled, starting, running bool
		state, status              string
		wantEnabled, wantRunning   bool
	}{
		{"disabled", false, false, false, "disabled", "disabled", false, false},
		{"starting", true, true, false, "starting", "starting", true, false},
		{"healthy", true, false, true, "running", "healthy", true, true},
		{"unavailable", true, false, false, "error", "unavailable", true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TorState(tt.enabled, tt.starting, tt.running, "safe.onion")
			if got.State != tt.state || got.Status != tt.status || got.Enabled != tt.wantEnabled || got.Running != tt.wantRunning {
				t.Fatalf("TorState() = %#v", got)
			}
			if got.Status != "healthy" && got.Hostname != "" {
				t.Fatalf("non-healthy Tor leaked hostname: %#v", got)
			}
		})
	}
}

func TestI2PStatePublicContract(t *testing.T) {
	tests := []struct {
		state, status    string
		enabled, running bool
	}{
		{"disabled", "disabled", false, false},
		{"no_provider", "disabled", false, false},
		{"starting", "starting", true, false},
		{"building", "starting", true, false},
		{"running", "healthy", true, true},
		{"error", "unavailable", true, false},
		{"error: /private/path", "disabled", false, false},
	}
	for _, tt := range tests {
		t.Run(tt.state, func(t *testing.T) {
			got := I2PState(tt.state, "i2pd", "safe.b32.i2p")
			if got.State != tt.state && tt.state != "error: /private/path" {
				t.Fatalf("state = %q", got.State)
			}
			if got.Status != tt.status || got.Enabled != tt.enabled || got.Running != tt.running {
				t.Fatalf("I2PState() = %#v", got)
			}
			if got.Status != "healthy" && got.Hostname != "" {
				t.Fatalf("non-healthy I2P leaked hostname: %#v", got)
			}
			if !got.Enabled && got.Provider != "none" {
				t.Fatalf("disabled I2P provider = %q, want none", got.Provider)
			}
		})
	}
	if got := I2PState("error", "/private/provider/path", "/private/hostname"); got.Provider != "" || got.Hostname != "" {
		t.Fatalf("I2P error state leaked details: %#v", got)
	}
}
