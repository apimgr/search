package direct

import (
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/apimgr/search/src/common/closeutil"
	"github.com/apimgr/search/src/common/i18n"
	"github.com/apimgr/search/src/geoip"
	"github.com/apimgr/search/src/version"
	"github.com/miekg/dns"
)

var dnsResolverAddress = "8.8.8.8:53"

// DNSHandler handles dns:{domain} queries
type DNSHandler struct {
	resolverAddress string
}

// NewDNSHandler creates a new DNS handler
func NewDNSHandler() *DNSHandler {
	return &DNSHandler{
		resolverAddress: dnsResolverAddress,
	}
}

func (h *DNSHandler) Type() AnswerType {
	return AnswerTypeDNS
}

func (h *DNSHandler) HandleDirectQuery(ctx context.Context, term string) (*Answer, error) {
	term = strings.TrimSpace(term)
	if term == "" {
		return nil, fmt.Errorf("domain name required")
	}

	lang := LangFromContext(ctx)

	// Parse for specific record type: dns:example.com/mx
	domain := term
	recordType := ""
	if idx := strings.LastIndex(term, "/"); idx > 0 {
		domain = term[:idx]
		recordType = strings.ToUpper(term[idx+1:])
	}

	records := make(map[string]interface{})

	queryTypes := []struct {
		name string
		code uint16
	}{{"A", dns.TypeA}, {"AAAA", dns.TypeAAAA}, {"MX", dns.TypeMX}, {"NS", dns.TypeNS}, {"TXT", dns.TypeTXT}, {"CNAME", dns.TypeCNAME}, {"SOA", dns.TypeSOA}}
	for _, queryType := range queryTypes {
		if recordType != "" && recordType != queryType.name {
			continue
		}
		answers, err := h.lookup(ctx, domain, queryType.code)
		if err != nil || len(answers) == 0 {
			continue
		}
		values := make([]map[string]interface{}, 0, len(answers))
		for _, answer := range answers {
			value := mapDNSRecord(answer)
			if value != nil {
				values = append(values, value)
			}
		}
		if len(values) > 0 {
			records[queryType.name] = values
		}
	}

	if len(records) == 0 {
		return &Answer{
			Type:        AnswerTypeDNS,
			Term:        term,
			Title:       fmt.Sprintf("DNS: %s", domain),
			Description: i18n.T(lang, "direct.no_dns_records_found"),
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

func (h *DNSHandler) lookup(ctx context.Context, domain string, recordType uint16) ([]dns.RR, error) {
	message := new(dns.Msg)
	message.SetQuestion(dns.Fqdn(domain), recordType)
	client := &dns.Client{Net: "udp", Timeout: 5 * time.Second}
	response, _, err := client.ExchangeContext(ctx, message, h.resolverAddress)
	if err != nil {
		return nil, err
	}
	if response.Truncated {
		client.Net = "tcp"
		response, _, err = client.ExchangeContext(ctx, message, h.resolverAddress)
		if err != nil {
			return nil, err
		}
	}
	if response.Rcode != dns.RcodeSuccess {
		return nil, fmt.Errorf("DNS lookup returned %s", dns.RcodeToString[response.Rcode])
	}
	return response.Answer, nil
}

func mapDNSRecord(record dns.RR) map[string]interface{} {
	header := record.Header()
	result := map[string]interface{}{"ttl": header.Ttl}
	switch value := record.(type) {
	case *dns.A:
		result["value"] = value.A.String()
	case *dns.AAAA:
		result["value"] = value.AAAA.String()
	case *dns.MX:
		result["host"] = value.Mx
		result["pref"] = value.Preference
	case *dns.NS:
		result["value"] = value.Ns
	case *dns.TXT:
		result["value"] = strings.Join(value.Txt, "")
	case *dns.CNAME:
		result["value"] = value.Target
	case *dns.SOA:
		result["mname"] = value.Ns
		result["rname"] = value.Mbox
		result["serial"] = value.Serial
		result["refresh"] = value.Refresh
		result["retry"] = value.Retry
		result["expire"] = value.Expire
		result["minimum"] = value.Minttl
	default:
		return nil
	}
	return result
}

func formatDNSRecords(domain string, records map[string]interface{}) string {
	var html strings.Builder
	html.WriteString("<div class=\"dns-records\">")
	fmt.Fprintf(&html, "<h2>DNS Records for %s</h2>", escapeHTML(domain))
	html.WriteString("<table class=\"records-table\">")
	html.WriteString("<thead><tr><th>Type</th><th>Value</th></tr></thead>")
	html.WriteString("<tbody>")

	recordTypes := make([]string, 0, len(records))
	for recordType := range records {
		recordTypes = append(recordTypes, recordType)
	}
	sort.Strings(recordTypes)
	for _, recordType := range recordTypes {
		values := records[recordType]
		switch v := values.(type) {
		case []string:
			for _, val := range v {
				fmt.Fprintf(&html, "<tr><td>%s</td><td><code>%s</code></td></tr>", recordType, escapeHTML(val))
			}
		case []map[string]interface{}:
			for _, val := range v {
				switch recordType {
				case "MX":
					fmt.Fprintf(&html, "<tr><td>%s</td><td><code>%v %s</code> <span>TTL %ss</span></td></tr>",
						recordType, val["pref"], escapeHTML(val["host"].(string)), val["ttl"])
				case "SOA":
					fmt.Fprintf(&html, "<tr><td>SOA</td><td><code>%s %s</code> serial %v, refresh %v, retry %v, expire %v, minimum %v <span>TTL %ds</span></td></tr>",
						escapeHTML(val["mname"].(string)), escapeHTML(val["rname"].(string)), val["serial"], val["refresh"], val["retry"], val["expire"], val["minimum"], val["ttl"])
				default:
					fmt.Fprintf(&html, "<tr><td>%s</td><td><code>%s</code> <span>TTL %ss</span></td></tr>",
						escapeHTML(recordType), escapeHTML(val["value"].(string)), val["ttl"])
				}
			}
		case string:
			fmt.Fprintf(&html, "<tr><td>%s</td><td><code>%s</code></td></tr>", recordType, escapeHTML(v))
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

func (h *WhoisHandler) HandleDirectQuery(ctx context.Context, term string) (*Answer, error) {
	lang := LangFromContext(ctx)
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
	defer closeutil.Close(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return h.fallbackWhois(ctx, term)
	}

	var rdap struct {
		Handle  string   `json:"handle"`
		LdhName string   `json:"ldhName"`
		Status  []string `json:"status"`
		Events  []struct {
			EventAction string `json:"eventAction"`
			EventDate   string `json:"eventDate"`
		} `json:"events"`
		Nameservers []struct {
			LdhName string `json:"ldhName"`
		} `json:"nameservers"`
		Entities []struct {
			Handle     string        `json:"handle"`
			Roles      []string      `json:"roles"`
			VcardArray []interface{} `json:"vcardArray"`
		} `json:"entities"`
		SecureDNS *struct {
			DelegationSigned *bool `json:"delegationSigned"`
		} `json:"secureDNS"`
	}

	if err := decodeBoundedJSON(resp.Body, 2<<20, &rdap); err != nil {
		return h.fallbackWhois(ctx, term)
	}

	// Format RDAP data
	data := make(map[string]interface{})
	data["domain"] = rdap.LdhName
	data["status"] = rdap.Status
	data["dnssec_status"] = i18n.T(lang, "direct.whois_dnssec_unknown")
	if rdap.SecureDNS != nil && rdap.SecureDNS.DelegationSigned != nil {
		if *rdap.SecureDNS.DelegationSigned {
			data["dnssec_status"] = i18n.T(lang, "direct.whois_dnssec_signed")
		} else {
			data["dnssec_status"] = i18n.T(lang, "direct.whois_dnssec_unsigned")
		}
	}
	for _, entity := range rdap.Entities {
		name, organization, email := rdapVCardContact(entity.VcardArray)
		for _, role := range entity.Roles {
			switch strings.ToLower(role) {
			case "registrar":
				registrar := firstNonEmpty(name, organization, entity.Handle)
				if registrar != "" {
					data["registrar"] = registrar
				}
			case "registrant":
				registrant := make(map[string]string)
				if name != "" {
					registrant["name"] = name
				}
				if organization != "" {
					registrant["organization"] = organization
				}
				if email != "" {
					registrant["email"] = email
				}
				if len(registrant) > 0 {
					data["registrant"] = registrant
				}
			}
		}
	}

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
		Title:       fmt.Sprintf("%s: %s", i18n.T(lang, "direct.whois_title"), term),
		Description: strings.ReplaceAll(i18n.T(lang, "direct.whois_description"), "{domain}", term),
		Content:     formatWhoisData(lang, term, data),
		Source:      "RDAP",
		SourceURL:   apiURL,
		Data:        data,
	}, nil
}

func (h *WhoisHandler) fallbackWhois(ctx context.Context, domain string) (*Answer, error) {
	lang := LangFromContext(ctx)
	return &Answer{
		Type:        AnswerTypeWhois,
		Term:        domain,
		Title:       fmt.Sprintf("%s: %s", i18n.T(lang, "direct.whois_title"), domain),
		Description: i18n.T(lang, "direct.whois_lookup_failed"),
		Content:     fmt.Sprintf("<p>%s</p>", escapeHTML(strings.ReplaceAll(i18n.T(lang, "direct.whois_unavailable"), "{domain}", domain))),
		Error:       "lookup_failed",
	}, nil
}

func formatWhoisData(lang, domain string, data map[string]interface{}) string {
	var html strings.Builder
	html.WriteString("<div class=\"whois-data\">")
	fmt.Fprintf(&html, "<h2>%s: %s</h2>", escapeHTML(i18n.T(lang, "direct.whois_title")), escapeHTML(domain))
	html.WriteString("<dl class=\"whois-details\">")
	if v, ok := data["registrar"].(string); ok && v != "" {
		fmt.Fprintf(&html, "<dt>%s</dt><dd>%s</dd>", escapeHTML(i18n.T(lang, "direct.whois_registrar")), escapeHTML(v))
	}
	if v, ok := data["registrant"].(map[string]string); ok && len(v) > 0 {
		fmt.Fprintf(&html, "<dt>%s</dt><dd><ul>", escapeHTML(i18n.T(lang, "direct.whois_registrant")))
		for _, field := range []struct{ key, translationKey string }{{"name", "direct.whois_name"}, {"organization", "direct.whois_organization"}, {"email", "direct.whois_email"}} {
			if value := v[field.key]; value != "" {
				fmt.Fprintf(&html, "<li>%s: %s</li>", escapeHTML(i18n.T(lang, field.translationKey)), escapeHTML(value))
			}
		}
		html.WriteString("</ul></dd>")
	}

	if v, ok := data["created"].(string); ok && v != "" {
		fmt.Fprintf(&html, "<dt>%s</dt><dd>%s</dd>", escapeHTML(i18n.T(lang, "direct.whois_registered")), escapeHTML(v))
	}
	if v, ok := data["expires"].(string); ok && v != "" {
		fmt.Fprintf(&html, "<dt>%s</dt><dd>%s</dd>", escapeHTML(i18n.T(lang, "direct.whois_expires")), escapeHTML(v))
	}
	if v, ok := data["updated"].(string); ok && v != "" {
		fmt.Fprintf(&html, "<dt>%s</dt><dd>%s</dd>", escapeHTML(i18n.T(lang, "direct.whois_updated")), escapeHTML(v))
	}
	if v, ok := data["dnssec_status"].(string); ok {
		fmt.Fprintf(&html, "<dt>%s</dt><dd>%s</dd>", escapeHTML(i18n.T(lang, "direct.whois_dnssec")), escapeHTML(v))
	}
	if v, ok := data["status"].([]string); ok && len(v) > 0 {
		fmt.Fprintf(&html, "<dt>%s</dt><dd>", escapeHTML(i18n.T(lang, "direct.whois_status")))
		for _, s := range v {
			fmt.Fprintf(&html, "<span class=\"status-badge\">%s</span> ", escapeHTML(s))
		}
		html.WriteString("</dd>")
	}
	if v, ok := data["nameservers"].([]string); ok && len(v) > 0 {
		fmt.Fprintf(&html, "<dt>%s</dt><dd><ul>", escapeHTML(i18n.T(lang, "direct.whois_nameservers")))
		for _, ns := range v {
			fmt.Fprintf(&html, "<li><code>%s</code></li>", escapeHTML(ns))
		}
		html.WriteString("</ul></dd>")
	}

	html.WriteString("</dl></div>")
	return html.String()
}

func rdapVCardContact(vcard []interface{}) (name, organization, email string) {
	if len(vcard) != 2 {
		return "", "", ""
	}
	rows, ok := vcard[1].([]interface{})
	if !ok {
		return "", "", ""
	}
	for _, rawRow := range rows {
		row, ok := rawRow.([]interface{})
		if !ok || len(row) < 4 {
			continue
		}
		key, ok := row[0].(string)
		if !ok {
			continue
		}
		value := rdapVCardValue(row[3])
		switch strings.ToLower(key) {
		case "fn":
			name = value
		case "org":
			organization = value
		case "email":
			email = value
		}
	}
	return name, organization, email
}

func rdapVCardValue(raw interface{}) string {
	switch value := raw.(type) {
	case string:
		return strings.TrimSpace(value)
	case []interface{}:
		parts := make([]string, 0, len(value))
		for _, part := range value {
			if text, ok := part.(string); ok && strings.TrimSpace(text) != "" {
				parts = append(parts, strings.TrimSpace(text))
			}
		}
		return strings.Join(parts, ", ")
	default:
		return ""
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

// ResolveHandler handles resolve:{hostname} queries
type ResolveHandler struct {
	resolver *net.Resolver
}

var resolveDNSServers = []string{"1.1.1.1:53", "8.8.8.8:53", "9.9.9.9:53"}

// NewResolveHandler creates a new resolve handler
func NewResolveHandler() *ResolveHandler {
	return &ResolveHandler{
		resolver: &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
				return (&net.Dialer{Timeout: 3 * time.Second}).DialContext(ctx, network, dnsResolverAddress)
			},
		},
	}
}

func (h *ResolveHandler) Type() AnswerType {
	return AnswerTypeResolve
}

func (h *ResolveHandler) HandleDirectQuery(ctx context.Context, term string) (*Answer, error) {
	term = strings.TrimSpace(term)
	if term == "" {
		return nil, fmt.Errorf("hostname required")
	}

	lang := LangFromContext(ctx)

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
	if len(ipv4Addrs) > 0 {
		names, err := h.resolver.LookupAddr(ctx, ipv4Addrs[0].String())
		if err == nil && len(names) > 0 {
			data["ptr"] = names
		}
	}
	if lookup := geoIPLookupFromContext(ctx); lookup != nil {
		geo := make(map[string]*geoip.Result)
		for _, addr := range append(ipv4Addrs, ipv6Addrs...) {
			result := lookup.Lookup(addr.String())
			if result.Found {
				geo[addr.String()] = result
			}
		}
		if len(geo) > 0 {
			data["geoip"] = geo
		}
	}
	if timings := resolveDNSTimings(ctx, term); len(timings) > 0 {
		data["dns_timings"] = timings
	}

	if data["ipv4"] == nil && data["ipv6"] == nil {
		return &Answer{
			Type:        AnswerTypeResolve,
			Term:        term,
			Title:       fmt.Sprintf("Resolve: %s", term),
			Description: i18n.T(lang, "direct.hostname_not_found"),
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
	fmt.Fprintf(&html, "<h2>%s</h2>", escapeHTML(hostname))

	if v, ok := data["ipv4"].([]string); ok && len(v) > 0 {
		html.WriteString("<h3>IPv4 Addresses</h3><ul>")
		for _, ip := range v {
			fmt.Fprintf(&html, "<li><code>%s</code></li>", ip)
		}
		html.WriteString("</ul>")
	}

	if v, ok := data["ipv6"].([]string); ok && len(v) > 0 {
		html.WriteString("<h3>IPv6 Addresses</h3><ul>")
		for _, ip := range v {
			fmt.Fprintf(&html, "<li><code>%s</code></li>", ip)
		}
		html.WriteString("</ul>")
	}

	if v, ok := data["ptr"].([]string); ok && len(v) > 0 {
		html.WriteString("<h3>Reverse DNS</h3><ul>")
		for _, name := range v {
			fmt.Fprintf(&html, "<li><code>%s</code></li>", escapeHTML(name))
		}
		html.WriteString("</ul>")
	}
	if v, ok := data["geoip"].(map[string]*geoip.Result); ok && len(v) > 0 {
		html.WriteString("<h3>GeoIP and ASN</h3><ul>")
		for ip, result := range v {
			details := result.CountryName
			if result.City != "" {
				details = result.City + ", " + details
			}
			if result.ASN != 0 {
				details += fmt.Sprintf(" (AS%d", result.ASN)
				if result.ASNOrg != "" {
					details += " " + result.ASNOrg
				}
				details += ")"
			}
			fmt.Fprintf(&html, "<li><code>%s</code>: %s</li>", escapeHTML(ip), escapeHTML(details))
		}
		html.WriteString("</ul>")
	}
	if v, ok := data["dns_timings"].(map[string]time.Duration); ok && len(v) > 0 {
		html.WriteString("<h3>DNS Resolver Response Times</h3><ul>")
		for server, duration := range v {
			fmt.Fprintf(&html, "<li>%s: %s</li>", escapeHTML(strings.TrimSuffix(server, ":53")), escapeHTML(duration.Round(time.Millisecond).String()))
		}
		html.WriteString("</ul>")
	}

	html.WriteString("</div>")
	return html.String()
}

func resolveDNSTimings(ctx context.Context, hostname string) map[string]time.Duration {
	timings := make(map[string]time.Duration, len(resolveDNSServers))
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, server := range resolveDNSServers {
		server := server
		wg.Add(1)
		go func() {
			defer wg.Done()
			resolver := &net.Resolver{
				PreferGo: true,
				Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
					return (&net.Dialer{Timeout: 3 * time.Second}).DialContext(ctx, network, server)
				},
			}
			start := time.Now()
			_, err := resolver.LookupIPAddr(ctx, hostname)
			if err == nil {
				mu.Lock()
				timings[server] = time.Since(start)
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	return timings
}

// CertHandler handles cert:{domain} queries
type CertHandler struct {
	ctClient *http.Client
}

// NewCertHandler creates a new certificate handler
func NewCertHandler() *CertHandler {
	return &CertHandler{ctClient: newPublicHTTPClient(3 * time.Second)}
}

func (h *CertHandler) Type() AnswerType {
	return AnswerTypeCert
}

func (h *CertHandler) HandleDirectQuery(ctx context.Context, term string) (*Answer, error) {
	term = strings.TrimSpace(term)
	if term == "" {
		return nil, fmt.Errorf("domain name required")
	}

	lang := LangFromContext(ctx)

	hostname, port, err := parseCertificateTarget(term)
	if err != nil {
		return nil, err
	}

	address := net.JoinHostPort(hostname, strconv.Itoa(port))
	connectionContext, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	connection, err := publicNetworkDialOverride(connectionContext, "tcp", address)
	if err != nil {
		return &Answer{
			Type:        AnswerTypeCert,
			Term:        term,
			Title:       fmt.Sprintf("Certificate: %s", term),
			Description: i18n.T(lang, "direct.failed_to_connect"),
			Content:     fmt.Sprintf("<p>Unable to connect to <code>%s</code>: %s</p>", escapeHTML(term), escapeHTML(err.Error())),
			Error:       "connection_failed",
		}, nil
	}
	conn := tls.Client(connection, &tls.Config{
		InsecureSkipVerify: true,
		ServerName:         hostname,
		MinVersion:         tls.VersionTLS12,
	})
	if err = conn.HandshakeContext(connectionContext); err != nil {
		closeutil.Close(connection)
		return &Answer{
			Type:        AnswerTypeCert,
			Term:        term,
			Title:       fmt.Sprintf("Certificate: %s", term),
			Description: i18n.T(lang, "direct.failed_to_connect"),
			Content:     fmt.Sprintf("<p>Unable to connect to <code>%s</code>: %s</p>", escapeHTML(term), escapeHTML(err.Error())),
			Error:       "connection_failed",
		}, nil
	}
	defer closeutil.Close(conn)

	certs := conn.ConnectionState().PeerCertificates
	if len(certs) == 0 {
		return &Answer{
			Type:        AnswerTypeCert,
			Term:        term,
			Title:       fmt.Sprintf("Certificate: %s", term),
			Description: i18n.T(lang, "direct.no_certificate_found"),
			Content:     "<p>No SSL/TLS certificate found.</p>",
			Error:       "no_cert",
		}, nil
	}

	cert := certs[0]
	data := map[string]interface{}{
		"subject":   cert.Subject.String(),
		"issuer":    cert.Issuer.String(),
		"notBefore": cert.NotBefore.Format(time.RFC3339),
		"notAfter":  cert.NotAfter.Format(time.RFC3339),
		"sans":      cert.DNSNames,
		"serial":    cert.SerialNumber.String(),
		"version":   cert.Version,
		"keyAlgo":   cert.PublicKeyAlgorithm.String(),
		"keySize":   certificateKeySize(cert),
		"sigAlgo":   cert.SignatureAlgorithm.String(),
		"isCA":      cert.IsCA,
		"chainLen":  len(certs),
	}

	// Validate the presented leaf against the requested host and system trust roots.
	now := time.Now()
	dateValid := !now.Before(cert.NotBefore) && now.Before(cert.NotAfter)
	intermediates := x509.NewCertPool()
	for _, intermediate := range certs[1:] {
		intermediates.AddCert(intermediate)
	}
	verifiedChains, verifyErr := cert.Verify(x509.VerifyOptions{DNSName: hostname, CurrentTime: now, Intermediates: intermediates})
	hostnameValid := cert.VerifyHostname(hostname) == nil
	chainTrusted := verifyErr == nil && len(verifiedChains) > 0
	valid := dateValid && hostnameValid && chainTrusted
	daysUntilExpiry := int(cert.NotAfter.Sub(now).Hours() / 24)
	data["valid"] = valid
	data["dateValid"] = dateValid
	data["hostnameValid"] = hostnameValid
	data["chainTrusted"] = chainTrusted
	data["verificationError"] = ""
	if verifyErr != nil {
		data["verificationError"] = verifyErr.Error()
	}
	chainCertificates := certs
	chainOrder := "presented"
	if chainTrusted {
		chainCertificates = verifiedChains[0]
		chainOrder = "verified"
	}
	chain := make([]map[string]string, 0, len(chainCertificates))
	for index := len(chainCertificates) - 1; index >= 0; index-- {
		item := chainCertificates[index]
		chain = append(chain, map[string]string{"subject": item.Subject.String(), "issuer": item.Issuer.String()})
	}
	data["chain"] = chain
	data["chainOrder"] = chainOrder
	sctCount := 0
	for _, extension := range cert.Extensions {
		if extension.Id.Equal([]int{1, 3, 6, 1, 4, 1, 11129, 2, 4, 2}) {
			sctCount++
		}
	}
	data["embeddedSCTs"] = sctCount
	ctEntries, ctAvailable := h.lookupCertificateTransparency(connectionContext, hostname)
	data["ctEntries"] = ctEntries
	data["ctAvailable"] = ctAvailable
	data["rating"] = certificateRating(valid, cert)
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

type certificateTransparencyEntry struct {
	ID         int64  `json:"id"`
	CommonName string `json:"common_name"`
	NameValue  string `json:"name_value"`
	Issuer     string `json:"issuer_name"`
	NotBefore  string `json:"not_before"`
	NotAfter   string `json:"not_after"`
}

func (h *CertHandler) lookupCertificateTransparency(ctx context.Context, hostname string) ([]certificateTransparencyEntry, bool) {
	client := h.ctClient
	if client == nil {
		client = newPublicHTTPClient(3 * time.Second)
	}
	clientCopy := *client
	clientCopy.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	requestURL := "https://crt.sh/?q=" + url.QueryEscape(hostname) + "&output=json"
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, false
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", version.BrowserUserAgent)
	response, err := clientCopy.Do(request)
	if err != nil {
		return nil, false
	}
	defer closeutil.Close(response.Body)
	if response.StatusCode != http.StatusOK {
		return nil, false
	}
	var entries []certificateTransparencyEntry
	if err := decodeBoundedJSON(response.Body, 1<<20, &entries); err != nil {
		return nil, false
	}
	matching := make([]certificateTransparencyEntry, 0, min(len(entries), 10))
	for _, entry := range entries {
		matchesHost := false
		for _, name := range strings.FieldsFunc(entry.NameValue, func(r rune) bool { return r == '\n' || r == '\r' }) {
			name = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(name)), "*.")
			if name == strings.ToLower(hostname) {
				matchesHost = true
				break
			}
		}
		if !matchesHost || entry.ID <= 0 {
			continue
		}
		matching = append(matching, entry)
		if len(matching) == 10 {
			break
		}
	}
	return matching, true
}

func certificateRating(valid bool, cert *x509.Certificate) string {
	if !valid || cert.PublicKeyAlgorithm == x509.DSA || cert.SignatureAlgorithm == x509.MD2WithRSA || cert.SignatureAlgorithm == x509.MD5WithRSA {
		return "F"
	}
	keySize := certificateKeySize(cert)
	if cert.PublicKeyAlgorithm == x509.RSA && keySize < 2048 {
		return "F"
	}
	if keySize < 224 || cert.SignatureAlgorithm == x509.SHA1WithRSA || cert.SignatureAlgorithm == x509.ECDSAWithSHA1 {
		return "C"
	}
	return "A"
}

func certificateKeySize(cert *x509.Certificate) int {
	switch key := cert.PublicKey.(type) {
	case *rsa.PublicKey:
		return key.N.BitLen()
	case *ecdsa.PublicKey:
		return key.Curve.Params().BitSize
	case ed25519.PublicKey:
		return len(key) * 8
	default:
		return 0
	}
}

func parseCertificateTarget(target string) (string, int, error) {
	target = strings.TrimSpace(target)
	if target == "" || strings.Contains(target, "@") || strings.ContainsAny(target, "/?#") {
		return "", 0, fmt.Errorf("invalid certificate host")
	}
	hostname := strings.Trim(target, "[]")
	port := 443
	if host, portText, err := net.SplitHostPort(target); err == nil {
		hostname = host
		parsedPort, parseErr := strconv.Atoi(portText)
		if parseErr != nil || parsedPort < 1 || parsedPort > 65535 {
			return "", 0, fmt.Errorf("invalid certificate port")
		}
		port = parsedPort
	} else if strings.Count(target, ":") == 1 {
		host, portText, found := strings.Cut(target, ":")
		parsedPort, parseErr := strconv.Atoi(portText)
		if !found || parseErr != nil || parsedPort < 1 || parsedPort > 65535 {
			return "", 0, fmt.Errorf("invalid certificate host or port")
		}
		hostname, port = host, parsedPort
	}
	if hostname == "" {
		return "", 0, fmt.Errorf("invalid certificate host")
	}
	return hostname, port, nil
}

func formatCertData(domain string, data map[string]interface{}) string {
	var html strings.Builder
	html.WriteString("<div class=\"cert-data\">")
	fmt.Fprintf(&html, "<h2>Certificate for %s</h2>", escapeHTML(domain))

	// Validity status
	valid := data["valid"].(bool)
	days := data["daysUntilExpiry"].(int)
	if valid {
		if days < 30 {
			fmt.Fprintf(&html, "<p class=\"status warning\">Valid (expires in %d days)</p>", days)
		} else {
			fmt.Fprintf(&html, "<p class=\"status valid\">Valid (%d days remaining)</p>", days)
		}
	} else {
		html.WriteString("<p class=\"status invalid\">Not fully valid (date, hostname, or trust check failed)</p>")
	}
	for _, check := range []struct{ name, key string }{{"Date validity", "dateValid"}, {"Hostname match", "hostnameValid"}, {"Trusted chain", "chainTrusted"}} {
		fmt.Fprintf(&html, "<p>%s: %t</p>", check.name, data[check.key])
	}
	rating, _ := data["rating"].(string)
	if rating == "" {
		rating = "Unrated"
	}
	fmt.Fprintf(&html, "<p>Security rating: %s (local certificate checks)</p><p>Embedded CT SCT extensions: %v</p>", escapeHTML(rating), data["embeddedSCTs"])
	if available, ok := data["ctAvailable"].(bool); ok && available {
		html.WriteString("<h3>Certificate transparency records (crt.sh)</h3>")
		entries, _ := data["ctEntries"].([]certificateTransparencyEntry)
		if len(entries) == 0 {
			html.WriteString("<p>No matching certificate records were returned.</p>")
		} else {
			html.WriteString("<ul>")
			for _, entry := range entries {
				fmt.Fprintf(&html, "<li><a href=\"https://crt.sh/?id=%d\" rel=\"noopener noreferrer\">Record %d</a>: %s; issuer %s; %s to %s</li>", entry.ID, entry.ID, escapeHTML(entry.CommonName), escapeHTML(entry.Issuer), escapeHTML(entry.NotBefore), escapeHTML(entry.NotAfter))
			}
			html.WriteString("</ul>")
		}
	} else if _, ok := data["ctAvailable"]; ok {
		html.WriteString("<p>Certificate transparency records are temporarily unavailable.</p>")
	}
	if chain, ok := data["chain"].([]map[string]string); ok {
		chainOrder, _ := data["chainOrder"].(string)
		if chainOrder == "verified" {
			html.WriteString("<h3>Verified certificate chain (root to leaf)</h3><ol>")
		} else {
			html.WriteString("<h3>Presented certificate chain (rootward to leaf; trust not established)</h3><ol>")
		}
		for _, item := range chain {
			fmt.Fprintf(&html, "<li><code>%s</code> issued by <code>%s</code></li>", escapeHTML(item["subject"]), escapeHTML(item["issuer"]))
		}
		html.WriteString("</ol>")
	}

	html.WriteString("<dl class=\"cert-details\">")
	fmt.Fprintf(&html, "<dt>Subject</dt><dd><code>%s</code></dd>", escapeHTML(data["subject"].(string)))
	fmt.Fprintf(&html, "<dt>Issuer</dt><dd><code>%s</code></dd>", escapeHTML(data["issuer"].(string)))
	fmt.Fprintf(&html, "<dt>Valid From</dt><dd>%s</dd>", escapeHTML(data["notBefore"].(string)))
	fmt.Fprintf(&html, "<dt>Valid Until</dt><dd>%s</dd>", escapeHTML(data["notAfter"].(string)))

	if sans, ok := data["sans"].([]string); ok && len(sans) > 0 {
		html.WriteString("<dt>Subject Alternative Names</dt><dd><ul>")
		for _, san := range sans {
			fmt.Fprintf(&html, "<li><code>%s</code></li>", escapeHTML(san))
		}
		html.WriteString("</ul></dd>")
	}

	fmt.Fprintf(&html, "<dt>Key Algorithm</dt><dd>%s</dd>", escapeHTML(data["keyAlgo"].(string)))
	fmt.Fprintf(&html, "<dt>Key Size</dt><dd>%v bits</dd>", data["keySize"])
	fmt.Fprintf(&html, "<dt>Signature Algorithm</dt><dd>%s</dd>", escapeHTML(data["sigAlgo"].(string)))
	fmt.Fprintf(&html, "<dt>Chain Length</dt><dd>%d certificates</dd>", data["chainLen"].(int))

	html.WriteString("</dl></div>")
	return html.String()
}

// HeadersHandler handles headers:{url} queries
type HeadersHandler struct {
	client *http.Client
}

// NewHeadersHandler creates a new HTTP headers handler
func NewHeadersHandler() *HeadersHandler {
	client := newPublicHTTPClient(15 * time.Second)
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	return &HeadersHandler{client: client}
}

func (h *HeadersHandler) Type() AnswerType {
	return AnswerTypeHeaders
}

func (h *HeadersHandler) HandleDirectQuery(ctx context.Context, term string) (*Answer, error) {
	term = strings.TrimSpace(term)
	if term == "" {
		return nil, fmt.Errorf("URL required")
	}

	lang := LangFromContext(ctx)

	// Add https:// if no scheme
	targetURL := term
	if !strings.HasPrefix(strings.ToLower(term), "http://") && !strings.HasPrefix(strings.ToLower(term), "https://") {
		targetURL = "https://" + term
	}
	if _, err := validatePublicHTTPURL(targetURL); err != nil {
		return nil, err
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
			Description: i18n.T(lang, "direct.request_failed"),
			Content:     fmt.Sprintf("<p>Unable to fetch headers from <code>%s</code>: %s</p>", escapeHTML(targetURL), escapeHTML(err.Error())),
			Error:       "request_failed",
		}, nil
	}
	defer closeutil.Close(resp.Body)

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
	html.WriteString("<h2>HTTP Headers</h2>")
	fmt.Fprintf(&html, "<p><code>%s</code></p>", escapeHTML(url))
	statusText, _ := data["statusText"].(string)
	fmt.Fprintf(&html, "<p>Status: <strong>%s</strong></p>", escapeHTML(statusText))

	// Security grade
	if sec, ok := data["security"].(map[string]interface{}); ok {
		grade, _ := sec["grade"].(string)
		present, _ := sec["present"].(int)
		total, _ := sec["total"].(int)
		fmt.Fprintf(&html, "<p>Security Grade: <span class=\"grade grade-%s\">%s</span> (%d/%d headers present)</p>",
			strings.ToLower(grade), grade, present, total)
	}

	// All headers
	html.WriteString("<h3>Response Headers</h3>")
	html.WriteString("<table class=\"headers-table\">")
	html.WriteString("<thead><tr><th>Header</th><th>Value</th></tr></thead>")
	html.WriteString("<tbody>")

	if headers, ok := data["headers"].(map[string][]string); ok {
		for key, values := range headers {
			for _, val := range values {
				fmt.Fprintf(&html, "<tr><td><code>%s</code></td><td><code>%s</code></td></tr>",
					escapeHTML(key), escapeHTML(val))
			}
		}
	}

	html.WriteString("</tbody></table></div>")
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

func (h *SubnetHandler) HandleDirectQuery(ctx context.Context, term string) (*Answer, error) {
	term = strings.TrimSpace(term)
	if term == "" {
		return nil, fmt.Errorf("CIDR notation required (e.g., 192.168.1.0/24)")
	}

	lang := LangFromContext(ctx)

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
				Description: i18n.T(lang, "direct.invalid_cidr_notation"),
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
	fmt.Fprintf(&html, "<h2>%s</h2>", escapeHTML(data["cidr"].(string)))

	html.WriteString("<table class=\"subnet-table\">")
	html.WriteString("<tbody>")
	fmt.Fprintf(&html, "<tr><td>Network Address</td><td><code>%s</code></td></tr>", escapeHTML(data["network"].(string)))
	fmt.Fprintf(&html, "<tr><td>Broadcast Address</td><td><code>%s</code></td></tr>", escapeHTML(data["broadcast"].(string)))
	fmt.Fprintf(&html, "<tr><td>First Usable</td><td><code>%s</code></td></tr>", escapeHTML(data["firstUsable"].(string)))
	fmt.Fprintf(&html, "<tr><td>Last Usable</td><td><code>%s</code></td></tr>", escapeHTML(data["lastUsable"].(string)))
	fmt.Fprintf(&html, "<tr><td>Subnet Mask</td><td><code>%s</code></td></tr>", escapeHTML(data["subnetMask"].(string)))
	fmt.Fprintf(&html, "<tr><td>Wildcard Mask</td><td><code>%s</code></td></tr>", escapeHTML(data["wildcardMask"].(string)))
	fmt.Fprintf(&html, "<tr><td>Prefix Length</td><td><code>/%d</code></td></tr>", data["prefixLength"].(int))
	fmt.Fprintf(&html, "<tr><td>Total Hosts</td><td>%s</td></tr>", data["totalHosts"].(string))
	fmt.Fprintf(&html, "<tr><td>Usable Hosts</td><td>%s</td></tr>", data["usableHosts"].(string))
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
							fmt.Fprintf(&html, "<span class=\"network-bit\">%d</span>", (octet>>bit)&1)
						} else {
							fmt.Fprintf(&html, "<span class=\"host-bit\">%d</span>", (octet>>bit)&1)
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
