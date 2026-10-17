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
			regexp.MustCompile(`(?i)^random\s+uuid\s*$`),
			regexp.MustCompile(`(?i)^uuid\s+v4\s*$`),
			regexp.MustCompile(`(?i)^guid\s*$`),
		},
	}
}

func (h *UUIDHandler) Name() string               { return "uuid" }
func (h *UUIDHandler) Patterns() []*regexp.Regexp { return h.patterns }

func (h *UUIDHandler) CanHandle(query string) bool {
	for _, p := range h.patterns {
		if p.MatchString(query) {
			return true
		}
	}
	return false
}

func (h *UUIDHandler) HandleInstantQuery(ctx context.Context, query string) (*Answer, error) {
	id := uuid.New()

	return &Answer{
		Type:  AnswerTypeUUID,
		Query: query,
		Title: "UUID Generator",
		Content: fmt.Sprintf(`<div class="uuid-result">
<strong>UUID v4:</strong> <code data-uuid-format="standard">%s</code> <button class="copy-btn" data-copy="%s">Copy</button><br>
<strong>Uppercase:</strong> <code data-uuid-format="uppercase">%s</code> <button class="copy-btn" data-copy="%s">Copy</button><br>
<strong>No dashes:</strong> <code data-uuid-format="compact">%s</code> <button class="copy-btn" data-copy="%s">Copy</button><br>
<button type="button" data-action="uuid-regenerate">Generate another</button>
</div>`,
			id.String(), id.String(),
			strings.ToUpper(id.String()), strings.ToUpper(id.String()),
			strings.ReplaceAll(id.String(), "-", ""), strings.ReplaceAll(id.String(), "-", "")),
		Data: map[string]interface{}{
			"uuid": id.String(),
		},
	}, nil
}
