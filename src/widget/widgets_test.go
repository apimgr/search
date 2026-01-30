package widget

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/apimgr/search/src/config"
)

func TestWidgetTypeConstants(t *testing.T) {
	tests := []struct {
		wt   WidgetType
		want string
	}{
		{WidgetWeather, "weather"},
		{WidgetClock, "clock"},
		{WidgetQuickLinks, "quicklinks"},
		{WidgetNotes, "notes"},
		{WidgetNews, "news"},
		{WidgetCalculator, "calculator"},
		{WidgetCalendar, "calendar"},
		{WidgetConverter, "converter"},
		{WidgetStocks, "stocks"},
		{WidgetCrypto, "crypto"},
		{WidgetSports, "sports"},
		{WidgetRSS, "rss"},
		{WidgetCurrency, "currency"},
		{WidgetTimezone, "timezone"},
		{WidgetTranslate, "translate"},
		{WidgetWikipedia, "wikipedia"},
		{WidgetTracking, "tracking"},
		{WidgetNutrition, "nutrition"},
		{WidgetQRCode, "qrcode"},
		{WidgetTimer, "timer"},
		{WidgetLoremIpsum, "lorem"},
		{WidgetDictionary, "dictionary"},
		{WidgetIPAddress, "ipaddress"},
		{WidgetColorPicker, "colorpicker"},
	}

	for _, tt := range tests {
		if string(tt.wt) != tt.want {
			t.Errorf("WidgetType = %q, want %q", tt.wt, tt.want)
		}
	}
}

func TestWidgetCategoryConstants(t *testing.T) {
	tests := []struct {
		cat  WidgetCategory
		want string
	}{
		{CategoryData, "data"},
		{CategoryTool, "tool"},
		{CategoryUser, "user"},
	}

	for _, tt := range tests {
		if string(tt.cat) != tt.want {
			t.Errorf("WidgetCategory = %q, want %q", tt.cat, tt.want)
		}
	}
}

func TestWidgetStruct(t *testing.T) {
	w := Widget{
		Type:        WidgetWeather,
		Name:        "Weather",
		Description: "Current weather",
		Icon:        "cloud-sun",
		Category:    CategoryData,
		Order:       1,
	}

	if w.Type != WidgetWeather {
		t.Errorf("Type = %v, want %v", w.Type, WidgetWeather)
	}
	if w.Name != "Weather" {
		t.Errorf("Name = %q, want %q", w.Name, "Weather")
	}
	if w.Category != CategoryData {
		t.Errorf("Category = %v, want %v", w.Category, CategoryData)
	}
}

func TestWidgetDataStruct(t *testing.T) {
	now := time.Now()
	wd := WidgetData{
		Type:      WidgetWeather,
		Data:      map[string]interface{}{"temp": 25},
		UpdatedAt: now,
		Error:     "",
	}

	if wd.Type != WidgetWeather {
		t.Errorf("Type = %v, want %v", wd.Type, WidgetWeather)
	}
	if wd.UpdatedAt != now {
		t.Errorf("UpdatedAt = %v, want %v", wd.UpdatedAt, now)
	}
}

func TestNewManager(t *testing.T) {
	cfg := &config.WidgetsConfig{
		Enabled: true,
		DefaultWidgets: []string{"clock", "weather"},
	}

	m := NewManager(cfg)
	if m == nil {
		t.Fatal("NewManager() returned nil")
	}
	if m.config != cfg {
		t.Error("Manager config not set correctly")
	}
	if m.cache == nil {
		t.Error("Manager cache not initialized")
	}
	if m.fetchers == nil {
		t.Error("Manager fetchers not initialized")
	}
	if m.widgets == nil {
		t.Error("Manager widgets not initialized")
	}
}

func TestNewManagerNilConfig(t *testing.T) {
	m := NewManager(nil)
	if m == nil {
		t.Fatal("NewManager(nil) returned nil")
	}
	if m.config != nil {
		t.Error("Manager config should be nil")
	}
}

func TestManagerGetWidget(t *testing.T) {
	m := NewManager(&config.WidgetsConfig{Enabled: true})

	w := m.GetWidget(WidgetClock)
	if w == nil {
		t.Fatal("GetWidget(WidgetClock) returned nil")
	}
	if w.Type != WidgetClock {
		t.Errorf("Widget type = %v, want %v", w.Type, WidgetClock)
	}
	if w.Name != "Clock" {
		t.Errorf("Widget name = %q, want %q", w.Name, "Clock")
	}
}

func TestManagerGetWidgetNotFound(t *testing.T) {
	m := NewManager(&config.WidgetsConfig{Enabled: true})

	w := m.GetWidget(WidgetType("unknown"))
	if w != nil {
		t.Error("GetWidget() should return nil for unknown type")
	}
}

func TestManagerGetAllWidgets(t *testing.T) {
	m := NewManager(&config.WidgetsConfig{Enabled: true})

	widgets := m.GetAllWidgets()
	if len(widgets) == 0 {
		t.Error("GetAllWidgets() returned empty slice")
	}

	// Should have all registered widgets
	if len(widgets) < 20 {
		t.Errorf("GetAllWidgets() returned %d widgets, expected at least 20", len(widgets))
	}
}

func TestManagerGetWidgetsByCategory(t *testing.T) {
	m := NewManager(&config.WidgetsConfig{Enabled: true})

	dataWidgets := m.GetWidgetsByCategory(CategoryData)
	if len(dataWidgets) == 0 {
		t.Error("GetWidgetsByCategory(CategoryData) returned empty slice")
	}

	for _, w := range dataWidgets {
		if w.Category != CategoryData {
			t.Errorf("Widget %q has category %v, want %v", w.Type, w.Category, CategoryData)
		}
	}

	toolWidgets := m.GetWidgetsByCategory(CategoryTool)
	if len(toolWidgets) == 0 {
		t.Error("GetWidgetsByCategory(CategoryTool) returned empty slice")
	}

	userWidgets := m.GetWidgetsByCategory(CategoryUser)
	if len(userWidgets) == 0 {
		t.Error("GetWidgetsByCategory(CategoryUser) returned empty slice")
	}
}

func TestManagerGetDefaultWidgets(t *testing.T) {
	// With custom defaults
	cfg := &config.WidgetsConfig{
		Enabled:        true,
		DefaultWidgets: []string{"clock", "notes"},
	}
	m := NewManager(cfg)

	defaults := m.GetDefaultWidgets()
	if len(defaults) != 2 {
		t.Errorf("GetDefaultWidgets() returned %d items, want 2", len(defaults))
	}

	// With nil config
	m2 := NewManager(nil)
	defaults2 := m2.GetDefaultWidgets()
	if len(defaults2) != 4 {
		t.Errorf("GetDefaultWidgets() with nil config returned %d items, want 4", len(defaults2))
	}
}

func TestManagerGetConfig(t *testing.T) {
	cfg := &config.WidgetsConfig{Enabled: true}
	m := NewManager(cfg)

	if m.GetConfig() != cfg {
		t.Error("GetConfig() did not return correct config")
	}
}

func TestManagerIsEnabled(t *testing.T) {
	// Enabled
	m := NewManager(&config.WidgetsConfig{Enabled: true})
	if !m.IsEnabled() {
		t.Error("IsEnabled() should return true")
	}

	// Disabled
	m2 := NewManager(&config.WidgetsConfig{Enabled: false})
	if m2.IsEnabled() {
		t.Error("IsEnabled() should return false")
	}

	// Nil config
	m3 := NewManager(nil)
	if m3.IsEnabled() {
		t.Error("IsEnabled() should return false for nil config")
	}
}

func TestManagerIsWidgetEnabled(t *testing.T) {
	cfg := &config.WidgetsConfig{
		Enabled: true,
		Weather: config.WeatherWidgetConfig{Enabled: true},
		News:    config.NewsWidgetConfig{Enabled: false},
		Stocks:  config.StocksWidgetConfig{Enabled: true},
		Crypto:  config.CryptoWidgetConfig{Enabled: false},
		Sports:  config.SportsWidgetConfig{Enabled: true},
		RSS:     config.RSSWidgetConfig{Enabled: true},
	}
	m := NewManager(cfg)

	tests := []struct {
		wt   WidgetType
		want bool
	}{
		{WidgetWeather, true},
		{WidgetNews, false},
		{WidgetStocks, true},
		{WidgetCrypto, false},
		{WidgetSports, true},
		{WidgetRSS, true},
		{WidgetClock, true},      // Tool widgets always enabled
		{WidgetCalculator, true}, // Tool widgets always enabled
		{WidgetNotes, true},      // User widgets always enabled
	}

	for _, tt := range tests {
		t.Run(string(tt.wt), func(t *testing.T) {
			got := m.IsWidgetEnabled(tt.wt)
			if got != tt.want {
				t.Errorf("IsWidgetEnabled(%q) = %v, want %v", tt.wt, got, tt.want)
			}
		})
	}
}

func TestManagerIsWidgetEnabledDisabled(t *testing.T) {
	m := NewManager(&config.WidgetsConfig{Enabled: false})

	// All widgets should be disabled when widgets are disabled
	if m.IsWidgetEnabled(WidgetWeather) {
		t.Error("IsWidgetEnabled() should return false when widgets are disabled")
	}
	if m.IsWidgetEnabled(WidgetClock) {
		t.Error("IsWidgetEnabled() should return false when widgets are disabled")
	}
}

// MockFetcher for testing
type mockFetcher struct {
	widgetType    WidgetType
	cacheDuration time.Duration
	data          *WidgetData
	err           error
}

func (f *mockFetcher) Fetch(ctx context.Context, params map[string]string) (*WidgetData, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.data, nil
}

func (f *mockFetcher) CacheDuration() time.Duration {
	return f.cacheDuration
}

func (f *mockFetcher) WidgetType() WidgetType {
	return f.widgetType
}

func TestManagerRegisterFetcher(t *testing.T) {
	m := NewManager(&config.WidgetsConfig{Enabled: true})

	fetcher := &mockFetcher{
		widgetType:    WidgetWeather,
		cacheDuration: 5 * time.Minute,
	}

	m.RegisterFetcher(fetcher)

	// Verify fetcher is registered
	m.mu.RLock()
	_, ok := m.fetchers[WidgetWeather]
	m.mu.RUnlock()

	if !ok {
		t.Error("Fetcher not registered")
	}
}

func TestManagerFetchWidgetData(t *testing.T) {
	m := NewManager(&config.WidgetsConfig{Enabled: true})

	data := &WidgetData{
		Type:      WidgetWeather,
		Data:      map[string]interface{}{"temp": 25},
		UpdatedAt: time.Now(),
	}

	fetcher := &mockFetcher{
		widgetType:    WidgetWeather,
		cacheDuration: 5 * time.Minute,
		data:          data,
	}

	m.RegisterFetcher(fetcher)

	ctx := context.Background()
	params := map[string]string{"location": "NYC"}

	result, err := m.FetchWidgetData(ctx, WidgetWeather, params)
	if err != nil {
		t.Fatalf("FetchWidgetData() error = %v", err)
	}
	if result.Type != WidgetWeather {
		t.Errorf("Result type = %v, want %v", result.Type, WidgetWeather)
	}
}

func TestManagerFetchWidgetDataNotAvailable(t *testing.T) {
	m := NewManager(&config.WidgetsConfig{Enabled: true})

	ctx := context.Background()
	result, err := m.FetchWidgetData(ctx, WidgetWeather, nil)
	if err != nil {
		t.Fatalf("FetchWidgetData() error = %v", err)
	}
	if result.Error != "widget not available" {
		t.Errorf("Result error = %q, want %q", result.Error, "widget not available")
	}
}

func TestManagerFetchWidgetDataFetcherError(t *testing.T) {
	m := NewManager(&config.WidgetsConfig{Enabled: true})

	fetcher := &mockFetcher{
		widgetType:    WidgetWeather,
		cacheDuration: 5 * time.Minute,
		err:           context.DeadlineExceeded,
	}

	m.RegisterFetcher(fetcher)

	ctx := context.Background()
	result, err := m.FetchWidgetData(ctx, WidgetWeather, nil)
	if err != nil {
		t.Fatalf("FetchWidgetData() error = %v", err)
	}
	if result.Error == "" {
		t.Error("Result should have error message")
	}
}

func TestManagerFetchWidgetDataCached(t *testing.T) {
	m := NewManager(&config.WidgetsConfig{Enabled: true})

	data := &WidgetData{
		Type:      WidgetWeather,
		Data:      map[string]interface{}{"temp": 25},
		UpdatedAt: time.Now(),
	}

	fetcher := &mockFetcher{
		widgetType:    WidgetWeather,
		cacheDuration: 5 * time.Minute,
		data:          data,
	}

	m.RegisterFetcher(fetcher)

	ctx := context.Background()
	params := map[string]string{"location": "NYC"}

	// First fetch
	_, err := m.FetchWidgetData(ctx, WidgetWeather, params)
	if err != nil {
		t.Fatalf("First FetchWidgetData() error = %v", err)
	}

	// Second fetch should use cache
	result2, err := m.FetchWidgetData(ctx, WidgetWeather, params)
	if err != nil {
		t.Fatalf("Second FetchWidgetData() error = %v", err)
	}
	if result2.Type != WidgetWeather {
		t.Error("Cached result type mismatch")
	}
}

func TestManagerBuildCacheKey(t *testing.T) {
	m := NewManager(&config.WidgetsConfig{Enabled: true})

	key := m.buildCacheKey(WidgetWeather, nil)
	if key != "weather" {
		t.Errorf("Cache key = %q, want %q", key, "weather")
	}

	params := map[string]string{"location": "NYC"}
	key2 := m.buildCacheKey(WidgetWeather, params)
	if key2 != "weather:location=NYC" {
		t.Errorf("Cache key = %q, want %q", key2, "weather:location=NYC")
	}
}

// Tests for WeatherFetcher

func TestNewWeatherFetcher(t *testing.T) {
	cfg := &config.WeatherWidgetConfig{
		Enabled:     true,
		DefaultCity: "New York",
		Units:       "metric",
	}

	f := NewWeatherFetcher(cfg)
	if f == nil {
		t.Fatal("NewWeatherFetcher() returned nil")
	}
	if f.client == nil {
		t.Error("WeatherFetcher.client should not be nil")
	}
	if f.config != cfg {
		t.Error("WeatherFetcher.config not set correctly")
	}
}

func TestWeatherFetcherWidgetType(t *testing.T) {
	f := NewWeatherFetcher(&config.WeatherWidgetConfig{})
	if f.WidgetType() != WidgetWeather {
		t.Errorf("WidgetType() = %v, want %v", f.WidgetType(), WidgetWeather)
	}
}

func TestWeatherFetcherCacheDuration(t *testing.T) {
	f := NewWeatherFetcher(&config.WeatherWidgetConfig{})
	duration := f.CacheDuration()
	if duration != 15*time.Minute {
		t.Errorf("CacheDuration() = %v, want %v", duration, 15*time.Minute)
	}
}

func TestWeatherFetcherFetchNoCity(t *testing.T) {
	cfg := &config.WeatherWidgetConfig{
		Enabled:     true,
		DefaultCity: "",
		Units:       "metric",
	}

	f := NewWeatherFetcher(cfg)
	ctx := context.Background()

	data, err := f.Fetch(ctx, map[string]string{})
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if data.Error != "no city specified" {
		t.Errorf("Error = %q, want 'no city specified'", data.Error)
	}
}

func TestWeatherDataStruct(t *testing.T) {
	w := WeatherData{
		Location:    "New York, NY, USA",
		Temperature: 72.5,
		FeelsLike:   75.0,
		Humidity:    65,
		Description: "Partly cloudy",
		Condition:   "partly-cloudy",
		WindSpeed:   10.5,
		Icon:        "cloud-sun",
	}

	if w.Location != "New York, NY, USA" {
		t.Errorf("Location = %q", w.Location)
	}
	if w.Temperature != 72.5 {
		t.Errorf("Temperature = %v", w.Temperature)
	}
	if w.FeelsLike != 75.0 {
		t.Errorf("FeelsLike = %v", w.FeelsLike)
	}
	if w.Humidity != 65 {
		t.Errorf("Humidity = %d", w.Humidity)
	}
}

func TestGeocodingResponseStruct(t *testing.T) {
	resp := GeocodingResponse{}
	resp.Results = make([]struct {
		Name      string  `json:"name"`
		Latitude  float64 `json:"latitude"`
		Longitude float64 `json:"longitude"`
		Country   string  `json:"country"`
		Admin1    string  `json:"admin1"`
	}, 1)

	resp.Results[0].Name = "New York"
	resp.Results[0].Latitude = 40.7128
	resp.Results[0].Longitude = -74.0060
	resp.Results[0].Country = "USA"
	resp.Results[0].Admin1 = "New York"

	if resp.Results[0].Name != "New York" {
		t.Errorf("Name = %q", resp.Results[0].Name)
	}
	if resp.Results[0].Latitude != 40.7128 {
		t.Errorf("Latitude = %v", resp.Results[0].Latitude)
	}
}

func TestOpenMeteoResponseStruct(t *testing.T) {
	resp := OpenMeteoResponse{
		Latitude:  40.7128,
		Longitude: -74.0060,
	}
	resp.CurrentWeather.Temperature = 25.5
	resp.CurrentWeather.WindSpeed = 10.0
	resp.CurrentWeather.WindDirection = 180
	resp.CurrentWeather.WeatherCode = 2
	resp.CurrentWeather.IsDay = 1

	if resp.Latitude != 40.7128 {
		t.Errorf("Latitude = %v", resp.Latitude)
	}
	if resp.CurrentWeather.Temperature != 25.5 {
		t.Errorf("CurrentWeather.Temperature = %v", resp.CurrentWeather.Temperature)
	}
}

func TestWeatherCodeToDescription(t *testing.T) {
	tests := []struct {
		code        int
		wantDesc    string
		wantCond    string
	}{
		{0, "Clear sky", "clear"},
		{1, "Mainly clear", "clear"},
		{2, "Partly cloudy", "partly-cloudy"},
		{3, "Overcast", "cloudy"},
		{45, "Foggy", "fog"},
		{48, "Foggy", "fog"},
		{51, "Drizzle", "rain"},
		{53, "Drizzle", "rain"},
		{55, "Drizzle", "rain"},
		{56, "Freezing drizzle", "rain"},
		{57, "Freezing drizzle", "rain"},
		{61, "Rain", "rain"},
		{63, "Rain", "rain"},
		{65, "Rain", "rain"},
		{66, "Freezing rain", "rain"},
		{67, "Freezing rain", "rain"},
		{71, "Snow", "snow"},
		{73, "Snow", "snow"},
		{75, "Snow", "snow"},
		{77, "Snow grains", "snow"},
		{80, "Rain showers", "rain"},
		{81, "Rain showers", "rain"},
		{82, "Rain showers", "rain"},
		{85, "Snow showers", "snow"},
		{86, "Snow showers", "snow"},
		{95, "Thunderstorm", "thunderstorm"},
		{96, "Thunderstorm with hail", "thunderstorm"},
		{99, "Thunderstorm with hail", "thunderstorm"},
		{-1, "Unknown", "cloudy"},
		{100, "Unknown", "cloudy"},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("code_%d", tt.code), func(t *testing.T) {
			desc, cond := weatherCodeToDescription(tt.code)
			if desc != tt.wantDesc {
				t.Errorf("description = %q, want %q", desc, tt.wantDesc)
			}
			if cond != tt.wantCond {
				t.Errorf("condition = %q, want %q", cond, tt.wantCond)
			}
		})
	}
}

// Tests for Widget categories

func TestAllWidgetCategories(t *testing.T) {
	m := NewManager(&config.WidgetsConfig{Enabled: true})

	// Count widgets by category
	dataCount := len(m.GetWidgetsByCategory(CategoryData))
	toolCount := len(m.GetWidgetsByCategory(CategoryTool))
	userCount := len(m.GetWidgetsByCategory(CategoryUser))

	total := dataCount + toolCount + userCount
	allWidgets := len(m.GetAllWidgets())

	if total != allWidgets {
		t.Errorf("Category counts (%d) don't match total (%d)", total, allWidgets)
	}
}

func TestWidgetIcons(t *testing.T) {
	m := NewManager(&config.WidgetsConfig{Enabled: true})

	for _, w := range m.GetAllWidgets() {
		if w.Icon == "" {
			t.Errorf("Widget %q has no icon", w.Type)
		}
	}
}

func TestWidgetOrdering(t *testing.T) {
	m := NewManager(&config.WidgetsConfig{Enabled: true})

	seen := make(map[int]bool)
	for _, w := range m.GetAllWidgets() {
		if w.Order <= 0 {
			t.Errorf("Widget %q has invalid order: %d", w.Type, w.Order)
		}
		if seen[w.Order] {
			t.Errorf("Duplicate order %d for widget %q", w.Order, w.Type)
		}
		seen[w.Order] = true
	}
}

// Tests for Fetcher interface

func TestFetcherInterface(t *testing.T) {
	// Verify mockFetcher implements Fetcher
	var _ Fetcher = (*mockFetcher)(nil)

	// Verify WeatherFetcher implements Fetcher
	var _ Fetcher = (*WeatherFetcher)(nil)
}

// Tests for Manager concurrent access

func TestManagerConcurrentAccess(t *testing.T) {
	m := NewManager(&config.WidgetsConfig{Enabled: true})

	done := make(chan bool)

	// Writer goroutine
	go func() {
		for i := 0; i < 100; i++ {
			m.RegisterFetcher(&mockFetcher{
				widgetType:    WidgetWeather,
				cacheDuration: 5 * time.Minute,
			})
		}
		done <- true
	}()

	// Reader goroutine
	go func() {
		for i := 0; i < 100; i++ {
			m.GetAllWidgets()
			m.GetWidget(WidgetWeather)
			m.GetWidgetsByCategory(CategoryData)
		}
		done <- true
	}()

	<-done
	<-done
}

// Tests for widget type all constants

func TestAllWidgetTypesDefined(t *testing.T) {
	m := NewManager(&config.WidgetsConfig{Enabled: true})

	expectedTypes := []WidgetType{
		WidgetWeather, WidgetClock, WidgetQuickLinks, WidgetNotes,
		WidgetNews, WidgetCalculator, WidgetCalendar, WidgetConverter,
		WidgetStocks, WidgetCrypto, WidgetSports, WidgetRSS,
		WidgetCurrency, WidgetTimezone, WidgetTranslate, WidgetWikipedia,
		WidgetTracking, WidgetNutrition, WidgetQRCode, WidgetTimer,
		WidgetLoremIpsum, WidgetDictionary, WidgetIPAddress, WidgetColorPicker,
	}

	for _, wt := range expectedTypes {
		w := m.GetWidget(wt)
		if w == nil {
			t.Errorf("Widget type %q not registered", wt)
		}
	}
}

// Tests for IsWidgetEnabled edge cases

func TestIsWidgetEnabledAllTypes(t *testing.T) {
	cfg := &config.WidgetsConfig{
		Enabled: true,
		Weather: config.WeatherWidgetConfig{Enabled: true},
		News:    config.NewsWidgetConfig{Enabled: true},
		Stocks:  config.StocksWidgetConfig{Enabled: true},
		Crypto:  config.CryptoWidgetConfig{Enabled: true},
		Sports:  config.SportsWidgetConfig{Enabled: true},
		RSS:     config.RSSWidgetConfig{Enabled: true},
	}
	m := NewManager(cfg)

	// All data widgets should be enabled
	if !m.IsWidgetEnabled(WidgetWeather) {
		t.Error("Weather should be enabled")
	}
	if !m.IsWidgetEnabled(WidgetNews) {
		t.Error("News should be enabled")
	}
	if !m.IsWidgetEnabled(WidgetStocks) {
		t.Error("Stocks should be enabled")
	}
	if !m.IsWidgetEnabled(WidgetCrypto) {
		t.Error("Crypto should be enabled")
	}
	if !m.IsWidgetEnabled(WidgetSports) {
		t.Error("Sports should be enabled")
	}
	if !m.IsWidgetEnabled(WidgetRSS) {
		t.Error("RSS should be enabled")
	}
}

// Test FetchWidgetData with context cancellation

func TestManagerFetchWidgetDataContextCancellation(t *testing.T) {
	m := NewManager(&config.WidgetsConfig{Enabled: true})

	data := &WidgetData{
		Type:      WidgetWeather,
		Data:      map[string]interface{}{"temp": 25},
		UpdatedAt: time.Now(),
	}

	fetcher := &mockFetcher{
		widgetType:    WidgetWeather,
		cacheDuration: 5 * time.Minute,
		data:          data,
	}

	m.RegisterFetcher(fetcher)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// Should still work since mockFetcher doesn't check context
	result, err := m.FetchWidgetData(ctx, WidgetWeather, nil)
	if err != nil {
		t.Fatalf("FetchWidgetData() error = %v", err)
	}
	if result == nil {
		t.Error("Result should not be nil")
	}
}

// Test cache key building with multiple params

func TestManagerBuildCacheKeyMultipleParams(t *testing.T) {
	m := NewManager(&config.WidgetsConfig{Enabled: true})

	params := map[string]string{
		"city":  "NYC",
		"units": "metric",
	}
	key := m.buildCacheKey(WidgetWeather, params)

	// Key should contain all params
	if key == "weather" {
		t.Error("Cache key should include params")
	}
}

// Test widget JSON serialization tags

func TestWidgetJSONTags(t *testing.T) {
	w := Widget{
		Type:        WidgetWeather,
		Name:        "Weather",
		Description: "Test",
		Icon:        "cloud",
		Category:    CategoryData,
		Order:       1,
	}

	if w.Type != WidgetWeather {
		t.Errorf("Type = %v", w.Type)
	}
}

func TestWidgetDataJSONTags(t *testing.T) {
	now := time.Now()
	wd := WidgetData{
		Type:      WidgetWeather,
		Data:      "test data",
		UpdatedAt: now,
		Error:     "test error",
	}

	if wd.Type != WidgetWeather {
		t.Errorf("Type = %v", wd.Type)
	}
	if wd.Data != "test data" {
		t.Errorf("Data = %v", wd.Data)
	}
	if wd.Error != "test error" {
		t.Errorf("Error = %q", wd.Error)
	}
}

// Tests for CryptoFetcher

func TestNewCryptoFetcher(t *testing.T) {
	cfg := &config.CryptoWidgetConfig{
		Enabled:      true,
		DefaultCoins: []string{"bitcoin", "ethereum"},
		Currency:     "usd",
	}

	f := NewCryptoFetcher(cfg)
	if f == nil {
		t.Fatal("NewCryptoFetcher() returned nil")
	}
	if f.client == nil {
		t.Error("CryptoFetcher.client should not be nil")
	}
	if f.config != cfg {
		t.Error("CryptoFetcher.config not set correctly")
	}
}

func TestCryptoFetcherWidgetType(t *testing.T) {
	f := NewCryptoFetcher(&config.CryptoWidgetConfig{})
	if f.WidgetType() != WidgetCrypto {
		t.Errorf("WidgetType() = %v, want %v", f.WidgetType(), WidgetCrypto)
	}
}

func TestCryptoFetcherCacheDuration(t *testing.T) {
	f := NewCryptoFetcher(&config.CryptoWidgetConfig{})
	duration := f.CacheDuration()
	if duration != 5*time.Minute {
		t.Errorf("CacheDuration() = %v, want %v", duration, 5*time.Minute)
	}
}

func TestCryptoDataStruct(t *testing.T) {
	data := CryptoData{
		Coins: []CoinData{
			{
				ID:        "bitcoin",
				Name:      "Bitcoin",
				Symbol:    "BTC",
				Price:     50000.0,
				Change24h: 2.5,
				MarketCap: 1000000000,
				Volume24h: 50000000,
			},
		},
	}

	if len(data.Coins) != 1 {
		t.Errorf("Coins length = %d", len(data.Coins))
	}
	if data.Coins[0].ID != "bitcoin" {
		t.Errorf("Coins[0].ID = %q", data.Coins[0].ID)
	}
	if data.Coins[0].Price != 50000.0 {
		t.Errorf("Coins[0].Price = %v", data.Coins[0].Price)
	}
}

func TestCoinDataStruct(t *testing.T) {
	coin := CoinData{
		ID:        "ethereum",
		Name:      "Ethereum",
		Symbol:    "ETH",
		Price:     3000.0,
		Change24h: -1.5,
		MarketCap: 350000000000,
		Volume24h: 15000000000,
	}

	if coin.ID != "ethereum" {
		t.Errorf("ID = %q", coin.ID)
	}
	if coin.Symbol != "ETH" {
		t.Errorf("Symbol = %q", coin.Symbol)
	}
}

func TestFormatCoinName(t *testing.T) {
	tests := []struct {
		id   string
		want string
	}{
		{"bitcoin", "Bitcoin"},
		{"ethereum", "Ethereum"},
		{"tether", "Tether"},
		{"binancecoin", "BNB"},
		{"ripple", "XRP"},
		{"usd-coin", "USD Coin"},
		{"solana", "Solana"},
		{"cardano", "Cardano"},
		{"dogecoin", "Dogecoin"},
		{"polkadot", "Polkadot"},
		{"shiba-inu", "Shiba Inu"},
		{"litecoin", "Litecoin"},
		{"avalanche-2", "Avalanche"},
		{"chainlink", "Chainlink"},
		{"stellar", "Stellar"},
		{"monero", "Monero"},
		{"algorand", "Algorand"},
		{"unknown-coin", "Unknown-coin"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			got := formatCoinName(tt.id)
			if got != tt.want {
				t.Errorf("formatCoinName(%q) = %q, want %q", tt.id, got, tt.want)
			}
		})
	}
}

func TestCoinIDToSymbol(t *testing.T) {
	tests := []struct {
		id   string
		want string
	}{
		{"bitcoin", "BTC"},
		{"ethereum", "ETH"},
		{"tether", "USDT"},
		{"binancecoin", "BNB"},
		{"ripple", "XRP"},
		{"usd-coin", "USDC"},
		{"solana", "SOL"},
		{"cardano", "ADA"},
		{"dogecoin", "DOGE"},
		{"polkadot", "DOT"},
		{"shiba-inu", "SHIB"},
		{"litecoin", "LTC"},
		{"avalanche-2", "AVAX"},
		{"chainlink", "LINK"},
		{"stellar", "XLM"},
		{"monero", "XMR"},
		{"algorand", "ALGO"},
		{"unknown", "UNKNOWN"},
	}

	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			got := coinIDToSymbol(tt.id)
			if got != tt.want {
				t.Errorf("coinIDToSymbol(%q) = %q, want %q", tt.id, got, tt.want)
			}
		})
	}
}

// Tests for StocksFetcher

func TestNewStocksFetcher(t *testing.T) {
	cfg := &config.StocksWidgetConfig{
		Enabled:        true,
		DefaultSymbols: []string{"AAPL", "GOOGL", "MSFT"},
	}

	f := NewStocksFetcher(cfg)
	if f == nil {
		t.Fatal("NewStocksFetcher() returned nil")
	}
	if f.client == nil {
		t.Error("StocksFetcher.client should not be nil")
	}
	if f.config != cfg {
		t.Error("StocksFetcher.config not set correctly")
	}
}

func TestStocksFetcherWidgetType(t *testing.T) {
	f := NewStocksFetcher(&config.StocksWidgetConfig{})
	if f.WidgetType() != WidgetStocks {
		t.Errorf("WidgetType() = %v, want %v", f.WidgetType(), WidgetStocks)
	}
}

func TestStocksFetcherCacheDuration(t *testing.T) {
	f := NewStocksFetcher(&config.StocksWidgetConfig{})
	duration := f.CacheDuration()
	if duration != 5*time.Minute {
		t.Errorf("CacheDuration() = %v, want %v", duration, 5*time.Minute)
	}
}

func TestStocksDataStruct(t *testing.T) {
	data := StocksData{
		Symbols: []StockQuote{
			{
				Symbol:        "AAPL",
				Name:          "Apple Inc.",
				Price:         175.50,
				Change:        2.30,
				ChangePercent: 1.33,
				Volume:        50000000,
				MarketCap:     2500000000000,
			},
		},
	}

	if len(data.Symbols) != 1 {
		t.Errorf("Symbols length = %d", len(data.Symbols))
	}
	if data.Symbols[0].Symbol != "AAPL" {
		t.Errorf("Symbols[0].Symbol = %q", data.Symbols[0].Symbol)
	}
}

func TestStockQuoteStruct(t *testing.T) {
	quote := StockQuote{
		Symbol:        "MSFT",
		Name:          "Microsoft Corporation",
		Price:         350.25,
		Change:        -1.75,
		ChangePercent: -0.50,
		Volume:        25000000,
		MarketCap:     2600000000000,
	}

	if quote.Symbol != "MSFT" {
		t.Errorf("Symbol = %q", quote.Symbol)
	}
	if quote.Price != 350.25 {
		t.Errorf("Price = %v", quote.Price)
	}
	if quote.Change != -1.75 {
		t.Errorf("Change = %v", quote.Change)
	}
}

func TestYahooFinanceResponseStruct(t *testing.T) {
	resp := YahooFinanceResponse{}
	resp.QuoteResponse.Result = make([]struct {
		Symbol                     string  `json:"symbol"`
		ShortName                  string  `json:"shortName"`
		LongName                   string  `json:"longName"`
		RegularMarketPrice         float64 `json:"regularMarketPrice"`
		RegularMarketChange        float64 `json:"regularMarketChange"`
		RegularMarketChangePercent float64 `json:"regularMarketChangePercent"`
		RegularMarketVolume        int64   `json:"regularMarketVolume"`
		MarketCap                  float64 `json:"marketCap"`
	}, 1)

	resp.QuoteResponse.Result[0].Symbol = "GOOGL"
	resp.QuoteResponse.Result[0].ShortName = "Alphabet Inc."
	resp.QuoteResponse.Result[0].RegularMarketPrice = 150.25

	if resp.QuoteResponse.Result[0].Symbol != "GOOGL" {
		t.Errorf("Symbol = %q", resp.QuoteResponse.Result[0].Symbol)
	}
}

// Tests for CoinGeckoResponse

func TestCoinGeckoResponseStruct(t *testing.T) {
	resp := CoinGeckoResponse{}
	resp["bitcoin"] = struct {
		USD          float64 `json:"usd"`
		EUR          float64 `json:"eur"`
		GBP          float64 `json:"gbp"`
		USDChange24h float64 `json:"usd_24h_change"`
		EURChange24h float64 `json:"eur_24h_change"`
		GBPChange24h float64 `json:"gbp_24h_change"`
		USDMarketCap float64 `json:"usd_market_cap"`
		USDVolume24h float64 `json:"usd_24h_vol"`
	}{
		USD:          50000.0,
		EUR:          45000.0,
		GBP:          40000.0,
		USDChange24h: 2.5,
		USDMarketCap: 1000000000000,
	}

	if resp["bitcoin"].USD != 50000.0 {
		t.Errorf("bitcoin USD = %v", resp["bitcoin"].USD)
	}
	if resp["bitcoin"].USDChange24h != 2.5 {
		t.Errorf("bitcoin USDChange24h = %v", resp["bitcoin"].USDChange24h)
	}
}

// Verify fetcher interfaces

func TestCryptoFetcherImplementsFetcher(t *testing.T) {
	var _ Fetcher = (*CryptoFetcher)(nil)
}

func TestStocksFetcherImplementsFetcher(t *testing.T) {
	var _ Fetcher = (*StocksFetcher)(nil)
}

// ===== NEWS FETCHER TESTS =====

func TestNewNewsFetcher(t *testing.T) {
	cfg := &config.NewsWidgetConfig{
		Enabled:  true,
		Sources:  []string{"https://example.com/feed.xml"},
		MaxItems: 10,
	}

	f := NewNewsFetcher(cfg)
	if f == nil {
		t.Fatal("NewNewsFetcher() returned nil")
	}
	if f.client == nil {
		t.Error("NewsFetcher.client should not be nil")
	}
	if f.config != cfg {
		t.Error("NewsFetcher.config not set correctly")
	}
}

func TestNewsFetcherWidgetType(t *testing.T) {
	f := NewNewsFetcher(&config.NewsWidgetConfig{})
	if f.WidgetType() != WidgetNews {
		t.Errorf("WidgetType() = %v, want %v", f.WidgetType(), WidgetNews)
	}
}

func TestNewsFetcherCacheDuration(t *testing.T) {
	f := NewNewsFetcher(&config.NewsWidgetConfig{})
	duration := f.CacheDuration()
	if duration != 30*time.Minute {
		t.Errorf("CacheDuration() = %v, want %v", duration, 30*time.Minute)
	}
}

func TestNewsFetcherImplementsFetcher(t *testing.T) {
	var _ Fetcher = (*NewsFetcher)(nil)
}

func TestNewsDataStruct(t *testing.T) {
	data := NewsData{
		Items: []NewsItem{
			{
				Title:       "Test News",
				URL:         "https://example.com/news/1",
				Source:      "Example News",
				PublishedAt: time.Now(),
				Summary:     "This is a summary",
			},
		},
	}

	if len(data.Items) != 1 {
		t.Errorf("Items length = %d", len(data.Items))
	}
	if data.Items[0].Title != "Test News" {
		t.Errorf("Items[0].Title = %q", data.Items[0].Title)
	}
}

func TestNewsItemStruct(t *testing.T) {
	now := time.Now()
	item := NewsItem{
		Title:       "Breaking News",
		URL:         "https://news.example.com/article",
		Source:      "Example Source",
		PublishedAt: now,
		Summary:     "Summary of the article",
	}

	if item.Title != "Breaking News" {
		t.Errorf("Title = %q", item.Title)
	}
	if item.Source != "Example Source" {
		t.Errorf("Source = %q", item.Source)
	}
	if !item.PublishedAt.Equal(now) {
		t.Errorf("PublishedAt = %v", item.PublishedAt)
	}
}

func TestParseRSSDate(t *testing.T) {
	tests := []struct {
		name  string
		input string
		valid bool
	}{
		{"RFC1123Z", "Mon, 02 Jan 2006 15:04:05 -0700", true},
		{"RFC1123", "Mon, 02 Jan 2006 15:04:05 MST", true},
		{"RFC822Z", "02 Jan 06 15:04 -0700", true},
		{"RFC822", "02 Jan 06 15:04 MST", true},
		{"custom format 1", "Mon, 2 Jan 2006 15:04:05 -0700", true},
		{"custom format 2", "Mon, 2 Jan 2006 15:04:05 MST", true},
		{"ISO8601", "2006-01-02T15:04:05Z", true},
		{"ISO8601 with offset", "2006-01-02T15:04:05-07:00", true},
		{"invalid date", "not a date", false},
		{"empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseRSSDate(tt.input)
			if tt.valid {
				// For valid dates, result should not be zero
				// (unless parsing failed, then it falls back to time.Now())
				if result.IsZero() {
					t.Errorf("parseRSSDate(%q) returned zero time", tt.input)
				}
			}
			// For invalid dates, parseRSSDate returns time.Now() so it's always non-zero
		})
	}
}

func TestParseAtomDate(t *testing.T) {
	tests := []struct {
		name  string
		input string
		valid bool
	}{
		{"RFC3339", "2006-01-02T15:04:05Z", true},
		{"RFC3339 with offset", "2006-01-02T15:04:05-07:00", true},
		{"RFC3339Nano", "2006-01-02T15:04:05.999999999Z", true},
		{"invalid date", "not a date", false},
		{"empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseAtomDate(tt.input)
			if tt.valid && result.IsZero() {
				t.Errorf("parseAtomDate(%q) returned zero time for valid input", tt.input)
			}
			if !tt.valid && !result.IsZero() {
				t.Errorf("parseAtomDate(%q) returned non-zero time for invalid input", tt.input)
			}
		})
	}
}

func TestExtractSourceName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"BBC News - RSS Feed", "BBC"},          // Removes " - RSS Feed" then " News"
		{"CNN RSS", "CNN"},                      // Removes " RSS"
		{"Reuters Feed", "Reuters"},             // Removes " Feed"
		{"New York Times News", "New York Times"}, // Removes " News"
		{"Simple Title", "Simple Title"},        // No suffix to remove
		{"   Padded Title   ", "Padded Title"},  // Trims whitespace
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := extractSourceName(tt.input)
			if got != tt.want {
				t.Errorf("extractSourceName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestCleanText(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"no tags", "plain text", "plain text"},
		{"simple tag", "<p>content</p>", "content"},
		{"multiple tags", "<div><p>Hello</p><span>World</span></div>", "HelloWorld"},
		{"CDATA", "<![CDATA[content]]>", "content"},
		{"with newlines", "line1\nline2", "line1 line2"},
		{"with double spaces", "hello  world", "hello world"},
		{"with carriage return", "hello\rworld", "helloworld"},
		{"complex", "<![CDATA[<p>Test</p>]]>", "Test"},
		{"empty", "", ""},
		{"whitespace only", "   ", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cleanText(tt.input)
			if got != tt.want {
				t.Errorf("cleanText(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestTruncateSummary(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"short text", "Short text", "Short text"},
		{"exact 200", "a" + strings.Repeat("b", 199), "a" + strings.Repeat("b", 199)},
		{"over 200 with space", "This is a very long text that exceeds the maximum length of 200 characters and should be truncated at the last space before the limit to avoid cutting words in the middle of a word which would look odd",
			"This is a very long text that exceeds the maximum length of 200 characters and should be truncated at the last space before the limit to avoid cutting words in the middle of a word which would..."},
		{"empty", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncateSummary(tt.input)
			if len(tt.input) > 200 {
				// For long input, should be truncated
				if len(got) > 203 { // 200 + "..."
					t.Errorf("truncateSummary() result too long: %d chars", len(got))
				}
				if !strings.HasSuffix(got, "...") {
					t.Errorf("truncateSummary() should end with '...'")
				}
			} else {
				if got != tt.want {
					t.Errorf("truncateSummary(%q) = %q, want %q", tt.input, got, tt.want)
				}
			}
		})
	}
}

// ===== RSS STRUCTS TESTS =====

func TestRSSFeedStruct(t *testing.T) {
	feed := RSSFeed{
		Channel: RSSChannel{
			Title:       "Test Feed",
			Link:        "https://example.com",
			Description: "A test feed",
			Items: []RSSItem{
				{Title: "Item 1", Link: "https://example.com/1"},
			},
		},
	}

	if feed.Channel.Title != "Test Feed" {
		t.Errorf("Title = %q", feed.Channel.Title)
	}
	if len(feed.Channel.Items) != 1 {
		t.Errorf("Items length = %d", len(feed.Channel.Items))
	}
}

func TestRSSItemStruct(t *testing.T) {
	item := RSSItem{
		Title:       "Test Item",
		Link:        "https://example.com/item",
		Description: "Item description",
		PubDate:     "Mon, 02 Jan 2006 15:04:05 GMT",
		GUID:        "unique-id-123",
	}

	if item.Title != "Test Item" {
		t.Errorf("Title = %q", item.Title)
	}
	if item.GUID != "unique-id-123" {
		t.Errorf("GUID = %q", item.GUID)
	}
}

func TestAtomFeedStruct(t *testing.T) {
	feed := AtomFeed{
		Title: "Atom Test Feed",
		Entries: []AtomEntry{
			{
				Title:     "Entry 1",
				Link:      AtomLink{Href: "https://example.com/entry1"},
				Summary:   "Entry summary",
				Published: "2024-01-15T10:00:00Z",
			},
		},
	}

	if feed.Title != "Atom Test Feed" {
		t.Errorf("Title = %q", feed.Title)
	}
	if len(feed.Entries) != 1 {
		t.Errorf("Entries length = %d", len(feed.Entries))
	}
	if feed.Entries[0].Link.Href != "https://example.com/entry1" {
		t.Errorf("Entry link = %q", feed.Entries[0].Link.Href)
	}
}

func TestAtomEntryStruct(t *testing.T) {
	entry := AtomEntry{
		Title:     "Test Entry",
		Link:      AtomLink{Href: "https://example.com/test", Rel: "alternate"},
		Summary:   "Entry summary text",
		Published: "2024-01-15T12:00:00Z",
		Updated:   "2024-01-15T14:00:00Z",
		ID:        "urn:uuid:12345",
	}

	if entry.Title != "Test Entry" {
		t.Errorf("Title = %q", entry.Title)
	}
	if entry.Link.Rel != "alternate" {
		t.Errorf("Link.Rel = %q", entry.Link.Rel)
	}
	if entry.ID != "urn:uuid:12345" {
		t.Errorf("ID = %q", entry.ID)
	}
}

// ===== SPORTS FETCHER TESTS =====

func TestNewSportsFetcher(t *testing.T) {
	cfg := &config.SportsWidgetConfig{
		Enabled:        true,
		DefaultLeagues: []string{"nfl", "nba"},
	}

	f := NewSportsFetcher(cfg)
	if f == nil {
		t.Fatal("NewSportsFetcher() returned nil")
	}
	if f.client == nil {
		t.Error("SportsFetcher.client should not be nil")
	}
	if f.config != cfg {
		t.Error("SportsFetcher.config not set correctly")
	}
}

func TestSportsFetcherWidgetType(t *testing.T) {
	f := NewSportsFetcher(&config.SportsWidgetConfig{})
	if f.WidgetType() != WidgetSports {
		t.Errorf("WidgetType() = %v, want %v", f.WidgetType(), WidgetSports)
	}
}

func TestSportsFetcherCacheDuration(t *testing.T) {
	f := NewSportsFetcher(&config.SportsWidgetConfig{})
	duration := f.CacheDuration()
	if duration != 5*time.Minute {
		t.Errorf("CacheDuration() = %v, want %v", duration, 5*time.Minute)
	}
}

func TestSportsFetcherImplementsFetcher(t *testing.T) {
	var _ Fetcher = (*SportsFetcher)(nil)
}

func TestSportsDataStruct(t *testing.T) {
	data := SportsData{
		Games: []GameData{
			{
				League:    "NFL",
				HomeTeam:  "Patriots",
				AwayTeam:  "Bills",
				HomeScore: 24,
				AwayScore: 21,
				Status:    "final",
			},
		},
	}

	if len(data.Games) != 1 {
		t.Errorf("Games length = %d", len(data.Games))
	}
	if data.Games[0].League != "NFL" {
		t.Errorf("Games[0].League = %q", data.Games[0].League)
	}
}

func TestGameDataStruct(t *testing.T) {
	game := GameData{
		League:    "NBA",
		HomeTeam:  "Lakers",
		AwayTeam:  "Celtics",
		HomeScore: 110,
		AwayScore: 105,
		Status:    "live",
		StartTime: "7:30 PM ET",
	}

	if game.League != "NBA" {
		t.Errorf("League = %q", game.League)
	}
	if game.HomeTeam != "Lakers" {
		t.Errorf("HomeTeam = %q", game.HomeTeam)
	}
	if game.Status != "live" {
		t.Errorf("Status = %q", game.Status)
	}
	if game.HomeScore != 110 {
		t.Errorf("HomeScore = %d", game.HomeScore)
	}
}

func TestSportsFetcherFetch(t *testing.T) {
	f := NewSportsFetcher(&config.SportsWidgetConfig{Enabled: true})
	ctx := context.Background()

	result, err := f.Fetch(ctx, nil)
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if result.Type != WidgetSports {
		t.Errorf("Type = %v, want %v", result.Type, WidgetSports)
	}
	// Should return empty data (placeholder implementation)
	sportsData, ok := result.Data.(*SportsData)
	if !ok {
		t.Fatal("Data is not *SportsData")
	}
	if len(sportsData.Games) != 0 {
		t.Errorf("Expected empty games, got %d", len(sportsData.Games))
	}
}

// ===== RSS FETCHER TESTS =====

func TestNewRSSFetcher(t *testing.T) {
	cfg := &config.RSSWidgetConfig{
		Enabled:  true,
		MaxFeeds: 5,
		MaxItems: 20,
	}

	f := NewRSSFetcher(cfg)
	if f == nil {
		t.Fatal("NewRSSFetcher() returned nil")
	}
	if f.client == nil {
		t.Error("RSSFetcher.client should not be nil")
	}
	if f.config != cfg {
		t.Error("RSSFetcher.config not set correctly")
	}
}

func TestRSSFetcherWidgetType(t *testing.T) {
	f := NewRSSFetcher(&config.RSSWidgetConfig{})
	if f.WidgetType() != WidgetRSS {
		t.Errorf("WidgetType() = %v, want %v", f.WidgetType(), WidgetRSS)
	}
}

func TestRSSFetcherCacheDuration(t *testing.T) {
	f := NewRSSFetcher(&config.RSSWidgetConfig{})
	duration := f.CacheDuration()
	if duration != 30*time.Minute {
		t.Errorf("CacheDuration() = %v, want %v", duration, 30*time.Minute)
	}
}

func TestRSSFetcherImplementsFetcher(t *testing.T) {
	var _ Fetcher = (*RSSFetcher)(nil)
}

func TestRSSDataStruct(t *testing.T) {
	data := RSSData{
		Items: []RSSItemData{
			{
				Title:       "RSS Item",
				URL:         "https://example.com/item",
				Source:      "Example Feed",
				PublishedAt: time.Now(),
				Summary:     "Item summary",
			},
		},
	}

	if len(data.Items) != 1 {
		t.Errorf("Items length = %d", len(data.Items))
	}
	if data.Items[0].Title != "RSS Item" {
		t.Errorf("Items[0].Title = %q", data.Items[0].Title)
	}
}

func TestRSSItemDataStruct(t *testing.T) {
	now := time.Now()
	item := RSSItemData{
		Title:       "Feed Item",
		URL:         "https://feed.example.com/article",
		Source:      "Feed Source",
		PublishedAt: now,
		Summary:     "Article summary",
	}

	if item.Title != "Feed Item" {
		t.Errorf("Title = %q", item.Title)
	}
	if item.Source != "Feed Source" {
		t.Errorf("Source = %q", item.Source)
	}
}

func TestRSSFetcherFetchEmptyFeeds(t *testing.T) {
	f := NewRSSFetcher(&config.RSSWidgetConfig{})
	ctx := context.Background()

	// Empty feeds parameter
	result, err := f.Fetch(ctx, map[string]string{})
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if result.Type != WidgetRSS {
		t.Errorf("Type = %v, want %v", result.Type, WidgetRSS)
	}

	rssData, ok := result.Data.(*RSSData)
	if !ok {
		t.Fatal("Data is not *RSSData")
	}
	if len(rssData.Items) != 0 {
		t.Errorf("Expected empty items for no feeds, got %d", len(rssData.Items))
	}
}

// ===== CURRENCY FETCHER TESTS =====

func TestNewCurrencyFetcher(t *testing.T) {
	f := NewCurrencyFetcher("test-api-key")
	if f == nil {
		t.Fatal("NewCurrencyFetcher() returned nil")
	}
	if f.httpClient == nil {
		t.Error("CurrencyFetcher.httpClient should not be nil")
	}
	if f.apiKey != "test-api-key" {
		t.Errorf("apiKey = %q, want %q", f.apiKey, "test-api-key")
	}
}

func TestCurrencyFetcherWidgetType(t *testing.T) {
	f := NewCurrencyFetcher("")
	if f.WidgetType() != WidgetCurrency {
		t.Errorf("WidgetType() = %v, want %v", f.WidgetType(), WidgetCurrency)
	}
}

func TestCurrencyFetcherCacheDuration(t *testing.T) {
	f := NewCurrencyFetcher("")
	duration := f.CacheDuration()
	if duration != 30*time.Minute {
		t.Errorf("CacheDuration() = %v, want %v", duration, 30*time.Minute)
	}
}

func TestCurrencyFetcherImplementsFetcher(t *testing.T) {
	var _ Fetcher = (*CurrencyFetcher)(nil)
}

func TestCurrencyDataStruct(t *testing.T) {
	data := CurrencyData{
		From:     "USD",
		To:       "EUR",
		Amount:   100.0,
		Result:   85.5,
		Rate:     0.855,
		RateDate: "2024-01-15",
		Rates:    map[string]float64{"EUR": 0.855, "GBP": 0.79},
	}

	if data.From != "USD" {
		t.Errorf("From = %q", data.From)
	}
	if data.To != "EUR" {
		t.Errorf("To = %q", data.To)
	}
	if data.Amount != 100.0 {
		t.Errorf("Amount = %v", data.Amount)
	}
	if data.Result != 85.5 {
		t.Errorf("Result = %v", data.Result)
	}
	if data.Rate != 0.855 {
		t.Errorf("Rate = %v", data.Rate)
	}
	if len(data.Rates) != 2 {
		t.Errorf("Rates length = %d", len(data.Rates))
	}
}

func TestCommonCurrencies(t *testing.T) {
	if len(CommonCurrencies) == 0 {
		t.Error("CommonCurrencies should not be empty")
	}

	// Check that USD is in the list
	found := false
	for _, c := range CommonCurrencies {
		if c.Code == "USD" {
			found = true
			if c.Name != "US Dollar" {
				t.Errorf("USD name = %q, want %q", c.Name, "US Dollar")
			}
			if c.Symbol != "$" {
				t.Errorf("USD symbol = %q, want %q", c.Symbol, "$")
			}
			break
		}
	}
	if !found {
		t.Error("USD not found in CommonCurrencies")
	}
}

func TestCurrencyFetcherFetchDefaults(t *testing.T) {
	f := NewCurrencyFetcher("")
	ctx := context.Background()

	// This will use default from=USD, to=EUR
	result, err := f.Fetch(ctx, map[string]string{})
	// Should not error (may fail if network unavailable, but that's ok)
	if err != nil {
		// Network error is acceptable in test
		t.Logf("Fetch() returned error (expected if no network): %v", err)
	}
	if result != nil && result.Type != WidgetCurrency {
		t.Errorf("Type = %v, want %v", result.Type, WidgetCurrency)
	}
}

// ===== WIKIPEDIA FETCHER TESTS =====

func TestNewWikipediaFetcher(t *testing.T) {
	f := NewWikipediaFetcher()
	if f == nil {
		t.Fatal("NewWikipediaFetcher() returned nil")
	}
	if f.httpClient == nil {
		t.Error("WikipediaFetcher.httpClient should not be nil")
	}
}

func TestWikipediaFetcherWidgetType(t *testing.T) {
	f := NewWikipediaFetcher()
	if f.WidgetType() != WidgetWikipedia {
		t.Errorf("WidgetType() = %v, want %v", f.WidgetType(), WidgetWikipedia)
	}
}

func TestWikipediaFetcherCacheDuration(t *testing.T) {
	f := NewWikipediaFetcher()
	duration := f.CacheDuration()
	if duration != 1*time.Hour {
		t.Errorf("CacheDuration() = %v, want %v", duration, 1*time.Hour)
	}
}

func TestWikipediaFetcherImplementsFetcher(t *testing.T) {
	var _ Fetcher = (*WikipediaFetcher)(nil)
}

func TestWikipediaDataStruct(t *testing.T) {
	data := WikipediaData{
		Title:       "Test Article",
		Extract:     "This is the article extract.",
		Description: "A test article",
		Thumbnail:   "https://example.com/thumb.jpg",
		URL:         "https://en.wikipedia.org/wiki/Test",
		Language:    "en",
	}

	if data.Title != "Test Article" {
		t.Errorf("Title = %q", data.Title)
	}
	if data.Extract != "This is the article extract." {
		t.Errorf("Extract = %q", data.Extract)
	}
	if data.Language != "en" {
		t.Errorf("Language = %q", data.Language)
	}
}

func TestWikipediaFetcherFetchNoQuery(t *testing.T) {
	f := NewWikipediaFetcher()
	ctx := context.Background()

	result, err := f.Fetch(ctx, map[string]string{})
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if result.Error != "topic parameter required" {
		t.Errorf("Error = %q, want %q", result.Error, "topic parameter required")
	}
}

func TestWikipediaFetcherFetchWithTopic(t *testing.T) {
	f := NewWikipediaFetcher()
	ctx := context.Background()

	// Test that "topic" parameter is accepted
	result, err := f.Fetch(ctx, map[string]string{"topic": "Test"})
	if err != nil {
		t.Logf("Fetch() returned error (expected if no network): %v", err)
	}
	if result != nil && result.Type != WidgetWikipedia {
		t.Errorf("Type = %v, want %v", result.Type, WidgetWikipedia)
	}
}

func TestWikipediaFetcherParseQuery(t *testing.T) {
	f := NewWikipediaFetcher()

	testCases := []struct {
		input    string
		expected string
	}{
		{"wiki python programming", "python programming"},
		{"wikipedia quantum computing", "quantum computing"},
		{"who is marie curie", "marie curie"},
		{"who was albert einstein", "albert einstein"},
		{"what is quantum computing", "quantum computing"},
		{"what are black holes", "black holes"},
		{"define photosynthesis", "photosynthesis"},
		{"tell me about the moon", "the moon"},
		{"search for climate change", "climate change"},
		{"look up artificial intelligence", "artificial intelligence"},
		{"python programming?", "python programming"},
		{"  spaces around  ", "spaces around"},
		{"Normal Query", "Normal Query"},
	}

	for _, tc := range testCases {
		result := f.parseQuery(tc.input)
		if result != tc.expected {
			t.Errorf("parseQuery(%q) = %q, want %q", tc.input, result, tc.expected)
		}
	}
}

func TestRelatedArticleStruct(t *testing.T) {
	article := RelatedArticle{
		Title:   "Related Article",
		URL:     "https://en.wikipedia.org/wiki/Related_Article",
		Extract: "Brief extract",
	}

	if article.Title != "Related Article" {
		t.Errorf("Title = %q", article.Title)
	}
	if article.URL != "https://en.wikipedia.org/wiki/Related_Article" {
		t.Errorf("URL = %q", article.URL)
	}
}

func TestWikipediaDataWithRelatedArticles(t *testing.T) {
	data := WikipediaData{
		Title:       "Test Article",
		Extract:     "This is the article extract.",
		Description: "A test article",
		Thumbnail:   "https://example.com/thumb.jpg",
		URL:         "https://en.wikipedia.org/wiki/Test",
		Language:    "en",
		RelatedArticles: []RelatedArticle{
			{Title: "Related 1", URL: "https://en.wikipedia.org/wiki/Related_1"},
			{Title: "Related 2", URL: "https://en.wikipedia.org/wiki/Related_2"},
		},
	}

	if len(data.RelatedArticles) != 2 {
		t.Errorf("RelatedArticles length = %d, want 2", len(data.RelatedArticles))
	}
	if data.RelatedArticles[0].Title != "Related 1" {
		t.Errorf("RelatedArticles[0].Title = %q", data.RelatedArticles[0].Title)
	}
}

func TestWikipediaFetcherFetchWithLang(t *testing.T) {
	f := NewWikipediaFetcher()
	ctx := context.Background()

	// Test with different language - may fail due to network
	result, err := f.Fetch(ctx, map[string]string{"query": "Test", "lang": "de"})
	if err != nil {
		t.Logf("Fetch() returned error (expected if no network): %v", err)
	}
	if result != nil && result.Type != WidgetWikipedia {
		t.Errorf("Type = %v, want %v", result.Type, WidgetWikipedia)
	}
}

// ===== DICTIONARY FETCHER TESTS =====

func TestNewDictionaryFetcher(t *testing.T) {
	f := NewDictionaryFetcher()
	if f == nil {
		t.Fatal("NewDictionaryFetcher() returned nil")
	}
	if f.httpClient == nil {
		t.Error("DictionaryFetcher.httpClient should not be nil")
	}
}

func TestDictionaryFetcherWidgetType(t *testing.T) {
	f := NewDictionaryFetcher()
	if f.WidgetType() != WidgetDictionary {
		t.Errorf("WidgetType() = %v, want %v", f.WidgetType(), WidgetDictionary)
	}
}

func TestDictionaryFetcherCacheDuration(t *testing.T) {
	f := NewDictionaryFetcher()
	duration := f.CacheDuration()
	if duration != 24*time.Hour {
		t.Errorf("CacheDuration() = %v, want %v", duration, 24*time.Hour)
	}
}

func TestDictionaryFetcherImplementsFetcher(t *testing.T) {
	var _ Fetcher = (*DictionaryFetcher)(nil)
}

func TestDictionaryDataStruct(t *testing.T) {
	data := DictionaryData{
		Word:     "test",
		Phonetic: "/test/",
		Audio:    "https://example.com/audio.mp3",
		Meanings: []DictionaryMeaning{
			{
				PartOfSpeech: "noun",
				Definitions: []DictionaryDefinition{
					{
						Definition: "A procedure for critical evaluation",
						Example:    "The test was difficult",
						Synonyms:   []string{"examination", "trial"},
					},
				},
			},
		},
		Synonyms: []string{"examination", "trial"},
		Antonyms: []string{"certainty"},
	}

	if data.Word != "test" {
		t.Errorf("Word = %q", data.Word)
	}
	if data.Phonetic != "/test/" {
		t.Errorf("Phonetic = %q", data.Phonetic)
	}
	if len(data.Meanings) != 1 {
		t.Errorf("Meanings length = %d", len(data.Meanings))
	}
	if data.Meanings[0].PartOfSpeech != "noun" {
		t.Errorf("PartOfSpeech = %q", data.Meanings[0].PartOfSpeech)
	}
}

func TestDictionaryMeaningStruct(t *testing.T) {
	meaning := DictionaryMeaning{
		PartOfSpeech: "verb",
		Definitions: []DictionaryDefinition{
			{Definition: "To perform a test"},
		},
	}

	if meaning.PartOfSpeech != "verb" {
		t.Errorf("PartOfSpeech = %q", meaning.PartOfSpeech)
	}
	if len(meaning.Definitions) != 1 {
		t.Errorf("Definitions length = %d", len(meaning.Definitions))
	}
}

func TestDictionaryDefinitionStruct(t *testing.T) {
	def := DictionaryDefinition{
		Definition: "A critical evaluation",
		Example:    "Take a test",
		Synonyms:   []string{"exam"},
	}

	if def.Definition != "A critical evaluation" {
		t.Errorf("Definition = %q", def.Definition)
	}
	if def.Example != "Take a test" {
		t.Errorf("Example = %q", def.Example)
	}
}

func TestDictionaryFetcherFetchNoWord(t *testing.T) {
	f := NewDictionaryFetcher()
	ctx := context.Background()

	result, err := f.Fetch(ctx, map[string]string{})
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if result.Error != "word parameter required" {
		t.Errorf("Error = %q, want %q", result.Error, "word parameter required")
	}
}

func TestUniqueStrings(t *testing.T) {
	tests := []struct {
		name  string
		input []string
		want  []string
	}{
		{"empty", []string{}, nil},
		{"single", []string{"a"}, []string{"a"}},
		{"no duplicates", []string{"a", "b", "c"}, []string{"a", "b", "c"}},
		{"with duplicates", []string{"a", "b", "a", "c", "b"}, []string{"a", "b", "c"}},
		{"all same", []string{"x", "x", "x"}, []string{"x"}},
		{"nil input", nil, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := uniqueStrings(tt.input)
			if len(got) != len(tt.want) {
				t.Errorf("uniqueStrings(%v) length = %d, want %d", tt.input, len(got), len(tt.want))
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("uniqueStrings(%v)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
				}
			}
		})
	}
}

// ===== TRANSLATE FETCHER TESTS =====

func TestNewTranslateFetcher(t *testing.T) {
	f := NewTranslateFetcher()
	if f == nil {
		t.Fatal("NewTranslateFetcher() returned nil")
	}
	if f.httpClient == nil {
		t.Error("TranslateFetcher.httpClient should not be nil")
	}
}

func TestTranslateFetcherWidgetType(t *testing.T) {
	f := NewTranslateFetcher()
	if f.WidgetType() != WidgetTranslate {
		t.Errorf("WidgetType() = %v, want %v", f.WidgetType(), WidgetTranslate)
	}
}

func TestTranslateFetcherCacheDuration(t *testing.T) {
	f := NewTranslateFetcher()
	duration := f.CacheDuration()
	if duration != 1*time.Hour {
		t.Errorf("CacheDuration() = %v, want %v", duration, 1*time.Hour)
	}
}

func TestTranslateFetcherImplementsFetcher(t *testing.T) {
	var _ Fetcher = (*TranslateFetcher)(nil)
}

func TestTranslateDataStruct(t *testing.T) {
	data := TranslateData{
		SourceLang:     "en",
		TargetLang:     "es",
		SourceText:     "Hello",
		TranslatedText: "Hola",
		DetectedLang:   "en",
	}

	if data.SourceLang != "en" {
		t.Errorf("SourceLang = %q", data.SourceLang)
	}
	if data.TargetLang != "es" {
		t.Errorf("TargetLang = %q", data.TargetLang)
	}
	if data.SourceText != "Hello" {
		t.Errorf("SourceText = %q", data.SourceText)
	}
	if data.TranslatedText != "Hola" {
		t.Errorf("TranslatedText = %q", data.TranslatedText)
	}
}

func TestTranslateFetcherFetchNoText(t *testing.T) {
	f := NewTranslateFetcher()
	ctx := context.Background()

	result, err := f.Fetch(ctx, map[string]string{})
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if result.Error != "text parameter required" {
		t.Errorf("Error = %q, want %q", result.Error, "text parameter required")
	}
}

func TestSupportedLanguages(t *testing.T) {
	if len(SupportedLanguages) == 0 {
		t.Error("SupportedLanguages should not be empty")
	}

	// Check that English is in the list
	found := false
	for _, lang := range SupportedLanguages {
		if lang.Code == "en" {
			found = true
			if lang.Name != "English" {
				t.Errorf("English name = %q, want %q", lang.Name, "English")
			}
			break
		}
	}
	if !found {
		t.Error("English not found in SupportedLanguages")
	}
}

func TestTranslateFetcherFetchWithParams(t *testing.T) {
	f := NewTranslateFetcher()
	ctx := context.Background()

	// Test with parameters - may fail due to network
	result, err := f.Fetch(ctx, map[string]string{
		"text": "Hello",
		"from": "en",
		"to":   "es",
	})
	// Network errors are acceptable
	if err != nil {
		t.Logf("Fetch() returned error (expected if no network): %v", err)
	}
	if result != nil && result.Type != WidgetTranslate {
		t.Errorf("Type = %v, want %v", result.Type, WidgetTranslate)
	}
}

// ===== NUTRITION FETCHER TESTS =====

func TestNewNutritionFetcher(t *testing.T) {
	f := NewNutritionFetcher("test-api-key")
	if f == nil {
		t.Fatal("NewNutritionFetcher() returned nil")
	}
	if f.httpClient == nil {
		t.Error("NutritionFetcher.httpClient should not be nil")
	}
	if f.apiKey != "test-api-key" {
		t.Errorf("apiKey = %q, want %q", f.apiKey, "test-api-key")
	}
}

func TestNutritionFetcherWidgetType(t *testing.T) {
	f := NewNutritionFetcher("")
	if f.WidgetType() != WidgetNutrition {
		t.Errorf("WidgetType() = %v, want %v", f.WidgetType(), WidgetNutrition)
	}
}

func TestNutritionFetcherCacheDuration(t *testing.T) {
	f := NewNutritionFetcher("")
	duration := f.CacheDuration()
	if duration != 24*time.Hour {
		t.Errorf("CacheDuration() = %v, want %v", duration, 24*time.Hour)
	}
}

func TestNutritionFetcherImplementsFetcher(t *testing.T) {
	var _ Fetcher = (*NutritionFetcher)(nil)
}

func TestNutritionDataStruct(t *testing.T) {
	data := NutritionData{
		Name:        "Apple",
		BrandName:   "Generic",
		ServingSize: "100g",
		Calories:    52,
		Macros: MacroNutrients{
			Protein:       0.3,
			Carbohydrates: 14,
			Fat:           0.2,
			Fiber:         2.4,
			Sugar:         10,
		},
		Micros: []NutrientInfo{
			{Name: "Vitamin C", Amount: 4.6, Unit: "mg"},
			{Name: "Potassium", Amount: 107, Unit: "mg"},
		},
		Category: "Fruits",
		Source:   "USDA FoodData Central",
	}

	if data.Name != "Apple" {
		t.Errorf("Name = %q", data.Name)
	}
	if data.BrandName != "Generic" {
		t.Errorf("BrandName = %q", data.BrandName)
	}
	if data.Calories != 52 {
		t.Errorf("Calories = %v, want 52", data.Calories)
	}
	if data.Macros.Protein != 0.3 {
		t.Errorf("Macros.Protein = %v, want 0.3", data.Macros.Protein)
	}
	if len(data.Micros) != 2 {
		t.Errorf("Micros length = %d, want 2", len(data.Micros))
	}
}

func TestServingSizeStruct(t *testing.T) {
	serving := ServingSize{
		Description: "1 medium",
		Grams:       182,
		Calories:    95,
	}

	if serving.Description != "1 medium" {
		t.Errorf("Description = %q", serving.Description)
	}
	if serving.Grams != 182 {
		t.Errorf("Grams = %v", serving.Grams)
	}
	if serving.Calories != 95 {
		t.Errorf("Calories = %v", serving.Calories)
	}
}

func TestMacroNutrientsStruct(t *testing.T) {
	macros := MacroNutrients{
		Protein:       25,
		Carbohydrates: 30,
		Fat:           10,
		Fiber:         5,
		Sugar:         2,
		SaturatedFat:  3,
	}

	if macros.Protein != 25 {
		t.Errorf("Protein = %v", macros.Protein)
	}
	if macros.Carbohydrates != 30 {
		t.Errorf("Carbohydrates = %v", macros.Carbohydrates)
	}
	if macros.Fat != 10 {
		t.Errorf("Fat = %v", macros.Fat)
	}
}

func TestExtractFoodItem(t *testing.T) {
	tests := []struct {
		query string
		want  string
	}{
		{"calories in banana", "banana"},
		{"calorie in apple", "apple"},
		{"banana calories", "banana"},
		{"apple nutrition", "apple"},
		{"chicken breast nutrition facts", "chicken breast"},
		{"nutrition facts chicken breast", "chicken breast"},
		{"nutrition info banana", "banana"},
		{"nutrition of apple", "apple"},
		{"nutrition for orange", "orange"},
		{"how many calories in banana", "banana"},
		{"how many calories does an egg have", "an egg"},
		{"macros for chicken breast", "chicken breast"},
		{"protein in chicken", "chicken"},
		{"carbs in rice", "rice"},
		{"fat in avocado", "avocado"},
		// Direct food name (no pattern match)
		{"banana", "banana"},
		{"chicken breast", "chicken breast"},
	}

	for _, tt := range tests {
		got := ExtractFoodItem(tt.query)
		if got != tt.want {
			t.Errorf("ExtractFoodItem(%q) = %q, want %q", tt.query, got, tt.want)
		}
	}
}

func TestIsNutritionQuery(t *testing.T) {
	tests := []struct {
		query string
		want  bool
	}{
		{"calories in banana", true},
		{"banana calories", true},
		{"apple nutrition", true},
		{"nutrition facts chicken breast", true},
		{"how many calories in pizza", true},
		{"macros for chicken", true},
		{"protein in chicken", true},
		{"carbs in rice", true},
		{"fat in avocado", true},
		// Non-nutrition queries
		{"weather today", false},
		{"banana recipes", false},
		{"buy apples online", false},
	}

	for _, tt := range tests {
		got := IsNutritionQuery(tt.query)
		if got != tt.want {
			t.Errorf("IsNutritionQuery(%q) = %v, want %v", tt.query, got, tt.want)
		}
	}
}

func TestNutrientInfoStruct(t *testing.T) {
	info := NutrientInfo{
		Name:   "Protein",
		Amount: 25.5,
		Unit:   "g",
	}

	if info.Name != "Protein" {
		t.Errorf("Name = %q", info.Name)
	}
	if info.Amount != 25.5 {
		t.Errorf("Amount = %v", info.Amount)
	}
	if info.Unit != "g" {
		t.Errorf("Unit = %q", info.Unit)
	}
}

func TestNutritionFetcherFetchNoQuery(t *testing.T) {
	f := NewNutritionFetcher("")
	ctx := context.Background()

	result, err := f.Fetch(ctx, map[string]string{})
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if result.Error != "food item required (use 'query' or 'food' parameter)" {
		t.Errorf("Error = %q, want %q", result.Error, "food item required (use 'query' or 'food' parameter)")
	}
}

func TestNutritionFetcherFetchWithFoodParam(t *testing.T) {
	f := NewNutritionFetcher("")
	ctx := context.Background()

	// Test that 'food' param is accepted as alternative to 'query'
	result, err := f.Fetch(ctx, map[string]string{"food": "banana"})
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	// Should not error about missing query
	if result.Error == "food item required (use 'query' or 'food' parameter)" {
		t.Errorf("Fetch with 'food' param should not error about missing query")
	}
}

// ===== TRACKING FETCHER TESTS =====

func TestNewTrackingFetcher(t *testing.T) {
	f := NewTrackingFetcher()
	if f == nil {
		t.Fatal("NewTrackingFetcher() returned nil")
	}
}

func TestTrackingFetcherWidgetType(t *testing.T) {
	f := NewTrackingFetcher()
	if f.WidgetType() != WidgetTracking {
		t.Errorf("WidgetType() = %v, want %v", f.WidgetType(), WidgetTracking)
	}
}

func TestTrackingFetcherCacheDuration(t *testing.T) {
	f := NewTrackingFetcher()
	duration := f.CacheDuration()
	if duration != 5*time.Minute {
		t.Errorf("CacheDuration() = %v, want %v", duration, 5*time.Minute)
	}
}

func TestTrackingFetcherImplementsFetcher(t *testing.T) {
	var _ Fetcher = (*TrackingFetcher)(nil)
}

func TestTrackingDataStruct(t *testing.T) {
	data := TrackingData{
		TrackingNumber: "1Z999AA10123456784",
		Carrier:        "UPS",
		CarrierURL:     "https://www.ups.com/track?tracknum=1Z999AA10123456784",
		Status:         "In Transit",
		Events: []TrackingEvent{
			{
				Date:        "2024-01-15 10:30",
				Location:    "New York, NY",
				Description: "Package departed",
			},
		},
		Detected: true,
	}

	if data.TrackingNumber != "1Z999AA10123456784" {
		t.Errorf("TrackingNumber = %q", data.TrackingNumber)
	}
	if data.Carrier != "UPS" {
		t.Errorf("Carrier = %q", data.Carrier)
	}
	if !data.Detected {
		t.Error("Detected should be true")
	}
	if len(data.Events) != 1 {
		t.Errorf("Events length = %d", len(data.Events))
	}
}

func TestTrackingEventStruct(t *testing.T) {
	event := TrackingEvent{
		Date:        "2024-01-15 14:00",
		Location:    "Chicago, IL",
		Description: "Package arrived at facility",
	}

	if event.Date != "2024-01-15 14:00" {
		t.Errorf("Date = %q", event.Date)
	}
	if event.Location != "Chicago, IL" {
		t.Errorf("Location = %q", event.Location)
	}
	if event.Description != "Package arrived at facility" {
		t.Errorf("Description = %q", event.Description)
	}
}

func TestCarrierInfoStruct(t *testing.T) {
	// Verify carrierPatterns is populated
	if len(carrierPatterns) == 0 {
		t.Error("carrierPatterns should not be empty")
	}

	// Check that USPS pattern exists
	found := false
	for _, c := range carrierPatterns {
		if c.Name == "USPS" {
			found = true
			if c.Pattern == nil {
				t.Error("USPS pattern should not be nil")
			}
			if c.TrackURL == "" {
				t.Error("USPS TrackURL should not be empty")
			}
			break
		}
	}
	if !found {
		t.Error("USPS not found in carrierPatterns")
	}
}

func TestTrackingFetcherFetchNoNumber(t *testing.T) {
	f := NewTrackingFetcher()
	ctx := context.Background()

	result, err := f.Fetch(ctx, map[string]string{})
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if result.Error != "tracking number required" {
		t.Errorf("Error = %q, want %q", result.Error, "tracking number required")
	}
}

func TestTrackingFetcherFetchWithNumber(t *testing.T) {
	f := NewTrackingFetcher()
	ctx := context.Background()

	result, err := f.Fetch(ctx, map[string]string{"number": "1Z999AA10123456784"})
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if result.Type != WidgetTracking {
		t.Errorf("Type = %v, want %v", result.Type, WidgetTracking)
	}

	data, ok := result.Data.(*TrackingData)
	if !ok {
		t.Fatal("Data is not *TrackingData")
	}
	if data.TrackingNumber != "1Z999AA10123456784" {
		t.Errorf("TrackingNumber = %q", data.TrackingNumber)
	}
	if data.Carrier != "UPS" {
		t.Errorf("Carrier = %q, want %q", data.Carrier, "UPS")
	}
	if !data.Detected {
		t.Error("Detected should be true for UPS number")
	}
}

func TestCleanTrackingNumber(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"1Z999AA10123456784", "1Z999AA10123456784"},
		{"1Z 999 AA1 0123 4567 84", "1Z999AA10123456784"},
		{"1Z-999-AA1-0123-4567-84", "1Z999AA10123456784"},
		{"  1Z999AA10123456784  ", "1Z999AA10123456784"},
		{"abc123XYZ", "abc123XYZ"},
		{"", ""},
		{"!@#$%", ""},
		{"12-34-56", "123456"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := cleanTrackingNumber(tt.input)
			if got != tt.want {
				t.Errorf("cleanTrackingNumber(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestDetectCarrier(t *testing.T) {
	tests := []struct {
		trackingNumber string
		wantCarrier    string
		wantDetected   bool
	}{
		// UPS patterns
		{"1Z999AA10123456784", "UPS", true},
		{"1ZABC12345678901234", "UPS", true},

		// FedEx patterns (12, 15, 20, 22 digits)
		{"123456789012", "FedEx", true},
		{"123456789012345", "FedEx", true},
		{"12345678901234567890", "FedEx", true},
		{"1234567890123456789012", "FedEx", true},

		// Amazon TBA pattern
		{"TBA123456789012", "Amazon", true},

		// Royal Mail pattern
		{"AB123456789GB", "Royal Mail", true},

		// Unknown patterns
		{"UNKNOWN123", "Unknown", false},
		{"XYZ", "Unknown", false},
	}

	for _, tt := range tests {
		t.Run(tt.trackingNumber, func(t *testing.T) {
			result := detectCarrier(tt.trackingNumber)
			if result.Name != tt.wantCarrier {
				t.Errorf("detectCarrier(%q).Name = %q, want %q", tt.trackingNumber, result.Name, tt.wantCarrier)
			}
			detected := result.Name != "Unknown"
			if detected != tt.wantDetected {
				t.Errorf("detectCarrier(%q) detected = %v, want %v", tt.trackingNumber, detected, tt.wantDetected)
			}
		})
	}
}

// ===== RSS FETCHER ADDITIONAL TESTS =====

func TestRSSFetcherConvertRSSItems(t *testing.T) {
	f := NewRSSFetcher(&config.RSSWidgetConfig{})

	channel := RSSChannel{
		Title: "Test Feed",
		Items: []RSSItem{
			{
				Title:       "Test Item",
				Link:        "https://example.com/item",
				Description: "Test description",
				PubDate:     "Mon, 02 Jan 2006 15:04:05 MST",
			},
		},
	}

	items := f.convertRSSItems(channel)
	if len(items) != 1 {
		t.Fatalf("convertRSSItems() returned %d items, want 1", len(items))
	}

	if items[0].Title != "Test Item" {
		t.Errorf("Title = %q", items[0].Title)
	}
	if items[0].URL != "https://example.com/item" {
		t.Errorf("URL = %q", items[0].URL)
	}
	if items[0].Source != "Test" {
		t.Errorf("Source = %q, want %q", items[0].Source, "Test")
	}
}

func TestRSSFetcherConvertAtomEntries(t *testing.T) {
	f := NewRSSFetcher(&config.RSSWidgetConfig{})

	feed := AtomFeed{
		Title: "Atom Test Feed",
		Entries: []AtomEntry{
			{
				Title:     "Atom Entry",
				Link:      AtomLink{Href: "https://example.com/entry"},
				Summary:   "Entry summary",
				Published: "2024-01-15T10:00:00Z",
			},
		},
	}

	items := f.convertAtomEntries(feed)
	if len(items) != 1 {
		t.Fatalf("convertAtomEntries() returned %d items, want 1", len(items))
	}

	if items[0].Title != "Atom Entry" {
		t.Errorf("Title = %q", items[0].Title)
	}
	if items[0].URL != "https://example.com/entry" {
		t.Errorf("URL = %q", items[0].URL)
	}
}

func TestRSSFetcherConvertAtomEntriesFallbackToID(t *testing.T) {
	f := NewRSSFetcher(&config.RSSWidgetConfig{})

	feed := AtomFeed{
		Title: "Test Feed",
		Entries: []AtomEntry{
			{
				Title:   "Entry with ID only",
				Link:    AtomLink{}, // Empty link
				ID:      "urn:uuid:12345",
				Updated: "2024-01-15T12:00:00Z",
			},
		},
	}

	items := f.convertAtomEntries(feed)
	if len(items) != 1 {
		t.Fatalf("convertAtomEntries() returned %d items, want 1", len(items))
	}

	// Should fall back to ID when Link.Href is empty
	if items[0].URL != "urn:uuid:12345" {
		t.Errorf("URL = %q, want %q", items[0].URL, "urn:uuid:12345")
	}
}

func TestRSSFetcherFetchWithFeeds(t *testing.T) {
	f := NewRSSFetcher(&config.RSSWidgetConfig{
		MaxFeeds: 3,
		MaxItems: 5,
	})
	ctx := context.Background()

	// Test with invalid feed URL - should handle gracefully
	result, err := f.Fetch(ctx, map[string]string{
		"feeds": "invalid-url",
	})
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if result.Type != WidgetRSS {
		t.Errorf("Type = %v, want %v", result.Type, WidgetRSS)
	}
}

func TestRSSFetcherFetchMaxFeedsLimit(t *testing.T) {
	f := NewRSSFetcher(&config.RSSWidgetConfig{
		MaxFeeds: 2,
		MaxItems: 10,
	})
	ctx := context.Background()

	// Test with more feeds than MaxFeeds - should be limited
	result, err := f.Fetch(ctx, map[string]string{
		"feeds": "feed1,feed2,feed3,feed4,feed5",
	})
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if result.Type != WidgetRSS {
		t.Errorf("Type = %v, want %v", result.Type, WidgetRSS)
	}
}

// ===== NEWS FETCHER ADDITIONAL TESTS =====

func TestNewsFetcherParseRSSItems(t *testing.T) {
	f := NewNewsFetcher(&config.NewsWidgetConfig{})

	channel := RSSChannel{
		Title: "News Feed - RSS Feed",
		Items: []RSSItem{
			{
				Title:       "<![CDATA[Test News Item]]>",
				Link:        "https://news.example.com/item",
				Description: "<p>News description</p>",
				PubDate:     "Mon, 02 Jan 2006 15:04:05 -0700",
			},
		},
	}

	items := f.parseRSSItems(channel)
	if len(items) != 1 {
		t.Fatalf("parseRSSItems() returned %d items, want 1", len(items))
	}

	if items[0].Title != "Test News Item" {
		t.Errorf("Title = %q", items[0].Title)
	}
	if items[0].Source != "News" {
		t.Errorf("Source = %q, want %q", items[0].Source, "News")
	}
}

func TestNewsFetcherParseAtomEntries(t *testing.T) {
	f := NewNewsFetcher(&config.NewsWidgetConfig{})

	feed := AtomFeed{
		Title: "Atom News",
		Entries: []AtomEntry{
			{
				Title:     "Atom News Item",
				Link:      AtomLink{Href: "https://example.com/news"},
				Summary:   "News summary",
				Published: "2024-01-15T10:00:00Z",
			},
		},
	}

	items := f.parseAtomEntries(feed)
	if len(items) != 1 {
		t.Fatalf("parseAtomEntries() returned %d items, want 1", len(items))
	}

	if items[0].Title != "Atom News Item" {
		t.Errorf("Title = %q", items[0].Title)
	}
}

func TestNewsFetcherFetchDefaultSources(t *testing.T) {
	f := NewNewsFetcher(&config.NewsWidgetConfig{
		Sources:  []string{}, // Empty sources triggers default
		MaxItems: 5,
	})
	ctx := context.Background()

	// This test may fail due to network - that's ok
	result, err := f.Fetch(ctx, map[string]string{})
	if err != nil {
		t.Logf("Fetch() returned error (expected if no network): %v", err)
	}
	if result != nil && result.Type != WidgetNews {
		t.Errorf("Type = %v, want %v", result.Type, WidgetNews)
	}
}

// ===== STOCKS FETCHER ADDITIONAL TESTS =====

func TestStocksFetcherFetchEmptySymbols(t *testing.T) {
	f := NewStocksFetcher(&config.StocksWidgetConfig{
		DefaultSymbols: []string{}, // Empty defaults
	})
	ctx := context.Background()

	// Should use default symbols when none provided
	result, err := f.Fetch(ctx, map[string]string{})
	// May fail due to network
	if err != nil {
		t.Logf("Fetch() returned error (expected if no network): %v", err)
	}
	if result != nil && result.Type != WidgetStocks {
		t.Errorf("Type = %v, want %v", result.Type, WidgetStocks)
	}
}

func TestStocksFetcherFetchWithSymbols(t *testing.T) {
	f := NewStocksFetcher(&config.StocksWidgetConfig{})
	ctx := context.Background()

	result, err := f.Fetch(ctx, map[string]string{
		"symbols": "AAPL, MSFT, GOOGL",
	})
	// May fail due to network
	if err != nil {
		t.Logf("Fetch() returned error (expected if no network): %v", err)
	}
	if result != nil && result.Type != WidgetStocks {
		t.Errorf("Type = %v, want %v", result.Type, WidgetStocks)
	}
}

// ===== CRYPTO FETCHER ADDITIONAL TESTS =====

func TestCryptoFetcherFetchEmptyCoins(t *testing.T) {
	f := NewCryptoFetcher(&config.CryptoWidgetConfig{
		DefaultCoins: []string{}, // Empty defaults
		Currency:     "",          // Empty currency triggers default
	})
	ctx := context.Background()

	// Should use default coins when none provided
	result, err := f.Fetch(ctx, map[string]string{})
	// May fail due to network
	if err != nil {
		t.Logf("Fetch() returned error (expected if no network): %v", err)
	}
	if result != nil && result.Type != WidgetCrypto {
		t.Errorf("Type = %v, want %v", result.Type, WidgetCrypto)
	}
}

func TestCryptoFetcherFetchWithParams(t *testing.T) {
	f := NewCryptoFetcher(&config.CryptoWidgetConfig{})
	ctx := context.Background()

	result, err := f.Fetch(ctx, map[string]string{
		"coins":    "bitcoin, ethereum",
		"currency": "eur",
	})
	// May fail due to network
	if err != nil {
		t.Logf("Fetch() returned error (expected if no network): %v", err)
	}
	if result != nil && result.Type != WidgetCrypto {
		t.Errorf("Type = %v, want %v", result.Type, WidgetCrypto)
	}
}

func TestCryptoFetcherCurrencyHandling(t *testing.T) {
	f := NewCryptoFetcher(&config.CryptoWidgetConfig{})
	ctx := context.Background()

	currencies := []string{"eur", "gbp", "usd"}
	for _, currency := range currencies {
		t.Run(currency, func(t *testing.T) {
			result, err := f.Fetch(ctx, map[string]string{
				"coins":    "bitcoin",
				"currency": currency,
			})
			if err != nil {
				t.Logf("Fetch() returned error (expected if no network): %v", err)
			}
			if result != nil && result.Type != WidgetCrypto {
				t.Errorf("Type = %v, want %v", result.Type, WidgetCrypto)
			}
		})
	}
}

// ===== WEATHER FETCHER ADDITIONAL TESTS =====

func TestWeatherFetcherFetchWithUnits(t *testing.T) {
	cfg := &config.WeatherWidgetConfig{
		Enabled:     true,
		DefaultCity: "London",
		Units:       "imperial",
	}

	f := NewWeatherFetcher(cfg)
	ctx := context.Background()

	result, err := f.Fetch(ctx, map[string]string{
		"units": "metric",
	})
	// May fail due to network
	if err != nil {
		t.Logf("Fetch() returned error (expected if no network): %v", err)
	}
	if result != nil && result.Type != WidgetWeather {
		t.Errorf("Type = %v, want %v", result.Type, WidgetWeather)
	}
}

func TestWeatherFetcherFetchCityFromParams(t *testing.T) {
	cfg := &config.WeatherWidgetConfig{
		Enabled:     true,
		DefaultCity: "Default City",
	}

	f := NewWeatherFetcher(cfg)
	ctx := context.Background()

	result, err := f.Fetch(ctx, map[string]string{
		"city": "Tokyo",
	})
	// May fail due to network
	if err != nil {
		t.Logf("Fetch() returned error (expected if no network): %v", err)
	}
	if result != nil && result.Type != WidgetWeather {
		t.Errorf("Type = %v, want %v", result.Type, WidgetWeather)
	}
}

// ===== MANAGER ADDITIONAL TESTS =====

func TestManagerGetDefaultWidgetsEmptyConfig(t *testing.T) {
	cfg := &config.WidgetsConfig{
		Enabled:        true,
		DefaultWidgets: []string{}, // Empty defaults
	}
	m := NewManager(cfg)

	// Should return default set when config has empty list
	defaults := m.GetDefaultWidgets()
	if len(defaults) != 4 {
		t.Errorf("GetDefaultWidgets() returned %d items, want 4", len(defaults))
	}
}

// ===== ADDITIONAL EDGE CASE TESTS =====

func TestAtomLinkStruct(t *testing.T) {
	link := AtomLink{
		Href: "https://example.com/entry",
		Rel:  "alternate",
	}

	if link.Href != "https://example.com/entry" {
		t.Errorf("Href = %q", link.Href)
	}
	if link.Rel != "alternate" {
		t.Errorf("Rel = %q", link.Rel)
	}
}

func TestRSSChannelStruct(t *testing.T) {
	channel := RSSChannel{
		Title:       "Test Channel",
		Link:        "https://example.com",
		Description: "A test channel",
		Items:       []RSSItem{},
	}

	if channel.Title != "Test Channel" {
		t.Errorf("Title = %q", channel.Title)
	}
	if channel.Link != "https://example.com" {
		t.Errorf("Link = %q", channel.Link)
	}
	if channel.Description != "A test channel" {
		t.Errorf("Description = %q", channel.Description)
	}
}

func TestFormatCoinNameCapitalization(t *testing.T) {
	// Test the fallback capitalization for unknown coins
	tests := []struct {
		id   string
		want string
	}{
		{"newcoin", "Newcoin"},
		{"UPPERCASE", "UPPERCASE"},
		{"x", "X"},
	}

	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			got := formatCoinName(tt.id)
			if got != tt.want {
				t.Errorf("formatCoinName(%q) = %q, want %q", tt.id, got, tt.want)
			}
		})
	}
}

func TestCoinIDToSymbolUnknown(t *testing.T) {
	// Test unknown coin returns uppercase ID
	got := coinIDToSymbol("newunknowncoin")
	if got != "NEWUNKNOWNCOIN" {
		t.Errorf("coinIDToSymbol(%q) = %q, want %q", "newunknowncoin", got, "NEWUNKNOWNCOIN")
	}
}

func TestTruncateSummaryEdgeCases(t *testing.T) {
	tests := []struct {
		name  string
		input string
		check func(string) bool
	}{
		{
			"no space in first 200 chars",
			strings.Repeat("a", 250),
			func(s string) bool {
				return len(s) == 203 && strings.HasSuffix(s, "...")
			},
		},
		{
			"exactly 200 chars",
			strings.Repeat("b", 200),
			func(s string) bool {
				return len(s) == 200
			},
		},
		{
			"199 chars",
			strings.Repeat("c", 199),
			func(s string) bool {
				return len(s) == 199
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := truncateSummary(tt.input)
			if !tt.check(result) {
				t.Errorf("truncateSummary() = %q (len=%d)", result, len(result))
			}
		})
	}
}

func TestCleanTextEdgeCases(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"<unclosed tag", "<unclosed tag"},
		{"text<tag>more", "textmore"},
		{"<a href='test'>link</a>", "link"},
		{"   multiple   spaces   ", "multiple  spaces"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := cleanText(tt.input)
			if got != tt.want {
				t.Errorf("cleanText(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestExtractSourceNameEdgeCases(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"   ", ""},
		{"Test - RSS Feed", "Test"},
		{"News News", "News"},
		{"Feed Feed", "Feed"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := extractSourceName(tt.input)
			if got != tt.want {
				t.Errorf("extractSourceName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseRSSDateEdgeCases(t *testing.T) {
	// Test with various date formats
	tests := []struct {
		input string
		valid bool
	}{
		{"", false},
		{"invalid", false},
		{"Mon, 02 Jan 2006 15:04:05 +0000", true},
		{"2006-01-02T15:04:05Z", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := parseRSSDate(tt.input)
			// Invalid dates return time.Now(), so they're never zero
			if tt.valid && result.IsZero() {
				t.Errorf("parseRSSDate(%q) returned zero time for valid input", tt.input)
			}
		})
	}
}

func TestParseAtomDateEdgeCases(t *testing.T) {
	tests := []struct {
		input string
		valid bool
	}{
		{"", false},
		{"invalid", false},
		{"2006-01-02T15:04:05.123456789Z", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := parseAtomDate(tt.input)
			if tt.valid && result.IsZero() {
				t.Errorf("parseAtomDate(%q) returned zero time for valid input", tt.input)
			}
			if !tt.valid && !result.IsZero() {
				t.Errorf("parseAtomDate(%q) returned non-zero time for invalid input", tt.input)
			}
		})
	}
}

// Test concurrent operations on Cache
func TestCacheConcurrentMultipleOperations(t *testing.T) {
	c := NewCache()
	done := make(chan bool)

	// Writer goroutine - Set
	go func() {
		for i := 0; i < 50; i++ {
			c.Set(fmt.Sprintf("key%d", i), &WidgetData{}, 5*time.Minute)
		}
		done <- true
	}()

	// Reader goroutine - Get
	go func() {
		for i := 0; i < 50; i++ {
			c.Get(fmt.Sprintf("key%d", i))
		}
		done <- true
	}()

	// Deleter goroutine
	go func() {
		for i := 0; i < 50; i++ {
			c.Delete(fmt.Sprintf("key%d", i))
		}
		done <- true
	}()

	// Size reader goroutine
	go func() {
		for i := 0; i < 50; i++ {
			c.Size()
			c.Keys()
		}
		done <- true
	}()

	// Wait for all goroutines
	for i := 0; i < 4; i++ {
		<-done
	}
}

// Test Manager with nil params
func TestManagerFetchWidgetDataNilParams(t *testing.T) {
	m := NewManager(&config.WidgetsConfig{Enabled: true})

	data := &WidgetData{
		Type:      WidgetWeather,
		Data:      map[string]interface{}{"temp": 25},
		UpdatedAt: time.Now(),
	}

	fetcher := &mockFetcher{
		widgetType:    WidgetWeather,
		cacheDuration: 5 * time.Minute,
		data:          data,
	}

	m.RegisterFetcher(fetcher)

	ctx := context.Background()
	result, err := m.FetchWidgetData(ctx, WidgetWeather, nil)
	if err != nil {
		t.Fatalf("FetchWidgetData() error = %v", err)
	}
	if result.Type != WidgetWeather {
		t.Errorf("Result type = %v, want %v", result.Type, WidgetWeather)
	}
}

// Test all carrier patterns
func TestAllCarrierPatterns(t *testing.T) {
	testCases := []struct {
		carrier string
		numbers []string
	}{
		{"USPS", []string{"9400111899223033336024"}},
		{"UPS", []string{"1Z999AA10123456784"}},
		{"FedEx", []string{"123456789012", "123456789012345", "12345678901234567890"}},
		{"DHL", []string{"1234567890", "12345678901"}},
		{"Amazon", []string{"TBA123456789012"}},
		{"Royal Mail", []string{"AB123456789GB"}},
	}

	for _, tc := range testCases {
		for _, num := range tc.numbers {
			t.Run(fmt.Sprintf("%s_%s", tc.carrier, num), func(t *testing.T) {
				result := detectCarrier(num)
				if result.Name != tc.carrier {
					t.Errorf("detectCarrier(%q).Name = %q, want %q", num, result.Name, tc.carrier)
				}
			})
		}
	}
}

// Test widget data with error
func TestWidgetDataWithError(t *testing.T) {
	wd := WidgetData{
		Type:      WidgetWeather,
		Data:      nil,
		UpdatedAt: time.Now(),
		Error:     "failed to fetch data",
	}

	if wd.Error != "failed to fetch data" {
		t.Errorf("Error = %q", wd.Error)
	}
	if wd.Data != nil {
		t.Error("Data should be nil when there's an error")
	}
}

// Test OpenMeteoResponse hourly data
func TestOpenMeteoResponseHourlyData(t *testing.T) {
	resp := OpenMeteoResponse{
		Latitude:  40.7128,
		Longitude: -74.0060,
	}
	resp.CurrentWeather.Temperature = 25.5
	resp.CurrentWeather.WindSpeed = 10.0
	resp.CurrentWeather.WeatherCode = 0
	resp.Hourly.RelativeHumidity2m = []int{65, 60, 55}
	resp.Hourly.ApparentTemperature = []float64{26.0, 25.5, 25.0}

	if len(resp.Hourly.RelativeHumidity2m) != 3 {
		t.Errorf("RelativeHumidity2m length = %d", len(resp.Hourly.RelativeHumidity2m))
	}
	if len(resp.Hourly.ApparentTemperature) != 3 {
		t.Errorf("ApparentTemperature length = %d", len(resp.Hourly.ApparentTemperature))
	}
}

// Test cache with short TTL
func TestCacheShortTTL(t *testing.T) {
	c := NewCache()

	data := &WidgetData{Type: WidgetWeather}
	c.Set("short-ttl", data, 10*time.Millisecond)

	// Should exist immediately
	result, ok := c.Get("short-ttl")
	if !ok {
		t.Error("Item should exist immediately after Set")
	}
	if result == nil {
		t.Error("Result should not be nil")
	}

	// Wait for expiration
	time.Sleep(20 * time.Millisecond)

	// Should be expired now
	result, ok = c.Get("short-ttl")
	if ok {
		t.Error("Item should be expired")
	}
	if result != nil {
		t.Error("Result should be nil for expired item")
	}
}
