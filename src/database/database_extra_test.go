package database

import (
	"context"
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

// --- GetMode branches ---

// --- InitSchema error path ---

// TestInitSchema_ClosedDB verifies InitSchema returns an error when the underlying DB
// connection is not ready (simulated by constructing a DatabaseManager with a closed db).
func TestInitSchema_ClosedDB(t *testing.T) {
	raw, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	raw.SetMaxOpenConns(1)
	if err := raw.Ping(); err != nil {
		t.Fatalf("ping: %v", err)
	}
	raw.Close()

	// Build a DatabaseManager whose serverDB is backed by the now-closed *sql.DB.
	closedDB := &DB{db: raw, driver: "sqlite", ready: false}
	dm := &DatabaseManager{
		serverDB: closedDB,
		usersDB:  closedDB,
	}

	ctx := context.Background()
	if err := InitSchema(ctx, dm); err == nil {
		t.Error("InitSchema() with closed/not-ready DB should return error, got nil")
	}
}

// TestInitSchema_WithReadyDB verifies InitSchema succeeds on a fresh DatabaseManager.
func TestInitSchema_WithReadyDB(t *testing.T) {
	dm := newManagerTempDir(t)
	ctx := context.Background()

	if err := InitSchema(ctx, dm); err != nil {
		t.Fatalf("InitSchema() error = %v", err)
	}

	// Verify at least one expected table was created.
	var name string
	err := dm.serverDB.db.QueryRowContext(ctx, `SELECT name FROM sqlite_master WHERE type='table' AND name='audit_log'`).Scan(&name)
	if err != nil {
		t.Errorf("audit_log table not found after InitSchema: %v", err)
	}
}

// --- MigrateToRemote using two in-memory SQLite databases ---

// --- createTargetTable ---

// --- migrateTableData ---

// --- ValidateRemoteConnection error paths ---

// --- convertSchema ---

// --- GetSupportedDrivers / IsRemoteDriver ---

// --- BuildDSN ---

// --- BackupBeforeMigration ---

// --- NewDB connect ---

// TestNewDB_SQLiteInMemoryExtra verifies NewDB works with in-memory SQLite DSN.
func TestNewDB_SQLiteInMemoryExtra(t *testing.T) {
	cfg := &Config{
		Driver:   "sqlite",
		DSN:      ":memory:",
		MaxOpen:  1,
		MaxIdle:  1,
		Lifetime: 60,
	}
	db, err := NewDB(cfg)
	if err != nil {
		t.Fatalf("NewDB() error = %v", err)
	}
	defer db.Close()

	if !db.IsReady() {
		t.Error("NewDB() IsReady() = false, want true")
	}
	if db.Driver() != "sqlite" {
		t.Errorf("Driver() = %q, want sqlite", db.Driver())
	}
}

// TestNewDB_UnsupportedDriverExtra verifies NewDB returns an error for unknown drivers.
func TestNewDB_UnsupportedDriverExtra(t *testing.T) {
	cfg := &Config{
		Driver:   "nosuchdb",
		DSN:      "dsn",
		Lifetime: 60,
	}
	_, err := NewDB(cfg)
	if err == nil {
		t.Error("NewDB() with unsupported driver should error")
	}
}

// TestNewDB_LibSQLBadDSN verifies that libsql uses a lazy connection — sql.Open and
// PingContext both succeed even for an unreachable DSN because the driver defers the
// actual network connection until the first real query. NewDB therefore returns no error.
func TestNewDB_LibSQLBadDSN(t *testing.T) {
	cfg := &Config{
		Driver:   "libsql",
		DSN:      "libsql://nonexistent.invalid/db",
		Lifetime: 60,
	}
	db, err := NewDB(cfg)
	if err != nil {
		t.Skipf("libsql open unexpectedly failed (driver behaviour may have changed): %v", err)
	}
	if db == nil {
		t.Error("NewDB() returned nil db with no error")
	}
}

// TestDB_SQL returns the underlying *sql.DB.
func TestDB_SQL(t *testing.T) {
	db := newInMemoryDB(t)
	raw := db.SQL()
	if raw == nil {
		t.Error("SQL() returned nil")
	}
}

// --- MixedDB.GetPlaceholder ---

// --- DB.IsRemote ---

// TestDBIsRemote verifies IsRemote is true for non-sqlite drivers.
func TestDBIsRemote(t *testing.T) {
	tests := []struct {
		driver string
		want   bool
	}{
		{"sqlite", false},
		{"libsql", true},
		{"pgx", true},
		{"mysql", true},
	}
	for _, tt := range tests {
		t.Run(tt.driver, func(t *testing.T) {
			db := &DB{driver: tt.driver}
			got := db.IsRemote()
			if got != tt.want {
				t.Errorf("IsRemote() for driver %q = %v, want %v", tt.driver, got, tt.want)
			}
		})
	}
}

// --- MixedModeManager nil config ---

// --- MixedModeManager GetStatus ---

// --- getSourceTables ---
