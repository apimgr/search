//go:build windows

package service

import (
	"testing"

	"github.com/apimgr/search/src/config"
)

func TestServiceManagerStatusWindowsService(t *testing.T) {
	cfg := config.DefaultConfig()
	sm := NewServiceManager(cfg)

	status, err := sm.statusWindowsService()
	_ = status
	_ = err
}

func TestServiceManagerInstallWindowsServiceError(t *testing.T) {
	cfg := config.DefaultConfig()
	sm := NewServiceManager(cfg)

	err := sm.installWindowsService()
	_ = err
}

func TestServiceManagerUninstallWindowsService(t *testing.T) {
	cfg := config.DefaultConfig()
	sm := NewServiceManager(cfg)

	err := sm.uninstallWindowsService()
	_ = err
}
