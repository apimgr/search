package security

import "testing"

// TestGenerateSecurityID verifies the known-answer HMAC derivation and that
// truncation to SecurityIDLength hex characters is applied.
func TestGenerateSecurityID(t *testing.T) {
	tests := []struct {
		name   string
		secret string
		now    int64
		want   string
	}{
		{
			name:   "known secret and time window 0",
			secret: "test-installation-secret",
			now:    0,
			want:   hashWindow("test-installation-secret", 0),
		},
		{
			name:   "known secret mid-window",
			secret: "test-installation-secret",
			now:    SecurityIDWindowSeconds + 100,
			want:   hashWindow("test-installation-secret", 1),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GenerateSecurityID(tt.secret, tt.now)
			if got != tt.want {
				t.Errorf("GenerateSecurityID() = %q, want %q", got, tt.want)
			}
			if len(got) != SecurityIDLength {
				t.Errorf("GenerateSecurityID() length = %d, want %d", len(got), SecurityIDLength)
			}
		})
	}
}

// TestGenerateSecurityID_RotationBoundary verifies the id changes once the
// unix-seconds window advances past SecurityIDWindowSeconds.
func TestGenerateSecurityID_RotationBoundary(t *testing.T) {
	secret := "rotation-secret"

	beforeRotation := GenerateSecurityID(secret, SecurityIDWindowSeconds-1)
	afterRotation := GenerateSecurityID(secret, SecurityIDWindowSeconds)

	if beforeRotation == afterRotation {
		t.Error("GenerateSecurityID() should differ across a window boundary")
	}
}

// TestValidateSecurityID covers the current-window match, previous-window
// grace period, and rejection of ids from two or more windows ago.
func TestValidateSecurityID(t *testing.T) {
	secret := "validate-secret"

	currentWindowTime := int64(5 * SecurityIDWindowSeconds)
	currentID := GenerateSecurityID(secret, currentWindowTime)
	previousID := GenerateSecurityID(secret, currentWindowTime-SecurityIDWindowSeconds)
	expiredID := GenerateSecurityID(secret, currentWindowTime-2*SecurityIDWindowSeconds)

	tests := []struct {
		name string
		id   string
		want bool
	}{
		{"current window id validates", currentID, true},
		{"previous window id still validates (grace period)", previousID, true},
		{"expired id (two windows ago) rejected", expiredID, false},
		{"empty id rejected", "", false},
		{"garbage id rejected", "not-a-real-id-000", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ValidateSecurityID(secret, tt.id, currentWindowTime)
			if got != tt.want {
				t.Errorf("ValidateSecurityID(%q) = %v, want %v", tt.id, got, tt.want)
			}
		})
	}
}

// TestValidateSecurityID_EmptySecret ensures an empty installation secret
// never validates, even against an id computed with the empty secret — the
// server must never treat an unconfigured secret as a valid credential.
func TestValidateSecurityID_EmptySecret(t *testing.T) {
	now := int64(1000000)
	id := GenerateSecurityID("", now)

	if ValidateSecurityID("", id, now) {
		t.Error("ValidateSecurityID() should reject when installation secret is empty")
	}
}
