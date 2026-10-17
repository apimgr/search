//go:build !windows

package update

import (
	"fmt"
	"os"
	"syscall"
)

// replaceBinary replaces the running binary (Unix).
// Per AI.md PART 22: Unix allows renaming over a running executable - the old
// image stays mapped until the process exits, then the new one takes over.
func replaceBinary(currentPath, newBinaryPath string) error {
	// Get current binary permissions
	mode := os.FileMode(0755)
	if info, err := os.Stat(currentPath); err == nil {
		mode = info.Mode()
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("failed to stat current binary: %w", err)
	}

	// Atomic rename: new binary replaces current
	if err := os.Rename(newBinaryPath, currentPath); err != nil {
		// A rename cannot cross filesystems; fall back to an in-place copy
		if err := copyFile(currentPath, newBinaryPath); err != nil {
			return fmt.Errorf("failed to replace binary: %w", err)
		}
		if err := os.Remove(newBinaryPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("binary replaced but failed to remove staged update: %w", err)
		}
	}

	// Restore permissions
	if err := os.Chmod(currentPath, mode); err != nil {
		return fmt.Errorf("failed to restore permissions: %w", err)
	}

	return nil
}

// restartSelf re-executes the current process (Unix).
// syscall.Exec replaces the process image, so the updated binary takes over
// the same PID and never returns to this function on success.
func restartSelf() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}

	return syscall.Exec(exe, os.Args, os.Environ())
}
