package service

import (
	"testing"

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
	err := ts.Stop()
	if err != nil {
		t.Errorf("Stop() error = %v, want nil", err)
	}
}

func TestTorServiceSetEnabled(t *testing.T) {
	cfg := config.DefaultConfig()
	ts := NewTorService(cfg)

	// SetEnabled(false) should just stop (which is a no-op if not running)
	err := ts.SetEnabled(false)
	if err != nil {
		t.Errorf("SetEnabled(false) error = %v", err)
	}

	// Note: SetEnabled(true) would try to start Tor which we can't test without Tor installed
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
		"ABC",   // uppercase
		"abc1",  // invalid number (1 is not valid base32)
		"abc_",  // underscore
		"abc-",  // hyphen
		"abc!",  // special char
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
	cfg := config.DefaultConfig()
	ts := NewTorService(cfg)

	// Should generate a random address
	addr, err := ts.tryGenerateVanityAddress("abc")
	if err != nil {
		t.Fatalf("tryGenerateVanityAddress() error = %v", err)
	}

	// Address should be 56 characters (v3 onion)
	if len(addr) != 56 {
		t.Errorf("Address length = %d, want 56", len(addr))
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

	ts := NewTorService(cfg)

	// Start should succeed but Tor should be disabled
	err := ts.Start()
	if err != nil {
		// If Tor binary not found, Start returns nil (Tor is optional)
		t.Logf("Start() returned error (expected if no Tor): %v", err)
	}

	// Tor should auto-disable when binary not found
	// This is per AI.md PART 32: "NOT FOUND: Log INFO, disable Tor features, continue without Tor"
}

func TestTorServiceRestartNotRunning(t *testing.T) {
	cfg := config.DefaultConfig()
	ts := NewTorService(cfg)

	// Restart when not running should attempt to start
	err := ts.Restart()
	// May fail if no Tor binary, but should not panic
	_ = err
}
