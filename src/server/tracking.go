package server

import (
	"encoding/json"
	"html"
	"html/template"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/apimgr/search/src/config"
)

// ConsentCookie is the parsed form of the cookie_consent cookie, per AI.md
// PART 16 "Consent Logic (Granular)".
type ConsentCookie struct {
	Essential   bool  `json:"essential"`
	Preferences bool  `json:"preferences"`
	Analytics   bool  `json:"analytics"`
	Timestamp   int64 `json:"timestamp"`
}

func consentFromRequest(r *http.Request) *ConsentCookie {
	if r == nil {
		return nil
	}
	c, err := r.Cookie("cookie_consent")
	if err != nil || c.Value == "" {
		return nil
	}
	raw, err := url.PathUnescape(c.Value)
	if err != nil {
		raw = c.Value
	}
	return parseConsentJSON(raw)
}

// CanSetPreferenceCookie reports whether the visitor explicitly permits UI preference cookies.
func CanSetPreferenceCookie(r *http.Request) bool {
	consent := consentFromRequest(r)
	return consent != nil && consent.Preferences
}

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

// tracker is the provider contribution. Script bodies are served from the
// same-origin endpoint and are never inserted inline into an HTML page.
type tracker struct {
	tags string
	body string
}

// jsString serializes a value as a JavaScript string literal.
func jsString(v string) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func trackerFor(cfg config.TrackingConfig) (tracker, bool) {
	typeName := strings.ToLower(strings.TrimSpace(cfg.Type))
	id := html.EscapeString(cfg.ID)
	base := strings.TrimSuffix(strings.TrimSpace(cfg.URL), "/")
	switch typeName {
	case "google":
		if strings.TrimSpace(cfg.ID) == "" {
			return tracker{}, false
		}
		if strings.HasPrefix(cfg.ID, "G-") {
			return tracker{
				tags: `<script async src="https://www.googletagmanager.com/gtag/js?id=` + url.QueryEscape(cfg.ID) + `"></script>`,
				body: "window.dataLayer=window.dataLayer||[];function gtag(){dataLayer.push(arguments);}" +
					"gtag('js',new Date());gtag('config'," + jsString(cfg.ID) + ");",
			}, true
		}
		return tracker{
			tags: `<script async src="https://www.google-analytics.com/analytics.js"></script>`,
			body: "window.ga=window.ga||function(){(ga.q=ga.q||[]).push(arguments)};ga.l=+new Date;" +
				"ga('create'," + jsString(cfg.ID) + ",'auto');ga('send','pageview');",
		}, true
	case "matomo", "piwik":
		if strings.TrimSpace(cfg.ID) == "" || base == "" {
			return tracker{}, false
		}
		return tracker{body: "var _paq=window._paq=window._paq||[];_paq.push(['trackPageView']);" +
			"_paq.push(['enableLinkTracking']);_paq.push(['setTrackerUrl'," + jsString(base+"/matomo.php") + "]);" +
			"_paq.push(['setSiteId'," + jsString(cfg.ID) + "]);var g=document.createElement('script');" +
			"g.async=true;g.src=" + jsString(base+"/matomo.js") + ";document.head.appendChild(g);"}, true
	case "owa":
		if strings.TrimSpace(cfg.ID) == "" || base == "" {
			return tracker{}, false
		}
		return tracker{body: "var owa_baseUrl=" + jsString(base+"/") + ";var owa_cmds=window.owa_cmds=window.owa_cmds||[];" +
			"owa_cmds.push(['setSiteId'," + jsString(cfg.ID) + "]);owa_cmds.push(['trackPageView']);owa_cmds.push(['trackClicks']);" +
			"var o=document.createElement('script');o.async=true;o.src=owa_baseUrl+'modules/base/js/owa.tracker-combined-min.js';" +
			"document.head.appendChild(o);"}, true
	case "fathom":
		if strings.TrimSpace(cfg.ID) == "" {
			return tracker{}, false
		}
		src := "https://cdn.usefathom.com/script.js"
		if base != "" {
			src = base + "/tracker.js"
		}
		return tracker{tags: `<script src="` + html.EscapeString(src) + `" data-site="` + id + `" defer></script>`}, true
	case "plausible":
		if strings.TrimSpace(cfg.ID) == "" {
			return tracker{}, false
		}
		src := "https://plausible.io/js/script.js"
		if base != "" {
			src = base + "/js/script.js"
		}
		return tracker{tags: `<script defer data-domain="` + id + `" src="` + html.EscapeString(src) + `"></script>`}, true
	case "umami":
		if strings.TrimSpace(cfg.ID) == "" || base == "" {
			return tracker{}, false
		}
		return tracker{tags: `<script defer src="` + html.EscapeString(base) + `/script.js" data-website-id="` + id + `"></script>`}, true
	case "simple":
		return tracker{tags: `<script async defer src="https://scripts.simpleanalyticscdn.com/latest.js"></script>`}, true
	case "cloudflare":
		if strings.TrimSpace(cfg.ID) == "" {
			return tracker{}, false
		}
		return tracker{tags: `<script defer src="https://static.cloudflareinsights.com/beacon.min.js" data-cf-beacon='{"token": "` + id + `"}'></script>`}, true
	case "", "none":
		return tracker{}, false
	default:
		return tracker{}, false
	}
}

func trackingCSP(csp string, cfg config.TrackingConfig) string {
	add := func(directive, source string) {
		csp = appendCSPDirectiveSources(csp, directive, source)
	}
	addHost := func(raw string) string {
		u, err := url.Parse(raw)
		if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
			return ""
		}
		return u.Scheme + "://" + u.Host
	}
	switch strings.ToLower(strings.TrimSpace(cfg.Type)) {
	case "google":
		if strings.HasPrefix(cfg.ID, "G-") {
			add("script-src", "https://www.googletagmanager.com")
		} else {
			add("script-src", "https://www.google-analytics.com")
		}
		add("connect-src", "https://www.google-analytics.com")
	case "matomo", "piwik", "owa", "umami":
		if host := addHost(cfg.URL); host != "" {
			add("script-src", host)
			add("connect-src", host)
		}
	case "fathom":
		host := "https://cdn.usefathom.com"
		if cfg.URL != "" {
			host = addHost(cfg.URL)
		}
		if host != "" {
			add("script-src", host)
			add("connect-src", host)
		}
	case "plausible":
		host := "https://plausible.io"
		if cfg.URL != "" {
			host = addHost(cfg.URL)
		}
		if host != "" {
			add("script-src", host)
			add("connect-src", host)
		}
	case "simple":
		add("script-src", "https://scripts.simpleanalyticscdn.com")
		add("img-src", "https://queue.simpleanalyticscdn.com")
	case "cloudflare":
		add("script-src", "https://static.cloudflareinsights.com")
		add("connect-src", "https://cloudflareinsights.com")
	}
	return csp
}

// TrackingScript returns provider external tags and a same-origin body loader.
func TrackingScript(cfg config.TrackingConfig) template.HTML {
	t, ok := trackerFor(cfg)
	if !ok {
		return ""
	}
	out := t.tags
	if t.body != "" {
		out += `<script async src="/server/tracking.js"></script>`
	}
	return template.HTML(out)
}

// TrackingScriptBody returns the provider JavaScript served by the same-origin endpoint.
func TrackingScriptBody(cfg config.TrackingConfig) string {
	t, ok := trackerFor(cfg)
	if !ok {
		return ""
	}
	return t.body
}

// TrackingNoscript returns the Simple Analytics pixel for placement at body start.
func TrackingNoscript(cfg config.TrackingConfig) template.HTML {
	if strings.ToLower(strings.TrimSpace(cfg.Type)) != "simple" {
		return ""
	}
	return template.HTML(`<noscript><img src="https://queue.simpleanalyticscdn.com/noscript.gif" alt="" referrerpolicy="no-referrer-when-downgrade" /></noscript>`)
}

func (s *Server) trackingAllowed(r *http.Request) bool {
	consent := consentFromRequest(r)
	if consent == nil || !consent.Analytics || s == nil || s.config == nil || isOnionRequest(s.config, r) || isI2PRequest(r) {
		return false
	}
	_, ok := trackerFor(s.config.Server.Tracking)
	return ok
}

func isI2PRequest(r *http.Request) bool {
	if r == nil {
		return false
	}
	host := r.Host
	if parsed, _, err := net.SplitHostPort(host); err == nil {
		host = parsed
	}
	host = strings.Trim(strings.ToLower(host), "[]")
	return strings.HasSuffix(host, ".i2p")
}

// CheckTrackingAllowed reports whether this request may load the configured tracker.
func (s *Server) CheckTrackingAllowed(r *http.Request) bool { return s.trackingAllowed(r) }

func (s *Server) trackingScript(r *http.Request) template.HTML {
	if !s.trackingAllowed(r) {
		return ""
	}
	return TrackingScript(s.config.Server.Tracking)
}

func (s *Server) handleTrackingScript(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet || !s.trackingAllowed(r) {
		http.NotFound(w, r)
		return
	}
	body := TrackingScriptBody(s.config.Server.Tracking)
	if body == "" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	writeResponseText(w, body)
}

func (s *Server) trackingNoscript(r *http.Request) template.HTML {
	if !s.trackingAllowed(r) {
		return ""
	}
	return TrackingNoscript(s.config.Server.Tracking)
}
