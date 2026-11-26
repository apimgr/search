package security

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/gofrs/flock"
)

// LockReportMutation serializes security-report insertion with PGP key
// rotation across the server and maintenance processes.
func LockReportMutation(configDir string) (*flock.Flock, error) {
	dir := filepath.Join(configDir, PGPDirName)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create PGP security directory: %w", err)
	}
	lock := flock.New(filepath.Join(dir, "security-reports.lock"))
	if err := lock.Lock(); err != nil {
		return nil, fmt.Errorf("lock security report mutation: %w", err)
	}
	return lock, nil
}
