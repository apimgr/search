package server

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/apimgr/search/src/config"
	"github.com/apimgr/search/src/logging"
)

func TestMinimumComplianceAuditRetention(t *testing.T) {
	tests := []struct {
		name       string
		compliance config.ComplianceConfig
		want       time.Duration
	}{
		{name: "disabled", want: 0},
		{name: "GDPR", compliance: config.ComplianceConfig{GDPR: true}, want: 365 * 24 * time.Hour},
		{name: "HIPAA", compliance: config.ComplianceConfig{HIPAA: true}, want: 6 * 365 * 24 * time.Hour},
		{name: "strictest enabled", compliance: config.ComplianceConfig{HIPAA: true, ISO27001: true}, want: 6 * 365 * 24 * time.Hour},
		{name: "unspecified template retention", compliance: config.ComplianceConfig{FedRAMP: true}, want: 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := MinimumComplianceAuditRetention(test.compliance); got != test.want {
				t.Errorf("MinimumComplianceAuditRetention() = %s, want %s", got, test.want)
			}
		})
	}
}

func TestConfigureAuditRetentionKeepsRecentAuditForComplianceMinimum(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.log")
	audit := logging.NewAuditLogger(path)
	t.Cleanup(func() { _ = audit.Close() })
	cfg := config.DefaultConfig()
	cfg.Server.Logs.Defaults.Keep = "none"
	cfg.Server.Compliance.GDPR = true
	if err := configureAuditRetention(cfg, audit); err != nil {
		t.Fatalf("configureAuditRetention: %v", err)
	}
	audit.LogConfigChange("operator", "", "test", "value")
	if err := audit.Rotate(); err != nil {
		t.Fatalf("Rotate: %v", err)
	}
	rotated := path + "." + time.Now().Format("2006-01-02")
	if _, err := os.Stat(rotated); err != nil {
		t.Fatalf("compliance minimum removed recent rotated audit file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(path), "audit.anchor")); err != nil {
		t.Fatalf("audit anchor missing after rotation: %v", err)
	}
}
