package widget

import (
	"context"
	"testing"
	"time"
)

func TestParseTranslateQuery(t *testing.T) {
	tests := []struct {
		name           string
		query          string
		wantNil        bool
		wantText       string
		wantSourceLang string
		wantTargetLang string
	}{
		{
			name:           "translate X to Y pattern",
			query:          "translate hello to spanish",
			wantText:       "hello",
			wantSourceLang: "auto",
			wantTargetLang: "es",
		},
		{
			name:           "translate X from Y to Z pattern",
			query:          "translate hello from english to spanish",
			wantText:       "hello",
			wantSourceLang: "en",
			wantTargetLang: "es",
		},
		{
			name:           "X in Y pattern",
			query:          "bonjour in english",
			wantText:       "bonjour",
			wantSourceLang: "auto",
			wantTargetLang: "en",
		},
		{
			name:           "how do you say X in Y pattern",
			query:          "how do you say goodbye in french",
			wantText:       "goodbye",
			wantSourceLang: "auto",
			wantTargetLang: "fr",
		},
		{
			name:           "what is X in Y pattern",
			query:          "what is hello in german",
			wantText:       "hello",
			wantSourceLang: "auto",
			wantTargetLang: "de",
		},
		{
			name:    "empty string returns nil",
			query:   "",
			wantNil: true,
		},
		{
			name:    "no translation pattern returns nil",
			query:   "hello world",
			wantNil: true,
		},
		{
			name:           "case insensitive Translate Hello To Spanish",
			query:          "Translate Hello To Spanish",
			wantText:       "Hello",
			wantSourceLang: "auto",
			wantTargetLang: "es",
		},
		{
			name:           "translate into is also supported",
			query:          "translate goodbye into french",
			wantText:       "goodbye",
			wantSourceLang: "auto",
			wantTargetLang: "fr",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseTranslateQuery(tt.query)
			if tt.wantNil {
				if got != nil {
					t.Errorf("ParseTranslateQuery(%q) = %+v, want nil", tt.query, got)
				}
				return
			}
			if got == nil {
				t.Fatalf("ParseTranslateQuery(%q) = nil, want non-nil", tt.query)
			}
			if got.Text != tt.wantText {
				t.Errorf("ParseTranslateQuery(%q).Text = %q, want %q", tt.query, got.Text, tt.wantText)
			}
			if got.SourceLang != tt.wantSourceLang {
				t.Errorf("ParseTranslateQuery(%q).SourceLang = %q, want %q", tt.query, got.SourceLang, tt.wantSourceLang)
			}
			if got.TargetLang != tt.wantTargetLang {
				t.Errorf("ParseTranslateQuery(%q).TargetLang = %q, want %q", tt.query, got.TargetLang, tt.wantTargetLang)
			}
		})
	}
}

func TestNormalizeLanguage(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"english maps to en", "english", "en"},
		{"spanish maps to es", "spanish", "es"},
		{"french maps to fr", "french", "fr"},
		{"german maps to de", "german", "de"},
		{"already 2-char code en unchanged", "en", "en"},
		{"already 2-char code zh unchanged", "zh", "zh"},
		{"unknown language returned unchanged", "unknown language", "unknown language"},
		{"whitespace trimmed before lookup", "  english  ", "en"},
		{"case insensitive lookup", "English", "en"},
		{"portuguese maps to pt", "portuguese", "pt"},
		{"russian maps to ru", "russian", "ru"},
		{"japanese maps to ja", "japanese", "ja"},
		{"korean maps to ko", "korean", "ko"},
		{"chinese maps to zh", "chinese", "zh"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeLanguage(tt.input)
			if got != tt.want {
				t.Errorf("normalizeLanguage(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestTranslateFetcherCacheDuration(t *testing.T) {
	f := NewTranslateFetcher()
	got := f.CacheDuration()
	if got != 1*time.Hour {
		t.Errorf("CacheDuration() = %v, want %v", got, 1*time.Hour)
	}
}

func TestTranslateFetcherWidgetType(t *testing.T) {
	f := NewTranslateFetcher()
	got := f.WidgetType()
	if got != WidgetTranslate {
		t.Errorf("WidgetType() = %q, want %q", got, WidgetTranslate)
	}
}

func TestSupportedLanguages(t *testing.T) {
	t.Run("non-empty list", func(t *testing.T) {
		if len(SupportedLanguages) == 0 {
			t.Error("SupportedLanguages should not be empty")
		}
	})

	t.Run("contains english entry", func(t *testing.T) {
		found := false
		for _, lang := range SupportedLanguages {
			if lang.Code == "en" {
				found = true
				break
			}
		}
		if !found {
			t.Error("SupportedLanguages should contain an entry with code 'en'")
		}
	})

	t.Run("contains spanish entry", func(t *testing.T) {
		found := false
		for _, lang := range SupportedLanguages {
			if lang.Code == "es" {
				found = true
				break
			}
		}
		if !found {
			t.Error("SupportedLanguages should contain an entry with code 'es'")
		}
	})

	t.Run("all entries have Code and Name populated", func(t *testing.T) {
		for _, lang := range SupportedLanguages {
			if lang.Code == "" {
				t.Errorf("language %q has empty Code", lang.Name)
			}
			if lang.Name == "" {
				t.Errorf("language with code %q has empty Name", lang.Code)
			}
		}
	})
}

func TestTranslateFetcherFetchMissingText(t *testing.T) {
	f := NewTranslateFetcher()
	ctx := context.Background()

	t.Run("empty params returns error WidgetData with required text message", func(t *testing.T) {
		data, err := f.Fetch(ctx, map[string]string{})
		if err != nil {
			t.Fatalf("Fetch() returned unexpected error: %v", err)
		}
		if data.Error == "" {
			t.Error("WidgetData.Error should be set when text is missing")
		}
		if data.Type != WidgetTranslate {
			t.Errorf("WidgetData.Type = %q, want %q", data.Type, WidgetTranslate)
		}
	})

	t.Run("unparseable query with no text param returns error", func(t *testing.T) {
		data, err := f.Fetch(ctx, map[string]string{"query": "hello world"})
		if err != nil {
			t.Fatalf("Fetch() returned unexpected error: %v", err)
		}
		if data.Error == "" {
			t.Error("WidgetData.Error should be set when query does not parse and no text param")
		}
	})
}
