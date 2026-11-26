package server

import (
	"bufio"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/apimgr/search/src/config"
	"github.com/apimgr/search/src/logging"
)

func TestLogConsentUpdatedRecordsPolicyVersionAndAnonymizedAddress(t *testing.T) {
	logDir := filepath.Join(newProjectTempDir(t), "logs")
	logManager := logging.NewManager(logDir)
	t.Cleanup(func() {
		if err := logManager.Close(); err != nil {
			t.Errorf("close log manager: %v", err)
		}
	})

	cfg := config.DefaultConfig()
	cfg.Server.Privacy.Consent.Policy.Version = "2026-01"
	s := &Server{config: cfg, logManager: logManager}
	r := httptest.NewRequest("POST", "/server/consent", nil)
	r.RemoteAddr = "203.0.113.44:12345"
	s.logConsentUpdated(r, map[string]bool{
		"essential":   true,
		"preferences": false,
		"analytics":   false,
	})

	file, err := os.Open(filepath.Join(logDir, "audit.log"))
	if err != nil {
		t.Fatalf("open audit log: %v", err)
	}
	defer file.Close()

	var entry logging.AuditEntry
	if err := json.NewDecoder(bufio.NewReader(file)).Decode(&entry); err != nil {
		t.Fatalf("decode audit entry: %v", err)
	}
	if entry.Event != logging.AuditActionConsentUpdated {
		t.Errorf("event = %q, want %q", entry.Event, logging.AuditActionConsentUpdated)
	}
	if entry.Actor.Type != "anonymous" || entry.Actor.ID != "" {
		t.Errorf("anonymous actor = %+v, want anonymous with no ID", entry.Actor)
	}
	if entry.Actor.IP != "203.0.113.0" {
		t.Errorf("actor IP = %q, want anonymized address 203.0.113.0", entry.Actor.IP)
	}
	if entry.Details["policy_version"] != "2026-01" {
		t.Errorf("policy_version = %v, want 2026-01", entry.Details["policy_version"])
	}
	if entry.Details["consent_type"] != "cookie_categories" {
		t.Errorf("consent_type = %v, want cookie_categories", entry.Details["consent_type"])
	}
}

func TestAnonymizeConsentIP(t *testing.T) {
	tests := []struct {
		name string
		ip   string
		want string
	}{
		{name: "IPv4", ip: "192.0.2.129", want: "192.0.2.0"},
		{name: "IPv6", ip: "2001:db8:1234:5678:9abc:def0:1234:5678", want: "2001:db8:1234::"},
		{name: "mapped IPv4", ip: "::ffff:192.0.2.129", want: "192.0.2.0"},
		{name: "invalid", ip: "not-an-ip", want: ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := anonymizeConsentIP(test.ip); got != test.want {
				t.Errorf("anonymizeConsentIP(%q) = %q, want %q", test.ip, got, test.want)
			}
		})
	}
}
