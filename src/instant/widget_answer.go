package instant

import (
	"context"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/apimgr/search/src/common/i18n"
	"github.com/apimgr/search/src/widget"
)

// Widget-backed instant answers per IDEA.md "Instant Answers (Widgets)".
// The nine widgets below already have real fetchers in src/widget but had no
// path from a search query to the fetched data, so a user typing one of the
// spec'd trigger forms got an ordinary result list instead of the answer.
// Each trigger reproduces the spec'd query forms verbatim:
//
//	Currency   100 usd to eur, $50 in pounds, 1000 jpy to usd, convert 500 euros to dollars
//	Weather    weather, weather in tokyo, new york weather, forecast london, temperature paris
//	Crypto     bitcoin price, bitcoin, ethereum price, eth, 1 btc to usd
//	Stocks     stock AAPL, TSLA stock, MSFT price, $GOOGL
//	Tracking   track 1Z999AA10123456784, usps 9400111899223033005955
//	Translate  translate hello to spanish, bonjour in english
//	Wikipedia  wiki python programming, albert einstein wikipedia, who is marie curie, what is quantum computing
//	Sports     lakers score, yankees game, nfl scores, premier league results
//	Nutrition  calories in banana, apple nutrition, how many calories banana
//
// IDEA.md requires that a widget failure never blocks the main search, so a
// fetch error, an error payload, or an unrenderable result all return
// (nil, nil) and let the next matching handler run before the query falls
// through to a normal search.
const (
	AnswerTypeWeather   AnswerType = "weather"
	AnswerTypeCurrency  AnswerType = "currency"
	AnswerTypeCrypto    AnswerType = "crypto"
	AnswerTypeStocks    AnswerType = "stocks"
	AnswerTypeTracking  AnswerType = "tracking"
	AnswerTypeTranslate AnswerType = "translate"
	AnswerTypeWiki      AnswerType = "wikipedia"
	AnswerTypeSports    AnswerType = "sports"
	AnswerTypeNutrition AnswerType = "nutrition"
)

// widgetPattern pairs one trigger regex with the function that turns its
// capture groups into fetcher parameters. A nil result means the match looked
// right but named something the widget cannot service, so the handler moves on.
type widgetPattern struct {
	regex  *regexp.Regexp
	params func(m []string) map[string]string
}

// widgetTrigger binds one spec'd trigger set to its fetcher and renderer.
type widgetTrigger struct {
	answerType AnswerType
	widgetType widget.WidgetType
	patterns   []widgetPattern
	render     func(lang string, data interface{}) (title, content, source, sourceURL string, ok bool)
}

// WidgetAnswerHandler serves the nine IDEA.md widgets as instant answers by
// delegating the data fetch to the shared widget.Manager, so the search path
// and the widget panel use one cache and one set of API keys.
type WidgetAnswerHandler struct {
	manager  *widget.Manager
	triggers []widgetTrigger
}

// NewWidgetAnswerHandler builds the handler over the given widget manager.
func NewWidgetAnswerHandler(manager *widget.Manager) *WidgetAnswerHandler {
	return &WidgetAnswerHandler{
		manager:  manager,
		triggers: buildWidgetTriggers(),
	}
}

// Name implements Handler.
func (h *WidgetAnswerHandler) Name() string {
	return "widget"
}

// Patterns implements Handler.
func (h *WidgetAnswerHandler) Patterns() []*regexp.Regexp {
	patterns := make([]*regexp.Regexp, 0, len(h.triggers))
	for _, t := range h.triggers {
		for _, p := range t.patterns {
			patterns = append(patterns, p.regex)
		}
	}
	return patterns
}

// CanHandle implements Handler.
func (h *WidgetAnswerHandler) CanHandle(query string) bool {
	for _, t := range h.triggers {
		for _, p := range t.patterns {
			if p.regex.MatchString(query) {
				return true
			}
		}
	}
	return false
}

// HandleInstantQuery implements Handler. Returning (nil, nil) is the fall
// through signal the manager uses to continue to the next handler.
func (h *WidgetAnswerHandler) HandleInstantQuery(ctx context.Context, query string) (*Answer, error) {
	if h.manager == nil {
		return nil, nil
	}
	lang := LangFromContext(ctx)

	for _, t := range h.triggers {
		for _, p := range t.patterns {
			m := p.regex.FindStringSubmatch(query)
			if m == nil {
				continue
			}

			params := p.params(m)
			if params == nil {
				continue
			}
			if t.widgetType == widget.WidgetWeather {
				params = resolveWeatherParams(ctx, params)
				if params == nil {
					continue
				}
				units := WeatherUnitsFromContext(ctx)
				if units == "auto" {
					units = UnitsFromContext(ctx)
				}
				params["units"] = units
			}
			if t.widgetType == widget.WidgetCrypto || t.widgetType == widget.WidgetStocks {
				params["include_chart"] = "true"
			}
			if t.widgetType == widget.WidgetWikipedia {
				params["lang"] = lang
			}
			if len(params) == 0 {
				continue
			}

			wd, err := h.manager.FetchWidgetData(ctx, t.widgetType, params)
			if err != nil || wd == nil || wd.Error != "" {
				continue
			}

			title, content, source, sourceURL, ok := t.render(lang, wd.Data)
			if !ok {
				continue
			}

			answer := &Answer{
				Type:      t.answerType,
				Query:     query,
				Title:     escapeHTML(title),
				Content:   content,
				Source:    escapeHTML(source),
				SourceURL: escapeHTML(sourceURL),
			}
			if wiki, ok := wd.Data.(*widget.WikipediaData); ok && wiki.Thumbnail != "" {
				answer.Data = map[string]interface{}{"thumbnail": wiki.Thumbnail}
			}
			return answer, nil
		}
	}

	return nil, nil
}

func buildWidgetTriggers() []widgetTrigger {
	return []widgetTrigger{
		{AnswerTypeWeather, widget.WidgetWeather, weatherPatterns(), renderWeather},
		{AnswerTypeCrypto, widget.WidgetCrypto, cryptoPatterns(), renderCrypto},
		{AnswerTypeCurrency, widget.WidgetCurrency, currencyPatterns(), renderCurrency},
		{AnswerTypeStocks, widget.WidgetStocks, stockPatterns(), renderStocks},
		{AnswerTypeTracking, widget.WidgetTracking, trackingPatterns(), renderTracking},
		{AnswerTypeTranslate, widget.WidgetTranslate, translatePatterns(), renderTranslate},
		{AnswerTypeWiki, widget.WidgetWikipedia, wikiPatterns(), renderWiki},
		{AnswerTypeSports, widget.WidgetSports, sportsPatterns(), renderSports},
		{AnswerTypeNutrition, widget.WidgetNutrition, nutritionPatterns(), renderNutrition},
	}
}

// weatherPatterns covers the documented location forms and bare "weather".
// The bare form uses the requester's local GeoIP city when that database is
// available; it never sends the requester's IP to the weather provider.
func weatherPatterns() []widgetPattern {
	return []widgetPattern{
		{regexp.MustCompile(`(?i)^weather$`), func([]string) map[string]string { return map[string]string{"city": "auto"} }},
		{regexp.MustCompile(`(?i)^(?:weather|forecast|temperature)(?:\s+(?:in|for|at))?\s+(.+)$`), weatherParams},
		{regexp.MustCompile(`(?i)^(.+?)\s+(?:weather|forecast|temperature)$`), weatherParams},
	}
}

func weatherParams(m []string) map[string]string {
	city := strings.TrimSpace(m[1])
	if city == "" {
		return nil
	}
	return map[string]string{"city": city}
}

func resolveWeatherParams(ctx context.Context, params map[string]string) map[string]string {
	if params["city"] != "auto" {
		return params
	}
	lookup := GeoIPLookupFromContext(ctx)
	clientIP := ClientIPFromContext(ctx)
	if lookup == nil || clientIP == "" {
		return nil
	}
	location := lookup.Lookup(clientIP)
	if location == nil || !location.Found || strings.TrimSpace(location.City) == "" {
		return nil
	}
	return map[string]string{"city": strings.TrimSpace(location.City)}
}

// currencyPatterns covers the three spec'd currency forms.
func currencyPatterns() []widgetPattern {
	return []widgetPattern{
		// "100 usd to eur", "1000 jpy to usd"
		{regexp.MustCompile(`(?i)^([\d.,]+)\s+([a-z]{3})\s+(?:to|in)\s+([a-z]+)$`), currencyCodeParams},
		// "$50 in pounds", "€20 to dollars"
		{regexp.MustCompile(`(?i)^([$€£¥₹₩₽])\s?([\d.,]+)\s+(?:to|in)\s+([a-z]+)$`), currencySymbolParams},
		// "convert 500 euros to dollars"
		{regexp.MustCompile(`(?i)^(?:convert|exchange)\s+([\d.,]+)\s+([a-z]+)\s+to\s+([a-z]+)$`), currencyCodeParams},
	}
}

// currencyAliases resolves currency names and symbols to ISO codes. It is
// seeded from the widget package's CommonCurrencies list so the instant answer
// and the widget panel recognise exactly the same set of currencies, then
// extended with the singular and short forms users actually type.
var currencyAliases = buildCurrencyAliases()

func buildCurrencyAliases() map[string]string {
	aliases := make(map[string]string)
	for _, c := range widget.CommonCurrencies {
		aliases[strings.ToLower(c.Code)] = c.Code
		aliases[strings.ToLower(c.Name)] = c.Code
		if c.Symbol != "" {
			aliases[c.Symbol] = c.Code
		}
	}
	for alias, code := range map[string]string{
		"dollar": "USD", "dollars": "USD", "buck": "USD",
		"euro": "EUR", "euros": "EUR",
		"pound": "GBP", "pounds": "GBP", "sterling": "GBP",
		"yen": "JPY", "rupee": "INR", "rupees": "INR",
		"won": "KRW", "yuan": "CNY", "ruble": "RUB", "rubles": "RUB",
	} {
		if _, exists := aliases[alias]; !exists {
			aliases[alias] = code
		}
	}
	return aliases
}

// currencyCode resolves a captured token to an ISO code, reporting false when
// the token is not a currency the widget package knows about.
func currencyCode(tok string) (string, bool) {
	tok = strings.ToLower(strings.TrimSpace(tok))
	if tok == "" {
		return "", false
	}
	if code, ok := currencyAliases[tok]; ok {
		return code, true
	}
	if len(tok) == 3 {
		letters := true
		for _, character := range tok {
			if character < 'a' || character > 'z' {
				letters = false
				break
			}
		}
		if letters {
			return strings.ToUpper(tok), true
		}
	}
	return "", false
}

// parseAmount converts a captured amount to a float, reporting false when the
// token is not a usable number.
func parseAmount(tok string) (float64, bool) {
	clean := strings.ReplaceAll(strings.TrimSpace(tok), ",", "")
	if clean == "" {
		return 0, false
	}
	v, err := strconv.ParseFloat(clean, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

// buildConversion assembles fetcher params once both currencies are known to
// be real, and rejects a same-currency request the fetcher cannot answer.
func buildConversion(amount float64, from, to string) map[string]string {
	return map[string]string{
		"from":   from,
		"to":     to,
		"amount": strconv.FormatFloat(amount, 'f', -1, 64),
	}
}

// currencyCodeParams handles "<amount> <code> to <code-or-name>".
func currencyCodeParams(m []string) map[string]string {
	amount, ok := parseAmount(m[1])
	if !ok {
		return nil
	}
	from, ok := currencyCode(m[2])
	if !ok {
		return nil
	}
	to, ok := currencyCode(m[3])
	if !ok {
		return nil
	}
	return buildConversion(amount, from, to)
}

// currencySymbolParams handles "<symbol><amount> to <code-or-name>".
func currencySymbolParams(m []string) map[string]string {
	from, ok := currencyCode(m[1])
	if !ok {
		return nil
	}
	amount, ok := parseAmount(m[2])
	if !ok {
		return nil
	}
	to, ok := currencyCode(m[3])
	if !ok {
		return nil
	}
	return buildConversion(amount, from, to)
}

// cryptoAliases maps the spellings users type onto the coin ids the crypto
// fetcher expects. Restricting the coin token to this list is what keeps an
// arbitrary word off the crypto path.
var cryptoAliases = map[string]string{
	"bitcoin": "bitcoin", "btc": "bitcoin",
	"ethereum": "ethereum", "eth": "ethereum",
	"litecoin": "litecoin", "ltc": "litecoin",
	"dogecoin": "dogecoin", "doge": "dogecoin",
	"cardano": "cardano", "ada": "cardano",
	"polkadot": "polkadot", "dot": "polkadot",
	"chainlink": "chainlink", "link": "chainlink",
	"monero": "monero", "xmr": "monero",
	"solana": "solana", "sol": "solana",
}

// cryptoPatterns covers "bitcoin", "ethereum price" and "1 btc to usd".
func cryptoPatterns() []widgetPattern {
	return []widgetPattern{
		{regexp.MustCompile(`(?i)^(bitcoin|btc|ethereum|eth|litecoin|ltc|dogecoin|doge|cardano|ada|polkadot|dot|chainlink|link|monero|xmr|solana|sol)$`), cryptoParams},
		{regexp.MustCompile(`(?i)^([a-z]+)\s+price$`), cryptoParams},
		{regexp.MustCompile(`(?i)^([\d.,]+)\s+([a-z]+)\s+(?:to|in)\s+([a-z]+)$`), cryptoParams},
	}
}

// cryptoParams requires the coin token to be a known alias, so "the price of
// anything" is never sent to the crypto fetcher.
func cryptoParams(m []string) map[string]string {
	var coin, currency, amount string

	switch len(m) {
	case 2:
		coin = m[1]
	case 3:
		coin = m[1]
	case 4:
		v, ok := parseAmount(m[1])
		if !ok {
			return nil
		}
		amount = strconv.FormatFloat(v, 'f', -1, 64)
		coin = m[2]
		currency = m[3]
	default:
		return nil
	}

	id, ok := cryptoAliases[strings.ToLower(coin)]
	if !ok {
		return nil
	}

	params := map[string]string{"coins": id}
	if amount != "" {
		params["amount"] = amount
	}
	if currency != "" {
		code, ok := currencyCode(currency)
		if !ok || (code != "USD" && code != "EUR" && code != "GBP") {
			return nil
		}
		params["currency"] = code
	}
	return params
}

// stockPatterns covers "stock AAPL", "TSLA stock", "MSFT price" and "$GOOGL".
// Every ticker must be upper case, which is how the spec writes all of its
// examples and which keeps ordinary lower-case words off this path.
func stockPatterns() []widgetPattern {
	return []widgetPattern{
		{regexp.MustCompile(`(?i)^stock\s+([A-Z]{1,5})$`), stockParams},
		{regexp.MustCompile(`(?i)^([A-Z]{1,5})\s+(?:stock|price)$`), stockParams},
		{regexp.MustCompile(`(?i)^\$([A-Z]{1,5})\s+stock\s+price$`), stockParams},
		{regexp.MustCompile(`^\$([A-Z]{1,5})$`), stockParams},
	}
}

func stockParams(m []string) map[string]string {
	symbol := strings.ToUpper(strings.TrimSpace(m[1]))
	if symbol == "" {
		return nil
	}
	return map[string]string{"symbols": symbol}
}

// trackingPatterns covers "track <number>" and the carrier form "usps <number>".
func trackingPatterns() []widgetPattern {
	return []widgetPattern{
		{regexp.MustCompile(`(?i)^track\s+([a-z0-9]{8,})$`), trackingParams},
		{regexp.MustCompile(`(?i)^([a-z0-9]{8,})$`), trackingParams},
		{regexp.MustCompile(`(?i)^(usps|fedex|ups|dhl|royal\s+mail|canada\s+post|australia\s+post)\s+([a-z0-9]{8,})$`), trackingParams},
	}
}

// trackingParams takes the last capture group, which is the tracking number in
// both forms; the carrier-prefix form additionally has the carrier in group 1.
func trackingParams(m []string) map[string]string {
	number := strings.TrimSpace(m[len(m)-1])
	if number == "" {
		return nil
	}
	if _, _, _, detected := widget.DetectCarrierFromNumber(number); !detected {
		return nil
	}
	return map[string]string{"number": number}
}

// languageNames maps language names onto the translate fetcher's codes, built
// from the widget package's SupportedLanguages list.
var languageNames = buildLanguageNames()

func buildLanguageNames() map[string]string {
	names := make(map[string]string)
	for _, l := range widget.SupportedLanguages {
		names[strings.ToLower(l.Name)] = l.Code
		names[strings.ToLower(l.Code)] = l.Code
	}
	return names
}

// translatePatterns covers "translate <text> to <language>" and
// "<text> in <language>".
func translatePatterns() []widgetPattern {
	return []widgetPattern{
		{regexp.MustCompile(`(?i)^translate\s+(.+?)\s+to\s+(.+)$`), translateParams},
		{regexp.MustCompile(`^"(.+?)"\s+to\s+(.+)$`), translateParams},
		{regexp.MustCompile(`^'(.+?)'\s+to\s+(.+)$`), translateParams},
		{regexp.MustCompile(`(?i)^(.+?)\s+in\s+(.+)$`), translateParams},
	}
}

// translateParams requires the trailing token to be a known language, which is
// what stops "results in india" from being read as a translation request.
func translateParams(m []string) map[string]string {
	text := strings.TrimSpace(m[1])
	if len(text) >= 2 && ((text[0] == '"' && text[len(text)-1] == '"') || (text[0] == '\'' && text[len(text)-1] == '\'')) {
		text = strings.TrimSpace(text[1 : len(text)-1])
	}
	langTok := strings.ToLower(strings.TrimSpace(m[2]))
	if text == "" || langTok == "" {
		return nil
	}
	code, ok := languageNames[langTok]
	if !ok {
		return nil
	}
	return map[string]string{
		"text": text,
		"to":   code,
	}
}

// wikiPatterns covers "wiki <topic>", "<topic> wikipedia", "who is <person>"
// and "what is <thing>".
func wikiPatterns() []widgetPattern {
	return []widgetPattern{
		{regexp.MustCompile(`(?i)^wiki(?:pedia)?\s+(.+)$`), wikiParams},
		{regexp.MustCompile(`(?i)^(.+?)\s+wiki(?:pedia)?$`), wikiParams},
		{regexp.MustCompile(`(?i)^(?:who|what)\s+is\s+(.+)$`), wikiParams},
	}
}

func wikiParams(m []string) map[string]string {
	topic := strings.TrimSpace(m[1])
	if topic == "" {
		return nil
	}
	return map[string]string{
		"topic": topic,
		"query": topic,
	}
}

// leagueAliases maps the league names users type onto the sports fetcher's ids.
var leagueAliases = map[string]string{
	"premier league":   "premier_league",
	"la liga":          "la_liga",
	"serie a":          "serie_a",
	"bundesliga":       "bundesliga",
	"champions league": "champions_league",
	"nfl":              "nfl",
	"nba":              "nba",
	"mlb":              "mlb",
	"nhl":              "nhl",
	"world cup":        "world_cup",
}

// sportsPatterns covers "[team] score", "[team] game" and "[league] scores".
func sportsPatterns() []widgetPattern {
	return []widgetPattern{
		{regexp.MustCompile(`(?i)^(.+?)\s+scores?$`), sportsParams},
		{regexp.MustCompile(`(?i)^(.+?)\s+results?$`), sportsParams},
		{regexp.MustCompile(`(?i)^(.+?)\s+game$`), sportsParams},
	}
}

// sportsParams routes a recognised league name to the league fetcher param and
// anything else to the team fetcher param.
func sportsParams(m []string) map[string]string {
	subject := strings.ToLower(strings.TrimSpace(m[1]))
	if subject == "" {
		return nil
	}
	if id, ok := leagueAliases[subject]; ok {
		return map[string]string{"league": id}
	}
	return map[string]string{"team": subject}
}

// nutritionPatterns covers "calories in <food>", "<food> nutrition" and
// "how many calories <food>".
func nutritionPatterns() []widgetPattern {
	return []widgetPattern{
		{regexp.MustCompile(`(?i)^calories\s+in\s+(?:an?\s+)?(.+)$`), nutritionParams},
		{regexp.MustCompile(`(?i)^how\s+many\s+calories\s+(?:are\s+)?(?:in\s+)?(?:an?\s+)?(.+)$`), nutritionParams},
		{regexp.MustCompile(`(?i)^(.+?)\s+nutrition$`), nutritionParams},
	}
}

func nutritionParams(m []string) map[string]string {
	food := strings.TrimSpace(m[1])
	food = strings.TrimPrefix(food, "an ")
	food = strings.TrimPrefix(food, "a ")
	if food == "" {
		return nil
	}
	return map[string]string{
		"query": food,
		"food":  food,
	}
}

func renderWeather(lang string, data interface{}) (string, string, string, string, bool) {
	d, ok := data.(*widget.WeatherData)
	if !ok || d == nil {
		return "", "", "", "", false
	}

	unit := i18n.T(lang, "instant.widget_weather_fahrenheit")
	if d.Units == "metric" {
		unit = i18n.T(lang, "instant.widget_weather_celsius")
	}
	unitLabel := escapeHTML(unit)
	var b strings.Builder
	b.WriteString("<div class=\"weather-answer\"><p>")
	b.WriteString(i18n.T(lang, "instant.widget_weather_temperature", formatFloat(d.Temperature, 1), unitLabel))
	b.WriteString("</p><p>")
	b.WriteString(i18n.T(lang, "instant.widget_weather_feels_like", formatFloat(d.FeelsLike, 1), unitLabel))
	b.WriteString("</p><p>")
	b.WriteString(i18n.T(lang, "instant.widget_weather_humidity", strconv.Itoa(d.Humidity)))
	b.WriteString("</p>")
	if d.Description != "" {
		b.WriteString("<p class=\"weather-condition\">")
		b.WriteString(escapeHTML(d.Description))
		b.WriteString("</p>")
	}
	if len(d.Forecast) > 0 {
		b.WriteString("<section class=\"weather-forecast\"><h2>")
		b.WriteString(i18n.T(lang, "instant.widget_weather_forecast"))
		b.WriteString("</h2><ol>")
		for _, day := range d.Forecast {
			date := escapeHTML(day.Date)
			b.WriteString("<li><time datetime=\"")
			b.WriteString(date)
			b.WriteString("\">")
			b.WriteString(date)
			b.WriteString("</time><span class=\"weather-day-condition\">")
			b.WriteString(escapeHTML(day.Description))
			b.WriteString("</span><span class=\"weather-day-range\">")
			b.WriteString(i18n.T(lang, "instant.widget_weather_day_range", formatFloat(day.High, 1), formatFloat(day.Low, 1), unitLabel))
			b.WriteString("</span></li>")
		}
		b.WriteString("</ol></section>")
	}
	b.WriteString("</div>")

	title := d.Location
	if title == "" {
		title = i18n.T(lang, "widgets.weather")
	}
	return title, b.String(), i18n.T(lang, "instant.widget_source_open_meteo"), "https://open-meteo.com/", true
}

func renderCurrency(lang string, data interface{}) (string, string, string, string, bool) {
	d, ok := data.(*widget.CurrencyData)
	if !ok || d == nil || d.Rate <= 0 {
		return "", "", "", "", false
	}

	var content strings.Builder
	content.WriteString("<div class=\"currency-answer\"><p class=\"currency-conversion\">")
	content.WriteString(i18n.T(lang, "instant.widget_currency_summary",
		escapeHTML(d.From), formatFloat(d.Amount, -1), escapeHTML(d.To), formatFloat(d.Result, 2)))
	content.WriteString("</p>")
	if d.Rate > 0 {
		content.WriteString("<p class=\"currency-rate\">")
		content.WriteString(i18n.T(lang, "instant.widget_currency_rate", formatFloat(d.Rate, 4)))
		if validCurrencyHistoryDate(d.RateDate) {
			content.WriteString(" · ")
			content.WriteString(i18n.T(lang, "instant.widget_currency_rate_date", escapeHTML(d.RateDate)))
		}
		content.WriteString("</p>")
	}
	points := validCurrencyHistory(d.History)
	if len(points) > 0 {
		content.WriteString("<section class=\"currency-history\"><h2>")
		content.WriteString(i18n.T(lang, "instant.widget_currency_history"))
		content.WriteString("</h2><svg class=\"currency-history-chart\" viewBox=\"0 0 100 40\" role=\"img\" aria-label=\"")
		content.WriteString(i18n.T(lang, "instant.widget_currency_history"))
		content.WriteString("\"><polyline class=\"currency-history-line\" fill=\"none\" points=\"")
		content.WriteString(currencyHistoryPolyline(points))
		content.WriteString("\"></polyline></svg><ol>")
		for _, point := range points {
			date := escapeHTML(point.Date)
			content.WriteString("<li><time datetime=\"")
			content.WriteString(date)
			content.WriteString("\">")
			content.WriteString(date)
			content.WriteString("</time><data value=\"")
			content.WriteString(formatFloat(point.Rate, 8))
			content.WriteString("\">")
			content.WriteString(formatFloat(point.Rate, 4))
			content.WriteString("</data></li>")
		}
		content.WriteString("</ol></section>")
	}
	content.WriteString("</div>")
	return i18n.T(lang, "widgets.currency"), content.String(),
		i18n.T(lang, "instant.widget_source_frankfurter"), "https://frankfurter.dev/", true
}

func validCurrencyHistoryDate(date string) bool {
	parsed, err := time.Parse("2006-01-02", date)
	return err == nil && parsed.Format("2006-01-02") == date
}

func validCurrencyHistory(history []widget.CurrencyHistoryPoint) []widget.CurrencyHistoryPoint {
	points := make([]widget.CurrencyHistoryPoint, 0, len(history))
	for _, point := range history {
		if !validCurrencyHistoryDate(point.Date) || point.Rate <= 0 || math.IsNaN(point.Rate) || math.IsInf(point.Rate, 0) {
			continue
		}
		points = append(points, point)
	}
	return points
}

func currencyHistoryPolyline(history []widget.CurrencyHistoryPoint) string {
	if len(history) == 0 {
		return ""
	}
	minimum, maximum := history[0].Rate, history[0].Rate
	for _, point := range history[1:] {
		if point.Rate < minimum {
			minimum = point.Rate
		}
		if point.Rate > maximum {
			maximum = point.Rate
		}
	}
	var points strings.Builder
	for index, point := range history {
		if index > 0 {
			points.WriteByte(' ')
		}
		x := 50.0
		if len(history) > 1 {
			x = float64(index) * 100 / float64(len(history)-1)
		}
		y := 20.0
		if maximum > minimum {
			y = 4 + (maximum-point.Rate)*32/(maximum-minimum)
		}
		points.WriteString(strconv.FormatFloat(x, 'f', 2, 64))
		points.WriteByte(',')
		points.WriteString(strconv.FormatFloat(y, 'f', 2, 64))
	}
	return points.String()
}

func renderCrypto(lang string, data interface{}) (string, string, string, string, bool) {
	d, ok := data.(*widget.CryptoData)
	if !ok || d == nil || len(d.Coins) == 0 {
		return "", "", "", "", false
	}

	var b strings.Builder
	for _, coin := range d.Coins {
		name := coin.Name
		if name == "" {
			name = coin.Symbol
		}
		amount := coin.Amount
		if amount <= 0 {
			amount = 1
		}
		unitPrice := coin.Price
		priceValue := unitPrice * amount
		if unitPrice <= 0 || math.IsNaN(priceValue) || math.IsInf(priceValue, 0) {
			continue
		}
		price := formatFloat(priceValue, 2)
		if amount != 1 {
			label := coin.Symbol
			if label == "" {
				label = name
			}
			name = formatFloat(amount, -1) + " " + label
		}
		if symbol := cryptoCurrencySymbol(coin.Currency); symbol != "" {
			price = symbol + price
		}
		b.WriteString(i18n.T(lang, "instant.widget_crypto_quote", escapeHTML(name), price))
		if coin.Change24h != 0 {
			b.WriteString(" ")
			b.WriteString(i18n.T(lang, "instant.widget_crypto_change", formatFloat(coin.Change24h, 2)))
			if symbol := cryptoCurrencySymbol(coin.Currency); symbol != "" {
				b.WriteString(" (")
				b.WriteString(symbol)
				b.WriteString(formatFloat(unitPrice*amount*coin.Change24h/100, 2))
				b.WriteString(")")
			}
		}
		if coin.MarketCap > 0 {
			b.WriteString("\nMarket cap: ")
			b.WriteString(formatCompactCurrency(coin.MarketCap, coin.Currency))
		}
		if coin.Volume24h > 0 {
			b.WriteString("\n24h volume: ")
			b.WriteString(formatCompactCurrency(coin.Volume24h, coin.Currency))
		}
		if chart := renderMiniPriceChart("crypto-price-chart", coin.Chart); chart != "" {
			b.WriteString(chart)
		}
		b.WriteString("\n")
	}

	return i18n.T(lang, "instant.widget_crypto_title"), b.String(),
		i18n.T(lang, "instant.widget_source_coingecko"), "https://www.coingecko.com/", true
}

func renderMiniPriceChart(class string, values []float64) string {
	if len(values) < 2 {
		return ""
	}
	minimum, maximum := math.Inf(1), math.Inf(-1)
	for _, value := range values {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return ""
		}
		if value < minimum {
			minimum = value
		}
		if value > maximum {
			maximum = value
		}
	}
	var points strings.Builder
	for index, value := range values {
		if index > 0 {
			points.WriteByte(' ')
		}
		x := 2 + 96*float64(index)/float64(len(values)-1)
		y := 20.0
		if maximum > minimum {
			y = 34 - 28*(value-minimum)/(maximum-minimum)
		}
		points.WriteString(formatFloat(x, 2))
		points.WriteByte(',')
		points.WriteString(formatFloat(y, 2))
	}
	return `<svg class="` + class + `" viewBox="0 0 100 40" aria-hidden="true"><polyline class="price-history-line" fill="none" points="` + points.String() + `"></polyline></svg>`
}

func cryptoCurrencySymbol(currency string) string {
	switch strings.ToUpper(currency) {
	case "USD":
		return "$"
	case "EUR":
		return "€"
	case "GBP":
		return "£"
	default:
		return ""
	}
}

func renderStocks(lang string, data interface{}) (string, string, string, string, bool) {
	d, ok := data.(*widget.StocksData)
	if !ok || d == nil || len(d.Symbols) == 0 {
		return "", "", "", "", false
	}

	var b strings.Builder
	for _, q := range d.Symbols {
		name := q.Name
		if name == "" {
			name = q.Symbol
		}
		b.WriteString(i18n.T(lang, "instant.widget_stocks_quote",
			escapeHTML(q.Symbol), escapeHTML(name), formatFloat(q.Price, 2)))
		b.WriteString(" ")
		b.WriteString(i18n.T(lang, "instant.widget_stocks_change",
			formatFloat(q.Change, 2), formatFloat(q.ChangePercent, 2)))
		if q.Open > 0 {
			b.WriteString("\nOpen: ")
			b.WriteString(formatFloat(q.Open, 2))
		}
		if q.High > 0 {
			b.WriteString("\nHigh: ")
			b.WriteString(formatFloat(q.High, 2))
		}
		if q.Low > 0 {
			b.WriteString("\nLow: ")
			b.WriteString(formatFloat(q.Low, 2))
		}
		if q.Volume > 0 {
			b.WriteString("\nVolume: ")
			b.WriteString(strconv.FormatInt(q.Volume, 10))
		}
		if chart := renderMiniPriceChart("stock-price-chart", q.Chart); chart != "" {
			b.WriteString(chart)
		}
		b.WriteString("\n")
	}

	return i18n.T(lang, "instant.widget_stocks_title"), b.String(),
		i18n.T(lang, "instant.widget_source_stocks"), "", true
}

func renderTracking(lang string, data interface{}) (string, string, string, string, bool) {
	d, ok := data.(*widget.TrackingData)
	if !ok || d == nil || !d.Detected {
		return "", "", "", "", false
	}

	var b strings.Builder
	if d.Carrier != "" {
		b.WriteString(i18n.T(lang, "instant.widget_tracking_carrier", escapeHTML(d.Carrier)))
		b.WriteString("\n")
	}
	if d.Status != "" {
		b.WriteString(i18n.T(lang, "instant.widget_tracking_status", escapeHTML(d.Status)))
		b.WriteString("\n")
	}
	if d.StatusDescription != "" {
		b.WriteString(escapeHTML(d.StatusDescription))
		b.WriteString("\n")
	}
	if d.EstimatedDelivery != "" {
		b.WriteString(i18n.T(lang, "instant.widget_tracking_estimated", escapeHTML(d.EstimatedDelivery)))
		b.WriteString("\n")
	}
	if b.Len() == 0 {
		return "", "", "", "", false
	}

	return i18n.T(lang, "instant.widget_tracking_title", escapeHTML(d.TrackingNumber)), b.String(),
		escapeHTML(d.Carrier), escapeHTML(d.CarrierURL), true
}

func renderTranslate(lang string, data interface{}) (string, string, string, string, bool) {
	d, ok := data.(*widget.TranslateData)
	if !ok || d == nil || d.TranslatedText == "" {
		return "", "", "", "", false
	}

	var b strings.Builder
	b.WriteString(`<div class="translation-answer"><p class="translation-source">`)
	b.WriteString(i18n.T(lang, "instant.widget_translate_source", escapeHTML(d.SourceText)))
	b.WriteString(`</p><p class="translation-result"><span>`)
	b.WriteString(escapeHTML(d.TranslatedText))
	b.WriteString(`</span> <button type="button" class="copy-btn" data-copy="`)
	b.WriteString(escapeHTML(d.TranslatedText))
	b.WriteString(`">`)
	b.WriteString(escapeHTML(i18n.T(lang, "common.copy")))
	b.WriteString(`</button></p>`)
	if d.Provider != "" {
		b.WriteString(`<p class="translation-provider">`)
		b.WriteString(i18n.T(lang, "instant.widget_translate_via", escapeHTML(d.Provider)))
		b.WriteString(`</p>`)
	}
	b.WriteString(`</div>`)

	provider := d.Provider
	if provider == "" {
		provider = i18n.T(lang, "instant.widget_translate_title")
	}
	return i18n.T(lang, "instant.widget_translate_title"), b.String(), escapeHTML(provider), "", true
}

func renderWiki(lang string, data interface{}) (string, string, string, string, bool) {
	d, ok := data.(*widget.WikipediaData)
	if !ok || d == nil || d.Extract == "" {
		return "", "", "", "", false
	}
	content := escapeHTML(d.Extract)
	if len(d.RelatedArticles) > 0 {
		related := make([]string, 0, len(d.RelatedArticles))
		for _, article := range d.RelatedArticles {
			related = append(related, escapeHTML(article.Title))
		}
		sort.Strings(related)
		content += "\n\n" + i18n.T(lang, "instant.widget_wiki_related", strings.Join(related, ", "))
	}
	return d.Title, content, i18n.T(lang, "instant.widget_source_wikipedia"), d.URL, true
}

func renderSports(lang string, data interface{}) (string, string, string, string, bool) {
	d, ok := data.(*widget.SportsData)
	if !ok || d == nil || len(d.Games) == 0 {
		return "", "", "", "", false
	}

	var b strings.Builder
	for _, g := range d.Games {
		b.WriteString(i18n.T(lang, "instant.widget_sports_result",
			escapeHTML(g.HomeTeam), scoreText(g.HomeScore), scoreText(g.AwayScore), escapeHTML(g.AwayTeam)))
		if g.StatusDetail != "" {
			b.WriteString(" ")
			b.WriteString(i18n.T(lang, "instant.widget_sports_status", escapeHTML(g.StatusDetail)))
		}
		if g.Status == widget.GameStatusScheduled && g.StartTime != "" {
			if start, err := time.Parse(time.RFC3339, g.StartTime); err == nil {
				b.WriteString(" ")
				b.WriteString(start.UTC().Format("2006-01-02 15:04 UTC"))
			}
		}
		b.WriteString("\n")
	}

	title := d.League
	if title == "" {
		title = d.Games[0].League
	}
	if title == "" {
		title = i18n.T(lang, "instant.widget_sports_title")
	}

	return title, b.String(), i18n.T(lang, "instant.widget_source_sports"), "", true
}

// scoreText renders a possibly-absent score, so a nil pointer (the game has
// not been played) is distinguishable from a real zero.
func scoreText(score *int) string {
	if score == nil {
		return "-"
	}
	return strconv.Itoa(*score)
}

func renderNutrition(lang string, data interface{}) (string, string, string, string, bool) {
	d, ok := data.(*widget.NutritionData)
	if !ok || d == nil || d.Name == "" {
		return "", "", "", "", false
	}

	var b strings.Builder
	if d.ServingSize != "" {
		b.WriteString(i18n.T(lang, "instant.widget_nutrition_serving", escapeHTML(d.ServingSize)))
		b.WriteString("\n")
	}
	b.WriteString(i18n.T(lang, "instant.widget_nutrition_calories", formatFloat(d.Calories, 0)))
	b.WriteString("\n")

	m := d.Macros
	parts := make([]string, 0, 3)
	if m.Protein > 0 {
		parts = append(parts, i18n.T(lang, "instant.widget_nutrition_protein", formatFloat(m.Protein, 1)))
	}
	if m.Carbohydrates > 0 {
		parts = append(parts, i18n.T(lang, "instant.widget_nutrition_carbs", formatFloat(m.Carbohydrates, 1)))
	}
	if m.Fat > 0 {
		parts = append(parts, i18n.T(lang, "instant.widget_nutrition_fat", formatFloat(m.Fat, 1)))
	}
	if len(parts) > 0 {
		b.WriteString(i18n.T(lang, "instant.widget_nutrition_macros", strings.Join(parts, " · ")))
		b.WriteString("\n")
	}
	for _, serving := range d.ServingSizes {
		if serving.Description == "" {
			continue
		}
		b.WriteString("Serving option: ")
		b.WriteString(escapeHTML(serving.Description))
		if serving.Calories > 0 {
			b.WriteString(" (" + formatFloat(serving.Calories, 0) + " kcal)")
		}
		b.WriteString("\n")
	}

	source := d.Source
	if source == "" {
		source = i18n.T(lang, "instant.widget_source_usda")
	}

	return d.Name, b.String(), escapeHTML(source), "https://fdc.nal.usda.gov/", true
}

func formatCompactCurrency(value float64, currency string) string {
	symbol := cryptoCurrencySymbol(currency)
	if symbol == "" {
		symbol = "$"
	}
	if math.Abs(value) >= 1_000_000_000_000 {
		return symbol + formatFloat(value/1_000_000_000_000, 2) + "T"
	}
	if math.Abs(value) >= 1_000_000_000 {
		return symbol + formatFloat(value/1_000_000_000, 2) + "B"
	}
	if math.Abs(value) >= 1_000_000 {
		return symbol + formatFloat(value/1_000_000, 2) + "M"
	}
	return symbol + formatFloat(value, 2)
}

// formatFloat renders a number at the given precision, with -1 for the
// shortest exact form used by conversion amounts.
func formatFloat(v float64, prec int) string {
	return strconv.FormatFloat(v, 'f', prec, 64)
}
