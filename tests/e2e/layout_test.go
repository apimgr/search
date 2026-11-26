//go:build e2e

package e2e

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/kb"
)

func TestBrowserLayoutContract(t *testing.T) {
	site := newTestSite(t)
	ctx := newBrowserContextWithTimeout(t, 15*time.Minute)
	viewports := [][2]int{{320, 640}, {375, 812}, {390, 844}, {768, 1024}, {820, 1180}, {1024, 768}, {1280, 800}, {1440, 900}, {1920, 1080}}
	paths := []string{
		"/",
		"/search?q=layout+contract",
		"/direct/math:2%2B2",
		"/alerts/new?q=layout+contract",
		"/server/about",
		"/server/privacy",
		"/server/contact",
		"/server/help",
		"/server/help/mcp",
		"/server/terms",
		"/server/dpo",
		"/server/healthz",
		"/server/security",
		"/server/security/policy",
		"/server/security/thanks",
		"/server/preferences",
		"/server/preferences/tools",
		"/server/preferences/tokens",
	}

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
					BodyWidth   int      `json:"bodyWidth"`
					BodyScroll  int      `json:"bodyScroll"`
					BodyAfter   string   `json:"bodyAfter"`
					WideContent []string `json:"wideContent"`
					BadElements []string `json:"badElements"`
					BadFields   []string `json:"badFields"`
					EmptySelect []string `json:"emptySelect"`
				}
				if err := chromedp.Run(ctx, chromedp.Evaluate(`(() => {
					const visible = element => {
						const style = getComputedStyle(element);
						return style.display !== 'none' && element.getClientRects().length > 0;
					};
					return {
						width: document.documentElement.scrollWidth,
						innerWidth: window.innerWidth,
						bodyWidth: Math.round(document.body.getBoundingClientRect().width),
						bodyScroll: document.body.scrollWidth,
						bodyAfter: getComputedStyle(document.body, '::after').content,
						wideContent: Array.from(document.body.querySelectorAll('*')).filter(e => e.scrollWidth > e.clientWidth + 1).map(e => {
							const r = e.getBoundingClientRect();
							return e.tagName.toLowerCase() + '.' + (e.getAttribute('class') || '') + '@' + Math.round(r.left) + ':' + Math.round(r.right) + ' scroll=' + e.scrollWidth + '/' + e.clientWidth;
						}).slice(0, 12),
						badElements: Array.from(document.body.querySelectorAll('*')).filter(visible).filter(element => {
							const wrapper = element.closest('.table-wrapper');
							if (wrapper && getComputedStyle(wrapper).overflowX === 'auto') return false;
							const rect = element.getBoundingClientRect();
							return rect.left < -1 || rect.right > window.innerWidth + 1;
						}).map(element => {
							const rect = element.getBoundingClientRect();
							const classes = typeof element.className === 'string' ? '.' + element.className.trim().split(/\s+/).filter(Boolean).join('.') : '';
							return element.tagName.toLowerCase() + (element.id ? '#' + element.id : '') + classes + '@' + Math.round(rect.left) + ':' + Math.round(rect.right);
						}).slice(0, 12),
						badFields: Array.from(document.querySelectorAll('input:not([type="hidden"]),select,textarea')).filter(element => !element.labels?.length && !element.getAttribute('aria-label')).map(element => element.id || element.outerHTML.slice(0, 80)),
						emptySelect: Array.from(document.querySelectorAll('select')).filter(element => !element.options.length).map(element => element.id || 'unnamed select')
					};
				})()`, &result)); err != nil {
					t.Fatalf("inspect layout: %v", err)
				}
				if result.Width > result.InnerWidth {
					t.Errorf("document overflows horizontally: scrollWidth=%d innerWidth=%d bodyWidth=%d bodyScroll=%d bodyAfter=%q elements=%v wideContent=%v", result.Width, result.InnerWidth, result.BodyWidth, result.BodyScroll, result.BodyAfter, result.BadElements, result.WideContent)
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
							Width      int      `json:"width"`
							InnerWidth int      `json:"innerWidth"`
							Direction  string   `json:"direction"`
							Theme      string   `json:"theme"`
							Overflow   []string `json:"overflow"`
						}
						if err := chromedp.Run(ctx,
							chromedp.Navigate(url),
							chromedp.Evaluate(`({width: document.documentElement.scrollWidth, innerWidth: innerWidth, direction: document.documentElement.dir, theme: document.documentElement.dataset.themeMode, overflow: Array.from(document.body.querySelectorAll('*')).map(e => { const r = e.getBoundingClientRect(); return r.width > 0 && r.right > innerWidth + 1 ? e.tagName.toLowerCase() + '.' + (e.getAttribute('class') || '') + '@' + Math.round(r.left) + ':' + Math.round(r.right) : '' }).filter(Boolean).slice(0, 12)})`, &result),
						); err != nil {
							t.Fatalf("load layout variant: %v", err)
						}
						wantDirection := "ltr"
						if language == "ar" {
							wantDirection = "rtl"
						}
						if result.Width > result.InnerWidth || result.Direction != wantDirection || result.Theme != theme {
							t.Errorf("layout variant failed: width=%d innerWidth=%d direction=%q theme=%q overflowing elements=%v", result.Width, result.InnerWidth, result.Direction, result.Theme, result.Overflow)
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
	if drawer.Height < 843 || drawer.Width > 390 || len(drawer.Links) != 1 || drawer.Links[0] != "/" || !drawer.Visible || !drawer.Focused {
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
