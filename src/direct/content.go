package direct

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/mod/modfile"

	"github.com/apimgr/search/src/common/closeutil"
	"github.com/apimgr/search/src/common/i18n"
	"github.com/apimgr/search/src/version"
	"github.com/apimgr/search/src/widget"
	xhtml "golang.org/x/net/html"
)

func htmlText(raw string) string {
	var output strings.Builder
	tokenizer := xhtml.NewTokenizer(strings.NewReader(raw))
	for {
		tokenType := tokenizer.Next()
		if tokenType == xhtml.ErrorToken {
			break
		}
		if tokenType == xhtml.TextToken {
			output.Write(tokenizer.Text())
		}
	}
	return strings.Join(strings.Fields(output.String()), " ")
}

// WikiHandler handles wiki:{topic} queries
type WikiHandler struct {
	client *http.Client
}

var directWikiLanguagePattern = regexp.MustCompile(`^[a-z]{2,3}$`)

// NewWikiHandler creates a new Wikipedia handler
func NewWikiHandler() *WikiHandler {
	return &WikiHandler{
		client: newPublicHTTPClient(10 * time.Second),
	}
}

func (h *WikiHandler) Type() AnswerType {
	return AnswerTypeWiki
}

func (h *WikiHandler) HandleDirectQuery(ctx context.Context, term string) (*Answer, error) {
	term = strings.TrimSpace(term)
	if term == "" {
		return nil, fmt.Errorf("topic required")
	}
	lang := LangFromContext(ctx)
	if !directWikiLanguagePattern.MatchString(lang) {
		lang = "en"
	}
	ctx = WithLang(ctx, lang)

	// Search Wikipedia API
	searchURL := fmt.Sprintf("https://%s.wikipedia.org/api/rest_v1/page/summary/%s", lang, url.PathEscape(term))

	req, err := http.NewRequestWithContext(ctx, "GET", searchURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", version.BrowserUserAgent)

	resp, err := h.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer closeutil.Close(resp.Body)

	if resp.StatusCode == http.StatusNotFound {
		// Try search instead
		return h.searchWikipedia(ctx, term)
	}

	var article struct {
		Title        string `json:"title"`
		DisplayTitle string `json:"displaytitle"`
		Extract      string `json:"extract"`
		ExtractHTML  string `json:"extract_html"`
		Description  string `json:"description"`
		Thumbnail    struct {
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

	if err := decodeBoundedJSON(resp.Body, 2<<20, &article); err != nil {
		return nil, err
	}

	pageURL := article.ContentURLs.Desktop.Page
	if !validWikipediaURL(pageURL) {
		pageURL = fmt.Sprintf("https://%s.wikipedia.org/wiki/%s", lang, url.PathEscape(article.Title))
	}
	thumbnailURL := article.Thumbnail.Source
	if !validWikipediaThumbnailURL(thumbnailURL) {
		thumbnailURL = ""
	}
	data := map[string]interface{}{
		"title":       article.Title,
		"extract":     article.Extract,
		"description": article.Description,
		"url":         pageURL,
	}

	if thumbnailURL != "" {
		data["thumbnail"] = thumbnailURL
	}
	data["language"] = lang
	articleDetails := &widget.WikipediaArticleDetails{}
	if fetchedDetails, detailsErr := widget.FetchWikipediaArticleDetails(ctx, h.client, article.Title, lang); detailsErr == nil && fetchedDetails != nil {
		articleDetails = fetchedDetails
		data["sections"] = articleDetails.Sections
		data["infobox"] = articleDetails.Infobox
		data["related_articles"] = articleDetails.RelatedArticles
	}
	renderDetails := &wikiArticleDetails{
		Query:           "wiki:" + term,
		Language:        lang,
		Sections:        articleDetails.Sections,
		Infobox:         articleDetails.Infobox,
		RelatedArticles: articleDetails.RelatedArticles,
	}

	return &Answer{
		Type:        AnswerTypeWiki,
		Term:        term,
		Title:       article.Title,
		Description: article.Description,
		Content:     formatWikiContent(article.Title, article.ExtractHTML, thumbnailURL, pageURL, renderDetails),
		Source:      "Wikipedia",
		SourceURL:   pageURL,
		Data:        data,
	}, nil
}

func (h *WikiHandler) searchWikipedia(ctx context.Context, term string) (*Answer, error) {
	lang := LangFromContext(ctx)

	searchURL := fmt.Sprintf("https://%s.wikipedia.org/w/api.php?action=query&list=search&srsearch=%s&format=json&utf8=1", lang, url.QueryEscape(term))

	req, err := http.NewRequestWithContext(ctx, "GET", searchURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", version.BrowserUserAgent)

	resp, err := h.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer closeutil.Close(resp.Body)

	var result struct {
		Query struct {
			Search []struct {
				Title   string `json:"title"`
				Snippet string `json:"snippet"`
			} `json:"search"`
		} `json:"query"`
	}

	if err := decodeBoundedJSON(resp.Body, 2<<20, &result); err != nil {
		return nil, err
	}

	if len(result.Query.Search) == 0 {
		return &Answer{
			Type:        AnswerTypeWiki,
			Term:        term,
			Title:       fmt.Sprintf("Wiki: %s", term),
			Description: i18n.T(lang, "direct.no_results_found"),
			Content:     fmt.Sprintf("<p>No Wikipedia article found for <code>%s</code>.</p>", escapeHTML(term)),
			Error:       "not_found",
		}, nil
	}
	// Return search results
	var html strings.Builder
	html.WriteString("<div class=\"wiki-search-results\">")
	fmt.Fprintf(&html, "<h2>%s %s</h2>", i18n.T(lang, "direct.search_results_for"), escapeHTML(term))
	html.WriteString("<ul>")
	for _, result := range result.Query.Search {
		wikiURL := fmt.Sprintf("https://%s.wikipedia.org/wiki/%s", lang, url.PathEscape(result.Title))
		fmt.Fprintf(&html, "<li><a href=\"%s\">%s</a><br>%s</li>",
			wikiURL, escapeHTML(result.Title), sanitizeManHTML(result.Snippet))
	}
	html.WriteString("</ul></div>")

	return &Answer{
		Type:        AnswerTypeWiki,
		Term:        term,
		Title:       fmt.Sprintf("Wiki: %s", term),
		Description: i18n.T(lang, "direct.search_results"),
		Content:     html.String(),
		Source:      "Wikipedia",
		SourceURL:   fmt.Sprintf("https://%s.wikipedia.org/wiki/Special:Search?search=%s", lang, url.QueryEscape(term)),
	}, nil
}

func validWikipediaURL(rawURL string) bool {
	parsed, err := url.Parse(rawURL)
	return err == nil && parsed.Scheme == "https" && parsed.User == nil && (parsed.Hostname() == "wikipedia.org" || strings.HasSuffix(parsed.Hostname(), ".wikipedia.org"))
}

func validWikipediaThumbnailURL(rawURL string) bool {
	parsed, err := url.Parse(rawURL)
	return err == nil && parsed.Scheme == "https" && parsed.User == nil && parsed.Hostname() == "upload.wikimedia.org"
}

type wikiArticleDetails struct {
	Query           string
	Language        string
	Sections        []widget.WikipediaSection
	Infobox         []widget.WikipediaField
	RelatedArticles []widget.RelatedArticle
}

func formatWikiContent(title, extractHTML, thumbnail, pageURL string, details ...*wikiArticleDetails) string {
	var html strings.Builder
	html.WriteString("<div class=\"wiki-content\">")

	if validWikipediaThumbnailURL(thumbnail) {
		fmt.Fprintf(&html, "<img src=\"%s\" alt=\"%s\" class=\"wiki-thumbnail\">", escapeHTML(thumbnail), escapeHTML(title))
	}

	fmt.Fprintf(&html, "<h1>%s</h1>", escapeHTML(title))
	html.WriteString("<div class=\"wiki-extract\">")
	html.WriteString(sanitizeManHTML(extractHTML))
	html.WriteString("</div>")
	if len(details) > 0 && details[0] != nil {
		writeWikiArticleDetails(&html, details[0])
	}

	if validWikipediaURL(pageURL) {
		fmt.Fprintf(&html, "<p><a href=\"%s\" class=\"wiki-link\">Read full article on Wikipedia</a></p>", escapeHTML(pageURL))
	}
	html.WriteString("</div>")

	return html.String()
}

func writeWikiArticleDetails(output *strings.Builder, details *wikiArticleDetails) {
	if len(details.Infobox) > 0 {
		output.WriteString(`<table class="wiki-infobox"><tbody>`)
		for _, field := range details.Infobox {
			output.WriteString(`<tr><th scope="row">`)
			output.WriteString(escapeHTML(field.Label))
			output.WriteString(`</th><td>`)
			output.WriteString(escapeHTML(field.Value))
			output.WriteString(`</td></tr>`)
		}
		output.WriteString(`</tbody></table>`)
	}
	if len(details.Sections) > 0 {
		output.WriteString(`<nav class="wiki-toc" aria-label="`)
		output.WriteString(escapeHTML(i18n.T(details.Language, "help.toc.heading")))
		output.WriteString(`"><strong>`)
		output.WriteString(escapeHTML(i18n.T(details.Language, "help.toc.heading")))
		output.WriteString(`</strong><ol>`)
		for index, section := range details.Sections {
			fmt.Fprintf(output, `<li><a href="#wiki-section-%d">%s</a></li>`, index, escapeHTML(section.Title))
		}
		output.WriteString(`</ol></nav><div class="wiki-sections">`)
		for index, section := range details.Sections {
			fmt.Fprintf(output, `<details class="wiki-section" id="wiki-section-%d"><summary>%s</summary><p>%s</p></details>`, index, escapeHTML(section.Title), escapeHTML(section.Text))
		}
		output.WriteString(`</div>`)
	}
	output.WriteString(`<form class="wiki-language" method="get" action="/search"><input type="hidden" name="q" value="`)
	output.WriteString(escapeHTML(details.Query))
	output.WriteString(`"><label for="wiki-language">`)
	output.WriteString(escapeHTML(i18n.T(details.Language, "search.language")))
	output.WriteString(`</label><select id="wiki-language" name="lang">`)
	for _, language := range []struct{ code, name string }{
		{"en", "English"}, {"ar", "العربية"}, {"de", "Deutsch"}, {"es", "Español"},
		{"fr", "Français"}, {"it", "Italiano"}, {"ja", "日本語"}, {"nl", "Nederlands"},
		{"pt", "Português"}, {"ru", "Русский"}, {"zh", "中文"},
	} {
		fmt.Fprintf(output, `<option value="%s"`, language.code)
		if language.code == details.Language {
			output.WriteString(` selected`)
		}
		output.WriteString(`>`)
		output.WriteString(language.name)
		output.WriteString(`</option>`)
	}
	output.WriteString(`</select><button type="submit">`)
	output.WriteString(escapeHTML(i18n.T(details.Language, "common.search")))
	output.WriteString(`</button></form>`)
	if len(details.RelatedArticles) > 0 {
		output.WriteString(`<p class="wiki-related">`)
		output.WriteString(escapeHTML(i18n.T(details.Language, "instant.widget_wiki_related", "")))
		for index, article := range details.RelatedArticles {
			if index > 0 {
				output.WriteString(`, `)
			}
			if directWikiArticleURL(article.URL, details.Language) {
				output.WriteString(`<a href="`)
				output.WriteString(escapeHTML(article.URL))
				output.WriteString(`">`)
			}
			output.WriteString(escapeHTML(article.Title))
			if directWikiArticleURL(article.URL, details.Language) {
				output.WriteString(`</a>`)
			}
		}
		output.WriteString(`</p>`)
	}
}

func directWikiArticleURL(rawURL, lang string) bool {
	parsed, err := url.Parse(rawURL)
	return err == nil && parsed.Scheme == "https" && parsed.Host == lang+".wikipedia.org" && strings.HasPrefix(parsed.Path, "/wiki/") && parsed.User == nil
}

// DictHandler handles dict:{word} queries
type DictHandler struct {
	client *http.Client
}

// NewDictHandler creates a new dictionary handler
func NewDictHandler() *DictHandler {
	return &DictHandler{
		client: newNoRedirectHTTPClient(10 * time.Second),
	}
}

func (h *DictHandler) Type() AnswerType {
	return AnswerTypeDict
}

func (h *DictHandler) HandleDirectQuery(ctx context.Context, term string) (*Answer, error) {
	term = strings.TrimSpace(strings.ToLower(term))
	if term == "" {
		return nil, fmt.Errorf("word required")
	}

	lang := LangFromContext(ctx)

	// Wiktionary is the project-defined source for dictionary definitions.
	apiURL := fmt.Sprintf("https://en.wiktionary.org/api/rest_v1/page/definition/%s", url.PathEscape(term))

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", version.BrowserUserAgent)

	resp, err := h.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer closeutil.Close(resp.Body)

	if resp.StatusCode == http.StatusNotFound {
		return &Answer{
			Type:        AnswerTypeDict,
			Term:        term,
			Title:       fmt.Sprintf("Dictionary: %s", term),
			Description: i18n.T(lang, "direct.word_not_found"),
			Content:     fmt.Sprintf("<p>No definition found for <code>%s</code>.</p>", escapeHTML(term)),
			Error:       "not_found",
		}, nil
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("dictionary provider returned HTTP %d", resp.StatusCode)
	}
	var wiktionary map[string][]struct {
		PartOfSpeech string `json:"partOfSpeech"`
		Language     string `json:"language"`
		Definitions  []struct {
			Definition string   `json:"definition"`
			Examples   []string `json:"examples"`
		} `json:"definitions"`
	}
	if err := decodeBoundedJSON(resp.Body, 2<<20, &wiktionary); err != nil {
		return nil, err
	}
	entries := wiktionary["en"]
	if len(entries) == 0 {
		return &Answer{
			Type:        AnswerTypeDict,
			Term:        term,
			Title:       fmt.Sprintf("Dictionary: %s", term),
			Description: i18n.T(lang, "direct.word_not_found"),
			Content:     fmt.Sprintf("<p>No definition found for <code>%s</code>.</p>", escapeHTML(term)),
			Error:       "not_found",
		}, nil
	}

	meanings := make([]struct {
		PartOfSpeech string `json:"partOfSpeech"`
		Definitions  []struct {
			Definition string   `json:"definition"`
			Example    string   `json:"example"`
			Synonyms   []string `json:"synonyms"`
			Antonyms   []string `json:"antonyms"`
		} `json:"definitions"`
		Synonyms []string `json:"synonyms"`
		Antonyms []string `json:"antonyms"`
	}, 0, len(entries))
	for _, item := range entries {
		meaning := struct {
			PartOfSpeech string `json:"partOfSpeech"`
			Definitions  []struct {
				Definition string   `json:"definition"`
				Example    string   `json:"example"`
				Synonyms   []string `json:"synonyms"`
				Antonyms   []string `json:"antonyms"`
			} `json:"definitions"`
			Synonyms []string `json:"synonyms"`
			Antonyms []string `json:"antonyms"`
		}{PartOfSpeech: item.PartOfSpeech}
		for _, definition := range item.Definitions {
			plain := htmlText(definition.Definition)
			if plain == "" {
				continue
			}
			example := ""
			if len(definition.Examples) > 0 {
				example = htmlText(definition.Examples[0])
			}
			meaning.Definitions = append(meaning.Definitions, struct {
				Definition string   `json:"definition"`
				Example    string   `json:"example"`
				Synonyms   []string `json:"synonyms"`
				Antonyms   []string `json:"antonyms"`
			}{Definition: plain, Example: example})
		}
		if len(meaning.Definitions) > 0 {
			meanings = append(meanings, meaning)
		}
	}
	if len(meanings) == 0 {
		return nil, fmt.Errorf("dictionary provider returned no usable English definitions")
	}
	entryWord, phonetic, audioURL, origin := term, "", "", ""
	metadataURL := fmt.Sprintf("https://api.dictionaryapi.dev/api/v2/entries/en/%s", url.PathEscape(term))
	metadataReq, metadataErr := http.NewRequestWithContext(ctx, http.MethodGet, metadataURL, nil)
	if metadataErr == nil {
		metadataReq.Header.Set("User-Agent", version.BrowserUserAgent)
		if metadataResp, fetchErr := h.client.Do(metadataReq); fetchErr == nil {
			defer closeutil.Close(metadataResp.Body)
			if metadataResp.StatusCode >= http.StatusOK && metadataResp.StatusCode < http.StatusMultipleChoices {
				var metadata []struct {
					Word      string `json:"word"`
					Phonetic  string `json:"phonetic"`
					Phonetics []struct {
						Audio string `json:"audio"`
					} `json:"phonetics"`
					Origin   string `json:"origin"`
					Meanings []struct {
						PartOfSpeech string `json:"partOfSpeech"`
						Definitions  []struct {
							Example  string   `json:"example"`
							Synonyms []string `json:"synonyms"`
							Antonyms []string `json:"antonyms"`
						} `json:"definitions"`
						Synonyms []string `json:"synonyms"`
						Antonyms []string `json:"antonyms"`
					} `json:"meanings"`
				}
				if decodeBoundedJSON(metadataResp.Body, 2<<20, &metadata) == nil {
					for _, item := range metadata {
						if item.Word != "" {
							entryWord = item.Word
						}
						if phonetic == "" {
							phonetic = item.Phonetic
						}
						if origin == "" {
							origin = item.Origin
						}
						for _, metadataMeaning := range item.Meanings {
							for meaningIndex := range meanings {
								if !strings.EqualFold(meanings[meaningIndex].PartOfSpeech, metadataMeaning.PartOfSpeech) {
									continue
								}
								for definitionIndex, metadataDefinition := range metadataMeaning.Definitions {
									if definitionIndex >= len(meanings[meaningIndex].Definitions) {
										break
									}
									if meanings[meaningIndex].Definitions[definitionIndex].Example == "" {
										meanings[meaningIndex].Definitions[definitionIndex].Example = metadataDefinition.Example
									}
									meanings[meaningIndex].Definitions[definitionIndex].Synonyms = append(meanings[meaningIndex].Definitions[definitionIndex].Synonyms, metadataDefinition.Synonyms...)
									meanings[meaningIndex].Definitions[definitionIndex].Antonyms = append(meanings[meaningIndex].Definitions[definitionIndex].Antonyms, metadataDefinition.Antonyms...)
								}
								meanings[meaningIndex].Synonyms = append(meanings[meaningIndex].Synonyms, metadataMeaning.Synonyms...)
								meanings[meaningIndex].Antonyms = append(meanings[meaningIndex].Antonyms, metadataMeaning.Antonyms...)
							}
						}
						for _, sound := range item.Phonetics {
							if audioURL = safeDictionaryAudioSource(sound.Audio); audioURL != "" {
								break
							}
						}
					}
				}
			}
		}
	}
	frequency := h.fetchWordFrequency(ctx, term)

	proxyAudioURL := ""
	if audioURL != "" {
		proxyAudioURL = "/direct/audio?url=" + url.QueryEscape(audioURL)
	}
	data := map[string]interface{}{
		"word":     entryWord,
		"phonetic": phonetic,
		"meanings": meanings,
		"origin":   origin,
		"audioURL": proxyAudioURL,
	}
	if phonetic != "" {
		data["phonetic"] = phonetic
	}
	if frequency > 0 {
		data["frequency_per_million"] = frequency
	}

	return &Answer{
		Type:        AnswerTypeDict,
		Term:        term,
		Title:       entryWord,
		Description: fmt.Sprintf("Dictionary entry for %s", entryWord),
		Content:     formatDictContentWithFrequency(entryWord, phonetic, audioURL, meanings, origin, frequency),
		Source:      "Wiktionary API",
		SourceURL:   "https://en.wiktionary.org/api/rest_v1/",
		Data:        data,
	}, nil
}

func (h *DictHandler) fetchWordFrequency(ctx context.Context, word string) float64 {
	query := url.Values{"sp": {word}, "qe": {"sp"}, "md": {"f"}, "max": {"1"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.datamuse.com/words?"+query.Encode(), nil)
	if err != nil {
		return 0
	}
	req.Header.Set("User-Agent", version.BrowserUserAgent)
	resp, err := h.client.Do(req)
	if err != nil {
		return 0
	}
	defer closeutil.Close(resp.Body)
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return 0
	}
	var results []struct {
		Tags []string `json:"tags"`
	}
	if decodeBoundedJSON(resp.Body, 256<<10, &results) != nil {
		return 0
	}
	for _, result := range results {
		for _, tag := range result.Tags {
			if strings.HasPrefix(tag, "f:") {
				value, parseErr := strconv.ParseFloat(strings.TrimPrefix(tag, "f:"), 64)
				if parseErr == nil && value >= 0 && !math.IsInf(value, 0) && !math.IsNaN(value) {
					return value
				}
			}
		}
	}
	return 0
}

func safeDictionaryAudioSource(raw string) string {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "//") {
		raw = "https:" + raw
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.User != nil || parsed.Port() != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return ""
	}
	host := strings.ToLower(parsed.Hostname())
	switch {
	case host == "api.dictionaryapi.dev" && strings.HasPrefix(parsed.Path, "/media/pronunciations/"):
	case host == "ssl.gstatic.com" && strings.HasPrefix(parsed.Path, "/dictionary/static/sounds/"):
	default:
		return ""
	}
	return parsed.String()
}

// SafeDictionaryAudioSource returns a normalized pronunciation URL for a supported provider.
func SafeDictionaryAudioSource(raw string) string {
	return safeDictionaryAudioSource(raw)
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
	return formatDictContentWithFrequency(word, phonetic, audioURL, meanings, origin, 0)
}

func formatDictContentWithFrequency(word, phonetic, audioURL string, meanings []struct {
	PartOfSpeech string `json:"partOfSpeech"`
	Definitions  []struct {
		Definition string   `json:"definition"`
		Example    string   `json:"example"`
		Synonyms   []string `json:"synonyms"`
		Antonyms   []string `json:"antonyms"`
	} `json:"definitions"`
	Synonyms []string `json:"synonyms"`
	Antonyms []string `json:"antonyms"`
}, origin string, frequency float64) string {
	var html strings.Builder
	html.WriteString("<div class=\"dict-content\">")

	// Word and pronunciation
	fmt.Fprintf(&html, "<h1>%s</h1>", escapeHTML(word))
	if phonetic != "" {
		fmt.Fprintf(&html, "<p class=\"phonetic\">%s</p>", escapeHTML(phonetic))
	}
	if safeAudioURL := safeDictionaryAudioSource(audioURL); safeAudioURL != "" {
		proxyURL := "/direct/audio?url=" + url.QueryEscape(safeAudioURL)
		fmt.Fprintf(&html, "<audio class=\"audio-player\" controls preload=\"none\" aria-label=\"Pronunciation audio\" src=\"%s\"></audio>", escapeHTML(proxyURL))
	}
	if frequency > 0 {
		fmt.Fprintf(&html, "<p class=\"word-frequency\">Frequency: %.2f occurrences per million words (Google Books Ngram estimate)</p>", frequency)
	}

	// Meanings
	for _, meaning := range meanings {
		fmt.Fprintf(&html, "<h2>%s</h2>", escapeHTML(meaning.PartOfSpeech))
		html.WriteString("<ol class=\"definitions\">")

		for i, def := range meaning.Definitions {
			if i >= 5 {
				// Limit to 5 definitions per part of speech
				break
			}
			html.WriteString("<li>")
			fmt.Fprintf(&html, "<p>%s</p>", escapeHTML(def.Definition))
			if def.Example != "" {
				fmt.Fprintf(&html, "<p class=\"example\"><em>\"%s\"</em></p>", escapeHTML(def.Example))
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
				fmt.Fprintf(&html, "<a href=\"/direct/dict/%s\">%s</a>", url.PathEscape(syn), escapeHTML(syn))
			}
			html.WriteString("</p>")
		}
		if len(meaning.Antonyms) > 0 {
			html.WriteString("<p class=\"antonyms\"><strong>Antonyms:</strong> ")
			for i, antonym := range meaning.Antonyms {
				if i > 0 {
					html.WriteString(", ")
				}
				if i >= 10 {
					html.WriteString("...")
					break
				}
				fmt.Fprintf(&html, "<a href=\"/direct/dict/%s\">%s</a>", url.PathEscape(antonym), escapeHTML(antonym))
			}
			html.WriteString("</p>")
		}
	}

	// Etymology
	if origin != "" {
		html.WriteString("<h2>Etymology</h2>")
		fmt.Fprintf(&html, "<p>%s</p>", escapeHTML(origin))
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
		client: newNoRedirectHTTPClient(10 * time.Second),
	}
}

func newNoRedirectHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout, CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}}
}

func (h *ThesaurusHandler) Type() AnswerType {
	return AnswerTypeThesaurus
}

func (h *ThesaurusHandler) HandleDirectQuery(ctx context.Context, term string) (*Answer, error) {
	term = strings.TrimSpace(strings.ToLower(term))
	if term == "" {
		return nil, fmt.Errorf("word required")
	}

	lang := LangFromContext(ctx)
	type thesaurusSenseResult struct {
		senses []thesaurusSense
		err    error
	}
	senseContext, cancelSense := context.WithCancel(ctx)
	defer cancelSense()
	senseResults := make(chan thesaurusSenseResult, 1)
	go func() {
		senses, err := h.fetchThesaurusSenses(senseContext, term)
		senseResults <- thesaurusSenseResult{senses: senses, err: err}
	}()

	// Use Datamuse API for synonyms and antonyms
	synURL := fmt.Sprintf("https://api.datamuse.com/words?rel_syn=%s&max=20", url.QueryEscape(term))
	antURL := fmt.Sprintf("https://api.datamuse.com/words?rel_ant=%s&max=20", url.QueryEscape(term))

	synonymEntries, synonymsErr := h.fetchThesaurusEntries(ctx, synURL)
	antonymEntries, antonymsErr := h.fetchThesaurusEntries(ctx, antURL)
	relatedURL := fmt.Sprintf("https://api.datamuse.com/words?rel_trg=%s&max=20&md=p", url.QueryEscape(term))
	relatedEntries, relatedErr := h.fetchThesaurusEntries(ctx, relatedURL)
	senseResult := <-senseResults
	synonyms := thesaurusEntryWords(synonymEntries)
	antonyms := thesaurusEntryWords(antonymEntries)

	if synonymsErr != nil && antonymsErr != nil && relatedErr != nil && (senseResult.err != nil || len(senseResult.senses) == 0) {
		return &Answer{
			Type: AnswerTypeThesaurus, Term: term, Title: fmt.Sprintf("Thesaurus: %s", term),
			Description: i18n.T(lang, "direct.failed_to_fetch"),
			Content:     "<p class=\"error\">Thesaurus data could not be loaded.</p>", Error: "fetch_failed",
		}, nil
	}
	if synonymsErr == nil && antonymsErr == nil && relatedErr == nil && senseResult.err == nil && len(synonyms) == 0 && len(antonyms) == 0 && len(relatedEntries) == 0 && len(senseResult.senses) == 0 {
		return &Answer{
			Type:        AnswerTypeThesaurus,
			Term:        term,
			Title:       fmt.Sprintf("Thesaurus: %s", term),
			Description: i18n.T(lang, "direct.no_results_found"),
			Content:     fmt.Sprintf("<p>No synonyms or antonyms found for <code>%s</code>.</p>", escapeHTML(term)),
			Error:       "not_found",
		}, nil
	}

	content := formatThesaurusEntriesContent(term, synonymEntries, antonymEntries, relatedEntries, senseResult.senses)
	if synonymsErr != nil {
		content += "<p class=\"error\">Synonyms could not be loaded.</p>"
	}
	if antonymsErr != nil {
		content += "<p class=\"error\">Antonyms could not be loaded.</p>"
	}
	if relatedErr != nil {
		content += fmt.Sprintf("<p class=\"error\">%s</p>", escapeHTML(i18n.T(lang, "direct.failed_to_fetch")))
	}
	if senseResult.err != nil {
		content += fmt.Sprintf("<p class=\"error\">%s</p>", escapeHTML(i18n.T(lang, "direct.failed_to_fetch")))
	}
	data := map[string]interface{}{
		"word": term, "synonyms": synonyms, "antonyms": antonyms,
		"synonym_entries": synonymEntries, "antonym_entries": antonymEntries,
		"senses":             senseResult.senses,
		"related_words":      thesaurusEntryWords(relatedEntries),
		"synonyms_available": synonymsErr == nil, "antonyms_available": antonymsErr == nil,
		"related_available": relatedErr == nil,
		"senses_available":  senseResult.err == nil,
	}

	return &Answer{
		Type:        AnswerTypeThesaurus,
		Term:        term,
		Title:       fmt.Sprintf("Thesaurus: %s", term),
		Description: fmt.Sprintf("Synonyms, antonyms, senses, and usage examples for %s", term),
		Content:     content,
		Source:      "Datamuse and Free Dictionary API",
		SourceURL:   "https://www.datamuse.com/api/",
		Data:        data,
	}, nil
}

type thesaurusEntry struct {
	Word        string   `json:"word"`
	Tags        []string `json:"tags"`
	Definitions []string `json:"defs"`
}

type thesaurusSense struct {
	PartOfSpeech string   `json:"partOfSpeech"`
	Definition   string   `json:"definition"`
	Example      string   `json:"example,omitempty"`
	Synonyms     []string `json:"synonyms,omitempty"`
	Antonyms     []string `json:"antonyms,omitempty"`
}

func thesaurusEntryWords(entries []thesaurusEntry) []string {
	words := make([]string, 0, len(entries))
	for _, entry := range entries {
		if word := strings.TrimSpace(entry.Word); word != "" {
			words = append(words, word)
		}
	}
	return words
}

func (h *ThesaurusHandler) fetchThesaurusEntries(ctx context.Context, apiURL string) ([]thesaurusEntry, error) {
	parsed, err := url.Parse(apiURL)
	if err != nil {
		return nil, err
	}
	query := parsed.Query()
	query.Set("md", "dp")
	parsed.RawQuery = query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", version.BrowserUserAgent)
	resp, err := h.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer closeutil.Close(resp.Body)
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("datamuse returned HTTP %d", resp.StatusCode)
	}
	var entries []thesaurusEntry
	if err := decodeBoundedJSON(resp.Body, 2<<20, &entries); err != nil {
		return nil, err
	}
	return entries, nil
}

func (h *ThesaurusHandler) fetchThesaurusSenses(ctx context.Context, word string) ([]thesaurusSense, error) {
	apiURL := "https://api.dictionaryapi.dev/api/v2/entries/en/" + url.PathEscape(word)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", version.BrowserUserAgent)
	resp, err := h.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer closeutil.Close(resp.Body)
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("dictionary provider returned HTTP %d", resp.StatusCode)
	}
	var entries []struct {
		Meanings []struct {
			PartOfSpeech string   `json:"partOfSpeech"`
			Synonyms     []string `json:"synonyms"`
			Antonyms     []string `json:"antonyms"`
			Definitions  []struct {
				Definition string   `json:"definition"`
				Example    string   `json:"example"`
				Synonyms   []string `json:"synonyms"`
				Antonyms   []string `json:"antonyms"`
			} `json:"definitions"`
		} `json:"meanings"`
	}
	if err := decodeBoundedJSON(resp.Body, 2<<20, &entries); err != nil {
		return nil, err
	}
	const maxSenses = 20
	senses := make([]thesaurusSense, 0, maxSenses)
	for _, entry := range entries {
		for _, meaning := range entry.Meanings {
			for _, definition := range meaning.Definitions {
				if len(senses) >= maxSenses {
					return senses, nil
				}
				definitionText := strings.TrimSpace(definition.Definition)
				if definitionText == "" {
					continue
				}
				sense := thesaurusSense{
					PartOfSpeech: truncateString(strings.TrimSpace(meaning.PartOfSpeech), 40),
					Definition:   truncateString(definitionText, 1000),
					Example:      truncateString(strings.TrimSpace(definition.Example), 500),
					Synonyms:     boundedUniqueWords(append(append([]string{}, meaning.Synonyms...), definition.Synonyms...), 20),
					Antonyms:     boundedUniqueWords(append(append([]string{}, meaning.Antonyms...), definition.Antonyms...), 20),
				}
				senses = append(senses, sense)
			}
		}
	}
	return senses, nil
}

func boundedUniqueWords(words []string, maximum int) []string {
	unique := make([]string, 0, min(len(words), maximum))
	seen := make(map[string]struct{}, min(len(words), maximum))
	for _, word := range words {
		word = strings.TrimSpace(word)
		key := strings.ToLower(word)
		if word == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		unique = append(unique, truncateString(word, 80))
		if len(unique) == maximum {
			break
		}
	}
	return unique
}

func formatThesaurusEntriesContent(word string, synonyms, antonyms, related []thesaurusEntry, senses []thesaurusSense) string {
	var html strings.Builder
	html.WriteString(`<div class="thesaurus-content">`)
	fmt.Fprintf(&html, `<h1>%s</h1>`, escapeHTML(word))
	writeThesaurusRelation(&html, "Synonyms", synonyms)
	writeThesaurusRelation(&html, "Antonyms", antonyms)
	writeThesaurusRelation(&html, "Related words and phrases", related)
	writeThesaurusSenses(&html, senses)
	return html.String() + `</div>`
}

func writeThesaurusSenses(output *strings.Builder, senses []thesaurusSense) {
	if len(senses) == 0 {
		return
	}
	output.WriteString(`<h2>Senses and usage examples</h2><div class="thesaurus-senses">`)
	for _, sense := range senses {
		output.WriteString(`<section class="thesaurus-sense">`)
		if sense.PartOfSpeech != "" {
			fmt.Fprintf(output, `<span class="part-of-speech">%s</span>`, escapeHTML(sense.PartOfSpeech))
		}
		fmt.Fprintf(output, `<p class="sense-definition">%s</p>`, escapeHTML(sense.Definition))
		if sense.Example != "" {
			fmt.Fprintf(output, `<p class="usage-example"><em>“%s”</em></p>`, escapeHTML(sense.Example))
		}
		writeThesaurusWordTags(output, "Synonyms", sense.Synonyms)
		writeThesaurusWordTags(output, "Antonyms", sense.Antonyms)
		output.WriteString(`</section>`)
	}
	output.WriteString(`</div>`)
}

func writeThesaurusWordTags(output *strings.Builder, label string, words []string) {
	if len(words) == 0 {
		return
	}
	fmt.Fprintf(output, `<p class="sense-%s"><strong>%s:</strong> `, strings.ToLower(label), escapeHTML(label))
	for index, word := range words {
		if index > 0 {
			output.WriteString(`, `)
		}
		fmt.Fprintf(output, `<a href="/direct/thesaurus/%s">%s</a>`, url.PathEscape(word), escapeHTML(word))
	}
	output.WriteString(`</p>`)
}

func writeThesaurusRelation(output *strings.Builder, title string, entries []thesaurusEntry) {
	if len(entries) == 0 {
		return
	}
	fmt.Fprintf(output, `<h2>%s</h2>`, escapeHTML(title))
	output.WriteString(`<div class="word-list">`)
	for _, entry := range entries {
		word := strings.TrimSpace(entry.Word)
		if word == "" {
			continue
		}
		fmt.Fprintf(output, `<section class="thesaurus-entry"><a class="word-tag" href="/direct/thesaurus/%s">%s</a>`, url.PathEscape(word), escapeHTML(word))
		parts := make([]string, 0, len(entry.Tags))
		for _, tag := range entry.Tags {
			if slices.Contains([]string{"n", "v", "adj", "adv", "u"}, tag) {
				parts = append(parts, tag)
			}
		}
		if len(parts) > 0 {
			fmt.Fprintf(output, `<span class="part-of-speech">%s</span>`, escapeHTML(strings.Join(parts, ", ")))
		}
		for _, definition := range entry.Definitions {
			definition = strings.TrimSpace(definition)
			if definition != "" {
				fmt.Fprintf(output, `<p class="sense-definition">%s</p>`, escapeHTML(definition))
			}
		}
		output.WriteString(`</section>`)
	}
	output.WriteString(`</div>`)
}

func formatThesaurusContent(word string, synonyms, antonyms []string) string {
	var html strings.Builder
	html.WriteString("<div class=\"thesaurus-content\">")
	fmt.Fprintf(&html, "<h1>%s</h1>", escapeHTML(word))

	if len(synonyms) > 0 {
		html.WriteString("<h2>Synonyms</h2>")
		html.WriteString("<div class=\"word-list synonyms\">")
		for _, syn := range synonyms {
			fmt.Fprintf(&html, "<a href=\"/direct/thesaurus/%s\" class=\"word-tag\">%s</a> ",
				url.PathEscape(syn), escapeHTML(syn))
		}
		html.WriteString("</div>")
	}

	if len(antonyms) > 0 {
		html.WriteString("<h2>Antonyms</h2>")
		html.WriteString("<div class=\"word-list antonyms\">")
		for _, ant := range antonyms {
			fmt.Fprintf(&html, "<a href=\"/direct/thesaurus/%s\" class=\"word-tag\">%s</a> ",
				url.PathEscape(ant), escapeHTML(ant))
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

var packageDownloadsCache = struct {
	sync.Mutex
	entries map[string]pypiDownloadCacheEntry
}{entries: make(map[string]pypiDownloadCacheEntry)}

var packageReadmeCache = struct {
	sync.Mutex
	entries map[string]packageReadmeCacheEntry
}{entries: make(map[string]packageReadmeCacheEntry)}

type packageReadmeCacheEntry struct {
	preview   string
	expiresAt time.Time
}

type pypiDownloadCacheEntry struct {
	weeklyDownloads int
	expiresAt       time.Time
}

var cratesIOThrottle struct {
	sync.Mutex
	lastRequest time.Time
}

// NewPkgHandler creates a new package handler
func NewPkgHandler() *PkgHandler {
	return &PkgHandler{
		client: &http.Client{
			Timeout: 10 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

func (h *PkgHandler) Type() AnswerType {
	return AnswerTypePkg
}

func (h *PkgHandler) HandleDirectQuery(ctx context.Context, term string) (*Answer, error) {
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
	} else if strings.HasPrefix(term, "crate/") || strings.HasPrefix(term, "crates/") {
		registry = "crates"
		packageName = strings.TrimPrefix(strings.TrimPrefix(term, "crate/"), "crates/")
	} else if strings.HasPrefix(term, "gem/") || strings.HasPrefix(term, "rubygems/") {
		registry = "rubygems"
		packageName = strings.TrimPrefix(strings.TrimPrefix(term, "gem/"), "rubygems/")
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
	case "crates":
		return h.fetchCrates(ctx, packageName)
	case "rubygems":
		return h.fetchRubyGem(ctx, packageName)
	default:
		return h.fetchNPM(ctx, packageName)
	}
}

func (h *PkgHandler) fetchNPM(ctx context.Context, name string) (*Answer, error) {
	lang := LangFromContext(ctx)

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
	defer closeutil.Close(resp.Body)

	if resp.StatusCode == http.StatusNotFound {
		return &Answer{
			Type:        AnswerTypePkg,
			Term:        name,
			Title:       fmt.Sprintf("Package: %s", name),
			Description: i18n.T(lang, "direct.package_not_found"),
			Content:     fmt.Sprintf("<p>Package <code>%s</code> not found on npm.</p>", escapeHTML(name)),
			Error:       "not_found",
		}, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("npm registry returned HTTP %d", resp.StatusCode)
	}

	var pkg struct {
		Name        string          `json:"name"`
		Description json.RawMessage `json:"description"`
		DistTags    struct {
			Latest string `json:"latest"`
		} `json:"dist-tags"`
		Time       json.RawMessage `json:"time"`
		License    json.RawMessage `json:"license"`
		Repository json.RawMessage `json:"repository"`
		Readme     json.RawMessage `json:"readme"`
		Versions   json.RawMessage `json:"versions"`
	}

	if err := decodeBoundedJSON(resp.Body, 16<<20, &pkg); err != nil {
		return nil, err
	}
	if strings.TrimSpace(pkg.Name) == "" || strings.TrimSpace(pkg.DistTags.Latest) == "" {
		return nil, fmt.Errorf("npm registry returned incomplete package metadata")
	}
	var license string
	decodeOptionalJSONString(pkg.License, &license)
	description := ""
	decodeOptionalJSONString(pkg.Description, &description)
	readme := ""
	decodeOptionalJSONString(pkg.Readme, &readme)
	var npmTime struct {
		Modified json.RawMessage `json:"modified"`
	}
	_ = json.Unmarshal(pkg.Time, &npmTime)
	updated := ""
	decodeOptionalJSONString(npmTime.Modified, &updated)
	var npmRepository struct {
		URL json.RawMessage `json:"url"`
	}
	_ = json.Unmarshal(pkg.Repository, &npmRepository)
	repository := ""
	decodeOptionalJSONString(npmRepository.URL, &repository)
	var downloads struct {
		Downloads *int `json:"downloads"`
	}
	weeklyDownloads := interface{}(nil)
	downloadURL := "https://api.npmjs.org/downloads/point/last-week/" + url.PathEscape(name)
	if err := h.fetchPackageJSON(ctx, downloadURL, &downloads); err == nil && downloads.Downloads != nil && *downloads.Downloads >= 0 {
		weeklyDownloads = *downloads.Downloads
	}
	var npmVersions map[string]struct {
		Dependencies json.RawMessage `json:"dependencies"`
	}
	_ = json.Unmarshal(pkg.Versions, &npmVersions)
	var dependencies interface{}
	if latestVersion, ok := npmVersions[pkg.DistTags.Latest]; ok {
		var dependencyMap map[string]json.RawMessage
		if json.Unmarshal(latestVersion.Dependencies, &dependencyMap) == nil && dependencyMap != nil {
			dependencies = len(dependencyMap)
		}
	}
	readmePreview := truncateString(strings.TrimSpace(readme), 2000)
	data := map[string]interface{}{
		"name":             pkg.Name,
		"description":      description,
		"version":          pkg.DistTags.Latest,
		"license":          packageOptionalString(license),
		"registry":         "npm",
		"weekly_downloads": weeklyDownloads,
		"dependencies":     dependencies,
		"updated":          packageOptionalString(updated),
		"repository":       packageOptionalString(repository),
		"readme_preview":   readmePreview,
	}

	return &Answer{
		Type:        AnswerTypePkg,
		Term:        name,
		Title:       pkg.Name,
		Description: description,
		Content:     formatPkgContentWithDetails(lang, "npm", pkg.Name, description, pkg.DistTags.Latest, license, "npm install "+pkg.Name, data),
		Source:      "npm",
		SourceURL:   fmt.Sprintf("https://www.npmjs.com/package/%s", url.PathEscape(name)),
		Data:        data,
	}, nil
}

func decodeOptionalJSONString(raw json.RawMessage, destination *string) {
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, destination)
	}
}

func packageOptionalString(value string) interface{} {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

func (h *PkgHandler) fetchPyPI(ctx context.Context, name string) (*Answer, error) {
	lang := LangFromContext(ctx)

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
	defer closeutil.Close(resp.Body)

	if resp.StatusCode == http.StatusNotFound {
		return &Answer{
			Type:        AnswerTypePkg,
			Term:        name,
			Title:       fmt.Sprintf("Package: %s", name),
			Description: i18n.T(lang, "direct.package_not_found"),
			Content:     fmt.Sprintf("<p>Package <code>%s</code> not found on PyPI.</p>", escapeHTML(name)),
			Error:       "not_found",
		}, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("PyPI returned HTTP %d", resp.StatusCode)
	}

	var pkg struct {
		Info struct {
			Name            string            `json:"name"`
			Summary         string            `json:"summary"`
			Description     string            `json:"description"`
			Version         string            `json:"version"`
			License         string            `json:"license"`
			ProjectURLs     map[string]string `json:"project_urls"`
			ProjectURL      string            `json:"project_url"`
			Homepage        string            `json:"home_page"`
			RequiresDist    []string          `json:"requires_dist"`
			DescriptionType string            `json:"description_content_type"`
		} `json:"info"`
		Releases map[string][]struct {
			UploadTime string `json:"upload_time_iso_8601"`
		} `json:"releases"`
	}

	if err := decodeBoundedJSON(resp.Body, 4<<20, &pkg); err != nil {
		return nil, err
	}
	if strings.TrimSpace(pkg.Info.Name) == "" || strings.TrimSpace(pkg.Info.Version) == "" {
		return nil, fmt.Errorf("PyPI returned incomplete package metadata")
	}

	repository := ""
	for key, value := range pkg.Info.ProjectURLs {
		if strings.EqualFold(key, "repository") || strings.EqualFold(key, "source") || strings.EqualFold(key, "source code") {
			repository = value
			break
		}
	}
	if repository == "" {
		repository = pkg.Info.ProjectURL
	}
	if repository == "" {
		repository = pkg.Info.Homepage
	}
	var updated interface{}
	if releaseTimestamps := pkg.Releases[pkg.Info.Version]; len(releaseTimestamps) > 0 && releaseTimestamps[0].UploadTime != "" {
		updated = releaseTimestamps[0].UploadTime
	}
	var dependencies interface{}
	if pkg.Info.RequiresDist != nil {
		dependencies = len(pkg.Info.RequiresDist)
	}
	readmePreview := truncateString(strings.TrimSpace(pkg.Info.Description), 2000)
	var weeklyDownloads interface{}
	if count, err := h.fetchPyPIWeeklyDownloads(ctx, pkg.Info.Name); err == nil {
		weeklyDownloads = count
	}
	data := map[string]interface{}{
		"name":             pkg.Info.Name,
		"description":      pkg.Info.Summary,
		"version":          pkg.Info.Version,
		"license":          pkg.Info.License,
		"registry":         "pypi",
		"weekly_downloads": weeklyDownloads,
		"dependencies":     dependencies,
		"updated":          updated,
		"repository":       repository,
		"readme_preview":   readmePreview,
		"description_type": pkg.Info.DescriptionType,
	}

	return &Answer{
		Type:        AnswerTypePkg,
		Term:        name,
		Title:       pkg.Info.Name,
		Description: pkg.Info.Summary,
		Content:     formatPkgContentWithDetails(lang, "pypi", pkg.Info.Name, pkg.Info.Summary, pkg.Info.Version, pkg.Info.License, "pip install "+pkg.Info.Name, data),
		Source:      "PyPI",
		SourceURL:   fmt.Sprintf("https://pypi.org/project/%s/", url.PathEscape(name)),
		Data:        data,
	}, nil
}

func (h *PkgHandler) fetchPyPIWeeklyDownloads(ctx context.Context, name string) (int, error) {
	cacheKey := "pypi:" + strings.ToLower(strings.TrimSpace(name))
	if cached, ok := cachedPackageDownloads(cacheKey); ok {
		return cached, nil
	}

	apiURL := "https://pypistats.org/api/packages/" + url.PathEscape(name) + "/recent?period=week"
	var response struct {
		Data struct {
			LastWeek *int `json:"last_week"`
		} `json:"data"`
	}
	if err := h.fetchPackageJSON(ctx, apiURL, &response); err != nil {
		return 0, err
	}
	if response.Data.LastWeek == nil || *response.Data.LastWeek < 0 {
		return 0, fmt.Errorf("PyPI Stats returned no weekly download count")
	}
	cachePackageDownloads(cacheKey, *response.Data.LastWeek)
	return *response.Data.LastWeek, nil
}

func cachedPackageDownloads(cacheKey string) (int, bool) {
	packageDownloadsCache.Lock()
	defer packageDownloadsCache.Unlock()
	entry, ok := packageDownloadsCache.entries[cacheKey]
	return entry.weeklyDownloads, ok && time.Now().Before(entry.expiresAt)
}

func cachePackageDownloads(cacheKey string, weeklyDownloads int) {
	packageDownloadsCache.Lock()
	defer packageDownloadsCache.Unlock()
	now := time.Now()
	for key, entry := range packageDownloadsCache.entries {
		if !now.Before(entry.expiresAt) {
			delete(packageDownloadsCache.entries, key)
		}
	}
	if len(packageDownloadsCache.entries) >= 500 {
		var oldestKey string
		var oldestExpiry time.Time
		for key, entry := range packageDownloadsCache.entries {
			if oldestKey == "" || entry.expiresAt.Before(oldestExpiry) {
				oldestKey = key
				oldestExpiry = entry.expiresAt
			}
		}
		delete(packageDownloadsCache.entries, oldestKey)
	}
	packageDownloadsCache.entries[cacheKey] = pypiDownloadCacheEntry{weeklyDownloads: weeklyDownloads, expiresAt: now.Add(24 * time.Hour)}
}

func (h *PkgHandler) fetchGo(ctx context.Context, name string) (*Answer, error) {
	modulePath := escapeGoModulePath(name)
	apiURL := "https://proxy.golang.org/" + modulePath + "/@latest"
	var pkg struct {
		Version string `json:"Version"`
		Time    string `json:"Time"`
	}
	if err := h.fetchPackageJSON(ctx, apiURL, &pkg); err != nil {
		return nil, err
	}
	if pkg.Version == "" {
		return nil, fmt.Errorf("module %q has no published version", name)
	}
	var dependencies interface{}
	modURL := "https://proxy.golang.org/" + modulePath + "/@v/" + escapeGoModulePath(pkg.Version) + ".mod"
	if modText, err := h.fetchPackageText(ctx, modURL, 2<<20); err == nil {
		if moduleFile, err := modfile.Parse("go.mod", []byte(modText), nil); err == nil {
			directDependencies := 0
			for _, requirement := range moduleFile.Require {
				if !requirement.Indirect {
					directDependencies++
				}
			}
			dependencies = directDependencies
		}
	}
	var readmePreview interface{}
	var license interface{}
	var archive []byte
	zipURL := "https://proxy.golang.org/" + modulePath + "/@v/" + escapeGoModulePath(pkg.Version) + ".zip"
	if archiveBytes, err := h.fetchPackageBytes(ctx, zipURL, 16<<20); err == nil {
		archive = archiveBytes
		if preview, err := goModuleReadmePreview(archive, modulePath+"@"+escapeGoModulePath(pkg.Version)+"/"); err == nil && preview != "" {
			readmePreview = preview
		}
		if value, err := goModuleLicense(archive, modulePath+"@"+escapeGoModulePath(pkg.Version)+"/"); err == nil && value != "" {
			license = value
		}
	}
	repository := goModuleRepository(name)
	data := map[string]interface{}{
		"name": name, "version": pkg.Version, "updated": pkg.Time, "registry": "go",
		"license": license, "weekly_downloads": nil, "dependencies": dependencies,
		"repository": repository, "readme_preview": readmePreview,
	}
	return &Answer{Type: AnswerTypePkg, Term: name, Title: name, Description: "Go module " + name,
		Content: formatPkgContentWithDetails(LangFromContext(ctx), "go", name, "Go module", pkg.Version, "", "go get "+name, data), Source: "Go Module Proxy",
		SourceURL: "https://pkg.go.dev/" + name, Data: data}, nil
}

func goModuleRepository(modulePath string) interface{} {
	parts := strings.Split(strings.Trim(modulePath, "/"), "/")
	if len(parts) < 3 {
		return nil
	}
	host := strings.ToLower(parts[0])
	switch host {
	case "github.com", "gitlab.com", "bitbucket.org":
		return "https://" + host + "/" + url.PathEscape(parts[1]) + "/" + url.PathEscape(parts[2])
	default:
		return nil
	}
}

func goModuleLicense(archive []byte, modulePrefix string) (string, error) {
	reader, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		return "", err
	}
	for _, file := range reader.File {
		if file.FileInfo().IsDir() || !strings.HasPrefix(file.Name, modulePrefix) {
			continue
		}
		name := strings.TrimPrefix(file.Name, modulePrefix)
		if strings.Contains(name, "/") || !regexp.MustCompile(`(?i)^(license|licence|copying)(\..*)?$`).MatchString(name) || file.UncompressedSize64 > 64<<10 {
			continue
		}
		opened, err := file.Open()
		if err != nil {
			continue
		}
		body, readErr := io.ReadAll(io.LimitReader(opened, (64<<10)+1))
		closeErr := opened.Close()
		if readErr != nil || closeErr != nil || len(body) > 64<<10 {
			continue
		}
		match := regexp.MustCompile(`(?im)^\s*SPDX-License-Identifier:\s*([A-Za-z0-9.-]+(?:\s+(?:AND|OR|WITH)\s+[A-Za-z0-9.-]+)*)\s*$`).FindSubmatch(body)
		if len(match) == 2 {
			return string(match[1]), nil
		}
	}
	return "", nil
}

func escapeGoModulePath(modulePath string) string {
	var escaped strings.Builder
	escaped.Grow(len(modulePath))
	for _, character := range modulePath {
		if character >= 'A' && character <= 'Z' {
			escaped.WriteByte('!')
			escaped.WriteRune(character + ('a' - 'A'))
		} else {
			escaped.WriteRune(character)
		}
	}
	return escaped.String()
}

func (h *PkgHandler) fetchCrates(ctx context.Context, name string) (*Answer, error) {
	apiURL := "https://crates.io/api/v1/crates/" + url.PathEscape(name)
	var response struct {
		Crate struct {
			ID          string `json:"id"`
			Name        string `json:"name"`
			Description string `json:"description"`
			MaxVersion  string `json:"max_version"`
			License     string `json:"license"`
			Repository  string `json:"repository"`
			Downloads   int    `json:"downloads"`
			Updated     string `json:"updated_at"`
		} `json:"crate"`
	}
	if err := h.fetchPackageJSON(ctx, apiURL, &response); err != nil {
		return nil, err
	}
	p := response.Crate
	if strings.TrimSpace(p.Name) == "" || strings.TrimSpace(p.MaxVersion) == "" {
		return nil, fmt.Errorf("crates.io returned incomplete package metadata")
	}
	var totalDownloads interface{}
	if p.Downloads >= 0 {
		totalDownloads = p.Downloads
	}
	versionURL := apiURL + "/" + url.PathEscape(p.MaxVersion)
	var versionResponse struct {
		Version struct {
			ReadmePath string `json:"readme_path"`
		} `json:"version"`
	}
	if p.MaxVersion != "" {
		_ = h.fetchPackageJSON(ctx, versionURL, &versionResponse)
	}
	var dependencies interface{}
	if p.MaxVersion != "" {
		var dependencyResponse struct {
			Dependencies []json.RawMessage `json:"dependencies"`
		}
		if err := h.fetchPackageJSON(ctx, versionURL+"/dependencies", &dependencyResponse); err == nil && dependencyResponse.Dependencies != nil {
			dependencies = len(dependencyResponse.Dependencies)
		}
	}
	var readmePreview string
	readmePath := versionResponse.Version.ReadmePath
	if readmePath != "" && strings.HasPrefix(readmePath, "/api/v1/crates/") {
		if readmeHTML, err := h.fetchPackageText(ctx, "https://crates.io"+readmePath, 1<<20); err == nil {
			if document, err := xhtml.Parse(strings.NewReader(readmeHTML)); err == nil {
				readmePreview = truncateString(strings.TrimSpace(nodeText(document)), 2000)
			}
		}
	}
	var weeklyDownloads interface{}
	if count, err := h.fetchCratesWeeklyDownloads(ctx, p.Name); err == nil {
		weeklyDownloads = count
	}
	data := map[string]interface{}{
		"name": p.Name, "version": p.MaxVersion, "license": p.License,
		"downloads": totalDownloads, "weekly_downloads": weeklyDownloads, "dependencies": dependencies,
		"updated": p.Updated, "repository": p.Repository, "readme_preview": readmePreview,
		"registry": "crates.io",
	}
	return &Answer{Type: AnswerTypePkg, Term: name, Title: p.Name, Description: p.Description,
		Content: formatPkgContentWithDetails(LangFromContext(ctx), "crates.io", p.Name, p.Description, p.MaxVersion, p.License, "cargo add "+p.Name, data), Source: "crates.io",
		SourceURL: "https://crates.io/crates/" + url.PathEscape(p.ID), Data: data}, nil
}

func (h *PkgHandler) fetchCratesWeeklyDownloads(ctx context.Context, name string) (int64, error) {
	cacheKey := "crates:" + strings.ToLower(strings.TrimSpace(name))
	if cached, ok := cachedPackageDownloads(cacheKey); ok {
		return int64(cached), nil
	}
	type dailyDownloads struct {
		Downloads int64  `json:"downloads"`
		Date      string `json:"date"`
	}
	var response struct {
		VersionDownloads []dailyDownloads `json:"version_downloads"`
		Meta             struct {
			ExtraDownloads []dailyDownloads `json:"extra_downloads"`
		} `json:"meta"`
	}
	apiURL := "https://crates.io/api/v1/crates/" + url.PathEscape(name) + "/downloads"
	if err := h.fetchPackageJSON(ctx, apiURL, &response); err != nil {
		return 0, err
	}
	today := time.Now().UTC().Truncate(24 * time.Hour)
	firstDay := today.AddDate(0, 0, -6)
	var total int64
	var includedDays int
	downloads := append(response.VersionDownloads, response.Meta.ExtraDownloads...)
	for _, daily := range downloads {
		date, err := time.Parse("2006-01-02", daily.Date)
		if err != nil || date.Before(firstDay) || date.After(today) {
			continue
		}
		if daily.Downloads < 0 || daily.Downloads > int64(^uint64(0)>>1)-total {
			return 0, fmt.Errorf("crates.io weekly download count is invalid")
		}
		total += daily.Downloads
		includedDays++
	}
	if includedDays == 0 {
		return 0, fmt.Errorf("crates.io returned no recent download data")
	}
	if total > int64(^uint(0)>>1) {
		return 0, fmt.Errorf("crates.io weekly download count exceeds supported range")
	}
	cachePackageDownloads(cacheKey, int(total))
	return total, nil
}

func (h *PkgHandler) fetchRubyGem(ctx context.Context, name string) (*Answer, error) {
	apiURL := "https://rubygems.org/api/v1/gems/" + url.PathEscape(name) + ".json"
	var p struct {
		Name           string   `json:"name"`
		Info           string   `json:"info"`
		Version        string   `json:"version"`
		VersionCreated string   `json:"version_created_at"`
		Licenses       []string `json:"licenses"`
		ProjectURI     string   `json:"project_uri"`
		SourceCodeURI  string   `json:"source_code_uri"`
		Downloads      int      `json:"downloads"`
		Dependencies   struct {
			Runtime []struct {
				Name string `json:"name"`
			} `json:"runtime"`
		} `json:"dependencies"`
	}
	if err := h.fetchPackageJSON(ctx, apiURL, &p); err != nil {
		return nil, err
	}
	if strings.TrimSpace(p.Name) == "" || strings.TrimSpace(p.Version) == "" {
		return nil, fmt.Errorf("RubyGems returned incomplete package metadata")
	}
	var totalDownloads interface{}
	if p.Downloads >= 0 {
		totalDownloads = p.Downloads
	}
	var runtimeDependencies interface{}
	if p.Dependencies.Runtime != nil {
		runtimeDependencies = len(p.Dependencies.Runtime)
	}
	var readmePreview interface{}
	readmeCacheKey := "rubygems:" + strings.ToLower(p.Name) + "@" + p.Version
	if preview, ok := cachedPackageReadme(readmeCacheKey); ok {
		readmePreview = preview
	} else {
		archiveURL := "https://rubygems.org/downloads/" + url.PathEscape(p.Name+"-"+p.Version) + ".gem"
		if archive, err := h.fetchPackageBytes(ctx, archiveURL, 15<<20); err == nil {
			if preview, err := rubyGemReadmePreview(archive); err == nil && preview != "" {
				readmePreview = preview
				cachePackageReadme(readmeCacheKey, preview)
			}
		}
	}
	data := map[string]interface{}{
		"name": p.Name, "version": p.Version, "license": strings.Join(p.Licenses, ", "),
		"updated": p.VersionCreated, "downloads": totalDownloads, "weekly_downloads": nil,
		"dependencies": runtimeDependencies, "repository": p.SourceCodeURI, "readme_preview": readmePreview,
		"registry": "rubygems",
	}
	return &Answer{Type: AnswerTypePkg, Term: name, Title: p.Name, Description: p.Info,
		Content: formatPkgContentWithDetails(LangFromContext(ctx), "RubyGems", p.Name, p.Info, p.Version, strings.Join(p.Licenses, ", "), "gem install "+p.Name, data), Source: "RubyGems",
		SourceURL: "https://rubygems.org/gems/" + url.PathEscape(p.Name), Data: data}, nil
}

func rubyGemReadmePreview(gemArchive []byte) (string, error) {
	outer := tar.NewReader(bytes.NewReader(gemArchive))
	var dataArchive []byte
	for {
		header, err := outer.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
		if header.Name != "data.tar.gz" || header.Size < 0 || header.Size > 15<<20 {
			continue
		}
		dataArchive, err = io.ReadAll(io.LimitReader(outer, (15<<20)+1))
		if err != nil {
			return "", err
		}
		if len(dataArchive) > 15<<20 {
			return "", fmt.Errorf("RubyGems data archive exceeds limit")
		}
		break
	}
	if len(dataArchive) == 0 {
		return "", fmt.Errorf("RubyGems archive has no data archive")
	}
	gzipReader, err := gzip.NewReader(bytes.NewReader(dataArchive))
	if err != nil {
		return "", err
	}
	defer closeutil.Close(gzipReader)
	files, err := io.ReadAll(io.LimitReader(gzipReader, (30<<20)+1))
	if err != nil {
		return "", err
	}
	if len(files) > 30<<20 {
		return "", fmt.Errorf("RubyGems expanded archive exceeds limit")
	}
	inner := tar.NewReader(bytes.NewReader(files))
	for {
		header, err := inner.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
		name := strings.TrimPrefix(header.Name, "./")
		if header.Typeflag != tar.TypeReg || strings.Contains(name, "/") || !strings.HasPrefix(strings.ToLower(name), "readme") || header.Size < 0 || header.Size > 1<<20 {
			continue
		}
		body, err := io.ReadAll(io.LimitReader(inner, (1<<20)+1))
		if err != nil {
			return "", err
		}
		if len(body) > 1<<20 {
			return "", fmt.Errorf("RubyGems README exceeds limit")
		}
		return truncateString(strings.TrimSpace(string(body)), 2000), nil
	}
	return "", nil
}

func cachedPackageReadme(cacheKey string) (string, bool) {
	packageReadmeCache.Lock()
	defer packageReadmeCache.Unlock()
	entry, ok := packageReadmeCache.entries[cacheKey]
	if !ok || !time.Now().Before(entry.expiresAt) {
		delete(packageReadmeCache.entries, cacheKey)
		return "", false
	}
	return entry.preview, true
}

func cachePackageReadme(cacheKey, preview string) {
	packageReadmeCache.Lock()
	defer packageReadmeCache.Unlock()
	now := time.Now()
	for key, entry := range packageReadmeCache.entries {
		if !now.Before(entry.expiresAt) {
			delete(packageReadmeCache.entries, key)
		}
	}
	if _, exists := packageReadmeCache.entries[cacheKey]; !exists && len(packageReadmeCache.entries) >= 500 {
		var oldestKey string
		var oldestExpiry time.Time
		for key, entry := range packageReadmeCache.entries {
			if oldestKey == "" || entry.expiresAt.Before(oldestExpiry) {
				oldestKey = key
				oldestExpiry = entry.expiresAt
			}
		}
		delete(packageReadmeCache.entries, oldestKey)
	}
	packageReadmeCache.entries[cacheKey] = packageReadmeCacheEntry{preview: preview, expiresAt: now.Add(24 * time.Hour)}
}

func (h *PkgHandler) fetchPackageJSON(ctx context.Context, apiURL string, destination interface{}) error {
	if strings.HasPrefix(apiURL, "https://crates.io/api/") {
		if err := waitForCratesIO(ctx); err != nil {
			return err
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return err
	}
	if strings.HasPrefix(apiURL, "https://crates.io/api/") {
		req.Header.Set("User-Agent", "search/"+version.Version+" (+https://scour.li)")
	} else {
		req.Header.Set("User-Agent", version.BrowserUserAgent)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := h.client.Do(req)
	if err != nil {
		return err
	}
	defer closeutil.Close(resp.Body)
	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("package not found")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("package registry returned HTTP %d", resp.StatusCode)
	}
	if err := decodeBoundedJSON(resp.Body, 2<<20, destination); err != nil {
		return fmt.Errorf("decode package registry response: %w", err)
	}
	return nil
}

func goModuleReadmePreview(archive []byte, modulePrefix string) (string, error) {
	reader, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		return "", err
	}
	var candidates []*zip.File
	for _, file := range reader.File {
		if file.FileInfo().IsDir() || !strings.HasPrefix(file.Name, modulePrefix) {
			continue
		}
		name := strings.TrimPrefix(file.Name, modulePrefix)
		if strings.Contains(name, "/") || !strings.HasPrefix(strings.ToLower(name), "readme") {
			continue
		}
		candidates = append(candidates, file)
	}
	slices.SortFunc(candidates, func(left, right *zip.File) int {
		leftName := strings.ToLower(strings.TrimPrefix(left.Name, modulePrefix))
		rightName := strings.ToLower(strings.TrimPrefix(right.Name, modulePrefix))
		leftRank, rightRank := 1, 1
		if leftName == "readme.md" || leftName == "readme" {
			leftRank = 0
		}
		if rightName == "readme.md" || rightName == "readme" {
			rightRank = 0
		}
		return leftRank - rightRank
	})
	for _, file := range candidates {
		if file.UncompressedSize64 > 1<<20 {
			continue
		}
		content, err := file.Open()
		if err != nil {
			continue
		}
		body, readErr := io.ReadAll(io.LimitReader(content, (1<<20)+1))
		closeErr := content.Close()
		if readErr != nil || closeErr != nil || len(body) > 1<<20 {
			continue
		}
		return truncateString(strings.TrimSpace(string(body)), 2000), nil
	}
	return "", nil
}

func waitForCratesIO(ctx context.Context) error {
	cratesIOThrottle.Lock()
	defer cratesIOThrottle.Unlock()
	if delay := time.Until(cratesIOThrottle.lastRequest.Add(time.Second)); delay > 0 {
		timer := time.NewTimer(delay)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
		}
	}
	cratesIOThrottle.lastRequest = time.Now()
	return nil
}

func (h *PkgHandler) fetchPackageText(ctx context.Context, apiURL string, maximumBytes int64) (string, error) {
	body, err := h.fetchPackageBytes(ctx, apiURL, maximumBytes)
	return string(body), err
}

func (h *PkgHandler) fetchPackageBytes(ctx context.Context, apiURL string, maximumBytes int64) ([]byte, error) {
	if strings.HasPrefix(apiURL, "https://crates.io/api/") {
		if err := waitForCratesIO(ctx); err != nil {
			return nil, err
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}
	if strings.HasPrefix(apiURL, "https://crates.io/api/") {
		req.Header.Set("User-Agent", "search/"+version.Version+" (+https://scour.li)")
	} else {
		req.Header.Set("User-Agent", version.BrowserUserAgent)
	}
	resp, err := h.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer closeutil.Close(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("package registry returned HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maximumBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > maximumBytes {
		return nil, fmt.Errorf("package registry response exceeds %d bytes", maximumBytes)
	}
	return body, nil
}

func formatPkgContent(registry, name, description, version, license, installCmd string) string {
	return formatPkgContentWithDetails("en", registry, name, description, version, license, installCmd, nil)
}

func formatPkgContentWithDetails(lang, registry, name, description, version, license, installCmd string, details map[string]interface{}) string {
	var html strings.Builder
	html.WriteString("<div class=\"pkg-content\">")
	fmt.Fprintf(&html, "<h1>%s</h1>", escapeHTML(name))
	fmt.Fprintf(&html, "<p class=\"registry-badge\">%s</p>", escapeHTML(registry))

	if description != "" {
		fmt.Fprintf(&html, "<p class=\"description\">%s</p>", escapeHTML(description))
	}

	html.WriteString("<dl class=\"pkg-details\">")
	if version != "" {
		fmt.Fprintf(&html, "<dt>Version</dt><dd><code>%s</code></dd>", escapeHTML(version))
	}
	if license != "" {
		fmt.Fprintf(&html, "<dt>License</dt><dd>%s</dd>", escapeHTML(license))
	} else if details["license"] == nil || details["license"] == "" {
		fmt.Fprintf(&html, "<dt>License</dt><dd>%s</dd>", escapeHTML(i18n.T(lang, "direct.pkg_unavailable")))
	}
	for _, field := range []struct {
		key   string
		label string
	}{
		{"weekly_downloads", i18n.T(lang, "direct.pkg_weekly_downloads")},
		{"dependencies", i18n.T(lang, "direct.pkg_dependencies")},
		{"updated", i18n.T(lang, "direct.pkg_last_updated")},
		{"repository", i18n.T(lang, "direct.pkg_repository")},
	} {
		value, ok := details[field.key]
		if !ok {
			continue
		}
		if value == nil || value == "" {
			fmt.Fprintf(&html, "<dt>%s</dt><dd>%s</dd>", field.label, escapeHTML(i18n.T(lang, "direct.pkg_unavailable")))
			continue
		}
		if field.key == "repository" {
			repositoryURL, valid := value.(string)
			if !valid {
				fmt.Fprintf(&html, "<dt>%s</dt><dd>%s</dd>", field.label, escapeHTML(i18n.T(lang, "direct.pkg_unavailable")))
				continue
			}
			parsed, err := url.Parse(repositoryURL)
			if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" || parsed.User != nil {
				fmt.Fprintf(&html, "<dt>%s</dt><dd>%s</dd>", field.label, escapeHTML(i18n.T(lang, "direct.pkg_unavailable")))
				continue
			}
			fmt.Fprintf(&html, "<dt>%s</dt><dd><a href=\"%s\" rel=\"nofollow noopener\">%s</a></dd>", field.label, escapeHTML(repositoryURL), escapeHTML(repositoryURL))
			continue
		}
		fmt.Fprintf(&html, "<dt>%s</dt><dd>%s</dd>", field.label, escapeHTML(fmt.Sprint(value)))
	}
	html.WriteString("</dl>")
	if previewValue, ok := details["readme_preview"]; ok {
		fmt.Fprintf(&html, "<details class=\"pkg-readme-preview\"><summary>%s</summary><pre><code>", escapeHTML(i18n.T(lang, "direct.pkg_readme_preview")))
		if preview, valid := previewValue.(string); valid && preview != "" {
			html.WriteString(escapeHTML(preview))
		} else {
			html.WriteString(escapeHTML(i18n.T(lang, "direct.pkg_unavailable")))
		}
		html.WriteString("</code></pre></details>")
	}

	html.WriteString("<h2>Install</h2>")
	fmt.Fprintf(&html, "<pre><code>%s</code></pre>", escapeHTML(installCmd))
	html.WriteString("<button type=\"button\" class=\"copy-btn\">Copy</button>")

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

func (h *CVEHandler) HandleDirectQuery(ctx context.Context, term string) (*Answer, error) {
	term = strings.TrimSpace(strings.ToUpper(term))
	if term == "" {
		return nil, fmt.Errorf("CVE ID required")
	}

	lang := LangFromContext(ctx)

	// Normalize CVE ID
	if !strings.HasPrefix(term, "CVE-") {
		term = "CVE-" + term
	}
	if !regexp.MustCompile(`^CVE-[0-9]{4}-[0-9]{4,}$`).MatchString(term) {
		return nil, fmt.Errorf("invalid CVE ID")
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
	defer closeutil.Close(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("nvd returned HTTP %d", resp.StatusCode)
	}

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
					CVSSV30 []struct {
						CVSS struct {
							BaseScore    float64 `json:"baseScore"`
							BaseSeverity string  `json:"baseSeverity"`
							VectorString string  `json:"vectorString"`
						} `json:"cvssData"`
					} `json:"cvssMetricV30"`
					CVSSV2 []struct {
						CVSS struct {
							BaseScore    float64 `json:"baseScore"`
							VectorString string  `json:"vectorString"`
						} `json:"cvssData"`
						Severity string `json:"baseSeverity"`
					} `json:"cvssMetricV2"`
				} `json:"metrics"`
				Configurations []struct {
					Nodes []struct {
						CPEMatch []struct {
							Vulnerable            bool   `json:"vulnerable"`
							Criteria              string `json:"criteria"`
							VersionStartIncluding string `json:"versionStartIncluding"`
							VersionStartExcluding string `json:"versionStartExcluding"`
							VersionEndIncluding   string `json:"versionEndIncluding"`
							VersionEndExcluding   string `json:"versionEndExcluding"`
						} `json:"cpeMatch"`
					} `json:"nodes"`
				} `json:"configurations"`
				Published    string `json:"published"`
				LastModified string `json:"lastModified"`
				References   []struct {
					URL  string   `json:"url"`
					Tags []string `json:"tags"`
				} `json:"references"`
			} `json:"cve"`
		} `json:"vulnerabilities"`
	}

	if err := decodeBoundedJSON(resp.Body, 2<<20, &result); err != nil {
		return nil, err
	}

	if len(result.Vulnerabilities) == 0 {
		return &Answer{
			Type:        AnswerTypeCVE,
			Term:        term,
			Title:       term,
			Description: i18n.T(lang, "direct.cve_not_found"),
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
	} else if len(cve.Metrics.CVSSV30) > 0 {
		cvssScore = cve.Metrics.CVSSV30[0].CVSS.BaseScore
		severity = cve.Metrics.CVSSV30[0].CVSS.BaseSeverity
		vector = cve.Metrics.CVSSV30[0].CVSS.VectorString
	} else if len(cve.Metrics.CVSSV2) > 0 {
		cvssScore = cve.Metrics.CVSSV2[0].CVSS.BaseScore
		severity = cve.Metrics.CVSSV2[0].Severity
		vector = cve.Metrics.CVSSV2[0].CVSS.VectorString
	}

	affected := make([]string, 0)
	for _, configuration := range cve.Configurations {
		for _, node := range configuration.Nodes {
			for _, match := range node.CPEMatch {
				if !match.Vulnerable {
					continue
				}
				versions := make([]string, 0, 2)
				if match.VersionStartIncluding != "" {
					versions = append(versions, ">="+match.VersionStartIncluding)
				}
				if match.VersionStartExcluding != "" {
					versions = append(versions, ">"+match.VersionStartExcluding)
				}
				if match.VersionEndIncluding != "" {
					versions = append(versions, "<="+match.VersionEndIncluding)
				}
				if match.VersionEndExcluding != "" {
					versions = append(versions, "<"+match.VersionEndExcluding)
				}
				label := match.Criteria
				if len(versions) > 0 {
					label += " (" + strings.Join(versions, ", ") + ")"
				}
				affected = append(affected, label)
			}
		}
	}
	refs := make([]string, 0, 10)
	remediation := make([]string, 0)
	exploitAvailable := false
	for _, ref := range cve.References {
		if !strings.HasPrefix(strings.ToLower(ref.URL), "https://") && !strings.HasPrefix(strings.ToLower(ref.URL), "http://") {
			continue
		}
		if len(refs) < 10 {
			refs = append(refs, ref.URL)
		}
		isRemediation := false
		for _, tag := range ref.Tags {
			switch strings.ToLower(tag) {
			case "exploit":
				exploitAvailable = true
			case "patch", "vendor advisory":
				isRemediation = true
			}
		}
		if isRemediation {
			remediation = append(remediation, ref.URL)
		}
	}

	data := map[string]interface{}{
		"id":                cve.ID,
		"description":       description,
		"cvss":              cvssScore,
		"severity":          severity,
		"vector":            vector,
		"published":         cve.Published,
		"modified":          cve.LastModified,
		"references":        refs,
		"affected":          affected,
		"remediation":       remediation,
		"exploit_available": exploitAvailable,
	}

	return &Answer{
		Type:        AnswerTypeCVE,
		Term:        term,
		Title:       cve.ID,
		Description: description,
		Content:     formatCVEContentDetails(cve.ID, description, cvssScore, severity, vector, cve.Published, cve.LastModified, affected, remediation, exploitAvailable, refs),
		Source:      "NVD",
		SourceURL:   fmt.Sprintf("https://nvd.nist.gov/vuln/detail/%s", cve.ID),
		Data:        data,
	}, nil
}

func formatCVEContentDetails(id, description string, cvss float64, severity, vector, published, modified string, affected, remediation []string, exploitAvailable bool, refs []string) string {
	html := formatCVEContent(id, description, cvss, severity, vector, published, nil)
	body := strings.TrimSuffix(html, "</div>")
	var out strings.Builder
	out.WriteString(body)
	if len(affected) > 0 {
		out.WriteString("<h2>Affected products and versions</h2><ul>")
		for _, item := range affected {
			out.WriteString("<li>" + escapeHTML(item) + "</li>")
		}
		out.WriteString("</ul>")
	}
	if len(remediation) > 0 {
		out.WriteString("<h2>Remediation and patches</h2><ul>")
		for _, ref := range remediation {
			fmt.Fprintf(&out, "<li><a href=\"%s\" target=\"_blank\" rel=\"noopener noreferrer\">%s</a></li>", escapeHTML(ref), escapeHTML(ref))
		}
		out.WriteString("</ul>")
	}
	if exploitAvailable {
		out.WriteString("<p class=\"exploit-available\">NVD lists an exploit reference.</p>")
	}
	if modified != "" {
		out.WriteString("<p><strong>Last modified:</strong> " + escapeHTML(modified) + "</p>")
	}
	if len(refs) > 0 {
		out.WriteString("<h2>References and advisories</h2><ul>")
		for _, ref := range refs {
			fmt.Fprintf(&out, "<li><a href=\"%s\" target=\"_blank\" rel=\"noopener noreferrer\">%s</a></li>", escapeHTML(ref), escapeHTML(ref))
		}
		out.WriteString("</ul>")
	}
	out.WriteString("</div>")
	return out.String()
}

func formatCVEContent(id, description string, cvss float64, severity, vector, published string, refs []string) string {
	var html strings.Builder
	html.WriteString("<div class=\"cve-content\">")
	fmt.Fprintf(&html, "<h1>%s</h1>", escapeHTML(id))

	// Severity badge
	severityClass := strings.ToLower(severity)
	if severityClass == "" {
		severityClass = "unknown"
	}
	fmt.Fprintf(&html, "<p class=\"severity %s\">%s (CVSS %.1f)</p>", severityClass, severity, cvss)

	// Description
	fmt.Fprintf(&html, "<p class=\"description\">%s</p>", escapeHTML(description))

	// Details
	html.WriteString("<dl class=\"cve-details\">")
	if vector != "" {
		fmt.Fprintf(&html, "<dt>CVSS Vector</dt><dd><code>%s</code></dd>", escapeHTML(vector))
	}
	if published != "" {
		fmt.Fprintf(&html, "<dt>Published</dt><dd>%s</dd>", escapeHTML(published))
	}
	html.WriteString("</dl>")

	// References
	if len(refs) > 0 {
		html.WriteString("<h2>References</h2><ul>")
		for _, ref := range refs {
			fmt.Fprintf(&html, "<li><a href=\"%s\" target=\"_blank\" rel=\"noopener\">%s</a></li>",
				escapeHTML(ref), escapeHTML(ref))
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

func (h *RFCHandler) HandleDirectQuery(ctx context.Context, term string) (*Answer, error) {
	term = strings.TrimSpace(term)
	if term == "" {
		return nil, fmt.Errorf("RFC number required")
	}

	lang := LangFromContext(ctx)

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
		return h.basicRFCInfo(term, lang)
	}
	defer closeutil.Close(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return h.basicRFCInfo(term, lang)
	}

	var doc struct {
		Title    string `json:"title"`
		Name     string `json:"name"`
		Abstract string `json:"abstract"`
		Authors  []struct {
			Name string `json:"name"`
		} `json:"authors"`
		Time string `json:"time"`
	}

	if err := decodeBoundedJSON(resp.Body, 2<<20, &doc); err != nil {
		return h.basicRFCInfo(term, lang)
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

func (h *RFCHandler) basicRFCInfo(number, lang string) (*Answer, error) {
	return &Answer{
		Type:        AnswerTypeRFC,
		Term:        number,
		Title:       fmt.Sprintf("RFC %s", number),
		Description: i18n.T(lang, "direct.rfc_document"),
		Content:     formatBasicRFCContent(number),
		Source:      "IETF",
		SourceURL:   fmt.Sprintf("https://www.rfc-editor.org/rfc/rfc%s", number),
	}, nil
}

func formatRFCContent(number, title, abstract string, authors []string, date string) string {
	var html strings.Builder
	html.WriteString("<div class=\"rfc-content\">")
	fmt.Fprintf(&html, "<h1>RFC %s</h1>", escapeHTML(number))
	fmt.Fprintf(&html, "<h2>%s</h2>", escapeHTML(title))

	if len(authors) > 0 {
		html.WriteString("<p class=\"authors\"><strong>Authors:</strong> ")
		html.WriteString(escapeHTML(strings.Join(authors, ", ")))
		html.WriteString("</p>")
	}

	if date != "" {
		fmt.Fprintf(&html, "<p class=\"date\"><strong>Date:</strong> %s</p>", escapeHTML(date))
	}

	if abstract != "" {
		html.WriteString("<h3>Abstract</h3>")
		fmt.Fprintf(&html, "<p>%s</p>", escapeHTML(abstract))
	}

	html.WriteString("<h3>Read Full Document</h3>")
	html.WriteString("<ul>")
	fmt.Fprintf(&html, "<li><a href=\"https://www.rfc-editor.org/rfc/rfc%s.html\">HTML</a></li>", number)
	fmt.Fprintf(&html, "<li><a href=\"https://www.rfc-editor.org/rfc/rfc%s.txt\">Plain Text</a></li>", number)
	fmt.Fprintf(&html, "<li><a href=\"https://www.rfc-editor.org/rfc/rfc%s.pdf\">PDF</a></li>", number)
	html.WriteString("</ul>")

	html.WriteString("</div>")
	return html.String()
}

func formatBasicRFCContent(number string) string {
	var html strings.Builder
	html.WriteString("<div class=\"rfc-content\">")
	fmt.Fprintf(&html, "<h1>RFC %s</h1>", escapeHTML(number))

	html.WriteString("<h3>Read Full Document</h3>")
	html.WriteString("<ul>")
	fmt.Fprintf(&html, "<li><a href=\"https://www.rfc-editor.org/rfc/rfc%s.html\">HTML</a></li>", number)
	fmt.Fprintf(&html, "<li><a href=\"https://www.rfc-editor.org/rfc/rfc%s.txt\">Plain Text</a></li>", number)
	fmt.Fprintf(&html, "<li><a href=\"https://www.rfc-editor.org/rfc/rfc%s.pdf\">PDF</a></li>", number)
	html.WriteString("</ul>")

	html.WriteString("</div>")
	return html.String()
}

// DirectoryHandler handles directory:{term} queries (open directory search)
type DirectoryFile struct {
	Name     string `json:"name"`
	URL      string `json:"url"`
	Size     string `json:"size,omitempty"`
	Modified string `json:"modified,omitempty"`
}

var directorySizePattern = regexp.MustCompile(`(?i)^\d+(?:\.\d+)?\s*(?:[kmgtpe]i?b?)?$`)

type DirectoryResult struct {
	Title              string
	URL                string
	Description        string
	ServerType         string          `json:"server_type,omitempty"`
	EstimatedFileCount int             `json:"estimated_file_count,omitempty"`
	Files              []DirectoryFile `json:"files,omitempty"`
}

type DirectoryFilters struct {
	FileType    string `json:"file_type,omitempty"`
	Site        string `json:"site,omitempty"`
	MinimumSize int64  `json:"minimum_size,omitempty"`
	After       string `json:"after,omitempty"`
	Before      string `json:"before,omitempty"`
	ScanIndex   int    `json:"scan_index,omitempty"`
}

type DirectorySearcher func(context.Context, string) ([]DirectoryResult, error)

var directorySearcherOverride DirectorySearcher

type DirectoryHandler struct {
	client   *http.Client
	searcher DirectorySearcher
}

// NewDirectoryHandler creates a new directory search handler
func NewDirectoryHandler() *DirectoryHandler {
	return &DirectoryHandler{client: newPublicHTTPClient(5 * time.Second), searcher: directorySearcherOverride}
}

func validatePublicHTTPURL(rawURL string) (*url.URL, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	if (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" || parsed.User != nil {
		return nil, fmt.Errorf("URL must use HTTP or HTTPS and must not contain user information")
	}
	return parsed, nil
}

var publicNetworkDialOverride = dialPublicDirectoryHost
var publicNetworkTLSConfig *tls.Config
var publicNetworkTransportOverride http.RoundTripper
var publicDirectoryLookup = net.DefaultResolver.LookupIPAddr

func newPublicHTTPClient(timeout time.Duration) *http.Client {
	var roundTripper http.RoundTripper
	if publicNetworkTransportOverride != nil {
		roundTripper = publicNetworkTransportOverride
	} else {
		transport, ok := http.DefaultTransport.(*http.Transport)
		if !ok {
			transport = &http.Transport{}
		} else {
			transport = transport.Clone()
		}
		transport.Proxy = nil
		transport.DialContext = publicNetworkDialOverride
		if publicNetworkTLSConfig != nil {
			transport.TLSClientConfig = publicNetworkTLSConfig.Clone()
		}
		transport.MaxResponseHeaderBytes = 1 << 20
		roundTripper = transport
	}
	return &http.Client{
		Timeout:   timeout,
		Transport: roundTripper,
		CheckRedirect: func(request *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("too many redirects")
			}
			if request.URL.Scheme != "http" && request.URL.Scheme != "https" {
				return fmt.Errorf("redirect uses a disallowed URL scheme")
			}
			return validatePublicDirectoryHost(request.Context(), request.URL.Hostname())
		},
	}
}

func (h *DirectoryHandler) Type() AnswerType {
	return AnswerTypeDirectory
}

func (h *DirectoryHandler) SetSearcher(searcher DirectorySearcher) {
	h.searcher = searcher
}

func (h *DirectoryHandler) HandleDirectQuery(ctx context.Context, term string) (*Answer, error) {
	term = strings.TrimSpace(term)
	if term == "" {
		return nil, fmt.Errorf("search term required")
	}

	lang := LangFromContext(ctx)

	searchTerm, filters, err := parseDirectoryQuery(term)
	if err != nil {
		return nil, err
	}
	if searchTerm == "" {
		return nil, fmt.Errorf("search term required")
	}
	searchQuery := fmt.Sprintf(`intitle:"index of" OR intitle:"directory of" "%s" -html -htm -php -asp`, searchTerm)
	if filters.Site != "" {
		searchQuery += " site:" + filters.Site
	}

	// Detect file type from term
	fileExt := ""
	lowerTerm := strings.ToLower(searchTerm)
	if strings.Contains(lowerTerm, "mp3") || strings.Contains(lowerTerm, "music") || strings.Contains(lowerTerm, "audio") {
		fileExt = "mp3 OR flac OR wav OR ogg"
	} else if strings.Contains(lowerTerm, "mp4") || strings.Contains(lowerTerm, "video") || strings.Contains(lowerTerm, "movie") {
		fileExt = "mp4 OR mkv OR avi OR webm"
	} else if strings.Contains(lowerTerm, "pdf") || strings.Contains(lowerTerm, "book") || strings.Contains(lowerTerm, "ebook") {
		fileExt = "pdf OR epub OR mobi"
	} else if strings.Contains(lowerTerm, "iso") {
		fileExt = "iso"
	}

	if filters.FileType == "" && fileExt != "" {
		searchQuery += " " + fileExt
	}
	if filters.FileType != "" {
		extensions := directoryTypeExtensions(filters.FileType)
		if len(extensions) > 0 {
			searchQuery += " (" + strings.Join(extensions, " OR ") + ")"
		}
	}

	var results []DirectoryResult
	if h.searcher != nil {
		results, err = h.searcher(ctx, searchQuery)
		if err != nil {
			return nil, fmt.Errorf("search open directories: %w", err)
		}
	}
	for index := range results {
		if index >= 3 {
			break
		}
		if !strings.Contains(strings.ToLower(results[index].Title+" "+results[index].Description), "index of") && !strings.Contains(strings.ToLower(results[index].Title+" "+results[index].Description), "directory listing") {
			continue
		}
		limit := 20
		if filters.ScanIndex == index+1 {
			limit = 500
		}
		files, serverType, estimatedFileCount := h.scrapeDirectory(ctx, results[index].URL, limit)
		results[index].Files = files
		results[index].ServerType = serverType
		results[index].EstimatedFileCount = estimatedFileCount
		results[index].Files = filterDirectoryFiles(results[index].Files, filters)
	}
	data := map[string]interface{}{"term": searchTerm, "query": searchQuery, "filters": filters, "results": results}
	return &Answer{
		Type:        AnswerTypeDirectory,
		Term:        searchTerm,
		Title:       fmt.Sprintf("Directory Search: %s", searchTerm),
		Description: i18n.T(lang, "direct.directory_search_description"),
		Content:     formatDirectoryResults(searchTerm, searchQuery, results, filters),
		Source:      "Google, Bing, DuckDuckGo",
		Data:        data,
	}, nil
}

var directoryFilterPattern = regexp.MustCompile(`(?i)(?:^|\s)(type|min-size|after|before|site|scan):(\S+)`)

func parseDirectoryQuery(term string) (string, DirectoryFilters, error) {
	filters := DirectoryFilters{}
	query := term
	for _, match := range directoryFilterPattern.FindAllStringSubmatch(term, -1) {
		value := match[2]
		switch strings.ToLower(match[1]) {
		case "type":
			value = strings.ToLower(value)
			if len(directoryTypeExtensions(value)) == 0 {
				return "", filters, fmt.Errorf("unsupported directory file type %q", value)
			}
			filters.FileType = value
		case "min-size":
			size, err := parseDirectorySize(value)
			if err != nil {
				return "", filters, fmt.Errorf("invalid minimum file size: %w", err)
			}
			filters.MinimumSize = size
		case "after", "before":
			if _, err := time.Parse("2006-01-02", value); err != nil {
				return "", filters, fmt.Errorf("invalid directory date %q; use YYYY-MM-DD", value)
			}
			if strings.EqualFold(match[1], "after") {
				filters.After = value
			} else {
				filters.Before = value
			}
		case "site":
			if strings.ContainsAny(value, "/:@") || value == "" {
				return "", filters, fmt.Errorf("invalid directory site restriction")
			}
			filters.Site = value
		case "scan":
			scanIndex, err := strconv.Atoi(value)
			if err != nil || scanIndex < 1 || scanIndex > 3 {
				return "", filters, fmt.Errorf("scan index must be between 1 and 3")
			}
			filters.ScanIndex = scanIndex
		}
	}
	query = directoryFilterPattern.ReplaceAllString(query, " ")
	query = strings.Join(strings.Fields(query), " ")
	return query, filters, nil
}

func directoryTypeExtensions(fileType string) []string {
	switch strings.ToLower(fileType) {
	case "audio":
		return []string{"mp3", "flac", "wav", "ogg", "m4a", "opus"}
	case "video":
		return []string{"mp4", "mkv", "avi", "webm", "mov", "m4v"}
	case "documents":
		return []string{"pdf", "epub", "mobi", "doc", "docx", "txt", "odt"}
	case "archives":
		return []string{"zip", "rar", "7z", "tar", "gz", "bz2", "xz"}
	case "images":
		return []string{"jpg", "jpeg", "png", "gif", "webp", "svg", "avif"}
	default:
		return nil
	}
}

func parseDirectorySize(value string) (int64, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	var multiplier float64 = 1
	for _, unit := range []struct {
		suffix string
		factor float64
	}{{"kib", 1 << 10}, {"kb", 1 << 10}, {"k", 1 << 10}, {"mib", 1 << 20}, {"mb", 1 << 20}, {"m", 1 << 20}, {"gib", 1 << 30}, {"gb", 1 << 30}, {"g", 1 << 30}, {"b", 1}} {
		suffix, factor := unit.suffix, unit.factor
		if strings.HasSuffix(value, suffix) {
			value = strings.TrimSpace(strings.TrimSuffix(value, suffix))
			multiplier = factor
			break
		}
	}
	amount, err := strconv.ParseFloat(value, 64)
	if err != nil || amount <= 0 || amount > float64(math.MaxInt64)/multiplier {
		return 0, fmt.Errorf("size must be a positive value such as 100M")
	}
	return int64(amount * multiplier), nil
}

func parseDirectoryTimestamp(value string) time.Time {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\u00a0", " "))
	for _, layout := range []string{
		"2006-01-02 15:04", "2006-01-02 15:04:05", time.RFC3339,
		"02-Jan-2006 15:04", "02-Jan-2006 15:04:05", "2006-01-02",
	} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed
		}
	}
	return time.Time{}
}

func filterDirectoryFiles(files []DirectoryFile, filters DirectoryFilters) []DirectoryFile {
	filtered := make([]DirectoryFile, 0, len(files))
	for _, file := range files {
		if filters.FileType != "" {
			extension := strings.TrimPrefix(strings.ToLower(path.Ext(file.Name)), ".")
			if !slices.Contains(directoryTypeExtensions(filters.FileType), extension) {
				continue
			}
		}
		if filters.MinimumSize > 0 {
			size, err := parseDirectorySize(file.Size)
			if err != nil || size < filters.MinimumSize {
				continue
			}
		}
		if filters.After != "" || filters.Before != "" {
			modified := parseDirectoryTimestamp(file.Modified)
			if modified.IsZero() {
				continue
			}
			if filters.After != "" {
				after, _ := time.Parse("2006-01-02", filters.After)
				if modified.Before(after) {
					continue
				}
			}
			if filters.Before != "" {
				before, _ := time.Parse("2006-01-02", filters.Before)
				if modified.After(before.Add(24*time.Hour - time.Nanosecond)) {
					continue
				}
			}
		}
		filtered = append(filtered, file)
	}
	return filtered
}

func (h *DirectoryHandler) scrapeDirectory(ctx context.Context, rawURL string, limit int) ([]DirectoryFile, string, int) {
	parsed, err := validatePublicHTTPURL(rawURL)
	if err != nil {
		return nil, "", 0
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return nil, "", 0
	}
	request.Header.Set("User-Agent", version.BrowserUserAgent)
	response, err := h.client.Do(request)
	if err != nil {
		return nil, "", 0
	}
	defer closeutil.Close(response.Body)
	if response.StatusCode != http.StatusOK || !strings.HasPrefix(strings.ToLower(response.Header.Get("Content-Type")), "text/html") {
		return nil, response.Header.Get("Server"), 0
	}
	document, err := xhtml.Parse(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return nil, response.Header.Get("Server"), 0
	}
	if limit < 1 || limit > 500 {
		limit = 20
	}
	files := make([]DirectoryFile, 0, min(limit, 20))
	estimatedFileCount := 0
	var visit func(*xhtml.Node)
	visit = func(node *xhtml.Node) {
		if node.Type == xhtml.ElementNode && node.Data == "a" {
			for _, attr := range node.Attr {
				if attr.Key != "href" || attr.Val == "" || strings.HasPrefix(attr.Val, "#") || strings.HasPrefix(attr.Val, "?") {
					continue
				}
				link, err := parsed.Parse(attr.Val)
				if err != nil || (link.Host != "" && !strings.EqualFold(link.Host, parsed.Host)) {
					continue
				}
				name := strings.TrimSpace(nodeText(node))
				if name == "" {
					name = link.Path
				}
				if name == "../" || name == ".." || name == "/" || strings.HasSuffix(link.Path, "/") {
					continue
				}
				estimatedFileCount++
				modified, size := directoryFileMetadata(node)
				if len(files) < limit {
					files = append(files, DirectoryFile{Name: name, URL: parsed.ResolveReference(link).String(), Size: size, Modified: modified})
				}
			}
		}
		for child := node.FirstChild; child != nil && estimatedFileCount < 10000; child = child.NextSibling {
			visit(child)
		}
	}
	visit(document)
	return files, truncateString(response.Header.Get("Server"), 200), estimatedFileCount
}

func directoryFileMetadata(link *xhtml.Node) (string, string) {
	var row *xhtml.Node
	for ancestor := link.Parent; ancestor != nil; ancestor = ancestor.Parent {
		if ancestor.Type == xhtml.ElementNode && ancestor.Data == "tr" {
			row = ancestor
			break
		}
	}
	if row == nil {
		return "", ""
	}
	var modified, size string
	var inspect func(*xhtml.Node)
	inspect = func(node *xhtml.Node) {
		if node.Type == xhtml.ElementNode && node.Data == "td" {
			value := strings.TrimSpace(strings.ReplaceAll(nodeText(node), "\u00a0", ""))
			if value == "" {
				return
			}
			if modified == "" && isDirectoryTimestamp(value) {
				modified = value
				return
			}
			if size == "" && directorySizePattern.MatchString(value) {
				size = value
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			inspect(child)
		}
	}
	inspect(row)
	return modified, size
}

func isDirectoryTimestamp(value string) bool {
	return !parseDirectoryTimestamp(value).IsZero()
}

func nodeText(node *xhtml.Node) string {
	if node.Type == xhtml.TextNode {
		return node.Data
	}
	var text strings.Builder
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		text.WriteString(nodeText(child))
	}
	return text.String()
}

func dialPublicDirectoryHost(ctx context.Context, network, address string) (net.Conn, error) {
	return dialPublicDirectoryHostWithLookup(ctx, network, address, publicDirectoryLookup)
}

func dialPublicDirectoryHostWithLookup(ctx context.Context, network, address string, lookup func(context.Context, string) ([]net.IPAddr, error)) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	if err := validatePublicDirectoryHostWithLookup(ctx, host, lookup); err != nil {
		return nil, err
	}
	addresses, err := lookup(ctx, host)
	if ip := net.ParseIP(host); ip != nil {
		addresses = []net.IPAddr{{IP: ip}}
		err = nil
	}
	if err != nil {
		return nil, err
	}
	for _, address := range addresses {
		if !isPublicDirectoryIP(address.IP) {
			continue
		}
		return (&net.Dialer{Timeout: 3 * time.Second}).DialContext(ctx, network, net.JoinHostPort(address.IP.String(), port))
	}
	return nil, fmt.Errorf("directory host has no public address")
}

func validatePublicDirectoryHost(ctx context.Context, host string) error {
	return validatePublicDirectoryHostWithLookup(ctx, host, publicDirectoryLookup)
}

func validatePublicDirectoryHostWithLookup(ctx context.Context, host string, lookup func(context.Context, string) ([]net.IPAddr, error)) error {
	if host == "" {
		return fmt.Errorf("directory host is empty")
	}
	if ip := net.ParseIP(host); ip != nil {
		if !isPublicDirectoryIP(ip) {
			return fmt.Errorf("directory host is not public")
		}
		return nil
	}
	ips, err := lookup(ctx, host)
	if err != nil {
		return err
	}
	if len(ips) == 0 {
		return fmt.Errorf("directory host has no address")
	}
	for _, ip := range ips {
		if !isPublicDirectoryIP(ip.IP) {
			return fmt.Errorf("directory host resolves to a non-public address")
		}
	}
	return nil
}

func isPublicDirectoryIP(ip net.IP) bool {
	if ip == nil || !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() {
		return false
	}
	for _, cidr := range []string{"100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "2001:db8::/32"} {
		_, network, _ := net.ParseCIDR(cidr)
		if network.Contains(ip) {
			return false
		}
	}
	return true
}

func formatDirectoryResults(term, query string, results []DirectoryResult, filters DirectoryFilters) string {
	var html strings.Builder
	html.WriteString("<div class=\"directory-content\">")
	fmt.Fprintf(&html, "<h1>Open Directory Search: %s</h1>", escapeHTML(term))
	fmt.Fprintf(&html, "<form class=\"directory-filter-form\" method=\"get\" action=\"/search\" data-query=\"%s\">", escapeHTML(term))
	fmt.Fprintf(&html, "<input type=\"hidden\" name=\"q\" value=\"%s\">", escapeHTML(directoryQueryString(term, DirectoryFilters{})))
	html.WriteString("<fieldset><legend>Filter files</legend>")
	html.WriteString("<label>File type <select name=\"directory_type\"><option value=\"\">Any</option>")
	for _, option := range []struct{ value, label string }{{"audio", "Audio"}, {"video", "Video"}, {"documents", "Documents"}, {"archives", "Archives"}, {"images", "Images"}} {
		selected := ""
		if filters.FileType == option.value {
			selected = " selected"
		}
		fmt.Fprintf(&html, "<option value=\"%s\"%s>%s</option>", option.value, selected, option.label)
	}
	html.WriteString("</select></label>")
	minimumSize := ""
	if filters.MinimumSize > 0 {
		minimumSize = strconv.FormatInt(filters.MinimumSize, 10)
	}
	fmt.Fprintf(&html, "<label>Minimum size (bytes) <input name=\"directory_min_size\" inputmode=\"numeric\" value=\"%s\"></label>", escapeHTML(minimumSize))
	fmt.Fprintf(&html, "<label>Modified after <input type=\"date\" name=\"directory_after\" value=\"%s\"></label>", escapeHTML(filters.After))
	fmt.Fprintf(&html, "<label>Modified before <input type=\"date\" name=\"directory_before\" value=\"%s\"></label>", escapeHTML(filters.Before))
	fmt.Fprintf(&html, "<label>Site <input name=\"directory_site\" value=\"%s\"></label>", escapeHTML(filters.Site))
	fmt.Fprintf(&html, "<input type=\"hidden\" name=\"directory_scan\" value=\"%d\">", filters.ScanIndex)
	html.WriteString("<button type=\"submit\">Apply filters</button></fieldset></form>")
	if len(results) == 0 {
		html.WriteString("<p>No open directory listings were found.</p>")
	} else {
		html.WriteString("<h2>Discovered listings</h2><ol>")
		for index, result := range results {
			parsed, err := url.Parse(result.URL)
			if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" {
				continue
			}
			title := result.Title
			if title == "" {
				title = parsed.Host
			}
			fmt.Fprintf(&html, "<li><a href=\"%s\" rel=\"nofollow noopener\">%s</a>", escapeHTML(result.URL), escapeHTML(title))
			if result.Description != "" {
				fmt.Fprintf(&html, "<p>%s</p>", escapeHTML(result.Description))
			}
			if result.ServerType != "" || result.EstimatedFileCount > 0 {
				html.WriteString("<p class=\"directory-metadata\">Directory metadata: ")
				metadata := make([]string, 0, 2)
				if result.ServerType != "" {
					metadata = append(metadata, "Server: "+escapeHTML(result.ServerType))
				}
				if result.EstimatedFileCount > 0 {
					metadata = append(metadata, fmt.Sprintf("Estimated files: %d", result.EstimatedFileCount))
				}
				html.WriteString(strings.Join(metadata, " · "))
				html.WriteString("</p>")
			}
			if index < 3 {
				scanFilters := filters
				scanFilters.ScanIndex = index + 1
				scanQuery := directoryQueryString(term, scanFilters)
				fmt.Fprintf(&html, "<form class=\"directory-scan-form\" method=\"get\" action=\"/search\" data-query=\"%s\">", escapeHTML(scanQuery))
				fmt.Fprintf(&html, "<input type=\"hidden\" name=\"q\" value=\"%s\">", escapeHTML(scanQuery))
				html.WriteString("<button type=\"submit\">Scan directory</button></form>")
			}
			if len(result.Files) > 0 {
				html.WriteString("<ul class=\"directory-files\">")
				for _, file := range result.Files {
					fmt.Fprintf(&html, "<li><a href=\"%s\" rel=\"nofollow noopener\">%s</a>", escapeHTML(file.URL), escapeHTML(file.Name))
					if file.Size != "" || file.Modified != "" {
						html.WriteString(" <span class=\"directory-file-metadata\">")
						metadata := make([]string, 0, 2)
						if file.Size != "" {
							metadata = append(metadata, escapeHTML(file.Size))
						}
						if file.Modified != "" {
							metadata = append(metadata, escapeHTML(file.Modified))
						}
						html.WriteString(strings.Join(metadata, " · "))
						html.WriteString("</span>")
					}
					html.WriteString("</li>")
				}
				html.WriteString("</ul>")
			}
			html.WriteString("</li>")
		}
		html.WriteString("</ol>")
	}
	fmt.Fprintf(&html, "<details><summary>Search query</summary><pre><code>%s</code></pre></details>", escapeHTML(query))
	html.WriteString("<p class=\"note\">Results link to publicly accessible servers. Respect copyright and terms of use.</p></div>")
	return html.String()
}

func directoryQueryString(term string, filters DirectoryFilters) string {
	query := "directory:" + term
	if filters.FileType != "" {
		query += " type:" + filters.FileType
	}
	if filters.MinimumSize > 0 {
		query += " min-size:" + strconv.FormatInt(filters.MinimumSize, 10)
	}
	if filters.After != "" {
		query += " after:" + filters.After
	}
	if filters.Before != "" {
		query += " before:" + filters.Before
	}
	if filters.Site != "" {
		query += " site:" + filters.Site
	}
	if filters.ScanIndex > 0 {
		query += " scan:" + strconv.Itoa(filters.ScanIndex)
	}
	return query
}

func formatDirectoryContent(term, query string) string {
	var html strings.Builder
	html.WriteString("<div class=\"directory-content\">")
	fmt.Fprintf(&html, "<h1>Open Directory Search: %s</h1>", escapeHTML(term))

	html.WriteString("<p>Open directories are web servers with directory listing enabled, allowing direct file access.</p>")

	html.WriteString("<h2>Search Query</h2>")
	html.WriteString("<p>Use this query in a search engine:</p>")
	fmt.Fprintf(&html, "<pre><code>%s</code></pre>", escapeHTML(query))
	html.WriteString("<button type=\"button\" class=\"copy-btn\">Copy</button>")

	html.WriteString("<h2>Quick Search</h2>")
	html.WriteString("<ul>")
	fmt.Fprintf(&html, "<li><a href=\"https://www.google.com/search?q=%s\" target=\"_blank\" rel=\"noopener\">Search on Google</a></li>",
		url.QueryEscape(query))
	fmt.Fprintf(&html, "<li><a href=\"https://www.bing.com/search?q=%s\" target=\"_blank\" rel=\"noopener\">Search on Bing</a></li>",
		url.QueryEscape(query))
	fmt.Fprintf(&html, "<li><a href=\"https://duckduckgo.com/?q=%s\" target=\"_blank\" rel=\"noopener\">Search on DuckDuckGo</a></li>",
		url.QueryEscape(query))
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
