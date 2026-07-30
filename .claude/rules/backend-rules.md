# Backend Rules (PART 9, 10, 11, 31)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO

- Never expose stack traces, internal error chains, or `_debug` fields in production responses.
- Never use `==`, `bytes.Equal`, or `strings.EqualFold` for token/password/HMAC/signature comparison — use `crypto/subtle.ConstantTimeCompare` only.
- Never return different auth-failure messages/timing for "wrong password" vs "no such user" vs "locked" — always the same generic message + status, padded to a fixed floor (≥100ms).
- Never expose sequential integer IDs in public URLs/JSON/logs — external IDs must be opaque (UUIDv4/v7).
- Never log tokens, passwords, secrets, or credentials in full — only a stable hash/prefix, and only in `audit.log`/`security.log` with masking (first 8 chars).
- Never `DROP COLUMN`, `DROP TABLE`, `DELETE`, or rename a DB column — add new, migrate in app code, deprecate old.
- Never run a DB query without a `context.Context` timeout.
- Never build SQL with `fmt.Sprintf`/string concat of user input — parameterized queries only.
- Never cast user-controlled content to `template.HTML`; never render user HTML/SVG/XML inline as trusted content.
- Never shell out with raw user content, filenames, refs, or repo metadata; never execute user-supplied content server-side.
- Never write ANSI codes, emojis, or color to log FILES (console/stdout is fine, respecting `NO_COLOR`).
- Never use default Tor ports (9050/9051) or a fixed control port — always `127.0.0.1:auto`; never use system Tor.
- Never let the server fail to start because of a Tor error — Tor is optional, best-effort only.
- Never emit `Server-Timing` header in production (debug-mode only).
- Never emit deprecated headers (`Expect-CT`, `Public-Key-Pins`, `Feature-Policy`).

## CRITICAL - ALWAYS DO

- Always use the canonical response envelope: `{"ok":true,"data":{}}` / `{"ok":false,"error":"CODE","message":"..."}`.
- Always log every error with request ID, error code, HTTP status, and internal detail (server-side only).
- Always use exponential backoff (0,1,2,4,8s, cap 30s) for retryable errors only (network/timeout/503, never 4xx).
- Always use `CREATE TABLE IF NOT EXISTS` + idempotent `ALTER TABLE ... ADD COLUMN` for schema — no migration files, no version table.
- Always use connection pooling with configured `max_open`/`max_idle`/`max_lifetime`/`max_idle_time`.
- Always wrap multi-statement writes in a transaction; use `sql.LevelSerializable` + retry-on-serialization-failure for contested resources.
- Always run every public response through the Output Sanitization Pipeline: allow-list fields, redact sensitive query params, strip internal IPs/paths, truncate long strings, strip `dev_only` fields, pad timing.
- Always set the full security header set on every response (`X-Content-Type-Options`, `X-Frame-Options`, CSP, Permissions-Policy, Referrer-Policy, HSTS when SSL on, etc.).
- Always classify data into Tier 1 (never public, even in debug), Tier 2 (always public — version/commit/uptime), Tier 3 (debug-only) before exposing it anywhere.
- Always use Argon2id for password hashing (no bcrypt/MD5/SHA-* option).
- Always write `audit.log` as JSON Lines with `id` (ULID), UTC `time` with ms, `event`, `category`, `severity`, `actor`, `result`.
- Always auto-start Tor hidden service if the Tor binary is found (no enable flag) as a dedicated child process inheriting the server's (possibly dropped) user.

## Key Rules Summary

**Error handling**
- Standard error codes map to fixed HTTP statuses (`BAD_REQUEST`→400, `UNAUTHORIZED`→401, `FORBIDDEN`→403, `NOT_FOUND`→404, `CONFLICT`→409, `RATE_LIMITED`→429, `SERVER_ERROR`→500, `MAINTENANCE`→503).
- Log level scales with HTTP status: ≥500 → Error, 400-499 → Warn.

**Caching**
- Drivers: `memory` (dev default), `valkey` (preferred prod), `redis`.
- Keys: lowercase, colon-hierarchical (`{type}:{id}`, `rate:{type}:{key}`), version-prefixed for busting.
- TTLs: rate counters 1m, user profile 5m, config 1m, static hash 24h, GeoIP 7d, blocklist 1h, page cache 5m, API cache 30s.
- Invalidation: TTL, event-based (delete on write), version-based, or tag-based — pick per data shape.
- HTTP Cache-Control: static `public, max-age=31536000, immutable`; HTML/authenticated `no-store`; public API `public, max-age=60`.

**Database**
- Self-creating, idempotent schema only; no migration tooling.
- New columns must be nullable or have a `DEFAULT`.
- Query timeouts: simple SELECT 5s, JOIN 15s, write 10s, bulk 60s, reports 2m.
- Optimistic locking via `version` column; return `CONFLICT` on 0 rows affected.

**Security & logging**
- Defense-in-depth: input validation, parameterized queries, output escaping/CSP, and transport TLS — each layer assumes the others failed.
- Constant-time comparison + identical error message/timing for all auth failures.
- Crypto secrets (`installation_secret`, `cookie_signing_key`, `csrf_token_secret`, `encryption_key`) generated on first start, stored in DB/`server.yml`, never logged or returned via API, always in backups, rotated only via `--maintenance secret rotate <name>`.
- CSP default: `script-src 'self'` (no inline), `style-src 'self' 'unsafe-inline'`, `object-src 'none'`, per-directive operator extension only (append, not replace).
- Log files: `access.log`, `server.log`, `error.log`, `app.log`, `auth.log`, `audit.log` (JSON only), `security.log`, `debug.log` — each with a defined default format; raw text/no ANSI/no emoji in all of them.
- Audit log: append-only, JSON Lines, never logs private keys/SMTP creds/full credentials/financial data; always logs timestamp, IP, actor, result, event ID.
- Health-check 2xx requests suppressed from `access.log` by default; failures always logged.

**Tor hidden service**
- Always available if Tor binary found; no enable/disable toggle.
- Uses `github.com/cretz/bine`, v3 onion addresses (ed25519), `HiddenServiceVersion 3`.
- Control port always `127.0.0.1:auto`; hidden service forwards `.onion:{virtual_port}` → `localhost:{server_port}`.
- Directories: `{config_dir}/tor/` (0700, torrc 0600), `{data_dir}/tor/`, `{data_dir}/tor/site/` (keys) — not operator-configurable paths.
- Tor absence/errors are INFO/WARN only, never fatal to server startup.
- Outbound Tor routing (`use_network`) is a separate opt-in, off by default, via SOCKS through the same dedicated process.

For complete details, see AI.md PART 9, 10, 11, 31.
