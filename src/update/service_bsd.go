//go:build freebsd || openbsd || netbsd

package update

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/apimgr/search/src/config"
)

// restartService restarts the service on BSD via the rc.d script
// per AI.md PART 22
func restartService() error {
	cmd := exec.Command("service", config.ProjectName, "restart")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("service %s restart failed: %w: %s", config.ProjectName, err, string(out))
	}
	return nil
}

// serviceManaged reports whether an rc.d script started this process.
// rc.d exports RC_PID into the environment of the services it launches.
func serviceManaged() bool {
	return os.Getenv("RC_PID") != ""
}

// serviceInstalled reports whether the rc.d script exists
func serviceInstalled() bool {
	for _, path := range []string{"/usr/local/etc/rc.d/" + config.ProjectName, "/etc/rc.d/" + config.ProjectName} {
		if _, err := os.Stat(path); err == nil {
			return true
		}
	}
	return false
}
