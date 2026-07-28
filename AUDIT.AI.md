# Project Audit

Started: 2026-07-27

Scope: AI.md PART 11 (Security & Logging) compliance audit — Sec-Fetch-*
request validation, Clear-Site-Data emission, and verification that the
Compliance Standards / Abuse Detection / IP Block Management / Data Protection
sections are actually implemented (not just documented).

## Pass 1: Security

- [ ] src/security/cve.go (line 70-73): `github_advisory` CVE source URL is
  `https://github.com/advisories?query=type%3Areviewed`, which returns an HTML
  page. `parseSource` routes unknown sources to `parseGenericJSON`, which cannot
  unmarshal HTML, so the source silently contributes 0 entries while writing an
  HTML body to a `.json` file. Needs a real JSON data source (OSV.dev API or the
  github/advisory-database git archive) or removal — data-source choice is a
  design decision, deferred pending operator/spec direction.

## Pass 5: Spec Compliance (PART 11 documented-but-unimplemented)

- [ ] Abuse Detection subsystem (AI.md PART 11 "Abuse Detection"): no
  `abuse_detection` config block, no `request_flood` multiplier/block_duration,
  no `auto_block_ip` / `auto_alert` auto-actions, no injection-pattern database
  anomaly detection. Large feature — not part of the Sec-Fetch/Clear-Site-Data
  pass. Static `blocked_ips` + `allowlist` enforcement IS implemented
  (src/config/config.go:663-665, src/server/middleware.go Allowlist/Blocklist).

- [ ] IP Block Management dynamic model (AI.md PART 11 "IP Block Management"):
  the `IPBlock` data model (temporary vs permanent, `ExpiresAt`, `OffenseCount`,
  `AutoBlocked`), the every-minute auto-release scheduler task, and offense
  counting are not implemented. Only the static config-file blocked_ips /
  allowlist path exists. Depends on the Abuse Detection subsystem above.

- [ ] Compliance Standards framework (AI.md PART 11 "Compliance Standards",
  lines 15228+): the multi-standard list (gdpr/ccpa/pdpa/…), the Compliance
  Routes (`/server/dpo` DPO contact, others), the compliance-requirements matrix
  behaviors, and compliance audit events are not implemented. What IS present:
  `ComplianceConfig{Enabled}` gating backup-encryption (PART 22 scope,
  src/config/config.go:985) and the CCPA consent handler
  (src/server/pages.go handleConsentCCPA). Large feature — deferred.

## Completed

- src/security/pgp.go (IdentityMatches): the `appName` parameter was accepted
  but unused — only the email was validated, so any key with the correct
  security-contact email passed regardless of its identity name. Per AI.md
  PART 11 the key identity must match "{appName} Security <{securityContact}>".
  Fixed: now validates both `UserId.Name == "{appName} Security"` AND
  `UserId.Email == securityContact`. Test extended with a mismatched-app-name
  case (src/security/pgp_test.go).

- Sec-Fetch-* request validation (AI.md PART 11): implemented
  `Middleware.SecFetch` + `isCSRFExempt` (src/server/middleware.go), wired into
  the chain after SecGPC (src/server/server.go), config field
  `Headers.SecFetchValidation` (default true) + `CSRF.ExemptPaths`, i18n keys
  errors.sec_fetch_{cross_site,navigate,frame} (en.json). Table-driven tests in
  src/server/secfetch_test.go (all pass).

- Clear-Site-Data emission (AI.md PART 11): implemented
  `Server.setClearSiteData` (src/server/pages.go) with cookie-include and
  executionContexts permutations, config `Headers.ClearSiteData`
  {OnTokenRevocation, OnConsentWithdrawal, ExecutionContexts}. Wired into
  handleConsent + handleConsentCCPA (consent withdrawal, cookies excluded to
  preserve the essential cookie) and handleAlertDelete (token revocation, full
  clear). Table-driven tests in src/server/secfetch_test.go (all pass).
