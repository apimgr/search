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
	// myIPPatterns match queries about the user's own IP
	myIPPatterns []*regexp.Regexp
	// specificIPPattern matches queries about a specific IPv4 address
	specificIPPattern *regexp.Regexp
	// bareIPPattern matches a bare IPv4 address as the entire query
	bareIPPattern *regexp.Regexp
}

func NewIPHandler() *IPHandler {
	return &IPHandler{
		myIPPatterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)^my\s+ip\s*$`),
			regexp.MustCompile(`(?i)^my\s+ip\s+address\s*$`),
			regexp.MustCompile(`(?i)^what\s+is\s+my\s+ip\s*\??$`),
			regexp.MustCompile(`(?i)^ip\s+address\s*$`),
			regexp.MustCompile(`(?i)^ip\s+info\s*$`),
			// bare "ip" or "ip:" with no address — shows local interfaces
			regexp.MustCompile(`(?i)^ip[:\s]*$`),
		},
		// matches "ip 1.2.3.4" or "ip: 1.2.3.4" — looks up a specific address
		specificIPPattern: regexp.MustCompile(`(?i)^ip[:\s]+([\d]{1,3}\.[\d]{1,3}\.[\d]{1,3}\.[\d]{1,3})\s*$`),
		// matches a bare IPv4 address, e.g. "8.8.8.8"
		bareIPPattern: regexp.MustCompile(`^([\d]{1,3}\.[\d]{1,3}\.[\d]{1,3}\.[\d]{1,3})\s*$`),
	}
}

func (h *IPHandler) Name() string { return "ip" }

func (h *IPHandler) Patterns() []*regexp.Regexp {
	all := make([]*regexp.Regexp, 0, len(h.myIPPatterns)+2)
	all = append(all, h.myIPPatterns...)
	all = append(all, h.specificIPPattern, h.bareIPPattern)
	return all
}

func (h *IPHandler) CanHandle(query string) bool {
	for _, p := range h.myIPPatterns {
		if p.MatchString(query) {
			return true
		}
	}
	return h.specificIPPattern.MatchString(query) || h.bareIPPattern.MatchString(query)
}

func (h *IPHandler) HandleInstantQuery(ctx context.Context, query string) (*Answer, error) {
	// Check for specific IP lookup first ("ip 1.2.3.4" or bare "8.8.8.8")
	if m := h.specificIPPattern.FindStringSubmatch(query); len(m) == 2 {
		return h.lookupSpecificIP(query, m[1])
	}
	if m := h.bareIPPattern.FindStringSubmatch(query); len(m) == 2 {
		return h.lookupSpecificIP(query, m[1])
	}
	return h.handleMyIP(query)
}

// handleMyIP returns the server's local network interfaces.
func (h *IPHandler) handleMyIP(query string) (*Answer, error) {
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

// lookupSpecificIP returns classification and basic information for a given IP.
func (h *IPHandler) lookupSpecificIP(query, rawIP string) (*Answer, error) {
	ip := net.ParseIP(rawIP)
	if ip == nil {
		return &Answer{
			Type:    AnswerTypeIP,
			Query:   query,
			Title:   "IP Address",
			Content: fmt.Sprintf("<strong>%s</strong> is not a valid IP address", rawIP),
			Data:    map[string]interface{}{"ip": rawIP, "valid": false},
		}, nil
	}

	class := ipClassification(ip)

	var content strings.Builder
	content.WriteString(fmt.Sprintf("<strong>IP:</strong> %s<br>", ip.String()))
	content.WriteString(fmt.Sprintf("<strong>Type:</strong> %s<br>", class))

	return &Answer{
		Type:    AnswerTypeIP,
		Query:   query,
		Title:   "IP Address: " + ip.String(),
		Content: content.String(),
		Data: map[string]interface{}{
			"ip":    ip.String(),
			"valid": true,
			"type":  class,
		},
	}, nil
}

// ipClassification returns a human-readable classification for an IP address.
func ipClassification(ip net.IP) string {
	switch {
	case ip.IsLoopback():
		return "Loopback"
	case ip.IsLinkLocalUnicast():
		return "Link-local"
	case ip.IsPrivate():
		return "Private"
	case ip.IsMulticast():
		return "Multicast"
	default:
		return "Public"
	}
}
