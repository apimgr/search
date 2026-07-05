# Frontend Rules (PART 16)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO

- Hardcode colors — use CSS custom properties
- Default to light mode (dark mode is default)
- Use inline styles (all styles via CSS classes)
- Ignore mobile responsiveness (mobile-first from day one)
- Hardcode English text in templates (all text via i18n keys)
- Use JavaScript frameworks for simple interactions (prefer vanilla JS or HTMX)

## CRITICAL - ALWAYS DO

- Dark mode as default; support dark/light/auto
- Mobile-responsive from day one
- All user-facing text via i18n key lookups
- CSS custom properties for theming (never hardcoded colors)
- Keyboard shortcuts for power users
- Accessible (WCAG AA minimum)
- Preferences stored in localStorage (no account required)

## Theme System

```css
:root {
  --color-bg: #1a1a2e;
  --color-surface: #16213e;
  --color-primary: #e94560;
  --color-text: #eaeaea;
  --color-text-muted: #a0a0b0;
}

[data-theme="light"] {
  --color-bg: #f5f5f5;
  --color-surface: #ffffff;
  --color-primary: #c0392b;
  --color-text: #1a1a2e;
  --color-text-muted: #555566;
}
```

## User Preferences (localStorage)

Stored client-side, no account required:
- Theme (dark/light/auto)
- Language
- Safe search level
- Results per page
- Default search engine weight overrides
- Keyboard shortcut preferences

Exportable as preference string (shareable URL param).

## Branding Configuration

```yaml
server:
  branding:
    title: "Search"
    tagline: ""
    description: ""
  seo:
    keywords: []
```

## Template Structure

```
src/
└── server/
    └── templates/
        ├── base.html       # Base layout with i18n support
        ├── search.html     # Main search page
        ├── results.html    # Results page
        ├── prefs.html      # Preferences page
        └── error.html      # Error pages
```

## i18n in Templates

```html
<!-- WRONG -->
<h1>Search Results</h1>

<!-- CORRECT -->
<h1>{{ t "search.results_title" }}</h1>
```

For complete details, see AI.md PART 16
