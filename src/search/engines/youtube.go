package engines

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/apimgr/search/src/model"
	"github.com/apimgr/search/src/search"
)

// YouTube implements YouTube Search engine
type YouTube struct {
	*search.BaseEngine
	client *http.Client
}

// NewYouTubeEngine creates a new YouTube search engine
func NewYouTubeEngine() *YouTube {
	config := model.NewEngineConfig("youtube")
	config.DisplayName = "YouTube"
	config.Priority = 65
	config.Categories = []string{"videos"}
	config.SupportsTor = false

	return &YouTube{
		BaseEngine: search.NewBaseEngine(config),
		client: &http.Client{
			Timeout: time.Duration(config.GetTimeout()) * time.Second,
		},
	}
}

// Search performs a YouTube search
func (e *YouTube) Search(ctx context.Context, query *model.Query) ([]model.Result, error) {
	searchURL := "https://www.youtube.com/results"

	params := url.Values{}
	params.Set("search_query", query.Text)

	reqURL := fmt.Sprintf("%s?%s", searchURL, params.Encode())

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("youtube returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return e.parseResults(string(body), query)
}

func (e *YouTube) parseResults(html string, query *model.Query) ([]model.Result, error) {
	maxResults := 10

	// YouTube embeds search results in a JSON object within the page
	// Look for ytInitialData
	jsonPattern := regexp.MustCompile(`var ytInitialData = (\{.+?\});`)
	jsonMatch := jsonPattern.FindStringSubmatch(html)

	if len(jsonMatch) < 2 {
		// Try alternative pattern
		altPattern := regexp.MustCompile(`ytInitialData"\s*:\s*(\{.+?\})\s*,`)
		jsonMatch = altPattern.FindStringSubmatch(html)
	}

	if len(jsonMatch) >= 2 {
		return e.parseJSON(jsonMatch[1], query, maxResults)
	}

	// Fallback to HTML parsing
	return e.parseHTML(html, query, maxResults)
}

func (e *YouTube) parseJSON(jsonStr string, query *model.Query, maxResults int) ([]model.Result, error) {
	var results []model.Result

	var data map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
		return nil, err
	}

	// Navigate the complex YouTube JSON structure
	contents, ok := data["contents"].(map[string]interface{})
	if !ok {
		return results, nil
	}

	twoColumn, ok := contents["twoColumnSearchResultsRenderer"].(map[string]interface{})
	if !ok {
		return results, nil
	}

	primaryContents, ok := twoColumn["primaryContents"].(map[string]interface{})
	if !ok {
		return results, nil
	}

	sectionList, ok := primaryContents["sectionListRenderer"].(map[string]interface{})
	if !ok {
		return results, nil
	}

	sectionContents, ok := sectionList["contents"].([]interface{})
	if !ok {
		return results, nil
	}

	for _, section := range sectionContents {
		sectionMap, ok := section.(map[string]interface{})
		if !ok {
			continue
		}

		itemSection, ok := sectionMap["itemSectionRenderer"].(map[string]interface{})
		if !ok {
			continue
		}

		items, ok := itemSection["contents"].([]interface{})
		if !ok {
			continue
		}

		for _, item := range items {
			if len(results) >= maxResults {
				break
			}

			itemMap, ok := item.(map[string]interface{})
			if !ok {
				continue
			}

			videoRenderer, ok := itemMap["videoRenderer"].(map[string]interface{})
			if !ok {
				continue
			}

			result := e.parseVideoRenderer(videoRenderer, query)
			if result != nil {
				results = append(results, *result)
			}
		}
	}

	return results, nil
}

func (e *YouTube) parseVideoRenderer(video map[string]interface{}, query *model.Query) *model.Result {
	// Extract video ID
	videoID, ok := video["videoId"].(string)
	if !ok || videoID == "" {
		return nil
	}

	// Extract title
	title := ""
	if titleObj, ok := video["title"].(map[string]interface{}); ok {
		if runs, ok := titleObj["runs"].([]interface{}); ok && len(runs) > 0 {
			if run, ok := runs[0].(map[string]interface{}); ok {
				title, _ = run["text"].(string)
			}
		}
	}

	// Extract description
	description := ""
	if descObj, ok := video["descriptionSnippet"].(map[string]interface{}); ok {
		if runs, ok := descObj["runs"].([]interface{}); ok {
			for _, run := range runs {
				if runMap, ok := run.(map[string]interface{}); ok {
					if text, ok := runMap["text"].(string); ok {
						description += text
					}
				}
			}
		}
	}

	// Extract thumbnail
	thumbnail := ""
	if thumbObj, ok := video["thumbnail"].(map[string]interface{}); ok {
		if thumbs, ok := thumbObj["thumbnails"].([]interface{}); ok && len(thumbs) > 0 {
			if thumb, ok := thumbs[len(thumbs)-1].(map[string]interface{}); ok {
				thumbnail, _ = thumb["url"].(string)
			}
		}
	}

	// Extract channel name
	channel := ""
	if ownerObj, ok := video["ownerText"].(map[string]interface{}); ok {
		if runs, ok := ownerObj["runs"].([]interface{}); ok && len(runs) > 0 {
			if run, ok := runs[0].(map[string]interface{}); ok {
				channel, _ = run["text"].(string)
			}
		}
	}

	// Extract view count
	viewCount := ""
	if viewObj, ok := video["viewCountText"].(map[string]interface{}); ok {
		viewCount, _ = viewObj["simpleText"].(string)
	}

	// Extract published time
	published := ""
	if pubObj, ok := video["publishedTimeText"].(map[string]interface{}); ok {
		published, _ = pubObj["simpleText"].(string)
	}

	// Build result
	videoURL := fmt.Sprintf("https://www.youtube.com/watch?v=%s", videoID)

	// Combine metadata into description
	if channel != "" || viewCount != "" || published != "" {
		meta := []string{}
		if channel != "" {
			meta = append(meta, channel)
		}
		if viewCount != "" {
			meta = append(meta, viewCount)
		}
		if published != "" {
			meta = append(meta, published)
		}
		if description != "" {
			description = strings.Join(meta, " - ") + " | " + description
		} else {
			description = strings.Join(meta, " - ")
		}
	}

	return &model.Result{
		Title:     title,
		URL:       videoURL,
		Content:   description,
		Thumbnail: thumbnail,
		Engine:    "YouTube",
		Category:  query.Category,
		Score:     65,
	}
}

func (e *YouTube) parseHTML(html string, query *model.Query, maxResults int) ([]model.Result, error) {
	var results []model.Result

	// Fallback HTML parsing for when JSON extraction fails
	videoPattern := regexp.MustCompile(`/watch\?v=([a-zA-Z0-9_-]{11})`)

	seen := make(map[string]bool)
	matches := videoPattern.FindAllStringSubmatch(html, 30)

	for _, match := range matches {
		if len(results) >= maxResults {
			break
		}

		if len(match) < 2 {
			continue
		}

		videoID := match[1]
		if seen[videoID] {
			continue
		}
		seen[videoID] = true

		videoURL := fmt.Sprintf("https://www.youtube.com/watch?v=%s", videoID)
		thumbnail := fmt.Sprintf("https://img.youtube.com/vi/%s/mqdefault.jpg", videoID)

		results = append(results, model.Result{
			Title:     fmt.Sprintf("YouTube Video %s", videoID),
			URL:       videoURL,
			Thumbnail: thumbnail,
			Engine:    "YouTube",
			Category:  query.Category,
			Score:     float64(65) / float64(len(results)+1),
		})
	}

	return results, nil
}
