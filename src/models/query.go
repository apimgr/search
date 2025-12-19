package models

// SortOrder defines how results are sorted
type SortOrder string

const (
	SortRelevance  SortOrder = "relevance"  // Default: by score
	SortDate       SortOrder = "date"       // By date (newest first)
	SortDateAsc    SortOrder = "date_asc"   // By date (oldest first)
	SortPopularity SortOrder = "popularity" // By popularity/engagement
	SortRandom     SortOrder = "random"     // Random order
)

// Query represents a search query
type Query struct {
	// User input
	Text string `json:"text"`

	// Filters
	Category   Category `json:"category"`
	Language   string   `json:"language"`
	Region     string   `json:"region,omitempty"`     // Region code (us, uk, de, etc.)
	SafeSearch int      `json:"safe_search"`          // 0: off, 1: moderate, 2: strict

	// Pagination
	Page    int `json:"page"`
	PerPage int `json:"per_page"`

	// Sorting
	SortBy SortOrder `json:"sort_by,omitempty"`

	// Time range
	TimeRange string `json:"time_range,omitempty"` // any, day, week, month, year

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
	DateBefore string `json:"date_before,omitempty"` // YYYY-MM-DD
	DateAfter  string `json:"date_after,omitempty"`  // YYYY-MM-DD

	// Media-specific filters
	ImageSize   string `json:"image_size,omitempty"`   // small, medium, large, xlarge
	ImageType   string `json:"image_type,omitempty"`   // photo, clipart, lineart, animated
	ImageColor  string `json:"image_color,omitempty"`  // color, gray, trans, red, etc.
	ImageAspect string `json:"image_aspect,omitempty"` // square, wide, tall
	VideoLength string `json:"video_length,omitempty"` // short, medium, long
	VideoQuality string `json:"video_quality,omitempty"` // hd, 4k

	// News-specific
	NewsSource string `json:"news_source,omitempty"` // source:nytimes

	// Engine selection
	Engines        []string `json:"engines,omitempty"`
	ExcludeEngines []string `json:"exclude_engines,omitempty"`

	// Parsed operators (internal use)
	ParsedOperators interface{} `json:"-"`
	CleanedText     string      `json:"-"` // Text with operators removed
}

// NewQuery creates a new Query with defaults
func NewQuery(text string) *Query {
	return &Query{
		Text:        text,
		Category:    CategoryGeneral,
		Language:    "en",
		SafeSearch:  1, // Moderate by default
		Page:        1,
		PerPage:     20,
		TimeRange:   "any",
		SortBy:      SortRelevance,
		CleanedText: text,
	}
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

// Validate checks if the query is valid
func (q *Query) Validate() error {
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
