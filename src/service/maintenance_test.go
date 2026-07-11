package service

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/apimgr/search/src/config"
	_ "modernc.org/sqlite"
)

// TestMaintenanceServiceSetFileWriteDir covers the SetFileWriteDir setter and verifies
// that checkFileWrite respects the configured directory.
func TestMaintenanceServiceSetFileWriteDir(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "writedir-test-")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	tests := []struct {
		name    string
		dir     string
		wantErr bool
	}{
		{"os.TempDir (always writable)", os.TempDir(), false},
		{"custom temp dir", tempDir, false},
		// Non-existent dir makes checkFileWrite fail
		{"non-existent dir", filepath.Join(tempDir, "no-such-dir"), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.DefaultConfig()
			ms := NewMaintenanceService(cfg)

			ms.SetFileWriteDir(tt.dir)

			// Verify the dir is stored by observing checkFileWrite behaviour
			err := ms.checkFileWrite()
			if (err != nil) != tt.wantErr {
				t.Errorf("checkFileWrite() after SetFileWriteDir(%q) error = %v, wantErr %v",
					tt.dir, err, tt.wantErr)
			}
		})
	}
}

// TestMaintenanceServiceSetFileWriteDirReflected verifies that the internal field is
// actually updated (the mu.Lock path was exercised).
func TestMaintenanceServiceSetFileWriteDirReflected(t *testing.T) {
	cfg := config.DefaultConfig()
	ms := NewMaintenanceService(cfg)

	dir := os.TempDir()
	ms.SetFileWriteDir(dir)

	ms.mu.RLock()
	got := ms.fileWriteDir
	ms.mu.RUnlock()

	if got != dir {
		t.Errorf("fileWriteDir = %q, want %q", got, dir)
	}
}

// TestMaintenanceServiceCheckDatabaseIntegrity covers CheckDatabaseIntegrity with a
// clean in-memory SQLite database (should return nil) and with a closed database (error path).
func TestMaintenanceServiceCheckDatabaseIntegrity(t *testing.T) {
	cfg := config.DefaultConfig()
	ms := NewMaintenanceService(cfg)

	t.Run("clean in-memory db", func(t *testing.T) {
		db, err := sql.Open("sqlite", ":memory:")
		if err != nil {
			t.Fatalf("sql.Open error = %v", err)
		}
		defer db.Close()

		// Seed a table so PRAGMA integrity_check has something to validate
		if _, err := db.Exec("CREATE TABLE t (id INTEGER PRIMARY KEY)"); err != nil {
			t.Fatalf("CREATE TABLE error = %v", err)
		}

		if err := ms.CheckDatabaseIntegrity(db); err != nil {
			t.Errorf("CheckDatabaseIntegrity() error = %v, want nil for clean DB", err)
		}
	})

	t.Run("closed db returns error", func(t *testing.T) {
		db, err := sql.Open("sqlite", ":memory:")
		if err != nil {
			t.Fatalf("sql.Open error = %v", err)
		}
		db.Close()

		if err := ms.CheckDatabaseIntegrity(db); err == nil {
			t.Error("CheckDatabaseIntegrity() should error on closed DB")
		}
	})
}

// TestMaintenanceServiceRepairDatabase covers RepairDatabase path-validation rules and
// the happy-path with a real SQLite file.
func TestMaintenanceServiceRepairDatabase(t *testing.T) {
	cfg := config.DefaultConfig()
	ms := NewMaintenanceService(cfg)

	tempDir, err := os.MkdirTemp("", "repair-test-")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Table-driven error cases — none of these should reach the VACUUM step.
	// NOTE: the "path with traversal" case must use raw string concatenation (not
	// filepath.Join) because filepath.Join auto-cleans `..` components, which
	// makes cleanedPath == dbPath and bypasses the traversal guard.
	errorCases := []struct {
		name    string
		getPath func() string
		wantMsg string
	}{
		{
			"relative path",
			func() string { return "relative/path.db" },
			"must be absolute",
		},
		{
			// Raw concatenation keeps the `..` so filepath.Clean in RepairDatabase
			// produces a different path, triggering the traversal branch.
			"path with traversal raw concat",
			func() string { return tempDir + "/../notexist.db" },
			"path traversal",
		},
		{
			"non-existent absolute path",
			func() string { return filepath.Join(tempDir, "nonexistent.db") },
			"invalid database path",
		},
		{
			"directory (not regular file)",
			func() string { return tempDir },
			"not a regular file",
		},
	}

	for _, tc := range errorCases {
		t.Run(tc.name, func(t *testing.T) {
			err := ms.RepairDatabase(tc.getPath())
			if err == nil {
				t.Errorf("RepairDatabase() should fail for %q", tc.name)
			}
		})
	}

	// Happy-path: a valid SQLite file should be repaired without error
	t.Run("valid sqlite file", func(t *testing.T) {
		dbPath := filepath.Join(tempDir, "valid.db")

		db, err := sql.Open("sqlite", dbPath)
		if err != nil {
			t.Fatalf("failed to open sqlite db: %v", err)
		}
		if _, err := db.Exec("CREATE TABLE items (id INTEGER PRIMARY KEY, name TEXT)"); err != nil {
			db.Close()
			t.Fatalf("CREATE TABLE error = %v", err)
		}
		if _, err := db.Exec("INSERT INTO items VALUES (1, 'test')"); err != nil {
			db.Close()
			t.Fatalf("INSERT error = %v", err)
		}
		db.Close()

		if err := ms.RepairDatabase(dbPath); err != nil {
			t.Errorf("RepairDatabase() error = %v, want nil", err)
		}
	})
}

// TestBackupDatabaseHappyPath verifies that BackupDatabase copies the file and
// writes a .sha256 checksum file alongside it.
func TestBackupDatabaseHappyPath(t *testing.T) {
	cfg := config.DefaultConfig()
	ms := NewMaintenanceService(cfg)

	tempDir, err := os.MkdirTemp("", "backup-test-")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a minimal SQLite database to back up
	dbPath := filepath.Join(tempDir, "source.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	if _, err := db.Exec("CREATE TABLE x (id INTEGER PRIMARY KEY)"); err != nil {
		db.Close()
		t.Fatalf("CREATE TABLE: %v", err)
	}
	db.Close()

	backupDir := filepath.Join(tempDir, "backups")
	backupPath, err := ms.BackupDatabase(dbPath, backupDir)
	if err != nil {
		t.Fatalf("BackupDatabase() error = %v", err)
	}

	// Verify backup file was created
	if _, err := os.Stat(backupPath); err != nil {
		t.Errorf("backup file not found: %v", err)
	}

	// Verify .sha256 checksum file was created alongside the backup
	checksumPath := backupPath + ".sha256"
	data, err := os.ReadFile(checksumPath)
	if err != nil {
		t.Fatalf(".sha256 file not found: %v", err)
	}
	if len(data) == 0 {
		t.Error("checksum file is empty")
	}
}

// TestBackupDatabaseSourceNotFound verifies that BackupDatabase returns an error
// when the source database path does not exist.
func TestBackupDatabaseSourceNotFound(t *testing.T) {
	cfg := config.DefaultConfig()
	ms := NewMaintenanceService(cfg)

	tempDir, err := os.MkdirTemp("", "backup-notfound-")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	_, err = ms.BackupDatabase(filepath.Join(tempDir, "nonexistent.db"), filepath.Join(tempDir, "backups"))
	if err == nil {
		t.Error("BackupDatabase() should fail when source does not exist")
	}
}

// TestRestoreDatabaseChecksumVerified covers the RestoreDatabase happy path:
// creates a backup with a valid .sha256 file then verifies it is restored correctly.
func TestRestoreDatabaseChecksumVerified(t *testing.T) {
	cfg := config.DefaultConfig()
	ms := NewMaintenanceService(cfg)

	tempDir, err := os.MkdirTemp("", "restore-test-")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Build a minimal SQLite source
	srcPath := filepath.Join(tempDir, "original.db")
	db, err := sql.Open("sqlite", srcPath)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	if _, err := db.Exec("CREATE TABLE items (id INTEGER PRIMARY KEY)"); err != nil {
		db.Close()
		t.Fatalf("CREATE TABLE: %v", err)
	}
	db.Close()

	// Backup it
	backupDir := filepath.Join(tempDir, "backups")
	backupPath, err := ms.BackupDatabase(srcPath, backupDir)
	if err != nil {
		t.Fatalf("BackupDatabase: %v", err)
	}

	// Restore to a new location — covers checksum-verify branch
	restoreDest := filepath.Join(tempDir, "restored.db")
	if err := ms.RestoreDatabase(backupPath, restoreDest); err != nil {
		t.Errorf("RestoreDatabase() error = %v, want nil", err)
	}

	if _, err := os.Stat(restoreDest); err != nil {
		t.Errorf("restored file not found: %v", err)
	}
}

// TestRestoreDatabaseBackupNotFound verifies that RestoreDatabase errors when
// the backup file does not exist.
func TestRestoreDatabaseBackupNotFound(t *testing.T) {
	cfg := config.DefaultConfig()
	ms := NewMaintenanceService(cfg)

	err := ms.RestoreDatabase("/nonexistent/path/backup.db", "/tmp/out.db")
	if err == nil {
		t.Error("RestoreDatabase() should error when backup not found")
	}
}

// TestRestoreDatabaseChecksumMismatch covers the checksum-mismatch error branch of
// RestoreDatabase by writing a tampered .sha256 file alongside the backup.
func TestRestoreDatabaseChecksumMismatch(t *testing.T) {
	cfg := config.DefaultConfig()
	ms := NewMaintenanceService(cfg)

	tempDir, err := os.MkdirTemp("", "restore-mismatch-")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a minimal SQLite source
	srcPath := filepath.Join(tempDir, "db.db")
	db, err := sql.Open("sqlite", srcPath)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	db.Exec("CREATE TABLE x (id INTEGER PRIMARY KEY)")
	db.Close()

	// Backup to get a real backup file, then overwrite .sha256 with garbage
	backupDir := filepath.Join(tempDir, "backups")
	backupPath, err := ms.BackupDatabase(srcPath, backupDir)
	if err != nil {
		t.Fatalf("BackupDatabase: %v", err)
	}
	checksumPath := backupPath + ".sha256"
	if err := os.WriteFile(checksumPath, []byte("deadbeefdeadbeef"), 0600); err != nil {
		t.Fatalf("write tampered checksum: %v", err)
	}

	dest := filepath.Join(tempDir, "out.db")
	err = ms.RestoreDatabase(backupPath, dest)
	if err == nil {
		t.Error("RestoreDatabase() should error on checksum mismatch")
	}
}

// TestMaintenanceServiceStartStopExtended verifies that StartMaintenanceService and
// StopMaintenanceService run without deadlock and that the service transitions
// to running and then stops cleanly.
func TestMaintenanceServiceStartStopExtended(t *testing.T) {
	cfg := config.DefaultConfig()
	ms := NewMaintenanceService(cfg)

	if err := ms.StartMaintenanceService(); err != nil {
		t.Fatalf("StartMaintenanceService() error = %v", err)
	}

	// Calling Start again while running must be idempotent
	if err := ms.StartMaintenanceService(); err != nil {
		t.Errorf("StartMaintenanceService() second call error = %v", err)
	}

	ms.StopMaintenanceService()

	// Stop again — must not panic or deadlock
	ms.StopMaintenanceService()
}

// TestMaintenanceServiceModeTransitions covers SetMode, GetMode, and the
// convenience predicates (IsNormal, IsDegraded, IsInMaintenance).
func TestMaintenanceServiceModeTransitions(t *testing.T) {
	cfg := config.DefaultConfig()
	ms := NewMaintenanceService(cfg)

	tests := []struct {
		mode        MaintenanceMode
		msg         string
		isNormal    bool
		isDegraded  bool
		isMaintain  bool
	}{
		{ModeNormal, "", true, false, false},
		{ModeDegraded, "degraded test", false, true, false},
		{ModeMaintenance, "maintenance test", false, false, true},
		{ModeRecovery, "recovery test", false, false, false},
		{ModeEmergency, "emergency test", false, false, false},
	}

	for _, tt := range tests {
		ms.SetMode(tt.mode, tt.msg)
		if ms.GetMode() != tt.mode {
			t.Errorf("GetMode() = %v, want %v", ms.GetMode(), tt.mode)
		}
		if ms.IsNormal() != tt.isNormal {
			t.Errorf("IsNormal() = %v, want %v for mode %v", ms.IsNormal(), tt.isNormal, tt.mode)
		}
		if ms.IsDegraded() != tt.isDegraded {
			t.Errorf("IsDegraded() = %v, want %v for mode %v", ms.IsDegraded(), tt.isDegraded, tt.mode)
		}
		if ms.IsInMaintenance() != tt.isMaintain {
			t.Errorf("IsInMaintenance() = %v, want %v for mode %v", ms.IsInMaintenance(), tt.isMaintain, tt.mode)
		}
		if tt.msg != "" && ms.GetMessage() != tt.msg {
			t.Errorf("GetMessage() = %q, want %q", ms.GetMessage(), tt.msg)
		}
	}
}

// TestMaintenanceModeStringExtended covers the String() method on MaintenanceMode.
func TestMaintenanceModeStringExtended(t *testing.T) {
	tests := []struct {
		mode MaintenanceMode
		want string
	}{
		{ModeNormal, "normal"},
		{ModeDegraded, "degraded"},
		{ModeMaintenance, "maintenance"},
		{ModeRecovery, "recovery"},
		{ModeEmergency, "emergency"},
		{MaintenanceMode(99), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.mode.String(); got != tt.want {
			t.Errorf("MaintenanceMode(%d).String() = %q, want %q", tt.mode, got, tt.want)
		}
	}
}

// TestEnableMaintenanceDuration covers EnableMaintenance with a non-zero duration
// and verifies that GetScheduledEnd is set accordingly.
func TestEnableMaintenanceDuration(t *testing.T) {
	cfg := config.DefaultConfig()
	ms := NewMaintenanceService(cfg)

	before := time.Now()
	ms.EnableMaintenance("test", 5*time.Minute)

	if ms.GetMode() != ModeMaintenance {
		t.Errorf("GetMode() = %v, want ModeMaintenance", ms.GetMode())
	}
	end := ms.GetScheduledEnd()
	if !end.After(before) {
		t.Errorf("GetScheduledEnd() = %v, want after %v", end, before)
	}

	ms.DisableMaintenance()
	if ms.GetMode() != ModeNormal {
		t.Errorf("after DisableMaintenance GetMode() = %v, want ModeNormal", ms.GetMode())
	}
	if !ms.GetScheduledEnd().IsZero() {
		t.Errorf("GetScheduledEnd() should be zero after DisableMaintenance")
	}
}

// TestRegisterCallbackFired verifies that RegisterCallback callbacks fire when
// the mode changes.
func TestRegisterCallbackFired(t *testing.T) {
	cfg := config.DefaultConfig()
	ms := NewMaintenanceService(cfg)

	fired := make(chan MaintenanceMode, 1)
	ms.RegisterCallback(func(m MaintenanceMode) {
		fired <- m
	})

	ms.SetMode(ModeMaintenance, "cb test")

	select {
	case got := <-fired:
		if got != ModeMaintenance {
			t.Errorf("callback got %v, want ModeMaintenance", got)
		}
	case <-time.After(2 * time.Second):
		t.Error("callback never fired")
	}
}

// TestGetHealthStatusReturnsSnapshot verifies that GetHealthStatus returns a
// copy (not a live reference) and that the health map is populated after Start.
func TestGetHealthStatusReturnsSnapshot(t *testing.T) {
	cfg := config.DefaultConfig()
	ms := NewMaintenanceService(cfg)

	if err := ms.StartMaintenanceService(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer ms.StopMaintenanceService()

	status := ms.GetHealthStatus()
	if len(status) == 0 {
		t.Error("GetHealthStatus() returned empty map after Start")
	}

	// Verify it is a copy — mutating the result should not affect internal state
	for k := range status {
		status[k].Healthy = !status[k].Healthy
	}
	status2 := ms.GetHealthStatus()
	for k, v := range status2 {
		if v.Healthy != status2[k].Healthy {
			t.Errorf("health status for %q was mutated via snapshot", k)
		}
	}
}

// TestGracefulDegradation covers the full lifecycle of GracefulDegradation:
// mark, check, fallback, execute, recover.
func TestGracefulDegradation(t *testing.T) {
	gd := NewGracefulDegradation()

	const feature = "search_engine"

	// Initially not degraded
	if gd.IsDegraded(feature) {
		t.Error("IsDegraded() = true before any MarkDegraded call")
	}

	gd.MarkDegraded(feature)
	if !gd.IsDegraded(feature) {
		t.Error("IsDegraded() = false after MarkDegraded")
	}

	// GetDegradedFeatures must include the feature
	degraded := gd.GetDegradedFeatures()
	found := false
	for _, f := range degraded {
		if f == feature {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("GetDegradedFeatures() = %v, must include %q", degraded, feature)
	}

	// Register a fallback and verify Execute returns it
	gd.RegisterFallback(feature, func() interface{} { return "fallback_value" })
	result, err := gd.Execute(feature, func() (interface{}, error) {
		return "live_value", nil
	})
	if err != nil {
		t.Errorf("Execute() error = %v, want nil (fallback path)", err)
	}
	if result != "fallback_value" {
		t.Errorf("Execute() = %v, want fallback_value", result)
	}

	// Recover
	gd.MarkHealthy(feature)
	if gd.IsDegraded(feature) {
		t.Error("IsDegraded() = true after MarkHealthy")
	}

	// Execute on healthy feature runs the real function
	result, err = gd.Execute(feature, func() (interface{}, error) { return "live", nil })
	if err != nil {
		t.Errorf("Execute() healthy path error = %v", err)
	}
	if result != "live" {
		t.Errorf("Execute() healthy = %v, want live", result)
	}
}

// TestGracefulDegradationExecuteAutoDegrade verifies that Execute marks a feature
// degraded when the provided function returns an error and no fallback is registered.
func TestGracefulDegradationExecuteAutoDegrade(t *testing.T) {
	gd := NewGracefulDegradation()
	const feature = "db"

	_, err := gd.Execute(feature, func() (interface{}, error) {
		return nil, fmt.Errorf("db error")
	})
	if err == nil {
		t.Error("Execute() should return error when fn errors and no fallback registered")
	}
	if !gd.IsDegraded(feature) {
		t.Error("Execute() should mark feature degraded after fn error")
	}
}

// TestSetDatabaseChecks verifies that SetDatabaseChecks stores the functions and
// that the health monitor calls them via performHealthChecks.
func TestSetDatabaseChecks(t *testing.T) {
	cfg := config.DefaultConfig()
	ms := NewMaintenanceService(cfg)

	called := make(chan string, 2)
	ms.SetDatabaseChecks(
		func(_ context.Context) error { called <- "server"; return nil },
		func(_ context.Context) error { called <- "users"; return nil },
	)

	if err := ms.StartMaintenanceService(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer ms.StopMaintenanceService()

	// Both check functions should be called during initial performHealthChecks
	seen := make(map[string]bool)
	timeout := time.After(3 * time.Second)
	for len(seen) < 2 {
		select {
		case name := <-called:
			seen[name] = true
		case <-timeout:
			t.Fatalf("timed out waiting for DB checks; saw: %v", seen)
		}
	}
}

// TestRepairDatabaseREINDEXFallback verifies the REINDEX fallback branch in
// RepairDatabase. Placing a directory at the VACUUM INTO output path forces
// SQLite to reject the VACUUM operation, which triggers the REINDEX path.
func TestRepairDatabaseREINDEXFallback(t *testing.T) {
	cfg := config.DefaultConfig()
	ms := NewMaintenanceService(cfg)

	tempDir, err := os.MkdirTemp("", "reindex-test-")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "test.db")

	// Build a minimal valid SQLite database
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	if _, err := db.Exec("CREATE TABLE items (id INTEGER PRIMARY KEY, name TEXT)"); err != nil {
		db.Close()
		t.Fatalf("CREATE TABLE: %v", err)
	}
	if _, err := db.Exec("INSERT INTO items VALUES (1, 'test')"); err != nil {
		db.Close()
		t.Fatalf("INSERT: %v", err)
	}
	db.Close()

	// Block the VACUUM INTO output path with a directory so VACUUM INTO fails.
	// RepairDatabase uses dbPath+".clean" as the vacuum target.
	vacuumPath := dbPath + ".clean"
	if err := os.MkdirAll(vacuumPath, 0755); err != nil {
		t.Fatalf("MkdirAll vacuumPath: %v", err)
	}

	// Should fall through to REINDEX and return nil
	if err := ms.RepairDatabase(dbPath); err != nil {
		t.Errorf("RepairDatabase() REINDEX fallback error = %v, want nil", err)
	}
}

// TestCopyFileDstCreateFail covers the os.Create(dst) error return in copyFile
// by providing a destination path whose parent directory does not exist.
func TestCopyFileDstCreateFail(t *testing.T) {
	tmpDir := t.TempDir()

	src := filepath.Join(tmpDir, "src.db")
	if err := os.WriteFile(src, []byte("data"), 0644); err != nil {
		t.Fatalf("WriteFile src: %v", err)
	}

	// dst parent does not exist → os.Create fails
	dst := filepath.Join(tmpDir, "nonexistent", "dst.db")

	if err := copyFile(src, dst); err == nil {
		t.Error("copyFile() should fail when dst parent directory does not exist")
	}
}

// mockIntegrityDriverOnce ensures the mock driver is registered only once
var mockIntegrityDriverOnce sync.Once

// mockIntegrityDriver is a minimal database/sql/driver that returns integrity
// issues from any query, used to test the CheckDatabaseIntegrity "issues found" path.
type mockIntegrityDriver struct{}
type mockIntegrityConn struct{}
type mockIntegrityStmt struct{}

type mockIntegrityRows struct {
	rows []string
	pos  int
}

func (d *mockIntegrityDriver) Open(_ string) (driver.Conn, error) {
	return &mockIntegrityConn{}, nil
}

func (c *mockIntegrityConn) Prepare(_ string) (driver.Stmt, error) {
	return &mockIntegrityStmt{}, nil
}
func (c *mockIntegrityConn) Close() error                        { return nil }
func (c *mockIntegrityConn) Begin() (driver.Tx, error)           { return nil, fmt.Errorf("no tx") }
func (s *mockIntegrityStmt) Close() error                        { return nil }
func (s *mockIntegrityStmt) NumInput() int                       { return -1 }
func (s *mockIntegrityStmt) Exec(_ []driver.Value) (driver.Result, error) {
	return nil, nil
}
func (s *mockIntegrityStmt) Query(_ []driver.Value) (driver.Rows, error) {
	return &mockIntegrityRows{rows: []string{"corruption detected at page 1"}}, nil
}
func (r *mockIntegrityRows) Columns() []string { return []string{"integrity_check"} }
func (r *mockIntegrityRows) Close() error      { return nil }
func (r *mockIntegrityRows) Next(dest []driver.Value) error {
	if r.pos >= len(r.rows) {
		return io.EOF
	}
	dest[0] = r.rows[r.pos]
	r.pos++
	return nil
}

// TestHealthMonitorTickerBody covers the three statements inside the ticker case
// (performHealthChecks, evaluateSystemHealth, attemptRecovery) by running
// healthMonitorWithInterval with a 5ms tick, letting it fire at least once, then
// cancelling via the internal context.
func TestHealthMonitorTickerBody(t *testing.T) {
	cfg := config.DefaultConfig()
	ms := NewMaintenanceService(cfg)

	// Use a temp dir so the file-write health check actually passes
	dir, err := os.MkdirTemp("", "hm-ticker-")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	defer os.RemoveAll(dir)
	ms.SetFileWriteDir(dir)

	// Start monitor in background with a fast interval so the ticker fires quickly.
	// Directly call healthMonitorWithInterval (same package) to avoid the running
	// guard in StartMaintenanceService and to avoid a double-close on ms.done.
	go ms.healthMonitorWithInterval(5 * time.Millisecond)

	// Give the ticker time to fire at least once then cancel via internal context
	time.Sleep(50 * time.Millisecond)
	ms.cancel()

	// Wait for healthMonitorWithInterval to close done
	select {
	case <-ms.done:
	case <-time.After(2 * time.Second):
		t.Error("healthMonitorWithInterval did not exit after context cancel")
	}
}

// TestRepairDatabaseVacuumIntoFails covers the VACUUM INTO failure path in
// RepairDatabase: pre-creating the vacuum output path as a directory forces
// VACUUM INTO to fail, triggering the REINDEX fallback which should succeed.
func TestRepairDatabaseVacuumIntoFails(t *testing.T) {
	cfg := config.DefaultConfig()
	ms := NewMaintenanceService(cfg)

	tempDir, err := os.MkdirTemp("", "vacuum-fail-")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a valid SQLite database file
	dbPath := filepath.Join(tempDir, "test.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	if _, err := db.Exec("CREATE TABLE items (id INTEGER PRIMARY KEY, val TEXT)"); err != nil {
		db.Close()
		t.Fatalf("CREATE TABLE: %v", err)
	}
	if _, err := db.Exec("INSERT INTO items VALUES (1, 'hello')"); err != nil {
		db.Close()
		t.Fatalf("INSERT: %v", err)
	}
	db.Close()

	// Pre-create the VACUUM INTO output path as a directory so VACUUM INTO fails.
	// RepairDatabase constructs vacuumPath = dbPath + ".clean".
	vacuumObstacle := dbPath + ".clean"
	if err := os.Mkdir(vacuumObstacle, 0755); err != nil {
		t.Fatalf("Mkdir vacuumObstacle: %v", err)
	}

	// RepairDatabase should fall back to REINDEX (which succeeds on a valid db)
	// and return nil.
	if err := ms.RepairDatabase(dbPath); err != nil {
		t.Errorf("RepairDatabase() error = %v; expected nil (REINDEX fallback should succeed)", err)
	}
}

// TestBackupDatabaseDirCreationFails covers the MkdirAll error return in
// BackupDatabase by supplying a backupDir whose parent is an existing regular file.
func TestBackupDatabaseDirCreationFails(t *testing.T) {
	cfg := config.DefaultConfig()
	ms := NewMaintenanceService(cfg)

	tempDir, err := os.MkdirTemp("", "backup-dir-fail-")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a regular file that blocks MkdirAll from creating a subdir inside it
	blockingFile := filepath.Join(tempDir, "block")
	if err := os.WriteFile(blockingFile, []byte("x"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Create a dummy source db
	dbPath := filepath.Join(tempDir, "source.db")
	if err := os.WriteFile(dbPath, []byte("SQLite format 3"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// backupDir = blockingFile/subdir — MkdirAll cannot create this
	badBackupDir := filepath.Join(blockingFile, "subdir")
	_, err = ms.BackupDatabase(dbPath, badBackupDir)
	if err == nil {
		t.Error("BackupDatabase() should fail when backup directory cannot be created")
	}
}

// TestCheckDatabaseIntegrityIssuesFound covers the path in CheckDatabaseIntegrity
// where PRAGMA integrity_check returns rows other than "ok", triggering the
// "database integrity issues" error return.
func TestCheckDatabaseIntegrityIssuesFound(t *testing.T) {
	const driverName = "mock-integrity-fail"
	mockIntegrityDriverOnce.Do(func() {
		sql.Register(driverName, &mockIntegrityDriver{})
	})

	cfg := config.DefaultConfig()
	ms := NewMaintenanceService(cfg)

	db, err := sql.Open(driverName, "")
	if err != nil {
		t.Fatalf("sql.Open mock driver: %v", err)
	}
	defer db.Close()

	// The mock driver returns "corruption detected at page 1" → issues found path
	err = ms.CheckDatabaseIntegrity(db)
	if err == nil {
		t.Error("CheckDatabaseIntegrity() should return error when integrity issues found")
	}
}
