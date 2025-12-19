package models

import (
	"encoding/csv"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"strings"
	"time"
)

// Result represents a single search result from an engine
type Result struct {
	// Core fields
	Title    string   `json:"title" xml:"title"`
	URL      string   `json:"url" xml:"link"`
	Content  string   `json:"content" xml:"description"`
	Engine   string   `json:"engine" xml:"source"`
	Category Category `json:"category" xml:"category"`

	// Additional fields
	Thumbnail   string    `json:"thumbnail,omitempty" xml:"thumbnail,omitempty"`
	Author      string    `json:"author,omitempty" xml:"author,omitempty"`
	PublishedAt time.Time `json:"published_at,omitempty" xml:"pubDate,omitempty"`
	Domain      string    `json:"domain,omitempty" xml:"domain,omitempty"`

	// Media-specific fields
	ImageWidth  int    `json:"image_width,omitempty" xml:"-"`
	ImageHeight int    `json:"image_height,omitempty" xml:"-"`
	ImageFormat string `json:"image_format,omitempty" xml:"-"`
	Duration    int    `json:"duration,omitempty" xml:"-"`    // Video duration in seconds
	ViewCount   int64  `json:"view_count,omitempty" xml:"-"`  // Video view count
	FileSize    int64  `json:"file_size,omitempty" xml:"-"`   // File size in bytes
	FileType    string `json:"file_type,omitempty" xml:"-"`   // File extension

	// Scoring fields
	Score          float64 `json:"score" xml:"-"`
	Position       int     `json:"position" xml:"-"`
	Relevance      float64 `json:"relevance,omitempty" xml:"-"`      // Engine-provided relevance
	Popularity     float64 `json:"popularity,omitempty" xml:"-"`     // Engagement/popularity score
	DuplicateCount int     `json:"duplicate_count,omitempty" xml:"-"` // How many engines returned this

	// Language detection
	Language string `json:"language,omitempty" xml:"language,omitempty"`

	// Metadata
	Metadata map[string]interface{} `json:"metadata,omitempty" xml:"-"`
}

// ExtractDomain extracts the domain from the URL
func (r *Result) ExtractDomain() string {
	if r.Domain != "" {
		return r.Domain
	}

	url := r.URL
	// Remove protocol
	url = strings.TrimPrefix(url, "https://")
	url = strings.TrimPrefix(url, "http://")

	// Get domain part
	if idx := strings.Index(url, "/"); idx != -1 {
		url = url[:idx]
	}

	// Remove www prefix
	url = strings.TrimPrefix(url, "www.")

	r.Domain = url
	return r.Domain
}

// Age returns how old the result is
func (r *Result) Age() time.Duration {
	if r.PublishedAt.IsZero() {
		return 0
	}
	return time.Since(r.PublishedAt)
}

// IsRecent checks if the result is from the last n hours
func (r *Result) IsRecent(hours int) bool {
	if r.PublishedAt.IsZero() {
		return false
	}
	return r.Age() < time.Duration(hours)*time.Hour
}

// SearchResults represents aggregated search results
type SearchResults struct {
	Query        string    `json:"query" xml:"query"`
	Category     Category  `json:"category" xml:"category"`
	Results      []Result  `json:"results" xml:"item"`
	TotalResults int       `json:"total_results" xml:"totalResults"`
	Page         int       `json:"page" xml:"page"`
	PerPage      int       `json:"per_page" xml:"perPage"`
	TotalPages   int       `json:"total_pages" xml:"totalPages"`
	SearchTime   float64   `json:"search_time" xml:"searchTime"`
	Engines      []string  `json:"engines" xml:"engines"`
	Suggestions  []string  `json:"suggestions,omitempty" xml:"suggestions,omitempty"`
	SortedBy     SortOrder `json:"sorted_by,omitempty" xml:"sortedBy,omitempty"`

	// Facets for filtering (future use)
	Domains   map[string]int `json:"domains,omitempty" xml:"-"`
	Languages map[string]int `json:"languages,omitempty" xml:"-"`
}

// NewSearchResults creates a new SearchResults instance
func NewSearchResults(query string, category Category) *SearchResults {
	return &SearchResults{
		Query:     query,
		Category:  category,
		Results:   make([]Result, 0),
		Engines:   make([]string, 0),
		Page:      1,
		PerPage:   20,
		SortedBy:  SortRelevance,
		Domains:   make(map[string]int),
		Languages: make(map[string]int),
	}
}

// AddResult adds a result to the collection
func (sr *SearchResults) AddResult(result Result) {
	sr.Results = append(sr.Results, result)
	sr.TotalResults = len(sr.Results)
	sr.updateFacets(result)
}

// AddResults adds multiple results to the collection
func (sr *SearchResults) AddResults(results []Result) {
	sr.Results = append(sr.Results, results...)
	sr.TotalResults = len(sr.Results)
	for _, r := range results {
		sr.updateFacets(r)
	}
}

// updateFacets updates domain and language facet counts
func (sr *SearchResults) updateFacets(r Result) {
	domain := r.ExtractDomain()
	if domain != "" {
		sr.Domains[domain]++
	}
	if r.Language != "" {
		sr.Languages[r.Language]++
	}
}

// CalculateTotalPages calculates the total number of pages
func (sr *SearchResults) CalculateTotalPages() {
	if sr.PerPage > 0 {
		sr.TotalPages = (sr.TotalResults + sr.PerPage - 1) / sr.PerPage
	}
}

// GetPage returns results for a specific page
func (sr *SearchResults) GetPage(page int) []Result {
	start := (page - 1) * sr.PerPage
	end := start + sr.PerPage

	if start >= len(sr.Results) {
		return []Result{}
	}

	if end > len(sr.Results) {
		end = len(sr.Results)
	}

	return sr.Results[start:end]
}

// ToJSON exports results as JSON
func (sr *SearchResults) ToJSON(w io.Writer, pretty bool) error {
	enc := json.NewEncoder(w)
	if pretty {
		enc.SetIndent("", "  ")
	}
	return enc.Encode(sr)
}

// ToCSV exports results as CSV
func (sr *SearchResults) ToCSV(w io.Writer) error {
	writer := csv.NewWriter(w)
	defer writer.Flush()

	// Header
	header := []string{"Title", "URL", "Content", "Engine", "Category", "Domain", "Author", "Published", "Score"}
	if err := writer.Write(header); err != nil {
		return err
	}

	// Data rows
	for _, r := range sr.Results {
		published := ""
		if !r.PublishedAt.IsZero() {
			published = r.PublishedAt.Format(time.RFC3339)
		}

		row := []string{
			r.Title,
			r.URL,
			r.Content,
			r.Engine,
			string(r.Category),
			r.ExtractDomain(),
			r.Author,
			published,
			fmt.Sprintf("%.2f", r.Score),
		}
		if err := writer.Write(row); err != nil {
			return err
		}
	}

	return nil
}

// RSSFeed represents an RSS 2.0 feed
type RSSFeed struct {
	XMLName xml.Name   `xml:"rss"`
	Version string     `xml:"version,attr"`
	Channel RSSChannel `xml:"channel"`
}

// RSSChannel represents an RSS channel
type RSSChannel struct {
	Title         string    `xml:"title"`
	Link          string    `xml:"link"`
	Description   string    `xml:"description"`
	Language      string    `xml:"language,omitempty"`
	LastBuildDate string    `xml:"lastBuildDate"`
	Items         []RSSItem `xml:"item"`
}

// RSSItem represents an RSS item
type RSSItem struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	Description string `xml:"description"`
	Author      string `xml:"author,omitempty"`
	PubDate     string `xml:"pubDate,omitempty"`
	Source      string `xml:"source,omitempty"`
	GUID        string `xml:"guid"`
}

// ToRSS exports results as RSS 2.0 feed
func (sr *SearchResults) ToRSS(w io.Writer, baseURL string) error {
	items := make([]RSSItem, 0, len(sr.Results))

	for _, r := range sr.Results {
		pubDate := ""
		if !r.PublishedAt.IsZero() {
			pubDate = r.PublishedAt.Format(time.RFC1123Z)
		}

		items = append(items, RSSItem{
			Title:       r.Title,
			Link:        r.URL,
			Description: r.Content,
			Author:      r.Author,
			PubDate:     pubDate,
			Source:      r.Engine,
			GUID:        r.URL,
		})
	}

	feed := RSSFeed{
		Version: "2.0",
		Channel: RSSChannel{
			Title:         fmt.Sprintf("Search results for: %s", sr.Query),
			Link:          baseURL,
			Description:   fmt.Sprintf("Search results from %d engines", len(sr.Engines)),
			Language:      "en",
			LastBuildDate: time.Now().Format(time.RFC1123Z),
			Items:         items,
		},
	}

	w.Write([]byte(xml.Header))
	enc := xml.NewEncoder(w)
	enc.Indent("", "  ")
	return enc.Encode(feed)
}

// ToAtom exports results as Atom feed
func (sr *SearchResults) ToAtom(w io.Writer, baseURL string) error {
	// Atom feed structure
	type AtomLink struct {
		Href string `xml:"href,attr"`
		Rel  string `xml:"rel,attr,omitempty"`
		Type string `xml:"type,attr,omitempty"`
	}

	type AtomEntry struct {
		Title   string    `xml:"title"`
		Link    AtomLink  `xml:"link"`
		ID      string    `xml:"id"`
		Updated string    `xml:"updated"`
		Summary string    `xml:"summary"`
		Author  *struct {
			Name string `xml:"name"`
		} `xml:"author,omitempty"`
	}

	type AtomFeed struct {
		XMLName xml.Name    `xml:"feed"`
		XMLNS   string      `xml:"xmlns,attr"`
		Title   string      `xml:"title"`
		Link    AtomLink    `xml:"link"`
		Updated string      `xml:"updated"`
		ID      string      `xml:"id"`
		Entries []AtomEntry `xml:"entry"`
	}

	entries := make([]AtomEntry, 0, len(sr.Results))
	for _, r := range sr.Results {
		updated := time.Now()
		if !r.PublishedAt.IsZero() {
			updated = r.PublishedAt
		}

		entry := AtomEntry{
			Title:   r.Title,
			Link:    AtomLink{Href: r.URL},
			ID:      r.URL,
			Updated: updated.Format(time.RFC3339),
			Summary: r.Content,
		}

		if r.Author != "" {
			entry.Author = &struct {
				Name string `xml:"name"`
			}{Name: r.Author}
		}

		entries = append(entries, entry)
	}

	feed := AtomFeed{
		XMLNS:   "http://www.w3.org/2005/Atom",
		Title:   fmt.Sprintf("Search results for: %s", sr.Query),
		Link:    AtomLink{Href: baseURL, Rel: "self", Type: "application/atom+xml"},
		Updated: time.Now().Format(time.RFC3339),
		ID:      baseURL,
		Entries: entries,
	}

	w.Write([]byte(xml.Header))
	enc := xml.NewEncoder(w)
	enc.Indent("", "  ")
	return enc.Encode(feed)
}
