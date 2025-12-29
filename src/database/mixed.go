package database

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// MixedModeConfig holds configuration for mixed mode database operation
// Per AI.md PART 24: Mixed Mode (heterogeneous database backends)
type MixedModeConfig struct {
	// ServerDBConfig for server.db (admin credentials, scheduler, audit)
	// Can be SQLite, PostgreSQL, or MySQL
	ServerDB DatabaseBackendConfig `yaml:"server_db"`

	// UsersDBConfig for users.db (user accounts, sessions)
	// Can be SQLite, PostgreSQL, or MySQL
	UsersDB DatabaseBackendConfig `yaml:"users_db"`
}

// DatabaseBackendConfig represents configuration for a single database backend
type DatabaseBackendConfig struct {
	Driver   string `yaml:"driver"`   // sqlite, postgres, mysql
	DSN      string `yaml:"dsn"`      // connection string (for remote databases)
	Path     string `yaml:"path"`     // file path (for SQLite)
	MaxOpen  int    `yaml:"max_open"` // max open connections
	MaxIdle  int    `yaml:"max_idle"` // max idle connections
	Lifetime int    `yaml:"lifetime"` // connection max lifetime in seconds
}

// DefaultMixedModeConfig returns default mixed mode configuration
// Both databases use SQLite by default (standalone mode)
func DefaultMixedModeConfig(dataDir string) *MixedModeConfig {
	return &MixedModeConfig{
		ServerDB: DatabaseBackendConfig{
			Driver:   "sqlite",
			Path:     filepath.Join(dataDir, "server.db"),
			MaxOpen:  10,
			MaxIdle:  5,
			Lifetime: 300,
		},
		UsersDB: DatabaseBackendConfig{
			Driver:   "sqlite",
			Path:     filepath.Join(dataDir, "users.db"),
			MaxOpen:  10,
			MaxIdle:  5,
			Lifetime: 300,
		},
	}
}

// MixedModeManager manages databases with potentially different backends
// Per AI.md PART 24: Mixed Mode (heterogeneous database backends)
type MixedModeManager struct {
	serverDB *MixedDB
	usersDB  *MixedDB
	config   *MixedModeConfig
	mu       sync.RWMutex
}

// MixedDB represents a database that can be SQLite or remote
type MixedDB struct {
	db     *sql.DB
	driver string
	dsn    string
	path   string
	mu     sync.RWMutex
	ready  bool
}

// NewMixedModeManager creates a new mixed mode manager
func NewMixedModeManager(cfg *MixedModeConfig) (*MixedModeManager, error) {
	if cfg == nil {
		return nil, fmt.Errorf("mixed mode configuration is nil")
	}

	mm := &MixedModeManager{
		config: cfg,
	}

	// Connect server database
	serverDB, err := mm.connectBackend(&cfg.ServerDB)
	if err != nil {
		return nil, fmt.Errorf("failed to connect server database: %w", err)
	}
	mm.serverDB = serverDB

	// Connect users database
	usersDB, err := mm.connectBackend(&cfg.UsersDB)
	if err != nil {
		serverDB.Close()
		return nil, fmt.Errorf("failed to connect users database: %w", err)
	}
	mm.usersDB = usersDB

	log.Printf("[MixedMode] Connected: server=%s, users=%s",
		cfg.ServerDB.Driver, cfg.UsersDB.Driver)

	return mm, nil
}

// connectBackend connects to a database backend based on configuration
func (mm *MixedModeManager) connectBackend(cfg *DatabaseBackendConfig) (*MixedDB, error) {
	mdb := &MixedDB{
		driver: cfg.Driver,
		dsn:    cfg.DSN,
		path:   cfg.Path,
	}

	var err error

	switch cfg.Driver {
	case "sqlite", "sqlite3":
		// Ensure directory exists
		if cfg.Path != "" {
			dir := filepath.Dir(cfg.Path)
			if err := os.MkdirAll(dir, 0755); err != nil {
				return nil, fmt.Errorf("failed to create database directory: %w", err)
			}
		}

		// Use modernc.org/sqlite (pure Go SQLite)
		mdb.db, err = sql.Open("sqlite", cfg.Path)
		if err != nil {
			return nil, fmt.Errorf("failed to open SQLite: %w", err)
		}

		// Enable foreign keys and WAL mode
		if _, err := mdb.db.Exec("PRAGMA foreign_keys = ON"); err != nil {
			return nil, fmt.Errorf("failed to enable foreign keys: %w", err)
		}
		if _, err := mdb.db.Exec("PRAGMA journal_mode = WAL"); err != nil {
			return nil, fmt.Errorf("failed to enable WAL mode: %w", err)
		}

	case "postgres", "mysql", "mariadb":
		// Open remote database connection
		mdb.db, err = sql.Open(cfg.Driver, cfg.DSN)
		if err != nil {
			return nil, fmt.Errorf("failed to open %s: %w", cfg.Driver, err)
		}

	default:
		return nil, fmt.Errorf("unsupported database driver: %s", cfg.Driver)
	}

	// Configure connection pool
	if cfg.MaxOpen > 0 {
		mdb.db.SetMaxOpenConns(cfg.MaxOpen)
	}
	if cfg.MaxIdle > 0 {
		mdb.db.SetMaxIdleConns(cfg.MaxIdle)
	}
	if cfg.Lifetime > 0 {
		mdb.db.SetConnMaxLifetime(time.Duration(cfg.Lifetime) * time.Second)
	}

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := mdb.db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	mdb.ready = true
	return mdb, nil
}

// ServerDB returns the server database
func (mm *MixedModeManager) ServerDB() *MixedDB {
	mm.mu.RLock()
	defer mm.mu.RUnlock()
	return mm.serverDB
}

// UsersDB returns the users database
func (mm *MixedModeManager) UsersDB() *MixedDB {
	mm.mu.RLock()
	defer mm.mu.RUnlock()
	return mm.usersDB
}

// Close closes both database connections
func (mm *MixedModeManager) Close() error {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	var errs []error
	if mm.serverDB != nil {
		if err := mm.serverDB.Close(); err != nil {
			errs = append(errs, fmt.Errorf("server db: %w", err))
		}
	}
	if mm.usersDB != nil {
		if err := mm.usersDB.Close(); err != nil {
			errs = append(errs, fmt.Errorf("users db: %w", err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors closing databases: %v", errs)
	}
	return nil
}

// IsReady returns true if both databases are ready
func (mm *MixedModeManager) IsReady() bool {
	mm.mu.RLock()
	defer mm.mu.RUnlock()
	return mm.serverDB != nil && mm.serverDB.IsReady() &&
		mm.usersDB != nil && mm.usersDB.IsReady()
}

// IsMixedMode returns true if the databases use different backends
func (mm *MixedModeManager) IsMixedMode() bool {
	mm.mu.RLock()
	defer mm.mu.RUnlock()
	return mm.config.ServerDB.Driver != mm.config.UsersDB.Driver
}

// GetMode returns a string describing the current mode
func (mm *MixedModeManager) GetMode() string {
	mm.mu.RLock()
	defer mm.mu.RUnlock()

	if mm.config.ServerDB.Driver == mm.config.UsersDB.Driver {
		if mm.config.ServerDB.Driver == "sqlite" {
			return "standalone"
		}
		return "cluster"
	}
	return "mixed"
}

// GetStatus returns detailed status information
func (mm *MixedModeManager) GetStatus() map[string]interface{} {
	mm.mu.RLock()
	defer mm.mu.RUnlock()

	return map[string]interface{}{
		"mode":              mm.GetMode(),
		"mixed_mode":        mm.config.ServerDB.Driver != mm.config.UsersDB.Driver,
		"server_db_driver":  mm.config.ServerDB.Driver,
		"users_db_driver":   mm.config.UsersDB.Driver,
		"server_db_ready":   mm.serverDB != nil && mm.serverDB.IsReady(),
		"users_db_ready":    mm.usersDB != nil && mm.usersDB.IsReady(),
	}
}

// MixedDB methods

// Close closes the database connection
func (mdb *MixedDB) Close() error {
	mdb.mu.Lock()
	defer mdb.mu.Unlock()

	if mdb.db != nil {
		mdb.ready = false
		return mdb.db.Close()
	}
	return nil
}

// IsReady returns true if the database is ready
func (mdb *MixedDB) IsReady() bool {
	mdb.mu.RLock()
	defer mdb.mu.RUnlock()
	return mdb.ready
}

// Driver returns the database driver name
func (mdb *MixedDB) Driver() string {
	return mdb.driver
}

// IsLocal returns true if using SQLite (local storage)
func (mdb *MixedDB) IsLocal() bool {
	return mdb.driver == "sqlite" || mdb.driver == "sqlite3"
}

// IsRemote returns true if using a remote database
func (mdb *MixedDB) IsRemote() bool {
	return !mdb.IsLocal()
}

// Exec executes a query without returning rows
func (mdb *MixedDB) Exec(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	mdb.mu.RLock()
	defer mdb.mu.RUnlock()

	if !mdb.ready {
		return nil, fmt.Errorf("database not ready")
	}

	// Adapt query for different drivers if needed
	query = mdb.adaptQuery(query)

	return mdb.db.ExecContext(ctx, query, args...)
}

// Query executes a query that returns rows
func (mdb *MixedDB) Query(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	mdb.mu.RLock()
	defer mdb.mu.RUnlock()

	if !mdb.ready {
		return nil, fmt.Errorf("database not ready")
	}

	query = mdb.adaptQuery(query)

	return mdb.db.QueryContext(ctx, query, args...)
}

// QueryRow executes a query that returns a single row
func (mdb *MixedDB) QueryRow(ctx context.Context, query string, args ...interface{}) *sql.Row {
	mdb.mu.RLock()
	defer mdb.mu.RUnlock()

	query = mdb.adaptQuery(query)

	return mdb.db.QueryRowContext(ctx, query, args...)
}

// Begin starts a transaction
func (mdb *MixedDB) Begin(ctx context.Context) (*sql.Tx, error) {
	mdb.mu.RLock()
	defer mdb.mu.RUnlock()

	if !mdb.ready {
		return nil, fmt.Errorf("database not ready")
	}

	return mdb.db.BeginTx(ctx, nil)
}

// Ping checks database connectivity
func (mdb *MixedDB) Ping(ctx context.Context) error {
	mdb.mu.RLock()
	defer mdb.mu.RUnlock()

	if !mdb.ready || mdb.db == nil {
		return fmt.Errorf("database not ready")
	}

	return mdb.db.PingContext(ctx)
}

// adaptQuery adapts a query for different database drivers
func (mdb *MixedDB) adaptQuery(query string) string {
	// SQLite uses ? for placeholders
	// PostgreSQL uses $1, $2, etc.
	// MySQL uses ?

	// For now, we assume queries are written with ? placeholders
	// This works for SQLite and MySQL
	// PostgreSQL queries might need adaptation

	return query
}

// SupportsReturning returns true if the database supports RETURNING clause
func (mdb *MixedDB) SupportsReturning() bool {
	switch mdb.driver {
	case "postgres":
		return true
	case "sqlite", "sqlite3":
		// SQLite 3.35+ supports RETURNING
		return true
	default:
		return false
	}
}

// SupportsUpsert returns true if the database supports upsert operations
func (mdb *MixedDB) SupportsUpsert() bool {
	switch mdb.driver {
	case "postgres":
		return true // ON CONFLICT
	case "sqlite", "sqlite3":
		return true // ON CONFLICT
	case "mysql", "mariadb":
		return true // ON DUPLICATE KEY UPDATE
	default:
		return false
	}
}

// GetPlaceholder returns the placeholder format for the database
func (mdb *MixedDB) GetPlaceholder(index int) string {
	switch mdb.driver {
	case "postgres":
		return fmt.Sprintf("$%d", index)
	default:
		return "?"
	}
}
