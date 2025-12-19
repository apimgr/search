package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// AdminUser represents an admin user
type AdminUser struct {
	ID           int64      `json:"id"`
	Username     string     `json:"username"`
	PasswordHash string     `json:"-"`
	Email        string     `json:"email,omitempty"`
	Role         string     `json:"role"`
	Active       bool       `json:"active"`
	LastLogin    *time.Time `json:"last_login,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

// AdminSession represents an admin session
type AdminSession struct {
	ID        int64     `json:"id"`
	UserID    int64     `json:"user_id"`
	Token     string    `json:"token"`
	IPAddress string    `json:"ip_address,omitempty"`
	UserAgent string    `json:"user_agent,omitempty"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
}

// APIToken represents an API token
type APIToken struct {
	ID          int64      `json:"id"`
	Name        string     `json:"name"`
	Token       string     `json:"token"`
	Description string     `json:"description,omitempty"`
	Permissions []string   `json:"permissions,omitempty"`
	RateLimit   int        `json:"rate_limit"`
	Active      bool       `json:"active"`
	LastUsed    *time.Time `json:"last_used,omitempty"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

// SearchStats represents search statistics
type SearchStats struct {
	ID              int64     `json:"id"`
	Date            time.Time `json:"date"`
	Hour            int       `json:"hour"`
	QueryCount      int       `json:"query_count"`
	ResultCount     int       `json:"result_count"`
	AvgResponseTime float64   `json:"avg_response_time"`
	EnginesUsed     []string  `json:"engines_used,omitempty"`
	Categories      []string  `json:"categories,omitempty"`
}

// EngineStats represents engine statistics
type EngineStats struct {
	ID              int64     `json:"id"`
	Date            time.Time `json:"date"`
	Engine          string    `json:"engine"`
	QueryCount      int       `json:"query_count"`
	ResultCount     int       `json:"result_count"`
	ErrorCount      int       `json:"error_count"`
	AvgResponseTime float64   `json:"avg_response_time"`
}

// BlockedIP represents a blocked IP address
type BlockedIP struct {
	ID        int64      `json:"id"`
	IPAddress string     `json:"ip_address"`
	Reason    string     `json:"reason,omitempty"`
	BlockedBy string     `json:"blocked_by,omitempty"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

// AuditLogEntry represents an audit log entry
type AuditLogEntry struct {
	ID        int64     `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	UserID    *int64    `json:"user_id,omitempty"`
	Action    string    `json:"action"`
	Resource  string    `json:"resource,omitempty"`
	Details   string    `json:"details,omitempty"`
	IPAddress string    `json:"ip_address,omitempty"`
	UserAgent string    `json:"user_agent,omitempty"`
}

// Repository provides database operations
type Repository struct {
	db *DB
}

// NewRepository creates a new repository
func NewRepository(db *DB) *Repository {
	return &Repository{db: db}
}

// Admin User Operations

// CreateAdminUser creates a new admin user
func (r *Repository) CreateAdminUser(ctx context.Context, user *AdminUser) error {
	result, err := r.db.Exec(ctx,
		`INSERT INTO admin_users (username, password_hash, email, role, active)
		 VALUES (?, ?, ?, ?, ?)`,
		user.Username, user.PasswordHash, user.Email, user.Role, user.Active)
	if err != nil {
		return err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return err
	}
	user.ID = id
	return nil
}

// GetAdminUserByUsername retrieves an admin user by username
func (r *Repository) GetAdminUserByUsername(ctx context.Context, username string) (*AdminUser, error) {
	row := r.db.QueryRow(ctx,
		`SELECT id, username, password_hash, email, role, active, last_login, created_at, updated_at
		 FROM admin_users WHERE username = ?`, username)

	user := &AdminUser{}
	err := row.Scan(&user.ID, &user.Username, &user.PasswordHash, &user.Email, &user.Role,
		&user.Active, &user.LastLogin, &user.CreatedAt, &user.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return user, nil
}

// GetAdminUserByID retrieves an admin user by ID
func (r *Repository) GetAdminUserByID(ctx context.Context, id int64) (*AdminUser, error) {
	row := r.db.QueryRow(ctx,
		`SELECT id, username, password_hash, email, role, active, last_login, created_at, updated_at
		 FROM admin_users WHERE id = ?`, id)

	user := &AdminUser{}
	err := row.Scan(&user.ID, &user.Username, &user.PasswordHash, &user.Email, &user.Role,
		&user.Active, &user.LastLogin, &user.CreatedAt, &user.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return user, nil
}

// UpdateAdminUserLastLogin updates the last login time
func (r *Repository) UpdateAdminUserLastLogin(ctx context.Context, userID int64) error {
	_, err := r.db.Exec(ctx,
		`UPDATE admin_users SET last_login = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		userID)
	return err
}

// Admin Session Operations

// CreateAdminSession creates a new admin session
func (r *Repository) CreateAdminSession(ctx context.Context, session *AdminSession) error {
	result, err := r.db.Exec(ctx,
		`INSERT INTO admin_sessions (user_id, token, ip_address, user_agent, expires_at)
		 VALUES (?, ?, ?, ?, ?)`,
		session.UserID, session.Token, session.IPAddress, session.UserAgent, session.ExpiresAt)
	if err != nil {
		return err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return err
	}
	session.ID = id
	return nil
}

// GetAdminSessionByToken retrieves a session by token
func (r *Repository) GetAdminSessionByToken(ctx context.Context, token string) (*AdminSession, error) {
	row := r.db.QueryRow(ctx,
		`SELECT id, user_id, token, ip_address, user_agent, expires_at, created_at
		 FROM admin_sessions WHERE token = ? AND expires_at > CURRENT_TIMESTAMP`, token)

	session := &AdminSession{}
	err := row.Scan(&session.ID, &session.UserID, &session.Token, &session.IPAddress,
		&session.UserAgent, &session.ExpiresAt, &session.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return session, nil
}

// DeleteAdminSession deletes a session
func (r *Repository) DeleteAdminSession(ctx context.Context, token string) error {
	_, err := r.db.Exec(ctx, `DELETE FROM admin_sessions WHERE token = ?`, token)
	return err
}

// DeleteExpiredSessions removes all expired sessions
func (r *Repository) DeleteExpiredSessions(ctx context.Context) (int64, error) {
	result, err := r.db.Exec(ctx, `DELETE FROM admin_sessions WHERE expires_at <= CURRENT_TIMESTAMP`)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// API Token Operations

// CreateAPIToken creates a new API token
func (r *Repository) CreateAPIToken(ctx context.Context, token *APIToken) error {
	perms, _ := json.Marshal(token.Permissions)
	result, err := r.db.Exec(ctx,
		`INSERT INTO api_tokens (name, token, description, permissions, rate_limit, active, expires_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		token.Name, token.Token, token.Description, string(perms), token.RateLimit, token.Active, token.ExpiresAt)
	if err != nil {
		return err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return err
	}
	token.ID = id
	return nil
}

// GetAPITokenByToken retrieves an API token
func (r *Repository) GetAPITokenByToken(ctx context.Context, token string) (*APIToken, error) {
	row := r.db.QueryRow(ctx,
		`SELECT id, name, token, description, permissions, rate_limit, active, last_used, expires_at, created_at
		 FROM api_tokens WHERE token = ? AND active = 1`, token)

	t := &APIToken{}
	var perms string
	err := row.Scan(&t.ID, &t.Name, &t.Token, &t.Description, &perms, &t.RateLimit,
		&t.Active, &t.LastUsed, &t.ExpiresAt, &t.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if perms != "" {
		json.Unmarshal([]byte(perms), &t.Permissions)
	}
	return t, nil
}

// UpdateAPITokenLastUsed updates the last used time
func (r *Repository) UpdateAPITokenLastUsed(ctx context.Context, tokenID int64) error {
	_, err := r.db.Exec(ctx, `UPDATE api_tokens SET last_used = CURRENT_TIMESTAMP WHERE id = ?`, tokenID)
	return err
}

// ListAPITokens lists all API tokens
func (r *Repository) ListAPITokens(ctx context.Context) ([]*APIToken, error) {
	rows, err := r.db.Query(ctx,
		`SELECT id, name, token, description, permissions, rate_limit, active, last_used, expires_at, created_at
		 FROM api_tokens ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tokens := make([]*APIToken, 0)
	for rows.Next() {
		t := &APIToken{}
		var perms string
		if err := rows.Scan(&t.ID, &t.Name, &t.Token, &t.Description, &perms, &t.RateLimit,
			&t.Active, &t.LastUsed, &t.ExpiresAt, &t.CreatedAt); err != nil {
			return nil, err
		}
		if perms != "" {
			json.Unmarshal([]byte(perms), &t.Permissions)
		}
		tokens = append(tokens, t)
	}
	return tokens, rows.Err()
}

// DeleteAPIToken deletes an API token
func (r *Repository) DeleteAPIToken(ctx context.Context, tokenID int64) error {
	_, err := r.db.Exec(ctx, `DELETE FROM api_tokens WHERE id = ?`, tokenID)
	return err
}

// Search Stats Operations

// RecordSearchStats records search statistics
func (r *Repository) RecordSearchStats(ctx context.Context, queryCount, resultCount int, responseTime float64, engines, categories []string) error {
	enginesJSON, _ := json.Marshal(engines)
	categoriesJSON, _ := json.Marshal(categories)

	_, err := r.db.Exec(ctx,
		`INSERT INTO search_stats (date, hour, query_count, result_count, avg_response_time, engines_used, categories)
		 VALUES (DATE('now'), CAST(strftime('%H', 'now') AS INTEGER), ?, ?, ?, ?, ?)
		 ON CONFLICT(date, hour) DO UPDATE SET
		 query_count = query_count + excluded.query_count,
		 result_count = result_count + excluded.result_count,
		 avg_response_time = (avg_response_time + excluded.avg_response_time) / 2`,
		queryCount, resultCount, responseTime, string(enginesJSON), string(categoriesJSON))
	return err
}

// GetSearchStats retrieves search statistics for a date range
func (r *Repository) GetSearchStats(ctx context.Context, startDate, endDate time.Time) ([]*SearchStats, error) {
	rows, err := r.db.Query(ctx,
		`SELECT id, date, hour, query_count, result_count, avg_response_time, engines_used, categories
		 FROM search_stats WHERE date BETWEEN ? AND ? ORDER BY date, hour`,
		startDate.Format("2006-01-02"), endDate.Format("2006-01-02"))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	stats := make([]*SearchStats, 0)
	for rows.Next() {
		s := &SearchStats{}
		var enginesJSON, categoriesJSON string
		if err := rows.Scan(&s.ID, &s.Date, &s.Hour, &s.QueryCount, &s.ResultCount,
			&s.AvgResponseTime, &enginesJSON, &categoriesJSON); err != nil {
			return nil, err
		}
		if enginesJSON != "" {
			json.Unmarshal([]byte(enginesJSON), &s.EnginesUsed)
		}
		if categoriesJSON != "" {
			json.Unmarshal([]byte(categoriesJSON), &s.Categories)
		}
		stats = append(stats, s)
	}
	return stats, rows.Err()
}

// Blocked IP Operations

// BlockIP blocks an IP address
func (r *Repository) BlockIP(ctx context.Context, ip *BlockedIP) error {
	result, err := r.db.Exec(ctx,
		`INSERT INTO blocked_ips (ip_address, reason, blocked_by, expires_at)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(ip_address) DO UPDATE SET reason = excluded.reason, expires_at = excluded.expires_at`,
		ip.IPAddress, ip.Reason, ip.BlockedBy, ip.ExpiresAt)
	if err != nil {
		return err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return err
	}
	ip.ID = id
	return nil
}

// IsIPBlocked checks if an IP is blocked
func (r *Repository) IsIPBlocked(ctx context.Context, ipAddress string) (bool, error) {
	row := r.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM blocked_ips WHERE ip_address = ? AND (expires_at IS NULL OR expires_at > CURRENT_TIMESTAMP)`,
		ipAddress)
	var count int
	if err := row.Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

// UnblockIP removes an IP block
func (r *Repository) UnblockIP(ctx context.Context, ipAddress string) error {
	_, err := r.db.Exec(ctx, `DELETE FROM blocked_ips WHERE ip_address = ?`, ipAddress)
	return err
}

// ListBlockedIPs lists all blocked IPs
func (r *Repository) ListBlockedIPs(ctx context.Context) ([]*BlockedIP, error) {
	rows, err := r.db.Query(ctx,
		`SELECT id, ip_address, reason, blocked_by, expires_at, created_at
		 FROM blocked_ips WHERE expires_at IS NULL OR expires_at > CURRENT_TIMESTAMP
		 ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	ips := make([]*BlockedIP, 0)
	for rows.Next() {
		ip := &BlockedIP{}
		if err := rows.Scan(&ip.ID, &ip.IPAddress, &ip.Reason, &ip.BlockedBy,
			&ip.ExpiresAt, &ip.CreatedAt); err != nil {
			return nil, err
		}
		ips = append(ips, ip)
	}
	return ips, rows.Err()
}

// Audit Log Operations

// RecordAudit records an audit log entry
func (r *Repository) RecordAudit(ctx context.Context, entry *AuditLogEntry) error {
	result, err := r.db.Exec(ctx,
		`INSERT INTO audit_log (user_id, action, resource, details, ip_address, user_agent)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		entry.UserID, entry.Action, entry.Resource, entry.Details, entry.IPAddress, entry.UserAgent)
	if err != nil {
		return err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return err
	}
	entry.ID = id
	return nil
}

// GetAuditLog retrieves audit log entries
func (r *Repository) GetAuditLog(ctx context.Context, limit, offset int) ([]*AuditLogEntry, error) {
	rows, err := r.db.Query(ctx,
		`SELECT id, timestamp, user_id, action, resource, details, ip_address, user_agent
		 FROM audit_log ORDER BY timestamp DESC LIMIT ? OFFSET ?`,
		limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	entries := make([]*AuditLogEntry, 0)
	for rows.Next() {
		e := &AuditLogEntry{}
		if err := rows.Scan(&e.ID, &e.Timestamp, &e.UserID, &e.Action, &e.Resource,
			&e.Details, &e.IPAddress, &e.UserAgent); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// CleanupOldAuditLogs removes audit logs older than specified days
func (r *Repository) CleanupOldAuditLogs(ctx context.Context, olderThanDays int) (int64, error) {
	result, err := r.db.Exec(ctx,
		`DELETE FROM audit_log WHERE timestamp < DATE('now', ?)`,
		fmt.Sprintf("-%d days", olderThanDays))
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}
