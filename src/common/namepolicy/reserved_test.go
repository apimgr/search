package namepolicy

import "testing"

func TestIsReserved(t *testing.T) {
	reservedCases := []string{"api", "ADMIN", "@root", "．admin", "_private", "report.xml", "avatar.svg", "admin1", "root2", "sysadmin7"}
	for _, candidate := range reservedCases {
		if !IsReserved(candidate) {
			t.Errorf("IsReserved(%q) = false, want true", candidate)
		}
	}
	allowedCases := []string{"searcher", "administered", "profile-1", "custom-name", "ordinary"}
	for _, candidate := range allowedCases {
		if IsReserved(candidate) {
			t.Errorf("IsReserved(%q) = true, want false", candidate)
		}
	}
}
