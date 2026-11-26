package service

import "testing"

func TestValidateTorKeyMaterialRejectsInvalidBytes(t *testing.T) {
	if err := ValidateTorKeyMaterial([]byte("not a tor key")); err == nil {
		t.Fatal("ValidateTorKeyMaterial accepted invalid key bytes")
	}
}
