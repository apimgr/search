# Backend Rules (PART 9, 10, 11, 31)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

Read: `AI.md` PART 9 (Error Handling & Caching), PART 10 (Database), PART 11 (Security & Logging), PART 31 (Overlay Networks — Tor & I2P).

## CRITICAL - NEVER DO
- Use bcrypt for config/backup passwords — Argon2id only
- Store a raw API token — hash with SHA-256
- Hardcode credentials anywhere in the code
- Build SQL by string concatenation — prepared statements only
- Require a manual schema step — migrations run automatically on startup
- Emit stack traces outside development mode
- Log sensitive data (tokens, credentials, internal paths, secrets)
- Execute or serve untrusted user content as active content on the app origin
- Use the system Tor daemon — the app runs its own dedicated tor process
- Enable I2P by default — it is opt-in only

## CRITICAL - ALWAYS DO
- Return one consistent JSON error format with stable, documented error codes
- Set correct `Cache-Control` and `ETag` headers per resource type; serve `/sw.js` and `/manifest.json` `no-cache` with a build-stamp ETag
- Purge on version change: build-stamp cookie plus `Clear-Site-Data: "cache", "storage"` on mismatch
- Default to SQLite locally and libsql/Turso remotely, using `CREATE TABLE IF NOT EXISTS` and connection pooling
- Set all security headers (CSP, X-Frame-Options, …) and enable HSTS whenever SSL is active
- Enable configurable rate limiting; auto-revoke tokens after the specified number of consecutive authentication failures
- Compare tokens, password hashes, HMACs, signatures, and credentials in constant time; return identical generic authentication failures and pad response timing
- Use opaque public resource IDs; never expose sequential database IDs, and never echo credentials in responses or logs
- Keep private file responses `private, no-store`; serve user-controlled active content only as an attachment with exact content type and `nosniff`
- Write audit events to `audit.log` and security events to `security.log` in JSON, with rotation and `debug/info/warn/error` levels
- Auto-enable and supervise the dedicated Tor hidden-service process when its binary is available; config at `{config_dir}/tor/torrc`, data at `{data_dir}/tor/`; never use a system Tor daemon
- Keep I2P opt-in; support the in-app i2pd model or the documented external SAM router model, using a dedicated loopback backend port
- Report `features.tor.*` and `features.i2p.*` in the healthz API

## KEY DECISIONS (pre-answered)
| Question | Answer | Reference |
|----------|--------|-----------|
| Password hash? | Argon2id (NEVER bcrypt) | PART 11 |
| API token hash? | SHA-256 | PART 11 |
| Local database? | SQLite (`modernc.org/sqlite`, no CGO) | PART 10 |
| Remote database? | libsql / Turso | PART 10 |
| Migrations? | Automatic on startup | PART 10 |
| Tor? | Auto-enabled when the binary is found | PART 31.1 |
| I2P? | OPTIONAL, opt-in, disabled by default | PART 31.2 |
| Auth comparisons? | Constant-time; authentication failures have generic message/status and padded timing | PART 11 |

## TERMINOLOGY
| Term | Meaning |
|------|---------|
| audit.log | JSON log of user/admin actions |
| security.log | JSON log of security events |
| Model A | Dedicated in-app i2pd process, not a system router |
| eepsite | `.b32.i2p` service address |

## QUICK REFERENCE
- Database access: `src/database`
- Security middleware and secret handling: `src/security`
- Structured logging: `src/logging`
- Tor: `{config_dir}/tor/torrc`, `{data_dir}/tor/`
- I2P: `{config_dir}/i2p/tunnels.conf`, `{data_dir}/i2p/` (key persists at `{data_dir}/i2p/site/`)
- Enable I2P via `features.i2p.enabled` / `I2P_ENABLED=true` / `--i2p`

---

For complete details, see AI.md PART 9, 10, 11, 31
