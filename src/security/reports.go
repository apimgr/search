package security

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/ProtonMail/gopenpgp/v2/crypto"
	"github.com/apimgr/search/src/database"
)

var reportEmailPattern = regexp.MustCompile(`(?i)[a-z0-9._%+-]+@[a-z0-9.-]+\.[a-z]{2,}`)

// EnsurePreviousEncryptionKey migrates legacy plaintext previous-key values
// into the canonical wrapped app_secrets rows and returns the active grace key.
func EnsurePreviousEncryptionKey(ctx context.Context, db *database.DB, currentKey, legacyPrevious, legacyUntil string) (string, time.Time, error) {
	if db == nil || currentKey == "" {
		return "", time.Time{}, fmt.Errorf("encryption key metadata requires the database and active key")
	}
	previousRow, err := database.GetSecret(ctx, db, database.SecretEncryptionKeyPrevious)
	if errors.Is(err, sql.ErrNoRows) && legacyPrevious != "" {
		if legacyUntil == "" {
			return "", time.Time{}, fmt.Errorf("legacy previous encryption key has no expiry")
		}
		if err := StorePreviousEncryptionKey(ctx, db, currentKey, legacyPrevious, legacyUntil); err != nil {
			return "", time.Time{}, err
		}
		previousRow, err = database.GetSecret(ctx, db, database.SecretEncryptionKeyPrevious)
	}
	if errors.Is(err, sql.ErrNoRows) {
		return "", time.Time{}, nil
	}
	if err != nil {
		return "", time.Time{}, err
	}
	untilRow, err := database.GetSecret(ctx, db, database.SecretEncryptionKeyPreviousUntil)
	if errors.Is(err, sql.ErrNoRows) {
		// Older releases stored the expiry with the *_expires suffix.
		untilRow, err = database.GetSecret(ctx, db, "encryption_key_previous_expires")
		if err == nil {
			if err := database.SetAppSecretValue(ctx, db, database.SecretEncryptionKeyPreviousUntil, untilRow.Value); err != nil {
				return "", time.Time{}, err
			}
			if err := database.DeleteAppSecretValue(ctx, db, "encryption_key_previous_expires"); err != nil {
				return "", time.Time{}, err
			}
		}
	}
	if err != nil {
		return "", time.Time{}, err
	}
	until, err := time.Parse(time.RFC3339, strings.TrimSpace(untilRow.Value))
	if err != nil {
		return "", time.Time{}, fmt.Errorf("parse previous encryption key expiry")
	}
	previous, err := DecryptAESGCM(currentKey, previousRow.Value)
	if err != nil {
		// Older versions stored the previous AES key as a base64-encoded 32-byte
		// value. Convert it before any consumer relies on the grace fallback.
		decoded, decodeErr := base64.StdEncoding.DecodeString(previousRow.Value)
		if decodeErr != nil || len(decoded) != 32 {
			return "", time.Time{}, fmt.Errorf("previous encryption key cannot be unwrapped")
		}
		legacy := previousRow.Value
		if legacyPrevious != "" && legacyPrevious != legacy {
			return "", time.Time{}, fmt.Errorf("legacy previous encryption key does not match stored value")
		}
		if err := StorePreviousEncryptionKey(ctx, db, currentKey, legacy, until.UTC().Format(time.RFC3339)); err != nil {
			return "", time.Time{}, err
		}
		previous = []byte(legacy)
	}
	if !time.Now().UTC().Before(until) {
		return "", until, nil
	}
	return string(previous), until, nil
}

// StorePreviousEncryptionKey stores the previous key wrapped by currentKey
// and its grace expiry in app_secrets in one transaction.
func StorePreviousEncryptionKey(ctx context.Context, db *database.DB, currentKey, previousKey, expires string) error {
	wrapped, err := EncryptAESGCM(currentKey, []byte(previousKey))
	if err != nil {
		return fmt.Errorf("wrap previous encryption key: %w", err)
	}
	table := database.ServerTableName(db, "app_secrets")
	return db.Write(ctx, func(tx *sql.Tx) error {
		for _, item := range []struct{ name, value string }{{database.SecretEncryptionKeyPrevious, wrapped}, {database.SecretEncryptionKeyPreviousUntil, expires}} {
			if _, err := database.ExecContext(ctx, tx, fmt.Sprintf(`INSERT INTO %s (name,value,created_at,rotated_at) VALUES (?,?,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP) ON CONFLICT(name) DO UPDATE SET value=excluded.value,rotated_at=CURRENT_TIMESTAMP`, table), item.name, item.value); err != nil {
				return fmt.Errorf("store %s: %w", item.name, err)
			}
		}
		return nil
	})
}

// ReencryptAESReportData resumes AES report-body and contact rotation in
// bounded, restartable batches. PGP report bodies are intentionally untouched.
func ReencryptAESReportData(ctx context.Context, db *database.DB, currentKey, previousKey string) (int, error) {
	if db == nil || currentKey == "" {
		return 0, fmt.Errorf("security report database and current key are required")
	}
	table := database.ServerTableName(db, "security_reports")
	var cursor string
	total := 0
	for {
		var pending []struct {
			id            string
			body, contact []byte
			alg           string
		}
		err := db.Write(ctx, func(tx *sql.Tx) error {
			rows, err := database.QueryContext(ctx, tx, database.TimeoutBulk, fmt.Sprintf(`SELECT tracking_id,body_enc,body_alg,contact_enc FROM %s WHERE tracking_id>? AND (body_alg=? OR contact_enc IS NOT NULL) ORDER BY tracking_id LIMIT 500`, table), cursor, EncryptionMethodAESGCM)
			if err != nil {
				return err
			}
			for rows.Next() {
				var item struct {
					id            string
					body, contact []byte
					alg           string
				}
				if err := rows.Scan(&item.id, &item.body, &item.alg, &item.contact); err != nil {
					return errors.Join(err, rows.Close())
				}
				pending = append(pending, item)
			}
			if err := rows.Err(); err != nil {
				return errors.Join(err, rows.Close())
			}
			return rows.Close()
		})
		if err != nil {
			return total, fmt.Errorf("read security report ciphertext batch: %w", err)
		}
		if len(pending) == 0 {
			return total, nil
		}
		err = db.Write(ctx, func(tx *sql.Tx) error {
			for _, item := range pending {
				cursor = item.id
				body := item.body
				if item.alg == EncryptionMethodAESGCM && len(body) > 0 {
					_, decErr := DecryptAESGCM(currentKey, string(body))
					if decErr != nil {
						if previousKey == "" {
							return fmt.Errorf("security.decrypt_failed table=security_reports row=%s field=body", item.id)
						}
						plain, decErr := DecryptAESGCM(previousKey, string(body))
						if decErr != nil {
							return fmt.Errorf("security.decrypt_failed table=security_reports row=%s field=body", item.id)
						}
						encoded, encErr := EncryptAESGCM(currentKey, plain)
						if encErr != nil {
							return fmt.Errorf("re-encrypt security report %s body: %w", item.id, encErr)
						}
						body = []byte(encoded)
					}
				}
				contact := item.contact
				if len(contact) > 0 {
					_, currentErr := DecryptAESGCM(currentKey, string(contact))
					if currentErr != nil {
						if previousKey == "" {
							return fmt.Errorf("security.decrypt_failed table=security_reports row=%s field=contact_enc", item.id)
						}
						plain, decErr := DecryptAESGCM(previousKey, string(contact))
						if decErr != nil {
							return fmt.Errorf("security.decrypt_failed table=security_reports row=%s field=contact_enc", item.id)
						}
						encoded, encErr := EncryptAESGCM(currentKey, plain)
						if encErr != nil {
							return fmt.Errorf("re-encrypt security report %s contact: %w", item.id, encErr)
						}
						contact = []byte(encoded)
					}
				}
				if bytes.Equal(body, item.body) && bytes.Equal(contact, item.contact) {
					continue
				}
				result, err := database.ExecContext(ctx, tx, fmt.Sprintf("UPDATE %s SET body_enc=?,contact_enc=?,updated_at=CURRENT_TIMESTAMP WHERE tracking_id=? AND body_enc=? AND COALESCE(contact_enc,X'')=COALESCE(?,X'')", table), body, nullableReportCipher(contact), item.id, item.body, nullableReportCipher(item.contact))
				if err != nil {
					return fmt.Errorf("update security report %s: %w", item.id, err)
				}
				count, err := result.RowsAffected()
				if err != nil {
					return err
				}
				if count != 1 {
					return fmt.Errorf("security report %s changed during encryption rotation", item.id)
				}
				total++
			}
			return nil
		})
		if err != nil {
			return total, err
		}
		if len(pending) < 500 {
			return total, nil
		}
	}
}

func nullableReportCipher(value []byte) interface{} {
	if len(value) == 0 {
		return nil
	}
	return value
}

// MigrateLegacyReportPlaintext decrypts legacy report bodies using their
// recorded algorithm, folds plaintext fields into the body, and then encrypts
// the complete body and extracted contact under the current project keys.
func MigrateLegacyReportPlaintext(ctx context.Context, db *database.DB, encryptionKey, previousEncryptionKey, publicKey, encryptedPrivateKey, installationSecret string) (int, error) {
	if db == nil || encryptionKey == "" {
		return 0, fmt.Errorf("security report migration requires the database and encryption key")
	}
	table := database.ServerTableName(db, "security_reports")
	columns := map[string]bool{}
	rows, err := db.QueryWithTimeout(ctx, database.TimeoutMigration, "PRAGMA table_info("+table+")")
	if err != nil {
		return 0, err
	}
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, typ string
		var def interface{}
		if err := rows.Scan(&cid, &name, &typ, &notNull, &def, &primaryKey); err != nil {
			return 0, errors.Join(err, rows.Close())
		}
		columns[name] = true
	}
	if err := rows.Err(); err != nil {
		return 0, errors.Join(err, rows.Close())
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}
	if !columns["summary"] && !columns["affected_endpoint"] && !columns["encrypted_body"] {
		return 0, nil
	}
	legacyExpr := func(name string) string {
		if columns[name] {
			return name
		}
		return "''"
	}
	type legacyRow struct {
		id                     string
		body                   []byte
		alg, summary, endpoint string
	}
	var migrated int
	err = db.Write(ctx, func(tx *sql.Tx) error {
		legacyBodyCondition := "0"
		if columns["encrypted_body"] {
			legacyBodyCondition = "(encrypted_body IS NOT NULL AND body_enc = encrypted_body)"
		}
		query := fmt.Sprintf("SELECT tracking_id,body_enc,body_alg,%s,%s FROM %s WHERE COALESCE(%s,'')<>'' OR COALESCE(%s,'')<>'' OR %s", legacyExpr("summary"), legacyExpr("affected_endpoint"), table, legacyExpr("summary"), legacyExpr("affected_endpoint"), legacyBodyCondition)
		rows, err := database.QueryContext(ctx, tx, database.TimeoutMigration, query)
		if err != nil {
			return err
		}
		var pending []legacyRow
		for rows.Next() {
			var r legacyRow
			if err := rows.Scan(&r.id, &r.body, &r.alg, &r.summary, &r.endpoint); err != nil {
				return errors.Join(err, rows.Close())
			}
			pending = append(pending, r)
		}
		if err := rows.Err(); err != nil {
			return errors.Join(err, rows.Close())
		}
		if err := rows.Close(); err != nil {
			return err
		}
		var oldPrivate *crypto.Key
		for _, r := range pending {
			body := string(r.body)
			if len(r.body) > 0 {
				switch r.alg {
				case EncryptionMethodAESGCM:
					plain, decErr := DecryptAESGCM(encryptionKey, body)
					if decErr != nil && previousEncryptionKey != "" {
						plain, decErr = DecryptAESGCM(previousEncryptionKey, body)
					}
					if decErr != nil {
						return fmt.Errorf("decrypt legacy AES report %s: %w", r.id, decErr)
					}
					body = string(plain)
				case EncryptionMethodPGP:
					if oldPrivate == nil {
						if encryptedPrivateKey == "" || installationSecret == "" {
							return fmt.Errorf("legacy PGP report %s cannot be decrypted without the project private key", r.id)
						}
						var unlockErr error
						oldPrivate, unlockErr = UnlockPrivateKey(encryptedPrivateKey, installationSecret)
						if unlockErr != nil {
							return fmt.Errorf("unlock legacy PGP private key: %w", unlockErr)
						}
					}
					plain, decErr := DecryptArmoredMessage(oldPrivate, body)
					if decErr != nil {
						return fmt.Errorf("decrypt legacy PGP report %s: %w", r.id, decErr)
					}
					body = string(plain)
				default:
					return fmt.Errorf("legacy security report %s has unsupported body algorithm %q", r.id, r.alg)
				}
			}
			if r.summary != "" {
				body += "\n\nLegacy summary:\n" + r.summary
			}
			if r.endpoint != "" {
				body += "\n\nLegacy affected endpoint:\n" + r.endpoint
			}
			var contact struct {
				Email         string `json:"email,omitempty"`
				ResearcherGPG string `json:"researcher_gpg,omitempty"`
			}
			if match := reportEmailPattern.FindString(body); match != "" {
				contact.Email = match
			}
			if start := strings.Index(body, "-----BEGIN PGP PUBLIC KEY BLOCK-----"); start >= 0 {
				if end := strings.Index(body[start:], "-----END PGP PUBLIC KEY BLOCK-----"); end >= 0 {
					contact.ResearcherGPG = body[start : start+end+len("-----END PGP PUBLIC KEY BLOCK-----")]
				}
			}
			var encrypted, algorithm string
			if publicKey != "" {
				encrypted, err = EncryptMessageToArmoredKey(publicKey, []byte(body))
				algorithm = EncryptionMethodPGP
			} else {
				encrypted, err = EncryptAESGCM(encryptionKey, []byte(body))
				algorithm = EncryptionMethodAESGCM
			}
			if err != nil {
				return fmt.Errorf("encrypt migrated report %s: %w", r.id, err)
			}
			var contactCipher interface{}
			if contact.Email != "" || contact.ResearcherGPG != "" {
				data, marshalErr := json.Marshal(contact)
				if marshalErr != nil {
					return marshalErr
				}
				cipher, encryptErr := EncryptAESGCM(encryptionKey, data)
				if encryptErr != nil {
					return fmt.Errorf("encrypt migrated contact %s: %w", r.id, encryptErr)
				}
				contactCipher = []byte(cipher)
			}
			clearSQL := ""
			if columns["summary"] {
				clearSQL += ", summary = ''"
			}
			if columns["affected_endpoint"] {
				clearSQL += ", affected_endpoint = ''"
			}
			if columns["encrypted_body"] {
				clearSQL += ", encrypted_body = NULL"
			}
			if columns["encryption_method"] {
				clearSQL += ", encryption_method = ''"
			}
			if _, err := database.ExecContext(ctx, tx, fmt.Sprintf("UPDATE %s SET body_enc=?,body_alg=?,contact_enc=COALESCE(contact_enc,?),updated_at=CURRENT_TIMESTAMP%s WHERE tracking_id=?", table, clearSQL), []byte(encrypted), algorithm, contactCipher, r.id); err != nil {
				return fmt.Errorf("migrate legacy report %s: %w", r.id, err)
			}
			migrated++
		}
		return nil
	})
	if err != nil {
		return migrated, err
	}
	return migrated, nil
}

// MigrateSecurityThanks fills missing durable acknowledgment rows for reports
// that were already disclosed before the security_thanks table existed.
func MigrateSecurityThanks(ctx context.Context, db *database.DB) (int, error) {
	if db == nil {
		return 0, fmt.Errorf("security report database is unavailable")
	}
	reports := database.ServerTableName(db, "security_reports")
	thanks := database.ServerTableName(db, "security_thanks")
	inserted := 0
	err := db.Write(ctx, func(tx *sql.Tx) error {
		rows, err := database.QueryContext(ctx, tx, database.TimeoutReport, fmt.Sprintf(`SELECT r.tracking_id,r.credit_pref,r.credit_name,r.severity,COALESCE(date(r.closed_at),date(r.created_at),date('now'))
			FROM %s r LEFT JOIN %s t ON t.tracking_id=r.tracking_id
			WHERE r.status='disclosed' AND r.credit_pref<>'no' AND t.tracking_id IS NULL
			ORDER BY COALESCE(r.closed_at,r.created_at),r.tracking_id`, reports, thanks))
		if err != nil {
			return err
		}
		type row struct{ id, pref, name, severity, date string }
		var pending []row
		for rows.Next() {
			var item row
			if err := rows.Scan(&item.id, &item.pref, &item.name, &item.severity, &item.date); err != nil {
				return errors.Join(err, rows.Close())
			}
			pending = append(pending, item)
		}
		if err := rows.Err(); err != nil {
			return errors.Join(err, rows.Close())
		}
		if err := rows.Close(); err != nil {
			return err
		}
		for _, item := range pending {
			credit := item.name
			if item.pref == "anon" {
				var count int
				countQuery := "SELECT COUNT(*) FROM " + thanks + " WHERE credit LIKE 'Anonymous Researcher #%'"
				if err := database.QueryRowContext(ctx, tx, database.TimeoutReport, countQuery).Scan(&count); err != nil {
					return err
				}
				credit = fmt.Sprintf("Anonymous Researcher #%d", count+1)
			}
			if credit == "" {
				continue
			}
			if _, err := database.ExecContext(ctx, tx, fmt.Sprintf("INSERT INTO %s (tracking_id,credit,severity,disclosed_on) VALUES (?,?,?,?) ON CONFLICT(tracking_id) DO NOTHING", thanks), item.id, credit, item.severity, item.date); err != nil {
				return err
			}
			inserted++
		}
		return nil
	})
	if err != nil {
		return inserted, fmt.Errorf("migrate security acknowledgments: %w", err)
	}
	return inserted, nil
}

// TrackingIDPrefix is the fixed prefix for coordinated-disclosure tracking
// ids per AI.md PART 11 Submission Flow step 2.
const TrackingIDPrefix = "sec_"

// TrackingIDRandomHexChars is the number of random hex characters appended
// to TrackingIDPrefix.
const TrackingIDRandomHexChars = 16

// ReportTokenExpiryAfterClose is how long a researcher's status-page token
// stays valid after the report is closed, per AI.md PART 11 Public Pages
// table ("expires after the report is closed for 30 days").
const ReportTokenExpiryAfterClose = 30 * 24 * time.Hour

// EncryptionMethodPGP and EncryptionMethodAESGCM identify how a report body
// was encrypted at rest, stored in {prefix}security_reports.body_alg.
const (
	EncryptionMethodPGP    = "pgp"
	EncryptionMethodAESGCM = "aes-256-gcm"
)

// Report holds the fields of a coordinated-disclosure security report,
// per AI.md PART 11 "Security-mode form fields" table.
type Report struct {
	TrackingID        string
	IDWindow          int64
	Severity          string
	Component         string
	BodyEnc           []byte
	BodyAlg           string
	ContactEnc        []byte
	CreditPref        string
	CreditName        string
	ResearcherComment string
	DisclosureDate    *time.Time
	CVERequested      bool
	TokenHash         string
	Summary           string
}

// GenerateTrackingID allocates a new "sec_" + 16 random hex character
// tracking id per AI.md PART 11 Submission Flow step 2.
func GenerateTrackingID() (string, error) {
	buf := make([]byte, TrackingIDRandomHexChars/2)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate tracking id: %w", err)
	}
	return TrackingIDPrefix + hex.EncodeToString(buf), nil
}

// GenerateReportToken creates a one-shot researcher status-page token and
// its SHA-256 hash. The raw token is emailed to the researcher and never
// persisted; only the hash is stored in report_token_hash.
func GenerateReportToken() (rawToken, hash string, err error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", "", fmt.Errorf("generate report token: %w", err)
	}
	rawToken = hex.EncodeToString(buf)
	sum := sha256.Sum256([]byte(rawToken))
	hash = hex.EncodeToString(sum[:])
	return rawToken, hash, nil
}

// HashReportToken hashes a researcher-supplied token for comparison against
// the stored report_token_hash.
func HashReportToken(rawToken string) string {
	sum := sha256.Sum256([]byte(rawToken))
	return hex.EncodeToString(sum[:])
}

// ReportTokenMatches compares a submitted token with its stored SHA-256 hash.
func ReportTokenMatches(rawToken, storedHash string) bool {
	stored, err := hex.DecodeString(storedHash)
	if err != nil || len(stored) != sha256.Size {
		return false
	}
	presented := sha256.Sum256([]byte(rawToken))
	return subtle.ConstantTimeCompare(presented[:], stored) == 1
}

// InsertReport persists a new security report row into {prefix}security_reports.
// The report body must already be encrypted (see EncryptMessageToArmoredKey /
// EncryptAESGCM) — plaintext is never written to disk, per AI.md PART 11.
func InsertReport(ctx context.Context, db *database.DB, report Report) error {
	table := database.ServerTableName(db, "security_reports")
	query := fmt.Sprintf("INSERT INTO %s (tracking_id, id_window, severity, component, body_enc, body_alg, contact_enc, credit_pref, credit_name, researcher_comment, disclosure_date, cve_requested, token_hash) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)", table)
	_, err := db.Exec(ctx, query,
		report.TrackingID, report.IDWindow, report.Severity, report.Component,
		report.BodyEnc, report.BodyAlg, report.ContactEnc, report.CreditPref,
		report.CreditName, report.ResearcherComment, report.DisclosureDate,
		boolToInt(report.CVERequested), report.TokenHash)
	if err != nil {
		return fmt.Errorf("insert security report: %w", err)
	}
	return nil
}

// ReencryptPGPReportBodiesTx moves every PGP-encrypted report body to newPublic
// inside the caller's key-rotation transaction. Contact ciphertext is AES-GCM
// and is intentionally untouched.
func ReencryptPGPReportBodiesTx(ctx context.Context, tx *sql.Tx, db *database.DB, oldKey *crypto.Key, newPublic string) (int, error) {
	table := database.ServerTableName(db, "security_reports")
	rows, err := database.QueryContext(ctx, tx, database.TimeoutBulk, fmt.Sprintf("SELECT tracking_id, body_enc FROM %s WHERE body_alg = ?", table), EncryptionMethodPGP)
	if err != nil {
		return 0, fmt.Errorf("select PGP security report bodies: %w", err)
	}
	type pending struct {
		trackingID string
		body       string
	}
	updates := make([]pending, 0)
	for rows.Next() {
		var item pending
		var encrypted []byte
		if err := rows.Scan(&item.trackingID, &encrypted); err != nil {
			return 0, errors.Join(fmt.Errorf("read PGP security report body: %w", err), rows.Close())
		}
		plaintext, err := DecryptArmoredMessage(oldKey, string(encrypted))
		if err != nil {
			return 0, errors.Join(fmt.Errorf("decrypt security report %s: %w", item.trackingID, err), rows.Close())
		}
		item.body, err = EncryptMessageToArmoredKey(newPublic, plaintext)
		if err != nil {
			return 0, errors.Join(fmt.Errorf("re-encrypt security report %s: %w", item.trackingID, err), rows.Close())
		}
		updates = append(updates, item)
	}
	if err := rows.Err(); err != nil {
		return 0, errors.Join(fmt.Errorf("iterate PGP security report bodies: %w", err), rows.Close())
	}
	if err := rows.Close(); err != nil {
		return 0, fmt.Errorf("close PGP security report rows: %w", err)
	}
	for _, item := range updates {
		query := fmt.Sprintf("UPDATE %s SET body_enc = ?, updated_at = CURRENT_TIMESTAMP WHERE tracking_id = ? AND body_alg = ?", table)
		if _, err := database.ExecContext(ctx, tx, query, []byte(item.body), item.trackingID, EncryptionMethodPGP); err != nil {
			return 0, fmt.Errorf("update PGP security report %s: %w", item.trackingID, err)
		}
	}
	return len(updates), nil
}

// ReportStatus is the researcher-visible status of a report per AI.md
// PART 11 Public Pages table.
type ReportStatus struct {
	TrackingID      string
	Status          string
	ResearcherComment string
	DisclosureDate string
	ReportTokenHash string
	ClosedAt        *time.Time
	LastAccessDate  string
}

// LookupReportStatus fetches the researcher-visible status fields for a
// tracking id, for the "/server/security/report/{tracking_id}" status page.
// Returns (nil, nil) if no report exists with that tracking id.
func LookupReportStatus(ctx context.Context, db *database.DB, trackingID string) (*ReportStatus, error) {
	table := database.ServerTableName(db, "security_reports")
	query := fmt.Sprintf("SELECT tracking_id, status, COALESCE(researcher_comment, ''), COALESCE(disclosure_date, ''), token_hash, closed_at, token_last_used FROM %s WHERE tracking_id = ?", table)
	row := db.QueryRow(ctx, query, trackingID)

	var status ReportStatus
	var closedAt, lastAccessDate *string
	if err := row.Scan(&status.TrackingID, &status.Status, &status.ResearcherComment, &status.DisclosureDate, &status.ReportTokenHash, &closedAt, &lastAccessDate); err != nil {
		return nil, err
	}
	if lastAccessDate != nil {
		status.LastAccessDate = *lastAccessDate
	}
	if closedAt != nil && *closedAt != "" {
		if parsed, err := time.Parse(time.RFC3339, *closedAt); err == nil {
			status.ClosedAt = &parsed
		} else if parsed, err := time.Parse("2006-01-02 15:04:05", *closedAt); err == nil {
			status.ClosedAt = &parsed
		}
	}
	return &status, nil
}

// RecordReportStatusAccess permits one successful researcher status lookup
// per UTC calendar day, atomically across concurrent requests.
func RecordReportStatusAccess(ctx context.Context, db *database.DB, trackingID, accessDate string) (bool, error) {
	table := database.ServerTableName(db, "security_reports")
	query := fmt.Sprintf("UPDATE %s SET token_last_used = ? WHERE tracking_id = ? AND (token_last_used IS NULL OR token_last_used <> ?)", table)
	result, err := db.Exec(ctx, query, accessDate, trackingID, accessDate)
	if err != nil {
		return false, fmt.Errorf("record security report status access: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("read security report status access result: %w", err)
	}
	return count == 1, nil
}

// DeleteClosedReportsBefore removes only closed reports older than cutoff,
// committing at most 500 rows per transaction and yielding between batches.
func DeleteClosedReportsBefore(ctx context.Context, db *database.DB, cutoff time.Time) (int64, error) {
	if db == nil {
		return 0, fmt.Errorf("security report database is unavailable")
	}
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	table := database.ServerTableName(db, "security_reports")
	var total int64
	for {
		var deleted int64
		err := db.Write(ctx, func(tx *sql.Tx) error {
			query := fmt.Sprintf("DELETE FROM %s WHERE tracking_id IN (SELECT tracking_id FROM %s WHERE closed_at IS NOT NULL AND datetime(closed_at) < datetime(?) ORDER BY closed_at LIMIT 500)", table, table)
			result, err := database.ExecContext(ctx, tx, query, cutoff.UTC().Format(time.RFC3339))
			if err != nil {
				return fmt.Errorf("delete expired security reports: %w", err)
			}
			deleted, err = result.RowsAffected()
			return err
		})
		if err != nil {
			return total, err
		}
		total += deleted
		if deleted < 500 {
			return total, nil
		}
		timer := time.NewTimer(10 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return total, ctx.Err()
		case <-timer.C:
		}
	}
}

// TokenExpired reports whether a report's one-shot status-page token has
// expired: 30 days after the report was closed, per AI.md PART 11.
func (rs *ReportStatus) TokenExpired(now time.Time) bool {
	if rs.ClosedAt == nil {
		return false
	}
	return now.After(rs.ClosedAt.Add(ReportTokenExpiryAfterClose))
}

// boolToInt converts a bool to the 0/1 form used by the cve_requested column.
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
