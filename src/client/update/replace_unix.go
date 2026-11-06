//go:build !windows

package update

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

func replaceBinary(currentPath, stagedPath string) error {
	info, err := os.Stat(currentPath)
	if err != nil {
		return fmt.Errorf("stat installed binary: %w", err)
	}
	if err := os.Chmod(stagedPath, info.Mode().Perm()); err != nil {
		return fmt.Errorf("set installed binary mode: %w", err)
	}
	if err := os.Rename(stagedPath, currentPath); err != nil {
		if !errors.Is(err, syscall.EXDEV) {
			return fmt.Errorf("atomically replace installed binary: %w", err)
		}
		if err := copyStagedBinary(stagedPath, currentPath); err != nil {
			return fmt.Errorf("atomically replace installed binary across filesystems: %w", err)
		}
		if err := os.Remove(stagedPath); err != nil {
			return fmt.Errorf("remove staged client update: %w", err)
		}
	}
	return nil
}

func restart(path string) error {
	return syscall.Exec(path, os.Args, os.Environ())
}
