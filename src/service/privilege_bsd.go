//go:build freebsd || openbsd || netbsd

package service

import (
	"fmt"
	"os/exec"
	"os/user"
	"strconv"
)

// GetServiceUser returns the service user name for BSD (matches the service name).
// Per AI.md PART 24: BSD uses bare service name, no prefix.
func GetServiceUser(serviceName string) string {
	return serviceName
}

// GetServiceGroup returns the service group name for BSD (matches the service name).
// Per AI.md PART 24: BSD uses bare service name for the group.
func GetServiceGroup(serviceName string) string {
	return serviceName
}

// FindAvailableSystemID finds an available UID/GID in the safe system range on BSD.
// Per AI.md PART 24: delegates to the shared Unix scanner (range 200–899).
func FindAvailableSystemID() (int, error) {
	return findAvailableUnixSystemID()
}

// CreateSystemUser creates a BSD system user for the service.
// Per AI.md PART 24: System user creation logic.
func CreateSystemUser(name string) (*SystemUser, error) {
	return createFreeBSDSystemUser(name)
}

// createFreeBSDSystemUser creates a FreeBSD/BSD system user via pw(8).
func createFreeBSDSystemUser(name string) (*SystemUser, error) {
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

	id, err := FindAvailableSystemID()
	if err != nil {
		return nil, fmt.Errorf("failed to find available ID: %w", err)
	}

	// Create user with pw command using org-scoped home path per AI.md PART 23
	cmd := exec.Command("pw", "useradd", name,
		"-u", strconv.Itoa(id),
		"-g", strconv.Itoa(id),
		"-d", "/var/lib/apimgr/"+name,
		"-s", "/usr/sbin/nologin",
		"-c", name+" service account")
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	return &SystemUser{
		Name:  name,
		UID:   id,
		GID:   id,
		Home:  "/var/lib/apimgr/" + name,
		Shell: "/usr/sbin/nologin",
	}, nil
}

// DeleteSystemUser removes the BSD system user and group created by the
// server. Per AI.md PART 23 uninstall: pw userdel/pw groupdel are the inverse
// of the pw useradd creation commands.
func DeleteSystemUser(name string) error {
	if err := exec.Command("pw", "userdel", name, "-r").Run(); err != nil {
		if _, lookErr := user.Lookup(name); lookErr != nil {
			if _, grpErr := user.LookupGroup(name); grpErr != nil {
				return nil
			}
		}
		return fmt.Errorf("failed to delete user: %w", err)
	}
	if err := exec.Command("pw", "groupdel", name).Run(); err != nil {
		if _, grpErr := user.LookupGroup(name); grpErr == nil {
			return fmt.Errorf("failed to delete group: %w", err)
		}
	}
	return nil
}
