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
- macOS (root): `/Library/Application Support/apimgr/search/server.yml`
- macOS (user): `~/Library/Application Support/apimgr/search/server.yml`
- Windows (administrator): `%ProgramData%\apimgr\search\server.yml`
- Windows (user): `%AppData%\apimgr\search\server.yml`
- FreeBSD (root): `/usr/local/etc/apimgr/search/server.yml`
- FreeBSD (user): `~/.config/apimgr/search/server.yml`
- Docker: `/config/search/server.yml`

The file is auto-generated with defaults on first run. There is no admin web UI — all configuration is file-only.

The native configuration roots are `server`, `search`, `engines`, `data`,
`web`, and `tor`. Server settings are grouped under `server`: `tokens`, `mcp`,
`branding`, `rate_limit`,
`logs`, `ssl`, `tor`, `i2p`, `notifications`, `security`, `auth`, `pages`,
`privacy`, `scheduler`, `cache`, `geoip`, `metrics`, `image_proxy`, `contact`,
`tracking`, `seo`, `compression`, `trusted_proxies`, `limits`, `i18n`,
`healthz`, `backup`, `compliance`, `database`, `maintenance`, `update`, and
`debug`. The sections below document settings with additional operational
requirements; the server validates all configuration on load and reports
invalid values without exposing secrets.

The directory paths can be overridden with `CONFIG_DIR`, `DATA_DIR`,
`LOG_DIR`, `DATABASE_DIR`, `BACKUP_DIR`, or `SEARCH_CACHE_DIR`; most also
accept the corresponding `SEARCH_`-prefixed name. See the environment table
below for the exact supported names.

### Reddit Search

Reddit search uses Reddit's OAuth API and is disabled by default. To enable it,
register an application with Reddit and configure its credentials and your
Reddit username in the top-level `engines` section:

```yaml
engines:
  reddit:
    enabled: true
    client_id: ""
    client_secret: ""
    contact_username: ""
```

The username is sent in the API User-Agent as Reddit's contact identifier.
Reddit API access is subject to Reddit's terms; the free API allowance is for
eligible non-commercial use, and commercial use requires a separate agreement.
Search requests use the authenticated API only.

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

  # Public URL path prefix for the application
  baseurl: ""

  # Fully qualified domain name (auto-detected when empty)
  fqdn: ""

  # Operator token (auto-generated on first run)
  token: ""
```

### SSL/TLS Settings

```yaml
server:
  ssl:
    enabled: false
    cert: ""
    key: ""
    min_version: TLS1.2
    auto_tls: false
    letsencrypt:
      enabled: false
      email: "operator@example.com"
      domains:
        - "search.example.com"
      staging: false
      challenge: http-01
    dns01:
      provider: ""
      credentials_encrypted: ""
```

### Rate Limiting

```yaml
server:
  rate_limit:
    enabled: true
    read:
      requests: 120
      window: 60
    write:
      requests: 10
      window: 60
    health:
      requests: 120
      window: 60
    global_burst: 240
```

### Logging

```yaml
server:
  logs:
    # debug, info, warn, or error
    level: info
    format: logfmt
    access:
      filename: access.log
      format: combined
      log_health_checks: false
    server:
      filename: server.log
      format: logfmt
    error:
      filename: error.log
      format: iso8601
    audit:
      filename: audit.log
      format: json
    security:
      filename: security.log
      format: json
```

Access logs record the request path without its query string or the client IP.
Successful health checks are omitted unless `log_health_checks` is enabled.
Debug-only request-body and SQL logging can expose sensitive data; enable debug
logging only in a controlled environment.

### Tor Hidden Service

```yaml
server:
  tor:
    # Auto-enabled when tor binary is found on PATH
    use_network: false
```

### Contact Form CAPTCHA

The contact form uses the built-in challenge by default. To use reCAPTCHA or
hCaptcha after a failed submission, set its public site key and private server
key in `server.yml`:

```yaml
server:
  pages:
    contact:
      # simple, recaptcha, or hcaptcha
      captcha: hcaptcha
      captcha_site_key: ""
      captcha_secret_key: ""
```

Provider modes fail closed when either key is missing. Keep
`captcha_secret_key` private and restrict access to the configuration file.
Requests through the Tor hidden service use the built-in challenge and never
send verification traffic to a clearnet provider.

### Contact Notification Webhooks

Contact notifications support `telegram`, `discord`, `slack`, `mattermost`,
`pushover`, `gotify`, and `generic` transports for the `admin`, `security`,
`abuse`, and `general` roles. Configure them with the operator-authenticated
`POST /api/v1/server/config/contact/webhooks` endpoint using `role`,
`transport`, and an HTTPS `url`. The generated signing secret is returned once
when a URL is created or changed; save it in the receiving service. Later saves
do not reveal it again. Requests are signed with HMAC-SHA256 and retried after
1 minute, 5 minutes, 15 minutes, 1 hour, 6 hours, and 24 hours.

### Search Settings

```yaml
search:
  # Safe search: 0 (off), 1 (moderate), or 2 (strict)
  safe_search: 1

  # Empty uses the built-in autocomplete provider
  autocomplete: ""

  # Default search language and categories
  default_lang: en
  default_categories:
    - general

  # Results per page for server-rendered searches
  results_per_page: 100

  # Search timeout in seconds
  timeout: 10

  # Zero queries every enabled engine for the selected category
  max_concurrent: 0

server:
  cache:
    type: memory
    # Default cache lifetime in seconds
    ttl: 300
```

The REST API uses its own `limit` parameter (default 20, maximum 100); it does
not inherit `search.results_per_page`.

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

### Metrics

```yaml
server:
  metrics:
    enabled: true
    root:
      enabled: true
    auth:
      allow_unauthenticated: false
      tokens:
        prometheus: ""
        grafana: ""
        loki: ""
    include_system: true
    include_runtime: true
    loki:
      max_entries: 1000
      max_age: 1h
```

Metrics are enabled by default, but each service is disabled until its own
bearer token is configured. Set a strong token for each service you use and
send it only in `Authorization: Bearer ...`; query-string tokens are rejected.
The root `/metrics` aliases are enabled by default. Set
`server.metrics.root.enabled: false` to disable them. The
`allow_unauthenticated` option is only for a firewalled internal network.

See [Integrations](integrations.md) for a Prometheus scrape example.

### Image Proxy

```yaml
server:
  image_proxy:
    enabled: true
    # Maximum image size in bytes (10 MiB)
    max_size: 10485760
    timeout: 30
```

Search and Wikipedia thumbnails use signed same-origin `/image-proxy` URLs when enabled, so the endpoint cannot be used as an open relay. It accepts public HTTPS sources on port 443, rejects redirects and private or reserved IP addresses, and only serves validated PNG, JPEG, GIF, or WebP images. The response limit is capped at 10 MiB and the request timeout at 30 seconds; the browser referrer is never forwarded to the image host.

## Environment Variables

Server settings can be set via environment variables. Canonical names follow
AI.md PART 5 (unprefixed); a few also accept a `SEARCH_` prefix as noted.
Run `search --help` for the authoritative, complete list.

| Variable | Description | Default |
|----------|-------------|---------|
| `PORT` | Listen port | random `64000-64999` |
| `LISTEN` | Listen address | all interfaces |
| `MODE` | Application mode (`production`/`development`) | `production` |
| `DEBUG` (or `SEARCH_DEBUG`) | Enable debug mode (`0`/`1`, `true`/`false`) | `false` |
| `SEARCH_BASE_URL` | Public base URL / path prefix override | derived |
| `SEARCH_COLOR` | Color output mode (`always`/`never`/`auto`) | `auto` |
| `PID_FILE` (or `SEARCH_PID_FILE`) | Path to PID file | platform default |
| `DOMAIN` | FQDN override | detected |
| `DATABASE_DRIVER` | `sqlite` or `libsql` | `sqlite` |
| `DATABASE_URL` | Database connection string | local file |
| `BACKUP_PASSWORD` | Password for backup encryption (AES-256-GCM) | unset (plaintext backups) |
| `SMTP_HOST`, `SMTP_PORT`, `SMTP_USERNAME`, `SMTP_PASSWORD`, `SMTP_TLS`, `SMTP_FROM_EMAIL`, `SMTP_FROM_NAME` | Email delivery configuration | unset |

Directory overrides (each accepts the unprefixed canonical name or a `SEARCH_`
prefix, except `SEARCH_CACHE_DIR` which is prefixed only):

| Variable | Description | Default |
|----------|-------------|---------|
| `CONFIG_DIR` (or `SEARCH_CONFIG_DIR`) | Configuration directory | platform default |
| `DATA_DIR` (or `SEARCH_DATA_DIR`) | Data directory | platform default |
| `LOG_DIR` (or `SEARCH_LOG_DIR`) | Log directory | platform default |
| `DATABASE_DIR` (or `SEARCH_DATABASE_DIR`) | SQLite database directory | platform default |
| `BACKUP_DIR` (or `SEARCH_BACKUP_DIR`) | Backup directory | platform default |
| `SEARCH_CACHE_DIR` | Cache directory | platform default |

Client-side (`search-cli` talking to a remote server):

| Variable | Description |
|----------|-------------|
| `SEARCH_SERVER_PRIMARY` | Remote server base URL |
| `SEARCH_TOKEN` | Operator token for privileged CLI actions |
| `SEARCH_SERVER_TIMEOUT` | Request timeout in seconds |
| `SEARCH_OUTPUT_FORMAT` | Default output format (e.g. `table`, `json`) |

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
  --baseurl URL          Public base URL override
  --lang CODE            Default language
  --color MODE           Color output (always|never|auto)
  --pid FILE             Write PID to file
  --status               Show server status
  --service CMD          Service management
  --maintenance CMD      Maintenance commands
  --update CMD           Update management
```

`search --help` is the authoritative and complete flag reference.
