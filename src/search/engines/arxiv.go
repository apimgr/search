package engines

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/apimgr/search/src/model"
	"github.com/apimgr/search/src/search"
)

// ArXiv implements the arXiv search engine for scientific papers
type ArXiv struct {
	*search.BaseEngine
	client *http.Client
}

// NewArXiv creates a new arXiv search engine
func NewArXiv() *ArXiv {
	config := model.NewEngineConfig("arxiv")
	config.DisplayName = "arXiv"
	config.Priority = 60
	config.Categories = []string{"science", "general"}
	config.SupportsTor = true

	return &ArXiv{
		BaseEngine: search.NewBaseEngine(config),
		client: &http.Client{
			Timeout: time.Duration(config.GetTimeout()) * time.Second,
		},
	}
}

// arxivFeed represents the Atom feed response from arXiv API
type arxivFeed struct {
	XMLName xml.Name     `xml:"feed"`
	Entries []arxivEntry `xml:"entry"`
}

// arxivEntry represents a single entry in the arXiv Atom feed
type arxivEntry struct {
	ID        string        `xml:"id"`
	Title     string        `xml:"title"`
	Summary   string        `xml:"summary"`
	Published string        `xml:"published"`
	Updated   string        `xml:"updated"`
	Authors   []arxivAuthor `xml:"author"`
	Links     []arxivLink   `xml:"link"`
	Category  []struct {
		Term string `xml:"term,attr"`
	} `xml:"category"`
}

// arxivAuthor represents an author in the arXiv feed
type arxivAuthor struct {
	Name string `xml:"name"`
}

// arxivLink represents a link in the arXiv feed
type arxivLink struct {
	Href  string `xml:"href,attr"`
	Rel   string `xml:"rel,attr"`
	Type  string `xml:"type,attr"`
	Title string `xml:"title,attr"`
}

// Search performs an arXiv search
func (e *ArXiv) Search(ctx context.Context, query *model.Query) ([]model.Result, error) {
	// arXiv API endpoint
	searchURL := "http://export.arxiv.org/api/query"

	params := url.Values{}
	params.Set("search_query", fmt.Sprintf("all:%s", query.Text))
	params.Set("start", fmt.Sprintf("%d", (query.Page-1)*10))
	params.Set("max_results", "10")
	params.Set("sortBy", "relevance")
	params.Set("sortOrder", "descending")

	reqURL := fmt.Sprintf("%s?%s", searchURL, params.Encode())

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Accept", "application/atom+xml")

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("arXiv API returned status %d", resp.StatusCode)
	}

	var feed arxivFeed
	if err := xml.NewDecoder(resp.Body).Decode(&feed); err != nil {
		return nil, fmt.Errorf("failed to parse arXiv response: %w", err)
	}

	results := make([]model.Result, 0)

	for i, entry := range feed.Entries {
		if i >= e.GetConfig().GetMaxResults() {
			break
		}

		// Get the abstract page URL (prefer alternate link)
		resultURL := entry.ID
		for _, link := range entry.Links {
			if link.Rel == "alternate" && link.Type == "text/html" {
				resultURL = link.Href
				break
			}
		}

		// Extract author names
		authors := make([]string, 0, len(entry.Authors))
		for _, author := range entry.Authors {
			authors = append(authors, strings.TrimSpace(author.Name))
		}
		authorStr := ""
		if len(authors) > 0 {
			if len(authors) > 3 {
				authorStr = fmt.Sprintf("%s et al.", authors[0])
			} else {
				authorStr = strings.Join(authors, ", ")
			}
		}

		// Extract categories
		categories := make([]string, 0, len(entry.Category))
		for _, cat := range entry.Category {
			categories = append(categories, cat.Term)
		}
		categoryStr := ""
		if len(categories) > 0 {
			categoryStr = fmt.Sprintf("[%s]", strings.Join(categories, ", "))
		}

		// Build content with author and category info
		content := cleanArXivText(entry.Summary)
		if len(content) > 300 {
			content = content[:300] + "..."
		}
		if authorStr != "" {
			content = fmt.Sprintf("%s — %s", authorStr, content)
		}
		if categoryStr != "" {
			content = fmt.Sprintf("%s %s", categoryStr, content)
		}

		// Parse published date
		var publishedAt time.Time
		if entry.Published != "" {
			if t, err := time.Parse(time.RFC3339, entry.Published); err == nil {
				publishedAt = t
			}
		}

		results = append(results, model.Result{
			Title:       cleanArXivText(entry.Title),
			URL:         resultURL,
			Content:     content,
			Engine:      e.Name(),
			Category:    model.CategoryScience,
			Author:      authorStr,
			PublishedAt: publishedAt,
			Score:       calculateScore(e.GetPriority(), i, 1),
			Position:    i,
		})
	}

	return results, nil
}

// cleanArXivText cleans whitespace and newlines from arXiv text fields
func cleanArXivText(text string) string {
	// Replace newlines with spaces
	text = strings.ReplaceAll(text, "\n", " ")
	// Replace multiple spaces with single space
	for strings.Contains(text, "  ") {
		text = strings.ReplaceAll(text, "  ", " ")
	}
	// Trim whitespace
	text = strings.TrimSpace(text)
	return text
}
