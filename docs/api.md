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

## Admin API

The admin API requires authentication via Bearer token.

### Authentication

Include the API token in the Authorization header:

```bash
curl -H "Authorization: Bearer YOUR_API_TOKEN" \
  "https://search.example.com/api/v1/admin/status"
```

### Status

#### `GET /api/v1/admin/status`

Get server status.

**Response:**

```json
{
  "status": "online",
  "uptime": "3d 14h 22m",
  "version": "1.0.0",
  "engines": 12,
  "requests_24h": 15234
}
```

### Configuration

#### `GET /api/v1/admin/config`

Get current configuration.

#### `PUT /api/v1/admin/config`

Update configuration.

### Backups

#### `GET /api/v1/admin/backups`

List available backups.

#### `POST /api/v1/admin/backups`

Create a new backup.

## GraphQL API

Access the GraphQL endpoint at `/graphql`:

- **GET**: Opens GraphiQL (interactive IDE)
- **POST**: Execute GraphQL queries

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

All errors return a JSON response:

```json
{
  "error": {
    "code": "RATE_LIMITED",
    "message": "Too many requests. Please wait before trying again.",
    "retry_after": 60
  }
}
```

Common error codes:

| Code | HTTP Status | Description |
|------|-------------|-------------|
| `INVALID_REQUEST` | 400 | Invalid request parameters |
| `UNAUTHORIZED` | 401 | Missing or invalid authentication |
| `FORBIDDEN` | 403 | Insufficient permissions |
| `NOT_FOUND` | 404 | Resource not found |
| `RATE_LIMITED` | 429 | Rate limit exceeded |
| `INTERNAL_ERROR` | 500 | Server error |
