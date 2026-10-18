# Security

Search is designed with security and privacy as primary concerns. This document outlines the security model and best practices.

## Security Model

### Authentication

- **Operator token** (`server.token` in `server.yml`) — auto-generated on first run; gates all `/api/v1/server/*` management endpoints
- **Per-resource tokens** — for search alerts and webhooks; stored as SHA-256 hashes
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
| `X-Frame-Options` | `DENY` |
| `X-Content-Type-Options` | `nosniff` |
| `X-XSS-Protection` | `1; mode=block` |
| `Referrer-Policy` | `strict-origin-when-cross-origin` |
| `Content-Security-Policy` | Restrictive policy |
| `Permissions-Policy` | All sensors locked; tracking proposals locked |

### Privacy

- **No query logging** — user searches are never written to logs
- **No IP logging** — request IPs are never stored or logged
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

The operator token is auto-generated on first run and stored in `server.yml`. Keep the config file permissions restrictive:

```bash
chmod 600 /etc/apimgr/search/server.yml
```

### Rate Limiting

Enable and configure rate limiting:

```yaml
server:
  rate_limit:
    enabled: true
    requests_per_minute: 60
    burst: 10
```

### IP and Domain Blocklists

Missing enabled blocklist and CVE data is queued for a first-run refresh after the HTTP listener is ready; the refresh does not delay startup. GeoIP databases are loaded or downloaded during initialization. The built-in scheduler keeps the data updated daily. The default blocklist feeds cover IP addresses and CIDR ranges. Add operator-managed feeds under `server.security.blocklists` in `server.yml`:

```yaml
server:
  security:
    blocklists:
      - name: phishing_domains
        url: https://feeds.example.net/domains.txt
        type: domain
        enabled: true
```

Supported feed types are `ip`, `cidr`, and `domain`. Domain feeds accept one domain per line or hosts-file entries. A blocked domain applies to that exact host and its subdomains. Feed URLs must use HTTPS; invalid entries are ignored and logged without exposing their URL. Each download is limited to 64 MiB and persisted only after parsing succeeds with at least one usable rule. If any configured feed fails, the updater keeps the last known rules, merges newly downloaded entries conservatively, and reports an incomplete update for scheduler visibility. Changing or disabling feeds reloads only currently configured and enabled feed files, so removed rules do not remain active from stale files.

### Regular Updates

Keep Search updated to receive security patches:

```bash
search --update yes
```

### Firewall Configuration

Restrict the metrics endpoint — it must never be proxied to the public internet:

```bash
# Block external access to the metrics endpoint
iptables -A INPUT -p tcp --dport 64580 -m string --string "/server/metrics" --algo bm -j DROP
```

## Public Security Endpoints

The following endpoints are public (no authentication) and support responsible
disclosure and health monitoring:

| Endpoint | Purpose |
|----------|---------|
| `/.well-known/security.txt` | RFC 9116 security contact and policy |
| `/.well-known/pgp-key.asc` | PGP public key (returns `404` until a keypair exists) |
| `/.well-known/llms.txt` | Machine-readable summary for LLM agents |
| `/.well-known/change-password` | Password-change discovery redirect |
| `/server/security` | Human-readable security overview and how to report |
| `/server/security/policy` | Full security/disclosure policy |
| `/server/contact` | Contact form (supports `security_id` deep links) |
| `/server/healthz` | Public health check |
| `/api/v1/server/healthz` | Public health check (JSON) |

The browser contact form uses a non-interactive honeypot and initially renders
without a CAPTCHA. After a failed form attempt, it sets a short-lived signed
cookie and requires the built-in arithmetic challenge on subsequent attempts.
The cookie is scoped to `/server/contact`, HttpOnly, SameSite=Strict, and Secure
when the server's cookie policy requires it.

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

## Security Logs

Security events (rate limit violations, blocked requests, CSRF violations) are written to the security log at `/var/log/apimgr/search/security.log`. No user queries, IPs, or identifying information are ever logged.

## Tor Hidden Service

For enhanced privacy, Search automatically enables a Tor hidden service when the `tor` binary is found on PATH:

```bash
# Install Tor
apt-get install tor

# Start search — it will auto-detect Tor and create a .onion address
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
- Environment variable `MODE=development` by default; set `MODE=production` for production deployments

```bash
docker run \
  -e MODE=production \
  -v search_config:/config \
  -v search_data:/data \
  ghcr.io/apimgr/search:latest
```

The default Content Security Policy allows frames from the same origin only. To enable the on-demand YouTube privacy-enhanced video previews, add the provider to the per-directive frame source allow-list in `server.yml`:

```yaml
web:
  csp:
    frame_src_extra: https://www.youtube-nocookie.com
```

This appends the source to the existing `frame-src` directive. Video embeds are created only after a user activates a preview, and the static thumbnail remains available when the provider is blocked.
