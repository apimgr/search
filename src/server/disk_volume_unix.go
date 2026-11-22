//go:build !windows

package server

import (
	"fmt"
	"os"
	"syscall"
)

func diskVolumeKey(path string) (string, error) {
	resolved, err := existingDiskPath(path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return "", fmt.Errorf("filesystem identity is unavailable for %q", resolved)
	}
	return fmt.Sprintf("%v", stat.Dev), nil
}
