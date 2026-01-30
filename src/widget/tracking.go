package widget

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode"
)

// TrackingFetcher fetches package tracking info
// Implements the Fetcher interface for the widget system
type TrackingFetcher struct {
	httpClient  *http.Client
	apiKey      string // Optional: API key for 17track or similar service
	rateLimiter *trackingRateLimiter
}

// TrackingData represents package tracking result
type TrackingData struct {
	TrackingNumber    string          `json:"tracking_number"`
	Carrier           string          `json:"carrier"`
	CarrierCode       string          `json:"carrier_code"`
	CarrierURL        string          `json:"carrier_url"`
	Status            string          `json:"status"`
	StatusCode        string          `json:"status_code,omitempty"`
	StatusDescription string          `json:"status_description,omitempty"`
	Events            []TrackingEvent `json:"events,omitempty"`
	EstimatedDelivery string          `json:"estimated_delivery,omitempty"`
	Detected          bool            `json:"detected"`
	APIEnabled        bool            `json:"api_enabled"`
	LastUpdated       time.Time       `json:"last_updated,omitempty"`
}

// TrackingEvent represents a tracking history event
type TrackingEvent struct {
	Date        string `json:"date"`
	Time        string `json:"time,omitempty"`
	Location    string `json:"location"`
	City        string `json:"city,omitempty"`
	State       string `json:"state,omitempty"`
	Country     string `json:"country,omitempty"`
	Description string `json:"description"`
	StatusCode  string `json:"status_code,omitempty"`
}

// CarrierInfo represents carrier detection info
type CarrierInfo struct {
	Name     string
	Code     string
	Pattern  *regexp.Regexp
	TrackURL string
	Priority int // Higher priority patterns are checked first
}

// trackingRateLimiter implements simple rate limiting for API calls
type trackingRateLimiter struct {
	mu          sync.Mutex
	requests    map[string][]time.Time
	maxRequests int
	window      time.Duration
}

// newTrackingRateLimiter creates a rate limiter
func newTrackingRateLimiter(maxRequests int, window time.Duration) *trackingRateLimiter {
	return &trackingRateLimiter{
		requests:    make(map[string][]time.Time),
		maxRequests: maxRequests,
		window:      window,
	}
}

// Allow checks if a request is allowed for the given key
func (rl *trackingRateLimiter) Allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	windowStart := now.Add(-rl.window)

	// Clean old entries and count recent requests
	var recent []time.Time
	for _, t := range rl.requests[key] {
		if t.After(windowStart) {
			recent = append(recent, t)
		}
	}

	if len(recent) >= rl.maxRequests {
		rl.requests[key] = recent
		return false
	}

	rl.requests[key] = append(recent, now)
	return true
}

// Carrier patterns ordered by specificity (most specific first)
// Per requirements:
// - USPS: starts with 9, 20-22 digits
// - UPS: starts with 1Z, 18 chars
// - FedEx: 12-15 digits
// - DHL: 10-11 digits
var carrierPatterns = []CarrierInfo{
	// USPS: starts with 9, 20-22 digits total
	{
		Name:     "USPS",
		Code:     "usps",
		Pattern:  regexp.MustCompile(`^9\d{19,21}$`),
		TrackURL: "https://tools.usps.com/go/TrackConfirmAction?tLabels=",
		Priority: 100,
	},
	// USPS: Additional formats (20-22 digits starting with specific prefixes)
	{
		Name:     "USPS",
		Code:     "usps",
		Pattern:  regexp.MustCompile(`^(94|93|92|91|90)\d{18,20}$`),
		TrackURL: "https://tools.usps.com/go/TrackConfirmAction?tLabels=",
		Priority: 99,
	},
	// USPS: Certified mail, registered mail (13 chars: 2 letters + 9 digits + US)
	{
		Name:     "USPS",
		Code:     "usps",
		Pattern:  regexp.MustCompile(`^[A-Z]{2}\d{9}US$`),
		TrackURL: "https://tools.usps.com/go/TrackConfirmAction?tLabels=",
		Priority: 98,
	},
	// UPS: starts with 1Z, 18 characters total (1Z + 16 alphanumeric)
	{
		Name:     "UPS",
		Code:     "ups",
		Pattern:  regexp.MustCompile(`^1Z[0-9A-Z]{16}$`),
		TrackURL: "https://www.ups.com/track?tracknum=",
		Priority: 95,
	},
	// UPS: Mail Innovations (starts with T, 11 characters)
	{
		Name:     "UPS",
		Code:     "ups",
		Pattern:  regexp.MustCompile(`^T\d{10}$`),
		TrackURL: "https://www.ups.com/track?tracknum=",
		Priority: 94,
	},
	// UPS: Freight (9-12 digits)
	{
		Name:     "UPS",
		Code:     "ups",
		Pattern:  regexp.MustCompile(`^[0-9]{9,12}$`),
		TrackURL: "https://www.ups.com/track?tracknum=",
		Priority: 50, // Lower priority to avoid conflicts
	},
	// FedEx: 12 digits (most common)
	{
		Name:     "FedEx",
		Code:     "fedex",
		Pattern:  regexp.MustCompile(`^\d{12}$`),
		TrackURL: "https://www.fedex.com/fedextrack/?trknbr=",
		Priority: 90,
	},
	// FedEx: 15 digits (door tag)
	{
		Name:     "FedEx",
		Code:     "fedex",
		Pattern:  regexp.MustCompile(`^\d{15}$`),
		TrackURL: "https://www.fedex.com/fedextrack/?trknbr=",
		Priority: 89,
	},
	// FedEx: 20-22 digits (SmartPost uses 20, 22 chars)
	{
		Name:     "FedEx",
		Code:     "fedex",
		Pattern:  regexp.MustCompile(`^\d{20,22}$`),
		TrackURL: "https://www.fedex.com/fedextrack/?trknbr=",
		Priority: 88,
	},
	// DHL: 10-11 digits (standard)
	{
		Name:     "DHL",
		Code:     "dhl",
		Pattern:  regexp.MustCompile(`^\d{10,11}$`),
		TrackURL: "https://www.dhl.com/us-en/home/tracking.html?tracking-id=",
		Priority: 85,
	},
	// DHL: eCommerce (starts with JD + 18 digits or GM + 16-17 digits)
	{
		Name:     "DHL eCommerce",
		Code:     "dhl_ecommerce",
		Pattern:  regexp.MustCompile(`^(JD\d{18}|GM\d{16,17})$`),
		TrackURL: "https://www.dhl.com/us-en/home/tracking.html?tracking-id=",
		Priority: 84,
	},
	// DHL Express: 10 digits (waybill)
	{
		Name:     "DHL Express",
		Code:     "dhl_express",
		Pattern:  regexp.MustCompile(`^[0-9]{10}$`),
		TrackURL: "https://www.dhl.com/us-en/home/tracking.html?tracking-id=",
		Priority: 83,
	},
	// Amazon: TBA + 12 digits
	{
		Name:     "Amazon Logistics",
		Code:     "amazon",
		Pattern:  regexp.MustCompile(`^TBA\d{12,15}$`),
		TrackURL: "https://www.amazon.com/gp/css/shiptrack/view.html?trackingId=",
		Priority: 80,
	},
	// Royal Mail (UK): 2 letters + 9 digits + GB
	{
		Name:     "Royal Mail",
		Code:     "royal_mail",
		Pattern:  regexp.MustCompile(`^[A-Z]{2}\d{9}GB$`),
		TrackURL: "https://www.royalmail.com/track-your-item#/tracking-results/",
		Priority: 75,
	},
	// Canada Post: 16 digits
	{
		Name:     "Canada Post",
		Code:     "canada_post",
		Pattern:  regexp.MustCompile(`^\d{16}$`),
		TrackURL: "https://www.canadapost-postescanada.ca/track-reperage/en#/search?searchFor=",
		Priority: 74,
	},
	// Deutsche Post/DHL: 2 letters + 9 digits + DE
	{
		Name:     "Deutsche Post",
		Code:     "deutsche_post",
		Pattern:  regexp.MustCompile(`^[A-Z]{2}\d{9}DE$`),
		TrackURL: "https://www.dhl.de/en/privatkunden/pakete-empfangen/verfolgen.html?piececode=",
		Priority: 73,
	},
	// La Poste (France): 2 letters + 9 digits + FR
	{
		Name:     "La Poste",
		Code:     "laposte",
		Pattern:  regexp.MustCompile(`^[A-Z]{2}\d{9}FR$`),
		TrackURL: "https://www.laposte.fr/outils/suivre-vos-envois?code=",
		Priority: 72,
	},
	// China Post: 2 letters + 9 digits + CN
	{
		Name:     "China Post",
		Code:     "china_post",
		Pattern:  regexp.MustCompile(`^[A-Z]{2}\d{9}CN$`),
		TrackURL: "https://www.17track.net/en/track?nums=",
		Priority: 71,
	},
	// Japan Post: 2 letters + 9 digits + JP
	{
		Name:     "Japan Post",
		Code:     "japan_post",
		Pattern:  regexp.MustCompile(`^[A-Z]{2}\d{9}JP$`),
		TrackURL: "https://trackings.post.japanpost.jp/services/srv/search/direct?searchKind=S002&locale=en&reqCodeNo1=",
		Priority: 70,
	},
	// Australia Post: 2 letters + 9 digits + AU
	{
		Name:     "Australia Post",
		Code:     "australia_post",
		Pattern:  regexp.MustCompile(`^[A-Z]{2}\d{9}AU$`),
		TrackURL: "https://auspost.com.au/mypost/track/#/details/",
		Priority: 69,
	},
	// OnTrac: C + 14 digits
	{
		Name:     "OnTrac",
		Code:     "ontrac",
		Pattern:  regexp.MustCompile(`^C\d{14}$`),
		TrackURL: "https://www.ontrac.com/tracking/?trackingnumber=",
		Priority: 65,
	},
	// LaserShip: 1LS + 12 alphanumeric or LX + 10 digits
	{
		Name:     "LaserShip",
		Code:     "lasership",
		Pattern:  regexp.MustCompile(`^(1LS[A-Z0-9]{12}|LX\d{10})$`),
		TrackURL: "https://www.lasership.com/track/",
		Priority: 64,
	},
	// Purolator: 3 letters + 9 digits
	{
		Name:     "Purolator",
		Code:     "purolator",
		Pattern:  regexp.MustCompile(`^[A-Z]{3}\d{9}$`),
		TrackURL: "https://www.purolator.com/en/shipping/tracker?pin=",
		Priority: 63,
	},
}

// TrackingConfig holds configuration for the tracking fetcher
type TrackingConfig struct {
	APIKey         string        // API key for 17track or similar
	APIEnabled     bool          // Whether to use API for live tracking
	RateLimitMax   int           // Max requests per window (default: 10)
	RateLimitWindow time.Duration // Rate limit window (default: 1 minute)
}

// NewTrackingFetcher creates a basic tracking fetcher without API support
// For backward compatibility - use NewTrackingFetcherWithConfig for custom configuration
func NewTrackingFetcher() *TrackingFetcher {
	return NewTrackingFetcherWithConfig(nil)
}

// NewTrackingFetcherWithConfig creates a new tracking fetcher with optional API support
func NewTrackingFetcherWithConfig(cfg *TrackingConfig) *TrackingFetcher {
	var apiKey string
	var rateLimiter *trackingRateLimiter

	if cfg != nil {
		apiKey = cfg.APIKey
		maxReq := cfg.RateLimitMax
		if maxReq <= 0 {
			maxReq = 10 // Default: 10 requests per window
		}
		window := cfg.RateLimitWindow
		if window <= 0 {
			window = time.Minute // Default: 1 minute window
		}
		rateLimiter = newTrackingRateLimiter(maxReq, window)
	} else {
		rateLimiter = newTrackingRateLimiter(10, time.Minute)
	}

	return &TrackingFetcher{
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
		apiKey:      apiKey,
		rateLimiter: rateLimiter,
	}
}

// NewTrackingFetcherSimple creates a basic tracking fetcher without API support
// This is an alias for NewTrackingFetcherDefault() for clearer naming
func NewTrackingFetcherSimple() *TrackingFetcher {
	return NewTrackingFetcherWithConfig(nil)
}

// NewTrackingFetcherDefault creates a basic tracking fetcher with default settings
// Backward-compatible alias - use NewTrackingFetcherWithConfig for custom configuration
func NewTrackingFetcherDefault() *TrackingFetcher {
	return NewTrackingFetcherWithConfig(nil)
}

// Fetch detects carrier and provides tracking URL
// Implements the Fetcher interface
func (f *TrackingFetcher) Fetch(ctx context.Context, params map[string]string) (*WidgetData, error) {
	trackingNumber := params["number"]
	if trackingNumber == "" {
		return &WidgetData{
			Type:      WidgetTracking,
			Error:     "tracking number required",
			UpdatedAt: time.Now(),
		}, nil
	}

	// Clean up tracking number
	trackingNumber = cleanTrackingNumber(trackingNumber)

	// Validate minimum length
	if len(trackingNumber) < 8 {
		return &WidgetData{
			Type:      WidgetTracking,
			Error:     "tracking number too short (minimum 8 characters)",
			UpdatedAt: time.Now(),
		}, nil
	}

	// Detect carrier
	carrier := detectCarrier(trackingNumber)

	data := &TrackingData{
		TrackingNumber: trackingNumber,
		Carrier:        carrier.Name,
		CarrierCode:    carrier.Code,
		CarrierURL:     carrier.TrackURL + trackingNumber,
		Status:         "Click to track on carrier website",
		Detected:       carrier.Name != "Unknown",
		APIEnabled:     f.apiKey != "",
		LastUpdated:    time.Now(),
	}

	// If API key is configured, try to fetch live tracking data
	if f.apiKey != "" && carrier.Name != "Unknown" {
		// Check rate limit before making API call
		if f.rateLimiter.Allow(trackingNumber) {
			liveData, err := f.fetchLiveTracking(ctx, trackingNumber, carrier.Code)
			if err == nil && liveData != nil {
				// Merge live data with carrier detection
				data.Status = liveData.Status
				data.StatusCode = liveData.StatusCode
				data.StatusDescription = liveData.StatusDescription
				data.Events = liveData.Events
				data.EstimatedDelivery = liveData.EstimatedDelivery
			}
			// If API call fails, fall back to carrier detection only (no error returned)
		}
	}

	return &WidgetData{
		Type:      WidgetTracking,
		Data:      data,
		UpdatedAt: time.Now(),
	}, nil
}

// fetchLiveTracking fetches live tracking data from 17track API
// This is optional and requires an API key from 17track.net
func (f *TrackingFetcher) fetchLiveTracking(ctx context.Context, trackingNumber, carrierCode string) (*TrackingData, error) {
	if f.apiKey == "" {
		return nil, fmt.Errorf("API key not configured")
	}

	// 17track API endpoint
	url := "https://api.17track.net/track/v2/gettrackinfo"

	// Prepare request body
	reqBody := map[string]interface{}{
		"number": trackingNumber,
	}
	if carrierCode != "" {
		reqBody["carrier"] = map[string]string{"code": carrierCode}
	}

	bodyBytes, err := json.Marshal([]map[string]interface{}{reqBody})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, strings.NewReader(string(bodyBytes)))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("17token", f.apiKey)

	resp, err := f.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, fmt.Errorf("rate limited by API")
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	// Parse 17track response
	var apiResp struct {
		Code int `json:"code"`
		Data struct {
			Accepted []struct {
				Number  string `json:"number"`
				Carrier struct {
					Code string `json:"code"`
					Name string `json:"name"`
				} `json:"carrier"`
				Track struct {
					Status    string `json:"e"`
					Time      string `json:"z0"`
					Providers []struct {
						Provider struct {
							Name string `json:"name"`
						} `json:"provider"`
						LatestEvent struct {
							Status string `json:"a"`
							Time   string `json:"b"`
							Place  string `json:"c"`
							Desc   string `json:"z"`
						} `json:"latest_event"`
						Events []struct {
							Status string `json:"a"`
							Time   string `json:"b"`
							Place  string `json:"c"`
							Desc   string `json:"z"`
						} `json:"events"`
						DeliveryTime string `json:"delivery_time"`
					} `json:"providers"`
				} `json:"track"`
			} `json:"accepted"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, err
	}

	if len(apiResp.Data.Accepted) == 0 {
		return nil, fmt.Errorf("no tracking data found")
	}

	accepted := apiResp.Data.Accepted[0]
	data := &TrackingData{
		TrackingNumber: trackingNumber,
		Status:         mapTrackingStatus(accepted.Track.Status),
		StatusCode:     accepted.Track.Status,
	}

	// Extract events from first provider
	if len(accepted.Track.Providers) > 0 {
		provider := accepted.Track.Providers[0]
		data.EstimatedDelivery = provider.DeliveryTime

		for _, event := range provider.Events {
			data.Events = append(data.Events, TrackingEvent{
				Date:        extractDate(event.Time),
				Time:        extractTime(event.Time),
				Location:    event.Place,
				Description: event.Desc,
				StatusCode:  event.Status,
			})
		}
	}

	return data, nil
}

// mapTrackingStatus maps 17track status codes to human-readable status
func mapTrackingStatus(code string) string {
	statusMap := map[string]string{
		"NotFound":     "Not Found",
		"InfoReceived": "Information Received",
		"InTransit":    "In Transit",
		"Expired":      "Expired",
		"PickedUp":     "Picked Up",
		"Undelivered":  "Delivery Attempted",
		"Delivered":    "Delivered",
		"Alert":        "Alert - Check Details",
	}
	if status, ok := statusMap[code]; ok {
		return status
	}
	return code
}

// extractDate extracts date portion from datetime string
func extractDate(datetime string) string {
	if len(datetime) >= 10 {
		return datetime[:10]
	}
	return datetime
}

// extractTime extracts time portion from datetime string
func extractTime(datetime string) string {
	if len(datetime) > 11 {
		return datetime[11:]
	}
	return ""
}

// cleanTrackingNumber removes common formatting from tracking numbers
// and normalizes case
func cleanTrackingNumber(s string) string {
	var result strings.Builder
	result.Grow(len(s))
	for _, c := range s {
		if unicode.IsLetter(c) || unicode.IsDigit(c) {
			result.WriteRune(unicode.ToUpper(c))
		}
	}
	return result.String()
}

// detectCarrier identifies the carrier from tracking number format
// Patterns are checked in priority order (highest first)
func detectCarrier(trackingNumber string) CarrierInfo {
	// Sort patterns by priority (already sorted in slice, but ensure)
	for _, carrier := range carrierPatterns {
		if carrier.Pattern.MatchString(trackingNumber) {
			return carrier
		}
	}

	// Return generic tracking search if no carrier detected
	return CarrierInfo{
		Name:     "Unknown",
		Code:     "unknown",
		TrackURL: "https://www.17track.net/en/track?nums=",
	}
}

// DetectCarrierFromNumber is a public helper to detect carrier without creating a fetcher
func DetectCarrierFromNumber(trackingNumber string) (name, code, url string, detected bool) {
	cleaned := cleanTrackingNumber(trackingNumber)
	carrier := detectCarrier(cleaned)
	return carrier.Name, carrier.Code, carrier.TrackURL + cleaned, carrier.Name != "Unknown"
}

// ValidateTrackingNumber checks if a tracking number matches any known carrier pattern
func ValidateTrackingNumber(trackingNumber string) bool {
	cleaned := cleanTrackingNumber(trackingNumber)
	carrier := detectCarrier(cleaned)
	return carrier.Name != "Unknown"
}

// GetSupportedCarriers returns a list of supported carriers
func GetSupportedCarriers() []struct {
	Name string `json:"name"`
	Code string `json:"code"`
} {
	seen := make(map[string]bool)
	var carriers []struct {
		Name string `json:"name"`
		Code string `json:"code"`
	}
	for _, c := range carrierPatterns {
		if !seen[c.Code] {
			seen[c.Code] = true
			carriers = append(carriers, struct {
				Name string `json:"name"`
				Code string `json:"code"`
			}{Name: c.Name, Code: c.Code})
		}
	}
	return carriers
}

// CacheDuration returns how long to cache tracking data
// Returns shorter duration (5 min) for API-enabled tracking to get fresh updates
// Returns longer duration (15 min) for carrier-detection-only mode
func (f *TrackingFetcher) CacheDuration() time.Duration {
	if f.apiKey != "" {
		return 5 * time.Minute // Shorter TTL when API is enabled for fresher data
	}
	return 15 * time.Minute // Longer TTL for carrier detection only
}

// WidgetType returns the widget type
// Implements the Fetcher interface
func (f *TrackingFetcher) WidgetType() WidgetType {
	return WidgetTracking
}

// HasAPIEnabled returns whether API tracking is enabled
func (f *TrackingFetcher) HasAPIEnabled() bool {
	return f.apiKey != ""
}
