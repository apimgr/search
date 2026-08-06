package cmd

import (
	"errors"
	"fmt"
	"testing"

	"github.com/apimgr/search/src/client/api"
)

// TestExitCodeForError covers every branch of the sentinel-error-to-exit-code
// mapping documented in AI.md PART 32's Exit Codes table.
func TestExitCodeForError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{"nil error", nil, 0},
		{"usage error direct", ErrUsage, 64},
		{"usage error wrapped", fmt.Errorf("bad flag: %w", ErrUsage), 64},
		{"not found direct", api.ErrNotFound, 5},
		{"not found wrapped", fmt.Errorf("lookup failed: %w", api.ErrNotFound), 5},
		{"authentication direct", api.ErrAuthentication, 4},
		{"authentication wrapped", fmt.Errorf("auth failed: %w", api.ErrAuthentication), 4},
		{"connection direct", api.ErrConnection, 3},
		{"connection wrapped", fmt.Errorf("dial failed: %w", api.ErrConnection), 3},
		{"configuration direct", ErrConfiguration, 2},
		{"configuration wrapped", fmt.Errorf("no server: %w", ErrConfiguration), 2},
		{"unmapped error falls through to default", errors.New("boom"), 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExitCodeForError(tt.err)
			if got != tt.want {
				t.Errorf("ExitCodeForError(%v) = %d, want %d", tt.err, got, tt.want)
			}
		})
	}
}

// TestExitCodeForErrorPriorityOrder verifies that when an error could
// plausibly match more than one sentinel, the switch's declared order wins.
// ErrUsage is checked before ErrConfiguration, so a hypothetical error
// wrapping both would resolve to the usage exit code.
func TestExitCodeForErrorPriorityOrder(t *testing.T) {
	wrapped := fmt.Errorf("%w: %w", ErrUsage, ErrConfiguration)
	got := ExitCodeForError(wrapped)
	if got != 64 {
		t.Errorf("ExitCodeForError(usage+configuration) = %d, want 64 (ErrUsage checked first)", got)
	}
}
