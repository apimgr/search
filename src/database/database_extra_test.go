package database

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"

	_ "modernc.org/sqlite"
)

// newInMemoryRawSQLite opens a raw in-memory SQLite connection.
// Caller owns Close().
func newInMemoryRawSQLite(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open :memory: %v", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if err := db.Ping(); err != nil {
		t.Fatalf("ping :memory: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// --- GetMode branches ---

// TestGetMode_StandaloneMode tests GetMode returns "standalone" when both drivers are "sqlite".
func TestGetMode_StandaloneMode(t *testing.T) {
	mm, err := NewMixedModeManager(DefaultMixedModeConfig(t.TempDir()))
	if err != nil {
		t.Fatalf("NewMixedModeManager: %v", err)
	}
	defer mm.Close()

	got := mm.GetMode()
	if got != "standalone" {
		t.Errorf("GetMode() = %q, want standalone", got)
	}
}

// TestGetMode_RemoteMode tests GetMode returns "remote" when both drivers are the same non-sqlite.
// We construct the manager directly since no real remote DB is available.
func TestGetMode_RemoteMode(t *testing.T) {
	raw := newInMemoryRawSQLite(t)

	// Build two MixedDB instances that look like they use the same "postgres" driver.
	serverDB := &MixedDB{db: raw, driver: "postgres", ready: true}
	usersDB := &MixedDB{db: raw, driver: "postgres", ready: true}

	mm := &MixedModeManager{
		serverDB: serverDB,
		usersDB:  usersDB,
		config: &MixedModeConfig{
			ServerDB: DatabaseBackendConfig{Driver: "postgres"},
			UsersDB:  DatabaseBackendConfig{Driver: "postgres"},
		},
	}

	got := mm.GetMode()
	if got != "remote" {
		t.Errorf("GetMode() = %q, want remote", got)
	}
}

// TestGetMode_MixedMode tests GetMode returns "mixed" when drivers differ.
func TestGetMode_MixedMode(t *testing.T) {
	raw := newInMemoryRawSQLite(t)

	serverDB := &MixedDB{db: raw, driver: "sqlite", ready: true}
	usersDB := &MixedDB{db: raw, driver: "postgres", ready: true}

	mm := &MixedModeManager{
		serverDB: serverDB,
		usersDB:  usersDB,
		config: &MixedModeConfig{
			ServerDB: DatabaseBackendConfig{Driver: "sqlite"},
			UsersDB:  DatabaseBackendConfig{Driver: "postgres"},
		},
	}

	got := mm.GetMode()
	if got != "mixed" {
		t.Errorf("GetMode() = %q, want mixed", got)
	}
}

// TestIsMixedMode_FalseWhenSameDriver verifies IsMixedMode returns false for same driver.
func TestIsMixedMode_FalseWhenSameDriver(t *testing.T) {
	mm, err := NewMixedModeManager(DefaultMixedModeConfig(t.TempDir()))
	if err != nil {
		t.Fatalf("NewMixedModeManager: %v", err)
	}
	defer mm.Close()

	if mm.IsMixedMode() {
		t.Error("IsMixedMode() = true for two sqlite backends, want false")
	}
}

// TestIsMixedMode_TrueWhenDifferentDrivers verifies IsMixedMode returns true when drivers differ.
func TestIsMixedMode_TrueWhenDifferentDrivers(t *testing.T) {
	raw := newInMemoryRawSQLite(t)
	mm := &MixedModeManager{
		config: &MixedModeConfig{
			ServerDB: DatabaseBackendConfig{Driver: "sqlite"},
			UsersDB:  DatabaseBackendConfig{Driver: "postgres"},
		},
		serverDB: &MixedDB{db: raw, driver: "sqlite", ready: true},
		usersDB:  &MixedDB{db: raw, driver: "postgres", ready: true},
	}
	if !mm.IsMixedMode() {
		t.Error("IsMixedMode() = false for different drivers, want true")
	}
}

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

// newRemoteDBFromSQLite constructs a RemoteDB wrapping an in-memory SQLite for testing.
// The "driver" is set to "sqlite" so insertQuery uses "?" placeholders.
func newRemoteDBFromSQLite(t *testing.T, raw *sql.DB) *RemoteDB {
	t.Helper()
	return &RemoteDB{
		db:     raw,
		driver: "sqlite",
		config: &RemoteDBConfig{Driver: "sqlite"},
		ready:  true,
	}
}

// TestMigrateToRemote_EmptySource verifies MigrateToRemote succeeds on an empty source DB.
func TestMigrateToRemote_EmptySource(t *testing.T) {
	source := newInMemoryDB(t)
	targetRaw := newInMemoryRawSQLite(t)
	target := newRemoteDBFromSQLite(t, targetRaw)

	mm := NewMigrationManager(source, target, t.TempDir())
	progress := make(chan MigrationProgress, 10)

	ctx := context.Background()
	if err := mm.MigrateToRemote(ctx, progress); err != nil {
		t.Fatalf("MigrateToRemote() on empty source error = %v", err)
	}
}

// TestMigrateToRemote_WithData migrates a table with rows from source to target.
func TestMigrateToRemote_WithData(t *testing.T) {
	source := newInMemoryDB(t)
	ctx := context.Background()

	// Create a simple table in the source and insert rows.
	_, err := source.db.ExecContext(ctx, `CREATE TABLE test_items (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL)`)
	if err != nil {
		t.Fatalf("create source table: %v", err)
	}
	for i := 0; i < 3; i++ {
		_, err := source.db.ExecContext(ctx, `INSERT INTO test_items (name) VALUES (?)`, fmt.Sprintf("item%d", i))
		if err != nil {
			t.Fatalf("insert row %d: %v", i, err)
		}
	}

	targetRaw := newInMemoryRawSQLite(t)
	target := newRemoteDBFromSQLite(t, targetRaw)

	mm := NewMigrationManager(source, target, t.TempDir())
	progress := make(chan MigrationProgress, 20)

	if err := mm.MigrateToRemote(ctx, progress); err != nil {
		t.Fatalf("MigrateToRemote() error = %v", err)
	}

	// Verify rows landed in target.
	var count int
	if err := targetRaw.QueryRowContext(ctx, `SELECT COUNT(*) FROM test_items`).Scan(&count); err != nil {
		t.Fatalf("count rows in target: %v", err)
	}
	if count != 3 {
		t.Errorf("target has %d rows, want 3", count)
	}
}

// TestMigrateToRemote_NilSource verifies error when source is nil.
func TestMigrateToRemote_NilSource(t *testing.T) {
	targetRaw := newInMemoryRawSQLite(t)
	target := newRemoteDBFromSQLite(t, targetRaw)

	mm := NewMigrationManager(nil, target, t.TempDir())
	if err := mm.MigrateToRemote(context.Background(), nil); err == nil {
		t.Error("MigrateToRemote() with nil source should return error")
	}
}

// TestMigrateToRemote_NilTarget verifies error when target is nil.
func TestMigrateToRemote_NilTarget(t *testing.T) {
	source := newInMemoryDB(t)
	mm := NewMigrationManager(source, nil, t.TempDir())
	if err := mm.MigrateToRemote(context.Background(), nil); err == nil {
		t.Error("MigrateToRemote() with nil target should return error")
	}
}

// --- createTargetTable ---

// TestCreateTargetTable_SQLite creates a table in a SQLite target DB.
func TestCreateTargetTable_SQLite(t *testing.T) {
	source := newInMemoryDB(t)
	targetRaw := newInMemoryRawSQLite(t)
	target := newRemoteDBFromSQLite(t, targetRaw)
	mm := NewMigrationManager(source, target, t.TempDir())

	schema := `CREATE TABLE IF NOT EXISTS test_table (id INTEGER PRIMARY KEY, val TEXT)`
	ctx := context.Background()
	if err := mm.createTargetTable(ctx, "test_table", schema); err != nil {
		t.Fatalf("createTargetTable() error = %v", err)
	}

	// Verify table exists in target.
	var name string
	if err := targetRaw.QueryRowContext(ctx, `SELECT name FROM sqlite_master WHERE type='table' AND name='test_table'`).Scan(&name); err != nil {
		t.Errorf("test_table not found in target after createTargetTable: %v", err)
	}
}

// TestCreateTargetTable_AlreadyExists verifies createTargetTable does not error when table exists.
func TestCreateTargetTable_AlreadyExists(t *testing.T) {
	source := newInMemoryDB(t)
	targetRaw := newInMemoryRawSQLite(t)
	target := newRemoteDBFromSQLite(t, targetRaw)
	mm := NewMigrationManager(source, target, t.TempDir())

	schema := `CREATE TABLE IF NOT EXISTS dup_table (id INTEGER PRIMARY KEY, val TEXT)`
	ctx := context.Background()

	if err := mm.createTargetTable(ctx, "dup_table", schema); err != nil {
		t.Fatalf("first createTargetTable() error = %v", err)
	}
	// Second call: table already exists — should not error.
	if err := mm.createTargetTable(ctx, "dup_table", schema); err != nil {
		t.Errorf("second createTargetTable() (already exists) error = %v", err)
	}
}

// --- migrateTableData ---

// TestMigrateTableData_BasicRows migrates rows and verifies count.
func TestMigrateTableData_BasicRows(t *testing.T) {
	source := newInMemoryDB(t)
	ctx := context.Background()

	_, err := source.db.ExecContext(ctx, `CREATE TABLE items (id INTEGER PRIMARY KEY AUTOINCREMENT, label TEXT)`)
	if err != nil {
		t.Fatalf("create source table: %v", err)
	}
	for i := 0; i < 5; i++ {
		_, err := source.db.ExecContext(ctx, `INSERT INTO items (label) VALUES (?)`, fmt.Sprintf("label%d", i))
		if err != nil {
			t.Fatalf("insert: %v", err)
		}
	}

	targetRaw := newInMemoryRawSQLite(t)
	// Create matching table in target.
	_, err = targetRaw.ExecContext(ctx, `CREATE TABLE items (id INTEGER PRIMARY KEY AUTOINCREMENT, label TEXT)`)
	if err != nil {
		t.Fatalf("create target table: %v", err)
	}

	target := newRemoteDBFromSQLite(t, targetRaw)
	mm := NewMigrationManager(source, target, t.TempDir())

	progress := make(chan MigrationProgress, 20)
	migrated, err := mm.migrateTableData(ctx, "items", progress, 5)
	if err != nil {
		t.Fatalf("migrateTableData() error = %v", err)
	}
	if migrated != 5 {
		t.Errorf("migratedRows = %d, want 5", migrated)
	}
}

// TestMigrateTableData_InvalidTableName verifies the identifier guard rejects bad names.
func TestMigrateTableData_InvalidTableName(t *testing.T) {
	source := newInMemoryDB(t)
	targetRaw := newInMemoryRawSQLite(t)
	target := newRemoteDBFromSQLite(t, targetRaw)
	mm := NewMigrationManager(source, target, t.TempDir())

	_, err := mm.migrateTableData(context.Background(), "bad-name; DROP TABLE x", nil, 0)
	if err == nil {
		t.Error("migrateTableData() with invalid table name should error")
	}
}

// --- ValidateRemoteConnection error paths ---

// TestValidateRemoteConnection_UnsupportedDriver verifies that an unsupported driver errors.
func TestValidateRemoteConnection_UnsupportedDriver(t *testing.T) {
	cfg := &RemoteDBConfig{
		Driver: "nosuchdb",
		Host:   "localhost",
		Port:   9999,
	}
	if err := ValidateRemoteConnection(cfg); err == nil {
		t.Error("ValidateRemoteConnection() with unsupported driver should error")
	}
}

// TestValidateRemoteConnection_BadPostgresAddress verifies that a bad postgres address errors.
func TestValidateRemoteConnection_BadPostgresAddress(t *testing.T) {
	cfg := &RemoteDBConfig{
		Driver:   "postgres",
		Host:     "192.0.2.1",
		Port:     5432,
		Database: "testdb",
		Username: "testuser",
		Password: "testpass",
		SSLMode:  "disable",
	}
	if err := ValidateRemoteConnection(cfg); err == nil {
		t.Error("ValidateRemoteConnection() with unreachable host should error")
	}
}

// --- convertSchema ---

// TestConvertSchema covers all driver branches.
func TestConvertSchema(t *testing.T) {
	tests := []struct {
		name         string
		driver       string
		inputSchema  string
		wantContains string
	}{
		{
			name:         "postgres AUTOINCREMENT to SERIAL",
			driver:       "postgres",
			inputSchema:  "CREATE TABLE t (id INTEGER PRIMARY KEY AUTOINCREMENT, ts DATETIME)",
			wantContains: "SERIAL PRIMARY KEY",
		},
		{
			name:         "mysql AUTOINCREMENT to AUTO_INCREMENT",
			driver:       "mysql",
			inputSchema:  "CREATE TABLE t (id INTEGER PRIMARY KEY AUTOINCREMENT)",
			wantContains: "AUTO_INCREMENT",
		},
		{
			name:         "mssql DATETIME to DATETIME2",
			driver:       "mssql",
			inputSchema:  "CREATE TABLE t (id INTEGER PRIMARY KEY AUTOINCREMENT, ts DATETIME, data TEXT)",
			wantContains: "DATETIME2",
		},
		{
			name:         "sqlite pass-through unchanged",
			driver:       "sqlite",
			inputSchema:  "CREATE TABLE t (id INTEGER PRIMARY KEY)",
			wantContains: "CREATE TABLE",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw := newInMemoryRawSQLite(t)
			target := &RemoteDB{db: raw, driver: tt.driver, ready: true}
			source := newInMemoryDB(t)
			mm := NewMigrationManager(source, target, t.TempDir())

			got := mm.convertSchema(tt.inputSchema)
			if !containsStr(got, tt.wantContains) {
				t.Errorf("convertSchema(%q) for driver %q = %q, want it to contain %q",
					tt.inputSchema, tt.driver, got, tt.wantContains)
			}
		})
	}
}

// containsStr is a simple substring check to avoid importing strings in test code.
func containsStr(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		func() bool {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		}())
}

// --- GetSupportedDrivers / IsRemoteDriver ---

// TestGetSupportedDriversAll verifies all expected drivers are listed.
func TestGetSupportedDriversAll(t *testing.T) {
	drivers := GetSupportedDrivers()
	if len(drivers) == 0 {
		t.Fatal("GetSupportedDrivers() returned empty list")
	}
	for _, expected := range []string{"postgres", "mysql"} {
		found := false
		for _, d := range drivers {
			if d == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("GetSupportedDrivers() missing %q", expected)
		}
	}
}

// TestIsRemoteDriverExtra verifies known drivers are recognized.
func TestIsRemoteDriverExtra(t *testing.T) {
	tests := []struct {
		driver string
		want   bool
	}{
		{"postgres", true},
		{"mysql", true},
		{"sqlite", false},
		{"libsql", false},
		{"nosuchdb", false},
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

// --- BuildDSN ---

// TestBuildDSN covers all driver branches of RemoteDBConfig.BuildDSN.
func TestBuildDSN(t *testing.T) {
	tests := []struct {
		name    string
		cfg     RemoteDBConfig
		wantPfx string
	}{
		{
			name: "postgres",
			cfg: RemoteDBConfig{
				Driver:   "postgres",
				Host:     "db.example.com",
				Port:     5432,
				Database: "mydb",
				Username: "user",
				Password: "pass",
				SSLMode:  "disable",
			},
			wantPfx: "postgres://",
		},
		{
			name: "pgx alias",
			cfg: RemoteDBConfig{
				Driver:   "pgx",
				Host:     "db.example.com",
				Port:     5432,
				Database: "mydb",
				Username: "user",
				Password: "pass",
				SSLMode:  "disable",
			},
			wantPfx: "postgres://",
		},
		{
			name: "mysql",
			cfg: RemoteDBConfig{
				Driver:   "mysql",
				Host:     "db.example.com",
				Port:     3306,
				Database: "mydb",
				Username: "user",
				Password: "pass",
			},
			wantPfx: "user:",
		},
		{
			name: "mssql",
			cfg: RemoteDBConfig{
				Driver:   "mssql",
				Host:     "db.example.com",
				Port:     1433,
				Database: "mydb",
				Username: "user",
				Password: "pass",
			},
			wantPfx: "sqlserver://",
		},
		{
			name:    "unsupported returns empty",
			cfg:     RemoteDBConfig{Driver: "nosuchdb"},
			wantPfx: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.cfg.BuildDSN()
			if tt.wantPfx == "" {
				if got != "" {
					t.Errorf("BuildDSN() = %q, want empty", got)
				}
				return
			}
			if !containsStr(got, tt.wantPfx) {
				t.Errorf("BuildDSN() = %q, want prefix %q", got, tt.wantPfx)
			}
		})
	}
}

// --- BackupBeforeMigration ---

// TestBackupBeforeMigration creates a real file backup and verifies the copy.
func TestBackupBeforeMigration(t *testing.T) {
	dir := t.TempDir()
	dbPath := dir + "/test.db"

	// Write a small file to "backup".
	if err := os.WriteFile(dbPath, []byte("test database content"), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	source := newInMemoryDB(t)
	targetRaw := newInMemoryRawSQLite(t)
	target := newRemoteDBFromSQLite(t, targetRaw)
	mm := NewMigrationManager(source, target, dir)

	backupPath, err := mm.BackupBeforeMigration(dbPath)
	if err != nil {
		t.Fatalf("BackupBeforeMigration() error = %v", err)
	}
	if backupPath == "" {
		t.Error("BackupBeforeMigration() returned empty path")
	}

	// Verify the backup file exists and has the same content.
	got, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatalf("ReadFile backup: %v", err)
	}
	if string(got) != "test database content" {
		t.Errorf("backup content = %q, want original content", got)
	}
}

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

// TestMixedDBGetPlaceholder covers all driver branches.
func TestMixedDBGetPlaceholder(t *testing.T) {
	tests := []struct {
		driver string
		index  int
		want   string
	}{
		{"postgres", 1, "$1"},
		{"postgres", 3, "$3"},
		{"sqlite", 1, "?"},
		{"mysql", 2, "?"},
	}
	for _, tt := range tests {
		t.Run(tt.driver, func(t *testing.T) {
			mdb := &MixedDB{driver: tt.driver}
			got := mdb.GetPlaceholder(tt.index)
			if got != tt.want {
				t.Errorf("GetPlaceholder(%d) for driver %q = %q, want %q", tt.index, tt.driver, got, tt.want)
			}
		})
	}
}

// TestMixedDBSupportsReturning covers SupportsReturning.
func TestMixedDBSupportsReturning(t *testing.T) {
	tests := []struct {
		driver string
		want   bool
	}{
		{"postgres", true},
		{"sqlite", true},
		{"mysql", false},
		{"mssql", false},
	}
	for _, tt := range tests {
		t.Run(tt.driver, func(t *testing.T) {
			mdb := &MixedDB{driver: tt.driver}
			got := mdb.SupportsReturning()
			if got != tt.want {
				t.Errorf("SupportsReturning() for driver %q = %v, want %v", tt.driver, got, tt.want)
			}
		})
	}
}

// TestMixedDBSupportsUpsert covers SupportsUpsert.
func TestMixedDBSupportsUpsert(t *testing.T) {
	tests := []struct {
		driver string
		want   bool
	}{
		{"postgres", true},
		{"sqlite", true},
		{"mysql", true},
		{"mariadb", true},
		{"mssql", false},
	}
	for _, tt := range tests {
		t.Run(tt.driver, func(t *testing.T) {
			mdb := &MixedDB{driver: tt.driver}
			got := mdb.SupportsUpsert()
			if got != tt.want {
				t.Errorf("SupportsUpsert() for driver %q = %v, want %v", tt.driver, got, tt.want)
			}
		})
	}
}

// TestMixedDBIsLocal covers IsLocal and IsRemote.
func TestMixedDBIsLocal(t *testing.T) {
	tests := []struct {
		driver   string
		wantLoc  bool
		wantRem  bool
	}{
		{"sqlite", true, false},
		{"sqlite3", true, false},
		{"postgres", false, true},
		{"mysql", false, true},
	}
	for _, tt := range tests {
		t.Run(tt.driver, func(t *testing.T) {
			mdb := &MixedDB{driver: tt.driver}
			if got := mdb.IsLocal(); got != tt.wantLoc {
				t.Errorf("IsLocal() for %q = %v, want %v", tt.driver, got, tt.wantLoc)
			}
			if got := mdb.IsRemote(); got != tt.wantRem {
				t.Errorf("IsRemote() for %q = %v, want %v", tt.driver, got, tt.wantRem)
			}
		})
	}
}

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

// TestNewMixedModeManager_NilConfigExtra verifies nil config returns error.
func TestNewMixedModeManager_NilConfigExtra(t *testing.T) {
	_, err := NewMixedModeManager(nil)
	if err == nil {
		t.Error("NewMixedModeManager(nil) should return error")
	}
}

// TestNewMixedModeManager_UnsupportedDriverExtra verifies error for unsupported driver in config.
func TestNewMixedModeManager_UnsupportedDriverExtra(t *testing.T) {
	cfg := &MixedModeConfig{
		ServerDB: DatabaseBackendConfig{Driver: "nosuchdb", DSN: "dsn"},
		UsersDB:  DatabaseBackendConfig{Driver: "sqlite", Path: t.TempDir() + "/u.db"},
	}
	_, err := NewMixedModeManager(cfg)
	if err == nil {
		t.Error("NewMixedModeManager() with unsupported driver should return error")
	}
}

// --- MixedModeManager GetStatus ---

// TestMixedModeManager_GetStatusKeys verifies GetStatus returns expected keys.
func TestMixedModeManager_GetStatusKeys(t *testing.T) {
	mm, err := NewMixedModeManager(DefaultMixedModeConfig(t.TempDir()))
	if err != nil {
		t.Fatalf("NewMixedModeManager: %v", err)
	}
	defer mm.Close()

	status := mm.GetStatus()
	for _, key := range []string{"mode", "server_db_driver", "users_db_driver", "server_db_ready", "users_db_ready"} {
		if _, ok := status[key]; !ok {
			t.Errorf("GetStatus() missing key %q", key)
		}
	}
}

// --- getSourceTables ---

// TestGetSourceTables_EmptyDB verifies empty result for a DB with no user tables.
func TestGetSourceTables_EmptyDB(t *testing.T) {
	source := newInMemoryDB(t)
	targetRaw := newInMemoryRawSQLite(t)
	target := newRemoteDBFromSQLite(t, targetRaw)
	mm := NewMigrationManager(source, target, t.TempDir())

	tables, err := mm.getSourceTables(context.Background())
	if err != nil {
		t.Fatalf("getSourceTables() error = %v", err)
	}
	if len(tables) != 0 {
		t.Errorf("getSourceTables() = %v, want empty", tables)
	}
}

// TestGetSourceTables_WithTables verifies tables are returned after creation.
func TestGetSourceTables_WithTables(t *testing.T) {
	source := newInMemoryDB(t)
	ctx := context.Background()
	_, err := source.db.ExecContext(ctx, `CREATE TABLE alpha (id INTEGER PRIMARY KEY)`)
	if err != nil {
		t.Fatalf("create alpha: %v", err)
	}
	_, err = source.db.ExecContext(ctx, `CREATE TABLE beta (id INTEGER PRIMARY KEY)`)
	if err != nil {
		t.Fatalf("create beta: %v", err)
	}

	targetRaw := newInMemoryRawSQLite(t)
	target := newRemoteDBFromSQLite(t, targetRaw)
	mm := NewMigrationManager(source, target, t.TempDir())

	tables, err := mm.getSourceTables(ctx)
	if err != nil {
		t.Fatalf("getSourceTables() error = %v", err)
	}
	if len(tables) != 2 {
		t.Errorf("getSourceTables() = %v, want 2 tables", tables)
	}
}
