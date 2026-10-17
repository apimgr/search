package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// Query timeouts per AI.md PART 10 "Query Timeouts". Every query must run
// under one of these budgets; callers pick the class that matches the work.
const (
	// TimeoutSimpleQuery bounds a simple SELECT.
	TimeoutSimpleQuery = 5 * time.Second
	// TimeoutComplexQuery bounds a SELECT with JOINs.
	TimeoutComplexQuery = 15 * time.Second
	// TimeoutWrite bounds INSERT/UPDATE/DELETE.
	TimeoutWrite = 10 * time.Second
	// TimeoutBulk bounds bulk operations over large data sets.
	TimeoutBulk = 60 * time.Second
	// TimeoutMigration bounds schema changes.
	TimeoutMigration = 5 * time.Minute
	// TimeoutReport bounds aggregation/report queries.
	TimeoutReport = 2 * time.Minute
	// TimeoutTransaction bounds a whole WithTransaction unit of work.
	TimeoutTransaction = 30 * time.Second
)

// WithTransaction runs fn inside a transaction bounded by
// TimeoutTransaction, rolling back on any error and on panic.
// AI.md PART 10 "Query Timeouts" → "For transactions".
func WithTransaction(ctx context.Context, db *DB, fn func(*sql.Tx) error) error {
	ctx, cancel := context.WithTimeout(ctx, TimeoutTransaction)
	defer cancel()

	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}

	// A panic inside fn must not leave the transaction open.
	committed := false
	defer func() {
		if !committed {
			if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
				slog.Warn("failed to roll back database transaction", "error", err)
			}
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

// HandleQueryError maps a database error onto the stable error codes from
// AI.md PART 10 "Handling Timeouts". The underlying error is wrapped only
// in the default branch so no driver detail leaks into client-facing text
// for the known cases.
func HandleQueryError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return errors.New("TIMEOUT: Request timed out")
	case errors.Is(err, sql.ErrNoRows):
		return errors.New("NOT_FOUND: Resource not found")
	case errors.Is(err, context.Canceled):
		return errors.New("CANCELED: Request was canceled")
	default:
		return fmt.Errorf("SERVER_ERROR: Database error: %w", err)
	}
}

// WithSerializableRetry runs fn in a SERIALIZABLE transaction, retrying on
// serialization failures with a linear backoff.
// AI.md PART 10 "Retry on Serialization Failure".
func WithSerializableRetry(ctx context.Context, db *DB, maxRetries int, fn func(*sql.Tx) error) error {
	if maxRetries < 1 {
		maxRetries = 1
	}

	for attempt := 0; attempt < maxRetries; attempt++ {
		tx, err := db.SQL().BeginTx(ctx, &sql.TxOptions{
			Isolation: sql.LevelSerializable,
		})
		if err != nil {
			return err
		}

		if err := fn(tx); err != nil {
			rollbackErr := tx.Rollback()
			err = errors.Join(err, rollbackErr)
			if IsSerializationError(err) && attempt < maxRetries-1 {
				time.Sleep(time.Duration(attempt*10) * time.Millisecond)
				continue
			}
			return err
		}

		if err := tx.Commit(); err != nil {
			if IsSerializationError(err) && attempt < maxRetries-1 {
				time.Sleep(time.Duration(attempt*10) * time.Millisecond)
				continue
			}
			return err
		}
		return nil
	}
	return errors.New("max retries exceeded")
}

// IsSerializationError reports whether err is a SQLite/libsql write
// conflict that is safe to retry.
func IsSerializationError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "SQLITE_BUSY") ||
		strings.Contains(msg, "database is locked")
}

// IsColumnExistsError reports whether err is the "duplicate column" error
// raised by an idempotent ALTER TABLE ADD COLUMN that has already been
// applied. AI.md PART 10 "Remote Database Updates".
func IsColumnExistsError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "duplicate column") ||
		strings.Contains(msg, "already exists")
}
