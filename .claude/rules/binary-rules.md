# Binary Rules (PART 7, 8, 32)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO

- NEVER build with CGO enabled or link non-Go/non-static deps — single static binary only
- NEVER hand-roll flag parsing for the server (stdlib `flag` only); NEVER skip cobra/viper conventions for CLI
- NEVER hardcode the displayed binary name — `--help`/`--version`/error text MUST use `filepath.Base(os.Args[0])`
- NEVER hardcode User-Agent or config paths to the actual (possibly renamed) binary name — these ALWAYS use the compiled-in `{project_name}` regardless of rename
- NEVER implement `--tui`, `--cli`, `--gui`, `--mode tui/cli/gui`, or a `tui` subcommand — display mode is auto-detected only, override via config (`display.mode` / `cli.yml`) not flags
- NEVER re-derive directory mode after a privilege drop — lock root-vs-user mode at EUID-at-startup
- NEVER use `strconv.ParseBool()` for boolean flags/config — use `config.ParseBool()`/`config.IsTruthy()`
- NEVER construct URLs by string-concatenating raw user input — always `url.PathEscape`/`url.QueryEscape` via `urlutil.BuildAPIURL()`
- NEVER attempt GUI over SSH/Mosh — remote sessions always get TUI/CLI, even if `DISPLAY` is set (X11 forwarding)
- NEVER use Electron/web-based GUI — native toolkit only (GTK4/Qt6 Linux, Cocoa macOS, Win32/WinUI Windows)
- NEVER add short flags beyond `-h`/`-v` — everything else is long-form only
- NEVER call `sudo` or require root/admin to display help at any level
- NEVER save an invalid or empty flag value over a valid existing config value (`server.primary`, `auth.token`, etc.)
- NEVER put CLI runtime state (config/data/cache/logs) in OS system dirs (`/etc`, `/var/lib`, `/var/log`, `C:\ProgramData`) even when invoked as root/admin
- NEVER skip SHA-256 verification on CLI self-update — mismatch = delete temp, abort
- NEVER require auth for public/pastebin-style endpoints — tokens are for ownership/operator actions only, not general API access
- NEVER commit code with TODO/stub logic in binary startup/CLI paths

## CRITICAL - ALWAYS DO

- ALWAYS build with `CGO_ENABLED=0`, `-buildvcs=false -trimpath`, embedding assets via Go `embed`
- ALWAYS detect display/terminal environment (GUI/TUI/CLI/Headless) via `display.DetectDisplayEnv()` from `src/common/display/detect.go`; respect `TERM=dumb` and `NO_COLOR` (colors AND emojis)
- ALWAYS support `--shell completions [SHELL]` and `--shell init [SHELL]` built into the binary (bash/zsh/fish/sh/dash/ksh/powershell/pwsh), auto-detecting `$SHELL` when omitted
- ALWAYS accept both `--flag=value` and `--flag value` syntax
- ALWAYS provide a config-file equivalent for every flag (flag > env var > config file > compiled default precedence)
- ALWAYS run the full 21-phase server startup sequence in order: immediate-exit flags → service commands → maintenance commands → update commands → real startup (root/user branch, privilege drop after directory creation/ownership and privileged-port bind, as early as possible after that)
- ALWAYS use exact (not substring) process identity match and stale-PID detection for PID files; skip PID files entirely in containers
- ALWAYS use root perms 0755/0644 and user perms 0700/0600, decided once at EUID-at-startup
- ALWAYS follow the client IP priority chain (CF-Connecting-IP > True-Client-IP > X-Real-IP > X-Forwarded-For > X-Client-IP > RemoteAddr), gated by trusted_proxies
- ALWAYS resolve auth tokens in header priority order: Authorization > X-API-Key > X-Auth-Token > X-Token > query param (last resort)
- ALWAYS hash tokens with SHA-256, compare in constant time, show raw token once only
- ALWAYS give the CLI a full-featured TUI (bubbletea, default when interactive+no args) and, if built, a fully native GUI with 100% feature parity — never a GUI wrapper around the TUI
- ALWAYS run the CLI setup wizard on first run / no server configured (GUI if local display, TUI if SSH/Mosh/terminal-only, error if neither)
- ALWAYS keep CLI runtime state under the invoking user's XDG/profile dirs, even if the binary is installed system-wide or run as root
- ALWAYS create directories/files with correct perms before any I/O (`EnsureDirs()` on every CLI startup: config 0700 dirs, 0600 files)
- ALWAYS resolve `--config NAME` against `{config_dir}/NAME.yml` (or `.yaml`), absolute paths as-is, `~` expanded
- ALWAYS make destructive CLI operations require `--force` or interactive confirmation
- ALWAYS handle SIGWINCH (Unix) / poll (Windows) for terminal resize and reflow TUI layout
- ALWAYS support terminal sizes from Micro (<40 cols) through Massive (400+ cols) gracefully — phone SSH is a primary use case
- ALWAYS verify SHA-256 checksum and do an atomic binary swap + re-exec for CLI self-update, matching PART 22's server self-update flow

## Key Rules Summary

**Binary naming:**
- Display (help/version/errors): actual `os.Args[0]` basename — respects user renames
- Internal (User-Agent, hardcoded project identity, config subpaths): always compiled-in `{project_name}` / `{project_name}-cli`, never affected by rename
- `--version` format: `{binary_name} {version} ({commit_sha}) built {build_date}`

**Single-binary rule:**
- Server: `{project_name}` binary, stdlib `flag` parsing only, embeds all assets, static/CGO-free
- CLI: `{project_name}-cli` binary, cobra/viper, built by the same Makefile/`make build`, same versioning as server
- No external installers/scripts — the binary creates its own dirs, perms, and default config on first run

**CLI flag conventions:**
- Universal flags: `-h/--help`, `-v/--version`, `--color {auto|yes|no}`, `--lang CODE`, `--debug` (required, not optional)
- `--server URL` (priority: flag > `{PROJECT}_SERVER_PRIMARY` env > `cli.yml server.primary` > compiled official site > error)
- `--token`/`--token-file` (priority: flag > `{PROJECT}_TOKEN` env > `cli.yml auth.token`; env var never persists to config)
- `--config NAME` selects an alternate `cli.yml`-style profile
- Flags only persist to config when current config value is empty or invalid; a valid existing value is never overwritten, flag is used session-only
- Prefer smart argument/stdin/file detection over extra flags (e.g. `cli notes.txt` beats `cli create --file notes.txt`)
- No `config`/`tui` subcommands — TUI auto-launches with no args in an interactive terminal; help/version are flags, never commands

**Client behavior:**
- Mode auto-detection: `-h`/`-v` → exit immediately (CLI, never TUI); interactive terminal + no command/only config flags → TUI; interactive terminal + command/args → CLI text; piped/non-interactive → plain output
- Server binary only ever shows a status banner (console/GUI tray/log) — it has no interactive wizard; configure via editing `server.yml`
- CLI is the only binary with a full setup wizard, full TUI, and optional full native GUI
- Containers: server starts immediately and logs; CLI requires `--server` (or env/mounted config) since there's no interactive display
- HTTP client always sends `User-Agent: {project_name}-cli/{version}` (fixed, not derived from renamed binary)
- All outbound URLs built via `urlutil.BuildAPIURL()` / `PathEscape`/`QueryEscape` — never raw string concatenation
- Output formats: table (default), json, yaml, plain, csv — controlled by `output.format` in `cli.yml`
- Exit codes: 0 success, 1 general error, 2 config error, 3 connection error, 4 auth error, 5 not found, 64 usage error

For complete details, see AI.md PART 7, 8, 32.
