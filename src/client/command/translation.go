package cmd

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strings"

	"github.com/apimgr/search/src/common/i18n"
)

func cliLanguage() string {
	lang := strings.TrimSpace(langFlag)
	if lang == "" {
		lang = strings.TrimSpace(os.Getenv("SEARCH_LANG"))
	}
	if lang == "" {
		lang = strings.TrimSpace(os.Getenv("LANG"))
	}
	manager, err := i18n.CachedDefaultManager()
	if err != nil || manager == nil {
		return "en"
	}
	return manager.ResolveSupportedLanguage(lang)
}

func localizedCLIText(source string) string {
	lang := cliLanguage()
	switch source {
	case "Error:":
		return i18n.T(lang, "common.error") + ":"
	case "Preferences updated":
		return i18n.T(lang, "common.saved")
	}
	key := cliTranslationKey(source)
	translated := i18n.T(lang, key)
	if translated == key {
		return source
	}
	return translated
}

// LocalizedCLIText translates a static client message for the selected locale.
func LocalizedCLIText(source string) string { return localizedCLIText(source) }

func localizedCLITextf(source string, replacements ...interface{}) string {
	key := cliTranslationKey(source)
	translated := i18n.T(cliLanguage(), key)
	if translated == key {
		translated = source
	}
	for index := 0; index+1 < len(replacements); index += 2 {
		name := fmt.Sprint(replacements[index])
		translated = strings.ReplaceAll(translated, "{"+name+"}", fmt.Sprint(replacements[index+1]))
	}
	return translated
}

func cliTranslationKey(source string) string {
	var slug strings.Builder
	lastUnderscore := false
	for _, char := range strings.ToLower(source) {
		if slug.Len() >= 48 {
			break
		}
		if char >= 'a' && char <= 'z' || char >= '0' && char <= '9' {
			slug.WriteRune(char)
			lastUnderscore = false
			continue
		}
		if slug.Len() > 0 && !lastUnderscore {
			slug.WriteByte('_')
			lastUnderscore = true
		}
	}
	prefix := strings.Trim(slug.String(), "_")
	if prefix == "" {
		prefix = "text"
	}
	digest := sha256.Sum256([]byte(source))
	key := "client.cli." + prefix + "_" + hex.EncodeToString(digest[:6])
	return key
}
