package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	// Database drivers per AI.md PART 5: only SQLite and libSQL allowed
	// libSQL/Turso
	_ "github.com/tursodatabase/libsql-client-go/libsql"
	// SQLite
	_ "modernc.org/sqlite"
)

// normalizeDriver maps user-friendly config values to actual Go driver names.
// Per AI.md PART 5: only modernc.org/sqlite and libsql-client-go are allowed
func normalizeDriver(driver string) string {
	switch strings.ToLower(strings.TrimSpace(driver)) {
	case "sqlite", "sqlite2", "sqlite3":
		return "sqlite"
	case "libsql", "turso":
		return "libsql"
	default:
		return driver
	}
}

// DB represents a single database connection
type DB struct {
	db     *sql.DB
	driver string
	dsn    string
	mu     sync.RWMutex
	ready  bool
}

// DatabaseManager manages both server and users databases per AI.md PART 24
// Two separate databases:
// - server.db: Admin credentials, server state, scheduler
// - user.db: User accounts, tokens, sessions
type DatabaseManager struct {
	serverDB *DB
	usersDB  *DB
	dataDir  string
	mu       sync.RWMutex
}

// Config holds database configuration
type Config struct {
	// sqlite or libsql (turso)
	Driver string `yaml:"driver"`
	// connection string (for non-sqlite)
	DSN string `yaml:"dsn"`
	// data directory (for sqlite)
	DataDir string `yaml:"data_dir"`
	// max open connections
	MaxOpen int `yaml:"max_open"`
	// max idle connections
	MaxIdle int `yaml:"max_idle"`
	// connection max lifetime in seconds
	Lifetime int `yaml:"lifetime"`
	// max idle time in seconds before an idle connection is closed
	// (AI.md PART 10 "Connection Pool Configuration": server.database.pool.max_idle_time)
	IdleTime int `yaml:"max_idle_time"`
}

// DefaultConfig returns default database configuration
func DefaultConfig() *Config {
	return &Config{
		Driver:   "sqlite",
		DataDir:  "/data/db",
		MaxOpen:  10,
		MaxIdle:  5,
		Lifetime: 300,
		IdleTime: 60,
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
	usersDB, err := dm.connectDatabase(cfg, "user.db")
	if err != nil {
		return nil, fmt.Errorf("failed to connect users database: %w", errors.Join(err, serverDB.Close()))
	}
	dm.usersDB = usersDB

	return dm, nil
}

// connectDatabase creates a connection to a specific database
// Per AI.md PART 5: Multi-database driver support with config aliases
func (dm *DatabaseManager) connectDatabase(cfg *Config, dbName string) (*DB, error) {
	// Normalize driver name per AI.md PART 5
	normalizedDriver := normalizeDriver(cfg.Driver)

	db := &DB{
		driver: normalizedDriver,
	}

	var err error
	switch normalizedDriver {
	case "sqlite":
		// Build DSN for SQLite (modernc.org/sqlite). PRAGMAs are embedded in the
		// DSN (not just run once after Ping) so every connection the pool opens
		// gets them — required now that MaxOpenConns is no longer hardcoded to 1.
		db.dsn = filepath.Join(cfg.DataDir, dbName)
		db.db, err = sql.Open("sqlite", sqliteDSNWithPragmas(db.dsn))
		if err != nil {
			return nil, fmt.Errorf("failed to open sqlite database: %w", err)
		}

	case "libsql":
		// libSQL/Turso - uses DSN from config
		// DSN format: libsql://your-db.turso.io?authToken=xxx
		// Or: https://your-db.turso.io with separate token
		if cfg.DSN == "" {
			return nil, fmt.Errorf("libsql requires DSN in config (libsql://host?authToken=xxx)")
		}
		db.dsn = cfg.DSN
		db.db, err = sql.Open("libsql", db.dsn)
		if err != nil {
			return nil, fmt.Errorf("failed to open libsql database: %w", err)
		}

	default:
		return nil, fmt.Errorf("unsupported database driver: %s (supported: sqlite, libsql)", cfg.Driver)
	}

	// Configure connection pool.
	// Per AI.md "Connection Pooling": SQLite allows 1 writer + concurrent WAL
	// readers, so cfg.MaxOpen/MaxIdle apply to sqlite the same as other
	// drivers — write conflicts are serialized by SQLite itself (busy_timeout
	// PRAGMA, embedded in the DSN above) rather than by capping the Go pool
	// at a single connection, which previously forced every read to queue
	// behind unrelated in-flight queries.
	db.applyPoolSettings(cfg)

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.db.PingContext(ctx); err != nil {
		// Close the handle the failed Ping left open; sql.Open succeeds
		// lazily, so returning here without Close leaks the pool.
		closeErr := db.db.Close()
		db.db = nil
		return nil, fmt.Errorf("failed to ping database: %w", errors.Join(err, closeErr))
	}

	db.ready = true
	return db, nil
}

// applyPoolSettings configures the connection pool per AI.md PART 10
// "Connection Pooling": max_open, max_idle, max_lifetime and max_idle_time
// are all mandatory. SQLite allows 1 writer + concurrent WAL readers, so
// cfg.MaxOpen/MaxIdle apply to sqlite the same as to libsql — write
// conflicts are serialized by SQLite itself via the busy_timeout PRAGMA
// embedded in the DSN rather than by capping the Go pool at one connection.
func (db *DB) applyPoolSettings(cfg *Config) {
	db.db.SetMaxOpenConns(cfg.MaxOpen)
	db.db.SetMaxIdleConns(cfg.MaxIdle)
	db.db.SetConnMaxLifetime(time.Duration(cfg.Lifetime) * time.Second)
	idleTime := cfg.IdleTime
	if idleTime <= 0 {
		idleTime = 60
	}
	db.db.SetConnMaxIdleTime(time.Duration(idleTime) * time.Second)
}

// sqliteDSNWithPragmas appends modernc.org/sqlite's shorthand _pragma query
// parameters to a plain file path DSN so every pooled connection (not just
// the one used for the startup Ping) gets foreign_keys/WAL/busy_timeout.
// An empty path is left untouched: modernc.org/sqlite only strips the query
// string from the DSN when '?' is not the first character, so appending the
// pragma suffix to an empty path would make the literal string
// "?_pragma=..." get opened as a filename instead of being parsed as query
// parameters.
func sqliteDSNWithPragmas(path string) string {
	if path == "" {
		return path
	}
	return path + "?_pragma=foreign_keys(ON)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"
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

// NewDB creates a new database connection (single-database mode for direct use)
func NewDB(cfg *Config) (*DB, error) {
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

	// Normalize driver name per AI.md PART 5
	normalizedDriver := normalizeDriver(cfg.Driver)
	db.driver = normalizedDriver

	var err error

	switch normalizedDriver {
	case "sqlite":
		// Use modernc.org/sqlite (pure Go SQLite). PRAGMAs embedded in the DSN
		// so every pooled connection gets them (see sqliteDSNWithPragmas).
		db.db, err = sql.Open("sqlite", sqliteDSNWithPragmas(cfg.DSN))
	case "libsql":
		// libSQL/Turso remote database
		db.db, err = sql.Open("libsql", cfg.DSN)
	default:
		// AI.md PART 10 lines 13482-13487 define exactly two backends: local
		// SQLite and remote libsql/Turso. No other driver is imported, so any
		// other name could only ever fail at sql.Open with a confusing
		// "unknown driver" - reject it here with the supported list instead.
		return fmt.Errorf("unsupported database driver: %s (supported: sqlite, libsql)", cfg.Driver)
	}

	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}

	// Configure connection pool.
	// Per AI.md "Connection Pooling": SQLite allows 1 writer + concurrent WAL
	// readers, so cfg.MaxOpen/MaxIdle apply to sqlite the same as other
	// drivers — write conflicts are serialized by SQLite itself (busy_timeout
	// PRAGMA, embedded in the DSN above) rather than by capping the Go pool
	// at a single connection, which previously forced every read to queue
	// behind unrelated in-flight queries.
	db.applyPoolSettings(cfg)

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.db.PingContext(ctx); err != nil {
		// Close the handle the failed Ping left open; sql.Open succeeds
		// lazily, so returning here without Close leaks the pool.
		closeErr := db.db.Close()
		db.db = nil
		return fmt.Errorf("failed to ping database: %w", errors.Join(err, closeErr))
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

// IsRemote returns true if using a remote database (not local SQLite)
// Per AI.md PART 5: remote databases (libsql/Turso) are supported in single-instance mode
func (db *DB) IsRemote() bool {
	db.mu.RLock()
	defer db.mu.RUnlock()
	// AI.md PART 10 defines exactly two backends: sqlite is local, libsql/Turso is remote
	return db.driver != "" && db.driver != "sqlite"
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
// CleanupExpiredAPITokens deletes expired API tokens from the server database.
func (db *DB) CleanupExpiredAPITokens(ctx context.Context) (int64, error) {
	query := "DELETE FROM " + serverTablePrefix(db) + "api_tokens WHERE expires_at IS NOT NULL AND expires_at <= CURRENT_TIMESTAMP"
	result, err := db.Exec(ctx, query)
	if err != nil {
		return 0, fmt.Errorf("delete expired API tokens: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("read expired API token cleanup count: %w", err)
	}
	return count, nil
}

func (db *DB) Ping(ctx context.Context) error {
	db.mu.RLock()
	defer db.mu.RUnlock()

	if !db.ready || db.db == nil {
		return fmt.Errorf("database not ready")
	}

	return db.db.PingContext(ctx)
}
