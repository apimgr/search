# TODO.AI.md

## Pending

## [ ] `.claude/settings.json` missing (PART 0 "NOT optional" requirement)
Read: AI.md PART 0

Attempted to create per the PART 0 example config (permissions.allow/deny/ask,
hooks, model, env). The Claude Code auto-mode permission classifier denied the
write as self-modification: an agent granting itself broad wildcard Bash
permissions (`git *`, `docker *`, `mv *`, `cp *`, `chmod *`, ...) exceeds what
the user explicitly authorized. Needs the user to either create the file
directly, approve a specific permission set, or authorize a narrower retry.

## [ ] CVE source URL returns HTML, not JSON
Read: AI.md PART 11

`src/security/cve.go` (line 70-73): the `github_advisory` CVE source URL is
`https://github.com/advisories?query=type%3Areviewed`, which returns an HTML
page. `parseSource` routes unknown sources to `parseGenericJSON`, which cannot
unmarshal HTML, so the source silently contributes 0 entries while writing an
HTML body to a `.json` file. Needs a real JSON data source (OSV.dev API or the
github/advisory-database git archive) or removal — data-source choice is a
design decision, deferred pending direction.

## [ ] Abuse Detection subsystem not implemented
Read: AI.md PART 11

No `abuse_detection` config block, no `request_flood` multiplier/block_duration,
no `auto_block_ip` / `auto_alert` auto-actions, no injection-pattern database
anomaly detection. Static `blocked_ips` + `allowlist` enforcement IS implemented
(src/config/config.go:663-665, src/server/middleware.go Allowlist/Blocklist).
Large feature.

## [ ] IP Block Management dynamic model not implemented
Read: AI.md PART 11

The `IPBlock` data model (temporary vs permanent, `ExpiresAt`, `OffenseCount`,
`AutoBlocked`), the every-minute auto-release scheduler task, and offense
counting are not implemented. Only the static config-file blocked_ips /
allowlist path exists. Depends on the Abuse Detection subsystem above.

## [ ] Compliance Standards framework not implemented
Read: AI.md PART 11

The multi-standard list (gdpr/ccpa/pdpa/...), the Compliance Routes
(`/server/dpo` DPO contact, others), the compliance-requirements matrix
behaviors, and compliance audit events are not implemented. What IS present:
`ComplianceConfig{Enabled}` gating backup-encryption (PART 22 scope,
src/config/config.go:985) and the CCPA consent handler
(src/server/pages.go handleConsentCCPA). Large feature.

## [ ] ConvertHandler regex false-positive on non-unit phrases
Read: AI.md PART 17

`ConvertHandler`'s explicit-direction regex (`src/instant/convert.go`,
`patterns[0]`: `^(?:convert\s+)?(\d+(?:\.\d+)?)\s*([a-zA-Z°]+)\s+(?:to|in|->)\s+([a-zA-Z°]+)$`)
matches any `"<number> <word> to <word>"` phrase, not just real unit names —
e.g. a query like `"10 things to do"` false-positive-matches as a conversion
request (unit="things"->"do") instead of falling through to the normal
search/instant-answer flow. Fix: validate the captured `from`/`to` groups
against the known unit table before treating the query as a conversion; only
treat it as a bare/explicit conversion match if both resolve to a recognized
unit.

## [ ] docker-compose.dev.yml uses `:latest` image tag instead of `:devel`
Read: AI.md PART 26

`docker/docker-compose.dev.yml` line 11 sets `image: ghcr.io/apimgr/search:latest`.
Per AI.md PART 26 the dev compose file must reference the `:devel` tag
(`{PLATFORM_CONTAINER_REGISTRY}/{project_org}/{internal_name}:devel`), built
from `docker/Dockerfile.dev`. Out of PART 0-6 scope for this bootstrap pass.
