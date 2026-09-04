# Frontend Rules (PART 16)

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

## CRITICAL - ALWAYS DO
- Render server-side with Go templates from `src/server/template`; serve static assets from `src/server/static`
- Serve the homepage at `/`
- Support light, dark, and auto themes with dark as the DEFAULT, a visible toggle, and persistence in the `theme` cookie (server renders the class on `<html>`)
- Design mobile-first and responsive, with touch targets at least 44x44px
- Meet WCAG 2.1 AA: semantic HTML, working keyboard navigation, screen reader compatibility, AA color contrast in BOTH themes
- Treat JavaScript as progressive enhancement only
- Justify every `<script>` in a diff with a capability impossible in HTML5 + CSS

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
