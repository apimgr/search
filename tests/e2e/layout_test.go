//go:build e2e

package e2e

import (
	"fmt"
	"strings"
	"testing"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/kb"
)

func TestBrowserLayoutContract(t *testing.T) {
	site := newTestSite(t)
	ctx := newBrowserContext(t)
	viewports := [][2]int{{320, 640}, {375, 812}, {390, 844}, {768, 1024}, {820, 1180}, {1024, 768}, {1280, 800}, {1440, 900}, {1920, 1080}}
	paths := []string{"/", "/server/about", "/server/help", "/server/dpo", "/server/preferences/tokens"}

	for _, viewport := range viewports {
		for _, path := range paths {
			t.Run(fmt.Sprintf("%dx%d%s", viewport[0], viewport[1], path), func(t *testing.T) {
				if err := chromedp.Run(ctx,
					emulation.SetDeviceMetricsOverride(int64(viewport[0]), int64(viewport[1]), 1, false),
					chromedp.Navigate(site.URL+path),
				); err != nil {
					t.Fatalf("load %s: %v", path, err)
				}
				var result struct {
					Width       int      `json:"width"`
					InnerWidth  int      `json:"innerWidth"`
					BadElements []string `json:"badElements"`
					BadFields   []string `json:"badFields"`
					EmptySelect []string `json:"emptySelect"`
				}
				if err := chromedp.Run(ctx, chromedp.Evaluate(`(() => {
					const visible = element => {
						const style = getComputedStyle(element);
						return style.display !== 'none' && style.visibility !== 'hidden' && element.getClientRects().length > 0;
					};
					return {
						width: document.documentElement.scrollWidth,
						innerWidth: window.innerWidth,
						badElements: Array.from(document.body.querySelectorAll('*')).filter(visible).filter(element => {
							const rect = element.getBoundingClientRect();
							return rect.left < -1 || rect.right > window.innerWidth + 1;
						}).map(element => element.tagName.toLowerCase() + (element.id ? '#' + element.id : '')).slice(0, 12),
						badFields: Array.from(document.querySelectorAll('input:not([type="hidden"]),select,textarea')).filter(element => !element.labels?.length && !element.getAttribute('aria-label')).map(element => element.id || element.outerHTML.slice(0, 80)),
						emptySelect: Array.from(document.querySelectorAll('select')).filter(element => !element.options.length).map(element => element.id || 'unnamed select')
					};
				})()`, &result)); err != nil {
					t.Fatalf("inspect layout: %v", err)
				}
				if result.Width > result.InnerWidth {
					t.Errorf("document overflows horizontally: scrollWidth=%d innerWidth=%d", result.Width, result.InnerWidth)
				}
				if len(result.BadElements) != 0 {
					t.Errorf("elements extend beyond viewport: %v", result.BadElements)
				}
				if len(result.BadFields) != 0 {
					t.Errorf("form controls have no label: %v", result.BadFields)
				}
				if len(result.EmptySelect) != 0 {
					t.Errorf("selects have no options: %v", result.EmptySelect)
				}
			})
		}
	}

	for _, viewport := range [][2]int{{320, 640}, {768, 1024}, {1280, 800}} {
		for _, theme := range []string{"dark", "light", "auto"} {
			for _, language := range []string{"en", "ar"} {
				for _, path := range paths {
					t.Run(fmt.Sprintf("%dx%d-%s-%s%s", viewport[0], viewport[1], theme, language, path), func(t *testing.T) {
						if err := chromedp.Run(ctx,
							emulation.SetDeviceMetricsOverride(int64(viewport[0]), int64(viewport[1]), 1, false),
							network.SetCookie("theme", theme).WithURL(site.URL),
							network.SetCookie("lang", language).WithURL(site.URL),
						); err != nil {
							t.Fatalf("set viewport and theme: %v", err)
						}
						url := site.URL + path
						if language == "ar" {
							separator := "?"
							if strings.Contains(url, "?") {
								separator = "&"
							}
							url += separator + "lang=ar"
						}
						var result struct {
							Width      int    `json:"width"`
							InnerWidth int    `json:"innerWidth"`
							Direction  string `json:"direction"`
							Theme      string `json:"theme"`
						}
						if err := chromedp.Run(ctx,
							chromedp.Navigate(url),
							chromedp.Evaluate(`({width: document.documentElement.scrollWidth, innerWidth: innerWidth, direction: document.documentElement.dir, theme: document.documentElement.dataset.themeMode})`, &result),
						); err != nil {
							t.Fatalf("load layout variant: %v", err)
						}
						wantDirection := "ltr"
						if language == "ar" {
							wantDirection = "rtl"
						}
						if result.Width > result.InnerWidth || result.Direction != wantDirection || result.Theme != theme {
							t.Errorf("layout variant failed: width=%d innerWidth=%d direction=%q theme=%q", result.Width, result.InnerWidth, result.Direction, result.Theme)
						}
					})
				}
			}
		}
	}

	if err := chromedp.Run(ctx,
		emulation.SetDeviceMetricsOverride(390, 844, 1, false),
		chromedp.Navigate(site.URL+"/"),
		chromedp.Click(".nav-toggle", chromedp.ByQuery),
	); err != nil {
		t.Fatalf("open mobile navigation: %v", err)
	}
	var drawer struct {
		Height  float64  `json:"height"`
		Width   float64  `json:"width"`
		Links   []string `json:"links"`
		Visible bool     `json:"visible"`
		Focused bool     `json:"focused"`
	}
	if err := chromedp.Run(ctx, chromedp.Evaluate(`(() => {
		const panel = document.querySelector('#nav-panel');
		return {height: panel.getBoundingClientRect().height, width: panel.getBoundingClientRect().width,
			visible: getComputedStyle(panel).visibility === 'visible',
			focused: panel.contains(document.activeElement),
			links: Array.from(panel.querySelectorAll('a')).map(link => link.getAttribute('href'))};
	})()`, &drawer)); err != nil {
		t.Fatalf("inspect mobile navigation drawer: %v", err)
	}
	if drawer.Height < 843 || drawer.Width > 390 || len(drawer.Links) != 3 || !drawer.Visible || !drawer.Focused {
		t.Errorf("mobile drawer has unexpected visibility, dimensions, focus, or links: visible=%t focused=%t height=%.0f width=%.0f links=%v", drawer.Visible, drawer.Focused, drawer.Height, drawer.Width, drawer.Links)
	}
	var closed bool
	if err := chromedp.Run(ctx,
		chromedp.KeyEvent(kb.Escape),
		chromedp.Evaluate(`!document.querySelector('#nav-toggle').checked`, &closed),
	); err != nil {
		t.Fatalf("close mobile navigation with Escape: %v", err)
	}
	if !closed {
		t.Error("Escape did not close the mobile navigation drawer")
	}
}
