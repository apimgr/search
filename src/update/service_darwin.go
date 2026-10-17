//go:build darwin

package update

import (
	"fmt"
	"os"
	"os/exec"
)

// launchdLabel is the launchd job label per AI.md PART 23-24
const launchdLabel = "io.github.apimgr.search"

// launchdPlistPath is the system LaunchDaemon plist installed by --service --install
const launchdPlistPath = "/Library/LaunchDaemons/" + launchdLabel + ".plist"

// restartService restarts the service on macOS.
// Per AI.md PART 22: launchctl kickstart -k stops the running job and starts
// it again in one step.
func restartService() error {
	cmd := exec.Command("launchctl", "kickstart", "-k", "system/"+launchdLabel)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("launchctl kickstart %s failed: %w: %s", launchdLabel, err, string(out))
	}
	return nil
}

// serviceManaged reports whether launchd started this process.
// launchd sets XPC_SERVICE_NAME to the job label; interactive shells inherit
// the placeholder "0" instead.
func serviceManaged() bool {
	name := os.Getenv("XPC_SERVICE_NAME")
	return name != "" && name != "0"
}

// serviceInstalled reports whether the LaunchDaemon plist exists
func serviceInstalled() bool {
	_, err := os.Stat(launchdPlistPath)
	return err == nil
}
