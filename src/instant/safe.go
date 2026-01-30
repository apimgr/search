package instant

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/apimgr/search/src/version"
)

// SafeHandler performs basic URL safety checks
type SafeHandler struct {
	client   *http.Client
	patterns []*regexp.Regexp
}

// SafetyCheck represents a single safety check result
type SafetyCheck struct {
	Name    string `json:"name"`
	Status  string `json:"status"` // "pass", "warn", "fail", "info"
	Message string `json:"message"`
}

// SafetyResult represents the overall safety assessment
type SafetyResult struct {
	URL         string        `json:"url"`
	Safe        bool          `json:"safe"`
	Score       int           `json:"score"` // 0-100
	Checks      []SafetyCheck `json:"checks"`
	Certificate *CertInfo     `json:"certificate,omitempty"`
}

// CertInfo represents SSL certificate information
type CertInfo struct {
	Subject    string    `json:"subject"`
	Issuer     string    `json:"issuer"`
	ValidFrom  time.Time `json:"valid_from"`
	ValidUntil time.Time `json:"valid_until"`
	DaysLeft   int       `json:"days_left"`
	IsValid    bool      `json:"is_valid"`
}

// NewSafeHandler creates a new safety check handler
func NewSafeHandler() *SafeHandler {
	return &SafeHandler{
		client: &http.Client{
			Timeout: 15 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				// Allow up to 10 redirects
				if len(via) >= 10 {
					return http.ErrUseLastResponse
				}
				return nil
			},
		},
		patterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)^safe[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^safety[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^check[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^is\s+(.+)\s+safe\??$`),
			regexp.MustCompile(`(?i)^scan[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^verify[:\s]+(.+)$`),
		},
	}
}

func (h *SafeHandler) Name() string {
	return "safe"
}

func (h *SafeHandler) Patterns() []*regexp.Regexp {
	return h.patterns
}

func (h *SafeHandler) CanHandle(query string) bool {
	for _, p := range h.patterns {
		if p.MatchString(query) {
			return true
		}
	}
	return false
}

func (h *SafeHandler) Handle(ctx context.Context, query string) (*Answer, error) {
	// Extract URL from query
	urlStr := ""
	for _, p := range h.patterns {
		if matches := p.FindStringSubmatch(query); len(matches) > 1 {
			urlStr = strings.TrimSpace(matches[1])
			break
		}
	}

	if urlStr == "" {
		return nil, nil
	}

	// Normalize URL
	urlStr = normalizeURLForSafety(urlStr)

	// Perform safety checks
	result := h.checkSafety(ctx, urlStr)

	// Build content
	var content strings.Builder

	// Overall status
	statusIcon := "Warning"
	statusClass := "warn"
	if result.Score >= 80 {
		statusIcon = "Safe"
		statusClass = "safe"
	} else if result.Score < 50 {
		statusIcon = "Potentially Unsafe"
		statusClass = "unsafe"
	}

	content.WriteString(fmt.Sprintf("<div class=\"safety-result %s\">", statusClass))
	content.WriteString(fmt.Sprintf("<strong>%s</strong> - Safety Score: %d/100<br><br>", statusIcon, result.Score))
	content.WriteString(fmt.Sprintf("<strong>URL:</strong> %s<br><br>", result.URL))

	// Individual checks
	content.WriteString("<strong>Security Checks:</strong><br>")
	for _, check := range result.Checks {
		icon := "[ ]"
		switch check.Status {
		case "pass":
			icon = "[+]"
		case "warn":
			icon = "[!]"
		case "fail":
			icon = "[-]"
		case "info":
			icon = "[i]"
		}
		content.WriteString(fmt.Sprintf("%s %s: %s<br>", icon, check.Name, check.Message))
	}

	// Certificate info
	if result.Certificate != nil {
		content.WriteString("<br><strong>SSL Certificate:</strong><br>")
		content.WriteString(fmt.Sprintf("&nbsp;&nbsp;Subject: %s<br>", result.Certificate.Subject))
		content.WriteString(fmt.Sprintf("&nbsp;&nbsp;Issuer: %s<br>", result.Certificate.Issuer))
		content.WriteString(fmt.Sprintf("&nbsp;&nbsp;Valid Until: %s (%d days left)<br>",
			result.Certificate.ValidUntil.Format("2006-01-02"),
			result.Certificate.DaysLeft))
	}

	content.WriteString("</div>")

	content.WriteString("<br><em>Note: This is a basic check. For comprehensive security analysis, use dedicated security tools.</em>")

	return &Answer{
		Type:    AnswerTypeSafe,
		Query:   query,
		Title:   fmt.Sprintf("Safety Check: %s", truncateSafeURL(urlStr, 50)),
		Content: content.String(),
		Data: map[string]interface{}{
			"url":         result.URL,
			"safe":        result.Safe,
			"score":       result.Score,
			"checks":      result.Checks,
			"certificate": result.Certificate,
		},
	}, nil
}

// checkSafety performs various safety checks on a URL
func (h *SafeHandler) checkSafety(ctx context.Context, urlStr string) *SafetyResult {
	result := &SafetyResult{
		URL:    urlStr,
		Safe:   true,
		Score:  100,
		Checks: make([]SafetyCheck, 0),
	}

	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		result.Checks = append(result.Checks, SafetyCheck{
			Name:    "URL Parsing",
			Status:  "fail",
			Message: "Invalid URL format",
		})
		result.Score -= 50
		result.Safe = false
		return result
	}

	// Check 1: HTTPS
	if parsedURL.Scheme == "https" {
		result.Checks = append(result.Checks, SafetyCheck{
			Name:    "HTTPS",
			Status:  "pass",
			Message: "Uses secure HTTPS connection",
		})
	} else {
		result.Checks = append(result.Checks, SafetyCheck{
			Name:    "HTTPS",
			Status:  "warn",
			Message: "Not using HTTPS - data may not be encrypted",
		})
		result.Score -= 20
	}

	// Check 2: Suspicious patterns in URL
	suspiciousPatterns := []struct {
		pattern string
		desc    string
	}{
		{`@`, "Contains @ symbol - possible phishing"},
		{`\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}`, "Uses IP address instead of domain"},
		{`-{3,}`, "Contains many hyphens - suspicious"},
		{`login.*\.(tk|ml|ga|cf|gq)$`, "Login page on free TLD"},
		{`paypal|amazon|google|facebook|bank`, "Contains brand name - verify legitimacy"},
		{`%[0-9a-fA-F]{2}.*%[0-9a-fA-F]{2}`, "Heavy URL encoding - possibly hiding content"},
	}

	suspiciousFound := false
	for _, sp := range suspiciousPatterns {
		if matched, _ := regexp.MatchString(sp.pattern, urlStr); matched {
			result.Checks = append(result.Checks, SafetyCheck{
				Name:    "URL Pattern",
				Status:  "warn",
				Message: sp.desc,
			})
			result.Score -= 10
			suspiciousFound = true
		}
	}

	if !suspiciousFound {
		result.Checks = append(result.Checks, SafetyCheck{
			Name:    "URL Pattern",
			Status:  "pass",
			Message: "No suspicious patterns detected",
		})
	}

	// Check 3: Domain TLD
	risky_tlds := map[string]bool{
		"tk": true, "ml": true, "ga": true, "cf": true, "gq": true,
		"xyz": true, "top": true, "work": true, "click": true, "link": true,
	}

	parts := strings.Split(parsedURL.Hostname(), ".")
	if len(parts) > 0 {
		tld := parts[len(parts)-1]
		if risky_tlds[tld] {
			result.Checks = append(result.Checks, SafetyCheck{
				Name:    "Domain TLD",
				Status:  "warn",
				Message: fmt.Sprintf(".%s TLD is commonly associated with spam/phishing", tld),
			})
			result.Score -= 15
		} else {
			result.Checks = append(result.Checks, SafetyCheck{
				Name:    "Domain TLD",
				Status:  "pass",
				Message: fmt.Sprintf(".%s is a standard TLD", tld),
			})
		}
	}

	// Check 4: DNS Resolution
	host := parsedURL.Hostname()
	ips, err := net.LookupHost(host)
	if err != nil {
		result.Checks = append(result.Checks, SafetyCheck{
			Name:    "DNS Resolution",
			Status:  "fail",
			Message: "Domain does not resolve - may not exist",
		})
		result.Score -= 30
		result.Safe = false
	} else {
		result.Checks = append(result.Checks, SafetyCheck{
			Name:    "DNS Resolution",
			Status:  "pass",
			Message: fmt.Sprintf("Domain resolves to %d IP(s)", len(ips)),
		})
	}

	// Check 5: SSL Certificate (for HTTPS)
	if parsedURL.Scheme == "https" {
		certInfo, err := h.checkSSLCertificate(ctx, parsedURL.Host)
		if err != nil {
			result.Checks = append(result.Checks, SafetyCheck{
				Name:    "SSL Certificate",
				Status:  "fail",
				Message: fmt.Sprintf("SSL error: %v", err),
			})
			result.Score -= 25
			result.Safe = false
		} else {
			result.Certificate = certInfo
			if certInfo.IsValid {
				if certInfo.DaysLeft < 7 {
					result.Checks = append(result.Checks, SafetyCheck{
						Name:    "SSL Certificate",
						Status:  "warn",
						Message: fmt.Sprintf("Certificate expiring soon (%d days)", certInfo.DaysLeft),
					})
					result.Score -= 5
				} else {
					result.Checks = append(result.Checks, SafetyCheck{
						Name:    "SSL Certificate",
						Status:  "pass",
						Message: fmt.Sprintf("Valid certificate (%d days remaining)", certInfo.DaysLeft),
					})
				}
			} else {
				result.Checks = append(result.Checks, SafetyCheck{
					Name:    "SSL Certificate",
					Status:  "fail",
					Message: "Certificate is expired or invalid",
				})
				result.Score -= 25
				result.Safe = false
			}
		}
	}

	// Check 6: HTTP Response
	req, err := http.NewRequestWithContext(ctx, "HEAD", urlStr, nil)
	if err == nil {
		req.Header.Set("User-Agent", version.BrowserUserAgent)
		resp, err := h.client.Do(req)
		if err != nil {
			result.Checks = append(result.Checks, SafetyCheck{
				Name:    "HTTP Response",
				Status:  "warn",
				Message: fmt.Sprintf("Connection error: %v", err),
			})
			result.Score -= 10
		} else {
			defer resp.Body.Close()

			if resp.StatusCode >= 200 && resp.StatusCode < 400 {
				result.Checks = append(result.Checks, SafetyCheck{
					Name:    "HTTP Response",
					Status:  "pass",
					Message: fmt.Sprintf("Server responds with %d", resp.StatusCode),
				})

				// Check security headers
				h.checkSecurityHeaders(resp.Header, result)
			} else if resp.StatusCode >= 400 {
				result.Checks = append(result.Checks, SafetyCheck{
					Name:    "HTTP Response",
					Status:  "warn",
					Message: fmt.Sprintf("Server returns error %d", resp.StatusCode),
				})
				result.Score -= 10
			}
		}
	}

	// Determine overall safety
	if result.Score < 50 {
		result.Safe = false
	}

	return result
}

// checkSSLCertificate checks the SSL certificate of a host
func (h *SafeHandler) checkSSLCertificate(ctx context.Context, host string) (*CertInfo, error) {
	// Add port if not present
	if !strings.Contains(host, ":") {
		host = host + ":443"
	}

	conn, err := tls.Dial("tcp", host, &tls.Config{
		InsecureSkipVerify: false,
	})
	if err != nil {
		// Try with InsecureSkipVerify to still get cert info
		conn, err = tls.Dial("tcp", host, &tls.Config{
			InsecureSkipVerify: true,
		})
		if err != nil {
			return nil, err
		}
		defer conn.Close()

		certs := conn.ConnectionState().PeerCertificates
		if len(certs) == 0 {
			return nil, fmt.Errorf("no certificate found")
		}

		cert := certs[0]
		now := time.Now()
		daysLeft := int(cert.NotAfter.Sub(now).Hours() / 24)

		return &CertInfo{
			Subject:    cert.Subject.CommonName,
			Issuer:     cert.Issuer.CommonName,
			ValidFrom:  cert.NotBefore,
			ValidUntil: cert.NotAfter,
			DaysLeft:   daysLeft,
			IsValid:    false, // We used InsecureSkipVerify
		}, nil
	}
	defer conn.Close()

	certs := conn.ConnectionState().PeerCertificates
	if len(certs) == 0 {
		return nil, fmt.Errorf("no certificate found")
	}

	cert := certs[0]
	now := time.Now()
	daysLeft := int(cert.NotAfter.Sub(now).Hours() / 24)

	return &CertInfo{
		Subject:    cert.Subject.CommonName,
		Issuer:     cert.Issuer.CommonName,
		ValidFrom:  cert.NotBefore,
		ValidUntil: cert.NotAfter,
		DaysLeft:   daysLeft,
		IsValid:    now.After(cert.NotBefore) && now.Before(cert.NotAfter),
	}, nil
}

// checkSecurityHeaders checks for important security headers
func (h *SafeHandler) checkSecurityHeaders(headers http.Header, result *SafetyResult) {
	securityHeaders := []struct {
		header string
		desc   string
	}{
		{"Strict-Transport-Security", "HSTS enabled"},
		{"Content-Security-Policy", "CSP enabled"},
		{"X-Content-Type-Options", "X-Content-Type-Options set"},
		{"X-Frame-Options", "X-Frame-Options set"},
	}

	hasSecurityHeaders := false
	for _, sh := range securityHeaders {
		if headers.Get(sh.header) != "" {
			hasSecurityHeaders = true
			break
		}
	}

	if hasSecurityHeaders {
		result.Checks = append(result.Checks, SafetyCheck{
			Name:    "Security Headers",
			Status:  "pass",
			Message: "Site uses security headers",
		})
	} else {
		result.Checks = append(result.Checks, SafetyCheck{
			Name:    "Security Headers",
			Status:  "info",
			Message: "No common security headers detected",
		})
	}
}

// normalizeURLForSafety normalizes a URL for safety checking
func normalizeURLForSafety(urlStr string) string {
	urlStr = strings.TrimSpace(urlStr)

	// Add scheme if missing
	if !strings.HasPrefix(urlStr, "http://") && !strings.HasPrefix(urlStr, "https://") {
		urlStr = "https://" + urlStr
	}

	return urlStr
}

// truncateSafeURL truncates a URL for display
func truncateSafeURL(urlStr string, maxLen int) string {
	if len(urlStr) <= maxLen {
		return urlStr
	}
	return urlStr[:maxLen-3] + "..."
}
