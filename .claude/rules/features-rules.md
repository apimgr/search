# Feature Rules (PART 17-22)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO
- Introduce paid tiers, external cron, embedded GeoIP data, or a MaxMind fallback.
- Let missing remote datasets prevent startup or turn optional feature faults into server outages.
- Restore untrusted backups without the specified authorization, limits, and validation.

## CRITICAL - ALWAYS DO
- Use the built-in scheduler for scheduled work and keep its state persistent.
- Validate GeoIP MMDB metadata and known-address lookups; treat GeoIP as fail-open risk signal.
- Keep backup encryption, integrity checks, restore authorization, and rollback behavior per spec.
- Implement email, metrics, and update behavior with documented configuration and failure handling.

## Key rules
- GeoIP sources are ip-location-db GitHub Release assets; do not use upstream checksum files.
- Keep operator-facing feature pages, APIs, config, and docs paired and synchronized.

For complete details, see AI.md PART 17-22.
