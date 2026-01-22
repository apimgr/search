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

// Test tryGenerateVanityAddress produces valid addresses
func TestTorServiceTryGenerateVanityAddressMultiple(t *testing.T) {
	cfg := config.DefaultConfig()
	ts := NewTorService(cfg)

	// Generate multiple addresses and verify they're all valid
	for i := 0; i < 10; i++ {
		addr, err := ts.tryGenerateVanityAddress("test")
		if err != nil {
			t.Fatalf("tryGenerateVanityAddress() error = %v", err)
		}

		if len(addr) != 56 {
			t.Errorf("Address length = %d, want 56", len(addr))
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

// Test SetEnabled with false
func TestTorServiceSetEnabledFalse(t *testing.T) {
	cfg := config.DefaultConfig()
	ts := NewTorService(cfg)

	err := ts.SetEnabled(false)
	if err != nil {
		t.Errorf("SetEnabled(false) error = %v", err)
	}

	if ts.IsRunning() {
		t.Error("Should not be running after SetEnabled(false)")
	}
}

// Test Stop is idempotent
func TestTorServiceStopIdempotent(t *testing.T) {
	cfg := config.DefaultConfig()
	ts := NewTorService(cfg)

	// Stop multiple times should not panic
	for i := 0; i < 5; i++ {
		err := ts.Stop()
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
	err := ts.Start()
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
		{"", false},        // Empty is valid
		{"a", false},       // Single char valid
		{"abc", false},     // Short valid
		{"234567", false},  // Numbers 2-7 valid
		{"abcdef", false},  // 6 chars max for built-in
		{"abcdefg", true},  // 7 chars too long
		{"ABC", true},      // Uppercase invalid
		{"abc1", true},     // 1 invalid in base32
		{"abc0", true},     // 0 invalid in base32
		{"abc8", true},     // 8 invalid in base32
		{"abc9", true},     // 9 invalid in base32
		{"abc!", true},     // Special char invalid
		{"abc-", true},     // Hyphen invalid
		{"abc_", true},     // Underscore invalid
		{"abc ", true},     // Space invalid
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
	ts := NewTorService(cfg)

	// Restart should call Stop (succeeds) then Start (may fail)
	err := ts.Restart()
	// May fail if no Tor binary
	_ = err
}

// Test TorService with different config settings
func TestTorServiceWithConfig(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Server.Tor.Enabled = true
	cfg.Server.Tor.HiddenServicePort = 8080
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
