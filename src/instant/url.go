package instant

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// URLHandler handles URL encoding/decoding and parsing
type URLHandler struct {
	patterns []*regexp.Regexp
}

func NewURLHandler() *URLHandler {
	return &URLHandler{
		patterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)^url\s+encode[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^url\s+decode[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^urlencode[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^urldecode[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^parse\s+url[:\s]+(.+)$`),
		},
	}
}

func (h *URLHandler) Name() string               { return "url" }
func (h *URLHandler) Patterns() []*regexp.Regexp { return h.patterns }

func (h *URLHandler) CanHandle(query string) bool {
	for _, p := range h.patterns {
		if p.MatchString(query) {
			return true
		}
	}
	return false
}

func (h *URLHandler) Handle(ctx context.Context, query string) (*Answer, error) {
	lowerQuery := strings.ToLower(query)
	isDecode := strings.Contains(lowerQuery, "decode")
	isParse := strings.Contains(lowerQuery, "parse")

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

	if isParse {
		parsed, err := url.Parse(text)
		if err != nil {
			return &Answer{
				Type:    AnswerTypeURL,
				Query:   query,
				Title:   "URL Parser",
				Content: fmt.Sprintf("Error: Invalid URL"),
			}, nil
		}

		var content strings.Builder
		content.WriteString(fmt.Sprintf("<strong>URL:</strong> %s<br><br>", text))
		content.WriteString(fmt.Sprintf("<strong>Scheme:</strong> %s<br>", parsed.Scheme))
		content.WriteString(fmt.Sprintf("<strong>Host:</strong> %s<br>", parsed.Host))
		content.WriteString(fmt.Sprintf("<strong>Path:</strong> %s<br>", parsed.Path))
		if parsed.RawQuery != "" {
			content.WriteString(fmt.Sprintf("<strong>Query:</strong> %s<br>", parsed.RawQuery))
		}
		if parsed.Fragment != "" {
			content.WriteString(fmt.Sprintf("<strong>Fragment:</strong> %s<br>", parsed.Fragment))
		}

		return &Answer{
			Type:    AnswerTypeURL,
			Query:   query,
			Title:   "URL Parser",
			Content: content.String(),
		}, nil
	}

	var result, operation string
	if isDecode {
		decoded, err := url.QueryUnescape(text)
		if err != nil {
			result = text
		} else {
			result = decoded
		}
		operation = "Decoded"
	} else {
		result = url.QueryEscape(text)
		operation = "Encoded"
	}

	return &Answer{
		Type:    AnswerTypeURL,
		Query:   query,
		Title:   fmt.Sprintf("URL %s", operation),
		Content: fmt.Sprintf("<strong>Input:</strong> %s<br><br><strong>%s:</strong> <code>%s</code>", text, operation, result),
	}, nil
}
