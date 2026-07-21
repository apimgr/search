// Package security implements security-related features per AI.md PART 11.
package security

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"strconv"
)

// SecurityIDWindowSeconds is the rotation window for the {security_id} token:
// 48 hours, per AI.md PART 11.
const SecurityIDWindowSeconds = 172800

// SecurityIDLength is the number of hex characters kept from the HMAC digest.
const SecurityIDLength = 16

// GenerateSecurityID computes the rotating {security_id} token per AI.md PART 11:
// HMAC-SHA256(installationSecret, floor(now / SecurityIDWindowSeconds)), hex-encoded,
// truncated to SecurityIDLength characters. now is unix seconds, injected by the
// caller (never read from time.Now() here) so the algorithm is deterministically
// testable.
func GenerateSecurityID(installationSecret string, now int64) string {
	window := now / SecurityIDWindowSeconds
	return hashWindow(installationSecret, window)
}

// ValidateSecurityID reports whether the supplied id matches the current 48h
// window or the previous one. Per AI.md PART 11, the validation window spans
// both windows so a researcher who loads security.txt near a rotation boundary
// still succeeds. Ids from two or more windows ago are rejected.
func ValidateSecurityID(installationSecret, id string, now int64) bool {
	if id == "" || installationSecret == "" {
		return false
	}

	currentWindow := now / SecurityIDWindowSeconds
	current := hashWindow(installationSecret, currentWindow)
	previous := hashWindow(installationSecret, currentWindow-1)

	matchesCurrent := subtle.ConstantTimeCompare([]byte(id), []byte(current)) == 1
	matchesPrevious := subtle.ConstantTimeCompare([]byte(id), []byte(previous)) == 1

	return matchesCurrent || matchesPrevious
}

// hashWindow computes the truncated hex HMAC digest for a given window index.
func hashWindow(installationSecret string, window int64) string {
	mac := hmac.New(sha256.New, []byte(installationSecret))
	mac.Write([]byte(strconv.FormatInt(window, 10)))
	sum := mac.Sum(nil)

	digest := hex.EncodeToString(sum)
	if len(digest) < SecurityIDLength {
		return digest
	}
	return digest[:SecurityIDLength]
}
