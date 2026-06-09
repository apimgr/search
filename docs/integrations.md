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
  "event": "alert_results",
  "alert_id": "<opaque-id>",
  "query": "golang security",
  "results": [
    {
      "title": "...",
      "url": "https://...",
      "snippet": "..."
    }
  ],
  "result_count": 5,
  "triggered_at": "2025-01-15T03:00:00Z"
}
```

Webhooks are delivered with a 30-second timeout and retried up to the configured
`search.alerts.webhook_max_retries` times on failure.

## Alert RSS Feeds

Each alert has a unique RSS feed URL. After verifying your email for an alert,
the management page shows your personal RSS URL.

RSS feeds are compatible with any standard RSS reader (Feedly, NetNewsWire,
Miniflux, etc.) and support the Atom 1.0 and RSS 2.0 formats.

## Prometheus Metrics

Search exposes Prometheus-compatible metrics at `/metrics`. This endpoint is
intended for internal monitoring only — do not expose it to the public internet.

```yaml
# prometheus.yml scrape config
scrape_configs:
  - job_name: search
    static_configs:
      - targets: ['localhost:64080']
    metrics_path: /metrics
    # Optional bearer token if configured:
    # bearer_token: <server.metrics.token>
```

See [Configuration](configuration.md) for `server.metrics` settings.

## GraphQL

Search exposes a GraphQL API at `/graphql` (POST for queries, GET for GraphiQL UI).
The schema mirrors the REST API and is documented in the embedded GraphiQL explorer.

```bash
curl -X POST https://your-instance/graphql \
  -H "Content-Type: application/json" \
  -d '{"query": "{ search(q: \"golang\") { results { title url } } }"}'
```

## Security.txt / Well-Known

Search serves a `/.well-known/security.txt` file for responsible disclosure.
Operators configure the contact and PGP key via `server.security.reporting`
in `server.yml`. See [Security](security.md) for details.
