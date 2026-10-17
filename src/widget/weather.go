package widget

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/apimgr/search/src/common/closeutil"

	"github.com/apimgr/search/src/config"
)

// US state abbreviations to full names
var usStateAbbreviations = map[string]string{
	"AL": "Alabama", "AK": "Alaska", "AZ": "Arizona", "AR": "Arkansas",
	"CA": "California", "CO": "Colorado", "CT": "Connecticut", "DE": "Delaware",
	"FL": "Florida", "GA": "Georgia", "HI": "Hawaii", "ID": "Idaho",
	"IL": "Illinois", "IN": "Indiana", "IA": "Iowa", "KS": "Kansas",
	"KY": "Kentucky", "LA": "Louisiana", "ME": "Maine", "MD": "Maryland",
	"MA": "Massachusetts", "MI": "Michigan", "MN": "Minnesota", "MS": "Mississippi",
	"MO": "Missouri", "MT": "Montana", "NE": "Nebraska", "NV": "Nevada",
	"NH": "New Hampshire", "NJ": "New Jersey", "NM": "New Mexico", "NY": "New York",
	"NC": "North Carolina", "ND": "North Dakota", "OH": "Ohio", "OK": "Oklahoma",
	"OR": "Oregon", "PA": "Pennsylvania", "RI": "Rhode Island", "SC": "South Carolina",
	"SD": "South Dakota", "TN": "Tennessee", "TX": "Texas", "UT": "Utah",
	"VT": "Vermont", "VA": "Virginia", "WA": "Washington", "WV": "West Virginia",
	"WI": "Wisconsin", "WY": "Wyoming", "DC": "District of Columbia",
}

// Common country abbreviations
var countryAbbreviations = map[string]string{
	"UK": "United Kingdom", "GB": "United Kingdom", "US": "United States",
	"USA": "United States", "DE": "Germany", "FR": "France", "ES": "Spain",
	"IT": "Italy", "NL": "Netherlands", "BE": "Belgium", "CH": "Switzerland",
	"AT": "Austria", "AU": "Australia", "NZ": "New Zealand", "CA": "Canada",
	"MX": "Mexico", "BR": "Brazil", "AR": "Argentina", "JP": "Japan",
	"CN": "China", "KR": "South Korea", "IN": "India", "RU": "Russia",
}

// expandLocationAbbreviations expands common abbreviations in location strings
func expandLocationAbbreviations(location string) string {
	parts := strings.Split(location, ",")
	for i, part := range parts {
		trimmed := strings.TrimSpace(part)
		upper := strings.ToUpper(trimmed)

		// Check US state abbreviations
		if fullName, ok := usStateAbbreviations[upper]; ok {
			parts[i] = fullName
			continue
		}

		// Check country abbreviations
		if fullName, ok := countryAbbreviations[upper]; ok {
			parts[i] = fullName
			continue
		}

		// Not found: normalize whitespace to avoid double-spacing on join
		parts[i] = trimmed
	}

	return strings.Join(parts, ", ")
}

// WeatherFetcher fetches weather data from Open-Meteo API
type WeatherFetcher struct {
	client *http.Client
	config *config.WeatherWidgetConfig
}

// WeatherData represents weather widget data
type WeatherData struct {
	Location    string               `json:"location"`
	Temperature float64              `json:"temperature"`
	FeelsLike   float64              `json:"feels_like"`
	Humidity    int                  `json:"humidity"`
	Description string               `json:"description"`
	Condition   string               `json:"condition"`
	WindSpeed   float64              `json:"wind_speed"`
	Icon        string               `json:"icon"`
	Units       string               `json:"units"`
	Forecast    []WeatherForecastDay `json:"forecast,omitempty"`
}

// WeatherForecastDay contains one day's high, low, and expected conditions.
type WeatherForecastDay struct {
	Date        string  `json:"date"`
	High        float64 `json:"high"`
	Low         float64 `json:"low"`
	Description string  `json:"description"`
	Condition   string  `json:"condition"`
}

// GeocodingResponse represents Open-Meteo geocoding API response
type GeocodingResponse struct {
	Results []struct {
		Name      string  `json:"name"`
		Latitude  float64 `json:"latitude"`
		Longitude float64 `json:"longitude"`
		Country   string  `json:"country"`
		// State/Region
		Admin1 string `json:"admin1"`
	} `json:"results"`
}

// OpenMeteoResponse represents Open-Meteo weather API response
type OpenMeteoResponse struct {
	Latitude       float64 `json:"latitude"`
	Longitude      float64 `json:"longitude"`
	CurrentWeather struct {
		Temperature   float64 `json:"temperature"`
		WindSpeed     float64 `json:"windspeed"`
		WindDirection int     `json:"winddirection"`
		WeatherCode   int     `json:"weathercode"`
		IsDay         int     `json:"is_day"`
		Time          string  `json:"time"`
	} `json:"current_weather"`
	Hourly struct {
		Time                []string  `json:"time"`
		RelativeHumidity2m  []int     `json:"relativehumidity_2m"`
		ApparentTemperature []float64 `json:"apparent_temperature"`
	} `json:"hourly"`
	Daily struct {
		Time           []string  `json:"time"`
		TemperatureMax []float64 `json:"temperature_2m_max"`
		TemperatureMin []float64 `json:"temperature_2m_min"`
		WeatherCode    []int     `json:"weather_code"`
	} `json:"daily"`
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
	units := params["units"]
	if units == "" {
		units = f.config.Units
	}
	if units == "" {
		units = "imperial"
	}

	var lat, lon float64
	var locationName string
	var err error

	// Check if lat/lon coordinates are provided (from browser geolocation)
	if params["lat"] != "" && params["lon"] != "" {
		if _, err := fmt.Sscanf(params["lat"], "%f", &lat); err != nil {
			return &WidgetData{
				Type:      WidgetWeather,
				Error:     "invalid latitude",
				UpdatedAt: time.Now(),
			}, nil
		}
		if _, err := fmt.Sscanf(params["lon"], "%f", &lon); err != nil {
			return &WidgetData{
				Type:      WidgetWeather,
				Error:     "invalid longitude",
				UpdatedAt: time.Now(),
			}, nil
		}
		// Reverse geocode to get location name
		locationName, err = f.reverseGeocode(ctx, lat, lon)
		if err != nil {
			locationName = fmt.Sprintf("%.2f, %.2f", lat, lon)
		}
	} else {
		// Fall back to city name geocoding
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

		// Geocode the city to get coordinates
		lat, lon, locationName, err = f.geocodeCity(ctx, city)
		if err != nil {
			return &WidgetData{
				Type:      WidgetWeather,
				Error:     fmt.Sprintf("failed to find city: %v", err),
				UpdatedAt: time.Now(),
			}, nil
		}
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
	weather.Units = units

	return &WidgetData{
		Type:      WidgetWeather,
		Data:      weather,
		UpdatedAt: time.Now(),
	}, nil
}

// NominatimResponse represents OpenStreetMap Nominatim reverse geocoding response
type NominatimResponse struct {
	DisplayName string `json:"display_name"`
	Address     struct {
		City        string `json:"city"`
		Town        string `json:"town"`
		Village     string `json:"village"`
		County      string `json:"county"`
		State       string `json:"state"`
		Country     string `json:"country"`
		CountryCode string `json:"country_code"`
	} `json:"address"`
}

// reverseGeocode converts coordinates to a location name using OpenStreetMap Nominatim
func (f *WeatherFetcher) reverseGeocode(ctx context.Context, lat, lon float64) (string, error) {
	// Use OpenStreetMap Nominatim for reverse geocoding (free, no API key required)
	apiURL := fmt.Sprintf("https://nominatim.openstreetmap.org/reverse?lat=%f&lon=%f&format=json&zoom=10",
		lat, lon)

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return fmt.Sprintf("%.2f, %.2f", lat, lon), err
	}

	// Nominatim requires a User-Agent header
	req.Header.Set("User-Agent", "Search/1.0")

	resp, err := f.client.Do(req)
	if err != nil {
		return fmt.Sprintf("%.2f, %.2f", lat, lon), err
	}
	defer closeutil.Close(resp.Body)

	// If reverse geocoding fails, return coordinates as string
	if resp.StatusCode != http.StatusOK {
		return fmt.Sprintf("%.2f, %.2f", lat, lon), nil
	}

	var nomResp NominatimResponse
	if err := json.NewDecoder(resp.Body).Decode(&nomResp); err != nil {
		return fmt.Sprintf("%.2f, %.2f", lat, lon), nil
	}

	// Build location name from address parts
	var locationName string
	addr := nomResp.Address

	// Get city/town/village
	if addr.City != "" {
		locationName = addr.City
	} else if addr.Town != "" {
		locationName = addr.Town
	} else if addr.Village != "" {
		locationName = addr.Village
	} else if addr.County != "" {
		locationName = addr.County
	}

	// Add state/region if available
	if addr.State != "" && locationName != "" {
		locationName += ", " + addr.State
	}

	// Add country
	if addr.Country != "" {
		if locationName != "" {
			locationName += ", " + addr.Country
		} else {
			locationName = addr.Country
		}
	}

	if locationName == "" {
		return fmt.Sprintf("%.2f, %.2f", lat, lon), nil
	}

	return locationName, nil
}

// geocodeCity converts a city name to coordinates
func (f *WeatherFetcher) geocodeCity(ctx context.Context, city string) (float64, float64, string, error) {
	// Parse city input - could be "City", "City, State", "City, Country", etc.
	parts := strings.Split(city, ",")
	cityName := strings.TrimSpace(parts[0])

	// Extract and expand state/country filter if provided
	var stateFilter, countryFilter string
	if len(parts) > 1 {
		// Expand abbreviations for the second part
		expanded := expandLocationAbbreviations(strings.TrimSpace(parts[1]))
		// Could be state or country
		stateFilter = strings.ToLower(expanded)
		countryFilter = stateFilter
	}
	if len(parts) > 2 {
		// Third part is country
		expanded := expandLocationAbbreviations(strings.TrimSpace(parts[2]))
		countryFilter = strings.ToLower(expanded)
	}

	// Query API with city name only, get multiple results to filter
	apiURL := fmt.Sprintf("https://geocoding-api.open-meteo.com/v1/search?name=%s&count=10&language=en&format=json",
		url.QueryEscape(cityName))

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return 0, 0, "", err
	}

	resp, err := f.client.Do(req)
	if err != nil {
		return 0, 0, "", err
	}
	defer closeutil.Close(resp.Body)

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

	// Find the best match based on state/country filter
	// Default to first result
	var result = geoResp.Results[0]
	if stateFilter != "" || countryFilter != "" {
		for _, r := range geoResp.Results {
			admin1Lower := strings.ToLower(r.Admin1)
			countryLower := strings.ToLower(r.Country)

			// Check for state/admin1 match
			if stateFilter != "" && strings.Contains(admin1Lower, stateFilter) {
				result = r
				break
			}

			// Check for country match
			if countryFilter != "" && strings.Contains(countryLower, countryFilter) {
				result = r
				break
			}
		}
	}

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

	apiURL := fmt.Sprintf("https://api.open-meteo.com/v1/forecast?latitude=%f&longitude=%f&current_weather=true&hourly=relativehumidity_2m,apparent_temperature&daily=weather_code,temperature_2m_max,temperature_2m_min&temperature_unit=%s&windspeed_unit=%s&timezone=auto&forecast_days=5",
		lat, lon, tempUnit, windUnit)

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer closeutil.Close(resp.Body)

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
	hourIndex := currentWeatherHourIndex(weatherResp.Hourly.Time, weatherResp.CurrentWeather.Time)
	if hourIndex >= 0 && hourIndex < len(weatherResp.Hourly.RelativeHumidity2m) {
		humidity = weatherResp.Hourly.RelativeHumidity2m[hourIndex]
	}
	if hourIndex >= 0 && hourIndex < len(weatherResp.Hourly.ApparentTemperature) {
		feelsLike = weatherResp.Hourly.ApparentTemperature[hourIndex]
	}

	forecast := make([]WeatherForecastDay, 0, 5)
	for index, date := range weatherResp.Daily.Time {
		if index >= len(weatherResp.Daily.TemperatureMax) || index >= len(weatherResp.Daily.TemperatureMin) || index >= len(weatherResp.Daily.WeatherCode) {
			break
		}
		dayDescription, dayCondition := weatherCodeToDescription(weatherResp.Daily.WeatherCode[index])
		forecast = append(forecast, WeatherForecastDay{Date: date, High: weatherResp.Daily.TemperatureMax[index], Low: weatherResp.Daily.TemperatureMin[index], Description: dayDescription, Condition: dayCondition})
	}

	if len(forecast) != 5 {
		return nil, fmt.Errorf("weather API returned an incomplete five-day forecast")
	}

	return &WeatherData{
		Temperature: weatherResp.CurrentWeather.Temperature,
		FeelsLike:   feelsLike,
		Humidity:    humidity,
		Description: description,
		Condition:   condition,
		WindSpeed:   weatherResp.CurrentWeather.WindSpeed,
		Forecast:    forecast,
	}, nil
}

func currentWeatherHourIndex(hourlyTimes []string, currentTime string) int {
	if len(hourlyTimes) == 0 {
		return -1
	}
	currentHour := currentTime
	if len(currentHour) >= 13 {
		currentHour = currentHour[:13]
	}
	for index, hour := range hourlyTimes {
		if len(hour) >= 13 && hour[:13] == currentHour {
			return index
		}
	}
	return 0
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
