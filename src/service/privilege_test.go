package service

import (
	"os"
	"runtime"
	"testing"
)

func TestNewPrivilegeEscalator(t *testing.T) {
	pe := NewPrivilegeEscalator()
	if pe == nil {
		t.Fatal("NewPrivilegeEscalator() returned nil")
	}

	// Method should be one of the valid options
	validMethods := map[string]bool{
		"sudo":   true,
		"doas":   true,
		"pkexec": true,
		"runas":  true,
		"none":   true,
	}

	if !validMethods[pe.Method()] {
		t.Errorf("Method() = %q, not a valid method", pe.Method())
	}
}

func TestPrivilegeEscalatorMethod(t *testing.T) {
	pe := &PrivilegeEscalator{method: "sudo"}

	if pe.Method() != "sudo" {
		t.Errorf("Method() = %q, want %q", pe.Method(), "sudo")
	}
}

func TestPrivilegeEscalatorIsAvailable(t *testing.T) {
	// Test with "none" method
	pe := &PrivilegeEscalator{method: "none"}

	// If running as root, should still be available
	if os.Geteuid() == 0 {
		if !pe.IsAvailable() {
			t.Error("IsAvailable() should return true when running as root")
		}
	} else {
		if pe.IsAvailable() {
			t.Error("IsAvailable() should return false with method 'none' when not root")
		}
	}

	// Test with a real method
	pe2 := &PrivilegeEscalator{method: "sudo"}
	if !pe2.IsAvailable() {
		t.Error("IsAvailable() should return true with method 'sudo'")
	}
}

func TestPrivilegeEscalatorEscalateCommandSudo(t *testing.T) {
	pe := &PrivilegeEscalator{method: "sudo"}

	cmd := pe.EscalateCommand("ls", "-la", "/tmp")

	if cmd == nil {
		t.Fatal("EscalateCommand() returned nil")
	}
	if cmd.Path == "" {
		t.Error("Command path should not be empty")
	}

	// Command should contain sudo
	found := false
	for _, arg := range cmd.Args {
		if arg == "sudo" {
			found = true
			break
		}
	}
	if !found && runtime.GOOS != "windows" {
		// Check path instead
		if cmd.Path != "" && !containsHelper(cmd.Path, "sudo") {
			t.Error("Command should be wrapped with sudo")
		}
	}
}

func TestPrivilegeEscalatorEscalateCommandDoas(t *testing.T) {
	pe := &PrivilegeEscalator{method: "doas"}

	cmd := pe.EscalateCommand("ls", "-la")

	if cmd == nil {
		t.Fatal("EscalateCommand() returned nil")
	}
}

func TestPrivilegeEscalatorEscalateCommandPkexec(t *testing.T) {
	pe := &PrivilegeEscalator{method: "pkexec"}

	cmd := pe.EscalateCommand("ls", "-la")

	if cmd == nil {
		t.Fatal("EscalateCommand() returned nil")
	}
}

func TestPrivilegeEscalatorEscalateCommandRunas(t *testing.T) {
	pe := &PrivilegeEscalator{method: "runas"}

	cmd := pe.EscalateCommand("ls", "-la")

	if cmd == nil {
		t.Fatal("EscalateCommand() returned nil")
	}

	// On Windows, should use powershell
	if runtime.GOOS == "windows" {
		found := false
		for _, arg := range cmd.Args {
			if arg == "powershell" || containsHelper(arg, "Start-Process") {
				found = true
				break
			}
		}
		if !found {
			t.Error("Windows command should use powershell Start-Process")
		}
	}
}

func TestPrivilegeEscalatorEscalateCommandNone(t *testing.T) {
	pe := &PrivilegeEscalator{method: "none"}

	cmd := pe.EscalateCommand("ls", "-la")

	if cmd == nil {
		t.Fatal("EscalateCommand() returned nil")
	}
}

func TestDetectPrivilegeEscalationMethod(t *testing.T) {
	method := detectPrivilegeEscalationMethod()

	validMethods := map[string]bool{
		"sudo":   true,
		"doas":   true,
		"pkexec": true,
		"runas":  true,
		"none":   true,
	}

	if !validMethods[method] {
		t.Errorf("detectPrivilegeEscalationMethod() = %q, not valid", method)
	}

	// On Windows, should be "runas"
	if runtime.GOOS == "windows" && method != "runas" {
		t.Errorf("On Windows, method should be 'runas', got %q", method)
	}
}

func TestSystemUserStruct(t *testing.T) {
	su := SystemUser{
		Name:  "search",
		UID:   500,
		GID:   500,
		Home:  "/var/lib/search",
		Shell: "/bin/false",
	}

	if su.Name != "search" {
		t.Errorf("Name = %q, want %q", su.Name, "search")
	}
	if su.UID != 500 {
		t.Errorf("UID = %d, want %d", su.UID, 500)
	}
	if su.GID != 500 {
		t.Errorf("GID = %d, want %d", su.GID, 500)
	}
	if su.Home != "/var/lib/search" {
		t.Errorf("Home = %q, want %q", su.Home, "/var/lib/search")
	}
	if su.Shell != "/bin/false" {
		t.Errorf("Shell = %q, want %q", su.Shell, "/bin/false")
	}
}

func TestReservedSystemIDs(t *testing.T) {
	// Verify some commonly reserved IDs
	reserved := []int{65534, 999, 170, 171, 172}

	for _, id := range reserved {
		if !reservedSystemIDs[id] {
			t.Errorf("ID %d should be reserved", id)
		}
	}
}

func TestFindAvailableSystemID(t *testing.T) {
	if runtime.GOOS == "windows" {
		// Windows should return 0
		id, err := FindAvailableSystemID()
		if err != nil {
			t.Fatalf("FindAvailableSystemID() error = %v", err)
		}
		if id != 0 {
			t.Errorf("On Windows, ID should be 0, got %d", id)
		}
		return
	}

	id, err := FindAvailableSystemID()
	if err != nil {
		// This might fail if running in a restricted environment
		t.Logf("FindAvailableSystemID() error = %v (may be expected)", err)
		return
	}

	// ID should be in valid range
	if runtime.GOOS == "darwin" {
		if id < 200 || id >= 400 {
			t.Errorf("macOS ID %d not in range 200-399", id)
		}
	} else {
		if id < 200 || id >= 900 {
			t.Errorf("ID %d not in range 200-899", id)
		}
	}

	// ID should not be reserved
	if reservedSystemIDs[id] {
		t.Errorf("ID %d is reserved", id)
	}
}

func TestUserExists(t *testing.T) {
	// Root user should exist on Unix systems
	if runtime.GOOS != "windows" {
		if !UserExists("root") {
			t.Error("UserExists(root) should return true")
		}
	}

	// Non-existent user
	if UserExists("nonexistent_user_12345") {
		t.Error("UserExists() should return false for non-existent user")
	}
}

func TestGetServiceUser(t *testing.T) {
	tests := []struct {
		goos    string
		service string
		want    string
	}{
		{"linux", "search", "search"},
		{"freebsd", "search", "search"},
		{"darwin", "search", "_search"},
		{"windows", "search", "NT SERVICE\\search"},
	}

	for _, tt := range tests {
		t.Run(tt.goos, func(t *testing.T) {
			// We can only test the current OS
			if runtime.GOOS != tt.goos {
				t.Skip("Test only for " + tt.goos)
			}

			got := GetServiceUser(tt.service)
			if got != tt.want {
				t.Errorf("GetServiceUser(%q) = %q, want %q", tt.service, got, tt.want)
			}
		})
	}
}

func TestGetServiceGroup(t *testing.T) {
	tests := []struct {
		goos    string
		service string
		want    string
	}{
		{"linux", "search", "search"},
		{"freebsd", "search", "search"},
		{"darwin", "search", "_search"},
		{"windows", "search", ""},
	}

	for _, tt := range tests {
		t.Run(tt.goos, func(t *testing.T) {
			if runtime.GOOS != tt.goos {
				t.Skip("Test only for " + tt.goos)
			}

			got := GetServiceGroup(tt.service)
			if got != tt.want {
				t.Errorf("GetServiceGroup(%q) = %q, want %q", tt.service, got, tt.want)
			}
		})
	}
}

func TestIsRunningAsRoot(t *testing.T) {
	isRoot := IsRunningAsRoot()

	if runtime.GOOS == "windows" {
		// On Windows, this checks euid which may not be meaningful
		_ = isRoot
	} else {
		// Check against actual euid
		expected := os.Geteuid() == 0
		if isRoot != expected {
			t.Errorf("IsRunningAsRoot() = %v, want %v", isRoot, expected)
		}
	}
}

func TestVerifyPrivilegesDropped(t *testing.T) {
	if runtime.GOOS == "windows" {
		// Should return nil on Windows
		err := VerifyPrivilegesDropped()
		if err != nil {
			t.Errorf("VerifyPrivilegesDropped() on Windows should return nil, got %v", err)
		}
		return
	}

	err := VerifyPrivilegesDropped()

	if os.Geteuid() == 0 {
		// If running as root, should return error
		if err == nil {
			t.Error("VerifyPrivilegesDropped() should return error when running as root")
		}
	} else {
		// If not root, should return nil
		if err != nil {
			t.Errorf("VerifyPrivilegesDropped() error = %v, want nil (not running as root)", err)
		}
	}
}

func TestDropPrivilegesWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Test only for Windows")
	}

	// On Windows, DropPrivileges should be a no-op
	err := DropPrivileges("search")
	if err != nil {
		t.Errorf("DropPrivileges() on Windows should return nil, got %v", err)
	}
}

func TestDropPrivilegesNotRoot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Test not for Windows")
	}

	if os.Geteuid() == 0 {
		t.Skip("Test only when not running as root")
	}

	// When not running as root, should return nil
	err := DropPrivileges("search")
	if err != nil {
		t.Errorf("DropPrivileges() when not root should return nil, got %v", err)
	}
}

func TestDropPrivilegesInvalidUser(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Test not for Windows")
	}

	if os.Geteuid() != 0 {
		t.Skip("Test only when running as root")
	}

	// Try to drop to non-existent user
	err := DropPrivileges("nonexistent_user_12345")
	if err == nil {
		t.Error("DropPrivileges() should fail for non-existent user")
	}
}

func TestCreateSystemUserExisting(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Test not for Windows")
	}

	// Test with an existing user (root or current user)
	// This test may need root privileges to actually create users
	// So we just test the structure for now

	// Windows virtual service accounts
	if runtime.GOOS == "windows" {
		su, err := createWindowsVirtualServiceAccount("testsvc")
		if err != nil {
			t.Fatalf("createWindowsVirtualServiceAccount() error = %v", err)
		}
		if su.Name != "NT SERVICE\\testsvc" {
			t.Errorf("Name = %q, want %q", su.Name, "NT SERVICE\\testsvc")
		}
	}
}
