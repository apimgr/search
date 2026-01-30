package instant

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// SlugHandler handles URL slug generation
type SlugHandler struct {
	patterns []*regexp.Regexp
}

// NewSlugHandler creates a new slug handler
func NewSlugHandler() *SlugHandler {
	return &SlugHandler{
		patterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)^slug[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^slugify[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^url\s+slug[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^to\s+slug[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^make\s+slug[:\s]+(.+)$`),
		},
	}
}

func (h *SlugHandler) Name() string {
	return "slug"
}

func (h *SlugHandler) Patterns() []*regexp.Regexp {
	return h.patterns
}

func (h *SlugHandler) CanHandle(query string) bool {
	for _, p := range h.patterns {
		if p.MatchString(query) {
			return true
		}
	}
	return false
}

func (h *SlugHandler) Handle(ctx context.Context, query string) (*Answer, error) {
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

	// Generate different slug variations
	basic := toBasicSlug(text)
	underscored := toUnderscoreSlug(text)
	camelSlug := toSlugCamelCase(text)
	maxLen := toTruncatedSlug(text, 50)

	content := fmt.Sprintf(`<div class="slug-result">
<strong>Input:</strong> %s<br><br>
<table class="slug-table">
<tr><td><strong>Standard Slug:</strong></td><td><code>%s</code></td></tr>
<tr><td><strong>Underscore Slug:</strong></td><td><code>%s</code></td></tr>
<tr><td><strong>CamelCase:</strong></td><td><code>%s</code></td></tr>
<tr><td><strong>Truncated (50 chars):</strong></td><td><code>%s</code></td></tr>
</table>
<br>
<small>Slugs are URL-friendly versions of text with special characters removed.</small>
</div>`, text, basic, underscored, camelSlug, maxLen)

	return &Answer{
		Type:    AnswerTypeSlug,
		Query:   query,
		Title:   "URL Slug Generator",
		Content: content,
		Data: map[string]interface{}{
			"input":       text,
			"slug":        basic,
			"underscore":  underscored,
			"camel":       camelSlug,
			"truncated":   maxLen,
		},
	}, nil
}

// toBasicSlug creates a standard URL slug (lowercase, hyphens)
func toBasicSlug(s string) string {
	// Normalize unicode characters (e.g., accents)
	s = normalizeText(s)

	// Convert to lowercase
	s = strings.ToLower(s)

	// Replace spaces and underscores with hyphens
	s = strings.ReplaceAll(s, " ", "-")
	s = strings.ReplaceAll(s, "_", "-")

	// Remove all non-alphanumeric characters except hyphens
	var result strings.Builder
	prevHyphen := false
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			result.WriteRune(r)
			prevHyphen = false
		} else if r == '-' && !prevHyphen && result.Len() > 0 {
			result.WriteRune('-')
			prevHyphen = true
		}
	}

	// Trim trailing hyphens
	return strings.Trim(result.String(), "-")
}

// toUnderscoreSlug creates a slug with underscores instead of hyphens
func toUnderscoreSlug(s string) string {
	s = normalizeText(s)
	s = strings.ToLower(s)

	// Replace spaces and hyphens with underscores
	s = strings.ReplaceAll(s, " ", "_")
	s = strings.ReplaceAll(s, "-", "_")

	// Remove all non-alphanumeric characters except underscores
	var result strings.Builder
	prevUnderscore := false
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			result.WriteRune(r)
			prevUnderscore = false
		} else if r == '_' && !prevUnderscore && result.Len() > 0 {
			result.WriteRune('_')
			prevUnderscore = true
		}
	}

	return strings.Trim(result.String(), "_")
}

// toSlugCamelCase creates a camelCase version without separators
func toSlugCamelCase(s string) string {
	s = normalizeText(s)

	// Split by non-alphanumeric
	words := regexp.MustCompile(`[^a-zA-Z0-9]+`).Split(s, -1)

	var result strings.Builder
	for i, word := range words {
		if word == "" {
			continue
		}
		if i == 0 {
			result.WriteString(strings.ToLower(word))
		} else {
			result.WriteString(strings.ToUpper(string(word[0])))
			if len(word) > 1 {
				result.WriteString(strings.ToLower(word[1:]))
			}
		}
	}

	return result.String()
}

// toTruncatedSlug creates a slug truncated to maxLen characters
func toTruncatedSlug(s string, maxLen int) string {
	slug := toBasicSlug(s)

	if len(slug) <= maxLen {
		return slug
	}

	// Find the last hyphen before maxLen
	truncated := slug[:maxLen]
	lastHyphen := strings.LastIndex(truncated, "-")
	if lastHyphen > 0 {
		truncated = truncated[:lastHyphen]
	}

	return truncated
}

// normalizeText removes accents and normalizes unicode
func normalizeText(s string) string {
	t := transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)
	result, _, _ := transform.String(t, s)
	return result
}
