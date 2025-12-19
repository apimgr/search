# TODO.AI.md - Search Project Task Tracking

**Last Updated:** 2025-12-19
**Audit Source:** TEMPLATE.md comprehensive audit

---

## Critical Issues (Must Fix)

*All critical issues resolved!*

---

## Completed Items

- [x] Makefile uses Docker for ALL builds (GO_DOCKER)
- [x] CLI Flags implemented (--mode, --data, --config, --address, --port)
- [x] Request ID uses UUID v4 format (google/uuid)
- [x] FQDN Validation with IsValidHost() in src/config/validation.go
- [x] Service reload implemented (--service reload)
- [x] Request ID included in logs ($request_id variable)
- [x] Widget system complete (all 12 widgets)
- [x] Admin panel templates (login, dashboard, all settings pages)
- [x] Well-known files (robots.txt, security.txt)
- [x] Audit logging system
- [x] Backup/restore functionality
- [x] Admin API endpoints
- [x] Email template system (10 templates)
- [x] Config options (SEO, compression, i18n)
- [x] Update system
- [x] Scheduler admin UI
- [x] Custom log format variables
- [x] Security headers implementation
- [x] Service file generation (systemd, runit, launchd, rc.d, Windows)
- [x] AI.md file created
- [x] REST API endpoints (13 endpoints)
- [x] Swagger/OpenAPI specification in JSON format (src/api/openapi.json)
- [x] Docker configuration (Dockerfile, docker-compose.yml, entrypoint.sh)
- [x] GitHub Actions (release.yml, beta.yml, daily.yml, docker.yml)
- [x] GraphQL API with full schema (src/api/graphql.go)
- [x] GraphiQL UI with Dracula theme at /graphql GET (per TEMPLATE.md spec)
- [x] GraphQL queries at /graphql POST (per TEMPLATE.md spec)
- [x] Swagger UI with Dracula theme at /openapi (per TEMPLATE.md spec)
- [x] OpenAPI JSON endpoint at /openapi.json (per TEMPLATE.md spec)
- [x] Health endpoints per spec: /healthz (HTML), /api/v1/healthz (JSON) - NO /health anywhere
- [x] Go unit tests (api_test.go, validation_test.go, query_test.go, category_test.go)
- [x] OpenAPI spec includes all REST endpoints (/bangs, /widgets, /instant) - API parity verified
- [x] PART 29: I18N System (src/i18n/i18n.go, languages.go) with Accept-Language parsing
- [x] PART 29: English translation file (src/server/templates/data/en.json) - 200+ keys
- [x] PART 29: RTL language support (Arabic, Hebrew, Persian, Urdu)
- [x] PART 31: User database migrations (users, sessions, 2fa, recovery, tokens, verification)
- [x] PART 31: User model with 100+ blocked usernames (src/users/users.go)
- [x] PART 31: User authentication with sessions (src/users/auth.go)
- [x] PART 31: TOTP 2FA implementation (src/users/totp.go) with pquerna/otp
- [x] PART 31: Recovery keys (src/users/recovery.go) - 10 single-use keys
- [x] PART 31: User API tokens (src/users/tokens.go) with permissions
- [x] PART 31: Email verification & password reset (src/users/verification.go)

---

## Notes

- All builds MUST use Docker with `golang:alpine`
- Test directories MUST use `/tmp/apimgr-test/search/` format
- Never use project directory for test data
- CGO_ENABLED=0 for all builds
- GraphQL and Swagger MUST expose identical functionality as REST API

## Root-Level Endpoints (per TEMPLATE.md line 4530-4545)

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/` | GET | Web interface (HTML) |
| `/healthz` | GET | Health check (HTML) |
| `/openapi` | GET | Swagger UI |
| `/openapi.json` | GET | OpenAPI spec (JSON) |
| `/graphql` | GET | GraphiQL interface |
| `/graphql` | POST | GraphQL queries |
| `/api/v1/healthz` | GET | Health check (JSON) |
