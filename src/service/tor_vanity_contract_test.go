package service

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestApplyVanityCandidateReturnsNotFoundError(t *testing.T) {
	ts := newTestTorService(t, nil)
	if _, err := ts.ApplyVanityCandidate("missing"); !errors.Is(err, ErrVanityCandidateNotFound) {
		t.Fatalf("ApplyVanityCandidate() error = %v, want ErrVanityCandidateNotFound", err)
	}
}

func TestApplyVanityCandidateReturnsAmbiguousMatches(t *testing.T) {
	ts := newTestTorService(t, nil)
	for _, candidate := range []string{"abc2candidate", "abc3candidate"} {
		if err := os.MkdirAll(filepath.Join(ts.vanityDir(), candidate), 0o700); err != nil {
			t.Fatalf("create candidate directory: %v", err)
		}
	}
	_, err := ts.ApplyVanityCandidate("abc")
	var ambiguous *AmbiguousVanityCandidateError
	if !errors.As(err, &ambiguous) {
		t.Fatalf("ApplyVanityCandidate() error = %v, want AmbiguousVanityCandidateError", err)
	}
	if len(ambiguous.Candidates) != 2 {
		t.Errorf("ambiguous candidates = %v, want two matches", ambiguous.Candidates)
	}
}

func TestGenerateVanityReturnsConflictType(t *testing.T) {
	ts := newTestTorService(t, nil)
	if err := ts.GenerateVanity("abcdef"); err != nil {
		t.Fatalf("GenerateVanity() error = %v", err)
	}
	defer ts.CancelVanity()

	err := ts.GenerateVanity("ghijkl")
	var conflict *VanitySearchConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("GenerateVanity() error = %v, want VanitySearchConflictError", err)
	}
	if conflict.Prefix != "abcdef" {
		t.Errorf("conflict prefix = %q, want abcdef", conflict.Prefix)
	}
}
