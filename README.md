# search

[![CI](https://github.com/apimgr/search/actions/workflows/ci.yml/badge.svg)](https://github.com/apimgr/search/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/apimgr/search)](https://github.com/apimgr/search/releases)
[![License](https://img.shields.io/github/license/apimgr/search)](LICENSE.md)

## About

Search is a privacy-respecting metasearch engine that aggregates results from multiple search engines without tracking you. Built in Go for enhanced performance, security, and ease of deployment.

Single self-contained binary. Runs with zero configuration on first start. No admin web UI — all settings are in `server.yml`.

## Official Site

**https://scour.li**

## Features

- Privacy First: No third-party analytics by default, consent-aware preferences, self-hosted data control
- Tor Support: Full Tor integration with SOCKS5, circuit rotation, and .onion service
- Portable Preferences: Save settings locally, export/import them, or share them with portable `prefs` links
- Search Alerts: Accountless alerts with email verification plus private RSS and webhook delivery
- Fast and Efficient: Written in Go with concurrent engine queries
- Multiple Engines: Aggregate results from Google, Bing, DuckDuckGo, and more
- Instant Answers: Calculator, unit/currency converter, weather, dictionary, and more
- Bang Shortcuts: Quick redirects like `!g`, `!w`, `!gh` for fast searching
- Keyboard Navigation: Vim-style shortcuts for power users (j/k, /, Enter)
- Mobile Friendly: Responsive design that works on all devices
- Dark and Light Themes: Modern interface with dark (Dracula) and light themes
- File-only Configuration: All settings in `server.yml`; no admin web UI
- Multi-Category Search: Web, images, videos, news, maps, files, music, science, IT, and social
- Built-in SSL: Let's Encrypt integration for automatic HTTPS
- Monitoring: Prometheus metrics and health endpoints
- Container Ready: Docker and Docker Compose support
- GeoIP: Country detection and blocking capabilities
- Email Notifications: Alerts for important events

## Production

### Binary

```bash
# Download latest release
curl -fsSL https://raw.githubusercontent.com/apimgr/search/main/scripts/install.sh | bash

# Start the server (auto-selects port in 64000-65535 range)
search

# Specify port
search --port 64080
```

### Docker

```bash
docker run -d \
  -p 172.17.0.1:64080:80 \
  -v ./data:/data \
  -v ./config:/config \
  --name search \
  ghcr.io/apimgr/search:latest
```

### Docker Compose

```bash
git clone https://github.com/apimgr/search.git
cd search
docker compose -f docker/docker-compose.yml up -d
```

### Install as System Service

```bash
# Install (writes systemd unit / launchd plist / Windows service)
sudo search install

# Uninstall
sudo search uninstall
```

### Multi-Platform Downloads

| Platform | Download |
|----------|----------|
| Linux amd64 | `search-linux-amd64` |
| Linux arm64 | `search-linux-arm64` |
| macOS amd64 | `search-darwin-amd64` |
| macOS arm64 | `search-darwin-arm64` |
| Windows amd64 | `search-windows-amd64.exe` |
| Windows arm64 | `search-windows-arm64.exe` |
| FreeBSD amd64 | `search-freebsd-amd64` |
| FreeBSD arm64 | `search-freebsd-arm64` |

All binaries at: https://github.com/apimgr/search/releases/latest

## Client

A companion CLI/TUI client (`search-cli`) is available for interacting with the Search API from the terminal.

### Install

```bash
# Linux
curl -LO https://github.com/apimgr/search/releases/latest/download/search-cli-linux-amd64
chmod +x search-cli-linux-amd64
sudo mv search-cli-linux-amd64 /usr/local/bin/search-cli

# macOS
curl -LO https://github.com/apimgr/search/releases/latest/download/search-cli-darwin-arm64
chmod +x search-cli-darwin-arm64
sudo mv search-cli-darwin-arm64 /usr/local/bin/search-cli
```

### Configure

```bash
# Connect to a server
search-cli config --server https://search.example.com --token YOUR_API_TOKEN

# Or via environment variables
export SEARCH_SERVER="https://search.example.com"
export SEARCH_TOKEN="your-api-token"
```

### Usage

```bash
# Search from command line
search-cli search "golang tutorials"

# Search with category
search-cli search --category images "cute cats"

# Interactive TUI mode
search-cli tui

# Show help
search-cli --help
```

## Configuration

On first run, Search creates a configuration file with sane defaults.

**Config file locations:**

| OS | Path |
|----|------|
| Linux (root) | `/etc/apimgr/search/server.yml` |
| Linux (user) | `~/.config/apimgr/search/server.yml` |
| macOS | `~/Library/Application Support/apimgr/search/server.yml` |
| Windows | `%AppData%\apimgr\search\server.yml` |
| BSD | `/usr/local/etc/apimgr/search/server.yml` |

### Basic Configuration

```yaml
server:
  # Port in 64000-65535 range (random on first run, saved to config)
  port: 64080

  # Listen address: [::] (all interfaces) or 127.0.0.1 (localhost only)
  address: "[::]"

  # Application mode: production or development
  mode: production

  # Operator bearer token (auto-generated on first run)
  token: ""

# Enable search engines
engines:
  google: true
  bing: true
  duckduckgo: true
  wikipedia: true
```

### Environment Variables

```bash
SEARCH_MODE=production
SEARCH_PORT=64080
SEARCH_LISTEN="0.0.0.0"
SEARCH_CONFIG_DIR="/etc/apimgr/search"
SEARCH_DATA_DIR="/var/lib/apimgr/search"
```

### Operator Access

There is **no admin web UI**. Configuration is file-driven via `server.yml` and reloaded with `SIGHUP`. To rotate the operator token:

```bash
search maintenance rotate-token
```

## API

All API endpoints are under `/api/v1/`. Every web page has a corresponding JSON API endpoint.

### Search

```bash
# Search
curl "http://localhost:PORT/api/v1/search?q=golang&category=general"

# Autocomplete
curl "http://localhost:PORT/api/v1/autocomplete?q=gol"

# List engines
curl "http://localhost:PORT/api/v1/engines"
```

### Alerts

```bash
# Create an alert (accountless)
curl -X POST "http://localhost:PORT/api/v1/alerts" \
  -H "Content-Type: application/json" \
  -d '{
    "query": "golang release notes",
    "category": "news",
    "email": "alerts@example.com",
    "frequency": "daily",
    "deliver_rss": true
  }'

# Manage alert with returned token
curl "http://localhost:PORT/api/v1/alerts/MANAGE_TOKEN"
```

### Health and Status

```bash
# Public health (no auth)
curl http://localhost:PORT/server/healthz

# Engine status (operator token required)
curl -H "Authorization: Bearer YOUR_TOKEN" \
  http://localhost:PORT/api/v1/server/engines
```

### API Documentation

Interactive API docs at: `http://localhost:PORT/server/docs/swagger`

GraphQL playground at: `http://localhost:PORT/server/docs/graphql`

## Other

### Supported Search Engines

Web: Google, Bing, DuckDuckGo, Yahoo, Brave Search, Startpage, Qwant, Wikipedia

Video: YouTube

News and Social: Reddit

Code: GitHub, Stack Overflow

### Instant Answers

Zero-click answers displayed above search results:

| Widget | Example |
|--------|---------|
| Calculator | `2 + 2`, `sqrt(144)` |
| Unit Converter | `5 miles in km`, `100F to C` |
| Currency | `100 usd to eur` |
| Weather | `weather tokyo` |
| Dictionary | `define ubiquitous` |
| IP Lookup | `my ip`, `8.8.8.8` |
| QR Code | `qr https://example.com` |
| UUID | `uuid` |
| Hash | `sha256 hello` |

### Bang Shortcuts

Quick redirects with `!` prefix: `!g` Google, `!w` Wikipedia, `!gh` GitHub, `!yt` YouTube, `!r` Reddit, `!so` Stack Overflow, `!npm` NPM, `!ddg` DuckDuckGo, and 170+ more.

### Keyboard Navigation

| Key | Action |
|-----|--------|
| `/` | Focus search box |
| `j` / `k` | Navigate results down/up |
| `Enter` | Open selected result |
| `h` / `l` | Previous/next page |
| `t` | Toggle theme |
| `?` | Show shortcuts help |

### Tor Integration

Search auto-detects a running Tor daemon and enables the hidden service. Configuration in `server.yml`:

```yaml
server:
  tor:
    enabled: true
    socks_proxy: "127.0.0.1:9050"
    control_port: "127.0.0.1:9051"
    hidden_service:
      enabled: false
      port: 80
    circuit_rotation: true
    stream_isolation: true
```

### Privacy

- No third-party tracking by default
- No ads; clean result links with tracking params stripped
- Preferences stored client-side (localStorage + portable `?prefs=` links)
- Optional image proxy
- Full Tor support with per-engine routing and stream isolation

### Monitoring

```bash
# Prometheus metrics
curl http://localhost:PORT/metrics

# Health check
curl http://localhost:PORT/server/healthz
```

### Security Headers

HSTS, CSP, X-Frame-Options, X-Content-Type-Options, rate limiting (120 req/min default), CSRF protection, and input validation on all endpoints.

### Internationalization

Language resolved from: `?lang=` query param → `lang` cookie → `Accept-Language` header → English.

Supported: English, German, French, Spanish, Italian, Portuguese, Dutch, Polish, Russian, Japanese, Chinese, Korean, Arabic, Turkish, and more.

## Development

Development instructions are for contributors only.

### Prerequisites

- Docker (for builds and tests — Go is NOT installed on host)
- Make
- Git

### Build

```bash
# Clone
git clone https://github.com/apimgr/search.git
cd search

# Quick dev build (Docker, outputs to /tmp/apimgr/)
make dev

# Full cross-platform build (8 platforms)
make build

# Run tests (Docker)
make test

# Build and push Docker image
make docker
```

### Project Structure

```
src/           # Go source code
  ├── client/  # CLI/TUI client (search-cli)
  ├── config/  # Configuration
  ├── server/  # HTTP server + handlers
  ├── search/  # Search engine aggregator
  ├── database/# Database layer
  └── ...
docker/        # Docker configuration
  ├── Dockerfile        # Production image
  ├── Dockerfile.build  # Build toolchain image
  └── Dockerfile.dev    # Development image
docs/          # MkDocs documentation
tests/         # Integration tests
binaries/      # Built binaries (gitignored)
```

### Adding a Search Engine

1. Create `src/search/engines/myengine.go`
2. Implement the `Engine` interface
3. Register in `src/search/engines/registry.go`
4. Add configuration key to `server.yml` defaults
5. Write unit tests

### Debug Mode

```bash
# Enable debug endpoints (/debug/pprof, /debug/config, /debug/routes, etc.)
search --debug

# Or via environment
DEBUG=true search
```

## Disclaimer

This software is provided "as is" without warranty of any kind. Use at your own risk.

- **No Warranty**: The authors are not responsible for any damages, data loss, or issues arising from use of this software
- **Not Professional Advice**: This software does not constitute legal, financial, medical, or other professional advice
- **Third-Party Services**: If this software connects to external APIs or services, their terms of service apply separately
- **Security**: While we strive to follow security best practices, no software is guaranteed to be free of vulnerabilities
- **Production Use**: Evaluate thoroughly before deploying in production environments

By using this software, you acknowledge that you have read and understood this disclaimer.

## License

This project is licensed under the MIT License — see [LICENSE.md](LICENSE.md) for details.
