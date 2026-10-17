package instant

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/apimgr/search/src/common/closeutil"
	"github.com/apimgr/search/src/common/httputil"
	"github.com/apimgr/search/src/version"
	"golang.org/x/mod/modfile"
	"golang.org/x/mod/module"
)

// AnswerTypePkg is the answer type for package lookups
const AnswerTypePkg AnswerType = "pkg"

const maxPackageMetadataBytes = 5 << 20

// PkgHandler handles package information lookups across the supported registries.
type PkgHandler struct {
	client     *http.Client
	patterns   []*regexp.Regexp
	cacheMu    sync.Mutex
	popularity map[string]packagePopularity
	readmeMu   sync.Mutex
	readmes    map[string]cachedPackageReadme
}

type packagePopularity struct {
	weeklyDownloads int64
	available       bool
	expiresAt       time.Time
}

type cachedPackageReadme struct {
	text      string
	expiresAt time.Time
}

// NewPkgHandler creates a new package handler
func NewPkgHandler() *PkgHandler {
	return &PkgHandler{
		client:     &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
		popularity: make(map[string]packagePopularity),
		readmes:    make(map[string]cachedPackageReadme),
		patterns: []*regexp.Regexp{
			// pkg:registry:name format
			regexp.MustCompile(`(?i)^pkg[:\s]+(npm|pypi|pip|go|crates|cargo|rubygems|gem)[:/]\s*(.+)$`),
			// pkg:name (auto-detect)
			regexp.MustCompile(`(?i)^pkg[:\s]+(.+)$`),
			// registry-specific shortcuts
			regexp.MustCompile(`(?i)^npm[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^pypi[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^pip[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^gopkg[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^(crates|cargo|rubygems|gem)[:\s]+(.+)$`),
		},
	}
}

func (h *PkgHandler) Name() string               { return "pkg" }
func (h *PkgHandler) Patterns() []*regexp.Regexp { return h.patterns }

func (h *PkgHandler) CanHandle(query string) bool {
	for _, p := range h.patterns {
		if p.MatchString(query) {
			return true
		}
	}
	return false
}

func (h *PkgHandler) HandleInstantQuery(ctx context.Context, query string) (*Answer, error) {
	registry, pkgName := h.parseQuery(query)

	if pkgName == "" {
		return nil, nil
	}

	pkgName = strings.TrimSpace(pkgName)

	// If no registry specified, try to auto-detect or use npm as default
	if registry == "" {
		registry = h.detectRegistry(pkgName)
	}

	var answer *Answer
	var err error

	switch registry {
	case "npm":
		answer, err = h.fetchNPM(ctx, pkgName)
	case "pypi":
		answer, err = h.fetchPyPI(ctx, pkgName)
	case "go":
		answer, err = h.fetchGoPkg(ctx, pkgName)
	case "crates", "cargo":
		answer, err = h.fetchCrate(ctx, pkgName)
	case "rubygems", "gem":
		answer, err = h.fetchRubyGem(ctx, pkgName)
	default:
		// Try npm first, then pypi
		answer, err = h.fetchNPM(ctx, pkgName)
		if answer != nil && strings.Contains(answer.Content, "not found") {
			answer, err = h.fetchPyPI(ctx, pkgName)
		}
	}

	if err != nil {
		return nil, err
	}

	return answer, nil
}

func (h *PkgHandler) parseQuery(query string) (registry, pkgName string) {
	lowerQuery := strings.ToLower(query)

	// Check for registry-specific patterns first
	if strings.HasPrefix(lowerQuery, "npm:") || strings.HasPrefix(lowerQuery, "npm ") {
		return "npm", strings.TrimSpace(query[4:])
	}
	if strings.HasPrefix(lowerQuery, "pypi:") || strings.HasPrefix(lowerQuery, "pypi ") {
		return "pypi", strings.TrimSpace(query[5:])
	}
	if strings.HasPrefix(lowerQuery, "pip:") || strings.HasPrefix(lowerQuery, "pip ") {
		return "pypi", strings.TrimSpace(query[4:])
	}
	if strings.HasPrefix(lowerQuery, "gopkg:") || strings.HasPrefix(lowerQuery, "gopkg ") {
		return "go", strings.TrimSpace(query[6:])
	}
	for _, prefix := range []struct{ text, registry string }{{"crates:", "crates"}, {"crates ", "crates"}, {"cargo:", "crates"}, {"cargo ", "crates"}, {"rubygems:", "rubygems"}, {"rubygems ", "rubygems"}, {"gem:", "rubygems"}, {"gem ", "rubygems"}} {
		if strings.HasPrefix(lowerQuery, prefix.text) {
			return prefix.registry, strings.TrimSpace(query[len(prefix.text):])
		}
	}

	// Check for pkg:registry:name format
	regPattern := regexp.MustCompile(`(?i)^pkg[:\s]+(npm|pypi|pip|go|crates|cargo|rubygems|gem)[:/]\s*(.+)$`)
	if matches := regPattern.FindStringSubmatch(query); len(matches) == 3 {
		registry := strings.ToLower(matches[1])
		if registry == "cargo" {
			registry = "crates"
		}
		if registry == "pip" {
			registry = "pypi"
		}
		if registry == "gem" {
			registry = "rubygems"
		}
		return registry, matches[2]
	}

	// Check for pkg:name format
	pkgPattern := regexp.MustCompile(`(?i)^pkg[:\s]+(.+)$`)
	if matches := pkgPattern.FindStringSubmatch(query); len(matches) == 2 {
		return "", matches[1]
	}

	return "", ""
}

func (h *PkgHandler) detectRegistry(pkgName string) string {
	// Go packages typically have domain-like paths
	if strings.Contains(pkgName, "/") && (strings.HasPrefix(pkgName, "github.com") ||
		strings.HasPrefix(pkgName, "golang.org") || strings.HasPrefix(pkgName, "go.") ||
		strings.Contains(pkgName, ".")) {
		return "go"
	}
	// Default to npm for single-word packages
	return "npm"
}

// fetchNPM fetches package info from npm registry
func (h *PkgHandler) fetchNPM(ctx context.Context, pkgName string) (*Answer, error) {
	apiURL := fmt.Sprintf("https://registry.npmjs.org/%s", url.PathEscape(pkgName))

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", version.BrowserUserAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := h.client.Do(req)
	if err != nil {
		return h.errorAnswer(pkgName, "npm", "Failed to connect to npm registry"), nil
	}
	defer closeutil.Close(resp.Body)

	if resp.StatusCode == http.StatusNotFound {
		return h.errorAnswer(pkgName, "npm", fmt.Sprintf("Package '%s' not found on npm", pkgName)), nil
	}

	if resp.StatusCode != http.StatusOK {
		return h.errorAnswer(pkgName, "npm", fmt.Sprintf("npm returned status %d", resp.StatusCode)), nil
	}

	var npmPkg NPMPackage
	if err := decodePackageJSON(resp.Body, &npmPkg); err != nil {
		return h.errorAnswer(pkgName, "npm", "Failed to parse npm response"), nil
	}

	// Get latest version info
	latestVersion := npmPkg.DistTags.Latest
	versionInfo, hasLatestVersion := npmPkg.Versions[latestVersion]
	if npmPkg.Name == "" || latestVersion == "" || !hasLatestVersion {
		return h.errorAnswer(pkgName, "npm", "Package metadata is incomplete"), nil
	}

	var content strings.Builder
	fmt.Fprintf(&content, "<strong>%s</strong> <code>v%s</code><br><br>", escapeHTML(npmPkg.Name), latestVersion)

	if npmPkg.Description != "" {
		fmt.Fprintf(&content, "<strong>Description:</strong> %s<br><br>", escapeHTML(npmPkg.Description))
	}

	content.WriteString("<strong>Install:</strong><br>")
	fmt.Fprintf(&content, "<code>npm install %s</code><br><br>", escapeHTML(pkgName))

	if npmPkg.License != "" {
		fmt.Fprintf(&content, "<strong>License:</strong> %s<br>", escapeHTML(npmPkg.License))
	}

	writePackageLink(&content, "Homepage", npmPkg.Homepage)

	if versionInfo.Repository.URL != "" {
		repoURL := cleanGitURL(versionInfo.Repository.URL)
		writePackageLink(&content, "Repository", repoURL)
	}

	if len(versionInfo.Keywords) > 0 {
		fmt.Fprintf(&content, "<strong>Keywords:</strong> %s<br>", escapeHTML(strings.Join(versionInfo.Keywords[:min(5, len(versionInfo.Keywords))], ", ")))
	}
	weeklyDownloads, hasWeeklyDownloads := h.weeklyDownloads(ctx, "npm", pkgName)
	if hasWeeklyDownloads {
		fmt.Fprintf(&content, "<strong>Weekly downloads:</strong> %d<br>", weeklyDownloads)
	} else {
		content.WriteString("<strong>Weekly downloads:</strong> unavailable<br>")
	}
	dependencyCount := len(versionInfo.Dependencies)
	fmt.Fprintf(&content, "<strong>Dependencies:</strong> %d<br>", dependencyCount)
	if published := npmPkg.Time.Modified; published != "" {
		fmt.Fprintf(&content, "<strong>Last updated:</strong> %s<br>", escapeHTML(published))
	}
	writePackagePreview(&content, npmPkg.Readme)

	return &Answer{
		Type:      AnswerTypePkg,
		Query:     fmt.Sprintf("pkg:%s", pkgName),
		Title:     fmt.Sprintf("npm: %s", npmPkg.Name),
		Content:   content.String(),
		Source:    "npm Registry",
		SourceURL: fmt.Sprintf("https://www.npmjs.com/package/%s", url.PathEscape(pkgName)),
		Data: map[string]interface{}{
			"name":             npmPkg.Name,
			"version":          latestVersion,
			"description":      npmPkg.Description,
			"weekly_downloads": optionalCount(weeklyDownloads, hasWeeklyDownloads),
			"dependencies":     dependencyCount,
			"updated_at":       npmPkg.Time.Modified,
			"readme_preview":   packagePreview(npmPkg.Readme),
			"registry":         "npm",
		},
	}, nil
}

// fetchPyPI fetches package info from PyPI
func (h *PkgHandler) fetchPyPI(ctx context.Context, pkgName string) (*Answer, error) {
	apiURL := fmt.Sprintf("https://pypi.org/pypi/%s/json", url.PathEscape(pkgName))

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", version.BrowserUserAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := h.client.Do(req)
	if err != nil {
		return h.errorAnswer(pkgName, "pypi", "Failed to connect to PyPI"), nil
	}
	defer closeutil.Close(resp.Body)

	if resp.StatusCode == http.StatusNotFound {
		return h.errorAnswer(pkgName, "pypi", fmt.Sprintf("Package '%s' not found on PyPI", pkgName)), nil
	}

	if resp.StatusCode != http.StatusOK {
		return h.errorAnswer(pkgName, "pypi", fmt.Sprintf("PyPI returned status %d", resp.StatusCode)), nil
	}

	var pypiPkg PyPIPackage
	if err := decodePackageJSON(resp.Body, &pypiPkg); err != nil {
		return h.errorAnswer(pkgName, "pypi", "Failed to parse PyPI response"), nil
	}

	info := pypiPkg.Info
	if info.Name == "" || info.Version == "" {
		return h.errorAnswer(pkgName, "pypi", "Package metadata is incomplete"), nil
	}

	var content strings.Builder
	fmt.Fprintf(&content, "<strong>%s</strong> <code>v%s</code><br><br>", escapeHTML(info.Name), info.Version)

	if info.Summary != "" {
		fmt.Fprintf(&content, "<strong>Description:</strong> %s<br><br>", escapeHTML(info.Summary))
	}

	content.WriteString("<strong>Install:</strong><br>")
	fmt.Fprintf(&content, "<code>pip install %s</code><br><br>", escapeHTML(pkgName))

	if info.License != "" {
		fmt.Fprintf(&content, "<strong>License:</strong> %s<br>", escapeHTML(info.License))
	}

	if info.Author != "" {
		fmt.Fprintf(&content, "<strong>Author:</strong> %s<br>", escapeHTML(info.Author))
	}

	if info.RequiresPython != "" {
		fmt.Fprintf(&content, "<strong>Python:</strong> %s<br>", escapeHTML(info.RequiresPython))
	}

	writePackageLink(&content, "Homepage", info.HomePage)
	writePackageLink(&content, "Project URL", info.ProjectURL)

	if len(info.Keywords) > 0 {
		fmt.Fprintf(&content, "<strong>Keywords:</strong> %s<br>", escapeHTML(info.Keywords))
	}
	weeklyDownloads, hasWeeklyDownloads := h.weeklyDownloads(ctx, "pypi", pkgName)
	if hasWeeklyDownloads {
		fmt.Fprintf(&content, "<strong>Weekly downloads:</strong> %d<br>", weeklyDownloads)
	} else {
		content.WriteString("<strong>Weekly downloads:</strong> unavailable<br>")
	}
	dependencyCount := len(info.RequiresDist)
	fmt.Fprintf(&content, "<strong>Dependencies:</strong> %d<br>", dependencyCount)
	updatedAt := pypiPkg.latestReleaseTime(info.Version)
	if updatedAt != "" {
		fmt.Fprintf(&content, "<strong>Last updated:</strong> %s<br>", escapeHTML(updatedAt))
	}
	writePackagePreview(&content, info.Description)

	return &Answer{
		Type:      AnswerTypePkg,
		Query:     fmt.Sprintf("pkg:%s", pkgName),
		Title:     fmt.Sprintf("PyPI: %s", info.Name),
		Content:   content.String(),
		Source:    "Python Package Index (PyPI)",
		SourceURL: fmt.Sprintf("https://pypi.org/project/%s/", url.PathEscape(pkgName)),
		Data: map[string]interface{}{
			"name":             info.Name,
			"version":          info.Version,
			"description":      info.Summary,
			"weekly_downloads": optionalCount(weeklyDownloads, hasWeeklyDownloads),
			"dependencies":     dependencyCount,
			"updated_at":       updatedAt,
			"readme_preview":   packagePreview(info.Description),
			"registry":         "pypi",
		},
	}, nil
}

func (h *PkgHandler) weeklyDownloads(ctx context.Context, registry, name string) (int64, bool) {
	key := registry + ":" + strings.ToLower(name)
	now := time.Now()
	h.cacheMu.Lock()
	if cached, ok := h.popularity[key]; ok && now.Before(cached.expiresAt) {
		h.cacheMu.Unlock()
		return cached.weeklyDownloads, cached.available
	}
	h.cacheMu.Unlock()

	var downloads int64
	var err error
	switch registry {
	case "npm":
		var response struct {
			Downloads *int64 `json:"downloads"`
		}
		err = h.getPackageJSON(ctx, "https://api.npmjs.org/downloads/point/last-week/"+url.PathEscape(name), &response)
		if err == nil && response.Downloads == nil {
			err = fmt.Errorf("npm downloads response is incomplete")
		}
		if response.Downloads != nil {
			downloads = *response.Downloads
		}
	case "pypi":
		var response struct {
			Data struct {
				LastWeek *int64 `json:"last_week"`
			} `json:"data"`
		}
		err = h.getPackageJSON(ctx, "https://pypistats.org/api/packages/"+url.PathEscape(name)+"/recent?period=week", &response)
		if err == nil && response.Data.LastWeek == nil {
			err = fmt.Errorf("PyPI stats response is incomplete")
		}
		if response.Data.LastWeek != nil {
			downloads = *response.Data.LastWeek
		}
	case "crates":
		var response struct {
			VersionDownloads *[]struct {
				Date      string `json:"date"`
				Downloads int64  `json:"downloads"`
			} `json:"version_downloads"`
		}
		err = h.getPackageJSON(ctx, "https://crates.io/api/v1/crates/"+url.PathEscape(name)+"/downloads", &response)
		if err == nil {
			if response.VersionDownloads == nil {
				err = fmt.Errorf("crates.io download response is incomplete")
			}
		}
		if err == nil {
			cutoff := time.Now().UTC().AddDate(0, 0, -6).Format("2006-01-02")
			for _, item := range *response.VersionDownloads {
				if item.Date >= cutoff {
					downloads += item.Downloads
				}
			}
		}
	}
	cacheTTL := 24 * time.Hour
	if err != nil {
		cacheTTL = 5 * time.Minute
	}
	result := packagePopularity{weeklyDownloads: downloads, available: err == nil, expiresAt: now.Add(cacheTTL)}
	h.cacheMu.Lock()
	if h.popularity == nil {
		h.popularity = make(map[string]packagePopularity)
	}
	if _, exists := h.popularity[key]; !exists && len(h.popularity) >= 500 {
		oldestKey := ""
		var oldestExpiry time.Time
		for cachedKey, cachedValue := range h.popularity {
			if oldestKey == "" || cachedValue.expiresAt.Before(oldestExpiry) {
				oldestKey, oldestExpiry = cachedKey, cachedValue.expiresAt
			}
		}
		delete(h.popularity, oldestKey)
	}
	h.popularity[key] = result
	h.cacheMu.Unlock()
	return result.weeklyDownloads, result.available
}

func optionalCount(value int64, available bool) interface{} {
	if !available {
		return nil
	}
	return value
}

func packagePreview(raw string) string {
	raw = strings.TrimSpace(raw)
	runes := []rune(raw)
	if len(runes) > 400 {
		raw = string(runes[:400]) + "…"
	}
	return raw
}

func writePackagePreview(content *strings.Builder, raw string) {
	preview := packagePreview(raw)
	if preview == "" {
		content.WriteString("<strong>README preview:</strong> unavailable<br>")
		return
	}
	fmt.Fprintf(content, "<strong>README preview:</strong><br><pre>%s</pre>", escapeHTML(preview))
}

// fetchGoPkg loads public module metadata through the Go module proxy.
func (h *PkgHandler) fetchGoPkg(ctx context.Context, pkgName string) (*Answer, error) {
	pkgName = strings.TrimPrefix(pkgName, "https://")
	pkgName = strings.TrimPrefix(pkgName, "http://")
	escapedPath, err := module.EscapePath(pkgName)
	if err != nil {
		return h.errorAnswer(pkgName, "Go Modules", "Invalid Go module path"), nil
	}
	latestVersion, updatedAt := h.getGoModuleDetails(ctx, escapedPath)
	dependencyCount := -1
	license := ""
	readme := ""
	if latestVersion != "" {
		escapedVersion, escapeErr := module.EscapeVersion(latestVersion)
		if escapeErr == nil {
			if moduleFile, fetchErr := h.getPackageBytes(ctx, "https://proxy.golang.org/"+escapedPath+"/@v/"+escapedVersion+".mod", 1<<20); fetchErr == nil {
				if parsed, parseErr := modfile.ParseLax("go.mod", moduleFile, nil); parseErr == nil {
					dependencyCount = 0
					for _, requirement := range parsed.Require {
						if !requirement.Indirect {
							dependencyCount++
						}
					}
				}
			}
			if archive, fetchErr := h.getPackageBytes(ctx, "https://proxy.golang.org/"+escapedPath+"/@v/"+escapedVersion+".zip", 10<<20); fetchErr == nil {
				readme, license = extractGoModuleDocs(archive, escapedPath+"@"+escapedVersion+"/")
			}
		}
	}

	var content strings.Builder
	fmt.Fprintf(&content, "<strong>%s</strong><br><br>", escapeHTML(pkgName))

	content.WriteString("<strong>Install:</strong><br>")
	fmt.Fprintf(&content, "<code>go get %s</code><br><br>", escapeHTML(pkgName))

	content.WriteString("<strong>Import:</strong><br>")
	fmt.Fprintf(&content, "<code>import \"%s\"</code><br><br>", escapeHTML(pkgName))

	if latestVersion != "" {
		fmt.Fprintf(&content, "<strong>Latest Version:</strong> %s<br><br>", escapeHTML(latestVersion))
	}
	content.WriteString("<strong>Weekly downloads:</strong> unavailable from the Go module proxy<br>")
	if dependencyCount < 0 {
		content.WriteString("<strong>Dependencies:</strong> unavailable<br>")
	} else {
		fmt.Fprintf(&content, "<strong>Dependencies:</strong> %d direct<br>", dependencyCount)
	}
	if license != "" {
		fmt.Fprintf(&content, "<strong>License:</strong> %s<br>", escapeHTML(license))
	} else {
		content.WriteString("<strong>License:</strong> unavailable<br>")
	}
	if updatedAt != "" {
		fmt.Fprintf(&content, "<strong>Last updated:</strong> %s<br>", escapeHTML(updatedAt))
	} else {
		content.WriteString("<strong>Last updated:</strong> unavailable<br>")
	}
	writePackagePreview(&content, readme)

	content.WriteString("<strong>Links:</strong><br>")
	fmt.Fprintf(&content, "&bull; <a href=\"https://pkg.go.dev/%s\" target=\"_blank\">pkg.go.dev</a><br>", url.PathEscape(pkgName))

	// Add source link if it looks like a GitHub package
	if strings.HasPrefix(pkgName, "github.com/") {
		writePackageLink(&content, "Source Code", "https://"+pkgName)
	}

	return &Answer{
		Type:      AnswerTypePkg,
		Query:     fmt.Sprintf("pkg:%s", pkgName),
		Title:     fmt.Sprintf("Go: %s", pkgName),
		Content:   content.String(),
		Source:    "pkg.go.dev",
		SourceURL: fmt.Sprintf("https://pkg.go.dev/%s", url.PathEscape(pkgName)),
		Data: map[string]interface{}{
			"name":             pkgName,
			"version":          latestVersion,
			"weekly_downloads": nil,
			"dependencies":     optionalDependencyCount(dependencyCount),
			"license":          license,
			"updated_at":       updatedAt,
			"readme_preview":   packagePreview(readme),
			"registry":         "go",
		},
	}, nil
}

func (h *PkgHandler) getGoModuleDetails(ctx context.Context, escapedPath string) (string, string) {
	var modInfo struct {
		Version string `json:"Version"`
		Time    string `json:"Time"`
	}
	if err := h.getPackageJSON(ctx, "https://proxy.golang.org/"+escapedPath+"/@latest", &modInfo); err != nil {
		return "", ""
	}
	return modInfo.Version, modInfo.Time
}

func (h *PkgHandler) getPackageBytes(ctx context.Context, endpoint string, maxBytes int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	return h.getPackageResponse(req, maxBytes)
}

func (h *PkgHandler) getPackageResponse(req *http.Request, maxBytes int64) ([]byte, error) {
	req.Header.Set("User-Agent", version.BrowserUserAgent)
	resp, err := h.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer closeutil.Close(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("registry returned HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("registry response exceeds %d bytes", maxBytes)
	}
	return data, nil
}

func extractGoModuleDocs(archive []byte, root string) (readme, license string) {
	reader, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		return "", ""
	}
	for _, file := range reader.File {
		if !strings.HasPrefix(file.Name, root) || strings.Contains(strings.TrimPrefix(file.Name, root), "/") {
			continue
		}
		base := strings.ToLower(strings.TrimPrefix(file.Name, root))
		if base != "readme" && base != "readme.md" && base != "readme.txt" && base != "license" && base != "license.md" && base != "license.txt" {
			continue
		}
		if file.UncompressedSize64 > 1<<20 {
			continue
		}
		body, openErr := file.Open()
		if openErr != nil {
			continue
		}
		data, readErr := io.ReadAll(io.LimitReader(body, (1<<20)+1))
		_ = body.Close()
		if readErr != nil || len(data) > 1<<20 {
			continue
		}
		if strings.HasPrefix(base, "readme") && readme == "" {
			readme = string(data)
		}
		if strings.HasPrefix(base, "license") && license == "" {
			license = strings.TrimSpace(string(data))
		}
	}
	return readme, license
}

func optionalDependencyCount(value int) interface{} {
	if value < 0 {
		return nil
	}
	return value
}

func (h *PkgHandler) errorAnswer(pkgName, registry, message string) *Answer {
	return &Answer{
		Type:    AnswerTypePkg,
		Query:   fmt.Sprintf("pkg:%s", pkgName),
		Title:   fmt.Sprintf("Package Lookup: %s", pkgName),
		Content: fmt.Sprintf("<span class=\"error\">%s</span>", message),
		Source:  registry,
	}
}

func (h *PkgHandler) fetchCrate(ctx context.Context, name string) (*Answer, error) {
	var response struct {
		Crate struct {
			Name            string `json:"name"`
			Description     string `json:"description"`
			Version         string `json:"max_version"`
			License         string `json:"license"`
			Updated         string `json:"updated_at"`
			Repository      string `json:"repository"`
			Downloads       int64  `json:"downloads"`
			RecentDownloads int64  `json:"recent_downloads"`
		} `json:"crate"`
	}
	if err := h.getPackageJSON(ctx, "https://crates.io/api/v1/crates/"+url.PathEscape(name), &response); err != nil {
		return h.packageFetchError(name, "crates.io", err), nil
	}
	c := response.Crate
	if c.Name == "" {
		return h.errorAnswer(name, "crates.io", "Package metadata is incomplete"), nil
	}
	var deps struct {
		Dependencies []json.RawMessage `json:"dependencies"`
	}
	dependencyCount := -1
	if err := h.getPackageJSON(ctx, "https://crates.io/api/v1/crates/"+url.PathEscape(name)+"/"+url.PathEscape(c.Version)+"/dependencies", &deps); err == nil {
		dependencyCount = len(deps.Dependencies)
	}
	weeklyDownloads, hasWeeklyDownloads := h.weeklyDownloads(ctx, "crates", name)
	readme := ""
	if c.Version != "" {
		readmeURL := "https://crates.io/api/v1/crates/" + url.PathEscape(name) + "/" + url.PathEscape(c.Version) + "/readme"
		if body, err := h.getPackageBytes(ctx, readmeURL, 2<<20); err == nil {
			readme = httputil.HTML2TextConverter(string(body), 80)
		}
	}
	var content strings.Builder
	fmt.Fprintf(&content, "<strong>%s</strong> <code>v%s</code><br><br>", escapeHTML(c.Name), escapeHTML(c.Version))
	if c.Description != "" {
		fmt.Fprintf(&content, "<strong>Description:</strong> %s<br><br>", escapeHTML(c.Description))
	}
	fmt.Fprintf(&content, "<strong>Install:</strong><br><code>cargo add %s</code><br><br>", escapeHTML(c.Name))
	if c.License != "" {
		fmt.Fprintf(&content, "<strong>License:</strong> %s<br>", escapeHTML(c.License))
	}
	fmt.Fprintf(&content, "<strong>Downloads:</strong> %d total, %d in the last 90 days<br>", c.Downloads, c.RecentDownloads)
	if c.Updated != "" {
		fmt.Fprintf(&content, "<strong>Last updated:</strong> %s<br>", escapeHTML(c.Updated))
	}
	if dependencyCount < 0 {
		content.WriteString("<strong>Dependencies:</strong> unavailable<br>")
	} else {
		fmt.Fprintf(&content, "<strong>Dependencies:</strong> %d<br>", dependencyCount)
	}
	writePackageLink(&content, "Repository", c.Repository)
	if hasWeeklyDownloads {
		fmt.Fprintf(&content, "<strong>Weekly downloads:</strong> %d<br>", weeklyDownloads)
	} else {
		content.WriteString("<strong>Weekly downloads:</strong> unavailable<br>")
	}
	writePackagePreview(&content, readme)
	return &Answer{Type: AnswerTypePkg, Query: "pkg:crates/" + name, Title: "crates.io: " + c.Name, Content: content.String(), Source: "crates.io", SourceURL: "https://crates.io/crates/" + url.PathEscape(c.Name), Data: map[string]interface{}{"name": c.Name, "version": c.Version, "description": c.Description, "license": c.License, "downloads": c.Downloads, "recent_downloads": c.RecentDownloads, "weekly_downloads": optionalCount(weeklyDownloads, hasWeeklyDownloads), "dependencies": dependencyCount, "updated_at": c.Updated, "repository": c.Repository, "readme_preview": packagePreview(readme), "registry": "crates.io"}}, nil
}

func (h *PkgHandler) fetchRubyGem(ctx context.Context, name string) (*Answer, error) {
	var gem struct {
		Name         string   `json:"name"`
		Info         string   `json:"info"`
		Version      string   `json:"version"`
		Licenses     []string `json:"licenses"`
		Project      string   `json:"project_uri"`
		Repository   string   `json:"source_code_uri"`
		Downloads    int64    `json:"downloads"`
		Updated      string   `json:"version_created_at"`
		GemURI       string   `json:"gem_uri"`
		Dependencies struct {
			Runtime []json.RawMessage `json:"runtime"`
		} `json:"dependencies"`
	}
	if err := h.getPackageJSON(ctx, "https://rubygems.org/api/v1/gems/"+url.PathEscape(name)+".json", &gem); err != nil {
		return h.packageFetchError(name, "RubyGems", err), nil
	}
	if gem.Name == "" {
		return h.errorAnswer(name, "RubyGems", "Package metadata is incomplete"), nil
	}
	var content strings.Builder
	fmt.Fprintf(&content, "<strong>%s</strong> <code>v%s</code><br><br>", escapeHTML(gem.Name), escapeHTML(gem.Version))
	if gem.Info != "" {
		fmt.Fprintf(&content, "<strong>Description:</strong> %s<br><br>", escapeHTML(gem.Info))
	}
	fmt.Fprintf(&content, "<strong>Install:</strong><br><code>gem install %s</code><br><br>", escapeHTML(gem.Name))
	if len(gem.Licenses) > 0 {
		fmt.Fprintf(&content, "<strong>License:</strong> %s<br>", escapeHTML(strings.Join(gem.Licenses, ", ")))
	}
	fmt.Fprintf(&content, "<strong>Downloads:</strong> %d total<br><strong>Weekly downloads:</strong> unavailable from the registry API<br><strong>Dependencies:</strong> %d runtime<br>", gem.Downloads, len(gem.Dependencies.Runtime))
	if gem.Updated != "" {
		fmt.Fprintf(&content, "<strong>Last updated:</strong> %s<br>", escapeHTML(gem.Updated))
	}
	writePackageLink(&content, "Homepage", gem.Project)
	writePackageLink(&content, "Repository", gem.Repository)
	readme := h.getRubyGemReadme(ctx, name, gem.Version, gem.GemURI)
	writePackagePreview(&content, readme)
	return &Answer{Type: AnswerTypePkg, Query: "pkg:rubygems/" + name, Title: "RubyGems: " + gem.Name, Content: content.String(), Source: "RubyGems", SourceURL: "https://rubygems.org/gems/" + url.PathEscape(gem.Name), Data: map[string]interface{}{"name": gem.Name, "version": gem.Version, "description": gem.Info, "license": gem.Licenses, "downloads": gem.Downloads, "weekly_downloads": nil, "dependencies": len(gem.Dependencies.Runtime), "updated_at": gem.Updated, "repository": gem.Repository, "readme_preview": packagePreview(readme), "registry": "rubygems"}}, nil
}

func (h *PkgHandler) getRubyGemReadme(ctx context.Context, name, version, gemURI string) string {
	key := strings.ToLower(name) + ":" + version
	now := time.Now()
	h.readmeMu.Lock()
	if cached, ok := h.readmes[key]; ok && now.Before(cached.expiresAt) {
		h.readmeMu.Unlock()
		return cached.text
	}
	h.readmeMu.Unlock()
	parsed, err := url.Parse(gemURI)
	var preview string
	if err == nil && parsed.Scheme == "https" && parsed.Host == "rubygems.org" && parsed.User == nil {
		if packageFile, fetchErr := h.getPackageBytes(ctx, parsed.String(), 15<<20); fetchErr == nil {
			preview = extractRubyGemReadme(packageFile)
		}
	}
	ttl := 24 * time.Hour
	if preview == "" {
		ttl = 5 * time.Minute
	}
	h.readmeMu.Lock()
	if h.readmes == nil {
		h.readmes = make(map[string]cachedPackageReadme)
	}
	if _, exists := h.readmes[key]; !exists && len(h.readmes) >= 500 {
		oldestKey := ""
		var oldestExpiry time.Time
		for cacheKey, value := range h.readmes {
			if oldestKey == "" || value.expiresAt.Before(oldestExpiry) {
				oldestKey, oldestExpiry = cacheKey, value.expiresAt
			}
		}
		delete(h.readmes, oldestKey)
	}
	h.readmes[key] = cachedPackageReadme{text: preview, expiresAt: now.Add(ttl)}
	h.readmeMu.Unlock()
	return preview
}

func extractRubyGemReadme(packageFile []byte) string {
	outer := tar.NewReader(bytes.NewReader(packageFile))
	var dataArchive []byte
	for {
		header, err := outer.Next()
		if err != nil {
			break
		}
		if header.Name != "data.tar.gz" || header.Size < 0 || header.Size > 15<<20 {
			continue
		}
		dataArchive, err = io.ReadAll(io.LimitReader(outer, (15<<20)+1))
		if err != nil || len(dataArchive) > 15<<20 {
			return ""
		}
		break
	}
	if len(dataArchive) == 0 {
		return ""
	}
	compressed, err := gzip.NewReader(bytes.NewReader(dataArchive))
	if err != nil {
		return ""
	}
	defer closeutil.Close(compressed)
	data, err := io.ReadAll(io.LimitReader(compressed, (30<<20)+1))
	if err != nil || len(data) > 30<<20 {
		return ""
	}
	inner := tar.NewReader(bytes.NewReader(data))
	var fallback string
	for {
		header, err := inner.Next()
		if err != nil {
			break
		}
		if header.Typeflag != tar.TypeReg {
			continue
		}
		base := strings.ToLower(path.Base(header.Name))
		if base != "readme" && base != "readme.md" && base != "readme.rdoc" && base != "readme.txt" {
			continue
		}
		if header.Size < 0 || header.Size > 1<<20 {
			continue
		}
		body, err := io.ReadAll(io.LimitReader(inner, (1<<20)+1))
		if err != nil || len(body) > 1<<20 {
			continue
		}
		if !strings.Contains(header.Name, "/") {
			return string(body)
		}
		if fallback == "" {
			fallback = string(body)
		}
	}
	return fallback
}

func (h *PkgHandler) getPackageJSON(ctx context.Context, endpoint string, target interface{}) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", version.BrowserUserAgent)
	req.Header.Set("Accept", "application/json")
	resp, err := h.client.Do(req)
	if err != nil {
		return err
	}
	defer closeutil.Close(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("registry returned HTTP %d", resp.StatusCode)
	}
	return decodePackageJSON(resp.Body, target)
}

func (h *PkgHandler) packageFetchError(name, registry string, err error) *Answer {
	if strings.Contains(err.Error(), "HTTP 404") {
		return h.errorAnswer(name, registry, fmt.Sprintf("Package '%s' not found on %s", name, registry))
	}
	return h.errorAnswer(name, registry, "Package metadata is temporarily unavailable")
}

func decodePackageJSON(body io.Reader, target interface{}) error {
	data, err := io.ReadAll(io.LimitReader(body, maxPackageMetadataBytes+1))
	if err != nil {
		return err
	}
	if len(data) > maxPackageMetadataBytes {
		return fmt.Errorf("package metadata exceeds %d bytes", maxPackageMetadataBytes)
	}
	return json.Unmarshal(data, target)
}

func writePackageLink(content *strings.Builder, label, raw string) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Host == "" || parsed.User != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") {
		return
	}
	link := parsed.String()
	fmt.Fprintf(content, "<strong>%s:</strong> <a href=\"%s\" target=\"_blank\" rel=\"noopener noreferrer\">%s</a><br>", escapeHTML(label), escapeHTML(link), escapeHTML(truncateString(link, 50)))
}

// NPM response structures
type NPMPackage struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	License     string `json:"license"`
	Homepage    string `json:"homepage"`
	Readme      string `json:"readme"`
	Time        struct {
		Modified string `json:"modified"`
	} `json:"time"`
	DistTags struct {
		Latest string `json:"latest"`
	} `json:"dist-tags"`
	Versions map[string]NPMVersionInfo `json:"versions"`
}

type NPMVersionInfo struct {
	Name         string                     `json:"name"`
	Version      string                     `json:"version"`
	Description  string                     `json:"description"`
	Keywords     []string                   `json:"keywords"`
	Dependencies map[string]json.RawMessage `json:"dependencies"`
	Repository   struct {
		Type string `json:"type"`
		URL  string `json:"url"`
	} `json:"repository"`
}

// PyPI response structures
type PyPIPackage struct {
	Releases map[string][]struct {
		UploadTime string `json:"upload_time_iso_8601"`
	} `json:"releases"`
	Info struct {
		Name           string   `json:"name"`
		Version        string   `json:"version"`
		Summary        string   `json:"summary"`
		Description    string   `json:"description"`
		License        string   `json:"license"`
		Author         string   `json:"author"`
		AuthorEmail    string   `json:"author_email"`
		HomePage       string   `json:"home_page"`
		ProjectURL     string   `json:"project_url"`
		RequiresPython string   `json:"requires_python"`
		Keywords       string   `json:"keywords"`
		RequiresDist   []string `json:"requires_dist"`
	} `json:"info"`
}

func (p PyPIPackage) latestReleaseTime(version string) string {
	files := p.Releases[version]
	latest := ""
	for _, file := range files {
		if file.UploadTime > latest {
			latest = file.UploadTime
		}
	}
	return latest
}

func cleanGitURL(gitURL string) string {
	// Convert git+https://github.com/... to https://github.com/...
	gitURL = strings.TrimPrefix(gitURL, "git+")
	gitURL = strings.TrimSuffix(gitURL, ".git")
	return gitURL
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
