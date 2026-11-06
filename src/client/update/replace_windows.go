//go:build windows

package update

import (
	"errors"
	"fmt"
	"os"
	"os/exec"

	"golang.org/x/sys/windows"
)

func replaceBinary(currentPath, stagedPath string) error {
	oldPath := currentPath + ".old"
	if err := os.Remove(oldPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove previous client backup: %w", err)
	}
	if err := os.Rename(currentPath, oldPath); err != nil {
		return fmt.Errorf("stage current client binary: %w", err)
	}
	if err := copyStagedBinary(stagedPath, currentPath); err != nil {
		if restoreErr := os.Rename(oldPath, currentPath); restoreErr != nil {
			return errors.Join(fmt.Errorf("install staged client binary: %w", err), fmt.Errorf("restore original client binary: %w", restoreErr))
		}
		return fmt.Errorf("install staged client binary: %w", err)
	}
	oldPathPtr, err := windows.UTF16PtrFromString(oldPath)
	if err != nil {
		return fmt.Errorf("convert client backup path for delayed deletion: %w", err)
	}
	if err := windows.MoveFileEx(oldPathPtr, nil, windows.MOVEFILE_DELAY_UNTIL_REBOOT); err != nil {
		return fmt.Errorf("schedule previous client binary for deletion: %w", err)
	}
	return nil
}

func restart(path string) error {
	cmd := exec.Command(path, os.Args[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start updated client: %w", err)
	}
	return ErrRestartStarted
}
