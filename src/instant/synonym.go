package instant

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/apimgr/search/src/common/closeutil"

	"github.com/apimgr/search/src/cache"
	"github.com/apimgr/search/src/version"
)

// SynonymHandler handles synonym lookups
type SynonymHandler struct {
	client   *http.Client
	patterns []*regexp.Regexp
	endpoint string
	cache    cache.Cache
}

// NewSynonymHandler creates a new synonym handler
func NewSynonymHandler() *SynonymHandler {
	return &SynonymHandler{
		client:   &http.Client{Timeout: 10 * time.Second},
		endpoint: "https://api.datamuse.com/words",
		cache:    cache.NewMemoryCache(500, 24*time.Hour),
		patterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)^synonym[s]?\s+for[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^(.+?)\s+synonym[s]?$`),
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

func (h *SynonymHandler) HandleInstantQuery(ctx context.Context, query string) (*Answer, error) {
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

	data, err := h.fetchRelated(ctx, "rel_syn", word, "dp")
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return &Answer{
			Type: AnswerTypeSynonym, Query: query,
			Title:   fmt.Sprintf("Synonyms for: %s", word),
			Content: fmt.Sprintf("No synonyms found for '%s'", escapeHTML(word)),
		}, nil
	}

	words, groups := groupRelatedWords(data)
	content := renderRelatedWords("Synonyms", word, groups)

	return &Answer{
		Type:      AnswerTypeSynonym,
		Query:     query,
		Title:     fmt.Sprintf("Synonyms for: %s", word),
		Content:   content,
		Source:    "Datamuse API",
		SourceURL: "https://www.datamuse.com/api/",
		Data: map[string]interface{}{
			"word":     word,
			"synonyms": words,
			"groups":   groups,
		},
	}, nil
}

// AntonymHandler handles antonym lookups
type AntonymHandler struct {
	client   *http.Client
	patterns []*regexp.Regexp
	endpoint string
	cache    cache.Cache
}

// NewAntonymHandler creates a new antonym handler
func NewAntonymHandler() *AntonymHandler {
	return &AntonymHandler{
		client:   &http.Client{Timeout: 10 * time.Second},
		endpoint: "https://api.datamuse.com/words",
		cache:    cache.NewMemoryCache(500, 24*time.Hour),
		patterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)^antonym[s]?\s+of[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^(.+?)\s+antonym[s]?$`),
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

func (h *AntonymHandler) HandleInstantQuery(ctx context.Context, query string) (*Answer, error) {
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

	data, err := h.fetchRelated(ctx, "rel_ant", word, "dp")
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return &Answer{
			Type: AnswerTypeAntonym, Query: query,
			Title:   fmt.Sprintf("Antonyms for: %s", word),
			Content: fmt.Sprintf("No antonyms found for '%s'", escapeHTML(word)),
		}, nil
	}

	words, groups := groupRelatedWords(data)
	content := renderRelatedWords("Antonyms", word, groups)

	return &Answer{
		Type:      AnswerTypeAntonym,
		Query:     query,
		Title:     fmt.Sprintf("Antonyms for: %s", word),
		Content:   content,
		Source:    "Datamuse API",
		SourceURL: "https://www.datamuse.com/api/",
		Data: map[string]interface{}{
			"word":     word,
			"antonyms": words,
			"groups":   groups,
		},
	}, nil
}

func (h *SynonymHandler) fetchRelated(ctx context.Context, relation, word, metadata string) ([]relatedWord, error) {
	return fetchRelated(ctx, h.client, h.endpoint, h.cache, relation, word, metadata)
}

func (h *AntonymHandler) fetchRelated(ctx context.Context, relation, word, metadata string) ([]relatedWord, error) {
	return fetchRelated(ctx, h.client, h.endpoint, h.cache, relation, word, metadata)
}

func fetchRelated(ctx context.Context, client *http.Client, endpoint string, responseCache cache.Cache, relation, word, metadata string) ([]relatedWord, error) {
	word = strings.TrimSpace(word)
	if word == "" || len([]rune(word)) > 200 {
		return nil, fmt.Errorf("related-word query must contain 1 to 200 characters")
	}
	if endpoint == "" {
		endpoint = "https://api.datamuse.com/words"
	}
	digest := sha256.Sum256([]byte(strings.ToLower(word) + ":" + metadata))
	cacheKey := "instant:datamuse:" + relation + ":" + hex.EncodeToString(digest[:])
	if responseCache != nil {
		if cached, err := responseCache.Get(ctx, cacheKey); err == nil && len(cached) > 0 {
			return decodeRelatedWords(strings.NewReader(string(cached)))
		}
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" {
		return nil, fmt.Errorf("invalid related-word provider endpoint")
	}
	query := parsed.Query()
	query.Set(relation, word)
	query.Set("max", "20")
	query.Set("md", metadata)
	parsed.RawQuery = query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", version.BrowserUserAgent)
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("related-word provider request failed: %w", err)
	}
	defer closeutil.Close(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("related-word provider returned HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, (1<<20)+1))
	if err != nil {
		return nil, err
	}
	if len(body) > 1<<20 {
		return nil, fmt.Errorf("datamuse response exceeds 1048576 bytes")
	}
	data, err := decodeRelatedWords(strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	if responseCache != nil {
		_ = responseCache.Set(ctx, cacheKey, body, 24*time.Hour)
	}
	return data, nil
}

type relatedWord struct {
	Word        string   `json:"word"`
	Tags        []string `json:"tags"`
	Definitions []string `json:"defs"`
}

func groupRelatedWords(words []relatedWord) ([]string, map[string][]string) {
	groups := map[string][]string{}
	all := make([]string, 0, len(words))
	for _, item := range words {
		word := strings.TrimSpace(item.Word)
		if word == "" {
			continue
		}
		all = append(all, word)

		partOfSpeech := "Other"
		for _, tag := range item.Tags {
			switch tag {
			case "n":
				partOfSpeech = "Nouns"
			case "v":
				partOfSpeech = "Verbs"
			case "adj":
				partOfSpeech = "Adjectives"
			case "adv":
				partOfSpeech = "Adverbs"
			}
			if partOfSpeech != "Other" {
				break
			}
		}

		added := false
		for _, definition := range item.Definitions {
			definition = strings.TrimSpace(definition)
			parts := strings.SplitN(definition, "\t", 2)
			if len(parts) == 2 {
				definition = strings.TrimSpace(parts[1])
			}
			if definition == "" {
				continue
			}
			group := partOfSpeech + " — " + truncateRelatedDefinition(definition, 180)
			if !containsWord(groups[group], word) {
				groups[group] = append(groups[group], word)
			}
			added = true
		}
		if !added {
			groups[partOfSpeech] = append(groups[partOfSpeech], word)
		}
	}
	return all, groups
}

func containsWord(words []string, candidate string) bool {
	for _, word := range words {
		if strings.EqualFold(word, candidate) {
			return true
		}
	}
	return false
}

func truncateRelatedDefinition(definition string, maximum int) string {
	runes := []rune(definition)
	if len(runes) <= maximum {
		return definition
	}
	return string(runes[:maximum-1]) + "…"
}

func renderRelatedWords(kind, query string, groups map[string][]string) string {
	var content strings.Builder
	fmt.Fprintf(&content, "<strong>%s for \"%s\":</strong>", kind, escapeHTML(query))
	orderedGroups := make([]string, 0, len(groups))
	for group := range groups {
		orderedGroups = append(orderedGroups, group)
	}
	sort.Strings(orderedGroups)
	for _, group := range orderedGroups {
		words := groups[group]
		if len(words) == 0 {
			continue
		}
		content.WriteString("<section><h4>" + escapeHTML(group) + "</h4><ul>")
		for _, word := range words {
			content.WriteString("<li>" + escapeHTML(word) + "</li>")
		}
		content.WriteString("</ul></section>")
	}
	return content.String()
}

func decodeRelatedWords(body io.Reader) ([]relatedWord, error) {
	const maximumBytes = 1 << 20
	data, err := io.ReadAll(io.LimitReader(body, maximumBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maximumBytes {
		return nil, fmt.Errorf("datamuse response exceeds %d bytes", maximumBytes)
	}
	var words []relatedWord
	if err := json.Unmarshal(data, &words); err != nil {
		return nil, err
	}
	return words, nil
}
