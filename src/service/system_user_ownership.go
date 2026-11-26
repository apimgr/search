package service

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/apimgr/search/src/config"
)

func markSystemUserOwned(name string) error {
	if name == "" || filepath.Base(name) != name {
		return fmt.Errorf("invalid system account name %q", name)
	}
	directory := config.GetConfigDir()
	if directory == "" {
		return fmt.Errorf("configuration directory is unavailable")
	}
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create private configuration directory for service account marker: %w", err)
	}
	if err := os.Chmod(directory, 0o700); err != nil {
		return fmt.Errorf("secure configuration directory for service account marker: %w", err)
	}
	marker := filepath.Join(directory, ".service-account-created")
	file, err := os.OpenFile(marker, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if !os.IsExist(err) {
			return fmt.Errorf("record service account ownership: %w", err)
		}
		info, statErr := os.Lstat(marker)
		if statErr != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
			return fmt.Errorf("service account ownership marker is invalid")
		}
		data, readErr := os.ReadFile(marker)
		if readErr != nil || strings.TrimSpace(string(data)) != name {
			return fmt.Errorf("service account ownership marker is invalid")
		}
		return nil
	}
	if _, err := file.WriteString(name + "\n"); err != nil {
		_ = file.Close()
		_ = os.Remove(marker)
		return fmt.Errorf("write service account ownership marker: %w", err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(marker)
		return fmt.Errorf("close service account ownership marker: %w", err)
	}
	return nil
}

func systemUserOwned(name string) bool {
	if name == "" || filepath.Base(name) != name {
		return false
	}
	marker := filepath.Join(config.GetConfigDir(), ".service-account-created")
	info, err := os.Lstat(marker)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
		return false
	}
	data, err := os.ReadFile(marker)
	return err == nil && strings.TrimSpace(string(data)) == name
}

func clearSystemUserOwnership(name string) error {
	if !systemUserOwned(name) {
		return nil
	}
	if err := os.Remove(filepath.Join(config.GetConfigDir(), ".service-account-created")); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove service account ownership marker: %w", err)
	}
	return nil
}
