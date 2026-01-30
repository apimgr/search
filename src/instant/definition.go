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

	"github.com/apimgr/search/src/version"
)

// DefinitionHandler handles word definitions
type DefinitionHandler struct {
	client   *http.Client
	patterns []*regexp.Regexp
}

// NewDefinitionHandler creates a new definition handler
func NewDefinitionHandler() *DefinitionHandler {
	return &DefinitionHandler{
		client: &http.Client{Timeout: 10 * time.Second},
		patterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)^define[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^definition[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^meaning\s+of\s+(.+)$`),
			regexp.MustCompile(`(?i)^what\s+is\s+(.+)\??$`),
			regexp.MustCompile(`(?i)^what\s+does\s+(.+)\s+mean\??$`),
		},
	}
}

func (h *DefinitionHandler) Name() string {
	return "definition"
}

func (h *DefinitionHandler) Patterns() []*regexp.Regexp {
	return h.patterns
}

func (h *DefinitionHandler) CanHandle(query string) bool {
	for _, p := range h.patterns {
		if p.MatchString(query) {
			return true
		}
	}
	return false
}

func (h *DefinitionHandler) Handle(ctx context.Context, query string) (*Answer, error) {
	// Extract word from query
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

	// Use Free Dictionary API
	apiURL := fmt.Sprintf("https://api.dictionaryapi.dev/api/v2/entries/en/%s", url.PathEscape(word))

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

	if resp.StatusCode != http.StatusOK {
		return &Answer{
			Type:    AnswerTypeDefinition,
			Query:   query,
			Title:   fmt.Sprintf("Definition: %s", word),
			Content: fmt.Sprintf("No definition found for '%s'", word),
		}, nil
	}

	var data []struct {
		Word      string `json:"word"`
		Phonetic  string `json:"phonetic"`
		Phonetics []struct {
			Text  string `json:"text"`
			Audio string `json:"audio"`
		} `json:"phonetics"`
		Meanings []struct {
			PartOfSpeech string `json:"partOfSpeech"`
			Definitions  []struct {
				Definition string   `json:"definition"`
				Example    string   `json:"example"`
				Synonyms   []string `json:"synonyms"`
				Antonyms   []string `json:"antonyms"`
			} `json:"definitions"`
		} `json:"meanings"`
		Origin string `json:"origin"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	if len(data) == 0 {
		return &Answer{
			Type:    AnswerTypeDefinition,
			Query:   query,
			Title:   fmt.Sprintf("Definition: %s", word),
			Content: fmt.Sprintf("No definition found for '%s'", word),
		}, nil
	}

	entry := data[0]

	// Build content
	var content strings.Builder
	content.WriteString(fmt.Sprintf("<strong>%s</strong>", entry.Word))

	if entry.Phonetic != "" {
		content.WriteString(fmt.Sprintf(" <span class=\"phonetic\">%s</span>", entry.Phonetic))
	}

	content.WriteString("<br><br>")

	for _, meaning := range entry.Meanings {
		content.WriteString(fmt.Sprintf("<em>%s</em><br>", meaning.PartOfSpeech))

		for i, def := range meaning.Definitions {
			if i >= 3 {
				break
			}
			content.WriteString(fmt.Sprintf("%d. %s<br>", i+1, def.Definition))
			if def.Example != "" {
				content.WriteString(fmt.Sprintf("   <span class=\"example\">Example: \"%s\"</span><br>", def.Example))
			}
		}
		content.WriteString("<br>")
	}

	if entry.Origin != "" {
		content.WriteString(fmt.Sprintf("<strong>Origin:</strong> %s", entry.Origin))
	}

	return &Answer{
		Type:      AnswerTypeDefinition,
		Query:     query,
		Title:     fmt.Sprintf("Definition: %s", entry.Word),
		Content:   content.String(),
		Source:    "Free Dictionary API",
		SourceURL: fmt.Sprintf("https://dictionaryapi.dev/"),
		Data: map[string]interface{}{
			"word":     entry.Word,
			"phonetic": entry.Phonetic,
			"meanings": entry.Meanings,
		},
	}, nil
}

// DictionaryHandler is an alias for DefinitionHandler with different patterns
type DictionaryHandler struct {
	*DefinitionHandler
}

// NewDictionaryHandler creates a new dictionary handler
func NewDictionaryHandler() *DictionaryHandler {
	h := &DictionaryHandler{
		DefinitionHandler: NewDefinitionHandler(),
	}
	h.patterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)^dictionary[:\s]+(.+)$`),
		regexp.MustCompile(`(?i)^dict[:\s]+(.+)$`),
		regexp.MustCompile(`(?i)^lookup[:\s]+(.+)$`),
	}
	return h
}

func (h *DictionaryHandler) Name() string {
	return "dictionary"
}
