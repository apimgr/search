# Project Audit

Started: 2026-06-01

## Pass 1: Security
(no Tier-1 secrets found in source; Argon2id used for backup KDF; CSPRNG used appropriately)

## Pass 2: Code Quality
- [x] src/main.go: help text had reversed URL split across two lines — FIXED (now `For more information: https://github.com/apimgr/search`)

## Pass 3: Logic & Correctness
- [ ] src/model/query_test.go:65 — references `Query.Validate` which doesn't exist on the type; `go vet` fails. Test or code is out of sync (pre-existing).
- [ ] src/service/service_test.go:281 — references `MaintenanceService.Start` which doesn't exist on the type; `go vet` fails (pre-existing).
- [ ] src/scheduler/cluster_test.go:798 — references `Scheduler.Stop` which doesn't exist on the type; `go vet` fails (pre-existing).

## Pass 4: Documentation
(README/LICENSE/CLAUDE.md present; help string and shell completions consistent)

## Pass 5: Spec / Privacy Compliance (CLAUDE.md rule #10: "Log user queries or IPs → privacy is the product, no server-side logs")
- [x] src/server/server.go:1163 — `renderSearchError` logged the user query verbatim. FIXED (query no longer logged; only the error is logged).
- [x] src/server/middleware.go:611-627 — panic-recovery `slog.Error` included `remote_addr` (client IP) and `user_agent` in production. FIXED (IP removed entirely; UA gated to development/debug; switched to `slog.LogAttrs` so attrs are top-level fields, not collapsed into a single `attrs` value).
- [x] src/server/middleware.go:556-566 — dev-mode access log printed the client IP to stdout. FIXED (IP replaced with `-`).
- [x] src/logging/logging.go:408+ — `AccessLogger.LogRequest` / `LogRequestWithID` recorded `IP`, `QueryString`, `XForwardedFor`, `XRealIP`, `Referer`, `User-Agent`, `Host` to access.log on every request. FIXED (IP set to `-`, query string / referer / user-agent / forwarded headers no longer captured; only method/path/status/size/latency/TLS metadata recorded).
- [x] src/logging/logging_test.go — three tests previously asserted that IPs WERE present in the log. FIXED (inverted to assert IPs are NOT present, per privacy posture).
- [ ] src/logging/logging.go custom format / `formatWithVariables` still exposes `$remote_addr`, `$http_x_forwarded_for`, `$http_x_real_ip`, `$http_referer`, `$http_user_agent`, `$http_host`, `$query_string` as variables that resolve from `AccessEntry`. Since `LogRequest`/`LogRequestWithID` no longer populate those fields, these vars now always resolve to empty/"-" — non-blocking, but the `ValidateFormat` allow-list still advertises them as supported. Consider removing from the allow-list in a follow-up.

## Pass 6: Code Flow Trace
- [x] src/server/middleware.go:627 — `slog.Error("PANIC recovered", slog.Any("attrs", attrs))` wrapped `[]slog.Attr` inside `slog.Any`, collapsing the attrs into a single `attrs` field. FIXED (now uses `slog.LogAttrs` so attrs emit as top-level fields).
- [ ] src/logging/logging.go — `getClientIP` is still used by middleware rate-limit code (paths 321, 753, 828) — legit security use, IP is consumed in memory and never logged. OK.

## Completed
- src/main.go: help text URL fix
- src/server/server.go: stopped logging search query in error path
- src/server/middleware.go: stopped logging IP + UA in panic recovery (UA debug-gated); used `slog.LogAttrs` for correct attr serialization
- src/server/middleware.go: stopped logging client IP in dev-mode access log
- src/logging/logging.go: AccessLogger no longer captures IP / QueryString / Referer / User-Agent / X-Forwarded-For / X-Real-IP / Host
- src/logging/logging_test.go: inverted IP-presence assertions to enforce privacy posture
