package database

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

// --- helpers ---

// newInMemoryDB opens a raw in-memory SQLite connection and marks it ready.
// The returned *DB can be passed to any function that needs a *DB without
// going through NewDatabaseManager's file-creation path.
func newInMemoryDB(t *testing.T) *DB {
	t.Helper()
	raw, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open :memory: %v", err)
	}
	raw.SetMaxOpenConns(1)
	raw.SetMaxIdleConns(1)
	ctx := context.Background()
	if err := raw.PingContext(ctx); err != nil {
		t.Fatalf("ping :memory: %v", err)
	}
	t.Cleanup(func() { raw.Close() })
	return &DB{db: raw, driver: "sqlite", dsn: ":memory:", ready: true}
}

// newManagerTempDir returns a DatabaseManager backed by two on-disk SQLite
// files in a fresh t.TempDir(). Cleanup is registered automatically.
func newManagerTempDir(t *testing.T) *DatabaseManager {
	t.Helper()
	dir := t.TempDir()
	cfg := &Config{
		Driver:   "sqlite",
		DataDir:  dir,
		MaxOpen:  5,
		MaxIdle:  2,
		Lifetime: 60,
	}
	dm, err := NewDatabaseManager(cfg)
	if err != nil {
		t.Fatalf("NewDatabaseManager: %v", err)
	}
	t.Cleanup(func() { dm.Close() })
	return dm
}

// --- normalizeDriver ---

func TestNormalizeDriver(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"sqlite", "sqlite"},
		{"sqlite2", "sqlite"},
		{"sqlite3", "sqlite"},
		{"SQLITE", "sqlite"},
		{"  sqlite  ", "sqlite"},
		{"libsql", "libsql"},
		{"turso", "libsql"},
		{"LIBSQL", "libsql"},
		{"TURSO", "libsql"},
		{"postgres", "postgres"},
		{"mysql", "mysql"},
		{"unknown", "unknown"},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := normalizeDriver(tt.input)
			if got != tt.want {
				t.Errorf("normalizeDriver(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// --- DefaultConfig ---

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg == nil {
		t.Fatal("DefaultConfig() returned nil")
	}
	if cfg.Driver != "sqlite" {
		t.Errorf("Driver = %q, want sqlite", cfg.Driver)
	}
	if cfg.DataDir == "" {
		t.Error("DataDir must not be empty")
	}
	if cfg.MaxOpen <= 0 {
		t.Errorf("MaxOpen = %d, want > 0", cfg.MaxOpen)
	}
	if cfg.MaxIdle <= 0 {
		t.Errorf("MaxIdle = %d, want > 0", cfg.MaxIdle)
	}
	if cfg.Lifetime <= 0 {
		t.Errorf("Lifetime = %d, want > 0", cfg.Lifetime)
	}
}

// --- NewDatabaseManager ---

func TestNewDatabaseManager_NilConfigUsesDefaults(t *testing.T) {
	// Pass nil — the function must substitute DefaultConfig().
	// DefaultConfig().DataDir is /data/db which likely doesn't exist and
	// os.MkdirAll may fail in a sandboxed environment. We only assert that
	// the function does not panic and returns a consistent error or a valid
	// manager (depending on whether /data/db is writable).
	dm, err := NewDatabaseManager(nil)
	if err == nil {
		defer dm.Close()
		if !dm.IsReady() {
			t.Error("manager should be ready when created without error")
		}
	}
	// Either outcome (success or failure) is acceptable; what matters is no panic.
}

func TestNewDatabaseManager_SQLite_Success(t *testing.T) {
	dm := newManagerTempDir(t)

	if !dm.IsReady() {
		t.Error("IsReady() = false, want true")
	}
	if dm.ServerDB() == nil {
		t.Error("ServerDB() returned nil")
	}
	if dm.UsersDB() == nil {
		t.Error("UsersDB() returned nil")
	}
}

func TestNewDatabaseManager_UnwritableDir(t *testing.T) {
	// Point DataDir to a path inside a file (not a directory) to force MkdirAll failure.
	f, err := os.CreateTemp(t.TempDir(), "notadir")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	cfg := &Config{
		Driver:  "sqlite",
		DataDir: filepath.Join(f.Name(), "subdir"),
	}
	_, err = NewDatabaseManager(cfg)
	if err == nil {
		t.Error("expected error when DataDir cannot be created")
	}
}

func TestNewDatabaseManager_LibSQLMissingDSN(t *testing.T) {
	cfg := &Config{
		Driver:  "libsql",
		DataDir: t.TempDir(),
	}
	_, err := NewDatabaseManager(cfg)
	if err == nil {
		t.Error("expected error for libsql with no DSN")
	}
}

func TestNewDatabaseManager_UnsupportedDriver(t *testing.T) {
	cfg := &Config{
		Driver:  "baddriver",
		DataDir: t.TempDir(),
	}
	_, err := NewDatabaseManager(cfg)
	if err == nil {
		t.Error("expected error for unsupported driver")
	}
}

// --- DatabaseManager accessors & lifecycle ---

func TestDatabaseManager_Close(t *testing.T) {
	dm := newManagerTempDir(t)
	if err := dm.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}
}

func TestDatabaseManager_IsReady_AfterClose(t *testing.T) {
	dm := newManagerTempDir(t)
	dm.Close()
	// After Close the underlying connections are gone; IsReady must not panic
	// and the result is implementation-defined (we just confirm no panic).
	_ = dm.IsReady()
}

func TestDatabaseManager_Ping(t *testing.T) {
	dm := newManagerTempDir(t)
	ctx := context.Background()
	if err := dm.Ping(ctx); err != nil {
		t.Errorf("Ping() error = %v", err)
	}
}

func TestDatabaseManager_Ping_CancelledContext(t *testing.T) {
	dm := newManagerTempDir(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	// A cancelled context may or may not error depending on driver timing.
	// What matters is: no panic.
	_ = dm.Ping(ctx)
}

// --- DB (single-connection) ---

func TestNewDB_SQLiteInMemory(t *testing.T) {
	cfg := &Config{
		Driver:   "sqlite",
		DSN:      ":memory:",
		Lifetime: 60,
	}
	db, err := NewDB(cfg)
	if err != nil {
		t.Fatalf("NewDB() error = %v", err)
	}
	defer db.Close()

	if !db.IsReady() {
		t.Error("IsReady() = false")
	}
	if db.Driver() != "sqlite" {
		t.Errorf("Driver() = %q, want sqlite", db.Driver())
	}
	if db.IsRemote() {
		t.Error("IsRemote() = true for sqlite, want false")
	}
	if db.SQL() == nil {
		t.Error("SQL() returned nil")
	}
}

func TestNewDB_NilConfigUsesDefault(t *testing.T) {
	// nil config → DefaultConfig() which uses /data/db — just confirm no panic.
	db, err := NewDB(nil)
	if err == nil {
		defer db.Close()
	}
}

func TestNewDB_UnsupportedDriver(t *testing.T) {
	cfg := &Config{Driver: "baddriver", DSN: "x", Lifetime: 60}
	_, err := NewDB(cfg)
	if err == nil {
		t.Error("expected error for unsupported driver")
	}
}

func TestDB_Close_Idempotent(t *testing.T) {
	db := newInMemoryDB(t)
	if err := db.Close(); err != nil {
		t.Errorf("first Close() = %v", err)
	}
	// Second close on a nil db.db must not panic and returns nil.
	if err := db.Close(); err != nil {
		t.Errorf("second Close() = %v", err)
	}
}

func TestDB_IsReady(t *testing.T) {
	db := newInMemoryDB(t)
	if !db.IsReady() {
		t.Error("IsReady() = false before close")
	}
	db.Close()
	if db.IsReady() {
		t.Error("IsReady() = true after close")
	}
}

func TestDB_IsRemote_SQLite(t *testing.T) {
	db := newInMemoryDB(t)
	if db.IsRemote() {
		t.Error("IsRemote() = true for sqlite driver")
	}
}

func TestDB_IsRemote_LibSQL(t *testing.T) {
	db := &DB{driver: "libsql", ready: true}
	if !db.IsRemote() {
		t.Error("IsRemote() = false for libsql driver")
	}
}

func TestDB_IsRemote_EmptyDriver(t *testing.T) {
	db := &DB{driver: "", ready: false}
	if db.IsRemote() {
		t.Error("IsRemote() = true for empty driver")
	}
}

func TestDB_Exec_WhenNotReady(t *testing.T) {
	db := &DB{driver: "sqlite", ready: false}
	_, err := db.Exec(context.Background(), "SELECT 1")
	if err == nil {
		t.Error("Exec() should fail when not ready")
	}
}

func TestDB_Query_WhenNotReady(t *testing.T) {
	db := &DB{driver: "sqlite", ready: false}
	_, err := db.Query(context.Background(), "SELECT 1")
	if err == nil {
		t.Error("Query() should fail when not ready")
	}
}

func TestDB_Begin_WhenNotReady(t *testing.T) {
	db := &DB{driver: "sqlite", ready: false}
	_, err := db.Begin(context.Background())
	if err == nil {
		t.Error("Begin() should fail when not ready")
	}
}

func TestDB_Ping_WhenNotReady(t *testing.T) {
	db := &DB{driver: "sqlite", ready: false}
	if err := db.Ping(context.Background()); err == nil {
		t.Error("Ping() should fail when not ready")
	}
}

func TestDB_Exec_Query_Begin_Ping_Ready(t *testing.T) {
	db := newInMemoryDB(t)
	ctx := context.Background()

	// Exec
	if _, err := db.Exec(ctx, "CREATE TABLE t (id INTEGER PRIMARY KEY)"); err != nil {
		t.Fatalf("Exec CREATE: %v", err)
	}
	if _, err := db.Exec(ctx, "INSERT INTO t VALUES (1)"); err != nil {
		t.Fatalf("Exec INSERT: %v", err)
	}

	// Query
	rows, err := db.Query(ctx, "SELECT id FROM t")
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	var count int
	for rows.Next() {
		count++
	}
	rows.Close()
	if count != 1 {
		t.Errorf("Query returned %d rows, want 1", count)
	}

	// QueryRow
	var id int
	if err := db.QueryRow(ctx, "SELECT id FROM t WHERE id = ?", 1).Scan(&id); err != nil {
		t.Fatalf("QueryRow: %v", err)
	}
	if id != 1 {
		t.Errorf("QueryRow id = %d, want 1", id)
	}

	// Begin / commit
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	// Ping
	if err := db.Ping(ctx); err != nil {
		t.Errorf("Ping: %v", err)
	}
}

// --- InitSchema (migrations) ---

func TestInitSchema_CreatesAllTables(t *testing.T) {
	dm := newManagerTempDir(t)
	ctx := context.Background()

	if err := InitSchema(ctx, dm); err != nil {
		t.Fatalf("InitSchema() error = %v", err)
	}

	// Verify a representative set of tables exist
	expectedTables := []string{
		"scheduler_tasks",
		"scheduler_history",
		"audit_log",
		"config",
		"config_meta",
		"rate_limits",
		"backups",
		"api_tokens",
		"search_stats",
		"engine_stats",
		"blocked_ips",
		"custom_bangs",
		"search_alerts",
		"search_alert_results",
	}
	for _, table := range expectedTables {
		t.Run("table_"+table, func(t *testing.T) {
			var name string
			err := dm.ServerDB().QueryRow(ctx,
				"SELECT name FROM sqlite_master WHERE type='table' AND name=?", table,
			).Scan(&name)
			if err != nil {
				t.Errorf("table %q missing: %v", table, err)
			}
		})
	}
}

func TestInitSchema_Idempotent(t *testing.T) {
	dm := newManagerTempDir(t)
	ctx := context.Background()

	if err := InitSchema(ctx, dm); err != nil {
		t.Fatalf("first InitSchema: %v", err)
	}
	// Running again must not error (IF NOT EXISTS guards)
	if err := InitSchema(ctx, dm); err != nil {
		t.Fatalf("second InitSchema: %v", err)
	}
}

// --- RemoteDBConfig ---

// --- GetSupportedDrivers / IsRemoteDriver ---

// --- MigrationManager (source = in-memory SQLite, no real remote) ---

// --- convertSchema ---

// --- MixedModeManager ---

// --- MixedDB methods ---

// --- RemoteDB direct construction (no network) ---

// --- connectDatabase error path for libsql (no network) ---

func TestConnectDatabase_LibSQL_EmptyDSN(t *testing.T) {
	dir := t.TempDir()
	dm := &DatabaseManager{dataDir: dir}
	cfg := &Config{Driver: "libsql", DSN: ""}
	_, err := dm.connectDatabase(cfg, "test.db")
	if err == nil {
		t.Error("expected error for libsql with empty DSN")
	}
}

// --- MigrationProgress struct fields ---

// --- MixedDB.Close idempotent ---

func TestCleanupExpiredAPITokens(t *testing.T) {
	db := newInMemoryDB(t)
	ctx := context.Background()
	if _, err := db.Exec(ctx, `CREATE TABLE api_tokens (id INTEGER PRIMARY KEY, expires_at DATETIME)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO api_tokens (expires_at) VALUES (datetime('now', '-1 day')), (datetime('now', '+1 day')), (NULL)`); err != nil {
		t.Fatal(err)
	}

	removed, err := db.CleanupExpiredAPITokens(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if removed != 1 {
		t.Fatalf("removed = %d, want 1", removed)
	}
	var remaining int
	if err := db.QueryRow(ctx, `SELECT COUNT(*) FROM api_tokens`).Scan(&remaining); err != nil {
		t.Fatal(err)
	}
	if remaining != 2 {
		t.Fatalf("remaining = %d, want 2", remaining)
	}
}
