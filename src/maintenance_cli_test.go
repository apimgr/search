package main

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/apimgr/search/src/config"
	"github.com/apimgr/search/src/logging"
)

func captureMaintenanceOutput(t *testing.T, fn func()) (string, string, int) {
	t.Helper()

	originalStdout, originalStderr, originalExit := os.Stdout, os.Stderr, exitFunc
	stdoutReader, stdoutWriter, err := os.Pipe()
	if err != nil {
		t.Fatalf("create stdout pipe: %v", err)
	}
	stderrReader, stderrWriter, err := os.Pipe()
	if err != nil {
		_ = stdoutReader.Close()
		_ = stdoutWriter.Close()
		t.Fatalf("create stderr pipe: %v", err)
	}
	os.Stdout, os.Stderr = stdoutWriter, stderrWriter
	exitCode := 0
	exitFunc = func(code int) { exitCode = code }
	t.Cleanup(func() {
		os.Stdout, os.Stderr, exitFunc = originalStdout, originalStderr, originalExit
		_ = stdoutReader.Close()
		_ = stdoutWriter.Close()
		_ = stderrReader.Close()
		_ = stderrWriter.Close()
	})

	stdoutDone := make(chan string, 1)
	stderrDone := make(chan string, 1)
	go func() {
		data, _ := io.ReadAll(stdoutReader)
		stdoutDone <- string(data)
	}()
	go func() {
		data, _ := io.ReadAll(stderrReader)
		stderrDone <- string(data)
	}()

	fn()
	_ = stdoutWriter.Close()
	_ = stderrWriter.Close()
	stdout, stderr := <-stdoutDone, <-stderrDone
	return stdout, stderr, exitCode
}

func TestMaintenanceUsageErrorsUseStderrAndExitTwo(t *testing.T) {
	tests := []struct {
		name string
		call func()
	}{
		{
			name: "unknown secret action",
			call: func() { runMaintenanceSecret("unknown", "") },
		},
		{
			name: "unknown secret name",
			call: func() { runMaintenanceSecret("rotate", "unknown_secret") },
		},
		{
			name: "unknown token action",
			call: func() { runMaintenanceToken("unknown", "") },
		},
		{
			name: "missing token prefix",
			call: func() { runMaintenanceToken("revoke", "") },
		},
		{
			name: "unknown data action",
			call: func() { runMaintenanceData("unknown", nil) },
		},
		{
			name: "malformed data options",
			call: func() { runMaintenanceData("export", []string{"--unknown"}) },
		},
		{
			name: "unknown compliance action",
			call: func() { runMaintenanceCompliance("unknown") },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdout, stderr, exitCode := captureMaintenanceOutput(t, tt.call)
			if stdout != "" {
				t.Errorf("usage error wrote to stdout: %q", stdout)
			}
			if strings.TrimSpace(stderr) == "" {
				t.Error("usage error did not write a diagnostic and usage to stderr")
			}
			if exitCode != 2 {
				t.Errorf("usage error exit code = %d, want 2", exitCode)
			}
		})
	}
}

func TestParseMaintenanceEmailArgs(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantDryRun bool
		wantTo     string
		wantErr    bool
	}{
		{name: "defaults", args: []string{}},
		{name: "dry run", args: []string{"--dry-run"}, wantDryRun: true},
		{name: "recipient", args: []string{"--to", "operator@example.com"}, wantTo: "operator@example.com"},
		{name: "combined", args: []string{"--to", "operator@example.com", "--dry-run"}, wantDryRun: true, wantTo: "operator@example.com"},
		{name: "missing recipient", args: []string{"--to"}, wantErr: true},
		{name: "unknown option", args: []string{"--force"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotDryRun, gotTo, err := parseMaintenanceEmailArgs(tt.args)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseMaintenanceEmailArgs() error = %v, wantErr %t", err, tt.wantErr)
			}
			if gotDryRun != tt.wantDryRun || gotTo != tt.wantTo {
				t.Fatalf("parseMaintenanceEmailArgs() = (%t, %q), want (%t, %q)", gotDryRun, gotTo, tt.wantDryRun, tt.wantTo)
			}
		})
	}
}

func TestMaintenanceAPIURL(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Server.FQDN = "search.example.com"
	cfg.Server.Port = 8080
	got, err := maintenanceAPIURL(cfg)
	if err != nil {
		t.Fatalf("maintenanceAPIURL() error = %v", err)
	}
	if want := "http://127.0.0.1:8080"; got != want {
		t.Fatalf("maintenanceAPIURL() = %q, want %q", got, want)
	}
	cfg.Server.HTTPSPort = 8443
	cfg.Server.BaseURL = "/search"
	got, err = maintenanceAPIURL(cfg)
	if err != nil {
		t.Fatalf("maintenanceAPIURL() dual-port error = %v", err)
	}
	if want := "https://search.example.com:8443/search"; got != want {
		t.Fatalf("maintenanceAPIURL() dual-port = %q, want %q", got, want)
	}
}

func TestMaintenanceAuditActorIsOperator(t *testing.T) {
	if got := maintenanceAuditActor().Type; got != "operator" {
		t.Fatalf("maintenanceAuditActor().Type = %q, want operator", got)
	}
}

func TestSchedulerTaskStateAuditUsesCanonicalEventsAndTransitionDetails(t *testing.T) {
	for _, tt := range []struct {
		action     string
		oldEnabled bool
		newEnabled bool
		wantEvent  logging.AuditAction
	}{
		{action: "enable", oldEnabled: false, newEnabled: true, wantEvent: logging.AuditActionSchedulerTaskEnabled},
		{action: "disable", oldEnabled: true, newEnabled: false, wantEvent: logging.AuditActionSchedulerTaskDisabled},
	} {
		event, details := schedulerTaskStateAudit(tt.action, tt.oldEnabled, tt.newEnabled)
		if event != tt.wantEvent {
			t.Errorf("schedulerTaskStateAudit(%q) event = %q, want %q", tt.action, event, tt.wantEvent)
		}
		if details["source"] != "cli" || details["old_enabled"] != tt.oldEnabled || details["new_enabled"] != tt.newEnabled {
			t.Errorf("schedulerTaskStateAudit(%q) details = %#v", tt.action, details)
		}
	}
}
