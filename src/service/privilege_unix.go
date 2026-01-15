//go:build !windows

package service

import (
	"fmt"
	"syscall"
)

// dropPrivilegesUnix drops privileges using setgid/setuid syscalls
// Per AI.md PART 8: Step 8g - DROP PRIVILEGES to search user
// Order matters: setgroups, setgid, then setuid
func dropPrivilegesUnix(uid, gid int) error {
	// Clear supplementary groups first
	if err := syscall.Setgroups([]int{gid}); err != nil {
		return fmt.Errorf("failed to set supplementary groups: %w", err)
	}

	// Set GID before UID (can't change GID after dropping root)
	if err := syscall.Setgid(gid); err != nil {
		return fmt.Errorf("failed to setgid(%d): %w", gid, err)
	}

	// Set UID last
	if err := syscall.Setuid(uid); err != nil {
		return fmt.Errorf("failed to setuid(%d): %w", uid, err)
	}

	return nil
}
