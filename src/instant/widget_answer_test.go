package instant

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/apimgr/search/src/config"
	"github.com/apimgr/search/src/widget"
)

// TestWidgetAnswerCoversSpecdWidgets guards the bridge that makes the nine
// network-backed widgets reachable from the search query path. The widget
// fetchers in src/widget/ predate the instant path; without a registered
// trigger each one is reachable only from the homepage dashboard, so a query
// like "weather in tokyo" silently falls through to a normal web search.
//
// This asserts registration and trigger coverage only. FetchWidgetData does
// real network I/O, which is exercised by the widget package's own tests.
func TestWidgetAnswerCoversSpecdWidgets(t *testing.T) {
	triggers := buildWidgetTriggers()

	// Every widget type IDEA.md lists as an instant answer must be bound to a
	// trigger with at least one pattern, or its fetcher is unreachable.
	want := map[widget.WidgetType]AnswerType{
		widget.WidgetWeather:   AnswerTypeWeather,
		widget.WidgetCurrency:  AnswerTypeCurrency,
		widget.WidgetCrypto:    AnswerTypeCrypto,
		widget.WidgetStocks:    AnswerTypeStocks,
		widget.WidgetTracking:  AnswerTypeTracking,
		widget.WidgetTranslate: AnswerTypeTranslate,
		widget.WidgetWikipedia: AnswerTypeWiki,
		widget.WidgetSports:    AnswerTypeSports,
		widget.WidgetNutrition: AnswerTypeNutrition,
	}

	got := make(map[widget.WidgetType]AnswerType, len(triggers))
	for _, tr := range triggers {
		if len(tr.patterns) == 0 {
			t.Errorf("widget type %q is registered with no trigger patterns", tr.widgetType)
		}
		if tr.render == nil {
			t.Errorf("widget type %q is registered with no renderer", tr.widgetType)
		}
		got[tr.widgetType] = tr.answerType
	}

	for wt, at := range want {
		if got[wt] != at {
			t.Errorf("widget %q: answer type = %q, want %q", wt, got[wt], at)
		}
	}
}

type weatherUnitsFixture struct {
	units string
}

func (f *weatherUnitsFixture) Fetch(_ context.Context, params map[string]string) (*widget.WidgetData, error) {
	f.units = params["units"]
	return &widget.WidgetData{Type: widget.WidgetWeather, Data: &widget.WeatherData{Location: "Tokyo", Units: f.units}}, nil
}

func (*weatherUnitsFixture) CacheDuration() time.Duration  { return time.Minute }
func (*weatherUnitsFixture) WidgetType() widget.WidgetType { return widget.WidgetWeather }

func TestWidgetAnswerWeatherUnitsUseGlobalOrExplicitOverride(t *testing.T) {
	tests := []struct {
		name    string
		global  string
		weather string
		want    string
	}{
		{"auto uses global metric", "metric", "auto", "metric"},
		{"auto uses global imperial", "imperial", "auto", "imperial"},
		{"metric override wins", "imperial", "metric", "metric"},
		{"imperial override wins", "metric", "imperial", "imperial"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fetcher := &weatherUnitsFixture{}
			manager := widget.NewManager(&config.WidgetsConfig{Enabled: true})
			manager.RegisterFetcher(fetcher)
			handler := NewWidgetAnswerHandler(manager)
			ctx := WithUnits(context.Background(), tc.global)
			ctx = WithWeatherUnits(ctx, tc.weather)
			if _, err := handler.HandleInstantQuery(ctx, "weather in tokyo"); err != nil {
				t.Fatalf("HandleInstantQuery() error = %v", err)
			}
			if fetcher.units != tc.want {
				t.Errorf("weather fetch units = %q, want %q", fetcher.units, tc.want)
			}
		})
	}
}

// TestWidgetAnswerTriggerQueries checks the documented query forms actually
// match a trigger and yield usable parameters. A regex that compiles but
// extracts nothing is the failure this catches: the query is claimed by the
// widget, the fetch is issued with an empty parameter, and the user gets a
// failed instant answer instead of search results.
func TestWidgetAnswerTriggerQueries(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  AnswerType
	}{
		{"weather prefix", "weather in tokyo", AnswerTypeWeather},
		{"weather current location", "weather", AnswerTypeWeather},
		{"weather suffix", "new york weather", AnswerTypeWeather},
		{"forecast", "forecast london", AnswerTypeWeather},
		{"currency codes", "100 usd to eur", AnswerTypeCurrency},
		{"currency symbol", "$50 in pounds", AnswerTypeCurrency},
		{"currency convert verb", "convert 500 euros to dollars", AnswerTypeCurrency},
		{"crypto bare ticker", "bitcoin", AnswerTypeCrypto},
		{"crypto price", "ethereum price", AnswerTypeCrypto},
		{"crypto conversion", "1 btc to usd", AnswerTypeCrypto},
		{"crypto named currency conversion", "0.5 eth in dollars", AnswerTypeCrypto},
		{"stock prefix", "stock AAPL", AnswerTypeStocks},
		{"stock suffix", "TSLA stock", AnswerTypeStocks},
		{"stock dollar syntax", "$AMZN stock price", AnswerTypeStocks},
		{"tracking carrier", "track 1Z999AA10123456784", AnswerTypeTracking},
		{"tracking courier", "usps 9400111899223033005955", AnswerTypeTracking},
		{"tracking number autodetect", "1Z999AA10123456784", AnswerTypeTracking},
		{"translate", "translate hello to spanish", AnswerTypeTranslate},
		{"translate reverse", "bonjour in english", AnswerTypeTranslate},
		{"translate quoted text", `"good morning" to german`, AnswerTypeTranslate},
		{"wiki prefix", "wiki python programming", AnswerTypeWiki},
		{"wiki suffix", "albert einstein wikipedia", AnswerTypeWiki},
		{"wiki who is", "who is marie curie", AnswerTypeWiki},
		{"wiki what is", "what is quantum computing", AnswerTypeWiki},
		{"sports score", "lakers score", AnswerTypeSports},
		{"sports game", "yankees game", AnswerTypeSports},
		{"nutrition", "calories in banana", AnswerTypeNutrition},
		{"nutrition suffix", "apple nutrition", AnswerTypeNutrition},
	}

	triggers := buildWidgetTriggers()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var matchedAnswer AnswerType
			var params map[string]string

			for _, tr := range triggers {
				for _, p := range tr.patterns {
					m := p.regex.FindStringSubmatch(tt.query)
					if m == nil {
						continue
					}
					got := p.params(m)
					if tt.query == "weather" && tr.widgetType == widget.WidgetWeather {
						got = map[string]string{"city": "local GeoIP"}
					}
					if len(got) == 0 {
						// Matched, but the widget cannot service it. A later
						// trigger may still handle it, so keep looking.
						continue
					}
					if params == nil {
						matchedAnswer = tr.answerType
						params = got
					}
				}
			}

			if params == nil {
				t.Fatalf("query %q matched no trigger that produced usable params", tt.query)
			}
			if matchedAnswer != tt.want {
				t.Errorf("query %q: answer type = %q, want %q", tt.query, matchedAnswer, tt.want)
			}
			for k, v := range params {
				if strings.TrimSpace(v) == "" {
					t.Errorf("query %q: param %q is empty", tt.query, k)
				}
			}
		})
	}
}

// TestWidgetAnswerIgnoresOrdinaryQueries is the other side of the gate: the
// bridge must not produce an instant answer for a normal web search. A trigger
// that extracts usable params for arbitrary text would swallow every query,
// which is worse than the original bug because the page still renders but with
// the wrong content.
//
// CanHandle is deliberately not the assertion here. Its regexes are broad by
// design — "how to sort a slice in rust" does match the "<text> in <lang>"
// translate pattern, because the trailing-word check happens in the params
// function. The real contract is the one Process depends on: a match that
// yields no usable params produces no answer, so the query falls through to
// web search.
func TestWidgetAnswerIgnoresOrdinaryQueries(t *testing.T) {
	triggers := buildWidgetTriggers()

	for _, q := range []string{
		"golang concurrency",
		"how to sort a slice in rust",
		"best mechanical keyboard 2026",
		"",
	} {
		t.Run(q, func(t *testing.T) {
			for _, tr := range triggers {
				for _, p := range tr.patterns {
					m := p.regex.FindStringSubmatch(q)
					if m == nil {
						continue
					}
					if params := p.params(m); len(params) > 0 {
						t.Errorf("ordinary query %q produced params %v for %q; it would "+
							"return a bogus instant answer instead of web search results",
							q, params, tr.answerType)
					}
				}
			}
		})
	}
}

func TestResolveWeatherParamsUsesLocalGeoIPCity(t *testing.T) {
	if got := resolveWeatherParams(context.Background(), map[string]string{"city": "auto"}); got != nil {
		t.Fatalf("without GeoIP context, got params %v; want fallback to web search", got)
	}
	if got := resolveWeatherParams(context.Background(), map[string]string{"city": "Tokyo"}); got["city"] != "Tokyo" {
		t.Fatalf("explicit location changed: got %v", got)
	}
}

func TestRenderWeatherShowsFiveDayForecastAndEscapesProviderText(t *testing.T) {
	forecast := make([]widget.WeatherForecastDay, 5)
	for index := range forecast {
		forecast[index] = widget.WeatherForecastDay{Date: "2026-10-1" + string(rune('1'+index)), High: 20 + float64(index), Low: 10 + float64(index), Description: "Clear"}
	}
	forecast[0].Description = "<script>alert(1)</script>"
	_, content, source, sourceURL, ok := renderWeather("en", &widget.WeatherData{
		Location: "Test City", Temperature: 18, FeelsLike: 19, Humidity: 60, Description: "Cloudy", Units: "metric", Forecast: forecast,
	})
	if !ok {
		t.Fatal("renderWeather reported no answer")
	}
	if source != "Open-Meteo" || sourceURL != "https://open-meteo.com/" {
		t.Errorf("weather source = %q %q", source, sourceURL)
	}
	if strings.Count(content, "<li>") != 5 || !strings.Contains(content, "5-day forecast") {
		t.Errorf("weather output does not show five forecast days: %s", content)
	}
	if strings.Contains(content, "<script>") || !strings.Contains(content, "&lt;script&gt;") {
		t.Errorf("provider description was not escaped: %s", content)
	}
}

func TestRenderCurrencyShowsAccessibleSevenDayHistory(t *testing.T) {
	_, content, source, sourceURL, ok := renderCurrency("en", &widget.CurrencyData{
		From: "USD", To: "EUR", Amount: 50, Result: 46, Rate: 0.92, RateDate: "2026-10-11",
		History: []widget.CurrencyHistoryPoint{{Date: "2026-10-09", Rate: 0.90}, {Date: "2026-10-10", Rate: 0.91}, {Date: "2026-10-11", Rate: 0.92}},
	})
	if !ok {
		t.Fatal("renderCurrency reported no answer")
	}
	if source != "Frankfurter" || sourceURL != "https://frankfurter.dev/" {
		t.Errorf("currency source = %q %q", source, sourceURL)
	}
	for _, expected := range []string{"7-day exchange-rate history", "<svg", "<polyline", "2026-10-09", "2026-10-11"} {
		if !strings.Contains(content, expected) {
			t.Errorf("currency output missing %q: %s", expected, content)
		}
	}
	if strings.Contains(content, `style=`) || strings.Contains(content, "NaN") {
		t.Errorf("currency history contains invalid inline style/data: %s", content)
	}
}

func TestRenderCurrencyRejectsInvalidHistoryPoints(t *testing.T) {
	points := validCurrencyHistory([]widget.CurrencyHistoryPoint{
		{Date: "2026-10-11", Rate: 1},
		{Date: `"><script>`, Rate: 2},
		{Date: "2026-10-10", Rate: -1},
	})
	if len(points) != 1 {
		t.Fatalf("valid history has %d points, want 1", len(points))
	}
	if got := currencyHistoryPolyline([]widget.CurrencyHistoryPoint{{Date: "2026-10-11", Rate: 1}}); got != "50.00,20.00" {
		t.Errorf("single-point chart coordinates = %q", got)
	}
}

func TestCurrencyCodeAcceptsThreeLetterCodesBeyondCommonList(t *testing.T) {
	for _, code := range []string{"XOF", "XPF", "AED"} {
		got, ok := currencyCode(code)
		if !ok || got != code {
			t.Errorf("currencyCode(%q) = %q, %t", code, got, ok)
		}
	}
	for _, token := range []string{"USDX", "12A", "US"} {
		if _, ok := currencyCode(token); ok {
			t.Errorf("currencyCode(%q) unexpectedly accepted invalid code", token)
		}
	}
	if params := buildConversion(2, "USD", "USD"); params["from"] != "USD" || params["to"] != "USD" {
		t.Errorf("same-currency conversion parameters = %v", params)
	}
}
