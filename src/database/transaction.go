package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
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
	// TimeoutTransaction bounds a whole remote transaction; SQLite writes use TimeoutWrite.
	TimeoutTransaction = 30 * time.Second
)

// TimedRows keeps a query deadline alive until the result stream is closed.
type TimedRows struct {
	*sql.Rows
	cancel context.CancelFunc
}

// Next releases the context as soon as the stream reaches EOF.
func (rows *TimedRows) Next() bool {
	if rows.Rows.Next() {
		return true
	}
	rows.cancel()
	return false
}

// Close closes the result stream and releases its timeout context.
func (rows *TimedRows) Close() error {
	err := rows.Rows.Close()
	rows.cancel()
	return err
}

// TimedRow keeps a query deadline alive until the single row is scanned.
type TimedRow struct {
	row    *sql.Row
	cancel context.CancelFunc
}

type queryContextRunner interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

type queryRowContextRunner interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

type execContextRunner interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

// Scan reads the row and releases its timeout context.
func (row *TimedRow) Scan(dest ...any) error {
	defer row.cancel()
	return row.row.Scan(dest...)
}

// QueryContext executes a row query with a bounded context that remains active
// until the caller closes the returned result stream.
func QueryContext(ctx context.Context, db queryContextRunner, timeout time.Duration, query string, args ...any) (*TimedRows, error) {
	queryCtx, cancel := context.WithTimeout(ctx, timeout)
	rows, err := db.QueryContext(queryCtx, query, args...)
	if err != nil {
		cancel()
		return nil, err
	}
	return &TimedRows{Rows: rows, cancel: cancel}, nil
}

// QueryRowContext executes a single-row query with a bounded context. The
// context is released when Scan finishes, including when Scan returns an error.
func QueryRowContext(ctx context.Context, db queryRowContextRunner, timeout time.Duration, query string, args ...any) *TimedRow {
	queryCtx, cancel := context.WithTimeout(ctx, timeout)
	return &TimedRow{row: db.QueryRowContext(queryCtx, query, args...), cancel: cancel}
}

// ExecContext executes a write with a bounded context.
func ExecContext(ctx context.Context, db execContextRunner, query string, args ...any) (sql.Result, error) {
	writeCtx, cancel := context.WithTimeout(ctx, TimeoutWrite)
	defer cancel()
	return db.ExecContext(writeCtx, query, args...)
}

// WithTransaction uses DB.Write's transaction budget: 10 seconds for SQLite
// and 30 seconds for remote drivers, rolling back on errors and panics.
func WithTransaction(ctx context.Context, db *DB, fn func(*sql.Tx) error) error {
	return db.Write(ctx, fn)
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
	if db.Driver() == "sqlite" {
		return db.Write(ctx, fn)
	}
	ctx, cancel := context.WithTimeout(ctx, TimeoutTransaction)
	defer cancel()
	if maxRetries < 1 {
		maxRetries = 1
	}

	for attempt := 0; attempt < maxRetries; attempt++ {
		tx, err := db.beginTx(ctx, &sql.TxOptions{
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
