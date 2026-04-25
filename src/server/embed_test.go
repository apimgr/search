package server

import (
	"testing"
	"time"
)

func TestFormatSearchDate(t *testing.T) {
	if got := formatSearchDate(time.Time{}); got != "" {
		t.Fatalf("formatSearchDate(zero) = %q, want empty string", got)
	}

	value := time.Date(2026, time.April, 23, 22, 4, 44, 0, time.UTC)
	if got := formatSearchDate(value); got != "2026-04-23" {
		t.Fatalf("formatSearchDate() = %q, want 2026-04-23", got)
	}
}
