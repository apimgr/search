//go:build darwin

package service

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Install installs the launchd service on macOS.
func (sm *ServiceManager) Install() error {
	return sm.installLaunchd()
}

// Uninstall removes the launchd service on macOS.
func (sm *ServiceManager) Uninstall() error {
	return sm.uninstallLaunchd()
}

// Status returns the launchd service status on macOS.
func (sm *ServiceManager) Status() (string, error) {
	return sm.statusLaunchd()
}

// StartAllServices starts the service on macOS.
func (sm *ServiceManager) StartAllServices() error {
	return runCommand("launchctl", "load", sm.getLaunchdPath())
}

// StopAllServices stops the service on macOS.
func (sm *ServiceManager) StopAllServices() error {
	return runCommand("launchctl", "unload", sm.getLaunchdPath())
}

// RestartAllServices restarts the service on macOS.
func (sm *ServiceManager) RestartAllServices() error {
	// Ignore stop errors
	sm.StopAllServices() //nolint:errcheck
	return sm.StartAllServices()
}

// Reload reloads the service on macOS (no native reload; use restart).
func (sm *ServiceManager) Reload() error {
	return sm.RestartAllServices()
}

// Enable enables the service to start on boot on macOS.
func (sm *ServiceManager) Enable() error {
	return runCommand("launchctl", "load", "-w", sm.getLaunchdPath())
}

// Disable disables the service from starting on boot on macOS.
func (sm *ServiceManager) Disable() error {
	return runCommand("launchctl", "unload", "-w", sm.getLaunchdPath())
}

// InstallUserService installs a user-level LaunchAgent on macOS.
// Per AI.md PART 23: Fallback when user cannot or declines to escalate.
func (sm *ServiceManager) InstallUserService() error {
	return sm.installLaunchdUserAgent()
}

// ensureSystemUser creates the macOS service account if it doesn't exist.
func (sm *ServiceManager) ensureSystemUser() error {
	_, err := CreateSystemUser("search")
	return err
}

func (sm *ServiceManager) getLaunchdPath() string {
	return "/Library/LaunchDaemons/io.github.apimgr.search.plist"
}

// launchdTemplate is the macOS launchd plist template.
// Per AI.md PART 25: Service starts as root, binary drops to search user after port binding.
const launchdTemplate = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>io.github.apimgr.search</string>
    <key>ProgramArguments</key>
    <array>
        <string>/usr/local/bin/search</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>/var/log/apimgr/search/stdout.log</string>
    <key>StandardErrorPath</key>
    <string>/var/log/apimgr/search/stderr.log</string>
</dict>
</plist>
`

func (sm *ServiceManager) installLaunchd() error {
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

	plistPath := sm.getLaunchdPath()
	if err := os.WriteFile(plistPath, []byte(launchdTemplate), 0644); err != nil {
		return fmt.Errorf("failed to write plist file: %w", err)
	}
	return nil
}

func (sm *ServiceManager) uninstallLaunchd() error {
	plistPath := sm.getLaunchdPath()
	runCommand("launchctl", "unload", plistPath) //nolint:errcheck
	os.Remove(plistPath)
	return nil
}

func (sm *ServiceManager) statusLaunchd() (string, error) {
	out, err := exec.Command("launchctl", "list", "io.github.apimgr.search").Output()
	if err != nil {
		return "inactive", nil
	}
	if strings.Contains(string(out), "io.github.apimgr.search") {
		return "active", nil
	}
	return "inactive", nil
}

// launchdUserAgentTemplate is the macOS LaunchAgent plist template.
// Per AI.md PART 23/24: User agent runs as calling user under ~/Library/LaunchAgents.
const launchdUserAgentTemplate = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>io.github.apimgr.search</string>
	<key>ProgramArguments</key>
	<array>
		<string>/usr/local/bin/search</string>
	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<true/>
	<key>StandardOutPath</key>
	<string>/tmp/apimgr/search.log</string>
	<key>StandardErrorPath</key>
	<string>/tmp/apimgr/search.err</string>
</dict>
</plist>
`

// installLaunchdUserAgent installs a macOS LaunchAgent (user-level).
// Per AI.md PART 23/24: ~/Library/LaunchAgents/io.github.apimgr.search.plist
func (sm *ServiceManager) installLaunchdUserAgent() error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to determine home directory: %w", err)
	}

	agentDir := filepath.Join(homeDir, "Library", "LaunchAgents")
	if err := os.MkdirAll(agentDir, 0755); err != nil {
		return fmt.Errorf("failed to create LaunchAgents directory: %w", err)
	}

	plistPath := filepath.Join(agentDir, "io.github.apimgr.search.plist")
	if err := os.WriteFile(plistPath, []byte(launchdUserAgentTemplate), 0644); err != nil {
		return fmt.Errorf("failed to write LaunchAgent plist: %w", err)
	}
	return nil
}
