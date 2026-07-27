package server

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

// maxReportBodyBytes caps the size of a browser-emitted report body. Reports are
// public and unauthenticated, so the body is bounded to prevent memory abuse per
// AI.md PART 11 "Reporting API → same rate limits, same Output Sanitization Pipeline".
const maxReportBodyBytes = 64 * 1024

// maxReportDirectiveLen bounds the sanitized directive string that gets written to
// security.log, per the Output Sanitization Pipeline truncation rule (PART 11).
const maxReportDirectiveLen = 200

// registerReportRoutes registers the public browser-report endpoints advertised by
// the CSP report-uri/report-to, Reporting-Endpoints, Report-To, and NEL headers
// (see middleware.go SecurityHeaders and config.go default CSP). Per AI.md PART 11
// "Reporting API (Modern + Legacy)" and "Content Security Policy → Reports Endpoint"
// every browser-emitted report — CSP, NEL, deprecation, intervention, crash, and the
// generic default group — shares the same public, per-IP rate-limited endpoint shape:
// /api/{api_version}/server/reports/{name}. Without these handlers the browser's
// violation POSTs return 404 while the headers still advertise the endpoints.
func (s *Server) registerReportRoutes(prefix string) {
	// Named groups advertised by the headers. The catch-all covers any other
	// report group a browser may target (deprecation, intervention, crash).
	s.router.Post(prefix+"/server/reports/csp", s.handleBrowserReport)
	s.router.Post(prefix+"/server/reports/nel", s.handleBrowserReport)
	s.router.Post(prefix+"/server/reports/default", s.handleBrowserReport)
	s.router.Post(prefix+"/server/reports/{name}", s.handleBrowserReport)
}

// handleBrowserReport ingests a CSP violation / NEL / Reporting-API report.
// Per AI.md PART 11 "Reports Endpoint":
//   - accepts application/csp-report (legacy) and application/reports+json (Reporting API)
//   - logs to security.log as security.csp_violation
//   - responds 204 No Content to keep the browser happy
//   - NEVER returns user-controlled fields back in the response body
//
// Privacy is the product (PART 11): the client IP is never persisted — "-" is logged.
func (s *Server) handleBrowserReport(w http.ResponseWriter, r *http.Request) {
	reportType := reportGroupFromPath(r.URL.Path)

	// Bound the body: public, unauthenticated endpoint.
	body, _ := io.ReadAll(io.LimitReader(r.Body, maxReportBodyBytes))

	directive := extractReportDirective(r.Header.Get("Content-Type"), body)

	if s.logManager != nil {
		// IP is deliberately "-" — never log client addresses (PART 11 privacy rule).
		s.logManager.Security().LogCSPViolation("-", reportType, directive)
	}

	// 204 with no body — user-controlled report fields are never echoed back.
	w.WriteHeader(http.StatusNoContent)
}

// reportGroupFromPath returns the trailing report-group name from a
// /api/{api_version}/server/reports/{name} path, defaulting to "default".
func reportGroupFromPath(path string) string {
	idx := strings.LastIndex(path, "/reports/")
	if idx == -1 {
		return "default"
	}
	name := path[idx+len("/reports/"):]
	name = strings.Trim(name, "/")
	if name == "" {
		return "default"
	}
	return name
}

// extractReportDirective pulls a single sanitized directive/disposition string out
// of a report body for logging only. It never returns raw user-controlled content
// verbatim beyond the truncated directive field, per the Output Sanitization
// Pipeline (PART 11). Returns "-" when nothing useful can be parsed.
func extractReportDirective(contentType string, body []byte) string {
	if len(body) == 0 {
		return "-"
	}

	// Legacy CSP report: {"csp-report": {"violated-directive": "...", ...}}
	if strings.Contains(contentType, "application/csp-report") {
		var legacy struct {
			CSPReport struct {
				ViolatedDirective  string `json:"violated-directive"`
				EffectiveDirective string `json:"effective-directive"`
			} `json:"csp-report"`
		}
		if err := json.Unmarshal(body, &legacy); err == nil {
			if d := firstNonEmpty(legacy.CSPReport.EffectiveDirective, legacy.CSPReport.ViolatedDirective); d != "" {
				return truncateDirective(d)
			}
		}
		return "-"
	}

	// Reporting API (application/reports+json): array of {"type","body":{...}}.
	// NEL, deprecation, intervention, crash, and csp-violation all share this shape.
	var reports []struct {
		Type string `json:"type"`
		Body struct {
			EffectiveDirective string `json:"effectiveDirective"`
			ViolatedDirective  string `json:"violatedDirective"`
			Disposition        string `json:"disposition"`
		} `json:"body"`
	}
	if err := json.Unmarshal(body, &reports); err == nil && len(reports) > 0 {
		first := reports[0]
		d := firstNonEmpty(first.Body.EffectiveDirective, first.Body.ViolatedDirective, first.Type)
		if d != "" {
			return truncateDirective(d)
		}
	}
	return "-"
}

// firstNonEmpty returns the first non-empty string in the arguments.
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// truncateDirective bounds a directive string to maxReportDirectiveLen and strips
// characters that would break the single-line security.log format.
func truncateDirective(d string) string {
	d = strings.Map(func(rn rune) rune {
		if rn == '\n' || rn == '\r' {
			return ' '
		}
		return rn
	}, d)
	if len(d) > maxReportDirectiveLen {
		return d[:maxReportDirectiveLen]
	}
	return d
}
