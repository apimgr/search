package service

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cretz/bine/control"
)

const torStatusControlTimeout = 500 * time.Millisecond

// ControlStatus returns the read-only status projection required by the Tor
// loopback control endpoint. It excludes service-internal addresses, circuits,
// and key material.
func (t *TorService) ControlStatus() map[string]any {
	t.mu.RLock()
	running := t.running && t.tor != nil
	onionAddress := t.onionAddr
	torInstance := t.tor
	dataDir := t.dataDir
	t.mu.RUnlock()

	version := ""
	bootstrapPercent := 0
	circuitEstablished := false
	if running && torInstance.Control != nil {
		if info, ok := readTorStatusControl(&t.statusQueryMu, torInstance.Control, torStatusControlTimeout); ok {
			version = info["version"]
			bootstrapPercent = torBootstrapPercent(info["status/bootstrap-phase"])
			circuitEstablished = torCircuitEstablished(info["circuit-status"])
		}
	}

	vanity := t.GetVanityProgress()
	if vanity.State == "" {
		vanity.State = "idle"
	}
	vanityStatus := map[string]any{
		"state":           vanity.State,
		"prefix":          vanity.Prefix,
		"workers":         vanity.Workers,
		"attempts":        vanity.Attempts,
		"rate":            vanity.Rate,
		"elapsed_seconds": vanity.ElapsedSeconds,
		"candidates":      append([]string{}, vanity.Candidates...),
	}
	status := map[string]any{
		"running":             running,
		"version":             version,
		"bootstrap_percent":   bootstrapPercent,
		"circuit_established": circuitEstablished,
		"vanity":              vanityStatus,
	}
	if running {
		if pidData, err := os.ReadFile(filepath.Join(dataDir, "tor.pid")); err == nil {
			if pid, err := strconv.Atoi(strings.TrimSpace(string(pidData))); err == nil && pid > 0 {
				status["pid"] = pid
			}
		}
	}
	if onionAddress != "" {
		status["onion_address"] = onionAddress
	}
	return status
}

func readTorStatusControl(queryMu *sync.Mutex, controlConn *control.Conn, timeout time.Duration) (map[string]string, bool) {
	// bine does not expose the underlying control socket's deadline setter, so
	// isolate its blocking request from the HTTP status caller with a timeout.
	if !queryMu.TryLock() {
		return nil, false
	}
	result := make(chan map[string]string, 1)
	go func() {
		defer queryMu.Unlock()
		values, err := controlConn.GetInfo("version", "status/bootstrap-phase", "circuit-status")
		if err != nil {
			result <- nil
			return
		}
		info := make(map[string]string, len(values))
		for _, item := range values {
			if item != nil {
				info[item.Key] = item.Val
			}
		}
		result <- info
	}()

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case info := <-result:
		return info, info != nil
	case <-timer.C:
		return nil, false
	}
}

func torBootstrapPercent(status string) int {
	for _, field := range strings.Fields(status) {
		key, value, ok := strings.Cut(field, "=")
		if !ok || key != "PROGRESS" {
			continue
		}
		progress, err := strconv.Atoi(value)
		if err != nil {
			return 0
		}
		if progress < 0 {
			return 0
		}
		if progress > 100 {
			return 100
		}
		return progress
	}
	return 0
}

func torCircuitEstablished(status string) bool {
	for _, line := range strings.Split(status, "\n") {
		fields := strings.Fields(line)
		if len(fields) > 1 && fields[1] == "BUILT" {
			return true
		}
	}
	return false
}
