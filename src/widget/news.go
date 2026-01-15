package widget

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

// NewsFetcher fetches news from RSS feeds
type NewsFetcher struct {
	client *http.Client
	config *config.NewsWidgetConfig
}

// NewsData represents news widget data
type NewsData struct {
	Items []NewsItem `json:"items"`
}

// NewsItem represents a single news item
type NewsItem struct {
	Title       string    `json:"title"`
	URL         string    `json:"url"`
	Source      string    `json:"source"`
	PublishedAt time.Time `json:"published_at"`
	Summary     string    `json:"summary,omitempty"`
}

// RSS feed structures
type RSSFeed struct {
	XMLName xml.Name   `xml:"rss"`
	Channel RSSChannel `xml:"channel"`
}

type RSSChannel struct {
	Title       string    `xml:"title"`
	Link        string    `xml:"link"`
	Description string    `xml:"description"`
	Items       []RSSItem `xml:"item"`
}

type RSSItem struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	Description string `xml:"description"`
	PubDate     string `xml:"pubDate"`
	GUID        string `xml:"guid"`
}

// Atom feed structures
type AtomFeed struct {
	XMLName xml.Name    `xml:"feed"`
	Title   string      `xml:"title"`
	Entries []AtomEntry `xml:"entry"`
}

type AtomEntry struct {
	Title     string     `xml:"title"`
	Link      AtomLink   `xml:"link"`
	Summary   string     `xml:"summary"`
	Published string     `xml:"published"`
	Updated   string     `xml:"updated"`
	ID        string     `xml:"id"`
}

type AtomLink struct {
	Href string `xml:"href,attr"`
	Rel  string `xml:"rel,attr"`
}

// NewNewsFetcher creates a new news fetcher
func NewNewsFetcher(cfg *config.NewsWidgetConfig) *NewsFetcher {
	return &NewsFetcher{
		client: &http.Client{
			Timeout: 15 * time.Second,
		},
		config: cfg,
	}
}

// WidgetType returns the widget type
func (f *NewsFetcher) WidgetType() WidgetType {
	return WidgetNews
}

// CacheDuration returns how long to cache the data
func (f *NewsFetcher) CacheDuration() time.Duration {
	return 30 * time.Minute
}

// Fetch fetches news from configured RSS feeds
func (f *NewsFetcher) Fetch(ctx context.Context, params map[string]string) (*WidgetData, error) {
	sources := f.config.Sources
	if len(sources) == 0 {
		// Default news sources
		sources = []string{
			"https://feeds.bbci.co.uk/news/world/rss.xml",
			"https://rss.nytimes.com/services/xml/rss/nyt/World.xml",
		}
	}

	maxItems := f.config.MaxItems
	if maxItems <= 0 {
		maxItems = 10
	}

	// Fetch all feeds concurrently
	type fetchResult struct {
		items []NewsItem
		err   error
	}

	results := make(chan fetchResult, len(sources))
	for _, source := range sources {
		go func(feedURL string) {
			items, err := f.fetchFeed(ctx, feedURL)
			results <- fetchResult{items: items, err: err}
		}(source)
	}

	// Collect results
	var allItems []NewsItem
	for range sources {
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
		Type:      WidgetNews,
		Data:      &NewsData{Items: allItems},
		UpdatedAt: time.Now(),
	}, nil
}

// fetchFeed fetches and parses a single RSS/Atom feed
func (f *NewsFetcher) fetchFeed(ctx context.Context, feedURL string) ([]NewsItem, error) {
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

	// Try parsing as RSS first
	var rssFeed RSSFeed
	decoder := xml.NewDecoder(resp.Body)
	if err := decoder.Decode(&rssFeed); err == nil && len(rssFeed.Channel.Items) > 0 {
		return f.parseRSSItems(rssFeed.Channel), nil
	}

	// Try Atom format
	resp.Body.Close()
	resp, err = f.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var atomFeed AtomFeed
	decoder = xml.NewDecoder(resp.Body)
	if err := decoder.Decode(&atomFeed); err == nil && len(atomFeed.Entries) > 0 {
		return f.parseAtomEntries(atomFeed), nil
	}

	return nil, fmt.Errorf("failed to parse feed")
}

// parseRSSItems converts RSS items to NewsItems
func (f *NewsFetcher) parseRSSItems(channel RSSChannel) []NewsItem {
	items := make([]NewsItem, 0, len(channel.Items))
	sourceName := extractSourceName(channel.Title)

	for _, item := range channel.Items {
		pubDate := parseRSSDate(item.PubDate)
		items = append(items, NewsItem{
			Title:       cleanText(item.Title),
			URL:         item.Link,
			Source:      sourceName,
			PublishedAt: pubDate,
			Summary:     truncateSummary(cleanText(item.Description)),
		})
	}

	return items
}

// parseAtomEntries converts Atom entries to NewsItems
func (f *NewsFetcher) parseAtomEntries(feed AtomFeed) []NewsItem {
	items := make([]NewsItem, 0, len(feed.Entries))
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

		items = append(items, NewsItem{
			Title:       cleanText(entry.Title),
			URL:         link,
			Source:      sourceName,
			PublishedAt: pubDate,
			Summary:     truncateSummary(cleanText(entry.Summary)),
		})
	}

	return items
}

// parseRSSDate parses various RSS date formats
func parseRSSDate(dateStr string) time.Time {
	formats := []string{
		time.RFC1123Z,
		time.RFC1123,
		time.RFC822Z,
		time.RFC822,
		"Mon, 2 Jan 2006 15:04:05 -0700",
		"Mon, 2 Jan 2006 15:04:05 MST",
		"2006-01-02T15:04:05Z",
		"2006-01-02T15:04:05-07:00",
	}

	for _, format := range formats {
		if t, err := time.Parse(format, dateStr); err == nil {
			return t
		}
	}

	return time.Now()
}

// parseAtomDate parses Atom date formats
func parseAtomDate(dateStr string) time.Time {
	formats := []string{
		time.RFC3339,
		time.RFC3339Nano,
		"2006-01-02T15:04:05Z",
		"2006-01-02T15:04:05-07:00",
	}

	for _, format := range formats {
		if t, err := time.Parse(format, dateStr); err == nil {
			return t
		}
	}

	return time.Time{}
}

// extractSourceName extracts a clean source name from feed title
func extractSourceName(title string) string {
	// Remove common suffixes
	suffixes := []string{" - RSS Feed", " RSS", " Feed", " News"}
	for _, suffix := range suffixes {
		title = strings.TrimSuffix(title, suffix)
	}
	return strings.TrimSpace(title)
}

// cleanText removes HTML tags and cleans up text
func cleanText(text string) string {
	// Simple HTML tag removal
	text = strings.ReplaceAll(text, "<![CDATA[", "")
	text = strings.ReplaceAll(text, "]]>", "")

	// Remove HTML tags
	for {
		start := strings.Index(text, "<")
		if start == -1 {
			break
		}
		end := strings.Index(text[start:], ">")
		if end == -1 {
			break
		}
		text = text[:start] + text[start+end+1:]
	}

	// Clean up whitespace
	text = strings.TrimSpace(text)
	text = strings.ReplaceAll(text, "\n", " ")
	text = strings.ReplaceAll(text, "\r", "")
	text = strings.ReplaceAll(text, "  ", " ")

	return text
}

// truncateSummary truncates summary to a reasonable length
func truncateSummary(text string) string {
	maxLen := 200
	if len(text) <= maxLen {
		return text
	}

	// Find last space before maxLen
	lastSpace := strings.LastIndex(text[:maxLen], " ")
	if lastSpace > 0 {
		return text[:lastSpace] + "..."
	}
	return text[:maxLen] + "..."
}
