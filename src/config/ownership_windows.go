//go:build windows

package config

import (
	"golang.org/x/sys/windows"
)

// IsPrivileged returns true if running with elevated privileges (Windows)
// Per AI.md PART 4: Check if running as Administrator
func IsPrivileged() bool {
	// Use Windows API to check if running elevated
	var sid *windows.SID
	err := windows.AllocateAndInitializeSid(
		&windows.SECURITY_NT_AUTHORITY,
		2,
		windows.SECURITY_BUILTIN_DOMAIN_RID,
		windows.DOMAIN_ALIAS_RID_ADMINS,
		0, 0, 0, 0, 0, 0,
		&sid)
	if err != nil {
		return false
	}
	defer windows.FreeSid(sid)

	member, err := windows.Token(0).IsMember(sid)
	if err != nil {
		return false
	}
	return member
}

// setOwnership is a no-op on Windows
// Windows uses ACLs instead of Unix permissions
// Per AI.md PART 7: ownership handling is Unix-specific
func setOwnership(path string) error {
	// Windows doesn't use Unix-style ownership
	// File permissions are handled via ACLs
	return nil
}

// GetCurrentUID returns 0 on Windows (no Unix UID concept)
func GetCurrentUID() int {
	return 0
}

// GetCurrentGID returns 0 on Windows (no Unix GID concept)
func GetCurrentGID() int {
	return 0
}

// SetFileOwnership is a no-op on Windows
func SetFileOwnership(path string, uid, gid int) error {
	return nil
}

// GetFileOwnership returns 0,0 on Windows
func GetFileOwnership(path string) (uid, gid int, err error) {
	return 0, 0, nil
}

// chownPath is a no-op on Windows
// Windows uses different security model (ACLs, service accounts)
func chownPath(path, userName string) error {
	return nil
}
