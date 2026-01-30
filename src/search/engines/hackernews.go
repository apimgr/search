package engines

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/apimgr/search/src/model"
	"github.com/apimgr/search/src/search"
)

// HackerNews implements Hacker News search engine using the Algolia API
type HackerNews struct {
	*search.BaseEngine
	client *http.Client
}

// NewHackerNews creates a new Hacker News search engine
func NewHackerNews() *HackerNews {
	config := model.NewEngineConfig("hackernews")
	config.DisplayName = "Hacker News"
	config.Priority = 48
	config.Categories = []string{"general", "news"}
	config.SupportsTor = true

	return &HackerNews{
		BaseEngine: search.NewBaseEngine(config),
		client: &http.Client{
			Timeout: time.Duration(config.GetTimeout()) * time.Second,
		},
	}
}

// Search performs a Hacker News search using the Algolia API
func (e *HackerNews) Search(ctx context.Context, query *model.Query) ([]model.Result, error) {
	// HN Algolia API
	searchURL := "https://hn.algolia.com/api/v1/search"

	params := url.Values{}
	params.Set("query", query.Text)
	params.Set("tags", "story")
	params.Set("hitsPerPage", "10")

	reqURL := fmt.Sprintf("%s?%s", searchURL, params.Encode())

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Hacker News API returned status %d", resp.StatusCode)
	}

	var data struct {
		Hits []struct {
			Title       string `json:"title"`
			URL         string `json:"url"`
			Author      string `json:"author"`
			Points      int    `json:"points"`
			NumComments int    `json:"num_comments"`
			CreatedAt   string `json:"created_at"`
			ObjectID    string `json:"objectID"`
			StoryText   string `json:"story_text"`
		} `json:"hits"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	results := make([]model.Result, 0)

	for i, item := range data.Hits {
		if i >= e.GetConfig().GetMaxResults() {
			break
		}

		// Build the URL - use original URL if available, otherwise link to HN
		resultURL := item.URL
		if resultURL == "" {
			// For Ask HN, Show HN, etc. that don't have external URLs
			resultURL = fmt.Sprintf("https://news.ycombinator.com/item?id=%s", item.ObjectID)
		}

		// Build content with points and comments
		content := fmt.Sprintf("by %s | %d points | %d comments",
			item.Author, item.Points, item.NumComments)

		// Add a snippet of story text if available (for Ask HN, Show HN, etc.)
		if item.StoryText != "" && len(item.StoryText) > 0 {
			snippet := item.StoryText
			if len(snippet) > 150 {
				snippet = snippet[:150] + "..."
			}
			content += " | " + snippet
		}

		// Parse the published date
		var publishedAt time.Time
		if item.CreatedAt != "" {
			if parsed, err := time.Parse(time.RFC3339, item.CreatedAt); err == nil {
				publishedAt = parsed
			}
		}

		results = append(results, model.Result{
			Title:       item.Title,
			URL:         resultURL,
			Content:     content,
			Engine:      e.Name(),
			Category:    model.CategoryGeneral,
			Author:      item.Author,
			PublishedAt: publishedAt,
			Score:       calculateScore(e.GetPriority(), i, 1),
			Position:    i,
		})
	}

	return results, nil
}
