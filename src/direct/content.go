package direct

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/apimgr/search/src/version"
)

// WikiHandler handles wiki:{topic} queries
type WikiHandler struct {
	client *http.Client
}

// NewWikiHandler creates a new Wikipedia handler
func NewWikiHandler() *WikiHandler {
	return &WikiHandler{
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

func (h *WikiHandler) Type() AnswerType {
	return AnswerTypeWiki
}

func (h *WikiHandler) Handle(ctx context.Context, term string) (*Answer, error) {
	term = strings.TrimSpace(term)
	if term == "" {
		return nil, fmt.Errorf("topic required")
	}

	// Search Wikipedia API
	searchURL := fmt.Sprintf("https://en.wikipedia.org/api/rest_v1/page/summary/%s", url.PathEscape(term))

	req, err := http.NewRequestWithContext(ctx, "GET", searchURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", version.BrowserUserAgent)

	resp, err := h.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		// Try search instead
		return h.searchWikipedia(ctx, term)
	}

	var article struct {
		Title       string `json:"title"`
		DisplayTitle string `json:"displaytitle"`
		Extract     string `json:"extract"`
		ExtractHTML string `json:"extract_html"`
		Description string `json:"description"`
		Thumbnail   struct {
			Source string `json:"source"`
			Width  int    `json:"width"`
			Height int    `json:"height"`
		} `json:"thumbnail"`
		ContentURLs struct {
			Desktop struct {
				Page string `json:"page"`
			} `json:"desktop"`
		} `json:"content_urls"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&article); err != nil {
		return nil, err
	}

	data := map[string]interface{}{
		"title":       article.Title,
		"extract":     article.Extract,
		"description": article.Description,
		"url":         article.ContentURLs.Desktop.Page,
	}

	if article.Thumbnail.Source != "" {
		data["thumbnail"] = article.Thumbnail.Source
	}

	return &Answer{
		Type:        AnswerTypeWiki,
		Term:        term,
		Title:       article.Title,
		Description: article.Description,
		Content:     formatWikiContent(article.Title, article.ExtractHTML, article.Thumbnail.Source, article.ContentURLs.Desktop.Page),
		Source:      "Wikipedia",
		SourceURL:   article.ContentURLs.Desktop.Page,
		Data:        data,
	}, nil
}

func (h *WikiHandler) searchWikipedia(ctx context.Context, term string) (*Answer, error) {
	searchURL := fmt.Sprintf("https://en.wikipedia.org/w/api.php?action=query&list=search&srsearch=%s&format=json&utf8=1", url.QueryEscape(term))

	req, err := http.NewRequestWithContext(ctx, "GET", searchURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", version.BrowserUserAgent)

	resp, err := h.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Query struct {
			Search []struct {
				Title   string `json:"title"`
				Snippet string `json:"snippet"`
			} `json:"search"`
		} `json:"query"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	if len(result.Query.Search) == 0 {
		return &Answer{
			Type:        AnswerTypeWiki,
			Term:        term,
			Title:       fmt.Sprintf("Wiki: %s", term),
			Description: "No results found",
			Content:     fmt.Sprintf("<p>No Wikipedia article found for <code>%s</code>.</p>", escapeHTML(term)),
			Error:       "not_found",
		}, nil
	}

	// Return search results
	var html strings.Builder
	html.WriteString("<div class=\"wiki-search-results\">")
	html.WriteString(fmt.Sprintf("<h2>Search results for: %s</h2>", escapeHTML(term)))
	html.WriteString("<ul>")
	for _, result := range result.Query.Search {
		wikiURL := fmt.Sprintf("https://en.wikipedia.org/wiki/%s", url.PathEscape(result.Title))
		html.WriteString(fmt.Sprintf("<li><a href=\"%s\">%s</a><br>%s</li>",
			wikiURL, escapeHTML(result.Title), result.Snippet))
	}
	html.WriteString("</ul></div>")

	return &Answer{
		Type:        AnswerTypeWiki,
		Term:        term,
		Title:       fmt.Sprintf("Wiki: %s", term),
		Description: "Search results",
		Content:     html.String(),
		Source:      "Wikipedia",
		SourceURL:   fmt.Sprintf("https://en.wikipedia.org/wiki/Special:Search?search=%s", url.QueryEscape(term)),
	}, nil
}

func formatWikiContent(title, extractHTML, thumbnail, pageURL string) string {
	var html strings.Builder
	html.WriteString("<div class=\"wiki-content\">")

	if thumbnail != "" {
		html.WriteString(fmt.Sprintf("<img src=\"%s\" alt=\"%s\" class=\"wiki-thumbnail\">", escapeHTML(thumbnail), escapeHTML(title)))
	}

	html.WriteString(fmt.Sprintf("<h1>%s</h1>", escapeHTML(title)))
	html.WriteString("<div class=\"wiki-extract\">")
	html.WriteString(extractHTML)
	html.WriteString("</div>")

	html.WriteString(fmt.Sprintf("<p><a href=\"%s\" class=\"wiki-link\">Read full article on Wikipedia</a></p>", escapeHTML(pageURL)))
	html.WriteString("</div>")

	return html.String()
}

// DictHandler handles dict:{word} queries
type DictHandler struct {
	client *http.Client
}

// NewDictHandler creates a new dictionary handler
func NewDictHandler() *DictHandler {
	return &DictHandler{
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

func (h *DictHandler) Type() AnswerType {
	return AnswerTypeDict
}

func (h *DictHandler) Handle(ctx context.Context, term string) (*Answer, error) {
	term = strings.TrimSpace(strings.ToLower(term))
	if term == "" {
		return nil, fmt.Errorf("word required")
	}

	// Use Free Dictionary API
	apiURL := fmt.Sprintf("https://api.dictionaryapi.dev/api/v2/entries/en/%s", url.PathEscape(term))

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", version.BrowserUserAgent)

	resp, err := h.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return &Answer{
			Type:        AnswerTypeDict,
			Term:        term,
			Title:       fmt.Sprintf("Dictionary: %s", term),
			Description: "Word not found",
			Content:     fmt.Sprintf("<p>No definition found for <code>%s</code>.</p>", escapeHTML(term)),
			Error:       "not_found",
		}, nil
	}

	var entries []struct {
		Word      string `json:"word"`
		Phonetic  string `json:"phonetic"`
		Phonetics []struct {
			Text      string `json:"text"`
			Audio     string `json:"audio"`
			SourceURL string `json:"sourceUrl"`
		} `json:"phonetics"`
		Meanings []struct {
			PartOfSpeech string `json:"partOfSpeech"`
			Definitions  []struct {
				Definition string   `json:"definition"`
				Example    string   `json:"example"`
				Synonyms   []string `json:"synonyms"`
				Antonyms   []string `json:"antonyms"`
			} `json:"definitions"`
			Synonyms []string `json:"synonyms"`
			Antonyms []string `json:"antonyms"`
		} `json:"meanings"`
		Origin string `json:"origin"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return nil, err
	}

	if len(entries) == 0 {
		return &Answer{
			Type:        AnswerTypeDict,
			Term:        term,
			Title:       fmt.Sprintf("Dictionary: %s", term),
			Description: "Word not found",
			Content:     fmt.Sprintf("<p>No definition found for <code>%s</code>.</p>", escapeHTML(term)),
			Error:       "not_found",
		}, nil
	}

	entry := entries[0]

	// Find audio URL
	audioURL := ""
	for _, p := range entry.Phonetics {
		if p.Audio != "" {
			audioURL = p.Audio
			break
		}
	}

	data := map[string]interface{}{
		"word":      entry.Word,
		"phonetic":  entry.Phonetic,
		"meanings":  entry.Meanings,
		"origin":    entry.Origin,
		"audioURL":  audioURL,
	}

	return &Answer{
		Type:        AnswerTypeDict,
		Term:        term,
		Title:       entry.Word,
		Description: fmt.Sprintf("Dictionary entry for %s", entry.Word),
		Content:     formatDictContent(entry.Word, entry.Phonetic, audioURL, entry.Meanings, entry.Origin),
		Source:      "Free Dictionary API",
		SourceURL:   "https://dictionaryapi.dev/",
		Data:        data,
	}, nil
}

func formatDictContent(word, phonetic, audioURL string, meanings []struct {
	PartOfSpeech string `json:"partOfSpeech"`
	Definitions  []struct {
		Definition string   `json:"definition"`
		Example    string   `json:"example"`
		Synonyms   []string `json:"synonyms"`
		Antonyms   []string `json:"antonyms"`
	} `json:"definitions"`
	Synonyms []string `json:"synonyms"`
	Antonyms []string `json:"antonyms"`
}, origin string) string {
	var html strings.Builder
	html.WriteString("<div class=\"dict-content\">")

	// Word and pronunciation
	html.WriteString(fmt.Sprintf("<h1>%s</h1>", escapeHTML(word)))
	if phonetic != "" {
		html.WriteString(fmt.Sprintf("<p class=\"phonetic\">%s", escapeHTML(phonetic)))
		if audioURL != "" {
			html.WriteString(fmt.Sprintf(" <button onclick=\"new Audio('%s').play()\" class=\"audio-btn\" aria-label=\"Play pronunciation\">🔊</button>", escapeHTML(audioURL)))
		}
		html.WriteString("</p>")
	}

	// Meanings
	for _, meaning := range meanings {
		html.WriteString(fmt.Sprintf("<h2>%s</h2>", escapeHTML(meaning.PartOfSpeech)))
		html.WriteString("<ol class=\"definitions\">")

		for i, def := range meaning.Definitions {
			if i >= 5 {
				break // Limit to 5 definitions per part of speech
			}
			html.WriteString("<li>")
			html.WriteString(fmt.Sprintf("<p>%s</p>", escapeHTML(def.Definition)))
			if def.Example != "" {
				html.WriteString(fmt.Sprintf("<p class=\"example\"><em>\"%s\"</em></p>", escapeHTML(def.Example)))
			}
			html.WriteString("</li>")
		}

		html.WriteString("</ol>")

		// Synonyms for this meaning
		if len(meaning.Synonyms) > 0 {
			html.WriteString("<p class=\"synonyms\"><strong>Synonyms:</strong> ")
			for i, syn := range meaning.Synonyms {
				if i > 0 {
					html.WriteString(", ")
				}
				if i >= 10 {
					html.WriteString("...")
					break
				}
				html.WriteString(fmt.Sprintf("<a href=\"/direct/dict/%s\">%s</a>", url.PathEscape(syn), escapeHTML(syn)))
			}
			html.WriteString("</p>")
		}
	}

	// Etymology
	if origin != "" {
		html.WriteString("<h2>Etymology</h2>")
		html.WriteString(fmt.Sprintf("<p>%s</p>", escapeHTML(origin)))
	}

	html.WriteString("</div>")
	return html.String()
}

// ThesaurusHandler handles thesaurus:{word} queries
type ThesaurusHandler struct {
	client *http.Client
}

// NewThesaurusHandler creates a new thesaurus handler
func NewThesaurusHandler() *ThesaurusHandler {
	return &ThesaurusHandler{
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

func (h *ThesaurusHandler) Type() AnswerType {
	return AnswerTypeThesaurus
}

func (h *ThesaurusHandler) Handle(ctx context.Context, term string) (*Answer, error) {
	term = strings.TrimSpace(strings.ToLower(term))
	if term == "" {
		return nil, fmt.Errorf("word required")
	}

	// Use Datamuse API for synonyms and antonyms
	synURL := fmt.Sprintf("https://api.datamuse.com/words?rel_syn=%s&max=20", url.QueryEscape(term))
	antURL := fmt.Sprintf("https://api.datamuse.com/words?rel_ant=%s&max=20", url.QueryEscape(term))

	synonyms, _ := h.fetchWords(ctx, synURL)
	antonyms, _ := h.fetchWords(ctx, antURL)

	if len(synonyms) == 0 && len(antonyms) == 0 {
		return &Answer{
			Type:        AnswerTypeThesaurus,
			Term:        term,
			Title:       fmt.Sprintf("Thesaurus: %s", term),
			Description: "No results found",
			Content:     fmt.Sprintf("<p>No synonyms or antonyms found for <code>%s</code>.</p>", escapeHTML(term)),
			Error:       "not_found",
		}, nil
	}

	data := map[string]interface{}{
		"word":     term,
		"synonyms": synonyms,
		"antonyms": antonyms,
	}

	return &Answer{
		Type:        AnswerTypeThesaurus,
		Term:        term,
		Title:       fmt.Sprintf("Thesaurus: %s", term),
		Description: fmt.Sprintf("Synonyms and antonyms for %s", term),
		Content:     formatThesaurusContent(term, synonyms, antonyms),
		Source:      "Datamuse",
		SourceURL:   "https://www.datamuse.com/api/",
		Data:        data,
	}, nil
}

func (h *ThesaurusHandler) fetchWords(ctx context.Context, apiURL string) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", version.BrowserUserAgent)

	resp, err := h.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var results []struct {
		Word  string `json:"word"`
		Score int    `json:"score"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		return nil, err
	}

	words := make([]string, len(results))
	for i, r := range results {
		words[i] = r.Word
	}
	return words, nil
}

func formatThesaurusContent(word string, synonyms, antonyms []string) string {
	var html strings.Builder
	html.WriteString("<div class=\"thesaurus-content\">")
	html.WriteString(fmt.Sprintf("<h1>%s</h1>", escapeHTML(word)))

	if len(synonyms) > 0 {
		html.WriteString("<h2>Synonyms</h2>")
		html.WriteString("<div class=\"word-list synonyms\">")
		for _, syn := range synonyms {
			html.WriteString(fmt.Sprintf("<a href=\"/direct/thesaurus/%s\" class=\"word-tag\">%s</a> ",
				url.PathEscape(syn), escapeHTML(syn)))
		}
		html.WriteString("</div>")
	}

	if len(antonyms) > 0 {
		html.WriteString("<h2>Antonyms</h2>")
		html.WriteString("<div class=\"word-list antonyms\">")
		for _, ant := range antonyms {
			html.WriteString(fmt.Sprintf("<a href=\"/direct/thesaurus/%s\" class=\"word-tag\">%s</a> ",
				url.PathEscape(ant), escapeHTML(ant)))
		}
		html.WriteString("</div>")
	}

	html.WriteString("</div>")
	return html.String()
}

// PkgHandler handles pkg:{name} queries
type PkgHandler struct {
	client *http.Client
}

// NewPkgHandler creates a new package handler
func NewPkgHandler() *PkgHandler {
	return &PkgHandler{
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

func (h *PkgHandler) Type() AnswerType {
	return AnswerTypePkg
}

func (h *PkgHandler) Handle(ctx context.Context, term string) (*Answer, error) {
	term = strings.TrimSpace(term)
	if term == "" {
		return nil, fmt.Errorf("package name required")
	}

	// Detect registry from term format
	registry := ""
	packageName := term

	if strings.HasPrefix(term, "npm/") {
		registry = "npm"
		packageName = strings.TrimPrefix(term, "npm/")
	} else if strings.HasPrefix(term, "pip/") || strings.HasPrefix(term, "pypi/") {
		registry = "pypi"
		packageName = strings.TrimPrefix(strings.TrimPrefix(term, "pip/"), "pypi/")
	} else if strings.HasPrefix(term, "go/") {
		registry = "go"
		packageName = strings.TrimPrefix(term, "go/")
	} else if strings.Contains(term, "/") && !strings.HasPrefix(term, "@") {
		// Looks like a Go package
		registry = "go"
		packageName = term
	} else {
		// Default to npm
		registry = "npm"
	}

	switch registry {
	case "npm":
		return h.fetchNPM(ctx, packageName)
	case "pypi":
		return h.fetchPyPI(ctx, packageName)
	case "go":
		return h.fetchGo(ctx, packageName)
	default:
		return h.fetchNPM(ctx, packageName)
	}
}

func (h *PkgHandler) fetchNPM(ctx context.Context, name string) (*Answer, error) {
	apiURL := fmt.Sprintf("https://registry.npmjs.org/%s", url.PathEscape(name))

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", version.BrowserUserAgent)

	resp, err := h.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return &Answer{
			Type:        AnswerTypePkg,
			Term:        name,
			Title:       fmt.Sprintf("Package: %s", name),
			Description: "Package not found",
			Content:     fmt.Sprintf("<p>Package <code>%s</code> not found on npm.</p>", escapeHTML(name)),
			Error:       "not_found",
		}, nil
	}

	var pkg struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		DistTags    struct {
			Latest string `json:"latest"`
		} `json:"dist-tags"`
		License    string `json:"license"`
		Repository struct {
			URL string `json:"url"`
		} `json:"repository"`
		Homepage string `json:"homepage"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&pkg); err != nil {
		return nil, err
	}

	data := map[string]interface{}{
		"name":        pkg.Name,
		"description": pkg.Description,
		"version":     pkg.DistTags.Latest,
		"license":     pkg.License,
		"registry":    "npm",
	}

	return &Answer{
		Type:        AnswerTypePkg,
		Term:        name,
		Title:       pkg.Name,
		Description: pkg.Description,
		Content:     formatPkgContent("npm", pkg.Name, pkg.Description, pkg.DistTags.Latest, pkg.License, "npm install "+pkg.Name),
		Source:      "npm",
		SourceURL:   fmt.Sprintf("https://www.npmjs.com/package/%s", url.PathEscape(name)),
		Data:        data,
	}, nil
}

func (h *PkgHandler) fetchPyPI(ctx context.Context, name string) (*Answer, error) {
	apiURL := fmt.Sprintf("https://pypi.org/pypi/%s/json", url.PathEscape(name))

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", version.BrowserUserAgent)

	resp, err := h.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return &Answer{
			Type:        AnswerTypePkg,
			Term:        name,
			Title:       fmt.Sprintf("Package: %s", name),
			Description: "Package not found",
			Content:     fmt.Sprintf("<p>Package <code>%s</code> not found on PyPI.</p>", escapeHTML(name)),
			Error:       "not_found",
		}, nil
	}

	var pkg struct {
		Info struct {
			Name        string `json:"name"`
			Summary     string `json:"summary"`
			Version     string `json:"version"`
			License     string `json:"license"`
			ProjectURL  string `json:"project_url"`
			Homepage    string `json:"home_page"`
		} `json:"info"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&pkg); err != nil {
		return nil, err
	}

	data := map[string]interface{}{
		"name":        pkg.Info.Name,
		"description": pkg.Info.Summary,
		"version":     pkg.Info.Version,
		"license":     pkg.Info.License,
		"registry":    "pypi",
	}

	return &Answer{
		Type:        AnswerTypePkg,
		Term:        name,
		Title:       pkg.Info.Name,
		Description: pkg.Info.Summary,
		Content:     formatPkgContent("pypi", pkg.Info.Name, pkg.Info.Summary, pkg.Info.Version, pkg.Info.License, "pip install "+pkg.Info.Name),
		Source:      "PyPI",
		SourceURL:   fmt.Sprintf("https://pypi.org/project/%s/", url.PathEscape(name)),
		Data:        data,
	}, nil
}

func (h *PkgHandler) fetchGo(ctx context.Context, name string) (*Answer, error) {
	apiURL := fmt.Sprintf("https://pkg.go.dev/%s?tab=overview", url.PathEscape(name))

	// Go doesn't have a simple JSON API, return basic info
	return &Answer{
		Type:        AnswerTypePkg,
		Term:        name,
		Title:       name,
		Description: fmt.Sprintf("Go package: %s", name),
		Content:     formatPkgContent("go", name, "Go module", "", "", "go get "+name),
		Source:      "pkg.go.dev",
		SourceURL:   apiURL,
		Data: map[string]interface{}{
			"name":     name,
			"registry": "go",
		},
	}, nil
}

func formatPkgContent(registry, name, description, version, license, installCmd string) string {
	var html strings.Builder
	html.WriteString("<div class=\"pkg-content\">")
	html.WriteString(fmt.Sprintf("<h1>%s</h1>", escapeHTML(name)))
	html.WriteString(fmt.Sprintf("<p class=\"registry-badge\">%s</p>", escapeHTML(registry)))

	if description != "" {
		html.WriteString(fmt.Sprintf("<p class=\"description\">%s</p>", escapeHTML(description)))
	}

	html.WriteString("<dl class=\"pkg-details\">")
	if version != "" {
		html.WriteString(fmt.Sprintf("<dt>Version</dt><dd><code>%s</code></dd>", escapeHTML(version)))
	}
	if license != "" {
		html.WriteString(fmt.Sprintf("<dt>License</dt><dd>%s</dd>", escapeHTML(license)))
	}
	html.WriteString("</dl>")

	html.WriteString("<h2>Install</h2>")
	html.WriteString(fmt.Sprintf("<pre><code>%s</code></pre>", escapeHTML(installCmd)))
	html.WriteString("<button class=\"copy-btn\" onclick=\"copyCode(this)\">Copy</button>")

	html.WriteString("</div>")
	return html.String()
}

// CVEHandler handles cve:{id} queries
type CVEHandler struct {
	client *http.Client
}

// NewCVEHandler creates a new CVE handler
func NewCVEHandler() *CVEHandler {
	return &CVEHandler{
		client: &http.Client{Timeout: 15 * time.Second},
	}
}

func (h *CVEHandler) Type() AnswerType {
	return AnswerTypeCVE
}

func (h *CVEHandler) Handle(ctx context.Context, term string) (*Answer, error) {
	term = strings.TrimSpace(strings.ToUpper(term))
	if term == "" {
		return nil, fmt.Errorf("CVE ID required")
	}

	// Normalize CVE ID
	if !strings.HasPrefix(term, "CVE-") {
		term = "CVE-" + term
	}

	// Use NVD API
	apiURL := fmt.Sprintf("https://services.nvd.nist.gov/rest/json/cves/2.0?cveId=%s", url.QueryEscape(term))

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", version.BrowserUserAgent)

	resp, err := h.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Vulnerabilities []struct {
			CVE struct {
				ID          string `json:"id"`
				Description []struct {
					Lang  string `json:"lang"`
					Value string `json:"value"`
				} `json:"descriptions"`
				Metrics struct {
					CVSSV31 []struct {
						CVSS struct {
							BaseScore    float64 `json:"baseScore"`
							BaseSeverity string  `json:"baseSeverity"`
							VectorString string  `json:"vectorString"`
						} `json:"cvssData"`
					} `json:"cvssMetricV31"`
				} `json:"metrics"`
				Published    string `json:"published"`
				LastModified string `json:"lastModified"`
				References   []struct {
					URL string `json:"url"`
				} `json:"references"`
			} `json:"cve"`
		} `json:"vulnerabilities"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	if len(result.Vulnerabilities) == 0 {
		return &Answer{
			Type:        AnswerTypeCVE,
			Term:        term,
			Title:       term,
			Description: "CVE not found",
			Content:     fmt.Sprintf("<p>CVE <code>%s</code> not found in NVD database.</p>", escapeHTML(term)),
			Error:       "not_found",
		}, nil
	}

	cve := result.Vulnerabilities[0].CVE

	// Get English description
	description := ""
	for _, desc := range cve.Description {
		if desc.Lang == "en" {
			description = desc.Value
			break
		}
	}

	// Get CVSS score
	var cvssScore float64
	var severity string
	var vector string
	if len(cve.Metrics.CVSSV31) > 0 {
		cvssScore = cve.Metrics.CVSSV31[0].CVSS.BaseScore
		severity = cve.Metrics.CVSSV31[0].CVSS.BaseSeverity
		vector = cve.Metrics.CVSSV31[0].CVSS.VectorString
	}

	// Get references
	refs := make([]string, 0)
	for _, ref := range cve.References {
		if len(refs) < 5 {
			refs = append(refs, ref.URL)
		}
	}

	data := map[string]interface{}{
		"id":          cve.ID,
		"description": description,
		"cvss":        cvssScore,
		"severity":    severity,
		"vector":      vector,
		"published":   cve.Published,
		"modified":    cve.LastModified,
		"references":  refs,
	}

	return &Answer{
		Type:        AnswerTypeCVE,
		Term:        term,
		Title:       cve.ID,
		Description: description,
		Content:     formatCVEContent(cve.ID, description, cvssScore, severity, vector, cve.Published, refs),
		Source:      "NVD",
		SourceURL:   fmt.Sprintf("https://nvd.nist.gov/vuln/detail/%s", cve.ID),
		Data:        data,
	}, nil
}

func formatCVEContent(id, description string, cvss float64, severity, vector, published string, refs []string) string {
	var html strings.Builder
	html.WriteString("<div class=\"cve-content\">")
	html.WriteString(fmt.Sprintf("<h1>%s</h1>", escapeHTML(id)))

	// Severity badge
	severityClass := strings.ToLower(severity)
	if severityClass == "" {
		severityClass = "unknown"
	}
	html.WriteString(fmt.Sprintf("<p class=\"severity %s\">%s (CVSS %.1f)</p>", severityClass, severity, cvss))

	// Description
	html.WriteString(fmt.Sprintf("<p class=\"description\">%s</p>", escapeHTML(description)))

	// Details
	html.WriteString("<dl class=\"cve-details\">")
	if vector != "" {
		html.WriteString(fmt.Sprintf("<dt>CVSS Vector</dt><dd><code>%s</code></dd>", escapeHTML(vector)))
	}
	if published != "" {
		html.WriteString(fmt.Sprintf("<dt>Published</dt><dd>%s</dd>", escapeHTML(published)))
	}
	html.WriteString("</dl>")

	// References
	if len(refs) > 0 {
		html.WriteString("<h2>References</h2><ul>")
		for _, ref := range refs {
			html.WriteString(fmt.Sprintf("<li><a href=\"%s\" target=\"_blank\" rel=\"noopener\">%s</a></li>",
				escapeHTML(ref), escapeHTML(ref)))
		}
		html.WriteString("</ul>")
	}

	html.WriteString("</div>")
	return html.String()
}

// RFCHandler handles rfc:{number} queries
type RFCHandler struct {
	client *http.Client
}

// NewRFCHandler creates a new RFC handler
func NewRFCHandler() *RFCHandler {
	return &RFCHandler{
		client: &http.Client{Timeout: 15 * time.Second},
	}
}

func (h *RFCHandler) Type() AnswerType {
	return AnswerTypeRFC
}

func (h *RFCHandler) Handle(ctx context.Context, term string) (*Answer, error) {
	term = strings.TrimSpace(term)
	if term == "" {
		return nil, fmt.Errorf("RFC number required")
	}

	// Remove "RFC" prefix if present
	term = strings.TrimPrefix(strings.ToUpper(term), "RFC")
	term = strings.TrimSpace(term)

	// Fetch RFC info from IETF datatracker
	apiURL := fmt.Sprintf("https://datatracker.ietf.org/doc/rfc%s/doc.json", term)

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", version.BrowserUserAgent)

	resp, err := h.client.Do(req)
	if err != nil {
		// Fallback to basic info
		return h.basicRFCInfo(term)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return h.basicRFCInfo(term)
	}

	var doc struct {
		Title   string `json:"title"`
		Name    string `json:"name"`
		Abstract string `json:"abstract"`
		Authors []struct {
			Name string `json:"name"`
		} `json:"authors"`
		Time string `json:"time"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		return h.basicRFCInfo(term)
	}

	authors := make([]string, len(doc.Authors))
	for i, a := range doc.Authors {
		authors[i] = a.Name
	}

	data := map[string]interface{}{
		"number":   term,
		"title":    doc.Title,
		"abstract": doc.Abstract,
		"authors":  authors,
		"date":     doc.Time,
	}

	return &Answer{
		Type:        AnswerTypeRFC,
		Term:        term,
		Title:       fmt.Sprintf("RFC %s: %s", term, doc.Title),
		Description: doc.Abstract,
		Content:     formatRFCContent(term, doc.Title, doc.Abstract, authors, doc.Time),
		Source:      "IETF",
		SourceURL:   fmt.Sprintf("https://www.rfc-editor.org/rfc/rfc%s", term),
		Data:        data,
	}, nil
}

func (h *RFCHandler) basicRFCInfo(number string) (*Answer, error) {
	return &Answer{
		Type:        AnswerTypeRFC,
		Term:        number,
		Title:       fmt.Sprintf("RFC %s", number),
		Description: "RFC document",
		Content:     formatBasicRFCContent(number),
		Source:      "IETF",
		SourceURL:   fmt.Sprintf("https://www.rfc-editor.org/rfc/rfc%s", number),
	}, nil
}

func formatRFCContent(number, title, abstract string, authors []string, date string) string {
	var html strings.Builder
	html.WriteString("<div class=\"rfc-content\">")
	html.WriteString(fmt.Sprintf("<h1>RFC %s</h1>", escapeHTML(number)))
	html.WriteString(fmt.Sprintf("<h2>%s</h2>", escapeHTML(title)))

	if len(authors) > 0 {
		html.WriteString("<p class=\"authors\"><strong>Authors:</strong> ")
		html.WriteString(escapeHTML(strings.Join(authors, ", ")))
		html.WriteString("</p>")
	}

	if date != "" {
		html.WriteString(fmt.Sprintf("<p class=\"date\"><strong>Date:</strong> %s</p>", escapeHTML(date)))
	}

	if abstract != "" {
		html.WriteString("<h3>Abstract</h3>")
		html.WriteString(fmt.Sprintf("<p>%s</p>", escapeHTML(abstract)))
	}

	html.WriteString("<h3>Read Full Document</h3>")
	html.WriteString("<ul>")
	html.WriteString(fmt.Sprintf("<li><a href=\"https://www.rfc-editor.org/rfc/rfc%s.html\">HTML</a></li>", number))
	html.WriteString(fmt.Sprintf("<li><a href=\"https://www.rfc-editor.org/rfc/rfc%s.txt\">Plain Text</a></li>", number))
	html.WriteString(fmt.Sprintf("<li><a href=\"https://www.rfc-editor.org/rfc/rfc%s.pdf\">PDF</a></li>", number))
	html.WriteString("</ul>")

	html.WriteString("</div>")
	return html.String()
}

func formatBasicRFCContent(number string) string {
	var html strings.Builder
	html.WriteString("<div class=\"rfc-content\">")
	html.WriteString(fmt.Sprintf("<h1>RFC %s</h1>", escapeHTML(number)))

	html.WriteString("<h3>Read Full Document</h3>")
	html.WriteString("<ul>")
	html.WriteString(fmt.Sprintf("<li><a href=\"https://www.rfc-editor.org/rfc/rfc%s.html\">HTML</a></li>", number))
	html.WriteString(fmt.Sprintf("<li><a href=\"https://www.rfc-editor.org/rfc/rfc%s.txt\">Plain Text</a></li>", number))
	html.WriteString(fmt.Sprintf("<li><a href=\"https://www.rfc-editor.org/rfc/rfc%s.pdf\">PDF</a></li>", number))
	html.WriteString("</ul>")

	html.WriteString("</div>")
	return html.String()
}

// DirectoryHandler handles directory:{term} queries (open directory search)
type DirectoryHandler struct {
	client *http.Client
}

// NewDirectoryHandler creates a new directory search handler
func NewDirectoryHandler() *DirectoryHandler {
	return &DirectoryHandler{
		client: &http.Client{Timeout: 15 * time.Second},
	}
}

func (h *DirectoryHandler) Type() AnswerType {
	return AnswerTypeDirectory
}

func (h *DirectoryHandler) Handle(ctx context.Context, term string) (*Answer, error) {
	term = strings.TrimSpace(term)
	if term == "" {
		return nil, fmt.Errorf("search term required")
	}

	// Build open directory search query
	// This constructs a query to find open directory listings
	searchQuery := fmt.Sprintf(`intitle:"index of" OR intitle:"directory of" "%s" -html -htm -php -asp`, term)

	// Detect file type from term
	fileExt := ""
	lowerTerm := strings.ToLower(term)
	if strings.Contains(lowerTerm, "mp3") || strings.Contains(lowerTerm, "music") || strings.Contains(lowerTerm, "audio") {
		fileExt = "mp3 OR flac OR wav OR ogg"
	} else if strings.Contains(lowerTerm, "mp4") || strings.Contains(lowerTerm, "video") || strings.Contains(lowerTerm, "movie") {
		fileExt = "mp4 OR mkv OR avi OR webm"
	} else if strings.Contains(lowerTerm, "pdf") || strings.Contains(lowerTerm, "book") || strings.Contains(lowerTerm, "ebook") {
		fileExt = "pdf OR epub OR mobi"
	} else if strings.Contains(lowerTerm, "iso") {
		fileExt = "iso"
	}

	if fileExt != "" {
		searchQuery += " " + fileExt
	}

	data := map[string]interface{}{
		"term":  term,
		"query": searchQuery,
	}

	// Return the constructed search query for the user to use
	return &Answer{
		Type:        AnswerTypeDirectory,
		Term:        term,
		Title:       fmt.Sprintf("Directory Search: %s", term),
		Description: "Search for open directory listings",
		Content:     formatDirectoryContent(term, searchQuery),
		Source:      "Directory Search",
		Data:        data,
	}, nil
}

func formatDirectoryContent(term, query string) string {
	var html strings.Builder
	html.WriteString("<div class=\"directory-content\">")
	html.WriteString(fmt.Sprintf("<h1>Open Directory Search: %s</h1>", escapeHTML(term)))

	html.WriteString("<p>Open directories are web servers with directory listing enabled, allowing direct file access.</p>")

	html.WriteString("<h2>Search Query</h2>")
	html.WriteString("<p>Use this query in a search engine:</p>")
	html.WriteString(fmt.Sprintf("<pre><code>%s</code></pre>", escapeHTML(query)))
	html.WriteString("<button class=\"copy-btn\" onclick=\"copyCode(this)\">Copy</button>")

	html.WriteString("<h2>Quick Search</h2>")
	html.WriteString("<ul>")
	html.WriteString(fmt.Sprintf("<li><a href=\"https://www.google.com/search?q=%s\" target=\"_blank\" rel=\"noopener\">Search on Google</a></li>",
		url.QueryEscape(query)))
	html.WriteString(fmt.Sprintf("<li><a href=\"https://www.bing.com/search?q=%s\" target=\"_blank\" rel=\"noopener\">Search on Bing</a></li>",
		url.QueryEscape(query)))
	html.WriteString(fmt.Sprintf("<li><a href=\"https://duckduckgo.com/?q=%s\" target=\"_blank\" rel=\"noopener\">Search on DuckDuckGo</a></li>",
		url.QueryEscape(query)))
	html.WriteString("</ul>")

	html.WriteString("<h2>Tips</h2>")
	html.WriteString("<ul>")
	html.WriteString("<li>Add <code>site:edu</code> to search educational institutions</li>")
	html.WriteString("<li>Add file extensions like <code>mp3</code>, <code>pdf</code>, <code>iso</code></li>")
	html.WriteString("<li>Use <code>-</code> to exclude terms: <code>-ubuntu -mint</code></li>")
	html.WriteString("</ul>")

	html.WriteString("<p class=\"note\"><strong>Note:</strong> These are links to publicly accessible servers. Always respect copyright and terms of use.</p>")

	html.WriteString("</div>")
	return html.String()
}
