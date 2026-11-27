# API Rules (PART 13, 14, 15)
<!-- Synchronized with AI.md revision f5ea82acc67d. -->

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

Read: `AI.md` PART 13 (Health & Versioning), PART 14 (API Structure), PART 15 (SSL/TLS & Let's Encrypt).

## CRITICAL - NEVER DO
- Expose sensitive data (tokens, credentials, filesystem paths, internal IPs) in a healthz response
- Wrap health JSON in the normal API envelope; health JSON is bare on every health route
- Introduce a breaking API change without a major version bump
- Omit the API version from the URL path
- Hand-roll certificate handling outside `src/ssl`
- Redirect `/healthz` — when `server.healthz.root.enabled=true` it maps directly to the same handler
- Use `localhost` in docs or README public URLs — use `https://scour.li`

## CRITICAL - ALWAYS DO
- Serve `/server/healthz` (frontend, smart detection) and `/api/{api_version}/server/healthz` (API, supports `.txt`)
- Also expose `/api/healthz` as a direct unversioned JSON alias; only expose root `/healthz` when configured, using the same handler
- Keep the complete public-safe health schema and exact field order from PART 13: project, status/restart state, version/build, runtime, public features, checks, aggregate stats, then IDEA-defined app fields
- Support the health states `healthy`, `degraded`, `unhealthy`, `restart_required`, `maintenance`, and `shutting_down`; include pending-restart reasons only when present and maintenance self-healing details only during maintenance
- Keep health check keys stable and public-safe; disabled optional datasets are omitted and checks are cached. Database/disk errors make health `unhealthy`; a dataset at `degraded` makes it `degraded`, while a dataset error makes it `unhealthy` only when registered `critical`; optional downloaded datasets/upstreams and Tor/I2P are never critical
- Return 200 when healthy; frontend health content negotiation follows PART 14 and API health is JSON by default with supported text output
- Keep the version in `release.txt`, in the healthz response, and in `--version`, using semantic versioning `MAJOR.MINOR.PATCH`
- Serve REST under `/api/{api_version}/` with a consistent response envelope and proper HTTP status codes
- Format JSON with two-space indentation and one trailing newline; success/error envelopes have explicit exceptions for health, compatibility protocols, binary responses, MCP JSON-RPC, and GraphQL's standard response object
- Keep canonical API routes versioned, lowercase, plural, hyphenated, and without trailing slashes; `/api/{api_version}/` redirects with 301 to `/api/{api_version}`
- Restrict unversioned API routes to the documented aliases (`/api/swagger`, `/api/graphql`, `/api/healthz`, `/api/metrics[/{service}]`, `/api/mcp`, `/mcp`) and `/api/autodiscover`; aliases use the same handlers and have parity tests
- Paginate every list endpoint and support filter/sort/search parameters
- Rate-limit per endpoint type
- Determine frontend response type in order: explicit `Accept` (`application/json`, `text/html`, `text/plain`), companion CLI, text browser, known HTTP tool, then browser HTML
- Treat HTTP tool detection as a User-Agent denylist; empty User-Agent and unrecognized clients follow the documented defaults
- Keep API negotiation separate: `.txt`, explicit JSON/plain-text, HTTP tool, then JSON default
- Use one route definition for router registration, OpenAPI, llms.txt, and text serialization; serialize response structs in declared field order and maps by sorted key
- Keep API CORS on the same configured-origin policy as other routes; `*` never permits credentials, and credentialed browser requests require an explicitly allowed origin
- Pair user-facing project API features with server-rendered pages; machine-only endpoints and privileged API-project features do not get browser forms without a defined browser credential
- Apply owner-token scopes on owner resources and management routes: `read` permits GET/HEAD/OPTIONS, `read-write` adds mutation methods, and `global` is required for token management; insufficient scope returns 403 with `WWW-Authenticate: Bearer error="insufficient_scope"`
- Keep the route inventory explicit and drive router, OpenAPI, `llms.txt`, GraphQL and MCP drift checks from registered routes; internal routes remain absent from public API documents
- Support Let's Encrypt HTTP-01 and TLS-ALPN-01 (DNS-01 optional) plus manual certificates
- Support DNS-01 with provider credentials encrypted at rest; validate them at startup and before certificate requests
- Resolve request hosts only through the documented trusted proxy policy; static FQDN uses DOMAIN, hostname, public IPs, then localhost
- Build embedded absolute URLs from request context with `BuildURL(r, path)`; never derive public URLs from `server.fqdn` or hardcode a host
- Keep `/api/autodiscover` as the non-versioned discovery document with the exact fields `name`, `version`, `api`, `urls`, optional exposed `onion`/`i2p`, `cli_versions`, and `cli_min_version`
- Auto-renew certificates via the scheduler, monitor expiry, and send HSTS headers when SSL is enabled

## KEY DECISIONS (pre-answered)
| Question | Answer | Reference |
|----------|--------|-----------|
| Health endpoint? | `/server/healthz` + `/api/{api_version}/server/healthz` | PART 13 |
| Health JSON shape? | Bare body, exact canonical field order; no standard `{ok,data}` envelope | PART 13 |
| Root `/healthz`? | Only if `server.healthz.root.enabled=true`, same handler | PART 13 |
| Versioning scheme? | Semver `MAJOR.MINOR.PATCH` from `release.txt` | PART 13 |
| API base path? | `/api/{api_version}` (no trailing slash) | PART 14 |
| Cert handling location? | `src/ssl` only | PART 15 |
| Cert renewal? | Automatic, via the built-in scheduler | PART 15 |

## TERMINOLOGY
| Term | Meaning |
|------|---------|
| smart detection | Frontend content negotiation: CLI gets JSON, text browsers get no-JS HTML, HTTP tools get formatted text, browsers get HTML |
| `{api_version}` | Version segment in the API URL path |
| `official_site` | `https://scour.li` — the public URL used in all docs |

## QUICK REFERENCE
- API routes: `src/api`, `src/graphql`
- SSL/TLS: `src/ssl`
- Metrics endpoints: see `features-rules.md` (PART 20)
- ACME challenges: HTTP-01, TLS-ALPN-01, DNS-01 (optional)

---

For complete details, see AI.md PART 13, 14, 15
