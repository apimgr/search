//go:build linux

package update

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/apimgr/search/src/config"
)

// restartService restarts the service on Linux.
// Per AI.md PART 22: systemd is preferred; other supervisors (runit, s6,
// OpenRC) are reached through the generic service command.
func restartService() error {
	if _, err := exec.LookPath("systemctl"); err == nil {
		cmd := exec.Command("systemctl", "restart", config.ProjectName)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("systemctl restart %s failed: %w: %s", config.ProjectName, err, string(out))
		}
		return nil
	}

	if _, err := exec.LookPath("service"); err != nil {
		return fmt.Errorf("no service manager found to restart %s", config.ProjectName)
	}
	cmd := exec.Command("service", config.ProjectName, "restart")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("service %s restart failed: %w: %s", config.ProjectName, err, string(out))
	}
	return nil
}

// serviceManaged reports whether systemd started this process.
// systemd exports INVOCATION_ID (and JOURNAL_STREAM when logging is captured)
// into every unit it launches.
func serviceManaged() bool {
	return os.Getenv("INVOCATION_ID") != "" || os.Getenv("JOURNAL_STREAM") != ""
}

// serviceInstalled reports whether a unit or init script exists for the project
func serviceInstalled() bool {
	if _, err := exec.LookPath("systemctl"); err == nil {
		if err := exec.Command("systemctl", "cat", config.ProjectName).Run(); err == nil {
			return true
		}
	}
	for _, path := range []string{"/etc/init.d/" + config.ProjectName, "/etc/sv/" + config.ProjectName} {
		if _, err := os.Stat(path); err == nil {
			return true
		}
	}
	return false
}
