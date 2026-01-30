package instant

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/apimgr/search/src/version"
)

// AnswerTypeRFC is the answer type for RFC lookups
const AnswerTypeRFC AnswerType = "rfc"

// RFCHandler handles RFC (Request for Comments) document lookups
type RFCHandler struct {
	client   *http.Client
	patterns []*regexp.Regexp
}

// NewRFCHandler creates a new RFC handler
func NewRFCHandler() *RFCHandler {
	return &RFCHandler{
		client: &http.Client{Timeout: 15 * time.Second},
		patterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)^rfc[:\s-]*(\d+)$`),
			regexp.MustCompile(`(?i)^rfc(\d+)$`),
		},
	}
}

func (h *RFCHandler) Name() string              { return "rfc" }
func (h *RFCHandler) Patterns() []*regexp.Regexp { return h.patterns }

func (h *RFCHandler) CanHandle(query string) bool {
	for _, p := range h.patterns {
		if p.MatchString(query) {
			return true
		}
	}
	return false
}

func (h *RFCHandler) Handle(ctx context.Context, query string) (*Answer, error) {
	// Extract RFC number from query
	var rfcNum string
	for _, p := range h.patterns {
		if matches := p.FindStringSubmatch(query); len(matches) > 1 {
			rfcNum = matches[1]
			break
		}
	}

	if rfcNum == "" {
		return nil, nil
	}

	// Pad RFC number if needed (RFC 1 -> RFC 0001)
	rfcNumInt, err := strconv.Atoi(rfcNum)
	if err != nil {
		return h.errorAnswer(query, rfcNum, "Invalid RFC number"), nil
	}

	// Fetch RFC document from IETF
	rfcURL := fmt.Sprintf("https://www.rfc-editor.org/rfc/rfc%d.txt", rfcNumInt)

	req, err := http.NewRequestWithContext(ctx, "GET", rfcURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", version.BrowserUserAgent)

	resp, err := h.client.Do(req)
	if err != nil {
		return h.errorAnswer(query, rfcNum, "Failed to connect to RFC Editor"), nil
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return h.errorAnswer(query, rfcNum, fmt.Sprintf("RFC %s not found", rfcNum)), nil
	}

	if resp.StatusCode != http.StatusOK {
		return h.errorAnswer(query, rfcNum, fmt.Sprintf("RFC Editor returned status %d", resp.StatusCode)), nil
	}

	// Read and parse the RFC text (first few KB for metadata)
	limitedReader := io.LimitReader(resp.Body, 32*1024)
	content, err := io.ReadAll(limitedReader)
	if err != nil {
		return h.errorAnswer(query, rfcNum, "Failed to read RFC content"), nil
	}

	// Parse RFC metadata from header
	rfcInfo := h.parseRFCHeader(string(content))
	rfcInfo.Number = rfcNumInt

	// Build response content
	var htmlContent strings.Builder
	htmlContent.WriteString(fmt.Sprintf("<strong>RFC %d</strong><br><br>", rfcNumInt))

	if rfcInfo.Title != "" {
		htmlContent.WriteString(fmt.Sprintf("<strong>Title:</strong> %s<br><br>", escapeHTML(rfcInfo.Title)))
	}

	if len(rfcInfo.Authors) > 0 {
		htmlContent.WriteString(fmt.Sprintf("<strong>Author(s):</strong> %s<br>", escapeHTML(strings.Join(rfcInfo.Authors, ", "))))
	}

	if rfcInfo.Date != "" {
		htmlContent.WriteString(fmt.Sprintf("<strong>Date:</strong> %s<br>", escapeHTML(rfcInfo.Date)))
	}

	if rfcInfo.Status != "" {
		htmlContent.WriteString(fmt.Sprintf("<strong>Status:</strong> %s<br>", escapeHTML(rfcInfo.Status)))
	}

	if rfcInfo.Category != "" {
		htmlContent.WriteString(fmt.Sprintf("<strong>Category:</strong> %s<br>", escapeHTML(rfcInfo.Category)))
	}

	if len(rfcInfo.Obsoletes) > 0 {
		htmlContent.WriteString(fmt.Sprintf("<strong>Obsoletes:</strong> %s<br>", escapeHTML(strings.Join(rfcInfo.Obsoletes, ", "))))
	}

	if len(rfcInfo.Updates) > 0 {
		htmlContent.WriteString(fmt.Sprintf("<strong>Updates:</strong> %s<br>", escapeHTML(strings.Join(rfcInfo.Updates, ", "))))
	}

	htmlContent.WriteString("<br>")

	// Abstract (if found)
	if rfcInfo.Abstract != "" {
		abstractPreview := rfcInfo.Abstract
		if len(abstractPreview) > 500 {
			abstractPreview = abstractPreview[:500] + "..."
		}
		htmlContent.WriteString(fmt.Sprintf("<strong>Abstract:</strong><br>%s<br>", escapeHTML(abstractPreview)))
	}

	// Links
	htmlContent.WriteString("<br><strong>Links:</strong><br>")
	htmlContent.WriteString(fmt.Sprintf("&bull; <a href=\"%s\" target=\"_blank\">Plain Text</a><br>", rfcURL))
	htmlContent.WriteString(fmt.Sprintf("&bull; <a href=\"https://www.rfc-editor.org/info/rfc%d\" target=\"_blank\">RFC Info Page</a><br>", rfcNumInt))
	htmlContent.WriteString(fmt.Sprintf("&bull; <a href=\"https://datatracker.ietf.org/doc/rfc%d/\" target=\"_blank\">IETF Datatracker</a><br>", rfcNumInt))

	return &Answer{
		Type:      AnswerTypeRFC,
		Query:     query,
		Title:     fmt.Sprintf("RFC %d: %s", rfcNumInt, rfcInfo.Title),
		Content:   htmlContent.String(),
		Source:    "IETF RFC Editor",
		SourceURL: fmt.Sprintf("https://www.rfc-editor.org/info/rfc%d", rfcNumInt),
		Data: map[string]interface{}{
			"rfc_number": rfcNumInt,
			"title":      rfcInfo.Title,
			"authors":    rfcInfo.Authors,
			"date":       rfcInfo.Date,
			"status":     rfcInfo.Status,
		},
	}, nil
}

type RFCInfo struct {
	Number    int
	Title     string
	Authors   []string
	Date      string
	Status    string
	Category  string
	Obsoletes []string
	Updates   []string
	Abstract  string
}

func (h *RFCHandler) parseRFCHeader(content string) RFCInfo {
	info := RFCInfo{}

	scanner := bufio.NewScanner(strings.NewReader(content))
	var lines []string
	lineCount := 0

	// Read first ~100 lines for header info
	for scanner.Scan() && lineCount < 100 {
		lines = append(lines, scanner.Text())
		lineCount++
	}

	// Parse header lines
	inAbstract := false
	var abstractLines []string

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		lowerLine := strings.ToLower(trimmed)

		// Detect abstract section
		if lowerLine == "abstract" {
			inAbstract = true
			continue
		}

		// End abstract on next major section
		if inAbstract {
			if isRFCSectionHeader(trimmed) && trimmed != "" {
				inAbstract = false
			} else if trimmed != "" {
				abstractLines = append(abstractLines, trimmed)
				if len(abstractLines) > 10 {
					inAbstract = false
				}
			}
		}

		// Look for title (usually a centered, non-empty line after header info)
		if info.Title == "" && i > 5 && i < 30 {
			// Title lines are often centered and substantial
			if len(trimmed) > 10 && !strings.Contains(lowerLine, "request for comments") &&
				!strings.Contains(lowerLine, "category:") &&
				!strings.Contains(lowerLine, "obsoletes:") &&
				!strings.Contains(lowerLine, "updates:") &&
				!strings.HasPrefix(lowerLine, "rfc") &&
				!containsMonth(lowerLine) {
				// Check if it looks like a title
				if isLikelyTitle(trimmed, lines, i) {
					info.Title = trimmed
				}
			}
		}

		// Parse metadata lines
		if strings.HasPrefix(lowerLine, "category:") {
			info.Category = strings.TrimSpace(strings.TrimPrefix(trimmed, "Category:"))
			info.Category = strings.TrimSpace(strings.TrimPrefix(info.Category, "category:"))
		}

		if strings.HasPrefix(lowerLine, "obsoletes:") {
			info.Obsoletes = parseRFCList(strings.TrimPrefix(trimmed, "Obsoletes:"))
		}

		if strings.HasPrefix(lowerLine, "updates:") {
			info.Updates = parseRFCList(strings.TrimPrefix(trimmed, "Updates:"))
		}

		// Look for date (month year pattern)
		if info.Date == "" && containsMonth(lowerLine) {
			datePattern := regexp.MustCompile(`(?i)(January|February|March|April|May|June|July|August|September|October|November|December)\s+\d{4}`)
			if match := datePattern.FindString(trimmed); match != "" {
				info.Date = match
			}
		}

		// Look for status
		if strings.Contains(lowerLine, "status of this memo") {
			// Next non-empty line might have status info
			for j := i + 1; j < len(lines) && j < i+5; j++ {
				statusLine := strings.TrimSpace(lines[j])
				if statusLine != "" {
					if strings.Contains(strings.ToLower(statusLine), "standards track") {
						info.Status = "Standards Track"
					} else if strings.Contains(strings.ToLower(statusLine), "informational") {
						info.Status = "Informational"
					} else if strings.Contains(strings.ToLower(statusLine), "experimental") {
						info.Status = "Experimental"
					} else if strings.Contains(strings.ToLower(statusLine), "best current practice") {
						info.Status = "Best Current Practice"
					} else if strings.Contains(strings.ToLower(statusLine), "historic") {
						info.Status = "Historic"
					}
					if info.Status != "" {
						break
					}
				}
			}
		}
	}

	// Set abstract
	if len(abstractLines) > 0 {
		info.Abstract = strings.Join(abstractLines, " ")
	}

	return info
}

func (h *RFCHandler) errorAnswer(query, rfcNum, message string) *Answer {
	return &Answer{
		Type:      AnswerTypeRFC,
		Query:     query,
		Title:     fmt.Sprintf("RFC Lookup: %s", rfcNum),
		Content:   fmt.Sprintf("<span class=\"error\">%s</span>", message),
		Source:    "IETF RFC Editor",
		SourceURL: "https://www.rfc-editor.org/",
	}
}

func containsMonth(s string) bool {
	months := []string{"january", "february", "march", "april", "may", "june",
		"july", "august", "september", "october", "november", "december"}
	s = strings.ToLower(s)
	for _, m := range months {
		if strings.Contains(s, m) {
			return true
		}
	}
	return false
}

func parseRFCList(s string) []string {
	s = strings.TrimSpace(s)
	parts := strings.Split(s, ",")
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

func isRFCSectionHeader(line string) bool {
	// Common RFC section headers
	headers := []string{"abstract", "introduction", "table of contents",
		"status of this memo", "copyright notice", "terminology"}
	lower := strings.ToLower(strings.TrimSpace(line))
	for _, h := range headers {
		if lower == h {
			return true
		}
	}
	// Numbered section
	if matched, _ := regexp.MatchString(`^\d+\.?\s+\w+`, line); matched {
		return true
	}
	return false
}

func isLikelyTitle(line string, allLines []string, index int) bool {
	// Title is usually a substantial line that's centered or prominent
	// and followed by author names or dates
	if len(line) < 10 {
		return false
	}

	// Check if subsequent lines look like author names or dates
	if index+1 < len(allLines) {
		nextLine := strings.TrimSpace(allLines[index+1])
		// Empty line after title is common
		if nextLine == "" && index+2 < len(allLines) {
			nextLine = strings.TrimSpace(allLines[index+2])
		}
		// Look for author-like patterns or organization names
		if strings.Contains(nextLine, "@") || containsMonth(nextLine) {
			return true
		}
	}

	return false
}
