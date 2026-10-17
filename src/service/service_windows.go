//go:build windows

package service

import (
	"fmt"
	"log/slog"
	"os/exec"
	"strings"

	"golang.org/x/sys/windows/svc"

	"github.com/apimgr/search/src/config"
)

// windowsService adapts the server lifecycle to the Windows Service Control Manager.
// Per AI.md PART 24: Windows Service integration uses golang.org/x/sys/windows/svc.
type windowsService struct {
	run      func() error
	shutdown func()
}

// Execute implements svc.Handler and drives the SCM state machine.
// Per AI.md PART 24: accept SERVICE_CONTROL_STOP and SERVICE_CONTROL_SHUTDOWN.
func (ws *windowsService) Execute(args []string, r <-chan svc.ChangeRequest, s chan<- svc.Status) (bool, uint32) {
	s <- svc.Status{State: svc.StartPending}

	runErr := make(chan error, 1)
	go func() { runErr <- ws.run() }()

	s <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}

	for {
		select {
		case err := <-runErr:
			if err != nil {
				slog.Error("Service exited with error", "err", err)
				return false, 1
			}
			return false, 0
		case c := <-r:
			switch c.Cmd {
			case svc.Interrogate:
				s <- c.CurrentStatus
			case svc.Stop, svc.Shutdown:
				s <- svc.Status{State: svc.StopPending}
				ws.shutdown()
				<-runErr
				return false, 0
			}
		}
	}
}

// IsWindowsService reports whether the process was started by the Service Control Manager.
func IsWindowsService() bool {
	inService, err := svc.IsWindowsService()
	if err != nil {
		return false
	}
	return inService
}

// RunAsWindowsService runs the server under the Service Control Manager.
// run blocks until the server stops; shutdown triggers a graceful stop.
// Per AI.md PART 24: Windows Service integration.
func RunAsWindowsService(run func() error, shutdown func()) error {
	return svc.Run(config.InternalName, &windowsService{run: run, shutdown: shutdown})
}

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
	// Per AI.md PART 23/24: Windows services run under a Virtual Service Account
	// (NT SERVICE\{internal_name}) — never LocalSystem, which sc uses by default.
	return runCommand("sc", "create", "search",
		"binPath=", fmt.Sprintf("\"%s\" --config \"%s\"", binary, configDir),
		"DisplayName=", "Search - Privacy-Respecting Metasearch Engine",
		"obj=", "NT SERVICE\\"+config.InternalName,
		"start=", "auto")
}

func (sm *ServiceManager) uninstallWindowsService() error {
	runCommand("sc", "stop", "search") //nolint:errcheck
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
