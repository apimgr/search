# Configuration

Search is configured via:

1. Configuration file (`server.yml`)
2. Environment variables
3. CLI flags

**Priority**: CLI flags > Environment variables > Config file

## Configuration File

Default location:
- Linux (root): `/etc/apimgr/search/server.yml`
- Linux (user): `~/.config/apimgr/search/server.yml`
- Docker: `/config/search/server.yml`

The file is auto-generated with defaults on first run. There is no admin web UI — all configuration is file-only.

### Server Settings

```yaml
server:
  # Site title displayed in browser
  title: "Search"

  # Site description
  description: "Privacy-Respecting Metasearch Engine"

  # Listen port
  port: 64580

  # Listen address (empty = all interfaces)
  address: ""

  # Application mode: production or development
  mode: production

  # Base URL for the application
  base_url: ""

  # Operator token (auto-generated on first run)
  token: ""
```

### SSL/TLS Settings

```yaml
server:
  ssl:
    enabled: false
    cert_file: "/config/search/ssl/cert.pem"
    key_file: "/config/search/ssl/key.pem"
    auto_tls: false
    letsencrypt:
      enabled: false
      email: "operator@example.com"
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
    error:
      enabled: true
```

Note: access logging (per-request IPs and queries) is intentionally not supported — privacy is the product.

### Tor Hidden Service

```yaml
server:
  tor:
    # Auto-enabled when tor binary is found on PATH
    use_network: false
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

### Search Alert Settings

```yaml
search:
  alerts:
    create_rate_limit_per_hour: 10
    webhook_max_retries: 3
    webhook_retry_delay_minutes: 5
    retention_days: 30
    default_frequency: "daily"
    default_deliver_rss: true
    default_deliver_webhook: false
```

These settings control accountless search alert creation limits, webhook retry and backoff behavior, how long previously seen alert results are retained for deduplication, and which delivery options are enabled by default in the alert UI.

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
| `SEARCH_PORT` | Listen port | `64580` |
| `SEARCH_ADDRESS` | Listen address | `` |
| `SEARCH_MODE` | Application mode | `production` |
| `SEARCH_SSL_ENABLED` | Enable SSL | `false` |

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
