package widgets

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/apimgr/search/src/config"
)

// WeatherFetcher fetches weather data from Open-Meteo API
type WeatherFetcher struct {
	client *http.Client
	config *config.WeatherWidgetConfig
}

// WeatherData represents weather widget data
type WeatherData struct {
	Location    string  `json:"location"`
	Temperature float64 `json:"temperature"`
	FeelsLike   float64 `json:"feels_like"`
	Humidity    int     `json:"humidity"`
	Description string  `json:"description"`
	Condition   string  `json:"condition"`
	WindSpeed   float64 `json:"wind_speed"`
	Icon        string  `json:"icon"`
}

// GeocodingResponse represents Open-Meteo geocoding API response
type GeocodingResponse struct {
	Results []struct {
		Name      string  `json:"name"`
		Latitude  float64 `json:"latitude"`
		Longitude float64 `json:"longitude"`
		Country   string  `json:"country"`
		Admin1    string  `json:"admin1"` // State/Region
	} `json:"results"`
}

// OpenMeteoResponse represents Open-Meteo weather API response
type OpenMeteoResponse struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	CurrentWeather struct {
		Temperature   float64 `json:"temperature"`
		WindSpeed     float64 `json:"windspeed"`
		WindDirection int     `json:"winddirection"`
		WeatherCode   int     `json:"weathercode"`
		IsDay         int     `json:"is_day"`
		Time          string  `json:"time"`
	} `json:"current_weather"`
	Hourly struct {
		RelativeHumidity2m []int     `json:"relativehumidity_2m"`
		ApparentTemperature []float64 `json:"apparent_temperature"`
	} `json:"hourly"`
}

// NewWeatherFetcher creates a new weather fetcher
func NewWeatherFetcher(cfg *config.WeatherWidgetConfig) *WeatherFetcher {
	return &WeatherFetcher{
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		config: cfg,
	}
}

// WidgetType returns the widget type
func (f *WeatherFetcher) WidgetType() WidgetType {
	return WidgetWeather
}

// CacheDuration returns how long to cache the data
func (f *WeatherFetcher) CacheDuration() time.Duration {
	return 15 * time.Minute
}

// Fetch fetches weather data
func (f *WeatherFetcher) Fetch(ctx context.Context, params map[string]string) (*WidgetData, error) {
	city := params["city"]
	if city == "" {
		city = f.config.DefaultCity
	}
	if city == "" {
		return &WidgetData{
			Type:      WidgetWeather,
			Error:     "no city specified",
			UpdatedAt: time.Now(),
		}, nil
	}

	units := params["units"]
	if units == "" {
		units = f.config.Units
	}
	if units == "" {
		units = "metric"
	}

	// First, geocode the city to get coordinates
	lat, lon, locationName, err := f.geocodeCity(ctx, city)
	if err != nil {
		return &WidgetData{
			Type:      WidgetWeather,
			Error:     fmt.Sprintf("failed to find city: %v", err),
			UpdatedAt: time.Now(),
		}, nil
	}

	// Fetch weather data
	weather, err := f.fetchWeather(ctx, lat, lon, units)
	if err != nil {
		return &WidgetData{
			Type:      WidgetWeather,
			Error:     fmt.Sprintf("failed to fetch weather: %v", err),
			UpdatedAt: time.Now(),
		}, nil
	}

	weather.Location = locationName

	return &WidgetData{
		Type:      WidgetWeather,
		Data:      weather,
		UpdatedAt: time.Now(),
	}, nil
}

// geocodeCity converts a city name to coordinates
func (f *WeatherFetcher) geocodeCity(ctx context.Context, city string) (float64, float64, string, error) {
	apiURL := fmt.Sprintf("https://geocoding-api.open-meteo.com/v1/search?name=%s&count=1&language=en&format=json",
		url.QueryEscape(city))

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return 0, 0, "", err
	}

	resp, err := f.client.Do(req)
	if err != nil {
		return 0, 0, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, 0, "", fmt.Errorf("geocoding API returned status %d", resp.StatusCode)
	}

	var geoResp GeocodingResponse
	if err := json.NewDecoder(resp.Body).Decode(&geoResp); err != nil {
		return 0, 0, "", err
	}

	if len(geoResp.Results) == 0 {
		return 0, 0, "", fmt.Errorf("city not found: %s", city)
	}

	result := geoResp.Results[0]
	locationName := result.Name
	if result.Admin1 != "" {
		locationName += ", " + result.Admin1
	}
	if result.Country != "" {
		locationName += ", " + result.Country
	}

	return result.Latitude, result.Longitude, locationName, nil
}

// fetchWeather fetches weather data from Open-Meteo
func (f *WeatherFetcher) fetchWeather(ctx context.Context, lat, lon float64, units string) (*WeatherData, error) {
	tempUnit := "celsius"
	windUnit := "kmh"
	if units == "imperial" {
		tempUnit = "fahrenheit"
		windUnit = "mph"
	}

	apiURL := fmt.Sprintf("https://api.open-meteo.com/v1/forecast?latitude=%f&longitude=%f&current_weather=true&hourly=relativehumidity_2m,apparent_temperature&temperature_unit=%s&windspeed_unit=%s&timezone=auto&forecast_days=1",
		lat, lon, tempUnit, windUnit)

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("weather API returned status %d", resp.StatusCode)
	}

	var weatherResp OpenMeteoResponse
	if err := json.NewDecoder(resp.Body).Decode(&weatherResp); err != nil {
		return nil, err
	}

	// Map weather code to description and condition
	description, condition := weatherCodeToDescription(weatherResp.CurrentWeather.WeatherCode)

	// Get current hour's humidity and apparent temperature
	humidity := 0
	feelsLike := weatherResp.CurrentWeather.Temperature
	if len(weatherResp.Hourly.RelativeHumidity2m) > 0 {
		humidity = weatherResp.Hourly.RelativeHumidity2m[0]
	}
	if len(weatherResp.Hourly.ApparentTemperature) > 0 {
		feelsLike = weatherResp.Hourly.ApparentTemperature[0]
	}

	return &WeatherData{
		Temperature: weatherResp.CurrentWeather.Temperature,
		FeelsLike:   feelsLike,
		Humidity:    humidity,
		Description: description,
		Condition:   condition,
		WindSpeed:   weatherResp.CurrentWeather.WindSpeed,
	}, nil
}

// weatherCodeToDescription maps WMO weather codes to descriptions
func weatherCodeToDescription(code int) (string, string) {
	switch code {
	case 0:
		return "Clear sky", "clear"
	case 1:
		return "Mainly clear", "clear"
	case 2:
		return "Partly cloudy", "partly-cloudy"
	case 3:
		return "Overcast", "cloudy"
	case 45, 48:
		return "Foggy", "fog"
	case 51, 53, 55:
		return "Drizzle", "rain"
	case 56, 57:
		return "Freezing drizzle", "rain"
	case 61, 63, 65:
		return "Rain", "rain"
	case 66, 67:
		return "Freezing rain", "rain"
	case 71, 73, 75:
		return "Snow", "snow"
	case 77:
		return "Snow grains", "snow"
	case 80, 81, 82:
		return "Rain showers", "rain"
	case 85, 86:
		return "Snow showers", "snow"
	case 95:
		return "Thunderstorm", "thunderstorm"
	case 96, 99:
		return "Thunderstorm with hail", "thunderstorm"
	default:
		return "Unknown", "cloudy"
	}
}
