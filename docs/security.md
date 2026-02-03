# Security

Search is designed with security as a primary concern. This document outlines security features and best practices.

## Security Features

### Authentication

- **Session-based authentication** for admin panel
- **Bearer token authentication** for API access
- **CSRF protection** on all forms
- **Rate limiting** to prevent brute force attacks
- **2FA support** (TOTP)

### Transport Security

- **TLS/SSL** encryption
- **Let's Encrypt** integration
- **HSTS** headers when SSL is enabled
- **Secure cookies** with HttpOnly and SameSite flags

### Security Headers

Search sets the following security headers:

| Header | Value |
|--------|-------|
| `X-Frame-Options` | `DENY` |
| `X-Content-Type-Options` | `nosniff` |
| `X-XSS-Protection` | `1; mode=block` |
| `Referrer-Policy` | `strict-origin-when-cross-origin` |
| `Content-Security-Policy` | Restrictive policy |
| `Permissions-Policy` | Minimal permissions |

### Data Protection

- **No logging** of search queries
- **No user tracking**
- **Image proxy** to prevent third-party tracking
- **Encrypted backups** (AES-256-GCM)

## Best Practices

### Enable SSL

Always use SSL in production:

```yaml
server:
  ssl:
    enabled: true
    letsencrypt:
      enabled: true
      email: "admin@example.com"
      domains:
        - "search.example.com"
```

### Strong Admin Password

Use a strong, unique password for the admin account. The password should be:

- At least 12 characters
- Include uppercase and lowercase letters
- Include numbers and symbols
- Not used elsewhere

### API Token Security

- Generate unique tokens for each integration
- Set appropriate expiration dates
- Revoke unused tokens
- Never share tokens publicly

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

Restrict access to the admin panel:

```bash
# Example: Only allow admin access from internal network
iptables -A INPUT -p tcp --dport 64580 -s 192.168.1.0/24 -j ACCEPT
iptables -A INPUT -p tcp --dport 64580 -j DROP
```

## Security Reporting

### Reporting Vulnerabilities

If you discover a security vulnerability, please report it responsibly:

1. **Do not** disclose publicly until fixed
2. Email security details to the maintainers
3. Include steps to reproduce
4. Allow time for a fix before disclosure

### security.txt

Search serves a security.txt file at `/.well-known/security.txt` with contact information for security reports.

## Audit Logging

All admin actions are logged to the audit log:

- Login attempts (success and failure)
- Configuration changes
- Backup and restore operations
- Token creation and revocation

View audit logs at `/admin/logs` or in the log files.

## Tor Hidden Service

For enhanced privacy, enable Tor:

```yaml
server:
  tor:
    enabled: true
```

This creates a .onion address for your instance, accessible via the Tor network.

## GeoIP Blocking

Block or allow traffic from specific countries:

```yaml
server:
  geoip:
    enabled: true
    deny_countries:
      - CN
      - RU
    # Or use allowlist mode:
    # allowed_countries:
    #   - US
    #   - CA
    #   - GB
```

## Container Security

When running in Docker:

- The container runs as a non-root user
- Uses tini as init system for proper signal handling
- Minimal attack surface (Alpine-based)
- Read-only root filesystem support

```bash
docker run --read-only \
  --tmpfs /tmp \
  -v search_data:/data \
  ghcr.io/apimgr/search:latest
```
