package service

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// vanityCharset is the base32 alphabet a v3 onion address can contain.
// Per AI.md PART 31.1: "0", "1", "8", "9" and uppercase never appear.
const vanityCharset = "abcdefghijklmnopqrstuvwxyz234567"

// vanityMaxPrefix is the longest prefix the built-in search accepts. Longer
// prefixes are external-only per AI.md PART 31.1 — the operator generates them
// with a GPU-capable tool and brings them in via `tor import-keys`.
const vanityMaxPrefix = 6

// VanityProgress reports the state of the built-in vanity onion search per
// AI.md PART 31.1 "Vanity Onion Address Search".
type VanityProgress struct {
	// idle, running or found
	State   string
	Prefix  string
	Workers int
	// Approximate: per-worker counters, sampled
	Attempts       int64
	Rate           float64
	ElapsedSeconds float64
	StartTime      time.Time
	Running        bool
	Found          bool
	Address        string
	// Expanded ed25519 secret key of the found address — used by ApplyVanityAddress
	PrivateKey []byte
	// Addresses of every candidate currently on disk
	Candidates []string
	Error      string
}

// vanityGenerator holds the state of the single in-process vanity search.
// Per AI.md PART 31.1 only one search may run at a time.
type vanityGenerator struct {
	progress *VanityProgress
	cancel   context.CancelFunc
	attempts atomic.Int64
	wg       sync.WaitGroup
	mu       sync.RWMutex
}

var vanityGen = &vanityGenerator{
	progress: &VanityProgress{State: "idle"},
}

// vanityDir returns {data_dir}/tor/vanity — the candidate key store.
func (t *TorService) vanityDir() string {
	return filepath.Join(t.dataDir, "vanity")
}

// listVanityCandidates returns the addresses of every candidate key directory
// currently on disk, sorted by directory order.
func (t *TorService) listVanityCandidates() []string {
	entries, err := os.ReadDir(t.vanityDir())
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			out = append(out, e.Name())
		}
	}
	return out
}

// GetVanityProgress returns a snapshot of the current search progress.
func (t *TorService) GetVanityProgress() *VanityProgress {
	vanityGen.mu.RLock()
	defer vanityGen.mu.RUnlock()

	p := vanityGen.progress
	pk := make([]byte, len(p.PrivateKey))
	copy(pk, p.PrivateKey)

	snapshot := &VanityProgress{
		State:      p.State,
		Prefix:     p.Prefix,
		Workers:    p.Workers,
		Attempts:   vanityGen.attempts.Load(),
		StartTime:  p.StartTime,
		Running:    p.Running,
		Found:      p.Found,
		Address:    p.Address,
		PrivateKey: pk,
		Candidates: t.listVanityCandidates(),
		Error:      p.Error,
	}
	if !p.StartTime.IsZero() {
		snapshot.ElapsedSeconds = time.Since(p.StartTime).Seconds()
		if snapshot.ElapsedSeconds > 0 {
			snapshot.Rate = float64(snapshot.Attempts) / snapshot.ElapsedSeconds
		}
	}
	return snapshot
}

// GenerateVanity starts a vanity search with the default worker count
// (logical CPUs - 1, minimum 1) per AI.md PART 31.1.
func (t *TorService) GenerateVanity(prefix string) error {
	return t.GenerateVanityWithWorkers(prefix, 0)
}

// GenerateVanityWithWorkers starts the background vanity search. workers <= 0
// selects the default (logical CPUs - 1, minimum 1). Per AI.md PART 31.1 the
// prefix must be 1-6 base32 characters and only one search may run at a time.
func (t *TorService) GenerateVanityWithWorkers(prefix string, workers int) error {
	// AI.md PART 31.1: the prefix is validated raw, not trimmed. A space is not
	// in the a-z2-7 charset, so "abc " is an invalid prefix and must be rejected
	// rather than silently accepted as "abc".
	if prefix == "" {
		return fmt.Errorf("prefix is required: 1-%d characters from %s", vanityMaxPrefix, vanityCharset)
	}
	if len(prefix) > vanityMaxPrefix {
		return fmt.Errorf("prefix too long: the built-in search accepts at most %d characters; generate longer prefixes with an external GPU-capable tool (e.g. mkp224o) and import them with `tor import-keys`", vanityMaxPrefix)
	}
	for _, c := range prefix {
		if !strings.ContainsRune(vanityCharset, c) {
			return fmt.Errorf("invalid character '%c' in prefix: must be lowercase a-z or 2-7", c)
		}
	}

	maxWorkers := runtime.NumCPU()
	if workers <= 0 {
		workers = maxWorkers - 1
		if workers < 1 {
			workers = 1
		}
	}
	if workers > maxWorkers {
		return fmt.Errorf("workers must be between 1 and %d (logical CPUs)", maxWorkers)
	}

	vanityGen.mu.Lock()
	if vanityGen.progress.Running {
		running := vanityGen.progress.Prefix
		vanityGen.mu.Unlock()
		return fmt.Errorf("a vanity search is already running for prefix %q", running)
	}

	ctx, cancel := context.WithCancel(context.Background())
	vanityGen.cancel = cancel
	vanityGen.attempts.Store(0)
	vanityGen.progress = &VanityProgress{
		State:     "running",
		Prefix:    prefix,
		Workers:   workers,
		StartTime: time.Now(),
		Running:   true,
	}
	vanityGen.mu.Unlock()

	if err := os.MkdirAll(t.vanityDir(), 0700); err != nil {
		t.CancelVanity()
		return fmt.Errorf("create vanity dir: %w", err)
	}

	for i := 0; i < workers; i++ {
		vanityGen.wg.Add(1)
		go func() {
			defer vanityGen.wg.Done()
			t.runVanityWorker(ctx, cancel, prefix)
		}()
	}

	return nil
}

// runVanityWorker brute-forces keypairs until one derives an address with the
// requested prefix, then persists it as a candidate and stops the search.
func (t *TorService) runVanityWorker(ctx context.Context, cancel context.CancelFunc, prefix string) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		address, expanded, pub, err := generateOnionV3Candidate()
		vanityGen.attempts.Add(1)
		if err != nil {
			vanityGen.mu.Lock()
			vanityGen.progress.Error = err.Error()
			vanityGen.mu.Unlock()
			continue
		}

		if !strings.HasPrefix(address, prefix) {
			continue
		}

		if err := t.writeVanityCandidate(address, expanded, pub); err != nil {
			vanityGen.mu.Lock()
			vanityGen.progress.Error = err.Error()
			vanityGen.mu.Unlock()
			cancel()
			return
		}

		vanityGen.mu.Lock()
		vanityGen.progress.State = "found"
		vanityGen.progress.Found = true
		vanityGen.progress.Running = false
		vanityGen.progress.Address = address
		vanityGen.progress.PrivateKey = expanded
		attempts := vanityGen.attempts.Load()
		vanityGen.mu.Unlock()

		// Only the address is ever logged — key material never is.
		slog.Info("Tor vanity address found", "address", address+".onion", "attempts", attempts)
		cancel()
		return
	}
}

// writeVanityCandidate persists a found candidate atomically (temp dir +
// rename) under {data_dir}/tor/vanity/{address}/ per AI.md PART 31.1. It never
// writes into the live {data_dir}/tor/site/ directory.
func (t *TorService) writeVanityCandidate(address string, expanded, pub []byte) error {
	base := t.vanityDir()
	if err := os.MkdirAll(base, 0700); err != nil {
		return fmt.Errorf("create vanity dir: %w", err)
	}
	tmp := filepath.Join(base, ".tmp-"+address)
	if err := os.RemoveAll(tmp); err != nil {
		return fmt.Errorf("remove previous vanity staging directory: %w", err)
	}
	if err := writeTorKeyDir(tmp, expanded, pub, address); err != nil {
		return errors.Join(err, os.RemoveAll(tmp))
	}
	final := filepath.Join(base, address)
	if err := os.Rename(tmp, final); err != nil {
		return fmt.Errorf("publish candidate: %w", errors.Join(err, os.RemoveAll(tmp)))
	}
	return nil
}

// CancelVanity cancels a running search and blocks until every worker
// goroutine has exited, so no further writes land in the vanity directory
// after it returns. Candidates already written to disk are kept, per AI.md
// PART 31.1.
func (t *TorService) CancelVanity() {
	vanityGen.mu.Lock()
	if vanityGen.cancel != nil {
		vanityGen.cancel()
		vanityGen.cancel = nil
	}
	vanityGen.progress.Running = false
	if vanityGen.progress.State == "running" {
		vanityGen.progress.State = "idle"
	}
	vanityGen.mu.Unlock()

	vanityGen.wg.Wait()
}

// ApplyVanityAddress applies the only candidate on disk (or the most recent
// find). Per AI.md PART 31.1 the address argument MAY be omitted when exactly
// one candidate exists.
func (t *TorService) ApplyVanityAddress() (string, error) {
	return t.ApplyVanityCandidate("")
}

// ApplyVanityCandidate resolves match against {data_dir}/tor/vanity/ (a unique
// prefix of one candidate is accepted; an empty match requires exactly one
// candidate), then swaps its keys into the live site directory and restarts
// Tor, verifying the published hostname per AI.md PART 31.1.
func (t *TorService) ApplyVanityCandidate(match string) (string, error) {
	candidates := t.listVanityCandidates()
	if len(candidates) == 0 {
		return "", fmt.Errorf("no vanity candidates found in %s", t.vanityDir())
	}

	match = strings.TrimSpace(strings.ToLower(strings.TrimSuffix(match, ".onion")))
	var matched []string
	if match == "" {
		matched = candidates
	} else {
		for _, c := range candidates {
			if strings.HasPrefix(c, match) {
				matched = append(matched, c)
			}
		}
	}
	if len(matched) != 1 {
		return "", fmt.Errorf("expected exactly one matching candidate, got %d: %s", len(matched), strings.Join(candidates, ", "))
	}

	address := matched[0]
	applied, err := t.applyKeyDir(filepath.Join(t.vanityDir(), address), address)
	if err != nil {
		return "", err
	}

	// The candidate directory is consumed by the swap; other candidates stay.
	if err := os.RemoveAll(filepath.Join(t.vanityDir(), address)); err != nil {
		return "", fmt.Errorf("remove vanity candidate: %w", err)
	}

	vanityGen.mu.Lock()
	vanityGen.progress.State = "idle"
	vanityGen.progress.Found = false
	vanityGen.progress.Address = ""
	vanityGen.progress.PrivateKey = nil
	vanityGen.mu.Unlock()

	return applied, nil
}

// applyKeyDir stops Tor, replaces the live hidden service keys with the three
// files in srcDir, restarts Tor, and verifies the published hostname matches
// expected (empty means "trust the imported hostname file"). A mismatch fails
// loudly rather than silently serving an unexpected address.
func (t *TorService) applyKeyDir(srcDir, expected string) (string, error) {
	for _, name := range torKeyFiles {
		if _, err := os.Stat(filepath.Join(srcDir, name)); err != nil {
			return "", fmt.Errorf("key directory %s is missing %s", srcDir, name)
		}
	}

	if expected == "" {
		hostBytes, err := os.ReadFile(filepath.Join(srcDir, "hostname"))
		if err != nil {
			return "", fmt.Errorf("read hostname: %w", err)
		}
		expected = strings.TrimSuffix(strings.TrimSpace(string(hostBytes)), ".onion")
	}

	if err := t.StopTorService(); err != nil {
		return "", fmt.Errorf("stop tor: %w", err)
	}

	siteDir := filepath.Join(t.dataDir, "site")
	if err := os.RemoveAll(siteDir); err != nil {
		return "", fmt.Errorf("remove old keys: %w", err)
	}
	if err := os.MkdirAll(siteDir, 0700); err != nil {
		return "", fmt.Errorf("create site dir: %w", err)
	}
	for _, name := range torKeyFiles {
		data, err := os.ReadFile(filepath.Join(srcDir, name))
		if err != nil {
			return "", fmt.Errorf("read %s: %w", name, err)
		}
		if err := os.WriteFile(filepath.Join(siteDir, name), data, 0600); err != nil {
			return "", fmt.Errorf("write %s: %w", name, err)
		}
	}

	if err := t.StartTorService(); err != nil {
		return "", fmt.Errorf("start tor with new keys: %w", err)
	}

	got := strings.TrimSuffix(t.GetOnionAddress(), ".onion")
	if got != "" && got != expected {
		slog.Error("Tor published hostname does not match applied keys", "expected", expected, "got", got)
		return "", fmt.Errorf("published hostname %s does not match applied keys %s", got, expected)
	}

	return expected + ".onion", nil
}

// generateOnionV3Candidate generates a fresh ed25519 keypair and derives its v3
// onion address, returning Tor's expanded secret key alongside the public key.
func generateOnionV3Candidate() (address string, expanded, pub []byte, err error) {
	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", nil, nil, err
	}
	expanded = expandedFromSeed(privKey.Seed())
	return onionAddressFromPublic(pubKey), expanded, pubKey, nil
}

// generateOnionV3Address returns a fresh v3 onion address (56-character base32,
// without the .onion suffix) and its expanded ed25519 secret key.
func generateOnionV3Address() (string, []byte, error) {
	address, expanded, _, err := generateOnionV3Candidate()
	if err != nil {
		return "", nil, err
	}
	return address, expanded, nil
}
