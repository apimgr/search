package service

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
	"text/template"

	"github.com/apimgr/search/src/config"
)

// ServiceManager handles system service installation
type ServiceManager struct {
	config *config.Config
}

// NewServiceManager creates a new service manager
func NewServiceManager(cfg *config.Config) *ServiceManager {
	return &ServiceManager{config: cfg}
}

// Install installs the system service
func (sm *ServiceManager) Install() error {
	switch runtime.GOOS {
	case "linux":
		// Check for runit first (Void Linux, some Alpine setups)
		if sm.hasRunit() {
			return sm.installRunit()
		}
		return sm.installSystemd()
	case "darwin":
		return sm.installLaunchd()
	case "freebsd", "openbsd", "netbsd":
		return sm.installRCd()
	case "windows":
		return sm.installWindowsService()
	default:
		return fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}
}

// hasRunit checks if runit is the init system
func (sm *ServiceManager) hasRunit() bool {
	// Check for runit by looking for runsv
	if _, err := exec.LookPath("runsv"); err == nil {
		// Also check if /var/service or /service exists (runit service dir)
		if _, err := os.Stat("/var/service"); err == nil {
			return true
		}
		if _, err := os.Stat("/service"); err == nil {
			return true
		}
	}
	return false
}

// Uninstall removes the system service
func (sm *ServiceManager) Uninstall() error {
	switch runtime.GOOS {
	case "linux":
		if sm.hasRunit() {
			return sm.uninstallRunit()
		}
		return sm.uninstallSystemd()
	case "darwin":
		return sm.uninstallLaunchd()
	case "freebsd", "openbsd", "netbsd":
		return sm.uninstallRCd()
	case "windows":
		return sm.uninstallWindowsService()
	default:
		return fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}
}

// Status returns the service status
func (sm *ServiceManager) Status() (string, error) {
	switch runtime.GOOS {
	case "linux":
		if sm.hasRunit() {
			return sm.statusRunit()
		}
		return sm.statusSystemd()
	case "darwin":
		return sm.statusLaunchd()
	case "freebsd", "openbsd", "netbsd":
		return sm.statusRCd()
	case "windows":
		return sm.statusWindowsService()
	default:
		return "", fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}
}

// Start starts the service
func (sm *ServiceManager) Start() error {
	switch runtime.GOOS {
	case "linux":
		if sm.hasRunit() {
			return runCommand("sv", "start", "search")
		}
		return runCommand("systemctl", "start", "search")
	case "darwin":
		return runCommand("launchctl", "load", sm.getLaunchdPath())
	case "freebsd", "openbsd", "netbsd":
		return runCommand("service", "search", "start")
	case "windows":
		return runCommand("sc", "start", "search")
	default:
		return fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}
}

// Stop stops the service
func (sm *ServiceManager) Stop() error {
	switch runtime.GOOS {
	case "linux":
		if sm.hasRunit() {
			return runCommand("sv", "stop", "search")
		}
		return runCommand("systemctl", "stop", "search")
	case "darwin":
		return runCommand("launchctl", "unload", sm.getLaunchdPath())
	case "freebsd", "openbsd", "netbsd":
		return runCommand("service", "search", "stop")
	case "windows":
		return runCommand("sc", "stop", "search")
	default:
		return fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}
}

// Restart restarts the service
func (sm *ServiceManager) Restart() error {
	switch runtime.GOOS {
	case "linux":
		if sm.hasRunit() {
			return runCommand("sv", "restart", "search")
		}
		return runCommand("systemctl", "restart", "search")
	case "darwin":
		if err := sm.Stop(); err != nil {
			// Ignore stop errors
		}
		return sm.Start()
	case "freebsd", "openbsd", "netbsd":
		return runCommand("service", "search", "restart")
	case "windows":
		if err := sm.Stop(); err != nil {
			// Ignore stop errors
		}
		return sm.Start()
	default:
		return fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}
}

// Reload reloads the service configuration (sends SIGHUP)
func (sm *ServiceManager) Reload() error {
	switch runtime.GOOS {
	case "linux":
		if sm.hasRunit() {
			return runCommand("sv", "hup", "search")
		}
		return runCommand("systemctl", "reload", "search")
	case "darwin":
		// macOS doesn't have a standard reload, use restart
		return sm.Restart()
	case "freebsd", "openbsd", "netbsd":
		return runCommand("service", "search", "reload")
	case "windows":
		// Windows doesn't support reload, use restart
		return sm.Restart()
	default:
		return fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}
}

// Enable enables the service to start on boot
func (sm *ServiceManager) Enable() error {
	switch runtime.GOOS {
	case "linux":
		if sm.hasRunit() {
			// For runit, enabling means creating the symlink
			serviceDir := sm.getRunitServiceDir()
			activeDir := sm.getRunitActiveDir()
			linkPath := filepath.Join(activeDir, "search")
			os.Remove(linkPath) // Remove if exists
			return os.Symlink(serviceDir, linkPath)
		}
		return runCommand("systemctl", "enable", "search")
	case "darwin":
		return runCommand("launchctl", "load", "-w", sm.getLaunchdPath())
	case "freebsd", "openbsd", "netbsd":
		// Add search_enable="YES" to rc.conf
		return appendToFile("/etc/rc.conf", "search_enable=\"YES\"\n")
	case "windows":
		return runCommand("sc", "config", "search", "start=", "auto")
	default:
		return fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}
}

// Disable disables the service from starting on boot
func (sm *ServiceManager) Disable() error {
	switch runtime.GOOS {
	case "linux":
		if sm.hasRunit() {
			// For runit, disabling means removing the symlink
			activeDir := sm.getRunitActiveDir()
			return os.Remove(filepath.Join(activeDir, "search"))
		}
		return runCommand("systemctl", "disable", "search")
	case "darwin":
		return runCommand("launchctl", "unload", "-w", sm.getLaunchdPath())
	case "freebsd", "openbsd", "netbsd":
		// Remove search_enable from rc.conf (or set to NO)
		return runCommand("sysrc", "search_enable=NO")
	case "windows":
		return runCommand("sc", "config", "search", "start=", "disabled")
	default:
		return fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}
}

// Linux systemd

// systemdTemplate per AI.md PART 25 - EXACT MATCH to spec
const systemdTemplate = `[Unit]
Description=search service
Documentation=https://apimgr.github.io/search
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=search
Group=search
ExecStart=/usr/local/bin/search
Restart=on-failure
RestartSec=5
StandardOutput=journal
StandardError=journal

# Security hardening
NoNewPrivileges=yes
ProtectSystem=strict
ProtectHome=yes
PrivateTmp=yes
ReadWritePaths=/etc/apimgr/search
ReadWritePaths=/var/lib/apimgr/search
ReadWritePaths=/var/cache/apimgr/search
ReadWritePaths=/var/log/apimgr/search

[Install]
WantedBy=multi-user.target
`

func (sm *ServiceManager) installSystemd() error {
	// Per AI.md PART 25: Template uses User=search, so user must exist
	// Create system user and directories before installing service
	if err := sm.ensureSystemUser(); err != nil {
		return fmt.Errorf("failed to create system user: %w", err)
	}

	if err := sm.createServiceDirectories(); err != nil {
		return fmt.Errorf("failed to create directories: %w", err)
	}

	// Generate service file (no template vars needed - hardcoded per spec)
	content := systemdTemplate

	// Write service file
	servicePath := "/etc/systemd/system/search.service"
	if err := os.WriteFile(servicePath, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write service file: %w", err)
	}

	// Reload systemd
	if err := runCommand("systemctl", "daemon-reload"); err != nil {
		return fmt.Errorf("failed to reload systemd: %w", err)
	}

	// Enable service
	if err := runCommand("systemctl", "enable", "search"); err != nil {
		return fmt.Errorf("failed to enable service: %w", err)
	}

	return nil
}

// ensureSystemUser creates the system user and group if they don't exist
// Per AI.md PART 25: System user with home in /var/lib/apimgr/search
func (sm *ServiceManager) ensureSystemUser() error {
	userName := "search"
	homeDir := "/var/lib/apimgr/search"

	// Check if user already exists
	if _, err := user.Lookup(userName); err == nil {
		return nil // User exists
	}

	// Create system group first
	if err := runCommand("groupadd", "--system", userName); err != nil {
		// Ignore error if group already exists
	}

	// Create system user with home in data directory (not /home)
	// This works with ProtectHome=yes in systemd
	return runCommand("useradd",
		"--system",
		"--no-create-home",
		"--home-dir", homeDir,
		"--shell", "/bin/false",
		"--gid", userName,
		userName,
	)
}

// createServiceDirectories creates directories needed by the service
// Per AI.md: config, data, cache, log directories
func (sm *ServiceManager) createServiceDirectories() error {
	dirs := []string{
		config.GetConfigDir(),
		config.GetDataDir(),
		config.GetCacheDir(),
		config.GetLogDir(),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create %s: %w", dir, err)
		}
		// Set ownership to service user
		if err := runCommand("chown", "-R", "search:search", dir); err != nil {
			// Non-fatal: chown might fail if running as non-root
		}
	}

	return nil
}

func (sm *ServiceManager) uninstallSystemd() error {
	// Stop service
	runCommand("systemctl", "stop", "search")

	// Disable service
	runCommand("systemctl", "disable", "search")

	// Remove service file
	os.Remove("/etc/systemd/system/search.service")

	// Reload systemd
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

// Linux runit - per AI.md PART 25 EXACT MATCH

const runitRunTemplate = `#!/bin/sh
exec chpst -u search:search /usr/local/bin/search 2>&1
`

const runitLogRunTemplate = `#!/bin/sh
exec svlogd -tt /var/log/apimgr/search
`

func (sm *ServiceManager) getRunitServiceDir() string {
	// Prefer /etc/sv (Void Linux, Artix)
	if _, err := os.Stat("/etc/sv"); err == nil {
		return "/etc/sv/search"
	}
	// Fallback to /etc/runit/sv (some distros)
	return "/etc/runit/sv/search"
}

func (sm *ServiceManager) getRunitActiveDir() string {
	// Check for /var/service (most common)
	if _, err := os.Stat("/var/service"); err == nil {
		return "/var/service"
	}
	// Check for /service (Void Linux alternative)
	if _, err := os.Stat("/service"); err == nil {
		return "/service"
	}
	return "/var/service"
}

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

	// Create service directory structure
	if err := os.MkdirAll(serviceDir, 0755); err != nil {
		return fmt.Errorf("failed to create service directory: %w", err)
	}
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return fmt.Errorf("failed to create log directory: %w", err)
	}
	if err := os.MkdirAll(data["LogDir"], 0755); err != nil {
		return fmt.Errorf("failed to create log storage directory: %w", err)
	}

	// Generate and write run script
	runContent, err := sm.renderTemplate(runitRunTemplate, data)
	if err != nil {
		return err
	}
	runPath := filepath.Join(serviceDir, "run")
	if err := os.WriteFile(runPath, []byte(runContent), 0755); err != nil {
		return fmt.Errorf("failed to write run script: %w", err)
	}

	// Generate and write log/run script
	logRunContent, err := sm.renderTemplate(runitLogRunTemplate, data)
	if err != nil {
		return err
	}
	logRunPath := filepath.Join(logDir, "run")
	if err := os.WriteFile(logRunPath, []byte(logRunContent), 0755); err != nil {
		return fmt.Errorf("failed to write log run script: %w", err)
	}

	// Create symlink to enable the service
	activeDir := sm.getRunitActiveDir()
	linkPath := filepath.Join(activeDir, "search")

	// Remove existing symlink if present
	os.Remove(linkPath)

	if err := os.Symlink(serviceDir, linkPath); err != nil {
		return fmt.Errorf("failed to enable service (symlink): %w", err)
	}

	return nil
}

func (sm *ServiceManager) uninstallRunit() error {
	// Stop the service first
	runCommand("sv", "stop", "search")

	// Remove the symlink from active services
	activeDir := sm.getRunitActiveDir()
	os.Remove(filepath.Join(activeDir, "search"))

	// Remove the service directory
	serviceDir := sm.getRunitServiceDir()
	os.RemoveAll(serviceDir)

	return nil
}

func (sm *ServiceManager) statusRunit() (string, error) {
	out, err := exec.Command("sv", "status", "search").Output()
	if err != nil {
		return "inactive", nil
	}
	output := string(out)
	if strings.HasPrefix(output, "run:") {
		return "active", nil
	}
	if strings.HasPrefix(output, "down:") {
		return "inactive", nil
	}
	return "unknown", nil
}

// macOS launchd

// launchdTemplate is the macOS launchd plist template
// Per AI.md PART 25: launchd plist MUST include UserName/GroupName
const launchdTemplate = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.apimgr.search</string>
    <key>UserName</key>
    <string>{{.User}}</string>
    <key>GroupName</key>
    <string>{{.Group}}</string>
    <key>ProgramArguments</key>
    <array>
        <string>{{.Binary}}</string>
        <string>--config</string>
        <string>{{.ConfigDir}}</string>
    </array>
    <key>WorkingDirectory</key>
    <string>{{.WorkDir}}</string>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>{{.LogDir}}/search.log</string>
    <key>StandardErrorPath</key>
    <string>{{.LogDir}}/search-error.log</string>
</dict>
</plist>
`

func (sm *ServiceManager) getLaunchdPath() string {
	return "/Library/LaunchDaemons/com.apimgr.search.plist"
}

func (sm *ServiceManager) installLaunchd() error {
	// Per AI.md PART 25: launchd plist must include UserName/GroupName
	data := map[string]string{
		"User":      "_search",
		"Group":     "_search",
		"Binary":    config.GetBinaryPath(),
		"ConfigDir": config.GetConfigDir(),
		"WorkDir":   config.GetDataDir(),
		"LogDir":    filepath.Join(config.GetDataDir(), "logs"),
	}

	// Generate plist
	content, err := sm.renderTemplate(launchdTemplate, data)
	if err != nil {
		return err
	}

	// Write plist file
	plistPath := sm.getLaunchdPath()
	if err := os.WriteFile(plistPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write plist file: %w", err)
	}

	return nil
}

func (sm *ServiceManager) uninstallLaunchd() error {
	plistPath := sm.getLaunchdPath()

	// Unload service
	runCommand("launchctl", "unload", plistPath)

	// Remove plist
	os.Remove(plistPath)

	return nil
}

func (sm *ServiceManager) statusLaunchd() (string, error) {
	out, err := exec.Command("launchctl", "list", "com.apimgr.search").Output()
	if err != nil {
		return "inactive", nil
	}
	if strings.Contains(string(out), "com.apimgr.search") {
		return "active", nil
	}
	return "inactive", nil
}

// BSD rc.d - per AI.md PART 25 EXACT MATCH

const rcdTemplate = `#!/bin/sh

# PROVIDE: search
# REQUIRE: NETWORKING
# KEYWORD: shutdown

. /etc/rc.subr

name="search"
rcvar="search_enable"
command="/usr/local/bin/search"
search_user="search"

load_rc_config $name
run_rc_command "$1"
`

func (sm *ServiceManager) installRCd() error {
	// Per AI.md PART 25: Create user and directories first
	if err := sm.ensureSystemUser(); err != nil {
		return fmt.Errorf("failed to create system user: %w", err)
	}
	if err := sm.createServiceDirectories(); err != nil {
		return fmt.Errorf("failed to create directories: %w", err)
	}

	// Write rc script - per AI.md PART 25: /usr/local/etc/rc.d/search
	rcPath := "/usr/local/etc/rc.d/search"
	if err := os.WriteFile(rcPath, []byte(rcdTemplate), 0755); err != nil {
		return fmt.Errorf("failed to write rc script: %w", err)
	}

	return nil
}

func (sm *ServiceManager) uninstallRCd() error {
	runCommand("service", "search", "stop")
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

// Windows service

func (sm *ServiceManager) installWindowsService() error {
	binary := config.GetBinaryPath()
	configDir := config.GetConfigDir()

	// Create Windows service
	return runCommand("sc", "create", "search",
		"binPath=", fmt.Sprintf("\"%s\" --config \"%s\"", binary, configDir),
		"DisplayName=", "Search - Privacy-Respecting Metasearch Engine",
		"start=", "auto")
}

func (sm *ServiceManager) uninstallWindowsService() error {
	runCommand("sc", "stop", "search")
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

// Helper methods

func (sm *ServiceManager) renderTemplate(tmpl string, data interface{}) (string, error) {
	t, err := template.New("service").Parse(tmpl)
	if err != nil {
		return "", err
	}

	var buf strings.Builder
	if err := t.Execute(&buf, data); err != nil {
		return "", err
	}

	return buf.String(), nil
}

func runCommand(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func appendToFile(path, content string) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(content)
	return err
}

// GetServiceStatus returns formatted service status information
func GetServiceStatus() string {
	cfg := config.DefaultConfig()
	sm := NewServiceManager(cfg)

	status, err := sm.Status()
	if err != nil {
		return fmt.Sprintf("Service status: unknown (%v)", err)
	}

	return fmt.Sprintf("Service status: %s", status)
}
