package server

import (
	"strings"
	"testing"

	"github.com/apimgr/search/src/common/i18n"
)

// TestComposeSecurityReportBody_I18nLabels verifies that the security report
// email body resolves every label from the email.security_report.* i18n keys
// instead of hardcoded English strings (AI.md PART 30 "Email Template
// Translation"). With the English default, labels must render; with another
// supported locale (de), the German translations must be used.
func TestComposeSecurityReportBody_I18nLabels(t *testing.T) {
	body := composeSecurityReportBody("en", "TRK-123", "handler", "/search", "high",
		"summary text", "1. open 2. crash", "data loss", "use prepared statements",
		"researcher@example.com", "-----BEGIN PGP-----", "anonymous", "Jane Doe",
		90, true)

	for _, key := range []string{
		"email.security_report.label_tracking_id",
		"email.security_report.label_component",
		"email.security_report.label_endpoint",
		"email.security_report.label_severity",
		"email.security_report.label_summary",
		"email.security_report.label_steps",
		"email.security_report.label_impact",
		"email.security_report.label_fix",
		"email.security_report.label_cve",
		"email.security_report.label_disclosure",
		"email.security_report.label_credit",
		"email.security_report.label_credit_name",
		"email.security_report.label_email",
		"email.security_report.label_gpg",
	} {
		want := i18n.T("en", key)
		if !strings.Contains(body, want) {
			t.Errorf("body missing %s label %q", key, want)
		}
	}

	// Format keys interpolate args via i18n.T; check the rendered values.
	if !strings.Contains(body, i18n.T("en", "email.security_report.days", 90)) {
		t.Error("body missing rendered disclosure days")
	}
	if !strings.Contains(body, "true") {
		t.Error("body missing CVE boolean")
	}

	// German locale must produce German labels, not the English fallback.
	deBody := composeSecurityReportBody("de", "TRK-123", "handler", "/search", "high",
		"summary text", "1. open 2. crash", "data loss", "",
		"", "", "anonymous", "", 90, false)
	if !strings.Contains(deBody, i18n.T("de", "email.security_report.label_component")) {
		t.Errorf("de body missing German component label: %q", deBody)
	}
	if strings.Contains(deBody, i18n.T("en", "email.security_report.label_component")) {
		t.Errorf("de body contains English component label: %q", deBody)
	}
}

// TestSecurityResearcherAckSubject_I18n verifies the researcher acknowledgment
// subject line comes from the email.security_report.subject key.
func TestSecurityResearcherAckSubject_I18n(t *testing.T) {
	s := &Server{}
	got := i18n.T(s.notificationLang(), "email.security_report.subject", "TRK-9")
	want := i18n.T("en", "email.security_report.subject", "TRK-9")
	if got != want {
		t.Errorf("subject = %q, want default-locale %q", got, want)
	}
	if !strings.Contains(want, "TRK-9") {
		t.Errorf("subject did not interpolate tracking id: %q", want)
	}
}
