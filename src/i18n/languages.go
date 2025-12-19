package i18n

// Language represents a supported language
type Language struct {
	Code       string // BCP 47 language code (e.g., "en", "de")
	Name       string // English name (e.g., "English", "German")
	NativeName string // Native name (e.g., "English", "Deutsch")
	RTL        bool   // Right-to-left language
}

// Languages is the map of all supported languages
var Languages = map[string]Language{
	"en": {
		Code:       "en",
		Name:       "English",
		NativeName: "English",
		RTL:        false,
	},
	"de": {
		Code:       "de",
		Name:       "German",
		NativeName: "Deutsch",
		RTL:        false,
	},
	"fr": {
		Code:       "fr",
		Name:       "French",
		NativeName: "Français",
		RTL:        false,
	},
	"es": {
		Code:       "es",
		Name:       "Spanish",
		NativeName: "Español",
		RTL:        false,
	},
	"it": {
		Code:       "it",
		Name:       "Italian",
		NativeName: "Italiano",
		RTL:        false,
	},
	"pt": {
		Code:       "pt",
		Name:       "Portuguese",
		NativeName: "Português",
		RTL:        false,
	},
	"nl": {
		Code:       "nl",
		Name:       "Dutch",
		NativeName: "Nederlands",
		RTL:        false,
	},
	"pl": {
		Code:       "pl",
		Name:       "Polish",
		NativeName: "Polski",
		RTL:        false,
	},
	"ru": {
		Code:       "ru",
		Name:       "Russian",
		NativeName: "Русский",
		RTL:        false,
	},
	"ja": {
		Code:       "ja",
		Name:       "Japanese",
		NativeName: "日本語",
		RTL:        false,
	},
	"zh": {
		Code:       "zh",
		Name:       "Chinese",
		NativeName: "中文",
		RTL:        false,
	},
	"ko": {
		Code:       "ko",
		Name:       "Korean",
		NativeName: "한국어",
		RTL:        false,
	},
	"ar": {
		Code:       "ar",
		Name:       "Arabic",
		NativeName: "العربية",
		RTL:        true,
	},
	"he": {
		Code:       "he",
		Name:       "Hebrew",
		NativeName: "עברית",
		RTL:        true,
	},
	"fa": {
		Code:       "fa",
		Name:       "Persian",
		NativeName: "فارسی",
		RTL:        true,
	},
	"ur": {
		Code:       "ur",
		Name:       "Urdu",
		NativeName: "اردو",
		RTL:        true,
	},
	"tr": {
		Code:       "tr",
		Name:       "Turkish",
		NativeName: "Türkçe",
		RTL:        false,
	},
	"vi": {
		Code:       "vi",
		Name:       "Vietnamese",
		NativeName: "Tiếng Việt",
		RTL:        false,
	},
	"th": {
		Code:       "th",
		Name:       "Thai",
		NativeName: "ไทย",
		RTL:        false,
	},
	"id": {
		Code:       "id",
		Name:       "Indonesian",
		NativeName: "Bahasa Indonesia",
		RTL:        false,
	},
	"uk": {
		Code:       "uk",
		Name:       "Ukrainian",
		NativeName: "Українська",
		RTL:        false,
	},
	"cs": {
		Code:       "cs",
		Name:       "Czech",
		NativeName: "Čeština",
		RTL:        false,
	},
	"sv": {
		Code:       "sv",
		Name:       "Swedish",
		NativeName: "Svenska",
		RTL:        false,
	},
	"da": {
		Code:       "da",
		Name:       "Danish",
		NativeName: "Dansk",
		RTL:        false,
	},
	"fi": {
		Code:       "fi",
		Name:       "Finnish",
		NativeName: "Suomi",
		RTL:        false,
	},
	"no": {
		Code:       "no",
		Name:       "Norwegian",
		NativeName: "Norsk",
		RTL:        false,
	},
	"hu": {
		Code:       "hu",
		Name:       "Hungarian",
		NativeName: "Magyar",
		RTL:        false,
	},
	"el": {
		Code:       "el",
		Name:       "Greek",
		NativeName: "Ελληνικά",
		RTL:        false,
	},
	"ro": {
		Code:       "ro",
		Name:       "Romanian",
		NativeName: "Română",
		RTL:        false,
	},
	"bg": {
		Code:       "bg",
		Name:       "Bulgarian",
		NativeName: "Български",
		RTL:        false,
	},
	"sk": {
		Code:       "sk",
		Name:       "Slovak",
		NativeName: "Slovenčina",
		RTL:        false,
	},
	"hr": {
		Code:       "hr",
		Name:       "Croatian",
		NativeName: "Hrvatski",
		RTL:        false,
	},
	"sr": {
		Code:       "sr",
		Name:       "Serbian",
		NativeName: "Српски",
		RTL:        false,
	},
	"sl": {
		Code:       "sl",
		Name:       "Slovenian",
		NativeName: "Slovenščina",
		RTL:        false,
	},
	"et": {
		Code:       "et",
		Name:       "Estonian",
		NativeName: "Eesti",
		RTL:        false,
	},
	"lv": {
		Code:       "lv",
		Name:       "Latvian",
		NativeName: "Latviešu",
		RTL:        false,
	},
	"lt": {
		Code:       "lt",
		Name:       "Lithuanian",
		NativeName: "Lietuvių",
		RTL:        false,
	},
}

// DefaultSupportedLanguages returns the default list of supported languages
func DefaultSupportedLanguages() []string {
	return []string{
		"en", "de", "fr", "es", "it", "pt", "nl", "pl", "ru", "ja", "zh",
	}
}

// AllLanguageCodes returns all available language codes
func AllLanguageCodes() []string {
	codes := make([]string, 0, len(Languages))
	for code := range Languages {
		codes = append(codes, code)
	}
	return codes
}

// GetLanguage returns language info by code
func GetLanguage(code string) (Language, bool) {
	lang, ok := Languages[code]
	return lang, ok
}

// IsValidLanguageCode checks if a language code is valid
func IsValidLanguageCode(code string) bool {
	_, ok := Languages[code]
	return ok
}

// RTLLanguages returns the list of RTL language codes
func RTLLanguages() []string {
	var rtl []string
	for code, lang := range Languages {
		if lang.RTL {
			rtl = append(rtl, code)
		}
	}
	return rtl
}
