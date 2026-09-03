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

// detectPrivilegeEscalationMethod returns "runas" on Windows (UAC elevation).
// Per AI.md PART 24: Windows uses ShellExecute runas for UAC.
func detectPrivilegeEscalationMethod() string {
	return "runas"
}

// DropPrivileges is a no-op on Windows (different security model).
// Per AI.md PART 8: Windows uses service accounts, not setuid/setgid.
func DropPrivileges(userName string) error {
	return nil
}

// VerifyPrivilegesDropped is a no-op on Windows.
// Per AI.md PART 8: Windows does not use Unix privilege drop model.
func VerifyPrivilegesDropped() error {
	return nil
}

// GetServiceUser returns the NT Virtual Service Account name for the given service.
// Per AI.md PART 24: Windows uses NT SERVICE\<name> virtual accounts.
func GetServiceUser(serviceName string) string {
	return "NT SERVICE\\" + serviceName
}

// GetServiceGroup returns an empty string on Windows (no group concept for virtual accounts).
// Per AI.md PART 24: Windows virtual service accounts have no separate group.
func GetServiceGroup(serviceName string) string {
	return ""
}

// FindAvailableSystemID returns 0 on Windows — virtual service accounts have no
// traditional UID/GID. Per AI.md PART 24: Windows uses NT SERVICE\ virtual accounts.
func FindAvailableSystemID() (int, error) {
	return 0, nil
}

// CreateSystemUser creates a Windows virtual service account wrapper.
// Per AI.md PART 24: No actual OS-level user is created on Windows.
func CreateSystemUser(name string) (*SystemUser, error) {
	return createWindowsVirtualServiceAccount(name)
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
