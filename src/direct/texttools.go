package direct

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/evanw/esbuild/pkg/api"
	"github.com/tdewolff/minify/v2"
	"github.com/tdewolff/minify/v2/css"
	"github.com/tdewolff/minify/v2/html"
	"github.com/tdewolff/minify/v2/js"
	minifyjson "github.com/tdewolff/minify/v2/json"
	"github.com/tdewolff/minify/v2/xml"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"github.com/apimgr/search/src/common/i18n"
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

func (h *CaseHandler) HandleDirectQuery(ctx context.Context, term string) (*Answer, error) {
	uiLang := LangFromContext(ctx)
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
		"UPPERCASE":       strings.ToUpper(text),
		"lowercase":       strings.ToLower(text),
		"Title Case":      toTitleCase(text),
		"Sentence case":   toSentenceCase(text),
		"camelCase":       toCamelCase(text),
		"PascalCase":      toPascalCase(text),
		"snake_case":      toSnakeCase(text),
		"kebab-case":      toKebabCase(text),
		"SCREAMING_SNAKE": toScreamingSnake(text),
		"dot.case":        toDotCase(text),
	}

	data := map[string]interface{}{
		"input":       text,
		"conversions": conversions,
		"target":      targetCase,
	}

	return &Answer{
		Type:        AnswerTypeCase,
		Term:        term,
		Title:       i18n.T(uiLang, "direct.case_converter_title"),
		Description: i18n.T(uiLang, "direct.text_case_conversions"),
		Content:     formatCaseContent(text, conversions, targetCase),
		Source:      "Case Converter",
		Data:        data,
	}, nil
}

func toTitleCase(s string) string {
	return cases.Title(language.Und, cases.NoLower).String(strings.ToLower(s))
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
	fmt.Fprintf(&html, "<pre><code>%s</code></pre>", escapeHTML(text))

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
		fmt.Fprintf(&html, "<tr%s><td><strong>%s</strong></td><td><code>%s</code></td><td><button type=\"button\" class=\"copy-btn\" data-copy=\"%s\">Copy</button></td></tr>",
			class, name, escapeHTML(val), escapeHTML(val))
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

func (h *SlugHandler) HandleDirectQuery(ctx context.Context, term string) (*Answer, error) {
	uiLang := LangFromContext(ctx)
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
		Title:       i18n.T(uiLang, "direct.url_slug_generator_title"),
		Description: i18n.T(uiLang, "direct.generated_url_safe_slug"),
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
	fmt.Fprintf(&html, "<pre><code>%s</code></pre>", escapeHTML(text))

	html.WriteString("<h2>Generated Slugs</h2>")
	html.WriteString("<table class=\"slug-table\">")
	html.WriteString("<tbody>")

	for name, slug := range slugs {
		fmt.Fprintf(&html, "<tr><td><strong>%s</strong></td><td><code>%s</code></td><td><button type=\"button\" class=\"copy-btn\" data-copy=\"%s\">Copy</button></td></tr>",
			name, escapeHTML(slug), escapeHTML(slug))
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

func (h *LoremHandler) HandleDirectQuery(ctx context.Context, term string) (*Answer, error) {
	uiLang := LangFromContext(ctx)
	term = strings.TrimSpace(term)
	if term == "" {
		// Default to 3 paragraphs
		term = "3"
	}

	// Parse count and unit
	count := 3
	unit := "paragraphs"

	parts := strings.Fields(strings.ToLower(term))
	if len(parts) >= 1 {
		if n, err := strconv.Atoi(parts[0]); err == nil && n > 0 {
			count = n
			if count > 100 {
				// Limit
				count = 100
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
		Title:       i18n.T(uiLang, "direct.lorem_ipsum_generator_title"),
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
		// 5-14 words per sentence
		wordCount := 5 + r.Intn(10)
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
		// 3-5 sentences per paragraph
		sentenceCount := 3 + rand.Intn(3)
		paragraphs[i] = generateLoremSentences(sentenceCount)
	}
	return strings.Join(paragraphs, "\n\n")
}

func formatLoremContent(text string, wordCount, charCount int) string {
	var html strings.Builder
	html.WriteString("<div class=\"lorem-content\">")
	html.WriteString("<h1>Lorem Ipsum Generator</h1>")

	fmt.Fprintf(&html, "<p>%d words, %d characters</p>", wordCount, charCount)

	html.WriteString("<div class=\"lorem-text\">")
	// Convert paragraphs to HTML
	paragraphs := strings.Split(text, "\n\n")
	for _, p := range paragraphs {
		fmt.Fprintf(&html, "<p>%s</p>", escapeHTML(p))
	}
	html.WriteString("</div>")

	fmt.Fprintf(&html, "<button type=\"button\" class=\"copy-btn\" data-copy=\"%s\">Copy Text</button>", escapeHTML(text))

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

func (h *WordHandler) HandleDirectQuery(ctx context.Context, term string) (*Answer, error) {
	uiLang := LangFromContext(ctx)
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
		Title:       i18n.T(uiLang, "direct.text_statistics_title"),
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
	fmt.Fprintf(&html, "<tr><td>Characters</td><td>%d</td></tr>", stats["charCount"].(int))
	fmt.Fprintf(&html, "<tr><td>Characters (no spaces)</td><td>%d</td></tr>", stats["charNoSpaces"].(int))
	fmt.Fprintf(&html, "<tr><td>Words</td><td>%d</td></tr>", stats["wordCount"].(int))
	fmt.Fprintf(&html, "<tr><td>Sentences</td><td>%d</td></tr>", stats["sentenceCount"].(int))
	fmt.Fprintf(&html, "<tr><td>Paragraphs</td><td>%d</td></tr>", stats["paragraphCount"].(int))
	fmt.Fprintf(&html, "<tr><td>Lines</td><td>%d</td></tr>", stats["lineCount"].(int))
	fmt.Fprintf(&html, "<tr><td>Average Word Length</td><td>%.1f characters</td></tr>", stats["avgWordLen"].(float64))
	fmt.Fprintf(&html, "<tr><td>Reading Time</td><td>%s</td></tr>", stats["readingTime"].(string))
	fmt.Fprintf(&html, "<tr><td>Speaking Time</td><td>%s</td></tr>", stats["speakingTime"].(string))
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
			fmt.Fprintf(&html, "<li><strong>%s</strong>: %d</li>", escapeHTML(wc.word), wc.count)
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

func (h *BeautifyHandler) HandleDirectQuery(ctx context.Context, term string) (*Answer, error) {
	uiLang := LangFromContext(ctx)
	term = strings.TrimSpace(term)
	if term == "" {
		return nil, fmt.Errorf("code required")
	}

	// Detect language from prefix
	lang := ""
	code := term

	prefixes := []string{"js ", "javascript ", "typescript ", "ts ", "css ", "scss ", "less ", "html ", "sql ", "json ", "xml ", "php ", "python "}
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
	indent, minify, style, remaining, optionErr := parseBeautifyOptions(code)
	if optionErr != nil {
		return nil, optionErr
	}
	code = remaining
	if strings.TrimSpace(code) == "" {
		return nil, fmt.Errorf("code required")
	}
	style, styleErr := normalizeBeautifyStyle(lang, style)
	if styleErr != nil {
		return nil, styleErr
	}
	if lang == "json" && !json.Valid([]byte(code)) {
		return nil, fmt.Errorf("cannot format invalid JSON")
	}

	formatCode := code
	var restorePHPHeredocs func(string) string
	if lang == "php" {
		formatCode, restorePHPHeredocs = protectPHPHeredocs(code)
	}

	// Beautify
	beautified := beautifyCode(formatCode, lang)
	beautified, styleErr = applyBeautifyStyle(beautified, formatCode, lang, style)
	if styleErr != nil {
		return nil, styleErr
	}
	if indent != "" {
		beautified = reindentFormatted(beautified, lang, indent)
	}
	if restorePHPHeredocs != nil {
		beautified = restorePHPHeredocs(beautified)
	}
	minified := ""
	if minify {
		compact, err := minifyCode(formatCode, lang)
		if err != nil {
			return nil, err
		}
		if restorePHPHeredocs != nil {
			compact = restorePHPHeredocs(compact)
		}
		minified = compact
	}

	data := map[string]interface{}{
		"input":        code,
		"output":       beautified,
		"language":     lang,
		"input_lines":  countLines(code),
		"output_lines": countLines(beautified),
		"indent":       indent,
		"style":        style,
		"minified":     minified,
	}

	return &Answer{
		Type:        AnswerTypeBeautify,
		Term:        truncateString(term, 50),
		Title:       fmt.Sprintf("Code Beautifier (%s)", lang),
		Description: i18n.T(uiLang, "direct.formatted_code"),
		Content:     formatBeautifyContent(code, beautified, lang, indent, style, minified),
		Source:      "Code Beautifier",
		Data:        data,
	}, nil
}

func minifyCode(code, language string) (string, error) {
	switch language {
	case "sql":
		return minifySQLSource(code)
	case "scss", "less":
		return minifyStylesheetSource(code)
	case "python":
		return minifyPythonSource(code), nil
	}
	if language == "ts" || language == "typescript" {
		result := api.Transform(code, api.TransformOptions{
			Loader:            api.LoaderTS,
			Target:            api.ESNext,
			MinifyWhitespace:  true,
			MinifySyntax:      true,
			MinifyIdentifiers: true,
			LogLevel:          api.LogLevelSilent,
		})
		if len(result.Errors) > 0 {
			return "", fmt.Errorf("cannot minify TypeScript source: %s", result.Errors[0].Text)
		}
		return strings.TrimSpace(string(result.Code)), nil
	}
	mediaType := ""
	switch language {
	case "js", "javascript":
		mediaType = "application/javascript"
	case "css":
		mediaType = "text/css"
	case "html":
		mediaType = "text/html"
	case "xml":
		mediaType = "application/xml"
	case "json":
		mediaType = "application/json"
	case "php":
		mediaType = "application/x-httpd-php"
	default:
		return "", fmt.Errorf("minification is unavailable for %s", language)
	}
	if language == "json" {
		var value interface{}
		if err := json.Unmarshal([]byte(code), &value); err != nil {
			return "", fmt.Errorf("cannot minify invalid JSON: %w", err)
		}
	}
	minifier := minify.New()
	minifier.AddFunc("application/javascript", js.Minify)
	minifier.AddFunc("text/css", css.Minify)
	minifier.AddFunc("text/html", html.Minify)
	minifier.AddFunc("application/json", minifyjson.Minify)
	minifier.AddFunc("application/xml", xml.Minify)
	phpMinifier := &html.Minifier{TemplateDelims: [2]string{"<?", "?>"}}
	minifier.Add("application/x-httpd-php", phpMinifier)
	minified, err := minifier.String(mediaType, code)
	if err != nil {
		return "", fmt.Errorf("cannot minify %s source: %w", language, err)
	}
	return minified, nil
}

func minifyStylesheetSource(source string) (string, error) {
	var output strings.Builder
	quote := byte(0)
	escaped := false
	blockComment := false
	preserveBlockComment := false
	lineComment := false
	pendingSpace := false
	for index := 0; index < len(source); index++ {
		character := source[index]
		var next byte
		if index+1 < len(source) {
			next = source[index+1]
		}
		if blockComment {
			if preserveBlockComment {
				output.WriteByte(character)
			}
			if character == '*' && next == '/' {
				if preserveBlockComment {
					output.WriteByte(next)
				}
				blockComment = false
				preserveBlockComment = false
				index++
				pendingSpace = true
			}
			continue
		}
		if lineComment {
			if character == '\n' {
				lineComment = false
				pendingSpace = true
			}
			continue
		}
		if quote != 0 {
			output.WriteByte(character)
			if escaped {
				escaped = false
			} else if character == '\\' {
				escaped = true
			} else if character == quote {
				quote = 0
			}
			continue
		}
		if character == '\'' || character == '"' {
			writePendingSpace(&output, pendingSpace)
			pendingSpace = false
			quote = character
			output.WriteByte(character)
			continue
		}
		if character == '/' && next == '*' {
			if index+2 < len(source) && source[index+2] == '!' {
				writePendingSpace(&output, pendingSpace)
				pendingSpace = false
				output.WriteString("/*!")
				index += 2
				blockComment = true
				preserveBlockComment = true
				continue
			}
			blockComment = true
			index++
			pendingSpace = true
			continue
		}
		if character == '/' && next == '/' && (index == 0 || isCSSCommentBoundary(source[index-1])) {
			lineComment = true
			index++
			pendingSpace = true
			continue
		}
		if character == ' ' || character == '\t' || character == '\r' || character == '\n' {
			pendingSpace = true
			continue
		}
		if strings.ContainsRune("{}:;,", rune(character)) {
			trimTrailingSpace(&output)
			pendingSpace = false
			output.WriteByte(character)
			continue
		}
		if pendingSpace && output.Len() > 0 {
			previous := output.String()[output.Len()-1]
			if cssTokenByte(previous) && cssTokenByte(character) || character == '+' || character == '-' || character == '.' || character == '#' || previous == '+' || previous == '-' {
				output.WriteByte(' ')
			}
		}
		pendingSpace = false
		output.WriteByte(character)
	}
	if quote != 0 {
		return "", fmt.Errorf("cannot minify stylesheet with an unterminated string")
	}
	if blockComment {
		return "", fmt.Errorf("cannot minify stylesheet with an unterminated block comment")
	}
	return strings.TrimSpace(output.String()), nil
}

func isCSSCommentBoundary(character byte) bool {
	return character == ' ' || character == '\t' || character == '\r' || character == '\n' || character == ';' || character == '{' || character == '}'
}

func cssTokenByte(character byte) bool {
	return character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9' || strings.ContainsRune("_$#@-", rune(character))
}

func trimTrailingSpace(output *strings.Builder) {
	value := strings.TrimRight(output.String(), " \t")
	output.Reset()
	output.WriteString(value)
}

func writePendingSpace(output *strings.Builder, pending bool) {
	if pending && output.Len() > 0 && !strings.HasSuffix(output.String(), " ") {
		output.WriteByte(' ')
	}
}

func minifySQLSource(source string) (string, error) {
	var output strings.Builder
	quote := byte(0)
	escaped := false
	blockComment := false
	preserveBlockComment := false
	pendingSpace := false
	dollarQuote := ""
	for index := 0; index < len(source); index++ {
		character := source[index]
		var next byte
		if index+1 < len(source) {
			next = source[index+1]
		}
		if dollarQuote != "" {
			if strings.HasPrefix(source[index:], dollarQuote) {
				output.WriteString(dollarQuote)
				index += len(dollarQuote) - 1
				dollarQuote = ""
			} else {
				output.WriteByte(character)
			}
			continue
		}
		if blockComment {
			if preserveBlockComment {
				output.WriteByte(character)
			}
			if character == '*' && next == '/' {
				if preserveBlockComment {
					output.WriteByte(next)
				}
				blockComment = false
				preserveBlockComment = false
				index++
				pendingSpace = true
			}
			continue
		}
		if quote != 0 {
			output.WriteByte(character)
			if escaped {
				escaped = false
			} else if character == '\\' {
				escaped = true
			} else if character == quote {
				if next == quote {
					output.WriteByte(next)
					index++
				} else {
					quote = 0
				}
			}
			continue
		}
		if character == '\'' || character == '"' || character == '`' {
			writePendingSpace(&output, pendingSpace)
			pendingSpace = false
			quote = character
			output.WriteByte(character)
			continue
		}
		if character == '[' {
			writePendingSpace(&output, pendingSpace)
			pendingSpace = false
			quote = ']'
			output.WriteByte(character)
			continue
		}
		if character == '$' {
			if delimiter := sqlDollarQuoteDelimiter(source, index); delimiter != "" {
				writePendingSpace(&output, pendingSpace)
				pendingSpace = false
				output.WriteString(delimiter)
				index += len(delimiter) - 1
				dollarQuote = delimiter
				continue
			}
		}
		if character == '-' && next == '-' {
			for index < len(source) && source[index] != '\n' {
				index++
			}
			pendingSpace = true
			continue
		}
		if character == '/' && next == '*' {
			if index+2 < len(source) && source[index+2] == '+' {
				writePendingSpace(&output, pendingSpace)
				pendingSpace = false
				output.WriteString("/*+")
				index += 2
				blockComment = true
				preserveBlockComment = true
				continue
			}
			blockComment = true
			index++
			pendingSpace = true
			continue
		}
		if character == ' ' || character == '\t' || character == '\r' || character == '\n' {
			pendingSpace = true
			continue
		}
		if strings.ContainsRune(",;()", rune(character)) {
			trimTrailingSpace(&output)
			pendingSpace = false
			output.WriteByte(character)
			continue
		}
		writePendingSpace(&output, pendingSpace)
		pendingSpace = false
		output.WriteByte(character)
	}
	if quote != 0 {
		return "", fmt.Errorf("cannot minify SQL with an unterminated quoted value")
	}
	if blockComment {
		return "", fmt.Errorf("cannot minify SQL with an unterminated block comment")
	}
	if dollarQuote != "" {
		return "", fmt.Errorf("cannot minify SQL with an unterminated dollar-quoted value")
	}
	return strings.TrimSpace(output.String()), nil
}

func minifyPythonSource(source string) string {
	lines := strings.Split(strings.ReplaceAll(source, "\r\n", "\n"), "\n")
	kept := make([]string, 0, len(lines))
	var tripleQuote byte
	for _, line := range lines {
		if tripleQuote == 0 {
			line = stripPythonComment(line, &tripleQuote)
			if tripleQuote == 0 {
				line = strings.TrimRight(line, " \t")
			}
			if strings.TrimSpace(line) == "" {
				continue
			}
		} else {
			stripPythonComment(line, &tripleQuote)
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}

func stripPythonComment(line string, tripleQuote *byte) string {
	quote := byte(0)
	escaped := false
	for index := 0; index < len(line); index++ {
		character := line[index]
		if *tripleQuote != 0 {
			if character == '\\' {
				index++
				if index < len(line) {
					index++
				}
				continue
			}
			if character == *tripleQuote && index+2 < len(line) && line[index+1] == character && line[index+2] == character {
				*tripleQuote = 0
				index += 2
			}
			continue
		}
		if quote != 0 {
			if escaped {
				escaped = false
			} else if character == '\\' {
				escaped = true
			} else if character == quote {
				quote = 0
			}
			continue
		}
		if (character == '\'' || character == '"') && index+2 < len(line) && line[index+1] == character && line[index+2] == character {
			*tripleQuote = character
			index += 2
			continue
		}
		if character == '\'' || character == '"' {
			quote = character
			continue
		}
		if character == '#' {
			return line[:index]
		}
	}
	return line
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
	if regexp.MustCompile(`(?m)^(def|class|import|from)\s+`).MatchString(code) {
		return "python"
	}
	if strings.Contains(code, "{") && (strings.Contains(code, ":") || strings.Contains(code, ";")) {
		return "css"
	}
	if strings.Contains(strings.ToUpper(code), "SELECT") || strings.Contains(strings.ToUpper(code), "INSERT") {
		return "sql"
	}

	return "unknown"
}

func parseBeautifyOptions(code string) (string, bool, string, string, error) {
	fields := strings.Fields(code)
	indent := ""
	style := ""
	minify := false
	consumed := 0
	for consumed < len(fields) {
		field := fields[consumed]
		switch {
		case strings.HasPrefix(field, "--indent="):
			indent = strings.TrimPrefix(field, "--indent=")
			if indent != "2" && indent != "4" && indent != "tab" {
				return "", false, "", "", fmt.Errorf("indent must be 2, 4, or tab")
			}
			consumed++
		case strings.HasPrefix(field, "--style="):
			style = strings.TrimPrefix(field, "--style=")
			consumed++
		case field == "--minify":
			minify = true
			consumed++
		default:
			return indent, minify, style, remainingBeautifySource(code, fields, consumed), nil
		}
	}
	return indent, minify, style, remainingBeautifySource(code, fields, consumed), nil
}

func remainingBeautifySource(code string, fields []string, consumed int) string {
	remaining := code
	for index := 0; index < consumed; index++ {
		remaining = strings.TrimLeft(remaining, " \t\r\n")
		remaining = strings.TrimPrefix(remaining, fields[index])
	}
	return strings.TrimSpace(remaining)
}

func normalizeBeautifyStyle(language, requested string) (string, error) {
	language = strings.ToLower(language)
	if requested == "" || requested == "standard" {
		switch language {
		case "js", "javascript", "ts", "typescript", "php":
			return "kr", nil
		case "css", "scss", "less", "html", "xml":
			return "expanded", nil
		case "json":
			return "pretty", nil
		case "sql":
			return "uppercase", nil
		case "python":
			return "pep8", nil
		default:
			return "standard", nil
		}
	}
	allowed := map[string]map[string]bool{
		"js": {"kr": true, "allman": true, "compact": true}, "javascript": {"kr": true, "allman": true, "compact": true},
		"ts": {"kr": true, "allman": true, "compact": true}, "typescript": {"kr": true, "allman": true, "compact": true},
		"php": {"kr": true, "allman": true, "compact": true},
		"css": {"expanded": true, "compact": true}, "scss": {"expanded": true, "compact": true}, "less": {"expanded": true, "compact": true},
		"html": {"expanded": true, "compact": true}, "xml": {"expanded": true, "compact": true},
		"json":   {"pretty": true, "compact": true},
		"sql":    {"uppercase": true, "lowercase": true, "compact": true},
		"python": {"pep8": true},
	}
	if !allowed[language][requested] {
		return "", fmt.Errorf("style %q is not supported for %s", requested, language)
	}
	return requested, nil
}

func applyBeautifyStyle(formatted, source, language, style string) (string, error) {
	switch style {
	case "allman":
		return allmanBraceStyle(formatted), nil
	case "compact":
		return minifyCode(source, language)
	case "lowercase":
		return lowercaseSQLKeywords(formatted), nil
	default:
		return formatted, nil
	}
}

func allmanBraceStyle(formatted string) string {
	var output strings.Builder
	quote := byte(0)
	escaped := false
	lineComment := false
	blockComment := false
	for index := 0; index < len(formatted); index++ {
		current := formatted[index]
		if lineComment {
			output.WriteByte(current)
			if current == '\n' {
				lineComment = false
			}
			continue
		}
		if blockComment {
			output.WriteByte(current)
			if current == '*' && index+1 < len(formatted) && formatted[index+1] == '/' {
				output.WriteByte(formatted[index+1])
				index++
				blockComment = false
			}
			continue
		}
		if quote != 0 {
			output.WriteByte(current)
			if escaped {
				escaped = false
			} else if current == '\\' {
				escaped = true
			} else if current == quote {
				quote = 0
			}
			continue
		}
		if current == '/' && index+1 < len(formatted) {
			if formatted[index+1] == '/' {
				output.WriteString("//")
				index++
				lineComment = true
				continue
			}
			if formatted[index+1] == '*' {
				output.WriteString("/*")
				index++
				blockComment = true
				continue
			}
		}
		if current == '\'' || current == '"' || current == '`' {
			quote = current
			output.WriteByte(current)
			continue
		}
		if current == '{' {
			for output.Len() > 0 {
				text := output.String()
				if text[len(text)-1] != ' ' && text[len(text)-1] != '\t' {
					break
				}
				output.Reset()
				output.WriteString(strings.TrimRight(text, " \t"))
			}
			text := output.String()
			if len(text) > 0 && text[len(text)-1] != '\n' {
				lineStart := strings.LastIndex(text, "\n") + 1
				indent := text[lineStart:]
				indent = indent[:len(indent)-len(strings.TrimLeft(indent, " \t"))]
				output.WriteByte('\n')
				output.WriteString(indent)
			}
		}
		output.WriteByte(current)
	}
	return output.String()
}

func lowercaseSQLKeywords(formatted string) string {
	keywords := map[string]struct{}{}
	for _, keyword := range strings.Fields("SELECT FROM WHERE AND OR NOT IN IS NULL LIKE BETWEEN JOIN INNER LEFT RIGHT OUTER ON AS GROUP BY ORDER HAVING LIMIT OFFSET INSERT INTO VALUES UPDATE SET DELETE CREATE TABLE ALTER DROP DISTINCT COUNT SUM AVG MIN MAX CASE WHEN THEN ELSE END UNION ALL ASC DESC") {
		keywords[keyword] = struct{}{}
	}
	var output strings.Builder
	quote := byte(0)
	escaped := false
	for index := 0; index < len(formatted); {
		current := formatted[index]
		if quote != 0 {
			output.WriteByte(current)
			index++
			if escaped {
				escaped = false
			} else if current == '\\' {
				escaped = true
			} else if current == quote {
				quote = 0
			}
			continue
		}
		if current == '\'' || current == '"' || current == '`' {
			quote = current
			output.WriteByte(current)
			index++
			continue
		}
		if current >= 'A' && current <= 'Z' {
			end := index + 1
			for end < len(formatted) && ((formatted[end] >= 'A' && formatted[end] <= 'Z') || (formatted[end] >= '0' && formatted[end] <= '9') || formatted[end] == '_') {
				end++
			}
			word := formatted[index:end]
			if _, ok := keywords[word]; ok {
				output.WriteString(strings.ToLower(word))
			} else {
				output.WriteString(word)
			}
			index = end
			continue
		}
		output.WriteByte(current)
		index++
	}
	return output.String()
}

func reindentFormatted(formatted, language, indent string) string {
	unit := 2
	if language == "python" {
		unit = 4
	}
	indentText := "  "
	switch indent {
	case "4":
		indentText = "    "
	case "tab":
		indentText = "\t"
	}
	lines := strings.Split(formatted, "\n")
	for index, line := range lines {
		trimmed := strings.TrimLeft(line, " \t")
		if trimmed == "" {
			lines[index] = ""
			continue
		}
		leading := len(line) - len(trimmed)
		level := leading / unit
		lines[index] = strings.Repeat(indentText, level) + trimmed
	}
	return strings.Join(lines, "\n")
}

func beautifyCode(code, lang string) string {
	switch lang {
	case "json":
		return beautifyJSON(code)
	case "html", "xml":
		return beautifyHTML(code)
	case "css", "scss", "less":
		return beautifyCSS(code)
	case "sql":
		return beautifySQL(code)
	case "js", "javascript", "ts", "typescript":
		return beautifyJavaScript(code)
	case "php":
		return beautifyJavaScript(code)
	case "python":
		return beautifyPython(code)
	default:
		return code
	}
}

var phpHeredocStart = regexp.MustCompile(`^<<<[ \t]*(?:'([A-Za-z_][A-Za-z0-9_]*)'|"([A-Za-z_][A-Za-z0-9_]*)"|([A-Za-z_][A-Za-z0-9_]*))[ \t]*\r?\n`)

func protectPHPHeredocs(source string) (string, func(string) string) {
	var protected []string
	var masked strings.Builder
	masked.Grow(len(source))
	quote := byte(0)
	escaped := false
	lineComment := false
	blockComment := false
	placeholderPrefix := "__SEARCH_PHP_LITERAL_"
	for strings.Contains(source, placeholderPrefix) {
		placeholderPrefix += "_"
	}

	for index := 0; index < len(source); {
		current := source[index]
		if lineComment {
			masked.WriteByte(current)
			index++
			if current == '\n' {
				lineComment = false
			}
			continue
		}
		if blockComment {
			masked.WriteByte(current)
			index++
			if current == '*' && index < len(source) && source[index] == '/' {
				masked.WriteByte(source[index])
				index++
				blockComment = false
			}
			continue
		}
		if quote != 0 {
			masked.WriteByte(current)
			index++
			if escaped {
				escaped = false
			} else if current == '\\' {
				escaped = true
			} else if current == quote {
				quote = 0
			}
			continue
		}
		if current == '/' && index+1 < len(source) {
			switch source[index+1] {
			case '/':
				masked.WriteString("//")
				index += 2
				lineComment = true
				continue
			case '*':
				masked.WriteString("/*")
				index += 2
				blockComment = true
				continue
			}
		}
		if current == '#' && (index+1 >= len(source) || source[index+1] != '[') {
			masked.WriteByte(current)
			index++
			lineComment = true
			continue
		}
		if current == '\'' || current == '"' || current == '`' {
			quote = current
			masked.WriteByte(current)
			index++
			continue
		}
		if strings.HasPrefix(source[index:], "<<<") {
			match := phpHeredocStart.FindStringSubmatch(source[index:])
			if len(match) > 0 {
				label := match[1]
				if label == "" {
					label = match[2]
				}
				if label == "" {
					label = match[3]
				}
				openerEnd := index + len(match[0])
				literalEnd := phpHeredocEnd(source, openerEnd, label)
				if literalEnd > openerEnd {
					placeholder := fmt.Sprintf("%s%06d__", placeholderPrefix, len(protected))
					protected = append(protected, source[index:literalEnd])
					masked.WriteString(placeholder)
					index = literalEnd
					continue
				}
			}
		}
		masked.WriteByte(current)
		index++
	}

	if len(protected) == 0 {
		return source, nil
	}
	return masked.String(), func(formatted string) string {
		for index, literal := range protected {
			placeholder := fmt.Sprintf("%s%06d__", placeholderPrefix, index)
			formatted = strings.ReplaceAll(formatted, placeholder, literal)
		}
		return formatted
	}
}

func phpHeredocEnd(source string, start int, label string) int {
	for lineStart := start; lineStart < len(source); {
		lineEnd := strings.IndexByte(source[lineStart:], '\n')
		if lineEnd < 0 {
			lineEnd = len(source)
		} else {
			lineEnd += lineStart
		}
		line := strings.TrimSuffix(source[lineStart:lineEnd], "\r")
		trimmed := strings.TrimLeft(line, " \t")
		if strings.HasPrefix(trimmed, label) {
			tail := strings.TrimSpace(strings.TrimPrefix(trimmed, label))
			next, _ := utf8.DecodeRuneInString(tail)
			if tail == "" || (!unicode.IsLetter(next) && !unicode.IsDigit(next) && next != '_') {
				return lineStart + len(line) - len(strings.TrimLeft(line, " \t")) + len(label)
			}
		}
		if lineEnd == len(source) {
			break
		}
		lineStart = lineEnd + 1
	}
	return 0
}

func countLines(text string) int {
	if text == "" {
		return 0
	}
	return strings.Count(text, "\n") + 1
}

func beautifyPython(code string) string {
	lines := strings.Split(strings.ReplaceAll(code, "\r\n", "\n"), "\n")
	formatted := make([]string, 0, len(lines))
	indentLevel := 0
	continuationDepth := 0
	tripleQuote := rune(0)
	for _, line := range lines {
		if tripleQuote != 0 {
			// Whitespace inside a multiline string is data, not source
			// indentation. Preserve those lines byte-for-byte so formatting
			// cannot silently change the Python program's string value.
			formatted = append(formatted, line)
			continuationDepth = pythonDelimiterDepth(line, continuationDepth, &tripleQuote)
			continue
		}
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			formatted = append(formatted, "")
			continue
		}
		if continuationDepth == 0 && pythonDedentPattern.MatchString(trimmed) && indentLevel > 0 {
			indentLevel--
		}
		level := indentLevel + continuationDepth
		if level < 0 {
			level = 0
		}
		formatted = append(formatted, strings.Repeat("    ", level)+trimmed)
		continuationDepth = pythonDelimiterDepth(trimmed, continuationDepth, &tripleQuote)
		if tripleQuote != 0 {
			trailing := line[len(strings.TrimRight(line, " \t")):]
			formatted[len(formatted)-1] += trailing
		}
		if continuationDepth == 0 && tripleQuote == 0 && strings.HasSuffix(trimmed, ":") {
			indentLevel++
		}
	}
	return strings.TrimSpace(strings.Join(formatted, "\n"))
}

var pythonDedentPattern = regexp.MustCompile(`^(elif\b|else\s*:|except\b|finally\s*:|case\b)`)

func pythonDelimiterDepth(line string, depth int, tripleQuote *rune) int {
	quote := rune(0)
	escaped := false
	for index := 0; index < len(line); {
		character, width := utf8.DecodeRuneInString(line[index:])
		if *tripleQuote != 0 {
			if character == '\\' {
				index += width
				if index < len(line) {
					_, escapedWidth := utf8.DecodeRuneInString(line[index:])
					index += escapedWidth
				}
				continue
			}
			quote := string(*tripleQuote) + string(*tripleQuote) + string(*tripleQuote)
			if strings.HasPrefix(line[index:], quote) {
				*tripleQuote = 0
				index += 3
				continue
			}
			_, size := utf8.DecodeRuneInString(line[index:])
			index += size
			continue
		}
		if quote != 0 {
			if escaped {
				escaped = false
			} else if character == '\\' {
				escaped = true
			} else if character == quote {
				quote = 0
			}
			index += width
			continue
		}
		if character == '#' {
			break
		}
		if (character == '\'' || character == '"') && strings.HasPrefix(line[index:], strings.Repeat(string(character), 3)) {
			*tripleQuote = character
			index += 3
			continue
		}
		switch character {
		case '\'', '"':
			quote = character
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			if depth > 0 {
				depth--
			}
		}
		index += width
	}
	return depth
}

func beautifyJavaScript(code string) string {
	var output strings.Builder
	indent := 0
	parenDepth := 0
	quote := rune(0)
	escaped := false
	lineComment := false
	blockComment := false
	pendingSpace := false
	lineStart := true
	skipNext := false
	skipUntil := 0
	writeIndent := func() {
		if lineStart {
			output.WriteString(strings.Repeat("  ", indent))
			lineStart = false
		}
	}
	writeNewline := func() {
		trimmed := strings.TrimRight(output.String(), " \t")
		output.Reset()
		output.WriteString(trimmed)
		if !strings.HasSuffix(trimmed, "\n") {
			output.WriteByte('\n')
		}
		lineStart = true
		pendingSpace = false
	}
	for index, char := range code {
		if index < skipUntil {
			continue
		}
		if skipNext {
			skipNext = false
			continue
		}
		next := rune(0)
		if index+1 < len(code) {
			next, _ = utf8.DecodeRuneInString(code[index+1:])
		}
		if quote != 0 {
			writeIndent()
			output.WriteRune(char)
			if escaped {
				escaped = false
			} else if char == '\\' {
				escaped = true
			} else if char == quote {
				quote = 0
			}
			continue
		}
		if lineComment {
			writeIndent()
			output.WriteRune(char)
			if char == '\n' {
				lineComment = false
				lineStart = true
			}
			continue
		}
		if blockComment {
			writeIndent()
			output.WriteRune(char)
			if char == '/' && index > 0 && code[index-1] == '*' {
				blockComment = false
			}
			continue
		}
		if char == '/' && next == '/' {
			writeIndent()
			if pendingSpace {
				output.WriteByte(' ')
			}
			output.WriteString("//")
			skipNext = true
			lineComment = true
			pendingSpace = false
			continue
		}
		if char == '/' && next == '*' {
			writeIndent()
			if pendingSpace {
				output.WriteByte(' ')
			}
			output.WriteString("/*")
			skipNext = true
			blockComment = true
			pendingSpace = false
			continue
		}
		if char == '/' && canStartJavaScriptRegex(output.String()) {
			if end := scanJavaScriptRegex(code, index); end > index {
				writeIndent()
				if pendingSpace && output.Len() > 0 && !strings.HasSuffix(output.String(), " ") && !strings.HasSuffix(output.String(), "\n") {
					output.WriteByte(' ')
				}
				output.WriteString(code[index:end])
				pendingSpace = false
				skipUntil = end
				continue
			}
		}
		if char == '\'' || char == '"' || char == '`' {
			writeIndent()
			if pendingSpace {
				output.WriteByte(' ')
			}
			output.WriteRune(char)
			quote = char
			pendingSpace = false
			continue
		}
		if unicode.IsSpace(char) {
			pendingSpace = true
			continue
		}
		switch char {
		case '{':
			writeIndent()
			if pendingSpace || (output.Len() > 0 && !strings.HasSuffix(output.String(), " ") && !strings.HasSuffix(output.String(), "\n")) {
				output.WriteByte(' ')
			}
			output.WriteRune(char)
			indent++
			writeNewline()
		case '}':
			if !lineStart {
				writeNewline()
			}
			if indent > 0 {
				indent--
			}
			writeIndent()
			output.WriteRune(char)
			if next != ';' && next != ',' && next != ')' && next != ']' {
				writeNewline()
			}
		case ';':
			writeIndent()
			output.WriteRune(char)
			if parenDepth == 0 {
				writeNewline()
			} else {
				output.WriteByte(' ')
			}
		case ',':
			writeIndent()
			output.WriteString(", ")
			pendingSpace = false
		case ':':
			writeIndent()
			output.WriteString(": ")
			pendingSpace = false
		case '(':
			writeIndent()
			if pendingSpace {
				output.WriteByte(' ')
			}
			output.WriteRune(char)
			parenDepth++
			pendingSpace = false
		case ')':
			writeIndent()
			output.WriteRune(char)
			if parenDepth > 0 {
				parenDepth--
			}
		case '[':
			writeIndent()
			if pendingSpace {
				output.WriteByte(' ')
			}
			output.WriteRune(char)
			pendingSpace = false
		case ']':
			writeIndent()
			output.WriteRune(char)
		default:
			writeIndent()
			if pendingSpace && output.Len() > 0 && !strings.HasSuffix(output.String(), " ") && !strings.HasSuffix(output.String(), "\n") {
				output.WriteByte(' ')
			}
			output.WriteRune(char)
			pendingSpace = false
		}
	}
	return strings.TrimSpace(output.String())
}

func canStartJavaScriptRegex(formatted string) bool {
	previous := strings.TrimSpace(formatted)
	if previous == "" {
		return true
	}
	if strings.HasSuffix(previous, "++") || strings.HasSuffix(previous, "--") {
		return false
	}
	last, _ := utf8.DecodeLastRuneInString(previous)
	switch last {
	case '=', '(', '[', '{', ',', ':', ';', '!', '&', '|', '?', '+', '-', '*', '%', '^', '~', '<', '>':
		return true
	}
	end := len(previous)
	for end > 0 {
		character, size := utf8.DecodeLastRuneInString(previous[:end])
		if !unicode.IsLetter(character) && !unicode.IsDigit(character) && character != '_' && character != '$' {
			break
		}
		end -= size
	}
	switch previous[end:] {
	case "return", "throw", "case", "delete", "void", "typeof", "instanceof", "in", "of", "yield", "await":
		return true
	default:
		return false
	}
}

func scanJavaScriptRegex(code string, start int) int {
	escaped := false
	inCharacterClass := false
	for index := start + 1; index < len(code); index++ {
		character := code[index]
		if character == '\n' || character == '\r' {
			return 0
		}
		if escaped {
			escaped = false
			continue
		}
		switch character {
		case '\\':
			escaped = true
		case '[':
			inCharacterClass = true
		case ']':
			inCharacterClass = false
		case '/':
			if !inCharacterClass {
				index++
				for index < len(code) && (code[index] >= 'a' && code[index] <= 'z' || code[index] >= 'A' && code[index] <= 'Z') {
					index++
				}
				return index
			}
		}
	}
	return 0
}

func beautifyJSON(code string) string {
	var result bytes.Buffer
	if err := json.Indent(&result, []byte(code), "", "  "); err != nil {
		return code
	}
	return result.String()
}

func beautifyHTML(code string) string {
	code, preserved := preserveHTMLRawText(code)
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
		if strings.HasPrefix(line, "</") && indent > 0 {
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

	formatted := strings.TrimSpace(result.String())
	for index, block := range preserved {
		formatted = strings.ReplaceAll(formatted, htmlRawTextPlaceholder(index), block)
	}
	return formatted
}

var htmlRawTextBlockPattern = regexp.MustCompile(`(?is)<(?:pre|textarea|script|style)\b[^>]*>.*?</(?:pre|textarea|script|style)\s*>`)

func preserveHTMLRawText(source string) (string, []string) {
	blocks := htmlRawTextBlockPattern.FindAllString(source, -1)
	for index, block := range blocks {
		source = strings.Replace(source, block, htmlRawTextPlaceholder(index), 1)
	}
	return source, blocks
}

func htmlRawTextPlaceholder(index int) string {
	return fmt.Sprintf("HTMLRAWTEXTBLOCK%dPLACEHOLDER", index)
}

func beautifyCSS(code string) string {
	var output strings.Builder
	indent := 0
	parenDepth := 0
	for index := 0; index < len(code); {
		character := code[index]
		if character == '\\' && index+1 < len(code) {
			output.WriteString(code[index : index+2])
			index += 2
			continue
		}
		if character == '\'' || character == '"' {
			end := scanCSSQuotedToken(code, index, character)
			output.WriteString(code[index:end])
			index = end
			continue
		}
		if index+1 < len(code) && code[index:index+2] == "/*" {
			end := strings.Index(code[index+2:], "*/")
			if end < 0 {
				output.WriteString(code[index:])
				break
			}
			end += index + 4
			output.WriteString(code[index:end])
			index = end
			continue
		}
		if character == '#' && index+1 < len(code) && code[index+1] == '{' {
			end := scanCSSInterpolation(code, index)
			output.WriteString(code[index:end])
			index = end
			continue
		}
		switch character {
		case '(':
			parenDepth++
			output.WriteByte(character)
		case ')':
			if parenDepth > 0 {
				parenDepth--
			}
			output.WriteByte(character)
		case '{':
			formattedPrefix := output.String()
			if output.Len() > 0 && !strings.HasSuffix(formattedPrefix, " ") && !strings.HasSuffix(formattedPrefix, "\t") && !strings.HasSuffix(formattedPrefix, "\n") {
				output.WriteByte(' ')
			}
			output.WriteByte('{')
			indent++
			output.WriteString("\n" + strings.Repeat("  ", indent))
		case ';':
			output.WriteByte(character)
			if parenDepth == 0 {
				output.WriteString("\n" + strings.Repeat("  ", indent))
			}
		case '}':
			if parenDepth == 0 {
				trimmed := strings.TrimRight(output.String(), " \t\r\n")
				output.Reset()
				output.WriteString(trimmed)
				if output.Len() > 0 && !strings.HasSuffix(output.String(), "\n") {
					output.WriteByte('\n')
				}
				if indent > 0 {
					indent--
				}
				output.WriteString(strings.Repeat("  ", indent))
			}
			output.WriteByte(character)
			if parenDepth == 0 {
				output.WriteString("\n\n")
			}
		default:
			output.WriteByte(character)
		}
		index++
	}
	return strings.TrimSpace(output.String())
}

func scanCSSInterpolation(code string, start int) int {
	depth := 0
	for index := start; index < len(code); index++ {
		if code[index] == '\'' || code[index] == '"' {
			index = scanCSSQuotedToken(code, index, code[index]) - 1
			continue
		}
		switch code[index] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return index + 1
			}
		}
	}
	return len(code)
}

func scanCSSQuotedToken(code string, start int, quote byte) int {
	escaped := false
	for index := start + 1; index < len(code); index++ {
		if escaped {
			escaped = false
			continue
		}
		if code[index] == '\\' {
			escaped = true
			continue
		}
		if code[index] == quote {
			return index + 1
		}
	}
	return len(code)
}

func beautifySQL(code string) string {
	keywords := []string{"SELECT", "FROM", "WHERE", "AND", "OR", "ORDER BY", "GROUP BY", "HAVING", "JOIN", "LEFT JOIN", "RIGHT JOIN", "INNER JOIN", "ON", "INSERT", "INTO", "VALUES", "UPDATE", "SET", "DELETE"}
	pattern := regexp.MustCompile(`(?i)\b(?:` + strings.Join(keywords, `|`) + `)\b`)
	var output strings.Builder
	codeStart := 0
	for index := 0; index < len(code); {
		if delimiter := sqlDollarQuoteDelimiter(code, index); delimiter != "" {
			output.WriteString(formatSQLCodeSegment(pattern, code[codeStart:index]))
			end := strings.Index(code[index+len(delimiter):], delimiter)
			if end < 0 {
				output.WriteString(code[index:])
				return strings.TrimSpace(output.String())
			}
			end += index + 2*len(delimiter)
			output.WriteString(code[index:end])
			index = end
			codeStart = end
			continue
		}
		quote := code[index]
		if quote == '\'' || quote == '"' || quote == '`' || quote == '[' {
			output.WriteString(formatSQLCodeSegment(pattern, code[codeStart:index]))
			end := scanSQLQuotedToken(code, index, quote)
			output.WriteString(code[index:end])
			index = end
			codeStart = end
			continue
		}
		if (index+1 < len(code) && code[index:index+2] == "--") || code[index] == '#' {
			output.WriteString(formatSQLCodeSegment(pattern, code[codeStart:index]))
			end := strings.IndexByte(code[index:], '\n')
			if end < 0 {
				output.WriteString(code[index:])
				return strings.TrimSpace(output.String())
			}
			end += index + 1
			output.WriteString(code[index:end])
			index = end
			codeStart = end
			continue
		}
		if index+1 < len(code) && code[index:index+2] == "/*" {
			output.WriteString(formatSQLCodeSegment(pattern, code[codeStart:index]))
			end := strings.Index(code[index+2:], "*/")
			if end < 0 {
				output.WriteString(code[index:])
				return strings.TrimSpace(output.String())
			}
			end += index + 4
			output.WriteString(code[index:end])
			index = end
			codeStart = end
			continue
		}
		index++
	}
	output.WriteString(formatSQLCodeSegment(pattern, code[codeStart:]))
	return strings.TrimSpace(output.String())
}

func sqlDollarQuoteDelimiter(code string, start int) string {
	if code[start] != '$' {
		return ""
	}
	end := strings.IndexByte(code[start+1:], '$')
	if end < 0 {
		return ""
	}
	end += start + 1
	for index := start + 1; index < end; index++ {
		character := code[index]
		if character == '_' || character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || index > start+1 && character >= '0' && character <= '9' {
			continue
		}
		return ""
	}
	return code[start : end+1]
}

func scanSQLQuotedToken(code string, start int, quote byte) int {
	closeQuote := quote
	if quote == '[' {
		closeQuote = ']'
	}
	for index := start + 1; index < len(code); index++ {
		if code[index] != closeQuote {
			continue
		}
		if index+1 < len(code) && code[index+1] == closeQuote {
			index++
			continue
		}
		return index + 1
	}
	return len(code)
}

func formatSQLCodeSegment(pattern *regexp.Regexp, code string) string {
	return pattern.ReplaceAllStringFunc(code, func(keyword string) string {
		return "\n" + strings.ToUpper(keyword)
	})
}

func formatBeautifyContent(input, output, lang, indent, style, minified string) string {
	var html strings.Builder
	html.WriteString("<div class=\"beautify-content\">")
	fmt.Fprintf(&html, "<h1>Code Beautifier - %s</h1>", escapeHTML(lang))
	fmt.Fprintf(&html, "<p class=\"line-counts\">Lines before: %d · Lines after: %d</p>", countLines(input), countLines(output))
	if indent != "" {
		fmt.Fprintf(&html, "<p>Indentation: %s</p>", escapeHTML(indent))
	}
	fmt.Fprintf(&html, "<p class=\"formatter-style\">Style: %s</p>", escapeHTML(style))

	html.WriteString("<h2>Formatted Code</h2>")
	fmt.Fprintf(&html, "<pre class=\"code %s\"><code>%s</code></pre>", escapeHTML(lang), highlightFormattedCode(output, lang))
	html.WriteString("<button type=\"button\" class=\"copy-btn\">Copy</button>")
	if minified != "" {
		html.WriteString("<details class=\"minified-code\"><summary>Minified output</summary>")
		html.WriteString("<pre><code>" + highlightFormattedCode(minified, lang) + "</code></pre></details>")
	}
	html.WriteString("<p class=\"formatter-options\">Options: prefix the source with <code>--indent=2</code>, <code>--indent=4</code>, or <code>--indent=tab</code>. Use <code>--style=allman</code> or <code>--style=kr</code> for JavaScript, TypeScript, and PHP; <code>--style=expanded</code> or <code>--style=compact</code> for CSS, SCSS, LESS, HTML, and XML; <code>--style=pretty</code> or <code>--style=compact</code> for JSON; and <code>--style=uppercase</code> or <code>--style=lowercase</code> for SQL. Python uses <code>pep8</code>. <code>--minify</code> is also available where supported.</p>")

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

func (h *DiffHandler) HandleDirectQuery(ctx context.Context, term string) (*Answer, error) {
	uiLang := LangFromContext(ctx)
	term = strings.TrimSpace(term)
	if term == "" {
		return nil, fmt.Errorf("two texts required (separated by |||)")
	}

	wordLevel := strings.HasPrefix(term, "--word ")
	ignoreWhitespace := strings.HasPrefix(term, "--ignore-whitespace ")
	unified := strings.HasPrefix(term, "--unified ")
	if wordLevel {
		term = strings.TrimSpace(strings.TrimPrefix(term, "--word "))
	}
	if ignoreWhitespace {
		term = strings.TrimSpace(strings.TrimPrefix(term, "--ignore-whitespace "))
	}
	if unified {
		term = strings.TrimSpace(strings.TrimPrefix(term, "--unified "))
	}
	parts := strings.Split(term, "|||")
	if len(parts) != 2 {
		return &Answer{
			Type:        AnswerTypeDiff,
			Term:        truncateString(term, 50),
			Title:       i18n.T(uiLang, "direct.text_diff_title"),
			Description: i18n.T(uiLang, "direct.invalid_format"),
			Content:     "<p class=\"error\">Please provide two texts separated by <code>|||</code></p><p>Example: <code>diff:hello world|||hello there</code></p>",
			Error:       "invalid_format",
		}, nil
	}

	text1 := strings.TrimSpace(parts[0])
	text2 := strings.TrimSpace(parts[1])

	diff := alignedDiff(text1, text2, ignoreWhitespace)

	data := map[string]interface{}{
		"text1":             text1,
		"text2":             text2,
		"diff":              diff,
		"word_level":        wordLevel,
		"ignore_whitespace": ignoreWhitespace,
	}

	return &Answer{
		Type:        AnswerTypeDiff,
		Term:        truncateString(term, 50),
		Title:       i18n.T(uiLang, "direct.text_diff_title"),
		Description: i18n.T(uiLang, "direct.comparison_of_two_texts"),
		Content:     formatDiffContentOptions(text1, text2, diff, wordLevel, ignoreWhitespace, unified),
		Source:      "Diff Tool",
		Data:        data,
	}, nil
}

type diffLine struct {
	// "same", "add", "remove"
	Type    string
	Text    string
	OldLine int
	NewLine int
}

const maxDiffCells = 1_000_000

func alignedDiff(text1, text2 string, ignoreWhitespace bool) []diffLine {
	a, b := strings.Split(text1, "\n"), strings.Split(text2, "\n")
	if len(a) > maxDiffCells/len(b) {
		return simpleDiff(text1, text2)
	}
	key := func(s string) string {
		if ignoreWhitespace {
			return strings.Join(strings.Fields(s), "")
		}
		return s
	}
	cols := len(b) + 1
	dp := make([]int, (len(a)+1)*cols)
	for i := len(a) - 1; i >= 0; i-- {
		for j := len(b) - 1; j >= 0; j-- {
			if key(a[i]) == key(b[j]) {
				dp[i*cols+j] = dp[(i+1)*cols+j+1] + 1
			} else if dp[(i+1)*cols+j] >= dp[i*cols+j+1] {
				dp[i*cols+j] = dp[(i+1)*cols+j]
			} else {
				dp[i*cols+j] = dp[i*cols+j+1]
			}
		}
	}
	var out []diffLine
	i, j := 0, 0
	for i < len(a) || j < len(b) {
		if i < len(a) && j < len(b) && key(a[i]) == key(b[j]) {
			out = append(out, diffLine{Type: "same", Text: a[i], OldLine: i + 1, NewLine: j + 1})
			i++
			j++
		} else if i < len(a) && (j == len(b) || dp[(i+1)*cols+j] >= dp[i*cols+j+1]) {
			out = append(out, diffLine{Type: "remove", Text: a[i], OldLine: i + 1})
			i++
		} else {
			out = append(out, diffLine{Type: "add", Text: b[j], NewLine: j + 1})
			j++
		}
	}
	return out
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
		if i < len(lines1) && i < len(lines2) && lines1[i] == lines2[i] {
			l1 := lines1[i]
			result = append(result, diffLine{Type: "same", Text: l1, OldLine: i + 1, NewLine: i + 1})
		} else {
			if i < len(lines1) {
				result = append(result, diffLine{Type: "remove", Text: lines1[i], OldLine: i + 1})
			}
			if i < len(lines2) {
				result = append(result, diffLine{Type: "add", Text: lines2[i], NewLine: i + 1})
			}
		}
	}

	return result
}

func formatDiffContentOptions(text1, text2 string, diff []diffLine, wordLevel, ignoreWhitespace, unified bool) string {
	var html strings.Builder
	html.WriteString("<div class=\"diff-content\">")
	html.WriteString("<h1>Text Diff</h1>")
	query := func(flag string) string { return url.QueryEscape("diff:" + flag + text1 + "|||" + text2) }
	fmt.Fprintf(&html, "<nav class=\"diff-options\"><a href=\"/?q=%s\">Side by side</a> <a href=\"/?q=%s\">Unified</a> <a href=\"/?q=%s\">Word-level</a> <a href=\"/?q=%s\">Ignore whitespace</a></nav>", query(""), query("--unified "), query("--word "), query("--ignore-whitespace "))

	// Statistics
	added := 0
	removed := 0
	changed := 0
	for _, d := range diff {
		switch d.Type {
		case "add":
			added++
		case "remove":
			removed++
		}
	}
	for i := 0; i+1 < len(diff); i++ {
		if diff[i].Type == "remove" && diff[i+1].Type == "add" {
			changed++
			i++
		}
	}
	fmt.Fprintf(&html, "<p class=\"diff-stats\">+%d additions, -%d removals, %d changed</p>", added, removed, changed)
	var copyOutput strings.Builder
	for _, line := range diff {
		prefix := "  "
		if line.Type == "add" {
			prefix = "+ "
		}
		if line.Type == "remove" {
			prefix = "- "
		}
		copyOutput.WriteString(prefix + line.Text + "\n")
	}
	fmt.Fprintf(&html, "<button type=\"button\" class=\"copy-btn\" data-copy=\"%s\"><span class=\"copy-text\">Copy diff</span></button>", escapeHTML(copyOutput.String()))
	language := detectLanguage(text1 + "\n" + text2)
	contentFor := func(text string) string {
		if language == "unknown" {
			return escapeHTML(text)
		}
		return highlightFormattedCode(text, language)
	}

	if unified {
		html.WriteString("<pre class=\"unified-diff\"><code>")
		writeUnifiedDiff(&html, text1, text2, diff, contentFor)
		html.WriteString("</code></pre></div>")
		return html.String()
	}
	// Side by side
	html.WriteString("<div class=\"diff-view\">")
	html.WriteString("<table class=\"diff-table\">")
	html.WriteString("<thead><tr><th>Original</th><th>Modified</th></tr></thead>")
	html.WriteString("<tbody>")

	for i := 0; i < len(diff); i++ {
		d := diff[i]
		switch d.Type {
		case "same":
			fmt.Fprintf(&html, "<tr><td><span class=\"line-number\">%d</span>%s</td><td><span class=\"line-number\">%d</span>%s</td></tr>", d.OldLine, contentFor(d.Text), d.NewLine, contentFor(d.Text))
		case "remove":
			if wordLevel && i+1 < len(diff) && diff[i+1].Type == "add" {
				oldText, newText := inlineWordDiff(d.Text, diff[i+1].Text)
				fmt.Fprintf(&html, "<tr class=\"changed\"><td class=\"removed\"><span class=\"line-number\">%d</span>%s</td><td class=\"added\"><span class=\"line-number\">%d</span>%s</td></tr>", d.OldLine, oldText, diff[i+1].NewLine, newText)
				i++
				continue
			}
			content := contentFor(d.Text)
			if wordLevel {
				content = "<del>" + content + "</del>"
			}
			fmt.Fprintf(&html, "<tr class=\"removed\"><td class=\"removed\"><span class=\"line-number\">%d</span>%s</td><td></td></tr>", d.OldLine, content)
		case "add":
			content := contentFor(d.Text)
			if wordLevel {
				content = "<ins>" + content + "</ins>"
			}
			fmt.Fprintf(&html, "<tr class=\"added\"><td></td><td class=\"added\"><span class=\"line-number\">%d</span>%s</td></tr>", d.NewLine, content)
		}
	}

	html.WriteString("</tbody></table>")
	html.WriteString("</div>")
	html.WriteString("<h2>Unified diff</h2><pre class=\"unified-diff\"><code>")
	writeUnifiedDiff(&html, text1, text2, diff, contentFor)
	html.WriteString("</code></pre>")

	html.WriteString("</div>")
	return html.String()
}

func writeUnifiedDiff(html *strings.Builder, text1, text2 string, diff []diffLine, formatLine func(string) string) {
	oldLines, newLines := strings.Split(text1, "\n"), strings.Split(text2, "\n")
	fmt.Fprintf(html, "--- Original\n+++ Modified\n@@ -1,%d +1,%d @@\n", len(oldLines), len(newLines))
	for _, line := range diff {
		prefix := " "
		if line.Type == "add" {
			prefix = "+"
		}
		if line.Type == "remove" {
			prefix = "-"
		}
		fmt.Fprintf(html, "%s%s\n", prefix, formatLine(line.Text))
	}
}

func inlineWordDiff(oldText, newText string) (string, string) {
	oldTokens, newTokens := diffWordTokens(oldText), diffWordTokens(newText)
	prefix := 0
	for prefix < len(oldTokens) && prefix < len(newTokens) && oldTokens[prefix] == newTokens[prefix] {
		prefix++
	}
	suffix := 0
	for suffix < len(oldTokens)-prefix && suffix < len(newTokens)-prefix && oldTokens[len(oldTokens)-1-suffix] == newTokens[len(newTokens)-1-suffix] {
		suffix++
	}
	oldHTML := escapeHTML(strings.Join(oldTokens[:prefix], "")) + "<del>" + escapeHTML(strings.Join(oldTokens[prefix:len(oldTokens)-suffix], "")) + "</del>" + escapeHTML(strings.Join(oldTokens[len(oldTokens)-suffix:], ""))
	newHTML := escapeHTML(strings.Join(newTokens[:prefix], "")) + "<ins>" + escapeHTML(strings.Join(newTokens[prefix:len(newTokens)-suffix], "")) + "</ins>" + escapeHTML(strings.Join(newTokens[len(newTokens)-suffix:], ""))
	return oldHTML, newHTML
}

func diffWordTokens(text string) []string {
	runes := []rune(text)
	var tokens []string
	for start := 0; start < len(runes); {
		kind := unicode.IsSpace(runes[start])
		end := start + 1
		for end < len(runes) && unicode.IsSpace(runes[end]) == kind {
			end++
		}
		if !kind {
			for end < len(runes) && !unicode.IsSpace(runes[end]) {
				end++
			}
		}
		tokens = append(tokens, string(runes[start:end]))
		start = end
	}
	return tokens
}
