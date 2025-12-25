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
search --service install

# Uninstall system service
search --service uninstall

# Start service
search --service start

# Stop service
search --service stop

# Restart service
search --service restart

# Reload configuration
search --service reload

# Show service help
search --service help
```

### Maintenance Commands

```bash
# Create backup
search --maintenance backup

# Restore from backup
search --maintenance restore /path/to/backup.tar.gz

# Run setup wizard
search --maintenance setup

# Show maintenance help
search --maintenance help
```

### Update Management

```bash
# Check for updates
search --update check

# Update to latest version
search --update yes

# Update to specific branch
search --update branch=beta
```

### Build Commands

For development:

```bash
# Build all platforms
search --build all

# Build specific platform
search --build linux/amd64

# Build with custom version
search --build all --build-version 1.2.3
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

Configure the CLI client by creating `~/.config/search/cli.yml`:

```yaml
# Server to connect to
server: "https://search.example.com"

# API token (optional, for authenticated access)
api_token: "key_xxxxxxxxxxxxx"

# Default output format
format: text  # text, json

# Results per page
per_page: 10

# Safe search level
safe_search: moderate

# Default category
category: general
```

### Basic Usage

```bash
# Search for something
search-cli "privacy tools"

# Search with specific category
search-cli --category images "cats"

# Output as JSON
search-cli --format json "privacy"

# Limit results
search-cli --limit 5 "privacy"
```

### TUI Mode

Launch the interactive terminal UI:

```bash
search-cli --tui
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
| `Enter` | Open selected result |
| `↑/↓` | Navigate results |
| `Tab` | Switch between input and results |
| `/` | Focus search input |
| `q` | Quit |
| `?` | Show help |

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
| `SEARCH_SERVER` | Server URL |
| `SEARCH_API_TOKEN` | API token |
| `SEARCH_FORMAT` | Output format |
| `NO_COLOR` | Disable color output |
