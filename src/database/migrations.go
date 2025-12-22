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

// DatabaseMigrator handles migrations for both databases per TEMPLATE.md PART 24
type DatabaseMigrator struct {
	dm              *DatabaseManager
	serverMigrator  *Migrator
	usersMigrator   *Migrator
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

	// Create users database migrator
	dbm.usersMigrator = &Migrator{
		db:         dm.UsersDB(),
		migrations: make([]Migration, 0),
	}
	dbm.registerUsersMigrations()

	return dbm
}

// registerServerMigrations registers migrations for server.db
// Tables: admin_credentials, admin_sessions, scheduler_state, audit_log, etc.
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

	// Migration 2: Admin credentials (single row - primary server admin)
	m.Register(Migration{
		Version:     2,
		Description: "Create admin_credentials table",
		Up: `
			CREATE TABLE IF NOT EXISTS admin_credentials (
				id INTEGER PRIMARY KEY CHECK (id = 1),
				username TEXT UNIQUE NOT NULL,
				email TEXT,
				password_hash TEXT NOT NULL,
				token_hash TEXT,
				token_prefix TEXT,
				totp_secret TEXT,
				totp_enabled INTEGER DEFAULT 0,
				created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				last_login_at DATETIME
			);
		`,
		Down: `DROP TABLE IF EXISTS admin_credentials`,
	})

	// Migration 3: Admin sessions
	m.Register(Migration{
		Version:     3,
		Description: "Create admin_sessions table",
		Up: `
			CREATE TABLE IF NOT EXISTS admin_sessions (
				id TEXT PRIMARY KEY,
				token_hash TEXT UNIQUE NOT NULL,
				ip_address TEXT,
				user_agent TEXT,
				location TEXT,
				expires_at DATETIME NOT NULL,
				created_at DATETIME DEFAULT CURRENT_TIMESTAMP
			);
			CREATE INDEX idx_admin_sessions_expires ON admin_sessions(expires_at);
		`,
		Down: `DROP TABLE IF EXISTS admin_sessions`,
	})

	// Migration 4: Scheduler state
	m.Register(Migration{
		Version:     4,
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

	// Migration 5: Audit log
	m.Register(Migration{
		Version:     5,
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

	// Migration 6: Server settings
	m.Register(Migration{
		Version:     6,
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

	// Migration 7: API tokens (admin tokens)
	m.Register(Migration{
		Version:     7,
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

	// Migration 8: Search statistics
	m.Register(Migration{
		Version:     8,
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

	// Migration 9: Engine statistics
	m.Register(Migration{
		Version:     9,
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

	// Migration 10: Blocked IPs
	m.Register(Migration{
		Version:     10,
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

	// Migration 11: Custom bangs
	m.Register(Migration{
		Version:     11,
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

	// Migration 12: Multi-admin support per TEMPLATE.md PART 31
	// Recreate admin_credentials without CHECK constraint, add new columns
	m.Register(Migration{
		Version:     12,
		Description: "Add multi-admin support to admin_credentials",
		Up: `
			-- Create new table with multi-admin support
			CREATE TABLE IF NOT EXISTS admin_credentials_new (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				username TEXT UNIQUE NOT NULL,
				email TEXT UNIQUE,
				password_hash TEXT NOT NULL,
				token_hash TEXT,
				token_prefix TEXT,
				totp_secret TEXT,
				totp_enabled INTEGER DEFAULT 0,
				is_primary INTEGER DEFAULT 0,
				source TEXT DEFAULT 'local',
				external_id TEXT,
				created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				last_login_at DATETIME
			);

			-- Copy existing data, marking as primary admin
			INSERT INTO admin_credentials_new (
				id, username, email, password_hash, token_hash, token_prefix,
				totp_secret, totp_enabled, is_primary, source,
				created_at, updated_at, last_login_at
			)
			SELECT
				id, username, email, password_hash, token_hash, token_prefix,
				totp_secret, totp_enabled, 1, 'local',
				created_at, updated_at, last_login_at
			FROM admin_credentials;

			-- Drop old table and rename new one
			DROP TABLE admin_credentials;
			ALTER TABLE admin_credentials_new RENAME TO admin_credentials;

			-- Create indexes
			CREATE INDEX idx_admin_credentials_username ON admin_credentials(username);
			CREATE INDEX idx_admin_credentials_email ON admin_credentials(email);
			CREATE INDEX idx_admin_credentials_source ON admin_credentials(source);
		`,
		Down: `
			-- Revert to single-admin table
			CREATE TABLE IF NOT EXISTS admin_credentials_old (
				id INTEGER PRIMARY KEY CHECK (id = 1),
				username TEXT UNIQUE NOT NULL,
				email TEXT,
				password_hash TEXT NOT NULL,
				token_hash TEXT,
				token_prefix TEXT,
				totp_secret TEXT,
				totp_enabled INTEGER DEFAULT 0,
				created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				last_login_at DATETIME
			);

			INSERT INTO admin_credentials_old
			SELECT id, username, email, password_hash, token_hash, token_prefix,
				   totp_secret, totp_enabled, created_at, updated_at, last_login_at
			FROM admin_credentials WHERE is_primary = 1;

			DROP TABLE admin_credentials;
			ALTER TABLE admin_credentials_old RENAME TO admin_credentials;
		`,
	})

	// Migration 13: Admin invites table per TEMPLATE.md PART 31
	m.Register(Migration{
		Version:     13,
		Description: "Create admin_invites table",
		Up: `
			CREATE TABLE IF NOT EXISTS admin_invites (
				id TEXT PRIMARY KEY,
				token_hash TEXT UNIQUE NOT NULL,
				username TEXT,
				created_by INTEGER NOT NULL,
				expires_at DATETIME NOT NULL,
				used_at DATETIME,
				used_by INTEGER,
				created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				FOREIGN KEY (created_by) REFERENCES admin_credentials(id),
				FOREIGN KEY (used_by) REFERENCES admin_credentials(id)
			);
			CREATE INDEX idx_admin_invites_token ON admin_invites(token_hash);
			CREATE INDEX idx_admin_invites_expires ON admin_invites(expires_at);
		`,
		Down: `DROP TABLE IF EXISTS admin_invites`,
	})

	// Migration 14: Setup token table for --maintenance setup command
	m.Register(Migration{
		Version:     14,
		Description: "Create setup_token table",
		Up: `
			CREATE TABLE IF NOT EXISTS setup_token (
				id INTEGER PRIMARY KEY CHECK (id = 1),
				token_hash TEXT NOT NULL,
				created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				expires_at DATETIME,
				used_at DATETIME
			);
		`,
		Down: `DROP TABLE IF EXISTS setup_token`,
	})

	// Migration 15: External admins table for OIDC/LDAP per TEMPLATE.md PART 31
	m.Register(Migration{
		Version:     15,
		Description: "Create external_admins table",
		Up: `
			CREATE TABLE IF NOT EXISTS external_admins (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				provider_type TEXT NOT NULL,
				provider_id TEXT NOT NULL,
				external_id TEXT NOT NULL,
				username TEXT NOT NULL,
				email TEXT,
				groups_json TEXT,
				is_admin INTEGER DEFAULT 0,
				cached_at DATETIME NOT NULL,
				last_login_at DATETIME,
				created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				UNIQUE(provider_type, provider_id, external_id)
			);
			CREATE INDEX idx_external_admins_provider ON external_admins(provider_type, provider_id);
			CREATE INDEX idx_external_admins_external ON external_admins(external_id);
		`,
		Down: `DROP TABLE IF EXISTS external_admins`,
	})

	// Sort migrations by version
	sort.Slice(m.migrations, func(i, j int) bool {
		return m.migrations[i].Version < m.migrations[j].Version
	})
}

// registerUsersMigrations registers migrations for users.db
// Tables: users, user_sessions, user_2fa, recovery_keys, user_tokens, etc.
func (dbm *DatabaseMigrator) registerUsersMigrations() {
	m := dbm.usersMigrator

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

	// Migration 2: Users table
	m.Register(Migration{
		Version:     2,
		Description: "Create users table",
		Up: `
			CREATE TABLE IF NOT EXISTS users (
				id TEXT PRIMARY KEY,
				username TEXT UNIQUE NOT NULL,
				email TEXT UNIQUE NOT NULL,
				password_hash TEXT NOT NULL,
				display_name TEXT,
				avatar_url TEXT,
				bio TEXT,
				role TEXT DEFAULT 'user',
				email_verified INTEGER DEFAULT 0,
				approved INTEGER DEFAULT 1,
				disabled INTEGER DEFAULT 0,
				totp_secret TEXT,
				totp_enabled INTEGER DEFAULT 0,
				timezone TEXT,
				language TEXT,
				created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				last_login_at DATETIME
			);
			CREATE INDEX idx_users_username ON users(username);
			CREATE INDEX idx_users_email ON users(email);
			CREATE INDEX idx_users_disabled ON users(disabled);
		`,
		Down: `DROP TABLE IF EXISTS users`,
	})

	// Migration 3: User sessions
	m.Register(Migration{
		Version:     3,
		Description: "Create user_sessions table",
		Up: `
			CREATE TABLE IF NOT EXISTS user_sessions (
				id TEXT PRIMARY KEY,
				user_id TEXT NOT NULL,
				token_hash TEXT UNIQUE NOT NULL,
				ip_address TEXT,
				user_agent TEXT,
				location TEXT,
				device_name TEXT,
				created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				expires_at DATETIME NOT NULL,
				last_used DATETIME DEFAULT CURRENT_TIMESTAMP,
				FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
			);
			CREATE INDEX idx_user_sessions_hash ON user_sessions(token_hash);
			CREATE INDEX idx_user_sessions_user ON user_sessions(user_id);
			CREATE INDEX idx_user_sessions_expires ON user_sessions(expires_at);
		`,
		Down: `DROP TABLE IF EXISTS user_sessions`,
	})

	// Migration 4: User 2FA
	m.Register(Migration{
		Version:     4,
		Description: "Create user_2fa table",
		Up: `
			CREATE TABLE IF NOT EXISTS user_2fa (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				user_id TEXT UNIQUE NOT NULL,
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

	// Migration 5: Recovery keys
	m.Register(Migration{
		Version:     5,
		Description: "Create recovery_keys table",
		Up: `
			CREATE TABLE IF NOT EXISTS recovery_keys (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				user_id TEXT NOT NULL,
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

	// Migration 6: User API tokens
	m.Register(Migration{
		Version:     6,
		Description: "Create user_tokens table",
		Up: `
			CREATE TABLE IF NOT EXISTS user_tokens (
				id TEXT PRIMARY KEY,
				user_id TEXT NOT NULL,
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

	// Migration 7: Verification tokens
	m.Register(Migration{
		Version:     7,
		Description: "Create verification_tokens table",
		Up: `
			CREATE TABLE IF NOT EXISTS verification_tokens (
				id TEXT PRIMARY KEY,
				user_id TEXT NOT NULL,
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

	// Migration 8: User preferences
	m.Register(Migration{
		Version:     8,
		Description: "Create user_preferences table",
		Up: `
			CREATE TABLE IF NOT EXISTS user_preferences (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				user_id TEXT UNIQUE NOT NULL,
				preferences TEXT NOT NULL,
				created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
			);
			CREATE INDEX idx_user_preferences_user ON user_preferences(user_id);
		`,
		Down: `DROP TABLE IF EXISTS user_preferences`,
	})

	// Migration 9: Invites table
	m.Register(Migration{
		Version:     9,
		Description: "Create invites table",
		Up: `
			CREATE TABLE IF NOT EXISTS invites (
				id TEXT PRIMARY KEY,
				code TEXT UNIQUE NOT NULL,
				role TEXT DEFAULT 'user',
				max_uses INTEGER DEFAULT 1,
				use_count INTEGER DEFAULT 0,
				expires_at DATETIME,
				created_by TEXT,
				created_at DATETIME DEFAULT CURRENT_TIMESTAMP
			);
			CREATE INDEX idx_invites_code ON invites(code);
		`,
		Down: `DROP TABLE IF EXISTS invites`,
	})

	// Migration 10: Passkeys table per TEMPLATE.md PART 31
	m.Register(Migration{
		Version:     10,
		Description: "Create passkeys table for WebAuthn/FIDO2",
		Up: `
			CREATE TABLE IF NOT EXISTS passkeys (
				id TEXT PRIMARY KEY,
				user_id TEXT NOT NULL,
				credential_id TEXT UNIQUE NOT NULL,
				public_key TEXT NOT NULL,
				attestation_type TEXT DEFAULT 'none',
				transport TEXT,
				aaguid TEXT,
				sign_count INTEGER DEFAULT 0,
				name TEXT NOT NULL,
				created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				last_used_at DATETIME,
				FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
			);
			CREATE INDEX idx_passkeys_user ON passkeys(user_id);
			CREATE INDEX idx_passkeys_credential ON passkeys(credential_id);
		`,
		Down: `DROP TABLE IF EXISTS passkeys`,
	})

	// Migration 11: Passkey challenges table
	m.Register(Migration{
		Version:     11,
		Description: "Create passkey_challenges table",
		Up: `
			CREATE TABLE IF NOT EXISTS passkey_challenges (
				id TEXT PRIMARY KEY,
				user_id TEXT,
				challenge TEXT NOT NULL,
				type TEXT NOT NULL,
				expires_at DATETIME NOT NULL,
				created_at DATETIME DEFAULT CURRENT_TIMESTAMP
			);
			CREATE INDEX idx_passkey_challenges_user ON passkey_challenges(user_id);
			CREATE INDEX idx_passkey_challenges_expires ON passkey_challenges(expires_at);
		`,
		Down: `DROP TABLE IF EXISTS passkey_challenges`,
	})

	// Migration 12: Dual email support per TEMPLATE.md PART 31
	// Account email (security) vs Notification email (non-security)
	m.Register(Migration{
		Version:     12,
		Description: "Add notification email support for dual email",
		Up: `
			ALTER TABLE users ADD COLUMN notification_email TEXT;
			ALTER TABLE users ADD COLUMN notification_email_verified INTEGER DEFAULT 0;
		`,
		Down: `
			-- SQLite doesn't support DROP COLUMN directly
			-- These columns will be ignored if not used
		`,
	})

	// Migration 13: User additional emails table for multiple verified emails
	m.Register(Migration{
		Version:     13,
		Description: "Create user_emails table for additional emails",
		Up: `
			CREATE TABLE IF NOT EXISTS user_emails (
				id TEXT PRIMARY KEY,
				user_id TEXT NOT NULL,
				email TEXT UNIQUE NOT NULL,
				verified INTEGER DEFAULT 0,
				is_primary INTEGER DEFAULT 0,
				is_notification INTEGER DEFAULT 0,
				verification_token TEXT,
				verification_expires DATETIME,
				created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				verified_at DATETIME,
				FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
			);
			CREATE INDEX idx_user_emails_user ON user_emails(user_id);
			CREATE INDEX idx_user_emails_email ON user_emails(email);
			CREATE INDEX idx_user_emails_token ON user_emails(verification_token);
		`,
		Down: `DROP TABLE IF EXISTS user_emails`,
	})

	// Sort migrations by version
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
