package admin

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/apimgr/search/src/config"
	"github.com/apimgr/search/src/i18n"
	"github.com/apimgr/search/src/ssl"
)

// renderAdminPage renders admin pages with the admin layout.
// Uses a buffered writer so the configured admin path (default: "admin")
// can be substituted for the hardcoded "/admin/" placeholders in the template.
func (h *Handler) renderAdminPage(w http.ResponseWriter, r *http.Request, page string, data *AdminPageData) {
	ap := "/" + config.GetAdminPath() // e.g. "/admin" or "/manage"
	bw := &bytes.Buffer{}
	h.renderAdminPageInner(bw, r, page, data)
	// Replace hardcoded /admin prefix with the configured admin path
	html := strings.ReplaceAll(bw.String(), `href="/admin/`, `href="`+ap+`/`)
	html = strings.ReplaceAll(html, `href="/admin"`, `href="`+ap+`"`)
	html = strings.ReplaceAll(html, `action="/admin/`, `action="`+ap+`/`)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, html)
}

func translatedAdminString(r *http.Request, key, fallback string) string {
	translated := i18n.RequestString(r, key)
	if translated == key {
		return fallback
	}

	return translated
}

func adminPageTitleKey(page string) string {
	switch page {
	case "dashboard":
		return "admin.dashboard"
	case "profile":
		return "common.profile"
	case "notifications":
		return "user.notifications"
	case "preferences":
		return "common.settings"
	case "config":
		return "admin.configuration"
	case "engines":
		return "admin.search_engines"
	case "tokens":
		return "user.tokens"
	case "logs", "audit-logs":
		return "admin.logs"
	case "admins":
		return "admin.server_admins"
	case "nodes":
		return "admin.cluster"
	case "server-settings":
		return "admin.general"
	case "server-branding":
		return "admin.branding"
	case "server-ssl":
		return "admin.ssl_tls"
	case "server-network":
		return "admin.network"
	case "server-security":
		return "admin.security"
	case "server-web":
		return "admin.web_server"
	case "server-email":
		return "admin.email"
	case "server-announcements":
		return "admin.announcements"
	case "server-metrics":
		return "admin.metrics"
	case "scheduler":
		return "admin.scheduler"
	case "help":
		return "common.help"
	default:
		return ""
	}
}

func (h *Handler) resolveAdminPageTitle(r *http.Request, page, fallback string) string {
	if key := adminPageTitleKey(page); key != "" {
		return translatedAdminString(r, key, fallback)
	}

	return fallback
}

// renderAdminPageInner writes the raw admin page HTML (with "/admin/" placeholders) to w.
func (h *Handler) renderAdminPageInner(w io.Writer, r *http.Request, page string, data *AdminPageData) {
	title := h.resolveAdminPageTitle(r, page, data.Title)
	lang := data.Lang
	dir := data.Dir
	if lang == "" || dir == "" {
		lang, dir = i18n.DetectRequestLocale(r)
	}

	adminLabel := translatedAdminString(r, "common.admin", "Admin")
	logoutLabel := translatedAdminString(r, "auth.logout", "Logout")
	dashboardLabel := translatedAdminString(r, "admin.dashboard", "Dashboard")
	configurationLabel := translatedAdminString(r, "admin.configuration", "Configuration")
	searchEnginesLabel := translatedAdminString(r, "admin.search_engines", "Search Engines")
	apiTokensLabel := translatedAdminString(r, "user.tokens", "API Tokens")
	logsLabel := translatedAdminString(r, "admin.logs", "Logs")
	serverAdminsLabel := translatedAdminString(r, "admin.server_admins", "Server Admins")
	clusterLabel := translatedAdminString(r, "admin.cluster", "Cluster")
	serverSettingsLabel := translatedAdminString(r, "admin.server_settings", "Server Settings")
	generalLabel := translatedAdminString(r, "admin.general", "General")
	brandingLabel := translatedAdminString(r, "admin.branding", "Branding")
	sslLabel := translatedAdminString(r, "admin.ssl_tls", "SSL/TLS")
	networkLabel := translatedAdminString(r, "admin.network", "Network")
	securityLabel := translatedAdminString(r, "admin.security", "Security")
	webServerLabel := translatedAdminString(r, "admin.web_server", "Web Server")
	emailLabel := translatedAdminString(r, "admin.email", "Email")
	announcementsLabel := translatedAdminString(r, "admin.announcements", "Announcements")
	metricsLabel := translatedAdminString(r, "admin.metrics", "Metrics")
	schedulerLabel := translatedAdminString(r, "admin.scheduler", "Scheduler")
	viewSiteLabel := translatedAdminString(r, "admin.view_site", "View Site")

	// Render admin header
	fmt.Fprintf(w, `<!DOCTYPE html>
<html lang="%s" dir="%s" data-theme="dark">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <meta name="robots" content="noindex, nofollow">
    <title>%s - %s - %s</title>
    <link rel="stylesheet" href="/static/css/common.css">
    <link rel="stylesheet" href="/static/css/components.css">
    <link rel="stylesheet" href="/static/css/admin.css">
</head>
<body>
    <div class="admin-layout">
        <aside class="admin-sidebar">
            <div class="admin-logo">
                <a href="/admin">🔍 %s</a>
            </div>
            <ul class="admin-nav">
                <li><a href="/admin/dashboard" class="%s"><span class="nav-icon">📊</span> %s</a></li>
                <li><a href="/admin/config" class="%s"><span class="nav-icon">⚙️</span> %s</a></li>
                <li><a href="/admin/engines" class="%s"><span class="nav-icon">🔎</span> %s</a></li>
                <li><a href="/admin/tokens" class="%s"><span class="nav-icon">🔑</span> %s</a></li>
                <li><a href="/admin/logs" class="%s"><span class="nav-icon">📜</span> %s</a></li>
                <li><a href="/admin/users/admins" class="%s"><span class="nav-icon">👥</span> %s</a></li>
                <li><a href="/admin/server/nodes" class="%s"><span class="nav-icon">🖧</span> %s</a></li>
            </ul>
            <div class="sidebar-section-header">
                <span>%s</span>
            </div>
            <ul class="admin-nav">
                <li><a href="/admin/server/settings" class="%s"><span class="nav-icon">🖥️</span> %s</a></li>
                <li><a href="/admin/server/branding" class="%s"><span class="nav-icon">🎨</span> %s</a></li>
                <li><a href="/admin/server/ssl" class="%s"><span class="nav-icon">🔒</span> %s</a></li>
                <li><a href="/admin/server/network" class="%s"><span class="nav-icon">🌐</span> %s</a></li>
                <li><a href="/admin/server/security" class="%s"><span class="nav-icon">🛡️</span> %s</a></li>
                <li><a href="/admin/server/web" class="%s"><span class="nav-icon">🌍</span> %s</a></li>
                <li><a href="/admin/server/email" class="%s"><span class="nav-icon">📧</span> %s</a></li>
                <li><a href="/admin/server/announcements" class="%s"><span class="nav-icon">📢</span> %s</a></li>
                <li><a href="/admin/server/metrics" class="%s"><span class="nav-icon">📈</span> %s</a></li>
                <li><a href="/admin/scheduler" class="%s"><span class="nav-icon">⏰</span> %s</a></li>
                <li><a href="/" target="_blank"><span class="nav-icon">👁️</span> %s</a></li>
            </ul>
        </aside>
        <main class="admin-main">
            <div class="admin-header">
                <h1>%s</h1>
                <div class="admin-user">
                    <span>%s</span>
                    <a href="/admin/logout" class="logout-btn">%s</a>
                </div>
            </div>`,
		lang,
		dir,
		title,
		adminLabel,
		h.config.Server.Title,
		adminLabel,
		activeClass(page, "dashboard"),
		dashboardLabel,
		activeClass(page, "config"),
		configurationLabel,
		activeClass(page, "engines"),
		searchEnginesLabel,
		activeClass(page, "tokens"),
		apiTokensLabel,
		activeClass(page, "logs"),
		logsLabel,
		activeClass(page, "admins"),
		serverAdminsLabel,
		activeClass(page, "nodes"),
		clusterLabel,
		serverSettingsLabel,
		activeClass(page, "server-settings"),
		generalLabel,
		activeClass(page, "server-branding"),
		brandingLabel,
		activeClass(page, "server-ssl"),
		sslLabel,
		activeClass(page, "server-network"),
		networkLabel,
		activeClass(page, "server-security"),
		securityLabel,
		activeClass(page, "server-web"),
		webServerLabel,
		activeClass(page, "server-email"),
		emailLabel,
		activeClass(page, "server-announcements"),
		announcementsLabel,
		activeClass(page, "server-metrics"),
		metricsLabel,
		activeClass(page, "scheduler"),
		schedulerLabel,
		viewSiteLabel,
		title,
		adminLabel,
		logoutLabel,
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
	case "server-network":
		h.renderServerNetworkContent(w, data)
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
	case "audit-logs":
		h.renderAuditLogsContent(w, data)
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

// formatFlashMessages formats error and success flash messages for display
func formatFlashMessages(errorMsg, successMsg string) string {
	var result string
	if errorMsg != "" {
		result += fmt.Sprintf(`<div class="flash-message flash-error">%s</div>`, errorMsg)
	}
	if successMsg != "" {
		result += fmt.Sprintf(`<div class="flash-message flash-success">%s</div>`, successMsg)
	}
	return result
}

func adminDataString(data *AdminPageData, key, fallback string, args ...interface{}) string {
	if data == nil {
		if len(args) > 0 {
			return fmt.Sprintf(fallback, args...)
		}
		return fallback
	}

	manager, err := i18n.CachedDefaultManager()
	if err != nil || manager == nil {
		if len(args) > 0 {
			return fmt.Sprintf(fallback, args...)
		}
		return fallback
	}

	lang := data.Lang
	if lang == "" {
		lang = "en"
	}

	translated := manager.T(lang, key, args...)
	if translated == key {
		if len(args) > 0 {
			return fmt.Sprintf(fallback, args...)
		}
		return fallback
	}

	return translated
}

func adminDataStringRaw(data *AdminPageData, key, fallback string) string {
	if data == nil {
		return fallback
	}

	manager, err := i18n.CachedDefaultManager()
	if err != nil {
		return fallback
	}

	lang := data.Lang
	if lang == "" {
		lang = "en"
	}

	translated := manager.T(lang, key)
	if translated == key {
		return fallback
	}

	return translated
}

func localizedEnabledText(data *AdminPageData, enabled bool) string {
	if enabled {
		return adminDataString(data, "common.enabled", "Enabled")
	}

	return adminDataString(data, "common.disabled", "Disabled")
}

func dashboardStatusText(data *AdminPageData, status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "online":
		return adminDataString(data, "admin.dashboard_page.online", "Online")
	case "maintenance":
		return adminDataString(data, "admin.dashboard_page.maintenance", "Maintenance")
	case "error":
		return adminDataString(data, "admin.dashboard_page.error", "Error")
	default:
		return status
	}
}

func dashboardActivityMessage(data *AdminPageData, message string) string {
	switch message {
	case "Server started":
		return adminDataString(data, "admin.dashboard_page.server_started", "Server started")
	default:
		return message
	}
}

func (h *Handler) renderDashboardContent(w io.Writer, data *AdminPageData) {
	if data.Stats == nil {
		return
	}
	s := data.Stats

	// Status indicator color
	statusColor := "green"
	statusIcon := "●"
	if strings.EqualFold(s.Status, "Maintenance") {
		statusColor = "orange"
	} else if strings.EqualFold(s.Status, "Error") {
		statusColor = "red"
	}

	statusLabel := adminDataString(data, "health.status", "Status")
	uptimeLabel := adminDataString(data, "health.uptime", "Uptime")
	requestsLabel := adminDataString(data, "admin.dashboard_page.requests_24h", "Requests (24h)")
	errorsLabel := adminDataString(data, "admin.dashboard_page.errors_24h", "Errors (24h)")
	systemResourcesLabel := adminDataString(data, "admin.dashboard_page.system_resources", "System Resources")
	cpuLabel := adminDataString(data, "admin.dashboard_page.cpu", "CPU")
	memoryLabel := adminDataString(data, "admin.dashboard_page.memory", "Memory")
	diskLabel := adminDataString(data, "admin.dashboard_page.disk", "Disk")
	quickActionsLabel := adminDataString(data, "admin.dashboard_page.quick_actions", "Quick Actions")
	reloadConfigLabel := adminDataString(data, "admin.dashboard_page.reload_config", "Reload Config")
	createBackupLabel := adminDataString(data, "admin.dashboard_page.create_backup", "Create Backup")
	viewLogsLabel := adminDataString(data, "admin.dashboard_page.view_logs", "View Logs")
	recentActivityLabel := adminDataString(data, "admin.dashboard_page.recent_activity", "Recent Activity")
	noRecentActivityLabel := adminDataString(data, "admin.dashboard_page.no_recent_activity", "No recent activity")
	scheduledTasksLabel := adminDataString(data, "admin.dashboard_page.scheduled_tasks", "Scheduled Tasks")
	noScheduledTasksLabel := adminDataString(data, "admin.dashboard_page.no_scheduled_tasks", "No scheduled tasks")
	alertsWarningsLabel := adminDataString(data, "admin.dashboard_page.alerts_warnings", "Alerts & Warnings")
	systemInformationLabel := adminDataString(data, "admin.dashboard_page.system_information", "System Information")
	versionLabel := adminDataString(data, "health.version", "Version")
	goVersionLabel := adminDataString(data, "admin.dashboard_page.go_version", "Go Version")
	cpusLabel := adminDataString(data, "admin.dashboard_page.cpus", "CPUs")
	goroutinesLabel := adminDataString(data, "admin.dashboard_page.goroutines", "Goroutines")
	serverModeLabel := adminDataString(data, "admin.dashboard_page.server_mode", "Server Mode")
	totalMemoryAllocatedLabel := adminDataString(data, "admin.dashboard_page.total_memory_allocated", "Total Memory Allocated")
	featuresStatusLabel := adminDataString(data, "admin.dashboard_page.features_status", "Features Status")
	sslLabel := adminDataString(data, "admin.ssl_tls", "SSL/TLS")
	torHiddenServiceLabel := adminDataString(data, "admin.dashboard_page.tor_hidden_service", "Tor Hidden Service")
	searchEnginesLabel := adminDataString(data, "admin.search_engines", "Search Engines")
	statusText := dashboardStatusText(data, s.Status)
	sslEnabledText := localizedEnabledText(data, s.SSLEnabled)
	torEnabledText := localizedEnabledText(data, s.TorEnabled)
	enginesEnabledText := adminDataString(data, "admin.dashboard_page.enabled_count", "%d enabled", s.EnginesEnabled)

	// Top stat cards: Status, Uptime, Requests, Errors
	fmt.Fprintf(w, `
            <div class="stats-grid">
                <div class="stat-card">
                    <h3>%s</h3>
                    <div class="value %s">%s %s</div>
                </div>
                <div class="stat-card">
                    <h3>%s</h3>
                    <div class="value green">%s</div>
                </div>
                <div class="stat-card">
                    <h3>%s</h3>
                    <div class="value cyan">%d</div>
                </div>
                <div class="stat-card">
                    <h3>%s</h3>
                    <div class="value orange">%d</div>
                </div>
             </div>`,
		statusLabel,
		statusColor, statusIcon, statusText,
		uptimeLabel,
		s.Uptime,
		requestsLabel,
		s.Requests24h,
		errorsLabel,
		s.Errors24h,
	)

	// System Resources and Quick Actions row
	fmt.Fprintf(w, `
            <div class="dashboard-grid">
                <div class="admin-section mb-0">
                    <h2>%s</h2>
                    <div class="mb-16">
                        <div class="flex-bar">
                            <span>%s</span><span>%.0f%%</span>
                        </div>
                        <div class="progress-bar">
                            <div class="progress-bar-fill primary" style="width: %.0f%%;"></div>
                        </div>
                    </div>
                    <div class="mb-16">
                        <div class="flex-bar">
                            <span>%s</span><span>%.0f%% (%s)</span>
                        </div>
                        <div class="progress-bar">
                            <div class="progress-bar-fill cyan" style="width: %.0f%%;"></div>
                        </div>
                    </div>
                    <div>
                        <div class="flex-bar">
                            <span>%s</span><span>%.0f%%</span>
                        </div>
                        <div class="progress-bar">
                            <div class="progress-bar-fill green" style="width: %.0f%%;"></div>
                        </div>
                    </div>
                </div>
                <div class="admin-section mb-0">
                    <h2>%s</h2>
                    <div class="quick-actions">
                        <a href="/api/v1/admin/reload" class="btn">%s</a>
                        <a href="/api/v1/admin/backups" class="btn">%s</a>
                        <a href="/admin/logs" class="btn">%s</a>
                    </div>
                </div>
            </div>`,
		systemResourcesLabel,
		cpuLabel, s.CPUPercent, s.CPUPercent,
		memoryLabel, s.MemPercent, s.MemAlloc, s.MemPercent,
		diskLabel, s.DiskPercent, s.DiskPercent,
		quickActionsLabel,
		reloadConfigLabel,
		createBackupLabel,
		viewLogsLabel,
	)

	// Recent Activity and Scheduled Tasks row
	fmt.Fprintf(w, `
            <div class="dashboard-grid">
                <div class="admin-section mb-0">
                    <h2>%s</h2>
                    <table class="admin-table">`,
		recentActivityLabel,
	)

	if len(s.RecentActivity) == 0 {
		fmt.Fprintf(w, `<tr><td colspan="2" class="empty-message">%s</td></tr>`, noRecentActivityLabel)
	} else {
		for _, activity := range s.RecentActivity {
			fmt.Fprintf(w, `<tr><td class="td-time">%s</td><td>%s</td></tr>`, activity.Time, dashboardActivityMessage(data, activity.Message))
		}
	}

	fmt.Fprintf(w, `
                    </table>
                </div>
                <div class="admin-section mb-0">
                    <h2>%s</h2>
                    <table class="admin-table">`,
		scheduledTasksLabel,
	)

	if len(s.ScheduledTasks) == 0 {
		fmt.Fprintf(w, `<tr><td colspan="2" class="empty-message">%s</td></tr>`, noScheduledTasksLabel)
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
                <h2>%s</h2>`,
			alertsWarningsLabel,
		)
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
                    <h2>%s</h2>
                    <table class="admin-table">
                        <tr><td>%s</td><td>%s</td></tr>
                        <tr><td>%s</td><td>%s</td></tr>
                        <tr><td>%s</td><td>%d</td></tr>
                        <tr><td>%s</td><td>%d</td></tr>
                        <tr><td>%s</td><td>%s</td></tr>
                        <tr><td>%s</td><td>%s</td></tr>
                    </table>
                </div>
                <div class="admin-section mb-0">
                    <h2>%s</h2>
                    <table class="admin-table">
                        <tr>
                            <td>%s</td>
                            <td><span class="status-badge %s">%s</span></td>
                        </tr>
                        <tr>
                            <td>%s</td>
                            <td><span class="status-badge %s">%s</span></td>
                        </tr>
                        <tr>
                            <td>%s</td>
                            <td>%s</td>
                        </tr>
                    </table>
                </div>
            </div>`,
		systemInformationLabel,
		versionLabel, s.Version,
		goVersionLabel, s.GoVersion,
		cpusLabel, s.NumCPU,
		goroutinesLabel, s.NumGoroutines,
		serverModeLabel, s.ServerMode,
		totalMemoryAllocatedLabel, s.MemTotal,
		featuresStatusLabel,
		sslLabel, enabledClass(s.SSLEnabled), sslEnabledText,
		torHiddenServiceLabel, enabledClass(s.TorEnabled), torEnabledText,
		searchEnginesLabel, enginesEnabledText,
	)
}

func (h *Handler) renderConfigContent(w io.Writer, data *AdminPageData) {
	serverConfigurationLabel := adminDataString(data, "admin.config_page.title", "Server Configuration")
	siteTitleLabel := adminDataString(data, "admin.config_page.site_title", "Site Title")
	descriptionLabel := adminDataString(data, "admin.config_page.description", "Description")
	portLabel := adminDataString(data, "admin.config_page.port", "Port")
	modeLabel := adminDataString(data, "admin.config_page.mode", "Mode")
	productionLabel := adminDataString(data, "admin.config_page.production", "Production")
	developmentLabel := adminDataString(data, "admin.config_page.development", "Development")
	saveChangesLabel := adminDataString(data, "admin.config_page.save_changes", "Save Changes")

	fmt.Fprintf(w, `
            <div class="admin-section">
                <h2>%s</h2>
                <form method="POST" action="/admin/config">
                    <input type="hidden" name="csrf_token" value="%s">
                    <div class="form-row">
                        <label>%s</label>
                        <input type="text" name="title" value="%s">
                    </div>
                    <div class="form-row">
                        <label>%s</label>
                        <input type="text" name="description" value="%s">
                    </div>
                    <div class="form-row">
                        <label>%s</label>
                        <input type="number" name="port" value="%d">
                    </div>
                    <div class="form-row">
                        <label>%s</label>
                        <select name="mode">
                            <option value="production" %s>%s</option>
                            <option value="development" %s>%s</option>
                        </select>
                    </div>
                    <button type="submit" class="btn">%s</button>
                </form>
            </div>`,
		serverConfigurationLabel,
		data.CSRFToken,
		siteTitleLabel,
		h.config.Server.Title,
		descriptionLabel,
		h.config.Server.Description,
		portLabel,
		h.config.Server.Port,
		modeLabel,
		selectedValue(h.config.Server.Mode, "production"),
		productionLabel,
		selectedValue(h.config.Server.Mode, "development"),
		developmentLabel,
		saveChangesLabel,
	)
}

func (h *Handler) renderEnginesContent(w io.Writer, data *AdminPageData) {
	searchEnginesLabel := adminDataString(data, "admin.search_engines", "Search Engines")
	engineLabel := adminDataString(data, "admin.engines_page.engine", "Engine")
	statusLabel := adminDataString(data, "health.status", "Status")
	priorityLabel := adminDataString(data, "admin.engines_page.priority", "Priority")
	categoriesLabel := adminDataString(data, "admin.engines_page.categories", "Categories")
	actionsLabel := adminDataString(data, "admin.engines_page.actions", "Actions")
	disableLabel := adminDataString(data, "admin.engines_page.disable", "Disable")
	enabledLabel := adminDataString(data, "common.enabled", "Enabled")
	generalLabel := adminDataString(data, "search.categories.general", "General")
	imagesLabel := adminDataString(data, "search.categories.images", "Images")
	videosLabel := adminDataString(data, "search.categories.videos", "Videos")
	newsLabel := adminDataString(data, "search.categories.news", "News")

	fmt.Fprintf(w, `
            <div class="admin-section">
                <h2>%s</h2>
                <table class="admin-table">
                    <thead>
                        <tr>
                            <th>%s</th>
                            <th>%s</th>
                            <th>%s</th>
                            <th>%s</th>
                            <th>%s</th>
                        </tr>
                    </thead>
                    <tbody>
                        <tr>
                            <td>DuckDuckGo</td>
                            <td><span class="status-badge enabled">%s</span></td>
                            <td>100</td>
                            <td>%s, %s</td>
                            <td><button class="btn btn-danger">%s</button></td>
                        </tr>
                        <tr>
                            <td>Google</td>
                            <td><span class="status-badge enabled">%s</span></td>
                            <td>90</td>
                            <td>%s, %s, %s, %s</td>
                            <td><button class="btn btn-danger">%s</button></td>
                        </tr>
                        <tr>
                            <td>Bing</td>
                            <td><span class="status-badge enabled">%s</span></td>
                            <td>80</td>
                            <td>%s, %s, %s, %s</td>
                            <td><button class="btn btn-danger">%s</button></td>
                        </tr>
                    </tbody>
                    </table>
            </div>`,
		searchEnginesLabel,
		engineLabel,
		statusLabel,
		priorityLabel,
		categoriesLabel,
		actionsLabel,
		enabledLabel,
		generalLabel, imagesLabel,
		disableLabel,
		enabledLabel,
		generalLabel, imagesLabel, videosLabel, newsLabel,
		disableLabel,
		enabledLabel,
		generalLabel, imagesLabel, videosLabel, newsLabel,
		disableLabel,
	)
}

func (h *Handler) renderTokensContent(w io.Writer, data *AdminPageData) {
	// Show new token if just created
	newTokenHTML := ""
	// Note: In real implementation, pass this through query param

	createNewTokenLabel := adminDataString(data, "admin.tokens_page.create_new_token", "Create New Token")
	nameRequiredLabel := adminDataString(data, "admin.tokens_page.name_required", "Name *")
	tokenPlaceholder := adminDataString(data, "admin.tokens_page.name_placeholder", "My API Token")
	descriptionLabel := adminDataString(data, "admin.tokens_page.description", "Description")
	descriptionPlaceholder := adminDataString(data, "admin.tokens_page.description_placeholder", "What this token is used for")
	createTokenLabel := adminDataString(data, "admin.tokens_page.create_token", "Create Token")
	activeTokensLabel := adminDataString(data, "admin.tokens_page.active_tokens", "Active Tokens")
	tokenLabel := adminDataString(data, "user.tokens", "API Tokens")
	createdLabel := adminDataString(data, "admin.tokens_page.created", "Created")
	expiresLabel := adminDataString(data, "admin.tokens_page.expires", "Expires")
	lastUsedLabel := adminDataString(data, "admin.tokens_page.last_used", "Last Used")
	actionsLabel := adminDataString(data, "admin.tokens_page.actions", "Actions")
	noTokensLabel := adminDataString(data, "admin.tokens_page.no_tokens", "No API tokens created yet")
	revokeLabel := adminDataString(data, "admin.tokens_page.revoke", "Revoke")

	fmt.Fprintf(w, `
            %s
            <div class="admin-section">
                <h2>%s</h2>
                <form method="POST" action="/admin/tokens">
                    <input type="hidden" name="csrf_token" value="%s">
                    <div class="form-row">
                        <label>%s</label>
                        <input type="text" name="name" required placeholder="%s">
                    </div>
                    <div class="form-row">
                        <label>%s</label>
                        <input type="text" name="description" placeholder="%s">
                    </div>
                    <button type="submit" class="btn">%s</button>
                </form>
            </div>

            <div class="admin-section">
                <h2>%s</h2>
                <table class="admin-table">
                    <thead>
                        <tr>
                            <th>%s</th>
                            <th>%s</th>
                            <th>%s</th>
                            <th>%s</th>
                            <th>%s</th>
                            <th>%s</th>
                        </tr>
                    </thead>
                    <tbody>`,
		newTokenHTML,
		createNewTokenLabel,
		data.CSRFToken,
		nameRequiredLabel,
		tokenPlaceholder,
		descriptionLabel,
		descriptionPlaceholder,
		createTokenLabel,
		activeTokensLabel,
		adminDataString(data, "admin.tokens_page.name", "Name"),
		tokenLabel,
		createdLabel,
		expiresLabel,
		lastUsedLabel,
		actionsLabel,
	)

	if len(data.Tokens) == 0 {
		fmt.Fprintf(w, `
                        <tr>
                            <td colspan="6" class="empty-message">%s</td>
                        </tr>`, noTokensLabel)
	} else {
		for _, token := range data.Tokens {
			fmt.Fprintf(w, `
                        <tr>
                            <td>%s</td>
                            <td><code>%s</code></td>
                            <td>%s</td>
                            <td>%s</td>
                            <td>%s</td>
                            <td><button class="btn btn-danger">%s</button></td>
                        </tr>`,
				token.Name,
				token.Token,
				token.CreatedAt.Format("2006-01-02"),
				token.ExpiresAt.Format("2006-01-02"),
				formatLastUsed(data, token.LastUsed),
				revokeLabel,
			)
		}
	}

	fmt.Fprintf(w, `
                    </tbody>
                </table>
            </div>`)
}

func (h *Handler) renderLogsContent(w io.Writer, data *AdminPageData) {
	logDir := config.GetLogDir()
	serverLogsLabel := adminDataString(data, "admin.logs_page.server_logs", "Server Logs")
	logDirectoryDescription := adminDataString(data, "admin.logs_page.log_directory_description", "Server logs are written to the configured log directory.")
	logLocationLabel := adminDataString(data, "admin.logs_page.log_location", "Log Location")
	logFilesLabel := adminDataString(data, "admin.logs_page.log_files", "Log Files")
	accessLogDescription := adminDataString(data, "admin.logs_page.access_log_description", "HTTP request logs")
	errorLogDescription := adminDataString(data, "admin.logs_page.error_log_description", "Error and warning messages")
	appLogDescription := adminDataString(data, "admin.logs_page.app_log_description", "Application events")
	viewLogsLabel := adminDataString(data, "admin.logs_page.view_logs", "View Logs")
	viewLogsDescription := adminDataString(data, "admin.logs_page.view_logs_description", "Use these commands to view logs:")
	auditLogsLabel := adminDataString(data, "admin.audit_logs_page.title", "Audit Logs")
	auditLogsDescription := adminDataString(data, "admin.logs_page.audit_logs_description", "For security audit logs (login attempts, config changes, etc.), visit:")
	viewAuditLogsLabel := adminDataString(data, "admin.logs_page.view_audit_logs", "View Audit Logs")

	fmt.Fprintf(w, `
            <div class="admin-section">
                <h2>%s</h2>
                <p class="text-secondary">%s</p>

                <div class="info-card">
                    <h3>%s</h3>
                    <code class="long-string">%s</code>
                </div>

                <div class="info-card">
                    <h3>%s</h3>
                    <ul>
                        <li><strong>access.log</strong> - %s</li>
                        <li><strong>error.log</strong> - %s</li>
                        <li><strong>app.log</strong> - %s</li>
                    </ul>
                </div>

                <div class="info-card">
                    <h3>%s</h3>
                    <p>%s</p>
                    <pre>tail -f %s/access.log
tail -f %s/error.log
tail -f %s/app.log</pre>
                </div>

                <div class="info-card">
                    <h3>%s</h3>
                    <p>%s</p>
                    <a href="/admin/server/logs/audit" class="btn btn-primary">%s</a>
                </div>
            </div>`,
		serverLogsLabel,
		logDirectoryDescription,
		logLocationLabel,
		logDir,
		logFilesLabel,
		accessLogDescription,
		errorLogDescription,
		appLogDescription,
		viewLogsLabel,
		viewLogsDescription,
		logDir, logDir, logDir,
		auditLogsLabel,
		auditLogsDescription,
		viewAuditLogsLabel,
	)
}

func (h *Handler) renderAuditLogsContent(w io.Writer, data *AdminPageData) {
	titleLabel := adminDataString(data, "admin.audit_logs_page.title", "Audit Logs")
	descriptionLabel := adminDataString(data, "admin.audit_logs_page.description", "Security audit trail for administrative actions.")
	timestampLabel := adminDataString(data, "admin.audit_logs_page.timestamp", "Timestamp")
	actionLabel := adminDataString(data, "admin.audit_logs_page.action", "Action")
	resourceLabel := adminDataString(data, "admin.audit_logs_page.resource", "Resource")
	ipAddressLabel := adminDataString(data, "admin.audit_logs_page.ip_address", "IP Address")
	detailsLabel := adminDataString(data, "admin.audit_logs_page.details", "Details")

	fmt.Fprintf(w, `
            <div class="admin-section">
                <h2>%s</h2>
                <p class="text-secondary">%s</p>

                <div class="table-container">
                    <table class="data-table">
                        <thead>
                            <tr>
                                <th>%s</th>
                                <th>%s</th>
                                <th>%s</th>
                                <th>%s</th>
                                <th>%s</th>
                            </tr>
                        </thead>
                        <tbody>`,
		titleLabel,
		descriptionLabel,
		timestampLabel,
		actionLabel,
		resourceLabel,
		ipAddressLabel,
		detailsLabel,
	)

	// Get audit logs from database if available
	if h.service != nil {
		ctx := context.Background()
		logs, err := h.service.GetAuditLogs(ctx, 100, 0)
		if err == nil && len(logs) > 0 {
			for _, entry := range logs {
				details := entry.Details
				if len(details) > 50 {
					details = details[:50] + "..."
				}
				fmt.Fprintf(w, `
                            <tr>
                                <td>%s</td>
                                <td><code>%s</code></td>
                                <td>%s</td>
                                <td class="long-string">%s</td>
                                <td>%s</td>
                            </tr>`,
					entry.Timestamp.Format("2006-01-02 15:04:05"),
					template.HTMLEscapeString(entry.Action),
					template.HTMLEscapeString(entry.Resource),
					template.HTMLEscapeString(entry.IPAddress),
					template.HTMLEscapeString(details))
			}
		} else if err != nil {
			fmt.Fprintf(w, `
                            <tr>
                                <td colspan="5" class="text-center text-secondary">%s</td>
                            </tr>`, template.HTMLEscapeString(adminDataString(data, "admin.audit_logs_page.error_loading", "Error loading audit logs: %s", err.Error())))
		} else {
			fmt.Fprintf(w, `
                            <tr>
                                <td colspan="5" class="text-center text-secondary">%s</td>
                            </tr>`, adminDataString(data, "admin.audit_logs_page.no_entries", "No audit log entries found."))
		}
	} else {
		fmt.Fprintf(w, `
                            <tr>
                                <td colspan="5" class="text-center text-secondary">%s</td>
                            </tr>`, adminDataString(data, "admin.audit_logs_page.database_not_available", "Database not available."))
	}

	aboutLabel := adminDataString(data, "admin.audit_logs_page.about", "About Audit Logs")
	aboutDescription := adminDataString(data, "admin.audit_logs_page.about_description", "Audit logs track security-relevant events:")
	loginAttemptsLabel := adminDataString(data, "admin.audit_logs_page.admin_login_attempts", "Admin login attempts (successful and failed)")
	configChangesLabel := adminDataString(data, "admin.audit_logs_page.configuration_changes", "Configuration changes")
	userManagementLabel := adminDataString(data, "admin.audit_logs_page.user_management_actions", "User management actions")
	tokenEventsLabel := adminDataString(data, "admin.audit_logs_page.api_token_events", "API token creation/revocation")
	backupRestoreLabel := adminDataString(data, "admin.audit_logs_page.backup_restore_operations", "Backup and restore operations")

	fmt.Fprintf(w, `
                        </tbody>
                    </table>
                </div>

                <div class="info-card">
                    <h3>%s</h3>
                    <p>%s</p>
                    <ul>
                        <li>%s</li>
                        <li>%s</li>
                        <li>%s</li>
                        <li>%s</li>
                        <li>%s</li>
                    </ul>
                </div>
            </div>`,
		aboutLabel,
		aboutDescription,
		loginAttemptsLabel,
		configChangesLabel,
		userManagementLabel,
		tokenEventsLabel,
		backupRestoreLabel,
	)
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

func formatLastUsed(data *AdminPageData, t time.Time) string {
	if t.IsZero() {
		return adminDataString(data, "admin.tokens_page.never", "Never")
	}
	return t.Format("2006-01-02 15:04")
}

func checked(b bool) string {
	if b {
		return "checked"
	}
	return ""
}

// Server Settings Pages

func (h *Handler) renderServerSettingsContent(w io.Writer, data *AdminPageData) {
	generalSettingsLabel := adminDataString(data, "admin.settings_page.general_title", "General Server Settings")
	instanceTitleLabel := adminDataString(data, "admin.settings_page.instance_title", "Instance Title")
	descriptionLabel := adminDataString(data, "admin.settings_page.description", "Description")
	baseURLLabel := adminDataString(data, "admin.settings_page.base_url", "Base URL")
	baseURLPlaceholder := adminDataString(data, "admin.settings_page.base_url_placeholder", "https://example.com")
	portLabel := adminDataString(data, "admin.settings_page.port", "Port")
	portHelpLabel := adminDataString(data, "admin.settings_page.port_help", "0 = random port in 64000-64999 range")
	httpsPortLabel := adminDataString(data, "admin.settings_page.https_port", "HTTPS Port (optional, for dual port mode)")
	httpsPortPlaceholder := adminDataString(data, "admin.settings_page.https_port_placeholder", "0 = disabled")
	bindAddressLabel := adminDataString(data, "admin.settings_page.bind_address", "Bind Address")
	bindAddressPlaceholder := adminDataString(data, "admin.settings_page.bind_address_placeholder", "[::] or 127.0.0.1")
	modeLabel := adminDataString(data, "admin.settings_page.mode", "Mode")
	productionLabel := adminDataString(data, "admin.settings_page.mode_production", "Production")
	developmentLabel := adminDataString(data, "admin.settings_page.mode_development", "Development")
	searchAlertsLabel := adminDataString(data, "admin.settings_page.search_alerts", "Search Alerts")
	alertCreationsLabel := adminDataString(data, "admin.settings_page.alert_creations_per_ip", "Alert creations per IP per hour")
	webhookMaxRetriesLabel := adminDataString(data, "admin.settings_page.webhook_max_retries", "Webhook max retries")
	webhookRetryDelayLabel := adminDataString(data, "admin.settings_page.webhook_retry_delay", "Webhook retry delay (minutes)")
	storedResultsRetentionLabel := adminDataString(data, "admin.settings_page.stored_results_retention", "Stored results retention (days)")
	defaultAlertFrequencyLabel := adminDataString(data, "admin.settings_page.default_alert_frequency", "Default alert frequency")
	immediateLabel := adminDataString(data, "admin.settings_page.frequency_immediate", "Immediate")
	dailyLabel := adminDataString(data, "admin.settings_page.frequency_daily", "Daily")
	weeklyLabel := adminDataString(data, "admin.settings_page.frequency_weekly", "Weekly")
	enableRSSByDefaultLabel := adminDataString(data, "admin.settings_page.enable_rss_default", "Enable RSS by default")
	enableWebhookByDefaultLabel := adminDataString(data, "admin.settings_page.enable_webhook_default", "Enable webhook by default")
	saveChangesLabel := adminDataString(data, "admin.settings_page.save_changes", "Save Changes")
	rateLimitingLabel := adminDataString(data, "admin.settings_page.rate_limiting", "Rate Limiting")
	enableRateLimitingLabel := adminDataString(data, "admin.settings_page.enable_rate_limiting", "Enable Rate Limiting")
	requestsPerMinuteLabel := adminDataString(data, "admin.settings_page.requests_per_minute", "Requests per Minute")
	requestsPerHourLabel := adminDataString(data, "admin.settings_page.requests_per_hour", "Requests per Hour")
	requestsPerDayLabel := adminDataString(data, "admin.settings_page.requests_per_day", "Requests per Day")
	burstSizeLabel := adminDataString(data, "admin.settings_page.burst_size", "Burst Size")
	saveRateLimitsLabel := adminDataString(data, "admin.settings_page.save_rate_limits", "Save Rate Limits")

	fmt.Fprintf(w, `
            <div class="admin-section">
                <h2>%s</h2>
                <form method="POST" action="/admin/server/settings">
                    <input type="hidden" name="csrf_token" value="%s">
                    <div class="form-row">
                        <label>%s</label>
                        <input type="text" name="title" value="%s">
                    </div>
                    <div class="form-row">
                        <label>%s</label>
                        <textarea name="description" rows="3">%s</textarea>
                    </div>
                    <div class="form-row">
                        <label>%s</label>
                        <input type="text" name="base_url" value="%s" placeholder="%s">
                    </div>
                    <div class="form-row">
                        <label>%s</label>
                        <input type="number" name="port" value="%d">
                        <p class="help-text">%s</p>
                    </div>
                    <div class="form-row">
                        <label>%s</label>
                        <input type="number" name="https_port" value="%d" placeholder="%s">
                    </div>
                    <div class="form-row">
                        <label>%s</label>
                        <input type="text" name="address" value="%s" placeholder="%s">
                    </div>
                    <div class="form-row">
                        <label>%s</label>
                        <select name="mode">
                            <option value="production" %s>%s</option>
                            <option value="development" %s>%s</option>
                        </select>
                    </div>
                    <h3>%s</h3>
                    <div class="form-row">
                        <label>%s</label>
                        <input type="number" name="alerts_create_rate_limit" value="%d" min="1">
                    </div>
                    <div class="form-row">
                        <label>%s</label>
                        <input type="number" name="alerts_webhook_retries" value="%d" min="1">
                    </div>
                    <div class="form-row">
                        <label>%s</label>
                        <input type="number" name="alerts_webhook_retry_delay" value="%d" min="1">
                    </div>
                    <div class="form-row">
                        <label>%s</label>
                        <input type="number" name="alerts_retention_days" value="%d" min="1">
                    </div>
                    <div class="form-row">
                        <label>%s</label>
                        <select name="alerts_default_frequency">
                            <option value="immediate" %s>%s</option>
                            <option value="daily" %s>%s</option>
                            <option value="weekly" %s>%s</option>
                        </select>
                    </div>
                    <div class="form-row">
                        <label><input type="checkbox" name="alerts_default_rss" %s> %s</label>
                    </div>
                    <div class="form-row">
                        <label><input type="checkbox" name="alerts_default_webhook" %s> %s</label>
                    </div>
                    <button type="submit" class="btn">%s</button>
                </form>
            </div>

            <div class="admin-section">
                <h2>%s</h2>
                <form method="POST" action="/admin/server/settings/rate-limit">
                    <input type="hidden" name="csrf_token" value="%s">
                    <div class="form-row">
                        <label><input type="checkbox" name="enabled" %s> %s</label>
                    </div>
                    <div class="form-row">
                        <label>%s</label>
                        <input type="number" name="requests_per_minute" value="%d">
                    </div>
                    <div class="form-row">
                        <label>%s</label>
                        <input type="number" name="requests_per_hour" value="%d">
                    </div>
                    <div class="form-row">
                        <label>%s</label>
                        <input type="number" name="requests_per_day" value="%d">
                    </div>
                    <div class="form-row">
                        <label>%s</label>
                        <input type="number" name="burst_size" value="%d">
                    </div>
                    <button type="submit" class="btn">%s</button>
                </form>
            </div>`,
		generalSettingsLabel,
		data.CSRFToken,
		instanceTitleLabel,
		h.config.Server.Title,
		descriptionLabel,
		h.config.Server.Description,
		baseURLLabel,
		h.config.Server.BaseURL,
		baseURLPlaceholder,
		portLabel,
		h.config.Server.Port,
		portHelpLabel,
		httpsPortLabel,
		h.config.Server.HTTPSPort,
		httpsPortPlaceholder,
		bindAddressLabel,
		h.config.Server.Address,
		bindAddressPlaceholder,
		modeLabel,
		selectedValue(h.config.Server.Mode, "production"),
		productionLabel,
		selectedValue(h.config.Server.Mode, "development"),
		developmentLabel,
		searchAlertsLabel,
		alertCreationsLabel,
		h.config.Search.Alerts.CreateRateLimitPerHour,
		webhookMaxRetriesLabel,
		h.config.Search.Alerts.WebhookMaxRetries,
		webhookRetryDelayLabel,
		h.config.Search.Alerts.WebhookRetryDelayMinutes,
		storedResultsRetentionLabel,
		h.config.Search.Alerts.RetentionDays,
		defaultAlertFrequencyLabel,
		selectedValue(h.config.Search.Alerts.DefaultFrequency, "immediate"),
		immediateLabel,
		selectedValue(h.config.Search.Alerts.DefaultFrequency, "daily"),
		dailyLabel,
		selectedValue(h.config.Search.Alerts.DefaultFrequency, "weekly"),
		weeklyLabel,
		checked(h.config.Search.Alerts.DefaultDeliverRSS),
		enableRSSByDefaultLabel,
		checked(h.config.Search.Alerts.DefaultDeliverWebhook),
		enableWebhookByDefaultLabel,
		saveChangesLabel,
		rateLimitingLabel,
		data.CSRFToken,
		checked(h.config.Server.RateLimit.Enabled),
		enableRateLimitingLabel,
		requestsPerMinuteLabel,
		h.config.Server.RateLimit.RequestsPerMinute,
		requestsPerHourLabel,
		h.config.Server.RateLimit.RequestsPerHour,
		requestsPerDayLabel,
		h.config.Server.RateLimit.RequestsPerDay,
		burstSizeLabel,
		h.config.Server.RateLimit.BurstSize,
		saveRateLimitsLabel,
	)
}

func (h *Handler) renderServerBrandingContent(w io.Writer, data *AdminPageData) {
	brandingSettingsLabel := adminDataString(data, "admin.branding_page.title", "Branding Settings")
	applicationNameLabel := adminDataString(data, "admin.branding_page.application_name", "Application Name")
	logoURLLabel := adminDataString(data, "admin.branding_page.logo_url", "Logo URL")
	faviconURLLabel := adminDataString(data, "admin.branding_page.favicon_url", "Favicon URL")
	sourceCodeURLLabel := adminDataString(data, "admin.branding_page.source_code_url", "Source Code URL")
	footerTextLabel := adminDataString(data, "admin.branding_page.footer_text", "Footer Text")
	themeLabel := adminDataString(data, "preferences.theme", "Theme")
	primaryColorLabel := adminDataString(data, "admin.branding_page.primary_color", "Primary Color")
	saveBrandingLabel := adminDataString(data, "admin.branding_page.save_branding", "Save Branding")
	darkLabel := adminDataString(data, "preferences.theme_dark", "Dark")
	lightLabel := adminDataString(data, "preferences.theme_light", "Light")
	autoLabel := adminDataString(data, "admin.branding_page.theme_auto", "Auto (System Preference)")
	logoURLPlaceholder := adminDataString(data, "admin.branding_page.logo_url_placeholder", "/static/img/logo.png")
	faviconURLPlaceholder := adminDataString(data, "admin.branding_page.favicon_url_placeholder", "/static/img/favicon.ico")
	sourceCodeURLPlaceholder := adminDataString(data, "admin.branding_page.source_code_url_placeholder", "https://github.com/org/repo")

	fmt.Fprintf(w, `
            <div class="admin-section">
                <h2>%s</h2>
                <form method="POST" action="/admin/server/branding">
                    <input type="hidden" name="csrf_token" value="%s">
                    <div class="form-row">
                        <label>%s</label>
                        <input type="text" name="app_name" value="%s">
                    </div>
                    <div class="form-row">
                        <label>%s</label>
                        <input type="text" name="logo_url" value="%s" placeholder="%s">
                    </div>
                    <div class="form-row">
                        <label>%s</label>
                        <input type="text" name="favicon_url" value="%s" placeholder="%s">
                    </div>
                    <div class="form-row">
                        <label>%s</label>
                        <input type="url" name="source_code_url" value="%s" placeholder="%s">
                    </div>
                    <div class="form-row">
                        <label>%s</label>
                        <textarea name="footer_text" rows="2">%s</textarea>
                    </div>
                    <div class="form-row">
                        <label>%s</label>
                        <select name="theme">
                            <option value="dark" %s>%s</option>
                            <option value="light" %s>%s</option>
                            <option value="auto" %s>%s</option>
                        </select>
                    </div>
                    <div class="form-row">
                        <label>%s</label>
                        <input type="color" name="primary_color" value="%s">
                    </div>
                    <button type="submit" class="btn">%s</button>
                </form>
            </div>`,
		brandingSettingsLabel,
		data.CSRFToken,
		applicationNameLabel,
		h.config.Server.Branding.Title,
		logoURLLabel,
		h.config.Server.Branding.LogoURL,
		logoURLPlaceholder,
		faviconURLLabel,
		h.config.Server.Branding.FaviconURL,
		faviconURLPlaceholder,
		sourceCodeURLLabel,
		h.config.Server.Branding.SourceCodeURL,
		sourceCodeURLPlaceholder,
		footerTextLabel,
		h.config.Server.Branding.FooterText,
		themeLabel,
		selectedValue(h.config.Server.Branding.Theme, "dark"),
		darkLabel,
		selectedValue(h.config.Server.Branding.Theme, "light"),
		lightLabel,
		selectedValue(h.config.Server.Branding.Theme, "auto"),
		autoLabel,
		primaryColorLabel,
		h.config.Server.Branding.PrimaryColor,
		saveBrandingLabel,
	)
}

func (h *Handler) renderServerSSLContent(w io.Writer, data *AdminPageData) {
	// Get DNS providers from Extra data
	dnsProviders, _ := data.Extra["DNSProviders"].([]ssl.DNSProviderInfo)
	currentProvider, _ := data.Extra["CurrentDNSProvider"].(string)
	dns01Configured, _ := data.Extra["DNS01Configured"].(bool)
	dns01ValidatedAt, _ := data.Extra["DNS01ValidatedAt"].(string)

	// Get current challenge type
	challengeType := h.config.Server.SSL.LetsEncrypt.Challenge
	if challengeType == "" {
		challengeType = "http-01"
	}

	sslConfigurationLabel := adminDataString(data, "admin.ssl_page.title", "SSL/TLS Configuration")
	enableSSLLabel := adminDataString(data, "admin.ssl_page.enable_ssl", "Enable SSL/TLS")
	autoTLSLabel := adminDataString(data, "admin.ssl_page.auto_tls", "Auto TLS (automatic certificate management)")
	certificateFileLabel := adminDataString(data, "admin.ssl_page.certificate_file", "Certificate File")
	certificateFileHelpLabel := adminDataString(data, "admin.ssl_page.certificate_file_help", "Path to the SSL certificate file (PEM format)")
	keyFileLabel := adminDataString(data, "admin.ssl_page.key_file", "Key File")
	keyFileHelpLabel := adminDataString(data, "admin.ssl_page.key_file_help", "Path to the private key file (PEM format)")
	letsEncryptLabel := adminDataString(data, "admin.ssl_page.lets_encrypt", "Let's Encrypt")
	enableLetsEncryptLabel := adminDataString(data, "admin.ssl_page.enable_lets_encrypt", "Enable Let's Encrypt")
	emailAddressLabel := adminDataString(data, "admin.ssl_page.email_address", "Email Address")
	emailAddressHelpLabel := adminDataString(data, "admin.ssl_page.email_address_help", "Required for certificate expiration notices")
	domainsLabel := adminDataString(data, "admin.ssl_page.domains", "Domains (one per line)")
	domainsHelpLabel := adminDataString(data, "admin.ssl_page.domains_help", "Domains to request certificates for")
	useStagingLabel := adminDataString(data, "admin.ssl_page.use_staging", "Use Staging Server (for testing)")
	acmeChallengeTypeLabel := adminDataString(data, "admin.ssl_page.acme_challenge_type", "ACME Challenge Type")
	http01Label := adminDataString(data, "admin.ssl_page.http_01", "HTTP-01 (requires port 80)")
	tlsALPN01Label := adminDataString(data, "admin.ssl_page.tls_alpn_01", "TLS-ALPN-01 (requires port 443)")
	dns01Label := adminDataString(data, "admin.ssl_page.dns_01", "DNS-01 (wildcard certs, no port requirements)")
	acmeChallengeHelpLabel := adminDataString(data, "admin.ssl_page.acme_challenge_help", "Select how Let's Encrypt verifies domain ownership")
	dnsProviderConfigurationLabel := adminDataString(data, "admin.ssl_page.dns_provider_configuration", "DNS Provider Configuration")
	dnsProviderLabel := adminDataString(data, "admin.ssl_page.dns_provider", "DNS Provider")
	selectProviderLabel := adminDataString(data, "admin.ssl_page.select_provider", "Select a provider...")
	dnsProviderHelpLabel := adminDataString(data, "admin.ssl_page.dns_provider_help", "Select your DNS provider for DNS-01 challenge")
	dns01ConfiguredLabel := adminDataString(data, "admin.ssl_page.dns_01_configured", "DNS-01 configured with %s (validated: %s)", currentProvider, dns01ValidatedAt)
	saveSSLSettingsLabel := adminDataString(data, "admin.ssl_page.save_ssl_settings", "Save SSL Settings")
	certificateFilePlaceholder := adminDataString(data, "admin.ssl_page.certificate_file_placeholder", "/path/to/cert.pem")
	keyFilePlaceholder := adminDataString(data, "admin.ssl_page.key_file_placeholder", "/path/to/key.pem")
	emailAddressPlaceholder := adminDataString(data, "admin.ssl_page.email_address_placeholder", "admin@example.com")
	domainsPlaceholder := adminDataString(data, "admin.ssl_page.domains_placeholder", "example.com&#10;www.example.com")

	fmt.Fprintf(w, `
            <div class="admin-section">
                <h2>%s</h2>
                <form method="POST" action="/admin/server/ssl" id="ssl-form">
                    <input type="hidden" name="csrf_token" value="%s">
                    <div class="form-row">
                        <label class="toggle">
                            <input type="checkbox" name="enabled" %s>
                            <span class="slider"></span>
                            %s
                        </label>
                    </div>
                    <div class="form-row">
                        <label class="toggle">
                            <input type="checkbox" name="auto_tls" %s>
                            <span class="slider"></span>
                            %s
                        </label>
                    </div>
                    <div class="form-row">
                        <label for="cert_file">%s</label>
                        <input type="text" id="cert_file" name="cert_file" value="%s" placeholder="%s">
                        <span class="help-text">%s</span>
                    </div>
                    <div class="form-row">
                        <label for="key_file">%s</label>
                        <input type="text" id="key_file" name="key_file" value="%s" placeholder="%s">
                        <span class="help-text">%s</span>
                    </div>

                    <h3>%s</h3>
                    <div class="form-row">
                        <label class="toggle">
                            <input type="checkbox" name="letsencrypt_enabled" %s>
                            <span class="slider"></span>
                            %s
                        </label>
                    </div>
                    <div class="form-row">
                        <label for="letsencrypt_email">%s</label>
                        <input type="email" id="letsencrypt_email" name="letsencrypt_email" value="%s" placeholder="%s">
                        <span class="help-text">%s</span>
                    </div>
                    <div class="form-row">
                        <label for="le_domains">%s</label>
                        <textarea id="le_domains" name="le_domains" rows="3" placeholder="%s">%s</textarea>
                        <span class="help-text">%s</span>
                    </div>
                    <div class="form-row">
                        <label class="toggle">
                            <input type="checkbox" name="le_staging" %s>
                            <span class="slider"></span>
                            %s
                        </label>
                    </div>

                    <div class="form-row">
                        <label for="letsencrypt_challenge">%s</label>
                        <select id="letsencrypt_challenge" name="letsencrypt_challenge" onchange="toggleDNSProvider(this.value)">
                            <option value="http-01" %s>%s</option>
                            <option value="tls-alpn-01" %s>%s</option>
                            <option value="dns-01" %s>%s</option>
                        </select>
                        <span class="help-text">%s</span>
                    </div>`,
		sslConfigurationLabel,
		data.CSRFToken,
		checked(h.config.Server.SSL.Enabled),
		enableSSLLabel,
		checked(h.config.Server.SSL.AutoTLS),
		autoTLSLabel,
		certificateFileLabel,
		h.config.Server.SSL.CertFile,
		certificateFilePlaceholder,
		certificateFileHelpLabel,
		keyFileLabel,
		h.config.Server.SSL.KeyFile,
		keyFilePlaceholder,
		keyFileHelpLabel,
		letsEncryptLabel,
		checked(h.config.Server.SSL.LetsEncrypt.Enabled),
		enableLetsEncryptLabel,
		emailAddressLabel,
		h.config.Server.SSL.LetsEncrypt.Email,
		emailAddressPlaceholder,
		emailAddressHelpLabel,
		domainsLabel,
		domainsPlaceholder,
		joinStrings(h.config.Server.SSL.LetsEncrypt.Domains),
		domainsHelpLabel,
		checked(h.config.Server.SSL.LetsEncrypt.Staging),
		useStagingLabel,
		acmeChallengeTypeLabel,
		selectedBool(challengeType == "http-01"),
		http01Label,
		selectedBool(challengeType == "tls-alpn-01"),
		tlsALPN01Label,
		selectedBool(challengeType == "dns-01"),
		dns01Label,
		acmeChallengeHelpLabel,
	)

	// DNS-01 Provider Section - shown when DNS-01 is selected
	hiddenClass := ""
	if challengeType != "dns-01" {
		hiddenClass = " hidden"
	}

	fmt.Fprintf(w, `
                    <div id="dns01-config" class="dns01-section%s">
                        <h3>%s</h3>
                        <div class="form-row">
                            <label for="dns_provider">%s</label>
                            <select id="dns_provider" name="dns_provider" onchange="showProviderFields(this.value)">
                                <option value="">%s</option>`, hiddenClass, dnsProviderConfigurationLabel, dnsProviderLabel, selectProviderLabel)

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
                            <span class="help-text">%s</span>
                        </div>`, dnsProviderHelpLabel)

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
                            <span>%s</span>
                        </div>`, dns01ConfiguredLabel)
	}

	fmt.Fprintf(w, `
                    </div>

                    <button type="submit" class="btn btn-primary">%s</button>
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
            </script>`, saveSSLSettingsLabel)
}

// selectedBool returns "selected" if condition is true
func selectedBool(b bool) string {
	if b {
		return "selected"
	}
	return ""
}

// renderServerNetworkContent renders the network settings overview page
// Per AI.md PART 17: Network settings overview with links to Tor and GeoIP
func (h *Handler) renderServerNetworkContent(w io.Writer, data *AdminPageData) {
	// Get status from Extra data
	torStatus := data.Extra["TorStatus"].(map[string]interface{})
	geoipStatus := data.Extra["GeoIPStatus"].(map[string]interface{})

	// Tor status display
	torRunning := torStatus["running"].(bool)
	torEnabled := torStatus["enabled"].(bool)
	torStatusText := adminDataString(data, "common.disabled", "Disabled")
	torStatusClass := "disabled"
	if torEnabled {
		torStatusText = adminDataString(data, "admin.network_page.available", "Available")
		torStatusClass = "warning"
		if torRunning {
			torStatusText = adminDataString(data, "admin.network_page.running", "Running")
			torStatusClass = "enabled"
		}
	}

	// GeoIP status display
	geoipEnabled := geoipStatus["enabled"].(bool)
	geoipStatusText := adminDataString(data, "common.disabled", "Disabled")
	geoipStatusClass := "disabled"
	if geoipEnabled {
		geoipStatusText = adminDataString(data, "common.enabled", "Enabled")
		geoipStatusClass = "enabled"
	}

	networkDescriptionLabel := adminDataString(data, "admin.network_page.description", "Configure network-related settings including Tor hidden service and GeoIP filtering.")
	torHiddenServiceLabel := adminDataString(data, "admin.network_page.tor_hidden_service", "Tor Hidden Service")
	torDescriptionLabel := adminDataString(data, "admin.network_page.tor_description", "Enable access via Tor for privacy-conscious users. Provides a .onion address for anonymous access.")
	configureTorLabel := adminDataString(data, "admin.network_page.configure_tor", "Configure Tor")
	geoIPFilteringLabel := adminDataString(data, "admin.network_page.geoip_filtering", "GeoIP Filtering")
	geoIPDescriptionLabel := adminDataString(data, "admin.network_page.geoip_description", "Block or allow access based on geographic location. Uses MaxMind-compatible MMDB databases.")
	configureGeoIPLabel := adminDataString(data, "admin.network_page.configure_geoip", "Configure GeoIP")

	fmt.Fprintf(w, `
            <div class="admin-content">
                %s
                <p class="desc-text">%s</p>

                <div class="settings-grid">
                    <div class="settings-card">
                        <div class="card-header">
                            <h3>%s</h3>
                            <span class="status-badge %s">%s</span>
                        </div>
                        <p>%s</p>
                        <div class="card-footer">
                            <a href="/admin/server/network/tor" class="btn">%s</a>
                        </div>
                    </div>

                    <div class="settings-card">
                        <div class="card-header">
                            <h3>%s</h3>
                            <span class="status-badge %s">%s</span>
                        </div>
                        <p>%s</p>
                        <div class="card-footer">
                            <a href="/admin/server/network/geoip" class="btn">%s</a>
                        </div>
                    </div>
                </div>
            </div>

            <style>
                .settings-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(300px, 1fr)); gap: 20px; margin-top: 20px; }
                .settings-card { background: var(--bg-secondary); border-radius: 8px; padding: 20px; border: 1px solid var(--border-color); }
                .card-header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 15px; }
                .card-header h3 { margin: 0; }
                .card-footer { margin-top: 15px; }
            </style>`,
		formatFlashMessages(data.Error, data.Success),
		networkDescriptionLabel,
		torHiddenServiceLabel,
		torStatusClass, torStatusText,
		torDescriptionLabel,
		configureTorLabel,
		geoIPFilteringLabel,
		geoipStatusClass, geoipStatusText,
		geoIPDescriptionLabel,
		configureGeoIPLabel,
	)
}

func (h *Handler) renderServerTorContent(w io.Writer, data *AdminPageData) {
	// Per AI.md PART 32: Tor is auto-enabled if binary found
	torStatus := adminDataString(data, "admin.tor_page.disabled_not_found", "Disabled (tor binary not found)")
	torStatusClass := "disabled"
	if h.config.Server.Tor.Enabled {
		torStatus = adminDataString(data, "common.enabled", "Enabled")
		torStatusClass = "enabled"
		if h.config.Server.Tor.OnionAddress != "" {
			torStatus = adminDataString(data, "admin.tor_page.running", "Running: %s", h.config.Server.Tor.OnionAddress)
		}
	}

	torHiddenServiceLabel := adminDataString(data, "admin.tor_page.title", "Tor Hidden Service")
	torDescriptionLabel := adminDataString(data, "admin.tor_page.description", "Per AI.md PART 32: Tor is automatically enabled when the tor binary is found.")
	statusLabel := adminDataString(data, "health.status", "Status")
	onionAddressLabel := adminDataString(data, "admin.tor_page.onion_address", "Onion Address")
	torBinaryPathLabel := adminDataString(data, "admin.tor_page.binary_path", "Tor Binary Path (optional)")
	binaryPlaceholderLabel := adminDataString(data, "admin.tor_page.binary_path_placeholder", "Auto-detect from PATH")
	binaryHelpLabel := adminDataString(data, "admin.tor_page.binary_path_help", "Leave empty to auto-detect. Common locations: /usr/bin/tor, /usr/local/bin/tor")
	updatePathLabel := adminDataString(data, "admin.tor_page.update_path", "Update Path")
	serviceControlLabel := adminDataString(data, "admin.tor_page.service_control", "Service Control")
	startTorLabel := adminDataString(data, "admin.tor_page.start_tor", "Start Tor")
	stopTorLabel := adminDataString(data, "admin.tor_page.stop_tor", "Stop Tor")
	restartTorLabel := adminDataString(data, "admin.tor_page.restart_tor", "Restart Tor")
	vanityAddressGenerationLabel := adminDataString(data, "admin.tor_page.vanity_generation", "Vanity Address Generation")
	vanityDescriptionLabel := adminDataString(data, "admin.tor_page.vanity_description", "Generate a custom .onion address with a memorable prefix (max 6 characters for built-in generation).")
	prefixLabel := adminDataString(data, "admin.tor_page.prefix", "Prefix (a-z, 2-7 only)")
	prefixPlaceholderLabel := adminDataString(data, "admin.tor_page.prefix_placeholder", "search")
	startGenerationLabel := adminDataString(data, "admin.tor_page.start_generation", "Start Generation")
	cancelLabel := adminDataString(data, "common.cancel", "Cancel")
	prefixDisplayLabel := adminDataString(data, "admin.tor_page.prefix_display", "Prefix")
	attemptsLabel := adminDataString(data, "admin.tor_page.attempts", "Attempts")
	startingLabel := adminDataString(data, "admin.tor_page.starting", "Starting...")
	keyManagementLabel := adminDataString(data, "admin.tor_page.key_management", "Key Management")
	keyManagementDescriptionLabel := adminDataString(data, "admin.tor_page.key_management_description", "Export or import hidden service keys. Useful for backup or using externally-generated vanity addresses.")
	exportKeysLabel := adminDataString(data, "admin.tor_page.export_keys", "Export Keys")
	importKeysLabel := adminDataString(data, "admin.tor_page.import_keys", "Import Keys")
	regenerateAddressLabel := adminDataString(data, "admin.tor_page.regenerate_address", "Regenerate Address")
	warningLabel := adminDataString(data, "admin.tor_page.warning_label", "Warning")
	regenerateWarningLabel := adminDataString(data, "admin.tor_page.regenerate_warning", "Regenerating will create a new .onion address. The old address will no longer work.")
	torStartedTemplate := adminDataStringRaw(data, "admin.tor_page.tor_started", "Tor started: %s")
	failedTemplate := adminDataStringRaw(data, "admin.tor_page.failed", "Failed: %s")
	torStoppedLabel := adminDataString(data, "admin.tor_page.tor_stopped", "Tor stopped")
	torRestartedTemplate := adminDataStringRaw(data, "admin.tor_page.tor_restarted", "Tor restarted: %s")
	invalidPrefixLabel := adminDataString(data, "admin.tor_page.invalid_prefix", "Invalid prefix: use a-z and 2-7 only")
	vanityGenerationStartedLabel := adminDataString(data, "admin.tor_page.vanity_started", "Vanity generation started")
	failedToStartLabel := adminDataString(data, "admin.tor_page.failed_to_start", "Failed to start")
	vanityGenerationCancelledLabel := adminDataString(data, "admin.tor_page.vanity_cancelled", "Vanity generation cancelled")
	foundTemplate := adminDataStringRaw(data, "admin.tor_page.found", "Found: %s")
	vanityAddressFoundTemplate := adminDataStringRaw(data, "admin.tor_page.vanity_found", "Vanity address found: %s")
	stoppedLabel := adminDataString(data, "admin.tor_page.stopped", "Stopped")
	searchingLabel := adminDataString(data, "admin.tor_page.searching", "Searching...")
	keysImportedTemplate := adminDataStringRaw(data, "admin.tor_page.keys_imported", "Keys imported, new address: %s")
	importFailedLabel := adminDataString(data, "admin.tor_page.import_failed", "Import failed")
	regenerateConfirmLabel := adminDataString(data, "admin.tor_page.regenerate_confirm", "Are you sure? This will create a new .onion address and the old one will stop working.")
	newAddressTemplate := adminDataStringRaw(data, "admin.tor_page.new_address", "New address: %s")
	failedLabel := adminDataString(data, "admin.tor_page.failed_short", "Failed")

	fmt.Fprintf(w, `
            <div class="admin-section">
                <h2>%s</h2>
                <p class="desc-text">%s</p>
                <table class="admin-table max-w-400">
                    <tr>
                        <td class="text-secondary">%s</td>
                        <td><span class="status-badge %s">%s</span></td>
                    </tr>
                    <tr>
                        <td class="text-secondary">%s</td>
                        <td><input type="text" id="onion-address" value="%s" readonly class="form-control"></td>
                    </tr>
                </table>

                <form method="POST" action="/admin/server/tor" class="mt-20">
                    <input type="hidden" name="csrf_token" value="%s">
                    <div class="form-row">
                        <label>%s</label>
                        <input type="text" name="binary" value="%s" placeholder="%s">
                        <p class="help-text">%s</p>
                    </div>
                    <button type="submit" class="btn">%s</button>
                </form>
            </div>`,
		torHiddenServiceLabel,
		torDescriptionLabel,
		statusLabel,
		torStatusClass, torStatus,
		onionAddressLabel,
		h.config.Server.Tor.OnionAddress,
		data.CSRFToken,
		torBinaryPathLabel,
		h.config.Server.Tor.Binary,
		binaryPlaceholderLabel,
		binaryHelpLabel,
		updatePathLabel,
	)

	// Service Control, Vanity Address, and Key Management sections
	fmt.Fprintf(w, `
            <!-- Service Control per AI.md PART 32 -->
            <div class="admin-section">
                <h2>%s</h2>
                <div class="btn-group">
                    <button type="button" class="btn btn-green" onclick="torStart()">%s</button>
                    <button type="button" class="btn btn-red" onclick="torStop()">%s</button>
                    <button type="button" class="btn btn-cyan" onclick="torRestart()">%s</button>
                </div>
                <div id="tor-status" class="mt-10"></div>
            </div>

            <!-- Vanity Address Generation per AI.md PART 32 -->
            <div class="admin-section">
                <h2>%s</h2>
                <p class="help-text">%s</p>
                <div class="form-row">
                    <label>%s</label>
                    <input type="text" id="vanity-prefix" maxlength="6" pattern="[a-z2-7]+" placeholder="%s">
                </div>
                <div class="btn-group">
                    <button type="button" class="btn btn-cyan" onclick="vanityStart()">%s</button>
                    <button type="button" class="btn btn-red" onclick="vanityCancel()">%s</button>
                </div>
                <div id="vanity-progress" class="mt-10" style="display:none;">
                    <p>%s: <span id="vanity-prefix-display"></span></p>
                    <p>%s: <span id="vanity-attempts">0</span></p>
                    <p>%s: <span id="vanity-status">%s</span></p>
                </div>
            </div>

            <!-- Key Management per AI.md PART 32 -->
            <div class="admin-section">
                <h2>%s</h2>
                <p class="help-text">%s</p>
                <div class="btn-group">
                    <button type="button" class="btn" onclick="exportKeys()">%s</button>
                    <button type="button" class="btn" onclick="document.getElementById('key-import-file').click()">%s</button>
                    <input type="file" id="key-import-file" style="display:none" onchange="importKeys(this.files[0])">
                </div>
                <button type="button" class="btn btn-red mt-10" onclick="regenerateAddress()">%s</button>
                <p class="help-text mt-10"><strong>%s:</strong> %s</p>
            </div>

            <script>
            function formatMessage(template, value) {
                return template.replace('%%s', value);
            }

            // Tor Service Control
            function torStart() {
                fetch('/api/v1/admin/tor/start', {method: 'POST'})
                    .then(r => r.json())
                    .then(d => {
                        showToast(d.status === 'started' ? formatMessage(%q, d.address) : d.error, d.status === 'started' ? 'success' : 'error');
                        if (d.address) document.getElementById('onion-address').value = d.address;
                    })
                    .catch(e => showToast(formatMessage(%q, e), 'error'));
            }
            function torStop() {
                fetch('/api/v1/admin/tor/stop', {method: 'POST'})
                    .then(r => r.json())
                    .then(d => showToast(d.status === 'stopped' ? %q : d.error, d.status === 'stopped' ? 'success' : 'error'))
                    .catch(e => showToast(formatMessage(%q, e), 'error'));
            }
            function torRestart() {
                fetch('/api/v1/admin/tor/restart', {method: 'POST'})
                    .then(r => r.json())
                    .then(d => {
                        showToast(d.status === 'restarted' ? formatMessage(%q, d.address) : d.error, d.status === 'restarted' ? 'success' : 'error');
                        if (d.address) document.getElementById('onion-address').value = d.address;
                    })
                    .catch(e => showToast(formatMessage(%q, e), 'error'));
            }

            // Vanity Generation
            var vanityPoll = null;
            function vanityStart() {
                var prefix = document.getElementById('vanity-prefix').value.toLowerCase();
                if (!prefix || !/^[a-z2-7]+$/.test(prefix)) {
                    showToast(%q, 'error');
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
                        showToast(%q, 'success');
                        document.getElementById('vanity-progress').style.display = 'block';
                        document.getElementById('vanity-prefix-display').textContent = prefix;
                        vanityPoll = setInterval(pollVanityStatus, 2000);
                    } else {
                        showToast(d.error || %q, 'error');
                    }
                })
                .catch(e => showToast(formatMessage(%q, e), 'error'));
            }
            function vanityCancel() {
                if (vanityPoll) clearInterval(vanityPoll);
                fetch('/api/v1/admin/tor/vanity/cancel', {method: 'POST'})
                    .then(() => {
                        showToast(%q, 'success');
                        document.getElementById('vanity-progress').style.display = 'none';
                    })
                    .catch(e => showToast(formatMessage(%q, e), 'error'));
            }
            function pollVanityStatus() {
                fetch('/api/v1/admin/tor/vanity/status')
                    .then(r => r.json())
                    .then(d => {
                        document.getElementById('vanity-attempts').textContent = d.attempts || 0;
                        if (d.found) {
                            clearInterval(vanityPoll);
                            document.getElementById('vanity-status').textContent = formatMessage(%q, d.address + '.onion');
                            showToast(formatMessage(%q, d.address + '.onion'), 'success');
                        } else if (!d.running) {
                            clearInterval(vanityPoll);
                            document.getElementById('vanity-status').textContent = d.error || %q;
                        } else {
                            document.getElementById('vanity-status').textContent = %q;
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
                            showToast(formatMessage(%q, d.address), 'success');
                            document.getElementById('onion-address').value = d.address;
                        } else {
                            showToast(d.error || %q, 'error');
                        }
                    })
                    .catch(e => showToast(formatMessage(%q, e), 'error'));
                };
                reader.readAsArrayBuffer(file);
            }
            function regenerateAddress() {
                if (!confirm(%q)) return;
                fetch('/api/v1/admin/tor/address/regenerate', {method: 'POST'})
                    .then(r => r.json())
                    .then(d => {
                        if (d.status === 'regenerated') {
                            showToast(formatMessage(%q, d.new_address), 'success');
                            document.getElementById('onion-address').value = d.new_address;
                        } else {
                            showToast(d.error || %q, 'error');
                        }
                    })
                    .catch(e => showToast(formatMessage(%q, e), 'error'));
            }
            </script>`,
		serviceControlLabel,
		startTorLabel,
		stopTorLabel,
		restartTorLabel,
		vanityAddressGenerationLabel,
		vanityDescriptionLabel,
		prefixLabel,
		prefixPlaceholderLabel,
		startGenerationLabel,
		cancelLabel,
		prefixDisplayLabel,
		attemptsLabel,
		statusLabel,
		startingLabel,
		keyManagementLabel,
		keyManagementDescriptionLabel,
		exportKeysLabel,
		importKeysLabel,
		regenerateAddressLabel,
		warningLabel,
		regenerateWarningLabel,
		torStartedTemplate,
		failedTemplate,
		torStoppedLabel,
		failedTemplate,
		torRestartedTemplate,
		failedTemplate,
		invalidPrefixLabel,
		vanityGenerationStartedLabel,
		failedToStartLabel,
		failedTemplate,
		vanityGenerationCancelledLabel,
		failedTemplate,
		foundTemplate,
		vanityAddressFoundTemplate,
		stoppedLabel,
		searchingLabel,
		keysImportedTemplate,
		importFailedLabel,
		failedTemplate,
		regenerateConfirmLabel,
		newAddressTemplate,
		failedLabel,
		failedTemplate,
	)
}

func (h *Handler) renderServerWebContent(w io.Writer, data *AdminPageData) {
	robotsTitleLabel := adminDataString(data, "admin.web_page.robots_title", "robots.txt Configuration")
	allowPathsLabel := adminDataString(data, "admin.web_page.allow_paths", "Allow Paths (one per line)")
	denyPathsLabel := adminDataString(data, "admin.web_page.deny_paths", "Deny Paths (one per line)")
	saveRobotsLabel := adminDataString(data, "admin.web_page.save_robots", "Save robots.txt")
	securityTitleLabel := adminDataString(data, "admin.web_page.security_title", "security.txt Configuration")
	contactLabel := adminDataString(data, "admin.web_page.contact", "Contact")
	expiresLabel := adminDataString(data, "admin.web_page.expires", "Expires")
	saveSecurityLabel := adminDataString(data, "admin.web_page.save_security", "Save security.txt")
	cookieTitleLabel := adminDataString(data, "admin.web_page.cookie_title", "Cookie Consent")
	enableCookieConsentLabel := adminDataString(data, "admin.web_page.enable_cookie_consent", "Enable Cookie Consent Popup")
	messageLabel := adminDataString(data, "admin.web_page.message", "Message")
	privacyPolicyURLLabel := adminDataString(data, "admin.web_page.privacy_policy_url", "Privacy Policy URL")
	saveCookieSettingsLabel := adminDataString(data, "admin.web_page.save_cookie_settings", "Save Cookie Settings")
	corsTitleLabel := adminDataString(data, "admin.web_page.cors_title", "CORS Settings")
	allowedOriginsLabel := adminDataString(data, "admin.web_page.allowed_origins", "Allowed Origins")
	allowedOriginsPlaceholder := adminDataString(data, "admin.web_page.allowed_origins_placeholder", "* or comma-separated origins")
	saveCORSSettingsLabel := adminDataString(data, "admin.web_page.save_cors_settings", "Save CORS Settings")
	contactPlaceholder := adminDataString(data, "admin.web_page.contact_placeholder", "mailto:security@example.com")
	privacyPolicyURLPlaceholder := adminDataString(data, "admin.web_page.privacy_policy_url_placeholder", "/server/privacy")

	fmt.Fprintf(w, `
            <div class="admin-section">
                <h2>%s</h2>
                <form method="POST" action="/admin/server/web/robots">
                    <input type="hidden" name="csrf_token" value="%s">
                    <div class="form-row">
                        <label>%s</label>
                        <textarea name="allow" rows="4" placeholder="/&#10;/api">%s</textarea>
                    </div>
                    <div class="form-row">
                        <label>%s</label>
                        <textarea name="deny" rows="4" placeholder="/admin&#10;/private">%s</textarea>
                    </div>
                    <button type="submit" class="btn">%s</button>
                </form>
            </div>

            <div class="admin-section">
                <h2>%s</h2>
                <form method="POST" action="/admin/server/web/security">
                    <input type="hidden" name="csrf_token" value="%s">
                    <div class="form-row">
                        <label>%s</label>
                        <input type="text" name="contact" value="%s" placeholder="%s">
                    </div>
                    <div class="form-row">
                        <label>%s</label>
                        <input type="date" name="expires" value="%s">
                    </div>
                    <button type="submit" class="btn">%s</button>
                </form>
            </div>

            <div class="admin-section">
                <h2>%s</h2>
                <form method="POST" action="/admin/server/web/cookies">
                    <input type="hidden" name="csrf_token" value="%s">
                    <div class="form-row">
                        <label><input type="checkbox" name="enabled" %s> %s</label>
                    </div>
                    <div class="form-row">
                        <label>%s</label>
                        <textarea name="message" rows="2">%s</textarea>
                    </div>
                    <div class="form-row">
                        <label>%s</label>
                        <input type="text" name="policy_url" value="%s" placeholder="%s">
                    </div>
                    <button type="submit" class="btn">%s</button>
                </form>
            </div>

            <div class="admin-section">
                <h2>%s</h2>
                <form method="POST" action="/admin/server/web/cors">
                    <input type="hidden" name="csrf_token" value="%s">
                    <div class="form-row">
                        <label>%s</label>
                        <input type="text" name="cors" value="%s" placeholder="%s">
                    </div>
                    <button type="submit" class="btn">%s</button>
                </form>
            </div>`,
		robotsTitleLabel,
		data.CSRFToken,
		allowPathsLabel,
		joinStrings(h.config.Server.Web.Robots.Allow),
		denyPathsLabel,
		joinStrings(h.config.Server.Web.Robots.Deny),
		saveRobotsLabel,
		securityTitleLabel,
		data.CSRFToken,
		contactLabel,
		h.config.Server.Web.Security.Contact,
		contactPlaceholder,
		expiresLabel,
		formatDateForInput(h.config.Server.Web.Security.Expires),
		saveSecurityLabel,
		cookieTitleLabel,
		data.CSRFToken,
		checked(h.config.Server.Web.CookieConsent.Enabled),
		enableCookieConsentLabel,
		messageLabel,
		h.config.Server.Web.CookieConsent.Message,
		privacyPolicyURLLabel,
		h.config.Server.Web.CookieConsent.PolicyURL,
		privacyPolicyURLPlaceholder,
		saveCookieSettingsLabel,
		corsTitleLabel,
		data.CSRFToken,
		allowedOriginsLabel,
		h.config.Server.Web.CORS,
		allowedOriginsPlaceholder,
		saveCORSSettingsLabel,
	)
}

// renderServerEmailContent renders the email/SMTP settings page
// Per AI.md PART 18: Nested SMTP and From blocks with TLS mode dropdown
func (h *Handler) renderServerEmailContent(w io.Writer, data *AdminPageData) {
	// Per AI.md PART 18: TLS mode selection
	tlsMode := h.config.Server.Email.SMTP.TLS
	if tlsMode == "" {
		tlsMode = "auto"
	}

	emailConfigurationLabel := adminDataString(data, "admin.email_page.title", "Email / SMTP Configuration")
	emailHelpLabel := adminDataString(data, "admin.email_page.description", "Email is automatically enabled when SMTP host is configured.")
	smtpServerLabel := adminDataString(data, "admin.email_page.smtp_server", "SMTP Server")
	smtpHostLabel := adminDataString(data, "admin.email_page.smtp_host", "SMTP Host")
	smtpHostHelpLabel := adminDataString(data, "admin.email_page.smtp_host_help", "Leave empty to auto-detect local SMTP server")
	smtpPortLabel := adminDataString(data, "admin.email_page.smtp_port", "SMTP Port")
	smtpUsernameLabel := adminDataString(data, "admin.email_page.smtp_username", "SMTP Username")
	smtpPasswordLabel := adminDataString(data, "admin.email_page.smtp_password", "SMTP Password")
	smtpPasswordPlaceholder := adminDataString(data, "admin.email_page.smtp_password_placeholder", "Leave empty to keep current")
	tlsModeLabel := adminDataString(data, "admin.email_page.tls_mode", "TLS Mode")
	tlsAutoLabel := adminDataString(data, "admin.email_page.tls_auto", "Auto (try STARTTLS)")
	tlsStartTLSLabel := adminDataString(data, "admin.email_page.tls_starttls", "STARTTLS")
	tlsDirectLabel := adminDataString(data, "admin.email_page.tls_direct", "TLS (direct)")
	tlsNoneLabel := adminDataString(data, "admin.email_page.tls_none", "None (insecure)")
	fromAddressLabel := adminDataString(data, "admin.email_page.from_address", "From Address")
	fromNameLabel := adminDataString(data, "admin.email_page.from_name", "From Name")
	fromNamePlaceholder := adminDataString(data, "admin.email_page.from_name_placeholder", "Search (defaults to app name)")
	fromEmailLabel := adminDataString(data, "admin.email_page.from_email", "From Email")
	fromEmailPlaceholder := adminDataString(data, "admin.email_page.from_email_placeholder", "noreply@example.com (defaults to no-reply@fqdn)")
	saveEmailSettingsLabel := adminDataString(data, "admin.email_page.save_email_settings", "Save Email Settings")
	sendTestEmailLabel := adminDataString(data, "admin.email_page.send_test_email", "Send Test Email")
	testEmailSentLabel := adminDataString(data, "admin.email_page.test_email_sent", "Test email sent!")
	testEmailFailedLabel := adminDataString(data, "admin.email_page.test_email_failed", "Failed to send test email:")
	smtpHostPlaceholder := adminDataString(data, "admin.email_page.smtp_host_placeholder", "smtp.example.com")
	smtpUsernamePlaceholder := adminDataString(data, "admin.email_page.smtp_username_placeholder", "user@example.com")

	fmt.Fprintf(w, `
            <div class="admin-section">
                <h2>%s</h2>
                <p class="help-text">%s</p>
                <form method="POST" action="/admin/server/email">
                    <input type="hidden" name="csrf_token" value="%s">
                    <h3>%s</h3>
                    <div class="form-row">
                        <label>%s</label>
                        <input type="text" name="smtp_host" value="%s" placeholder="%s">
                        <small>%s</small>
                    </div>
                    <div class="form-row">
                        <label>%s</label>
                        <input type="number" name="smtp_port" value="%d" placeholder="587">
                    </div>
                    <div class="form-row">
                        <label>%s</label>
                        <input type="text" name="smtp_username" value="%s" placeholder="%s">
                    </div>
                    <div class="form-row">
                        <label>%s</label>
                        <input type="password" name="smtp_password" value="%s" placeholder="%s">
                    </div>
                    <div class="form-row">
                        <label>%s</label>
                        <select name="smtp_tls">
                            <option value="auto" %s>%s</option>
                            <option value="starttls" %s>%s</option>
                            <option value="tls" %s>%s</option>
                            <option value="none" %s>%s</option>
                        </select>
                    </div>
                    <h3>%s</h3>
                    <div class="form-row">
                        <label>%s</label>
                        <input type="text" name="from_name" value="%s" placeholder="%s">
                    </div>
                    <div class="form-row">
                        <label>%s</label>
                        <input type="email" name="from_email" value="%s" placeholder="%s">
                    </div>
                    <button type="submit" class="btn">%s</button>
                    <button type="button" class="btn ml-10 btn-cyan" onclick="testEmail()">%s</button>
                </form>
            </div>
            <script>
            function testEmail() {
                fetch('/admin/api/email/test', {method: 'POST'})
                    .then(r => r.json())
                    .then(d => showToast(d.message || %q, 'success'))
                    .catch(e => showToast(%q + ' ' + e, 'error'));
            }
            </script>`,
		emailConfigurationLabel,
		emailHelpLabel,
		data.CSRFToken,
		smtpServerLabel,
		smtpHostLabel,
		h.config.Server.Email.SMTP.Host,
		smtpHostPlaceholder,
		smtpHostHelpLabel,
		smtpPortLabel,
		h.config.Server.Email.SMTP.Port,
		smtpUsernameLabel,
		h.config.Server.Email.SMTP.Username,
		smtpUsernamePlaceholder,
		smtpPasswordLabel,
		maskPassword(h.config.Server.Email.SMTP.Password),
		smtpPasswordPlaceholder,
		tlsModeLabel,
		selected(tlsMode == "auto"),
		tlsAutoLabel,
		selected(tlsMode == "starttls"),
		tlsStartTLSLabel,
		selected(tlsMode == "tls"),
		tlsDirectLabel,
		selected(tlsMode == "none"),
		tlsNoneLabel,
		fromAddressLabel,
		fromNameLabel,
		h.config.Server.Email.From.Name,
		fromNamePlaceholder,
		fromEmailLabel,
		h.config.Server.Email.From.Email,
		fromEmailPlaceholder,
		saveEmailSettingsLabel,
		sendTestEmailLabel,
		testEmailSentLabel,
		testEmailFailedLabel,
	)
}

func (h *Handler) renderServerAnnouncementsContent(w io.Writer, data *AdminPageData) {
	announcementsLabel := adminDataString(data, "admin.announcements_page.title", "Announcements")
	enableAnnouncementsLabel := adminDataString(data, "admin.announcements_page.enable", "Enable Announcements")
	saveSettingsLabel := adminDataString(data, "admin.announcements_page.save_settings", "Save Settings")
	addNewAnnouncementLabel := adminDataString(data, "admin.announcements_page.add_new", "Add New Announcement")
	idLabel := adminDataString(data, "admin.announcements_page.id", "ID")
	idHelpLabel := adminDataString(data, "admin.announcements_page.id_help", "ID (unique identifier)")
	typeLabel := adminDataString(data, "admin.announcements_page.type", "Type")
	infoLabel := adminDataString(data, "admin.announcements_page.info", "Info")
	warningLabel := adminDataString(data, "admin.announcements_page.warning", "Warning")
	errorLabel := adminDataString(data, "common.error", "Error")
	successLabel := adminDataString(data, "common.success", "Success")
	titleLabel := adminDataString(data, "admin.announcements_page.announcement_title", "Title")
	titlePlaceholder := adminDataString(data, "admin.announcements_page.title_placeholder", "Announcement Title")
	messageLabel := adminDataString(data, "admin.announcements_page.message", "Message")
	messagePlaceholder := adminDataString(data, "admin.announcements_page.message_placeholder", "Announcement message...")
	startDateLabel := adminDataString(data, "admin.announcements_page.start_date", "Start Date (optional)")
	endDateLabel := adminDataString(data, "admin.announcements_page.end_date", "End Date (optional)")
	dismissibleLabel := adminDataString(data, "admin.announcements_page.dismissible", "User can dismiss")
	addAnnouncementLabel := adminDataString(data, "admin.announcements_page.add_announcement", "Add Announcement")
	activeAnnouncementsLabel := adminDataString(data, "admin.announcements_page.active_announcements", "Active Announcements")
	startLabel := adminDataString(data, "admin.announcements_page.start", "Start")
	endLabel := adminDataString(data, "admin.announcements_page.end", "End")
	actionsLabel := adminDataString(data, "admin.announcements_page.actions", "Actions")
	noAnnouncementsLabel := adminDataString(data, "admin.announcements_page.no_announcements", "No announcements configured")
	deleteLabel := adminDataString(data, "common.delete", "Delete")
	idPlaceholder := adminDataString(data, "admin.announcements_page.id_placeholder", "announcement-1")

	fmt.Fprintf(w, `
            <div class="admin-section">
                <h2>%s</h2>
                <form method="POST" action="/admin/server/announcements">
                    <input type="hidden" name="csrf_token" value="%s">
                    <div class="form-row">
                        <label><input type="checkbox" name="enabled" %s> %s</label>
                    </div>
                    <button type="submit" class="btn">%s</button>
                </form>
            </div>

            <div class="admin-section">
                <h2>%s</h2>
                <form method="POST" action="/admin/server/announcements/add">
                    <input type="hidden" name="csrf_token" value="%s">
                    <div class="form-row">
                        <label>%s</label>
                        <input type="text" name="id" required placeholder="%s">
                    </div>
                    <div class="form-row">
                        <label>%s</label>
                        <select name="type">
                            <option value="info">%s</option>
                            <option value="warning">%s</option>
                            <option value="error">%s</option>
                            <option value="success">%s</option>
                        </select>
                    </div>
                    <div class="form-row">
                        <label>%s</label>
                        <input type="text" name="title" required placeholder="%s">
                    </div>
                    <div class="form-row">
                        <label>%s</label>
                        <textarea name="message" rows="3" required placeholder="%s"></textarea>
                    </div>
                    <div class="form-row">
                        <label>%s</label>
                        <input type="datetime-local" name="start">
                    </div>
                    <div class="form-row">
                        <label>%s</label>
                        <input type="datetime-local" name="end">
                    </div>
                    <div class="form-row">
                        <label><input type="checkbox" name="dismissible" checked> %s</label>
                    </div>
                    <button type="submit" class="btn">%s</button>
                </form>
            </div>

            <div class="admin-section">
                <h2>%s</h2>
                <table class="admin-table">
                    <thead>
                        <tr>
                            <th>%s</th>
                            <th>%s</th>
                            <th>%s</th>
                            <th>%s</th>
                            <th>%s</th>
                            <th>%s</th>
                        </tr>
                    </thead>
                    <tbody>`,
		announcementsLabel,
		data.CSRFToken,
		checked(h.config.Server.Web.Announcements.Enabled),
		enableAnnouncementsLabel,
		saveSettingsLabel,
		addNewAnnouncementLabel,
		data.CSRFToken,
		idHelpLabel,
		idPlaceholder,
		typeLabel,
		infoLabel,
		warningLabel,
		errorLabel,
		successLabel,
		titleLabel,
		titlePlaceholder,
		messageLabel,
		messagePlaceholder,
		startDateLabel,
		endDateLabel,
		dismissibleLabel,
		addAnnouncementLabel,
		activeAnnouncementsLabel,
		idLabel,
		typeLabel,
		titleLabel,
		startLabel,
		endLabel,
		actionsLabel,
	)

	if len(h.config.Server.Web.Announcements.Messages) == 0 {
		fmt.Fprintf(w, `
                        <tr>
                            <td colspan="6" class="empty-message">%s</td>
                        </tr>`, noAnnouncementsLabel)
	} else {
		for _, a := range h.config.Server.Web.Announcements.Messages {
			typeText := a.Type
			switch a.Type {
			case "info":
				typeText = infoLabel
			case "warning":
				typeText = warningLabel
			case "error":
				typeText = errorLabel
			case "success":
				typeText = successLabel
			}

			fmt.Fprintf(w, `
                        <tr>
                            <td>%s</td>
                            <td><span class="status-badge %s">%s</span></td>
                            <td>%s</td>
                            <td>%s</td>
                            <td>%s</td>
                            <td>
                                <form method="POST" action="/admin/server/announcements/delete" class="form-inline">
                                    <input type="hidden" name="csrf_token" value="%s">
                                    <input type="hidden" name="id" value="%s">
                                    <button type="submit" class="btn btn-danger btn-sm">%s</button>
                                </form>
                            </td>
                        </tr>`,
				a.ID,
				announcementTypeClass(a.Type),
				typeText,
				a.Title,
				formatAnnouncementDate(data, a.Start),
				formatAnnouncementDate(data, a.End),
				data.CSRFToken,
				a.ID,
				deleteLabel,
			)
		}
	}

	fmt.Fprintf(w, `
                    </tbody>
                </table>
            </div>`)
}

func (h *Handler) renderServerGeoIPContent(w io.Writer, data *AdminPageData) {
	geoIPConfigurationLabel := adminDataString(data, "admin.geoip_page.title", "GeoIP Configuration")
	geoIPDescriptionLabel := adminDataString(data, "admin.geoip_page.description", "Uses MMDB format databases from sapics/ip-location-db (free, no API key required).")
	enableGeoIPLabel := adminDataString(data, "admin.geoip_page.enable_geoip", "Enable GeoIP")
	databaseDirectoryLabel := adminDataString(data, "admin.geoip_page.database_directory", "Database Directory")
	updateFrequencyLabel := adminDataString(data, "admin.geoip_page.update_frequency", "Update Frequency")
	neverLabel := adminDataString(data, "admin.geoip_page.never", "Never")
	dailyLabel := adminDataString(data, "admin.geoip_page.daily", "Daily")
	weeklyLabel := adminDataString(data, "admin.geoip_page.weekly", "Weekly")
	monthlyLabel := adminDataString(data, "admin.geoip_page.monthly", "Monthly")
	enableASNLookupsLabel := adminDataString(data, "admin.geoip_page.enable_asn", "Enable ASN Lookups")
	enableCountryLookupsLabel := adminDataString(data, "admin.geoip_page.enable_country", "Enable Country Lookups")
	enableCityLookupsLabel := adminDataString(data, "admin.geoip_page.enable_city", "Enable City Lookups (larger download)")
	saveGeoIPSettingsLabel := adminDataString(data, "admin.geoip_page.save_settings", "Save GeoIP Settings")
	countryRestrictionsLabel := adminDataString(data, "admin.geoip_page.country_restrictions", "Country Restrictions")
	denyCountriesLabel := adminDataString(data, "admin.geoip_page.deny_countries", "Deny Countries (ISO 3166-1 alpha-2, comma-separated)")
	allowOnlyCountriesLabel := adminDataString(data, "admin.geoip_page.allow_only_countries", "Allow Only Countries (leave empty for no restriction)")
	saveRestrictionsLabel := adminDataString(data, "admin.geoip_page.save_restrictions", "Save Restrictions")
	denyCountriesPlaceholder := adminDataString(data, "admin.geoip_page.deny_countries_placeholder", "CN, RU, KP")
	allowedCountriesPlaceholder := adminDataString(data, "admin.geoip_page.allowed_countries_placeholder", "US, CA, GB")

	fmt.Fprintf(w, `
            <div class="admin-section">
                <h2>%s</h2>
                <p class="desc-text">%s</p>
                <form method="POST" action="/admin/server/geoip">
                    <input type="hidden" name="csrf_token" value="%s">
                    <div class="form-row">
                        <label><input type="checkbox" name="enabled" %s> %s</label>
                    </div>
                    <div class="form-row">
                        <label>%s</label>
                        <input type="text" name="dir" value="%s" placeholder="/data/geoip">
                    </div>
                    <div class="form-row">
                        <label>%s</label>
                        <select name="update">
                            <option value="never" %s>%s</option>
                            <option value="daily" %s>%s</option>
                            <option value="weekly" %s>%s</option>
                            <option value="monthly" %s>%s</option>
                        </select>
                    </div>
                    <div class="form-row">
                        <label><input type="checkbox" name="asn" %s> %s</label>
                    </div>
                    <div class="form-row">
                        <label><input type="checkbox" name="country" %s> %s</label>
                    </div>
                    <div class="form-row">
                        <label><input type="checkbox" name="city" %s> %s</label>
                    </div>
                    <button type="submit" class="btn">%s</button>
                </form>
            </div>

            <div class="admin-section">
                <h2>%s</h2>
                <form method="POST" action="/admin/server/geoip/restrictions">
                    <input type="hidden" name="csrf_token" value="%s">
                    <div class="form-row">
                        <label>%s</label>
                        <input type="text" name="deny_countries" value="%s" placeholder="%s">
                    </div>
                    <div class="form-row">
                        <label>%s</label>
                        <input type="text" name="allowed_countries" value="%s" placeholder="%s">
                    </div>
                    <button type="submit" class="btn">%s</button>
                </form>
            </div>`,
		geoIPConfigurationLabel,
		geoIPDescriptionLabel,
		data.CSRFToken,
		checked(h.config.Server.GeoIP.Enabled),
		enableGeoIPLabel,
		databaseDirectoryLabel,
		h.config.Server.GeoIP.Dir,
		updateFrequencyLabel,
		selectedValue(h.config.Server.GeoIP.Update, "never"),
		neverLabel,
		selectedValue(h.config.Server.GeoIP.Update, "daily"),
		dailyLabel,
		selectedValue(h.config.Server.GeoIP.Update, "weekly"),
		weeklyLabel,
		selectedValue(h.config.Server.GeoIP.Update, "monthly"),
		monthlyLabel,
		checked(h.config.Server.GeoIP.ASN),
		enableASNLookupsLabel,
		checked(h.config.Server.GeoIP.Country),
		enableCountryLookupsLabel,
		checked(h.config.Server.GeoIP.City),
		enableCityLookupsLabel,
		saveGeoIPSettingsLabel,
		countryRestrictionsLabel,
		data.CSRFToken,
		denyCountriesLabel,
		joinStrings(h.config.Server.GeoIP.DenyCountries),
		denyCountriesPlaceholder,
		allowOnlyCountriesLabel,
		joinStrings(h.config.Server.GeoIP.AllowedCountries),
		allowedCountriesPlaceholder,
		saveRestrictionsLabel,
	)
}

func (h *Handler) renderServerMetricsContent(w io.Writer, data *AdminPageData) {
	prometheusMetricsLabel := adminDataString(data, "admin.metrics_page.title", "Prometheus Metrics")
	metricsDescriptionLabel := adminDataString(data, "admin.metrics_page.description", "Expose Prometheus-compatible metrics endpoint for monitoring.")
	enableMetricsEndpointLabel := adminDataString(data, "admin.metrics_page.enable_endpoint", "Enable Metrics Endpoint")
	endpointPathLabel := adminDataString(data, "admin.metrics_page.endpoint_path", "Endpoint Path")
	includeSystemMetricsLabel := adminDataString(data, "admin.metrics_page.include_system", "Include System Metrics (CPU, Memory, Disk)")
	bearerTokenLabel := adminDataString(data, "admin.metrics_page.bearer_token", "Bearer Token (empty = no authentication)")
	bearerTokenHelpLabel := adminDataString(data, "admin.metrics_page.bearer_token_help", "If set, requests must include: Authorization: Bearer <token>")
	saveMetricsSettingsLabel := adminDataString(data, "admin.metrics_page.save_settings", "Save Metrics Settings")
	availableMetricsLabel := adminDataString(data, "admin.metrics_page.available_metrics", "Available Metrics")
	metricLabel := adminDataString(data, "admin.metrics_page.metric", "Metric")
	typeLabel := adminDataString(data, "admin.metrics_page.type", "Type")
	descriptionLabel := adminDataString(data, "admin.metrics_page.description_label", "Description")
	counterLabel := adminDataString(data, "admin.metrics_page.counter", "Counter")
	histogramLabel := adminDataString(data, "admin.metrics_page.histogram", "Histogram")
	gaugeLabel := adminDataString(data, "admin.metrics_page.gauge", "Gauge")
	totalSearchRequestsLabel := adminDataString(data, "admin.metrics_page.total_search_requests", "Total search requests")
	requestDurationLabel := adminDataString(data, "admin.metrics_page.request_duration", "Request duration in seconds")
	requestsPerEngineLabel := adminDataString(data, "admin.metrics_page.requests_per_engine", "Requests per search engine")
	errorsPerEngineLabel := adminDataString(data, "admin.metrics_page.errors_per_engine", "Errors per search engine")
	resultsReturnedLabel := adminDataString(data, "admin.metrics_page.results_returned", "Number of results returned")
	cacheHitCountLabel := adminDataString(data, "admin.metrics_page.cache_hit_count", "Cache hit count")
	cacheMissCountLabel := adminDataString(data, "admin.metrics_page.cache_miss_count", "Cache miss count")
	cpuTimeUsedLabel := adminDataString(data, "admin.metrics_page.cpu_time_used", "CPU time used (if system metrics enabled)")
	memoryUsageLabel := adminDataString(data, "admin.metrics_page.memory_usage", "Memory usage (if system metrics enabled)")
	endpointPathPlaceholder := adminDataString(data, "admin.metrics_page.endpoint_path_placeholder", "/metrics")
	bearerTokenPlaceholder := adminDataString(data, "admin.metrics_page.bearer_token_placeholder", "Leave empty for no auth")

	fmt.Fprintf(w, `
            <div class="admin-section">
                <h2>%s</h2>
                <p class="desc-text">%s</p>
                <form method="POST" action="/admin/server/metrics">
                    <input type="hidden" name="csrf_token" value="%s">
                    <div class="form-row">
                        <label><input type="checkbox" name="enabled" %s> %s</label>
                    </div>
                    <div class="form-row">
                        <label>%s</label>
                        <input type="text" name="path" value="%s" placeholder="%s">
                    </div>
                    <div class="form-row">
                        <label><input type="checkbox" name="include_system" %s> %s</label>
                    </div>
                    <div class="form-row">
                        <label>%s</label>
                        <input type="text" name="token" value="%s" placeholder="%s">
                        <p class="help-text">
                            %s
                        </p>
                    </div>
                    <button type="submit" class="btn">%s</button>
                </form>
            </div>

            <div class="admin-section">
                <h2>%s</h2>
                <table class="admin-table">
                    <thead>
                        <tr>
                            <th>%s</th>
                            <th>%s</th>
                            <th>%s</th>
                        </tr>
                    </thead>
                    <tbody>
                        <tr><td>search_requests_total</td><td>%s</td><td>%s</td></tr>
                        <tr><td>search_request_duration_seconds</td><td>%s</td><td>%s</td></tr>
                        <tr><td>search_engine_requests_total</td><td>%s</td><td>%s</td></tr>
                        <tr><td>search_engine_errors_total</td><td>%s</td><td>%s</td></tr>
                        <tr><td>search_results_returned</td><td>%s</td><td>%s</td></tr>
                        <tr><td>search_cache_hits_total</td><td>%s</td><td>%s</td></tr>
                        <tr><td>search_cache_misses_total</td><td>%s</td><td>%s</td></tr>
                        <tr><td>process_cpu_seconds_total</td><td>%s</td><td>%s</td></tr>
                        <tr><td>process_resident_memory_bytes</td><td>%s</td><td>%s</td></tr>
                    </tbody>
                </table>
            </div>`,
		prometheusMetricsLabel,
		metricsDescriptionLabel,
		data.CSRFToken,
		checked(h.config.Server.Metrics.Enabled),
		enableMetricsEndpointLabel,
		endpointPathLabel,
		h.config.Server.Metrics.Endpoint,
		endpointPathPlaceholder,
		checked(h.config.Server.Metrics.IncludeSystem),
		includeSystemMetricsLabel,
		bearerTokenLabel,
		h.config.Server.Metrics.Token,
		bearerTokenPlaceholder,
		bearerTokenHelpLabel,
		saveMetricsSettingsLabel,
		availableMetricsLabel,
		metricLabel,
		typeLabel,
		descriptionLabel,
		counterLabel,
		totalSearchRequestsLabel,
		histogramLabel,
		requestDurationLabel,
		counterLabel,
		requestsPerEngineLabel,
		counterLabel,
		errorsPerEngineLabel,
		histogramLabel,
		resultsReturnedLabel,
		counterLabel,
		cacheHitCountLabel,
		counterLabel,
		cacheMissCountLabel,
		counterLabel,
		cpuTimeUsedLabel,
		gaugeLabel,
		memoryUsageLabel,
	)
}

// renderSchedulerContent renders the scheduler management page
func (h *Handler) renderSchedulerContent(w io.Writer, data *AdminPageData) {
	scheduledTasksLabel := adminDataString(data, "admin.scheduler_page.title", "Scheduled Tasks")
	schedulerDescriptionLabel := adminDataString(data, "admin.scheduler_page.description", "Manage background tasks that run on a schedule.")
	taskLabel := adminDataString(data, "admin.scheduler_page.task", "Task")
	scheduleLabel := adminDataString(data, "admin.scheduler_page.schedule", "Schedule")
	lastRunLabel := adminDataString(data, "admin.scheduler_page.last_run", "Last Run")
	nextRunLabel := adminDataString(data, "admin.scheduler_page.next_run", "Next Run")
	statusLabel := adminDataString(data, "health.status", "Status")
	actionsLabel := adminDataString(data, "admin.scheduler_page.actions", "Actions")
	runNowLabel := adminDataString(data, "admin.scheduler_page.run_now", "Run Now")
	taskHistoryLabel := adminDataString(data, "admin.scheduler_page.history_title", "Task History")
	timeLabel := adminDataString(data, "admin.scheduler_page.time", "Time")
	durationLabel := adminDataString(data, "admin.scheduler_page.duration", "Duration")
	resultLabel := adminDataString(data, "admin.scheduler_page.result", "Result")
	loadingHistoryLabel := adminDataString(data, "admin.scheduler_page.loading_history", "Loading history...")
	noHistoryLabel := adminDataString(data, "admin.scheduler_page.no_history", "No task history available")
	failedLoadHistoryLabel := adminDataString(data, "admin.scheduler_page.failed_load_history", "Failed to load history")
	runConfirmTemplate := adminDataStringRaw(data, "admin.scheduler_page.run_confirm_message", "Run task \"%s\" now?")
	runConfirmTitle := adminDataString(data, "admin.scheduler_page.run_confirm_title", "Run Scheduled Task")
	cancelLabel := adminDataString(data, "common.cancel", "Cancel")
	taskStartedLabel := adminDataString(data, "admin.scheduler_page.task_started", "Task started successfully")
	errorLabel := adminDataString(data, "common.error", "Error")
	unknownErrorLabel := adminDataString(data, "admin.scheduler_page.unknown_error", "Unknown error")
	successLabel := adminDataString(data, "common.success", "Success")
	failedLabel := adminDataString(data, "admin.scheduler_page.failed", "Failed")
	backupLabel := adminDataString(data, "admin.scheduler_page.backup", "Backup")
	backupDescriptionLabel := adminDataString(data, "admin.scheduler_page.backup_description", "Create automatic backups of configuration and data")
	backupScheduleLabel := adminDataString(data, "admin.scheduler_page.backup_schedule", "Daily at 3:00 AM")
	cacheCleanupLabel := adminDataString(data, "admin.scheduler_page.cache_cleanup", "Cache Cleanup")
	cacheCleanupDescriptionLabel := adminDataString(data, "admin.scheduler_page.cache_cleanup_description", "Remove expired cache entries")
	cacheCleanupScheduleLabel := adminDataString(data, "admin.scheduler_page.cache_cleanup_schedule", "Every 6 hours")
	logRotationLabel := adminDataString(data, "admin.scheduler_page.log_rotation", "Log Rotation")
	logRotationDescriptionLabel := adminDataString(data, "admin.scheduler_page.log_rotation_description", "Rotate and compress old log files")
	logRotationScheduleLabel := adminDataString(data, "admin.scheduler_page.log_rotation_schedule", "Weekly on Sunday")
	geoIPUpdateLabel := adminDataString(data, "admin.scheduler_page.geoip_update", "GeoIP Update")
	geoIPUpdateDescriptionLabel := adminDataString(data, "admin.scheduler_page.geoip_update_description", "Download latest GeoIP database")
	geoIPUpdateScheduleLabel := adminDataString(data, "admin.scheduler_page.geoip_update_schedule", "Weekly on Wednesday")
	engineHealthCheckLabel := adminDataString(data, "admin.scheduler_page.engine_health", "Engine Health Check")
	engineHealthCheckDescriptionLabel := adminDataString(data, "admin.scheduler_page.engine_health_description", "Verify all search engines are responding")
	engineHealthCheckScheduleLabel := adminDataString(data, "admin.scheduler_page.engine_health_schedule", "Every 15 minutes")
	noHistoryHTML := fmt.Sprintf(`<tr><td colspan="4" class="empty-message">%s</td></tr>`, noHistoryLabel)
	failedHistoryHTML := fmt.Sprintf(`<tr><td colspan="4" class="empty-message">%s</td></tr>`, failedLoadHistoryLabel)

	fmt.Fprintf(w, `
            <div class="admin-section">
                <h2>%s</h2>
                <p class="desc-text">%s</p>

                <table class="admin-table">
                    <thead>
                        <tr>
                            <th>%s</th>
                            <th>%s</th>
                            <th>%s</th>
                            <th>%s</th>
                            <th>%s</th>
                            <th>%s</th>
                        </tr>
                    </thead>
                    <tbody>
                        <tr>
                            <td><strong>%s</strong><br><small class="text-secondary">%s</small></td>
                            <td><code>0 3 * * *</code><br><small>%s</small></td>
                            <td>%s</td>
                            <td>%s</td>
                            <td><span class="status-badge %s">%s</span></td>
                            <td>
                                <button class="btn btn-xs" onclick="runTask('backup')">%s</button>
                            </td>
                        </tr>
                        <tr>
                            <td><strong>%s</strong><br><small class="text-secondary">%s</small></td>
                            <td><code>0 */6 * * *</code><br><small>%s</small></td>
                            <td>%s</td>
                            <td>%s</td>
                            <td><span class="status-badge %s">%s</span></td>
                            <td>
                                <button class="btn btn-xs" onclick="runTask('cache_cleanup')">%s</button>
                            </td>
                        </tr>
                        <tr>
                            <td><strong>%s</strong><br><small class="text-secondary">%s</small></td>
                            <td><code>0 0 * * 0</code><br><small>%s</small></td>
                            <td>%s</td>
                            <td>%s</td>
                            <td><span class="status-badge %s">%s</span></td>
                            <td>
                                <button class="btn btn-xs" onclick="runTask('log_rotation')">%s</button>
                            </td>
                        </tr>
                        <tr>
                            <td><strong>%s</strong><br><small class="text-secondary">%s</small></td>
                            <td><code>0 4 * * 3</code><br><small>%s</small></td>
                            <td>%s</td>
                            <td>%s</td>
                            <td><span class="status-badge %s">%s</span></td>
                            <td>
                                <button class="btn btn-xs" onclick="runTask('geoip_update')">%s</button>
                            </td>
                        </tr>
                        <tr>
                            <td><strong>%s</strong><br><small class="text-secondary">%s</small></td>
                            <td><code>*/15 * * * *</code><br><small>%s</small></td>
                            <td>%s</td>
                            <td>%s</td>
                            <td><span class="status-badge %s">%s</span></td>
                            <td>
                                <button class="btn btn-xs" onclick="runTask('engine_health')">%s</button>
                            </td>
                        </tr>
                    </tbody>
                </table>
            </div>

            <div class="admin-section">
                <h2>%s</h2>
                <div id="task-history">
                    <table class="admin-table">
                        <thead>
                            <tr>
                                <th>%s</th>
                                <th>%s</th>
                                <th>%s</th>
                                <th>%s</th>
                            </tr>
                        </thead>
                        <tbody id="history-body">
                            <tr><td colspan="4" class="empty-message">%s</td></tr>
                        </tbody>
                    </table>
                </div>
            </div>

            <script>
            async function runTask(taskName) {
                const confirmed = await showConfirm(%q.replace('%%s', taskName), {
                    title: %q,
                    confirmText: %q,
                    cancelText: %q
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
                        showToast(%q, 'success');
                        location.reload();
                    } else {
                        showToast(%q + ': ' + (data.error || %q), 'error');
                    }
                })
                .catch(err => showToast(%q + ': ' + err, 'error'));
            }

            // Load task history
            document.addEventListener('DOMContentLoaded', function() {
                fetch('/api/v1/admin/scheduler?history=true')
                .then(r => r.json())
                .then(data => {
                    const tbody = document.getElementById('history-body');
                    if (!data.history || data.history.length === 0) {
                        tbody.innerHTML = %q;
                        return;
                    }
                    tbody.innerHTML = data.history.map(h =>
                        '<tr><td>' + h.time + '</td><td>' + h.task + '</td><td>' + h.duration + '</td><td><span class="status-badge ' + (h.success ? 'enabled' : 'disabled') + '">' + (h.success ? %q : %q) + '</span></td></tr>'
                    ).join('');
                })
                .catch(() => {
                    document.getElementById('history-body').innerHTML = %q;
                });
            });
            </script>`,
		scheduledTasksLabel,
		schedulerDescriptionLabel,
		taskLabel,
		scheduleLabel,
		lastRunLabel,
		nextRunLabel,
		statusLabel,
		actionsLabel,
		backupLabel,
		backupDescriptionLabel,
		backupScheduleLabel,
		// Backup task
		formatTaskTime(data, data.SchedulerTasks["backup"].LastRun),
		formatTaskTime(data, data.SchedulerTasks["backup"].NextRun),
		taskStatusClass(data.SchedulerTasks["backup"].Enabled),
		taskStatusText(data, data.SchedulerTasks["backup"].Enabled),
		runNowLabel,
		cacheCleanupLabel,
		cacheCleanupDescriptionLabel,
		cacheCleanupScheduleLabel,
		// Cache cleanup task
		formatTaskTime(data, data.SchedulerTasks["cache_cleanup"].LastRun),
		formatTaskTime(data, data.SchedulerTasks["cache_cleanup"].NextRun),
		taskStatusClass(data.SchedulerTasks["cache_cleanup"].Enabled),
		taskStatusText(data, data.SchedulerTasks["cache_cleanup"].Enabled),
		runNowLabel,
		logRotationLabel,
		logRotationDescriptionLabel,
		logRotationScheduleLabel,
		// Log rotation task
		formatTaskTime(data, data.SchedulerTasks["log_rotation"].LastRun),
		formatTaskTime(data, data.SchedulerTasks["log_rotation"].NextRun),
		taskStatusClass(data.SchedulerTasks["log_rotation"].Enabled),
		taskStatusText(data, data.SchedulerTasks["log_rotation"].Enabled),
		runNowLabel,
		geoIPUpdateLabel,
		geoIPUpdateDescriptionLabel,
		geoIPUpdateScheduleLabel,
		// GeoIP update task
		formatTaskTime(data, data.SchedulerTasks["geoip_update"].LastRun),
		formatTaskTime(data, data.SchedulerTasks["geoip_update"].NextRun),
		taskStatusClass(data.SchedulerTasks["geoip_update"].Enabled),
		taskStatusText(data, data.SchedulerTasks["geoip_update"].Enabled),
		runNowLabel,
		engineHealthCheckLabel,
		engineHealthCheckDescriptionLabel,
		engineHealthCheckScheduleLabel,
		// Engine health check task
		formatTaskTime(data, data.SchedulerTasks["engine_health"].LastRun),
		formatTaskTime(data, data.SchedulerTasks["engine_health"].NextRun),
		taskStatusClass(data.SchedulerTasks["engine_health"].Enabled),
		taskStatusText(data, data.SchedulerTasks["engine_health"].Enabled),
		runNowLabel,
		taskHistoryLabel,
		timeLabel,
		taskLabel,
		durationLabel,
		resultLabel,
		loadingHistoryLabel,
		runConfirmTemplate,
		runConfirmTitle,
		runNowLabel,
		cancelLabel,
		taskStartedLabel,
		errorLabel,
		unknownErrorLabel,
		errorLabel,
		noHistoryHTML,
		successLabel,
		failedLabel,
		failedHistoryHTML,
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

func formatAnnouncementDate(data *AdminPageData, dateStr string) string {
	if dateStr == "" {
		return adminDataString(data, "admin.announcements_page.always", "Always")
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

func formatTaskTime(data *AdminPageData, t time.Time) string {
	if t.IsZero() {
		return adminDataString(data, "admin.scheduler_page.never", "Never")
	}
	return t.Format("Jan 2, 15:04")
}

func taskStatusClass(enabled bool) string {
	if enabled {
		return "enabled"
	}
	return "disabled"
}

func taskStatusText(data *AdminPageData, enabled bool) string {
	if enabled {
		return adminDataString(data, "common.enabled", "Enabled")
	}
	return adminDataString(data, "common.disabled", "Disabled")
}

// ============================================================
// Multi-Admin Templates per AI.md PART 31
// ============================================================

// renderSetupContent renders the initial setup page
func (h *Handler) renderSetupContent(w io.Writer, data *AdminPageData) {
	setupTokenRequired := false
	if data.Extra != nil {
		if v, ok := data.Extra["SetupTokenRequired"].(bool); ok {
			setupTokenRequired = v
		}
	}

	setupTokenLabel := adminDataString(data, "admin.setup_page.setup_token", "Setup Token")
	setupTokenPlaceholder := adminDataString(data, "admin.setup_page.setup_token_placeholder", "Enter the setup token from console")
	setupTokenHelp := adminDataString(data, "admin.setup_page.setup_token_help", "The setup token was displayed in the server console. Use --maintenance setup to regenerate.")
	createAdminAccountLabel := adminDataString(data, "admin.setup_page.create_admin_account", "Create Admin Account")
	setupDescription := adminDataString(data, "admin.setup_page.description", "Welcome! Create the primary administrator account to get started.")
	usernameHelp := adminDataString(data, "admin.setup_page.username_help", "3-32 characters, lowercase letters, numbers, underscore, hyphen")
	usernamePlaceholder := adminDataString(data, "admin.setup_page.username_placeholder", "admin")
	emailPlaceholder := adminDataString(data, "admin.setup_page.email_placeholder", "admin@example.com")
	passwordPlaceholder := adminDataString(data, "admin.setup_page.password_placeholder", "Min. 8 characters")
	confirmPasswordPlaceholder := adminDataString(data, "admin.setup_page.confirm_password_placeholder", "Repeat password")

	tokenField := ""
	if setupTokenRequired {
		tokenField = fmt.Sprintf(`
                    <div class="form-row">
                        <label>%s</label>
                        <input type="text" name="setup_token" required placeholder="%s">
                        <p class="help-text">
                            %s
                        </p>
                    </div>`,
			setupTokenLabel,
			setupTokenPlaceholder,
			setupTokenHelp,
		)
	}

	fmt.Fprintf(w, `
            <div class="admin-section">
                <h2>%s</h2>
                <p class="desc-text">
                    %s
                </p>
                <form method="POST" action="/admin/setup">
                    <input type="hidden" name="csrf_token" value="%s">
                    %s
                    <div class="form-row">
                        <label>%s</label>
                        <input type="text" name="username" required pattern="[a-z][a-z0-9_-]{2,31}"
                               placeholder="%s" autocomplete="username">
                        <p class="help-text">
                            %s
                        </p>
                    </div>
                    <div class="form-row">
                        <label>%s</label>
                        <input type="email" name="email" placeholder="%s" autocomplete="email">
                    </div>
                    <div class="form-row">
                        <label>%s</label>
                        <input type="password" name="password" required minlength="8"
                               placeholder="%s" autocomplete="new-password">
                    </div>
                    <div class="form-row">
                        <label>%s</label>
                        <input type="password" name="confirm_password" required minlength="8"
                               placeholder="%s" autocomplete="new-password">
                    </div>
                    <button type="submit" class="btn">%s</button>
                </form>
            </div>`,
		createAdminAccountLabel,
		setupDescription,
		data.CSRFToken,
		tokenField,
		adminDataString(data, "auth.username", "Username"),
		usernamePlaceholder,
		usernameHelp,
		adminDataString(data, "auth.email", "Email"),
		emailPlaceholder,
		adminDataString(data, "auth.password", "Password"),
		passwordPlaceholder,
		adminDataString(data, "auth.confirm_password", "Confirm Password"),
		confirmPasswordPlaceholder,
		createAdminAccountLabel,
	)
}

// renderAdminsContent renders the server admins management page
func (h *Handler) renderAdminsContent(w io.Writer, data *AdminPageData) {
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

	if data.Success != "" {
		inviteURL = data.Success
	}

	inviteCreatedLabel := adminDataString(data, "admin.admins_page.invite_created", "Invite Created")
	shareInviteLabel := adminDataString(data, "admin.admins_page.share_invite", "Share this link with the new admin:")
	inviteWarningLabel := adminDataString(data, "admin.admins_page.invite_warning", "This link expires in 7 days and can only be used once.")
	inviteNewAdminLabel := adminDataString(data, "admin.admins_page.invite_new_admin", "Invite New Admin")
	suggestedUsernameLabel := adminDataString(data, "admin.admins_page.suggested_username_optional", "Suggested Username (optional)")
	suggestedUsernamePlaceholder := adminDataString(data, "admin.admins_page.suggested_username_placeholder", "Leave empty for invite to choose")
	createInviteLinkLabel := adminDataString(data, "admin.admins_page.create_invite_link", "Create Invite Link")
	serverAdminsLabel := adminDataString(data, "admin.server_admins", "Server Admins")
	totalLabel := adminDataString(data, "admin.admins_page.total", "total")
	roleHeaderLabel := adminDataString(data, "admin.admins_page.role", "Role")
	sourceHeaderLabel := adminDataString(data, "admin.admins_page.source", "Source")
	twoFactorHeaderLabel := adminDataString(data, "admin.admins_page.two_factor", "2FA")
	lastLoginHeaderLabel := adminDataString(data, "admin.admins_page.last_login", "Last Login")
	actionsHeaderLabel := adminDataString(data, "admin.admins_page.actions", "Actions")
	noAdminsLabel := adminDataString(data, "admin.admins_page.no_admins", "No admins to display")
	adminRoleLabel := adminDataString(data, "admin.admins_page.admin_role", "Admin")
	primaryRoleLabel := adminDataString(data, "admin.admins_page.primary_role", "Primary")
	neverLabel := adminDataString(data, "admin.admins_page.never", "Never")
	deleteLabel := adminDataString(data, "common.delete", "Delete")
	deleteConfirmMessage := adminDataString(data, "admin.admins_page.delete_confirm_message", "Delete this admin account? This action cannot be undone.")
	deleteConfirmTitle := adminDataString(data, "admin.admins_page.delete_confirm_title", "Delete Admin")
	cancelLabel := adminDataString(data, "common.cancel", "Cancel")
	privacyNoteLabel := adminDataString(data, "admin.admins_page.privacy_note", "Note: For privacy, you can only view your own admin account. The primary admin can see all admins.")

	inviteSection := ""
	if inviteURL != "" && isPrimary {
		inviteSection = fmt.Sprintf(`
            <div class="admin-section border-success">
                <h2 class="text-success">%s</h2>
                <p>%s</p>
                <div class="token-display">%s</div>
                <p class="token-warning">%s</p>
            </div>`,
			inviteCreatedLabel,
			shareInviteLabel,
			inviteURL,
			inviteWarningLabel,
		)
	}

	createInviteForm := ""
	if isPrimary {
		createInviteForm = fmt.Sprintf(`
            <div class="admin-section">
                <h2>%s</h2>
                <form method="POST" action="/admin/users/admins/invite">
                    <input type="hidden" name="csrf_token" value="%s">
                    <div class="form-row">
                        <label>%s</label>
                        <input type="text" name="username" placeholder="%s">
                    </div>
                    <button type="submit" class="btn">%s</button>
                </form>
            </div>`,
			inviteNewAdminLabel,
			data.CSRFToken,
			suggestedUsernameLabel,
			suggestedUsernamePlaceholder,
			createInviteLinkLabel,
		)
	}

	fmt.Fprintf(w, `
            %s
            %s
            <div class="admin-section">
                <h2>%s (%d %s)</h2>
                <table class="admin-table">
                    <thead>
                        <tr>
                            <th>%s</th>
                            <th>%s</th>
                            <th>%s</th>
                            <th>%s</th>
                            <th>%s</th>
                            <th>%s</th>
                            <th>%s</th>
                        </tr>
                    </thead>
                    <tbody>`,
		inviteSection,
		createInviteForm,
		serverAdminsLabel,
		totalCount,
		totalLabel,
		adminDataString(data, "auth.username", "Username"),
		adminDataString(data, "auth.email", "Email"),
		roleHeaderLabel,
		sourceHeaderLabel,
		twoFactorHeaderLabel,
		lastLoginHeaderLabel,
		actionsHeaderLabel,
	)

	if len(admins) == 0 {
		fmt.Fprintf(w, `
                        <tr>
                            <td colspan="7" class="empty-message">%s</td>
                        </tr>`, noAdminsLabel)
	} else {
		for _, admin := range admins {
			role := adminRoleLabel
			if admin.IsPrimary {
				role = primaryRoleLabel
			}
			lastLogin := neverLabel
			if admin.LastLoginAt != nil {
				lastLogin = admin.LastLoginAt.Format("2006-01-02 15:04")
			}
			totpStatus := adminDataString(data, "common.disabled", "Disabled")
			totpClass := "disabled"
			if admin.TOTPEnabled {
				totpStatus = adminDataString(data, "common.enabled", "Enabled")
				totpClass = "enabled"
			}

			actionButtons := ""
			if isPrimary && !admin.IsPrimary {
				actionButtons = fmt.Sprintf(`
                                <form id="delete-admin-%d" method="POST" action="/admin/users/admins/%d/delete" class="form-inline">
                                    <input type="hidden" name="csrf_token" value="%s">
                                    <button type="button" class="btn btn-danger btn-sm"
                                            onclick="confirmDeleteAdmin(%d)">%s</button>
                                </form>`,
					admin.ID, admin.ID, data.CSRFToken, admin.ID, deleteLabel,
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
                const confirmed = await showConfirm(%q, {
                    title: %q,
                    confirmText: %q,
                    cancelText: %q,
                    danger: true
                });
                if (confirmed) {
                    document.getElementById('delete-admin-' + adminId).submit();
                }
            }
            </script>`,
		deleteConfirmMessage,
		deleteConfirmTitle,
		deleteLabel,
		cancelLabel,
	)

	if !isPrimary {
		fmt.Fprintf(w, `
            <div class="admin-section">
                <p class="text-secondary">
                    <em>%s</em>
                </p>
            </div>`,
			privacyNoteLabel,
		)
	}
}

// renderInviteAcceptContent renders the invite acceptance page
func (h *Handler) renderInviteAcceptContent(w io.Writer, data *AdminPageData) {
	suggestedUsername := ""
	if data.Extra != nil {
		if v, ok := data.Extra["SuggestedUsername"].(string); ok {
			suggestedUsername = v
		}
	}

	titleLabel := adminDataString(data, "admin.invite_accept_page.title", "Accept Admin Invite")
	descriptionLabel := adminDataString(data, "admin.invite_accept_page.description", "You've been invited to become a server administrator. Create your account below.")
	usernameHelp := adminDataString(data, "admin.setup_page.username_help", "3-32 characters, lowercase letters, numbers, underscore, hyphen")
	usernamePlaceholder := adminDataString(data, "admin.setup_page.username_placeholder", "admin")
	emailPlaceholder := adminDataString(data, "admin.setup_page.email_placeholder", "admin@example.com")
	passwordPlaceholder := adminDataString(data, "admin.setup_page.password_placeholder", "Min. 8 characters")
	confirmPasswordPlaceholder := adminDataString(data, "admin.setup_page.confirm_password_placeholder", "Repeat password")
	createAdminAccountLabel := adminDataString(data, "admin.setup_page.create_admin_account", "Create Admin Account")

	fmt.Fprintf(w, `
            <div class="admin-section">
                <h2>%s</h2>
                <p class="desc-text">
                    %s
                </p>
                <form method="POST">
                    <input type="hidden" name="csrf_token" value="%s">
                    <div class="form-row">
                        <label>%s</label>
                        <input type="text" name="username" required value="%s" pattern="[a-z][a-z0-9_-]{2,31}"
                               placeholder="%s" autocomplete="username">
                        <p class="help-text">
                            %s
                        </p>
                    </div>
                    <div class="form-row">
                        <label>%s</label>
                        <input type="email" name="email" placeholder="%s" autocomplete="email">
                    </div>
                    <div class="form-row">
                        <label>%s</label>
                        <input type="password" name="password" required minlength="8"
                               placeholder="%s" autocomplete="new-password">
                    </div>
                    <div class="form-row">
                        <label>%s</label>
                        <input type="password" name="confirm_password" required minlength="8"
                               placeholder="%s" autocomplete="new-password">
                    </div>
                    <button type="submit" class="btn">%s</button>
                </form>
            </div>`,
		titleLabel,
		descriptionLabel,
		data.CSRFToken,
		adminDataString(data, "auth.username", "Username"),
		suggestedUsername,
		usernamePlaceholder,
		usernameHelp,
		adminDataString(data, "auth.email", "Email"),
		emailPlaceholder,
		adminDataString(data, "auth.password", "Password"),
		passwordPlaceholder,
		adminDataString(data, "auth.confirm_password", "Confirm Password"),
		confirmPasswordPlaceholder,
		createAdminAccountLabel,
	)
}

// renderInviteErrorContent renders the invite error page
func (h *Handler) renderInviteErrorContent(w io.Writer, data *AdminPageData) {
	fmt.Fprintf(w, `
            <div class="admin-section text-center">
                <h2 class="text-danger">%s</h2>
                <p class="text-secondary my-20">
                    %s
                </p>
                <p class="text-secondary">
                    %s
                </p>
                <a href="/admin/login" class="btn mt-20">%s</a>
            </div>`,
		adminDataString(data, "admin.invite_error_page.title", "Invalid Invite"),
		data.Error,
		adminDataString(data, "admin.invite_error_page.description", "Please contact the server administrator for a new invite link."),
		adminDataString(data, "admin.invite_error_page.go_to_login", "Go to Login"),
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
func (h *Handler) renderNodesContent(w io.Writer, data *AdminPageData) {
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
		joinTokenGeneratedLabel := adminDataString(data, "admin.nodes_page.join_token_generated", "Join Token Generated")
		joinTokenHelpLabel := adminDataString(data, "admin.nodes_page.join_token_help", "Use this token to join a new node to the cluster:")
		joinTokenWarningLabel := adminDataString(data, "admin.nodes_page.join_token_warning", "This token expires in 24 hours and can only be used once.")
		tokenSection = fmt.Sprintf(`
            <div class="admin-section border-success">
                <h2 class="text-success">%s</h2>
                <p>%s</p>
                <div class="token-display">%s</div>
                <p class="token-warning">%s</p>
            </div>`,
			joinTokenGeneratedLabel,
			joinTokenHelpLabel,
			token,
			joinTokenWarningLabel,
		)
	}

	// Mode info section
	modeClass := "enabled"
	modeText := adminDataString(data, "admin.nodes_page.cluster_mode", "Cluster Mode")
	if !isCluster {
		modeClass = "disabled"
		modeText = adminDataString(data, "admin.nodes_page.standalone_mode", "Standalone Mode")
	}

	clusterStatusLabel := adminDataString(data, "admin.nodes_page.cluster_status", "Cluster Status")
	modeLabel := adminDataString(data, "admin.nodes_page.mode", "Mode")
	thisNodeLabel := adminDataString(data, "admin.nodes_page.this_node", "This Node")
	hostnameLabel := adminDataString(data, "admin.nodes_page.hostname", "Hostname")
	nodeIDLabel := adminDataString(data, "admin.nodes_page.node_id", "Node ID")
	roleLabel := adminDataString(data, "admin.nodes_page.role", "Role")
	roleBadgeClass := ""
	roleText := adminDataString(data, "admin.nodes_page.secondary", "Secondary")
	if isPrimary {
		roleBadgeClass = "enabled"
		roleText = adminDataString(data, "admin.nodes_page.primary", "Primary")
	}

	fmt.Fprintf(w, `
            %s
            <div class="admin-section">
                <h2>%s</h2>
                <table class="admin-table max-w-400">
                    <tr>
                        <td class="text-secondary">%s</td>
                        <td><span class="status-badge %s">%s</span></td>
                    </tr>
                    <tr>
                        <td class="text-secondary">%s</td>
                        <td>%s</td>
                    </tr>
                    <tr>
                        <td class="text-secondary">%s</td>
                        <td>%s</td>
                    </tr>
                    <tr>
                        <td class="text-secondary">%s</td>
                        <td><span class="status-badge %s">%s</span></td>
                    </tr>
                </table>
            </div>`,
		tokenSection,
		clusterStatusLabel,
		modeLabel,
		modeClass, modeText,
		nodeIDLabel,
		nodeID,
		hostnameLabel,
		hostname,
		roleLabel,
		roleBadgeClass,
		roleText,
	)

	// Actions section (cluster mode only)
	if isCluster {
		actionsSection := ""
		if isPrimary {
			clusterActionsLabel := adminDataString(data, "admin.nodes_page.cluster_actions", "Cluster Actions")
			generateJoinTokenLabel := adminDataString(data, "admin.nodes_page.generate_join_token", "Generate Join Token")
			actionsSection = fmt.Sprintf(`
            <div class="admin-section">
                <h2>%s</h2>
                <form method="POST" action="/admin/server/nodes/token" class="d-inline-block mr-10">
                    <input type="hidden" name="csrf_token" value="%s">
                    <button type="submit" class="btn">%s</button>
                </form>
            </div>`, clusterActionsLabel, data.CSRFToken, generateJoinTokenLabel)
		} else {
			nodeActionsLabel := adminDataString(data, "admin.nodes_page.node_actions", "Node Actions")
			leaveClusterLabel := adminDataString(data, "admin.nodes_page.leave_cluster", "Leave Cluster")
			leaveConfirmMessage := adminDataString(data, "admin.nodes_page.leave_confirm_message", "Leave the cluster? This node will become standalone.")
			leaveConfirmTitle := adminDataString(data, "admin.nodes_page.leave_confirm_title", "Leave Cluster")
			leaveLabel := adminDataString(data, "admin.nodes_page.leave", "Leave")
			cancelLabel := adminDataString(data, "common.cancel", "Cancel")
			actionsSection = fmt.Sprintf(`
            <div class="admin-section">
                <h2>%s</h2>
                <form id="leave-cluster-form" method="POST" action="/admin/server/nodes/leave">
                    <input type="hidden" name="csrf_token" value="%s">
                    <button type="button" class="btn btn-danger" onclick="confirmLeaveCluster()">%s</button>
                </form>
            </div>
            <script>
            async function confirmLeaveCluster() {
                const confirmed = await showConfirm(%q, {
                    title: %q,
                    confirmText: %q,
                    cancelText: %q,
                    danger: true
                });
                if (confirmed) {
                    document.getElementById('leave-cluster-form').submit();
                }
            }
            </script>`, nodeActionsLabel, data.CSRFToken, leaveClusterLabel, leaveConfirmMessage, leaveConfirmTitle, leaveLabel, cancelLabel)
		}
		fmt.Fprintf(w, "%s", actionsSection)
	}

	clusterNodesLabel := adminDataString(data, "admin.nodes_page.cluster_nodes", "Cluster Nodes")
	lastSeenLabel := adminDataString(data, "admin.nodes_page.last_seen", "Last Seen")
	joinedLabel := adminDataString(data, "admin.nodes_page.joined", "Joined")
	statusColumnLabel := adminDataString(data, "health.status", "Status")

	// Nodes table
	fmt.Fprintf(w, `
            <div class="admin-section">
                <h2>%s (%d)</h2>
                <table class="admin-table">
                    <thead>
                        <tr>
                            <th>%s</th>
                            <th>%s</th>
                            <th>%s</th>
                            <th>%s</th>
                            <th>%s</th>
                            <th>%s</th>
                        </tr>
                    </thead>
                    <tbody>`,
		clusterNodesLabel,
		len(nodes),
		hostnameLabel,
		thisNodeLabel,
		roleLabel,
		statusColumnLabel,
		lastSeenLabel,
		joinedLabel,
	)

	if len(nodes) == 0 {
		noNodesLabel := adminDataString(data, "admin.nodes_page.no_nodes", "No nodes found")
		fmt.Fprintf(w, `
                        <tr>
                            <td colspan="6" class="empty-message">%s</td>
                        </tr>`, noNodesLabel)
	} else {
		for _, node := range nodes {
			role := adminDataString(data, "admin.nodes_page.secondary", "Secondary")
			roleClass := ""
			if node.IsPrimary {
				role = adminDataString(data, "admin.nodes_page.primary", "Primary")
				roleClass = "enabled"
			}

			statusClass := nodeStatusClass(node.Status)
			statusText := nodeStatusText(data, node.Status)

			thisNode := ""
			if node.ID == nodeID {
				thisNode = adminDataString(data, "admin.nodes_page.this_node_suffix", " (this node)")
			}

			lastSeen := formatNodeTime(data, node.LastSeen)
			joinedAt := formatNodeDate(data, node.JoinedAt)

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
				statusClass, statusText,
				lastSeen,
				joinedAt,
			)
		}
	}

	fmt.Fprintf(w, `
                    </tbody>
                </table>
            </div>`)

	if !isCluster {
		clusterModeNoteLabel := adminDataString(data, "admin.nodes_page.cluster_mode_note", "Note: Cluster mode requires a remote database (PostgreSQL or MySQL). In standalone mode, only this node is shown.")
		fmt.Fprintf(w, `
            <div class="admin-section">
                <p class="text-secondary">
                    <em>%s</em>
                </p>
            </div>`, clusterModeNoteLabel)
	}
}

func formatNodeTime(data *AdminPageData, t time.Time) string {
	if t.IsZero() {
		return adminDataString(data, "admin.nodes_page.never", "Never")
	}
	return t.Format("Jan 2, 15:04")
}

func formatNodeDate(data *AdminPageData, t time.Time) string {
	if t.IsZero() {
		return adminDataString(data, "admin.nodes_page.never", "Never")
	}
	return t.Format("Jan 2, 2006")
}

func nodeStatusClass(status string) string {
	switch strings.ToLower(status) {
	case "online", "healthy":
		return "enabled"
	case "degraded":
		return ""
	default:
		return "disabled"
	}
	return "enabled"
}

func nodeStatusText(data *AdminPageData, status string) string {
	switch strings.ToLower(status) {
	case "online":
		return adminDataString(data, "admin.nodes_page.online", "Online")
	case "healthy":
		return adminDataString(data, "admin.nodes_page.healthy", "Healthy")
	case "degraded":
		return adminDataString(data, "admin.nodes_page.degraded", "Degraded")
	case "offline":
		return adminDataString(data, "admin.nodes_page.offline", "Offline")
	case "removed":
		return adminDataString(data, "admin.nodes_page.removed", "Removed")
	default:
		return status
	}
}

// renderServerBackupContent renders the backup management page content
func (h *Handler) renderServerBackupContent(w io.Writer, data *AdminPageData) {
	createBackupLabel := adminDataString(data, "admin.backup_page.create_backup", "Create Backup")
	backupDescriptionLabel := adminDataString(data, "admin.backup_page.description", "Create a backup of your database, configuration, and data files.")
	createBackupNowLabel := adminDataString(data, "admin.backup_page.create_backup_now", "Create Backup Now")
	availableBackupsLabel := adminDataString(data, "admin.backup_page.available_backups", "Available Backups")
	filenameLabel := adminDataString(data, "admin.backup_page.filename", "Filename")
	sizeLabel := adminDataString(data, "admin.backup_page.size", "Size")
	createdLabel := adminDataString(data, "admin.backup_page.created", "Created")
	actionsLabel := adminDataString(data, "admin.backup_page.actions", "Actions")
	noBackupsLabel := adminDataString(data, "admin.backup_page.no_backups", "No backups available")
	backupSettingsLabel := adminDataString(data, "admin.backup_page.settings", "Backup Settings")
	automaticBackupsLabel := adminDataString(data, "admin.backup_page.automatic_backups", "Automatic Backups")
	dailyAt0200Label := adminDataString(data, "admin.backup_page.daily_at_0200", "Daily at 02:00")
	weeklyOnSundayLabel := adminDataString(data, "admin.backup_page.weekly_on_sunday", "Weekly on Sunday")
	disabledLabel := adminDataString(data, "common.disabled", "Disabled")
	maxBackupsLabel := adminDataString(data, "admin.backup_page.max_backups", "Maximum Backups to Keep")
	saveSettingsLabel := adminDataString(data, "admin.backup_page.save_settings", "Save Settings")

	fmt.Fprintf(w, `
            <div class="admin-section">
                <h2>%s</h2>
                <p class="desc-text mb-16">
                    %s
                </p>
                <form method="POST" action="/admin/server/backup">
                    <input type="hidden" name="csrf_token" value="%s">
                    <input type="hidden" name="action" value="create">
                    <button type="submit" class="btn">%s</button>
                </form>
            </div>

            <div class="admin-section">
                <h2>%s</h2>
                <table class="admin-table">
                    <thead>
                        <tr>
                            <th>%s</th>
                            <th>%s</th>
                            <th>%s</th>
                            <th>%s</th>
                        </tr>
                    </thead>
                    <tbody>
                        <tr>
                            <td colspan="4" class="empty-message">%s</td>
                        </tr>
                    </tbody>
                </table>
            </div>

            <div class="admin-section">
                <h2>%s</h2>
                <form method="POST" action="/admin/server/backup">
                    <input type="hidden" name="csrf_token" value="%s">
                    <input type="hidden" name="action" value="settings">
                    <div class="form-row">
                        <label>%s</label>
                        <select name="auto_backup">
                            <option value="daily">%s</option>
                            <option value="weekly">%s</option>
                            <option value="disabled">%s</option>
                        </select>
                    </div>
                    <div class="form-row">
                        <label>%s</label>
                        <input type="number" name="max_backups" value="4" min="1" max="30">
                    </div>
                    <button type="submit" class="btn">%s</button>
                </form>
            </div>`,
		createBackupLabel,
		backupDescriptionLabel,
		data.CSRFToken,
		createBackupNowLabel,
		availableBackupsLabel,
		filenameLabel,
		sizeLabel,
		createdLabel,
		actionsLabel,
		noBackupsLabel,
		backupSettingsLabel,
		data.CSRFToken,
		automaticBackupsLabel,
		dailyAt0200Label,
		weeklyOnSundayLabel,
		disabledLabel,
		maxBackupsLabel,
		saveSettingsLabel,
	)
}

// renderServerMaintenanceContent renders the maintenance mode page content
func (h *Handler) renderServerMaintenanceContent(w io.Writer, data *AdminPageData) {
	maintenanceEnabled := ""
	if data.Config.Server.MaintenanceMode {
		maintenanceEnabled = "checked"
	}

	maintenanceModeLabel := adminDataString(data, "admin.maintenance_page.title", "Maintenance Mode")
	maintenanceDescriptionLabel := adminDataString(data, "admin.maintenance_page.description", "When enabled, all users will see a maintenance page. Admins can still access the admin panel.")
	enableMaintenanceModeLabel := adminDataString(data, "admin.maintenance_page.enable_mode", "Enable Maintenance Mode")
	saveChangesLabel := adminDataString(data, "admin.maintenance_page.save_changes", "Save Changes")
	quickActionsLabel := adminDataString(data, "admin.maintenance_page.quick_actions", "Quick Actions")
	reloadConfigurationLabel := adminDataString(data, "admin.maintenance_page.reload_configuration", "Reload Configuration")
	createBackupLabel := adminDataString(data, "admin.backup_page.create_backup", "Create Backup")

	fmt.Fprintf(w, `
            <div class="admin-section">
                <h2>%s</h2>
                <p class="desc-text mb-16">
                    %s
                </p>
                <form method="POST" action="/admin/server/maintenance">
                    <input type="hidden" name="csrf_token" value="%s">
                    <div class="form-row">
                        <label>
                            <input type="checkbox" name="enabled" %s class="mr-8">
                            %s
                        </label>
                    </div>
                    <button type="submit" class="btn">%s</button>
                </form>
            </div>

            <div class="admin-section">
                <h2>%s</h2>
                <div class="d-flex gap-10 flex-wrap">
                    <a href="/api/v1/admin/reload" class="btn">%s</a>
                    <a href="/admin/server/backup" class="btn">%s</a>
                </div>
            </div>`,
		maintenanceModeLabel,
		maintenanceDescriptionLabel,
		data.CSRFToken,
		maintenanceEnabled,
		enableMaintenanceModeLabel,
		saveChangesLabel,
		quickActionsLabel,
		reloadConfigurationLabel,
		createBackupLabel,
	)
}

// renderServerUpdatesContent renders the updates page content
func (h *Handler) renderServerUpdatesContent(w io.Writer, data *AdminPageData) {
	currentVersionLabel := adminDataString(data, "admin.updates_page.current_version", "Current Version")
	versionLabel := adminDataString(data, "health.version", "Version")
	commitLabel := adminDataString(data, "admin.updates_page.commit", "Commit")
	buildDateLabel := adminDataString(data, "admin.updates_page.build_date", "Build Date")
	checkForUpdatesLabel := adminDataString(data, "admin.updates_page.check_for_updates", "Check for Updates")
	updatesDescriptionLabel := adminDataString(data, "admin.updates_page.description", "Check if a newer version is available.")
	clickToCheckLabel := adminDataString(data, "admin.updates_page.click_to_check", "Click the button below to check for updates.")
	checkingLabel := adminDataString(data, "admin.updates_page.checking", "Checking for updates...")
	updateAvailableLabel := adminDataString(data, "admin.updates_page.update_available", "Update available")
	updateNowLabel := adminDataString(data, "admin.updates_page.update_now", "Update Now")
	latestVersionLabel := adminDataString(data, "admin.updates_page.latest_version", "You are running the latest version.")
	errorCheckingLabel := adminDataString(data, "admin.updates_page.error_checking", "Error checking for updates.")

	fmt.Fprintf(w, `
            <div class="admin-section">
                <h2>%s</h2>
                <table class="admin-table">
                    <tr><td>%s</td><td>%s</td></tr>
                    <tr><td>%s</td><td>%s</td></tr>
                    <tr><td>%s</td><td>%s</td></tr>
                </table>
            </div>

            <div class="admin-section">
                <h2>%s</h2>
                <p class="desc-text mb-16">
                    %s
                </p>
                <div id="update-status" class="status-box">
                    %s
                </div>
                <button class="btn" onclick="checkUpdates()">%s</button>
            </div>

            <script>
            function checkUpdates() {
                document.getElementById('update-status').innerHTML = %q;
                fetch('/api/v1/admin/update/check', {
                    method: 'GET',
                    headers: {'Authorization': 'Bearer ' + document.cookie}
                })
                .then(r => r.json())
                .then(data => {
                    if (data.update_available) {
                        document.getElementById('update-status').innerHTML =
                            %q + ': ' + data.latest_version +
                            '<br><a href="/admin/server/updates?action=update" class="btn mt-10">' + %q + '</a>';
                    } else {
                        document.getElementById('update-status').innerHTML = %q;
                    }
                })
                .catch(e => {
                    document.getElementById('update-status').innerHTML = %q;
                });
            }
            </script>`,
		currentVersionLabel,
		versionLabel,
		config.Version,
		commitLabel,
		config.CommitID,
		buildDateLabel,
		config.BuildDate,
		checkForUpdatesLabel,
		updatesDescriptionLabel,
		clickToCheckLabel,
		checkForUpdatesLabel,
		checkingLabel,
		updateAvailableLabel,
		updateNowLabel,
		latestVersionLabel,
		errorCheckingLabel,
	)
}

// renderServerInfoContent renders the server info page content
func (h *Handler) renderServerInfoContent(w io.Writer, data *AdminPageData) {
	if data.Stats == nil {
		fmt.Fprintf(w, `<div class="admin-section"><p>%s</p></div>`, adminDataString(data, "admin.info_page.unable_to_load", "Unable to load server info."))
		return
	}

	s := data.Stats
	applicationSectionLabel := adminDataString(data, "admin.info_page.application", "Application")
	applicationNameLabel := adminDataString(data, "admin.info_page.application_name", "Search")
	versionLabel := adminDataString(data, "health.version", "Version")
	goVersionLabel := adminDataString(data, "admin.info_page.go_version", "Go Version")
	uptimeLabel := adminDataString(data, "health.uptime", "Uptime")
	systemLabel := adminDataString(data, "admin.info_page.system", "System")
	cpusLabel := adminDataString(data, "admin.info_page.cpus", "CPUs")
	goroutinesLabel := adminDataString(data, "admin.info_page.goroutines", "Goroutines")
	memoryAllocatedLabel := adminDataString(data, "admin.info_page.memory_allocated", "Memory Allocated")
	totalMemoryLabel := adminDataString(data, "admin.info_page.total_memory", "Total Memory")
	pathsLabel := adminDataString(data, "admin.info_page.paths", "Paths")
	configDirectoryLabel := adminDataString(data, "admin.info_page.config_directory", "Config Directory")
	dataDirectoryLabel := adminDataString(data, "admin.info_page.data_directory", "Data Directory")
	logDirectoryLabel := adminDataString(data, "admin.info_page.log_directory", "Log Directory")

	fmt.Fprintf(w, `
            <div class="admin-section">
                <h2>%s</h2>
                <table class="admin-table">
                    <tr><td>%s</td><td>%s</td></tr>
                    <tr><td>%s</td><td>%s</td></tr>
                    <tr><td>%s</td><td>%s</td></tr>
                    <tr><td>%s</td><td>%s</td></tr>
                </table>
            </div>

            <div class="admin-section">
                <h2>%s</h2>
                <table class="admin-table">
                    <tr><td>%s</td><td>%d</td></tr>
                    <tr><td>%s</td><td>%d</td></tr>
                    <tr><td>%s</td><td>%s</td></tr>
                    <tr><td>%s</td><td>%s</td></tr>
                </table>
            </div>

            <div class="admin-section">
                <h2>%s</h2>
                <table class="admin-table">
                    <tr><td>%s</td><td><code>/config</code></td></tr>
                    <tr><td>%s</td><td><code>/data</code></td></tr>
                    <tr><td>%s</td><td><code>/data/logs</code></td></tr>
                </table>
            </div>`,
		applicationSectionLabel,
		applicationSectionLabel,
		applicationNameLabel,
		versionLabel,
		s.Version,
		goVersionLabel,
		s.GoVersion,
		uptimeLabel,
		s.Uptime,
		systemLabel,
		cpusLabel,
		s.NumCPU,
		goroutinesLabel,
		s.NumGoroutines,
		memoryAllocatedLabel,
		s.MemAlloc,
		totalMemoryLabel,
		s.MemTotal,
		pathsLabel,
		configDirectoryLabel,
		dataDirectoryLabel,
		logDirectoryLabel,
	)
}

// renderServerSecurityContent renders the security settings page content
func (h *Handler) renderServerSecurityContent(w io.Writer, data *AdminPageData) {
	rateLimitEnabled := ""
	if data.Config.Server.RateLimit.Enabled {
		rateLimitEnabled = "checked"
	}

	rateLimitingLabel := adminDataString(data, "admin.security_page.rate_limiting", "Rate Limiting")
	enableRateLimitingLabel := adminDataString(data, "admin.security_page.enable_rate_limiting", "Enable Rate Limiting")
	requestsPerMinuteLabel := adminDataString(data, "admin.security_page.requests_per_minute", "Requests per Minute")
	burstSizeLabel := adminDataString(data, "admin.security_page.burst_size", "Burst Size")
	saveSecuritySettingsLabel := adminDataString(data, "admin.security_page.save_settings", "Save Security Settings")
	securityHeadersLabel := adminDataString(data, "admin.security_page.security_headers", "Security Headers")
	relatedSettingsLabel := adminDataString(data, "admin.security_page.related_settings", "Related Settings")
	sslSettingsLabel := adminDataString(data, "admin.security_page.ssl_settings", "SSL/TLS Settings")
	geoIPBlockingLabel := adminDataString(data, "admin.security_page.geoip_blocking", "GeoIP Blocking")
	apiTokensLabel := adminDataString(data, "admin.security_page.api_tokens", "API Tokens")

	fmt.Fprintf(w, `
            <div class="admin-section">
                <h2>%s</h2>
                <form method="POST" action="/admin/server/security">
                    <input type="hidden" name="csrf_token" value="%s">
                    <div class="form-row">
                        <label>
                            <input type="checkbox" name="rate_limit_enabled" %s class="mr-8">
                            %s
                        </label>
                    </div>
                    <div class="form-row">
                        <label>%s</label>
                        <input type="number" name="rate_limit_rpm" value="%d" min="1" max="1000">
                    </div>
                    <div class="form-row">
                        <label>%s</label>
                        <input type="number" name="rate_limit_burst" value="%d" min="1" max="100">
                    </div>
                    <button type="submit" class="btn">%s</button>
                </form>
            </div>

            <div class="admin-section">
                <h2>%s</h2>
                <table class="admin-table">
                    <tr><td>X-Frame-Options</td><td><span class="status-badge enabled">DENY</span></td></tr>
                    <tr><td>X-Content-Type-Options</td><td><span class="status-badge enabled">nosniff</span></td></tr>
                    <tr><td>X-XSS-Protection</td><td><span class="status-badge enabled">1; mode=block</span></td></tr>
                    <tr><td>Referrer-Policy</td><td><span class="status-badge enabled">strict-origin-when-cross-origin</span></td></tr>
                </table>
            </div>

            <div class="admin-section">
                <h2>%s</h2>
                <ul class="text-secondary pl-20">
                    <li><a href="/admin/server/ssl">%s</a></li>
                    <li><a href="/admin/server/geoip">%s</a></li>
                    <li><a href="/admin/tokens">%s</a></li>
                </ul>
            </div>`,
		rateLimitingLabel,
		data.CSRFToken,
		rateLimitEnabled,
		enableRateLimitingLabel,
		requestsPerMinuteLabel,
		data.Config.Server.RateLimit.RequestsPerMinute,
		burstSizeLabel,
		data.Config.Server.RateLimit.BurstSize,
		saveSecuritySettingsLabel,
		securityHeadersLabel,
		relatedSettingsLabel,
		sslSettingsLabel,
		geoIPBlockingLabel,
		apiTokensLabel,
	)
}

// renderHelpContent renders the help/documentation page content
func (h *Handler) renderHelpContent(w io.Writer, data *AdminPageData) {
	documentationLabel := adminDataString(data, "admin.help_page.documentation", "Documentation")
	officialDocumentationLabel := adminDataString(data, "admin.help_page.official_documentation", "Official Documentation")
	officialDocumentationDescriptionLabel := adminDataString(data, "admin.help_page.official_documentation_description", "Complete guides and reference")
	sourceCodeRepositoryLabel := adminDataString(data, "admin.help_page.source_code_repository", "Source Code Repository")
	sourceCodeDescriptionLabel := adminDataString(data, "admin.help_page.source_code_description", "Source code and issues")
	quickLinksLabel := adminDataString(data, "admin.help_page.quick_links", "Quick Links")
	apiDocumentationLabel := adminDataString(data, "admin.help_page.api_documentation", "API Documentation")
	graphqlExplorerLabel := adminDataString(data, "admin.help_page.graphql_explorer", "GraphQL Explorer")
	viewLogsLabel := adminDataString(data, "admin.help_page.view_logs", "View Logs")
	keyboardShortcutsLabel := adminDataString(data, "admin.help_page.keyboard_shortcuts", "Keyboard Shortcuts")
	goToDashboardLabel := adminDataString(data, "admin.help_page.go_to_dashboard", "Go to Dashboard")
	goToConfigurationLabel := adminDataString(data, "admin.help_page.go_to_configuration", "Go to Configuration")
	goToEnginesLabel := adminDataString(data, "admin.help_page.go_to_engines", "Go to Engines")
	goToLogsLabel := adminDataString(data, "admin.help_page.go_to_logs", "Go to Logs")
	showThisHelpLabel := adminDataString(data, "admin.help_page.show_this_help", "Show this help")

	fmt.Fprintf(w, `
            <div class="admin-section">
                <h2>%s</h2>
                <ul class="resource-list">
                    <li>
                        <a href="https://apimgr-search.readthedocs.io" target="_blank">
                            <span class="icon">📚</span>
                            <div>
                                <strong>%s</strong>
                                <p>%s</p>
                            </div>
                        </a>
                    </li>`,
		documentationLabel,
		officialDocumentationLabel,
		officialDocumentationDescriptionLabel,
	)
	if h.config.Server.Branding.SourceCodeURL != "" {
		fmt.Fprintf(w, `
                    <li>
                        <a href="%s" target="_blank">
                            <span class="icon">💻</span>
                            <div>
                                <strong>%s</strong>
                                <p>%s</p>
                            </div>
                        </a>
                    </li>`, h.config.Server.Branding.SourceCodeURL, sourceCodeRepositoryLabel, sourceCodeDescriptionLabel)
	}
	fmt.Fprintf(w, `
                </ul>
            </div>

            <div class="admin-section">
                <h2>%s</h2>
                <div class="d-grid grid-auto gap-15">
                    <a href="/openapi" target="_blank" class="btn text-center">%s</a>
                    <a href="/graphql" target="_blank" class="btn text-center">%s</a>
                    <a href="/admin/logs" class="btn text-center">%s</a>
                </div>
            </div>

            <div class="admin-section">
                <h2>%s</h2>
                <table class="admin-table">
                    <tr><td><kbd>g</kbd> then <kbd>d</kbd></td><td>%s</td></tr>
                    <tr><td><kbd>g</kbd> then <kbd>c</kbd></td><td>%s</td></tr>
                    <tr><td><kbd>g</kbd> then <kbd>e</kbd></td><td>%s</td></tr>
                    <tr><td><kbd>g</kbd> then <kbd>l</kbd></td><td>%s</td></tr>
                    <tr><td><kbd>?</kbd></td><td>%s</td></tr>
                </table>
            </div>`,
		quickLinksLabel,
		apiDocumentationLabel,
		graphqlExplorerLabel,
		viewLogsLabel,
		keyboardShortcutsLabel,
		goToDashboardLabel,
		goToConfigurationLabel,
		goToEnginesLabel,
		goToLogsLabel,
		showThisHelpLabel,
	)
}
