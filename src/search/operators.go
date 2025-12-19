package search

import (
	"regexp"
	"strings"
)

// SearchOperators contains parsed search operators from query text
type SearchOperators struct {
	// Original and cleaned query
	OriginalQuery string
	CleanedQuery  string

	// Site restriction
	Site       string   // site:example.com
	Sites      []string // Multiple sites
	ExcludeSite string   // -site:example.com

	// File type
	FileType  string   // filetype:pdf
	FileTypes []string // Multiple file types

	// URL filters
	InURL    string // inurl:keyword
	InTitle  string // intitle:keyword
	AllInURL string // allinurl:keyword1 keyword2

	// Content filters
	InText      string // intext:keyword
	AllInText   string // allintext:keyword1 keyword2
	InAnchor    string // inanchor:keyword
	AllInAnchor string // allinanchor:keyword1 keyword2

	// Exact match
	ExactPhrases []string // "exact phrase"

	// Exclusions
	ExcludeTerms []string // -word

	// Related/cache
	Related string // related:example.com
	Cache   string // cache:example.com
	Info    string // info:example.com

	// Date range
	DateRange  string // daterange:start-end
	Before     string // before:2024-01-01
	After      string // after:2023-01-01

	// Numeric range
	NumRange string // $100..$500 or 100..500

	// Boolean operators
	HasOR  bool // Contains OR operator
	HasAND bool // Contains AND operator

	// Special operators
	Define     string // define:word
	Weather    string // weather:city
	Stocks     string // stocks:AAPL
	Map        string // map:location
	Movie      string // movie:title
	Source     string // source:nytimes (news)
	Location   string // loc:city or location:city
	Language   string // lang:en

	// Wildcard
	HasWildcard bool // Contains * wildcard
}

// Operator patterns
var (
	sitePattern        = regexp.MustCompile(`(?i)site:(\S+)`)
	excludeSitePattern = regexp.MustCompile(`(?i)-site:(\S+)`)
	fileTypePattern    = regexp.MustCompile(`(?i)(?:filetype|ext):(\S+)`)
	inURLPattern       = regexp.MustCompile(`(?i)inurl:(\S+)`)
	allInURLPattern    = regexp.MustCompile(`(?i)allinurl:(.+?)(?:\s+(?:site:|filetype:|inurl:|intitle:|OR|AND|$))`)
	inTitlePattern     = regexp.MustCompile(`(?i)intitle:(\S+)`)
	allInTitlePattern  = regexp.MustCompile(`(?i)allintitle:(.+?)(?:\s+(?:site:|filetype:|inurl:|intitle:|OR|AND|$))`)
	inTextPattern      = regexp.MustCompile(`(?i)intext:(\S+)`)
	allInTextPattern   = regexp.MustCompile(`(?i)allintext:(.+?)(?:\s+(?:site:|filetype:|inurl:|intitle:|OR|AND|$))`)
	inAnchorPattern    = regexp.MustCompile(`(?i)inanchor:(\S+)`)
	allInAnchorPattern = regexp.MustCompile(`(?i)allinanchor:(.+?)(?:\s+(?:site:|filetype:|inurl:|intitle:|OR|AND|$))`)
	exactPhrasePattern = regexp.MustCompile(`"([^"]+)"`)
	excludeTermPattern = regexp.MustCompile(`(?:^|\s)-(\S+)`)
	relatedPattern     = regexp.MustCompile(`(?i)related:(\S+)`)
	cachePattern       = regexp.MustCompile(`(?i)cache:(\S+)`)
	infoPattern        = regexp.MustCompile(`(?i)info:(\S+)`)
	dateRangePattern   = regexp.MustCompile(`(?i)daterange:(\d+-\d+)`)
	beforePattern      = regexp.MustCompile(`(?i)before:(\S+)`)
	afterPattern       = regexp.MustCompile(`(?i)after:(\S+)`)
	numRangePattern    = regexp.MustCompile(`\$?(\d+)\.\.\$?(\d+)`)
	definePattern      = regexp.MustCompile(`(?i)define:(\S+)`)
	weatherPattern     = regexp.MustCompile(`(?i)weather:(\S+)`)
	stocksPattern      = regexp.MustCompile(`(?i)stocks:(\S+)`)
	mapPattern         = regexp.MustCompile(`(?i)map:(\S+)`)
	moviePattern       = regexp.MustCompile(`(?i)movie:(\S+)`)
	sourcePattern      = regexp.MustCompile(`(?i)source:(\S+)`)
	locationPattern    = regexp.MustCompile(`(?i)(?:loc|location):(\S+)`)
	langPattern        = regexp.MustCompile(`(?i)lang:(\S+)`)
	orPattern          = regexp.MustCompile(`\s+OR\s+`)
	andPattern         = regexp.MustCompile(`\s+AND\s+`)
	wildcardPattern    = regexp.MustCompile(`\*`)
)

// ParseOperators extracts search operators from query text
func ParseOperators(query string) *SearchOperators {
	ops := &SearchOperators{
		OriginalQuery: query,
		CleanedQuery:  query,
	}

	// Extract exact phrases first (preserve them)
	if matches := exactPhrasePattern.FindAllStringSubmatch(query, -1); matches != nil {
		for _, m := range matches {
			ops.ExactPhrases = append(ops.ExactPhrases, m[1])
		}
	}

	// Site operators
	if match := excludeSitePattern.FindStringSubmatch(query); match != nil {
		ops.ExcludeSite = match[1]
		ops.CleanedQuery = excludeSitePattern.ReplaceAllString(ops.CleanedQuery, "")
	}

	if matches := sitePattern.FindAllStringSubmatch(query, -1); matches != nil {
		for _, m := range matches {
			if ops.Site == "" {
				ops.Site = m[1]
			}
			ops.Sites = append(ops.Sites, m[1])
		}
		ops.CleanedQuery = sitePattern.ReplaceAllString(ops.CleanedQuery, "")
	}

	// File type
	if matches := fileTypePattern.FindAllStringSubmatch(query, -1); matches != nil {
		for _, m := range matches {
			if ops.FileType == "" {
				ops.FileType = m[1]
			}
			ops.FileTypes = append(ops.FileTypes, m[1])
		}
		ops.CleanedQuery = fileTypePattern.ReplaceAllString(ops.CleanedQuery, "")
	}

	// URL filters
	if match := allInURLPattern.FindStringSubmatch(query); match != nil {
		ops.AllInURL = strings.TrimSpace(match[1])
		ops.CleanedQuery = allInURLPattern.ReplaceAllString(ops.CleanedQuery, "")
	} else if match := inURLPattern.FindStringSubmatch(query); match != nil {
		ops.InURL = match[1]
		ops.CleanedQuery = inURLPattern.ReplaceAllString(ops.CleanedQuery, "")
	}

	// Title filters
	if match := allInTitlePattern.FindStringSubmatch(query); match != nil {
		ops.AllInURL = strings.TrimSpace(match[1])
		ops.CleanedQuery = allInTitlePattern.ReplaceAllString(ops.CleanedQuery, "")
	} else if match := inTitlePattern.FindStringSubmatch(query); match != nil {
		ops.InTitle = match[1]
		ops.CleanedQuery = inTitlePattern.ReplaceAllString(ops.CleanedQuery, "")
	}

	// Text filters
	if match := allInTextPattern.FindStringSubmatch(query); match != nil {
		ops.AllInText = strings.TrimSpace(match[1])
		ops.CleanedQuery = allInTextPattern.ReplaceAllString(ops.CleanedQuery, "")
	} else if match := inTextPattern.FindStringSubmatch(query); match != nil {
		ops.InText = match[1]
		ops.CleanedQuery = inTextPattern.ReplaceAllString(ops.CleanedQuery, "")
	}

	// Anchor filters
	if match := allInAnchorPattern.FindStringSubmatch(query); match != nil {
		ops.AllInAnchor = strings.TrimSpace(match[1])
		ops.CleanedQuery = allInAnchorPattern.ReplaceAllString(ops.CleanedQuery, "")
	} else if match := inAnchorPattern.FindStringSubmatch(query); match != nil {
		ops.InAnchor = match[1]
		ops.CleanedQuery = inAnchorPattern.ReplaceAllString(ops.CleanedQuery, "")
	}

	// Exclusions (after other operators to avoid false matches)
	if matches := excludeTermPattern.FindAllStringSubmatch(query, -1); matches != nil {
		for _, m := range matches {
			term := m[1]
			// Skip if it's an operator
			if !strings.Contains(term, ":") {
				ops.ExcludeTerms = append(ops.ExcludeTerms, term)
			}
		}
		// Only remove non-operator exclusions
		for _, term := range ops.ExcludeTerms {
			ops.CleanedQuery = strings.ReplaceAll(ops.CleanedQuery, "-"+term, "")
		}
	}

	// Related/cache/info
	if match := relatedPattern.FindStringSubmatch(query); match != nil {
		ops.Related = match[1]
		ops.CleanedQuery = relatedPattern.ReplaceAllString(ops.CleanedQuery, "")
	}
	if match := cachePattern.FindStringSubmatch(query); match != nil {
		ops.Cache = match[1]
		ops.CleanedQuery = cachePattern.ReplaceAllString(ops.CleanedQuery, "")
	}
	if match := infoPattern.FindStringSubmatch(query); match != nil {
		ops.Info = match[1]
		ops.CleanedQuery = infoPattern.ReplaceAllString(ops.CleanedQuery, "")
	}

	// Date filters
	if match := dateRangePattern.FindStringSubmatch(query); match != nil {
		ops.DateRange = match[1]
		ops.CleanedQuery = dateRangePattern.ReplaceAllString(ops.CleanedQuery, "")
	}
	if match := beforePattern.FindStringSubmatch(query); match != nil {
		ops.Before = match[1]
		ops.CleanedQuery = beforePattern.ReplaceAllString(ops.CleanedQuery, "")
	}
	if match := afterPattern.FindStringSubmatch(query); match != nil {
		ops.After = match[1]
		ops.CleanedQuery = afterPattern.ReplaceAllString(ops.CleanedQuery, "")
	}

	// Numeric range
	if match := numRangePattern.FindStringSubmatch(query); match != nil {
		ops.NumRange = match[0]
	}

	// Special operators
	if match := definePattern.FindStringSubmatch(query); match != nil {
		ops.Define = match[1]
		ops.CleanedQuery = definePattern.ReplaceAllString(ops.CleanedQuery, "")
	}
	if match := weatherPattern.FindStringSubmatch(query); match != nil {
		ops.Weather = match[1]
		ops.CleanedQuery = weatherPattern.ReplaceAllString(ops.CleanedQuery, "")
	}
	if match := stocksPattern.FindStringSubmatch(query); match != nil {
		ops.Stocks = match[1]
		ops.CleanedQuery = stocksPattern.ReplaceAllString(ops.CleanedQuery, "")
	}
	if match := mapPattern.FindStringSubmatch(query); match != nil {
		ops.Map = match[1]
		ops.CleanedQuery = mapPattern.ReplaceAllString(ops.CleanedQuery, "")
	}
	if match := moviePattern.FindStringSubmatch(query); match != nil {
		ops.Movie = match[1]
		ops.CleanedQuery = moviePattern.ReplaceAllString(ops.CleanedQuery, "")
	}
	if match := sourcePattern.FindStringSubmatch(query); match != nil {
		ops.Source = match[1]
		ops.CleanedQuery = sourcePattern.ReplaceAllString(ops.CleanedQuery, "")
	}
	if match := locationPattern.FindStringSubmatch(query); match != nil {
		ops.Location = match[1]
		ops.CleanedQuery = locationPattern.ReplaceAllString(ops.CleanedQuery, "")
	}
	if match := langPattern.FindStringSubmatch(query); match != nil {
		ops.Language = match[1]
		ops.CleanedQuery = langPattern.ReplaceAllString(ops.CleanedQuery, "")
	}

	// Boolean operators
	ops.HasOR = orPattern.MatchString(query)
	ops.HasAND = andPattern.MatchString(query)
	ops.HasWildcard = wildcardPattern.MatchString(query)

	// Clean up the query
	ops.CleanedQuery = strings.TrimSpace(ops.CleanedQuery)
	ops.CleanedQuery = regexp.MustCompile(`\s+`).ReplaceAllString(ops.CleanedQuery, " ")

	return ops
}

// HasOperators returns true if any operators were found
func (ops *SearchOperators) HasOperators() bool {
	return ops.Site != "" ||
		ops.ExcludeSite != "" ||
		ops.FileType != "" ||
		ops.InURL != "" ||
		ops.InTitle != "" ||
		ops.InText != "" ||
		ops.InAnchor != "" ||
		len(ops.ExactPhrases) > 0 ||
		len(ops.ExcludeTerms) > 0 ||
		ops.Related != "" ||
		ops.Cache != "" ||
		ops.Info != "" ||
		ops.DateRange != "" ||
		ops.Before != "" ||
		ops.After != "" ||
		ops.Define != "" ||
		ops.Weather != "" ||
		ops.Stocks != "" ||
		ops.Map != "" ||
		ops.Movie != "" ||
		ops.Source != "" ||
		ops.Location != "" ||
		ops.Language != "" ||
		ops.HasOR ||
		ops.HasAND
}

// ToGoogleQuery converts operators to Google-compatible query string
func (ops *SearchOperators) ToGoogleQuery() string {
	parts := []string{}

	if ops.CleanedQuery != "" {
		parts = append(parts, ops.CleanedQuery)
	}

	if ops.Site != "" {
		parts = append(parts, "site:"+ops.Site)
	}
	if ops.ExcludeSite != "" {
		parts = append(parts, "-site:"+ops.ExcludeSite)
	}
	if ops.FileType != "" {
		parts = append(parts, "filetype:"+ops.FileType)
	}
	if ops.InURL != "" {
		parts = append(parts, "inurl:"+ops.InURL)
	}
	if ops.InTitle != "" {
		parts = append(parts, "intitle:"+ops.InTitle)
	}
	if ops.InText != "" {
		parts = append(parts, "intext:"+ops.InText)
	}
	for _, phrase := range ops.ExactPhrases {
		parts = append(parts, `"`+phrase+`"`)
	}
	for _, term := range ops.ExcludeTerms {
		parts = append(parts, "-"+term)
	}
	if ops.Before != "" {
		parts = append(parts, "before:"+ops.Before)
	}
	if ops.After != "" {
		parts = append(parts, "after:"+ops.After)
	}

	return strings.Join(parts, " ")
}

// ToDuckDuckGoQuery converts operators to DuckDuckGo-compatible query string
func (ops *SearchOperators) ToDuckDuckGoQuery() string {
	parts := []string{}

	if ops.CleanedQuery != "" {
		parts = append(parts, ops.CleanedQuery)
	}

	// DuckDuckGo supports: site:, filetype:, intitle:, -term, "exact"
	if ops.Site != "" {
		parts = append(parts, "site:"+ops.Site)
	}
	if ops.ExcludeSite != "" {
		parts = append(parts, "-site:"+ops.ExcludeSite)
	}
	if ops.FileType != "" {
		parts = append(parts, "filetype:"+ops.FileType)
	}
	if ops.InTitle != "" {
		parts = append(parts, "intitle:"+ops.InTitle)
	}
	for _, phrase := range ops.ExactPhrases {
		parts = append(parts, `"`+phrase+`"`)
	}
	for _, term := range ops.ExcludeTerms {
		parts = append(parts, "-"+term)
	}

	return strings.Join(parts, " ")
}

// ToBingQuery converts operators to Bing-compatible query string
func (ops *SearchOperators) ToBingQuery() string {
	parts := []string{}

	if ops.CleanedQuery != "" {
		parts = append(parts, ops.CleanedQuery)
	}

	// Bing supports: site:, filetype:, intitle:, inurl:, -term, "exact"
	if ops.Site != "" {
		parts = append(parts, "site:"+ops.Site)
	}
	if ops.FileType != "" {
		parts = append(parts, "filetype:"+ops.FileType)
	}
	if ops.InTitle != "" {
		parts = append(parts, "intitle:"+ops.InTitle)
	}
	if ops.InURL != "" {
		parts = append(parts, "inurl:"+ops.InURL)
	}
	for _, phrase := range ops.ExactPhrases {
		parts = append(parts, `"`+phrase+`"`)
	}
	for _, term := range ops.ExcludeTerms {
		parts = append(parts, "-"+term)
	}

	return strings.Join(parts, " ")
}

// ToBasicQuery returns the cleaned query without operators for engines
// that don't support advanced operators
func (ops *SearchOperators) ToBasicQuery() string {
	query := ops.CleanedQuery

	// Add exact phrases back
	for _, phrase := range ops.ExactPhrases {
		query += ` "` + phrase + `"`
	}

	return strings.TrimSpace(query)
}
