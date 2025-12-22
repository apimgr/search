# TODO.AI.md - Search Project Task Tracking

**Last Updated:** 2025-12-21
**Project:** apimgr/search

---

## Status: All Requirements Complete

All specification requirements have been implemented and verified against TEMPLATE.md (13,966 lines).

---

## Latest Audit (2025-12-21)

### TEMPLATE.md Full Re-Read Complete

The entire TEMPLATE.md specification (13,966 lines, 32 PARTs) was read and verified.

### Dockerfile Compliance (PART 13)
| Requirement | Status |
|-------------|--------|
| Multi-stage: golang:alpine → alpine:latest | ✅ |
| ARG: TARGETARCH, VERSION, BUILD_DATE, COMMIT_ID | ✅ |
| CGO_ENABLED=0 | ✅ |
| Required packages: curl, bash, tini, tor | ✅ |
| ENTRYPOINT: tini with SIGTERM propagation | ✅ |
| STOPSIGNAL: SIGRTMIN+3 | ✅ |
| HEALTHCHECK: 10m/5m/15s/3 | ✅ |
| EXPOSE 80 | ✅ |
| ENV MODE=development | ✅ |
| OCI labels | ✅ |

### Makefile Compliance (PART 11)
| Requirement | Status |
|-------------|--------|
| 4 targets: build, release, docker, test | ✅ |
| COMMIT_ID (no VCS_REF alias) | ✅ |
| Uses golang:alpine via Docker | ✅ |
| CGO_ENABLED=0 | ✅ |
| 8 platform builds (4 OS × 2 arch) | ✅ |
| Multi-stage Dockerfile (no pre-built binaries) | ✅ |
| BUILD_DATE format: "Thu Dec 17, 2025 at 18:19:24 EST" | ✅ |

### GitHub Actions Compliance (PART 14)
| Workflow | Status | Key Points |
|----------|--------|------------|
| release.yml | ✅ | 8 platforms, RELEASE_TAG logic, COMMIT_ID |
| beta.yml | ✅ | Linux only, prerelease |
| daily.yml | ✅ | Rolling daily tag, Linux only |
| docker.yml | ✅ | All pushes → devel, tags → latest, COMMIT_ID |

---

## Build Infrastructure

### Dockerfile (`docker/Dockerfile`)
| Requirement | Status |
|-------------|--------|
| golang:alpine builder | [x] DONE |
| alpine:latest runtime | [x] DONE |
| tini as init | [x] DONE |
| CGO_ENABLED=0 | [x] DONE |
| OCI labels | [x] DONE |
| MODE=development (per spec) | [x] DONE |
| COMMIT_ID (not VCS_REF) | [x] DONE |
| Tor installed | [x] DONE |
| STOPSIGNAL SIGRTMIN+3 | [x] DONE |
| Health check (10m/5m/15s) | [x] DONE |

### Makefile
| Requirement | Status |
|-------------|--------|
| golang:alpine via Docker | [x] DONE |
| CGO_ENABLED=0 | [x] DONE |
| 4 targets (build, release, docker, test) | [x] DONE |
| 8 platform builds | [x] DONE |
| LDFLAGS with version info | [x] DONE |
| COMMIT_ID (no VCS_REF alias) | [x] DONE |
| Multi-stage docker build | [x] DONE |

### GitHub Actions
| Workflow | Status |
|----------|--------|
| release.yml (RELEASE_TAG logic) | [x] DONE |
| beta.yml (Linux only) | [x] DONE |
| daily.yml (rolling release) | [x] DONE |
| docker.yml (COMMIT_ID, devel tag) | [x] DONE |

---

## Core Requirements

| Requirement | Status |
|-------------|--------|
| CGO_ENABLED=0 | [x] DONE |
| Pure Go only | [x] DONE |
| modernc.org/sqlite v1.34.4 | [x] DONE |
| Argon2id passwords (Time=3) | [x] DONE |
| Single static binary | [x] DONE |
| Two database architecture | [x] DONE |

---

## Features Implemented

### Infrastructure
- [x] Service support (systemd, launchd, runit, rc.d, Windows)
- [x] Maintenance mode with self-healing
- [x] Startup banner
- [x] Scheduler (cluster-safe)
- [x] GeoIP with MMDB format
- [x] Metrics endpoint (Prometheus)
- [x] Cache/Valkey support

### API
- [x] REST API at /api/v1 (13 endpoints)
- [x] Swagger UI at /openapi (Dracula theme)
- [x] GraphQL at /graphql (Dracula theme)
- [x] OpenAPI JSON at /openapi.json

### Admin Panel
- [x] /admin web UI
- [x] Multiple server admins with invite
- [x] Admin privacy (can't see other admins)
- [x] OIDC/LDAP group mapping

### Email
- [x] 16 email templates
- [x] SMTP auto-detection
- [x] Template preview

### Database
- [x] server.db for admin data
- [x] users.db for user data
- [x] Cluster mode (PostgreSQL, MySQL)
- [x] Node management
- [x] Mixed mode support

### Security
- [x] Audit logging (ULID format)
- [x] Backup/restore with SHA256 checksums
- [x] --maintenance setup command

### I18n
- [x] Accept-Language detection
- [x] Cookie-based persistence
- [x] RTL language support

### User Management
- [x] Username blocklist (100+)
- [x] Recovery keys (10 single-use)
- [x] 2FA with TOTP
- [x] Passkey support (WebAuthn/FIDO2)
- [x] Account email vs Notification email

### Tor
- [x] github.com/cretz/bine integration
- [x] Dedicated Tor process
- [x] Process monitoring with auto-restart
- [x] Vanity address generation (max 6 chars)
- [x] Key import/export

---

## Compilation Fixes (2025-12-21)

| File | Fix Applied |
|------|-------------|
| `src/config/config.go` | Added `SessionDurationDays` to default config struct |
| `src/config/config.go` | Added `SSO` struct to UsersConfig (OIDC + LDAP) |
| `src/api/auth.go` | Added `context` import |
| `src/api/auth.go` | Fixed `h.config.Users` → `h.config.Server.Users` |
| `src/api/user.go` | Fixed `h.config.Users` → `h.config.Server.Users` |
| `src/server/auth.go` | Fixed `s.config.Users` → `s.config.Server.Users` |
| `src/server/auth.go` | Renamed `getClientIP` → `getClientIPSimple` (avoid redeclaration) |
| `src/server/user.go` | Fixed `s.config.Users` → `s.config.Server.Users` |
| `src/server/middleware.go` | Added `ValidateToken()` method to CSRFMiddleware |
| `src/server/embed.go` | Added `Extra` field to PageData struct |

**Build Status:** ✅ Compiles successfully with `CGO_ENABLED=0`

---

## Notes

- All builds use Docker with `golang:alpine`
- Test directories use `/tmp/apimgr-test/search/`
- CGO_ENABLED=0 for all builds
- Comments always above code, never inline
- AI.md contains the complete project specification (13,966 lines)

## Recent Fixes (2025-12-20)

### Spec Compliance Audit

| Item | Fix Applied |
|------|-------------|
| Dockerfile MODE | Changed from `production` to `development` per spec |
| Dockerfile ARG | Changed `VCS_REF` to `COMMIT_ID` |
| Makefile | Removed `VCS_REF` alias, use `COMMIT_ID` directly |
| Makefile docker target | Simplified to use multi-stage build (no pre-build) |
| docker.yml | Changed `GIT_COMMIT`/`VCS_REF` to `COMMIT_ID` |
| release.yml | Added RELEASE_TAG logic for v-prefix handling |
| AI.md | Created complete 13,877 line specification from template |

**Note:** `MODE=development` in Dockerfile is intentional per spec.
docker-compose.yml overrides with `MODE=production` for production deployments.

---

## Full TEMPLATE.md Audit (2025-12-20)

**Status:** ✅ ALL FILES COMPLIANT

Complete TEMPLATE.md (13,898 lines) was read and verified against all project files.

### Dockerfile Compliance (PART 13)
| Requirement | Status |
|-------------|--------|
| Multi-stage: golang:alpine → alpine:latest | ✅ |
| ARG: TARGETARCH, VERSION, BUILD_DATE, COMMIT_ID | ✅ |
| CGO_ENABLED=0 | ✅ |
| Required packages: curl, bash, tini, tor | ✅ |
| ENTRYPOINT: tini with SIGTERM propagation | ✅ |
| STOPSIGNAL: SIGRTMIN+3 | ✅ |
| HEALTHCHECK: 10m/5m/15s/3 | ✅ |
| EXPOSE 80 | ✅ |
| ENV MODE=development | ✅ |
| OCI labels | ✅ |

### Makefile Compliance (PART 11)
| Requirement | Status |
|-------------|--------|
| 4 targets: build, release, docker, test | ✅ |
| COMMIT_ID (no VCS_REF alias) | ✅ |
| Uses golang:alpine via Docker | ✅ |
| CGO_ENABLED=0 | ✅ |
| 8 platform builds (4 OS × 2 arch) | ✅ |
| Multi-stage Dockerfile (no pre-built binaries) | ✅ |
| BUILD_DATE format: "Thu Dec 17, 2025 at 18:19:24 EST" | ✅ |

### GitHub Actions Compliance (PART 14)
| Workflow | Status | Key Points |
|----------|--------|------------|
| release.yml | ✅ | 8 platforms, RELEASE_TAG logic, COMMIT_ID |
| beta.yml | ✅ | Linux only, prerelease |
| daily.yml | ✅ | Rolling daily tag, Linux only |
| docker.yml | ✅ | All pushes → devel, tags → latest, COMMIT_ID |

### TEMPLATE.md Sections Verified
- PART 0-4: Critical rules, project structure, paths, configuration
- PART 5-7: App modes, CLI commands, update command
- PART 8-9: Privilege escalation, service support
- PART 10-11: Binary requirements, Makefile (4 targets only)
- PART 12-13: Testing/development, Docker spec
- PART 14: CI/CD workflows (4 workflows)
- PART 15-17: Health/versioning, web frontend, branding
- PART 18-19: Admin panel, API structure (REST/Swagger/GraphQL)
- PART 20-21: SSL/TLS, security headers, logging (6 log types)
- PART 22: User management (admin-only vs multi-user)
- PART 23-24: Database/cluster, backup/restore
- PART 25-26: Email/notifications, scheduler
- PART 27-29: GeoIP, metrics, Tor hidden service
- PART 30-31: Error handling, i18n/a11y
- PART 32: Project-specific sections

---

## Implementation Work (2025-12-20)

### Templates Created

**Auth Templates** (`src/server/templates/pages/auth/`):
| Template | Status | Description |
|----------|--------|-------------|
| `login.tmpl` | ✅ Created | User login with password toggle, SSO support |
| `register.tmpl` | ✅ Created | Registration with password strength, validation |
| `forgot.tmpl` | ✅ Created | Password reset request |
| `verify.tmpl` | ✅ Created | Email verification |

**User Templates** (`src/server/templates/pages/user/`):
| Template | Status | Description |
|----------|--------|-------------|
| `profile.tmpl` | ✅ Created | User profile with avatar, bio, email settings |
| `security.tmpl` | ✅ Created | Password change, 2FA, sessions |
| `tokens.tmpl` | ✅ Created | API token management |

### Translation Files Created

| File | Status | Description |
|------|--------|-------------|
| `en.json` | ✅ Existed | English (base) - 293 keys |
| `de.json` | ✅ Created | German translations |
| `fr.json` | ✅ Created | French translations |
| `es.json` | ✅ Created | Spanish translations |

### Existing Implementation (Verified)

**i18n System** (`src/i18n/`):
- `i18n.go` - Translation manager, Accept-Language parsing, cookie persistence
- `languages.go` - Language metadata with RTL support

**Users System** (`src/users/`):
- `users.go` - User model, Argon2id hashing, 100+ username blocklist
- `auth.go` - Authentication, sessions
- `totp.go` - 2FA/TOTP
- `recovery.go` - Recovery keys
- `tokens.go` - API tokens
- `passkeys.go` - WebAuthn/FIDO2
- `emails.go` - Dual email system (account vs notification)

---

## Root-Level Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/` | GET | Web interface (HTML) |
| `/healthz` | GET | Health check (HTML) |
| `/openapi` | GET | Swagger UI |
| `/openapi.json` | GET | OpenAPI spec (JSON) |
| `/graphql` | GET | GraphiQL interface |
| `/graphql` | POST | GraphQL queries |
| `/api/v1/healthz` | GET | Health check (JSON) |
| `/metrics` | GET | Prometheus metrics |
