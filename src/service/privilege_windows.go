//go:build windows

package service

// dropPrivilegesUnix is a no-op on Windows
// Windows uses different security model (impersonation, service accounts)
func dropPrivilegesUnix(uid, gid int) error {
	return nil
}
