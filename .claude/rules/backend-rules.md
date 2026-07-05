# Backend Rules (PART 9, 10, 11, 31)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO

- Use mattn/go-sqlite3 (CGO — use modernc.org/sqlite)
- Store operator tokens in plaintext (SHA-256 hash always)
- Log raw tokens, passwords, or API keys
- Use fmt.Println for logging (use log/slog)
- Return stack traces in production error responses
- Create user accounts (two-tier auth: anonymous + operator bearer token only)
- Use goroutines without proper cancellation/shutdown handling
- Skip parameterized queries (SQL injection prevention is mandatory)

## CRITICAL - ALWAYS DO

- Database: modernc.org/sqlite (pure Go, CGO_ENABLED=0)
- Logging: log/slog with structured fields
- Error responses: canonical JSON body {ok, error, message, details}
- Operator token: stored as SHA-256 hash, compared in constant time
- All SQL: parameterized queries only
- Graceful shutdown: context cancellation, drain in-flight requests
- Rate limiting: golang.org/x/time/rate
- CORS: github.com/rs/cors

## Database

### Driver Priority
1. modernc.org/sqlite (local — default)
2. gitlab.com/cznic/sqlite (alternative pure Go)
3. libsql/Turso (remote — when configured)

### Table Prefix
- Server tables: srv_ prefix in remote DB
- Local SQLite: no prefix needed

### Required Tables
| Table | Purpose |
|-------|---------|
| srv_config | Configuration key-value |
| srv_config_meta | Config defaults and restart flags |
| srv_rate_limits | Rate limiting counters |
| srv_audit_log | Config changes, security events |
| srv_scheduler_tasks | Scheduled task definitions |
| srv_scheduler_history | Task execution history |
| srv_backups | Backup metadata |

## Error Response Format

```json
{
  "ok": false,
  "error": "ERROR_CODE",
  "message": "Human readable message (i18n key resolved)",
  "details": {}
}
```

Success:
```json
{
  "ok": true,
  "data": {}
}
```

## Auth (Two-Tier)

| Tier | Who | How |
|------|-----|-----|
| Anonymous | Public users | No auth, rate limited |
| Operator | Server owner | Bearer token (server.token in server.yml) |

- server.token stored as SHA-256 hash in server.yml
- Constant-time comparison (subtle.ConstantTimeCompare)
- In --debug mode: operator token check BYPASSED (dev only)

## Logging (log/slog)

```go
slog.Info("server started", "port", port, "mode", mode)
slog.Error("database error", "err", err, "query", query)
slog.Debug("cache hit", "key", key, "duration_us", d.Microseconds())
```

- Never: fmt.Println, fmt.Printf, log.Printf (old logger)
- Never log: tokens, passwords, API keys, personal data
- Sensitive fields: mask with xxxxx

## Security Headers (Required)

All responses must include:
- X-Content-Type-Options: nosniff
- X-Frame-Options: DENY
- X-XSS-Protection: 1; mode=block
- Referrer-Policy: strict-origin-when-cross-origin
- Content-Security-Policy: (project-appropriate)
- Permissions-Policy: (restrict unnecessary APIs)

## Tor Hidden Service (PART 31)

- Optional: enabled via server.yml tor section
- Library: github.com/cretz/bine
- Auto-generate .onion address on first enable
- Store private key encrypted in config_dir/tor/

For complete details, see AI.md PART 9, 10, 11, 31
