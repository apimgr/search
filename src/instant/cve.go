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

// AnswerTypeCVE is the answer type for CVE lookups
const AnswerTypeCVE AnswerType = "cve"

// CVEHandler handles CVE (Common Vulnerabilities and Exposures) lookups
type CVEHandler struct {
	client   *http.Client
	patterns []*regexp.Regexp
}

// NewCVEHandler creates a new CVE handler
func NewCVEHandler() *CVEHandler {
	return &CVEHandler{
		client: &http.Client{Timeout: 15 * time.Second},
		patterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)^cve[:\s-]+(CVE-)?(\d{4})[:\s-]+(\d{4,})$`),
			regexp.MustCompile(`(?i)^cve[:\s-]+(CVE-)?(\d{4})-(\d{4,})$`),
			regexp.MustCompile(`(?i)^(CVE-\d{4}-\d{4,})$`),
		},
	}
}

func (h *CVEHandler) Name() string              { return "cve" }
func (h *CVEHandler) Patterns() []*regexp.Regexp { return h.patterns }

func (h *CVEHandler) CanHandle(query string) bool {
	for _, p := range h.patterns {
		if p.MatchString(query) {
			return true
		}
	}
	return false
}

func (h *CVEHandler) Handle(ctx context.Context, query string) (*Answer, error) {
	// Extract CVE ID from query
	cveID := h.extractCVEID(query)
	if cveID == "" {
		return nil, nil
	}

	// Normalize CVE ID format (CVE-YYYY-NNNNN)
	cveID = strings.ToUpper(cveID)
	if !strings.HasPrefix(cveID, "CVE-") {
		cveID = "CVE-" + cveID
	}

	// Query NVD API
	apiURL := fmt.Sprintf("https://services.nvd.nist.gov/rest/json/cves/2.0?cveId=%s", cveID)

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", version.BrowserUserAgent)

	resp, err := h.client.Do(req)
	if err != nil {
		return h.errorAnswer(query, cveID, "Failed to connect to NVD API"), nil
	}
	defer resp.Body.Close()

	// Handle rate limiting
	if resp.StatusCode == http.StatusTooManyRequests {
		return h.errorAnswer(query, cveID, "NVD API rate limit exceeded. Please try again later."), nil
	}

	if resp.StatusCode != http.StatusOK {
		return h.errorAnswer(query, cveID, fmt.Sprintf("NVD API returned status %d", resp.StatusCode)), nil
	}

	// Parse response
	var nvdResp NVDResponse
	if err := json.NewDecoder(resp.Body).Decode(&nvdResp); err != nil {
		return h.errorAnswer(query, cveID, "Failed to parse NVD response"), nil
	}

	if len(nvdResp.Vulnerabilities) == 0 {
		return h.errorAnswer(query, cveID, "CVE not found in NVD database"), nil
	}

	cve := nvdResp.Vulnerabilities[0].CVE

	// Build content
	var content strings.Builder
	content.WriteString(fmt.Sprintf("<strong>%s</strong><br><br>", cve.ID))

	// Description
	if len(cve.Descriptions) > 0 {
		for _, desc := range cve.Descriptions {
			if desc.Lang == "en" {
				content.WriteString(fmt.Sprintf("<strong>Description:</strong><br>%s<br><br>", escapeHTML(desc.Value)))
				break
			}
		}
	}

	// CVSS Score
	if cve.Metrics.CVSSMetricV31 != nil && len(cve.Metrics.CVSSMetricV31) > 0 {
		cvss := cve.Metrics.CVSSMetricV31[0]
		severity := cvss.CVSSData.BaseSeverity
		score := cvss.CVSSData.BaseScore
		severityClass := getSeverityClass(severity)
		content.WriteString(fmt.Sprintf("<strong>CVSS 3.1 Score:</strong> <span class=\"%s\">%.1f (%s)</span><br>", severityClass, score, severity))
		content.WriteString(fmt.Sprintf("<strong>Vector:</strong> <code>%s</code><br><br>", cvss.CVSSData.VectorString))
	} else if cve.Metrics.CVSSMetricV2 != nil && len(cve.Metrics.CVSSMetricV2) > 0 {
		cvss := cve.Metrics.CVSSMetricV2[0]
		severity := cvss.BaseSeverity
		score := cvss.CVSSData.BaseScore
		severityClass := getSeverityClass(severity)
		content.WriteString(fmt.Sprintf("<strong>CVSS 2.0 Score:</strong> <span class=\"%s\">%.1f (%s)</span><br>", severityClass, score, severity))
		content.WriteString(fmt.Sprintf("<strong>Vector:</strong> <code>%s</code><br><br>", cvss.CVSSData.VectorString))
	}

	// Dates
	content.WriteString(fmt.Sprintf("<strong>Published:</strong> %s<br>", formatCVEDate(cve.Published)))
	content.WriteString(fmt.Sprintf("<strong>Last Modified:</strong> %s<br><br>", formatCVEDate(cve.LastModified)))

	// References (first 5)
	if len(cve.References) > 0 {
		content.WriteString("<strong>References:</strong><br>")
		maxRefs := 5
		if len(cve.References) < maxRefs {
			maxRefs = len(cve.References)
		}
		for i := 0; i < maxRefs; i++ {
			ref := cve.References[i]
			content.WriteString(fmt.Sprintf("&bull; <a href=\"%s\" target=\"_blank\">%s</a><br>", ref.URL, truncateString(ref.URL, 60)))
		}
		if len(cve.References) > 5 {
			content.WriteString(fmt.Sprintf("&bull; ... and %d more<br>", len(cve.References)-5))
		}
	}

	return &Answer{
		Type:      AnswerTypeCVE,
		Query:     query,
		Title:     fmt.Sprintf("CVE Details: %s", cve.ID),
		Content:   content.String(),
		Source:    "National Vulnerability Database (NVD)",
		SourceURL: fmt.Sprintf("https://nvd.nist.gov/vuln/detail/%s", cve.ID),
		Data: map[string]interface{}{
			"cve_id":      cve.ID,
			"published":   cve.Published,
			"modified":    cve.LastModified,
			"description": getEnglishDescription(cve.Descriptions),
		},
	}, nil
}

func (h *CVEHandler) extractCVEID(query string) string {
	// Try to extract CVE ID in various formats
	for _, p := range h.patterns {
		if matches := p.FindStringSubmatch(query); len(matches) > 0 {
			// Handle full CVE-YYYY-NNNNN format
			if strings.HasPrefix(strings.ToUpper(matches[0]), "CVE-") {
				return matches[0]
			}
			// Handle year and number separately
			if len(matches) >= 4 {
				return fmt.Sprintf("%s-%s", matches[2], matches[3])
			}
			if len(matches) >= 3 {
				return fmt.Sprintf("%s-%s", matches[2], matches[3])
			}
		}
	}
	return ""
}

func (h *CVEHandler) errorAnswer(query, cveID, message string) *Answer {
	return &Answer{
		Type:      AnswerTypeCVE,
		Query:     query,
		Title:     fmt.Sprintf("CVE Lookup: %s", cveID),
		Content:   fmt.Sprintf("<span class=\"error\">%s</span>", message),
		Source:    "National Vulnerability Database (NVD)",
		SourceURL: "https://nvd.nist.gov/",
	}
}

// NVD API response structures
type NVDResponse struct {
	ResultsPerPage  int `json:"resultsPerPage"`
	StartIndex      int `json:"startIndex"`
	TotalResults    int `json:"totalResults"`
	Vulnerabilities []struct {
		CVE CVEItem `json:"cve"`
	} `json:"vulnerabilities"`
}

type CVEItem struct {
	ID           string `json:"id"`
	Published    string `json:"published"`
	LastModified string `json:"lastModified"`
	Descriptions []struct {
		Lang  string `json:"lang"`
		Value string `json:"value"`
	} `json:"descriptions"`
	Metrics struct {
		CVSSMetricV31 []struct {
			CVSSData struct {
				Version      string  `json:"version"`
				VectorString string  `json:"vectorString"`
				BaseScore    float64 `json:"baseScore"`
				BaseSeverity string  `json:"baseSeverity"`
			} `json:"cvssData"`
		} `json:"cvssMetricV31"`
		CVSSMetricV2 []struct {
			CVSSData struct {
				Version      string  `json:"version"`
				VectorString string  `json:"vectorString"`
				BaseScore    float64 `json:"baseScore"`
			} `json:"cvssData"`
			BaseSeverity string `json:"baseSeverity"`
		} `json:"cvssMetricV2"`
	} `json:"metrics"`
	References []struct {
		URL    string   `json:"url"`
		Source string   `json:"source"`
		Tags   []string `json:"tags"`
	} `json:"references"`
}

func getSeverityClass(severity string) string {
	switch strings.ToUpper(severity) {
	case "CRITICAL":
		return "severity-critical"
	case "HIGH":
		return "severity-high"
	case "MEDIUM":
		return "severity-medium"
	case "LOW":
		return "severity-low"
	default:
		return "severity-unknown"
	}
}

func formatCVEDate(dateStr string) string {
	t, err := time.Parse(time.RFC3339, dateStr)
	if err != nil {
		return dateStr
	}
	return t.Format("January 2, 2006")
}

func getEnglishDescription(descriptions []struct {
	Lang  string `json:"lang"`
	Value string `json:"value"`
}) string {
	for _, desc := range descriptions {
		if desc.Lang == "en" {
			return desc.Value
		}
	}
	if len(descriptions) > 0 {
		return descriptions[0].Value
	}
	return ""
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
