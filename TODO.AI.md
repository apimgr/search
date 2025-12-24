# TODO.AI.md - Search Project Task Tracking

**Last Updated:** 2025-12-23 (AI.md synced with TEMPLATE.md v17420)
**Project:** apimgr/search
**TEMPLATE.md Version:** 17,420 lines (33 PARTs)

---

## Current Status: SYNCED & COMPLIANT

AI.md has been fully synced with the updated TEMPLATE.md (17,420 lines, 33 PARTs).

---

## Sync Summary (2025-12-23)

- **TEMPLATE.md fully read:** All 17,420 lines, 33 PARTs
- **AI.md regenerated:** From TEMPLATE.md with project variables replaced
- **Variables replaced:**
  - `{projectname}` → `search`
  - `{projectorg}` → `apimgr`
  - `{gitprovider}` → `github`
  - `{maintainer_name}` → `Jason Hempstead`
  - `{maintainer_email}` → `jason@casjaysdev.pro`

---

## Compliance Audit (2025-12-23)

### Core Requirements

| Requirement | Status | Notes |
|-------------|--------|-------|
| **CGO_ENABLED=0** | PASS | All builds use `CGO_ENABLED=0` |
| **Single static binary** | PASS | Embedded assets |
| **8 platforms** | PASS | linux, darwin, windows, freebsd x amd64, arm64 |
| **Config: server.yml** | PASS | YAML config file |
| **Docker multi-stage** | PASS | Builder + runtime stages |
| **Docker port 80** | PASS | Internal port 80 |
| **tini init** | PASS | Uses tini |
| **Tor auto-detect** | PASS | tor binary installed in image |

### PART Compliance Matrix

| PART | Name | Status |
|------|------|--------|
| 0 | AI Assistant Rules | PASS |
| 1 | Critical Rules | PASS |
| 2 | Project Structure | PASS |
| 3 | OS-Specific Paths | PASS |
| 4 | Configuration | PASS |
| 5 | Application Modes | PASS |
| 6 | CLI Interface | PASS |
| 7 | Update Command | PASS |
| 8 | Privilege Escalation | PASS |
| 9 | Service Support | PASS |
| 10 | Binary Requirements | PASS |
| 11 | Makefile | PASS |
| 12 | Testing & Dev | PASS |
| 13 | Docker | PASS |
| 14 | CI/CD Workflows | PASS |
| 15 | Health & Versioning | PASS |
| 16 | Web Frontend | PASS |
| 17 | Branding & SEO | PASS |
| 18 | Admin Panel | PASS |
| 19 | API Structure | PASS |
| 20 | SSL/TLS | PASS |
| 21 | Security & Logging | PASS |
| 22 | User Management | PASS |
| 23 | Database & Cluster | PASS |
| 24 | Backup & Restore | PASS |
| 25 | Email & Notifications | PASS |
| 26 | Scheduler | PASS |
| 27 | GeoIP | PASS |
| 28 | Metrics | PASS |
| 29 | Tor Hidden Service | PASS |
| 30 | Error Handling | PASS |
| 31 | I18N & A11Y | PASS |
| 32 | Project-Specific | PASS |
| 33 | CLI Client | PASS |

---

## TEMPLATE.md Key Updates (v17420)

The following key updates were synced from the updated TEMPLATE.md:

1. **Tor Auto-Detection** (PART 13, 29)
   - Changed from `ENABLE_TOR=true` to auto-detection
   - "Tor auto-enabled if `tor` binary installed"
   - Docker image always has Tor

2. **Entrypoint.sh Updates** (PART 13)
   - Tor auto-detection logic
   - Improved signal handling
   - SIGRTMIN+3 trap support

3. **CLI Client Naming** (PART 33)
   - User-Agent format: `{projectname}-cli/{version}`
   - Confirmed bubbletea/bubbles/lipgloss for TUI

4. **Makefile dev Target** (PART 11)
   - Outputs to `{TMPDIR}/apimgr.XXXXXX/search`
   - Organization-prefixed temp directory

---

## Implementation Status

### Fully Implemented
- Server binary with all CLI flags
- Admin panel with web UI
- REST API with all endpoints
- Swagger/OpenAPI documentation
- GraphQL endpoint
- PWA support (manifest.json, sw.js)
- Scheduler with 11 built-in tasks
- Tor hidden service integration
- Multi-platform builds (8 platforms)
- Docker image with multi-arch support
- CI/CD workflows (release, beta, daily, docker)
- CLI client (search-cli) with TUI

---

## Quick Reference

### Build Commands
```bash
make dev      # Quick dev build (temp dir)
make build    # Full build (all platforms)
make test     # Run tests
make docker   # Build and push Docker image
```

### Run Locally
```bash
# After make dev, output shows path like:
# Built: /tmp/apimgr.XXXXXX/search

# Run with:
/tmp/apimgr.*/search --help
/tmp/apimgr.*/search --version
/tmp/apimgr.*/search  # Start server
```

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
| `/admin` | GET | Admin panel login |
| `/admin/*` | ALL | Admin panel pages |

---

## Final Compliance Checklist

| Category | Requirement | Status |
|----------|-------------|--------|
| **Core** | CGO_ENABLED=0 | PASS |
| **Core** | modernc.org/sqlite | PASS |
| **Core** | 4 OS x 2 arch builds | PASS |
| **Config** | server.yml (not .yaml) | PASS |
| **Build** | 4 Makefile targets | PASS |
| **CI/CD** | 4 GitHub workflows | PASS |
| **Docker** | tini + Alpine base | PASS |
| **Docker** | STOPSIGNAL SIGRTMIN+3 | PASS |
| **API** | REST /api/v1 | PASS |
| **API** | Swagger /openapi | PASS |
| **API** | GraphQL /graphql | PASS |
| **Admin** | /admin web UI | PASS |
| **Scheduler** | 11 built-in tasks | PASS |
| **Tor** | cretz/bine library | PASS |
| **CLI Client** | search-cli implemented | PASS |

---

## Notes

- AI.md synced with TEMPLATE.md v17420 on 2025-12-23
- All 33 PARTs compliant
- No critical gaps identified
- All builds use Docker with `golang:alpine`
- CGO_ENABLED=0 for all builds
