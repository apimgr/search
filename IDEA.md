# search - Project Idea

## Purpose

Search is a privacy-respecting, self-hosted metasearch engine that aggregates results directly from primary search engines (Google, Bing, DuckDuckGo, Brave, Qwant, Mojeek) without tracking users. It combines the best features from Whoogle, SearX, SearXNG, DuckDuckGo, and major search engines into a single, reliable, always-working solution - but queries source engines directly rather than through proxies or other metasearch engines.

## Target Users

- Privacy-conscious individuals who want to search the web without being tracked
- Self-hosters who want to run their own search engine infrastructure
- Organizations requiring private, internal search capabilities
- Tor users seeking a search engine accessible via .onion services
- Power users who want customizable, keyboard-driven search
- Developers who need API access to search functionality

---

## Features

### Core Search

- **Multi-Engine Aggregation**: Query multiple engines simultaneously, merge and deduplicate results
- **Smart Ranking**: Weight and rank results based on engine reliability, result position, and frequency across engines
- **Multi-Category Search**: Web, images, videos, news, maps, files, music, science, IT, social media
- **Search Operators**: AND, OR, NOT, quotes, site:, filetype:, intitle:, inurl:, daterange:
- **Advanced Search Form**: GUI for building complex queries without knowing operators
- **Infinite Scroll / Pagination**: User choice between continuous loading or page-based navigation
- **Related Searches**: Suggestions for similar or refined queries

### Reliability ("Always Works")

- **Engine Health Monitoring**: Track response times, error rates, and availability per engine
- **Automatic Failover**: If primary engines fail, seamlessly switch to backups
- **Engine Rotation**: Distribute requests to avoid rate limiting and detection
- **Multiple Parsers**: HTML scraping + API fallback for each engine
- **Cached Results Fallback**: Serve stale results if all engines are temporarily down
- **Self-Healing**: Automatically re-enable recovered engines

### Privacy & Security

- **Zero Tracking**: No server-side logging of queries, IPs, or user behavior
- **No Cookies Required**: Fully functional without cookies
- **No JavaScript Required**: Core search works with JS disabled (progressive enhancement)
- **Tor Integration**: SOCKS5 proxy support, automatic circuit rotation, .onion hidden service
- **Proxy Chain Support**: Route requests through custom proxy chains
- **Request Sanitization**: Strip tracking parameters from outgoing requests
- **Referrer Hiding**: Never leak search queries to result sites

### User Preferences

Preferences persist across sessions without accounts. Three storage methods:

#### 1. localStorage (Default)
- Automatic, seamless browser storage
- Survives browser restarts

#### 2. Import/Export (Backup)
- Download preferences as JSON file
- Upload to restore on any device/browser
- Useful for backup and migration

#### 3. Preference String (Portable)
Compact URL-safe encoded string for sharing/bookmarking:

```
Format: Base64-encoded JSON or compact key=value

Compact format example:
t=d;l=en;s=m;e=g,b,d,q;c=web;k=1;r=20

Keys:
  t   = theme (d=dark, l=light, a=auto)
  l   = language (ISO code)
  s   = safe_search (o=off, m=moderate, s=strict)
  e   = engines (comma-separated: g=google, b=bing, d=duckduckgo, etc.)
  c   = default_category
  k   = keyboard_shortcuts (1=on, 0=off)
  r   = results_per_page
  n   = new_tab (1=on, 0=off)
  p   = pagination (i=infinite, p=pages)
  f   = font_size (s=small, m=medium, l=large)
```

**Use Cases:**
- **URL Parameter**: `https://search.example.com/?p=dDtkO2w9ZW4...`
- **Bookmarklet**: Search with preferences baked in
- **Share Config**: Give others your engine setup without accounts
- **QR Code**: Scan to apply preferences on mobile
- **CLI Flag**: `search-cli --prefs "t=d;e=g,b,d"`

**Workflow:**
1. Settings page → "Generate Link" button
2. Creates URL with `?prefs=ENCODED_STRING`
3. On page load, if `prefs` param exists → apply settings
4. Optional: save to localStorage for persistence

### Instant Answers (Widgets)

Zero-click answers displayed above search results. Each widget has trigger patterns and displays contextual information.

---

#### Calculator
**Triggers**: Mathematical expressions
```
2 + 2, 15% of 200, sqrt(144), sin(45), 2^10, (5+3)*2
```
**Displays**: Result with expression, copy button
**Features**: Basic arithmetic, percentages, powers, roots, trigonometry, logarithms, constants (pi, e)

---

#### Unit Converter
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

#### Currency Converter
**Triggers**: Amount + currency, "X USD to EUR"
```
100 usd to eur, $50 in pounds, 1000 jpy to usd
convert 500 euros to dollars
```
**Displays**: Converted amount, exchange rate, last updated time
**Features**: 150+ currencies, real-time rates, historical chart (7 days)
**Source**: exchangerate.host API (free, no key required)

---

#### Weather
**Triggers**: "weather", "weather in [location]", "[location] weather"
```
weather, weather in tokyo, new york weather
forecast london, temperature paris
```
**Displays**:
- Current: temp, feels like, conditions, humidity, wind
- Forecast: 5-day outlook with highs/lows
- Icon: sun, cloud, rain, snow, etc.
**Source**: wttr.in or OpenWeatherMap

---

#### Dictionary
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
**Source**: Wiktionary API / Free Dictionary API

---

#### Thesaurus
**Triggers**: "synonyms for [word]", "[word] synonyms", "antonyms of [word]"
```
synonyms for happy, beautiful synonyms, antonyms of good
words like "important"
```
**Displays**: Grouped synonyms/antonyms by meaning, word type labels
**Source**: Datamuse API / Wiktionary

---

#### IP Lookup
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
**Source**: ip-api.com / ipinfo.io / local GeoIP database

---

#### Color Picker
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

#### Timezone Converter
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
**Source**: Built-in Go time library + timezone database

---

#### Calendar / Date Calculator
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

#### Hash Generator
**Triggers**: "md5 [text]", "sha256 [text]", "hash [text]"
```
md5 hello world, sha256 password123
sha1 test, sha512 secret
hash my string
```
**Displays**: Hash output with algorithm label, copy button
**Algorithms**: MD5, SHA1, SHA256, SHA384, SHA512, CRC32

---

#### Base64 Encode/Decode
**Triggers**: "base64 encode [text]", "base64 decode [encoded]"
```
base64 encode hello world
base64 decode aGVsbG8gd29ybGQ=
b64 encode test, b64 decode dGVzdA==
```
**Displays**: Result with copy button, input/output labels

---

#### URL Encode/Decode
**Triggers**: "url encode [text]", "url decode [encoded]", "urlencode [text]"
```
url encode hello world!
url decode hello%20world%21
urlencode special chars: &?=
```
**Displays**: Encoded/decoded result with copy button

---

#### UUID Generator
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

#### Password Generator
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

#### QR Code Generator
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

#### Stopwatch / Timer
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

#### Random Number
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

#### Lorem Ipsum
**Triggers**: "lorem ipsum", "placeholder text", "dummy text"
```
lorem ipsum, lorem ipsum 3 paragraphs
placeholder text 100 words
dummy text 5 sentences
```
**Displays**: Generated text with copy button
**Options**: Paragraphs, sentences, or word count

---

#### Cryptocurrency Prices
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
**Source**: CoinGecko API (free tier)

---

#### Stock Prices
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
**Source**: Alpha Vantage / Yahoo Finance API
**Note**: May be delayed 15-20 minutes (free tier limitation)

---

#### Package Tracking
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

#### Translate
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
**Source**: LibreTranslate (self-hosted) or Lingva API

---

#### Wikipedia Summary
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
**Source**: Wikipedia API

---

#### Sports Scores
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
**Source**: ESPN API / TheSportsDB

---

#### Nutrition Facts
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
**Source**: USDA FoodData Central API

### Direct Answers (Full Page Results)

Unlike Instant Answers (widgets above search results), Direct Answers ARE the result. When a direct answer operator is detected, the response is a full-page dedicated view - no search results list.

**Syntax:** `{type}:{term}` or `{type}: {term}` (space after colon allowed)

---

#### tldr:{command}
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
**Source**: tldr-pages (https://tldr.sh) - cached locally, updated weekly
**Fallback**: If command not found, offer to search or show man page

---

#### man:{page}
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
**Source**: man.cx API / local man-db cache
**Features**:
- Section selector: `man:printf.3` (C library) vs `man:printf.1` (shell command)
- Search within page
- Related pages (SEE ALSO section)

---

#### cache:{url}
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

#### whois:{domain}
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
**Source**: WHOIS protocol servers / RDAP

---

#### dns:{domain}
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
**Source**: Built-in DNS resolver / multiple DNS servers
**Features**: Query specific record types: `dns:example.com/mx`, `dns:example.com/txt`

---

#### wiki:{topic}
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
**Source**: Wikipedia API
**Features**: Language selection, mobile-friendly rendering

---

#### dict:{word}
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
**Source**: Wiktionary API / Free Dictionary API

---

#### thesaurus:{word}
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
**Source**: Datamuse API / WordNet / Wiktionary

---

#### pkg:{name}
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

#### cve:{id}
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
**Source**: NVD (National Vulnerability Database) / MITRE

---

#### rfc:{number}
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
**Source**: IETF datatracker / rfc-editor.org
**Features**: Section deep-linking, search within document

---

#### ascii:{text}
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
**Source**: Built-in figlet-compatible renderer

---

#### qr:{text}
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
**Source**: Built-in QR generator
**Features**: WiFi QR format, vCard format, URL shortening option

---

#### resolve:{hostname}
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
**Source**: Built-in resolver + GeoIP database

---

#### cert:{domain}
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
**Source**: Direct TLS connection / crt.sh

---

#### headers:{url}
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
**Source**: Direct HTTP request
**Features**: Follow redirects option, custom request headers

---

### Direct Answer Behavior

**Query Processing Order:**
1. Check for direct answer operator (`type:term`)
2. If found → render full-page direct answer
3. If not found → continue to instant answers → search

**URL Format:**
- Direct answers accessible via: `/direct/{type}/{term}`
- Example: `/direct/tldr/git`, `/direct/man/bash`
- Also triggered from search: `/search?q=tldr:git`

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

### Bang Shortcuts

Quick redirects to specific sites/engines:

```
!g query     → Google
!b query     → Bing
!d query     → DuckDuckGo
!w query     → Wikipedia
!yt query    → YouTube
!gh query    → GitHub
!so query    → StackOverflow
!r query     → Reddit
!tw query    → Twitter/X
!amz query   → Amazon
!ebay query  → eBay
!maps query  → Google Maps
!osm query   → OpenStreetMap
!wa query    → Wolfram Alpha
!npm query   → NPM
!pypi query  → PyPI
!crates query → Crates.io
!mdn query   → MDN Web Docs
!arch query  → Arch Wiki
```

- **Custom Bangs**: Define your own shortcuts in settings
- **Bang Autocomplete**: Suggestions as you type `!`
- **Bang Categories**: Organize bangs by type (search, shopping, dev, etc.)

### Keyboard Shortcuts

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

### Theming & Customization

- **Built-in Themes**: Dark (Dracula), Light, Auto (system preference)
- **Custom CSS**: User-provided stylesheet override
- **Font Size**: Small, medium, large
- **Results Density**: Compact, comfortable, spacious
- **Accent Color**: Customizable highlight color
- **Logo Customization**: Admin can set custom logo/branding

### Filtering & Refinement

- **Time Filter**: Past hour, day, week, month, year, custom range
- **Region Filter**: Country/region-specific results
- **Language Filter**: Results in specific language
- **File Type Filter**: PDF, DOC, XLS, PPT, etc.
- **Site Filter**: Include/exclude specific domains
- **Domain Blocklist**: Never show results from blocked domains
- **Safe Search**: Off, moderate, strict

### Search History (Local Only)

- Stored in browser localStorage only (never server)
- Quick access to recent searches
- Clear history button
- Disable history option
- Export/import with preferences

### Browser Integration

- **OpenSearch**: Add as browser search engine
- **PWA Support**: Install as standalone app
- **Browser Extension**: Quick search from any page (future)
- **Search Bar Widget**: Embeddable search box for other sites

### Video Previews

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

## UI/Layout Specification

### Design Philosophy

Google-like simplicity with SearXNG-style preferences:
- Clean, minimal homepage with centered logo and search bar
- Results page with left-aligned results, no clutter
- Preferences accessible via gear icon, organized in tabs

### Homepage Layout

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

### Search Results Layout

```
┌─────────────────────────────────────────────────────────┐
│ [Logo] ┌─────────────────────┬───┐  [⚙️] [🌙]          │
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

### Image Results Layout

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

### Video Results Layout

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

### Preferences Page (SearXNG-style Tabs)

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
│ │ │ t=d;l=en;s=m;e=g,b,d;c=web;k=1;r=20          │   │ │
│ │ └───────────────────────────────────────────────┘   │ │
│ │ [Copy] [QR Code]                                    │ │
│ └─────────────────────────────────────────────────────┘ │
│                                                         │
│                              [Save Preferences]         │
└─────────────────────────────────────────────────────────┘
```

### Mobile Responsive

- Hamburger menu for categories on small screens
- Full-width search bar
- Stack results vertically
- Touch-friendly tap targets (44px minimum)
- Swipe gestures for video previews
- Bottom sheet for preferences on mobile

---

## Data Models

### Search Query
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

### Search Result
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

### Image Result (extends Search Result)
- width: int - Image width
- height: int - Image height
- format: string - Image format (jpg, png, gif, webp)
- file_size: int - File size in bytes

### Video Result (extends Search Result)
- duration: int - Video duration in seconds
- views: int - View count
- channel: string - Channel/author name
- embed_url: string - Embeddable player URL

### User Preferences
- theme: string - dark, light, auto
- language: string - ISO language code
- region: string - ISO country code
- safe_search: string - off, moderate, strict
- default_category: string - Default search category
- engines: []string - Enabled engines (ordered by preference)
- results_per_page: int - 10, 20, 50, 100
- new_tab: bool - Open results in new tab
- keyboard_shortcuts: bool - Enable keyboard navigation
- infinite_scroll: bool - Use infinite scroll vs pagination
- show_engine_badges: bool - Show source engine on results
- font_size: string - small, medium, large
- custom_css: string - User custom stylesheet
- custom_bangs: map[string]string - Custom bang shortcuts
- blocked_domains: []string - Never show results from these
- search_history_enabled: bool - Save local search history

### Engine Status
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

### Instant Answer
- type: string - calculator, weather, dictionary, etc.
- query: string - Matched query pattern
- result: any - Answer data (type-specific)
- source: string - Data source attribution
- cache_ttl: int - Seconds to cache

---

## Business Rules

### Privacy
- No server-side logging of user queries, IPs, or behavior
- No cookies required for core functionality
- All user preferences stored client-side only
- Search queries never associated with user identifiers
- Referrer headers stripped before following result links
- Tracking parameters removed from result URLs

### Engine Management
- Engines automatically disabled after consecutive failures
- Engines re-enabled after successful health check
- Request distribution balanced across healthy engines
- Rate limits respected per engine configuration
- Results cached briefly to reduce engine load

### Result Ranking
- Results weighted by: source engine reliability, position in source, frequency across engines
- Duplicate URLs merged, keeping best metadata
- Blocked domains filtered before display
- Safe search applied at query time

### Caching
- Search results cached 5 minutes (configurable)
- Autocomplete cached 1 hour
- Instant answers cached by type (weather: 30min, currency: 1hr, etc.)
- Engine health status cached 1 minute

---

## Endpoints

### Web Interface
| Method | URL | Description |
|--------|-----|-------------|
| GET | / | Homepage with search box |
| GET | /search | Search results page |
| GET | /images | Image search |
| GET | /videos | Video search |
| GET | /news | News search |
| GET | /settings | User preferences page |
| GET | /about | About page |
| GET | /stats | Public instance statistics |

### API
| Method | URL | Description |
|--------|-----|-------------|
| GET | /api/v1/search | JSON search results |
| GET | /api/v1/autocomplete | Search suggestions |
| GET | /api/v1/instant | Instant answers |
| GET | /api/v1/engines | Available engines and status |
| GET | /api/v1/categories | Search categories |
| GET | /api/v1/bangs | Available bang shortcuts |
| GET | /api/v1/config | Public instance configuration |
| GET | /opensearch.xml | OpenSearch descriptor |
| GET | /search.rss | Search results as RSS feed |

### Admin (see AI.md PART 17)
| Method | URL | Description |
|--------|-----|-------------|
| GET | /admin/dashboard | Admin dashboard |
| GET | /admin/engines | Engine management |
| GET | /admin/settings | Server settings |
| GET | /admin/stats | Detailed statistics |

---

## Data Sources

### Search Engines (Direct Sources Only)

Primary engines with their own indexes - no proxies or metasearch:

| Engine | Categories | Method | Notes |
|--------|-----------|--------|-------|
| Google | web, images, videos, news, maps | HTML scraping | Largest index |
| Bing | web, images, videos, news | HTML scraping | Microsoft's index |
| DuckDuckGo | web, images | HTML scraping | Privacy-focused |
| Brave | web, images, videos, news | API | Independent index |
| Qwant | web, images, news | API | EU-based independent index |
| Mojeek | web | API | UK-based independent index |
| Yandex | web, images | HTML scraping | Russian index (optional) |
| Baidu | web, images | HTML scraping | Chinese index (optional) |

**Not included** (proxies/metasearch - we go direct):
- Startpage (Google proxy)
- Yahoo (Bing-powered)
- Ecosia (Bing-powered)
- SearXNG (metasearch)

### Specialized Engines
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

### Instant Answer Data
| Type | Source | Update Frequency |
|------|--------|------------------|
| Weather | OpenWeatherMap / wttr.in | 30 minutes |
| Currency | exchangerate.host | 1 hour |
| Dictionary | Wiktionary API | On demand |
| IP Geolocation | sapics/ip-location-db | Weekly |
| Timezone | Built-in Go stdlib | Static |
| Calculator | Built-in | Static |

---

## Future Considerations

- **AI Summary**: Optional AI-powered result summaries (local models only)
- **Federated Instances**: Connect multiple search instances
- **Result Voting**: Community-driven result quality feedback
- **Personal Index**: Index bookmarks/notes for personal search
- **Browser Extension**: Quick search from any page
- **Mobile Apps**: Native iOS/Android apps
