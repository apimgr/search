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
	patterns []*regexp.Regexp
	mathExpr *regexp.Regexp
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
		// Pattern to detect math expressions (numbers and operators)
		mathExpr: regexp.MustCompile(`^[\d\s\+\-\*\/\(\)\.\^\%]+$`),
	}
}

func (h *MathHandler) Name() string {
	return "math"
}

func (h *MathHandler) Patterns() []*regexp.Regexp {
	return h.patterns
}

func (h *MathHandler) CanHandle(query string) bool {
	// Check explicit patterns
	for _, p := range h.patterns {
		if p.MatchString(query) {
			return true
		}
	}

	// Check if it looks like a math expression
	cleaned := strings.ReplaceAll(query, " ", "")
	if h.mathExpr.MatchString(cleaned) && len(cleaned) > 2 {
		// Must contain at least one operator
		for _, op := range []string{"+", "-", "*", "/", "^", "%"} {
			if strings.Contains(cleaned, op) {
				return true
			}
		}
	}

	return false
}

func (h *MathHandler) Handle(ctx context.Context, query string) (*Answer, error) {
	// Extract expression
	expr := query
	for _, p := range h.patterns {
		if matches := p.FindStringSubmatch(query); len(matches) > 1 {
			expr = strings.TrimSpace(matches[1])
			break
		}
	}

	// Evaluate expression
	result, err := evaluateExpression(expr)
	if err != nil {
		return &Answer{
			Type:    AnswerTypeMath,
			Query:   query,
			Title:   "Calculator",
			Content: fmt.Sprintf("Error: %v", err),
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

// evaluateExpression safely evaluates a mathematical expression
func evaluateExpression(expr string) (float64, error) {
	// Replace ^ with ** for power (we'll handle it manually)
	expr = strings.ReplaceAll(expr, "^", "**")

	// Handle percentages
	percentPattern := regexp.MustCompile(`(\d+(?:\.\d+)?)\s*%\s*of\s*(\d+(?:\.\d+)?)`)
	if matches := percentPattern.FindStringSubmatch(expr); len(matches) == 3 {
		percent, _ := strconv.ParseFloat(matches[1], 64)
		value, _ := strconv.ParseFloat(matches[2], 64)
		return (percent / 100) * value, nil
	}

	// Simple percentage at end
	if strings.HasSuffix(expr, "%") {
		numStr := strings.TrimSuffix(expr, "%")
		num, err := strconv.ParseFloat(strings.TrimSpace(numStr), 64)
		if err == nil {
			return num / 100, nil
		}
	}

	// Try to parse and evaluate using Go's parser
	result, err := evalSimple(expr)
	if err != nil {
		return 0, err
	}

	return result, nil
}

// evalSimple evaluates simple arithmetic expressions
func evalSimple(expr string) (float64, error) {
	// Handle power operator
	if strings.Contains(expr, "**") {
		parts := strings.SplitN(expr, "**", 2)
		if len(parts) == 2 {
			base, err := evalSimple(strings.TrimSpace(parts[0]))
			if err != nil {
				return 0, err
			}
			exp, err := evalSimple(strings.TrimSpace(parts[1]))
			if err != nil {
				return 0, err
			}
			return math.Pow(base, exp), nil
		}
	}

	// Parse as Go expression
	node, err := parser.ParseExpr(expr)
	if err != nil {
		return 0, fmt.Errorf("invalid expression: %s", expr)
	}

	return evalNode(node)
}

// evalNode evaluates an AST node
func evalNode(node ast.Expr) (float64, error) {
	switch n := node.(type) {
	case *ast.BasicLit:
		if n.Kind == token.INT || n.Kind == token.FLOAT {
			return strconv.ParseFloat(n.Value, 64)
		}
		return 0, fmt.Errorf("unsupported literal type")

	case *ast.BinaryExpr:
		left, err := evalNode(n.X)
		if err != nil {
			return 0, err
		}
		right, err := evalNode(n.Y)
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
		default:
			return 0, fmt.Errorf("unsupported operator")
		}

	case *ast.ParenExpr:
		return evalNode(n.X)

	case *ast.UnaryExpr:
		val, err := evalNode(n.X)
		if err != nil {
			return 0, err
		}
		if n.Op == token.SUB {
			return -val, nil
		}
		return val, nil

	default:
		return 0, fmt.Errorf("unsupported expression type")
	}
}

// formatNumber formats a number for display
func formatNumber(n float64) string {
	// Check if it's an integer (include up to 1e15)
	if n == math.Trunc(n) && math.Abs(n) <= 1e15 {
		return strconv.FormatInt(int64(n), 10)
	}

	// Format with appropriate precision
	// Use 'g' format for scientific notation to trim trailing zeros
	if math.Abs(n) < 0.0001 || math.Abs(n) >= 1e10 {
		return strconv.FormatFloat(n, 'g', -1, 64)
	}

	// Remove trailing zeros
	s := strconv.FormatFloat(n, 'f', 10, 64)
	s = strings.TrimRight(s, "0")
	s = strings.TrimRight(s, ".")
	return s
}
