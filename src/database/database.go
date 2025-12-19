package database

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// DB represents the database connection
type DB struct {
	db       *sql.DB
	driver   string
	dsn      string
	mu       sync.RWMutex
	ready    bool
}

// Config holds database configuration
type Config struct {
	Driver   string `yaml:"driver"`    // sqlite, postgres, mysql
	DSN      string `yaml:"dsn"`       // connection string
	MaxOpen  int    `yaml:"max_open"`  // max open connections
	MaxIdle  int    `yaml:"max_idle"`  // max idle connections
	Lifetime int    `yaml:"lifetime"`  // connection max lifetime in seconds
}

// DefaultConfig returns default database configuration
func DefaultConfig() *Config {
	return &Config{
		Driver:   "sqlite",
		DSN:      "/data/db/search.db",
		MaxOpen:  10,
		MaxIdle:  5,
		Lifetime: 300,
	}
}

// New creates a new database connection
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

// Ping checks database connectivity
func (db *DB) Ping(ctx context.Context) error {
	db.mu.RLock()
	defer db.mu.RUnlock()

	if !db.ready || db.db == nil {
		return fmt.Errorf("database not ready")
	}

	return db.db.PingContext(ctx)
}
