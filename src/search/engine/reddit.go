package engine

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/apimgr/search/src/model"
	"github.com/apimgr/search/src/search"
)

// Reddit implements Reddit search engine
type Reddit struct {
	*search.BaseEngine
	client *http.Client

	tokenMu     sync.Mutex
	accessToken string
	tokenExpiry time.Time
}

// NewReddit creates a new Reddit search engine
func NewReddit() *Reddit {
	config := model.NewEngineConfig("reddit")
	config.DisplayName = "Reddit"
	config.Priority = 45
	config.Categories = []string{"general", "social"}
	config.SupportsTor = true

	return &Reddit{
		BaseEngine: search.NewBaseEngine(config),
		client: &http.Client{
			Timeout:   time.Duration(config.GetTimeout()) * time.Second,
			Transport: SharedTransport,
		},
	}
}

// redditListing is the shared JSON shape returned by both the public
// old.reddit.com search endpoint and the authenticated oauth.reddit.com one.
type redditListing struct {
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

// userAgent returns a descriptive User-Agent for Reddit's API rules
// (<platform>:<app ID>:<version> (by /contact)). Reddit requires a
// descriptive, contactable UA to reduce 403/429 risk; since no Reddit
// username is collected from the operator, an operator-configured contact
// email is used instead when available.
func (e *Reddit) userAgent() string {
	contact := e.GetConfig().GetSetting("contact_email")
	if contact != "" {
		return fmt.Sprintf("web:search:1.0 (by %s)", contact)
	}
	return "web:search:1.0 (contact not configured)"
}

// getAccessToken obtains (and caches) an OAuth2 client-credentials access
// token from Reddit's app-only API, per Reddit's official free "app-only"
// API mode.
func (e *Reddit) getAccessToken(ctx context.Context, clientID, clientSecret string) (string, error) {
	e.tokenMu.Lock()
	defer e.tokenMu.Unlock()

	if e.accessToken != "" && time.Now().Before(e.tokenExpiry) {
		return e.accessToken, nil
	}

	tokenURL := "https://www.reddit.com/api/v1/access_token"
	body := strings.NewReader("grant_type=client_credentials")

	req, err := http.NewRequestWithContext(ctx, "POST", tokenURL, body)
	if err != nil {
		return "", err
	}

	creds := base64.StdEncoding.EncodeToString([]byte(clientID + ":" + clientSecret))
	req.Header.Set("Authorization", "Basic "+creds)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", e.userAgent())

	resp, err := e.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Reddit access_token request returned status %d", resp.StatusCode)
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", fmt.Errorf("failed to parse Reddit access_token response: %w", err)
	}
	if tokenResp.AccessToken == "" {
		return "", fmt.Errorf("Reddit access_token response missing access_token")
	}

	e.accessToken = tokenResp.AccessToken
	e.tokenExpiry = time.Now().Add(time.Duration(tokenResp.ExpiresIn)*time.Second - 30*time.Second)

	return e.accessToken, nil
}

// Search performs a Reddit search
func (e *Reddit) Search(ctx context.Context, query *model.Query) ([]model.Result, error) {
	clientID := e.GetConfig().GetSetting("client_id")
	clientSecret := e.GetConfig().GetSetting("client_secret")

	var listing *redditListing
	var err error

	if clientID != "" && clientSecret != "" {
		listing, err = e.searchAuthenticated(ctx, query, clientID, clientSecret)
	} else {
		listing, err = e.searchPublic(ctx, query)
	}
	if err != nil {
		return nil, err
	}

	return e.listingToResults(listing), nil
}

// searchPublic queries the public old.reddit.com JSON API (avoids OAuth
// requirement, but is aggressively rate-limited/blocked without an
// authenticated app).
func (e *Reddit) searchPublic(ctx context.Context, query *model.Query) (*redditListing, error) {
	searchURL := "https://old.reddit.com/search.json"

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

	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Reddit API returned status %d", resp.StatusCode)
	}

	var data redditListing
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	return &data, nil
}

// searchAuthenticated queries the OAuth2 app-only oauth.reddit.com API using
// an operator-configured client_id/client_secret, avoiding the aggressive
// rate limiting/blocking seen on the public endpoint.
func (e *Reddit) searchAuthenticated(ctx context.Context, query *model.Query, clientID, clientSecret string) (*redditListing, error) {
	token, err := e.getAccessToken(ctx, clientID, clientSecret)
	if err != nil {
		return nil, err
	}

	searchURL := "https://oauth.reddit.com/search"

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

	req.Header.Set("User-Agent", e.userAgent())
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Reddit API returned status %d", resp.StatusCode)
	}

	var data redditListing
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	return &data, nil
}

// listingToResults converts a Reddit "Listing" JSON payload into search results.
func (e *Reddit) listingToResults(data *redditListing) []model.Result {
	results := make([]model.Result, 0)

	for i, child := range data.Data.Children {
		if i >= e.GetConfig().GetMaxResults() {
			break
		}

		item := child.Data

		// Build the full URL using old.reddit.com for consistency
		postURL := fmt.Sprintf("https://old.reddit.com%s", item.Permalink)

		// Build content
		content := fmt.Sprintf("r/%s | ⬆ %d | 💬 %d",
			item.Subreddit, item.Score, item.NumComments)

		// Add a snippet of selftext if available
		if item.Selftext != "" && len(item.Selftext) > 0 {
			snippet := item.Selftext
			if len(snippet) > 400 {
				snippet = snippet[:400] + "..."
			}
			content += " | " + snippet
		}

		results = append(results, model.Result{
			Title:    item.Title,
			URL:      postURL,
			Content:  content,
			Engine:   e.Name(),
			Category: model.CategoryGeneral,
			Score:    calculateScore(e.GetPriority(), i, 1),
			Position: i,
		})
	}

	return results
}
