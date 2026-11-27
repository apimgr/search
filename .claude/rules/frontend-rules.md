# Frontend Rules (PART 16)
<!-- Synchronized with AI.md revision c2f65bf274b5. -->

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

Read: `AI.md` PART 16 (Web Frontend).

## CRITICAL - NEVER DO
- Use client-side rendering (React, Vue, or any SPA framework) — server-side Go templates only
- Use inline CSS — external stylesheets only
- Use JavaScript `alert()`/`confirm()` — toast notifications instead
- Add JavaScript for anything HTML5 + CSS already does (forms, validation, disclosure, dialogs, tabs use native mechanisms)
- Hardcode a color — use the theme tokens
- Ship a feature whose core functionality requires JavaScript
- Let long strings (IPv6, `.onion`, tokens, hashes) overflow — they need `word-break: break-all`
- Cache or serve an offline navigation copy of a private, authenticated, token-bearing, or independently form-bearing page
- Allow a service worker `respondWith()` branch to reject or resolve without a real `Response`; every handled request needs a final offline/error response

## CRITICAL - ALWAYS DO
- Render server-side with Go templates from `src/server/template`; serve static assets from `src/server/static`
- Serve the homepage at `/`
- Support light, dark, and auto themes with dark as the DEFAULT, a visible toggle, and persistence in the `theme` cookie (server renders the class on `<html>`)
- Design mobile-first and responsive, with touch targets at least 44x44px
- Meet WCAG 2.1 AA: semantic HTML, working keyboard navigation, screen reader compatibility, AA color contrast in BOTH themes
- Treat JavaScript as progressive enhancement only
- Justify every `<script>` in a diff with a capability impossible in HTML5 + CSS
- Support PWA installation and offline behavior, with service worker and manifest responses using `no-cache` and a build-stamp ETag
- Cache only public navigation pages explicitly marked `X-Offline-Cache: public`; use network-first navigation with an 8-second bound, then a cached public page or synthesized 503 page. Keep APIs and non-GET/cross-origin requests outside service-worker interception
- Cache only successful, non-redirected responses; static assets are cache-first with a final 504 fallback. Version caches with `{project_version}-{short_commit}`, clear old version caches during activation, and keep offline strings separately
- Keep service-worker and manifest responses `no-cache` with a build-stamp ETag; never require the visitor to clear browser data to recover
- Check page layout without relying on an `overflow-x` backstop
- Use the shared `.container` width modifiers and standard page shell, cards, and controls; never add page-specific wrappers or classes
- Add `Access-Control-Allow-Origin: *` to API endpoints and implement the PART 16 CORS policy for browser requests
- Apply the same CORS policy to every route: configured/derived origins or `*`; credentials are allowed only for explicit origins, and every permitted request header is named (never `Access-Control-Allow-Headers: *`)
- Normalize URLs before routing: remove trailing slashes (except `/`) with a 301 while preserving the query string; keep explicit file paths intact
- Build absolute links with the request-resolved FQDN through `BuildURL(r, path)`; do not read `server.fqdn` as a URL source
- Keep the specified middleware order, beginning with URL normalization, request ID, path security, and security headers
- Render Markdown only through Goldmark with raw HTML disabled, then sanitize with the allow-list policy; apply that policy to operator HTML as well
- Keep public navigation free of server-administration links, dashboards, settings pages, and hints to admin routes

## KEY DECISIONS (pre-answered)
| Question | Answer | Reference |
|----------|--------|-----------|
| Rendering? | Server-side Go templates | PART 16 |
| Default theme? | dark | PART 16 |
| Theme persistence? | `theme` cookie, class rendered on `<html>` | PART 16 |
| Inline CSS? | NEVER | PART 16 |
| JS alerts? | NEVER — toasts | PART 16 |
| Works without JS? | Yes — always | PART 16 |
| Accessibility bar? | WCAG 2.1 AA | PART 16, 30 |
| API CORS origin? | `Access-Control-Allow-Origin: *`, with the defined preflight and allow-list policy | PART 16 |
| PWA? | Installable and offline-capable | PART 16 |
| URL canonical form? | No trailing slash except `/`; normalize before route processing | PART 16 |

## TERMINOLOGY
| Term | Meaning |
|------|---------|
| progressive enhancement | Core function works without JS; JS only improves it |
| toast | Non-blocking in-page notification replacing `alert()` |
| auto theme | Follows the OS `prefers-color-scheme` setting |

## QUICK REFERENCE
- Templates: `src/server/template`
- Static assets: `src/server/static`
- Theme tokens: `src/common/theme`
- I18N strings: `src/common/i18n` (see `testing-rules.md`, PART 30)

---

For complete details, see AI.md PART 16
