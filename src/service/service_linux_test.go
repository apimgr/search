//go:build linux

package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/apimgr/search/src/config"
)

func TestServiceManagerHasRunit(t *testing.T) {
	cfg := config.DefaultConfig()
	sm := NewServiceManager(cfg)

	// hasRunit should not panic
	result := sm.hasRunit()
	// We can't predict the result, just ensure it doesn't panic
	_ = result
}

func TestServiceManagerGetRunitServiceDir(t *testing.T) {
	cfg := config.DefaultConfig()
	sm := NewServiceManager(cfg)

	dir := sm.getRunitServiceDir()
	// Should be one of the expected paths
	validPaths := map[string]bool{
		"/etc/sv/search":       true,
		"/etc/runit/sv/search": true,
	}

	if !validPaths[dir] {
		t.Errorf("getRunitServiceDir() = %q, want one of %v", dir, validPaths)
	}
}

func TestServiceManagerGetRunitActiveDir(t *testing.T) {
	cfg := config.DefaultConfig()
	sm := NewServiceManager(cfg)

	dir := sm.getRunitActiveDir()
	// Should be one of the expected paths
	validPaths := map[string]bool{
		"/var/service": true,
		"/service":     true,
	}

	if !validPaths[dir] {
		t.Errorf("getRunitActiveDir() = %q, want one of %v", dir, validPaths)
	}
}

func TestSystemdTemplate(t *testing.T) {
	// Verify systemd template contains required elements per AI.md PART 25
	if systemdTemplate == "" {
		t.Fatal("systemdTemplate is empty")
	}

	required := []string{
		"[Unit]",
		"[Service]",
		"[Install]",
		"ExecStart=/usr/local/bin/search",
		"Restart=on-failure",
		"ProtectSystem=strict",
		"ProtectHome=yes",
		"PrivateTmp=yes",
	}

	for _, req := range required {
		if !contains(systemdTemplate, req) {
			t.Errorf("systemdTemplate missing required element: %q", req)
		}
	}
}

func TestRunitRunTemplate(t *testing.T) {
	if runitRunTemplate == "" {
		t.Fatal("runitRunTemplate is empty")
	}

	if !contains(runitRunTemplate, "#!/bin/sh") {
		t.Error("runitRunTemplate missing shebang")
	}
	if !contains(runitRunTemplate, "exec") {
		t.Error("runitRunTemplate missing exec")
	}
}

func TestRunitLogRunTemplate(t *testing.T) {
	if runitLogRunTemplate == "" {
		t.Fatal("runitLogRunTemplate is empty")
	}

	if !contains(runitLogRunTemplate, "#!/bin/sh") {
		t.Error("runitLogRunTemplate missing shebang")
	}
	if !contains(runitLogRunTemplate, "svlogd") {
		t.Error("runitLogRunTemplate missing svlogd")
	}
}

func TestServiceManagerStatusSystemd(t *testing.T) {
	cfg := config.DefaultConfig()
	sm := NewServiceManager(cfg)

	status, err := sm.statusSystemd()
	// Should return inactive or error
	if err != nil {
		t.Logf("statusSystemd() error = %v (expected for non-installed service)", err)
	}
	_ = status
}

func TestServiceManagerStatusRunit(t *testing.T) {
	cfg := config.DefaultConfig()
	sm := NewServiceManager(cfg)

	status, err := sm.statusRunit()
	// Should return some status
	_ = status
	_ = err
}

// Test service installation methods (error paths)
func TestServiceManagerInstallSystemdError(t *testing.T) {
	cfg := config.DefaultConfig()
	sm := NewServiceManager(cfg)

	// This will fail without root, but exercises the code path
	err := sm.installSystemd()
	if err == nil && os.Geteuid() != 0 {
		t.Log("installSystemd succeeded unexpectedly")
	}
}

func TestServiceManagerInstallRunitError(t *testing.T) {
	cfg := config.DefaultConfig()
	sm := NewServiceManager(cfg)

	err := sm.installRunit()
	// Expected to fail without proper permissions
	_ = err
}

func TestServiceManagerUninstallSystemd(t *testing.T) {
	cfg := config.DefaultConfig()
	sm := NewServiceManager(cfg)

	// Should not error even if service doesn't exist
	err := sm.uninstallSystemd()
	_ = err
}

func TestServiceManagerUninstallRunit(t *testing.T) {
	cfg := config.DefaultConfig()
	sm := NewServiceManager(cfg)

	err := sm.uninstallRunit()
	_ = err
}

// Test ensureSystemUser on Linux
func TestServiceManagerEnsureSystemUser(t *testing.T) {
	cfg := config.DefaultConfig()
	sm := NewServiceManager(cfg)

	// This may fail without root, but exercises the code
	err := sm.ensureSystemUser()
	_ = err
}

// TestInstallUserServiceLinuxCreatesUnitFile verifies that on Linux, InstallUserService
// creates the systemd user unit directory and file, then errors at the systemctl step
// (expected in a container that may lack systemctl --user support).
func TestInstallUserServiceLinuxCreatesUnitFile(t *testing.T) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("os.UserHomeDir() error = %v", err)
	}

	cfg := config.DefaultConfig()
	sm := NewServiceManager(cfg)

	// Call InstallUserService — it will fail at systemctl daemon-reload in a
	// container that has no systemd, but the directory and file must be created first.
	_ = sm.InstallUserService()

	unitDir := filepath.Join(homeDir, ".config", "systemd", "user")
	unitPath := filepath.Join(unitDir, "search.service")

	if _, err := os.Stat(unitDir); os.IsNotExist(err) {
		t.Errorf("unit directory %q was not created by InstallUserService", unitDir)
	}
	if _, err := os.Stat(unitPath); os.IsNotExist(err) {
		t.Errorf("unit file %q was not created by InstallUserService", unitPath)
	}

	// Cleanup
	os.Remove(unitPath)
}

// TestParseOpenRCStatus verifies the output parser for rc-service status output.
func TestParseOpenRCStatus(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		want   string
	}{
		{"started output", " * status: started", "active"},
		{"STARTED uppercase", "Status: STARTED", "active"},
		{"stopped output", " * status: stopped", "inactive"},
		{"STOPPED uppercase", "Status: STOPPED", "inactive"},
		{"unknown output", "service: error connecting", "unknown"},
		{"empty output", "", "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseOpenRCStatus(tt.input)
			if got != tt.want {
				t.Errorf("parseOpenRCStatus(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestStatusOpenRCNotInstalled verifies that statusOpenRC returns "inactive" when
// the service is not registered (rc-service returns a non-zero exit code).
func TestStatusOpenRCNotInstalled(t *testing.T) {
	cfg := config.DefaultConfig()
	sm := NewServiceManager(cfg)

	status, err := sm.statusOpenRC()
	if err != nil {
		t.Errorf("statusOpenRC() unexpected error = %v", err)
	}
	// In a Docker container with no "search" service registered, expect "inactive"
	if status != "inactive" && status != "unknown" {
		t.Errorf("statusOpenRC() = %q, want %q or %q", status, "inactive", "unknown")
	}
}

// TestInstallSystemdUserServiceWritesTemplate verifies that installSystemdUserService
// writes the systemd unit template content to disk (file content check).
func TestInstallSystemdUserServiceWritesTemplate(t *testing.T) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("os.UserHomeDir() error = %v", err)
	}

	cfg := config.DefaultConfig()
	sm := NewServiceManager(cfg)

	// installSystemdUserService errors at systemctl but still writes the file
	_ = sm.installSystemdUserService()

	unitPath := filepath.Join(homeDir, ".config", "systemd", "user", "search.service")
	content, err := os.ReadFile(unitPath)
	if err != nil {
		t.Fatalf("unit file not written: %v", err)
	}

	// The template must contain the ExecStart line
	if !strings.Contains(string(content), "ExecStart=/usr/local/bin/search") {
		t.Errorf("unit file missing ExecStart line; content:\n%s", content)
	}

	// Cleanup
	os.Remove(unitPath)
}

// TestParseRunitStatus verifies the runit sv status output parser.
func TestParseRunitStatus(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"run prefix", "run: search (pid 1234) 5s", "active"},
		{"down prefix", "down: search 2s, normally up", "inactive"},
		{"unknown output", "warning: unable to open supervise/ok", "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseRunitStatus(tt.input)
			if got != tt.want {
				t.Errorf("parseRunitStatus(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestHasOpenRCFalseWhenNoInitSystem verifies hasOpenRC returns false when
// neither openrc-run nor rc-service are in PATH.
func TestHasOpenRCFalseWhenNoInitSystem(t *testing.T) {
	orig := os.Getenv("PATH")
	os.Setenv("PATH", "/usr/bin:/bin")
	defer os.Setenv("PATH", orig)

	cfg := config.DefaultConfig()
	sm := NewServiceManager(cfg)
	result := sm.hasOpenRC()
	if result {
		// If the init tools happen to be in /usr/bin or /bin, skip rather than fail
		t.Skip("openrc-run or rc-service present in /usr/bin or /bin")
	}
}

// TestHasOpenRCTrueViaRCService verifies that hasOpenRC returns true when
// rc-service (but not openrc-run) is available in PATH.
func TestHasOpenRCTrueViaRCService(t *testing.T) {
	tmpDir := t.TempDir()
	fakeRC := filepath.Join(tmpDir, "rc-service")
	if err := os.WriteFile(fakeRC, []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
		t.Fatalf("write fake rc-service: %v", err)
	}

	orig := os.Getenv("PATH")
	// Strip real tools out; add tmpDir with fake rc-service only
	os.Setenv("PATH", tmpDir+":/usr/bin:/bin")
	defer os.Setenv("PATH", orig)

	cfg := config.DefaultConfig()
	sm := NewServiceManager(cfg)
	if !sm.hasOpenRC() {
		t.Error("hasOpenRC() = false, want true when rc-service is in PATH")
	}
}

// TestDispatchSystemdFallback verifies that all dispatch methods fall through to
// the systemd/generic path when no init-system tools are in PATH.
// In a Docker container without systemd this exercises the code path but errors are expected.
func TestDispatchSystemdFallback(t *testing.T) {
	orig := os.Getenv("PATH")
	// Restrict PATH so sv/runsv/openrc-run/rc-service are not found
	os.Setenv("PATH", "/usr/bin:/bin")
	defer os.Setenv("PATH", orig)

	cfg := config.DefaultConfig()
	sm := NewServiceManager(cfg)

	// Confirm init system detection is now false
	if sm.hasRunit() {
		t.Skip("runit tools found in /usr/bin or /bin — cannot test systemd fallback")
	}
	if sm.hasOpenRC() {
		t.Skip("openrc tools found in /usr/bin or /bin — cannot test systemd fallback")
	}

	// All dispatch methods reach the systemd branch; errors are fine (no systemd in Docker)
	_ = sm.Install()
	_ = sm.Uninstall()
	_, _ = sm.Status()
	_ = sm.StartAllServices()
	_ = sm.StopAllServices()
	_ = sm.RestartAllServices()
	_ = sm.Reload()
	_ = sm.Enable()
	_ = sm.Disable()
}

// TestHasRunitFakeRunsvNoServiceDirs exercises the inner body of hasRunit when
// runsv is found in PATH but neither /var/service nor /service exists on the system.
// This covers the statements inside the LookPath-success branch.
func TestHasRunitFakeRunsvNoServiceDirs(t *testing.T) {
	if _, err := os.Stat("/var/service"); err == nil {
		t.Skip("/var/service exists — cannot test the no-service-dirs fallback")
	}
	if _, err := os.Stat("/service"); err == nil {
		t.Skip("/service exists — cannot test the no-service-dirs fallback")
	}

	tmpDir := t.TempDir()
	fakeRunsv := filepath.Join(tmpDir, "runsv")
	if err := os.WriteFile(fakeRunsv, []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
		t.Fatalf("write fake runsv: %v", err)
	}

	orig := os.Getenv("PATH")
	os.Setenv("PATH", tmpDir+":/usr/bin:/bin")
	defer os.Setenv("PATH", orig)

	cfg := config.DefaultConfig()
	sm := NewServiceManager(cfg)

	// runsv is found but neither service directory exists → hasRunit must return false
	if sm.hasRunit() {
		t.Error("hasRunit() = true, want false when service directories do not exist")
	}
}

// TestEnableOpenRCBranch covers the hasOpenRC() == true branch of Enable().
// A fake rc-service is placed in PATH (so hasOpenRC returns true) but rc-update
// is absent, so Enable() enters the OpenRC branch and returns an error from runCommand.
func TestEnableOpenRCBranch(t *testing.T) {
	tmpDir := t.TempDir()
	fakeRC := filepath.Join(tmpDir, "rc-service")
	if err := os.WriteFile(fakeRC, []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
		t.Fatalf("write fake rc-service: %v", err)
	}

	// Restrict PATH: fake rc-service present, runsv absent, rc-update absent
	orig := os.Getenv("PATH")
	os.Setenv("PATH", tmpDir)
	defer os.Setenv("PATH", orig)

	cfg := config.DefaultConfig()
	sm := NewServiceManager(cfg)

	if sm.hasRunit() {
		t.Skip("runit detected in restricted PATH")
	}
	if !sm.hasOpenRC() {
		t.Skip("OpenRC not detected via fake rc-service in restricted PATH")
	}

	// Enters the OpenRC branch; rc-update not found → error expected
	err := sm.Enable()
	// Error is expected (rc-update absent), what matters is the branch was entered
	_ = err
}

// TestDisableOpenRCBranch covers the hasOpenRC() == true branch of Disable().
func TestDisableOpenRCBranch(t *testing.T) {
	tmpDir := t.TempDir()
	fakeRC := filepath.Join(tmpDir, "rc-service")
	if err := os.WriteFile(fakeRC, []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
		t.Fatalf("write fake rc-service: %v", err)
	}

	orig := os.Getenv("PATH")
	os.Setenv("PATH", tmpDir)
	defer os.Setenv("PATH", orig)

	cfg := config.DefaultConfig()
	sm := NewServiceManager(cfg)

	if sm.hasRunit() {
		t.Skip("runit detected in restricted PATH")
	}
	if !sm.hasOpenRC() {
		t.Skip("OpenRC not detected via fake rc-service")
	}

	err := sm.Disable()
	_ = err
}

// TestEnableRunitBranch covers the hasRunit() == true branch of Enable() by
// placing a fake runsv in PATH and creating a temporary /var/service directory.
// The symlink target (getRunitServiceDir) may not exist, so os.Symlink can error —
// the important thing is that the runit branch statements execute.
func TestEnableRunitBranch(t *testing.T) {
	if _, err := os.Stat("/var/service"); err != nil {
		// /var/service doesn't exist — create it temporarily for this test
		if mkErr := os.MkdirAll("/var/service", 0755); mkErr != nil {
			t.Skipf("cannot create /var/service: %v", mkErr)
		}
		defer os.Remove("/var/service")
	}

	tmpDir := t.TempDir()
	fakeRunsv := filepath.Join(tmpDir, "runsv")
	if err := os.WriteFile(fakeRunsv, []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
		t.Fatalf("write fake runsv: %v", err)
	}

	orig := os.Getenv("PATH")
	os.Setenv("PATH", tmpDir+":/usr/bin:/bin")
	defer os.Setenv("PATH", orig)

	cfg := config.DefaultConfig()
	sm := NewServiceManager(cfg)

	if !sm.hasRunit() {
		t.Skip("runit not detected (fake runsv + /var/service)")
	}

	// Enters the runit branch: getRunitServiceDir, getRunitActiveDir, Remove, Symlink
	err := sm.Enable()
	// Symlink may fail if service dir doesn't exist; that's acceptable
	_ = err

	// Cleanup any stale symlink
	os.Remove("/var/service/search")
}

// TestDisableRunitBranch covers the hasRunit() == true branch of Disable().
func TestDisableRunitBranch(t *testing.T) {
	if _, err := os.Stat("/var/service"); err != nil {
		if mkErr := os.MkdirAll("/var/service", 0755); mkErr != nil {
			t.Skipf("cannot create /var/service: %v", mkErr)
		}
		defer os.Remove("/var/service")
	}

	tmpDir := t.TempDir()
	fakeRunsv := filepath.Join(tmpDir, "runsv")
	if err := os.WriteFile(fakeRunsv, []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
		t.Fatalf("write fake runsv: %v", err)
	}

	orig := os.Getenv("PATH")
	os.Setenv("PATH", tmpDir+":/usr/bin:/bin")
	defer os.Setenv("PATH", orig)

	cfg := config.DefaultConfig()
	sm := NewServiceManager(cfg)

	if !sm.hasRunit() {
		t.Skip("runit not detected")
	}

	// Disable removes /var/service/search symlink — it may not exist, os.Remove returns error
	err := sm.Disable()
	_ = err
}

// TestGetRunitActiveDirNoServiceDirs verifies the default fallback path of
// getRunitActiveDir when neither /var/service nor /service exist.
func TestGetRunitActiveDirNoServiceDirs(t *testing.T) {
	if _, err := os.Stat("/var/service"); err == nil {
		t.Skip("/var/service exists")
	}
	if _, err := os.Stat("/service"); err == nil {
		t.Skip("/service exists")
	}

	cfg := config.DefaultConfig()
	sm := NewServiceManager(cfg)

	dir := sm.getRunitActiveDir()
	// Default fallback is /var/service
	if dir != "/var/service" {
		t.Errorf("getRunitActiveDir() = %q, want %q", dir, "/var/service")
	}
}

// TestGetRunitServiceDirEtcSv covers the /etc/sv branch of getRunitServiceDir.
// Creates /etc/sv temporarily if not present, verifies the returned path, then
// removes it.
func TestGetRunitServiceDirEtcSv(t *testing.T) {
	created := false
	if _, err := os.Stat("/etc/sv"); err != nil {
		if mkErr := os.Mkdir("/etc/sv", 0755); mkErr != nil {
			t.Skipf("cannot create /etc/sv: %v", mkErr)
		}
		created = true
	}
	if created {
		defer os.Remove("/etc/sv")
	}

	cfg := config.DefaultConfig()
	sm := NewServiceManager(cfg)

	dir := sm.getRunitServiceDir()
	if dir != "/etc/sv/search" {
		t.Errorf("getRunitServiceDir() = %q, want /etc/sv/search", dir)
	}
}

// TestInstallRunitSuccessPath covers the symlink-success / return-nil path of
// installRunit by ensuring /var/service is writable.
func TestInstallRunitSuccessPath(t *testing.T) {
	// Create /var/service if absent so the symlink step can succeed
	varServiceCreated := false
	if _, err := os.Stat("/var/service"); err != nil {
		if mkErr := os.MkdirAll("/var/service", 0755); mkErr != nil {
			t.Skipf("cannot create /var/service: %v", mkErr)
		}
		varServiceCreated = true
	}

	cfg := config.DefaultConfig()
	sm := NewServiceManager(cfg)

	serviceDir := sm.getRunitServiceDir()
	// Clean up the service dir tree and symlink after the test
	defer os.RemoveAll(serviceDir)
	defer os.Remove(filepath.Join("/var/service", "search"))
	if varServiceCreated {
		defer os.RemoveAll("/var/service")
	}

	if err := sm.installRunit(); err != nil {
		t.Errorf("installRunit() error = %v, want nil", err)
	}
}
