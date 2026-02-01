package server

import (
	"embed"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/apimgr/search/src/config"
)

//go:embed template/layouts/*.tmpl template/partials/*.tmpl template/partials/admin/*.tmpl template/partials/public/*.tmpl template/components/*.tmpl template/pages/*.tmpl template/pages/auth/*.tmpl template/pages/user/*.tmpl static/*
var EmbeddedFS embed.FS

// TemplateRenderer handles template rendering
type TemplateRenderer struct {
	templates map[string]*template.Template
	mu        sync.RWMutex
	config    *config.Config
	devMode   bool
	funcMap   template.FuncMap
}

// NewTemplateRenderer creates a new template renderer
// i18nFuncs provides translation functions (t, lang, isRTL, dir, languages)
// If nil, a fallback t function that returns the key is used
func NewTemplateRenderer(cfg *config.Config, i18nFuncs template.FuncMap) *TemplateRenderer {
	tr := &TemplateRenderer{
		templates: make(map[string]*template.Template),
		config:    cfg,
		devMode:   cfg.IsDevelopment(),
	}

	tr.funcMap = template.FuncMap{
		// i18n functions - use provided funcs or fallback
		"t": func(key string, args ...interface{}) string {
			if i18nFuncs != nil {
				if tFunc, ok := i18nFuncs["t"].(func(string, ...interface{}) string); ok {
					return tFunc(key, args...)
				}
			}
			return key // Fallback: return key as-is
		},
		"lang": func() string {
			if i18nFuncs != nil {
				if f, ok := i18nFuncs["lang"].(func() string); ok {
					return f()
				}
			}
			return "en"
		},
		"isRTL": func() bool {
			if i18nFuncs != nil {
				if f, ok := i18nFuncs["isRTL"].(func() bool); ok {
					return f()
				}
			}
			return false
		},
		"dir": func() string {
			if i18nFuncs != nil {
				if f, ok := i18nFuncs["dir"].(func() string); ok {
					return f()
				}
			}
			return "ltr"
		},
		"safe":     func(s string) template.HTML { return template.HTML(s) },
		"safeHTML": func(s string) template.HTML { return template.HTML(s) },
		"safeURL":  func(s string) template.URL { return template.URL(s) },
		"safeCSS":  func(s string) template.CSS { return template.CSS(s) },
		"safeJS":   func(s string) template.JS { return template.JS(s) },
		"lower":    strings.ToLower,
		"upper":    strings.ToUpper,
		"title":    strings.Title,
		"contains": strings.Contains,
		"hasPrefix": strings.HasPrefix,
		"hasSuffix": strings.HasSuffix,
		"replace":  strings.ReplaceAll,
		"trim":     strings.TrimSpace,
		"join":     strings.Join,
		"split":    strings.Split,
		"default": func(def, val interface{}) interface{} {
			if val == nil || val == "" {
				return def
			}
			return val
		},
		"eq": func(a, b interface{}) bool { return a == b },
		"ne": func(a, b interface{}) bool { return a != b },
		"add": func(a, b int) int { return a + b },
		"sub": func(a, b int) int { return a - b },
		"mul": func(a, b int) int { return a * b },
		"div": func(a, b int) int {
			if b == 0 {
				return 0
			}
			return a / b
		},
		"mod": func(a, b int) int {
			if b == 0 {
				return 0
			}
			return a % b
		},
		"seq": func(start, end int) []int {
			var result []int
			for i := start; i <= end; i++ {
				result = append(result, i)
			}
			return result
		},
		"truncate": func(length int, s string) string {
			if len(s) <= length {
				return s
			}
			return s[:length] + "..."
		},
		"config": func() *config.Config { return cfg },
		"version": func() string { return config.Version },
		"year": func() int {
			return time.Now().Year()
		},
		"urlquery": func(s string) string {
			return url.QueryEscape(s)
		},
		"formatVideoDuration": formatVideoDuration,
		"formatViewCount": formatViewCount,
	}

	tr.loadTemplates()
	return tr
}

// loadTemplates loads all templates from embedded filesystem
func (tr *TemplateRenderer) loadTemplates() error {
	tr.mu.Lock()
	defer tr.mu.Unlock()

	// Load layout template
	layoutContent, err := fs.ReadFile(EmbeddedFS, "template/layouts/base.tmpl")
	if err != nil {
		return err
	}

	// Load all partials (including subdirectories like partials/public/)
	partials := make(map[string]string)
	tr.loadPartialsRecursive("template/partials", partials)

	// Load all page templates (including subdirectories like pages/auth/, pages/user/)
	tr.loadPagesRecursive("template/pages", "", string(layoutContent), partials)

	return nil
}

// loadPartialsRecursive recursively loads partials from a directory
func (tr *TemplateRenderer) loadPartialsRecursive(dir string, partials map[string]string) {
	entries, err := fs.ReadDir(EmbeddedFS, dir)
	if err != nil {
		return
	}

	for _, entry := range entries {
		path := dir + "/" + entry.Name()
		if entry.IsDir() {
			// Recurse into subdirectory
			tr.loadPartialsRecursive(path, partials)
		} else if strings.HasSuffix(entry.Name(), ".tmpl") {
			content, err := fs.ReadFile(EmbeddedFS, path)
			if err != nil {
				continue
			}
			name := strings.TrimSuffix(entry.Name(), ".tmpl")
			partials[name] = string(content)
		}
	}
}

// loadPagesRecursive recursively loads page templates from a directory
func (tr *TemplateRenderer) loadPagesRecursive(dir, prefix, layoutContent string, partials map[string]string) {
	entries, err := fs.ReadDir(EmbeddedFS, dir)
	if err != nil {
		return
	}

	for _, entry := range entries {
		path := dir + "/" + entry.Name()
		if entry.IsDir() {
			// Recurse into subdirectory with updated prefix
			subPrefix := entry.Name()
			if prefix != "" {
				subPrefix = prefix + "/" + entry.Name()
			}
			tr.loadPagesRecursive(path, subPrefix, layoutContent, partials)
		} else if strings.HasSuffix(entry.Name(), ".tmpl") {
			pageContent, err := fs.ReadFile(EmbeddedFS, path)
			if err != nil {
				continue
			}

			// Build template name with path prefix (e.g., "auth/login")
			baseName := strings.TrimSuffix(entry.Name(), ".tmpl")
			name := baseName
			if prefix != "" {
				name = prefix + "/" + baseName
			}

			// Combine layout + partials + page
			combined := layoutContent
			for _, partialContent := range partials {
				combined += "\n" + partialContent
			}
			combined += "\n" + string(pageContent)

			tmpl, err := template.New(name).Funcs(tr.funcMap).Parse(combined)
			if err != nil {
				continue
			}

			tr.templates[name] = tmpl
		}
	}
}

// Render renders a template with the given data
func (tr *TemplateRenderer) Render(w io.Writer, name string, data interface{}) error {
	// In dev mode, reload templates on each request
	if tr.devMode {
		tr.loadTemplates()
	}

	tr.mu.RLock()
	tmpl, ok := tr.templates[name]
	tr.mu.RUnlock()

	if !ok {
		return &TemplateNotFoundError{Name: name}
	}

	return tmpl.ExecuteTemplate(w, "base", data)
}

// TemplateNotFoundError is returned when a template is not found
type TemplateNotFoundError struct {
	Name string
}

func (e *TemplateNotFoundError) Error() string {
	return "template not found: " + e.Name
}

// StaticFileServer returns an http.Handler for serving static files
func StaticFileServer() http.Handler {
	staticFS, err := fs.Sub(EmbeddedFS, "static")
	if err != nil {
		return http.NotFoundHandler()
	}
	return http.FileServer(http.FS(staticFS))
}

// GetStaticFile returns the content of a static file
func GetStaticFile(path string) ([]byte, error) {
	return fs.ReadFile(EmbeddedFS, filepath.Join("static", path))
}

// PageData represents common data passed to all page templates
type PageData struct {
	Title          string
	Description    string
	Page           string
	Theme          string
	Lang           string // Language code for html lang attribute (default: "en")
	Dir            string // Text direction for html dir attribute (default: "ltr")
	Config         *config.Config
	User           interface{}
	CSRF           string
	CSRFToken      string
	Flash          *FlashMessage
	Data           interface{}
	Query          string
	Category       string
	BuildDate      string
	Announcements  []Announcement // Active announcements
	TorAddress     string
	WidgetsEnabled bool
	DefaultWidgets string // JSON array of default widget types
	CookieConsent  *CookieConsentData
	Extra          map[string]interface{}
}

// ErrorPageData extends PageData with error-specific fields
type ErrorPageData struct {
	PageData
	ErrorCode    int
	ErrorTitle   string
	ErrorMessage string
	ErrorDetails string
}

// SearchPageData extends PageData with search-specific fields
type SearchPageData struct {
	PageData
	Query         string
	Category      string
	Results       interface{}
	TotalResults  int
	SearchTime    float64
	Pagination    *Pagination
	Error         string
	InstantAnswer interface{} // Instant answer result (if any)
}

// HealthPageData extends PageData with health-specific fields
type HealthPageData struct {
	PageData
	Health *HealthInfo
}

// HealthInfo represents health check information per AI.md PART 13
type HealthInfo struct {
	Project        *ProjectInfo      `json:"project,omitempty"`
	Status         string            `json:"status"`
	Version        string            `json:"version"`
	GoVersion      string            `json:"go_version"`
	Mode           string            `json:"mode"`
	Uptime         string            `json:"uptime"`
	Timestamp      string            `json:"timestamp"`
	Build          *BuildInfo        `json:"build,omitempty"`
	Node           *NodeInfo         `json:"node,omitempty"`
	Cluster        *ClusterInfo      `json:"cluster,omitempty"`
	Features       *HealthFeatures   `json:"features,omitempty"`
	Checks         map[string]string `json:"checks"`
	Stats          *HealthStats      `json:"stats,omitempty"`
	System         *SystemInfo       `json:"system,omitempty"`
	PendingRestart bool              `json:"pending_restart,omitempty"`
	RestartReason  []string          `json:"restart_reason,omitempty"`
	Maintenance    *MaintenanceInfo  `json:"maintenance,omitempty"`
}

// ProjectInfo represents project information for healthz per AI.md PART 13
type ProjectInfo struct {
	Name        string `json:"name"`        // branding.app_name or server.title
	Tagline     string `json:"tagline"`     // branding.tagline (short slogan)
	Description string `json:"description"` // server.description (longer)
}

// BuildInfo represents build information per AI.md PART 13
// Note: Fields are "commit" and "date" per spec, not "commit_id" and "build_date"
type BuildInfo struct {
	Commit string `json:"commit"`
	Date   string `json:"date"`
}

// HealthFeatures represents feature status per AI.md PART 13
type HealthFeatures struct {
	MultiUser     bool        `json:"multi_user"`
	Organizations bool        `json:"organizations"`
	Tor           *TorFeature `json:"tor"`
	GeoIP         bool        `json:"geoip"`
	Metrics       bool        `json:"metrics"`
}

// TorFeature represents Tor status per AI.md PART 13
type TorFeature struct {
	Enabled  bool   `json:"enabled"`
	Running  bool   `json:"running"`
	Status   string `json:"status"`
	Hostname string `json:"hostname"`
}

// HealthStats represents health statistics per AI.md PART 13
type HealthStats struct {
	RequestsTotal     int64 `json:"requests_total"`
	Requests24h       int64 `json:"requests_24h"`
	ActiveConnections int   `json:"active_connections"`
}

// NodeInfo represents node information for cluster mode
type NodeInfo struct {
	ID       string `json:"id"`
	Hostname string `json:"hostname"`
}

// ClusterInfo represents cluster status per AI.md PART 13
type ClusterInfo struct {
	Enabled   bool     `json:"enabled"`
	Status    string   `json:"status,omitempty"`    // "connected", "disconnected"
	Primary   string   `json:"primary,omitempty"`   // primary node public URL
	Nodes     []string `json:"nodes,omitempty"`     // all node public URLs
	NodeCount int      `json:"node_count,omitempty"` // total nodes
	Role      string   `json:"role,omitempty"`      // "primary" or "member"
}

// MaintenanceInfo represents maintenance mode status
type MaintenanceInfo struct {
	Reason  string `json:"reason,omitempty"`
	Message string `json:"message,omitempty"`
	Since   string `json:"since,omitempty"`
}

// SystemInfo represents system information
type SystemInfo struct {
	GoVersion    string `json:"go_version"`
	NumCPU       int    `json:"num_cpu"`
	NumGoroutine int    `json:"num_goroutine"`
	MemAlloc     string `json:"mem_alloc"`
}

// Pagination represents pagination information
type Pagination struct {
	CurrentPage int
	TotalPages  int
	HasPrev     bool
	HasNext     bool
}

// ContactPageData extends PageData with contact form fields
type ContactPageData struct {
	PageData
	ContactSent  bool
	ContactError string
	CaptchaA     int
	CaptchaB     int
	CaptchaID    string
}

// FlashMessage represents a flash message
type FlashMessage struct {
	Type    string
	Message string
}

// Announcement represents a site announcement (local type for templates)
type Announcement struct {
	ID          string
	Type        string
	Title       string
	Message     string
	Dismissible bool
}

// CookieConsentData represents cookie consent popup data
type CookieConsentData struct {
	Enabled   bool
	Message   string
	PolicyURL string
}

// NewPageData creates a new PageData with defaults
func NewPageData(cfg *config.Config, title, page string) *PageData {
	pd := &PageData{
		Title:       title,
		Description: cfg.Server.Description,
		Page:        page,
		Theme:       "dark",
		Lang:        "en",
		Dir:         "ltr",
		Config:      cfg,
		BuildDate:   time.Now().Format(time.RFC3339),
	}

	// Populate active announcements from config
	if cfg.Server.Web.Announcements.Enabled {
		active := cfg.Server.Web.Announcements.ActiveAnnouncements()
		for _, a := range active {
			pd.Announcements = append(pd.Announcements, Announcement{
				ID:          a.ID,
				Type:        a.Type,
				Title:       a.Title,
				Message:     a.Message,
				Dismissible: a.Dismissible,
			})
		}
	}

	// Populate cookie consent if enabled
	cc := cfg.Server.Web.CookieConsent
	if cc.Enabled {
		pd.CookieConsent = &CookieConsentData{
			Enabled:   true,
			Message:   cc.Message,
			PolicyURL: cc.PolicyURL,
		}
	}

	return pd
}

// formatVideoDuration formats seconds to MM:SS or H:MM:SS format for video durations
func formatVideoDuration(seconds int) string {
	if seconds <= 0 {
		return ""
	}
	h := seconds / 3600
	m := (seconds % 3600) / 60
	s := seconds % 60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}

// formatViewCount formats large numbers with K, M, B suffixes
func formatViewCount(count int64) string {
	if count <= 0 {
		return ""
	}
	if count >= 1000000000 {
		v := float64(count) / 1000000000
		if v >= 10 {
			return fmt.Sprintf("%.0fB", v)
		}
		return strings.TrimSuffix(fmt.Sprintf("%.1fB", v), ".0B") + ""
	}
	if count >= 1000000 {
		v := float64(count) / 1000000
		if v >= 10 {
			return fmt.Sprintf("%.0fM", v)
		}
		return strings.TrimSuffix(fmt.Sprintf("%.1fM", v), ".0M") + ""
	}
	if count >= 1000 {
		v := float64(count) / 1000
		if v >= 10 {
			return fmt.Sprintf("%.0fK", v)
		}
		return strings.TrimSuffix(fmt.Sprintf("%.1fK", v), ".0K") + ""
	}
	return fmt.Sprintf("%d", count)
}
