package widget

import (
	"context"
	"regexp"
	"time"
)

// TrackingFetcher fetches package tracking info
type TrackingFetcher struct{}

// TrackingData represents package tracking result
type TrackingData struct {
	TrackingNumber string           `json:"tracking_number"`
	Carrier        string           `json:"carrier"`
	CarrierURL     string           `json:"carrier_url"`
	Status         string           `json:"status"`
	Events         []TrackingEvent  `json:"events,omitempty"`
	Detected       bool             `json:"detected"`
}

// TrackingEvent represents a tracking history event
type TrackingEvent struct {
	Date        string `json:"date"`
	Location    string `json:"location"`
	Description string `json:"description"`
}

// CarrierInfo represents carrier detection info
type CarrierInfo struct {
	Name    string
	Pattern *regexp.Regexp
	TrackURL string
}

// Common carrier patterns
var carrierPatterns = []CarrierInfo{
	{
		Name:    "USPS",
		Pattern: regexp.MustCompile(`^(94|93|92|91|90|94\d{20}|9[0-4]\d{20,26})$`),
		TrackURL: "https://tools.usps.com/go/TrackConfirmAction?tLabels=",
	},
	{
		Name:    "UPS",
		Pattern: regexp.MustCompile(`^1Z[0-9A-Z]{15,18}$|^T\d{10}$|^[0-9]{26}$`),
		TrackURL: "https://www.ups.com/track?tracknum=",
	},
	{
		Name:    "FedEx",
		Pattern: regexp.MustCompile(`^(\d{12}|\d{15}|\d{20}|[0-9]{22})$`),
		TrackURL: "https://www.fedex.com/fedextrack/?trknbr=",
	},
	{
		Name:    "DHL",
		Pattern: regexp.MustCompile(`^[0-9]{10,11}$|^JD\d{18}$|^[0-9]{16}$`),
		TrackURL: "https://www.dhl.com/us-en/home/tracking.html?tracking-id=",
	},
	{
		Name:    "Amazon",
		Pattern: regexp.MustCompile(`^TBA\d{12}$`),
		TrackURL: "https://www.amazon.com/gp/css/shiptrack/view.html?trackingId=",
	},
	{
		Name:    "Royal Mail",
		Pattern: regexp.MustCompile(`^[A-Z]{2}\d{9}GB$`),
		TrackURL: "https://www.royalmail.com/track-your-item#/tracking-results/",
	},
	{
		Name:    "Canada Post",
		Pattern: regexp.MustCompile(`^\d{16}$`),
		TrackURL: "https://www.canadapost-postescanada.ca/track-reperage/en#/search?searchFor=",
	},
	{
		Name:    "Deutsche Post/DHL",
		Pattern: regexp.MustCompile(`^[A-Z]{2}\d{9}DE$`),
		TrackURL: "https://www.dhl.de/en/privatkunden/pakete-empfangen/verfolgen.html?piececode=",
	},
	{
		Name:    "La Poste",
		Pattern: regexp.MustCompile(`^[A-Z]{2}\d{9}FR$`),
		TrackURL: "https://www.laposte.fr/outils/suivre-vos-envois?code=",
	},
	{
		Name:    "China Post",
		Pattern: regexp.MustCompile(`^[A-Z]{2}\d{9}CN$`),
		TrackURL: "http://track.4px.com/#/result/0/",
	},
	{
		Name:    "Japan Post",
		Pattern: regexp.MustCompile(`^[A-Z]{2}\d{9}JP$`),
		TrackURL: "https://trackings.post.japanpost.jp/services/srv/search/direct?searchKind=S002&locale=en&reqCodeNo1=",
	},
}

// NewTrackingFetcher creates a new tracking fetcher
func NewTrackingFetcher() *TrackingFetcher {
	return &TrackingFetcher{}
}

// Fetch detects carrier and provides tracking URL
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

	// Detect carrier
	carrier := detectCarrier(trackingNumber)

	data := &TrackingData{
		TrackingNumber: trackingNumber,
		Carrier:        carrier.Name,
		CarrierURL:     carrier.TrackURL + trackingNumber,
		Status:         "Click to track on carrier website",
		Detected:       carrier.Name != "Unknown",
	}

	// Note: Actual tracking status would require carrier API integrations
	// which typically require business accounts. For now, we provide
	// carrier detection and direct link to carrier tracking page.

	return &WidgetData{
		Type:      WidgetTracking,
		Data:      data,
		UpdatedAt: time.Now(),
	}, nil
}

// cleanTrackingNumber removes common formatting from tracking numbers
func cleanTrackingNumber(s string) string {
	// Remove spaces, dashes, and other common separators
	result := ""
	for _, c := range s {
		if (c >= '0' && c <= '9') || (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') {
			result += string(c)
		}
	}
	return result
}

// detectCarrier identifies the carrier from tracking number format
func detectCarrier(trackingNumber string) CarrierInfo {
	for _, carrier := range carrierPatterns {
		if carrier.Pattern.MatchString(trackingNumber) {
			return carrier
		}
	}

	// Return generic tracking search if no carrier detected
	return CarrierInfo{
		Name:    "Unknown",
		TrackURL: "https://www.google.com/search?q=track+package+",
	}
}

// CacheDuration returns how long to cache tracking data
func (f *TrackingFetcher) CacheDuration() time.Duration {
	return 5 * time.Minute
}

// WidgetType returns the widget type
func (f *TrackingFetcher) WidgetType() WidgetType {
	return WidgetTracking
}
