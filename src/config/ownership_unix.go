//go:build !windows

package config

import (
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"syscall"
)

// IsPrivileged returns true if running with elevated privileges (Unix)
// Per AI.md PART 4: Check EUID for root/sudo
func IsPrivileged() bool {
	return os.Geteuid() == 0
}

// setOwnership sets file/directory ownership to current user/group
// Per AI.md PART 7: Set ownership to current user/group
func setOwnership(path string) error {
	uid := os.Getuid()
	gid := os.Getgid()

	// Skip if we're not root - can only chown as root
	if uid != 0 {
		return nil
	}

	return os.Chown(path, uid, gid)
}

// GetCurrentUID returns the current user ID
func GetCurrentUID() int {
	return os.Getuid()
}

// GetCurrentGID returns the current group ID
func GetCurrentGID() int {
	return os.Getgid()
}

// SetFileOwnership sets ownership for a specific file
// Per AI.md PART 7: Tor files, key files owned by app user
func SetFileOwnership(path string, uid, gid int) error {
	return os.Chown(path, uid, gid)
}

// GetFileOwnership returns the owner UID and GID of a file
func GetFileOwnership(path string) (uid, gid int, err error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, 0, err
	}

	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, 0, nil
	}

	return int(stat.Uid), int(stat.Gid), nil
}

// chownPath changes ownership of a path to the specified user
// Per AI.md PART 8: Step 8c - Set ownership while still root
func chownPath(path, userName string) error {
	// Look up user
	u, err := user.Lookup(userName)
	if err != nil {
		return err
	}

	uid, err := strconv.Atoi(u.Uid)
	if err != nil {
		return err
	}

	gid, err := strconv.Atoi(u.Gid)
	if err != nil {
		return err
	}

	// Walk directory and chown all files
	return filepath.Walk(path, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		return os.Chown(p, uid, gid)
	})
}
