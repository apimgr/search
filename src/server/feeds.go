package server

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/apimgr/search/src/common/httputil"
)

type feedQuery struct {
	Query string
	At    time.Time
}

var (
	feedEmailPattern    = regexp.MustCompile(`(?i)[a-z0-9.!#$%&'*+/=?^_\x60{|}~-]+@[a-z0-9-]+(?:\.[a-z0-9-]+)+`)
	feedStreetPattern   = regexp.MustCompile(`(?i)\b\d+\s+[a-z0-9.'-]+\s+(street|st|avenue|ave|road|rd|lane|ln|drive|dr|blvd|court|way|suite|apt)\b|\bpo\s+box\s+\d+`)
	feedNumberPattern   = regexp.MustCompile(`\d{9,}`)
	feedSSNPattern      = regexp.MustCompile(`\b\d{3}-\d{2}-\d{4}\b`)
	feedSensitiveURL    = regexp.MustCompile(`(?i)https?://[^\s]*[?&](token|key|password|secret)=`)
	feedTokenPattern    = regexp.MustCompile(`[A-Za-z0-9+/_=-]{20,}`)
	feedTokenHasLetters = regexp.MustCompile(`[A-Za-z]`)
	feedTokenHasDigits  = regexp.MustCompile(`[0-9]`)
	feedCardPattern     = regexp.MustCompile(`\d{13,19}`)
)

func safeFeedQuery(query string) bool {
	query = strings.TrimSpace(query)
	if query == "" || utf8.RuneCountInString(query) > 100 || feedEmailPattern.MatchString(query) || feedStreetPattern.MatchString(query) || feedSSNPattern.MatchString(query) || feedSensitiveURL.MatchString(query) {
		return false
	}
	for _, field := range strings.Fields(query) {
		if net.ParseIP(strings.Trim(field, "[](),;")) != nil {
			return false
		}
	}
	for _, match := range feedTokenPattern.FindAllString(query, -1) {
		if feedTokenHasLetters.MatchString(match) && feedTokenHasDigits.MatchString(match) {
			return false
		}
	}
	phoneDigits := 0
	phoneOnlyDigits := true
	for index, r := range query {
		switch r {
		case ' ', '-', '.', '(', ')':
		default:
			if r == '+' && index == 0 {
				continue
			}
			if r >= '0' && r <= '9' {
				phoneDigits++
			} else {
				phoneOnlyDigits = false
			}
		}
	}
	if phoneOnlyDigits && phoneDigits >= 7 || feedNumberPattern.MatchString(query) {
		return false
	}
	for _, run := range feedCardPattern.FindAllString(query, -1) {
		if luhnValid(run) {
			return false
		}
	}
	return true
}

func luhnValid(value string) bool {
	sum, double := 0, false
	for i := len(value) - 1; i >= 0; i-- {
		digit, _ := strconv.Atoi(value[i : i+1])
		if double {
			digit *= 2
			if digit > 9 {
				digit -= 9
			}
		}
		sum += digit
		double = !double
	}
	return sum%10 == 0
}

func (s *Server) recordFeedQuery(query string, at time.Time) {
	query = strings.TrimSpace(query)
	if !safeFeedQuery(query) {
		return
	}
	s.feedQueriesMu.Lock()
	defer s.feedQueriesMu.Unlock()
	for i := range s.feedQueries {
		if s.feedQueries[i].Query == query {
			s.feedQueries[i].At = at
			sort.Slice(s.feedQueries, func(a, b int) bool { return s.feedQueries[a].At.After(s.feedQueries[b].At) })
			return
		}
	}
	s.feedQueries = append(s.feedQueries, feedQuery{Query: query, At: at})
	if len(s.feedQueries) > 200 {
		s.feedQueries = s.feedQueries[:200]
	}
}

func (s *Server) feedEntries() []feedQuery {
	s.feedQueriesMu.RLock()
	defer s.feedQueriesMu.RUnlock()
	items := append([]feedQuery(nil), s.feedQueries...)
	sort.Slice(items, func(i, j int) bool { return items[i].At.After(items[j].At) })
	limit := s.config.Server.SEO.Feed.MaxItems
	if limit < 1 || limit > 200 {
		limit = 50
	}
	if len(items) > limit {
		items = items[:limit]
	}
	return items
}

func xmlText(value string) string {
	var b strings.Builder
	clean := strings.Map(func(r rune) rune {
		if r == 0x9 || r == 0xA || r == 0xD || (r >= 0x20 && r <= 0xD7FF) || (r >= 0xE000 && r <= 0xFFFD) || (r >= 0x10000 && r <= 0x10FFFF) {
			return r
		}
		return -1
	}, value)
	_ = xml.EscapeText(&b, []byte(clean))
	return b.String()
}

func (s *Server) feedTitle() string {
	if title := strings.TrimSpace(s.config.Server.SEO.Feed.Title); title != "" {
		return title
	}
	if title := strings.TrimSpace(s.config.Server.Branding.Title); title != "" {
		return title
	}
	return s.config.Server.Title
}

func (s *Server) serveFeed(w http.ResponseWriter, r *http.Request, atom bool) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	items := s.feedEntries()
	base := httpURL(r, "/")
	title := xmlText(s.feedTitle())
	updated := s.startTime.UTC()
	if len(items) != 0 {
		updated = items[0].At.UTC()
	}
	var body strings.Builder
	if atom {
		fmt.Fprintf(&body, "<?xml version=\"1.0\" encoding=\"utf-8\"?>\n<feed xmlns=\"http://www.w3.org/2005/Atom\"><id>%s</id><title>%s</title><updated>%s</updated><link href=\"%sfeed.xml\" rel=\"self\"/><link href=\"%s\"/>", xmlText(strings.TrimRight(base, "/")), title, updated.Format(time.RFC3339), xmlText(base), xmlText(base))
		for _, item := range items {
			permalink := httpURL(r, "/search?q="+url.QueryEscape(item.Query))
			fmt.Fprintf(&body, "<entry><id>%s</id><title>%s</title><link href=\"%s\"/><updated>%s</updated><published>%s</published><summary>%s</summary></entry>", xmlText(permalink), xmlText(item.Query), xmlText(permalink), item.At.UTC().Format(time.RFC3339), item.At.UTC().Format(time.RFC3339), xmlText(item.Query))
		}
		body.WriteString("</feed>\n")
		w.Header().Set("Content-Type", "application/atom+xml; charset=utf-8")
	} else {
		fmt.Fprintf(&body, "<?xml version=\"1.0\" encoding=\"utf-8\"?>\n<rss version=\"2.0\"><channel><title>%s</title><link>%s</link><description>%s</description><lastBuildDate>%s</lastBuildDate>", title, xmlText(base), title, updated.Format(time.RFC1123Z))
		for _, item := range items {
			permalink := httpURL(r, "/search?q="+url.QueryEscape(item.Query))
			fmt.Fprintf(&body, "<item><title>%s</title><link>%s</link><guid isPermaLink=\"true\">%s</guid><pubDate>%s</pubDate><description>%s</description></item>", xmlText(item.Query), xmlText(permalink), xmlText(permalink), item.At.UTC().Format(time.RFC1123Z), xmlText(item.Query))
		}
		body.WriteString("</channel></rss>\n")
		w.Header().Set("Content-Type", "application/rss+xml; charset=utf-8")
	}
	bytes := []byte(body.String())
	sum := sha256.Sum256(bytes)
	etag := `"` + hex.EncodeToString(sum[:]) + `"`
	w.Header().Set("ETag", etag)
	w.Header().Set("Last-Modified", updated.Truncate(time.Second).Format(http.TimeFormat))
	w.Header().Set("Cache-Control", "public, max-age=300")
	if r.Header.Get("If-None-Match") == etag || (r.Header.Get("If-None-Match") == "" && notModifiedSince(r, updated)) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	if r.Method != http.MethodHead {
		_, _ = w.Write(bytes)
	}
}

func (s *Server) handleAtomFeed(w http.ResponseWriter, r *http.Request) { s.serveFeed(w, r, true) }
func (s *Server) handleRSSFeed(w http.ResponseWriter, r *http.Request)  { s.serveFeed(w, r, false) }

func notModifiedSince(r *http.Request, updated time.Time) bool {
	value := r.Header.Get("If-Modified-Since")
	if value == "" {
		return false
	}
	parsed, err := time.Parse(http.TimeFormat, value)
	return err == nil && !updated.Truncate(time.Second).After(parsed)
}

func httpURL(r *http.Request, path string) string {
	return httputil.BuildURL(r, path)
}

func sitemapRobotsAllows(path string, allow, deny []string) bool {
	if len(allow) == 0 {
		allow = []string{"/", "/api"}
	}
	allowed := false
	for _, rule := range allow {
		if robotsRuleMatches(path, rule) {
			allowed = true
			break
		}
	}
	if !allowed {
		return false
	}
	for _, rule := range deny {
		if robotsRuleMatches(path, rule) {
			return false
		}
	}
	return true
}

func robotsRuleMatches(path, rule string) bool {
	rule = strings.TrimSpace(rule)
	if rule == "" {
		return false
	}
	exact := strings.HasSuffix(rule, "$")
	rule = strings.TrimSuffix(rule, "$")
	prefix := strings.TrimSuffix(rule, "*")
	if strings.Contains(prefix, "*") {
		return false
	}
	if exact {
		return path == prefix
	}
	return strings.HasPrefix(path, prefix)
}
