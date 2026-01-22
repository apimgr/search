package config

import (
	"database/sql"
	"os"
	"testing"
	"time"
)

func TestNewConfigSync(t *testing.T) {
	cfg := DefaultConfig()
	cs := NewConfigSync(nil, cfg, "/test/path", false)

	if cs == nil {
		t.Fatal("NewConfigSync() returned nil")
	}
	if cs.config != cfg {
		t.Error("NewConfigSync() did not store config correctly")
	}
	if cs.configPath != "/test/path" {
		t.Errorf("NewConfigSync() configPath = %q, want %q", cs.configPath, "/test/path")
	}
	if cs.isCluster {
		t.Error("NewConfigSync() isCluster should be false")
	}
}

func TestNewConfigSyncClusterMode(t *testing.T) {
	cfg := DefaultConfig()
	cs := NewConfigSync(nil, cfg, "/test/path", true)

	if cs == nil {
		t.Fatal("NewConfigSync() returned nil")
	}
	if !cs.isCluster {
		t.Error("NewConfigSync() isCluster should be true")
	}
}

func TestConfigSyncIsClusterMode(t *testing.T) {
	cfg := DefaultConfig()

	cs := NewConfigSync(nil, cfg, "/test/path", true)
	if !cs.IsClusterMode() {
		t.Error("IsClusterMode() should return true for cluster mode")
	}

	cs = NewConfigSync(nil, cfg, "/test/path", false)
	if cs.IsClusterMode() {
		t.Error("IsClusterMode() should return false for standalone mode")
	}
}

func TestConfigSyncLastSync(t *testing.T) {
	cfg := DefaultConfig()
	cs := NewConfigSync(nil, cfg, "/test/path", false)

	// Initially zero time
	if !cs.LastSync().IsZero() {
		t.Error("LastSync() should return zero time initially")
	}

	// Set a time
	now := time.Now()
	cs.mu.Lock()
	cs.lastSync = now
	cs.mu.Unlock()

	got := cs.LastSync()
	if !got.Equal(now) {
		t.Errorf("LastSync() = %v, want %v", got, now)
	}
}

func TestConfigSyncSaveSettingStandalone(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "config-sync-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	configPath := tmpDir + "/server.yml"

	cfg := DefaultConfig()
	cs := NewConfigSync(nil, cfg, configPath, false)

	// Save a setting (standalone mode writes directly to yml)
	err = cs.SaveSetting("server.title", "TestTitle")
	if err != nil {
		t.Errorf("SaveSetting() error = %v", err)
	}

	// Verify file was created
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Error("SaveSetting() did not create config file")
	}
}

func TestConfigSyncSaveSettingClusterNoDb(t *testing.T) {
	cfg := DefaultConfig()
	cs := NewConfigSync(nil, cfg, "/test/path", true)

	// Save in cluster mode without database should error
	err := cs.SaveSetting("server.title", "TestTitle")
	if err == nil {
		t.Error("SaveSetting() in cluster mode without database should return error")
	}
}

func TestConfigSyncLoadFromSourceStandalone(t *testing.T) {
	cfg := DefaultConfig()
	cs := NewConfigSync(nil, cfg, "/test/path", false)

	// Standalone mode should return nil (already loaded)
	err := cs.LoadFromSource()
	if err != nil {
		t.Errorf("LoadFromSource() error = %v", err)
	}
}

func TestConfigSyncLoadFromSourceClusterNoDb(t *testing.T) {
	cfg := DefaultConfig()
	cs := NewConfigSync(nil, cfg, "/test/path", true)

	// Cluster mode without database should error
	err := cs.LoadFromSource()
	if err == nil {
		t.Error("LoadFromSource() in cluster mode without database should return error")
	}
}

func TestConfigSyncSyncToLocalStandalone(t *testing.T) {
	cfg := DefaultConfig()
	cs := NewConfigSync(nil, cfg, "/test/path", false)

	// Standalone mode should return nil (not needed)
	err := cs.SyncToLocal()
	if err != nil {
		t.Errorf("SyncToLocal() in standalone mode should return nil, got %v", err)
	}
}

func TestConfigSyncSyncToLocalClusterNoPath(t *testing.T) {
	cfg := DefaultConfig()
	cs := NewConfigSync(nil, cfg, "", true)

	// Cluster mode without config path should error
	err := cs.SyncToLocal()
	if err == nil {
		t.Error("SyncToLocal() in cluster mode without config path should return error")
	}
}

func TestConfigSyncSyncToLocalCluster(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "config-sync-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	configPath := tmpDir + "/server.yml"

	cfg := DefaultConfig()
	cfg.Server.Title = "SyncTest"
	cs := NewConfigSync(nil, cfg, configPath, true)

	// SyncToLocal should write to file
	err = cs.SyncToLocal()
	if err != nil {
		t.Errorf("SyncToLocal() error = %v", err)
	}

	// Verify file was created
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Error("SyncToLocal() did not create config file")
	}

	// Verify lastSync was updated
	if cs.LastSync().IsZero() {
		t.Error("SyncToLocal() should update lastSync")
	}
}

func TestConfigSyncStartPeriodicSyncStandalone(t *testing.T) {
	cfg := DefaultConfig()
	cs := NewConfigSync(nil, cfg, "/test/path", false)

	// Should not start goroutine in standalone mode
	// This just verifies it doesn't panic
	cs.StartPeriodicSync(time.Hour)
}

func TestConfigSyncStartPeriodicSyncCluster(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "config-sync-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	configPath := tmpDir + "/server.yml"

	cfg := DefaultConfig()
	cs := NewConfigSync(nil, cfg, configPath, true)

	// Start periodic sync with short interval
	cs.StartPeriodicSync(50 * time.Millisecond)

	// Wait for at least one sync
	time.Sleep(100 * time.Millisecond)

	// Verify file was created (sync happened)
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Error("StartPeriodicSync() should have synced config file")
	}
}

func TestConfigSyncWriteToDatabaseNoDb(t *testing.T) {
	cfg := DefaultConfig()
	cs := NewConfigSync(nil, cfg, "/test/path", true)

	err := cs.writeToDatabase("key", "value")
	if err == nil {
		t.Error("writeToDatabase() without database should return error")
	}
}

func TestConfigSyncLoadFromDatabaseNoDb(t *testing.T) {
	cfg := DefaultConfig()
	cs := NewConfigSync(nil, cfg, "/test/path", true)

	err := cs.loadFromDatabase()
	if err == nil {
		t.Error("loadFromDatabase() without database should return error")
	}
}

func TestConfigSyncApplySettings(t *testing.T) {
	cfg := DefaultConfig()
	cs := NewConfigSync(nil, cfg, "/test/path", false)

	settings := map[string]string{
		"server.title":       "AppliedTitle",
		"server.description": "AppliedDescription",
		"server.port":        "9999",
		"server.mode":        "development",
		"server.base_url":    "https://applied.example.com",
		"unknown.key":        "ignored",
	}

	err := cs.applySettings(settings)
	if err != nil {
		t.Errorf("applySettings() error = %v", err)
	}

	if cfg.Server.Title != "AppliedTitle" {
		t.Errorf("Title = %q, want %q", cfg.Server.Title, "AppliedTitle")
	}
	if cfg.Server.Description != "AppliedDescription" {
		t.Errorf("Description = %q, want %q", cfg.Server.Description, "AppliedDescription")
	}
	if cfg.Server.Port != 9999 {
		t.Errorf("Port = %d, want 9999", cfg.Server.Port)
	}
	if cfg.Server.Mode != "development" {
		t.Errorf("Mode = %q, want %q", cfg.Server.Mode, "development")
	}
	if cfg.Server.BaseURL != "https://applied.example.com" {
		t.Errorf("BaseURL = %q, want %q", cfg.Server.BaseURL, "https://applied.example.com")
	}
}

func TestConfigSyncApplySettingsInvalidPort(t *testing.T) {
	cfg := DefaultConfig()
	originalPort := cfg.Server.Port
	cs := NewConfigSync(nil, cfg, "/test/path", false)

	settings := map[string]string{
		"server.port": "invalid",
	}

	err := cs.applySettings(settings)
	if err != nil {
		t.Errorf("applySettings() error = %v", err)
	}

	// Port should remain unchanged with invalid value
	if cfg.Server.Port != originalPort {
		t.Errorf("Port = %d, should remain %d with invalid value", cfg.Server.Port, originalPort)
	}
}

func TestConfigSyncSaveToYml(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "config-sync-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	configPath := tmpDir + "/server.yml"

	cfg := DefaultConfig()
	cfg.Server.Title = "SaveToYmlTest"
	cs := NewConfigSync(nil, cfg, configPath, false)

	err = cs.saveToYml()
	if err != nil {
		t.Errorf("saveToYml() error = %v", err)
	}

	// Verify file was created
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Error("saveToYml() did not create config file")
	}

	// Load and verify content
	loaded, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if loaded.Server.Title != "SaveToYmlTest" {
		t.Errorf("Loaded title = %q, want %q", loaded.Server.Title, "SaveToYmlTest")
	}
}

func TestConfigSyncSyncToLocalConfig(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "config-sync-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	configPath := tmpDir + "/server.yml"

	cfg := DefaultConfig()
	cfg.Server.Title = "SyncToLocalConfigTest"
	cs := NewConfigSync(nil, cfg, configPath, true)

	err = cs.syncToLocalConfig()
	if err != nil {
		t.Errorf("syncToLocalConfig() error = %v", err)
	}

	// Verify file was created
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Error("syncToLocalConfig() did not create config file")
	}

	// Verify lastSync was updated
	if cs.LastSync().IsZero() {
		t.Error("syncToLocalConfig() should update lastSync")
	}
}

func TestConfigSyncSyncToLocalConfigNoPath(t *testing.T) {
	cfg := DefaultConfig()
	cs := NewConfigSync(nil, cfg, "", true)

	err := cs.syncToLocalConfig()
	if err == nil {
		t.Error("syncToLocalConfig() without config path should return error")
	}
}

// TestConfigSyncWithMockDatabase tests database operations with a mock
// This is a placeholder for when a proper database mock is available
func TestConfigSyncWithMockDatabase(t *testing.T) {
	// Note: This test would require a proper database mock
	// For now, we test that operations fail gracefully without a database

	cfg := DefaultConfig()

	// Test with nil database pointer
	var nilDB *sql.DB = nil
	cs := NewConfigSync(nilDB, cfg, "/test/path", true)

	// All database operations should fail gracefully
	if err := cs.writeToDatabase("key", "value"); err == nil {
		t.Error("writeToDatabase with nil db should return error")
	}

	if err := cs.loadFromDatabase(); err == nil {
		t.Error("loadFromDatabase with nil db should return error")
	}
}
