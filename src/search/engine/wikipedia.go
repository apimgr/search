package engine

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

// WikipediaEngine implements Wikipedia search engine
type WikipediaEngine struct {
	*search.BaseEngine
}

// NewWikipediaEngine creates a new Wikipedia search engine
func NewWikipediaEngine() *WikipediaEngine {
	config := model.NewEngineConfig("wikipedia")
	config.DisplayName = "Wikipedia"
	config.Categories = []string{"general"}
	config.Priority = 70

	return &WikipediaEngine{
		BaseEngine: search.NewBaseEngine(config),
	}
}

// wikipediaExtractsResponse is the response from the generator+extracts API.
// Pages are keyed by pageid (as string), returned in discovery order.
type wikipediaExtractsResponse struct {
	Query struct {
		Pages map[string]struct {
			Title   string `json:"title"`
			PageID  int    `json:"pageid"`
			Extract string `json:"extract"`
		} `json:"pages"`
	} `json:"query"`
}

// Search performs a Wikipedia search using the generator+extracts API which
// returns the first 3 sentences of each article intro as plain text.
func (e *WikipediaEngine) Search(ctx context.Context, query *model.Query) ([]model.Result, error) {
	params := url.Values{}
	params.Set("action", "query")
	params.Set("format", "json")
	// generator=search discovers pages matching the query
	params.Set("generator", "search")
	params.Set("gsrsearch", query.Text)
	params.Set("gsrlimit", "10")
	params.Set("gsroffset", fmt.Sprintf("%d", query.Page*10))
	// prop=extracts gives the article text
	params.Set("prop", "extracts")
	// exintro=true limits to the intro section only
	params.Set("exintro", "true")
	// explaintext=true strips HTML from the extract
	params.Set("explaintext", "true")
	// exsentences=3 returns only the first 3 sentences
	params.Set("exsentences", "3")

	searchURL := "https://en.wikipedia.org/w/api.php?" + params.Encode()

	req, err := http.NewRequestWithContext(ctx, "GET", searchURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", UserAgent)

	client := &http.Client{Timeout: 10 * time.Second, Transport: SharedTransport}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("wikipedia returned status %d", resp.StatusCode)
	}

	var wikiResp wikipediaExtractsResponse
	if err := json.NewDecoder(resp.Body).Decode(&wikiResp); err != nil {
		return nil, err
	}

	results := make([]model.Result, 0, len(wikiResp.Query.Pages))
	position := 0
	for _, page := range wikiResp.Query.Pages {
		extract := page.Extract
		if len(extract) > 500 {
			extract = extract[:500] + "…"
		}
		results = append(results, model.Result{
			Title:       page.Title,
			URL:         fmt.Sprintf("https://en.wikipedia.org/?curid=%d", page.PageID),
			Content:     extract,
			Engine:      e.Name(),
			Category:    query.Category,
			PublishedAt: time.Now(),
			Score:       calculateScore(e.GetPriority(), position, 1),
			Position:    position,
		})
		position++
		if position >= e.GetConfig().GetMaxResults() {
			break
		}
	}

	return results, nil
}
