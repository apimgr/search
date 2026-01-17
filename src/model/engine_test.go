package model

import "testing"

func TestNewEngineConfig(t *testing.T) {
	ec := NewEngineConfig("google")

	if ec.Name != "google" {
		t.Errorf("Name = %q, want %q", ec.Name, "google")
	}
	if ec.DisplayName != "google" {
		t.Errorf("DisplayName = %q, want %q", ec.DisplayName, "google")
	}
	if !ec.Enabled {
		t.Error("Enabled should be true by default")
	}
	if ec.Priority != 50 {
		t.Errorf("Priority = %d, want %d", ec.Priority, 50)
	}
	if len(ec.Categories) != 1 || ec.Categories[0] != "general" {
		t.Errorf("Categories = %v, want [general]", ec.Categories)
	}
	if ec.Language != "en" {
		t.Errorf("Language = %q, want %q", ec.Language, "en")
	}
	if ec.Timeout != 10 {
		t.Errorf("Timeout = %d, want %d", ec.Timeout, 10)
	}
	if ec.MaxResults != 100 {
		t.Errorf("MaxResults = %d, want %d", ec.MaxResults, 100)
	}
	if ec.SupportsTor {
		t.Error("SupportsTor should be false by default")
	}
	if ec.UseTor {
		t.Error("UseTor should be false by default")
	}
	if ec.Settings == nil {
		t.Error("Settings should not be nil")
	}
}

func TestEngineConfigIsEnabled(t *testing.T) {
	ec := NewEngineConfig("test")
	if !ec.IsEnabled() {
		t.Error("IsEnabled() should return true for enabled engine")
	}

	ec.Enabled = false
	if ec.IsEnabled() {
		t.Error("IsEnabled() should return false for disabled engine")
	}
}

func TestEngineConfigSupportsCategory(t *testing.T) {
	ec := NewEngineConfig("test")
	ec.Categories = []string{"general", "news"}

	tests := []struct {
		category Category
		want     bool
	}{
		{CategoryGeneral, true},
		{CategoryNews, true},
		{CategoryImages, false},
		{CategoryVideos, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.category), func(t *testing.T) {
			got := ec.SupportsCategory(tt.category)
			if got != tt.want {
				t.Errorf("SupportsCategory(%q) = %v, want %v", tt.category, got, tt.want)
			}
		})
	}
}

func TestEngineConfigSupportsAllCategory(t *testing.T) {
	ec := NewEngineConfig("test")
	ec.Categories = []string{"all"}

	// When categories contains "all", it should support any category
	for _, cat := range AllCategories() {
		if !ec.SupportsCategory(cat) {
			t.Errorf("SupportsCategory(%q) = false for engine with 'all' category", cat)
		}
	}
}

func TestEngineConfigGetTimeout(t *testing.T) {
	tests := []struct {
		name    string
		timeout int
		want    int
	}{
		{"positive", 30, 30},
		{"zero", 0, 10},
		{"negative", -5, 10},
		{"default from constructor", 10, 10},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ec := NewEngineConfig("test")
			ec.Timeout = tt.timeout
			got := ec.GetTimeout()
			if got != tt.want {
				t.Errorf("GetTimeout() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestEngineConfigGetMaxResults(t *testing.T) {
	tests := []struct {
		name       string
		maxResults int
		want       int
	}{
		{"positive", 50, 50},
		{"zero", 0, 100},
		{"negative", -10, 100},
		{"default from constructor", 100, 100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ec := NewEngineConfig("test")
			ec.MaxResults = tt.maxResults
			got := ec.GetMaxResults()
			if got != tt.want {
				t.Errorf("GetMaxResults() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestEngineConfigGetPriority(t *testing.T) {
	ec := NewEngineConfig("test")
	if ec.GetPriority() != 50 {
		t.Errorf("GetPriority() = %d, want %d", ec.GetPriority(), 50)
	}

	ec.Priority = 100
	if ec.GetPriority() != 100 {
		t.Errorf("GetPriority() = %d, want %d", ec.GetPriority(), 100)
	}

	ec.Priority = 1
	if ec.GetPriority() != 1 {
		t.Errorf("GetPriority() = %d, want %d", ec.GetPriority(), 1)
	}
}

func TestEngineConfigStruct(t *testing.T) {
	ec := &EngineConfig{
		Name:        "test-engine",
		DisplayName: "Test Engine",
		Enabled:     true,
		Priority:    75,
		Categories:  []string{"general", "news", "images"},
		Language:    "de",
		Timeout:     15,
		MaxResults:  200,
		SupportsTor: true,
		UseTor:      true,
		Settings: map[string]interface{}{
			"api_key": "secret",
		},
	}

	ec.RateLimit.Requests = 10
	ec.RateLimit.Window = 60

	if ec.Name != "test-engine" {
		t.Errorf("Name = %q, want %q", ec.Name, "test-engine")
	}
	if ec.DisplayName != "Test Engine" {
		t.Errorf("DisplayName = %q, want %q", ec.DisplayName, "Test Engine")
	}
	if !ec.Enabled {
		t.Error("Enabled should be true")
	}
	if ec.Priority != 75 {
		t.Errorf("Priority = %d, want %d", ec.Priority, 75)
	}
	if len(ec.Categories) != 3 {
		t.Errorf("Categories length = %d, want %d", len(ec.Categories), 3)
	}
	if ec.Language != "de" {
		t.Errorf("Language = %q, want %q", ec.Language, "de")
	}
	if ec.Timeout != 15 {
		t.Errorf("Timeout = %d, want %d", ec.Timeout, 15)
	}
	if ec.MaxResults != 200 {
		t.Errorf("MaxResults = %d, want %d", ec.MaxResults, 200)
	}
	if !ec.SupportsTor {
		t.Error("SupportsTor should be true")
	}
	if !ec.UseTor {
		t.Error("UseTor should be true")
	}
	if ec.RateLimit.Requests != 10 {
		t.Errorf("RateLimit.Requests = %d, want %d", ec.RateLimit.Requests, 10)
	}
	if ec.RateLimit.Window != 60 {
		t.Errorf("RateLimit.Window = %d, want %d", ec.RateLimit.Window, 60)
	}
}
