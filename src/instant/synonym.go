package instant

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/apimgr/search/src/common/version"
)

// SynonymHandler handles synonym lookups
type SynonymHandler struct {
	client   *http.Client
	patterns []*regexp.Regexp
}

// NewSynonymHandler creates a new synonym handler
func NewSynonymHandler() *SynonymHandler {
	return &SynonymHandler{
		client: &http.Client{Timeout: 10 * time.Second},
		patterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)^synonym[s]?[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^syn[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^similar\s+to[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^words?\s+like[:\s]+(.+)$`),
		},
	}
}

func (h *SynonymHandler) Name() string {
	return "synonym"
}

func (h *SynonymHandler) Patterns() []*regexp.Regexp {
	return h.patterns
}

func (h *SynonymHandler) CanHandle(query string) bool {
	for _, p := range h.patterns {
		if p.MatchString(query) {
			return true
		}
	}
	return false
}

func (h *SynonymHandler) Handle(ctx context.Context, query string) (*Answer, error) {
	word := ""
	for _, p := range h.patterns {
		if matches := p.FindStringSubmatch(query); len(matches) > 1 {
			word = strings.TrimSpace(matches[1])
			break
		}
	}

	if word == "" {
		return nil, nil
	}

	// Use Datamuse API for synonyms
	apiURL := fmt.Sprintf("https://api.datamuse.com/words?rel_syn=%s&max=20", url.QueryEscape(word))

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", version.BrowserUserAgent)

	resp, err := h.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var data []struct {
		Word  string `json:"word"`
		Score int    `json:"score"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	if len(data) == 0 {
		return &Answer{
			Type:    AnswerTypeSynonym,
			Query:   query,
			Title:   fmt.Sprintf("Synonyms for: %s", word),
			Content: fmt.Sprintf("No synonyms found for '%s'", word),
		}, nil
	}

	// Build content
	synonyms := make([]string, 0, len(data))
	for _, item := range data {
		synonyms = append(synonyms, item.Word)
	}

	var content strings.Builder
	content.WriteString(fmt.Sprintf("<strong>Synonyms for \"%s\":</strong><br><br>", word))
	content.WriteString(strings.Join(synonyms, ", "))

	return &Answer{
		Type:      AnswerTypeSynonym,
		Query:     query,
		Title:     fmt.Sprintf("Synonyms for: %s", word),
		Content:   content.String(),
		Source:    "Datamuse API",
		SourceURL: "https://www.datamuse.com/api/",
		Data: map[string]interface{}{
			"word":     word,
			"synonyms": synonyms,
		},
	}, nil
}

// AntonymHandler handles antonym lookups
type AntonymHandler struct {
	client   *http.Client
	patterns []*regexp.Regexp
}

// NewAntonymHandler creates a new antonym handler
func NewAntonymHandler() *AntonymHandler {
	return &AntonymHandler{
		client: &http.Client{Timeout: 10 * time.Second},
		patterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)^antonym[s]?[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^ant[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^opposite\s+of[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^opposite[:\s]+(.+)$`),
		},
	}
}

func (h *AntonymHandler) Name() string {
	return "antonym"
}

func (h *AntonymHandler) Patterns() []*regexp.Regexp {
	return h.patterns
}

func (h *AntonymHandler) CanHandle(query string) bool {
	for _, p := range h.patterns {
		if p.MatchString(query) {
			return true
		}
	}
	return false
}

func (h *AntonymHandler) Handle(ctx context.Context, query string) (*Answer, error) {
	word := ""
	for _, p := range h.patterns {
		if matches := p.FindStringSubmatch(query); len(matches) > 1 {
			word = strings.TrimSpace(matches[1])
			break
		}
	}

	if word == "" {
		return nil, nil
	}

	// Use Datamuse API for antonyms
	apiURL := fmt.Sprintf("https://api.datamuse.com/words?rel_ant=%s&max=20", url.QueryEscape(word))

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", version.BrowserUserAgent)

	resp, err := h.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var data []struct {
		Word  string `json:"word"`
		Score int    `json:"score"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	if len(data) == 0 {
		return &Answer{
			Type:    AnswerTypeAntonym,
			Query:   query,
			Title:   fmt.Sprintf("Antonyms for: %s", word),
			Content: fmt.Sprintf("No antonyms found for '%s'", word),
		}, nil
	}

	// Build content
	antonyms := make([]string, 0, len(data))
	for _, item := range data {
		antonyms = append(antonyms, item.Word)
	}

	var content strings.Builder
	content.WriteString(fmt.Sprintf("<strong>Antonyms for \"%s\":</strong><br><br>", word))
	content.WriteString(strings.Join(antonyms, ", "))

	return &Answer{
		Type:      AnswerTypeAntonym,
		Query:     query,
		Title:     fmt.Sprintf("Antonyms for: %s", word),
		Content:   content.String(),
		Source:    "Datamuse API",
		SourceURL: "https://www.datamuse.com/api/",
		Data: map[string]interface{}{
			"word":     word,
			"antonyms": antonyms,
		},
	}, nil
}
