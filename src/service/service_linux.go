//go:build linux

package service

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"

	"github.com/apimgr/search/src/config"
)

// Install installs the system service on Linux (runit → OpenRC → systemd).
func (sm *ServiceManager) Install() error {
	if sm.hasRunit() {
		return sm.installRunit()
	}
	if sm.hasOpenRC() {
		return sm.installOpenRC()
	}
	return sm.installSystemd()
}

// Uninstall removes the system service on Linux.
func (sm *ServiceManager) Uninstall() error {
	if sm.hasRunit() {
		return sm.uninstallRunit()
	}
	if sm.hasOpenRC() {
		return sm.uninstallOpenRC()
	}
	return sm.uninstallSystemd()
}

// Status returns the service status on Linux.
func (sm *ServiceManager) Status() (string, error) {
	if sm.hasRunit() {
		return sm.statusRunit()
	}
	if sm.hasOpenRC() {
		return sm.statusOpenRC()
	}
	return sm.statusSystemd()
}

// StartAllServices starts the service on Linux.
func (sm *ServiceManager) StartAllServices() error {
	if sm.hasRunit() {
		return runCommand("sv", "start", "search")
	}
	if sm.hasOpenRC() {
		return runCommand("rc-service", "search", "start")
	}
	return runCommand("systemctl", "start", "search")
}

// StopAllServices stops the service on Linux.
func (sm *ServiceManager) StopAllServices() error {
	if sm.hasRunit() {
		return runCommand("sv", "stop", "search")
	}
	if sm.hasOpenRC() {
		return runCommand("rc-service", "search", "stop")
	}
	return runCommand("systemctl", "stop", "search")
}

// RestartAllServices restarts the service on Linux.
func (sm *ServiceManager) RestartAllServices() error {
	if sm.hasRunit() {
		return runCommand("sv", "restart", "search")
	}
	if sm.hasOpenRC() {
		return runCommand("rc-service", "search", "restart")
	}
	return runCommand("systemctl", "restart", "search")
}

// Reload reloads the service configuration on Linux.
func (sm *ServiceManager) Reload() error {
	if sm.hasRunit() {
		return runCommand("sv", "hup", "search")
	}
	if sm.hasOpenRC() {
		// OpenRC doesn't have a reload command; use restart
		return runCommand("rc-service", "search", "restart")
	}
	return runCommand("systemctl", "reload", "search")
}

// Enable enables the service to start on boot on Linux.
func (sm *ServiceManager) Enable() error {
	if sm.hasRunit() {
		serviceDir := sm.getRunitServiceDir()
		activeDir := sm.getRunitActiveDir()
		linkPath := filepath.Join(activeDir, "search")
		os.Remove(linkPath)
		return os.Symlink(serviceDir, linkPath)
	}
	if sm.hasOpenRC() {
		return runCommand("rc-update", "add", "search", "default")
	}
	return runCommand("systemctl", "enable", "search")
}

// Disable disables the service from starting on boot on Linux.
func (sm *ServiceManager) Disable() error {
	if sm.hasRunit() {
		activeDir := sm.getRunitActiveDir()
		return os.Remove(filepath.Join(activeDir, "search"))
	}
	if sm.hasOpenRC() {
		return runCommand("rc-update", "del", "search", "default")
	}
	return runCommand("systemctl", "disable", "search")
}

// InstallUserService installs a user-level systemd service unit on Linux.
// Per AI.md PART 23: Fallback when user cannot or declines to escalate.
func (sm *ServiceManager) InstallUserService() error {
	return sm.installSystemdUserService()
}

// hasRunit checks if runit is the init system.
func (sm *ServiceManager) hasRunit() bool {
	if _, err := exec.LookPath("runsv"); err == nil {
		if _, err := os.Stat("/var/service"); err == nil {
			return true
		}
		if _, err := os.Stat("/service"); err == nil {
			return true
		}
	}
	return false
}

// hasOpenRC checks if OpenRC is the init system.
func (sm *ServiceManager) hasOpenRC() bool {
	if _, err := exec.LookPath("openrc-run"); err == nil {
		return true
	}
	if _, err := exec.LookPath("rc-service"); err == nil {
		return true
	}
	return false
}

// ensureSystemUser creates the system user and group if they don't exist.
// Per AI.md PART 25: System user with home in /var/lib/apimgr/search.
func (sm *ServiceManager) ensureSystemUser() error {
	userName := "search"
	homeDir := "/var/lib/apimgr/search"

	if _, err := user.Lookup(userName); err == nil {
		return nil
	}

	// Create system group first; ignore error if group already exists
	runCommand("groupadd", "--system", userName)

	return runCommand("useradd",
		"--system",
		"--no-create-home",
		"--home-dir", homeDir,
		"--shell", "/sbin/nologin",
		"--gid", userName,
		userName,
	)
}

func (sm *ServiceManager) getRunitServiceDir() string {
	if _, err := os.Stat("/etc/sv"); err == nil {
		return "/etc/sv/search"
	}
	return "/etc/runit/sv/search"
}

func (sm *ServiceManager) getRunitActiveDir() string {
	if _, err := os.Stat("/var/service"); err == nil {
		return "/var/service"
	}
	if _, err := os.Stat("/service"); err == nil {
		return "/service"
	}
	return "/var/service"
}

// systemdTemplate per AI.md PART 25 — EXACT MATCH to spec.
// Service starts as root, binary drops to search user after port binding.
const systemdTemplate = `[Unit]
Description=search service
Documentation=https://apimgr.github.io/search
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=/usr/local/bin/search
Restart=on-failure
RestartSec=5
StandardOutput=journal
StandardError=journal

# Security hardening (binary drops privileges after port binding)
ProtectSystem=strict
ProtectHome=yes
PrivateTmp=yes
NoNewPrivileges=true
ReadWritePaths=/etc/apimgr/search
ReadWritePaths=/var/lib/apimgr/search
ReadWritePaths=/var/cache/apimgr/search
ReadWritePaths=/var/log/apimgr/search
ReadWritePaths=/var/run/apimgr

[Install]
WantedBy=multi-user.target
`

func (sm *ServiceManager) installSystemd() error {
	if err := sm.ensureSystemUser(); err != nil {
		return fmt.Errorf("failed to create system user: %w", err)
	}
	if err := sm.createServiceDirectories(); err != nil {
		return fmt.Errorf("failed to create directories: %w", err)
	}

	servicePath := "/etc/systemd/system/search.service"
	if err := os.WriteFile(servicePath, []byte(systemdTemplate), 0644); err != nil {
		return fmt.Errorf("failed to write service file: %w", err)
	}
	if err := runCommand("systemctl", "daemon-reload"); err != nil {
		return fmt.Errorf("failed to reload systemd: %w", err)
	}
	if err := runCommand("systemctl", "enable", "search"); err != nil {
		return fmt.Errorf("failed to enable service: %w", err)
	}
	return nil
}

func (sm *ServiceManager) uninstallSystemd() error {
	runCommand("systemctl", "stop", "search")
	runCommand("systemctl", "disable", "search")
	os.Remove("/etc/systemd/system/search.service")
	runCommand("systemctl", "daemon-reload")
	return nil
}

func (sm *ServiceManager) statusSystemd() (string, error) {
	out, err := exec.Command("systemctl", "is-active", "search").Output()
	if err != nil {
		return "inactive", nil
	}
	return strings.TrimSpace(string(out)), nil
}

const runitRunTemplate = `#!/bin/sh
exec chpst -u search:search /usr/local/bin/search 2>&1
`

const runitLogRunTemplate = `#!/bin/sh
exec svlogd -tt /var/log/apimgr/search
`

func (sm *ServiceManager) installRunit() error {
	serviceDir := sm.getRunitServiceDir()
	logDir := filepath.Join(serviceDir, "log")

	data := map[string]string{
		"User":      "search",
		"Group":     "search",
		"Binary":    config.GetBinaryPath(),
		"ConfigDir": config.GetConfigDir(),
		"LogDir":    filepath.Join(config.GetDataDir(), "logs", "runit"),
	}

	if err := os.MkdirAll(serviceDir, 0755); err != nil {
		return fmt.Errorf("failed to create service directory: %w", err)
	}
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return fmt.Errorf("failed to create log directory: %w", err)
	}
	if err := os.MkdirAll(data["LogDir"], 0755); err != nil {
		return fmt.Errorf("failed to create log storage directory: %w", err)
	}

	runContent, err := sm.renderTemplate(runitRunTemplate, data)
	if err != nil {
		return err
	}
	runPath := filepath.Join(serviceDir, "run")
	if err := os.WriteFile(runPath, []byte(runContent), 0755); err != nil {
		return fmt.Errorf("failed to write run script: %w", err)
	}

	logRunContent, err := sm.renderTemplate(runitLogRunTemplate, data)
	if err != nil {
		return err
	}
	logRunPath := filepath.Join(logDir, "run")
	if err := os.WriteFile(logRunPath, []byte(logRunContent), 0755); err != nil {
		return fmt.Errorf("failed to write log run script: %w", err)
	}

	activeDir := sm.getRunitActiveDir()
	linkPath := filepath.Join(activeDir, "search")
	os.Remove(linkPath)
	if err := os.Symlink(serviceDir, linkPath); err != nil {
		return fmt.Errorf("failed to enable service (symlink): %w", err)
	}
	return nil
}

func (sm *ServiceManager) uninstallRunit() error {
	runCommand("sv", "stop", "search")
	activeDir := sm.getRunitActiveDir()
	os.Remove(filepath.Join(activeDir, "search"))
	serviceDir := sm.getRunitServiceDir()
	os.RemoveAll(serviceDir)
	return nil
}

// parseRunitStatus maps raw sv status output to a canonical status string.
func parseRunitStatus(output string) string {
	if strings.HasPrefix(output, "run:") {
		return "active"
	}
	if strings.HasPrefix(output, "down:") {
		return "inactive"
	}
	return "unknown"
}

func (sm *ServiceManager) statusRunit() (string, error) {
	out, err := exec.Command("sv", "status", "search").Output()
	if err != nil {
		return "inactive", nil
	}
	return parseRunitStatus(string(out)), nil
}

// openrcTemplate is the OpenRC init script.
// Per AI.md PART 25: Service starts as root, binary drops to search user after port binding.
const openrcTemplate = `#!/sbin/openrc-run

description="search - Privacy-respecting metasearch engine"
command="/usr/local/bin/search"
command_background=true
pidfile="/run/search.pid"
output_log="/var/log/apimgr/search/stdout.log"
error_log="/var/log/apimgr/search/stderr.log"

depend() {
	need net
	after firewall
}

start_pre() {
	checkpath --directory --owner search:search /var/lib/apimgr/search
	checkpath --directory --owner search:search /var/log/apimgr/search
	checkpath --directory --owner search:search /var/cache/apimgr/search
}
`

func (sm *ServiceManager) installOpenRC() error {
	if err := sm.ensureSystemUser(); err != nil {
		return fmt.Errorf("failed to create system user: %w", err)
	}
	if err := sm.createServiceDirectories(); err != nil {
		return fmt.Errorf("failed to create directories: %w", err)
	}

	logDir := "/var/log/apimgr/search"
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return fmt.Errorf("failed to create log directory: %w", err)
	}

	initPath := "/etc/init.d/search"
	if err := os.WriteFile(initPath, []byte(openrcTemplate), 0755); err != nil {
		return fmt.Errorf("failed to write init script: %w", err)
	}
	if err := runCommand("rc-update", "add", "search", "default"); err != nil {
		return fmt.Errorf("failed to add service to default runlevel: %w", err)
	}
	return nil
}

func (sm *ServiceManager) uninstallOpenRC() error {
	runCommand("rc-service", "search", "stop")
	runCommand("rc-update", "del", "search", "default")
	os.Remove("/etc/init.d/search")
	return nil
}

// parseOpenRCStatus maps raw rc-service output to a canonical status string.
func parseOpenRCStatus(output string) string {
	lower := strings.ToLower(output)
	if strings.Contains(lower, "started") {
		return "active"
	}
	if strings.Contains(lower, "stopped") {
		return "inactive"
	}
	return "unknown"
}

func (sm *ServiceManager) statusOpenRC() (string, error) {
	out, err := exec.Command("rc-service", "search", "status").Output()
	if err != nil {
		return "inactive", nil
	}
	return parseOpenRCStatus(string(out)), nil
}

// systemdUserTemplate is the systemd user service unit template.
// Per AI.md PART 23/24: User service runs as calling user, ports >1024 only.
const systemdUserTemplate = `[Unit]
Description=Search - Privacy-respecting self-hosted metasearch engine
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/search
Restart=always
RestartSec=5

[Install]
WantedBy=default.target
`

// installSystemdUserService installs a systemd --user service unit.
// Per AI.md PART 23/24: ~/.config/systemd/user/search.service
func (sm *ServiceManager) installSystemdUserService() error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to determine home directory: %w", err)
	}

	unitDir := filepath.Join(homeDir, ".config", "systemd", "user")
	if err := os.MkdirAll(unitDir, 0755); err != nil {
		return fmt.Errorf("failed to create systemd user unit directory: %w", err)
	}

	unitPath := filepath.Join(unitDir, "search.service")
	if err := os.WriteFile(unitPath, []byte(systemdUserTemplate), 0644); err != nil {
		return fmt.Errorf("failed to write user service unit: %w", err)
	}
	if err := runCommand("systemctl", "--user", "daemon-reload"); err != nil {
		return fmt.Errorf("failed to reload systemd user daemon: %w", err)
	}
	if err := runCommand("systemctl", "--user", "enable", "search"); err != nil {
		return fmt.Errorf("failed to enable user service: %w", err)
	}
	return nil
}
