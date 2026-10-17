package server

import (
	"encoding/json"
	"html/template"
	"net/http"
	"net/url"
	"strings"

	"github.com/apimgr/search/src/config"
)

// ConsentCookie is the parsed form of the cookie_consent cookie, per AI.md
// PART 16 "Consent Logic (Granular)".
//
// The cookie is stored percent-encoded JSON so it survives net/http's cookie
// sanitization and round-trips through the client's
// decodeURIComponent + JSON.parse (AI.md PART 16 readConsentCookie).
type ConsentCookie struct {
	Essential   bool  `json:"essential"`
	Preferences bool  `json:"preferences"`
	Analytics   bool  `json:"analytics"`
	Timestamp   int64 `json:"timestamp"`
}

// consentFromRequest reads and decodes the cookie_consent cookie. It returns
// nil when the cookie is absent, empty, malformed, or not valid JSON — a
// cookie the client cannot parse is treated as no consent at all, so a corrupt
// value can never unlock tracking.
func consentFromRequest(r *http.Request) *ConsentCookie {
	if r == nil {
		return nil
	}
	c, err := r.Cookie("cookie_consent")
	if err != nil || c.Value == "" {
		return nil
	}
	// The client writes the value with encodeURIComponent; decode with the
	// matching unescape before handing it to the JSON parser. An unencoded
	// (legacy, unquoted) value simply fails to parse and yields nil, which is
	// the safe outcome.
	raw, err := url.PathUnescape(c.Value)
	if err != nil {
		raw = c.Value
	}
	return parseConsentJSON(raw)
}

// parseConsentJSON decodes a consent cookie payload. The essential flag is
// forced true on a successful parse: essential cookies are required for the app
// to function and can never be switched off (AI.md PART 16 cookie categories).
func parseConsentJSON(raw string) *ConsentCookie {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var c ConsentCookie
	if err := json.Unmarshal([]byte(raw), &c); err != nil {
		return nil
	}
	c.Essential = true
	return &c
}

// CheckTrackingAllowed reports whether analytics may be loaded for this
// request. Per AI.md PART 16 it returns true only when the visitor explicitly
// accepted the analytics category AND server.tracking names a provider.
//
// The spec spells this as a free function reading a global config; this server
// threads *config.Config through the Server instead, so the provider check
// takes s.config rather than calling config.Get().
//
// No cookie means no consent, so a first-time visitor gets no tracking — the
// "Decline" row of the spec's decline-behavior table ("NEVER loaded").
func (s *Server) CheckTrackingAllowed(r *http.Request) bool {
	consent := consentFromRequest(r)
	if consent == nil || !consent.Analytics {
		return false
	}
	if s == nil || s.config == nil {
		return false
	}
	t := s.config.Server.Tracking.Type
	return t != "" && t != "none"
}

// trackingScript returns the analytics snippet to inject for this request, or
// an empty template when the visitor has not consented. It is the single
// injection point: the layout renders {{.TrackingHTML}}, so no other template
// can smuggle a third-party script past the consent gate.
func (s *Server) trackingScript(r *http.Request) template.HTML {
	if !s.CheckTrackingAllowed(r) {
		return ""
	}
	return TrackingScript(s.config.Server.Tracking)
}

// TrackingScript returns the provider-specific analytics markup for cfg, per
// AI.md PART 12 "Analytics / Tracking". An empty or "none" type, or a type with
// no site ID where one is required, returns nothing rather than a broken
// snippet.
//
// The returned value is template.HTML, which bypasses html/template escaping.
// Every interpolated value comes from the operator's own config file and is
// passed through the escaping helpers used below.
func TrackingScript(cfg config.TrackingConfig) template.HTML {
	t := strings.ToLower(strings.TrimSpace(cfg.Type))
	if t == "" || t == "none" {
		return ""
	}

	// esc escapes a value for use inside a double-quoted HTML attribute or a
	// single-quoted JS string literal. html/template's escaper is not in play
	// here because the whole block is template.HTML, so the values are escaped
	// explicitly at the point of interpolation.
	attr := template.HTMLEscapeString
	js := func(s string) string {
		return template.JSEscapeString(s)
	}
	// base trims a trailing slash from a configured base URL so the callers can
	// append a path with a single separator.
	base := func(s string) string { return strings.TrimSuffix(strings.TrimSpace(s), "/") }

	switch t {
	case "google":
		if cfg.ID == "" {
			return ""
		}
		id := js(cfg.ID)
		if strings.HasPrefix(cfg.ID, "G-") {
			// GA4
			return template.HTML(`<script async src="https://www.googletagmanager.com/gtag/js?id=` + attr(cfg.ID) + `"></script>
<script>
  window.dataLayer = window.dataLayer || [];
  function gtag(){dataLayer.push(arguments);}
  gtag('js', new Date());
  gtag('config', '` + id + `');
</script>`)
		}
		// Universal Analytics (legacy)
		return template.HTML(`<script async src="https://www.google-analytics.com/analytics.js"></script>
<script>
  window.ga=window.ga||function(){(ga.q=ga.q||[]).push(arguments)};ga.l=+new Date;
  ga('create', '` + id + `', 'auto');
  ga('send', 'pageview');
</script>`)

	case "matomo", "piwik":
		if cfg.ID == "" || cfg.URL == "" {
			return ""
		}
		u := base(cfg.URL)
		return template.HTML(`<script>
  var _paq = window._paq = window._paq || [];
  _paq.push(['trackPageView']);
  _paq.push(['enableLinkTracking']);
  (function() {
    var u="` + js(u) + `/";
    _paq.push(['setTrackerUrl', u+'matomo.php']);
    _paq.push(['setSiteId', '` + js(cfg.ID) + `']);
    var d=document, g=d.createElement('script'), s=d.getElementsByTagName('script')[0];
    g.async=true; g.src=u+'matomo.js'; s.parentNode.insertBefore(g,s);
  })();
</script>`)

	case "owa":
		if cfg.ID == "" || cfg.URL == "" {
			return ""
		}
		u := base(cfg.URL)
		return template.HTML(`<script>
  var owa_baseUrl = '` + js(u) + `/';
  var owa_cmds = owa_cmds || [];
  owa_cmds.push(['setSiteId', '` + js(cfg.ID) + `']);
  owa_cmds.push(['trackPageView']);
  owa_cmds.push(['trackClicks']);
  (function() {
    var _owa = document.createElement('script'); _owa.type = 'text/javascript'; _owa.async = true;
    _owa.src = owa_baseUrl + 'modules/base/js/owa.tracker-combined-min.js';
    var _owa_s = document.getElementsByTagName('script')[0]; _owa_s.parentNode.insertBefore(_owa, _owa_s);
  })();
</script>`)

	case "fathom":
		scriptURL := "https://cdn.usefathom.com/script.js"
		if cfg.URL != "" {
			// Self-hosted Fathom Lite
			scriptURL = base(cfg.URL) + "/tracker.js"
		}
		return template.HTML(`<script src="` + attr(scriptURL) + `" data-site="` + attr(cfg.ID) + `" defer></script>`)

	case "plausible":
		domain := cfg.ID
		scriptURL := "https://plausible.io/js/script.js"
		if cfg.URL != "" {
			scriptURL = base(cfg.URL) + "/js/script.js"
		}
		return template.HTML(`<script defer data-domain="` + attr(domain) + `" src="` + attr(scriptURL) + `"></script>`)

	case "umami":
		if cfg.ID == "" || cfg.URL == "" {
			return ""
		}
		return template.HTML(`<script defer src="` + attr(base(cfg.URL)+"/script.js") + `" data-website-id="` + attr(cfg.ID) + `"></script>`)

	case "simple":
		return template.HTML(`<script async defer src="https://scripts.simpleanalyticscdn.com/latest.js"></script>
<noscript><img src="https://queue.simpleanalyticscdn.com/noscript.gif" alt="" referrerpolicy="no-referrer-when-downgrade" /></noscript>`)

	case "cloudflare":
		if cfg.ID == "" {
			return ""
		}
		return template.HTML(`<script defer src='https://static.cloudflareinsights.com/beacon.min.js' data-cf-beacon='{"token": "` + attr(cfg.ID) + `"}'></script>`)
	}

	// Unknown provider: config validation rejects these, but never emit a
	// half-built snippet if one slips through.
	return ""
}
