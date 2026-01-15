package widget

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// TranslateFetcher fetches translations
type TranslateFetcher struct {
	httpClient *http.Client
}

// TranslateData represents translation result
type TranslateData struct {
	SourceLang     string `json:"source_lang"`
	TargetLang     string `json:"target_lang"`
	SourceText     string `json:"source_text"`
	TranslatedText string `json:"translated_text"`
	DetectedLang   string `json:"detected_lang,omitempty"`
}

// NewTranslateFetcher creates a new translate fetcher
func NewTranslateFetcher() *TranslateFetcher {
	return &TranslateFetcher{
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

// Fetch fetches translation
func (f *TranslateFetcher) Fetch(ctx context.Context, params map[string]string) (*WidgetData, error) {
	text := params["text"]
	if text == "" {
		return &WidgetData{
			Type:      WidgetTranslate,
			Error:     "text parameter required",
			UpdatedAt: time.Now(),
		}, nil
	}

	sourceLang := strings.ToLower(params["from"])
	targetLang := strings.ToLower(params["to"])
	if targetLang == "" {
		targetLang = "en"
	}
	if sourceLang == "" {
		sourceLang = "auto"
	}

	// Use LibreTranslate public API (free, open source)
	// Note: In production, you'd want to self-host or use a paid service
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
		// Fall back to MyMemory Translation API
		return f.fetchFromMyMemory(ctx, text, sourceLang, targetLang)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Fall back to MyMemory
		return f.fetchFromMyMemory(ctx, text, sourceLang, targetLang)
	}

	var result struct {
		TranslatedText string `json:"translatedText"`
		DetectedLang   struct {
			Language   string  `json:"language"`
			Confidence float64 `json:"confidence"`
		} `json:"detectedLanguage"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return f.fetchFromMyMemory(ctx, text, sourceLang, targetLang)
	}

	data := &TranslateData{
		SourceLang:     sourceLang,
		TargetLang:     targetLang,
		SourceText:     text,
		TranslatedText: result.TranslatedText,
		DetectedLang:   result.DetectedLang.Language,
	}

	return &WidgetData{
		Type:      WidgetTranslate,
		Data:      data,
		UpdatedAt: time.Now(),
	}, nil
}

// fetchFromMyMemory uses MyMemory as fallback translation service
func (f *TranslateFetcher) fetchFromMyMemory(ctx context.Context, text, sourceLang, targetLang string) (*WidgetData, error) {
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
		ResponseStatus int    `json:"responseStatus"`
		ResponseData   struct {
			TranslatedText string `json:"translatedText"`
		} `json:"responseData"`
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

	data := &TranslateData{
		SourceLang:     sourceLang,
		TargetLang:     targetLang,
		SourceText:     text,
		TranslatedText: result.ResponseData.TranslatedText,
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
