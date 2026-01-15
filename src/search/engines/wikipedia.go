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

type WikipediaEngine struct {
	*search.BaseEngine
}

func NewWikipediaEngine() *WikipediaEngine {
	config := model.NewEngineConfig("wikipedia")
	config.DisplayName = "Wikipedia"
	config.Categories = []string{"general"}
	config.Priority = 70

	return &WikipediaEngine{
		BaseEngine: search.NewBaseEngine(config),
	}
}

type wikipediaResponse struct {
	Query struct {
		Search []struct {
			Title      string `json:"title"`
			PageID     int    `json:"pageid"`
			Snippet    string `json:"snippet"`
			Timestamp  string `json:"timestamp"`
		} `json:"search"`
	} `json:"query"`
}

func (e *WikipediaEngine) Search(ctx context.Context, query *model.Query) ([]model.Result, error) {
	offset := query.Page * 10
	searchURL := fmt.Sprintf("https://en.wikipedia.org/w/api.php?action=query&list=search&srsearch=%s&format=json&sroffset=%d&srlimit=10",
		url.QueryEscape(query.Text), offset)

	req, err := http.NewRequestWithContext(ctx, "GET", searchURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36 Edg/131.0.0.0")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("wikipedia returned status %d", resp.StatusCode)
	}

	var wikiResp wikipediaResponse
	if err := json.NewDecoder(resp.Body).Decode(&wikiResp); err != nil {
		return nil, err
	}

	var results []model.Result
	for _, item := range wikiResp.Query.Search {
		results = append(results, model.Result{
			Title:       item.Title,
			URL:         fmt.Sprintf("https://en.wikipedia.org/?curid=%d", item.PageID),
			Content:     item.Snippet,
			Engine:      e.Name(),
			Category:    query.Category,
			PublishedAt: time.Now(),
		})
	}

	return results, nil
}
