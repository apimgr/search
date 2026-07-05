# Configuration Rules (PART 5, 6, 12)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO

- Use strconv.ParseBool() — always use config.ParseBool()
- Use inline YAML comments (comments go ABOVE the key)
- Hardcode port numbers (first run picks random 64000-64999, saves to server.yml)
- Create admin web UI for config (operator edits server.yml directly)
- Store user accounts in database (database is for resource state only)
- Use server.yaml (must be server.yml — auto-migrate on startup)
- Use generic variable names: MODE, CONFIG — use intent-revealing names

## CRITICAL - ALWAYS DO

- Config file: server.yml (not .yaml)
- Boolean parsing: config.ParseBool() — accepts: 1/0, yes/no, true/false, enable/disable, on/off, oui/non, da/nein, etc.
- YAML comments above keys only, single line, ≤140 characters
- server.yml is sole source of truth for operator configuration
- Hot reload: watch server.yml for changes, reload without restart
- Port selection: random 64000-64999 on first run, then save and persist
- Auto-migrate server.yaml → server.yml on startup

## Configuration File Location

| User Type | Path |
|-----------|------|
| Root | /etc/apimgr/search/server.yml |
| Regular | ~/.config/apimgr/search/server.yml |

## Boolean Values Accepted

Truthy: 1, y, t, yes, true, on, ok, enable, enabled, yep, yup, yeah, aye, si, oui, da, hai, affirmative, accept, allow, grant, sure, totally
Falsy: 0, n, f, no, false, off, disable, disabled, nope, nah, nay, nein, non, niet, iie, lie, negative, reject, block, revoke, deny, never, noway

## Environment Variables

### Runtime (Always Checked)
- NO_COLOR — disable ANSI color output
- TERM — terminal type; dumb forces CLI mode
- DOMAIN — FQDN override
- MODE — production (default) or development
- DATABASE_DRIVER — sqlite or libsql
- DATABASE_URL — database connection string
- SMTP_* — email configuration

### Init-Only (First Run Only)
- CONFIG_DIR, DATA_DIR, LOG_DIR, DATABASE_DIR, BACKUP_DIR
- PORT, LISTEN, APPLICATION_NAME, APPLICATION_TAGLINE

## Application Modes

| Mode | Debug | Logging | Debug Endpoints |
|------|-------|---------|-----------------|
| production | false | info, minimal | Disabled (404) |
| production | true | verbose | Enabled |
| development | false | debug | Disabled |
| development | true | full | Enabled |

## Debug Detection Priority
1. --debug CLI flag
2. DEBUG environment variable (truthy)
3. Default: false

## Mode Shortcuts
- --mode dev or --mode development → development
- --mode prod or --mode production → production

## Maintenance Mode
- Only TWO critical errors: database connection failure, cannot write files
- All other errors: server self-heals, never enters maintenance
- In maintenance: read-only, 503 for writes, self-healing in background
- Recovery: automatic when issue resolves

## Path Security (Global Rule)
- All paths normalized with path.Clean()
- PathSecurityMiddleware MUST be first in chain (before auth, routing)
- Block: .., %2e%2e, uppercase letters in path segments
- SafePath() and SafeFilePath() functions required in src/path/

## Middleware Execution Order
1. URLNormalizeMiddleware (first)
2. RequestIDMiddleware
3. PathSecurityMiddleware
4. SecurityHeadersMiddleware
5. AllowlistMiddleware
6. BlocklistMiddleware
7. RateLimitMiddleware
8. GeoIPMiddleware
9. AuthMiddleware
10. LoggingMiddleware (last)

For complete details, see AI.md PART 5, 6, 12
