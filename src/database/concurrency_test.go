package database

import (
	"context"
	"database/sql"
	"sync"
	"testing"
	"time"
)

func TestSQLiteConcurrencyContract(t *testing.T) {
	manager, err := NewDatabaseManager(&Config{Driver: "sqlite", DataDir: t.TempDir(), MaxOpen: 10, MaxIdle: 5})
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	db := manager.ServerDB()
	if _, err := db.Exec(context.Background(), "CREATE TABLE concurrency_probe (id INTEGER PRIMARY KEY)"); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	errs := make(chan error, 40)
	var workers sync.WaitGroup
	for worker := 0; worker < 20; worker++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for ctx.Err() == nil {
				if err := db.Write(ctx, func(tx *sql.Tx) error {
					_, err := tx.ExecContext(ctx, "INSERT INTO concurrency_probe DEFAULT VALUES")
					return err
				}); err != nil && ctx.Err() == nil {
					errs <- err
					return
				}
			}
		}()
	}
	for worker := 0; worker < 20; worker++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for ctx.Err() == nil {
				var count int64
				if err := db.QueryRow(ctx, "SELECT COUNT(*) FROM concurrency_probe").Scan(&count); err != nil && ctx.Err() == nil {
					errs <- err
					return
				}
			}
		}()
	}
	<-ctx.Done()
	workers.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("concurrent SQLite operation failed: %v", err)
	}
	var count int64
	if err := db.QueryRow(context.Background(), "SELECT COUNT(*) FROM concurrency_probe").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count == 0 {
		t.Error("concurrency test inserted no rows")
	}
}
