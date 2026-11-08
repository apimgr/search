# Translation Guide

Thank you for helping translate Search. The English locale at
`src/common/i18n/locales/en.json` is the source of truth for message keys and
meaning. Translators edit their locale file, never `en.json`.

## Current languages

Search ships 15 embedded locale files. The registry and default-enabled locale
set contain the same languages.

| Code | Language | Native name | Direction |
|------|----------|-------------|-----------|
| `ar` | Arabic | العربية | RTL |
| `de` | German | Deutsch | LTR |
| `en` | English | English | LTR |
| `es` | Spanish | Español | LTR |
| `fa` | Persian | فارسی | RTL |
| `fr` | French | Français | LTR |
| `he` | Hebrew | עברית | RTL |
| `it` | Italian | Italiano | LTR |
| `ja` | Japanese | 日本語 | LTR |
| `nl` | Dutch | Nederlands | LTR |
| `pl` | Polish | Polski | LTR |
| `pt` | Portuguese | Português | LTR |
| `ru` | Russian | Русский | LTR |
| `ur` | Urdu | اردو | RTL |
| `zh` | Chinese | 中文 | LTR |

Locale credits belong in each file's `meta.translators` field when contributors
provide a preferred name or handle. The project does not claim individual
translation credits without contributor consent.

## Contribute or update a translation

Anyone may contribute. Open a pull request with one locale file, or send that
file to the maintainer. Updating an existing language follows the same process:
edit only `src/common/i18n/locales/{lang}.json`. Adding a new language also
requires its registry and `config.server.i18n.available_languages` entries;
see [Configuration](configuration.md) and [Development](development.md).

Locale files use UTF-8 JSON with the same structure and key order as `en.json`,
two-space indentation, no trailing commas, and no comments. Keep every key and
its nesting unchanged; translate values only. Every locale must have the same
key set as English and must not contain empty values.

## Translation rules

- Preserve every interpolation placeholder exactly, including its name and
  braces (`{count}`, `{total}`, `{datetime}`, and all others). Its position in
  the sentence may change. Placeholders must match the English source.
- Include every CLDR plural category required by the language: `zero`, `one`,
  `two`, `few`, `many`, and `other`, as applicable. Check the language's CLDR
  cardinal rules; missing categories fail validation.
- Write whole phrases and sentences. Do not translate fragments on the
  assumption that the UI will join them together. Report unclear or incomplete
  English source strings as project issues.
- Keep a consistent formal register and a short, neutral tone. Avoid slang,
  humor, and idioms that do not translate clearly.
- Leave product and brand names, protocol and technology names, code, file
  names, configuration keys, URLs, and literal user-entered values unchanged.
- Follow local typography, including spacing before punctuation and quotation
  marks. Locale-aware numbers and dates come from the application formatter.
- AI translation is expected and encouraged. The AI writes natural, idiomatic
  translations for languages it can translate reliably; those languages do
  not wait for a native speaker before shipping. If the AI cannot produce a
  reliable translation for a language, wait for a human translation instead
  of shipping guesses. Human translators may refine or correct translations
  at any time, and their corrections take precedence in later updates.

### Glossary

| English term | Translation guidance |
|--------------|----------------------|
| Search | Product name; keep unchanged |
| API | Keep unchanged |
| Tor, `.onion`, I2P | Keep protocol names and `.onion` unchanged |
| JSON | Keep unchanged |
| token | Translate consistently as a bearer credential; do not confuse it with a search result |
| paste | Translate as a text item submitted to a paste service; report source copy that refers to pastes if it does not fit Search |
| rotate | Replace a credential with a new one while preserving its identity |
| revoke | Disable a credential so it can no longer be used |
| linked token | A separately revocable token associated with an owner |

## Review checklist

Before merging, a maintainer checks tests, placeholders, plural categories,
glossary use, and layout. Native-speaker review is welcome but is not a merge
requirement. Review the page in the target language at 320, 768, and 1280 CSS
pixels. Check right-to-left languages in their mirrored direction. Long
translations must not clip, overflow, or break controls. Scripts that the
system font stack may not cover need an appropriate `--font-family` entry and
a check that long unspaced text wraps.

Run the project test suite in Docker to verify locale keys, placeholders,
plural categories, and embedded locale files. Record reviewer sign-off in the
pull request, and update the README language list and this page's language
count when adding a locale.
