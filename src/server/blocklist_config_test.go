package server

import (
	"testing"

	"github.com/apimgr/search/src/config"
)

func TestBlocklistSourcesKeepsBuiltInAndAddsOnlyExtraSources(t *testing.T) {
	sources := blocklistSources([]config.BlocklistSourceConfig{
		{ID: "firehol_level1", URL: "https://feeds.example.net/firehol", Type: "cidr", Enabled: false},
		{ID: "FIREHOL_LEVEL1", URL: "https://feeds.example.net/firehol", Type: "cidr", Enabled: false},
		{ID: "phishing_domains", URL: "https://feeds.example.net/domains", Type: "domain", Enabled: true},
		{ID: "UpperCase", URL: "https://feeds.example.net/domains", Type: "domain", Enabled: true},
		{ID: "hyphen-name", URL: "https://feeds.example.net/domains", Type: "domain", Enabled: true},
		{ID: "invalid", URL: "http://feeds.example.net/domains", Type: "domain", Enabled: true},
	})
	if len(sources) != 2 {
		t.Fatalf("got %d sources, want built-in and one valid extra", len(sources))
	}
	if sources[0].URL != "https://raw.githubusercontent.com/firehol/blocklist-ipsets/master/firehol_level1.netset" || !sources[0].Enabled {
		t.Errorf("built-in source changed = %#v", sources[0])
	}
	if sources[1].Name != "phishing_domains" || sources[1].Type != "domain" || !sources[1].Enabled {
		t.Errorf("custom domain source = %#v", sources[1])
	}
}
