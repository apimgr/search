package service

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"strconv"
	"strings"
)

// PrivilegeEscalator handles privilege escalation for service installation
// Per AI.md PART 24: Privilege Escalation (NON-NEGOTIABLE)
type PrivilegeEscalator struct {
	// "sudo", "doas", "pkexec", "runas", "none"
	method string
}

// NewPrivilegeEscalator creates a new privilege escalator
// Per AI.md PART 24: Auto-detect available privilege escalation method
func NewPrivilegeEscalator() *PrivilegeEscalator {
	return &PrivilegeEscalator{
		method: detectPrivilegeEscalationMethod(),
	}
}

// Method returns the detected privilege escalation method
func (p *PrivilegeEscalator) Method() string {
	return p.method
}

// IsAvailable returns true if privilege escalation is available
func (p *PrivilegeEscalator) IsAvailable() bool {
	return p.method != "none" || os.Geteuid() == 0
}

// EscalateCommand wraps a command with privilege escalation
// Per AI.md PART 24: Wrap commands appropriately for each method
func (p *PrivilegeEscalator) EscalateCommand(cmd string, args ...string) *exec.Cmd {
	switch p.method {
	case "sudo":
		fullArgs := append([]string{cmd}, args...)
		return exec.Command("sudo", fullArgs...)
	case "doas":
		fullArgs := append([]string{cmd}, args...)
		return exec.Command("doas", fullArgs...)
	case "pkexec":
		fullArgs := append([]string{cmd}, args...)
		return exec.Command("pkexec", fullArgs...)
	case "runas":
		// Windows: use powershell Start-Process with -Verb RunAs
		fullArgs := strings.Join(append([]string{cmd}, args...), " ")
		return exec.Command("powershell", "-Command",
			fmt.Sprintf("Start-Process -FilePath '%s' -ArgumentList '%s' -Verb RunAs -Wait",
				cmd, strings.Replace(fullArgs, "'", "''", -1)))
	default:
		return exec.Command(cmd, args...)
	}
}

// SystemUser represents a system service user
// Per AI.md PART 24: System user creation logic
type SystemUser struct {
	Name  string
	UID   int
	GID   int
	Home  string
	Shell string
}

// reservedSystemIDs contains IDs to avoid per AI.md PART 24
// These are commonly used by system services
var reservedSystemIDs = map[int]bool{
	// nobody/nogroup
	65534: true,
	// docker (common)
	999: true,
	// systemd-coredump
	998: true,
	// systemd-oom
	997: true,
	// systemd-timesync
	996: true,
	// systemd-resolve
	995: true,
	// systemd-network
	994: true,
	// systemd-journal
	993: true,
	// systemd-journal
	101: true,
	// systemd-network
	102: true,
	// systemd-resolve
	103: true,
	// systemd-timesync
	104: true,
	// messagebus
	105: true,
	// sshd
	106: true,
	// tss
	107: true,
	// uuidd
	108: true,
	// tcpdump
	109: true,
	// landscape
	110: true,
	// postgres common
	170: true,
	// redis common
	171: true,
	// mysql common
	172: true,
	// mongodb common
	173: true,
	// elasticsearch
	174: true,
	// kibana
	175: true,
	// logstash
	176: true,
	// nginx
	177: true,
	// www-data
	178: true,
	// apache
	179: true,
}

// findAvailableUnixSystemID finds available ID in range 200-899
// Per AI.md PART 24: Safe system range 200-899 (avoids well-known service IDs)
func findAvailableUnixSystemID() (int, error) {
	usedIDs := make(map[int]bool)

	// Copy reserved IDs
	for id := range reservedSystemIDs {
		usedIDs[id] = true
	}

	// Read /etc/passwd to get used UIDs
	if data, err := os.ReadFile("/etc/passwd"); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			parts := strings.Split(line, ":")
			if len(parts) >= 3 {
				if uid, err := strconv.Atoi(parts[2]); err == nil {
					usedIDs[uid] = true
				}
			}
		}
	}

	// Read /etc/group to get used GIDs
	if data, err := os.ReadFile("/etc/group"); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			parts := strings.Split(line, ":")
			if len(parts) >= 3 {
				if gid, err := strconv.Atoi(parts[2]); err == nil {
					usedIDs[gid] = true
				}
			}
		}
	}

	// Scan descending from 899 to 200 to reserve low IDs for traditional services.
	// Per AI.md PART 23: highest available safe ID selected first.
	for id := 899; id >= 200; id-- {
		if !usedIDs[id] {
			return id, nil
		}
	}

	return 0, fmt.Errorf("no available system ID in range 200-899")
}

// createWindowsVirtualServiceAccount creates a Windows Virtual Service Account
// Per AI.md PART 24: Windows Virtual Service Account support
func createWindowsVirtualServiceAccount(name string) (*SystemUser, error) {
	// Windows Virtual Service Accounts are of the form NT SERVICE\<servicename>
	// They are automatically created when a service is configured to use them
	svcAccount := "NT SERVICE\\" + name

	return &SystemUser{
		Name: svcAccount,
		// Not applicable on Windows
		UID:   0,
		GID:   0,
		Home:  "",
		Shell: "",
	}, nil
}

// UserExists checks if a system user exists
func UserExists(name string) bool {
	_, err := user.Lookup(name)
	return err == nil
}

// IsRunningAsRoot returns true if the process is running with elevated privileges
func IsRunningAsRoot() bool {
	return isElevated()
}

// ReExecWithPrivileges re-executes the current process with elevated privileges.
// Per AI.md PART 7: never call sudo directly in business logic — use this wrapper.
// Returns nil if already elevated. The current process should exit after calling
// this function, as the elevated child process takes over.
func ReExecWithPrivileges() error {
	if os.Geteuid() == 0 {
		return nil
	}

	escalator := NewPrivilegeEscalator()
	if !escalator.IsAvailable() {
		return fmt.Errorf("no privilege escalation method available (not in sudo/wheel group)")
	}

	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to determine executable path: %w", err)
	}

	cmd := escalator.EscalateCommand(self, os.Args[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// handleEscalation manages privilege escalation for service installation operations.
// Per AI.md PART 7: sudo must never be called directly in business logic — use this instead.
// Flow: already elevated → proceed; canEscalate → prompt then re-exec; else → user service path.
func handleEscalation() error {
	if isElevated() {
		return nil
	}
	if !canEscalate() {
		fmt.Println("No admin access available, falling back to user service installation.")
		return nil
	}
	fmt.Print("Install system service? Requires elevated privileges. [Y/n]: ")
	var answer string
	fmt.Scanln(&answer)
	answer = strings.TrimSpace(answer)
	if strings.EqualFold(answer, "n") || strings.EqualFold(answer, "no") {
		fmt.Println("Installing as user service...")
		return nil
	}
	return execElevated()
}

// HandleEscalation is the exported entry point for privilege escalation during service installation.
// Per AI.md PART 7: call sites in main and service code must use this instead of ad-hoc sudo checks.
func HandleEscalation() error {
	return handleEscalation()
}
