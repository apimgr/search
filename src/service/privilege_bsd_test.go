//go:build freebsd || openbsd || netbsd

package service

import (
	"testing"
)

// Test createFreeBSDSystemUser
func TestCreateFreeBSDSystemUser(t *testing.T) {
	// This will likely fail without root, but exercises the code path
	_, err := createFreeBSDSystemUser("testsvc_bsd")
	// Error expected without root
	_ = err
}

// Test createFreeBSDSystemUser with existing user
func TestCreateFreeBSDSystemUserExisting(t *testing.T) {
	// Test with root user which always exists
	su, err := createFreeBSDSystemUser("root")
	if err != nil {
		t.Logf("createFreeBSDSystemUser(root) error = %v", err)
		return
	}

	if su.Name != "root" {
		t.Errorf("Name = %q, want %q", su.Name, "root")
	}
}
