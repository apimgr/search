package instant

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

// ASCIIHandler handles ASCII art text generation
type ASCIIHandler struct {
	patterns []*regexp.Regexp
	font     map[rune][]string
}

// NewASCIIHandler creates a new ASCII art handler
func NewASCIIHandler() *ASCIIHandler {
	return &ASCIIHandler{
		patterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)^ascii[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^ascii\s+art[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^figlet[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^banner[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^text\s+art[:\s]+(.+)$`),
		},
		font: buildASCIIFont(),
	}
}

func (h *ASCIIHandler) Name() string {
	return "ascii"
}

func (h *ASCIIHandler) Patterns() []*regexp.Regexp {
	return h.patterns
}

func (h *ASCIIHandler) CanHandle(query string) bool {
	for _, p := range h.patterns {
		if p.MatchString(query) {
			return true
		}
	}
	return false
}

func (h *ASCIIHandler) Handle(ctx context.Context, query string) (*Answer, error) {
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

	// Generate ASCII art
	art := h.generateASCII(strings.ToUpper(text))

	content := fmt.Sprintf(`<div class="ascii-result">
<strong>Input:</strong> %s<br><br>
<pre style="font-family: monospace; line-height: 1.2;">%s</pre>
</div>`, text, art)

	return &Answer{
		Type:    AnswerTypeASCII,
		Query:   query,
		Title:   "ASCII Art Generator",
		Content: content,
		Data: map[string]interface{}{
			"input": text,
			"art":   art,
		},
	}, nil
}

// generateASCII creates ASCII art from text
func (h *ASCIIHandler) generateASCII(text string) string {
	if len(text) == 0 {
		return ""
	}

	height := 6 // Each character is 6 lines tall
	lines := make([]strings.Builder, height)

	for _, char := range text {
		charArt, ok := h.font[char]
		if !ok {
			// Use space for unknown characters
			charArt = h.font[' ']
		}

		for i := 0; i < height; i++ {
			if i < len(charArt) {
				lines[i].WriteString(charArt[i])
			}
			lines[i].WriteString(" ") // Space between characters
		}
	}

	var result strings.Builder
	for i := 0; i < height; i++ {
		result.WriteString(lines[i].String())
		result.WriteString("\n")
	}

	return result.String()
}

// buildASCIIFont creates a simple block font
func buildASCIIFont() map[rune][]string {
	return map[rune][]string{
		'A': {
			"  ###  ",
			" #   # ",
			"#     #",
			"#######",
			"#     #",
			"#     #",
		},
		'B': {
			"###### ",
			"#     #",
			"###### ",
			"#     #",
			"#     #",
			"###### ",
		},
		'C': {
			" ##### ",
			"#     #",
			"#      ",
			"#      ",
			"#     #",
			" ##### ",
		},
		'D': {
			"###### ",
			"#     #",
			"#     #",
			"#     #",
			"#     #",
			"###### ",
		},
		'E': {
			"#######",
			"#      ",
			"#####  ",
			"#      ",
			"#      ",
			"#######",
		},
		'F': {
			"#######",
			"#      ",
			"#####  ",
			"#      ",
			"#      ",
			"#      ",
		},
		'G': {
			" ##### ",
			"#     #",
			"#      ",
			"#  ####",
			"#     #",
			" ##### ",
		},
		'H': {
			"#     #",
			"#     #",
			"#######",
			"#     #",
			"#     #",
			"#     #",
		},
		'I': {
			"#######",
			"   #   ",
			"   #   ",
			"   #   ",
			"   #   ",
			"#######",
		},
		'J': {
			"    ###",
			"      #",
			"      #",
			"      #",
			"#     #",
			" ##### ",
		},
		'K': {
			"#    # ",
			"#   #  ",
			"####   ",
			"#   #  ",
			"#    # ",
			"#     #",
		},
		'L': {
			"#      ",
			"#      ",
			"#      ",
			"#      ",
			"#      ",
			"#######",
		},
		'M': {
			"#     #",
			"##   ##",
			"# # # #",
			"#  #  #",
			"#     #",
			"#     #",
		},
		'N': {
			"#     #",
			"##    #",
			"# #   #",
			"#  #  #",
			"#   # #",
			"#    ##",
		},
		'O': {
			" ##### ",
			"#     #",
			"#     #",
			"#     #",
			"#     #",
			" ##### ",
		},
		'P': {
			"###### ",
			"#     #",
			"###### ",
			"#      ",
			"#      ",
			"#      ",
		},
		'Q': {
			" ##### ",
			"#     #",
			"#     #",
			"#   # #",
			"#    # ",
			" #### #",
		},
		'R': {
			"###### ",
			"#     #",
			"###### ",
			"#   #  ",
			"#    # ",
			"#     #",
		},
		'S': {
			" ##### ",
			"#      ",
			" ##### ",
			"      #",
			"      #",
			" ##### ",
		},
		'T': {
			"#######",
			"   #   ",
			"   #   ",
			"   #   ",
			"   #   ",
			"   #   ",
		},
		'U': {
			"#     #",
			"#     #",
			"#     #",
			"#     #",
			"#     #",
			" ##### ",
		},
		'V': {
			"#     #",
			"#     #",
			"#     #",
			" #   # ",
			"  # #  ",
			"   #   ",
		},
		'W': {
			"#     #",
			"#     #",
			"#  #  #",
			"# # # #",
			"##   ##",
			"#     #",
		},
		'X': {
			"#     #",
			" #   # ",
			"  # #  ",
			"  # #  ",
			" #   # ",
			"#     #",
		},
		'Y': {
			"#     #",
			" #   # ",
			"  # #  ",
			"   #   ",
			"   #   ",
			"   #   ",
		},
		'Z': {
			"#######",
			"     # ",
			"    #  ",
			"   #   ",
			"  #    ",
			"#######",
		},
		'0': {
			" ##### ",
			"#    ##",
			"#   # #",
			"#  #  #",
			"# #   #",
			" ##### ",
		},
		'1': {
			"   #   ",
			"  ##   ",
			"   #   ",
			"   #   ",
			"   #   ",
			" ##### ",
		},
		'2': {
			" ##### ",
			"#     #",
			"     # ",
			"  ###  ",
			" #     ",
			"#######",
		},
		'3': {
			" ##### ",
			"#     #",
			"    ## ",
			"      #",
			"#     #",
			" ##### ",
		},
		'4': {
			"#     #",
			"#     #",
			"#######",
			"      #",
			"      #",
			"      #",
		},
		'5': {
			"#######",
			"#      ",
			"###### ",
			"      #",
			"#     #",
			" ##### ",
		},
		'6': {
			" ##### ",
			"#      ",
			"###### ",
			"#     #",
			"#     #",
			" ##### ",
		},
		'7': {
			"#######",
			"     # ",
			"    #  ",
			"   #   ",
			"  #    ",
			"  #    ",
		},
		'8': {
			" ##### ",
			"#     #",
			" ##### ",
			"#     #",
			"#     #",
			" ##### ",
		},
		'9': {
			" ##### ",
			"#     #",
			" ######",
			"      #",
			"#     #",
			" ##### ",
		},
		' ': {
			"       ",
			"       ",
			"       ",
			"       ",
			"       ",
			"       ",
		},
		'!': {
			"   #   ",
			"   #   ",
			"   #   ",
			"   #   ",
			"       ",
			"   #   ",
		},
		'?': {
			" ##### ",
			"#     #",
			"    ## ",
			"   #   ",
			"       ",
			"   #   ",
		},
		'.': {
			"       ",
			"       ",
			"       ",
			"       ",
			"       ",
			"   #   ",
		},
		',': {
			"       ",
			"       ",
			"       ",
			"       ",
			"   #   ",
			"  #    ",
		},
		'-': {
			"       ",
			"       ",
			"#######",
			"       ",
			"       ",
			"       ",
		},
		'_': {
			"       ",
			"       ",
			"       ",
			"       ",
			"       ",
			"#######",
		},
		'@': {
			" ##### ",
			"#     #",
			"# ### #",
			"# ### #",
			"#      ",
			" ##### ",
		},
		'#': {
			" # # # ",
			"#######",
			" # # # ",
			"#######",
			" # # # ",
			"       ",
		},
		':': {
			"       ",
			"   #   ",
			"       ",
			"       ",
			"   #   ",
			"       ",
		},
		'/': {
			"      #",
			"     # ",
			"    #  ",
			"   #   ",
			"  #    ",
			" #     ",
		},
	}
}
