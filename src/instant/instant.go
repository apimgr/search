package instant

import (
	"context"
	"regexp"
	"strings"
)

// AnswerType represents the type of instant answer
type AnswerType string

const (
	AnswerTypeDefinition AnswerType = "definition"
	AnswerTypeDictionary AnswerType = "dictionary"
	AnswerTypeSynonym    AnswerType = "synonym"
	AnswerTypeAntonym    AnswerType = "antonym"
	AnswerTypeMath       AnswerType = "math"
	AnswerTypeConvert    AnswerType = "convert"
	AnswerTypeTime       AnswerType = "time"
	AnswerTypeDate       AnswerType = "date"
	AnswerTypeCalendar   AnswerType = "calendar"
	AnswerTypeTimezone   AnswerType = "timezone"
	AnswerTypeStopwatch  AnswerType = "stopwatch"
	AnswerTypeIP         AnswerType = "ip"
	AnswerTypeHash       AnswerType = "hash"
	AnswerTypeBase64     AnswerType = "base64"
	AnswerTypeURL        AnswerType = "url"
	AnswerTypeColor      AnswerType = "color"
	AnswerTypeUUID       AnswerType = "uuid"
	AnswerTypeRandom     AnswerType = "random"
	AnswerTypePassword   AnswerType = "password"
	AnswerTypeQR         AnswerType = "qr"
	AnswerTypeASCII      AnswerType = "ascii"
	AnswerTypeCase       AnswerType = "case"
	AnswerTypeSlug       AnswerType = "slug"
	AnswerTypeJSON       AnswerType = "json"
	AnswerTypeYAML       AnswerType = "yaml"
	AnswerTypeEscape     AnswerType = "escape"
	AnswerTypeASN        AnswerType = "asn"
	AnswerTypeWHOIS      AnswerType = "whois"
	AnswerTypeDNS        AnswerType = "dns"
	AnswerTypeBeautify   AnswerType = "beautify"

	// Direct answer types (type:query pattern)
	AnswerTypeTLDR      AnswerType = "tldr"
	AnswerTypeHTTPCode  AnswerType = "httpcode"
	AnswerTypePort      AnswerType = "port"
	AnswerTypeCron      AnswerType = "cron"
	AnswerTypeChmod     AnswerType = "chmod"
	AnswerTypeTimestamp AnswerType = "timestamp"
	AnswerTypeSubnet    AnswerType = "subnet"
	AnswerTypeJWT       AnswerType = "jwt"

	// Web analysis types
	AnswerTypeFeed       AnswerType = "feed"
	AnswerTypeTech       AnswerType = "tech"
	AnswerTypeSitemap    AnswerType = "sitemap"
	AnswerTypeSafe       AnswerType = "safe"
	AnswerTypeEmoji      AnswerType = "emoji"
	AnswerTypeHTMLEntity AnswerType = "htmlentity"
	AnswerTypeUnicode    AnswerType = "unicode"
)

// Answer represents an instant answer result
type Answer struct {
	Type        AnswerType             `json:"type"`
	Query       string                 `json:"query"`
	Title       string                 `json:"title"`
	Content     string                 `json:"content"`
	Data        map[string]interface{} `json:"data,omitempty"`
	Source      string                 `json:"source,omitempty"`
	SourceURL   string                 `json:"source_url,omitempty"`
	RelatedHTML string                 `json:"related_html,omitempty"`
}

// Handler interface for instant answer handlers
type Handler interface {
	// Name returns the handler name
	Name() string
	// Patterns returns regex patterns that trigger this handler
	Patterns() []*regexp.Regexp
	// CanHandle checks if query matches this handler
	CanHandle(query string) bool
	// Handle processes the query and returns an answer
	Handle(ctx context.Context, query string) (*Answer, error)
}

// Manager manages all instant answer handlers
type Manager struct {
	handlers []Handler
}

// NewManager creates a new instant answer manager
func NewManager() *Manager {
	m := &Manager{
		handlers: make([]Handler, 0),
	}

	// Register all handlers
	m.Register(NewDefinitionHandler())
	m.Register(NewDictionaryHandler())
	m.Register(NewSynonymHandler())
	m.Register(NewAntonymHandler())
	m.Register(NewMathHandler())
	m.Register(NewConvertHandler())
	m.Register(NewTimeHandler())
	m.Register(NewCalendarHandler())
	m.Register(NewTimezoneHandler())
	m.Register(NewStopwatchHandler())
	m.Register(NewHashHandler())
	m.Register(NewBase64Handler())
	m.Register(NewURLHandler())
	m.Register(NewColorHandler())
	m.Register(NewUUIDHandler())
	m.Register(NewRandomHandler())
	m.Register(NewPasswordHandler())
	m.Register(NewIPHandler())
	m.Register(NewQRHandler())
	m.Register(NewASCIIHandler())
	m.Register(NewCaseHandler())
	m.Register(NewSlugHandler())
	m.Register(NewJSONHandler())
	m.Register(NewYAMLHandler())
	m.Register(NewEscapeHandler())
	m.Register(NewBeautifyHandler())

	// Network and security handlers
	m.Register(NewCertHandler())
	m.Register(NewHeadersHandler())
	m.Register(NewRobotsHandler())
	m.Register(NewExpandHandler())
	m.Register(NewResolveHandler())

	// Direct answer handlers (type:query pattern)
	m.Register(NewTLDRHandler())
	m.Register(NewWHOISHandler())
	m.Register(NewDNSHandler())
	m.Register(NewASNHandler())
	m.Register(NewHTTPCodeHandler())
	m.Register(NewPortHandler())
	m.Register(NewCronHandler())
	m.Register(NewChmodHandler())
	m.Register(NewTimestampHandler())
	m.Register(NewSubnetHandler())
	m.Register(NewJWTHandler())

	// Web analysis handlers
	m.Register(NewFeedHandler())
	m.Register(NewTechHandler())
	m.Register(NewSitemapHandler())
	m.Register(NewSafeHandler())

	// Developer-focused direct answer handlers
	m.Register(NewRegexHandler())
	m.Register(NewCVEHandler())
	m.Register(NewRFCHandler())
	m.Register(NewPkgHandler())
	m.Register(NewManHandler())
	m.Register(NewCheatHandler())

	// Character and encoding handlers
	m.Register(NewEmojiHandler())
	m.Register(NewUnicodeHandler())
	m.Register(NewHTMLEntityHandler())

	return m
}

// Register adds a handler to the manager
func (m *Manager) Register(h Handler) {
	m.handlers = append(m.handlers, h)
}

// Process checks if query matches any instant answer and returns the result
func (m *Manager) Process(ctx context.Context, query string) (*Answer, error) {
	query = strings.TrimSpace(query)

	for _, handler := range m.handlers {
		if handler.CanHandle(query) {
			return handler.Handle(ctx, query)
		}
	}

	return nil, nil
}

// GetHandlers returns all registered handlers
func (m *Manager) GetHandlers() []Handler {
	return m.handlers
}
