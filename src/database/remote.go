package database

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// RemoteDBConfig holds configuration for remote database connections
// Per AI.md PART 24: Remote database support (PostgreSQL, MySQL/MariaDB)
type RemoteDBConfig struct {
	Driver   string `yaml:"driver"`   // postgres, mysql
	Host     string `yaml:"host"`     // database host
	Port     int    `yaml:"port"`     // database port
	Database string `yaml:"database"` // database name
	Username string `yaml:"username"` // database user
	Password string `yaml:"password"` // database password
	SSLMode  string `yaml:"ssl_mode"` // disable, require, verify-ca, verify-full (postgres)
	Options  string `yaml:"options"`  // additional connection options
}

// DefaultRemoteDBConfig returns default remote database configuration
func DefaultRemoteDBConfig() *RemoteDBConfig {
	return &RemoteDBConfig{
		Driver:  "postgres",
		Host:    "localhost",
		Port:    5432,
		SSLMode: "require",
	}
}

// BuildDSN builds the connection string for the remote database
func (r *RemoteDBConfig) BuildDSN() string {
	switch r.Driver {
	case "postgres":
		return fmt.Sprintf(
			"postgres://%s:%s@%s:%d/%s?sslmode=%s",
			r.Username, r.Password, r.Host, r.Port, r.Database, r.SSLMode,
		)
	case "mysql", "mariadb":
		return fmt.Sprintf(
			"%s:%s@tcp(%s:%d)/%s?parseTime=true&charset=utf8mb4",
			r.Username, r.Password, r.Host, r.Port, r.Database,
		)
	default:
		return ""
	}
}

// RemoteDB represents a connection to a remote database
type RemoteDB struct {
	db     *sql.DB
	driver string
	config *RemoteDBConfig
	mu     sync.RWMutex
	ready  bool
}

// NewRemoteDB creates a new remote database connection
func NewRemoteDB(cfg *RemoteDBConfig) (*RemoteDB, error) {
	if cfg == nil {
		return nil, fmt.Errorf("remote database configuration is nil")
	}

	rdb := &RemoteDB{
		driver: cfg.Driver,
		config: cfg,
	}

	if err := rdb.connect(); err != nil {
		return nil, err
	}

	return rdb, nil
}

// connect establishes connection to the remote database
func (rdb *RemoteDB) connect() error {
	rdb.mu.Lock()
	defer rdb.mu.Unlock()

	dsn := rdb.config.BuildDSN()
	if dsn == "" {
		return fmt.Errorf("unsupported database driver: %s", rdb.driver)
	}

	var err error

	// Note: In production, you would import the drivers:
	// _ "github.com/lib/pq" for PostgreSQL
	// _ "github.com/go-sql-driver/mysql" for MySQL
	// For now, we return an error since the drivers aren't imported
	rdb.db, err = sql.Open(rdb.driver, dsn)
	if err != nil {
		return fmt.Errorf("failed to open remote database: %w", err)
	}

	// Configure connection pool
	rdb.db.SetMaxOpenConns(25)
	rdb.db.SetMaxIdleConns(5)
	rdb.db.SetConnMaxLifetime(5 * time.Minute)

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := rdb.db.PingContext(ctx); err != nil {
		return fmt.Errorf("failed to connect to remote database: %w", err)
	}

	rdb.ready = true
	log.Printf("[Database] Connected to remote %s database at %s:%d",
		rdb.driver, rdb.config.Host, rdb.config.Port)

	return nil
}

// Close closes the remote database connection
func (rdb *RemoteDB) Close() error {
	rdb.mu.Lock()
	defer rdb.mu.Unlock()

	if rdb.db != nil {
		rdb.ready = false
		return rdb.db.Close()
	}
	return nil
}

// IsReady returns true if the remote database is ready
func (rdb *RemoteDB) IsReady() bool {
	rdb.mu.RLock()
	defer rdb.mu.RUnlock()
	return rdb.ready
}

// DB returns the underlying sql.DB
func (rdb *RemoteDB) DB() *sql.DB {
	rdb.mu.RLock()
	defer rdb.mu.RUnlock()
	return rdb.db
}

// MigrationManager handles migration from SQLite to remote database
// Per AI.md PART 24: Auto-migrate local → remote
type MigrationManager struct {
	sourceDB *DB
	targetDB *RemoteDB
	dataDir  string
}

// NewMigrationManager creates a new migration manager
func NewMigrationManager(sourceDB *DB, targetDB *RemoteDB, dataDir string) *MigrationManager {
	return &MigrationManager{
		sourceDB: sourceDB,
		targetDB: targetDB,
		dataDir:  dataDir,
	}
}

// MigrationProgress represents the progress of a migration
type MigrationProgress struct {
	Phase       string    `json:"phase"`
	Table       string    `json:"table"`
	TotalRows   int64     `json:"total_rows"`
	MigratedRows int64    `json:"migrated_rows"`
	StartTime   time.Time `json:"start_time"`
	Error       string    `json:"error,omitempty"`
}

// MigrateToRemote migrates a SQLite database to the remote database
func (mm *MigrationManager) MigrateToRemote(ctx context.Context, progress chan<- MigrationProgress) error {
	if mm.sourceDB == nil {
		return fmt.Errorf("source database is nil")
	}
	if mm.targetDB == nil || !mm.targetDB.IsReady() {
		return fmt.Errorf("target database is not ready")
	}

	// Get list of tables from SQLite
	tables, err := mm.getSourceTables(ctx)
	if err != nil {
		return fmt.Errorf("failed to get source tables: %w", err)
	}

	startTime := time.Now()

	for _, table := range tables {
		// Skip internal SQLite tables
		if strings.HasPrefix(table, "sqlite_") {
			continue
		}

		if progress != nil {
			progress <- MigrationProgress{
				Phase:     "migrating",
				Table:     table,
				StartTime: startTime,
			}
		}

		// Get table schema
		schema, err := mm.getTableSchema(ctx, table)
		if err != nil {
			errMsg := fmt.Sprintf("failed to get schema for %s: %v", table, err)
			if progress != nil {
				progress <- MigrationProgress{Phase: "error", Table: table, Error: errMsg}
			}
			return fmt.Errorf("failed to get schema for %s: %w", table, err)
		}

		// Create table in target database
		if err := mm.createTargetTable(ctx, table, schema); err != nil {
			errMsg := fmt.Sprintf("failed to create target table %s: %v", table, err)
			if progress != nil {
				progress <- MigrationProgress{Phase: "error", Table: table, Error: errMsg}
			}
			return fmt.Errorf("failed to create target table %s: %w", table, err)
		}

		// Count rows for progress
		totalRows, err := mm.countRows(ctx, table)
		if err != nil {
			log.Printf("[Migration] Warning: failed to count rows in %s: %v", table, err)
		}

		// Migrate data
		migratedRows, err := mm.migrateTableData(ctx, table, progress, totalRows)
		if err != nil {
			errMsg := fmt.Sprintf("failed to migrate data for %s: %v", table, err)
			if progress != nil {
				progress <- MigrationProgress{Phase: "error", Table: table, Error: errMsg}
			}
			return fmt.Errorf("failed to migrate data for %s: %w", table, err)
		}

		if progress != nil {
			progress <- MigrationProgress{
				Phase:        "completed",
				Table:        table,
				TotalRows:    totalRows,
				MigratedRows: migratedRows,
				StartTime:    startTime,
			}
		}

		log.Printf("[Migration] Migrated table %s: %d rows", table, migratedRows)
	}

	if progress != nil {
		progress <- MigrationProgress{Phase: "done", StartTime: startTime}
		close(progress)
	}

	return nil
}

// getSourceTables returns the list of tables in the source database
func (mm *MigrationManager) getSourceTables(ctx context.Context) ([]string, error) {
	rows, err := mm.sourceDB.Query(ctx, `
		SELECT name FROM sqlite_master
		WHERE type='table' AND name NOT LIKE 'sqlite_%'
		ORDER BY name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		tables = append(tables, name)
	}

	return tables, rows.Err()
}

// getTableSchema returns the CREATE TABLE statement for a SQLite table
func (mm *MigrationManager) getTableSchema(ctx context.Context, table string) (string, error) {
	var schema string
	err := mm.sourceDB.QueryRow(ctx, `
		SELECT sql FROM sqlite_master WHERE type='table' AND name=?
	`, table).Scan(&schema)
	return schema, err
}

// countRows counts the rows in a source table
func (mm *MigrationManager) countRows(ctx context.Context, table string) (int64, error) {
	var count int64
	err := mm.sourceDB.QueryRow(ctx, fmt.Sprintf(`SELECT COUNT(*) FROM "%s"`, table)).Scan(&count)
	return count, err
}

// createTargetTable creates a table in the target database
func (mm *MigrationManager) createTargetTable(ctx context.Context, table, schema string) error {
	// Convert SQLite schema to target database schema
	targetSchema := mm.convertSchema(schema)

	_, err := mm.targetDB.db.ExecContext(ctx, targetSchema)
	if err != nil {
		// Table might already exist
		if strings.Contains(err.Error(), "already exists") {
			log.Printf("[Migration] Table %s already exists in target", table)
			return nil
		}
		return err
	}

	return nil
}

// convertSchema converts SQLite schema to target database schema
func (mm *MigrationManager) convertSchema(sqliteSchema string) string {
	schema := sqliteSchema

	switch mm.targetDB.driver {
	case "postgres":
		// Convert SQLite types to PostgreSQL
		schema = strings.ReplaceAll(schema, "INTEGER PRIMARY KEY AUTOINCREMENT", "SERIAL PRIMARY KEY")
		schema = strings.ReplaceAll(schema, "AUTOINCREMENT", "")
		schema = strings.ReplaceAll(schema, "DATETIME", "TIMESTAMP")
		schema = strings.ReplaceAll(schema, "datetime", "TIMESTAMP")

	case "mysql", "mariadb":
		// Convert SQLite types to MySQL
		schema = strings.ReplaceAll(schema, "INTEGER PRIMARY KEY AUTOINCREMENT", "INT AUTO_INCREMENT PRIMARY KEY")
		schema = strings.ReplaceAll(schema, "AUTOINCREMENT", "AUTO_INCREMENT")
		schema = strings.ReplaceAll(schema, "DATETIME", "DATETIME")
		// Add engine specification
		if !strings.HasSuffix(schema, ";") {
			schema += ";"
		}
		schema = strings.TrimSuffix(schema, ";") + " ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;"
	}

	return schema
}

// migrateTableData migrates data from source to target table
func (mm *MigrationManager) migrateTableData(ctx context.Context, table string, progress chan<- MigrationProgress, totalRows int64) (int64, error) {
	// Get column names
	columns, err := mm.getTableColumns(ctx, table)
	if err != nil {
		return 0, fmt.Errorf("failed to get columns: %w", err)
	}

	if len(columns) == 0 {
		return 0, nil
	}

	// Build SELECT query
	selectQuery := fmt.Sprintf(`SELECT "%s" FROM "%s"`, strings.Join(columns, `", "`), table)

	rows, err := mm.sourceDB.Query(ctx, selectQuery)
	if err != nil {
		return 0, fmt.Errorf("failed to query source: %w", err)
	}
	defer rows.Close()

	// Build INSERT query
	placeholders := make([]string, len(columns))
	for i := range placeholders {
		switch mm.targetDB.driver {
		case "postgres":
			placeholders[i] = fmt.Sprintf("$%d", i+1)
		default:
			placeholders[i] = "?"
		}
	}
	insertQuery := fmt.Sprintf(
		`INSERT INTO "%s" ("%s") VALUES (%s)`,
		table,
		strings.Join(columns, `", "`),
		strings.Join(placeholders, ", "),
	)

	// Migrate rows
	var migratedRows int64
	values := make([]interface{}, len(columns))
	valuePtrs := make([]interface{}, len(columns))
	for i := range values {
		valuePtrs[i] = &values[i]
	}

	for rows.Next() {
		if err := rows.Scan(valuePtrs...); err != nil {
			return migratedRows, fmt.Errorf("failed to scan row: %w", err)
		}

		_, err := mm.targetDB.db.ExecContext(ctx, insertQuery, values...)
		if err != nil {
			// Skip duplicate key errors
			if !strings.Contains(err.Error(), "duplicate") && !strings.Contains(err.Error(), "UNIQUE constraint") {
				return migratedRows, fmt.Errorf("failed to insert row: %w", err)
			}
		}

		migratedRows++

		// Report progress every 1000 rows
		if progress != nil && migratedRows%1000 == 0 {
			progress <- MigrationProgress{
				Phase:        "migrating",
				Table:        table,
				TotalRows:    totalRows,
				MigratedRows: migratedRows,
			}
		}
	}

	return migratedRows, rows.Err()
}

// getTableColumns returns the column names for a table
func (mm *MigrationManager) getTableColumns(ctx context.Context, table string) ([]string, error) {
	rows, err := mm.sourceDB.Query(ctx, fmt.Sprintf(`PRAGMA table_info("%s")`, table))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var columns []string
	for rows.Next() {
		var cid int
		var name, colType string
		var notNull, pk int
		var defaultValue interface{}

		if err := rows.Scan(&cid, &name, &colType, &notNull, &defaultValue, &pk); err != nil {
			return nil, err
		}
		columns = append(columns, name)
	}

	return columns, rows.Err()
}

// BackupBeforeMigration creates a backup of the SQLite database before migration
func (mm *MigrationManager) BackupBeforeMigration(dbPath string) (string, error) {
	backupDir := filepath.Join(mm.dataDir, "backups", "pre-migration")
	if err := os.MkdirAll(backupDir, 0700); err != nil {
		return "", fmt.Errorf("failed to create backup directory: %w", err)
	}

	timestamp := time.Now().Format("20060102-150405")
	backupPath := filepath.Join(backupDir, filepath.Base(dbPath)+"."+timestamp)

	// Copy the file
	src, err := os.Open(dbPath)
	if err != nil {
		return "", fmt.Errorf("failed to open source: %w", err)
	}
	defer src.Close()

	dst, err := os.Create(backupPath)
	if err != nil {
		return "", fmt.Errorf("failed to create backup: %w", err)
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return "", fmt.Errorf("failed to copy data: %w", err)
	}

	log.Printf("[Migration] Created backup: %s", backupPath)
	return backupPath, nil
}

// ValidateRemoteConnection tests the connection to a remote database
func ValidateRemoteConnection(cfg *RemoteDBConfig) error {
	rdb, err := NewRemoteDB(cfg)
	if err != nil {
		return err
	}
	defer rdb.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := rdb.db.PingContext(ctx); err != nil {
		return fmt.Errorf("ping failed: %w", err)
	}

	return nil
}

// GetSupportedDrivers returns the list of supported remote database drivers
func GetSupportedDrivers() []string {
	return []string{"postgres", "mysql", "mariadb"}
}

// IsRemoteDriver returns true if the driver is a remote database driver
func IsRemoteDriver(driver string) bool {
	for _, d := range GetSupportedDrivers() {
		if d == driver {
			return true
		}
	}
	return false
}
