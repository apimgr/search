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

### Regular Updates

Keep Search updated to receive security patches:

```bash
search --update yes
```

### Firewall Configuration

Restrict the metrics endpoint — it must never be proxied to the public internet:

```bash
# Block external access to the metrics endpoint
iptables -A INPUT -p tcp --dport 64580 -m string --string "/metrics" --algo bm -j DROP
```

## Security Reporting

If you discover a security vulnerability, please report it responsibly:

1. **Do not** disclose publicly until fixed
2. Email security details to the maintainers
3. Include steps to reproduce
4. Allow time for a fix before disclosure

Search serves a security contact file at `/.well-known/security.txt`.

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
