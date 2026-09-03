package instant

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// ColorHandler handles color conversions
type ColorHandler struct {
	patterns []*regexp.Regexp
}

func NewColorHandler() *ColorHandler {
	return &ColorHandler{
		patterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)^color[:\s]+#?([0-9a-fA-F]{6}|[0-9a-fA-F]{3})$`),
			regexp.MustCompile(`(?i)^rgb[:\s]+\(?(\d{1,3})[,\s]+(\d{1,3})[,\s]+(\d{1,3})\)?$`),
			regexp.MustCompile(`(?i)^#([0-9a-fA-F]{6}|[0-9a-fA-F]{3})$`),
		},
	}
}

func (h *ColorHandler) Name() string               { return "color" }
func (h *ColorHandler) Patterns() []*regexp.Regexp { return h.patterns }

func (h *ColorHandler) CanHandle(query string) bool {
	for _, p := range h.patterns {
		if p.MatchString(query) {
			return true
		}
	}
	return false
}

func (h *ColorHandler) HandleInstantQuery(ctx context.Context, query string) (*Answer, error) {
	var r, g, b int
	var hexColor string

	// Try to parse hex
	hexPattern := regexp.MustCompile(`(?i)#?([0-9a-fA-F]{6}|[0-9a-fA-F]{3})`)
	if matches := hexPattern.FindStringSubmatch(query); len(matches) > 1 {
		hexColor = matches[1]
		if len(hexColor) == 3 {
			hexColor = string(hexColor[0]) + string(hexColor[0]) +
				string(hexColor[1]) + string(hexColor[1]) +
				string(hexColor[2]) + string(hexColor[2])
		}
		rVal, _ := strconv.ParseInt(hexColor[0:2], 16, 64)
		gVal, _ := strconv.ParseInt(hexColor[2:4], 16, 64)
		bVal, _ := strconv.ParseInt(hexColor[4:6], 16, 64)
		r, g, b = int(rVal), int(gVal), int(bVal)
	}

	// Try to parse RGB
	rgbPattern := regexp.MustCompile(`(?i)rgb[:\s]+\(?(\d{1,3})[,\s]+(\d{1,3})[,\s]+(\d{1,3})\)?`)
	if matches := rgbPattern.FindStringSubmatch(query); len(matches) == 4 {
		r, _ = strconv.Atoi(matches[1])
		g, _ = strconv.Atoi(matches[2])
		b, _ = strconv.Atoi(matches[3])
		hexColor = fmt.Sprintf("%02x%02x%02x", r, g, b)
	}

	if hexColor == "" {
		return nil, nil
	}

	content := fmt.Sprintf(`<div class="color-result">
<div class="color-preview" style="background-color: #%s; width: 100px; height: 100px; border: 1px solid #333; display: inline-block;"></div>
<br><br>
<strong>HEX:</strong> #%s<br>
<strong>RGB:</strong> rgb(%d, %d, %d)<br>
<strong>HSL:</strong> %s
</div>`,
		hexColor, strings.ToUpper(hexColor), r, g, b, rgbToHSL(r, g, b))

	return &Answer{
		Type:    AnswerTypeColor,
		Query:   query,
		Title:   "Color Converter",
		Content: content,
	}, nil
}

func rgbToHSL(r, g, b int) string {
	rf := float64(r) / 255
	gf := float64(g) / 255
	bf := float64(b) / 255

	max := rf
	if gf > max {
		max = gf
	}
	if bf > max {
		max = bf
	}
	min := rf
	if gf < min {
		min = gf
	}
	if bf < min {
		min = bf
	}

	l := (max + min) / 2

	var h, s float64
	if max == min {
		h, s = 0, 0
	} else {
		d := max - min
		if l > 0.5 {
			s = d / (2 - max - min)
		} else {
			s = d / (max + min)
		}

		switch max {
		case rf:
			h = (gf - bf) / d
			if gf < bf {
				h += 6
			}
		case gf:
			h = (bf-rf)/d + 2
		case bf:
			h = (rf-gf)/d + 4
		}
		h /= 6
	}

	return fmt.Sprintf("hsl(%.0f, %.0f%%, %.0f%%)", h*360, s*100, l*100)
}
