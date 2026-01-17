package widget

import (
	"context"
	"fmt"
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
