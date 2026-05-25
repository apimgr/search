package database

import (
	"context"
	"fmt"
	"sort"
	"time"
)

// Migration represents a database migration
type Migration struct {
	Version     int
	Description string
	Up          string
	Down        string
}

// Migrator handles database migrations
type Migrator struct {
	db         *DB
	migrations []Migration
}

// DatabaseMigrator handles migrations for both databases per AI.md PART 24
type DatabaseMigrator struct {
	dm             *DatabaseManager
	serverMigrator *Migrator
	usersMigrator  *Migrator
}

// NewDatabaseMigrator creates a new migrator for both databases
func NewDatabaseMigrator(dm *DatabaseManager) *DatabaseMigrator {
	dbm := &DatabaseMigrator{
		dm: dm,
	}

	// Create server database migrator
	dbm.serverMigrator = &Migrator{
		db:         dm.ServerDB(),
		migrations: make([]Migration, 0),
	}
	dbm.registerServerMigrations()

	// Create users database migrator. This project has NO user accounts;
	// the database is provisioned with only a schema_version table so the
	// DatabaseManager API stays unchanged.
	dbm.usersMigrator = &Migrator{
		db:         dm.UsersDB(),
		migrations: make([]Migration, 0),
	}
	dbm.registerUsersMigrations()

	return dbm
}

// registerServerMigrations registers migrations for server.db.
//
// Per spec there is no admin web UI, no sessions, and no user accounts.
// Tables kept: scheduler_state, audit_log, server_settings, api_tokens
// (per-resource owner tokens, SHA-256 hashed), and the search alert tables.
func (dbm *DatabaseMigrator) registerServerMigrations() {
	m := dbm.serverMigrator

	// Migration 1: Schema version table
	m.Register(Migration{
		Version:     1,
		Description: "Create schema_version table",
		Up: `
			CREATE TABLE IF NOT EXISTS schema_version (
				version INTEGER PRIMARY KEY,
				description TEXT NOT NULL,
				applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
			)
		`,
		Down: `DROP TABLE IF EXISTS schema_version`,
	})

	// Migration 2: Scheduler state
	m.Register(Migration{
		Version:     2,
		Description: "Create scheduler_state table",
		Up: `
			CREATE TABLE IF NOT EXISTS scheduler_state (
				task_id TEXT PRIMARY KEY,
				last_run DATETIME,
				next_run DATETIME,
				last_result TEXT,
				last_error TEXT,
				run_count INTEGER DEFAULT 0,
				enabled INTEGER DEFAULT 1
			);
		`,
		Down: `DROP TABLE IF EXISTS scheduler_state`,
	})

	// Migration 3: Audit log
	m.Register(Migration{
		Version:     3,
		Description: "Create audit_log table",
		Up: `
			CREATE TABLE IF NOT EXISTS audit_log (
				id TEXT PRIMARY KEY,
				timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
				actor_type TEXT NOT NULL,
				actor_id TEXT,
				action TEXT NOT NULL,
				resource_type TEXT,
				resource_id TEXT,
				details TEXT,
				ip_address TEXT,
				user_agent TEXT,
				severity TEXT DEFAULT 'info',
				category TEXT
			);
			CREATE INDEX idx_audit_log_timestamp ON audit_log(timestamp);
			CREATE INDEX idx_audit_log_actor ON audit_log(actor_type, actor_id);
			CREATE INDEX idx_audit_log_action ON audit_log(action);
			CREATE INDEX idx_audit_log_resource ON audit_log(resource_type, resource_id);
		`,
		Down: `DROP TABLE IF EXISTS audit_log`,
	})

	// Migration 4: Server settings
	m.Register(Migration{
		Version:     4,
		Description: "Create server_settings table",
		Up: `
			CREATE TABLE IF NOT EXISTS server_settings (
				key TEXT PRIMARY KEY,
				value TEXT,
				updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
			);
		`,
		Down: `DROP TABLE IF EXISTS server_settings`,
	})

	// Migration 5: API tokens (per-resource owner tokens; SHA-256 hashed)
	m.Register(Migration{
		Version:     5,
		Description: "Create api_tokens table",
		Up: `
			CREATE TABLE IF NOT EXISTS api_tokens (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				name TEXT NOT NULL,
				token_hash TEXT UNIQUE NOT NULL,
				token_prefix TEXT NOT NULL,
				description TEXT,
				permissions TEXT,
				rate_limit INTEGER DEFAULT 100,
				active INTEGER DEFAULT 1,
				last_used DATETIME,
				expires_at DATETIME,
				created_at DATETIME DEFAULT CURRENT_TIMESTAMP
			);
			CREATE INDEX idx_api_tokens_hash ON api_tokens(token_hash);
			CREATE INDEX idx_api_tokens_prefix ON api_tokens(token_prefix);
		`,
		Down: `DROP TABLE IF EXISTS api_tokens`,
	})

	// Migration 6: Search statistics
	m.Register(Migration{
		Version:     6,
		Description: "Create search_stats table",
		Up: `
			CREATE TABLE IF NOT EXISTS search_stats (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				date DATE NOT NULL,
				hour INTEGER NOT NULL,
				query_count INTEGER DEFAULT 0,
				result_count INTEGER DEFAULT 0,
				avg_response_time REAL DEFAULT 0,
				engines_used TEXT,
				categories TEXT,
				UNIQUE(date, hour)
			);
			CREATE INDEX idx_search_stats_date ON search_stats(date);
		`,
		Down: `DROP TABLE IF EXISTS search_stats`,
	})

	// Migration 7: Engine statistics
	m.Register(Migration{
		Version:     7,
		Description: "Create engine_stats table",
		Up: `
			CREATE TABLE IF NOT EXISTS engine_stats (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				date DATE NOT NULL,
				engine TEXT NOT NULL,
				query_count INTEGER DEFAULT 0,
				result_count INTEGER DEFAULT 0,
				error_count INTEGER DEFAULT 0,
				avg_response_time REAL DEFAULT 0,
				UNIQUE(date, engine)
			);
			CREATE INDEX idx_engine_stats_date ON engine_stats(date);
			CREATE INDEX idx_engine_stats_engine ON engine_stats(engine);
		`,
		Down: `DROP TABLE IF EXISTS engine_stats`,
	})

	// Migration 8: Blocked IPs
	m.Register(Migration{
		Version:     8,
		Description: "Create blocked_ips table",
		Up: `
			CREATE TABLE IF NOT EXISTS blocked_ips (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				ip_address TEXT UNIQUE NOT NULL,
				reason TEXT,
				blocked_by TEXT,
				expires_at DATETIME,
				created_at DATETIME DEFAULT CURRENT_TIMESTAMP
			);
			CREATE INDEX idx_blocked_ips_ip ON blocked_ips(ip_address);
		`,
		Down: `DROP TABLE IF EXISTS blocked_ips`,
	})

	// Migration 9: Custom bangs
	m.Register(Migration{
		Version:     9,
		Description: "Create custom_bangs table",
		Up: `
			CREATE TABLE IF NOT EXISTS custom_bangs (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				shortcut TEXT UNIQUE NOT NULL,
				name TEXT NOT NULL,
				url TEXT NOT NULL,
				category TEXT DEFAULT 'custom',
				description TEXT,
				active INTEGER DEFAULT 1,
				created_at DATETIME DEFAULT CURRENT_TIMESTAMP
			);
			CREATE INDEX idx_custom_bangs_shortcut ON custom_bangs(shortcut);
		`,
		Down: `DROP TABLE IF EXISTS custom_bangs`,
	})

	// Migration 10: Search alerts
	m.Register(Migration{
		Version:     10,
		Description: "Create search alert tables",
		Up: `
			CREATE TABLE IF NOT EXISTS search_alerts (
				id TEXT PRIMARY KEY,
				email TEXT NOT NULL,
				query TEXT NOT NULL,
				category TEXT NOT NULL,
				safe_search INTEGER NOT NULL DEFAULT 1,
				frequency TEXT NOT NULL,
				deliver_email INTEGER NOT NULL DEFAULT 0,
				deliver_rss INTEGER NOT NULL DEFAULT 1,
				deliver_webhook INTEGER NOT NULL DEFAULT 0,
				webhook_url TEXT,
				webhook_secret_encrypted TEXT,
				email_verified INTEGER NOT NULL DEFAULT 0,
				status TEXT NOT NULL DEFAULT 'pending',
				verify_token_hash TEXT,
				verify_token_expires DATETIME,
				manage_token_hash TEXT NOT NULL,
				rss_token_hash TEXT NOT NULL,
				base_url TEXT,
				last_checked_at DATETIME,
				last_sent_at DATETIME,
				last_error TEXT,
				created_from_ip TEXT,
				created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				verified_at DATETIME,
				paused_at DATETIME,
				language TEXT NOT NULL DEFAULT 'en',
				region TEXT NOT NULL DEFAULT '',
				engines_json TEXT NOT NULL DEFAULT '[]',
				manage_token_encrypted TEXT NOT NULL DEFAULT '',
				rss_token_encrypted TEXT NOT NULL DEFAULT ''
			);
			CREATE UNIQUE INDEX idx_search_alerts_manage_token ON search_alerts(manage_token_hash);
			CREATE UNIQUE INDEX idx_search_alerts_rss_token ON search_alerts(rss_token_hash);
			CREATE INDEX idx_search_alerts_verify_token ON search_alerts(verify_token_hash);
			CREATE INDEX idx_search_alerts_status_frequency ON search_alerts(status, frequency);

			CREATE TABLE IF NOT EXISTS search_alert_results (
				id TEXT PRIMARY KEY,
				alert_id TEXT NOT NULL,
				fingerprint TEXT NOT NULL,
				title TEXT NOT NULL,
				url TEXT NOT NULL,
				content TEXT,
				engine TEXT,
				published_at DATETIME,
				first_seen_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				notified_email_at DATETIME,
				notified_webhook_at DATETIME,
				FOREIGN KEY (alert_id) REFERENCES search_alerts(id) ON DELETE CASCADE,
				UNIQUE(alert_id, fingerprint)
			);
			CREATE INDEX idx_search_alert_results_alert ON search_alert_results(alert_id, first_seen_at);
		`,
		Down: `
			DROP TABLE IF EXISTS search_alert_results;
			DROP TABLE IF EXISTS search_alerts;
		`,
	})

	// Sort migrations by version
	sort.Slice(m.migrations, func(i, j int) bool {
		return m.migrations[i].Version < m.migrations[j].Version
	})
}

// registerUsersMigrations registers migrations for user.db.
//
// This project has no user account system; only a schema_version stub is
// created so existing DatabaseManager code paths keep working.
func (dbm *DatabaseMigrator) registerUsersMigrations() {
	m := dbm.usersMigrator

	m.Register(Migration{
		Version:     1,
		Description: "Create schema_version table",
		Up: `
			CREATE TABLE IF NOT EXISTS schema_version (
				version INTEGER PRIMARY KEY,
				description TEXT NOT NULL,
				applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
			)
		`,
		Down: `DROP TABLE IF EXISTS schema_version`,
	})

	sort.Slice(m.migrations, func(i, j int) bool {
		return m.migrations[i].Version < m.migrations[j].Version
	})
}

// MigrateAll runs migrations for both databases
func (dbm *DatabaseMigrator) MigrateAll(ctx context.Context) error {
	// Migrate server database
	if err := dbm.serverMigrator.Migrate(ctx); err != nil {
		return fmt.Errorf("server database migration failed: %w", err)
	}

	// Migrate users database
	if err := dbm.usersMigrator.Migrate(ctx); err != nil {
		return fmt.Errorf("users database migration failed: %w", err)
	}

	return nil
}

// GetServerMigrator returns the server database migrator
func (dbm *DatabaseMigrator) GetServerMigrator() *Migrator {
	return dbm.serverMigrator
}

// GetUsersMigrator returns the users database migrator
func (dbm *DatabaseMigrator) GetUsersMigrator() *Migrator {
	return dbm.usersMigrator
}

// NewMigrator creates a new migrator (legacy single-database mode)
func NewMigrator(db *DB) *Migrator {
	m := &Migrator{
		db:         db,
		migrations: make([]Migration, 0),
	}
	m.registerMigrations()
	return m
}

// registerMigrations registers all migrations (legacy single-database mode)
func (m *Migrator) registerMigrations() {
	m.Register(Migration{
		Version:     1,
		Description: "Create schema_version table",
		Up: `
			CREATE TABLE IF NOT EXISTS schema_version (
				version INTEGER PRIMARY KEY,
				description TEXT NOT NULL,
				applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
			)
		`,
		Down: `DROP TABLE IF EXISTS schema_version`,
	})

	sort.Slice(m.migrations, func(i, j int) bool {
		return m.migrations[i].Version < m.migrations[j].Version
	})
}

// Register adds a migration
func (m *Migrator) Register(migration Migration) {
	m.migrations = append(m.migrations, migration)
}

// Migrate runs all pending migrations
func (m *Migrator) Migrate(ctx context.Context) error {
	// Get current version
	currentVersion, err := m.getCurrentVersion(ctx)
	if err != nil {
		// Schema version table might not exist yet, try to create it
		if _, err := m.db.Exec(ctx, m.migrations[0].Up); err != nil {
			return fmt.Errorf("failed to create schema_version table: %w", err)
		}
		currentVersion = 1
	}

	// Apply pending migrations
	for _, migration := range m.migrations {
		if migration.Version <= currentVersion {
			continue
		}

		if err := m.applyMigration(ctx, migration); err != nil {
			return fmt.Errorf("migration %d failed: %w", migration.Version, err)
		}
	}

	return nil
}

// getCurrentVersion returns the current schema version
func (m *Migrator) getCurrentVersion(ctx context.Context) (int, error) {
	row := m.db.QueryRow(ctx, "SELECT COALESCE(MAX(version), 0) FROM schema_version")
	var version int
	if err := row.Scan(&version); err != nil {
		return 0, err
	}
	return version, nil
}

// applyMigration applies a single migration
func (m *Migrator) applyMigration(ctx context.Context, migration Migration) error {
	tx, err := m.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Execute migration
	if _, err := tx.ExecContext(ctx, migration.Up); err != nil {
		return fmt.Errorf("failed to execute migration: %w", err)
	}

	// Record migration
	if _, err := tx.ExecContext(ctx,
		"INSERT INTO schema_version (version, description, applied_at) VALUES (?, ?, ?)",
		migration.Version, migration.Description, time.Now()); err != nil {
		return fmt.Errorf("failed to record migration: %w", err)
	}

	return tx.Commit()
}

// Rollback rolls back the last migration
func (m *Migrator) Rollback(ctx context.Context) error {
	currentVersion, err := m.getCurrentVersion(ctx)
	if err != nil {
		return err
	}

	if currentVersion == 0 {
		return fmt.Errorf("no migrations to rollback")
	}

	// Find the migration to rollback
	for i := len(m.migrations) - 1; i >= 0; i-- {
		if m.migrations[i].Version == currentVersion {
			return m.rollbackMigration(ctx, m.migrations[i])
		}
	}

	return fmt.Errorf("migration %d not found", currentVersion)
}

// rollbackMigration rolls back a single migration
func (m *Migrator) rollbackMigration(ctx context.Context, migration Migration) error {
	tx, err := m.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Execute rollback
	if _, err := tx.ExecContext(ctx, migration.Down); err != nil {
		return fmt.Errorf("failed to rollback migration: %w", err)
	}

	// Remove migration record
	if _, err := tx.ExecContext(ctx,
		"DELETE FROM schema_version WHERE version = ?",
		migration.Version); err != nil {
		return fmt.Errorf("failed to remove migration record: %w", err)
	}

	return tx.Commit()
}

// GetVersion returns the current schema version
func (m *Migrator) GetVersion(ctx context.Context) (int, error) {
	return m.getCurrentVersion(ctx)
}

// GetMigrations returns all migrations
func (m *Migrator) GetMigrations() []Migration {
	return m.migrations
}
