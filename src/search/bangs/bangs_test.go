package bangs

import (
	"strings"
	"testing"
)

// Tests for Bang struct

func TestBangStruct(t *testing.T) {
	bang := &Bang{
		Shortcut:    "g",
		Name:        "Google",
		URL:         "https://www.google.com/search?q={query}",
		Category:    "search",
		Description: "Search Google",
		Icon:        "google.png",
		Aliases:     []string{"google", "goog"},
	}

	if bang.Shortcut != "g" {
		t.Errorf("Shortcut = %q", bang.Shortcut)
	}
	if bang.Name != "Google" {
		t.Errorf("Name = %q", bang.Name)
	}
	if bang.Category != "search" {
		t.Errorf("Category = %q", bang.Category)
	}
	if len(bang.Aliases) != 2 {
		t.Errorf("Aliases length = %d", len(bang.Aliases))
	}
}

// Tests for BangResult struct

func TestBangResultStruct(t *testing.T) {
	bang := &Bang{Shortcut: "g", Name: "Google"}
	result := &BangResult{
		Bang:       bang,
		Query:      "test query",
		TargetURL:  "https://google.com?q=test+query",
		IsBangOnly: false,
	}

	if result.Bang != bang {
		t.Error("Bang not set correctly")
	}
	if result.Query != "test query" {
		t.Errorf("Query = %q", result.Query)
	}
	if result.IsBangOnly {
		t.Error("IsBangOnly should be false")
	}
}

// Tests for Manager

func TestNewManager(t *testing.T) {
	m := NewManager()

	if m == nil {
		t.Fatal("NewManager() returned nil")
	}
	if m.builtins == nil {
		t.Error("builtins should be initialized")
	}
	if m.custom == nil {
		t.Error("custom should be initialized")
	}
	if m.user == nil {
		t.Error("user should be initialized")
	}
	// Should have built-in bangs loaded
	if len(m.builtins) == 0 {
		t.Error("Should have built-in bangs loaded")
	}
}

func TestManagerSetCustomBangs(t *testing.T) {
	m := NewManager()
	customBangs := []*Bang{
		{Shortcut: "custom1", Name: "Custom One", URL: "https://custom1.com?q={query}"},
		{Shortcut: "custom2", Name: "Custom Two", URL: "https://custom2.com?q={query}", Aliases: []string{"c2"}},
	}

	m.SetCustomBangs(customBangs)

	if len(m.custom) != 3 { // 2 bangs + 1 alias
		t.Errorf("custom map size = %d, want 3", len(m.custom))
	}

	// Verify custom bang can be found
	result := m.Parse("!custom1 test")
	if result == nil {
		t.Fatal("Should find custom bang")
	}
	if result.Bang.Shortcut != "custom1" {
		t.Errorf("Bang.Shortcut = %q", result.Bang.Shortcut)
	}

	// Verify alias works
	result = m.Parse("!c2 test")
	if result == nil {
		t.Fatal("Should find custom bang by alias")
	}
	if result.Bang.Shortcut != "custom2" {
		t.Errorf("Bang.Shortcut = %q, want custom2", result.Bang.Shortcut)
	}
}

func TestManagerSetUserBangs(t *testing.T) {
	m := NewManager()
	userBangs := []*Bang{
		{Shortcut: "mysite", Name: "My Site", URL: "https://mysite.com?q={query}"},
	}

	m.SetUserBangs(userBangs)

	result := m.Parse("!mysite test")
	if result == nil {
		t.Fatal("Should find user bang")
	}
	if result.Bang.Shortcut != "mysite" {
		t.Errorf("Bang.Shortcut = %q", result.Bang.Shortcut)
	}
}

func TestManagerUserBangOverridesBuiltin(t *testing.T) {
	m := NewManager()

	// Set user bang with same shortcut as builtin
	userBangs := []*Bang{
		{Shortcut: "g", Name: "My Google", URL: "https://mygoogle.com?q={query}"},
	}
	m.SetUserBangs(userBangs)

	result := m.Parse("!g test")
	if result == nil {
		t.Fatal("Should find bang")
	}
	if result.Bang.Name != "My Google" {
		t.Errorf("User bang should override builtin, got Name = %q", result.Bang.Name)
	}
}

// Tests for Parse

func TestManagerParseEmpty(t *testing.T) {
	m := NewManager()

	result := m.Parse("")
	if result != nil {
		t.Error("Parse('') should return nil")
	}

	result = m.Parse("   ")
	if result != nil {
		t.Error("Parse('   ') should return nil")
	}
}

func TestManagerParseNoBang(t *testing.T) {
	m := NewManager()

	result := m.Parse("regular search query")
	if result != nil {
		t.Error("Parse() with no bang should return nil")
	}
}

func TestManagerParsePrefixBang(t *testing.T) {
	m := NewManager()

	result := m.Parse("!g golang tutorial")
	if result == nil {
		t.Fatal("Should find Google bang")
	}
	if result.Bang.Shortcut != "g" {
		t.Errorf("Bang.Shortcut = %q", result.Bang.Shortcut)
	}
	if result.Query != "golang tutorial" {
		t.Errorf("Query = %q, want 'golang tutorial'", result.Query)
	}
	if result.IsBangOnly {
		t.Error("IsBangOnly should be false")
	}
}

func TestManagerParseSuffixBang(t *testing.T) {
	m := NewManager()

	result := m.Parse("golang tutorial !g")
	if result == nil {
		t.Fatal("Should find Google bang")
	}
	if result.Bang.Shortcut != "g" {
		t.Errorf("Bang.Shortcut = %q", result.Bang.Shortcut)
	}
	if result.Query != "golang tutorial" {
		t.Errorf("Query = %q, want 'golang tutorial'", result.Query)
	}
}

func TestManagerParseBangOnly(t *testing.T) {
	m := NewManager()

	result := m.Parse("!g")
	if result == nil {
		t.Fatal("Should find Google bang")
	}
	if !result.IsBangOnly {
		t.Error("IsBangOnly should be true")
	}
	if result.Query != "" {
		t.Errorf("Query = %q, want empty", result.Query)
	}
}

func TestManagerParseUnknownBang(t *testing.T) {
	m := NewManager()

	result := m.Parse("!unknownbang123 test")
	if result != nil {
		t.Error("Parse() with unknown bang should return nil")
	}
}

func TestManagerParseCaseInsensitive(t *testing.T) {
	m := NewManager()

	tests := []string{"!G test", "!g test", "!Google test"}
	for _, query := range tests {
		result := m.Parse(query)
		if result == nil {
			t.Errorf("Should find bang for %q", query)
		}
	}
}

// Tests for buildURL

func TestManagerBuildURL(t *testing.T) {
	m := NewManager()

	tests := []struct {
		name     string
		bang     *Bang
		query    string
		wantHas  string
	}{
		{
			name:    "query placeholder",
			bang:    &Bang{URL: "https://example.com?q={query}"},
			query:   "test query",
			wantHas: "test+query",
		},
		{
			name:    "legacy %s placeholder",
			bang:    &Bang{URL: "https://example.com?s=%s"},
			query:   "test",
			wantHas: "test",
		},
		{
			name:    "append with existing query param",
			bang:    &Bang{URL: "https://example.com?lang=en"},
			query:   "test",
			wantHas: "&q=test",
		},
		{
			name:    "append without query param",
			bang:    &Bang{URL: "https://example.com"},
			query:   "test",
			wantHas: "?q=test",
		},
		{
			name:    "empty query returns base URL",
			bang:    &Bang{URL: "https://example.com?q={query}"},
			query:   "",
			wantHas: "https://example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := m.buildURL(tt.bang, tt.query)
			if !strings.Contains(url, tt.wantHas) {
				t.Errorf("buildURL() = %q, should contain %q", url, tt.wantHas)
			}
		})
	}
}

func TestManagerBuildURLEncodesSpecialChars(t *testing.T) {
	m := NewManager()
	bang := &Bang{URL: "https://example.com?q={query}"}

	url := m.buildURL(bang, "hello world & test")
	if !strings.Contains(url, "hello+world") {
		t.Errorf("buildURL() = %q, should encode spaces", url)
	}
	if !strings.Contains(url, "%26") {
		t.Errorf("buildURL() = %q, should encode ampersand", url)
	}
}

// Tests for GetAll

func TestManagerGetAll(t *testing.T) {
	m := NewManager()

	all := m.GetAll()
	if len(all) == 0 {
		t.Error("GetAll() should return bangs")
	}
}

func TestManagerGetAllIncludesUserAndCustom(t *testing.T) {
	m := NewManager()
	m.SetCustomBangs([]*Bang{{Shortcut: "custom1", Name: "Custom 1", URL: "https://custom1.com"}})
	m.SetUserBangs([]*Bang{{Shortcut: "user1", Name: "User 1", URL: "https://user1.com"}})

	all := m.GetAll()

	foundCustom := false
	foundUser := false
	for _, b := range all {
		if b.Shortcut == "custom1" {
			foundCustom = true
		}
		if b.Shortcut == "user1" {
			foundUser = true
		}
	}

	if !foundCustom {
		t.Error("GetAll() should include custom bangs")
	}
	if !foundUser {
		t.Error("GetAll() should include user bangs")
	}
}

// Tests for GetBuiltins

func TestManagerGetBuiltins(t *testing.T) {
	m := NewManager()

	builtins := m.GetBuiltins()
	if len(builtins) == 0 {
		t.Error("GetBuiltins() should return bangs")
	}

	// All returned should be builtins (exist in m.builtins)
	for _, b := range builtins {
		if _, exists := m.builtins[b.Shortcut]; !exists {
			t.Errorf("GetBuiltins() returned non-builtin: %s", b.Shortcut)
		}
	}
}

// Tests for GetByCategory

func TestManagerGetByCategory(t *testing.T) {
	m := NewManager()

	searchBangs := m.GetByCategory("search")

	for _, b := range searchBangs {
		if b.Category != "search" {
			t.Errorf("GetByCategory('search') returned wrong category: %s", b.Category)
		}
	}
}

func TestManagerGetByCategoryEmpty(t *testing.T) {
	m := NewManager()

	result := m.GetByCategory("nonexistent_category_12345")
	if len(result) != 0 {
		t.Errorf("GetByCategory() for nonexistent category should return empty, got %d", len(result))
	}
}

// Tests for GetCategories

func TestManagerGetCategories(t *testing.T) {
	m := NewManager()

	categories := m.GetCategories()
	if len(categories) == 0 {
		t.Error("GetCategories() should return categories")
	}

	// Check for unique values
	seen := make(map[string]bool)
	for _, cat := range categories {
		if seen[cat] {
			t.Errorf("GetCategories() returned duplicate: %s", cat)
		}
		seen[cat] = true
	}
}

// Tests for IsBang

func TestManagerIsBang(t *testing.T) {
	m := NewManager()

	if !m.IsBang("!g test") {
		t.Error("IsBang('!g test') should return true")
	}
	if m.IsBang("regular query") {
		t.Error("IsBang('regular query') should return false")
	}
}

// Tests for ExtractBang

func TestExtractBang(t *testing.T) {
	tests := []struct {
		query    string
		expected string
	}{
		{"!g test", "g"},
		{"test !g", "g"},
		{"!google query", "google"},
		{"query !ddg", "ddg"},
		{"no bang here", ""},
		{"", ""},
	}

	for _, tt := range tests {
		result := ExtractBang(tt.query)
		if result != tt.expected {
			t.Errorf("ExtractBang(%q) = %q, want %q", tt.query, result, tt.expected)
		}
	}
}
