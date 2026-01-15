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

type QwantEngine struct {
	*search.BaseEngine
}

func NewQwantEngine() *QwantEngine {
	config := model.NewEngineConfig("qwant")
	config.DisplayName = "Qwant"
	config.Categories = []string{"general", "images", "videos", "news"}
	config.Priority = 75

	return &QwantEngine{
		BaseEngine: search.NewBaseEngine(config),
	}
}

type qwantResponse struct {
	Data struct {
		Result struct {
			Items []struct {
				Title   string `json:"title"`
				URL     string `json:"url"`
				Desc    string `json:"desc"`
				Source  string `json:"source"`
				Date    string `json:"date"`
				Media   string `json:"media"`
				Thumb   string `json:"thumbnail"`
			} `json:"items"`
		} `json:"result"`
	} `json:"data"`
}

func (e *QwantEngine) Search(ctx context.Context, query *model.Query) ([]model.Result, error) {
	offset := query.Page * 10

	var qwantCategory string
	switch query.Category {
	case model.CategoryImages:
		qwantCategory = "images"
	case model.CategoryVideos:
		qwantCategory = "videos"
	case model.CategoryNews:
		qwantCategory = "news"
	default:
		qwantCategory = "web"
	}

	searchURL := fmt.Sprintf("https://api.qwant.com/v3/search/%s?q=%s&count=10&offset=%d&locale=en_US",
		qwantCategory, url.QueryEscape(query.Text), offset)

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
		return nil, fmt.Errorf("qwant returned status %d", resp.StatusCode)
	}

	var qwantResp qwantResponse
	if err := json.NewDecoder(resp.Body).Decode(&qwantResp); err != nil {
		return nil, err
	}

	var results []model.Result
	for _, item := range qwantResp.Data.Result.Items {
		result := model.Result{
			Title:       item.Title,
			URL:         item.URL,
			Content:     item.Desc,
			Engine:      e.Name(),
			Category:    query.Category,
			PublishedAt: time.Now(),
		}

		if item.Thumb != "" {
			result.Thumbnail = item.Thumb
		}
		if item.Media != "" {
			result.Thumbnail = item.Media
		}
		if item.Source != "" {
			result.Author = item.Source
		}

		results = append(results, result)
	}

	return results, nil
}
