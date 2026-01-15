# Search - TODO List

**Project**: search
**Updated**: 2026-01-14
**Spec**: AI.md (~47,696 lines from TEMPLATE.md)

## Status: COMPLIANCE AUDIT COMPLETE

Full AI.md specification compliance audit performed. Results below.

---

## Compliance Summary

| PARTS | Coverage | Status | Notes |
|-------|----------|--------|-------|
| 0-5 | Core Rules | COMPLIANT | Argon2id, SafePath, middleware order |
| 6-10 | Infrastructure | COMPLIANT | Mode, CLI, errors, database |
| 11-15 | Security | 60% | Missing: security headers, Swagger, GraphQL |
| 16-20 | Frontend | 85% | Admin panel complete, email templates missing |
| 21-25 | Operations | 95% | Metrics, backup, scheduler, Tor complete |
| 26-30 | Build | 100% | Makefile, Docker, CI/CD all present |
| 31-36 | Extensions | 40% | I18N partial, A11Y missing, orgs not needed |

---

## Critical Gaps (Priority Order)

### HIGH PRIORITY

1. **Security Headers Middleware** (PART 11)
   - Missing: X-Content-Type-Options, X-Frame-Options, CSP, Referrer-Policy
   - Location: `src/server/middleware.go`
   - Impact: Security vulnerability

2. **Swagger/OpenAPI** (PART 14)
   - Missing: `/openapi` and `/openapi.json` endpoints
   - Missing: Auto-generated spec from code
   - Location: Need `src/swagger/` directory

3. **GraphQL Support** (PART 14)
   - Missing: `/graphql` endpoint
   - Missing: Auto-generated schema
   - Location: Need `src/graphql/` directory

4. **Content Negotiation** (PART 14)
   - Missing: CLI client detection (User-Agent parsing)
   - Missing: Smart format detection (HTML vs JSON vs text)
   - Missing: HTML2TextConverter for CLI users

### MEDIUM PRIORITY

5. **I18N Translation Files** (PART 31)
   - Framework exists in `src/i18n/`
   - Missing: Actual translation files (en.json, es.json, etc.)
   - Missing: `locales/` directory

6. **Accessibility (A11Y)** (PART 32)
   - Missing: WCAG 2.1 AA compliance
   - Missing: Skip links, ARIA patterns, focus management
   - Missing: Screen reader announcements

7. **Email Template Content** (PART 18)
   - Framework exists in `src/email/`
   - Missing: Default template text content
   - Missing: Template customization UI

8. **Tor Admin Features** (PART 32)
   - Service works but missing admin UI
   - Missing: Vanity address generation
   - Missing: Key import/export via UI

### LOW PRIORITY (Optional Features)

9. **Organizations** (PART 35) - NOT NEEDED for search
10. **Custom Domains** (PART 36) - NOT NEEDED for search

---

## Implemented Features

### Core (100%)
- Multi-engine search aggregation
- Search categories (web, images, videos, news)
- Search operators (site:, filetype:, etc.)
- Engine health monitoring and failover

### User System (100%)
- User accounts with Argon2id passwords
- Email verification, 2FA (TOTP), passkeys
- Session management, API tokens
- Profile and preferences

### Admin Panel (95%)
- Full admin route hierarchy
- Server settings, branding, SSL
- Scheduler management
- Log viewing, backup/restore

### Infrastructure (100%)
- Prometheus metrics
- AES-256-GCM encrypted backups
- Built-in scheduler (12 tasks)
- GeoIP support (4 databases)

### Build/Deploy (100%)
- Makefile with 6 targets
- Multi-stage Docker builds
- GitHub + Gitea CI/CD workflows
- Container-only development

---

## IDEA.md Summary

### Core Features
- Multi-engine aggregation with smart ranking
- 10+ search categories (web, images, videos, news, maps, etc.)
- Search operators (site:, filetype:, daterange:, etc.)
- Reliability features (health monitoring, failover, self-healing)

### User Preferences (No Account Required)
1. **localStorage** - automatic browser storage
2. **Import/Export** - JSON backup/restore
3. **Preference String** - compact URL-safe format for sharing

### Instant Answers (26 widgets)
- Calculator, unit/currency converter, weather
- Dictionary, IP lookup, color picker, timezone
- Hash/UUID/password generators, QR codes

### Power User Features
- Vim-style keyboard shortcuts
- Custom bangs (!g, !w, !yt, custom)
- Domain blocklist, custom CSS
- Local-only search history

### Privacy
- Zero server-side tracking
- No cookies required
- Works without JavaScript
- Tor integration

---

## Notes

- AI.md is READ-ONLY (PARTS 0-36)
- IDEA.md contains complete feature specification
- ldflags use `config.*` instead of `main.*` (intentional deviation)
- Organizations and Custom Domains NOT NEEDED for this project
