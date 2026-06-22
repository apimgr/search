package config

import (
	"database/sql"
	"os"
	"testing"

	_ "modernc.org/sqlite"
)

// openTestDB opens an in-memory SQLite database and creates the server_settings table.
func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS server_settings (
			key TEXT PRIMARY KEY,
			value TEXT,
			updated_at TEXT
		)
	`)
	if err != nil {
		db.Close()
		t.Fatalf("CREATE TABLE error = %v", err)
	}

	return db
}

// TestWriteToDatabaseSuccess verifies writeToDatabase upserts to the DB correctly.
func TestWriteToDatabaseSuccess(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	cfg := DefaultConfig()
	cs := NewConfigSync(db, cfg, "/test/path")

	err := cs.writeToDatabase("server.title", "TestTitle")
	if err != nil {
		t.Fatalf("writeToDatabase() error = %v", err)
	}

	var value string
	row := db.QueryRow("SELECT value FROM server_settings WHERE key = ?", "server.title")
	if err := row.Scan(&value); err != nil {
		t.Fatalf("QueryRow() error = %v", err)
	}

	// Value is stored as JSON
	if value != `"TestTitle"` {
		t.Errorf("stored value = %q, want %q", value, `"TestTitle"`)
	}
}

// TestWriteToDatabaseUpsert verifies writeToDatabase updates an existing key.
func TestWriteToDatabaseUpsert(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	cfg := DefaultConfig()
	cs := NewConfigSync(db, cfg, "/test/path")

	if err := cs.writeToDatabase("server.port", 8080); err != nil {
		t.Fatalf("writeToDatabase() first write error = %v", err)
	}
	if err := cs.writeToDatabase("server.port", 9090); err != nil {
		t.Fatalf("writeToDatabase() second write error = %v", err)
	}

	var value string
	row := db.QueryRow("SELECT value FROM server_settings WHERE key = ?", "server.port")
	if err := row.Scan(&value); err != nil {
		t.Fatalf("QueryRow() error = %v", err)
	}

	// Should have the updated value
	if value != "9090" {
		t.Errorf("upserted value = %q, want '9090'", value)
	}
}

// TestWriteToDatabaseNonSerializableValue verifies writeToDatabase errors on unmarshalable value.
func TestWriteToDatabaseNonSerializableValue(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	cfg := DefaultConfig()
	cs := NewConfigSync(db, cfg, "/test/path")

	// Channels cannot be JSON-marshaled
	ch := make(chan int)
	err := cs.writeToDatabase("key", ch)
	if err == nil {
		t.Error("writeToDatabase() with unmarshalable value should return error")
	}
}

// TestEnsureSensitiveFileSuccess verifies EnsureSensitiveFile works on an existing file.
func TestEnsureSensitiveFileSuccess(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "sensitive-file-test-*")
	if err != nil {
		t.Fatalf("MkdirTemp() error = %v", err)
	}
	defer os.RemoveAll(tmpDir)

	path := tmpDir + "/secret.key"
	if err := os.WriteFile(path, []byte("secret"), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if err := EnsureSensitiveFile(path); err != nil {
		t.Errorf("EnsureSensitiveFile() error = %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}

	// Permissions should be 0600 (sensitive)
	if info.Mode().Perm() != GetSensitiveFilePermissions() {
		t.Errorf("permissions = %v, want %v", info.Mode().Perm(), GetSensitiveFilePermissions())
	}
}

// TestEnsureSensitiveFileNonExistent verifies EnsureSensitiveFile errors on missing file.
func TestEnsureSensitiveFileNonExistent(t *testing.T) {
	err := EnsureSensitiveFile("/nonexistent/path/to/file.key")
	if err == nil {
		t.Error("EnsureSensitiveFile() on nonexistent file should return error")
	}
}

// TestWriteToDatabaseExecError verifies writeToDatabase returns error when the DB exec fails.
func TestWriteToDatabaseExecError(t *testing.T) {
	db := openTestDB(t)
	// Close DB immediately so any exec will fail
	db.Close()

	cfg := DefaultConfig()
	cs := NewConfigSync(db, cfg, "/test/path")

	err := cs.writeToDatabase("some.key", "value")
	if err == nil {
		t.Error("writeToDatabase() with closed DB should return error")
	}
}

// TestSyncToLocalConfigEmptyPath verifies syncToLocalConfig errors when configPath is empty.
func TestSyncToLocalConfigEmptyPath(t *testing.T) {
	cfg := DefaultConfig()
	cs := NewConfigSync(nil, cfg, "")

	err := cs.syncToLocalConfig()
	if err == nil {
		t.Error("syncToLocalConfig() with empty configPath should return error")
	}
}

// TestSyncToLocalConfigUnwritablePath verifies syncToLocalConfig errors when the path is a directory.
func TestSyncToLocalConfigUnwritablePath(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "sync-unwritable-*")
	if err != nil {
		t.Fatalf("MkdirTemp() error = %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a directory at the config path location — os.WriteFile on a directory fails
	dirPath := tmpDir + "/server.yml"
	if err := os.MkdirAll(dirPath, 0755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	cfg := DefaultConfig()
	cs := NewConfigSync(nil, cfg, dirPath)

	err = cs.syncToLocalConfig()
	if err == nil {
		t.Error("syncToLocalConfig() with directory target path should return error")
	}
}
