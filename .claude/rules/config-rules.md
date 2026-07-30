# Config Rules (PART 5, 6, 12)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO
- NEVER use `strconv.ParseBool()` — always `config.ParseBool()` / `config.IsTruthy()`
- NEVER put YAML comments inline — always on the line ABOVE (exception: GitHub Actions SHA-pin `# vX.Y.Z` annotations)
- NEVER store user accounts or operator-editable config in the database — `server.yml` is the sole source of truth
- NEVER fail startup on invalid config — warn and replace with default
- NEVER let `--debug`/`DEBUG=true` bypass auth or the `server.token` check — debug affects verbosity/diagnostics only
- NEVER enable debug endpoints (`/debug/*`, pprof, expvar) unless `--debug`/`DEBUG=true` is set, regardless of `mode`
- NEVER trust `X-Forwarded-*` headers from a peer not in `trusted_proxies`
- NEVER fall back Tor responses to the clearnet email/FQDN — omit entirely if `tor.contact_email`/`tor.onion_address` unset
- NEVER auto-advertise `abuse@{fqdn}` — operator must opt in explicitly (unlike `security@{fqdn}`, which defaults on)
- NEVER re-resolve the backup directory at cleanup time — use the path cached at startup step 7
- NEVER expand comments past 140 chars (config file) or use `server.yaml` (must be `server.yml`, auto-migrated)

## CRITICAL - ALWAYS DO
- ALWAYS accept the full truthy/falsy vocabulary (yes/no, on/off, enable/disable, si/non, etc.), case-insensitive
- ALWAYS treat only 2 error classes as critical: DB connection failure, cannot write to files — everything else is recoverable via self-healing
- ALWAYS persist the selected port (random or explicit) to `server.yml` after first run
- ALWAYS validate config on load; invalid value → log warning + use default, never crash
- ALWAYS resolve mode/debug via documented priority order (CLI flag > env var > alias > default)
- ALWAYS preserve the original TCP peer address before real-IP middleware rewrites `r.RemoteAddr`, and gate `trusted_proxies` checks on that original peer
- ALWAYS require `server.token` OR root for sensitive `--maintenance` operations (setup, restore, mode, pgp, secret rotate)
- ALWAYS drop privileges to the dedicated `{project_name}` system user after binding privileged ports (Unix), unless the project has a documented IDEA.md permanent-root exception
- ALWAYS use canonical contact keys only: `server.contact.{admin,security,abuse,general}.email`

## Key Rules Summary

**Config file & precedence**
- Location: `/etc/{internal_org}/{internal_name}/server.yml` (root) or `~/.config/{internal_org}/{internal_name}/server.yml` (user)
- `server.yml` = source of truth for all settings; database only holds resource state, tokens (SHA-256), audit log
- `server.yaml` auto-migrates to `server.yml` on startup
- Config validation never fails startup — invalid → default + warning

**Environment variables**
- Runtime (always checked): `NO_COLOR`, `TERM`, `DOMAIN`, `MODE`, `DATABASE_DRIVER`, `DATABASE_URL`, `SMTP_*`
- Init-only (first run only, then ignored): `CONFIG_DIR`, `DATA_DIR`, `LOG_DIR`, `DATABASE_DIR`, `BACKUP_DIR`, `PORT`, `LISTEN`, `APPLICATION_NAME`, `APPLICATION_TAGLINE`
- URL resolution priority: reverse-proxy headers → `DOMAIN`/config → `os.Hostname()`/`$HOSTNAME` → global IP → `localhost`

**Runtime mode detection**
- Mode priority: `--mode` CLI > `MODE` env > default `production`
- Debug priority: `--debug` CLI > `DEBUG` env > `--mode debug`/`MODE=debug` alias > default `false`
- `MODE=debug` = development + debug on, but an explicitly set `DEBUG` (true or false) always wins over the alias
- 4 states: Production, Production+Debug, Development, Development+Debug
- Debug endpoints (`/debug/*`, pprof, expvar) return 404 unless `--debug`/`DEBUG=true`

**Server config keys (`server.*`)**
- `port`: single, dual (`"8090,8443"`), `0` (OS-assigned), or omitted (random 64000-64999, saved on first run)
- `address`, `fqdn`, `mode`, `api_version`, `baseurl` (default `/`, resolved from `X-Forwarded-Prefix`/`X-Forwarded-Path`/`X-Script-Name` first)
- `maintenance.self_healing`, `maintenance.cleanup`, `maintenance.notify`
- `database.driver` (`sqlite` default, or `libsql`/`turso`), `database.url`
- `ssl.enabled`, `ssl.cert`/`key` (optional manual override), `ssl.letsencrypt.*`
- `scheduler.tasks.*` (geoip_update, blocklist_update, cve_update, log_rotation, token_cleanup, backup, ssl_renewal, health_check, tor_health)
- `rate_limit.{read,write,health,global_burst}` — 429 + `Retry-After` header (never a body field)
- `trusted_proxies.additional` — IP/CIDR/DNS list; private ranges always trusted
- `tor.onion_address`, `tor.contact_email` — priority-0 FQDN resolution, bypasses proxy trust gate
- `contact.{admin,security,abuse,general}.{email,webhooks}` — fallback chains flow to `admin`
- `tracking.{type,id,url}` — 9 supported platforms (google, matomo, piwik, owa, fathom, plausible, umami, simple, cloudflare)
- `privacy.*` — consent banner, cookie categories, dynamic messaging keyed on `data.sold`
- `cache.{type,url,host,port,...}` — `memory` default, `valkey`/`redis` optional; `url` takes precedence over host/port
- `limits.{max_body_size,read_timeout,write_timeout,idle_timeout}`, `compression.*`, `i18n.*`

For complete details, see AI.md PART 5, 6, 12.
