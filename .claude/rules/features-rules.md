# Features Rules (PART 17-22)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO

- Use robfig/cron — always use github.com/go-co-op/gocron/v2
- Store SMTP passwords in plaintext (use environment variable or encrypted config)
- Send email without user opt-in (GDPR compliance)
- Use hardcoded GeoIP database paths (use configurable path from server.yml)
- Skip backup verification (always verify backup integrity after write)
- Auto-update binary without user confirmation

## CRITICAL - ALWAYS DO

- Scheduler: github.com/go-co-op/gocron/v2 (not robfig/cron)
- Email: SMTP with TLS support (auto/starttls/tls/none)
- GeoIP: oschwald/maxminddb-golang with configurable DB path
- Metrics: prometheus/client_golang (Prometheus format)
- Backup: timestamped archives, configurable retention, integrity verification
- Update check: version comparison only; actual update requires user confirmation

## Scheduler (PART 18)

Required library: github.com/go-co-op/gocron/v2

Built-in scheduled tasks:
| Task | Schedule | Default |
|------|----------|---------|
| geoip_update | 0 3 * * 0 (Sun 3AM) | enabled |
| blocklist_update | 0 4 * * * (4AM daily) | enabled |
| cve_update | 0 5 * * * (5AM daily) | enabled |
| log_rotation | 0 0 * * * (midnight) | enabled |
| token_cleanup | */15 * * * * (every 15m) | enabled |
| backup | 0 2 * * * (2AM daily) | enabled |
| ssl_renewal | 0 3 * * * (3AM daily) | enabled |
| health_check | */5 * * * * (every 5m) | enabled |
| tor_health | */10 * * * * (every 10m) | enabled |

## Email / Notifications (PART 17)

- SMTP configuration in server.yml (smtp section)
- TLS modes: auto, starttls, tls, none
- Default from: no-reply@{fqdn}
- Search alerts: Google Alerts-style, accountless via email verification
- Notification events: maintenance enter/exit, search alert matches

## GeoIP (PART 19)

- Library: github.com/oschwald/maxminddb-golang
- Database: MaxMind GeoLite2-Country.mmdb (free) or GeoIP2 (paid)
- Auto-update schedule: configurable (default Sunday 3AM)
- Country blocking: configured in server.yml geoip.blocked_countries list
- Database path: configurable in server.yml

## Metrics (PART 20)

- Format: Prometheus (text/plain; version=0.0.4)
- Endpoint: /server/metrics (operator token required)
- Standard metrics: request count, duration, errors, goroutines
- Search-specific: queries per engine, result counts, latency percentiles

## Backup & Restore (PART 21)

- Format: timestamped .tar.gz archive
- Contents: server.yml + database + TLS certs (excludes binary)
- Retention: configurable (default: keep last 4)
- Restore: requires operator token OR root (destructive operation)
- Auto-backup: 2AM daily (configurable)

## Update Command (PART 22)

- Check: curl -q -LSsf https://api.github.com/repos/apimgr/search/releases/latest
- Version comparison only on check
- Actual update: downloads new binary to temp, verifies checksum, replaces with confirmation
- Never auto-update silently
- --maintenance update checks if binary path is writable (escalate if not)

For complete details, see AI.md PART 17, 18, 19, 20, 21, 22
