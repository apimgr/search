# Configuration

Search can be configured via:

1. Configuration file (`server.yml`)
2. Environment variables
3. CLI flags
4. Admin panel (web UI)

**Priority**: CLI flags > Environment > Config file

## Configuration File

Default location: `/etc/search/server.yml` (or `/config/server.yml` in Docker)

### Server Settings

```yaml
server:
  # Site title displayed in browser
  title: "Search"

  # Site description
  description: "Privacy-Respecting Metasearch Engine"

  # Listen port
  port: 8080

  # Listen address (empty = all interfaces)
  address: ""

  # Application mode: production or development
  mode: production

  # Base URL for the application
  base_url: "https://search.example.com"

  # Secret key for sessions (auto-generated if empty)
  secret_key: ""
```

### Admin Settings

```yaml
server:
  admin:
    enabled: true
    username: "admin"
    password: "changeme"
    email: "admin@example.com"
```

### SSL/TLS Settings

```yaml
server:
  ssl:
    enabled: false
    cert_file: "/config/ssl/cert.pem"
    key_file: "/config/ssl/key.pem"
    auto_tls: false
    letsencrypt:
      enabled: false
      email: "admin@example.com"
      domains:
        - "search.example.com"
      staging: false
```

### Rate Limiting

```yaml
server:
  rate_limit:
    enabled: true
    requests_per_minute: 60
    burst: 10
```

### Logging

```yaml
server:
  logs:
    level: info  # debug, info, warn, error
    access:
      enabled: true
      format: combined  # common, combined, json
    error:
      enabled: true
```

### Tor Hidden Service

```yaml
server:
  tor:
    enabled: false
    # Tor will auto-start if the tor binary is installed
```

### Search Settings

```yaml
search:
  # Default search engine
  default_engine: "auto"

  # Results per page
  results_per_page: 10

  # Safe search level: off, moderate, strict
  safe_search: moderate

  # Default language
  default_language: "en"

  # Cache settings
  cache:
    enabled: true
    ttl: 300  # seconds
```

### Image Proxy

```yaml
server:
  image_proxy:
    enabled: true
    max_size: 10485760  # 10MB
    timeout: 30
```

## Environment Variables

All configuration can be set via environment variables with the `SEARCH_` prefix:

| Variable | Description | Default |
|----------|-------------|---------|
| `SEARCH_PORT` | Listen port | `8080` |
| `SEARCH_ADDRESS` | Listen address | `` |
| `SEARCH_MODE` | Application mode | `production` |
| `SEARCH_ADMIN_USERNAME` | Admin username | `admin` |
| `SEARCH_ADMIN_PASSWORD` | Admin password | - |
| `SEARCH_SSL_ENABLED` | Enable SSL | `false` |
| `SEARCH_TOR_ENABLED` | Enable Tor | `false` |

## CLI Flags

```bash
search --help

Usage: search [options]

Options:
  --help                 Show help
  --version              Show version
  --mode MODE            Application mode (production|development)
  --config DIR           Config directory
  --data DIR             Data directory
  --log DIR              Log directory
  --address ADDR         Listen address
  --port PORT            Listen port
  --status               Show server status
  --service CMD          Service management
  --maintenance CMD      Maintenance commands
  --update CMD           Update management
```

## Admin Panel

All settings can be configured via the admin panel at `/admin/server/settings`.

See [Admin Panel](admin.md) for details.
