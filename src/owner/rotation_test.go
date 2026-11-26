package owner

import (
	"bytes"
	"context"
	"encoding/base64"
	"path/filepath"
	"testing"

	"github.com/apimgr/search/src/database"
)

func TestReencryptTokenCopiesIsRestartable(t *testing.T) {
	ctx := context.Background()
	dm, err := database.NewDatabaseManager(&database.Config{Driver: "sqlite", DataDir: filepath.Join(t.TempDir(), "db"), MaxOpen: 2, MaxIdle: 2})
	if err != nil { t.Fatal(err) }
	defer dm.Close()
	if err := database.InitSchema(ctx, dm); err != nil { t.Fatal(err) }
	oldKey := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{1}, 32))
	newKey := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{2}, 32))
	manager, err := NewManagerWithDB(dm.ServerDB(), "", oldKey, 5)
	if err != nil { t.Fatal(err) }
	identity, token, err := manager.Issue(ctx)
	if err != nil { t.Fatal(err) }
	count, err := ReencryptTokenCopies(ctx, dm.ServerDB(), "", oldKey, newKey)
	if err != nil || count != 1 { t.Fatalf("first migration = %d, %v", count, err) }
	rotated, err := NewManagerWithDB(dm.ServerDB(), "", newKey, 5)
	if err != nil { t.Fatal(err) }
	got, err := rotated.Reveal(ctx, identity, identity.TokenID)
	if err != nil || got != token { t.Fatalf("Reveal() = %q, %v; want original token", got, err) }
	count, err = ReencryptTokenCopies(ctx, dm.ServerDB(), "", oldKey, newKey)
	if err != nil || count != 0 { t.Fatalf("resume migration = %d, %v; want no changed rows", count, err) }
}
