// Package version provides build info and version strings
// Per AI.md PART 7: Common Go Modules - version/version.go
package version

import (
	"fmt"
	"runtime"
	"strings"
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
	sb.WriteString(fmt.Sprintf("Version:    %s\n", i.Version))
	sb.WriteString(fmt.Sprintf("Commit:     %s\n", i.Commit))
	sb.WriteString(fmt.Sprintf("Build Date: %s\n", i.BuildDate))
	sb.WriteString(fmt.Sprintf("Branch:     %s\n", i.Branch))
	sb.WriteString(fmt.Sprintf("Go Version: %s\n", i.GoVersion))
	sb.WriteString(fmt.Sprintf("OS/Arch:    %s/%s\n", i.OS, i.Arch))
	sb.WriteString(fmt.Sprintf("Compiler:   %s", i.Compiler))
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
