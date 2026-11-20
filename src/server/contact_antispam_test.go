package server

import (
	"bufio"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/apimgr/search/src/api"
	"github.com/apimgr/search/src/config"
	"github.com/apimgr/search/src/email"
)

func TestContactFormTokenRejectsTooFastForgedAndExpiredTokens(t *testing.T) {
	s := newTestServer(t)
	original := s.config.Get()
	defer s.config.Update(func(cfg *config.ServerConfig) {
		cfg.Pages.Contact.Antispam = original.Pages.Contact.Antispam
		cfg.Security.InstallationSecret = original.Security.InstallationSecret
	})
	s.config.Update(func(cfg *config.ServerConfig) {
		cfg.Security.InstallationSecret = "contact-test-installation-secret"
		cfg.Pages.Contact.Antispam.MinFillSeconds = 3
		cfg.Pages.Contact.Antispam.MaxAgeSeconds = 3600
	})
	fresh := s.ContactFormToken()
	if s.ValidateContactFormToken(fresh) {
		t.Fatal("a form token used before min_fill_seconds was accepted")
	}
	issued := strings.SplitN(fresh, ".", 2)[0]
	if s.ValidateContactFormToken(issued + "." + strings.Repeat("0", 64)) {
		t.Fatal("a forged form token was accepted")
	}
	s.config.Update(func(cfg *config.ServerConfig) {
		cfg.Pages.Contact.Antispam.MinFillSeconds = 0
		cfg.Pages.Contact.Antispam.MaxAgeSeconds = 1
	})
	oldIssued := time.Now().Add(-2 * time.Second).Unix()
	mac := hmac.New(sha256.New, []byte("contact-test-installation-secret"))
	_, _ = fmt.Fprintf(mac, "contact-form|%d", oldIssued)
	expired := fmt.Sprintf("%d.%s", oldIssued, hex.EncodeToString(mac.Sum(nil)))
	if s.ValidateContactFormToken(expired) {
		t.Fatal("a correctly signed but expired form token was accepted")
	}
}

func TestContactReviewTokenBindsFieldsAndIsSingleUse(t *testing.T) {
	s := newTestServer(t)
	values := url.Values{
		"type": {"abuse"}, "name": {"Alice"}, "email": {"alice@example.net"},
		"subject": {"Report"}, "message": {"Details"}, "form_token": {"ignored"},
	}
	token := s.ContactReviewToken(values, "")
	changedValues := url.Values{
		"type": {"abuse"}, "name": {"Alice"}, "email": {"alice@example.net"},
		"subject": {"Report"}, "message": {"Details"},
	}
	changedToken := s.ContactReviewToken(changedValues, "")
	time.Sleep(2100 * time.Millisecond)
	if !s.ValidateContactReviewToken(values, token, true) {
		t.Fatal("valid contact review token was rejected")
	}
	if s.ValidateContactReviewToken(values, token, true) {
		t.Fatal("contact review token was accepted more than once")
	}
	values.Set("message", "changed after review")
	if s.ValidateContactReviewToken(values, changedToken, true) {
		t.Fatal("review token was accepted for changed form values")
	}
}

func TestContactShapeRejectsHeaderInjectionAndInvisibleUnicode(t *testing.T) {
	s := newTestServer(t)
	for _, test := range []struct {
		name  string
		field string
		value string
	}{
		{name: "name header injection", field: "name", value: "Alice\r\nBcc: victim@example.com"},
		{name: "email line break", field: "email", value: "alice@example.com\nBcc: victim@example.com"},
		{name: "subject line break", field: "subject", value: "Hello\r\nBcc: victim@example.com"},
		{name: "message bidi override", field: "message", value: "hello\u202eexample"},
	} {
		t.Run(test.name, func(t *testing.T) {
			values := url.Values{
				"name": {"Alice"}, "email": {"alice@example.com"}, "subject": {"Hello"},
				"message": {"A message"}, "form_token": {"token"}, "website": {""},
			}
			values.Set(test.field, test.value)
			req := httptest.NewRequest(http.MethodPost, "/server/contact", strings.NewReader(values.Encode()))
			if reason := s.ValidateContactShape(req, values, false, false); reason != "shape" {
				t.Fatalf("ValidateContactShape() = %q, want shape", reason)
			}
		})
	}
}

func TestContactRateLimitReturnsFourthAttemptLimit(t *testing.T) {
	s := newTestServer(t)
	original := s.config.Get()
	defer s.config.Update(func(cfg *config.ServerConfig) { cfg.Pages.Contact.Antispam = original.Pages.Contact.Antispam })
	s.config.Update(func(cfg *config.ServerConfig) {
		cfg.Pages.Contact.Antispam.PerIP = 3
		cfg.Pages.Contact.Antispam.PerEmail = 0
		cfg.Pages.Contact.Antispam.Global = 0
		cfg.Pages.Contact.Antispam.Window = "1h"
	})
	req := httptest.NewRequest(http.MethodPost, "/server/contact", nil)
	req.RemoteAddr = "203.0.113.45:1234"
	for attempt := 1; attempt <= 3; attempt++ {
		if reason, _ := s.AcceptContactSubmission(req, "203.0.113.45", "person@example.com", "Person", "Topic", fmt.Sprintf("Message %d", attempt), false); reason != "" {
			t.Fatalf("attempt %d was rejected: %s", attempt, reason)
		}
	}
	if reason, retryAfter := s.AcceptContactSubmission(req, "203.0.113.45", "person@example.com", "Person", "Topic", "Message 4", false); reason != "rate_ip" || retryAfter <= 0 {
		t.Fatalf("fourth attempt = (%q, %s), want rate_ip with retry delay", reason, retryAfter)
	}
}

func TestContactDuplicateIsSilentAndKeepsOneLRUEntry(t *testing.T) {
	s := newTestServer(t)
	original := s.config.Get()
	defer s.config.Update(func(cfg *config.ServerConfig) { cfg.Pages.Contact.Antispam = original.Pages.Contact.Antispam })
	s.config.Update(func(cfg *config.ServerConfig) {
		cfg.Pages.Contact.Antispam.PerIP = 0
		cfg.Pages.Contact.Antispam.PerEmail = 0
		cfg.Pages.Contact.Antispam.Global = 0
		cfg.Pages.Contact.Antispam.DuplicateWindowHours = 24
	})
	req := httptest.NewRequest(http.MethodPost, "/server/contact", nil)
	before := s.contactSpam().order.Len()
	if reason, _ := s.AcceptContactSubmission(req, "203.0.113.45", "person@example.com", "Person", "Topic", "Same message", false); reason != "" {
		t.Fatalf("first submission returned %q", reason)
	}
	if got := s.contactSpam().order.Len(); got != before+1 {
		t.Fatalf("first LRU entry count = %d, want %d", got, before+1)
	}
	if reason, _ := s.AcceptContactSubmission(req, "203.0.113.45", "person@example.com", " Person ", "Topic", "Same   message", false); reason != "duplicate" {
		t.Fatalf("repeat submission returned %q, want duplicate", reason)
	}
	if got := s.contactSpam().order.Len(); got != before+1 {
		t.Fatalf("duplicate LRU entries = %d, want unchanged count %d", got, before+1)
	}
	req.Header.Set("Accept", "application/json")
	recorder := httptest.NewRecorder()
	s.respondContactSpamSuccess(recorder, req)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"status":"received"`) {
		t.Fatalf("silent duplicate response = %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestFailedContactDeliveryReleasesRateAndDuplicateReservation(t *testing.T) {
	s := newTestServer(t)
	original := s.config.Get()
	defer s.config.Update(func(cfg *config.ServerConfig) { cfg.Pages.Contact.Antispam = original.Pages.Contact.Antispam })
	s.config.Update(func(cfg *config.ServerConfig) {
		cfg.Pages.Contact.Antispam.PerIP = 5
		cfg.Pages.Contact.Antispam.PerEmail = 5
		cfg.Pages.Contact.Antispam.Global = 10
		cfg.Pages.Contact.Antispam.DuplicateWindowHours = 24
	})
	req := httptest.NewRequest(http.MethodPost, "/server/contact", nil)
	if reason, _ := s.AcceptContactSubmission(req, "203.0.113.45", "person@example.com", "Person", "Topic", "Retry me", false); reason != "" {
		t.Fatalf("reservation returned %q", reason)
	}
	s.ForgetContactSubmission(req, "203.0.113.45", "person@example.com", "Person", "Topic", "Retry me", false)
	if reason, _ := s.AcceptContactSubmission(req, "203.0.113.45", "person@example.com", "Person", "Topic", "Retry me", false); reason != "" {
		t.Fatalf("retry after failed delivery returned %q", reason)
	}
}

func TestContactDeliveryUsesConfiguredFromAndOneEmail(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	portText := strings.Split(listener.Addr().String(), ":")
	port, err := strconv.Atoi(portText[len(portText)-1])
	if err != nil {
		t.Fatal(err)
	}
	type smtpResult struct {
		from, data string
		recipients int
		messages   int
	}
	result := make(chan smtpResult, 1)
	go func() {
		var captured smtpResult
		conn, acceptErr := listener.Accept()
		if acceptErr != nil {
			result <- captured
			return
		}
		defer conn.Close()
		reader := bufio.NewReader(conn)
		_, _ = fmt.Fprint(conn, "220 test SMTP\r\n")
		inData := false
		for {
			line, readErr := reader.ReadString('\n')
			if readErr != nil {
				break
			}
			line = strings.TrimRight(line, "\r\n")
			if inData {
				if line == "." {
					inData = false
					captured.messages++
					_, _ = fmt.Fprint(conn, "250 queued\r\n")
				} else {
					captured.data += line + "\n"
				}
				continue
			}
			switch {
			case strings.HasPrefix(strings.ToUpper(line), "EHLO"), strings.HasPrefix(strings.ToUpper(line), "HELO"):
				_, _ = fmt.Fprint(conn, "250-test\r\n250 OK\r\n")
			case strings.HasPrefix(strings.ToUpper(line), "MAIL FROM:"):
				captured.from = line
				_, _ = fmt.Fprint(conn, "250 sender ok\r\n")
			case strings.HasPrefix(strings.ToUpper(line), "RCPT TO:"):
				captured.recipients++
				_, _ = fmt.Fprint(conn, "250 recipient ok\r\n")
			case strings.EqualFold(line, "DATA"):
				inData = true
				_, _ = fmt.Fprint(conn, "354 send data\r\n")
			case strings.EqualFold(line, "QUIT"):
				_, _ = fmt.Fprint(conn, "221 bye\r\n")
				result <- captured
				return
			default:
				_, _ = fmt.Fprint(conn, "250 OK\r\n")
			}
		}
		result <- captured
	}()

	s := newTestServer(t)
	s.notifyManager = nil
	s.config.Server.Contact.General.Email = "contact@example.test"
	s.config.Server.Branding.Title = "Search"
	s.mailer = email.NewMailer(&email.Config{
		Enabled: true,
		SMTP:    email.SMTPConfig{Host: "127.0.0.1", Port: port, TLS: "none"},
		From:    email.FromConfig{Name: "Search Sender", Email: "sender@example.test"},
	})
	if err := s.SubmitContact(context.Background(), api.ContactMessage{
		Name: "Alice", Email: "alice@example.net", Subject: "Question", Message: "Plain text body",
	}); err != nil {
		t.Fatalf("SubmitContact() error = %v", err)
	}
	captured := <-result
	if captured.messages != 1 || captured.recipients != 1 {
		t.Fatalf("delivered messages=%d recipients=%d, want one each", captured.messages, captured.recipients)
	}
	if !strings.Contains(captured.from, "sender@example.test") || strings.Contains(captured.from, "alice@example.net") {
		t.Fatalf("SMTP envelope From = %q, want configured sender only", captured.from)
	}
	if !strings.Contains(captured.data, "From: Search Sender <sender@example.test>") ||
		!strings.Contains(captured.data, "Reply-To: alice@example.net") ||
		!strings.Contains(captured.data, "Subject: [Search contact] Question") ||
		!strings.Contains(captured.data, "Content-Type: text/plain") {
		t.Fatalf("contact email headers violate delivery policy: %s", captured.data)
	}
}
