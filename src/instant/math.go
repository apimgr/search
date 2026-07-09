package instant

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"math"
	"regexp"
	"strconv"
	"strings"
)

// MathHandler handles mathematical expressions
type MathHandler struct {
	patterns    []*regexp.Regexp
	mathExpr    *regexp.Regexp
	percentOf   *regexp.Regexp
	funcPattern *regexp.Regexp
	funcNames   map[string]func(float64) float64
	constants   map[string]float64
}

// NewMathHandler creates a new math handler
func NewMathHandler() *MathHandler {
	return &MathHandler{
		patterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)^calc(?:ulate)?[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^math[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^eval(?:uate)?[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^compute[:\s]+(.+)$`),
		},
		// Pattern to detect math expressions (numbers, operators, function names, constants)
		mathExpr:    regexp.MustCompile(`^[\d\s\+\-\*\/\(\)\.\^\%a-zA-Z_]+$`),
		percentOf:   regexp.MustCompile(`(?i)(\d+(?:\.\d+)?)\s*%\s*of\s*(\d+(?:\.\d+)?)`),
		funcPattern: regexp.MustCompile(`(?i)^(sqrt|abs|ceil|floor|round|sin|cos|tan|asin|acos|atan|log|log2|ln|exp|cbrt)\s*\((.+)\)\s*$`),
		funcNames: map[string]func(float64) float64{
			"sqrt":  math.Sqrt,
			"abs":   math.Abs,
			"ceil":  math.Ceil,
			"floor": math.Floor,
			"round": math.Round,
			"sin":   func(x float64) float64 { return math.Sin(x * math.Pi / 180) },
			"cos":   func(x float64) float64 { return math.Cos(x * math.Pi / 180) },
			"tan":   func(x float64) float64 { return math.Tan(x * math.Pi / 180) },
			"asin":  func(x float64) float64 { return math.Asin(x) * 180 / math.Pi },
			"acos":  func(x float64) float64 { return math.Acos(x) * 180 / math.Pi },
			"atan":  func(x float64) float64 { return math.Atan(x) * 180 / math.Pi },
			"log":   math.Log10,
			"log2":  math.Log2,
			"ln":    math.Log,
			"exp":   math.Exp,
			"cbrt":  math.Cbrt,
		},
		constants: map[string]float64{
			"pi":  math.Pi,
			"e":   math.E,
			"tau": 2 * math.Pi,
			"phi": (1 + math.Sqrt(5)) / 2,
		},
	}
}

func (h *MathHandler) Name() string {
	return "math"
}

func (h *MathHandler) Patterns() []*regexp.Regexp {
	return h.patterns
}

func (h *MathHandler) CanHandle(query string) bool {
	// Check explicit patterns (calc:, math:, eval:, compute:)
	for _, p := range h.patterns {
		if p.MatchString(query) {
			return true
		}
	}

	// Check percentage patterns: "15% of 200"
	if h.percentOf.MatchString(query) {
		return true
	}

	// Check if it looks like a math expression.
	// Require at least one digit so that words containing a hyphen
	// (e.g. "apt dist-upgrade") are not misidentified as subtraction.
	cleaned := strings.TrimSpace(query)
	hasDigit := strings.ContainsAny(cleaned, "0123456789")
	if hasDigit && h.mathExpr.MatchString(cleaned) && len(cleaned) > 2 {
		// Must contain at least one operator or function call
		for _, op := range []string{"+", "-", "*", "/", "^", "%", "("} {
			if strings.Contains(cleaned, op) {
				return true
			}
		}
	}

	return false
}

func (h *MathHandler) HandleInstantQuery(ctx context.Context, query string) (*Answer, error) {
	// Extract expression from prefix patterns
	expr := query
	for _, p := range h.patterns {
		if matches := p.FindStringSubmatch(query); len(matches) > 1 {
			expr = strings.TrimSpace(matches[1])
			break
		}
	}

	// Evaluate expression
	result, err := h.evaluate(expr)
	if err != nil {
		return &Answer{
			Type:    AnswerTypeMath,
			Query:   query,
			Title:   "Calculator",
			Content: fmt.Sprintf("Error: %s", err.Error()),
		}, nil
	}

	// Format result
	resultStr := formatNumber(result)

	return &Answer{
		Type:    AnswerTypeMath,
		Query:   query,
		Title:   "Calculator",
		Content: fmt.Sprintf("<div class=\"math-result\"><span class=\"expression\">%s</span> = <span class=\"result\">%s</span></div>", expr, resultStr),
		Data: map[string]interface{}{
			"expression": expr,
			"result":     result,
		},
	}, nil
}

// evaluate safely evaluates a mathematical expression
func (h *MathHandler) evaluate(expr string) (float64, error) {
	// Handle "X% of Y" pattern
	if matches := h.percentOf.FindStringSubmatch(expr); len(matches) == 3 {
		percent, _ := strconv.ParseFloat(matches[1], 64)
		value, _ := strconv.ParseFloat(matches[2], 64)
		return (percent / 100) * value, nil
	}

	// Handle trailing percentage: "50%" => 0.5
	trimmed := strings.TrimSpace(expr)
	if strings.HasSuffix(trimmed, "%") {
		numStr := strings.TrimSuffix(trimmed, "%")
		if num, err := strconv.ParseFloat(strings.TrimSpace(numStr), 64); err == nil {
			return num / 100, nil
		}
	}

	// Try function call syntax first: funcname(expr)
	// This handles cases where the Go parser might not recognize function names
	if matches := h.funcPattern.FindStringSubmatch(trimmed); len(matches) == 3 {
		funcName := strings.ToLower(matches[1])
		argStr := strings.TrimSpace(matches[2])

		fn, ok := h.funcNames[funcName]
		if !ok {
			return 0, fmt.Errorf("unknown function: %s", funcName)
		}

		arg, err := h.evaluate(argStr)
		if err != nil {
			return 0, err
		}

		result := fn(arg)
		if math.IsNaN(result) || math.IsInf(result, 0) {
			return 0, fmt.Errorf("result is not a valid number")
		}
		return result, nil
	}

	// Replace ** with ^ before parsing; evalNode treats ^ as power (math.Pow).
	trimmed = strings.ReplaceAll(trimmed, "**", "^")

	// Parse as Go expression using AST
	// Note: ^ is parsed as XOR by Go but we treat it as power in evalNode
	node, err := parser.ParseExpr(trimmed)
	if err != nil {
		return 0, fmt.Errorf("invalid expression: %s", trimmed)
	}

	return h.evalNode(node)
}

// evalNode recursively evaluates an AST node
func (h *MathHandler) evalNode(node ast.Expr) (float64, error) {
	switch n := node.(type) {
	case *ast.BasicLit:
		if n.Kind == token.INT || n.Kind == token.FLOAT {
			return strconv.ParseFloat(n.Value, 64)
		}
		return 0, fmt.Errorf("unsupported literal: %s", n.Value)

	case *ast.BinaryExpr:
		left, err := h.evalNode(n.X)
		if err != nil {
			return 0, err
		}
		right, err := h.evalNode(n.Y)
		if err != nil {
			return 0, err
		}

		switch n.Op {
		case token.ADD:
			return left + right, nil
		case token.SUB:
			return left - right, nil
		case token.MUL:
			return left * right, nil
		case token.QUO:
			if right == 0 {
				return 0, fmt.Errorf("division by zero")
			}
			return left / right, nil
		case token.REM:
			if right == 0 {
				return 0, fmt.Errorf("division by zero")
			}
			return math.Mod(left, right), nil
		case token.XOR:
			// Go parses ^ as XOR, but we treat it as power for calculator
			return math.Pow(left, right), nil
		default:
			return 0, fmt.Errorf("unsupported operator: %s", n.Op)
		}

	case *ast.ParenExpr:
		return h.evalNode(n.X)

	case *ast.UnaryExpr:
		val, err := h.evalNode(n.X)
		if err != nil {
			return 0, err
		}
		switch n.Op {
		case token.SUB:
			return -val, nil
		case token.ADD:
			return val, nil
		case token.XOR:
			// Go parses ^x as bitwise complement; treat as positive value
			return val, nil
		default:
			return val, nil
		}

	case *ast.CallExpr:
		// Handle function calls: sqrt(144), sin(45), etc.
		ident, ok := n.Fun.(*ast.Ident)
		if !ok {
			return 0, fmt.Errorf("unsupported function call")
		}

		funcName := strings.ToLower(ident.Name)
		fn, exists := h.funcNames[funcName]
		if !exists {
			return 0, fmt.Errorf("unknown function: %s", funcName)
		}

		if len(n.Args) != 1 {
			return 0, fmt.Errorf("%s takes exactly 1 argument", funcName)
		}

		arg, err := h.evalNode(n.Args[0])
		if err != nil {
			return 0, err
		}

		result := fn(arg)
		if math.IsNaN(result) || math.IsInf(result, 0) {
			return 0, fmt.Errorf("result is not a valid number")
		}
		return result, nil

	case *ast.Ident:
		// Handle named constants: pi, e, tau, phi
		name := strings.ToLower(n.Name)
		if val, ok := h.constants[name]; ok {
			return val, nil
		}
		return 0, fmt.Errorf("unknown identifier: %s", n.Name)

	default:
		return 0, fmt.Errorf("unsupported expression type: %T", node)
	}
}

// formatNumber formats a number for display
func formatNumber(n float64) string {
	// Check if it's an integer
	if n == math.Trunc(n) && math.Abs(n) <= 1e15 {
		return strconv.FormatInt(int64(n), 10)
	}

	// Use scientific notation for very small or very large numbers
	if math.Abs(n) < 0.0001 || math.Abs(n) >= 1e10 {
		return strconv.FormatFloat(n, 'g', -1, 64)
	}

	// Standard decimal with trailing zeros removed
	s := strconv.FormatFloat(n, 'f', 10, 64)
	s = strings.TrimRight(s, "0")
	s = strings.TrimRight(s, ".")
	return s
}

// evaluateExpression is a package-level helper used by tests.
func evaluateExpression(expr string) (float64, error) {
	return NewMathHandler().evaluate(expr)
}

// evalSimple is a package-level helper for simple expressions, used by tests.
func evalSimple(expr string) (float64, error) {
	return NewMathHandler().evaluate(expr)
}
