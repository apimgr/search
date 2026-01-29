package direct

import (
	"context"
	"testing"
	"time"
)

func TestNewManager(t *testing.T) {
	m := NewManager()
	if m == nil {
		t.Fatal("NewManager returned nil")
	}

	// Verify all handlers are registered
	expectedTypes := []AnswerType{
		AnswerTypeTLDR, AnswerTypeMan, AnswerTypeCheat,
		AnswerTypeDNS, AnswerTypeWhois, AnswerTypeResolve, AnswerTypeCert, AnswerTypeHeaders, AnswerTypeASN, AnswerTypeSubnet,
		AnswerTypeWiki, AnswerTypeDict, AnswerTypeThesaurus, AnswerTypePkg, AnswerTypeCVE, AnswerTypeRFC, AnswerTypeDirectory,
		AnswerTypeHTML, AnswerTypeUnicode, AnswerTypeEmoji, AnswerTypeEscape, AnswerTypeJSON, AnswerTypeYAML,
		AnswerTypeHTTP, AnswerTypePort, AnswerTypeCron, AnswerTypeChmod, AnswerTypeRegex, AnswerTypeJWT, AnswerTypeTimestamp,
		AnswerTypeRobots, AnswerTypeSitemap, AnswerTypeTech, AnswerTypeFeed, AnswerTypeExpand, AnswerTypeSafe, AnswerTypeCache,
		AnswerTypeCase, AnswerTypeSlug, AnswerTypeLorem, AnswerTypeWord, AnswerTypeBeautify, AnswerTypeDiff,
		AnswerTypeUserAgent, AnswerTypeMIME, AnswerTypeLicense, AnswerTypeCountry, AnswerTypeASCII, AnswerTypeQR,
	}

	for _, at := range expectedTypes {
		if _, ok := m.handlers[at]; !ok {
			t.Errorf("Handler for type %s not registered", at)
		}
	}
}

func TestParse(t *testing.T) {
	m := NewManager()

	tests := []struct {
		query        string
		expectedType AnswerType
		expectedTerm string
	}{
		{"tldr:git", AnswerTypeTLDR, "git"},
		{"dns:example.com", AnswerTypeDNS, "example.com"},
		{"wiki: Python programming", AnswerTypeWiki, "Python programming"},
		{"http:404", AnswerTypeHTTP, "404"},
		{"chmod:755", AnswerTypeChmod, "755"},
		{"cron:* * * * *", AnswerTypeCron, "* * * * *"},
		{"", "", ""},
		{"notacommand", "", ""},
		{"invalid:", "", ""},
		{"unknown:term", "", ""}, // unknown type should not match
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			answerType, term := m.Parse(tt.query)
			if answerType != tt.expectedType {
				t.Errorf("Parse(%q) type = %v, want %v", tt.query, answerType, tt.expectedType)
			}
			if term != tt.expectedTerm {
				t.Errorf("Parse(%q) term = %v, want %v", tt.query, term, tt.expectedTerm)
			}
		})
	}
}

func TestIsDirectAnswer(t *testing.T) {
	m := NewManager()

	tests := []struct {
		query    string
		expected bool
	}{
		{"tldr:git", true},
		{"dns:example.com", true},
		{"wiki:Python", true},
		{"regular search query", false},
		{"not:ahandler", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			result := m.IsDirectAnswer(tt.query)
			if result != tt.expected {
				t.Errorf("IsDirectAnswer(%q) = %v, want %v", tt.query, result, tt.expected)
			}
		})
	}
}

func TestCacheDurations(t *testing.T) {
	// Verify key cache durations are set correctly
	tests := []struct {
		answerType AnswerType
		minTTL     time.Duration
		maxTTL     time.Duration
	}{
		{AnswerTypeTLDR, 7 * 24 * time.Hour, 7 * 24 * time.Hour},
		{AnswerTypeMan, 30 * 24 * time.Hour, 30 * 24 * time.Hour},
		{AnswerTypeDNS, 1 * time.Hour, 1 * time.Hour},
		{AnswerTypeHTTP, 0, 0},       // Static, no cache
		{AnswerTypeChmod, 0, 0},      // Static, no cache
		{AnswerTypeCache, 0, 0},      // Never cache
		{AnswerTypeWiki, 24 * time.Hour, 24 * time.Hour},
	}

	for _, tt := range tests {
		t.Run(string(tt.answerType), func(t *testing.T) {
			duration := CacheDurations[tt.answerType]
			if duration < tt.minTTL || duration > tt.maxTTL {
				t.Errorf("CacheDurations[%s] = %v, want between %v and %v", tt.answerType, duration, tt.minTTL, tt.maxTTL)
			}
		})
	}
}

func TestHTTPHandler(t *testing.T) {
	h := NewHTTPHandler()

	ctx := context.Background()

	// Test valid status codes
	validCodes := []string{"200", "404", "500", "301", "403"}
	for _, code := range validCodes {
		t.Run("code_"+code, func(t *testing.T) {
			answer, err := h.Handle(ctx, code)
			if err != nil {
				t.Fatalf("Handle(%s) error: %v", code, err)
			}
			if answer == nil {
				t.Fatal("Handle returned nil answer")
			}
			if answer.Type != AnswerTypeHTTP {
				t.Errorf("Answer type = %v, want %v", answer.Type, AnswerTypeHTTP)
			}
			if answer.Content == "" {
				t.Error("Answer content is empty")
			}
		})
	}

	// Test invalid status code
	t.Run("invalid_code", func(t *testing.T) {
		answer, err := h.Handle(ctx, "invalid")
		if err != nil {
			t.Fatalf("Handle(invalid) error: %v", err)
		}
		if answer.Error == "" {
			t.Error("Expected error in answer for invalid code")
		}
	})
}

func TestPortHandler(t *testing.T) {
	h := NewPortHandler()

	ctx := context.Background()

	// Test well-known ports
	tests := []struct {
		port        string
		expectMatch bool
	}{
		{"80", true},
		{"443", true},
		{"22", true},
		{"21", true},
		{"25", true},
		{"3306", true},
		{"5432", true},
		{"99999", false}, // Port out of range or unknown
	}

	for _, tt := range tests {
		t.Run("port_"+tt.port, func(t *testing.T) {
			answer, err := h.Handle(ctx, tt.port)
			if err != nil {
				t.Fatalf("Handle(%s) error: %v", tt.port, err)
			}
			if answer == nil {
				t.Fatal("Handle returned nil answer")
			}
			if tt.expectMatch && answer.Error != "" {
				t.Errorf("Expected match for port %s but got error: %s", tt.port, answer.Error)
			}
		})
	}
}

func TestChmodHandler(t *testing.T) {
	h := NewChmodHandler()

	ctx := context.Background()

	tests := []struct {
		input string
	}{
		{"755"},
		{"644"},
		{"777"},
		{"400"},
		{"rwxr-xr-x"},
		{"rw-r--r--"},
	}

	for _, tt := range tests {
		t.Run("chmod_"+tt.input, func(t *testing.T) {
			answer, err := h.Handle(ctx, tt.input)
			if err != nil {
				t.Fatalf("Handle(%s) error: %v", tt.input, err)
			}
			if answer == nil {
				t.Fatal("Handle returned nil answer")
			}
			if answer.Content == "" {
				t.Error("Answer content is empty")
			}
		})
	}
}

func TestCronHandler(t *testing.T) {
	h := NewCronHandler()

	ctx := context.Background()

	tests := []struct {
		expr string
	}{
		{"* * * * *"},
		{"0 0 * * *"},
		{"0 0 1 * *"},
		{"30 4 1,15 * 5"},
		{"@daily"},
		{"@weekly"},
		{"@monthly"},
	}

	for _, tt := range tests {
		t.Run("cron_"+tt.expr, func(t *testing.T) {
			answer, err := h.Handle(ctx, tt.expr)
			if err != nil {
				t.Fatalf("Handle(%s) error: %v", tt.expr, err)
			}
			if answer == nil {
				t.Fatal("Handle returned nil answer")
			}
			if answer.Content == "" {
				t.Error("Answer content is empty")
			}
		})
	}
}

func TestSubnetHandler(t *testing.T) {
	h := NewSubnetHandler()

	ctx := context.Background()

	tests := []struct {
		cidr string
	}{
		{"192.168.1.0/24"},
		{"10.0.0.0/8"},
		{"172.16.0.0/12"},
		{"192.168.0.0/16"},
	}

	for _, tt := range tests {
		t.Run("subnet_"+tt.cidr, func(t *testing.T) {
			answer, err := h.Handle(ctx, tt.cidr)
			if err != nil {
				t.Fatalf("Handle(%s) error: %v", tt.cidr, err)
			}
			if answer == nil {
				t.Fatal("Handle returned nil answer")
			}
			if answer.Content == "" {
				t.Error("Answer content is empty")
			}
		})
	}
}

func TestCaseHandler(t *testing.T) {
	h := NewCaseHandler()

	ctx := context.Background()

	tests := []struct {
		input string
	}{
		{"hello world"},
		{"HelloWorld"},
		{"hello_world"},
		{"HELLO WORLD"},
	}

	for _, tt := range tests {
		t.Run("case_"+tt.input, func(t *testing.T) {
			answer, err := h.Handle(ctx, tt.input)
			if err != nil {
				t.Fatalf("Handle(%s) error: %v", tt.input, err)
			}
			if answer == nil {
				t.Fatal("Handle returned nil answer")
			}
			if answer.Content == "" {
				t.Error("Answer content is empty")
			}
		})
	}
}

func TestSlugHandler(t *testing.T) {
	h := NewSlugHandler()

	ctx := context.Background()

	tests := []struct {
		input    string
		expected string
	}{
		{"Hello World", "hello-world"},
		{"This is a Test!", "this-is-a-test"},
		{"Special & Characters", "special-characters"},
	}

	for _, tt := range tests {
		t.Run("slug_"+tt.input, func(t *testing.T) {
			answer, err := h.Handle(ctx, tt.input)
			if err != nil {
				t.Fatalf("Handle(%s) error: %v", tt.input, err)
			}
			if answer == nil {
				t.Fatal("Handle returned nil answer")
			}
			if answer.Content == "" {
				t.Error("Answer content is empty")
			}
		})
	}
}

func TestLoremHandler(t *testing.T) {
	h := NewLoremHandler()

	ctx := context.Background()

	tests := []struct {
		input string
	}{
		{"1"},
		{"3"},
		{"5"},
		{"words:20"},
		{"sentences:5"},
	}

	for _, tt := range tests {
		t.Run("lorem_"+tt.input, func(t *testing.T) {
			answer, err := h.Handle(ctx, tt.input)
			if err != nil {
				t.Fatalf("Handle(%s) error: %v", tt.input, err)
			}
			if answer == nil {
				t.Fatal("Handle returned nil answer")
			}
			if answer.Content == "" {
				t.Error("Answer content is empty")
			}
		})
	}
}

func TestMIMEHandler(t *testing.T) {
	h := NewMIMEHandler()

	ctx := context.Background()

	tests := []struct {
		input string
	}{
		{"json"},
		{"html"},
		{"pdf"},
		{"png"},
		{"mp4"},
		{"application/json"},
	}

	for _, tt := range tests {
		t.Run("mime_"+tt.input, func(t *testing.T) {
			answer, err := h.Handle(ctx, tt.input)
			if err != nil {
				t.Fatalf("Handle(%s) error: %v", tt.input, err)
			}
			if answer == nil {
				t.Fatal("Handle returned nil answer")
			}
			if answer.Content == "" {
				t.Error("Answer content is empty")
			}
		})
	}
}

func TestLicenseHandler(t *testing.T) {
	h := NewLicenseHandler()

	ctx := context.Background()

	tests := []struct {
		input string
	}{
		{"MIT"},
		{"Apache-2.0"},
		{"GPL-3.0"},
		{"BSD-3-Clause"},
		{"ISC"},
	}

	for _, tt := range tests {
		t.Run("license_"+tt.input, func(t *testing.T) {
			answer, err := h.Handle(ctx, tt.input)
			if err != nil {
				t.Fatalf("Handle(%s) error: %v", tt.input, err)
			}
			if answer == nil {
				t.Fatal("Handle returned nil answer")
			}
			if answer.Content == "" {
				t.Error("Answer content is empty")
			}
		})
	}
}

func TestTimestampHandler(t *testing.T) {
	h := NewTimestampHandler()

	ctx := context.Background()

	tests := []struct {
		input string
	}{
		{"now"},
		{"1704067200"},
		{"2024-01-01"},
		{"2024-01-01T00:00:00Z"},
	}

	for _, tt := range tests {
		t.Run("timestamp_"+tt.input, func(t *testing.T) {
			answer, err := h.Handle(ctx, tt.input)
			if err != nil {
				t.Fatalf("Handle(%s) error: %v", tt.input, err)
			}
			if answer == nil {
				t.Fatal("Handle returned nil answer")
			}
			if answer.Content == "" {
				t.Error("Answer content is empty")
			}
		})
	}
}

func TestProcessUnknownType(t *testing.T) {
	m := NewManager()

	ctx := context.Background()

	// Test processing unknown type directly
	answer, err := m.ProcessType(ctx, "unknowntype", "term")
	if err != nil {
		t.Fatalf("ProcessType error: %v", err)
	}
	if answer == nil {
		t.Fatal("ProcessType returned nil answer")
	}
	if answer.Error != "unknown_type" {
		t.Errorf("Expected error 'unknown_type', got: %s", answer.Error)
	}
}
