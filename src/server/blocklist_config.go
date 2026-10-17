package server

import (
	"log/slog"
	"net/url"
	"strings"

	"github.com/apimgr/search/src/config"
	"github.com/apimgr/search/src/security"
)

func blocklistSources(configured []config.BlocklistSourceConfig) []security.BlocklistSource {
	sources := security.DefaultBlocklistSources()
	indices := make(map[string]int, len(sources))
	for index, source := range sources {
		indices[source.Name] = index
	}
	for _, source := range configured {
		name := strings.TrimSpace(source.Name)
		parsedURL, err := url.ParseRequestURI(strings.TrimSpace(source.URL))
		if !validBlocklistSourceName(name) || err != nil || parsedURL.Scheme != "https" || parsedURL.Host == "" || parsedURL.User != nil || (source.Type != "ip" && source.Type != "cidr" && source.Type != "domain") {
			slog.Warn("invalid blocklist source ignored")
			continue
		}
		configuredSource := security.BlocklistSource{Name: name, URL: parsedURL.String(), Type: source.Type, Enabled: source.Enabled}
		if index, exists := indices[name]; exists {
			sources[index] = configuredSource
			continue
		}
		indices[name] = len(sources)
		sources = append(sources, configuredSource)
	}
	return sources
}

func validBlocklistSourceName(name string) bool {
	if name == "" || len(name) > 64 {
		return false
	}
	for _, char := range name {
		if (char < 'a' || char > 'z') && (char < 'A' || char > 'Z') && (char < '0' || char > '9') && char != '-' && char != '_' {
			return false
		}
	}
	return true
}
