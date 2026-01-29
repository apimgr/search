package direct

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/apimgr/search/src/common/version"
)

// DNSHandler handles dns:{domain} queries
type DNSHandler struct {
	resolver *net.Resolver
}

// NewDNSHandler creates a new DNS handler
func NewDNSHandler() *DNSHandler {
	return &DNSHandler{
		resolver: &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
				d := net.Dialer{Timeout: 5 * time.Second}
				return d.DialContext(ctx, network, "8.8.8.8:53")
			},
		},
	}
}

func (h *DNSHandler) Type() AnswerType {
	return AnswerTypeDNS
}

func (h *DNSHandler) Handle(ctx context.Context, term string) (*Answer, error) {
	term = strings.TrimSpace(term)
	if term == "" {
		return nil, fmt.Errorf("domain name required")
	}

	// Parse for specific record type: dns:example.com/mx
	domain := term
	recordType := ""
	if idx := strings.LastIndex(term, "/"); idx > 0 {
		domain = term[:idx]
		recordType = strings.ToUpper(term[idx+1:])
	}

	records := make(map[string]interface{})
	var errors []string

	// Lookup A records
	if recordType == "" || recordType == "A" {
		addrs, err := h.resolver.LookupIP(ctx, "ip4", domain)
		if err == nil && len(addrs) > 0 {
			ips := make([]string, len(addrs))
			for i, addr := range addrs {
				ips[i] = addr.String()
			}
			records["A"] = ips
		} else if err != nil {
			errors = append(errors, fmt.Sprintf("A: %v", err))
		}
	}

	// Lookup AAAA records
	if recordType == "" || recordType == "AAAA" {
		addrs, err := h.resolver.LookupIP(ctx, "ip6", domain)
		if err == nil && len(addrs) > 0 {
			ips := make([]string, len(addrs))
			for i, addr := range addrs {
				ips[i] = addr.String()
			}
			records["AAAA"] = ips
		}
	}

	// Lookup MX records
	if recordType == "" || recordType == "MX" {
		mxs, err := h.resolver.LookupMX(ctx, domain)
		if err == nil && len(mxs) > 0 {
			mxRecords := make([]map[string]interface{}, len(mxs))
			for i, mx := range mxs {
				mxRecords[i] = map[string]interface{}{
					"host": mx.Host,
					"pref": mx.Pref,
				}
			}
			records["MX"] = mxRecords
		}
	}

	// Lookup NS records
	if recordType == "" || recordType == "NS" {
		nss, err := h.resolver.LookupNS(ctx, domain)
		if err == nil && len(nss) > 0 {
			nsRecords := make([]string, len(nss))
			for i, ns := range nss {
				nsRecords[i] = ns.Host
			}
			records["NS"] = nsRecords
		}
	}

	// Lookup TXT records
	if recordType == "" || recordType == "TXT" {
		txts, err := h.resolver.LookupTXT(ctx, domain)
		if err == nil && len(txts) > 0 {
			records["TXT"] = txts
		}
	}

	// Lookup CNAME record
	if recordType == "" || recordType == "CNAME" {
		cname, err := h.resolver.LookupCNAME(ctx, domain)
		if err == nil && cname != "" && cname != domain+"." {
			records["CNAME"] = cname
		}
	}

	if len(records) == 0 {
		return &Answer{
			Type:        AnswerTypeDNS,
			Term:        term,
			Title:       fmt.Sprintf("DNS: %s", domain),
			Description: "No DNS records found",
			Content:     fmt.Sprintf("<p>No DNS records found for <code>%s</code>.</p>", escapeHTML(domain)),
			Error:       "not_found",
		}, nil
	}

	return &Answer{
		Type:        AnswerTypeDNS,
		Term:        term,
		Title:       fmt.Sprintf("DNS: %s", domain),
		Description: fmt.Sprintf("DNS records for %s", domain),
		Content:     formatDNSRecords(domain, records),
		Source:      "DNS Resolver",
		Data: map[string]interface{}{
			"domain":  domain,
			"records": records,
		},
	}, nil
}

func formatDNSRecords(domain string, records map[string]interface{}) string {
	var html strings.Builder
	html.WriteString("<div class=\"dns-records\">")
	html.WriteString(fmt.Sprintf("<h2>DNS Records for %s</h2>", escapeHTML(domain)))
	html.WriteString("<table class=\"records-table\">")
	html.WriteString("<thead><tr><th>Type</th><th>Value</th></tr></thead>")
	html.WriteString("<tbody>")

	for recordType, values := range records {
		switch v := values.(type) {
		case []string:
			for _, val := range v {
				html.WriteString(fmt.Sprintf("<tr><td>%s</td><td><code>%s</code></td></tr>", recordType, escapeHTML(val)))
			}
		case []map[string]interface{}:
			for _, val := range v {
				if recordType == "MX" {
					html.WriteString(fmt.Sprintf("<tr><td>%s</td><td><code>%v %s</code></td></tr>",
						recordType, val["pref"], escapeHTML(val["host"].(string))))
				}
			}
		case string:
			html.WriteString(fmt.Sprintf("<tr><td>%s</td><td><code>%s</code></td></tr>", recordType, escapeHTML(v)))
		}
	}

	html.WriteString("</tbody></table></div>")
	return html.String()
}

// WhoisHandler handles whois:{domain} queries
type WhoisHandler struct {
	client *http.Client
}

// NewWhoisHandler creates a new WHOIS handler
func NewWhoisHandler() *WhoisHandler {
	return &WhoisHandler{
		client: &http.Client{Timeout: 15 * time.Second},
	}
}

func (h *WhoisHandler) Type() AnswerType {
	return AnswerTypeWhois
}

func (h *WhoisHandler) Handle(ctx context.Context, term string) (*Answer, error) {
	term = strings.TrimSpace(strings.ToLower(term))
	if term == "" {
		return nil, fmt.Errorf("domain name required")
	}

	// Use RDAP API for WHOIS data (modern replacement)
	apiURL := fmt.Sprintf("https://rdap.org/domain/%s", url.PathEscape(term))

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", version.BrowserUserAgent)
	req.Header.Set("Accept", "application/rdap+json, application/json")

	resp, err := h.client.Do(req)
	if err != nil {
		return h.fallbackWhois(ctx, term)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return h.fallbackWhois(ctx, term)
	}

	var rdap struct {
		Handle   string `json:"handle"`
		LdhName  string `json:"ldhName"`
		Status   []string `json:"status"`
		Events   []struct {
			EventAction string `json:"eventAction"`
			EventDate   string `json:"eventDate"`
		} `json:"events"`
		Nameservers []struct {
			LdhName string `json:"ldhName"`
		} `json:"nameservers"`
		Entities []struct {
			Roles      []string `json:"roles"`
			VcardArray []interface{} `json:"vcardArray"`
		} `json:"entities"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&rdap); err != nil {
		return h.fallbackWhois(ctx, term)
	}

	// Format RDAP data
	data := make(map[string]interface{})
	data["domain"] = rdap.LdhName
	data["status"] = rdap.Status

	for _, event := range rdap.Events {
		switch event.EventAction {
		case "registration":
			data["created"] = event.EventDate
		case "expiration":
			data["expires"] = event.EventDate
		case "last changed":
			data["updated"] = event.EventDate
		}
	}

	nameservers := make([]string, len(rdap.Nameservers))
	for i, ns := range rdap.Nameservers {
		nameservers[i] = ns.LdhName
	}
	data["nameservers"] = nameservers

	return &Answer{
		Type:        AnswerTypeWhois,
		Term:        term,
		Title:       fmt.Sprintf("WHOIS: %s", term),
		Description: fmt.Sprintf("Domain registration information for %s", term),
		Content:     formatWhoisData(term, data),
		Source:      "RDAP",
		SourceURL:   apiURL,
		Data:        data,
	}, nil
}

func (h *WhoisHandler) fallbackWhois(ctx context.Context, domain string) (*Answer, error) {
	return &Answer{
		Type:        AnswerTypeWhois,
		Term:        domain,
		Title:       fmt.Sprintf("WHOIS: %s", domain),
		Description: "WHOIS lookup failed",
		Content:     fmt.Sprintf("<p>Unable to fetch WHOIS data for <code>%s</code>. Try again later.</p>", escapeHTML(domain)),
		Error:       "lookup_failed",
	}, nil
}

func formatWhoisData(domain string, data map[string]interface{}) string {
	var html strings.Builder
	html.WriteString("<div class=\"whois-data\">")
	html.WriteString(fmt.Sprintf("<h2>WHOIS: %s</h2>", escapeHTML(domain)))
	html.WriteString("<dl class=\"whois-details\">")

	if v, ok := data["created"].(string); ok && v != "" {
		html.WriteString(fmt.Sprintf("<dt>Registered</dt><dd>%s</dd>", escapeHTML(v)))
	}
	if v, ok := data["expires"].(string); ok && v != "" {
		html.WriteString(fmt.Sprintf("<dt>Expires</dt><dd>%s</dd>", escapeHTML(v)))
	}
	if v, ok := data["updated"].(string); ok && v != "" {
		html.WriteString(fmt.Sprintf("<dt>Updated</dt><dd>%s</dd>", escapeHTML(v)))
	}
	if v, ok := data["status"].([]string); ok && len(v) > 0 {
		html.WriteString("<dt>Status</dt><dd>")
		for _, s := range v {
			html.WriteString(fmt.Sprintf("<span class=\"status-badge\">%s</span> ", escapeHTML(s)))
		}
		html.WriteString("</dd>")
	}
	if v, ok := data["nameservers"].([]string); ok && len(v) > 0 {
		html.WriteString("<dt>Name Servers</dt><dd><ul>")
		for _, ns := range v {
			html.WriteString(fmt.Sprintf("<li><code>%s</code></li>", escapeHTML(ns)))
		}
		html.WriteString("</ul></dd>")
	}

	html.WriteString("</dl></div>")
	return html.String()
}

// ResolveHandler handles resolve:{hostname} queries
type ResolveHandler struct {
	resolver *net.Resolver
}

// NewResolveHandler creates a new resolve handler
func NewResolveHandler() *ResolveHandler {
	return &ResolveHandler{
		resolver: &net.Resolver{PreferGo: true},
	}
}

func (h *ResolveHandler) Type() AnswerType {
	return AnswerTypeResolve
}

func (h *ResolveHandler) Handle(ctx context.Context, term string) (*Answer, error) {
	term = strings.TrimSpace(term)
	if term == "" {
		return nil, fmt.Errorf("hostname required")
	}

	data := make(map[string]interface{})
	data["hostname"] = term

	// Resolve IPv4
	ipv4Addrs, err := h.resolver.LookupIP(ctx, "ip4", term)
	if err == nil && len(ipv4Addrs) > 0 {
		ips := make([]string, len(ipv4Addrs))
		for i, addr := range ipv4Addrs {
			ips[i] = addr.String()
		}
		data["ipv4"] = ips
	}

	// Resolve IPv6
	ipv6Addrs, err := h.resolver.LookupIP(ctx, "ip6", term)
	if err == nil && len(ipv6Addrs) > 0 {
		ips := make([]string, len(ipv6Addrs))
		for i, addr := range ipv6Addrs {
			ips[i] = addr.String()
		}
		data["ipv6"] = ips
	}

	// Reverse DNS for first IP
	if ipv4Addrs != nil && len(ipv4Addrs) > 0 {
		names, err := h.resolver.LookupAddr(ctx, ipv4Addrs[0].String())
		if err == nil && len(names) > 0 {
			data["ptr"] = names
		}
	}

	if data["ipv4"] == nil && data["ipv6"] == nil {
		return &Answer{
			Type:        AnswerTypeResolve,
			Term:        term,
			Title:       fmt.Sprintf("Resolve: %s", term),
			Description: "Hostname not found",
			Content:     fmt.Sprintf("<p>Unable to resolve <code>%s</code>.</p>", escapeHTML(term)),
			Error:       "not_found",
		}, nil
	}

	return &Answer{
		Type:        AnswerTypeResolve,
		Term:        term,
		Title:       fmt.Sprintf("Resolve: %s", term),
		Description: fmt.Sprintf("IP resolution for %s", term),
		Content:     formatResolveData(term, data),
		Source:      "DNS Resolver",
		Data:        data,
	}, nil
}

func formatResolveData(hostname string, data map[string]interface{}) string {
	var html strings.Builder
	html.WriteString("<div class=\"resolve-data\">")
	html.WriteString(fmt.Sprintf("<h2>%s</h2>", escapeHTML(hostname)))

	if v, ok := data["ipv4"].([]string); ok && len(v) > 0 {
		html.WriteString("<h3>IPv4 Addresses</h3><ul>")
		for _, ip := range v {
			html.WriteString(fmt.Sprintf("<li><code>%s</code></li>", ip))
		}
		html.WriteString("</ul>")
	}

	if v, ok := data["ipv6"].([]string); ok && len(v) > 0 {
		html.WriteString("<h3>IPv6 Addresses</h3><ul>")
		for _, ip := range v {
			html.WriteString(fmt.Sprintf("<li><code>%s</code></li>", ip))
		}
		html.WriteString("</ul>")
	}

	if v, ok := data["ptr"].([]string); ok && len(v) > 0 {
		html.WriteString("<h3>Reverse DNS</h3><ul>")
		for _, name := range v {
			html.WriteString(fmt.Sprintf("<li><code>%s</code></li>", escapeHTML(name)))
		}
		html.WriteString("</ul>")
	}

	html.WriteString("</div>")
	return html.String()
}

// CertHandler handles cert:{domain} queries
type CertHandler struct {
	client *http.Client
}

// NewCertHandler creates a new certificate handler
func NewCertHandler() *CertHandler {
	return &CertHandler{
		client: &http.Client{
			Timeout: 15 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
		},
	}
}

func (h *CertHandler) Type() AnswerType {
	return AnswerTypeCert
}

func (h *CertHandler) Handle(ctx context.Context, term string) (*Answer, error) {
	term = strings.TrimSpace(term)
	if term == "" {
		return nil, fmt.Errorf("domain name required")
	}

	// Add port if not specified
	host := term
	if !strings.Contains(term, ":") {
		host = term + ":443"
	}

	// Create TLS connection
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	conn, err := tls.DialWithDialer(dialer, "tcp", host, &tls.Config{
		InsecureSkipVerify: true,
	})
	if err != nil {
		return &Answer{
			Type:        AnswerTypeCert,
			Term:        term,
			Title:       fmt.Sprintf("Certificate: %s", term),
			Description: "Failed to connect",
			Content:     fmt.Sprintf("<p>Unable to connect to <code>%s</code>: %s</p>", escapeHTML(term), escapeHTML(err.Error())),
			Error:       "connection_failed",
		}, nil
	}
	defer conn.Close()

	certs := conn.ConnectionState().PeerCertificates
	if len(certs) == 0 {
		return &Answer{
			Type:        AnswerTypeCert,
			Term:        term,
			Title:       fmt.Sprintf("Certificate: %s", term),
			Description: "No certificate found",
			Content:     "<p>No SSL/TLS certificate found.</p>",
			Error:       "no_cert",
		}, nil
	}

	cert := certs[0]
	data := map[string]interface{}{
		"subject":    cert.Subject.String(),
		"issuer":     cert.Issuer.String(),
		"notBefore":  cert.NotBefore.Format(time.RFC3339),
		"notAfter":   cert.NotAfter.Format(time.RFC3339),
		"sans":       cert.DNSNames,
		"serial":     cert.SerialNumber.String(),
		"version":    cert.Version,
		"keyAlgo":    cert.PublicKeyAlgorithm.String(),
		"sigAlgo":    cert.SignatureAlgorithm.String(),
		"isCA":       cert.IsCA,
		"chainLen":   len(certs),
	}

	// Check validity
	now := time.Now()
	valid := now.After(cert.NotBefore) && now.Before(cert.NotAfter)
	daysUntilExpiry := int(cert.NotAfter.Sub(now).Hours() / 24)
	data["valid"] = valid
	data["daysUntilExpiry"] = daysUntilExpiry

	return &Answer{
		Type:        AnswerTypeCert,
		Term:        term,
		Title:       fmt.Sprintf("Certificate: %s", term),
		Description: fmt.Sprintf("SSL/TLS certificate for %s", term),
		Content:     formatCertData(term, data),
		Source:      "TLS Connection",
		Data:        data,
	}, nil
}

func formatCertData(domain string, data map[string]interface{}) string {
	var html strings.Builder
	html.WriteString("<div class=\"cert-data\">")
	html.WriteString(fmt.Sprintf("<h2>Certificate for %s</h2>", escapeHTML(domain)))

	// Validity status
	valid := data["valid"].(bool)
	days := data["daysUntilExpiry"].(int)
	if valid {
		if days < 30 {
			html.WriteString(fmt.Sprintf("<p class=\"status warning\">Valid (expires in %d days)</p>", days))
		} else {
			html.WriteString(fmt.Sprintf("<p class=\"status valid\">Valid (%d days remaining)</p>", days))
		}
	} else {
		html.WriteString("<p class=\"status invalid\">Invalid or Expired</p>")
	}

	html.WriteString("<dl class=\"cert-details\">")
	html.WriteString(fmt.Sprintf("<dt>Subject</dt><dd><code>%s</code></dd>", escapeHTML(data["subject"].(string))))
	html.WriteString(fmt.Sprintf("<dt>Issuer</dt><dd><code>%s</code></dd>", escapeHTML(data["issuer"].(string))))
	html.WriteString(fmt.Sprintf("<dt>Valid From</dt><dd>%s</dd>", escapeHTML(data["notBefore"].(string))))
	html.WriteString(fmt.Sprintf("<dt>Valid Until</dt><dd>%s</dd>", escapeHTML(data["notAfter"].(string))))

	if sans, ok := data["sans"].([]string); ok && len(sans) > 0 {
		html.WriteString("<dt>Subject Alternative Names</dt><dd><ul>")
		for _, san := range sans {
			html.WriteString(fmt.Sprintf("<li><code>%s</code></li>", escapeHTML(san)))
		}
		html.WriteString("</ul></dd>")
	}

	html.WriteString(fmt.Sprintf("<dt>Key Algorithm</dt><dd>%s</dd>", escapeHTML(data["keyAlgo"].(string))))
	html.WriteString(fmt.Sprintf("<dt>Signature Algorithm</dt><dd>%s</dd>", escapeHTML(data["sigAlgo"].(string))))
	html.WriteString(fmt.Sprintf("<dt>Chain Length</dt><dd>%d certificates</dd>", data["chainLen"].(int)))

	html.WriteString("</dl></div>")
	return html.String()
}

// HeadersHandler handles headers:{url} queries
type HeadersHandler struct {
	client *http.Client
}

// NewHeadersHandler creates a new HTTP headers handler
func NewHeadersHandler() *HeadersHandler {
	return &HeadersHandler{
		client: &http.Client{
			Timeout: 15 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse // Don't follow redirects
			},
		},
	}
}

func (h *HeadersHandler) Type() AnswerType {
	return AnswerTypeHeaders
}

func (h *HeadersHandler) Handle(ctx context.Context, term string) (*Answer, error) {
	term = strings.TrimSpace(term)
	if term == "" {
		return nil, fmt.Errorf("URL required")
	}

	// Add https:// if no scheme
	targetURL := term
	if !strings.HasPrefix(term, "http://") && !strings.HasPrefix(term, "https://") {
		targetURL = "https://" + term
	}

	req, err := http.NewRequestWithContext(ctx, "HEAD", targetURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", version.BrowserUserAgent)

	resp, err := h.client.Do(req)
	if err != nil {
		return &Answer{
			Type:        AnswerTypeHeaders,
			Term:        term,
			Title:       fmt.Sprintf("Headers: %s", term),
			Description: "Request failed",
			Content:     fmt.Sprintf("<p>Unable to fetch headers from <code>%s</code>: %s</p>", escapeHTML(targetURL), escapeHTML(err.Error())),
			Error:       "request_failed",
		}, nil
	}
	defer resp.Body.Close()

	headers := make(map[string][]string)
	for key, values := range resp.Header {
		headers[key] = values
	}

	// Analyze security headers
	securityAnalysis := analyzeSecurityHeaders(headers)

	data := map[string]interface{}{
		"url":        targetURL,
		"status":     resp.StatusCode,
		"statusText": resp.Status,
		"headers":    headers,
		"security":   securityAnalysis,
	}

	return &Answer{
		Type:        AnswerTypeHeaders,
		Term:        term,
		Title:       fmt.Sprintf("Headers: %s", term),
		Description: fmt.Sprintf("HTTP headers for %s", term),
		Content:     formatHeadersData(targetURL, data),
		Source:      "HTTP Request",
		Data:        data,
	}, nil
}

func analyzeSecurityHeaders(headers map[string][]string) map[string]interface{} {
	analysis := make(map[string]interface{})

	// Check important security headers
	secHeaders := map[string]bool{
		"Strict-Transport-Security": false,
		"Content-Security-Policy":   false,
		"X-Frame-Options":           false,
		"X-Content-Type-Options":    false,
		"X-XSS-Protection":          false,
		"Referrer-Policy":           false,
		"Permissions-Policy":        false,
	}

	for header := range secHeaders {
		for key := range headers {
			if strings.EqualFold(key, header) {
				secHeaders[header] = true
				break
			}
		}
	}

	// Calculate grade
	present := 0
	for _, v := range secHeaders {
		if v {
			present++
		}
	}

	var grade string
	switch {
	case present >= 6:
		grade = "A"
	case present >= 4:
		grade = "B"
	case present >= 2:
		grade = "C"
	default:
		grade = "D"
	}

	analysis["headers"] = secHeaders
	analysis["grade"] = grade
	analysis["present"] = present
	analysis["total"] = len(secHeaders)

	return analysis
}

func formatHeadersData(url string, data map[string]interface{}) string {
	var html strings.Builder
	html.WriteString("<div class=\"headers-data\">")
	html.WriteString(fmt.Sprintf("<h2>HTTP Headers</h2>"))
	html.WriteString(fmt.Sprintf("<p><code>%s</code></p>", escapeHTML(url)))
	html.WriteString(fmt.Sprintf("<p>Status: <strong>%s</strong></p>", escapeHTML(data["statusText"].(string))))

	// Security grade
	if sec, ok := data["security"].(map[string]interface{}); ok {
		grade := sec["grade"].(string)
		html.WriteString(fmt.Sprintf("<p>Security Grade: <span class=\"grade grade-%s\">%s</span> (%d/%d headers present)</p>",
			strings.ToLower(grade), grade, sec["present"].(int), sec["total"].(int)))
	}

	// All headers
	html.WriteString("<h3>Response Headers</h3>")
	html.WriteString("<table class=\"headers-table\">")
	html.WriteString("<thead><tr><th>Header</th><th>Value</th></tr></thead>")
	html.WriteString("<tbody>")

	if headers, ok := data["headers"].(map[string][]string); ok {
		for key, values := range headers {
			for _, val := range values {
				html.WriteString(fmt.Sprintf("<tr><td><code>%s</code></td><td><code>%s</code></td></tr>",
					escapeHTML(key), escapeHTML(val)))
			}
		}
	}

	html.WriteString("</tbody></table></div>")
	return html.String()
}

// ASNHandler handles asn:{number} queries
type ASNHandler struct {
	client *http.Client
}

// NewASNHandler creates a new ASN handler
func NewASNHandler() *ASNHandler {
	return &ASNHandler{
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

func (h *ASNHandler) Type() AnswerType {
	return AnswerTypeASN
}

func (h *ASNHandler) Handle(ctx context.Context, term string) (*Answer, error) {
	term = strings.TrimSpace(strings.ToUpper(term))
	if term == "" {
		return nil, fmt.Errorf("ASN number required")
	}

	// Remove AS prefix if present
	asn := strings.TrimPrefix(term, "AS")

	// Fetch from BGPView API
	apiURL := fmt.Sprintf("https://api.bgpview.io/asn/%s", url.PathEscape(asn))

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", version.BrowserUserAgent)

	resp, err := h.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Data struct {
			ASN            int    `json:"asn"`
			Name           string `json:"name"`
			Description    string `json:"description_short"`
			CountryCode    string `json:"country_code"`
			EmailContacts  []string `json:"email_contacts"`
			AbuseContacts  []string `json:"abuse_contacts"`
			RIRAllocation  struct {
				RIRName      string `json:"rir_name"`
				CountryCode  string `json:"country_code"`
				DateAllocated string `json:"date_allocated"`
			} `json:"rir_allocation"`
		} `json:"data"`
		Status        string `json:"status"`
		StatusMessage string `json:"status_message"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	if result.Status != "ok" {
		return &Answer{
			Type:        AnswerTypeASN,
			Term:        term,
			Title:       fmt.Sprintf("ASN: %s", term),
			Description: "ASN not found",
			Content:     fmt.Sprintf("<p>No information found for ASN <code>%s</code>.</p>", escapeHTML(term)),
			Error:       "not_found",
		}, nil
	}

	data := map[string]interface{}{
		"asn":           result.Data.ASN,
		"name":          result.Data.Name,
		"description":   result.Data.Description,
		"country":       result.Data.CountryCode,
		"rir":           result.Data.RIRAllocation.RIRName,
		"dateAllocated": result.Data.RIRAllocation.DateAllocated,
	}

	return &Answer{
		Type:        AnswerTypeASN,
		Term:        term,
		Title:       fmt.Sprintf("AS%d - %s", result.Data.ASN, result.Data.Name),
		Description: result.Data.Description,
		Content:     formatASNData(data),
		Source:      "BGPView",
		SourceURL:   fmt.Sprintf("https://bgpview.io/asn/%s", asn),
		Data:        data,
	}, nil
}

func formatASNData(data map[string]interface{}) string {
	var html strings.Builder
	html.WriteString("<div class=\"asn-data\">")
	html.WriteString(fmt.Sprintf("<h2>AS%d</h2>", data["asn"].(int)))
	html.WriteString(fmt.Sprintf("<p class=\"asn-name\">%s</p>", escapeHTML(data["name"].(string))))

	html.WriteString("<dl class=\"asn-details\">")
	if v, ok := data["description"].(string); ok && v != "" {
		html.WriteString(fmt.Sprintf("<dt>Description</dt><dd>%s</dd>", escapeHTML(v)))
	}
	if v, ok := data["country"].(string); ok && v != "" {
		html.WriteString(fmt.Sprintf("<dt>Country</dt><dd>%s</dd>", escapeHTML(v)))
	}
	if v, ok := data["rir"].(string); ok && v != "" {
		html.WriteString(fmt.Sprintf("<dt>RIR</dt><dd>%s</dd>", escapeHTML(v)))
	}
	if v, ok := data["dateAllocated"].(string); ok && v != "" {
		html.WriteString(fmt.Sprintf("<dt>Allocated</dt><dd>%s</dd>", escapeHTML(v)))
	}
	html.WriteString("</dl></div>")

	return html.String()
}

// SubnetHandler handles subnet:{cidr} queries
type SubnetHandler struct{}

// NewSubnetHandler creates a new subnet calculator handler
func NewSubnetHandler() *SubnetHandler {
	return &SubnetHandler{}
}

func (h *SubnetHandler) Type() AnswerType {
	return AnswerTypeSubnet
}

func (h *SubnetHandler) Handle(ctx context.Context, term string) (*Answer, error) {
	term = strings.TrimSpace(term)
	if term == "" {
		return nil, fmt.Errorf("CIDR notation required (e.g., 192.168.1.0/24)")
	}

	// Parse CIDR
	_, ipNet, err := net.ParseCIDR(term)
	if err != nil {
		// Try parsing as just IP and default to /24
		ip := net.ParseIP(term)
		if ip != nil {
			_, ipNet, _ = net.ParseCIDR(term + "/24")
		}
		if ipNet == nil {
			return &Answer{
				Type:        AnswerTypeSubnet,
				Term:        term,
				Title:       fmt.Sprintf("Subnet: %s", term),
				Description: "Invalid CIDR notation",
				Content:     fmt.Sprintf("<p>Invalid CIDR notation: <code>%s</code>. Use format like <code>192.168.1.0/24</code>.</p>", escapeHTML(term)),
				Error:       "invalid_cidr",
			}, nil
		}
	}

	// Calculate subnet details
	ones, bits := ipNet.Mask.Size()
	isIPv6 := bits == 128

	var totalHosts, usableHosts *big.Int
	if isIPv6 {
		totalHosts = new(big.Int).Lsh(big.NewInt(1), uint(bits-ones))
		usableHosts = new(big.Int).Sub(totalHosts, big.NewInt(2))
		if usableHosts.Sign() < 0 {
			usableHosts = big.NewInt(0)
		}
	} else {
		hostBits := bits - ones
		totalHosts = big.NewInt(1 << hostBits)
		if hostBits > 1 {
			usableHosts = big.NewInt((1 << hostBits) - 2)
		} else {
			usableHosts = big.NewInt(0)
		}
	}

	// Calculate network and broadcast addresses
	networkAddr := ipNet.IP.String()
	broadcastAddr := calculateBroadcast(ipNet)

	// Calculate first and last usable
	firstUsable := incrementIP(ipNet.IP)
	lastUsable := decrementIP(net.ParseIP(broadcastAddr))

	// Calculate wildcard mask
	wildcardMask := make(net.IP, len(ipNet.Mask))
	for i := range ipNet.Mask {
		wildcardMask[i] = ^ipNet.Mask[i]
	}

	data := map[string]interface{}{
		"cidr":         term,
		"network":      networkAddr,
		"broadcast":    broadcastAddr,
		"firstUsable":  firstUsable.String(),
		"lastUsable":   lastUsable.String(),
		"subnetMask":   net.IP(ipNet.Mask).String(),
		"wildcardMask": wildcardMask.String(),
		"prefixLength": ones,
		"totalHosts":   totalHosts.String(),
		"usableHosts":  usableHosts.String(),
		"isIPv6":       isIPv6,
	}

	return &Answer{
		Type:        AnswerTypeSubnet,
		Term:        term,
		Title:       fmt.Sprintf("Subnet Calculator: %s", term),
		Description: fmt.Sprintf("Subnet calculation for %s", term),
		Content:     formatSubnetData(data),
		Source:      "Local Calculator",
		Data:        data,
	}, nil
}

func calculateBroadcast(ipNet *net.IPNet) string {
	ip := make(net.IP, len(ipNet.IP))
	copy(ip, ipNet.IP)

	for i := range ip {
		ip[i] |= ^ipNet.Mask[i]
	}

	return ip.String()
}

func incrementIP(ip net.IP) net.IP {
	result := make(net.IP, len(ip))
	copy(result, ip)

	for i := len(result) - 1; i >= 0; i-- {
		result[i]++
		if result[i] != 0 {
			break
		}
	}
	return result
}

func decrementIP(ip net.IP) net.IP {
	result := make(net.IP, len(ip))
	copy(result, ip)

	for i := len(result) - 1; i >= 0; i-- {
		if result[i] > 0 {
			result[i]--
			break
		}
		result[i] = 255
	}
	return result
}

func formatSubnetData(data map[string]interface{}) string {
	var html strings.Builder
	html.WriteString("<div class=\"subnet-data\">")
	html.WriteString(fmt.Sprintf("<h2>%s</h2>", escapeHTML(data["cidr"].(string))))

	html.WriteString("<table class=\"subnet-table\">")
	html.WriteString("<tbody>")
	html.WriteString(fmt.Sprintf("<tr><td>Network Address</td><td><code>%s</code></td></tr>", escapeHTML(data["network"].(string))))
	html.WriteString(fmt.Sprintf("<tr><td>Broadcast Address</td><td><code>%s</code></td></tr>", escapeHTML(data["broadcast"].(string))))
	html.WriteString(fmt.Sprintf("<tr><td>First Usable</td><td><code>%s</code></td></tr>", escapeHTML(data["firstUsable"].(string))))
	html.WriteString(fmt.Sprintf("<tr><td>Last Usable</td><td><code>%s</code></td></tr>", escapeHTML(data["lastUsable"].(string))))
	html.WriteString(fmt.Sprintf("<tr><td>Subnet Mask</td><td><code>%s</code></td></tr>", escapeHTML(data["subnetMask"].(string))))
	html.WriteString(fmt.Sprintf("<tr><td>Wildcard Mask</td><td><code>%s</code></td></tr>", escapeHTML(data["wildcardMask"].(string))))
	html.WriteString(fmt.Sprintf("<tr><td>Prefix Length</td><td><code>/%d</code></td></tr>", data["prefixLength"].(int)))
	html.WriteString(fmt.Sprintf("<tr><td>Total Hosts</td><td>%s</td></tr>", data["totalHosts"].(string)))
	html.WriteString(fmt.Sprintf("<tr><td>Usable Hosts</td><td>%s</td></tr>", data["usableHosts"].(string)))
	html.WriteString("</tbody></table>")

	// Binary representation for IPv4
	if !data["isIPv6"].(bool) {
		html.WriteString("<h3>Binary Representation</h3>")
		html.WriteString("<pre class=\"binary\">")

		// Parse network address for binary display
		cidr := data["cidr"].(string)
		if ip, _, err := net.ParseCIDR(cidr); err == nil {
			ip = ip.To4()
			if ip != nil {
				prefixLen := data["prefixLength"].(int)
				for i, octet := range ip {
					if i > 0 {
						html.WriteString(".")
					}
					for bit := 7; bit >= 0; bit-- {
						bitPos := i*8 + (7 - bit)
						if bitPos < prefixLen {
							html.WriteString(fmt.Sprintf("<span class=\"network-bit\">%d</span>", (octet>>bit)&1))
						} else {
							html.WriteString(fmt.Sprintf("<span class=\"host-bit\">%d</span>", (octet>>bit)&1))
						}
					}
				}
			}
		}
		html.WriteString("</pre>")
		html.WriteString("<p><small><span class=\"network-bit\">Network bits</span> | <span class=\"host-bit\">Host bits</span></small></p>")
	}

	html.WriteString("</div>")
	return html.String()
}

// ipRe is a compiled regex for IP validation
var ipRe = regexp.MustCompile(`^\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}$`)

// parseASN parses an ASN string and returns the number
func parseASN(s string) (int, error) {
	s = strings.TrimPrefix(strings.ToUpper(s), "AS")
	return strconv.Atoi(s)
}
