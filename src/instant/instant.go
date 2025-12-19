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
	AnswerTypeIP         AnswerType = "ip"
	AnswerTypeHash       AnswerType = "hash"
	AnswerTypeBase64     AnswerType = "base64"
	AnswerTypeURL        AnswerType = "url"
	AnswerTypeColor      AnswerType = "color"
	AnswerTypeUUID       AnswerType = "uuid"
	AnswerTypeRandom     AnswerType = "random"
	AnswerTypePassword   AnswerType = "password"
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
	m.Register(NewHashHandler())
	m.Register(NewBase64Handler())
	m.Register(NewURLHandler())
	m.Register(NewColorHandler())
	m.Register(NewUUIDHandler())
	m.Register(NewRandomHandler())
	m.Register(NewPasswordHandler())
	m.Register(NewIPHandler())

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
