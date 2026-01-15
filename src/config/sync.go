// Package config provides configuration sync for cluster mode
// Per AI.md PART 5 lines 5212-5310: Configuration Source of Truth (NON-NEGOTIABLE)
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

// ConfigSync handles syncing configuration between database and server.yml
// Per AI.md PART 5:
// - Single Instance (SQLite): server.yml is source of truth
// - Cluster Mode (Remote DB): Database is source of truth, server.yml is cache/backup
type ConfigSync struct {
	db         *sql.DB
	config     *Config
	configPath string
	isCluster  bool
	mu         sync.RWMutex
	lastSync   time.Time
}

// NewConfigSync creates a new config sync manager
func NewConfigSync(db *sql.DB, config *Config, configPath string, isCluster bool) *ConfigSync {
	return &ConfigSync{
		db:         db,
		config:     config,
		configPath: configPath,
		isCluster:  isCluster,
	}
}

// SaveSetting saves a config setting to the appropriate source of truth
// Per AI.md PART 5 lines 5263-5310: server.yml as Cache/Backup (NON-NEGOTIABLE)
func (cs *ConfigSync) SaveSetting(key string, value interface{}) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	if cs.isCluster {
		// Cluster mode: Database is source of truth
		// 1. Write to database
		if err := cs.writeToDatabase(key, value); err != nil {
			return fmt.Errorf("failed to write to database: %w", err)
		}
		// 2. Sync to local server.yml (cache)
		if err := cs.syncToLocalConfig(); err != nil {
			slog.Warn("failed to sync to local config", "error", err)
			// Non-fatal: database is source of truth
		}
	} else {
		// Single instance: server.yml is source of truth
		if err := cs.saveToYml(); err != nil {
			return fmt.Errorf("failed to save config: %w", err)
		}
	}

	return nil
}

// LoadFromSource loads config from the appropriate source
// Per AI.md PART 5: Cluster mode reads from database, standalone from server.yml
func (cs *ConfigSync) LoadFromSource() error {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	if cs.isCluster {
		// Load from database
		return cs.loadFromDatabase()
	}
	// Single instance: already loaded from server.yml
	return nil
}

// SyncToLocal syncs database config to local server.yml
// Per AI.md PART 5 lines 5291-5310: Config Sync (Database → server.yml)
func (cs *ConfigSync) SyncToLocal() error {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	if !cs.isCluster {
		return nil // Not needed in standalone mode
	}

	return cs.syncToLocalConfig()
}

// writeToDatabase writes a config key-value pair to the database
func (cs *ConfigSync) writeToDatabase(key string, value interface{}) error {
	if cs.db == nil {
		return fmt.Errorf("database not available")
	}

	// Serialize value to JSON
	jsonValue, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to marshal value: %w", err)
	}

	// Upsert the setting
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

// loadFromDatabase loads all settings from database into config
func (cs *ConfigSync) loadFromDatabase() error {
	if cs.db == nil {
		return fmt.Errorf("database not available")
	}

	rows, err := cs.db.Query("SELECT key, value FROM server_settings")
	if err != nil {
		return fmt.Errorf("failed to query settings: %w", err)
	}
	defer rows.Close()

	settings := make(map[string]string)
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			continue
		}
		settings[key] = value
	}

	// Apply settings to config
	if err := cs.applySettings(settings); err != nil {
		return fmt.Errorf("failed to apply settings: %w", err)
	}

	cs.lastSync = time.Now()
	slog.Debug("config loaded from database", "count", len(settings))
	return nil
}

// applySettings applies database settings to the config object
func (cs *ConfigSync) applySettings(settings map[string]string) error {
	for key, value := range settings {
		switch key {
		case "server.title":
			cs.config.Server.Title = value
		case "server.description":
			cs.config.Server.Description = value
		case "server.port":
			var port int
			if err := json.Unmarshal([]byte(value), &port); err == nil {
				cs.config.Server.Port = port
			}
		case "server.mode":
			cs.config.Server.Mode = value
		case "server.base_url":
			cs.config.Server.BaseURL = value
		// Add more settings as needed
		default:
			slog.Debug("unknown config key from database", "key", key)
		}
	}
	return nil
}

// syncToLocalConfig saves current config to server.yml
// Per AI.md PART 5 lines 5294-5306: Every config change synced to local server.yml
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

// saveToYml saves config directly to server.yml (standalone mode)
func (cs *ConfigSync) saveToYml() error {
	return cs.config.Save(cs.configPath)
}

// StartPeriodicSync starts a background goroutine to sync config periodically
// Per AI.md PART 5 lines 5308-5310: Sync periodically (every 5 minutes)
func (cs *ConfigSync) StartPeriodicSync(interval time.Duration) {
	if !cs.isCluster {
		return // Not needed in standalone mode
	}

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for range ticker.C {
			if err := cs.SyncToLocal(); err != nil {
				slog.Warn("periodic config sync failed", "error", err)
			}
		}
	}()

	slog.Info("config sync started", "interval", interval)
}

// IsClusterMode returns true if running in cluster mode
func (cs *ConfigSync) IsClusterMode() bool {
	return cs.isCluster
}

// LastSync returns the last sync time
func (cs *ConfigSync) LastSync() time.Time {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return cs.lastSync
}
