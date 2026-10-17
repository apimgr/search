//go:build e2e

// Package e2e holds the browser end-to-end suite required by AI.md PART 28
// ("Browser E2E Testing"). It is gated behind the `e2e` build tag so the
// default `make test` run never compiles it, and it is entered through
// ./tests/e2e.sh rather than a Makefile target — PART 25 fixes the target set
// at six and forbids a seventh.
//
// The suite runs against the real router (server.Server.Handler) wrapped in an
// httptest.Server, not against a compiled binary, so it exercises the exact
// middleware chain, templates, and handlers a deployed service uses.
package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/apimgr/search/src/config"
	"github.com/apimgr/search/src/direct"
	"github.com/apimgr/search/src/server"
	"github.com/miekg/dns"
)

// e2eOperatorToken is the operator token baked into the test configuration.
// /server/status and /server/config are gated by RequireOperator (PART 11), so
// the suite needs a known token to exercise the authorized path and to assert
// that the unauthenticated path is refused.
const e2eOperatorToken = "e2e-operator-token-do-not-use-in-production"

var (
	sharedOnce  sync.Once
	sharedHTTP  *httptest.Server
	sharedTitle string
	sharedDescr string

	trackedOnce          sync.Once
	trackedHTTP          *httptest.Server
	e2eOriginalTransport http.RoundTripper
	e2eHTTPSFixture      *httptest.Server
	e2eDNSServer         *dns.Server
)

// localInstantAnswerPages lists deterministic instant answers backed by local
// algorithms or provider fixtures. Every case runs in all three browser tiers.
var localInstantAnswerPages = []struct {
	name     string
	query    string
	typeName string
}{
	{"calculator", "sqrt(144)", "math"},
	{"percentage calculator", "15% of 200", "math"},
	{"trigonometry calculator", "sin(45)", "math"},
	{"IP lookup", "8.8.8.8", "ip"},
	{"unit converter", "5 miles to km", "convert"},
	{"temperature converter", "100 fahrenheit to celsius", "convert"},
	{"current time", "current time", "time"},
	{"timezone converter", "time in tokyo", "timezone"},
	{"timezone current-time phrase", "current time london", "timezone"},
	{"date calculator", "days until 2030-01-01", "date"},
	{"hash generator", "sha256 hello", "hash"},
	{"base64 encoder", "base64 encode hello", "base64"},
	{"url encoder", "url encode hello world", "url"},
	{"color parser", "#ff0000", "color"},
	{"uuid generator", "generate uuid", "uuid"},
	{"uuid version trigger", "uuid v4", "uuid"},
	{"password generator", "generate password", "password"},
	{"password requested length", "password 16 characters", "password"},
	{"strong password", "strong password", "password"},
	{"qr generator", "qr hello world", "qr"},
	{"ascii generator", "ascii hello", "ascii"},
	{"case converter", "uppercase hello world", "case"},
	{"slug generator", "slug hello world", "slug"},
	{"lorem ipsum", "lorem ipsum 3 paragraphs", "lorem"},
	{"lorem word count", "placeholder text 100 words", "lorem"},
	{"stopwatch", "stopwatch", "time"},
	{"countdown timer", "timer 1 second", "time"},
	{"random number", "random number", "random"},
	{"weather forecast", "weather in Tokyo", "weather"},
	{"dictionary definition", `what does "ephemeral" mean`, "definition"},
	{"synonyms", "synonyms for happy", "synonym"},
	{"antonyms", "antonyms of good", "antonym"},
	{"cryptocurrency price", "bitcoin price", "crypto"},
	{"cryptocurrency amount conversion", "0.5 eth in dollars", "crypto"},
	{"stock quote", "stock AAPL", "stocks"},
	{"stock dollar syntax", "$AMZN stock price", "stocks"},
	{"translation", "translate hello to spanish", "translate"},
	{"quoted translation", `"good morning" to german`, "translate"},
	{"Wikipedia summary", "wiki Python programming", "wikipedia"},
	{"sports score", "lakers score", "sports"},
	{"nutrition facts", "calories in banana", "nutrition"},
	{"package tracking", "track 1Z999AA10123456784", "tracking"},
	{"package tracking number autodetect", "1Z999AA10123456784", "tracking"},
	{"currency conversion", "10 USD to EUR", "currency"},
}

func assertFullWikipediaFixture(t *testing.T, html string) {
	t.Helper()
	for _, marker := range []string{`class="wiki-toc"`, `class="wiki-infobox"`, `class="wiki-section"`, "Guido van Rossum", "Python began in the late 1980s.", `name="lang"`} {
		if !strings.Contains(html, marker) {
			t.Errorf("full Wikipedia answer is missing %q", marker)
		}
	}
}

type providerFixtureTransport struct{ fallback http.RoundTripper }

func (transport providerFixtureTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	var body string
	switch request.URL.Host {
	case "api.datamuse.com":
		if request.URL.Query().Has("rel_syn") {
			body = `[{"word":"joyful","score":1000,"tags":["adj"]},{"word":"cheerful","score":900,"tags":["adj"]}]`
		} else {
			body = `[{"word":"bad","score":1000,"tags":["adj"]},{"word":"poor","score":900,"tags":["adj"]}]`
		}
	case "api.urbandictionary.com":
		body = `{"list":[{"word":"yeet","definition":"To throw something with force.","example":"She yeeted the ball across the field.","thumbs_up":1200,"thumbs_down":20,"author":"fixture","written_on":"2026-01-01T00:00:00.000Z","permalink":"https://www.urbandictionary.com/define.php?term=yeet"}]}`
	case "en.wiktionary.org":
		body = `{"en":[{"partOfSpeech":"adjective","language":"English","definitions":[{"definition":"Lasting for a short time."}]},{"partOfSpeech":"noun","language":"English","definitions":[{"definition":"A short-lived thing."}]}]}`
	case "api.dictionaryapi.dev":
		body = `[{"word":"ephemeral","phonetic":"/ɪˈfem.ər.əl/","phonetics":[{"text":"/ɪˈfem.ər.əl/","audio":"https://api.dictionaryapi.dev/media/pronunciations/en/ephemeral-us.mp3"}],"meanings":[{"partOfSpeech":"adjective","definitions":[{"definition":"Lasting for a short time.","example":"The ephemeral beauty of the sunset."}]}],"origin":"From Greek ephēmeros"},{"word":"ephemeral","meanings":[{"partOfSpeech":"noun","definitions":[{"definition":"A short-lived thing."}]}],"origin":"From Greek ephēmeros"}]`
	case "geocoding-api.open-meteo.com":
		body = `{"results":[{"name":"Tokyo","latitude":35.68,"longitude":139.69,"country":"Japan","admin1":"Tokyo"}]}`
	case "api.nal.usda.gov":
		body = `{"totalHits":0,"foods":[]}`
	case "world.openfoodfacts.org":
		body = `{"count":1,"products":[{"product_name":"Banana","serving_size":"100 g","nutriments":{"energy-kcal_100g":89,"proteins_100g":1.1,"carbohydrates_100g":22.8,"fat_100g":0.3}}]}`
	case "www.thesportsdb.com":
		switch request.URL.Path {
		case "/api/v1/json/3/searchteams.php":
			body = `{"teams":[{"idTeam":"123","strTeam":"Los Angeles Lakers","strLeague":"NBA","idLeague":"4387"}]}`
		case "/api/v1/json/3/eventslast.php", "/api/v1/json/3/eventsnext.php":
			body = `{"events":[{"idEvent":"456","strEvent":"Los Angeles Lakers vs Boston Celtics","strLeague":"NBA","idLeague":"4387","strHomeTeam":"Los Angeles Lakers","strAwayTeam":"Boston Celtics","intHomeScore":"110","intAwayScore":"105","strStatus":"Match Finished","dateEvent":"2026-10-10"}]}`
		default:
			body = `{"events":[]}`
		}
	case "api.open-meteo.com":
		now := time.Now().UTC().Truncate(time.Hour)
		days := make([]string, 5)
		highs, lows, codes := make([]float64, 5), make([]float64, 5), make([]int, 5)
		for index := range days {
			days[index] = now.AddDate(0, 0, index).Format("2006-01-02")
			highs[index], lows[index], codes[index] = 22, 14, 1
		}
		daily, _ := json.Marshal(map[string]interface{}{"time": days, "temperature_2m_max": highs, "temperature_2m_min": lows, "weather_code": codes})
		hourly, _ := json.Marshal(map[string]interface{}{"time": []string{now.Format("2006-01-02T15:00")}, "relativehumidity_2m": []int{45}, "apparent_temperature": []float64{19}})
		current, _ := json.Marshal(map[string]interface{}{"temperature": 20, "windspeed": 5, "weathercode": 1, "time": now.Format("2006-01-02T15:00")})
		body = fmt.Sprintf(`{"current_weather":%s,"hourly":%s,"daily":%s}`, current, hourly, daily)
	case "raw.githubusercontent.com":
		if strings.Contains(request.URL.Path, "/countries/master/dist/countries.json") {
			body = `[{"name":{"common":"United States","official":"United States of America"},"cca2":"US","cca3":"USA","ccn3":"840","capital":["Washington, D.C."],"region":"Americas","subregion":"North America","area":9833517,"population":331000000,"timezones":["UTC-12:00","UTC-11:00"],"flag":"🇺🇸","tld":[".us"],"idd":{"root":"+1","suffixes":["1"]},"currencies":{"USD":{"name":"United States dollar","symbol":"$"}},"languages":{"eng":"English"}}]`
		} else {
			body = "# curl\n> Transfer data from or to a server.\n\n- Download the contents of a URL:\n`curl {{https://example.com}}`\n"
		}
	case "man.cx":
		body = `{"name":"printf","section":"1","description":"Format and print data","content":"NAME\nprintf - format and print data\n\nSYNOPSIS\nprintf FORMAT [ARGUMENT]..."}`
	case "cheat.sh":
		body = "Show the current directory:\npwd\n"
	case "datatracker.ietf.org":
		body = `{"title":"HTTP Semantics","abstract":"This document defines HTTP semantics.","authors":[{"name":"R. Fielding"}],"time":"2022-06"}`
	case "pypi.org":
		body = `{"info":{"name":"numpy","summary":"Array computing","description":"NumPy package fixture README","version":"2.1.0","license":"BSD-3-Clause","project_urls":{"Repository":"https://github.com/numpy/numpy"},"requires_dist":["pytest"]},"releases":{}}`
	case "pypistats.org":
		body = `{"data":{"last_week":2345678}}`
	case "proxy.golang.org":
		if strings.HasSuffix(request.URL.Path, ".mod") {
			body = "module github.com/go-chi/chi\n\ngo 1.20\n\nrequire github.com/go-chi/chi/v5 v5.0.0\n"
		} else {
			body = `{"Version":"v5.0.0","Time":"2026-10-01T00:00:00Z"}`
		}
	case "crates.io":
		switch request.URL.Path {
		case "/api/v1/crates/serde":
			body = `{"crate":{"id":"serde","name":"serde","description":"Serialization framework","max_version":"1.0.0","license":"MIT","repository":"https://github.com/serde-rs/serde","downloads":123456,"updated_at":"2026-10-01T00:00:00Z"}}`
		case "/api/v1/crates/serde/1.0.0":
			body = `{"version":{"readme_path":""}}`
		case "/api/v1/crates/serde/1.0.0/dependencies":
			body = `{"dependencies":[]}`
		case "/api/v1/crates/serde/downloads":
			body = fmt.Sprintf(`{"version_downloads":[{"date":%q,"downloads":100}],"meta":{"extra_downloads":[]}}`, time.Now().UTC().Format("2006-01-02"))
		default:
			body = `{}`
		}
	case "rubygems.org":
		body = `{"name":"rails","info":"Web framework","version":"7.0.0","version_created_at":"2026-10-01T00:00:00Z","licenses":["MIT"],"source_code_uri":"https://github.com/rails/rails","downloads":1234,"dependencies":{"runtime":[{"name":"rack"}]}}`
	case "registry.npmjs.org":
		body = `{"name":"express","description":"Fast HTTP framework","dist-tags":{"latest":"4.18.2"},"time":{"modified":"2026-10-01T00:00:00.000Z"},"license":"MIT","repository":{"url":"https://github.com/expressjs/express"},"homepage":"https://expressjs.com","readme":"Express fixture README","versions":{"4.18.2":{"dependencies":{"accepts":"~1.3.8"}}}}`
	case "api.npmjs.org":
		body = `{"downloads":123456}`
	case "lingva.ml":
		body = `{"translation":"hola","info":{"detectedSource":"en"}}`
	case "en.wikipedia.org", "de.wikipedia.org":
		if strings.HasPrefix(request.URL.Path, "/api/rest_v1/page/summary/") {
			body = `{"type":"standard","title":"Python","extract":"Python is a high-level programming language.","extract_html":"<p>Python is a high-level programming language.</p>"}`
		} else if request.URL.Path == "/w/api.php" && request.URL.Query().Get("action") == "parse" {
			body = `{"parse":{"sections":[{"line":"History","anchor":"History","level":"2"}],"text":"<div class=\"mw-parser-output\"><table class=\"infobox\"><tr><th>Designed by</th><td>Guido van Rossum</td></tr></table><h2 id=\"History\">History</h2><p>Python began in the late 1980s.</p></div>"}}`
		} else {
			body = `{"query":{"pages":{"1":{"links":[]}}}}`
		}
	case "stat.ripe.net":
		switch request.URL.Path {
		case "/data/as-overview/data.json":
			body = `{"data_call_status":"ok","data":{"resource":"AS15169","holder":"Google LLC","block":{"desc":"Google"}}}`
		case "/data/announced-prefixes/data.json":
			body = `{"data_call_status":"ok","data":{"prefixes":[{"prefix":"8.8.8.0/24"}]}}`
		case "/data/asn-neighbours/data.json":
			body = `{"data_call_status":"ok","data":{"neighbours":[{"asn":6453,"name":"TATA","type":"left"},{"asn":15169,"name":"Google LLC","type":"right"}]}}`
		case "/data/asn-neighbours-history/data.json":
			body = `{"data_call_status":"ok","data":{"neighbours":[{"neighbour":6453,"timelines":[{"starttime":"2025-01-01T00:00:00","endtime":"2026-01-01T00:00:00"}]}]}}`
		case "/data/whois/data.json":
			body = `{"data_call_status":"ok","data":{"authorities":["arin"],"records":[[{"key":"country","value":"US"},{"key":"created","value":"2000-03-30T00:00:00Z"}]]}}`
		default:
			body = `{"data_call_status":"ok","data":{}}`
		}
	case "web.archive.org":
		if strings.Contains(request.URL.Path, "/cdx/search/cdx") {
			body = `[["timestamp","original"],["20210101000000","https://8.8.8.8/"],["20200101000000","https://8.8.8.8/"]]`
		} else if strings.Contains(request.URL.Path, "20200101000000") {
			body = `<html><body>Archived fixture page older capture</body></html>`
		} else {
			body = `<html><body>Archived fixture page newer capture</body></html>`
		}
	case "archive.today":
		if strings.HasPrefix(request.URL.Path, "/timemap/") {
			body = `<https://archive.today/20190102030405/https://8.8.8.8/>; rel="memento"; datetime="Wed, 02 Jan 2019 03:04:05 GMT"`
		} else {
			body = `<html><body>Archive.today fixture page</body></html>`
		}
	case "index.commoncrawl.org":
		if strings.HasSuffix(request.URL.Path, "/collinfo.json") {
			body = `[{"id":"CC-MAIN-2026-40"}]`
		} else {
			body = ""
		}
	case "rdap.org":
		body = `{"handle":"EXAMPLE","ldhName":"example.test","status":["active"],"events":[{"eventAction":"registration","eventDate":"2020-01-01T00:00:00Z"}],"nameservers":[{"ldhName":"ns1.example.test"}]}`
	case "services.nvd.nist.gov":
		body = `{"resultsPerPage":1,"startIndex":0,"totalResults":1,"vulnerabilities":[{"cve":{"id":"CVE-2021-44228","descriptions":[{"lang":"en","value":"Remote code execution vulnerability in Log4j."}],"metrics":{"cvssMetricV31":[{"cvssData":{"baseScore":10.0,"baseSeverity":"CRITICAL","vectorString":"CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:C/C:H/I:H/A:H"}}]},"configurations":[{"nodes":[{"cpeMatch":[{"vulnerable":true,"criteria":"cpe:2.3:a:apache:log4j:*:*:*:*:*:*:*:*","versionEndExcluding":"2.15.0"}]}]}],"published":"2021-12-10T10:15:00.000","lastModified":"2021-12-18T00:00:00.000","references":[{"url":"https://logging.apache.org/log4j/2.x/security.html","tags":["Vendor Advisory","Patch"]},{"url":"https://exploit.example/log4shell","tags":["Exploit"]}]}}]}`
	case "api.coingecko.com":
		if strings.HasSuffix(request.URL.Path, "/market_chart") {
			body = `{"prices":[[1,100],[2,102],[3,99]]}`
		} else {
			body = `{"bitcoin":{"usd":65000,"usd_24h_change":1.5,"usd_market_cap":1280000000000,"usd_24h_vol":25000000000},"ethereum":{"usd":3000,"usd_24h_change":2.0,"usd_market_cap":350000000000,"usd_24h_vol":15000000000}}`
		}
	case "query1.finance.yahoo.com":
		symbol := strings.TrimPrefix(request.URL.Path, "/v8/finance/chart/")
		name := "Apple Inc."
		if symbol == "AMZN" {
			name = "Amazon.com, Inc."
		}
		body = fmt.Sprintf(`{"chart":{"result":[{"meta":{"symbol":%q,"shortName":%q,"regularMarketPrice":225,"chartPreviousClose":223},"indicators":{"quote":[{"open":[223,224],"high":[225,226],"low":[222,223],"close":[225,226,224],"volume":[400000,300000,300000]}]}}],"error":null}}`, symbol, name)
	case "api.frankfurter.dev":
		today := time.Now().UTC().Format("2006-01-02")
		if strings.HasPrefix(request.URL.Path, "/v2/rate/") {
			body = fmt.Sprintf(`{"date":%q,"base":"USD","quote":"EUR","rate":0.92}`, today)
		} else {
			body = fmt.Sprintf(`[{"date":%q,"base":"USD","quote":"EUR","rate":0.92}]`, today)
		}
	default:
		return transport.fallback.RoundTrip(request)
	}
	contentType := "application/json"
	if request.URL.Host == "web.archive.org" {
		contentType = "text/html; charset=utf-8"
	}
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{contentType}}, Body: io.NopCloser(strings.NewReader(body)), Request: request}, nil
}

// localDirectAnswerPages lists direct-answer features with deterministic
// bundled data, local algorithms, or offline provider fixtures.
var localDirectAnswerPages = []struct {
	name     string
	path     string
	typeName string
}{
	{"http status reference", "/direct/http/404", "http"},
	{"tldr command reference", "/direct/tldr/curl", "tldr"},
	{"man page reference", "/direct/man/printf", "man"},
	{"cheat sheet reference", "/direct/cheat/curl", "cheat"},
	{"RFC reference", "/direct/rfc/9110", "rfc"},
	{"country reference", "/direct/country/us", "country"},
	{"npm package metadata", "/direct/pkg/npm%2Fexpress", "pkg"},
	{"PyPI package metadata", "/direct/pkg/pip%2Fnumpy", "pkg"},
	{"Go module metadata", "/direct/pkg/go%2Fgithub.com%2Fgo-chi%2Fchi", "pkg"},
	{"crates.io package metadata", "/direct/pkg/crate%2Fserde", "pkg"},
	{"RubyGems package metadata", "/direct/pkg/gem%2Frails", "pkg"},
	{"port reference", "/direct/port/443", "port"},
	{"cron explainer", "/direct/cron/%2A%2F5%20%2A%20%2A%20%2A%20%2A", "cron"},
	{"permission calculator", "/direct/chmod/755", "chmod"},
	{"timestamp converter", "/direct/timestamp/1704067200", "timestamp"},
	{"subnet calculator", "/direct/subnet/192.168.1.0%2F24", "subnet"},
	{"html entity converter", "/direct/html/encode%20%3Cscript%3E", "html"},
	{"html entity decoder", "/direct/html/decode%20%26lt%3Bscript%26gt%3B", "html"},
	{"unicode reference", "/direct/unicode/a", "unicode"},
	{"emoji lookup", "/direct/emoji/smile", "emoji"},
	{"string escaper", "/direct/escape/json%20hello", "escape"},
	{"SQL string escaper", "/direct/escape/sql%20o%27brien", "escape"},
	{"URL string escaper", "/direct/escape/url%20hello%20world", "escape"},
	{"json formatter", "/direct/json/%7B%22a%22%3A1%7D", "json"},
	{"json minifier", "/direct/json/minify%20%7B%20%22a%22%20%3A%201%20%7D", "json"},
	{"json tree view", "/direct/json/tree%20%7B%22a%22%3A1%7D", "json"},
	{"json validation", "/direct/json/validate%20%7B%22a%22%3A1%7D", "json"},
	{"yaml formatter", "/direct/yaml/key%3A%20value", "yaml"},
	{"YAML to JSON", "/direct/yaml/to-json%20key%3A%20value", "yaml"},
	{"JSON to YAML", "/direct/yaml/from-json%20%7B%22key%22%3A%22value%22%7D", "yaml"},
	{"case converter", "/direct/case/hello%20world", "case"},
	{"slug converter", "/direct/slug/hello%20world", "slug"},
	{"lorem generator", "/direct/lorem/3", "lorem"},
	{"lorem sentence generator", "/direct/lorem/5%20sentences", "lorem"},
	{"word information", "/direct/word/test", "word"},
	{"word statistics", "/direct/word/the%20quick%20brown%20fox", "word"},
	{"user agent parser", "/direct/useragent/firefox%2F120", "useragent"},
	{"mime reference", "/direct/mime/json", "mime"},
	{"license reference", "/direct/license/mit", "license"},
	{"rules reference", "/direct/rules/34", "rules"},
	{"all rules reference", "/direct/rules/all", "rules"},
	{"rule search", "/direct/rules/cat", "rules"},
	{"invalid rule fallback", "/direct/rules/999", "rules"},
	{"ascii generator", "/direct/ascii/hi", "ascii"},
	{"regular expression explainer", "/direct/regex/%5E%5Ba-z%5D%2B%24", "regex"},
	{"regex capture groups", "/direct/regex/%28%5Cd%7B3%7D%29-%28%5Cd%7B4%7D%29", "regex"},
	{"JWT decoder", "/direct/jwt/eyJhbGciOiJub25lIn0.eyJzdWIiOiJ0ZXN0In0.c2ln", "jwt"},
	{"ISO timestamp converter", "/direct/timestamp/2024-01-01T00%3A00%3A00Z", "timestamp"},
	{"text diff", "/direct/diff/old%7C%7C%7Cnew", "diff"},
	{"JSON diff", "/direct/diff/%7B%22a%22%3A1%7D%7C%7C%7B%22a%22%3A2%7D", "diff"},
	{"code formatter", "/direct/beautify/js%20function%20f()%7Breturn%201%7D", "beautify"},
	{"Allman code formatter", "/direct/beautify/js%20--style=allman%20function%20f()%7Breturn%201%7D", "beautify"},
	{"compact CSS formatter", "/direct/beautify/css%20--style=compact%20.a%7Bcolor:red%7D", "beautify"},
	{"lowercase SQL formatter", "/direct/beautify/sql%20--style=lowercase%20SELECT%20ID%20FROM%20Users", "beautify"},
	{"scss minifier", "/direct/beautify/scss%20--minify%20.a%20.b%7Bwidth:calc(100%25%20-%202em)%7D", "beautify"},
	{"less minifier", "/direct/beautify/less%20--minify%20%40color:red;.a%7Bcolor:%40color%7D", "beautify"},
	{"sql minifier", "/direct/beautify/sql%20--minify%20select%20a%20from%20t", "beautify"},
	{"python minifier", "/direct/beautify/python%20--minify%20x%20=%201%20%23%20comment", "beautify"},
	{"dictionary reference", "/direct/dict/ephemeral", "dict"},
	{"thesaurus reference", "/direct/thesaurus/happy", "thesaurus"},
	{"slang definition", "/direct/slang/yeet", "slang"},
	{"Wikipedia reference", "/direct/wiki/python", "wiki"},
	{"localized Wikipedia reference", "/direct/wiki/python?lang=de", "wiki"},
	{"CVE vulnerability details", "/direct/cve/2021-44228", "cve"},
	{"DNS records", "/direct/dns/fixture.example.test", "dns"},
	{"WHOIS domain lookup", "/direct/whois/example.test", "whois"},
	{"hostname resolution", "/direct/resolve/fixture.example.test", "resolve"},
	{"ASN details", "/direct/asn/AS15169", "asn"},
	{"TLS certificate inspection", "/direct/cert/8.8.8.8", "cert"},
	{"HTTP headers analysis", "/direct/headers/8.8.8.8", "headers"},
	{"robots.txt analysis", "/direct/robots/8.8.8.8", "robots"},
	{"sitemap analysis", "/direct/sitemap/8.8.8.8", "sitemap"},
	{"directory discovery", "/direct/directory/fixture", "directory"},
	{"archive cache view", "/direct/cache/8.8.8.8", "cache"},
	{"technology detection", "/direct/tech/8.8.8.8", "tech"},
	{"feed discovery", "/direct/feed/8.8.8.8", "feed"},
	{"URL expansion", "/direct/expand/8.8.8.8/redirect", "expand"},
	{"URL safety analysis", "/direct/safe/8.8.8.8", "safe"},
}

func assertDirectAnswerFixture(t *testing.T, typeName, path, answer string) {
	t.Helper()
	if typeName == "beautify" {
		switch {
		case strings.Contains(path, "style=allman"):
			if !strings.Contains(answer, "Style: allman") || !strings.Contains(answer, "function f()\n{") {
				t.Error("Allman formatter output is missing its style or brace placement")
			}
		case strings.Contains(path, "style=compact"):
			if !strings.Contains(answer, "Style: compact") || strings.Contains(snippet(answer, "<pre><code"), "\n") {
				t.Error("compact formatter output is missing its style or contains line breaks")
			}
		case strings.Contains(path, "style=lowercase"):
			if !strings.Contains(answer, "Style: lowercase") || !strings.Contains(answer, "select ID") {
				t.Error("lowercase SQL formatter did not lowercase keywords while preserving identifiers")
			}
		}
	}
	if typeName == "tldr" && (!strings.Contains(answer, "curl") || !strings.Contains(answer, "Transfer data")) {
		t.Error("tldr answer is missing fixture command or description")
		return
	}
	if typeName == "man" && (!strings.Contains(answer, "printf") || !strings.Contains(answer, "format and print data")) {
		t.Error("man answer is missing fixture page content")
		return
	}
	if typeName == "cheat" && (!strings.Contains(answer, "pwd") || !strings.Contains(answer, "current directory")) {
		t.Error("cheat answer is missing fixture command content")
		return
	}
	if typeName == "rfc" && (!strings.Contains(answer, "9110") || !strings.Contains(answer, "HTTP Semantics")) {
		t.Error("RFC answer is missing fixture document metadata")
		return
	}
	if typeName == "country" && (!strings.Contains(answer, "United States") || !strings.Contains(answer, "Washington, D.C.")) {
		t.Error("country answer is missing fixture country data")
		return
	}
	if typeName == "dict" && (!strings.Contains(answer, "ephemeral") || !strings.Contains(answer, "Lasting for a short time.") || !strings.Contains(answer, "A short-lived thing.")) {
		t.Error("dictionary answer is missing one or more fixture records")
		return
	}
	if typeName == "thesaurus" && (!strings.Contains(answer, "joyful") || !strings.Contains(answer, "bad")) {
		t.Error("thesaurus answer is missing fixture synonyms or antonyms")
		return
	}
	if typeName == "wiki" && !strings.Contains(answer, "Python is a high-level programming language.") {
		t.Error("Wikipedia answer is missing fixture summary")
		return
	}
	if typeName == "cve" {
		for _, marker := range []string{"CVE-2021-44228", "CRITICAL", "Affected products and versions", "Remediation and patches", "exploit reference", "Last modified"} {
			if !strings.Contains(answer, marker) {
				t.Errorf("CVE answer missing %q", marker)
			}
		}
		return
	}
	if typeName == "dns" && (!strings.Contains(answer, "8.8.8.8") || !strings.Contains(answer, "mail.fixture.example.test")) {
		t.Error("DNS answer is missing fixture address or MX record")
	}
	if typeName == "resolve" && (!strings.Contains(answer, "8.8.8.8") || !strings.Contains(answer, "fixture.example.test")) {
		t.Error("resolve answer is missing fixture address or PTR record")
	}
	if typeName == "asn" && (!strings.Contains(answer, "Google LLC") || !strings.Contains(answer, "8.8.8.0/24")) {
		t.Error("ASN answer is missing fixture organization or prefix")
	}
	if typeName == "directory" && (!strings.Contains(answer, "Fixture directory listing") || !strings.Contains(answer, "fixture.txt")) {
		t.Error("directory answer is missing fixture result")
	}
	archiveTodayDownloadVisible := strings.Contains(answer, "/direct/cache-download-archive-today?") ||
		(strings.Contains(answer, "Archive.today: View") && strings.Contains(answer, "Download archived response"))
	if typeName == "cache" && (!strings.Contains(answer, "Wayback Machine") || !strings.Contains(answer, "Archived fixture page") || !archiveTodayDownloadVisible) {
		t.Errorf("archive answer is missing fixture snapshot, archived content, or Archive.today download")
	}
	if typeName == "whois" && (!strings.Contains(answer, "example.test") || !strings.Contains(answer, "ns1.example.test")) {
		t.Error("WHOIS answer is missing fixture domain or nameserver")
	}
	if typeName == "headers" && (!strings.Contains(answer, "fixture-server") || !strings.Contains(answer, "Security Grade")) {
		t.Error("headers answer is missing fixture response metadata")
	}
	if typeName == "robots" && (!strings.Contains(answer, "/private") || !strings.Contains(answer, "Sitemap")) {
		t.Errorf("robots answer is missing fixture directives: %s", snippet(answer, `class="direct-content`))
	}
	if typeName == "tech" && (!strings.Contains(answer, "fixture-server") || !strings.Contains(answer, "React")) {
		t.Errorf("technology answer is missing fixture stack details: %s", snippet(answer, `class="direct-content`))
	}
	if typeName == "feed" && (!strings.Contains(answer, "Fixture feed") || !strings.Contains(answer, "Fixture feed description") || !strings.Contains(answer, "Latest fixture story") || !strings.Contains(answer, "Entries: 1") || !strings.Contains(answer, "Feedly")) {
		t.Errorf("feed answer is missing fixture feed metadata: %s", snippet(answer, `class="direct-content`))
	}
	if typeName == "slang" && (!strings.Contains(answer, "yeet") || !strings.Contains(answer, "To throw something with force.") || !strings.Contains(answer, "1200")) {
		t.Error("slang answer is missing fixture term, definition, or popularity")
		return
	}
	if typeName == "rules" {
		switch {
		case strings.HasSuffix(path, "/all"), strings.HasSuffix(path, "/999"):
			if !strings.Contains(answer, "Rule 100") {
				t.Error("rules answer is missing the final bundled rule")
			}
		case strings.HasSuffix(path, "/cat"):
			if !strings.Contains(answer, "cat") {
				t.Error("rules search answer is missing its matching rule")
			}
		}
		return
	}
	if typeName != "pkg" {
		return
	}
	name, version := "express", "4.18.2"
	switch {
	case strings.Contains(path, "pip%2F"):
		name, version = "numpy", "2.1.0"
	case strings.Contains(path, "github.com%2Fgo-chi"):
		name, version = "github.com/go-chi/chi", "v5.0.0"
	case strings.Contains(path, "crate%2F"):
		name, version = "serde", "1.0.0"
	case strings.Contains(path, "gem%2F"):
		name, version = "rails", "7.0.0"
	}
	if !strings.Contains(answer, name) || !strings.Contains(answer, version) {
		t.Errorf("package answer is missing fixture name %q or version %q", name, version)
	}
}

func assertInstantAnswerFixture(t *testing.T, typeName, html string) {
	t.Helper()
	switch typeName {
	case "timezone":
		if !strings.Contains(html, `class="world-clock"`) || strings.Count(html, `<li><strong>`) != 6 {
			t.Error("timezone answer is missing its six-city world clock")
		}
	case "date":
		if !strings.Contains(html, `class="calendar-preview"`) || !strings.Contains(html, `aria-current="date"`) {
			t.Error("date answer is missing its accessible month preview or selected day")
		}
	case "weather":
		if got := strings.Count(html, `class="weather-day-range"`); got != 5 {
			t.Errorf("weather answer has %d forecast days, want 5", got)
		}
	case "translate":
		if !strings.Contains(html, "hola") || (!strings.Contains(html, "hello") && !strings.Contains(html, "good morning")) {
			t.Error("translation answer is missing fixture text")
		}
	case "ip":
		if !strings.Contains(html, "8.8.8.8") {
			t.Error("IP answer is missing the queried address")
		}
	case "tracking":
		if !strings.Contains(html, "UPS") || !strings.Contains(html, "1Z999AA10123456784") {
			t.Error("tracking answer is missing carrier detection or tracking number")
		}
	case "sports":
		if !strings.Contains(html, "Los Angeles Lakers") || !strings.Contains(html, "Boston Celtics") || !strings.Contains(html, "110") {
			t.Error("sports answer is missing fixture teams or score")
		}
	case "nutrition":
		if !strings.Contains(html, "Banana") || !strings.Contains(html, "89") {
			t.Error("nutrition answer is missing fixture food or calories")
		}
	case "wikipedia":
		if !strings.Contains(html, "Python is a high-level programming language.") {
			t.Error("Wikipedia answer is missing fixture summary")
		}
	case "crypto":
		if !strings.Contains(html, `class="crypto-price-chart"`) {
			t.Error("crypto answer is missing its 24-hour price chart")
		}
		if !strings.Contains(html, "65,000") && !strings.Contains(html, "65000") && !strings.Contains(html, "1500.00") {
			t.Error("crypto answer is missing fixture price")
		}
		if strings.Contains(html, "Ethereum") && !strings.Contains(html, "0.5 ETH: $1500.00") {
			t.Error("crypto amount conversion is missing the converted amount")
		}
	case "stocks":
		if !strings.Contains(html, `class="stock-price-chart"`) {
			t.Error("stock answer is missing its intraday price chart")
		}
		if !((strings.Contains(html, "AAPL") && strings.Contains(html, "Apple Inc.")) || (strings.Contains(html, "AMZN") && strings.Contains(html, "Amazon.com, Inc."))) {
			t.Error("stock answer is missing fixture quote")
		}
	case "synonym":
		if !strings.Contains(html, "joyful") || !strings.Contains(html, "cheerful") {
			t.Error("synonym answer is missing fixture words")
		}
	case "antonym":
		if !strings.Contains(html, "bad") || !strings.Contains(html, "poor") {
			t.Error("antonym answer is missing fixture words")
		}
	case "definition":
		for _, marker := range []string{`class="phonetic"`, `audio controls`, `Lasting for a short time.`, "A short-lived thing."} {
			if !strings.Contains(html, marker) {
				t.Errorf("dictionary answer missing %q", marker)
			}
		}
	case "currency":
		for _, marker := range []string{`class="currency-conversion"`, `class="currency-history-chart"`} {
			if !strings.Contains(html, marker) {
				t.Errorf("currency answer missing %q", marker)
			}
		}
	}
}

// trackingMarker is the site id baked into the tracked test site. Its presence
// in a rendered page is what proves the analytics snippet was injected.
const trackingMarker = "e2e-tracked.example.com"

// newTestSite boots the application once per run and returns a live
// httptest.Server in front of the real handler.
//
// The underlying server.Server is expensive to construct (logging managers,
// database, widget/bang/direct managers, engine registry), so it is built once
// and shared. Each caller still gets its own cookie jar via newSession, so
// theme and CSRF state never leaks between tests.
func newTestSite(t *testing.T) *httptest.Server {
	t.Helper()

	sharedOnce.Do(func() {
		cfg := config.DefaultConfig()
		cfg.Server.Security.EncryptionKey = "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY="

		// Known operator token for the RequireOperator-gated routes.
		cfg.Server.Token = e2eOperatorToken

		// The suite crawls roughly 40 pages per tier in a tight loop. The
		// production default of 120 read requests per 60s would rate-limit the
		// crawl itself rather than the behaviour under test; rate limiting has
		// its own coverage in src/server.
		cfg.Server.RateLimit.Enabled = false
		cfg.Server.GeoIP.Enabled = false
		// The suite serves the application over httptest's HTTP listener, so
		// CSP must not upgrade same-origin browser form/fetch requests to HTTPS.
		cfg.Server.Security.Headers.ContentSecurityPolicy = strings.ReplaceAll(
			cfg.Server.Security.Headers.ContentSecurityPolicy,
			"upgrade-insecure-requests;", "",
		)
		cfg.Search.Widgets.Enabled = true
		cfg.Search.Widgets.Sports.Enabled = true
		for name, engine := range cfg.Engines {
			engine.Enabled = false
			cfg.Engines[name] = engine
		}

		e2eOriginalTransport = http.DefaultTransport
		http.DefaultTransport = providerFixtureTransport{fallback: e2eOriginalTransport}
		e2eHTTPSFixture = httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			writer.Header().Set("Server", "fixture-server")
			writer.Header().Set("X-Frame-Options", "DENY")
			switch request.URL.Path {
			case "/feed.xml":
				writer.Header().Set("Content-Type", "application/rss+xml")
				_, _ = io.WriteString(writer, `<rss version="2.0"><channel><title>Fixture feed</title><description>Fixture feed description</description><lastBuildDate>Thu, 01 Jan 2026 00:00:00 GMT</lastBuildDate><item><title>Latest fixture story</title><link>https://example.test/story</link><description>Fixture story preview</description><pubDate>Thu, 01 Jan 2026 00:00:00 GMT</pubDate></item></channel></rss>`)
			case "/robots.txt":
				_, _ = io.WriteString(writer, "User-agent: *\nDisallow: /private\nSitemap: http://8.8.8.8/sitemap.xml\n")
			case "/sitemap.xml":
				writer.Header().Set("Content-Type", "application/xml")
				_, _ = io.WriteString(writer, "<urlset><url><loc>http://8.8.8.8/</loc></url></urlset>")
			case "/redirect":
				http.Redirect(writer, request, "https://8.8.8.8/final", http.StatusFound)
			case "/":
				writer.Header().Set("Content-Type", "text/html")
				_, _ = io.WriteString(writer, `<html><head><link rel="alternate" type="application/rss+xml" href="/feed.xml" title="Fixture feed"></head><body>fixture React example.com<a href="/fixture.txt">fixture.txt</a></body></html>`)
			default:
				writer.WriteHeader(http.StatusNotFound)
			}
		}))
		fixtureAddress := e2eHTTPSFixture.Listener.Addr().String()
		dialFixture := func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, fixtureAddress)
		}
		fixtureLookup := func(context.Context, string) ([]net.IPAddr, error) {
			return []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}}, nil
		}
		dnsPacketConn, err := net.ListenPacket("udp", "127.0.0.1:0")
		if err != nil {
			panic(fmt.Errorf("start fixture DNS server: %w", err))
		}
		dnsHandler := dns.HandlerFunc(func(writer dns.ResponseWriter, request *dns.Msg) {
			response := new(dns.Msg)
			response.SetReply(request)
			for _, question := range request.Question {
				name := strings.ToLower(strings.TrimSuffix(question.Name, "."))
				switch question.Qtype {
				case dns.TypeA:
					if name == "fixture.example.test" {
						record, _ := dns.NewRR(question.Name + " 60 IN A 8.8.8.8")
						response.Answer = append(response.Answer, record)
					}
				case dns.TypeAAAA:
				case dns.TypeMX:
					record, _ := dns.NewRR(question.Name + " 60 IN MX 10 mail.fixture.example.test.")
					response.Answer = append(response.Answer, record)
				case dns.TypeNS:
					record, _ := dns.NewRR(question.Name + " 60 IN NS ns1.fixture.example.test.")
					response.Answer = append(response.Answer, record)
				case dns.TypeTXT:
					record, _ := dns.NewRR(question.Name + ` 60 IN TXT "fixture dns record"`)
					response.Answer = append(response.Answer, record)
				case dns.TypePTR:
					record, _ := dns.NewRR(question.Name + " 60 IN PTR fixture.example.test.")
					response.Answer = append(response.Answer, record)
				}
			}
			_ = writer.WriteMsg(response)
		})
		e2eDNSServer = &dns.Server{PacketConn: dnsPacketConn, Handler: dnsHandler}
		go func() { _ = e2eDNSServer.ActivateAndServe() }()
		dnsAddress := dnsPacketConn.LocalAddr().String()
		directorySearcher := func(context.Context, string) ([]direct.DirectoryResult, error) {
			return []direct.DirectoryResult{{Title: "Fixture directory listing", URL: "https://8.8.8.8/", Description: "Deterministic fixture file", EstimatedFileCount: 1}}, nil
		}
		direct.SetE2ENetworkHooks(dialFixture, dnsAddress, fixtureLookup, directorySearcher)
		sharedTitle = cfg.Server.Title
		sharedDescr = cfg.Server.Description

		sharedHTTP = httptest.NewServer(server.NewServerWithPublicIPProviders(cfg, []string{}).Handler())
	})

	return sharedHTTP
}

// newTrackedSite boots a second application instance with an analytics
// provider configured, so the consent gate can be exercised end to end: the
// snippet must be absent before consent and present after it.
//
// It is a separate server from newTestSite because server.tracking is process
// configuration — toggling it on the shared site would leak tracking into every
// other test and make the "no consent, no tracker" assertion meaningless.
func newTrackedSite(t *testing.T) *httptest.Server {
	t.Helper()

	trackedOnce.Do(func() {
		cfg := config.DefaultConfig()
		cfg.Server.Token = e2eOperatorToken
		cfg.Server.RateLimit.Enabled = false
		cfg.Server.GeoIP.Enabled = false
		cfg.Search.Widgets.Enabled = false
		for name, engine := range cfg.Engines {
			engine.Enabled = false
			cfg.Engines[name] = engine
		}
		cfg.Server.Tracking = config.TrackingConfig{
			Type: "plausible",
			ID:   trackingMarker,
		}
		trackedHTTP = httptest.NewServer(server.NewServerWithPublicIPProviders(cfg, []string{}).Handler())
	})

	return trackedHTTP
}

// shutdownTrackedSite closes the tracked listener. Called from TestMain.
func shutdownTrackedSite() {
	if trackedHTTP != nil {
		trackedHTTP.Close()
		trackedHTTP = nil
	}
}

// shutdownTestSite closes the shared listener once the whole package is done.// Called from TestMain in ssr_test.go.
func shutdownTestSite() {
	if sharedHTTP != nil {
		sharedHTTP.Close()
		sharedHTTP = nil
	}
	if e2eOriginalTransport != nil {
		http.DefaultTransport = e2eOriginalTransport
		e2eOriginalTransport = nil
	}
	if e2eHTTPSFixture != nil {
		e2eHTTPSFixture.Close()
		e2eHTTPSFixture = nil
	}
	if e2eDNSServer != nil {
		_ = e2eDNSServer.Shutdown()
		e2eDNSServer = nil
	}
}

// session is a browser-like client bound to one test: a fresh cookie jar, a
// real browser User-Agent, and the base URL of the test site.
type session struct {
	t        *testing.T
	client   *http.Client
	baseURL  string
	title    string
	descr    string
	forcedUA string
}

// browserUserAgent is a current desktop Chrome string. It matters: the server
// content-negotiates on the User-Agent (PART 14), and Go's default client
// string is detected as an HTTP tool, which would return plain text instead of
// the HTML a browser sees.
const browserUserAgent = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 " +
	"(KHTML, like Gecko) Chrome/140.0.0.0 Safari/537.36"

// textBrowserUserAgent is lynx. The server routes it to the dedicated no-JS
// renderers (src/server/nojs.go) rather than the full templates, which is a
// different code path from "browser with scripting disabled" and is covered
// separately.
const textBrowserUserAgent = "Lynx/2.9.0 libwww-FM/2.14"

// httpToolUserAgent is a plain HTTP client; the server renders the page as
// text via the HTML2Text converter.
const httpToolUserAgent = "curl/8.5.0"

// newSession returns a session with an empty cookie jar.
func newSession(t *testing.T) *session {
	t.Helper()

	site := newTestSite(t)

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookiejar: %v", err)
	}

	return &session{
		t:        t,
		client:   &http.Client{Jar: jar, Timeout: 60 * time.Second},
		baseURL:  site.URL,
		title:    sharedTitle,
		descr:    sharedDescr,
		forcedUA: browserUserAgent,
	}
}

// newTrackedSession is newSession bound to the analytics-configured site, with
// its own cookie jar so a consent decision here never leaks into another test.
func newTrackedSession(t *testing.T) *session {
	t.Helper()

	site := newTrackedSite(t)

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookiejar: %v", err)
	}

	return &session{
		t:        t,
		client:   &http.Client{Jar: jar, Timeout: 60 * time.Second},
		baseURL:  site.URL,
		title:    sharedTitle,
		descr:    sharedDescr,
		forcedUA: browserUserAgent,
	}
}

// withoutRedirects returns a copy of the session whose client stops at the
// 3xx. Redirect assertions need this: with a following client a 303 comes
// back as the 200 of the redirect target, so a missing or hijacked redirect
// would pass unnoticed. Cookies set on the 3xx are still recorded by the jar.
func (s *session) withoutRedirects() *session {
	c := *s
	c.client = &http.Client{
		Jar:           s.client.Jar,
		Timeout:       s.client.Timeout,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	return &c
}

// withUserAgent returns a copy of the session sending the given User-Agent.
func (s *session) withUserAgent(ua string) *session {
	c := *s
	c.forcedUA = ua
	return &c
}

// do issues a request with the session's browser headers and returns the
// response with its body already drained and closed.
func (s *session) do(method, path string, mutate ...func(*http.Request)) (*http.Response, string) {
	s.t.Helper()

	req, err := http.NewRequest(method, s.baseURL+path, nil)
	if err != nil {
		s.t.Fatalf("new request %s %s: %v", method, path, err)
	}
	req.Header.Set("User-Agent", s.forcedUA)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")

	for _, m := range mutate {
		m(req)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		s.t.Fatalf("request %s %s: %v", method, path, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		s.t.Fatalf("read body %s %s: %v", method, path, err)
	}

	return resp, string(body)
}

// get performs a browser-like GET and returns the response and body.
func (s *session) get(path string, mutate ...func(*http.Request)) (*http.Response, string) {
	return s.do(http.MethodGet, path, mutate...)
}

// postForm submits url.Values as a native HTML form POST, exactly as a browser
// without JavaScript would. The client does not follow redirects so the caller
// can assert on the 303 and its Location, which is what proves the no-JS
// redirect path works.
func (s *session) postForm(path string, form url.Values) (*http.Response, string) {
	s.t.Helper()

	req, err := http.NewRequest(http.MethodPost, s.baseURL+path, strings.NewReader(form.Encode()))
	if err != nil {
		s.t.Fatalf("new form request: %v", err)
	}
	req.Header.Set("User-Agent", s.forcedUA)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	req.Header.Set("Sec-Fetch-Dest", "document")

	resp, err := s.client.Do(req)
	if err != nil {
		s.t.Fatalf("post %s: %v", path, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		s.t.Fatalf("read body %s: %v", path, err)
	}

	return resp, string(body)
}

// asOperator attaches the operator bearer token to a request.
func asOperator(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+e2eOperatorToken)
}

// csrfTokenFor loads path, extracts the csrf_token hidden field, and returns it
// along with the response cookie of the same name. The CSRF middleware uses a
// stateless double-submit pattern (PART 11), so a raw client must echo both.
func (s *session) csrfTokenFor(path string) (field string, cookie *http.Cookie) {
	s.t.Helper()

	resp, body := s.get(path)

	const marker = `name="csrf_token" value="`
	idx := strings.Index(body, marker)
	if idx < 0 {
		s.t.Fatalf("no csrf_token hidden field found in %s", path)
	}
	rest := body[idx+len(marker):]
	end := strings.Index(rest, `"`)
	if end < 0 {
		s.t.Fatalf("unterminated csrf_token value in %s", path)
	}

	for _, c := range resp.Cookies() {
		if c.Name == "csrf_token" {
			return rest[:end], c
		}
	}

	// The CSRF middleware only issues the cookie when the request does not
	// already carry a valid one, so on a repeat request the token lives in the
	// session jar rather than in this response. A browser replays it from
	// storage, so the harness has to as well.
	u, err := url.Parse(s.baseURL + path)
	if err != nil {
		s.t.Fatalf("parse %s: %v", path, err)
	}
	for _, c := range s.client.Jar.Cookies(u) {
		if c.Name == "csrf_token" {
			return rest[:end], c
		}
	}

	s.t.Fatalf("no csrf_token cookie available for %s", path)
	return "", nil
}

// requireStatus fails the test unless the response carries want.
func requireStatus(t *testing.T, resp *http.Response, want int) {
	t.Helper()
	if resp.StatusCode != want {
		t.Errorf("GET %s status = %d, want %d", resp.Request.URL.Path, resp.StatusCode, want)
	}
}

// requireRedirect asserts a 3xx (303 See Other, which replays as GET) to the
// exact location, and returns the response so the caller can inspect cookies.
func requireRedirect(t *testing.T, resp *http.Response, wantStatus int, wantLocation string) *http.Response {
	t.Helper()
	ctx := resp.Request.URL.Path
	if resp.StatusCode != wantStatus {
		t.Errorf("POST %s status = %d, want %d", ctx, resp.StatusCode, wantStatus)
	}
	if loc := resp.Header.Get("Location"); loc != wantLocation {
		t.Errorf("POST %s Location = %q, want %q", ctx, loc, wantLocation)
	}
	return resp
}

// requireCookie returns the named Set-Cookie, failing the test when absent.
func requireCookie(t *testing.T, resp *http.Response, name string) *http.Cookie {
	t.Helper()
	for _, c := range resp.Cookies() {
		if c.Name == name {
			return c
		}
	}
	t.Errorf("no %q cookie set by %s", name, resp.Request.URL.Path)
	return &http.Cookie{Name: name}
}

// requireContains fails the test with a readable message when body lacks want.
func requireContains(t *testing.T, body, want, context string) {
	t.Helper()
	if !strings.Contains(body, want) {
		t.Errorf("%s: missing %q", context, want)
	}
}

// requireNotContains fails the test when body wrongly contains unwanted.
func requireNotContains(t *testing.T, body, unwanted, context string) {
	t.Helper()
	if strings.Contains(body, unwanted) {
		t.Errorf("%s: unexpectedly contains %q", context, unwanted)
	}
}

// requireHTMLShell asserts the standard document scaffold emitted by
// template/layout/public.tmpl: doctype, themed <html>, the skip link, the
// <main> landmark, and a <title>. Every page tier asserts this, so a template
// that fails to render degrades the suite loudly instead of silently passing.
func requireHTMLShell(t *testing.T, body, path string) {
	t.Helper()
	ctx := "page " + path
	requireContains(t, body, "<!DOCTYPE html>", ctx)
	requireContains(t, body, "<title>", ctx)
	requireContains(t, body, `class="theme-`, ctx)
	requireContains(t, body, `href="#main-content"`, ctx)
	requireContains(t, body, `id="main-content"`, ctx)
}
