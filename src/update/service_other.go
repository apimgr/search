//go:build !linux && !darwin && !windows && !freebsd && !openbsd && !netbsd

package update

import (
	"fmt"
	"runtime"
)

// restartService reports that no service manager integration exists for this
// platform; the operator restarts the process manually
func restartService() error {
	return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
}

// serviceManaged is always false where no service manager is supported
func serviceManaged() bool {
	return false
}

// serviceInstalled is always false where no service manager is supported
func serviceInstalled() bool {
	return false
}
