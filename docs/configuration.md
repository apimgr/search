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

Most server settings can be set via `SEARCH_`-prefixed environment variables.
Run `search --help` for the authoritative, complete list.

| Variable | Description | Default |
|----------|-------------|---------|
| `SEARCH_PORT` (or `PORT`) | Listen port | random `64000-64999` |
| `SEARCH_ADDRESS` | Listen address | all interfaces |
| `SEARCH_MODE` (or `MODE`) | Application mode (`production`/`development`) | `production` |
| `SEARCH_DEBUG` (or `DEBUG`) | Enable debug mode (`0`/`1`, `true`/`false`) | `false` |
| `SEARCH_BASE_URL` | Public base URL override | derived |
| `SEARCH_COLOR` | Color output mode (`always`/`never`/`auto`) | `auto` |
| `SEARCH_LANG` | Default language | `en` |
| `SEARCH_PID_FILE` | Path to PID file | platform default |
| `DOMAIN` | FQDN override | detected |
| `DATABASE_DRIVER` | `sqlite` or `libsql` | `sqlite` |
| `DATABASE_URL` | Database connection string | local file |
| `BACKUP_PASSWORD` | Password for backup encryption (AES-256-GCM) | unset (plaintext backups) |
| `SMTP_HOST`, `SMTP_PORT`, `SMTP_USERNAME`, `SMTP_PASSWORD`, `SMTP_TLS`, `SMTP_FROM_EMAIL`, `SMTP_FROM_NAME` | Email delivery configuration | unset |

Client-side (`search` CLI talking to a remote server):

| Variable | Description |
|----------|-------------|
| `SEARCH_SERVER` | Remote server base URL |
| `SEARCH_TOKEN` | Operator token for privileged CLI actions |

## CLI Flags

```bash
search --help

Usage: search [options]

Options:
  --help                 Show help
  --version              Show version
  --mode MODE            Application mode (production|development)
  --debug                Enable debug mode
  --config DIR           Config directory
  --data DIR             Data directory
  --log DIR              Log directory
  --address ADDR         Listen address
  --port PORT            Listen port
  --base-url URL         Public base URL override
  --lang CODE            Default language
  --color MODE           Color output (always|never|auto)
  --pid FILE             Write PID to file
  --status               Show server status
  --service CMD          Service management
  --maintenance CMD      Maintenance commands
  --update CMD           Update management
```

`search --help` is the authoritative and complete flag reference.
