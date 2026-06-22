package widget

import (
	"context"
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
}
