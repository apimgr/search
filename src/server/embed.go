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
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/apimgr/search/src/config"
	"github.com/apimgr/search/src/i18n"
)

//go:embed template/layout/*.tmpl template/partial/*.tmpl template/partial/public/*.tmpl template/component/*.tmpl template/page/*.tmpl template/auth/*.tmpl static/*
var EmbeddedFS embed.FS

// TemplateRenderer handles template rendering
type TemplateRenderer struct {
	templates   map[string]map[string]*template.Template
	mu          sync.RWMutex
	config      *config.Config
	devMode     bool
	i18nManager *i18n.Manager
}

// NewTemplateRenderer creates a new template renderer
func NewTemplateRenderer(cfg *config.Config, i18nManager *i18n.Manager) *TemplateRenderer {
	tr := &TemplateRenderer{
		templates:   make(map[string]map[string]*template.Template),
		config:      cfg,
		devMode:     cfg.IsDevelopment(),
		i18nManager: i18nManager,
	}

	tr.loadTemplates()
	return tr
}

func (tr *TemplateRenderer) newFuncMap(i18nFuncs template.FuncMap) template.FuncMap {
	return template.FuncMap{
		// i18n functions - use provided funcs or fallback
		"t": func(key string, args ...interface{}) string {
			if i18nFuncs != nil {
				if tFunc, ok := i18nFuncs["t"].(func(string, ...interface{}) string); ok {
					return tFunc(key, args...)
				}
			}
			// Key not found in i18n; return key as-is
			return key
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
		"safe":      func(s string) template.HTML { return template.HTML(s) },
		"safeHTML":  func(s string) template.HTML { return template.HTML(s) },
		"safeURL":   func(s string) template.URL { return template.URL(s) },
		"safeCSS":   func(s string) template.CSS { return template.CSS(s) },
		"safeJS":    func(s string) template.JS { return template.JS(s) },
		"lower":     strings.ToLower,
		"upper":     strings.ToUpper,
		"title":     strings.Title,
		"contains":  strings.Contains,
		"hasPrefix": strings.HasPrefix,
		"hasSuffix": strings.HasSuffix,
		"replace":   strings.ReplaceAll,
		"trim":      strings.TrimSpace,
		"join":      strings.Join,
		"split":     strings.Split,
		"default": func(def, val interface{}) interface{} {
			if val == nil || val == "" {
				return def
			}
			return val
		},
		"eq":  func(a, b interface{}) bool { return a == b },
		"ne":  func(a, b interface{}) bool { return a != b },
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
		"config":  func() *config.Config { return tr.config },
		"version": func() string { return config.Version },
		"year": func() int {
			return time.Now().Year()
		},
		"urlquery": func(s string) string {
			return url.QueryEscape(s)
		},
		"formatVideoDuration": formatVideoDuration,
		"formatViewCount":     formatViewCount,
		// Use a numeric date format so search results do not hardcode English month names.
		"formatSearchDate": formatSearchDate,
	}
}

// loadTemplates loads all templates from embedded filesystem
func (tr *TemplateRenderer) loadTemplates() error {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	tr.templates = make(map[string]map[string]*template.Template)

	// Load layout template
	layoutContent, err := fs.ReadFile(EmbeddedFS, "template/layout/base.tmpl")
	if err != nil {
		return err
	}

	// Load all partials (including subdirectories like partial/public/)
	partials := make(map[string]string)
	tr.loadPartialsRecursive("template/partial", partials)

	for _, lang := range tr.supportedLanguages() {
		tr.templates[lang] = make(map[string]*template.Template)
		funcMap := tr.newFuncMap(tr.languageFuncMap(lang))

		// Load page templates from template/page/ (index, healthz, error, etc.)
		tr.loadPagesRecursive("template/page", "", string(layoutContent), partials, tr.templates[lang], funcMap)

		// Load auth pages from template/auth/ (per AI.md: auth/ is sibling of page/)
		tr.loadPagesRecursive("template/auth", "auth", string(layoutContent), partials, tr.templates[lang], funcMap)
	}

	return nil
}

// loadPartialsRecursive recursively loads partials from a directory
// baseDir is the root partials directory (e.g., "template/partial")
func (tr *TemplateRenderer) loadPartialsRecursive(dir string, partials map[string]string) {
	tr.loadPartialsRecursiveWithPrefix(dir, "", partials)
}

// loadPartialsRecursiveWithPrefix recursively loads partials with subdirectory prefix
func (tr *TemplateRenderer) loadPartialsRecursiveWithPrefix(dir, prefix string, partials map[string]string) {
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
			tr.loadPartialsRecursiveWithPrefix(path, subPrefix, partials)
		} else if strings.HasSuffix(entry.Name(), ".tmpl") {
			content, err := fs.ReadFile(EmbeddedFS, path)
			if err != nil {
				continue
			}
			// Build partial name with prefix (e.g., "public/header" for partial/public/header.tmpl)
			baseName := strings.TrimSuffix(entry.Name(), ".tmpl")
			name := baseName
			if prefix != "" {
				name = prefix + "/" + baseName
			}
			partials[name] = string(content)
		}
	}
}

// loadPagesRecursive recursively loads page templates from a directory
func (tr *TemplateRenderer) loadPagesRecursive(dir, prefix, layoutContent string, partials map[string]string, templateSet map[string]*template.Template, funcMap template.FuncMap) {
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
			tr.loadPagesRecursive(path, subPrefix, layoutContent, partials, templateSet, funcMap)
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

			tmpl, err := template.New(name).Funcs(funcMap).Parse(combined)
			if err != nil {
				continue
			}

			templateSet[name] = tmpl
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
	templateSet, ok := tr.templates[tr.resolveTemplateLanguage(data)]
	if !ok {
		templateSet = tr.templates[tr.defaultLanguage()]
	}
	tmpl, ok := templateSet[name]
	tr.mu.RUnlock()

	if !ok {
		return &TemplateNotFoundError{Name: name}
	}

	return tmpl.ExecuteTemplate(w, "base", data)
}

func (tr *TemplateRenderer) supportedLanguages() []string {
	if tr.i18nManager == nil {
		return []string{"en"}
	}
	langs := tr.i18nManager.SupportedLanguageCodes()
	if len(langs) == 0 {
		return []string{tr.i18nManager.DefaultLanguage()}
	}
	return langs
}

func (tr *TemplateRenderer) defaultLanguage() string {
	if tr.i18nManager != nil && tr.i18nManager.DefaultLanguage() != "" {
		return tr.i18nManager.DefaultLanguage()
	}
	return "en"
}

func (tr *TemplateRenderer) languageFuncMap(lang string) template.FuncMap {
	if tr.i18nManager == nil {
		return nil
	}
	return tr.i18nManager.TemplateFuncs(lang)
}

func (tr *TemplateRenderer) resolveTemplateLanguage(data interface{}) string {
	if data == nil {
		return tr.defaultLanguage()
	}

	value := reflect.ValueOf(data)
	if !value.IsValid() {
		return tr.defaultLanguage()
	}
	if value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return tr.defaultLanguage()
		}
		value = value.Elem()
	}
	if value.Kind() != reflect.Struct {
		return tr.defaultLanguage()
	}

	if lang := getStringField(value, "Lang"); lang != "" {
		return lang
	}
	if pageData := value.FieldByName("PageData"); pageData.IsValid() && pageData.Kind() == reflect.Struct {
		if lang := getStringField(pageData, "Lang"); lang != "" {
			return lang
		}
	}

	return tr.defaultLanguage()
}

func getStringField(value reflect.Value, field string) string {
	result := value.FieldByName(field)
	if !result.IsValid() || result.Kind() != reflect.String {
		return ""
	}
	return strings.TrimSpace(result.String())
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

// PageData represents common data passed to all page templates.
// Theme is the resolved CSS class ("dark" or "light", never "auto").
// ThemeMode is the user preference ("dark", "light", or "auto").
// Lang and Dir are the html lang/dir attributes (defaults: "en" / "ltr").
// AdminPath is the configurable admin route prefix per AI.md PART 17 (default: "admin").
type PageData struct {
	Title              string
	Description        string
	Page               string
	Theme              string
	ThemeMode          string
	Lang               string
	Dir                string
	AvailableLanguages []i18n.Language
	Config             *config.Config
	User               interface{}
	CSRF               string
	CSRFToken          string
	Flash              *FlashMessage
	Data               interface{}
	Query              string
	Category           string
	BuildDate          string
	Announcements      []Announcement
	TorEnabled         bool
	TorStatus          string
	TorAddress         string
	WidgetsEnabled     bool
	DefaultWidgets     string
	CookieConsent      *CookieConsentData
	Extra              map[string]interface{}
	AdminPath          string
	ServerURL          string
	PrefsQuery         string
}

// ErrorPageData extends PageData with error-specific fields.
// ErrorDetails contains dev-only technical information; omitted in production.
type ErrorPageData struct {
	PageData
	StatusCode   int
	StatusText   string
	Message      string
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
	Engines       []string
	PerPage       int
	SafeSearch    int
	Pagination    *Pagination
	Error         string
	InstantAnswer interface{}
}

// HealthPageData extends PageData with health-specific fields
type HealthPageData struct {
	PageData
	Health *HealthResponse
}

// HealthResponse represents health check information per AI.md PART 13.
// All fields use canonical order: project, status, version, build, runtime,
// cluster, features, checks, stats.
type HealthResponse struct {
	// 1. Project identification (PART 16: branding config)
	Project ProjectInfo `json:"project"`
	// 2. Overall status
	Status         string   `json:"status"`
	PendingRestart bool     `json:"pending_restart,omitempty"`
	RestartReason  []string `json:"restart_reason,omitempty"`
	// 3. Version & build info (PART 7)
	Version   string    `json:"version"`
	GoVersion string    `json:"go_version"`
	Build     BuildInfo `json:"build"`
	// 4. Runtime info (PART 6)
	Uptime    string `json:"uptime"`
	Mode      string `json:"mode"`
	Timestamp string `json:"timestamp"`
	// 5. Cluster info (PART 10)
	Cluster ClusterInfo `json:"cluster"`
	// 6. Features - PUBLIC only (PARTS 20, 32)
	Features FeaturesInfo `json:"features"`
	// 7. Component health checks
	Checks ChecksInfo `json:"checks"`
	// 8. Statistics (public-safe aggregates)
	Stats StatsInfo `json:"stats"`
}

// ProjectInfo represents project identification per AI.md PART 13.
// Name maps to branding.title; Tagline is the short slogan.
type ProjectInfo struct {
	Name        string `json:"name"`
	Tagline     string `json:"tagline"`
	Description string `json:"description"`
}

// BuildInfo represents build information per AI.md PART 13.
// Fields are "commit" and "date" per spec.
type BuildInfo struct {
	Commit string `json:"commit"`
	Date   string `json:"date"`
}

// FeaturesInfo represents PUBLIC feature status per AI.md PART 13.
// Only non-optional features are listed; optional features absent until implemented.
type FeaturesInfo struct {
	Tor   TorInfo `json:"tor"`
	GeoIP bool    `json:"geoip"`
}

// TorInfo represents Tor hidden service status per AI.md PART 13.
type TorInfo struct {
	Enabled  bool   `json:"enabled"`
	Running  bool   `json:"running"`
	Status   string `json:"status"`
	Hostname string `json:"hostname"`
}

// ChecksInfo represents component health per AI.md PART 13.
// Values are "ok" or "error" (or "disabled" when component not configured).
type ChecksInfo struct {
	Database  string `json:"database"`
	Cache     string `json:"cache"`
	Disk      string `json:"disk"`
	Scheduler string `json:"scheduler"`
	Cluster   string `json:"cluster,omitempty"`
	Tor       string `json:"tor,omitempty"`
}

// StatsInfo represents public-safe aggregate statistics per AI.md PART 13.
type StatsInfo struct {
	RequestsTotal int64 `json:"requests_total"`
	Requests24h   int64 `json:"requests_24h"`
	ActiveConns   int   `json:"active_connections"`
}

// ClusterInfo represents cluster status per AI.md PART 13.
// Status is "connected" or "disconnected"; Role is "primary" or "member".
type ClusterInfo struct {
	Enabled   bool     `json:"enabled"`
	Status    string   `json:"status,omitempty"`
	Primary   string   `json:"primary,omitempty"`
	Nodes     []string `json:"nodes,omitempty"`
	NodeCount int      `json:"node_count,omitempty"`
	Role      string   `json:"role,omitempty"`
}

// Pagination represents pagination information
type Pagination struct {
	CurrentPage int
	TotalPages  int
	HasPrev     bool
	HasNext     bool
	PrevPage    int
	NextPage    int
	Pages       []int
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
		AdminPath:   config.GetAdminPath(),
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

func formatSearchDate(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("2006-01-02")
}
