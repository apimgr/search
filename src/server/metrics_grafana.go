package server

import (
	"encoding/json"
	"net/http"
)

// grafanaTarget is one Prometheus query inside a dashboard panel.
type grafanaTarget struct {
	Datasource   map[string]string `json:"datasource"`
	Expr         string            `json:"expr"`
	LegendFormat string            `json:"legendFormat,omitempty"`
	RefID        string            `json:"refId"`
}

// grafanaGridPos is a panel's position on the dashboard grid.
type grafanaGridPos struct {
	H int `json:"h"`
	W int `json:"w"`
	X int `json:"x"`
	Y int `json:"y"`
}

// grafanaPanel is a single dashboard panel.
type grafanaPanel struct {
	Datasource  map[string]string `json:"datasource"`
	Description string            `json:"description"`
	FieldConfig map[string]any    `json:"fieldConfig"`
	GridPos     grafanaGridPos    `json:"gridPos"`
	ID          int               `json:"id"`
	Options     map[string]any    `json:"options"`
	Targets     []grafanaTarget   `json:"targets"`
	Title       string            `json:"title"`
	Type        string            `json:"type"`
}

// grafanaQuery pairs a PromQL expression with its legend label.
type grafanaQuery struct {
	expr   string
	legend string
}

// grafanaPanelSpec describes a panel before grid placement and ID assignment.
type grafanaPanelSpec struct {
	title       string
	description string
	panelType   string
	unit        string
	min         *float64
	max         *float64
	queries     []grafanaQuery
}

// datasourceRef is the template-variable reference every panel and target uses,
// so the dashboard imports against any Prometheus datasource per AI.md PART 20.
var datasourceRef = map[string]string{"type": "prometheus", "uid": "${datasource}"}

// metricsGrafanaHandler serves GET /server/metrics/grafana - a complete,
// importable Grafana dashboard definition covering every metric category this
// specification requires (HTTP, database, cache, scheduler, system, business),
// with the datasource left as a template variable.
func (s *Server) metricsGrafanaHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")

		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		if err := enc.Encode(buildGrafanaDashboard()); err != nil && s.logManager != nil {
			s.logManager.Server().Error("Failed to encode Grafana dashboard", map[string]interface{}{
				"error": err.Error(),
			})
		}
	})
}

// buildGrafanaDashboard assembles the dashboard JSON document.
func buildGrafanaDashboard() map[string]any {
	return map[string]any{
		"annotations":          map[string]any{"list": []any{}},
		"editable":             true,
		"fiscalYearStartMonth": 0,
		"graphTooltip":         1,
		"links":                []any{},
		"panels":               buildGrafanaPanels(),
		"refresh":              "30s",
		"schemaVersion":        39,
		"tags":                 []string{"search", "prometheus"},
		"templating": map[string]any{
			"list": []any{
				map[string]any{
					"current":     map[string]any{},
					"hide":        0,
					"includeAll":  false,
					"label":       "Datasource",
					"multi":       false,
					"name":        "datasource",
					"options":     []any{},
					"query":       "prometheus",
					"queryValue":  "",
					"refresh":     1,
					"regex":       "",
					"skipUrlSync": false,
					"type":        "datasource",
				},
			},
		},
		"time":       map[string]any{"from": "now-6h", "to": "now"},
		"timepicker": map[string]any{},
		"timezone":   "browser",
		"title":      "search Metrics",
		"uid":        "search-metrics",
		"version":    1,
		"weekStart":  "",
	}
}

// buildGrafanaPanels lays the panel specs out on a two-column grid.
func buildGrafanaPanels() []grafanaPanel {
	specs := grafanaPanelSpecs()
	panels := make([]grafanaPanel, 0, len(specs))

	for i, spec := range specs {
		targets := make([]grafanaTarget, 0, len(spec.queries))
		for j, q := range spec.queries {
			targets = append(targets, grafanaTarget{
				Datasource:   datasourceRef,
				Expr:         q.expr,
				LegendFormat: q.legend,
				RefID:        string(rune('A' + j)),
			})
		}

		custom := map[string]any{}
		if spec.panelType == "timeseries" {
			custom["fillOpacity"] = 10
			custom["lineWidth"] = 1
			custom["showPoints"] = "never"
		}

		defaults := map[string]any{
			"color":  map[string]any{"mode": "palette-classic"},
			"custom": custom,
			"unit":   spec.unit,
		}
		if spec.min != nil {
			defaults["min"] = *spec.min
		}
		if spec.max != nil {
			defaults["max"] = *spec.max
		}

		options := map[string]any{}
		switch spec.panelType {
		case "timeseries":
			options["legend"] = map[string]any{"displayMode": "list", "placement": "bottom", "showLegend": true}
			options["tooltip"] = map[string]any{"mode": "multi", "sort": "desc"}
		case "stat", "gauge":
			options["colorMode"] = "value"
			options["graphMode"] = "area"
			options["reduceOptions"] = map[string]any{"calcs": []string{"lastNotNull"}, "fields": "", "values": false}
		case "table":
			options["showHeader"] = true
		}

		panels = append(panels, grafanaPanel{
			Datasource:  datasourceRef,
			Description: spec.description,
			FieldConfig: map[string]any{"defaults": defaults, "overrides": []any{}},
			GridPos:     grafanaGridPos{H: 8, W: 12, X: (i % 2) * 12, Y: (i / 2) * 8},
			ID:          i + 1,
			Options:     options,
			Targets:     targets,
			Title:       spec.title,
			Type:        spec.panelType,
		})
	}

	return panels
}

// percentMin and percentMax bound the 0-100 percentage gauges.
var (
	percentMin = 0.0
	percentMax = 100.0
)

// grafanaPanelSpecs enumerates one panel per exported metric group, covering
// every category AI.md PART 20 requires.
func grafanaPanelSpecs() []grafanaPanelSpec {
	return []grafanaPanelSpec{
		// Application
		{
			title:       "Build Info",
			description: "search_app_info - version, commit, build date and Go version",
			panelType:   "table",
			queries:     []grafanaQuery{{expr: "search_app_info", legend: "{{version}} {{commit}}"}},
		},
		{
			title:       "Uptime",
			description: "search_app_uptime_seconds / search_app_start_timestamp",
			panelType:   "stat",
			unit:        "s",
			queries: []grafanaQuery{
				{expr: "search_app_uptime_seconds", legend: "uptime"},
				{expr: "search_app_start_timestamp", legend: "started"},
			},
		},

		// HTTP
		{
			title:       "Request Rate",
			description: "search_http_requests_total",
			panelType:   "timeseries",
			unit:        "reqps",
			queries:     []grafanaQuery{{expr: "sum(rate(search_http_requests_total[5m]))", legend: "requests"}},
		},
		{
			title:       "Error Rate",
			description: "5xx share of search_http_requests_total",
			panelType:   "timeseries",
			unit:        "percentunit",
			queries: []grafanaQuery{
				{expr: `sum(rate(search_http_requests_total{status=~"5.."}[5m])) / sum(rate(search_http_requests_total[5m]))`, legend: "5xx"},
				{expr: `sum(rate(search_http_requests_total{status=~"4.."}[5m])) / sum(rate(search_http_requests_total[5m]))`, legend: "4xx"},
			},
		},
		{
			title:       "Latency (p50, p95, p99)",
			description: "search_http_request_duration_seconds",
			panelType:   "timeseries",
			unit:        "s",
			queries: []grafanaQuery{
				{expr: "histogram_quantile(0.50, sum(rate(search_http_request_duration_seconds_bucket[5m])) by (le))", legend: "p50"},
				{expr: "histogram_quantile(0.95, sum(rate(search_http_request_duration_seconds_bucket[5m])) by (le))", legend: "p95"},
				{expr: "histogram_quantile(0.99, sum(rate(search_http_request_duration_seconds_bucket[5m])) by (le))", legend: "p99"},
			},
		},
		{
			title:       "Active Requests",
			description: "search_http_active_requests",
			panelType:   "stat",
			unit:        "short",
			queries:     []grafanaQuery{{expr: "search_http_active_requests", legend: "in flight"}},
		},
		{
			title:       "Request / Response Size (p95)",
			description: "search_http_request_size_bytes and search_http_response_size_bytes",
			panelType:   "timeseries",
			unit:        "bytes",
			queries: []grafanaQuery{
				{expr: "histogram_quantile(0.95, sum(rate(search_http_request_size_bytes_bucket[5m])) by (le))", legend: "request p95"},
				{expr: "histogram_quantile(0.95, sum(rate(search_http_response_size_bytes_bucket[5m])) by (le))", legend: "response p95"},
			},
		},

		// Database
		{
			title:       "Database Connections",
			description: "search_db_connections_open and search_db_connections_in_use",
			panelType:   "timeseries",
			unit:        "short",
			queries: []grafanaQuery{
				{expr: "search_db_connections_open", legend: "open"},
				{expr: "search_db_connections_in_use", legend: "in_use"},
			},
		},
		{
			title:       "Database Query Rate",
			description: "search_db_queries_total",
			panelType:   "timeseries",
			unit:        "ops",
			queries:     []grafanaQuery{{expr: "sum(rate(search_db_queries_total[5m])) by (operation)", legend: "{{operation}}"}},
		},
		{
			title:       "Database Query Latency (p95)",
			description: "search_db_query_duration_seconds",
			panelType:   "timeseries",
			unit:        "s",
			queries:     []grafanaQuery{{expr: "histogram_quantile(0.95, sum(rate(search_db_query_duration_seconds_bucket[5m])) by (le, operation))", legend: "{{operation}}"}},
		},
		{
			title:       "Database Errors",
			description: "search_db_errors_total",
			panelType:   "timeseries",
			unit:        "ops",
			queries:     []grafanaQuery{{expr: "sum(rate(search_db_errors_total[5m])) by (error_type)", legend: "{{error_type}}"}},
		},

		// Cache
		{
			title:       "Cache Hit Rate",
			description: "search_cache_hits_total vs search_cache_misses_total",
			panelType:   "timeseries",
			unit:        "percentunit",
			queries:     []grafanaQuery{{expr: "sum(rate(search_cache_hits_total[5m])) / (sum(rate(search_cache_hits_total[5m])) + sum(rate(search_cache_misses_total[5m])))", legend: "hit rate"}},
		},
		{
			title:       "Cache Size",
			description: "search_cache_size and search_cache_bytes",
			panelType:   "timeseries",
			unit:        "short",
			queries: []grafanaQuery{
				{expr: "search_cache_size", legend: "{{cache}} items"},
				{expr: "search_cache_bytes", legend: "{{cache}} bytes"},
			},
		},
		{
			title:       "Cache Evictions",
			description: "search_cache_evictions_total",
			panelType:   "timeseries",
			unit:        "ops",
			queries:     []grafanaQuery{{expr: "sum(rate(search_cache_evictions_total[5m])) by (cache)", legend: "{{cache}}"}},
		},

		// Scheduler
		{
			title:       "Scheduler Task Executions",
			description: "search_scheduler_tasks_total",
			panelType:   "timeseries",
			unit:        "ops",
			queries:     []grafanaQuery{{expr: "sum(rate(search_scheduler_tasks_total[1h])) by (task, status)", legend: "{{task}} {{status}}"}},
		},
		{
			title:       "Scheduler Task Duration (p95)",
			description: "search_scheduler_task_duration_seconds",
			panelType:   "timeseries",
			unit:        "s",
			queries:     []grafanaQuery{{expr: "histogram_quantile(0.95, sum(rate(search_scheduler_task_duration_seconds_bucket[1h])) by (le, task))", legend: "{{task}}"}},
		},
		{
			title:       "Scheduler Tasks Running",
			description: "search_scheduler_tasks_running",
			panelType:   "timeseries",
			unit:        "short",
			queries:     []grafanaQuery{{expr: "search_scheduler_tasks_running", legend: "{{task}}"}},
		},
		{
			title:       "Scheduler Last Run",
			description: "search_scheduler_last_run_timestamp",
			panelType:   "table",
			unit:        "dateTimeAsIso",
			queries:     []grafanaQuery{{expr: "search_scheduler_last_run_timestamp * 1000", legend: "{{task}}"}},
		},

		// Rate limiting
		{
			title:       "Rate Limiter",
			description: "search_ratelimit_requests_total and search_ratelimit_blocked_total",
			panelType:   "timeseries",
			unit:        "reqps",
			queries: []grafanaQuery{
				{expr: "sum(rate(search_ratelimit_requests_total[5m])) by (limit, status)", legend: "{{limit}} {{status}}"},
				{expr: "sum(rate(search_ratelimit_blocked_total[5m])) by (limit)", legend: "{{limit}} blocked"},
			},
		},

		// Authentication
		{
			title:       "Authentication",
			description: "search_auth_attempts_total and search_auth_sessions_active",
			panelType:   "timeseries",
			unit:        "short",
			queries: []grafanaQuery{
				{expr: "sum(rate(search_auth_attempts_total[5m])) by (method, status)", legend: "{{method}} {{status}}"},
				{expr: "search_auth_sessions_active", legend: "active sessions"},
			},
		},

		// System
		{
			title:       "CPU Usage",
			description: "search_system_cpu_usage_percent",
			panelType:   "gauge",
			unit:        "percent",
			min:         &percentMin,
			max:         &percentMax,
			queries:     []grafanaQuery{{expr: "search_system_cpu_usage_percent", legend: "cpu"}},
		},
		{
			title:       "Memory Usage",
			description: "search_system_memory_usage_percent",
			panelType:   "gauge",
			unit:        "percent",
			min:         &percentMin,
			max:         &percentMax,
			queries:     []grafanaQuery{{expr: "search_system_memory_usage_percent", legend: "memory"}},
		},
		{
			title:       "Memory Used / Total",
			description: "search_system_memory_used_bytes and search_system_memory_total_bytes",
			panelType:   "timeseries",
			unit:        "bytes",
			queries: []grafanaQuery{
				{expr: "search_system_memory_used_bytes", legend: "used"},
				{expr: "search_system_memory_total_bytes", legend: "total"},
			},
		},
		{
			title:       "Disk Usage",
			description: "search_system_disk_usage_percent, search_system_disk_used_bytes, search_system_disk_total_bytes",
			panelType:   "timeseries",
			unit:        "bytes",
			queries: []grafanaQuery{
				{expr: "search_system_disk_used_bytes", legend: "{{path}} used"},
				{expr: "search_system_disk_total_bytes", legend: "{{path}} total"},
				{expr: "search_system_disk_usage_percent", legend: "{{path}} percent"},
			},
		},

		// Go runtime
		{
			title:       "Goroutines",
			description: "search_go_goroutines",
			panelType:   "timeseries",
			unit:        "short",
			queries:     []grafanaQuery{{expr: "search_go_goroutines", legend: "goroutines"}},
		},
		{
			title:       "Go Memory",
			description: "search_go_mem_alloc_bytes and search_go_mem_sys_bytes",
			panelType:   "timeseries",
			unit:        "bytes",
			queries: []grafanaQuery{
				{expr: "search_go_mem_alloc_bytes", legend: "alloc"},
				{expr: "search_go_mem_sys_bytes", legend: "sys"},
			},
		},
		{
			title:       "Garbage Collection",
			description: "search_go_gc_runs_total and search_go_gc_pause_total_seconds",
			panelType:   "timeseries",
			unit:        "short",
			queries: []grafanaQuery{
				{expr: "rate(search_go_gc_runs_total[5m])", legend: "gc runs/s"},
				{expr: "rate(search_go_gc_pause_total_seconds[5m])", legend: "pause s/s"},
			},
		},

		// Business (search domain)
		{
			title:       "Search Rate",
			description: "search_searches_total",
			panelType:   "timeseries",
			unit:        "reqps",
			queries:     []grafanaQuery{{expr: "rate(search_searches_total[5m])", legend: "searches"}},
		},
		{
			title:       "Search Duration (p95)",
			description: "search_search_duration_seconds",
			panelType:   "timeseries",
			unit:        "s",
			queries:     []grafanaQuery{{expr: "histogram_quantile(0.95, sum(rate(search_search_duration_seconds_bucket[5m])) by (le, category))", legend: "{{category}}"}},
		},
		{
			title:       "Engine Requests",
			description: "search_engine_requests_total",
			panelType:   "timeseries",
			unit:        "reqps",
			queries:     []grafanaQuery{{expr: "sum(rate(search_engine_requests_total[5m])) by (engine)", legend: "{{engine}}"}},
		},
		{
			title:       "Engine Errors",
			description: "search_engine_errors_total",
			panelType:   "timeseries",
			unit:        "reqps",
			queries:     []grafanaQuery{{expr: "sum(rate(search_engine_errors_total[5m])) by (engine)", legend: "{{engine}}"}},
		},

		// Tor
		{
			title:       "Tor Hidden Service",
			description: "search_tor_enabled, search_tor_running, search_tor_circuit_established, search_tor_requests_total",
			panelType:   "timeseries",
			unit:        "short",
			queries: []grafanaQuery{
				{expr: "search_tor_enabled", legend: "enabled"},
				{expr: "search_tor_running", legend: "running"},
				{expr: "search_tor_circuit_established", legend: "circuit"},
				{expr: "rate(search_tor_requests_total[5m])", legend: "requests/s"},
			},
		},
	}
}
