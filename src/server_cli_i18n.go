package main

import (
	"os"
	"strings"

	"github.com/apimgr/search/src/common/i18n"
	"github.com/apimgr/search/src/config"
)

func serverCLILanguage() string {
	if flagLang != "" {
		return supportedCLILanguage(flagLang)
	}
	if cfg, err := config.Load(config.GetConfigPath()); err == nil && cfg.Server.I18n.DefaultLanguage != "" {
		return supportedCLILanguage(cfg.Server.I18n.DefaultLanguage)
	}
	for _, name := range []string{"SEARCH_LANG", "LC_ALL", "LANG"} {
		if value := os.Getenv(name); value != "" {
			return supportedCLILanguage(value)
		}
	}
	return "en"
}

func supportedCLILanguage(value string) string {
	lang := strings.ToLower(strings.TrimSpace(value))
	if cut, _, ok := strings.Cut(lang, "."); ok {
		lang = cut
	}
	if cut, _, ok := strings.Cut(lang, "_"); ok {
		lang = cut
	}
	if i18n.IsValidLanguageCode(lang) {
		return lang
	}
	return "en"
}

func serverCLIText(key string, args ...interface{}) string {
	return i18n.T(serverCLILanguage(), key, args...)
}

func serverCLITf(key string, args ...interface{}) string {
	return i18n.Tf(serverCLILanguage(), key, args...)
}
