//go:build windows

package update

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"time"

	"golang.org/x/sys/windows"
)

var scheduleOldBinaryForDeletion = func(path string) error {
	pathPtr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return fmt.Errorf("convert old binary path for Windows: %w", err)
	}
	if err := windows.MoveFileEx(pathPtr, nil, windows.MOVEFILE_DELAY_UNTIL_REBOOT); err != nil {
		return fmt.Errorf("schedule old binary deletion on reboot: %w", err)
	}
	return nil
}

// replaceBinary replaces the running binary (Windows).
// Per AI.md PART 22: Windows cannot delete or rename a running executable in
// place, so the current binary is renamed to .old, the new binary is moved
// into its place, and the .old copy is scheduled for deletion on reboot.
func replaceBinary(currentPath, newBinaryPath string) error {
	oldPath := currentPath + ".old"
	var cleanupErr error

	// Remove any existing .old file from a previous update
	if err := os.Remove(oldPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("failed to remove previous old binary: %w", err)
	}

	// Rename running binary to .old (this works on Windows)
	if err := os.Rename(currentPath, oldPath); err != nil {
		return fmt.Errorf("failed to rename current binary: %w", err)
	}

	// Move new binary to current path
	if err := os.Rename(newBinaryPath, currentPath); err != nil {
		if copyErr := copyFile(currentPath, newBinaryPath); copyErr != nil {
			// Restore the original binary so the install stays usable
			if restoreErr := os.Rename(oldPath, currentPath); restoreErr != nil {
				return errors.Join(fmt.Errorf("failed to move new binary: %w", err), fmt.Errorf("failed to restore original binary: %w", restoreErr))
			}
			return fmt.Errorf("failed to move new binary: %w", err)
		}
		if removeErr := os.Remove(newBinaryPath); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			cleanupErr = fmt.Errorf("failed to remove temporary update file: %w", removeErr)
		}
	}

	// Schedule old binary for deletion on reboot
	if err := scheduleOldBinaryForDeletion(oldPath); err != nil {
		scheduleErr := fmt.Errorf("old binary cleanup could not be scheduled: %w", err)
		if cleanupErr != nil {
			return errors.Join(fmt.Errorf("new binary installed"), cleanupErr, scheduleErr)
		}
		return fmt.Errorf("new binary installed, but %w", scheduleErr)
	}
	if cleanupErr != nil {
		return fmt.Errorf("new binary installed and old binary cleanup scheduled, but %w", cleanupErr)
	}

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
