package security

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/apimgr/search/src/database"
)

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
// was encrypted at rest, stored in {prefix}security_reports.encryption_method.
const (
	EncryptionMethodPGP    = "pgp"
	EncryptionMethodAESGCM = "aes-256-gcm"
)

// Report holds the fields of a coordinated-disclosure security report,
// per AI.md PART 11 "Security-mode form fields" table.
type Report struct {
	TrackingID        string
	SecurityIDUsed    string
	AffectedComponent string
	AffectedEndpoint  string
	Severity          string
	Summary           string
	EncryptedBody     []byte
	EncryptionMethod  string
	CreditPreference  string
	CreditName        string
	DisclosureDays    int
	CVERequested      bool
	ReportTokenHash   string
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
	_, err := db.Exec(ctx, fmt.Sprintf(
		`INSERT INTO %s (
			tracking_id, security_id_used, affected_component, affected_endpoint,
			severity, summary, encrypted_body, encryption_method,
			credit_preference, credit_name, disclosure_days, cve_requested,
			report_token_hash
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, table),
		report.TrackingID, report.SecurityIDUsed, report.AffectedComponent, report.AffectedEndpoint,
		report.Severity, report.Summary, report.EncryptedBody, report.EncryptionMethod,
		report.CreditPreference, report.CreditName, report.DisclosureDays, boolToInt(report.CVERequested),
		report.ReportTokenHash,
	)
	if err != nil {
		return fmt.Errorf("insert security report: %w", err)
	}
	return nil
}

// ReportStatus is the researcher-visible status of a report per AI.md
// PART 11 Public Pages table.
type ReportStatus struct {
	TrackingID      string
	Status          string
	ReportTokenHash string
	ClosedAt        *time.Time
	LastAccessDate  string
}

// LookupReportStatus fetches the researcher-visible status fields for a
// tracking id, for the "/server/security/report/{tracking_id}" status page.
// Returns (nil, nil) if no report exists with that tracking id.
func LookupReportStatus(ctx context.Context, db *database.DB, trackingID string) (*ReportStatus, error) {
	table := database.ServerTableName(db, "security_reports")
	row := db.QueryRow(ctx, fmt.Sprintf(
		`SELECT tracking_id, status, report_token_hash, closed_at, last_status_access_date FROM %s WHERE tracking_id = ?`, table),
		trackingID)

	var status ReportStatus
	var closedAt, lastAccessDate *string
	if err := row.Scan(&status.TrackingID, &status.Status, &status.ReportTokenHash, &closedAt, &lastAccessDate); err != nil {
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
	result, err := db.Exec(ctx, fmt.Sprintf(
		`UPDATE %s SET last_status_access_date = ?
		WHERE tracking_id = ? AND (last_status_access_date IS NULL OR last_status_access_date <> ?)`, table),
		accessDate, trackingID, accessDate,
	)
	if err != nil {
		return false, fmt.Errorf("record security report status access: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("read security report status access result: %w", err)
	}
	return count == 1, nil
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
