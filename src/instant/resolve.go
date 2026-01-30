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

// AnswerTypeResolve is the answer type for DNS resolution
const AnswerTypeResolve AnswerType = "resolve"

// DNSRecord represents a DNS record
type DNSRecord struct {
	Type  string `json:"type"`
	Value string `json:"value"`
	TTL   int    `json:"ttl,omitempty"`
}

// ResolveHandler handles DNS resolution queries
type ResolveHandler struct {
	resolver *net.Resolver
	patterns []*regexp.Regexp
}

// NewResolveHandler creates a new DNS resolution handler
func NewResolveHandler() *ResolveHandler {
	return &ResolveHandler{
		resolver: &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
				d := net.Dialer{Timeout: 5 * time.Second}
				return d.DialContext(ctx, network, address)
			},
		},
		patterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)^resolve[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^dns[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^nslookup[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^dig[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^lookup[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^host[:\s]+(.+)$`),
		},
	}
}

func (h *ResolveHandler) Name() string {
	return "resolve"
}

func (h *ResolveHandler) Patterns() []*regexp.Regexp {
	return h.patterns
}

func (h *ResolveHandler) CanHandle(query string) bool {
	for _, p := range h.patterns {
		if p.MatchString(query) {
			return true
		}
	}
	return false
}

func (h *ResolveHandler) Handle(ctx context.Context, query string) (*Answer, error) {
	// Extract hostname from query
	hostname := ""
	for _, p := range h.patterns {
		if matches := p.FindStringSubmatch(query); len(matches) > 1 {
			hostname = strings.TrimSpace(matches[1])
			break
		}
	}

	if hostname == "" {
		return nil, nil
	}

	// Clean up hostname
	hostname = strings.TrimPrefix(hostname, "https://")
	hostname = strings.TrimPrefix(hostname, "http://")
	hostname = strings.TrimSuffix(hostname, "/")
	if idx := strings.Index(hostname, "/"); idx != -1 {
		hostname = hostname[:idx]
	}
	// Remove port if present
	if idx := strings.LastIndex(hostname, ":"); idx != -1 {
		hostname = hostname[:idx]
	}

	// Create context with timeout
	resolveCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Collect all DNS records
	records := make([]DNSRecord, 0)
	var errors []string

	// A records (IPv4)
	ips, err := h.resolver.LookupIP(resolveCtx, "ip4", hostname)
	if err != nil {
		errors = append(errors, fmt.Sprintf("A: %v", err))
	} else {
		for _, ip := range ips {
			records = append(records, DNSRecord{Type: "A", Value: ip.String()})
		}
	}

	// AAAA records (IPv6)
	ips, err = h.resolver.LookupIP(resolveCtx, "ip6", hostname)
	if err != nil {
		errors = append(errors, fmt.Sprintf("AAAA: %v", err))
	} else {
		for _, ip := range ips {
			records = append(records, DNSRecord{Type: "AAAA", Value: ip.String()})
		}
	}

	// CNAME record
	cname, err := h.resolver.LookupCNAME(resolveCtx, hostname)
	if err == nil && cname != "" && cname != hostname+"." {
		records = append(records, DNSRecord{Type: "CNAME", Value: strings.TrimSuffix(cname, ".")})
	}

	// MX records
	mxRecords, err := h.resolver.LookupMX(resolveCtx, hostname)
	if err != nil {
		errors = append(errors, fmt.Sprintf("MX: %v", err))
	} else {
		for _, mx := range mxRecords {
			records = append(records, DNSRecord{
				Type:  "MX",
				Value: fmt.Sprintf("%d %s", mx.Pref, strings.TrimSuffix(mx.Host, ".")),
			})
		}
	}

	// TXT records
	txtRecords, err := h.resolver.LookupTXT(resolveCtx, hostname)
	if err != nil {
		errors = append(errors, fmt.Sprintf("TXT: %v", err))
	} else {
		for _, txt := range txtRecords {
			records = append(records, DNSRecord{Type: "TXT", Value: txt})
		}
	}

	// NS records
	nsRecords, err := h.resolver.LookupNS(resolveCtx, hostname)
	if err != nil {
		errors = append(errors, fmt.Sprintf("NS: %v", err))
	} else {
		for _, ns := range nsRecords {
			records = append(records, DNSRecord{Type: "NS", Value: strings.TrimSuffix(ns.Host, ".")})
		}
	}

	// Check if we got any records
	if len(records) == 0 {
		return &Answer{
			Type:    AnswerTypeResolve,
			Query:   query,
			Title:   fmt.Sprintf("DNS Lookup: %s", hostname),
			Content: fmt.Sprintf("<strong>Error:</strong> No DNS records found for %s<br><br>Errors:<br>%s",
				escapeHTML(hostname), escapeHTML(strings.Join(errors, "<br>"))),
			Data: map[string]interface{}{
				"hostname": hostname,
				"errors":   errors,
			},
		}, nil
	}

	// Build content
	var content strings.Builder
	content.WriteString("<div class=\"resolve-result\">")
	content.WriteString(fmt.Sprintf("<strong>Hostname:</strong> %s<br><br>", escapeHTML(hostname)))

	// Group records by type
	recordsByType := make(map[string][]DNSRecord)
	for _, record := range records {
		recordsByType[record.Type] = append(recordsByType[record.Type], record)
	}

	// Sort types for consistent display
	var types []string
	for t := range recordsByType {
		types = append(types, t)
	}
	sort.Strings(types)

	// Display records by type
	for _, recordType := range types {
		recs := recordsByType[recordType]
		content.WriteString(fmt.Sprintf("<strong>%s Records (%d):</strong><br>", recordType, len(recs)))
		for _, rec := range recs {
			value := rec.Value
			// Truncate long TXT records
			if recordType == "TXT" && len(value) > 100 {
				value = value[:100] + "..."
			}
			content.WriteString(fmt.Sprintf("&nbsp;&nbsp;<code>%s</code><br>", escapeHTML(value)))
		}
		content.WriteString("<br>")
	}

	// Reverse DNS lookup for the first A record
	var reverseHostname string
	for _, rec := range records {
		if rec.Type == "A" {
			names, err := h.resolver.LookupAddr(resolveCtx, rec.Value)
			if err == nil && len(names) > 0 {
				reverseHostname = strings.TrimSuffix(names[0], ".")
				content.WriteString(fmt.Sprintf("<strong>Reverse DNS (PTR):</strong><br>&nbsp;&nbsp;%s -> %s<br><br>",
					escapeHTML(rec.Value), escapeHTML(reverseHostname)))
			}
			break
		}
	}

	// Summary
	content.WriteString("<strong>Summary:</strong><br>")
	for _, t := range types {
		content.WriteString(fmt.Sprintf("&nbsp;&nbsp;%s: %d record(s)<br>", t, len(recordsByType[t])))
	}

	content.WriteString("</div>")

	data := map[string]interface{}{
		"hostname":         hostname,
		"records":          records,
		"record_count":     len(records),
		"reverse_hostname": reverseHostname,
	}

	return &Answer{
		Type:    AnswerTypeResolve,
		Query:   query,
		Title:   fmt.Sprintf("DNS Lookup: %s", hostname),
		Content: content.String(),
		Data:    data,
	}, nil
}
