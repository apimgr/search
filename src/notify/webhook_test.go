package notify

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/apimgr/search/src/config"
)

func TestWebhookRequestAdapters(t *testing.T) {
	event := Event{Role: "general", Name: "contact.received", Subject: "Question", Body: "Hello", Severity: "info", AppName: "Search", ProjectName: "search", ProjectVersion: "1.0", AppURL: "https://search.example", Timestamp: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC), SourceIP: "192.0.2.4"}
	tests := []struct {
		name      string
		transport string
		url       string
		check     func(*testing.T, string, []byte, string)
	}{
		{name: "telegram", transport: "telegram", url: "https://api.telegram.org/botTOKEN/sendMessage?chat_id=123", check: func(t *testing.T, target string, body []byte, contentType string) {
			parsed, err := url.Parse(target)
			if err != nil || parsed.Query().Get("text") != "Question\n\nHello" || contentType != "application/x-www-form-urlencoded" || len(body) != 0 {
				t.Fatalf("unexpected Telegram request: %q %q %q %v", target, body, contentType, err)
			}
		}},
		{name: "discord", transport: "discord", url: "https://discord.example/hook", check: func(t *testing.T, _ string, body []byte, contentType string) {
			var payload map[string]string
			if err := json.Unmarshal(body, &payload); err != nil || payload["content"] != "Question\n\nHello" || payload["username"] != "Search" || contentType != "application/json" {
				t.Fatalf("unexpected Discord request: %s %q %v", body, contentType, err)
			}
		}},
		{name: "slack", transport: "slack", url: "https://slack.example/hook", check: func(t *testing.T, _ string, body []byte, _ string) {
			var payload map[string]string
			if err := json.Unmarshal(body, &payload); err != nil || payload["text"] != "Question\n\nHello" {
				t.Fatalf("unexpected Slack request: %s %v", body, err)
			}
		}},
		{name: "mattermost", transport: "mattermost", url: "https://mattermost.example/hook", check: func(t *testing.T, _ string, body []byte, _ string) {
			var payload map[string]string
			if err := json.Unmarshal(body, &payload); err != nil || payload["text"] != "Question\n\nHello" {
				t.Fatalf("unexpected Mattermost request: %s %v", body, err)
			}
		}},
		{name: "pushover", transport: "pushover", url: "https://api.pushover.net/1/messages.json?user=USER&token=TOKEN", check: func(t *testing.T, target string, body []byte, _ string) {
			parsed, _ := url.Parse(target)
			form, _ := url.ParseQuery(string(body))
			if parsed.Query().Has("user") || parsed.Query().Has("token") || form.Get("user") != "USER" || form.Get("token") != "TOKEN" || form.Get("message") != "Question\n\nHello" {
				t.Fatalf("Pushover credentials were not moved to the request body: %q %q", target, body)
			}
		}},
		{name: "gotify", transport: "gotify", url: "https://gotify.example/base?token=TOKEN", check: func(t *testing.T, target string, body []byte, _ string) {
			parsed, _ := url.Parse(target)
			var payload map[string]interface{}
			if err := json.Unmarshal(body, &payload); err != nil || parsed.Path != "/base/message" || parsed.Query().Get("token") != "TOKEN" || payload["title"] != "Question" || payload["message"] != "Question\n\nHello" {
				t.Fatalf("unexpected Gotify request: %q %s %v", target, body, err)
			}
		}},
		{name: "generic", transport: "generic", url: "https://hooks.example/events", check: func(t *testing.T, _ string, body []byte, _ string) {
			var payload map[string]interface{}
			if err := json.Unmarshal(body, &payload); err != nil || payload["event"] != "contact.received" || payload["role"] != "general" || payload["tracking_id"] != nil || payload["source_ip"] != nil {
				t.Fatalf("unexpected generic payload: %s %v", body, err)
			}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			target, body, contentType, err := webhookRequest(delivery{url: test.url, transport: test.transport, event: event})
			if err != nil {
				t.Fatal(err)
			}
			test.check(t, target, body, contentType)
		})
	}
}

func TestWebhookRequestRejectsUnsafeAndUnknownTargets(t *testing.T) {
	for _, test := range []delivery{
		{url: "http://example.com/hook", transport: "generic"},
		{url: "https://user:pass@example.com/hook", transport: "generic"},
		{url: "https://example.com/hook", transport: "other"},
	} {
		if _, _, _, err := webhookRequest(test); err == nil {
			t.Errorf("expected rejection for %q transport %q", test.url, test.transport)
		}
	}
}

func TestWebhookDeliverySignsExactBodyAndSetsHeaders(t *testing.T) {
	const secret = "test-secret"
	var received bool
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received = true
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		mac := hmac.New(sha256.New, []byte(secret))
		_, _ = mac.Write(body)
		if got, want := r.Header.Get("X-Webhook-Signature"), "sha256="+hex.EncodeToString(mac.Sum(nil)); got != want {
			t.Errorf("signature %q does not match expected body HMAC", got)
		}
		if r.Header.Get("X-Webhook-ID") != "event-id" || r.Header.Get("X-Webhook-Event") != "contact.received" || r.Header.Get("X-Webhook-Timestamp") != "1767323045" || !strings.HasPrefix(r.Header.Get("User-Agent"), "search/1.0 (+") {
			t.Errorf("missing webhook metadata headers: %#v", r.Header)
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()
	client := server.Client()
	manager := NewManager(nil, client, nil)
	event := Event{Role: "general", Name: "contact.received", Subject: "Question", Body: "Hello", ProjectName: "search", ProjectVersion: "1.0", AppURL: "https://search.example", Timestamp: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)}
	err := manager.deliver(context.Background(), delivery{url: server.URL, transport: "generic", secret: secret, event: event, id: "event-id"})
	if err != nil {
		t.Fatal(err)
	}
	if !received {
		t.Fatal("webhook endpoint was not called")
	}
}

func TestResolveWebhooksUsesPerTransportRoleFallback(t *testing.T) {
	contact := config.ContactConfig{
		Admin:   config.ContactRoleConfig{Webhooks: config.WebhookNotifyConfig{"generic": "https://admin.example", "generic_secret": "admin-secret", "discord": "https://admin-discord.example", "discord_secret": "discord-secret"}},
		General: config.ContactRoleConfig{Webhooks: config.WebhookNotifyConfig{"generic": "https://general.example", "generic_secret": "general-secret"}},
	}
	got := resolveWebhooks(contact, "general")
	if got["generic"].url != "https://general.example" || got["generic"].secret != "general-secret" || got["discord"].url != "https://admin-discord.example" {
		t.Fatalf("unexpected fallback resolution: %#v", got)
	}
	if !HasConfiguredWebhook(contact, "general") || HasConfiguredWebhook(config.ContactConfig{}, "unknown") {
		t.Fatal("configured webhook detection did not follow role rules")
	}
	if HasExactWebhook(contact, "general") != true || HasExactWebhook(contact, "security") {
		t.Fatal("exact role detection unexpectedly used fallback destinations")
	}
}

func TestGenerateWebhookSecret(t *testing.T) {
	first, err := GenerateWebhookSecret()
	if err != nil {
		t.Fatal(err)
	}
	second, err := GenerateWebhookSecret()
	if err != nil {
		t.Fatal(err)
	}
	if first == second || len(first) < 40 {
		t.Fatal("webhook secret was not sufficiently random")
	}
}
