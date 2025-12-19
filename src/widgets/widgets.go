package widgets

import (
	"context"
	"sync"
	"time"

	"github.com/apimgr/search/src/config"
)

// WidgetType represents the type of widget
type WidgetType string

const (
	WidgetWeather    WidgetType = "weather"
	WidgetClock      WidgetType = "clock"
	WidgetQuickLinks WidgetType = "quicklinks"
	WidgetNotes      WidgetType = "notes"
	WidgetNews       WidgetType = "news"
	WidgetCalculator WidgetType = "calculator"
	WidgetCalendar   WidgetType = "calendar"
	WidgetConverter  WidgetType = "converter"
	WidgetStocks     WidgetType = "stocks"
	WidgetCrypto     WidgetType = "crypto"
	WidgetSports     WidgetType = "sports"
	WidgetRSS        WidgetType = "rss"
)

// WidgetCategory represents the category of a widget
type WidgetCategory string

const (
	CategoryData WidgetCategory = "data" // Requires API calls
	CategoryTool WidgetCategory = "tool" // Client-side only
	CategoryUser WidgetCategory = "user" // User-customizable, localStorage
)

// Widget represents a widget definition
type Widget struct {
	Type        WidgetType     `json:"type"`
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Icon        string         `json:"icon"`
	Category    WidgetCategory `json:"category"`
	Order       int            `json:"order"`
}

// WidgetData represents the data returned by a widget fetcher
type WidgetData struct {
	Type      WidgetType  `json:"type"`
	Data      interface{} `json:"data"`
	UpdatedAt time.Time   `json:"updated_at"`
	Error     string      `json:"error,omitempty"`
}

// Fetcher is the interface for widgets that fetch external data
type Fetcher interface {
	Fetch(ctx context.Context, params map[string]string) (*WidgetData, error)
	CacheDuration() time.Duration
	WidgetType() WidgetType
}

// Manager manages widgets and their data fetching
type Manager struct {
	mu       sync.RWMutex
	config   *config.WidgetsConfig
	cache    *Cache
	fetchers map[WidgetType]Fetcher
	widgets  map[WidgetType]*Widget
}

// NewManager creates a new widget manager
func NewManager(cfg *config.WidgetsConfig) *Manager {
	m := &Manager{
		config:   cfg,
		cache:    NewCache(),
		fetchers: make(map[WidgetType]Fetcher),
		widgets:  make(map[WidgetType]*Widget),
	}

	// Register built-in widget definitions
	m.registerWidgets()

	return m
}

// registerWidgets registers all available widget definitions
func (m *Manager) registerWidgets() {
	widgets := []*Widget{
		{Type: WidgetClock, Name: "Clock", Description: "Display current time and date", Icon: "clock", Category: CategoryTool, Order: 1},
		{Type: WidgetWeather, Name: "Weather", Description: "Current weather and forecast", Icon: "cloud-sun", Category: CategoryData, Order: 2},
		{Type: WidgetQuickLinks, Name: "Quick Links", Description: "Your favorite bookmarks", Icon: "link", Category: CategoryUser, Order: 3},
		{Type: WidgetCalculator, Name: "Calculator", Description: "Basic calculator", Icon: "calculator", Category: CategoryTool, Order: 4},
		{Type: WidgetNotes, Name: "Notes", Description: "Quick notes and reminders", Icon: "sticky-note", Category: CategoryUser, Order: 5},
		{Type: WidgetCalendar, Name: "Calendar", Description: "Monthly calendar view", Icon: "calendar", Category: CategoryTool, Order: 6},
		{Type: WidgetConverter, Name: "Unit Converter", Description: "Convert between units", Icon: "exchange-alt", Category: CategoryTool, Order: 7},
		{Type: WidgetNews, Name: "News", Description: "Latest news headlines", Icon: "newspaper", Category: CategoryData, Order: 8},
		{Type: WidgetStocks, Name: "Stocks", Description: "Stock market quotes", Icon: "chart-line", Category: CategoryData, Order: 9},
		{Type: WidgetCrypto, Name: "Crypto", Description: "Cryptocurrency prices", Icon: "bitcoin", Category: CategoryData, Order: 10},
		{Type: WidgetSports, Name: "Sports", Description: "Sports scores and updates", Icon: "futbol", Category: CategoryData, Order: 11},
		{Type: WidgetRSS, Name: "RSS Feeds", Description: "Custom RSS feeds", Icon: "rss", Category: CategoryUser, Order: 12},
	}

	for _, w := range widgets {
		m.widgets[w.Type] = w
	}
}

// RegisterFetcher registers a data fetcher for a widget type
func (m *Manager) RegisterFetcher(fetcher Fetcher) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.fetchers[fetcher.WidgetType()] = fetcher
}

// GetWidget returns a widget definition by type
func (m *Manager) GetWidget(widgetType WidgetType) *Widget {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.widgets[widgetType]
}

// GetAllWidgets returns all available widgets
func (m *Manager) GetAllWidgets() []*Widget {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*Widget, 0, len(m.widgets))
	for _, w := range m.widgets {
		result = append(result, w)
	}
	return result
}

// GetWidgetsByCategory returns widgets filtered by category
func (m *Manager) GetWidgetsByCategory(category WidgetCategory) []*Widget {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*Widget
	for _, w := range m.widgets {
		if w.Category == category {
			result = append(result, w)
		}
	}
	return result
}

// GetDefaultWidgets returns the default enabled widgets
func (m *Manager) GetDefaultWidgets() []string {
	if m.config != nil && len(m.config.DefaultWidgets) > 0 {
		return m.config.DefaultWidgets
	}
	// Default set
	return []string{"clock", "weather", "quicklinks", "calculator"}
}

// FetchWidgetData fetches data for a data widget
func (m *Manager) FetchWidgetData(ctx context.Context, widgetType WidgetType, params map[string]string) (*WidgetData, error) {
	m.mu.RLock()
	fetcher, ok := m.fetchers[widgetType]
	m.mu.RUnlock()

	if !ok {
		return &WidgetData{
			Type:      widgetType,
			Error:     "widget not available",
			UpdatedAt: time.Now(),
		}, nil
	}

	// Check cache first
	cacheKey := m.buildCacheKey(widgetType, params)
	if cached, ok := m.cache.Get(cacheKey); ok {
		return cached, nil
	}

	// Fetch fresh data
	data, err := fetcher.Fetch(ctx, params)
	if err != nil {
		return &WidgetData{
			Type:      widgetType,
			Error:     err.Error(),
			UpdatedAt: time.Now(),
		}, nil
	}

	// Cache the result
	m.cache.Set(cacheKey, data, fetcher.CacheDuration())

	return data, nil
}

// buildCacheKey builds a cache key from widget type and params
func (m *Manager) buildCacheKey(widgetType WidgetType, params map[string]string) string {
	key := string(widgetType)
	for k, v := range params {
		key += ":" + k + "=" + v
	}
	return key
}

// GetConfig returns the widgets configuration
func (m *Manager) GetConfig() *config.WidgetsConfig {
	return m.config
}

// IsEnabled returns whether widgets are enabled
func (m *Manager) IsEnabled() bool {
	return m.config != nil && m.config.Enabled
}

// IsWidgetEnabled checks if a specific widget type is enabled
func (m *Manager) IsWidgetEnabled(widgetType WidgetType) bool {
	if !m.IsEnabled() {
		return false
	}

	switch widgetType {
	case WidgetWeather:
		return m.config.Weather.Enabled
	case WidgetNews:
		return m.config.News.Enabled
	case WidgetStocks:
		return m.config.Stocks.Enabled
	case WidgetCrypto:
		return m.config.Crypto.Enabled
	case WidgetSports:
		return m.config.Sports.Enabled
	case WidgetRSS:
		return m.config.RSS.Enabled
	default:
		// Tool and user widgets are always available if widgets are enabled
		return true
	}
}
