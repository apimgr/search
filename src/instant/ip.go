package instant

import (
	"context"
	"fmt"
	"net"
	"regexp"
	"strings"

	"github.com/apimgr/search/src/geoip"
)

// IPHandler handles IP address lookups — both "what is my ip" queries and
// specific IP lookups ("ip 1.2.3.4" or bare "8.8.8.8").
// When a *geoip.Lookup is present in the context (via WithGeoIPLookup) the
// response is enriched with country, city, region, timezone, and ASN data.
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
			// bare "ip" or "ip:" with no address — shows client's IP
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
		return h.lookupSpecificIP(ctx, query, m[1])
	}
	if m := h.bareIPPattern.FindStringSubmatch(query); len(m) == 2 {
		return h.lookupSpecificIP(ctx, query, m[1])
	}
	return h.handleMyIP(ctx, query)
}

// handleMyIP returns the client's public IP address as seen by the server,
// enriched with GeoIP data when the MMDB lookup service is available.
func (h *IPHandler) handleMyIP(ctx context.Context, query string) (*Answer, error) {
	rawClientIP := ClientIPFromContext(ctx)

	// Parse through net.ParseIP so only a well-formed IP literal reaches HTML.
	// This strips any header-injection attempts before they reach the template.
	var safeIP string
	var parsed net.IP
	if rawClientIP != "" {
		if parsed = net.ParseIP(rawClientIP); parsed != nil {
			// .String() always returns a canonical IP literal — safe to interpolate.
			safeIP = parsed.String()
		}
	}

	data := map[string]interface{}{"ip": safeIP}

	var content strings.Builder
	if safeIP == "" {
		content.WriteString("<em>Unable to determine your IP address</em>")
		return &Answer{
			Type:    AnswerTypeIP,
			Query:   query,
			Title:   "Your IP Address",
			Content: content.String(),
			Data:    data,
		}, nil
	}

	content.WriteString(fmt.Sprintf("<strong>Your IP:</strong> <code>%s</code><br>", safeIP))
	content.WriteString(fmt.Sprintf("<strong>Version:</strong> %s<br>", ipVersion(parsed)))
	content.WriteString(fmt.Sprintf("<strong>Type:</strong> %s<br>", ipClassification(parsed)))

	// Enrich with GeoIP data when available — fail-open, never block on lookup error.
	if lookup := GeoIPLookupFromContext(ctx); lookup != nil && parsed.IsGlobalUnicast() {
		if geo := lookup.Lookup(safeIP); geo != nil && geo.Found {
			appendGeoFields(&content, data, geo)
		}
	}

	return &Answer{
		Type:    AnswerTypeIP,
		Query:   query,
		Title:   "Your IP Address",
		Content: content.String(),
		Data:    data,
	}, nil
}

// lookupSpecificIP returns classification and GeoIP information for an explicit IP.
func (h *IPHandler) lookupSpecificIP(ctx context.Context, query, rawIP string) (*Answer, error) {
	ip := net.ParseIP(rawIP)
	if ip == nil {
		return &Answer{
			Type:  AnswerTypeIP,
			Query: query,
			Title: "IP Address",
			// rawIP comes from a regex that only matches digit-and-dot sequences — safe to display.
			Content: fmt.Sprintf("<strong>%s</strong> is not a valid IP address", rawIP),
			Data:    map[string]interface{}{"ip": rawIP, "valid": false},
		}, nil
	}

	safeIP := ip.String()
	class := ipClassification(ip)
	data := map[string]interface{}{
		"ip":    safeIP,
		"valid": true,
		"type":  class,
	}

	var content strings.Builder
	content.WriteString(fmt.Sprintf("<strong>IP:</strong> <code>%s</code><br>", safeIP))
	content.WriteString(fmt.Sprintf("<strong>Version:</strong> %s<br>", ipVersion(ip)))
	content.WriteString(fmt.Sprintf("<strong>Type:</strong> %s<br>", class))

	// Enrich with GeoIP data for public unicast addresses only.
	if lookup := GeoIPLookupFromContext(ctx); lookup != nil && ip.IsGlobalUnicast() {
		if geo := lookup.Lookup(safeIP); geo != nil && geo.Found {
			appendGeoFields(&content, data, geo)
		}
	}

	return &Answer{
		Type:    AnswerTypeIP,
		Query:   query,
		Title:   "IP Address: " + safeIP,
		Content: content.String(),
		Data:    data,
	}, nil
}

// appendGeoFields writes non-empty GeoIP result fields into content and data.
// Callers must check geo.Found before calling — this function trusts that invariant.
func appendGeoFields(content *strings.Builder, data map[string]interface{}, geo *geoip.Result) {
	if geo.CountryCode != "" {
		line := geo.CountryCode
		if geo.CountryName != "" {
			line = fmt.Sprintf("%s (%s)", geo.CountryName, geo.CountryCode)
		}
		content.WriteString(fmt.Sprintf("<strong>Country:</strong> %s<br>", line))
		data["country_code"] = geo.CountryCode
		data["country_name"] = geo.CountryName
	}

	if geo.Continent != "" {
		data["continent"] = geo.Continent
	}

	if geo.City != "" {
		if geo.Region != "" {
			content.WriteString(fmt.Sprintf("<strong>City:</strong> %s, %s<br>", geo.City, geo.Region))
		} else {
			content.WriteString(fmt.Sprintf("<strong>City:</strong> %s<br>", geo.City))
		}
		data["city"] = geo.City
		data["region"] = geo.Region
	} else if geo.Region != "" {
		content.WriteString(fmt.Sprintf("<strong>Region:</strong> %s<br>", geo.Region))
		data["region"] = geo.Region
	}

	if geo.PostalCode != "" {
		data["postal_code"] = geo.PostalCode
	}

	if geo.Timezone != "" {
		content.WriteString(fmt.Sprintf("<strong>Timezone:</strong> %s<br>", geo.Timezone))
		data["timezone"] = geo.Timezone
	}

	if geo.Latitude != 0 {
		data["latitude"] = geo.Latitude
		data["longitude"] = geo.Longitude
	}

	if geo.ASN != 0 {
		if geo.ASNOrg != "" {
			content.WriteString(fmt.Sprintf("<strong>ASN:</strong> AS%d (%s)<br>", geo.ASN, geo.ASNOrg))
		} else {
			content.WriteString(fmt.Sprintf("<strong>ASN:</strong> AS%d<br>", geo.ASN))
		}
		data["asn"] = geo.ASN
		data["asn_org"] = geo.ASNOrg
	}

	if geo.RegistrantOrg != "" {
		data["registrant_org"] = geo.RegistrantOrg
	}
	if geo.RegistrantNet != "" {
		data["registrant_net"] = geo.RegistrantNet
	}
}

// ipVersion returns "IPv4" or "IPv6" for an already-parsed address.
func ipVersion(ip net.IP) string {
	if ip.To4() != nil {
		return "IPv4"
	}
	return "IPv6"
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
