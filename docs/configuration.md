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

Directory variables are checked at every startup, with CLI flags taking
precedence. Only the unprefixed names documented below are accepted by the
server. `PORT`, `LISTEN`, `APPLICATION_NAME`, and `APPLICATION_TAGLINE` seed
the configuration on first run and are ignored after `server.yml` exists.

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

Runtime settings are checked at each startup. Directory overrides are also
checked at every startup. Init-only values seed `server.yml` on first run.
CLI flags override environment values where both exist.

### Runtime variables

| Variable | Description | Default |
|----------|-------------|---------|
| `DOMAIN` | FQDN override | detected |
| `MODE` | `production`, `development`, or explicit `debug` mode | `production` |
| `DEBUG` | Enable debug features; accepts the truthy/falsy values in AI.md PART 5/6 | `false` |
| `DATABASE_DRIVER` | `sqlite`, `sqlite2`, `sqlite3`, `libsql`, or `turso` | `sqlite` |
| `DATABASE_URL` | Database connection string | local file |
| `SMTP_HOST` | SMTP server hostname; when set, skips autodetect | unset |
| `SMTP_PORT` | SMTP server port | `587` |
| `SMTP_USERNAME`, `SMTP_PASSWORD` | SMTP authentication | unset |
| `SMTP_FROM_NAME` | Sender name | application title |
| `SMTP_FROM_EMAIL` | Sender email | `no-reply@{fqdn}` |
| `SMTP_TLS` | `auto`, `starttls`, `tls`, or `none` | `auto` |

`DEBUG` controls diagnostics independently of `MODE`. `MODE=debug` enables
debug mode by default; an explicit `DEBUG=false` disables debug endpoints.

### Directory variables

These are checked at every startup. The matching CLI flag takes precedence.

| Variable | Description | Default |
|----------|-------------|---------|
| `CONFIG_DIR` | Configuration directory (`--config`) | platform default |
| `DATA_DIR` | Data directory (`--data`) | platform default |
| `LOG_DIR` | Log directory (`--log`) | platform default |
| `DATABASE_DIR` | SQLite database directory | platform default |
| `BACKUP_DIR` | Backup directory (`--backup`) | platform default |

Cache and PID paths are controlled by `--cache` and `--pid`; they do not have
environment-variable overrides.

### Init-only variables

These values are used only to seed a new `server.yml`:

| Variable | Description | Default |
|----------|-------------|---------|
| `PORT` | Listen port | random `64000-64999` |
| `LISTEN` | Listen address | all interfaces |
| `APPLICATION_NAME` | Application title | `Search` |
| `APPLICATION_TAGLINE` | Application description | project default |

### Backup password

`BACKUP_PASSWORD` supplies a backup-encryption password to non-interactive
CLI and scheduled-backup workflows. It is never written to `server.yml`.
Interactive CLI operations prompt for the password when no environment value
is set.

### Terminal variables

| Variable | Description |
|----------|-------------|
| `NO_COLOR` | Disable ANSI color output when set and non-empty |
| `TERM` | `dumb` disables ANSI output and selects CLI mode |

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


## Complete Configuration Defaults

The table below lists every YAML path emitted by the current `config.DefaultConfig()` implementation and its default value. Sequence items use `[index]` suffixes. Empty credentials and generated secrets are shown as empty or `<redacted>`; replace them only through the documented configuration or secret-management workflow. This index complements the operational explanations above and is refreshed whenever configuration defaults change.

| `server.yml` path | Default value |
|---|---|
| `data.cve.filter_by_cpe` | `false` |
| `data.cve.source` | `https://services.nvd.nist.gov/rest/json/cves/2.0` |
| `data.security_dir` | `/data/search/security` |
| `data.trivy.enabled` | `false` |
| `data.trivy.source` | `ghcr.io/aquasecurity/trivy-db:2` |
| `engines.bing.categories[0]` | `general` |
| `engines.bing.categories[1]` | `images` |
| `engines.bing.categories[2]` | `news` |
| `engines.bing.categories[3]` | `videos` |
| `engines.bing.enabled` | `true` |
| `engines.bing.priority` | `80` |
| `engines.bing.timeout` | `10` |
| `engines.bing.weight` | `1` |
| `engines.brave.categories[0]` | `general` |
| `engines.brave.categories[1]` | `images` |
| `engines.brave.categories[2]` | `news` |
| `engines.brave.enabled` | `true` |
| `engines.brave.priority` | `75` |
| `engines.brave.timeout` | `10` |
| `engines.brave.weight` | `1` |
| `engines.duckduckgo.categories[0]` | `general` |
| `engines.duckduckgo.categories[1]` | `images` |
| `engines.duckduckgo.enabled` | `true` |
| `engines.duckduckgo.priority` | `100` |
| `engines.duckduckgo.timeout` | `10` |
| `engines.duckduckgo.weight` | `1` |
| `engines.github.categories[0]` | `general` |
| `engines.github.categories[1]` | `it` |
| `engines.github.enabled` | `true` |
| `engines.github.priority` | `50` |
| `engines.github.timeout` | `10` |
| `engines.github.weight` | `0.8` |
| `engines.google.categories[0]` | `general` |
| `engines.google.categories[1]` | `images` |
| `engines.google.categories[2]` | `news` |
| `engines.google.categories[3]` | `videos` |
| `engines.google.enabled` | `true` |
| `engines.google.priority` | `90` |
| `engines.google.timeout` | `10` |
| `engines.google.weight` | `1` |
| `engines.reddit.categories[0]` | `general` |
| `engines.reddit.categories[1]` | `social` |
| `engines.reddit.enabled` | `false` |
| `engines.reddit.priority` | `45` |
| `engines.reddit.timeout` | `10` |
| `engines.reddit.weight` | `0.7` |
| `engines.stackoverflow.categories[0]` | `general` |
| `engines.stackoverflow.categories[1]` | `it` |
| `engines.stackoverflow.enabled` | `true` |
| `engines.stackoverflow.priority` | `55` |
| `engines.stackoverflow.timeout` | `10` |
| `engines.stackoverflow.weight` | `0.8` |
| `engines.wikipedia.categories[0]` | `general` |
| `engines.wikipedia.enabled` | `true` |
| `engines.wikipedia.priority` | `60` |
| `engines.wikipedia.timeout` | `10` |
| `engines.wikipedia.weight` | `0.5` |
| `engines.yahoo.categories[0]` | `general` |
| `engines.yahoo.categories[1]` | `images` |
| `engines.yahoo.categories[2]` | `news` |
| `engines.yahoo.enabled` | `true` |
| `engines.yahoo.priority` | `65` |
| `engines.yahoo.timeout` | `10` |
| `engines.yahoo.weight` | `1` |
| `search.alerts.create_rate_limit_per_hour` | `10` |
| `search.alerts.default_deliver_rss` | `true` |
| `search.alerts.default_deliver_webhook` | `false` |
| `search.alerts.default_frequency` | `daily` |
| `search.alerts.retention_days` | `30` |
| `search.alerts.webhook_max_retries` | `3` |
| `search.alerts.webhook_retry_delay_minutes` | `5` |
| `search.autocomplete` | `` |
| `search.bangs.enabled` | `true` |
| `search.bangs.proxy_requests` | `false` |
| `search.default_categories[0]` | `general` |
| `search.default_lang` | `en` |
| `search.max_concurrent` | `0` |
| `search.opensearch.contact` | `` |
| `search.opensearch.description` | `` |
| `search.opensearch.enabled` | `true` |
| `search.opensearch.image` | `/static/img/favicon.png` |
| `search.opensearch.long_name` | `` |
| `search.opensearch.short_name` | `` |
| `search.opensearch.tags` | `search privacy metasearch` |
| `search.results_per_page` | `100` |
| `search.safe_search` | `1` |
| `search.timeout` | `10` |
| `search.widgets.cache_ttl` | `300` |
| `search.widgets.crypto.currency` | `usd` |
| `search.widgets.crypto.default_coins[0]` | `bitcoin` |
| `search.widgets.crypto.default_coins[1]` | `ethereum` |
| `search.widgets.crypto.enabled` | `true` |
| `search.widgets.default_widgets[0]` | `clock` |
| `search.widgets.default_widgets[1]` | `weather` |
| `search.widgets.default_widgets[2]` | `quicklinks` |
| `search.widgets.default_widgets[3]` | `calculator` |
| `search.widgets.enabled` | `true` |
| `search.widgets.news.enabled` | `true` |
| `search.widgets.news.max_items` | `10` |
| `search.widgets.rss.enabled` | `true` |
| `search.widgets.rss.max_feeds` | `5` |
| `search.widgets.rss.max_items` | `10` |
| `search.widgets.sports.enabled` | `false` |
| `search.widgets.stocks.default_symbols[0]` | `AAPL` |
| `search.widgets.stocks.default_symbols[1]` | `GOOGL` |
| `search.widgets.stocks.default_symbols[2]` | `MSFT` |
| `search.widgets.stocks.enabled` | `true` |
| `search.widgets.weather.default_city` | `` |
| `search.widgets.weather.enabled` | `true` |
| `search.widgets.weather.units` | `imperial` |
| `server.address` | `[::]` |
| `server.api_version` | `v1` |
| `server.backup.disk_threshold` | `90` |
| `server.backup.encryption.enabled` | `false` |
| `server.backup.encryption.hint` | `` |
| `server.backup.retention.keep_monthly` | `0` |
| `server.backup.retention.keep_weekly` | `0` |
| `server.backup.retention.keep_yearly` | `0` |
| `server.backup.retention.max_backups` | `1` |
| `server.backup.retention.max_total_size` | `10%` |
| `server.baseurl` | `` |
| `server.branding.description` | `A privacy-respecting metasearch engine` |
| `server.branding.favicon` | `` |
| `server.branding.footer_text` | `` |
| `server.branding.logo` | `` |
| `server.branding.primary_color` | `#bd93f9` |
| `server.branding.source_code_url` | `` |
| `server.branding.tagline` | `Privacy-respecting metasearch` |
| `server.branding.theme` | `dark` |
| `server.branding.title` | `Search` |
| `server.cache.db` | `0` |
| `server.cache.host` | `localhost` |
| `server.cache.min_idle` | `2` |
| `server.cache.password` | `<redacted>` |
| `server.cache.pool_size` | `10` |
| `server.cache.port` | `6379` |
| `server.cache.prefix` | `search:` |
| `server.cache.timeout` | `5s` |
| `server.cache.tls_skip_verify` | `false` |
| `server.cache.tls` | `false` |
| `server.cache.ttl` | `3600` |
| `server.cache.type` | `memory` |
| `server.cache.url` | `` |
| `server.cache.username` | `` |
| `server.compliance.enabled` | `false` |
| `server.compression.brotli` | `false` |
| `server.compression.disable_proxy` | `false` |
| `server.compression.enabled` | `true` |
| `server.compression.gzip` | `true` |
| `server.compression.level` | `5` |
| `server.compression.min_size` | `1024` |
| `server.compression.types[0]` | `text/html` |
| `server.compression.types[1]` | `text/css` |
| `server.compression.types[2]` | `text/javascript` |
| `server.compression.types[3]` | `application/javascript` |
| `server.compression.types[4]` | `application/json` |
| `server.compression.types[5]` | `application/xml` |
| `server.compression.types[6]` | `text/xml` |
| `server.contact.abuse.email` | `` |
| `server.contact.admin.email` | `` |
| `server.contact.dpo.email` | `` |
| `server.contact.dpo.name` | `` |
| `server.contact.general.email` | `` |
| `server.contact.security.email` | `` |
| `server.daemonize` | `false` |
| `server.database.driver` | `sqlite` |
| `server.database.url` | `` |
| `server.debug.block_profile_rate` | `1` |
| `server.debug.log_bodies` | `false` |
| `server.debug.log_cache` | `true` |
| `server.debug.log_queries` | `true` |
| `server.debug.max_body_log_size` | `10KB` |
| `server.debug.mutex_profile_fraction` | `1` |
| `server.debug.pprof` | `true` |
| `server.debug.runtime_endpoints` | `true` |
| `server.description` | `Privacy-Respecting Metasearch Engine` |
| `server.fqdn` | `` |
| `server.geoip.databases.asn` | `true` |
| `server.geoip.databases.city` | `true` |
| `server.geoip.databases.country` | `true` |
| `server.geoip.dir` | `` |
| `server.geoip.enabled` | `true` |
| `server.group` | `search` |
| `server.healthz.root.enabled` | `false` |
| `server.https_port` | `0` |
| `server.i18n.auto_detect` | `true` |
| `server.i18n.default_language` | `en` |
| `server.i18n.enabled` | `true` |
| `server.i18n.fallback_language` | `en` |
| `server.i18n.rtl_languages[0]` | `ar` |
| `server.i18n.rtl_languages[1]` | `fa` |
| `server.i18n.rtl_languages[2]` | `he` |
| `server.i18n.rtl_languages[3]` | `ur` |
| `server.i18n.show_selector` | `true` |
| `server.i18n.available_languages[0]` | `en` |
| `server.i18n.available_languages[1]` | `ar` |
| `server.i18n.available_languages[2]` | `de` |
| `server.i18n.available_languages[3]` | `es` |
| `server.i18n.available_languages[4]` | `fa` |
| `server.i18n.available_languages[5]` | `fr` |
| `server.i18n.available_languages[6]` | `he` |
| `server.i18n.available_languages[7]` | `it` |
| `server.i18n.available_languages[8]` | `ja` |
| `server.i18n.available_languages[9]` | `nl` |
| `server.i18n.available_languages[10]` | `pl` |
| `server.i18n.available_languages[11]` | `pt` |
| `server.i18n.available_languages[12]` | `ru` |
| `server.i18n.available_languages[13]` | `ur` |
| `server.i18n.available_languages[14]` | `zh` |
| `server.i18n.translations_dir` | `/data/translations` |
| `server.i2p.binary` | `` |
| `server.i2p.enabled` | `false` |
| `server.i2p.inbound_length` | `3` |
| `server.i2p.inbound_quantity` | `5` |
| `server.i2p.outbound_length` | `3` |
| `server.i2p.outbound_quantity` | `5` |
| `server.i2p.sam_address` | `127.0.0.1:7656` |
| `server.i2p.virtual_port` | `80` |
| `server.image_proxy.enabled` | `true` |
| `server.image_proxy.max_size` | `10485760` |
| `server.image_proxy.timeout` | `30` |
| `server.limits.idle_timeout` | `120s` |
| `server.limits.max_body_size` | `10MB` |
| `server.limits.read_timeout` | `30s` |
| `server.limits.write_timeout` | `30s` |
| `server.logs.access.custom` | `` |
| `server.logs.access.filename` | `access.log` |
| `server.logs.access.format` | `apache` |
| `server.logs.access.keep` | `none` |
| `server.logs.access.log_health_checks` | `false` |
| `server.logs.access.rotate` | `monthly` |
| `server.logs.audit.custom` | `` |
| `server.logs.audit.filename` | `audit.log` |
| `server.logs.audit.format` | `json` |
| `server.logs.audit.keep` | `none` |
| `server.logs.audit.rotate` | `weekly,50MB` |
| `server.logs.debug.custom` | `` |
| `server.logs.debug.enabled` | `false` |
| `server.logs.debug.filename` | `debug.log` |
| `server.logs.debug.format` | `text` |
| `server.logs.debug.keep` | `none` |
| `server.logs.debug.rotate` | `weekly,50MB` |
| `server.logs.error.custom` | `` |
| `server.logs.error.filename` | `error.log` |
| `server.logs.error.format` | `text` |
| `server.logs.error.keep` | `none` |
| `server.logs.error.rotate` | `weekly,50MB` |
| `server.logs.file` | `` |
| `server.logs.format` | `` |
| `server.logs.level` | `warn` |
| `server.logs.security.custom` | `` |
| `server.logs.security.filename` | `security.log` |
| `server.logs.security.format` | `fail2ban` |
| `server.logs.security.keep` | `none` |
| `server.logs.security.rotate` | `weekly,50MB` |
| `server.logs.server.custom` | `` |
| `server.logs.server.filename` | `server.log` |
| `server.logs.server.format` | `text` |
| `server.logs.server.keep` | `none` |
| `server.logs.server.rotate` | `weekly,50MB` |
| `server.maintenance.cleanup.backup_keep_count` | `5` |
| `server.maintenance.cleanup.disk_threshold` | `90` |
| `server.maintenance.cleanup.log_retention_days` | `7` |
| `server.maintenance.notify.on_enter` | `true` |
| `server.maintenance.notify.on_exit` | `true` |
| `server.maintenance.self_healing.enabled` | `true` |
| `server.maintenance.self_healing.max_attempts` | `0` |
| `server.maintenance.self_healing.retry_interval` | `30s` |
| `server.maintenance_mode` | `false` |
| `server.mcp.enabled` | `true` |
| `server.metrics.auth.allow_unauthenticated` | `false` |
| `server.metrics.auth.tokens.grafana` | `<redacted>` |
| `server.metrics.auth.tokens.loki` | `<redacted>` |
| `server.metrics.auth.tokens.prometheus` | `<redacted>` |
| `server.metrics.enabled` | `true` |
| `server.metrics.endpoint` | `/server/metrics` |
| `server.metrics.include_runtime` | `true` |
| `server.metrics.include_system` | `true` |
| `server.metrics.loki.max_age` | `1h` |
| `server.metrics.loki.max_entries` | `1000` |
| `server.metrics.root.enabled` | `true` |
| `server.mode` | `production` |
| `server.notifications.email.events.backup_complete` | `false` |
| `server.notifications.email.events.backup_failed` | `true` |
| `server.notifications.email.events.scheduler_error` | `true` |
| `server.notifications.email.events.security_alert` | `true` |
| `server.notifications.email.events.shutdown` | `false` |
| `server.notifications.email.events.ssl_expiring` | `true` |
| `server.notifications.email.events.ssl_renewal_failed` | `true` |
| `server.notifications.email.events.ssl_renewed` | `false` |
| `server.notifications.email.events.startup` | `false` |
| `server.notifications.email.events.update_available` | `false` |
| `server.notifications.email.events.update_installed` | `true` |
| `server.notifications.email.from.email` | `` |
| `server.notifications.email.from.name` | `` |
| `server.notifications.email.smtp.host` | `` |
| `server.notifications.email.smtp.password` | `<redacted>` |
| `server.notifications.email.smtp.port` | `587` |
| `server.notifications.email.smtp.tls` | `auto` |
| `server.notifications.email.smtp.username` | `` |
| `server.notifications.webui.duration` | `5` |
| `server.notifications.webui.position` | `top-right` |
| `server.pages.about.content` | `` |
| `server.pages.about.enabled` | `true` |
| `server.pages.contact.captcha_secret_key` | `<redacted>` |
| `server.pages.contact.captcha_site_key` | `<redacted>` |
| `server.pages.contact.captcha` | `simple` |
| `server.pages.contact.email` | `` |
| `server.pages.contact.enabled` | `true` |
| `server.pages.contact.success_message` | `Thank you for your message. We'll respond soon.` |
| `server.pages.help.content` | `` |
| `server.pages.help.enabled` | `true` |
| `server.pages.help.mcp` | `## Search tools

The live tools above are generated from Search's OpenAPI operations. \`getApiV1Search\` and \`postApiV1Search\` search across enabled engines; \`getApiV1Autocomplete\` suggests queries; \`getApiV1Instant\` returns an instant answer; \`getApiV1Categories\` and \`getApiV1Engines\` describe available sources.

Example prompt: Search for recent privacy-preserving search engine projects, then summarize the distinct results and identify their source engines.

Public search tools work without authentication. Create a linked token in Search preferences only when the assistant needs operator-only tools; never use the server operator token.` |
| `server.pages.privacy.content` | `` |
| `server.pages.privacy.enabled` | `true` |
| `server.pages.terms.content` | `` |
| `server.pages.terms.enabled` | `false` |
| `server.pidfile` | `true` |
| `server.port` | `0` |
| `server.privacy.consent.buttons.accept` | `I Agree` |
| `server.privacy.consent.buttons.decline` | `Decline` |
| `server.privacy.consent.default_enabled` | `true` |
| `server.privacy.consent.message_if_sold` | `In accordance with the EU GDPR law this message is being displayed. We use cookies for essential site functionality and, with your consent, for preferences and analytics. Your data may be shared with or sold to third parties as described in our Privacy Policy.` |
| `server.privacy.consent.message` | `In accordance with the EU GDPR law this message is being displayed. We use cookies for essential site functionality and, with your consent, for preferences and analytics. Your data is stored on our servers and is never sold.` |
| `server.privacy.consent.policy.text` | `Privacy Policy` |
| `server.privacy.consent.policy.url` | `/server/privacy` |
| `server.privacy.consent.position` | `bottom` |
| `server.privacy.consent.preferences_text` | `Manage Preferences` |
| `server.privacy.consent.show_preferences` | `true` |
| `server.privacy.consent.show_until_acknowledged` | `true` |
| `server.privacy.content.data_collection` | `**We collect only what is necessary to provide our service:**

**Usage Information (with consent):**
- Pages visited and features used
- Browser type and device information
- Approximate location (country/region from IP, not precise)

**Technical Information:**
- IP address (for security and abuse prevention)
- API token identifiers (hashed, never stored in plain text)

**We do NOT collect:**
- Payment information (unless explicitly required by the service)
- Precise location data
- Data from other websites or apps
` |
| `server.privacy.content.data_security` | `**How we protect your data:**

- All data is stored on our servers (not third-party cloud services unless specified)
- API tokens are stored as SHA-256 hashes (never in plain text)
- All connections are encrypted (HTTPS/TLS)
- Regular security audits and updates
- Access controls and audit logging for operator actions
` |
| `server.privacy.content.data_usage_if_sold` | `**Your data may be used to:**

- **Provide the service:** Authentication, core functionality
- **Improve the experience:** Performance optimization, bug fixes, feature improvements
- **Ensure security:** Prevent abuse, detect fraud, protect your account
- **Communicate:** Service updates, security alerts, and (with consent) product news
- **Third-party sharing:** Your data may be shared with or sold to third parties for analytics, advertising, or other purposes as described below

**Your rights:**
- You can opt out of data sales via your account privacy settings
- You can request deletion of your data at any time
- See "Your Rights" section below for details
` |
| `server.privacy.content.data_usage` | `**Your data is used solely to:**

- **Provide the service:** Authentication, core functionality
- **Improve the experience:** Performance optimization, bug fixes, feature improvements
- **Ensure security:** Prevent abuse, detect fraud, protect your account
- **Communicate:** Service updates, security alerts, and (with consent) product news

**Your data is NEVER:**
- Sold to third parties
- Used for targeted advertising
- Shared without your explicit consent (except as required by law)
` |
| `server.privacy.cookies.analytics.description_suffix_not_sold` | `Analytics data is anonymized and never sold.` |
| `server.privacy.cookies.analytics.description_suffix_sold` | `Analytics data may be shared with third parties.` |
| `server.privacy.cookies.analytics.description` | `Help us understand how visitors use our site to improve the experience.` |
| `server.privacy.cookies.analytics.enabled` | `true` |
| `server.privacy.cookies.essential.description` | `Required for the site to function. Includes security tokens (CSRF) and site preferences. These cookies are strictly necessary and cannot be disabled.` |
| `server.privacy.cookies.essential.enabled` | `true` |
| `server.privacy.cookies.preferences.description` | `Remember your settings such as theme (dark/light), language, and UI preferences. Disabling will reset to defaults on each visit.` |
| `server.privacy.cookies.preferences.enabled` | `true` |
| `server.privacy.data.sharing[0].condition` | `analytics` |
| `server.privacy.data.sharing[0].data` | `Anonymized: page views, browser type, country` |
| `server.privacy.data.sharing[0].when` | `Tracking configured (server.tracking.type set) AND user consents` |
| `server.privacy.data.sharing[1].condition` | `email` |
| `server.privacy.data.sharing[1].data` | `Email address, message content` |
| `server.privacy.data.sharing[1].when` | `SMTP configured for sending emails` |
| `server.privacy.data.sharing[2].condition` | `user_initiated` |
| `server.privacy.data.sharing[2].data` | `Whatever user chooses to share` |
| `server.privacy.data.sharing[2].when` | `User explicitly shares content (social buttons, exports)` |
| `server.privacy.data.sold` | `false` |
| `server.privacy.data.stored_on_server` | `true` |
| `server.privacy.retention.deletion_available` | `true` |
| `server.privacy.retention.export_available` | `true` |
| `server.privacy.retention.period` | `Account data is retained while your account is active. Upon account deletion, all personal data is permanently deleted within 30 days. Anonymized analytics data may be retained for up to 12 months.` |
| `server.rate_limit.enabled` | `true` |
| `server.rate_limit.global_burst` | `240` |
| `server.rate_limit.health.requests` | `120` |
| `server.rate_limit.health.window` | `60` |
| `server.rate_limit.read.requests` | `120` |
| `server.rate_limit.read.window` | `60` |
| `server.rate_limit.write.requests` | `10` |
| `server.rate_limit.write.window` | `60` |
| `server.scheduler.catch_up_window` | `1h` |
| `server.scheduler.tasks.backup_daily.enabled` | `true` |
| `server.scheduler.tasks.backup_daily.schedule` | `0 2 * * *` |
| `server.scheduler.tasks.backup_hourly.enabled` | `false` |
| `server.scheduler.tasks.backup_hourly.schedule` | `@hourly` |
| `server.scheduler.tasks.blocklist_update.enabled` | `true` |
| `server.scheduler.tasks.blocklist_update.schedule` | `0 4 * * *` |
| `server.scheduler.tasks.cve_update.enabled` | `true` |
| `server.scheduler.tasks.cve_update.schedule` | `0 5 * * *` |
| `server.scheduler.tasks.geoip_update.enabled` | `true` |
| `server.scheduler.tasks.geoip_update.schedule` | `0 3 * * 0` |
| `server.scheduler.tasks.update_check.enabled` | `true` |
| `server.scheduler.tasks.update_check.schedule` | `0 6 * * *` |
| `server.scheduler.timezone` | `America/New_York` |
| `server.secret_key` | `<redacted>` |
| `server.security.csrf.cookie_name` | `csrf_token` |
| `server.security.csrf.enabled` | `true` |
| `server.security.csrf.field_name` | `csrf_token` |
| `server.security.csrf.header_name` | `X-CSRF-Token` |
| `server.security.csrf.secure` | `auto` |
| `server.security.csrf.token_length` | `32` |
| `server.security.encryption_key_previous_expires` | `` |
| `server.security.encryption_key_previous` | `` |
| `server.security.encryption_key` | `<redacted>` |
| `server.security.headers.clear_site_data.execution_contexts` | `false` |
| `server.security.headers.clear_site_data.on_consent_withdrawal` | `true` |
| `server.security.headers.clear_site_data.on_token_revocation` | `true` |
| `server.security.headers.coep` | `unsafe-none` |
| `server.security.headers.content_security_policy` | `default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob: https:; font-src 'self' https:; connect-src 'self'; media-src 'self' blob:; worker-src 'self' blob:; manifest-src 'self'; frame-src 'self'; frame-ancestors 'self'; base-uri 'self'; form-action 'self'; object-src 'none'; upgrade-insecure-requests; report-to default; report-uri /api/v1/server/reports/csp` |
| `server.security.headers.coop` | `unsafe-none` |
| `server.security.headers.corp` | `cross-origin` |
| `server.security.headers.cross_domain_policies` | `none` |
| `server.security.headers.origin_agent_cluster` | `true` |
| `server.security.headers.permissions_policy` | `accelerometer=(), ambient-light-sensor=(), attribution-reporting=(), battery=(), browsing-topics=(), camera=(), display-capture=(), geolocation=(self), gyroscope=(), hid=(), idle-detection=(), interest-cohort=(), magnetometer=(), microphone=(), midi=(), screen-wake-lock=(), serial=(), usb=(), xr-spatial-tracking=(), autoplay=(self), encrypted-media=(self), fullscreen=(self), payment=(self), picture-in-picture=(self), publickey-credentials-get=(self), storage-access=(self), web-share=(self)` |
| `server.security.headers.referrer_policy` | `strict-origin-when-cross-origin` |
| `server.security.headers.sec_fetch_validation` | `true` |
| `server.security.headers.x_content_type_options` | `nosniff` |
| `server.security.headers.x_frame_options` | `SAMEORIGIN` |
| `server.security.headers.x_xss_protection` | `1; mode=block` |
| `server.security.hsts.enabled` | `true` |
| `server.security.hsts.include_subdomains` | `true` |
| `server.security.hsts.max_age_seconds` | `63072000` |
| `server.security.hsts.preload` | `true` |
| `server.security.installation_secret_previous_expires` | `<redacted>` |
| `server.security.installation_secret_previous` | `<redacted>` |
| `server.security.installation_secret` | `<redacted>` |
| `server.security.nel.enabled` | `true` |
| `server.security.nel.include_subdomains` | `true` |
| `server.security.nel.max_age_seconds` | `2592000` |
| `server.security.nel.sample_rate` | `1` |
| `server.seo.canonical` | `true` |
| `server.seo.default_description` | `A privacy-respecting metasearch engine` |
| `server.seo.default_title` | `Search` |
| `server.seo.enabled` | `true` |
| `server.seo.keywords[0]` | `search` |
| `server.seo.keywords[1]` | `privacy` |
| `server.seo.keywords[2]` | `metasearch` |
| `server.seo.noindex` | `true` |
| `server.seo.opengraph.description` | `` |
| `server.seo.opengraph.enabled` | `true` |
| `server.seo.opengraph.image` | `` |
| `server.seo.opengraph.site_name` | `Search` |
| `server.seo.opengraph.type` | `website` |
| `server.seo.sitemap` | `false` |
| `server.seo.title_separator` | ` - ` |
| `server.seo.twitter.card` | `summary` |
| `server.seo.twitter.creator` | `` |
| `server.seo.twitter.enabled` | `true` |
| `server.seo.twitter.site` | `` |
| `server.ssl.auto_tls` | `false` |
| `server.ssl.cert` | `` |
| `server.ssl.dns01.credentials_encrypted` | `<redacted>` |
| `server.ssl.dns01.provider` | `` |
| `server.ssl.dns01.validated_at` | `` |
| `server.ssl.enabled` | `false` |
| `server.ssl.key` | `<redacted>` |
| `server.ssl.letsencrypt.challenge` | `` |
| `server.ssl.letsencrypt.email` | `` |
| `server.ssl.letsencrypt.enabled` | `false` |
| `server.ssl.letsencrypt.staging` | `false` |
| `server.ssl.min_version` | `TLS1.2` |
| `server.title` | `Search` |
| `server.token` | `<redacted>` |
| `server.tokens.cookie_max_age` | `1y` |
| `server.tokens.idle_purge_days` | `0` |
| `server.tokens.max_linked` | `25` |
| `server.tor.allow_user_preference` | `true` |
| `server.tor.bandwidth_burst` | `2 MB` |
| `server.tor.bandwidth_rate` | `1 MB` |
| `server.tor.binary` | `` |
| `server.tor.bootstrap_timeout` | `180` |
| `server.tor.circuit_timeout` | `60` |
| `server.tor.close_circuit_on_stream_limit` | `true` |
| `server.tor.contact_email` | `` |
| `server.tor.max_circuits` | `32` |
| `server.tor.max_monthly_bandwidth` | `100 GB` |
| `server.tor.max_streams_per_circuit` | `100` |
| `server.tor.num_intro_points` | `3` |
| `server.tor.safe_logging` | `true` |
| `server.tor.use_network` | `false` |
| `server.tor.virtual_port` | `80` |
| `server.tracking.id` | `` |
| `server.tracking.type` | `` |
| `server.tracking.url` | `` |
| `server.update.auto_install` | `false` |
| `server.update.branch` | `stable` |
| `server.update.defer_days` | `0` |
| `server.user` | `search` |
| `server.web.announcements.enabled` | `true` |
| `server.web.cookie_consent.enabled` | `false` |
| `server.web.cookie_consent.message` | `This site uses cookies for functionality and analytics.` |
| `server.web.cookie_consent.policy_url` | `/server/privacy` |
| `server.web.cors` | `*` |
| `server.web.robots.allow[0]` | `/` |
| `server.web.robots.allow[1]` | `/api` |
| `server.web.security.contact` | `` |
| `server.web.security.expires` | `2027-11-07T12:09:08Z` |
| `server.web.security.keyservers[0]` | `https://keys.openpgp.org` |
| `server.web.security.publish_pgp_key` | `<redacted>` |
| `server.web.security.report_url` | `https://github.com/apimgr/search/security/advisories/new` |
| `tor.contact_email` | `` |
| `tor.onion_address` | `` |
| `web.csp.frame_src_extra` | `` |
