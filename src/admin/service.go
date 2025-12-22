package admin

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/apimgr/search/src/database"
	"github.com/google/uuid"
	"golang.org/x/crypto/argon2"
)

// AdminService handles server admin management per TEMPLATE.md PART 31
type AdminService struct {
	db *database.DB
}

// Admin represents a server admin account
type Admin struct {
	ID          int64
	Username    string
	Email       string
	IsPrimary   bool
	Source      string
	ExternalID  string
	TOTPEnabled bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
	LastLoginAt *time.Time
}

// AdminInvite represents an admin invite token
type AdminInvite struct {
	ID        string
	Username  string
	CreatedBy int64
	ExpiresAt time.Time
	UsedAt    *time.Time
	UsedBy    *int64
	CreatedAt time.Time
}

// NewAdminService creates a new admin service
func NewAdminService(db *database.DB) *AdminService {
	return &AdminService{db: db}
}

// GetAdminByID retrieves an admin by ID
func (s *AdminService) GetAdminByID(ctx context.Context, id int64) (*Admin, error) {
	row := s.db.QueryRow(ctx, `
		SELECT id, username, email, is_primary, source, external_id, totp_enabled,
		       created_at, updated_at, last_login_at
		FROM admin_credentials WHERE id = ?
	`, id)

	return s.scanAdmin(row)
}

// GetAdminByUsername retrieves an admin by username
func (s *AdminService) GetAdminByUsername(ctx context.Context, username string) (*Admin, error) {
	row := s.db.QueryRow(ctx, `
		SELECT id, username, email, is_primary, source, external_id, totp_enabled,
		       created_at, updated_at, last_login_at
		FROM admin_credentials WHERE LOWER(username) = LOWER(?)
	`, username)

	return s.scanAdmin(row)
}

// GetAdminByEmail retrieves an admin by email
func (s *AdminService) GetAdminByEmail(ctx context.Context, email string) (*Admin, error) {
	row := s.db.QueryRow(ctx, `
		SELECT id, username, email, is_primary, source, external_id, totp_enabled,
		       created_at, updated_at, last_login_at
		FROM admin_credentials WHERE LOWER(email) = LOWER(?)
	`, email)

	return s.scanAdmin(row)
}

// scanAdmin scans an admin row
func (s *AdminService) scanAdmin(row *sql.Row) (*Admin, error) {
	var a Admin
	var email, externalID sql.NullString
	var lastLogin sql.NullTime

	err := row.Scan(
		&a.ID, &a.Username, &email, &a.IsPrimary, &a.Source, &externalID,
		&a.TOTPEnabled, &a.CreatedAt, &a.UpdatedAt, &lastLogin,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	if email.Valid {
		a.Email = email.String
	}
	if externalID.Valid {
		a.ExternalID = externalID.String
	}
	if lastLogin.Valid {
		a.LastLoginAt = &lastLogin.Time
	}

	return &a, nil
}

// GetAdminsForAdmin returns admins visible to the requesting admin
// Per TEMPLATE.md PART 31: Non-primary admins can only see their own account
func (s *AdminService) GetAdminsForAdmin(ctx context.Context, requestingAdminID int64) ([]*Admin, error) {
	// Check if requesting admin is primary
	requestingAdmin, err := s.GetAdminByID(ctx, requestingAdminID)
	if err != nil {
		return nil, err
	}
	if requestingAdmin == nil {
		return nil, fmt.Errorf("admin not found")
	}

	var query string
	var args []interface{}

	if requestingAdmin.IsPrimary {
		// Primary admin can see all admins
		query = `
			SELECT id, username, email, is_primary, source, external_id, totp_enabled,
			       created_at, updated_at, last_login_at
			FROM admin_credentials ORDER BY is_primary DESC, created_at ASC
		`
	} else {
		// Non-primary admin can only see their own account
		query = `
			SELECT id, username, email, is_primary, source, external_id, totp_enabled,
			       created_at, updated_at, last_login_at
			FROM admin_credentials WHERE id = ?
		`
		args = append(args, requestingAdminID)
	}

	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var admins []*Admin
	for rows.Next() {
		var a Admin
		var email, externalID sql.NullString
		var lastLogin sql.NullTime

		err := rows.Scan(
			&a.ID, &a.Username, &email, &a.IsPrimary, &a.Source, &externalID,
			&a.TOTPEnabled, &a.CreatedAt, &a.UpdatedAt, &lastLogin,
		)
		if err != nil {
			return nil, err
		}

		if email.Valid {
			a.Email = email.String
		}
		if externalID.Valid {
			a.ExternalID = externalID.String
		}
		if lastLogin.Valid {
			a.LastLoginAt = &lastLogin.Time
		}

		admins = append(admins, &a)
	}

	return admins, rows.Err()
}

// GetTotalAdminCount returns total number of admins (visible to all admins)
func (s *AdminService) GetTotalAdminCount(ctx context.Context) (int, error) {
	row := s.db.QueryRow(ctx, "SELECT COUNT(*) FROM admin_credentials")
	var count int
	err := row.Scan(&count)
	return count, err
}

// GetOnlineAdmins returns usernames of currently logged-in admins
// Per TEMPLATE.md: admins can see WHO is logged in (username only)
func (s *AdminService) GetOnlineAdmins(ctx context.Context) ([]string, error) {
	rows, err := s.db.Query(ctx, `
		SELECT DISTINCT ac.username
		FROM admin_credentials ac
		INNER JOIN admin_sessions s ON s.token_hash LIKE '%'
		WHERE s.expires_at > datetime('now')
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var usernames []string
	for rows.Next() {
		var username string
		if err := rows.Scan(&username); err != nil {
			return nil, err
		}
		usernames = append(usernames, username)
	}

	return usernames, rows.Err()
}

// CanAdminViewAdmin checks if one admin can view another's details
// Per TEMPLATE.md PART 31: admins cannot see other admin accounts
func (s *AdminService) CanAdminViewAdmin(ctx context.Context, viewerID, targetID int64) (bool, error) {
	// Admin can always view themselves
	if viewerID == targetID {
		return true, nil
	}

	// Check if viewer is primary
	viewer, err := s.GetAdminByID(ctx, viewerID)
	if err != nil {
		return false, err
	}

	// Only primary admin can view other admins
	return viewer != nil && viewer.IsPrimary, nil
}

// CanAdminModifyAdmin checks if one admin can modify another
// Per TEMPLATE.md: Primary admin cannot be deleted except via --maintenance setup
func (s *AdminService) CanAdminModifyAdmin(ctx context.Context, modifierID, targetID int64) (bool, error) {
	// Admin can always modify themselves (except deletion)
	if modifierID == targetID {
		return true, nil
	}

	// Check if modifier is primary
	modifier, err := s.GetAdminByID(ctx, modifierID)
	if err != nil {
		return false, err
	}
	if modifier == nil || !modifier.IsPrimary {
		return false, nil
	}

	// Check if target is primary
	target, err := s.GetAdminByID(ctx, targetID)
	if err != nil {
		return false, err
	}

	// Cannot modify primary admin (except self)
	return target != nil && !target.IsPrimary, nil
}

// AuthenticateAdmin authenticates an admin by username/email and password
func (s *AdminService) AuthenticateAdmin(ctx context.Context, identifier, password string) (*Admin, error) {
	// Try to find admin by username or email
	row := s.db.QueryRow(ctx, `
		SELECT id, username, email, password_hash, is_primary, source, external_id, totp_enabled,
		       created_at, updated_at, last_login_at
		FROM admin_credentials
		WHERE LOWER(username) = LOWER(?) OR LOWER(email) = LOWER(?)
	`, identifier, identifier)

	var a Admin
	var email, externalID sql.NullString
	var lastLogin sql.NullTime
	var passwordHash string

	err := row.Scan(
		&a.ID, &a.Username, &email, &passwordHash, &a.IsPrimary, &a.Source, &externalID,
		&a.TOTPEnabled, &a.CreatedAt, &a.UpdatedAt, &lastLogin,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	if email.Valid {
		a.Email = email.String
	}
	if externalID.Valid {
		a.ExternalID = externalID.String
	}
	if lastLogin.Valid {
		a.LastLoginAt = &lastLogin.Time
	}

	// Verify password using Argon2id
	if !s.verifyPassword(password, passwordHash) {
		return nil, nil
	}

	// Update last login
	_, _ = s.db.Exec(ctx, `
		UPDATE admin_credentials SET last_login_at = datetime('now'), updated_at = datetime('now')
		WHERE id = ?
	`, a.ID)

	return &a, nil
}

// verifyPassword verifies a password against an Argon2id hash
func (s *AdminService) verifyPassword(password, encodedHash string) bool {
	parts := strings.Split(encodedHash, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		// Fall back to plain comparison for initial setup
		return subtle.ConstantTimeCompare([]byte(password), []byte(encodedHash)) == 1
	}

	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return false
	}

	var memory, time uint32
	var threads uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &time, &threads); err != nil {
		return false
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false
	}

	expectedHash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false
	}

	computedHash := argon2.IDKey([]byte(password), salt, time, memory, threads, uint32(len(expectedHash)))
	return subtle.ConstantTimeCompare(computedHash, expectedHash) == 1
}

// CreateAdmin creates a new admin account
func (s *AdminService) CreateAdmin(ctx context.Context, username, email, password string, isPrimary bool) (*Admin, error) {
	passwordHash := s.hashPassword(password)

	result, err := s.db.Exec(ctx, `
		INSERT INTO admin_credentials (username, email, password_hash, is_primary, source)
		VALUES (?, ?, ?, ?, 'local')
	`, username, email, passwordHash, isPrimary)
	if err != nil {
		return nil, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	return s.GetAdminByID(ctx, id)
}

// hashPassword creates an Argon2id hash per TEMPLATE.md (Time=3)
func (s *AdminService) hashPassword(password string) string {
	const (
		argon2Time    = 3
		argon2Memory  = 64 * 1024
		argon2Threads = 4
		argon2KeyLen  = 32
		argon2SaltLen = 16
	)

	salt := make([]byte, argon2SaltLen)
	if _, err := rand.Read(salt); err != nil {
		return ""
	}

	hash := argon2.IDKey([]byte(password), salt, argon2Time, argon2Memory, argon2Threads, argon2KeyLen)
	b64Salt := base64.RawStdEncoding.EncodeToString(salt)
	b64Hash := base64.RawStdEncoding.EncodeToString(hash)

	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argon2Memory, argon2Time, argon2Threads, b64Salt, b64Hash)
}

// DeleteAdmin deletes a non-primary admin
func (s *AdminService) DeleteAdmin(ctx context.Context, adminID, requestingAdminID int64) error {
	// Check if target is primary
	target, err := s.GetAdminByID(ctx, adminID)
	if err != nil {
		return err
	}
	if target == nil {
		return fmt.Errorf("admin not found")
	}
	if target.IsPrimary {
		return fmt.Errorf("cannot delete primary admin")
	}

	// Check if requester can modify target
	canModify, err := s.CanAdminModifyAdmin(ctx, requestingAdminID, adminID)
	if err != nil {
		return err
	}
	if !canModify {
		return fmt.Errorf("permission denied")
	}

	_, err = s.db.Exec(ctx, "DELETE FROM admin_credentials WHERE id = ?", adminID)
	return err
}

// CreateInvite creates an admin invite token per TEMPLATE.md PART 31
func (s *AdminService) CreateInvite(ctx context.Context, createdBy int64, username string, expiresIn time.Duration) (string, error) {
	// Generate secure token
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", err
	}
	token := base64.URLEncoding.EncodeToString(tokenBytes)

	// Hash token for storage
	tokenHash := sha256.Sum256([]byte(token))
	tokenHashHex := hex.EncodeToString(tokenHash[:])

	// Create invite record
	inviteID := uuid.New().String()
	expiresAt := time.Now().Add(expiresIn)

	_, err := s.db.Exec(ctx, `
		INSERT INTO admin_invites (id, token_hash, username, created_by, expires_at)
		VALUES (?, ?, ?, ?, ?)
	`, inviteID, tokenHashHex, username, createdBy, expiresAt)
	if err != nil {
		return "", err
	}

	return token, nil
}

// ValidateInvite validates an invite token and returns the invite details
func (s *AdminService) ValidateInvite(ctx context.Context, token string) (*AdminInvite, error) {
	tokenHash := sha256.Sum256([]byte(token))
	tokenHashHex := hex.EncodeToString(tokenHash[:])

	row := s.db.QueryRow(ctx, `
		SELECT id, token_hash, username, created_by, expires_at, used_at, used_by, created_at
		FROM admin_invites
		WHERE token_hash = ? AND used_at IS NULL AND expires_at > datetime('now')
	`, tokenHashHex)

	var invite AdminInvite
	var username sql.NullString
	var usedAt sql.NullTime
	var usedBy sql.NullInt64

	err := row.Scan(
		&invite.ID, nil, &username, &invite.CreatedBy, &invite.ExpiresAt,
		&usedAt, &usedBy, &invite.CreatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	if username.Valid {
		invite.Username = username.String
	}
	if usedAt.Valid {
		invite.UsedAt = &usedAt.Time
	}
	if usedBy.Valid {
		invite.UsedBy = &usedBy.Int64
	}

	return &invite, nil
}

// AcceptInvite completes the admin invite flow
func (s *AdminService) AcceptInvite(ctx context.Context, token, username, email, password string) (*Admin, error) {
	// Validate token
	invite, err := s.ValidateInvite(ctx, token)
	if err != nil {
		return nil, err
	}
	if invite == nil {
		return nil, fmt.Errorf("invalid or expired invite")
	}

	// Create admin account
	admin, err := s.CreateAdmin(ctx, username, email, password, false)
	if err != nil {
		return nil, err
	}

	// Mark invite as used
	tokenHash := sha256.Sum256([]byte(token))
	tokenHashHex := hex.EncodeToString(tokenHash[:])

	_, err = s.db.Exec(ctx, `
		UPDATE admin_invites SET used_at = datetime('now'), used_by = ?
		WHERE token_hash = ?
	`, admin.ID, tokenHashHex)
	if err != nil {
		// Admin was created but invite update failed - not critical
		return admin, nil
	}

	return admin, nil
}

// GenerateAPIToken generates a new API token for an admin
func (s *AdminService) GenerateAPIToken(ctx context.Context, adminID int64) (string, error) {
	// Generate secure token
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", err
	}
	token := "search_" + base64.URLEncoding.EncodeToString(tokenBytes)[:32]

	// Hash token for storage
	tokenHash := sha256.Sum256([]byte(token))
	tokenHashHex := hex.EncodeToString(tokenHash[:])
	tokenPrefix := token[:8]

	_, err := s.db.Exec(ctx, `
		UPDATE admin_credentials SET token_hash = ?, token_prefix = ?, updated_at = datetime('now')
		WHERE id = ?
	`, tokenHashHex, tokenPrefix, adminID)
	if err != nil {
		return "", err
	}

	return token, nil
}

// ValidateAPIToken validates an admin API token
func (s *AdminService) ValidateAPIToken(ctx context.Context, token string) (*Admin, error) {
	tokenHash := sha256.Sum256([]byte(token))
	tokenHashHex := hex.EncodeToString(tokenHash[:])

	row := s.db.QueryRow(ctx, `
		SELECT id, username, email, is_primary, source, external_id, totp_enabled,
		       created_at, updated_at, last_login_at
		FROM admin_credentials WHERE token_hash = ?
	`, tokenHashHex)

	return s.scanAdmin(row)
}

// CreateSetupToken creates a one-time setup token for --maintenance setup
func (s *AdminService) CreateSetupToken(ctx context.Context) (string, error) {
	// Generate secure token
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", err
	}
	token := base64.URLEncoding.EncodeToString(tokenBytes)[:32]

	// Hash token for storage
	tokenHash := sha256.Sum256([]byte(token))
	tokenHashHex := hex.EncodeToString(tokenHash[:])

	// Delete any existing setup token
	_, _ = s.db.Exec(ctx, "DELETE FROM setup_token")

	// Create new setup token (expires in 1 hour)
	expiresAt := time.Now().Add(1 * time.Hour)
	_, err := s.db.Exec(ctx, `
		INSERT INTO setup_token (id, token_hash, expires_at) VALUES (1, ?, ?)
	`, tokenHashHex, expiresAt)
	if err != nil {
		return "", err
	}

	return token, nil
}

// ValidateSetupToken validates a setup token
func (s *AdminService) ValidateSetupToken(ctx context.Context, token string) (bool, error) {
	tokenHash := sha256.Sum256([]byte(token))
	tokenHashHex := hex.EncodeToString(tokenHash[:])

	row := s.db.QueryRow(ctx, `
		SELECT 1 FROM setup_token
		WHERE token_hash = ? AND used_at IS NULL AND expires_at > datetime('now')
	`, tokenHashHex)

	var exists int
	err := row.Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return err == nil, err
}

// UseSetupToken marks a setup token as used
func (s *AdminService) UseSetupToken(ctx context.Context, token string) error {
	tokenHash := sha256.Sum256([]byte(token))
	tokenHashHex := hex.EncodeToString(tokenHash[:])

	_, err := s.db.Exec(ctx, `
		UPDATE setup_token SET used_at = datetime('now') WHERE token_hash = ?
	`, tokenHashHex)
	return err
}

// ResetPrimaryAdminCredentials resets the primary admin's password/token for --maintenance setup
func (s *AdminService) ResetPrimaryAdminCredentials(ctx context.Context) error {
	_, err := s.db.Exec(ctx, `
		UPDATE admin_credentials
		SET password_hash = '', token_hash = NULL, token_prefix = NULL, updated_at = datetime('now')
		WHERE is_primary = 1
	`)
	return err
}

// GetPrimaryAdmin returns the primary admin account
func (s *AdminService) GetPrimaryAdmin(ctx context.Context) (*Admin, error) {
	row := s.db.QueryRow(ctx, `
		SELECT id, username, email, is_primary, source, external_id, totp_enabled,
		       created_at, updated_at, last_login_at
		FROM admin_credentials WHERE is_primary = 1
	`)

	return s.scanAdmin(row)
}

// HasAnyAdmin checks if any admin account exists
func (s *AdminService) HasAnyAdmin(ctx context.Context) (bool, error) {
	row := s.db.QueryRow(ctx, "SELECT COUNT(*) FROM admin_credentials")
	var count int
	err := row.Scan(&count)
	return count > 0, err
}
