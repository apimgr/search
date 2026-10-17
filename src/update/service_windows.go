//go:build windows

package update

import (
	"fmt"
	"os/exec"
	"time"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"

	"github.com/apimgr/search/src/config"
)

// restartService restarts the Windows service through the SCM
// per AI.md PART 22
func restartService() error {
	// Stop the service; a stopped service is not an error here
	exec.Command("sc", "stop", config.ProjectName).Run()

	// Wait for the stop to settle before starting again
	time.Sleep(2 * time.Second)

	startCmd := exec.Command("sc", "start", config.ProjectName)
	if out, err := startCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("sc start %s failed: %w: %s", config.ProjectName, err, string(out))
	}
	return nil
}

// serviceManaged reports whether the SCM started this process
func serviceManaged() bool {
	isService, err := svc.IsWindowsService()
	if err != nil {
		return false
	}
	return isService
}

// serviceInstalled reports whether the service is registered with the SCM
func serviceInstalled() bool {
	m, err := mgr.Connect()
	if err != nil {
		return false
	}
	defer m.Disconnect()

	s, err := m.OpenService(config.ProjectName)
	if err != nil {
		return false
	}
	s.Close()
	return true
}
