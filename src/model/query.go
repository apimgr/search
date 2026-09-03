package model

import "strings"

// SortOrder defines how results are sorted
type SortOrder string

const (
	// Default: by score
	SortRelevance SortOrder = "relevance"
	// By date (newest first)
	SortDate SortOrder = "date"
	// By date (oldest first)
	SortDateAsc SortOrder = "date_asc"
	// By popularity/engagement
	SortPopularity SortOrder = "popularity"
	// Random order
	SortRandom SortOrder = "random"
)

// Query represents a search query
type Query struct {
	// User input
	Text string `json:"text"`

	// Filters
	Category Category `json:"category"`
	Language string   `json:"language"`
	// Region code (us, uk, de, etc.)
	Region string `json:"region,omitempty"`
	// 0: off, 1: moderate, 2: strict
	SafeSearch int `json:"safe_search"`

	// Pagination
	Page    int `json:"page"`
	PerPage int `json:"per_page"`

	// Sorting
	SortBy SortOrder `json:"sort_by,omitempty"`

	// Time range
	// any, day, week, month, year
	TimeRange string `json:"time_range,omitempty"`

	// Advanced filters (parsed from operators or set directly)
	Site         string   `json:"site,omitempty"`
	ExcludeSite  string   `json:"exclude_site,omitempty"`
	FileType     string   `json:"file_type,omitempty"`
	FileTypes    []string `json:"file_types,omitempty"`
	InURL        string   `json:"in_url,omitempty"`
	InTitle      string   `json:"in_title,omitempty"`
	InText       string   `json:"in_text,omitempty"`
	ExactTerms   string   `json:"exact_terms,omitempty"`
	ExactPhrases []string `json:"exact_phrases,omitempty"`
	ExcludeTerms []string `json:"exclude_terms,omitempty"`

	// Date filters
	// YYYY-MM-DD
	DateBefore string `json:"date_before,omitempty"`
	// YYYY-MM-DD
	DateAfter string `json:"date_after,omitempty"`

	// Media-specific filters
	// small, medium, large, xlarge
	ImageSize string `json:"image_size,omitempty"`
	// photo, clipart, lineart, animated
	ImageType string `json:"image_type,omitempty"`
	// color, gray, trans, red, etc.
	ImageColor string `json:"image_color,omitempty"`
	// square, wide, tall
	ImageAspect string `json:"image_aspect,omitempty"`
	// short, medium, long
	VideoLength string `json:"video_length,omitempty"`
	// hd, 4k
	VideoQuality string `json:"video_quality,omitempty"`

	// News-specific
	// source:nytimes
	NewsSource string `json:"news_source,omitempty"`

	// Engine selection
	Engines        []string `json:"engines,omitempty"`
	ExcludeEngines []string `json:"exclude_engines,omitempty"`

	// Parsed operators (internal use)
	ParsedOperators interface{} `json:"-"`
	// Text with operators removed
	CleanedText string `json:"-"`
}

// NewQuery creates a new Query with defaults (sanitizes input)
func NewQuery(text string) *Query {
	text = strings.TrimSpace(text)
	return &Query{
		Text:     text,
		Category: CategoryGeneral,
		Language: "en",
		// Moderate by default
		SafeSearch:  1,
		Page:        1,
		PerPage:     20,
		TimeRange:   "any",
		SortBy:      SortRelevance,
		CleanedText: text,
	}
}

// Sanitize strips leading and trailing whitespace from all text fields
func (q *Query) Sanitize() {
	q.Text = strings.TrimSpace(q.Text)
	q.CleanedText = strings.TrimSpace(q.CleanedText)
	q.Language = strings.TrimSpace(q.Language)
	q.Region = strings.TrimSpace(q.Region)
	q.TimeRange = strings.TrimSpace(q.TimeRange)
	q.Site = strings.TrimSpace(q.Site)
	q.ExcludeSite = strings.TrimSpace(q.ExcludeSite)
	q.FileType = strings.TrimSpace(q.FileType)
	q.InURL = strings.TrimSpace(q.InURL)
	q.InTitle = strings.TrimSpace(q.InTitle)
	q.InText = strings.TrimSpace(q.InText)
	q.ExactTerms = strings.TrimSpace(q.ExactTerms)
	q.DateBefore = strings.TrimSpace(q.DateBefore)
	q.DateAfter = strings.TrimSpace(q.DateAfter)
	q.ImageSize = strings.TrimSpace(q.ImageSize)
	q.ImageType = strings.TrimSpace(q.ImageType)
	q.ImageColor = strings.TrimSpace(q.ImageColor)
	q.ImageAspect = strings.TrimSpace(q.ImageAspect)
	q.VideoLength = strings.TrimSpace(q.VideoLength)
	q.VideoQuality = strings.TrimSpace(q.VideoQuality)
	q.NewsSource = strings.TrimSpace(q.NewsSource)
}

// ValidSortOrders is a list of valid sort orders
var ValidSortOrders = []SortOrder{SortRelevance, SortDate, SortDateAsc, SortPopularity, SortRandom}

// IsValidSortOrder checks if a sort order is valid
func IsValidSortOrder(s SortOrder) bool {
	for _, v := range ValidSortOrders {
		if v == s {
			return true
		}
	}
	return false
}

// Validate checks if the query is valid (sanitizes first)
func (q *Query) ValidateSearchQuery() error {
	q.Sanitize()
	if q.Text == "" {
		return ErrEmptyQuery
	}

	if !q.Category.IsValid() {
		return ErrInvalidCategory
	}

	if q.Page < 1 {
		q.Page = 1
	}

	if q.PerPage < 1 {
		q.PerPage = 20
	}

	if q.PerPage > 100 {
		q.PerPage = 100
	}

	if q.SafeSearch < 0 || q.SafeSearch > 2 {
		q.SafeSearch = 1
	}

	if q.SortBy == "" || !IsValidSortOrder(q.SortBy) {
		q.SortBy = SortRelevance
	}

	// Initialize CleanedText if empty
	if q.CleanedText == "" {
		q.CleanedText = q.Text
	}

	return nil
}

// IsEmpty checks if the query text is empty
func (q *Query) IsEmpty() bool {
	return q.Text == ""
}

// HasAdvancedFilters checks if advanced filters are set
func (q *Query) HasAdvancedFilters() bool {
	return q.Site != "" ||
		q.ExcludeSite != "" ||
		q.FileType != "" ||
		len(q.FileTypes) > 0 ||
		q.InURL != "" ||
		q.InTitle != "" ||
		q.InText != "" ||
		q.ExactTerms != "" ||
		len(q.ExactPhrases) > 0 ||
		len(q.ExcludeTerms) > 0 ||
		q.DateBefore != "" ||
		q.DateAfter != ""
}

// HasMediaFilters checks if media-specific filters are set
func (q *Query) HasMediaFilters() bool {
	return q.ImageSize != "" ||
		q.ImageType != "" ||
		q.ImageColor != "" ||
		q.ImageAspect != "" ||
		q.VideoLength != "" ||
		q.VideoQuality != ""
}

// GetEffectiveText returns the text to use for searching (cleaned or original)
func (q *Query) GetEffectiveText() string {
	if q.CleanedText != "" {
		return q.CleanedText
	}
	return q.Text
}
