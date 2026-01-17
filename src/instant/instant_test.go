package instant

import (
	"context"
	"testing"
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
