package database

import (
	"context"
	"fmt"
	"strings"
)

// InitSchema creates all database tables idempotently on startup.
// All statements use CREATE TABLE IF NOT EXISTS and ALTER TABLE ADD COLUMN IF NOT EXISTS.
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
		// API tokens (SHA-256 hashed)
		`CREATE TABLE IF NOT EXISTS {prefix}api_tokens (
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
	}

	for _, stmt := range statements {
		sql := applyPrefix(stmt)
		if _, err := db.Exec(ctx, sql); err != nil {
			return fmt.Errorf("schema statement failed: %w\nSQL: %s", err, sql)
		}
	}
	return nil
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
