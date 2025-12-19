package engines

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/apimgr/search/src/models"
	"github.com/apimgr/search/src/search"
)

// Reddit implements Reddit search engine
type Reddit struct {
	*search.BaseEngine
	client *http.Client
}

// NewReddit creates a new Reddit search engine
func NewReddit() *Reddit {
	config := models.NewEngineConfig("reddit")
	config.DisplayName = "Reddit"
	config.Priority = 45
	config.Categories = []string{"general", "social"}
	config.SupportsTor = true

	return &Reddit{
		BaseEngine: search.NewBaseEngine(config),
		client: &http.Client{
			Timeout: time.Duration(config.GetTimeout()) * time.Second,
		},
	}
}

// Search performs a Reddit search
func (e *Reddit) Search(ctx context.Context, query *models.Query) ([]models.Result, error) {
	// Reddit JSON API
	searchURL := "https://www.reddit.com/search.json"

	params := url.Values{}
	params.Set("q", query.Text)
	params.Set("sort", "relevance")
	params.Set("limit", "10")
	params.Set("type", "link")

	reqURL := fmt.Sprintf("%s?%s", searchURL, params.Encode())

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36 Edg/131.0.0.0")
	req.Header.Set("Accept", "application/json")

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Reddit API returned status %d", resp.StatusCode)
	}

	var data struct {
		Data struct {
			Children []struct {
				Data struct {
					Title       string  `json:"title"`
					Permalink   string  `json:"permalink"`
					Selftext    string  `json:"selftext"`
					Subreddit   string  `json:"subreddit"`
					Score       int     `json:"score"`
					NumComments int     `json:"num_comments"`
					URL         string  `json:"url"`
					CreatedUTC  float64 `json:"created_utc"`
					IsSelf      bool    `json:"is_self"`
				} `json:"data"`
			} `json:"children"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	results := make([]models.Result, 0)

	for i, child := range data.Data.Children {
		if i >= e.GetConfig().GetMaxResults() {
			break
		}

		item := child.Data

		// Build the full URL
		postURL := fmt.Sprintf("https://www.reddit.com%s", item.Permalink)

		// Build content
		content := fmt.Sprintf("r/%s | ⬆ %d | 💬 %d",
			item.Subreddit, item.Score, item.NumComments)

		// Add a snippet of selftext if available
		if item.Selftext != "" && len(item.Selftext) > 0 {
			snippet := item.Selftext
			if len(snippet) > 150 {
				snippet = snippet[:150] + "..."
			}
			content += " | " + snippet
		}

		results = append(results, models.Result{
			Title:    item.Title,
			URL:      postURL,
			Content:  content,
			Engine:   e.Name(),
			Category: models.CategoryGeneral,
			Score:    calculateScore(e.GetPriority(), i, 1),
			Position: i,
		})
	}

	return results, nil
}
