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

#### directory:{term}
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

#### cheat:{command}
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
**Source**: cheat.sh API
**Difference from tldr**: More verbose, community examples, covers edge cases

---

#### http:{code}
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
**Source**: Built-in database (RFC 9110)

---

#### port:{number}
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
**Source**: IANA port registry + common knowledge database

---

#### cron:{expression}
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
**Source**: Built-in parser
**Features**: Validates expression, warns about common mistakes

---

#### chmod:{permissions}
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
**Source**: Built-in calculator

---

#### regex:{pattern}
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
**Source**: Built-in parser + regex101-style engine
**Features**: Supports test strings, shows capture groups

---

#### jwt:{token}
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
**Source**: Built-in decoder
**Security**: Never logs tokens, decoding is client-side only

---

#### timestamp:{value}
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
**Source**: Built-in converter
**Features**: Bidirectional (timestamp ↔ date), timezone selector

---

#### asn:{number}
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
**Source**: RIPE NCC / BGPView / ipinfo.io

---

#### subnet:{cidr}
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
**Source**: Built-in calculator
**Features**: IPv4 and IPv6 support

---

#### robots:{domain}
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
**Source**: Direct fetch from domain/robots.txt

---

#### sitemap:{domain}
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
**Source**: Direct fetch from domain/sitemap.xml (and robots.txt sitemap references)

---

#### tech:{domain}
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
**Source**: HTTP headers + HTML analysis + Wappalyzer-style detection
**Features**: Confidence scores, detection method shown

---

#### feed:{domain}
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
**Source**: HTML link tags, common paths (/feed, /rss, /atom.xml), robots.txt

---

#### expand:{url}
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
**Source**: Direct HTTP follow with redirect capture
**Supported shorteners**: bit.ly, t.co, tinyurl, goo.gl, ow.ly, is.gd, and any HTTP redirect

---

#### safe:{url}
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
**Source**: Google Safe Browsing API, domain reputation databases, SSL check
**Privacy**: Only domain is checked, not full URLs with parameters

---

#### html:{text}
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
**Source**: Built-in encoder
**Features**: Named entities (&amp;), decimal (&#38;), hex (&#x26;)

---

#### unicode:{char}
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
**Source**: Unicode database (built-in)

---

#### emoji:{name}
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
**Source**: Unicode CLDR + emoji-data

---

#### escape:{text}
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
**Source**: Built-in escapers

---

#### json:{data}
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
**Source**: Built-in parser

---

#### yaml:{data}
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
**Source**: Built-in parser

---

#### diff:{texts}
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
**Source**: Built-in diff algorithm
**Features**: Syntax highlighting for code, multiple diff formats

---

#### beautify:{code}
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
**Source**: Built-in formatters

---

#### case:{text}
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
**Source**: Built-in converter

---

#### slug:{text}
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
**Source**: Built-in generator

---

#### lorem:{count}
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
**Source**: Built-in generator

---

#### word:{text}
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
**Source**: Built-in analyzer

---

#### useragent:{string}
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
**Source**: Built-in parser + UA database
**Features**: `useragent:my` shows current browser's UA

---

#### mime:{type}
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
**Source**: IANA media types registry
**Features**: Lookup by MIME type, extension, or common name

---

#### license:{name}
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
**Source**: SPDX license list + choosealicense.com data

---

#### country:{code}
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
**Source**: REST Countries API / built-in database

---

#### slang:{term}
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
**Source**: Urban Dictionary API
**Features**:
- Multiple definitions shown (top 5 by votes)
- NSFW filter option (configurable)
- "Random slang" if no term provided
- Number codes (67, 420, etc.) supported

---

#### rules:{query}
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
**Source**: Built-in database (classic internet culture)
**Features**:
- Complete rules 1-100+ from internet folklore
- Search functionality for finding relevant rules
- Humorous/nostalgic easter egg for internet veterans

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

**Not included** (metasearch only - we go direct):
- Ecosia (Bing-powered)
- SearXNG (metasearch)

**Note**: Startpage and Yahoo ARE implemented - they provide alternative access paths.

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

