# Features Rules (PART 17, 18, 19, 20, 21, 22)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO

- NEVER send, queue, or attempt an email without valid, working SMTP — no "would have sent" log lines
- NEVER show email-dependent UI/options when SMTP is not configured
- NEVER create account-related emails (no users, no password resets, no verification flows)
- NEVER use an external scheduler (cron, systemd timers, Task Scheduler, launchd, K8s CronJob, cloud schedulers) for ANY scheduled task — the built-in scheduler is the only mechanism, no exceptions
- NEVER treat GeoIP/country as the sole access-control gate or as authentication — it is a risk signal only
- NEVER block a request because a GeoIP lookup failed or the database is missing/stale (fail-open for GeoIP)
- NEVER use a raw client IP as a Prometheus label value (unbounded cardinality / memory-DoS)
- NEVER expose `/metrics` publicly — it is internal-only (firewall/proxy/NetworkPolicy restricted)
- NEVER delete old backups before the new backup passes ALL verification checks
- NEVER store the backup encryption password anywhere — it is prompted on demand, never a CLI flag
- NEVER allow backups when compliance mode is enabled and no encryption password is set
- NEVER surface update/version status on any public endpoint, header, or API response (Tier 3 info)
- NEVER let a security-alert/backup-failed/ssl-renewal-failed event skip its dedicated notification in favor of double-firing `scheduler_error` (suppression rules apply)
- NEVER render operator events (backups, SSL, updates, scheduler, abuse) in the public WebUI — operators only see logs/email/CLI

## CRITICAL - ALWAYS DO

- ALWAYS fall back to the embedded default email template when no custom template exists at `{config_dir}/template/email/`
- ALWAYS auto-detect local SMTP on first run (loopback → docker bridge → gateway → fqdn → global IPv4 → mail./smtp. subdomains) and test the configured connection on every startup
- ALWAYS let `SMTP_*` env vars override `server.yml` SMTP config
- ALWAYS validate email templates before saving (unknown variables, empty subject/body, syntax)
- ALWAYS keep the scheduler running continuously with state persisted in `server.db` and catch-up execution of missed tasks within `catch_up_window`
- ALWAYS include the required built-in scheduler tasks (ssl_renewal, geoip_update, blocklist_update, cve_update, update_check, token_cleanup, log_rotation, backup_daily, backup_hourly, healthcheck_self, tor_health)
- ALWAYS download GeoIP MMDB databases at runtime (from sapics/ip-location-db) — never embed them in the binary
- ALWAYS use `github.com/oschwald/maxminddb-golang` for GeoIP (not `geoip2-golang`, which rejects ip-location-db's custom `database_type` strings)
- ALWAYS let allowlisted IPs bypass GeoIP country blocking; never country-block private/RFC1918 IPs
- ALWAYS prefix Prometheus metrics with `{project_name}_`, use snake_case, and suffix counters with `_total` and durations/sizes with `_seconds`/`_bytes`
- ALWAYS verify every backup immediately after creation (existence, size>0, checksum, decrypt test, manifest, extraction, DB integrity) — all checks must pass
- ALWAYS check free disk space / usage threshold before creating a scheduled backup; abort and log `backup.skipped_disk_full` if insufficient
- ALWAYS require operator authorization for restore (token, root, or first-run-empty-db only) and verify the backup fully before restoring
- ALWAYS verify the downloaded update binary's SHA256 against the release's `checksums.txt` before replacing the running binary
- ALWAYS let manual `--update check`/`--update yes` see and install the true latest release, bypassing `defer_days` (only the scheduled task honors the defer window)

## Key Rules Summary

**Email & Notifications (PART 17):**
- Templates: embedded defaults in binary, custom overrides in `{config_dir}/template/email/`; `{variable}` syntax, `Subject: ... --- body` format
- Templates: security_alert, backup_complete/failed, ssl_expiring/renewed/renewal_failed, scheduler_error, update_available/installed, test
- Three channels only: Public WebUI (toast + banner, visitors only), Logs (always), Email (SMTP required)
- No notification center/bell/history — dismissal via `dismissed_announcements` cookie only
- Per-event email toggles live under `server.notifications.email.events`; failure-specific notifications suppress the generic `scheduler_error`

**Scheduler (PART 18):**
- Single built-in scheduler, always running, DB-backed state, catch-up on restart, graceful shutdown (30s drain)
- Cron syntax + `@hourly`/`@daily`/`@weekly`/`@monthly`/`@every Xm/Xh`
- CLI: `scheduler list|show|run|enable|disable|history`
- Retry policy default: 3 retries, 5m delay, exponential backoff

**GeoIP (PART 19):**
- sapics/ip-location-db MMDB files (asn, country, city ipv4/ipv6); WHOIS is a combined ASN+country lookup, not a separate file
- `deny_countries` vs `allow_countries` (allowlist wins if both set); ISO 3166-1 alpha-2 codes
- Weekly update via scheduler (Sunday 03:00); always a risk signal, never sole gate, fail-open on error

**Metrics (PART 20):**
- Prometheus text format at `/metrics` (configurable path), optional bearer token, internal-only access
- Required: app_info, app_uptime/start, HTTP request/duration/size/active metrics
- Conditional: DB, cache, scheduler, system, runtime, Tor, rate-limit metrics per feature/config
- Path labels normalized (`:id` for UUIDs/IDs); low-cardinality labels only

**Backup & Restore (PART 21):**
- `--maintenance backup [filename]` / `--maintenance restore <file>`
- Contents: server.yml + server.db always; template/theme dirs if present; ssl/data optional via flags
- Encryption optional unless compliance mode enabled (then mandatory); AES-256-GCM + Argon2id, password never stored, no CLI password flag (interactive prompt only)
- Retention: max_backups/keep_weekly/keep_monthly/keep_yearly + max_total_size hard cap (size cap overrides counts); priority yearly > monthly > weekly > daily
- Restore requires verification (checksum, decrypt, manifest, version) and operator authorization

**Update Command (PART 22):**
- `--update [check|yes|branch {stable|beta|daily}]`, alias `--maintenance update`; exit 0 = updated/current, 1 = error
- Channels cumulative: beta includes stable, daily includes beta+stable; newest eligible release wins
- `defer_days` gates only the scheduled `update_check` task, never manual commands
- `auto_install` default false (notify-only); binary replacement is platform-specific (Unix rename, Windows rename-to-.old + reboot cleanup); service-aware restart per OS
- Update status/version is Tier 3 info — operator-only (logs/email/CLI), never in public API responses

For complete details, see AI.md PART 17, 18, 19, 20, 21, 22.
