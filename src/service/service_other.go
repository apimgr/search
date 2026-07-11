//go:build !linux && !darwin && !freebsd && !openbsd && !netbsd && !windows

package service

import (
	"fmt"
	"runtime"
)

// Install is not supported on this platform.
func (sm *ServiceManager) Install() error {
	return fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
}

// Uninstall is not supported on this platform.
func (sm *ServiceManager) Uninstall() error {
	return fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
}

// Status is not supported on this platform.
func (sm *ServiceManager) Status() (string, error) {
	return "", fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
}

// StartAllServices is not supported on this platform.
func (sm *ServiceManager) StartAllServices() error {
	return fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
}

// StopAllServices is not supported on this platform.
func (sm *ServiceManager) StopAllServices() error {
	return fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
}

// RestartAllServices is not supported on this platform.
func (sm *ServiceManager) RestartAllServices() error {
	return fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
}

// Reload is not supported on this platform.
func (sm *ServiceManager) Reload() error {
	return fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
}

// Enable is not supported on this platform.
func (sm *ServiceManager) Enable() error {
	return fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
}

// Disable is not supported on this platform.
func (sm *ServiceManager) Disable() error {
	return fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
}

// InstallUserService is not supported on this platform.
func (sm *ServiceManager) InstallUserService() error {
	return fmt.Errorf("user service not supported on %s", runtime.GOOS)
}
