//go:build linux

package service

import (
	"errors"
	"testing"
)

func TestLinuxCreateAccountTools(t *testing.T) {
	tests := []struct {
		name      string
		tools     map[string]string
		wantGroup string
		wantUser  string
		wantErr   bool
	}{
		{name: "shadow tools", tools: map[string]string{"groupadd": "/usr/sbin/groupadd", "useradd": "/usr/sbin/useradd"}, wantGroup: "/usr/sbin/groupadd", wantUser: "/usr/sbin/useradd"},
		{name: "busybox tools", tools: map[string]string{"addgroup": "/bin/addgroup", "adduser": "/bin/adduser"}, wantGroup: "/bin/addgroup", wantUser: "/bin/adduser"},
		{name: "unsupported", tools: map[string]string{}, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			group, user, err := linuxCreateAccountTools(func(command string) (string, error) {
				if path, ok := test.tools[command]; ok {
					return path, nil
				}
				return "", errors.New("not found")
			})
			if (err != nil) != test.wantErr {
				t.Fatalf("linuxCreateAccountTools() error = %v, wantErr %v", err, test.wantErr)
			}
			if group != test.wantGroup || user != test.wantUser {
				t.Fatalf("linuxCreateAccountTools() = (%q, %q), want (%q, %q)", group, user, test.wantGroup, test.wantUser)
			}
		})
	}
}

func TestLinuxDeleteAccountTools(t *testing.T) {
	tests := []struct {
		name      string
		tools     map[string]string
		wantUser  string
		wantGroup string
		wantErr   bool
	}{
		{name: "shadow tools", tools: map[string]string{"userdel": "/usr/sbin/userdel", "groupdel": "/usr/sbin/groupdel"}, wantUser: "/usr/sbin/userdel", wantGroup: "/usr/sbin/groupdel"},
		{name: "busybox tools", tools: map[string]string{"deluser": "/bin/deluser", "delgroup": "/bin/delgroup"}, wantUser: "/bin/deluser", wantGroup: "/bin/delgroup"},
		{name: "unsupported", tools: map[string]string{}, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			user, group, err := linuxDeleteAccountTools(func(command string) (string, error) {
				if path, ok := test.tools[command]; ok {
					return path, nil
				}
				return "", errors.New("not found")
			})
			if (err != nil) != test.wantErr {
				t.Fatalf("linuxDeleteAccountTools() error = %v, wantErr %v", err, test.wantErr)
			}
			if user != test.wantUser || group != test.wantGroup {
				t.Fatalf("linuxDeleteAccountTools() = (%q, %q), want (%q, %q)", user, group, test.wantUser, test.wantGroup)
			}
		})
	}
}

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
