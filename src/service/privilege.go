package service

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"runtime"
	"strconv"
	"strings"
)

// PrivilegeEscalator handles privilege escalation for service installation
// Per AI.md PART 24: Privilege Escalation (NON-NEGOTIABLE)
type PrivilegeEscalator struct {
	method string // "sudo", "doas", "pkexec", "runas", "none"
}

// NewPrivilegeEscalator creates a new privilege escalator
// Per AI.md PART 24: Auto-detect available privilege escalation method
func NewPrivilegeEscalator() *PrivilegeEscalator {
	return &PrivilegeEscalator{
		method: detectPrivilegeEscalationMethod(),
	}
}

// detectPrivilegeEscalationMethod detects available privilege escalation tool
// Per AI.md PART 24: Support sudo, doas, pkexec in that order
func detectPrivilegeEscalationMethod() string {
	if runtime.GOOS == "windows" {
		return "runas"
	}

	// Check if already running as root
	if os.Geteuid() == 0 {
		return "none"
	}

	// Check for sudo (most common on Linux)
	if _, err := exec.LookPath("sudo"); err == nil {
		return "sudo"
	}

	// Check for doas (OpenBSD, some Linux distros)
	if _, err := exec.LookPath("doas"); err == nil {
		return "doas"
	}

	// Check for pkexec (PolicyKit, desktop Linux)
	if _, err := exec.LookPath("pkexec"); err == nil {
		return "pkexec"
	}

	return "none"
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

// FindAvailableSystemID finds an available UID/GID in the system range
// Per AI.md PART 24: UID/GID 100-999 for system accounts
func FindAvailableSystemID() (int, error) {
	switch runtime.GOOS {
	case "linux", "freebsd", "openbsd", "netbsd":
		return findAvailableUnixSystemID()
	case "darwin":
		return findAvailableMacOSSystemID()
	case "windows":
		// Windows doesn't use numeric IDs
		return 0, nil
	default:
		return 0, fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
}

// reservedSystemIDs contains IDs to avoid per AI.md PART 24
// These are commonly used by system services
var reservedSystemIDs = map[int]bool{
	65534: true, // nobody/nogroup
	999:   true, // docker (common)
	998:   true, // systemd-coredump
	997:   true, // systemd-oom
	996:   true, // systemd-timesync
	995:   true, // systemd-resolve
	994:   true, // systemd-network
	993:   true, // systemd-journal
	101:   true, // systemd-journal
	102:   true, // systemd-network
	103:   true, // systemd-resolve
	104:   true, // systemd-timesync
	105:   true, // messagebus
	106:   true, // sshd
	107:   true, // tss
	108:   true, // uuidd
	109:   true, // tcpdump
	110:   true, // landscape
	170:   true, // postgres common
	171:   true, // redis common
	172:   true, // mysql common
	173:   true, // mongodb common
	174:   true, // elasticsearch
	175:   true, // kibana
	176:   true, // logstash
	177:   true, // nginx
	178:   true, // www-data
	179:   true, // apache
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

	// Find first available ID in safe system range (200-899)
	// Per AI.md PART 24: Avoids 100-199 (well-known) and 900-999 (docker, etc.)
	for id := 200; id < 900; id++ {
		if !usedIDs[id] {
			return id, nil
		}
	}

	return 0, fmt.Errorf("no available system ID in range 200-899")
}

// findAvailableMacOSSystemID finds available ID for macOS (uses _underscored users)
// Per AI.md PART 24: macOS safe range is 200-399
func findAvailableMacOSSystemID() (int, error) {
	// macOS uses dscl for user management
	usedIDs := make(map[int]bool)

	// Copy reserved IDs
	for id := range reservedSystemIDs {
		usedIDs[id] = true
	}

	// Use dscl to list users and their UIDs
	cmd := exec.Command("dscl", ".", "-list", "/Users", "UniqueID")
	output, err := cmd.Output()
	if err == nil {
		for _, line := range strings.Split(string(output), "\n") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				if uid, err := strconv.Atoi(parts[1]); err == nil {
					usedIDs[uid] = true
				}
			}
		}
	}

	// Find first available ID in macOS safe range (200-399)
	// Per AI.md PART 24: macOS safe range is 200-399
	for id := 200; id < 400; id++ {
		if !usedIDs[id] {
			return id, nil
		}
	}

	return 0, fmt.Errorf("no available system ID in range 200-399")
}

// CreateSystemUser creates a system user for the service
// Per AI.md PART 24: System user creation logic
func CreateSystemUser(name string) (*SystemUser, error) {
	switch runtime.GOOS {
	case "linux":
		return createLinuxSystemUser(name)
	case "darwin":
		return createMacOSServiceAccount(name)
	case "freebsd":
		return createFreeBSDSystemUser(name)
	case "windows":
		return createWindowsVirtualServiceAccount(name)
	default:
		return nil, fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
}

// createLinuxSystemUser creates a Linux system user
func createLinuxSystemUser(name string) (*SystemUser, error) {
	// Check if user already exists
	if u, err := user.Lookup(name); err == nil {
		uid, _ := strconv.Atoi(u.Uid)
		gid, _ := strconv.Atoi(u.Gid)
		return &SystemUser{
			Name:  name,
			UID:   uid,
			GID:   gid,
			Home:  u.HomeDir,
			Shell: "/bin/false",
		}, nil
	}

	// Find available ID
	id, err := FindAvailableSystemID()
	if err != nil {
		return nil, fmt.Errorf("failed to find available ID: %w", err)
	}

	// Create group first
	cmd := exec.Command("groupadd", "-r", "-g", strconv.Itoa(id), name)
	if err := cmd.Run(); err != nil {
		// Try without explicit GID
		cmd = exec.Command("groupadd", "-r", name)
		if err := cmd.Run(); err != nil {
			return nil, fmt.Errorf("failed to create group: %w", err)
		}
	}

	// Create user
	cmd = exec.Command("useradd", "-r",
		"-u", strconv.Itoa(id),
		"-g", name,
		"-d", "/var/lib/"+name,
		"-s", "/bin/false",
		"-c", name+" service account",
		name)
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	return &SystemUser{
		Name:  name,
		UID:   id,
		GID:   id,
		Home:  "/var/lib/" + name,
		Shell: "/bin/false",
	}, nil
}

// createMacOSServiceAccount creates a macOS service account
// Per AI.md PART 24: macOS service account creation
func createMacOSServiceAccount(name string) (*SystemUser, error) {
	// macOS uses _underscored names for system accounts
	svcName := "_" + name

	// Check if user already exists
	cmd := exec.Command("dscl", ".", "-read", "/Users/"+svcName)
	if err := cmd.Run(); err == nil {
		// User exists, get details
		uidCmd := exec.Command("dscl", ".", "-read", "/Users/"+svcName, "UniqueID")
		output, _ := uidCmd.Output()
		uid := 0
		if len(output) > 0 {
			parts := strings.Fields(string(output))
			if len(parts) >= 2 {
				uid, _ = strconv.Atoi(parts[1])
			}
		}
		return &SystemUser{
			Name:  svcName,
			UID:   uid,
			GID:   uid,
			Home:  "/var/empty",
			Shell: "/usr/bin/false",
		}, nil
	}

	// Find available ID
	id, err := FindAvailableSystemID()
	if err != nil {
		return nil, fmt.Errorf("failed to find available ID: %w", err)
	}

	// Create group
	cmds := [][]string{
		{"dscl", ".", "-create", "/Groups/" + svcName},
		{"dscl", ".", "-create", "/Groups/" + svcName, "PrimaryGroupID", strconv.Itoa(id)},
		{"dscl", ".", "-create", "/Groups/" + svcName, "RealName", name + " service account"},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		if err := cmd.Run(); err != nil {
			return nil, fmt.Errorf("failed to create group: %w", err)
		}
	}

	// Create user
	// Per AI.md PART 24: macOS service accounts must be hidden from login screen
	cmds = [][]string{
		{"dscl", ".", "-create", "/Users/" + svcName},
		{"dscl", ".", "-create", "/Users/" + svcName, "UniqueID", strconv.Itoa(id)},
		{"dscl", ".", "-create", "/Users/" + svcName, "PrimaryGroupID", strconv.Itoa(id)},
		{"dscl", ".", "-create", "/Users/" + svcName, "UserShell", "/usr/bin/false"},
		{"dscl", ".", "-create", "/Users/" + svcName, "NFSHomeDirectory", "/var/empty"},
		{"dscl", ".", "-create", "/Users/" + svcName, "RealName", name + " service account"},
		{"dscl", ".", "-create", "/Users/" + svcName, "IsHidden", "1"},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		if err := cmd.Run(); err != nil {
			return nil, fmt.Errorf("failed to create user: %w", err)
		}
	}

	return &SystemUser{
		Name:  svcName,
		UID:   id,
		GID:   id,
		Home:  "/var/empty",
		Shell: "/usr/bin/false",
	}, nil
}

// createFreeBSDSystemUser creates a FreeBSD system user
func createFreeBSDSystemUser(name string) (*SystemUser, error) {
	// Check if user already exists
	if u, err := user.Lookup(name); err == nil {
		uid, _ := strconv.Atoi(u.Uid)
		gid, _ := strconv.Atoi(u.Gid)
		return &SystemUser{
			Name:  name,
			UID:   uid,
			GID:   gid,
			Home:  u.HomeDir,
			Shell: "/usr/sbin/nologin",
		}, nil
	}

	// Find available ID
	id, err := FindAvailableSystemID()
	if err != nil {
		return nil, fmt.Errorf("failed to find available ID: %w", err)
	}

	// Create user with pw command
	cmd := exec.Command("pw", "useradd", name,
		"-u", strconv.Itoa(id),
		"-g", strconv.Itoa(id),
		"-d", "/var/db/"+name,
		"-s", "/usr/sbin/nologin",
		"-c", name+" service account")
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	return &SystemUser{
		Name:  name,
		UID:   id,
		GID:   id,
		Home:  "/var/db/" + name,
		Shell: "/usr/sbin/nologin",
	}, nil
}

// createWindowsVirtualServiceAccount creates a Windows Virtual Service Account
// Per AI.md PART 24: Windows Virtual Service Account support
func createWindowsVirtualServiceAccount(name string) (*SystemUser, error) {
	// Windows Virtual Service Accounts are of the form NT SERVICE\<servicename>
	// They are automatically created when a service is configured to use them
	svcAccount := "NT SERVICE\\" + name

	return &SystemUser{
		Name:  svcAccount,
		UID:   0, // Not applicable on Windows
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

// GetServiceUser returns the appropriate service user name for the current OS
// Per AI.md PART 24: OS-specific user naming
func GetServiceUser(serviceName string) string {
	switch runtime.GOOS {
	case "darwin":
		return "_" + serviceName
	case "windows":
		return "NT SERVICE\\" + serviceName
	default:
		return serviceName
	}
}

// GetServiceGroup returns the appropriate service group name for the current OS
func GetServiceGroup(serviceName string) string {
	switch runtime.GOOS {
	case "darwin":
		return "_" + serviceName
	case "windows":
		return "" // Not applicable
	default:
		return serviceName
	}
}

// DropPrivileges drops from root to the specified user
// Per AI.md PART 8: Step 8g - DROP PRIVILEGES to search user
// This uses syscall.Setgid and syscall.Setuid to drop privileges
func DropPrivileges(userName string) error {
	if runtime.GOOS == "windows" {
		// Windows doesn't support setuid/setgid in the same way
		return nil
	}

	// Check if we're running as root
	if os.Geteuid() != 0 {
		// Already running as non-root, nothing to do
		return nil
	}

	// Look up the user
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

	// Drop privileges using syscall
	return dropPrivilegesUnix(uid, gid)
}

// IsRunningAsRoot returns true if the process is running as root
func IsRunningAsRoot() bool {
	return os.Geteuid() == 0
}

// VerifyPrivilegesDropped verifies that privileges have been dropped
// Per AI.md PART 8: Step 8h - Verify privilege drop succeeded
func VerifyPrivilegesDropped() error {
	if runtime.GOOS == "windows" {
		return nil
	}

	if os.Geteuid() == 0 {
		return fmt.Errorf("privilege drop failed: still running as root (euid=0)")
	}

	return nil
}
