//go:build e2e

// Shared chromedp plumbing for Tier 2 (nojs_test.go, JS disabled) and Tier 3
// (browser_test.go, JS on). Both tiers drive the same headless Chromium
// instance against the shared httptest.Server from harness_test.go — the only
// thing that changes between them is whether script execution is disabled on
// the browser context before navigation (AI.md PART 28).
package e2e

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/kb"
)

// newBrowserContext launches one headless Chromium tab for the test and
// returns a context bound to it plus a teardown func.
//
// When CHROMEDP_WS is set (tests/e2e.sh does this when running the
// chromedp/headless-shell sidecar) the tab attaches to that CDP endpoint
// instead of exec'ing a local Chromium. Locally launched Chromium runs
// --no-sandbox because the E2E container has no user-namespace sandbox
// available; that is a container property, not something the tests under
// test can see or depend on.
func newBrowserContext(t *testing.T) context.Context {
	return newBrowserContextWithTimeout(t, 30*time.Second)
}

func newBrowserContextWithTimeout(t *testing.T, timeout time.Duration) context.Context {
	t.Helper()

	base := context.Background()
	var cancelBrowser context.CancelFunc

	var ctx context.Context
	if ws := strings.TrimSpace(os.Getenv("CHROMEDP_WS")); ws != "" {
		// Attach to the running headless-shell sidecar over CDP. A remote
		// allocator is not runnable on its own — it only describes how to
		// reach the browser — so it must still be wrapped in NewContext
		// before Run. Skipping that wrap makes every action fail with
		// "invalid context".
		allocCtx, allocCancel := chromedp.NewRemoteAllocator(base, ws)
		ctx, cancelBrowser = chromedp.NewContext(allocCtx, chromedp.WithNewBrowserContext())
		t.Cleanup(allocCancel)
	} else {
		// Exec a local Chromium; --no-sandbox because the E2E container has
		// no user-namespace sandbox available.
		allocCtx, allocCancel := chromedp.NewExecAllocator(base,
			append(chromedp.DefaultExecAllocatorOptions[:],
				chromedp.Flag("headless", true),
				chromedp.Flag("no-sandbox", true),
				chromedp.Flag("disable-gpu", true),
				chromedp.Flag("disable-dev-shm-usage", true),
			)...,
		)
		ctx, cancelBrowser = chromedp.NewContext(allocCtx, chromedp.WithNewBrowserContext())
		t.Cleanup(allocCancel)
	}

	if err := chromedp.Run(ctx); err != nil {
		cancelBrowser()
		t.Fatalf("chromedp: connect Chromium: %v (no local Chromium? run tests/e2e.sh, which starts the headless-shell sidecar and sets CHROMEDP_WS)", err)
	}

	t.Cleanup(cancelBrowser)

	tctx, tcancel := context.WithTimeout(ctx, timeout)
	t.Cleanup(tcancel)
	return tctx
}

func clickAndWaitForMainFrameNavigation(ctx context.Context, selector string) error {
	navigated := make(chan struct{}, 1)
	var startURL string
	if err := chromedp.Run(ctx, chromedp.Location(&startURL)); err != nil {
		return err
	}
	chromedp.ListenTarget(ctx, func(event interface{}) {
		frameEvent, ok := event.(*page.EventFrameNavigated)
		if !ok || frameEvent.Frame.ParentID != "" {
			return
		}
		select {
		case navigated <- struct{}{}:
		default:
		}
	})
	clickErr := chromedp.Run(ctx,
		chromedp.WaitVisible(selector, chromedp.ByQuery),
		chromedp.ScrollIntoView(selector, chromedp.ByQuery),
		waitForClickTarget(selector),
		chromedp.Click(selector, chromedp.ByQuery),
	)
	select {
	case <-navigated:
		return nil
	case <-time.After(5 * time.Second):
		if clickErr != nil {
			return clickErr
		}
		var currentURL string
		if err := chromedp.Run(ctx, chromedp.Location(&currentURL)); err == nil && currentURL != startURL {
			return nil
		}
		var state string
		_ = chromedp.Run(ctx, chromedp.Evaluate(`JSON.stringify({url:location.href, forms:Array.from(document.forms).map(form => ({action:form.action, valid:form.checkValidity()})), submitButtons:Array.from(document.querySelectorAll('form[method="post"] button[type="submit"]')).map(button => { const rect = button.getBoundingClientRect(); const point = document.elementFromPoint(rect.x + rect.width / 2, rect.y + rect.height / 2); return {text:button.textContent.trim(), disabled:button.disabled, width:rect.width, height:rect.height, action:button.form?.action, top:point?.tagName + '.' + point?.className}; }), openDetails:Array.from(document.querySelectorAll('details')).map(details => details.open)})`, &state))
		return fmt.Errorf("click %q did not navigate the main frame from %q (page state %s)", selector, startURL, state)
	}
}

func waitForClickTarget(selector string) chromedp.Action {
	return chromedp.Poll(fmt.Sprintf(`(() => {
		const element = document.querySelector(%s);
		if (!element) return false;
		const rect = element.getBoundingClientRect();
		if (rect.width <= 0 || rect.height <= 0 || rect.top < 0 || rect.bottom > innerHeight) return false;
		const target = document.elementFromPoint(rect.left + rect.width / 2, rect.top + rect.height / 2);
		return target === element || element.contains(target);
	})()`, strconv.Quote(selector)), nil,
		chromedp.WithPollingInterval(25*time.Millisecond), chromedp.WithPollingTimeout(5*time.Second))
}

func pressEnterAndWaitForMainFrameNavigation(ctx context.Context, selector string) error {
	navigated := make(chan struct{}, 1)
	var startURL string
	if err := chromedp.Run(ctx, chromedp.Location(&startURL)); err != nil {
		return err
	}
	chromedp.ListenTarget(ctx, func(event interface{}) {
		frameEvent, ok := event.(*page.EventFrameNavigated)
		if !ok || frameEvent.Frame.ParentID != "" {
			return
		}
		select {
		case navigated <- struct{}{}:
		default:
		}
	})
	if err := chromedp.Run(ctx,
		chromedp.WaitVisible(selector, chromedp.ByQuery),
		chromedp.ScrollIntoView(selector, chromedp.ByQuery),
		waitForClickTarget(selector),
		chromedp.Focus(selector, chromedp.ByQuery),
		chromedp.KeyEvent(kb.Enter),
	); err != nil {
		return err
	}
	select {
	case <-navigated:
		return nil
	case <-time.After(5 * time.Second):
		var currentURL string
		if err := chromedp.Run(ctx, chromedp.Location(&currentURL)); err == nil && currentURL != startURL {
			return nil
		}
		return fmt.Errorf("press Enter on %q did not navigate the main frame from %q", selector, startURL)
	}
}

// pageFailures collects Tier 3 failure signals: console.error/exception text
// and failed asset/XHR loads. CDP events arrive on chromedp's internal
// goroutine while the test goroutine reads via requireNoFailures, so every
// append and every read is mutex-guarded.
type pageFailures struct {
	mu            sync.Mutex
	consoleErrors []string
	failedLoads   []string
}

func (pf *pageFailures) addConsole(msg string) {
	pf.mu.Lock()
	defer pf.mu.Unlock()
	pf.consoleErrors = append(pf.consoleErrors, msg)
}

func (pf *pageFailures) addFailedLoad(msg string) {
	pf.mu.Lock()
	defer pf.mu.Unlock()
	pf.failedLoads = append(pf.failedLoads, msg)
}

func (pf *pageFailures) snapshot() (console, loads []string) {
	pf.mu.Lock()
	defer pf.mu.Unlock()
	return pf.consoleErrors, pf.failedLoads
}

func watchForFailures(ctx context.Context) *pageFailures {
	pf := &pageFailures{}

	// Console and network events are only delivered once the corresponding
	// CDP domain is enabled for this target.
	_ = chromedp.Run(ctx, network.Enable(), runtime.Enable())

	// The top-level document's own status is the subject of some tests (the
	// 404 page test navigates to a missing route on purpose), so it must not
	// be reported as a failed load. Everything else the page pulls in — CSS,
	// JS, images, XHR, and documents inside subframes — still counts.
	// e.FrameID is empty for non-document responses, so only real top-level
	// document loads are excluded; an empty mainFrame (frame tree not yet
	// available) falls back to reporting everything rather than hiding a
	// genuine failure.
	var mainFrame cdp.FrameID
	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		tree, err := page.GetFrameTree().Do(ctx)
		if err != nil {
			return err
		}
		mainFrame = tree.Frame.ID
		return nil
	})); err != nil {
		slog.Debug("E2E frame tree unavailable; top-level document status will be reported", "err", err)
	}

	requestURLs := make(map[network.RequestID]string)
	chromedp.ListenTarget(ctx, func(ev interface{}) {
		switch e := ev.(type) {
		case *network.EventRequestWillBeSent:
			requestURLs[e.RequestID] = e.Request.URL
		case *network.EventResponseReceived:
			if e.Response.Status >= 400 && !(mainFrame != "" && e.FrameID == mainFrame) {
				pf.addFailedLoad(fmt.Sprintf("%s -> %d", e.Response.URL, e.Response.Status))
			}
		case *network.EventLoadingFailed:
			// canceled/aborted loads are normal (e.g. a prefetch the browser
			// itself drops); only a real failure reason is a defect.
			if e.ErrorText != "" && e.ErrorText != "net::ERR_ABORTED" {
				url := requestURLs[e.RequestID]
				if url == "" {
					url = string(e.RequestID)
				}
				pf.addFailedLoad(fmt.Sprintf("%s (%s)", url, e.ErrorText))
			}
		case *runtime.EventExceptionThrown:
			message := e.ExceptionDetails.Text
			if e.ExceptionDetails.Exception != nil && e.ExceptionDetails.Exception.Description != "" {
				message += ": " + e.ExceptionDetails.Exception.Description
			}
			pf.addConsole(message)
		case *runtime.EventConsoleAPICalled:
			if e.Type == "error" {
				parts := make([]string, 0, len(e.Args))
				for _, a := range e.Args {
					if a.Value != nil {
						parts = append(parts, string(a.Value))
					} else {
						parts = append(parts, a.Description)
					}
				}
				pf.addConsole(strings.Join(parts, " "))
			}
		}
	})

	return pf
}

// requireNoFailures fails the test with every recorded console error and
// failed request, so a red run names exactly what broke instead of just
// "something failed".
func requireNoFailures(t *testing.T, pf *pageFailures, path string) {
	t.Helper()
	console, loads := pf.snapshot()
	if len(console) > 0 {
		t.Errorf("%s: %d console error(s):\n%s", path, len(console), strings.Join(console, "\n"))
	}
	if len(loads) > 0 {
		t.Errorf("%s: %d failed asset/XHR request(s):\n%s", path, len(loads), strings.Join(loads, "\n"))
	}
}
