// Package version provides build info and version strings
// Per AI.md PART 7: Common Go Modules - version/version.go
package version

import (
	"fmt"
	"runtime"
	"strings"
)

// APIVersion is the current API version string. All API routes use this.
// Never hardcode "v1" directly in code — always reference APIVersion or APIPrefix.
const APIVersion = "v1"

// APIPrefix is the versioned API base path (e.g., "/api/v1").
const APIPrefix = "/api/" + APIVersion

// BrowserUserAgent is the standard User-Agent string for browser-like HTTP requests.
// Windows 11 Edge - consistent across all engines for privacy and compatibility.
const BrowserUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36 Edg/131.0.0.0"

// Label constants for the Full() output format — used in tests to avoid hardcoded strings.
const (
	LabelVersion   = "Version:"
	LabelCommit    = "Commit:"
	LabelBuildDate = "Build Date:"
	LabelBuilt     = "Built:"
	LabelBranch    = "Branch:"
	LabelGoVersion = "Go Version:"
	LabelGo        = "Go:"
	LabelOSArch    = "OS/Arch:"
	LabelCompiler  = "Compiler:"
)

// Build-time variables - set via ldflags
var (
	// Version is the semantic version (e.g., "1.0.0")
	Version = "dev"

	// Commit is the git commit hash
	Commit = "unknown"

	// BuildDate is the build timestamp
	BuildDate = "unknown"

	// Branch is the git branch name
	Branch = "unknown"

	// GoVersion is set at build time
	GoVersion = runtime.Version()
)

// Info contains all version information
type Info struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildDate string `json:"build_date"`
	Branch    string `json:"branch"`
	GoVersion string `json:"go_version"`
	OS        string `json:"os"`
	Arch      string `json:"arch"`
	Compiler  string `json:"compiler"`
}

// Get returns the current version info
func Get() Info {
	return Info{
		Version:   Version,
		Commit:    Commit,
		BuildDate: BuildDate,
		Branch:    Branch,
		GoVersion: GoVersion,
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
		Compiler:  runtime.Compiler,
	}
}

// String returns a formatted version string
func (i Info) String() string {
	return fmt.Sprintf("%s (%s/%s)", i.Version, i.OS, i.Arch)
}

// Full returns a detailed version string
func (i Info) Full() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%s    %s\n", LabelVersion, i.Version))
	sb.WriteString(fmt.Sprintf("%s     %s\n", LabelCommit, i.Commit))
	sb.WriteString(fmt.Sprintf("%s %s\n", LabelBuildDate, i.BuildDate))
	sb.WriteString(fmt.Sprintf("%s     %s\n", LabelBranch, i.Branch))
	sb.WriteString(fmt.Sprintf("%s %s\n", LabelGoVersion, i.GoVersion))
	sb.WriteString(fmt.Sprintf("%s    %s/%s\n", LabelOSArch, i.OS, i.Arch))
	sb.WriteString(fmt.Sprintf("%s   %s", LabelCompiler, i.Compiler))
	return sb.String()
}

// Short returns just the version number
func (i Info) Short() string {
	return i.Version
}

// UserAgent returns a User-Agent string for HTTP clients
func (i Info) UserAgent(binaryName string) string {
	return fmt.Sprintf("%s/%s", binaryName, i.Version)
}

// GetShort returns just the version string
func GetShort() string {
	return Version
}

// GetCommitShort returns the first 7 characters of the commit hash
func GetCommitShort() string {
	if len(Commit) >= 7 {
		return Commit[:7]
	}
	return Commit
}

// IsDev returns true if this is a development build
func IsDev() bool {
	return Version == "dev" || Version == "" || strings.HasSuffix(Version, "-dev")
}
