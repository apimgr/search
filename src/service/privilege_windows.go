//go:build windows

package service

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"golang.org/x/sys/windows"
)

// dropPrivilegesUnix is a no-op on Windows
// Windows uses different security model (impersonation, service accounts)
func dropPrivilegesUnix(uid, gid int) error {
	return nil
}

// isElevated returns true when the process runs with administrator privileges.
// Per AI.md PART 7: platform-independent privilege check (Windows variant).
func isElevated() bool {
	return windows.GetCurrentProcessToken().IsElevated()
}

// canEscalate returns true when UAC elevation is available (process is not already elevated).
// Per AI.md PART 7: on Windows, any non-elevated interactive process can request UAC elevation.
func canEscalate() bool {
	return !isElevated()
}

// execElevated re-executes the current binary with UAC elevation via ShellExecute.
// Per AI.md PART 7: re-exec self via ShellExecute runas — never call sudo directly in business logic.
func execElevated() error {
	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to resolve executable path: %w", err)
	}
	args := strings.Join(os.Args[1:], " ")
	cmd := exec.Command("powershell", "-Command",
		fmt.Sprintf("Start-Process -FilePath '%s' -ArgumentList '%s' -Verb RunAs -Wait",
			self, strings.ReplaceAll(args, "'", "''")))
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// CanEscalate checks if the current process can request UAC elevation.
// Per AI.md PART 23: Smart escalation flow — only prompt if user actually can escalate.
func CanEscalate() bool {
	return canEscalate()
}

// ExecElevated re-executes the given args with UAC elevation via PowerShell ShellExecute.
// Per AI.md PART 23: Re-exec with escalation when user confirms.
func ExecElevated(args []string) error {
	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to resolve executable path: %w", err)
	}
	argsStr := strings.Join(args, " ")
	cmd := exec.Command("powershell", "-Command",
		fmt.Sprintf("Start-Process -FilePath '%s' -ArgumentList '%s' -Verb RunAs -Wait",
			self, strings.ReplaceAll(argsStr, "'", "''")))
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
