# Security

Search is designed with security and privacy as primary concerns. This document outlines the security model and best practices.

## Security Model

### Authentication

- **Operator token** (`server.token` in `secrets.yml`) — auto-generated on first run; gates operator management endpoints
- **Per-resource tokens** — for search alerts and webhooks; stored as SHA-256 hashes
- **Owner and linked tokens** — owner-scoped credentials for a person's resources and their automation clients
- **Bearer token authentication** for all protected API access
- **CSRF protection** on all cookie-authenticated browser forms
- **Rate limiting** to prevent abuse

There is no admin web UI, no login form, no session management, and no user accounts. Configuration is entirely file-based via `server.yml`.

### Transport Security

- **TLS/SSL** encryption
- **Let's Encrypt** integration (HTTP-01, TLS-ALPN-01, DNS-01)
- **HSTS** headers when SSL is enabled
- **Secure cookies** with HttpOnly and SameSite=Strict flags

### Security Headers

Search sets the following security headers on every response:

| Header | Value |
|--------|-------|
| `X-Frame-Options` | `SAMEORIGIN` |
| `X-Content-Type-Options` | `nosniff` |
| `X-XSS-Protection` | `1; mode=block` |
| `Referrer-Policy` | `strict-origin-when-cross-origin` |
| `Content-Security-Policy` | Restrictive policy |
| `Permissions-Policy` | All sensors locked; tracking proposals locked |

### Privacy

- **Privacy-aware access logs** — paths are logged without query strings or client IPs
- **Security and audit logs** — selected abuse and authentication events include a client IP and may include a user agent for operator investigation
- **Debug logging** — request-body and SQL logging can expose sensitive data and should only be enabled in a controlled environment
- **No user tracking** — no analytics, no fingerprinting
- **Image proxy** to prevent third-party tracking of search results
- **Encrypted backups** (AES-256-GCM, Argon2id KDF)

## Best Practices

### Enable SSL

Always use SSL in production:

```yaml
server:
  ssl:
    enabled: true
    letsencrypt:
      enabled: true
      email: "operator@example.com"
      domains:
        - "search.example.com"
```

### Operator Token

The operator token is auto-generated on first run and stored in `secrets.yml` with mode `0600`. Keep the secrets file private:

```bash
chmod 600 /etc/apimgr/search/secrets.yml
```

### Rate Limiting

Enable and configure rate limiting:

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

### IP and Domain Blocklists

Missing enabled blocklist and CVE data is queued for a first-run refresh after the HTTP listener is ready; the refresh does not delay startup. GeoIP databases are loaded or downloaded during initialization and refreshed weekly by the built-in scheduler; blocklists refresh daily. The built-in `firehol_level1` source always filters IP addresses and CIDR ranges. Add operator-managed feeds under `server.security.blocklists.extra_sources` in `server.yml`:

```yaml
server:
  security:
    blocklists:
      enabled: true
      auto_update: true
      extra_sources:
        - id: phishing_domains
          type: domain
          url: https://feeds.example.net/domains.txt
          enabled: true
```

The built-in `firehol_level1` source is always active and cannot be disabled or removed. Additional sources use `id`, `type`, `url`, and `enabled`: `id` is a unique lowercase identifier; `type` is `ip` (an IPv4 or IPv6 address or CIDR per line) or `domain` (a lowercase hostname per line); and `url` must use HTTPS. Domain entries may use a leading `*.` to match subdomains. Invalid lines are skipped and counted, but an update with no valid entries is rejected and leaves the previous file in use. Each source has its own last-success timestamp and health check; sources older than the fixed 72-hour freshness limit report `degraded`, while sources that have never loaded report `error`.

### Regular Updates

Keep Search updated to receive security patches:

```bash
search --update yes
```

### Firewall Configuration

Metrics are internal operational endpoints. Do not proxy `/server/metrics`,
`/api/metrics`, `/api/v1/server/metrics`, or their `/metrics` aliases to the
public internet. Restrict them at the reverse proxy or machine firewall to the
monitoring network. Each service also requires its own bearer token; token
authentication is defense in depth and does not replace network restrictions.

## Public Security Endpoints

The following endpoints are public (no authentication) and support responsible
disclosure and health monitoring:

| Endpoint | Purpose |
|----------|---------|
| `/.well-known/security.txt` | RFC 9116 security contact and policy |
| `/.well-known/pgp-key.asc` | PGP public key (returns `404` until a keypair exists) |
| `/.well-known/llms.txt` | Machine-readable summary for LLM agents |
| `/.well-known/change-password` | `404` because this project has no user passwords or account system |
| `/server/security` | Human-readable security overview and how to report |
| `/server/security/policy` | Full security/disclosure policy |
| `/server/security/thanks` | Publicly credited, completed disclosures |
| `/api/v1/server/security` | Security contact order, expiry, PGP metadata, and policy/thanks links (JSON) |
| `/api/v1/server/security/policy` | Configured disclosure policy Markdown (JSON) |
| `/api/v1/server/security/thanks` | Opt-in credits for disclosed reports (JSON) |
| `/server/contact` | Contact form (supports `security_id` deep links) |
| `/server/healthz` | Public health check |
| `/api/v1/server/healthz` | Public health check (JSON) |

The health response includes the NVD vulnerability dataset when its feature is
enabled. It reports `degraded` when the last successful update is more than
72 hours old; the scheduler attempts an update daily.

The contact form includes a honeypot and requires a single-use CAPTCHA on every
submission. The default `simple` challenge is served locally and works without
JavaScript, including over Tor. Operators may configure reCAPTCHA or hCaptcha
with their site and secret keys; secret keys stay server-side. Tor requests use
the built-in challenge even when an external provider is configured.

## Well-Known Namespace

Search serves a fixed set of `/.well-known/*` entries (listed above). Any
unrecognized `/.well-known/*` path returns `404`. Feature-gated entries such as
WebFinger, OpenID Provider Metadata, App Links, or Apple app-site-association are
**not** enabled for this project and are not served.

## Security Reporting

If you discover a security vulnerability, please report it responsibly:

1. **Do not** disclose publicly until fixed
2. Retrieve the security contact from `/.well-known/security.txt`
3. Use `/server/security` for a human-readable overview and reporting steps
4. Deep-link a specific report via `/server/contact?security_id=<id>`
5. Include steps to reproduce and allow time for a fix before disclosure

The full disclosure policy is published at `/server/security/policy`.

The default security.txt contact is the project's GitHub private
vulnerability-reporting page. Operators can set `server.web.security.report_url`
and `server.web.security.contact`. PGP publication is controlled by
`server.web.security.publish_pgp_key`; manage the keypair with
`search --maintenance pgp generate`, `publish`, or `export public`.
Set `server.web.security.policy` to Markdown to replace the built-in disclosure
policy shown at `/server/security/policy` and returned by its API endpoint.

## Security Logs

Rate-limit violations, blocked requests, CSRF violations, and authentication
events are recorded in security or audit logs. These event logs can include a
client IP and, for authentication attempts, a user agent. Routine access logs
omit both the client IP and query string. Debug-only request-body and SQL
logging can expose sensitive data and should only be enabled in a controlled
environment.

## Tor Hidden Service

For enhanced privacy, Search automatically starts and manages an isolated Tor
process for its hidden service when a Tor binary is found on `PATH` or configured
in `server.tor.binary`. It does not attach to or manage the machine's system Tor
daemon. Install the Tor binary on the deployment machine yourself:

Install Tor on the deployment machine yourself; do not have an AI agent execute
the package-manager command below.

```bash
# Install Tor
apt-get install tor

# Start Search — it launches its own Tor process and creates an .onion address
search
```

The .onion address is displayed in the startup banner.

## GeoIP Blocking

Block or allow traffic from specific countries (as a risk signal, not the sole gate):

```yaml
server:
  geoip:
    enabled: true
    deny_countries:
      - CN
      - RU
```

## Container Security

When running in Docker:

- The container uses tini as init system for proper signal handling
- Alpine-based minimal attack surface
- Production is the default mode. Set `MODE=development` only for development deployments; debug endpoints require the separate `DEBUG` setting.

```bash
docker run \
  --name search-app \
  -p 172.17.0.1:64580:80 \
  -v search_config:/config \
  -v search_data:/data \
  ghcr.io/apimgr/search:latest
```

Video preview clips must be same-origin WebM or MP4 files. Third-party embeds are not loaded; when a provider supplies no eligible clip, the static thumbnail remains available.
