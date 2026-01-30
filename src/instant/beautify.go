package instant

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// BeautifyHandler handles code beautification queries
type BeautifyHandler struct {
	patterns []*regexp.Regexp
}

// NewBeautifyHandler creates a new code beautify handler
func NewBeautifyHandler() *BeautifyHandler {
	return &BeautifyHandler{
		patterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)^beautify:(\w+)\s+(.+)$`),
			regexp.MustCompile(`(?i)^beautify:minify\s+(.+)$`),
			regexp.MustCompile(`(?i)^format:(\w+)\s+(.+)$`),
		},
	}
}

func (h *BeautifyHandler) Name() string {
	return "beautify"
}

func (h *BeautifyHandler) Patterns() []*regexp.Regexp {
	return h.patterns
}

func (h *BeautifyHandler) CanHandle(query string) bool {
	for _, p := range h.patterns {
		if p.MatchString(query) {
			return true
		}
	}
	return false
}

// BeautifyConfig holds beautification settings
type BeautifyConfig struct {
	IndentSize int
	UseTabs    bool
}

// DefaultBeautifyConfig returns default beautification config
func DefaultBeautifyConfig() *BeautifyConfig {
	return &BeautifyConfig{
		IndentSize: 2,
		UseTabs:    false,
	}
}

func (h *BeautifyHandler) Handle(ctx context.Context, query string) (*Answer, error) {
	var lang, code string
	var minify bool

	// Check for minify mode
	minifyPattern := regexp.MustCompile(`(?i)^beautify:minify\s+(.+)$`)
	if matches := minifyPattern.FindStringSubmatch(query); len(matches) == 2 {
		minify = true
		code = strings.TrimSpace(matches[1])
		lang = detectCodeLanguage(code)
	} else {
		// Check for language-specific patterns
		for _, p := range h.patterns[:2] {
			if matches := p.FindStringSubmatch(query); len(matches) >= 3 {
				lang = strings.ToLower(matches[1])
				code = strings.TrimSpace(matches[2])
				break
			}
		}
	}

	if code == "" {
		return nil, nil
	}

	// Normalize language names
	lang = normalizeLanguage(lang)

	config := DefaultBeautifyConfig()
	var result string
	var err error

	if minify {
		result, err = minifyCode(code, lang)
	} else {
		result, err = beautifyCodeWithConfig(code, lang, config)
	}

	if err != nil {
		return &Answer{
			Type:    AnswerTypeBeautify, // Using JSON type for code formatting
			Query:   query,
			Title:   "Code Beautifier",
			Content: fmt.Sprintf("Error: %v", err),
		}, nil
	}

	mode := "Beautified"
	if minify {
		mode = "Minified"
	}

	return &Answer{
		Type:  AnswerTypeBeautify,
		Query: query,
		Title: fmt.Sprintf("Code %s (%s)", mode, lang),
		Content: fmt.Sprintf(`<div class="beautify-result">
<h2>%s Code (%s)</h2>
<pre><code class="%s">%s</code></pre>
<button class="copy-btn" onclick="copyCode(this)">Copy</button>
</div>`, mode, strings.ToUpper(lang), lang, escapeHTMLContent(result)),
		Data: map[string]interface{}{
			"language":   lang,
			"mode":       strings.ToLower(mode),
			"input":      code,
			"output":     result,
			"inputSize":  len(code),
			"outputSize": len(result),
			"indentSize": config.IndentSize,
		},
	}, nil
}

// normalizeLanguage normalizes language identifiers
func normalizeLanguage(lang string) string {
	aliases := map[string]string{
		"js":         "javascript",
		"javascript": "javascript",
		"css":        "css",
		"html":       "html",
		"htm":        "html",
		"sql":        "sql",
		"json":       "json",
		"xml":        "xml",
	}

	if normalized, ok := aliases[lang]; ok {
		return normalized
	}
	return lang
}

// detectCodeLanguage attempts to auto-detect the code language
func detectCodeLanguage(code string) string {
	code = strings.TrimSpace(code)

	// JSON detection
	if (strings.HasPrefix(code, "{") && strings.HasSuffix(code, "}")) ||
		(strings.HasPrefix(code, "[") && strings.HasSuffix(code, "]")) {
		return "json"
	}

	// HTML/XML detection
	if strings.HasPrefix(code, "<") {
		if strings.Contains(strings.ToLower(code), "<!doctype") ||
			strings.Contains(strings.ToLower(code), "<html") ||
			strings.Contains(strings.ToLower(code), "<body") {
			return "html"
		}
		return "xml"
	}

	// CSS detection
	if strings.Contains(code, "{") && strings.Contains(code, ":") &&
		(strings.Contains(code, ";") || strings.Contains(code, "}")) &&
		!strings.Contains(code, "function") {
		return "css"
	}

	// SQL detection
	upperCode := strings.ToUpper(code)
	if strings.Contains(upperCode, "SELECT") || strings.Contains(upperCode, "INSERT") ||
		strings.Contains(upperCode, "UPDATE") || strings.Contains(upperCode, "DELETE") ||
		strings.Contains(upperCode, "CREATE TABLE") {
		return "sql"
	}

	// JavaScript detection
	if strings.Contains(code, "function") || strings.Contains(code, "const ") ||
		strings.Contains(code, "let ") || strings.Contains(code, "var ") ||
		strings.Contains(code, "=>") {
		return "javascript"
	}

	return "unknown"
}

// beautifyCodeWithConfig beautifies code with the given configuration
func beautifyCodeWithConfig(code, lang string, config *BeautifyConfig) (string, error) {
	indent := strings.Repeat(" ", config.IndentSize)
	if config.UseTabs {
		indent = "\t"
	}

	switch lang {
	case "json":
		return beautifyJSONCode(code, indent)
	case "javascript":
		return beautifyJavaScript(code, indent)
	case "css":
		return beautifyCSS(code, indent)
	case "html":
		return beautifyHTML(code, indent)
	case "xml":
		return beautifyHTML(code, indent) // Reuse HTML beautifier
	case "sql":
		return beautifySQLCode(code, indent)
	default:
		return code, fmt.Errorf("unsupported language: %s", lang)
	}
}

// minifyCode minifies code by removing unnecessary whitespace
func minifyCode(code, lang string) (string, error) {
	switch lang {
	case "json":
		var parsed interface{}
		if err := json.Unmarshal([]byte(code), &parsed); err != nil {
			return "", fmt.Errorf("invalid JSON: %v", err)
		}
		b, _ := json.Marshal(parsed)
		return string(b), nil
	case "css":
		return minifyCSS(code), nil
	case "html", "xml":
		return minifyHTML(code), nil
	case "javascript":
		return minifyJavaScript(code), nil
	case "sql":
		return minifySQL(code), nil
	default:
		return strings.Join(strings.Fields(code), " "), nil
	}
}

// beautifyJSONCode formats JSON with proper indentation
func beautifyJSONCode(code, indent string) (string, error) {
	var parsed interface{}
	if err := json.Unmarshal([]byte(code), &parsed); err != nil {
		return "", fmt.Errorf("invalid JSON: %v", err)
	}

	b, err := json.MarshalIndent(parsed, "", indent)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// beautifyJavaScript formats JavaScript code
func beautifyJavaScript(code, indent string) (string, error) {
	var result strings.Builder
	indentLevel := 0
	inString := false
	stringChar := rune(0)
	escaped := false
	prevChar := rune(0)

	writeIndent := func() {
		for i := 0; i < indentLevel; i++ {
			result.WriteString(indent)
		}
	}

	for _, r := range code {
		if escaped {
			result.WriteRune(r)
			escaped = false
			prevChar = r
			continue
		}

		if r == '\\' && inString {
			result.WriteRune(r)
			escaped = true
			prevChar = r
			continue
		}

		if (r == '"' || r == '\'' || r == '`') && !inString {
			inString = true
			stringChar = r
			result.WriteRune(r)
			prevChar = r
			continue
		}

		if r == stringChar && inString {
			inString = false
			stringChar = 0
			result.WriteRune(r)
			prevChar = r
			continue
		}

		if inString {
			result.WriteRune(r)
			prevChar = r
			continue
		}

		// Skip existing whitespace
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			prevChar = r
			continue
		}

		switch r {
		case '{', '[':
			result.WriteRune(r)
			indentLevel++
			result.WriteRune('\n')
			writeIndent()
		case '}', ']':
			indentLevel--
			result.WriteRune('\n')
			writeIndent()
			result.WriteRune(r)
		case ',':
			result.WriteRune(r)
			result.WriteRune('\n')
			writeIndent()
		case ':':
			result.WriteRune(r)
			result.WriteRune(' ')
		case ';':
			result.WriteRune(r)
			result.WriteRune('\n')
			writeIndent()
		default:
			if prevChar == ',' || prevChar == '{' || prevChar == '[' || prevChar == ';' {
				// Already handled by newline
			}
			result.WriteRune(r)
		}
		prevChar = r
	}

	return result.String(), nil
}

// beautifyCSS formats CSS code
func beautifyCSS(code, indent string) (string, error) {
	var result strings.Builder
	indentLevel := 0
	inString := false
	stringChar := rune(0)

	writeIndent := func() {
		for i := 0; i < indentLevel; i++ {
			result.WriteString(indent)
		}
	}

	for i, r := range code {
		// Handle strings
		if (r == '"' || r == '\'') && !inString {
			inString = true
			stringChar = r
			result.WriteRune(r)
			continue
		}
		if r == stringChar && inString {
			inString = false
			stringChar = 0
			result.WriteRune(r)
			continue
		}
		if inString {
			result.WriteRune(r)
			continue
		}

		// Skip whitespace
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			// Keep single space between tokens if needed
			if i > 0 && result.Len() > 0 {
				lastStr := result.String()
				lastChar := rune(lastStr[len(lastStr)-1])
				if lastChar != ' ' && lastChar != '\n' && lastChar != '{' && lastChar != ':' && lastChar != ';' {
					result.WriteRune(' ')
				}
			}
			continue
		}

		switch r {
		case '{':
			result.WriteString(" {\n")
			indentLevel++
			writeIndent()
		case '}':
			indentLevel--
			result.WriteRune('\n')
			writeIndent()
			result.WriteString("}\n\n")
		case ';':
			result.WriteString(";\n")
			writeIndent()
		case ':':
			result.WriteString(": ")
		default:
			result.WriteRune(r)
		}
	}

	return strings.TrimSpace(result.String()), nil
}

// beautifyHTML formats HTML code
func beautifyHTML(code, indent string) (string, error) {
	var result strings.Builder
	indentLevel := 0
	inTag := false
	tagName := ""
	isClosingTag := false

	// Self-closing tags
	selfClosing := map[string]bool{
		"br": true, "hr": true, "img": true, "input": true,
		"meta": true, "link": true, "area": true, "base": true,
		"col": true, "embed": true, "source": true, "track": true, "wbr": true,
	}

	writeIndent := func() {
		for i := 0; i < indentLevel; i++ {
			result.WriteString(indent)
		}
	}

	// Split by < to process tags
	code = strings.TrimSpace(code)
	parts := strings.Split(code, "<")

	for i, part := range parts {
		if part == "" {
			continue
		}

		// Find end of tag
		endIdx := strings.Index(part, ">")
		if endIdx == -1 {
			result.WriteString(part)
			continue
		}

		tag := part[:endIdx]
		content := part[endIdx+1:]

		// Determine tag type
		isClosingTag = strings.HasPrefix(tag, "/")
		isSelfClosing := strings.HasSuffix(tag, "/") || selfClosing[strings.ToLower(strings.Fields(strings.TrimPrefix(tag, "/"))[0])]

		if isClosingTag {
			indentLevel--
			if indentLevel < 0 {
				indentLevel = 0
			}
		}

		// Write tag
		if i > 0 {
			result.WriteRune('\n')
		}
		writeIndent()
		result.WriteRune('<')
		result.WriteString(tag)
		result.WriteRune('>')

		// Increment indent for opening tags (not self-closing)
		if !isClosingTag && !isSelfClosing {
			tagName = strings.ToLower(strings.Fields(tag)[0])
			inTag = true
			indentLevel++
		}

		// Write content
		content = strings.TrimSpace(content)
		if content != "" {
			// Check if content contains more tags
			if !strings.Contains(content, "<") {
				result.WriteString(content)
			} else {
				result.WriteString(content)
			}
		}

		_ = inTag
		_ = tagName
	}

	return strings.TrimSpace(result.String()), nil
}

// beautifySQLCode formats SQL code
func beautifySQLCode(code, indent string) (string, error) {
	// Keywords that start a new line
	newlineKeywords := []string{
		"SELECT", "FROM", "WHERE", "AND", "OR", "ORDER BY", "GROUP BY",
		"HAVING", "JOIN", "LEFT JOIN", "RIGHT JOIN", "INNER JOIN", "OUTER JOIN",
		"ON", "SET", "VALUES", "INSERT INTO", "UPDATE", "DELETE FROM", "CREATE TABLE",
		"ALTER TABLE", "DROP TABLE", "UNION", "UNION ALL", "LIMIT", "OFFSET",
	}

	result := code

	// Add newlines before keywords
	for _, kw := range newlineKeywords {
		re := regexp.MustCompile(`(?i)\s*\b(` + regexp.QuoteMeta(kw) + `)\b`)
		result = re.ReplaceAllStringFunc(result, func(m string) string {
			return "\n" + strings.ToUpper(strings.TrimSpace(m))
		})
	}

	// Indent subqueries and after keywords
	lines := strings.Split(result, "\n")
	var formattedLines []string
	indentLevel := 0

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Decrease indent for closing parentheses
		if strings.HasPrefix(line, ")") {
			indentLevel--
			if indentLevel < 0 {
				indentLevel = 0
			}
		}

		// Apply indent
		indentStr := ""
		for i := 0; i < indentLevel; i++ {
			indentStr += indent
		}
		formattedLines = append(formattedLines, indentStr+line)

		// Increase indent for opening parentheses
		if strings.HasSuffix(line, "(") {
			indentLevel++
		}
	}

	return strings.Join(formattedLines, "\n"), nil
}

// minifyCSS removes unnecessary whitespace from CSS
func minifyCSS(code string) string {
	// Remove comments
	re := regexp.MustCompile(`/\*[\s\S]*?\*/`)
	code = re.ReplaceAllString(code, "")

	// Remove newlines and extra spaces
	code = regexp.MustCompile(`\s+`).ReplaceAllString(code, " ")
	code = regexp.MustCompile(`\s*([{:;}])\s*`).ReplaceAllString(code, "$1")

	return strings.TrimSpace(code)
}

// minifyHTML removes unnecessary whitespace from HTML
func minifyHTML(code string) string {
	// Remove HTML comments
	re := regexp.MustCompile(`<!--[\s\S]*?-->`)
	code = re.ReplaceAllString(code, "")

	// Remove whitespace between tags
	code = regexp.MustCompile(`>\s+<`).ReplaceAllString(code, "><")

	// Normalize whitespace
	code = regexp.MustCompile(`\s+`).ReplaceAllString(code, " ")

	return strings.TrimSpace(code)
}

// minifyJavaScript removes unnecessary whitespace from JavaScript
func minifyJavaScript(code string) string {
	// Remove single-line comments
	re := regexp.MustCompile(`//.*$`)
	lines := strings.Split(code, "\n")
	for i, line := range lines {
		lines[i] = re.ReplaceAllString(line, "")
	}
	code = strings.Join(lines, "")

	// Remove multi-line comments
	re = regexp.MustCompile(`/\*[\s\S]*?\*/`)
	code = re.ReplaceAllString(code, "")

	// Remove extra whitespace (but be careful around operators)
	code = regexp.MustCompile(`\s+`).ReplaceAllString(code, " ")
	code = regexp.MustCompile(`\s*([{}\[\];,:])\s*`).ReplaceAllString(code, "$1")

	return strings.TrimSpace(code)
}

// minifySQL removes unnecessary whitespace from SQL
func minifySQL(code string) string {
	// Remove SQL comments
	re := regexp.MustCompile(`--.*$`)
	lines := strings.Split(code, "\n")
	for i, line := range lines {
		lines[i] = re.ReplaceAllString(line, "")
	}
	code = strings.Join(lines, " ")

	// Remove multi-line comments
	re = regexp.MustCompile(`/\*[\s\S]*?\*/`)
	code = re.ReplaceAllString(code, "")

	// Normalize whitespace
	code = regexp.MustCompile(`\s+`).ReplaceAllString(code, " ")

	return strings.TrimSpace(code)
}

// escapeHTMLContent escapes HTML special characters for display
func escapeHTMLContent(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	s = strings.ReplaceAll(s, "'", "&#39;")
	return s
}
