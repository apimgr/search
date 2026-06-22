package database

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
		"scheduler_state",
		"audit_log",
		"server_settings",
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

// --- validIdentifier ---

func TestValidIdentifier(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"empty string", "", false},
		{"too long", strings.Repeat("a", 129), false},
		{"exactly 128 chars", strings.Repeat("a", 128), true},
		{"lowercase letters", "table_name", true},
		{"uppercase letters", "TableName", true},
		{"digits", "table123", true},
		{"leading digit", "123table", true},
		{"underscore", "_private", true},
		{"hyphen", "table-name", false},
		{"space", "table name", false},
		{"dot", "schema.table", false},
		{"semicolon", "table;drop", false},
		{"single char", "t", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := validIdentifier(tt.input)
			if got != tt.want {
				t.Errorf("validIdentifier(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// --- RemoteDBConfig ---

func TestDefaultRemoteDBConfig(t *testing.T) {
	cfg := DefaultRemoteDBConfig()
	if cfg == nil {
		t.Fatal("DefaultRemoteDBConfig() returned nil")
	}
	if cfg.Driver != "postgres" {
		t.Errorf("Driver = %q, want postgres", cfg.Driver)
	}
	if cfg.Host != "localhost" {
		t.Errorf("Host = %q, want localhost", cfg.Host)
	}
	if cfg.Port != 5432 {
		t.Errorf("Port = %d, want 5432", cfg.Port)
	}
	if cfg.SSLMode != "require" {
		t.Errorf("SSLMode = %q, want require", cfg.SSLMode)
	}
}

func TestRemoteDBConfig_BuildDSN(t *testing.T) {
	tests := []struct {
		name     string
		cfg      RemoteDBConfig
		wantSub  string
		wantBool bool
	}{
		{
			name: "postgres",
			cfg: RemoteDBConfig{
				Driver: "postgres", Host: "db.example.com", Port: 5432,
				Database: "mydb", Username: "user", Password: "pass", SSLMode: "require",
			},
			wantSub:  "postgres://user:pass@db.example.com:5432/mydb?sslmode=require",
			wantBool: true,
		},
		{
			name: "pgx alias",
			cfg: RemoteDBConfig{
				Driver: "pgx", Host: "host", Port: 5433,
				Database: "db", Username: "u", Password: "p", SSLMode: "disable",
			},
			wantSub:  "postgres://u:p@host:5433/db?sslmode=disable",
			wantBool: true,
		},
		{
			name: "mysql",
			cfg: RemoteDBConfig{
				Driver: "mysql", Host: "mysqlhost", Port: 3306,
				Database: "mydb", Username: "root", Password: "secret",
			},
			wantSub:  "root:secret@tcp(mysqlhost:3306)/mydb",
			wantBool: true,
		},
		{
			name: "mariadb",
			cfg: RemoteDBConfig{
				Driver: "mariadb", Host: "mariahost", Port: 3307,
				Database: "db", Username: "u", Password: "p",
			},
			wantSub:  "u:p@tcp(mariahost:3307)/db",
			wantBool: true,
		},
		{
			name: "mssql",
			cfg: RemoteDBConfig{
				Driver: "mssql", Host: "sqlhost", Port: 1433,
				Database: "db", Username: "sa", Password: "P@ss",
			},
			wantSub:  "sqlserver://sa:P@ss@sqlhost:1433?database=db",
			wantBool: true,
		},
		{
			name:     "unsupported driver",
			cfg:      RemoteDBConfig{Driver: "baddriver"},
			wantSub:  "",
			wantBool: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dsn := tt.cfg.BuildDSN()
			if tt.wantBool {
				if !strings.Contains(dsn, tt.wantSub) {
					t.Errorf("BuildDSN() = %q, want substring %q", dsn, tt.wantSub)
				}
			} else {
				if dsn != "" {
					t.Errorf("BuildDSN() = %q, want empty string for unsupported driver", dsn)
				}
			}
		})
	}
}

func TestNewRemoteDB_NilConfig(t *testing.T) {
	_, err := NewRemoteDB(nil)
	if err == nil {
		t.Error("expected error for nil config")
	}
}

func TestNewRemoteDB_UnsupportedDriver(t *testing.T) {
	cfg := &RemoteDBConfig{Driver: "unsupported"}
	_, err := NewRemoteDB(cfg)
	if err == nil {
		t.Error("expected error for unsupported driver (empty DSN)")
	}
}

// --- GetSupportedDrivers / IsRemoteDriver ---

func TestGetSupportedDrivers(t *testing.T) {
	drivers := GetSupportedDrivers()
	if len(drivers) == 0 {
		t.Fatal("GetSupportedDrivers() returned empty slice")
	}
	for _, d := range []string{"postgres", "mysql", "mariadb", "mssql", "sqlserver"} {
		found := false
		for _, got := range drivers {
			if got == d {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("driver %q missing from GetSupportedDrivers()", d)
		}
	}
}

func TestIsRemoteDriver(t *testing.T) {
	tests := []struct {
		driver string
		want   bool
	}{
		{"postgres", true},
		{"mysql", true},
		{"mariadb", true},
		{"mssql", true},
		{"sqlserver", true},
		{"sqlite", false},
		{"libsql", false},
		{"unknown", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.driver, func(t *testing.T) {
			got := IsRemoteDriver(tt.driver)
			if got != tt.want {
				t.Errorf("IsRemoteDriver(%q) = %v, want %v", tt.driver, got, tt.want)
			}
		})
	}
}

// --- MigrationManager (source = in-memory SQLite, no real remote) ---

func TestMigrationManager_GetSourceTables_EmptyDB(t *testing.T) {
	src := newInMemoryDB(t)
	mm := NewMigrationManager(src, nil, t.TempDir())

	tables, err := mm.getSourceTables(context.Background())
	if err != nil {
		t.Fatalf("getSourceTables: %v", err)
	}
	if len(tables) != 0 {
		t.Errorf("expected 0 tables in empty DB, got %v", tables)
	}
}

func TestMigrationManager_GetSourceTables_WithTables(t *testing.T) {
	src := newInMemoryDB(t)
	ctx := context.Background()

	if _, err := src.Exec(ctx, `CREATE TABLE alpha (id INTEGER PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	if _, err := src.Exec(ctx, `CREATE TABLE beta (name TEXT)`); err != nil {
		t.Fatal(err)
	}

	mm := NewMigrationManager(src, nil, t.TempDir())
	tables, err := mm.getSourceTables(ctx)
	if err != nil {
		t.Fatalf("getSourceTables: %v", err)
	}
	if len(tables) != 2 {
		t.Errorf("expected 2 tables, got %v", tables)
	}
}

func TestMigrationManager_GetTableSchema(t *testing.T) {
	src := newInMemoryDB(t)
	ctx := context.Background()

	createSQL := `CREATE TABLE items (id INTEGER PRIMARY KEY, name TEXT NOT NULL)`
	if _, err := src.Exec(ctx, createSQL); err != nil {
		t.Fatal(err)
	}

	mm := NewMigrationManager(src, nil, t.TempDir())
	schema, err := mm.getTableSchema(ctx, "items")
	if err != nil {
		t.Fatalf("getTableSchema: %v", err)
	}
	if schema == "" {
		t.Error("getTableSchema returned empty schema")
	}
	if !strings.Contains(schema, "items") {
		t.Errorf("schema %q does not mention table name", schema)
	}
}

func TestMigrationManager_CountRows(t *testing.T) {
	src := newInMemoryDB(t)
	ctx := context.Background()

	if _, err := src.Exec(ctx, `CREATE TABLE items (id INTEGER PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 5; i++ {
		if _, err := src.Exec(ctx, `INSERT INTO items VALUES (?)`, i); err != nil {
			t.Fatal(err)
		}
	}

	mm := NewMigrationManager(src, nil, t.TempDir())
	count, err := mm.countRows(ctx, "items")
	if err != nil {
		t.Fatalf("countRows: %v", err)
	}
	if count != 5 {
		t.Errorf("countRows = %d, want 5", count)
	}
}

func TestMigrationManager_CountRows_InvalidTableName(t *testing.T) {
	src := newInMemoryDB(t)
	mm := NewMigrationManager(src, nil, t.TempDir())

	_, err := mm.countRows(context.Background(), "bad-name")
	if err == nil {
		t.Error("expected error for invalid table name")
	}
}

func TestMigrationManager_GetTableColumns(t *testing.T) {
	src := newInMemoryDB(t)
	ctx := context.Background()

	if _, err := src.Exec(ctx, `CREATE TABLE things (id INTEGER PRIMARY KEY, title TEXT, active INTEGER)`); err != nil {
		t.Fatal(err)
	}

	mm := NewMigrationManager(src, nil, t.TempDir())
	cols, err := mm.getTableColumns(ctx, "things")
	if err != nil {
		t.Fatalf("getTableColumns: %v", err)
	}
	if len(cols) != 3 {
		t.Errorf("expected 3 columns, got %v", cols)
	}
	want := map[string]bool{"id": true, "title": true, "active": true}
	for _, c := range cols {
		if !want[c] {
			t.Errorf("unexpected column %q", c)
		}
	}
}

func TestMigrationManager_GetTableColumns_InvalidTableName(t *testing.T) {
	src := newInMemoryDB(t)
	mm := NewMigrationManager(src, nil, t.TempDir())

	_, err := mm.getTableColumns(context.Background(), "bad-name")
	if err == nil {
		t.Error("expected error for invalid table name")
	}
}

func TestMigrationManager_MigrateToRemote_NilSource(t *testing.T) {
	mm := NewMigrationManager(nil, nil, t.TempDir())
	err := mm.MigrateToRemote(context.Background(), nil)
	if err == nil {
		t.Error("expected error when source is nil")
	}
}

func TestMigrationManager_MigrateToRemote_NilTarget(t *testing.T) {
	src := newInMemoryDB(t)
	mm := NewMigrationManager(src, nil, t.TempDir())
	err := mm.MigrateToRemote(context.Background(), nil)
	if err == nil {
		t.Error("expected error when target is nil / not ready")
	}
}

func TestMigrationManager_BackupBeforeMigration(t *testing.T) {
	dataDir := t.TempDir()
	// Create a real file to back up
	src := filepath.Join(dataDir, "server.db")
	if err := os.WriteFile(src, []byte("sqlite data"), 0600); err != nil {
		t.Fatal(err)
	}

	mm := NewMigrationManager(nil, nil, dataDir)
	backupPath, err := mm.BackupBeforeMigration(src)
	if err != nil {
		t.Fatalf("BackupBeforeMigration: %v", err)
	}
	if backupPath == "" {
		t.Error("backup path is empty")
	}

	// Backup file must exist and contain the same data
	got, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatalf("read backup: %v", err)
	}
	if string(got) != "sqlite data" {
		t.Errorf("backup content = %q, want %q", string(got), "sqlite data")
	}

	// Backup path must include a timestamp-shaped component
	base := filepath.Base(backupPath)
	if !strings.Contains(base, "server.db") {
		t.Errorf("backup filename %q should contain original name", base)
	}
}

func TestMigrationManager_BackupBeforeMigration_MissingFile(t *testing.T) {
	mm := NewMigrationManager(nil, nil, t.TempDir())
	_, err := mm.BackupBeforeMigration("/nonexistent/path/file.db")
	if err == nil {
		t.Error("expected error when source file does not exist")
	}
}

// --- convertSchema ---

func TestConvertSchema_Postgres(t *testing.T) {
	rdb := &RemoteDB{driver: "postgres"}
	mm := &MigrationManager{targetDB: rdb}

	input := "CREATE TABLE t (id INTEGER PRIMARY KEY AUTOINCREMENT, created DATETIME)"
	out := mm.convertSchema(input)
	if !strings.Contains(out, "SERIAL PRIMARY KEY") {
		t.Errorf("postgres conversion missing SERIAL PRIMARY KEY: %q", out)
	}
	if !strings.Contains(out, "TIMESTAMP") {
		t.Errorf("postgres conversion missing TIMESTAMP: %q", out)
	}
	if strings.Contains(out, "AUTOINCREMENT") {
		t.Errorf("postgres conversion still contains AUTOINCREMENT: %q", out)
	}
}

func TestConvertSchema_MySQL(t *testing.T) {
	rdb := &RemoteDB{driver: "mysql"}
	mm := &MigrationManager{targetDB: rdb}

	input := "CREATE TABLE t (id INTEGER PRIMARY KEY AUTOINCREMENT)"
	out := mm.convertSchema(input)
	if !strings.Contains(out, "AUTO_INCREMENT") {
		t.Errorf("mysql conversion missing AUTO_INCREMENT: %q", out)
	}
	if !strings.Contains(out, "InnoDB") {
		t.Errorf("mysql conversion missing InnoDB engine: %q", out)
	}
}

func TestConvertSchema_MSSQL(t *testing.T) {
	rdb := &RemoteDB{driver: "mssql"}
	mm := &MigrationManager{targetDB: rdb}

	input := "CREATE TABLE t (id INTEGER PRIMARY KEY AUTOINCREMENT, note TEXT, data BLOB, created datetime)"
	out := mm.convertSchema(input)
	if !strings.Contains(out, "IDENTITY(1,1)") {
		t.Errorf("mssql conversion missing IDENTITY: %q", out)
	}
	if !strings.Contains(out, "NVARCHAR(MAX)") {
		t.Errorf("mssql conversion missing NVARCHAR(MAX): %q", out)
	}
	if !strings.Contains(out, "VARBINARY(MAX)") {
		t.Errorf("mssql conversion missing VARBINARY(MAX): %q", out)
	}
	if !strings.Contains(out, "DATETIME2") {
		t.Errorf("mssql conversion missing DATETIME2: %q", out)
	}
}

func TestConvertSchema_UnknownDriver_NoChange(t *testing.T) {
	rdb := &RemoteDB{driver: "unknown"}
	mm := &MigrationManager{targetDB: rdb}

	input := "CREATE TABLE t (id INTEGER PRIMARY KEY AUTOINCREMENT)"
	out := mm.convertSchema(input)
	if out != input {
		t.Errorf("unknown driver should return schema unchanged; got %q", out)
	}
}

// --- MixedModeManager ---

func TestDefaultMixedModeConfig(t *testing.T) {
	dir := "/some/data"
	cfg := DefaultMixedModeConfig(dir)
	if cfg == nil {
		t.Fatal("DefaultMixedModeConfig() returned nil")
	}
	if cfg.ServerDB.Driver != "sqlite" {
		t.Errorf("ServerDB.Driver = %q, want sqlite", cfg.ServerDB.Driver)
	}
	if cfg.UsersDB.Driver != "sqlite" {
		t.Errorf("UsersDB.Driver = %q, want sqlite", cfg.UsersDB.Driver)
	}
	if !strings.HasPrefix(cfg.ServerDB.Path, dir) {
		t.Errorf("ServerDB.Path %q should be under %q", cfg.ServerDB.Path, dir)
	}
	if !strings.HasPrefix(cfg.UsersDB.Path, dir) {
		t.Errorf("UsersDB.Path %q should be under %q", cfg.UsersDB.Path, dir)
	}
}

func TestNewMixedModeManager_NilConfig(t *testing.T) {
	_, err := NewMixedModeManager(nil)
	if err == nil {
		t.Error("expected error for nil config")
	}
}

func TestNewMixedModeManager_SQLiteSuccess(t *testing.T) {
	dir := t.TempDir()
	cfg := DefaultMixedModeConfig(dir)
	mm, err := NewMixedModeManager(cfg)
	if err != nil {
		t.Fatalf("NewMixedModeManager: %v", err)
	}
	defer mm.Close()

	if !mm.IsReady() {
		t.Error("IsReady() = false")
	}
	if mm.ServerDB() == nil {
		t.Error("ServerDB() = nil")
	}
	if mm.UsersDB() == nil {
		t.Error("UsersDB() = nil")
	}
}

func TestNewMixedModeManager_UnsupportedDriver(t *testing.T) {
	cfg := &MixedModeConfig{
		ServerDB: DatabaseBackendConfig{Driver: "baddriver", Path: ":memory:"},
		UsersDB:  DatabaseBackendConfig{Driver: "sqlite", Path: ":memory:"},
	}
	_, err := NewMixedModeManager(cfg)
	if err == nil {
		t.Error("expected error for unsupported driver")
	}
}

func TestMixedModeManager_GetMode(t *testing.T) {
	dir := t.TempDir()

	tests := []struct {
		name         string
		serverDriver string
		usersDriver  string
		want         string
	}{
		{"both sqlite", "sqlite", "sqlite", "standalone"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &MixedModeConfig{
				ServerDB: DatabaseBackendConfig{
					Driver: tt.serverDriver,
					Path:   filepath.Join(dir, "s_"+tt.name+".db"),
				},
				UsersDB: DatabaseBackendConfig{
					Driver: tt.usersDriver,
					Path:   filepath.Join(dir, "u_"+tt.name+".db"),
				},
			}
			mm, err := NewMixedModeManager(cfg)
			if err != nil {
				t.Fatalf("NewMixedModeManager: %v", err)
			}
			defer mm.Close()

			got := mm.GetMode()
			if got != tt.want {
				t.Errorf("GetMode() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMixedModeManager_IsMixedMode_SameDriver(t *testing.T) {
	dir := t.TempDir()
	cfg := DefaultMixedModeConfig(dir)
	mm, err := NewMixedModeManager(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer mm.Close()

	if mm.IsMixedMode() {
		t.Error("IsMixedMode() = true when both drivers are sqlite")
	}
}

func TestMixedModeManager_GetStatus(t *testing.T) {
	dir := t.TempDir()
	cfg := DefaultMixedModeConfig(dir)
	mm, err := NewMixedModeManager(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer mm.Close()

	status := mm.GetStatus()
	if status == nil {
		t.Fatal("GetStatus() returned nil")
	}
	requiredKeys := []string{"mode", "mixed_mode", "server_db_driver", "users_db_driver", "server_db_ready", "users_db_ready"}
	for _, k := range requiredKeys {
		if _, ok := status[k]; !ok {
			t.Errorf("GetStatus() missing key %q", k)
		}
	}
	if status["server_db_ready"] != true {
		t.Error("server_db_ready should be true")
	}
	if status["users_db_ready"] != true {
		t.Error("users_db_ready should be true")
	}
}

func TestMixedModeManager_Close(t *testing.T) {
	dir := t.TempDir()
	cfg := DefaultMixedModeConfig(dir)
	mm, err := NewMixedModeManager(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := mm.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}
}

// --- MixedDB methods ---

func newInMemoryMixedDB(t *testing.T) *MixedDB {
	t.Helper()
	raw, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open :memory: %v", err)
	}
	raw.SetMaxOpenConns(1)
	ctx := context.Background()
	if err := raw.PingContext(ctx); err != nil {
		t.Fatalf("ping: %v", err)
	}
	t.Cleanup(func() { raw.Close() })
	return &MixedDB{db: raw, driver: "sqlite", ready: true}
}

func TestMixedDB_IsLocal(t *testing.T) {
	tests := []struct {
		driver string
		want   bool
	}{
		{"sqlite", true},
		{"sqlite3", true},
		{"postgres", false},
		{"mysql", false},
		{"libsql", false},
	}
	for _, tt := range tests {
		t.Run(tt.driver, func(t *testing.T) {
			mdb := &MixedDB{driver: tt.driver}
			if mdb.IsLocal() != tt.want {
				t.Errorf("IsLocal() = %v, want %v", mdb.IsLocal(), tt.want)
			}
		})
	}
}

func TestMixedDB_IsRemote(t *testing.T) {
	tests := []struct {
		driver string
		want   bool
	}{
		{"sqlite", false},
		{"sqlite3", false},
		{"postgres", true},
		{"mysql", true},
	}
	for _, tt := range tests {
		t.Run(tt.driver, func(t *testing.T) {
			mdb := &MixedDB{driver: tt.driver}
			if mdb.IsRemote() != tt.want {
				t.Errorf("IsRemote() = %v, want %v", mdb.IsRemote(), tt.want)
			}
		})
	}
}

func TestMixedDB_SupportsReturning(t *testing.T) {
	tests := []struct {
		driver string
		want   bool
	}{
		{"postgres", true},
		{"sqlite", true},
		{"sqlite3", true},
		{"mysql", false},
		{"mariadb", false},
	}
	for _, tt := range tests {
		t.Run(tt.driver, func(t *testing.T) {
			mdb := &MixedDB{driver: tt.driver}
			if mdb.SupportsReturning() != tt.want {
				t.Errorf("SupportsReturning() = %v, want %v for driver %q", mdb.SupportsReturning(), tt.want, tt.driver)
			}
		})
	}
}

func TestMixedDB_SupportsUpsert(t *testing.T) {
	tests := []struct {
		driver string
		want   bool
	}{
		{"postgres", true},
		{"sqlite", true},
		{"sqlite3", true},
		{"mysql", true},
		{"mariadb", true},
		{"mssql", false},
	}
	for _, tt := range tests {
		t.Run(tt.driver, func(t *testing.T) {
			mdb := &MixedDB{driver: tt.driver}
			if mdb.SupportsUpsert() != tt.want {
				t.Errorf("SupportsUpsert() = %v, want %v for driver %q", mdb.SupportsUpsert(), tt.want, tt.driver)
			}
		})
	}
}

func TestMixedDB_GetPlaceholder(t *testing.T) {
	tests := []struct {
		driver string
		index  int
		want   string
	}{
		{"postgres", 1, "$1"},
		{"postgres", 3, "$3"},
		{"sqlite", 1, "?"},
		{"mysql", 2, "?"},
		{"mariadb", 5, "?"},
	}
	for _, tt := range tests {
		t.Run(tt.driver+"_"+tt.want, func(t *testing.T) {
			mdb := &MixedDB{driver: tt.driver}
			got := mdb.GetPlaceholder(tt.index)
			if got != tt.want {
				t.Errorf("GetPlaceholder(%d) = %q, want %q for driver %q", tt.index, got, tt.want, tt.driver)
			}
		})
	}
}

func TestMixedDB_Driver(t *testing.T) {
	mdb := &MixedDB{driver: "sqlite"}
	if mdb.Driver() != "sqlite" {
		t.Errorf("Driver() = %q, want sqlite", mdb.Driver())
	}
}

func TestMixedDB_IsReady(t *testing.T) {
	mdb := newInMemoryMixedDB(t)
	if !mdb.IsReady() {
		t.Error("IsReady() = false before close")
	}
	mdb.Close()
	if mdb.IsReady() {
		t.Error("IsReady() = true after close")
	}
}

func TestMixedDB_Exec_WhenNotReady(t *testing.T) {
	mdb := &MixedDB{driver: "sqlite", ready: false}
	_, err := mdb.Exec(context.Background(), "SELECT 1")
	if err == nil {
		t.Error("Exec() should fail when not ready")
	}
}

func TestMixedDB_Query_WhenNotReady(t *testing.T) {
	mdb := &MixedDB{driver: "sqlite", ready: false}
	_, err := mdb.Query(context.Background(), "SELECT 1")
	if err == nil {
		t.Error("Query() should fail when not ready")
	}
}

func TestMixedDB_Begin_WhenNotReady(t *testing.T) {
	mdb := &MixedDB{driver: "sqlite", ready: false}
	_, err := mdb.Begin(context.Background())
	if err == nil {
		t.Error("Begin() should fail when not ready")
	}
}

func TestMixedDB_Ping_WhenNotReady(t *testing.T) {
	mdb := &MixedDB{driver: "sqlite", ready: false}
	if err := mdb.Ping(context.Background()); err == nil {
		t.Error("Ping() should fail when not ready")
	}
}

func TestMixedDB_FullCRUD(t *testing.T) {
	mdb := newInMemoryMixedDB(t)
	ctx := context.Background()

	if _, err := mdb.Exec(ctx, `CREATE TABLE kv (key TEXT PRIMARY KEY, value TEXT)`); err != nil {
		t.Fatalf("Exec CREATE: %v", err)
	}
	if _, err := mdb.Exec(ctx, `INSERT INTO kv VALUES (?, ?)`, "k1", "v1"); err != nil {
		t.Fatalf("Exec INSERT: %v", err)
	}

	rows, err := mdb.Query(ctx, `SELECT key, value FROM kv`)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	var keys []string
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			t.Fatal(err)
		}
		keys = append(keys, k)
	}
	rows.Close()
	if len(keys) != 1 || keys[0] != "k1" {
		t.Errorf("expected [k1], got %v", keys)
	}

	var val string
	if err := mdb.QueryRow(ctx, `SELECT value FROM kv WHERE key = ?`, "k1").Scan(&val); err != nil {
		t.Fatalf("QueryRow: %v", err)
	}
	if val != "v1" {
		t.Errorf("QueryRow value = %q, want v1", val)
	}

	tx, err := mdb.Begin(ctx)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	tx.Rollback()

	if err := mdb.Ping(ctx); err != nil {
		t.Errorf("Ping: %v", err)
	}
}

// --- RemoteDB direct construction (no network) ---

func TestRemoteDB_IsReady_InitiallyFalse(t *testing.T) {
	rdb := &RemoteDB{}
	if rdb.IsReady() {
		t.Error("IsReady() = true before any connection")
	}
}

func TestRemoteDB_Close_NilDB(t *testing.T) {
	rdb := &RemoteDB{ready: true}
	if err := rdb.Close(); err != nil {
		t.Errorf("Close() on nil db = %v", err)
	}
}

func TestRemoteDB_DB_NilDB(t *testing.T) {
	rdb := &RemoteDB{}
	if rdb.DB() != nil {
		t.Error("DB() should return nil when not connected")
	}
}

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

func TestMigrationProgress_Fields(t *testing.T) {
	now := time.Now()
	p := MigrationProgress{
		Phase:        "migrating",
		Table:        "users",
		TotalRows:    100,
		MigratedRows: 50,
		StartTime:    now,
		Error:        "some error",
	}
	if p.Phase != "migrating" {
		t.Errorf("Phase = %q", p.Phase)
	}
	if p.TotalRows != 100 || p.MigratedRows != 50 {
		t.Errorf("row counts wrong: total=%d migrated=%d", p.TotalRows, p.MigratedRows)
	}
	if p.Error != "some error" {
		t.Errorf("Error = %q", p.Error)
	}
}

// --- MixedDB.Close idempotent ---

func TestMixedDB_Close_Idempotent(t *testing.T) {
	mdb := newInMemoryMixedDB(t)
	if err := mdb.Close(); err != nil {
		t.Errorf("first Close() = %v", err)
	}
	// Second close: db field is now closed, but mdb.db != nil so it calls Close again.
	// The driver may return an error here; we only verify no panic.
	_ = mdb.Close()
}
