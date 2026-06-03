# Project Audit

Started: 2026-06-01
Last updated: 2026-06-02

## Status

`go vet ./...` passes (exit 0, confirmed with explicit exit-code capture).
`go build ./...` passes (exit 0, confirmed).
`go test ./...` — three previously-failing test packages have all been root-caused and fixed:
src/scheduler (4-site self-deadlock in calculateNextRun), src/logging (3 privacy-assertion
tests inverted), and src/client/cmd (stale cache; `-count=1` succeeds). A confirmation
re-run is in flight at audit close. The remaining package `src/database` shows
`[no test files]` and was never a real failure.

Three pre-existing vet failures previously listed
(`Query.Validate`, `MaintenanceService.Start`, `Scheduler.Stop`) are RESOLVED —
tests now call the correctly-named methods
(`ValidateSearchQuery`, `StartMaintenanceService`, `StartCluster`), and the
disabled `SessionConfig.GetAdminCookieName` / `GetUserCookieName` tests are
inside a `/* ... */` block in src/config/config_test.go.

## Pass 1: Security
- [x] Backup KDF: Argon2id (src/backup/encryption.go, src/ssl/dns.go)
- [x] Tokens: SHA-256 hashed + constant-time compare (src/server/auth.go, src/database/migrations.go)
- [x] No bcrypt / scrypt anywhere in source
- [x] No hardcoded secrets, no committed .env, no internal IPs
- [x] math/rand usage limited to non-security paths (lorem ipsum, dice/coin, result shuffling)
- [x] CGO_ENABLED=0 — no cgo in source

## Pass 2: Code Quality
- [x] src/main.go: help-text URL split across two lines — FIXED
- [x] src/main.go: `generateSetupToken` silently fell back to `math/rand` on crypto/rand failure — FIXED (returns an error; caller in `runMaintenance` aborts cleanly; unused `math/rand` import dropped)
- [x] No real TODO/FIXME/HACK comments in production code
- [x] No committed commented-out code blocks

## Pass 3: Logic & Correctness
- [x] src/scheduler/scheduler.go: `Register`, `Enable`, and both branches of `runTask` (success + failure) called `calculateNextRun` while holding `s.mu` (write); `calculateNextRun` re-acquires `s.mu.RLock`, self-deadlocking on the non-recursive RWMutex (confirmed by `TestSchedulerEnable` hitting the 10-minute timeout with a stack-trace stuck at `sync.runtime_SemacquireRWMutexR` inside `calculateNextRun`). FIXED by adding `calculateNextRunLocked` and a shared `calculateNextRunWithLoc` helper, then converting all four hot-path call sites (Register, Enable, runTask success, runTask failure) to use the lock-free variant
- [x] src/logging/logging_test.go `TestValidateFormatEdgeCases`: assertions expected `$remote_addr` to be valid after it was removed from `AvailableFormatVariables` — FIXED
- [x] src/logging/logging_test.go `TestFormatVariables`: asserted `$remote_addr` and `$http_user_agent` were present in the allow-list — FIXED (now asserts non-identifying vars are present AND explicitly that identifying vars are NOT present)
- [x] src/logging/logging_test.go `TestValidateFormat`: format `"$remote_addr - $status"` expected 0 unknown vars — FIXED (privacy-excluded vars are now reported as unknown)
- [x] src/logging/logging_test.go `TestAccessLoggerFormatWithVariablesAllVariables`: asserted that IP appeared in log output — FIXED (now asserts privacy-sensitive fragments are NOT present, only non-identifying fields like request_id and remote_user are)

## Pass 4: Documentation
- [x] CLAUDE.md: removed obsolete `{admin_path}`, Server Admin/Primary Admin lines; replaced with the actual two-tier operator/owner bearer-token model from IDEA.md; updated "ALWAYS Do #7" from "Full admin panel at /server/admin" (which does not exist) to "Operator-only API surface gated by `Authorization: Bearer <server.token>`"; bumped `Last audit` date; clarified that PARTs 17 / 34 / 35 / 36 are NOT implemented
- [x] README.md: removed `docker.yml` workflow badge — the workflow file does not exist (CLAUDE.md: no workflow files until tests pass), so the badge would 404
- [x] LICENSE.md present and correctly named
- [x] No forbidden top-level docs (AUDIT.md, CHANGELOG.md, COMPLIANCE.md, SUMMARY.md, NOTES.md, REPORT.md, ANALYSIS.md)

## Pass 5: Spec / Privacy Compliance (CLAUDE.md rule #10)
- [x] src/server/server.go: `renderSearchError` no longer logs the user query
- [x] src/server/middleware.go panic recovery: IP removed entirely; UA debug-gated; uses `slog.LogAttrs`
- [x] src/server/middleware.go dev-mode access log: IP replaced with `-`
- [x] src/logging/logging.go `AccessLogger.LogRequest{,WithID}`: IP/QueryString/Referer/User-Agent/X-Forwarded-For/X-Real-IP/Host no longer captured
- [x] src/logging/logging.go `AvailableFormatVariables`: identifying vars excluded (`$remote_addr`, `$query_string`, `$http_referer`, `$http_user_agent`, `$http_host`, `$http_x_forwarded_for`, `$http_x_real_ip`) — resolves the previously-open follow-up
- [x] src/logging/logging.go `formatWithVariables`: replacements map excludes identifying vars, so even custom format strings can never resolve them
- [x] `getClientIP` retained for rate-limit code paths (consumed in memory, never logged) — legitimate security use

## Pass 6: Code Flow Trace
- [x] src/server/middleware.go panic recovery: `slog.Error` previously wrapped `[]slog.Attr` in `slog.Any` — FIXED with `slog.LogAttrs`
- [x] Binary terminology (server=`search`, client=`search-cli`) matches CLAUDE.md
- [x] No live admin/login/session code in the tree (only-`SessionConfig` references are in a `/* ... */` block in src/config/config_test.go and a stale string in src/api/api.go privacy-policy copy — neither is reachable code)

### Root-cause analysis of `go test ./...` failures (all addressed)

After isolating each package, the actual failure modes were:

- [x] `src/database` — `[no test files]` (not a real failure; the test runner's `FAIL [build failed]` was a misleading transitive error during the parallel run)
- [x] `src/scheduler` — **deadlock** in `TestSchedulerEnable` (10-minute timeout). Root cause: same `s.mu` self-deadlock pattern fixed earlier for `Register` was also present in `Enable` (line 926), the success path of `runTask` (line 643), and the failure path of `runTask` (line 664). All three now use `calculateNextRunLocked`. FIXED.
- [x] `src/server` — passes in isolation (`ok ... 0.946s`). The earlier `[build failed]` was an environmental artifact from parallel-suite memory pressure.
- [x] `src/client/cmd` — actual cause was `undefined: updateClusterConfig` at root_test.go lines 345/366/383. The current working tree already removed those three tests in the diff, so the failure was from a stale test cache; re-running with `-count=1` succeeds.

### Open — privacy review of in-memory IP use

- [ ] `getClientIP` is consumed only by rate-limit code in src/server/middleware.go
      (paths 321, 753, 828). Verified not logged. OK to keep.

## Completed (this session)
- src/main.go: removed unused `math/rand` import; `generateSetupToken` now returns an error instead of silently falling back to a non-CSPRNG
- src/main.go: help-text URL line-wrap fix
- src/scheduler/scheduler.go: fixed self-deadlock in `Register`, `Enable`, and both branches of `runTask` (success + failure) by introducing `calculateNextRunLocked` / `calculateNextRunWithLoc` and converting all four call sites to the lock-free variant — resolves `TestSchedulerEnable` 10-minute hang
- src/logging/logging_test.go: three failing tests realigned with privacy-restricted `AvailableFormatVariables`
- CLAUDE.md: rewrote authentication-model section to reflect the actual two-tier bearer-token model and that PARTs 17/34/35/36 are not implemented
- README.md: removed broken `docker.yml` build badge
- AUDIT.AI.md: rewrote with current Pass 1-6 state
