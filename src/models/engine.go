package models

// EngineConfig represents configuration for a search engine
type EngineConfig struct {
	Name        string   `yaml:"name" json:"name"`
	DisplayName string   `yaml:"display_name" json:"display_name"`
	Enabled     bool     `yaml:"enabled" json:"enabled"`
	Priority    int      `yaml:"priority" json:"priority"`
	Categories  []string `yaml:"categories" json:"categories"`
	Language    string   `yaml:"language" json:"language"`
	Timeout     int      `yaml:"timeout" json:"timeout"` // seconds
	MaxResults  int      `yaml:"max_results" json:"max_results"`
	
	// Tor support
	SupportsTor bool `yaml:"supports_tor" json:"supports_tor"`
	UseTor      bool `yaml:"use_tor" json:"use_tor"`
	
	// Rate limiting
	RateLimit struct {
		Requests int `yaml:"requests" json:"requests"`
		Window   int `yaml:"window" json:"window"` // seconds
	} `yaml:"rate_limit" json:"rate_limit"`
	
	// Engine-specific settings
	Settings map[string]interface{} `yaml:"settings,omitempty" json:"settings,omitempty"`
}

// NewEngineConfig creates a new EngineConfig with defaults
func NewEngineConfig(name string) *EngineConfig {
	return &EngineConfig{
		Name:        name,
		DisplayName: name,
		Enabled:     true,
		Priority:    50,
		Categories:  []string{"general"},
		Language:    "en",
		Timeout:     10,
		MaxResults:  100,
		SupportsTor: false,
		UseTor:      false,
		Settings:    make(map[string]interface{}),
	}
}

// IsEnabled checks if the engine is enabled
func (ec *EngineConfig) IsEnabled() bool {
	return ec.Enabled
}

// SupportsCategory checks if the engine supports a category
func (ec *EngineConfig) SupportsCategory(category Category) bool {
	for _, cat := range ec.Categories {
		if cat == category.String() || cat == "all" {
			return true
		}
	}
	return false
}

// GetTimeout returns the timeout in seconds
func (ec *EngineConfig) GetTimeout() int {
	if ec.Timeout <= 0 {
		return 10
	}
	return ec.Timeout
}

// GetMaxResults returns the maximum number of results
func (ec *EngineConfig) GetMaxResults() int {
	if ec.MaxResults <= 0 {
		return 100
	}
	return ec.MaxResults
}

// GetPriority returns the engine priority
func (ec *EngineConfig) GetPriority() int {
	return ec.Priority
}
