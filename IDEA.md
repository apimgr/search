## Project description

Search is a privacy-respecting, self-hosted metasearch engine that aggregates results directly from primary search engines (Google, Bing, DuckDuckGo, Brave, Qwant, Mojeek) without tracking users. It combines the best reliability and feature ideas from Whoogle, SearX, and SearXNG into a single, always-working solution — but queries source engines directly rather than through proxies or other metasearch engines.

Target users include privacy-conscious individuals who want web search without being tracked, self-hosters running their own search infrastructure, organizations requiring private internal search, Tor users seeking an .onion-accessible search engine, power users wanting keyboard-driven search with vim-style shortcuts, and developers needing structured API access to aggregated search results.

## Project variables

project_name:     search
project_org:      apimgr
internal_name:    search
app_name:         Search
maintainer_name:  casjay
maintainer_email: casjay@yahoo.com

---

## Business logic

### Product scope & non-goals

`search` aggregates results from primary search engines (Google, Bing, DuckDuckGo, Brave, Qwant, Mojeek, Yandex, Baidu) plus specialized engines (Wikipedia, YouTube, Reddit, StackOverflow, GitHub, Hacker News, arXiv, PubMed, Wolfram Alpha, OpenStreetMap). The full feature inventory lives in `### Features` below.

**Non-goals:**
- No proxying through other metasearch engines — we hit primary sources directly.
- Not included as sources: Ecosia (Bing-powered), SearXNG (metasearch).
- No user accounts on the public surface; preferences are client-side only (localStorage / portable preference strings).
- No server-side query or IP logging for end users.
- No paid tiers, license keys, or feature gating (per AI.md PART 1 — all features free).
- No SPA / client-rendered core UX — server-side rendered, progressive enhancement, mobile-first (per AI.md PART 16).

### Roles & permissions

| Role | Authentication | Permissions |
|------|---------------|-------------|
| Anonymous public user | None | Search, view results, set client-side preferences, create/manage email-verified search alerts via signed manage links |
| Operator | Possession of `server.token` (operator bearer token from `server.yml`) | Operator-only API endpoints (engine state queries, alert moderation, maintenance). Configuration is file-only — all server settings are set in `server.yml` and managed via CLI / config-reload. |

There is **no admin web UI** and **no end-user account system**. The operator manages the service entirely via `server.yml` plus the `search-cli` client. The spec (AI.md) has no regular-user accounts, organization tiers, or custom-domain features — these are simply not in scope.

### Data model & sensitivity

Full schemas are documented under `### Data Models` below. Sensitivity classification:

| Data | Sensitivity | Storage | Retention |
|------|------------|---------|-----------|
| Search queries (per request) | High (potentially identifying) | Memory only — never logged | Discarded after response |
| User preferences | Low | Client-side (localStorage / URL param) | User-controlled |
| Search alerts (email + query + tokens) | Medium (PII: email, signed tokens) | Server DB | Until user deletes; opt-in only with email verification |
| Alert deduplication state (URL hashes) | Low | Server DB | Lifetime of alert |
| Engine health metrics (response times, error rates) | None | Server DB / Prometheus | Per metrics retention |
| Cached search results | None (no user attribution) | Server cache | 5 minutes default, configurable |
| Operator bearer token (`server.token`) | High | `server.yml` (restricted permissions, never logged) | Until operator rotates |
| Per-resource owner tokens | High | Server DB (hashed before storage, never stored plaintext) | Until owner revokes |

### Trust boundaries & external services

Full provider list under `### Data Sources`. Trust assumptions and failure modes:

| External service | Trusted for | Failure mode |
|------------------|-------------|--------------|
| Primary search engines (Google, Bing, DuckDuckGo, Brave, Qwant, Mojeek, Yandex, Baidu) | Web/image/video/news/maps results | Engine health monitor disables on consecutive failures; failover to remaining engines; results cached briefly to mask transient outages |
| Specialized engines (Wikipedia, YouTube, Reddit, StackOverflow, GitHub, HN, arXiv, PubMed, Wolfram Alpha, OpenStreetMap) | Domain-specific search/answers | Same as above |
| Instant-answer providers (OpenWeatherMap/wttr.in, exchangerate.host, Wiktionary, ip-location-db, etc.) | Read-only widget data | Skip widget on failure; never block main search |
| Outbound HTTP responses from any engine | Untrusted (responses parsed) | All HTML/JSON parsed defensively; user input never embedded in scraping requests; tracking parameters stripped |
| SMTP for alert delivery | Trusted to deliver | Retry with backoff; pause channel on repeated failures (per AI.md PART 17) |
| Webhook destinations (per-alert) | Untrusted (user-supplied URL) | Signed payload (HMAC); SSRF defenses on outbound URL (private CIDRs blocked, scheme allowlist); retry with backoff |
| Tor SOCKS5 proxy (optional, AI.md PART 31) | Privacy-preserving outbound | If unavailable, fall back to direct or surface error per admin config |

### Threat model & abuse cases

**Primary assets:** end-user search queries (must remain unlinkable to user identity), alert subscriptions (signed manage tokens), operator bearer token, server availability.

**Untrusted inputs:** end-user HTTP requests, alert email addresses, alert webhook URLs, search engine response bodies (HTML/JSON), instant-answer provider responses, ip-location-db downloads.

**Trusted (with controls):** operator bearer token, signed alert manage tokens, internal scheduler, internal cache.

**Attacker goals & defenses:**

| Goal | Vector | Defense / explicit non-goal |
|------|--------|-----------------------------|
| De-anonymize a query to a specific user | Query logs, IP logs | No server-side query/IP logging (per AI.md PART 11). Image proxy strips Referer. |
| Scrape the public service as free metasearch | High-volume bots | Rate limiting (per AI.md PART 1). Engine rotation. Optional CAPTCHA on abuse signals. |
| Spam alert subscriptions / abuse for email amplification | Mass alert creation, bogus addresses | Rate limit alert creation. Email verification required before activation. Per-email cap. |
| Phish via alert manage links | Signed-token forgery, link confusion | Alert manage tokens are cryptographically signed and unguessable; emails sent from the configured sender address. |
| SSRF via webhook destinations | User-supplied webhook URL | Validate URL scheme; block private/loopback ranges; signed payload so target can verify origin; outbound timeout. |
| Inject malicious content into result snippets | Hostile engine response | All result snippets HTML-escaped; CSP headers; result links never auto-clicked; tracking params stripped. |
| Steal operator bearer token | Disk read on the server host, MitM on plain-HTTP traffic | Operator token file restricted to service user ownership; token never logged; operator-rotatable; TLS required for any remote operator traffic. |
| Deny availability | DDoS, engine bans | Rate limits, engine rotation, cached fallback. Admin-tunable. |
| Exfiltrate alert subscriber list | Server DB read by attacker with host access | Audit log of operator actions; service user runs unprivileged; restrictive filesystem permissions on DB and server config. |
| Abuse instant-answer widgets | Crafted query → widget XSS | All widget output server-rendered with HTML escape; no inline script execution from external answer data. |
| Enumerate alerts publicly | Token guessing | Manage and RSS tokens are cryptographically random and unguessable; not enumerable. |

### Security decisions & exceptions

- **Anonymous-by-default public surface.** No user accounts on the public side; all preferences are client-side. Trade-off: cannot offer per-user history. Reason: privacy is the product.
- **Email-verified accountless alerts.** Alerts require email + verification but no account. Reason: keeps the privacy posture while still letting users monitor queries. Manage tokens are signed, single-use confirm + long-term manage links.
- **Direct querying of primary engines.** We query Google/Bing/etc. directly (no intermediary). Trade-off: engines may rate-limit or change their response format. Mitigation: per-engine health monitoring, parser fallback, engine rotation, brief result caching.
- **Image proxy enabled by default.** Result thumbnails are proxied through this server to strip Referer and prevent third-party tracking when users hover/load images. Trade-off: bandwidth cost on the server. Reason: privacy.
- **Tor integration is optional, off by default.** When the operator enables Tor integration (per AI.md PART 31), an `.onion` hidden service is published. Reason: optional anonymity for users on Tor.
- **Cached results may be stale.** Default cache is 5 minutes (configurable). Reason: reduce engine load and protect against transient failures. Trade-off: results can lag by up to TTL.
- **Webhook URL is user-supplied.** SSRF mitigations apply but cannot prevent a user from configuring webhooks pointing at private addresses they own. Reason: legitimate self-hosted automation. Mitigation: SSRF protections applied (per AI.md trust boundary rules).
- **No admin web UI or user accounts.** The spec (AI.md PART 0–33) has no admin web panel, no session system, no login form, no user registration, no organizations, and no custom-domain features. The operator surface is the two-tier bearer-token model (`server.token` + per-resource owner tokens) documented above. Reason: privacy is the product; the application has no UI-driven configuration and no end-user accounts.

---

### Features

#### Core Search

- **Multi-Engine Aggregation**: Query multiple engines simultaneously, merge and deduplicate results
- **Smart Ranking**: Weight and rank results based on engine reliability, result position, and frequency across engines
- **Multi-Category Search**: Web, images, videos, news, maps, files, music, science, IT, social media
- **Search Operators**: AND, OR, NOT, quotes, site:, filetype:, intitle:, inurl:, daterange:
- **Advanced Search Form**: GUI for building complex queries without knowing operators
- **Infinite Scroll / Pagination**: User choice between continuous loading or page-based navigation
- **Related Searches**: Suggestions for similar or refined queries

#### Reliability ("Always Works")

- **Engine Health Monitoring**: Track response times, error rates, and availability per engine
- **Automatic Failover**: If primary engines fail, seamlessly switch to backups
- **Engine Rotation**: Distribute requests to avoid rate limiting and detection
- **Multiple Query Strategies**: Multiple query approaches per engine with automatic fallback
- **Cached Results Fallback**: Serve stale results if all engines are temporarily down
- **Self-Healing**: Automatically re-enable recovered engines

#### Privacy & Security

- **Zero Tracking**: No server-side logging of queries, IPs, or user behavior
- **No Cookies Required**: Fully functional without cookies
- **No JavaScript Required**: Core search works with JS disabled (progressive enhancement)
- **Tor Integration**: SOCKS5 proxy support, automatic circuit rotation, .onion hidden service
- **Proxy Chain Support**: Route requests through custom proxy chains
- **Request Sanitization**: Strip tracking parameters from outgoing requests
- **Referrer Hiding**: Never leak search queries to result sites

#### User Preferences

Preferences persist across sessions without accounts. Three storage methods:

##### 1. localStorage (Default)
- Automatic, seamless browser storage
- Survives browser restarts

##### 2. Import/Export (Backup)
- Download preferences as JSON file
- Upload to restore on any device/browser
- Useful for backup and migration

##### 3. Preference String (Portable)
Compact URL-safe encoded string for sharing/bookmarking.

**Use Cases:**
- **URL Parameter**: `https://search.example.com/?prefs=t%3Dd%3Bc%3Dweb%3Bs%3Dm`
- **Bookmarklet**: Search with preferences baked in
- **Share Config**: Give others your default search behavior without accounts
- **QR Code**: Scan to apply preferences on mobile

**Workflow:**
1. Settings page → "Generate Link" button
2. Creates URL with `?prefs=ENCODED_STRING`
3. On page load, if `prefs` param exists → apply settings
4. Optional: save to localStorage for persistence

#### Search Alerts

Google Alerts-style monitoring for saved queries without requiring user accounts.

- **Accountless subscriptions**: Create alerts with email verification and a signed manage link
- **Delivery Channels**: Email notifications, private RSS feed, and webhook delivery
- **Flexible Schedules**: Immediate, daily digest, or weekly digest
- **New Results Only**: Only send newly discovered results since the last successful check
- **Filter-Aware**: Alert preserves query, category, safe search, language, region, and selected engines
- **Pause/Resume**: Temporarily disable alerts without deleting them
- **Per-Alert Manage Page**: Update query, frequency, destinations, or delete without logging in
- **Privacy-Respecting Storage**: Store only the minimum server-side data needed to deliver opted-in alerts

**Create Alert Workflow:**
1. User performs a search
2. Clicks **Create Alert** from the results page
3. Chooses delivery methods: email, RSS, webhook (one or more)
4. Selects frequency: immediate, daily, weekly
5. Submits email address and optional webhook URL
6. Server sends verification email with one-time confirmation link
7. After confirmation, server begins scheduled monitoring
8. User manages the alert later via signed manage link or private RSS URL

**Alert Delivery Rules:**
- **Email**: Summary of new results with direct links and unsubscribe/manage links
- **RSS**: Private, tokenized feed containing only newly matched results for that alert
- **Webhook**: Signed JSON payload with alert metadata and new results
- **Deduplication**: Track seen result URLs/hashes so the same result is not re-sent repeatedly
- **Failure Handling**: Temporary delivery failures retry with backoff; repeated failures pause the affected channel and notify the user when possible

**Use Cases:**
- Track new results for a brand, person, topic, or domain
- Monitor news or blog mentions for a query
- Watch for new PDF/files matching a research topic
- Trigger automations through webhook when new results appear

#### Instant Answers (Widgets)

Zero-click answers displayed above search results. Each widget has trigger patterns and displays contextual information.

---

##### Calculator
**Triggers**: Mathematical expressions
```
2 + 2, 15% of 200, sqrt(144), sin(45), 2^10, (5+3)*2
```
**Displays**: Result with expression, copy button
**Features**: Basic arithmetic, percentages, powers, roots, trigonometry, logarithms, constants (pi, e)

---

##### Unit Converter
**Triggers**: Number + unit, "convert X to Y"
```
5 miles in km, 100 fahrenheit to celsius, 2 cups in ml
50kg to lbs, 1000 bytes to kb, 90 degrees in radians
```
**Displays**: Converted value with both units, common conversions
**Categories**:
- Length: mm, cm, m, km, in, ft, yd, mi
- Weight: mg, g, kg, oz, lb, st
- Volume: ml, l, tsp, tbsp, cup, pt, qt, gal
- Temperature: C, F, K
- Area: sq ft, sq m, acre, hectare
- Speed: mph, km/h, m/s, knots
- Data: B, KB, MB, GB, TB
- Time: sec, min, hr, day, week, year

---

##### Currency Converter
**Triggers**: Amount + currency, "X USD to EUR"
```
100 usd to eur, $50 in pounds, 1000 jpy to usd
convert 500 euros to dollars
```
**Displays**: Converted amount, exchange rate, last updated time
**Features**: 150+ currencies, real-time rates, historical chart (7 days)

---

##### Weather
**Triggers**: "weather", "weather in [location]", "[location] weather"
```
weather, weather in tokyo, new york weather
forecast london, temperature paris
```
**Displays**:
- Current: temp, feels like, conditions, humidity, wind
- Forecast: 5-day outlook with highs/lows
- Icon: sun, cloud, rain, snow, etc.

---

##### Dictionary
**Triggers**: "define [word]", "meaning of [word]", "[word] definition"
```
define ubiquitous, meaning of ephemeral, serendipity definition
what does "catharsis" mean
```
**Displays**:
- Word, pronunciation (IPA), audio button
- Part of speech
- Definitions (numbered)
- Example sentences
- Etymology (origin)

---

##### Thesaurus
**Triggers**: "synonyms for [word]", "[word] synonyms", "antonyms of [word]"
```
synonyms for happy, beautiful synonyms, antonyms of good
words like "important"
```
**Displays**: Grouped synonyms/antonyms by meaning, word type labels

---

##### IP Lookup
**Triggers**: IP address, "my ip", "what is my ip", "ip [address]"
```
my ip, what is my ip, 8.8.8.8, ip 1.1.1.1
ip address, my ip address
```
**Displays**:
- IP address
- Location (city, region, country)
- ISP / Organization
- Timezone
- Map preview (optional)

---

##### Color Picker
**Triggers**: Hex code, RGB, HSL, color name
```
#ff5733, rgb(255,87,51), hsl(11,100%,60%)
red, dark blue, coral
```
**Displays**:
- Color swatch (large preview)
- All formats: HEX, RGB, HSL, CMYK
- Color name (if applicable)
- Complementary colors
- Copy buttons for each format
**Features**: Click swatch to open full color picker tool

---

##### Timezone Converter
**Triggers**: "time in [location]", "[time] [zone] to [zone]", "current time [city]"
```
time in tokyo, current time london
3pm EST to PST, 14:00 UTC to CET
what time is it in sydney
```
**Displays**:
- Current time in location
- UTC offset
- Conversion result with both times
- World clock (multiple cities)

---

##### Calendar / Date Calculator
**Triggers**: "days until [date]", "days between [date] and [date]", "[date] + [days]"
```
days until christmas, days until 2025-12-31
days between jan 1 and mar 15
today + 90 days, 2025-06-15 - 30 days
what day is july 4 2025
```
**Displays**: Result with day of week, calendar preview
**Features**: Date arithmetic, weekday lookup, countdown

---

##### Hash Generator
**Triggers**: "md5 [text]", "sha256 [text]", "hash [text]"
```
md5 hello world, sha256 password123
sha1 test, sha512 secret
hash my string
```
**Displays**: Hash output with algorithm label, copy button
**Algorithms**: MD5, SHA1, SHA256, SHA384, SHA512, CRC32

---

##### Base64 Encode/Decode
**Triggers**: "base64 encode [text]", "base64 decode [encoded]"
```
base64 encode hello world
base64 decode aGVsbG8gd29ybGQ=
b64 encode test, b64 decode dGVzdA==
```
**Displays**: Result with copy button, input/output labels

---

##### URL Encode/Decode
**Triggers**: "url encode [text]", "url decode [encoded]", "urlencode [text]"
```
url encode hello world!
url decode hello%20world%21
urlencode special chars: &?=
```
**Displays**: Encoded/decoded result with copy button

---

##### UUID Generator
**Triggers**: "uuid", "generate uuid", "new uuid", "guid"
```
uuid, generate uuid, new uuid, guid
random uuid, uuid v4
```
**Displays**:
- Generated UUID (v4 random)
- Copy button
- Generate another button
**Format**: `xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx`

---

##### Password Generator
**Triggers**: "password", "generate password", "random password"
```
password, generate password, random password
strong password, secure password
password 16 characters
```
**Displays**:
- Generated password
- Strength indicator
- Copy button
- Regenerate button
**Options**: Length (8-64), include uppercase, lowercase, numbers, symbols

---

##### QR Code Generator
**Triggers**: "qr [text/url]", "qr code [text]", "generate qr [url]"
```
qr https://example.com
qr code hello world
generate qr my wifi password
```
**Displays**:
- QR code image
- Download button (PNG/SVG)
- Size options
**Features**: URLs auto-linked, WiFi config format supported

---

##### Stopwatch / Timer
**Triggers**: "stopwatch", "timer [duration]", "countdown [duration]"
```
stopwatch, timer 5 minutes, countdown 30 seconds
timer 1h30m, pomodoro timer
```
**Displays**:
- Interactive stopwatch/timer
- Start/stop/reset buttons
- Lap times (stopwatch)
- Audio notification when done (timer)
**Note**: Runs in browser, persists during session

---

##### Random Number
**Triggers**: "random number", "roll dice", "flip coin", "random [min]-[max]"
```
random number, random 1-100, random 1-6
roll dice, roll d20, roll 2d6
flip coin, coin flip
pick random 1-10
```
**Displays**: Result with animation, roll again button
**Features**: Dice notation (d4, d6, d8, d10, d12, d20, d100)

---

##### Lorem Ipsum
**Triggers**: "lorem ipsum", "placeholder text", "dummy text"
```
lorem ipsum, lorem ipsum 3 paragraphs
placeholder text 100 words
dummy text 5 sentences
```
**Displays**: Generated text with copy button
**Options**: Paragraphs, sentences, or word count

---

##### Cryptocurrency Prices
**Triggers**: "[crypto] price", "bitcoin", "ethereum price", "[amount] btc to usd"
```
bitcoin price, btc, ethereum, eth price
1 btc to usd, 0.5 eth in dollars
dogecoin, doge price
```
**Displays**:
- Current price (USD)
- 24h change (% and $)
- Market cap
- Mini price chart (24h)

---

##### Stock Prices
**Triggers**: "stock [symbol]", "[symbol] stock", "[symbol] price"
```
stock AAPL, TSLA stock, MSFT price
$GOOGL, $AMZN stock price
```
**Displays**:
- Current price
- Change ($ and %)
- Open, high, low, volume
- Mini chart
**Note**: May be delayed 15-20 minutes (free tier limitation)

---

##### Package Tracking
**Triggers**: "track [number]", "[carrier] [number]", tracking number patterns
```
track 1Z999AA10123456784
usps 9400111899223033005955
fedex 123456789012
```
**Displays**:
- Carrier detected
- Current status
- Location history
- Estimated delivery
**Carriers**: USPS, UPS, FedEx, DHL (auto-detected from format)

---

##### Translate
**Triggers**: "translate [text] to [language]", "[text] in [language]"
```
translate hello to spanish
bonjour in english
"good morning" to german
translate こんにちは to english
```
**Displays**:
- Translated text
- Source language (auto-detected)
- Pronunciation (if applicable)
- Copy button

---

##### Wikipedia Summary
**Triggers**: "wiki [topic]", "[topic] wikipedia", "who is [person]", "what is [thing]"
```
wiki python programming
albert einstein wikipedia
who is marie curie
what is quantum computing
```
**Displays**:
- Title and thumbnail
- First paragraph summary
- "Read more" link to Wikipedia

---

##### Sports Scores
**Triggers**: "[team] score", "[team] game", "[league] scores"
```
lakers score, yankees game
nfl scores, premier league results
world cup scores
```
**Displays**:
- Live/final score
- Teams and logos
- Game time/status
- Upcoming game (if no live)

---

##### Nutrition Facts
**Triggers**: "calories in [food]", "[food] nutrition", "how many calories [food]"
```
calories in banana, apple nutrition
how many calories in pizza
nutrition facts chicken breast
```
**Displays**:
- Calories per serving
- Macros (protein, carbs, fat)
- Common serving sizes

#### Direct Answers (Full Page Results)

Unlike Instant Answers (widgets above search results), Direct Answers ARE the result. When a direct answer operator is detected, the response is a full-page dedicated view - no search results list.

**Syntax:** `{type}:{term}` or `{type}: {term}` (space after colon allowed)

---

##### tldr:{command}
**Purpose**: Quick command reference from tldr-pages
**Triggers**: `tldr:git`, `tldr:tar`, `tldr: docker`
```
tldr:curl
tldr:ffmpeg
tldr: kubectl
```
**Displays**:
- Command name and description
- Common usage examples with explanations
- Platform indicator (linux/osx/windows/common)
- Copy button for each example
- Link to full man page
**Fallback**: If command not found, offer to search or show man page

---

##### man:{page}
**Purpose**: Unix/Linux manual pages
**Triggers**: `man:ls`, `man:grep`, `man: awk`
```
man:bash
man:ssh
man: systemctl
```
**Displays**:
- Man page content with sections (NAME, SYNOPSIS, DESCRIPTION, OPTIONS, etc.)
- Syntax highlighting for code blocks
- Collapsible sections for long pages
- Table of contents sidebar
- Section navigation (1-8)
- Copy button for command examples
**Features**:
- Section selector: `man:printf.3` (C library) vs `man:printf.1` (shell command)
- Search within page
- Related pages (SEE ALSO section)

---

##### cache:{url}
**Purpose**: View cached/archived version of a webpage
**Triggers**: `cache:example.com`, `cache:https://site.com/page`
```
cache:reddit.com
cache:https://news.ycombinator.com/item?id=12345
```
**Displays**:
- Archived page content (rendered)
- Archive date/timestamp
- Multiple archive sources if available
- Original URL link
- "Save new snapshot" button
**Sources** (checked in order):
1. Google Cache (if available)
2. Wayback Machine (archive.org)
3. Archive.today
4. Common Crawl
**Features**:
- Timeline slider to view different archive dates
- Side-by-side comparison with live site
- Download archived version

---

##### whois:{domain}
**Purpose**: Domain registration and ownership information
**Triggers**: `whois:google.com`, `whois: example.org`
```
whois:github.com
whois:cloudflare.com
```
**Displays**:
- Registrar information
- Registration/expiration dates
- Name servers
- Registrant info (if public)
- Domain status codes
- DNSSEC status

---

##### dns:{domain}
**Purpose**: DNS record lookup
**Triggers**: `dns:example.com`, `dns: google.com`
```
dns:cloudflare.com
dns:github.io
```
**Displays**:
- A/AAAA records (IP addresses)
- MX records (mail servers)
- NS records (name servers)
- TXT records (SPF, DKIM, etc.)
- CNAME records
- SOA record
- TTL values
**Features**: Query specific record types: `dns:example.com/mx`, `dns:example.com/txt`

---

##### wiki:{topic}
**Purpose**: Wikipedia article summary and content
**Triggers**: `wiki:quantum computing`, `wiki: linux`
```
wiki:rust programming language
wiki:solar system
```
**Displays**:
- Article title and main image
- Introduction/summary paragraphs
- Table of contents
- Infobox data (if applicable)
- Key sections expandable
- Related articles
- "Read full article" link
**Features**: Language selection, mobile-friendly rendering

---

##### dict:{word}
**Purpose**: Full dictionary entry (alias for define:, but full-page)
**Triggers**: `dict:serendipity`, `dict: ephemeral`
```
dict:ubiquitous
dict:catharsis
```
**Displays**:
- Word, pronunciation (IPA), audio playback
- All parts of speech with definitions
- Example sentences for each meaning
- Etymology (word origin)
- Related words (synonyms, antonyms)
- Word frequency/usage level
- Translations (if multilingual enabled)

---

##### thesaurus:{word}
**Purpose**: Comprehensive synonym/antonym lookup
**Triggers**: `thesaurus:happy`, `thesaurus: important`
```
thesaurus:beautiful
thesaurus:fast
```
**Displays**:
- Synonyms grouped by meaning/sense
- Antonyms grouped by meaning/sense
- Related words and phrases
- Usage examples
- Formal/informal indicators
- Word intensity scale (e.g., happy → ecstatic → elated)

---

##### pkg:{name}
**Purpose**: Package/library information across registries
**Triggers**: `pkg:lodash`, `pkg:requests`, `pkg: chi`
```
pkg:express
pkg:numpy
pkg:go-chi/chi
```
**Displays**:
- Package name, description, version
- Installation command (npm, pip, go get, etc.)
- Weekly downloads / popularity
- License
- Dependencies count
- Last updated
- Repository link
- README preview
**Sources**: npm, PyPI, pkg.go.dev, crates.io, RubyGems
**Features**:
- Auto-detect registry from package name format
- Explicit registry: `pkg:npm/lodash`, `pkg:pip/requests`, `pkg:go/chi`

---

##### cve:{id}
**Purpose**: Security vulnerability details
**Triggers**: `cve:2021-44228`, `cve: CVE-2023-1234`
```
cve:2021-44228
cve:2014-0160
```
**Displays**:
- CVE ID and description
- CVSS score and severity
- Affected products/versions
- Remediation/patches
- References and advisories
- Exploit availability indicator
- Timeline (published, modified dates)

---

##### rfc:{number}
**Purpose**: IETF RFC document viewer
**Triggers**: `rfc:2616`, `rfc: 7231`
```
rfc:2616
rfc:793
rfc:7540
```
**Displays**:
- RFC title and status
- Abstract
- Table of contents
- Full document with section navigation
- Related RFCs (obsoletes, updated by)
- Authors and date
**Features**: Section deep-linking, search within document

---

##### ascii:{text}
**Purpose**: ASCII art text generator
**Triggers**: `ascii:hello`, `ascii: SEARCH`
```
ascii:Hello World
ascii:SEARCH
```
**Displays**:
- Large ASCII art rendering of text
- Multiple font style options (banner, big, block, bubble, etc.)
- Copy button
- Download as text file

---

##### qr:{text}
**Purpose**: QR code generator (full page with options)
**Triggers**: `qr:https://example.com`, `qr: wifi:...`
```
qr:https://github.com
qr:WIFI:T:WPA;S:MyNetwork;P:MyPassword;;
```
**Displays**:
- Large QR code image
- Size options (S/M/L/XL)
- Download buttons (PNG/SVG)
- Error correction level selector
- Customization (colors, logo embed)
**Features**: WiFi QR format, vCard format, URL shortening option

---

##### resolve:{hostname}
**Purpose**: Hostname to IP resolution with details
**Triggers**: `resolve:google.com`, `resolve: cloudflare.com`
```
resolve:github.com
resolve:api.example.com
```
**Displays**:
- IPv4 addresses (A records)
- IPv6 addresses (AAAA records)
- Reverse DNS (PTR)
- GeoIP location for each IP
- ASN information
- Response time from multiple DNS servers

---

##### cert:{domain}
**Purpose**: SSL/TLS certificate information
**Triggers**: `cert:google.com`, `cert: github.com`
```
cert:cloudflare.com
cert:example.com
```
**Displays**:
- Certificate chain (root → intermediate → leaf)
- Validity dates (issued, expires)
- Subject and issuer details
- SANs (Subject Alternative Names)
- Key algorithm and size
- Certificate transparency logs
- Grade/security rating

---

##### headers:{url}
**Purpose**: HTTP response headers inspection
**Triggers**: `headers:example.com`, `headers: https://api.github.com`
```
headers:google.com
headers:https://cloudflare.com
```
**Displays**:
- All response headers
- Security headers analysis (CSP, HSTS, X-Frame-Options, etc.)
- Server identification
- Caching headers
- Cookie attributes
- Security grade
**Features**: Follow redirects option, custom request headers

---

##### directory:{term}
**Purpose**: Search open directory indexes (servers with directory listing enabled)
**Triggers**: `directory:music mp3`, `directory:linux iso`, `directory: ebooks pdf`
```
directory:beatles mp3
directory:ubuntu iso
directory:programming pdf
directory:movies mkv
```
**How it works**:
Automatically constructs complex search query:
```
intitle:"index of" OR intitle:"directory of" {term}
"parent directory" OR "last modified"
-html -htm -php -asp -aspx -jsp
{file_extension if detected}
```
**Displays**:
- List of discovered open directories
- URL with clickable path
- File listing preview (scraped if accessible):
  - Filename
  - File size
  - Last modified date
  - Direct download link
- Directory metadata (server type, estimated file count)
- Filter controls:
  - File type (audio/video/documents/archives/images)
  - Minimum file size
  - Date range
**Sources**: Google, Bing, DuckDuckGo (aggregated, deduplicated)
**Features**:
- Smart file extension detection: `directory:music` assumes audio files
- Site restriction: `directory:site:edu textbook pdf`
- Exclude terms: `directory:linux iso -ubuntu -mint`
- Size hints: `directory:movie 1080p` (implies large files)
- Direct scraping of accessible directories for file listings
- "Scan directory" button to enumerate files from a result

**Common use cases**:
| Query | Finds |
|-------|-------|
| `directory:mp3 album` | Music directories |
| `directory:pdf book` | Ebook/document directories |
| `directory:iso linux` | Linux ISO mirrors |
| `directory:mkv movie` | Video file directories |
| `directory:apk android` | Android APK repositories |
| `directory:rom nintendo` | ROM/emulator file directories |

**Privacy note**: Results link to public servers with open directory listing.
Files are not hosted by search - only indexed.

---

##### cheat:{command}
**Purpose**: Detailed command cheatsheets from cheat.sh
**Triggers**: `cheat:tar`, `cheat:curl`, `cheat: git`
```
cheat:rsync
cheat:find
cheat:awk
```
**Displays**:
- Command with detailed examples (more than tldr)
- Multiple use cases with explanations
- Related commands
- Community-contributed examples
- Copy button for each example
**Difference from tldr**: More verbose, community examples, covers edge cases

---

##### http:{code}
**Purpose**: HTTP status code explanation
**Triggers**: `http:404`, `http:503`, `http: 418`
```
http:200
http:301
http:429
http:502
```
**Displays**:
- Status code and official name
- Category (1xx/2xx/3xx/4xx/5xx)
- Detailed explanation
- Common causes
- How to fix (for errors)
- Related status codes
- RFC reference

---

##### port:{number}
**Purpose**: Port number to service mapping
**Triggers**: `port:22`, `port:443`, `port: 8080`
```
port:80
port:3306
port:5432
port:6379
```
**Displays**:
- Port number and protocol (TCP/UDP)
- Service name
- Description
- Common software using this port
- Security notes (if applicable)
- IANA registration status
- Alternative ports for the service

---

##### cron:{expression}
**Purpose**: Cron expression explainer and validator
**Triggers**: `cron:*/5 * * * *`, `cron:0 2 * * 0`
```
cron:0 0 * * *
cron:*/15 * * * *
cron:0 9-17 * * 1-5
```
**Displays**:
- Human-readable explanation
- Next 5 scheduled run times
- Visual breakdown of each field
- Field editor (interactive)
- Common presets (hourly, daily, weekly, monthly)
- Timezone selector
**Features**: Validates expression, warns about common mistakes

---

##### chmod:{permissions}
**Purpose**: Unix permission calculator
**Triggers**: `chmod:755`, `chmod:rwxr-xr-x`, `chmod: 644`
```
chmod:777
chmod:600
chmod:rwx------
chmod:u+x,g+r
```
**Displays**:
- Numeric notation (octal)
- Symbolic notation (rwx)
- Visual permission grid (owner/group/other × read/write/execute)
- Interactive toggle to modify
- Explanation of what each permission means
- Common use cases for this permission set
- Security warnings (e.g., 777 is dangerous)

---

##### regex:{pattern}
**Purpose**: Regular expression tester and explainer
**Triggers**: `regex:^[a-z]+$`, `regex:\d{3}-\d{4}`
```
regex:^[\w.-]+@[\w.-]+\.\w+$
regex:\b\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}\b
regex:(?<=@)\w+
```
**Displays**:
- Pattern breakdown (visual tree)
- Explanation of each component
- Test input field with live highlighting
- Match results with groups
- Common regex library differences (PCRE, RE2, JS)
- Optimization suggestions
**Features**: Supports test strings, shows capture groups

---

##### jwt:{token}
**Purpose**: JWT (JSON Web Token) decoder
**Triggers**: `jwt:eyJhbG...`
```
jwt:eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U
```
**Displays**:
- Header (algorithm, type) - decoded JSON
- Payload (claims) - decoded JSON with timestamps converted
- Signature (verification status if secret provided)
- Expiration status (valid/expired)
- Standard claims explained (iat, exp, sub, iss, aud)
- Copy buttons for each section
**Security**: Never logs tokens, decoding is client-side only

---

##### timestamp:{value}
**Purpose**: Unix timestamp converter
**Triggers**: `timestamp:1704067200`, `timestamp:2024-01-01`
```
timestamp:now
timestamp:1700000000
timestamp:2025-06-15T14:30:00Z
```
**Displays**:
- Unix timestamp (seconds)
- Unix timestamp (milliseconds)
- ISO 8601 format
- RFC 2822 format
- Human readable (multiple formats)
- Relative time (X days ago / in X days)
- Multiple timezone conversions
**Features**: Bidirectional (timestamp ↔ date), timezone selector

---

##### asn:{number}
**Purpose**: Autonomous System Number lookup
**Triggers**: `asn:15169`, `asn:AS13335`, `asn: 32934`
```
asn:15169
asn:13335
asn:32934
```
**Displays**:
- ASN number and name
- Organization/company
- Country of registration
- IP prefixes announced (IPv4 and IPv6)
- Number of IPs
- Upstream/downstream peers
- Looking glass links
- Historical changes

---

##### subnet:{cidr}
**Purpose**: Subnet/CIDR calculator
**Triggers**: `subnet:192.168.1.0/24`, `subnet:10.0.0.0/8`
```
subnet:192.168.1.0/24
subnet:10.0.0.0/16
subnet:172.16.0.0/12
```
**Displays**:
- Network address
- Broadcast address
- First/last usable host
- Total hosts
- Subnet mask (decimal and binary)
- Wildcard mask
- CIDR notation
- Binary representation
- Subnetting table (split into smaller subnets)
- Supernetting (combine into larger)
**Features**: IPv4 and IPv6 support

---

##### robots:{domain}
**Purpose**: Fetch and display robots.txt
**Triggers**: `robots:google.com`, `robots:github.com`
```
robots:example.com
robots:reddit.com
```
**Displays**:
- Full robots.txt content (syntax highlighted)
- Parsed rules by user-agent
- Disallowed paths
- Allowed paths
- Sitemap references
- Crawl-delay directives
- Analysis: what's blocked, what's allowed
- Warnings about common issues

---

##### sitemap:{domain}
**Purpose**: Fetch and parse sitemap.xml
**Triggers**: `sitemap:example.com`, `sitemap:blog.example.com`
```
sitemap:github.com
sitemap:news.ycombinator.com
```
**Displays**:
- Sitemap index (if multiple sitemaps)
- URL list with last modified dates
- URL count and structure analysis
- Frequency/priority if specified
- Tree view of site structure
- Export options (CSV, JSON)
- Filter by path pattern

---

##### tech:{domain}
**Purpose**: Technology stack detection
**Triggers**: `tech:github.com`, `tech:stripe.com`
```
tech:netflix.com
tech:shopify.com
tech:cloudflare.com
```
**Displays**:
- Web server (nginx, Apache, etc.)
- Programming language/framework
- CMS/Platform (WordPress, Shopify, etc.)
- JavaScript libraries/frameworks
- Analytics tools
- CDN provider
- SSL certificate issuer
- Hosting provider
- Security headers present
**Features**: Confidence scores, detection method shown

---

##### feed:{domain}
**Purpose**: Discover RSS/Atom feeds on a website
**Triggers**: `feed:example.com`, `feed:blog.example.com`
```
feed:bbc.com
feed:techcrunch.com
feed:reddit.com/r/programming
```
**Displays**:
- List of discovered feeds (RSS, Atom, JSON Feed)
- Feed title and description
- Last update time
- Entry count
- Subscribe links (various readers)
- Feed preview (latest 5 items)
- Auto-discovery method used

---

##### expand:{url}
**Purpose**: Expand shortened URLs and reveal destination
**Triggers**: `expand:bit.ly/abc123`, `expand:t.co/xyz`
```
expand:bit.ly/3xKpQr5
expand:tinyurl.com/example
expand:t.co/abc123
```
**Displays**:
- Original short URL
- Final destination URL
- Redirect chain (all hops)
- Each redirect's status code
- Final page title and description
- Screenshot preview (optional)
- Safety warning if suspicious
**Supported shorteners**: bit.ly, t.co, tinyurl, goo.gl, ow.ly, is.gd, and any HTTP redirect

---

##### safe:{url}
**Purpose**: URL safety and reputation check
**Triggers**: `safe:suspicious-site.com`, `safe:example.com`
```
safe:google.com
safe:some-unknown-site.xyz
```
**Displays**:
- Overall safety rating (Safe/Suspicious/Malicious)
- Google Safe Browsing status
- Domain age
- SSL certificate validity
- WHOIS privacy (hidden owner = flag)
- Known blacklist presence
- Redirect chain analysis
- Phishing indicators
- Related malicious domains
**Privacy**: Only domain is checked, not full URLs with parameters

---

##### html:{text}
**Purpose**: HTML entity encode/decode
**Triggers**: `html:encode <script>`, `html:decode &lt;script&gt;`
```
html:encode <div class="test">Hello & Goodbye</div>
html:decode &lt;p&gt;Test &amp; Demo&lt;/p&gt;
html:&copy; &rarr; &mdash;
```
**Displays**:
- Encoded/decoded result
- Character-by-character breakdown
- Named entities vs numeric entities
- Copy button
- Bidirectional converter (input either format)
**Features**: Named entities (&amp;), decimal (&#38;), hex (&#x26;)

---

##### unicode:{char}
**Purpose**: Unicode character information
**Triggers**: `unicode:U+1F600`, `unicode:😀`, `unicode:A`
```
unicode:U+1F600
unicode:😀
unicode:\u0041
unicode:SNOWMAN
```
**Displays**:
- Character rendered large
- Code point (U+XXXX)
- UTF-8 bytes (hex)
- UTF-16 encoding
- HTML entity
- Name and category
- Block/script
- Related characters
- Copy in various formats

---

##### emoji:{name}
**Purpose**: Emoji search and information
**Triggers**: `emoji:smile`, `emoji:heart`, `emoji:flag`
```
emoji:thumbs up
emoji:fire
emoji:country flag
emoji:cat
```
**Displays**:
- Matching emojis with names
- Unicode code points
- Shortcodes (:smile:, :+1:)
- Copy buttons
- Categories (smileys, animals, flags, etc.)
- Skin tone variants (if applicable)
- Platform rendering differences
- Recently added emojis labeled

---

##### escape:{text}
**Purpose**: String escaper for various languages/formats
**Triggers**: `escape:json "hello\nworld"`, `escape:sql O'Brien`
```
escape:json {"key": "value with \"quotes\""}
escape:sql It's a test
escape:html <script>alert('xss')</script>
escape:url hello world?foo=bar
escape:regex file.txt
escape:shell $HOME/*.txt
```
**Displays**:
- Escaped output for multiple languages simultaneously:
  - JSON
  - SQL (MySQL, PostgreSQL, SQLite)
  - HTML
  - URL (percent encoding)
  - Regex
  - Shell (bash)
  - JavaScript
  - Python
  - C/C++
- Copy button for each
- Unescape mode available

---

##### json:{data}
**Purpose**: JSON formatter, validator, and tools
**Triggers**: `json:{"key":"value"}`, `json:validate {...}`
```
json:{"name":"test","values":[1,2,3]}
json:minify { "spaced" : "json" }
json:validate {"test":}
```
**Displays**:
- Formatted/pretty-printed JSON (syntax highlighted)
- Collapsible tree view
- Validation errors with line numbers
- Minified version
- JSON path navigator
- Type annotations
- Size (bytes, keys, depth)
**Modes**:
- `json:` or `json:format` - Pretty print
- `json:minify` - Compress
- `json:validate` - Check validity
- `json:tree` - Tree view

---

##### yaml:{data}
**Purpose**: YAML formatter, validator, and converter
**Triggers**: `yaml:key: value`, `yaml:to-json {...}`
```
yaml:name: test
  items:
    - one
    - two
yaml:to-json key: value
yaml:from-json {"key": "value"}
```
**Displays**:
- Formatted YAML (syntax highlighted)
- JSON equivalent
- Validation errors
- Type detection
- Anchor/alias resolution
**Modes**:
- `yaml:` - Format/validate
- `yaml:to-json` - Convert to JSON
- `yaml:from-json` - Convert JSON to YAML

---

##### diff:{texts}
**Purpose**: Text/code comparison tool
**Triggers**: `diff:old|||new` (triple pipe separator)
```
diff:hello world|||hello there
diff:function old()|||function new()
```
**Displays**:
- Side-by-side comparison
- Unified diff format
- Inline highlighting of changes
- Line numbers
- Added/removed/changed statistics
- Word-level diff option
- Ignore whitespace option
- Copy diff output
**Features**: Syntax highlighting for code, multiple diff formats

---

##### beautify:{code}
**Purpose**: Code beautifier/formatter
**Triggers**: `beautify:js {...}`, `beautify:css {...}`
```
beautify:js function test(){return{a:1,b:2}}
beautify:css .class{color:red;margin:0}
beautify:html <div><p>test</p></div>
beautify:sql SELECT * FROM users WHERE id=1
```
**Displays**:
- Formatted code (syntax highlighted)
- Indentation options (2/4 spaces, tabs)
- Style options per language
- Minified version toggle
- Copy button
- Line count before/after
**Supported languages**:
- JavaScript/TypeScript
- CSS/SCSS/LESS
- HTML/XML
- SQL
- JSON (redirects to json:)
- PHP
- Python (via autopep8 rules)

---

##### case:{text}
**Purpose**: Text case converter
**Triggers**: `case:hello world`, `case:camel hello world`
```
case:Hello World
case:upper hello world
case:snake Hello World
case:camel hello-world
```
**Displays all conversions simultaneously**:
- UPPERCASE
- lowercase
- Title Case
- Sentence case
- camelCase
- PascalCase
- snake_case
- kebab-case
- SCREAMING_SNAKE_CASE
- dot.case
- Copy button for each

---

##### slug:{text}
**Purpose**: URL slug generator
**Triggers**: `slug:Hello World!`, `slug:Café & Résumé`
```
slug:Hello, World! How are you?
slug:Über die Brücke
slug:日本語テスト
```
**Displays**:
- Generated slug (hello-world-how-are-you)
- Character replacements shown
- Options:
  - Separator (- or _)
  - Lowercase only
  - Max length
  - Transliteration (ü→u, é→e)
  - CJK romanization
- Multiple slug styles
- Copy button

---

##### lorem:{count}
**Purpose**: Lorem ipsum placeholder text generator
**Triggers**: `lorem:3`, `lorem:5 paragraphs`, `lorem:100 words`
```
lorem:3
lorem:5 paragraphs
lorem:100 words
lorem:50 sentences
```
**Displays**:
- Generated lorem ipsum text
- Word/character/paragraph count
- Options:
  - Paragraphs (default)
  - Sentences
  - Words
  - Characters
  - Start with "Lorem ipsum..." toggle
- Alternative generators:
  - Hipster ipsum
  - Bacon ipsum
  - Tech ipsum
- Copy button

---

##### word:{text}
**Purpose**: Word, character, and text statistics
**Triggers**: `word:The quick brown fox`
```
word:The quick brown fox jumps over the lazy dog.
word:This is a longer piece of text to analyze...
```
**Displays**:
- Character count (with/without spaces)
- Word count
- Sentence count
- Paragraph count
- Line count
- Average word length
- Reading time estimate
- Speaking time estimate
- Most frequent words
- Unique words percentage
- Text input area for live counting

---

##### useragent:{string}
**Purpose**: Parse and explain user agent strings
**Triggers**: `useragent:Mozilla/5.0...`
```
useragent:Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36
useragent:my (detect current browser)
```
**Displays**:
- Browser name and version
- Rendering engine
- Operating system and version
- Device type (desktop/mobile/tablet)
- Architecture (x64, ARM)
- Bot detection
- Parsed components breakdown
- Raw string analysis
- Known UA database match
**Features**: `useragent:my` shows current browser's UA

---

##### mime:{type}
**Purpose**: MIME type information and lookup
**Triggers**: `mime:application/json`, `mime:pdf`, `mime:.mp4`
```
mime:application/json
mime:image/png
mime:pdf
mime:.docx
```
**Displays**:
- Full MIME type
- File extensions associated
- Type category (text, image, audio, video, application)
- Description
- Common software that uses it
- Binary vs text
- Compressible flag
- IANA registration status
- Related MIME types
**Features**: Lookup by MIME type, extension, or common name

---

##### license:{name}
**Purpose**: Software license information and text
**Triggers**: `license:MIT`, `license:GPL-3.0`, `license:Apache`
```
license:MIT
license:GPL-3.0
license:Apache-2.0
license:BSD-3-Clause
license:ISC
```
**Displays**:
- License name and SPDX identifier
- Full license text
- Summary (permissions, conditions, limitations)
- OSI approved status
- Copyleft/permissive classification
- Compatibility with other licenses
- Use cases (when to use this license)
- Copy button for full text
- Template with placeholders filled

---

##### country:{code}
**Purpose**: Country information lookup
**Triggers**: `country:US`, `country:Japan`, `country:DE`
```
country:US
country:United States
country:JP
country:276
```
**Displays**:
- Country name (official and common)
- Flag emoji and SVG
- ISO codes (alpha-2, alpha-3, numeric)
- Capital city
- Region and subregion
- Population
- Area (km²)
- Currency (code, name, symbol)
- Languages (official)
- Calling code
- TLD (top-level domain)
- Timezone(s)
- Driving side
- Map preview

---

##### slang:{term}
**Purpose**: Slang, internet lingo, and informal language definitions
**Triggers**: `slang:yeet`, `slang:67`, `slang: bussin`
```
slang:yeet
slang:no cap
slang:simp
slang:67
slang:rizz
```
**Displays**:
- Term and pronunciation (if applicable)
- Definition(s) ranked by popularity/votes
- Example usage in context
- Origin/etymology (when known)
- Related slang terms
- Age/generation indicator (Gen Z, Millennial, etc.)
- Region indicator (US, UK, internet-wide, etc.)
- Upvotes/popularity score
- Date added/last updated
- NSFW warning (if applicable)
**Features**:
- Multiple definitions shown (top 5 by votes)
- NSFW filter option (configurable)
- "Random slang" if no term provided
- Number codes (67, 420, etc.) supported

---

##### rules:{query}
**Purpose**: Rules of the Internet lookup (easter egg)
**Triggers**: `rules:`, `rules:34`, `rules: cat`, `rules:all`
```
rules:
rules:34
rules:1
rules:cat
rules:all
```
**Displays**:
- `rules:` or `rules:all` - All rules of the internet
- `rules:{number}` - Specific rule by number (e.g., rule 34)
- `rules:{term}` - Search rules containing the term
- Invalid number falls back to showing all rules
**Features**:
- Complete rules 1-100+ from internet folklore
- Search functionality for finding relevant rules
- Humorous/nostalgic easter egg for internet veterans

---

#### Direct Answer Behavior

**Query Processing Order:**
1. Check for direct answer operator (`type:term`)
2. If found → render full-page direct answer
3. If not found → continue to instant answers → search

**Access:**
- Direct answers are accessible as full-page views and can also be triggered from a regular search query using the `type:term` operator syntax.
- Route paths are defined in AI.md PART 14.

**Caching:**
- tldr pages: 7 days (updated weekly)
- man pages: 30 days
- DNS/whois: 1 hour
- cache/archive: no cache (always fetch fresh)
- wiki: 24 hours
- pkg: 6 hours
- cve: 24 hours

**Error Handling:**
- Not found: Show "No results for {type}:{term}" with suggestions
- Timeout: Show cached version if available, else error
- Rate limited: Queue request, show loading state

#### Bang Shortcuts

Quick redirects to specific sites/engines. 180+ bangs implemented across categories:

**General Search**: `!g` Google, `!b` Bing, `!ddg` DuckDuckGo, `!sp` Startpage, `!br` Brave, `!q` Qwant, `!ya` Yahoo, `!kagi` Kagi, `!you` You.com, `!perplexity` Perplexity

**Images**: `!gi` Google Images, `!bi` Bing Images, `!fl` Flickr, `!pexels` Pexels, `!pixabay` Pixabay, `!500px` 500px, `!pinterest` Pinterest, `!imgur` Imgur, `!tineye` TinEye

**Video**: `!yt` YouTube, `!v` Vimeo, `!twitch` Twitch, `!tiktok` TikTok, `!dtube` DTube, `!pt` PeerTube, `!rumble` Rumble

**Maps**: `!gm` Google Maps, `!osm` OpenStreetMap, `!waze` Waze, `!mapquest` MapQuest, `!yelp` Yelp, `!tripadvisor` TripAdvisor

**News**: `!gn` Google News, `!reuters` Reuters, `!bbc` BBC, `!techcrunch` TechCrunch, `!verge` The Verge, `!hn` Hacker News, `!r` Reddit

**Knowledge**: `!w` Wikipedia, `!wa` Wolfram Alpha, `!wd` Wikidata, `!britannica` Britannica, `!quora` Quora

**Social**: `!tw` Twitter/X, `!mast` Mastodon, `!fb` Facebook, `!linkedin` LinkedIn, `!bluesky` Bluesky, `!lb` Lobsters

**Code & Dev**: `!gh` GitHub, `!gl` GitLab, `!bb` Bitbucket, `!so` Stack Overflow, `!npm` NPM, `!pypi` PyPI, `!crates` Crates.io, `!gopkg` Go Packages, `!hex` Hex.pm, `!nuget` NuGet, `!mdn` MDN, `!devdocs` DevDocs, `!can` Can I Use, `!tldr` tldr pages, `!docker` Docker Hub, `!codepen` CodePen

**Shopping**: `!amz` Amazon, `!eb` eBay, `!aliexpress` AliExpress, `!walmart` Walmart, `!etsy` Etsy, `!camelcamelcamel` CamelCamelCamel

**Files & Books**: `!archive` Internet Archive, `!libgen` Library Genesis, `!annas` Anna's Archive, `!gutenberg` Project Gutenberg, `!goodreads` Goodreads, `!googlebooks` Google Books

**Music**: `!spot` Spotify, `!sc` SoundCloud, `!bc` Bandcamp, `!lastfm` Last.fm, `!discogs` Discogs

**Science**: `!scholar` Google Scholar, `!arxiv` arXiv, `!pubmed` PubMed, `!semanticscholar` Semantic Scholar, `!doi` DOI Resolver

**Translation**: `!gt` Google Translate, `!deepl` DeepL, `!dict` Dictionary.com, `!thesaurus` Thesaurus.com, `!mw` Merriam-Webster, `!ud` Urban Dictionary

**Privacy & Security**: `!wbm` Wayback Machine, `!virustotal` VirusTotal, `!shodan` Shodan, `!whois` WHOIS, `!urlscan` URLScan

**Movies & TV**: `!imdb` IMDb, `!rt` Rotten Tomatoes, `!letterboxd` Letterboxd, `!justwatch` JustWatch, `!tmdb` TMDB

**Games**: `!steam` Steam, `!gog` GOG, `!itch` itch.io, `!igdb` IGDB, `!howlongtobeat` HowLongToBeat

**Jobs**: `!indeed` Indeed, `!glassdoor` Glassdoor, `!linkedin` LinkedIn Jobs

- **Custom Bangs**: Define your own shortcuts in settings
- **Bang Autocomplete**: Suggestions as you type `!`
- **Bang Categories**: Organize bangs by type (search, shopping, dev, etc.)

#### Keyboard Shortcuts

Vim-inspired navigation for power users:

```
/         → Focus search box
Escape    → Clear/unfocus search box
j/k       → Navigate results down/up
Enter     → Open selected result
o         → Open in current tab
O         → Open in new tab
h/l       → Previous/next page
g g       → Go to first result
G         → Go to last result
t         → Toggle theme
s         → Open settings
?         → Show keyboard shortcuts help
1-9       → Jump to result N
```

#### Theming & Customization

- **Built-in Themes**: Dark (Dracula), Light, Auto (system preference)
- **Custom CSS**: User-provided stylesheet override
- **Font Size**: Small, medium, large
- **Results Density**: Compact, comfortable, spacious
- **Accent Color**: Customizable highlight color
- **Logo Customization**: Admin can set custom logo/branding

#### Filtering & Refinement

- **Time Filter**: Past hour, day, week, month, year, custom range
- **Region Filter**: Country/region-specific results
- **Language Filter**: Results in specific language
- **File Type Filter**: PDF, DOC, XLS, PPT, etc.
- **Site Filter**: Include/exclude specific domains
- **Domain Blocklist**: Never show results from blocked domains
- **Safe Search**: Off, moderate, strict

#### Search History (Local Only)

- Stored in browser localStorage only (never server)
- Quick access to recent searches
- Clear history button
- Disable history option
- Export/import with preferences

#### Browser Integration

- **OpenSearch**: Add as browser search engine
- **PWA Support**: Install as standalone app
- **Browser Extension**: Quick search from any page (future)
- **Search Bar Widget**: Embeddable search box for other sites

#### Video Previews

Interactive video thumbnails without leaving search results:

**Desktop (Hover)**:
- Hover over video thumbnail → animated preview plays (muted)
- Preview loads first 5-10 seconds of video as WebM/MP4 snippet
- Shows duration overlay, view count, channel name
- Click to open video page or embedded player

**Mobile (Touch)**:
- Tap thumbnail → plays inline preview (muted)
- Swipe left/right on thumbnail → scrub through video timeline
- Double-tap → open full video
- Shows progress bar during preview

**Implementation**:
- Thumbnails served via image proxy for privacy
- Preview clips fetched on-demand (not preloaded)
- Fallback to static thumbnail if preview unavailable
- Respects user preference to disable previews (bandwidth/privacy)

---

### UI/Layout Specification

#### Design Philosophy

Google-like simplicity with SearXNG-style preferences:
- Clean, minimal homepage with centered logo and search bar
- Results page with left-aligned results, no clutter
- Preferences accessible via gear icon, organized in tabs

#### Homepage Layout

```
┌─────────────────────────────────────────────────────────┐
│                                              [⚙️] [🌙]  │
│                                                         │
│                                                         │
│                      ┌─────────┐                        │
│                      │  LOGO   │                        │
│                      └─────────┘                        │
│                                                         │
│            ┌─────────────────────────┬───┐              │
│            │ Search...               │ 🔍│              │
│            └─────────────────────────┴───┘              │
│                                                         │
│            [Web] [Images] [Videos] [News] [More ▼]      │
│                                                         │
│                                                         │
└─────────────────────────────────────────────────────────┘
```

#### Search Results Layout

```
┌─────────────────────────────────────────────────────────┐
│ [Logo] ┌─────────────────────┬───┐  [Create Alert] [⚙️] [🌙] │
│        │ query               │ 🔍│                      │
│        └─────────────────────┴───┘                      │
│ [Web] [Images] [Videos] [News]    [Tools ▼] [Region ▼]  │
├─────────────────────────────────────────────────────────┤
│                                                         │
│ ┌─ Instant Answer (if applicable) ─────────────────────┐│
│ │ 2 + 2 = 4                                            ││
│ └──────────────────────────────────────────────────────┘│
│                                                         │
│ example.com                                             │
│ Result Title - Clickable Link                           │
│ Snippet text showing preview of the page content with   │
│ search terms highlighted...                             │
│                                                         │
│ another-site.org › path › page                          │
│ Another Result Title                                    │
│ More snippet text describing this result...             │
│                                                         │
│ [1] [2] [3] [4] [5] [Next →]                           │
│                                                         │
└─────────────────────────────────────────────────────────┘
```

#### Image Results Layout

```
┌─────────────────────────────────────────────────────────┐
│ [Logo] [Search bar]                        [⚙️] [🌙]   │
│ [Web] [Images•] [Videos] [News]                         │
├─────────────────────────────────────────────────────────┤
│ ┌───────┐ ┌───────┐ ┌───────┐ ┌───────┐ ┌───────┐      │
│ │       │ │       │ │       │ │       │ │       │      │
│ │  img  │ │  img  │ │  img  │ │  img  │ │  img  │      │
│ │       │ │       │ │       │ │       │ │       │      │
│ └───────┘ └───────┘ └───────┘ └───────┘ └───────┘      │
│ ┌───────┐ ┌───────┐ ┌───────┐ ┌───────┐ ┌───────┐      │
│ │       │ │       │ │       │ │       │ │       │      │
│ │  img  │ │  img  │ │  img  │ │  img  │ │  img  │      │
│ ...                                                     │
└─────────────────────────────────────────────────────────┘

Click image → Side panel with:
- Full size preview
- Source URL
- Image dimensions
- Download button
- "Visit page" button
```

#### Video Results Layout

```
┌─────────────────────────────────────────────────────────┐
│ [Logo] [Search bar]                        [⚙️] [🌙]   │
│ [Web] [Images] [Videos•] [News]                         │
├─────────────────────────────────────────────────────────┤
│                                                         │
│ ┌──────────┐  Video Title                               │
│ │ ▶ 10:34  │  channel-name · 1.2M views · 2 days ago   │
│ │ thumbnail │  Video description snippet text showing   │
│ │  [hover]  │  what this video is about...              │
│ └──────────┘                                            │
│                                                         │
│ ┌──────────┐  Another Video Title                       │
│ │ ▶ 5:22   │  another-channel · 500K views · 1 week    │
│ │ thumbnail │  Description of this video content...     │
│ └──────────┘                                            │
│                                                         │
└─────────────────────────────────────────────────────────┘

Hover on thumbnail → animated preview plays
Click → opens video in embedded player or source site
```

#### Preferences Page (SearXNG-style Tabs)

```
┌─────────────────────────────────────────────────────────┐
│ ← Back to Search                                        │
│                                                         │
│ Preferences                                             │
│ ─────────────────────────────────────────────────────── │
│ [General] [Interface] [Privacy] [Engines] [Data]        │
├─────────────────────────────────────────────────────────┤
│                                                         │
│ GENERAL TAB:                                            │
│ ┌─────────────────────────────────────────────────────┐ │
│ │ Default Category      [Web ▼]                       │ │
│ │ Language              [English ▼]                   │ │
│ │ Region                [United States ▼]             │ │
│ │ Safe Search           [Moderate ▼]                  │ │
│ │ Autocomplete          [✓] Enabled                   │ │
│ └─────────────────────────────────────────────────────┘ │
│                                                         │
│ INTERFACE TAB:                                          │
│ ┌─────────────────────────────────────────────────────┐ │
│ │ Theme                 [Auto ▼] (Dark/Light/Auto)    │ │
│ │ Results per page      [20 ▼]                        │ │
│ │ Open in new tab       [✓] Enabled                   │ │
│ │ Infinite scroll       [ ] Disabled                  │ │
│ │ Keyboard shortcuts    [✓] Enabled                   │ │
│ │ Show engine badges    [✓] Show source engine        │ │
│ │ Video previews        [✓] Enabled                   │ │
│ │ Font size             [Medium ▼]                    │ │
│ └─────────────────────────────────────────────────────┘ │
│                                                         │
│ PRIVACY TAB:                                            │
│ ┌─────────────────────────────────────────────────────┐ │
│ │ Image proxy           [✓] Proxy images for privacy  │ │
│ │ Link tracking removal [✓] Remove tracking params    │ │
│ │ Search history        [ ] Disabled (local only)     │ │
│ └─────────────────────────────────────────────────────┘ │
│                                                         │
│ ENGINES TAB:                                            │
│ ┌─────────────────────────────────────────────────────┐ │
│ │ ◉ Google         [✓] Web [✓] Images [✓] Videos     │ │
│ │ ◉ Bing           [✓] Web [✓] Images [ ] Videos     │ │
│ │ ◉ DuckDuckGo     [✓] Web [✓] Images                │ │
│ │ ◉ Brave          [✓] Web [ ] Images [ ] Videos     │ │
│ │ ○ Qwant          [ ] Web [ ] Images                │ │
│ │ ○ Mojeek         [ ] Web                           │ │
│ │                                                     │ │
│ │ Drag to reorder priority                            │ │
│ └─────────────────────────────────────────────────────┘ │
│                                                         │
│ DATA TAB:                                               │
│ ┌─────────────────────────────────────────────────────┐ │
│ │ [Export Settings]  Download as JSON                 │ │
│ │ [Import Settings]  Upload JSON file                 │ │
│ │ [Generate Link]    Create shareable preferences URL │ │
│ │ [Reset All]        Restore default settings         │ │
│ │                                                     │ │
│ │ Preference String:                                  │ │
│ │ ┌───────────────────────────────────────────────┐   │ │
│ │ │ t=d;c=web;s=m;r=20;n=1;p=i;k=1              │   │ │
│ │ └───────────────────────────────────────────────┘   │ │
│ │ [Copy] [QR Code]                                    │ │
│ └─────────────────────────────────────────────────────┘ │
│                                                         │
│                              [Save Preferences]         │
└─────────────────────────────────────────────────────────┘
```

#### Mobile Responsive

- Hamburger menu for categories on small screens
- Full-width search bar
- Stack results vertically
- Touch-friendly tap targets (44px minimum)
- Swipe gestures for video previews
- Bottom sheet for preferences on mobile

---

### Data Models

#### Search Query
- query: string - Search query text
- category: enum - web, images, videos, news, maps, files, music, science, it, social
- page: int - Pagination (1-indexed)
- language: string - ISO language code
- region: string - ISO country code
- safe_search: enum - off, moderate, strict
- time_range: enum - any, hour, day, week, month, year, custom
- time_from: date - Custom range start
- time_to: date - Custom range end
- engines: []string - Specific engines to query
- format: enum - html, json, rss, csv

#### Search Result
- title: string - Result title
- url: string - Result URL
- snippet: string - Text excerpt/description
- thumbnail: string - Image/video thumbnail URL
- source: string - Source engine name
- engine_rank: int - Position in source engine
- score: float - Aggregated relevance score
- category: string - Result category
- published: date - Publication date (if available)
- cached_url: string - Link to cached version

#### Image Result (extends Search Result)
- width: int - Image width
- height: int - Image height
- format: string - Image format (jpg, png, gif, webp)
- file_size: int - File size in bytes

#### Video Result (extends Search Result)
- duration: int - Video duration in seconds
- views: int - View count
- channel: string - Channel/author name
- embed_url: string - Embeddable player URL

#### User Preferences
- theme: string - dark, light, auto
- safe_search: string - off, moderate, strict
- default_category: string - Default search category
- results_per_page: int - 10, 20, 50, 100
- new_tab: bool - Open results in new tab
- keyboard_shortcuts: bool - Enable keyboard navigation
- infinite_scroll: bool - Use infinite scroll vs pagination
- custom_bangs: map[string]string - Custom bang shortcuts
- homepage_widgets: []string - Enabled homepage widget types

#### Search Alert
- id: string - Unique alert identifier
- query: string - Search query being monitored
- category: string - web, images, videos, news, maps, files, music, science, it, social
- language: string - ISO language code
- region: string - ISO country code
- safe_search: string - off, moderate, strict
- engines: []string - Optional engine subset for this alert
- frequency: string - immediate, daily, weekly
- email: string - Verified destination email
- rss_token: string - Private token for the alert RSS feed
- webhook_url: string - Optional signed webhook target
- webhook_secret: string - Secret used to sign webhook payloads
- verified: bool - Alert activation state
- paused: bool - Delivery temporarily disabled
- last_checked: timestamp - Last search check time
- last_sent: timestamp - Last successful delivery time
- seen_results: []string - Result URL hashes already delivered
- manage_token: string - Signed token for accountless management links

#### Engine Status
- name: string - Engine identifier
- display_name: string - Human-readable name
- enabled: bool - Admin-enabled
- healthy: bool - Currently responding
- last_check: timestamp - Last health check
- avg_response_ms: int - Average response time
- error_rate: float - Recent error percentage
- categories: []string - Supported categories
- rate_limit: int - Requests per minute limit
- weight: float - Ranking weight multiplier

#### Instant Answer
- type: string - calculator, weather, dictionary, etc.
- query: string - Matched query pattern
- result: any - Answer data (type-specific)
- source: string - Data source attribution
- cache_ttl: int - Seconds to cache

---

### Business Rules

#### Privacy
- No server-side logging of user queries, IPs, or behavior
- No cookies required for core functionality
- All user preferences stored client-side only
- Search queries never associated with user identifiers
- Referrer headers stripped before following result links
- Tracking parameters removed from result URLs
- Search alerts are opt-in and store only the minimum data required for verification, scheduling, deduplication, and delivery

#### Search Alerts
- Alerts require email verification before activation
- Alerts may be delivered by email, private RSS, webhook, or any enabled combination
- Each alert checks for newly matched results only; previously delivered results are suppressed
- Manage/unsubscribe actions use signed, unguessable tokens instead of accounts
- Webhook deliveries are signed and retried with backoff on transient failures
- RSS feeds are private and tokenized; they must not be publicly enumerable
- Immediate alerts should batch closely-timed results to avoid notification spam

#### Engine Management
- Engines automatically disabled after consecutive failures
- Engines re-enabled after successful health check
- Request distribution balanced across healthy engines
- Rate limits respected per engine configuration
- Results cached briefly to reduce engine load

#### Result Ranking
- Results weighted by: source engine reliability, position in source, frequency across engines
- Duplicate URLs merged, keeping best metadata
- Blocked domains filtered before display
- Safe search applied at query time

#### Caching
- Search results cached 5 minutes (configurable)
- Autocomplete cached 1 hour
- Instant answers cached by type (weather: 30min, currency: 1hr, etc.)
- Engine health status cached 1 minute

---

### Endpoints

Route paths and HTTP methods are defined in AI.md PART 14 (HOW). This section describes endpoint capabilities (WHAT).

#### Web Interface Capabilities
- Homepage with search entry point
- Search results pages per category (web, images, videos, news)
- User preferences settings page
- Search alert creation from any search context
- Alert email verification flow (confirm subscription)
- Per-alert management page (update, pause, delete) — accountless, accessed via signed token
- Private RSS feed per alert subscription
- About page and public instance statistics

#### JSON API Capabilities
- Search results as structured JSON with category filtering
- Autocomplete suggestions for search queries
- Instant answers (weather, currency, calculator, etc.)
- Available engine list with current health status
- Available search categories
- Available bang shortcuts
- Public instance configuration (non-sensitive settings only)
- Alert subscription lifecycle: create, email-verify, update settings, pause/resume, delete
- RSS feed payload for a single alert subscription

#### Autodiscovery
- OpenSearch descriptor for browser address-bar integration
- RSS feed of search results for any query

### Data Sources

#### Search Engines (Direct Sources Only)

Primary engines with their own indexes - no proxies or metasearch:

| Engine | Categories | Notes |
|--------|-----------|-------|
| Google | web, images, videos, news, maps | Largest index |
| Bing | web, images, videos, news | Microsoft's index |
| DuckDuckGo | web, images | Privacy-focused |
| Brave | web, images, videos, news | Independent index |
| Qwant | web, images, news | EU-based independent index |
| Mojeek | web | UK-based independent index |
| Yandex | web, images | Russian index (optional) |
| Baidu | web, images | Chinese index (optional) |

**Not included** (metasearch only - we go direct):
- Ecosia (Bing-powered)
- SearXNG (metasearch)

**Note**: Startpage and Yahoo ARE implemented - they provide alternative access paths.

#### Specialized Engines
| Engine | Category | Notes |
|--------|----------|-------|
| Wikipedia | web, instant | Instant answers |
| YouTube | videos | Video search |
| Reddit | social | Discussions |
| StackOverflow | it | Programming Q&A |
| GitHub | it | Code/repositories |
| Hacker News | it, news | Tech news |
| arXiv | science | Academic papers |
| PubMed | science | Medical research |
| Wolfram Alpha | instant | Computational answers |
| OpenStreetMap | maps | Open map data |

#### Instant Answer Data
| Type | Source | Update Frequency |
|------|--------|------------------|
| Weather | OpenWeatherMap / wttr.in | 30 minutes |
| Currency | exchangerate.host | 1 hour |
| Dictionary | Wiktionary API | On demand |
| IP Geolocation | sapics/ip-location-db | Weekly |
| Timezone | Built-in | Static |
| Calculator | Built-in | Static |
