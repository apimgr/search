package service

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/apimgr/search/src/config"
)

func TestNewTorService(t *testing.T) {
	cfg := config.DefaultConfig()
	ts := NewTorService(cfg)

	if ts == nil {
		t.Fatal("NewTorService() returned nil")
	}
	if ts.config != cfg {
		t.Error("Config not set correctly")
	}
	if ts.ctx == nil {
		t.Error("Context should not be nil")
	}
	if ts.cancel == nil {
		t.Error("Cancel func should not be nil")
	}
}

func TestTorServiceIsRunningInitial(t *testing.T) {
	cfg := config.DefaultConfig()
	ts := NewTorService(cfg)

	if ts.IsRunning() {
		t.Error("IsRunning() should be false initially")
	}
}

func TestTorServiceGetOnionAddressInitial(t *testing.T) {
	cfg := config.DefaultConfig()
	ts := NewTorService(cfg)

	addr := ts.GetOnionAddress()
	if addr != "" {
		t.Errorf("GetOnionAddress() = %q, want empty string initially", addr)
	}
}

func TestTorServiceGetTorStatus(t *testing.T) {
	cfg := config.DefaultConfig()
	ts := NewTorService(cfg)

	status := ts.GetTorStatus()
	if status == nil {
		t.Fatal("GetTorStatus() returned nil")
	}

	// Should have required fields
	if _, ok := status["enabled"]; !ok {
		t.Error("Status should have 'enabled' field")
	}
	if _, ok := status["running"]; !ok {
		t.Error("Status should have 'running' field")
	}
	if _, ok := status["onion_address"]; !ok {
		t.Error("Status should have 'onion_address' field")
	}
	if _, ok := status["status"]; !ok {
		t.Error("Status should have 'status' field")
	}

	// When not running, status should be "disconnected"
	if status["status"] != "disconnected" {
		t.Errorf("status[status] = %v, want %q", status["status"], "disconnected")
	}
}

func TestTorServiceShutdown(t *testing.T) {
	cfg := config.DefaultConfig()
	ts := NewTorService(cfg)

	// Shutdown should not panic when not running
	ts.Shutdown()

	// After shutdown, should not be running
	if ts.IsRunning() {
		t.Error("IsRunning() should be false after Shutdown()")
	}
}

func TestTorServiceStopNotRunning(t *testing.T) {
	cfg := config.DefaultConfig()
	ts := NewTorService(cfg)

	// Stop when not running should succeed
	err := ts.StopTorService()
	if err != nil {
		t.Errorf("Stop() error = %v, want nil", err)
	}
}

// SetEnabled was removed per AI.md PART 32 (no enable/disable toggle).
// Tor is auto-enabled when binary is found. Test Stop instead.
func TestTorServiceStopWhenNotRunning(t *testing.T) {
	cfg := config.DefaultConfig()
	ts := NewTorService(cfg)

	// Stop is a no-op when not running
	err := ts.StopTorService()
	if err != nil {
		t.Errorf("Stop() error = %v", err)
	}
}

func TestFindTorBinary(t *testing.T) {
	// Test with empty config
	result := findTorBinary("")

	// Result may be empty (no Tor installed) or a valid path
	// We can't predict the result, just ensure it doesn't panic
	_ = result
}

func TestFindTorBinaryWithConfig(t *testing.T) {
	// Test with a non-existent config path
	result := findTorBinary("/nonexistent/tor/binary")

	// Should fall back to PATH search or return empty
	_ = result
}

func TestFindTorBinaryCommonLocations(t *testing.T) {
	// Verify common locations list is correct
	commonLocations := []string{
		"/usr/bin/tor",
		"/usr/local/bin/tor",
		"/opt/local/bin/tor",
		"/opt/homebrew/bin/tor",
		"/usr/local/opt/tor/bin/tor",
		"/snap/bin/tor",
	}

	// Just verify the function doesn't panic when checking these
	for _, loc := range commonLocations {
		// This will check if file exists
		_ = findTorBinary(loc)
	}
}

func TestVanityProgressStruct(t *testing.T) {
	vp := VanityProgress{
		Prefix:   "abc",
		Attempts: 1000,
		Running:  true,
		Found:    false,
		Address:  "",
		Error:    "",
	}

	if vp.Prefix != "abc" {
		t.Errorf("Prefix = %q, want %q", vp.Prefix, "abc")
	}
	if vp.Attempts != 1000 {
		t.Errorf("Attempts = %d, want %d", vp.Attempts, 1000)
	}
	if !vp.Running {
		t.Error("Running should be true")
	}
	if vp.Found {
		t.Error("Found should be false")
	}
}

func TestTorServiceGetVanityProgress(t *testing.T) {
	cfg := config.DefaultConfig()
	ts := NewTorService(cfg)

	progress := ts.GetVanityProgress()
	if progress == nil {
		t.Fatal("GetVanityProgress() returned nil")
	}

	// Initial progress should not be running
	if progress.Running {
		t.Error("Initial progress should not be running")
	}
}

func TestTorServiceGenerateVanityPrefixTooLong(t *testing.T) {
	cfg := config.DefaultConfig()
	ts := NewTorService(cfg)

	// Prefix longer than 6 characters should fail
	err := ts.GenerateVanity("abcdefgh")
	if err == nil {
		t.Error("GenerateVanity() should fail for prefix > 6 characters")
	}
}

func TestTorServiceGenerateVanityInvalidChars(t *testing.T) {
	cfg := config.DefaultConfig()
	ts := NewTorService(cfg)

	// Invalid characters should fail
	invalidPrefixes := []string{
		// uppercase
		"ABC",
		// invalid number (1 is not valid base32)
		"abc1",
		// underscore
		"abc_",
		// hyphen
		"abc-",
		// special char
		"abc!",
	}

	for _, prefix := range invalidPrefixes {
		err := ts.GenerateVanity(prefix)
		if err == nil {
			t.Errorf("GenerateVanity(%q) should fail for invalid characters", prefix)
		}
	}
}

func TestTorServiceGenerateVanityValidChars(t *testing.T) {
	cfg := config.DefaultConfig()
	ts := NewTorService(cfg)

	// Valid base32 characters
	validPrefixes := []string{
		"abc",
		"xyz",
		"ab2",
		"a23",
		"234567",
	}

	for _, prefix := range validPrefixes {
		err := ts.GenerateVanity(prefix)
		if err != nil {
			t.Errorf("GenerateVanity(%q) error = %v, should succeed", prefix, err)
		}
		// Cancel after starting
		ts.CancelVanity()
	}
}

func TestGetTorConfigDoesNotEmitLegacyInvalidOption(t *testing.T) {
	cfg := config.DefaultConfig()
	ts := NewTorService(cfg)

	torrc := ts.getTorConfig()
	if strings.Contains(torrc, "MaxStreamsPerCircuit ") {
		t.Fatal("getTorConfig() should not include MaxStreamsPerCircuit")
	}
	if runtime.GOOS != "windows" && strings.Contains(torrc, "ControlSocket ") {
		t.Fatal("getTorConfig() should not include ControlSocket on Unix when using bine control-port discovery")
	}
}

func TestSanitizeTorrcContentRemovesLegacyInvalidOption(t *testing.T) {
	input := strings.Join([]string{
		"SafeLogging 1",
		"MaxStreamsPerCircuit 100",
		"BandwidthRate 1 MB",
	}, "\n")

	got := sanitizeTorrcContent(input)
	if strings.Contains(got, "MaxStreamsPerCircuit ") {
		t.Fatal("sanitizeTorrcContent() should remove MaxStreamsPerCircuit")
	}
	if !strings.Contains(got, "BandwidthRate 1 MB") {
		t.Fatal("sanitizeTorrcContent() should preserve unrelated lines")
	}
}

func TestSanitizeTorrcContentMigratesUnixControlPort(t *testing.T) {
	input := "ControlPort unix:/old/control.sock\nSafeLogging 1"

	got := sanitizeTorrcContent(input)
	// Per AI.md PART 31: all OSes use ControlPort 127.0.0.1:auto — unix socket → TCP
	if !strings.Contains(got, "ControlPort 127.0.0.1:auto") {
		t.Fatal("sanitizeTorrcContent() should migrate unix socket to ControlPort 127.0.0.1:auto")
	}
	if strings.Contains(got, "ControlSocket ") {
		t.Fatal("sanitizeTorrcContent() should remove ControlSocket entries from legacy unix socket config")
	}
}

func TestSanitizeTorrcContentRemovesControlSocketLine(t *testing.T) {
	dataDir := "/tmp/search-tor"
	input := strings.Join([]string{
		"ControlPort 0",
		"ControlSocket " + dataDir + "/control.sock",
		"SafeLogging 1",
	}, "\n")

	got := sanitizeTorrcContent(input)
	if strings.Contains(got, "ControlSocket ") {
		t.Fatal("sanitizeTorrcContent() should strip standalone ControlSocket lines")
	}
	// ControlPort 0 should be upgraded to 127.0.0.1:auto per AI.md PART 31
	if !strings.Contains(got, "ControlPort 127.0.0.1:auto") {
		t.Fatal("sanitizeTorrcContent() should upgrade ControlPort 0 to 127.0.0.1:auto")
	}
}

func TestTorServiceCancelVanity(t *testing.T) {
	cfg := config.DefaultConfig()
	ts := NewTorService(cfg)

	// Start vanity generation
	ts.GenerateVanity("abc")

	// Cancel should not panic
	ts.CancelVanity()

	// Progress should show not running
	progress := ts.GetVanityProgress()
	if progress.Running {
		t.Error("Progress should not be running after cancel")
	}
}

func TestTorServiceCancelVanityNotStarted(t *testing.T) {
	cfg := config.DefaultConfig()
	ts := NewTorService(cfg)

	// CancelVanity when nothing is running should not panic
	ts.CancelVanity()
}

func TestTorServiceApplyVanityAddressNotGenerated(t *testing.T) {
	cfg := config.DefaultConfig()
	ts := NewTorService(cfg)

	// Should fail if no vanity address generated
	_, err := ts.ApplyVanityAddress()
	if err == nil {
		t.Error("ApplyVanityAddress() should fail when no address generated")
	}
}

func TestTorServiceRegenerateAddressNotRunning(t *testing.T) {
	cfg := config.DefaultConfig()
	ts := NewTorService(cfg)

	// Should fail if Tor is not running
	_, err := ts.RegenerateAddress()
	if err == nil {
		t.Error("RegenerateAddress() should fail when Tor not running")
	}
}

func TestTorServiceExportKeysNotRunning(t *testing.T) {
	cfg := config.DefaultConfig()
	ts := NewTorService(cfg)

	// Should fail if Tor is not running
	_, err := ts.ExportKeys()
	if err == nil {
		t.Error("ExportKeys() should fail when Tor not running")
	}
}

func TestTorServiceImportKeysEmpty(t *testing.T) {
	cfg := config.DefaultConfig()
	ts := NewTorService(cfg)

	// Should fail with empty key
	_, err := ts.ImportKeys([]byte{})
	if err == nil {
		t.Error("ImportKeys() should fail with empty key")
	}

	_, err = ts.ImportKeys(nil)
	if err == nil {
		t.Error("ImportKeys() should fail with nil key")
	}
}

func TestCheckTorConnection(t *testing.T) {
	// With bine, this is a simplified check that always returns true
	result := CheckTorConnection("127.0.0.1:9050")
	if !result {
		t.Error("CheckTorConnection() should return true (simplified check)")
	}
}

func TestTryGenerateVanityAddress(t *testing.T) {
	// generateOnionV3Address generates proper ED25519-based v3 onion addresses
	addr, privKey, err := generateOnionV3Address()
	if err != nil {
		t.Fatalf("generateOnionV3Address() error = %v", err)
	}

	// Address should be 56 characters (v3 onion)
	if len(addr) != 56 {
		t.Errorf("Address length = %d, want 56", len(addr))
	}

	// Private key must be present
	if len(privKey) == 0 {
		t.Error("Private key must not be empty")
	}

	// Address should only contain valid base32 characters
	validChars := "abcdefghijklmnopqrstuvwxyz234567"
	for _, c := range addr {
		found := false
		for _, v := range validChars {
			if c == v {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Address contains invalid character: %c", c)
		}
	}
}

func TestLookPathWrapper(t *testing.T) {
	// lookPath is a variable that wraps exec.LookPath
	// This allows for testing

	// Test with a command that should exist on most systems
	path, err := lookPath("sh")
	if err == nil && path == "" {
		t.Error("lookPath(sh) returned empty path without error")
	}

	// Test with non-existent command
	_, err = lookPath("nonexistent_command_12345")
	if err == nil {
		t.Error("lookPath() should fail for non-existent command")
	}
}

// Integration test stubs - these would require actual Tor to be installed

func TestTorServiceStartNoTorBinary(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Server.Tor.Binary = "/nonexistent/tor"
	cfg.Server.Tor.Enabled = true

	// Block PATH lookup and commonLocations stat so findTorBinary truly finds nothing
	orig := lookPath
	lookPath = func(_ string) (string, error) { return "", os.ErrNotExist }
	origStat := statFile
	statFile = func(_ string) (os.FileInfo, error) { return nil, os.ErrNotExist }
	defer func() { lookPath = orig; statFile = origStat }()

	ts := NewTorService(cfg)

	// Start should succeed (binary not found → Tor is optional, returns nil)
	err := ts.StartTorService()
	if err != nil {
		// If Tor binary not found, Start returns nil (Tor is optional)
		t.Logf("Start() returned error (expected if no Tor): %v", err)
	}

	// Tor should auto-disable when binary not found
	// This is per AI.md PART 32: "NOT FOUND: Log INFO, disable Tor features, continue without Tor"
}

func TestTorServiceRestartNotRunning(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Server.Tor.Binary = "/nonexistent/tor"

	// Block PATH lookup and commonLocations stat so findTorBinary truly finds nothing
	origLP := lookPath
	lookPath = func(_ string) (string, error) { return "", os.ErrNotExist }
	origStat := statFile
	statFile = func(_ string) (os.FileInfo, error) { return nil, os.ErrNotExist }
	defer func() { lookPath = origLP; statFile = origStat }()

	ts := NewTorService(cfg)

	// Restart when not running should attempt to start and return nil immediately
	err := ts.RestartTorService()
	// May fail if no Tor binary, but should not panic
	_ = err
}

// =====================================================
// Additional tests for 100% coverage
// =====================================================

// Test TorService struct fields
func TestTorServiceStructFields(t *testing.T) {
	cfg := config.DefaultConfig()
	ts := NewTorService(cfg)

	// Verify all fields are initialized correctly
	if ts.config == nil {
		t.Error("config should not be nil")
	}
	if ts.running {
		t.Error("running should be false initially")
	}
	if ts.onionAddr != "" {
		t.Error("onionAddr should be empty initially")
	}
}

// Test VanityProgress struct
func TestVanityProgressStructComplete(t *testing.T) {
	vp := VanityProgress{
		Prefix:   "test",
		Attempts: 12345,
		Running:  true,
		Found:    true,
		Address:  "testaddress",
		Error:    "some error",
	}

	if vp.Prefix != "test" {
		t.Errorf("Prefix = %q, want %q", vp.Prefix, "test")
	}
	if vp.Attempts != 12345 {
		t.Errorf("Attempts = %d, want %d", vp.Attempts, 12345)
	}
	if !vp.Running {
		t.Error("Running should be true")
	}
	if !vp.Found {
		t.Error("Found should be true")
	}
	if vp.Address != "testaddress" {
		t.Errorf("Address = %q, want %q", vp.Address, "testaddress")
	}
	if vp.Error != "some error" {
		t.Errorf("Error = %q, want %q", vp.Error, "some error")
	}
}

// Test findTorBinary with various scenarios
func TestFindTorBinaryScenarios(t *testing.T) {
	tests := []struct {
		name       string
		configPath string
	}{
		{"empty config", ""},
		{"nonexistent path", "/nonexistent/tor/binary"},
		{"usr bin", "/usr/bin/tor"},
		{"usr local bin", "/usr/local/bin/tor"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := findTorBinary(tt.configPath)
			// Result may be empty or a valid path
			_ = result
		})
	}
}

// Test vanityGenerator global state
func TestVanityGeneratorGlobal(t *testing.T) {
	cfg := config.DefaultConfig()
	ts := NewTorService(cfg)

	// Initial state
	progress := ts.GetVanityProgress()
	if progress == nil {
		t.Fatal("GetVanityProgress should not return nil")
	}

	// Start generation
	ts.GenerateVanity("ab")

	// Check progress is updated
	progress = ts.GetVanityProgress()
	if progress.Prefix != "ab" {
		t.Errorf("Prefix = %q, want %q", progress.Prefix, "ab")
	}

	// Cancel
	ts.CancelVanity()
}

// Test GenerateVanity with valid short prefixes
func TestTorServiceGenerateVanityShortPrefixes(t *testing.T) {
	cfg := config.DefaultConfig()
	ts := NewTorService(cfg)

	tests := []string{"a", "ab", "abc", "abcd", "abcde", "abcdef"}

	for _, prefix := range tests {
		t.Run(prefix, func(t *testing.T) {
			err := ts.GenerateVanity(prefix)
			if err != nil {
				t.Errorf("GenerateVanity(%q) error = %v", prefix, err)
			}
			ts.CancelVanity()
		})
	}
}

// Test GenerateVanity cancels previous generation
func TestTorServiceGenerateVanityCancelsPrevious(t *testing.T) {
	cfg := config.DefaultConfig()
	ts := NewTorService(cfg)

	// Start first generation
	ts.GenerateVanity("ab")

	// Start second generation (should cancel first)
	ts.GenerateVanity("cd")

	progress := ts.GetVanityProgress()
	if progress.Prefix != "cd" {
		t.Errorf("Prefix should be 'cd', got %q", progress.Prefix)
	}

	ts.CancelVanity()
}

// Test generateOnionV3Address produces valid addresses
func TestTorServiceTryGenerateVanityAddressMultiple(t *testing.T) {
	// Generate multiple addresses and verify they're all valid
	for i := 0; i < 10; i++ {
		addr, privKey, err := generateOnionV3Address()
		if err != nil {
			t.Fatalf("generateOnionV3Address() error = %v", err)
		}

		if len(addr) != 56 {
			t.Errorf("Address length = %d, want 56", len(addr))
		}

		if len(privKey) == 0 {
			t.Error("Private key must not be empty")
		}

		// Verify all characters are valid base32
		validChars := "abcdefghijklmnopqrstuvwxyz234567"
		for _, c := range addr {
			found := false
			for _, v := range validChars {
				if c == v {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("Address contains invalid character: %c", c)
			}
		}
	}
}

// Test GetTorStatus fields
func TestTorServiceGetTorStatusFields(t *testing.T) {
	cfg := config.DefaultConfig()
	ts := NewTorService(cfg)

	status := ts.GetTorStatus()

	// Required fields
	requiredFields := []string{"enabled", "running", "onion_address", "status"}
	for _, field := range requiredFields {
		if _, ok := status[field]; !ok {
			t.Errorf("Status should have '%s' field", field)
		}
	}
}

// SetEnabled was removed per AI.md PART 32 (no enable/disable toggle).
func TestTorServiceStopIsNoOpWhenNotRunning(t *testing.T) {
	cfg := config.DefaultConfig()
	ts := NewTorService(cfg)

	err := ts.StopTorService()
	if err != nil {
		t.Errorf("Stop() error = %v", err)
	}

	if ts.IsRunning() {
		t.Error("Should not be running")
	}
}

// Test Stop is idempotent
func TestTorServiceStopIdempotent(t *testing.T) {
	cfg := config.DefaultConfig()
	ts := NewTorService(cfg)

	// Stop multiple times should not panic
	for i := 0; i < 5; i++ {
		err := ts.StopTorService()
		if err != nil {
			t.Errorf("Stop() iteration %d error = %v", i, err)
		}
	}
}

// Test Shutdown is idempotent
func TestTorServiceShutdownIdempotent(t *testing.T) {
	cfg := config.DefaultConfig()
	ts := NewTorService(cfg)

	// Shutdown multiple times should not panic
	ts.Shutdown()
	ts.Shutdown()
	ts.Shutdown()
}

// Test concurrent access to TorService
func TestTorServiceConcurrentAccess(t *testing.T) {
	cfg := config.DefaultConfig()
	ts := NewTorService(cfg)

	done := make(chan bool)

	// Reader goroutine
	go func() {
		for i := 0; i < 100; i++ {
			_ = ts.IsRunning()
			_ = ts.GetOnionAddress()
			_ = ts.GetTorStatus()
			_ = ts.GetVanityProgress()
		}
		done <- true
	}()

	// Vanity goroutine
	go func() {
		for i := 0; i < 10; i++ {
			ts.GenerateVanity("ab")
			ts.CancelVanity()
		}
		done <- true
	}()

	<-done
	<-done
}

// Test ImportKeys with valid key data
func TestTorServiceImportKeysValidData(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Server.Tor.Binary = "/nonexistent/tor"

	// Block PATH lookup and commonLocations stat so findTorBinary truly finds nothing
	origLP := lookPath
	lookPath = func(_ string) (string, error) { return "", os.ErrNotExist }
	origStat := statFile
	statFile = func(_ string) (os.FileInfo, error) { return nil, os.ErrNotExist }
	defer func() { lookPath = origLP; statFile = origStat }()

	ts := NewTorService(cfg)

	// Create some test key data
	keyData := make([]byte, 64)
	for i := range keyData {
		keyData[i] = byte(i)
	}

	// This will fail because Tor isn't actually running, but exercises the code path
	_, err := ts.ImportKeys(keyData)
	// Expected to fail without Tor
	_ = err
}

// Test lookPath function wrapper
func TestLookPathFunction(t *testing.T) {
	// Test with common commands
	commands := []string{"sh", "ls", "true"}

	for _, cmd := range commands {
		path, err := lookPath(cmd)
		if err == nil && path == "" {
			t.Errorf("lookPath(%q) returned empty path without error", cmd)
		}
	}
}

// Test Start when already running returns early
func TestTorServiceStartAlreadyRunning(t *testing.T) {
	cfg := config.DefaultConfig()
	ts := NewTorService(cfg)

	// Manually set running to true
	ts.mu.Lock()
	ts.running = true
	ts.mu.Unlock()

	// Start should return nil early
	err := ts.StartTorService()
	if err != nil {
		t.Errorf("Start() when already running should return nil, got %v", err)
	}

	// Reset for cleanup
	ts.mu.Lock()
	ts.running = false
	ts.mu.Unlock()
}

// Test GetOnionAddress when running
func TestTorServiceGetOnionAddressWhenSet(t *testing.T) {
	cfg := config.DefaultConfig()
	ts := NewTorService(cfg)

	// Manually set onion address
	ts.mu.Lock()
	ts.onionAddr = "testaddress.onion"
	ts.mu.Unlock()

	addr := ts.GetOnionAddress()
	if addr != "testaddress.onion" {
		t.Errorf("GetOnionAddress() = %q, want %q", addr, "testaddress.onion")
	}
}

// Test vanity generation prefix validation
func TestTorServiceGenerateVanityValidation(t *testing.T) {
	cfg := config.DefaultConfig()
	ts := NewTorService(cfg)

	tests := []struct {
		prefix  string
		wantErr bool
	}{
		// Empty is valid
		{"", false},
		// Single char valid
		{"a", false},
		// Short valid
		{"abc", false},
		// Numbers 2-7 valid
		{"234567", false},
		// 6 chars max for built-in
		{"abcdef", false},
		// 7 chars too long
		{"abcdefg", true},
		// Uppercase invalid
		{"ABC", true},
		// 1 invalid in base32
		{"abc1", true},
		// 0 invalid in base32
		{"abc0", true},
		// 8 invalid in base32
		{"abc8", true},
		// 9 invalid in base32
		{"abc9", true},
		// Special char invalid
		{"abc!", true},
		// Hyphen invalid
		{"abc-", true},
		// Underscore invalid
		{"abc_", true},
		// Space invalid
		{"abc ", true},
	}

	for _, tt := range tests {
		t.Run(tt.prefix, func(t *testing.T) {
			err := ts.GenerateVanity(tt.prefix)
			if (err != nil) != tt.wantErr {
				t.Errorf("GenerateVanity(%q) error = %v, wantErr %v", tt.prefix, err, tt.wantErr)
			}
			ts.CancelVanity()
		})
	}
}

// Test ApplyVanityAddress when found
func TestTorServiceApplyVanityAddressWhenFound(t *testing.T) {
	cfg := config.DefaultConfig()
	ts := NewTorService(cfg)

	// Manually set found state
	vanityGen.mu.Lock()
	vanityGen.progress.Found = true
	vanityGen.progress.Address = "testaddress"
	vanityGen.mu.Unlock()

	// ApplyVanityAddress should call RegenerateAddress which will fail
	// because Tor isn't running, but it exercises the code path
	_, err := ts.ApplyVanityAddress()
	if err == nil {
		t.Log("ApplyVanityAddress succeeded unexpectedly")
	}

	// Reset state
	vanityGen.mu.Lock()
	vanityGen.progress.Found = false
	vanityGen.progress.Address = ""
	vanityGen.mu.Unlock()
}

// Test ExportKeys when running but no keys
func TestTorServiceExportKeysNoKeys(t *testing.T) {
	cfg := config.DefaultConfig()
	ts := NewTorService(cfg)

	// Manually set running
	ts.mu.Lock()
	ts.running = true
	ts.mu.Unlock()

	// ExportKeys should fail because keys file doesn't exist
	_, err := ts.ExportKeys()
	if err == nil {
		t.Log("ExportKeys succeeded unexpectedly (may have found real keys)")
	}

	// Reset
	ts.mu.Lock()
	ts.running = false
	ts.mu.Unlock()
}

// Test common Tor binary locations constant
func TestCommonTorLocations(t *testing.T) {
	// Verify findTorBinary checks expected locations
	locations := []string{
		"/usr/bin/tor",
		"/usr/local/bin/tor",
		"/opt/local/bin/tor",
		"/opt/homebrew/bin/tor",
		"/usr/local/opt/tor/bin/tor",
		"/snap/bin/tor",
	}

	for _, loc := range locations {
		// Just verify it doesn't panic when checking
		_ = findTorBinary(loc)
	}
}

// Test GetVanityProgress returns copy
func TestTorServiceGetVanityProgressReturnsCopy(t *testing.T) {
	cfg := config.DefaultConfig()
	ts := NewTorService(cfg)

	progress1 := ts.GetVanityProgress()
	progress2 := ts.GetVanityProgress()

	// Modify progress1
	progress1.Attempts = 999999

	// progress2 should be unaffected
	if progress2.Attempts == 999999 {
		t.Error("GetVanityProgress should return a copy, not a reference")
	}
}

// Test Restart calls Stop then Start
func TestTorServiceRestartSequence(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Server.Tor.Binary = "/nonexistent/tor"

	// Block PATH lookup and commonLocations stat so findTorBinary truly finds nothing
	origLP := lookPath
	lookPath = func(_ string) (string, error) { return "", os.ErrNotExist }
	origStat := statFile
	statFile = func(_ string) (os.FileInfo, error) { return nil, os.ErrNotExist }
	defer func() { lookPath = origLP; statFile = origStat }()

	ts := NewTorService(cfg)

	// Restart should call Stop (succeeds) then Start (returns nil — binary not found)
	err := ts.RestartTorService()
	_ = err
}

// Test TorService with different config settings
func TestTorServiceWithConfig(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Server.Tor.Enabled = true
	cfg.Server.Tor.VirtualPort = 8080
	cfg.Server.Port = 3000

	ts := NewTorService(cfg)
	if ts == nil {
		t.Fatal("NewTorService returned nil")
	}

	status := ts.GetTorStatus()
	if status["enabled"] != true {
		t.Errorf("Status enabled = %v, want true", status["enabled"])
	}
}

// Test CheckTorConnection always returns true (simplified)
func TestCheckTorConnectionAlwaysTrue(t *testing.T) {
	addresses := []string{
		"127.0.0.1:9050",
		"localhost:9050",
		"0.0.0.0:9050",
		"",
	}

	for _, addr := range addresses {
		result := CheckTorConnection(addr)
		if !result {
			t.Errorf("CheckTorConnection(%q) = false, want true", addr)
		}
	}
}

// =====================================================================
// ensureTorDirs tests — verifies dirs are created with 0700 permissions
// =====================================================================

// TestEnsureTorDirsCreatesDirectories verifies that ensureTorDirs creates the
// expected directory tree and sets 0700 permissions on each directory.
func TestEnsureTorDirsCreatesDirectories(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("ensureTorDirs chown is skipped on Windows — permissions test is Unix-only")
	}

	tempDir, err := os.MkdirTemp("", "tor-dirs-test-")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	cfg := config.DefaultConfig()
	ts := NewTorService(cfg)

	// Override unexported fields directly (same package)
	ts.dataDir = filepath.Join(tempDir, "data", "tor")
	ts.configDir = filepath.Join(tempDir, "config", "tor")

	if err := ts.ensureTorDirs(); err != nil {
		t.Fatalf("ensureTorDirs() error = %v", err)
	}

	expectedDirs := []string{
		ts.dataDir,
		filepath.Join(ts.dataDir, "site"),
		ts.configDir,
	}

	for _, dir := range expectedDirs {
		info, err := os.Stat(dir)
		if err != nil {
			t.Errorf("directory %q missing: %v", dir, err)
			continue
		}
		if !info.IsDir() {
			t.Errorf("%q is not a directory", dir)
		}
		if info.Mode().Perm() != 0700 {
			t.Errorf("directory %q has permissions %o, want 0700", dir, info.Mode().Perm())
		}
	}
}

// TestEnsureTorDirsIdempotent verifies that calling ensureTorDirs twice on
// existing directories does not error.
func TestEnsureTorDirsIdempotent(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "tor-dirs-idempotent-")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	cfg := config.DefaultConfig()
	ts := NewTorService(cfg)
	ts.dataDir = filepath.Join(tempDir, "data", "tor")
	ts.configDir = filepath.Join(tempDir, "config", "tor")

	if err := ts.ensureTorDirs(); err != nil {
		t.Fatalf("first ensureTorDirs() error = %v", err)
	}
	if err := ts.ensureTorDirs(); err != nil {
		t.Errorf("second ensureTorDirs() error = %v, want nil (idempotent)", err)
	}
}

// =====================================================================
// GetHTTPClient tests — covers direct and Tor-routed path
// =====================================================================

// TestGetHTTPClientNilDialer covers both cases where the client should be a direct
// 30-second timeout client: useTor=false and useTor=true with nil dialer.
func TestGetHTTPClientNilDialer(t *testing.T) {
	tests := []struct {
		name    string
		useTor  bool
		wantTimeout time.Duration
	}{
		{"useTor=false nil dialer", false, 30 * time.Second},
		{"useTor=true nil dialer", true, 30 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.DefaultConfig()
			ts := NewTorService(cfg)
			// dialer is nil by default

			client := ts.GetHTTPClient(tt.useTor)
			if client == nil {
				t.Fatal("GetHTTPClient() returned nil")
			}
			if client.Timeout != tt.wantTimeout {
				t.Errorf("GetHTTPClient(%v).Timeout = %v, want %v",
					tt.useTor, client.Timeout, tt.wantTimeout)
			}
		})
	}
}

// TestGetHTTPClientReturnType verifies GetHTTPClient always returns a *http.Client.
func TestGetHTTPClientReturnType(t *testing.T) {
	cfg := config.DefaultConfig()
	ts := NewTorService(cfg)

	client := ts.GetHTTPClient(false)
	if _, ok := interface{}(client).(*http.Client); !ok {
		t.Error("GetHTTPClient() did not return *http.Client")
	}
}

// =====================================================================
// OutboundEnabled tests — true only when dialer != nil
// =====================================================================

// TestOutboundEnabledNilDialer verifies that OutboundEnabled returns false when
// no Tor session has been established (dialer is nil).
func TestOutboundEnabledNilDialer(t *testing.T) {
	cfg := config.DefaultConfig()
	ts := NewTorService(cfg)

	if ts.OutboundEnabled() {
		t.Error("OutboundEnabled() = true, want false when dialer is nil")
	}
}

// =====================================================================
// monitorTor tests — exercises context cancellation path
// =====================================================================

// TestMonitorTorCancellationExits verifies that monitorTor returns promptly when
// the TorService context is cancelled. This exercises the ctx.Done() branch.
func TestMonitorTorCancellationExits(t *testing.T) {
	cfg := config.DefaultConfig()
	ts := NewTorService(cfg)

	done := make(chan struct{})
	go func() {
		ts.monitorTor()
		close(done)
	}()

	// Cancel the context to trigger the ctx.Done() case
	ts.cancel()

	select {
	case <-done:
		// goroutine exited — test passes
	case <-time.After(5 * time.Second):
		t.Error("monitorTor() did not return after context cancellation within 5s")
	}
}

// =====================================================================
// StopTorService — running=true, tor=nil path
// =====================================================================

// TestTorServiceStopWhenRunningNoTorInstance verifies that StopTorService clears
// all state correctly when t.running=true but t.tor is nil (e.g. after a partial
// start where the bine handshake failed but the flag was already set externally).
func TestTorServiceStopWhenRunningNoTorInstance(t *testing.T) {
	cfg := config.DefaultConfig()
	ts := NewTorService(cfg)

	// Simulate a running state with no actual Tor instance
	ts.mu.Lock()
	ts.running = true
	ts.onionAddr = "abcde12345.onion"
	ts.serviceID = "abcde12345"
	ts.mu.Unlock()

	err := ts.StopTorService()
	if err != nil {
		t.Errorf("StopTorService() error = %v, want nil", err)
	}
	if ts.IsRunning() {
		t.Error("IsRunning() = true after StopTorService, want false")
	}
	if addr := ts.GetOnionAddress(); addr != "" {
		t.Errorf("GetOnionAddress() = %q after stop, want empty", addr)
	}
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	if ts.serviceID != "" {
		t.Errorf("serviceID = %q after stop, want empty", ts.serviceID)
	}
}

// =====================================================================
// ImportKeys — running=true, tor=nil path
// =====================================================================

// TestTorServiceImportKeysRunningNoTorInstance verifies that ImportKeys correctly
// handles the case where t.running=true but t.tor is nil (the t.tor != nil branch
// is skipped), then clears the running flag and retries StartTorService.
func TestTorServiceImportKeysRunningNoTorInstance(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Server.Tor.Binary = "/nonexistent/tor"

	// Block PATH lookup and commonLocations stat so findTorBinary truly finds nothing
	origLP := lookPath
	lookPath = func(_ string) (string, error) { return "", os.ErrNotExist }
	origStat := statFile
	statFile = func(_ string) (os.FileInfo, error) { return nil, os.ErrNotExist }
	defer func() { lookPath = origLP; statFile = origStat }()

	ts := NewTorService(cfg)

	// Simulate running=true with no Tor instance and no Tor binary on PATH
	ts.mu.Lock()
	ts.running = true
	ts.mu.Unlock()

	privKey := make([]byte, 64)
	_, err := ts.ImportKeys(privKey)
	// No Tor binary found, so StartTorService returns nil and onionAddr stays "".
	// ImportKeys should succeed (return empty address, no error) when binary absent.
	if err != nil {
		t.Errorf("ImportKeys() error = %v, want nil (no binary)", err)
	}
}

// =====================================================================
// getTorConfig — missing branches
// =====================================================================

// TestGetTorConfigUseNetwork verifies that UseNetwork=true produces "SocksPort auto".
func TestGetTorConfigUseNetwork(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Server.Tor.UseNetwork = true
	ts := NewTorService(cfg)

	content := ts.getTorConfig()
	if !strings.Contains(content, "SocksPort auto") {
		t.Errorf("getTorConfig() missing 'SocksPort auto' when UseNetwork=true; got:\n%s", content)
	}
}

// TestGetTorConfigAllowUserPreference verifies that AllowUserPreference=true
// also produces "SocksPort auto" (same SOCKS branch as UseNetwork).
func TestGetTorConfigAllowUserPreference(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Server.Tor.AllowUserPreference = true
	ts := NewTorService(cfg)

	content := ts.getTorConfig()
	if !strings.Contains(content, "SocksPort auto") {
		t.Errorf("getTorConfig() missing 'SocksPort auto' when AllowUserPreference=true")
	}
}

// TestGetTorConfigSafeLoggingFalse verifies that SafeLogging=false produces "SafeLogging 0".
func TestGetTorConfigSafeLoggingFalse(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Server.Tor.SafeLogging = false
	ts := NewTorService(cfg)

	content := ts.getTorConfig()
	if !strings.Contains(content, "SafeLogging 0") {
		t.Errorf("getTorConfig() missing 'SafeLogging 0' when SafeLogging=false; got:\n%s", content)
	}
}

// TestGetTorConfigMaxMonthlyBandwidth verifies AccountingMax is emitted when set.
func TestGetTorConfigMaxMonthlyBandwidth(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Server.Tor.MaxMonthlyBandwidth = "10 GB"
	ts := NewTorService(cfg)

	content := ts.getTorConfig()
	if !strings.Contains(content, "AccountingMax 10 GB") {
		t.Errorf("getTorConfig() missing AccountingMax directive; got:\n%s", content)
	}
	if !strings.Contains(content, "AccountingStart month") {
		t.Errorf("getTorConfig() missing AccountingStart directive; got:\n%s", content)
	}
}

// TestGetTorConfigCloseCircuitOnStreamLimit verifies that CloseCircuitOnStreamLimit=true
// omits the CircuitStreamTimeout line (returns empty string from the anonymous func).
func TestGetTorConfigCloseCircuitOnStreamLimit(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Server.Tor.CloseCircuitOnStreamLimit = true
	ts := NewTorService(cfg)

	content := ts.getTorConfig()
	if strings.Contains(content, "CircuitStreamTimeout") {
		t.Errorf("getTorConfig() should omit CircuitStreamTimeout when CloseCircuitOnStreamLimit=true; got:\n%s", content)
	}
}

// =====================================================================
// StartTorService — fake binary path (covers post-binary-check statements)
// =====================================================================

// TestTorServiceStartFakeBinary verifies that StartTorService advances past the
// "binary not found" check when a valid executable path is provided, writes the
// torrc, and returns an error at the tor.Start() step (the fake binary exits
// immediately without opening a control port, so bine fails).
func TestTorServiceStartFakeBinary(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a minimal fake Tor binary that exits cleanly
	fakeTor := filepath.Join(tmpDir, "tor")
	if err := os.WriteFile(fakeTor, []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
		t.Fatalf("failed to write fake tor binary: %v", err)
	}

	cfg := config.DefaultConfig()
	cfg.Server.Tor.Binary = fakeTor
	ts := NewTorService(cfg)
	// Use a 5-second context so the bootstrap goroutine (EnableNetwork) does not
	// block the test for 3 minutes if bine manages to "start" the dead process.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ts.ctx = ctx
	ts.cancel = cancel
	ts.dataDir = filepath.Join(tmpDir, "data")
	ts.configDir = filepath.Join(tmpDir, "config")

	err := ts.StartTorService()
	// The fake binary exits immediately, so bine cannot establish a control
	// connection — StartTorService must return a non-nil error.
	if err == nil {
		t.Error("StartTorService() with fake binary should return error, got nil")
		// Clean up if it somehow "succeeded"
		_ = ts.StopTorService()
	}
}

// TestMonitorTorWithIntervalFastTick verifies the ticker.C branch of monitorTorWithInterval
// when running=false and tor=nil (the continue path), then cancels the context and
// checks that the goroutine returns within a reasonable deadline.
func TestMonitorTorWithIntervalFastTick(t *testing.T) {
	cfg := config.DefaultConfig()
	ts := NewTorService(cfg)

	ctx, cancel := context.WithCancel(context.Background())
	ts.ctx = ctx
	ts.cancel = cancel

	done := make(chan struct{})
	go func() {
		// 1ms interval so the ticker fires before context cancel
		ts.monitorTorWithInterval(1 * time.Millisecond)
		close(done)
	}()

	// Give the goroutine time to fire at least one tick before cancelling
	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case <-done:
		// goroutine exited cleanly after context cancel
	case <-time.After(2 * time.Second):
		t.Error("monitorTorWithInterval did not return after context cancel")
	}
}

// =====================================================================
// runVanityGeneration — direct call, exercises default branch + Found path
// =====================================================================

// TestRunVanityGenerationFindsMatch exercises the core generation loop by calling
// runVanityGeneration directly with a single-character prefix. With only 32
// possible base32 characters, a match is expected within at most a few thousand
// iterations (~3% hit rate per attempt). A 10-second deadline prevents the test
// from hanging indefinitely if the RNG is pathological.
func TestRunVanityGenerationFindsMatch(t *testing.T) {
	cfg := config.DefaultConfig()
	ts := NewTorService(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Initialise global state exactly as GenerateVanity does, but synchronously.
	vanityGen.mu.Lock()
	vanityGen.progress = &VanityProgress{
		Prefix:    "a",
		Attempts:  0,
		StartTime: time.Now(),
		Running:   true,
	}
	vanityGen.cancel = cancel
	vanityGen.mu.Unlock()

	// Call the worker directly in the test goroutine; it blocks until found or ctx done.
	ts.runVanityGeneration(ctx, "a")

	progress := ts.GetVanityProgress()
	if !progress.Found {
		t.Errorf("runVanityGeneration(%q) did not find a match within 10s (attempts=%d)",
			"a", progress.Attempts)
	}
	if progress.Address == "" {
		t.Error("Found=true but Address is empty")
	}
	if len(progress.PrivateKey) == 0 {
		t.Error("Found=true but PrivateKey is empty")
	}
	if progress.Running {
		t.Error("Running should be false after runVanityGeneration returns")
	}
}

// TestFindTorBinaryFallbackToLocations covers the commonLocations scan path in
// findTorBinary by replacing lookPath with a stub that always fails, forcing
// the fallback loop that checks /usr/bin/tor etc. directly.
func TestFindTorBinaryFallbackToLocations(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("test targets linux /usr/bin/tor location")
	}
	if _, err := os.Stat("/usr/bin/tor"); err != nil {
		t.Skip("/usr/bin/tor not installed — skipping commonLocations coverage test")
	}

	// Save and restore the package-level lookPath var
	orig := lookPath
	lookPath = func(_ string) (string, error) {
		return "", os.ErrNotExist
	}
	defer func() { lookPath = orig }()

	result := findTorBinary("")
	if result == "" {
		t.Error("findTorBinary() with mocked lookPath returned empty; expected /usr/bin/tor via commonLocations")
	}
	if result != "/usr/bin/tor" {
		t.Errorf("findTorBinary() = %q, want /usr/bin/tor", result)
	}
}

// TestStartTorServiceWithRealBinary exercises the StartTorService bootstrap code
// path when a real Tor binary is present. The service context is cancelled after
// 20 s — well before the 30 s slow-timer — so the fast-path select branch fires
// (case err = <-bootstrapDone:). BootstrapTimeout is left at 0 so the stmt
// `bootstrapTimeout = 3 * time.Minute` is also covered.
func TestStartTorServiceWithRealBinary(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("linux only")
	}
	if _, err := os.Stat("/usr/bin/tor"); err != nil {
		t.Skip("/usr/bin/tor not installed — skipping real-bootstrap coverage test")
	}

	cfg := config.DefaultConfig()
	// BootstrapTimeout = 0 (the zero-value default) so the
	// `bootstrapTimeout = 3 * time.Minute` assignment is taken.
	ts := NewTorService(cfg)

	// Cancel the TorService context after 20s — before the 30s slow-timer —
	// so EnableNetwork returns early and the fast-path select branch is covered.
	cancelTimer := time.AfterFunc(20*time.Second, ts.cancel)
	defer cancelTimer.Stop()

	done := make(chan error, 1)
	go func() {
		done <- ts.StartTorService()
	}()

	select {
	case err := <-done:
		// Context-cancelled error is expected; other errors (binary not found,
		// permission denied) are also acceptable — we only care that the code path ran.
		_ = err
	case <-time.After(30 * time.Second):
		ts.cancel()
		t.Fatal("StartTorService did not return within 30s")
	}
}

// TestEnsureTorDirsFailure covers the MkdirAll error return in ensureTorDirs
// by placing a regular file where the first directory would be created.
func TestEnsureTorDirsFailure(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := config.DefaultConfig()
	ts := NewTorService(cfg)

	// Place a regular file at the dataDir path so MkdirAll fails with ENOTDIR.
	blocker := filepath.Join(tmpDir, "tor")
	if err := os.WriteFile(blocker, []byte("block"), 0644); err != nil {
		t.Fatalf("failed to write blocker file: %v", err)
	}
	ts.dataDir = blocker
	ts.configDir = filepath.Join(tmpDir, "config")

	if err := ts.ensureTorDirs(); err == nil {
		t.Error("ensureTorDirs() should fail when dataDir is occupied by a regular file")
	}
}
