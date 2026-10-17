//go:build windows

package update

import (
	"fmt"
	"os"
	"os/exec"
	"time"

	"golang.org/x/sys/windows"
)

// replaceBinary replaces the running binary (Windows).
// Per AI.md PART 22: Windows cannot delete or rename a running executable in
// place, so the current binary is renamed to .old, the new binary is moved
// into its place, and the .old copy is scheduled for deletion on reboot.
func replaceBinary(currentPath, newBinaryPath string) error {
	oldPath := currentPath + ".old"

	// Remove any existing .old file from a previous update
	os.Remove(oldPath)

	// Rename running binary to .old (this works on Windows)
	if err := os.Rename(currentPath, oldPath); err != nil {
		return fmt.Errorf("failed to rename current binary: %w", err)
	}

	// Move new binary to current path
	if err := os.Rename(newBinaryPath, currentPath); err != nil {
		if copyErr := copyFile(currentPath, newBinaryPath); copyErr != nil {
			// Restore the original binary so the install stays usable
			os.Rename(oldPath, currentPath)
			return fmt.Errorf("failed to move new binary: %w", err)
		}
		os.Remove(newBinaryPath)
	}

	// Schedule old binary for deletion on reboot
	oldPathPtr, err := windows.UTF16PtrFromString(oldPath)
	if err != nil {
		return nil
	}
	windows.MoveFileEx(oldPathPtr, nil, windows.MOVEFILE_DELAY_UNTIL_REBOOT)

	return nil
}

// restartSelf starts a new instance and exits (Windows).
// Windows has no exec() replacement, so a fresh process is spawned and the
// current one terminates.
func restartSelf() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}

	cmd := exec.Command(exe, os.Args[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start new process: %w", err)
	}

	// Give the new process time to start
	time.Sleep(100 * time.Millisecond)

	os.Exit(0)
	return nil
}
