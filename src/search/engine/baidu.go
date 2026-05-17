package engine

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/apimgr/search/src/model"
	"github.com/apimgr/search/src/search"
)

// Pre-compiled regexes for Baidu HTML parsing.
//
// Strategy: use the stable mu="URL" attribute on each result container (present
// since ~2015) to obtain the real destination URL without following redirects.
// Titles and snippets are then extracted from each result block by position.
var (
	// baiduMuRe finds the real destination URL from Baidu's result container.
	// Baidu wraps links via /link?url=... but also exposes the actual URL in mu=.
	baiduMuRe = regexp.MustCompile(`\bmu="(https?://[^"]{5,500})"`)

	// baiduTitleRe matches <h3 class="c-title ..."> or <h3 class="t ..."> blocks.
	baiduTitleRe = regexp.MustCompile(`(?s)<h3[^>]*\bclass="[^"]*\b(?:c-title|t)\b[^"]*"[^>]*>([\s\S]{0,500}?)</h3>`)

	// baiduSnipRe matches the snippet container.
	baiduSnipRe = regexp.MustCompile(`(?s)<div[^>]*\bclass="[^"]*c-abstract[^"]*"[^>]*>([\s\S]{0,700}?)</div>`)

	// baiduTagRe strips HTML tags for plain-text extraction.
	baiduTagRe = regexp.MustCompile(`<[^>]+>`)
)

// Baidu implements Baidu search engine
type Baidu struct {
	*search.BaseEngine
	client *http.Client
}

// NewBaidu creates a new Baidu search engine
func NewBaidu() *Baidu {
	config := model.NewEngineConfig("baidu")
	config.DisplayName = "Baidu"
	config.Priority = 60
	config.Categories = []string{"general", "images", "news", "videos", "files", "music"}
	// Baidu blocks Tor exit nodes
	config.SupportsTor = false

	return &Baidu{
		BaseEngine: search.NewBaseEngine(config),
		client: &http.Client{
			Timeout:   time.Duration(config.GetTimeout()) * time.Second,
			Transport: SharedTransport,
		},
	}
}

// Search performs a Baidu search
func (e *Baidu) Search(ctx context.Context, query *model.Query) ([]model.Result, error) {
	searchURL := "https://www.baidu.com/s"
	params := url.Values{}
	params.Set("wd", query.Text)
	params.Set("ie", "utf-8")

	switch query.Category {
	case model.CategoryImages:
		searchURL = "https://image.baidu.com/search/index"
		params.Set("word", query.Text)
		params.Del("wd")
	case model.CategoryNews:
		searchURL = "https://news.baidu.com/ns"
		params.Set("word", query.Text)
		params.Del("wd")
	case model.CategoryVideos:
		searchURL = "https://v.baidu.com/v"
		params.Set("word", query.Text)
		params.Del("wd")
	}

	if query.Page > 1 {
		params.Set("pn", fmt.Sprintf("%d", (query.Page-1)*10))
	}

	req, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("%s?%s", searchURL, params.Encode()), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	req.Header.Set("DNT", "1")

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("baidu returned status %d", resp.StatusCode)
	}

	body, err := ReadBody(resp)
	if err != nil {
		return nil, err
	}

	return e.parseResults(string(body), query.Category)
}

// parseResults parses HTML results from Baidu.
//
// Each organic result container has a mu="real-url" attribute. We locate each
// container by mu= position, then search the following 3 KB for the h3 title
// and c-abstract snippet. This avoids following Baidu's /link?url= redirects.
func (e *Baidu) parseResults(html string, category model.Category) ([]model.Result, error) {
	muMatches := baiduMuRe.FindAllStringSubmatchIndex(html, -1)
	if len(muMatches) == 0 {
		return nil, nil
	}

	maxResults := e.GetConfig().GetMaxResults()
	results := make([]model.Result, 0, len(muMatches))

	for i, m := range muMatches {
		rawURL := html[m[2]:m[3]]

		// Search in the HTML block from this mu= to the next (or +3 KB).
		blockStart := m[0]
		blockEnd := len(html)
		if i+1 < len(muMatches) {
			blockEnd = muMatches[i+1][0]
		}
		if blockEnd > blockStart+3000 {
			blockEnd = blockStart + 3000
		}
		block := html[blockStart:blockEnd]

		// Extract title from h3.
		title := ""
		if tm := baiduTitleRe.FindStringSubmatch(block); tm != nil {
			title = strings.TrimSpace(unescapeHTML(baiduTagRe.ReplaceAllString(tm[1], " ")))
		}
		if title == "" {
			continue
		}

		// Extract snippet from c-abstract.
		content := ""
		if sm := baiduSnipRe.FindStringSubmatch(block); sm != nil {
			content = strings.TrimSpace(unescapeHTML(baiduTagRe.ReplaceAllString(sm[1], " ")))
		}

		results = append(results, model.Result{
			Title:    title,
			URL:      rawURL,
			Content:  content,
			Engine:   e.Name(),
			Category: category,
			Score:    calculateScore(e.GetPriority(), i, 1),
			Position: i,
		})

		if len(results) >= maxResults {
			break
		}
	}

	return results, nil
}
