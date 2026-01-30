package engines

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/apimgr/search/src/model"
	"github.com/apimgr/search/src/search"
)

// PubMed implements PubMed search engine using NCBI E-utilities API
type PubMed struct {
	*search.BaseEngine
	client *http.Client
}

// NewPubMed creates a new PubMed search engine
func NewPubMed() *PubMed {
	config := model.NewEngineConfig("pubmed")
	config.DisplayName = "PubMed"
	config.Priority = 60
	config.Categories = []string{"science"}
	config.SupportsTor = true

	return &PubMed{
		BaseEngine: search.NewBaseEngine(config),
		client: &http.Client{
			Timeout: time.Duration(config.GetTimeout()) * time.Second,
		},
	}
}

// PubMed E-utilities response structures

// esearchResult represents the response from esearch.fcgi
type esearchResult struct {
	XMLName xml.Name `xml:"eSearchResult"`
	Count   int      `xml:"Count"`
	RetMax  int      `xml:"RetMax"`
	RetStart int     `xml:"RetStart"`
	IdList  struct {
		Ids []string `xml:"Id"`
	} `xml:"IdList"`
}

// efetchResult represents the response from efetch.fcgi for PubMed articles
type efetchResult struct {
	XMLName  xml.Name        `xml:"PubmedArticleSet"`
	Articles []pubmedArticle `xml:"PubmedArticle"`
}

type pubmedArticle struct {
	MedlineCitation struct {
		PMID struct {
			Value string `xml:",chardata"`
		} `xml:"PMID"`
		Article struct {
			Journal struct {
				Title string `xml:"Title"`
				JournalIssue struct {
					PubDate struct {
						Year  string `xml:"Year"`
						Month string `xml:"Month"`
						Day   string `xml:"Day"`
					} `xml:"PubDate"`
				} `xml:"JournalIssue"`
			} `xml:"Journal"`
			ArticleTitle string `xml:"ArticleTitle"`
			Abstract     struct {
				AbstractText []abstractText `xml:"AbstractText"`
			} `xml:"Abstract"`
			AuthorList struct {
				Authors []struct {
					LastName string `xml:"LastName"`
					ForeName string `xml:"ForeName"`
				} `xml:"Author"`
			} `xml:"AuthorList"`
		} `xml:"Article"`
	} `xml:"MedlineCitation"`
}

type abstractText struct {
	Label string `xml:"Label,attr"`
	Text  string `xml:",chardata"`
}

// Search performs a PubMed search
func (e *PubMed) Search(ctx context.Context, query *model.Query) ([]model.Result, error) {
	// Step 1: Search for article IDs using esearch
	ids, err := e.searchIDs(ctx, query)
	if err != nil {
		return nil, err
	}

	if len(ids) == 0 {
		return []model.Result{}, nil
	}

	// Step 2: Fetch article details using efetch
	articles, err := e.fetchArticles(ctx, ids)
	if err != nil {
		return nil, err
	}

	// Step 3: Convert to search results
	results := make([]model.Result, 0)
	for i, article := range articles {
		if i >= e.GetConfig().GetMaxResults() {
			break
		}

		result := e.articleToResult(article, i)
		results = append(results, result)
	}

	return results, nil
}

// searchIDs searches PubMed and returns article IDs
func (e *PubMed) searchIDs(ctx context.Context, query *model.Query) ([]string, error) {
	baseURL := "https://eutils.ncbi.nlm.nih.gov/entrez/eutils/esearch.fcgi"

	params := url.Values{}
	params.Set("db", "pubmed")
	params.Set("term", query.Text)
	params.Set("retmode", "xml")
	params.Set("retmax", "20")
	params.Set("sort", "relevance")

	// Handle pagination
	if query.Page > 1 {
		retstart := (query.Page - 1) * 20
		params.Set("retstart", fmt.Sprintf("%d", retstart))
	}

	// Handle time range
	switch query.TimeRange {
	case "day":
		params.Set("datetype", "pdat")
		params.Set("reldate", "1")
	case "week":
		params.Set("datetype", "pdat")
		params.Set("reldate", "7")
	case "month":
		params.Set("datetype", "pdat")
		params.Set("reldate", "30")
	case "year":
		params.Set("datetype", "pdat")
		params.Set("reldate", "365")
	}

	reqURL := fmt.Sprintf("%s?%s", baseURL, params.Encode())

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Accept", "application/xml")

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("PubMed esearch returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var searchResult esearchResult
	if err := xml.Unmarshal(body, &searchResult); err != nil {
		return nil, fmt.Errorf("failed to parse PubMed esearch response: %w", err)
	}

	return searchResult.IdList.Ids, nil
}

// fetchArticles fetches article details for the given IDs
func (e *PubMed) fetchArticles(ctx context.Context, ids []string) ([]pubmedArticle, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	baseURL := "https://eutils.ncbi.nlm.nih.gov/entrez/eutils/efetch.fcgi"

	params := url.Values{}
	params.Set("db", "pubmed")
	params.Set("id", strings.Join(ids, ","))
	params.Set("retmode", "xml")
	params.Set("rettype", "abstract")

	reqURL := fmt.Sprintf("%s?%s", baseURL, params.Encode())

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Accept", "application/xml")

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("PubMed efetch returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var fetchResult efetchResult
	if err := xml.Unmarshal(body, &fetchResult); err != nil {
		return nil, fmt.Errorf("failed to parse PubMed efetch response: %w", err)
	}

	return fetchResult.Articles, nil
}

// articleToResult converts a PubMed article to a search result
func (e *PubMed) articleToResult(article pubmedArticle, position int) model.Result {
	citation := article.MedlineCitation
	pmid := citation.PMID.Value
	title := citation.Article.ArticleTitle

	// Build abstract content
	content := e.buildAbstract(citation.Article.Abstract.AbstractText)

	// Build author string
	author := e.buildAuthorString(citation.Article.AuthorList.Authors)

	// Parse publication date
	pubDate := e.parsePublicationDate(citation.Article.Journal.JournalIssue.PubDate)

	// Add journal info to content
	journal := citation.Article.Journal.Title
	if journal != "" {
		if content != "" {
			content = fmt.Sprintf("[%s] %s", journal, content)
		} else {
			content = fmt.Sprintf("[%s]", journal)
		}
	}

	// PubMed URL
	pubmedURL := fmt.Sprintf("https://pubmed.ncbi.nlm.nih.gov/%s/", pmid)

	return model.Result{
		Title:       cleanPubMedText(title),
		URL:         pubmedURL,
		Content:     cleanPubMedText(content),
		Engine:      e.Name(),
		Category:    model.CategoryScience,
		Author:      author,
		PublishedAt: pubDate,
		Score:       calculateScore(e.GetPriority(), position, 1),
		Position:    position,
	}
}

// buildAbstract combines abstract text elements into a single string
func (e *PubMed) buildAbstract(texts []abstractText) string {
	if len(texts) == 0 {
		return ""
	}

	var parts []string
	for _, t := range texts {
		text := strings.TrimSpace(t.Text)
		if text == "" {
			continue
		}
		if t.Label != "" {
			parts = append(parts, fmt.Sprintf("%s: %s", t.Label, text))
		} else {
			parts = append(parts, text)
		}
	}

	abstract := strings.Join(parts, " ")

	// Truncate if too long
	if len(abstract) > 500 {
		abstract = abstract[:497] + "..."
	}

	return abstract
}

// buildAuthorString builds a formatted author string
func (e *PubMed) buildAuthorString(authors []struct {
	LastName string `xml:"LastName"`
	ForeName string `xml:"ForeName"`
}) string {
	if len(authors) == 0 {
		return ""
	}

	var authorNames []string
	for i, author := range authors {
		if i >= 3 {
			authorNames = append(authorNames, "et al.")
			break
		}
		name := strings.TrimSpace(author.LastName)
		if author.ForeName != "" {
			// Get initials
			forename := strings.TrimSpace(author.ForeName)
			if len(forename) > 0 {
				name = fmt.Sprintf("%s %c", name, forename[0])
			}
		}
		if name != "" {
			authorNames = append(authorNames, name)
		}
	}

	return strings.Join(authorNames, ", ")
}

// parsePublicationDate parses the publication date from PubMed format
func (e *PubMed) parsePublicationDate(pubDate struct {
	Year  string `xml:"Year"`
	Month string `xml:"Month"`
	Day   string `xml:"Day"`
}) time.Time {
	year := pubDate.Year
	month := pubDate.Month
	day := pubDate.Day

	if year == "" {
		return time.Time{}
	}

	// Try parsing with different formats
	dateStr := year
	format := "2006"

	if month != "" {
		// Month could be numeric or text
		dateStr = year + " " + month
		format = "2006 January"
		// Try numeric format first
		if len(month) <= 2 {
			format = "2006 01"
		}
	}

	if day != "" {
		dateStr = dateStr + " " + day
		if len(month) <= 2 {
			format = format + " 02"
		} else {
			format = format + " 2"
		}
	}

	t, err := time.Parse(format, dateStr)
	if err != nil {
		// Fallback: try year only
		t, _ = time.Parse("2006", year)
	}

	return t
}

// cleanPubMedText cleans text from PubMed XML
func cleanPubMedText(text string) string {
	// Remove extra whitespace
	text = strings.Join(strings.Fields(text), " ")

	// Unescape HTML entities
	text = strings.ReplaceAll(text, "&amp;", "&")
	text = strings.ReplaceAll(text, "&lt;", "<")
	text = strings.ReplaceAll(text, "&gt;", ">")
	text = strings.ReplaceAll(text, "&quot;", "\"")
	text = strings.ReplaceAll(text, "&#39;", "'")
	text = strings.ReplaceAll(text, "&apos;", "'")

	return strings.TrimSpace(text)
}
