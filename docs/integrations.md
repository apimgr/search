# Integrations

Search supports several machine-readable integration protocols for CLI tools,
browser plugins, automation, and monitoring agents.

## Autodiscovery

The `/api/autodiscover` endpoint (non-versioned, per spec) returns machine-readable
server metadata for CLI clients, agent tools, and other software that needs to
discover the server's capabilities automatically.

```bash
curl https://your-instance.example.com/api/autodiscover
```

### Response fields

| Field | Description |
|-------|-------------|
| `server.name` | Project name (`search`) |
| `server.version` | Running version |
| `server.min_version` | Minimum compatible client version |
| `server.url` | Canonical base URL |
| `server.onion` | Tor `.onion` address (if Tor enabled) |
| `api.version` | Current API version (`v1`) |
| `cli_versions` | Available client binary downloads per platform |
| `cli_min_version` | Minimum supported `search-cli` version |
| `features` | Enabled features flag map |
| `auth` | Auth method (`bearer`) and token scope |

The CLI client (`search-cli`) reads this endpoint on every startup to check for
updates and verify API compatibility.

## Search Bar Widget

Add a native HTML form to another site to send searches to a Search instance.
Replace the example host with the instance's public base URL, and change the
input ID if the page already uses `search-query`:

```html
<form action="https://your-instance.example.com/search" method="get" role="search">
  <label for="search-query">Search</label>
  <input id="search-query" type="search" name="q" required>
  <button type="submit">Search</button>
</form>
```

The browser submits the query using the same public search route as the main
site. The form works without JavaScript and loads no third-party scripts or
iframes. The instance's normal search privacy and result preferences apply.

## OpenSearch

Search ships a standard OpenSearch description file at `/opensearch.xml`.
Modern browsers (Chrome, Firefox, Edge, Safari) can import this file so users
can set Search as a browser search engine directly from the address bar.

```xml
<!-- The browser discovers the description via this <link> tag in the HTML head -->
<link rel="search"
      type="application/opensearchdescription+xml"
      title="Search"
      href="/opensearch.xml" />
```

No configuration is required. The OpenSearch description is generated
dynamically and always reflects the running instance's base URL.

## Alert Webhooks

Search alerts can deliver results to a webhook endpoint in addition to
email and RSS. Configure a webhook URL when creating an alert:

```bash
# Via API
curl -X POST https://your-instance/api/v1/alerts \
  -H "Content-Type: application/json" \
  -d '{
    "query": "golang security",
    "frequency": "daily",
    "deliver_webhook": true,
    "webhook_url": "https://hooks.example.com/ingest"
  }'
```

### Webhook payload

```json
{
  "alert": {
    "id": "<opaque-id>",
    "query": "golang security",
    "category": "general",
    "frequency": "daily",
    "manage_url": "https://your-instance.example.com/alerts/manage/<token>"
  },
  "results": [
    {
      "title": "...",
      "url": "https://...",
      "snippet": "..."
    }
  ],
  "sent_at": "2025-01-15T03:00:00Z"
}
```

The request includes `X-Search-Alert-Signature: sha256=<hex>`, an HMAC-SHA256
signature over the exact JSON request body using the alert's webhook secret.
Webhook requests use the configured retry count and retry delay from
`search.alerts.webhook_max_retries` and
`search.alerts.webhook_retry_delay_minutes`.

## Alert RSS Feeds

Each alert has a unique private RSS feed URL. The server returns the RSS URL
when the alert is created, and the accountless management page shows the URL
for an existing alert. Keep the URL private: possession of its unguessable
token grants access to that alert's feed. Email verification is not required.

RSS feeds are compatible with any standard RSS reader (Feedly, NetNewsWire,
Miniflux, etc.) and support the Atom 1.0 and RSS 2.0 formats.

## Prometheus Metrics

Search exposes Prometheus, Grafana dashboard, and Loki log services under
`/server/metrics`, `/api/v1/server/metrics`, `/api/metrics`, and (by default)
`/metrics`. Metrics are enabled by default, but each service requires its own
bearer token. Configure `server.metrics.auth.tokens.prometheus` for Prometheus,
`server.metrics.auth.tokens.grafana` for the dashboard, and
`server.metrics.auth.tokens.loki` for recent logs. Keep all metrics endpoints
internal and do not expose them through a public reverse proxy.

```yaml
# prometheus.yml scrape config
scrape_configs:
  - job_name: search
    static_configs:
      - targets: ['localhost:64580']
    metrics_path: /server/metrics
    authorization:
      credentials: 'your-prometheus-service-token'
```

The token is sent in the `Authorization` header. Empty service tokens disable
that service and return `403`; query-string tokens are not accepted. The
`server.metrics.auth.allow_unauthenticated` escape hatch is only for a
firewalled internal network. See [Configuration](configuration.md) for all
metrics settings.

## GraphQL

Search exposes a GraphQL API. The interactive GraphiQL explorer is served at
`GET /server/docs/graphql`, and queries are accepted at `POST /api/graphql`
(unversioned alias) or `POST /api/v1/server/graphql` (versioned canonical).
Core search, autocomplete, instant-answer, direct-answer, and health fields use the same services as REST; inspect the live GraphiQL schema for current query arguments and response fields. REST-only operations remain documented in the REST API reference.

```bash
curl -X POST https://your-instance/api/graphql \
  -H "Content-Type: application/json" \
  -d '{"query": "{ search(q: \"golang\") { results { title url } } }"}'
```

## Model Context Protocol (MCP)

Search exposes an MCP Streamable HTTP endpoint backed by the REST API. It uses
JSON-RPC over `POST`; there is no stdio mode or separate MCP service. The
canonical endpoint is `/api/v1/server/mcp`, with direct aliases at `/api/mcp`
and `/mcp`. The endpoint is enabled by default and can be disabled with
`server.mcp.enabled` in `server.yml`.

MCP uses the same bearer credentials and authorization as the REST API. Public
tools work anonymously; operator-only tools require the operator token. For a
client that needs owner-scoped access, create a separate linked token at
`/server/preferences/tokens` and send it in the `Authorization: Bearer ...`
header. Do not use the operator token for an MCP client. Cookies are ignored.
Tool availability follows the caller's permissions, and token-management
operations are not exposed as tools.

The server renders connection instructions and the current tool list at
`/server/help/mcp`; the page is available only while MCP is enabled. The
supported protocol revisions and request rules follow the [MCP
specification](https://modelcontextprotocol.io/specification).

## Discovery and web standards

| Route | Purpose |
|-------|---------|
| `/api/autodiscover` | Machine-readable server, API, and CLI compatibility metadata |
| `/opensearch.xml` | OpenSearch description, when OpenSearch is enabled |
| `/.well-known/llms.txt` and `/llms.txt` | Machine-readable API and capability summary |
| `/.well-known/security.txt` | RFC 9116 security contact and disclosure details |
| `/.well-known/pgp-key.asc` | Published security-report encryption key, when configured |
| `/.well-known/change-password` | Password-change discovery redirect |
| `/robots.txt` | Crawler guidance |
| `/sitemap.xml` | Public-page sitemap |

Search does not enable WebFinger, OpenID Provider Metadata, Android App Links,
or Apple app-site-association endpoints. Unknown `/.well-known/*` paths return
`404`. See [Security](security.md) for the security-reporting endpoints and
configuration.

## Security.txt / Well-Known

Search serves a `/.well-known/security.txt` file for responsible disclosure.
Operators configure the contact and PGP key via `server.security.reporting`
in `server.yml`. See [Security](security.md) for details.
