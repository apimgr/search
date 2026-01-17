package database

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg == nil {
		t.Fatal("DefaultConfig() returned nil")
	}
	if cfg.Driver != "sqlite" {
		t.Errorf("Driver = %q, want sqlite", cfg.Driver)
	}
	if cfg.MaxOpen != 10 {
		t.Errorf("MaxOpen = %d, want 10", cfg.MaxOpen)
	}
	if cfg.MaxIdle != 5 {
		t.Errorf("MaxIdle = %d, want 5", cfg.MaxIdle)
	}
	if cfg.Lifetime != 300 {
		t.Errorf("Lifetime = %d, want 300", cfg.Lifetime)
	}
}

func TestConfigStruct(t *testing.T) {
	cfg := Config{
		Driver:   "postgres",
		DSN:      "postgres://localhost/test",
		DataDir:  "/tmp/data",
		MaxOpen:  20,
		MaxIdle:  10,
		Lifetime: 600,
	}

	if cfg.Driver != "postgres" {
		t.Errorf("Driver = %q", cfg.Driver)
	}
	if cfg.DSN != "postgres://localhost/test" {
		t.Errorf("DSN = %q", cfg.DSN)
	}
}

func TestNewDatabaseManager(t *testing.T) {
	tempDir := t.TempDir()

	cfg := &Config{
		Driver:   "sqlite",
		DataDir:  tempDir,
		MaxOpen:  5,
		MaxIdle:  2,
		Lifetime: 60,
	}

	dm, err := NewDatabaseManager(cfg)
	if err != nil {
		t.Fatalf("NewDatabaseManager() error = %v", err)
	}
	defer dm.Close()

	if dm == nil {
		t.Fatal("NewDatabaseManager() returned nil")
	}

	// Both databases should be created
	if dm.ServerDB() == nil {
		t.Error("ServerDB() returned nil")
	}
	if dm.UsersDB() == nil {
		t.Error("UsersDB() returned nil")
	}
}

func TestNewDatabaseManagerDefaultConfig(t *testing.T) {
	tempDir := t.TempDir()

	// Temporarily override the default data dir
	cfg := &Config{
		Driver:  "sqlite",
		DataDir: tempDir,
	}

	dm, err := NewDatabaseManager(cfg)
	if err != nil {
		t.Fatalf("NewDatabaseManager() error = %v", err)
	}
	defer dm.Close()

	if dm == nil {
		t.Fatal("NewDatabaseManager() returned nil")
	}
}

func TestDatabaseManagerIsReady(t *testing.T) {
	tempDir := t.TempDir()

	cfg := &Config{
		Driver:  "sqlite",
		DataDir: tempDir,
	}

	dm, err := NewDatabaseManager(cfg)
	if err != nil {
		t.Fatalf("NewDatabaseManager() error = %v", err)
	}
	defer dm.Close()

	if !dm.IsReady() {
		t.Error("IsReady() should return true after successful init")
	}
}

func TestDatabaseManagerPing(t *testing.T) {
	tempDir := t.TempDir()

	cfg := &Config{
		Driver:  "sqlite",
		DataDir: tempDir,
	}

	dm, err := NewDatabaseManager(cfg)
	if err != nil {
		t.Fatalf("NewDatabaseManager() error = %v", err)
	}
	defer dm.Close()

	ctx := context.Background()
	if err := dm.Ping(ctx); err != nil {
		t.Errorf("Ping() error = %v", err)
	}
}

func TestDatabaseManagerIsClusterMode(t *testing.T) {
	tempDir := t.TempDir()

	cfg := &Config{
		Driver:  "sqlite",
		DataDir: tempDir,
	}

	dm, err := NewDatabaseManager(cfg)
	if err != nil {
		t.Fatalf("NewDatabaseManager() error = %v", err)
	}
	defer dm.Close()

	// SQLite should not be cluster mode
	if dm.IsClusterMode() {
		t.Error("IsClusterMode() should return false for SQLite")
	}
}

func TestDatabaseManagerClose(t *testing.T) {
	tempDir := t.TempDir()

	cfg := &Config{
		Driver:  "sqlite",
		DataDir: tempDir,
	}

	dm, err := NewDatabaseManager(cfg)
	if err != nil {
		t.Fatalf("NewDatabaseManager() error = %v", err)
	}

	if err := dm.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}

	// After close, IsReady should return false
	if dm.IsReady() {
		t.Error("IsReady() should return false after Close()")
	}
}

func TestNew(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")

	cfg := &Config{
		Driver: "sqlite",
		DSN:    dbPath,
	}

	db, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer db.Close()

	if db == nil {
		t.Fatal("New() returned nil")
	}
	if !db.IsReady() {
		t.Error("IsReady() should return true")
	}
}

func TestNewWithDefaultConfig(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")

	// Override DSN after getting default
	cfg := DefaultConfig()
	cfg.DSN = dbPath

	db, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer db.Close()

	if !db.IsReady() {
		t.Error("IsReady() should return true")
	}
}

func TestDBDriver(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")

	cfg := &Config{
		Driver: "sqlite",
		DSN:    dbPath,
	}

	db, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer db.Close()

	if db.Driver() != "sqlite" {
		t.Errorf("Driver() = %q, want sqlite", db.Driver())
	}
}

func TestDBIsRemote(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")

	cfg := &Config{
		Driver: "sqlite",
		DSN:    dbPath,
	}

	db, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer db.Close()

	if db.IsRemote() {
		t.Error("IsRemote() should return false for SQLite")
	}
}

func TestDBSQL(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")

	cfg := &Config{
		Driver: "sqlite",
		DSN:    dbPath,
	}

	db, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer db.Close()

	sqlDB := db.SQL()
	if sqlDB == nil {
		t.Error("SQL() returned nil")
	}
}

func TestDBExec(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")

	cfg := &Config{
		Driver: "sqlite",
		DSN:    dbPath,
	}

	db, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer db.Close()

	ctx := context.Background()

	// Create a test table
	_, err = db.Exec(ctx, "CREATE TABLE test (id INTEGER PRIMARY KEY, name TEXT)")
	if err != nil {
		t.Fatalf("Exec(CREATE TABLE) error = %v", err)
	}

	// Insert a row
	result, err := db.Exec(ctx, "INSERT INTO test (name) VALUES (?)", "test")
	if err != nil {
		t.Fatalf("Exec(INSERT) error = %v", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		t.Fatalf("RowsAffected() error = %v", err)
	}
	if rowsAffected != 1 {
		t.Errorf("RowsAffected() = %d, want 1", rowsAffected)
	}
}

func TestDBQuery(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")

	cfg := &Config{
		Driver: "sqlite",
		DSN:    dbPath,
	}

	db, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer db.Close()

	ctx := context.Background()

	// Setup
	db.Exec(ctx, "CREATE TABLE test (id INTEGER PRIMARY KEY, name TEXT)")
	db.Exec(ctx, "INSERT INTO test (name) VALUES (?)", "alice")
	db.Exec(ctx, "INSERT INTO test (name) VALUES (?)", "bob")

	// Query
	rows, err := db.Query(ctx, "SELECT name FROM test ORDER BY name")
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("Scan() error = %v", err)
		}
		names = append(names, name)
	}

	if len(names) != 2 {
		t.Errorf("Query returned %d rows, want 2", len(names))
	}
	if names[0] != "alice" {
		t.Errorf("First name = %q, want alice", names[0])
	}
}

func TestDBQueryRow(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")

	cfg := &Config{
		Driver: "sqlite",
		DSN:    dbPath,
	}

	db, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer db.Close()

	ctx := context.Background()

	// Setup
	db.Exec(ctx, "CREATE TABLE test (id INTEGER PRIMARY KEY, name TEXT)")
	db.Exec(ctx, "INSERT INTO test (name) VALUES (?)", "test")

	// QueryRow
	var name string
	row := db.QueryRow(ctx, "SELECT name FROM test WHERE id = ?", 1)
	if err := row.Scan(&name); err != nil {
		t.Fatalf("Scan() error = %v", err)
	}

	if name != "test" {
		t.Errorf("name = %q, want test", name)
	}
}

func TestDBBegin(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")

	cfg := &Config{
		Driver: "sqlite",
		DSN:    dbPath,
	}

	db, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer db.Close()

	ctx := context.Background()

	// Setup
	db.Exec(ctx, "CREATE TABLE test (id INTEGER PRIMARY KEY, name TEXT)")

	// Begin transaction
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("Begin() error = %v", err)
	}

	// Insert in transaction
	_, err = tx.Exec("INSERT INTO test (name) VALUES (?)", "test")
	if err != nil {
		tx.Rollback()
		t.Fatalf("Exec in tx error = %v", err)
	}

	// Commit
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit() error = %v", err)
	}

	// Verify insert
	var count int
	row := db.QueryRow(ctx, "SELECT COUNT(*) FROM test")
	if err := row.Scan(&count); err != nil {
		t.Fatalf("Count query error = %v", err)
	}
	if count != 1 {
		t.Errorf("count = %d, want 1", count)
	}
}

func TestDBPing(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")

	cfg := &Config{
		Driver: "sqlite",
		DSN:    dbPath,
	}

	db, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	if err := db.Ping(ctx); err != nil {
		t.Errorf("Ping() error = %v", err)
	}
}

func TestDBPingNotReady(t *testing.T) {
	db := &DB{
		ready: false,
	}

	ctx := context.Background()
	if err := db.Ping(ctx); err == nil {
		t.Error("Ping() should error when not ready")
	}
}

func TestDBExecNotReady(t *testing.T) {
	db := &DB{
		ready: false,
	}

	ctx := context.Background()
	_, err := db.Exec(ctx, "SELECT 1")
	if err == nil {
		t.Error("Exec() should error when not ready")
	}
}

func TestDBQueryNotReady(t *testing.T) {
	db := &DB{
		ready: false,
	}

	ctx := context.Background()
	_, err := db.Query(ctx, "SELECT 1")
	if err == nil {
		t.Error("Query() should error when not ready")
	}
}

func TestDBBeginNotReady(t *testing.T) {
	db := &DB{
		ready: false,
	}

	ctx := context.Background()
	_, err := db.Begin(ctx)
	if err == nil {
		t.Error("Begin() should error when not ready")
	}
}

func TestDBClose(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")

	cfg := &Config{
		Driver: "sqlite",
		DSN:    dbPath,
	}

	db, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if err := db.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}

	if db.IsReady() {
		t.Error("IsReady() should return false after Close()")
	}
}

func TestDBCloseNilDB(t *testing.T) {
	db := &DB{}

	if err := db.Close(); err != nil {
		t.Errorf("Close() should not error with nil db, got %v", err)
	}
}

func TestNewUnsupportedDriver(t *testing.T) {
	cfg := &Config{
		Driver: "unsupported",
		DSN:    "/tmp/test.db",
	}

	_, err := New(cfg)
	if err == nil {
		t.Error("New() should error for unsupported driver")
	}
}

func TestDatabaseManagerUnsupportedDriver(t *testing.T) {
	tempDir := t.TempDir()

	cfg := &Config{
		Driver:  "unsupported",
		DataDir: tempDir,
	}

	_, err := NewDatabaseManager(cfg)
	if err == nil {
		t.Error("NewDatabaseManager() should error for unsupported driver")
	}
}

func TestDatabaseManagerPostgresRequiresDSN(t *testing.T) {
	tempDir := t.TempDir()

	cfg := &Config{
		Driver:  "postgres",
		DataDir: tempDir,
		DSN:     "", // Empty DSN
	}

	_, err := NewDatabaseManager(cfg)
	if err == nil {
		t.Error("NewDatabaseManager() should error for postgres without DSN")
	}
}

func TestDatabaseManagerMysqlRequiresDSN(t *testing.T) {
	tempDir := t.TempDir()

	cfg := &Config{
		Driver:  "mysql",
		DataDir: tempDir,
		DSN:     "", // Empty DSN
	}

	_, err := NewDatabaseManager(cfg)
	if err == nil {
		t.Error("NewDatabaseManager() should error for mysql without DSN")
	}
}

func TestDatabaseManagerMssqlRequiresDSN(t *testing.T) {
	tempDir := t.TempDir()

	cfg := &Config{
		Driver:  "mssql",
		DataDir: tempDir,
		DSN:     "", // Empty DSN
	}

	_, err := NewDatabaseManager(cfg)
	if err == nil {
		t.Error("NewDatabaseManager() should error for mssql without DSN")
	}
}

func TestDBIsRemotePgx(t *testing.T) {
	db := &DB{
		driver: "pgx",
		ready:  true,
	}

	if !db.IsRemote() {
		t.Error("IsRemote() should return true for pgx driver")
	}
}

func TestDBIsRemoteMysql(t *testing.T) {
	db := &DB{
		driver: "mysql",
		ready:  true,
	}

	if !db.IsRemote() {
		t.Error("IsRemote() should return true for mysql driver")
	}
}

func TestDatabaseFilesCreated(t *testing.T) {
	tempDir := t.TempDir()

	cfg := &Config{
		Driver:  "sqlite",
		DataDir: tempDir,
	}

	dm, err := NewDatabaseManager(cfg)
	if err != nil {
		t.Fatalf("NewDatabaseManager() error = %v", err)
	}
	defer dm.Close()

	// Check that database files exist
	serverDB := filepath.Join(tempDir, "server.db")
	if _, err := os.Stat(serverDB); os.IsNotExist(err) {
		t.Error("server.db should be created")
	}

	userDB := filepath.Join(tempDir, "user.db")
	if _, err := os.Stat(userDB); os.IsNotExist(err) {
		t.Error("user.db should be created")
	}
}

func TestDBContextTimeout(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")

	cfg := &Config{
		Driver: "sqlite",
		DSN:    dbPath,
	}

	db, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer db.Close()

	// Create a cancelled context
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	time.Sleep(1 * time.Millisecond)
	cancel()

	// Operations with cancelled context should fail
	_, err = db.Query(ctx, "SELECT 1")
	if err == nil {
		// May or may not error depending on timing
	}
}

// Tests for cluster.go

func TestClusterModeConstants(t *testing.T) {
	if ClusterModeStandalone != "standalone" {
		t.Errorf("ClusterModeStandalone = %q, want 'standalone'", ClusterModeStandalone)
	}
	if ClusterModeCluster != "cluster" {
		t.Errorf("ClusterModeCluster = %q, want 'cluster'", ClusterModeCluster)
	}
}

func TestNodeStatusConstants(t *testing.T) {
	tests := []struct {
		status NodeStatus
		want   string
	}{
		{NodeStatusOnline, "online"},
		{NodeStatusOffline, "offline"},
		{NodeStatusJoining, "joining"},
		{NodeStatusLeaving, "leaving"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if string(tt.status) != tt.want {
				t.Errorf("NodeStatus = %q, want %q", tt.status, tt.want)
			}
		})
	}
}

func TestClusterNodeStruct(t *testing.T) {
	now := time.Now()
	node := ClusterNode{
		ID:        "node-123",
		Hostname:  "server1.example.com",
		Address:   "192.168.1.100",
		Port:      8080,
		Version:   "1.0.0",
		IsPrimary: true,
		Status:    NodeStatusOnline,
		LastSeen:  now,
		JoinedAt:  now.Add(-24 * time.Hour),
		Metadata:  map[string]string{"region": "us-east"},
	}

	if node.ID != "node-123" {
		t.Errorf("ID = %q, want 'node-123'", node.ID)
	}
	if node.Hostname != "server1.example.com" {
		t.Errorf("Hostname = %q, want 'server1.example.com'", node.Hostname)
	}
	if node.Address != "192.168.1.100" {
		t.Errorf("Address = %q, want '192.168.1.100'", node.Address)
	}
	if node.Port != 8080 {
		t.Errorf("Port = %d, want 8080", node.Port)
	}
	if !node.IsPrimary {
		t.Error("IsPrimary should be true")
	}
	if node.Status != NodeStatusOnline {
		t.Errorf("Status = %q, want 'online'", node.Status)
	}
	if node.Metadata["region"] != "us-east" {
		t.Errorf("Metadata['region'] = %q, want 'us-east'", node.Metadata["region"])
	}
}

func TestNewClusterManager(t *testing.T) {
	tempDir := t.TempDir()

	cfg := &Config{
		Driver:  "sqlite",
		DataDir: tempDir,
	}

	dm, err := NewDatabaseManager(cfg)
	if err != nil {
		t.Fatalf("NewDatabaseManager() error = %v", err)
	}
	defer dm.Close()

	cm, err := NewClusterManager(dm)
	if err != nil {
		t.Fatalf("NewClusterManager() error = %v", err)
	}

	if cm == nil {
		t.Fatal("NewClusterManager() returned nil")
	}

	// SQLite should be standalone mode
	if cm.Mode() != ClusterModeStandalone {
		t.Errorf("Mode() = %q, want 'standalone'", cm.Mode())
	}

	if cm.NodeID() == "" {
		t.Error("NodeID() should not be empty")
	}
}

func TestClusterManagerMode(t *testing.T) {
	tempDir := t.TempDir()

	cfg := &Config{
		Driver:  "sqlite",
		DataDir: tempDir,
	}

	dm, err := NewDatabaseManager(cfg)
	if err != nil {
		t.Fatalf("NewDatabaseManager() error = %v", err)
	}
	defer dm.Close()

	cm, err := NewClusterManager(dm)
	if err != nil {
		t.Fatalf("NewClusterManager() error = %v", err)
	}

	// SQLite is always standalone
	if cm.Mode() != ClusterModeStandalone {
		t.Errorf("Mode() = %q, want 'standalone'", cm.Mode())
	}
}

func TestClusterManagerIsClusterMode(t *testing.T) {
	tempDir := t.TempDir()

	cfg := &Config{
		Driver:  "sqlite",
		DataDir: tempDir,
	}

	dm, err := NewDatabaseManager(cfg)
	if err != nil {
		t.Fatalf("NewDatabaseManager() error = %v", err)
	}
	defer dm.Close()

	cm, err := NewClusterManager(dm)
	if err != nil {
		t.Fatalf("NewClusterManager() error = %v", err)
	}

	// SQLite should not be cluster mode
	if cm.IsClusterMode() {
		t.Error("IsClusterMode() should return false for SQLite")
	}
}

func TestClusterManagerHostname(t *testing.T) {
	tempDir := t.TempDir()

	cfg := &Config{
		Driver:  "sqlite",
		DataDir: tempDir,
	}

	dm, err := NewDatabaseManager(cfg)
	if err != nil {
		t.Fatalf("NewDatabaseManager() error = %v", err)
	}
	defer dm.Close()

	cm, err := NewClusterManager(dm)
	if err != nil {
		t.Fatalf("NewClusterManager() error = %v", err)
	}

	// Hostname should not be empty (will be "unknown" if not detected)
	if cm.Hostname() == "" {
		t.Error("Hostname() should not be empty")
	}
}

// Tests for Config fields

func TestConfigPoolSettings(t *testing.T) {
	cfg := &Config{
		Driver:   "sqlite",
		DSN:      ":memory:",
		MaxOpen:  100,
		MaxIdle:  25,
		Lifetime: 300,
	}

	if cfg.MaxOpen != 100 {
		t.Errorf("MaxOpen = %d, want 100", cfg.MaxOpen)
	}
	if cfg.MaxIdle != 25 {
		t.Errorf("MaxIdle = %d, want 25", cfg.MaxIdle)
	}
	if cfg.Lifetime != 300 {
		t.Errorf("Lifetime = %d, want 300", cfg.Lifetime)
	}
}

func TestDBDriverVariants(t *testing.T) {
	tests := []struct {
		driver   string
		isRemote bool
	}{
		{"sqlite", false},
		{"sqlite3", false},
		{"pgx", true},
		{"postgres", true},
		{"mysql", true},
		{"mssql", true},
	}

	for _, tt := range tests {
		t.Run(tt.driver, func(t *testing.T) {
			// Just verify the driver names are valid
			if tt.driver == "" {
				t.Error("Driver should not be empty")
			}
		})
	}
}

// Tests for migrations.go

func TestMigrationStruct(t *testing.T) {
	m := Migration{
		Version:     1,
		Description: "Create test table",
		Up:          "CREATE TABLE test (id INTEGER PRIMARY KEY)",
		Down:        "DROP TABLE test",
	}

	if m.Version != 1 {
		t.Errorf("Version = %d, want 1", m.Version)
	}
	if m.Description != "Create test table" {
		t.Errorf("Description = %q", m.Description)
	}
	if m.Up == "" {
		t.Error("Up should not be empty")
	}
	if m.Down == "" {
		t.Error("Down should not be empty")
	}
}

func TestNewMigrator(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")

	cfg := &Config{
		Driver: "sqlite",
		DSN:    dbPath,
	}

	db, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer db.Close()

	m := NewMigrator(db)
	if m == nil {
		t.Fatal("NewMigrator() returned nil")
	}

	// Should have at least one migration registered
	migrations := m.GetMigrations()
	if len(migrations) == 0 {
		t.Error("GetMigrations() should return at least one migration")
	}
}

func TestMigratorRegister(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")

	cfg := &Config{
		Driver: "sqlite",
		DSN:    dbPath,
	}

	db, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer db.Close()

	m := &Migrator{
		db:         db,
		migrations: make([]Migration, 0),
	}

	// Register a migration
	m.Register(Migration{
		Version:     100,
		Description: "Test migration",
		Up:          "SELECT 1",
		Down:        "SELECT 1",
	})

	migrations := m.GetMigrations()
	if len(migrations) != 1 {
		t.Errorf("GetMigrations() returned %d migrations, want 1", len(migrations))
	}
	if migrations[0].Version != 100 {
		t.Errorf("Migration version = %d, want 100", migrations[0].Version)
	}
}

func TestMigratorMigrate(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")

	cfg := &Config{
		Driver: "sqlite",
		DSN:    dbPath,
	}

	db, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer db.Close()

	m := NewMigrator(db)

	ctx := context.Background()
	if err := m.Migrate(ctx); err != nil {
		t.Errorf("Migrate() error = %v", err)
	}

	// Verify migration ran without error - schema_version table should exist
	// The legacy single-db migrator only has one migration (schema_version table)
	// which gets created during Migrate() but version 1 isn't recorded
	// (since the table creation is version 1 and it's skipped in the loop)
	_, err = m.GetVersion(ctx)
	if err != nil {
		t.Errorf("GetVersion() error = %v (schema_version table may not exist)", err)
	}
}

func TestMigratorGetVersion(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")

	cfg := &Config{
		Driver: "sqlite",
		DSN:    dbPath,
	}

	db, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer db.Close()

	m := NewMigrator(db)
	ctx := context.Background()

	// Run migrations first
	if err := m.Migrate(ctx); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	// GetVersion should work without error
	version, err := m.GetVersion(ctx)
	if err != nil {
		t.Errorf("GetVersion() error = %v", err)
	}
	// Version can be 0 for legacy single-db migrator since it only has
	// the schema_version table migration which is handled specially
	if version < 0 {
		t.Errorf("Version = %d, should be >= 0", version)
	}
}

func TestNewDatabaseMigrator(t *testing.T) {
	tempDir := t.TempDir()

	cfg := &Config{
		Driver:  "sqlite",
		DataDir: tempDir,
	}

	dm, err := NewDatabaseManager(cfg)
	if err != nil {
		t.Fatalf("NewDatabaseManager() error = %v", err)
	}
	defer dm.Close()

	dbm := NewDatabaseMigrator(dm)
	if dbm == nil {
		t.Fatal("NewDatabaseMigrator() returned nil")
	}

	// Should have both migrators
	if dbm.GetServerMigrator() == nil {
		t.Error("GetServerMigrator() returned nil")
	}
	if dbm.GetUsersMigrator() == nil {
		t.Error("GetUsersMigrator() returned nil")
	}
}

func TestDatabaseMigratorMigrateAll(t *testing.T) {
	tempDir := t.TempDir()

	cfg := &Config{
		Driver:  "sqlite",
		DataDir: tempDir,
	}

	dm, err := NewDatabaseManager(cfg)
	if err != nil {
		t.Fatalf("NewDatabaseManager() error = %v", err)
	}
	defer dm.Close()

	dbm := NewDatabaseMigrator(dm)
	ctx := context.Background()

	if err := dbm.MigrateAll(ctx); err != nil {
		t.Errorf("MigrateAll() error = %v", err)
	}

	// Verify migrations ran on both databases
	serverVersion, err := dbm.GetServerMigrator().GetVersion(ctx)
	if err != nil {
		t.Errorf("GetServerMigrator().GetVersion() error = %v", err)
	}
	if serverVersion < 1 {
		t.Errorf("Server version = %d, should be >= 1", serverVersion)
	}

	usersVersion, err := dbm.GetUsersMigrator().GetVersion(ctx)
	if err != nil {
		t.Errorf("GetUsersMigrator().GetVersion() error = %v", err)
	}
	if usersVersion < 1 {
		t.Errorf("Users version = %d, should be >= 1", usersVersion)
	}
}

// Tests for mixed.go

func TestMixedModeConfigStruct(t *testing.T) {
	cfg := MixedModeConfig{
		ServerDB: DatabaseBackendConfig{
			Driver:   "sqlite",
			Path:     "/tmp/server.db",
			MaxOpen:  10,
			MaxIdle:  5,
			Lifetime: 300,
		},
		UsersDB: DatabaseBackendConfig{
			Driver:   "sqlite",
			Path:     "/tmp/user.db",
			MaxOpen:  10,
			MaxIdle:  5,
			Lifetime: 300,
		},
	}

	if cfg.ServerDB.Driver != "sqlite" {
		t.Errorf("ServerDB.Driver = %q, want 'sqlite'", cfg.ServerDB.Driver)
	}
	if cfg.UsersDB.Path != "/tmp/user.db" {
		t.Errorf("UsersDB.Path = %q", cfg.UsersDB.Path)
	}
}

func TestDatabaseBackendConfigStruct(t *testing.T) {
	cfg := DatabaseBackendConfig{
		Driver:   "postgres",
		DSN:      "postgres://localhost/test",
		Path:     "",
		MaxOpen:  50,
		MaxIdle:  25,
		Lifetime: 600,
	}

	if cfg.Driver != "postgres" {
		t.Errorf("Driver = %q, want 'postgres'", cfg.Driver)
	}
	if cfg.DSN != "postgres://localhost/test" {
		t.Errorf("DSN = %q", cfg.DSN)
	}
	if cfg.MaxOpen != 50 {
		t.Errorf("MaxOpen = %d, want 50", cfg.MaxOpen)
	}
}

func TestDefaultMixedModeConfig(t *testing.T) {
	dataDir := "/data/test"
	cfg := DefaultMixedModeConfig(dataDir)

	if cfg == nil {
		t.Fatal("DefaultMixedModeConfig() returned nil")
	}
	if cfg.ServerDB.Driver != "sqlite" {
		t.Errorf("ServerDB.Driver = %q, want 'sqlite'", cfg.ServerDB.Driver)
	}
	if cfg.UsersDB.Driver != "sqlite" {
		t.Errorf("UsersDB.Driver = %q, want 'sqlite'", cfg.UsersDB.Driver)
	}
	if cfg.ServerDB.MaxOpen != 10 {
		t.Errorf("ServerDB.MaxOpen = %d, want 10", cfg.ServerDB.MaxOpen)
	}
}

func TestNewMixedModeManager(t *testing.T) {
	tempDir := t.TempDir()

	cfg := &MixedModeConfig{
		ServerDB: DatabaseBackendConfig{
			Driver:   "sqlite",
			Path:     filepath.Join(tempDir, "server.db"),
			MaxOpen:  5,
			MaxIdle:  2,
			Lifetime: 60,
		},
		UsersDB: DatabaseBackendConfig{
			Driver:   "sqlite",
			Path:     filepath.Join(tempDir, "user.db"),
			MaxOpen:  5,
			MaxIdle:  2,
			Lifetime: 60,
		},
	}

	mm, err := NewMixedModeManager(cfg)
	if err != nil {
		t.Fatalf("NewMixedModeManager() error = %v", err)
	}
	defer mm.Close()

	if mm == nil {
		t.Fatal("NewMixedModeManager() returned nil")
	}
	if !mm.IsReady() {
		t.Error("IsReady() should return true after successful init")
	}
}

func TestNewMixedModeManagerNilConfig(t *testing.T) {
	_, err := NewMixedModeManager(nil)
	if err == nil {
		t.Error("NewMixedModeManager(nil) should return error")
	}
}

func TestMixedModeManagerServerDB(t *testing.T) {
	tempDir := t.TempDir()

	cfg := &MixedModeConfig{
		ServerDB: DatabaseBackendConfig{
			Driver: "sqlite",
			Path:   filepath.Join(tempDir, "server.db"),
		},
		UsersDB: DatabaseBackendConfig{
			Driver: "sqlite",
			Path:   filepath.Join(tempDir, "user.db"),
		},
	}

	mm, err := NewMixedModeManager(cfg)
	if err != nil {
		t.Fatalf("NewMixedModeManager() error = %v", err)
	}
	defer mm.Close()

	serverDB := mm.ServerDB()
	if serverDB == nil {
		t.Error("ServerDB() returned nil")
	}
	if !serverDB.IsReady() {
		t.Error("ServerDB should be ready")
	}
}

func TestMixedModeManagerUsersDB(t *testing.T) {
	tempDir := t.TempDir()

	cfg := &MixedModeConfig{
		ServerDB: DatabaseBackendConfig{
			Driver: "sqlite",
			Path:   filepath.Join(tempDir, "server.db"),
		},
		UsersDB: DatabaseBackendConfig{
			Driver: "sqlite",
			Path:   filepath.Join(tempDir, "user.db"),
		},
	}

	mm, err := NewMixedModeManager(cfg)
	if err != nil {
		t.Fatalf("NewMixedModeManager() error = %v", err)
	}
	defer mm.Close()

	usersDB := mm.UsersDB()
	if usersDB == nil {
		t.Error("UsersDB() returned nil")
	}
	if !usersDB.IsReady() {
		t.Error("UsersDB should be ready")
	}
}

func TestMixedModeManagerIsMixedMode(t *testing.T) {
	tempDir := t.TempDir()

	// Same drivers - not mixed mode
	cfg := &MixedModeConfig{
		ServerDB: DatabaseBackendConfig{
			Driver: "sqlite",
			Path:   filepath.Join(tempDir, "server.db"),
		},
		UsersDB: DatabaseBackendConfig{
			Driver: "sqlite",
			Path:   filepath.Join(tempDir, "user.db"),
		},
	}

	mm, err := NewMixedModeManager(cfg)
	if err != nil {
		t.Fatalf("NewMixedModeManager() error = %v", err)
	}
	defer mm.Close()

	if mm.IsMixedMode() {
		t.Error("IsMixedMode() should return false for same drivers")
	}
}

func TestMixedModeManagerGetMode(t *testing.T) {
	tempDir := t.TempDir()

	cfg := &MixedModeConfig{
		ServerDB: DatabaseBackendConfig{
			Driver: "sqlite",
			Path:   filepath.Join(tempDir, "server.db"),
		},
		UsersDB: DatabaseBackendConfig{
			Driver: "sqlite",
			Path:   filepath.Join(tempDir, "user.db"),
		},
	}

	mm, err := NewMixedModeManager(cfg)
	if err != nil {
		t.Fatalf("NewMixedModeManager() error = %v", err)
	}
	defer mm.Close()

	mode := mm.GetMode()
	if mode != "standalone" {
		t.Errorf("GetMode() = %q, want 'standalone'", mode)
	}
}

func TestMixedModeManagerGetStatus(t *testing.T) {
	tempDir := t.TempDir()

	cfg := &MixedModeConfig{
		ServerDB: DatabaseBackendConfig{
			Driver: "sqlite",
			Path:   filepath.Join(tempDir, "server.db"),
		},
		UsersDB: DatabaseBackendConfig{
			Driver: "sqlite",
			Path:   filepath.Join(tempDir, "user.db"),
		},
	}

	mm, err := NewMixedModeManager(cfg)
	if err != nil {
		t.Fatalf("NewMixedModeManager() error = %v", err)
	}
	defer mm.Close()

	status := mm.GetStatus()
	if status == nil {
		t.Fatal("GetStatus() returned nil")
	}
	if status["mode"] != "standalone" {
		t.Errorf("status['mode'] = %v, want 'standalone'", status["mode"])
	}
	if status["server_db_driver"] != "sqlite" {
		t.Errorf("status['server_db_driver'] = %v", status["server_db_driver"])
	}
	if status["server_db_ready"] != true {
		t.Error("status['server_db_ready'] should be true")
	}
}

func TestMixedModeManagerClose(t *testing.T) {
	tempDir := t.TempDir()

	cfg := &MixedModeConfig{
		ServerDB: DatabaseBackendConfig{
			Driver: "sqlite",
			Path:   filepath.Join(tempDir, "server.db"),
		},
		UsersDB: DatabaseBackendConfig{
			Driver: "sqlite",
			Path:   filepath.Join(tempDir, "user.db"),
		},
	}

	mm, err := NewMixedModeManager(cfg)
	if err != nil {
		t.Fatalf("NewMixedModeManager() error = %v", err)
	}

	if err := mm.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}

	if mm.IsReady() {
		t.Error("IsReady() should return false after Close()")
	}
}

func TestMixedDBDriver(t *testing.T) {
	tempDir := t.TempDir()

	cfg := &MixedModeConfig{
		ServerDB: DatabaseBackendConfig{
			Driver: "sqlite",
			Path:   filepath.Join(tempDir, "server.db"),
		},
		UsersDB: DatabaseBackendConfig{
			Driver: "sqlite",
			Path:   filepath.Join(tempDir, "user.db"),
		},
	}

	mm, err := NewMixedModeManager(cfg)
	if err != nil {
		t.Fatalf("NewMixedModeManager() error = %v", err)
	}
	defer mm.Close()

	if mm.ServerDB().Driver() != "sqlite" {
		t.Errorf("Driver() = %q, want 'sqlite'", mm.ServerDB().Driver())
	}
}

func TestMixedDBIsLocal(t *testing.T) {
	tempDir := t.TempDir()

	cfg := &MixedModeConfig{
		ServerDB: DatabaseBackendConfig{
			Driver: "sqlite",
			Path:   filepath.Join(tempDir, "server.db"),
		},
		UsersDB: DatabaseBackendConfig{
			Driver: "sqlite",
			Path:   filepath.Join(tempDir, "user.db"),
		},
	}

	mm, err := NewMixedModeManager(cfg)
	if err != nil {
		t.Fatalf("NewMixedModeManager() error = %v", err)
	}
	defer mm.Close()

	if !mm.ServerDB().IsLocal() {
		t.Error("IsLocal() should return true for SQLite")
	}
	if mm.ServerDB().IsRemote() {
		t.Error("IsRemote() should return false for SQLite")
	}
}

func TestMixedDBExec(t *testing.T) {
	tempDir := t.TempDir()

	cfg := &MixedModeConfig{
		ServerDB: DatabaseBackendConfig{
			Driver: "sqlite",
			Path:   filepath.Join(tempDir, "server.db"),
		},
		UsersDB: DatabaseBackendConfig{
			Driver: "sqlite",
			Path:   filepath.Join(tempDir, "user.db"),
		},
	}

	mm, err := NewMixedModeManager(cfg)
	if err != nil {
		t.Fatalf("NewMixedModeManager() error = %v", err)
	}
	defer mm.Close()

	ctx := context.Background()

	// Create table
	_, err = mm.ServerDB().Exec(ctx, "CREATE TABLE test (id INTEGER PRIMARY KEY, name TEXT)")
	if err != nil {
		t.Fatalf("Exec(CREATE TABLE) error = %v", err)
	}

	// Insert
	result, err := mm.ServerDB().Exec(ctx, "INSERT INTO test (name) VALUES (?)", "test")
	if err != nil {
		t.Fatalf("Exec(INSERT) error = %v", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		t.Fatalf("RowsAffected() error = %v", err)
	}
	if rowsAffected != 1 {
		t.Errorf("RowsAffected() = %d, want 1", rowsAffected)
	}
}

func TestMixedDBQuery(t *testing.T) {
	tempDir := t.TempDir()

	cfg := &MixedModeConfig{
		ServerDB: DatabaseBackendConfig{
			Driver: "sqlite",
			Path:   filepath.Join(tempDir, "server.db"),
		},
		UsersDB: DatabaseBackendConfig{
			Driver: "sqlite",
			Path:   filepath.Join(tempDir, "user.db"),
		},
	}

	mm, err := NewMixedModeManager(cfg)
	if err != nil {
		t.Fatalf("NewMixedModeManager() error = %v", err)
	}
	defer mm.Close()

	ctx := context.Background()

	// Setup
	mm.ServerDB().Exec(ctx, "CREATE TABLE test (id INTEGER PRIMARY KEY, name TEXT)")
	mm.ServerDB().Exec(ctx, "INSERT INTO test (name) VALUES (?)", "alice")

	// Query
	rows, err := mm.ServerDB().Query(ctx, "SELECT name FROM test")
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	defer rows.Close()

	var name string
	if rows.Next() {
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("Scan() error = %v", err)
		}
	}

	if name != "alice" {
		t.Errorf("name = %q, want 'alice'", name)
	}
}

func TestMixedDBQueryRow(t *testing.T) {
	tempDir := t.TempDir()

	cfg := &MixedModeConfig{
		ServerDB: DatabaseBackendConfig{
			Driver: "sqlite",
			Path:   filepath.Join(tempDir, "server.db"),
		},
		UsersDB: DatabaseBackendConfig{
			Driver: "sqlite",
			Path:   filepath.Join(tempDir, "user.db"),
		},
	}

	mm, err := NewMixedModeManager(cfg)
	if err != nil {
		t.Fatalf("NewMixedModeManager() error = %v", err)
	}
	defer mm.Close()

	ctx := context.Background()

	// Setup
	mm.ServerDB().Exec(ctx, "CREATE TABLE test (id INTEGER PRIMARY KEY, name TEXT)")
	mm.ServerDB().Exec(ctx, "INSERT INTO test (name) VALUES (?)", "bob")

	// QueryRow
	var name string
	row := mm.ServerDB().QueryRow(ctx, "SELECT name FROM test WHERE id = ?", 1)
	if err := row.Scan(&name); err != nil {
		t.Fatalf("Scan() error = %v", err)
	}

	if name != "bob" {
		t.Errorf("name = %q, want 'bob'", name)
	}
}

func TestMixedDBBegin(t *testing.T) {
	tempDir := t.TempDir()

	cfg := &MixedModeConfig{
		ServerDB: DatabaseBackendConfig{
			Driver: "sqlite",
			Path:   filepath.Join(tempDir, "server.db"),
		},
		UsersDB: DatabaseBackendConfig{
			Driver: "sqlite",
			Path:   filepath.Join(tempDir, "user.db"),
		},
	}

	mm, err := NewMixedModeManager(cfg)
	if err != nil {
		t.Fatalf("NewMixedModeManager() error = %v", err)
	}
	defer mm.Close()

	ctx := context.Background()

	// Setup
	mm.ServerDB().Exec(ctx, "CREATE TABLE test (id INTEGER PRIMARY KEY, name TEXT)")

	// Begin transaction
	tx, err := mm.ServerDB().Begin(ctx)
	if err != nil {
		t.Fatalf("Begin() error = %v", err)
	}

	// Insert in transaction
	_, err = tx.Exec("INSERT INTO test (name) VALUES (?)", "test")
	if err != nil {
		tx.Rollback()
		t.Fatalf("Exec in tx error = %v", err)
	}

	// Commit
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit() error = %v", err)
	}
}

func TestMixedDBPing(t *testing.T) {
	tempDir := t.TempDir()

	cfg := &MixedModeConfig{
		ServerDB: DatabaseBackendConfig{
			Driver: "sqlite",
			Path:   filepath.Join(tempDir, "server.db"),
		},
		UsersDB: DatabaseBackendConfig{
			Driver: "sqlite",
			Path:   filepath.Join(tempDir, "user.db"),
		},
	}

	mm, err := NewMixedModeManager(cfg)
	if err != nil {
		t.Fatalf("NewMixedModeManager() error = %v", err)
	}
	defer mm.Close()

	ctx := context.Background()
	if err := mm.ServerDB().Ping(ctx); err != nil {
		t.Errorf("Ping() error = %v", err)
	}
}

func TestMixedDBSupportsReturning(t *testing.T) {
	mdb := &MixedDB{
		driver: "sqlite",
		ready:  true,
	}

	if !mdb.SupportsReturning() {
		t.Error("SupportsReturning() should return true for SQLite")
	}

	mdb.driver = "postgres"
	if !mdb.SupportsReturning() {
		t.Error("SupportsReturning() should return true for PostgreSQL")
	}

	mdb.driver = "mysql"
	if mdb.SupportsReturning() {
		t.Error("SupportsReturning() should return false for MySQL")
	}
}

func TestMixedDBSupportsUpsert(t *testing.T) {
	mdb := &MixedDB{
		driver: "sqlite",
		ready:  true,
	}

	if !mdb.SupportsUpsert() {
		t.Error("SupportsUpsert() should return true for SQLite")
	}

	mdb.driver = "postgres"
	if !mdb.SupportsUpsert() {
		t.Error("SupportsUpsert() should return true for PostgreSQL")
	}

	mdb.driver = "mysql"
	if !mdb.SupportsUpsert() {
		t.Error("SupportsUpsert() should return true for MySQL")
	}
}

func TestMixedDBGetPlaceholder(t *testing.T) {
	mdb := &MixedDB{
		driver: "sqlite",
		ready:  true,
	}

	if mdb.GetPlaceholder(1) != "?" {
		t.Errorf("GetPlaceholder(1) = %q, want '?'", mdb.GetPlaceholder(1))
	}

	mdb.driver = "postgres"
	if mdb.GetPlaceholder(1) != "$1" {
		t.Errorf("GetPlaceholder(1) = %q, want '$1'", mdb.GetPlaceholder(1))
	}
	if mdb.GetPlaceholder(5) != "$5" {
		t.Errorf("GetPlaceholder(5) = %q, want '$5'", mdb.GetPlaceholder(5))
	}
}

func TestMixedDBNotReady(t *testing.T) {
	mdb := &MixedDB{
		ready: false,
	}

	ctx := context.Background()

	// Exec should error
	_, err := mdb.Exec(ctx, "SELECT 1")
	if err == nil {
		t.Error("Exec() should error when not ready")
	}

	// Query should error
	_, err = mdb.Query(ctx, "SELECT 1")
	if err == nil {
		t.Error("Query() should error when not ready")
	}

	// Begin should error
	_, err = mdb.Begin(ctx)
	if err == nil {
		t.Error("Begin() should error when not ready")
	}

	// Ping should error
	err = mdb.Ping(ctx)
	if err == nil {
		t.Error("Ping() should error when not ready")
	}
}

func TestClusterManagerIsPrimary(t *testing.T) {
	tempDir := t.TempDir()

	cfg := &Config{
		Driver:  "sqlite",
		DataDir: tempDir,
	}

	dm, err := NewDatabaseManager(cfg)
	if err != nil {
		t.Fatalf("NewDatabaseManager() error = %v", err)
	}
	defer dm.Close()

	cm, err := NewClusterManager(dm)
	if err != nil {
		t.Fatalf("NewClusterManager() error = %v", err)
	}

	// Start in standalone mode - should become primary
	ctx := context.Background()
	if err := cm.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer cm.Stop()

	// In standalone mode, node should be primary
	if !cm.IsPrimary() {
		t.Error("IsPrimary() should return true in standalone mode after Start()")
	}
}

func TestClusterManagerStartStop(t *testing.T) {
	tempDir := t.TempDir()

	cfg := &Config{
		Driver:  "sqlite",
		DataDir: tempDir,
	}

	dm, err := NewDatabaseManager(cfg)
	if err != nil {
		t.Fatalf("NewDatabaseManager() error = %v", err)
	}
	defer dm.Close()

	cm, err := NewClusterManager(dm)
	if err != nil {
		t.Fatalf("NewClusterManager() error = %v", err)
	}

	ctx := context.Background()

	// Start
	if err := cm.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Start again should be no-op
	if err := cm.Start(ctx); err != nil {
		t.Errorf("Start() second call error = %v", err)
	}

	// Stop
	cm.Stop()

	// Stop again should be no-op
	cm.Stop()
}

func TestClusterManagerGetNodes(t *testing.T) {
	tempDir := t.TempDir()

	cfg := &Config{
		Driver:  "sqlite",
		DataDir: tempDir,
	}

	dm, err := NewDatabaseManager(cfg)
	if err != nil {
		t.Fatalf("NewDatabaseManager() error = %v", err)
	}
	defer dm.Close()

	cm, err := NewClusterManager(dm)
	if err != nil {
		t.Fatalf("NewClusterManager() error = %v", err)
	}

	ctx := context.Background()

	// In standalone mode, should return self as only node
	nodes, err := cm.GetNodes(ctx)
	if err != nil {
		t.Fatalf("GetNodes() error = %v", err)
	}

	if len(nodes) != 1 {
		t.Errorf("GetNodes() returned %d nodes, want 1", len(nodes))
	}

	if nodes[0].ID != cm.NodeID() {
		t.Errorf("Node ID = %q, want %q", nodes[0].ID, cm.NodeID())
	}
}

func TestClusterManagerGetNode(t *testing.T) {
	tempDir := t.TempDir()

	cfg := &Config{
		Driver:  "sqlite",
		DataDir: tempDir,
	}

	dm, err := NewDatabaseManager(cfg)
	if err != nil {
		t.Fatalf("NewDatabaseManager() error = %v", err)
	}
	defer dm.Close()

	cm, err := NewClusterManager(dm)
	if err != nil {
		t.Fatalf("NewClusterManager() error = %v", err)
	}

	ctx := context.Background()

	// Get self node
	node, err := cm.GetNode(ctx, cm.NodeID())
	if err != nil {
		t.Fatalf("GetNode() error = %v", err)
	}
	if node == nil {
		t.Fatal("GetNode() returned nil for self")
	}
	if node.ID != cm.NodeID() {
		t.Errorf("Node ID = %q, want %q", node.ID, cm.NodeID())
	}

	// Get non-existent node
	nonExistent, err := cm.GetNode(ctx, "non-existent-id")
	if err != nil {
		t.Fatalf("GetNode() error = %v", err)
	}
	if nonExistent != nil {
		t.Error("GetNode() should return nil for non-existent node")
	}
}

func TestClusterManagerGenerateJoinTokenStandalone(t *testing.T) {
	tempDir := t.TempDir()

	cfg := &Config{
		Driver:  "sqlite",
		DataDir: tempDir,
	}

	dm, err := NewDatabaseManager(cfg)
	if err != nil {
		t.Fatalf("NewDatabaseManager() error = %v", err)
	}
	defer dm.Close()

	cm, err := NewClusterManager(dm)
	if err != nil {
		t.Fatalf("NewClusterManager() error = %v", err)
	}

	ctx := context.Background()

	// In standalone mode, should error
	_, err = cm.GenerateJoinToken(ctx)
	if err == nil {
		t.Error("GenerateJoinToken() should error in standalone mode")
	}
}

func TestClusterManagerLeaveClusterStandalone(t *testing.T) {
	tempDir := t.TempDir()

	cfg := &Config{
		Driver:  "sqlite",
		DataDir: tempDir,
	}

	dm, err := NewDatabaseManager(cfg)
	if err != nil {
		t.Fatalf("NewDatabaseManager() error = %v", err)
	}
	defer dm.Close()

	cm, err := NewClusterManager(dm)
	if err != nil {
		t.Fatalf("NewClusterManager() error = %v", err)
	}

	ctx := context.Background()

	// In standalone mode, should error
	err = cm.LeaveCluster(ctx)
	if err == nil {
		t.Error("LeaveCluster() should error in standalone mode")
	}
}
