package config

import (
	"os"
	"testing"
	"time"
)

func TestNewConfigSync(t *testing.T) {
	cfg := DefaultConfig()
	cs := NewConfigSync(nil, cfg, "/test/path")

	if cs == nil {
		t.Fatal("NewConfigSync() returned nil")
	}
	if cs.config != cfg {
		t.Error("NewConfigSync() did not store config correctly")
	}
	if cs.configPath != "/test/path" {
		t.Errorf("NewConfigSync() configPath = %q, want %q", cs.configPath, "/test/path")
	}
}

func TestConfigSyncLastSync(t *testing.T) {
	cfg := DefaultConfig()
	cs := NewConfigSync(nil, cfg, "/test/path")

	if !cs.LastSync().IsZero() {
		t.Error("LastSync() should return zero time initially")
	}

	now := time.Now()
	cs.mu.Lock()
	cs.lastSync = now
	cs.mu.Unlock()

	got := cs.LastSync()
	if !got.Equal(now) {
		t.Errorf("LastSync() = %v, want %v", got, now)
	}
}

func TestConfigSyncSaveSetting(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "config-sync-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	configPath := tmpDir + "/server.yml"

	cfg := DefaultConfig()
	cs := NewConfigSync(nil, cfg, configPath)

	// SaveSetting writes to server.yml (source of truth)
	err = cs.SaveSetting("server.title", "TestTitle")
	if err != nil {
		t.Errorf("SaveSetting() error = %v", err)
	}

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Error("SaveSetting() did not create config file")
	}
}

func TestConfigSyncLoadFromSource(t *testing.T) {
	cfg := DefaultConfig()
	cs := NewConfigSync(nil, cfg, "/test/path")

	// Single-instance mode: always returns nil (config already loaded from server.yml)
	err := cs.LoadFromSource()
	if err != nil {
		t.Errorf("LoadFromSource() error = %v", err)
	}
}

func TestConfigSyncSyncToLocal(t *testing.T) {
	cfg := DefaultConfig()
	cs := NewConfigSync(nil, cfg, "/test/path")

	// Single-instance mode: always returns nil (server.yml is always current)
	err := cs.SyncToLocal()
	if err != nil {
		t.Errorf("SyncToLocal() in standalone mode should return nil, got %v", err)
	}
}

func TestConfigSyncWriteToDatabaseNoDb(t *testing.T) {
	cfg := DefaultConfig()
	cs := NewConfigSync(nil, cfg, "/test/path")

	err := cs.writeToDatabase("key", "value")
	if err == nil {
		t.Error("writeToDatabase() without database should return error")
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
	cs := NewConfigSync(nil, cfg, configPath)

	err = cs.saveToYml()
	if err != nil {
		t.Errorf("saveToYml() error = %v", err)
	}

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Error("saveToYml() did not create config file")
	}

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
	cs := NewConfigSync(nil, cfg, configPath)

	err = cs.syncToLocalConfig()
	if err != nil {
		t.Errorf("syncToLocalConfig() error = %v", err)
	}

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Error("syncToLocalConfig() did not create config file")
	}

	if cs.LastSync().IsZero() {
		t.Error("syncToLocalConfig() should update lastSync")
	}
}

func TestConfigSyncSyncToLocalConfigNoPath(t *testing.T) {
	cfg := DefaultConfig()
	cs := NewConfigSync(nil, cfg, "")

	err := cs.syncToLocalConfig()
	if err == nil {
		t.Error("syncToLocalConfig() without config path should return error")
	}
}
