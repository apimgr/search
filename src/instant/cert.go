package instant

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"regexp"
	"strings"
	"time"
)

// AnswerTypeCert is the answer type for certificate information
const AnswerTypeCert AnswerType = "cert"

// CertHandler handles SSL certificate information queries
type CertHandler struct {
	patterns []*regexp.Regexp
}

// NewCertHandler creates a new certificate handler
func NewCertHandler() *CertHandler {
	return &CertHandler{
		patterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)^cert[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^certificate[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^ssl[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^tls[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^ssl\s+cert(?:ificate)?[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^check\s+cert(?:ificate)?[:\s]+(.+)$`),
		},
	}
}

func (h *CertHandler) Name() string {
	return "cert"
}

func (h *CertHandler) Patterns() []*regexp.Regexp {
	return h.patterns
}

func (h *CertHandler) CanHandle(query string) bool {
	for _, p := range h.patterns {
		if p.MatchString(query) {
			return true
		}
	}
	return false
}

func (h *CertHandler) Handle(ctx context.Context, query string) (*Answer, error) {
	// Extract domain from query
	domain := ""
	for _, p := range h.patterns {
		if matches := p.FindStringSubmatch(query); len(matches) > 1 {
			domain = strings.TrimSpace(matches[1])
			break
		}
	}

	if domain == "" {
		return nil, nil
	}

	// Clean up domain - remove protocol if present
	domain = strings.TrimPrefix(domain, "https://")
	domain = strings.TrimPrefix(domain, "http://")
	domain = strings.TrimSuffix(domain, "/")

	// Extract host if path included
	if idx := strings.Index(domain, "/"); idx != -1 {
		domain = domain[:idx]
	}

	// Default port is 443
	host := domain
	if !strings.Contains(host, ":") {
		host = host + ":443"
	}

	// Create a context with timeout
	dialCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Dial with TLS
	dialer := &net.Dialer{}
	conn, err := tls.DialWithDialer(dialer, "tcp", host, &tls.Config{
		InsecureSkipVerify: true, // We want to see the cert even if invalid
	})
	if err != nil {
		return &Answer{
			Type:    AnswerTypeCert,
			Query:   query,
			Title:   fmt.Sprintf("SSL Certificate: %s", domain),
			Content: fmt.Sprintf("<strong>Error:</strong> Could not connect to %s<br><br>%s", domain, escapeHTML(err.Error())),
			Data: map[string]interface{}{
				"domain": domain,
				"error":  err.Error(),
			},
		}, nil
	}
	defer conn.Close()

	// Check context cancellation
	select {
	case <-dialCtx.Done():
		return &Answer{
			Type:    AnswerTypeCert,
			Query:   query,
			Title:   fmt.Sprintf("SSL Certificate: %s", domain),
			Content: fmt.Sprintf("<strong>Error:</strong> Connection timeout for %s", domain),
		}, nil
	default:
	}

	// Get certificate chain
	certs := conn.ConnectionState().PeerCertificates
	if len(certs) == 0 {
		return &Answer{
			Type:    AnswerTypeCert,
			Query:   query,
			Title:   fmt.Sprintf("SSL Certificate: %s", domain),
			Content: fmt.Sprintf("<strong>Error:</strong> No certificates found for %s", domain),
		}, nil
	}

	// Get the leaf certificate (first in chain)
	cert := certs[0]

	// Check validity
	now := time.Now()
	isValid := now.After(cert.NotBefore) && now.Before(cert.NotAfter)
	daysRemaining := int(cert.NotAfter.Sub(now).Hours() / 24)

	// Build content
	var content strings.Builder
	content.WriteString(fmt.Sprintf("<div class=\"cert-result\">"))

	// Status indicator
	if isValid {
		if daysRemaining < 30 {
			content.WriteString(fmt.Sprintf("<strong>Status:</strong> <span style=\"color: orange;\">Valid (expires in %d days)</span><br><br>", daysRemaining))
		} else {
			content.WriteString(fmt.Sprintf("<strong>Status:</strong> <span style=\"color: green;\">Valid (%d days remaining)</span><br><br>", daysRemaining))
		}
	} else if now.Before(cert.NotBefore) {
		content.WriteString("<strong>Status:</strong> <span style=\"color: red;\">Not yet valid</span><br><br>")
	} else {
		content.WriteString("<strong>Status:</strong> <span style=\"color: red;\">Expired</span><br><br>")
	}

	// Subject information
	content.WriteString("<strong>Subject:</strong><br>")
	if cert.Subject.CommonName != "" {
		content.WriteString(fmt.Sprintf("&nbsp;&nbsp;Common Name: %s<br>", escapeHTML(cert.Subject.CommonName)))
	}
	if len(cert.Subject.Organization) > 0 {
		content.WriteString(fmt.Sprintf("&nbsp;&nbsp;Organization: %s<br>", escapeHTML(strings.Join(cert.Subject.Organization, ", "))))
	}
	if len(cert.Subject.Country) > 0 {
		content.WriteString(fmt.Sprintf("&nbsp;&nbsp;Country: %s<br>", escapeHTML(strings.Join(cert.Subject.Country, ", "))))
	}
	content.WriteString("<br>")

	// Issuer information
	content.WriteString("<strong>Issuer:</strong><br>")
	if cert.Issuer.CommonName != "" {
		content.WriteString(fmt.Sprintf("&nbsp;&nbsp;Common Name: %s<br>", escapeHTML(cert.Issuer.CommonName)))
	}
	if len(cert.Issuer.Organization) > 0 {
		content.WriteString(fmt.Sprintf("&nbsp;&nbsp;Organization: %s<br>", escapeHTML(strings.Join(cert.Issuer.Organization, ", "))))
	}
	content.WriteString("<br>")

	// Validity period
	content.WriteString("<strong>Validity:</strong><br>")
	content.WriteString(fmt.Sprintf("&nbsp;&nbsp;Not Before: %s<br>", cert.NotBefore.Format("2006-01-02 15:04:05 MST")))
	content.WriteString(fmt.Sprintf("&nbsp;&nbsp;Not After: %s<br><br>", cert.NotAfter.Format("2006-01-02 15:04:05 MST")))

	// SANs (Subject Alternative Names)
	if len(cert.DNSNames) > 0 {
		content.WriteString("<strong>Subject Alternative Names:</strong><br>")
		for _, name := range cert.DNSNames {
			content.WriteString(fmt.Sprintf("&nbsp;&nbsp;%s<br>", escapeHTML(name)))
		}
		content.WriteString("<br>")
	}

	// Signature algorithm
	content.WriteString(fmt.Sprintf("<strong>Signature Algorithm:</strong> %s<br>", cert.SignatureAlgorithm.String()))

	// Public key info
	content.WriteString(fmt.Sprintf("<strong>Public Key Algorithm:</strong> %s<br>", cert.PublicKeyAlgorithm.String()))

	// Serial number
	content.WriteString(fmt.Sprintf("<strong>Serial Number:</strong> %s<br>", formatSerialNumber(cert.SerialNumber.Bytes())))

	// Certificate chain length
	content.WriteString(fmt.Sprintf("<strong>Certificate Chain:</strong> %d certificate(s)<br>", len(certs)))

	content.WriteString("</div>")

	// Build data map
	data := map[string]interface{}{
		"domain":             domain,
		"subject_cn":         cert.Subject.CommonName,
		"subject_org":        cert.Subject.Organization,
		"issuer_cn":          cert.Issuer.CommonName,
		"issuer_org":         cert.Issuer.Organization,
		"not_before":         cert.NotBefore.Format(time.RFC3339),
		"not_after":          cert.NotAfter.Format(time.RFC3339),
		"days_remaining":     daysRemaining,
		"is_valid":           isValid,
		"dns_names":          cert.DNSNames,
		"signature_algo":     cert.SignatureAlgorithm.String(),
		"public_key_algo":    cert.PublicKeyAlgorithm.String(),
		"chain_length":       len(certs),
		"key_usage":          formatKeyUsage(cert.KeyUsage),
		"ext_key_usage":      formatExtKeyUsage(cert.ExtKeyUsage),
	}

	return &Answer{
		Type:    AnswerTypeCert,
		Query:   query,
		Title:   fmt.Sprintf("SSL Certificate: %s", domain),
		Content: content.String(),
		Data:    data,
	}, nil
}

// escapeHTML escapes HTML special characters
func escapeHTML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	s = strings.ReplaceAll(s, "'", "&#39;")
	return s
}

// formatSerialNumber formats a certificate serial number
func formatSerialNumber(bytes []byte) string {
	var parts []string
	for _, b := range bytes {
		parts = append(parts, fmt.Sprintf("%02X", b))
	}
	return strings.Join(parts, ":")
}

// formatKeyUsage formats key usage flags
func formatKeyUsage(usage x509.KeyUsage) []string {
	var usages []string
	if usage&x509.KeyUsageDigitalSignature != 0 {
		usages = append(usages, "Digital Signature")
	}
	if usage&x509.KeyUsageContentCommitment != 0 {
		usages = append(usages, "Content Commitment")
	}
	if usage&x509.KeyUsageKeyEncipherment != 0 {
		usages = append(usages, "Key Encipherment")
	}
	if usage&x509.KeyUsageDataEncipherment != 0 {
		usages = append(usages, "Data Encipherment")
	}
	if usage&x509.KeyUsageKeyAgreement != 0 {
		usages = append(usages, "Key Agreement")
	}
	if usage&x509.KeyUsageCertSign != 0 {
		usages = append(usages, "Certificate Sign")
	}
	if usage&x509.KeyUsageCRLSign != 0 {
		usages = append(usages, "CRL Sign")
	}
	return usages
}

// formatExtKeyUsage formats extended key usage
func formatExtKeyUsage(usage []x509.ExtKeyUsage) []string {
	var usages []string
	for _, u := range usage {
		switch u {
		case x509.ExtKeyUsageServerAuth:
			usages = append(usages, "Server Authentication")
		case x509.ExtKeyUsageClientAuth:
			usages = append(usages, "Client Authentication")
		case x509.ExtKeyUsageCodeSigning:
			usages = append(usages, "Code Signing")
		case x509.ExtKeyUsageEmailProtection:
			usages = append(usages, "Email Protection")
		case x509.ExtKeyUsageTimeStamping:
			usages = append(usages, "Time Stamping")
		case x509.ExtKeyUsageOCSPSigning:
			usages = append(usages, "OCSP Signing")
		}
	}
	return usages
}
