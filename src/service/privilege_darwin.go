//go:build darwin

package service

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// GetServiceUser returns the macOS service user name (underscore prefix convention).
// Per AI.md PART 24: macOS daemon accounts use the _name convention.
func GetServiceUser(serviceName string) string {
	return "_" + serviceName
}

// GetServiceGroup returns the macOS service group name (underscore prefix convention).
// Per AI.md PART 24: macOS daemon accounts use the _name convention.
func GetServiceGroup(serviceName string) string {
	return "_" + serviceName
}

// FindAvailableSystemID finds an available UID/GID in the macOS safe range (200–399).
// Per AI.md PART 24: macOS uses dscl and a separate ID range from Linux/BSD.
func FindAvailableSystemID() (int, error) {
	return findAvailableMacOSSystemID()
}

// CreateSystemUser creates a macOS service account.
// Per AI.md PART 24: System user creation logic.
func CreateSystemUser(name string) (*SystemUser, error) {
	return createMacOSServiceAccount(name)
}

// findAvailableMacOSSystemID finds an available ID for macOS (uses _underscored users).
// Per AI.md PART 24: macOS safe range is 200–399.
func findAvailableMacOSSystemID() (int, error) {
	usedIDs := make(map[int]bool)

	for id := range reservedSystemIDs {
		usedIDs[id] = true
	}

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

	// Scan descending to prefer higher IDs.
	// Per AI.md PART 23: highest available safe ID selected first.
	for id := 399; id >= 200; id-- {
		if !usedIDs[id] {
			return id, nil
		}
	}
	return 0, fmt.Errorf("no available system ID in range 200-399")
}

// createMacOSServiceAccount creates a macOS service account via dscl.
// Per AI.md PART 24: macOS service accounts must be hidden from login screen.
func createMacOSServiceAccount(name string) (*SystemUser, error) {
	svcName := "_" + name

	cmd := exec.Command("dscl", ".", "-read", "/Users/"+svcName)
	if err := cmd.Run(); err == nil {
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

	id, err := FindAvailableSystemID()
	if err != nil {
		return nil, fmt.Errorf("failed to find available ID: %w", err)
	}

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
