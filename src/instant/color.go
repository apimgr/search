package instant

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/image/colornames"
)

var namedColors = func() map[string]string {
	colors := map[string]string{
		"dark blue": "00008b", "dark green": "006400", "grey": "808080",
		"rebeccapurple": "663399",
	}
	for name, value := range colornames.Map {
		colors[strings.ToLower(name)] = fmt.Sprintf("%02x%02x%02x", value.R, value.G, value.B)
	}
	return colors
}()

// ColorHandler handles color conversions.
type ColorHandler struct {
	patterns []*regexp.Regexp
}

func NewColorHandler() *ColorHandler {
	return &ColorHandler{
		patterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)^color[:\s]+#?([0-9a-fA-F]{6}|[0-9a-fA-F]{3})$`),
			regexp.MustCompile(`(?i)^color[:\s]+([a-z ]+)$`),
			regexp.MustCompile(`(?i)^rgb[:\s]*\(?(\d{1,3})[,\s]+(\d{1,3})[,\s]+(\d{1,3})\)?$`),
			regexp.MustCompile(`(?i)^hsl[:\s]*\(?(\d+(?:\.\d+)?)[,\s]+(\d+(?:\.\d+)?)%[,\s]+(\d+(?:\.\d+)?)%\)?$`),
			regexp.MustCompile(`(?i)^#([0-9a-fA-F]{6}|[0-9a-fA-F]{3})$`),
		},
	}
}

func (h *ColorHandler) Name() string { return "color" }

func (h *ColorHandler) Patterns() []*regexp.Regexp { return h.patterns }

func (h *ColorHandler) CanHandle(query string) bool {
	query = strings.TrimSpace(strings.ToLower(query))
	if _, ok := namedColors[query]; ok {
		return true
	}
	for index, pattern := range h.patterns {
		matches := pattern.FindStringSubmatch(query)
		if matches == nil {
			continue
		}
		if index == 1 {
			_, ok := namedColors[strings.TrimSpace(strings.ToLower(matches[1]))]
			return ok
		}
		if index == 2 {
			_, _, _, ok := parseRGBColor(query)
			return ok
		}
		if index == 3 {
			_, _, _, ok := parseHSLColor(query)
			return ok
		}
		return true
	}
	return false
}

func (h *ColorHandler) HandleInstantQuery(ctx context.Context, query string) (*Answer, error) {
	query = strings.ToLower(strings.TrimSpace(query))
	r, g, b, ok := parseHSLColor(query)
	if !ok {
		r, g, b, ok = parseRGBColor(query)
	}
	if !ok {
		name := strings.TrimSpace(strings.ToLower(query))
		if strings.HasPrefix(name, `color:`) {
			name = strings.TrimSpace(strings.TrimPrefix(name, `color:`))
		}
		if hex, found := namedColors[name]; found {
			r, g, b = parseHexColor(hex)
			ok = true
		}
	}
	if !ok {
		hexPattern := regexp.MustCompile(`(?i)^#?([0-9a-fA-F]{6}|[0-9a-fA-F]{3})$`)
		hexQuery := strings.TrimSpace(strings.TrimPrefix(strings.ToLower(query), `color:`))
		if matches := hexPattern.FindStringSubmatch(hexQuery); len(matches) == 2 {
			r, g, b = parseHexColor(matches[1])
			ok = true
		}
	}
	if !ok {
		return nil, nil
	}

	hex := fmt.Sprintf(`%02x%02x%02x`, r, g, b)
	rgb := fmt.Sprintf(`rgb(%d, %d, %d)`, r, g, b)
	hsl := rgbToHSL(r, g, b)
	cmyk := rgbToCMYK(r, g, b)
	complement := complementaryColor(r, g, b)
	name := colorName(hex)
	colorNamesJSON, _ := json.Marshal(namedColors)
	content := fmt.Sprintf(`<div class="color-result" data-color-names="%s">
<input class="color-preview" type="color" value="#%s" aria-label="Choose a color">
<strong>HEX:</strong> <span data-color-value="hex">#%s</span> <button class="copy-btn" data-color-copy="hex" data-copy="#%s">Copy HEX</button><br>
<strong>RGB:</strong> <span data-color-value="rgb">%s</span> <button class="copy-btn" data-color-copy="rgb" data-copy="%s">Copy RGB</button><br>
<strong>HSL:</strong> <span data-color-value="hsl">%s</span> <button class="copy-btn" data-color-copy="hsl" data-copy="%s">Copy HSL</button><br>
<strong>CMYK:</strong> <span data-color-value="cmyk">%s</span> <button class="copy-btn" data-color-copy="cmyk" data-copy="%s">Copy CMYK</button><br>
<strong>Name:</strong> <span data-color-value="name">%s</span><br>
<strong>Complementary:</strong> <span data-color-value="complementary">#%s</span> <button class="copy-btn" data-color-copy="complementary" data-copy="#%s">Copy</button>
</div>`,
		escapeHTML(string(colorNamesJSON)), hex, strings.ToUpper(hex), strings.ToUpper(hex), rgb, rgb, hsl, hsl, cmyk, cmyk,
		escapeHTML(name), strings.ToUpper(complement), strings.ToUpper(complement))

	return &Answer{
		Type:    AnswerTypeColor,
		Query:   query,
		Title:   "Color Converter",
		Content: content,
		Data: map[string]interface{}{
			"hex": strings.ToUpper(hex), "rgb": rgb, "hsl": hsl, "cmyk": cmyk,
			"name": name, "complementary": strings.ToUpper(complement),
		},
	}, nil
}

func parseHexColor(value string) (int, int, int) {
	if len(value) == 3 {
		value = string(value[0]) + string(value[0]) + string(value[1]) + string(value[1]) + string(value[2]) + string(value[2])
	}
	r, _ := strconv.ParseInt(value[0:2], 16, 64)
	g, _ := strconv.ParseInt(value[2:4], 16, 64)
	b, _ := strconv.ParseInt(value[4:6], 16, 64)
	return int(r), int(g), int(b)
}

func parseRGBColor(query string) (int, int, int, bool) {
	matches := regexp.MustCompile(`(?i)^rgb[:\s]*\(?(\d{1,3})[,\s]+(\d{1,3})[,\s]+(\d{1,3})\)?$`).FindStringSubmatch(strings.TrimSpace(query))
	if len(matches) != 4 {
		return 0, 0, 0, false
	}
	values := [3]int{}
	for i := range values {
		value, err := strconv.Atoi(matches[i+1])
		if err != nil || value > 255 {
			return 0, 0, 0, false
		}
		values[i] = value
	}
	return values[0], values[1], values[2], true
}

func parseHSLColor(query string) (int, int, int, bool) {
	matches := regexp.MustCompile(`(?i)^hsl[:\s]*\(?(\d+(?:\.\d+)?)[,\s]+(\d+(?:\.\d+)?)%[,\s]+(\d+(?:\.\d+)?)%\)?$`).FindStringSubmatch(strings.TrimSpace(query))
	if len(matches) != 4 {
		return 0, 0, 0, false
	}
	hue, hueErr := strconv.ParseFloat(matches[1], 64)
	saturation, saturationErr := strconv.ParseFloat(matches[2], 64)
	lightness, lightnessErr := strconv.ParseFloat(matches[3], 64)
	if hueErr != nil || saturationErr != nil || lightnessErr != nil || saturation < 0 || saturation > 100 || lightness < 0 || lightness > 100 {
		return 0, 0, 0, false
	}
	hue = math.Mod(hue, 360) / 360
	saturation /= 100
	lightness /= 100
	if saturation == 0 {
		value := int(math.Round(lightness * 255))
		return value, value, value, true
	}
	q := lightness * (1 + saturation)
	if lightness >= 0.5 {
		q = lightness + saturation - lightness*saturation
	}
	p := 2*lightness - q
	return int(math.Round(hueToRGB(p, q, hue+1.0/3) * 255)),
		int(math.Round(hueToRGB(p, q, hue) * 255)),
		int(math.Round(hueToRGB(p, q, hue-1.0/3) * 255)), true
}

func hueToRGB(p, q, t float64) float64 {
	if t < 0 {
		t++
	}
	if t > 1 {
		t--
	}
	switch {
	case t < 1.0/6:
		return p + (q-p)*6*t
	case t < 1.0/2:
		return q
	case t < 2.0/3:
		return p + (q-p)*(2.0/3-t)*6
	default:
		return p
	}
}

func colorName(hex string) string {
	names := make([]string, 0, len(namedColors))
	for name := range namedColors {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if namedColors[name] == hex {
			return name
		}
	}
	return ""
}

func rgbToCMYK(r, g, b int) string {
	red, green, blue := float64(r)/255, float64(g)/255, float64(b)/255
	k := 1 - math.Max(red, math.Max(green, blue))
	if k >= 1 {
		return "cmyk(0%, 0%, 0%, 100%)"
	}
	c := (1 - red - k) / (1 - k)
	m := (1 - green - k) / (1 - k)
	y := (1 - blue - k) / (1 - k)
	return fmt.Sprintf("cmyk(%.0f%%, %.0f%%, %.0f%%, %.0f%%)", c*100, m*100, y*100, k*100)
}

func rgbToHSL(r, g, b int) string {
	rf, gf, bf := float64(r)/255, float64(g)/255, float64(b)/255
	max := math.Max(rf, math.Max(gf, bf))
	min := math.Min(rf, math.Min(gf, bf))
	l := (max + min) / 2
	var h, s float64
	if max != min {
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
		default:
			h = (rf-gf)/d + 4
		}
		h /= 6
	}
	return fmt.Sprintf("hsl(%.0f, %.0f%%, %.0f%%)", h*360, s*100, l*100)
}

func complementaryColor(r, g, b int) string {
	rf, gf, bf := float64(r)/255, float64(g)/255, float64(b)/255
	max := math.Max(rf, math.Max(gf, bf))
	min := math.Min(rf, math.Min(gf, bf))
	delta := max - min
	lightness := (max + min) / 2
	var hue, saturation float64
	if delta != 0 {
		if lightness > 0.5 {
			saturation = delta / (2 - max - min)
		} else {
			saturation = delta / (max + min)
		}
		switch max {
		case rf:
			hue = (gf - bf) / delta
			if gf < bf {
				hue += 6
			}
		case gf:
			hue = (bf-rf)/delta + 2
		default:
			hue = (rf-gf)/delta + 4
		}
		hue = math.Mod(hue/6+0.5, 1)
	}
	if saturation == 0 {
		value := int(math.Round(lightness * 255))
		return fmt.Sprintf("%02x%02x%02x", value, value, value)
	}
	q := lightness * (1 + saturation)
	if lightness >= 0.5 {
		q = lightness + saturation - lightness*saturation
	}
	p := 2*lightness - q
	return fmt.Sprintf("%02x%02x%02x",
		int(math.Round(hueToRGB(p, q, hue+1.0/3)*255)),
		int(math.Round(hueToRGB(p, q, hue)*255)),
		int(math.Round(hueToRGB(p, q, hue-1.0/3)*255)))
}
