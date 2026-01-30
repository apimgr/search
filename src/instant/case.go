package instant

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

// CaseHandler handles text case conversions
type CaseHandler struct {
	patterns []*regexp.Regexp
}

// NewCaseHandler creates a new case converter handler
func NewCaseHandler() *CaseHandler {
	return &CaseHandler{
		patterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)^case[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^convert\s+case[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^text\s+case[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^uppercase[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^lowercase[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^titlecase[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^camelcase[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^snakecase[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^snake_case[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^kebabcase[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^kebab-case[:\s]+(.+)$`),
		},
	}
}

func (h *CaseHandler) Name() string {
	return "case"
}

func (h *CaseHandler) Patterns() []*regexp.Regexp {
	return h.patterns
}

func (h *CaseHandler) CanHandle(query string) bool {
	for _, p := range h.patterns {
		if p.MatchString(query) {
			return true
		}
	}
	return false
}

func (h *CaseHandler) Handle(ctx context.Context, query string) (*Answer, error) {
	// Extract text from query
	text := ""
	for _, p := range h.patterns {
		if matches := p.FindStringSubmatch(query); len(matches) > 1 {
			text = strings.TrimSpace(matches[1])
			break
		}
	}

	if text == "" {
		return nil, nil
	}

	// Generate all case variations
	upper := strings.ToUpper(text)
	lower := strings.ToLower(text)
	title := toTitleCase(text)
	camel := toCamelCase(text)
	pascal := toPascalCase(text)
	snake := toSnakeCase(text)
	kebab := toKebabCase(text)
	constant := toConstantCase(text)

	content := fmt.Sprintf(`<div class="case-result">
<strong>Input:</strong> %s<br><br>
<table class="case-table">
<tr><td><strong>UPPERCASE:</strong></td><td><code>%s</code></td></tr>
<tr><td><strong>lowercase:</strong></td><td><code>%s</code></td></tr>
<tr><td><strong>Title Case:</strong></td><td><code>%s</code></td></tr>
<tr><td><strong>camelCase:</strong></td><td><code>%s</code></td></tr>
<tr><td><strong>PascalCase:</strong></td><td><code>%s</code></td></tr>
<tr><td><strong>snake_case:</strong></td><td><code>%s</code></td></tr>
<tr><td><strong>kebab-case:</strong></td><td><code>%s</code></td></tr>
<tr><td><strong>CONSTANT_CASE:</strong></td><td><code>%s</code></td></tr>
</table>
</div>`, text, upper, lower, title, camel, pascal, snake, kebab, constant)

	return &Answer{
		Type:    AnswerTypeCase,
		Query:   query,
		Title:   "Text Case Converter",
		Content: content,
		Data: map[string]interface{}{
			"input":     text,
			"upper":     upper,
			"lower":     lower,
			"title":     title,
			"camel":     camel,
			"pascal":    pascal,
			"snake":     snake,
			"kebab":     kebab,
			"constant":  constant,
		},
	}, nil
}

// toTitleCase converts text to Title Case
func toTitleCase(s string) string {
	words := splitIntoWords(s)
	for i, word := range words {
		if len(word) > 0 {
			words[i] = strings.ToUpper(string(word[0])) + strings.ToLower(word[1:])
		}
	}
	return strings.Join(words, " ")
}

// toCamelCase converts text to camelCase
func toCamelCase(s string) string {
	words := splitIntoWords(s)
	for i, word := range words {
		if i == 0 {
			words[i] = strings.ToLower(word)
		} else if len(word) > 0 {
			words[i] = strings.ToUpper(string(word[0])) + strings.ToLower(word[1:])
		}
	}
	return strings.Join(words, "")
}

// toPascalCase converts text to PascalCase
func toPascalCase(s string) string {
	words := splitIntoWords(s)
	for i, word := range words {
		if len(word) > 0 {
			words[i] = strings.ToUpper(string(word[0])) + strings.ToLower(word[1:])
		}
	}
	return strings.Join(words, "")
}

// toSnakeCase converts text to snake_case
func toSnakeCase(s string) string {
	words := splitIntoWords(s)
	for i, word := range words {
		words[i] = strings.ToLower(word)
	}
	return strings.Join(words, "_")
}

// toKebabCase converts text to kebab-case
func toKebabCase(s string) string {
	words := splitIntoWords(s)
	for i, word := range words {
		words[i] = strings.ToLower(word)
	}
	return strings.Join(words, "-")
}

// toConstantCase converts text to CONSTANT_CASE
func toConstantCase(s string) string {
	words := splitIntoWords(s)
	for i, word := range words {
		words[i] = strings.ToUpper(word)
	}
	return strings.Join(words, "_")
}

// splitIntoWords splits a string into words, handling various formats
func splitIntoWords(s string) []string {
	// First, replace common separators with spaces
	s = strings.ReplaceAll(s, "_", " ")
	s = strings.ReplaceAll(s, "-", " ")

	// Handle camelCase and PascalCase
	var words []string
	var currentWord strings.Builder

	for i, r := range s {
		if r == ' ' {
			if currentWord.Len() > 0 {
				words = append(words, currentWord.String())
				currentWord.Reset()
			}
			continue
		}

		// Check for uppercase letters that indicate word boundaries
		if unicode.IsUpper(r) && currentWord.Len() > 0 {
			// Check if the previous character was lowercase (camelCase boundary)
			prev := []rune(s)[i-1]
			if unicode.IsLower(prev) {
				words = append(words, currentWord.String())
				currentWord.Reset()
			}
		}

		currentWord.WriteRune(r)
	}

	if currentWord.Len() > 0 {
		words = append(words, currentWord.String())
	}

	// Filter out empty words
	var result []string
	for _, word := range words {
		word = strings.TrimSpace(word)
		if word != "" {
			result = append(result, word)
		}
	}

	return result
}
