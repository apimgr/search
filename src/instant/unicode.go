package instant

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// UnicodeHandler handles unicode character lookups
type UnicodeHandler struct {
	patterns []*regexp.Regexp
}

// NewUnicodeHandler creates a new unicode handler
func NewUnicodeHandler() *UnicodeHandler {
	return &UnicodeHandler{
		patterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)^unicode[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^char(?:acter)?[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^codepoint[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^u\+([0-9a-fA-F]{4,6})$`),
		},
	}
}

func (h *UnicodeHandler) Name() string {
	return "unicode"
}

func (h *UnicodeHandler) Patterns() []*regexp.Regexp {
	return h.patterns
}

func (h *UnicodeHandler) CanHandle(query string) bool {
	for _, p := range h.patterns {
		if p.MatchString(query) {
			return true
		}
	}
	return false
}

func (h *UnicodeHandler) Handle(ctx context.Context, query string) (*Answer, error) {
	var input string
	for _, p := range h.patterns {
		if matches := p.FindStringSubmatch(query); len(matches) > 1 {
			input = strings.TrimSpace(matches[1])
			break
		}
	}

	if input == "" {
		return nil, nil
	}

	// Check if it's a U+XXXX code point
	var r rune
	codePointPattern := regexp.MustCompile(`(?i)^U?\+?([0-9a-fA-F]{4,6})$`)
	if matches := codePointPattern.FindStringSubmatch(input); len(matches) > 1 {
		var codePoint int
		fmt.Sscanf(matches[1], "%x", &codePoint)
		r = rune(codePoint)
	} else {
		// Use the first character of input
		r, _ = utf8.DecodeRuneInString(input)
		if r == utf8.RuneError {
			return &Answer{
				Type:    AnswerTypeUnicode,
				Query:   query,
				Title:   "Unicode Character Info",
				Content: "Invalid character or encoding",
			}, nil
		}
	}

	// Get character info
	charStr := string(r)
	codePoint := fmt.Sprintf("U+%04X", r)
	utf8Bytes := []byte(charStr)
	utf8Hex := make([]string, len(utf8Bytes))
	for i, b := range utf8Bytes {
		utf8Hex[i] = fmt.Sprintf("%02X", b)
	}

	// Get Unicode block name
	blockName := getUnicodeBlock(r)

	// Get category
	category := getUnicodeCategory(r)

	// Get character name (simplified)
	charName := getCharacterName(r)

	var content strings.Builder
	content.WriteString(fmt.Sprintf(`<div class="unicode-result">
<div class="char-display" style="font-size: 48px; text-align: center; padding: 20px;">%s</div>
<table class="unicode-table">
<tr><td><strong>Character:</strong></td><td>%s</td></tr>
<tr><td><strong>Code Point:</strong></td><td>%s (decimal: %d)</td></tr>
<tr><td><strong>UTF-8 Bytes:</strong></td><td>%s (%d bytes)</td></tr>
<tr><td><strong>UTF-16:</strong></td><td>%s</td></tr>
<tr><td><strong>Name:</strong></td><td>%s</td></tr>
<tr><td><strong>Block:</strong></td><td>%s</td></tr>
<tr><td><strong>Category:</strong></td><td>%s</td></tr>
</table>
</div>`,
		charStr,
		charStr,
		codePoint, r,
		strings.Join(utf8Hex, " "), len(utf8Bytes),
		getUTF16Encoding(r),
		charName,
		blockName,
		category))

	return &Answer{
		Type:    AnswerTypeUnicode,
		Query:   query,
		Title:   fmt.Sprintf("Unicode: %s", codePoint),
		Content: content.String(),
		Data: map[string]interface{}{
			"character":  charStr,
			"codePoint":  codePoint,
			"decimal":    int(r),
			"utf8Bytes":  utf8Hex,
			"name":       charName,
			"block":      blockName,
			"category":   category,
		},
	}, nil
}

// getUnicodeBlock returns the Unicode block name for a rune
func getUnicodeBlock(r rune) string {
	blocks := []struct {
		start, end rune
		name       string
	}{
		{0x0000, 0x007F, "Basic Latin"},
		{0x0080, 0x00FF, "Latin-1 Supplement"},
		{0x0100, 0x017F, "Latin Extended-A"},
		{0x0180, 0x024F, "Latin Extended-B"},
		{0x0250, 0x02AF, "IPA Extensions"},
		{0x0300, 0x036F, "Combining Diacritical Marks"},
		{0x0370, 0x03FF, "Greek and Coptic"},
		{0x0400, 0x04FF, "Cyrillic"},
		{0x0500, 0x052F, "Cyrillic Supplement"},
		{0x0530, 0x058F, "Armenian"},
		{0x0590, 0x05FF, "Hebrew"},
		{0x0600, 0x06FF, "Arabic"},
		{0x0900, 0x097F, "Devanagari"},
		{0x0980, 0x09FF, "Bengali"},
		{0x0A00, 0x0A7F, "Gurmukhi"},
		{0x0A80, 0x0AFF, "Gujarati"},
		{0x0B00, 0x0B7F, "Oriya"},
		{0x0B80, 0x0BFF, "Tamil"},
		{0x0C00, 0x0C7F, "Telugu"},
		{0x0C80, 0x0CFF, "Kannada"},
		{0x0D00, 0x0D7F, "Malayalam"},
		{0x0E00, 0x0E7F, "Thai"},
		{0x0E80, 0x0EFF, "Lao"},
		{0x1000, 0x109F, "Myanmar"},
		{0x10A0, 0x10FF, "Georgian"},
		{0x1100, 0x11FF, "Hangul Jamo"},
		{0x1E00, 0x1EFF, "Latin Extended Additional"},
		{0x1F00, 0x1FFF, "Greek Extended"},
		{0x2000, 0x206F, "General Punctuation"},
		{0x2070, 0x209F, "Superscripts and Subscripts"},
		{0x20A0, 0x20CF, "Currency Symbols"},
		{0x20D0, 0x20FF, "Combining Diacritical Marks for Symbols"},
		{0x2100, 0x214F, "Letterlike Symbols"},
		{0x2150, 0x218F, "Number Forms"},
		{0x2190, 0x21FF, "Arrows"},
		{0x2200, 0x22FF, "Mathematical Operators"},
		{0x2300, 0x23FF, "Miscellaneous Technical"},
		{0x2400, 0x243F, "Control Pictures"},
		{0x2500, 0x257F, "Box Drawing"},
		{0x2580, 0x259F, "Block Elements"},
		{0x25A0, 0x25FF, "Geometric Shapes"},
		{0x2600, 0x26FF, "Miscellaneous Symbols"},
		{0x2700, 0x27BF, "Dingbats"},
		{0x2800, 0x28FF, "Braille Patterns"},
		{0x2E80, 0x2EFF, "CJK Radicals Supplement"},
		{0x3000, 0x303F, "CJK Symbols and Punctuation"},
		{0x3040, 0x309F, "Hiragana"},
		{0x30A0, 0x30FF, "Katakana"},
		{0x3100, 0x312F, "Bopomofo"},
		{0x3130, 0x318F, "Hangul Compatibility Jamo"},
		{0x3200, 0x32FF, "Enclosed CJK Letters and Months"},
		{0x3300, 0x33FF, "CJK Compatibility"},
		{0x4E00, 0x9FFF, "CJK Unified Ideographs"},
		{0xAC00, 0xD7AF, "Hangul Syllables"},
		{0xE000, 0xF8FF, "Private Use Area"},
		{0xFB00, 0xFB4F, "Alphabetic Presentation Forms"},
		{0xFB50, 0xFDFF, "Arabic Presentation Forms-A"},
		{0xFE00, 0xFE0F, "Variation Selectors"},
		{0xFE20, 0xFE2F, "Combining Half Marks"},
		{0xFE30, 0xFE4F, "CJK Compatibility Forms"},
		{0xFE50, 0xFE6F, "Small Form Variants"},
		{0xFE70, 0xFEFF, "Arabic Presentation Forms-B"},
		{0xFF00, 0xFFEF, "Halfwidth and Fullwidth Forms"},
		{0x1F300, 0x1F5FF, "Miscellaneous Symbols and Pictographs"},
		{0x1F600, 0x1F64F, "Emoticons"},
		{0x1F680, 0x1F6FF, "Transport and Map Symbols"},
		{0x1F900, 0x1F9FF, "Supplemental Symbols and Pictographs"},
	}

	for _, block := range blocks {
		if r >= block.start && r <= block.end {
			return block.name
		}
	}
	return "Unknown"
}

// getUnicodeCategory returns the Unicode category for a rune
func getUnicodeCategory(r rune) string {
	switch {
	case unicode.IsLetter(r):
		if unicode.IsUpper(r) {
			return "Letter, Uppercase (Lu)"
		} else if unicode.IsLower(r) {
			return "Letter, Lowercase (Ll)"
		} else if unicode.IsTitle(r) {
			return "Letter, Titlecase (Lt)"
		}
		return "Letter (L)"
	case unicode.IsDigit(r):
		return "Number, Digit (Nd)"
	case unicode.IsNumber(r):
		return "Number (N)"
	case unicode.IsPunct(r):
		return "Punctuation (P)"
	case unicode.IsSymbol(r):
		return "Symbol (S)"
	case unicode.IsMark(r):
		return "Mark (M)"
	case unicode.IsSpace(r):
		return "Separator, Space (Zs)"
	case unicode.IsControl(r):
		return "Control (Cc)"
	default:
		return "Other"
	}
}

// getCharacterName returns a descriptive name for common characters
func getCharacterName(r rune) string {
	// Common character names
	names := map[rune]string{
		' ':    "SPACE",
		'!':    "EXCLAMATION MARK",
		'"':    "QUOTATION MARK",
		'#':    "NUMBER SIGN",
		'$':    "DOLLAR SIGN",
		'%':    "PERCENT SIGN",
		'&':    "AMPERSAND",
		'\'':   "APOSTROPHE",
		'(':    "LEFT PARENTHESIS",
		')':    "RIGHT PARENTHESIS",
		'*':    "ASTERISK",
		'+':    "PLUS SIGN",
		',':    "COMMA",
		'-':    "HYPHEN-MINUS",
		'.':    "FULL STOP",
		'/':    "SOLIDUS",
		':':    "COLON",
		';':    "SEMICOLON",
		'<':    "LESS-THAN SIGN",
		'=':    "EQUALS SIGN",
		'>':    "GREATER-THAN SIGN",
		'?':    "QUESTION MARK",
		'@':    "COMMERCIAL AT",
		'[':    "LEFT SQUARE BRACKET",
		'\\':   "REVERSE SOLIDUS",
		']':    "RIGHT SQUARE BRACKET",
		'^':    "CIRCUMFLEX ACCENT",
		'_':    "LOW LINE",
		'`':    "GRAVE ACCENT",
		'{':    "LEFT CURLY BRACKET",
		'|':    "VERTICAL LINE",
		'}':    "RIGHT CURLY BRACKET",
		'~':    "TILDE",
		'\t':   "CHARACTER TABULATION",
		'\n':   "LINE FEED",
		'\r':   "CARRIAGE RETURN",
		0x00A0: "NO-BREAK SPACE",
		0x00A9: "COPYRIGHT SIGN",
		0x00AE: "REGISTERED SIGN",
		0x00B0: "DEGREE SIGN",
		0x00B1: "PLUS-MINUS SIGN",
		0x00B7: "MIDDLE DOT",
		0x00D7: "MULTIPLICATION SIGN",
		0x00F7: "DIVISION SIGN",
		0x2013: "EN DASH",
		0x2014: "EM DASH",
		0x2018: "LEFT SINGLE QUOTATION MARK",
		0x2019: "RIGHT SINGLE QUOTATION MARK",
		0x201C: "LEFT DOUBLE QUOTATION MARK",
		0x201D: "RIGHT DOUBLE QUOTATION MARK",
		0x2022: "BULLET",
		0x2026: "HORIZONTAL ELLIPSIS",
		0x20AC: "EURO SIGN",
		0x2122: "TRADE MARK SIGN",
		0x2190: "LEFTWARDS ARROW",
		0x2191: "UPWARDS ARROW",
		0x2192: "RIGHTWARDS ARROW",
		0x2193: "DOWNWARDS ARROW",
		0x2194: "LEFT RIGHT ARROW",
		0x2195: "UP DOWN ARROW",
		0x2212: "MINUS SIGN",
		0x221E: "INFINITY",
		0x2260: "NOT EQUAL TO",
		0x2264: "LESS-THAN OR EQUAL TO",
		0x2265: "GREATER-THAN OR EQUAL TO",
		0x2714: "HEAVY CHECK MARK",
		0x2716: "HEAVY MULTIPLICATION X",
		0x2764: "HEAVY BLACK HEART",
	}

	if name, ok := names[r]; ok {
		return name
	}

	// Generate name for letters and digits
	if r >= 'A' && r <= 'Z' {
		return fmt.Sprintf("LATIN CAPITAL LETTER %c", r)
	}
	if r >= 'a' && r <= 'z' {
		return fmt.Sprintf("LATIN SMALL LETTER %c", r-32)
	}
	if r >= '0' && r <= '9' {
		return fmt.Sprintf("DIGIT %c", r)
	}

	return fmt.Sprintf("CHARACTER U+%04X", r)
}

// getUTF16Encoding returns UTF-16 encoding representation
func getUTF16Encoding(r rune) string {
	if r <= 0xFFFF {
		return fmt.Sprintf("0x%04X", r)
	}
	// Surrogate pair for characters above BMP
	r -= 0x10000
	high := 0xD800 + (r >> 10)
	low := 0xDC00 + (r & 0x3FF)
	return fmt.Sprintf("0x%04X 0x%04X", high, low)
}
