package server

import (
	"testing"

	"github.com/apimgr/search/src/config"
)

func TestBlocklistSourcesAddsAndOverridesDefaults(t *testing.T) {
	sources := blocklistSources([]config.BlocklistSourceConfig{
		{Name: "firehol_level1", URL: "https://feeds.example.net/firehol", Type: "cidr", Enabled: false},
		{Name: "phishing_domains", URL: "https://feeds.example.net/domains", Type: "domain", Enabled: true},
		{Name: "invalid", URL: "http://feeds.example.net/domains", Type: "domain", Enabled: true},
	})
	if len(sources) != 3 {
		t.Fatalf("got %d sources, want 3 defaults/valid overrides", len(sources))
	}
	if sources[0].URL != "https://feeds.example.net/firehol" || sources[0].Enabled {
		t.Errorf("default source override = %#v", sources[0])
	}
	if sources[2].Name != "phishing_domains" || sources[2].Type != "domain" || !sources[2].Enabled {
		t.Errorf("custom domain source = %#v", sources[2])
	}
}
