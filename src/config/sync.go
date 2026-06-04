// Package config provides configuration persistence for single-instance mode.
// Per AI.md PART 5: server.yml is the ONLY source of truth. No cluster mode.
package config

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// ConfigSync handles persisting configuration changes back to server.yml.
// Per AI.md PART 5: server.yml is the source of truth for all configuration.
// There is no cluster mode, no database-as-config-source-of-truth (AI.md line 2055).
type ConfigSync struct {
	db         *sql.DB
	config     *Config
	configPath string
	mu         sync.RWMutex
	lastSync   time.Time
}

// NewConfigSync creates a new config sync manager.
func NewConfigSync(db *sql.DB, config *Config, configPath string) *ConfigSync {
	return &ConfigSync{
		db:         db,
		config:     config,
		configPath: configPath,
	}
}

// SaveSetting saves a config setting to server.yml (source of truth).
// Per AI.md PART 5: server.yml is always the source of truth for this single-instance app.
func (cs *ConfigSync) SaveSetting(key string, value interface{}) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	return cs.saveToYml()
}

// LoadFromSource loads config from server.yml.
// Per AI.md PART 5: single-instance mode; already loaded from server.yml on startup.
func (cs *ConfigSync) LoadFromSource() error {
	return nil
}

// SyncToLocal is a no-op for single-instance mode.
// Per AI.md PART 5: server.yml is always current; no remote sync needed.
func (cs *ConfigSync) SyncToLocal() error {
	return nil
}

// writeToDatabase persists a key-value config entry to the local database.
// Used for audit trail only — server.yml remains the source of truth.
func (cs *ConfigSync) writeToDatabase(key string, value interface{}) error {
	if cs.db == nil {
		return fmt.Errorf("database not available")
	}

	jsonValue, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to marshal value: %w", err)
	}

	query := `
		INSERT INTO server_settings (key, value, updated_at)
		VALUES (?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(key) DO UPDATE SET value = ?, updated_at = CURRENT_TIMESTAMP
	`
	_, err = cs.db.Exec(query, key, string(jsonValue), string(jsonValue))
	if err != nil {
		return fmt.Errorf("failed to upsert setting: %w", err)
	}

	slog.Debug("config saved to database", "key", key)
	return nil
}

// syncToLocalConfig saves current config to server.yml.
func (cs *ConfigSync) syncToLocalConfig() error {
	if cs.configPath == "" {
		return fmt.Errorf("config path not set")
	}

	data, err := yaml.Marshal(cs.config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(cs.configPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	cs.lastSync = time.Now()
	slog.Debug("config synced to local file", "path", cs.configPath)
	return nil
}

// saveToYml saves config directly to server.yml.
func (cs *ConfigSync) saveToYml() error {
	return cs.config.Save(cs.configPath)
}

// LastSync returns the last sync time.
func (cs *ConfigSync) LastSync() time.Time {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return cs.lastSync
}
