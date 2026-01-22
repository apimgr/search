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

// Additional tests for 100% coverage

// Test parseBangSuffix returns nil when bang not found
func TestManagerParseSuffixBangNotFound(t *testing.T) {
	m := NewManager()

	// Suffix bang that doesn't exist
	result := m.Parse("golang tutorial !unknownbang12345")
	if result != nil {
		t.Error("Parse() with unknown suffix bang should return nil")
	}
}

// Test SetUserBangs with aliases
func TestManagerSetUserBangsWithAliases(t *testing.T) {
	m := NewManager()
	userBangs := []*Bang{
		{
			Shortcut: "mysite",
			Name:     "My Site",
			URL:      "https://mysite.com?q={query}",
			Aliases:  []string{"ms", "mysite2"},
		},
	}

	m.SetUserBangs(userBangs)

	// Test main shortcut
	result := m.Parse("!mysite test")
	if result == nil {
		t.Fatal("Should find user bang by shortcut")
	}
	if result.Bang.Shortcut != "mysite" {
		t.Errorf("Bang.Shortcut = %q, want 'mysite'", result.Bang.Shortcut)
	}

	// Test first alias
	result = m.Parse("!ms test")
	if result == nil {
		t.Fatal("Should find user bang by alias 'ms'")
	}
	if result.Bang.Shortcut != "mysite" {
		t.Errorf("Bang.Shortcut = %q, want 'mysite'", result.Bang.Shortcut)
	}

	// Test second alias
	result = m.Parse("!mysite2 test")
	if result == nil {
		t.Fatal("Should find user bang by alias 'mysite2'")
	}
	if result.Bang.Shortcut != "mysite" {
		t.Errorf("Bang.Shortcut = %q, want 'mysite'", result.Bang.Shortcut)
	}
}

// Test lookup priority: user > custom > builtin
func TestManagerLookupPriority(t *testing.T) {
	m := NewManager()

	// Set custom bang with same shortcut as builtin
	m.SetCustomBangs([]*Bang{
		{Shortcut: "g", Name: "Custom Google", URL: "https://custom-google.com?q={query}"},
	})

	// Custom should override builtin
	result := m.Parse("!g test")
	if result == nil {
		t.Fatal("Should find bang")
	}
	if result.Bang.Name != "Custom Google" {
		t.Errorf("Custom bang should override builtin, got Name = %q", result.Bang.Name)
	}

	// Now set user bang - should override custom
	m.SetUserBangs([]*Bang{
		{Shortcut: "g", Name: "User Google", URL: "https://user-google.com?q={query}"},
	})

	result = m.Parse("!g test")
	if result == nil {
		t.Fatal("Should find bang")
	}
	if result.Bang.Name != "User Google" {
		t.Errorf("User bang should override custom, got Name = %q", result.Bang.Name)
	}
}

// Test lookup returns nil when shortcut not found in any map
func TestManagerLookupNotFound(t *testing.T) {
	m := NewManager()

	// Clear all maps to ensure nothing is found
	m.builtins = make(map[string]*Bang)
	m.custom = make(map[string]*Bang)
	m.user = make(map[string]*Bang)

	result := m.Parse("!anything test")
	if result != nil {
		t.Error("Should return nil when bang not found in any map")
	}
}

// Test buildURL with URL that has no query parameter placeholder and no existing ?
func TestManagerBuildURLNoPlaceholderNoQuestion(t *testing.T) {
	m := NewManager()
	bang := &Bang{URL: "https://example.com/search"}

	url := m.buildURL(bang, "test query")
	expected := "https://example.com/search?q=test+query"
	if url != expected {
		t.Errorf("buildURL() = %q, want %q", url, expected)
	}
}

// Test buildURL with empty query and URL containing question mark
func TestManagerBuildURLEmptyQueryWithQuestionMark(t *testing.T) {
	m := NewManager()
	bang := &Bang{URL: "https://example.com/search?lang=en&q={query}"}

	url := m.buildURL(bang, "")
	// Should strip everything after ?
	if url != "https://example.com/search" {
		t.Errorf("buildURL() with empty query = %q, want 'https://example.com/search'", url)
	}
}

// Test buildURL with empty query and URL without question mark
func TestManagerBuildURLEmptyQueryNoQuestionMark(t *testing.T) {
	m := NewManager()
	bang := &Bang{URL: "https://example.com/search"}

	url := m.buildURL(bang, "")
	// Should return URL as-is
	if url != "https://example.com/search" {
		t.Errorf("buildURL() with empty query = %q, want 'https://example.com/search'", url)
	}
}

// Test ExtractBang with trailing bang format (word!)
func TestExtractBangTrailingBang(t *testing.T) {
	tests := []struct {
		query    string
		expected string
	}{
		{"google!", "google"},
		{"ddg!", "ddg"},
		{"wiki!", "wiki"},
	}

	for _, tt := range tests {
		result := ExtractBang(tt.query)
		if result != tt.expected {
			t.Errorf("ExtractBang(%q) = %q, want %q", tt.query, result, tt.expected)
		}
	}
}

// Test ExtractBang with middle-of-query bang
func TestExtractBangMiddleOfQuery(t *testing.T) {
	tests := []struct {
		query    string
		expected string
	}{
		{"golang !g tutorial", "g"},
		{"search !wiki for info", "wiki"},
		{"test !ddg query", "ddg"},
	}

	for _, tt := range tests {
		result := ExtractBang(tt.query)
		if result != tt.expected {
			t.Errorf("ExtractBang(%q) = %q, want %q", tt.query, result, tt.expected)
		}
	}
}

// Test parseBangPrefix with whitespace in query
func TestManagerParsePrefixBangWithExtraWhitespace(t *testing.T) {
	m := NewManager()

	// Extra whitespace after bang
	result := m.Parse("!g    golang tutorial")
	if result == nil {
		t.Fatal("Should find bang")
	}
	if result.Query != "golang tutorial" {
		t.Errorf("Query = %q, want 'golang tutorial'", result.Query)
	}
}

// Test parseBangSuffix with IsBangOnly edge case
func TestManagerParseSuffixBangOnly(t *testing.T) {
	m := NewManager()

	// This won't match suffix pattern because there's no space before !
	// Pattern requires " !" with space before
	result := m.Parse("query !g")
	if result == nil {
		t.Fatal("Should find bang")
	}
	// Query should be "query" not empty
	if result.Query != "query" {
		t.Errorf("Query = %q, want 'query'", result.Query)
	}
	if result.IsBangOnly {
		t.Error("IsBangOnly should be false for suffix bang with query")
	}
}

// Test GetAll deduplication
func TestManagerGetAllDeduplication(t *testing.T) {
	m := NewManager()

	// Add custom and user bangs with same shortcut
	m.SetCustomBangs([]*Bang{
		{Shortcut: "test", Name: "Custom Test", URL: "https://custom.com"},
	})
	m.SetUserBangs([]*Bang{
		{Shortcut: "test", Name: "User Test", URL: "https://user.com"},
	})

	all := m.GetAll()

	// Count how many have shortcut "test"
	count := 0
	for _, b := range all {
		if b.Shortcut == "test" {
			count++
		}
	}

	if count != 1 {
		t.Errorf("GetAll() should deduplicate, found %d 'test' bangs", count)
	}
}

// Test GetCategories with empty category
func TestManagerGetCategoriesSkipsEmpty(t *testing.T) {
	m := NewManager()

	// Add bang with empty category
	m.SetCustomBangs([]*Bang{
		{Shortcut: "nocategory", Name: "No Category", URL: "https://example.com", Category: ""},
	})

	categories := m.GetCategories()

	for _, cat := range categories {
		if cat == "" {
			t.Error("GetCategories() should not include empty categories")
		}
	}
}

// Test Parse with leading/trailing whitespace
func TestManagerParseWithWhitespace(t *testing.T) {
	m := NewManager()

	result := m.Parse("  !g test  ")
	if result == nil {
		t.Fatal("Should find bang with leading/trailing whitespace")
	}
	if result.Bang.Shortcut != "g" {
		t.Errorf("Bang.Shortcut = %q", result.Bang.Shortcut)
	}
}

// Test Parse suffix bang at exact position (edge case for LastIndex)
func TestManagerParseSuffixBangExactPosition(t *testing.T) {
	m := NewManager()

	// Query where " !" appears at exact end
	result := m.Parse("a !g")
	if result == nil {
		t.Fatal("Should find suffix bang")
	}
	if result.Query != "a" {
		t.Errorf("Query = %q, want 'a'", result.Query)
	}
}

// Test that suffix bang with additional text after works
func TestManagerParseSuffixBangWithTrailingText(t *testing.T) {
	m := NewManager()

	// Suffix bang followed by more text (edge case in parseBangSuffix)
	result := m.Parse("query !g extra")
	if result == nil {
		t.Fatal("Should find suffix bang")
	}
	// The parseBangSuffix only takes the first word after !
	if result.Bang.Shortcut != "g" {
		t.Errorf("Bang.Shortcut = %q, want 'g'", result.Bang.Shortcut)
	}
}

// Test IsBang with various edge cases
func TestManagerIsBangEdgeCases(t *testing.T) {
	m := NewManager()

	tests := []struct {
		query    string
		expected bool
	}{
		{"!g", true},
		{"!g test", true},
		{"test !g", true},
		{"test", false},
		{"!", false},            // Just exclamation, no shortcut
		{"!unknown12345", false}, // Unknown bang
		{"   !g   ", true},      // Whitespace around
	}

	for _, tt := range tests {
		result := m.IsBang(tt.query)
		if result != tt.expected {
			t.Errorf("IsBang(%q) = %v, want %v", tt.query, result, tt.expected)
		}
	}
}

// Test buildURL with multiple {query} placeholders
func TestManagerBuildURLMultiplePlaceholders(t *testing.T) {
	m := NewManager()
	bang := &Bang{URL: "https://example.com?q={query}&search={query}"}

	url := m.buildURL(bang, "test")
	// Should replace all occurrences
	if !strings.Contains(url, "q=test") || !strings.Contains(url, "search=test") {
		t.Errorf("buildURL() = %q, should replace all {query} placeholders", url)
	}
}

// Test default bangs are properly loaded with aliases
func TestDefaultBangsAliasesLoaded(t *testing.T) {
	m := NewManager()

	// Test that Google alias works
	result := m.Parse("!google test")
	if result == nil {
		t.Fatal("Should find Google by alias 'google'")
	}
	if result.Bang.Shortcut != "g" {
		t.Errorf("Bang.Shortcut = %q, want 'g'", result.Bang.Shortcut)
	}

	// Test that DuckDuckGo alias works
	result = m.Parse("!duck test")
	if result == nil {
		t.Fatal("Should find DuckDuckGo by alias 'duck'")
	}
	if result.Bang.Shortcut != "ddg" {
		t.Errorf("Bang.Shortcut = %q, want 'ddg'", result.Bang.Shortcut)
	}
}

// Test SetCustomBangs replaces previous custom bangs
func TestManagerSetCustomBangsReplaces(t *testing.T) {
	m := NewManager()

	// Set first custom bangs
	m.SetCustomBangs([]*Bang{
		{Shortcut: "first", Name: "First", URL: "https://first.com"},
	})

	// Verify first is found
	result := m.Parse("!first test")
	if result == nil {
		t.Fatal("Should find 'first' custom bang")
	}

	// Set new custom bangs (should replace)
	m.SetCustomBangs([]*Bang{
		{Shortcut: "second", Name: "Second", URL: "https://second.com"},
	})

	// First should no longer be found
	result = m.Parse("!first test")
	if result != nil {
		t.Error("'first' custom bang should have been replaced")
	}

	// Second should be found
	result = m.Parse("!second test")
	if result == nil {
		t.Fatal("Should find 'second' custom bang")
	}
}

// Test SetUserBangs replaces previous user bangs
func TestManagerSetUserBangsReplaces(t *testing.T) {
	m := NewManager()

	// Set first user bangs
	m.SetUserBangs([]*Bang{
		{Shortcut: "first", Name: "First", URL: "https://first.com"},
	})

	// Verify first is found
	result := m.Parse("!first test")
	if result == nil {
		t.Fatal("Should find 'first' user bang")
	}

	// Set new user bangs (should replace)
	m.SetUserBangs([]*Bang{
		{Shortcut: "second", Name: "Second", URL: "https://second.com"},
	})

	// First should no longer be found
	result = m.Parse("!first test")
	if result != nil {
		t.Error("'first' user bang should have been replaced")
	}

	// Second should be found
	result = m.Parse("!second test")
	if result == nil {
		t.Fatal("Should find 'second' user bang")
	}
}

// Test GetByCategory returns correct results
func TestManagerGetByCategoryReturnsCorrect(t *testing.T) {
	m := NewManager()

	// Get general category bangs
	generalBangs := m.GetByCategory("general")
	if len(generalBangs) == 0 {
		t.Fatal("Should have general category bangs")
	}

	// All returned should have correct category
	for _, b := range generalBangs {
		if b.Category != "general" {
			t.Errorf("GetByCategory('general') returned bang with category %q", b.Category)
		}
	}

	// Get code category bangs
	codeBangs := m.GetByCategory("code")
	if len(codeBangs) == 0 {
		t.Fatal("Should have code category bangs")
	}

	for _, b := range codeBangs {
		if b.Category != "code" {
			t.Errorf("GetByCategory('code') returned bang with category %q", b.Category)
		}
	}
}

// Test concurrent access to Manager (thread safety)
func TestManagerConcurrentAccess(t *testing.T) {
	m := NewManager()

	done := make(chan bool)

	// Concurrent reads
	for i := 0; i < 10; i++ {
		go func() {
			m.Parse("!g test")
			m.GetAll()
			m.GetBuiltins()
			m.GetCategories()
			m.GetByCategory("general")
			done <- true
		}()
	}

	// Concurrent writes
	for i := 0; i < 5; i++ {
		go func(id int) {
			m.SetCustomBangs([]*Bang{
				{Shortcut: "custom", Name: "Custom", URL: "https://custom.com"},
			})
			m.SetUserBangs([]*Bang{
				{Shortcut: "user", Name: "User", URL: "https://user.com"},
			})
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 15; i++ {
		<-done
	}
}

// Test TargetURL is set correctly in BangResult
func TestBangResultTargetURL(t *testing.T) {
	m := NewManager()

	result := m.Parse("!g golang")
	if result == nil {
		t.Fatal("Should find bang")
	}

	if result.TargetURL == "" {
		t.Error("TargetURL should not be empty")
	}

	if !strings.Contains(result.TargetURL, "golang") {
		t.Errorf("TargetURL = %q, should contain 'golang'", result.TargetURL)
	}
}

// Test TargetURL for bang-only query
func TestBangResultTargetURLBangOnly(t *testing.T) {
	m := NewManager()

	result := m.Parse("!g")
	if result == nil {
		t.Fatal("Should find bang")
	}

	if result.TargetURL == "" {
		t.Error("TargetURL should not be empty even for bang-only")
	}

	// Should be base URL without query param
	if strings.Contains(result.TargetURL, "{query}") {
		t.Errorf("TargetURL = %q, should not contain {query} placeholder", result.TargetURL)
	}
}

// Test buildURL with URL where ? is at the very beginning (edge case)
func TestManagerBuildURLQuestionAtStart(t *testing.T) {
	m := NewManager()
	// Edge case: URL that starts with ? (unlikely but possible)
	bang := &Bang{URL: "?query={query}"}

	// With query
	url := m.buildURL(bang, "test")
	if !strings.Contains(url, "test") {
		t.Errorf("buildURL() = %q, should contain 'test'", url)
	}

	// Without query - idx == 0 so shouldn't strip
	urlEmpty := m.buildURL(bang, "")
	if urlEmpty != "?query={query}" {
		t.Errorf("buildURL() with empty = %q, expected '?query={query}'", urlEmpty)
	}
}

// Test parseBangPrefix with single character bang
func TestManagerParsePrefixSingleCharBang(t *testing.T) {
	m := NewManager()

	result := m.Parse("!g test")
	if result == nil {
		t.Fatal("Should find single char bang 'g'")
	}
	if result.Bang.Shortcut != "g" {
		t.Errorf("Bang.Shortcut = %q, want 'g'", result.Bang.Shortcut)
	}
}

// Test that Parse handles special characters in query correctly
func TestManagerParseSpecialCharsInQuery(t *testing.T) {
	m := NewManager()

	tests := []struct {
		input       string
		shouldFind  bool
		expectedQuery string
	}{
		{"!g golang & tutorial", true, "golang & tutorial"},
		{"!g hello=world", true, "hello=world"},
		{"!g test?query", true, "test?query"},
		{"!g a+b", true, "a+b"},
	}

	for _, tt := range tests {
		result := m.Parse(tt.input)
		if tt.shouldFind && result == nil {
			t.Errorf("Parse(%q) should find bang", tt.input)
			continue
		}
		if result != nil && result.Query != tt.expectedQuery {
			t.Errorf("Parse(%q).Query = %q, want %q", tt.input, result.Query, tt.expectedQuery)
		}
	}
}

// Test GetBuiltins does not include custom or user bangs
func TestManagerGetBuiltinsExcludesCustomAndUser(t *testing.T) {
	m := NewManager()

	// Add custom and user bangs
	m.SetCustomBangs([]*Bang{
		{Shortcut: "customonly", Name: "Custom Only", URL: "https://custom.com"},
	})
	m.SetUserBangs([]*Bang{
		{Shortcut: "useronly", Name: "User Only", URL: "https://user.com"},
	})

	builtins := m.GetBuiltins()

	for _, b := range builtins {
		if b.Shortcut == "customonly" {
			t.Error("GetBuiltins() should not include custom bangs")
		}
		if b.Shortcut == "useronly" {
			t.Error("GetBuiltins() should not include user bangs")
		}
	}
}

// Test suffix bang detection when bang is at the very end with multiple spaces
func TestManagerParseSuffixBangMultipleSpaces(t *testing.T) {
	m := NewManager()

	result := m.Parse("search query  !g")
	if result == nil {
		t.Fatal("Should find suffix bang with multiple spaces")
	}
	if result.Query != "search query" {
		t.Errorf("Query = %q, want 'search query'", result.Query)
	}
}

// Test empty custom bangs slice
func TestManagerSetCustomBangsEmpty(t *testing.T) {
	m := NewManager()

	// First set some custom bangs
	m.SetCustomBangs([]*Bang{
		{Shortcut: "custom1", Name: "Custom 1", URL: "https://custom1.com"},
	})

	// Then set empty slice
	m.SetCustomBangs([]*Bang{})

	// Custom bang should not be found
	result := m.Parse("!custom1 test")
	if result != nil {
		t.Error("Custom bang should have been cleared")
	}
}

// Test empty user bangs slice
func TestManagerSetUserBangsEmpty(t *testing.T) {
	m := NewManager()

	// First set some user bangs
	m.SetUserBangs([]*Bang{
		{Shortcut: "user1", Name: "User 1", URL: "https://user1.com"},
	})

	// Then set empty slice
	m.SetUserBangs([]*Bang{})

	// User bang should not be found
	result := m.Parse("!user1 test")
	if result != nil {
		t.Error("User bang should have been cleared")
	}
}

// Test ExtractBang with various edge cases
func TestExtractBangEdgeCases(t *testing.T) {
	tests := []struct {
		query    string
		expected string
	}{
		{"!test", "test"},              // Start bang
		{"test!", "test"},              // End bang
		{"query !test more", "test"},   // Middle bang with continuation
		{"!test123", "test123"},        // Bang with numbers
		{"123!", "123"},                // Numeric-only bang
		{"!a", "a"},                    // Single char bang
		{"a!", "a"},                    // Single char trailing bang
		{"!abc def !xyz", "abc"},       // Multiple bangs (first wins)
		{"word word2", ""},             // No bang
	}

	for _, tt := range tests {
		result := ExtractBang(tt.query)
		if result != tt.expected {
			t.Errorf("ExtractBang(%q) = %q, want %q", tt.query, result, tt.expected)
		}
	}
}

// Test that URL encoding works correctly for special characters
func TestManagerBuildURLSpecialChars(t *testing.T) {
	m := NewManager()

	tests := []struct {
		query    string
		contains string
	}{
		{"hello world", "hello+world"},
		{"a&b", "%26"},
		{"test=value", "%3D"},
		{"search?query", "%3F"},
		{"100%", "100%25"},
		{"path/to/file", "%2F"},
	}

	bang := &Bang{URL: "https://example.com?q={query}"}

	for _, tt := range tests {
		url := m.buildURL(bang, tt.query)
		if !strings.Contains(url, tt.contains) {
			t.Errorf("buildURL(%q) = %q, should contain %q", tt.query, url, tt.contains)
		}
	}
}

// Test that all default bangs have required fields
func TestDefaultBangsHaveRequiredFields(t *testing.T) {
	m := NewManager()
	all := m.GetBuiltins()

	for _, b := range all {
		if b.Shortcut == "" {
			t.Errorf("Bang %q has empty shortcut", b.Name)
		}
		if b.Name == "" {
			t.Errorf("Bang with shortcut %q has empty name", b.Shortcut)
		}
		if b.URL == "" {
			t.Errorf("Bang %q has empty URL", b.Shortcut)
		}
		if b.Category == "" {
			t.Errorf("Bang %q has empty category", b.Shortcut)
		}
	}
}

// Test that BangResult has all fields set correctly for prefix bang
func TestBangResultFieldsPrefixBang(t *testing.T) {
	m := NewManager()

	result := m.Parse("!g search term")
	if result == nil {
		t.Fatal("Should find bang")
	}

	// Check all fields
	if result.Bang == nil {
		t.Error("Bang should not be nil")
	}
	if result.Bang.Shortcut != "g" {
		t.Errorf("Bang.Shortcut = %q", result.Bang.Shortcut)
	}
	if result.Query != "search term" {
		t.Errorf("Query = %q", result.Query)
	}
	if result.TargetURL == "" {
		t.Error("TargetURL should not be empty")
	}
	if result.IsBangOnly {
		t.Error("IsBangOnly should be false")
	}
}

// Test that BangResult has all fields set correctly for suffix bang
func TestBangResultFieldsSuffixBang(t *testing.T) {
	m := NewManager()

	result := m.Parse("search term !g")
	if result == nil {
		t.Fatal("Should find bang")
	}

	// Check all fields
	if result.Bang == nil {
		t.Error("Bang should not be nil")
	}
	if result.Bang.Shortcut != "g" {
		t.Errorf("Bang.Shortcut = %q", result.Bang.Shortcut)
	}
	if result.Query != "search term" {
		t.Errorf("Query = %q", result.Query)
	}
	if result.TargetURL == "" {
		t.Error("TargetURL should not be empty")
	}
	if result.IsBangOnly {
		t.Error("IsBangOnly should be false for suffix bang")
	}
}

// Test GetCategories uniqueness with overlapping bangs
func TestManagerGetCategoriesUniqueness(t *testing.T) {
	m := NewManager()

	categories := m.GetCategories()

	// Check uniqueness
	seen := make(map[string]int)
	for _, cat := range categories {
		seen[cat]++
	}

	for cat, count := range seen {
		if count > 1 {
			t.Errorf("Category %q appears %d times, should be unique", cat, count)
		}
	}
}

// Test that prefix bang with only bang shortcut (no query) works
func TestManagerParsePrefixBangOnlyNoSpace(t *testing.T) {
	m := NewManager()

	result := m.Parse("!ddg")
	if result == nil {
		t.Fatal("Should find ddg bang")
	}
	if result.Bang.Shortcut != "ddg" {
		t.Errorf("Bang.Shortcut = %q, want 'ddg'", result.Bang.Shortcut)
	}
	if !result.IsBangOnly {
		t.Error("IsBangOnly should be true")
	}
	if result.Query != "" {
		t.Errorf("Query = %q, should be empty", result.Query)
	}
}
