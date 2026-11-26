package server

import (
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
)

func (s *Server) handleAccentCSS(w http.ResponseWriter, r *http.Request) {
	accent := s.config.Web.UI.Accent
	if preference := resolveGeneralPrefs(r).AccentColor; validAccentColor(preference) {
		accent = preference
	}
	dark, light, ok := accentThemeColors(accent)
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = fmt.Fprintf(w, ":root{--color-primary:%s;--color-primary-bg:%s}\n", dark, accentWithAlpha(dark, 0.15))
	_, _ = fmt.Fprintf(w, "html.theme-light{--color-primary:%s;--color-primary-bg:%s}\n", light, accentWithAlpha(light, 0.12))
	_, _ = fmt.Fprintf(w, "@media (prefers-color-scheme:light){html.theme-auto{--color-primary:%s;--color-primary-bg:%s}}\n", light, accentWithAlpha(light, 0.12))
}

func (s *Server) handleCustomCSS(w http.ResponseWriter, r *http.Request) {
	css := resolveGeneralPrefs(r).CustomCSS
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write([]byte(css))
}

func validAccentColor(value string) bool {
	_, _, ok := accentThemeColors(value)
	return ok
}

func accentThemeColors(value string) (string, string, bool) {
	if len(value) != 7 || value[0] != '#' {
		return "", "", false
	}
	rgb, err := strconv.ParseUint(value[1:], 16, 24)
	if err != nil {
		return "", "", false
	}
	r, g, b := float64(rgb>>16)/255, float64((rgb>>8)&0xff)/255, float64(rgb&0xff)/255
	hue, saturation, lightness := rgbToHSL(r, g, b)
	dark := [3]float64{r, g, b}
	light := dark
	if contrastRatio(dark[0], dark[1], dark[2], 40.0/255, 42.0/255, 54.0/255) < 4.5 {
		dark = adjustAccent(hue, saturation, lightness, 40.0/255, 42.0/255, 54.0/255, true)
	}
	if contrastRatio(light[0], light[1], light[2], 1, 1, 1) < 4.5 {
		light = adjustAccent(hue, saturation, lightness, 1, 1, 1, false)
	}
	return rgbHex(dark), rgbHex(light), true
}

func adjustAccent(hue, saturation, lightness, br, bg, bb float64, lighter bool) [3]float64 {
	for step := 1; step <= 100; step++ {
		candidate := lightness + float64(step)/100
		if !lighter {
			candidate = lightness - float64(step)/100
		}
		if candidate < 0 || candidate > 1 {
			break
		}
		r, g, b := hslToRGB(hue, saturation, candidate)
		if contrastRatio(r, g, b, br, bg, bb) >= 4.5 {
			return [3]float64{r, g, b}
		}
	}
	if lighter {
		return [3]float64{1, 1, 1}
	}
	return [3]float64{0, 0, 0}
}

func rgbToHSL(r, g, b float64) (float64, float64, float64) {
	maximum := math.Max(r, math.Max(g, b))
	minimum := math.Min(r, math.Min(g, b))
	delta := maximum - minimum
	lightness := (maximum + minimum) / 2
	if delta == 0 {
		return 0, 0, lightness
	}
	saturation := delta / (1 - math.Abs(2*lightness-1))
	var hue float64
	switch maximum {
	case r:
		hue = (g - b) / delta
		if g < b {
			hue += 6
		}
	case g:
		hue = (b-r)/delta + 2
	default:
		hue = (r-g)/delta + 4
	}
	return hue / 6, saturation, lightness
}

func hslToRGB(hue, saturation, lightness float64) (float64, float64, float64) {
	chroma := (1 - math.Abs(2*lightness-1)) * saturation
	x := chroma * (1 - math.Abs(math.Mod(hue*6, 2)-1))
	var r, g, b float64
	switch int(hue * 6) {
	case 0:
		r, g = chroma, x
	case 1:
		r, g = x, chroma
	case 2:
		g, b = chroma, x
	case 3:
		g, b = x, chroma
	case 4:
		r, b = x, chroma
	default:
		r, b = chroma, x
	}
	m := lightness - chroma/2
	return r + m, g + m, b + m
}

func contrastRatio(r, g, b, br, bg, bb float64) float64 {
	left := relativeLuminance(r, g, b)
	right := relativeLuminance(br, bg, bb)
	if right > left {
		left, right = right, left
	}
	return (left + 0.05) / (right + 0.05)
}

func relativeLuminance(r, g, b float64) float64 {
	linear := func(value float64) float64 {
		if value <= 0.04045 {
			return value / 12.92
		}
		return math.Pow((value+0.055)/1.055, 2.4)
	}
	return 0.2126*linear(r) + 0.7152*linear(g) + 0.0722*linear(b)
}

func rgbHex(color [3]float64) string {
	return fmt.Sprintf("#%02x%02x%02x", int(color[0]*255+0.5), int(color[1]*255+0.5), int(color[2]*255+0.5))
}

func accentWithAlpha(color string, alpha float64) string {
	rgb, _ := strconv.ParseUint(strings.TrimPrefix(color, "#"), 16, 24)
	return fmt.Sprintf("rgba(%d,%d,%d,%.2f)", rgb>>16, (rgb>>8)&0xff, rgb&0xff, alpha)
}
