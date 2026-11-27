package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// CreateCaptchaChallenge stores only the answer hash and its expiration.
func (db *DB) CreateCaptchaChallenge(ctx context.Context, id, answerHash string, expiresAt time.Time) error {
	if id == "" || len(id) != 32 || answerHash == "" || len(answerHash) != 64 {
		return fmt.Errorf("invalid CAPTCHA challenge fields")
	}
	table := ServerTableName(db, "captcha_challenges")
	_, err := db.Exec(ctx, fmt.Sprintf(
		"INSERT INTO %s (id, answer_hash, expires_at) VALUES (?, ?, ?)", table),
		id, answerHash, expiresAt.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("store CAPTCHA challenge: %w", err)
	}
	return nil
}

// ConsumeCaptchaChallenge atomically returns an unexpired hash and deletes the
// challenge regardless of answer correctness, making every attempt one-use.
func (db *DB) ConsumeCaptchaChallenge(ctx context.Context, id string, now time.Time) (string, bool, error) {
	if id == "" || len(id) != 32 {
		return "", false, nil
	}
	table := ServerTableName(db, "captcha_challenges")
	var answerHash string
	found := false
	ctx, cancel := context.WithTimeout(ctx, TimeoutTransaction)
	defer cancel()
	err := db.WriteWithTimeout(ctx, TimeoutTransaction, func(tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, fmt.Sprintf(
			"SELECT answer_hash FROM %s WHERE id = ? AND expires_at > ?", table),
			id, now.UTC().Format(time.RFC3339Nano)).Scan(&answerHash)
		if err == nil {
			found = true
		} else if err != sql.ErrNoRows {
			return err
		}
		if _, err := tx.ExecContext(ctx, fmt.Sprintf("DELETE FROM %s WHERE id = ?", table), id); err != nil {
			return fmt.Errorf("delete consumed CAPTCHA challenge: %w", err)
		}
		return nil
	})
	if err != nil {
		return "", false, fmt.Errorf("consume CAPTCHA challenge: %w", err)
	}
	return answerHash, found, nil
}

// CleanupExpiredCaptchaChallenges removes expired one-use challenges.
func (db *DB) CleanupExpiredCaptchaChallenges(ctx context.Context, now time.Time) (int64, error) {
	table := ServerTableName(db, "captcha_challenges")
	result, err := db.ExecWithTimeout(ctx, TimeoutWrite,
		fmt.Sprintf("DELETE FROM %s WHERE expires_at <= ?", table),
		now.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return 0, fmt.Errorf("delete expired CAPTCHA challenges: %w", err)
	}
	removed, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("count expired CAPTCHA challenges: %w", err)
	}
	return removed, nil
}
