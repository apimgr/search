# API Rules (PART 13, 14, 15)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO

- Return different JSON shapes for success vs error (use canonical {ok, error, message, details})
- Use /healthz at root level without the /server/ prefix (canonical: /server/healthz)
- Skip request validation — validate all inputs
- Return 200 for errors (use correct HTTP status codes)
- Mix camelCase and snake_case in JSON (use snake_case always)
- Expose internal error details in production (generic messages only)
- Skip rate limiting on any public endpoint

## CRITICAL - ALWAYS DO

- JSON keys: snake_case always
- API versioning: /api/v1/ prefix for all API routes
- Health endpoint: /server/healthz (canonical) — /healthz is optional alias
- Canonical error body: {ok: false, error: "CODE", message: "...", details: {}}
- Canonical success body: {ok: true, data: {}}
- Validate all inputs with go-playground/validator/v10
- Rate limit all public endpoints

## Route Structure

```
/                           → Search UI (HTML)
/search                     → Search handler
/api/v1/                    → API root
/api/v1/search              → Search API
/api/v1/engines             → Engine status
/api/v1/instant             → Instant answers
/api/v1/preferences         → User preferences
/api/v1/alerts              → Search alerts (operator auth)
/server/                    → Server management
/server/healthz             → Health check
/server/status              → Detailed status (operator token)
/server/config              → Config viewer (operator token)
/debug/                     → Debug endpoints (--debug only)
/debug/pprof/               → pprof profiles (--debug only)
```

## Health Check Response

```json
{
  "status": "ok",
  "version": "1.0.0",
  "mode": "production",
  "uptime": "2d 5h 30m",
  "checks": {
    "database": "ok",
    "disk": "ok",
    "config": "ok"
  }
}
```

## HTTP Status Code Rules

| Situation | Code |
|-----------|------|
| Success with data | 200 |
| Created | 201 |
| No content | 204 |
| Bad request / validation error | 400 |
| Unauthorized (no/invalid token) | 401 |
| Forbidden (valid token, no permission) | 403 |
| Not found | 404 |
| Rate limited | 429 |
| Server error | 500 |
| Maintenance mode | 503 |

## API Versioning

- URL prefix: /api/v1/ (api_version configurable in server.yml)
- Version header: X-API-Version: v1
- Always return current version in response headers

## SSL/TLS (PART 15)

Auto-detection order for certificates:
1. /etc/letsencrypt/live/domain/ (certbot managed)
2. /etc/letsencrypt/live/{fqdn}/ (certbot managed)
3. {config_dir}/ssl/letsencrypt/{fqdn}/ (app managed, auto-renew)
4. {config_dir}/ssl/local/{fqdn}/ (user managed, no auto-renew)

Let's Encrypt challenges:
- Port 80: HTTP-01
- Port 443: TLS-ALPN-01
- DNS: DNS-01 (manual or DNS provider plugin)

Min TLS version: TLS 1.2

For complete details, see AI.md PART 13, 14, 15
