package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// RepairSQLite creates a clean SQLite copy with VACUUM INTO, then falls back
// to REINDEX when SQLite cannot produce the copy. Raw SQLite handles remain
// inside the database store package, including recovery operations.
func RepairSQLite(ctx context.Context, path, destination string) (vacuumed bool, retErr error) {
	if filepath.Dir(filepath.Clean(path)) != filepath.Dir(filepath.Clean(destination)) {
		return false, fmt.Errorf("repair destination must be in the database directory")
	}
	db, err := sql.Open("sqlite", sqliteDSNWithPragmas(path))
	if err != nil {
		return false, fmt.Errorf("open sqlite database for repair: %w", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			retErr = errors.Join(retErr, fmt.Errorf("close sqlite database after repair: %w", err))
		}
	}()

	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	if _, err := db.ExecContext(ctx, "VACUUM INTO ?", destination); err != nil {
		if _, reindexErr := db.ExecContext(ctx, "REINDEX"); reindexErr != nil {
			return false, fmt.Errorf("vacuum repair failed: %w", errors.Join(err, reindexErr))
		}
		if removeErr := os.Remove(destination); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			return false, fmt.Errorf("remove incomplete vacuum destination: %w", removeErr)
		}
		return false, nil
	}
	return true, nil
}
