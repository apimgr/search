# Admin Panel

The admin panel provides a web-based interface for managing your Search instance.

## Accessing the Admin Panel

Navigate to `/admin` on your Search instance:

```
https://your-search-instance.com/admin
```

## First-Time Setup

On first run, you'll be directed to the setup wizard at `/admin/setup`. This wizard will guide you through:

1. Creating an admin account
2. Basic server configuration
3. Initial settings

## Authentication

### Web UI

The admin panel uses session-based authentication. Log in with your admin username and password at `/admin/login`.

Sessions expire after 30 days of inactivity by default.

### API Access

The admin API at `/api/v1/admin/*` uses Bearer token authentication:

```bash
curl -H "Authorization: Bearer adm_YOUR_TOKEN" \
  "https://search.example.com/api/v1/admin/status"
```

## Dashboard

The dashboard (`/admin/dashboard`) provides an overview of your instance:

### Status Widgets

- **Status**: Server status (Online/Maintenance/Error)
- **Uptime**: How long the server has been running
- **Requests (24h)**: Total requests in the last 24 hours
- **Errors (24h)**: Error count in the last 24 hours

### System Resources

- CPU usage
- Memory usage
- Disk usage

### Quick Actions

- Reload Config
- Create Backup
- View Logs

### Recent Activity

Shows recent admin actions and system events.

### Scheduled Tasks

Displays upcoming scheduled tasks:

- Automatic Backup (02:00 daily)
- SSL Renewal Check (03:00 daily)
- GeoIP Update (03:00 Sunday)
- Session Cleanup (hourly)

### Alerts & Warnings

Displays any system warnings or alerts that need attention.

## Server Settings

### General (`/admin/server/settings`)

- Instance title and description
- Base URL
- Server port
- Application mode

### Branding (`/admin/server/branding`)

- Logo and favicon
- Color scheme
- Custom CSS

### SSL/TLS (`/admin/server/ssl`)

- Enable/disable SSL
- Certificate management
- Let's Encrypt configuration
- Auto-renewal settings

### Tor (`/admin/server/tor`)

- Enable/disable Tor hidden service
- View .onion address
- Regenerate address
- Vanity address generation

### Web Server (`/admin/server/web`)

- robots.txt configuration
- security.txt settings
- CORS settings
- Compression options

### Email (`/admin/server/email`)

- SMTP configuration
- Email templates
- Test email sending

### GeoIP (`/admin/server/geoip`)

- Enable/disable GeoIP
- Country blocking/allowing
- Database updates

### Metrics (`/admin/server/metrics`)

- Prometheus metrics configuration
- Metrics retention

## Search Engines

Manage search engines at `/admin/engines`:

- Enable/disable engines
- Set priorities
- Configure engine-specific settings
- View engine status and health

## API Tokens

Manage API tokens at `/admin/tokens`:

- Create new tokens
- Set expiration dates
- View usage statistics
- Revoke tokens

Token format: `key_XXXXXXXXXXXXXXXXXXXX`

## Logs

View server logs at `/admin/logs`:

- Access logs
- Error logs
- Audit logs
- Filter by date and severity

## Scheduler

Manage scheduled tasks at `/admin/scheduler`:

- View scheduled tasks
- Run tasks manually
- Configure task schedules
- View task history

## Cluster Management

For multi-node deployments at `/admin/server/nodes`:

- View cluster nodes
- Generate join tokens
- Remove nodes
- View cluster health

## Admin Management

Manage admin users at `/admin/users/admins`:

- Invite new admins
- Edit admin permissions
- Remove admins
- View admin activity

## Keyboard Shortcuts

| Shortcut | Action |
|----------|--------|
| `g d` | Go to Dashboard |
| `g c` | Go to Configuration |
| `g e` | Go to Engines |
| `g l` | Go to Logs |
| `?` | Show shortcuts help |
