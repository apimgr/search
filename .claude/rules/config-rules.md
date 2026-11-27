# Config Rules (PART 5, 6, 12)
<!-- Synchronized with AI.md revision f5ea82acc67d. -->

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

Read: `AI.md` PART 5 (Configuration), PART 6 (Application Modes), PART 12 (Server Configuration).

## CRITICAL - NEVER DO
- Name the config file `server.yaml` or `server.json` — it is always `server.yml`
- Silently ignore an unknown config key — unknown keys are ERRORS
- Ship a config value without a sane default
- Use SIGHUP for reload — `src/signal/signal_unix.go` intentionally ignores SIGHUP; reload is driven by the file watcher
- Enable `/debug/*` endpoints (pprof, expvar) from a mode — only the debug flag enables them
- Enable debug endpoints in development mode
- Use a config library outside the PART 5 approved list
- Hardcode a config path — resolve it through the path package

## CRITICAL - ALWAYS DO
- Honor the precedence chain: CLI flags > env vars > config file > defaults
- Invent environment variable prefixes or aliases — use only names and aliases explicitly listed in PART 5
- Use `strconv.ParseBool`; use `config.ParseBool()` or `config.IsTruthy()` for boolean parsing and reject invalid values
- Validate the whole config on load and fail loudly on error
- Hot-reload safe settings via the config file watcher (`src/config/config.go`)
- Document clearly which settings require a restart
- Default to production mode; normalize `prod`/`production` and `dev`/`development`
- Keep operator configuration file-based across `server.yml`, the ordered `yaml.d/` area files, and `secrets.yml`; store credentials only in `secrets.yml`, never in the database or a runtime config API
- Keep every server setting in its owning `server.yml` or `yaml.d/` area, including Valkey/Redis when used; hot-reload `server.yml`, `yaml.d/`, `secrets.yml`, and watched assets
- Use canonical nested `server.*` keys for rate limits, CORS, branding/SEO, logs, notifications (including `server.notifications.email.smtp.*`), security headers, and allowed origins
- `server.logs.audit.group` is system-mode-only and may name an existing group; the server never creates groups, and an unknown group is warned about and treated as empty
- Use the canonical contact recipient keys only; Tor responses use `server.tor.contact_email`, and test-email recipient defaults to `server.notifications.email.reply_to`
- Normalize paths and validate all configured values before they are used; persisted port selection is 64000–64999 when the server chooses a first-run port
- Trust forwarded host/proto/IP headers only from configured trusted proxies; arbitrary clients cannot supply trusted proxy identity
- Never upgrade or redirect Tor hidden-service HTTP requests to HTTPS, and do not expose clearnet identity in Tor responses
- Treat `maintenance.self_healing.max_attempts: 0` as unlimited; after a positive limit, report exhaustion and require the documented operator restart to leave maintenance mode
- Keep cache backend selection in `server.yml`; configure memory by default and prevent setting both URL and host/port for Valkey/Redis
- Treat `DATABASE_DRIVER`, `DATABASE_URL`, and `CACHE_URL` as first-run-only seed values: save credentials in `secrets.yml`, other settings in `server.yml`, and ignore those variables after initialization
- Resolve database drivers from the URL scheme when a URL is present; SQLite driver selection is the only case that needs `DATABASE_DRIVER` without a URL

## KEY DECISIONS (pre-answered)
| Question | Answer | Reference |
|----------|--------|-----------|
| Config filename? | `server.yml` (never `.yaml`) | PART 5 |
| Unknown keys? | ERROR, never ignored | PART 5 |
| Reload mechanism? | File watcher, NOT SIGHUP | PART 5, 12 |
| Default mode? | production | PART 6 |
| What enables `/debug/*`? | The debug flag only | PART 6 |
| Does `--mode debug` set debug? | Yes, it defaults the debug flag on | PART 6 |
| Invalid config values? | Fail startup/reload validation with a useful operator error | PART 5, 12 |
| Proxy headers? | Honor only behind configured trusted proxies | PART 12 |
| Tor HTTP? | Keep Tor traffic HTTP; never HTTPS-upgrade it | PART 12 |
| Database environment variables? | `DATABASE_DRIVER`, `DATABASE_URL`, and `CACHE_URL` seed first-run config only | PART 5 |

## TERMINOLOGY
| Term | Meaning |
|------|---------|
| mode | `production` \| `development` \| `debug` (`server.mode`, `MODE`/`SEARCH_MODE`) |
| debug flag | Independent switch that gates `/debug/*`; set by `--debug` or `DEBUG` |
| hot-reload | Applying a changed `server.yml` without restarting the process |

## QUICK REFERENCE
| Layer | Source | Wins over |
|-------|--------|-----------|
| 1 | CLI flags | everything |
| 2 | Explicitly documented environment variable names | file, defaults |
| 3 | `server.yml` + ordered `yaml.d/` area files + `secrets.yml` | defaults |
| 4 | Built-in defaults | — |

- Mode dispatch: `src/mode/mode.go`, `src/config/env.go`
- Config load/watch: `src/config/config.go`

---

For complete details, see AI.md PART 5, 6, 12
