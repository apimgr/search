package security

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/apimgr/search/src/database"
)

func TestMigrateLegacyReportPlaintextEncryptsBeforeClearing(t *testing.T) {
	ctx := context.Background()
	const migrationKey = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="
	dm, err := database.NewDatabaseManager(&database.Config{Driver:"sqlite",DataDir:filepath.Join(t.TempDir(),"db"),MaxOpen:1,MaxIdle:1})
	if err != nil { t.Fatal(err) }
	defer dm.Close()
	db:=dm.ServerDB()
	_,err=db.Exec(ctx,`CREATE TABLE security_reports (tracking_id TEXT PRIMARY KEY, security_id_used TEXT, affected_component TEXT, affected_endpoint TEXT, severity TEXT, summary TEXT, encrypted_body BLOB, encryption_method TEXT, credit_preference TEXT, credit_name TEXT, disclosure_days INTEGER, cve_requested INTEGER, status TEXT, report_token_hash TEXT, last_status_access_date TEXT, created_at DATETIME DEFAULT CURRENT_TIMESTAMP, updated_at DATETIME DEFAULT CURRENT_TIMESTAMP, closed_at DATETIME)`)
	if err != nil { t.Fatal(err) }
	legacyCiphertext,err:=EncryptAESGCM(migrationKey,[]byte("legacy report body\nEmail: researcher@example.com"))
	if err!=nil{t.Fatal(err)}
	_,err=db.Exec(ctx,`INSERT INTO security_reports (tracking_id,security_id_used,affected_component,affected_endpoint,severity,summary,encrypted_body,encryption_method,credit_preference,report_token_hash) VALUES (?,?,?,?,?,?,?,?,?,?)`,"sec_0123456789abcdef","legacy-id-digest","search","/private/path","high","legacy summary",[]byte(legacyCiphertext),"aes-256-gcm","no","token-hash")
	if err != nil { t.Fatal(err) }
	if err:=database.InitSchema(ctx,dm);err!=nil{t.Fatal(err)}
	count,err:=MigrateLegacyReportPlaintext(ctx,db,migrationKey,"","","","")
	if err!=nil||count!=1{t.Fatalf("MigrateLegacyReportPlaintext() = %d, %v",count,err)}
	var encrypted []byte; var algorithm,summary,endpoint,legacyID string
	err=db.QueryRow(ctx,`SELECT body_enc,body_alg,summary,affected_endpoint,security_id_used FROM security_reports WHERE tracking_id=?`,"sec_0123456789abcdef").Scan(&encrypted,&algorithm,&summary,&endpoint,&legacyID)
	if err!=nil{t.Fatal(err)}
	plain,err:=DecryptAESGCM(migrationKey,string(encrypted));if err!=nil{t.Fatal(err)}
	if !strings.Contains(string(plain),"legacy report body")||!strings.Contains(string(plain),"legacy summary")||!strings.Contains(string(plain),"/private/path"){t.Fatalf("migrated body = %q",plain)}
	if algorithm!=EncryptionMethodAESGCM||summary!=""||endpoint!=""{t.Fatalf("algorithm=%q summary=%q endpoint=%q",algorithm,summary,endpoint)}
	if legacyID!="legacy-id-digest"{t.Fatalf("legacy id digest changed before policy decision: %q",legacyID)}
	var contactCipher []byte
	if err:=db.QueryRow(ctx,`SELECT contact_enc FROM security_reports WHERE tracking_id=?`,"sec_0123456789abcdef").Scan(&contactCipher);err!=nil{t.Fatal(err)}
	contactPlain,err:=DecryptAESGCM(migrationKey,string(contactCipher));if err!=nil{t.Fatal(err)}
	var contact struct{Email string `json:"email"`};if err:=json.Unmarshal(contactPlain,&contact);err!=nil{t.Fatal(err)}
	if contact.Email!="researcher@example.com"{t.Fatalf("migrated contact email = %q",contact.Email)}
	var window int64
	if err:=db.QueryRow(ctx,`SELECT id_window FROM security_reports WHERE tracking_id=?`,"sec_0123456789abcdef").Scan(&window);err!=nil{t.Fatal(err)}
	if window!=0{t.Fatalf("legacy id window = %d, want migration sentinel 0 pending policy",window)}
}

// TestGenerateTrackingID verifies the "sec_" prefix and correct hex length,
// and that repeated calls produce distinct ids.
func TestGenerateTrackingID(t *testing.T) {
	id, err := GenerateTrackingID()
	if err != nil {
		t.Fatalf("GenerateTrackingID() error = %v", err)
	}
	if !strings.HasPrefix(id, TrackingIDPrefix) {
		t.Errorf("GenerateTrackingID() = %q, want prefix %q", id, TrackingIDPrefix)
	}
	hexPart := strings.TrimPrefix(id, TrackingIDPrefix)
	if len(hexPart) != TrackingIDRandomHexChars {
		t.Errorf("GenerateTrackingID() hex part length = %d, want %d", len(hexPart), TrackingIDRandomHexChars)
	}

	second, err := GenerateTrackingID()
	if err != nil {
		t.Fatalf("GenerateTrackingID() second call error = %v", err)
	}
	if id == second {
		t.Error("GenerateTrackingID() should produce distinct ids across calls")
	}
}

// TestGenerateReportToken verifies the raw token hashes to the returned hash
// and that HashReportToken independently reproduces the same value.
func TestGenerateReportToken(t *testing.T) {
	rawToken, hash, err := GenerateReportToken()
	if err != nil {
		t.Fatalf("GenerateReportToken() error = %v", err)
	}
	if rawToken == "" || hash == "" {
		t.Fatal("GenerateReportToken() returned empty token or hash")
	}
	if rawToken == hash {
		t.Error("GenerateReportToken() raw token and hash must differ")
	}

	if got := HashReportToken(rawToken); got != hash {
		t.Errorf("HashReportToken(%q) = %q, want %q (from GenerateReportToken)", rawToken, got, hash)
	}
}

// TestGenerateReportToken_Distinct verifies repeated calls produce distinct
// tokens (no accidental reuse of randomness).
func TestGenerateReportToken_Distinct(t *testing.T) {
	firstToken, firstHash, err := GenerateReportToken()
	if err != nil {
		t.Fatalf("GenerateReportToken() error = %v", err)
	}
	secondToken, secondHash, err := GenerateReportToken()
	if err != nil {
		t.Fatalf("GenerateReportToken() error = %v", err)
	}
	if firstToken == secondToken {
		t.Error("GenerateReportToken() should produce distinct raw tokens")
	}
	if firstHash == secondHash {
		t.Error("GenerateReportToken() should produce distinct hashes")
	}
}

// TestHashReportToken_Deterministic verifies the same input always hashes
// to the same output, and different inputs hash to different outputs.
func TestHashReportToken_Deterministic(t *testing.T) {
	a := HashReportToken("token-a")
	b := HashReportToken("token-a")
	c := HashReportToken("token-b")

	if a != b {
		t.Error("HashReportToken() should be deterministic for the same input")
	}
	if a == c {
		t.Error("HashReportToken() should differ for different inputs")
	}
	if len(a) != 64 {
		t.Errorf("HashReportToken() length = %d, want 64 (SHA-256 hex)", len(a))
	}
}

// TestReportStatus_TokenExpired covers the nil-ClosedAt boundary (never
// expires), the not-yet-expired case, and the expired-past-window case.
func TestReportStatus_TokenExpired(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name     string
		closedAt *time.Time
		want     bool
	}{
		{"nil ClosedAt never expires", nil, false},
		{"closed just now, well within 30 days", timePtr(now.Add(-1 * time.Hour)), false},
		{"closed exactly 30 days ago, not yet past boundary", timePtr(now.Add(-ReportTokenExpiryAfterClose)), false},
		{"closed 31 days ago, expired", timePtr(now.Add(-31 * 24 * time.Hour)), true},
		{"closed in the future never expired yet", timePtr(now.Add(1 * time.Hour)), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rs := &ReportStatus{ClosedAt: tt.closedAt}
			got := rs.TokenExpired(now)
			if got != tt.want {
				t.Errorf("TokenExpired() = %v, want %v", got, tt.want)
			}
		})
	}
}

// timePtr returns a pointer to the given time value, for table-driven test fixtures.
func timePtr(t time.Time) *time.Time {
	return &t
}

// TestBoolToInt covers both boolean values of the internal 0/1 conversion
// used for the cve_requested column.
func TestBoolToInt(t *testing.T) {
	if got := boolToInt(true); got != 1 {
		t.Errorf("boolToInt(true) = %d, want 1", got)
	}
	if got := boolToInt(false); got != 0 {
		t.Errorf("boolToInt(false) = %d, want 0", got)
	}
}
