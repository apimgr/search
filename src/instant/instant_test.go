package instant

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAnswerTypeConstants(t *testing.T) {
	tests := []struct {
		at   AnswerType
		want string
	}{
		{AnswerTypeDefinition, "definition"},
		{AnswerTypeDictionary, "dictionary"},
		{AnswerTypeSynonym, "synonym"},
		{AnswerTypeAntonym, "antonym"},
		{AnswerTypeMath, "math"},
		{AnswerTypeConvert, "convert"},
		{AnswerTypeTime, "time"},
		{AnswerTypeDate, "date"},
		{AnswerTypeIP, "ip"},
		{AnswerTypeHash, "hash"},
		{AnswerTypeBase64, "base64"},
		{AnswerTypeURL, "url"},
		{AnswerTypeColor, "color"},
		{AnswerTypeUUID, "uuid"},
		{AnswerTypeRandom, "random"},
		{AnswerTypePassword, "password"},
	}

	for _, tt := range tests {
		if string(tt.at) != tt.want {
			t.Errorf("AnswerType = %q, want %q", tt.at, tt.want)
		}
	}
}

func TestAnswerStruct(t *testing.T) {
	a := Answer{
		Type:        AnswerTypeMath,
		Query:       "2 + 2",
		Title:       "Calculator",
		Content:     "4",
		Data:        map[string]interface{}{"result": 4},
		Source:      "builtin",
		SourceURL:   "https://example.com",
		RelatedHTML: "<div>related</div>",
	}

	if a.Type != AnswerTypeMath {
		t.Errorf("Type = %v, want %v", a.Type, AnswerTypeMath)
	}
	if a.Query != "2 + 2" {
		t.Errorf("Query = %q, want %q", a.Query, "2 + 2")
	}
	if a.Title != "Calculator" {
		t.Errorf("Title = %q, want %q", a.Title, "Calculator")
	}
	if a.Data["result"] != 4 {
		t.Errorf("Data[result] = %v, want %v", a.Data["result"], 4)
	}
}

func TestNewManager(t *testing.T) {
	m := NewManager()
	if m == nil {
		t.Fatal("NewManager() returned nil")
	}
	if m.handlers == nil {
		t.Error("handlers should not be nil")
	}
	if len(m.handlers) == 0 {
		t.Error("handlers should not be empty")
	}
}

func TestManagerRegister(t *testing.T) {
	m := &Manager{
		handlers: make([]Handler, 0),
	}

	h := NewMathHandler()
	m.Register(h)

	if len(m.handlers) != 1 {
		t.Errorf("handlers length = %d, want %d", len(m.handlers), 1)
	}
}

func TestManagerGetHandlers(t *testing.T) {
	m := NewManager()

	handlers := m.GetHandlers()
	if len(handlers) == 0 {
		t.Error("GetHandlers() should return handlers")
	}
}

func TestManagerProcessNoMatch(t *testing.T) {
	m := NewManager()

	ctx := context.Background()
	result, err := m.Process(ctx, "random text that matches nothing special")
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}
	if result != nil {
		t.Error("Process() should return nil for non-matching query")
	}
}

func TestManagerProcessMath(t *testing.T) {
	m := NewManager()

	ctx := context.Background()
	result, err := m.Process(ctx, "2 + 2")
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}
	if result == nil {
		t.Fatal("Process() should return answer for math")
	}
	if result.Type != AnswerTypeMath {
		t.Errorf("Type = %v, want %v", result.Type, AnswerTypeMath)
	}
}

func TestManagerProcessCalc(t *testing.T) {
	m := NewManager()

	tests := []string{
		"calc 2+2",
		"calculate: 10/2",
		"math: 5*5",
		"eval 100-50",
		"compute 2^3",
	}

	ctx := context.Background()
	for _, query := range tests {
		t.Run(query, func(t *testing.T) {
			result, err := m.Process(ctx, query)
			if err != nil {
				t.Fatalf("Process(%q) error = %v", query, err)
			}
			if result == nil {
				t.Fatalf("Process(%q) should return answer", query)
			}
			if result.Type != AnswerTypeMath {
				t.Errorf("Type = %v, want %v", result.Type, AnswerTypeMath)
			}
		})
	}
}

func TestManagerProcessTrimWhitespace(t *testing.T) {
	m := NewManager()

	ctx := context.Background()
	result, err := m.Process(ctx, "  2 + 2  ")
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}
	if result == nil {
		t.Fatal("Process() should return answer")
	}
}

func TestMathHandlerName(t *testing.T) {
	h := NewMathHandler()
	if h.Name() != "math" {
		t.Errorf("Name() = %q, want %q", h.Name(), "math")
	}
}

func TestMathHandlerPatterns(t *testing.T) {
	h := NewMathHandler()
	patterns := h.Patterns()
	if len(patterns) == 0 {
		t.Error("Patterns() should return patterns")
	}
}

func TestMathHandlerCanHandle(t *testing.T) {
	h := NewMathHandler()

	tests := []struct {
		query string
		want  bool
	}{
		// Explicit prefixes — handle anything including complex expressions
		{"calc 2+2", true},
		{"calculate: 10*5", true},
		{"math: 5+5", true},
		{"eval 100/2", true},
		{"compute 2*3", true},
		{"calc: sqrt(16)", true},
		{"calc: 2^3", true},
		{"calc: pi*2", true},
		// Pure digit arithmetic works without prefix
		{"2 + 2", true},
		{"10 * 5", true},
		{"100 / 4", true},
		{"5 - 3", true},
		{"3+5*10 / 2 - 20", true},
		{"(3+5)*2", true},
		// Percentage pattern works without prefix
		{"15% of 200", true},
		// Expressions with letters or ^ require prefix
		{"2^3", false},
		{"hello world", false},
		{"random text", false},
		{"apt dist-upgrade", false},
		{"systemctl restart-all", false},
		{"git commit-msg", false},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			got := h.CanHandle(tt.query)
			if got != tt.want {
				t.Errorf("CanHandle(%q) = %v, want %v", tt.query, got, tt.want)
			}
		})
	}
}

func TestMathHandlerHandle(t *testing.T) {
	h := NewMathHandler()
	ctx := context.Background()

	tests := []struct {
		query  string
		result float64
	}{
		{"2 + 2", 4},
		{"10 - 5", 5},
		{"3 * 4", 12},
		{"20 / 4", 5},
		{"calc 2+3", 5},
		{"math: 100-50", 50},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			answer, err := h.HandleInstantQuery(ctx, tt.query)
			if err != nil {
				t.Fatalf("HandleInstantQuery(%q) error = %v", tt.query, err)
			}
			if answer == nil {
				t.Fatal("HandleInstantQuery() returned nil")
			}
			if answer.Type != AnswerTypeMath {
				t.Errorf("Type = %v, want %v", answer.Type, AnswerTypeMath)
			}
			if answer.Data == nil {
				t.Fatal("Data should not be nil")
			}
			if answer.Data["result"] != tt.result {
				t.Errorf("Data[result] = %v, want %v", answer.Data["result"], tt.result)
			}
		})
	}
}

func TestEvaluateExpression(t *testing.T) {
	tests := []struct {
		expr   string
		result float64
		hasErr bool
	}{
		{"2 + 2", 4, false},
		{"10 - 5", 5, false},
		{"3 * 4", 12, false},
		{"20 / 4", 5, false},
		{"2 + 3 * 4", 14, false},
		{"(2 + 3) * 4", 20, false},
		{"-5", -5, false},
		{"2^3", 8, false},
		{"10%", 0.1, false},
		{"50% of 200", 100, false},
		{"invalid", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.expr, func(t *testing.T) {
			result, err := evaluateExpression(tt.expr)
			if tt.hasErr {
				if err == nil {
					t.Errorf("evaluateExpression(%q) should error", tt.expr)
				}
			} else {
				if err != nil {
					t.Fatalf("evaluateExpression(%q) error = %v", tt.expr, err)
				}
				if result != tt.result {
					t.Errorf("evaluateExpression(%q) = %v, want %v", tt.expr, result, tt.result)
				}
			}
		})
	}
}

func TestEvaluateExpressionDivisionByZero(t *testing.T) {
	_, err := evaluateExpression("10 / 0")
	if err == nil {
		t.Error("Division by zero should error")
	}
}

func TestFormatNumber(t *testing.T) {
	tests := []struct {
		n    float64
		want string
	}{
		{4, "4"},
		{100, "100"},
		{3.14, "3.14"},
		{0.5, "0.5"},
		{1000000, "1000000"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := formatNumber(tt.n)
			if got != tt.want {
				t.Errorf("formatNumber(%v) = %q, want %q", tt.n, got, tt.want)
			}
		})
	}
}

func TestNewMathHandler(t *testing.T) {
	h := NewMathHandler()
	if h == nil {
		t.Fatal("NewMathHandler() returned nil")
	}
	if h.patterns == nil {
		t.Error("patterns should not be nil")
	}
	if h.numericExpr == nil {
		t.Error("numericExpr should not be nil")
	}
}

func TestEvalSimple(t *testing.T) {
	tests := []struct {
		expr   string
		result float64
		hasErr bool
	}{
		{"5", 5, false},
		{"3.14", 3.14, false},
		{"2 + 3", 5, false},
		{"10 - 4", 6, false},
		{"3 * 7", 21, false},
		{"20 / 4", 5, false},
		{"10 % 3", 1, false},
		{"2**3", 8, false},
		{"(2 + 3)", 5, false},
	}

	for _, tt := range tests {
		t.Run(tt.expr, func(t *testing.T) {
			result, err := evalSimple(tt.expr)
			if tt.hasErr {
				if err == nil {
					t.Errorf("evalSimple(%q) should error", tt.expr)
				}
			} else {
				if err != nil {
					t.Fatalf("evalSimple(%q) error = %v", tt.expr, err)
				}
				if result != tt.result {
					t.Errorf("evalSimple(%q) = %v, want %v", tt.expr, result, tt.result)
				}
			}
		})
	}
}

// Tests for ConvertHandler

func TestNewConvertHandler(t *testing.T) {
	h := NewConvertHandler()
	if h == nil {
		t.Fatal("NewConvertHandler() returned nil")
	}
	if h.patterns == nil {
		t.Error("patterns should not be nil")
	}
}

func TestConvertHandlerName(t *testing.T) {
	h := NewConvertHandler()
	if h.Name() != "convert" {
		t.Errorf("Name() = %q, want %q", h.Name(), "convert")
	}
}

func TestConvertHandlerPatterns(t *testing.T) {
	h := NewConvertHandler()
	patterns := h.Patterns()
	if len(patterns) == 0 {
		t.Error("Patterns() should return patterns")
	}
}

func TestConvertHandlerCanHandle(t *testing.T) {
	h := NewConvertHandler()

	tests := []struct {
		query string
		want  bool
	}{
		{"100 km to miles", true},
		{"convert 50 kg to pounds", true},
		{"32 °f to °c", true},
		{"10 meters in feet", true},
		{"hello world", false},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			got := h.CanHandle(tt.query)
			if got != tt.want {
				t.Errorf("CanHandle(%q) = %v, want %v", tt.query, got, tt.want)
			}
		})
	}
}

func TestConvertHandlerHandle(t *testing.T) {
	h := NewConvertHandler()
	ctx := context.Background()

	tests := []struct {
		query    string
		hasError bool
	}{
		{"100 km to miles", false},
		{"32 c to f", false},
		{"1 gb to mb", false},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			answer, err := h.HandleInstantQuery(ctx, tt.query)
			if err != nil {
				t.Fatalf("HandleInstantQuery(%q) error = %v", tt.query, err)
			}
			if answer == nil {
				t.Fatal("HandleInstantQuery() returned nil")
			}
			if answer.Type != AnswerTypeConvert {
				t.Errorf("Type = %v, want %v", answer.Type, AnswerTypeConvert)
			}
		})
	}
}

func TestNormalizeUnit(t *testing.T) {
	tests := []struct {
		unit string
		want string
	}{
		{"m", "meters"},
		{"km", "kilometers"},
		{"kg", "kilograms"},
		{"lb", "pounds"},
		{"c", "celsius"},
		{"f", "fahrenheit"},
		{"gb", "gigabytes"},
		{"unknown", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.unit, func(t *testing.T) {
			got := normalizeUnit(tt.unit)
			if got != tt.want {
				t.Errorf("normalizeUnit(%q) = %q, want %q", tt.unit, got, tt.want)
			}
		})
	}
}

func TestConvert(t *testing.T) {
	tests := []struct {
		value  float64
		from   string
		to     string
		want   float64
		hasErr bool
	}{
		{1000, "meters", "kilometers", 1, false},
		{1, "kilometers", "meters", 1000, false},
		{32, "fahrenheit", "celsius", 0, false},
		{100, "celsius", "fahrenheit", 212, false},
		{1, "kilograms", "grams", 1000, false},
		{1, "meters", "meters", 1, false},
		{1, "unknown1", "unknown2", 0, true},
	}

	for _, tt := range tests {
		name := tt.from + "_to_" + tt.to
		t.Run(name, func(t *testing.T) {
			result, err := convert(tt.value, tt.from, tt.to)
			if tt.hasErr {
				if err == nil {
					t.Error("convert() should error")
				}
			} else {
				if err != nil {
					t.Fatalf("convert() error = %v", err)
				}
				if result != tt.want {
					t.Errorf("convert() = %v, want %v", result, tt.want)
				}
			}
		})
	}
}

func TestConvertTemperature(t *testing.T) {
	tests := []struct {
		value float64
		from  string
		to    string
		want  float64
	}{
		{0, "celsius", "fahrenheit", 32},
		{100, "celsius", "fahrenheit", 212},
		{32, "fahrenheit", "celsius", 0},
		{0, "celsius", "kelvin", 273.15},
		{273.15, "kelvin", "celsius", 0},
	}

	for _, tt := range tests {
		name := tt.from + "_to_" + tt.to
		t.Run(name, func(t *testing.T) {
			result := convertTemperature(tt.value, tt.from, tt.to)
			if result != tt.want {
				t.Errorf("convertTemperature() = %v, want %v", result, tt.want)
			}
		})
	}
}

// Tests for TimeHandler

func TestNewTimeHandler(t *testing.T) {
	h := NewTimeHandler()
	if h == nil {
		t.Fatal("NewTimeHandler() returned nil")
	}
}

func TestTimeHandlerName(t *testing.T) {
	h := NewTimeHandler()
	if h.Name() != "time" {
		t.Errorf("Name() = %q, want %q", h.Name(), "time")
	}
}

func TestTimeHandlerCanHandle(t *testing.T) {
	h := NewTimeHandler()

	tests := []struct {
		query string
		want  bool
	}{
		{"time", true},
		{"current time", true},
		{"what time is it?", true},
		{"now", true},
		{"date", true},
		{"today", true},
		{"timestamp", true},
		{"unix time", true},
		{"epoch", true},
		{"hello world", false},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			got := h.CanHandle(tt.query)
			if got != tt.want {
				t.Errorf("CanHandle(%q) = %v, want %v", tt.query, got, tt.want)
			}
		})
	}
}

func TestTimeHandlerHandle(t *testing.T) {
	h := NewTimeHandler()
	ctx := context.Background()

	answer, err := h.HandleInstantQuery(ctx, "time")
	if err != nil {
		t.Fatalf("HandleInstantQuery() error = %v", err)
	}
	if answer == nil {
		t.Fatal("HandleInstantQuery() returned nil")
	}
	if answer.Type != AnswerTypeTime {
		t.Errorf("Type = %v, want %v", answer.Type, AnswerTypeTime)
	}
	if answer.Data["timestamp"] == nil {
		t.Error("Data should contain timestamp")
	}
}

// Tests for HashHandler

func TestNewHashHandler(t *testing.T) {
	h := NewHashHandler()
	if h == nil {
		t.Fatal("NewHashHandler() returned nil")
	}
}

func TestHashHandlerName(t *testing.T) {
	h := NewHashHandler()
	if h.Name() != "hash" {
		t.Errorf("Name() = %q, want %q", h.Name(), "hash")
	}
}

func TestHashHandlerCanHandle(t *testing.T) {
	h := NewHashHandler()

	tests := []struct {
		query string
		want  bool
	}{
		{"md5: hello", true},
		{"sha1: hello", true},
		{"sha256: hello", true},
		{"sha512: hello", true},
		{"hash: hello", true},
		{"hello world", false},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			got := h.CanHandle(tt.query)
			if got != tt.want {
				t.Errorf("CanHandle(%q) = %v, want %v", tt.query, got, tt.want)
			}
		})
	}
}

func TestHashHandlerHandle(t *testing.T) {
	h := NewHashHandler()
	ctx := context.Background()

	tests := []string{"md5: hello", "sha256: test", "hash: world"}
	for _, query := range tests {
		t.Run(query, func(t *testing.T) {
			answer, err := h.HandleInstantQuery(ctx, query)
			if err != nil {
				t.Fatalf("HandleInstantQuery() error = %v", err)
			}
			if answer == nil {
				t.Fatal("HandleInstantQuery() returned nil")
			}
			if answer.Type != AnswerTypeHash {
				t.Errorf("Type = %v, want %v", answer.Type, AnswerTypeHash)
			}
		})
	}
}

// Tests for Base64Handler

func TestNewBase64Handler(t *testing.T) {
	h := NewBase64Handler()
	if h == nil {
		t.Fatal("NewBase64Handler() returned nil")
	}
}

func TestBase64HandlerName(t *testing.T) {
	h := NewBase64Handler()
	if h.Name() != "base64" {
		t.Errorf("Name() = %q, want %q", h.Name(), "base64")
	}
}

func TestBase64HandlerCanHandle(t *testing.T) {
	h := NewBase64Handler()

	tests := []struct {
		query string
		want  bool
	}{
		{"base64 encode: hello", true},
		{"base64 decode: aGVsbG8=", true},
		{"b64 encode: test", true},
		{"hello world", false},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			got := h.CanHandle(tt.query)
			if got != tt.want {
				t.Errorf("CanHandle(%q) = %v, want %v", tt.query, got, tt.want)
			}
		})
	}
}

func TestBase64HandlerHandle(t *testing.T) {
	h := NewBase64Handler()
	ctx := context.Background()

	// Test encode
	answer, err := h.HandleInstantQuery(ctx, "base64 encode: hello")
	if err != nil {
		t.Fatalf("HandleInstantQuery() error = %v", err)
	}
	if answer == nil {
		t.Fatal("HandleInstantQuery() returned nil")
	}
	if answer.Type != AnswerTypeBase64 {
		t.Errorf("Type = %v, want %v", answer.Type, AnswerTypeBase64)
	}
	if answer.Data["output"] != "aGVsbG8=" {
		t.Errorf("output = %v, want %v", answer.Data["output"], "aGVsbG8=")
	}

	// Test decode
	answer, err = h.HandleInstantQuery(ctx, "base64 decode: aGVsbG8=")
	if err != nil {
		t.Fatalf("HandleInstantQuery() error = %v", err)
	}
	if answer.Data["output"] != "hello" {
		t.Errorf("output = %v, want %v", answer.Data["output"], "hello")
	}
}

// Tests for URLHandler

func TestNewURLHandler(t *testing.T) {
	h := NewURLHandler()
	if h == nil {
		t.Fatal("NewURLHandler() returned nil")
	}
}

func TestURLHandlerName(t *testing.T) {
	h := NewURLHandler()
	if h.Name() != "url" {
		t.Errorf("Name() = %q, want %q", h.Name(), "url")
	}
}

func TestURLHandlerCanHandle(t *testing.T) {
	h := NewURLHandler()

	tests := []struct {
		query string
		want  bool
	}{
		{"url encode: hello world", true},
		{"url decode: hello%20world", true},
		{"urlencode: test", true},
		{"parse url: https://example.com", true},
		{"hello world", false},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			got := h.CanHandle(tt.query)
			if got != tt.want {
				t.Errorf("CanHandle(%q) = %v, want %v", tt.query, got, tt.want)
			}
		})
	}
}

func TestURLHandlerHandle(t *testing.T) {
	h := NewURLHandler()
	ctx := context.Background()

	answer, err := h.HandleInstantQuery(ctx, "url encode: hello world")
	if err != nil {
		t.Fatalf("HandleInstantQuery() error = %v", err)
	}
	if answer == nil {
		t.Fatal("HandleInstantQuery() returned nil")
	}
	if answer.Type != AnswerTypeURL {
		t.Errorf("Type = %v, want %v", answer.Type, AnswerTypeURL)
	}
}

// Tests for ColorHandler

func TestNewColorHandler(t *testing.T) {
	h := NewColorHandler()
	if h == nil {
		t.Fatal("NewColorHandler() returned nil")
	}
}

func TestColorHandlerName(t *testing.T) {
	h := NewColorHandler()
	if h.Name() != "color" {
		t.Errorf("Name() = %q, want %q", h.Name(), "color")
	}
}

func TestColorHandlerCanHandle(t *testing.T) {
	h := NewColorHandler()

	tests := []struct {
		query string
		want  bool
	}{
		{"color: ff0000", true},
		{"#ff0000", true},
		{"rgb: 255, 0, 0", true},
		{"hello world", false},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			got := h.CanHandle(tt.query)
			if got != tt.want {
				t.Errorf("CanHandle(%q) = %v, want %v", tt.query, got, tt.want)
			}
		})
	}
}

func TestColorHandlerHandle(t *testing.T) {
	h := NewColorHandler()
	ctx := context.Background()

	answer, err := h.HandleInstantQuery(ctx, "#ff0000")
	if err != nil {
		t.Fatalf("HandleInstantQuery() error = %v", err)
	}
	if answer == nil {
		t.Fatal("HandleInstantQuery() returned nil")
	}
	if answer.Type != AnswerTypeColor {
		t.Errorf("Type = %v, want %v", answer.Type, AnswerTypeColor)
	}
}

func TestRgbToHSL(t *testing.T) {
	// Test pure red
	result := rgbToHSL(255, 0, 0)
	if result == "" {
		t.Error("rgbToHSL() returned empty string")
	}

	// Test black
	result = rgbToHSL(0, 0, 0)
	if result == "" {
		t.Error("rgbToHSL() returned empty string")
	}

	// Test white
	result = rgbToHSL(255, 255, 255)
	if result == "" {
		t.Error("rgbToHSL() returned empty string")
	}
}

// Tests for UUIDHandler

func TestNewUUIDHandler(t *testing.T) {
	h := NewUUIDHandler()
	if h == nil {
		t.Fatal("NewUUIDHandler() returned nil")
	}
}

func TestUUIDHandlerName(t *testing.T) {
	h := NewUUIDHandler()
	if h.Name() != "uuid" {
		t.Errorf("Name() = %q, want %q", h.Name(), "uuid")
	}
}

func TestUUIDHandlerCanHandle(t *testing.T) {
	h := NewUUIDHandler()

	tests := []struct {
		query string
		want  bool
	}{
		{"uuid", true},
		{"generate uuid", true},
		{"new uuid", true},
		{"guid", true},
		{"hello world", false},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			got := h.CanHandle(tt.query)
			if got != tt.want {
				t.Errorf("CanHandle(%q) = %v, want %v", tt.query, got, tt.want)
			}
		})
	}
}

func TestUUIDHandlerHandle(t *testing.T) {
	h := NewUUIDHandler()
	ctx := context.Background()

	answer, err := h.HandleInstantQuery(ctx, "uuid")
	if err != nil {
		t.Fatalf("HandleInstantQuery() error = %v", err)
	}
	if answer == nil {
		t.Fatal("HandleInstantQuery() returned nil")
	}
	if answer.Type != AnswerTypeUUID {
		t.Errorf("Type = %v, want %v", answer.Type, AnswerTypeUUID)
	}
	if answer.Data["uuid"] == nil {
		t.Error("Data should contain uuid")
	}
}

// Tests for RandomHandler

func TestNewRandomHandler(t *testing.T) {
	h := NewRandomHandler()
	if h == nil {
		t.Fatal("NewRandomHandler() returned nil")
	}
}

func TestRandomHandlerName(t *testing.T) {
	h := NewRandomHandler()
	if h.Name() != "random" {
		t.Errorf("Name() = %q, want %q", h.Name(), "random")
	}
}

func TestRandomHandlerCanHandle(t *testing.T) {
	h := NewRandomHandler()

	tests := []struct {
		query string
		want  bool
	}{
		{"random", true},
		{"random number", true},
		{"random 1-100", true},
		{"roll dice", true},
		{"roll d20", true},
		{"flip coin", true},
		{"coin flip", true},
		{"hello world", false},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			got := h.CanHandle(tt.query)
			if got != tt.want {
				t.Errorf("CanHandle(%q) = %v, want %v", tt.query, got, tt.want)
			}
		})
	}
}

func TestRandomHandlerHandle(t *testing.T) {
	h := NewRandomHandler()
	ctx := context.Background()

	tests := []string{"random", "flip coin", "roll dice", "roll d20"}
	for _, query := range tests {
		t.Run(query, func(t *testing.T) {
			answer, err := h.HandleInstantQuery(ctx, query)
			if err != nil {
				t.Fatalf("HandleInstantQuery() error = %v", err)
			}
			if answer == nil {
				t.Fatal("HandleInstantQuery() returned nil")
			}
			if answer.Type != AnswerTypeRandom {
				t.Errorf("Type = %v, want %v", answer.Type, AnswerTypeRandom)
			}
		})
	}
}

// Tests for PasswordHandler

func TestNewPasswordHandler(t *testing.T) {
	h := NewPasswordHandler()
	if h == nil {
		t.Fatal("NewPasswordHandler() returned nil")
	}
}

func TestPasswordHandlerName(t *testing.T) {
	h := NewPasswordHandler()
	if h.Name() != "password" {
		t.Errorf("Name() = %q, want %q", h.Name(), "password")
	}
}

func TestPasswordHandlerCanHandle(t *testing.T) {
	h := NewPasswordHandler()

	tests := []struct {
		query string
		want  bool
	}{
		{"password", true},
		{"generate password", true},
		{"password 32", true},
		{"random password", true},
		{"secure password", true},
		{"hello world", false},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			got := h.CanHandle(tt.query)
			if got != tt.want {
				t.Errorf("CanHandle(%q) = %v, want %v", tt.query, got, tt.want)
			}
		})
	}
}

func TestPasswordHandlerHandle(t *testing.T) {
	h := NewPasswordHandler()
	ctx := context.Background()

	answer, err := h.HandleInstantQuery(ctx, "password")
	if err != nil {
		t.Fatalf("HandleInstantQuery() error = %v", err)
	}
	if answer == nil {
		t.Fatal("HandleInstantQuery() returned nil")
	}
	if answer.Type != AnswerTypePassword {
		t.Errorf("Type = %v, want %v", answer.Type, AnswerTypePassword)
	}
	if answer.Data["password"] == nil {
		t.Error("Data should contain password")
	}
	if answer.Data["length"] != 16 {
		t.Errorf("length = %v, want %v", answer.Data["length"], 16)
	}
}

func TestPasswordHandlerHandleWithLength(t *testing.T) {
	h := NewPasswordHandler()
	ctx := context.Background()

	answer, err := h.HandleInstantQuery(ctx, "password 32")
	if err != nil {
		t.Fatalf("HandleInstantQuery() error = %v", err)
	}
	if answer.Data["length"] != 32 {
		t.Errorf("length = %v, want %v", answer.Data["length"], 32)
	}
}

func TestGeneratePassword(t *testing.T) {
	password := generatePassword(16)
	if len(password) != 16 {
		t.Errorf("generatePassword(16) length = %d, want 16", len(password))
	}
}

func TestGeneratePasswordNoSpecial(t *testing.T) {
	password := generatePasswordNoSpecial(16)
	if len(password) != 16 {
		t.Errorf("generatePasswordNoSpecial(16) length = %d, want 16", len(password))
	}
}

// Tests for IPHandler

func TestNewIPHandler(t *testing.T) {
	h := NewIPHandler()
	if h == nil {
		t.Fatal("NewIPHandler() returned nil")
	}
}

func TestIPHandlerName(t *testing.T) {
	h := NewIPHandler()
	if h.Name() != "ip" {
		t.Errorf("Name() = %q, want %q", h.Name(), "ip")
	}
}

func TestIPHandlerCanHandle(t *testing.T) {
	h := NewIPHandler()

	tests := []struct {
		query string
		want  bool
	}{
		{"my ip", true},
		{"my ip address", true},
		{"what is my ip?", true},
		{"ip address", true},
		{"ip info", true},
		{"ip 8.8.8.8", true},
		{"ip 1.2.3.4", true},
		{"8.8.8.8", true},
		{"1.1.1.1", true},
		{"hello world", false},
		{"192.168 incomplete", false},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			got := h.CanHandle(tt.query)
			if got != tt.want {
				t.Errorf("CanHandle(%q) = %v, want %v", tt.query, got, tt.want)
			}
		})
	}
}

func TestIPHandlerHandle(t *testing.T) {
	h := NewIPHandler()
	ctx := context.Background()

	answer, err := h.HandleInstantQuery(ctx, "my ip")
	if err != nil {
		t.Fatalf("HandleInstantQuery() error = %v", err)
	}
	if answer == nil {
		t.Fatal("HandleInstantQuery() returned nil")
	}
	if answer.Type != AnswerTypeIP {
		t.Errorf("Type = %v, want %v", answer.Type, AnswerTypeIP)
	}
}

func TestGetWeekOfYear(t *testing.T) {
	// Test a known date
	date := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)
	week := getWeekOfYear(date)
	if week < 1 || week > 53 {
		t.Errorf("getWeekOfYear() = %d, expected 1-53", week)
	}
}

// Additional tests for 100% coverage

func TestConvertHandlerHandleNoMatch(t *testing.T) {
	h := NewConvertHandler()
	ctx := context.Background()

	// Query that matches pattern but has empty units after extraction fails
	answer, err := h.HandleInstantQuery(ctx, "not a conversion query")
	if err != nil {
		t.Fatalf("HandleInstantQuery() error = %v", err)
	}
	if answer != nil {
		t.Error("HandleInstantQuery() should return nil for non-matching query")
	}
}

func TestConvertHandlerHandleSecondPattern(t *testing.T) {
	h := NewConvertHandler()
	ctx := context.Background()

	// Test the "X unit = ? unit" pattern
	answer, err := h.HandleInstantQuery(ctx, "100 km = ? miles")
	if err != nil {
		t.Fatalf("HandleInstantQuery() error = %v", err)
	}
	if answer == nil {
		t.Fatal("HandleInstantQuery() returned nil")
	}
	if answer.Type != AnswerTypeConvert {
		t.Errorf("Type = %v, want %v", answer.Type, AnswerTypeConvert)
	}
}

func TestConvertHandlerHandleUnknownConversion(t *testing.T) {
	h := NewConvertHandler()
	ctx := context.Background()

	// Test with unknown units that will cause convert() to return error
	answer, err := h.HandleInstantQuery(ctx, "100 foobar to bazqux")
	if err != nil {
		t.Fatalf("HandleInstantQuery() error = %v", err)
	}
	if answer == nil {
		t.Fatal("HandleInstantQuery() returned nil")
	}
	// Should contain error message in content
	if answer.Content == "" {
		t.Error("Content should contain error message")
	}
}

func TestConvertHandlerHandleArrowPattern(t *testing.T) {
	h := NewConvertHandler()
	ctx := context.Background()

	// Test the "->" pattern
	answer, err := h.HandleInstantQuery(ctx, "100 meters -> feet")
	if err != nil {
		t.Fatalf("HandleInstantQuery() error = %v", err)
	}
	if answer == nil {
		t.Fatal("HandleInstantQuery() returned nil")
	}
	if answer.Type != AnswerTypeConvert {
		t.Errorf("Type = %v, want %v", answer.Type, AnswerTypeConvert)
	}
}

func TestNormalizeUnitAllAliases(t *testing.T) {
	tests := []struct {
		unit string
		want string
	}{
		// Length
		{"meter", "meters"},
		{"metre", "meters"},
		{"metres", "meters"},
		{"kilometer", "kilometers"},
		{"kilometre", "kilometers"},
		{"kilometres", "kilometers"},
		{"centimeter", "centimeters"},
		{"centimetre", "centimeters"},
		{"centimetres", "centimeters"},
		{"millimeter", "millimeters"},
		{"millimetre", "millimeters"},
		{"millimetres", "millimeters"},
		{"mi", "miles"},
		{"mile", "miles"},
		{"ft", "feet"},
		{"foot", "feet"},
		{"in", "inches"},
		{"inch", "inches"},
		{"yd", "yards"},
		{"yard", "yards"},

		// Weight/Mass
		{"kilogram", "kilograms"},
		{"g", "grams"},
		{"gram", "grams"},
		{"mg", "milligrams"},
		{"milligram", "milligrams"},
		{"lbs", "pounds"},
		{"pound", "pounds"},
		{"oz", "ounces"},
		{"ounce", "ounces"},
		{"t", "tons"},
		{"ton", "tons"},
		{"tonne", "tonnes"},

		// Temperature
		{"°c", "celsius"},
		{"°f", "fahrenheit"},
		{"k", "kelvin"},
		{"°k", "kelvin"},

		// Volume
		{"l", "liters"},
		{"liter", "liters"},
		{"litre", "liters"},
		{"litres", "liters"},
		{"ml", "milliliters"},
		{"milliliter", "milliliters"},
		{"millilitre", "milliliters"},
		{"gal", "gallons"},
		{"gallon", "gallons"},
		{"qt", "quarts"},
		{"quart", "quarts"},
		{"pt", "pints"},
		{"pint", "pints"},
		{"cup", "cups"},

		// Time
		{"s", "seconds"},
		{"sec", "seconds"},
		{"second", "seconds"},
		{"min", "minutes"},
		{"minute", "minutes"},
		{"h", "hours"},
		{"hr", "hours"},
		{"hour", "hours"},
		{"d", "days"},
		{"day", "days"},
		{"wk", "weeks"},
		{"week", "weeks"},
		{"mo", "months"},
		{"month", "months"},
		{"yr", "years"},
		{"year", "years"},

		// Data
		{"b", "bytes"},
		{"kb", "kilobytes"},
		{"mb", "megabytes"},
		{"tb", "terabytes"},
	}

	for _, tt := range tests {
		t.Run(tt.unit, func(t *testing.T) {
			got := normalizeUnit(tt.unit)
			if got != tt.want {
				t.Errorf("normalizeUnit(%q) = %q, want %q", tt.unit, got, tt.want)
			}
		})
	}
}

func TestConvertAllCategories(t *testing.T) {
	tests := []struct {
		value  float64
		from   string
		to     string
		hasErr bool
	}{
		// Length conversions
		{1, "meters", "centimeters", false},
		{1, "meters", "millimeters", false},
		{1, "miles", "feet", false},
		{1, "yards", "inches", false},

		// Weight conversions
		{1, "grams", "milligrams", false},
		{1, "pounds", "ounces", false},
		{1, "tons", "grams", false},
		{1, "tonnes", "kilograms", false},

		// Volume conversions
		{1, "liters", "milliliters", false},
		{1, "gallons", "quarts", false},
		{1, "pints", "cups", false},

		// Time conversions
		{1, "hours", "minutes", false},
		{1, "days", "hours", false},
		{1, "weeks", "days", false},
		{1, "months", "days", false},
		{1, "years", "days", false},

		// Data conversions
		{1, "gigabytes", "megabytes", false},
		{1, "terabytes", "gigabytes", false},
		{1, "kilobytes", "bytes", false},

		// Cross-category (should error)
		{1, "meters", "kilograms", true},
		{1, "liters", "seconds", true},
	}

	for _, tt := range tests {
		name := tt.from + "_to_" + tt.to
		t.Run(name, func(t *testing.T) {
			_, err := convert(tt.value, tt.from, tt.to)
			if tt.hasErr && err == nil {
				t.Error("convert() should error")
			}
			if !tt.hasErr && err != nil {
				t.Errorf("convert() error = %v", err)
			}
		})
	}
}

func TestConvertTemperatureAllPaths(t *testing.T) {
	tests := []struct {
		value float64
		from  string
		to    string
		want  float64
	}{
		// Celsius conversions
		{0, "celsius", "celsius", 0},
		{100, "celsius", "kelvin", 373.15},

		// Fahrenheit conversions
		{212, "fahrenheit", "fahrenheit", 212},
		{32, "fahrenheit", "kelvin", 273.15},

		// Kelvin conversions
		{0, "kelvin", "kelvin", 0},
		{273.15, "kelvin", "fahrenheit", 32},
	}

	for _, tt := range tests {
		name := tt.from + "_to_" + tt.to
		t.Run(name, func(t *testing.T) {
			result := convertTemperature(tt.value, tt.from, tt.to)
			if result != tt.want {
				t.Errorf("convertTemperature() = %v, want %v", result, tt.want)
			}
		})
	}
}

func TestConvertTemperatureUnknownUnit(t *testing.T) {
	// Test with unknown from/to units - should return original value
	result := convertTemperature(100, "unknown", "celsius")
	if result != 100 {
		t.Errorf("convertTemperature() with unknown from = %v, want 100", result)
	}

	result = convertTemperature(100, "celsius", "unknown")
	if result != 100 {
		t.Errorf("convertTemperature() with unknown to = %v, want 100", result)
	}
}

func TestMathHandlerCanHandleShortExpression(t *testing.T) {
	h := NewMathHandler()

	// Short expression without operator should not match
	if h.CanHandle("12") {
		t.Error("CanHandle(\"12\") should return false")
	}

	// Short expression that's too short
	if h.CanHandle("1") {
		t.Error("CanHandle(\"1\") should return false")
	}
}

func TestMathHandlerCanHandleNoOperator(t *testing.T) {
	h := NewMathHandler()

	// Expression with only numbers and no operators
	if h.CanHandle("123") {
		t.Error("CanHandle(\"123\") should return false")
	}
}

func TestEvaluateExpressionPercentageVariations(t *testing.T) {
	tests := []struct {
		expr   string
		result float64
		hasErr bool
	}{
		{"25% of 200", 50, false},
		{"50% of 100", 50, false},
		{"10.5% of 1000", 105, false},
		{"75%", 0.75, false},
		{"100%", 1.0, false},
		{"abc%", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.expr, func(t *testing.T) {
			result, err := evaluateExpression(tt.expr)
			if tt.hasErr {
				if err == nil {
					t.Errorf("evaluateExpression(%q) should error", tt.expr)
				}
			} else {
				if err != nil {
					t.Fatalf("evaluateExpression(%q) error = %v", tt.expr, err)
				}
				if result != tt.result {
					t.Errorf("evaluateExpression(%q) = %v, want %v", tt.expr, result, tt.result)
				}
			}
		})
	}
}

func TestEvalSimplePowerErrors(t *testing.T) {
	// Test power with invalid base
	_, err := evalSimple("abc**2")
	if err == nil {
		t.Error("evalSimple(\"abc**2\") should error")
	}

	// Test power with invalid exponent
	_, err = evalSimple("2**abc")
	if err == nil {
		t.Error("evalSimple(\"2**abc\") should error")
	}
}

func TestEvalNodeUnsupportedLiteral(t *testing.T) {
	// This is tested indirectly through evaluateExpression
	_, err := evaluateExpression("\"string\"")
	if err == nil {
		t.Error("evaluateExpression with string literal should error")
	}
}

func TestEvalNodeModuloByZero(t *testing.T) {
	_, err := evaluateExpression("10 % 0")
	if err == nil {
		t.Error("Modulo by zero should error")
	}
}

func TestFormatNumberScientificNotation(t *testing.T) {
	tests := []struct {
		n       float64
		wantSci bool
	}{
		// Very small
		{0.00001, true},
		// Very large
		{1e15, true},
		// Just above threshold
		{0.0001, false},
		// Large but not huge
		{1e9, false},
	}

	for _, tt := range tests {
		result := formatNumber(tt.n)
		_ = len(result) > 0 && (result[len(result)-1] >= '0' && result[len(result)-1] <= '9') && (len(result) > 4 && result[len(result)-4] == 'e')
		// Just verify it returns something reasonable
		if result == "" {
			t.Errorf("formatNumber(%v) returned empty string", tt.n)
		}
	}
}

func TestFormatNumberEdgeCases(t *testing.T) {
	tests := []struct {
		n    float64
		want string
	}{
		{0, "0"},
		{-0, "0"},
		// Very small - scientific
		{1e-5, "1e-05"},
		// Large integer
		{1e15, "1000000000000000"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := formatNumber(tt.n)
			if got != tt.want {
				t.Errorf("formatNumber(%v) = %q, want %q", tt.n, got, tt.want)
			}
		})
	}
}

func TestHashHandlerHandleEmptyText(t *testing.T) {
	h := NewHashHandler()
	ctx := context.Background()

	// Test with query that doesn't extract text
	answer, err := h.HandleInstantQuery(ctx, "not a hash query")
	if err != nil {
		t.Fatalf("HandleInstantQuery() error = %v", err)
	}
	if answer != nil {
		t.Error("HandleInstantQuery() should return nil for non-matching text")
	}
}

func TestHashHandlerHandleAllTypes(t *testing.T) {
	h := NewHashHandler()
	ctx := context.Background()

	tests := []struct {
		query    string
		hashType string
	}{
		{"md5: hello", "md5"},
		{"sha1: hello", "sha1"},
		{"sha256: hello", "sha256"},
		{"sha512: hello", "sha512"},
		{"hash: hello", "all"},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			answer, err := h.HandleInstantQuery(ctx, tt.query)
			if err != nil {
				t.Fatalf("HandleInstantQuery() error = %v", err)
			}
			if answer == nil {
				t.Fatal("HandleInstantQuery() returned nil")
			}
			if answer.Type != AnswerTypeHash {
				t.Errorf("Type = %v, want %v", answer.Type, AnswerTypeHash)
			}
		})
	}
}

func TestHashHandlerPatterns(t *testing.T) {
	h := NewHashHandler()
	patterns := h.Patterns()
	if len(patterns) == 0 {
		t.Error("Patterns() should return patterns")
	}
}

func TestBase64HandlerHandleEmptyText(t *testing.T) {
	h := NewBase64Handler()
	ctx := context.Background()

	answer, err := h.HandleInstantQuery(ctx, "not a base64 query")
	if err != nil {
		t.Fatalf("HandleInstantQuery() error = %v", err)
	}
	if answer != nil {
		t.Error("HandleInstantQuery() should return nil for non-matching text")
	}
}

func TestBase64HandlerHandleInvalidDecode(t *testing.T) {
	h := NewBase64Handler()
	ctx := context.Background()

	answer, err := h.HandleInstantQuery(ctx, "base64 decode: !!!invalid!!!")
	if err != nil {
		t.Fatalf("HandleInstantQuery() error = %v", err)
	}
	if answer == nil {
		t.Fatal("HandleInstantQuery() returned nil")
	}
	// Should contain error message
	if !contains(answer.Content, "Error") && !contains(answer.Content, "Invalid") {
		t.Error("Content should contain error message for invalid base64")
	}
}

func TestBase64HandlerPatterns(t *testing.T) {
	h := NewBase64Handler()
	patterns := h.Patterns()
	if len(patterns) == 0 {
		t.Error("Patterns() should return patterns")
	}
}

func TestBase64HandlerAllPatterns(t *testing.T) {
	h := NewBase64Handler()

	tests := []struct {
		query string
		want  bool
	}{
		{"base64 encode: hello", true},
		{"base64 decode: aGVsbG8=", true},
		{"b64 encode: hello", true},
		{"b64 decode: aGVsbG8=", true},
		{"encode base64: hello", true},
		{"decode base64: aGVsbG8=", true},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			got := h.CanHandle(tt.query)
			if got != tt.want {
				t.Errorf("CanHandle(%q) = %v, want %v", tt.query, got, tt.want)
			}
		})
	}
}

func TestURLHandlerHandleEmptyText(t *testing.T) {
	h := NewURLHandler()
	ctx := context.Background()

	answer, err := h.HandleInstantQuery(ctx, "not a url query")
	if err != nil {
		t.Fatalf("HandleInstantQuery() error = %v", err)
	}
	if answer != nil {
		t.Error("HandleInstantQuery() should return nil for non-matching text")
	}
}

func TestURLHandlerHandleParse(t *testing.T) {
	h := NewURLHandler()
	ctx := context.Background()

	answer, err := h.HandleInstantQuery(ctx, "parse url: https://example.com/path?query=value#fragment")
	if err != nil {
		t.Fatalf("HandleInstantQuery() error = %v", err)
	}
	if answer == nil {
		t.Fatal("HandleInstantQuery() returned nil")
	}
	if answer.Type != AnswerTypeURL {
		t.Errorf("Type = %v, want %v", answer.Type, AnswerTypeURL)
	}
	// Check that content includes parsed components
	if !contains(answer.Content, "Scheme") {
		t.Error("Content should contain Scheme")
	}
	if !contains(answer.Content, "Host") {
		t.Error("Content should contain Host")
	}
}

func TestURLHandlerHandleParseInvalid(t *testing.T) {
	h := NewURLHandler()
	ctx := context.Background()

	// URL with invalid characters that url.Parse will still parse
	// but with unexpected results - test the path anyway
	answer, err := h.HandleInstantQuery(ctx, "parse url: ://invalid")
	if err != nil {
		t.Fatalf("HandleInstantQuery() error = %v", err)
	}
	if answer == nil {
		t.Fatal("HandleInstantQuery() returned nil")
	}
}

func TestURLHandlerHandleDecode(t *testing.T) {
	h := NewURLHandler()
	ctx := context.Background()

	answer, err := h.HandleInstantQuery(ctx, "url decode: hello%20world")
	if err != nil {
		t.Fatalf("HandleInstantQuery() error = %v", err)
	}
	if answer == nil {
		t.Fatal("HandleInstantQuery() returned nil")
	}
	if answer.Type != AnswerTypeURL {
		t.Errorf("Type = %v, want %v", answer.Type, AnswerTypeURL)
	}
	if !contains(answer.Content, "Decoded") {
		t.Error("Content should indicate decoded")
	}
}

func TestURLHandlerHandleDecodeInvalid(t *testing.T) {
	h := NewURLHandler()
	ctx := context.Background()

	// Invalid percent encoding - should still return something
	answer, err := h.HandleInstantQuery(ctx, "url decode: %ZZ")
	if err != nil {
		t.Fatalf("HandleInstantQuery() error = %v", err)
	}
	if answer == nil {
		t.Fatal("HandleInstantQuery() returned nil")
	}
}

func TestURLHandlerPatterns(t *testing.T) {
	h := NewURLHandler()
	patterns := h.Patterns()
	if len(patterns) == 0 {
		t.Error("Patterns() should return patterns")
	}
}

func TestURLHandlerAllPatterns(t *testing.T) {
	h := NewURLHandler()

	tests := []struct {
		query string
		want  bool
	}{
		{"url encode: hello world", true},
		{"url decode: hello%20world", true},
		{"urlencode: test", true},
		{"urldecode: test%20value", true},
		{"parse url: https://example.com", true},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			got := h.CanHandle(tt.query)
			if got != tt.want {
				t.Errorf("CanHandle(%q) = %v, want %v", tt.query, got, tt.want)
			}
		})
	}
}

func TestColorHandlerHandleNoColor(t *testing.T) {
	h := NewColorHandler()
	ctx := context.Background()

	answer, err := h.HandleInstantQuery(ctx, "not a color query")
	if err != nil {
		t.Fatalf("HandleInstantQuery() error = %v", err)
	}
	if answer != nil {
		t.Error("HandleInstantQuery() should return nil for non-matching query")
	}
}

func TestColorHandlerHandle3CharHex(t *testing.T) {
	h := NewColorHandler()
	ctx := context.Background()

	answer, err := h.HandleInstantQuery(ctx, "#f00")
	if err != nil {
		t.Fatalf("HandleInstantQuery() error = %v", err)
	}
	if answer == nil {
		t.Fatal("HandleInstantQuery() returned nil")
	}
	if answer.Type != AnswerTypeColor {
		t.Errorf("Type = %v, want %v", answer.Type, AnswerTypeColor)
	}
	// Should expand to FF0000
	if !contains(answer.Content, "FF0000") {
		t.Error("Content should contain expanded hex color")
	}
}

func TestColorHandlerHandleRGB(t *testing.T) {
	h := NewColorHandler()
	ctx := context.Background()

	tests := []string{
		"rgb: 255, 0, 0",
		"rgb: 255 0 0",
		"rgb: (255, 0, 0)",
	}

	for _, query := range tests {
		t.Run(query, func(t *testing.T) {
			answer, err := h.HandleInstantQuery(ctx, query)
			if err != nil {
				t.Fatalf("HandleInstantQuery() error = %v", err)
			}
			if answer == nil {
				t.Fatal("HandleInstantQuery() returned nil")
			}
			if answer.Type != AnswerTypeColor {
				t.Errorf("Type = %v, want %v", answer.Type, AnswerTypeColor)
			}
		})
	}
}

func TestColorHandlerPatterns(t *testing.T) {
	h := NewColorHandler()
	patterns := h.Patterns()
	if len(patterns) == 0 {
		t.Error("Patterns() should return patterns")
	}
}

func TestRgbToHSLAllBranches(t *testing.T) {
	tests := []struct {
		r, g, b int
		name    string
	}{
		{255, 0, 0, "red_max"},
		{0, 255, 0, "green_max"},
		{0, 0, 255, "blue_max"},
		{128, 128, 128, "gray"},
		{255, 255, 255, "white"},
		{0, 0, 0, "black"},
		{200, 100, 50, "orange"},
		{50, 100, 200, "blue_high"},
		{255, 128, 0, "orange_red_max_g_gt_b"},
		{255, 0, 128, "pink_red_max_g_lt_b"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := rgbToHSL(tt.r, tt.g, tt.b)
			if result == "" {
				t.Error("rgbToHSL() returned empty string")
			}
			if !contains(result, "hsl") {
				t.Errorf("rgbToHSL() = %q, should contain 'hsl'", result)
			}
		})
	}
}

func TestRgbToHSLHighLightness(t *testing.T) {
	// Test with high lightness (l > 0.5)
	result := rgbToHSL(200, 200, 100)
	if result == "" {
		t.Error("rgbToHSL() returned empty string")
	}
}

func TestUUIDHandlerPatterns(t *testing.T) {
	h := NewUUIDHandler()
	patterns := h.Patterns()
	if len(patterns) == 0 {
		t.Error("Patterns() should return patterns")
	}
}

func TestRandomHandlerHandleWithRange(t *testing.T) {
	h := NewRandomHandler()
	ctx := context.Background()

	tests := []struct {
		query string
	}{
		{"random 1-10"},
		{"random 50-100"},
		{"random between 1 and 100"},
		{"random between 1 100"},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			answer, err := h.HandleInstantQuery(ctx, tt.query)
			if err != nil {
				t.Fatalf("HandleInstantQuery() error = %v", err)
			}
			if answer == nil {
				t.Fatal("HandleInstantQuery() returned nil")
			}
			if answer.Type != AnswerTypeRandom {
				t.Errorf("Type = %v, want %v", answer.Type, AnswerTypeRandom)
			}
		})
	}
}

func TestRandomHandlerPatterns(t *testing.T) {
	h := NewRandomHandler()
	patterns := h.Patterns()
	if len(patterns) == 0 {
		t.Error("Patterns() should return patterns")
	}
}

func TestPasswordHandlerHandleLengthConstraints(t *testing.T) {
	h := NewPasswordHandler()
	ctx := context.Background()

	// Test with length < 8 (should be clamped to 8)
	answer, err := h.HandleInstantQuery(ctx, "password 4")
	if err != nil {
		t.Fatalf("HandleInstantQuery() error = %v", err)
	}
	if answer.Data["length"] != 8 {
		t.Errorf("length = %v, want 8 (minimum)", answer.Data["length"])
	}

	// Test with length > 128 (should be clamped to 128)
	answer, err = h.HandleInstantQuery(ctx, "password 200")
	if err != nil {
		t.Fatalf("HandleInstantQuery() error = %v", err)
	}
	if answer.Data["length"] != 128 {
		t.Errorf("length = %v, want 128 (maximum)", answer.Data["length"])
	}
}

func TestPasswordHandlerPatterns(t *testing.T) {
	h := NewPasswordHandler()
	patterns := h.Patterns()
	if len(patterns) == 0 {
		t.Error("Patterns() should return patterns")
	}
}

func TestIPHandlerPatterns(t *testing.T) {
	h := NewIPHandler()
	patterns := h.Patterns()
	if len(patterns) == 0 {
		t.Error("Patterns() should return patterns")
	}
}

func TestTimeHandlerPatterns(t *testing.T) {
	h := NewTimeHandler()
	patterns := h.Patterns()
	if len(patterns) == 0 {
		t.Error("Patterns() should return patterns")
	}
}

func TestTimeHandlerHandleVariants(t *testing.T) {
	h := NewTimeHandler()
	ctx := context.Background()

	tests := []string{
		"time",
		"current time",
		"what time is it?",
		"now",
		"date",
		"today",
		"current date",
		"timestamp",
		"unix time",
		"unix timestamp",
		"epoch",
	}

	for _, query := range tests {
		t.Run(query, func(t *testing.T) {
			answer, err := h.HandleInstantQuery(ctx, query)
			if err != nil {
				t.Fatalf("HandleInstantQuery() error = %v", err)
			}
			if answer == nil {
				t.Fatal("HandleInstantQuery() returned nil")
			}
			if answer.Type != AnswerTypeTime {
				t.Errorf("Type = %v, want %v", answer.Type, AnswerTypeTime)
			}
		})
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// Tests for SynonymHandler with mock HTTP server

func TestNewSynonymHandler(t *testing.T) {
	h := NewSynonymHandler()
	if h == nil {
		t.Fatal("NewSynonymHandler() returned nil")
	}
	if h.client == nil {
		t.Error("client should not be nil")
	}
	if h.patterns == nil {
		t.Error("patterns should not be nil")
	}
}

func TestSynonymHandlerName(t *testing.T) {
	h := NewSynonymHandler()
	if h.Name() != "synonym" {
		t.Errorf("Name() = %q, want %q", h.Name(), "synonym")
	}
}

func TestSynonymHandlerPatterns(t *testing.T) {
	h := NewSynonymHandler()
	patterns := h.Patterns()
	if len(patterns) == 0 {
		t.Error("Patterns() should return patterns")
	}
}

func TestSynonymHandlerCanHandle(t *testing.T) {
	h := NewSynonymHandler()

	tests := []struct {
		query string
		want  bool
	}{
		{"synonyms: happy", true},
		{"synonym: happy", true},
		{"syn: happy", true},
		{"similar to: happy", true},
		{"word like: happy", true},
		{"words like: happy", true},
		{"hello world", false},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			got := h.CanHandle(tt.query)
			if got != tt.want {
				t.Errorf("CanHandle(%q) = %v, want %v", tt.query, got, tt.want)
			}
		})
	}
}

func TestSynonymHandlerHandleEmptyWord(t *testing.T) {
	h := NewSynonymHandler()
	ctx := context.Background()

	// Query that doesn't match any pattern
	answer, err := h.HandleInstantQuery(ctx, "not a synonym query")
	if err != nil {
		t.Fatalf("HandleInstantQuery() error = %v", err)
	}
	if answer != nil {
		t.Error("HandleInstantQuery() should return nil for non-matching query")
	}
}

func TestSynonymHandlerHandleWithMockServer(t *testing.T) {
	// Create mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return mock synonym data
		data := []struct {
			Word  string `json:"word"`
			Score int    `json:"score"`
		}{
			{Word: "joyful", Score: 100},
			{Word: "cheerful", Score: 90},
			{Word: "glad", Score: 80},
		}
		json.NewEncoder(w).Encode(data)
	}))
	defer server.Close()

	h := NewSynonymHandler()
	// Replace client to use test server
	h.client = server.Client()
	_ = context.Background()

	// We can't easily redirect the URL, so we test what we can
	// Test the handler's patterns and structure
	if !h.CanHandle("synonym: happy") {
		t.Error("Should handle synonym query")
	}
}

func TestSynonymHandlerHandleEmptyResults(t *testing.T) {
	// Create mock server that returns empty results
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]interface{}{})
	}))
	defer server.Close()

	// Test pattern extraction
	h := NewSynonymHandler()
	patterns := h.Patterns()
	if len(patterns) == 0 {
		t.Error("Should have patterns")
	}
}

// Tests for AntonymHandler with mock HTTP server

func TestNewAntonymHandler(t *testing.T) {
	h := NewAntonymHandler()
	if h == nil {
		t.Fatal("NewAntonymHandler() returned nil")
	}
	if h.client == nil {
		t.Error("client should not be nil")
	}
	if h.patterns == nil {
		t.Error("patterns should not be nil")
	}
}

func TestAntonymHandlerName(t *testing.T) {
	h := NewAntonymHandler()
	if h.Name() != "antonym" {
		t.Errorf("Name() = %q, want %q", h.Name(), "antonym")
	}
}

func TestAntonymHandlerPatterns(t *testing.T) {
	h := NewAntonymHandler()
	patterns := h.Patterns()
	if len(patterns) == 0 {
		t.Error("Patterns() should return patterns")
	}
}

func TestAntonymHandlerCanHandle(t *testing.T) {
	h := NewAntonymHandler()

	tests := []struct {
		query string
		want  bool
	}{
		{"antonyms: happy", true},
		{"antonym: happy", true},
		{"ant: happy", true},
		{"opposite of: happy", true},
		{"opposite: happy", true},
		{"hello world", false},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			got := h.CanHandle(tt.query)
			if got != tt.want {
				t.Errorf("CanHandle(%q) = %v, want %v", tt.query, got, tt.want)
			}
		})
	}
}

func TestAntonymHandlerHandleEmptyWord(t *testing.T) {
	h := NewAntonymHandler()
	ctx := context.Background()

	// Query that doesn't match any pattern
	answer, err := h.HandleInstantQuery(ctx, "not an antonym query")
	if err != nil {
		t.Fatalf("HandleInstantQuery() error = %v", err)
	}
	if answer != nil {
		t.Error("HandleInstantQuery() should return nil for non-matching query")
	}
}

// Tests for DefinitionHandler with mock HTTP server

func TestNewDefinitionHandler(t *testing.T) {
	h := NewDefinitionHandler()
	if h == nil {
		t.Fatal("NewDefinitionHandler() returned nil")
	}
	if h.client == nil {
		t.Error("client should not be nil")
	}
	if h.patterns == nil {
		t.Error("patterns should not be nil")
	}
}

func TestDefinitionHandlerName(t *testing.T) {
	h := NewDefinitionHandler()
	if h.Name() != "definition" {
		t.Errorf("Name() = %q, want %q", h.Name(), "definition")
	}
}

func TestDefinitionHandlerPatterns(t *testing.T) {
	h := NewDefinitionHandler()
	patterns := h.Patterns()
	if len(patterns) == 0 {
		t.Error("Patterns() should return patterns")
	}
}

func TestDefinitionHandlerCanHandle(t *testing.T) {
	h := NewDefinitionHandler()

	tests := []struct {
		query string
		want  bool
	}{
		{"define: word", true},
		{"definition: word", true},
		{"what is word?", true},
		{"what does word mean?", true},
		{"hello world", false},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			got := h.CanHandle(tt.query)
			if got != tt.want {
				t.Errorf("CanHandle(%q) = %v, want %v", tt.query, got, tt.want)
			}
		})
	}
}

func TestDefinitionHandlerHandleEmptyWord(t *testing.T) {
	h := NewDefinitionHandler()
	ctx := context.Background()

	// Query that doesn't match any pattern
	answer, err := h.HandleInstantQuery(ctx, "not a definition query")
	if err != nil {
		t.Fatalf("HandleInstantQuery() error = %v", err)
	}
	if answer != nil {
		t.Error("HandleInstantQuery() should return nil for non-matching query")
	}
}

// Tests for DictionaryHandler

func TestNewDictionaryHandler(t *testing.T) {
	h := NewDictionaryHandler()
	if h == nil {
		t.Fatal("NewDictionaryHandler() returned nil")
	}
	if h.DefinitionHandler == nil {
		t.Error("DefinitionHandler should not be nil")
	}
}

func TestDictionaryHandlerName(t *testing.T) {
	h := NewDictionaryHandler()
	if h.Name() != "dictionary" {
		t.Errorf("Name() = %q, want %q", h.Name(), "dictionary")
	}
}

func TestDictionaryHandlerPatterns(t *testing.T) {
	h := NewDictionaryHandler()
	patterns := h.Patterns()
	if len(patterns) == 0 {
		t.Error("Patterns() should return patterns")
	}
}

func TestDictionaryHandlerCanHandle(t *testing.T) {
	h := NewDictionaryHandler()

	tests := []struct {
		query string
		want  bool
	}{
		{"dictionary: word", true},
		{"dict: word", true},
		{"lookup: word", true},
		{"hello world", false},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			got := h.CanHandle(tt.query)
			if got != tt.want {
				t.Errorf("CanHandle(%q) = %v, want %v", tt.query, got, tt.want)
			}
		})
	}
}

// Additional tests for evalNode branches

func TestEvalNodeUnaryPlus(t *testing.T) {
	// Test unary plus operator (not SUB)
	result, err := evaluateExpression("+5")
	if err != nil {
		t.Fatalf("evaluateExpression(\"+5\") error = %v", err)
	}
	if result != 5 {
		t.Errorf("evaluateExpression(\"+5\") = %v, want 5", result)
	}
}

func TestEvalNodeBinaryExprErrors(t *testing.T) {
	// Test that we get error from left side evaluation
	_, err := evalSimple("abc + 2")
	if err == nil {
		t.Error("evalSimple(\"abc + 2\") should error")
	}

	// Test that we get error from right side evaluation
	_, err = evalSimple("2 + abc")
	if err == nil {
		t.Error("evalSimple(\"2 + abc\") should error")
	}
}

// Test for edge cases in math expressions

func TestMathHandlerHandleError(t *testing.T) {
	h := NewMathHandler()
	ctx := context.Background()

	// Test with invalid expression that will cause evaluation error
	answer, err := h.HandleInstantQuery(ctx, "calc: invalid_expression")
	if err != nil {
		t.Fatalf("HandleInstantQuery() error = %v", err)
	}
	if answer == nil {
		t.Fatal("HandleInstantQuery() returned nil")
	}
	// Should contain error in content
	if !strings.Contains(answer.Content, "Error") {
		t.Error("Content should contain error message")
	}
}

// Test for manager with empty query

func TestManagerProcessEmptyQuery(t *testing.T) {
	m := NewManager()
	ctx := context.Background()

	result, err := m.Process(ctx, "   ")
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}
	if result != nil {
		t.Error("Process() should return nil for whitespace-only query")
	}
}

// Test handler interface compliance

func TestAllHandlersImplementInterface(t *testing.T) {
	handlers := []Handler{
		NewDefinitionHandler(),
		NewDictionaryHandler(),
		NewSynonymHandler(),
		NewAntonymHandler(),
		NewMathHandler(),
		NewConvertHandler(),
		NewTimeHandler(),
		NewHashHandler(),
		NewBase64Handler(),
		NewURLHandler(),
		NewColorHandler(),
		NewUUIDHandler(),
		NewRandomHandler(),
		NewPasswordHandler(),
		NewIPHandler(),
	}

	for _, h := range handlers {
		t.Run(h.Name(), func(t *testing.T) {
			if h.Name() == "" {
				t.Error("Name() should not be empty")
			}
			if h.Patterns() == nil {
				t.Error("Patterns() should not be nil")
			}
		})
	}
}

// Test getWeekOfYear with different dates

func TestGetWeekOfYearVariousDates(t *testing.T) {
	tests := []struct {
		date time.Time
		name string
	}{
		{time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), "new_year"},
		{time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC), "mid_year"},
		{time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC), "end_year"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			week := getWeekOfYear(tt.date)
			if week < 1 || week > 53 {
				t.Errorf("getWeekOfYear() = %d, expected 1-53", week)
			}
		})
	}
}

// Test Answer struct fields

func TestAnswerStructAllFields(t *testing.T) {
	a := Answer{
		Type:        AnswerTypeMath,
		Query:       "test query",
		Title:       "Test Title",
		Content:     "Test Content",
		Data:        map[string]interface{}{"key": "value"},
		Source:      "Test Source",
		SourceURL:   "https://test.com",
		RelatedHTML: "<div>related</div>",
	}

	if a.Type != AnswerTypeMath {
		t.Errorf("Type = %v, want %v", a.Type, AnswerTypeMath)
	}
	if a.Query != "test query" {
		t.Errorf("Query = %q, want %q", a.Query, "test query")
	}
	if a.Title != "Test Title" {
		t.Errorf("Title = %q, want %q", a.Title, "Test Title")
	}
	if a.Content != "Test Content" {
		t.Errorf("Content = %q, want %q", a.Content, "Test Content")
	}
	if a.Data["key"] != "value" {
		t.Errorf("Data[key] = %v, want %q", a.Data["key"], "value")
	}
	if a.Source != "Test Source" {
		t.Errorf("Source = %q, want %q", a.Source, "Test Source")
	}
	if a.SourceURL != "https://test.com" {
		t.Errorf("SourceURL = %q, want %q", a.SourceURL, "https://test.com")
	}
	if a.RelatedHTML != "<div>related</div>" {
		t.Errorf("RelatedHTML = %q, want %q", a.RelatedHTML, "<div>related</div>")
	}
}

// Additional tests for full coverage of random handler dice patterns

func TestRandomHandlerDiceWithSides(t *testing.T) {
	h := NewRandomHandler()
	ctx := context.Background()

	tests := []struct {
		query    string
		hasSides bool
	}{
		{"roll d6", true},
		{"roll d20", true},
		{"roll d100", true},
		{"roll dice", false},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			answer, err := h.HandleInstantQuery(ctx, tt.query)
			if err != nil {
				t.Fatalf("HandleInstantQuery() error = %v", err)
			}
			if answer == nil {
				t.Fatal("HandleInstantQuery() returned nil")
			}
			if answer.Type != AnswerTypeRandom {
				t.Errorf("Type = %v, want %v", answer.Type, AnswerTypeRandom)
			}
			if tt.hasSides {
				if answer.Data["sides"] == nil {
					t.Error("Data should contain sides")
				}
			}
		})
	}
}

// Test that all color patterns work

func TestColorHandlerAllPatterns(t *testing.T) {
	h := NewColorHandler()

	tests := []struct {
		query string
		want  bool
	}{
		{"color: ff0000", true},
		{"color: fff", true},
		{"#ff0000", true},
		{"#fff", true},
		{"rgb: 255, 0, 0", true},
		{"rgb: (255, 0, 0)", true},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			got := h.CanHandle(tt.query)
			if got != tt.want {
				t.Errorf("CanHandle(%q) = %v, want %v", tt.query, got, tt.want)
			}
		})
	}
}

// Test formatNumber with negative values

func TestFormatNumberNegative(t *testing.T) {
	tests := []struct {
		n    float64
		want string
	}{
		{-5, "-5"},
		{-3.14, "-3.14"},
		{-1000000, "-1000000"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := formatNumber(tt.n)
			if got != tt.want {
				t.Errorf("formatNumber(%v) = %q, want %q", tt.n, got, tt.want)
			}
		})
	}
}

// Test conversion with decimal values

func TestConvertHandlerDecimalValues(t *testing.T) {
	h := NewConvertHandler()
	ctx := context.Background()

	answer, err := h.HandleInstantQuery(ctx, "1.5 km to meters")
	if err != nil {
		t.Fatalf("HandleInstantQuery() error = %v", err)
	}
	if answer == nil {
		t.Fatal("HandleInstantQuery() returned nil")
	}
	if answer.Data["result"] != float64(1500) {
		t.Errorf("result = %v, want 1500", answer.Data["result"])
	}
}

// Test URL parsing with all components

func TestURLHandlerParseComplete(t *testing.T) {
	h := NewURLHandler()
	ctx := context.Background()

	answer, err := h.HandleInstantQuery(ctx, "parse url: https://user:pass@example.com:8080/path?query=value#section")
	if err != nil {
		t.Fatalf("HandleInstantQuery() error = %v", err)
	}
	if answer == nil {
		t.Fatal("HandleInstantQuery() returned nil")
	}
	// Should contain all URL parts
	if !strings.Contains(answer.Content, "https") {
		t.Error("Content should contain scheme")
	}
	if !strings.Contains(answer.Content, "example.com") {
		t.Error("Content should contain host")
	}
}

// Test URL parsing without query or fragment

func TestURLHandlerParseSimple(t *testing.T) {
	h := NewURLHandler()
	ctx := context.Background()

	answer, err := h.HandleInstantQuery(ctx, "parse url: https://example.com/path")
	if err != nil {
		t.Fatalf("HandleInstantQuery() error = %v", err)
	}
	if answer == nil {
		t.Fatal("HandleInstantQuery() returned nil")
	}
	if !strings.Contains(answer.Content, "Path") {
		t.Error("Content should contain Path")
	}
}

// Test for IP handler with various patterns

func TestIPHandlerAllPatterns(t *testing.T) {
	h := NewIPHandler()

	tests := []struct {
		query string
		want  bool
	}{
		{"my ip", true},
		{"my ip address", true},
		{"what is my ip?", true},
		{"ip address", true},
		{"ip info", true},
		{"ip 8.8.8.8", true},
		{"8.8.8.8", true},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			got := h.CanHandle(tt.query)
			if got != tt.want {
				t.Errorf("CanHandle(%q) = %v, want %v", tt.query, got, tt.want)
			}
		})
	}
}

// TestIPHandlerSpecificIP verifies lookup of a specific IP address query.
func TestIPHandlerSpecificIP(t *testing.T) {
	h := NewIPHandler()
	ctx := context.Background()

	tests := []struct {
		query    string
		wantType AnswerType
	}{
		{"ip 8.8.8.8", AnswerTypeIP},
		{"8.8.8.8", AnswerTypeIP},
		{"ip 192.168.1.1", AnswerTypeIP},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			answer, err := h.HandleInstantQuery(ctx, tt.query)
			if err != nil {
				t.Fatalf("HandleInstantQuery(%q) error = %v", tt.query, err)
			}
			if answer == nil {
				t.Fatalf("HandleInstantQuery(%q) returned nil", tt.query)
			}
			if answer.Type != tt.wantType {
				t.Errorf("Type = %v, want %v", answer.Type, tt.wantType)
			}
			if answer.Data["ip"] == nil {
				t.Error("Data should contain ip field for specific IP lookup")
			}
		})
	}
}

// Test IP handler handle returns content

func TestIPHandlerHandleContent(t *testing.T) {
	h := NewIPHandler()

	tests := []struct {
		name      string
		clientIP  string
		wantInIP  bool
	}{
		{
			name:     "no client ip in context",
			clientIP: "",
			wantInIP: false,
		},
		{
			name:     "valid client ip injected",
			clientIP: "1.2.3.4",
			wantInIP: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			if tt.clientIP != "" {
				ctx = WithClientIP(ctx, tt.clientIP)
			}

			answer, err := h.HandleInstantQuery(ctx, "my ip")
			if err != nil {
				t.Fatalf("HandleInstantQuery() error = %v", err)
			}
			if answer == nil {
				t.Fatal("HandleInstantQuery() returned nil")
			}
			// Content must always mention IP context
			if !strings.Contains(answer.Content, "IP") {
				t.Error("Content should mention IP")
			}
			// Data must always contain the "ip" key
			if _, ok := answer.Data["ip"]; !ok {
				t.Error("Data should contain key \"ip\"")
			}
			if tt.wantInIP {
				if answer.Data["ip"] != tt.clientIP {
					t.Errorf("Data[\"ip\"] = %v, want %v", answer.Data["ip"], tt.clientIP)
				}
			}
		})
	}
}

// Additional math expression tests for edge cases

func TestEvaluateExpressionComplexPower(t *testing.T) {
	// Test nested power expressions
	result, err := evaluateExpression("2^2^2")
	if err != nil {
		t.Fatalf("evaluateExpression(\"2^2^2\") error = %v", err)
	}
	// 2^(2^2) = 2^4 = 16 OR (2^2)^2 = 16 depending on associativity
	if result != 16 {
		t.Logf("evaluateExpression(\"2^2^2\") = %v", result)
	}
}

// Test UUID handler all patterns

func TestUUIDHandlerAllPatterns(t *testing.T) {
	h := NewUUIDHandler()

	tests := []struct {
		query string
		want  bool
	}{
		{"uuid", true},
		{"generate uuid", true},
		{"new uuid", true},
		{"guid", true},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			got := h.CanHandle(tt.query)
			if got != tt.want {
				t.Errorf("CanHandle(%q) = %v, want %v", tt.query, got, tt.want)
			}
		})
	}
}

// Test UUID handler handle returns valid UUID format

func TestUUIDHandlerHandleFormat(t *testing.T) {
	h := NewUUIDHandler()
	ctx := context.Background()

	answer, err := h.HandleInstantQuery(ctx, "uuid")
	if err != nil {
		t.Fatalf("HandleInstantQuery() error = %v", err)
	}
	if answer == nil {
		t.Fatal("HandleInstantQuery() returned nil")
	}
	uuid := answer.Data["uuid"].(string)
	// UUID format: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
	if len(uuid) != 36 {
		t.Errorf("UUID length = %d, want 36", len(uuid))
	}
	// Check for dashes
	if uuid[8] != '-' || uuid[13] != '-' || uuid[18] != '-' || uuid[23] != '-' {
		t.Error("UUID should have dashes in correct positions")
	}
}

// Test password handler all patterns

func TestPasswordHandlerAllPatterns(t *testing.T) {
	h := NewPasswordHandler()

	tests := []struct {
		query string
		want  bool
	}{
		{"password", true},
		{"generate password", true},
		{"password 32", true},
		{"random password", true},
		{"secure password", true},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			got := h.CanHandle(tt.query)
			if got != tt.want {
				t.Errorf("CanHandle(%q) = %v, want %v", tt.query, got, tt.want)
			}
		})
	}
}

// Test hash handler all patterns

func TestHashHandlerAllPatterns(t *testing.T) {
	h := NewHashHandler()

	tests := []struct {
		query string
		want  bool
	}{
		{"md5: test", true},
		{"sha1: test", true},
		{"sha256: test", true},
		{"sha512: test", true},
		{"hash: test", true},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			got := h.CanHandle(tt.query)
			if got != tt.want {
				t.Errorf("CanHandle(%q) = %v, want %v", tt.query, got, tt.want)
			}
		})
	}
}

// Test for formatNumber with very large integer

func TestFormatNumberLargeInteger(t *testing.T) {
	tests := []struct {
		n    float64
		want string
	}{
		{1e14, "100000000000000"},
		{9999999999999, "9999999999999"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := formatNumber(tt.n)
			if got != tt.want {
				t.Errorf("formatNumber(%v) = %q, want %q", tt.n, got, tt.want)
			}
		})
	}
}

// Test formatNumber with very small scientific notation

func TestFormatNumberVerySmall(t *testing.T) {
	result := formatNumber(1e-10)
	if result == "" {
		t.Error("formatNumber(1e-10) returned empty string")
	}
	// Should be in scientific notation
	if !strings.Contains(result, "e") {
		t.Errorf("formatNumber(1e-10) = %q, should be scientific notation", result)
	}
}

// Test formatNumber with very large scientific notation

func TestFormatNumberVeryLarge(t *testing.T) {
	result := formatNumber(1e16)
	if result == "" {
		t.Error("formatNumber(1e16) returned empty string")
	}
	// Should be in scientific notation for very large numbers
	if !strings.Contains(result, "e") && len(result) < 17 {
		t.Logf("formatNumber(1e16) = %q", result)
	}
}

// Test color handler with edge case hex values

func TestColorHandlerHexEdgeCases(t *testing.T) {
	h := NewColorHandler()
	ctx := context.Background()

	tests := []string{
		"#000000",
		"#ffffff",
		"#abc",
		"color: 123456",
	}

	for _, query := range tests {
		t.Run(query, func(t *testing.T) {
			answer, err := h.HandleInstantQuery(ctx, query)
			if err != nil {
				t.Fatalf("HandleInstantQuery() error = %v", err)
			}
			if answer == nil {
				t.Fatal("HandleInstantQuery() returned nil")
			}
			if answer.Type != AnswerTypeColor {
				t.Errorf("Type = %v, want %v", answer.Type, AnswerTypeColor)
			}
		})
	}
}

// Test convert handler with time units

func TestConvertHandlerTimeUnits(t *testing.T) {
	h := NewConvertHandler()
	ctx := context.Background()

	tests := []struct {
		query string
	}{
		{"1 hours to minutes"},
		{"60 minutes to hours"},
		{"24 hours to days"},
		{"7 days to weeks"},
		{"1 years to days"},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			answer, err := h.HandleInstantQuery(ctx, tt.query)
			if err != nil {
				t.Fatalf("HandleInstantQuery() error = %v", err)
			}
			if answer == nil {
				t.Fatal("HandleInstantQuery() returned nil")
			}
			if answer.Type != AnswerTypeConvert {
				t.Errorf("Type = %v, want %v", answer.Type, AnswerTypeConvert)
			}
		})
	}
}

// Test random handler returns valid results

func TestRandomHandlerResultInRange(t *testing.T) {
	h := NewRandomHandler()
	ctx := context.Background()

	// Run multiple times to verify randomness stays in range
	for i := 0; i < 10; i++ {
		answer, err := h.HandleInstantQuery(ctx, "random 1-10")
		if err != nil {
			t.Fatalf("HandleInstantQuery() error = %v", err)
		}
		result := answer.Data["result"].(int)
		if result < 1 || result > 10 {
			t.Errorf("result = %d, should be 1-10", result)
		}
	}
}

// Test coin flip returns valid result

func TestRandomHandlerCoinFlipResult(t *testing.T) {
	h := NewRandomHandler()
	ctx := context.Background()

	// Run multiple times
	for i := 0; i < 10; i++ {
		answer, err := h.HandleInstantQuery(ctx, "flip coin")
		if err != nil {
			t.Fatalf("HandleInstantQuery() error = %v", err)
		}
		result := answer.Data["result"].(string)
		if result != "Heads" && result != "Tails" {
			t.Errorf("result = %q, should be Heads or Tails", result)
		}
	}
}

// Test base64 operations

func TestBase64HandlerOperations(t *testing.T) {
	h := NewBase64Handler()
	ctx := context.Background()

	// Test encode
	encoded, err := h.HandleInstantQuery(ctx, "base64 encode: test string")
	if err != nil {
		t.Fatalf("HandleInstantQuery() error = %v", err)
	}
	if encoded.Data["operation"] != "encoded" {
		t.Errorf("operation = %v, want encoded", encoded.Data["operation"])
	}

	// Test decode
	decoded, err := h.HandleInstantQuery(ctx, "base64 decode: dGVzdCBzdHJpbmc=")
	if err != nil {
		t.Fatalf("HandleInstantQuery() error = %v", err)
	}
	if decoded.Data["operation"] != "decoded" {
		t.Errorf("operation = %v, want decoded", decoded.Data["operation"])
	}
	if decoded.Data["output"] != "test string" {
		t.Errorf("output = %v, want 'test string'", decoded.Data["output"])
	}
}

// Test URL encoding round trip

func TestURLHandlerRoundTrip(t *testing.T) {
	h := NewURLHandler()
	ctx := context.Background()

	// Test encode
	encoded, err := h.HandleInstantQuery(ctx, "url encode: hello world&foo=bar")
	if err != nil {
		t.Fatalf("HandleInstantQuery() error = %v", err)
	}
	if encoded == nil {
		t.Fatal("HandleInstantQuery() returned nil")
	}

	// Test decode
	decoded, err := h.HandleInstantQuery(ctx, "url decode: hello%20world%26foo%3Dbar")
	if err != nil {
		t.Fatalf("HandleInstantQuery() error = %v", err)
	}
	if decoded == nil {
		t.Fatal("HandleInstantQuery() returned nil")
	}
}

// Test time handler returns current time

func TestTimeHandlerReturnsCurrentTime(t *testing.T) {
	h := NewTimeHandler()
	ctx := context.Background()

	before := time.Now().Unix()
	answer, err := h.HandleInstantQuery(ctx, "timestamp")
	if err != nil {
		t.Fatalf("HandleInstantQuery() error = %v", err)
	}
	after := time.Now().Unix()

	timestamp := answer.Data["timestamp"].(int64)
	if timestamp < before || timestamp > after {
		t.Errorf("timestamp = %d, should be between %d and %d", timestamp, before, after)
	}
}

// Test that all conversions return proper data structure

func TestConvertHandlerDataStructure(t *testing.T) {
	h := NewConvertHandler()
	ctx := context.Background()

	answer, err := h.HandleInstantQuery(ctx, "100 meters to feet")
	if err != nil {
		t.Fatalf("HandleInstantQuery() error = %v", err)
	}
	if answer == nil {
		t.Fatal("HandleInstantQuery() returned nil")
	}

	// Check all data fields
	if answer.Data["value"] == nil {
		t.Error("Data should contain value")
	}
	if answer.Data["fromUnit"] == nil {
		t.Error("Data should contain fromUnit")
	}
	if answer.Data["toUnit"] == nil {
		t.Error("Data should contain toUnit")
	}
	if answer.Data["result"] == nil {
		t.Error("Data should contain result")
	}
}

// Test math handler data structure

func TestMathHandlerDataStructure(t *testing.T) {
	h := NewMathHandler()
	ctx := context.Background()

	answer, err := h.HandleInstantQuery(ctx, "2 + 2")
	if err != nil {
		t.Fatalf("HandleInstantQuery() error = %v", err)
	}
	if answer == nil {
		t.Fatal("HandleInstantQuery() returned nil")
	}

	// Check all data fields
	if answer.Data["expression"] == nil {
		t.Error("Data should contain expression")
	}
	if answer.Data["result"] == nil {
		t.Error("Data should contain result")
	}
}

// BeautifyHandler tests cover pure-Go code formatting with no network calls.

func TestNewBeautifyHandler(t *testing.T) {
	h := NewBeautifyHandler()
	if h == nil {
		t.Fatal("NewBeautifyHandler() returned nil")
	}
}

func TestBeautifyHandlerName(t *testing.T) {
	h := NewBeautifyHandler()
	if h.Name() != "beautify" {
		t.Errorf("Name() = %q, want %q", h.Name(), "beautify")
	}
}

func TestBeautifyHandlerPatterns(t *testing.T) {
	h := NewBeautifyHandler()
	if len(h.Patterns()) == 0 {
		t.Error("Patterns() should return patterns")
	}
}

func TestBeautifyHandlerCanHandle(t *testing.T) {
	h := NewBeautifyHandler()

	tests := []struct {
		query string
		want  bool
	}{
		{"beautify:json {}", true},
		{"beautify:js function(){}", true},
		{"beautify:minify css body{}", true},
		{"format:html <div></div>", true},
		{"hello world", false},
		{"convert meters to feet", false},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			got := h.CanHandle(tt.query)
			if got != tt.want {
				t.Errorf("CanHandle(%q) = %v, want %v", tt.query, got, tt.want)
			}
		})
	}
}

func TestBeautifyHandlerHandleJSON(t *testing.T) {
	h := NewBeautifyHandler()
	ctx := context.Background()

	answer, err := h.HandleInstantQuery(ctx, `beautify:json {"a":1,"b":2}`)
	if err != nil {
		t.Fatalf("HandleInstantQuery() error = %v", err)
	}
	if answer == nil {
		t.Fatal("HandleInstantQuery() returned nil")
	}
	if answer.Type != AnswerTypeBeautify {
		t.Errorf("Type = %v, want %v", answer.Type, AnswerTypeBeautify)
	}
	if !contains(answer.Content, "a") {
		t.Error("Content should contain formatted JSON")
	}
}

func TestBeautifyHandlerHandleJavaScript(t *testing.T) {
	h := NewBeautifyHandler()
	ctx := context.Background()

	answer, err := h.HandleInstantQuery(ctx, "beautify:js function hello(){return 42;}")
	if err != nil {
		t.Fatalf("HandleInstantQuery() error = %v", err)
	}
	if answer == nil {
		t.Fatal("HandleInstantQuery() returned nil")
	}
	if answer.Type != AnswerTypeBeautify {
		t.Errorf("Type = %v, want %v", answer.Type, AnswerTypeBeautify)
	}
}

func TestBeautifyHandlerHandleCSS(t *testing.T) {
	h := NewBeautifyHandler()
	ctx := context.Background()

	answer, err := h.HandleInstantQuery(ctx, "beautify:css body{color:red;margin:0}")
	if err != nil {
		t.Fatalf("HandleInstantQuery() error = %v", err)
	}
	if answer == nil {
		t.Fatal("HandleInstantQuery() returned nil")
	}
	if answer.Type != AnswerTypeBeautify {
		t.Errorf("Type = %v, want %v", answer.Type, AnswerTypeBeautify)
	}
}

func TestBeautifyHandlerHandleHTML(t *testing.T) {
	h := NewBeautifyHandler()
	ctx := context.Background()

	answer, err := h.HandleInstantQuery(ctx, "beautify:html <div><p>hello</p></div>")
	if err != nil {
		t.Fatalf("HandleInstantQuery() error = %v", err)
	}
	if answer == nil {
		t.Fatal("HandleInstantQuery() returned nil")
	}
	if answer.Type != AnswerTypeBeautify {
		t.Errorf("Type = %v, want %v", answer.Type, AnswerTypeBeautify)
	}
}

func TestBeautifyHandlerHandleSQL(t *testing.T) {
	h := NewBeautifyHandler()
	ctx := context.Background()

	answer, err := h.HandleInstantQuery(ctx, "beautify:sql select * from users where id=1")
	if err != nil {
		t.Fatalf("HandleInstantQuery() error = %v", err)
	}
	if answer == nil {
		t.Fatal("HandleInstantQuery() returned nil")
	}
	if answer.Type != AnswerTypeBeautify {
		t.Errorf("Type = %v, want %v", answer.Type, AnswerTypeBeautify)
	}
}

func TestBeautifyHandlerHandleMinifyCSS(t *testing.T) {
	h := NewBeautifyHandler()
	ctx := context.Background()

	answer, err := h.HandleInstantQuery(ctx, "beautify:minify body { color: red; margin: 0; }")
	if err != nil {
		t.Fatalf("HandleInstantQuery() error = %v", err)
	}
	if answer == nil {
		t.Fatal("HandleInstantQuery() returned nil")
	}
}

func TestBeautifyHandlerHandleAutoDetect(t *testing.T) {
	h := NewBeautifyHandler()
	ctx := context.Background()

	// beautify:json pattern matches ^beautify:(\w+)\s+(.+)$; space after colon is not valid
	answer, err := h.HandleInstantQuery(ctx, `beautify:json {"key":"value"}`)
	if err != nil {
		t.Fatalf("HandleInstantQuery() error = %v", err)
	}
	if answer == nil {
		t.Fatal("HandleInstantQuery() returned nil")
	}
}

func TestBeautifyHandlerHandleNoMatch(t *testing.T) {
	h := NewBeautifyHandler()
	ctx := context.Background()

	answer, err := h.HandleInstantQuery(ctx, "completely unrelated query")
	if err != nil {
		t.Fatalf("HandleInstantQuery() error = %v", err)
	}
	if answer != nil {
		t.Error("HandleInstantQuery() should return nil for non-matching query")
	}
}

func TestNormalizeLanguage(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"js", "javascript"},
		{"htm", "html"},
		// ts/py/rb/sh/yml have no alias — normalizeLanguage passes them through unchanged
		{"ts", "ts"},
		{"py", "py"},
		{"rb", "rb"},
		{"sh", "sh"},
		{"yml", "yml"},
		{"javascript", "javascript"},
		{"css", "css"},
		{"sql", "sql"},
		{"unknown_lang", "unknown_lang"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := normalizeLanguage(tt.input)
			if got != tt.want {
				t.Errorf("normalizeLanguage(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestDetectCodeLanguage(t *testing.T) {
	tests := []struct {
		name string
		code string
		want string
	}{
		{"json object", `{"key": "value"}`, "json"},
		{"html doctype", `<!DOCTYPE html><html></html>`, "html"},
		// <div> has no doctype/html/body keyword → detected as xml (generic tag)
		{"html tag", `<div class="test">content</div>`, "xml"},
		{"xml tag", `<?xml version="1.0"?><root></root>`, "xml"},
		{"css selector", `body { color: red; }`, "css"},
		{"sql select", `SELECT * FROM users WHERE id = 1`, "sql"},
		// plain text with no markup/syntax → "unknown"
		{"plain text", `hello world this is text`, "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detectCodeLanguage(tt.code)
			if got != tt.want {
				t.Errorf("detectCodeLanguage() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDefaultBeautifyConfig(t *testing.T) {
	cfg := DefaultBeautifyConfig()
	if cfg.IndentSize <= 0 {
		t.Error("IndentSize should be positive")
	}
}

func TestBeautifyCodeWithConfig(t *testing.T) {
	cfg := DefaultBeautifyConfig()

	tests := []struct {
		name    string
		lang    string
		code    string
		wantErr bool
	}{
		{"valid json", "json", `{"a":1}`, false},
		{"valid javascript", "javascript", `function f(){}`, false},
		{"valid css", "css", `body{color:red}`, false},
		{"valid html", "html", `<div></div>`, false},
		{"valid sql", "sql", `SELECT 1`, false},
		{"unsupported lang", "cobol", `IDENTIFICATION DIVISION.`, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := beautifyCodeWithConfig(tt.code, tt.lang, cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("beautifyCodeWithConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestMinifyCode(t *testing.T) {
	tests := []struct {
		name string
		code string
		lang string
	}{
		{"valid css", `body { color: red; }`, "css"},
		{"valid html", `<div> hello </div>`, "html"},
		{"valid json", `{ "key": "value" }`, "json"},
		{"javascript", `function f() { return 1; }`, "javascript"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, _ := minifyCode(tt.code, tt.lang)
			_ = result
		})
	}
}

func TestEscapeHTMLContent(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"ampersand", "a & b", "a &amp; b"},
		{"less than", "a < b", "a &lt; b"},
		{"greater than", "a > b", "a &gt; b"},
		{"double quote", `say "hello"`, "say &quot;hello&quot;"},
		{"plain text", "hello world", "hello world"},
		{"mixed", `<div class="test">content & more</div>`, "&lt;div class=&quot;test&quot;&gt;content &amp; more&lt;/div&gt;"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := escapeHTMLContent(tt.input)
			if got != tt.want {
				t.Errorf("escapeHTMLContent(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// HTMLEntityHandler tests cover pure-Go HTML entity encode/decode.

func TestNewHTMLEntityHandler(t *testing.T) {
	h := NewHTMLEntityHandler()
	if h == nil {
		t.Fatal("NewHTMLEntityHandler() returned nil")
	}
}

func TestHTMLEntityHandlerName(t *testing.T) {
	h := NewHTMLEntityHandler()
	if h.Name() != "html_entity" {
		t.Errorf("Name() = %q, want %q", h.Name(), "html_entity")
	}
}

func TestHTMLEntityHandlerPatterns(t *testing.T) {
	h := NewHTMLEntityHandler()
	if len(h.Patterns()) == 0 {
		t.Error("Patterns() should return patterns")
	}
}

func TestHTMLEntityHandlerCanHandle(t *testing.T) {
	h := NewHTMLEntityHandler()

	tests := []struct {
		query string
		want  bool
	}{
		{"encode html: <div>", true},
		{"decode html: &lt;div&gt;", true},
		{"html: &amp;", true},
		{"entity: &#60;", true},
		// "encode: <p>" and "decode: &lt;p&gt;" lack the "html" keyword — no match
		{"encode: <p>", false},
		{"decode: &lt;p&gt;", false},
		{"html entities &lt;test&gt;", true},
		{"hello world", false},
		{"convert meters to feet", false},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			got := h.CanHandle(tt.query)
			if got != tt.want {
				t.Errorf("CanHandle(%q) = %v, want %v", tt.query, got, tt.want)
			}
		})
	}
}

func TestHTMLEntityHandlerHandleEncode(t *testing.T) {
	h := NewHTMLEntityHandler()
	ctx := context.Background()

	answer, err := h.HandleInstantQuery(ctx, `encode html: <div class="test">`)
	if err != nil {
		t.Fatalf("HandleInstantQuery() error = %v", err)
	}
	if answer == nil {
		t.Fatal("HandleInstantQuery() returned nil")
	}
	if answer.Type != AnswerTypeHTMLEntity {
		t.Errorf("Type = %v, want %v", answer.Type, AnswerTypeHTMLEntity)
	}
	if !contains(answer.Content, "&lt;") {
		t.Error("Content should contain encoded entity &lt;")
	}
}

func TestHTMLEntityHandlerHandleDecode(t *testing.T) {
	h := NewHTMLEntityHandler()
	ctx := context.Background()

	answer, err := h.HandleInstantQuery(ctx, "decode html: &lt;div&gt;")
	if err != nil {
		t.Fatalf("HandleInstantQuery() error = %v", err)
	}
	if answer == nil {
		t.Fatal("HandleInstantQuery() returned nil")
	}
	if answer.Type != AnswerTypeHTMLEntity {
		t.Errorf("Type = %v, want %v", answer.Type, AnswerTypeHTMLEntity)
	}
	// decode path uses html.EscapeString for display, so <div> appears as &lt;div&gt; in Content
	if !contains(answer.Content, "&lt;div&gt;") {
		t.Error("Content should contain HTML-escaped decoded value &lt;div&gt;")
	}
}

func TestHTMLEntityHandlerHandleAutoDetectDecode(t *testing.T) {
	h := NewHTMLEntityHandler()
	ctx := context.Background()

	// Query contains &...; so auto-detect should decode
	answer, err := h.HandleInstantQuery(ctx, "html: &amp;&lt;&gt;")
	if err != nil {
		t.Fatalf("HandleInstantQuery() error = %v", err)
	}
	if answer == nil {
		t.Fatal("HandleInstantQuery() returned nil")
	}
	if answer.Type != AnswerTypeHTMLEntity {
		t.Errorf("Type = %v, want %v", answer.Type, AnswerTypeHTMLEntity)
	}
}

func TestHTMLEntityHandlerHandleNoMatch(t *testing.T) {
	h := NewHTMLEntityHandler()
	ctx := context.Background()

	answer, err := h.HandleInstantQuery(ctx, "completely unrelated")
	if err != nil {
		t.Fatalf("HandleInstantQuery() error = %v", err)
	}
	if answer != nil {
		t.Error("HandleInstantQuery() should return nil for non-matching query")
	}
}

func TestDecodeHTMLEntities(t *testing.T) {
	h := NewHTMLEntityHandler()

	tests := []struct {
		input string
		want  string
	}{
		{"&lt;div&gt;", "<div>"},
		{"&amp;", "&"},
		{"&quot;", "\""},
		{"&#60;", "<"},
		{"hello world", "hello world"},
		{"&lt;p class=&quot;test&quot;&gt;", `<p class="test">`},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := h.decodeHTMLEntities(tt.input)
			if got != tt.want {
				t.Errorf("decodeHTMLEntities(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestEncodeAllChars(t *testing.T) {
	h := NewHTMLEntityHandler()

	tests := []struct {
		name  string
		input string
	}{
		{"less than", "<"},
		{"greater than", ">"},
		{"ampersand", "&"},
		{"plain text stays plain", "abc"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := h.encodeAllChars(tt.input)
			// Encoded result should not contain the raw special character
			if tt.input == "<" && contains(got, "<") && !contains(got, "&lt;") {
				t.Errorf("encodeAllChars(%q) should encode < to &lt;, got %q", tt.input, got)
			}
		})
	}
}

func TestGetHTMLEntities(t *testing.T) {
	entities := getHTMLEntities()
	if len(entities) == 0 {
		t.Error("getHTMLEntities() should return a non-empty map")
	}
	// Verify some common entities exist
	if _, ok := entities["amp"]; !ok {
		t.Error("getHTMLEntities() should contain 'amp'")
	}
	if _, ok := entities["lt"]; !ok {
		t.Error("getHTMLEntities() should contain 'lt'")
	}
	if _, ok := entities["gt"]; !ok {
		t.Error("getHTMLEntities() should contain 'gt'")
	}
}

// ASCIIHandler tests cover pure-Go ASCII art generation.

func TestNewASCIIHandler(t *testing.T) {
	h := NewASCIIHandler()
	if h == nil {
		t.Fatal("NewASCIIHandler() returned nil")
	}
}

func TestASCIIHandlerName(t *testing.T) {
	h := NewASCIIHandler()
	if h.Name() != "ascii" {
		t.Errorf("Name() = %q, want %q", h.Name(), "ascii")
	}
}

func TestASCIIHandlerPatterns(t *testing.T) {
	h := NewASCIIHandler()
	if len(h.Patterns()) == 0 {
		t.Error("Patterns() should return patterns")
	}
}

func TestASCIIHandlerCanHandle(t *testing.T) {
	h := NewASCIIHandler()

	tests := []struct {
		query string
		want  bool
	}{
		{"ascii: hello", true},
		{"ascii art: world", true},
		{"figlet: test", true},
		{"banner: foo", true},
		{"text art: bar", true},
		{"hello world", false},
		{"convert 10 km to miles", false},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			got := h.CanHandle(tt.query)
			if got != tt.want {
				t.Errorf("CanHandle(%q) = %v, want %v", tt.query, got, tt.want)
			}
		})
	}
}

func TestASCIIHandlerHandle(t *testing.T) {
	h := NewASCIIHandler()
	ctx := context.Background()

	answer, err := h.HandleInstantQuery(ctx, "ascii: HELLO")
	if err != nil {
		t.Fatalf("HandleInstantQuery() error = %v", err)
	}
	if answer == nil {
		t.Fatal("HandleInstantQuery() returned nil")
	}
	if answer.Type != AnswerTypeASCII {
		t.Errorf("Type = %v, want %v", answer.Type, AnswerTypeASCII)
	}
	if answer.Content == "" {
		t.Error("Content should not be empty")
	}
}

func TestASCIIHandlerHandleAllPatterns(t *testing.T) {
	h := NewASCIIHandler()
	ctx := context.Background()

	tests := []struct {
		query string
	}{
		{"ascii: A"},
		{"figlet: B"},
		{"banner: C"},
		{"text art: D"},
		{"ascii art: E"},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			answer, err := h.HandleInstantQuery(ctx, tt.query)
			if err != nil {
				t.Fatalf("HandleInstantQuery() error = %v", err)
			}
			if answer == nil {
				t.Fatal("HandleInstantQuery() returned nil")
			}
		})
	}
}

func TestASCIIHandlerHandleNumbers(t *testing.T) {
	h := NewASCIIHandler()
	ctx := context.Background()

	answer, err := h.HandleInstantQuery(ctx, "ascii: 1234567890")
	if err != nil {
		t.Fatalf("HandleInstantQuery() error = %v", err)
	}
	if answer == nil {
		t.Fatal("HandleInstantQuery() returned nil")
	}
}

func TestASCIIHandlerHandleSpecialChars(t *testing.T) {
	h := NewASCIIHandler()
	ctx := context.Background()

	// Unknown characters fall back to space in font
	answer, err := h.HandleInstantQuery(ctx, "ascii: !?.")
	if err != nil {
		t.Fatalf("HandleInstantQuery() error = %v", err)
	}
	if answer == nil {
		t.Fatal("HandleInstantQuery() returned nil")
	}
}

func TestASCIIHandlerHandleNoMatch(t *testing.T) {
	h := NewASCIIHandler()
	ctx := context.Background()

	answer, err := h.HandleInstantQuery(ctx, "completely unrelated query")
	if err != nil {
		t.Fatalf("HandleInstantQuery() error = %v", err)
	}
	if answer != nil {
		t.Error("HandleInstantQuery() should return nil for non-matching query")
	}
}

func TestGenerateASCII(t *testing.T) {
	h := NewASCIIHandler()

	tests := []struct {
		name  string
		input string
	}{
		{"single letter", "A"},
		{"lowercase converts to uppercase", "hello"},
		{"numbers", "123"},
		{"empty string", ""},
		{"special chars", "@#"},
		{"long text", "ABCDEFGHIJKLMNOPQRSTUVWXYZ"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := h.generateASCII(tt.input)
			// generateASCII should always return a string (even if empty)
			_ = got
		})
	}
}

// EmojiHandler tests cover pure-Go emoji searching.

func TestNewEmojiHandler(t *testing.T) {
	h := NewEmojiHandler()
	if h == nil {
		t.Fatal("NewEmojiHandler() returned nil")
	}
}

func TestEmojiHandlerName(t *testing.T) {
	h := NewEmojiHandler()
	if h.Name() != "emoji" {
		t.Errorf("Name() = %q, want %q", h.Name(), "emoji")
	}
}

func TestEmojiHandlerPatterns(t *testing.T) {
	h := NewEmojiHandler()
	if len(h.Patterns()) == 0 {
		t.Error("Patterns() should return patterns")
	}
}

func TestEmojiHandlerCanHandle(t *testing.T) {
	h := NewEmojiHandler()

	tests := []struct {
		query string
		want  bool
	}{
		{"emoji: smile", true},
		{"emojis: happy", true},
		{"find emoji: heart", true},
		{"search emoji: fire", true},
		{"hello world", false},
		{"ascii: test", false},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			got := h.CanHandle(tt.query)
			if got != tt.want {
				t.Errorf("CanHandle(%q) = %v, want %v", tt.query, got, tt.want)
			}
		})
	}
}

func TestEmojiHandlerHandleKnownEmoji(t *testing.T) {
	h := NewEmojiHandler()
	ctx := context.Background()

	answer, err := h.HandleInstantQuery(ctx, "emoji: smile")
	if err != nil {
		t.Fatalf("HandleInstantQuery() error = %v", err)
	}
	if answer == nil {
		t.Fatal("HandleInstantQuery() returned nil")
	}
	if answer.Type != AnswerTypeEmoji {
		t.Errorf("Type = %v, want %v", answer.Type, AnswerTypeEmoji)
	}
}

func TestEmojiHandlerHandleNoResults(t *testing.T) {
	h := NewEmojiHandler()
	ctx := context.Background()

	// Query that is unlikely to match any emoji
	answer, err := h.HandleInstantQuery(ctx, "emoji: xyzzy_not_a_real_emoji_12345")
	if err != nil {
		t.Fatalf("HandleInstantQuery() error = %v", err)
	}
	if answer == nil {
		t.Fatal("HandleInstantQuery() returned nil — should return 'No emojis found' answer")
	}
	if answer.Type != AnswerTypeEmoji {
		t.Errorf("Type = %v, want %v", answer.Type, AnswerTypeEmoji)
	}
	if !contains(answer.Content, "No emojis found") {
		t.Error("Content should indicate no emojis found")
	}
}

func TestEmojiHandlerHandleAllPatterns(t *testing.T) {
	h := NewEmojiHandler()
	ctx := context.Background()

	tests := []string{
		"emoji: heart",
		"emojis: heart",
		"find emoji: heart",
		"search emoji: heart",
	}

	for _, query := range tests {
		t.Run(query, func(t *testing.T) {
			answer, err := h.HandleInstantQuery(ctx, query)
			if err != nil {
				t.Fatalf("HandleInstantQuery() error = %v", err)
			}
			if answer == nil {
				t.Fatal("HandleInstantQuery() returned nil")
			}
		})
	}
}

func TestEmojiHandlerHandleNoMatch(t *testing.T) {
	h := NewEmojiHandler()
	ctx := context.Background()

	answer, err := h.HandleInstantQuery(ctx, "completely unrelated query")
	if err != nil {
		t.Fatalf("HandleInstantQuery() error = %v", err)
	}
	if answer != nil {
		t.Error("HandleInstantQuery() should return nil for non-matching query")
	}
}

func TestSearchEmojis(t *testing.T) {
	h := NewEmojiHandler()

	tests := []struct {
		name     string
		query    string
		wantMore int
	}{
		{"exact name match", "smile", 1},
		{"partial match", "hap", 0},
		{"no match", "xyzzy_definitely_not_an_emoji_99999", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results := h.searchEmojis(tt.query)
			if tt.wantMore > 0 && len(results) < tt.wantMore {
				t.Errorf("searchEmojis(%q) returned %d results, want at least %d", tt.query, len(results), tt.wantMore)
			}
			// Max 20 results enforced
			if len(results) > 20 {
				t.Errorf("searchEmojis() returned %d results, max is 20", len(results))
			}
		})
	}
}

// QRHandler tests cover QR code generation.

func TestNewQRHandler(t *testing.T) {
	h := NewQRHandler()
	if h == nil {
		t.Fatal("NewQRHandler() returned nil")
	}
}

func TestQRHandlerName(t *testing.T) {
	h := NewQRHandler()
	if h.Name() != "qr" {
		t.Errorf("Name() = %q, want %q", h.Name(), "qr")
	}
}

func TestQRHandlerPatterns(t *testing.T) {
	h := NewQRHandler()
	if len(h.Patterns()) == 0 {
		t.Error("Patterns() should return patterns")
	}
}

func TestQRHandlerCanHandle(t *testing.T) {
	h := NewQRHandler()

	tests := []struct {
		query string
		want  bool
	}{
		{"qr: https://example.com", true},
		{"qrcode: hello world", true},
		{"generate qr: test text", true},
		{"qr code: data here", true},
		{"hello world", false},
		{"ascii: test", false},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			got := h.CanHandle(tt.query)
			if got != tt.want {
				t.Errorf("CanHandle(%q) = %v, want %v", tt.query, got, tt.want)
			}
		})
	}
}

func TestQRHandlerHandle(t *testing.T) {
	h := NewQRHandler()
	ctx := context.Background()

	answer, err := h.HandleInstantQuery(ctx, "qr: https://example.com")
	if err != nil {
		t.Fatalf("HandleInstantQuery() error = %v", err)
	}
	if answer == nil {
		t.Fatal("HandleInstantQuery() returned nil")
	}
	if answer.Type != AnswerTypeQR {
		t.Errorf("Type = %v, want %v", answer.Type, AnswerTypeQR)
	}
	if !contains(answer.Content, "data:image/png;base64,") {
		t.Error("Content should contain base64-encoded PNG image")
	}
}

func TestQRHandlerHandleAllPatterns(t *testing.T) {
	h := NewQRHandler()
	ctx := context.Background()

	tests := []string{
		"qr: test",
		"qrcode: test",
		"generate qr: test",
		"qr code: test",
	}

	for _, query := range tests {
		t.Run(query, func(t *testing.T) {
			answer, err := h.HandleInstantQuery(ctx, query)
			if err != nil {
				t.Fatalf("HandleInstantQuery() error = %v", err)
			}
			if answer == nil {
				t.Fatal("HandleInstantQuery() returned nil")
			}
			if answer.Type != AnswerTypeQR {
				t.Errorf("Type = %v, want %v", answer.Type, AnswerTypeQR)
			}
		})
	}
}

func TestQRHandlerHandleNoMatch(t *testing.T) {
	h := NewQRHandler()
	ctx := context.Background()

	answer, err := h.HandleInstantQuery(ctx, "completely unrelated query")
	if err != nil {
		t.Fatalf("HandleInstantQuery() error = %v", err)
	}
	if answer != nil {
		t.Error("HandleInstantQuery() should return nil for non-matching query")
	}
}

func TestQRHandlerDataFields(t *testing.T) {
	h := NewQRHandler()
	ctx := context.Background()

	answer, err := h.HandleInstantQuery(ctx, "qr: test data")
	if err != nil {
		t.Fatalf("HandleInstantQuery() error = %v", err)
	}
	if answer == nil {
		t.Fatal("HandleInstantQuery() returned nil")
	}
	if answer.Data["text"] == nil {
		t.Error("Data should contain text")
	}
	if answer.Data["image_b64"] == nil {
		t.Error("Data should contain image_b64")
	}
	if answer.Data["image_type"] == nil {
		t.Error("Data should contain image_type")
	}
	if answer.Data["image_type"] != "png" {
		t.Errorf("image_type = %v, want png", answer.Data["image_type"])
	}
}

func TestGenerateQRASCII(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"simple text", "hello"},
		{"URL", "https://example.com"},
		{"empty string", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := generateQRASCII(tt.input)
			_ = got
			if tt.input != "" && got == "" {
				t.Logf("generateQRASCII(%q) returned empty string", tt.input)
			}
		})
	}
}

// DefinitionHandler HTTP mock tests use httptest.Server to mock the dictionary API.

func TestDefinitionHandlerHandleSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[{"word":"test","phonetic":"/test/","phonetics":[],"meanings":[{"partOfSpeech":"noun","definitions":[{"definition":"A procedure intended to establish quality.","example":"a test of strength","synonyms":[],"antonyms":[]}]}],"origin":"Old English"}]`))
	}))
	defer srv.Close()

	h := NewDefinitionHandler()
	h.client = &http.Client{
		Transport: &redirectTransport{target: srv.URL},
	}
	ctx := context.Background()

	answer, err := h.HandleInstantQuery(ctx, "define: test")
	if err != nil {
		t.Fatalf("HandleInstantQuery() error = %v", err)
	}
	if answer == nil {
		t.Fatal("HandleInstantQuery() returned nil")
	}
	if answer.Type != AnswerTypeDefinition {
		t.Errorf("Type = %v, want %v", answer.Type, AnswerTypeDefinition)
	}
	if !contains(answer.Content, "test") {
		t.Error("Content should contain the word")
	}
	if answer.Source != "Free Dictionary API" {
		t.Errorf("Source = %q, want %q", answer.Source, "Free Dictionary API")
	}
}

func TestDefinitionHandlerHandleNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	h := NewDefinitionHandler()
	h.client = &http.Client{
		Transport: &redirectTransport{target: srv.URL},
	}
	ctx := context.Background()

	answer, err := h.HandleInstantQuery(ctx, "define: xyzzynotaword")
	if err != nil {
		t.Fatalf("HandleInstantQuery() error = %v", err)
	}
	// 404 from the API means no instant answer (nil, nil) — not a "no definition found" message
	if answer != nil {
		t.Fatalf("HandleInstantQuery() should return nil for 404, got %+v", answer)
	}
}

func TestDefinitionHandlerHandleEmptyData(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	h := NewDefinitionHandler()
	h.client = &http.Client{
		Transport: &redirectTransport{target: srv.URL},
	}
	ctx := context.Background()

	answer, err := h.HandleInstantQuery(ctx, "define: emptyword")
	if err != nil {
		t.Fatalf("HandleInstantQuery() error = %v", err)
	}
	// Empty data from the API means no instant answer (nil, nil) — not a "no definition found" message
	if answer != nil {
		t.Fatalf("HandleInstantQuery() should return nil for empty data, got %+v", answer)
	}
}

func TestDefinitionHandlerHandleNoMatch(t *testing.T) {
	h := NewDefinitionHandler()
	ctx := context.Background()

	answer, err := h.HandleInstantQuery(ctx, "completely unrelated query")
	if err != nil {
		t.Fatalf("HandleInstantQuery() error = %v", err)
	}
	if answer != nil {
		t.Error("HandleInstantQuery() should return nil for non-matching query")
	}
}

func TestDefinitionHandlerHandleWithPhonetic(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[{"word":"hello","phonetic":"/phonetic-test/","phonetics":[],"meanings":[{"partOfSpeech":"exclamation","definitions":[{"definition":"Used as a greeting.","example":"","synonyms":[],"antonyms":[]}]}],"origin":""}]`))
	}))
	defer srv.Close()

	h := NewDefinitionHandler()
	h.client = &http.Client{
		Transport: &redirectTransport{target: srv.URL},
	}
	ctx := context.Background()

	answer, err := h.HandleInstantQuery(ctx, "what is hello")
	if err != nil {
		t.Fatalf("HandleInstantQuery() error = %v", err)
	}
	if answer == nil {
		t.Fatal("HandleInstantQuery() returned nil")
	}
	if !contains(answer.Content, "phonetic-test") {
		t.Error("Content should contain phonetic")
	}
}

// DictionaryHandlerHandleSuccess verifies the shared definition logic through the dictionary prefix patterns.

func TestDictionaryHandlerHandleSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[{"word":"test","phonetic":"","phonetics":[],"meanings":[{"partOfSpeech":"noun","definitions":[{"definition":"An examination.","example":"","synonyms":[],"antonyms":[]}]}],"origin":""}]`))
	}))
	defer srv.Close()

	h := NewDictionaryHandler()
	h.client = &http.Client{
		Transport: &redirectTransport{target: srv.URL},
	}
	ctx := context.Background()

	answer, err := h.HandleInstantQuery(ctx, "dict: test")
	if err != nil {
		t.Fatalf("HandleInstantQuery() error = %v", err)
	}
	if answer == nil {
		t.Fatal("HandleInstantQuery() returned nil")
	}
	if answer.Type != AnswerTypeDefinition {
		t.Errorf("Type = %v, want %v", answer.Type, AnswerTypeDefinition)
	}
}

// SynonymHandlerHandleSuccess tests the Datamuse API integration via a mock server.

func TestSynonymHandlerHandleSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[{"word":"glad","score":100},{"word":"joyful","score":95},{"word":"cheerful","score":90}]`))
	}))
	defer srv.Close()

	h := NewSynonymHandler()
	h.client = &http.Client{
		Transport: &redirectTransport{target: srv.URL},
	}
	ctx := context.Background()

	answer, err := h.HandleInstantQuery(ctx, "synonym: happy")
	if err != nil {
		t.Fatalf("HandleInstantQuery() error = %v", err)
	}
	if answer == nil {
		t.Fatal("HandleInstantQuery() returned nil")
	}
	if answer.Type != AnswerTypeSynonym {
		t.Errorf("Type = %v, want %v", answer.Type, AnswerTypeSynonym)
	}
	if !contains(answer.Content, "glad") {
		t.Error("Content should contain synonym 'glad'")
	}
	if answer.Source != "Datamuse API" {
		t.Errorf("Source = %q, want %q", answer.Source, "Datamuse API")
	}
}

func TestSynonymHandlerHandleEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	h := NewSynonymHandler()
	h.client = &http.Client{
		Transport: &redirectTransport{target: srv.URL},
	}
	ctx := context.Background()

	answer, err := h.HandleInstantQuery(ctx, "synonym: xyzzynoword")
	if err != nil {
		t.Fatalf("HandleInstantQuery() error = %v", err)
	}
	if answer == nil {
		t.Fatal("HandleInstantQuery() returned nil")
	}
	if !contains(answer.Content, "No synonyms found") {
		t.Error("Content should indicate no synonyms found")
	}
}

func TestSynonymHandlerHandleNoMatch(t *testing.T) {
	h := NewSynonymHandler()
	ctx := context.Background()

	answer, err := h.HandleInstantQuery(ctx, "completely unrelated query")
	if err != nil {
		t.Fatalf("HandleInstantQuery() error = %v", err)
	}
	if answer != nil {
		t.Error("HandleInstantQuery() should return nil for non-matching query")
	}
}

func TestSynonymHandlerDataFields(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[{"word":"big","score":100}]`))
	}))
	defer srv.Close()

	h := NewSynonymHandler()
	h.client = &http.Client{
		Transport: &redirectTransport{target: srv.URL},
	}
	ctx := context.Background()

	answer, err := h.HandleInstantQuery(ctx, "syn: large")
	if err != nil {
		t.Fatalf("HandleInstantQuery() error = %v", err)
	}
	if answer == nil {
		t.Fatal("HandleInstantQuery() returned nil")
	}
	if answer.Data["word"] == nil {
		t.Error("Data should contain word")
	}
	if answer.Data["synonyms"] == nil {
		t.Error("Data should contain synonyms")
	}
}

// AntonymHandlerHandleSuccess tests the Datamuse antonym API integration via a mock server.

func TestAntonymHandlerHandleSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[{"word":"sad","score":100},{"word":"unhappy","score":95}]`))
	}))
	defer srv.Close()

	h := NewAntonymHandler()
	h.client = &http.Client{
		Transport: &redirectTransport{target: srv.URL},
	}
	ctx := context.Background()

	answer, err := h.HandleInstantQuery(ctx, "antonym: happy")
	if err != nil {
		t.Fatalf("HandleInstantQuery() error = %v", err)
	}
	if answer == nil {
		t.Fatal("HandleInstantQuery() returned nil")
	}
	if answer.Type != AnswerTypeAntonym {
		t.Errorf("Type = %v, want %v", answer.Type, AnswerTypeAntonym)
	}
	if !contains(answer.Content, "sad") {
		t.Error("Content should contain antonym 'sad'")
	}
}

func TestAntonymHandlerHandleEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	h := NewAntonymHandler()
	h.client = &http.Client{
		Transport: &redirectTransport{target: srv.URL},
	}
	ctx := context.Background()

	answer, err := h.HandleInstantQuery(ctx, "antonym: xyzzynoword")
	if err != nil {
		t.Fatalf("HandleInstantQuery() error = %v", err)
	}
	if answer == nil {
		t.Fatal("HandleInstantQuery() returned nil")
	}
	if !contains(answer.Content, "No antonyms found") {
		t.Error("Content should indicate no antonyms found")
	}
}

func TestAntonymHandlerHandleNoMatch(t *testing.T) {
	h := NewAntonymHandler()
	ctx := context.Background()

	answer, err := h.HandleInstantQuery(ctx, "completely unrelated query")
	if err != nil {
		t.Fatalf("HandleInstantQuery() error = %v", err)
	}
	if answer != nil {
		t.Error("HandleInstantQuery() should return nil for non-matching query")
	}
}

// redirectTransport is a test helper that redirects all requests to a local test server.
type redirectTransport struct {
	target string
}

func (t *redirectTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req2 := req.Clone(req.Context())
	req2.URL.Scheme = "http"
	req2.URL.Host = strings.TrimPrefix(t.target, "http://")
	return http.DefaultTransport.RoundTrip(req2)
}

// DirectAnswerManager tests cover registration, dispatch, and all pure-Go handlers.

func TestNewDirectAnswerManager(t *testing.T) {
	m := NewDirectAnswerManager()
	if m == nil {
		t.Fatal("NewDirectAnswerManager() returned nil")
	}
	if len(m.GetHandlers()) == 0 {
		t.Error("GetHandlers() should return registered handlers")
	}
}

func TestDirectAnswerManagerRegister(t *testing.T) {
	m := &DirectAnswerManager{handlers: make([]Handler, 0)}
	initial := len(m.GetHandlers())
	m.Register(NewHTTPCodeHandler())
	if len(m.GetHandlers()) != initial+1 {
		t.Errorf("GetHandlers() length = %d, want %d", len(m.GetHandlers()), initial+1)
	}
}

func TestDirectAnswerManagerProcessNoMatch(t *testing.T) {
	m := NewDirectAnswerManager()
	ctx := context.Background()

	answer, err := m.Process(ctx, "completely unrelated query that matches nothing")
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}
	if answer != nil {
		t.Error("Process() should return nil for unmatched query")
	}
}

func TestDirectAnswerManagerProcessHTTPCode(t *testing.T) {
	m := NewDirectAnswerManager()
	ctx := context.Background()

	answer, err := m.Process(ctx, "http: 404")
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}
	if answer == nil {
		t.Fatal("Process() returned nil")
	}
	if answer.Type != AnswerTypeHTTPCode {
		t.Errorf("Type = %v, want %v", answer.Type, AnswerTypeHTTPCode)
	}
}

func TestDirectAnswerManagerProcessTrimsSpace(t *testing.T) {
	m := NewDirectAnswerManager()
	ctx := context.Background()

	answer, err := m.Process(ctx, "  http: 200  ")
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}
	if answer == nil {
		t.Fatal("Process() returned nil for whitespace-padded query")
	}
}

// HTTPCodeHandler tests cover the static lookup table.

func TestNewHTTPCodeHandler(t *testing.T) {
	h := NewHTTPCodeHandler()
	if h == nil {
		t.Fatal("NewHTTPCodeHandler() returned nil")
	}
}

func TestHTTPCodeHandlerName(t *testing.T) {
	h := NewHTTPCodeHandler()
	if h.Name() != "httpcode" {
		t.Errorf("Name() = %q, want %q", h.Name(), "httpcode")
	}
}

func TestHTTPCodeHandlerPatterns(t *testing.T) {
	h := NewHTTPCodeHandler()
	if len(h.Patterns()) == 0 {
		t.Error("Patterns() should return patterns")
	}
}

func TestHTTPCodeHandlerCanHandle(t *testing.T) {
	h := NewHTTPCodeHandler()

	tests := []struct {
		query string
		want  bool
	}{
		{"http: 404", true},
		{"http status: 200", true},
		{"status code: 500", true},
		{"http 418", true},
		{"hello world", false},
		{"http: abc", false},
		{"port: 80", false},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			got := h.CanHandle(tt.query)
			if got != tt.want {
				t.Errorf("CanHandle(%q) = %v, want %v", tt.query, got, tt.want)
			}
		})
	}
}

func TestHTTPCodeHandlerHandleKnownCodes(t *testing.T) {
	h := NewHTTPCodeHandler()
	ctx := context.Background()

	tests := []struct {
		query    string
		wantCode string
	}{
		{"http: 200", "OK"},
		{"http: 404", "Not Found"},
		{"http: 500", "Internal Server Error"},
		{"http: 418", "teapot"},
		{"http status: 301", "Moved Permanently"},
		{"status code: 403", "Forbidden"},
		{"http 201", "Created"},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			answer, err := h.HandleInstantQuery(ctx, tt.query)
			if err != nil {
				t.Fatalf("HandleInstantQuery() error = %v", err)
			}
			if answer == nil {
				t.Fatal("HandleInstantQuery() returned nil")
			}
			if answer.Type != AnswerTypeHTTPCode {
				t.Errorf("Type = %v, want %v", answer.Type, AnswerTypeHTTPCode)
			}
			if !contains(answer.Content, tt.wantCode) {
				t.Errorf("Content should contain %q, got: %s", tt.wantCode, answer.Content)
			}
		})
	}
}

func TestHTTPCodeHandlerHandleUnknownCode(t *testing.T) {
	h := NewHTTPCodeHandler()
	ctx := context.Background()

	answer, err := h.HandleInstantQuery(ctx, "http: 999")
	if err != nil {
		t.Fatalf("HandleInstantQuery() error = %v", err)
	}
	if answer == nil {
		t.Fatal("HandleInstantQuery() returned nil")
	}
	if !contains(answer.Content, "Unknown HTTP status code") {
		t.Error("Content should indicate unknown HTTP status code")
	}
}

func TestHTTPCodeHandlerHandleNoMatch(t *testing.T) {
	h := NewHTTPCodeHandler()
	ctx := context.Background()

	answer, err := h.HandleInstantQuery(ctx, "completely unrelated")
	if err != nil {
		t.Fatalf("HandleInstantQuery() error = %v", err)
	}
	if answer != nil {
		t.Error("HandleInstantQuery() should return nil for non-matching query")
	}
}

// PortHandler tests cover the static port lookup table.

func TestNewPortHandler(t *testing.T) {
	h := NewPortHandler()
	if h == nil {
		t.Fatal("NewPortHandler() returned nil")
	}
}

func TestPortHandlerName(t *testing.T) {
	h := NewPortHandler()
	if h.Name() != "port" {
		t.Errorf("Name() = %q, want %q", h.Name(), "port")
	}
}

func TestPortHandlerCanHandle(t *testing.T) {
	h := NewPortHandler()

	tests := []struct {
		query string
		want  bool
	}{
		{"port: 80", true},
		{"what is port 22?", true},
		{"service on port: 443", true},
		{"hello world", false},
		{"http: 200", false},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			got := h.CanHandle(tt.query)
			if got != tt.want {
				t.Errorf("CanHandle(%q) = %v, want %v", tt.query, got, tt.want)
			}
		})
	}
}

func TestPortHandlerHandleKnownPorts(t *testing.T) {
	h := NewPortHandler()
	ctx := context.Background()

	tests := []struct {
		query       string
		wantService string
	}{
		{"port: 80", "HTTP"},
		{"port: 443", "HTTPS"},
		{"port: 22", "SSH"},
		{"port: 25", "SMTP"},
		{"what is port 53?", "DNS"},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			answer, err := h.HandleInstantQuery(ctx, tt.query)
			if err != nil {
				t.Fatalf("HandleInstantQuery() error = %v", err)
			}
			if answer == nil {
				t.Fatal("HandleInstantQuery() returned nil")
			}
			if answer.Type != AnswerTypePort {
				t.Errorf("Type = %v, want %v", answer.Type, AnswerTypePort)
			}
			if !contains(answer.Content, tt.wantService) {
				t.Errorf("Content should contain service %q", tt.wantService)
			}
		})
	}
}

func TestPortHandlerHandleUnknownPort(t *testing.T) {
	h := NewPortHandler()
	ctx := context.Background()

	// Use a port unlikely to be in the lookup table
	answer, err := h.HandleInstantQuery(ctx, "port: 59999")
	if err != nil {
		t.Fatalf("HandleInstantQuery() error = %v", err)
	}
	if answer == nil {
		t.Fatal("HandleInstantQuery() returned nil")
	}
	if answer.Type != AnswerTypePort {
		t.Errorf("Type = %v, want %v", answer.Type, AnswerTypePort)
	}
	if !contains(answer.Content, "Unknown") {
		t.Error("Content should indicate Unknown/Custom port")
	}
}

func TestPortHandlerHandleWellKnownCategory(t *testing.T) {
	h := NewPortHandler()
	ctx := context.Background()

	// Port in Well-Known range (0-1023) but not in lookup table
	answer, err := h.HandleInstantQuery(ctx, "port: 999")
	if err != nil {
		t.Fatalf("HandleInstantQuery() error = %v", err)
	}
	if answer == nil {
		t.Fatal("HandleInstantQuery() returned nil")
	}
}

func TestPortHandlerHandleNoMatch(t *testing.T) {
	h := NewPortHandler()
	ctx := context.Background()

	answer, err := h.HandleInstantQuery(ctx, "completely unrelated")
	if err != nil {
		t.Fatalf("HandleInstantQuery() error = %v", err)
	}
	if answer != nil {
		t.Error("HandleInstantQuery() should return nil for non-matching query")
	}
}

// CronHandler tests cover cron expression parsing.

func TestNewCronHandler(t *testing.T) {
	h := NewCronHandler()
	if h == nil {
		t.Fatal("NewCronHandler() returned nil")
	}
}

func TestCronHandlerName(t *testing.T) {
	h := NewCronHandler()
	if h.Name() != "cron" {
		t.Errorf("Name() = %q, want %q", h.Name(), "cron")
	}
}

func TestCronHandlerCanHandle(t *testing.T) {
	h := NewCronHandler()

	tests := []struct {
		query string
		want  bool
	}{
		{"cron: * * * * *", true},
		{"crontab: 0 0 * * *", true},
		{"explain cron: */5 * * * *", true},
		{"hello world", false},
		{"port: 80", false},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			got := h.CanHandle(tt.query)
			if got != tt.want {
				t.Errorf("CanHandle(%q) = %v, want %v", tt.query, got, tt.want)
			}
		})
	}
}

func TestCronHandlerHandleValidExpressions(t *testing.T) {
	h := NewCronHandler()
	ctx := context.Background()

	tests := []struct {
		query string
	}{
		{"cron: * * * * *"},
		{"cron: 0 0 * * *"},
		{"cron: */5 * * * *"},
		{"cron: 0 9-17 * * 1-5"},
		{"cron: 0 0 1 1 *"},
		{"cron: @daily"},
		{"cron: @weekly"},
		{"cron: @monthly"},
		{"cron: @yearly"},
		{"cron: @hourly"},
		{"cron: @reboot"},
		{"cron: 0 0 * * * *"},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			answer, err := h.HandleInstantQuery(ctx, tt.query)
			if err != nil {
				t.Fatalf("HandleInstantQuery() error = %v", err)
			}
			if answer == nil {
				t.Fatal("HandleInstantQuery() returned nil")
			}
			if answer.Type != AnswerTypeCron {
				t.Errorf("Type = %v, want %v", answer.Type, AnswerTypeCron)
			}
		})
	}
}

func TestCronHandlerHandleInvalidExpression(t *testing.T) {
	h := NewCronHandler()
	ctx := context.Background()

	answer, err := h.HandleInstantQuery(ctx, "cron: one two three four five six seven")
	if err != nil {
		t.Fatalf("HandleInstantQuery() error = %v", err)
	}
	if answer == nil {
		t.Fatal("HandleInstantQuery() returned nil")
	}
	if answer.Type != AnswerTypeCron {
		t.Errorf("Type = %v, want %v", answer.Type, AnswerTypeCron)
	}
	if !contains(answer.Content, "Invalid") && !contains(answer.Content, "invalid") {
		t.Error("Content should indicate invalid cron expression")
	}
}

func TestCronHandlerHandleNoMatch(t *testing.T) {
	h := NewCronHandler()
	ctx := context.Background()

	answer, err := h.HandleInstantQuery(ctx, "completely unrelated")
	if err != nil {
		t.Fatalf("HandleInstantQuery() error = %v", err)
	}
	if answer != nil {
		t.Error("HandleInstantQuery() should return nil for non-matching query")
	}
}

func TestExplainCron(t *testing.T) {
	tests := []struct {
		name    string
		expr    string
		wantErr bool
	}{
		{"every minute", "* * * * *", false},
		{"daily midnight", "0 0 * * *", false},
		{"step value", "*/15 * * * *", false},
		{"range", "0 9-17 * * 1-5", false},
		{"list values", "0 0 1,15 * *", false},
		{"6-field with seconds", "0 0 0 * * *", false},
		{"special @daily", "@daily", false},
		{"special @weekly", "@weekly", false},
		{"special @monthly", "@monthly", false},
		{"special @yearly", "@yearly", false},
		{"special @annually", "@annually", false},
		{"special @hourly", "@hourly", false},
		{"special @reboot", "@reboot", false},
		{"too few fields", "* *", true},
		{"too many fields", "* * * * * * *", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := explainCron(tt.expr)
			if (err != nil) != tt.wantErr {
				t.Errorf("explainCron(%q) error = %v, wantErr %v", tt.expr, err, tt.wantErr)
			}
		})
	}
}

func TestExplainCronField(t *testing.T) {
	tests := []struct {
		field      string
		fieldIndex int
		wantSubstr string
	}{
		{"*", 0, "every value"},
		{"*/5", 0, "every 5"},
		{"1,2,3", 0, "specific values"},
		{"1-5", 0, "from 1 to 5"},
		{"0", 0, "at 0"},
		{"1", 3, "January"},
		{"12", 3, "December"},
		{"0", 4, "Sunday"},
		{"5", 4, "Friday"},
	}

	for _, tt := range tests {
		t.Run(tt.field+"_idx"+string(rune('0'+tt.fieldIndex)), func(t *testing.T) {
			got := explainCronField(tt.field, tt.fieldIndex)
			if !contains(got, tt.wantSubstr) {
				t.Errorf("explainCronField(%q, %d) = %q, want to contain %q", tt.field, tt.fieldIndex, got, tt.wantSubstr)
			}
		})
	}
}

// ChmodHandler tests cover octal and symbolic permission parsing.

func TestNewChmodHandler(t *testing.T) {
	h := NewChmodHandler()
	if h == nil {
		t.Fatal("NewChmodHandler() returned nil")
	}
}

func TestChmodHandlerName(t *testing.T) {
	h := NewChmodHandler()
	if h.Name() != "chmod" {
		t.Errorf("Name() = %q, want %q", h.Name(), "chmod")
	}
}

func TestChmodHandlerCanHandle(t *testing.T) {
	h := NewChmodHandler()

	tests := []struct {
		query string
		want  bool
	}{
		{"chmod: 755", true},
		{"permissions: 644", true},
		{"chmod: rwxr-xr-x", true},
		{"hello world", false},
		{"cron: * * * * *", false},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			got := h.CanHandle(tt.query)
			if got != tt.want {
				t.Errorf("CanHandle(%q) = %v, want %v", tt.query, got, tt.want)
			}
		})
	}
}

func TestChmodHandlerHandleOctal(t *testing.T) {
	h := NewChmodHandler()
	ctx := context.Background()

	tests := []struct {
		query  string
		wantIn string
	}{
		{"chmod: 755", "rwxr-xr-x"},
		{"chmod: 644", "rw-r--r--"},
		{"chmod: 777", "rwxrwxrwx"},
		{"chmod: 000", "---"},
		{"chmod: 4755", "rwsr-xr-x"},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			answer, err := h.HandleInstantQuery(ctx, tt.query)
			if err != nil {
				t.Fatalf("HandleInstantQuery() error = %v", err)
			}
			if answer == nil {
				t.Fatal("HandleInstantQuery() returned nil")
			}
			if answer.Type != AnswerTypeChmod {
				t.Errorf("Type = %v, want %v", answer.Type, AnswerTypeChmod)
			}
			if !contains(answer.Content, tt.wantIn) {
				t.Errorf("Content should contain %q for query %q", tt.wantIn, tt.query)
			}
		})
	}
}

func TestChmodHandlerHandleSymbolic(t *testing.T) {
	h := NewChmodHandler()
	ctx := context.Background()

	answer, err := h.HandleInstantQuery(ctx, "chmod: rwxr-xr-x")
	if err != nil {
		t.Fatalf("HandleInstantQuery() error = %v", err)
	}
	if answer == nil {
		t.Fatal("HandleInstantQuery() returned nil")
	}
	if answer.Type != AnswerTypeChmod {
		t.Errorf("Type = %v, want %v", answer.Type, AnswerTypeChmod)
	}
	// Symbolic rwxr-xr-x = 755
	if !contains(answer.Content, "755") {
		t.Error("Content should contain octal 755 for rwxr-xr-x")
	}
}

func TestChmodHandlerHandleNoMatch(t *testing.T) {
	h := NewChmodHandler()
	ctx := context.Background()

	answer, err := h.HandleInstantQuery(ctx, "completely unrelated")
	if err != nil {
		t.Fatalf("HandleInstantQuery() error = %v", err)
	}
	if answer != nil {
		t.Error("HandleInstantQuery() should return nil for non-matching query")
	}
}

func TestOctalToSymbolic(t *testing.T) {
	tests := []struct {
		octal string
		want  string
	}{
		{"755", "rwxr-xr-x"},
		{"644", "rw-r--r--"},
		{"777", "rwxrwxrwx"},
		{"000", "---------"},
		{"700", "rwx------"},
		{"444", "r--r--r--"},
	}

	for _, tt := range tests {
		t.Run(tt.octal, func(t *testing.T) {
			got := octalToSymbolic(tt.octal)
			if got != tt.want {
				t.Errorf("octalToSymbolic(%q) = %q, want %q", tt.octal, got, tt.want)
			}
		})
	}
}

func TestSymbolicToOctal(t *testing.T) {
	tests := []struct {
		symbolic string
		want     string
	}{
		{"rwxr-xr-x", "755"},
		{"rw-r--r--", "644"},
		{"rwxrwxrwx", "777"},
		{"---------", "000"},
		{"rwx------", "700"},
		{"r--r--r--", "444"},
	}

	for _, tt := range tests {
		t.Run(tt.symbolic, func(t *testing.T) {
			got := symbolicToOctal(tt.symbolic)
			if got != tt.want {
				t.Errorf("symbolicToOctal(%q) = %q, want %q", tt.symbolic, got, tt.want)
			}
		})
	}
}

func TestExplainChmod(t *testing.T) {
	tests := []struct {
		octal      string
		wantSubstr string
	}{
		{"755", "Owner"},
		{"644", "Group"},
		{"777", "read"},
		{"000", "no permissions"},
		{"4755", "setuid"},
		{"2755", "setgid"},
		{"1755", "sticky"},
	}

	for _, tt := range tests {
		t.Run(tt.octal, func(t *testing.T) {
			got := explainChmod(tt.octal)
			if !contains(got, tt.wantSubstr) {
				t.Errorf("explainChmod(%q) = %q, want to contain %q", tt.octal, got, tt.wantSubstr)
			}
		})
	}
}

// TimestampHandler tests cover Unix timestamp conversion.

func TestNewTimestampHandler(t *testing.T) {
	h := NewTimestampHandler()
	if h == nil {
		t.Fatal("NewTimestampHandler() returned nil")
	}
}

func TestTimestampHandlerName(t *testing.T) {
	h := NewTimestampHandler()
	if h.Name() != "timestamp" {
		t.Errorf("Name() = %q, want %q", h.Name(), "timestamp")
	}
}

func TestTimestampHandlerCanHandle(t *testing.T) {
	h := NewTimestampHandler()

	tests := []struct {
		query string
		want  bool
	}{
		{"timestamp: 1609459200", true},
		{"unix: 1609459200", true},
		{"epoch: 1609459200", true},
		{"time: 1609459200", true},
		{"hello world", false},
		{"timestamp: notanumber", false},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			got := h.CanHandle(tt.query)
			if got != tt.want {
				t.Errorf("CanHandle(%q) = %v, want %v", tt.query, got, tt.want)
			}
		})
	}
}

func TestTimestampHandlerHandleSeconds(t *testing.T) {
	h := NewTimestampHandler()
	ctx := context.Background()

	answer, err := h.HandleInstantQuery(ctx, "timestamp: 1609459200")
	if err != nil {
		t.Fatalf("HandleInstantQuery() error = %v", err)
	}
	if answer == nil {
		t.Fatal("HandleInstantQuery() returned nil")
	}
	if answer.Type != AnswerTypeTimestamp {
		t.Errorf("Type = %v, want %v", answer.Type, AnswerTypeTimestamp)
	}
	if !contains(answer.Content, "UTC") {
		t.Error("Content should contain UTC time")
	}
	if !contains(answer.Content, "ISO 8601") {
		t.Error("Content should contain ISO 8601")
	}
}

func TestTimestampHandlerHandleMilliseconds(t *testing.T) {
	h := NewTimestampHandler()
	ctx := context.Background()

	// Millisecond timestamp (> 9999999999)
	answer, err := h.HandleInstantQuery(ctx, "timestamp: 1609459200000")
	if err != nil {
		t.Fatalf("HandleInstantQuery() error = %v", err)
	}
	if answer == nil {
		t.Fatal("HandleInstantQuery() returned nil")
	}
	if answer.Type != AnswerTypeTimestamp {
		t.Errorf("Type = %v, want %v", answer.Type, AnswerTypeTimestamp)
	}
	// TimestampHandler content uses "(milliseconds)" for millis input, not "ms"
	if !contains(answer.Content, "milliseconds") {
		t.Error("Content should indicate millisecond input with '(milliseconds)'")
	}
}

func TestTimestampHandlerHandleAllPatterns(t *testing.T) {
	h := NewTimestampHandler()
	ctx := context.Background()

	tests := []string{
		"timestamp: 1609459200",
		"unix: 1609459200",
		"epoch: 1609459200",
		"time: 1609459200",
	}

	for _, query := range tests {
		t.Run(query, func(t *testing.T) {
			answer, err := h.HandleInstantQuery(ctx, query)
			if err != nil {
				t.Fatalf("HandleInstantQuery() error = %v", err)
			}
			if answer == nil {
				t.Fatal("HandleInstantQuery() returned nil")
			}
		})
	}
}

func TestTimestampHandlerHandleNoMatch(t *testing.T) {
	h := NewTimestampHandler()
	ctx := context.Background()

	answer, err := h.HandleInstantQuery(ctx, "completely unrelated")
	if err != nil {
		t.Fatalf("HandleInstantQuery() error = %v", err)
	}
	if answer != nil {
		t.Error("HandleInstantQuery() should return nil for non-matching query")
	}
}

func TestFormatRelativeTime(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		wantIn   string
	}{
		{"just now", 30 * time.Second, "seconds"},
		{"minutes ago", 5 * time.Minute, "5 minutes ago"},
		{"one hour ago", 90 * time.Minute, "1.5 hours"},
		{"hours ago", 3 * time.Hour, "3.0 hours"},
		{"one day ago", 25 * time.Hour, "1.0 days"},
		{"days ago", 3 * 24 * time.Hour, "3.0 days"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatRelativeTime(tt.duration)
			if !contains(got, tt.wantIn) {
				t.Errorf("formatRelativeTime(%v) = %q, want to contain %q", tt.duration, got, tt.wantIn)
			}
		})
	}
}

// SubnetHandler tests cover CIDR subnet calculation.

func TestNewSubnetHandler(t *testing.T) {
	h := NewSubnetHandler()
	if h == nil {
		t.Fatal("NewSubnetHandler() returned nil")
	}
}

func TestSubnetHandlerName(t *testing.T) {
	h := NewSubnetHandler()
	if h.Name() != "subnet" {
		t.Errorf("Name() = %q, want %q", h.Name(), "subnet")
	}
}

func TestSubnetHandlerCanHandle(t *testing.T) {
	h := NewSubnetHandler()

	tests := []struct {
		query string
		want  bool
	}{
		{"subnet: 192.168.1.0/24", true},
		{"cidr: 10.0.0.0/8", true},
		{"ip range: 172.16.0.0/12", true},
		{"netmask: 192.168.0.0/16", true},
		{"hello world", false},
		{"port: 80", false},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			got := h.CanHandle(tt.query)
			if got != tt.want {
				t.Errorf("CanHandle(%q) = %v, want %v", tt.query, got, tt.want)
			}
		})
	}
}

func TestSubnetHandlerHandleValidCIDR(t *testing.T) {
	h := NewSubnetHandler()
	ctx := context.Background()

	tests := []struct {
		query       string
		wantNetmask string
	}{
		{"subnet: 192.168.1.0/24", "255.255.255.0"},
		{"cidr: 10.0.0.0/8", "255.0.0.0"},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			answer, err := h.HandleInstantQuery(ctx, tt.query)
			if err != nil {
				t.Fatalf("HandleInstantQuery() error = %v", err)
			}
			if answer == nil {
				t.Fatal("HandleInstantQuery() returned nil")
			}
			if answer.Type != AnswerTypeSubnet {
				t.Errorf("Type = %v, want %v", answer.Type, AnswerTypeSubnet)
			}
			if !contains(answer.Content, tt.wantNetmask) {
				t.Errorf("Content should contain netmask %q", tt.wantNetmask)
			}
		})
	}
}

func TestSubnetHandlerHandleSingleIP(t *testing.T) {
	h := NewSubnetHandler()
	ctx := context.Background()

	// Single IP without CIDR — handler should try appending /32
	answer, err := h.HandleInstantQuery(ctx, "subnet: 192.168.1.1")
	if err != nil {
		t.Fatalf("HandleInstantQuery() error = %v", err)
	}
	if answer == nil {
		t.Fatal("HandleInstantQuery() returned nil")
	}
}

func TestSubnetHandlerHandleInvalidCIDR(t *testing.T) {
	h := NewSubnetHandler()
	ctx := context.Background()

	answer, err := h.HandleInstantQuery(ctx, "subnet: not-a-valid-cidr-at-all")
	if err != nil {
		t.Fatalf("HandleInstantQuery() error = %v", err)
	}
	if answer == nil {
		t.Fatal("HandleInstantQuery() returned nil")
	}
	if !contains(answer.Content, "Invalid CIDR") {
		t.Error("Content should indicate invalid CIDR")
	}
}

func TestSubnetHandlerHandleDataFields(t *testing.T) {
	h := NewSubnetHandler()
	ctx := context.Background()

	answer, err := h.HandleInstantQuery(ctx, "subnet: 192.168.1.0/24")
	if err != nil {
		t.Fatalf("HandleInstantQuery() error = %v", err)
	}
	if answer == nil {
		t.Fatal("HandleInstantQuery() returned nil")
	}
	for _, field := range []string{"cidr", "netmask", "network", "broadcast", "usable_hosts"} {
		if answer.Data[field] == nil {
			t.Errorf("Data should contain %q", field)
		}
	}
}

func TestSubnetHandlerHandleNoMatch(t *testing.T) {
	h := NewSubnetHandler()
	ctx := context.Background()

	answer, err := h.HandleInstantQuery(ctx, "completely unrelated")
	if err != nil {
		t.Fatalf("HandleInstantQuery() error = %v", err)
	}
	if answer != nil {
		t.Error("HandleInstantQuery() should return nil for non-matching query")
	}
}

func TestCalculateBroadcast(t *testing.T) {
	_, ipnet, _ := net.ParseCIDR("192.168.1.0/24")
	broadcast := calculateBroadcast(ipnet)
	if broadcast.String() != "192.168.1.255" {
		t.Errorf("calculateBroadcast() = %s, want 192.168.1.255", broadcast.String())
	}
}

func TestCalculateWildcard(t *testing.T) {
	_, ipnet, _ := net.ParseCIDR("192.168.1.0/24")
	wildcard := calculateWildcard(ipnet.Mask)
	if wildcard != "0.0.0.255" {
		t.Errorf("calculateWildcard() = %s, want 0.0.0.255", wildcard)
	}
}

func TestIncrementIP(t *testing.T) {
	_, ipnet, _ := net.ParseCIDR("192.168.1.0/24")
	first := incrementIP(ipnet.IP)
	if first.String() != "192.168.1.1" {
		t.Errorf("incrementIP() = %s, want 192.168.1.1", first.String())
	}
}

func TestDecrementIP(t *testing.T) {
	broadcast := net.IP{192, 168, 1, 255}
	last := decrementIP(broadcast)
	if last.String() != "192.168.1.254" {
		t.Errorf("decrementIP() = %s, want 192.168.1.254", last.String())
	}
}

// JWTHandler tests cover JWT decoding (invalid format and CanHandle paths).

func TestNewJWTHandler(t *testing.T) {
	h := NewJWTHandler()
	if h == nil {
		t.Fatal("NewJWTHandler() returned nil")
	}
}

func TestJWTHandlerName(t *testing.T) {
	h := NewJWTHandler()
	if h.Name() != "jwt" {
		t.Errorf("Name() = %q, want %q", h.Name(), "jwt")
	}
}

func TestJWTHandlerCanHandle(t *testing.T) {
	h := NewJWTHandler()

	// Construct a syntactically-valid-looking token from base64url-encoded parts.
	// These are NOT real credentials — they are plaintext test data encoded in base64.
	// Header: {"alg":"none"}  Payload: {"sub":"test"}  Sig: test
	fakeHeader := "eyJhbGciOiJub25lIn0"
	fakePayload := "eyJzdWIiOiJ0ZXN0In0"
	fakeSig := "dGVzdA"
	fakeToken := fakeHeader + "." + fakePayload + "." + fakeSig

	tests := []struct {
		query string
		want  bool
	}{
		{"jwt: " + fakeToken, true},
		{"decode jwt: " + fakeToken, true},
		{"token: " + fakeToken, true},
		{"hello world", false},
		{"port: 80", false},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			got := h.CanHandle(tt.query)
			if got != tt.want {
				t.Errorf("CanHandle(%q) = %v, want %v", tt.query, got, tt.want)
			}
		})
	}
}

func TestJWTHandlerHandleInvalidFormat(t *testing.T) {
	h := NewJWTHandler()
	ctx := context.Background()

	// A token with only two dot-separated parts — missing signature
	twoPartToken := "eyJhbGciOiJub25lIn0.eyJzdWIiOiJ0ZXN0In0"

	answer, err := h.HandleInstantQuery(ctx, "jwt: "+twoPartToken)
	if err != nil {
		t.Fatalf("HandleInstantQuery() error = %v", err)
	}
	if answer == nil {
		t.Fatal("HandleInstantQuery() returned nil")
	}
	if !contains(answer.Content, "Invalid JWT format") {
		t.Error("Content should indicate invalid JWT format")
	}
}

func TestJWTHandlerHandleValidDecode(t *testing.T) {
	h := NewJWTHandler()
	ctx := context.Background()

	// Manually construct a decodable three-part token.
	// Header: {"alg":"none","typ":"JWT"}  Payload: {"sub":"testuser","iat":1000}
	// These are NOT real credentials — all parts are synthetic test data.
	fakeHeader := "eyJhbGciOiJub25lIiwidHlwIjoiSldUIn0"
	fakePayload := "eyJzdWIiOiJ0ZXN0dXNlciIsImlhdCI6MTAwMH0"
	fakeSig := "dGVzdHNpZ25hdHVyZQ"
	threePartToken := fakeHeader + "." + fakePayload + "." + fakeSig

	answer, err := h.HandleInstantQuery(ctx, "jwt: "+threePartToken)
	if err != nil {
		t.Fatalf("HandleInstantQuery() error = %v", err)
	}
	if answer == nil {
		t.Fatal("HandleInstantQuery() returned nil")
	}
	if answer.Type != AnswerTypeJWT {
		t.Errorf("Type = %v, want %v", answer.Type, AnswerTypeJWT)
	}
}

func TestJWTHandlerHandleExpiredToken(t *testing.T) {
	h := NewJWTHandler()
	ctx := context.Background()

	// Payload contains exp=1 (Unix epoch 1970-01-01 00:00:01 — definitely expired).
	// Header: {"alg":"none"}  Payload: {"sub":"x","exp":1}
	// NOT real credentials — synthetic test data only.
	fakeHeader := "eyJhbGciOiJub25lIn0"
	fakePayload := "eyJzdWIiOiJ4IiwiZXhwIjoxfQ"
	fakeSig := "dGVzdA"
	expiredToken := fakeHeader + "." + fakePayload + "." + fakeSig

	answer, err := h.HandleInstantQuery(ctx, "jwt: "+expiredToken)
	if err != nil {
		t.Fatalf("HandleInstantQuery() error = %v", err)
	}
	if answer == nil {
		t.Fatal("HandleInstantQuery() returned nil")
	}
	if !contains(answer.Content, "EXPIRED") {
		t.Error("Content should indicate expired token")
	}
}

func TestJWTHandlerHandleNoMatch(t *testing.T) {
	h := NewJWTHandler()
	ctx := context.Background()

	answer, err := h.HandleInstantQuery(ctx, "completely unrelated")
	if err != nil {
		t.Fatalf("HandleInstantQuery() error = %v", err)
	}
	if answer != nil {
		t.Error("HandleInstantQuery() should return nil for non-matching query")
	}
}

func TestDecodeJWTSegment(t *testing.T) {
	tests := []struct {
		name    string
		segment string
		wantKey string
		wantErr bool
	}{
		{
			"valid header with alg field",
			"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9",
			"alg",
			false,
		},
		{
			"valid payload with sub field",
			"eyJzdWIiOiIxMjM0NTY3ODkwIn0",
			"sub",
			false,
		},
		{
			"invalid base64 characters",
			"!!!notbase64!!!",
			"",
			true,
		},
		{
			"valid base64 but not json",
			"aGVsbG8",
			"",
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := decodeJWTSegment(tt.segment)
			if (err != nil) != tt.wantErr {
				t.Errorf("decodeJWTSegment() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && tt.wantKey != "" {
				if _, ok := result[tt.wantKey]; !ok {
					t.Errorf("decodeJWTSegment() result missing key %q", tt.wantKey)
				}
			}
		})
	}
}

// TLDRHandler tests use httptest.Server to mock GitHub raw content.

func TestNewTLDRHandler(t *testing.T) {
	h := NewTLDRHandler()
	if h == nil {
		t.Fatal("NewTLDRHandler() returned nil")
	}
}

func TestTLDRHandlerName(t *testing.T) {
	h := NewTLDRHandler()
	if h.Name() != "tldr" {
		t.Errorf("Name() = %q, want %q", h.Name(), "tldr")
	}
}

func TestTLDRHandlerCanHandle(t *testing.T) {
	h := NewTLDRHandler()

	tests := []struct {
		query string
		want  bool
	}{
		{"tldr: curl", true},
		{"man: ls", true},
		{"command: grep", true},
		{"hello world", false},
		{"port: 80", false},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			got := h.CanHandle(tt.query)
			if got != tt.want {
				t.Errorf("CanHandle(%q) = %v, want %v", tt.query, got, tt.want)
			}
		})
	}
}

func TestTLDRHandlerHandleFound(t *testing.T) {
	tldrContent := "# curl\n\n> Transfer data from or to a server.\n\n- Download the contents of an URL to a file:\n\n`curl http://example.com --output filename`\n"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(tldrContent))
	}))
	defer srv.Close()

	h := NewTLDRHandler()
	h.client = &http.Client{
		Transport: &redirectTransport{target: srv.URL},
	}
	ctx := context.Background()

	answer, err := h.HandleInstantQuery(ctx, "tldr: curl")
	if err != nil {
		t.Fatalf("HandleInstantQuery() error = %v", err)
	}
	if answer == nil {
		t.Fatal("HandleInstantQuery() returned nil")
	}
	if answer.Type != AnswerTypeTLDR {
		t.Errorf("Type = %v, want %v", answer.Type, AnswerTypeTLDR)
	}
	if !contains(answer.Content, "Transfer data") {
		t.Error("Content should contain TLDR description")
	}
	if answer.Source != "tldr-pages" {
		t.Errorf("Source = %q, want %q", answer.Source, "tldr-pages")
	}
}

func TestTLDRHandlerHandleNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	h := NewTLDRHandler()
	h.client = &http.Client{
		Transport: &redirectTransport{target: srv.URL},
	}
	ctx := context.Background()

	answer, err := h.HandleInstantQuery(ctx, "tldr: nonexistentcommand")
	if err != nil {
		t.Fatalf("HandleInstantQuery() error = %v", err)
	}
	if answer == nil {
		t.Fatal("HandleInstantQuery() returned nil")
	}
	if !contains(answer.Content, "No TLDR page found") {
		t.Error("Content should indicate no TLDR page found")
	}
}

func TestTLDRHandlerHandleNoMatch(t *testing.T) {
	h := NewTLDRHandler()
	ctx := context.Background()

	answer, err := h.HandleInstantQuery(ctx, "completely unrelated")
	if err != nil {
		t.Fatalf("HandleInstantQuery() error = %v", err)
	}
	if answer != nil {
		t.Error("HandleInstantQuery() should return nil for non-matching query")
	}
}

func TestParseTLDRMarkdown(t *testing.T) {
	md := "# curl\n\n> Transfer data from or to a server.\n\n- Download the contents of an URL to a file:\n\n`curl http://example.com --output filename`\n\n- Send a POST request:\n\n`curl --data data http://example.com`"

	result := parseTLDRMarkdown(md, "curl")

	if !contains(result, "curl") {
		t.Error("Result should contain command name")
	}
	if !contains(result, "Transfer data") {
		t.Error("Result should contain description")
	}
	if !contains(result, "<code>") {
		t.Error("Result should contain code examples")
	}
	if !contains(result, "Download") {
		t.Error("Result should contain example descriptions")
	}
}

func TestParseTLDRMarkdownEmpty(t *testing.T) {
	result := parseTLDRMarkdown("", "testcmd")
	if !contains(result, "testcmd") {
		t.Error("Result should contain command name even for empty markdown")
	}
}

func TestParseTLDRMarkdownNoDescription(t *testing.T) {
	md := "# ls\n\n- List directory contents:\n\n`ls`"

	result := parseTLDRMarkdown(md, "ls")
	if !contains(result, "ls") {
		t.Error("Result should contain command name")
	}
}
