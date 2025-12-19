# Search Specification

**Name**: search
**Organization**: apimgr
**Repository**: https://github.com/apimgr/search

---

## Project Information

| Field | Value |
|-------|-------|
| **Name** | search |
| **Organization** | apimgr |
| **Official Site** | https://search.apimgr.us |
| **Repository** | https://github.com/apimgr/search |
| **README** | README.md |
| **License** | MIT > LICENSE.md |

## Project Description

Search is a privacy-focused metasearch engine that aggregates results from multiple search engines while protecting user privacy. It provides a clean, customizable interface with no tracking, no ads, and full control over search preferences.

## Project-Specific Features

- **Metasearch Aggregation**: Combines results from multiple search engines (Google, DuckDuckGo, Brave, Bing, etc.)
- **Privacy-First**: No tracking, no cookies required, optional Tor support
- **Bang Commands**: Quick search shortcuts (!g, !ddg, !w, etc.)
- **Customizable Widgets**: Homepage widgets (weather, clock, calculator, news, etc.)
- **Search Categories**: Web, images, videos, news, maps, code, files
- **Instant Answers**: Calculator, unit converter, definitions, weather
- **Admin Panel**: Full configuration via web UI
- **API Access**: REST API for programmatic search

---

## Specification Reference

This project follows the specification defined in `../TEMPLATE.md`. All NON-NEGOTIABLE sections MUST be implemented exactly as specified.

Key applicable sections:
- **PART 1**: Core Rules (NON-NEGOTIABLE)
- **PART 2**: Project Structure
- **PART 3**: OS-Specific Paths
- **PART 4**: Privilege Escalation
- **PART 5**: Service Support
- **PART 6**: Configuration
- **PART 14**: API Structure
- **PART 15**: Admin Panel
- **PART 17**: CLI Interface
- **PART 19**: Docker
- **PART 20**: Makefile
- **PART 21**: GitHub Actions

---

## Directory Structure

```
./
├── .github/workflows/      # GitHub Actions
│   ├── release.yml         # Stable releases
│   ├── beta.yml            # Beta releases
│   ├── daily.yml           # Daily builds
│   └── docker.yml          # Docker images
├── src/                    # All source files
│   ├── main.go             # Entry point
│   ├── config/             # Configuration
│   ├── server/             # HTTP server & templates
│   ├── search/             # Search engine logic
│   ├── api/                # REST API handlers
│   ├── admin/              # Admin panel
│   ├── widgets/            # Homepage widgets
│   ├── service/            # Service management
│   └── ...                 # Other packages
├── docker/                 # Docker files
│   ├── Dockerfile          # Production Dockerfile
│   ├── docker-compose.yml  # Production compose
│   └── rootfs/             # Container filesystem overlay
├── binaries/               # Built binaries (gitignored)
├── Makefile                # Build targets
├── README.md               # Documentation
├── LICENSE.md              # MIT License
├── AI.md                   # This file
└── TODO.AI.md              # Task tracking
```

---

## Configuration

Configuration file: `server.yml`

Key settings:
- `server.port` - Listen port
- `server.mode` - production or development
- `server.title` - Search page title
- `search.engines` - Enabled search engines
- `search.widgets` - Widget configuration

All settings editable via admin panel at `/admin`.

---

## API Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/healthz` | GET | Health check |
| `/api/v1/search` | GET | Search query |
| `/api/v1/autocomplete` | GET | Search suggestions |
| `/api/v1/widgets` | GET | List widgets |
| `/api/v1/widgets/{type}` | GET | Widget data |
| `/api/v1/config` | GET | Public config |

---

## CLI Interface

```
search [flags]

Flags:
  --help                     Show help
  --version                  Show version
  --mode <mode>              Set mode (production|development)
  --data <dir>               Set data directory
  --config <dir>             Set config directory
  --address <addr>           Set listen address
  --port <port>              Set listen port
  --service <action>         Service management
  --maintenance <action>     Maintenance operations
  --update <action>          Update management
  --status                   Show server status
  --init                     Initialize configuration
```

---

## Build

All builds use Docker with `golang:alpine`:

```bash
make build    # Build all platforms
make test     # Run tests
make docker   # Build Docker image
make release  # Create release
```

---

## Implementation Status

See TODO.AI.md for current task tracking.

All core features implemented:
- Metasearch engine with multiple backends
- Privacy-focused design
- Admin panel with live reload
- Widget system (12 widgets)
- CLI with full flag support
- Service management (systemd, launchd, etc.)
- Docker support
- GitHub Actions CI/CD
