package server

import (
	"container/list"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"html"
	"log/slog"
	"net/http"
	"net/mail"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/apimgr/search/src/config"
)

const maxContactBodyBytes = 256 << 10

var contactLinkPattern = regexp.MustCompile(`(?i)(?:https?://|www\.)[^\s<>]+|\b(?:[a-z0-9-]+\.)+[a-z]{2,}(?:/[^\s<>]*)?`)
var contactMarkupPattern = regexp.MustCompile(`(?is)<\s*/?\s*[a-z][^>]*>|<!|\[(?:/?(?:url|img|b|i|u))\b`)

type contactSpamState struct {
	mu          sync.Mutex
	events      map[string][]time.Time
	duplicates  map[string]*list.Element
	order       *list.List
	lastCleanup time.Time
	usedTokens  map[string]time.Time
	usedReviews map[string]time.Time
	strikes     map[string][]time.Time
	banned      map[string]time.Time
}

type contactDuplicateEntry struct {
	Key string
	At  time.Time
}

type ContactAntispamProvider interface {
	ContactFormToken() string
	ValidateContactShape(*http.Request, url.Values, bool, bool) string
	ContactHoneypotFilled(url.Values) bool
	ValidateContactFormToken(string) bool
	AcceptContactSubmission(*http.Request, string, string, string, string, string, bool) (string, time.Duration)
	RecordContactBlock(string, *http.Request)
}

func (s *Server) contactSpamSettings() config.ContactAntispamConfig {
	if s == nil || s.config == nil {
		return config.DefaultConfig().Server.Pages.Contact.Antispam
	}
	return s.config.Get().Pages.Contact.Antispam
}

func (s *Server) ContactFormToken() string {
	issued := time.Now().Unix()
	nonceBytes := make([]byte, 16)
	if _, err := rand.Read(nonceBytes); err != nil {
		return ""
	}
	nonce := base64.RawURLEncoding.EncodeToString(nonceBytes)
	message := fmt.Sprintf("contact-form|%d|%s", issued, nonce)
	mac := hmac.New(sha256.New, s.contactFormSigningKey())
	_, _ = mac.Write([]byte(message))
	return fmt.Sprintf("%d.%s.%s", issued, nonce, hex.EncodeToString(mac.Sum(nil)))
}

func (s *Server) ValidateContactFormToken(token string) bool {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return false
	}
	issued, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || issued <= 0 {
		return false
	}
	now := time.Now().Unix()
	settings := s.contactSpamSettings()
	maxAge := settings.MaxAgeSeconds
	if maxAge <= 0 {
		maxAge = 43200
	}
	if issued > now || now-issued > int64(maxAge) {
		return false
	}
	mac := hmac.New(sha256.New, s.contactFormSigningKey())
	_, _ = mac.Write([]byte("contact-form|" + parts[0] + "|" + parts[1]))
	provided, err := hex.DecodeString(parts[2])
	if err != nil || !hmac.Equal(provided, mac.Sum(nil)) {
		return false
	}
	minFill := settings.MinFillSeconds
	if minFill > 0 && now-issued < int64(minFill) {
		return false
	}
	state := s.contactSpam()
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.usedTokens == nil {
		state.usedTokens = make(map[string]time.Time)
	}
	nowTime := time.Now()
	for nonce, expiry := range state.usedTokens {
		if !nowTime.Before(expiry) {
			delete(state.usedTokens, nonce)
		}
	}
	if _, exists := state.usedTokens[parts[1]]; exists {
		return false
	}
	if len(state.usedTokens) >= 50000 {
		for nonce := range state.usedTokens {
			delete(state.usedTokens, nonce)
			if len(state.usedTokens) < 40000 {
				break
			}
		}
	}
	state.usedTokens[parts[1]] = time.Unix(issued+int64(maxAge), 0)
	return true
}

func (s *Server) ContactTokenFailureReason(token string) string {
	parts := strings.Split(token, ".")
	if len(parts) == 3 {
		state := s.contactSpam()
		state.mu.Lock()
		_, used := state.usedTokens[parts[1]]
		state.mu.Unlock()
		if used {
			return "replay"
		}
	}
	return "fill_time"
}

func (s *Server) ContactReviewToken(values url.Values, flags string) string {
	issued := time.Now().Unix()
	nonceBytes := make([]byte, 16)
	if _, err := rand.Read(nonceBytes); err != nil {
		return ""
	}
	nonce := base64.RawURLEncoding.EncodeToString(nonceBytes)
	issuedText := strconv.FormatInt(issued, 10)
	fieldHash := contactReviewFieldsHash(values)
	mac := hmac.New(sha256.New, s.contactFormSigningKey())
	_, _ = fmt.Fprintf(mac, "contact-review|%s|%s|%s|%s", issuedText, nonce, flags, fieldHash)
	return fmt.Sprintf("%s.%s.%s.%s", issuedText, nonce, flags, hex.EncodeToString(mac.Sum(nil)))
}

func (s *Server) ValidateContactReviewToken(values url.Values, token string, confirmed bool) bool {
	parts := strings.Split(token, ".")
	if len(parts) != 4 || !confirmed {
		return false
	}
	issued, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || issued <= 0 {
		return false
	}
	age := time.Now().Unix() - issued
	settings := s.contactSpamSettings()
	minAge, maxAge := settings.ReviewMinSeconds, settings.ReviewMaxSeconds
	if minAge <= 0 {
		minAge = 2
	}
	if maxAge <= 0 {
		maxAge = 900
	}
	if age < int64(minAge) || age > int64(maxAge) {
		return false
	}
	mac := hmac.New(sha256.New, s.contactFormSigningKey())
	_, _ = fmt.Fprintf(mac, "contact-review|%s|%s|%s|%s", parts[0], parts[1], parts[2], contactReviewFieldsHash(values))
	provided, err := hex.DecodeString(parts[3])
	if err != nil || !hmac.Equal(provided, mac.Sum(nil)) {
		return false
	}
	state := s.contactSpam()
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.usedReviews == nil {
		state.usedReviews = make(map[string]time.Time)
	}
	now := time.Now()
	for nonce, expiry := range state.usedReviews {
		if !now.Before(expiry) {
			delete(state.usedReviews, nonce)
		}
	}
	if _, exists := state.usedReviews[parts[1]]; exists {
		return false
	}
	if len(state.usedReviews) >= 50000 {
		for nonce := range state.usedReviews {
			delete(state.usedReviews, nonce)
			if len(state.usedReviews) < 40000 {
				break
			}
		}
	}
	state.usedReviews[parts[1]] = time.Unix(issued+int64(maxAge), 0)
	return true
}

func ContactReviewHoneypot(token string) bool {
	parts := strings.Split(token, ".")
	return len(parts) == 4 && strings.Contains(parts[2], "d")
}

func contactReviewFieldsHash(values url.Values) string {
	filtered := make(url.Values)
	for key, entries := range values {
		if key == "csrf_token" || key == "form_token" || key == "captcha" || key == "captcha_id" || key == "captcha_answer" || key == "captcha_response" || key == "g-recaptcha-response" || key == "h-captcha-response" || key == "website" || key == "step" || key == "review_token" || key == "confirmed" {
			continue
		}
		filtered[key] = append([]string(nil), entries...)
	}
	return fmt.Sprintf("%x", sha256.Sum256([]byte(filtered.Encode())))
}

func (s *Server) contactFormSigningKey() []byte {
	if s != nil && s.config != nil {
		cfg := s.config.Get()
		if cfg.Security.InstallationSecret != "" {
			return []byte(cfg.Security.InstallationSecret)
		}
		if cfg.SecretKey != "" {
			return []byte(cfg.SecretKey)
		}
	}
	return []byte("search-contact-form")
}

func (s *Server) ValidateContactShape(r *http.Request, values url.Values, securityMode, apiRequest bool) string {
	if r.ContentLength > maxContactBodyBytes {
		return "shape"
	}
	allowed := map[string]bool{
		"name": true, "email": true, "subject": true, "message": true,
		"type": true, "reported_url": true, "reason": true, "copyrighted_work": true,
		"good_faith_statement": true, "accuracy_statement": true, "signature": true, "request_kind": true,
		"step": true, "review_token": true, "confirmed": true,
		"captcha": true, "captcha_id": true, "captcha_answer": true,
		"captcha_response": true, "g-recaptcha-response": true, "h-captcha-response": true,
		"form_token": true, "security_id": true, "researcher_gpg": true,
		"affected_component": true, "affected_endpoint": true, "severity": true,
		"summary": true, "steps_to_reproduce": true, "impact": true,
		"suggested_fix": true, "cve_requested": true, "disclosure_days": true,
		"credit_preference": true, "credit_handle": true, "agreed_to_disclosure": true,
		"app_version": true, "commit_hash": true, "timestamp": true, "request_user_agent": true,
	}
	if !apiRequest {
		allowed["website"] = true
		allowed["csrf_token"] = true
	}
	for name, entries := range values {
		if !allowed[name] || len(entries) != 1 {
			return "shape"
		}
		for _, value := range entries {
			if !utf8.ValidString(value) || hasInvisibleUnicode(value) {
				return "shape"
			}
			if name == "name" || name == "email" || name == "subject" {
				for _, char := range value {
					if unicode.IsControl(char) {
						return "shape"
					}
				}
			}
		}
	}
	if !apiRequest && s.contactSpamSettings().Honeypot && !values.Has("website") {
		if values.Get("step") != "confirm" && values.Get("step") != "edit" {
			return "shape"
		}
	}
	if step := values.Get("step"); step != "" && step != "submit" && step != "confirm" && step != "edit" {
		return "shape"
	}
	for _, name := range []string{"name", "email", "form_token"} {
		if strings.TrimSpace(values.Get(name)) == "" {
			return "shape"
		}
	}
	if securityMode {
		if values.Get("type") != "security" {
			return "shape"
		}
		for _, name := range []string{"affected_component", "severity", "summary", "steps_to_reproduce", "impact", "credit_preference", "agreed_to_disclosure"} {
			if strings.TrimSpace(values.Get(name)) == "" {
				return "shape"
			}
		}
	} else {
		typeName := strings.TrimSpace(values.Get("type"))
		if typeName != "abuse" && typeName != "dmca" && typeName != "privacy" {
			return "shape"
		}
		if strings.TrimSpace(values.Get("subject")) == "" || strings.TrimSpace(values.Get("message")) == "" {
			return "shape"
		}
		if (typeName == "abuse" || typeName == "dmca") && strings.TrimSpace(values.Get("reported_url")) == "" {
			return "shape"
		}
		if typeName == "dmca" {
			for _, name := range []string{"copyrighted_work", "good_faith_statement", "accuracy_statement", "signature"} {
				if strings.TrimSpace(values.Get(name)) == "" {
					return "shape"
				}
			}
		}
		if typeName == "privacy" {
			validKinds := map[string]bool{"access": true, "export": true, "deletion": true, "correction": true, "opt_out": true, "other": true}
			if !validKinds[strings.TrimSpace(values.Get("request_kind"))] {
				return "shape"
			}
		}
	}
	address := values.Get("email")
	parsed, err := mail.ParseAddress(address)
	if err != nil || parsed.Address != address || strings.ContainsAny(address, ",;<>\r\n") {
		return "shape"
	}
	return ""
}

func (s *Server) ValidateContactOrigin(r *http.Request) bool {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	referer := strings.TrimSpace(r.Header.Get("Referer"))
	fetchSite := strings.TrimSpace(r.Header.Get("Sec-Fetch-Site"))
	if origin == "" && referer == "" && fetchSite == "" {
		return false
	}
	base, err := url.Parse(s.getBaseURL(r))
	if err != nil {
		return false
	}
	want := strings.ToLower(base.Scheme + "://" + base.Host)
	if origin != "" && !sameContactOrigin(origin, want) {
		return false
	}
	if origin == "" && referer != "" && !sameContactOrigin(referer, want) {
		return false
	}
	return fetchSite == "" || strings.EqualFold(fetchSite, "same-origin")
}

func sameContactOrigin(candidate, want string) bool {
	parsed, err := url.Parse(candidate)
	return err == nil && parsed.Scheme != "" && parsed.Host != "" && strings.EqualFold(parsed.Scheme+"://"+parsed.Host, want)
}

func hasInvisibleUnicode(value string) bool {
	for _, char := range value {
		if unicode.Is(unicode.Cf, char) || char == '\u200b' || char == '\u200c' || char == '\u200d' || char == '\ufeff' {
			return true
		}
	}
	return false
}

func (s *Server) ContactHoneypotFilled(values url.Values) bool {
	return s.contactSpamSettings().Honeypot && strings.TrimSpace(values.Get("website")) != ""
}

func (s *Server) AcceptContactSubmission(r *http.Request, ip, emailAddress, name, subject, message string, securityMode bool) (string, time.Duration) {
	settings := s.contactSpamSettings()
	state := s.contactSpam()
	now := time.Now()
	window := time.Hour
	if parsed, err := time.ParseDuration(settings.Window); err == nil && parsed > 0 {
		window = parsed
	}
	keys := []struct {
		name  string
		limit int
	}{
		{name: "global", limit: settings.Global},
		{name: "email:" + strings.ToLower(emailAddress), limit: settings.PerEmail},
	}
	if !isOnionRequest(s.config, r) && !isI2PRequest(r) {
		keys = append(keys, struct {
			name  string
			limit int
		}{name: "ip:" + ip, limit: settings.PerIP})
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.lastCleanup.IsZero() || now.Sub(state.lastCleanup) >= window {
		for key, events := range state.events {
			if len(contactWindow(events, now, window)) == 0 {
				delete(state.events, key)
			}
		}
		for key, element := range state.duplicates {
			entry := element.Value.(contactDuplicateEntry)
			if now.Sub(entry.At) >= time.Duration(max(settings.DuplicateWindowHours, 0))*time.Hour {
				delete(state.duplicates, key)
				state.order.Remove(element)
			}
		}
		for key := range state.events {
			if len(state.events) <= 10000 {
				break
			}
			if key != "global" {
				delete(state.events, key)
			}
		}
		state.lastCleanup = now
	}
	for _, key := range keys {
		if key.limit <= 0 {
			continue
		}
		events := contactWindow(state.events[key.name], now, window)
		state.events[key.name] = events
		if len(events) >= key.limit {
			remaining := window - now.Sub(events[0])
			if remaining < time.Second {
				remaining = time.Second
			}
			return "rate_" + strings.SplitN(key.name, ":", 2)[0], remaining
		}
	}
	if !securityMode {
		if reason := s.contactContentBlocked(settings, emailAddress, name, subject, message); reason != "" {
			return reason, 0
		}
	}
	if !securityMode {
		digest := contactDuplicateDigest(emailAddress, subject, message)
		if element, exists := state.duplicates[digest]; exists {
			entry := element.Value.(contactDuplicateEntry)
			if now.Sub(entry.At) < time.Duration(max(settings.DuplicateWindowHours, 0))*time.Hour {
				state.order.MoveToFront(element)
				return "duplicate", 0
			}
			delete(state.duplicates, digest)
			state.order.Remove(element)
		}
		entry := contactDuplicateEntry{Key: digest, At: now}
		state.duplicates[digest] = state.order.PushFront(entry)
		for state.order.Len() > 10000 {
			oldest := state.order.Back()
			delete(state.duplicates, oldest.Value.(contactDuplicateEntry).Key)
			state.order.Remove(oldest)
		}
	}
	for _, key := range keys {
		state.events[key.name] = append(state.events[key.name], now)
	}
	return "", 0
}

func (s *Server) CheckContactSubmissionLimit(r *http.Request, ip, emailAddress string) (string, time.Duration) {
	settings := s.contactSpamSettings()
	window := time.Hour
	if parsed, err := time.ParseDuration(settings.Window); err == nil && parsed > 0 {
		window = parsed
	}
	now := time.Now()
	keys := []struct {
		name  string
		limit int
	}{{"global", settings.Global}, {"email:" + strings.ToLower(emailAddress), settings.PerEmail}}
	if !isOnionRequest(s.config, r) && !isI2PRequest(r) {
		keys = append(keys, struct {
			name  string
			limit int
		}{"ip:" + ip, settings.PerIP})
	}
	state := s.contactSpam()
	state.mu.Lock()
	defer state.mu.Unlock()
	for _, key := range keys {
		if key.limit <= 0 {
			continue
		}
		events := contactWindow(state.events[key.name], now, window)
		if len(events) >= key.limit {
			remaining := window - now.Sub(events[0])
			if remaining < time.Second {
				remaining = time.Second
			}
			return "rate_" + strings.SplitN(key.name, ":", 2)[0], remaining
		}
	}
	return "", 0
}

func (s *Server) ForgetContactSubmission(r *http.Request, ip, emailAddress, name, subject, message string, securityMode bool) {
	settings := s.contactSpamSettings()
	state := s.contactSpam()
	window := time.Hour
	if parsed, err := time.ParseDuration(settings.Window); err == nil && parsed > 0 {
		window = parsed
	}
	keys := []string{"global", "email:" + strings.ToLower(emailAddress)}
	if !isOnionRequest(s.config, r) && !isI2PRequest(r) {
		keys = append(keys, "ip:"+ip)
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	now := time.Now()
	for _, key := range keys {
		events := state.events[key]
		if len(events) == 0 {
			continue
		}
		for i := len(events) - 1; i >= 0; i-- {
			if now.Sub(events[i]) <= window {
				state.events[key] = append(events[:i], events[i+1:]...)
				break
			}
		}
	}
	if !securityMode {
		digest := contactDuplicateDigest(emailAddress, subject, message)
		if element, exists := state.duplicates[digest]; exists {
			delete(state.duplicates, digest)
			state.order.Remove(element)
		}
	}
}

func contactWindow(events []time.Time, now time.Time, window time.Duration) []time.Time {
	first := 0
	for first < len(events) && now.Sub(events[first]) >= window {
		first++
	}
	return events[first:]
}

func (s *Server) contactSpam() *contactSpamState {
	s.contactSpamMu.Lock()
	defer s.contactSpamMu.Unlock()
	if s.contactSpamState == nil {
		s.contactSpamState = &contactSpamState{events: make(map[string][]time.Time), duplicates: make(map[string]*list.Element), order: list.New(), usedTokens: make(map[string]time.Time), usedReviews: make(map[string]time.Time), strikes: make(map[string][]time.Time), banned: make(map[string]time.Time)}
	}
	return s.contactSpamState
}

func (s *Server) contactContentBlocked(settings config.ContactAntispamConfig, address, name, subject, message string) string {
	contentFields := []string{name, subject, message}
	if settings.RejectMarkup {
		for _, field := range contentFields {
			decoded := html.UnescapeString(field)
			if strings.ContainsAny(decoded, "<>") || contactMarkupPattern.MatchString(field) {
				return "markup"
			}
		}
	}
	links := contactLinkMatches(message)
	if settings.MaxLinks > 0 && len(links) > settings.MaxLinks {
		return "content"
	}
	if len(contactLinkMatches(name+" "+subject)) > 0 {
		return "content"
	}
	parts := strings.SplitN(address, "@", 2)
	if len(parts) != 2 {
		return "content"
	}
	domain := strings.ToLower(parts[1])
	for _, blocked := range settings.BlockedEmailDomains {
		blocked = strings.Trim(strings.ToLower(strings.TrimSpace(blocked)), ".")
		if blocked != "" && (domain == blocked || strings.HasSuffix(domain, "."+blocked)) {
			return "content"
		}
	}
	content := strings.ToLower(strings.Join(contentFields, "\n"))
	for _, word := range settings.BlockedWords {
		if word = strings.TrimSpace(strings.ToLower(word)); word != "" && strings.Contains(content, word) {
			return "content"
		}
	}
	return ""
}

func contactLinkMatches(content string) []string {
	indices := contactLinkPattern.FindAllStringIndex(content, -1)
	links := make([]string, 0, len(indices))
	for _, match := range indices {
		if match[0] > 0 && content[match[0]-1] == '@' {
			continue
		}
		links = append(links, content[match[0]:match[1]])
	}
	return links
}

func contactDuplicateDigest(address, subject, message string) string {
	normalize := func(value string) string {
		return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(value)), " "))
	}
	return fmt.Sprintf("%x", sha256.Sum256([]byte(normalize(address)+"\x00"+normalize(subject)+"\x00"+normalize(message))))
}

func (s *Server) RecordContactBlock(reason string, r *http.Request) {
	ip := ""
	if r != nil {
		ip = getClientIPSimple(r)
	}
	slog.Warn("contact.spam_blocked", "reason", reason, "client_ip", ip)
	if r != nil && ip != "" && !isOnionRequest(s.config, r) && !isI2PRequest(r) && isContactStrike(reason) {
		settings := s.contactSpamSettings()
		state := s.contactSpam()
		state.mu.Lock()
		window := time.Hour
		if parsed, err := time.ParseDuration(settings.Window); err == nil && parsed > 0 {
			window = parsed
		}
		now := time.Now()
		state.strikes[ip] = append(contactWindow(state.strikes[ip], now, window), now)
		if settings.Strikes > 0 && len(state.strikes[ip]) >= settings.Strikes {
			banHours := settings.BanHours
			if banHours <= 0 {
				banHours = 24
			}
			state.banned[ip] = now.Add(time.Duration(banHours) * time.Hour)
		}
		state.mu.Unlock()
	}
	if s != nil && s.metrics != nil {
		s.metrics.contactBlocked.WithLabelValues(reason).Inc()
	}
}

func isContactStrike(reason string) bool {
	switch reason {
	case "honeypot", "captcha", "content", "markup", "duplicate", "origin", "replay":
		return true
	default:
		return false
	}
}

func (s *Server) ContactBanned(r *http.Request) bool {
	if r == nil || isOnionRequest(s.config, r) || isI2PRequest(r) {
		return false
	}
	state := s.contactSpam()
	state.mu.Lock()
	defer state.mu.Unlock()
	ip := getClientIPSimple(r)
	expires := state.banned[ip]
	if expires.IsZero() {
		return false
	}
	if time.Now().Before(expires) {
		return true
	}
	delete(state.banned, ip)
	return false
}
