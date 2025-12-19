package widgets

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/apimgr/search/src/config"
)

// RSSFetcher fetches items from user-configured RSS feeds
type RSSFetcher struct {
	client *http.Client
	config *config.RSSWidgetConfig
}

// RSSData represents RSS widget data
type RSSData struct {
	Items []RSSItemData `json:"items"`
}

// RSSItemData represents a single RSS item
type RSSItemData struct {
	Title       string    `json:"title"`
	URL         string    `json:"url"`
	Source      string    `json:"source"`
	PublishedAt time.Time `json:"published_at"`
	Summary     string    `json:"summary,omitempty"`
}

// NewRSSFetcher creates a new RSS fetcher
func NewRSSFetcher(cfg *config.RSSWidgetConfig) *RSSFetcher {
	return &RSSFetcher{
		client: &http.Client{
			Timeout: 15 * time.Second,
		},
		config: cfg,
	}
}

// WidgetType returns the widget type
func (f *RSSFetcher) WidgetType() WidgetType {
	return WidgetRSS
}

// CacheDuration returns how long to cache the data
func (f *RSSFetcher) CacheDuration() time.Duration {
	return 30 * time.Minute
}

// Fetch fetches items from user-configured RSS feeds
func (f *RSSFetcher) Fetch(ctx context.Context, params map[string]string) (*WidgetData, error) {
	// Get feeds from params (user-configured via localStorage)
	feedsStr := params["feeds"]
	if feedsStr == "" {
		return &WidgetData{
			Type:      WidgetRSS,
			Data:      &RSSData{Items: []RSSItemData{}},
			UpdatedAt: time.Now(),
		}, nil
	}

	feeds := strings.Split(feedsStr, ",")
	for i := range feeds {
		feeds[i] = strings.TrimSpace(feeds[i])
	}

	// Limit number of feeds
	maxFeeds := f.config.MaxFeeds
	if maxFeeds <= 0 {
		maxFeeds = 5
	}
	if len(feeds) > maxFeeds {
		feeds = feeds[:maxFeeds]
	}

	maxItems := f.config.MaxItems
	if maxItems <= 0 {
		maxItems = 10
	}

	// Fetch all feeds concurrently
	type fetchResult struct {
		items []RSSItemData
		err   error
	}

	results := make(chan fetchResult, len(feeds))
	for _, feedURL := range feeds {
		if feedURL == "" {
			continue
		}
		go func(url string) {
			items, err := f.fetchSingleFeed(ctx, url)
			results <- fetchResult{items: items, err: err}
		}(feedURL)
	}

	// Collect results
	var allItems []RSSItemData
	for range feeds {
		result := <-results
		if result.err == nil {
			allItems = append(allItems, result.items...)
		}
	}

	// Sort by published date (newest first)
	sort.Slice(allItems, func(i, j int) bool {
		return allItems[i].PublishedAt.After(allItems[j].PublishedAt)
	})

	// Limit to max items
	if len(allItems) > maxItems {
		allItems = allItems[:maxItems]
	}

	return &WidgetData{
		Type:      WidgetRSS,
		Data:      &RSSData{Items: allItems},
		UpdatedAt: time.Now(),
	}, nil
}

// fetchSingleFeed fetches and parses a single RSS/Atom feed
func (f *RSSFetcher) fetchSingleFeed(ctx context.Context, feedURL string) ([]RSSItemData, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", feedURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36 Edg/131.0.0.0")
	req.Header.Set("Accept", "application/rss+xml, application/atom+xml, application/xml, text/xml")

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("feed returned status %d", resp.StatusCode)
	}

	// Try parsing as RSS
	var rssFeed RSSFeed
	decoder := xml.NewDecoder(resp.Body)
	if err := decoder.Decode(&rssFeed); err == nil && len(rssFeed.Channel.Items) > 0 {
		return f.convertRSSItems(rssFeed.Channel), nil
	}

	// Retry and try Atom format
	resp.Body.Close()
	req, _ = http.NewRequestWithContext(ctx, "GET", feedURL, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36 Edg/131.0.0.0")
	resp, err = f.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var atomFeed AtomFeed
	decoder = xml.NewDecoder(resp.Body)
	if err := decoder.Decode(&atomFeed); err == nil && len(atomFeed.Entries) > 0 {
		return f.convertAtomEntries(atomFeed), nil
	}

	return nil, fmt.Errorf("failed to parse feed")
}

// convertRSSItems converts RSS items to RSSItemData
func (f *RSSFetcher) convertRSSItems(channel RSSChannel) []RSSItemData {
	items := make([]RSSItemData, 0, len(channel.Items))
	sourceName := extractSourceName(channel.Title)

	for _, item := range channel.Items {
		pubDate := parseRSSDate(item.PubDate)
		items = append(items, RSSItemData{
			Title:       cleanText(item.Title),
			URL:         item.Link,
			Source:      sourceName,
			PublishedAt: pubDate,
			Summary:     truncateSummary(cleanText(item.Description)),
		})
	}

	return items
}

// convertAtomEntries converts Atom entries to RSSItemData
func (f *RSSFetcher) convertAtomEntries(feed AtomFeed) []RSSItemData {
	items := make([]RSSItemData, 0, len(feed.Entries))
	sourceName := extractSourceName(feed.Title)

	for _, entry := range feed.Entries {
		pubDate := parseAtomDate(entry.Published)
		if pubDate.IsZero() {
			pubDate = parseAtomDate(entry.Updated)
		}

		link := entry.Link.Href
		if link == "" {
			link = entry.ID
		}

		items = append(items, RSSItemData{
			Title:       cleanText(entry.Title),
			URL:         link,
			Source:      sourceName,
			PublishedAt: pubDate,
			Summary:     truncateSummary(cleanText(entry.Summary)),
		})
	}

	return items
}
