package service

import "errors"

// ValidateTorKeyMaterial checks whether bytes contain a supported Tor v3
// expanded secret key, seed, or Tor key-file representation.
func ValidateTorKeyMaterial(privateKey []byte) error {
	if len(privateKey) == 0 {
		return errors.New("private key is empty")
	}
	_, _, _, err := parseTorKeyMaterial(privateKey)
	return err
}
