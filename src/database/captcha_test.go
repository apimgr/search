package database

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestCaptchaChallengesPersistHashAndConsumeOnce(t *testing.T) {
	dm := newManagerTempDir(t)
	ctx := context.Background()
	if err := InitSchema(ctx, dm); err != nil {
		t.Fatalf("InitSchema() error = %v", err)
	}
	db := dm.ServerDB()
	id := "0123456789abcdef0123456789abcdef"
	answerHash := strings.Repeat("a", 64)
	if err := db.CreateCaptchaChallenge(ctx, id, answerHash, time.Now().UTC().Add(10*time.Minute)); err != nil {
		t.Fatalf("CreateCaptchaChallenge() error = %v", err)
	}
	table := ServerTableName(db, "captcha_challenges")
	var storedHash string
	if err := db.QueryRow(ctx, fmt.Sprintf("SELECT answer_hash FROM %s WHERE id = ?", table), id).Scan(&storedHash); err != nil {
		t.Fatalf("read stored CAPTCHA answer hash: %v", err)
	}
	if storedHash != answerHash {
		t.Fatalf("stored answer hash = %q, want %q", storedHash, answerHash)
	}
	gotHash, found, err := db.ConsumeCaptchaChallenge(ctx, id, time.Now().UTC())
	if err != nil || !found || gotHash != answerHash {
		t.Fatalf("ConsumeCaptchaChallenge() = %q,%t,%v, want hash,true,nil", gotHash, found, err)
	}
	if _, found, err := db.ConsumeCaptchaChallenge(ctx, id, time.Now().UTC()); err != nil || found {
		t.Fatalf("second ConsumeCaptchaChallenge() found=%t err=%v, want false,nil", found, err)
	}
}

func TestConsumeExpiredCaptchaChallengeDeletesIt(t *testing.T) {
	dm := newManagerTempDir(t)
	ctx := context.Background()
	if err := InitSchema(ctx, dm); err != nil {
		t.Fatalf("InitSchema() error = %v", err)
	}
	db := dm.ServerDB()
	id := "abcdef0123456789abcdef0123456789"
	if err := db.CreateCaptchaChallenge(ctx, id, strings.Repeat("b", 64), time.Now().UTC().Add(-time.Minute)); err != nil {
		t.Fatalf("CreateCaptchaChallenge() error = %v", err)
	}
	if _, found, err := db.ConsumeCaptchaChallenge(ctx, id, time.Now().UTC()); err != nil || found {
		t.Fatalf("ConsumeCaptchaChallenge() found=%t err=%v, want false,nil for expired record", found, err)
	}
	var count int
	table := ServerTableName(db, "captcha_challenges")
	if err := db.QueryRow(ctx, fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE id = ?", table), id).Scan(&count); err != nil {
		t.Fatalf("count expired challenge rows: %v", err)
	}
	if count != 0 {
		t.Fatalf("expired challenge rows = %d, want 0", count)
	}
}

func TestCleanupExpiredCaptchaChallenges(t *testing.T) {
	dm := newManagerTempDir(t)
	ctx := context.Background()
	if err := InitSchema(ctx, dm); err != nil {
		t.Fatalf("InitSchema() error = %v", err)
	}
	db := dm.ServerDB()
	now := time.Now().UTC()
	for index, expiry := range []time.Time{now.Add(-time.Minute), now.Add(time.Minute)} {
		id := fmt.Sprintf("%032x", index+1)
		if err := db.CreateCaptchaChallenge(ctx, id, strings.Repeat("c", 64), expiry); err != nil {
			t.Fatalf("CreateCaptchaChallenge(%s) error = %v", id, err)
		}
	}
	removed, err := db.CleanupExpiredCaptchaChallenges(ctx, now)
	if err != nil || removed != 1 {
		t.Fatalf("CleanupExpiredCaptchaChallenges() = %d,%v, want 1,nil", removed, err)
	}
}
