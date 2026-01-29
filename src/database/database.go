package database

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	// Database drivers per AI.md PART 5
	_ "github.com/go-sql-driver/mysql"                            // MySQL/MariaDB
	_ "github.com/jackc/pgx/v5/stdlib"                             // PostgreSQL
	_ "github.com/microsoft/go-mssqldb"                            // MSSQL
	_ "github.com/tursodatabase/libsql-client-go/libsql"           // libSQL/Turso
	_ "modernc.org/sqlite"                                         // SQLite
)

// normalizeDriver maps user-friendly config values to actual Go driver names.
// Per AI.md PART 5: Database Drivers - Config Aliases
func normalizeDriver(driver string) string {
	switch strings.ToLower(strings.TrimSpace(driver)) {
	case "sqlite", "sqlite2", "sqlite3":
		return "sqlite"
	case "libsql", "turso":
		return "libsql"
	case "postgres", "pgsql", "postgresql", "pgx":
		return "pgx"
	case "mysql", "mariadb":
		return "mysql"
	case "mssql", "sqlserver":
		return "sqlserver"
	default:
		return driver
	}
}

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
// - user.db: User accounts, tokens, sessions
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
	usersDB, err := dm.connectDatabase(cfg, "user.db")
	if err != nil {
		serverDB.Close()
		return nil, fmt.Errorf("failed to connect users database: %w", err)
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
		// Build DSN for SQLite (modernc.org/sqlite)
		db.dsn = filepath.Join(cfg.DataDir, dbName)
		db.db, err = sql.Open("sqlite", db.dsn)
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

	case "pgx":
		// PostgreSQL - uses DSN from config
		// DSN format: postgres://user:password@host:port/database?sslmode=require
		if cfg.DSN == "" {
			return nil, fmt.Errorf("postgres requires DSN in config")
		}
		db.dsn = cfg.DSN
		db.db, err = sql.Open("pgx", db.dsn)
		if err != nil {
			return nil, fmt.Errorf("failed to open postgres database: %w", err)
		}

	case "mysql":
		// MySQL/MariaDB - uses DSN from config
		// DSN format: user:password@tcp(host:port)/database?parseTime=true&charset=utf8mb4
		if cfg.DSN == "" {
			return nil, fmt.Errorf("mysql requires DSN in config")
		}
		db.dsn = cfg.DSN
		db.db, err = sql.Open("mysql", db.dsn)
		if err != nil {
			return nil, fmt.Errorf("failed to open mysql database: %w", err)
		}

	case "sqlserver":
		// MSSQL - uses DSN from config
		// DSN format: sqlserver://user:password@host:port?database=dbname
		if cfg.DSN == "" {
			return nil, fmt.Errorf("mssql requires DSN in config")
		}
		db.dsn = cfg.DSN
		db.db, err = sql.Open("sqlserver", db.dsn)
		if err != nil {
			return nil, fmt.Errorf("failed to open mssql database: %w", err)
		}

	default:
		return nil, fmt.Errorf("unsupported database driver: %s (supported: sqlite, libsql, postgres, mysql, mssql)", cfg.Driver)
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
	// busy_timeout allows concurrent access without immediate "database locked" errors
	if normalizedDriver == "sqlite" {
		if _, err := db.db.Exec("PRAGMA foreign_keys = ON"); err != nil {
			return nil, fmt.Errorf("failed to enable foreign keys: %w", err)
		}
		if _, err := db.db.Exec("PRAGMA journal_mode = WAL"); err != nil {
			return nil, fmt.Errorf("failed to enable WAL mode: %w", err)
		}
		if _, err := db.db.Exec("PRAGMA busy_timeout = 5000"); err != nil {
			return nil, fmt.Errorf("failed to set busy timeout: %w", err)
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

// IsClusterMode returns true if running in cluster mode (remote database)
// Per AI.md PART 5: Cluster mode uses remote database as source of truth
func (dm *DatabaseManager) IsClusterMode() bool {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	if dm.serverDB == nil {
		return false
	}
	return dm.serverDB.IsRemote()
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

	// Normalize driver name per AI.md PART 5
	normalizedDriver := normalizeDriver(cfg.Driver)
	db.driver = normalizedDriver

	var err error

	switch normalizedDriver {
	case "sqlite":
		// Use modernc.org/sqlite (pure Go SQLite)
		db.db, err = sql.Open("sqlite", cfg.DSN)
	case "libsql":
		// libSQL/Turso remote database
		db.db, err = sql.Open("libsql", cfg.DSN)
	case "pgx":
		// PostgreSQL
		db.db, err = sql.Open("pgx", cfg.DSN)
	case "mysql":
		// MySQL/MariaDB
		db.db, err = sql.Open("mysql", cfg.DSN)
	case "sqlserver":
		// MSSQL
		db.db, err = sql.Open("sqlserver", cfg.DSN)
	default:
		return fmt.Errorf("unsupported database driver: %s (supported: sqlite, libsql, postgres, mysql, mssql)", cfg.Driver)
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

	// Enable foreign keys, WAL mode, and busy timeout for SQLite
	if normalizedDriver == "sqlite" {
		if _, err := db.db.Exec("PRAGMA foreign_keys = ON"); err != nil {
			return fmt.Errorf("failed to enable foreign keys: %w", err)
		}
		if _, err := db.db.Exec("PRAGMA journal_mode = WAL"); err != nil {
			return fmt.Errorf("failed to enable WAL mode: %w", err)
		}
		// Set busy timeout to 5 seconds to prevent "database locked" errors
		if _, err := db.db.Exec("PRAGMA busy_timeout = 5000"); err != nil {
			return fmt.Errorf("failed to set busy timeout: %w", err)
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

// IsRemote returns true if using a remote database (not local SQLite)
// Per AI.md PART 5: Cluster mode uses remote database
func (db *DB) IsRemote() bool {
	db.mu.RLock()
	defer db.mu.RUnlock()
	// sqlite is local, all others (libsql, pgx, mysql, sqlserver) are remote
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
func (db *DB) Ping(ctx context.Context) error {
	db.mu.RLock()
	defer db.mu.RUnlock()

	if !db.ready || db.db == nil {
		return fmt.Errorf("database not ready")
	}

	return db.db.PingContext(ctx)
}
