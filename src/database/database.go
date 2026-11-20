package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
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
	// db is the write handle and remains exposed for existing integrations.
	db           *sql.DB
	readDB       *sql.DB
	driver       string
	dsn          string
	mu           sync.RWMutex
	ready        bool
	busyObserver func(time.Duration, string)
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
		// SQLite uses independent read and write pools. The write pool is
		// single-connection and uses BEGIN IMMEDIATE; every read connection is
		// query-only. DSN pragmas apply to every connection in each pool.
		db.dsn = filepath.Join(cfg.DataDir, dbName)
		db.db, err = sql.Open("sqlite", sqliteDSNWithPragmas(db.dsn)+"&_txlock=immediate")
		if err != nil {
			return nil, fmt.Errorf("failed to open sqlite database: %w", err)
		}
		db.readDB, err = sql.Open("sqlite", sqliteDSNWithPragmas(db.dsn)+"&_pragma=query_only(1)")
		if err != nil {
			_ = db.db.Close()
			return nil, fmt.Errorf("failed to open sqlite read database: %w", err)
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

	// Configure separate write and read pools for SQLite.
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
	if db.readDB != nil {
		if err := verifySQLiteSetup(ctx, db.db, db.readDB, db.dsn); err != nil {
			_ = db.readDB.Close()
			_ = db.db.Close()
			return nil, err
		}
	}

	db.ready = true
	return db, nil
}

// applyPoolSettings keeps SQLite writes on one connection and uses a pooled
// read handle. Remote libSQL uses a single configured pool.
func (db *DB) applyPoolSettings(cfg *Config) {
	maxOpen := max(cfg.MaxOpen, 1)
	idleTime := cfg.IdleTime
	if idleTime <= 0 {
		idleTime = 60
	}
	if db.driver == "sqlite" && db.readDB != nil {
		db.db.SetMaxOpenConns(1)
		db.db.SetMaxIdleConns(1)
		db.db.SetConnMaxLifetime(0)
		db.db.SetConnMaxIdleTime(0)
		db.readDB.SetMaxOpenConns(maxOpen)
		db.readDB.SetMaxIdleConns(min(maxOpen, max(cfg.MaxIdle, 1)))
		db.readDB.SetConnMaxLifetime(time.Duration(cfg.Lifetime) * time.Second)
		db.readDB.SetConnMaxIdleTime(time.Duration(idleTime) * time.Second)
		return
	}
	db.db.SetMaxOpenConns(maxOpen)
	db.db.SetMaxIdleConns(min(maxOpen, max(cfg.MaxIdle, 1)))
	db.db.SetConnMaxLifetime(time.Duration(cfg.Lifetime) * time.Second)
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
	separator := "?"
	if strings.Contains(path, "?") {
		separator = "&"
	}
	return path + separator + "_pragma=foreign_keys(ON)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=busy_timeout(10000)"
}

func verifySQLiteSetup(ctx context.Context, writeDB, readDB *sql.DB, path string) error {
	if err := readDB.PingContext(ctx); err != nil {
		return fmt.Errorf("failed to ping read database: %w", err)
	}
	var journalMode string
	if err := writeDB.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&journalMode); err != nil {
		return fmt.Errorf("failed to verify sqlite journal mode for %s: %w", path, err)
	}
	if strings.Contains(path, "mode=memory") {
		return nil
	}
	if !strings.EqualFold(journalMode, "wal") {
		return fmt.Errorf("sqlite database %s requires WAL journal mode (got %q)", path, journalMode)
	}
	return nil
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

// SetBusyObserver installs an optional observer for delayed SQLite writes.
func (dm *DatabaseManager) SetBusyObserver(observer func(time.Duration, string)) {
	dm.mu.Lock()
	defer dm.mu.Unlock()
	if dm.serverDB != nil {
		dm.serverDB.SetBusyObserver(observer)
	}
	if dm.usersDB != nil {
		dm.usersDB.SetBusyObserver(observer)
	}
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
		// Keep writes on one immediate-transaction connection and reads in a
		// separate query-only pool.
		db.dsn = cfg.DSN
		if db.dsn == "" || db.dsn == ":memory:" {
			db.dsn = fmt.Sprintf("file:search-%d?mode=memory&cache=shared", time.Now().UnixNano())
		}
		db.db, err = sql.Open("sqlite", sqliteDSNWithPragmas(db.dsn)+"&_txlock=immediate")
		if err == nil {
			db.readDB, err = sql.Open("sqlite", sqliteDSNWithPragmas(db.dsn)+"&_pragma=query_only(1)")
		}
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

	// Configure independent SQLite write and read pools.
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
	if db.readDB != nil {
		if err := verifySQLiteSetup(ctx, db.db, db.readDB, db.dsn); err != nil {
			_ = db.readDB.Close()
			_ = db.db.Close()
			return err
		}
	}

	db.ready = true
	return nil
}

// Close closes the database connection
func (db *DB) Close() error {
	db.mu.Lock()
	defer db.mu.Unlock()

	db.ready = false
	var errs []error
	if db.driver == "sqlite" && db.db != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_, err := db.db.ExecContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)")
		cancel()
		if err != nil && !strings.Contains(strings.ToLower(err.Error()), "no such pragma") {
			errs = append(errs, fmt.Errorf("checkpoint sqlite WAL: %w", err))
		}
	}
	if db.readDB != nil {
		if err := db.readDB.Close(); err != nil {
			errs = append(errs, err)
		}
		db.readDB = nil
	}
	if db.db != nil {
		if err := db.db.Close(); err != nil {
			errs = append(errs, err)
		}
		db.db = nil
	}
	return errors.Join(errs...)
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
	if !db.ready {
		db.mu.RUnlock()
		return nil, fmt.Errorf("database not ready")
	}
	if db.driver != "sqlite" {
		defer db.mu.RUnlock()
		return db.db.ExecContext(ctx, query, args...)
	}
	db.mu.RUnlock()
	var result sql.Result
	err := db.Write(ctx, func(tx *sql.Tx) error {
		var execErr error
		result, execErr = tx.ExecContext(ctx, query, args...)
		return execErr
	})
	return result, err
}

// Query executes a query that returns rows
func (db *DB) Query(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	if !db.ready {
		return nil, fmt.Errorf("database not ready")
	}

	readDB := db.readDB
	if readDB == nil {
		readDB = db.db
	}
	return readDB.QueryContext(ctx, query, args...)
}

// QueryRow executes a query that returns a single row
func (db *DB) QueryRow(ctx context.Context, query string, args ...interface{}) *sql.Row {
	db.mu.RLock()
	defer db.mu.RUnlock()

	readDB := db.readDB
	if readDB == nil {
		readDB = db.db
	}
	return readDB.QueryRowContext(ctx, query, args...)
}

// SetBusyObserver configures the callback invoked when a write waited over one second.
func (db *DB) SetBusyObserver(observer func(time.Duration, string)) {
	db.mu.Lock()
	defer db.mu.Unlock()
	db.busyObserver = observer
}

// Write executes a short write transaction with bounded busy retries.
func (db *DB) Write(ctx context.Context, fn func(*sql.Tx) error) error {
	if fn == nil {
		return errors.New("write callback is required")
	}
	var busyWait time.Duration
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	var err error
	for attempt := 0; attempt <= 5; attempt++ {
		attemptStart := time.Now()
		err = db.writeOnce(ctx, fn)
		if IsSQLiteBusy(err) {
			busyWait += time.Since(attemptStart)
		}
		if !IsSQLiteBusy(err) || attempt == 5 {
			break
		}
		wait := time.Duration(25<<attempt)*time.Millisecond + time.Duration(rand.Intn(25))*time.Millisecond
		waitStart := time.Now()
		select {
		case <-time.After(wait):
		case <-ctx.Done():
			err = ctx.Err()
			attempt = 5
		}
		busyWait += time.Since(waitStart)
	}
	if busyWait > time.Second {
		caller := "unknown"
		if pc, _, _, ok := runtime.Caller(1); ok {
			if fn := runtime.FuncForPC(pc); fn != nil {
				caller = fn.Name()
			}
		}
		slog.Warn("db.busy", "wait_ms", busyWait.Milliseconds(), "caller", caller)
		db.mu.RLock()
		observer := db.busyObserver
		db.mu.RUnlock()
		if observer != nil {
			observer(busyWait, caller)
		}
	}
	return err
}

func (db *DB) writeOnce(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			if rollbackErr := tx.Rollback(); rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
				slog.Warn("failed to roll back database transaction", "error", rollbackErr)
			}
		}
	}()
	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	committed = true
	return nil
}

// IsSQLiteBusy reports busy or locked errors, including extended SQLite codes.
func IsSQLiteBusy(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToUpper(err.Error())
	return strings.Contains(message, "SQLITE_BUSY") || strings.Contains(message, "SQLITE_LOCKED") || strings.Contains(message, "DATABASE IS LOCKED")
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

// ReadSQL returns the pooled, query-only SQLite handle for integrations that
// still consume database/sql directly.
func (db *DB) ReadSQL() *sql.DB {
	db.mu.RLock()
	defer db.mu.RUnlock()
	if db.readDB != nil {
		return db.readDB
	}
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
