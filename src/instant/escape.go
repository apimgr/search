package instant

import (
	"context"
	"encoding/hex"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

// EscapeHandler handles string escaping for various formats
type EscapeHandler struct {
	patterns []*regexp.Regexp
}

// NewEscapeHandler creates a new escape handler
func NewEscapeHandler() *EscapeHandler {
	return &EscapeHandler{
		patterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)^escape[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^unescape[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^html\s+escape[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^js\s+escape[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^javascript\s+escape[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^sql\s+escape[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^regex\s+escape[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^shell\s+escape[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^csv\s+escape[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^xml\s+escape[:\s]+(.+)$`),
		},
	}
}

func (h *EscapeHandler) Name() string {
	return "escape"
}

func (h *EscapeHandler) Patterns() []*regexp.Regexp {
	return h.patterns
}

func (h *EscapeHandler) CanHandle(query string) bool {
	for _, p := range h.patterns {
		if p.MatchString(query) {
			return true
		}
	}
	return false
}

func (h *EscapeHandler) Handle(ctx context.Context, query string) (*Answer, error) {
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

	// Generate all escape variations
	htmlEsc := escapeHTML(text)
	jsEsc := escapeJavaScript(text)
	urlEsc := url.QueryEscape(text)
	sqlEsc := escapeSQL(text)
	regexEsc := escapeRegex(text)
	shellEsc := escapeShell(text)
	csvEsc := escapeCSV(text)
	xmlEsc := escapeXML(text)
	unicodeEsc := escapeUnicode(text)
	hexEsc := escapeHex(text)

	content := fmt.Sprintf(`<div class="escape-result">
<strong>Input:</strong> <code>%s</code><br><br>
<table class="escape-table">
<tr><td><strong>HTML:</strong></td><td><code>%s</code></td></tr>
<tr><td><strong>JavaScript:</strong></td><td><code>%s</code></td></tr>
<tr><td><strong>URL:</strong></td><td><code>%s</code></td></tr>
<tr><td><strong>SQL:</strong></td><td><code>%s</code></td></tr>
<tr><td><strong>Regex:</strong></td><td><code>%s</code></td></tr>
<tr><td><strong>Shell:</strong></td><td><code>%s</code></td></tr>
<tr><td><strong>CSV:</strong></td><td><code>%s</code></td></tr>
<tr><td><strong>XML:</strong></td><td><code>%s</code></td></tr>
<tr><td><strong>Unicode:</strong></td><td><code>%s</code></td></tr>
<tr><td><strong>Hex:</strong></td><td><code>%s</code></td></tr>
</table>
</div>`, escapeHTML(text), escapeHTML(htmlEsc), escapeHTML(jsEsc),
		escapeHTML(urlEsc), escapeHTML(sqlEsc), escapeHTML(regexEsc),
		escapeHTML(shellEsc), escapeHTML(csvEsc), escapeHTML(xmlEsc),
		escapeHTML(unicodeEsc), escapeHTML(hexEsc))

	return &Answer{
		Type:    AnswerTypeEscape,
		Query:   query,
		Title:   "String Escaper",
		Content: content,
		Data: map[string]interface{}{
			"input":      text,
			"html":       htmlEsc,
			"javascript": jsEsc,
			"url":        urlEsc,
			"sql":        sqlEsc,
			"regex":      regexEsc,
			"shell":      shellEsc,
			"csv":        csvEsc,
			"xml":        xmlEsc,
			"unicode":    unicodeEsc,
			"hex":        hexEsc,
		},
	}, nil
}

// Note: escapeHTML is defined in cert.go

// escapeJavaScript escapes JavaScript string special characters
func escapeJavaScript(s string) string {
	var result strings.Builder
	for _, r := range s {
		switch r {
		case '\\':
			result.WriteString("\\\\")
		case '"':
			result.WriteString("\\\"")
		case '\'':
			result.WriteString("\\'")
		case '\n':
			result.WriteString("\\n")
		case '\r':
			result.WriteString("\\r")
		case '\t':
			result.WriteString("\\t")
		case '\b':
			result.WriteString("\\b")
		case '\f':
			result.WriteString("\\f")
		case '<':
			result.WriteString("\\x3C")
		case '>':
			result.WriteString("\\x3E")
		case '/':
			result.WriteString("\\/")
		default:
			if r < 32 || r > 126 {
				if r <= 0xFFFF {
					result.WriteString(fmt.Sprintf("\\u%04X", r))
				} else {
					result.WriteString(fmt.Sprintf("\\u%04X\\u%04X",
						0xD800+((r-0x10000)>>10),
						0xDC00+((r-0x10000)&0x3FF)))
				}
			} else {
				result.WriteRune(r)
			}
		}
	}
	return result.String()
}

// escapeSQL escapes SQL string special characters
func escapeSQL(s string) string {
	replacer := strings.NewReplacer(
		"'", "''",
		"\\", "\\\\",
		"\x00", "\\0",
		"\n", "\\n",
		"\r", "\\r",
		"\x1a", "\\Z",
	)
	return replacer.Replace(s)
}

// escapeRegex escapes regular expression special characters
func escapeRegex(s string) string {
	special := []string{"\\", ".", "+", "*", "?", "(", ")", "[", "]", "{", "}", "|", "^", "$"}
	result := s
	for _, char := range special {
		result = strings.ReplaceAll(result, char, "\\"+char)
	}
	return result
}

// escapeShell escapes shell special characters
func escapeShell(s string) string {
	// For single-quoted strings, only single quotes need escaping
	// We'll use the double-quote style escaping here
	var result strings.Builder
	result.WriteRune('"')
	for _, r := range s {
		switch r {
		case '"', '$', '`', '\\', '!':
			result.WriteRune('\\')
			result.WriteRune(r)
		case '\n':
			result.WriteString("\\n")
		case '\t':
			result.WriteString("\\t")
		default:
			result.WriteRune(r)
		}
	}
	result.WriteRune('"')
	return result.String()
}

// escapeCSV escapes CSV field content
func escapeCSV(s string) string {
	needsQuotes := strings.ContainsAny(s, ",\"\n\r")
	if !needsQuotes {
		return s
	}

	// Double up any quotes and wrap in quotes
	escaped := strings.ReplaceAll(s, "\"", "\"\"")
	return "\"" + escaped + "\""
}

// escapeXML escapes XML special characters
func escapeXML(s string) string {
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		"\"", "&quot;",
		"'", "&apos;",
	)
	return replacer.Replace(s)
}

// escapeUnicode converts non-ASCII characters to Unicode escape sequences
func escapeUnicode(s string) string {
	var result strings.Builder
	for _, r := range s {
		if r > 127 {
			result.WriteString(fmt.Sprintf("\\u%04X", r))
		} else {
			result.WriteRune(r)
		}
	}
	return result.String()
}

// escapeHex converts string to hexadecimal representation
func escapeHex(s string) string {
	return hex.EncodeToString([]byte(s))
}

// unescapeString attempts to unescape a string
func unescapeString(s string) string {
	// Try URL decoding
	if decoded, err := url.QueryUnescape(s); err == nil && decoded != s {
		return decoded
	}

	// Try unescaping common sequences
	result := s
	result = strings.ReplaceAll(result, "\\n", "\n")
	result = strings.ReplaceAll(result, "\\r", "\r")
	result = strings.ReplaceAll(result, "\\t", "\t")
	result = strings.ReplaceAll(result, "\\\\", "\\")
	result = strings.ReplaceAll(result, "\\'", "'")
	result = strings.ReplaceAll(result, "\\\"", "\"")

	// Try unicode escapes
	unicodePattern := regexp.MustCompile(`\\u([0-9A-Fa-f]{4})`)
	result = unicodePattern.ReplaceAllStringFunc(result, func(match string) string {
		code, err := strconv.ParseInt(match[2:], 16, 32)
		if err != nil {
			return match
		}
		return string(rune(code))
	})

	// Try HTML entities
	htmlEntities := map[string]string{
		"&amp;":  "&",
		"&lt;":   "<",
		"&gt;":   ">",
		"&quot;": "\"",
		"&#39;":  "'",
		"&apos;": "'",
	}
	for entity, char := range htmlEntities {
		result = strings.ReplaceAll(result, entity, char)
	}

	return result
}

// countCharacters returns character count info
func countCharacters(s string) (int, int, int) {
	bytes := len(s)
	chars := utf8.RuneCountInString(s)
	words := len(strings.Fields(s))
	return bytes, chars, words
}
