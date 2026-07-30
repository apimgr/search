# Frontend Rules (PART 16)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO

- NEVER use client-side frameworks (React, Vue, Alpine, jQuery), bundlers (webpack/vite/rollup), transpilers (TypeScript/Babel), or npm/node for the frontend
- NEVER emit inline event handlers (`onclick`, `onchange`, etc.) — CSP blocks them; bind via `data-action` + `addEventListener` in `static/js/app.js`
- NEVER use `alert()`, `confirm()`, or `prompt()` — use custom modal / native `<dialog>` / custom input modal instead
- NEVER put inline styles (`style="..."`) in HTML — all styles live in CSS files
- NEVER use `!important` in CSS except print styles
- NEVER put the theme class on `<body>` — it goes on `<html>`
- NEVER pin/fix header, nav, or footer to the viewport (`position: fixed`/`sticky` forbidden on them) — everything scrolls with the page
- NEVER create layout-scoped or duplicate theme CSS files — one global theme system for the whole project
- NEVER link to server-administration routes from public nav/footer — there is no admin web UI
- NEVER pass user-controlled content through `template.HTML` unless it went through an approved sanitizer first
- NEVER split JavaScript across multiple files — everything goes in the single `static/js/app.js`
- NEVER list authenticated/server-management pages or `/api/*` routes in `sitemap.xml`
- NEVER render SEO/verification meta tags with empty, invalid, or unvalidated content (XSS risk)
- NEVER fetch remote branding images over `http`, from localhost, `.local`/`.internal` hosts, or private/loopback IPs (SSRF)
- NEVER let a page be totally broken with JavaScript disabled — core CRUD and navigation must work via plain HTML forms
- NEVER define page-specific partials (if used once, it is not a partial) or let a page skip the mandatory header/nav/footer partials

## CRITICAL - ALWAYS DO

- ALWAYS render all HTML server-side using Go's `html/template` (`.tmpl` files); auto-escaping stays on
- ALWAYS build mobile-first: base CSS = mobile, scale up with `min-width` media queries (768px tablet, 1024px desktop)
- ALWAYS support three themes — dark (default), light, auto — via a server-readable `theme` cookie rendered into the `<html class="theme-*">` attribute (no FOUC, no `matchMedia` JS needed for auto mode)
- ALWAYS define colors once as CSS custom properties (`--bg-color`, `--text-color`, etc.) in `common.css` and reuse everywhere; the same palette also drives CLI/TUI/GUI/Swagger/GraphiQL theming from `src/common/theme/colors.go`
- ALWAYS meet WCAG 2.1 AA — 4.5:1 text contrast, visible focus indicators, full keyboard nav, `aria-live`/`aria-label` where needed, `prefers-reduced-motion` support, no color-only signaling
- ALWAYS support CRUD through three channels: HTML forms (browser), JSON API endpoints, and form-encoded direct POST (CLI/scripting)
- ALWAYS give copy-to-clipboard buttons visible "Copied!" feedback (icon + text, `.copied` class, `aria-live="polite"`, revert after 2s)
- ALWAYS give every list/table/data view an empty state (icon + title + message + action) — never blank space
- ALWAYS validate forms with HTML5 attributes first (`required`, `pattern`, `type=`), style errors with `:user-invalid`, validate on blur not keystroke
- ALWAYS embed all templates, CSS, JS, images, fonts, and app data in the binary via `//go:embed`; only security-sensitive data (GeoIP, blocklists, CVE DBs, TLS certs) is fetched externally
- ALWAYS end non-HTML responses (JSON/text) with exactly one trailing `\n`
- ALWAYS use the unified JSON envelope `{"ok": true, "data": {...}}` / `{"ok": false, "error": "CODE", "message": "..."}` and the text envelope `OK: {message}` / `ERROR: {code}: {message}` for CLI/API clients, chosen via `detectClientType()` (Accept header, then User-Agent)
- ALWAYS keep error pages (400/401/403/404/500/502/503) themed via `error.tmpl` extending `public.tmpl` — never a raw/unstyled error page
- ALWAYS prefer HTML5/CSS solutions over JavaScript (`<details>`/`<summary>`, checkbox-hack menus, `:focus-within` dropdowns, native `<dialog>`) before writing JS
- ALWAYS keep the footer pinned to the bottom via flex layout (`min-height: 100vh` body + `flex: 1` main), never floating mid-page
- ALWAYS auto-close modals after any action button is clicked — never require a manual extra close click
- ALWAYS serve a dynamically generated `/sitemap.xml` (index+split files past 50,000 URLs)

## Key Rules Summary

- **Server-side rendering**: All HTML via `html/template`; templates organized under `layout/`, `partial/`, `page/`, `component/`; every page must include the mandatory `head`, `header`, `nav`, `footer`, `scripts` partials
- **Mobile-first/responsive**: Container widths 90% (≥768px) / 98% (<768px); nav collapses to a right-sliding, checkbox-driven hamburger menu with overlay (CSS-only, no JS); 44x44px minimum touch targets; theme toggle always stays in the header, never inside the mobile menu
- **Theme (dark/light/auto)**: dark is the default; theme is stored in a `theme` cookie, rendered server-side onto `<html>`; `auto` resolves via pure CSS `prefers-color-scheme` with zero detection JS; toggle JS only sets the cookie and swaps the class, all styling stays in CSS
- **No hardcoded colors**: every color is a CSS custom property defined once in `common.css`/`colors.go`, consumed by web CSS, Swagger, GraphiQL, CLI, TUI, and GUI alike
- **Toasts vs modals**: toasts = non-blocking/transient (success/info 3s, warning 5s, error never auto-dismisses, stack top-right, max 5 visible, pause-on-hover); modals = blocking decisions/destructive confirms, prefer native `<dialog>` for focus trap + Esc + backdrop with zero extra JS
- **JS discipline**: one file (`static/js/app.js`), progressive enhancement only, never load-bearing for core functionality; small one-off behaviors bound via `[data-action]` + `addEventListener`
- **Templates/assets**: `.tmpl` extension only, everything embedded in the binary; CSS split into `common.css` → `components.css` → `public.css` (load order matters); BEM-like naming; app-specific partials allowed alongside the mandatory set, named `{thing}-{purpose}.tmpl`
- **Branding/SEO**: cosmetic only (titles, logos, meta tags) — never touches system paths/binary/route names; verification meta tags and custom tags must pass format/length validation before rendering; remote branding images require HTTPS + SSRF-safe URL validation
- **Reserved routes / URL normalization**: canonical URLs have no trailing slash (redirect 301 to canonical); a fixed set of route names (api, server, static, healthz, search, docs, etc.) is reserved and cannot be claimed by project slugs; route priority is `/api/*` > `/server/healthz` > `/static/*` > `/server/*` > reserved names > project routes

For complete details, see AI.md PART 16.
