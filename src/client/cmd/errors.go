package cmd

import (
	"errors"

	"github.com/apimgr/search/src/client/api"
)

// Sentinel errors used to select the CLI process exit code.
// Per AI.md PART 32 Exit Codes table.
var (
	// ErrConfiguration indicates missing/invalid client configuration (exit 2).
	ErrConfiguration = errors.New("configuration error")
	// ErrUsage indicates invalid command-line arguments (exit 64).
	ErrUsage = errors.New("usage error")
)

// ExitCodeForError maps an error returned from ExecuteClientCLI to the exit
// code documented in AI.md PART 32's Exit Codes table.
func ExitCodeForError(err error) int {
	switch {
	case err == nil:
		return 0
	case errors.Is(err, ErrUsage):
		return 64
	case errors.Is(err, api.ErrNotFound):
		return 5
	case errors.Is(err, api.ErrAuthentication):
		return 4
	case errors.Is(err, api.ErrConnection):
		return 3
	case errors.Is(err, ErrConfiguration):
		return 2
	default:
		return 1
	}
}
