package engines

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/apimgr/search/src/model"
	"github.com/apimgr/search/src/search"
)

// OpenStreetMap implements OpenStreetMap/Nominatim search engine
// Uses the Nominatim API for geocoding and location search
type OpenStreetMap struct {
	*search.BaseEngine
	client *http.Client
}

// NewOpenStreetMap creates a new OpenStreetMap search engine
func NewOpenStreetMap() *OpenStreetMap {
	config := model.NewEngineConfig("openstreetmap")
	config.DisplayName = "OpenStreetMap"
	config.Priority = 80 // High priority for maps category
	config.Categories = []string{"maps"}
	config.SupportsTor = true

	return &OpenStreetMap{
		BaseEngine: search.NewBaseEngine(config),
		client: &http.Client{
			Timeout: time.Duration(config.GetTimeout()) * time.Second,
		},
	}
}

// nominatimResult represents a single result from Nominatim API
type nominatimResult struct {
	PlaceID     int64    `json:"place_id"`
	OSMType     string   `json:"osm_type"`
	OSMID       int64    `json:"osm_id"`
	Lat         string   `json:"lat"`
	Lon         string   `json:"lon"`
	DisplayName string   `json:"display_name"`
	Class       string   `json:"class"`
	Type        string   `json:"type"`
	Importance  float64  `json:"importance"`
	Icon        string   `json:"icon,omitempty"`
	BoundingBox []string `json:"boundingbox,omitempty"`
	Address     struct {
		HouseNumber   string `json:"house_number,omitempty"`
		Road          string `json:"road,omitempty"`
		Suburb        string `json:"suburb,omitempty"`
		City          string `json:"city,omitempty"`
		Town          string `json:"town,omitempty"`
		Village       string `json:"village,omitempty"`
		County        string `json:"county,omitempty"`
		State         string `json:"state,omitempty"`
		Postcode      string `json:"postcode,omitempty"`
		Country       string `json:"country,omitempty"`
		CountryCode   string `json:"country_code,omitempty"`
		Municipality  string `json:"municipality,omitempty"`
		StateDistrict string `json:"state_district,omitempty"`
	} `json:"address,omitempty"`
}

// Search performs an OpenStreetMap/Nominatim search
func (e *OpenStreetMap) Search(ctx context.Context, query *model.Query) ([]model.Result, error) {
	// Nominatim search API endpoint
	baseURL := "https://nominatim.openstreetmap.org/search"

	params := url.Values{}
	params.Set("q", query.Text)
	params.Set("format", "json")
	params.Set("addressdetails", "1")
	params.Set("limit", strconv.Itoa(e.GetConfig().GetMaxResults()))
	params.Set("dedupe", "1")

	// Add language preference if specified
	if query.Language != "" {
		params.Set("accept-language", query.Language)
	} else {
		params.Set("accept-language", "en")
	}

	reqURL := fmt.Sprintf("%s?%s", baseURL, params.Encode())

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, err
	}

	// Nominatim requires a valid User-Agent with contact info per their usage policy
	// Using the standard UserAgent which includes application name
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Nominatim API returned status %d", resp.StatusCode)
	}

	var nominatimResults []nominatimResult
	if err := json.NewDecoder(resp.Body).Decode(&nominatimResults); err != nil {
		return nil, err
	}

	return e.parseResults(nominatimResults, query), nil
}

// parseResults converts Nominatim results to search results
func (e *OpenStreetMap) parseResults(nominatimResults []nominatimResult, query *model.Query) []model.Result {
	results := make([]model.Result, 0, len(nominatimResults))

	for i, nr := range nominatimResults {
		if i >= e.GetConfig().GetMaxResults() {
			break
		}

		// Build title from location type and name
		title := e.buildTitle(nr)

		// Build URL to OpenStreetMap
		osmURL := e.buildOSMURL(nr)

		// Build content with address and coordinates
		content := e.buildContent(nr)

		// Store coordinates in metadata
		metadata := make(map[string]interface{})
		if lat, err := strconv.ParseFloat(nr.Lat, 64); err == nil {
			metadata["latitude"] = lat
		}
		if lon, err := strconv.ParseFloat(nr.Lon, 64); err == nil {
			metadata["longitude"] = lon
		}
		metadata["osm_type"] = nr.OSMType
		metadata["osm_id"] = nr.OSMID
		metadata["place_type"] = nr.Type
		metadata["place_class"] = nr.Class

		// Add bounding box if available
		if len(nr.BoundingBox) == 4 {
			metadata["bbox"] = nr.BoundingBox
		}

		results = append(results, model.Result{
			Title:     title,
			URL:       osmURL,
			Content:   content,
			Thumbnail: nr.Icon,
			Engine:    e.Name(),
			Category:  model.CategoryMaps,
			Score:     calculateScore(e.GetPriority(), i, 1) + nr.Importance*10,
			Position:  i,
			Metadata:  metadata,
		})
	}

	return results
}

// buildTitle creates a title for the location result
func (e *OpenStreetMap) buildTitle(nr nominatimResult) string {
	// Get the primary name from the display name (first part)
	displayParts := strings.Split(nr.DisplayName, ",")
	primaryName := strings.TrimSpace(displayParts[0])

	// Add type qualifier for clarity
	typeQualifier := ""
	switch nr.Class {
	case "place":
		switch nr.Type {
		case "city", "town", "village", "hamlet":
			typeQualifier = strings.Title(nr.Type)
		case "county", "state", "country":
			typeQualifier = strings.Title(nr.Type)
		}
	case "highway":
		typeQualifier = "Road"
	case "building":
		typeQualifier = "Building"
	case "amenity":
		typeQualifier = formatType(nr.Type)
	case "tourism":
		typeQualifier = formatType(nr.Type)
	case "natural":
		typeQualifier = formatType(nr.Type)
	case "boundary":
		if nr.Type == "administrative" {
			typeQualifier = "Administrative Area"
		}
	}

	if typeQualifier != "" && !strings.Contains(primaryName, typeQualifier) {
		return fmt.Sprintf("%s (%s)", primaryName, typeQualifier)
	}

	return primaryName
}

// buildOSMURL creates a URL to view the location on OpenStreetMap
func (e *OpenStreetMap) buildOSMURL(nr nominatimResult) string {
	// Use OSM object URL if we have type and ID
	if nr.OSMType != "" && nr.OSMID != 0 {
		osmTypeChar := "n" // node
		switch nr.OSMType {
		case "way":
			osmTypeChar = "w"
		case "relation":
			osmTypeChar = "r"
		}
		return fmt.Sprintf("https://www.openstreetmap.org/%s/%d", osmTypeChar, nr.OSMID)
	}

	// Fallback to coordinate-based URL
	return fmt.Sprintf("https://www.openstreetmap.org/?mlat=%s&mlon=%s&zoom=15", nr.Lat, nr.Lon)
}

// buildContent creates the content/description for the result
func (e *OpenStreetMap) buildContent(nr nominatimResult) string {
	parts := make([]string, 0)

	// Add formatted address
	address := e.formatAddress(nr)
	if address != "" {
		parts = append(parts, address)
	}

	// Add coordinates
	if nr.Lat != "" && nr.Lon != "" {
		lat, _ := strconv.ParseFloat(nr.Lat, 64)
		lon, _ := strconv.ParseFloat(nr.Lon, 64)
		parts = append(parts, fmt.Sprintf("Coordinates: %.6f, %.6f", lat, lon))
	}

	return strings.Join(parts, " | ")
}

// formatAddress creates a readable address from the address components
func (e *OpenStreetMap) formatAddress(nr nominatimResult) string {
	parts := make([]string, 0)

	// House number and road
	if nr.Address.HouseNumber != "" && nr.Address.Road != "" {
		parts = append(parts, fmt.Sprintf("%s %s", nr.Address.HouseNumber, nr.Address.Road))
	} else if nr.Address.Road != "" {
		parts = append(parts, nr.Address.Road)
	}

	// Suburb or neighborhood
	if nr.Address.Suburb != "" {
		parts = append(parts, nr.Address.Suburb)
	}

	// City/Town/Village
	city := nr.Address.City
	if city == "" {
		city = nr.Address.Town
	}
	if city == "" {
		city = nr.Address.Village
	}
	if city == "" {
		city = nr.Address.Municipality
	}
	if city != "" {
		parts = append(parts, city)
	}

	// State/County
	if nr.Address.State != "" {
		parts = append(parts, nr.Address.State)
	} else if nr.Address.County != "" {
		parts = append(parts, nr.Address.County)
	}

	// Postcode
	if nr.Address.Postcode != "" {
		parts = append(parts, nr.Address.Postcode)
	}

	// Country
	if nr.Address.Country != "" {
		parts = append(parts, nr.Address.Country)
	}

	return strings.Join(parts, ", ")
}

// formatType converts snake_case type to readable format
func formatType(t string) string {
	t = strings.ReplaceAll(t, "_", " ")
	return strings.Title(t)
}
