package admin

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	tlspkg "github.com/apimgr/search/src/tls"

	"github.com/apimgr/search/src/config"
)

// renderAdminLogin renders the admin login page
func (h *Handler) renderAdminLogin(w http.ResponseWriter, data *AdminPageData) {
	fmt.Fprintf(w, `<!DOCTYPE html>
<html lang="en" data-theme="dark">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <meta name="robots" content="noindex, nofollow">
    <title>%s - %s</title>
    <link rel="stylesheet" href="/static/css/main.css">
    <link rel="stylesheet" href="/static/css/admin.css">
</head>
<body>
    <div class="login-container">
        <div class="login-box">
            <div class="login-header">
                <h1>🔐 Admin Login</h1>
                <p>%s Administration</p>
            </div>
            %s
            <form method="POST" action="/admin/login">
                <input type="hidden" name="redirect" value="">
                <div class="form-group">
                    <label for="username">Username</label>
                    <input type="text" id="username" name="username" required autofocus>
                </div>
                <div class="form-group">
                    <label for="password">Password</label>
                    <input type="password" id="password" name="password" required>
                </div>
                <button type="submit" class="login-btn">Sign In</button>
            </form>
            <p class="back-link"><a href="/">← Back to Search</a></p>
        </div>
    </div>
</body>
</html>`,
		data.Title,
		h.config.Server.Title,
		h.config.Server.Title,
		func() string {
			if data.Error != "" {
				return fmt.Sprintf(`<div class="error-message">%s</div>`, data.Error)
			}
			return ""
		}(),
	)
}

// renderAdminPage renders admin pages with the admin layout
func (h *Handler) renderAdminPage(w http.ResponseWriter, page string, data *AdminPageData) {
	// Render admin header
	fmt.Fprintf(w, `<!DOCTYPE html>
<html lang="en" data-theme="dark">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <meta name="robots" content="noindex, nofollow">
    <title>%s - Admin - %s</title>
    <link rel="stylesheet" href="/static/css/main.css">
    <link rel="stylesheet" href="/static/css/admin.css">
</head>
<body>
    <div class="admin-layout">
        <aside class="admin-sidebar">
            <div class="admin-logo">
                <a href="/admin">🔍 Admin</a>
            </div>
            <ul class="admin-nav">
                <li><a href="/admin/dashboard" class="%s"><span class="nav-icon">📊</span> Dashboard</a></li>
                <li><a href="/admin/config" class="%s"><span class="nav-icon">⚙️</span> Configuration</a></li>
                <li><a href="/admin/engines" class="%s"><span class="nav-icon">🔎</span> Search Engines</a></li>
                <li><a href="/admin/tokens" class="%s"><span class="nav-icon">🔑</span> API Tokens</a></li>
                <li><a href="/admin/logs" class="%s"><span class="nav-icon">📜</span> Logs</a></li>
                <li><a href="/admin/users/admins" class="%s"><span class="nav-icon">👥</span> Server Admins</a></li>
                <li><a href="/admin/server/nodes" class="%s"><span class="nav-icon">🖧</span> Cluster</a></li>
            </ul>
            <div class="sidebar-section-header">
                <span>Server Settings</span>
            </div>
            <ul class="admin-nav">
                <li><a href="/admin/server/settings" class="%s"><span class="nav-icon">🖥️</span> General</a></li>
                <li><a href="/admin/server/branding" class="%s"><span class="nav-icon">🎨</span> Branding</a></li>
                <li><a href="/admin/server/ssl" class="%s"><span class="nav-icon">🔒</span> SSL/TLS</a></li>
                <li><a href="/admin/server/tor" class="%s"><span class="nav-icon">🧅</span> Tor</a></li>
                <li><a href="/admin/server/web" class="%s"><span class="nav-icon">🌐</span> Web Server</a></li>
                <li><a href="/admin/server/email" class="%s"><span class="nav-icon">📧</span> Email</a></li>
                <li><a href="/admin/server/announcements" class="%s"><span class="nav-icon">📢</span> Announcements</a></li>
                <li><a href="/admin/server/geoip" class="%s"><span class="nav-icon">🌍</span> GeoIP</a></li>
                <li><a href="/admin/server/metrics" class="%s"><span class="nav-icon">📈</span> Metrics</a></li>
                <li><a href="/admin/scheduler" class="%s"><span class="nav-icon">⏰</span> Scheduler</a></li>
                <li><a href="/" target="_blank"><span class="nav-icon">👁️</span> View Site</a></li>
            </ul>
        </aside>
        <main class="admin-main">
            <div class="admin-header">
                <h1>%s</h1>
                <div class="admin-user">
                    <span>Admin</span>
                    <a href="/admin/logout" class="logout-btn">Logout</a>
                </div>
            </div>`,
		data.Title,
		h.config.Server.Title,
		activeClass(page, "dashboard"),
		activeClass(page, "config"),
		activeClass(page, "engines"),
		activeClass(page, "tokens"),
		activeClass(page, "logs"),
		activeClass(page, "admins"),
		activeClass(page, "nodes"),
		activeClass(page, "server-settings"),
		activeClass(page, "server-branding"),
		activeClass(page, "server-ssl"),
		activeClass(page, "server-tor"),
		activeClass(page, "server-web"),
		activeClass(page, "server-email"),
		activeClass(page, "server-announcements"),
		activeClass(page, "server-geoip"),
		activeClass(page, "server-metrics"),
		activeClass(page, "scheduler"),
		data.Title,
	)

	// Render page content
	switch page {
	case "dashboard":
		h.renderDashboardContent(w, data)
	case "config":
		h.renderConfigContent(w, data)
	case "engines":
		h.renderEnginesContent(w, data)
	case "tokens":
		h.renderTokensContent(w, data)
	case "logs":
		h.renderLogsContent(w, data)
	case "server-settings":
		h.renderServerSettingsContent(w, data)
	case "server-branding":
		h.renderServerBrandingContent(w, data)
	case "server-ssl":
		h.renderServerSSLContent(w, data)
	case "server-tor":
		h.renderServerTorContent(w, data)
	case "server-web":
		h.renderServerWebContent(w, data)
	case "server-email":
		h.renderServerEmailContent(w, data)
	case "server-announcements":
		h.renderServerAnnouncementsContent(w, data)
	case "server-geoip":
		h.renderServerGeoIPContent(w, data)
	case "server-metrics":
		h.renderServerMetricsContent(w, data)
	case "scheduler":
		h.renderSchedulerContent(w, data)
	case "server-backup":
		h.renderServerBackupContent(w, data)
	case "server-maintenance":
		h.renderServerMaintenanceContent(w, data)
	case "server-updates":
		h.renderServerUpdatesContent(w, data)
	case "server-info":
		h.renderServerInfoContent(w, data)
	case "server-security":
		h.renderServerSecurityContent(w, data)
	case "help":
		h.renderHelpContent(w, data)
	case "setup":
		h.renderSetupContent(w, data)
	case "admins":
		h.renderAdminsContent(w, data)
	case "invite-accept":
		h.renderInviteAcceptContent(w, data)
	case "invite-error":
		h.renderInviteErrorContent(w, data)
	case "nodes":
		h.renderNodesContent(w, data)
	}

	// Render admin footer
	fmt.Fprintf(w, `
        </main>
    </div>
</body>
</html>`)
}

func activeClass(current, page string) string {
	if current == page {
		return "active"
	}
	return ""
}

func (h *Handler) renderDashboardContent(w http.ResponseWriter, data *AdminPageData) {
	if data.Stats == nil {
		return
	}
	s := data.Stats

	// Status indicator color
	statusColor := "green"
	statusIcon := "●"
	if s.Status == "Maintenance" {
		statusColor = "orange"
	} else if s.Status == "Error" {
		statusColor = "red"
	}

	// Top stat cards: Status, Uptime, Requests, Errors
	fmt.Fprintf(w, `
            <div class="stats-grid">
                <div class="stat-card">
                    <h3>Status</h3>
                    <div class="value %s">%s %s</div>
                </div>
                <div class="stat-card">
                    <h3>Uptime</h3>
                    <div class="value green">%s</div>
                </div>
                <div class="stat-card">
                    <h3>Requests (24h)</h3>
                    <div class="value cyan">%d</div>
                </div>
                <div class="stat-card">
                    <h3>Errors (24h)</h3>
                    <div class="value orange">%d</div>
                </div>
            </div>`,
		statusColor, statusIcon, s.Status,
		s.Uptime,
		s.Requests24h,
		s.Errors24h,
	)

	// System Resources and Quick Actions row
	fmt.Fprintf(w, `
            <div class="dashboard-grid">
                <div class="admin-section mb-0">
                    <h2>System Resources</h2>
                    <div class="mb-16">
                        <div class="flex-bar">
                            <span>CPU</span><span>%.0f%%</span>
                        </div>
                        <div class="progress-bar">
                            <div class="progress-bar-fill primary" style="width: %.0f%%;"></div>
                        </div>
                    </div>
                    <div class="mb-16">
                        <div class="flex-bar">
                            <span>Memory</span><span>%.0f%% (%s)</span>
                        </div>
                        <div class="progress-bar">
                            <div class="progress-bar-fill cyan" style="width: %.0f%%;"></div>
                        </div>
                    </div>
                    <div>
                        <div class="flex-bar">
                            <span>Disk</span><span>%.0f%%</span>
                        </div>
                        <div class="progress-bar">
                            <div class="progress-bar-fill green" style="width: %.0f%%;"></div>
                        </div>
                    </div>
                </div>
                <div class="admin-section mb-0">
                    <h2>Quick Actions</h2>
                    <div class="quick-actions">
                        <a href="/api/v1/admin/reload" class="btn">Reload Config</a>
                        <a href="/api/v1/admin/backups" class="btn">Create Backup</a>
                        <a href="/admin/logs" class="btn">View Logs</a>
                    </div>
                </div>
            </div>`,
		s.CPUPercent, s.CPUPercent,
		s.MemPercent, s.MemAlloc, s.MemPercent,
		s.DiskPercent, s.DiskPercent,
	)

	// Recent Activity and Scheduled Tasks row
	fmt.Fprintf(w, `
            <div class="dashboard-grid">
                <div class="admin-section mb-0">
                    <h2>Recent Activity</h2>
                    <table class="admin-table">`)

	if len(s.RecentActivity) == 0 {
		fmt.Fprintf(w, `<tr><td colspan="2" class="empty-message">No recent activity</td></tr>`)
	} else {
		for _, activity := range s.RecentActivity {
			fmt.Fprintf(w, `<tr><td class="td-time">%s</td><td>%s</td></tr>`, activity.Time, activity.Message)
		}
	}

	fmt.Fprintf(w, `
                    </table>
                </div>
                <div class="admin-section mb-0">
                    <h2>Scheduled Tasks</h2>
                    <table class="admin-table">`)

	if len(s.ScheduledTasks) == 0 {
		fmt.Fprintf(w, `<tr><td colspan="2" class="empty-message">No scheduled tasks</td></tr>`)
	} else {
		for _, task := range s.ScheduledTasks {
			fmt.Fprintf(w, `<tr><td>%s</td><td class="text-right text-secondary">%s</td></tr>`, task.Name, task.NextRun)
		}
	}

	fmt.Fprintf(w, `
                    </table>
                </div>
            </div>`)

	// Alerts/Warnings section (only if there are alerts)
	if len(s.Alerts) > 0 {
		fmt.Fprintf(w, `
            <div class="admin-section border-warning">
                <h2>Alerts &amp; Warnings</h2>`)
		for _, alert := range s.Alerts {
			icon := "ℹ️"
			if alert.Type == "warning" {
				icon = "⚠️"
			} else if alert.Type == "error" {
				icon = "❌"
			}
			fmt.Fprintf(w, `<div class="alert-box">%s %s</div>`, icon, alert.Message)
		}
		fmt.Fprintf(w, `</div>`)
	}

	// System Information and Features Status
	fmt.Fprintf(w, `
            <div class="d-grid grid-2 gap-20">
                <div class="admin-section mb-0">
                    <h2>System Information</h2>
                    <table class="admin-table">
                        <tr><td>Version</td><td>%s</td></tr>
                        <tr><td>Go Version</td><td>%s</td></tr>
                        <tr><td>CPUs</td><td>%d</td></tr>
                        <tr><td>Goroutines</td><td>%d</td></tr>
                        <tr><td>Server Mode</td><td>%s</td></tr>
                        <tr><td>Total Memory Allocated</td><td>%s</td></tr>
                    </table>
                </div>
                <div class="admin-section mb-0">
                    <h2>Features Status</h2>
                    <table class="admin-table">
                        <tr>
                            <td>SSL/TLS</td>
                            <td><span class="status-badge %s">%s</span></td>
                        </tr>
                        <tr>
                            <td>Tor Hidden Service</td>
                            <td><span class="status-badge %s">%s</span></td>
                        </tr>
                        <tr>
                            <td>Search Engines</td>
                            <td>%d enabled</td>
                        </tr>
                    </table>
                </div>
            </div>`,
		s.Version,
		s.GoVersion,
		s.NumCPU,
		s.NumGoroutines,
		s.ServerMode,
		s.MemTotal,
		enabledClass(s.SSLEnabled), enabledText(s.SSLEnabled),
		enabledClass(s.TorEnabled), enabledText(s.TorEnabled),
		s.EnginesEnabled,
	)
}

func (h *Handler) renderConfigContent(w http.ResponseWriter, data *AdminPageData) {
	fmt.Fprintf(w, `
            <div class="admin-section">
                <h2>Server Configuration</h2>
                <form method="POST" action="/admin/config">
                    <div class="form-row">
                        <label>Site Title</label>
                        <input type="text" name="title" value="%s">
                    </div>
                    <div class="form-row">
                        <label>Description</label>
                        <input type="text" name="description" value="%s">
                    </div>
                    <div class="form-row">
                        <label>Port</label>
                        <input type="number" name="port" value="%d">
                    </div>
                    <div class="form-row">
                        <label>Mode</label>
                        <select name="mode">
                            <option value="production" %s>Production</option>
                            <option value="development" %s>Development</option>
                        </select>
                    </div>
                    <button type="submit" class="btn">Save Changes</button>
                </form>
            </div>`,
		h.config.Server.Title,
		h.config.Server.Description,
		h.config.Server.Port,
		selectedValue(h.config.Server.Mode, "production"),
		selectedValue(h.config.Server.Mode, "development"),
	)
}

func (h *Handler) renderEnginesContent(w http.ResponseWriter, data *AdminPageData) {
	fmt.Fprintf(w, `
            <div class="admin-section">
                <h2>Search Engines</h2>
                <table class="admin-table">
                    <thead>
                        <tr>
                            <th>Engine</th>
                            <th>Status</th>
                            <th>Priority</th>
                            <th>Categories</th>
                            <th>Actions</th>
                        </tr>
                    </thead>
                    <tbody>
                        <tr>
                            <td>DuckDuckGo</td>
                            <td><span class="status-badge enabled">Enabled</span></td>
                            <td>100</td>
                            <td>General, Images</td>
                            <td><button class="btn btn-danger">Disable</button></td>
                        </tr>
                        <tr>
                            <td>Google</td>
                            <td><span class="status-badge enabled">Enabled</span></td>
                            <td>90</td>
                            <td>General, Images, Videos, News</td>
                            <td><button class="btn btn-danger">Disable</button></td>
                        </tr>
                        <tr>
                            <td>Bing</td>
                            <td><span class="status-badge enabled">Enabled</span></td>
                            <td>80</td>
                            <td>General, Images, Videos, News</td>
                            <td><button class="btn btn-danger">Disable</button></td>
                        </tr>
                    </tbody>
                </table>
            </div>`)
}

func (h *Handler) renderTokensContent(w http.ResponseWriter, data *AdminPageData) {
	// Show new token if just created
	newTokenHTML := ""
	// Note: In real implementation, pass this through query param

	fmt.Fprintf(w, `
            %s
            <div class="admin-section">
                <h2>Create New Token</h2>
                <form method="POST" action="/admin/tokens">
                    <div class="form-row">
                        <label>Name *</label>
                        <input type="text" name="name" required placeholder="My API Token">
                    </div>
                    <div class="form-row">
                        <label>Description</label>
                        <input type="text" name="description" placeholder="What this token is used for">
                    </div>
                    <button type="submit" class="btn">Create Token</button>
                </form>
            </div>

            <div class="admin-section">
                <h2>Active Tokens</h2>
                <table class="admin-table">
                    <thead>
                        <tr>
                            <th>Name</th>
                            <th>Token</th>
                            <th>Created</th>
                            <th>Expires</th>
                            <th>Last Used</th>
                            <th>Actions</th>
                        </tr>
                    </thead>
                    <tbody>`,
		newTokenHTML,
	)

	if len(data.Tokens) == 0 {
		fmt.Fprintf(w, `
                        <tr>
                            <td colspan="6" class="empty-message">No API tokens created yet</td>
                        </tr>`)
	} else {
		for _, token := range data.Tokens {
			fmt.Fprintf(w, `
                        <tr>
                            <td>%s</td>
                            <td><code>%s</code></td>
                            <td>%s</td>
                            <td>%s</td>
                            <td>%s</td>
                            <td><button class="btn btn-danger">Revoke</button></td>
                        </tr>`,
				token.Name,
				token.Token,
				token.CreatedAt.Format("Jan 2, 2006"),
				token.ExpiresAt.Format("Jan 2, 2006"),
				formatLastUsed(token.LastUsed),
			)
		}
	}

	fmt.Fprintf(w, `
                    </tbody>
                </table>
            </div>`)
}

func (h *Handler) renderLogsContent(w http.ResponseWriter, data *AdminPageData) {
	fmt.Fprintf(w, `
            <div class="admin-section">
                <h2>Server Logs</h2>
                <p class="text-secondary">Log viewing coming soon. Check server output for logs.</p>
            </div>`)
}

// Helper functions

func enabledClass(enabled bool) string {
	if enabled {
		return "enabled"
	}
	return "disabled"
}

func enabledText(enabled bool) string {
	if enabled {
		return "Enabled"
	}
	return "Disabled"
}

func selected(isSelected bool) string {
	if isSelected {
		return "selected"
	}
	return ""
}

func selectedValue(current, value string) string {
	if current == value {
		return "selected"
	}
	return ""
}

// maskPassword returns asterisks if password is set, empty otherwise
func maskPassword(pass string) string {
	if pass == "" {
		return ""
	}
	return "********"
}

func formatLastUsed(t time.Time) string {
	if t.IsZero() {
		return "Never"
	}
	return t.Format("Jan 2, 15:04")
}

func checked(b bool) string {
	if b {
		return "checked"
	}
	return ""
}

// Server Settings Pages

func (h *Handler) renderServerSettingsContent(w http.ResponseWriter, data *AdminPageData) {
	fmt.Fprintf(w, `
            <div class="admin-section">
                <h2>General Server Settings</h2>
                <form method="POST" action="/admin/server/settings">
                    <div class="form-row">
                        <label>Instance Title</label>
                        <input type="text" name="title" value="%s">
                    </div>
                    <div class="form-row">
                        <label>Description</label>
                        <textarea name="description" rows="3">%s</textarea>
                    </div>
                    <div class="form-row">
                        <label>Base URL</label>
                        <input type="text" name="base_url" value="%s" placeholder="https://example.com">
                    </div>
                    <div class="form-row">
                        <label>Port</label>
                        <input type="number" name="port" value="%d">
                        <p class="help-text">0 = random port in 64000-64999 range</p>
                    </div>
                    <div class="form-row">
                        <label>HTTPS Port (optional, for dual port mode)</label>
                        <input type="number" name="https_port" value="%d" placeholder="0 = disabled">
                    </div>
                    <div class="form-row">
                        <label>Bind Address</label>
                        <input type="text" name="address" value="%s" placeholder="[::] or 127.0.0.1">
                    </div>
                    <div class="form-row">
                        <label>Mode</label>
                        <select name="mode">
                            <option value="production" %s>Production</option>
                            <option value="development" %s>Development</option>
                        </select>
                    </div>
                    <button type="submit" class="btn">Save Changes</button>
                </form>
            </div>

            <div class="admin-section">
                <h2>Rate Limiting</h2>
                <form method="POST" action="/admin/server/settings/rate-limit">
                    <div class="form-row">
                        <label><input type="checkbox" name="enabled" %s> Enable Rate Limiting</label>
                    </div>
                    <div class="form-row">
                        <label>Requests per Minute</label>
                        <input type="number" name="requests_per_minute" value="%d">
                    </div>
                    <div class="form-row">
                        <label>Requests per Hour</label>
                        <input type="number" name="requests_per_hour" value="%d">
                    </div>
                    <div class="form-row">
                        <label>Requests per Day</label>
                        <input type="number" name="requests_per_day" value="%d">
                    </div>
                    <div class="form-row">
                        <label>Burst Size</label>
                        <input type="number" name="burst_size" value="%d">
                    </div>
                    <button type="submit" class="btn">Save Rate Limits</button>
                </form>
            </div>`,
		h.config.Server.Title,
		h.config.Server.Description,
		h.config.Server.BaseURL,
		h.config.Server.Port,
		h.config.Server.HTTPSPort,
		h.config.Server.Address,
		selectedValue(h.config.Server.Mode, "production"),
		selectedValue(h.config.Server.Mode, "development"),
		checked(h.config.Server.RateLimit.Enabled),
		h.config.Server.RateLimit.RequestsPerMinute,
		h.config.Server.RateLimit.RequestsPerHour,
		h.config.Server.RateLimit.RequestsPerDay,
		h.config.Server.RateLimit.BurstSize,
	)
}

func (h *Handler) renderServerBrandingContent(w http.ResponseWriter, data *AdminPageData) {
	fmt.Fprintf(w, `
            <div class="admin-section">
                <h2>Branding Settings</h2>
                <form method="POST" action="/admin/server/branding">
                    <div class="form-row">
                        <label>Application Name</label>
                        <input type="text" name="app_name" value="%s">
                    </div>
                    <div class="form-row">
                        <label>Logo URL</label>
                        <input type="text" name="logo_url" value="%s" placeholder="/static/img/logo.png">
                    </div>
                    <div class="form-row">
                        <label>Favicon URL</label>
                        <input type="text" name="favicon_url" value="%s" placeholder="/static/img/favicon.ico">
                    </div>
                    <div class="form-row">
                        <label>Footer Text</label>
                        <textarea name="footer_text" rows="2">%s</textarea>
                    </div>
                    <div class="form-row">
                        <label>Theme</label>
                        <select name="theme">
                            <option value="dark" %s>Dark</option>
                            <option value="light" %s>Light</option>
                            <option value="auto" %s>Auto (System Preference)</option>
                        </select>
                    </div>
                    <div class="form-row">
                        <label>Primary Color</label>
                        <input type="color" name="primary_color" value="%s">
                    </div>
                    <button type="submit" class="btn">Save Branding</button>
                </form>
            </div>`,
		h.config.Server.Branding.AppName,
		h.config.Server.Branding.LogoURL,
		h.config.Server.Branding.FaviconURL,
		h.config.Server.Branding.FooterText,
		selectedValue(h.config.Server.Branding.Theme, "dark"),
		selectedValue(h.config.Server.Branding.Theme, "light"),
		selectedValue(h.config.Server.Branding.Theme, "auto"),
		h.config.Server.Branding.PrimaryColor,
	)
}

func (h *Handler) renderServerSSLContent(w http.ResponseWriter, data *AdminPageData) {
	// Get DNS providers from Extra data
	dnsProviders, _ := data.Extra["DNSProviders"].([]tlspkg.DNSProviderInfo)
	currentProvider, _ := data.Extra["CurrentDNSProvider"].(string)
	dns01Configured, _ := data.Extra["DNS01Configured"].(bool)
	dns01ValidatedAt, _ := data.Extra["DNS01ValidatedAt"].(string)

	// Get current challenge type
	challengeType := h.config.Server.SSL.LetsEncrypt.Challenge
	if challengeType == "" {
		challengeType = "http-01"
	}

	fmt.Fprintf(w, `
            <div class="admin-section">
                <h2>SSL/TLS Configuration</h2>
                <form method="POST" action="/admin/server/ssl" id="ssl-form">
                    <div class="form-row">
                        <label class="toggle">
                            <input type="checkbox" name="enabled" %s>
                            <span class="slider"></span>
                            Enable SSL/TLS
                        </label>
                    </div>
                    <div class="form-row">
                        <label class="toggle">
                            <input type="checkbox" name="auto_tls" %s>
                            <span class="slider"></span>
                            Auto TLS (automatic certificate management)
                        </label>
                    </div>
                    <div class="form-row">
                        <label for="cert_file">Certificate File</label>
                        <input type="text" id="cert_file" name="cert_file" value="%s" placeholder="/path/to/cert.pem">
                        <span class="help-text">Path to the SSL certificate file (PEM format)</span>
                    </div>
                    <div class="form-row">
                        <label for="key_file">Key File</label>
                        <input type="text" id="key_file" name="key_file" value="%s" placeholder="/path/to/key.pem">
                        <span class="help-text">Path to the private key file (PEM format)</span>
                    </div>

                    <h3>Let's Encrypt</h3>
                    <div class="form-row">
                        <label class="toggle">
                            <input type="checkbox" name="letsencrypt_enabled" %s>
                            <span class="slider"></span>
                            Enable Let's Encrypt
                        </label>
                    </div>
                    <div class="form-row">
                        <label for="letsencrypt_email">Email Address</label>
                        <input type="email" id="letsencrypt_email" name="letsencrypt_email" value="%s" placeholder="admin@example.com">
                        <span class="help-text">Required for certificate expiration notices</span>
                    </div>
                    <div class="form-row">
                        <label for="le_domains">Domains (one per line)</label>
                        <textarea id="le_domains" name="le_domains" rows="3" placeholder="example.com&#10;www.example.com">%s</textarea>
                        <span class="help-text">Domains to request certificates for</span>
                    </div>
                    <div class="form-row">
                        <label class="toggle">
                            <input type="checkbox" name="le_staging" %s>
                            <span class="slider"></span>
                            Use Staging Server (for testing)
                        </label>
                    </div>

                    <div class="form-row">
                        <label for="letsencrypt_challenge">ACME Challenge Type</label>
                        <select id="letsencrypt_challenge" name="letsencrypt_challenge" onchange="toggleDNSProvider(this.value)">
                            <option value="http-01" %s>HTTP-01 (requires port 80)</option>
                            <option value="tls-alpn-01" %s>TLS-ALPN-01 (requires port 443)</option>
                            <option value="dns-01" %s>DNS-01 (wildcard certs, no port requirements)</option>
                        </select>
                        <span class="help-text">Select how Let's Encrypt verifies domain ownership</span>
                    </div>`,
		checked(h.config.Server.SSL.Enabled),
		checked(h.config.Server.SSL.AutoTLS),
		h.config.Server.SSL.CertFile,
		h.config.Server.SSL.KeyFile,
		checked(h.config.Server.SSL.LetsEncrypt.Enabled),
		h.config.Server.SSL.LetsEncrypt.Email,
		joinStrings(h.config.Server.SSL.LetsEncrypt.Domains),
		checked(h.config.Server.SSL.LetsEncrypt.Staging),
		selectedBool(challengeType == "http-01"),
		selectedBool(challengeType == "tls-alpn-01"),
		selectedBool(challengeType == "dns-01"),
	)

	// DNS-01 Provider Section - shown when DNS-01 is selected
	hiddenClass := ""
	if challengeType != "dns-01" {
		hiddenClass = " hidden"
	}

	fmt.Fprintf(w, `
                    <div id="dns01-config" class="dns01-section%s">
                        <h3>DNS Provider Configuration</h3>
                        <div class="form-row">
                            <label for="dns_provider">DNS Provider</label>
                            <select id="dns_provider" name="dns_provider" onchange="showProviderFields(this.value)">
                                <option value="">Select a provider...</option>`, hiddenClass)

	// Output provider options
	for _, provider := range dnsProviders {
		selectedAttr := ""
		if provider.ID == currentProvider {
			selectedAttr = " selected"
		}
		fmt.Fprintf(w, `
                                <option value="%s"%s>%s</option>`,
			provider.ID, selectedAttr, provider.Name)
	}

	fmt.Fprintf(w, `
                            </select>
                            <span class="help-text">Select your DNS provider for DNS-01 challenge</span>
                        </div>`)

	// Output provider-specific credential fields (hidden by default)
	for _, provider := range dnsProviders {
		hiddenProviderClass := " hidden"
		if provider.ID == currentProvider {
			hiddenProviderClass = ""
		}
		fmt.Fprintf(w, `
                        <div id="provider-%s" class="provider-fields%s">`, provider.ID, hiddenProviderClass)

		for _, field := range provider.Fields {
			inputType := field.Type
			if inputType == "" {
				inputType = "text"
			}
			requiredAttr := ""
			if field.Required {
				requiredAttr = " required"
			}
			fmt.Fprintf(w, `
                            <div class="form-row">
                                <label for="dns_%s">%s</label>
                                <input type="%s" id="dns_%s" name="dns_%s" placeholder="%s"%s>
                                <span class="help-text">%s</span>
                            </div>`,
				field.Name, field.Label, inputType, field.Name, field.Name,
				field.Placeholder, requiredAttr, field.Help)
		}

		fmt.Fprintf(w, `
                        </div>`)
	}

	// Show status if DNS-01 is configured
	if dns01Configured && currentProvider != "" {
		fmt.Fprintf(w, `
                        <div class="status-box status-success">
                            <span class="status-icon">&#10003;</span>
                            <span>DNS-01 configured with %s (validated: %s)</span>
                        </div>`, currentProvider, dns01ValidatedAt)
	}

	fmt.Fprintf(w, `
                    </div>

                    <button type="submit" class="btn btn-primary">Save SSL Settings</button>
                </form>
            </div>

            <script>
            function toggleDNSProvider(value) {
                var dns01Config = document.getElementById('dns01-config');
                if (value === 'dns-01') {
                    dns01Config.classList.remove('hidden');
                } else {
                    dns01Config.classList.add('hidden');
                }
            }

            function showProviderFields(provider) {
                // Hide all provider fields
                var allFields = document.querySelectorAll('.provider-fields');
                allFields.forEach(function(el) {
                    el.classList.add('hidden');
                });

                // Show selected provider fields
                if (provider) {
                    var selected = document.getElementById('provider-' + provider);
                    if (selected) {
                        selected.classList.remove('hidden');
                    }
                }
            }
            </script>`)
}

// selectedBool returns "selected" if condition is true
func selectedBool(b bool) string {
	if b {
		return "selected"
	}
	return ""
}

func (h *Handler) renderServerTorContent(w http.ResponseWriter, data *AdminPageData) {
	fmt.Fprintf(w, `
            <div class="admin-section">
                <h2>Tor Hidden Service</h2>
                <form method="POST" action="/admin/server/tor">
                    <div class="form-row">
                        <label><input type="checkbox" name="enabled" %s> Enable Tor</label>
                    </div>
                    <div class="form-row">
                        <label>Tor Binary Path</label>
                        <input type="text" name="binary" value="%s" placeholder="/usr/bin/tor">
                    </div>
                    <div class="form-row">
                        <label>SOCKS Proxy</label>
                        <input type="text" name="socks_proxy" value="%s" placeholder="127.0.0.1:9050">
                    </div>
                    <div class="form-row">
                        <label>SOCKS Port</label>
                        <input type="number" name="socks_port" value="%d">
                    </div>
                    <div class="form-row">
                        <label>Control Port</label>
                        <input type="number" name="control_port" value="%d">
                    </div>
                    <div class="form-row">
                        <label>Control Password</label>
                        <input type="password" name="control_password" value="%s" placeholder="Leave empty if not set">
                    </div>
                    <div class="form-row">
                        <label><input type="checkbox" name="stream_isolation" %s> Enable Stream Isolation</label>
                    </div>
                    <div class="form-row">
                        <label>Onion Address</label>
                        <input type="text" id="onion-address" name="onion_address" value="%s" readonly placeholder="Generated automatically">
                    </div>
                    <div class="form-row">
                        <label>Hidden Service Port</label>
                        <input type="number" name="hidden_service_port" value="%d">
                    </div>
                    <button type="submit" class="btn">Save Tor Settings</button>
                </form>
            </div>

            <!-- Service Control per AI.md PART 32 -->
            <div class="admin-section">
                <h2>Service Control</h2>
                <div class="btn-group">
                    <button type="button" class="btn btn-green" onclick="torStart()">Start Tor</button>
                    <button type="button" class="btn btn-red" onclick="torStop()">Stop Tor</button>
                    <button type="button" class="btn btn-cyan" onclick="torRestart()">Restart Tor</button>
                </div>
                <div id="tor-status" class="mt-10"></div>
            </div>

            <!-- Vanity Address Generation per AI.md PART 32 -->
            <div class="admin-section">
                <h2>Vanity Address Generation</h2>
                <p class="help-text">Generate a custom .onion address with a memorable prefix (max 6 characters for built-in generation).</p>
                <div class="form-row">
                    <label>Prefix (a-z, 2-7 only)</label>
                    <input type="text" id="vanity-prefix" maxlength="6" pattern="[a-z2-7]+" placeholder="search">
                </div>
                <div class="btn-group">
                    <button type="button" class="btn btn-cyan" onclick="vanityStart()">Start Generation</button>
                    <button type="button" class="btn btn-red" onclick="vanityCancel()">Cancel</button>
                </div>
                <div id="vanity-progress" class="mt-10" style="display:none;">
                    <p>Prefix: <span id="vanity-prefix-display"></span></p>
                    <p>Attempts: <span id="vanity-attempts">0</span></p>
                    <p>Status: <span id="vanity-status">Starting...</span></p>
                </div>
            </div>

            <!-- Key Management per AI.md PART 32 -->
            <div class="admin-section">
                <h2>Key Management</h2>
                <p class="help-text">Export or import hidden service keys. Useful for backup or using externally-generated vanity addresses.</p>
                <div class="btn-group">
                    <button type="button" class="btn" onclick="exportKeys()">Export Keys</button>
                    <button type="button" class="btn" onclick="document.getElementById('key-import-file').click()">Import Keys</button>
                    <input type="file" id="key-import-file" style="display:none" onchange="importKeys(this.files[0])">
                </div>
                <button type="button" class="btn btn-red mt-10" onclick="regenerateAddress()">Regenerate Address</button>
                <p class="help-text mt-10"><strong>Warning:</strong> Regenerating will create a new .onion address. The old address will no longer work.</p>
            </div>

            <script>
            // Tor Service Control
            function torStart() {
                fetch('/api/v1/admin/tor/start', {method: 'POST'})
                    .then(r => r.json())
                    .then(d => {
                        showToast(d.status === 'started' ? 'Tor started: ' + d.address : d.error, d.status === 'started' ? 'success' : 'error');
                        if (d.address) document.getElementById('onion-address').value = d.address;
                    })
                    .catch(e => showToast('Failed: ' + e, 'error'));
            }
            function torStop() {
                fetch('/api/v1/admin/tor/stop', {method: 'POST'})
                    .then(r => r.json())
                    .then(d => showToast(d.status === 'stopped' ? 'Tor stopped' : d.error, d.status === 'stopped' ? 'success' : 'error'))
                    .catch(e => showToast('Failed: ' + e, 'error'));
            }
            function torRestart() {
                fetch('/api/v1/admin/tor/restart', {method: 'POST'})
                    .then(r => r.json())
                    .then(d => {
                        showToast(d.status === 'restarted' ? 'Tor restarted: ' + d.address : d.error, d.status === 'restarted' ? 'success' : 'error');
                        if (d.address) document.getElementById('onion-address').value = d.address;
                    })
                    .catch(e => showToast('Failed: ' + e, 'error'));
            }

            // Vanity Generation
            var vanityPoll = null;
            function vanityStart() {
                var prefix = document.getElementById('vanity-prefix').value.toLowerCase();
                if (!prefix || !/^[a-z2-7]+$/.test(prefix)) {
                    showToast('Invalid prefix: use a-z and 2-7 only', 'error');
                    return;
                }
                fetch('/api/v1/admin/tor/vanity/start', {
                    method: 'POST',
                    headers: {'Content-Type': 'application/json'},
                    body: JSON.stringify({prefix: prefix})
                })
                .then(r => r.json())
                .then(d => {
                    if (d.status === 'started') {
                        showToast('Vanity generation started', 'success');
                        document.getElementById('vanity-progress').style.display = 'block';
                        document.getElementById('vanity-prefix-display').textContent = prefix;
                        vanityPoll = setInterval(pollVanityStatus, 2000);
                    } else {
                        showToast(d.error || 'Failed to start', 'error');
                    }
                })
                .catch(e => showToast('Failed: ' + e, 'error'));
            }
            function vanityCancel() {
                if (vanityPoll) clearInterval(vanityPoll);
                fetch('/api/v1/admin/tor/vanity/cancel', {method: 'POST'})
                    .then(() => {
                        showToast('Vanity generation cancelled', 'success');
                        document.getElementById('vanity-progress').style.display = 'none';
                    })
                    .catch(e => showToast('Failed: ' + e, 'error'));
            }
            function pollVanityStatus() {
                fetch('/api/v1/admin/tor/vanity/status')
                    .then(r => r.json())
                    .then(d => {
                        document.getElementById('vanity-attempts').textContent = d.attempts || 0;
                        if (d.found) {
                            clearInterval(vanityPoll);
                            document.getElementById('vanity-status').textContent = 'Found: ' + d.address + '.onion';
                            showToast('Vanity address found: ' + d.address + '.onion', 'success');
                        } else if (!d.running) {
                            clearInterval(vanityPoll);
                            document.getElementById('vanity-status').textContent = d.error || 'Stopped';
                        } else {
                            document.getElementById('vanity-status').textContent = 'Searching...';
                        }
                    });
            }

            // Key Management
            function exportKeys() {
                window.location.href = '/api/v1/admin/tor/keys/export';
            }
            function importKeys(file) {
                if (!file) return;
                var reader = new FileReader();
                reader.onload = function(e) {
                    var b64 = btoa(String.fromCharCode.apply(null, new Uint8Array(e.target.result)));
                    fetch('/api/v1/admin/tor/keys/import', {
                        method: 'POST',
                        headers: {'Content-Type': 'application/json'},
                        body: JSON.stringify({private_key: b64})
                    })
                    .then(r => r.json())
                    .then(d => {
                        if (d.status === 'imported') {
                            showToast('Keys imported, new address: ' + d.address, 'success');
                            document.getElementById('onion-address').value = d.address;
                        } else {
                            showToast(d.error || 'Import failed', 'error');
                        }
                    })
                    .catch(e => showToast('Failed: ' + e, 'error'));
                };
                reader.readAsArrayBuffer(file);
            }
            function regenerateAddress() {
                if (!confirm('Are you sure? This will create a new .onion address and the old one will stop working.')) return;
                fetch('/api/v1/admin/tor/address/regenerate', {method: 'POST'})
                    .then(r => r.json())
                    .then(d => {
                        if (d.status === 'regenerated') {
                            showToast('New address: ' + d.new_address, 'success');
                            document.getElementById('onion-address').value = d.new_address;
                        } else {
                            showToast(d.error || 'Failed', 'error');
                        }
                    })
                    .catch(e => showToast('Failed: ' + e, 'error'));
            }
            </script>`,
		checked(h.config.Server.Tor.Enabled),
		h.config.Server.Tor.Binary,
		h.config.Server.Tor.SocksProxy,
		h.config.Server.Tor.SocksPort,
		h.config.Server.Tor.ControlPort,
		h.config.Server.Tor.ControlPassword,
		checked(h.config.Server.Tor.StreamIsolation),
		h.config.Server.Tor.OnionAddress,
		h.config.Server.Tor.HiddenServicePort,
	)
}

func (h *Handler) renderServerWebContent(w http.ResponseWriter, data *AdminPageData) {
	fmt.Fprintf(w, `
            <div class="admin-section">
                <h2>robots.txt Configuration</h2>
                <form method="POST" action="/admin/server/web/robots">
                    <div class="form-row">
                        <label>Allow Paths (one per line)</label>
                        <textarea name="allow" rows="4" placeholder="/&#10;/api">%s</textarea>
                    </div>
                    <div class="form-row">
                        <label>Deny Paths (one per line)</label>
                        <textarea name="deny" rows="4" placeholder="/admin&#10;/private">%s</textarea>
                    </div>
                    <button type="submit" class="btn">Save robots.txt</button>
                </form>
            </div>

            <div class="admin-section">
                <h2>security.txt Configuration</h2>
                <form method="POST" action="/admin/server/web/security">
                    <div class="form-row">
                        <label>Contact</label>
                        <input type="text" name="contact" value="%s" placeholder="mailto:security@example.com">
                    </div>
                    <div class="form-row">
                        <label>Expires</label>
                        <input type="date" name="expires" value="%s">
                    </div>
                    <button type="submit" class="btn">Save security.txt</button>
                </form>
            </div>

            <div class="admin-section">
                <h2>Cookie Consent</h2>
                <form method="POST" action="/admin/server/web/cookies">
                    <div class="form-row">
                        <label><input type="checkbox" name="enabled" %s> Enable Cookie Consent Popup</label>
                    </div>
                    <div class="form-row">
                        <label>Message</label>
                        <textarea name="message" rows="2">%s</textarea>
                    </div>
                    <div class="form-row">
                        <label>Privacy Policy URL</label>
                        <input type="text" name="policy_url" value="%s" placeholder="/server/privacy">
                    </div>
                    <button type="submit" class="btn">Save Cookie Settings</button>
                </form>
            </div>

            <div class="admin-section">
                <h2>CORS Settings</h2>
                <form method="POST" action="/admin/server/web/cors">
                    <div class="form-row">
                        <label>Allowed Origins</label>
                        <input type="text" name="cors" value="%s" placeholder="* or comma-separated origins">
                    </div>
                    <button type="submit" class="btn">Save CORS Settings</button>
                </form>
            </div>`,
		joinStrings(h.config.Server.Web.Robots.Allow),
		joinStrings(h.config.Server.Web.Robots.Deny),
		h.config.Server.Web.Security.Contact,
		formatDateForInput(h.config.Server.Web.Security.Expires),
		checked(h.config.Server.Web.CookieConsent.Enabled),
		h.config.Server.Web.CookieConsent.Message,
		h.config.Server.Web.CookieConsent.PolicyURL,
		h.config.Server.Web.CORS,
	)
}

// renderServerEmailContent renders the email/SMTP settings page
// Per AI.md PART 18: Nested SMTP and From blocks with TLS mode dropdown
func (h *Handler) renderServerEmailContent(w http.ResponseWriter, data *AdminPageData) {
	// Per AI.md PART 18: TLS mode selection
	tlsMode := h.config.Server.Email.SMTP.TLS
	if tlsMode == "" {
		tlsMode = "auto"
	}

	fmt.Fprintf(w, `
            <div class="admin-section">
                <h2>Email / SMTP Configuration</h2>
                <p class="help-text">Email is automatically enabled when SMTP host is configured.</p>
                <form method="POST" action="/admin/server/email">
                    <h3>SMTP Server</h3>
                    <div class="form-row">
                        <label>SMTP Host</label>
                        <input type="text" name="smtp_host" value="%s" placeholder="smtp.example.com">
                        <small>Leave empty to auto-detect local SMTP server</small>
                    </div>
                    <div class="form-row">
                        <label>SMTP Port</label>
                        <input type="number" name="smtp_port" value="%d" placeholder="587">
                    </div>
                    <div class="form-row">
                        <label>SMTP Username</label>
                        <input type="text" name="smtp_username" value="%s" placeholder="user@example.com">
                    </div>
                    <div class="form-row">
                        <label>SMTP Password</label>
                        <input type="password" name="smtp_password" value="%s" placeholder="Leave empty to keep current">
                    </div>
                    <div class="form-row">
                        <label>TLS Mode</label>
                        <select name="smtp_tls">
                            <option value="auto" %s>Auto (try STARTTLS)</option>
                            <option value="starttls" %s>STARTTLS</option>
                            <option value="tls" %s>TLS (direct)</option>
                            <option value="none" %s>None (insecure)</option>
                        </select>
                    </div>
                    <h3>From Address</h3>
                    <div class="form-row">
                        <label>From Name</label>
                        <input type="text" name="from_name" value="%s" placeholder="Search (defaults to app name)">
                    </div>
                    <div class="form-row">
                        <label>From Email</label>
                        <input type="email" name="from_email" value="%s" placeholder="noreply@example.com (defaults to no-reply@fqdn)">
                    </div>
                    <button type="submit" class="btn">Save Email Settings</button>
                    <button type="button" class="btn ml-10 btn-cyan" onclick="testEmail()">Send Test Email</button>
                </form>
            </div>
            <script>
            function testEmail() {
                fetch('/admin/api/email/test', {method: 'POST'})
                    .then(r => r.json())
                    .then(d => showToast(d.message || 'Test email sent!', 'success'))
                    .catch(e => showToast('Failed to send test email: ' + e, 'error'));
            }
            </script>`,
		h.config.Server.Email.SMTP.Host,
		h.config.Server.Email.SMTP.Port,
		h.config.Server.Email.SMTP.Username,
		maskPassword(h.config.Server.Email.SMTP.Password),
		selected(tlsMode == "auto"),
		selected(tlsMode == "starttls"),
		selected(tlsMode == "tls"),
		selected(tlsMode == "none"),
		h.config.Server.Email.From.Name,
		h.config.Server.Email.From.Email,
	)
}

func (h *Handler) renderServerAnnouncementsContent(w http.ResponseWriter, data *AdminPageData) {
	fmt.Fprintf(w, `
            <div class="admin-section">
                <h2>Announcements</h2>
                <form method="POST" action="/admin/server/announcements">
                    <div class="form-row">
                        <label><input type="checkbox" name="enabled" %s> Enable Announcements</label>
                    </div>
                    <button type="submit" class="btn">Save Settings</button>
                </form>
            </div>

            <div class="admin-section">
                <h2>Add New Announcement</h2>
                <form method="POST" action="/admin/server/announcements/add">
                    <div class="form-row">
                        <label>ID (unique identifier)</label>
                        <input type="text" name="id" required placeholder="announcement-1">
                    </div>
                    <div class="form-row">
                        <label>Type</label>
                        <select name="type">
                            <option value="info">Info</option>
                            <option value="warning">Warning</option>
                            <option value="error">Error</option>
                            <option value="success">Success</option>
                        </select>
                    </div>
                    <div class="form-row">
                        <label>Title</label>
                        <input type="text" name="title" required placeholder="Announcement Title">
                    </div>
                    <div class="form-row">
                        <label>Message</label>
                        <textarea name="message" rows="3" required placeholder="Announcement message..."></textarea>
                    </div>
                    <div class="form-row">
                        <label>Start Date (optional)</label>
                        <input type="datetime-local" name="start">
                    </div>
                    <div class="form-row">
                        <label>End Date (optional)</label>
                        <input type="datetime-local" name="end">
                    </div>
                    <div class="form-row">
                        <label><input type="checkbox" name="dismissible" checked> User can dismiss</label>
                    </div>
                    <button type="submit" class="btn">Add Announcement</button>
                </form>
            </div>

            <div class="admin-section">
                <h2>Active Announcements</h2>
                <table class="admin-table">
                    <thead>
                        <tr>
                            <th>ID</th>
                            <th>Type</th>
                            <th>Title</th>
                            <th>Start</th>
                            <th>End</th>
                            <th>Actions</th>
                        </tr>
                    </thead>
                    <tbody>`,
		checked(h.config.Server.Web.Announcements.Enabled),
	)

	if len(h.config.Server.Web.Announcements.Messages) == 0 {
		fmt.Fprintf(w, `
                        <tr>
                            <td colspan="6" class="empty-message">No announcements configured</td>
                        </tr>`)
	} else {
		for _, a := range h.config.Server.Web.Announcements.Messages {
			fmt.Fprintf(w, `
                        <tr>
                            <td>%s</td>
                            <td><span class="status-badge %s">%s</span></td>
                            <td>%s</td>
                            <td>%s</td>
                            <td>%s</td>
                            <td>
                                <form method="POST" action="/admin/server/announcements/delete" class="form-inline">
                                    <input type="hidden" name="id" value="%s">
                                    <button type="submit" class="btn btn-danger btn-sm">Delete</button>
                                </form>
                            </td>
                        </tr>`,
				a.ID,
				announcementTypeClass(a.Type),
				a.Type,
				a.Title,
				formatAnnouncementDate(a.Start),
				formatAnnouncementDate(a.End),
				a.ID,
			)
		}
	}

	fmt.Fprintf(w, `
                    </tbody>
                </table>
            </div>`)
}

func (h *Handler) renderServerGeoIPContent(w http.ResponseWriter, data *AdminPageData) {
	fmt.Fprintf(w, `
            <div class="admin-section">
                <h2>GeoIP Configuration</h2>
                <p class="desc-text">Uses MMDB format databases from sapics/ip-location-db (free, no API key required).</p>
                <form method="POST" action="/admin/server/geoip">
                    <div class="form-row">
                        <label><input type="checkbox" name="enabled" %s> Enable GeoIP</label>
                    </div>
                    <div class="form-row">
                        <label>Database Directory</label>
                        <input type="text" name="dir" value="%s" placeholder="/data/geoip">
                    </div>
                    <div class="form-row">
                        <label>Update Frequency</label>
                        <select name="update">
                            <option value="never" %s>Never</option>
                            <option value="daily" %s>Daily</option>
                            <option value="weekly" %s>Weekly</option>
                            <option value="monthly" %s>Monthly</option>
                        </select>
                    </div>
                    <div class="form-row">
                        <label><input type="checkbox" name="asn" %s> Enable ASN Lookups</label>
                    </div>
                    <div class="form-row">
                        <label><input type="checkbox" name="country" %s> Enable Country Lookups</label>
                    </div>
                    <div class="form-row">
                        <label><input type="checkbox" name="city" %s> Enable City Lookups (larger download)</label>
                    </div>
                    <button type="submit" class="btn">Save GeoIP Settings</button>
                </form>
            </div>

            <div class="admin-section">
                <h2>Country Restrictions</h2>
                <form method="POST" action="/admin/server/geoip/restrictions">
                    <div class="form-row">
                        <label>Deny Countries (ISO 3166-1 alpha-2, comma-separated)</label>
                        <input type="text" name="deny_countries" value="%s" placeholder="CN, RU, KP">
                    </div>
                    <div class="form-row">
                        <label>Allow Only Countries (leave empty for no restriction)</label>
                        <input type="text" name="allowed_countries" value="%s" placeholder="US, CA, GB">
                    </div>
                    <button type="submit" class="btn">Save Restrictions</button>
                </form>
            </div>`,
		checked(h.config.Server.GeoIP.Enabled),
		h.config.Server.GeoIP.Dir,
		selectedValue(h.config.Server.GeoIP.Update, "never"),
		selectedValue(h.config.Server.GeoIP.Update, "daily"),
		selectedValue(h.config.Server.GeoIP.Update, "weekly"),
		selectedValue(h.config.Server.GeoIP.Update, "monthly"),
		checked(h.config.Server.GeoIP.ASN),
		checked(h.config.Server.GeoIP.Country),
		checked(h.config.Server.GeoIP.City),
		joinStrings(h.config.Server.GeoIP.DenyCountries),
		joinStrings(h.config.Server.GeoIP.AllowedCountries),
	)
}

func (h *Handler) renderServerMetricsContent(w http.ResponseWriter, data *AdminPageData) {
	fmt.Fprintf(w, `
            <div class="admin-section">
                <h2>Prometheus Metrics</h2>
                <p class="desc-text">Expose Prometheus-compatible metrics endpoint for monitoring.</p>
                <form method="POST" action="/admin/server/metrics">
                    <div class="form-row">
                        <label><input type="checkbox" name="enabled" %s> Enable Metrics Endpoint</label>
                    </div>
                    <div class="form-row">
                        <label>Endpoint Path</label>
                        <input type="text" name="path" value="%s" placeholder="/metrics">
                    </div>
                    <div class="form-row">
                        <label><input type="checkbox" name="include_system" %s> Include System Metrics (CPU, Memory, Disk)</label>
                    </div>
                    <div class="form-row">
                        <label>Bearer Token (empty = no authentication)</label>
                        <input type="text" name="token" value="%s" placeholder="Leave empty for no auth">
                        <p class="help-text">
                            If set, requests must include: <code>Authorization: Bearer &lt;token&gt;</code>
                        </p>
                    </div>
                    <button type="submit" class="btn">Save Metrics Settings</button>
                </form>
            </div>

            <div class="admin-section">
                <h2>Available Metrics</h2>
                <table class="admin-table">
                    <thead>
                        <tr>
                            <th>Metric</th>
                            <th>Type</th>
                            <th>Description</th>
                        </tr>
                    </thead>
                    <tbody>
                        <tr><td>search_requests_total</td><td>Counter</td><td>Total search requests</td></tr>
                        <tr><td>search_request_duration_seconds</td><td>Histogram</td><td>Request duration in seconds</td></tr>
                        <tr><td>search_engine_requests_total</td><td>Counter</td><td>Requests per search engine</td></tr>
                        <tr><td>search_engine_errors_total</td><td>Counter</td><td>Errors per search engine</td></tr>
                        <tr><td>search_results_returned</td><td>Histogram</td><td>Number of results returned</td></tr>
                        <tr><td>search_cache_hits_total</td><td>Counter</td><td>Cache hit count</td></tr>
                        <tr><td>search_cache_misses_total</td><td>Counter</td><td>Cache miss count</td></tr>
                        <tr><td>process_cpu_seconds_total</td><td>Counter</td><td>CPU time used (if system metrics enabled)</td></tr>
                        <tr><td>process_resident_memory_bytes</td><td>Gauge</td><td>Memory usage (if system metrics enabled)</td></tr>
                    </tbody>
                </table>
            </div>`,
		checked(h.config.Server.Metrics.Enabled),
		h.config.Server.Metrics.Path,
		checked(h.config.Server.Metrics.IncludeSystem),
		h.config.Server.Metrics.Token,
	)
}

// renderSchedulerContent renders the scheduler management page
func (h *Handler) renderSchedulerContent(w http.ResponseWriter, data *AdminPageData) {
	fmt.Fprintf(w, `
            <div class="admin-section">
                <h2>Scheduled Tasks</h2>
                <p class="desc-text">Manage background tasks that run on a schedule.</p>

                <table class="admin-table">
                    <thead>
                        <tr>
                            <th>Task</th>
                            <th>Schedule</th>
                            <th>Last Run</th>
                            <th>Next Run</th>
                            <th>Status</th>
                            <th>Actions</th>
                        </tr>
                    </thead>
                    <tbody>
                        <tr>
                            <td><strong>Backup</strong><br><small class="text-secondary">Create automatic backups of configuration and data</small></td>
                            <td><code>0 3 * * *</code><br><small>Daily at 3:00 AM</small></td>
                            <td>%s</td>
                            <td>%s</td>
                            <td><span class="status-badge %s">%s</span></td>
                            <td>
                                <button class="btn btn-xs" onclick="runTask('backup')">Run Now</button>
                            </td>
                        </tr>
                        <tr>
                            <td><strong>Cache Cleanup</strong><br><small class="text-secondary">Remove expired cache entries</small></td>
                            <td><code>0 */6 * * *</code><br><small>Every 6 hours</small></td>
                            <td>%s</td>
                            <td>%s</td>
                            <td><span class="status-badge %s">%s</span></td>
                            <td>
                                <button class="btn btn-xs" onclick="runTask('cache_cleanup')">Run Now</button>
                            </td>
                        </tr>
                        <tr>
                            <td><strong>Log Rotation</strong><br><small class="text-secondary">Rotate and compress old log files</small></td>
                            <td><code>0 0 * * 0</code><br><small>Weekly on Sunday</small></td>
                            <td>%s</td>
                            <td>%s</td>
                            <td><span class="status-badge %s">%s</span></td>
                            <td>
                                <button class="btn btn-xs" onclick="runTask('log_rotation')">Run Now</button>
                            </td>
                        </tr>
                        <tr>
                            <td><strong>GeoIP Update</strong><br><small class="text-secondary">Download latest GeoIP database</small></td>
                            <td><code>0 4 * * 3</code><br><small>Weekly on Wednesday</small></td>
                            <td>%s</td>
                            <td>%s</td>
                            <td><span class="status-badge %s">%s</span></td>
                            <td>
                                <button class="btn btn-xs" onclick="runTask('geoip_update')">Run Now</button>
                            </td>
                        </tr>
                        <tr>
                            <td><strong>Engine Health Check</strong><br><small class="text-secondary">Verify all search engines are responding</small></td>
                            <td><code>*/15 * * * *</code><br><small>Every 15 minutes</small></td>
                            <td>%s</td>
                            <td>%s</td>
                            <td><span class="status-badge %s">%s</span></td>
                            <td>
                                <button class="btn btn-xs" onclick="runTask('engine_health')">Run Now</button>
                            </td>
                        </tr>
                    </tbody>
                </table>
            </div>

            <div class="admin-section">
                <h2>Task History</h2>
                <div id="task-history">
                    <table class="admin-table">
                        <thead>
                            <tr>
                                <th>Time</th>
                                <th>Task</th>
                                <th>Duration</th>
                                <th>Result</th>
                            </tr>
                        </thead>
                        <tbody id="history-body">
                            <tr><td colspan="4" class="empty-message">Loading history...</td></tr>
                        </tbody>
                    </table>
                </div>
            </div>

            <script>
            async function runTask(taskName) {
                const confirmed = await showConfirm('Run task "' + taskName + '" now?', {
                    title: 'Run Scheduled Task',
                    confirmText: 'Run Now',
                    cancelText: 'Cancel'
                });
                if (!confirmed) return;
                fetch('/api/v1/admin/scheduler', {
                    method: 'POST',
                    headers: {'Content-Type': 'application/json'},
                    body: JSON.stringify({action: 'run', task: taskName})
                })
                .then(r => r.json())
                .then(data => {
                    if (data.success) {
                        showToast('Task started successfully', 'success');
                        location.reload();
                    } else {
                        showToast('Error: ' + (data.error || 'Unknown error'), 'error');
                    }
                })
                .catch(err => showToast('Error: ' + err, 'error'));
            }

            // Load task history
            document.addEventListener('DOMContentLoaded', function() {
                fetch('/api/v1/admin/scheduler?history=true')
                .then(r => r.json())
                .then(data => {
                    const tbody = document.getElementById('history-body');
                    if (!data.history || data.history.length === 0) {
                        tbody.innerHTML = '<tr><td colspan="4" class="empty-message">No task history available</td></tr>';
                        return;
                    }
                    tbody.innerHTML = data.history.map(h =>
                        '<tr><td>' + h.time + '</td><td>' + h.task + '</td><td>' + h.duration + '</td><td><span class="status-badge ' + (h.success ? 'enabled' : 'disabled') + '">' + (h.success ? 'Success' : 'Failed') + '</span></td></tr>'
                    ).join('');
                })
                .catch(() => {
                    document.getElementById('history-body').innerHTML = '<tr><td colspan="4" class="empty-message">Failed to load history</td></tr>';
                });
            });
            </script>`,
		// Backup task
		formatTaskTime(data.SchedulerTasks["backup"].LastRun),
		formatTaskTime(data.SchedulerTasks["backup"].NextRun),
		taskStatusClass(data.SchedulerTasks["backup"].Enabled),
		taskStatusText(data.SchedulerTasks["backup"].Enabled),
		// Cache cleanup task
		formatTaskTime(data.SchedulerTasks["cache_cleanup"].LastRun),
		formatTaskTime(data.SchedulerTasks["cache_cleanup"].NextRun),
		taskStatusClass(data.SchedulerTasks["cache_cleanup"].Enabled),
		taskStatusText(data.SchedulerTasks["cache_cleanup"].Enabled),
		// Log rotation task
		formatTaskTime(data.SchedulerTasks["log_rotation"].LastRun),
		formatTaskTime(data.SchedulerTasks["log_rotation"].NextRun),
		taskStatusClass(data.SchedulerTasks["log_rotation"].Enabled),
		taskStatusText(data.SchedulerTasks["log_rotation"].Enabled),
		// GeoIP update task
		formatTaskTime(data.SchedulerTasks["geoip_update"].LastRun),
		formatTaskTime(data.SchedulerTasks["geoip_update"].NextRun),
		taskStatusClass(data.SchedulerTasks["geoip_update"].Enabled),
		taskStatusText(data.SchedulerTasks["geoip_update"].Enabled),
		// Engine health check task
		formatTaskTime(data.SchedulerTasks["engine_health"].LastRun),
		formatTaskTime(data.SchedulerTasks["engine_health"].NextRun),
		taskStatusClass(data.SchedulerTasks["engine_health"].Enabled),
		taskStatusText(data.SchedulerTasks["engine_health"].Enabled),
	)
}

// Additional helper functions

func joinStrings(s []string) string {
	result := ""
	for i, str := range s {
		if i > 0 {
			result += "\n"
		}
		result += str
	}
	return result
}

func formatDateForInput(dateStr string) string {
	if dateStr == "" {
		return ""
	}
	// Try to parse RFC3339 and convert to YYYY-MM-DD for input type="date"
	t, err := time.Parse(time.RFC3339, dateStr)
	if err != nil {
		return dateStr
	}
	return t.Format("2006-01-02")
}

func formatAnnouncementDate(dateStr string) string {
	if dateStr == "" {
		return "Always"
	}
	t, err := time.Parse(time.RFC3339, dateStr)
	if err != nil {
		return dateStr
	}
	return t.Format("Jan 2, 2006 15:04")
}

func announcementTypeClass(t string) string {
	switch t {
	case "warning":
		return "status-badge" // orange-ish
	case "error":
		return "disabled" // red
	case "success":
		return "enabled" // green
	default:
		return "" // default/info
	}
}

func formatTaskTime(t time.Time) string {
	if t.IsZero() {
		return "Never"
	}
	return t.Format("Jan 2, 15:04")
}

func taskStatusClass(enabled bool) string {
	if enabled {
		return "enabled"
	}
	return "disabled"
}

func taskStatusText(enabled bool) string {
	if enabled {
		return "Enabled"
	}
	return "Disabled"
}

// ============================================================
// Multi-Admin Templates per AI.md PART 31
// ============================================================

// renderSetupContent renders the initial setup page
func (h *Handler) renderSetupContent(w http.ResponseWriter, data *AdminPageData) {
	setupTokenRequired := false
	if data.Extra != nil {
		if v, ok := data.Extra["SetupTokenRequired"].(bool); ok {
			setupTokenRequired = v
		}
	}

	tokenField := ""
	if setupTokenRequired {
		tokenField = `
                    <div class="form-row">
                        <label>Setup Token</label>
                        <input type="text" name="setup_token" required placeholder="Enter the setup token from console">
                        <p class="help-text">
                            The setup token was displayed in the server console. Use <code>--maintenance setup</code> to regenerate.
                        </p>
                    </div>`
	}

	fmt.Fprintf(w, `
            <div class="admin-section">
                <h2>Create Admin Account</h2>
                <p class="desc-text">
                    Welcome! Create the primary administrator account to get started.
                </p>
                <form method="POST" action="/admin/setup">
                    %s
                    <div class="form-row">
                        <label>Username</label>
                        <input type="text" name="username" required pattern="[a-z][a-z0-9_-]{2,31}"
                               placeholder="admin" autocomplete="username">
                        <p class="help-text">
                            3-32 characters, lowercase letters, numbers, underscore, hyphen
                        </p>
                    </div>
                    <div class="form-row">
                        <label>Email</label>
                        <input type="email" name="email" placeholder="admin@example.com" autocomplete="email">
                    </div>
                    <div class="form-row">
                        <label>Password</label>
                        <input type="password" name="password" required minlength="8"
                               placeholder="Min. 8 characters" autocomplete="new-password">
                    </div>
                    <div class="form-row">
                        <label>Confirm Password</label>
                        <input type="password" name="confirm_password" required minlength="8"
                               placeholder="Repeat password" autocomplete="new-password">
                    </div>
                    <button type="submit" class="btn">Create Admin Account</button>
                </form>
            </div>`,
		tokenField,
	)
}

// renderAdminsContent renders the server admins management page
func (h *Handler) renderAdminsContent(w http.ResponseWriter, data *AdminPageData) {
	admins := []*Admin{}
	totalCount := 0
	isPrimary := false
	inviteURL := ""

	if data.Extra != nil {
		if v, ok := data.Extra["Admins"].([]*Admin); ok {
			admins = v
		}
		if v, ok := data.Extra["TotalCount"].(int); ok {
			totalCount = v
		}
		if v, ok := data.Extra["IsPrimary"].(bool); ok {
			isPrimary = v
		}
	}

	// Check for invite URL in query params
	if data.Success != "" {
		inviteURL = data.Success
	}

	// Show invite URL if just created
	inviteSection := ""
	if inviteURL != "" && isPrimary {
		inviteSection = fmt.Sprintf(`
            <div class="admin-section border-success">
                <h2 class="text-success">Invite Created</h2>
                <p>Share this link with the new admin:</p>
                <div class="token-display">%s</div>
                <p class="token-warning">This link expires in 7 days and can only be used once.</p>
            </div>`,
			inviteURL,
		)
	}

	// Create invite form (primary admin only)
	createInviteForm := ""
	if isPrimary {
		createInviteForm = `
            <div class="admin-section">
                <h2>Invite New Admin</h2>
                <form method="POST" action="/admin/users/admins/invite">
                    <div class="form-row">
                        <label>Suggested Username (optional)</label>
                        <input type="text" name="username" placeholder="Leave empty for invite to choose">
                    </div>
                    <button type="submit" class="btn">Create Invite Link</button>
                </form>
            </div>`
	}

	fmt.Fprintf(w, `
            %s
            %s
            <div class="admin-section">
                <h2>Server Admins (%d total)</h2>
                <table class="admin-table">
                    <thead>
                        <tr>
                            <th>Username</th>
                            <th>Email</th>
                            <th>Role</th>
                            <th>Source</th>
                            <th>2FA</th>
                            <th>Last Login</th>
                            <th>Actions</th>
                        </tr>
                    </thead>
                    <tbody>`,
		inviteSection,
		createInviteForm,
		totalCount,
	)

	if len(admins) == 0 {
		fmt.Fprintf(w, `
                        <tr>
                            <td colspan="7" class="empty-message">No admins to display</td>
                        </tr>`)
	} else {
		for _, admin := range admins {
			role := "Admin"
			if admin.IsPrimary {
				role = "Primary"
			}
			lastLogin := "Never"
			if admin.LastLoginAt != nil {
				lastLogin = admin.LastLoginAt.Format("Jan 2, 15:04")
			}
			totpStatus := "Disabled"
			totpClass := "disabled"
			if admin.TOTPEnabled {
				totpStatus = "Enabled"
				totpClass = "enabled"
			}

			// Only show delete button for non-primary admins, and only if viewer is primary
			actionButtons := ""
			if isPrimary && !admin.IsPrimary {
				actionButtons = fmt.Sprintf(`
                                <form id="delete-admin-%d" method="POST" action="/admin/users/admins/%d/delete" class="form-inline">
                                    <button type="button" class="btn btn-danger btn-sm"
                                            onclick="confirmDeleteAdmin(%d)">Delete</button>
                                </form>`,
					admin.ID, admin.ID, admin.ID,
				)
			}

			fmt.Fprintf(w, `
                        <tr>
                            <td>%s</td>
                            <td>%s</td>
                            <td><span class="status-badge %s">%s</span></td>
                            <td>%s</td>
                            <td><span class="status-badge %s">%s</span></td>
                            <td>%s</td>
                            <td>%s</td>
                        </tr>`,
				admin.Username,
				maskEmail(admin.Email),
				func() string {
					if admin.IsPrimary {
						return "enabled"
					}
					return ""
				}(),
				role,
				admin.Source,
				totpClass,
				totpStatus,
				lastLogin,
				actionButtons,
			)
		}
	}

	fmt.Fprintf(w, `
                    </tbody>
                </table>
            </div>
            <script>
            async function confirmDeleteAdmin(adminId) {
                const confirmed = await showConfirm('Delete this admin account? This action cannot be undone.', {
                    title: 'Delete Admin',
                    confirmText: 'Delete',
                    cancelText: 'Cancel',
                    danger: true
                });
                if (confirmed) {
                    document.getElementById('delete-admin-' + adminId).submit();
                }
            }
            </script>`)

	if !isPrimary {
		fmt.Fprintf(w, `
            <div class="admin-section">
                <p class="text-secondary">
                    <em>Note: For privacy, you can only view your own admin account.
                    The primary admin can see all admins.</em>
                </p>
            </div>`)
	}
}

// renderInviteAcceptContent renders the invite acceptance page
func (h *Handler) renderInviteAcceptContent(w http.ResponseWriter, data *AdminPageData) {
	suggestedUsername := ""
	if data.Extra != nil {
		if v, ok := data.Extra["SuggestedUsername"].(string); ok {
			suggestedUsername = v
		}
	}

	fmt.Fprintf(w, `
            <div class="admin-section">
                <h2>Accept Admin Invite</h2>
                <p class="desc-text">
                    You've been invited to become a server administrator. Create your account below.
                </p>
                <form method="POST">
                    <div class="form-row">
                        <label>Username</label>
                        <input type="text" name="username" required value="%s" pattern="[a-z][a-z0-9_-]{2,31}"
                               placeholder="admin" autocomplete="username">
                        <p class="help-text">
                            3-32 characters, lowercase letters, numbers, underscore, hyphen
                        </p>
                    </div>
                    <div class="form-row">
                        <label>Email</label>
                        <input type="email" name="email" placeholder="admin@example.com" autocomplete="email">
                    </div>
                    <div class="form-row">
                        <label>Password</label>
                        <input type="password" name="password" required minlength="8"
                               placeholder="Min. 8 characters" autocomplete="new-password">
                    </div>
                    <div class="form-row">
                        <label>Confirm Password</label>
                        <input type="password" name="confirm_password" required minlength="8"
                               placeholder="Repeat password" autocomplete="new-password">
                    </div>
                    <button type="submit" class="btn">Create Admin Account</button>
                </form>
            </div>`,
		suggestedUsername,
	)
}

// renderInviteErrorContent renders the invite error page
func (h *Handler) renderInviteErrorContent(w http.ResponseWriter, data *AdminPageData) {
	fmt.Fprintf(w, `
            <div class="admin-section text-center">
                <h2 class="text-danger">Invalid Invite</h2>
                <p class="text-secondary my-20">
                    %s
                </p>
                <p class="text-secondary">
                    Please contact the server administrator for a new invite link.
                </p>
                <a href="/admin/login" class="btn mt-20">Go to Login</a>
            </div>`,
		data.Error,
	)
}

// maskEmail masks an email for privacy (j***n@e***.com)
func maskEmail(email string) string {
	if email == "" {
		return ""
	}
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return "***"
	}
	username := parts[0]
	domain := parts[1]

	maskedUsername := username
	if len(username) > 2 {
		maskedUsername = string(username[0]) + strings.Repeat("*", len(username)-2) + string(username[len(username)-1])
	}

	domainParts := strings.Split(domain, ".")
	maskedDomain := domain
	if len(domainParts) >= 2 && len(domainParts[0]) > 2 {
		maskedDomain = string(domainParts[0][0]) + strings.Repeat("*", len(domainParts[0])-2) + string(domainParts[0][len(domainParts[0])-1])
		for i := 1; i < len(domainParts); i++ {
			maskedDomain += "." + domainParts[i]
		}
	}

	return maskedUsername + "@" + maskedDomain
}

// ============================================================
// Cluster/Node Management Templates per AI.md PART 24
// ============================================================

// renderNodesContent renders the cluster nodes management page
func (h *Handler) renderNodesContent(w http.ResponseWriter, data *AdminPageData) {
	nodes := []ClusterNode{}
	isPrimary := true
	nodeID := "local"
	hostname := "unknown"
	isCluster := false
	token := ""

	if data.Extra != nil {
		if v, ok := data.Extra["Nodes"].([]ClusterNode); ok {
			nodes = v
		}
		if v, ok := data.Extra["IsPrimary"].(bool); ok {
			isPrimary = v
		}
		if v, ok := data.Extra["NodeID"].(string); ok {
			nodeID = v
		}
		if v, ok := data.Extra["Hostname"].(string); ok {
			hostname = v
		}
		if v, ok := data.Extra["IsCluster"].(bool); ok {
			isCluster = v
		}
	}

	// Check for join token in query params
	if data.Success != "" && strings.Contains(data.Success, "token=") {
		parts := strings.Split(data.Success, "token=")
		if len(parts) > 1 {
			token = parts[1]
		}
	}

	// Show token if just generated
	tokenSection := ""
	if token != "" && isPrimary {
		tokenSection = fmt.Sprintf(`
            <div class="admin-section border-success">
                <h2 class="text-success">Join Token Generated</h2>
                <p>Use this token to join a new node to the cluster:</p>
                <div class="token-display">%s</div>
                <p class="token-warning">This token expires in 24 hours and can only be used once.</p>
            </div>`,
			token,
		)
	}

	// Mode info section
	modeClass := "enabled"
	modeText := "Cluster Mode"
	if !isCluster {
		modeClass = "disabled"
		modeText = "Standalone Mode"
	}

	fmt.Fprintf(w, `
            %s
            <div class="admin-section">
                <h2>Cluster Status</h2>
                <table class="admin-table max-w-400">
                    <tr>
                        <td class="text-secondary">Mode</td>
                        <td><span class="status-badge %s">%s</span></td>
                    </tr>
                    <tr>
                        <td class="text-secondary">This Node</td>
                        <td>%s</td>
                    </tr>
                    <tr>
                        <td class="text-secondary">Hostname</td>
                        <td>%s</td>
                    </tr>
                    <tr>
                        <td class="text-secondary">Role</td>
                        <td><span class="status-badge %s">%s</span></td>
                    </tr>
                </table>
            </div>`,
		tokenSection,
		modeClass, modeText,
		nodeID,
		hostname,
		func() string {
			if isPrimary {
				return "enabled"
			}
			return ""
		}(),
		func() string {
			if isPrimary {
				return "Primary"
			}
			return "Secondary"
		}(),
	)

	// Actions section (cluster mode only)
	if isCluster {
		actionsSection := ""
		if isPrimary {
			actionsSection = `
            <div class="admin-section">
                <h2>Cluster Actions</h2>
                <form method="POST" action="/admin/server/nodes/token" class="d-inline-block mr-10">
                    <button type="submit" class="btn">Generate Join Token</button>
                </form>
            </div>`
		} else {
			actionsSection = `
            <div class="admin-section">
                <h2>Node Actions</h2>
                <form id="leave-cluster-form" method="POST" action="/admin/server/nodes/leave">
                    <button type="button" class="btn btn-danger" onclick="confirmLeaveCluster()">Leave Cluster</button>
                </form>
            </div>
            <script>
            async function confirmLeaveCluster() {
                const confirmed = await showConfirm('Leave the cluster? This node will become standalone.', {
                    title: 'Leave Cluster',
                    confirmText: 'Leave',
                    cancelText: 'Cancel',
                    danger: true
                });
                if (confirmed) {
                    document.getElementById('leave-cluster-form').submit();
                }
            }
            </script>`
		}
		fmt.Fprintf(w, "%s", actionsSection)
	}

	// Nodes table
	fmt.Fprintf(w, `
            <div class="admin-section">
                <h2>Cluster Nodes (%d)</h2>
                <table class="admin-table">
                    <thead>
                        <tr>
                            <th>Hostname</th>
                            <th>Node ID</th>
                            <th>Role</th>
                            <th>Status</th>
                            <th>Last Seen</th>
                            <th>Joined</th>
                        </tr>
                    </thead>
                    <tbody>`,
		len(nodes),
	)

	if len(nodes) == 0 {
		fmt.Fprintf(w, `
                        <tr>
                            <td colspan="6" class="empty-message">No nodes found</td>
                        </tr>`)
	} else {
		for _, node := range nodes {
			role := "Secondary"
			roleClass := ""
			if node.IsPrimary {
				role = "Primary"
				roleClass = "enabled"
			}

			statusClass := "disabled"
			if node.Status == "online" {
				statusClass = "enabled"
			}

			thisNode := ""
			if node.ID == nodeID {
				thisNode = " (this node)"
			}

			fmt.Fprintf(w, `
                        <tr>
                            <td>%s%s</td>
                            <td class="td-mono">%s</td>
                            <td><span class="status-badge %s">%s</span></td>
                            <td><span class="status-badge %s">%s</span></td>
                            <td>%s</td>
                            <td>%s</td>
                        </tr>`,
				node.Hostname, thisNode,
				node.ID,
				roleClass, role,
				statusClass, node.Status,
				node.LastSeen.Format("Jan 2, 15:04"),
				node.JoinedAt.Format("Jan 2, 2006"),
			)
		}
	}

	fmt.Fprintf(w, `
                    </tbody>
                </table>
            </div>`)

	if !isCluster {
		fmt.Fprintf(w, `
            <div class="admin-section">
                <p class="text-secondary">
                    <em>Note: Cluster mode requires a remote database (PostgreSQL or MySQL).
                    In standalone mode, only this node is shown.</em>
                </p>
            </div>`)
	}
}

// renderServerBackupContent renders the backup management page content
func (h *Handler) renderServerBackupContent(w http.ResponseWriter, data *AdminPageData) {
	fmt.Fprintf(w, `
            <div class="admin-section">
                <h2>Create Backup</h2>
                <p class="desc-text mb-16">
                    Create a backup of your database, configuration, and data files.
                </p>
                <form method="POST" action="/admin/server/backup">
                    <input type="hidden" name="action" value="create">
                    <button type="submit" class="btn">Create Backup Now</button>
                </form>
            </div>

            <div class="admin-section">
                <h2>Available Backups</h2>
                <table class="admin-table">
                    <thead>
                        <tr>
                            <th>Filename</th>
                            <th>Size</th>
                            <th>Created</th>
                            <th>Actions</th>
                        </tr>
                    </thead>
                    <tbody>
                        <tr>
                            <td colspan="4" class="empty-message">No backups available</td>
                        </tr>
                    </tbody>
                </table>
            </div>

            <div class="admin-section">
                <h2>Backup Settings</h2>
                <form method="POST" action="/admin/server/backup">
                    <input type="hidden" name="action" value="settings">
                    <div class="form-row">
                        <label>Automatic Backups</label>
                        <select name="auto_backup">
                            <option value="daily">Daily at 02:00</option>
                            <option value="weekly">Weekly on Sunday</option>
                            <option value="disabled">Disabled</option>
                        </select>
                    </div>
                    <div class="form-row">
                        <label>Maximum Backups to Keep</label>
                        <input type="number" name="max_backups" value="4" min="1" max="30">
                    </div>
                    <button type="submit" class="btn">Save Settings</button>
                </form>
            </div>`)
}

// renderServerMaintenanceContent renders the maintenance mode page content
func (h *Handler) renderServerMaintenanceContent(w http.ResponseWriter, data *AdminPageData) {
	maintenanceEnabled := ""
	if data.Config.Server.MaintenanceMode {
		maintenanceEnabled = "checked"
	}

	fmt.Fprintf(w, `
            <div class="admin-section">
                <h2>Maintenance Mode</h2>
                <p class="desc-text mb-16">
                    When enabled, all users will see a maintenance page. Admins can still access the admin panel.
                </p>
                <form method="POST" action="/admin/server/maintenance">
                    <div class="form-row">
                        <label>
                            <input type="checkbox" name="enabled" %s class="mr-8">
                            Enable Maintenance Mode
                        </label>
                    </div>
                    <button type="submit" class="btn">Save Changes</button>
                </form>
            </div>

            <div class="admin-section">
                <h2>Quick Actions</h2>
                <div class="d-flex gap-10 flex-wrap">
                    <a href="/api/v1/admin/reload" class="btn">Reload Configuration</a>
                    <a href="/admin/server/backup" class="btn">Create Backup</a>
                </div>
            </div>`, maintenanceEnabled)
}

// renderServerUpdatesContent renders the updates page content
func (h *Handler) renderServerUpdatesContent(w http.ResponseWriter, data *AdminPageData) {
	fmt.Fprintf(w, `
            <div class="admin-section">
                <h2>Current Version</h2>
                <table class="admin-table">
                    <tr><td>Version</td><td>%s</td></tr>
                    <tr><td>Commit</td><td>%s</td></tr>
                    <tr><td>Build Date</td><td>%s</td></tr>
                </table>
            </div>

            <div class="admin-section">
                <h2>Check for Updates</h2>
                <p class="desc-text mb-16">
                    Check if a newer version is available.
                </p>
                <div id="update-status" class="status-box">
                    Click the button below to check for updates.
                </div>
                <button class="btn" onclick="checkUpdates()">Check for Updates</button>
            </div>

            <script>
            function checkUpdates() {
                document.getElementById('update-status').innerHTML = 'Checking for updates...';
                fetch('/api/v1/admin/update/check', {
                    method: 'GET',
                    headers: {'Authorization': 'Bearer ' + document.cookie}
                })
                .then(r => r.json())
                .then(data => {
                    if (data.update_available) {
                        document.getElementById('update-status').innerHTML =
                            'Update available: ' + data.latest_version +
                            '<br><a href="/admin/server/updates?action=update" class="btn mt-10">Update Now</a>';
                    } else {
                        document.getElementById('update-status').innerHTML = 'You are running the latest version.';
                    }
                })
                .catch(e => {
                    document.getElementById('update-status').innerHTML = 'Error checking for updates.';
                });
            }
            </script>`,
		config.Version,
		config.CommitID,
		config.BuildDate,
	)
}

// renderServerInfoContent renders the server info page content
func (h *Handler) renderServerInfoContent(w http.ResponseWriter, data *AdminPageData) {
	if data.Stats == nil {
		fmt.Fprintf(w, `<div class="admin-section"><p>Unable to load server info.</p></div>`)
		return
	}

	s := data.Stats
	fmt.Fprintf(w, `
            <div class="admin-section">
                <h2>Application</h2>
                <table class="admin-table">
                    <tr><td>Application</td><td>Search</td></tr>
                    <tr><td>Version</td><td>%s</td></tr>
                    <tr><td>Go Version</td><td>%s</td></tr>
                    <tr><td>Uptime</td><td>%s</td></tr>
                </table>
            </div>

            <div class="admin-section">
                <h2>System</h2>
                <table class="admin-table">
                    <tr><td>CPUs</td><td>%d</td></tr>
                    <tr><td>Goroutines</td><td>%d</td></tr>
                    <tr><td>Memory Allocated</td><td>%s</td></tr>
                    <tr><td>Total Memory</td><td>%s</td></tr>
                </table>
            </div>

            <div class="admin-section">
                <h2>Paths</h2>
                <table class="admin-table">
                    <tr><td>Config Directory</td><td><code>/config</code></td></tr>
                    <tr><td>Data Directory</td><td><code>/data</code></td></tr>
                    <tr><td>Log Directory</td><td><code>/data/logs</code></td></tr>
                </table>
            </div>`,
		s.Version,
		s.GoVersion,
		s.Uptime,
		s.NumCPU,
		s.NumGoroutines,
		s.MemAlloc,
		s.MemTotal,
	)
}

// renderServerSecurityContent renders the security settings page content
func (h *Handler) renderServerSecurityContent(w http.ResponseWriter, data *AdminPageData) {
	rateLimitEnabled := ""
	if data.Config.Server.RateLimit.Enabled {
		rateLimitEnabled = "checked"
	}

	fmt.Fprintf(w, `
            <div class="admin-section">
                <h2>Rate Limiting</h2>
                <form method="POST" action="/admin/server/security">
                    <div class="form-row">
                        <label>
                            <input type="checkbox" name="rate_limit_enabled" %s class="mr-8">
                            Enable Rate Limiting
                        </label>
                    </div>
                    <div class="form-row">
                        <label>Requests per Minute</label>
                        <input type="number" name="rate_limit_rpm" value="%d" min="1" max="1000">
                    </div>
                    <div class="form-row">
                        <label>Burst Size</label>
                        <input type="number" name="rate_limit_burst" value="%d" min="1" max="100">
                    </div>
                    <button type="submit" class="btn">Save Security Settings</button>
                </form>
            </div>

            <div class="admin-section">
                <h2>Security Headers</h2>
                <table class="admin-table">
                    <tr><td>X-Frame-Options</td><td><span class="status-badge enabled">DENY</span></td></tr>
                    <tr><td>X-Content-Type-Options</td><td><span class="status-badge enabled">nosniff</span></td></tr>
                    <tr><td>X-XSS-Protection</td><td><span class="status-badge enabled">1; mode=block</span></td></tr>
                    <tr><td>Referrer-Policy</td><td><span class="status-badge enabled">strict-origin-when-cross-origin</span></td></tr>
                </table>
            </div>

            <div class="admin-section">
                <h2>Related Settings</h2>
                <ul class="text-secondary pl-20">
                    <li><a href="/admin/server/ssl">SSL/TLS Settings</a></li>
                    <li><a href="/admin/server/geoip">GeoIP Blocking</a></li>
                    <li><a href="/admin/tokens">API Tokens</a></li>
                </ul>
            </div>`,
		rateLimitEnabled,
		data.Config.Server.RateLimit.RequestsPerMinute,
		data.Config.Server.RateLimit.BurstSize,
	)
}

// renderHelpContent renders the help/documentation page content
func (h *Handler) renderHelpContent(w http.ResponseWriter, data *AdminPageData) {
	fmt.Fprintf(w, `
            <div class="admin-section">
                <h2>Documentation</h2>
                <ul class="resource-list">
                    <li>
                        <a href="https://apimgr-search.readthedocs.io" target="_blank">
                            <span class="icon">📚</span>
                            <div>
                                <strong>Official Documentation</strong>
                                <p>Complete guides and reference</p>
                            </div>
                        </a>
                    </li>
                    <li>
                        <a href="https://github.com/apimgr/search" target="_blank">
                            <span class="icon">💻</span>
                            <div>
                                <strong>GitHub Repository</strong>
                                <p>Source code and issues</p>
                            </div>
                        </a>
                    </li>
                </ul>
            </div>

            <div class="admin-section">
                <h2>Quick Links</h2>
                <div class="d-grid grid-auto gap-15">
                    <a href="/openapi" target="_blank" class="btn text-center">API Documentation</a>
                    <a href="/graphql" target="_blank" class="btn text-center">GraphQL Explorer</a>
                    <a href="/admin/logs" class="btn text-center">View Logs</a>
                </div>
            </div>

            <div class="admin-section">
                <h2>Keyboard Shortcuts</h2>
                <table class="admin-table">
                    <tr><td><kbd>g</kbd> then <kbd>d</kbd></td><td>Go to Dashboard</td></tr>
                    <tr><td><kbd>g</kbd> then <kbd>c</kbd></td><td>Go to Configuration</td></tr>
                    <tr><td><kbd>g</kbd> then <kbd>e</kbd></td><td>Go to Engines</td></tr>
                    <tr><td><kbd>g</kbd> then <kbd>l</kbd></td><td>Go to Logs</td></tr>
                    <tr><td><kbd>?</kbd></td><td>Show this help</td></tr>
                </table>
            </div>`)
}
