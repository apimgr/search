package security

import (
	"context"
	"encoding/base64"
	"bytes"
	"path/filepath"
	"testing"
	"time"

	"github.com/apimgr/search/src/database"
)

func TestEncryptionKeyRotationRecoveryAcrossConfigWrite(t *testing.T) {
	ctx := context.Background()
	dm, err := database.NewDatabaseManager(&database.Config{Driver: "sqlite", DataDir: filepath.Join(t.TempDir(), "db"), MaxOpen: 1, MaxIdle: 1})
	if err != nil { t.Fatal(err) }
	defer dm.Close()
	if err := database.InitSchema(ctx, dm); err != nil { t.Fatal(err) }
	oldKey := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x31}, 32))
	newKey := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x42}, 32))
	until := time.Now().UTC().Add(30 * 24 * time.Hour).Truncate(time.Second)
	if err := BeginEncryptionKeyRotation(ctx, dm.ServerDB(), oldKey, newKey, until, "planned maintenance"); err != nil { t.Fatal(err) }
	stored, err := database.GetSecret(ctx, dm.ServerDB(), database.SecretEncryptionKeyPrevious)
	if err != nil { t.Fatal(err) }
	if stored.Value == oldKey { t.Fatal("previous key was stored in plaintext") }
	opened, err := DecryptAESGCM(newKey, stored.Value)
	if err != nil || string(opened) != oldKey { t.Fatalf("wrapped previous key = %q, %v", opened, err) }

	active, previous, gotUntil, reason, pending, err := ResumeEncryptionKeyRotation(ctx, dm.ServerDB(), oldKey)
	if err != nil || !pending || active != newKey || previous != oldKey || !gotUntil.Equal(until) || reason != "planned maintenance" {
		t.Fatalf("resume before config persistence = active %q previous %q until %s reason %q pending %v err %v", active, previous, gotUntil, reason, pending, err)
	}
	active, previous, gotUntil, reason, pending, err = ResumeEncryptionKeyRotation(ctx, dm.ServerDB(), newKey)
	if err != nil || !pending || active != newKey || previous != oldKey || !gotUntil.Equal(until) || reason != "planned maintenance" {
		t.Fatalf("resume after config persistence = active %q previous %q until %s reason %q pending %v err %v", active, previous, gotUntil, reason, pending, err)
	}
	if err := CompleteEncryptionKeyRotation(ctx, dm.ServerDB()); err != nil { t.Fatal(err) }
	if _, err := database.GetSecret(ctx, dm.ServerDB(), database.SecretEncryptionKeyRotationPending); err == nil {
		t.Fatal("completed rotation retained its temporary recovery record")
	}
}

func TestReencryptAESReportDataAndQRShortlinks(t *testing.T) {
	ctx := context.Background()
	dm, err := database.NewDatabaseManager(&database.Config{Driver: "sqlite", DataDir: filepath.Join(t.TempDir(), "db"), MaxOpen: 1, MaxIdle: 1})
	if err != nil { t.Fatal(err) }
	defer dm.Close()
	if err := database.InitSchema(ctx, dm); err != nil { t.Fatal(err) }
	oldKey := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x53}, 32))
	newKey := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x64}, 32))
	bodyCipher, err := EncryptAESGCM(oldKey, []byte("report body"))
	if err != nil { t.Fatal(err) }
	contactCipher, err := EncryptAESGCM(oldKey, []byte(`{"email":"researcher@example.com"}`))
	if err != nil { t.Fatal(err) }
	reportTable := database.ServerTableName(dm.ServerDB(), "security_reports")
	if _, err := dm.ServerDB().Exec(ctx, "INSERT INTO "+reportTable+" (tracking_id,id_window,severity,component,status,body_enc,body_alg,contact_enc,credit_pref,token_hash) VALUES (?,?,?,?,?,?,?,?,?,?)", "sec_0123456789abcdef", 1, "high", "search", "received", []byte(bodyCipher), EncryptionMethodAESGCM, []byte(contactCipher), "no", "token-hash"); err != nil { t.Fatal(err) }
	shortlinkTable := database.ServerTableName(dm.ServerDB(), "qr_shortlinks")
	shortCipher, err := EncryptAESGCM(oldKey, []byte("https://example.com/private"))
	if err != nil { t.Fatal(err) }
	if _, err := dm.ServerDB().Exec(ctx, "INSERT INTO "+shortlinkTable+" (code,target_encrypted) VALUES (?,?)", "short-code", shortCipher); err != nil { t.Fatal(err) }
	if count, err := ReencryptAESReportData(ctx, dm.ServerDB(), newKey, oldKey); err != nil || count != 1 { t.Fatalf("report re-encryption = %d, %v", count, err) }
	if count, err := ReencryptQRShortlinks(ctx, dm.ServerDB(), newKey, oldKey); err != nil || count != 1 { t.Fatalf("shortlink re-encryption = %d, %v", count, err) }
	var gotBody, gotContact []byte
	if err := dm.ServerDB().QueryRow(ctx, "SELECT body_enc,contact_enc FROM "+reportTable+" WHERE tracking_id=?", "sec_0123456789abcdef").Scan(&gotBody,&gotContact); err != nil { t.Fatal(err) }
	plainBody, err := DecryptAESGCM(newKey, string(gotBody)); if err != nil || string(plainBody) != "report body" { t.Fatalf("rotated report body = %q, %v", plainBody, err) }
	plainContact, err := DecryptAESGCM(newKey, string(gotContact)); if err != nil || string(plainContact) != `{"email":"researcher@example.com"}` { t.Fatalf("rotated contact = %q, %v", plainContact, err) }
	var gotShort string
	if err := dm.ServerDB().QueryRow(ctx, "SELECT target_encrypted FROM "+shortlinkTable+" WHERE code=?", "short-code").Scan(&gotShort); err != nil { t.Fatal(err) }
	plainShort, err := DecryptAESGCM(newKey, gotShort); if err != nil || string(plainShort) != "https://example.com/private" { t.Fatalf("rotated shortlink = %q, %v", plainShort, err) }
}
