package database

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
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

func TestQueryHelpersRunAndCloseBoundedResults(t *testing.T) {
	db := newInMemoryDB(t)
	if _, err := db.Exec(context.Background(), "CREATE TABLE timeout_probe (value INTEGER)"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(context.Background(), "INSERT INTO timeout_probe (value) VALUES (?)", 42); err != nil {
		t.Fatal(err)
	}

	rows, err := db.Query(context.Background(), "SELECT value FROM timeout_probe")
	if err != nil {
		t.Fatal(err)
	}
	if !rows.Next() {
		t.Fatalf("timed query returned no row: %v", rows.Err())
	}
	var value int
	if err := rows.Scan(&value); err != nil {
		t.Fatal(err)
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	if value != 42 {
		t.Fatalf("timed query value = %d, want 42", value)
	}

	var rowValue int
	if err := db.QueryRow(context.Background(), "SELECT value FROM timeout_probe").Scan(&rowValue); err != nil {
		t.Fatal(err)
	}
	if rowValue != value {
		t.Fatalf("timed row value = %d, want %d", rowValue, value)
	}
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
	// DefaultConfig().DataDir is /data/db/sqlite which likely doesn't exist and
	// os.MkdirAll may fail in a sandboxed environment. We only assert that
	// the function does not panic and returns a consistent error or a valid
	// manager (depending on whether /data/db/sqlite is writable).
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

func TestDBSnapshotToIncludesCommittedSQLiteData(t *testing.T) {
	dm := newManagerTempDir(t)
	db := dm.ServerDB()
	ctx := context.Background()
	if _, err := db.Exec(ctx, "CREATE TABLE snapshot_test (value TEXT NOT NULL)"); err != nil {
		t.Fatalf("create snapshot test table: %v", err)
	}
	if _, err := db.Exec(ctx, "INSERT INTO snapshot_test (value) VALUES (?)", "committed"); err != nil {
		t.Fatalf("insert snapshot test row: %v", err)
	}

	destination := filepath.Join(t.TempDir(), "server.db")
	if err := db.SnapshotTo(ctx, destination); err != nil {
		t.Fatalf("SnapshotTo: %v", err)
	}

	snapshot, err := sql.Open("sqlite", destination)
	if err != nil {
		t.Fatalf("open snapshot: %v", err)
	}
	defer snapshot.Close()
	var value string
	if err := snapshot.QueryRowContext(ctx, "SELECT value FROM snapshot_test").Scan(&value); err != nil {
		t.Fatalf("read snapshot row: %v", err)
	}
	if value != "committed" {
		t.Fatalf("snapshot value = %q, want committed", value)
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

func TestDatabaseManager_ProbeReadAndWriteHandles(t *testing.T) {
	dm := newManagerTempDir(t)
	if err := dm.Probe(context.Background()); err != nil {
		t.Fatalf("Probe() error = %v", err)
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
	if db.Read(context.Background()) == nil {
		t.Error("Read() returned nil")
	}
}

func TestNewDB_NilConfigUsesDefault(t *testing.T) {
	// nil config → DefaultConfig() which uses /data/db/sqlite — just confirm no panic.
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

func TestDB_Write_WhenNotReady(t *testing.T) {
	db := &DB{driver: "sqlite", ready: false}
	err := db.Write(context.Background(), func(*sql.Tx) error { return nil })
	if err == nil {
		t.Error("Write() should fail when not ready")
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

	// Transactions are managed by Write.
	if err := db.Write(ctx, func(*sql.Tx) error { return nil }); err != nil {
		t.Fatalf("Write: %v", err)
	}

	// Ping
	if err := db.Ping(ctx); err != nil {
		t.Errorf("Ping: %v", err)
	}
}

// --- InitSchema (schema) ---

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

	columns, err := tableColumns(ctx, dm.ServerDB(), "scheduler_tasks")
	if err != nil {
		t.Fatalf("inspect scheduler_tasks columns: %v", err)
	}
	for _, column := range []string{"id", "name"} {
		if _, ok := columns[column]; !ok {
			t.Errorf("fresh scheduler_tasks schema is missing canonical column %q", column)
		}
	}
	for _, legacy := range []string{"task_id", "task_name"} {
		if _, ok := columns[legacy]; ok {
			t.Errorf("fresh scheduler_tasks schema contains legacy column %q", legacy)
		}
	}
	historyColumns, err := tableColumns(ctx, dm.ServerDB(), "scheduler_history")
	if err != nil {
		t.Fatalf("inspect scheduler_history columns: %v", err)
	}
	if !strings.Contains(strings.ToUpper(historyColumns["id"]), "INT") {
		t.Errorf("fresh scheduler_history id type = %q, want integer primary key", historyColumns["id"])
	}
	for _, column := range []string{"task_id", "started_at", "finished_at", "status", "error", "duration_ms"} {
		if _, ok := historyColumns[column]; !ok {
			t.Errorf("fresh scheduler_history schema is missing canonical column %q", column)
		}
	}
	if _, ok := historyColumns["success"]; ok {
		t.Error("fresh scheduler_history schema contains legacy success column")
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

func TestInitSchemaRenamesLegacySchedulerTaskColumnsInPlace(t *testing.T) {
	db := newInMemoryDB(t)
	ctx := context.Background()
	if _, err := db.ExecWithTimeout(ctx, TimeoutMigration, `CREATE TABLE scheduler_tasks (
		task_id TEXT PRIMARY KEY, task_name TEXT, schedule TEXT, last_run DATETIME,
		last_status TEXT, last_error TEXT, next_run DATETIME, run_count INTEGER DEFAULT 0,
		fail_count INTEGER DEFAULT 0, enabled INTEGER DEFAULT 1, retry_count INTEGER DEFAULT 0,
		next_retry DATETIME, locked_by TEXT, locked_at DATETIME)`); err != nil {
		t.Fatalf("create legacy scheduler_tasks schema: %v", err)
	}
	if _, err := db.ExecWithTimeout(ctx, TimeoutMigration, `INSERT INTO scheduler_tasks(task_id, task_name, schedule) VALUES ('backup_daily', 'Daily Backup', '0 2 * * *')`); err != nil {
		t.Fatalf("insert legacy scheduler task: %v", err)
	}
	if err := initServerSchema(ctx, db); err != nil {
		t.Fatalf("migrate legacy scheduler schema: %v", err)
	}
	columns, err := tableColumns(ctx, db, "scheduler_tasks")
	if err != nil {
		t.Fatalf("inspect migrated scheduler_tasks columns: %v", err)
	}
	for _, column := range []string{"id", "name"} {
		if _, ok := columns[column]; !ok {
			t.Errorf("migrated scheduler_tasks schema is missing canonical column %q", column)
		}
	}
	for _, legacy := range []string{"task_id", "task_name"} {
		if _, ok := columns[legacy]; ok {
			t.Errorf("migrated scheduler_tasks schema retains legacy column %q", legacy)
		}
	}
	var id, name string
	if err := db.QueryRow(ctx, "SELECT id, name FROM scheduler_tasks").Scan(&id, &name); err != nil {
		t.Fatalf("read migrated task row: %v", err)
	}
	if id != "backup_daily" || name != "Daily Backup" {
		t.Errorf("migrated task = (%q, %q), want (backup_daily, Daily Backup)", id, name)
	}
	if err := initServerSchema(ctx, db); err != nil {
		t.Fatalf("repeat legacy scheduler migration: %v", err)
	}
}

func TestInitSchemaRenamesLegacySchedulerHistoryColumnsInPlace(t *testing.T) {
	db := newInMemoryDB(t)
	ctx := context.Background()
	if _, err := db.ExecWithTimeout(ctx, TimeoutMigration, `CREATE TABLE scheduler_history (
		id TEXT PRIMARY KEY, task_id TEXT NOT NULL, ran_at DATETIME NOT NULL,
		finished_at DATETIME, success INTEGER NOT NULL DEFAULT 0, error TEXT)`); err != nil {
		t.Fatalf("create legacy scheduler_history schema: %v", err)
	}
	if _, err := db.ExecWithTimeout(ctx, TimeoutMigration, `INSERT INTO scheduler_history(id, task_id, ran_at, finished_at, success, error)
		VALUES ('legacy-row-id', 'backup_daily', '2026-01-02 03:04:05', '2026-01-02 03:04:06', 1, '')`); err != nil {
		t.Fatalf("insert legacy scheduler history: %v", err)
	}
	if err := initServerSchema(ctx, db); err != nil {
		t.Fatalf("migrate legacy scheduler history: %v", err)
	}
	columns, err := tableColumns(ctx, db, "scheduler_history")
	if err != nil {
		t.Fatalf("inspect migrated scheduler_history columns: %v", err)
	}
	for _, column := range []string{"id", "task_id", "started_at", "finished_at", "status", "error", "duration_ms"} {
		if _, ok := columns[column]; !ok {
			t.Errorf("migrated scheduler_history schema is missing canonical column %q", column)
		}
	}
	if _, ok := columns["ran_at"]; ok {
		t.Error("migrated scheduler_history schema retains ran_at")
	}
	var id, status, started string
	var duration int64
	if err := db.QueryRow(ctx, "SELECT id, status, started_at, duration_ms FROM scheduler_history").Scan(&id, &status, &started, &duration); err != nil {
		t.Fatalf("read migrated scheduler history: %v", err)
	}
	if id != "legacy-row-id" || status != "success" || started != "2026-01-02T03:04:05Z" || duration < 900 || duration > 1100 {
		t.Errorf("migrated history = (%q, %q, %q, %d ms), want preserved id/start, success, and about 1000 ms", id, status, started, duration)
	}
	if err := initServerSchema(ctx, db); err != nil {
		t.Fatalf("repeat legacy scheduler history migration: %v", err)
	}
}

func TestInitSchemaMigratesLegacyOwnerTokenIDsWithoutLosingReferences(t *testing.T) {
	db := newInMemoryDB(t)
	ctx := context.Background()
	for _, statement := range []string{
		`CREATE TABLE owners (id TEXT PRIMARY KEY, created_at INTEGER NOT NULL, last_seen_at INTEGER NOT NULL)`,
		`CREATE TABLE api_tokens (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL, token_hash TEXT UNIQUE NOT NULL, token_prefix TEXT NOT NULL, active INTEGER DEFAULT 1, expires_at INTEGER, created_at INTEGER, owner_id TEXT, kind TEXT NOT NULL, token_enc TEXT NOT NULL, rotated_at INTEGER, last_used_at INTEGER, revoked_at INTEGER, revoked_reason TEXT)`,
		`CREATE TABLE token_references (id INTEGER PRIMARY KEY, token_id INTEGER NOT NULL REFERENCES api_tokens(id))`,
		`INSERT INTO owners(id,created_at,last_seen_at) VALUES ('owner-1',1,1)`,
		`INSERT INTO api_tokens(id,name,token_hash,token_prefix,active,expires_at,created_at,owner_id,kind,token_enc) VALUES (7,'legacy','hash-1','own_12345678',1,NULL,10,'owner-1','linked','ciphertext-1')`,
		`INSERT INTO token_references(id,token_id) VALUES (1,7)`,
	} {
		if _, err := db.Exec(ctx, statement); err != nil {
			t.Fatalf("prepare legacy token schema: %v; statement=%s", err, statement)
		}
	}
	if err := initServerSchema(ctx, db); err != nil {
		t.Fatalf("migrate legacy token schema: %v", err)
	}
	var legacyID int64
	var publicID, tokenHash, tokenPrefix, tokenLast4, encrypted string
	if err := db.QueryRow(ctx, `SELECT legacy_id,id,token_hash,token_prefix,token_last4,token_enc FROM api_tokens WHERE legacy_id=7`).Scan(&legacyID, &publicID, &tokenHash, &tokenPrefix, &tokenLast4, &encrypted); err != nil {
		t.Fatalf("read migrated token: %v", err)
	}
	parsed, err := uuid.Parse(publicID)
	if err != nil || parsed.Version() != 7 {
		t.Fatalf("public token id = %q, want UUIDv7 (parse error %v)", publicID, err)
	}
	if legacyID != 7 || tokenHash != "hash-1" || tokenPrefix != "own_12345678" || tokenLast4 != "" || encrypted != "ciphertext-1" {
		t.Fatalf("migrated token lost legacy data: legacy_id=%d hash=%q prefix=%q last4=%q ciphertext=%q", legacyID, tokenHash, tokenPrefix, tokenLast4, encrypted)
	}
	var referencedID int64
	if err := db.QueryRow(ctx, `SELECT t.legacy_id FROM token_references r JOIN api_tokens t ON t.legacy_id=r.token_id WHERE r.id=1`).Scan(&referencedID); err != nil || referencedID != 7 {
		t.Fatalf("legacy foreign-key reference = %d, error=%v", referencedID, err)
	}
	if err := initServerSchema(ctx, db); err != nil {
		t.Fatalf("repeat owner-token schema migration: %v", err)
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

func TestCleanupExpiredAPITokensRetainsExpiredOwnerTokens(t *testing.T) {
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
	if removed != 0 {
		t.Fatalf("removed = %d, want 0", removed)
	}
	var remaining int
	if err := db.QueryRow(ctx, `SELECT COUNT(*) FROM api_tokens`).Scan(&remaining); err != nil {
		t.Fatal(err)
	}
	if remaining != 3 {
		t.Fatalf("remaining = %d, want 3", remaining)
	}
}
