package direct

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/apimgr/search/src/version"
)

// SlangHandler handles slang:{term} queries
// Uses Urban Dictionary API for definitions
type SlangHandler struct {
	client *http.Client
}

// NewSlangHandler creates a new slang handler
func NewSlangHandler() *SlangHandler {
	return &SlangHandler{
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

func (h *SlangHandler) Type() AnswerType {
	return AnswerTypeSlang
}

// urbanDictionaryResponse represents the Urban Dictionary API response
type urbanDictionaryResponse struct {
	List []urbanDictionaryEntry `json:"list"`
}

type urbanDictionaryEntry struct {
	Definition  string `json:"definition"`
	Permalink   string `json:"permalink"`
	ThumbsUp    int    `json:"thumbs_up"`
	ThumbsDown  int    `json:"thumbs_down"`
	Author      string `json:"author"`
	Word        string `json:"word"`
	DefID       int    `json:"defid"`
	CurrentVote string `json:"current_vote"`
	WrittenOn   string `json:"written_on"`
	Example     string `json:"example"`
}

func (h *SlangHandler) Handle(ctx context.Context, term string) (*Answer, error) {
	term = strings.TrimSpace(term)
	if term == "" {
		return nil, fmt.Errorf("slang term required")
	}

	// Fetch from Urban Dictionary API
	apiURL := fmt.Sprintf("https://api.urbandictionary.com/v0/define?term=%s", url.QueryEscape(term))

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", version.BrowserUserAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := h.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch slang definition: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return &Answer{
			Type:        AnswerTypeSlang,
			Term:        term,
			Title:       fmt.Sprintf("slang: %s", term),
			Description: "Failed to fetch definition",
			Content:     fmt.Sprintf("<p>Failed to fetch definition for <code>%s</code>. Status: %d</p>", escapeHTML(term), resp.StatusCode),
			Error:       "fetch_error",
		}, nil
	}

	var data urbanDictionaryResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if len(data.List) == 0 {
		return &Answer{
			Type:        AnswerTypeSlang,
			Term:        term,
			Title:       fmt.Sprintf("slang: %s", term),
			Description: "No definition found",
			Content:     fmt.Sprintf("<p>No slang definition found for <code>%s</code>.</p><p>Try a different spelling or check if it's a newer term.</p>", escapeHTML(term)),
			Error:       "not_found",
		}, nil
	}

	// Sort by thumbs up (popularity)
	sort.Slice(data.List, func(i, j int) bool {
		return data.List[i].ThumbsUp > data.List[j].ThumbsUp
	})

	// Take top 5 definitions
	maxDefs := 5
	if len(data.List) < maxDefs {
		maxDefs = len(data.List)
	}
	topDefs := data.List[:maxDefs]

	// Build HTML content
	htmlContent := h.formatSlangHTML(term, topDefs)

	// Prepare data for JSON response
	definitions := make([]map[string]interface{}, len(topDefs))
	for i, def := range topDefs {
		definitions[i] = map[string]interface{}{
			"definition":  def.Definition,
			"example":     def.Example,
			"thumbs_up":   def.ThumbsUp,
			"thumbs_down": def.ThumbsDown,
			"author":      def.Author,
			"written_on":  def.WrittenOn,
			"permalink":   def.Permalink,
		}
	}

	return &Answer{
		Type:        AnswerTypeSlang,
		Term:        term,
		Title:       fmt.Sprintf("slang: %s", topDefs[0].Word),
		Description: truncateText(cleanBrackets(topDefs[0].Definition), 150),
		Content:     htmlContent,
		Source:      "Urban Dictionary",
		SourceURL:   fmt.Sprintf("https://www.urbandictionary.com/define.php?term=%s", url.QueryEscape(term)),
		Data: map[string]interface{}{
			"word":            topDefs[0].Word,
			"definition_count": len(data.List),
			"definitions":     definitions,
		},
	}, nil
}

// formatSlangHTML formats slang definitions as HTML
func (h *SlangHandler) formatSlangHTML(term string, defs []urbanDictionaryEntry) string {
	var html strings.Builder

	html.WriteString("<div class=\"slang-content\">")

	// Header with word
	if len(defs) > 0 {
		html.WriteString(fmt.Sprintf("<h1>%s</h1>", escapeHTML(defs[0].Word)))
	}

	// Definitions
	for i, def := range defs {
		html.WriteString("<div class=\"slang-definition\">")

		// Definition number and votes
		html.WriteString(fmt.Sprintf("<div class=\"def-header\">"))
		html.WriteString(fmt.Sprintf("<span class=\"def-number\">#%d</span>", i+1))
		html.WriteString(fmt.Sprintf("<span class=\"def-votes\">👍 %d | 👎 %d</span>", def.ThumbsUp, def.ThumbsDown))
		html.WriteString("</div>")

		// Definition text (Urban Dictionary uses [brackets] for links)
		definition := formatUrbanDictionaryText(def.Definition)
		html.WriteString(fmt.Sprintf("<p class=\"definition\">%s</p>", definition))

		// Example if present
		if def.Example != "" {
			example := formatUrbanDictionaryText(def.Example)
			html.WriteString(fmt.Sprintf("<blockquote class=\"example\">%s</blockquote>", example))
		}

		// Metadata
		html.WriteString("<div class=\"def-meta\">")
		html.WriteString(fmt.Sprintf("<span class=\"author\">by %s</span>", escapeHTML(def.Author)))
		if def.WrittenOn != "" {
			// Parse and format date
			if t, err := time.Parse("2006-01-02T15:04:05.000Z", def.WrittenOn); err == nil {
				html.WriteString(fmt.Sprintf("<span class=\"date\">%s</span>", t.Format("Jan 2, 2006")))
			}
		}
		html.WriteString("</div>")

		html.WriteString("</div>")
	}

	html.WriteString("</div>")
	return html.String()
}

// formatUrbanDictionaryText formats Urban Dictionary text
// Urban Dictionary uses [word] to link to other definitions
func formatUrbanDictionaryText(text string) string {
	// Escape HTML first
	text = escapeHTML(text)

	// Convert [word] to links
	// Simple approach: replace [word] with linked version
	result := strings.Builder{}
	i := 0
	for i < len(text) {
		if text[i] == '[' {
			// Find closing bracket
			end := strings.Index(text[i:], "]")
			if end > 1 {
				word := text[i+1 : i+end]
				link := fmt.Sprintf("<a href=\"/search?q=slang:%s\" class=\"slang-link\">%s</a>",
					url.QueryEscape(word), word)
				result.WriteString(link)
				i = i + end + 1
				continue
			}
		}
		result.WriteByte(text[i])
		i++
	}

	// Convert newlines to <br>
	return strings.ReplaceAll(result.String(), "\n", "<br>")
}

// cleanBrackets removes Urban Dictionary bracket notation
func cleanBrackets(text string) string {
	result := strings.Builder{}
	inBracket := false
	for _, c := range text {
		if c == '[' {
			inBracket = true
			continue
		}
		if c == ']' {
			inBracket = false
			continue
		}
		if !inBracket || c != '[' && c != ']' {
			result.WriteRune(c)
		}
	}
	return result.String()
}

// truncateText truncates text to maxLen, adding ellipsis if needed
func truncateText(text string, maxLen int) string {
	text = strings.ReplaceAll(text, "\n", " ")
	text = strings.ReplaceAll(text, "\r", "")
	if len(text) <= maxLen {
		return text
	}
	return text[:maxLen-3] + "..."
}
