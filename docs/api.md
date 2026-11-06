# API Reference

Search provides both REST and GraphQL APIs for programmatic access.

Examples use the public service at `https://scour.li`. For a self-hosted
instance, replace that origin with the public base URL configured for your
server.

## Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/healthz` | GET | Optional public health alias; enabled by `server.healthz.root.enabled` |
| `/server/docs/swagger` | GET | Swagger UI |
| `/api/v1/server/swagger` | GET | Versioned OpenAPI specification (JSON) |
| `/api/swagger` | GET | OpenAPI specification for the current API version (JSON) |
| `/server/docs/graphql` | GET | GraphiQL interactive explorer |
| `/api/graphql` | POST | GraphQL queries (unversioned alias) |
| `/api/v1/server/graphql` | POST | GraphQL queries (versioned canonical) |
| `/server/help/mcp` | GET | MCP client help page (only while MCP is enabled) |
| `/api/mcp`, `/mcp` | POST | MCP direct aliases (only while MCP is enabled) |
| `/api/v1/server/mcp` | POST | Versioned MCP endpoint (only while MCP is enabled) |
| `/server/metrics[/{service}]` | GET | Internal metrics services (per-service bearer token required) |

### Public docs routes

Interactive API documentation is served under the `/server/docs/*` and OpenAPI
namespaces:

| Route | Method | Description |
|-------|--------|-------------|
| `/server/docs/swagger` | GET | Swagger UI |
| `/api/v1/server/swagger` | GET | Versioned OpenAPI 3 specification |
| `/api/swagger` | GET | OpenAPI 3 specification for the current API version |
| `/server/docs/graphql` | GET | GraphiQL explorer (POSTs to `/api/graphql`) |

### Unversioned aliases

Convenience aliases that map to the canonical versioned handlers:

| Alias | Canonical | Description |
|-------|-----------|-------------|
| `/api/healthz` | `/api/v1/server/healthz` | Health check |
| `/api/v1/server/healthz.txt` | `/api/v1/server/healthz` | Health check (plain text) |
| `/api/graphql` | `/api/v1/server/graphql` | GraphQL query endpoint |
| `/api/mcp`, `/mcp` | `/api/v1/server/mcp` | MCP Streamable HTTP endpoint |
| `/api/autodiscover` | — | Machine-readable server metadata (see [Integrations](integrations.md)) |

## REST API

### Search

#### `GET /api/v1/search`

Perform a search query.

**Query Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `q` | string | Yes | Search query |
| `page` | int | No | Page number (default: 1) |
| `limit` | int | No | Results per page (default: 20, max: 100) |
| `category` | string | No | Search category |
| `lang` | string | No | Language code (e.g., `en`) |
| `region` | string | No | ISO country or region code |
| `safe_search` | int | No | Safe search level: `0` off, `1` moderate (default), `2` strict |
| `time_range` | string | No | `any`, `hour`, `day`, `week`, `month`, `year`, or `custom` |
| `time_from`, `time_to` | date | For `custom` | Inclusive date bounds in `YYYY-MM-DD` format |
| `engines` | string | No | Comma-separated or repeated engine names |

**Example Request:**

```bash
curl -q -LSsf "https://scour.li/api/v1/search?q=privacy&limit=10&region=us&time_range=week&engines=google,bing"
```

Use `POST /api/v1/search` to keep the query out of the request URL. Send a JSON
object with `query` and any of `category`, `page`, `limit`, `language`,
`region`, `safe_search`, `time_range`, `time_from`, `time_to`, and `engines`.
Custom ranges require both date bounds, and the request body is capped
at 1 MiB.

```bash
curl -q -LSsf -X POST https://scour.li/api/v1/search \
  -H 'Content-Type: application/json' \
  -d '{"query":"privacy","limit":10,"region":"us","time_range":"week","engines":["google","bing"]}'
```

**Example Response:**

```json
{
  "ok": true,
  "data": {
    "query": "privacy",
    "category": "general",
    "results": [
      {
        "title": "Privacy - Wikipedia",
        "url": "https://en.wikipedia.org/wiki/Privacy",
        "description": "Privacy is the ability of an individual...",
        "engine": "duckduckgo"
      }
    ],
    "engines": ["duckduckgo"]
  },
  "pagination": {"page": 1, "limit": 10, "total": 100, "pages": 10},
  "meta": {"version": "v1", "process_time_ms": 12.5}
}
```

Image-category results also return `image_url`, `image_width`, `image_height`,
`image_format`, and `file_size` when the upstream engine provides those values.
With the image proxy enabled (the default), both `thumbnail` and `image_url`
are signed same-origin `/image-proxy` URLs; clients should render those values
directly instead of loading the upstream image hosts.

Video results also include `duration` (seconds), `view_count`, and `author`.
YouTube supplies privacy-enhanced `embed_url` metadata. PeerTube supplies a
same-origin `preview_url` for public MP4/WebM files hosted on PeerTube.TV; the
server fetches file metadata only for local videos and proxies bounded byte
ranges so the browser never contacts the provider directly. Clients must reject
external preview URLs and retain the static thumbnail when no eligible clip is
present.

### Suggestions

#### `GET /api/v1/autocomplete`

Get search suggestions.

**Query Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `q` | string | Yes | Partial search query |

**Example Request:**

```bash
curl -q -LSsf "https://scour.li/api/v1/autocomplete?q=priv"
```

**Example Response:**

```json
{
  "suggestions": [
    "privacy",
    "privacy policy",
    "private",
    "privacy settings"
  ]
}
```

### Other Public Endpoints

The following read-only endpoints are available under `/api/v1/` without
authentication. Each returns the canonical `{"ok": true, "data": {...}}`
envelope (with `.txt` variants returning plain text where noted).

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/v1/info` (`/api/v1/info.txt`) | GET | Server info and capabilities |
| `/api/v1/search/related` | GET | Related searches for a query (`q`) |
| `/api/v1/instant` | GET | Instant answer for a query (`q`) |
| `/api/v1/direct/*` | GET | Full-page direct answers (calculator, conversions, etc.) |
| `/api/v1/categories` | GET | Available search categories |
| `/api/v1/engines` | GET | Search engine list and health |
| `/api/v1/engines/{id}` | GET | Details for a single engine |
| `/api/v1/bangs` | GET | Bang shortcut definitions (filter with `category`) |
| `/api/v1/widgets` | GET | Available instant-answer widgets (filter with `category`) |
| `/api/v1/widgets/{id}` | GET | Data for a single widget (e.g. `clock`, `weather`) |
| `/api/v1/server/help/mcp` | GET | MCP connection guidance and the currently available tool list (404 when MCP is disabled) |
| `/api/v1/server/preferences` | GET | Default user preferences |
| `/api/v1/favicon` | GET | Privacy-preserving favicon proxy (`url` param) |
| `/api/v1/server/about` | GET | Server about page (JSON) |
| `/api/v1/server/privacy` | GET | Privacy policy (JSON) |
| `/api/v1/server/terms` | GET | Terms of service (JSON) |
| `/api/v1/server/help` | GET | Help content (JSON) |
| `/api/v1/server/contact` | GET, POST | Contact information and message submission (JSON) |

`GET /api/v1/server/contact` reports whether the contact form is enabled. `POST`
accepts a JSON object with required `name`, `email`, `subject`, and `message`
fields. The message is queued for the configured general contact recipient,
falling back to the admin recipient. Invalid input returns `400`; disabled
contact or unavailable delivery returns a standard API error.

```json
{
  "name": "Ada Lovelace",
  "email": "ada@example.org",
  "subject": "Search feedback",
  "message": "Please consider adding another search engine."
}
```

### Search Alerts

Search alerts are accountless and managed through unguessable manage and RSS tokens. Alerts activate at creation time without an email verification flow, following AI.md PART 17.

`GET /api/v1/alerts` lists alert inventory and requires the operator bearer
token. `POST /api/v1/alerts` is public. When the request does not include an
owner bearer token, the response also returns an `owner_token`; save it if you
want to manage linked alerts through owner-scoped operations. The separate
`manage_token` in the response manages only that alert.

#### `POST /api/v1/alerts`

Create an alert subscription for a query.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `query` | string | Yes | Search query to monitor |
| `category` | string | Yes | Search category |
| `language` | string | No | Language filter (defaults to `en`) |
| `region` | string | No | Region filter |
| `engines` | array | No | Restrict the alert to selected engine names |
| `safe_search` | int | No | Safe search level (`0`, `1`, `2`) |
| `frequency` | string | Yes | `immediate`, `daily`, or `weekly` |
| `email` | string | Yes | Contact email for alert management and optional email notifications |
| `deliver_email` | bool | No | Enable email digests when SMTP is configured |
| `deliver_rss` | bool | No | Enable the private RSS feed |
| `deliver_webhook` | bool | No | Enable webhook delivery |
| `webhook_url` | string | No | Webhook destination when webhook delivery is enabled |

**Example Response:**

```json
{
  "ok": true,
  "data": {
    "alert": {
      "ID": "6b6b4b8f31f40dc8309cc6b66c78cb80",
      "Email": "alerts@example.com",
      "Query": "golang release notes",
      "Category": "news",
      "Language": "en",
      "Region": "",
      "Engines": [],
      "SafeSearch": 1,
      "Frequency": "daily",
      "DeliverEmail": false,
      "DeliverRSS": true,
      "DeliverWebhook": false,
      "Status": "active",
      "BaseURL": "https://scour.li"
    },
    "manage_url": "https://scour.li/alerts/manage/MANAGE_TOKEN",
    "rss_url": "https://scour.li/alerts/RSS_TOKEN.rss",
    "manage_token": "MANAGE_TOKEN",
    "rss_token": "RSS_TOKEN"
  }
}
```

#### `GET /api/v1/alerts/{token}`

Return alert details for a manage token, including the current manage and RSS URLs.

#### `PATCH /api/v1/alerts/{token}`

Update alert query filters or delivery settings.

#### `POST /api/v1/alerts/{token}/pause`

Pause or resume an alert. Send `{"paused": true}` to pause or `{"paused": false}` to resume.

#### `DELETE /api/v1/alerts/{token}`

Delete an alert permanently.

#### `GET /api/v1/alerts/{token}/rss`

Return the private RSS feed for an alert.

## Server Management API

Most server management endpoints require the operator token (`server.token` in `server.yml`). Health checks are public and do not require a token.

### Authentication

Include the operator token in the Authorization header:

```bash
curl -q -LSsf -H "Authorization: Bearer YOUR_OPERATOR_TOKEN" \
  "https://scour.li/api/v1/server/scheduler/tasks"
```

### Health

#### `GET /api/v1/server/healthz`

Get server health status. This endpoint is public and also available at
`/server/healthz`. The root `/healthz` alias is disabled by default and can be
enabled with `server.healthz.root.enabled`.

**Response:**

```json
{
  "project": {
    "name": "Search",
    "tagline": "Privacy-respecting metasearch",
    "description": ""
  },
  "status": "healthy",
  "version": "1.0.0",
  "go_version": "go1.27.0",
  "build": {"commit": "a1b2c3d", "date": "2026-01-01T00:00:00Z"},
  "uptime": "3d 14h 22m",
  "mode": "production",
  "timestamp": "2026-01-04T14:22:00Z",
  "features": {
    "tor": {"enabled": false, "running": false, "status": "disabled", "hostname": ""},
    "i2p": {"enabled": false, "running": false, "status": "disabled", "hostname": "", "provider": "none"},
    "geoip": true
  },
  "checks": {"database": "ok", "cache": "ok", "disk": "ok", "scheduler": "ok"},
  "stats": {"requests_total": 0, "requests_24h": 0, "active_connections": 0}
}
```

### Scheduler

#### `GET /api/v1/server/scheduler/tasks`

List all scheduler tasks and their last run status.

#### `GET /api/v1/server/scheduler/tasks/{task}`

Return the status and schedule for one task.

#### `GET /api/v1/server/scheduler/tasks/{task}/history`

Return the 20 most recent persisted runs for one task.

#### `POST /api/v1/server/scheduler/tasks/{task}/run`

Trigger a scheduled task to run immediately.

#### `POST /api/v1/server/scheduler/tasks/{task}/enable`

Enable a skippable task. Required tasks cannot be toggled.

#### `POST /api/v1/server/scheduler/tasks/{task}/disable`

Disable a skippable task. Required tasks cannot be toggled.

All scheduler endpoints require the operator bearer token.

### GeoIP

#### `GET /api/v1/server/geoip/ips`

Look up the requesting address using the local GeoIP database. The response includes source attribution.

#### `GET /api/v1/server/geoip/ips/{ip}`

Look up an explicit IP address. Private addresses are not resolved to public location data.

#### Presets

`GET /api/v1/server/geoip/presets` lists operator-authored country-list presets. `GET /api/v1/server/geoip/presets/{name}` reads one preset. `POST /api/v1/server/geoip/presets` saves a preset using `name` and either `codes` or `from` (`deny_countries` or `allow_countries`). `DELETE /api/v1/server/geoip/presets/{name}` deletes a preset. All preset operations require the operator bearer token; saving a preset does not apply it.

### Status & Config

#### `GET /api/v1/server/status`

Return detailed server status (operator token required).

#### `GET /api/v1/server/config`

Return the effective configuration with secrets masked (operator token required).

### Preferences and owner tokens

Preferences are browser-side and require no account. `GET
/api/v1/server/preferences` returns the preference schema, and `POST` accepts
the preference-save acknowledgement. `GET /api/v1/server/preferences/export`
creates a portable preference URL and short code from the current request;
`GET /api/v1/server/preferences/import` validates and applies a URL or code.

Owner token management is available through the `/api/v1/server/preferences/tokens`
routes and requires the primary owner bearer token. The routes list masked
tokens, create linked tokens, reveal or rotate the primary or a linked token,
update a linked token's name or expiry, and revoke linked tokens. Linked tokens
are intended for individual CLI, automation, or MCP clients and can be revoked
without rotating the primary token. These routes are never exposed as MCP tools.

### Contact notification webhooks

`POST /api/v1/server/config/contact/webhooks` configures an operator contact
notification webhook and requires the operator bearer token. The request
selects a notification role (`admin`, `security`, `abuse`, or `general`), a
supported transport, and an HTTPS destination. A newly generated signing
secret is returned once; save it in the receiving service. Later updates do
not reveal the secret again. See [Configuration](configuration.md) for the
supported transports and retry schedule.

### MCP

The versioned MCP endpoint is `POST /api/v1/server/mcp`; `/api/mcp` and `/mcp`
are direct aliases. It uses Streamable HTTP and the same bearer authentication,
authorization, and rate limits as the corresponding REST operations. Anonymous
clients can call public tools; operator-only tools require the operator token.
See [Integrations](integrations.md#model-context-protocol-mcp) for connection
guidance, token scope, and enable/disable behavior.

## GraphQL API

GraphQL uses the same live services and authorization checks as the corresponding REST API. Alert creation, management, private RSS feeds, and operator inventory are available alongside search and server information. GeoIP preset and scheduler task operations are available to authenticated operators through GraphQL, using the same persisted configuration, task manager, and authorization checks as REST.

`apiRequest(method:, path:, input:)` routes any non-GraphQL REST endpoint
through the registered API router, preserving its HTTP method, bearer token,
validation, and response status. Paths must be relative `/api/...` routes;
GraphQL and MCP recursion is rejected. Request JSON is limited to 64 KiB and
the response is limited to 4 MiB. Use a GraphQL mutation for this field,
including when invoking a REST GET.

- **GraphiQL UI** — `GET /server/docs/graphql`
- **Queries** — `POST /api/graphql` (unversioned alias) or
  `POST /api/v1/server/graphql` (versioned canonical)
- Requests are limited to 64 KiB, 12 nested field levels, and 100 selected
  fields after fragment expansion. Requests exceeding these limits receive
  HTTP 400; bodies larger than 64 KiB receive HTTP 413.

### Query fields

- `search(q:, category:, page:, lang:, limit:, safeSearch:, timeRange:, region:, engines:)`
  returns result data, pagination, engine names, and suggestions.
- `autocomplete(q:)` returns provider suggestions.
- `relatedSearches(q:, limit:)` returns cached, provider-backed, and generated related queries (limit 1–20; default 8).
- `engines`, `engine(id:)`, `categories`, `bangs(category:, search:)`, and `widgets(category:)` return public catalogs. `widgetData(type:, params:)` fetches data through the same widget manager as REST.
- `instant(q:)` processes the same instant-answer triggers as web search.
- `directAnswer(type:, term:)` runs a named direct-answer handler.
- `health` reports the current application version and uptime.
- `serverInfo` returns the same public runtime information as `GET /api/v1/info`.
- `preferences` and `savePreferences` match the public GET/POST preferences API.
- `geoIPLookup(ip:)` looks up an explicit IP through the same database and attributed response builder as REST.
- `serverStatus` and `serverConfig` use the same operator bearer-token authorization and redacted payloads as their REST endpoints.
- `alerts` lists alerts for operators only. `alert(manageToken:)` and `alertFeed(rssToken:)` require the corresponding private token.
- `createAlert(input:)`, `updateAlert(manageToken:, input:)`, `pauseAlert(manageToken:, paused:)`, and `deleteAlert(manageToken:)` use the same alert manager and lifecycle rules as REST. Supply `input` as a JSON variable object. `alertFeed(rssToken:)` returns only feed items and the feed URL.
- `schedulerTasks`, `schedulerTask(task:)`, and `schedulerHistory(task:)` provide operator-only scheduler reads. `runSchedulerTask(task:)`, `enableSchedulerTask(task:)`, and `disableSchedulerTask(task:)` provide the REST scheduler controls; manual runs write the same audit event.
- `geoIPPresets` and `geoIPPreset(name:)` read operator-authored country lists. `saveGeoIPPreset(input:)` and `deleteGeoIPPreset(name:)` persist preset changes to `server.yml`; saving or deleting a preset never changes the active allow/deny lists.
- `apiRequest(method:, path:, input:)` exposes REST endpoints not otherwise represented by a dedicated GraphQL field through the same registered handler and authorization chain.
- `saveContactWebhook(input:)` configures an operator contact webhook; a newly generated signing secret is returned once and the GraphQL response is marked `no-store`.

Search results expose `title`, `url`, `content`, `engine`, `score`, `category`,
`thumbnail`, `imageUrl`, `imageWidth`, `imageHeight`, `imageFormat`, `fileSize`,
`duration`, `viewCount`, `author`, `embedUrl`, `published`, and `domain`. Image
and video fields are zero/empty when the source does not provide them. Search response fields include
`query`, `category`, `results`, `totalResults`, `page`, `perPage`, `totalPages`,
`searchTime`, `engines`, and `suggestions`. Direct and instant answers expose
`details: JSON` for the type-specific `data` object returned by REST.

### Example query

```graphql
query Search($q: String!, $limit: Int) {
  search(q: $q, category: "general", limit: $limit) {
    query
    totalResults
    results {
      title
      url
      engine
      thumbnail
    }
  }
}
```

```json
{"query":"query Search($q: String!, $limit: Int) { search(q: $q, limit: $limit) { query totalResults results { title url } } }","variables":{"q":"golang","limit":5}}
```

## Rate Limiting

API requests are rate limited. The default limits are:

- Read endpoints: 120 requests per 60 seconds
- Write endpoints: 10 requests per 60 seconds
- Health endpoints: 120 requests per 60 seconds
- Global per-IP burst: 240 requests

Rate-limited responses include `Retry-After`. Configure limits through
`server.rate_limit` in `server.yml`; see [Configuration](configuration.md).

## Error Responses

All errors return a JSON response in canonical form:

```json
{
  "ok": false,
  "error": "RATE_LIMITED",
  "message": "Too many requests. Please wait before trying again."
}
```

Common error codes:

| Code | HTTP Status | Description |
|------|-------------|-------------|
| `BAD_REQUEST` | 400 | Invalid request parameters |
| `VALIDATION_FAILED` | 400 | Request validation failed |
| `UNAUTHORIZED` | 401 | Missing or invalid authentication |
| `FORBIDDEN` | 403 | Insufficient permissions |
| `NOT_FOUND` | 404 | Resource not found |
| `RATE_LIMITED` | 429 | Rate limit exceeded (`Retry-After` header set) |
| `SERVER_ERROR` | 500 | Internal server error |
| `MAINTENANCE` | 503 | Server is in maintenance mode |
