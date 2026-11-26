package cmd

import (
	"testing"

	"github.com/apimgr/search/src/common/i18n"
)

func TestCLIHelpStringsUseRequestedLocale(t *testing.T) {
	previous := langFlag
	t.Cleanup(func() { langFlag = previous })
	for _, lang := range i18n.DefaultSupportedLanguages() {
		if lang == "en" {
			continue
		}
		t.Run(lang, func(t *testing.T) {
			langFlag = lang
			got := localizedCLIText("Search the index")
			if got == "Search the index" {
				t.Errorf("CLI help description was not translated for %s", lang)
			}
		})
	}
}
