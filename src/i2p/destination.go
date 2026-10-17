package i2p

import (
	"crypto/sha256"
	"encoding/base32"
	"encoding/base64"
	"fmt"
	"strings"
)

// i2pBase64 is the I2P variant of base64: identical to the standard alphabet
// except '+' and '/' are replaced by '-' and '~'. SAM and i2pd both use it for
// destinations and private-key blobs.
var i2pBase64 = base64.NewEncoding(
	"ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-~",
)

// minDestinationLen is the size of a destination carrying a NULL certificate:
// 256-byte public key + 128-byte signing public key + 3-byte certificate
// header. Certificates longer than the header extend the destination.
const minDestinationLen = 387

// b32Address derives the .b32.i2p address: base32(sha256(destination)) without
// padding, lowercased, plus the ".b32.i2p" suffix.
// Per AI.md PART 31.2 "SAM Session (Model B)".
func b32Address(destBinary []byte) string {
	sum := sha256.Sum256(destBinary)
	enc := base32.StdEncoding.WithPadding(base32.NoPadding)
	return strings.ToLower(enc.EncodeToString(sum[:])) + ".b32.i2p"
}

// destinationFromKeys extracts the leading destination from an I2P private-key
// blob. Both the SAM `DEST PRIV=` value and the i2pd `site-keys.dat` file store
// the destination first, followed by the private keys, so the same parse serves
// Model A and Model B. The certificate header at offset 384 is
// [type:1][length:2] and the declared length extends the destination.
func destinationFromKeys(keys []byte) ([]byte, error) {
	if len(keys) < minDestinationLen {
		return nil, fmt.Errorf("i2p key blob too short: %d bytes", len(keys))
	}
	certLen := int(keys[385])<<8 | int(keys[386])
	total := minDestinationLen + certLen
	if len(keys) < total {
		return nil, fmt.Errorf("i2p key blob truncated: need %d bytes, have %d", total, len(keys))
	}
	return keys[:total], nil
}

// decodeI2PBase64 decodes an I2P base64 string, tolerating the padding-free
// form some routers emit.
func decodeI2PBase64(s string) ([]byte, error) {
	s = strings.TrimSpace(s)
	if pad := len(s) % 4; pad != 0 {
		s += strings.Repeat("=", 4-pad)
	}
	b, err := i2pBase64.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("decode i2p base64: %w", err)
	}
	return b, nil
}
