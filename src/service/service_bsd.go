//go:build freebsd || openbsd || netbsd

package service

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Install installs the rc.d service on BSD.
func (sm *ServiceManager) Install() error {
	return sm.installRCd()
}

// Uninstall removes the rc.d service on BSD.
func (sm *ServiceManager) Uninstall() error {
	return sm.uninstallRCd()
}

// Status returns the rc.d service status on BSD.
func (sm *ServiceManager) Status() (string, error) {
	return sm.statusRCd()
}

// StartAllServices starts the service on BSD.
func (sm *ServiceManager) StartAllServices() error {
	return runCommand("service", "search", "start")
}

// StopAllServices stops the service on BSD.
func (sm *ServiceManager) StopAllServices() error {
	return runCommand("service", "search", "stop")
}

// RestartAllServices restarts the service on BSD.
func (sm *ServiceManager) RestartAllServices() error {
	return runCommand("service", "search", "restart")
}

// Reload reloads the service configuration on BSD.
func (sm *ServiceManager) Reload() error {
	return runCommand("service", "search", "reload")
}

// Enable enables the service to start on boot on BSD.
func (sm *ServiceManager) Enable() error {
	return appendToFile("/etc/rc.conf", "search_enable=\"YES\"\n")
}

// Disable disables the service from starting on boot on BSD.
func (sm *ServiceManager) Disable() error {
	return runCommand("sysrc", "search_enable=NO")
}

// InstallUserService is not supported on BSD.
func (sm *ServiceManager) InstallUserService() error {
	return fmt.Errorf("user service not supported on BSD")
}

// ensureSystemUser creates the BSD service user if it doesn't exist.
func (sm *ServiceManager) ensureSystemUser() error {
	_, err := CreateSystemUser("search")
	return err
}

// rcdTemplate is the BSD rc.d service script.
// Per AI.md PART 25 EXACT MATCH.
const rcdTemplate = `#!/bin/sh

# PROVIDE: search
# REQUIRE: NETWORKING
# KEYWORD: shutdown

. /etc/rc.subr

name="search"
rcvar="search_enable"
command="/usr/local/bin/search"

load_rc_config $name
run_rc_command "$1"
`

func (sm *ServiceManager) installRCd() error {
	if err := sm.ensureSystemUser(); err != nil {
		return fmt.Errorf("failed to create system user: %w", err)
	}
	if err := sm.createServiceDirectories(); err != nil {
		return fmt.Errorf("failed to create directories: %w", err)
	}

	rcPath := "/usr/local/etc/rc.d/search"
	if err := os.WriteFile(rcPath, []byte(rcdTemplate), 0755); err != nil {
		return fmt.Errorf("failed to write rc script: %w", err)
	}
	return nil
}

func (sm *ServiceManager) uninstallRCd() error {
	runCommand("service", "search", "stop") //nolint:errcheck
	os.Remove("/usr/local/etc/rc.d/search")
	return nil
}

func (sm *ServiceManager) statusRCd() (string, error) {
	out, err := exec.Command("service", "search", "status").Output()
	if err != nil {
		return "inactive", nil
	}
	if strings.Contains(string(out), "running") {
		return "active", nil
	}
	return "inactive", nil
}
