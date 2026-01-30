package instant

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/apimgr/search/src/version"
)

// AnswerTypePkg is the answer type for package lookups
const AnswerTypePkg AnswerType = "pkg"

// PkgHandler handles package information lookups from npm, PyPI, and pkg.go.dev
type PkgHandler struct {
	client   *http.Client
	patterns []*regexp.Regexp
}

// NewPkgHandler creates a new package handler
func NewPkgHandler() *PkgHandler {
	return &PkgHandler{
		client: &http.Client{Timeout: 15 * time.Second},
		patterns: []*regexp.Regexp{
			// pkg:registry:name format
			regexp.MustCompile(`(?i)^pkg[:\s]+(npm|pypi|go)[:\s]+(.+)$`),
			// pkg:name (auto-detect)
			regexp.MustCompile(`(?i)^pkg[:\s]+(.+)$`),
			// registry-specific shortcuts
			regexp.MustCompile(`(?i)^npm[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^pypi[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^gopkg[:\s]+(.+)$`),
		},
	}
}

func (h *PkgHandler) Name() string              { return "pkg" }
func (h *PkgHandler) Patterns() []*regexp.Regexp { return h.patterns }

func (h *PkgHandler) CanHandle(query string) bool {
	for _, p := range h.patterns {
		if p.MatchString(query) {
			return true
		}
	}
	return false
}

func (h *PkgHandler) Handle(ctx context.Context, query string) (*Answer, error) {
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
	if strings.HasPrefix(lowerQuery, "gopkg:") || strings.HasPrefix(lowerQuery, "gopkg ") {
		return "go", strings.TrimSpace(query[6:])
	}

	// Check for pkg:registry:name format
	regPattern := regexp.MustCompile(`(?i)^pkg[:\s]+(npm|pypi|go)[:\s]+(.+)$`)
	if matches := regPattern.FindStringSubmatch(query); len(matches) == 3 {
		return strings.ToLower(matches[1]), matches[2]
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
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return h.errorAnswer(pkgName, "npm", fmt.Sprintf("Package '%s' not found on npm", pkgName)), nil
	}

	if resp.StatusCode != http.StatusOK {
		return h.errorAnswer(pkgName, "npm", fmt.Sprintf("npm returned status %d", resp.StatusCode)), nil
	}

	var npmPkg NPMPackage
	if err := json.NewDecoder(resp.Body).Decode(&npmPkg); err != nil {
		return h.errorAnswer(pkgName, "npm", "Failed to parse npm response"), nil
	}

	// Get latest version info
	latestVersion := npmPkg.DistTags.Latest
	versionInfo := npmPkg.Versions[latestVersion]

	var content strings.Builder
	content.WriteString(fmt.Sprintf("<strong>%s</strong> <code>v%s</code><br><br>", escapeHTML(npmPkg.Name), latestVersion))

	if npmPkg.Description != "" {
		content.WriteString(fmt.Sprintf("<strong>Description:</strong> %s<br><br>", escapeHTML(npmPkg.Description)))
	}

	content.WriteString("<strong>Install:</strong><br>")
	content.WriteString(fmt.Sprintf("<code>npm install %s</code><br><br>", pkgName))

	if npmPkg.License != "" {
		content.WriteString(fmt.Sprintf("<strong>License:</strong> %s<br>", escapeHTML(npmPkg.License)))
	}

	if npmPkg.Homepage != "" {
		content.WriteString(fmt.Sprintf("<strong>Homepage:</strong> <a href=\"%s\" target=\"_blank\">%s</a><br>", npmPkg.Homepage, truncateString(npmPkg.Homepage, 50)))
	}

	if versionInfo.Repository.URL != "" {
		repoURL := cleanGitURL(versionInfo.Repository.URL)
		content.WriteString(fmt.Sprintf("<strong>Repository:</strong> <a href=\"%s\" target=\"_blank\">%s</a><br>", repoURL, truncateString(repoURL, 50)))
	}

	if len(versionInfo.Keywords) > 0 {
		content.WriteString(fmt.Sprintf("<strong>Keywords:</strong> %s<br>", escapeHTML(strings.Join(versionInfo.Keywords[:min(5, len(versionInfo.Keywords))], ", "))))
	}

	return &Answer{
		Type:      AnswerTypePkg,
		Query:     fmt.Sprintf("pkg:%s", pkgName),
		Title:     fmt.Sprintf("npm: %s", npmPkg.Name),
		Content:   content.String(),
		Source:    "npm Registry",
		SourceURL: fmt.Sprintf("https://www.npmjs.com/package/%s", url.PathEscape(pkgName)),
		Data: map[string]interface{}{
			"name":        npmPkg.Name,
			"version":     latestVersion,
			"description": npmPkg.Description,
			"registry":    "npm",
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
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return h.errorAnswer(pkgName, "pypi", fmt.Sprintf("Package '%s' not found on PyPI", pkgName)), nil
	}

	if resp.StatusCode != http.StatusOK {
		return h.errorAnswer(pkgName, "pypi", fmt.Sprintf("PyPI returned status %d", resp.StatusCode)), nil
	}

	var pypiPkg PyPIPackage
	if err := json.NewDecoder(resp.Body).Decode(&pypiPkg); err != nil {
		return h.errorAnswer(pkgName, "pypi", "Failed to parse PyPI response"), nil
	}

	info := pypiPkg.Info

	var content strings.Builder
	content.WriteString(fmt.Sprintf("<strong>%s</strong> <code>v%s</code><br><br>", escapeHTML(info.Name), info.Version))

	if info.Summary != "" {
		content.WriteString(fmt.Sprintf("<strong>Description:</strong> %s<br><br>", escapeHTML(info.Summary)))
	}

	content.WriteString("<strong>Install:</strong><br>")
	content.WriteString(fmt.Sprintf("<code>pip install %s</code><br><br>", pkgName))

	if info.License != "" {
		content.WriteString(fmt.Sprintf("<strong>License:</strong> %s<br>", escapeHTML(info.License)))
	}

	if info.Author != "" {
		content.WriteString(fmt.Sprintf("<strong>Author:</strong> %s<br>", escapeHTML(info.Author)))
	}

	if info.RequiresPython != "" {
		content.WriteString(fmt.Sprintf("<strong>Python:</strong> %s<br>", escapeHTML(info.RequiresPython)))
	}

	if info.HomePage != "" {
		content.WriteString(fmt.Sprintf("<strong>Homepage:</strong> <a href=\"%s\" target=\"_blank\">%s</a><br>", info.HomePage, truncateString(info.HomePage, 50)))
	}

	if info.ProjectURL != "" {
		content.WriteString(fmt.Sprintf("<strong>Project URL:</strong> <a href=\"%s\" target=\"_blank\">%s</a><br>", info.ProjectURL, truncateString(info.ProjectURL, 50)))
	}

	if len(info.Keywords) > 0 {
		content.WriteString(fmt.Sprintf("<strong>Keywords:</strong> %s<br>", escapeHTML(info.Keywords)))
	}

	return &Answer{
		Type:      AnswerTypePkg,
		Query:     fmt.Sprintf("pkg:%s", pkgName),
		Title:     fmt.Sprintf("PyPI: %s", info.Name),
		Content:   content.String(),
		Source:    "Python Package Index (PyPI)",
		SourceURL: fmt.Sprintf("https://pypi.org/project/%s/", url.PathEscape(pkgName)),
		Data: map[string]interface{}{
			"name":        info.Name,
			"version":     info.Version,
			"description": info.Summary,
			"registry":    "pypi",
		},
	}, nil
}

// fetchGoPkg fetches package info from pkg.go.dev
func (h *PkgHandler) fetchGoPkg(ctx context.Context, pkgName string) (*Answer, error) {
	// pkg.go.dev doesn't have a public JSON API, so we provide a link-based response
	// We can try to get basic info from the proxy API

	// Clean up package name
	pkgName = strings.TrimPrefix(pkgName, "https://")
	pkgName = strings.TrimPrefix(pkgName, "http://")

	var content strings.Builder
	content.WriteString(fmt.Sprintf("<strong>%s</strong><br><br>", escapeHTML(pkgName)))

	content.WriteString("<strong>Install:</strong><br>")
	content.WriteString(fmt.Sprintf("<code>go get %s</code><br><br>", pkgName))

	content.WriteString("<strong>Import:</strong><br>")
	content.WriteString(fmt.Sprintf("<code>import \"%s\"</code><br><br>", pkgName))

	// Try to get version info from Go proxy
	latestVersion := h.getGoModuleVersion(ctx, pkgName)
	if latestVersion != "" {
		content.WriteString(fmt.Sprintf("<strong>Latest Version:</strong> %s<br><br>", latestVersion))
	}

	content.WriteString("<strong>Links:</strong><br>")
	content.WriteString(fmt.Sprintf("&bull; <a href=\"https://pkg.go.dev/%s\" target=\"_blank\">pkg.go.dev</a><br>", url.PathEscape(pkgName)))

	// Add source link if it looks like a GitHub package
	if strings.HasPrefix(pkgName, "github.com/") {
		content.WriteString(fmt.Sprintf("&bull; <a href=\"https://%s\" target=\"_blank\">Source Code</a><br>", pkgName))
	}

	return &Answer{
		Type:      AnswerTypePkg,
		Query:     fmt.Sprintf("pkg:%s", pkgName),
		Title:     fmt.Sprintf("Go: %s", pkgName),
		Content:   content.String(),
		Source:    "pkg.go.dev",
		SourceURL: fmt.Sprintf("https://pkg.go.dev/%s", url.PathEscape(pkgName)),
		Data: map[string]interface{}{
			"name":     pkgName,
			"version":  latestVersion,
			"registry": "go",
		},
	}, nil
}

func (h *PkgHandler) getGoModuleVersion(ctx context.Context, modPath string) string {
	// Use Go module proxy to get latest version
	apiURL := fmt.Sprintf("https://proxy.golang.org/%s/@latest", url.PathEscape(modPath))

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("User-Agent", version.BrowserUserAgent)

	resp, err := h.client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		return ""
	}
	defer resp.Body.Close()

	var modInfo struct {
		Version string `json:"Version"`
		Time    string `json:"Time"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&modInfo); err != nil {
		return ""
	}

	return modInfo.Version
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

// NPM response structures
type NPMPackage struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	License     string `json:"license"`
	Homepage    string `json:"homepage"`
	DistTags    struct {
		Latest string `json:"latest"`
	} `json:"dist-tags"`
	Versions map[string]NPMVersionInfo `json:"versions"`
}

type NPMVersionInfo struct {
	Name        string   `json:"name"`
	Version     string   `json:"version"`
	Description string   `json:"description"`
	Keywords    []string `json:"keywords"`
	Repository  struct {
		Type string `json:"type"`
		URL  string `json:"url"`
	} `json:"repository"`
}

// PyPI response structures
type PyPIPackage struct {
	Info struct {
		Name           string `json:"name"`
		Version        string `json:"version"`
		Summary        string `json:"summary"`
		Description    string `json:"description"`
		License        string `json:"license"`
		Author         string `json:"author"`
		AuthorEmail    string `json:"author_email"`
		HomePage       string `json:"home_page"`
		ProjectURL     string `json:"project_url"`
		RequiresPython string `json:"requires_python"`
		Keywords       string `json:"keywords"`
	} `json:"info"`
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
