package server

import (
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/apimgr/search/src/config"
)

func TestAccentThemeColorsMeetContrastThreshold(t *testing.T) {
	dark, light, ok := accentThemeColors("#8250df")
	if !ok {
		t.Fatal("valid accent color was rejected")
	}
	for _, color := range []struct {
		name                                  string
		value                                 string
		backgroundR, backgroundG, backgroundB float64
	}{
		{name: "dark", value: dark, backgroundR: 40.0 / 255, backgroundG: 42.0 / 255, backgroundB: 54.0 / 255},
		{name: "light", value: light, backgroundR: 1, backgroundG: 1, backgroundB: 1},
	} {
		t.Run(color.name, func(t *testing.T) {
			rgb, err := strconv.ParseUint(strings.TrimPrefix(color.value, "#"), 16, 24)
			if err != nil {
				t.Fatal(err)
			}
			r, g, b := float64(rgb>>16)/255, float64((rgb>>8)&0xff)/255, float64(rgb&0xff)/255
			if contrastRatio(r, g, b, color.backgroundR, color.backgroundG, color.backgroundB) < 4.5 {
				t.Fatalf("accent %s contrast is below 4.5:1", color.value)
			}
		})
	}
}

func TestAccentStylesheetHandler(t *testing.T) {
	server := &Server{}
	server.config = &config.Config{}
	server.config.Web.UI.Accent = "#8250df"
	response := httptest.NewRecorder()
	server.handleAccentCSS(response, httptest.NewRequest("GET", "/static/css/accent.css", nil))
	if response.Code != 200 || !strings.Contains(response.Body.String(), "html.theme-light") || !strings.Contains(response.Body.String(), "theme-auto") {
		t.Fatalf("accent stylesheet response = %d %q", response.Code, response.Body.String())
	}
}
