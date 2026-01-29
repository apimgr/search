package direct

import (
	"context"
	"fmt"
	"math/rand"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// CaseHandler handles case:{text} queries
type CaseHandler struct{}

// NewCaseHandler creates a new case converter handler
func NewCaseHandler() *CaseHandler {
	return &CaseHandler{}
}

func (h *CaseHandler) Type() AnswerType {
	return AnswerTypeCase
}

func (h *CaseHandler) Handle(ctx context.Context, term string) (*Answer, error) {
	term = strings.TrimSpace(term)
	if term == "" {
		return nil, fmt.Errorf("text required")
	}

	// Check if specific case is requested
	text := term
	targetCase := ""

	prefixes := []string{"upper ", "lower ", "title ", "sentence ", "camel ", "pascal ", "snake ", "kebab ", "screaming "}
	for _, prefix := range prefixes {
		if strings.HasPrefix(strings.ToLower(term), prefix) {
			targetCase = strings.TrimSuffix(prefix, " ")
			text = term[len(prefix):]
			break
		}
	}

	// Generate all conversions
	conversions := map[string]string{
		"UPPERCASE":           strings.ToUpper(text),
		"lowercase":           strings.ToLower(text),
		"Title Case":          toTitleCase(text),
		"Sentence case":       toSentenceCase(text),
		"camelCase":           toCamelCase(text),
		"PascalCase":          toPascalCase(text),
		"snake_case":          toSnakeCase(text),
		"kebab-case":          toKebabCase(text),
		"SCREAMING_SNAKE":     toScreamingSnake(text),
		"dot.case":            toDotCase(text),
	}

	data := map[string]interface{}{
		"input":       text,
		"conversions": conversions,
		"target":      targetCase,
	}

	return &Answer{
		Type:        AnswerTypeCase,
		Term:        term,
		Title:       "Case Converter",
		Description: "Text case conversions",
		Content:     formatCaseContent(text, conversions, targetCase),
		Source:      "Case Converter",
		Data:        data,
	}, nil
}

func toTitleCase(s string) string {
	return strings.Title(strings.ToLower(s))
}

func toSentenceCase(s string) string {
	if len(s) == 0 {
		return s
	}
	lower := strings.ToLower(s)
	return strings.ToUpper(string(lower[0])) + lower[1:]
}

func toCamelCase(s string) string {
	words := splitWords(s)
	if len(words) == 0 {
		return ""
	}

	var result strings.Builder
	result.WriteString(strings.ToLower(words[0]))
	for _, word := range words[1:] {
		if word != "" {
			result.WriteString(strings.ToUpper(string(word[0])))
			result.WriteString(strings.ToLower(word[1:]))
		}
	}
	return result.String()
}

func toPascalCase(s string) string {
	words := splitWords(s)
	var result strings.Builder
	for _, word := range words {
		if word != "" {
			result.WriteString(strings.ToUpper(string(word[0])))
			result.WriteString(strings.ToLower(word[1:]))
		}
	}
	return result.String()
}

func toSnakeCase(s string) string {
	words := splitWords(s)
	for i, word := range words {
		words[i] = strings.ToLower(word)
	}
	return strings.Join(words, "_")
}

func toKebabCase(s string) string {
	words := splitWords(s)
	for i, word := range words {
		words[i] = strings.ToLower(word)
	}
	return strings.Join(words, "-")
}

func toScreamingSnake(s string) string {
	words := splitWords(s)
	for i, word := range words {
		words[i] = strings.ToUpper(word)
	}
	return strings.Join(words, "_")
}

func toDotCase(s string) string {
	words := splitWords(s)
	for i, word := range words {
		words[i] = strings.ToLower(word)
	}
	return strings.Join(words, ".")
}

func splitWords(s string) []string {
	// Replace common separators with space
	s = strings.ReplaceAll(s, "-", " ")
	s = strings.ReplaceAll(s, "_", " ")
	s = strings.ReplaceAll(s, ".", " ")

	// Insert space before capitals in camelCase
	var result strings.Builder
	for i, r := range s {
		if i > 0 && unicode.IsUpper(r) && !unicode.IsUpper(rune(s[i-1])) {
			result.WriteRune(' ')
		}
		result.WriteRune(r)
	}

	// Split and filter empty
	parts := strings.Fields(result.String())
	words := make([]string, 0, len(parts))
	for _, p := range parts {
		if p != "" {
			words = append(words, p)
		}
	}
	return words
}

func formatCaseContent(text string, conversions map[string]string, target string) string {
	var html strings.Builder
	html.WriteString("<div class=\"case-content\">")
	html.WriteString("<h1>Case Converter</h1>")

	html.WriteString("<h2>Input</h2>")
	html.WriteString(fmt.Sprintf("<pre><code>%s</code></pre>", escapeHTML(text)))

	html.WriteString("<h2>Conversions</h2>")
	html.WriteString("<table class=\"case-table\">")
	html.WriteString("<tbody>")

	order := []string{"UPPERCASE", "lowercase", "Title Case", "Sentence case", "camelCase", "PascalCase", "snake_case", "kebab-case", "SCREAMING_SNAKE", "dot.case"}
	for _, name := range order {
		val := conversions[name]
		class := ""
		if strings.EqualFold(target, strings.ReplaceAll(name, " ", "")) ||
		   strings.EqualFold(target, strings.ReplaceAll(name, "_", "")) {
			class = " class=\"highlighted\""
		}
		html.WriteString(fmt.Sprintf("<tr%s><td><strong>%s</strong></td><td><code>%s</code></td><td><button class=\"copy-btn\" onclick=\"copyText('%s')\">Copy</button></td></tr>",
			class, name, escapeHTML(val), escapeHTML(val)))
	}

	html.WriteString("</tbody></table>")
	html.WriteString("</div>")
	return html.String()
}

// SlugHandler handles slug:{text} queries
type SlugHandler struct{}

// NewSlugHandler creates a new slug generator handler
func NewSlugHandler() *SlugHandler {
	return &SlugHandler{}
}

func (h *SlugHandler) Type() AnswerType {
	return AnswerTypeSlug
}

func (h *SlugHandler) Handle(ctx context.Context, term string) (*Answer, error) {
	term = strings.TrimSpace(term)
	if term == "" {
		return nil, fmt.Errorf("text required")
	}

	// Generate slug variations
	slugs := map[string]string{
		"Standard (kebab)": generateSlug(term, "-"),
		"Underscore":       generateSlug(term, "_"),
		"No separator":     generateSlug(term, ""),
	}

	data := map[string]interface{}{
		"input": term,
		"slugs": slugs,
	}

	return &Answer{
		Type:        AnswerTypeSlug,
		Term:        term,
		Title:       "URL Slug Generator",
		Description: "Generated URL-safe slug",
		Content:     formatSlugContent(term, slugs),
		Source:      "Slug Generator",
		Data:        data,
	}, nil
}

func generateSlug(s, separator string) string {
	// Convert to lowercase
	s = strings.ToLower(s)

	// Transliterate common characters
	replacements := map[rune]string{
		'ä': "ae", 'ö': "oe", 'ü': "ue", 'ß': "ss",
		'à': "a", 'á': "a", 'â': "a", 'ã': "a", 'å': "a",
		'è': "e", 'é': "e", 'ê': "e", 'ë': "e",
		'ì': "i", 'í': "i", 'î': "i", 'ï': "i",
		'ò': "o", 'ó': "o", 'ô': "o", 'õ': "o", 'ø': "o",
		'ù': "u", 'ú': "u", 'û': "u",
		'ñ': "n", 'ç': "c",
	}

	var result strings.Builder
	for _, r := range s {
		if rep, ok := replacements[r]; ok {
			result.WriteString(rep)
		} else if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			result.WriteRune(r)
		} else if r == ' ' || r == '-' || r == '_' {
			result.WriteString(" ")
		}
	}

	// Split into words
	words := strings.Fields(result.String())

	if separator == "" {
		return strings.Join(words, "")
	}
	return strings.Join(words, separator)
}

func formatSlugContent(text string, slugs map[string]string) string {
	var html strings.Builder
	html.WriteString("<div class=\"slug-content\">")
	html.WriteString("<h1>URL Slug Generator</h1>")

	html.WriteString("<h2>Input</h2>")
	html.WriteString(fmt.Sprintf("<pre><code>%s</code></pre>", escapeHTML(text)))

	html.WriteString("<h2>Generated Slugs</h2>")
	html.WriteString("<table class=\"slug-table\">")
	html.WriteString("<tbody>")

	for name, slug := range slugs {
		html.WriteString(fmt.Sprintf("<tr><td><strong>%s</strong></td><td><code>%s</code></td><td><button class=\"copy-btn\" onclick=\"copyText('%s')\">Copy</button></td></tr>",
			name, escapeHTML(slug), escapeHTML(slug)))
	}

	html.WriteString("</tbody></table>")
	html.WriteString("</div>")
	return html.String()
}

// LoremHandler handles lorem:{count} queries
type LoremHandler struct{}

// NewLoremHandler creates a new lorem ipsum generator handler
func NewLoremHandler() *LoremHandler {
	return &LoremHandler{}
}

func (h *LoremHandler) Type() AnswerType {
	return AnswerTypeLorem
}

var loremWords = []string{
	"lorem", "ipsum", "dolor", "sit", "amet", "consectetur", "adipiscing", "elit",
	"sed", "do", "eiusmod", "tempor", "incididunt", "ut", "labore", "et", "dolore",
	"magna", "aliqua", "enim", "ad", "minim", "veniam", "quis", "nostrud",
	"exercitation", "ullamco", "laboris", "nisi", "aliquip", "ex", "ea", "commodo",
	"consequat", "duis", "aute", "irure", "in", "reprehenderit", "voluptate",
	"velit", "esse", "cillum", "fugiat", "nulla", "pariatur", "excepteur", "sint",
	"occaecat", "cupidatat", "non", "proident", "sunt", "culpa", "qui", "officia",
	"deserunt", "mollit", "anim", "id", "est", "laborum",
}

func (h *LoremHandler) Handle(ctx context.Context, term string) (*Answer, error) {
	term = strings.TrimSpace(term)
	if term == "" {
		term = "3" // Default to 3 paragraphs
	}

	// Parse count and unit
	count := 3
	unit := "paragraphs"

	parts := strings.Fields(strings.ToLower(term))
	if len(parts) >= 1 {
		if n, err := strconv.Atoi(parts[0]); err == nil && n > 0 {
			count = n
			if count > 100 {
				count = 100 // Limit
			}
		}
	}
	if len(parts) >= 2 {
		switch parts[1] {
		case "word", "words":
			unit = "words"
		case "sentence", "sentences":
			unit = "sentences"
		default:
			unit = "paragraphs"
		}
	}

	// Generate text
	var text string
	switch unit {
	case "words":
		text = generateLoremWords(count)
	case "sentences":
		text = generateLoremSentences(count)
	default:
		text = generateLoremParagraphs(count)
	}

	// Character and word count
	wordCount := len(strings.Fields(text))
	charCount := len(text)

	data := map[string]interface{}{
		"text":      text,
		"count":     count,
		"unit":      unit,
		"wordCount": wordCount,
		"charCount": charCount,
	}

	return &Answer{
		Type:        AnswerTypeLorem,
		Term:        term,
		Title:       "Lorem Ipsum Generator",
		Description: fmt.Sprintf("%d %s", count, unit),
		Content:     formatLoremContent(text, wordCount, charCount),
		Source:      "Lorem Ipsum Generator",
		Data:        data,
	}, nil
}

func generateLoremWords(count int) string {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	words := make([]string, count)
	for i := 0; i < count; i++ {
		words[i] = loremWords[r.Intn(len(loremWords))]
	}
	result := strings.Join(words, " ")
	return strings.ToUpper(string(result[0])) + result[1:] + "."
}

func generateLoremSentences(count int) string {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	sentences := make([]string, count)
	for i := 0; i < count; i++ {
		wordCount := 5 + r.Intn(10) // 5-14 words per sentence
		words := make([]string, wordCount)
		for j := 0; j < wordCount; j++ {
			words[j] = loremWords[r.Intn(len(loremWords))]
		}
		sentence := strings.Join(words, " ")
		sentences[i] = strings.ToUpper(string(sentence[0])) + sentence[1:] + "."
	}
	return strings.Join(sentences, " ")
}

func generateLoremParagraphs(count int) string {
	paragraphs := make([]string, count)
	for i := 0; i < count; i++ {
		sentenceCount := 3 + rand.Intn(3) // 3-5 sentences per paragraph
		paragraphs[i] = generateLoremSentences(sentenceCount)
	}
	return strings.Join(paragraphs, "\n\n")
}

func formatLoremContent(text string, wordCount, charCount int) string {
	var html strings.Builder
	html.WriteString("<div class=\"lorem-content\">")
	html.WriteString("<h1>Lorem Ipsum Generator</h1>")

	html.WriteString(fmt.Sprintf("<p>%d words, %d characters</p>", wordCount, charCount))

	html.WriteString("<div class=\"lorem-text\">")
	// Convert paragraphs to HTML
	paragraphs := strings.Split(text, "\n\n")
	for _, p := range paragraphs {
		html.WriteString(fmt.Sprintf("<p>%s</p>", escapeHTML(p)))
	}
	html.WriteString("</div>")

	html.WriteString("<button class=\"copy-btn\" onclick=\"copyText(document.querySelector('.lorem-text').innerText)\">Copy Text</button>")

	html.WriteString("</div>")
	return html.String()
}

// WordHandler handles word:{text} queries (text statistics)
type WordHandler struct{}

// NewWordHandler creates a new word/text statistics handler
func NewWordHandler() *WordHandler {
	return &WordHandler{}
}

func (h *WordHandler) Type() AnswerType {
	return AnswerTypeWord
}

func (h *WordHandler) Handle(ctx context.Context, term string) (*Answer, error) {
	term = strings.TrimSpace(term)
	if term == "" {
		return nil, fmt.Errorf("text required")
	}

	// Calculate statistics
	charCount := len(term)
	charNoSpaces := len(strings.ReplaceAll(term, " ", ""))
	words := strings.Fields(term)
	wordCount := len(words)

	// Sentence count (rough estimate)
	sentenceCount := 0
	for _, r := range term {
		if r == '.' || r == '!' || r == '?' {
			sentenceCount++
		}
	}
	if sentenceCount == 0 && wordCount > 0 {
		sentenceCount = 1
	}

	// Paragraph count
	paragraphCount := len(strings.Split(term, "\n\n"))
	if paragraphCount == 0 {
		paragraphCount = 1
	}

	// Line count
	lineCount := len(strings.Split(term, "\n"))

	// Average word length
	totalChars := 0
	for _, w := range words {
		totalChars += len(w)
	}
	avgWordLen := 0.0
	if wordCount > 0 {
		avgWordLen = float64(totalChars) / float64(wordCount)
	}

	// Reading time (average 200 words per minute)
	readingMins := float64(wordCount) / 200.0
	readingTime := fmt.Sprintf("%.1f min", readingMins)
	if readingMins < 1 {
		readingTime = fmt.Sprintf("%d sec", int(readingMins*60))
	}

	// Speaking time (average 150 words per minute)
	speakingMins := float64(wordCount) / 150.0
	speakingTime := fmt.Sprintf("%.1f min", speakingMins)
	if speakingMins < 1 {
		speakingTime = fmt.Sprintf("%d sec", int(speakingMins*60))
	}

	// Word frequency (top 10)
	wordFreq := make(map[string]int)
	for _, w := range words {
		w = strings.ToLower(w)
		w = strings.Trim(w, ".,!?;:\"'()[]{}")
		if len(w) > 2 {
			wordFreq[w]++
		}
	}

	data := map[string]interface{}{
		"charCount":      charCount,
		"charNoSpaces":   charNoSpaces,
		"wordCount":      wordCount,
		"sentenceCount":  sentenceCount,
		"paragraphCount": paragraphCount,
		"lineCount":      lineCount,
		"avgWordLen":     avgWordLen,
		"readingTime":    readingTime,
		"speakingTime":   speakingTime,
	}

	return &Answer{
		Type:        AnswerTypeWord,
		Term:        truncateString(term, 50),
		Title:       "Text Statistics",
		Description: fmt.Sprintf("%d words, %d characters", wordCount, charCount),
		Content:     formatWordContent(term, data, wordFreq),
		Source:      "Text Analyzer",
		Data:        data,
	}, nil
}

func formatWordContent(text string, stats map[string]interface{}, wordFreq map[string]int) string {
	var html strings.Builder
	html.WriteString("<div class=\"word-content\">")
	html.WriteString("<h1>Text Statistics</h1>")

	html.WriteString("<table class=\"stats-table\">")
	html.WriteString("<tbody>")
	html.WriteString(fmt.Sprintf("<tr><td>Characters</td><td>%d</td></tr>", stats["charCount"].(int)))
	html.WriteString(fmt.Sprintf("<tr><td>Characters (no spaces)</td><td>%d</td></tr>", stats["charNoSpaces"].(int)))
	html.WriteString(fmt.Sprintf("<tr><td>Words</td><td>%d</td></tr>", stats["wordCount"].(int)))
	html.WriteString(fmt.Sprintf("<tr><td>Sentences</td><td>%d</td></tr>", stats["sentenceCount"].(int)))
	html.WriteString(fmt.Sprintf("<tr><td>Paragraphs</td><td>%d</td></tr>", stats["paragraphCount"].(int)))
	html.WriteString(fmt.Sprintf("<tr><td>Lines</td><td>%d</td></tr>", stats["lineCount"].(int)))
	html.WriteString(fmt.Sprintf("<tr><td>Average Word Length</td><td>%.1f characters</td></tr>", stats["avgWordLen"].(float64)))
	html.WriteString(fmt.Sprintf("<tr><td>Reading Time</td><td>%s</td></tr>", stats["readingTime"].(string)))
	html.WriteString(fmt.Sprintf("<tr><td>Speaking Time</td><td>%s</td></tr>", stats["speakingTime"].(string)))
	html.WriteString("</tbody></table>")

	// Top words
	if len(wordFreq) > 0 {
		html.WriteString("<h2>Most Frequent Words</h2>")
		html.WriteString("<ul class=\"word-freq\">")

		// Get top 10
		type wordCount struct {
			word  string
			count int
		}
		var sorted []wordCount
		for w, c := range wordFreq {
			sorted = append(sorted, wordCount{w, c})
		}
		// Simple sort
		for i := 0; i < len(sorted)-1; i++ {
			for j := i + 1; j < len(sorted); j++ {
				if sorted[j].count > sorted[i].count {
					sorted[i], sorted[j] = sorted[j], sorted[i]
				}
			}
		}

		shown := 0
		for _, wc := range sorted {
			if shown >= 10 {
				break
			}
			html.WriteString(fmt.Sprintf("<li><strong>%s</strong>: %d</li>", escapeHTML(wc.word), wc.count))
			shown++
		}
		html.WriteString("</ul>")
	}

	html.WriteString("</div>")
	return html.String()
}

// BeautifyHandler handles beautify:{code} queries
type BeautifyHandler struct{}

// NewBeautifyHandler creates a new code beautifier handler
func NewBeautifyHandler() *BeautifyHandler {
	return &BeautifyHandler{}
}

func (h *BeautifyHandler) Type() AnswerType {
	return AnswerTypeBeautify
}

func (h *BeautifyHandler) Handle(ctx context.Context, term string) (*Answer, error) {
	term = strings.TrimSpace(term)
	if term == "" {
		return nil, fmt.Errorf("code required")
	}

	// Detect language from prefix
	lang := ""
	code := term

	prefixes := []string{"js ", "javascript ", "css ", "html ", "sql ", "json ", "xml "}
	for _, prefix := range prefixes {
		if strings.HasPrefix(strings.ToLower(term), prefix) {
			lang = strings.TrimSuffix(prefix, " ")
			code = term[len(prefix):]
			break
		}
	}

	// Auto-detect language
	if lang == "" {
		lang = detectLanguage(code)
	}

	// Beautify
	beautified := beautifyCode(code, lang)

	data := map[string]interface{}{
		"input":      code,
		"output":     beautified,
		"language":   lang,
	}

	return &Answer{
		Type:        AnswerTypeBeautify,
		Term:        truncateString(term, 50),
		Title:       fmt.Sprintf("Code Beautifier (%s)", lang),
		Description: "Formatted code",
		Content:     formatBeautifyContent(code, beautified, lang),
		Source:      "Code Beautifier",
		Data:        data,
	}, nil
}

func detectLanguage(code string) string {
	code = strings.TrimSpace(code)

	if strings.HasPrefix(code, "{") || strings.HasPrefix(code, "[") {
		return "json"
	}
	if strings.HasPrefix(code, "<") {
		if strings.Contains(strings.ToLower(code), "<!doctype") || strings.Contains(strings.ToLower(code), "<html") {
			return "html"
		}
		return "xml"
	}
	if strings.Contains(code, "function") || strings.Contains(code, "const ") || strings.Contains(code, "var ") || strings.Contains(code, "=>") {
		return "javascript"
	}
	if strings.Contains(code, "{") && (strings.Contains(code, ":") || strings.Contains(code, ";")) {
		return "css"
	}
	if strings.Contains(strings.ToUpper(code), "SELECT") || strings.Contains(strings.ToUpper(code), "INSERT") {
		return "sql"
	}

	return "unknown"
}

func beautifyCode(code, lang string) string {
	switch lang {
	case "json":
		return beautifyJSON(code)
	case "html", "xml":
		return beautifyHTML(code)
	case "css":
		return beautifyCSS(code)
	case "sql":
		return beautifySQL(code)
	default:
		return code
	}
}

func beautifyJSON(code string) string {
	// Simple JSON beautification
	var result strings.Builder
	indent := 0
	inString := false
	escaped := false

	for _, r := range code {
		if escaped {
			result.WriteRune(r)
			escaped = false
			continue
		}

		if r == '\\' && inString {
			result.WriteRune(r)
			escaped = true
			continue
		}

		if r == '"' {
			inString = !inString
			result.WriteRune(r)
			continue
		}

		if inString {
			result.WriteRune(r)
			continue
		}

		switch r {
		case '{', '[':
			result.WriteRune(r)
			indent++
			result.WriteRune('\n')
			result.WriteString(strings.Repeat("  ", indent))
		case '}', ']':
			indent--
			result.WriteRune('\n')
			result.WriteString(strings.Repeat("  ", indent))
			result.WriteRune(r)
		case ',':
			result.WriteRune(r)
			result.WriteRune('\n')
			result.WriteString(strings.Repeat("  ", indent))
		case ':':
			result.WriteRune(r)
			result.WriteRune(' ')
		case ' ', '\n', '\t':
			// Skip whitespace
		default:
			result.WriteRune(r)
		}
	}

	return result.String()
}

func beautifyHTML(code string) string {
	// Simple HTML beautification
	// Add newlines after closing tags
	code = regexp.MustCompile(`>\s*<`).ReplaceAllString(code, ">\n<")

	// Indent
	var result strings.Builder
	indent := 0
	lines := strings.Split(code, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Check if closing tag
		if strings.HasPrefix(line, "</") {
			indent--
		}

		result.WriteString(strings.Repeat("  ", indent))
		result.WriteString(line)
		result.WriteRune('\n')

		// Check if opening tag (not self-closing)
		if strings.HasPrefix(line, "<") && !strings.HasPrefix(line, "</") && !strings.HasSuffix(line, "/>") && !strings.Contains(line, "</") {
			indent++
		}
	}

	return strings.TrimSpace(result.String())
}

func beautifyCSS(code string) string {
	// Simple CSS beautification
	code = strings.ReplaceAll(code, "{", " {\n  ")
	code = strings.ReplaceAll(code, "}", "\n}\n\n")
	code = strings.ReplaceAll(code, ";", ";\n  ")
	code = regexp.MustCompile(`\n  \n`).ReplaceAllString(code, "\n")
	return strings.TrimSpace(code)
}

func beautifySQL(code string) string {
	// Simple SQL beautification
	keywords := []string{"SELECT", "FROM", "WHERE", "AND", "OR", "ORDER BY", "GROUP BY", "HAVING", "JOIN", "LEFT JOIN", "RIGHT JOIN", "INNER JOIN", "ON", "INSERT", "INTO", "VALUES", "UPDATE", "SET", "DELETE"}

	for _, kw := range keywords {
		re := regexp.MustCompile(`(?i)\b` + kw + `\b`)
		code = re.ReplaceAllStringFunc(code, func(m string) string {
			return "\n" + strings.ToUpper(m)
		})
	}

	return strings.TrimSpace(code)
}

func formatBeautifyContent(input, output, lang string) string {
	var html strings.Builder
	html.WriteString("<div class=\"beautify-content\">")
	html.WriteString(fmt.Sprintf("<h1>Code Beautifier - %s</h1>", escapeHTML(lang)))

	html.WriteString("<h2>Formatted Code</h2>")
	html.WriteString(fmt.Sprintf("<pre class=\"code %s\"><code>%s</code></pre>", escapeHTML(lang), escapeHTML(output)))
	html.WriteString("<button class=\"copy-btn\" onclick=\"copyCode(this)\">Copy</button>")

	html.WriteString("</div>")
	return html.String()
}

// DiffHandler handles diff:{texts} queries
type DiffHandler struct{}

// NewDiffHandler creates a new diff handler
func NewDiffHandler() *DiffHandler {
	return &DiffHandler{}
}

func (h *DiffHandler) Type() AnswerType {
	return AnswerTypeDiff
}

func (h *DiffHandler) Handle(ctx context.Context, term string) (*Answer, error) {
	term = strings.TrimSpace(term)
	if term == "" {
		return nil, fmt.Errorf("two texts required (separated by |||)")
	}

	// Split by ||| separator
	parts := strings.Split(term, "|||")
	if len(parts) != 2 {
		return &Answer{
			Type:        AnswerTypeDiff,
			Term:        truncateString(term, 50),
			Title:       "Text Diff",
			Description: "Invalid format",
			Content:     "<p class=\"error\">Please provide two texts separated by <code>|||</code></p><p>Example: <code>diff:hello world|||hello there</code></p>",
			Error:       "invalid_format",
		}, nil
	}

	text1 := strings.TrimSpace(parts[0])
	text2 := strings.TrimSpace(parts[1])

	// Simple diff
	diff := simpleDiff(text1, text2)

	data := map[string]interface{}{
		"text1": text1,
		"text2": text2,
		"diff":  diff,
	}

	return &Answer{
		Type:        AnswerTypeDiff,
		Term:        truncateString(term, 50),
		Title:       "Text Diff",
		Description: "Comparison of two texts",
		Content:     formatDiffContent(text1, text2, diff),
		Source:      "Diff Tool",
		Data:        data,
	}, nil
}

type diffLine struct {
	Type string // "same", "add", "remove"
	Text string
}

func simpleDiff(text1, text2 string) []diffLine {
	lines1 := strings.Split(text1, "\n")
	lines2 := strings.Split(text2, "\n")

	var result []diffLine

	// Simple line-by-line comparison
	max := len(lines1)
	if len(lines2) > max {
		max = len(lines2)
	}

	for i := 0; i < max; i++ {
		var l1, l2 string
		if i < len(lines1) {
			l1 = lines1[i]
		}
		if i < len(lines2) {
			l2 = lines2[i]
		}

		if l1 == l2 {
			result = append(result, diffLine{"same", l1})
		} else {
			if l1 != "" {
				result = append(result, diffLine{"remove", l1})
			}
			if l2 != "" {
				result = append(result, diffLine{"add", l2})
			}
		}
	}

	return result
}

func formatDiffContent(text1, text2 string, diff []diffLine) string {
	var html strings.Builder
	html.WriteString("<div class=\"diff-content\">")
	html.WriteString("<h1>Text Diff</h1>")

	// Statistics
	added := 0
	removed := 0
	for _, d := range diff {
		if d.Type == "add" {
			added++
		} else if d.Type == "remove" {
			removed++
		}
	}
	html.WriteString(fmt.Sprintf("<p>+%d additions, -%d removals</p>", added, removed))

	// Side by side
	html.WriteString("<div class=\"diff-view\">")
	html.WriteString("<table class=\"diff-table\">")
	html.WriteString("<thead><tr><th>Original</th><th>Modified</th></tr></thead>")
	html.WriteString("<tbody>")

	for _, d := range diff {
		switch d.Type {
		case "same":
			html.WriteString(fmt.Sprintf("<tr><td>%s</td><td>%s</td></tr>", escapeHTML(d.Text), escapeHTML(d.Text)))
		case "remove":
			html.WriteString(fmt.Sprintf("<tr class=\"removed\"><td class=\"removed\">%s</td><td></td></tr>", escapeHTML(d.Text)))
		case "add":
			html.WriteString(fmt.Sprintf("<tr class=\"added\"><td></td><td class=\"added\">%s</td></tr>", escapeHTML(d.Text)))
		}
	}

	html.WriteString("</tbody></table>")
	html.WriteString("</div>")

	html.WriteString("</div>")
	return html.String()
}
