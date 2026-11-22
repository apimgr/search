package server

import (
	"fmt"
	"os"
	"path/filepath"
)

func existingDiskPath(path string) (string, error) {
	resolved, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(resolved); err == nil {
			return resolved, nil
		} else if !os.IsNotExist(err) {
			return "", err
		}
		parent := filepath.Dir(resolved)
		if parent == resolved {
			return "", fmt.Errorf("no existing parent for %q", path)
		}
		resolved = parent
	}
}
