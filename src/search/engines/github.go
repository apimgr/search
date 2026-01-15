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

// GitHub implements GitHub search engine
type GitHub struct {
	*search.BaseEngine
	client *http.Client
}

// NewGitHub creates a new GitHub search engine
func NewGitHub() *GitHub {
	config := model.NewEngineConfig("github")
	config.DisplayName = "GitHub"
	config.Priority = 50
	config.Categories = []string{"general", "code"}
	config.SupportsTor = true

	return &GitHub{
		BaseEngine: search.NewBaseEngine(config),
		client: &http.Client{
			Timeout: time.Duration(config.GetTimeout()) * time.Second,
		},
	}
}

// Search performs a GitHub search
func (e *GitHub) Search(ctx context.Context, query *model.Query) ([]model.Result, error) {
	// GitHub Search API
	searchURL := "https://api.github.com/search/repositories"

	params := url.Values{}
	params.Set("q", query.Text)
	params.Set("sort", "stars")
	params.Set("order", "desc")
	params.Set("per_page", "10")

	reqURL := fmt.Sprintf("%s?%s", searchURL, params.Encode())

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36 Edg/131.0.0.0")
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	var data struct {
		Items []struct {
			FullName    string `json:"full_name"`
			HTMLURL     string `json:"html_url"`
			Description string `json:"description"`
			Stars       int    `json:"stargazers_count"`
			Forks       int    `json:"forks_count"`
			Language    string `json:"language"`
			UpdatedAt   string `json:"updated_at"`
		} `json:"items"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	results := make([]model.Result, 0)

	for i, item := range data.Items {
		if i >= e.GetConfig().GetMaxResults() {
			break
		}

		content := item.Description
		if item.Language != "" {
			content = fmt.Sprintf("[%s] %s", item.Language, content)
		}
		if item.Stars > 0 {
			content = fmt.Sprintf("%s (★ %d)", content, item.Stars)
		}

		results = append(results, model.Result{
			Title:    item.FullName,
			URL:      item.HTMLURL,
			Content:  content,
			Engine:   e.Name(),
			Category: model.CategoryGeneral,
			Score:    calculateScore(e.GetPriority(), i, 1),
			Position: i,
		})
	}

	return results, nil
}
