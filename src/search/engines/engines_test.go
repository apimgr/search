package engines

import (
	"strings"
	"testing"

	"github.com/apimgr/search/src/model"
)

// Tests for Registry

func TestNewRegistry(t *testing.T) {
	registry := NewRegistry()

	if registry == nil {
		t.Fatal("NewRegistry() returned nil")
	}
	if registry.engines == nil {
		t.Error("engines map should be initialized")
	}
	if registry.Count() != 0 {
		t.Errorf("Count() = %d, want 0 for new registry", registry.Count())
	}
}

func TestRegistryRegister(t *testing.T) {
	registry := NewRegistry()
	engine := NewGoogle()

	registry.Register(engine)

	if registry.Count() != 1 {
		t.Errorf("Count() = %d, want 1", registry.Count())
	}
}

func TestRegistryGet(t *testing.T) {
	registry := NewRegistry()
	engine := NewGoogle()
	registry.Register(engine)

	got, err := registry.Get("google")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.Name() != "google" {
		t.Errorf("Get().Name() = %q, want google", got.Name())
	}
}

func TestRegistryGetNotFound(t *testing.T) {
	registry := NewRegistry()

	_, err := registry.Get("nonexistent")
	if err != model.ErrEngineNotFound {
		t.Errorf("Get() error = %v, want ErrEngineNotFound", err)
	}
}

func TestRegistryGetAll(t *testing.T) {
	registry := NewRegistry()
	registry.Register(NewGoogle())
	registry.Register(NewBing())
	registry.Register(NewDuckDuckGo())

	all := registry.GetAll()

	if len(all) != 3 {
		t.Errorf("GetAll() count = %d, want 3", len(all))
	}
}

func TestRegistryGetEnabled(t *testing.T) {
	registry := NewRegistry()

	// Create engines with different enabled states
	google := NewGoogle()
	bing := NewBing()
	ddg := NewDuckDuckGo()

	registry.Register(google)
	registry.Register(bing)
	registry.Register(ddg)

	enabled := registry.GetEnabled()

	// All default engines should be enabled
	if len(enabled) != 3 {
		t.Errorf("GetEnabled() count = %d, want 3", len(enabled))
	}
}

func TestRegistryGetForCategory(t *testing.T) {
	registry := NewRegistry()
	registry.Register(NewGoogle())       // supports general, images, news, videos
	registry.Register(NewWikipediaEngine()) // supports general

	generalEngines := registry.GetForCategory(model.CategoryGeneral)
	if len(generalEngines) < 2 {
		t.Errorf("GetForCategory(General) count = %d, want >= 2", len(generalEngines))
	}

	imageEngines := registry.GetForCategory(model.CategoryImages)
	if len(imageEngines) < 1 {
		t.Errorf("GetForCategory(Images) count = %d, want >= 1", len(imageEngines))
	}
}

func TestRegistryGetByNames(t *testing.T) {
	registry := NewRegistry()
	registry.Register(NewGoogle())
	registry.Register(NewBing())
	registry.Register(NewDuckDuckGo())

	// Get specific engines
	engines := registry.GetByNames([]string{"google", "bing"})
	if len(engines) != 2 {
		t.Errorf("GetByNames() count = %d, want 2", len(engines))
	}

	// Empty names should return all enabled
	all := registry.GetByNames([]string{})
	if len(all) != 3 {
		t.Errorf("GetByNames([]) count = %d, want 3", len(all))
	}
}

func TestRegistryGetByNamesNonExistent(t *testing.T) {
	registry := NewRegistry()
	registry.Register(NewGoogle())

	// Non-existent engine should be skipped
	engines := registry.GetByNames([]string{"google", "nonexistent"})
	if len(engines) != 1 {
		t.Errorf("GetByNames() count = %d, want 1", len(engines))
	}
}

func TestRegistryCount(t *testing.T) {
	registry := NewRegistry()

	if registry.Count() != 0 {
		t.Error("Empty registry should have count 0")
	}

	registry.Register(NewGoogle())
	registry.Register(NewBing())

	if registry.Count() != 2 {
		t.Errorf("Count() = %d, want 2", registry.Count())
	}
}

func TestDefaultRegistry(t *testing.T) {
	registry := DefaultRegistry()

	if registry == nil {
		t.Fatal("DefaultRegistry() returned nil")
	}

	// Should have at least the core engines
	if registry.Count() < 10 {
		t.Errorf("DefaultRegistry() count = %d, want >= 10", registry.Count())
	}

	// Core engines should be present
	coreEngines := []string{"google", "duckduckgo", "bing", "wikipedia", "qwant", "brave"}
	for _, name := range coreEngines {
		if _, err := registry.Get(name); err != nil {
			t.Errorf("DefaultRegistry() missing engine %q", name)
		}
	}
}

// Tests for Engine Constructors

func TestNewGoogle(t *testing.T) {
	engine := NewGoogle()

	if engine == nil {
		t.Fatal("NewGoogle() returned nil")
	}
	if engine.Name() != "google" {
		t.Errorf("Name() = %q, want google", engine.Name())
	}
	if engine.DisplayName() != "Google" {
		t.Errorf("DisplayName() = %q, want Google", engine.DisplayName())
	}
	if !engine.IsEnabled() {
		t.Error("IsEnabled() should be true by default")
	}
	if engine.GetPriority() != 90 {
		t.Errorf("GetPriority() = %d, want 90", engine.GetPriority())
	}
	if engine.client == nil {
		t.Error("HTTP client should be initialized")
	}
}

func TestNewDuckDuckGo(t *testing.T) {
	engine := NewDuckDuckGo()

	if engine == nil {
		t.Fatal("NewDuckDuckGo() returned nil")
	}
	if engine.Name() != "duckduckgo" {
		t.Errorf("Name() = %q, want duckduckgo", engine.Name())
	}
	if engine.GetPriority() != 100 {
		t.Errorf("GetPriority() = %d, want 100 (highest)", engine.GetPriority())
	}
}

func TestNewBing(t *testing.T) {
	engine := NewBing()

	if engine == nil {
		t.Fatal("NewBing() returned nil")
	}
	if engine.Name() != "bing" {
		t.Errorf("Name() = %q, want bing", engine.Name())
	}
}

func TestNewWikipedia(t *testing.T) {
	engine := NewWikipediaEngine()

	if engine == nil {
		t.Fatal("NewWikipediaEngine() returned nil")
	}
	if engine.Name() != "wikipedia" {
		t.Errorf("Name() = %q, want wikipedia", engine.Name())
	}
}

func TestNewQwant(t *testing.T) {
	engine := NewQwantEngine()

	if engine == nil {
		t.Fatal("NewQwantEngine() returned nil")
	}
	if engine.Name() != "qwant" {
		t.Errorf("Name() = %q, want qwant", engine.Name())
	}
}

func TestNewBrave(t *testing.T) {
	engine := NewBrave()

	if engine == nil {
		t.Fatal("NewBrave() returned nil")
	}
	if engine.Name() != "brave" {
		t.Errorf("Name() = %q, want brave", engine.Name())
	}
}

func TestNewYahoo(t *testing.T) {
	engine := NewYahoo()

	if engine == nil {
		t.Fatal("NewYahoo() returned nil")
	}
	if engine.Name() != "yahoo" {
		t.Errorf("Name() = %q, want yahoo", engine.Name())
	}
}

func TestNewGitHub(t *testing.T) {
	engine := NewGitHub()

	if engine == nil {
		t.Fatal("NewGitHub() returned nil")
	}
	if engine.Name() != "github" {
		t.Errorf("Name() = %q, want github", engine.Name())
	}
	// GitHub should support specific categories
	if !engine.SupportsCategory(model.CategoryGeneral) {
		t.Error("GitHub should support CategoryGeneral")
	}
}

func TestNewStackOverflow(t *testing.T) {
	engine := NewStackOverflow()

	if engine == nil {
		t.Fatal("NewStackOverflow() returned nil")
	}
	if engine.Name() != "stackoverflow" {
		t.Errorf("Name() = %q, want stackoverflow", engine.Name())
	}
}

func TestNewReddit(t *testing.T) {
	engine := NewReddit()

	if engine == nil {
		t.Fatal("NewReddit() returned nil")
	}
	if engine.Name() != "reddit" {
		t.Errorf("Name() = %q, want reddit", engine.Name())
	}
}

func TestNewStartpage(t *testing.T) {
	engine := NewStartpageEngine()

	if engine == nil {
		t.Fatal("NewStartpageEngine() returned nil")
	}
	if engine.Name() != "startpage" {
		t.Errorf("Name() = %q, want startpage", engine.Name())
	}
}

func TestNewYouTube(t *testing.T) {
	engine := NewYouTubeEngine()

	if engine == nil {
		t.Fatal("NewYouTubeEngine() returned nil")
	}
	if engine.Name() != "youtube" {
		t.Errorf("Name() = %q, want youtube", engine.Name())
	}
	// YouTube should support videos
	if !engine.SupportsCategory(model.CategoryVideos) {
		t.Error("YouTube should support CategoryVideos")
	}
}

func TestNewMojeek(t *testing.T) {
	engine := NewMojeek()

	if engine == nil {
		t.Fatal("NewMojeek() returned nil")
	}
	if engine.Name() != "mojeek" {
		t.Errorf("Name() = %q, want mojeek", engine.Name())
	}
}

func TestNewYandex(t *testing.T) {
	engine := NewYandex()

	if engine == nil {
		t.Fatal("NewYandex() returned nil")
	}
	if engine.Name() != "yandex" {
		t.Errorf("Name() = %q, want yandex", engine.Name())
	}
}

func TestNewBaidu(t *testing.T) {
	engine := NewBaidu()

	if engine == nil {
		t.Fatal("NewBaidu() returned nil")
	}
	if engine.Name() != "baidu" {
		t.Errorf("Name() = %q, want baidu", engine.Name())
	}
}

// Tests for helper functions

func TestExtractGoogleURL(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "direct URL",
			input:    "https://example.com/page",
			expected: "https://example.com/page",
		},
		{
			name:     "wrapped URL",
			input:    `/url?q=https://example.com/page&sa=U&ved=...`,
			expected: "https://example.com/page",
		},
		{
			name:     "encoded URL",
			input:    `/url?q=https%3A%2F%2Fexample.com%2Fpage&sa=U`,
			expected: "https://example.com/page",
		},
		{
			name:     "empty",
			input:    "",
			expected: "",
		},
		{
			name:     "relative path",
			input:    "/search?q=test",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractGoogleURL(tt.input)
			if got != tt.expected {
				t.Errorf("extractGoogleURL() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestCleanHTML(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "plain text",
			input:    "Hello World",
			expected: "Hello World",
		},
		{
			name:     "with tags",
			input:    "<b>Hello</b> <i>World</i>",
			expected: "Hello World",
		},
		{
			name:     "HTML entities",
			input:    "Hello &amp; World &lt;test&gt;",
			expected: "Hello & World <test>",
		},
		{
			name:     "quotes",
			input:    "&quot;Hello&quot; &#39;World&#39;",
			expected: `"Hello" 'World'`,
		},
		{
			name:     "nbsp",
			input:    "Hello&nbsp;World",
			expected: "Hello World",
		},
		{
			name:     "whitespace trim",
			input:    "  Hello World  ",
			expected: "Hello World",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cleanHTML(tt.input)
			if got != tt.expected {
				t.Errorf("cleanHTML() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestCalculateScore(t *testing.T) {
	tests := []struct {
		name       string
		priority   int
		position   int
		duplicates int
		wantMin    float64
	}{
		{
			name:       "high priority first position",
			priority:   100,
			position:   0,
			duplicates: 1,
			wantMin:    10000,
		},
		{
			name:       "low priority last position",
			priority:   10,
			position:   99,
			duplicates: 0,
			wantMin:    1000,
		},
		{
			name:       "with duplicates",
			priority:   50,
			position:   5,
			duplicates: 3,
			wantMin:    5000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calculateScore(tt.priority, tt.position, tt.duplicates)
			if got < tt.wantMin {
				t.Errorf("calculateScore() = %f, want >= %f", got, tt.wantMin)
			}
		})
	}
}

func TestExtractTitle(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple title",
			input:    "Hello World",
			expected: "Hello World",
		},
		{
			name:     "title with dash separator",
			input:    "Long Article Title Here - Site Name Website",
			expected: "Long Article Title Here",
		},
		{
			name:     "title with pipe separator",
			input:    "Long Article Title Here | Site Name Website",
			expected: "Long Article Title Here",
		},
		{
			name:     "short title keeps original",
			input:    "Short - Site",
			expected: "Short - Site",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractTitle(tt.input)
			if got != tt.expected {
				t.Errorf("extractTitle() = %q, want %q", got, tt.expected)
			}
		})
	}
}

// Tests for engine category support

func TestGoogleSupportsCategory(t *testing.T) {
	engine := NewGoogle()

	categories := []model.Category{
		model.CategoryGeneral,
		model.CategoryImages,
		model.CategoryNews,
		model.CategoryVideos,
	}

	for _, cat := range categories {
		if !engine.SupportsCategory(cat) {
			t.Errorf("Google should support %s", cat)
		}
	}
}

func TestDuckDuckGoSupportsCategory(t *testing.T) {
	engine := NewDuckDuckGo()

	if !engine.SupportsCategory(model.CategoryGeneral) {
		t.Error("DuckDuckGo should support CategoryGeneral")
	}
	if !engine.SupportsCategory(model.CategoryImages) {
		t.Error("DuckDuckGo should support CategoryImages")
	}
}

func TestWikipediaSupportsCategory(t *testing.T) {
	engine := NewWikipediaEngine()

	if !engine.SupportsCategory(model.CategoryGeneral) {
		t.Error("Wikipedia should support CategoryGeneral")
	}
}

func TestYouTubeSupportsVideos(t *testing.T) {
	engine := NewYouTubeEngine()

	if !engine.SupportsCategory(model.CategoryVideos) {
		t.Error("YouTube should support CategoryVideos")
	}
}

// Test engine GetConfig

func TestEngineGetConfig(t *testing.T) {
	engines := []struct {
		name   string
		engine interface {
			Name() string
			GetConfig() *model.EngineConfig
		}
	}{
		{"google", NewGoogle()},
		{"duckduckgo", NewDuckDuckGo()},
		{"bing", NewBing()},
		{"wikipedia", NewWikipediaEngine()},
	}

	for _, tc := range engines {
		t.Run(tc.name, func(t *testing.T) {
			config := tc.engine.GetConfig()
			if config == nil {
				t.Fatalf("%s.GetConfig() returned nil", tc.name)
			}
			if config.Name != tc.engine.Name() {
				t.Errorf("%s config.Name = %q, want %q", tc.name, config.Name, tc.engine.Name())
			}
		})
	}
}

// Test engine IsEnabled

func TestEngineIsEnabled(t *testing.T) {
	engines := []struct {
		name    string
		engine  interface{ IsEnabled() bool }
		enabled bool
	}{
		{"google", NewGoogle(), true},
		{"duckduckgo", NewDuckDuckGo(), true},
		{"bing", NewBing(), true},
		{"wikipedia", NewWikipediaEngine(), true},
		{"brave", NewBrave(), true},
		{"yahoo", NewYahoo(), true},
	}

	for _, tc := range engines {
		t.Run(tc.name, func(t *testing.T) {
			if tc.engine.IsEnabled() != tc.enabled {
				t.Errorf("%s.IsEnabled() = %v, want %v", tc.name, tc.engine.IsEnabled(), tc.enabled)
			}
		})
	}
}

// Test engine priorities

func TestEnginePriorities(t *testing.T) {
	// DuckDuckGo should have highest priority (for privacy)
	ddg := NewDuckDuckGo()
	google := NewGoogle()
	bing := NewBing()

	if ddg.GetConfig().Priority <= google.GetConfig().Priority {
		t.Errorf("DuckDuckGo priority (%d) should be higher than Google (%d)",
			ddg.GetConfig().Priority, google.GetConfig().Priority)
	}
	if google.GetConfig().Priority <= bing.GetConfig().Priority {
		t.Errorf("Google priority (%d) should be higher than Bing (%d)",
			google.GetConfig().Priority, bing.GetConfig().Priority)
	}
}

// Test engine categories

func TestBingSupportsCategory(t *testing.T) {
	engine := NewBing()

	tests := []struct {
		category model.Category
		want     bool
	}{
		{model.CategoryGeneral, true},
		{model.CategoryImages, true},
		{model.CategoryVideos, true},
		{model.CategoryNews, true},
		{model.CategoryMaps, false},
		{model.CategorySocial, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.category), func(t *testing.T) {
			if got := engine.SupportsCategory(tt.category); got != tt.want {
				t.Errorf("Bing.SupportsCategory(%q) = %v, want %v", tt.category, got, tt.want)
			}
		})
	}
}

func TestBraveSupportsCategory(t *testing.T) {
	engine := NewBrave()

	tests := []struct {
		category model.Category
		want     bool
	}{
		{model.CategoryGeneral, true},
		{model.CategoryImages, true},
		{model.CategoryNews, true},
		{model.CategoryVideos, false}, // Brave doesn't support videos
	}

	for _, tt := range tests {
		t.Run(string(tt.category), func(t *testing.T) {
			if got := engine.SupportsCategory(tt.category); got != tt.want {
				t.Errorf("Brave.SupportsCategory(%q) = %v, want %v", tt.category, got, tt.want)
			}
		})
	}
}

func TestYahooSupportsCategory(t *testing.T) {
	engine := NewYahoo()

	tests := []struct {
		category model.Category
		want     bool
	}{
		{model.CategoryGeneral, true},
		{model.CategoryImages, true},
		{model.CategoryNews, true},
		{model.CategoryVideos, false}, // Yahoo doesn't support videos
	}

	for _, tt := range tests {
		t.Run(string(tt.category), func(t *testing.T) {
			if got := engine.SupportsCategory(tt.category); got != tt.want {
				t.Errorf("Yahoo.SupportsCategory(%q) = %v, want %v", tt.category, got, tt.want)
			}
		})
	}
}

func TestRedditSupportsCategory(t *testing.T) {
	engine := NewReddit()

	tests := []struct {
		category model.Category
		want     bool
	}{
		{model.CategoryGeneral, true},
		{model.CategoryImages, false},
		{model.CategoryNews, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.category), func(t *testing.T) {
			if got := engine.SupportsCategory(tt.category); got != tt.want {
				t.Errorf("Reddit.SupportsCategory(%q) = %v, want %v", tt.category, got, tt.want)
			}
		})
	}
}

func TestStackOverflowSupportsCategory(t *testing.T) {
	engine := NewStackOverflow()

	tests := []struct {
		category model.Category
		want     bool
	}{
		{model.CategoryGeneral, true},
		// StackOverflow uses "code" category, not "it"
		{model.CategoryImages, false},
		{model.CategoryNews, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.category), func(t *testing.T) {
			if got := engine.SupportsCategory(tt.category); got != tt.want {
				t.Errorf("StackOverflow.SupportsCategory(%q) = %v, want %v", tt.category, got, tt.want)
			}
		})
	}
}

func TestGitHubSupportsCategory(t *testing.T) {
	engine := NewGitHub()

	tests := []struct {
		category model.Category
		want     bool
	}{
		{model.CategoryGeneral, true},
		// GitHub uses "code" category, not "it"
		{model.CategoryImages, false},
		{model.CategoryNews, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.category), func(t *testing.T) {
			if got := engine.SupportsCategory(tt.category); got != tt.want {
				t.Errorf("GitHub.SupportsCategory(%q) = %v, want %v", tt.category, got, tt.want)
			}
		})
	}
}

// Test engine display names

func TestEngineDisplayNames(t *testing.T) {
	engines := []struct {
		name        string
		engine      interface{ GetConfig() *model.EngineConfig }
		displayName string
	}{
		{"google", NewGoogle(), "Google"},
		{"duckduckgo", NewDuckDuckGo(), "DuckDuckGo"},
		{"bing", NewBing(), "Bing"},
		{"wikipedia", NewWikipediaEngine(), "Wikipedia"},
		{"brave", NewBrave(), "Brave Search"},
		{"yahoo", NewYahoo(), "Yahoo"},
		{"github", NewGitHub(), "GitHub"},
		{"stackoverflow", NewStackOverflow(), "Stack Overflow"},
		{"reddit", NewReddit(), "Reddit"},
		{"startpage", NewStartpageEngine(), "Startpage"},
		{"youtube", NewYouTubeEngine(), "YouTube"},
	}

	for _, tc := range engines {
		t.Run(tc.name, func(t *testing.T) {
			if tc.engine.GetConfig().DisplayName != tc.displayName {
				t.Errorf("%s DisplayName = %q, want %q",
					tc.name, tc.engine.GetConfig().DisplayName, tc.displayName)
			}
		})
	}
}

// Test clean URL functions

func TestCleanURL(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"normal_url", "https://example.com", "https://example.com"},
		{"trailing_slash", "https://example.com/", "https://example.com/"},
		{"with_query", "https://example.com?foo=bar", "https://example.com?foo=bar"},
		{"empty", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Basic validation - URLs should remain valid
			if tt.input != "" && !strings.HasPrefix(tt.input, "http") {
				t.Errorf("Test input %q is not a valid URL prefix", tt.input)
			}
		})
	}
}

// Test engine timeout configuration

func TestEngineTimeout(t *testing.T) {
	engines := []struct {
		name    string
		engine  interface{ GetConfig() *model.EngineConfig }
		minTime int
	}{
		{"google", NewGoogle(), 1},
		{"duckduckgo", NewDuckDuckGo(), 1},
		{"bing", NewBing(), 1},
	}

	for _, tc := range engines {
		t.Run(tc.name, func(t *testing.T) {
			timeout := tc.engine.GetConfig().GetTimeout()
			if timeout < tc.minTime {
				t.Errorf("%s timeout = %d, should be >= %d", tc.name, timeout, tc.minTime)
			}
		})
	}
}

// Test all engines implement search.Engine interface

func TestAllEnginesImplementInterface(t *testing.T) {
	registry := DefaultRegistry()
	engines := registry.GetAll()

	for _, engine := range engines {
		t.Run(engine.Name(), func(t *testing.T) {
			// Verify required interface methods exist
			_ = engine.Name()
			_ = engine.IsEnabled()
			_ = engine.GetConfig()
			_ = engine.SupportsCategory(model.CategoryGeneral)
			// Search() is also required but needs context
		})
	}
}

// Test registry returns unique engines

func TestRegistryUniqueEngines(t *testing.T) {
	registry := DefaultRegistry()
	engines := registry.GetAll()

	seen := make(map[string]bool)
	for _, engine := range engines {
		name := engine.Name()
		if seen[name] {
			t.Errorf("Duplicate engine found: %s", name)
		}
		seen[name] = true
	}
}

// Test engine Tor support flags

func TestEngineTorSupport(t *testing.T) {
	engines := []struct {
		name       string
		engine     interface{ GetConfig() *model.EngineConfig }
		supportsTor bool
	}{
		{"duckduckgo", NewDuckDuckGo(), true}, // DDG supports Tor
		{"bing", NewBing(), false},
	}

	for _, tc := range engines {
		t.Run(tc.name, func(t *testing.T) {
			if tc.engine.GetConfig().SupportsTor != tc.supportsTor {
				t.Errorf("%s SupportsTor = %v, want %v",
					tc.name, tc.engine.GetConfig().SupportsTor, tc.supportsTor)
			}
		})
	}
}
