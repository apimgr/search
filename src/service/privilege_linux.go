//go:build linux

package service

import (
	"fmt"
	"os/exec"
	"os/user"
	"strconv"
)

// GetServiceUser returns the service user name for Linux (matches the service name).
// Per AI.md PART 24: Linux uses bare service name, no prefix.
func GetServiceUser(serviceName string) string {
	return serviceName
}

// GetServiceGroup returns the service group name for Linux (matches the service name).
// Per AI.md PART 24: Linux uses bare service name for the group.
func GetServiceGroup(serviceName string) string {
	return serviceName
}

// FindAvailableSystemID finds an available UID/GID in the safe system range on Linux.
// Per AI.md PART 24: delegates to the shared Unix scanner (range 200–899).
func FindAvailableSystemID() (int, error) {
	return findAvailableUnixSystemID()
}

// CreateSystemUser creates a Linux system user for the service.
// Per AI.md PART 24: System user creation logic.
func CreateSystemUser(name string) (*SystemUser, error) {
	return createLinuxSystemUser(name)
}

// createLinuxSystemUser creates a Linux system user via groupadd/useradd.
// Per AI.md PART 23: exact commands and flags are NON-NEGOTIABLE.
func createLinuxSystemUser(name string) (*SystemUser, error) {
	if u, err := user.Lookup(name); err == nil {
		uid, _ := strconv.Atoi(u.Uid)
		gid, _ := strconv.Atoi(u.Gid)
		return &SystemUser{
			Name:  name,
			UID:   uid,
			GID:   gid,
			Home:  u.HomeDir,
			Shell: "/sbin/nologin",
		}, nil
	}

	id, err := FindAvailableSystemID()
	if err != nil {
		return nil, fmt.Errorf("failed to find available ID: %w", err)
	}

	// Create group per AI.md PART 23: groupadd --system --gid {id} {name}
	cmd := exec.Command("groupadd", "--system", "--gid", strconv.Itoa(id), name)
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("failed to create group: %w", err)
	}

	// Create user per AI.md PART 23: home dir is config dir /etc/{org}/{name},
	// shell is /sbin/nologin (Linux), GID must be numeric.
	homeDir := "/etc/apimgr/" + name
	cmd = exec.Command("useradd", "--system",
		"--uid", strconv.Itoa(id),
		"--gid", strconv.Itoa(id),
		"--home-dir", homeDir,
		"--shell", "/sbin/nologin",
		"--comment", name+" service account",
		name)
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	return &SystemUser{
		Name:  name,
		UID:   id,
		GID:   id,
		Home:  homeDir,
		Shell: "/sbin/nologin",
	}, nil
}
