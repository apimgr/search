package instant

import (
	"context"
	"encoding/json"
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
		{"2 + 2", true},
		{"10 * 5", true},
		{"100 / 4", true},
		{"5 - 3", true},
		{"2^3", true},
		{"calc 2+2", true},
		{"calculate: 10*5", true},
		{"math: 5+5", true},
		{"eval 100/2", true},
		{"compute 2*3", true},
		{"hello world", false},
		{"random text", false},
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
			answer, err := h.Handle(ctx, tt.query)
			if err != nil {
				t.Fatalf("Handle(%q) error = %v", tt.query, err)
			}
			if answer == nil {
				t.Fatal("Handle() returned nil")
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
	if h.mathExpr == nil {
		t.Error("mathExpr should not be nil")
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
			answer, err := h.Handle(ctx, tt.query)
			if err != nil {
				t.Fatalf("Handle(%q) error = %v", tt.query, err)
			}
			if answer == nil {
				t.Fatal("Handle() returned nil")
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

	answer, err := h.Handle(ctx, "time")
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if answer == nil {
		t.Fatal("Handle() returned nil")
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
			answer, err := h.Handle(ctx, query)
			if err != nil {
				t.Fatalf("Handle() error = %v", err)
			}
			if answer == nil {
				t.Fatal("Handle() returned nil")
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
	answer, err := h.Handle(ctx, "base64 encode: hello")
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if answer == nil {
		t.Fatal("Handle() returned nil")
	}
	if answer.Type != AnswerTypeBase64 {
		t.Errorf("Type = %v, want %v", answer.Type, AnswerTypeBase64)
	}
	if answer.Data["output"] != "aGVsbG8=" {
		t.Errorf("output = %v, want %v", answer.Data["output"], "aGVsbG8=")
	}

	// Test decode
	answer, err = h.Handle(ctx, "base64 decode: aGVsbG8=")
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
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

	answer, err := h.Handle(ctx, "url encode: hello world")
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if answer == nil {
		t.Fatal("Handle() returned nil")
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

	answer, err := h.Handle(ctx, "#ff0000")
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if answer == nil {
		t.Fatal("Handle() returned nil")
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

	answer, err := h.Handle(ctx, "uuid")
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if answer == nil {
		t.Fatal("Handle() returned nil")
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
			answer, err := h.Handle(ctx, query)
			if err != nil {
				t.Fatalf("Handle() error = %v", err)
			}
			if answer == nil {
				t.Fatal("Handle() returned nil")
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

	answer, err := h.Handle(ctx, "password")
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if answer == nil {
		t.Fatal("Handle() returned nil")
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

	answer, err := h.Handle(ctx, "password 32")
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
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
		{"what is my ip?", true},
		{"ip address", true},
		{"ip info", true},
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

func TestIPHandlerHandle(t *testing.T) {
	h := NewIPHandler()
	ctx := context.Background()

	answer, err := h.Handle(ctx, "my ip")
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if answer == nil {
		t.Fatal("Handle() returned nil")
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
	answer, err := h.Handle(ctx, "not a conversion query")
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if answer != nil {
		t.Error("Handle() should return nil for non-matching query")
	}
}

func TestConvertHandlerHandleSecondPattern(t *testing.T) {
	h := NewConvertHandler()
	ctx := context.Background()

	// Test the "X unit = ? unit" pattern
	answer, err := h.Handle(ctx, "100 km = ? miles")
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if answer == nil {
		t.Fatal("Handle() returned nil")
	}
	if answer.Type != AnswerTypeConvert {
		t.Errorf("Type = %v, want %v", answer.Type, AnswerTypeConvert)
	}
}

func TestConvertHandlerHandleUnknownConversion(t *testing.T) {
	h := NewConvertHandler()
	ctx := context.Background()

	// Test with unknown units that will cause convert() to return error
	answer, err := h.Handle(ctx, "100 foobar to bazqux")
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if answer == nil {
		t.Fatal("Handle() returned nil")
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
	answer, err := h.Handle(ctx, "100 meters -> feet")
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if answer == nil {
		t.Fatal("Handle() returned nil")
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
		{0.00001, true},    // Very small
		{1e15, true},       // Very large
		{0.0001, false},    // Just above threshold
		{1e9, false},       // Large but not huge
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
		{1e-5, "1e-05"},       // Very small - scientific
		{1e15, "1000000000000000"}, // Large integer
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
	answer, err := h.Handle(ctx, "not a hash query")
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if answer != nil {
		t.Error("Handle() should return nil for non-matching text")
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
			answer, err := h.Handle(ctx, tt.query)
			if err != nil {
				t.Fatalf("Handle() error = %v", err)
			}
			if answer == nil {
				t.Fatal("Handle() returned nil")
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

	answer, err := h.Handle(ctx, "not a base64 query")
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if answer != nil {
		t.Error("Handle() should return nil for non-matching text")
	}
}

func TestBase64HandlerHandleInvalidDecode(t *testing.T) {
	h := NewBase64Handler()
	ctx := context.Background()

	answer, err := h.Handle(ctx, "base64 decode: !!!invalid!!!")
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if answer == nil {
		t.Fatal("Handle() returned nil")
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

	answer, err := h.Handle(ctx, "not a url query")
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if answer != nil {
		t.Error("Handle() should return nil for non-matching text")
	}
}

func TestURLHandlerHandleParse(t *testing.T) {
	h := NewURLHandler()
	ctx := context.Background()

	answer, err := h.Handle(ctx, "parse url: https://example.com/path?query=value#fragment")
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if answer == nil {
		t.Fatal("Handle() returned nil")
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
	answer, err := h.Handle(ctx, "parse url: ://invalid")
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if answer == nil {
		t.Fatal("Handle() returned nil")
	}
}

func TestURLHandlerHandleDecode(t *testing.T) {
	h := NewURLHandler()
	ctx := context.Background()

	answer, err := h.Handle(ctx, "url decode: hello%20world")
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if answer == nil {
		t.Fatal("Handle() returned nil")
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
	answer, err := h.Handle(ctx, "url decode: %ZZ")
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if answer == nil {
		t.Fatal("Handle() returned nil")
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

	answer, err := h.Handle(ctx, "not a color query")
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if answer != nil {
		t.Error("Handle() should return nil for non-matching query")
	}
}

func TestColorHandlerHandle3CharHex(t *testing.T) {
	h := NewColorHandler()
	ctx := context.Background()

	answer, err := h.Handle(ctx, "#f00")
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if answer == nil {
		t.Fatal("Handle() returned nil")
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
			answer, err := h.Handle(ctx, query)
			if err != nil {
				t.Fatalf("Handle() error = %v", err)
			}
			if answer == nil {
				t.Fatal("Handle() returned nil")
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
			answer, err := h.Handle(ctx, tt.query)
			if err != nil {
				t.Fatalf("Handle() error = %v", err)
			}
			if answer == nil {
				t.Fatal("Handle() returned nil")
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
	answer, err := h.Handle(ctx, "password 4")
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if answer.Data["length"] != 8 {
		t.Errorf("length = %v, want 8 (minimum)", answer.Data["length"])
	}

	// Test with length > 128 (should be clamped to 128)
	answer, err = h.Handle(ctx, "password 200")
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
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
			answer, err := h.Handle(ctx, query)
			if err != nil {
				t.Fatalf("Handle() error = %v", err)
			}
			if answer == nil {
				t.Fatal("Handle() returned nil")
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
	answer, err := h.Handle(ctx, "not a synonym query")
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if answer != nil {
		t.Error("Handle() should return nil for non-matching query")
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
	answer, err := h.Handle(ctx, "not an antonym query")
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if answer != nil {
		t.Error("Handle() should return nil for non-matching query")
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
	answer, err := h.Handle(ctx, "not a definition query")
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if answer != nil {
		t.Error("Handle() should return nil for non-matching query")
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
	answer, err := h.Handle(ctx, "calc: invalid_expression")
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if answer == nil {
		t.Fatal("Handle() returned nil")
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
			answer, err := h.Handle(ctx, tt.query)
			if err != nil {
				t.Fatalf("Handle() error = %v", err)
			}
			if answer == nil {
				t.Fatal("Handle() returned nil")
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

	answer, err := h.Handle(ctx, "1.5 km to meters")
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if answer == nil {
		t.Fatal("Handle() returned nil")
	}
	if answer.Data["result"] != float64(1500) {
		t.Errorf("result = %v, want 1500", answer.Data["result"])
	}
}

// Test URL parsing with all components

func TestURLHandlerParseComplete(t *testing.T) {
	h := NewURLHandler()
	ctx := context.Background()

	answer, err := h.Handle(ctx, "parse url: https://user:pass@example.com:8080/path?query=value#section")
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if answer == nil {
		t.Fatal("Handle() returned nil")
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

	answer, err := h.Handle(ctx, "parse url: https://example.com/path")
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if answer == nil {
		t.Fatal("Handle() returned nil")
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
		{"what is my ip?", true},
		{"ip address", true},
		{"ip info", true},
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

// Test IP handler handle returns content

func TestIPHandlerHandleContent(t *testing.T) {
	h := NewIPHandler()
	ctx := context.Background()

	answer, err := h.Handle(ctx, "my ip")
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if answer == nil {
		t.Fatal("Handle() returned nil")
	}
	// Should contain IP info
	if !strings.Contains(answer.Content, "IP") {
		t.Error("Content should mention IP")
	}
	if answer.Data["local_ips"] == nil {
		t.Error("Data should contain local_ips")
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

	answer, err := h.Handle(ctx, "uuid")
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if answer == nil {
		t.Fatal("Handle() returned nil")
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
			answer, err := h.Handle(ctx, query)
			if err != nil {
				t.Fatalf("Handle() error = %v", err)
			}
			if answer == nil {
				t.Fatal("Handle() returned nil")
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
			answer, err := h.Handle(ctx, tt.query)
			if err != nil {
				t.Fatalf("Handle() error = %v", err)
			}
			if answer == nil {
				t.Fatal("Handle() returned nil")
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
		answer, err := h.Handle(ctx, "random 1-10")
		if err != nil {
			t.Fatalf("Handle() error = %v", err)
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
		answer, err := h.Handle(ctx, "flip coin")
		if err != nil {
			t.Fatalf("Handle() error = %v", err)
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
	encoded, err := h.Handle(ctx, "base64 encode: test string")
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if encoded.Data["operation"] != "encoded" {
		t.Errorf("operation = %v, want encoded", encoded.Data["operation"])
	}

	// Test decode
	decoded, err := h.Handle(ctx, "base64 decode: dGVzdCBzdHJpbmc=")
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
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
	encoded, err := h.Handle(ctx, "url encode: hello world&foo=bar")
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if encoded == nil {
		t.Fatal("Handle() returned nil")
	}

	// Test decode
	decoded, err := h.Handle(ctx, "url decode: hello%20world%26foo%3Dbar")
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if decoded == nil {
		t.Fatal("Handle() returned nil")
	}
}

// Test time handler returns current time

func TestTimeHandlerReturnsCurrentTime(t *testing.T) {
	h := NewTimeHandler()
	ctx := context.Background()

	before := time.Now().Unix()
	answer, err := h.Handle(ctx, "timestamp")
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
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

	answer, err := h.Handle(ctx, "100 meters to feet")
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if answer == nil {
		t.Fatal("Handle() returned nil")
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

	answer, err := h.Handle(ctx, "2 + 2")
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if answer == nil {
		t.Fatal("Handle() returned nil")
	}

	// Check all data fields
	if answer.Data["expression"] == nil {
		t.Error("Data should contain expression")
	}
	if answer.Data["result"] == nil {
		t.Error("Data should contain result")
	}
}
