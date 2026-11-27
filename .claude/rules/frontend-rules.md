# Frontend Rules (PART 16)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO
- Use client-side rendering frameworks or add JavaScript for HTML/CSS-native behavior.
- Use hardcoded colors, fixed desktop-only layouts, or oversized unbounded SVGs.
- Make core pages or controls require JavaScript.

## CRITICAL - ALWAYS DO
- Render pages with Go templates on the server and make all features work without JavaScript.
- Build mobile-first responsive layouts with dark/light/auto themes and accessible markup.
- Bound every SVG; use the prescribed icon sizes and accessible names/hidden decoration.
- Apply `word-break` rules to prevent long strings from breaking mobile layouts.

## Key rules
- Reuse shared components and styles; keep all server pages consistent with PART 16.
- Keep theme and direction handling in server-rendered HTML/CSS.

For complete details, see AI.md PART 16.
