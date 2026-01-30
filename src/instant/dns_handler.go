package instant

import (
	"context"
	"fmt"
	"net"
	"regexp"
	"sort"
	"strings"
	"time"
)

// DNSHandler handles DNS record lookups
type DNSHandler struct {
	patterns []*regexp.Regexp
	resolver *net.Resolver
}

// NewDNSHandler creates a new DNS lookup handler
func NewDNSHandler() *DNSHandler {
	return &DNSHandler{
		patterns: []*regexp.Regexp{
			// "dns:example.com" or "dns: example.com"
			regexp.MustCompile(`(?i)^dns[:\s]+([a-zA-Z0-9][-a-zA-Z0-9]*(?:\.[a-zA-Z0-9][-a-zA-Z0-9]*)+)(?:/([a-zA-Z]+))?$`),
			// "dns lookup example.com" or "dns lookup example.com/mx"
			regexp.MustCompile(`(?i)^dns\s+lookup[:\s]+([a-zA-Z0-9][-a-zA-Z0-9]*(?:\.[a-zA-Z0-9][-a-zA-Z0-9]*)+)(?:/([a-zA-Z]+))?$`),
			// "nslookup example.com"
			regexp.MustCompile(`(?i)^nslookup[:\s]+([a-zA-Z0-9][-a-zA-Z0-9]*(?:\.[a-zA-Z0-9][-a-zA-Z0-9]*)+)(?:/([a-zA-Z]+))?$`),
			// "dig example.com"
			regexp.MustCompile(`(?i)^dig[:\s]+([a-zA-Z0-9][-a-zA-Z0-9]*(?:\.[a-zA-Z0-9][-a-zA-Z0-9]*)+)(?:/([a-zA-Z]+))?$`),
		},
		resolver: &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
				d := net.Dialer{
					Timeout: 5 * time.Second,
				}
				return d.DialContext(ctx, network, address)
			},
		},
	}
}

func (h *DNSHandler) Name() string {
	return "dns"
}

func (h *DNSHandler) Patterns() []*regexp.Regexp {
	return h.patterns
}

func (h *DNSHandler) CanHandle(query string) bool {
	for _, p := range h.patterns {
		if p.MatchString(query) {
			return true
		}
	}
	return false
}

// DNS record types
type dnsRecords struct {
	A     []string
	AAAA  []string
	CNAME string
	MX    []mxRecord
	NS    []string
	TXT   []string
	SOA   *soaRecord
}

type mxRecord struct {
	Host     string
	Priority uint16
}

type soaRecord struct {
	NS      string
	Mbox    string
	Serial  uint32
	Refresh uint32
	Retry   uint32
	Expire  uint32
	Minimum uint32
}

func (h *DNSHandler) Handle(ctx context.Context, query string) (*Answer, error) {
	// Extract domain and optional record type from query
	var domain, recordType string
	for _, p := range h.patterns {
		if matches := p.FindStringSubmatch(query); len(matches) > 1 {
			domain = strings.ToLower(matches[1])
			if len(matches) > 2 && matches[2] != "" {
				recordType = strings.ToUpper(matches[2])
			}
			break
		}
	}

	if domain == "" {
		return nil, nil
	}

	// Create context with timeout
	lookupCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	// Lookup DNS records
	records := h.lookupAllRecords(lookupCtx, domain, recordType)

	// Build content
	var content strings.Builder
	content.WriteString(fmt.Sprintf("<div class=\"dns-result\">"))
	content.WriteString(fmt.Sprintf("<strong>DNS Records for:</strong> %s<br><br>", domain))

	hasRecords := false

	// Show specific record type or all records
	showAll := recordType == "" || recordType == "ALL"

	if showAll || recordType == "A" {
		if len(records.A) > 0 {
			hasRecords = true
			content.WriteString("<strong>A Records (IPv4):</strong><br>")
			for _, ip := range records.A {
				content.WriteString(fmt.Sprintf("&nbsp;&nbsp;<code>%s</code><br>", ip))
			}
			content.WriteString("<br>")
		}
	}

	if showAll || recordType == "AAAA" {
		if len(records.AAAA) > 0 {
			hasRecords = true
			content.WriteString("<strong>AAAA Records (IPv6):</strong><br>")
			for _, ip := range records.AAAA {
				content.WriteString(fmt.Sprintf("&nbsp;&nbsp;<code>%s</code><br>", ip))
			}
			content.WriteString("<br>")
		}
	}

	if showAll || recordType == "CNAME" {
		if records.CNAME != "" {
			hasRecords = true
			content.WriteString(fmt.Sprintf("<strong>CNAME Record:</strong><br>"))
			content.WriteString(fmt.Sprintf("&nbsp;&nbsp;<code>%s</code><br><br>", records.CNAME))
		}
	}

	if showAll || recordType == "MX" {
		if len(records.MX) > 0 {
			hasRecords = true
			content.WriteString("<strong>MX Records (Mail):</strong><br>")
			for _, mx := range records.MX {
				content.WriteString(fmt.Sprintf("&nbsp;&nbsp;<code>%d %s</code><br>", mx.Priority, mx.Host))
			}
			content.WriteString("<br>")
		}
	}

	if showAll || recordType == "NS" {
		if len(records.NS) > 0 {
			hasRecords = true
			content.WriteString("<strong>NS Records (Name Servers):</strong><br>")
			for _, ns := range records.NS {
				content.WriteString(fmt.Sprintf("&nbsp;&nbsp;<code>%s</code><br>", ns))
			}
			content.WriteString("<br>")
		}
	}

	if showAll || recordType == "TXT" {
		if len(records.TXT) > 0 {
			hasRecords = true
			content.WriteString("<strong>TXT Records:</strong><br>")
			for _, txt := range records.TXT {
				// Escape HTML in TXT records
				escaped := strings.ReplaceAll(txt, "<", "&lt;")
				escaped = strings.ReplaceAll(escaped, ">", "&gt;")
				// Truncate long TXT records for display
				if len(escaped) > 200 {
					escaped = escaped[:200] + "..."
				}
				content.WriteString(fmt.Sprintf("&nbsp;&nbsp;<code>%s</code><br>", escaped))
			}
			content.WriteString("<br>")
		}
	}

	if !hasRecords {
		if recordType != "" && recordType != "ALL" {
			content.WriteString(fmt.Sprintf("<em>No %s records found for this domain.</em><br>", recordType))
		} else {
			content.WriteString("<em>No DNS records found for this domain.</em><br>")
		}
	}

	content.WriteString("</div>")

	// Build data map
	dataMap := map[string]interface{}{
		"domain": domain,
	}

	if len(records.A) > 0 {
		dataMap["a_records"] = records.A
	}
	if len(records.AAAA) > 0 {
		dataMap["aaaa_records"] = records.AAAA
	}
	if records.CNAME != "" {
		dataMap["cname"] = records.CNAME
	}
	if len(records.MX) > 0 {
		mxList := make([]map[string]interface{}, len(records.MX))
		for i, mx := range records.MX {
			mxList[i] = map[string]interface{}{
				"host":     mx.Host,
				"priority": mx.Priority,
			}
		}
		dataMap["mx_records"] = mxList
	}
	if len(records.NS) > 0 {
		dataMap["ns_records"] = records.NS
	}
	if len(records.TXT) > 0 {
		dataMap["txt_records"] = records.TXT
	}

	title := fmt.Sprintf("DNS Lookup: %s", domain)
	if recordType != "" && recordType != "ALL" {
		title = fmt.Sprintf("DNS Lookup: %s (%s)", domain, recordType)
	}

	return &Answer{
		Type:    AnswerTypeDNS,
		Query:   query,
		Title:   title,
		Content: content.String(),
		Source:  "DNS",
		Data:    dataMap,
	}, nil
}

func (h *DNSHandler) lookupAllRecords(ctx context.Context, domain string, recordType string) *dnsRecords {
	records := &dnsRecords{
		A:    make([]string, 0),
		AAAA: make([]string, 0),
		MX:   make([]mxRecord, 0),
		NS:   make([]string, 0),
		TXT:  make([]string, 0),
	}

	showAll := recordType == "" || recordType == "ALL"

	// Lookup A and AAAA records
	if showAll || recordType == "A" || recordType == "AAAA" {
		ips, err := h.resolver.LookupIPAddr(ctx, domain)
		if err == nil {
			for _, ip := range ips {
				if ip.IP.To4() != nil {
					records.A = append(records.A, ip.IP.String())
				} else {
					records.AAAA = append(records.AAAA, ip.IP.String())
				}
			}
		}
	}

	// Lookup CNAME
	if showAll || recordType == "CNAME" {
		cname, err := h.resolver.LookupCNAME(ctx, domain)
		if err == nil && cname != "" && cname != domain+"." {
			records.CNAME = strings.TrimSuffix(cname, ".")
		}
	}

	// Lookup MX records
	if showAll || recordType == "MX" {
		mxRecords, err := h.resolver.LookupMX(ctx, domain)
		if err == nil {
			for _, mx := range mxRecords {
				records.MX = append(records.MX, mxRecord{
					Host:     strings.TrimSuffix(mx.Host, "."),
					Priority: mx.Pref,
				})
			}
			// Sort by priority
			sort.Slice(records.MX, func(i, j int) bool {
				return records.MX[i].Priority < records.MX[j].Priority
			})
		}
	}

	// Lookup NS records
	if showAll || recordType == "NS" {
		nsRecords, err := h.resolver.LookupNS(ctx, domain)
		if err == nil {
			for _, ns := range nsRecords {
				records.NS = append(records.NS, strings.TrimSuffix(ns.Host, "."))
			}
			sort.Strings(records.NS)
		}
	}

	// Lookup TXT records
	if showAll || recordType == "TXT" {
		txtRecords, err := h.resolver.LookupTXT(ctx, domain)
		if err == nil {
			records.TXT = txtRecords
			sort.Strings(records.TXT)
		}
	}

	return records
}
