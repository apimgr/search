package instant

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"
)

// UUIDHandler generates UUIDs
type UUIDHandler struct {
	patterns []*regexp.Regexp
}

func NewUUIDHandler() *UUIDHandler {
	return &UUIDHandler{
		patterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)^uuid\s*$`),
			regexp.MustCompile(`(?i)^generate\s+uuid\s*$`),
			regexp.MustCompile(`(?i)^new\s+uuid\s*$`),
			regexp.MustCompile(`(?i)^guid\s*$`),
		},
	}
}

func (h *UUIDHandler) Name() string                { return "uuid" }
func (h *UUIDHandler) Patterns() []*regexp.Regexp  { return h.patterns }

func (h *UUIDHandler) CanHandle(query string) bool {
	for _, p := range h.patterns {
		if p.MatchString(query) {
			return true
		}
	}
	return false
}

func (h *UUIDHandler) Handle(ctx context.Context, query string) (*Answer, error) {
	id := uuid.New()

	return &Answer{
		Type:  AnswerTypeUUID,
		Query: query,
		Title: "UUID Generator",
		Content: fmt.Sprintf(`<div class="uuid-result">
<strong>UUID v4:</strong> <code>%s</code><br>
<strong>Uppercase:</strong> <code>%s</code><br>
<strong>No dashes:</strong> <code>%s</code>
</div>`,
			id.String(),
			strings.ToUpper(id.String()),
			strings.ReplaceAll(id.String(), "-", "")),
		Data: map[string]interface{}{
			"uuid": id.String(),
		},
	}, nil
}
