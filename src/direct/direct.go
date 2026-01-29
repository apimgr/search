package direct

import (
	"context"
	"regexp"
	"strings"
	"time"
)

// AnswerType represents the type of direct answer
type AnswerType string

// Direct answer types - these are full-page results, not widgets
const (
	// Command reference
	AnswerTypeTLDR  AnswerType = "tldr"
	AnswerTypeMan   AnswerType = "man"
	AnswerTypeCheat AnswerType = "cheat"

	// Network
	AnswerTypeDNS     AnswerType = "dns"
	AnswerTypeWhois   AnswerType = "whois"
	AnswerTypeResolve AnswerType = "resolve"
	AnswerTypeCert    AnswerType = "cert"
	AnswerTypeHeaders AnswerType = "headers"
	AnswerTypeASN     AnswerType = "asn"
	AnswerTypeSubnet  AnswerType = "subnet"

	// Content lookup
	AnswerTypeWiki      AnswerType = "wiki"
	AnswerTypeDict      AnswerType = "dict"
	AnswerTypeThesaurus AnswerType = "thesaurus"
	AnswerTypePkg       AnswerType = "pkg"
	AnswerTypeCVE       AnswerType = "cve"
	AnswerTypeRFC       AnswerType = "rfc"

	// Directory search
	AnswerTypeDirectory AnswerType = "directory"

	// Encoding/formatting
	AnswerTypeHTML    AnswerType = "html"
	AnswerTypeUnicode AnswerType = "unicode"
	AnswerTypeEmoji   AnswerType = "emoji"
	AnswerTypeEscape  AnswerType = "escape"
	AnswerTypeJSON    AnswerType = "json"
	AnswerTypeYAML    AnswerType = "yaml"

	// Utilities
	AnswerTypeHTTP      AnswerType = "http"
	AnswerTypePort      AnswerType = "port"
	AnswerTypeCron      AnswerType = "cron"
	AnswerTypeChmod     AnswerType = "chmod"
	AnswerTypeRegex     AnswerType = "regex"
	AnswerTypeJWT       AnswerType = "jwt"
	AnswerTypeTimestamp AnswerType = "timestamp"

	// URL tools
	AnswerTypeRobots  AnswerType = "robots"
	AnswerTypeSitemap AnswerType = "sitemap"
	AnswerTypeTech    AnswerType = "tech"
	AnswerTypeFeed    AnswerType = "feed"
	AnswerTypeExpand  AnswerType = "expand"
	AnswerTypeSafe    AnswerType = "safe"
	AnswerTypeCache   AnswerType = "cache"

	// Text tools
	AnswerTypeCase     AnswerType = "case"
	AnswerTypeSlug     AnswerType = "slug"
	AnswerTypeLorem    AnswerType = "lorem"
	AnswerTypeWord     AnswerType = "word"
	AnswerTypeBeautify AnswerType = "beautify"
	AnswerTypeDiff     AnswerType = "diff"

	// Reference
	AnswerTypeUserAgent AnswerType = "useragent"
	AnswerTypeMIME      AnswerType = "mime"
	AnswerTypeLicense   AnswerType = "license"
	AnswerTypeCountry   AnswerType = "country"
	AnswerTypeSlang     AnswerType = "slang"
	AnswerTypeRules     AnswerType = "rules"

	// Generators
	AnswerTypeASCII AnswerType = "ascii"
	AnswerTypeQR    AnswerType = "qr"
)

// CacheDurations defines how long to cache each answer type
// Per IDEA.md Direct Answer Caching rules
var CacheDurations = map[AnswerType]time.Duration{
	AnswerTypeTLDR:      7 * 24 * time.Hour,  // 7 days (updated weekly)
	AnswerTypeMan:       30 * 24 * time.Hour, // 30 days
	AnswerTypeCheat:     7 * 24 * time.Hour,  // 7 days
	AnswerTypeDNS:       1 * time.Hour,       // 1 hour
	AnswerTypeWhois:     1 * time.Hour,       // 1 hour
	AnswerTypeResolve:   1 * time.Hour,       // 1 hour
	AnswerTypeCert:      6 * time.Hour,       // 6 hours
	AnswerTypeHeaders:   1 * time.Hour,       // 1 hour
	AnswerTypeASN:       24 * time.Hour,      // 24 hours
	AnswerTypeSubnet:    0,                   // No cache (static calculation)
	AnswerTypeWiki:      24 * time.Hour,      // 24 hours
	AnswerTypeDict:      24 * time.Hour,      // 24 hours
	AnswerTypeThesaurus: 24 * time.Hour,      // 24 hours
	AnswerTypePkg:       6 * time.Hour,       // 6 hours
	AnswerTypeCVE:       24 * time.Hour,      // 24 hours
	AnswerTypeRFC:       30 * 24 * time.Hour, // 30 days (static)
	AnswerTypeDirectory: 1 * time.Hour,       // 1 hour
	AnswerTypeCache:     0,                   // No cache (always fetch fresh)
	// Encoding/formatting - no cache needed (static transformations)
	AnswerTypeHTML:    0,
	AnswerTypeUnicode: 0,
	AnswerTypeEmoji:   24 * time.Hour, // 24 hours (emoji database)
	AnswerTypeEscape:  0,
	AnswerTypeJSON:    0,
	AnswerTypeYAML:    0,
	// Utilities - mostly no cache (static)
	AnswerTypeHTTP:      0,
	AnswerTypePort:      0,
	AnswerTypeCron:      0,
	AnswerTypeChmod:     0,
	AnswerTypeRegex:     0,
	AnswerTypeJWT:       0,
	AnswerTypeTimestamp: 0,
	// URL tools
	AnswerTypeRobots:  1 * time.Hour,
	AnswerTypeSitemap: 1 * time.Hour,
	AnswerTypeTech:    6 * time.Hour,
	AnswerTypeFeed:    1 * time.Hour,
	AnswerTypeExpand:  24 * time.Hour,
	AnswerTypeSafe:    1 * time.Hour,
	// Text tools - no cache (static)
	AnswerTypeCase:     0,
	AnswerTypeSlug:     0,
	AnswerTypeLorem:    0,
	AnswerTypeWord:     0,
	AnswerTypeBeautify: 0,
	AnswerTypeDiff:     0,
	// Reference
	AnswerTypeUserAgent: 0,
	AnswerTypeMIME:      0,
	AnswerTypeLicense:   30 * 24 * time.Hour, // 30 days
	AnswerTypeCountry:   30 * 24 * time.Hour, // 30 days
	AnswerTypeSlang:     1 * time.Hour,       // 1 hour (votes/definitions change)
	AnswerTypeRules:     0,                   // No cache (static built-in)
	// Generators - no cache
	AnswerTypeASCII: 0,
	AnswerTypeQR:    0,
}

// Answer represents a direct answer result (full page)
type Answer struct {
	Type        AnswerType             `json:"type"`
	Term        string                 `json:"term"`
	Title       string                 `json:"title"`
	Description string                 `json:"description,omitempty"`
	Content     string                 `json:"content"`
	Data        map[string]interface{} `json:"data,omitempty"`
	Source      string                 `json:"source,omitempty"`
	SourceURL   string                 `json:"source_url,omitempty"`
	CacheTTL    time.Duration          `json:"cache_ttl,omitempty"`
	Error       string                 `json:"error,omitempty"`
	Timestamp   time.Time              `json:"timestamp"`
}

// Handler interface for direct answer handlers
type Handler interface {
	// Type returns the answer type this handler provides
	Type() AnswerType
	// Handle processes the term and returns a direct answer
	Handle(ctx context.Context, term string) (*Answer, error)
}

// Manager manages all direct answer handlers
type Manager struct {
	handlers map[AnswerType]Handler
	pattern  *regexp.Regexp
}

// NewManager creates a new direct answer manager
func NewManager() *Manager {
	m := &Manager{
		handlers: make(map[AnswerType]Handler),
		// Pattern to match type:term or type: term (space after colon allowed)
		pattern: regexp.MustCompile(`^([a-z]+):\s*(.+)$`),
	}

	// Register all handlers
	m.Register(NewTLDRHandler())
	m.Register(NewManHandler())
	m.Register(NewCheatHandler())
	m.Register(NewDNSHandler())
	m.Register(NewWhoisHandler())
	m.Register(NewResolveHandler())
	m.Register(NewCertHandler())
	m.Register(NewHeadersHandler())
	m.Register(NewASNHandler())
	m.Register(NewSubnetHandler())
	m.Register(NewWikiHandler())
	m.Register(NewDictHandler())
	m.Register(NewThesaurusHandler())
	m.Register(NewPkgHandler())
	m.Register(NewCVEHandler())
	m.Register(NewRFCHandler())
	m.Register(NewDirectoryHandler())
	m.Register(NewHTMLHandler())
	m.Register(NewUnicodeHandler())
	m.Register(NewEmojiHandler())
	m.Register(NewEscapeHandler())
	m.Register(NewJSONHandler())
	m.Register(NewYAMLHandler())
	m.Register(NewHTTPHandler())
	m.Register(NewPortHandler())
	m.Register(NewCronHandler())
	m.Register(NewChmodHandler())
	m.Register(NewRegexHandler())
	m.Register(NewJWTHandler())
	m.Register(NewTimestampHandler())
	m.Register(NewRobotsHandler())
	m.Register(NewSitemapHandler())
	m.Register(NewTechHandler())
	m.Register(NewFeedHandler())
	m.Register(NewExpandHandler())
	m.Register(NewSafeHandler())
	m.Register(NewCacheHandler())
	m.Register(NewCaseHandler())
	m.Register(NewSlugHandler())
	m.Register(NewLoremHandler())
	m.Register(NewWordHandler())
	m.Register(NewBeautifyHandler())
	m.Register(NewDiffHandler())
	m.Register(NewUserAgentHandler())
	m.Register(NewMIMEHandler())
	m.Register(NewLicenseHandler())
	m.Register(NewCountryHandler())
	m.Register(NewSlangHandler())
	m.Register(NewRulesHandler())
	m.Register(NewASCIIHandler())
	m.Register(NewQRHandler())

	return m
}

// Register adds a handler to the manager
func (m *Manager) Register(h Handler) {
	if h != nil {
		m.handlers[h.Type()] = h
	}
}

// Parse checks if query matches direct answer syntax and returns type and term
// Returns empty strings if not a direct answer query
func (m *Manager) Parse(query string) (AnswerType, string) {
	query = strings.TrimSpace(query)
	if query == "" {
		return "", ""
	}

	matches := m.pattern.FindStringSubmatch(query)
	if len(matches) != 3 {
		return "", ""
	}

	answerType := AnswerType(strings.ToLower(matches[1]))
	term := strings.TrimSpace(matches[2])

	// Check if we have a handler for this type
	if _, ok := m.handlers[answerType]; !ok {
		return "", ""
	}

	return answerType, term
}

// IsDirectAnswer checks if query is a direct answer query
func (m *Manager) IsDirectAnswer(query string) bool {
	answerType, _ := m.Parse(query)
	return answerType != ""
}

// Process handles a direct answer query
func (m *Manager) Process(ctx context.Context, query string) (*Answer, error) {
	answerType, term := m.Parse(query)
	if answerType == "" {
		return nil, nil
	}

	return m.ProcessType(ctx, answerType, term)
}

// ProcessType handles a specific direct answer type
func (m *Manager) ProcessType(ctx context.Context, answerType AnswerType, term string) (*Answer, error) {
	handler, ok := m.handlers[answerType]
	if !ok {
		return &Answer{
			Type:      answerType,
			Term:      term,
			Title:     "Unknown Type",
			Content:   "No handler found for this direct answer type.",
			Error:     "unknown_type",
			Timestamp: time.Now(),
		}, nil
	}

	answer, err := handler.Handle(ctx, term)
	if err != nil {
		return &Answer{
			Type:      answerType,
			Term:      term,
			Title:     "Error",
			Content:   err.Error(),
			Error:     "handler_error",
			Timestamp: time.Now(),
		}, err
	}

	// Set cache TTL if not already set
	if answer.CacheTTL == 0 {
		if ttl, ok := CacheDurations[answerType]; ok {
			answer.CacheTTL = ttl
		}
	}

	answer.Timestamp = time.Now()
	return answer, nil
}

// GetHandlerTypes returns all registered handler types
func (m *Manager) GetHandlerTypes() []AnswerType {
	types := make([]AnswerType, 0, len(m.handlers))
	for t := range m.handlers {
		types = append(types, t)
	}
	return types
}

// GetHandler returns the handler for a specific type
func (m *Manager) GetHandler(answerType AnswerType) (Handler, bool) {
	h, ok := m.handlers[answerType]
	return h, ok
}
