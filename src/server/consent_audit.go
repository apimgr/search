package server

import (
	"net/http"
	"net/netip"
	"strings"

	"github.com/apimgr/search/src/logging"
)

func (s *Server) logConsentUpdated(r *http.Request, newValue map[string]bool) {
	if s.logManager == nil || s.config == nil || r == nil {
		return
	}
	audit := s.logManager.Audit()
	if audit == nil {
		return
	}

	actor := logging.AuditActor{Type: "anonymous"}
	if s.ownerManager != nil {
		if cookie, err := r.Cookie(ownerTokenCookie); err == nil {
			if identity, err := s.ownerManager.Resolve(r.Context(), cookie.Value); err == nil {
				actor.Type = "owner"
				actor.ID = identity.OwnerID
			}
		}
	}

	actor.IP = s.consentAuditIP(r)
	policyVersion := strings.TrimSpace(s.config.Server.Privacy.Consent.Policy.Version)
	if policyVersion == "" {
		policyVersion = "1"
	}

	audit.LogEvent(
		logging.AuditActionConsentUpdated,
		logging.AuditCategoryCompliance,
		logging.AuditSeverityInfo,
		actor,
		nil,
		"success",
		map[string]interface{}{
			"consent_type":   "cookie_categories",
			"new_value":      newValue,
			"policy_version": policyVersion,
		},
		"",
	)
}

func (s *Server) consentAuditIP(r *http.Request) string {
	if torIdentity := s.torIdentity(r); torIdentity != "" {
		return torIdentity
	}
	return anonymizeConsentIP(getClientIP(r, s.config.Server.TrustedProxies.Additional))
}

func anonymizeConsentIP(value string) string {
	address, err := netip.ParseAddr(value)
	if err != nil {
		return ""
	}
	address = address.Unmap()
	maskBits := 48
	if address.Is4() {
		maskBits = 24
	}
	prefix, err := address.Prefix(maskBits)
	if err != nil {
		return ""
	}
	return prefix.Masked().Addr().String()
}
