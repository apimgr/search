# API Reference

Search provides both REST and GraphQL APIs for programmatic access.

## Endpoints

| Endpoint | Description |
|----------|-------------|
| `/healthz` | Health check page |
| `/openapi` | Swagger UI |
| `/openapi.json` | OpenAPI specification (JSON) |
| `/graphql` | GraphQL endpoint (GET=GraphiQL, POST=queries) |
| `/metrics` | Prometheus metrics |
| `/api/v1/` | REST API |

## REST API

### Search

#### `GET /api/v1/search`

Perform a search query.

**Query Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `q` | string | Yes | Search query |
| `page` | int | No | Page number (default: 1) |
| `per_page` | int | No | Results per page (default: 10, max: 100) |
| `category` | string | No | Search category (general, images, videos, news) |
| `lang` | string | No | Language code (e.g., "en") |
| `safe` | string | No | Safe search level (off, moderate, strict) |

**Example Request:**

```bash
curl "https://search.example.com/api/v1/search?q=privacy&per_page=10"
```

**Example Response:**

```json
{
  "query": "privacy",
  "results": [
    {
      "title": "Privacy - Wikipedia",
      "url": "https://en.wikipedia.org/wiki/Privacy",
      "description": "Privacy is the ability of an individual...",
      "engine": "duckduckgo",
      "position": 1
    }
  ],
  "total": 100,
  "page": 1,
  "per_page": 10
}
```

### Suggestions

#### `GET /api/v1/autocomplete`

Get search suggestions.

**Query Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `q` | string | Yes | Partial search query |

**Example Request:**

```bash
curl "https://search.example.com/api/v1/autocomplete?q=priv"
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

### Search Alerts

Search alerts are managed through the REST API and use unguessable manage and RSS tokens instead of accounts.

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
| `email` | string | Yes | Contact email for verification and notifications |
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
      "EmailVerified": true,
      "Status": "active",
      "BaseURL": "https://search.example.com"
    },
    "manage_url": "https://search.example.com/alerts/manage/MANAGE_TOKEN",
    "rss_url": "https://search.example.com/alerts/RSS_TOKEN.rss",
    "manage_token": "MANAGE_TOKEN",
    "rss_token": "RSS_TOKEN",
    "verification_sent": false
  }
}
```

#### `GET /api/v1/alerts/{token}`

Return alert details for a manage token, including the current manage and RSS URLs.

#### `PATCH /api/v1/alerts/{token}`

Update alert query filters or delivery settings.

#### `POST /api/v1/alerts/{token}/verify`

Verify and activate an alert using the one-time email verification token.

#### `POST /api/v1/alerts/{token}/pause`

Pause or resume an alert. Send `{"paused": true}` to pause or `{"paused": false}` to resume.

#### `DELETE /api/v1/alerts/{token}`

Delete an alert permanently.

#### `GET /api/v1/alerts/{token}/rss`

Return the private RSS feed for an alert.

## Server Management API

Server management endpoints require the operator token (`server.token` in `server.yml`).

### Authentication

Include the operator token in the Authorization header:

```bash
curl -H "Authorization: Bearer YOUR_OPERATOR_TOKEN" \
  "https://search.example.com/api/v1/server/healthz"
```

### Health

#### `GET /api/v1/server/healthz`

Get server health status. This endpoint is also available publicly at `/server/healthz`.

**Response:**

```json
{
  "ok": true,
  "data": {
    "status": "healthy",
    "version": "1.0.0",
    "uptime": "3d 14h 22m"
  }
}
```

### Scheduler

#### `GET /api/v1/server/scheduler/tasks`

List all scheduler tasks and their last run status.

#### `POST /api/v1/server/scheduler/tasks/{task}/run`

Trigger a scheduled task to run immediately.

### Backups

#### `GET /api/v1/server/backups`

List available backups.

#### `POST /api/v1/server/backups`

Create a new backup.

## GraphQL API

Access the GraphQL endpoint at `/graphql`:

- **GET**: Opens GraphiQL (interactive IDE)
- **POST**: Execute GraphQL queries

Search alert management is currently exposed through the REST API only.

### Schema

```graphql
type Query {
  search(query: String!, page: Int, perPage: Int): SearchResults!
  suggestions(query: String!): [String!]!
  status: ServerStatus!
}

type SearchResults {
  query: String!
  results: [SearchResult!]!
  total: Int!
  page: Int!
  perPage: Int!
}

type SearchResult {
  title: String!
  url: String!
  description: String
  engine: String!
  position: Int!
}

type ServerStatus {
  status: String!
  uptime: String!
  version: String!
}
```

### Example Query

```graphql
query {
  search(query: "privacy", perPage: 5) {
    query
    total
    results {
      title
      url
      description
    }
  }
}
```

## Rate Limiting

API requests are rate limited. The default limits are:

- 60 requests per minute
- Burst of 10 requests

Rate limit headers are included in responses:

- `X-RateLimit-Limit`: Maximum requests per minute
- `X-RateLimit-Remaining`: Remaining requests in current window
- `X-RateLimit-Reset`: Unix timestamp when the limit resets

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
