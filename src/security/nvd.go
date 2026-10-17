package security

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/apimgr/search/src/common/closeutil"
)

const (
	nvdPageSize      = 2000
	nvdMaxPageBytes  = 64 << 20
	nvdMaxCacheBytes = 2 << 30
)

type nvdCache struct {
	LastUpdated time.Time  `json:"last_updated"`
	Entries     []CVEEntry `json:"vulnerabilities"`
}

type nvdResponse struct {
	ResultsPerPage  int                `json:"resultsPerPage"`
	StartIndex      int                `json:"startIndex"`
	TotalResults    int                `json:"totalResults"`
	Vulnerabilities []nvdVulnerability `json:"vulnerabilities"`
}

type nvdVulnerability struct {
	CVE nvdCVE `json:"cve"`
}
type nvdCVE struct {
	ID           string `json:"id"`
	Published    string `json:"published"`
	LastModified string `json:"lastModified"`
	Descriptions []struct {
		Lang  string `json:"lang"`
		Value string `json:"value"`
	} `json:"descriptions"`
	Metrics struct {
		V40 []nvdMetric `json:"cvssMetricV40"`
		V31 []nvdMetric `json:"cvssMetricV31"`
		V30 []nvdMetric `json:"cvssMetricV30"`
		V2  []nvdMetric `json:"cvssMetricV2"`
	} `json:"metrics"`
	Configurations []nvdConfiguration `json:"configurations"`
	References     []struct {
		URL string `json:"url"`
	} `json:"references"`
}
type nvdMetric struct {
	CVSS nvdCVSS `json:"cvssData"`
}
type nvdCVSS struct {
	BaseScore    float64 `json:"baseScore"`
	BaseSeverity string  `json:"baseSeverity"`
	Severity     string  `json:"severity"`
}
type nvdConfiguration struct {
	Nodes []nvdNode `json:"nodes"`
}
type nvdNode struct {
	Matches  []nvdCPEMatch `json:"cpeMatch"`
	Children []nvdNode     `json:"children"`
}
type nvdCPEMatch struct {
	Criteria              string `json:"criteria"`
	VersionStartIncluding string `json:"versionStartIncluding"`
	VersionStartExcluding string `json:"versionStartExcluding"`
	VersionEndIncluding   string `json:"versionEndIncluding"`
	VersionEndExcluding   string `json:"versionEndExcluding"`
}

func (m *CVEManager) updateNVD(ctx context.Context, source CVESource) (int, error) {
	base, err := url.Parse(source.URL)
	if err != nil || base.Scheme != "https" || base.Host == "" {
		return 0, fmt.Errorf("invalid NVD API URL")
	}
	now := time.Now().UTC()
	full := m.lastSync.IsZero() || len(m.entries) == 0 || now.Sub(m.lastSync) > 120*24*time.Hour
	if !full {
		q := base.Query()
		q.Set("lastModStartDate", m.lastSync.UTC().Add(-5*time.Minute).Format("2006-01-02T15:04:05.000"))
		q.Set("lastModEndDate", now.Format("2006-01-02T15:04:05.000"))
		base.RawQuery = q.Encode()
	}
	entries := make(map[string]CVEEntry, len(m.entries))
	if !full {
		for id, entry := range m.entries {
			entries[id] = entry
		}
	}
	total, start := 0, 0
	for {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		page := *base
		q := page.Query()
		q.Set("startIndex", strconv.Itoa(start))
		q.Set("resultsPerPage", strconv.Itoa(nvdPageSize))
		page.RawQuery = q.Encode()
		response, err := m.fetchNVDPage(ctx, page.String())
		if err != nil {
			return 0, err
		}
		if start == 0 {
			total = response.TotalResults
		}
		if total < 0 || total > 2_000_000 || response.StartIndex != start ||
			response.ResultsPerPage < len(response.Vulnerabilities) || response.ResultsPerPage > nvdPageSize ||
			(response.ResultsPerPage == 0 && total > 0) || len(response.Vulnerabilities) > nvdPageSize ||
			start+len(response.Vulnerabilities) > total {
			return 0, fmt.Errorf("NVD returned invalid pagination data at offset %d", start)
		}
		if len(response.Vulnerabilities) == 0 && start < total {
			return 0, fmt.Errorf("NVD returned an incomplete page at offset %d", start)
		}
		for _, item := range response.Vulnerabilities {
			if entry, ok := normalizeNVDEntry(item.CVE); ok {
				entries[entry.ID] = entry
			}
		}
		start += len(response.Vulnerabilities)
		if start >= total {
			break
		}
		if err := waitDuration(ctx, 6*time.Second); err != nil {
			return 0, err
		}
	}
	cache := nvdCache{LastUpdated: now, Entries: make([]CVEEntry, 0, len(entries))}
	for _, entry := range entries {
		cache.Entries = append(cache.Entries, entry)
	}
	sort.Slice(cache.Entries, func(i, j int) bool {
		return cache.Entries[i].ID < cache.Entries[j].ID
	})
	if err := m.writeNVDCache(cache); err != nil {
		return 0, err
	}
	added := 0
	for id := range entries {
		if _, ok := m.entries[id]; !ok {
			added++
		}
	}
	m.entries, m.lastSync = entries, now
	return added, nil
}

func (m *CVEManager) fetchNVDPage(ctx context.Context, endpoint string) (nvdResponse, error) {
	var empty nvdResponse
	for attempt := 0; attempt < 4; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return empty, fmt.Errorf("create NVD request: %w", err)
		}
		req.Header.Set("User-Agent", "search/1.0 (NVD security database update)")
		req.Header.Set("Accept", "application/json")
		resp, err := m.client.Do(req)
		if err != nil {
			return empty, fmt.Errorf("request NVD data: %w", err)
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, nvdMaxPageBytes+1))
		closeutil.Close(resp.Body)
		if readErr != nil {
			return empty, fmt.Errorf("read NVD response: %w", readErr)
		}
		if len(body) > nvdMaxPageBytes {
			return empty, fmt.Errorf("NVD response exceeds size limit")
		}
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusForbidden || resp.StatusCode >= 500 {
			if attempt == 3 {
				return empty, fmt.Errorf("NVD returned HTTP %d after retries", resp.StatusCode)
			}
			delay := time.Duration(attempt+1) * 10 * time.Second
			if seconds, parseErr := strconv.Atoi(resp.Header.Get("Retry-After")); parseErr == nil && seconds > 0 && seconds < 300 {
				delay = time.Duration(seconds) * time.Second
			}
			if err := waitDuration(ctx, delay); err != nil {
				return empty, err
			}
			continue
		}
		if resp.StatusCode != http.StatusOK {
			return empty, fmt.Errorf("NVD returned HTTP %d", resp.StatusCode)
		}
		var result nvdResponse
		if err := json.Unmarshal(body, &result); err != nil {
			return empty, fmt.Errorf("decode NVD response: %w", err)
		}
		if result.Vulnerabilities == nil && result.TotalResults != 0 {
			return empty, fmt.Errorf("NVD response omitted vulnerabilities")
		}
		return result, nil
	}
	return empty, fmt.Errorf("NVD request failed")
}

func waitDuration(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (m *CVEManager) writeNVDCache(cache nvdCache) error {
	dir := m.cveCacheDir()
	if err := os.MkdirAll(dir, 0750); err != nil {
		return fmt.Errorf("create NVD cache directory: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".nvd-*.tmp")
	if err != nil {
		return fmt.Errorf("create NVD cache temp file: %w", err)
	}
	name := tmp.Name()
	defer func() {
		if err := os.Remove(name); err != nil && !os.IsNotExist(err) {
			slog.Warn("failed to remove temporary NVD cache", "error", err)
		}
	}()
	w := &nvdBoundedWriter{writer: tmp, remaining: nvdMaxCacheBytes}
	if err := json.NewEncoder(w).Encode(cache); err != nil {
		return fmt.Errorf("encode NVD cache: %w", errors.Join(err, tmp.Close()))
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("sync NVD cache: %w", errors.Join(err, tmp.Close()))
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close NVD cache: %w", err)
	}
	if err := os.Rename(name, filepath.Join(dir, "nvd.json")); err != nil {
		return fmt.Errorf("replace NVD cache: %w", err)
	}
	marker, err := os.CreateTemp(dir, ".last-updated-*.tmp")
	if err != nil {
		return fmt.Errorf("create NVD timestamp: %w", err)
	}
	markerName := marker.Name()
	defer func() {
		if err := os.Remove(markerName); err != nil && !os.IsNotExist(err) {
			slog.Warn("failed to remove temporary NVD update marker", "error", err)
		}
	}()
	if _, err := fmt.Fprintln(marker, cache.LastUpdated.UTC().Format(time.RFC3339)); err != nil {
		return fmt.Errorf("write NVD timestamp: %w", errors.Join(err, marker.Close()))
	}
	if err := marker.Sync(); err != nil {
		return fmt.Errorf("sync NVD timestamp: %w", errors.Join(err, marker.Close()))
	}
	if err := marker.Close(); err != nil {
		return fmt.Errorf("close NVD timestamp: %w", err)
	}
	if err := os.Rename(markerName, filepath.Join(dir, ".last_updated")); err != nil {
		return fmt.Errorf("replace NVD timestamp: %w", err)
	}
	return nil
}

type nvdBoundedWriter struct {
	writer    io.Writer
	remaining int64
}

func (w *nvdBoundedWriter) Write(data []byte) (int, error) {
	if int64(len(data)) > w.remaining {
		return 0, fmt.Errorf("NVD cache exceeds size limit")
	}
	n, err := w.writer.Write(data)
	w.remaining -= int64(n)
	return n, err
}

func normalizeNVDEntry(cve nvdCVE) (CVEEntry, bool) {
	id := strings.ToUpper(strings.TrimSpace(cve.ID))
	if !validCVEID(id) {
		return CVEEntry{}, false
	}
	entry := CVEEntry{ID: id, Published: cve.Published, Modified: cve.LastModified}
	for _, d := range cve.Descriptions {
		if d.Lang == "en" && d.Value != "" {
			entry.Description = d.Value
			break
		}
	}
	if entry.Description == "" && len(cve.Descriptions) > 0 {
		entry.Description = cve.Descriptions[0].Value
	}
	for _, group := range [][]nvdMetric{cve.Metrics.V40, cve.Metrics.V31, cve.Metrics.V30, cve.Metrics.V2} {
		if len(group) == 0 {
			continue
		}
		entry.CVSS = group[0].CVSS.BaseScore
		entry.Severity = strings.ToUpper(group[0].CVSS.BaseSeverity)
		if entry.Severity == "" {
			entry.Severity = strings.ToUpper(group[0].CVSS.Severity)
		}
		break
	}
	for _, ref := range cve.References {
		if ref.URL != "" {
			entry.References = append(entry.References, ref.URL)
		}
	}
	var walk func(nvdNode)
	walk = func(node nvdNode) {
		for _, m := range node.Matches {
			if m.Criteria == "" {
				continue
			}
			v := m.Criteria
			for _, b := range []struct{ name, value string }{{"start including", m.VersionStartIncluding}, {"start excluding", m.VersionStartExcluding}, {"end including", m.VersionEndIncluding}, {"end excluding", m.VersionEndExcluding}} {
				if b.value != "" {
					v += " (" + b.name + " " + b.value + ")"
				}
			}
			entry.Affected = append(entry.Affected, v)
		}
		for _, child := range node.Children {
			walk(child)
		}
	}
	for _, cfg := range cve.Configurations {
		for _, node := range cfg.Nodes {
			walk(node)
		}
	}
	return entry, true
}

func readNVDCache(path string) (nvdCache, error) {
	var cache nvdCache
	f, err := os.Open(path)
	if err != nil {
		return cache, err
	}
	defer closeutil.Close(f)
	info, err := f.Stat()
	if err != nil {
		return cache, err
	}
	if info.Size() > nvdMaxCacheBytes {
		return cache, fmt.Errorf("NVD cache exceeds size limit")
	}
	dec := json.NewDecoder(io.LimitReader(f, nvdMaxCacheBytes+1))
	if err := dec.Decode(&cache); err != nil {
		return cache, err
	}
	if cache.Entries == nil {
		return cache, fmt.Errorf("NVD cache has no vulnerability list")
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		return cache, fmt.Errorf("NVD cache has trailing data")
	}
	return cache, nil
}
