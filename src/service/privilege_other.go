//go:build !linux && !darwin && !freebsd && !openbsd && !netbsd && !windows

package service

import (
	"fmt"
	"runtime"
)

// GetServiceUser returns the service user name (bare name on unsupported platforms).
func GetServiceUser(serviceName string) string {
	return serviceName
}

// GetServiceGroup returns the service group name (bare name on unsupported platforms).
func GetServiceGroup(serviceName string) string {
	return serviceName
}

// FindAvailableSystemID is not supported on this platform.
func FindAvailableSystemID() (int, error) {
	return 0, fmt.Errorf("unsupported OS: %s", runtime.GOOS)
}

// CreateSystemUser is not supported on this platform.
func CreateSystemUser(name string) (*SystemUser, error) {
	return nil, fmt.Errorf("unsupported OS: %s", runtime.GOOS)
}
