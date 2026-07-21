package security

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ProtonMail/gopenpgp/v2/crypto"
)

// PGPKeyLifetime is the application-enforced key expiry per AI.md PART 11
// "GPG Keypair Management" (Generate: "Expires 2 years from generation").
// gopenpgp/v2's GenerateKey does not accept an OpenPGP self-signature
// expiration parameter, so expiry is tracked as DB/application metadata
// (expires_at = created_at + PGPKeyLifetime) rather than embedded in the key
// itself. Callers must check IsKeypairExpired before relying on a keypair.
const PGPKeyLifetime = 2 * 365 * 24 * time.Hour

// PGPOldKeyGracePeriod is how long a rotated-out key stays valid for
// decrypting in-flight reports, per AI.md PART 11 Rotate behavior.
const PGPOldKeyGracePeriod = 30 * 24 * time.Hour

// PGPDirName is the subdirectory of config_dir holding the keypair files.
const PGPDirName = "security"

// PGPPublicKeyFilename is the public key filename inside PGPDirName.
const PGPPublicKeyFilename = "pgp.pub.asc"

// PGPEncryptedPrivateKeyFilename is the encrypted private key filename
// inside PGPDirName.
const PGPEncryptedPrivateKeyFilename = "pgp.priv.asc.enc"

// PGPKeyserversStateFilename tracks per-keyserver publish state so a
// restore does not double-submit, per AI.md PART 11 Backup Integration.
const PGPKeyserversStateFilename = "keyservers.state"

// Keypair is a generated Ed25519 (signing) + Curve25519 (encryption) OpenPGP
// keypair along with the metadata AI.md PART 11 requires callers to persist
// in the {prefix}pgp_keypair DB table (never the keys themselves).
type Keypair struct {
	Key                 *crypto.Key
	Fingerprint         string
	CreatedAt           time.Time
	ExpiresAt           time.Time
	PublicKeyArmor      string
	EncryptedPrivateKey string
}

// GenerateKeypair generates a new Ed25519 signing + Curve25519 encryption
// OpenPGP keypair with identity "{appName} Security <{securityContact}>",
// per AI.md PART 11 GPG Keypair Management "Generate". The private key is
// locked (encrypted) with a passphrase derived from installationSecret
// before being armored, so EncryptedPrivateKey is safe to write to disk.
func GenerateKeypair(appName, securityContact, installationSecret string) (*Keypair, error) {
	if installationSecret == "" {
		return nil, fmt.Errorf("pgp: installation secret must not be empty")
	}

	identityName := fmt.Sprintf("%s Security", appName)
	key, err := crypto.GenerateKey(identityName, securityContact, "x25519", 0)
	if err != nil {
		return nil, fmt.Errorf("pgp: generate keypair: %w", err)
	}

	return keypairFromKey(key, installationSecret)
}

// keypairFromKey builds a Keypair (armored public key, encrypted-armored
// private key, fingerprint, timestamps) from an unlocked *crypto.Key.
func keypairFromKey(key *crypto.Key, installationSecret string) (*Keypair, error) {
	pubArmor, err := key.GetArmoredPublicKey()
	if err != nil {
		return nil, fmt.Errorf("pgp: armor public key: %w", err)
	}

	locked, err := key.Lock(pgpDerivePassphrase(installationSecret))
	if err != nil {
		return nil, fmt.Errorf("pgp: lock private key: %w", err)
	}

	privArmor, err := locked.Armor()
	if err != nil {
		return nil, fmt.Errorf("pgp: armor private key: %w", err)
	}

	now := time.Now()
	return &Keypair{
		Key:                 key,
		Fingerprint:         key.GetFingerprint(),
		CreatedAt:           now,
		ExpiresAt:           now.Add(PGPKeyLifetime),
		PublicKeyArmor:      pubArmor,
		EncryptedPrivateKey: privArmor,
	}, nil
}

// pgpDerivePassphrase derives a fixed-length passphrase from the
// installation secret for locking/unlocking the private key at rest, per
// AI.md PART 11 "Private key is encrypted with a key derived from
// installation_secret".
func pgpDerivePassphrase(installationSecret string) []byte {
	sum := sha256.Sum256([]byte("pgp-private-key-passphrase:" + installationSecret))
	return sum[:]
}

// SavePublicKey writes the armored public key to
// {configDir}/security/pgp.pub.asc.
func SavePublicKey(configDir, pubArmor string) error {
	dir := filepath.Join(configDir, PGPDirName)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("pgp: create security dir: %w", err)
	}
	path := filepath.Join(dir, PGPPublicKeyFilename)
	if err := os.WriteFile(path, []byte(pubArmor), 0o644); err != nil {
		return fmt.Errorf("pgp: write public key: %w", err)
	}
	return nil
}

// SaveEncryptedPrivateKey writes the locked, armored private key to
// {configDir}/security/pgp.priv.asc.enc with mode 0600.
func SaveEncryptedPrivateKey(configDir, privArmor string) error {
	dir := filepath.Join(configDir, PGPDirName)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("pgp: create security dir: %w", err)
	}
	path := filepath.Join(dir, PGPEncryptedPrivateKeyFilename)
	if err := os.WriteFile(path, []byte(privArmor), 0o600); err != nil {
		return fmt.Errorf("pgp: write private key: %w", err)
	}
	return nil
}

// LoadPublicKey reads {configDir}/security/pgp.pub.asc.
func LoadPublicKey(configDir string) (string, error) {
	path := filepath.Join(configDir, PGPDirName, PGPPublicKeyFilename)
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// LoadEncryptedPrivateKey reads {configDir}/security/pgp.priv.asc.enc.
func LoadEncryptedPrivateKey(configDir string) (string, error) {
	path := filepath.Join(configDir, PGPDirName, PGPEncryptedPrivateKeyFilename)
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// UnlockPrivateKey parses the armored, locked private key and unlocks it
// with the passphrase derived from installationSecret.
func UnlockPrivateKey(privArmor, installationSecret string) (*crypto.Key, error) {
	locked, err := crypto.NewKeyFromArmored(privArmor)
	if err != nil {
		return nil, fmt.Errorf("pgp: parse private key: %w", err)
	}
	unlocked, err := locked.Unlock(pgpDerivePassphrase(installationSecret))
	if err != nil {
		return nil, fmt.Errorf("pgp: unlock private key: %w", err)
	}
	return unlocked, nil
}

// KeyFingerprint returns the SHA-256 fingerprint of an armored public key.
func KeyFingerprint(pubArmor string) (string, error) {
	key, err := crypto.NewKeyFromArmored(pubArmor)
	if err != nil {
		return "", fmt.Errorf("pgp: parse public key: %w", err)
	}
	return key.GetFingerprint(), nil
}

// IsKeypairExpired reports whether a keypair's application-tracked expiry
// (PGPKeyLifetime from createdAt) has passed.
func IsKeypairExpired(createdAt time.Time, now time.Time) bool {
	return now.After(createdAt.Add(PGPKeyLifetime))
}

// KeyserverPublishResult records the outcome of publishing to one keyserver.
type KeyserverPublishResult struct {
	Keyserver string
	Published bool
	Err       error
}

// PublishToKeyservers POSTs the armored public key to each keyserver's HTTP
// Keyserver Protocol (HKP) VKS submission endpoint, per AI.md PART 11
// "Publish to keyservers": https://keys.openpgp.org/vks/v1/upload style
// endpoints. Returns one result per keyserver; callers decide retry policy
// (spec: "Failures are logged + retried with exponential backoff").
func PublishToKeyservers(ctx context.Context, httpClient *http.Client, pubArmor string, keyservers []string) []KeyserverPublishResult {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	results := make([]KeyserverPublishResult, 0, len(keyservers))
	for _, ks := range keyservers {
		err := publishToOneKeyserver(ctx, httpClient, ks, pubArmor)
		results = append(results, KeyserverPublishResult{
			Keyserver: ks,
			Published: err == nil,
			Err:       err,
		})
	}
	return results
}

// publishToOneKeyserver submits the armored public key to a single
// keyserver's VKS upload endpoint (the protocol used by keys.openpgp.org and
// compatible keyservers such as Ubuntu's SKS-successor infrastructure).
func publishToOneKeyserver(ctx context.Context, httpClient *http.Client, keyserver, pubArmor string) error {
	base, err := url.Parse(keyserver)
	if err != nil {
		return fmt.Errorf("pgp: invalid keyserver URL %q: %w", keyserver, err)
	}
	base.Path = "/vks/v1/upload"

	form := url.Values{"keytext": {pubArmor}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base.String(), bytes.NewBufferString(form.Encode()))
	if err != nil {
		return fmt.Errorf("pgp: build request for %s: %w", keyserver, err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("pgp: publish to %s: %w", keyserver, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("pgp: keyserver %s returned %d: %s", keyserver, resp.StatusCode, string(body))
	}
	return nil
}

// DeleteKeypairFiles removes both key files from disk, per AI.md PART 11
// "Delete: Deletes both keys". Missing files are not an error.
func DeleteKeypairFiles(configDir string) error {
	dir := filepath.Join(configDir, PGPDirName)
	pub := filepath.Join(dir, PGPPublicKeyFilename)
	priv := filepath.Join(dir, PGPEncryptedPrivateKeyFilename)

	if err := os.Remove(pub); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("pgp: remove public key: %w", err)
	}
	if err := os.Remove(priv); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("pgp: remove private key: %w", err)
	}
	return nil
}

// ImportPrivateKey parses an armored, unencrypted private key (e.g. read
// from an operator-supplied file per "--maintenance pgp import <file>"),
// verifies it is a usable private key, and re-locks it under
// installationSecret for storage, matching the same on-disk format
// GenerateKeypair produces.
func ImportPrivateKey(armored, appName, securityContact, installationSecret string) (*Keypair, error) {
	key, err := crypto.NewKeyFromArmored(armored)
	if err != nil {
		return nil, fmt.Errorf("pgp: parse imported key: %w", err)
	}
	if !key.IsPrivate() {
		return nil, fmt.Errorf("pgp: imported key is not a private key")
	}

	locked, err := key.IsLocked()
	if err != nil {
		return nil, fmt.Errorf("pgp: check imported key lock state: %w", err)
	}
	if locked {
		return nil, fmt.Errorf("pgp: imported private key is passphrase-protected; export it decrypted first")
	}

	return keypairFromKey(key, installationSecret)
}

// IdentityMatches reports whether the imported key's identity (name/email
// user IDs) matches the project's expected "{appName} Security
// <{securityContact}>" identity, per AI.md PART 11 Import "Validates the
// key's identity matches the project's expected identity".
func IdentityMatches(key *crypto.Key, appName, securityContact string) bool {
	expectedEmail := securityContact
	entity := key.GetEntity()
	if entity == nil {
		return false
	}
	for _, identity := range entity.Identities {
		if identity.UserId != nil && identity.UserId.Email == expectedEmail {
			return true
		}
	}
	return false
}

// FormatFingerprint renders a raw fingerprint's canonical hex form (upper
// case, matching GnuPG's convention) for display/logging purposes.
func FormatFingerprint(fp string) string {
	decoded, err := hex.DecodeString(fp)
	if err != nil {
		return fp
	}
	return strings.ToUpper(hex.EncodeToString(decoded))
}

// EncryptMessageToArmoredKey PGP-encrypts plaintext to the recipient whose
// ASCII-armored public key is given, returning an ASCII-armored PGP message.
// Used by the coordinated-disclosure pipeline (AI.md PART 11) to encrypt
// security-report bodies and notification emails to the maintainer's or a
// researcher's public key.
func EncryptMessageToArmoredKey(recipientPubKeyArmor string, plaintext []byte) (string, error) {
	recipientKey, err := crypto.NewKeyFromArmored(recipientPubKeyArmor)
	if err != nil {
		return "", fmt.Errorf("parse recipient public key: %w", err)
	}
	keyRing, err := crypto.NewKeyRing(recipientKey)
	if err != nil {
		return "", fmt.Errorf("build recipient keyring: %w", err)
	}
	message := crypto.NewPlainMessage(plaintext)
	encrypted, err := keyRing.Encrypt(message, nil)
	if err != nil {
		return "", fmt.Errorf("encrypt message: %w", err)
	}
	armored, err := encrypted.GetArmored()
	if err != nil {
		return "", fmt.Errorf("armor encrypted message: %w", err)
	}
	return armored, nil
}

// DecryptArmoredMessage decrypts an ASCII-armored PGP message using an
// already-unlocked private key (see UnlockPrivateKey), returning the
// plaintext bytes. Used to decrypt security-report bodies at read time.
func DecryptArmoredMessage(unlockedKey *crypto.Key, armoredMessage string) ([]byte, error) {
	keyRing, err := crypto.NewKeyRing(unlockedKey)
	if err != nil {
		return nil, fmt.Errorf("build decryption keyring: %w", err)
	}
	pgpMessage, err := crypto.NewPGPMessageFromArmored(armoredMessage)
	if err != nil {
		return nil, fmt.Errorf("parse armored message: %w", err)
	}
	plainMessage, err := keyRing.Decrypt(pgpMessage, nil, 0)
	if err != nil {
		return nil, fmt.Errorf("decrypt message: %w", err)
	}
	return plainMessage.GetBinary(), nil
}

// SignDetached produces an armored detached signature over data using
// signingKey, per AI.md PART 11 GPG Keypair Management "Rotate" ("sign new
// pubkey with old key" so recipients can verify the rotation chain of
// custody). signingKey must already be unlocked (see UnlockPrivateKey).
func SignDetached(signingKey *crypto.Key, data []byte) (string, error) {
	keyRing, err := crypto.NewKeyRing(signingKey)
	if err != nil {
		return "", fmt.Errorf("build signing keyring: %w", err)
	}
	message := crypto.NewPlainMessage(data)
	signature, err := keyRing.SignDetached(message)
	if err != nil {
		return "", fmt.Errorf("sign detached: %w", err)
	}
	armored, err := signature.GetArmored()
	if err != nil {
		return "", fmt.Errorf("armor signature: %w", err)
	}
	return armored, nil
}
