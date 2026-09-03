package security

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBlocklistManager_IsBlocked(t *testing.T) {
	m := NewBlocklistManager("", nil)

	// Add test data directly
	m.mu.Lock()
	m.blockedIPs["192.168.1.100"] = true
	m.mu.Unlock()

	tests := []struct {
		name string
		ip   string
		want bool
	}{
		{"blocked IP", "192.168.1.100", true},
		{"not blocked", "10.0.0.1", false},
		{"invalid IP", "not-an-ip", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := m.IsBlocked(tt.ip); got != tt.want {
				t.Errorf("IsBlocked(%q) = %v, want %v", tt.ip, got, tt.want)
			}
		})
	}
}

func TestBlocklistManager_ParseBlocklist(t *testing.T) {
	m := NewBlocklistManager("", nil)

	tests := []struct {
		name     string
		input    string
		listType string
		wantIPs  int
		wantNets int
	}{
		{
			name:     "IP list",
			input:    "1.2.3.4\n5.6.7.8\n# comment\n\n9.10.11.12",
			listType: "ip",
			wantIPs:  3,
			wantNets: 0,
		},
		{
			name:     "CIDR list",
			input:    "10.0.0.0/8\n192.168.0.0/16\n# comment",
			listType: "cidr",
			wantIPs:  0,
			wantNets: 2,
		},
		{
			name:     "mixed CIDR and IP",
			input:    "10.0.0.0/8\n1.2.3.4\n",
			listType: "cidr",
			wantIPs:  1,
			wantNets: 1,
		},
		{
			name:     "IP with comments",
			input:    "1.2.3.4 # malicious\n5.6.7.8",
			listType: "ip",
			wantIPs:  2,
			wantNets: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ips, nets, err := m.parseBlocklist(strings.NewReader(tt.input), tt.listType)
			if err != nil {
				t.Fatalf("parseBlocklist() error = %v", err)
			}
			if len(ips) != tt.wantIPs {
				t.Errorf("got %d IPs, want %d", len(ips), tt.wantIPs)
			}
			if len(nets) != tt.wantNets {
				t.Errorf("got %d nets, want %d", len(nets), tt.wantNets)
			}
		})
	}
}

func TestBlocklistManager_CIDRMatch(t *testing.T) {
	m := NewBlocklistManager("", nil)

	// Parse a CIDR blocklist
	input := "10.0.0.0/8\n192.168.0.0/16"
	ips, nets, err := m.parseBlocklist(strings.NewReader(input), "cidr")
	if err != nil {
		t.Fatalf("parseBlocklist() error = %v", err)
	}

	m.mu.Lock()
	m.blockedIPs = ips
	m.blockedNets = nets
	m.mu.Unlock()

	tests := []struct {
		ip   string
		want bool
	}{
		{"10.1.2.3", true},
		{"10.255.255.255", true},
		{"192.168.1.1", true},
		{"192.168.100.50", true},
		{"11.0.0.1", false},
		{"172.16.0.1", false},
	}

	for _, tt := range tests {
		t.Run(tt.ip, func(t *testing.T) {
			if got := m.IsBlocked(tt.ip); got != tt.want {
				t.Errorf("IsBlocked(%q) = %v, want %v", tt.ip, got, tt.want)
			}
		})
	}
}

func TestBlocklistManager_LoadFromDisk(t *testing.T) {
	// Create temp directory
	tmpDir := t.TempDir()
	blocklistDir := filepath.Join(tmpDir, "security", "blocklists")
	if err := os.MkdirAll(blocklistDir, 0755); err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	// Write test blocklist file
	testFile := filepath.Join(blocklistDir, "test_list.txt")
	content := "1.2.3.4\n5.6.7.8\n# comment\n10.0.0.0/8"
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	// Create manager with source matching the file name
	sources := []BlocklistSource{
		{Name: "test_list", Type: "cidr", Enabled: true},
	}
	m := NewBlocklistManager(tmpDir, sources)

	if err := m.LoadFromDisk(); err != nil {
		t.Fatalf("LoadFromDisk() error = %v", err)
	}

	// Check counts
	ips, nets := m.Count()
	if ips != 2 {
		t.Errorf("got %d IPs, want 2", ips)
	}
	if nets != 1 {
		t.Errorf("got %d nets, want 1", nets)
	}

	// Check that IPs are actually blocked
	if !m.IsBlocked("1.2.3.4") {
		t.Error("expected 1.2.3.4 to be blocked")
	}
	if !m.IsBlocked("10.1.1.1") {
		t.Error("expected 10.1.1.1 to be blocked (CIDR match)")
	}
}

func TestBlocklistManager_LoadFromDisk_NoDir(t *testing.T) {
	m := NewBlocklistManager("/nonexistent/path", nil)
	if err := m.LoadFromDisk(); err != nil {
		t.Errorf("LoadFromDisk() should not error on missing dir, got %v", err)
	}
}

func TestBlocklistManager_Count(t *testing.T) {
	m := NewBlocklistManager("", nil)
	m.mu.Lock()
	m.blockedIPs["1.2.3.4"] = true
	m.blockedIPs["5.6.7.8"] = true
	m.mu.Unlock()

	ips, nets := m.Count()
	if ips != 2 {
		t.Errorf("Count() ips = %d, want 2", ips)
	}
	if nets != 0 {
		t.Errorf("Count() nets = %d, want 0", nets)
	}
}

func TestDefaultBlocklistSources(t *testing.T) {
	sources := DefaultBlocklistSources()
	if len(sources) == 0 {
		t.Error("DefaultBlocklistSources() returned empty list")
	}
	for _, s := range sources {
		if s.Name == "" {
			t.Error("source has empty name")
		}
		if s.URL == "" {
			t.Error("source has empty URL")
		}
		if s.Type == "" {
			t.Error("source has empty type")
		}
	}
}

func TestBlocklistManager_Update_Canceled(t *testing.T) {
	m := NewBlocklistManager(t.TempDir(), nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := m.Update(ctx)
	if err == nil {
		t.Error("Update() should return error when context is canceled")
	}
}
