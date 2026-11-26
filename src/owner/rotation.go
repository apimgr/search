package owner

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/apimgr/search/src/database"
)

// ReencryptTokenCopies eagerly migrates every stored owner and linked-token
// ciphertext. It is restartable: rows already decryptable with newKey are
// skipped, while any row readable only with oldKey is rewritten in batches.
func ReencryptTokenCopies(ctx context.Context, db *database.DB, prefix, oldKey, newKey string) (int, error) {
	if db == nil {
		return 0, fmt.Errorf("owner token database is unavailable")
	}
	newManager, err := NewManagerWithDB(db, prefix, newKey, 1)
	if err != nil {
		return 0, fmt.Errorf("initialize current owner-token key: %w", err)
	}
	var oldManager *Manager
	if oldKey != "" {
		oldManager, err = NewManagerWithDB(db, prefix, oldKey, 1)
		if err != nil {
			return 0, fmt.Errorf("initialize previous owner-token key: %w", err)
		}
	}
	table := prefix + "api_tokens"
	var cursor string
	total := 0
	for {
		var rowsRead []struct {
			id        string
			encrypted string
		}
		err := db.Write(ctx, func(tx *sql.Tx) error {
			rows, err := tx.QueryContext(ctx, fmt.Sprintf("SELECT id,token_enc FROM %s WHERE id>? AND token_enc<>'' ORDER BY id LIMIT 500", table), cursor)
			if err != nil {
				return err
			}
			for rows.Next() {
				var row struct {
					id        string
					encrypted string
				}
				if err := rows.Scan(&row.id, &row.encrypted); err != nil {
					return errors.Join(err, rows.Close())
				}
				rowsRead = append(rowsRead, row)
			}
			if err := rows.Err(); err != nil {
				return errors.Join(err, rows.Close())
			}
			return rows.Close()
		})
		if err != nil {
			return total, fmt.Errorf("read owner token ciphertexts: %w", err)
		}
		if len(rowsRead) == 0 {
			return total, nil
		}
		err = db.Write(ctx, func(tx *sql.Tx) error {
			for _, row := range rowsRead {
				cursor = row.id
				_, currentErr := newManager.decrypt(row.encrypted)
				if currentErr == nil {
					continue
				}
				if oldManager == nil {
					return fmt.Errorf("security.decrypt_failed table=api_tokens row=%s field=token_enc", row.id)
				}
				plain, oldErr := oldManager.decrypt(row.encrypted)
				if oldErr != nil {
					return fmt.Errorf("security.decrypt_failed table=api_tokens row=%s field=token_enc", row.id)
				}
				encoded, err := newManager.encrypt(plain)
				if err != nil {
					return fmt.Errorf("encrypt owner token row %s: %w", row.id, err)
				}
				result, err := tx.ExecContext(ctx, fmt.Sprintf("UPDATE %s SET token_enc=? WHERE id=? AND token_enc=?", table), encoded, row.id, row.encrypted)
				if err != nil {
					return fmt.Errorf("update owner token row %s: %w", row.id, err)
				}
				count, err := result.RowsAffected()
				if err != nil {
					return fmt.Errorf("read owner token update result %s: %w", row.id, err)
				}
				if count != 1 {
					return fmt.Errorf("owner token row %s changed during encryption rotation", row.id)
				}
				total++
			}
			return nil
		})
		if err != nil {
			return total, err
		}
		if len(rowsRead) < 500 {
			return total, nil
		}
	}
}
