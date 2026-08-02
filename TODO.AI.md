## [ ] Add `.gitea/workflows/daily.yml` and `.gitea/workflows/docker.yml` to match `.github/workflows/`
Read: AI.md PART 27
Project already opted into `daily.yml` and `docker.yml` on GitHub — Gitea/Forgejo workflows are missing the equivalents, breaking provider parity. Must use Gitea Actions syntax (see AI.md PART 27 `.gitea/workflows/daily.yml` and `.gitea/workflows/docker.yml` reference sections), SHA-pinned third-party actions, and pass `act --list -W {file}` before commit.

## [ ] Decide whether `beta.yml` is needed (GitHub and Gitea)
Read: AI.md PART 27
No `beta` branch currently exists in the repo. `beta.yml` is optional/project-specific per AI.md PART 27 — only add if a beta release channel is actually adopted. If adopted, add to both `.github/workflows/beta.yml` and `.gitea/workflows/beta.yml` with workflow concurrency per the branch-push auto-cancel policy.

## [ ] Search engine test suite gives false confidence — no live-response verification exists
Read: `src/search/engine/engines_test.go`, `engines2_test.go`, `engines3_test.go`, `engines4_test.go`, `duckduckgo_test.go`, `wolfram_test.go`
Nearly every engine test either (a) mocks `httptest.NewServer` with hand-built fixture HTML/JSON that only proves the parser matches the fixture the test author wrote, not real upstream markup, or (b) calls the real live domain with a 1ms context timeout (`engines_test.go` Yahoo/Yandex/StackOverflow/Baidu-style tests), which guarantees an immediate `context deadline exceeded` and tests nothing but the error path. `stackoverflow.go` has no parsing test at all. Decide on an approach: recorded-fixture ("golden file") tests refreshed periodically against real responses, or an opt-in live-integration test tier run outside normal CI.

## [ ] `wolfram.go` engine is fully implemented and tested but unreachable in production
Read: `src/search/engine/registry.go` line ~134, `src/search/engine/wolfram.go`
`wolfram.go` is commented out of `DefaultRegistry()` in `registry.go:134`, so it never runs regardless of whether its scraping logic is correct. Decide: re-enable it, or remove the dead code entirely (per AI.md PART 1 "no stubs/dead code").

## [ ] `qwant.go` calls a likely-dead public API endpoint
Read: `src/search/engine/qwant.go` line ~61
Calls `api.qwant.com/v3/search/...` — Qwant's public search API was shut down years ago; this is likely a dead endpoint masked by fully-mocked tests. Decide: remove the qwant engine, or find/document a working replacement endpoint.

## [ ] Multiple engines scrape HTML from anti-bot-hardened search sites — high break risk
Read: `src/search/engine/google.go`, `brave.go`, `yandex.go`, `baidu.go`, `startpage.go`, `yahoo.go`, `mojeek.go`
These engines regex/class-name-scrape HTML from sites (Google, Brave, Yandex, Baidu, Startpage, Yahoo, Mojeek) known to CAPTCHA-wall or JS-challenge non-browser clients regardless of header quality, and/or scrape brittle generic CSS class names likely to drift. No live verification exists (see test-suite item above). Decide a strategy per engine: switch to an official API where one exists, accept scraping with monitoring/alerting for silent breakage, or drop the engine.

## [ ] `youtube.go` extracts an undocumented, frequently-changing JSON blob
Read: `src/search/engine/youtube.go` lines ~82-95
Regex-extracts the `ytInitialData` JSON blob from HTML (the code's own comment at lines 92-95 admits this is fragile); YouTube changes this schema without notice. No schema-drift detection exists — tests use static offline JSON fixtures only.

## [ ] `openstreetmap.go` violates Nominatim usage policy (User-Agent + rate limiting)
Read: `src/search/engine/registry.go` line ~12, `src/search/engine/openstreetmap.go`
Sends the shared generic browser `UserAgent` (registry.go:12) instead of an app-identifying one, and implements no 1-req/sec throttling — both required by Nominatim's usage policy and risk an IP ban.

## [ ] Several engines are unauthenticated and subject to strict rate limits under real traffic
Read: `src/search/engine/github.go`, `stackoverflow.go`, `pubmed.go`, `reddit.go`
`github.go` sends no auth token (10 req/min unauthenticated cap), `stackoverflow.go` sends no `key=` param (strict shared-IP quota), `pubmed.go` sends no `api_key`/`email`/`tool` params (enforced 3 req/sec limit without a key), and `reddit.go` sends a generic Chrome UA rather than the descriptive contact-info UA Reddit's API rules require (elevated 403/429 risk). Decide whether to add operator-configurable API keys/contact info for these.
