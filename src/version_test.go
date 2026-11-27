package main

import (
	"testing"
	"time"
)

func TestBuildEpochParsesEmbeddedValue(t *testing.T) {
	previous := BuildEpoch
	defer func() { BuildEpoch = previous }()
	BuildEpoch = "1765112713"
	if got := buildEpoch(); got != 1765112713 {
		t.Fatalf("buildEpoch() = %d, want %d", got, int64(1765112713))
	}
	BuildEpoch = "invalid"
	if got := buildEpoch(); got != 0 {
		t.Fatalf("buildEpoch() for invalid input = %d, want 0", got)
	}
}

func TestBuildDateFromEpochUsesUTC(t *testing.T) {
	const epoch = int64(1765112713)
	want := time.Unix(epoch, 0).UTC().Format("2006-01-02T15:04:05Z")
	if got := buildDateFromEpoch(epoch); got != want {
		t.Fatalf("buildDateFromEpoch(%d) = %q, want %q", epoch, got, want)
	}
	if got := buildDateFromEpoch(0); got != "N/A" {
		t.Fatalf("buildDateFromEpoch(0) = %q, want N/A", got)
	}
}
