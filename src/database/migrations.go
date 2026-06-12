package database

import (
	"context"
	"fmt"
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

// initServerSchema creates all tables for server.db.
func initServerSchema(ctx context.Context, db *DB) error {
	statements := []string{
		// Scheduler state
		`CREATE TABLE IF NOT EXISTS scheduler_state (
			task_id TEXT PRIMARY KEY,
			last_run DATETIME,
			next_run DATETIME,
			last_result TEXT,
			last_error TEXT,
			run_count INTEGER DEFAULT 0,
			enabled INTEGER DEFAULT 1
		)`,
		// Audit log
		`CREATE TABLE IF NOT EXISTS audit_log (
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
		`CREATE INDEX IF NOT EXISTS idx_audit_log_timestamp ON audit_log(timestamp)`,
		`CREATE INDEX IF NOT EXISTS idx_audit_log_actor ON audit_log(actor_type, actor_id)`,
		`CREATE INDEX IF NOT EXISTS idx_audit_log_action ON audit_log(action)`,
		`CREATE INDEX IF NOT EXISTS idx_audit_log_resource ON audit_log(resource_type, resource_id)`,
		// Server settings (key-value store)
		`CREATE TABLE IF NOT EXISTS server_settings (
			key TEXT PRIMARY KEY,
			value TEXT,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		// API tokens (SHA-256 hashed)
		`CREATE TABLE IF NOT EXISTS api_tokens (
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
		`CREATE INDEX IF NOT EXISTS idx_api_tokens_hash ON api_tokens(token_hash)`,
		`CREATE INDEX IF NOT EXISTS idx_api_tokens_prefix ON api_tokens(token_prefix)`,
		// Search statistics
		`CREATE TABLE IF NOT EXISTS search_stats (
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
		`CREATE INDEX IF NOT EXISTS idx_search_stats_date ON search_stats(date)`,
		// Engine statistics
		`CREATE TABLE IF NOT EXISTS engine_stats (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			date DATE NOT NULL,
			engine TEXT NOT NULL,
			query_count INTEGER DEFAULT 0,
			result_count INTEGER DEFAULT 0,
			error_count INTEGER DEFAULT 0,
			avg_response_time REAL DEFAULT 0,
			UNIQUE(date, engine)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_engine_stats_date ON engine_stats(date)`,
		`CREATE INDEX IF NOT EXISTS idx_engine_stats_engine ON engine_stats(engine)`,
		// Blocked IPs
		`CREATE TABLE IF NOT EXISTS blocked_ips (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			ip_address TEXT UNIQUE NOT NULL,
			reason TEXT,
			blocked_by TEXT,
			expires_at DATETIME,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_blocked_ips_ip ON blocked_ips(ip_address)`,
		// Custom bangs
		`CREATE TABLE IF NOT EXISTS custom_bangs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			shortcut TEXT UNIQUE NOT NULL,
			name TEXT NOT NULL,
			url TEXT NOT NULL,
			category TEXT DEFAULT 'custom',
			description TEXT,
			active INTEGER DEFAULT 1,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_custom_bangs_shortcut ON custom_bangs(shortcut)`,
		// Search alerts
		`CREATE TABLE IF NOT EXISTS search_alerts (
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
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_search_alerts_manage_token ON search_alerts(manage_token_hash)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_search_alerts_rss_token ON search_alerts(rss_token_hash)`,
		`CREATE INDEX IF NOT EXISTS idx_search_alerts_verify_token ON search_alerts(verify_token_hash)`,
		`CREATE INDEX IF NOT EXISTS idx_search_alerts_status_frequency ON search_alerts(status, frequency)`,
		// Search alert results
		`CREATE TABLE IF NOT EXISTS search_alert_results (
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
		)`,
		`CREATE INDEX IF NOT EXISTS idx_search_alert_results_alert ON search_alert_results(alert_id, first_seen_at)`,
	}

	for _, stmt := range statements {
		if _, err := db.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("schema statement failed: %w\nSQL: %s", err, stmt)
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
