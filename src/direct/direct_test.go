package direct

import (
	"context"
	"encoding/json"
	"strings"
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
		{"rule:34", AnswerTypeRules, "34"},
		{"rule: all", AnswerTypeRules, "all"},
		{"rule:", AnswerTypeRules, ""},
		{"rules:", AnswerTypeRules, ""},
		{"roti:34", AnswerTypeRules, "34"},
		{"roti: all", AnswerTypeRules, "all"},
		{"roti:", AnswerTypeRules, ""},
		{"", "", ""},
		{"notacommand", "", ""},
		{"invalid:", "", ""},
		// unknown type should not match
		{"unknown:term", "", ""},
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
		{"rule:33", true},
		{"rule:", true},
		{"roti:33", true},
		{"roti:", true},
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

func TestRulesAliasProcess(t *testing.T) {
	m := NewManager()
	ctx := context.Background()

	answer, err := m.Process(ctx, "rule:34")
	if err != nil {
		t.Fatalf("Process(rule:34) error: %v", err)
	}
	if answer == nil {
		t.Fatal("Process(rule:34) returned nil answer")
	}
	if answer.Type != AnswerTypeRules {
		t.Fatalf("answer type = %v, want %v", answer.Type, AnswerTypeRules)
	}
	if answer.Title != "Rule 34 of the Internet" {
		t.Fatalf("answer title = %q, want %q", answer.Title, "Rule 34 of the Internet")
	}

	answer, err = m.Process(ctx, "rule:")
	if err != nil {
		t.Fatalf("Process(rule:) error: %v", err)
	}
	if answer == nil {
		t.Fatal("Process(rule:) returned nil answer")
	}
	if answer.Title != "Rules of the Internet" {
		t.Fatalf("answer title = %q, want %q", answer.Title, "Rules of the Internet")
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
		// Static, no cache
		{AnswerTypeHTTP, 0, 0},
		// Static, no cache
		{AnswerTypeChmod, 0, 0},
		// Never cache
		{AnswerTypeCache, 0, 0},
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
			answer, err := h.HandleDirectQuery(ctx, code)
			if err != nil {
				t.Fatalf("HandleDirectQuery(%s) error: %v", code, err)
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
		answer, err := h.HandleDirectQuery(ctx, "invalid")
		if err != nil {
			t.Fatalf("HandleDirectQuery(invalid) error: %v", err)
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
		// Port out of range or unknown
		{"99999", false},
	}

	for _, tt := range tests {
		t.Run("port_"+tt.port, func(t *testing.T) {
			answer, err := h.HandleDirectQuery(ctx, tt.port)
			if err != nil {
				t.Fatalf("HandleDirectQuery(%s) error: %v", tt.port, err)
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
			answer, err := h.HandleDirectQuery(ctx, tt.input)
			if err != nil {
				t.Fatalf("HandleDirectQuery(%s) error: %v", tt.input, err)
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
			answer, err := h.HandleDirectQuery(ctx, tt.expr)
			if err != nil {
				t.Fatalf("HandleDirectQuery(%s) error: %v", tt.expr, err)
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
			answer, err := h.HandleDirectQuery(ctx, tt.cidr)
			if err != nil {
				t.Fatalf("HandleDirectQuery(%s) error: %v", tt.cidr, err)
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
			answer, err := h.HandleDirectQuery(ctx, tt.input)
			if err != nil {
				t.Fatalf("HandleDirectQuery(%s) error: %v", tt.input, err)
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
			answer, err := h.HandleDirectQuery(ctx, tt.input)
			if err != nil {
				t.Fatalf("HandleDirectQuery(%s) error: %v", tt.input, err)
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
			answer, err := h.HandleDirectQuery(ctx, tt.input)
			if err != nil {
				t.Fatalf("HandleDirectQuery(%s) error: %v", tt.input, err)
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
			answer, err := h.HandleDirectQuery(ctx, tt.input)
			if err != nil {
				t.Fatalf("HandleDirectQuery(%s) error: %v", tt.input, err)
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
			answer, err := h.HandleDirectQuery(ctx, tt.input)
			if err != nil {
				t.Fatalf("HandleDirectQuery(%s) error: %v", tt.input, err)
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
			answer, err := h.HandleDirectQuery(ctx, tt.input)
			if err != nil {
				t.Fatalf("HandleDirectQuery(%s) error: %v", tt.input, err)
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

// --- Manager accessor methods ---

func TestManagerGetHandlerTypes(t *testing.T) {
	m := NewManager()
	types := m.GetHandlerTypes()
	if len(types) == 0 {
		t.Fatal("GetHandlerTypes returned empty slice")
	}
	// Every returned type must resolve in GetHandler
	for _, at := range types {
		if _, ok := m.GetHandler(at); !ok {
			t.Errorf("GetHandler(%s) returned false even though type was in GetHandlerTypes", at)
		}
	}
}

func TestManagerGetHandler(t *testing.T) {
	m := NewManager()
	tests := []struct {
		at   AnswerType
		want bool
	}{
		{AnswerTypeHTTP, true},
		{AnswerTypeChmod, true},
		{AnswerTypeCron, true},
		{AnswerType("nonexistent_type"), false},
	}
	for _, tt := range tests {
		t.Run(string(tt.at), func(t *testing.T) {
			_, ok := m.GetHandler(tt.at)
			if ok != tt.want {
				t.Errorf("GetHandler(%s) ok = %v, want %v", tt.at, ok, tt.want)
			}
		})
	}
}

// --- encoding.go helpers ---

func TestHTMLEncodeDecode(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		encoded string
		// roundTrip is what htmlDecode(htmlEncode(input)) should return.
		// Empty string means the round-trip should equal input.
		roundTrip string
	}{
		{"ampersand", "&", "&amp;", ""},
		{"less_than", "<", "&lt;", ""},
		{"greater_than", ">", "&gt;", ""},
		{"double_quote", `"`, "&quot;", ""},
		{"single_quote", "'", "&#39;", ""},
		// \x80 is invalid UTF-8; Go replaces it with U+FFFD (&#65533;) when iterating.
		// The round-trip cannot recover the original invalid byte.
		{"non_ascii", "\x80", "&#65533;", "�"},
		{"plain_text", "hello", "hello", ""},
		{"empty", "", "", ""},
		{"combined", "<b>\"hello\"</b>", "&lt;b&gt;&quot;hello&quot;&lt;/b&gt;", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := htmlEncode(tt.input)
			if got != tt.encoded {
				t.Errorf("htmlEncode(%q) = %q, want %q", tt.input, got, tt.encoded)
			}
			// Round-trip: decode the encoded form and expect original (or roundTrip if set)
			wantDecoded := tt.input
			if tt.roundTrip != "" {
				wantDecoded = tt.roundTrip
			}
			decoded := htmlDecode(got)
			if decoded != wantDecoded {
				t.Errorf("htmlDecode(htmlEncode(%q)) = %q, want %q", tt.input, decoded, wantDecoded)
			}
		})
	}
}

func TestHTMLDecodeEntities(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"named_amp", "&amp;", "&"},
		{"named_lt", "&lt;", "<"},
		{"named_gt", "&gt;", ">"},
		{"named_quot", "&quot;", `"`},
		{"decimal", "&#65;", "A"},
		{"hex_lower", "&#x41;", "A"},
		{"hex_upper", "&#X41;", "A"},
		{"unknown_entity", "&unknown;", "&unknown;"},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := htmlDecode(tt.input)
			if got != tt.want {
				t.Errorf("htmlDecode(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestGetUnicodeCategory(t *testing.T) {
	tests := []struct {
		name string
		r    rune
		want string
	}{
		{"letter_a", 'a', "Letter"},
		{"letter_Z", 'Z', "Letter"},
		{"digit_0", '0', "Digit"},
		{"digit_9", '9', "Digit"},
		{"punct_dot", '.', "Punctuation"},
		{"punct_comma", ',', "Punctuation"},
		{"symbol_plus", '+', "Symbol"},
		{"symbol_equals", '=', "Symbol"},
		{"space", ' ', "Space"},
		{"tab", '\t', "Space"},
		{"control_null", '\x00', "Control"},
		{"control_del", '\x7f', "Control"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getUnicodeCategory(tt.r)
			if got != tt.want {
				t.Errorf("getUnicodeCategory(%q) = %q, want %q", tt.r, got, tt.want)
			}
		})
	}
}

func TestCountJSONKeys(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  int
	}{
		{"empty_object", `{}`, 0},
		{"flat_object", `{"a":1,"b":2}`, 2},
		{"nested_object", `{"a":{"c":3},"b":2}`, 3},
		{"array_of_objects", `[{"a":1},{"b":2}]`, 2},
		{"scalar_string", `"hello"`, 0},
		{"scalar_number", `42`, 0},
		{"null_value", `null`, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var v interface{}
			if err := json.Unmarshal([]byte(tt.input), &v); err != nil {
				t.Fatalf("json.Unmarshal: %v", err)
			}
			got := countJSONKeys(v)
			if got != tt.want {
				t.Errorf("countJSONKeys(%s) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestGetJSONDepth(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  int
	}{
		{"scalar", `42`, 0},
		{"null", `null`, 0},
		{"flat_object", `{"a":1}`, 1},
		{"flat_array", `[1,2,3]`, 1},
		{"nested_2", `{"a":{"b":1}}`, 2},
		{"nested_3", `{"a":{"b":{"c":1}}}`, 3},
		{"empty_object", `{}`, 1},
		{"empty_array", `[]`, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var v interface{}
			if err := json.Unmarshal([]byte(tt.input), &v); err != nil {
				t.Fatalf("json.Unmarshal: %v", err)
			}
			got := getJSONDepth(v)
			if got != tt.want {
				t.Errorf("getJSONDepth(%s) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestEscapeHelpers(t *testing.T) {
	t.Run("escapeJSON_string", func(t *testing.T) {
		got := escapeJSON("hello world")
		if got == "" {
			t.Error("escapeJSON returned empty string")
		}
		// Result must be valid JSON string
		var s string
		if err := json.Unmarshal([]byte(got), &s); err != nil {
			t.Errorf("escapeJSON output is not valid JSON string: %v (output=%q)", err, got)
		}
	})
	t.Run("escapeSQL_basic", func(t *testing.T) {
		got := escapeSQL("O'Brien")
		if !strings.Contains(got, "''") {
			t.Errorf("escapeSQL did not double single quotes: %q", got)
		}
		if got[0] != '\'' || got[len(got)-1] != '\'' {
			t.Errorf("escapeSQL output not wrapped in single quotes: %q", got)
		}
	})
	t.Run("escapeShell_basic", func(t *testing.T) {
		got := escapeShell("hello world")
		if got[0] != '\'' || got[len(got)-1] != '\'' {
			t.Errorf("escapeShell output not wrapped in single quotes: %q", got)
		}
	})
	t.Run("escapeJS_basic", func(t *testing.T) {
		got := escapeJS(`say "hi"`)
		if !strings.Contains(got, `\"`) {
			t.Errorf("escapeJS did not escape double quotes: %q", got)
		}
	})
}

func TestFormatYAMLMode(t *testing.T) {
	tests := []struct {
		mode string
		want string
	}{
		{"to-json", "to JSON"},
		{"from-json", "from JSON"},
		{"anything-else", "Format"},
		{"", "Format"},
	}
	for _, tt := range tests {
		t.Run(tt.mode, func(t *testing.T) {
			got := formatYAMLMode(tt.mode)
			if got != tt.want {
				t.Errorf("formatYAMLMode(%q) = %q, want %q", tt.mode, got, tt.want)
			}
		})
	}
}

// --- HTMLHandler via Manager ---

func TestHTMLHandlerViaManager(t *testing.T) {
	m := NewManager()
	ctx := context.Background()

	tests := []struct {
		name      string
		term      string
		wantError string
	}{
		{"encode_basic", "html:<b>hello</b>", ""},
		{"decode_entities", "html:&amp;&lt;", ""},
		{"auto_decode", "html:&amp;#65;", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			answer, err := m.Process(ctx, tt.term)
			if err != nil {
				t.Fatalf("Process(%q) error: %v", tt.term, err)
			}
			if answer == nil {
				t.Fatal("Process returned nil")
			}
			if tt.wantError != "" && answer.Error != tt.wantError {
				t.Errorf("answer.Error = %q, want %q", answer.Error, tt.wantError)
			}
		})
	}
}

func TestHTMLHandlerDirect(t *testing.T) {
	h := NewHTMLHandler()
	ctx := context.Background()

	tests := []struct {
		name      string
		term      string
		wantError string
		wantErr   bool
	}{
		{"encode_ampersand", "<p>hi & there</p>", "", false},
		{"decode_named_entity", "&lt;b&gt;", "", false},
		// empty term returns an error — no answer produced
		{"empty", "", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			answer, err := h.HandleDirectQuery(ctx, tt.term)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got none")
				}
				return
			}
			if err != nil {
				t.Fatalf("error: %v", err)
			}
			if answer == nil {
				t.Fatal("nil answer")
			}
			if tt.wantError != "" && answer.Error != tt.wantError {
				t.Errorf("answer.Error = %q, want %q", answer.Error, tt.wantError)
			}
			if tt.term != "" && answer.Content == "" {
				t.Error("Content must not be empty for non-empty input")
			}
		})
	}
}

// --- UnicodeHandler ---

func TestUnicodeHandlerDirect(t *testing.T) {
	h := NewUnicodeHandler()
	ctx := context.Background()

	tests := []struct {
		name      string
		term      string
		wantError string
	}{
		{"valid_hex", "U+0041", ""},
		{"valid_decimal", "65", ""},
		{"letter_A", "A", ""},
		{"invalid_codepoint", "U+ZZZZ", "invalid_input"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			answer, err := h.HandleDirectQuery(ctx, tt.term)
			if err != nil {
				t.Fatalf("error: %v", err)
			}
			if answer == nil {
				t.Fatal("nil answer")
			}
			if tt.wantError != "" && answer.Error != tt.wantError {
				t.Errorf("answer.Error = %q, want %q", answer.Error, tt.wantError)
			}
		})
	}
}

// --- EmojiHandler ---

func TestEmojiHandlerDirect(t *testing.T) {
	h := NewEmojiHandler()
	ctx := context.Background()

	tests := []struct {
		name    string
		term    string
		wantErr bool
	}{
		{"smile", "smile", false},
		{"heart", "heart", false},
		{"fire", "fire", false},
		{"single_emoji", "😀", false},
		// empty term returns an error — "emoji name or keyword required"
		{"empty", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			answer, err := h.HandleDirectQuery(ctx, tt.term)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got none")
				}
				return
			}
			if err != nil {
				t.Fatalf("error: %v", err)
			}
			if answer == nil {
				t.Fatal("nil answer for non-empty term")
			}
		})
	}
}

// --- EscapeHandler ---

func TestEscapeHandlerDirect(t *testing.T) {
	h := NewEscapeHandler()
	ctx := context.Background()

	tests := []struct {
		name      string
		term      string
		wantError string
	}{
		{"json_string", `{"key":"value"}`, ""},
		{"sql_apostrophe", "O'Brien", ""},
		{"shell_spaces", "hello world", ""},
		{"js_quotes", `say "hi"`, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			answer, err := h.HandleDirectQuery(ctx, tt.term)
			if err != nil {
				t.Fatalf("error: %v", err)
			}
			if answer == nil {
				t.Fatal("nil answer")
			}
			if answer.Content == "" {
				t.Error("Content must not be empty")
			}
		})
	}
}

// --- JSONHandler ---

func TestJSONHandlerDirect(t *testing.T) {
	h := NewJSONHandler()
	ctx := context.Background()

	tests := []struct {
		name      string
		term      string
		wantError string
		wantErr   bool
	}{
		{"valid_object", `{"key":"value"}`, "", false},
		{"valid_array", `[1,2,3]`, "", false},
		{"invalid_json", `{bad json`, "invalid_json", false},
		// empty term returns an error — "JSON data required"
		{"empty", "", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			answer, err := h.HandleDirectQuery(ctx, tt.term)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got none")
				}
				return
			}
			if err != nil {
				t.Fatalf("error: %v", err)
			}
			if answer == nil {
				t.Fatal("nil answer")
			}
			if tt.wantError != "" && answer.Error != tt.wantError {
				t.Errorf("answer.Error = %q, want %q", answer.Error, tt.wantError)
			}
			if tt.wantError == "" && answer.Content == "" {
				t.Error("Content must not be empty for valid JSON")
			}
		})
	}
}

// --- YAMLHandler ---

func TestYAMLHandlerDirect(t *testing.T) {
	h := NewYAMLHandler()
	ctx := context.Background()

	tests := []struct {
		name      string
		term      string
		wantError string
	}{
		{"valid_yaml", "key: value\nfoo: bar", ""},
		{"invalid_yaml", "key: :\n  bad:", "invalid_yaml"},
		{"to_json_mode", `{"convert":"to-json","content":"key: value"}`, ""},
		{"from_json_mode", `{"convert":"from-json","content":"{\"key\":\"value\"}"}`, ""},
		// This JSON object is valid YAML (YAML is a superset of JSON); no error is expected.
		{"invalid_json_content_for_from_json", `{"convert":"from-json","content":"{bad"}`, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			answer, err := h.HandleDirectQuery(ctx, tt.term)
			if err != nil {
				t.Fatalf("error: %v", err)
			}
			if answer == nil {
				t.Fatal("nil answer")
			}
			if tt.wantError != "" && answer.Error != tt.wantError {
				t.Errorf("answer.Error = %q, want %q", answer.Error, tt.wantError)
			}
		})
	}
}

// --- UserAgentHandler ---

func TestUserAgentHandlerDirect(t *testing.T) {
	h := NewUserAgentHandler()
	ctx := context.Background()

	tests := []struct {
		name        string
		term        string
		wantBrowser string
		wantBot     string
	}{
		{
			"chrome_windows",
			"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
			"Chrome",
			"No",
		},
		{
			"firefox_linux",
			"Mozilla/5.0 (X11; Linux x86_64; rv:109.0) Gecko/20100101 Firefox/115.0",
			"Firefox",
			"No",
		},
		{
			"googlebot",
			"Googlebot/2.1 (+http://www.google.com/bot.html)",
			"",
			"Yes",
		},
		{
			"curl_agent",
			"curl/7.88.0",
			"",
			"Yes",
		},
		{
			"safari_macos",
			"Mozilla/5.0 (Macintosh; Intel Mac OS X 13_0) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/16.0 Safari/605.1.15",
			"Safari",
			"No",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			answer, err := h.HandleDirectQuery(ctx, tt.term)
			if err != nil {
				t.Fatalf("error: %v", err)
			}
			if answer == nil {
				t.Fatal("nil answer")
			}
			if answer.Content == "" {
				t.Error("Content must not be empty")
			}
			parsed, ok := answer.Data["parsed"].(map[string]string)
			if !ok {
				t.Fatalf("Data[parsed] is not map[string]string: %T", answer.Data["parsed"])
			}
			if tt.wantBrowser != "" && parsed["browser"] != tt.wantBrowser {
				t.Errorf("browser = %q, want %q", parsed["browser"], tt.wantBrowser)
			}
			if parsed["isBot"] != tt.wantBot {
				t.Errorf("isBot = %q, want %q", parsed["isBot"], tt.wantBot)
			}
		})
	}
}

func TestUserAgentHandlerEmptyUsesDefault(t *testing.T) {
	h := NewUserAgentHandler()
	ctx := context.Background()

	for _, term := range []string{"", "my", "MY", "My"} {
		t.Run("term_"+term, func(t *testing.T) {
			answer, err := h.HandleDirectQuery(ctx, term)
			if err != nil {
				t.Fatalf("error: %v", err)
			}
			if answer == nil {
				t.Fatal("nil answer")
			}
			// Should not return an error answer for empty/my
			if answer.Error != "" {
				t.Errorf("unexpected error %q for term %q", answer.Error, term)
			}
		})
	}
}

func TestParseUserAgent(t *testing.T) {
	tests := []struct {
		name    string
		ua      string
		browser string
		os      string
		isBot   string
		arch    string
	}{
		{
			"windows_10_x64",
			"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/120.0.0.0",
			"Chrome",
			"Windows 11",
			"No",
			"64-bit",
		},
		{
			"arm_device",
			"Mozilla/5.0 (Linux; arm_64; Android 13) AppleWebKit/537.36",
			"",
			"Android",
			"No",
			"ARM",
		},
		{
			"spider",
			"SomeSpider/1.0 (compatible; BotCrawler)",
			"",
			"",
			"Yes",
			"",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseUserAgent(tt.ua)
			if tt.browser != "" && got["browser"] != tt.browser {
				t.Errorf("browser = %q, want %q", got["browser"], tt.browser)
			}
			if tt.os != "" && got["os"] != tt.os {
				t.Errorf("os = %q, want %q", got["os"], tt.os)
			}
			if got["isBot"] != tt.isBot {
				t.Errorf("isBot = %q, want %q", got["isBot"], tt.isBot)
			}
			if tt.arch != "" && got["architecture"] != tt.arch {
				t.Errorf("architecture = %q, want %q", got["architecture"], tt.arch)
			}
		})
	}
}

// --- ASCIIHandler ---

func TestASCIIHandlerDirect(t *testing.T) {
	h := NewASCIIHandler()
	ctx := context.Background()

	tests := []struct {
		name    string
		term    string
		wantErr bool
	}{
		{"simple_word", "hello", false},
		{"single_char", "A", false},
		{"empty", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			answer, err := h.HandleDirectQuery(ctx, tt.term)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if answer == nil {
				t.Fatal("nil answer")
			}
			if answer.Content == "" {
				t.Error("Content must not be empty")
			}
		})
	}
}

// --- QRHandler ---

func TestQRHandlerDirect(t *testing.T) {
	h := NewQRHandler()
	ctx := context.Background()

	tests := []struct {
		name    string
		term    string
		wantErr bool
	}{
		{"url", "https://example.com", false},
		{"text", "Hello, World!", false},
		{"empty", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			answer, err := h.HandleDirectQuery(ctx, tt.term)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if answer == nil {
				t.Fatal("nil answer")
			}
			if answer.Content == "" {
				t.Error("Content must not be empty")
			}
		})
	}
}

// --- HTTPHandler edge cases ---

func TestHTTPHandlerEdgeCases(t *testing.T) {
	h := NewHTTPHandler()
	ctx := context.Background()

	tests := []struct {
		name      string
		term      string
		wantErr   bool
		wantError string
	}{
		{"unknown_code_999", "999", false, "unknown_code"},
		{"not_a_number", "abc", false, "invalid_code"},
		// strconv.Atoi("-1") succeeds; -1 is not in the status map → unknown_code
		{"negative", "-1", false, "unknown_code"},
		// empty input returns (nil, error) — not an Answer with an error field
		{"empty", "", true, ""},
		{"200_ok", "200", false, ""},
		{"301_redirect", "301", false, ""},
		{"400_bad_request", "400", false, ""},
		{"503_service_unavailable", "503", false, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			answer, err := h.HandleDirectQuery(ctx, tt.term)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if answer == nil {
				t.Fatal("nil answer")
			}
			if tt.wantError != "" && answer.Error != tt.wantError {
				t.Errorf("answer.Error = %q, want %q", answer.Error, tt.wantError)
			}
			if tt.wantError == "" && answer.Error != "" {
				t.Errorf("unexpected error %q", answer.Error)
			}
		})
	}
}

// --- PortHandler edge cases ---

func TestPortHandlerEdgeCases(t *testing.T) {
	h := NewPortHandler()
	ctx := context.Background()

	tests := []struct {
		name      string
		term      string
		wantErr   bool
		wantError string
	}{
		{"port_zero", "0", false, ""},
		{"port_max", "65535", false, ""},
		{"out_of_range", "99999", false, "invalid_port"},
		{"negative", "-1", false, "invalid_port"},
		{"not_a_number", "abc", false, "invalid_port"},
		// empty input returns (nil, error) — not an Answer with an error field
		{"empty", "", true, ""},
		{"http_80", "80", false, ""},
		{"https_443", "443", false, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			answer, err := h.HandleDirectQuery(ctx, tt.term)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if answer == nil {
				t.Fatal("nil answer")
			}
			if tt.wantError != "" && answer.Error != tt.wantError {
				t.Errorf("answer.Error = %q, want %q", answer.Error, tt.wantError)
			}
		})
	}
}

// --- ChmodHandler edge cases ---

func TestChmodHandlerEdgeCases(t *testing.T) {
	h := NewChmodHandler()
	ctx := context.Background()

	tests := []struct {
		name      string
		term      string
		wantErr   bool
		wantError string
	}{
		{"octal_3digit", "755", false, ""},
		{"octal_4digit", "0755", false, ""},
		{"octal_644", "644", false, ""},
		{"octal_777", "777", false, ""},
		{"symbolic_9char", "rwxr-xr-x", false, ""},
		{"symbolic_rw", "rw-r--r--", false, ""},
		{"invalid_octal_999", "999", false, "invalid_permissions"},
		{"invalid_text", "badperm", false, "invalid_permissions"},
		// empty input returns (nil, error) — not an Answer with an error field
		{"empty", "", true, ""},
		{"too_short_symbolic", "rwx", false, "invalid_permissions"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			answer, err := h.HandleDirectQuery(ctx, tt.term)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if answer == nil {
				t.Fatal("nil answer")
			}
			if tt.wantError != "" && answer.Error != tt.wantError {
				t.Errorf("answer.Error = %q, want %q", answer.Error, tt.wantError)
			}
			if tt.wantError == "" && answer.Error != "" {
				t.Errorf("unexpected error %q", answer.Error)
			}
		})
	}
}

func TestPermToSymbolic(t *testing.T) {
	tests := []struct {
		perm int
		want string
	}{
		{0, "---"},
		{1, "--x"},
		{2, "-w-"},
		{3, "-wx"},
		{4, "r--"},
		{5, "r-x"},
		{6, "rw-"},
		{7, "rwx"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := permToSymbolic(tt.perm)
			if got != tt.want {
				t.Errorf("permToSymbolic(%d) = %q, want %q", tt.perm, got, tt.want)
			}
		})
	}
}

func TestSymbolicToPerm(t *testing.T) {
	tests := []struct {
		sym  string
		want int
	}{
		{"---", 0},
		{"--x", 1},
		{"-w-", 2},
		{"-wx", 3},
		{"r--", 4},
		{"r-x", 5},
		{"rw-", 6},
		{"rwx", 7},
	}
	for _, tt := range tests {
		t.Run(tt.sym, func(t *testing.T) {
			got := symbolicToPerm(tt.sym)
			if got != tt.want {
				t.Errorf("symbolicToPerm(%q) = %d, want %d", tt.sym, got, tt.want)
			}
		})
	}
}

// --- CronHandler edge cases ---

func TestCronHandlerEdgeCases(t *testing.T) {
	h := NewCronHandler()
	ctx := context.Background()

	tests := []struct {
		name      string
		term      string
		wantErr   bool
		wantError string
	}{
		{"every_minute", "* * * * *", false, ""},
		{"every_hour", "0 * * * *", false, ""},
		{"daily_midnight", "0 0 * * *", false, ""},
		{"monthly_first", "0 0 1 * *", false, ""},
		{"complex", "30 4 1,15 * 5", false, ""},
		{"six_field", "0 * * * * *", false, ""},
		{"too_few_fields", "* * *", false, "invalid_cron"},
		// empty input returns (nil, error) — not an Answer with an error field
		{"empty", "", true, ""},
		{"at_style_invalid", "@daily", false, "invalid_cron"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			answer, err := h.HandleDirectQuery(ctx, tt.term)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if answer == nil {
				t.Fatal("nil answer")
			}
			if tt.wantError != "" && answer.Error != tt.wantError {
				t.Errorf("answer.Error = %q, want %q", answer.Error, tt.wantError)
			}
			if tt.wantError == "" && answer.Error != "" {
				t.Errorf("unexpected error %q for term %q", answer.Error, tt.term)
			}
		})
	}
}

// --- RegexHandler ---

func TestRegexHandler(t *testing.T) {
	h := NewRegexHandler()
	ctx := context.Background()

	tests := []struct {
		name         string
		term         string
		wantError    string
		wantAnalysis []string
	}{
		{
			"valid_anchored",
			`^\d+$`,
			"",
			[]string{"Anchored to start of string", "Anchored to end of string", "Matches digits (\\d)"},
		},
		{
			"greedy_wildcard",
			`foo.*bar`,
			"",
			[]string{"Contains greedy wildcard (.*)"},
		},
		{
			"literal",
			`hello`,
			"",
			[]string{"Simple literal pattern"},
		},
		{
			"invalid_regex",
			`[unclosed`,
			"invalid_regex",
			nil,
		},
		{
			"capture_group",
			`(abc)+`,
			"",
			[]string{"Contains capture group(s)"},
		},
		{
			"char_class",
			`[a-z]+`,
			"",
			[]string{"Contains character class"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			answer, err := h.HandleDirectQuery(ctx, tt.term)
			if err != nil {
				t.Fatalf("error: %v", err)
			}
			if answer == nil {
				t.Fatal("nil answer")
			}
			if tt.wantError != "" {
				if answer.Error != tt.wantError {
					t.Errorf("answer.Error = %q, want %q", answer.Error, tt.wantError)
				}
				return
			}
			if answer.Error != "" {
				t.Errorf("unexpected error %q", answer.Error)
			}
			analysis, ok := answer.Data["analysis"].([]string)
			if !ok {
				t.Fatalf("Data[analysis] is not []string: %T", answer.Data["analysis"])
			}
			for _, want := range tt.wantAnalysis {
				found := false
				for _, got := range analysis {
					if got == want {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("analysis missing %q; got: %v", want, analysis)
				}
			}
		})
	}
}

// --- JWTHandler ---

func TestJWTHandlerDirect(t *testing.T) {
	h := NewJWTHandler()
	ctx := context.Background()

	// Build a minimal valid JWT (no real signature — just structural validity)
	// header: {"alg":"HS256","typ":"JWT"} → base64url
	// payload: {"sub":"1234567890","name":"Test","iat":1516239022}
	validHeader := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9"
	validPayload := "eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IlRlc3QiLCJpYXQiOjE1MTYyMzkwMjJ9"
	validSig := "SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c"
	validJWT := validHeader + "." + validPayload + "." + validSig

	// Expired JWT: payload has exp in the past
	expiredPayload := "eyJzdWIiOiIxMjM0IiwiZXhwIjoxMDAwMDAwMDAwfQ" // exp: 2001
	expiredJWT := validHeader + "." + expiredPayload + "." + validSig

	tests := []struct {
		name        string
		term        string
		wantError   string
		wantExpired bool
	}{
		{"valid_jwt", validJWT, "", false},
		{"expired_jwt", expiredJWT, "", true},
		{"two_parts_only", "aaa.bbb", "invalid_jwt", false},
		{"one_part", "aaa", "invalid_jwt", false},
		{"bad_header_base64", "!!!.payload.sig", "invalid_header", false},
		{"bad_payload_base64", validHeader + ".!!!.sig", "invalid_payload", false},
		{"empty", "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			answer, err := h.HandleDirectQuery(ctx, tt.term)
			if tt.term == "" {
				// Empty returns Go error, not Answer error
				if err == nil {
					t.Error("expected error for empty term")
				}
				return
			}
			if err != nil {
				t.Fatalf("error: %v", err)
			}
			if answer == nil {
				t.Fatal("nil answer")
			}
			if tt.wantError != "" {
				if answer.Error != tt.wantError {
					t.Errorf("answer.Error = %q, want %q", answer.Error, tt.wantError)
				}
				return
			}
			if answer.Error != "" {
				t.Errorf("unexpected error %q", answer.Error)
			}
			expired, _ := answer.Data["expired"].(bool)
			if expired != tt.wantExpired {
				t.Errorf("expired = %v, want %v", expired, tt.wantExpired)
			}
		})
	}
}

func TestBase64URLDecode(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{"standard", "aGVsbG8", "hello", false},
		{"with_padding", "aGVsbG8=", "hello", false},
		{"two_pad", "aGVs", "hel", false},
		{"empty", "", "", false},
		{"invalid", "!!!!", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := base64URLDecode(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("base64URLDecode(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if !tt.wantErr && string(got) != tt.want {
				t.Errorf("base64URLDecode(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// --- TimestampHandler edge cases ---

func TestTimestampHandlerEdgeCases(t *testing.T) {
	h := NewTimestampHandler()
	ctx := context.Background()

	tests := []struct {
		name      string
		term      string
		wantError string
	}{
		{"now", "now", ""},
		{"NOW_uppercase", "NOW", ""},
		{"unix_seconds", "1704067200", ""},
		{"unix_millis", "1704067200000", ""},
		{"iso8601", "2024-01-01T00:00:00Z", ""},
		{"date_only", "2024-01-01", ""},
		{"invalid_string", "notadate", "invalid_timestamp"},
		{"negative_number", "-1", ""},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			answer, err := h.HandleDirectQuery(ctx, tt.term)
			if tt.term == "" {
				// Empty returns Go error
				if err == nil {
					t.Error("expected error for empty term")
				}
				return
			}
			if err != nil {
				t.Fatalf("error: %v", err)
			}
			if answer == nil {
				t.Fatal("nil answer")
			}
			if tt.wantError != "" && answer.Error != tt.wantError {
				t.Errorf("answer.Error = %q, want %q", answer.Error, tt.wantError)
			}
			if tt.wantError == "" && answer.Error != "" {
				t.Errorf("unexpected error %q for term %q", answer.Error, tt.term)
			}
		})
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		name string
		d    time.Duration
		want string
	}{
		{"30_seconds", 30 * time.Second, "30 seconds"},
		{"90_seconds", 90 * time.Second, "1 minutes"},
		{"3_hours", 3 * time.Hour, "3 hours"},
		{"2_days", 48 * time.Hour, "2 days"},
		{"40_days", 40 * 24 * time.Hour, "1 months"},
		{"2_years", 2 * 365 * 24 * time.Hour, "2 years"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatDuration(tt.d)
			if got != tt.want {
				t.Errorf("formatDuration(%v) = %q, want %q", tt.d, got, tt.want)
			}
		})
	}
}

func TestTruncateString(t *testing.T) {
	tests := []struct {
		name  string
		input string
		max   int
		want  string
	}{
		{"no_truncate", "hello", 10, "hello"},
		{"exact_length", "hello", 5, "hello"},
		{"truncate", "hello world", 8, "hello wo..."},
		{"empty", "", 5, ""},
		// truncateString("hello", 0): len("hello")=5 > 0, so "hello"[:0]+"..." = "..."
		{"max_0", "hello", 0, "..."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncateString(tt.input, tt.max)
			if got != tt.want {
				t.Errorf("truncateString(%q, %d) = %q, want %q", tt.input, tt.max, got, tt.want)
			}
		})
	}
}

// --- CaseHandler extended coverage ---

func TestCaseHandlerConversions(t *testing.T) {
	h := NewCaseHandler()
	ctx := context.Background()

	tests := []struct {
		input         string
		wantUPPERCASE string
		wantLowercase string
		wantCamelCase string
		wantSnakeCase string
		wantKebabCase string
	}{
		{"hello world", "HELLO WORLD", "hello world", "helloWorld", "hello_world", "hello-world"},
		{"HelloWorld", "HELLOWORLD", "helloworld", "helloWorld", "hello_world", "hello-world"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			answer, err := h.HandleDirectQuery(ctx, tt.input)
			if err != nil {
				t.Fatalf("error: %v", err)
			}
			if answer == nil {
				t.Fatal("nil answer")
			}
			convs, ok := answer.Data["conversions"].(map[string]string)
			if !ok {
				t.Fatalf("Data[conversions] is not map[string]string: %T", answer.Data["conversions"])
			}
			if convs["UPPERCASE"] != tt.wantUPPERCASE {
				t.Errorf("UPPERCASE = %q, want %q", convs["UPPERCASE"], tt.wantUPPERCASE)
			}
			if convs["lowercase"] != tt.wantLowercase {
				t.Errorf("lowercase = %q, want %q", convs["lowercase"], tt.wantLowercase)
			}
			if convs["camelCase"] != tt.wantCamelCase {
				t.Errorf("camelCase = %q, want %q", convs["camelCase"], tt.wantCamelCase)
			}
			if convs["snake_case"] != tt.wantSnakeCase {
				t.Errorf("snake_case = %q, want %q", convs["snake_case"], tt.wantSnakeCase)
			}
			if convs["kebab-case"] != tt.wantKebabCase {
				t.Errorf("kebab-case = %q, want %q", convs["kebab-case"], tt.wantKebabCase)
			}
		})
	}
}

func TestCaseConversionHelpers(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
		fn    func(string) string
	}{
		{"snake_hello_world", "hello world", "hello_world", toSnakeCase},
		{"kebab_hello_world", "hello world", "hello-world", toKebabCase},
		{"camel_hello_world", "hello world", "helloWorld", toCamelCase},
		{"pascal_hello_world", "hello world", "HelloWorld", toPascalCase},
		{"screaming_hello_world", "hello world", "HELLO_WORLD", toScreamingSnake},
		{"dot_hello_world", "hello world", "hello.world", toDotCase},
		{"title_hello_world", "hello world", "Hello World", toTitleCase},
		{"sentence_hello_world", "hello world", "Hello world", toSentenceCase},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.fn(tt.input)
			if got != tt.want {
				t.Errorf("%q => %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// --- SlugHandler extended ---

func TestSlugHandlerData(t *testing.T) {
	h := NewSlugHandler()
	ctx := context.Background()

	answer, err := h.HandleDirectQuery(ctx, "Hello World")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if answer == nil {
		t.Fatal("nil answer")
	}
	slugs, ok := answer.Data["slugs"].(map[string]string)
	if !ok {
		t.Fatalf("Data[slugs] not map[string]string: %T", answer.Data["slugs"])
	}
	for _, key := range []string{"Standard (kebab)", "Underscore", "No separator"} {
		if _, exists := slugs[key]; !exists {
			t.Errorf("slugs missing key %q", key)
		}
	}
	if slugs["Standard (kebab)"] != "hello-world" {
		t.Errorf("Standard (kebab) = %q, want %q", slugs["Standard (kebab)"], "hello-world")
	}
	if slugs["Underscore"] != "hello_world" {
		t.Errorf("Underscore = %q, want %q", slugs["Underscore"], "hello_world")
	}
}

func TestGenerateSlug(t *testing.T) {
	tests := []struct {
		input     string
		separator string
		want      string
	}{
		{"Hello World", "-", "hello-world"},
		{"Hello World", "_", "hello_world"},
		{"Hello World", "", "helloworld"},
		{"Ärger über", "-", "aerger-ueber"},
		{"naïve café", "-", "naive-cafe"},
		{"Hello---World", "-", "hello-world"},
		{"special!@#chars", "-", "specialchars"},
		{"ß sharp s", "-", "ss-sharp-s"},
	}
	for _, tt := range tests {
		t.Run(tt.input+"_sep_"+tt.separator, func(t *testing.T) {
			got := generateSlug(tt.input, tt.separator)
			if got != tt.want {
				t.Errorf("generateSlug(%q, %q) = %q, want %q", tt.input, tt.separator, got, tt.want)
			}
		})
	}
}

// --- LoremHandler extended ---

func TestLoremHandlerExtended(t *testing.T) {
	h := NewLoremHandler()
	ctx := context.Background()

	tests := []struct {
		name     string
		term     string
		wantUnit string
	}{
		{"default_empty", "", "paragraphs"},
		{"paragraphs_3", "3", "paragraphs"},
		{"words_20", "20 words", "words"},
		{"sentences_5", "5 sentences", "sentences"},
		{"cap_at_100", "200", "paragraphs"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			answer, err := h.HandleDirectQuery(ctx, tt.term)
			if err != nil {
				t.Fatalf("error: %v", err)
			}
			if answer == nil {
				t.Fatal("nil answer")
			}
			if answer.Content == "" {
				t.Error("Content must not be empty")
			}
			unit, _ := answer.Data["unit"].(string)
			if unit != tt.wantUnit {
				t.Errorf("unit = %q, want %q", unit, tt.wantUnit)
			}
		})
	}
}

// --- WordHandler ---

func TestWordHandlerData(t *testing.T) {
	h := NewWordHandler()
	ctx := context.Background()

	tests := []struct {
		name    string
		term    string
		wantErr bool
	}{
		{"simple_sentence", "The quick brown fox jumps over the lazy dog.", false},
		{"single_word", "hello", false},
		{"empty_spaces", "   ", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			answer, err := h.HandleDirectQuery(ctx, tt.term)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got none")
				}
				return
			}
			if err != nil {
				t.Fatalf("error: %v", err)
			}
			if answer == nil {
				t.Fatal("nil answer")
			}
			_, hasWordCount := answer.Data["wordCount"]
			_, hasSentenceCount := answer.Data["sentenceCount"]
			if !hasWordCount {
				t.Error("Data missing wordCount")
			}
			if !hasSentenceCount {
				t.Error("Data missing sentenceCount")
			}
		})
	}
}

// --- BeautifyHandler ---

func TestBeautifyHandlerDirect(t *testing.T) {
	h := NewBeautifyHandler()
	ctx := context.Background()

	tests := []struct {
		name string
		term string
	}{
		{"json_oneliner", `{"key":"value","arr":[1,2,3]}`},
		{"html_compact", `<div><p>hello</p></div>`},
		{"css_compact", `body{margin:0;padding:0;}`},
		{"sql_lowercase", `select * from users where id=1`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			answer, err := h.HandleDirectQuery(ctx, tt.term)
			if err != nil {
				t.Fatalf("error: %v", err)
			}
			if answer == nil {
				t.Fatal("nil answer")
			}
			if answer.Content == "" {
				t.Error("Content must not be empty")
			}
		})
	}
}

func TestBeautifyCodeHelpers(t *testing.T) {
	t.Run("beautifyJSON_object", func(t *testing.T) {
		got := beautifyJSON(`{"a":1,"b":2}`)
		if !strings.Contains(got, "\n") {
			t.Error("beautifyJSON should add newlines")
		}
	})
	t.Run("beautifyCSS_rule", func(t *testing.T) {
		got := beautifyCSS(`body{margin:0;padding:0;}`)
		if !strings.Contains(got, "\n") {
			t.Error("beautifyCSS should add newlines")
		}
	})
	t.Run("beautifySQL_keywords", func(t *testing.T) {
		got := beautifySQL(`select * from users`)
		if !strings.Contains(got, "SELECT") {
			t.Error("beautifySQL should uppercase SELECT")
		}
	})
	t.Run("detectLanguage_json", func(t *testing.T) {
		lang := detectLanguage(`{"key":"value"}`)
		if lang != "json" {
			t.Errorf("detectLanguage = %q, want json", lang)
		}
	})
	t.Run("detectLanguage_html", func(t *testing.T) {
		lang := detectLanguage(`<html><body></body></html>`)
		if lang != "html" {
			t.Errorf("detectLanguage = %q, want html", lang)
		}
	})
	t.Run("detectLanguage_css", func(t *testing.T) {
		lang := detectLanguage(`body { margin: 0; }`)
		if lang != "css" {
			t.Errorf("detectLanguage = %q, want css", lang)
		}
	})
	t.Run("detectLanguage_sql", func(t *testing.T) {
		lang := detectLanguage(`SELECT * FROM users`)
		if lang != "sql" {
			t.Errorf("detectLanguage = %q, want sql", lang)
		}
	})
}

// --- DiffHandler ---

func TestDiffHandlerDirect(t *testing.T) {
	h := NewDiffHandler()
	ctx := context.Background()

	tests := []struct {
		name      string
		term      string
		wantError string
	}{
		{"same_text", "hello|||hello", ""},
		{"different_text", "hello\nworld|||hello\nGo", ""},
		{"no_separator", "hello world", "invalid_format"},
		{"empty_left", "|||right side", ""},
		{"empty_right", "left side|||", ""},
		{"empty_both", "|||", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			answer, err := h.HandleDirectQuery(ctx, tt.term)
			if err != nil {
				t.Fatalf("error: %v", err)
			}
			if answer == nil {
				t.Fatal("nil answer")
			}
			if tt.wantError != "" && answer.Error != tt.wantError {
				t.Errorf("answer.Error = %q, want %q", answer.Error, tt.wantError)
			}
			if tt.wantError == "" && answer.Error != "" {
				t.Errorf("unexpected error %q", answer.Error)
			}
		})
	}
}

func TestSimpleDiff(t *testing.T) {
	tests := []struct {
		name      string
		text1     string
		text2     string
		wantTypes map[string]bool
	}{
		{
			"identical",
			"hello\nworld",
			"hello\nworld",
			map[string]bool{"same": true},
		},
		{
			"addition",
			"hello",
			"hello\nworld",
			map[string]bool{"same": true, "add": true},
		},
		{
			"removal",
			"hello\nworld",
			"hello",
			map[string]bool{"same": true, "remove": true},
		},
		{
			"change",
			"hello\nworld",
			"hello\nGo",
			map[string]bool{"same": true, "remove": true, "add": true},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lines := simpleDiff(tt.text1, tt.text2)
			typesSeen := make(map[string]bool)
			for _, l := range lines {
				typesSeen[l.Type] = true
			}
			for want := range tt.wantTypes {
				if !typesSeen[want] {
					t.Errorf("expected diff type %q but it was not present; types: %v", want, typesSeen)
				}
			}
		})
	}
}

// --- RulesHandler ---

func TestRulesHandlerDirect(t *testing.T) {
	h := NewRulesHandler()
	ctx := context.Background()

	tests := []struct {
		name      string
		term      string
		wantMode  string
		wantTitle string
	}{
		{"empty", "", "all", "Rules of the Internet"},
		{"all_lower", "all", "all", "Rules of the Internet"},
		{"ALL_upper", "ALL", "all", "Rules of the Internet"},
		{"rule_34", "34", "single", "Rule 34 of the Internet"},
		{"rule_42", "42", "single", "Rule 42 of the Internet"},
		{"rule_1", "1", "single", "Rule 1 of the Internet"},
		{"out_of_range", "9999", "all", "Rules of the Internet"},
		{"search_text", "porn", "search", ""},
		{"rule_2", "2", "single", "Rule 2 of the Internet"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			answer, err := h.HandleDirectQuery(ctx, tt.term)
			if err != nil {
				t.Fatalf("error: %v", err)
			}
			if answer == nil {
				t.Fatal("nil answer")
			}
			if answer.Error != "" {
				t.Errorf("unexpected error %q", answer.Error)
			}
			mode, _ := answer.Data["mode"].(string)
			if mode != tt.wantMode {
				t.Errorf("mode = %q, want %q", mode, tt.wantMode)
			}
			if tt.wantTitle != "" && answer.Title != tt.wantTitle {
				t.Errorf("title = %q, want %q", answer.Title, tt.wantTitle)
			}
		})
	}
}

func TestHighlightTerm(t *testing.T) {
	tests := []struct {
		name string
		text string
		term string
		want string
	}{
		{"simple_match", "hello world", "world", "hello <mark>world</mark>"},
		{"case_insensitive", "Hello World", "hello", "<mark>Hello</mark> World"},
		{"no_match", "foo bar", "baz", "foo bar"},
		{"empty_term", "hello", "", "hello"},
		{"empty_text", "", "hello", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := highlightTerm(tt.text, tt.term)
			if got != tt.want {
				t.Errorf("highlightTerm(%q, %q) = %q, want %q", tt.text, tt.term, got, tt.want)
			}
		})
	}
}

func TestGetRuleContext(t *testing.T) {
	tests := []struct {
		num       int
		wantEmpty bool
	}{
		{1, false},
		{2, false},
		{34, false},
		{35, false},
		{42, false},
		{63, false},
		{69, false},
		{3, true},
		{100, true},
		{50, true},
	}
	for _, tt := range tests {
		t.Run(strings.Join([]string{"rule", string(rune('0' + tt.num/10)), string(rune('0' + tt.num%10))}, ""), func(t *testing.T) {
			got := getRuleContext(tt.num)
			isEmpty := got == ""
			if isEmpty != tt.wantEmpty {
				t.Errorf("getRuleContext(%d) empty = %v, want %v (got: %q)", tt.num, isEmpty, tt.wantEmpty, got)
			}
		})
	}
}

func TestRulesRule34Content(t *testing.T) {
	// Rule 34 must contain the canonical text
	h := NewRulesHandler()
	ctx := context.Background()

	answer, err := h.HandleDirectQuery(ctx, "34")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if answer == nil {
		t.Fatal("nil answer")
	}
	if !strings.Contains(answer.Content, "porn") {
		t.Error("Rule 34 content should mention 'porn'")
	}
}

func TestRulesRule42Content(t *testing.T) {
	h := NewRulesHandler()
	ctx := context.Background()

	answer, err := h.HandleDirectQuery(ctx, "42")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if !strings.Contains(answer.Content, "42") {
		t.Error("Rule 42 content should mention '42'")
	}
}

// --- slang.go pure helpers ---

func TestCleanBrackets(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"no_brackets", "hello world", "hello world"},
		{"single_bracket_word", "[linked]", "linked"},
		{"bracket_in_sentence", "see [this word] here", "see this word here"},
		{"multiple_brackets", "[foo] and [bar]", "foo and bar"},
		{"empty", "", ""},
		{"nested_bracket_chars", "a[b[c]d]e", "abcde"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cleanBrackets(tt.input)
			if got != tt.want {
				t.Errorf("cleanBrackets(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestTruncateText(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		maxLen int
		want   string
	}{
		{"no_truncate", "hello", 10, "hello"},
		{"exact", "hello", 5, "hello"},
		{"truncate", "hello world", 8, "hello..."},
		{"newline_replaced", "hello\nworld", 20, "hello world"},
		{"cr_removed", "hello\rworld", 20, "helloworld"},
		{"empty", "", 10, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncateText(tt.input, tt.maxLen)
			if got != tt.want {
				t.Errorf("truncateText(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.want)
			}
		})
	}
}

func TestFormatUrbanDictionaryText(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		contains    []string
		notContains []string
	}{
		{
			"plain_text",
			"just some text",
			[]string{"just some text"},
			nil,
		},
		{
			"bracket_to_link",
			"look up [something] here",
			[]string{`<a href=`, "something", `</a>`},
			[]string{"[something]"},
		},
		{
			"html_escaped",
			"<script>alert(1)</script>",
			[]string{"&lt;script&gt;"},
			[]string{"<script>"},
		},
		{
			"newline_to_br",
			"line1\nline2",
			[]string{"<br>"},
			nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatUrbanDictionaryText(tt.input)
			for _, want := range tt.contains {
				if !strings.Contains(got, want) {
					t.Errorf("formatUrbanDictionaryText output missing %q; got: %q", want, got)
				}
			}
			for _, notWant := range tt.notContains {
				if strings.Contains(got, notWant) {
					t.Errorf("formatUrbanDictionaryText output contains unexpected %q; got: %q", notWant, got)
				}
			}
		})
	}
}

// --- command.go: parseTLDRMarkdown ---

func TestParseTLDRMarkdown(t *testing.T) {
	tests := []struct {
		name     string
		markdown string
		command  string
		contains []string
	}{
		{
			"h1_heading",
			"# git",
			"git",
			[]string{"<h1>", "git", "</h1>"},
		},
		{
			"description",
			"> A distributed version control system.",
			"git",
			[]string{"<p", "distributed", "</p>"},
		},
		{
			"example_desc",
			"- Clone a repository:",
			"git",
			[]string{"<p", "Clone", "</p>"},
		},
		{
			"code_block",
			"`git clone {{url}}`",
			"git",
			[]string{"<pre", "<code>", "git clone", "</code>", "</pre>"},
		},
		{
			"full_example",
			"# git\n\n> Distributed VCS.\n\n- Clone:\n\n`git clone {{url}}`",
			"git",
			[]string{"<h1>", "Distributed VCS.", "<p", "Clone", "<pre", "git clone"},
		},
		{
			"empty_markdown",
			"",
			"git",
			[]string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseTLDRMarkdown(tt.markdown, tt.command)
			for _, want := range tt.contains {
				if !strings.Contains(got, want) {
					t.Errorf("parseTLDRMarkdown output missing %q;\ninput: %q\ngot: %q", want, tt.markdown, got)
				}
			}
		})
	}
}

// --- MIMEHandler extended ---

func TestMIMEHandlerExtended(t *testing.T) {
	h := NewMIMEHandler()
	ctx := context.Background()

	tests := []struct {
		name      string
		term      string
		wantErr   bool
		wantError string
	}{
		{"extension_txt", "txt", false, ""},
		{"extension_csv", "csv", false, ""},
		{"extension_xml", "xml", false, ""},
		{"mime_type_full", "text/html", false, ""},
		{"mime_type_json", "application/json", false, ""},
		{"unknown_ext", "xyz123unknown", false, ""},
		{"empty", "", true, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			answer, err := h.HandleDirectQuery(ctx, tt.term)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr = %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if answer == nil {
				t.Fatal("nil answer")
			}
			if tt.wantError != "" && answer.Error != tt.wantError {
				t.Errorf("answer.Error = %q, want %q", answer.Error, tt.wantError)
			}
		})
	}
}

// --- LicenseHandler extended ---

func TestLicenseHandlerExtended(t *testing.T) {
	h := NewLicenseHandler()
	ctx := context.Background()

	tests := []struct {
		name    string
		term    string
		wantErr bool
	}{
		{"mit_lowercase", "mit", false},
		{"apache_lowercase", "apache", false},
		{"gpl", "gpl", false},
		{"unknown_license", "UNLICENSE-XYZ-NONEXISTENT", false},
		{"empty", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			answer, err := h.HandleDirectQuery(ctx, tt.term)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr = %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if answer == nil {
				t.Fatal("nil answer")
			}
		})
	}
}

// --- escapeHTML helper (from command.go) ---

func TestEscapeHTMLHelper(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"ampersand", "&", "&amp;"},
		{"lt", "<", "&lt;"},
		{"gt", ">", "&gt;"},
		{"plain", "hello", "hello"},
		{"combined", "<b>bold</b>", "&lt;b&gt;bold&lt;/b&gt;"},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := escapeHTML(tt.input)
			if got != tt.want {
				t.Errorf("escapeHTML(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// --- splitWords helper (texttools.go) ---

func TestSplitWords(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"hello world", []string{"hello", "world"}},
		{"hello_world", []string{"hello", "world"}},
		{"hello-world", []string{"hello", "world"}},
		{"hello.world", []string{"hello", "world"}},
		{"helloWorld", []string{"hello", "World"}},
		{"HelloWorld", []string{"Hello", "World"}},
		{"", []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := splitWords(tt.input)
			if len(got) != len(tt.want) {
				t.Errorf("splitWords(%q) = %v, want %v", tt.input, got, tt.want)
				return
			}
			for i, w := range tt.want {
				if got[i] != w {
					t.Errorf("splitWords(%q)[%d] = %q, want %q", tt.input, i, got[i], w)
				}
			}
		})
	}
}

// --- Process idempotency: calling the same handler twice returns consistent results ---

func TestProcessIdempotency(t *testing.T) {
	m := NewManager()
	ctx := context.Background()

	queries := []string{
		"http:200",
		"chmod:755",
		"cron:* * * * *",
		"rule:1",
	}
	for _, q := range queries {
		t.Run(q, func(t *testing.T) {
			a1, err1 := m.Process(ctx, q)
			a2, err2 := m.Process(ctx, q)
			if (err1 != nil) != (err2 != nil) {
				t.Errorf("idempotency: first error=%v, second error=%v", err1, err2)
			}
			if a1 == nil || a2 == nil {
				t.Fatal("nil answer")
			}
			if a1.Error != a2.Error {
				t.Errorf("error field changed between calls: %q vs %q", a1.Error, a2.Error)
			}
			if a1.Type != a2.Type {
				t.Errorf("type changed between calls: %q vs %q", a1.Type, a2.Type)
			}
		})
	}
}
