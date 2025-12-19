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
	Up          string // SQL to apply
	Down        string // SQL to rollback
}

// Migrator handles database migrations
type Migrator struct {
	db         *DB
	migrations []Migration
}

// NewMigrator creates a new migrator
func NewMigrator(db *DB) *Migrator {
	m := &Migrator{
		db:         db,
		migrations: make([]Migration, 0),
	}
	m.registerMigrations()
	return m
}

// registerMigrations registers all migrations
func (m *Migrator) registerMigrations() {
	// Migration 1: Create schema version table
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

	// Migration 2: Create admin users table
	m.Register(Migration{
		Version:     2,
		Description: "Create admin_users table",
		Up: `
			CREATE TABLE IF NOT EXISTS admin_users (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				username TEXT UNIQUE NOT NULL,
				password_hash TEXT NOT NULL,
				email TEXT,
				role TEXT DEFAULT 'admin',
				active INTEGER DEFAULT 1,
				last_login DATETIME,
				created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
			);
			CREATE INDEX idx_admin_users_username ON admin_users(username);
		`,
		Down: `DROP TABLE IF EXISTS admin_users`,
	})

	// Migration 3: Create admin sessions table
	m.Register(Migration{
		Version:     3,
		Description: "Create admin_sessions table",
		Up: `
			CREATE TABLE IF NOT EXISTS admin_sessions (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				user_id INTEGER NOT NULL,
				token TEXT UNIQUE NOT NULL,
				ip_address TEXT,
				user_agent TEXT,
				expires_at DATETIME NOT NULL,
				created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				FOREIGN KEY (user_id) REFERENCES admin_users(id) ON DELETE CASCADE
			);
			CREATE INDEX idx_admin_sessions_token ON admin_sessions(token);
			CREATE INDEX idx_admin_sessions_expires ON admin_sessions(expires_at);
		`,
		Down: `DROP TABLE IF EXISTS admin_sessions`,
	})

	// Migration 4: Create API tokens table
	m.Register(Migration{
		Version:     4,
		Description: "Create api_tokens table",
		Up: `
			CREATE TABLE IF NOT EXISTS api_tokens (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				name TEXT NOT NULL,
				token TEXT UNIQUE NOT NULL,
				description TEXT,
				permissions TEXT,
				rate_limit INTEGER DEFAULT 100,
				active INTEGER DEFAULT 1,
				last_used DATETIME,
				expires_at DATETIME,
				created_at DATETIME DEFAULT CURRENT_TIMESTAMP
			);
			CREATE INDEX idx_api_tokens_token ON api_tokens(token);
		`,
		Down: `DROP TABLE IF EXISTS api_tokens`,
	})

	// Migration 5: Create search statistics table
	m.Register(Migration{
		Version:     5,
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

	// Migration 6: Create engine statistics table
	m.Register(Migration{
		Version:     6,
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

	// Migration 7: Create blocked IPs table
	m.Register(Migration{
		Version:     7,
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

	// Migration 8: Create custom bangs table
	m.Register(Migration{
		Version:     8,
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

	// Migration 9: Create audit log table
	m.Register(Migration{
		Version:     9,
		Description: "Create audit_log table",
		Up: `
			CREATE TABLE IF NOT EXISTS audit_log (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
				user_id INTEGER,
				action TEXT NOT NULL,
				resource TEXT,
				details TEXT,
				ip_address TEXT,
				user_agent TEXT
			);
			CREATE INDEX idx_audit_log_timestamp ON audit_log(timestamp);
			CREATE INDEX idx_audit_log_user ON audit_log(user_id);
			CREATE INDEX idx_audit_log_action ON audit_log(action);
		`,
		Down: `DROP TABLE IF EXISTS audit_log`,
	})

	// Migration 10: Create users table (for multi-user mode)
	m.Register(Migration{
		Version:     10,
		Description: "Create users table",
		Up: `
			CREATE TABLE IF NOT EXISTS users (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				username TEXT UNIQUE NOT NULL,
				email TEXT UNIQUE NOT NULL,
				password_hash TEXT NOT NULL,
				display_name TEXT,
				avatar_url TEXT,
				bio TEXT,
				role TEXT DEFAULT 'user',
				email_verified INTEGER DEFAULT 0,
				active INTEGER DEFAULT 1,
				created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				last_login DATETIME
			);
			CREATE INDEX idx_users_username ON users(username);
			CREATE INDEX idx_users_email ON users(email);
			CREATE INDEX idx_users_active ON users(active);
		`,
		Down: `DROP TABLE IF EXISTS users`,
	})

	// Migration 11: Create user sessions table
	m.Register(Migration{
		Version:     11,
		Description: "Create user_sessions table",
		Up: `
			CREATE TABLE IF NOT EXISTS user_sessions (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				user_id INTEGER NOT NULL,
				token TEXT UNIQUE NOT NULL,
				ip_address TEXT,
				user_agent TEXT,
				device_name TEXT,
				created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				expires_at DATETIME NOT NULL,
				last_used DATETIME DEFAULT CURRENT_TIMESTAMP,
				FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
			);
			CREATE INDEX idx_user_sessions_token ON user_sessions(token);
			CREATE INDEX idx_user_sessions_user ON user_sessions(user_id);
			CREATE INDEX idx_user_sessions_expires ON user_sessions(expires_at);
		`,
		Down: `DROP TABLE IF EXISTS user_sessions`,
	})

	// Migration 12: Create user 2FA table
	m.Register(Migration{
		Version:     12,
		Description: "Create user_2fa table",
		Up: `
			CREATE TABLE IF NOT EXISTS user_2fa (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				user_id INTEGER UNIQUE NOT NULL,
				secret_encrypted TEXT NOT NULL,
				enabled INTEGER DEFAULT 0,
				verified INTEGER DEFAULT 0,
				created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				enabled_at DATETIME,
				FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
			);
			CREATE INDEX idx_user_2fa_user ON user_2fa(user_id);
		`,
		Down: `DROP TABLE IF EXISTS user_2fa`,
	})

	// Migration 13: Create recovery keys table
	m.Register(Migration{
		Version:     13,
		Description: "Create recovery_keys table",
		Up: `
			CREATE TABLE IF NOT EXISTS recovery_keys (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				user_id INTEGER NOT NULL,
				key_hash TEXT NOT NULL,
				used INTEGER DEFAULT 0,
				created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				used_at DATETIME,
				FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
			);
			CREATE INDEX idx_recovery_keys_user ON recovery_keys(user_id);
			CREATE INDEX idx_recovery_keys_hash ON recovery_keys(key_hash);
		`,
		Down: `DROP TABLE IF EXISTS recovery_keys`,
	})

	// Migration 14: Create user API tokens table
	m.Register(Migration{
		Version:     14,
		Description: "Create user_tokens table",
		Up: `
			CREATE TABLE IF NOT EXISTS user_tokens (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				user_id INTEGER NOT NULL,
				name TEXT NOT NULL,
				token_hash TEXT UNIQUE NOT NULL,
				token_prefix TEXT NOT NULL,
				permissions TEXT,
				last_used DATETIME,
				expires_at DATETIME,
				created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
			);
			CREATE INDEX idx_user_tokens_user ON user_tokens(user_id);
			CREATE INDEX idx_user_tokens_hash ON user_tokens(token_hash);
		`,
		Down: `DROP TABLE IF EXISTS user_tokens`,
	})

	// Migration 15: Create verification tokens table
	m.Register(Migration{
		Version:     15,
		Description: "Create verification_tokens table",
		Up: `
			CREATE TABLE IF NOT EXISTS verification_tokens (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				user_id INTEGER NOT NULL,
				token TEXT UNIQUE NOT NULL,
				type TEXT NOT NULL,
				expires_at DATETIME NOT NULL,
				created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
			);
			CREATE INDEX idx_verification_tokens_token ON verification_tokens(token);
			CREATE INDEX idx_verification_tokens_user ON verification_tokens(user_id);
			CREATE INDEX idx_verification_tokens_type ON verification_tokens(type);
		`,
		Down: `DROP TABLE IF EXISTS verification_tokens`,
	})

	// Migration 16: Create user preferences table
	m.Register(Migration{
		Version:     16,
		Description: "Create user_preferences table",
		Up: `
			CREATE TABLE IF NOT EXISTS user_preferences (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				user_id INTEGER UNIQUE NOT NULL,
				preferences TEXT NOT NULL,
				created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
			);
			CREATE INDEX idx_user_preferences_user ON user_preferences(user_id);
		`,
		Down: `DROP TABLE IF EXISTS user_preferences`,
	})

	// Sort migrations by version
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
