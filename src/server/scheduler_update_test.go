package server

import (
	"testing"

	"github.com/apimgr/search/src/logging"
)

func TestUpdateManualCommandPrivilegeInstructions(t *testing.T) {
	tests := []struct {
		name           string
		goos           string
		startedElevated bool
		want           string
	}{
		{name: "root started unix process", goos: "linux", startedElevated: true, want: "search --update yes"},
		{name: "non-elevated unix process", goos: "linux", want: "sudo search --update yes"},
		{name: "windows process", goos: "windows", want: "search --update yes (run from an elevated prompt)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := updateManualCommand(tt.goos, tt.startedElevated); got != tt.want {
				t.Errorf("updateManualCommand(%q, %t) = %q, want %q", tt.goos, tt.startedElevated, got, tt.want)
			}
		})
	}
}

func TestUpdateVersionHasFailedOnlyForMatchingFailedEvent(t *testing.T) {
	entries := []logging.AuditEntry{
		{Event: "update.failed", Result: "failure", Details: map[string]interface{}{"version": "1.2.3"}},
		{Event: "update.failed", Result: "success", Details: map[string]interface{}{"version": "1.2.4"}},
		{Event: "update.staged", Result: "failure", Details: map[string]interface{}{"version": "1.2.5"}},
	}
	if !updateVersionHasFailed(entries, "1.2.3") {
		t.Fatal("failed version was not detected")
	}
	for _, version := range []string{"1.2.4", "1.2.5", "1.2.6"} {
		if updateVersionHasFailed(entries, version) {
			t.Errorf("version %s unexpectedly treated as a failed update", version)
		}
	}
}
