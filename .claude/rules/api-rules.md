# API Rules (PART 13, 14, 15)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

Read: `AI.md` PART 13 (Health & Versioning), PART 14 (API Structure), PART 15 (SSL/TLS & Let's Encrypt).

## CRITICAL - NEVER DO
- Expose sensitive data (tokens, credentials, filesystem paths, internal IPs) in a healthz response
- Introduce a breaking API change without a major version bump
- Omit the API version from the URL path
- Hand-roll certificate handling outside `src/ssl`
- Redirect `/healthz` — when `server.healthz.root.enabled=true` it maps directly to the same handler
- Use `localhost` in docs or README public URLs — use `https://scour.li`

## CRITICAL - ALWAYS DO
- Serve `/server/healthz` (frontend, smart detection) and `/api/{api_version}/server/healthz` (API, supports `.txt`)
- Return 200 OK when healthy; smart-detect browser → HTML, CLI → formatted text
- Include in the extended healthz payload: `version`, `go_version`, build info; `features` (tor, geoip, metrics); `checks` (database, cache, disk, scheduler); `stats` (`requests_total`, `requests_24h`, `active_connections`)
- Keep the version in `release.txt`, in the healthz response, and in `--version`, using semantic versioning `MAJOR.MINOR.PATCH`
- Serve REST under `/api/{api_version}/` with a consistent response envelope and proper HTTP status codes
- Paginate every list endpoint and support filter/sort/search parameters
- Rate-limit per endpoint type
- Support Let's Encrypt HTTP-01 and TLS-ALPN-01 (DNS-01 optional) plus manual certificates
- Auto-renew certificates via the scheduler, monitor expiry, and send HSTS headers when SSL is enabled

## KEY DECISIONS (pre-answered)
| Question | Answer | Reference |
|----------|--------|-----------|
| Health endpoint? | `/server/healthz` + `/api/{api_version}/server/healthz` | PART 13 |
| Root `/healthz`? | Only if `server.healthz.root.enabled=true`, same handler | PART 13 |
| Versioning scheme? | Semver `MAJOR.MINOR.PATCH` from `release.txt` | PART 13 |
| API base path? | `/api/{api_version}/` | PART 14 |
| Cert handling location? | `src/ssl` only | PART 15 |
| Cert renewal? | Automatic, via the built-in scheduler | PART 15 |

## TERMINOLOGY
| Term | Meaning |
|------|---------|
| smart detection | Content negotiation: browser gets HTML, CLI gets text |
| `{api_version}` | Version segment in the API URL path |
| `official_site` | `https://scour.li` — the public URL used in all docs |

## QUICK REFERENCE
- API routes: `src/api`, `src/graphql`
- SSL/TLS: `src/ssl`
- Metrics endpoints: see `features-rules.md` (PART 20)
- ACME challenges: HTTP-01, TLS-ALPN-01, DNS-01 (optional)

---

For complete details, see AI.md PART 13, 14, 15
