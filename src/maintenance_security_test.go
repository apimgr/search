package main

import "testing"

func TestValidSecurityTrackingID(t *testing.T) {
	for _, id := range []string{"sec_0123456789abcdef", "sec_ABCDEF0123456789"} {
		if !validSecurityTrackingID(id) { t.Errorf("validSecurityTrackingID(%q) = false", id) }
	}
	for _, id := range []string{"", "sec_0123456789abcdeg", "sec_0123456789abcdef00", "../sec_0123456789ab"} {
		if validSecurityTrackingID(id) { t.Errorf("validSecurityTrackingID(%q) = true", id) }
	}
}
