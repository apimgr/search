package database

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// DB represents a single database connection
type DB struct {
	db       *sql.DB
	driver   string
	dsn      string
	mu       sync.RWMutex
	ready    bool
}

// DatabaseManager manages both server and users databases per AI.md PART 24
// Two separate databases:
// - server.db: Admin credentials, server state, scheduler
// - users.db: User accounts, tokens, sessions
type DatabaseManager struct {
	serverDB *DB
	usersDB  *DB
	dataDir  string
	mu       sync.RWMutex
}

// Config holds database configuration
type Config struct {
	Driver   string `yaml:"driver"`    // sqlite, postgres, mysql
	DSN      string `yaml:"dsn"`       // connection string (for non-sqlite)
	DataDir  string `yaml:"data_dir"`  // data directory (for sqlite)
	MaxOpen  int    `yaml:"max_open"`  // max open connections
	MaxIdle  int    `yaml:"max_idle"`  // max idle connections
	Lifetime int    `yaml:"lifetime"`  // connection max lifetime in seconds
}

// DefaultConfig returns default database configuration
func DefaultConfig() *Config {
	return &Config{
		Driver:   "sqlite",
		DataDir:  "/data/db",
		MaxOpen:  10,
		MaxIdle:  5,
		Lifetime: 300,
	}
}

// NewDatabaseManager creates a new database manager with two databases
func NewDatabaseManager(cfg *Config) (*DatabaseManager, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	dm := &DatabaseManager{
		dataDir: cfg.DataDir,
	}

	// Ensure database directory exists
	if err := os.MkdirAll(cfg.DataDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	// Create server database
	serverDB, err := dm.connectDatabase(cfg, "server.db")
	if err != nil {
		return nil, fmt.Errorf("failed to connect server database: %w", err)
	}
	dm.serverDB = serverDB

	// Create users database
	usersDB, err := dm.connectDatabase(cfg, "users.db")
	if err != nil {
		serverDB.Close()
		return nil, fmt.Errorf("failed to connect users database: %w", err)
	}
	dm.usersDB = usersDB

	return dm, nil
}

// connectDatabase creates a connection to a specific database
func (dm *DatabaseManager) connectDatabase(cfg *Config, dbName string) (*DB, error) {
	db := &DB{
		driver: cfg.Driver,
	}

	switch cfg.Driver {
	case "sqlite", "sqlite3":
		// Build DSN for SQLite
		db.dsn = filepath.Join(cfg.DataDir, dbName)
		var err error
		db.db, err = sql.Open("sqlite", db.dsn)
		if err != nil {
			return nil, fmt.Errorf("failed to open database: %w", err)
		}
	default:
		return nil, fmt.Errorf("unsupported database driver: %s", cfg.Driver)
	}

	// Configure connection pool
	db.db.SetMaxOpenConns(cfg.MaxOpen)
	db.db.SetMaxIdleConns(cfg.MaxIdle)
	db.db.SetConnMaxLifetime(time.Duration(cfg.Lifetime) * time.Second)

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	// Enable foreign keys and WAL mode for SQLite
	if cfg.Driver == "sqlite" || cfg.Driver == "sqlite3" {
		if _, err := db.db.Exec("PRAGMA foreign_keys = ON"); err != nil {
			return nil, fmt.Errorf("failed to enable foreign keys: %w", err)
		}
		if _, err := db.db.Exec("PRAGMA journal_mode = WAL"); err != nil {
			return nil, fmt.Errorf("failed to enable WAL mode: %w", err)
		}
	}

	db.ready = true
	return db, nil
}

// ServerDB returns the server database connection
func (dm *DatabaseManager) ServerDB() *DB {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	return dm.serverDB
}

// UsersDB returns the users database connection
func (dm *DatabaseManager) UsersDB() *DB {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	return dm.usersDB
}

// Close closes both database connections
func (dm *DatabaseManager) Close() error {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	var errs []error
	if dm.serverDB != nil {
		if err := dm.serverDB.Close(); err != nil {
			errs = append(errs, fmt.Errorf("server db: %w", err))
		}
	}
	if dm.usersDB != nil {
		if err := dm.usersDB.Close(); err != nil {
			errs = append(errs, fmt.Errorf("users db: %w", err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors closing databases: %v", errs)
	}
	return nil
}

// IsReady returns true if both databases are ready
func (dm *DatabaseManager) IsReady() bool {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	return dm.serverDB != nil && dm.serverDB.IsReady() &&
		dm.usersDB != nil && dm.usersDB.IsReady()
}

// Ping checks connectivity to both databases
func (dm *DatabaseManager) Ping(ctx context.Context) error {
	if err := dm.serverDB.Ping(ctx); err != nil {
		return fmt.Errorf("server db: %w", err)
	}
	if err := dm.usersDB.Ping(ctx); err != nil {
		return fmt.Errorf("users db: %w", err)
	}
	return nil
}

// New creates a new database connection (legacy single-database mode for migration)
func New(cfg *Config) (*DB, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	db := &DB{
		driver: cfg.Driver,
		dsn:    cfg.DSN,
	}

	if err := db.connect(cfg); err != nil {
		return nil, err
	}

	return db, nil
}

// connect establishes database connection
func (db *DB) connect(cfg *Config) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	var err error

	switch cfg.Driver {
	case "sqlite", "sqlite3":
		// Use modernc.org/sqlite (pure Go SQLite)
		db.db, err = sql.Open("sqlite", cfg.DSN)
	default:
		return fmt.Errorf("unsupported database driver: %s", cfg.Driver)
	}

	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}

	// Configure connection pool
	db.db.SetMaxOpenConns(cfg.MaxOpen)
	db.db.SetMaxIdleConns(cfg.MaxIdle)
	db.db.SetConnMaxLifetime(time.Duration(cfg.Lifetime) * time.Second)

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.db.PingContext(ctx); err != nil {
		return fmt.Errorf("failed to ping database: %w", err)
	}

	// Enable foreign keys for SQLite
	if cfg.Driver == "sqlite" || cfg.Driver == "sqlite3" {
		if _, err := db.db.Exec("PRAGMA foreign_keys = ON"); err != nil {
			return fmt.Errorf("failed to enable foreign keys: %w", err)
		}
		// Enable WAL mode for better concurrency
		if _, err := db.db.Exec("PRAGMA journal_mode = WAL"); err != nil {
			return fmt.Errorf("failed to enable WAL mode: %w", err)
		}
	}

	db.ready = true
	return nil
}

// Close closes the database connection
func (db *DB) Close() error {
	db.mu.Lock()
	defer db.mu.Unlock()

	if db.db != nil {
		db.ready = false
		return db.db.Close()
	}
	return nil
}

// IsReady returns true if database is ready
func (db *DB) IsReady() bool {
	db.mu.RLock()
	defer db.mu.RUnlock()
	return db.ready
}

// Exec executes a query without returning rows
func (db *DB) Exec(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	if !db.ready {
		return nil, fmt.Errorf("database not ready")
	}

	return db.db.ExecContext(ctx, query, args...)
}

// Query executes a query that returns rows
func (db *DB) Query(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	if !db.ready {
		return nil, fmt.Errorf("database not ready")
	}

	return db.db.QueryContext(ctx, query, args...)
}

// QueryRow executes a query that returns a single row
func (db *DB) QueryRow(ctx context.Context, query string, args ...interface{}) *sql.Row {
	db.mu.RLock()
	defer db.mu.RUnlock()

	return db.db.QueryRowContext(ctx, query, args...)
}

// Begin starts a transaction
func (db *DB) Begin(ctx context.Context) (*sql.Tx, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	if !db.ready {
		return nil, fmt.Errorf("database not ready")
	}

	return db.db.BeginTx(ctx, nil)
}

// Driver returns the database driver name
func (db *DB) Driver() string {
	return db.driver
}

// SQL returns the underlying *sql.DB connection
// Use with caution - prefer the DB methods for standard operations
func (db *DB) SQL() *sql.DB {
	db.mu.RLock()
	defer db.mu.RUnlock()
	return db.db
}

// Ping checks database connectivity
func (db *DB) Ping(ctx context.Context) error {
	db.mu.RLock()
	defer db.mu.RUnlock()

	if !db.ready || db.db == nil {
		return fmt.Errorf("database not ready")
	}

	return db.db.PingContext(ctx)
}
