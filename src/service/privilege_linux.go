//go:build linux

package service

import (
	"fmt"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"

	"github.com/apimgr/search/src/config"
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
	created, err := createLinuxSystemUser(name)
	if err != nil || !created.Created {
		return created, err
	}
	if err := markSystemUserOwned(name); err != nil {
		return nil, err
	}
	return created, nil
}

// createLinuxSystemUser creates a Linux system user using the account tools
// provided by the host, including BusyBox tools on Alpine.
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

	homeDir := "/var/lib/" + config.InternalOrg + "/" + name
	createGroup, createUser, err := linuxCreateAccountTools(exec.LookPath)
	if err != nil {
		return nil, err
	}
	var cmd *exec.Cmd
	if filepath.Base(createGroup) == "addgroup" {
		cmd = exec.Command(createGroup, "-S", "-g", strconv.Itoa(id), name)
	} else {
		cmd = exec.Command(createGroup, "--system", "--gid", strconv.Itoa(id), name)
	}
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("failed to create group: %w", err)
	}

	if filepath.Base(createUser) == "adduser" {
		cmd = exec.Command(createUser, "-S", "-D", "-H",
			"-u", strconv.Itoa(id), "-G", name, "-h", homeDir,
			"-s", "/sbin/nologin", "-g", name+" service account", name)
	} else {
		cmd = exec.Command(createUser, "--system",
			"--uid", strconv.Itoa(id),
			"--gid", strconv.Itoa(id),
			"--home-dir", homeDir,
			"--no-create-home",
			"--shell", "/sbin/nologin",
			"--comment", name+" service account",
			name)
	}
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	return &SystemUser{
		Name:    name,
		UID:     id,
		GID:     id,
		Home:    homeDir,
		Shell:   "/sbin/nologin",
		Created: true,
	}, nil
}

// DeleteSystemUser removes the Linux system user and group created by the
// server using the account deletion tools available on the host.
func DeleteSystemUser(name string) error {
	if !systemUserOwned(name) {
		return nil
	}
	deleteUser, deleteGroup, err := linuxDeleteAccountTools(exec.LookPath)
	if err != nil {
		return err
	}
	deleteArgs := []string{"--remove", name}
	if filepath.Base(deleteUser) == "deluser" {
		deleteArgs = []string{name}
	}
	if err := exec.Command(deleteUser, deleteArgs...).Run(); err != nil {
		if _, lookErr := user.Lookup(name); lookErr != nil {
			// User no longer exists — treat as deleted.
			if _, grpErr := user.LookupGroup(name); grpErr != nil {
				return nil
			}
		}
		return fmt.Errorf("failed to delete user: %w", err)
	}
	// userdel may remove the primary group itself; clean up any leftover group.
	if err := exec.Command(deleteGroup, name).Run(); err != nil {
		if _, grpErr := user.LookupGroup(name); grpErr == nil {
			return fmt.Errorf("failed to delete group: %w", err)
		}
	}
	return clearSystemUserOwnership(name)
}

type accountToolLookup func(string) (string, error)

func linuxCreateAccountTools(lookPath accountToolLookup) (string, string, error) {
	if group, groupErr := lookPath("groupadd"); groupErr == nil {
		if user, userErr := lookPath("useradd"); userErr == nil {
			return group, user, nil
		}
	}
	if group, groupErr := lookPath("addgroup"); groupErr == nil {
		if user, userErr := lookPath("adduser"); userErr == nil {
			return group, user, nil
		}
	}
	return "", "", fmt.Errorf("no supported Linux account creation tools found (need groupadd/useradd or addgroup/adduser)")
}

func linuxDeleteAccountTools(lookPath accountToolLookup) (string, string, error) {
	if user, userErr := lookPath("userdel"); userErr == nil {
		if group, groupErr := lookPath("groupdel"); groupErr == nil {
			return user, group, nil
		}
	}
	if user, userErr := lookPath("deluser"); userErr == nil {
		if group, groupErr := lookPath("delgroup"); groupErr == nil {
			return user, group, nil
		}
	}
	return "", "", fmt.Errorf("no supported Linux account deletion tools found (need userdel/groupdel or deluser/delgroup)")
}
