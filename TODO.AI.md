## [x] Add `.gitea/workflows/daily.yml` and `.gitea/workflows/docker.yml` to match `.github/workflows/`
Done: added both files, adapted from the AI.md PART 27 Gitea templates to match this project's established Gitea conventions (GitHub-compatible `$GITHUB_ENV`/`$GITHUB_OUTPUT` env-file names as used in the existing `.gitea/workflows/ci.yml`/`release.yml`/`security.yml`, Docker-run-based builds via `casjaysdev/go:latest`, full Go import-path LDFLAGS, SHA-pinned third-party actions reused verbatim from `.github/workflows/`). `docker.yml` auto-detects the registry from `gitea.server_url` since Gitea is self-hostable with no fixed registry like `ghcr.io`. `act --list -W {file}` fails on both with "Unknown Variable Access gitea" — confirmed this is a pre-existing limitation of `act` (a GitHub-Actions-only runner that doesn't recognize the Gitea-specific `gitea.*` context) by reproducing the identical failure against the already-committed `.gitea/workflows/release.yml`; not a regression introduced by these two files.

## [x] Decide whether `beta.yml` is needed (GitHub and Gitea)
Decision: do not add. No `beta` branch exists in the repo (verified via `git branch -a`), so per AI.md PART 27 ("optional/project-specific — include only when the project requires them") no beta release channel has been adopted and `beta.yml` is correctly omitted on both GitHub and Gitea.

## [ ] Search engine test suite gives false confidence — no live-response verification exists
Read: `src/search/engine/engines_test.go`, `engines2_test.go`, `engines3_test.go`, `engines4_test.go`, `duckduckgo_test.go`
Nearly every engine test either (a) mocks `httptest.NewServer` with hand-built fixture HTML/JSON that only proves the parser matches the fixture the test author wrote, not real upstream markup, or (b) calls the real live domain with a 1ms context timeout (`engines_test.go` Yahoo/Yandex/StackOverflow/Baidu-style tests), which guarantees an immediate `context deadline exceeded` and tests nothing but the error path. `stackoverflow.go` has no parsing test at all. Decide on an approach: recorded-fixture ("golden file") tests refreshed periodically against real responses, or an opt-in live-integration test tier run outside normal CI.

## [x] `wolfram.go` engine is fully implemented and tested but unreachable in production
Read: `src/search/engine/registry.go` line ~134, `src/search/engine/wolfram.go`
`wolfram.go` is commented out of `DefaultRegistry()` in `registry.go:134`, so it never runs regardless of whether its scraping logic is correct. Decide: re-enable it, or remove the dead code entirely (per AI.md PART 1 "no stubs/dead code").
Done: removed entirely, live-confirmed Wolfram Alpha's /input page is now JS-rendered with no scrapable content, per AI.md's no-dead-code rule.

## [x] `qwant.go` calls a likely-dead public API endpoint
Read: `src/search/engine/qwant.go` line ~61
Calls `api.qwant.com/v3/search/...` — Qwant's public search API was shut down years ago; this is likely a dead endpoint masked by fully-mocked tests. Decide: remove the qwant engine, or find/document a working replacement endpoint.
Done: removed entirely, live-confirmed api.qwant.com returns 403 CAPTCHA (DataDome) on every request regardless of headers, not fixable without a paid CAPTCHA-bypass service.

## [ ] Startpage, Mojeek scrape HTML from anti-bot-hardened search sites — high break risk
Read: `src/search/engine/startpage.go`, `mojeek.go`
Live comprehensive beta test (2026-08-01/02) confirmed Google/Yandex/Baidu now detect and surface anti-bot block pages as errors instead of failing silently (see `transport.go` `detectBlockPage`), and Brave's markup drift was root-caused and fixed. Startpage and Mojeek remain unverified against live responses and still regex/class-name-scrape brittle markup with no block-page detection. Decide a strategy per engine: switch to an official API where one exists, accept scraping with monitoring/alerting for silent breakage, or drop the engine.

## [ ] Mojeek, Reddit, Startpage hard-blocked in live testing — needs a product decision
Read: `src/search/engine/mojeek.go`, `reddit.go`, `startpage.go`
Live comprehensive beta test (2026-08-01/02) confirmed these engines are hard-blocked in practice: Mojeek and Startpage return CAPTCHA/Cloudflare challenge pages to non-browser clients regardless of header quality; Reddit's public JSON endpoints are aggressively rate-limited/blocked without an authenticated app. (Qwant had the same problem and was removed entirely — see the `qwant.go` item above, now resolved.) None of these are fixable by header/parser changes alone — each needs a decision: pay for/register an official API key, accept degraded reliability with monitoring, or remove the engine. Awaiting a decision from the user before further action.

## [x] Yahoo blocked at the TLS/JA3 fingerprint level, not just headers
Read: `src/search/engine/yahoo.go`
Live comprehensive beta test (2026-08-01/02) found Yahoo's block is distinct from the generic UA-spoofing issue other engines hit — Go's `net/http`/`crypto/tls` client produces a JA3 TLS fingerprint that Yahoo's edge blocks regardless of User-Agent or other headers. Fixing this would require a custom TLS ClientHello (e.g. via `utls`) to mimic a real browser fingerprint, which is a nontrivial dependency/security-surface decision — needs a decision from the user: add a TLS-fingerprint-spoofing dependency, accept the engine as broken, or remove it.
Done: Added `github.com/refraction-networking/utls` (BSD-3-Clause) scoped only to the Yahoo engine via a new `src/search/engine/yahoo_transport.go` — a custom `http.RoundTripper` that dials a fresh connection per request and performs the TLS handshake with `utls.HelloChrome_Auto` instead of stdlib `crypto/tls`. Live testing found Yahoo's edge answers with an HTTP/2 SETTINGS frame even when only `http/1.1` was offered via ALPN, so the transport offers both `"h2"` and `"http/1.1"` and branches on the negotiated protocol (`golang.org/x/net/http2.Transport.NewClientConn` for h2, manual `req.Write`/`http.ReadResponse` for http/1.1). `yahoo.go` was changed by a single line (`Transport: yahooTransport` instead of `SharedTransport`) — every other engine's `SharedTransport` is untouched. Live-tested against real `https://search.yahoo.com/search?p=test` from a throwaway module (never inside the project tree) and got a genuine 200 with 7 real parsed results (speedtest.net, measurementlab.net, fast.com, etc.) — confirmed this unblocks Yahoo, not just a partial fix. `go build ./...`, `go vet ./...`, and `go test ./src/search/...` all pass.

## [x] i18n locale files still claim Qwant and Wolfram Alpha as aggregated engines
Read: `src/common/i18n/locales/*.json` key `a_engines` (~line 1535 in each of 14 locale files: it, pt, en, ru, fa, ur, zh, ja, ar, nl, fr, es, de, pl, he)
Each locale's translated `a_engines` string lists Qwant and Wolfram Alpha among the aggregated search engines. Both were removed entirely (see the `wolfram.go`/`qwant.go` items above) — these strings are now false claims and need to be updated/retranslated to drop both names, consistent with the `README.md`/`IDEA.md` updates already made.
Done: removed `Qwant, ` and the trailing `, and Wolfram Alpha`/language-specific conjunction equivalent from all 14 locale files' `a_engines` string, verified each file is still valid JSON (`jq empty`).

## [ ] `youtube.go` extracts an undocumented, frequently-changing JSON blob
Read: `src/search/engine/youtube.go` lines ~82-95
Regex-extracts the `ytInitialData` JSON blob from HTML (the code's own comment at lines 92-95 admits this is fragile); YouTube changes this schema without notice. No schema-drift detection exists — tests use static offline JSON fixtures only.

## [ ] `openstreetmap.go` violates Nominatim usage policy (User-Agent + rate limiting)
Read: `src/search/engine/registry.go` line ~12, `src/search/engine/openstreetmap.go`
Sends the shared generic browser `UserAgent` (registry.go:12) instead of an app-identifying one, and implements no 1-req/sec throttling — both required by Nominatim's usage policy and risk an IP ban.

## [ ] Several engines are unauthenticated and subject to strict rate limits under real traffic
Read: `src/search/engine/github.go`, `stackoverflow.go`, `pubmed.go`, `reddit.go`
`github.go` sends no auth token (10 req/min unauthenticated cap), `stackoverflow.go` sends no `key=` param (strict shared-IP quota), `pubmed.go` sends no `api_key`/`email`/`tool` params (enforced 3 req/sec limit without a key), and `reddit.go` sends a generic Chrome UA rather than the descriptive contact-info UA Reddit's API rules require (elevated 403/429 risk). Decide whether to add operator-configurable API keys/contact info for these.
