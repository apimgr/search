package widget

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/apimgr/search/src/config"
)

func TestExpandLocationAbbreviations(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"Boston MA expands state", "Boston, MA", "Boston, Massachusetts"},
		{"New York NY expands state", "New York, NY", "New York, New York"},
		{"London UK expands country", "London, UK", "London, United Kingdom"},
		{"Paris FR expands country", "Paris, FR", "Paris, France"},
		{"Los Angeles CA US expands both", "Los Angeles, CA, US", "Los Angeles, California, United States"},
		{"London with no abbreviation unchanged", "London", "London"},
		{"empty string unchanged", "", ""},
		{"city with unknown abbrev unchanged", "Springfield, XZ", "Springfield, XZ"},
		{"California abbreviation CA", "San Francisco, CA", "San Francisco, California"},
		{"Germany abbreviation DE", "Berlin, DE", "Berlin, Delaware"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := expandLocationAbbreviations(tt.input)
			if got != tt.want {
				t.Errorf("expandLocationAbbreviations(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestWeatherCodeToDescription(t *testing.T) {
	tests := []struct {
		name          string
		code          int
		wantDesc      string
		wantCondition string
	}{
		{"code 0 clear sky", 0, "Clear sky", "clear"},
		{"code 1 mainly clear", 1, "Mainly clear", "clear"},
		{"code 2 partly cloudy", 2, "Partly cloudy", "partly-cloudy"},
		{"code 3 overcast", 3, "Overcast", "cloudy"},
		{"code 45 foggy", 45, "Foggy", "fog"},
		{"code 61 rain", 61, "Rain", "rain"},
		{"code 71 snow", 71, "Snow", "snow"},
		{"code 95 thunderstorm", 95, "Thunderstorm", "thunderstorm"},
		{"unknown code 999 returns Unknown and cloudy", 999, "Unknown", "cloudy"},
		{"code 48 foggy variant", 48, "Foggy", "fog"},
		{"code 80 rain showers", 80, "Rain showers", "rain"},
		{"code 85 snow showers", 85, "Snow showers", "snow"},
		{"code 96 thunderstorm with hail", 96, "Thunderstorm with hail", "thunderstorm"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotDesc, gotCondition := weatherCodeToDescription(tt.code)
			if gotDesc != tt.wantDesc {
				t.Errorf("weatherCodeToDescription(%d) description = %q, want %q", tt.code, gotDesc, tt.wantDesc)
			}
			if gotCondition != tt.wantCondition {
				t.Errorf("weatherCodeToDescription(%d) condition = %q, want %q", tt.code, gotCondition, tt.wantCondition)
			}
		})
	}
}

func TestWeatherFetcherCacheDuration(t *testing.T) {
	cfg := &config.WeatherWidgetConfig{
		DefaultCity: "Boston",
		Units:       "metric",
	}
	f := NewWeatherFetcher(cfg)
	got := f.CacheDuration()
	if got != 15*time.Minute {
		t.Errorf("CacheDuration() = %v, want %v", got, 15*time.Minute)
	}
}

func TestWeatherFetcherWidgetType(t *testing.T) {
	cfg := &config.WeatherWidgetConfig{}
	f := NewWeatherFetcher(cfg)
	got := f.WidgetType()
	if got != WidgetWeather {
		t.Errorf("WidgetType() = %q, want %q", got, WidgetWeather)
	}
}

func TestWeatherFetcherFetchValidation(t *testing.T) {
	t.Run("no city specified and no lat/lon returns error", func(t *testing.T) {
		cfg := &config.WeatherWidgetConfig{DefaultCity: "", Units: "metric"}
		f := NewWeatherFetcher(cfg)
		data, err := f.Fetch(context.Background(), map[string]string{})
		if err != nil {
			t.Fatalf("Fetch() returned unexpected error: %v", err)
		}
		if data.Error == "" {
			t.Error("WidgetData.Error should be set when no city is specified")
		}
		if data.Error != "no city specified" {
			t.Errorf("WidgetData.Error = %q, want %q", data.Error, "no city specified")
		}
		if data.Type != WidgetWeather {
			t.Errorf("WidgetData.Type = %q, want %q", data.Type, WidgetWeather)
		}
	})

	t.Run("invalid latitude returns error", func(t *testing.T) {
		cfg := &config.WeatherWidgetConfig{DefaultCity: "", Units: "metric"}
		f := NewWeatherFetcher(cfg)
		data, err := f.Fetch(context.Background(), map[string]string{
			"lat": "invalid",
			"lon": "0",
		})
		if err != nil {
			t.Fatalf("Fetch() returned unexpected error: %v", err)
		}
		if data.Error == "" {
			t.Error("WidgetData.Error should be set for invalid latitude")
		}
		if data.Error != "invalid latitude" {
			t.Errorf("WidgetData.Error = %q, want %q", data.Error, "invalid latitude")
		}
	})

	t.Run("invalid longitude returns error", func(t *testing.T) {
		cfg := &config.WeatherWidgetConfig{DefaultCity: "", Units: "metric"}
		f := NewWeatherFetcher(cfg)
		data, err := f.Fetch(context.Background(), map[string]string{
			"lat": "0",
			"lon": "notanumber",
		})
		if err != nil {
			t.Fatalf("Fetch() returned unexpected error: %v", err)
		}
		if data.Error == "" {
			t.Error("WidgetData.Error should be set for invalid longitude")
		}
		if data.Error != "invalid longitude" {
			t.Errorf("WidgetData.Error = %q, want %q", data.Error, "invalid longitude")
		}
	})

	t.Run("nil config defaults without panicking", func(t *testing.T) {
		f := NewWeatherFetcher(nil)
		data, err := f.Fetch(context.Background(), map[string]string{})
		if err != nil {
			t.Fatal(err)
		}
		if data.Error != "no city specified" {
			t.Fatalf("Fetch() error = %q, want no city specified", data.Error)
		}
	})

	for _, test := range []struct {
		name string
		lat  string
		lon  string
	}{
		{name: "trailing latitude data", lat: "10junk", lon: "20"},
		{name: "latitude out of range", lat: "90.01", lon: "20"},
		{name: "infinite latitude", lat: "+Inf", lon: "20"},
		{name: "missing longitude", lat: "10"},
		{name: "longitude out of range", lat: "10", lon: "180.01"},
		{name: "NaN longitude", lat: "10", lon: "NaN"},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := NewWeatherFetcher(&config.WeatherWidgetConfig{})
			data, err := f.Fetch(context.Background(), map[string]string{"lat": test.lat, "lon": test.lon})
			if err != nil {
				t.Fatal(err)
			}
			if data.Error == "" {
				t.Fatal("Fetch() accepted invalid coordinates")
			}
		})
	}

	t.Run("rejects unsupported units", func(t *testing.T) {
		f := NewWeatherFetcher(nil)
		data, err := f.Fetch(context.Background(), map[string]string{"units": "kelvin"})
		if err != nil {
			t.Fatal(err)
		}
		if data.Error != "invalid units" {
			t.Fatalf("Fetch() error = %q, want invalid units", data.Error)
		}
	})
}

type weatherRoundTripper func(*http.Request) (*http.Response, error)

func (roundTrip weatherRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return roundTrip(request)
}

func TestFetchWeatherIncludesFiveDayForecastAndCurrentHour(t *testing.T) {
	body := `{"current_weather":{"temperature":18,"windspeed":7,"winddirection":90,"weathercode":2,"is_day":1,"time":"2026-10-11T13:00"},"hourly":{"time":["2026-10-11T12:00","2026-10-11T13:00"],"relativehumidity_2m":[30,61],"apparent_temperature":[17,19]},"daily":{"time":["2026-10-11","2026-10-12","2026-10-13","2026-10-14","2026-10-15"],"temperature_2m_max":[20,21,22,23,24],"temperature_2m_min":[10,11,12,13,14],"weather_code":[0,1,2,3,61]}}`
	fetcher := NewWeatherFetcher(&config.WeatherWidgetConfig{})
	fetcher.client = &http.Client{Transport: weatherRoundTripper(func(request *http.Request) (*http.Response, error) {
		query := request.URL.Query()
		if query.Get("forecast_days") != "5" || query.Get("daily") != "weather_code,temperature_2m_max,temperature_2m_min" {
			t.Errorf("weather query does not request five-day conditions: %s", request.URL.RawQuery)
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: request}, nil
	})}
	data, err := fetcher.fetchWeather(context.Background(), 40, -70, "metric")
	if err != nil {
		t.Fatal(err)
	}
	if data.Humidity != 61 || data.FeelsLike != 19 {
		t.Errorf("current-hour values = humidity %d, feels-like %v", data.Humidity, data.FeelsLike)
	}
	if len(data.Forecast) != 5 {
		t.Fatalf("forecast has %d days, want 5", len(data.Forecast))
	}
	if data.Forecast[4].Description != "Rain" || data.Forecast[4].High != 24 || data.Forecast[4].Low != 14 {
		t.Errorf("last forecast day = %#v", data.Forecast[4])
	}
}

func TestCurrentWeatherHourIndexFallsBackWhenNoMatch(t *testing.T) {
	if got := currentWeatherHourIndex([]string{"2026-10-11T12:00", "2026-10-11T13:00"}, "2026-10-11T14:00"); got != 0 {
		t.Errorf("fallback hour index = %d, want 0", got)
	}
	if got := currentWeatherHourIndex(nil, "2026-10-11T14:00"); got != -1 {
		t.Errorf("empty hour index = %d, want -1", got)
	}
}

func TestWeatherRequestUsesMetricAndImperialUnits(t *testing.T) {
	for _, test := range []struct {
		units       string
		temperature string
		wind        string
	}{{"metric", "celsius", "kmh"}, {"imperial", "fahrenheit", "mph"}} {
		t.Run(test.units, func(t *testing.T) {
			fetcher := NewWeatherFetcher(&config.WeatherWidgetConfig{})
			fetcher.client = &http.Client{Transport: weatherRoundTripper(func(request *http.Request) (*http.Response, error) {
				query, _ := url.ParseQuery(request.URL.RawQuery)
				if query.Get("temperature_unit") != test.temperature || query.Get("windspeed_unit") != test.wind {
					t.Errorf("unit parameters = %v", query)
				}
				return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"current_weather":{"temperature":1,"windspeed":2,"weathercode":0,"time":"2026-10-11T13:00"},"daily":{"time":["2026-10-11","2026-10-12","2026-10-13","2026-10-14","2026-10-15"],"temperature_2m_max":[1,2,3,4,5],"temperature_2m_min":[0,1,2,3,4],"weather_code":[0,1,2,3,4]}}`)), Request: request}, nil
			})}
			if _, err := fetcher.fetchWeather(context.Background(), 0, 0, test.units); err != nil {
				t.Fatal(err)
			}
		})
	}
}
