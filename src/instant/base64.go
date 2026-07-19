package instant

import (
	"context"
	"encoding/base64"
	"fmt"
	"regexp"
	"strings"
)

// Base64Handler handles base64 encoding/decoding
type Base64Handler struct {
	patterns []*regexp.Regexp
}

func NewBase64Handler() *Base64Handler {
	return &Base64Handler{
		patterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)^base64\s+encode[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^base64\s+decode[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^b64\s+encode[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^b64\s+decode[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^encode\s+base64[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^decode\s+base64[:\s]+(.+)$`),
			// "base64: text" and "base64 text" shorthand (defaults to encode)
			regexp.MustCompile(`(?i)^base64[:\s]+(.+)$`),
		},
	}
}

func (h *Base64Handler) Name() string               { return "base64" }
func (h *Base64Handler) Patterns() []*regexp.Regexp { return h.patterns }

func (h *Base64Handler) CanHandle(query string) bool {
	for _, p := range h.patterns {
		if p.MatchString(query) {
			return true
		}
	}
	return false
}

func (h *Base64Handler) HandleInstantQuery(ctx context.Context, query string) (*Answer, error) {
	lowerQuery := strings.ToLower(query)
	isDecode := strings.Contains(lowerQuery, "decode")

	var text string
	for _, p := range h.patterns {
		if matches := p.FindStringSubmatch(query); len(matches) > 1 {
			text = matches[1]
			break
		}
	}

	if text == "" {
		return nil, nil
	}

	var result, operation string
	if isDecode {
		decoded, err := base64.StdEncoding.DecodeString(text)
		if err != nil {
			return &Answer{
				Type:    AnswerTypeBase64,
				Query:   query,
				Title:   "Base64 Decoder",
				Content: "Error: Invalid base64 string",
			}, nil
		}
		result = string(decoded)
		operation = "Decoded"
	} else {
		result = base64.StdEncoding.EncodeToString([]byte(text))
		operation = "Encoded"
	}

	return &Answer{
		Type:    AnswerTypeBase64,
		Query:   query,
		Title:   fmt.Sprintf("Base64 %s", operation),
		Content: fmt.Sprintf("<strong>Input:</strong> %s<br><br><strong>%s:</strong> <code>%s</code>", escapeHTML(text), operation, escapeHTML(result)),
		Data: map[string]interface{}{
			"input":     text,
			"output":    result,
			"operation": strings.ToLower(operation),
		},
	}, nil
}
