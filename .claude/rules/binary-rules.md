# Binary Rules (PART 7, 8, 32)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO

- Build on host — always use Docker (casjaysdev/go:latest)
- Create docker/Dockerfile.build for Go projects
- Use CGO (CGO_ENABLED=0 always)
- Use gorilla/mux, cobra, or other archived/wrong libs
- Hard-code binary name (use BINARY variable from Makefile)
- Skip --strip-debug / ldflags strip (binary must be stripped)
- Ship binary without version info embedded via ldflags
- Use os.Exit() outside of main() — return errors instead
- Panic in production code (except during init for truly unrecoverable state)

## CRITICAL - ALWAYS DO

- Single self-contained static binary
- CGO_ENABLED=0 for all builds
- Build flags: -ldflags="-s -w -X main.Version=$(VERSION) -X main.BuildDate=$(BUILD_DATE) -X main.Commit=$(COMMIT)"
- Binary name: search (matches internal_name)
- Cross-compile: linux/amd64 + linux/arm64
- First run works with zero config

## Binary Requirements

| Requirement | Value |
|-------------|-------|
| Binary name | search |
| Module path | github.com/apimgr/search |
| Build env | CGO_ENABLED=0 GOOS=linux GOARCH=amd64 |
| Image | casjaysdev/go:latest |
| Strip | -ldflags="-s -w" |

## ldflags Version Injection

```go
// src/version/version.go
var (
    Version   = "dev"
    BuildDate = "unknown"
    Commit    = "unknown"
)
```

Build command:
```
-ldflags="-s -w -X github.com/apimgr/search/src/version.Version=$(VERSION) \
  -X github.com/apimgr/search/src/version.BuildDate=$(BUILD_DATE) \
  -X github.com/apimgr/search/src/version.Commit=$(COMMIT)"
```

## CLI Flags (Required)

| Flag | Description |
|------|-------------|
| --version | Print version and exit |
| --help | Print help and exit |
| --config | Path to server.yml |
| --port | Override listen port |
| --mode | production or development |
| --debug | Enable debug mode |
| --service | Service management subcommands |
| --maintenance | Maintenance subcommands |

## NO_COLOR Support

- Check NO_COLOR env var before any ANSI output
- TERM=dumb forces plain text (no ANSI)
- Pattern: if os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" { /* no color */ }

## Privilege Escalation

- isElevated() — platform-independent (os.Geteuid()==0 on Unix, windows.Token on Windows)
- canEscalate() — check sudo/wheel/admin group membership
- execElevated() — re-exec with sudo (Unix) or ShellExecute runas (Windows)
- Never call sudo directly in business logic — use handleEscalation()

## Service Management

System service: /etc/systemd/system/search.service
User service: ~/.config/systemd/user/search.service
macOS: /Library/LaunchDaemons/io.github.apimgr.search.plist
FreeBSD: /usr/local/etc/rc.d/search

For complete details, see AI.md PART 7, 8, 32
