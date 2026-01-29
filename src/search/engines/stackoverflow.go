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

// StackOverflow implements Stack Overflow search engine
type StackOverflow struct {
	*search.BaseEngine
	client *http.Client
}

// NewStackOverflow creates a new Stack Overflow search engine
func NewStackOverflow() *StackOverflow {
	config := model.NewEngineConfig("stackoverflow")
	config.DisplayName = "Stack Overflow"
	config.Priority = 55
	config.Categories = []string{"general", "code"}
	config.SupportsTor = true

	return &StackOverflow{
		BaseEngine: search.NewBaseEngine(config),
		client: &http.Client{
			Timeout: time.Duration(config.GetTimeout()) * time.Second,
		},
	}
}

// Search performs a Stack Overflow search
func (e *StackOverflow) Search(ctx context.Context, query *model.Query) ([]model.Result, error) {
	// Stack Exchange API
	searchURL := "https://api.stackexchange.com/2.3/search/advanced"

	params := url.Values{}
	params.Set("q", query.Text)
	params.Set("site", "stackoverflow")
	params.Set("order", "desc")
	params.Set("sort", "relevance")
	params.Set("pagesize", "10")
	params.Set("filter", "withbody")

	reqURL := fmt.Sprintf("%s?%s", searchURL, params.Encode())

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Accept-Encoding", "gzip, deflate")

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Stack Overflow API returned status %d", resp.StatusCode)
	}

	var data struct {
		Items []struct {
			QuestionID   int      `json:"question_id"`
			Title        string   `json:"title"`
			Link         string   `json:"link"`
			Body         string   `json:"body"`
			Tags         []string `json:"tags"`
			Score        int      `json:"score"`
			AnswerCount  int      `json:"answer_count"`
			IsAnswered   bool     `json:"is_answered"`
			CreationDate int64    `json:"creation_date"`
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

		// Build content from tags and answer info
		content := ""
		if len(item.Tags) > 0 {
			content = fmt.Sprintf("[%s]", item.Tags[0])
			for _, tag := range item.Tags[1:] {
				if len(content) < 50 {
					content += fmt.Sprintf(" [%s]", tag)
				}
			}
			content += " "
		}

		if item.IsAnswered {
			content += fmt.Sprintf("✓ %d answers", item.AnswerCount)
		} else if item.AnswerCount > 0 {
			content += fmt.Sprintf("%d answers", item.AnswerCount)
		} else {
			content += "No answers yet"
		}

		if item.Score != 0 {
			content += fmt.Sprintf(" | Score: %d", item.Score)
		}

		results = append(results, model.Result{
			Title:    unescapeHTML(item.Title),
			URL:      item.Link,
			Content:  content,
			Engine:   e.Name(),
			Category: model.CategoryGeneral,
			Score:    calculateScore(e.GetPriority(), i, 1),
			Position: i,
		})
	}

	return results, nil
}
