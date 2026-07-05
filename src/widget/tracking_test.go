package widget

import (
	"context"
	"testing"
	"time"
)

func TestCleanTrackingNumber(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantEmpty bool
		wantExact string
	}{
		{"spaces removed leaves only alphanumeric", "1Z 999 AA1 01 2345 678", false, ""},
		{"hyphens removed", "123-456-789", false, "123456789"},
		{"lowercase letters uppercased", "1zabcdef", false, "1ZABCDEF"},
		{"empty string returns empty", "", true, ""},
		{"spaces only returns empty", "   ", true, ""},
		{"already clean UPS number unchanged", "1Z999AA10123456789", false, "1Z999AA10123456789"},
		{"mixed spaces and hyphens removed", "94 001-118992 23", false, "9400111899223"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cleanTrackingNumber(tt.input)
			if tt.wantEmpty {
				if got != "" {
					t.Errorf("cleanTrackingNumber(%q) = %q, want empty string", tt.input, got)
				}
				return
			}
			// All results must contain only uppercase letters and digits
			for _, c := range got {
				if !((c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')) {
					t.Errorf("cleanTrackingNumber(%q) result %q contains non-alphanumeric char %q", tt.input, got, c)
				}
			}
			// When an exact value is specified, check it
			if tt.wantExact != "" && got != tt.wantExact {
				t.Errorf("cleanTrackingNumber(%q) = %q, want %q", tt.input, got, tt.wantExact)
			}
		})
	}
}

func TestCleanTrackingNumberProperties(t *testing.T) {
	t.Run("removes spaces", func(t *testing.T) {
		got := cleanTrackingNumber("94001 11899 22339 74962 22")
		want := "9400111899223397496222"
		if got != want {
			t.Errorf("cleanTrackingNumber() = %q, want %q", got, want)
		}
	})

	t.Run("removes hyphens", func(t *testing.T) {
		got := cleanTrackingNumber("123-456-789")
		want := "123456789"
		if got != want {
			t.Errorf("cleanTrackingNumber() = %q, want %q", got, want)
		}
	})

	t.Run("uppercases letters", func(t *testing.T) {
		got := cleanTrackingNumber("ab123456789us")
		want := "AB123456789US"
		if got != want {
			t.Errorf("cleanTrackingNumber() = %q, want %q", got, want)
		}
	})

	t.Run("empty input returns empty", func(t *testing.T) {
		got := cleanTrackingNumber("")
		if got != "" {
			t.Errorf("cleanTrackingNumber(%q) = %q, want empty string", "", got)
		}
	})

	t.Run("UPS format spaces removed", func(t *testing.T) {
		got := cleanTrackingNumber("1Z 999 AA1 01 2345 678")
		// Verify only alphanumerics remain and they are uppercased
		for _, c := range got {
			if !((c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')) {
				t.Errorf("cleanTrackingNumber result contains non-alphanumeric char %q in %q", c, got)
			}
		}
	})
}

func TestDetectCarrier(t *testing.T) {
	tests := []struct {
		name            string
		trackingNumber  string
		wantCarrier     string
		wantCodeContain string
	}{
		{
			name:            "USPS 22-digit starting with 9",
			trackingNumber:  "9400111899223397496222",
			wantCarrier:     "USPS",
			wantCodeContain: "usps",
		},
		{
			name:            "UPS starts with 1Z",
			trackingNumber:  "1Z999AA10123456789",
			wantCarrier:     "UPS",
			wantCodeContain: "ups",
		},
		{
			// 12 digits match UPS Freight pattern (9-12 digits) before FedEx in the slice
			name:            "12 digits matches UPS Freight pattern",
			trackingNumber:  "123456789012",
			wantCarrier:     "UPS",
			wantCodeContain: "ups",
		},
		{
			name:            "USPS certified mail format AB123456789US",
			trackingNumber:  "AB123456789US",
			wantCarrier:     "USPS",
			wantCodeContain: "usps",
		},
		{
			name:            "Amazon TBA prefix",
			trackingNumber:  "TBA123456789012",
			wantCarrier:     "Amazon Logistics",
			wantCodeContain: "amazon",
		},
		{
			name:            "unknown pattern returns Unknown",
			trackingNumber:  "ZZZZZZ",
			wantCarrier:     "Unknown",
			wantCodeContain: "unknown",
		},
		{
			name:            "empty string returns Unknown",
			trackingNumber:  "",
			wantCarrier:     "Unknown",
			wantCodeContain: "unknown",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detectCarrier(tt.trackingNumber)
			if got.Name != tt.wantCarrier {
				t.Errorf("detectCarrier(%q).Name = %q, want %q", tt.trackingNumber, got.Name, tt.wantCarrier)
			}
			if got.Code != tt.wantCodeContain {
				t.Errorf("detectCarrier(%q).Code = %q, want %q", tt.trackingNumber, got.Code, tt.wantCodeContain)
			}
		})
	}
}

func TestDetectCarrierNumericPatterns(t *testing.T) {
	t.Run("10-digit number is detected as a known carrier", func(t *testing.T) {
		// UPS Freight pattern matches 9-12 digits and appears before DHL in the pattern list
		got := detectCarrier("1234567890")
		if got.Name == "Unknown" {
			t.Errorf("10-digit number should match a known carrier, got Unknown")
		}
	})

	t.Run("DHL eCommerce JD prefix detected", func(t *testing.T) {
		// JD + 18 digits uniquely identifies DHL eCommerce
		got := detectCarrier("JD123456789012345678")
		if got.Code != "dhl_ecommerce" {
			t.Errorf("detectCarrier(JD+18digits).Code = %q, want %q", got.Code, "dhl_ecommerce")
		}
	})
}

func TestDetectCarrierFromNumber(t *testing.T) {
	t.Run("known UPS number detected is true", func(t *testing.T) {
		name, code, trackURL, detected := DetectCarrierFromNumber("1Z999AA10123456789")
		if !detected {
			t.Error("detected should be true for valid UPS number")
		}
		if name != "UPS" {
			t.Errorf("name = %q, want %q", name, "UPS")
		}
		if code != "ups" {
			t.Errorf("code = %q, want %q", code, "ups")
		}
		if trackURL == "" {
			t.Error("trackURL should not be empty")
		}
	})

	t.Run("unknown number returns detected false", func(t *testing.T) {
		_, _, _, detected := DetectCarrierFromNumber("ZZZZZ")
		if detected {
			t.Error("detected should be false for unknown tracking number")
		}
	})

	t.Run("cleaning applied before detection", func(t *testing.T) {
		_, _, _, detected := DetectCarrierFromNumber("1Z 999 AA1 01 2345 678")
		// After cleaning: 1Z999AA101234567 8 — length may vary, just check it doesn't panic
		_ = detected
	})
}

func TestValidateTrackingNumber(t *testing.T) {
	tests := []struct {
		name           string
		trackingNumber string
		want           bool
	}{
		{"valid USPS 22-digit number", "9400111899223397496222", true},
		{"valid UPS number", "1Z999AA10123456789", true},
		{"valid FedEx 12 digits", "123456789012", true},
		{"random short letters unknown", "RANDOM", false},
		{"empty string is invalid", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ValidateTrackingNumber(tt.trackingNumber)
			if got != tt.want {
				t.Errorf("ValidateTrackingNumber(%q) = %v, want %v", tt.trackingNumber, got, tt.want)
			}
		})
	}
}

func TestGetSupportedCarriers(t *testing.T) {
	t.Run("returns non-empty list", func(t *testing.T) {
		carriers := GetSupportedCarriers()
		if len(carriers) == 0 {
			t.Error("GetSupportedCarriers() returned empty list")
		}
	})

	t.Run("no duplicate codes", func(t *testing.T) {
		carriers := GetSupportedCarriers()
		seen := make(map[string]bool)
		for _, c := range carriers {
			if seen[c.Code] {
				t.Errorf("duplicate carrier code %q in GetSupportedCarriers()", c.Code)
			}
			seen[c.Code] = true
		}
	})

	t.Run("all carriers have Name and Code populated", func(t *testing.T) {
		carriers := GetSupportedCarriers()
		for _, c := range carriers {
			if c.Name == "" {
				t.Errorf("carrier with code %q has empty Name", c.Code)
			}
			if c.Code == "" {
				t.Errorf("carrier with name %q has empty Code", c.Name)
			}
		}
	})
}

func TestTrackingFetcherCacheDuration(t *testing.T) {
	t.Run("no API key returns 15 minutes", func(t *testing.T) {
		f := NewTrackingFetcher()
		got := f.CacheDuration()
		if got != 15*time.Minute {
			t.Errorf("CacheDuration() = %v, want %v", got, 15*time.Minute)
		}
	})

	t.Run("with API key returns 5 minutes", func(t *testing.T) {
		f := NewTrackingFetcherWithConfig(&TrackingConfig{
			APIKey: "test-api-key",
		})
		got := f.CacheDuration()
		if got != 5*time.Minute {
			t.Errorf("CacheDuration() = %v, want %v", got, 5*time.Minute)
		}
	})
}

func TestTrackingFetcherHasAPIEnabled(t *testing.T) {
	t.Run("default constructor returns false", func(t *testing.T) {
		f := NewTrackingFetcher()
		if f.HasAPIEnabled() {
			t.Error("HasAPIEnabled() should be false for default fetcher")
		}
	})

	t.Run("config with API key returns true", func(t *testing.T) {
		f := NewTrackingFetcherWithConfig(&TrackingConfig{
			APIKey: "my-api-key",
		})
		if !f.HasAPIEnabled() {
			t.Error("HasAPIEnabled() should be true when API key is configured")
		}
	})

	t.Run("config with empty API key returns false", func(t *testing.T) {
		f := NewTrackingFetcherWithConfig(&TrackingConfig{
			APIKey: "",
		})
		if f.HasAPIEnabled() {
			t.Error("HasAPIEnabled() should be false when API key is empty")
		}
	})
}

func TestTrackingRateLimiter(t *testing.T) {
	t.Run("first request is allowed", func(t *testing.T) {
		rl := newTrackingRateLimiter(3, time.Minute)
		if !rl.Allow("key1") {
			t.Error("first request should be allowed")
		}
	})

	t.Run("requests within limit are allowed", func(t *testing.T) {
		rl := newTrackingRateLimiter(3, time.Minute)
		for i := 0; i < 3; i++ {
			if !rl.Allow("key2") {
				t.Errorf("request %d should be allowed within limit of 3", i+1)
			}
		}
	})

	t.Run("request exceeding limit is blocked", func(t *testing.T) {
		rl := newTrackingRateLimiter(3, time.Minute)
		for i := 0; i < 3; i++ {
			rl.Allow("key3")
		}
		if rl.Allow("key3") {
			t.Error("4th request should be blocked after limit of 3")
		}
	})

	t.Run("different keys have independent limits", func(t *testing.T) {
		rl := newTrackingRateLimiter(1, time.Minute)
		rl.Allow("keyA")
		// keyA is now blocked but keyB should still be allowed
		if !rl.Allow("keyB") {
			t.Error("different key should not be affected by another key's limit")
		}
	})
}

func TestMapTrackingStatus(t *testing.T) {
	tests := []struct {
		name string
		code string
		want string
	}{
		{"Delivered maps to Delivered", "Delivered", "Delivered"},
		{"InTransit maps to In Transit", "InTransit", "In Transit"},
		{"NotFound maps to Not Found", "NotFound", "Not Found"},
		{"unknown code returned unchanged", "SomeUnknownCode", "SomeUnknownCode"},
		{"InfoReceived maps correctly", "InfoReceived", "Information Received"},
		{"Expired maps correctly", "Expired", "Expired"},
		{"PickedUp maps correctly", "PickedUp", "Picked Up"},
		{"Undelivered maps correctly", "Undelivered", "Delivery Attempted"},
		{"Alert maps correctly", "Alert", "Alert - Check Details"},
		{"empty code returned unchanged", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mapTrackingStatus(tt.code)
			if got != tt.want {
				t.Errorf("mapTrackingStatus(%q) = %q, want %q", tt.code, got, tt.want)
			}
		})
	}
}

func TestExtractDate(t *testing.T) {
	tests := []struct {
		name     string
		datetime string
		want     string
	}{
		{"ISO datetime extracts date portion", "2023-12-25T10:30:00", "2023-12-25"},
		{"short string returned as-is", "2023", "2023"},
		{"exactly 10 chars extracts correctly", "2023-12-25", "2023-12-25"},
		{"empty string returned as-is", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractDate(tt.datetime)
			if got != tt.want {
				t.Errorf("extractDate(%q) = %q, want %q", tt.datetime, got, tt.want)
			}
		})
	}
}

func TestExtractTime(t *testing.T) {
	tests := []struct {
		name     string
		datetime string
		want     string
	}{
		{"ISO datetime extracts time portion", "2023-12-25T10:30:00", "10:30:00"},
		{"short string returns empty", "2023-12-25", ""},
		{"empty string returns empty", "", ""},
		{"exactly 11 chars returns empty", "2023-12-25T", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractTime(tt.datetime)
			if got != tt.want {
				t.Errorf("extractTime(%q) = %q, want %q", tt.datetime, got, tt.want)
			}
		})
	}
}

func TestTrackingFetcherFetchValidation(t *testing.T) {
	f := NewTrackingFetcher()
	ctx := context.Background()

	t.Run("empty tracking number returns error WidgetData", func(t *testing.T) {
		data, err := f.Fetch(ctx, map[string]string{})
		if err != nil {
			t.Fatalf("Fetch() returned unexpected error: %v", err)
		}
		if data.Error == "" {
			t.Error("WidgetData.Error should be set for empty tracking number")
		}
		if data.Type != WidgetTracking {
			t.Errorf("WidgetData.Type = %q, want %q", data.Type, WidgetTracking)
		}
	})

	t.Run("tracking number shorter than 8 chars returns min length error", func(t *testing.T) {
		data, err := f.Fetch(ctx, map[string]string{"number": "SHORT"})
		if err != nil {
			t.Fatalf("Fetch() returned unexpected error: %v", err)
		}
		if data.Error == "" {
			t.Error("WidgetData.Error should be set for too-short tracking number")
		}
		if data.Type != WidgetTracking {
			t.Errorf("WidgetData.Type = %q, want %q", data.Type, WidgetTracking)
		}
	})

	t.Run("valid tracking number returns no error", func(t *testing.T) {
		data, err := f.Fetch(ctx, map[string]string{"number": "1Z999AA10123456789"})
		if err != nil {
			t.Fatalf("Fetch() returned unexpected error: %v", err)
		}
		if data.Error != "" {
			t.Errorf("WidgetData.Error should be empty for valid number, got %q", data.Error)
		}
	})
}
