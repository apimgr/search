package instant

import (
	"context"
	"fmt"
	"html"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

// HTMLEntityHandler handles HTML entity encoding/decoding
type HTMLEntityHandler struct {
	patterns []*regexp.Regexp
	entities map[string]string // name -> character
	reverse  map[rune]string   // character -> name
}

// NewHTMLEntityHandler creates a new HTML entity handler
func NewHTMLEntityHandler() *HTMLEntityHandler {
	h := &HTMLEntityHandler{
		patterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)^html\s+encode[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^html\s+decode[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^html\s+entity[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^html[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^entity[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^encode\s+html[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^decode\s+html[:\s]+(.+)$`),
		},
	}

	// Initialize common HTML entities
	h.entities = getHTMLEntities()
	h.reverse = make(map[rune]string)
	for name, char := range h.entities {
		r, _ := utf8.DecodeRuneInString(char)
		if _, exists := h.reverse[r]; !exists {
			h.reverse[r] = name
		}
	}

	return h
}

func (h *HTMLEntityHandler) Name() string {
	return "html_entity"
}

func (h *HTMLEntityHandler) Patterns() []*regexp.Regexp {
	return h.patterns
}

func (h *HTMLEntityHandler) CanHandle(query string) bool {
	for _, p := range h.patterns {
		if p.MatchString(query) {
			return true
		}
	}
	return false
}

func (h *HTMLEntityHandler) Handle(ctx context.Context, query string) (*Answer, error) {
	lowerQuery := strings.ToLower(query)
	isDecode := strings.Contains(lowerQuery, "decode")

	var text string
	for _, p := range h.patterns {
		if matches := p.FindStringSubmatch(query); len(matches) > 1 {
			text = strings.TrimSpace(matches[1])
			break
		}
	}

	if text == "" {
		return nil, nil
	}

	var content strings.Builder
	content.WriteString(fmt.Sprintf("<div class=\"html-entity-result\">\n"))
	content.WriteString(fmt.Sprintf("<strong>Input:</strong> <code>%s</code><br><br>", html.EscapeString(text)))

	// Auto-detect: if text contains &...; patterns or numeric entities, decode; otherwise encode
	hasEntities := strings.Contains(text, "&") && (strings.Contains(text, ";") || regexp.MustCompile(`&#\d+`).MatchString(text))
	if !strings.Contains(lowerQuery, "encode") && !strings.Contains(lowerQuery, "decode") {
		// Auto-detect mode
		isDecode = hasEntities
	}

	if isDecode {
		decoded := h.decodeHTMLEntities(text)
		content.WriteString(fmt.Sprintf("<strong>Decoded:</strong> <code>%s</code><br><br>", html.EscapeString(decoded)))

		// Show individual entity breakdown
		entities := h.extractEntities(text)
		if len(entities) > 0 {
			content.WriteString("<strong>Entities found:</strong><br>")
			content.WriteString("<table class=\"entity-table\" style=\"border-collapse: collapse;\">")
			content.WriteString("<tr><th style=\"padding: 5px; border: 1px solid #ddd;\">Entity</th><th style=\"padding: 5px; border: 1px solid #ddd;\">Character</th><th style=\"padding: 5px; border: 1px solid #ddd;\">Code Point</th></tr>")
			for _, ent := range entities {
				decoded := h.decodeHTMLEntities(ent)
				r, _ := utf8.DecodeRuneInString(decoded)
				content.WriteString(fmt.Sprintf("<tr><td style=\"padding: 5px; border: 1px solid #ddd;\"><code>%s</code></td><td style=\"padding: 5px; border: 1px solid #ddd;\">%s</td><td style=\"padding: 5px; border: 1px solid #ddd;\">U+%04X</td></tr>",
					html.EscapeString(ent), html.EscapeString(decoded), r))
			}
			content.WriteString("</table>")
		}

		return &Answer{
			Type:    AnswerTypeHTMLEntity,
			Query:   query,
			Title:   "HTML Entity Decoder",
			Content: content.String(),
			Data: map[string]interface{}{
				"input":     text,
				"decoded":   decoded,
				"operation": "decode",
			},
		}, nil
	}

	// Encode mode
	encoded := html.EscapeString(text)
	fullEncoded := h.encodeAllChars(text)

	content.WriteString(fmt.Sprintf("<strong>Basic Encoded:</strong> <code>%s</code><br><br>", html.EscapeString(encoded)))
	content.WriteString(fmt.Sprintf("<strong>Full Encoded (numeric):</strong> <code>%s</code><br><br>", html.EscapeString(fullEncoded)))

	// Show character breakdown
	content.WriteString("<strong>Character breakdown:</strong><br>")
	content.WriteString("<table class=\"entity-table\" style=\"border-collapse: collapse;\">")
	content.WriteString("<tr><th style=\"padding: 5px; border: 1px solid #ddd;\">Char</th><th style=\"padding: 5px; border: 1px solid #ddd;\">Named Entity</th><th style=\"padding: 5px; border: 1px solid #ddd;\">Numeric Entity</th><th style=\"padding: 5px; border: 1px solid #ddd;\">Hex Entity</th></tr>")

	for _, r := range text {
		namedEntity := "-"
		if name, ok := h.reverse[r]; ok {
			namedEntity = fmt.Sprintf("&%s;", name)
		}
		content.WriteString(fmt.Sprintf("<tr><td style=\"padding: 5px; border: 1px solid #ddd;\">%s</td><td style=\"padding: 5px; border: 1px solid #ddd;\"><code>%s</code></td><td style=\"padding: 5px; border: 1px solid #ddd;\"><code>&#%d;</code></td><td style=\"padding: 5px; border: 1px solid #ddd;\"><code>&#x%X;</code></td></tr>",
			html.EscapeString(string(r)), namedEntity, r, r))
	}
	content.WriteString("</table>")
	content.WriteString("</div>")

	return &Answer{
		Type:    AnswerTypeHTMLEntity,
		Query:   query,
		Title:   "HTML Entity Encoder",
		Content: content.String(),
		Data: map[string]interface{}{
			"input":       text,
			"encoded":     encoded,
			"fullEncoded": fullEncoded,
			"operation":   "encode",
		},
	}, nil
}

// decodeHTMLEntities decodes all HTML entities in the text
func (h *HTMLEntityHandler) decodeHTMLEntities(text string) string {
	// First use standard library for basic decoding
	result := html.UnescapeString(text)

	// Handle additional named entities not covered by standard library
	for name, char := range h.entities {
		result = strings.ReplaceAll(result, "&"+name+";", char)
	}

	// Handle numeric entities (decimal)
	numericPattern := regexp.MustCompile(`&#(\d+);`)
	result = numericPattern.ReplaceAllStringFunc(result, func(match string) string {
		matches := numericPattern.FindStringSubmatch(match)
		if len(matches) > 1 {
			code, err := strconv.ParseInt(matches[1], 10, 32)
			if err == nil {
				return string(rune(code))
			}
		}
		return match
	})

	// Handle hex entities
	hexPattern := regexp.MustCompile(`&#[xX]([0-9a-fA-F]+);`)
	result = hexPattern.ReplaceAllStringFunc(result, func(match string) string {
		matches := hexPattern.FindStringSubmatch(match)
		if len(matches) > 1 {
			code, err := strconv.ParseInt(matches[1], 16, 32)
			if err == nil {
				return string(rune(code))
			}
		}
		return match
	})

	return result
}

// encodeAllChars encodes all characters as numeric entities
func (h *HTMLEntityHandler) encodeAllChars(text string) string {
	var result strings.Builder
	for _, r := range text {
		if r > 127 || r == '<' || r == '>' || r == '&' || r == '"' || r == '\'' {
			result.WriteString(fmt.Sprintf("&#%d;", r))
		} else {
			result.WriteRune(r)
		}
	}
	return result.String()
}

// extractEntities extracts all HTML entities from text
func (h *HTMLEntityHandler) extractEntities(text string) []string {
	pattern := regexp.MustCompile(`&(?:#[xX]?[0-9a-fA-F]+|[a-zA-Z]+);?`)
	return pattern.FindAllString(text, -1)
}

// getHTMLEntities returns common HTML entities
func getHTMLEntities() map[string]string {
	return map[string]string{
		// Special characters
		"quot":     "\"",
		"amp":      "&",
		"apos":     "'",
		"lt":       "<",
		"gt":       ">",
		"nbsp":     "\u00A0",
		"iexcl":    "\u00A1",
		"cent":     "\u00A2",
		"pound":    "\u00A3",
		"curren":   "\u00A4",
		"yen":      "\u00A5",
		"brvbar":   "\u00A6",
		"sect":     "\u00A7",
		"uml":      "\u00A8",
		"copy":     "\u00A9",
		"ordf":     "\u00AA",
		"laquo":    "\u00AB",
		"not":      "\u00AC",
		"shy":      "\u00AD",
		"reg":      "\u00AE",
		"macr":     "\u00AF",
		"deg":      "\u00B0",
		"plusmn":   "\u00B1",
		"sup2":     "\u00B2",
		"sup3":     "\u00B3",
		"acute":    "\u00B4",
		"micro":    "\u00B5",
		"para":     "\u00B6",
		"middot":   "\u00B7",
		"cedil":    "\u00B8",
		"sup1":     "\u00B9",
		"ordm":     "\u00BA",
		"raquo":    "\u00BB",
		"frac14":   "\u00BC",
		"frac12":   "\u00BD",
		"frac34":   "\u00BE",
		"iquest":   "\u00BF",

		// Latin uppercase letters with accents
		"Agrave":   "\u00C0",
		"Aacute":   "\u00C1",
		"Acirc":    "\u00C2",
		"Atilde":   "\u00C3",
		"Auml":     "\u00C4",
		"Aring":    "\u00C5",
		"AElig":    "\u00C6",
		"Ccedil":   "\u00C7",
		"Egrave":   "\u00C8",
		"Eacute":   "\u00C9",
		"Ecirc":    "\u00CA",
		"Euml":     "\u00CB",
		"Igrave":   "\u00CC",
		"Iacute":   "\u00CD",
		"Icirc":    "\u00CE",
		"Iuml":     "\u00CF",
		"ETH":      "\u00D0",
		"Ntilde":   "\u00D1",
		"Ograve":   "\u00D2",
		"Oacute":   "\u00D3",
		"Ocirc":    "\u00D4",
		"Otilde":   "\u00D5",
		"Ouml":     "\u00D6",
		"times":    "\u00D7",
		"Oslash":   "\u00D8",
		"Ugrave":   "\u00D9",
		"Uacute":   "\u00DA",
		"Ucirc":    "\u00DB",
		"Uuml":     "\u00DC",
		"Yacute":   "\u00DD",
		"THORN":    "\u00DE",
		"szlig":    "\u00DF",

		// Latin lowercase letters with accents
		"agrave":   "\u00E0",
		"aacute":   "\u00E1",
		"acirc":    "\u00E2",
		"atilde":   "\u00E3",
		"auml":     "\u00E4",
		"aring":    "\u00E5",
		"aelig":    "\u00E6",
		"ccedil":   "\u00E7",
		"egrave":   "\u00E8",
		"eacute":   "\u00E9",
		"ecirc":    "\u00EA",
		"euml":     "\u00EB",
		"igrave":   "\u00EC",
		"iacute":   "\u00ED",
		"icirc":    "\u00EE",
		"iuml":     "\u00EF",
		"eth":      "\u00F0",
		"ntilde":   "\u00F1",
		"ograve":   "\u00F2",
		"oacute":   "\u00F3",
		"ocirc":    "\u00F4",
		"otilde":   "\u00F5",
		"ouml":     "\u00F6",
		"divide":   "\u00F7",
		"oslash":   "\u00F8",
		"ugrave":   "\u00F9",
		"uacute":   "\u00FA",
		"ucirc":    "\u00FB",
		"uuml":     "\u00FC",
		"yacute":   "\u00FD",
		"thorn":    "\u00FE",
		"yuml":     "\u00FF",

		// Greek letters
		"Alpha":    "\u0391",
		"Beta":     "\u0392",
		"Gamma":    "\u0393",
		"Delta":    "\u0394",
		"Epsilon":  "\u0395",
		"Zeta":     "\u0396",
		"Eta":      "\u0397",
		"Theta":    "\u0398",
		"Iota":     "\u0399",
		"Kappa":    "\u039A",
		"Lambda":   "\u039B",
		"Mu":       "\u039C",
		"Nu":       "\u039D",
		"Xi":       "\u039E",
		"Omicron":  "\u039F",
		"Pi":       "\u03A0",
		"Rho":      "\u03A1",
		"Sigma":    "\u03A3",
		"Tau":      "\u03A4",
		"Upsilon":  "\u03A5",
		"Phi":      "\u03A6",
		"Chi":      "\u03A7",
		"Psi":      "\u03A8",
		"Omega":    "\u03A9",
		"alpha":    "\u03B1",
		"beta":     "\u03B2",
		"gamma":    "\u03B3",
		"delta":    "\u03B4",
		"epsilon":  "\u03B5",
		"zeta":     "\u03B6",
		"eta":      "\u03B7",
		"theta":    "\u03B8",
		"iota":     "\u03B9",
		"kappa":    "\u03BA",
		"lambda":   "\u03BB",
		"mu":       "\u03BC",
		"nu":       "\u03BD",
		"xi":       "\u03BE",
		"omicron":  "\u03BF",
		"pi":       "\u03C0",
		"rho":      "\u03C1",
		"sigmaf":   "\u03C2",
		"sigma":    "\u03C3",
		"tau":      "\u03C4",
		"upsilon":  "\u03C5",
		"phi":      "\u03C6",
		"chi":      "\u03C7",
		"psi":      "\u03C8",
		"omega":    "\u03C9",

		// Math symbols
		"forall":   "\u2200",
		"part":     "\u2202",
		"exist":    "\u2203",
		"empty":    "\u2205",
		"nabla":    "\u2207",
		"isin":     "\u2208",
		"notin":    "\u2209",
		"ni":       "\u220B",
		"prod":     "\u220F",
		"sum":      "\u2211",
		"minus":    "\u2212",
		"lowast":   "\u2217",
		"radic":    "\u221A",
		"prop":     "\u221D",
		"infin":    "\u221E",
		"ang":      "\u2220",
		"and":      "\u2227",
		"or":       "\u2228",
		"cap":      "\u2229",
		"cup":      "\u222A",
		"int":      "\u222B",
		"there4":   "\u2234",
		"sim":      "\u223C",
		"cong":     "\u2245",
		"asymp":    "\u2248",
		"ne":       "\u2260",
		"equiv":    "\u2261",
		"le":       "\u2264",
		"ge":       "\u2265",
		"sub":      "\u2282",
		"sup":      "\u2283",
		"nsub":     "\u2284",
		"sube":     "\u2286",
		"supe":     "\u2287",
		"oplus":    "\u2295",
		"otimes":   "\u2297",
		"perp":     "\u22A5",
		"sdot":     "\u22C5",

		// Arrows
		"larr":     "\u2190",
		"uarr":     "\u2191",
		"rarr":     "\u2192",
		"darr":     "\u2193",
		"harr":     "\u2194",
		"crarr":    "\u21B5",
		"lArr":     "\u21D0",
		"uArr":     "\u21D1",
		"rArr":     "\u21D2",
		"dArr":     "\u21D3",
		"hArr":     "\u21D4",

		// Miscellaneous symbols
		"bull":     "\u2022",
		"hellip":   "\u2026",
		"prime":    "\u2032",
		"Prime":    "\u2033",
		"oline":    "\u203E",
		"frasl":    "\u2044",
		"weierp":   "\u2118",
		"image":    "\u2111",
		"real":     "\u211C",
		"trade":    "\u2122",
		"alefsym":  "\u2135",
		"spades":   "\u2660",
		"clubs":    "\u2663",
		"hearts":   "\u2665",
		"diams":    "\u2666",

		// Quotation marks
		"lsquo":    "\u2018",
		"rsquo":    "\u2019",
		"sbquo":    "\u201A",
		"ldquo":    "\u201C",
		"rdquo":    "\u201D",
		"bdquo":    "\u201E",
		"dagger":   "\u2020",
		"Dagger":   "\u2021",
		"permil":   "\u2030",
		"lsaquo":   "\u2039",
		"rsaquo":   "\u203A",
		"euro":     "\u20AC",

		// Dashes
		"ndash":    "\u2013",
		"mdash":    "\u2014",

		// Other common entities
		"ensp":     "\u2002",
		"emsp":     "\u2003",
		"thinsp":   "\u2009",
		"zwnj":     "\u200C",
		"zwj":      "\u200D",
		"lrm":      "\u200E",
		"rlm":      "\u200F",
	}
}
