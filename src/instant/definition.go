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

	"github.com/apimgr/search/src/common/closeutil"
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
			regexp.MustCompile(`(?i)^(.+?)\s+definition$`),
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

func (h *DefinitionHandler) HandleInstantQuery(ctx context.Context, query string) (*Answer, error) {
	// Extract word from query
	word := ""
	for _, p := range h.patterns {
		if matches := p.FindStringSubmatch(query); len(matches) > 1 {
			word = unquoteDefinitionWord(matches[1])
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
	defer closeutil.Close(resp.Body)

	if resp.StatusCode != http.StatusOK {
		// Not found — return nil so the caller treats this as no instant answer
		return nil, nil
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
		// No definition — return nil so the caller treats this as no instant answer
		return nil, nil
	}

	entry := data[0]

	// Build content
	var content strings.Builder
	fmt.Fprintf(&content, "<strong>%s</strong>", escapeHTML(entry.Word))

	if entry.Phonetic != "" {
		fmt.Fprintf(&content, " <span class=\"phonetic\">%s</span>", escapeHTML(entry.Phonetic))
	}
	for _, phonetic := range entry.Phonetics {
		if audioURL := safePronunciationURL(phonetic.Audio); audioURL != "" {
			fmt.Fprintf(&content, ` <audio controls preload="none" aria-label="Pronunciation of %s" src="%s">Audio pronunciation</audio>`, escapeHTML(entry.Word), escapeHTML(audioURL))
			break
		}
	}

	content.WriteString("<br><br>")

	for _, meaning := range entry.Meanings {
		fmt.Fprintf(&content, "<em>%s</em><br>", escapeHTML(meaning.PartOfSpeech))

		for i, def := range meaning.Definitions {
			if i >= 3 {
				break
			}
			fmt.Fprintf(&content, "%d. %s<br>", i+1, escapeHTML(def.Definition))
			if def.Example != "" {
				fmt.Fprintf(&content, "   <span class=\"example\">Example: \"%s\"</span><br>", escapeHTML(def.Example))
			}
		}
		content.WriteString("<br>")
	}

	if entry.Origin != "" {
		fmt.Fprintf(&content, "<strong>Origin:</strong> %s", escapeHTML(entry.Origin))
	}

	return &Answer{
		Type:      AnswerTypeDefinition,
		Query:     query,
		Title:     fmt.Sprintf("Definition: %s", escapeHTML(entry.Word)),
		Content:   content.String(),
		Source:    "Free Dictionary API",
		SourceURL: "https://dictionaryapi.dev/",
		Data: map[string]interface{}{
			"word":     entry.Word,
			"phonetic": entry.Phonetic,
			"meanings": entry.Meanings,
		},
	}, nil
}

func unquoteDefinitionWord(word string) string {
	word = strings.TrimSpace(word)
	if len(word) >= 2 {
		first, last := word[0], word[len(word)-1]
		if (first == '"' && last == '"') || (first == '\'' && last == '\'') {
			word = strings.TrimSpace(word[1 : len(word)-1])
		}
	}
	return word
}

func safePronunciationURL(raw string) string {
	parsed, err := url.ParseRequestURI(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
		return ""
	}
	return parsed.String()
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
