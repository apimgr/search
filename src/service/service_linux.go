//go:build linux

package service

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"

	"github.com/apimgr/search/src/config"
)

// initSystem identifies the detected Linux init system.
// Per AI.md PART 23 "Service Installation Logic": systemd, OpenRC, runit, s6;
// PART 24 additionally specifies SysVinit for legacy hosts.
type initSystem string

const (
	initRunit    initSystem = "runit"
	initS6       initSystem = "s6"
	initOpenRC   initSystem = "openrc"
	initSystemd  initSystem = "systemd"
	initSysVinit initSystem = "sysvinit"
)

// detectInitSystem resolves which init system manages this host.
// Per AI.md PART 24 SysVinit detection: SysVinit is selected only when
// openrc-run and systemctl are both absent and /etc/init.d exists with a
// working update-rc.d or chkconfig.
func (sm *ServiceManager) detectInitSystem() initSystem {
	if sm.hasRunit() {
		return initRunit
	}
	if sm.hasS6() {
		return initS6
	}
	if sm.hasOpenRC() {
		return initOpenRC
	}
	if sm.hasSystemd() {
		return initSystemd
	}
	if sm.hasSysVinit() {
		return initSysVinit
	}
	return initSystemd
}

// Install installs the system service on Linux.
func (sm *ServiceManager) Install() error {
	switch sm.detectInitSystem() {
	case initRunit:
		return sm.installRunit()
	case initS6:
		return sm.installS6()
	case initOpenRC:
		return sm.installOpenRC()
	case initSysVinit:
		return sm.installSysVinit()
	default:
		return sm.installSystemd()
	}
}

// Uninstall removes the system service on Linux.
func (sm *ServiceManager) Uninstall() error {
	switch sm.detectInitSystem() {
	case initRunit:
		return sm.uninstallRunit()
	case initS6:
		return sm.uninstallS6()
	case initOpenRC:
		return sm.uninstallOpenRC()
	case initSysVinit:
		return sm.uninstallSysVinit()
	default:
		return sm.uninstallSystemd()
	}
}

// Status returns the service status on Linux.
func (sm *ServiceManager) Status() (string, error) {
	switch sm.detectInitSystem() {
	case initRunit:
		return sm.statusRunit()
	case initS6:
		return sm.statusS6()
	case initOpenRC:
		return sm.statusOpenRC()
	case initSysVinit:
		return sm.statusSysVinit()
	default:
		return sm.statusSystemd()
	}
}

// StartAllServices starts the service on Linux.
func (sm *ServiceManager) StartAllServices() error {
	switch sm.detectInitSystem() {
	case initRunit:
		return runCommand("sv", "start", "search")
	case initS6:
		return runCommand("s6-svc", "-u", sm.getS6ActivePath())
	case initOpenRC:
		return runCommand("rc-service", "search", "start")
	case initSysVinit:
		return runCommand(sysVInitScriptPath, "start")
	default:
		return runCommand("systemctl", "start", "search")
	}
}

// StopAllServices stops the service on Linux.
func (sm *ServiceManager) StopAllServices() error {
	switch sm.detectInitSystem() {
	case initRunit:
		return runCommand("sv", "stop", "search")
	case initS6:
		return runCommand("s6-svc", "-d", sm.getS6ActivePath())
	case initOpenRC:
		return runCommand("rc-service", "search", "stop")
	case initSysVinit:
		return runCommand(sysVInitScriptPath, "stop")
	default:
		return runCommand("systemctl", "stop", "search")
	}
}

// RestartAllServices restarts the service on Linux.
func (sm *ServiceManager) RestartAllServices() error {
	switch sm.detectInitSystem() {
	case initRunit:
		return runCommand("sv", "restart", "search")
	case initS6:
		// s6-svc has no restart verb; -t signals the supervised process to
		// terminate and the supervisor immediately brings it back up.
		return runCommand("s6-svc", "-t", sm.getS6ActivePath())
	case initOpenRC:
		return runCommand("rc-service", "search", "restart")
	case initSysVinit:
		return runCommand(sysVInitScriptPath, "restart")
	default:
		return runCommand("systemctl", "restart", "search")
	}
}

// Reload reloads the service configuration on Linux.
func (sm *ServiceManager) Reload() error {
	switch sm.detectInitSystem() {
	case initRunit:
		return runCommand("sv", "hup", "search")
	case initS6:
		return runCommand("s6-svc", "-h", sm.getS6ActivePath())
	case initOpenRC:
		// OpenRC doesn't have a reload command; use restart
		return runCommand("rc-service", "search", "restart")
	case initSysVinit:
		// The SysVinit script implements restart as its reload equivalent
		return runCommand(sysVInitScriptPath, "restart")
	default:
		return runCommand("systemctl", "reload", "search")
	}
}

// Enable enables the service to start on boot on Linux.
func (sm *ServiceManager) Enable() error {
	switch sm.detectInitSystem() {
	case initRunit:
		serviceDir := sm.getRunitServiceDir()
		activeDir := sm.getRunitActiveDir()
		linkPath := filepath.Join(activeDir, "search")
		if err := os.Remove(linkPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove existing runit service link: %w", err)
		}
		return os.Symlink(serviceDir, linkPath)
	case initS6:
		return sm.enableS6()
	case initOpenRC:
		return runCommand("rc-update", "add", "search", "default")
	case initSysVinit:
		return enableSysVinit()
	default:
		return runCommand("systemctl", "enable", "search")
	}
}

// Disable disables the service from starting on boot on Linux.
func (sm *ServiceManager) Disable() error {
	switch sm.detectInitSystem() {
	case initRunit:
		activeDir := sm.getRunitActiveDir()
		return os.Remove(filepath.Join(activeDir, "search"))
	case initS6:
		return sm.disableS6()
	case initOpenRC:
		return runCommand("rc-update", "del", "search", "default")
	case initSysVinit:
		return disableSysVinit()
	default:
		return runCommand("systemctl", "disable", "search")
	}
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

// hasS6 checks if s6 is the supervision suite in use.
// Per AI.md PART 23: s6 is one of the four supported Linux init systems.
func (sm *ServiceManager) hasS6() bool {
	if _, err := exec.LookPath("s6-svscan"); err != nil {
		return false
	}
	for _, dir := range s6ScanDirs {
		if _, err := os.Stat(dir); err == nil {
			return true
		}
	}
	return false
}

// hasSystemd checks if systemd manages this host.
func (sm *ServiceManager) hasSystemd() bool {
	if _, err := exec.LookPath("systemctl"); err == nil {
		return true
	}
	return false
}

// hasSysVinit checks for a legacy SysVinit host.
// Per AI.md PART 24 SysVinit "Detection": chosen only when openrc-run and
// systemctl are absent and /etc/init.d exists with update-rc.d or chkconfig.
func (sm *ServiceManager) hasSysVinit() bool {
	if _, err := os.Stat("/sbin/openrc-run"); err == nil {
		return false
	}
	if _, err := exec.LookPath("systemctl"); err == nil {
		return false
	}
	if info, err := os.Stat("/etc/init.d"); err != nil || !info.IsDir() {
		return false
	}
	if _, err := exec.LookPath("update-rc.d"); err == nil {
		return true
	}
	if _, err := exec.LookPath("chkconfig"); err == nil {
		return true
	}
	return false
}

// ensureSystemUser creates the system user and group if they don't exist.
// Per AI.md PART 23/25: System user keyed on the frozen {internal_name}
// identity, with home in /var/lib/{internal_org}/{internal_name}.
func (sm *ServiceManager) ensureSystemUser() error {
	userName := config.InternalName

	if _, err := user.Lookup(userName); err == nil {
		return nil
	}

	// createLinuxSystemUser applies the PART 23 UID/GID selection rules
	// (matching UID==GID in the safe 200-899 range, reserved IDs skipped).
	if _, err := createLinuxSystemUser(userName); err != nil {
		return err
	}
	return nil
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
	var errs []error
	for _, args := range [][]string{{"stop", "search"}, {"disable", "search"}} {
		if err := runCommand("systemctl", args...); err != nil {
			errs = append(errs, err)
		}
	}
	if err := os.Remove("/etc/systemd/system/search.service"); err != nil && !os.IsNotExist(err) {
		errs = append(errs, err)
	}
	if err := runCommand("systemctl", "daemon-reload"); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func (sm *ServiceManager) statusSystemd() (string, error) {
	out, err := exec.Command("systemctl", "is-active", "search").Output()
	if err != nil {
		return "inactive", nil
	}
	return strings.TrimSpace(string(out)), nil
}

// runitRunTemplate per AI.md PART 24 "runit (Linux)" — EXACT MATCH to spec.
// The binary starts elevated and drops to the search user after port binding,
// so the run script must not pre-drop privileges.
const runitRunTemplate = `#!/bin/sh
exec /usr/local/bin/search 2>&1
`

const runitLogRunTemplate = `#!/bin/sh
exec svlogd -tt /var/log/apimgr/search
`

func (sm *ServiceManager) installRunit() error {
	serviceDir := sm.getRunitServiceDir()
	logDir := filepath.Join(serviceDir, "log")

	data := map[string]string{
		"User":      config.InternalName,
		"Group":     config.InternalName,
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
	if err := os.Remove(linkPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove existing runit service link: %w", err)
	}
	if err := os.Symlink(serviceDir, linkPath); err != nil {
		return fmt.Errorf("failed to enable service (symlink): %w", err)
	}
	return nil
}

func (sm *ServiceManager) uninstallRunit() error {
	if err := runCommand("sv", "stop", "search"); err != nil {
		slog.Warn("failed to stop runit service", "error", err)
	}
	activeDir := sm.getRunitActiveDir()
	if err := os.Remove(filepath.Join(activeDir, "search")); err != nil && !os.IsNotExist(err) {
		return err
	}
	serviceDir := sm.getRunitServiceDir()
	return os.RemoveAll(serviceDir)
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

// openrcTemplate returns the OpenRC init script.
// Per AI.md PART 24 "OpenRC (Alpine, Gentoo, Devuan)" — EXACT MATCH to spec.
func openrcTemplate() string {
	name := config.InternalName
	owner := name + ":" + name
	runDir := filepath.Join("/var/run", config.InternalOrg)
	logDir := filepath.Join("/var/log", config.InternalOrg, name)
	return fmt.Sprintf(`#!/sbin/openrc-run
# Service identity comes from %[1]s so config_dir/data_dir paths stay
# stable across binary renames.

name="%[1]s"
description="%[2]s"
# actual binary (may differ from %[1]s after rename)
command="/usr/local/bin/%[3]s"
command_args=""
command_user="%[4]s"
pidfile="%[5]s/%[1]s.pid"
command_background=true
output_log="%[6]s/server.log"
error_log="%[6]s/error.log"

depend() {
    need net
    after firewall
    use dns logger
}

start_pre() {
    checkpath -d -m 0755 -o %[4]s %[5]s
    checkpath -d -m 0755 -o %[4]s %[6]s
}
`, name, appDisplayName, config.ProjectName, owner, runDir, logDir)
}

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
	if err := os.WriteFile(initPath, []byte(openrcTemplate()), 0755); err != nil {
		return fmt.Errorf("failed to write init script: %w", err)
	}
	if err := runCommand("rc-update", "add", "search", "default"); err != nil {
		return fmt.Errorf("failed to add service to default runlevel: %w", err)
	}
	return nil
}

func (sm *ServiceManager) uninstallOpenRC() error {
	if err := runCommand("rc-service", "search", "stop"); err != nil {
		slog.Warn("failed to stop OpenRC service", "error", err)
	}
	if err := runCommand("rc-update", "del", "search", "default"); err != nil {
		slog.Warn("failed to remove OpenRC service from default runlevel", "error", err)
	}
	if err := os.Remove("/etc/init.d/search"); err != nil && !os.IsNotExist(err) {
		return err
	}
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

// appDisplayName is the human-readable service description used by the
// OpenRC and SysVinit scripts ({app_name} in AI.md PART 24).
const appDisplayName = "Search - Privacy-respecting metasearch engine"

// sysVInitScriptPath is the SysVinit install path.
// Per AI.md PART 24: /etc/init.d/{internal_name} — the same path OpenRC uses;
// only one of the two is installed per host, selected by detection.
const sysVInitScriptPath = "/etc/init.d/search"

// sysVInitTemplate per AI.md PART 24 "SysVinit (legacy Linux, init.d)" —
// EXACT MATCH to spec.
const sysVInitTemplate = `#!/bin/sh
### BEGIN INIT INFO
# Provides:          search
# Required-Start:    $network $remote_fs $syslog
# Required-Stop:     $network $remote_fs $syslog
# Default-Start:     2 3 4 5
# Default-Stop:      0 1 6
# Short-Description: Search - Privacy-respecting metasearch engine
# Description:       Search - Privacy-respecting metasearch engine daemon
### END INIT INFO

NAME=search
DAEMON=/usr/local/bin/search
DAEMON_USER=search
PIDFILE=/var/run/apimgr/search.pid
LOGFILE=/var/log/apimgr/search/server.log

case "$1" in
    start)
        echo "Starting $NAME..."
        mkdir -p $(dirname $PIDFILE) $(dirname $LOGFILE)
        chown -R $DAEMON_USER:$DAEMON_USER $(dirname $PIDFILE) $(dirname $LOGFILE)
        start-stop-daemon --start --quiet --background --make-pidfile \
            --pidfile $PIDFILE --chuid $DAEMON_USER --exec $DAEMON \
            --no-close >> $LOGFILE 2>&1
        ;;
    stop)
        echo "Stopping $NAME..."
        start-stop-daemon --stop --quiet --pidfile $PIDFILE --retry 30
        rm -f $PIDFILE
        ;;
    restart)
        $0 stop
        sleep 1
        $0 start
        ;;
    status)
        if [ -f $PIDFILE ] && kill -0 $(cat $PIDFILE) 2>/dev/null; then
            echo "$NAME is running (pid $(cat $PIDFILE))"
            exit 0
        else
            echo "$NAME is stopped"
            exit 3
        fi
        ;;
    *)
        echo "Usage: $0 {start|stop|restart|status}"
        exit 1
        ;;
esac
exit 0
`

// enableSysVinit registers the init script for boot.
// Per AI.md PART 24 SysVinit "Commands": update-rc.d on Debian-style hosts,
// chkconfig on RHEL-style hosts.
func enableSysVinit() error {
	if _, err := exec.LookPath("update-rc.d"); err == nil {
		return runCommand("update-rc.d", "search", "defaults")
	}
	if _, err := exec.LookPath("chkconfig"); err == nil {
		if err := runCommand("chkconfig", "--add", "search"); err != nil {
			return err
		}
		return runCommand("chkconfig", "search", "on")
	}
	return fmt.Errorf("no SysVinit boot manager found (update-rc.d or chkconfig required)")
}

// disableSysVinit removes the init script from boot.
func disableSysVinit() error {
	if _, err := exec.LookPath("update-rc.d"); err == nil {
		return runCommand("update-rc.d", "-f", "search", "remove")
	}
	if _, err := exec.LookPath("chkconfig"); err == nil {
		return runCommand("chkconfig", "search", "off")
	}
	return fmt.Errorf("no SysVinit boot manager found (update-rc.d or chkconfig required)")
}

func (sm *ServiceManager) installSysVinit() error {
	if err := sm.ensureSystemUser(); err != nil {
		return fmt.Errorf("failed to create system user: %w", err)
	}
	if err := sm.createServiceDirectories(); err != nil {
		return fmt.Errorf("failed to create directories: %w", err)
	}

	logDir := filepath.Join("/var/log", config.InternalOrg, config.InternalName)
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return fmt.Errorf("failed to create log directory: %w", err)
	}
	runDir := filepath.Join("/var/run", config.InternalOrg)
	if err := os.MkdirAll(runDir, 0755); err != nil {
		return fmt.Errorf("failed to create run directory: %w", err)
	}

	if err := os.WriteFile(sysVInitScriptPath, []byte(sysVInitTemplate), 0755); err != nil {
		return fmt.Errorf("failed to write init script: %w", err)
	}
	if err := enableSysVinit(); err != nil {
		return fmt.Errorf("failed to enable service at boot: %w", err)
	}
	return nil
}

func (sm *ServiceManager) uninstallSysVinit() error {
	if err := runCommand(sysVInitScriptPath, "stop"); err != nil {
		slog.Warn("failed to stop SysVinit service", "error", err)
	}
	if err := disableSysVinit(); err != nil {
		slog.Warn("failed to disable SysVinit service", "error", err)
	}
	if err := os.Remove(sysVInitScriptPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// parseSysVinitStatus maps init script status output to a canonical status.
func parseSysVinitStatus(output string) string {
	lower := strings.ToLower(output)
	if strings.Contains(lower, "is running") {
		return "active"
	}
	if strings.Contains(lower, "is stopped") {
		return "inactive"
	}
	return "unknown"
}

func (sm *ServiceManager) statusSysVinit() (string, error) {
	out, err := exec.Command(sysVInitScriptPath, "status").Output()
	if err != nil {
		return "inactive", nil
	}
	return parseSysVinitStatus(string(out)), nil
}

// s6ScanDirs lists the scan directories s6-svscan is conventionally pointed at.
var s6ScanDirs = []string{"/run/service", "/run/s6/service", "/etc/s6/service"}

// s6RunTemplate is the s6 supervised run script.
// Per AI.md PART 23 (s6 is a supported Linux init system) and PART 24's
// privilege model: the binary starts elevated and drops privileges itself.
const s6RunTemplate = `#!/bin/sh
exec 2>&1
exec /usr/local/bin/search
`

// s6LogRunTemplate is the s6 logger run script.
const s6LogRunTemplate = `#!/bin/sh
exec s6-log -b n20 s1000000 T /var/log/apimgr/search
`

// getS6ServiceDir returns the s6 service definition directory.
func (sm *ServiceManager) getS6ServiceDir() string {
	return filepath.Join("/etc/s6/sv", config.InternalName)
}

// getS6ScanDir returns the active s6 scan directory.
func (sm *ServiceManager) getS6ScanDir() string {
	for _, dir := range s6ScanDirs {
		if _, err := os.Stat(dir); err == nil {
			return dir
		}
	}
	return s6ScanDirs[0]
}

// getS6ActivePath returns the supervised service path inside the scan directory.
func (sm *ServiceManager) getS6ActivePath() string {
	return filepath.Join(sm.getS6ScanDir(), config.InternalName)
}

// enableS6 links the service definition into the scan directory and tells the
// running s6-svscan to pick it up.
func (sm *ServiceManager) enableS6() error {
	linkPath := sm.getS6ActivePath()
	if err := os.Remove(linkPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove existing service link: %w", err)
	}
	if err := os.Symlink(sm.getS6ServiceDir(), linkPath); err != nil {
		return fmt.Errorf("failed to enable service (symlink): %w", err)
	}
	// Non-fatal: s6-svscan may not be running yet during an offline install
	if err := runCommand("s6-svscanctl", "-a", sm.getS6ScanDir()); err != nil {
		slog.Debug("Failed to notify s6-svscan (non-fatal, scanner may not be running)", "error", err)
	}
	return nil
}

// disableS6 removes the service from the scan directory.
func (sm *ServiceManager) disableS6() error {
	if err := os.Remove(sm.getS6ActivePath()); err != nil {
		return err
	}
	if err := runCommand("s6-svscanctl", "-a", sm.getS6ScanDir()); err != nil {
		slog.Debug("Failed to notify s6-svscan (non-fatal, scanner may not be running)", "error", err)
	}
	return nil
}

func (sm *ServiceManager) installS6() error {
	if err := sm.ensureSystemUser(); err != nil {
		return fmt.Errorf("failed to create system user: %w", err)
	}
	if err := sm.createServiceDirectories(); err != nil {
		return fmt.Errorf("failed to create directories: %w", err)
	}

	serviceDir := sm.getS6ServiceDir()
	logServiceDir := filepath.Join(serviceDir, "log")
	if err := os.MkdirAll(logServiceDir, 0755); err != nil {
		return fmt.Errorf("failed to create service directory: %w", err)
	}
	logDir := filepath.Join("/var/log", config.InternalOrg, config.InternalName)
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return fmt.Errorf("failed to create log directory: %w", err)
	}

	if err := os.WriteFile(filepath.Join(serviceDir, "run"), []byte(s6RunTemplate), 0755); err != nil {
		return fmt.Errorf("failed to write run script: %w", err)
	}
	if err := os.WriteFile(filepath.Join(logServiceDir, "run"), []byte(s6LogRunTemplate), 0755); err != nil {
		return fmt.Errorf("failed to write log run script: %w", err)
	}

	return sm.enableS6()
}

func (sm *ServiceManager) uninstallS6() error {
	if err := runCommand("s6-svc", "-d", sm.getS6ActivePath()); err != nil {
		slog.Warn("failed to stop s6 service", "error", err)
	}
	if err := os.Remove(sm.getS6ActivePath()); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := runCommand("s6-svscanctl", "-a", sm.getS6ScanDir()); err != nil {
		slog.Warn("failed to reload s6 service directory", "error", err)
	}
	return os.RemoveAll(sm.getS6ServiceDir())
}

// parseS6Status maps raw s6-svstat output to a canonical status string.
func parseS6Status(output string) string {
	trimmed := strings.TrimSpace(output)
	if strings.HasPrefix(trimmed, "up") {
		return "active"
	}
	if strings.HasPrefix(trimmed, "down") {
		return "inactive"
	}
	return "unknown"
}

func (sm *ServiceManager) statusS6() (string, error) {
	out, err := exec.Command("s6-svstat", sm.getS6ActivePath()).Output()
	if err != nil {
		return "inactive", nil
	}
	return parseS6Status(string(out)), nil
}
