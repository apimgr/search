package service

import (
	"crypto/ed25519"
	"crypto/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestOnionAddressFromPublicIsValidV3 verifies the derived address is a
// 56-character base32 string using only the characters a v3 onion may contain.
func TestOnionAddressFromPublicIsValidV3(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}

	addr := onionAddressFromPublic(pub)
	if len(addr) != 56 {
		t.Errorf("address length = %d, want 56", len(addr))
	}
	for _, c := range addr {
		if !strings.ContainsRune(vanityCharset, c) {
			t.Errorf("address contains invalid character %q: %s", c, addr)
		}
	}
}

// TestPublicFromExpandedMatchesSeed verifies that recovering the public key from
// Tor's expanded secret key yields the same key ed25519 derives from the seed.
func TestPublicFromExpandedMatchesSeed(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}

	recovered, err := publicFromExpanded(expandedFromSeed(priv.Seed()))
	if err != nil {
		t.Fatalf("publicFromExpanded() error = %v", err)
	}
	if string(recovered) != string(pub) {
		t.Error("recovered public key does not match the generated key")
	}
}

// TestPublicFromExpandedRejectsShortKey verifies the length guard.
func TestPublicFromExpandedRejectsShortKey(t *testing.T) {
	if _, err := publicFromExpanded(make([]byte, 32)); err == nil {
		t.Error("publicFromExpanded() should reject a key that is not 64 bytes")
	}
}

// TestParseTorKeyMaterialFormats verifies every key encoding an operator can
// supply to `tor import-keys` resolves to the same address.
func TestParseTorKeyMaterialFormats(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}
	want := onionAddressFromPublic(pub)
	expanded := expandedFromSeed(priv.Seed())

	cases := map[string][]byte{
		"seed":            priv.Seed(),
		"ed25519 private": priv,
		"expanded":        expanded,
		"tor secret file": append([]byte(torSecretKeyHeader), expanded...),
	}

	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			_, gotPub, gotAddr, err := parseTorKeyMaterial(data)
			if err != nil {
				t.Fatalf("parseTorKeyMaterial() error = %v", err)
			}
			if gotAddr != want {
				t.Errorf("address = %q, want %q", gotAddr, want)
			}
			if string(gotPub) != string(pub) {
				t.Error("public key does not match")
			}
		})
	}
}

// TestParseTorKeyMaterialRejectsGarbage verifies unknown lengths are rejected
// rather than silently producing an address.
func TestParseTorKeyMaterialRejectsGarbage(t *testing.T) {
	if _, _, _, err := parseTorKeyMaterial([]byte("not a key")); err == nil {
		t.Error("parseTorKeyMaterial() should reject an unrecognized format")
	}
}

// TestWriteTorKeyDirFormats verifies the three files are written in Tor's own
// on-disk formats with 0600 permissions.
func TestWriteTorKeyDirFormats(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}
	expanded := expandedFromSeed(priv.Seed())
	addr := onionAddressFromPublic(pub)

	dir := filepath.Join(t.TempDir(), addr)
	if err := writeTorKeyDir(dir, expanded, pub, addr); err != nil {
		t.Fatalf("writeTorKeyDir() error = %v", err)
	}

	for _, name := range torKeyFiles {
		info, err := os.Stat(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("missing %s: %v", name, err)
		}
		if info.Mode().Perm() != 0600 {
			t.Errorf("%s permissions = %o, want 0600", name, info.Mode().Perm())
		}
	}

	secret, err := os.ReadFile(filepath.Join(dir, "hs_ed25519_secret_key"))
	if err != nil {
		t.Fatalf("read secret key: %v", err)
	}
	if len(secret) != len(torSecretKeyHeader)+64 {
		t.Errorf("secret key length = %d, want %d", len(secret), len(torSecretKeyHeader)+64)
	}
	if !strings.HasPrefix(string(secret), torSecretKeyHeader) {
		t.Error("secret key is missing the ed25519v1-secret header")
	}

	host, err := os.ReadFile(filepath.Join(dir, "hostname"))
	if err != nil {
		t.Fatalf("read hostname: %v", err)
	}
	if strings.TrimSpace(string(host)) != addr+".onion" {
		t.Errorf("hostname = %q, want %q", strings.TrimSpace(string(host)), addr+".onion")
	}
}
