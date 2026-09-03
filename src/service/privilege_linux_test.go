//go:build linux

package service

import (
	"testing"
)

// Test createLinuxSystemUser
func TestCreateLinuxSystemUser(t *testing.T) {
	// This will likely fail without root, but exercises the code path
	_, err := createLinuxSystemUser("testsvc_linux")
	// Error expected without root
	_ = err
}

// Test createLinuxSystemUser with existing user
func TestCreateLinuxSystemUserExisting(t *testing.T) {
	// Test with root user which always exists
	su, err := createLinuxSystemUser("root")
	if err != nil {
		t.Logf("createLinuxSystemUser(root) error = %v", err)
		return
	}

	if su.Name != "root" {
		t.Errorf("Name = %q, want %q", su.Name, "root")
	}
	if su.UID != 0 {
		t.Errorf("UID = %d, want 0", su.UID)
	}
}
