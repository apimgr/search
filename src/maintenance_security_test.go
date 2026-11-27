package main

import (
	"errors"
	"io"
	"os"
	"strings"
	"testing"
)

func TestValidSecurityTrackingID(t *testing.T) {
	for _, id := range []string{"sec_0123456789abcdef", "sec_ABCDEF0123456789"} {
		if !validSecurityTrackingID(id) { t.Errorf("validSecurityTrackingID(%q) = false", id) }
	}
	for _, id := range []string{"", "sec_0123456789abcdeg", "sec_0123456789abcdef00", "../sec_0123456789ab"} {
		if validSecurityTrackingID(id) { t.Errorf("validSecurityTrackingID(%q) = true", id) }
	}
}

func TestSecurityMaintenanceUsageValidation(t *testing.T) {
	validID := "sec_0123456789abcdef"
	tests := []struct {
		name   string
		action string
		args   []string
		want   string
	}{
		{name: "unknown action", action: "remove", want: "unknown security action"},
		{name: "missing show id", action: "show", want: "show requires one tracking ID"},
		{name: "invalid list status", action: "list", args: []string{"--status", "unknown"}, want: "invalid status"},
		{name: "missing export id", action: "export", want: "export requires a tracking ID"},
		{name: "update without fields", action: "update", args: []string{validID}, want: "at least one field"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reason, usage, invalid := securityMaintenanceUsageError(tt.action, tt.args)
			if !invalid || !strings.Contains(reason, tt.want) {
				t.Fatalf("securityMaintenanceUsageError(%q, %q) = (%q, %q, %v), want reason containing %q", tt.action, tt.args, reason, usage, invalid, tt.want)
			}
			if !strings.HasPrefix(usage, "Usage:") {
				t.Fatalf("usage = %q, want usage hint", usage)
			}
		})
	}
	if reason, usage, invalid := securityMaintenanceUsageError("show", []string{validID}); invalid || reason != "" || usage != "" {
		t.Fatalf("valid show arguments returned (%q, %q, %v)", reason, usage, invalid)
	}
}

func TestSecurityMaintenanceUsageGoesToStderrAndExitsTwo(t *testing.T) {
	oldArgs, oldStderr, oldExit := os.Args, os.Stderr, exitFunc
	readPipe, writePipe, err := os.Pipe()
	if err != nil {
		t.Fatalf("create stderr pipe: %v", err)
	}
	os.Args = []string{"search", "--maintenance", "security", "remove"}
	os.Stderr = writePipe
	var exitCode int
	exitFunc = func(code int) { exitCode = code }
	t.Cleanup(func() {
		os.Args, os.Stderr, exitFunc = oldArgs, oldStderr, oldExit
		if err := writePipe.Close(); err != nil && !errors.Is(err, os.ErrClosed) {
			t.Errorf("close stderr writer: %v", err)
		}
		if err := readPipe.Close(); err != nil && !errors.Is(err, os.ErrClosed) {
			t.Errorf("close stderr reader: %v", err)
		}
	})

	stdout := captureStdout(t, func() { runMaintenance("security") })
	if err := writePipe.Close(); err != nil {
		t.Fatalf("close stderr writer: %v", err)
	}
	stderr, err := io.ReadAll(readPipe)
	if err != nil {
		t.Fatalf("read stderr: %v", err)
	}
	if exitCode != 2 {
		t.Fatalf("exit code = %d, want 2", exitCode)
	}
	if stdout != "" {
		t.Fatalf("usage error wrote to stdout: %q", stdout)
	}
	if !strings.Contains(string(stderr), "Error: unknown security action") || !strings.Contains(string(stderr), "Usage:") {
		t.Fatalf("stderr = %q, want problem and usage hint", stderr)
	}
}
