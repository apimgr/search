package instant

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

// AnswerTypeRegex is the answer type for regex explanations
const AnswerTypeRegex AnswerType = "regex"

// RegexHandler handles regex pattern explanations and testing
type RegexHandler struct {
	patterns []*regexp.Regexp
}

// NewRegexHandler creates a new regex handler
func NewRegexHandler() *RegexHandler {
	return &RegexHandler{
		patterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)^regex[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^regexp[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^explain\s+regex[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^regex\s+explain[:\s]+(.+)$`),
		},
	}
}

func (h *RegexHandler) Name() string              { return "regex" }
func (h *RegexHandler) Patterns() []*regexp.Regexp { return h.patterns }

func (h *RegexHandler) CanHandle(query string) bool {
	for _, p := range h.patterns {
		if p.MatchString(query) {
			return true
		}
	}
	return false
}

func (h *RegexHandler) Handle(ctx context.Context, query string) (*Answer, error) {
	// Extract pattern from query
	var pattern string
	for _, p := range h.patterns {
		if matches := p.FindStringSubmatch(query); len(matches) > 1 {
			pattern = strings.TrimSpace(matches[1])
			break
		}
	}

	if pattern == "" {
		return nil, nil
	}

	// Validate the regex
	_, err := regexp.Compile(pattern)
	isValid := err == nil

	// Build explanation
	var content strings.Builder
	content.WriteString(fmt.Sprintf("<strong>Pattern:</strong> <code>%s</code><br><br>", escapeHTML(pattern)))

	if !isValid {
		content.WriteString(fmt.Sprintf("<span class=\"error\"><strong>Invalid Regex:</strong> %s</span><br>", escapeHTML(err.Error())))
	} else {
		content.WriteString("<strong>Status:</strong> <span class=\"success\">Valid</span><br><br>")
		content.WriteString("<strong>Explanation:</strong><br>")
		explanation := explainRegex(pattern)
		for _, line := range explanation {
			content.WriteString(fmt.Sprintf("&bull; %s<br>", line))
		}
	}

	// Add common regex reference
	content.WriteString("<br><strong>Quick Reference:</strong><br>")
	content.WriteString("<code>.</code> - Any character<br>")
	content.WriteString("<code>*</code> - Zero or more<br>")
	content.WriteString("<code>+</code> - One or more<br>")
	content.WriteString("<code>?</code> - Zero or one<br>")
	content.WriteString("<code>^</code> - Start of string<br>")
	content.WriteString("<code>$</code> - End of string<br>")
	content.WriteString("<code>\\d</code> - Digit<br>")
	content.WriteString("<code>\\w</code> - Word character<br>")
	content.WriteString("<code>\\s</code> - Whitespace<br>")

	return &Answer{
		Type:      AnswerTypeRegex,
		Query:     query,
		Title:     "Regex Explainer",
		Content:   content.String(),
		Source:    "Local regex analyzer",
		SourceURL: "https://regex101.com/",
		Data: map[string]interface{}{
			"pattern": pattern,
			"valid":   isValid,
		},
	}, nil
}

// explainRegex provides a human-readable explanation of regex components
func explainRegex(pattern string) []string {
	var explanations []string

	// Check for anchors
	if strings.HasPrefix(pattern, "^") {
		explanations = append(explanations, "Anchored to start of string (^)")
	}
	if strings.HasSuffix(pattern, "$") && !strings.HasSuffix(pattern, "\\$") {
		explanations = append(explanations, "Anchored to end of string ($)")
	}

	// Check for common patterns
	if strings.Contains(pattern, "\\d") {
		explanations = append(explanations, "Contains digit matcher (\\d)")
	}
	if strings.Contains(pattern, "\\D") {
		explanations = append(explanations, "Contains non-digit matcher (\\D)")
	}
	if strings.Contains(pattern, "\\w") {
		explanations = append(explanations, "Contains word character matcher (\\w = [a-zA-Z0-9_])")
	}
	if strings.Contains(pattern, "\\W") {
		explanations = append(explanations, "Contains non-word character matcher (\\W)")
	}
	if strings.Contains(pattern, "\\s") {
		explanations = append(explanations, "Contains whitespace matcher (\\s)")
	}
	if strings.Contains(pattern, "\\S") {
		explanations = append(explanations, "Contains non-whitespace matcher (\\S)")
	}
	if strings.Contains(pattern, "\\b") {
		explanations = append(explanations, "Contains word boundary (\\b)")
	}

	// Check for quantifiers
	if strings.Contains(pattern, "*") {
		explanations = append(explanations, "Uses zero-or-more quantifier (*)")
	}
	if strings.Contains(pattern, "+") {
		explanations = append(explanations, "Uses one-or-more quantifier (+)")
	}
	if strings.Contains(pattern, "?") && !strings.Contains(pattern, "(?") {
		explanations = append(explanations, "Uses optional quantifier (?)")
	}

	// Check for groups
	if strings.Contains(pattern, "(") {
		if strings.Contains(pattern, "(?:") {
			explanations = append(explanations, "Contains non-capturing group (?:...)")
		}
		if strings.Contains(pattern, "(?=") {
			explanations = append(explanations, "Contains positive lookahead (?=...)")
		}
		if strings.Contains(pattern, "(?!") {
			explanations = append(explanations, "Contains negative lookahead (?!...)")
		}
		if strings.Contains(pattern, "(?<=") {
			explanations = append(explanations, "Contains positive lookbehind (?<=...)")
		}
		if strings.Contains(pattern, "(?<!") {
			explanations = append(explanations, "Contains negative lookbehind (?<!...)")
		}
		if strings.Contains(pattern, "(?i)") {
			explanations = append(explanations, "Case-insensitive mode (?i)")
		}
		// Check for capturing groups (simple heuristic)
		groupCount := countCapturingGroups(pattern)
		if groupCount > 0 {
			explanations = append(explanations, fmt.Sprintf("Contains %d capturing group(s)", groupCount))
		}
	}

	// Check for character classes
	if strings.Contains(pattern, "[") {
		if strings.Contains(pattern, "[^") {
			explanations = append(explanations, "Contains negated character class [^...]")
		} else {
			explanations = append(explanations, "Contains character class [...]")
		}
	}

	// Check for alternation
	if strings.Contains(pattern, "|") {
		explanations = append(explanations, "Contains alternation (|)")
	}

	// Check for repetition ranges
	if strings.Contains(pattern, "{") {
		explanations = append(explanations, "Contains repetition quantifier {...}")
	}

	// Check for common email/URL patterns
	if strings.Contains(pattern, "@") && strings.Contains(pattern, "\\.") {
		explanations = append(explanations, "Appears to match email-like patterns")
	}
	if strings.Contains(pattern, "http") || strings.Contains(pattern, "://") {
		explanations = append(explanations, "Appears to match URL-like patterns")
	}

	if len(explanations) == 0 {
		explanations = append(explanations, "Simple literal pattern")
	}

	return explanations
}

// countCapturingGroups counts capturing groups (excludes non-capturing)
func countCapturingGroups(pattern string) int {
	count := 0
	for i := 0; i < len(pattern); i++ {
		if pattern[i] == '(' && i+1 < len(pattern) {
			if pattern[i+1] != '?' {
				count++
			}
		}
	}
	return count
}
