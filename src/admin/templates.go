package admin

import (
	"fmt"
	"net/http"
	"strings"
	"time"
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
    <style>
        .login-container {
            min-height: 100vh;
            display: flex;
            align-items: center;
            justify-content: center;
            padding: 20px;
        }
        .login-box {
            background: var(--bg-secondary);
            padding: 40px;
            border-radius: 12px;
            width: 100%%;
            max-width: 400px;
            box-shadow: 0 4px 20px rgba(0,0,0,0.3);
        }
        .login-header {
            text-align: center;
            margin-bottom: 30px;
        }
        .login-header h1 {
            color: var(--accent-primary);
            font-size: 24px;
            margin: 0 0 10px 0;
        }
        .login-header p {
            color: var(--text-secondary);
            margin: 0;
        }
        .form-group {
            margin-bottom: 20px;
        }
        .form-group label {
            display: block;
            margin-bottom: 8px;
            color: var(--text-primary);
            font-weight: 500;
        }
        .form-group input {
            width: 100%%;
            padding: 12px 16px;
            background: var(--bg-primary);
            border: 2px solid var(--border-primary);
            border-radius: 8px;
            color: var(--text-primary);
            font-size: 16px;
            transition: border-color 0.3s;
        }
        .form-group input:focus {
            outline: none;
            border-color: var(--accent-primary);
        }
        .login-btn {
            width: 100%%;
            padding: 14px;
            background: var(--accent-primary);
            color: var(--bg-primary);
            border: none;
            border-radius: 8px;
            font-size: 16px;
            font-weight: 600;
            cursor: pointer;
            transition: background 0.3s;
        }
        .login-btn:hover {
            background: var(--accent-secondary);
        }
        .error-message {
            background: rgba(255, 85, 85, 0.1);
            border: 1px solid var(--red);
            color: var(--red);
            padding: 12px;
            border-radius: 8px;
            margin-bottom: 20px;
            text-align: center;
        }
        .back-link {
            display: block;
            text-align: center;
            margin-top: 20px;
            color: var(--text-secondary);
        }
        .back-link a {
            color: var(--accent-primary);
        }
    </style>
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
    <style>
        .admin-layout {
            display: flex;
            min-height: 100vh;
        }
        .admin-sidebar {
            width: 250px;
            background: var(--bg-secondary);
            border-right: 1px solid var(--border-primary);
            padding: 20px 0;
            flex-shrink: 0;
        }
        .admin-logo {
            padding: 0 20px 20px;
            border-bottom: 1px solid var(--border-primary);
            margin-bottom: 20px;
        }
        .admin-logo a {
            color: var(--accent-primary);
            text-decoration: none;
            font-size: 20px;
            font-weight: bold;
        }
        .admin-nav {
            list-style: none;
            margin: 0;
            padding: 0;
        }
        .admin-nav li {
            margin: 4px 0;
        }
        .admin-nav a {
            display: flex;
            align-items: center;
            gap: 12px;
            padding: 12px 20px;
            color: var(--text-secondary);
            text-decoration: none;
            transition: all 0.2s;
        }
        .admin-nav a:hover {
            background: var(--bg-tertiary);
            color: var(--text-primary);
        }
        .admin-nav a.active {
            background: rgba(189, 147, 249, 0.1);
            color: var(--accent-primary);
            border-right: 3px solid var(--accent-primary);
        }
        .admin-nav .nav-icon {
            font-size: 18px;
        }
        .admin-main {
            flex: 1;
            padding: 30px;
            overflow-y: auto;
        }
        .admin-header {
            display: flex;
            justify-content: space-between;
            align-items: center;
            margin-bottom: 30px;
            padding-bottom: 20px;
            border-bottom: 1px solid var(--border-primary);
        }
        .admin-header h1 {
            margin: 0;
            color: var(--text-primary);
            font-size: 28px;
        }
        .admin-user {
            display: flex;
            align-items: center;
            gap: 15px;
        }
        .admin-user span {
            color: var(--text-secondary);
        }
        .logout-btn {
            padding: 8px 16px;
            background: transparent;
            border: 1px solid var(--red);
            color: var(--red);
            border-radius: 6px;
            cursor: pointer;
            text-decoration: none;
            font-size: 14px;
        }
        .logout-btn:hover {
            background: var(--red);
            color: var(--bg-primary);
        }
        .stats-grid {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
            gap: 20px;
            margin-bottom: 30px;
        }
        .stat-card {
            background: var(--bg-secondary);
            padding: 20px;
            border-radius: 12px;
            border: 1px solid var(--border-primary);
        }
        .stat-card h3 {
            color: var(--text-secondary);
            font-size: 14px;
            margin: 0 0 8px 0;
            text-transform: uppercase;
        }
        .stat-card .value {
            color: var(--text-primary);
            font-size: 28px;
            font-weight: bold;
        }
        .stat-card .value.green { color: var(--green); }
        .stat-card .value.purple { color: var(--accent-primary); }
        .stat-card .value.cyan { color: var(--cyan); }
        .stat-card .value.orange { color: var(--orange); }
        .admin-section {
            background: var(--bg-secondary);
            padding: 24px;
            border-radius: 12px;
            border: 1px solid var(--border-primary);
            margin-bottom: 24px;
        }
        .admin-section h2 {
            color: var(--text-primary);
            margin: 0 0 20px 0;
            font-size: 20px;
        }
        .admin-table {
            width: 100%%;
            border-collapse: collapse;
        }
        .admin-table th {
            text-align: left;
            padding: 12px;
            background: var(--bg-tertiary);
            color: var(--text-secondary);
            font-weight: 500;
            font-size: 14px;
        }
        .admin-table td {
            padding: 12px;
            border-bottom: 1px solid var(--border-primary);
            color: var(--text-primary);
        }
        .status-badge {
            display: inline-block;
            padding: 4px 8px;
            border-radius: 4px;
            font-size: 12px;
            font-weight: 500;
        }
        .status-badge.enabled {
            background: rgba(80, 250, 123, 0.2);
            color: var(--green);
        }
        .status-badge.disabled {
            background: rgba(255, 85, 85, 0.2);
            color: var(--red);
        }
        .btn {
            display: inline-block;
            padding: 10px 20px;
            background: var(--accent-primary);
            color: var(--bg-primary);
            border: none;
            border-radius: 6px;
            cursor: pointer;
            text-decoration: none;
            font-size: 14px;
            font-weight: 500;
        }
        .btn:hover {
            background: var(--accent-secondary);
        }
        .btn-danger {
            background: var(--red);
        }
        .btn-danger:hover {
            background: #ff3333;
        }
        .form-row {
            margin-bottom: 16px;
        }
        .form-row label {
            display: block;
            margin-bottom: 6px;
            color: var(--text-secondary);
            font-size: 14px;
        }
        .form-row input, .form-row textarea, .form-row select {
            width: 100%%;
            padding: 10px 14px;
            background: var(--bg-primary);
            border: 1px solid var(--border-primary);
            border-radius: 6px;
            color: var(--text-primary);
            font-size: 14px;
        }
        .form-row input:focus, .form-row textarea:focus {
            outline: none;
            border-color: var(--accent-primary);
        }
        .token-display {
            background: var(--bg-primary);
            padding: 16px;
            border-radius: 8px;
            font-family: monospace;
            word-break: break-all;
            margin-bottom: 16px;
            border: 2px solid var(--green);
        }
        .token-warning {
            color: var(--orange);
            font-size: 14px;
            margin-bottom: 16px;
        }
        @media (max-width: 768px) {
            .admin-layout {
                flex-direction: column;
            }
            .admin-sidebar {
                width: 100%%;
                border-right: none;
                border-bottom: 1px solid var(--border-primary);
            }
            .admin-nav {
                display: flex;
                overflow-x: auto;
                padding: 0 10px;
            }
            .admin-nav a {
                padding: 10px 15px;
            }
            .admin-nav a.active {
                border-right: none;
                border-bottom: 3px solid var(--accent-primary);
            }
        }
    </style>
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
            <div style="padding: 10px 20px; margin-top: 10px; border-top: 1px solid var(--border-primary);">
                <span style="color: var(--text-secondary); font-size: 12px; text-transform: uppercase;">Server Settings</span>
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

	fmt.Fprintf(w, `
            <div class="stats-grid">
                <div class="stat-card">
                    <h3>Uptime</h3>
                    <div class="value green">%s</div>
                </div>
                <div class="stat-card">
                    <h3>Version</h3>
                    <div class="value purple">%s</div>
                </div>
                <div class="stat-card">
                    <h3>Memory</h3>
                    <div class="value cyan">%s</div>
                </div>
                <div class="stat-card">
                    <h3>Goroutines</h3>
                    <div class="value orange">%d</div>
                </div>
            </div>

            <div class="admin-section">
                <h2>System Information</h2>
                <table class="admin-table">
                    <tr><td>Go Version</td><td>%s</td></tr>
                    <tr><td>CPUs</td><td>%d</td></tr>
                    <tr><td>Server Mode</td><td>%s</td></tr>
                    <tr><td>Total Memory Allocated</td><td>%s</td></tr>
                </table>
            </div>

            <div class="admin-section">
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
            </div>`,
		s.Uptime,
		s.Version,
		s.MemAlloc,
		s.NumGoroutines,
		s.GoVersion,
		s.NumCPU,
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
		selected(h.config.Server.Mode, "production"),
		selected(h.config.Server.Mode, "development"),
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
                            <td colspan="6" style="text-align: center; color: var(--text-secondary);">No API tokens created yet</td>
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
                <p style="color: var(--text-secondary);">Log viewing coming soon. Check server output for logs.</p>
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

func selected(current, value string) string {
	if current == value {
		return "selected"
	}
	return ""
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
                        <p style="font-size: 12px; color: var(--text-secondary); margin-top: 4px;">0 = random port in 64000-64999 range</p>
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
		selected(h.config.Server.Mode, "production"),
		selected(h.config.Server.Mode, "development"),
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
		selected(h.config.Server.Branding.Theme, "dark"),
		selected(h.config.Server.Branding.Theme, "light"),
		selected(h.config.Server.Branding.Theme, "auto"),
		h.config.Server.Branding.PrimaryColor,
	)
}

func (h *Handler) renderServerSSLContent(w http.ResponseWriter, data *AdminPageData) {
	fmt.Fprintf(w, `
            <div class="admin-section">
                <h2>SSL/TLS Configuration</h2>
                <form method="POST" action="/admin/server/ssl">
                    <div class="form-row">
                        <label><input type="checkbox" name="enabled" %s> Enable SSL/TLS</label>
                    </div>
                    <div class="form-row">
                        <label><input type="checkbox" name="auto_tls" %s> Auto TLS (automatic certificate management)</label>
                    </div>
                    <div class="form-row">
                        <label>Certificate File</label>
                        <input type="text" name="cert_file" value="%s" placeholder="/path/to/cert.pem">
                    </div>
                    <div class="form-row">
                        <label>Key File</label>
                        <input type="text" name="key_file" value="%s" placeholder="/path/to/key.pem">
                    </div>
                    <button type="submit" class="btn">Save SSL Settings</button>
                </form>
            </div>

            <div class="admin-section">
                <h2>Let's Encrypt</h2>
                <form method="POST" action="/admin/server/ssl/letsencrypt">
                    <div class="form-row">
                        <label><input type="checkbox" name="le_enabled" %s> Enable Let's Encrypt</label>
                    </div>
                    <div class="form-row">
                        <label>Email Address</label>
                        <input type="email" name="le_email" value="%s" placeholder="admin@example.com">
                    </div>
                    <div class="form-row">
                        <label>Domains (one per line)</label>
                        <textarea name="le_domains" rows="3" placeholder="example.com&#10;www.example.com">%s</textarea>
                    </div>
                    <div class="form-row">
                        <label><input type="checkbox" name="le_staging" %s> Use Staging Server (for testing)</label>
                    </div>
                    <button type="submit" class="btn">Save Let's Encrypt Settings</button>
                </form>
            </div>`,
		checked(h.config.Server.SSL.Enabled),
		checked(h.config.Server.SSL.AutoTLS),
		h.config.Server.SSL.CertFile,
		h.config.Server.SSL.KeyFile,
		checked(h.config.Server.SSL.LetsEncrypt.Enabled),
		h.config.Server.SSL.LetsEncrypt.Email,
		joinStrings(h.config.Server.SSL.LetsEncrypt.Domains),
		checked(h.config.Server.SSL.LetsEncrypt.Staging),
	)
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
                        <input type="text" name="onion_address" value="%s" readonly placeholder="Generated automatically">
                    </div>
                    <div class="form-row">
                        <label>Hidden Service Port</label>
                        <input type="number" name="hidden_service_port" value="%d">
                    </div>
                    <button type="submit" class="btn">Save Tor Settings</button>
                </form>
            </div>`,
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

func (h *Handler) renderServerEmailContent(w http.ResponseWriter, data *AdminPageData) {
	fmt.Fprintf(w, `
            <div class="admin-section">
                <h2>Email / SMTP Configuration</h2>
                <form method="POST" action="/admin/server/email">
                    <div class="form-row">
                        <label><input type="checkbox" name="enabled" %s> Enable Email</label>
                    </div>
                    <div class="form-row">
                        <label>SMTP Host</label>
                        <input type="text" name="smtp_host" value="%s" placeholder="smtp.example.com">
                    </div>
                    <div class="form-row">
                        <label>SMTP Port</label>
                        <input type="number" name="smtp_port" value="%d" placeholder="587">
                    </div>
                    <div class="form-row">
                        <label>SMTP Username</label>
                        <input type="text" name="smtp_user" value="%s" placeholder="user@example.com">
                    </div>
                    <div class="form-row">
                        <label>SMTP Password</label>
                        <input type="password" name="smtp_pass" value="%s" placeholder="Leave empty to keep current">
                    </div>
                    <div class="form-row">
                        <label>From Address</label>
                        <input type="email" name="from_address" value="%s" placeholder="noreply@example.com">
                    </div>
                    <div class="form-row">
                        <label>From Name</label>
                        <input type="text" name="from_name" value="%s" placeholder="Search">
                    </div>
                    <div class="form-row">
                        <label><input type="checkbox" name="tls" %s> Use TLS</label>
                    </div>
                    <div class="form-row">
                        <label><input type="checkbox" name="starttls" %s> Use STARTTLS</label>
                    </div>
                    <button type="submit" class="btn">Save Email Settings</button>
                    <button type="button" class="btn" style="margin-left: 10px; background: var(--cyan);" onclick="testEmail()">Send Test Email</button>
                </form>
            </div>
            <script>
            function testEmail() {
                fetch('/admin/api/email/test', {method: 'POST'})
                    .then(r => r.json())
                    .then(d => alert(d.message || 'Test email sent!'))
                    .catch(e => alert('Failed to send test email: ' + e));
            }
            </script>`,
		checked(h.config.Server.Email.Enabled),
		h.config.Server.Email.SMTPHost,
		h.config.Server.Email.SMTPPort,
		h.config.Server.Email.SMTPUser,
		h.config.Server.Email.SMTPPass,
		h.config.Server.Email.FromAddress,
		h.config.Server.Email.FromName,
		checked(h.config.Server.Email.TLS),
		checked(h.config.Server.Email.StartTLS),
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
                            <td colspan="6" style="text-align: center; color: var(--text-secondary);">No announcements configured</td>
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
                                <form method="POST" action="/admin/server/announcements/delete" style="display:inline;">
                                    <input type="hidden" name="id" value="%s">
                                    <button type="submit" class="btn btn-danger" style="padding: 4px 8px; font-size: 12px;">Delete</button>
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
                <p style="color: var(--text-secondary); margin-bottom: 20px;">Uses MMDB format databases from sapics/ip-location-db (free, no API key required).</p>
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
		selected(h.config.Server.GeoIP.Update, "never"),
		selected(h.config.Server.GeoIP.Update, "daily"),
		selected(h.config.Server.GeoIP.Update, "weekly"),
		selected(h.config.Server.GeoIP.Update, "monthly"),
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
                <p style="color: var(--text-secondary); margin-bottom: 20px;">Expose Prometheus-compatible metrics endpoint for monitoring.</p>
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
                        <p style="font-size: 12px; color: var(--text-secondary); margin-top: 4px;">
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
                <p style="color: var(--text-secondary); margin-bottom: 20px;">Manage background tasks that run on a schedule.</p>

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
                            <td><strong>Backup</strong><br><small style="color: var(--text-secondary);">Create automatic backups of configuration and data</small></td>
                            <td><code>0 3 * * *</code><br><small>Daily at 3:00 AM</small></td>
                            <td>%s</td>
                            <td>%s</td>
                            <td><span class="status-badge %s">%s</span></td>
                            <td>
                                <button class="btn" style="padding: 5px 10px; font-size: 12px;" onclick="runTask('backup')">Run Now</button>
                            </td>
                        </tr>
                        <tr>
                            <td><strong>Cache Cleanup</strong><br><small style="color: var(--text-secondary);">Remove expired cache entries</small></td>
                            <td><code>0 */6 * * *</code><br><small>Every 6 hours</small></td>
                            <td>%s</td>
                            <td>%s</td>
                            <td><span class="status-badge %s">%s</span></td>
                            <td>
                                <button class="btn" style="padding: 5px 10px; font-size: 12px;" onclick="runTask('cache_cleanup')">Run Now</button>
                            </td>
                        </tr>
                        <tr>
                            <td><strong>Log Rotation</strong><br><small style="color: var(--text-secondary);">Rotate and compress old log files</small></td>
                            <td><code>0 0 * * 0</code><br><small>Weekly on Sunday</small></td>
                            <td>%s</td>
                            <td>%s</td>
                            <td><span class="status-badge %s">%s</span></td>
                            <td>
                                <button class="btn" style="padding: 5px 10px; font-size: 12px;" onclick="runTask('log_rotation')">Run Now</button>
                            </td>
                        </tr>
                        <tr>
                            <td><strong>GeoIP Update</strong><br><small style="color: var(--text-secondary);">Download latest GeoIP database</small></td>
                            <td><code>0 4 * * 3</code><br><small>Weekly on Wednesday</small></td>
                            <td>%s</td>
                            <td>%s</td>
                            <td><span class="status-badge %s">%s</span></td>
                            <td>
                                <button class="btn" style="padding: 5px 10px; font-size: 12px;" onclick="runTask('geoip_update')">Run Now</button>
                            </td>
                        </tr>
                        <tr>
                            <td><strong>Engine Health Check</strong><br><small style="color: var(--text-secondary);">Verify all search engines are responding</small></td>
                            <td><code>*/15 * * * *</code><br><small>Every 15 minutes</small></td>
                            <td>%s</td>
                            <td>%s</td>
                            <td><span class="status-badge %s">%s</span></td>
                            <td>
                                <button class="btn" style="padding: 5px 10px; font-size: 12px;" onclick="runTask('engine_health')">Run Now</button>
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
                            <tr><td colspan="4" style="text-align: center; color: var(--text-secondary);">Loading history...</td></tr>
                        </tbody>
                    </table>
                </div>
            </div>

            <script>
            function runTask(taskName) {
                if (!confirm('Run task "' + taskName + '" now?')) return;
                fetch('/admin/api/v1/scheduler', {
                    method: 'POST',
                    headers: {'Content-Type': 'application/json'},
                    body: JSON.stringify({action: 'run', task: taskName})
                })
                .then(r => r.json())
                .then(data => {
                    if (data.success) {
                        alert('Task started successfully');
                        location.reload();
                    } else {
                        alert('Error: ' + (data.error || 'Unknown error'));
                    }
                })
                .catch(err => alert('Error: ' + err));
            }

            // Load task history
            document.addEventListener('DOMContentLoaded', function() {
                fetch('/admin/api/v1/scheduler?history=true')
                .then(r => r.json())
                .then(data => {
                    const tbody = document.getElementById('history-body');
                    if (!data.history || data.history.length === 0) {
                        tbody.innerHTML = '<tr><td colspan="4" style="text-align: center; color: var(--text-secondary);">No task history available</td></tr>';
                        return;
                    }
                    tbody.innerHTML = data.history.map(h =>
                        '<tr><td>' + h.time + '</td><td>' + h.task + '</td><td>' + h.duration + '</td><td><span class="status-badge ' + (h.success ? 'enabled' : 'disabled') + '">' + (h.success ? 'Success' : 'Failed') + '</span></td></tr>'
                    ).join('');
                })
                .catch(() => {
                    document.getElementById('history-body').innerHTML = '<tr><td colspan="4" style="text-align: center; color: var(--text-secondary);">Failed to load history</td></tr>';
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
// Multi-Admin Templates per TEMPLATE.md PART 31
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
                        <p style="font-size: 12px; color: var(--text-secondary); margin-top: 4px;">
                            The setup token was displayed in the server console. Use <code>--maintenance setup</code> to regenerate.
                        </p>
                    </div>`
	}

	fmt.Fprintf(w, `
            <div class="admin-section">
                <h2>Create Admin Account</h2>
                <p style="color: var(--text-secondary); margin-bottom: 20px;">
                    Welcome! Create the primary administrator account to get started.
                </p>
                <form method="POST" action="/admin/setup">
                    %s
                    <div class="form-row">
                        <label>Username</label>
                        <input type="text" name="username" required pattern="[a-z][a-z0-9_-]{2,31}"
                               placeholder="admin" autocomplete="username">
                        <p style="font-size: 12px; color: var(--text-secondary); margin-top: 4px;">
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
            <div class="admin-section" style="border-color: var(--green);">
                <h2 style="color: var(--green);">Invite Created</h2>
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
                            <td colspan="7" style="text-align: center; color: var(--text-secondary);">No admins to display</td>
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
                                <form method="POST" action="/admin/users/admins/%d/delete" style="display:inline;"
                                      onsubmit="return confirm('Delete this admin account?');">
                                    <button type="submit" class="btn btn-danger" style="padding: 4px 8px; font-size: 12px;">Delete</button>
                                </form>`,
					admin.ID,
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
            </div>`)

	if !isPrimary {
		fmt.Fprintf(w, `
            <div class="admin-section">
                <p style="color: var(--text-secondary);">
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
                <p style="color: var(--text-secondary); margin-bottom: 20px;">
                    You've been invited to become a server administrator. Create your account below.
                </p>
                <form method="POST">
                    <div class="form-row">
                        <label>Username</label>
                        <input type="text" name="username" required value="%s" pattern="[a-z][a-z0-9_-]{2,31}"
                               placeholder="admin" autocomplete="username">
                        <p style="font-size: 12px; color: var(--text-secondary); margin-top: 4px;">
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
            <div class="admin-section" style="text-align: center;">
                <h2 style="color: var(--red);">Invalid Invite</h2>
                <p style="color: var(--text-secondary); margin: 20px 0;">
                    %s
                </p>
                <p style="color: var(--text-secondary);">
                    Please contact the server administrator for a new invite link.
                </p>
                <a href="/admin/login" class="btn" style="margin-top: 20px;">Go to Login</a>
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
// Cluster/Node Management Templates per TEMPLATE.md PART 24
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
            <div class="admin-section" style="border-color: var(--green);">
                <h2 style="color: var(--green);">Join Token Generated</h2>
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
                <table class="admin-table" style="max-width: 400px;">
                    <tr>
                        <td style="color: var(--text-secondary);">Mode</td>
                        <td><span class="status-badge %s">%s</span></td>
                    </tr>
                    <tr>
                        <td style="color: var(--text-secondary);">This Node</td>
                        <td>%s</td>
                    </tr>
                    <tr>
                        <td style="color: var(--text-secondary);">Hostname</td>
                        <td>%s</td>
                    </tr>
                    <tr>
                        <td style="color: var(--text-secondary);">Role</td>
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
                <form method="POST" action="/admin/server/nodes/token" style="display: inline-block; margin-right: 10px;">
                    <button type="submit" class="btn">Generate Join Token</button>
                </form>
            </div>`
		} else {
			actionsSection = `
            <div class="admin-section">
                <h2>Node Actions</h2>
                <form method="POST" action="/admin/server/nodes/leave"
                      onsubmit="return confirm('Leave the cluster? This node will become standalone.');">
                    <button type="submit" class="btn btn-danger">Leave Cluster</button>
                </form>
            </div>`
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
                            <td colspan="6" style="text-align: center; color: var(--text-secondary);">No nodes found</td>
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
                            <td style="font-family: monospace; font-size: 12px;">%s</td>
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
                <p style="color: var(--text-secondary);">
                    <em>Note: Cluster mode requires a remote database (PostgreSQL or MySQL).
                    In standalone mode, only this node is shown.</em>
                </p>
            </div>`)
	}
}
