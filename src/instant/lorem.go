package instant

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/apimgr/search/src/common/i18n"
)

// LoremHandler generates bounded Lorem Ipsum placeholder text.
type LoremHandler struct {
	patterns []*regexp.Regexp
}

func NewLoremHandler() *LoremHandler {
	return &LoremHandler{patterns: []*regexp.Regexp{
		regexp.MustCompile(`(?i)^(?:lorem(?:\s+ipsum)?|placeholder\s+text|dummy\s+text)(?:\s+(.*))?$`),
	}}
}

func (h *LoremHandler) Name() string { return "lorem" }

func (h *LoremHandler) Patterns() []*regexp.Regexp { return h.patterns }

func (h *LoremHandler) CanHandle(query string) bool {
	for _, pattern := range h.patterns {
		if pattern.MatchString(strings.TrimSpace(query)) {
			return true
		}
	}
	return false
}

func (h *LoremHandler) HandleInstantQuery(ctx context.Context, query string) (*Answer, error) {
	query = strings.TrimSpace(query)
	var options string
	for _, pattern := range h.patterns {
		if matches := pattern.FindStringSubmatch(query); len(matches) > 0 {
			if len(matches) > 1 {
				options = strings.TrimSpace(matches[1])
			}
			break
		}
	}
	if options == "" && !h.CanHandle(query) {
		return nil, nil
	}

	count := 1
	mode := "paragraphs"
	if matches := regexp.MustCompile(`(?i)^(\d+)\s+(paragraphs?|sentences?|words?)$`).FindStringSubmatch(options); len(matches) == 3 {
		parsed, err := strconv.Atoi(matches[1])
		if err != nil || parsed < 1 {
			return nil, fmt.Errorf("count must be between 1 and 500")
		}
		count = parsed
		mode = strings.ToLower(matches[2])
		mode = strings.TrimSuffix(mode, "s")
		switch mode {
		case "paragraph":
			mode = "paragraphs"
		case "sentence":
			mode = "sentences"
		case "word":
			mode = "words"
		}
		if count > 500 || mode == "paragraphs" && count > 10 || mode == "sentences" && count > 100 {
			return nil, fmt.Errorf("count is above the supported limit")
		}
	} else if options != "" {
		return nil, fmt.Errorf("use a count followed by paragraphs, sentences, or words")
	}

	text := generateLorem(mode, count)
	content := fmt.Sprintf(`<div class="lorem-answer"><pre class="lorem-text">%s</pre><button class="copy-btn" data-copy="%s">Copy</button></div>`, escapeHTML(text), escapeHTML(text))
	return &Answer{
		Type:    AnswerTypeLorem,
		Query:   query,
		Title:   i18n.T(LangFromContext(ctx), "instant.lorem_ipsum_generator_title"),
		Content: content,
		Data:    map[string]interface{}{"text": text, "count": count, "mode": mode},
	}, nil
}

func generateLorem(mode string, count int) string {
	const sentence = "Lorem ipsum dolor sit amet, consectetur adipiscing elit."
	const paragraph = "Lorem ipsum dolor sit amet, consectetur adipiscing elit. Sed do eiusmod tempor incididunt ut labore et dolore magna aliqua. Ut enim ad minim veniam, quis nostrud exercitation ullamco laboris nisi ut aliquip ex ea commodo consequat."
	switch mode {
	case "words":
		words := strings.Fields(paragraph)
		result := make([]string, count)
		for i := range result {
			result[i] = strings.TrimRight(words[i%len(words)], ".,")
		}
		return strings.Join(result, " ")
	case "sentences":
		return strings.TrimSpace(strings.Repeat(sentence+" ", count))
	default:
		result := make([]string, count)
		for i := range result {
			result[i] = paragraph
		}
		return strings.Join(result, "\n\n")
	}
}
