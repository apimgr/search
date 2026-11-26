package database

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// InitSchema creates all database tables idempotently on startup.
// All statements use CREATE TABLE IF NOT EXISTS / CREATE INDEX IF NOT EXISTS so
// the schema is self-creating and safe to run on every startup. Per AI.md
// PART 10, schema changes are additive and idempotent only: any future column
// additions use ALTER TABLE ADD COLUMN with the "duplicate column" error
// ignored — never DROP TABLE, DROP COLUMN, or DELETE.
// Safe to call on every startup — never drops data.
func InitSchema(ctx context.Context, dm *DatabaseManager) error {
	if err := initServerSchema(ctx, dm.ServerDB()); err != nil {
		return fmt.Errorf("server database schema init failed: %w", err)
	}
	if err := initUsersSchema(ctx, dm.UsersDB()); err != nil {
		return fmt.Errorf("users database schema init failed: %w", err)
	}
	return nil
}

// serverTablePrefix returns the table prefix for server tables.
// Per AI.md PART 10: remote/libsql uses "srv_" prefix, local sqlite uses no prefix.
func serverTablePrefix(db *DB) string {
	if db.driver == "libsql" {
		return "srv_"
	}
	return ""
}

// initServerSchema creates all tables for server.db.
// Per AI.md PART 10: Server tables use srv_ prefix when using libSQL/Turso remote database.
func initServerSchema(ctx context.Context, db *DB) error {
	prefix := serverTablePrefix(db)

	// applyPrefix replaces {prefix} placeholders with the actual prefix
	applyPrefix := func(sql string) string {
		return strings.ReplaceAll(sql, "{prefix}", prefix)
	}

	statements := []string{
		`CREATE TABLE IF NOT EXISTS {prefix}owners (
			id TEXT PRIMARY KEY,
			created_at INTEGER NOT NULL DEFAULT (strftime('%s', 'now')),
			last_seen_at INTEGER NOT NULL DEFAULT (strftime('%s', 'now'))
		)`,
		// Scheduler task definitions — must match scheduler.go schema exactly
		`CREATE TABLE IF NOT EXISTS {prefix}scheduler_tasks (
			task_id TEXT PRIMARY KEY,
			task_name TEXT,
			schedule TEXT,
			last_run DATETIME,
			last_status TEXT,
			last_error TEXT,
			next_run DATETIME,
			run_count INTEGER DEFAULT 0,
			fail_count INTEGER DEFAULT 0,
			enabled INTEGER DEFAULT 1,
			retry_count INTEGER DEFAULT 0,
			next_retry DATETIME,
			locked_by TEXT,
			locked_at DATETIME
		)`,
		// Scheduler execution history
		`CREATE TABLE IF NOT EXISTS {prefix}scheduler_history (
			id TEXT PRIMARY KEY,
			task_id TEXT NOT NULL,
			started_at DATETIME NOT NULL,
			finished_at DATETIME,
			success INTEGER NOT NULL DEFAULT 0,
			error TEXT
		)`,
		`CREATE INDEX IF NOT EXISTS {prefix}idx_scheduler_history_task ON {prefix}scheduler_history(task_id, started_at)`,
		// Audit log
		`CREATE TABLE IF NOT EXISTS {prefix}audit_log (
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
		)`,
		`CREATE INDEX IF NOT EXISTS {prefix}idx_audit_log_timestamp ON {prefix}audit_log(timestamp)`,
		`CREATE INDEX IF NOT EXISTS {prefix}idx_audit_log_actor ON {prefix}audit_log(actor_type, actor_id)`,
		`CREATE INDEX IF NOT EXISTS {prefix}idx_audit_log_action ON {prefix}audit_log(action)`,
		`CREATE INDEX IF NOT EXISTS {prefix}idx_audit_log_resource ON {prefix}audit_log(resource_type, resource_id)`,
		// Configuration key-value store
		`CREATE TABLE IF NOT EXISTS {prefix}config (
			key TEXT PRIMARY KEY,
			value TEXT,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		// Configuration defaults and restart flags
		`CREATE TABLE IF NOT EXISTS {prefix}config_meta (
			key TEXT PRIMARY KEY,
			default_value TEXT,
			requires_restart INTEGER NOT NULL DEFAULT 0
		)`,
		// Rate limiting counters
		`CREATE TABLE IF NOT EXISTS {prefix}rate_limits (
			ip TEXT NOT NULL,
			endpoint TEXT NOT NULL,
			count INTEGER NOT NULL DEFAULT 0,
			window_start DATETIME NOT NULL,
			PRIMARY KEY (ip, endpoint)
		)`,
		// Backup metadata
		`CREATE TABLE IF NOT EXISTS {prefix}backups (
			id TEXT PRIMARY KEY,
			path TEXT NOT NULL,
			size INTEGER NOT NULL DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			type TEXT NOT NULL,
			verified INTEGER NOT NULL DEFAULT 0
		)`,
		// Owner and linked tokens. Existing installations are upgraded below
		// without deleting legacy token IDs or credential data.
		`CREATE TABLE IF NOT EXISTS {prefix}api_tokens (
			id TEXT PRIMARY KEY,
			owner_id TEXT NOT NULL REFERENCES {prefix}owners(id) ON DELETE CASCADE,
			kind TEXT NOT NULL CHECK (kind IN ('primary', 'linked')),
			name TEXT,
			token_hash TEXT NOT NULL UNIQUE,
			token_prefix TEXT NOT NULL,
			token_last4 TEXT NOT NULL,
			token_enc TEXT NOT NULL,
			created_at INTEGER NOT NULL DEFAULT (strftime('%s', 'now')),
			rotated_at INTEGER,
			expires_at INTEGER,
			last_used_at INTEGER,
			revoked_at INTEGER,
			revoked_reason TEXT
		)`,
		`CREATE INDEX IF NOT EXISTS {prefix}idx_api_tokens_hash ON {prefix}api_tokens(token_hash)`,
		`CREATE INDEX IF NOT EXISTS {prefix}idx_api_tokens_prefix ON {prefix}api_tokens(token_prefix)`,
		// Search statistics
		`CREATE TABLE IF NOT EXISTS {prefix}search_stats (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			date DATE NOT NULL,
			hour INTEGER NOT NULL,
			query_count INTEGER DEFAULT 0,
			result_count INTEGER DEFAULT 0,
			avg_response_time REAL DEFAULT 0,
			engines_used TEXT,
			categories TEXT,
			UNIQUE(date, hour)
		)`,
		`CREATE INDEX IF NOT EXISTS {prefix}idx_search_stats_date ON {prefix}search_stats(date)`,
		// Engine statistics
		`CREATE TABLE IF NOT EXISTS {prefix}engine_stats (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			date DATE NOT NULL,
			engine TEXT NOT NULL,
			query_count INTEGER DEFAULT 0,
			result_count INTEGER DEFAULT 0,
			error_count INTEGER DEFAULT 0,
			avg_response_time REAL DEFAULT 0,
			UNIQUE(date, engine)
		)`,
		`CREATE INDEX IF NOT EXISTS {prefix}idx_engine_stats_date ON {prefix}engine_stats(date)`,
		`CREATE INDEX IF NOT EXISTS {prefix}idx_engine_stats_engine ON {prefix}engine_stats(engine)`,
		// Blocked IPs
		`CREATE TABLE IF NOT EXISTS {prefix}blocked_ips (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			ip_address TEXT UNIQUE NOT NULL,
			reason TEXT,
			blocked_by TEXT,
			expires_at DATETIME,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS {prefix}idx_blocked_ips_ip ON {prefix}blocked_ips(ip_address)`,
		// Custom bangs
		`CREATE TABLE IF NOT EXISTS {prefix}custom_bangs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			shortcut TEXT UNIQUE NOT NULL,
			name TEXT NOT NULL,
			url TEXT NOT NULL,
			category TEXT DEFAULT 'custom',
			description TEXT,
			active INTEGER DEFAULT 1,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS {prefix}idx_custom_bangs_shortcut ON {prefix}custom_bangs(shortcut)`,
		// Search alerts
		`CREATE TABLE IF NOT EXISTS {prefix}search_alerts (
			id TEXT PRIMARY KEY,
			owner_id TEXT REFERENCES {prefix}owners(id),
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
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS {prefix}idx_search_alerts_manage_token ON {prefix}search_alerts(manage_token_hash)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS {prefix}idx_search_alerts_rss_token ON {prefix}search_alerts(rss_token_hash)`,
		`CREATE INDEX IF NOT EXISTS {prefix}idx_search_alerts_verify_token ON {prefix}search_alerts(verify_token_hash)`,
		`CREATE INDEX IF NOT EXISTS {prefix}idx_search_alerts_status_frequency ON {prefix}search_alerts(status, frequency)`,
		// Search alert results
		`CREATE TABLE IF NOT EXISTS {prefix}search_alert_results (
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
			FOREIGN KEY (alert_id) REFERENCES {prefix}search_alerts(id) ON DELETE CASCADE,
			UNIQUE(alert_id, fingerprint)
		)`,
		`CREATE INDEX IF NOT EXISTS {prefix}idx_search_alert_results_alert ON {prefix}search_alert_results(alert_id, first_seen_at)`,
		// Local QR short links store only AES-GCM encrypted destinations.
		`CREATE TABLE IF NOT EXISTS {prefix}qr_shortlinks (
			code TEXT PRIMARY KEY,
			target_encrypted TEXT NOT NULL,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		// Security reports — coordinated disclosure pipeline per AI.md PART 11.
		// Report bodies and researcher contact details are encrypted separately.
		`CREATE TABLE IF NOT EXISTS {prefix}security_reports (
			tracking_id TEXT PRIMARY KEY,
			id_window BIGINT NOT NULL,
			severity VARCHAR(16) NOT NULL,
			component VARCHAR(255) NOT NULL,
			status VARCHAR(16) NOT NULL DEFAULT 'received',
			body_enc BLOB NOT NULL,
			body_alg VARCHAR(16) NOT NULL,
			contact_enc BLOB,
			credit_pref VARCHAR(8) NOT NULL DEFAULT 'no',
			credit_name TEXT,
			researcher_comment TEXT NOT NULL DEFAULT '',
			disclosure_date DATE,
			cve_requested BOOLEAN NOT NULL DEFAULT FALSE,
			token_hash VARCHAR(64) NOT NULL,
			token_last_used DATE,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			closed_at DATETIME
		)`,
		`CREATE TABLE IF NOT EXISTS {prefix}security_thanks (
			tracking_id VARCHAR(32) PRIMARY KEY,
			credit VARCHAR(120) NOT NULL,
			severity VARCHAR(16) NOT NULL,
			disclosed_on DATE NOT NULL
		)`,
		// PGP keypair metadata — the keys themselves live on disk, never in the DB.
		`CREATE TABLE IF NOT EXISTS {prefix}pgp_keypair (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			fingerprint TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			expires_at DATETIME,
			last_rotated_at DATETIME,
			keyservers_published TEXT,
			revoked INTEGER NOT NULL DEFAULT 0
		)`,
		// Project-level cryptographic secrets per AI.md PART 11 "Cryptographic
		// Keys": installation_secret, cookie_signing_key and csrf_token_secret
		// live in server.db (not server.yml) because each has its own rotation
		// lifecycle. Values are base64-encoded; previous_value holds the prior
		// secret during its grace window so in-flight HMACs still validate.
		`CREATE TABLE IF NOT EXISTS {prefix}app_secrets (
			name TEXT PRIMARY KEY,
			value TEXT NOT NULL,
			previous_value TEXT,
			previous_expires_at DATETIME,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			rotated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS {prefix}server_request_stats_hourly (
			hour INTEGER PRIMARY KEY,
			request_count INTEGER NOT NULL DEFAULT 0
		)`,
	}
	for _, stmt := range statements {
		sql := applyPrefix(stmt)
		if _, err := db.Exec(ctx, sql); err != nil {
			return fmt.Errorf("schema statement failed: %w\nSQL: %s", err, sql)
		}
	}
	if err := ensureColumn(ctx, db, prefix+"search_alerts", "owner_id", "TEXT REFERENCES "+prefix+"owners(id)"); err != nil {
		return fmt.Errorf("add search alert owner column: %w", err)
	}
	if err := ensureSecurityReportColumns(ctx, db, prefix); err != nil {
		return err
	}
	if _, err := db.Exec(ctx, `CREATE INDEX IF NOT EXISTS `+prefix+`idx_security_reports_status ON `+prefix+`security_reports(status, created_at)`); err != nil {
		return fmt.Errorf("create security report status index: %w", err)
	}
	if _, err := db.Exec(ctx, `CREATE INDEX IF NOT EXISTS `+prefix+`idx_security_reports_token ON `+prefix+`security_reports(token_hash)`); err != nil {
		return fmt.Errorf("create security report token index: %w", err)
	}
	if _, err := db.Exec(ctx, `CREATE INDEX IF NOT EXISTS `+prefix+`idx_search_alerts_owner ON `+prefix+`search_alerts(owner_id)`); err != nil {
		return fmt.Errorf("create search alert owner index: %w", err)
	}
	apiTokens := prefix + "api_tokens"
	if err := migrateOwnerTokenIdentifiers(ctx, db, apiTokens); err != nil {
		return fmt.Errorf("migrate owner token identifiers: %w", err)
	}
	ownerTokenColumns := []struct {
		name string
		ddl  string
	}{
		{"owner_id", "TEXT REFERENCES " + prefix + "owners(id)"},
		{"kind", "TEXT NOT NULL DEFAULT 'primary' CHECK (kind IN ('primary', 'linked'))"},
		{"token_last4", "TEXT NOT NULL DEFAULT ''"},
		{"token_enc", "TEXT NOT NULL DEFAULT ''"},
		{"rotated_at", "INTEGER"},
		{"last_used_at", "INTEGER"},
		{"revoked_at", "INTEGER"},
		{"revoked_reason", "TEXT"},
	}
	for _, column := range ownerTokenColumns {
		if err := ensureColumn(ctx, db, apiTokens, column.name, column.ddl); err != nil {
			return fmt.Errorf("add owner token column %s: %w", column.name, err)
		}
	}
	if active, err := hasColumn(ctx, db, apiTokens, "active"); err != nil {
		return fmt.Errorf("inspect legacy owner token activity: %w", err)
	} else if active {
		if _, err := db.Exec(ctx, `UPDATE `+apiTokens+` SET revoked_at = COALESCE(revoked_at, strftime('%s', 'now')), revoked_reason = COALESCE(revoked_reason, 'legacy_inactive') WHERE active = 0`); err != nil {
			return fmt.Errorf("preserve legacy inactive owner token state: %w", err)
		}
	}
	for _, stmt := range []string{
		`CREATE INDEX IF NOT EXISTS ` + prefix + `idx_api_tokens_owner ON ` + apiTokens + `(owner_id)`,
		`CREATE INDEX IF NOT EXISTS ` + prefix + `idx_api_tokens_active ON ` + apiTokens + `(revoked_at) WHERE revoked_at IS NULL`,
		`CREATE UNIQUE INDEX IF NOT EXISTS ` + prefix + `idx_api_tokens_primary_owner ON ` + apiTokens + `(owner_id) WHERE kind = 'primary' AND revoked_at IS NULL AND owner_id IS NOT NULL`,
		`CREATE UNIQUE INDEX IF NOT EXISTS ` + prefix + `idx_api_tokens_linked_name ON ` + apiTokens + `(owner_id, name) WHERE kind = 'linked' AND revoked_at IS NULL`,
	} {
		if _, err := db.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("owner token index failed: %w\nSQL: %s", err, stmt)
		}
	}
	return nil
}

// migrateOwnerTokenIdentifiers moves legacy sequential token IDs out of the
// public id column and backfills UUIDv7 identifiers without deleting rows.
// The legacy integer primary key stays available for existing foreign keys.
func migrateOwnerTokenIdentifiers(ctx context.Context, db *DB, table string) error {
	columns, err := tableColumns(ctx, db, table)
	if err != nil {
		return err
	}
	idType, hasID := columns["id"]
	if !hasID {
		return fmt.Errorf("%s has no id column", table)
	}
	if strings.EqualFold(idType, "INTEGER") {
		if _, hasLegacyID := columns["legacy_id"]; !hasLegacyID {
			if _, err := db.Exec(ctx, `ALTER TABLE `+table+` RENAME COLUMN id TO legacy_id`); err != nil {
				return fmt.Errorf("preserve legacy integer token IDs: %w", err)
			}
			delete(columns, "id")
			columns["legacy_id"] = idType
		}
		if _, hasPublicID := columns["id"]; !hasPublicID {
			if _, err := db.Exec(ctx, `ALTER TABLE `+table+` ADD COLUMN id TEXT`); err != nil {
				return fmt.Errorf("add public UUID token ID: %w", err)
			}
			columns["id"] = "TEXT"
		}
	}
	if err := ensureColumn(ctx, db, table, "token_last4", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	orderColumn := "id"
	if _, hasLegacyID := columns["legacy_id"]; hasLegacyID {
		orderColumn = "legacy_id"
	}
	rows, err := db.Query(ctx, `SELECT `+orderColumn+` FROM `+table+` WHERE id IS NULL OR id = '' ORDER BY `+orderColumn)
	if err != nil {
		return fmt.Errorf("read token rows needing UUID IDs: %w", err)
	}
	var legacyIDs []interface{}
	for rows.Next() {
		var legacyID interface{}
		if err := rows.Scan(&legacyID); err != nil {
			return errors.Join(err, rows.Close())
		}
		legacyIDs = append(legacyIDs, legacyID)
	}
	if err := rows.Err(); err != nil {
		return errors.Join(err, rows.Close())
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, legacyID := range legacyIDs {
		id, err := uuid.NewV7()
		if err != nil {
			return fmt.Errorf("generate UUIDv7 token identifier: %w", err)
		}
		if _, err := db.Exec(ctx, `UPDATE `+table+` SET id = ? WHERE `+orderColumn+` = ? AND (id IS NULL OR id = '')`, id.String(), legacyID); err != nil {
			return fmt.Errorf("backfill UUIDv7 token identifier: %w", err)
		}
	}
	if _, err := db.Exec(ctx, `CREATE UNIQUE INDEX IF NOT EXISTS `+table+`_public_id ON `+table+`(id)`); err != nil {
		return fmt.Errorf("index public UUID token identifiers: %w", err)
	}
	return nil
}

func tableColumns(ctx context.Context, db *DB, table string) (map[string]string, error) {
	rows, err := db.Query(ctx, `PRAGMA table_info(`+table+`)`)
	if err != nil {
		return nil, err
	}
	columns := make(map[string]string)
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, columnType string
		var defaultValue interface{}
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			return nil, errors.Join(err, rows.Close())
		}
		columns[name] = columnType
	}
	if err := rows.Err(); err != nil {
		return nil, errors.Join(err, rows.Close())
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	return columns, nil
}

func ensureSecurityReportColumns(ctx context.Context, db *DB, prefix string) error {
	table := prefix + "security_reports"
	columns := []struct{ name, definition string }{
		{"id_window", "BIGINT NOT NULL DEFAULT 0"},
		{"severity", "VARCHAR(16) NOT NULL DEFAULT 'informational'"},
		{"component", "VARCHAR(255) NOT NULL DEFAULT ''"},
		{"status", "VARCHAR(16) NOT NULL DEFAULT 'received'"},
		{"body_enc", "BLOB"},
		{"body_alg", "VARCHAR(16) NOT NULL DEFAULT 'aes-256-gcm'"},
		{"contact_enc", "BLOB"},
		{"credit_pref", "VARCHAR(8) NOT NULL DEFAULT 'no'"},
		{"credit_name", "VARCHAR(120)"},
		{"researcher_comment", "TEXT NOT NULL DEFAULT ''"},
		{"disclosure_date", "DATE"},
		{"cve_requested", "BOOLEAN NOT NULL DEFAULT FALSE"},
		{"token_hash", "VARCHAR(64) NOT NULL DEFAULT ''"},
		{"token_last_used", "DATE"},
		{"created_at", "TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP"},
		{"updated_at", "TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP"},
		{"closed_at", "TIMESTAMP"},
	}
	for _, column := range columns {
		if err := ensureColumn(ctx, db, table, column.name, column.definition); err != nil {
			return fmt.Errorf("add security report %s column: %w", column.name, err)
		}
	}
	legacy := map[string]bool{}
	for _, column := range []string{"security_id_used", "affected_component", "affected_endpoint", "summary", "encrypted_body", "encryption_method", "credit_preference", "report_token_hash", "last_status_access_date", "disclosure_days"} {
		exists, err := hasColumn(ctx, db, table, column)
		if err != nil {
			return fmt.Errorf("inspect legacy security report %s column: %w", column, err)
		}
		legacy[column] = exists
	}
	if legacy["encrypted_body"] {
		if _, err := db.Exec(ctx, `UPDATE `+table+` SET body_enc = encrypted_body WHERE body_enc IS NULL AND encrypted_body IS NOT NULL`); err != nil {
			return fmt.Errorf("copy legacy security report body: %w", err)
		}
	}
	if legacy["encryption_method"] {
		if _, err := db.Exec(ctx, `UPDATE `+table+` SET body_alg = encryption_method WHERE encryption_method IS NOT NULL AND (body_enc = encrypted_body OR body_alg = 'aes-256-gcm')`); err != nil {
			return fmt.Errorf("copy legacy security report body algorithm: %w", err)
		}
	}
	if legacy["affected_component"] {
		if _, err := db.Exec(ctx, `UPDATE `+table+` SET component = affected_component WHERE component = '' AND affected_component IS NOT NULL`); err != nil {
			return fmt.Errorf("copy legacy security report component: %w", err)
		}
	}
	if legacy["report_token_hash"] {
		if _, err := db.Exec(ctx, `UPDATE `+table+` SET token_hash = report_token_hash WHERE token_hash = '' AND report_token_hash IS NOT NULL`); err != nil {
			return fmt.Errorf("copy legacy security report token hash: %w", err)
		}
	}
	if legacy["last_status_access_date"] {
		if _, err := db.Exec(ctx, `UPDATE `+table+` SET token_last_used = last_status_access_date WHERE token_last_used IS NULL`); err != nil {
			return fmt.Errorf("copy legacy security report access date: %w", err)
		}
	}
	if legacy["credit_preference"] {
		if _, err := db.Exec(ctx, `UPDATE `+table+` SET credit_pref = CASE credit_preference WHEN 'real_name' THEN 'name' WHEN 'anonymous' THEN 'anon' ELSE credit_preference END WHERE credit_preference IS NOT NULL`); err != nil {
			return fmt.Errorf("copy legacy security report credit preference: %w", err)
		}
	}
	if legacy["disclosure_days"] {
		if _, err := db.Exec(ctx, `UPDATE `+table+` SET disclosure_date = date(created_at, '+' || disclosure_days || ' days') WHERE disclosure_date IS NULL AND disclosure_days > 0`); err != nil {
			return fmt.Errorf("copy legacy security report disclosure date: %w", err)
		}
	}
	return nil
}

func hasColumn(ctx context.Context, db *DB, table, column string) (bool, error) {
	rows, err := db.Query(ctx, "PRAGMA table_info("+table+")")
	if err != nil {
		return false, err
	}
	found := false
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, columnType string
		var defaultValue interface{}
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			return false, errors.Join(err, rows.Close())
		}
		if name == column {
			found = true
		}
	}
	if err := rows.Err(); err != nil {
		return false, errors.Join(err, rows.Close())
	}
	if err := rows.Close(); err != nil {
		return false, err
	}
	return found, nil
}

func ensureColumn(ctx context.Context, db *DB, table, column, definition string) error {
	found, err := hasColumn(ctx, db, table, column)
	if err != nil {
		return err
	}
	if found {
		return nil
	}
	_, err = db.Exec(ctx, "ALTER TABLE "+table+" ADD COLUMN "+column+" "+definition)
	return err
}

// initUsersSchema creates all tables for user.db.
// This project has no user account system; user.db is provisioned empty
// so the DatabaseManager API remains consistent across projects.
func initUsersSchema(ctx context.Context, db *DB) error {
	_ = db
	return nil
}

// ServerTableName returns the prefixed table name for server database queries.
// Use this helper when building queries to ensure correct prefix is applied.
func ServerTableName(db *DB, table string) string {
	return serverTablePrefix(db) + table
}

// RequestStatsTable returns the prefix-aware server request statistics table.
func (db *DB) RequestStatsTable() string {
	return ServerTableName(db, "server_request_stats_hourly")
}
