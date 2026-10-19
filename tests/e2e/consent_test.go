//go:build e2e

package e2e

import (
	"context"

	"github.com/chromedp/cdproto/network"
)

// grantPreferenceConsent gives a test browser the explicit preference consent
// required before it stores theme, language, or project UI cookies.
func grantPreferenceConsent(ctx context.Context, siteURL string) error {
	const acceptedConsent = "%7B%22essential%22:true,%22preferences%22:true,%22analytics%22:false,%22timestamp%22:1704067200%7D"
	return network.SetCookie("cookie_consent", acceptedConsent).WithURL(siteURL).WithPath("/").Do(ctx)
}
