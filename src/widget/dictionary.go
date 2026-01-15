package widget

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// DictionaryFetcher fetches word definitions
type DictionaryFetcher struct {
	httpClient *http.Client
}

// DictionaryData represents dictionary lookup result
type DictionaryData struct {
	Word        string                  `json:"word"`
	Phonetic    string                  `json:"phonetic,omitempty"`
	Audio       string                  `json:"audio,omitempty"`
	Meanings    []DictionaryMeaning     `json:"meanings"`
	Synonyms    []string                `json:"synonyms,omitempty"`
	Antonyms    []string                `json:"antonyms,omitempty"`
}

// DictionaryMeaning represents a word meaning
type DictionaryMeaning struct {
	PartOfSpeech string                 `json:"part_of_speech"`
	Definitions  []DictionaryDefinition `json:"definitions"`
}

// DictionaryDefinition represents a single definition
type DictionaryDefinition struct {
	Definition string   `json:"definition"`
	Example    string   `json:"example,omitempty"`
	Synonyms   []string `json:"synonyms,omitempty"`
}

// NewDictionaryFetcher creates a new dictionary fetcher
func NewDictionaryFetcher() *DictionaryFetcher {
	return &DictionaryFetcher{
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// Fetch fetches dictionary definition
func (f *DictionaryFetcher) Fetch(ctx context.Context, params map[string]string) (*WidgetData, error) {
	word := params["word"]
	if word == "" {
		return &WidgetData{
			Type:      WidgetDictionary,
			Error:     "word parameter required",
			UpdatedAt: time.Now(),
		}, nil
	}

	// Use Free Dictionary API
	apiURL := fmt.Sprintf("https://api.dictionaryapi.dev/api/v2/entries/en/%s",
		url.PathEscape(word))

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := f.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return &WidgetData{
			Type:      WidgetDictionary,
			Error:     "word not found",
			UpdatedAt: time.Now(),
		}, nil
	}

	var results []struct {
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
			} `json:"definitions"`
			Synonyms []string `json:"synonyms"`
			Antonyms []string `json:"antonyms"`
		} `json:"meanings"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		return nil, err
	}

	if len(results) == 0 {
		return &WidgetData{
			Type:      WidgetDictionary,
			Error:     "no definitions found",
			UpdatedAt: time.Now(),
		}, nil
	}

	result := results[0]
	data := &DictionaryData{
		Word:     result.Word,
		Phonetic: result.Phonetic,
	}

	// Get audio URL
	for _, p := range result.Phonetics {
		if p.Audio != "" {
			data.Audio = p.Audio
			break
		}
	}

	// Convert meanings
	for _, m := range result.Meanings {
		meaning := DictionaryMeaning{
			PartOfSpeech: m.PartOfSpeech,
		}
		for _, d := range m.Definitions {
			meaning.Definitions = append(meaning.Definitions, DictionaryDefinition{
				Definition: d.Definition,
				Example:    d.Example,
				Synonyms:   d.Synonyms,
			})
		}
		data.Meanings = append(data.Meanings, meaning)

		// Collect synonyms and antonyms
		data.Synonyms = append(data.Synonyms, m.Synonyms...)
		data.Antonyms = append(data.Antonyms, m.Antonyms...)
	}

	// Deduplicate synonyms/antonyms
	data.Synonyms = uniqueStrings(data.Synonyms)
	data.Antonyms = uniqueStrings(data.Antonyms)

	// Limit to first 10
	if len(data.Synonyms) > 10 {
		data.Synonyms = data.Synonyms[:10]
	}
	if len(data.Antonyms) > 10 {
		data.Antonyms = data.Antonyms[:10]
	}

	return &WidgetData{
		Type:      WidgetDictionary,
		Data:      data,
		UpdatedAt: time.Now(),
	}, nil
}

// uniqueStrings removes duplicates from a string slice
func uniqueStrings(input []string) []string {
	seen := make(map[string]bool)
	var result []string
	for _, s := range input {
		if !seen[s] {
			seen[s] = true
			result = append(result, s)
		}
	}
	return result
}

// CacheDuration returns how long to cache dictionary data
func (f *DictionaryFetcher) CacheDuration() time.Duration {
	return 24 * time.Hour
}

// WidgetType returns the widget type
func (f *DictionaryFetcher) WidgetType() WidgetType {
	return WidgetDictionary
}
