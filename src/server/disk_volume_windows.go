//go:build windows

package server

import (
	"fmt"
	"path/filepath"
	"strings"
)

func diskVolumeKey(path string) (string, error) {
	resolved, err := existingDiskPath(path)
	if err != nil {
		return "", err
	}
	volume := filepath.VolumeName(resolved)
	if volume == "" {
		return "", fmt.Errorf("filesystem volume is unavailable for %q", resolved)
	}
	return strings.ToLower(volume), nil
}
