package config

import (
	"testing"
)

// TestIsValidHostNonICANNInProduction verifies that a non-ICANN TLD is rejected in production mode.
func TestIsValidHostNonICANNInProduction(t *testing.T) {
	tests := []struct {
		name        string
		host        string
		devMode     bool
		projectName string
		want        bool
	}{
		// Non-ICANN TLD in production — should be rejected (covers line 83-85)
		// "proprietary" is not in devOnlyTLDs and not ICANN, so it hits the !icann branch
		{"non-ICANN TLD in production", "example.proprietary", false, "", false},
		{"unknown TLD in production", "myapp.enterprise", false, "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsValidHost(tt.host, tt.devMode, tt.projectName)
			if got != tt.want {
				t.Errorf("IsValidHost(%q, devMode=%v) = %v, want %v", tt.host, tt.devMode, got, tt.want)
			}
		})
	}
}
