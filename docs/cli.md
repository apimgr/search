# CLI Reference

Search provides a comprehensive command-line interface for server management and a separate CLI client for searching from the terminal.

## Server CLI

The server binary includes management commands.

### Basic Commands

```bash
# Show help
search --help

# Show version
search --version

# Check server status
search --status
```

The shared `--color` option accepts `auto`, `yes`, or `no`; `auto` detects
terminal support. A non-empty `NO_COLOR` disables colors unless explicitly
overridden with `--color=yes`. The shared `--lang` option selects a supported
language, and `--shell` prints shell integration instructions.

### Shell Completions

Both binaries include their completion generators. `--shell completions`
prints a script to standard output, and `--shell init` prints the command to
load that script in the current shell. Omitting the shell name detects it from
`$SHELL`.

Supported shells are Bash, Zsh, Fish, POSIX `sh`, `dash`, `ksh`, PowerShell,
and PowerShell Core (`pwsh`). POSIX shells receive basic completion
candidates; the other shells use their native completion interfaces.

```bash
# Evaluate the detected shell's init command in the current shell
eval "$(search --shell init)"
eval "$(search-cli --shell init)"

# Generate a script explicitly
search --shell completions bash
search-cli --shell completions zsh
```

For persistent completion, add the corresponding init command to the shell's
startup file. Fish uses its native pipeline form:

```fish
search --shell init fish | source
search-cli --shell init fish | source
```

### Running the Server

Run these server commands on the host yourself — not by AI.

```bash
# Start server with defaults
search

# Start with custom port
search --port 64080

# Start with custom config directory
search --config /etc/apimgr/search

# Start with custom data directory
search --data /var/lib/apimgr/search

# Start in development mode
search --mode development

# Enable the I2P eepsite for this run
search --i2p

# Start as daemon
search --daemon
```

### Service Management

These commands control the service installed on the machine. Run them yourself
on the deployment machine; do not have an AI agent execute them.

```bash
# Install as system service
search --service --install

# Uninstall system service
search --service --uninstall

# Start service
search --service start

# Stop service
search --service stop

# Restart service
search --service restart

# Reload configuration
search --service reload

# Disable or remove service
search --service --disable
search --service --uninstall

# Show service help
search --service --help
```

`--service --uninstall` prompts for confirmation, then removes the service and
deletes its configuration, data, cache, logs, backups, PID file, and
server-created system user/group. The server binary remains installed. Use
`--service --disable` to stop and disable the service while keeping its files
and data.

### Maintenance Commands

```bash
# Create backup
search --maintenance backup

# Include optional SSL files and data files
search --maintenance backup --include-ssl --include-data

# Verify the newest backup without restoring it
search --maintenance backup test

# Verify a specific backup
search --maintenance backup test /path/to/backup.tar.gz.enc

# Restore from backup
search --maintenance restore /path/to/backup.tar.gz

# Rotate a server secret
search --maintenance secret rotate installation_secret
search --maintenance secret rotate encryption_key

# Manage API tokens
search --maintenance token list
search --maintenance token revoke <prefix>

# Export one owner's data (TOKEN_PREFIX comes from `--maintenance token list`)
search --maintenance data export --owner TOKEN_PREFIX [--output FILE]

# Delete one owner and all their resources
search --maintenance data delete --owner TOKEN_PREFIX [--yes]

# Print compliance status
search --maintenance compliance report

# List coordinated-disclosure reports without exposing report content
search --maintenance security list [--status received|triaged|confirmed|patching|disclosed|wont_fix]

# View report metadata and researcher-visible comment
search --maintenance security show sec_0123456789abcdef

# Export the already-encrypted report message
search --maintenance security export sec_0123456789abcdef [--output FILE]

# Update report triage and researcher-visible fields
search --maintenance security update sec_0123456789abcdef --status triaged --comment "Under review"

# Preview and validate every email template without sending
search --maintenance email test --dry-run

# Send a test email to the configured reply-to address
search --maintenance email test

# Send to a specific address
search --maintenance email test --to operator@example.com

# Inspect and operate the built-in scheduler
search --maintenance scheduler list
search --maintenance scheduler show backup_daily
search --maintenance scheduler run backup_daily
search --maintenance scheduler enable backup_hourly
search --maintenance scheduler disable backup_hourly
search --maintenance scheduler history backup_daily

# Show maintenance help
search --maintenance --help
```

Encryption-key rotation re-encrypts owner and linked API token copies,
AES-encrypted security reports and contacts, QR short-link destinations, and
DNS provider credentials. The previous key is stored wrapped in the database
for a 30-day grace period; interrupted migrations resume at server startup.
Another rotation is refused while that grace period is active.

Backup creation uses `server.backup.encryption_password` from `secrets.yml`
without prompting. Testing or restoring an encrypted archive prompts for that
archive's password.

Owner data export and deletion require the operator token or root. Export writes
a mode `0600` JSON file in the current directory by default; use `--output -`
for JSON on stdout or name a file explicitly. Exports contain owner and token
metadata and owned resources, never token values, hashes, or audit/security
logs. Deletion removes the owner, tokens, and owned resources in one transaction
after you confirm the owner ID; `--yes` skips the prompt for automation.

Backups include `server.db` as a consistent SQLite snapshot, including committed
WAL data. Data files and the optional `{config_dir}/ssl/` tree are omitted unless
`--include-data` and `--include-ssl` are supplied. Referenced cache TLS files
and external manually configured server certificates are included automatically.
The backup copy of `secrets.yml` blanks `server.token` and
`server.backup.encryption_password`; other configured credentials remain
available for restoring service configuration. Restore preserves the target
server's operator token and does not import an archived token. If the target
has no operator token, it remains blank so startup can generate a new one.
Restores replace the database together with the archived configuration and data.

### Update Management

```bash
# Check for updates
search --update check

# Update to latest version
search --update yes

# Update channels (stable | beta | devel)
# Legacy channel aliases daily, rolling, dev, and development normalize to devel
search --update branch beta
search --update branch devel
```

### Tor Management

Tor is managed by the server over its loopback-only control endpoint:

```bash
search --maintenance tor status
search --maintenance tor validate
search --maintenance tor restart
search --maintenance tor regenerate
search --maintenance tor vanity start <prefix> [--workers <count>]
search --maintenance tor vanity stop
search --maintenance tor vanity apply
search --maintenance tor import-keys <path>
```

Regenerating the address, applying a vanity candidate, and importing keys ask
for an explicit `yes` confirmation; pass `--yes` for scripted operation. Key
imports take an absolute path to a Tor key directory readable by the server.
The loopback request sends the path and confirmation only; private key bytes
remain on disk.

Vanity search defaults to one fewer worker than the number of logical CPUs
(minimum one); `--workers` accepts an explicit count from 1 through the
logical CPU count. A running search can be cancelled, and candidates already
found remain available to apply.

Email testing requires configured SMTP. The recipient defaults to
`server.notifications.email.reply_to`; `--to ADDRESS` overrides it. The
`--dry-run` option previews and validates every built-in and configured email
template without connecting to SMTP.

Security report commands require the operator token or root. `show` omits the
encrypted report body. `export` writes the stored encrypted message to a mode
`0600` file (default `TRACKING_ID.asc`); `--output -` writes it to stdout.
Updating a report can move its status forward, set a researcher-visible
comment, expected disclosure date, or severity. Disclosing a report writes its
acknowledgment row in the same database transaction.

Scheduler status and task execution require a running server and the operator
token from `secrets.yml`. Enabling or disabling a task updates its `enabled`
setting under `server.scheduler` in `yaml.d/maintenance.yml`; required tasks
cannot be disabled.

## CLI Client

The CLI client (`search-cli`) provides a terminal-based search experience.

### Installation

Download the matching client binary from [the latest release](https://github.com/apimgr/search/releases/latest):

| Platform | Binary |
|----------|--------|
| Linux amd64 | `search-cli-linux-amd64` |
| Linux arm64 | `search-cli-linux-arm64` |
| macOS amd64 | `search-cli-darwin-amd64` |
| macOS arm64 | `search-cli-darwin-arm64` |
| Windows amd64 | `search-cli-windows-amd64.exe` |
| Windows arm64 | `search-cli-windows-arm64.exe` |
| FreeBSD amd64 | `search-cli-freebsd-amd64` |
| FreeBSD arm64 | `search-cli-freebsd-arm64` |

The default CLI is static and portable across Linux distributions. An optional
native GUI build is published separately for Linux, macOS, and Windows as
`search-cli-gui-{os}-{arch}[.exe]`; it is dynamically linked and does not
replace itself through the default CLI's update flow. Download that artifact
when you want the native GUI. BSD uses the TUI build.

On Unix-like systems, make the downloaded file executable and place it on your
`PATH`:

```bash
# Example: Linux amd64
curl -q -LSsfO https://github.com/apimgr/search/releases/latest/download/search-cli-linux-amd64
chmod +x search-cli-linux-amd64
sudo mv search-cli-linux-amd64 /usr/local/bin/search-cli
```

On macOS, remove the quarantine attribute after installing with
`xattr -d com.apple.quarantine /usr/local/bin/search-cli`.

### Configuration

The client stores settings in `~/.config/apimgr/search/cli.yml` with owner-only
permissions. On first launch, it opens a setup wizard when a terminal or local
display is available.

```yaml
server:
  primary: https://scour.li
  api_version: v1
  timeout: 30s
auth:
  token: ""
display:
  # auto, gui, or tui
  mode: auto
output:
  # table, json, yaml, plain, or csv
  format: table
  color: auto
update:
  auto: false
  check_interval: per_invocation
  channel: stable
```

Set `auth.token` for operator-only commands. Public searches do not require a
token. The client also accepts `--server`, `--token`, `--output`, `--page`, and
`--limit` flags.

On first launch, the setup wizard selects the native GUI for a local display
and the TUI for a terminal session. SSH and Mosh sessions use the TUI even when
X forwarding is available. A `TERM=dumb` or narrower-than-40-column terminal
uses inline prompts instead; without an interactive terminal, configure
`server.primary` in `cli.yml` or pass `--server`.

The wizard can test the server URL and optional token, then save both values to
the active `cli.yml`. When using `--config profile.yml`, “Set as default
server” also writes those values to the default `cli.yml`; leaving “Save to
configuration” unchecked applies the values only to the current run. The GUI
also offers a per-user “Start automatically on login” option on supported
platforms.

Check for a client update with `search-cli --update check`. Install a verified
update from the configured server with `search-cli --update yes`; the client
checks the published SHA-256 before replacing and restarting its binary.
With `update.auto: true`, the client installs an available update automatically
only when it is running non-interactively. Interactive runs ask before
installing.

### Basic Usage

```bash
# Search for something
search-cli "privacy tools"

# Select pagination and result count
search-cli --page 2 --limit 5 "privacy"

# Render JSON output
search-cli --output json "privacy"

# Start the automatically selected GUI or TUI
search-cli
```

### GUI and TUI Modes

The optional `search-cli-gui` release artifact includes the native GUI on
Linux, macOS, and Windows. The default `search-cli` artifact is static and
opens the TUI on a local terminal; when GUI support is present, a local display
opens the GUI by default. SSH and Mosh sessions use the TUI. Set
`display.mode: gui` or `display.mode: tui` in `cli.yml` to choose a mode
explicitly. BSD builds use the TUI because the native GUI backend is not
available there.

The GUI and TUI each provide a screen or view for every server-facing CLI leaf
command, in the same order as `search-cli --help`. Select a command, fill its
structured fields, and run it; the interface displays the command output or
validation error. This includes search, health, configuration, preferences,
alerts, scheduler, and the other API-backed commands. The GUI uses the
operating system's light or dark theme. The TUI adapts to terminal size and
provides keyboard help with `?`; use `j`/`k` to choose a command, Enter to open
it, and Tab to move between its fields and the Run action.

### Scheduler Management

Scheduler commands require the server operator token configured for the CLI.
Tasks that the server marks as required cannot be enabled or disabled.

```bash
search-cli scheduler list
search-cli scheduler show token_cleanup
search-cli scheduler history token_cleanup
search-cli scheduler run backup_daily
```

The client can inspect tasks and trigger a run. Change a task's enabled state
with the server's local `search --maintenance scheduler enable|disable` command,
which writes `server.scheduler.tasks.<id>.enabled` in the configuration.

Results support `table`, `json`, `yaml`, `plain`, and `csv` output through `--output`.

### TUI Mode

The TUI launches automatically when `search-cli` is run interactively in a
terminal without a local graphical display and with no search query. A local
graphical display opens the GUI by default; there is no `--tui` flag.

```bash
search-cli
```

TUI features:

- Interactive search input
- Navigate results with arrow keys
- Open results in browser
- Search history
- Bang command support

### TUI Keyboard Shortcuts

| Key | Action |
|-----|--------|
| `Enter` | Submit the search or refresh the selected result view |
| `↑/↓`, `j`/`k` | Navigate results |
| `Page Up`/`Page Down`, `h`/`l` | Move through results by a page |
| `/` | Focus search input |
| `Esc` or `b` | Clear results and return to search |
| `q` | Quit |
| `?` | Show keyboard help |

### Output Formats

#### Text (Default)

```bash
search-cli "privacy"
```

Output:
```
1. Privacy - Wikipedia
   https://en.wikipedia.org/wiki/Privacy
   Privacy is the ability of an individual or group...

2. Privacy Policy Generator
   https://www.privacypolicygenerator.org/
   Generate a free privacy policy for your website...
```

#### JSON

```bash
search-cli --output json "privacy"
```

Output:
```json
{
  "query": "privacy",
  "results": [
    {
      "title": "Privacy - Wikipedia",
      "url": "https://en.wikipedia.org/wiki/Privacy",
      "description": "Privacy is the ability..."
    }
  ]
}
```

### Bang Commands

Use bang commands for quick redirects:

```bash
# Search Wikipedia
search-cli "!w privacy"

# Search Google directly
search-cli "!g privacy"

# Search DuckDuckGo
search-cli "!ddg privacy"
```

### Environment Variables

| Variable | Description |
|----------|-------------|
| `SEARCH_SERVER_PRIMARY` | Default server URL when `--server` is omitted |
| `SEARCH_TOKEN` | API token |
| `SEARCH_SERVER_TIMEOUT` | Request timeout in seconds |
| `SEARCH_OUTPUT_FORMAT` | Default output format (e.g. `table`, `json`) |
| `SEARCH_LANG` | Output language code (e.g. `en`, `es`, `zh`) |
| `SEARCH_COLOR` | Color output mode (`auto`, `yes`, or `no`) |
| `SEARCH_DEBUG` | Enable debug output unless `--debug` explicitly overrides it |
| `NO_COLOR` | Disable ANSI colors and emojis when set to any non-empty value |
