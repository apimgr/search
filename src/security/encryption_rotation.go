package security

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/apimgr/search/src/database"
)

type encryptionRotationPlan struct {
	NewKey string `json:"new_key"`
	Until  string `json:"until"`
	Reason string `json:"reason,omitempty"`
}

// BeginEncryptionKeyRotation stores a recoverable rotation plan and the
// wrapped previous key before the caller persists the new current key.
func BeginEncryptionKeyRotation(ctx context.Context, db *database.DB, currentKey, newKey string, until time.Time, reason string) error {
	if db == nil || currentKey == "" || newKey == "" || until.IsZero() {
		return fmt.Errorf("encryption rotation requires both keys and an expiry")
	}
	plan, err := json.Marshal(encryptionRotationPlan{NewKey: newKey, Until: until.UTC().Format(time.RFC3339), Reason: reason})
	if err != nil {
		return err
	}
	wrappedPlan, err := EncryptAESGCM(currentKey, plan)
	if err != nil {
		return fmt.Errorf("protect encryption rotation recovery plan: %w", err)
	}
	if err := database.SetAppSecretValue(ctx, db, database.SecretEncryptionKeyRotationPending, wrappedPlan); err != nil {
		return err
	}
	if err := StorePreviousEncryptionKey(ctx, db, newKey, currentKey, until.UTC().Format(time.RFC3339)); err != nil {
		return err
	}
	return nil
}

// ResumeEncryptionKeyRotation recognizes a crash on either side of the
// secrets.yml atomic replacement. If the current key still opens the pending
// plan, it returns the new key for the caller to persist before any reads or
// writes; otherwise it verifies the plan with the wrapped previous key.
func ResumeEncryptionKeyRotation(ctx context.Context, db *database.DB, currentKey string) (activeKey, previousKey string, until time.Time, reason string, pending bool, err error) {
	if db == nil || currentKey == "" {
		return "", "", time.Time{}, "", false, fmt.Errorf("encryption rotation recovery requires database and current key")
	}
	row, err := database.GetSecret(ctx, db, database.SecretEncryptionKeyRotationPending)
	if errors.Is(err, sql.ErrNoRows) {
		previousKey, until, err = EnsurePreviousEncryptionKey(ctx, db, currentKey, "", "")
		return currentKey, previousKey, until, "", false, err
	}
	if err != nil {
		return "", "", time.Time{}, "", false, err
	}
	planBytes, currentErr := DecryptAESGCM(currentKey, row.Value)
	if currentErr == nil {
		var plan encryptionRotationPlan
		if err := json.Unmarshal(planBytes, &plan); err != nil || plan.NewKey == "" {
			return "", "", time.Time{}, "", false, fmt.Errorf("invalid encryption rotation recovery plan")
		}
		until, err := time.Parse(time.RFC3339, plan.Until)
		if err != nil {
			return "", "", time.Time{}, "", false, fmt.Errorf("invalid encryption rotation expiry")
		}
		if err := StorePreviousEncryptionKey(ctx, db, plan.NewKey, currentKey, until.UTC().Format(time.RFC3339)); err != nil {
			return "", "", time.Time{}, "", false, err
		}
		return plan.NewKey, currentKey, until.UTC(), plan.Reason, true, nil
	}
	previousKey, until, err = EnsurePreviousEncryptionKey(ctx, db, currentKey, "", "")
	if err != nil {
		return "", "", time.Time{}, "", false, err
	}
	if previousKey == "" {
		return "", "", time.Time{}, "", false, fmt.Errorf("pending encryption rotation cannot be opened by current or previous key")
	}
	planBytes, err = DecryptAESGCM(previousKey, row.Value)
	if err != nil {
		return "", "", time.Time{}, "", false, fmt.Errorf("pending encryption rotation recovery plan is unreadable")
	}
	var plan encryptionRotationPlan
	if err := json.Unmarshal(planBytes, &plan); err != nil || plan.NewKey != currentKey {
		return "", "", time.Time{}, "", false, fmt.Errorf("pending encryption rotation does not match the active key")
	}
	if until.IsZero() {
		until, err = time.Parse(time.RFC3339, plan.Until)
		if err != nil {
			return "", "", time.Time{}, "", false, fmt.Errorf("invalid encryption rotation expiry")
		}
	}
	return currentKey, previousKey, until.UTC(), plan.Reason, true, nil
}

// CompleteEncryptionKeyRotation removes the temporary recovery record only
// after the caller has durably saved the active key.
func CompleteEncryptionKeyRotation(ctx context.Context, db *database.DB) error {
	return database.DeleteAppSecretValue(ctx, db, database.SecretEncryptionKeyRotationPending)
}

// CleanupExpiredPreviousEncryptionKey retires wrapped previous-key material
// after its recorded grace period, as required by the daily security cleanup.
func CleanupExpiredPreviousEncryptionKey(ctx context.Context, db *database.DB, currentKey string, now time.Time) (bool, error) {
	if db == nil || currentKey == "" {
		return false, fmt.Errorf("previous-key cleanup requires database and current key")
	}
	row, err := database.GetSecret(ctx, db, database.SecretEncryptionKeyPreviousUntil)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	until, err := time.Parse(time.RFC3339, row.Value)
	if err != nil {
		return false, fmt.Errorf("parse previous encryption-key expiry")
	}
	if now.UTC().Before(until) {
		return false, nil
	}
	// Validate that the wrapped value is readable before deleting it; never
	// place plaintext key material in an error or log.
	previous, err := database.GetSecret(ctx, db, database.SecretEncryptionKeyPrevious)
	if err == nil {
		if _, err := DecryptAESGCM(currentKey, previous.Value); err != nil {
			return false, fmt.Errorf("expired previous encryption key cannot be unwrapped")
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	for _, name := range []string{database.SecretEncryptionKeyPrevious, database.SecretEncryptionKeyPreviousUntil, "encryption_key_previous_expires"} {
		if err := database.DeleteAppSecretValue(ctx, db, name); err != nil {
			return false, err
		}
	}
	return true, nil
}

// ReencryptQRShortlinks migrates encrypted QR destinations to currentKey in
// bounded batches, resuming safely when a row already uses the current key.
func ReencryptQRShortlinks(ctx context.Context, db *database.DB, currentKey, previousKey string) (int, error) {
	if db == nil || currentKey == "" {
		return 0, fmt.Errorf("short-link database and current encryption key are required")
	}
	table := database.ServerTableName(db, "qr_shortlinks")
	var cursor string
	total := 0
	for {
		type item struct{ code, encrypted string }
		var batch []item
		err := db.Write(ctx, func(tx *sql.Tx) error {
			rows, err := database.QueryContext(ctx, tx, database.TimeoutBulk, fmt.Sprintf("SELECT code,target_encrypted FROM %s WHERE code>? ORDER BY code LIMIT 500", table), cursor)
			if err != nil {
				return err
			}
			for rows.Next() {
				var row item
				if err := rows.Scan(&row.code, &row.encrypted); err != nil {
					return errors.Join(err, rows.Close())
				}
				batch = append(batch, row)
			}
			if err := rows.Err(); err != nil {
				return errors.Join(err, rows.Close())
			}
			return rows.Close()
		})
		if err != nil {
			return total, fmt.Errorf("read encrypted QR short links: %w", err)
		}
		if len(batch) == 0 {
			return total, nil
		}
		err = db.Write(ctx, func(tx *sql.Tx) error {
			for _, row := range batch {
				cursor = row.code
				_, decErr := DecryptAESGCM(currentKey, row.encrypted)
				if decErr == nil {
					continue
				}
				if previousKey == "" {
					return fmt.Errorf("security.decrypt_failed table=qr_shortlinks row=%s field=target_encrypted", row.code)
				}
				plain, decErr := DecryptAESGCM(previousKey, row.encrypted)
				if decErr != nil {
					return fmt.Errorf("security.decrypt_failed table=qr_shortlinks row=%s field=target_encrypted", row.code)
				}
				replacement, err := EncryptAESGCM(currentKey, plain)
				if err != nil {
					return fmt.Errorf("encrypt QR short link %s: %w", row.code, err)
				}
				result, err := database.ExecContext(ctx, tx, fmt.Sprintf("UPDATE %s SET target_encrypted=? WHERE code=? AND target_encrypted=?", table), replacement, row.code, row.encrypted)
				if err != nil {
					return fmt.Errorf("update QR short link %s: %w", row.code, err)
				}
				count, err := result.RowsAffected()
				if err != nil {
					return err
				}
				if count != 1 {
					return fmt.Errorf("QR short link %s changed during encryption rotation", row.code)
				}
				total++
			}
			return nil
		})
		if err != nil {
			return total, err
		}
		if len(batch) < 500 {
			return total, nil
		}
	}
}
