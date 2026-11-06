# Feature Rules (PART 17-22)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

Read: `AI.md` PART 17 (Email & Notifications), PART 18 (Scheduler), PART 19 (GeoIP), PART 20 (Metrics), PART 21 (Backup & Restore), PART 22 (Update Command).

## CRITICAL - NEVER DO
- Rely on external cron — the scheduler is built in and always running
- Hard-fail when SMTP is unconfigured — email degrades gracefully
- Hard-fail when the GeoIP database is unavailable — degrade gracefully
- Expose a metrics service endpoint without bearer-token authentication
- Perform a partial restore — restore is atomic, all or nothing
- Allow an unencrypted backup when compliance mode is enabled
- Ship an update path with no rollback

## CRITICAL - ALWAYS DO
- Configure SMTP in `server.yml` and environment overrides; never mutate server configuration through an API
- Queue email with retry logic and log every success and failure; keep templates customizable
- Run the built-in scheduler always: backup 02:00 daily, SSL renewal 03:00 daily, GeoIP update 03:00 Sunday, session cleanup hourly — all configurable
- Expose scheduler status via CLI and a `server.token`-protected status endpoint, plus manual task triggering from the CLI
- Treat GeoIP as a risk signal only; fail open for lookup/database errors and never bypass auth because of an allowed country
- Support only the canonical ip-location-db source per GeoIP category, with visible attribution and operator-defined presets that never auto-apply
- Serve Prometheus metrics at `/server/metrics` (root `/metrics` gated on `server.metrics.root.enabled`) plus `/server/metrics/prometheus`, `/server/metrics/grafana`, `/server/metrics/loki`
- Authenticate each metrics service with its own bearer token; aliases call the same handler and never redirect
- Track request count, latency, errors, system metrics (memory, goroutines), and custom business metrics
- Verify every backup after creation (checksum, decrypt, extract, DB integrity); back up database, config, and uploads
- Honor retention settings: `max_backups` (default 1), `keep_weekly`/`keep_monthly`/`keep_yearly` (default 0)
- Implement `--maintenance backup` and `--maintenance restore {file}` (prompting for the password when encrypted)
- Implement `--update check`, `--update yes`, `--update branch {name}` across the stable, beta, and devel channels; accept legacy channel names only as aliases and normalize them to devel
- Keep scheduled update installation off by default; `defer_days` gates scheduled stable/beta updates only, while manual checks always see the newest eligible channel release
- Verify SHA-256 before replacing the executable; use platform-specific replacement and rollback-safe restart behavior

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
