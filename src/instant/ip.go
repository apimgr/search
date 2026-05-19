package instant

import (
	"context"
	"fmt"
	"net"
	"regexp"
	"strings"
)

// IPHandler handles IP address lookups
type IPHandler struct {
	patterns []*regexp.Regexp
}

func NewIPHandler() *IPHandler {
	return &IPHandler{
		patterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)^my\s+ip\s*$`),
			regexp.MustCompile(`(?i)^what\s+is\s+my\s+ip\s*\??$`),
			regexp.MustCompile(`(?i)^ip\s+address\s*$`),
			regexp.MustCompile(`(?i)^ip\s+info\s*$`),
		},
	}
}

func (h *IPHandler) Name() string               { return "ip" }
func (h *IPHandler) Patterns() []*regexp.Regexp { return h.patterns }

func (h *IPHandler) CanHandle(query string) bool {
	for _, p := range h.patterns {
		if p.MatchString(query) {
			return true
		}
	}
	return false
}

func (h *IPHandler) HandleInstantQuery(ctx context.Context, query string) (*Answer, error) {
	// Get local IPs
	var ips []string
	addrs, err := net.InterfaceAddrs()
	if err == nil {
		for _, addr := range addrs {
			if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
				if ipnet.IP.To4() != nil {
					ips = append(ips, ipnet.IP.String())
				}
			}
		}
	}

	var content strings.Builder
	content.WriteString("<strong>Local IP Addresses:</strong><br>")
	if len(ips) > 0 {
		for _, ip := range ips {
			content.WriteString(fmt.Sprintf("• %s<br>", ip))
		}
	} else {
		content.WriteString("Unable to determine local IP<br>")
	}
	content.WriteString("<br><em>Note: Your public IP is visible to websites you visit</em>")

	return &Answer{
		Type:    AnswerTypeIP,
		Query:   query,
		Title:   "IP Address Information",
		Content: content.String(),
		Data: map[string]interface{}{
			"local_ips": ips,
		},
	}, nil
}
