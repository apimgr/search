package widget

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// TranslateFetcher fetches translations
type TranslateFetcher struct {
	httpClient *http.Client
}

// TranslateData represents translation result
type TranslateData struct {
	SourceLang     string  `json:"source_lang"`
	TargetLang     string  `json:"target_lang"`
	SourceText     string  `json:"source_text"`
	TranslatedText string  `json:"translated_text"`
	DetectedLang   string  `json:"detected_lang,omitempty"`
	Confidence     float64 `json:"confidence,omitempty"`
	Provider       string  `json:"provider,omitempty"`
}

// TranslateQuery represents a parsed translation query
type TranslateQuery struct {
	Text       string
	SourceLang string
	TargetLang string
}

// Language name to code mapping
var languageNameToCode = map[string]string{
	"english":    "en",
	"spanish":    "es",
	"french":     "fr",
	"german":     "de",
	"italian":    "it",
	"portuguese": "pt",
	"russian":    "ru",
	"japanese":   "ja",
	"korean":     "ko",
	"chinese":    "zh",
	"arabic":     "ar",
	"hindi":      "hi",
	"dutch":      "nl",
	"polish":     "pl",
	"turkish":    "tr",
	"vietnamese": "vi",
	"thai":       "th",
	"indonesian": "id",
	"swedish":    "sv",
	"danish":     "da",
	"norwegian":  "no",
	"finnish":    "fi",
	"greek":      "el",
	"hebrew":     "he",
	"czech":      "cs",
	"hungarian":  "hu",
	"romanian":   "ro",
	"ukrainian":  "uk",
	"malay":      "ms",
	"tagalog":    "tl",
	"filipino":   "tl",
}

// Regex patterns for natural language translation queries
var (
	// "translate X to Y" or "translate X into Y"
	translateToPattern = regexp.MustCompile(`(?i)^translate\s+(.+?)\s+(?:to|into)\s+(\w+)$`)
	// "translate X from Y to Z"
	translateFromToPattern = regexp.MustCompile(`(?i)^translate\s+(.+?)\s+from\s+(\w+)\s+(?:to|into)\s+(\w+)$`)
	// "X in Y" (e.g., "bonjour in english", "hello in spanish")
	inLanguagePattern = regexp.MustCompile(`(?i)^(.+?)\s+in\s+(\w+)$`)
	// "how do you say X in Y"
	howDoYouSayPattern = regexp.MustCompile(`(?i)^how\s+(?:do\s+you\s+say|to\s+say)\s+(.+?)\s+in\s+(\w+)\??$`)
	// "what is X in Y"
	whatIsPattern = regexp.MustCompile(`(?i)^what\s+is\s+(.+?)\s+in\s+(\w+)\??$`)
)

// ParseTranslateQuery parses natural language translation queries
func ParseTranslateQuery(query string) *TranslateQuery {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil
	}

	// Try "translate X from Y to Z" pattern
	if matches := translateFromToPattern.FindStringSubmatch(query); matches != nil {
		return &TranslateQuery{
			Text:       strings.TrimSpace(matches[1]),
			SourceLang: normalizeLanguage(matches[2]),
			TargetLang: normalizeLanguage(matches[3]),
		}
	}

	// Try "translate X to Y" pattern
	if matches := translateToPattern.FindStringSubmatch(query); matches != nil {
		return &TranslateQuery{
			Text:       strings.TrimSpace(matches[1]),
			SourceLang: "auto",
			TargetLang: normalizeLanguage(matches[2]),
		}
	}

	// Try "how do you say X in Y" pattern
	if matches := howDoYouSayPattern.FindStringSubmatch(query); matches != nil {
		return &TranslateQuery{
			Text:       strings.TrimSpace(matches[1]),
			SourceLang: "auto",
			TargetLang: normalizeLanguage(matches[2]),
		}
	}

	// Try "what is X in Y" pattern
	if matches := whatIsPattern.FindStringSubmatch(query); matches != nil {
		return &TranslateQuery{
			Text:       strings.TrimSpace(matches[1]),
			SourceLang: "auto",
			TargetLang: normalizeLanguage(matches[2]),
		}
	}

	// Try "X in Y" pattern (must be last as it's most general)
	if matches := inLanguagePattern.FindStringSubmatch(query); matches != nil {
		targetLang := normalizeLanguage(matches[2])
		// Only match if it looks like a valid language
		if targetLang != matches[2] || len(matches[2]) == 2 {
			return &TranslateQuery{
				Text:       strings.TrimSpace(matches[1]),
				SourceLang: "auto",
				TargetLang: targetLang,
			}
		}
	}

	return nil
}

// normalizeLanguage converts language names or codes to standardized codes
func normalizeLanguage(lang string) string {
	lang = strings.ToLower(strings.TrimSpace(lang))

	// Check if it's already a valid 2-letter code
	if len(lang) == 2 {
		return lang
	}

	// Look up in language name mapping
	if code, ok := languageNameToCode[lang]; ok {
		return code
	}

	return lang
}

// NewTranslateFetcher creates a new translate fetcher
func NewTranslateFetcher() *TranslateFetcher {
	return &TranslateFetcher{
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

// Fetch fetches translation
// Supports params:
//   - text: text to translate (required unless query is provided)
//   - query: natural language query (e.g., "translate hello to spanish")
//   - from / source_lang: source language code or name (default: auto)
//   - to / target_lang: target language code or name (default: en)
func (f *TranslateFetcher) Fetch(ctx context.Context, params map[string]string) (*WidgetData, error) {
	var text, sourceLang, targetLang string

	// Check for natural language query first
	if query := params["query"]; query != "" {
		if parsed := ParseTranslateQuery(query); parsed != nil {
			text = parsed.Text
			sourceLang = parsed.SourceLang
			targetLang = parsed.TargetLang
		}
	}

	// Fall back to explicit params if query didn't parse
	if text == "" {
		text = params["text"]
	}

	if text == "" {
		return &WidgetData{
			Type:      WidgetTranslate,
			Error:     "text parameter required (or use query for natural language)",
			UpdatedAt: time.Now(),
		}, nil
	}

	// Support both "from"/"to" and "source_lang"/"target_lang" param names
	if sourceLang == "" {
		sourceLang = params["from"]
		if sourceLang == "" {
			sourceLang = params["source_lang"]
		}
	}
	if targetLang == "" {
		targetLang = params["to"]
		if targetLang == "" {
			targetLang = params["target_lang"]
		}
	}

	// Normalize language codes
	if sourceLang != "" {
		sourceLang = normalizeLanguage(sourceLang)
	} else {
		sourceLang = "auto"
	}
	if targetLang != "" {
		targetLang = normalizeLanguage(targetLang)
	} else {
		targetLang = "en"
	}

	// Try Lingva Translate API first (free, no API key required)
	data, err := f.fetchFromLingva(ctx, text, sourceLang, targetLang)
	if err == nil && data.Error == "" {
		return data, nil
	}

	// Fall back to LibreTranslate
	data, err = f.fetchFromLibreTranslate(ctx, text, sourceLang, targetLang)
	if err == nil && data.Error == "" {
		return data, nil
	}

	// Fall back to MyMemory Translation API
	return f.fetchFromMyMemory(ctx, text, sourceLang, targetLang)
}

// fetchFromLingva uses Lingva Translate API (free, open source)
func (f *TranslateFetcher) fetchFromLingva(ctx context.Context, text, sourceLang, targetLang string) (*WidgetData, error) {
	// Lingva uses "auto" for auto-detection
	source := sourceLang
	if source == "auto" {
		source = "auto"
	}

	// URL encode the text
	apiURL := fmt.Sprintf("https://lingva.ml/api/v1/%s/%s/%s",
		url.PathEscape(source),
		url.PathEscape(targetLang),
		url.PathEscape(text))

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := f.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return &WidgetData{
			Type:      WidgetTranslate,
			Error:     fmt.Sprintf("Lingva API returned status %d", resp.StatusCode),
			UpdatedAt: time.Now(),
		}, nil
	}

	var result struct {
		Translation string `json:"translation"`
		Info        struct {
			DetectedSource string `json:"detectedSource"`
		} `json:"info"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	detectedLang := result.Info.DetectedSource
	if detectedLang == "" && sourceLang != "auto" {
		detectedLang = sourceLang
	}

	data := &TranslateData{
		SourceLang:     sourceLang,
		TargetLang:     targetLang,
		SourceText:     text,
		TranslatedText: result.Translation,
		DetectedLang:   detectedLang,
		Provider:       "lingva",
	}

	return &WidgetData{
		Type:      WidgetTranslate,
		Data:      data,
		UpdatedAt: time.Now(),
	}, nil
}

// fetchFromLibreTranslate uses LibreTranslate public API
func (f *TranslateFetcher) fetchFromLibreTranslate(ctx context.Context, text, sourceLang, targetLang string) (*WidgetData, error) {
	apiURL := "https://libretranslate.com/translate"

	payload := url.Values{}
	payload.Set("q", text)
	payload.Set("source", sourceLang)
	payload.Set("target", targetLang)

	req, err := http.NewRequestWithContext(ctx, "POST", apiURL,
		strings.NewReader(payload.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := f.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return &WidgetData{
			Type:      WidgetTranslate,
			Error:     fmt.Sprintf("LibreTranslate API returned status %d", resp.StatusCode),
			UpdatedAt: time.Now(),
		}, nil
	}

	var result struct {
		TranslatedText string `json:"translatedText"`
		DetectedLang   struct {
			Language   string  `json:"language"`
			Confidence float64 `json:"confidence"`
		} `json:"detectedLanguage"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	data := &TranslateData{
		SourceLang:     sourceLang,
		TargetLang:     targetLang,
		SourceText:     text,
		TranslatedText: result.TranslatedText,
		DetectedLang:   result.DetectedLang.Language,
		Confidence:     result.DetectedLang.Confidence,
		Provider:       "libretranslate",
	}

	return &WidgetData{
		Type:      WidgetTranslate,
		Data:      data,
		UpdatedAt: time.Now(),
	}, nil
}

// fetchFromMyMemory uses MyMemory as fallback translation service
func (f *TranslateFetcher) fetchFromMyMemory(ctx context.Context, text, sourceLang, targetLang string) (*WidgetData, error) {
	originalSource := sourceLang
	if sourceLang == "auto" {
		sourceLang = "en" // Default to English if auto-detect not supported
	}

	langPair := fmt.Sprintf("%s|%s", sourceLang, targetLang)
	apiURL := fmt.Sprintf("https://api.mymemory.translated.net/get?q=%s&langpair=%s",
		url.QueryEscape(text), url.QueryEscape(langPair))

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := f.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		ResponseStatus int `json:"responseStatus"`
		ResponseData   struct {
			TranslatedText string  `json:"translatedText"`
			Match          float64 `json:"match"`
		} `json:"responseData"`
		DetectedLanguage string `json:"detectedLanguage,omitempty"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	if result.ResponseStatus != 200 {
		return &WidgetData{
			Type:      WidgetTranslate,
			Error:     "translation failed",
			UpdatedAt: time.Now(),
		}, nil
	}

	detectedLang := result.DetectedLanguage
	if detectedLang == "" {
		detectedLang = sourceLang
	}

	data := &TranslateData{
		SourceLang:     originalSource,
		TargetLang:     targetLang,
		SourceText:     text,
		TranslatedText: result.ResponseData.TranslatedText,
		DetectedLang:   detectedLang,
		Confidence:     result.ResponseData.Match,
		Provider:       "mymemory",
	}

	return &WidgetData{
		Type:      WidgetTranslate,
		Data:      data,
		UpdatedAt: time.Now(),
	}, nil
}

// CacheDuration returns how long to cache translation data
func (f *TranslateFetcher) CacheDuration() time.Duration {
	return 1 * time.Hour
}

// WidgetType returns the widget type
func (f *TranslateFetcher) WidgetType() WidgetType {
	return WidgetTranslate
}

// SupportedLanguages returns common translation languages
var SupportedLanguages = []struct {
	Code string `json:"code"`
	Name string `json:"name"`
}{
	{"en", "English"},
	{"es", "Spanish"},
	{"fr", "French"},
	{"de", "German"},
	{"it", "Italian"},
	{"pt", "Portuguese"},
	{"ru", "Russian"},
	{"ja", "Japanese"},
	{"ko", "Korean"},
	{"zh", "Chinese"},
	{"ar", "Arabic"},
	{"hi", "Hindi"},
	{"nl", "Dutch"},
	{"pl", "Polish"},
	{"tr", "Turkish"},
	{"vi", "Vietnamese"},
	{"th", "Thai"},
	{"id", "Indonesian"},
	{"sv", "Swedish"},
	{"da", "Danish"},
}
