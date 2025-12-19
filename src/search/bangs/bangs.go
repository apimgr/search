package bangs

import (
	"net/url"
	"regexp"
	"strings"
	"sync"
)

// Bang represents a search bang redirect
type Bang struct {
	Shortcut    string   `json:"shortcut" yaml:"shortcut"`
	Name        string   `json:"name" yaml:"name"`
	URL         string   `json:"url" yaml:"url"`
	Category    string   `json:"category" yaml:"category"`
	Description string   `json:"description,omitempty" yaml:"description,omitempty"`
	Icon        string   `json:"icon,omitempty" yaml:"icon,omitempty"`
	Aliases     []string `json:"aliases,omitempty" yaml:"aliases,omitempty"`
}

// BangResult represents the result of bang parsing
type BangResult struct {
	Bang       *Bang
	Query      string
	TargetURL  string
	IsBangOnly bool // Query was just the bang with no search terms
}

// Manager manages bangs from multiple sources
type Manager struct {
	mu       sync.RWMutex
	builtins map[string]*Bang
	custom   map[string]*Bang
	user     map[string]*Bang // per-request user bangs from localStorage
}

// NewManager creates a new bang manager with built-in defaults
func NewManager() *Manager {
	m := &Manager{
		builtins: make(map[string]*Bang),
		custom:   make(map[string]*Bang),
		user:     make(map[string]*Bang),
	}

	// Load built-in bangs
	for _, b := range defaultBangs {
		m.builtins[b.Shortcut] = b
		for _, alias := range b.Aliases {
			m.builtins[alias] = b
		}
	}

	return m
}

// SetCustomBangs sets custom bangs from server configuration
func (m *Manager) SetCustomBangs(bangs []*Bang) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.custom = make(map[string]*Bang)
	for _, b := range bangs {
		m.custom[b.Shortcut] = b
		for _, alias := range b.Aliases {
			m.custom[alias] = b
		}
	}
}

// Parse parses a query for bang commands
// Returns nil if no bang found
func (m *Manager) Parse(query string) *BangResult {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil
	}

	// Check for bang at start: !g query
	if strings.HasPrefix(query, "!") {
		return m.parseBangPrefix(query)
	}

	// Check for bang at end: query !g
	if idx := strings.LastIndex(query, " !"); idx > 0 {
		return m.parseBangSuffix(query, idx)
	}

	return nil
}

// parseBangPrefix handles "!g query" format
func (m *Manager) parseBangPrefix(query string) *BangResult {
	// Remove leading !
	rest := query[1:]

	// Find the bang shortcut (first word)
	parts := strings.SplitN(rest, " ", 2)
	shortcut := strings.ToLower(parts[0])

	bang := m.lookup(shortcut)
	if bang == nil {
		return nil
	}

	searchQuery := ""
	if len(parts) > 1 {
		searchQuery = strings.TrimSpace(parts[1])
	}

	return &BangResult{
		Bang:       bang,
		Query:      searchQuery,
		TargetURL:  m.buildURL(bang, searchQuery),
		IsBangOnly: searchQuery == "",
	}
}

// parseBangSuffix handles "query !g" format
func (m *Manager) parseBangSuffix(query string, idx int) *BangResult {
	searchQuery := strings.TrimSpace(query[:idx])
	bangPart := query[idx+2:] // Skip " !"

	// Get shortcut (may have trailing text)
	parts := strings.SplitN(bangPart, " ", 2)
	shortcut := strings.ToLower(parts[0])

	bang := m.lookup(shortcut)
	if bang == nil {
		return nil
	}

	return &BangResult{
		Bang:       bang,
		Query:      searchQuery,
		TargetURL:  m.buildURL(bang, searchQuery),
		IsBangOnly: searchQuery == "",
	}
}

// lookup finds a bang by shortcut, checking user -> custom -> builtin
func (m *Manager) lookup(shortcut string) *Bang {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Check user bangs first (highest priority)
	if b, ok := m.user[shortcut]; ok {
		return b
	}

	// Check custom bangs (server config)
	if b, ok := m.custom[shortcut]; ok {
		return b
	}

	// Check built-in bangs
	if b, ok := m.builtins[shortcut]; ok {
		return b
	}

	return nil
}

// buildURL builds the target URL with the search query
func (m *Manager) buildURL(bang *Bang, query string) string {
	if query == "" {
		// Just return base URL without query parameter
		// Extract base URL from template
		urlStr := bang.URL
		if idx := strings.Index(urlStr, "?"); idx > 0 {
			urlStr = urlStr[:idx]
		}
		return urlStr
	}

	// Replace {query} placeholder or append to URL
	if strings.Contains(bang.URL, "{query}") {
		return strings.ReplaceAll(bang.URL, "{query}", url.QueryEscape(query))
	}

	// If URL has %s placeholder (legacy format)
	if strings.Contains(bang.URL, "%s") {
		return strings.Replace(bang.URL, "%s", url.QueryEscape(query), 1)
	}

	// Append query parameter
	if strings.Contains(bang.URL, "?") {
		return bang.URL + "&q=" + url.QueryEscape(query)
	}
	return bang.URL + "?q=" + url.QueryEscape(query)
}

// SetUserBangs sets user-specific bangs (from localStorage)
func (m *Manager) SetUserBangs(bangs []*Bang) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.user = make(map[string]*Bang)
	for _, b := range bangs {
		m.user[b.Shortcut] = b
		for _, alias := range b.Aliases {
			m.user[alias] = b
		}
	}
}

// GetAll returns all available bangs (merged)
func (m *Manager) GetAll() []*Bang {
	m.mu.RLock()
	defer m.mu.RUnlock()

	seen := make(map[string]bool)
	var result []*Bang

	// Add user bangs first
	for _, b := range m.user {
		if !seen[b.Shortcut] {
			seen[b.Shortcut] = true
			result = append(result, b)
		}
	}

	// Add custom bangs
	for _, b := range m.custom {
		if !seen[b.Shortcut] {
			seen[b.Shortcut] = true
			result = append(result, b)
		}
	}

	// Add built-in bangs
	for _, b := range m.builtins {
		if !seen[b.Shortcut] {
			seen[b.Shortcut] = true
			result = append(result, b)
		}
	}

	return result
}

// GetBuiltins returns built-in bangs only
func (m *Manager) GetBuiltins() []*Bang {
	m.mu.RLock()
	defer m.mu.RUnlock()

	seen := make(map[string]bool)
	var result []*Bang

	for _, b := range m.builtins {
		if !seen[b.Shortcut] {
			seen[b.Shortcut] = true
			result = append(result, b)
		}
	}

	return result
}

// GetByCategory returns bangs filtered by category
func (m *Manager) GetByCategory(category string) []*Bang {
	all := m.GetAll()
	var result []*Bang
	for _, b := range all {
		if b.Category == category {
			result = append(result, b)
		}
	}
	return result
}

// GetCategories returns all unique categories
func (m *Manager) GetCategories() []string {
	all := m.GetAll()
	seen := make(map[string]bool)
	var result []string
	for _, b := range all {
		if b.Category != "" && !seen[b.Category] {
			seen[b.Category] = true
			result = append(result, b.Category)
		}
	}
	return result
}

// IsBang checks if a query contains a bang
func (m *Manager) IsBang(query string) bool {
	return m.Parse(query) != nil
}

// bangPattern matches bang syntax
var bangPattern = regexp.MustCompile(`(?:^!(\w+)|(\w+)!$|\s!(\w+)(?:\s|$))`)

// ExtractBang extracts just the bang shortcut without full parsing
func ExtractBang(query string) string {
	matches := bangPattern.FindStringSubmatch(query)
	if len(matches) > 1 {
		for _, m := range matches[1:] {
			if m != "" {
				return m
			}
		}
	}
	return ""
}
