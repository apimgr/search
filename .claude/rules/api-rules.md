# API Rules (PART 13, 14, 15)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO

- NEVER expose credentials, connection strings, internal IPs, file paths,
  env vars, secrets, stack traces, or `/metrics` status in `/server/healthz`.
- NEVER add sub-routes under `/server/healthz` (no `/server/healthz/db`).
- NEVER keep legacy/removed project endpoints "for compatibility" — delete
  them outright. No redirects, no deprecation shims.
- NEVER hardcode `v1` — always use `APIBasePath()` / `{api_version}`.
- NEVER use singular resource names, uppercase, underscores, verbs, or
  trailing slashes in routes (`/item`, `/Items`, `/api_keys`, `/getItems`,
  `/items/` are all wrong).
- NEVER emit a bare JSON array at API root — always `{ "data": [...] }`.
- NEVER invent a custom error shape or add ad-hoc top-level fields
  (`reason`, `retry_after`, `status` in body, etc.).
- NEVER manually edit generated `openapi.json` or GraphQL schema files —
  they are build-time generated from code, always.
- NEVER put Swagger/GraphQL/API-root files outside `src/swagger/` /
  `src/graphql/` (never in project root).
- NEVER implement redirects for unversioned aliases (`/api/swagger`,
  `/api/graphql`, `/api/healthz`) — mount the same handler directly.
- NEVER serve icons/emojis/ASCII art in log output — logs are always raw
  plain text.
- NEVER clone an external service's entire API surface unless the user
  explicitly asked for route/API/client compatibility (default = feature
  compatibility only).
- NEVER let the app auto-renew certs under `/etc/letsencrypt/**` (system/
  certbot owns those) or under `{config_dir}/ssl/local/{fqdn}/` (user-
  managed, manual only).
- NEVER set `DOMAIN` to an overlay address (`.onion`, `.i2p`, `.exit`).

## CRITICAL - ALWAYS DO

- ALWAYS version every API route: `/api/{api_version}/...`.
- ALWAYS use plural, lowercase, hyphenated resource names.
- ALWAYS follow content negotiation: `.txt` ext > `Accept: text/plain` >
  non-interactive client > default (JSON for API, HTML for frontend).
- ALWAYS return the canonical error envelope on every 4xx/5xx:
  `{ "ok": false, "error": "CODE", "message": "...", "details": {} }`.
- ALWAYS return `{ "ok": true, "data": {...} }` for action responses
  (create/update/delete); return the item directly (no wrapper) for
  single-item GETs.
- ALWAYS 2-space indent JSON/YAML/HTML/CSS/JS; tabs for Go/Makefiles;
  every response and file ends with exactly one trailing newline.
- ALWAYS keep Swagger and GraphQL specs generated from code and in sync
  with each other and the live API — regenerate at build time.
- ALWAYS give `/server/healthz` (HTML), `/api/{api_version}/server/healthz`
  (JSON default), and `/api/healthz` (unversioned alias, direct mount).
- ALWAYS keep healthz fields public-safe; checks report `ok`/`error` only,
  never internal detail.
- ALWAYS support HTTP-01, TLS-ALPN-01, and DNS-01 Let's Encrypt challenges.
- ALWAYS resolve FQDN in priority order: reverse-proxy headers → `DOMAIN`
  env → `os.Hostname()` → `$HOSTNAME` → public IPv6 → public IPv4 →
  `localhost`.
- ALWAYS check cert lookup order on startup: `/etc/letsencrypt/live/domain/`
  → `/etc/letsencrypt/live/{fqdn}/` → `{config_dir}/ssl/letsencrypt/{fqdn}/`
  → `{config_dir}/ssl/local/{fqdn}/` → request new cert.
- ALWAYS strip `:80`/`:443` from displayed URLs.

## Key Rules Summary

**Health & versioning:**
- `/server/healthz` = HTML/text/JSON via content negotiation; optional
  root alias `/healthz` only when `server.healthz.root.enabled: true`,
  same handler, no redirect.
- Canonical field order: project → status → version/build → runtime →
  features → checks → stats → app-specific extensions.
- SemVer for stable releases (`MAJOR.MINOR.PATCH`, start at `1.0.0`, no
  `v` prefix in the version string, tags get `v` prefix).
- Version sources, priority: `release.txt` → git tag → `dev` fallback.

**API route structure:**
- Route scopes: `/server/*` (server-owned) and `/*` (project resources),
  mirrored as `/api/{api_version}/server/*` and `/api/{api_version}/*`.
- Prefer path params for resource identity; query params for
  pagination/sort/filter only.
- Frontend routes require full CRUD parity with their API and must work
  without JavaScript (progressive enhancement); server does all business
  logic, validation, and rendering — JS is enhancement only.
- Client-type detection drives response shape: our CLI client → JSON;
  text browsers (lynx/w3m) → no-JS HTML; HTTP tools (curl/wget) →
  `HTML2TextConverter` plain text; regular browsers → HTML+JS.
- External-service compatibility defaults to feature/behavior parity via
  our own routes; only add the external route surface when the user
  explicitly asks for route/API/client compatibility.
- RFC-defined protocols (DNS, SMTP, HTTP, WebDAV, etc.) require full RFC
  compliance, not partial "compatibility."
- Pagination default limit is 250; response includes
  `{ data, pagination: { page, limit, total, pages } }`.

**Response formats:**
- Success/error envelopes as above; HTTP status code carries status, never
  duplicated in body.
- REST + Swagger + GraphQL required for every project; both docs UIs match
  project theme (light/dark/auto, dark default).

**SSL/TLS & Let's Encrypt:**
- Built-in Let's Encrypt required for all projects; all DNS-01 providers
  supported via `server.tls.dns_provider` + encrypted credentials.
- Cert directories mirror certbot layout:
  `{config_dir}/ssl/letsencrypt/{fqdn}/` (app-managed, auto-renew 7 days
  before expiry) and `{config_dir}/ssl/local/{fqdn}/` (manual).
- Port rules: single port = HTTP unless 443 (HTTPS-only); dual ports =
  first HTTP, second HTTPS; `ssl.enabled` can force HTTPS on any port.
- Overlay networks (Tor/I2P) use HTTP by default, self-signed certs only
  (LE doesn't cover `.onion`/`.i2p`); they inherit HTTPS-only mode from
  clearnet when clearnet is on port 443.
- Startup banner is responsive (plain/micro/minimal/compact/full by
  terminal width) and never appears in log output, which is always raw
  plain text.

For complete details, see AI.md PART 13, 14, 15.
