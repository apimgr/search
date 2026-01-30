package instant

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/apimgr/search/src/version"
)

// ASNHandler handles Autonomous System Number lookups
type ASNHandler struct {
	client   *http.Client
	patterns []*regexp.Regexp
}

// NewASNHandler creates a new ASN handler
func NewASNHandler() *ASNHandler {
	return &ASNHandler{
		client: &http.Client{Timeout: 10 * time.Second},
		patterns: []*regexp.Regexp{
			// "asn:15169" or "asn:AS15169" or "asn: 15169"
			regexp.MustCompile(`(?i)^asn[:\s]+(?:AS)?(\d+)$`),
			// "AS15169" standalone
			regexp.MustCompile(`(?i)^AS(\d+)$`),
			// "asn lookup 15169"
			regexp.MustCompile(`(?i)^asn\s+lookup[:\s]+(?:AS)?(\d+)$`),
		},
	}
}

func (h *ASNHandler) Name() string {
	return "asn"
}

func (h *ASNHandler) Patterns() []*regexp.Regexp {
	return h.patterns
}

func (h *ASNHandler) CanHandle(query string) bool {
	for _, p := range h.patterns {
		if p.MatchString(query) {
			return true
		}
	}
	return false
}

// BGPView API response structures
type bgpViewASNResponse struct {
	Status        string `json:"status"`
	StatusMessage string `json:"status_message"`
	Data          struct {
		ASN              int    `json:"asn"`
		Name             string `json:"name"`
		DescriptionShort string `json:"description_short"`
		CountryCode      string `json:"country_code"`
		Website          string `json:"website"`
		EmailContacts    []string `json:"email_contacts"`
		AbuseContacts    []string `json:"abuse_contacts"`
		LookingGlass     string `json:"looking_glass"`
		TrafficEstimation string `json:"traffic_estimation"`
		TrafficRatio     string `json:"traffic_ratio"`
		OwnerAddress     []string `json:"owner_address"`
		RIRAllocation    struct {
			RIRName          string `json:"rir_name"`
			CountryCode      string `json:"country_code"`
			DateAllocated    string `json:"date_allocated"`
			AllocationStatus string `json:"allocation_status"`
		} `json:"rir_allocation"`
	} `json:"data"`
}

type bgpViewPrefixesResponse struct {
	Status string `json:"status"`
	Data   struct {
		IPv4Prefixes []struct {
			Prefix      string `json:"prefix"`
			IP          string `json:"ip"`
			CIDR        int    `json:"cidr"`
			Name        string `json:"name"`
			Description string `json:"description"`
			CountryCode string `json:"country_code"`
		} `json:"ipv4_prefixes"`
		IPv6Prefixes []struct {
			Prefix      string `json:"prefix"`
			IP          string `json:"ip"`
			CIDR        int    `json:"cidr"`
			Name        string `json:"name"`
			Description string `json:"description"`
			CountryCode string `json:"country_code"`
		} `json:"ipv6_prefixes"`
	} `json:"data"`
}

func (h *ASNHandler) Handle(ctx context.Context, query string) (*Answer, error) {
	// Extract ASN number from query
	var asnNumber string
	for _, p := range h.patterns {
		if matches := p.FindStringSubmatch(query); len(matches) > 1 {
			asnNumber = matches[1]
			break
		}
	}

	if asnNumber == "" {
		return nil, nil
	}

	// Fetch ASN info from BGPView API
	asnInfo, err := h.fetchASNInfo(ctx, asnNumber)
	if err != nil {
		return &Answer{
			Type:    AnswerTypeASN,
			Query:   query,
			Title:   fmt.Sprintf("ASN Lookup: AS%s", asnNumber),
			Content: fmt.Sprintf("Error fetching ASN information: %v", err),
		}, nil
	}

	if asnInfo.Status != "ok" {
		return &Answer{
			Type:    AnswerTypeASN,
			Query:   query,
			Title:   fmt.Sprintf("ASN Lookup: AS%s", asnNumber),
			Content: fmt.Sprintf("ASN not found or error: %s", asnInfo.StatusMessage),
		}, nil
	}

	// Fetch prefixes
	prefixes, _ := h.fetchASNPrefixes(ctx, asnNumber)

	// Build content
	var content strings.Builder
	data := asnInfo.Data

	content.WriteString(fmt.Sprintf("<div class=\"asn-result\">"))
	content.WriteString(fmt.Sprintf("<strong>AS%d</strong>", data.ASN))
	if data.Name != "" {
		content.WriteString(fmt.Sprintf(" - %s", data.Name))
	}
	content.WriteString("<br><br>")

	if data.DescriptionShort != "" {
		content.WriteString(fmt.Sprintf("<strong>Description:</strong> %s<br>", data.DescriptionShort))
	}

	if data.CountryCode != "" {
		content.WriteString(fmt.Sprintf("<strong>Country:</strong> %s<br>", data.CountryCode))
	}

	if data.Website != "" {
		content.WriteString(fmt.Sprintf("<strong>Website:</strong> <a href=\"%s\" target=\"_blank\">%s</a><br>", data.Website, data.Website))
	}

	if len(data.OwnerAddress) > 0 {
		content.WriteString("<strong>Organization:</strong><br>")
		for _, line := range data.OwnerAddress {
			if line != "" {
				content.WriteString(fmt.Sprintf("&nbsp;&nbsp;%s<br>", line))
			}
		}
	}

	if data.RIRAllocation.RIRName != "" {
		content.WriteString(fmt.Sprintf("<br><strong>RIR:</strong> %s", data.RIRAllocation.RIRName))
		if data.RIRAllocation.DateAllocated != "" {
			content.WriteString(fmt.Sprintf(" (allocated: %s)", data.RIRAllocation.DateAllocated))
		}
		content.WriteString("<br>")
	}

	// Add prefix information
	if prefixes != nil && prefixes.Status == "ok" {
		ipv4Count := len(prefixes.Data.IPv4Prefixes)
		ipv6Count := len(prefixes.Data.IPv6Prefixes)

		if ipv4Count > 0 || ipv6Count > 0 {
			content.WriteString("<br><strong>IP Prefixes:</strong><br>")
			content.WriteString(fmt.Sprintf("&nbsp;&nbsp;IPv4: %d prefixes<br>", ipv4Count))
			content.WriteString(fmt.Sprintf("&nbsp;&nbsp;IPv6: %d prefixes<br>", ipv6Count))

			// Show first few prefixes
			if ipv4Count > 0 {
				content.WriteString("<br><strong>Sample IPv4 Prefixes:</strong><br>")
				maxShow := 5
				if ipv4Count < maxShow {
					maxShow = ipv4Count
				}
				for i := 0; i < maxShow; i++ {
					p := prefixes.Data.IPv4Prefixes[i]
					content.WriteString(fmt.Sprintf("&nbsp;&nbsp;<code>%s</code>", p.Prefix))
					if p.Name != "" {
						content.WriteString(fmt.Sprintf(" (%s)", p.Name))
					}
					content.WriteString("<br>")
				}
				if ipv4Count > maxShow {
					content.WriteString(fmt.Sprintf("&nbsp;&nbsp;<em>... and %d more</em><br>", ipv4Count-maxShow))
				}
			}

			if ipv6Count > 0 {
				content.WriteString("<br><strong>Sample IPv6 Prefixes:</strong><br>")
				maxShow := 3
				if ipv6Count < maxShow {
					maxShow = ipv6Count
				}
				for i := 0; i < maxShow; i++ {
					p := prefixes.Data.IPv6Prefixes[i]
					content.WriteString(fmt.Sprintf("&nbsp;&nbsp;<code>%s</code>", p.Prefix))
					if p.Name != "" {
						content.WriteString(fmt.Sprintf(" (%s)", p.Name))
					}
					content.WriteString("<br>")
				}
				if ipv6Count > maxShow {
					content.WriteString(fmt.Sprintf("&nbsp;&nbsp;<em>... and %d more</em><br>", ipv6Count-maxShow))
				}
			}
		}
	}

	content.WriteString("</div>")

	// Build data map for structured response
	dataMap := map[string]interface{}{
		"asn":          data.ASN,
		"name":         data.Name,
		"description":  data.DescriptionShort,
		"country_code": data.CountryCode,
		"website":      data.Website,
	}

	if len(data.OwnerAddress) > 0 {
		dataMap["organization"] = data.OwnerAddress
	}

	if prefixes != nil && prefixes.Status == "ok" {
		dataMap["ipv4_prefix_count"] = len(prefixes.Data.IPv4Prefixes)
		dataMap["ipv6_prefix_count"] = len(prefixes.Data.IPv6Prefixes)
	}

	return &Answer{
		Type:      AnswerTypeASN,
		Query:     query,
		Title:     fmt.Sprintf("ASN Lookup: AS%d - %s", data.ASN, data.Name),
		Content:   content.String(),
		Source:    "BGPView",
		SourceURL: fmt.Sprintf("https://bgpview.io/asn/%s", asnNumber),
		Data:      dataMap,
	}, nil
}

func (h *ASNHandler) fetchASNInfo(ctx context.Context, asn string) (*bgpViewASNResponse, error) {
	apiURL := fmt.Sprintf("https://api.bgpview.io/asn/%s", asn)

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", version.BrowserUserAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := h.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	var result bgpViewASNResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return &result, nil
}

func (h *ASNHandler) fetchASNPrefixes(ctx context.Context, asn string) (*bgpViewPrefixesResponse, error) {
	apiURL := fmt.Sprintf("https://api.bgpview.io/asn/%s/prefixes", asn)

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", version.BrowserUserAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := h.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	var result bgpViewPrefixesResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return &result, nil
}
