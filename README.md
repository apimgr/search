# Search - Privacy-Respecting Metasearch Engine

**A fast, privacy-focused metasearch engine written in Go**

Search is a privacy-respecting metasearch engine that aggregates results from multiple search engines without tracking you. Built in Go for enhanced performance, security, and ease of deployment.

[![Build](https://github.com/apimgr/search/actions/workflows/docker.yml/badge.svg)](https://github.com/apimgr/search/actions/workflows/docker.yml)
[![Release](https://img.shields.io/github/v/release/apimgr/search)](https://github.com/apimgr/search/releases)
[![License](https://img.shields.io/github/license/apimgr/search)](LICENSE.md)
[![Go Version](https://img.shields.io/badge/Go-1.23+-00ADD8?logo=go)](https://go.dev)
[![Docker](https://img.shields.io/badge/Docker-Available-2496ED?logo=docker)](https://github.com/apimgr/search/pkgs/container/search)

## ✨ Features

- **🔒 Privacy First**: No third-party analytics by default, consent-aware preferences, self-hosted data control
- **🧅 Tor Support**: Full Tor integration with SOCKS5, circuit rotation, and .onion service
- **💾 Portable Preferences**: Save settings locally, export/import them, or share them with portable `prefs` links
- **🔔 Search Alerts**: Accountless alerts with email verification plus private RSS and webhook delivery
- **🚀 Fast & Efficient**: Written in Go with concurrent engine queries
- **🔍 Multiple Engines**: Aggregate results from Google, Bing, DuckDuckGo, and more
- **💡 Instant Answers**: Calculator, unit/currency converter, weather, dictionary, and more
- **⚡ Bang Shortcuts**: Quick redirects like `!g`, `!w`, `!gh` for fast searching
- **⌨️ Keyboard Navigation**: Vim-style shortcuts for power users (j/k, /, Enter)
- **📱 Mobile Friendly**: Responsive design that works on all devices
- **🎨 Beautiful UI**: Modern interface with dark (Dracula) and light themes
- **⚙️ Easy Configuration**: Web-based admin panel and YAML config
- **🌐 Multi-Category**: Web, images, videos, news, maps, files, music, science, IT, and social
- **🔐 Built-in SSL**: Let's Encrypt integration for automatic HTTPS
- **📊 Monitoring**: Prometheus metrics and health endpoints
- **🐳 Container Ready**: Docker and Docker Compose support
- **🌍 GeoIP**: Country detection and blocking capabilities
- **📧 Notifications**: Email alerts for important events

## 📦 Official Site

**https://scour.li**

## 🚀 Quick Start

### Using Pre-built Binary

```bash
# Download the latest release
curl -fsSL https://raw.githubusercontent.com/apimgr/search/main/scripts/install.sh | bash

# Start the server
search

# Access at http://localhost:64xxx (random port, shown on startup)
```

### Using Docker

```bash
# Run with Docker
docker run -d \
  -p 64080:80 \
  -v ./data:/data \
  -v ./config:/config \
  --name search \
  ghcr.io/apimgr/search:latest

# Access at http://localhost:64080
```

### Using Docker Compose

```bash
# Clone the repository
git clone https://github.com/apimgr/search.git
cd search

# Start with Docker Compose
docker-compose up -d

# Access at http://localhost:64xxx (see docker-compose.yml for port)
```

## 📖 Installation

### Linux

```bash
# Using the install script (recommended)
curl -fsSL https://raw.githubusercontent.com/apimgr/search/main/scripts/install.sh | bash

# Or manually
wget https://github.com/apimgr/search/releases/latest/download/search-linux-amd64
chmod +x search-linux-amd64
sudo mv search-linux-amd64 /usr/local/bin/search

# Install as service
sudo search --service --install
sudo search --service start
```

### macOS

```bash
# Using Homebrew (coming soon)
# brew install apimgr/tap/search

# Or using the install script
curl -fsSL https://raw.githubusercontent.com/apimgr/search/main/scripts/macos.sh | bash

# Or manually
wget https://github.com/apimgr/search/releases/latest/download/search-darwin-amd64
chmod +x search-darwin-amd64
sudo mv search-darwin-amd64 /usr/local/bin/search

# Install as service
sudo search --service --install
sudo search --service start
```

### Windows

```powershell
# Using PowerShell (run as Administrator)
Invoke-WebRequest -Uri "https://raw.githubusercontent.com/apimgr/search/main/scripts/windows.ps1" -UseBasicParsing | Invoke-Expression

# Or manually
# Download from: https://github.com/apimgr/search/releases/latest/download/search-windows-amd64.exe
# Place in C:\Program Files\apimgr\search\
# Install as service:
# search.exe --service --install
```

### BSD

```bash
# FreeBSD/OpenBSD
curl -fsSL https://raw.githubusercontent.com/apimgr/search/main/scripts/install.sh | bash

# Or manually
fetch https://github.com/apimgr/search/releases/latest/download/search-freebsd-amd64
chmod +x search-freebsd-amd64
sudo mv search-freebsd-amd64 /usr/local/bin/search

# Install as service
sudo search --service --install
```

## 💻 CLI Client

A companion CLI client is available for interacting with the Search API from the terminal.

### Install CLI

```bash
# Linux/BSD
curl -LO https://github.com/apimgr/search/releases/latest/download/search-linux-amd64-cli
chmod +x search-linux-amd64-cli
sudo mv search-linux-amd64-cli /usr/local/bin/search-cli

# macOS
curl -LO https://github.com/apimgr/search/releases/latest/download/search-darwin-amd64-cli
chmod +x search-darwin-amd64-cli
sudo mv search-darwin-amd64-cli /usr/local/bin/search-cli

# Windows (PowerShell)
Invoke-WebRequest -Uri "https://github.com/apimgr/search/releases/latest/download/search-windows-amd64-cli.exe" -OutFile "search-cli.exe"
```

### Configure CLI

```bash
# Connect to a server (creates ~/.config/apimgr/search/cli.yml)
search-cli config --server https://search.example.com --token YOUR_API_TOKEN

# Or set environment variables
export SEARCH_SERVER="https://search.example.com"
export SEARCH_TOKEN="your-api-token"
```

### CLI Usage

```bash
# Search from command line
search-cli search "golang tutorials"

# Search with category
search-cli search --category images "cute cats"

# Interactive TUI mode
search-cli tui

# Show help
search-cli --help
search-cli search --help
```

### CLI Configuration File

Location: `~/.config/apimgr/search/cli.yml`

```yaml
server: https://search.example.com
token: your-api-token
default_category: general
theme: dark
```

## ⚙️ Configuration

On first run, Search creates a configuration file with sane defaults:

**Linux (root)**: `/etc/apimgr/search/server.yml`  
**Linux (user)**: `~/.config/apimgr/search/server.yml`  
**macOS**: `~/Library/Application Support/apimgr/search/server.yml`  
**Windows**: `%AppData%\apimgr\search\server.yml`  
**BSD**: `/usr/local/etc/apimgr/search/server.yml`

### Basic Configuration

```yaml
server:
  # Port: single (HTTP) or dual (HTTP,HTTPS) e.g., "8090" or "8090,64453"
  port: 64080
  
  # Listen address: [::] (all), 127.0.0.1 (localhost only)
  address: "[::]"
  
  # Application mode: production or development
  mode: production
  
  # Admin credentials (auto-generated on first run)
  admin:
    username: administrator
    password: {auto-generated}
    token: {auto-generated}

# Enable search engines
engines:
  google: true
  bing: true
  duckduckgo: true
  wikipedia: true
```

### Environment Variables

For initial setup (init only):

```bash
export MODE=production           # Application mode
export PORT=64080                # Server port (64xxx range for dev)
export LISTEN="0.0.0.0"         # Listen address
export CONFIG_DIR="/etc/apimgr/search"
export DATA_DIR="/var/lib/apimgr/search"
```

## 🎯 Usage

### Command Line

```bash
# Start the server (default: random 64xxx port)
search

# Specify port
search --port 64080

# Specify dual ports (HTTP + HTTPS)
search --port 80,443

# Development mode
search --mode development

# Check status
search --status

# Show version
search --version

# Show help
search --help
```

### Service Management

```bash
# Install as system service
sudo search --service --install

# Start/stop/restart
sudo search --service start
sudo search --service stop
sudo search --service restart

# Reload configuration
sudo search --service reload

# Uninstall service
sudo search --service --uninstall
```

### Maintenance

```bash
# Create backup
search --maintenance backup

# Restore from backup
search --maintenance restore

# Update to latest version
search --maintenance update

# Enable maintenance mode
search --maintenance mode enable
```

## 🌐 Web Interface

### Search Interface

Access the search interface at: `http://localhost:PORT/`

Features:
- Clean, ad-free search results
- Category tabs (Web, Images, Videos, News, Maps, Files, Music, Science, IT, Social)
- Advanced search options
- Create accountless alerts from the current query
- Dark/Light theme toggle
- Mobile-responsive design

### Admin Panel

Access the admin panel at: `http://localhost:PORT/admin`

Default credentials (shown once on first run):
- **Username**: administrator
- **Password**: (auto-generated, check logs)

Features:
- Server configuration
- Engine management
- User preferences
- Logs viewer
- Statistics
- System monitoring

## 💡 Instant Answers

Zero-click answers displayed above search results:

| Widget | Triggers | Example |
|--------|----------|---------|
| **Calculator** | Math expressions | `2 + 2`, `sqrt(144)`, `15% of 200` |
| **Unit Converter** | Number + unit | `5 miles in km`, `100F to C`, `2 cups in ml` |
| **Currency** | Amount + currency | `100 usd to eur`, `$50 in pounds` |
| **Weather** | Location weather | `weather tokyo`, `forecast london` |
| **Dictionary** | Word definitions | `define ubiquitous`, `dict: serendipity` |
| **Thesaurus** | Synonyms/antonyms | `synonyms for happy`, `antonyms of good` |
| **IP Lookup** | IP address | `my ip`, `8.8.8.8`, `ip 1.1.1.1` |
| **Timezone** | Time in location | `time in tokyo`, `3pm EST to PST` |
| **Calendar** | Date arithmetic | `days until christmas`, `days between jan 1 and mar 15` |
| **Stopwatch** | Timer/stopwatch | `stopwatch`, `timer 5 minutes`, `countdown 30 seconds` |
| **Color** | Color codes | `#ff5733`, `rgb(255,87,51)`, `dark blue` |
| **Hash** | Hash text | `md5 hello`, `sha256 password` |
| **Password** | Generate password | `password`, `random password`, `password 16` |
| **UUID** | Generate UUID | `uuid`, `generate uuid`, `guid` |
| **Random** | Random numbers | `random 1-100`, `roll d20`, `flip coin` |
| **QR Code** | Generate QR | `qr https://example.com` |

## 🔍 Direct Answer Operators

Type these prefixes in the search box for instant full-page answers:

### Network & Security
| Operator | Example | Description |
|----------|---------|-------------|
| `ip:` | `ip:8.8.8.8` | IP geolocation |
| `dns:` | `dns:example.com` | DNS records |
| `whois:` | `whois:example.com` | WHOIS info |
| `cert:` | `cert:example.com` | SSL certificate |
| `headers:` | `headers:example.com` | HTTP response headers |
| `asn:` | `asn:AS15169` | ASN information |
| `subnet:` | `subnet:192.168.1.0/24` | Subnet calculator |
| `resolve:` | `resolve:example.com` | Hostname to IP |
| `robots:` | `robots:example.com` | robots.txt contents |
| `sitemap:` | `sitemap:example.com` | Sitemap URLs |
| `tech:` | `tech:example.com` | Technology detection |
| `feed:` | `feed:example.com` | RSS/Atom feeds |
| `expand:` | `expand:short.url/x` | Expand shortened URL |
| `safe:` | `safe:example.com` | Safe browsing check |

### Encoding & Conversion
| Operator | Example | Description |
|----------|---------|-------------|
| `base64:` | `base64:hello world` | Base64 encode/decode |
| `url:` | `url:hello world!` | URL encode/decode |
| `hash:` | `hash:hello` | MD5, SHA1, SHA256, SHA512 |
| `html:` | `html:<b>text</b>` | HTML entity encode/decode |
| `escape:` | `escape:hello & world` | Escape special characters |
| `color:` | `color:#ff5500` | Color info & conversion |
| `unicode:` | `unicode:U+1F600` | Unicode character lookup |
| `emoji:` | `emoji:smile` | Emoji search |
| `ascii:` | `ascii:hello` | ASCII art |
| `qr:` | `qr:https://example.com` | QR code generator |
| `uuid:` | `uuid:` | Generate UUID v4 |
| `jwt:` | `jwt:eyJ...` | Decode JWT token |

### Developer Tools
| Operator | Example | Description |
|----------|---------|-------------|
| `json:` | `json:{"key":"val"}` | Format / validate JSON |
| `yaml:` | `yaml:key: value` | Format / validate YAML |
| `regex:` | `regex:[a-z]+` | Explain regex pattern |
| `beautify:` | `beautify:minified code` | Beautify / format code |
| `cron:` | `cron:0 * * * *` | Explain cron expression |
| `chmod:` | `chmod:755` | File permission converter |
| `timestamp:` | `timestamp:1700000000` | Unix timestamp converter |
| `slug:` | `slug:Hello World` | Generate URL slug |
| `case:` | `case:hello world` | Case conversion (upper/lower/camel/snake) |

### Reference
| Operator | Example | Description |
|----------|---------|-------------|
| `tldr:` | `tldr:git` | Simplified man page |
| `man:` | `man:grep` | Full Unix man page |
| `cheat:` | `cheat:git` | Command cheatsheet |
| `rfc:` | `rfc:2616` | RFC document |
| `cve:` | `cve:CVE-2024-1234` | CVE vulnerability info |
| `pkg:` | `pkg:express` | Package info (npm/PyPI/Go) |
| `http:` | `http:404` | HTTP status code |
| `port:` | `port:443` | Port number info |

## ⚡ Bang Shortcuts

Quick redirects to specific sites (180+ bangs available):

| Category | Bangs |
|----------|-------|
| **Search** | `!g` Google, `!b` Bing, `!ddg` DuckDuckGo, `!br` Brave, `!sp` Startpage, `!q` Qwant, `!kagi` Kagi |
| **Images** | `!gi` Google Images, `!bi` Bing Images, `!fl` Flickr, `!pexels`, `!pixabay`, `!tineye` |
| **Video** | `!yt` YouTube, `!v` Vimeo, `!twitch`, `!tiktok`, `!rumble` |
| **Maps** | `!gm` Google Maps, `!osm` OpenStreetMap, `!waze`, `!yelp`, `!tripadvisor` |
| **News** | `!gn` Google News, `!hn` Hacker News, `!bbc`, `!techcrunch`, `!verge`, `!reuters` |
| **Social** | `!tw` Twitter/X, `!r` Reddit, `!mast` Mastodon, `!lb` Lobsters, `!bluesky` |
| **Code & Dev** | `!gh` GitHub, `!gl` GitLab, `!so` Stack Overflow, `!npm`, `!pypi`, `!crates`, `!docker`, `!mdn` |
| **Shopping** | `!amz` Amazon, `!eb` eBay, `!etsy`, `!walmart`, `!camelcamelcamel` |
| **Science** | `!scholar` Google Scholar, `!arxiv`, `!pubmed`, `!doi` |
| **Reference** | `!w` Wikipedia, `!wa` Wolfram Alpha, `!mw` Merriam-Webster, `!ud` Urban Dictionary |
| **Privacy** | `!wbm` Wayback Machine, `!virustotal`, `!shodan`, `!urlscan` |
| **Entertainment** | `!imdb`, `!rt` Rotten Tomatoes, `!steam`, `!spot` Spotify, `!letterboxd` |

Examples:
```
!g golang tutorial     → Search Google
!w machine learning    → Wikipedia article
!gh octocat            → GitHub search
!npm express           → NPM package
```

Custom bangs can be defined in user preferences.

## ⌨️ Keyboard Shortcuts

Vim-inspired navigation for power users:

| Key | Action |
|-----|--------|
| `/` | Focus search box |
| `Escape` | Clear/unfocus search |
| `j` / `k` | Navigate results down/up |
| `Enter` | Open selected result |
| `o` / `O` | Open in current/new tab |
| `h` / `l` | Previous/next page |
| `g g` | Go to first result |
| `G` | Go to last result |
| `t` | Toggle theme |
| `s` | Open settings |
| `?` | Show shortcuts help |
| `1-9` | Jump to result N |

## 🔌 API

### REST API

Search uses a versioned REST API at `/api/v1`:

```bash
# Search
curl "http://localhost:PORT/api/v1/search?q=golang&category=general"

# Get text response
curl "http://localhost:PORT/api/v1/search.txt?q=golang"

# Autocomplete
curl "http://localhost:PORT/api/v1/autocomplete?q=gol"

# List engines
curl "http://localhost:PORT/api/v1/engines"
```

```bash
# Create an alert
curl -X POST "http://localhost:PORT/api/v1/alerts" \
  -H "Content-Type: application/json" \
  -d '{
    "query": "golang release notes",
    "category": "news",
    "language": "en",
    "frequency": "daily",
    "email": "alerts@example.com",
    "deliver_rss": true
  }'

# Inspect or update an alert later with the returned manage token
curl "http://localhost:PORT/api/v1/alerts/MANAGE_TOKEN"
curl -X PATCH "http://localhost:PORT/api/v1/alerts/MANAGE_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"deliver_webhook": true, "webhook_url": "https://example.com/search-alerts"}'
```

### Admin API

Manage the server via REST API:

```bash
# Get configuration (requires Bearer token)
curl -H "Authorization: Bearer YOUR_TOKEN" \
  http://localhost:PORT/api/v1/admin/config

# Update configuration
curl -X PUT \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"server": {"port": "64580"}}' \
  http://localhost:PORT/api/v1/admin/config
```

### OpenAPI

View API documentation at: `http://localhost:PORT/openapi`

## 🔧 Search Engines

Currently supported search engines:

### General (Web)
- Google
- Bing
- DuckDuckGo
- Yahoo
- Brave Search
- Startpage
- Qwant
- Wikipedia

### Videos
- YouTube

### News & Social
- Reddit

### Code
- GitHub
- Stack Overflow

*More engines coming soon!*

## 🛡️ Privacy

Search is designed with privacy as the top priority:

- ✅ **No third-party tracking by default**: Analytics scripts stay off unless you configure tracking and users consent
- ✅ **Consent-aware cookies**: Essential cookies keep auth/CSRF working, and preference cookies are optional
- ✅ **No data sharing**: Your data never leaves your server
- ✅ **No ads**: Clean, ad-free results
- ✅ **Clean result links**: Common tracking parameters are stripped from result URLs
- ✅ **Tor support**: Full Tor integration with circuit rotation and .onion service
- ✅ **Client-side preferences**: Settings live in localStorage or portable links, not server-side profiles
- ✅ **Proxy support**: Route requests through SOCKS5/HTTP/Tor proxies
- ✅ **Stream isolation**: Each engine query uses separate Tor circuit
- ✅ **Image proxy**: Optional image proxying for extra privacy
- ✅ **Self-hosted**: You control the data

## 🧅 Tor Integration

Search has comprehensive Tor support for maximum anonymity:

### Features

- **SOCKS5 Proxy**: Route all searches through Tor network
- **Circuit Rotation**: Automatically rotate Tor circuits for enhanced privacy
- **Hidden Service**: Run as .onion service for Tor-only access
- **Per-Engine Routing**: Configure which engines use Tor vs clearnet
- **Stream Isolation**: Each search engine query uses separate Tor circuit
- **Tor Browser Optimized**: Detects and optimizes for Tor Browser
- **Fallback Support**: Gracefully fallback to clearnet if Tor unavailable

### Configuration

```yaml
server:
  # Tor configuration
  tor:
    enabled: true
    
    # SOCKS5 proxy address (default Tor proxy)
    socks_proxy: "127.0.0.1:9050"
    
    # Tor control port for circuit management
    control_port: "127.0.0.1:9051"
    control_password: ""
    
    # Run as .onion hidden service
    hidden_service:
      enabled: false
      port: 80
    
    # Per-engine Tor routing
    # all: route all engines through Tor
    # none: use clearnet for all engines
    # auto: use Tor for engines that support it
    routing: auto
    
    # Force new circuit for each search (maximum privacy)
    circuit_rotation: true
    
    # Stream isolation (separate circuit per engine)
    stream_isolation: true
```

### Usage

```bash
# Start with Tor enabled
search --tor

# Run as .onion hidden service
search --tor --hidden-service

# Check Tor status
search --tor-status
```

## 💾 User Preferences

User preferences stay **client-side**. Most settings live in browser localStorage, can be exported/imported as JSON, or can be encoded into portable `?prefs=` links. Export/import covers the full browser-side preference set, while portable links currently carry the core search/display defaults. With consent, Search may also persist the active theme in a preference cookie for server-rendered pages. Nothing is stored in a server-side profile.

### Stored Preferences

- ✅ Default category (web, images, videos, etc.)
- ✅ Safe search level (off, moderate, strict)
- ✅ Theme preference (dark, light, auto)
- ✅ Results per page (10, 20, 50, 100)
- ✅ Infinite scroll vs pagination
- ✅ Open results in a new tab
- ✅ Keyboard shortcuts
- ✅ Homepage widgets and custom bangs

### Managing Preferences

All preferences can be managed in the web UI:

1. Click **Preferences** button
2. Adjust your settings
3. Click **Save** (stored in browser only)
4. **Export** preferences as JSON for backup
5. **Import** preferences from JSON on new browser
6. **Generate Link** to create a shareable `?prefs=` URL or QR code
7. **Clear All** to reset to defaults

Portable `?prefs=` links currently encode:

- theme
- default category
- safe search
- results per page
- open in new tab
- infinite scroll vs pagination
- keyboard shortcuts

### Privacy Notes

- **Client-side first**: Preferences use localStorage and portable links
- **Consent-aware cookies**: Theme cookies are optional and cleared when cookie preferences are declined
- **No server storage**: Nothing stored in a server-side profile
- **No tracking**: Your settings never leave your browser
- **Portable**: Export/import JSON to move between browsers
- **Optional**: Use default settings without saving anything

## 📊 Monitoring

### Health Check

```bash
# HTML response
curl http://localhost:PORT/healthz

# JSON response
curl http://localhost:PORT/api/v1/healthz
```

### Prometheus Metrics

```bash
# Enable in configuration
server:
  metrics:
    enabled: true
    endpoint: /metrics

# Access metrics
curl http://localhost:PORT/metrics
```

## 🐳 Docker

### Dockerfile

Build your own image:

```bash
docker build -t search:latest .
```

### Docker Compose

Full setup with dependencies:

```yaml
services:
  search:
    image: ghcr.io/apimgr/search:latest
    container_name: search
    ports:
      - "64080:80"
    volumes:
      - ./volumes/data/search:/data
      - ./volumes/config/search:/config
    environment:
      - MODE=production
    restart: unless-stopped
    healthcheck:
      test: ["CMD", "search", "--status"]
      interval: 30s
      timeout: 10s
      retries: 3
```

## 🔐 Security

- **HTTPS by default**: Automatic Let's Encrypt certificates
- **Rate limiting**: Protect against abuse (120 req/min default)
- **Security headers**: HSTS, CSP, X-Frame-Options, etc.
- **CSRF protection**: All forms protected
- **Input validation**: All inputs sanitized
- **GeoIP blocking**: Block countries by ISO code
- **Fail2ban**: Compatible security logs

## 🌍 Internationalization

Use the header language selector or `?lang=de` to choose a language. The server resolves language in this order: `?lang=` query param, `lang` cookie, `Accept-Language` header, then English.

Supported languages include English, German, French, Spanish, Italian, Portuguese, Dutch, Polish, Russian, Japanese, Chinese, Korean, Arabic, Hebrew, Persian, Urdu, Turkish, and Vietnamese.

## 📝 License

This project is licensed under the MIT License - see [LICENSE.md](LICENSE.md) for details.

## 🤝 Contributing

Contributions are welcome! Please read our [Contributing Guide](CONTRIBUTING.md) first.

### Development

See the [Development](#development-1) section below.

## 💬 Support

- **Documentation**: https://apimgr-search.readthedocs.io
- **Issues**: https://github.com/apimgr/search/issues
- **Discussions**: https://github.com/apimgr/search/discussions

## 🙏 Acknowledgments

Built with ❤️ by the apimgr team. Special thanks to all contributors and the open source community.

---

# Development

This section is for developers who want to build, test, or contribute to Search.

## 🛠️ Building from Source

### Prerequisites

- Go 1.23 or later
- Docker (for testing)
- Make

### Build

```bash
# Clone the repository
git clone https://github.com/apimgr/search.git
cd search

# Build for all platforms
make build

# Build for current platform
go build -o binaries/search ./src

# Run
./binaries/search
```

### Build Options

```bash
# Build all platforms to ./binaries
make build

# Run tests
make test

# Build and push Docker image
make docker

# Create GitHub release
make release
```

## 🧪 Testing

We use Docker for all testing to keep the host system clean:

```bash
# Run tests in Docker
docker run --rm \
  -v $PWD:/build \
  -w /build \
  golang:latest \
  go test ./...

# Build and test
docker run --rm \
  -v $PWD:/build \
  -w /build \
  -e CGO_ENABLED=0 \
  golang:latest \
  sh -c "go build -o /tmp/search ./src && /tmp/search --version"
```

## 📁 Project Structure

```
./
├── src/                    # Source code
│   ├── main.go            # Entry point
│   ├── config/            # Configuration
│   ├── server/            # HTTP server
│   ├── search/            # Search engine
│   ├── models/            # Data models
│   ├── database/          # Database drivers
│   └── ...
├── scripts/               # Installation scripts
├── tests/                 # Test files
├── binaries/              # Built binaries (gitignored)
├── releases/              # Release artifacts (gitignored)
├── Makefile               # Build automation
├── Dockerfile             # Container image
├── docker-compose.yml     # Local development
└── README.md              # This file
```

## 🎯 Adding Search Engines

To add a new search engine:

1. Create a new file in `src/search/engines/`
2. Implement the `Engine` interface
3. Register the engine in `src/search/engines/registry.go`
4. Add configuration to `server.yml`
5. Add tests

Example:

```go
package engines

type MyEngine struct {
    config EngineConfig
}

func (e *MyEngine) Name() string {
    return "myengine"
}

func (e *MyEngine) Category() Category {
    return CategoryGeneral
}

func (e *MyEngine) Search(ctx context.Context, query Query) ([]Result, error) {
    // Implement search logic
    return results, nil
}
```

## 🐛 Debugging

Enable development mode for verbose logging:

```bash
# Run in development mode
search --mode development

# Or set environment variable
export MODE=development
search
```

Development mode enables:
- Verbose logging
- Debug endpoints (/debug/pprof/)
- Detailed error messages
- Template hot-reload

## 📚 Documentation

Full documentation is available at: https://scour.li/docs

- [Installation Guide](https://scour.li/docs/installation)
- [Configuration Reference](https://scour.li/docs/configuration)
- [API Documentation](https://scour.li/docs/api)
- [Admin Guide](https://scour.li/docs/admin)

## 🛠️ Development

**Development instructions are for contributors only.**

### Prerequisites

- Go 1.23+ (latest stable recommended)
- Docker (for containerized builds)
- Make

### Build

```bash
# Clone the repository
git clone https://github.com/apimgr/search
cd search

# Quick dev build (outputs to OS temp dir)
make dev

# Full build (all platforms, outputs to binaries/)
make build

# Run tests
make test

# Clean build artifacts
make clean
```

### Project Structure

```
src/           # Go source code
  ├── admin/   # Admin panel
  ├── api/     # REST API
  ├── client/  # CLI client
  ├── config/  # Configuration
  ├── server/  # HTTP server
  └── ...
tests/         # Test files
docker/        # Docker configuration
docs/          # MkDocs documentation
binaries/      # Built binaries (gitignored)
```

### CI/CD

- **GitHub Actions**: Automated testing on pull requests
- **Jenkins**: Multi-architecture builds (AMD64, ARM64)

## 📄 License

This project is licensed under the MIT License - see [LICENSE.md](LICENSE.md) for details.

---

Made with ❤️ by [apimgr](https://github.com/apimgr)
