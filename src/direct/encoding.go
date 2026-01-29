package direct

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

// HTMLHandler handles html:{text} queries
type HTMLHandler struct{}

// NewHTMLHandler creates a new HTML encoding handler
func NewHTMLHandler() *HTMLHandler {
	return &HTMLHandler{}
}

func (h *HTMLHandler) Type() AnswerType {
	return AnswerTypeHTML
}

func (h *HTMLHandler) Handle(ctx context.Context, term string) (*Answer, error) {
	term = strings.TrimSpace(term)
	if term == "" {
		return nil, fmt.Errorf("text required")
	}

	// Check if encode or decode mode
	mode := "encode"
	text := term

	if strings.HasPrefix(strings.ToLower(term), "encode ") {
		text = strings.TrimPrefix(term, "encode ")
		text = strings.TrimPrefix(text, "Encode ")
	} else if strings.HasPrefix(strings.ToLower(term), "decode ") {
		mode = "decode"
		text = strings.TrimPrefix(term, "decode ")
		text = strings.TrimPrefix(text, "Decode ")
	} else if strings.Contains(term, "&") && (strings.Contains(term, ";") || strings.Contains(term, "#")) {
		// Looks like HTML entities, decode
		mode = "decode"
	}

	var result string
	if mode == "encode" {
		result = htmlEncode(text)
	} else {
		result = htmlDecode(text)
	}

	data := map[string]interface{}{
		"input":  text,
		"output": result,
		"mode":   mode,
	}

	return &Answer{
		Type:        AnswerTypeHTML,
		Term:        term,
		Title:       fmt.Sprintf("HTML %s", strings.Title(mode)),
		Description: fmt.Sprintf("HTML entity %s", mode),
		Content:     formatHTMLEncodingContent(mode, text, result),
		Source:      "Local Encoder",
		Data:        data,
	}, nil
}

func htmlEncode(s string) string {
	var result strings.Builder
	for _, r := range s {
		switch r {
		case '&':
			result.WriteString("&amp;")
		case '<':
			result.WriteString("&lt;")
		case '>':
			result.WriteString("&gt;")
		case '"':
			result.WriteString("&quot;")
		case '\'':
			result.WriteString("&#39;")
		default:
			if r > 127 {
				result.WriteString(fmt.Sprintf("&#%d;", r))
			} else {
				result.WriteRune(r)
			}
		}
	}
	return result.String()
}

func htmlDecode(s string) string {
	// Basic HTML entity decoding
	replacer := strings.NewReplacer(
		"&amp;", "&",
		"&lt;", "<",
		"&gt;", ">",
		"&quot;", "\"",
		"&#39;", "'",
		"&apos;", "'",
		"&nbsp;", " ",
		"&copy;", "©",
		"&reg;", "®",
		"&trade;", "™",
		"&mdash;", "—",
		"&ndash;", "–",
		"&rarr;", "→",
		"&larr;", "←",
	)
	s = replacer.Replace(s)

	// Decode numeric entities
	re := regexp.MustCompile(`&#(\d+);`)
	s = re.ReplaceAllStringFunc(s, func(m string) string {
		matches := re.FindStringSubmatch(m)
		if len(matches) == 2 {
			if n, err := strconv.Atoi(matches[1]); err == nil {
				return string(rune(n))
			}
		}
		return m
	})

	// Decode hex entities
	reHex := regexp.MustCompile(`&#[xX]([0-9a-fA-F]+);`)
	s = reHex.ReplaceAllStringFunc(s, func(m string) string {
		matches := reHex.FindStringSubmatch(m)
		if len(matches) == 2 {
			if n, err := strconv.ParseInt(matches[1], 16, 32); err == nil {
				return string(rune(n))
			}
		}
		return m
	})

	return s
}

func formatHTMLEncodingContent(mode, input, output string) string {
	var html strings.Builder
	html.WriteString("<div class=\"html-encoding-content\">")
	html.WriteString(fmt.Sprintf("<h1>HTML %s</h1>", strings.Title(mode)))

	html.WriteString("<h2>Input</h2>")
	html.WriteString(fmt.Sprintf("<pre><code>%s</code></pre>", escapeHTML(input)))

	html.WriteString("<h2>Output</h2>")
	html.WriteString(fmt.Sprintf("<pre><code>%s</code></pre>", escapeHTML(output)))
	html.WriteString("<button class=\"copy-btn\" onclick=\"copyCode(this)\">Copy</button>")

	html.WriteString("</div>")
	return html.String()
}

// UnicodeHandler handles unicode:{char} queries
type UnicodeHandler struct{}

// NewUnicodeHandler creates a new Unicode handler
func NewUnicodeHandler() *UnicodeHandler {
	return &UnicodeHandler{}
}

func (h *UnicodeHandler) Type() AnswerType {
	return AnswerTypeUnicode
}

func (h *UnicodeHandler) Handle(ctx context.Context, term string) (*Answer, error) {
	term = strings.TrimSpace(term)
	if term == "" {
		return nil, fmt.Errorf("character or code point required")
	}

	var r rune
	var codePoint string

	// Parse input
	if strings.HasPrefix(term, "U+") || strings.HasPrefix(term, "u+") {
		// U+XXXX format
		hex := strings.TrimPrefix(strings.TrimPrefix(term, "U+"), "u+")
		if n, err := strconv.ParseInt(hex, 16, 32); err == nil {
			r = rune(n)
			codePoint = fmt.Sprintf("U+%04X", r)
		}
	} else if strings.HasPrefix(term, "\\u") || strings.HasPrefix(term, "\\U") {
		// \uXXXX format
		hex := strings.TrimPrefix(strings.TrimPrefix(term, "\\u"), "\\U")
		if n, err := strconv.ParseInt(hex, 16, 32); err == nil {
			r = rune(n)
			codePoint = fmt.Sprintf("U+%04X", r)
		}
	} else if len(term) <= 4 && utf8.RuneCountInString(term) == 1 {
		// Single character
		r, _ = utf8.DecodeRuneInString(term)
		codePoint = fmt.Sprintf("U+%04X", r)
	} else {
		// Search by name
		return h.searchUnicodeName(term)
	}

	if r == 0 {
		return &Answer{
			Type:        AnswerTypeUnicode,
			Term:        term,
			Title:       "Unicode Lookup",
			Description: "Invalid input",
			Content:     "<p>Could not parse Unicode character or code point.</p>",
			Error:       "invalid_input",
		}, nil
	}

	// Get character info
	category := getUnicodeCategory(r)

	// UTF-8 bytes
	utf8Bytes := make([]byte, 4)
	n := utf8.EncodeRune(utf8Bytes, r)
	utf8Hex := ""
	for i := 0; i < n; i++ {
		if i > 0 {
			utf8Hex += " "
		}
		utf8Hex += fmt.Sprintf("%02X", utf8Bytes[i])
	}

	// UTF-16
	var utf16Hex string
	if r <= 0xFFFF {
		utf16Hex = fmt.Sprintf("%04X", r)
	} else {
		// Surrogate pair
		r1 := ((r - 0x10000) >> 10) + 0xD800
		r2 := ((r - 0x10000) & 0x3FF) + 0xDC00
		utf16Hex = fmt.Sprintf("%04X %04X", r1, r2)
	}

	data := map[string]interface{}{
		"character": string(r),
		"codePoint": codePoint,
		"name":      fmt.Sprintf("%U", r),
		"category":  category,
		"utf8":      utf8Hex,
		"utf16":     utf16Hex,
		"decimal":   int(r),
		"htmlEntity": fmt.Sprintf("&#%d;", r),
		"htmlHex":    fmt.Sprintf("&#x%X;", r),
	}

	return &Answer{
		Type:        AnswerTypeUnicode,
		Term:        term,
		Title:       fmt.Sprintf("Unicode: %s (%s)", string(r), codePoint),
		Description: fmt.Sprintf("Unicode character %s", codePoint),
		Content:     formatUnicodeContent(r, codePoint, category, utf8Hex, utf16Hex),
		Source:      "Unicode Database",
		Data:        data,
	}, nil
}

func (h *UnicodeHandler) searchUnicodeName(name string) (*Answer, error) {
	return &Answer{
		Type:        AnswerTypeUnicode,
		Term:        name,
		Title:       "Unicode Search",
		Description: "Search by name not implemented",
		Content:     fmt.Sprintf("<p>Searching by Unicode name is not yet implemented. Try entering a character directly or use <code>U+XXXX</code> format.</p>"),
		Error:       "not_implemented",
	}, nil
}

func getUnicodeCategory(r rune) string {
	switch {
	case unicode.IsLetter(r):
		return "Letter"
	case unicode.IsDigit(r):
		return "Digit"
	case unicode.IsPunct(r):
		return "Punctuation"
	case unicode.IsSymbol(r):
		return "Symbol"
	case unicode.IsSpace(r):
		return "Space"
	case unicode.IsControl(r):
		return "Control"
	default:
		return "Other"
	}
}

func formatUnicodeContent(r rune, codePoint, category, utf8Hex, utf16Hex string) string {
	var html strings.Builder
	html.WriteString("<div class=\"unicode-content\">")

	// Large character display
	html.WriteString(fmt.Sprintf("<div class=\"char-display\">%s</div>", escapeHTML(string(r))))

	html.WriteString("<table class=\"unicode-table\">")
	html.WriteString("<tbody>")
	html.WriteString(fmt.Sprintf("<tr><td>Code Point</td><td><code>%s</code></td></tr>", codePoint))
	html.WriteString(fmt.Sprintf("<tr><td>Character</td><td><code>%s</code></td></tr>", escapeHTML(string(r))))
	html.WriteString(fmt.Sprintf("<tr><td>Category</td><td>%s</td></tr>", category))
	html.WriteString(fmt.Sprintf("<tr><td>UTF-8</td><td><code>%s</code></td></tr>", utf8Hex))
	html.WriteString(fmt.Sprintf("<tr><td>UTF-16</td><td><code>%s</code></td></tr>", utf16Hex))
	html.WriteString(fmt.Sprintf("<tr><td>Decimal</td><td><code>%d</code></td></tr>", r))
	html.WriteString(fmt.Sprintf("<tr><td>HTML Entity</td><td><code>&amp;#%d;</code></td></tr>", r))
	html.WriteString(fmt.Sprintf("<tr><td>HTML Hex</td><td><code>&amp;#x%X;</code></td></tr>", r))
	html.WriteString(fmt.Sprintf("<tr><td>CSS</td><td><code>\\%X</code></td></tr>", r))
	html.WriteString(fmt.Sprintf("<tr><td>JavaScript</td><td><code>\\u%04X</code></td></tr>", r))
	html.WriteString("</tbody></table>")

	html.WriteString("</div>")
	return html.String()
}

// EmojiHandler handles emoji:{name} queries
type EmojiHandler struct{}

// NewEmojiHandler creates a new emoji handler
func NewEmojiHandler() *EmojiHandler {
	return &EmojiHandler{}
}

func (h *EmojiHandler) Type() AnswerType {
	return AnswerTypeEmoji
}

// Common emojis for search
var emojiDB = map[string][]string{
	"smile":     {"😀", "😃", "😄", "😁", "😆", "😊", "🙂"},
	"laugh":     {"😂", "🤣", "😅", "😆"},
	"love":      {"❤️", "💕", "💖", "💗", "💓", "💝", "😍", "🥰"},
	"heart":     {"❤️", "🧡", "💛", "💚", "💙", "💜", "🖤", "🤍", "🤎", "💔"},
	"sad":       {"😢", "😭", "😞", "😔", "🥺", "😿"},
	"angry":     {"😠", "😡", "🤬", "💢"},
	"fire":      {"🔥", "🌶️"},
	"thumbs":    {"👍", "👎"},
	"hand":      {"👋", "✋", "🖐️", "🤚", "👌", "🤏", "✌️", "🤞", "🤟", "🤘", "🤙", "👈", "👉", "👆", "👇", "☝️", "👏", "🙌", "🤝", "🙏"},
	"cat":       {"🐱", "😺", "😸", "😹", "😻", "😼", "😽", "🙀", "😿", "😾", "🐈"},
	"dog":       {"🐕", "🐶", "🐩", "🐕‍🦺", "🦮"},
	"sun":       {"☀️", "🌞", "🌅", "🌄"},
	"moon":      {"🌙", "🌛", "🌜", "🌝", "🌚", "🌕", "🌖", "🌗", "🌘", "🌑", "🌒", "🌓", "🌔"},
	"star":      {"⭐", "🌟", "✨", "💫", "🌠"},
	"weather":   {"☀️", "🌤️", "⛅", "🌥️", "☁️", "🌦️", "🌧️", "⛈️", "🌩️", "🌨️", "❄️", "🌬️", "💨", "🌪️", "🌈"},
	"food":      {"🍕", "🍔", "🍟", "🌭", "🥪", "🌮", "🌯", "🥗", "🍜", "🍝", "🍣", "🍱", "🍩", "🍪", "🎂", "🍰"},
	"drink":     {"☕", "🍵", "🥤", "🧃", "🍺", "🍻", "🥂", "🍷", "🍸", "🍹", "🧊"},
	"fruit":     {"🍎", "🍐", "🍊", "🍋", "🍌", "🍉", "🍇", "🍓", "🫐", "🍒", "🍑", "🥭", "🍍", "🥝", "🍅", "🥑"},
	"plant":     {"🌱", "🌿", "☘️", "🍀", "🌵", "🌴", "🌳", "🌲", "🪴"},
	"flower":    {"🌸", "💮", "🏵️", "🌹", "🥀", "🌺", "🌻", "🌼", "🌷", "🌾"},
	"flag":      {"🏳️", "🏴", "🏁", "🚩", "🎌", "🏴‍☠️"},
	"check":     {"✅", "✔️", "☑️"},
	"cross":     {"❌", "❎", "✖️"},
	"warning":   {"⚠️", "🚨", "⛔", "🚫", "❗", "❕"},
	"question":  {"❓", "❔", "⁉️", "‼️"},
	"clock":     {"🕐", "🕑", "🕒", "🕓", "🕔", "🕕", "🕖", "🕗", "🕘", "🕙", "🕚", "🕛", "⏰", "⏱️", "⏲️", "🕰️"},
	"music":     {"🎵", "🎶", "🎼", "🎹", "🎸", "🎺", "🎻", "🥁", "🎤", "🎧"},
	"sport":     {"⚽", "🏀", "🏈", "⚾", "🎾", "🏐", "🏉", "🎱", "🏓", "🏸", "🏒", "🏑", "🥍", "🏏", "🥅", "⛳", "🏹", "🎣", "🤿", "🥊", "🥋", "🎿", "⛷️", "🏂", "🏋️", "🤸", "🤺", "⛹️", "🤾", "🏌️", "🏇", "🧘"},
	"vehicle":   {"🚗", "🚕", "🚙", "🚌", "🚎", "🏎️", "🚓", "🚑", "🚒", "🚐", "🛻", "🚚", "🚛", "🚜", "🏍️", "🛵", "🚲", "🛴", "🚂", "✈️", "🚀", "🛸", "🚁", "⛵", "🚢"},
	"building":  {"🏠", "🏡", "🏢", "🏣", "🏤", "🏥", "🏦", "🏨", "🏩", "🏪", "🏫", "🏬", "🏭", "🏯", "🏰", "🗼", "🗽", "⛪", "🕌", "🛕", "🕍", "⛩️"},
	"money":     {"💰", "💵", "💴", "💶", "💷", "💸", "💳", "🧾", "💹"},
	"tech":      {"💻", "🖥️", "🖨️", "⌨️", "🖱️", "💾", "💿", "📀", "📱", "📲", "☎️", "📞", "📟", "📠"},
	"office":    {"📁", "📂", "📃", "📄", "📅", "📆", "📇", "📈", "📉", "📊", "📋", "📌", "📍", "📎", "🖇️", "📏", "📐", "✂️"},
	"write":     {"✏️", "✒️", "🖊️", "🖋️", "📝", "📒", "📓", "📔", "📕", "📖", "📗", "📘", "📙", "📚"},
}

func (h *EmojiHandler) Handle(ctx context.Context, term string) (*Answer, error) {
	term = strings.TrimSpace(strings.ToLower(term))
	if term == "" {
		return nil, fmt.Errorf("emoji name or keyword required")
	}

	// Search emoji database
	var matches []string
	for keyword, emojis := range emojiDB {
		if strings.Contains(keyword, term) || strings.Contains(term, keyword) {
			matches = append(matches, emojis...)
		}
	}

	// Remove duplicates
	seen := make(map[string]bool)
	unique := make([]string, 0)
	for _, e := range matches {
		if !seen[e] {
			seen[e] = true
			unique = append(unique, e)
		}
	}

	if len(unique) == 0 {
		return &Answer{
			Type:        AnswerTypeEmoji,
			Term:        term,
			Title:       fmt.Sprintf("Emoji: %s", term),
			Description: "No emojis found",
			Content:     fmt.Sprintf("<p>No emojis found for <code>%s</code>.</p>", escapeHTML(term)),
			Error:       "not_found",
		}, nil
	}

	data := map[string]interface{}{
		"search": term,
		"emojis": unique,
		"count":  len(unique),
	}

	return &Answer{
		Type:        AnswerTypeEmoji,
		Term:        term,
		Title:       fmt.Sprintf("Emoji: %s", term),
		Description: fmt.Sprintf("%d emojis found", len(unique)),
		Content:     formatEmojiContent(term, unique),
		Source:      "Emoji Database",
		Data:        data,
	}, nil
}

func formatEmojiContent(search string, emojis []string) string {
	var html strings.Builder
	html.WriteString("<div class=\"emoji-content\">")
	html.WriteString(fmt.Sprintf("<h1>Emojis: %s</h1>", escapeHTML(search)))
	html.WriteString(fmt.Sprintf("<p>Found %d matching emojis</p>", len(emojis)))

	html.WriteString("<div class=\"emoji-grid\">")
	for _, e := range emojis {
		// Get code point
		r, _ := utf8.DecodeRuneInString(e)
		codePoint := fmt.Sprintf("U+%X", r)
		html.WriteString(fmt.Sprintf("<div class=\"emoji-item\" onclick=\"copyEmoji('%s')\" title=\"%s\">", e, codePoint))
		html.WriteString(fmt.Sprintf("<span class=\"emoji\">%s</span>", e))
		html.WriteString(fmt.Sprintf("<span class=\"code\">%s</span>", codePoint))
		html.WriteString("</div>")
	}
	html.WriteString("</div>")

	html.WriteString("<p class=\"tip\">Click an emoji to copy it</p>")

	html.WriteString("</div>")
	return html.String()
}

// EscapeHandler handles escape:{text} queries
type EscapeHandler struct{}

// NewEscapeHandler creates a new escape handler
func NewEscapeHandler() *EscapeHandler {
	return &EscapeHandler{}
}

func (h *EscapeHandler) Type() AnswerType {
	return AnswerTypeEscape
}

func (h *EscapeHandler) Handle(ctx context.Context, term string) (*Answer, error) {
	term = strings.TrimSpace(term)
	if term == "" {
		return nil, fmt.Errorf("text required")
	}

	// Parse format if specified
	format := ""
	text := term

	formats := []string{"json", "sql", "html", "url", "regex", "shell", "js", "javascript", "python", "c"}
	for _, f := range formats {
		prefix := f + " "
		if strings.HasPrefix(strings.ToLower(term), prefix) {
			format = f
			text = term[len(prefix):]
			break
		}
	}

	// Generate all escape formats
	escapes := map[string]string{
		"JSON":       escapeJSON(text),
		"SQL":        escapeSQL(text),
		"HTML":       htmlEncode(text),
		"URL":        url.QueryEscape(text),
		"Regex":      regexp.QuoteMeta(text),
		"Shell":      escapeShell(text),
		"JavaScript": escapeJS(text),
	}

	data := map[string]interface{}{
		"input":   text,
		"format":  format,
		"escapes": escapes,
	}

	return &Answer{
		Type:        AnswerTypeEscape,
		Term:        term,
		Title:       "String Escape",
		Description: "Escaped strings for various formats",
		Content:     formatEscapeContent(text, escapes, format),
		Source:      "Local Escaper",
		Data:        data,
	}, nil
}

func escapeJSON(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func escapeSQL(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

func escapeShell(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

func escapeJS(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "'", "\\'")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\r", "\\r")
	s = strings.ReplaceAll(s, "\t", "\\t")
	return "\"" + s + "\""
}

func formatEscapeContent(input string, escapes map[string]string, highlight string) string {
	var html strings.Builder
	html.WriteString("<div class=\"escape-content\">")
	html.WriteString("<h1>String Escape</h1>")

	html.WriteString("<h2>Input</h2>")
	html.WriteString(fmt.Sprintf("<pre><code>%s</code></pre>", escapeHTML(input)))

	html.WriteString("<h2>Escaped Formats</h2>")
	html.WriteString("<table class=\"escape-table\">")
	html.WriteString("<tbody>")

	for format, escaped := range escapes {
		class := ""
		if strings.EqualFold(format, highlight) || strings.EqualFold(format, highlight+"script") {
			class = " class=\"highlighted\""
		}
		html.WriteString(fmt.Sprintf("<tr%s><td><strong>%s</strong></td><td><code>%s</code></td><td><button class=\"copy-btn\" onclick=\"copyText('%s')\">Copy</button></td></tr>",
			class, format, escapeHTML(escaped), escapeHTML(escaped)))
	}

	html.WriteString("</tbody></table>")
	html.WriteString("</div>")
	return html.String()
}

// JSONHandler handles json:{data} queries
type JSONHandler struct{}

// NewJSONHandler creates a new JSON handler
func NewJSONHandler() *JSONHandler {
	return &JSONHandler{}
}

func (h *JSONHandler) Type() AnswerType {
	return AnswerTypeJSON
}

func (h *JSONHandler) Handle(ctx context.Context, term string) (*Answer, error) {
	term = strings.TrimSpace(term)
	if term == "" {
		return nil, fmt.Errorf("JSON data required")
	}

	// Check for mode prefix
	mode := "format" // default
	jsonStr := term

	if strings.HasPrefix(strings.ToLower(term), "minify ") {
		mode = "minify"
		jsonStr = strings.TrimPrefix(term, "minify ")
		jsonStr = strings.TrimPrefix(jsonStr, "Minify ")
	} else if strings.HasPrefix(strings.ToLower(term), "validate ") {
		mode = "validate"
		jsonStr = strings.TrimPrefix(term, "validate ")
		jsonStr = strings.TrimPrefix(jsonStr, "Validate ")
	} else if strings.HasPrefix(strings.ToLower(term), "format ") {
		jsonStr = strings.TrimPrefix(term, "format ")
		jsonStr = strings.TrimPrefix(jsonStr, "Format ")
	}

	// Parse JSON
	var parsed interface{}
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		return &Answer{
			Type:        AnswerTypeJSON,
			Term:        term,
			Title:       "JSON Error",
			Description: "Invalid JSON",
			Content:     fmt.Sprintf("<p class=\"error\">Invalid JSON: %s</p><pre><code>%s</code></pre>", escapeHTML(err.Error()), escapeHTML(jsonStr)),
			Error:       "invalid_json",
		}, nil
	}

	var output string
	switch mode {
	case "minify":
		b, _ := json.Marshal(parsed)
		output = string(b)
	default: // format
		b, _ := json.MarshalIndent(parsed, "", "  ")
		output = string(b)
	}

	// Calculate stats
	keys := countJSONKeys(parsed)
	depth := getJSONDepth(parsed)

	data := map[string]interface{}{
		"input":     jsonStr,
		"output":    output,
		"mode":      mode,
		"valid":     true,
		"keys":      keys,
		"depth":     depth,
		"sizeInput": len(jsonStr),
		"sizeOutput": len(output),
	}

	return &Answer{
		Type:        AnswerTypeJSON,
		Term:        term,
		Title:       fmt.Sprintf("JSON %s", strings.Title(mode)),
		Description: "Valid JSON",
		Content:     formatJSONContent(mode, jsonStr, output, keys, depth),
		Source:      "JSON Parser",
		Data:        data,
	}, nil
}

func countJSONKeys(v interface{}) int {
	count := 0
	switch val := v.(type) {
	case map[string]interface{}:
		count += len(val)
		for _, v := range val {
			count += countJSONKeys(v)
		}
	case []interface{}:
		for _, v := range val {
			count += countJSONKeys(v)
		}
	}
	return count
}

func getJSONDepth(v interface{}) int {
	switch val := v.(type) {
	case map[string]interface{}:
		maxDepth := 0
		for _, v := range val {
			d := getJSONDepth(v)
			if d > maxDepth {
				maxDepth = d
			}
		}
		return maxDepth + 1
	case []interface{}:
		maxDepth := 0
		for _, v := range val {
			d := getJSONDepth(v)
			if d > maxDepth {
				maxDepth = d
			}
		}
		return maxDepth + 1
	default:
		return 0
	}
}

func formatJSONContent(mode, input, output string, keys, depth int) string {
	var html strings.Builder
	html.WriteString("<div class=\"json-content\">")
	html.WriteString(fmt.Sprintf("<h1>JSON %s</h1>", strings.Title(mode)))

	html.WriteString("<p class=\"valid\">✓ Valid JSON</p>")
	html.WriteString(fmt.Sprintf("<p>Keys: %d | Depth: %d | Size: %d → %d bytes</p>", keys, depth, len(input), len(output)))

	html.WriteString("<h2>Output</h2>")
	html.WriteString(fmt.Sprintf("<pre class=\"json-output\"><code>%s</code></pre>", escapeHTML(output)))
	html.WriteString("<button class=\"copy-btn\" onclick=\"copyCode(this)\">Copy</button>")

	html.WriteString("</div>")
	return html.String()
}

// YAMLHandler handles yaml:{data} queries
type YAMLHandler struct{}

// NewYAMLHandler creates a new YAML handler
func NewYAMLHandler() *YAMLHandler {
	return &YAMLHandler{}
}

func (h *YAMLHandler) Type() AnswerType {
	return AnswerTypeYAML
}

func (h *YAMLHandler) Handle(ctx context.Context, term string) (*Answer, error) {
	term = strings.TrimSpace(term)
	if term == "" {
		return nil, fmt.Errorf("YAML data required")
	}

	// Check for mode prefix
	mode := "format" // default
	yamlStr := term

	if strings.HasPrefix(strings.ToLower(term), "to-json ") || strings.HasPrefix(strings.ToLower(term), "tojson ") {
		mode = "to-json"
		yamlStr = strings.TrimPrefix(term, "to-json ")
		yamlStr = strings.TrimPrefix(yamlStr, "tojson ")
		yamlStr = strings.TrimPrefix(yamlStr, "To-json ")
		yamlStr = strings.TrimPrefix(yamlStr, "Tojson ")
	} else if strings.HasPrefix(strings.ToLower(term), "from-json ") || strings.HasPrefix(strings.ToLower(term), "fromjson ") {
		mode = "from-json"
		yamlStr = strings.TrimPrefix(term, "from-json ")
		yamlStr = strings.TrimPrefix(yamlStr, "fromjson ")
		yamlStr = strings.TrimPrefix(yamlStr, "From-json ")
		yamlStr = strings.TrimPrefix(yamlStr, "Fromjson ")
	}

	var output string
	var parsed interface{}

	switch mode {
	case "from-json":
		// Convert JSON to YAML
		if err := json.Unmarshal([]byte(yamlStr), &parsed); err != nil {
			return &Answer{
				Type:        AnswerTypeYAML,
				Term:        term,
				Title:       "YAML Error",
				Description: "Invalid JSON input",
				Content:     fmt.Sprintf("<p class=\"error\">Invalid JSON: %s</p>", escapeHTML(err.Error())),
				Error:       "invalid_json",
			}, nil
		}
		b, _ := yaml.Marshal(parsed)
		output = string(b)

	case "to-json":
		// Convert YAML to JSON
		if err := yaml.Unmarshal([]byte(yamlStr), &parsed); err != nil {
			return &Answer{
				Type:        AnswerTypeYAML,
				Term:        term,
				Title:       "YAML Error",
				Description: "Invalid YAML input",
				Content:     fmt.Sprintf("<p class=\"error\">Invalid YAML: %s</p>", escapeHTML(err.Error())),
				Error:       "invalid_yaml",
			}, nil
		}
		b, _ := json.MarshalIndent(parsed, "", "  ")
		output = string(b)

	default:
		// Format/validate YAML
		if err := yaml.Unmarshal([]byte(yamlStr), &parsed); err != nil {
			return &Answer{
				Type:        AnswerTypeYAML,
				Term:        term,
				Title:       "YAML Error",
				Description: "Invalid YAML",
				Content:     fmt.Sprintf("<p class=\"error\">Invalid YAML: %s</p>", escapeHTML(err.Error())),
				Error:       "invalid_yaml",
			}, nil
		}
		b, _ := yaml.Marshal(parsed)
		output = string(b)
	}

	data := map[string]interface{}{
		"input":  yamlStr,
		"output": output,
		"mode":   mode,
		"valid":  true,
	}

	return &Answer{
		Type:        AnswerTypeYAML,
		Term:        term,
		Title:       fmt.Sprintf("YAML %s", formatYAMLMode(mode)),
		Description: "Valid YAML",
		Content:     formatYAMLContent(mode, yamlStr, output),
		Source:      "YAML Parser",
		Data:        data,
	}, nil
}

func formatYAMLMode(mode string) string {
	switch mode {
	case "to-json":
		return "to JSON"
	case "from-json":
		return "from JSON"
	default:
		return "Format"
	}
}

func formatYAMLContent(mode, input, output string) string {
	var html strings.Builder
	html.WriteString("<div class=\"yaml-content\">")
	html.WriteString(fmt.Sprintf("<h1>YAML %s</h1>", formatYAMLMode(mode)))

	html.WriteString("<p class=\"valid\">✓ Valid</p>")

	html.WriteString("<h2>Output</h2>")
	html.WriteString(fmt.Sprintf("<pre class=\"yaml-output\"><code>%s</code></pre>", escapeHTML(output)))
	html.WriteString("<button class=\"copy-btn\" onclick=\"copyCode(this)\">Copy</button>")

	html.WriteString("</div>")
	return html.String()
}
