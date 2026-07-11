//go:build windows

package service

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/apimgr/search/src/config"
)

// Install installs the Windows service.
func (sm *ServiceManager) Install() error {
	return sm.installWindowsService()
}

// Uninstall removes the Windows service.
func (sm *ServiceManager) Uninstall() error {
	return sm.uninstallWindowsService()
}

// Status returns the Windows service status.
func (sm *ServiceManager) Status() (string, error) {
	return sm.statusWindowsService()
}

// StartAllServices starts the Windows service.
func (sm *ServiceManager) StartAllServices() error {
	return runCommand("sc", "start", "search")
}

// StopAllServices stops the Windows service.
func (sm *ServiceManager) StopAllServices() error {
	return runCommand("sc", "stop", "search")
}

// RestartAllServices restarts the Windows service.
func (sm *ServiceManager) RestartAllServices() error {
	// Ignore stop errors
	sm.StopAllServices() //nolint:errcheck
	return sm.StartAllServices()
}

// Reload reloads the service on Windows (no native reload; use restart).
func (sm *ServiceManager) Reload() error {
	return sm.RestartAllServices()
}

// Enable enables the Windows service to start automatically.
func (sm *ServiceManager) Enable() error {
	return runCommand("sc", "config", "search", "start=", "auto")
}

// Disable disables automatic start for the Windows service.
func (sm *ServiceManager) Disable() error {
	return runCommand("sc", "config", "search", "start=", "disabled")
}

// InstallUserService is not supported on Windows.
func (sm *ServiceManager) InstallUserService() error {
	return fmt.Errorf("user service not supported on windows")
}

func (sm *ServiceManager) installWindowsService() error {
	binary := config.GetBinaryPath()
	configDir := config.GetConfigDir()
	return runCommand("sc", "create", "search",
		"binPath=", fmt.Sprintf("\"%s\" --config \"%s\"", binary, configDir),
		"DisplayName=", "Search - Privacy-Respecting Metasearch Engine",
		"start=", "auto")
}

func (sm *ServiceManager) uninstallWindowsService() error {
	runCommand("sc", "stop", "search")   //nolint:errcheck
	return runCommand("sc", "delete", "search")
}

func (sm *ServiceManager) statusWindowsService() (string, error) {
	out, err := exec.Command("sc", "query", "search").Output()
	if err != nil {
		return "inactive", nil
	}
	if strings.Contains(string(out), "RUNNING") {
		return "active", nil
	}
	return "inactive", nil
}
