package owner

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math/big"
	"strings"
	"time"

	"github.com/apimgr/search/src/database"
	"github.com/google/uuid"
)

var (
	ErrInvalidToken = errors.New("invalid owner token")
	ErrNotFound     = errors.New("owner token not found")
	ErrForbidden    = errors.New("primary owner token required")
	ErrTokenLimit   = errors.New("linked token limit reached")
	ErrDuplicate    = errors.New("linked token name already exists")
	ErrConfirmation = errors.New("owner token confirmation did not match")
)

const tokenAlphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

type Identity struct {
	OwnerID string
	TokenID string
	Kind    string
	Prefix  string
}

type Token struct {
	ID         string
	Kind       string
	Name       string
	Prefix     string
	Last4      string
	CreatedAt  time.Time
	RotatedAt  *time.Time
	LastUsedAt *time.Time
	ExpiresAt  *time.Time
	RevokedAt  *time.Time
}

// OwnerRevocationCounts describes rows removed when an owner revokes the primary token.
type OwnerRevocationCounts struct {
	Tokens    int64
	Resources int64
}

type Manager struct {
	db        *sql.DB
	table     string
	owners    string
	audit     string
	key       []byte
	maxLinked int
	clock     func() time.Time
	writer    *database.DB
}

// MaxLinked returns the configured maximum number of active linked tokens per owner.
func (m *Manager) MaxLinked() int {
	return m.maxLinked
}

func NewManager(db *sql.DB, tablePrefix, encryptionKey string, maxLinked int) (*Manager, error) {
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encryptionKey))
	if err != nil || len(key) != 32 {
		return nil, errors.New("owner tokens require a base64-encoded 32-byte encryption key")
	}
	if maxLinked < 1 {
		maxLinked = 25
	}
	return &Manager{db: db, table: tablePrefix + "api_tokens", owners: tablePrefix + "owners", audit: tablePrefix + "audit_log", key: key, maxLinked: maxLinked, clock: time.Now}, nil
}

func NewManagerWithDB(db *database.DB, tablePrefix, encryptionKey string, maxLinked int) (*Manager, error) {
	manager, err := NewManager(db.ReadSQL(), tablePrefix, encryptionKey, maxLinked)
	if err == nil {
		manager.writer = db
	}
	return manager, err
}

func (m *Manager) withWrite(ctx context.Context, fn func(*sql.Tx) error) error {
	if m.writer != nil {
		return m.writer.Write(ctx, fn)
	}
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	committed = true
	return nil
}

func (m *Manager) execWrite(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	var result sql.Result
	err := m.withWrite(ctx, func(tx *sql.Tx) error {
		var err error
		result, err = tx.ExecContext(ctx, query, args...)
		return err
	})
	return result, err
}

func (m *Manager) Issue(ctx context.Context) (identity Identity, plaintext string, err error) {
	return m.IssueWithSource(ctx, "issue")
}

// IssueWithSource creates a primary token and records the issuing workflow in
// the same durable transaction as the owner, credential, and audit event.
func (m *Manager) IssueWithSource(ctx context.Context, source string) (identity Identity, plaintext string, err error) {
	ownerID := uuid.NewString()
	token, err := newToken()
	if err != nil {
		return Identity{}, "", err
	}
	hash := tokenHash(token)
	enc, err := m.encrypt(token)
	if err != nil {
		return Identity{}, "", err
	}
	now := m.clock().Unix()
	id, err := uuid.NewV7()
	if err != nil {
		return Identity{}, "", err
	}
	err = m.withWrite(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `INSERT INTO `+m.owners+` (id, created_at, last_seen_at) VALUES (?, ?, ?)`, ownerID, now, now); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO `+m.table+` (id, name, token_hash, token_prefix, token_last4, owner_id, kind, token_enc, created_at) VALUES (?, 'primary', ?, ?, ?, ?, 'primary', ?, ?)`, id.String(), hash, token[:12], token[len(token)-4:], ownerID, enc, now); err != nil {
			return err
		}
		details, err := json.Marshal(map[string]string{"token_prefix": token[:12], "source": source})
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO `+m.audit+` (id, timestamp, actor_type, actor_id, action, resource_type, resource_id, details) VALUES (?, ?, 'owner', ?, 'token.created', 'api_token', ?, ?)`, uuid.NewString(), now, ownerID, token[:12], string(details))
		return err
	})
	if err != nil {
		return Identity{}, "", err
	}
	identity = Identity{OwnerID: ownerID, TokenID: id.String(), Kind: "primary", Prefix: token[:12]}
	return identity, token, nil
}

func (m *Manager) Resolve(ctx context.Context, token string) (Identity, error) {
	if !validToken(token) {
		return Identity{}, ErrInvalidToken
	}
	hash := tokenHash(token)
	rows, err := m.db.QueryContext(ctx, `SELECT t.id, t.owner_id, t.kind, t.token_prefix, t.token_hash, t.expires_at, t.last_used_at, o.last_seen_at FROM `+m.table+` t JOIN `+m.owners+` o ON o.id = t.owner_id WHERE t.token_prefix = ? AND t.revoked_at IS NULL AND t.owner_id IS NOT NULL`, token[:12])
	if err != nil {
		return Identity{}, err
	}
	var identity Identity
	var expiry sql.NullInt64
	var lastUsed, ownerLastSeen sql.NullInt64
	matched := false
	for rows.Next() {
		var id Identity
		var storedHash string
		var expires sql.NullInt64
		var tokenLastUsed, lastSeen sql.NullInt64
			if err := rows.Scan(&id.TokenID, &id.OwnerID, &id.Kind, &id.Prefix, &storedHash, &expires, &tokenLastUsed, &lastSeen); err != nil {
			if closeErr := rows.Close(); closeErr != nil {
				return Identity{}, errors.Join(err, fmt.Errorf("close owner-token rows: %w", closeErr))
			}
			return Identity{}, err
		}
		equal := subtle.ConstantTimeCompare([]byte(hash), []byte(storedHash))
		if equal == 1 && id.OwnerID != "" {
			identity, expiry, lastUsed, ownerLastSeen, matched = id, expires, tokenLastUsed, lastSeen, true
		}
	}
	if err := rows.Err(); err != nil {
		if closeErr := rows.Close(); closeErr != nil {
			return Identity{}, errors.Join(err, fmt.Errorf("close owner-token rows: %w", closeErr))
		}
		return Identity{}, err
	}
	if err := rows.Close(); err != nil {
		return Identity{}, err
	}
	if !matched || (expiry.Valid && expiry.Int64 <= m.clock().Unix()) {
		return Identity{}, ErrInvalidToken
	}
	now := m.clock().Unix()
	if !lastUsed.Valid || lastUsed.Int64 <= now-300 || !ownerLastSeen.Valid || ownerLastSeen.Int64 <= now-300 {
		if err := m.withWrite(ctx, func(tx *sql.Tx) error {
			if !lastUsed.Valid || lastUsed.Int64 <= now-300 {
				if _, err := tx.ExecContext(ctx, `UPDATE `+m.table+` SET last_used_at = ? WHERE id = ? AND (last_used_at IS NULL OR last_used_at <= ?)`, now, identity.TokenID, now-300); err != nil {
					return err
				}
			}
			if !ownerLastSeen.Valid || ownerLastSeen.Int64 <= now-300 {
				if _, err := tx.ExecContext(ctx, `UPDATE `+m.owners+` SET last_seen_at = ? WHERE id = ? AND last_seen_at <= ?`, now, identity.OwnerID, now-300); err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
			return Identity{}, err
		}
	}
	return identity, nil
}

func (m *Manager) IssueOrResolve(ctx context.Context, token string) (Identity, string, error) {
	if token != "" {
		if identity, err := m.Resolve(ctx, token); err == nil {
			return identity, "", nil
		} else if !errors.Is(err, ErrInvalidToken) {
			return Identity{}, "", err
		}
	}
	return m.IssueWithSource(ctx, "resource")
}

func (m *Manager) List(ctx context.Context, identity Identity) (items []Token, err error) {
	if identity.Kind != "primary" {
		return nil, ErrForbidden
	}
	rows, err := m.db.QueryContext(ctx, `SELECT id, kind, COALESCE(name, ''), token_prefix, token_last4, created_at, rotated_at, last_used_at, expires_at, revoked_at FROM `+m.table+` WHERE owner_id = ? ORDER BY kind, id`, identity.OwnerID)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close owner-token list rows: %w", closeErr))
		}
	}()
	items = make([]Token, 0)
	for rows.Next() {
		var item Token
		var created int64
		var rotated, lastUsed, expires, revoked sql.NullInt64
		if err := rows.Scan(&item.ID, &item.Kind, &item.Name, &item.Prefix, &item.Last4, &created, &rotated, &lastUsed, &expires, &revoked); err != nil {
			return nil, err
		}
		item.CreatedAt = time.Unix(created, 0).UTC()
		item.RotatedAt = unixTime(rotated)
		item.LastUsedAt = unixTime(lastUsed)
		item.ExpiresAt = unixTime(expires)
		item.RevokedAt = unixTime(revoked)
		items = append(items, item)
	}
	return items, rows.Err()
}

func (m *Manager) ActiveCounts(ctx context.Context) (map[string]int, error) {
	rows, err := m.db.QueryContext(ctx, `SELECT kind, COUNT(*) FROM `+m.table+` WHERE revoked_at IS NULL AND (expires_at IS NULL OR expires_at > ?) GROUP BY kind`, m.clock().Unix())
	if err != nil {
		return nil, err
	}
	counts := map[string]int{"primary": 0, "linked": 0}
	for rows.Next() {
		var kind string
		var count int
		if err := rows.Scan(&kind, &count); err != nil {
			return nil, err
		}
		if kind == "primary" || kind == "linked" {
			counts[kind] = count
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	return counts, nil
}

func (m *Manager) Reveal(ctx context.Context, identity Identity, tokenID string) (string, error) {
	if identity.Kind != "primary" {
		return "", ErrForbidden
	}
	var encrypted string
	err := m.db.QueryRowContext(ctx, `SELECT token_enc FROM `+m.table+` WHERE id = ? AND owner_id = ? AND revoked_at IS NULL`, tokenID, identity.OwnerID).Scan(&encrypted)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	token, err := m.decrypt(encrypted)
	if err != nil {
		return "", err
	}
	if err := m.Audit(ctx, identity, "token.revealed"); err != nil {
		return "", err
	}
	return token, nil
}

// Mask returns a presentation-only token string; list responses never include encrypted or raw token values.
func (m *Manager) Mask(ctx context.Context, identity Identity, tokenID string) (string, error) {
	if identity.Kind != "primary" {
		return "", ErrForbidden
	}
	var prefix, last4 string
	if err := m.db.QueryRowContext(ctx, `SELECT token_prefix, token_last4 FROM `+m.table+` WHERE id = ? AND owner_id = ?`, tokenID, identity.OwnerID).Scan(&prefix, &last4); errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	} else if err != nil {
		return "", err
	}
	if len(last4) != 4 {
		return "", errors.New("owner token last-four metadata is unavailable")
	}
	if !strings.HasPrefix(prefix, "own_") {
		return "", ErrInvalidToken
	}
	return "own_••••••••••••••••••••••••••••" + last4, nil
}

// BackfillTokenLast4 derives display metadata from encrypted credentials after
// any pending encryption-key rotation has completed. It never stores plaintext.
func (m *Manager) BackfillTokenLast4(ctx context.Context) (int, error) {
	rows, err := m.db.QueryContext(ctx, `SELECT id, token_enc FROM `+m.table+` WHERE owner_id IS NOT NULL AND token_last4 = '' AND token_enc <> '' ORDER BY id`)
	if err != nil {
		return 0, fmt.Errorf("read owner token metadata to backfill last four: %w", err)
	}
	type rowData struct{ id, encrypted string }
	var pending []rowData
	for rows.Next() {
		var row rowData
		if err := rows.Scan(&row.id, &row.encrypted); err != nil {
			return 0, errors.Join(err, rows.Close())
		}
		pending = append(pending, row)
	}
	if err := rows.Err(); err != nil {
		return 0, errors.Join(err, rows.Close())
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}
	updated := 0
	for _, row := range pending {
		token, err := m.decrypt(row.encrypted)
		if err != nil {
			return updated, fmt.Errorf("decrypt owner token metadata for row %s: %w", row.id, err)
		}
		if !validToken(token) {
			return updated, fmt.Errorf("owner token metadata for row %s is invalid", row.id)
		}
		result, err := m.execWrite(ctx, `UPDATE `+m.table+` SET token_last4 = ? WHERE id = ? AND token_last4 = ''`, token[len(token)-4:], row.id)
		if err != nil {
			return updated, fmt.Errorf("write owner token metadata for row %s: %w", row.id, err)
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return updated, fmt.Errorf("read owner token metadata write result for row %s: %w", row.id, err)
		}
		updated += int(affected)
	}
	return updated, nil
}

func (m *Manager) Rotate(ctx context.Context, identity Identity, tokenID string) (string, error) {
	if identity.Kind != "primary" {
		return "", ErrForbidden
	}
	token, err := newToken()
	if err != nil {
		return "", err
	}
	encrypted, err := m.encrypt(token)
	if err != nil {
		return "", err
	}
	res, err := m.execWrite(ctx, `UPDATE `+m.table+` SET token_hash = ?, token_prefix = ?, token_last4 = ?, token_enc = ?, rotated_at = ?, last_used_at = NULL WHERE id = ? AND owner_id = ? AND revoked_at IS NULL`, tokenHash(token), token[:12], token[len(token)-4:], encrypted, m.clock().Unix(), tokenID, identity.OwnerID)
	if err != nil {
		return "", err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return "", ErrNotFound
	}
	if err := m.Audit(ctx, Identity{OwnerID: identity.OwnerID, TokenID: tokenID, Kind: identity.Kind, Prefix: token[:12]}, "token.rotated"); err != nil {
		return "", err
	}
	return token, nil
}

func (m *Manager) CreateLinked(ctx context.Context, identity Identity, name string, expiry *time.Time) (string, string, error) {
	if identity.Kind != "primary" {
		return "", "", ErrForbidden
	}
	name = strings.TrimSpace(name)
	if len(name) < 1 || len(name) > 64 {
		return "", "", errors.New("token name must be 1–64 characters")
	}
	var count int
	if err := m.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+m.table+` WHERE owner_id = ? AND kind = 'linked' AND revoked_at IS NULL AND (expires_at IS NULL OR expires_at > ?)`, identity.OwnerID, m.clock().Unix()).Scan(&count); err != nil {
		return "", "", err
	}
	if count >= m.maxLinked {
		return "", "", ErrTokenLimit
	}
	token, err := newToken()
	if err != nil {
		return "", "", err
	}
	encrypted, err := m.encrypt(token)
	if err != nil {
		return "", "", err
	}
	var expiryValue interface{}
	if expiry != nil {
		expiryValue = expiry.Unix()
	}
	id, err := uuid.NewV7()
	if err != nil {
		return "", "", err
	}
	_, err = m.execWrite(ctx, `INSERT INTO `+m.table+` (id, name, token_hash, token_prefix, token_last4, owner_id, kind, token_enc, created_at, expires_at) VALUES (?, ?, ?, ?, ?, ?, 'linked', ?, ?, ?)`, id.String(), name, tokenHash(token), token[:12], token[len(token)-4:], identity.OwnerID, encrypted, m.clock().Unix(), expiryValue)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return "", "", ErrDuplicate
		}
		return "", "", err
	}
	if err := m.Audit(ctx, Identity{OwnerID: identity.OwnerID, TokenID: id.String(), Kind: "linked", Prefix: token[:12]}, "token.created"); err != nil {
		return "", "", err
	}
	return id.String(), token, nil
}

func (m *Manager) UpdateLinked(ctx context.Context, identity Identity, tokenID string, name string, expiry *time.Time) error {
	if identity.Kind != "primary" {
		return ErrForbidden
	}
	name = strings.TrimSpace(name)
	if len(name) < 1 || len(name) > 64 {
		return errors.New("token name must be 1–64 characters")
	}
	var expiryValue interface{}
	if expiry != nil {
		expiryValue = expiry.Unix()
	}
	res, err := m.execWrite(ctx, `UPDATE `+m.table+` SET name = ?, expires_at = ? WHERE id = ? AND owner_id = ? AND kind = 'linked' AND revoked_at IS NULL`, name, expiryValue, tokenID, identity.OwnerID)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return ErrDuplicate
		}
		return err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return ErrNotFound
	}
	return nil
}

// Audit records a credential-management action without ever storing the raw token.
func (m *Manager) Audit(ctx context.Context, identity Identity, action string) error {
	_, err := m.execWrite(ctx, `INSERT INTO `+m.audit+` (id, timestamp, actor_type, actor_id, action, resource_type, resource_id, details) VALUES (?, ?, 'owner', ?, ?, 'api_token', ?, ?)`, uuid.NewString(), m.clock().Unix(), identity.OwnerID, action, identity.Prefix, `{"token_prefix":"`+identity.Prefix+`"}`)
	return err
}

func (m *Manager) RevokeLinked(ctx context.Context, identity Identity, tokenID string) error {
	if identity.Kind != "primary" {
		return ErrForbidden
	}
	var prefix string
	if err := m.db.QueryRowContext(ctx, `SELECT token_prefix FROM `+m.table+` WHERE id = ? AND owner_id = ? AND kind = 'linked' AND revoked_at IS NULL`, tokenID, identity.OwnerID).Scan(&prefix); errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return err
	}
	res, err := m.execWrite(ctx, `UPDATE `+m.table+` SET revoked_at = ?, revoked_reason = 'owner_revoked' WHERE id = ? AND owner_id = ? AND kind = 'linked' AND revoked_at IS NULL`, m.clock().Unix(), tokenID, identity.OwnerID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return ErrNotFound
	}
	return m.Audit(ctx, Identity{OwnerID: identity.OwnerID, TokenID: tokenID, Kind: "linked", Prefix: prefix}, "token.revoked")
}

// RevokeOwner atomically deletes an owner's resources and credentials after
// verifying the primary token's last four characters.
func (m *Manager) RevokeOwner(ctx context.Context, identity Identity, confirmation, source string) (OwnerRevocationCounts, error) {
	var counts OwnerRevocationCounts
	if identity.Kind != "primary" {
		return counts, ErrForbidden
	}
	err := m.withWrite(ctx, func(tx *sql.Tx) error {
		var last4 string
		if err := tx.QueryRowContext(ctx, `SELECT token_last4 FROM `+m.table+` WHERE id = ? AND owner_id = ? AND kind = 'primary' AND revoked_at IS NULL`, identity.TokenID, identity.OwnerID).Scan(&last4); errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		} else if err != nil {
			return err
		}
		if len(confirmation) != len(last4) || subtle.ConstantTimeCompare([]byte(confirmation), []byte(last4)) != 1 {
			return ErrConfirmation
		}
		alerts := strings.TrimSuffix(m.table, "api_tokens") + "search_alerts"
		results := strings.TrimSuffix(m.table, "api_tokens") + "search_alert_results"
		for query, target := range map[string]*int64{
			`SELECT COUNT(*) FROM ` + m.table + ` WHERE owner_id = ?`: &counts.Tokens,
			`SELECT COUNT(*) FROM ` + alerts + ` WHERE owner_id = ?`:   &counts.Resources,
		} {
			if err := tx.QueryRowContext(ctx, query, identity.OwnerID).Scan(target); err != nil {
				return err
			}
		}
		var resultCount int64
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+results+` WHERE alert_id IN (SELECT id FROM `+alerts+` WHERE owner_id = ?)`, identity.OwnerID).Scan(&resultCount); err != nil {
			return err
		}
		counts.Resources += resultCount
		if _, err := tx.ExecContext(ctx, `DELETE FROM `+results+` WHERE alert_id IN (SELECT id FROM `+alerts+` WHERE owner_id = ?)`, identity.OwnerID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM `+alerts+` WHERE owner_id = ?`, identity.OwnerID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM `+m.table+` WHERE owner_id = ?`, identity.OwnerID); err != nil {
			return err
		}
		deleted, err := tx.ExecContext(ctx, `DELETE FROM `+m.owners+` WHERE id = ?`, identity.OwnerID)
		if err != nil {
			return err
		}
		if affected, err := deleted.RowsAffected(); err != nil {
			return err
		} else if affected != 1 {
			return ErrNotFound
		}
		details, err := json.Marshal(map[string]interface{}{"scope": "owner", "tokens_deleted": counts.Tokens, "resources_deleted": counts.Resources, "source": source})
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO `+m.audit+` (id, timestamp, actor_type, actor_id, action, resource_type, resource_id, details) VALUES (?, ?, 'owner', ?, 'token.revoked', 'owner', ?, ?)`, uuid.NewString(), m.clock().Unix(), identity.OwnerID, identity.OwnerID, string(details))
		return err
	})
	return counts, err
}

func (m *Manager) Restore(ctx context.Context, token string) (Identity, error) {
	id, err := m.Resolve(ctx, token)
	if err != nil || id.Kind != "primary" {
		return Identity{}, ErrInvalidToken
	}
	if err := m.Audit(ctx, id, "token.restored"); err != nil {
		return Identity{}, err
	}
	return id, nil
}

func validToken(token string) bool {
	if len(token) != 36 || (!strings.HasPrefix(token, "own_") && !strings.HasPrefix(token, "tok_")) {
		return false
	}
	for _, c := range token[4:] {
		if !strings.ContainsRune(tokenAlphabet, c) {
			return false
		}
	}
	return true
}

func newToken() (string, error) {
	var b strings.Builder
	b.Grow(36)
	b.WriteString("own_")
	for b.Len() < 36 {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(tokenAlphabet))))
		if err != nil {
			return "", err
		}
		b.WriteByte(tokenAlphabet[n.Int64()])
	}
	return b.String(), nil
}

func tokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func unixTime(v sql.NullInt64) *time.Time {
	if !v.Valid {
		return nil
	}
	t := time.Unix(v.Int64, 0).UTC()
	return &t
}

func (m *Manager) encrypt(token string) (string, error) {
	block, err := aes.NewCipher(m.key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nonce, nonce, []byte(token), nil)
	return base64.RawStdEncoding.EncodeToString(sealed), nil
}

func (m *Manager) decrypt(encrypted string) (string, error) {
	sealed, err := base64.RawStdEncoding.DecodeString(encrypted)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(m.key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(sealed) < gcm.NonceSize() {
		return "", fmt.Errorf("encrypted owner token is truncated")
	}
	nonce, ciphertext := sealed[:gcm.NonceSize()], sealed[gcm.NonceSize():]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}
