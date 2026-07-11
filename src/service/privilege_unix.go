//go:build !windows

package service

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"strconv"
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

// isElevated returns true when the process effective UID is 0 (root).
// Per AI.md PART 7: platform-independent privilege check (Unix variant).
func isElevated() bool {
	return os.Geteuid() == 0
}

// canEscalate returns true when a privilege escalation tool or privileged group membership exists.
// Per AI.md PART 7: checks sudo/doas availability and wheel/sudo/admin group membership.
func canEscalate() bool {
	for _, tool := range []string{"sudo", "doas"} {
		if _, err := exec.LookPath(tool); err == nil {
			return true
		}
	}
	u, err := user.Current()
	if err != nil {
		return false
	}
	gids, err := u.GroupIds()
	if err != nil {
		return false
	}
	privilegedGroups := map[string]bool{"wheel": true, "sudo": true, "admin": true}
	for _, gid := range gids {
		g, err := user.LookupGroupId(gid)
		if err != nil {
			continue
		}
		if privilegedGroups[g.Name] {
			return true
		}
	}
	return false
}

// execElevated re-executes the current binary via sudo or doas with the same arguments.
// Per AI.md PART 7: re-exec self via sudo — never call sudo directly in business logic.
func execElevated() error {
	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to resolve executable path: %w", err)
	}
	var tool string
	for _, t := range []string{"sudo", "doas"} {
		if _, err := exec.LookPath(t); err == nil {
			tool = t
			break
		}
	}
	if tool == "" {
		return fmt.Errorf("no privilege escalation tool available (sudo/doas not found)")
	}
	args := append([]string{self}, os.Args[1:]...)
	cmd := exec.Command(tool, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// detectPrivilegeEscalationMethod detects available privilege escalation tool.
// Per AI.md PART 24: Support sudo, doas, pkexec in that order.
func detectPrivilegeEscalationMethod() string {
	if os.Geteuid() == 0 {
		return "none"
	}
	if _, err := exec.LookPath("sudo"); err == nil {
		return "sudo"
	}
	if _, err := exec.LookPath("doas"); err == nil {
		return "doas"
	}
	if _, err := exec.LookPath("pkexec"); err == nil {
		return "pkexec"
	}
	return "none"
}

// DropPrivileges drops from root to the specified user.
// Per AI.md PART 8: Step 8g - DROP PRIVILEGES to search user.
func DropPrivileges(userName string) error {
	if os.Geteuid() != 0 {
		return nil
	}
	u, err := user.Lookup(userName)
	if err != nil {
		return fmt.Errorf("failed to lookup user %s: %w", userName, err)
	}
	uid, err := strconv.Atoi(u.Uid)
	if err != nil {
		return fmt.Errorf("invalid UID for user %s: %w", userName, err)
	}
	gid, err := strconv.Atoi(u.Gid)
	if err != nil {
		return fmt.Errorf("invalid GID for user %s: %w", userName, err)
	}
	return dropPrivilegesUnix(uid, gid)
}

// VerifyPrivilegesDropped verifies that privileges have been dropped.
// Per AI.md PART 8: Step 8h - Verify privilege drop succeeded.
func VerifyPrivilegesDropped() error {
	if os.Geteuid() == 0 {
		return fmt.Errorf("privilege drop failed: still running as root (euid=0)")
	}
	return nil
}

// CanEscalate checks if the current user can escalate privileges.
// Per AI.md PART 23: Smart escalation flow — only prompt if user actually can escalate.
func CanEscalate() bool {
	return canEscalate()
}

// ExecElevated re-executes the given binary path with the provided args under sudo/doas.
// Per AI.md PART 23: Re-exec with escalation tool when user confirms escalation.
func ExecElevated(args []string) error {
	var tool string
	for _, t := range []string{"sudo", "doas"} {
		if _, err := exec.LookPath(t); err == nil {
			tool = t
			break
		}
	}
	if tool == "" {
		return fmt.Errorf("no privilege escalation tool available (sudo/doas not found)")
	}
	sudoArgs := append([]string{tool}, args...)
	cmd := exec.Command(sudoArgs[0], sudoArgs[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
