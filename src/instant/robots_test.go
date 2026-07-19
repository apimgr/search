package instant

import "testing"

// TestParseRobotsTxt covers RFC 9309 group parsing: consecutive User-agent
// lines with no intervening directive lines form a single group, and the
// directive block that follows applies to every agent in that group.
func TestParseRobotsTxt(t *testing.T) {
	tests := []struct {
		name          string
		content       string
		wantAgents    int
		wantSitemaps  int
		checkAgent    string
		wantDirective int
	}{
		{
			name:          "single user agent with directives",
			content:       "User-agent: *\nDisallow: /admin\nAllow: /public\n",
			wantAgents:    1,
			checkAgent:    "*",
			wantDirective: 2,
		},
		{
			name: "grouped user agents share following directives",
			content: "User-agent: AgentA\n" +
				"User-agent: AgentB\n" +
				"User-agent: AgentC\n" +
				"Disallow: /private\n" +
				"Allow: /public\n",
			wantAgents:    3,
			checkAgent:    "AgentA",
			wantDirective: 2,
		},
		{
			name: "grouped user agents all get the shared directives, not just the last",
			content: "User-agent: AgentA\n" +
				"User-agent: AgentB\n" +
				"Disallow: /private\n",
			wantAgents:    2,
			checkAgent:    "AgentB",
			wantDirective: 1,
		},
		{
			name: "new group starts after a directive block",
			content: "User-agent: AgentA\n" +
				"Disallow: /a\n" +
				"User-agent: AgentB\n" +
				"Disallow: /b\n",
			wantAgents:    2,
			checkAgent:    "AgentA",
			wantDirective: 1,
		},
		{
			name:          "sitemap directive parsed independently of groups",
			content:       "User-agent: *\nDisallow: /admin\nSitemap: https://example.com/sitemap.xml\n",
			wantAgents:    1,
			wantSitemaps:  1,
			checkAgent:    "*",
			wantDirective: 1,
		},
		{
			name:          "comments and blank lines ignored",
			content:       "# comment\n\nUser-agent: *\n\nDisallow: /x\n",
			wantAgents:    1,
			checkAgent:    "*",
			wantDirective: 1,
		},
		{
			name:       "empty content",
			content:    "",
			wantAgents: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseRobotsTxt(tt.content)

			if len(got.UserAgents) != tt.wantAgents {
				t.Errorf("parseRobotsTxt() UserAgents count = %d, want %d", len(got.UserAgents), tt.wantAgents)
			}
			if len(got.Sitemaps) != tt.wantSitemaps {
				t.Errorf("parseRobotsTxt() Sitemaps count = %d, want %d", len(got.Sitemaps), tt.wantSitemaps)
			}

			if tt.checkAgent == "" {
				return
			}

			var found *RobotUserAgent
			for i := range got.UserAgents {
				if got.UserAgents[i].Agent == tt.checkAgent {
					found = &got.UserAgents[i]
					break
				}
			}
			if found == nil {
				t.Fatalf("parseRobotsTxt() did not find expected agent %q in %+v", tt.checkAgent, got.UserAgents)
			}
			if len(found.Directives) != tt.wantDirective {
				t.Errorf("parseRobotsTxt() agent %q Directives count = %d, want %d", tt.checkAgent, len(found.Directives), tt.wantDirective)
			}
		})
	}
}

// TestParseRobotsTxtPinterestGroupedAgents reproduces a real-world pattern
// (as served by www.pinterest.com/robots.txt at the time of writing): a long
// run of User-agent lines followed by a shared directive block, ending with
// a separate block of Disallow-only directives. Every agent in the group
// must receive the group's shared directives, not just the last one seen.
func TestParseRobotsTxtPinterestGroupedAgents(t *testing.T) {
	content := "User-agent: AASA-Bot\n" +
		"User-agent: Storebot-Google\n" +
		"User-agent: Stripe\n" +
		"User-agent: Sucuri\n" +
		"User-agent: Yandex\n" +
		"Allow: /_ngjs/resource/*/get/\n" +
		"Allow: /business/create/\n" +
		"Allow: /resource/*/get/\n" +
		"Disallow: /*/*/*/_tools/*\n" +
		"Disallow: /*/*/*/more_ideas/\n"

	got := parseRobotsTxt(content)

	if len(got.UserAgents) != 5 {
		t.Fatalf("parseRobotsTxt() UserAgents count = %d, want 5", len(got.UserAgents))
	}

	for _, ua := range got.UserAgents {
		if len(ua.Directives) != 5 {
			t.Errorf("agent %q Directives count = %d, want 5 (the shared Allow+Disallow block); got %+v", ua.Agent, len(ua.Directives), ua.Directives)
		}
	}
}
