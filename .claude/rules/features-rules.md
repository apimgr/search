# Feature Rules (PART 17-22)
<!-- Synchronized with AI.md revision c2f65bf274b5. -->

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

Read: `AI.md` PART 17 (Email & Notifications), PART 18 (Scheduler), PART 19 (GeoIP), PART 20 (Metrics), PART 21 (Backup & Restore), PART 22 (Update Command).

## CRITICAL - NEVER DO
- Rely on external cron — the scheduler is built in and always running
- Hard-fail when SMTP is unconfigured — email degrades gracefully
- Hard-fail when the GeoIP database is unavailable — degrade gracefully
- Expose a metrics service endpoint without bearer-token authentication
- Render operator notifications in the public WebUI of an API project
- Perform a partial restore — restore is atomic, all or nothing
- Allow an unencrypted backup when compliance mode is enabled
- Store backup passwords or key material in plaintext; derive backup keys with Argon2id
- Ship an update path with no rollback

## CRITICAL - ALWAYS DO
- Configure SMTP in `server.yml` and environment overrides; never mutate server configuration through an API
- Auto-detect local SMTP on first run; a missing or unavailable SMTP server disables email with a warning and does not stop startup
- Apply port-aware SMTP `tls: auto`: implicit TLS only on 465, otherwise EHLO then STARTTLS when offered; never send AUTH before TLS
- Queue email with retry logic and log every success and failure; keep templates customizable
- Store template overrides under `{config_dir}/template/email/`; validate before loading and retain embedded defaults on blocking errors; email renders in one language chosen by request or configured default
- If SMTP is absent or unreachable, do not queue or attempt email; disable email features with one warning and keep in-app notifications working
- Keep notifications separate by audience: public WebUI notifications for visitors, operator events in logs/email (email is optional); API projects have no admin UI
- Provide `--maintenance email test [--to ADDRESS]` and `--maintenance email test --dry-run`; default recipient is `server.notifications.email.reply_to`, previews validate every template, and invalid overrides fall back to embedded defaults
- Run the built-in scheduler always; required tasks include backup 02:00 daily, SSL renewal 03:00 daily, GeoIP Sunday 03:00, log rotation, security cleanup, CVE/update checks, disk/database/health checks, token cleanup and keyserver retry every 15 minutes, plus Tor/I2P health only when available — scheduler settings live under `server.scheduler` and in `yaml.d/maintenance.yml`
- Provide `--maintenance scheduler list|show|run|enable|disable|history`; only the CLI changes task enabled state
- Isolate every scheduled task invocation with panic recovery so a single task cannot crash the scheduler or server
- Log one INFO inventory of registered tasks at startup; successful task runs are DEBUG, failures WARN and exhausted retries ERROR; feature-bound tasks are not registered while unavailable
- Treat GeoIP as a risk signal only; fail open for lookup/database errors and never bypass auth because of an allowed country
- Support only the canonical ip-location-db source per GeoIP category, with visible attribution and operator-defined presets that never auto-apply
- Serve Prometheus metrics at `/server/metrics` and `/server/metrics/prometheus`, a complete importable dashboard at `/server/metrics/grafana`, and Loki at `/server/metrics/loki`; root `/metrics[/{service}]` is enabled by default and configurable
- Authenticate each metrics service with its own bearer token stored in `secrets.yml`; aliases call the same handler and never redirect
- Track request count, latency, errors, system metrics (memory, goroutines), and custom business metrics
- Encrypt backups with AES-256-GCM and an Argon2id-derived key; the backup password is encrypted at rest as `enc:v1:` in `secrets.yml`, is never hashed or written to logs/manifests/archives, and is supplied by the operator for restore; verify each backup after creation (checksum, decrypt, extract, DB integrity)
- Honor retention settings: `max_backups` (default 1), `keep_weekly`/`keep_monthly`/`keep_yearly` (default 0)
- Disk-pressure cleanup honors `server.backup.retention`; never replace the configured policy with a fixed number of retained backups
- Implement `--maintenance backup` and `--maintenance restore {file}` (prompting for the password when encrypted)
- Implement `--update check`, `--update yes`, `--update branch {name}` across the stable, beta, and devel channels; accept legacy channel names only as aliases and normalize them to devel
- Keep scheduled update installation off by default; `defer_days` gates scheduled stable/beta updates only, while manual checks always see the newest release for the channel
- Only a process started elevated may install an update; stage as the service user, verify the release checksum again as root before replacing the executable, exit `75` for supervisor restart, and retain the old binary on every failure

## KEY DECISIONS (pre-answered)
| Question | Answer | Reference |
|----------|--------|-----------|
| External cron? | NEVER — built-in scheduler | PART 18 |
| Backup encryption? | AES-256-GCM; optional unless compliance is on | PART 21 |
| Default `max_backups`? | 1 | PART 21 |
| Metrics auth? | Per-service bearer token, mandatory | PART 20 |
| Hourly incremental backup? | Exists, disabled by default | PART 21 |
| Update channels? | stable, beta, devel | PART 22 |
| GeoIP presets? | Empty by default; operators save them explicitly and they never change enforced country lists | PART 19 |
| Update auto-install? | Disabled by default; scheduled checks notify only | PART 22 |

## TERMINOLOGY
| Term | Meaning |
|------|---------|
| task | A named scheduler job (e.g. `backup_daily`, `backup_hourly`) |
| compliance mode | Setting that makes backup encryption mandatory |
| graceful degradation | Feature disables itself cleanly instead of erroring |

## QUICK REFERENCE
| Area | Location |
|------|----------|
| Email + templates | `src/email`, `src/email/templates` |
| Scheduler | `src/scheduler` |
| GeoIP | `src/geoip` |
| Backup/restore | `src/backup` |
| Server self-update | `src/update` |
| CLI self-update | `src/client/update` |

Backup artifacts: `{project_name}-daily.tar.gz[.enc]`, `{project_name}-hourly.tar.gz[.enc]`

---

For complete details, see AI.md PART 17, 18, 19, 20, 21, 22
