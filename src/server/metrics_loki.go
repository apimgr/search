package server

import (
	"bufio"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"time"

	"github.com/apimgr/search/src/common/closeutil"
)

// lokiStream is one Loki push-API stream: a label set plus its
// [nanosecond-timestamp, line] value pairs.
type lokiStream struct {
	Stream map[string]string `json:"stream"`
	Values [][2]string       `json:"values"`
}

// lokiResponse is the Loki push-API envelope served by /server/metrics/loki.
type lokiResponse struct {
	Streams []lokiStream `json:"streams"`
}

// lokiJobLabel is the Loki "job" label for every stream this server emits.
// It matches the Prometheus metric prefix so both services key on one name.
const lokiJobLabel = "search"

// lokiSourceLogs are the structured JSON log files the loki service exposes.
// Each becomes its own stream, labelled by its log name.
var lokiSourceLogs = []string{"server.log", "error.log", "app.log", "security.log", "audit.log"}

// lokiCredentialPattern matches a JSON key whose name carries credential
// material, so its value can be redacted before the line ever leaves the
// process. AI.md PART 20 requires the loki service to apply the same
// sanitization as the log files themselves.
var lokiCredentialPattern = regexp.MustCompile(`(?i)"([^"]*(?:secret|password|token|api[_-]?key|authorization)[^"]*)"\s*:\s*"[^"]*"`)

// lokiTimestampPattern extracts the RFC3339 timestamp from a structured log
// line so entries can be bounded by loki.max_age.
var lokiTimestampPattern = regexp.MustCompile(`"(?:timestamp|time)"\s*:\s*"([^"]+)"`)

// metricsLokiHandler serves GET /server/metrics/loki — the recent tail of the
// structured log files in Loki push-API stream format, bounded by
// server.metrics.loki.max_entries and max_age (AI.md PART 20 "Service
// Semantics"). Credentials are always redacted.
func (s *Server) metricsLokiHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")

		resp := lokiResponse{Streams: []lokiStream{}}
		if s.logManager != nil {
			maxEntries := s.config.Server.Metrics.Loki.MaxEntries
			if maxEntries <= 0 {
				maxEntries = 1000
			}
			maxAge, err := time.ParseDuration(s.config.Server.Metrics.Loki.MaxAge)
			if err != nil || maxAge <= 0 {
				maxAge = time.Hour
			}
			cutoff := time.Now().Add(-maxAge)

			for _, name := range lokiSourceLogs {
				values := readLokiEntries(filepath.Join(s.logManager.LogDir(), name), maxEntries, cutoff)
				if len(values) == 0 {
					continue
				}
				resp.Streams = append(resp.Streams, lokiStream{
					Stream: map[string]string{
						"job": lokiJobLabel,
						"log": name,
					},
					Values: values,
				})
			}
		}

		_ = json.NewEncoder(w).Encode(resp)
	})
}

// readLokiEntries returns up to maxEntries redacted lines from path that are
// newer than cutoff, oldest first, as [ns-timestamp, line] pairs. A missing or
// unreadable log file yields no entries rather than an error: the loki service
// degrades gracefully like every other optional feature.
func readLokiEntries(path string, maxEntries int, cutoff time.Time) [][2]string {
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer closeutil.Close(file)

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	// Keep only the newest maxEntries lines in a ring so a large log file never
	// materializes in memory all at once.
	ring := make([][2]string, 0, maxEntries)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		stamp := lokiEntryTime(line)
		if stamp.Before(cutoff) {
			continue
		}
		entry := [2]string{strconv.FormatInt(stamp.UnixNano(), 10), lokiCredentialPattern.ReplaceAllString(line, `"$1":"xxxxx"`)}
		if len(ring) == maxEntries {
			ring = append(ring[1:], entry)
			continue
		}
		ring = append(ring, entry)
	}
	if err := scanner.Err(); err != nil {
		return nil
	}

	sort.SliceStable(ring, func(i, j int) bool { return ring[i][0] < ring[j][0] })
	return ring
}

// lokiEntryTime parses a structured log line's timestamp, falling back to the
// current time for a line that carries none so it is still surfaced.
func lokiEntryTime(line string) time.Time {
	match := lokiTimestampPattern.FindStringSubmatch(line)
	if len(match) != 2 {
		return time.Now()
	}
	parsed, err := time.Parse(time.RFC3339Nano, match[1])
	if err != nil {
		return time.Now()
	}
	return parsed
}
