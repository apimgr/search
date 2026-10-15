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

### Running the Server

```bash
# Start server with defaults
search

# Start with custom port
search --port 9000

# Start with custom config directory
search --config /etc/search

# Start with custom data directory
search --data /var/lib/search

# Start in development mode
search --mode development

# Start as daemon
search --daemon
```

### Service Management

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

### Maintenance Commands

```bash
# Create backup
search --maintenance backup

# Restore from backup
search --maintenance restore /path/to/backup.tar.gz

# Rotate a server secret
search --maintenance secret rotate installation_secret

# Manage API tokens
search --maintenance token list
search --maintenance token revoke <prefix>

# Export or delete a data subject's personal data
search --maintenance data export <subject>
search --maintenance data delete <subject>

# Print compliance status
search --maintenance compliance report

# Show maintenance help
search --maintenance --help
```

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
search tor status
search tor validate
search tor restart
search tor regenerate
search tor vanity start <prefix>
search tor vanity apply
search tor import-keys <path>
```

### Email Testing

Send a test message using the SMTP configuration in `server.yml`:

```bash
search email test <address>
```

## CLI Client

The CLI client (`search-cli`) provides a terminal-based search experience.

### Installation

The CLI client is included with the main distribution:

```bash
# Download
curl -LO https://github.com/apimgr/search/releases/latest/download/search-cli-linux-amd64
chmod +x search-cli-linux-amd64
sudo mv search-cli-linux-amd64 /usr/local/bin/search-cli
```

### Configuration

The client stores settings in `~/.config/apimgr/search/cli.yml` with owner-only
permissions. On first launch, it opens a setup wizard when a terminal or local
display is available.

```yaml
server:
  primary: https://search.example.com
  api_version: v1
  timeout: 30s
auth:
  token: ""
display:
  mode: auto  # auto, gui, or tui
output:
  format: table  # table, json, yaml, plain, csv
  color: auto
```

Set `auth.token` for operator-only commands. Public searches do not require a
token. The client also accepts `--server`, `--token`, `--output`, `--page`, and
`--limit` flags.

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

The release CLI includes the native GUI on Linux, macOS, and Windows. A local
display opens the GUI by default; a local terminal, SSH, or Mosh session opens
the TUI. Set `display.mode: gui` or `display.mode: tui` in `cli.yml` to choose a
mode explicitly. BSD builds use the TUI because the native GUI backend is not
available there.

Both interfaces support search, result browsing, and pagination. The GUI uses
the operating system's light or dark theme. The TUI adapts to terminal size and
provides keyboard help with `?`.

### Scheduler Management

Scheduler commands require the server operator token configured for the CLI.
Tasks that the server marks as required cannot be enabled or disabled.

```bash
search-cli scheduler list
search-cli scheduler show token_cleanup
search-cli scheduler history token_cleanup
search-cli scheduler run backup_daily
search-cli scheduler disable backup_hourly
search-cli scheduler enable backup_hourly
```

Results support `table`, `json`, `yaml`, `plain`, and `csv` output through `--output`.

### TUI Mode

The TUI launches automatically when `search-cli` is run interactively with no search query (auto-detected from the terminal environment — there is no `--tui` flag).

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
search-cli --format json "privacy"
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
| `SEARCH_SERVER` | Server URL (overrides `--server` flag when no flag given) |
| `SEARCH_TOKEN` | API token |
| `SEARCH_FORMAT` | Output format |
| `SEARCH_COLOR` | Color output: `auto`, `yes`, `no` |
| `SEARCH_LANG` | Output language code (e.g. `en`, `es`, `zh`) |
| `NO_COLOR` | Disable ANSI colors and emojis when set to any non-empty value |
