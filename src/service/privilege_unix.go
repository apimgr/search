//go:build !windows

package service

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"runtime"
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
	return detectPrivilegeEscalationMethod() != "none"
}

// execElevated re-executes the current binary via sudo or doas with the same arguments.
// Per AI.md PART 7: re-exec self via sudo — never call sudo directly in business logic.
func execElevated() error {
	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to resolve executable path: %w", err)
	}
	args := append([]string{self}, os.Args[1:]...)
	method := detectPrivilegeEscalationMethod()
	var cmd *exec.Cmd
	switch method {
	case "sudo", "doas", "pkexec":
		cmd = exec.Command(method, args...)
	case "su":
		cmd = exec.Command("su", "-c", shellCommand(args))
	case "osascript":
		cmd = exec.Command("osascript", "-e", appleScriptCommand(args))
	default:
		return fmt.Errorf("no privilege escalation method available")
	}
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
	for _, method := range escalationMethodOrder() {
		if method == "sudo" && !hasPrivilegedGroupMembership() {
			continue
		}
		if _, err := exec.LookPath(method); err == nil {
			return method
		}
	}
	return "none"
}

func escalationMethodOrder() []string {
	switch runtime.GOOS {
	case "darwin":
		return []string{"sudo", "osascript"}
	case "freebsd", "openbsd", "netbsd", "dragonfly":
		return []string{"doas", "sudo", "su"}
	default:
		return []string{"sudo", "su", "pkexec", "doas"}
	}
}

func hasPrivilegedGroupMembership() bool {
	current, err := user.Current()
	if err != nil {
		return false
	}
	gids, err := current.GroupIds()
	if err != nil {
		return false
	}
	privilegedGroups := map[string]bool{"wheel": true, "sudo": true, "admin": true}
	for _, gid := range gids {
		group, err := user.LookupGroupId(gid)
		if err == nil && privilegedGroups[group.Name] {
			return true
		}
	}
	return false
}

func shellCommand(args []string) string {
	return shellQuoteArgs(args)
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
	if len(args) == 0 {
		return fmt.Errorf("no command supplied for privilege escalation")
	}
	if isElevated() {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
		return cmd.Run()
	}
	escalator := NewPrivilegeEscalator()
	if !escalator.IsAvailable() {
		return fmt.Errorf("no privilege escalation method available")
	}
	cmd := escalator.EscalateCommand(args[0], args[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
