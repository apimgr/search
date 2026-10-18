# Search syntax

Search uses direct upstream search engines and combines their results. Search operators are normalized before a query is sent to each engine.

## Bang shortcuts

Use `!shortcut terms` to redirect a search to a built-in engine or site. In Preferences, custom bangs are stored in this browser and use a URL template containing `{query}`. The autocomplete suggests matching custom bangs; submitting one replaces `{query}` with the URL-encoded search terms. Custom targets must be HTTP or HTTPS URLs without embedded credentials.

## Image results

Image thumbnails and full-size previews load through the signed same-origin
image proxy by default. Selecting a thumbnail opens a preview panel with image
dimensions when available, a link to the source page, and a download action.
The REST and GraphQL search APIs expose the available image URL, dimensions,
format, and file size; unknown metadata is omitted. The proxy rejects private
destinations, redirects, unsupported media, and images above its configured
byte limit or built-in dimension limits.

## Boolean operators

`AND`, `OR`, and `NOT` are case-insensitive. Adjacent terms mean `AND`; `NOT` binds most tightly, followed by `AND`, then `OR`. Parentheses group expressions, and quoted phrases stay together as one search term.

```text
golang AND concurrency
golang OR rust AND concurrency
(privacy OR security) AND "search engine"
golang AND NOT java
NOT (malware OR phishing)
```

Malformed expressions, such as a trailing `AND` or an unmatched parenthesis, are rejected before any search engine is called.

## Suggestions

Search autocomplete uses DuckDuckGo suggestions. The related-search API combines DuckDuckGo and Google suggestions with local query variations. Successful autocomplete and related-search responses are cached for one hour, with bounded response sizes and cache entry counts; provider redirects and failed responses are ignored. Queries sent to these suggestion providers are visible to those providers.

## Instant answers

Weather answers show current conditions and a five-day outlook from Open-Meteo. In Preferences, the weather widget can follow the global units preference or use a metric/imperial override. Currency answers use Frankfurter daily exchange rates, calculate conversions locally, and show a seven-day rate chart; currency amounts are not sent to the provider.

Video results keep their proxied static thumbnails until a same-origin WebM/MP4 preview clip is available. **Video previews** can be disabled in Preferences to avoid loading clips on hover or touch.

URL safety answers report related domains only when a one-edit lookalike is present in a configured local domain blacklist. Reputation providers that cannot verify a result remain Unknown.

## Direct answers

Use a `type:term` prefix to open a full-page answer. Common types include `tldr:`, `man:`, `wiki:`, `pkg:`, `cve:`, `dns:`, `whois:`, `resolve:`, `cert:`, `headers:`, `safe:`, and `cache:`. See the project IDEA for the full list and each answer's data sources.

`wiki:` provides the article summary, a contents list, expandable text-only sections, infobox fields when present, related articles, and a language selector. Parsed Wikimedia markup is converted to escaped text before it reaches the page; if article details are unavailable, the summary remains available.

`cache:` checks the Wayback Machine, Common Crawl, and Archive.today, shows available capture dates, and provides sanitized archived and live previews side by side when both are available. Archive.today dates come from its Memento timeline and open at Archive.today. Wayback captures include view and download links; Common Crawl captures include a bounded preview and attachment download. Previews are bounded, and live fetches reject private destinations and unsafe redirects. `safe:` checks only the domain root and reports matches from configured, loaded domain blocklists. Set `GOOGLE_SAFE_BROWSING_API_KEY` and/or `PHISHTANK_API_KEY` to enable those reputation providers. Every configured reputation provider must return clean for a Safe rating; missing keys, unavailable providers, and inconclusive results remain Unknown.

`asn:` reads current registration and observed routing data from the RIPEstat Data API. Results include announced prefixes and unique address counts, observed upstream/downstream positions, registry country when available, neighbor history, and looking-glass links. RIS observations can be delayed or incomplete; path positions do not prove commercial relationships. Optional routing sections are omitted when their data source is unavailable. See the [RIPEstat Data API documentation](https://stat.ripe.net/docs/data-api/ripestat-data-api).

`dict:` renders English definitions from the [Wiktionary API](https://en.wiktionary.org/api/rest_v1/), with optional pronunciation and etymology metadata from the [Free Dictionary API](https://dictionaryapi.dev/). It also shows an optional Datamuse estimate based on Google Books Ngrams. Frequency is shown as estimated occurrences per million words. Audio is fetched through `/direct/audio` from the supported HTTPS pronunciation hosts, without following redirects; the proxy accepts only audio media types and bounded response sizes. See the [Datamuse API](https://www.datamuse.com/api/) documentation for frequency data.

`thesaurus:` combines Datamuse synonym, antonym, and related-word results with optional Free Dictionary senses and examples. [ABC Thesaurus](https://abcthesaurus.com/data/) adds sense-level formal/informal register labels and a sourced −2 to +2 intensity scale. Result pages attribute this metadata to ABC Thesaurus and link its CC BY-SA 4.0 license. Provider failures leave other available thesaurus content visible.

The beautifier accepts indentation options before the source, for example `beautify:js --indent=4 function run(){return 1}` or `beautify:json --minify {"ok": true}`. Minification is supported for JavaScript, TypeScript, CSS, HTML, XML, JSON, and PHP; languages without a parser-backed minifier report that the option is unavailable.

### Open directory search

Use `directory:` to find public directory listings. Optional filters are `type:`, `min-size:`, `after:`, `before:`, and `site:`.

```text
directory:ubuntu iso
directory:programming books type:documents min-size:10M
directory:linux textbook after:2024-01-01 before:2024-12-31 site:edu
```

Each result shows its server header and the count of file links found in the bounded listing response. The default preview shows up to 20 files. **Scan directory** expands one result to at most 500 files. The server validates public addresses and redirect destinations before fetching a listing.

## QR codes

Use `qr:https://example.com` to generate a QR code. The options form supports size, error correction, colors, and PNG/JPEG logo uploads. For URL payloads, **Shorten with this server** creates a random local `/s/{code}` link; no third-party shortener is contacted. Destinations are encrypted at rest with the server encryption key. The short link redirects only to an absolute HTTP or HTTPS URL without credentials.

## Local search history

Recent searches are kept in this browser's `localStorage`; Search does not send the history list to the server. The homepage shows recent queries as links. Disable **Save search history** in Preferences to stop recording and clear the stored list, or use **Clear history** to remove it at any time. When history saving is enabled, it is included in the downloadable preferences export and restored from that file's import. Importing preferences with history disabled clears imported history. History is not included in cross-device preference URLs.
