package service

import (
	"crypto/ed25519"
	"crypto/sha3"
	"crypto/sha512"
	"encoding/base32"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"filippo.io/edwards25519"
)

// Tor's on-disk key file headers per rend-spec-v3. A hidden service directory
// holds hs_ed25519_secret_key (header + 64-byte expanded key),
// hs_ed25519_public_key (header + 32-byte public key) and hostname.
// Per AI.md PART 31.1: vanity candidates, imported key directories and the live
// site directory all use these identical formats, so a swap is a file move.
const (
	torSecretKeyHeader = "== ed25519v1-secret: type0 ==\x00\x00\x00"
	torPublicKeyHeader = "== ed25519v1-public: type0 ==\x00\x00\x00"
)

// torKeyFiles are the three files that make up a hidden service identity.
var torKeyFiles = []string{"hs_ed25519_secret_key", "hs_ed25519_public_key", "hostname"}

// expandedFromSeed derives Tor's 64-byte expanded ed25519 secret key from a
// 32-byte seed (SHA-512 of the seed with the standard ed25519 clamping).
func expandedFromSeed(seed []byte) []byte {
	h := sha512.Sum512(seed)
	expanded := make([]byte, 64)
	copy(expanded, h[:])
	expanded[0] &= 248
	expanded[31] &= 127
	expanded[31] |= 64
	return expanded
}

// publicFromExpanded recovers the ed25519 public key from Tor's expanded secret
// key by multiplying the clamped scalar by the base point.
func publicFromExpanded(expanded []byte) ([]byte, error) {
	if len(expanded) != 64 {
		return nil, fmt.Errorf("expanded secret key must be 64 bytes, got %d", len(expanded))
	}
	s, err := edwards25519.NewScalar().SetBytesWithClamping(expanded[:32])
	if err != nil {
		return nil, fmt.Errorf("invalid ed25519 scalar: %w", err)
	}
	return new(edwards25519.Point).ScalarBaseMult(s).Bytes(), nil
}

// onionAddressFromPublic derives the 56-character v3 onion address (without the
// .onion suffix) from an ed25519 public key per rend-spec-v3:
// base32(pubkey || checksum || version) where
// checksum = SHA3-256(".onion checksum" || pubkey || version)[:2].
func onionAddressFromPublic(pub []byte) string {
	version := byte(0x03)
	h := sha3.New256()
	h.Write([]byte(".onion checksum"))
	h.Write(pub)
	h.Write([]byte{version})
	checksum := h.Sum(nil)

	payload := make([]byte, 35)
	copy(payload[0:32], pub)
	copy(payload[32:34], checksum[:2])
	payload[34] = version

	return strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(payload))
}

// writeTorKeyDir writes the three hidden service files into dir in Tor's own
// on-disk formats, with 0700 on the directory and 0600 on each file.
func writeTorKeyDir(dir string, expanded, pub []byte, address string) error {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create key dir %s: %w", dir, err)
	}
	if err := os.Chmod(dir, 0700); err != nil {
		return fmt.Errorf("chmod key dir %s: %w", dir, err)
	}

	files := map[string][]byte{
		"hs_ed25519_secret_key": append([]byte(torSecretKeyHeader), expanded...),
		"hs_ed25519_public_key": append([]byte(torPublicKeyHeader), pub...),
		"hostname":              []byte(address + ".onion\n"),
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), content, 0600); err != nil {
			return fmt.Errorf("write %s: %w", name, err)
		}
	}
	return nil
}

// parseTorKeyMaterial accepts any of the key encodings an operator can supply
// and normalizes them to (expanded secret key, public key, address):
//   - Tor's own hs_ed25519_secret_key file (header + 64-byte expanded key)
//   - a raw 64-byte expanded key
//   - a 32-byte ed25519 seed
//   - a 64-byte ed25519.PrivateKey (seed || public key)
//
// Per AI.md PART 31.1 an imported key is treated exactly like a found vanity
// candidate, so it must resolve to the same three on-disk files.
func parseTorKeyMaterial(data []byte) (expanded, pub []byte, address string, err error) {
	switch {
	case len(data) > len(torSecretKeyHeader) && strings.HasPrefix(string(data), "== ed25519v1-secret:"):
		expanded = data[len(torSecretKeyHeader):]
		if len(expanded) < 64 {
			return nil, nil, "", fmt.Errorf("tor secret key file too short")
		}
		expanded = expanded[:64]
	case len(data) == 64:
		// Ambiguous length: an ed25519.PrivateKey is seed || public key, so it
		// is only valid when the trailing half matches the derived public key.
		seed := data[:32]
		if derived := ed25519.NewKeyFromSeed(seed).Public().(ed25519.PublicKey); string(derived) == string(data[32:]) {
			expanded = expandedFromSeed(seed)
			pub = derived
			return expanded, pub, onionAddressFromPublic(pub), nil
		}
		expanded = data
	case len(data) == 32:
		expanded = expandedFromSeed(data)
	default:
		return nil, nil, "", fmt.Errorf("unrecognized key format (%d bytes)", len(data))
	}

	pub, err = publicFromExpanded(expanded)
	if err != nil {
		return nil, nil, "", err
	}
	return expanded, pub, onionAddressFromPublic(pub), nil
}
